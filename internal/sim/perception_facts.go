package sim

// CaseView is a local docket entry or a lead the actor can cite.
type CaseView struct {
	SuspectID    string `json:"suspect_id"`
	Status       string `json:"status"`
	BasisEventID int64  `json:"basis_event_id,omitempty"`
}

// FeudView shows the parties and any terms already offered in a known local feud.
type FeudView struct {
	ID                  string `json:"id"`
	OffenderID          string `json:"offender_id"`
	AggrievedID         string `json:"aggrieved_id"`
	Intensity           int    `json:"intensity"`
	MediationPending    bool   `json:"mediation_pending,omitempty"`
	MediationProposerID string `json:"mediation_proposer_id,omitempty"`
	MediationCoins      int64  `json:"mediation_coins,omitempty"`
	OffenderAccepted    bool   `json:"offender_accepted,omitempty"`
	AggrievedAccepted   bool   `json:"aggrieved_accepted,omitempty"`
}

// HouseSite is the location and ownership on a known deed.
type HouseSite struct {
	ID         string   `json:"id"`
	District   string   `json:"district"`
	VenueID    string   `json:"venue_id,omitempty"`
	OwnerID    string   `json:"owner_id"`
	Facilities []string `json:"facilities,omitempty"`
}

func (h House) Site() HouseSite {
	return HouseSite{ID: h.ID, District: h.District, VenueID: h.VenueID, OwnerID: h.OwnerID, Facilities: h.Facilities}
}

func (p Perception) KnownHouse(id string) (HouseSite, bool) {
	for _, h := range p.AccessibleHouses {
		if h.ID == id {
			return h.Site(), true
		}
	}
	for _, h := range p.KnownHouses {
		if h.ID == id {
			return h, true
		}
	}
	return HouseSite{}, false
}

func (p Perception) KnownWorkshop(facility string) (HouseSite, bool) {
	for _, h := range p.AccessibleHouses {
		if h.HasFacility(facility) {
			return h.Site(), true
		}
	}
	for _, h := range p.KnownHouses {
		if ActorHasTag(h.Facilities, facility) {
			return h, true
		}
	}
	return HouseSite{}, false
}

func (w *World) projectLocalFacts(p *Perception) {
	a := p.Self
	p.CreditTerms = DefaultMarketCreditSpec()
	if w.MarketCredit != nil {
		p.CreditTerms = EffectiveMarketCreditSpec(w.MarketCredit.Spec)
	}
	if w.Production.LaborTick == w.Tick {
		p.LaborUsed = w.Production.LaborUsed[holderKey("actor", a.ID)]
	}
	w.projectTradeAndCare(p)
	if w.Relations != nil {
		kin := w.Relations.Kinship[a.ID]
		for i := range p.Nearby {
			n := &p.Nearby[i]
			peer := w.Relations.Kinship[n.ID]
			n.Married = peer != nil && peer.SpouseID != ""
			switch {
			case kin != nil && kin.SpouseID == n.ID:
				n.KinshipToSelf = "spouse"
			case kin != nil && (ActorHasTag(kin.ParentIDs, n.ID) || ActorHasTag(kin.ChildIDs, n.ID)):
				n.KinshipToSelf = "family"
			case kin != nil && ActorHasTag(kin.SwornIDs, n.ID) || peer != nil && ActorHasTag(peer.SwornIDs, a.ID):
				n.KinshipToSelf = "sworn"
			}
		}
		for i := range p.Factions {
			f := &p.Factions[i]
			if !f.Joined || f.District != a.Location || !ActorHasTag(a.Guilds, f.ID) {
				continue
			}
			leader := w.Relations.FactionLeader[f.ID]
			if leader == "" || p.localNeighbor(leader) != nil {
				f.LeadershipContestOpen = w.canChallengeLeadership(a, f.ID)
			}
		}
		p.KnownFeuds = w.knownLocalFeuds(a)
	}
	p.LocalCases = w.knownLocalCases(a, p.Nearby)
	if w.JusticeRevolt != nil && !a.InTransit() {
		leads := ActorKnownWrongfulLeads(a)
		for i := len(w.JusticeRevolt.CourtCases) - 1; i >= 0 && len(leads) > 0; i-- {
			cc := w.JusticeRevolt.CourtCases[i]
			if (cc.Status != "convicted" && cc.Status != "served") || cc.District != a.Location {
				continue
			}
			if lead, ok := matchKnownLeadToCourtCase(leads, cc); ok {
				lead.SuspectID, lead.District = cc.SuspectID, cc.District
				p.KnownWrongfulCases = append(p.KnownWrongfulCases, lead)
			}
		}
	}
	p.Interactions = BuildInteractionViews(*p)
	p.ProductionBlockReasons = w.ProductionStartBlockReasons(a)
	p.ActivePlan = BuildPlanDirective(*p)
}

func (p Perception) localNeighbor(id string) *Neighbor {
	for i := range p.Nearby {
		if p.Nearby[i].ID == id {
			return &p.Nearby[i]
		}
	}
	return nil
}

func (w *World) projectTradeAndCare(p *Perception) {
	a := p.Self
	for _, o := range p.Offers {
		if o.Expires >= p.Tick && o.Seller != a.ID && p.localNeighbor(o.Seller) != nil {
			if p.Continuity.DisplayedGoods == nil {
				p.Continuity.DisplayedGoods = make(map[int64]int)
			}
			// The display and the offer each show at most the quantity on offer.
			p.Continuity.DisplayedGoods[o.ID] = min(o.Quantity, w.actor(o.Seller).Items[o.Item])
		}
	}
	signed := make(map[int64]bool, len(w.Production.CareAgreements))
	for _, terms := range w.Production.CareAgreements {
		if !terms.OwnerAccepted || !terms.CaretakerAccepted {
			continue
		}
		signed[terms.GrowthID] = true
		if terms.OwnerKind == "actor" && terms.OwnerID == a.ID ||
			terms.CaretakerKind == "actor" && terms.CaretakerID == a.ID ||
			terms.SupplierKind == "actor" && terms.SupplierID == a.ID {
			p.CareAgreements = append(p.CareAgreements, terms)
		}
	}
	for _, proposal := range w.Production.CareProposals {
		// Signing closes competing invitations, including stale copies in older snapshots.
		if signed[proposal.GrowthID] {
			continue
		}
		if proposal.OwnerKind == "actor" && proposal.OwnerID == a.ID ||
			proposal.CaretakerKind == "actor" && proposal.CaretakerID == a.ID {
			p.Continuity.CareProposals = append(p.Continuity.CareProposals, proposal)
		}
	}
}

// localFlowEdges returns only FlowGrid edges incident to a node in actor a's current district,
// preventing remote global flow-network topology/severance leaks into Perception.
func (w *World) localFlowEdges(a Actor) []FlowEdge {
	if w == nil || w.FlowGrid == nil || len(w.FlowGrid.Edges) == 0 {
		return nil
	}
	localNodes := map[string]bool{}
	for _, n := range w.FlowGrid.Nodes {
		distID := n.DistrictID
		if distID == "" && n.VenueID != "" {
			if v, ok := w.Map.VenueByID(n.VenueID); ok {
				distID = v.DistrictID
			}
		}
		if distID == "" || distID == a.Location {
			localNodes[n.ID] = true
		}
	}
	if len(localNodes) == 0 {
		return nil
	}
	var out []FlowEdge
	for _, e := range w.FlowGrid.Edges {
		if localNodes[e.FromNodeID] || localNodes[e.ToNodeID] {
			out = append(out, e)
		}
	}
	return out
}

// knownLocalFeuds exposes known feuds whose parties can talk here.
func (w *World) knownLocalFeuds(a Actor) []FeudView {
	if w.Relations == nil {
		return nil
	}
	var feuds []FeudView
	for _, g := range w.Relations.Grudges {
		if g.Resolved || !actorKnowsAboutGrudge(a, g) {
			continue
		}
		partiesPresent := true
		for _, id := range []string{g.OffenderID, g.AggrievedID} {
			party := w.actor(id)
			if party == nil || party.Dead || w.Tick < party.JailedUntil || !SamePlace(a, *party) || party.Task != nil && (party.Task.Action.Kind == "sleep" || party.Task.Dormant) {
				partiesPresent = false
				break
			}
		}
		if !partiesPresent {
			continue
		}
		feud := FeudView{ID: g.ID, OffenderID: g.OffenderID, AggrievedID: g.AggrievedID, Intensity: g.Intensity, MediationPending: g.MediationProposerID != ""}
		if a.ID == g.OffenderID || a.ID == g.AggrievedID {
			feud.MediationProposerID, feud.MediationCoins = g.MediationProposerID, g.MediationCoins
			feud.OffenderAccepted, feud.AggrievedAccepted = g.OffenderAccepted, g.AggrievedAccepted
		}
		feuds = append(feuds, feud)
	}
	return feuds
}

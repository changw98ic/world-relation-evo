package sim

import (
	"fmt"
	"strings"
)

// actionPeerView abstracts co-located target state for single-source action checks
// across both Perception (candidate generation) and World (authoritative execution).
type actionPeerView struct {
	ID              string
	CanArrest       bool
	Dead            bool
	Health          int
	DiseaseID       string
	Jailed          bool
	Sleeping        bool
	Tags            []string
	FactionRelation string
	KinshipToSelf   string
	PatronageToSelf string
}

// actionCheckEnv supplies the read-only facts needed by checkActionShared so that
// Perception.CheckAction and World.CheckAction execute the exact same condition logic.
type actionCheckEnv struct {
	tick                   int64
	checkTransmission      func(Action) string
	actor                  Actor
	catalog                Catalog
	worldMap               WorldMap
	worldLaws              map[string]int
	capabilities           map[string]bool
	districtState          *DistrictState
	currentHouse           *House
	accessibleHouse        func(houseID string) (House, bool)
	faction                func(factionID string) (FactionView, bool)
	peer                   func(targetID string) (actionPeerView, bool)
	openQuestion           func(peer string) (Memory, bool)
	offer                  func(offerID int64) (Offer, bool)
	agreements             []Agreement
	offerBlockReason       func(offer Offer) string
	marketLotAvailable     func(Action) bool
	recipeBlockReason      func(r Recipe) string
	actionSpecBlockReason  func(x Action, spec ActionSpec) string
	knownTechs             []string
	isEmbargoed            func(itemID string) bool
	roadCost               func(toDistrict string) (energyCost int, ok bool)
	venueRouteCost         func(toVenue, modeID string) (energyCost int, ok bool)
	hasActiveLoan          bool
	hasOpenWrongfulLead    bool
	districtController     string
	canBribeSecretWitness  func(targetID, secretID string) bool
	activeWorldEvent       func(instanceID string) (ActiveWorldEvent, bool)
	partyByID              func(partyID string) (Party, bool)
	actorPartyID           string
	leadsFaction           func(factionID string) bool
	diplomacyBetween       func(facA, facB string) (FactionDiplomacy, bool)
	hasKnownInvasion       bool
	bribeShielded          bool
	localCorruption        int
	corruptionFramingMin   int
	canEnforceLaw          bool
	canAppeal              bool
	isFreeWaterAvailable   bool
	checkTargetAcceptsCost bool // true on authoritative World, false when Perception doesn't know remote trust
}

// CheckAction evaluates single-source preconditions for action x from a Perception snapshot.
func (p Perception) CheckAction(x Action) ActionCheckResult {
	return checkActionShared(p.buildActionCheckEnv(), x, p.Self.ID)
}

// ActionChecker builds the shared precondition context once for one view.
func (p Perception) ActionChecker() func(Action) ActionCheckResult {
	env, id := p.buildActionCheckEnv(), p.Self.ID
	return func(x Action) ActionCheckResult {
		return checkActionShared(env, x, id)
	}
}

// CheckAction evaluates the same single-source preconditions for action x against authoritative World state.
func (w *World) CheckAction(actorID string, x Action) ActionCheckResult {
	if w == nil {
		return BlockAction("world is nil")
	}
	a := w.actor(actorID)
	if a == nil {
		return BlockAction("unknown actor: " + actorID)
	}
	return w.checkActorAction(a, x, w.Relations)
}

func (w *World) checkActorAction(a *Actor, x Action, relations *RelationEvoState) ActionCheckResult {
	if w.storedActorCohort(a.ID) != nil {
		return BlockAction("dehydrated cohort member must be hydrated before acting")
	}
	env := w.buildActionCheckEnv(*a, relations)
	return checkActionShared(env, x, a.ID)
}

func (p Perception) buildActionCheckEnv() actionCheckEnv {
	a := p.Self
	envSpec := p.Catalog.EffectiveEnvironment()
	var jrRaw JusticeRevoltSpec
	if p.JusticeRules != nil {
		jrRaw = *p.JusticeRules
	}
	jrSpec := EffectiveJusticeRevoltSpec(jrRaw)
	localCorr := 0
	bribeShielded := false
	if p.DistrictState != nil {
		localCorr = p.DistrictState.Corruption
		bribeShielded = p.DistrictState.BribeShieldTicks > 0
	}
	selfTags := p.EffectiveTags()
	selfCaps := p.Capabilities()
	canEnforce := p.Catalog.IsEnforcementTags(selfTags) || actorHasAnyTag(selfTags, envSpec.EnforcementTags) || selfCaps[jrSpec.LawCapability] || selfCaps["frame_suspect"]
	canAppeal := ActorHasTag(selfTags, jrSpec.OversightTag) || p.Catalog.IsOversightTags(selfTags) || selfCaps[jrSpec.OversightCapability] || selfCaps["file_appeal"] || p.Catalog.HasPublicTrustDuty(selfTags)

	myPartyID := ""
	for _, pt := range p.Parties {
		if ActorHasTag(pt.MemberIDs, a.ID) {
			myPartyID = pt.ID
			break
		}
	}
	hasInvasion := false
	for _, we := range p.ActiveWorldEvents {
		if we.Category == "external_invasion" {
			hasInvasion = true
			break
		}
	}
	freeWater := p.HasPotableWater()

	return actionCheckEnv{
		checkTransmission: func(x Action) string {
			local := World{Tick: p.Tick, Map: p.Map, ActiveEffects: p.ActiveEffects}
			if p.CurrentHouse != nil {
				local.Houses = []House{*p.CurrentHouse}
			}
			_, shielding := local.actorEnclosure(a)
			ch, reason := transmissionChannel(p.Catalog, a, selfTags, shielding, x)
			if reason == "" && !ch.Broadcast && p.localNeighbor(x.TargetID) == nil && !ActorHasTag(p.CommunicationContacts, x.TargetID) {
				return "unknown channel recipient"
			}
			return reason
		},
		tick:          p.Tick,
		actor:         a,
		catalog:       p.Catalog,
		worldMap:      p.Map,
		worldLaws:     p.WorldLaws,
		capabilities:  p.Capabilities(),
		districtState: p.DistrictState,
		currentHouse:  p.CurrentHouse,
		knownTechs:    p.KnownTechs,
		openQuestion: func(peer string) (Memory, bool) {
			question, open := p.OpenQuestions[peer]
			return question, open
		},
		marketLotAvailable: func(x Action) bool {
			for _, lot := range p.MarketLots {
				if sameMarketLot(x, lot) {
					return true
				}
			}
			return false
		},
		accessibleHouse: func(houseID string) (House, bool) {
			if p.CurrentHouse != nil && p.CurrentHouse.ID == houseID {
				return *p.CurrentHouse, true
			}
			for _, h := range p.AccessibleHouses {
				if h.ID == houseID {
					return h, true
				}
			}
			return House{}, false
		},
		faction: func(factionID string) (FactionView, bool) {
			for _, f := range p.Factions {
				if f.ID == factionID {
					return f, true
				}
			}
			return FactionView{}, false
		},
		peer: func(targetID string) (actionPeerView, bool) {
			for _, nb := range p.Nearby {
				if nb.ID == targetID {
					return actionPeerView{
						ID:              nb.ID,
						Health:          nb.Health,
						Jailed:          nb.Jailed,
						Sleeping:        nb.Sleeping,
						Tags:            nb.Tags,
						FactionRelation: nb.FactionRelation,
						KinshipToSelf:   nb.KinshipToSelf,
						PatronageToSelf: nb.PatronageToSelf,
					}, true
				}
			}
			return actionPeerView{}, false
		},
		offer: func(offerID int64) (Offer, bool) {
			for _, o := range p.Offers {
				if o.ID == offerID {
					return o, true
				}
			}
			return Offer{}, false
		},
		offerBlockReason: func(offer Offer) string {
			return p.OfferAcceptanceBlockReason(offer)
		},
		agreements: p.Continuity.Agreements,
		recipeBlockReason: func(r Recipe) string {
			return p.RecipeBlockReason(r)
		},
		actionSpecBlockReason: func(x Action, spec ActionSpec) string {
			return p.ActionSpecBlockReason(spec)
		},
		isEmbargoed: func(itemID string) bool {
			if p.Polity == nil {
				return false
			}
			return ActorHasTag(p.Polity.EmbargoItems, itemID)
		},
		roadCost: func(toDistrict string) (int, bool) {
			for _, r := range p.Roads {
				if r.To == toDistrict {
					return r.EnergyCost, true
				}
			}
			return 0, false
		},
		venueRouteCost: func(toVenue, modeID string) (int, bool) {
			for _, vr := range p.VenueRoutes {
				if vr.ToVenue == toVenue && (modeID == "" || vr.ModeID == modeID) {
					return vr.EnergyCost, true
				}
			}
			return 0, false
		},
		hasActiveLoan:       p.HasActiveLoan,
		hasOpenWrongfulLead: len(p.KnownWrongfulCases) > 0 || len(ActorKnownWrongfulLeads(a)) > 0,
		districtController:  p.DistrictController,
		canBribeSecretWitness: func(targetID, secretID string) bool {
			for _, s := range p.KnownSecrets {
				if s.OwnerID == a.ID && !s.Exposed && ActorHasTag(s.WitnessedBy, targetID) &&
					(secretID == "" || secretID == s.ID) {
					return true
				}
			}
			return false
		},
		activeWorldEvent: func(instanceID string) (ActiveWorldEvent, bool) {
			for _, we := range p.ActiveWorldEvents {
				if (instanceID == "" || we.InstanceID == instanceID) && we.Category == "artifact_emergence" && we.DistrictID == a.Location && we.ClaimedBy == "" {
					return we, true
				}
			}
			return ActiveWorldEvent{}, false
		},
		partyByID: func(partyID string) (Party, bool) {
			for _, pt := range p.Parties {
				if pt.ID == partyID {
					return pt, true
				}
			}
			return Party{}, false
		},
		actorPartyID: myPartyID,
		leadsFaction: func(factionID string) bool {
			return ActorHasTag(p.LedFactions, factionID)
		},
		diplomacyBetween: func(facA, facB string) (FactionDiplomacy, bool) {
			for _, d := range p.FactionDiplomacy {
				if (d.FactionA == facA && d.FactionB == facB) || (d.FactionA == facB && d.FactionB == facA) {
					return d, true
				}
			}
			return FactionDiplomacy{}, false
		},
		hasKnownInvasion:     hasInvasion,
		bribeShielded:        bribeShielded,
		localCorruption:      localCorr,
		corruptionFramingMin: jrSpec.CorruptionFramingThreshold,
		canEnforceLaw:        canEnforce,
		canAppeal:            canAppeal,
		isFreeWaterAvailable: freeWater,
	}
}

func (w *World) buildActionCheckEnv(a Actor, relations *RelationEvoState) actionCheckEnv {
	var agreements []Agreement
	if w.Continuity != nil {
		agreements = w.Continuity.Agreements
	}
	var dsPtr *DistrictState
	localCorr := 0
	bribeShielded := false
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[a.Location]; ok {
			dsc := ds
			dsPtr = &dsc
			localCorr = ds.Corruption
			bribeShielded = ds.BribeShieldTicks > 0
		}
	}
	var curHouse *House
	if a.InsideHouse != "" {
		if h := w.house(a.InsideHouse); h != nil {
			hc := *h
			curHouse = &hc
		}
	}
	jrRaw := JusticeRevoltSpec{}
	if w.JusticeRevolt != nil && w.JusticeRevolt.Spec != nil {
		jrRaw = *w.JusticeRevolt.Spec
	}
	jrSpec := EffectiveJusticeRevoltSpec(jrRaw)
	hasActiveLoan := false
	if w.MarketCredit != nil {
		for _, l := range w.MarketCredit.Loans {
			if l.Status == "active" && l.BorrowerID == a.ID && l.Principal+l.AccruedInterest-l.Repaid > 0 {
				hasActiveLoan = true
				break
			}
		}
	}

	myPartyID := ""
	for _, pt := range w.Parties {
		if ActorHasTag(pt.MemberIDs, a.ID) {
			myPartyID = pt.ID
			break
		}
	}
	hasInvasion := false
	for _, we := range w.perceivedActiveWorldEvents(a) {
		if we.Category == "external_invasion" {
			hasInvasion = true
			break
		}
	}
	freeWater := dsPtr != nil && w.potablePoolID(*dsPtr) != ""

	return actionCheckEnv{
		agreements:   agreements,
		openQuestion: a.OpenQuestionWith,
		marketLotAvailable: func(x Action) bool {
			c := w.findCohort(x.TargetID)
			lot, ok := w.cohortMarketLot(c, x.Item, x.Kind)
			return ok && sameMarketLot(x, lot) && (x.Kind != "market_buy" || w.buyerItemBlockReason(a, x.Item, "coins") == "")
		},
		checkTransmission: func(x Action) string {
			_, shielding := w.actorEnclosure(a)
			ch, reason := transmissionChannel(w.Catalog, a, w.actorEffectiveTags(a), shielding, x)
			if reason != "" || ch.Broadcast {
				return reason
			}
			target := w.actor(x.TargetID)
			if target == nil || target.Dead {
				return "unknown or dead channel recipient"
			}
			if !w.communicationRangeAllows(a, *target, ch.RangeKm) {
				return "recipient is outside the channel range or its distance cannot be verified"
			}
			return ""
		},
		tick:          w.Tick,
		actor:         a,
		catalog:       w.Catalog,
		worldMap:      w.Map,
		worldLaws:     w.EffectiveWorldLaws(),
		capabilities:  w.ActorCapabilities(a),
		districtState: dsPtr,
		currentHouse:  curHouse,
		knownTechs:    w.ActorKnownTechs(a.ID),
		accessibleHouse: func(houseID string) (House, bool) {
			h := w.house(houseID)
			if h == nil || !h.CanAccess(a.ID) {
				return House{}, false
			}
			return *h, true
		},
		faction: func(factionID string) (FactionView, bool) {
			f := w.faction(factionID)
			if f == nil {
				return FactionView{}, false
			}
			return FactionView{
				ID:              f.ID,
				Name:            f.Name,
				District:        f.District,
				Level:           f.Level,
				Professions:     f.Professions,
				Requirements:    f.Requirements,
				Joined:          f.HasMember(a.ID),
				JoinBlockReason: w.FactionJoinEligibilityReason(a, *f),
				Members:         len(f.Members),
			}, true
		},
		peer: func(targetID string) (actionPeerView, bool) {
			b := w.actor(targetID)
			if b == nil || b.ID == a.ID || !SamePlace(a, *b) {
				return actionPeerView{}, false
			}
			var kinship, patronage string
			if relations != nil {
				kinship, patronage = personalRelationship(a.ID, b.ID, relations.Kinship[a.ID], relations.Kinship[b.ID])
			}
			return actionPeerView{
				ID:              b.ID,
				CanArrest:       w.ActorCapabilities(*b)["arrest"],
				Dead:            b.Dead,
				Health:          b.Health,
				DiseaseID:       b.DiseaseID,
				Jailed:          w.Tick < b.JailedUntil,
				Sleeping:        b.Task != nil && (b.Task.Action.Kind == "sleep" || b.Task.Dormant),
				Tags:            w.neighborEffectiveTags(*b),
				FactionRelation: w.actorFactionRelation(a, *b),
				KinshipToSelf:   kinship,
				PatronageToSelf: patronage,
			}, true
		},
		offer: func(offerID int64) (Offer, bool) {
			for _, o := range w.Offers {
				if o.ID == offerID && o.Expires >= w.Tick {
					return o, true
				}
			}
			return Offer{}, false
		},
		offerBlockReason: func(offer Offer) string {
			buyer := w.actor(offer.Buyer)
			if buyer == nil {
				return "buyer not found"
			}
			return w.offerSettlementBlockReason(*buyer, offer)
		},
		recipeBlockReason: func(r Recipe) string {
			return recipeBlockReason(w.Catalog, w.Map, curHouse, a, r, w.productionStartBlockReason(a, r), w.availableDistrictResource(a.Location, r.ResourcePool), w.FacilityProductionBonusPct(a))
		},
		actionSpecBlockReason: func(x Action, spec ActionSpec) string {
			if reason := w.ActionSpecBlockReason(a, x, spec); reason != "" {
				return reason
			}
			return w.declarativeTargetFailure(a, &x, spec)
		},
		isEmbargoed: func(itemID string) bool {
			return w.IsItemEmbargoedAt(a.Location, a.VenueID, itemID)
		},
		roadCost: func(toDistrict string) (int, bool) {
			r, ok := w.Map.RoadBetween(a.Location, toDistrict)
			if !ok {
				return 0, false
			}
			if closed, _ := w.isRouteSevered(a.Location, r.To, r.Medium, ""); closed {
				return 0, false
			}
			_, effEnergy := w.effectiveRoadCost(r)
			return effEnergy, true
		},
		venueRouteCost: func(toVenue, modeID string) (int, bool) {
			for _, vr := range w.actorVenueRoutes(a) {
				if vr.ToVenue == toVenue && (modeID == "" || vr.ModeID == modeID) {
					return vr.EnergyCost, true
				}
			}
			return 0, false
		},
		hasActiveLoan:       hasActiveLoan,
		hasOpenWrongfulLead: len(ActorKnownWrongfulLeads(a)) > 0,
		districtController:  relations.DistrictController(a.Location),
		canBribeSecretWitness: func(targetID, secretID string) bool {
			for _, s := range w.Secrets {
				if s.OwnerID == a.ID && !s.Exposed && ActorHasTag(s.WitnessedBy, targetID) &&
					(secretID == "" || secretID == s.ID) {
					return true
				}
			}
			return false
		},
		activeWorldEvent: func(instanceID string) (ActiveWorldEvent, bool) {
			for _, we := range w.perceivedActiveWorldEvents(a) {
				if (instanceID == "" || we.InstanceID == instanceID) && we.Category == "artifact_emergence" && we.DistrictID == a.Location && we.ClaimedBy == "" {
					return we, true
				}
			}
			return ActiveWorldEvent{}, false
		},
		partyByID: func(partyID string) (Party, bool) {
			for _, pt := range w.Parties {
				if pt.ID == partyID {
					return pt, true
				}
			}
			return Party{}, false
		},
		actorPartyID: myPartyID,
		leadsFaction: func(factionID string) bool {
			return relations != nil && relations.FactionLeader[factionID] == a.ID
		},
		diplomacyBetween: func(facA, facB string) (FactionDiplomacy, bool) {
			for _, d := range w.perceivedFactionDiplomacy(a) {
				if (d.FactionA == facA && d.FactionB == facB) || (d.FactionA == facB && d.FactionB == facA) {
					return d, true
				}
			}
			return FactionDiplomacy{}, false
		},
		hasKnownInvasion:       hasInvasion,
		bribeShielded:          bribeShielded,
		localCorruption:        localCorr,
		corruptionFramingMin:   jrSpec.CorruptionFramingThreshold,
		canEnforceLaw:          w.actorCanEnforceLaw(a, jrSpec),
		canAppeal:              w.actorCanAppeal(a, jrSpec),
		isFreeWaterAvailable:   freeWater,
		checkTargetAcceptsCost: true,
	}
}

// checkActionShared is the single canonical business-precondition evaluator for
// both Perception.CheckAction (candidate filtering) and World.CheckAction (execution).
func checkActionShared(env actionCheckEnv, x Action, actorID string) ActionCheckResult {
	a := env.actor
	if a.Dead {
		return BlockAction("dead actors cannot act")
	}
	if env.tick < a.JailedUntil {
		return BlockAction(fmt.Sprintf("jailed until tick %d", a.JailedUntil))
	}
	if a.Task != nil {
		return BlockAction("busy with " + a.Task.Action.Kind)
	}
	if !env.capabilities[x.Kind] {
		return BlockAction("action not permitted for actor capabilities: " + x.Kind)
	}
	lawChecker := World{Catalog: env.catalog, WorldLaws: env.worldLaws}
	if blocked, reason := lawChecker.IsActionBlockedByWorldLaw(a, x); blocked {
		return BlockAction(reason)
	}
	isReply := x.Kind == "speak" && x.Statement != nil && x.Statement.Topic == ConversationReply
	if !isReply {
		if err := ValidateCitation(a.Memories, x.BasisEventID, x.Statement); err != nil {
			return BlockAction(err.Error())
		}
	}
	if x.WorkOrderID != "" {
		order, ok := env.catalog.WorkOrder(x.WorkOrderID)
		if !ok || !order.applies(a, x, env.worldMap) {
			return BlockAction("the action does not match the posted work")
		}
	}

	var rate actionWorkRate
	if builtinActionMetadataIndex[x.Kind].referenceEnergy > 0 {
		work := x
		switch x.Kind {
		case "accept":
			if offer, ok := env.offer(x.OfferID); ok {
				work.Item, work.Quantity = offer.Item, offer.Quantity
			}
		case "fulfill_agreement":
			work = withAgreementCargo(x, env.agreements)
		}
		targetNearby := true
		if x.Kind == "teach_knowledge" && x.TechID != "" && x.Channel != "" {
			_, targetNearby = env.peer(x.TargetID)
		}
		var ticks int
		rate, ticks = env.catalog.actionWork(work, a, targetNearby)
		if energy := rate.energyAfter(ticks); a.Energy < energy {
			return BlockAction(fmt.Sprintf("insufficient energy for %s: needs %d, has %d", x.Kind, energy, a.Energy))
		}
	}
	result := checkActionDetails(env, x, actorID, isReply)
	result.workRate = rate
	return result
}

func checkActionDetails(env actionCheckEnv, x Action, actorID string, isReply bool) ActionCheckResult {
	a := env.actor
	envProfile := env.catalog.EffectiveEnvironment()

	switch x.Kind {
	case "market_buy", "market_sell":
		if a.InsideHouse != "" || a.InTransit() || a.Location != x.Location || env.isEmbargoed(x.Item) || !env.marketLotAvailable(x) {
			return BlockAction("the posted market lot is unavailable at this place")
		}
		if x.Kind == "market_buy" && a.Money < x.Price || x.Kind == "market_sell" && a.Items[x.Item] < x.Quantity {
			return BlockAction("insufficient holdings for the posted market lot")
		}
		return AllowAction()
	case "speak":
		listener, ok := env.peer(x.TargetID)
		if !ok || listener.Dead || listener.Jailed {
			return BlockAction("listener is not nearby")
		}
		if listener.Sleeping {
			return BlockAction("listener cannot hear speech")
		}
		if x.Text == "" {
			return BlockAction("empty speech")
		}
		if isReply {
			question, open := env.openQuestion(x.TargetID)
			if !open || question.Source != x.TargetID || question.EventID != x.BasisEventID {
				return BlockAction("the reply must answer an open question from its listener")
			}
		}
		return AllowAction()

	case "transmit_message":
		if reason := env.checkTransmission(x); reason != "" {
			return BlockAction(reason)
		}
		return AllowAction()

	case "wait":
		return AllowAction()

	case "swear_bond":
		b, ok := env.peer(x.TargetID)
		if !ok || b.ID == a.ID || b.Dead || b.Jailed {
			return BlockAction("target is not nearby")
		}
		if isPatronBond(x.Text) {
			switch b.PatronageToSelf {
			case "patron":
				return BlockAction("patron bond already exists")
			case "protege":
				return BlockAction("reverse patronage already exists")
			}
		} else {
			switch b.KinshipToSelf {
			case "sworn":
				return BlockAction("sworn bond already exists")
			case "spouse", "family":
				return BlockAction("existing kinship supersedes sworn bond")
			}
		}
		if a.Money < bondGift(x) {
			return BlockAction("insufficient coins for bond ceremony gift")
		}
		return AllowAction()

	case "begin_rest":
		if a.Resting {
			return BlockAction("already resting")
		}
		return AllowAction()

	case "end_rest":
		if !a.Resting {
			return BlockAction("not resting")
		}
		return AllowAction()

	case "sleep":
		if a.Energy >= MaxEnergy && a.Health >= MaxHealth {
			return BlockAction("already fully rested and healthy")
		}
		return AllowAction()

	case "eat":
		if a.Hunger <= 0 {
			return BlockAction("not hungry")
		}
		itemID := x.Item
		if itemID == "" {
			itemID = bestHeldFood(env.catalog, a)
		}
		spec, ok := env.catalog.Items[itemID]
		if !ok || !spec.Edible {
			return BlockAction("item is not edible: " + itemID)
		}
		if a.Items[itemID] < 1 {
			return BlockAction("no " + itemID + " to eat")
		}
		if form, exists := env.catalog.ActorFormSpec(a); exists && !form.MeetsMinFoodTier(spec) {
			return BlockAction("actor form does not accept food: " + itemID)
		}
		return AllowAction()

	case "drink":
		if a.Thirst <= 0 {
			return BlockAction("not thirsty")
		}
		if x.Item == "" {
			waterCheck := World{Catalog: env.catalog}
			if !env.isFreeWaterAvailable && waterCheck.firstCarriedPotableItem(a) == "" {
				return BlockAction("no potable water available")
			}
			return AllowAction()
		}
		spec, ok := env.catalog.Items[x.Item]
		if !ok || !spec.Potable {
			return BlockAction("item is not potable: " + x.Item)
		}
		if a.Items[x.Item] < 1 {
			return BlockAction("no " + x.Item + " to drink")
		}
		return AllowAction()

	case "relieve":
		if a.Waste <= 0 {
			return BlockAction("no sanitation need")
		}
		return AllowAction()

	case "medicate":
		if a.Health >= MaxHealth && a.DiseaseID == "" {
			return BlockAction("already healthy")
		}
		spec, ok := env.catalog.Items[x.Item]
		if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) {
			return BlockAction("item is not medicinal: " + x.Item)
		}
		if a.Items[x.Item] < 1 {
			return BlockAction("no " + x.Item + " to medicate")
		}
		return AllowAction()

	case "heal":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead {
			return BlockAction("target is not nearby")
		}
		if b.Health >= MaxHealth && b.DiseaseID == "" {
			return BlockAction("target is already healthy")
		}
		spec, ok := env.catalog.Items[x.Item]
		if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) {
			return BlockAction("item is not medicinal: " + x.Item)
		}
		if a.Items[x.Item] < 1 {
			return BlockAction("no " + x.Item + " to heal")
		}
		return AllowAction()

	case "join_faction":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to join faction")
		}
		f, ok := env.faction(x.FactionID)
		if !ok {
			return BlockAction("unknown faction: " + x.FactionID)
		}
		if f.District != "" && f.District != a.Location {
			return BlockAction("must be in " + f.District + " to join " + f.ID)
		}
		if f.Joined {
			return BlockAction("already a member of " + f.ID)
		}
		if f.JoinBlockReason != "" {
			return BlockAction(f.JoinBlockReason)
		}
		return AllowAction()

	case "join_profession":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to learn profession")
		}
		prof, ok := env.catalog.ProfessionByID(x.ProfessionID)
		if !ok {
			return BlockAction("unknown profession: " + x.ProfessionID)
		}
		if prof.Guild != "" {
			f, hasFac := env.faction(prof.Guild)
			if !hasFac || !f.Joined {
				return BlockAction("must join guild " + prof.Guild + " first")
			}
			if f.District != "" && f.District != a.Location {
				return BlockAction("must be in " + f.District + " to train " + prof.ID)
			}
		}
		if ok, reason := env.catalog.CanLearnProfession(a, prof); !ok {
			return BlockAction(reason)
		}
		return AllowAction()

	case "enter_house":
		if a.InsideHouse != "" {
			return BlockAction("already inside a house")
		}
		h, ok := env.accessibleHouse(x.HouseID)
		if !ok {
			return BlockAction("house is not accessible: " + x.HouseID)
		}
		if h.District != a.Location || (h.VenueID != "" && a.VenueID != "" && h.VenueID != a.VenueID) {
			return BlockAction("house is not at current location")
		}
		return AllowAction()

	case "leave_house":
		if a.InsideHouse == "" {
			return BlockAction("not inside a house")
		}
		return AllowAction()

	case "move":
		if a.InsideHouse != "" {
			return BlockAction("leave house before traveling")
		}
		cost, ok := env.roadCost(x.Location)
		if !ok {
			return ActionCheckResult{BlockReason: "no passable road to " + x.Location, routeUnavailable: true}
		}
		if a.Energy < cost {
			return BlockAction("insufficient energy to travel")
		}
		return AllowAction()

	case "move_venue":
		if a.InsideHouse != "" {
			return BlockAction("leave house before moving venue")
		}
		cost, ok := env.venueRouteCost(x.Location, x.Channel)
		if !ok {
			return ActionCheckResult{BlockReason: "no passable venue route to " + x.Location, routeUnavailable: true}
		}
		if a.Energy < cost {
			return BlockAction("insufficient energy to move venue")
		}
		return AllowAction()

	case "craft":
		r, ok := env.catalog.RecipeByID(x.RecipeID)
		if !ok {
			return BlockAction("unknown recipe: " + x.RecipeID)
		}
		if reason := env.recipeBlockReason(r); reason != "" {
			return BlockAction(reason)
		}
		return AllowAction()

	case "repair_house":
		h, ok := env.accessibleHouse(x.HouseID)
		if !ok || h.District != a.Location || (a.InsideHouse != "" && a.InsideHouse != h.ID) {
			return BlockAction("house is not accessible for repair")
		}
		if h.Condition >= MaxCondition {
			return BlockAction("house does not need repair")
		}
		repairItem := envProfile.RepairItem
		if repairItem != "" && a.Items[repairItem] < 1 {
			return BlockAction("missing repair item: " + repairItem)
		}
		return AllowAction()

	case "upgrade_house":
		if env.currentHouse == nil || env.currentHouse.ID != x.HouseID || env.currentHouse.OwnerID != a.ID {
			return BlockAction("must be inside own house to upgrade")
		}
		h := env.currentHouse
		if h.Level >= 3 {
			return BlockAction("house is already at max level")
		}
		if envProfile.BuilderProfession != "" && a.Professions[envProfile.BuilderProfession] < 2 {
			return BlockAction("requires builder profession level 2")
		}
		if a.Items[envProfile.RepairItem] < h.Level {
			return BlockAction("insufficient repair materials to upgrade house")
		}
		if a.Money < 4*h.Level {
			return BlockAction("insufficient coins to upgrade house")
		}
		return AllowAction()

	case "transfer_house":
		h, ok := env.accessibleHouse(x.HouseID)
		if !ok || h.OwnerID != a.ID {
			return BlockAction("actor does not own house: " + x.HouseID)
		}
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("recipient is not nearby")
		}
		return AllowAction()

	case "maintain_infra":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to maintain infrastructure")
		}
		if env.districtState == nil {
			return BlockAction("no district state available")
		}
		spec, exists := env.catalog.Infrastructures[x.Item]
		if !exists {
			return BlockAction("unknown infrastructure: " + x.Item)
		}
		cond := env.districtState.Infrastructure[x.Item]
		if cond >= spec.MaxCondition {
			return BlockAction("infrastructure is already at full condition")
		}
		if a.Energy < spec.BuildEnergy {
			return BlockAction("insufficient energy to maintain infrastructure")
		}
		for itemID, qty := range spec.BuildInputs {
			if a.Items[itemID] < qty {
				return BlockAction("missing infrastructure material: " + itemID)
			}
		}
		return AllowAction()

	case "hunt_threat":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to hunt threat")
		}
		if a.Health < 50 || env.tick < a.RestUntil {
			return BlockAction("not fit to hunt threat")
		}
		if env.districtState == nil || env.districtState.ThreatActivity[x.TargetID] <= 5 {
			return BlockAction("no active threat to hunt")
		}
		return AllowAction()

	case "perform_ritual":
		if !envProfile.Institutions.RitualsEnabled() {
			return BlockAction("rituals are disabled in this world environment")
		}
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to perform ritual")
		}
		var found *RitualSpec
		for i := range env.catalog.Rituals {
			if env.catalog.Rituals[i].ID == x.RecipeID {
				found = &env.catalog.Rituals[i]
				break
			}
		}
		if found == nil {
			return BlockAction("unknown ritual: " + x.RecipeID)
		}
		if found.NodeFeature != "" && !env.worldMap.DistrictHasFeature(a.Location, found.NodeFeature) {
			return BlockAction("district lacks ritual feature: " + found.NodeFeature)
		}
		if a.Energy < found.EnergyCost || a.Money < found.MoneyCost {
			return BlockAction("insufficient energy or coins for ritual")
		}
		for itemID, qty := range found.Inputs {
			if a.Items[itemID] < qty {
				return BlockAction("missing ritual input: " + itemID)
			}
		}
		return AllowAction()

	case "claim_world_event":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to claim world event")
		}
		we, ok := env.activeWorldEvent(x.TargetID)
		if !ok {
			return BlockAction("world event is not known or active")
		}
		if we.Category != "artifact_emergence" || we.ClaimedBy != "" || we.DistrictID != a.Location {
			return BlockAction("world event cannot be claimed here")
		}
		if we.RequiredPowerTier > 0 && env.catalog.ActorPowerTier(a) < we.RequiredPowerTier {
			return BlockAction("insufficient power tier to claim world event")
		}
		return AllowAction()

	case "offer":
		if x.OfferID != 0 {
			return AllowAction()
		}
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("trade target is not nearby")
		}
		spec, ok := env.catalog.Items[x.Item]
		if !ok || !spec.Tradeable {
			return BlockAction("item is not tradeable: " + x.Item)
		}
		if env.isEmbargoed(x.Item) {
			return BlockAction("item is embargoed: " + x.Item)
		}
		cur := x.Currency
		if cur == "" {
			cur = "coins"
		}
		if !ItemAcceptsCurrency(spec, cur) {
			return BlockAction("item does not accept currency: " + cur)
		}
		if spec.BuyerRequiredTag != "" && !ActorHasTag(b.Tags, spec.BuyerRequiredTag) {
			return BlockAction("buyer lacks required tag: " + spec.BuyerRequiredTag)
		}
		if x.Quantity <= 0 || x.Price < 0 || a.Items[x.Item] < x.Quantity {
			return BlockAction("invalid quantity, price, or insufficient stock")
		}
		return AllowAction()

	case "accept", "reject":
		o, ok := env.offer(x.OfferID)
		if !ok {
			return BlockAction("offer not found or expired")
		}
		if o.ResponderID() != actorID {
			return BlockAction("actor is not offer responder")
		}
		if x.Kind == "accept" {
			other := o.Counterparty(actorID)
			b, ok := env.peer(other)
			if !ok || b.Dead || b.Jailed {
				return BlockAction("offer counterparty is not nearby")
			}
			if reason := env.offerBlockReason(o); reason != "" {
				return BlockAction(reason)
			}
		}
		return AllowAction()

	case "attack":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed || b.Health <= 0 {
			return BlockAction("attack target is not nearby or valid")
		}
		if a.Health < 25 {
			return BlockAction("insufficient health to attack")
		}
		return AllowAction()

	case "spar":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed || b.Sleeping {
			return BlockAction("spar partner is unavailable")
		}
		if a.Health < 65 || b.Health < 65 || env.tick < a.RestUntil {
			return BlockAction("not fit to spar")
		}
		return AllowAction()

	case "steal":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("steal target is not nearby")
		}
		if a.Health < 45 || env.tick < a.RestUntil {
			return BlockAction("not fit to steal")
		}
		return AllowAction()

	case "raid":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("raid target is not nearby")
		}
		if a.Health < 55 || env.tick < a.RestUntil {
			return BlockAction("not fit to raid")
		}
		return AllowAction()

	case "extort":
		if !envProfile.Institutions.RacketeeringEnabled() {
			return BlockAction("racketeering is disabled in this world environment")
		}
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("extortion target is not nearby")
		}
		if a.Health < 50 || env.tick < a.RestUntil {
			return BlockAction("not fit to extort")
		}
		return AllowAction()

	case "bribe":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed || b.Sleeping {
			return BlockAction("recipient cannot accept the bribe")
		}
		if a.Money < 4 {
			return BlockAction("insufficient coins to offer bribe")
		}
		isEnforcer := b.CanArrest || env.catalog.Capabilities(Actor{Tags: b.Tags})["arrest"]
		if !isEnforcer && !env.canBribeSecretWitness(b.ID, x.Item) {
			return BlockAction("recipient is neither an official nor a witness to the secret")
		}
		return AllowAction()

	case "avenge_feud":
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed || b.Health <= 0 {
			return BlockAction("feud target is not nearby")
		}
		if a.Health < 25 {
			return BlockAction("insufficient health to avenge feud")
		}
		return AllowAction()

	case "claim_turf":
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to claim district turf")
		}
		factionID := x.FactionID
		if factionID == "" && len(a.Guilds) > 0 {
			factionID = a.Guilds[0]
		}
		if factionID == "" || !ActorHasTag(a.Guilds, factionID) {
			return BlockAction("actor does not belong to faction: " + factionID)
		}
		if env.districtController == factionID {
			return BlockAction("faction already controls district " + a.Location)
		}
		return AllowAction()

	case "frame_suspect":
		if !envProfile.Institutions.EnforcementEnabled() {
			return BlockAction("enforcement framing is disabled in this world environment")
		}
		if !env.canEnforceLaw {
			return BlockAction("actor lacks enforcement authority")
		}
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("target suspect is not nearby")
		}
		if env.catalog.IsOutlawTags(b.Tags) {
			return BlockAction("cannot scapegoat-frame an outlaw")
		}
		if env.localCorruption < env.corruptionFramingMin && !env.bribeShielded && a.IllicitGain <= 0 {
			return BlockAction("local corruption is too low for scapegoat framing")
		}
		return AllowAction()

	case "file_appeal":
		if !envProfile.Institutions.OversightEnabled() {
			return BlockAction("oversight appeals are disabled in this world environment")
		}
		if !env.canAppeal {
			return BlockAction("actor lacks oversight or public-trust standing to file appeal")
		}
		if a.IllicitGain > 0 {
			return BlockAction("corrupt official cannot adjudicate appeal")
		}
		if !env.hasOpenWrongfulLead {
			return BlockAction("no known wrongful-conviction lead")
		}
		return AllowAction()

	case "practice_skill":
		sk, ok := env.catalog.Skills[x.SkillID]
		if !ok {
			return BlockAction("unknown skill: " + x.SkillID)
		}
		if a.Skills[x.SkillID] < 1 {
			return BlockAction("actor has not learned skill: " + x.SkillID)
		}
		if a.Skills[x.SkillID] >= sk.EffectiveMaxLevel() {
			return BlockAction("skill already at max level: " + x.SkillID)
		}
		return AllowAction()

	case "attempt_transition":
		trans, ok := env.catalog.Transitions[resolveTransitionID(x)]
		if !ok {
			return BlockAction("unknown transition: " + x.TransitionID)
		}
		if ok, reason := CanAttemptTransition(a, trans); !ok {
			return BlockAction(reason)
		}
		return AllowAction()

	case "equip":
		if ok, reason := CanEquipItem(env.catalog, a, x.Item); !ok {
			return BlockAction(reason)
		}
		return AllowAction()

	case "research_tech":
		tech, ok := env.catalog.Techs[x.TechID]
		if !ok {
			return BlockAction("unknown tech: " + x.TechID)
		}
		if ok, reason := CanResearchTechWithKnown(env.catalog, a, tech, env.knownTechs); !ok {
			return BlockAction(reason)
		}
		return AllowAction()

	case "adopt_facility":
		if a.Energy < env.catalog.Techs[x.TechID].AdoptEnergyCost {
			return BlockAction("insufficient energy to adopt facility " + x.TechID)
		}
		return AllowAction()

	case "form_party":
		if partyPurpose(x.Item) == "" {
			return BlockAction("unknown party purpose: " + x.Item)
		}
		if a.InsideHouse != "" {
			return BlockAction("must be outdoors to form party")
		}
		if env.actorPartyID != "" {
			return BlockAction("already in a party")
		}
		return AllowAction()

	case "join_party":
		if env.actorPartyID != "" {
			return BlockAction("already in a party")
		}
		if _, ok := env.partyByID(x.TargetID); !ok {
			return BlockAction("party not found: " + x.TargetID)
		}
		return AllowAction()

	case "leave_party":
		if env.actorPartyID == "" {
			return BlockAction("not in a party")
		}
		return AllowAction()

	case "betray_party":
		if env.actorPartyID == "" {
			return BlockAction("not in a party")
		}
		b, ok := env.peer(x.TargetID)
		if !ok || b.Dead || b.Jailed {
			return BlockAction("party betrayal target is not nearby")
		}
		return AllowAction()

	case "propose_alliance":
		if !env.leadsFaction(x.FactionID) {
			return BlockAction("actor does not lead faction: " + x.FactionID)
		}
		if rel, hasRel := env.diplomacyBetween(x.FactionID, x.TargetID); hasRel {
			if rel.Stance == "at_war" {
				return BlockAction("cannot propose alliance while at war")
			}
			if rel.Stance == "allied" && (rel.TreatyUntilTick == 0 || env.tick < rel.TreatyUntilTick) {
				return BlockAction("already allied")
			}
			if rel.AllianceProposer == x.FactionID {
				return BlockAction("alliance proposal already pending")
			}
			minScore := -40
			if env.hasKnownInvasion {
				minScore = -80
			}
			if rel.RelationScore < minScore {
				return BlockAction("faction relation too hostile for alliance")
			}
		}
		return AllowAction()

	case "declare_war":
		if !env.leadsFaction(x.FactionID) {
			return BlockAction("actor does not lead faction: " + x.FactionID)
		}
		if rel, hasRel := env.diplomacyBetween(x.FactionID, x.TargetID); hasRel {
			if rel.Stance == "at_war" {
				return BlockAction("already at war")
			}
			if env.tick < rel.TreatyUntilTick {
				return BlockAction("treaty truce still active")
			}
		}
		return AllowAction()

	case "make_peace":
		if !env.leadsFaction(x.FactionID) {
			return BlockAction("actor does not lead faction: " + x.FactionID)
		}
		rel, hasRel := env.diplomacyBetween(x.FactionID, x.TargetID)
		if !hasRel || rel.Stance != "at_war" {
			return BlockAction("factions are not at war")
		}
		if rel.PeaceProposer == x.FactionID {
			return BlockAction("peace proposal already pending")
		}
		return AllowAction()

	default:
		if spec, ok := env.catalog.ActionSpecs[x.Kind]; ok {
			if reason := env.actionSpecBlockReason(x, spec); reason != "" {
				return BlockAction(reason)
			}
			return AllowAction()
		}
		if !env.catalog.IsKnownActionKind(x.Kind) {
			return BlockAction("unknown action: " + x.Kind)
		}
		return AllowAction()
	}
}

func isPatronBond(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "patron", "mentor", "adopt", "disciple":
		return true
	default:
		return false
	}
}

func bondGift(x Action) int {
	if x.Price > 0 {
		return x.Price
	}
	return max(0, x.Quantity)
}

func actorHasAnyTag(tags []string, candidates []string) bool {
	for _, c := range candidates {
		if ActorHasTag(tags, c) {
			return true
		}
	}
	return false
}

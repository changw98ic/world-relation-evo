package sim

import (
	"fmt"
	"strconv"
)

func (w *World) growthCareActions(a Actor) []Action {
	var actions []Action
	for _, g := range w.Growth {
		if w.canHarvestGrowth(g, a.ID, false) {
			actions = append(actions, Action{Kind: "harvest", TargetID: strconv.FormatInt(g.ID, 10)})
		}
		signed := w.careAgreement(g)
		if g.Cohort && g.Ready && g.Harvested && g.Quantity > 0 && signed != nil && signed.Harvest && signed.CaretakerID == a.ID && signed.CaretakerKind == "actor" && w.growthOwnerPresent(w.growthCaretaker(g)) {
			actions = append(actions, Action{Kind: "dispatch_harvest", TargetID: strconv.FormatInt(g.ID, 10)})
		}
		if !w.canOfferGrowthCare(g, a.ID) {
			continue
		}
		for _, other := range w.Actors {
			if !w.canInviteCaretaker(g, a, other) {
				continue
			}
			pending := false
			for _, proposal := range w.Production.CareProposals {
				if proposal.GrowthID == g.ID && proposal.CaretakerID == other.ID {
					pending = true
				}
			}
			if pending {
				continue
			}
			terms := GrowthCareAgreement{GrowthID: g.ID, OwnerKind: "actor", OwnerID: a.ID, CaretakerKind: "actor", CaretakerID: other.ID, SupplierKind: "actor", SupplierID: other.ID, Harvest: true}
			// The parties can volunteer, split the harvest equally, or assign the
			// whole crop as compensation. Explicit proposals may quote any permille.
			for _, share := range []int64{0, 500, 1000} {
				offer := terms
				offer.SharePermille = share
				actions = append(actions, Action{Kind: "propose_growth_care", TargetID: other.ID, Care: &offer})
			}
		}
	}
	for _, proposal := range w.Production.CareProposals {
		if proposal.CaretakerKind != "actor" || proposal.CaretakerID != a.ID {
			continue
		}
		for _, g := range w.Growth {
			if g.ID == proposal.GrowthID && proposal.OwnerKind == holderKind(g.Cohort) && proposal.OwnerID == g.OwnerID && !g.Harvested && w.careAgreement(g) == nil && w.growthOwnerPresent(GrowthBatch{OwnerID: a.ID, District: g.District, VenueID: g.VenueID}) {
				terms := proposal
				actions = append(actions, Action{Kind: "accept_growth_care", TargetID: proposal.OwnerID, Care: &terms})
			}
		}
	}
	return actions
}

func (w *World) canOfferGrowthCare(g GrowthBatch, ownerID string) bool {
	return !g.Cohort && g.OwnerID == ownerID && !g.Harvested && w.careAgreement(g) == nil && w.growthOwnerPresent(g)
}

func (w *World) canInviteCaretaker(g GrowthBatch, owner, other Actor) bool {
	return other.ID != owner.ID && growthCareCanTalk(owner, other, w.Tick) && w.growthOwnerPresent(GrowthBatch{OwnerID: other.ID, District: g.District, VenueID: g.VenueID})
}

func growthCareCanTalk(a, b Actor, tick int64) bool {
	return !a.Dead && !b.Dead && tick >= a.JailedUntil && tick >= b.JailedUntil && SamePlace(a, b) && (a.Task == nil || a.Task.Action.Kind != "sleep" && !a.Task.Dormant) && (b.Task == nil || b.Task.Action.Kind != "sleep" && !b.Task.Dormant)
}

func (w *World) applyGrowthCareAction(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if x.Kind == "dispatch_harvest" {
		id, err := strconv.ParseInt(x.TargetID, 10, 64)
		if err != nil {
			return fail("invalid harvest id")
		}
		for _, g := range w.Growth {
			if g.ID != id {
				continue
			}
			agreement := w.careAgreement(g)
			if agreement == nil || !agreement.Harvest || agreement.CaretakerKind != "actor" || agreement.CaretakerID != a.ID || !w.growthOwnerPresent(w.growthCaretaker(g)) {
				return fail("dispatch requires the authorized local caretaker")
			}
			events, err := w.DispatchHarvest(id)
			if err != nil {
				return fail(err.Error())
			}
			return events
		}
		return fail("unknown harvest")
	}
	if x.Care == nil {
		return fail("care terms are required")
	}
	terms := *x.Care
	if x.Kind == "propose_growth_care" {
		if err := terms.validateTerms(); err != nil {
			return fail(err.Error())
		}
		if terms.OwnerKind != "actor" || terms.OwnerID != a.ID || terms.CaretakerKind != "actor" || terms.CaretakerID == a.ID || terms.CaretakerID != x.TargetID {
			return fail("care proposal must identify its owner and the intended caretaker")
		}
		other := w.actor(terms.CaretakerID)
		if other == nil {
			return fail("unknown caretaker")
		}
		for _, g := range w.Growth {
			if g.ID != terms.GrowthID {
				continue
			}
			if !w.canOfferGrowthCare(g, a.ID) {
				return fail("care requires an available, uncontracted batch at the owner's location")
			}
			if !w.canInviteCaretaker(g, *a, *other) {
				return fail("owner and available caretaker must meet at the production site")
			}
			terms.OwnerAccepted, terms.CaretakerAccepted = true, false
			for i, proposal := range w.Production.CareProposals {
				if proposal.GrowthID == terms.GrowthID && proposal.CaretakerID == terms.CaretakerID {
					w.Production.CareProposals[i] = terms
					return []Event{w.record(a, x, "growth_care_proposed", map[string]any{"text": "proposed care terms; caretaker assent is pending", "care": terms}, other)}
				}
			}
			w.Production.CareProposals = append(w.Production.CareProposals, terms)
			return []Event{w.record(a, x, "growth_care_proposed", map[string]any{"text": "proposed care terms; caretaker assent is pending", "care": terms}, other)}
		}
		return fail("unknown growth batch")
	}
	for _, proposal := range w.Production.CareProposals {
		if proposal.GrowthID != terms.GrowthID || proposal.CaretakerKind != "actor" || proposal.CaretakerID != a.ID || x.TargetID != proposal.OwnerID {
			continue
		}
		// Assent applies to the exact stored proposal, never to caller-forged flags.
		terms.OwnerAccepted, terms.CaretakerAccepted = proposal.OwnerAccepted, proposal.CaretakerAccepted
		if terms != proposal {
			return fail("care terms differ from the pending proposal")
		}
		proposal.CaretakerAccepted = true
		if err := w.SetGrowthCare(proposal); err != nil {
			return fail(err.Error())
		}
		return []Event{w.record(a, x, "growth_care_accepted", map[string]any{"text": fmt.Sprintf("accepted care with %d/1000 harvest share", proposal.SharePermille), "care": proposal})}
	}
	return fail("no matching care proposal for this caretaker")
}

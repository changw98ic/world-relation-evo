package sim

import (
	"fmt"
	"strconv"
)

func (w *World) canOfferGrowthCare(g GrowthBatch, ownerID string) bool {
	return !g.Cohort && g.OwnerID == ownerID && !g.Harvested && w.careAgreement(g) == nil && w.growthOwnerPresent(g)
}

func (w *World) canInviteCaretaker(g GrowthBatch, owner, other Actor) bool {
	return other.ID != owner.ID && growthCareCanTalk(owner, other, w.Tick) && w.growthOwnerPresent(GrowthBatch{OwnerID: other.ID, District: g.District, VenueID: g.VenueID})
}

func growthCareCanTalk(a, b Actor, tick int64) bool {
	return !a.Dead && !b.Dead && tick >= a.JailedUntil && tick >= b.JailedUntil && SamePlace(a, b) && (a.Task == nil || a.Task.Action.Kind != "sleep" && !a.Task.Dormant) && (b.Task == nil || b.Task.Action.Kind != "sleep" && !b.Task.Dormant)
}

func (w *World) applyGrowthCareAction(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
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
			events, err := w.dispatchHarvestWithTx(tx, id)
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
					return []Event{tx.record(a, x, "growth_care_proposed", map[string]any{"text": "proposed care terms; caretaker assent is pending", "care": terms}, other)}
				}
			}
			w.Production.CareProposals = append(w.Production.CareProposals, terms)
			return []Event{tx.record(a, x, "growth_care_proposed", map[string]any{"text": "proposed care terms; caretaker assent is pending", "care": terms}, other)}
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
		return []Event{tx.record(a, x, "growth_care_accepted", map[string]any{"text": fmt.Sprintf("accepted care with %d/1000 harvest share", proposal.SharePermille), "care": proposal})}
	}
	return fail("no matching care proposal for this caretaker")
}

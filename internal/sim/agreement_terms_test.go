package sim

import (
	"bytes"
	"context"
	"testing"
)

func TestDeliveryRecordsAndSettlesChosenTerms(t *testing.T) {
	for _, price := range []int{7, 0} {
		w := lawWorld()
		item := w.Catalog.Items["wood"]
		item.BatchQty, item.BatchPrice = 2, 2
		w.Catalog.Items["wood"] = item
		w.Actors[0].Items["wood"], w.Actors[1].Money = 3, price
		checkpoint := lawJSON(t, w)
		deadline := int64(2)
		proposal := Action{Kind: "propose_agreement", TargetID: "b", Item: "wood", Quantity: 3, Price: price, DueTick: &deadline}
		decider := lawDecider(func(_ context.Context, p Perception) (Action, error) {
			if p.Tick == 0 && p.Self.ID == "a" {
				return proposal, nil
			}
			if p.Tick == 1 && p.Self.ID == "b" {
				return Action{Kind: "accept_agreement", TargetID: "a", AgreementID: 1}, nil
			}
			if p.Tick == 2 && p.Self.ID == "a" {
				return Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: 1}, nil
			}
			return Action{Kind: "wait"}, nil
		})
		var logs []TickDecisionLog
		var events []Event
		var seen int
		for w.Tick <= 2 {
			evs, log, err := w.StepRecorded(context.Background(), decider)
			if err != nil {
				t.Fatal(err)
			}
			logs = append(logs, log)
			events = append(events, evs...)
			for _, e := range evs {
				switch e.Kind {
				case "agreement_proposed", "agreement_accepted", "agreement_fulfilled":
					seen++
					x := e.Action
					if x.Item != "wood" || x.Quantity != 3 || x.Price != price || x.AgreementID != 1 || x.DueTick == nil || *x.DueTick != 2 {
						t.Fatalf("%s recorded different terms: %+v", e.Kind, x)
					}
				}
			}
		}
		g := w.Continuity.Agreements[0]
		if seen != 3 || g.Status != "fulfilled" || g.Item != "wood" || g.Quantity != 3 || g.Price != price || g.ResponseBy != 2 || g.DueTick != 2 || w.Actors[0].Items["wood"] != 0 || w.Actors[1].Items["wood"] != 3 || w.Actors[0].Money != price || w.Actors[1].Money != 0 {
			t.Fatalf("wrong delivery: price=%d, contract=%+v", price, g)
		}
		deadline = 99 // Mutating the caller's request cannot change recorded terms.
		replayed, replayEvents, err := ReplayFromCheckpoint(checkpoint, logs, 3)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(lawJSON(t, w), lawJSON(t, replayed)) || !bytes.Equal(lawJSON(t, events), lawJSON(t, replayEvents)) {
			t.Fatal("replayed contract, assets or events differ")
		}
	}
}

func TestDeliveryReferencesCannotChangeTerms(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"] = 3
	w.Actors[1].Money = 7
	now := int64(0)
	lawOK(t, w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "wood", Quantity: 3, Price: 7, DueTick: &now}))
	lawRejected(t, w.Apply("b", Action{Kind: "accept_agreement", AgreementID: 1, TargetID: "a", Item: "wood", Quantity: 3, Price: 0}))
	lawOK(t, w.Apply("b", Action{Kind: "accept_agreement", AgreementID: 1, TargetID: "a"}))
	lawRejected(t, w.Apply("a", Action{Kind: "fulfill_agreement", AgreementID: 1, TargetID: "b", Item: "wood", Quantity: 2, Price: 7}))
	lawOK(t, w.Apply("a", Action{Kind: "fulfill_agreement", AgreementID: 1, TargetID: "b"}))
	if w.Actors[0].Money != 7 || w.Actors[1].Items["wood"] != 3 {
		t.Fatal("immediate deadline or reference-only fulfillment invalid")
	}
}

func TestDeliveryProposalRejectsUnsettleableTermsBeforeOpeningContract(t *testing.T) {
	past := int64(5)
	for _, x := range []Action{
		{Item: "wood", Quantity: 0, Price: 7},
		{Item: "wood", Quantity: -1, Price: 7},
		{Item: "wood", Quantity: 3, Price: -1},
		{Item: "wood", Quantity: 3, Price: 7, Currency: "scrip"},
		{Item: "wood", Quantity: 3, Price: 7, DueTick: &past},
		{Item: "coinless", Quantity: 1, Price: 7},
		{Item: "wood", Quantity: 4, Price: 7}, // The seller owns only three and cannot produce wood.
	} {
		w := lawWorld()
		w.Tick = 6
		w.Catalog.Items["coinless"] = ItemSpec{ID: "coinless", Tradeable: true, AcceptedCurrencies: []string{"scrip"}}
		w.Actors[0].Items["wood"], w.Actors[0].Items["coinless"] = 3, 1
		x.Kind, x.TargetID = "propose_agreement", "b"
		lawRejected(t, w.Apply("a", x))
		if w.Continuity != nil && len(w.Continuity.Agreements) != 0 {
			t.Fatalf("invalid terms opened a contract: %+v", x)
		}
	}
}

func TestDeliveryCandidateAndSettlementShareRecipientAvailability(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"], w.Actors[1].Money = 3, 7
	lawOK(t, w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "wood", Quantity: 3, Price: 7}))
	lawOK(t, w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: 1}))
	w.Actors[1].Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 1}
	fulfill := Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: 1}
	if len(w.View("a").Continuity.FulfillableAgreementIDs) != 0 {
		t.Fatal("sleeping recipient was offered delivery")
	}
	lawRejected(t, w.Apply("a", fulfill))
	w.Actors[1].Task = nil
	wood := w.Catalog.Items["wood"]
	wood.BuyerRequiredTag = "licensed"
	w.Catalog.Items["wood"] = wood
	if len(w.View("a").Continuity.FulfillableAgreementIDs) != 0 {
		t.Fatal("unlicensed recipient was offered delivery")
	}
	lawRejected(t, w.Apply("a", fulfill))
	w.Actors[1].Tags = append(w.Actors[1].Tags, "licensed")
	if len(w.View("a").Continuity.FulfillableAgreementIDs) != 1 {
		t.Fatal("available lawful recipient missing despite exact payment and stock")
	}
	lawOK(t, w.Apply("a", fulfill))
}

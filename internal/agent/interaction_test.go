package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCounterofferCrossesTicksAndReplayWithoutTransferringBeforeAssent(t *testing.T) {
	w := parcelWorld()
	w.Actors = w.Actors[:2]
	checkpoint := []byte(boundaryJSON(t, w))
	decider := boundaryDecider(func(_ context.Context, p sim.Perception) (sim.Action, error) {
		options := Options(p)
		if p.Tick == 1 && p.Self.ID == "alice" {
			for _, o := range options {
				if o.Action.Kind == "offer" {
					return o.Action, nil
				}
			}
		}
		if p.Tick == 2 && p.Self.ID == "bob" {
			for _, x := range p.Interactions {
				if x.Kind == "offer" && x.AwaitingActorID == "bob" {
					return sim.Action{Kind: "offer", OfferID: x.ID, TargetID: "alice", Item: "parcel", Quantity: 2, Price: 4, Currency: "coins"}, nil
				}
			}
		}
		if p.Tick == 3 && p.Self.ID == "alice" {
			for _, o := range options {
				if o.Action.Kind == "accept" {
					return o.Action, nil
				}
			}
		}
		for _, o := range options {
			if o.Action.Kind == "wait" {
				return o.Action, nil
			}
		}
		return sim.Action{}, fmt.Errorf("fixture missing intended action")
	})
	var logs []sim.TickDecisionLog
	var events []sim.Event
	for i := 0; i < 3; i++ {
		es, log, err := w.StepRecorded(context.Background(), decider)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
		events = append(events, es...)
		if i < 2 {
			if w.Actors[0].Items["parcel"] != 3 || w.Actors[1].Items["parcel"] != 0 || w.Actors[0].Money != 50 || w.Actors[1].Money != 50 {
				t.Fatal("proposal transferred assets")
			}
			if len(w.Offers) != 1 {
				t.Fatal("missing cross-turn offer")
			}
			v := w.View("alice")
			found := false
			for _, x := range v.Interactions {
				if x.Kind == "offer" && x.ID == w.Offers[0].ID && x.AwaitingActorID == w.Offers[0].ResponderID() {
					found = true
				}
			}
			if !found {
				t.Fatal("response state not visible next tick")
			}
		}
	}
	if w.Actors[0].Items["parcel"] != 1 || w.Actors[1].Items["parcel"] != 2 || w.Actors[0].Money != 54 || w.Actors[1].Money != 46 || len(w.Offers) != 0 {
		t.Fatal("counteroffer did not settle exact accepted terms")
	}
	replayed, replayedEvents, err := sim.ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, replayed) != boundaryJSON(t, w) || boundaryJSON(t, replayedEvents) != boundaryJSON(t, events) {
		t.Fatal("counteroffer replay changed outcome")
	}
	// Resume at the pending-counteroffer checkpoint, not just at the run's start.
	mid, _, err := sim.ReplayFromCheckpoint(checkpoint, logs[:2], 3)
	if err != nil {
		t.Fatal(err)
	}
	resumed, _, err := sim.ReplayFromCheckpoint([]byte(boundaryJSON(t, mid)), logs[2:], w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, resumed) != boundaryJSON(t, w) {
		t.Fatal("pending proposal was not durable across restart")
	}
}

func TestCounterofferIdentityExpiryAndSettlementFailure(t *testing.T) {
	for _, scenario := range []string{"third_party", "stale", "expired", "stock_lost", "buyer_unfunded", "dead", "absent", "repeat"} {
		t.Run(scenario, func(t *testing.T) {
			w := parcelWorld()
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "offer", "bob"))
			old := w.Offers[0]
			counter := sim.Action{Kind: "offer", OfferID: old.ID, TargetID: "alice", Item: "parcel", Quantity: 2, Price: 4}
			if scenario == "third_party" {
				boundaryRejected(t, w, "charlie", counter)
				return
			}
			if scenario == "expired" {
				w.Tick = old.Expires + 1
				boundaryRejected(t, w, "bob", counter)
				return
			}
			boundaryApply(t, w, "bob", counter)
			next := w.Offers[0]
			if next.ID == old.ID || next.PreviousID != old.ID || next.Seller != "alice" || next.Buyer != "bob" || next.ResponderID() != "alice" || next.Expires != old.Expires {
				t.Fatal("counteroffer changed identity or lifetime")
			}
			if scenario == "stale" {
				boundaryRejected(t, w, "bob", sim.Action{Kind: "accept", OfferID: old.ID})
				boundaryRejected(t, w, "bob", counter)
				return
			}
			switch scenario {
			case "stock_lost":
				w.Actors[0].Items["parcel"] = 1
			case "buyer_unfunded":
				w.Actors[1].Money = 3
			case "dead":
				w.Actors[1].Dead = true
			case "absent":
				w.Actors[1].Location = "remote"
			}
			if scenario == "repeat" {
				boundaryApply(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
				before := boundaryJSON(t, w.Actors)
				boundaryRejected(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
				if w.Actors[0].Items["parcel"] != 1 || w.Actors[1].Items["parcel"] != 2 || w.Actors[1].Money != 46 {
					t.Fatal("repeated acceptance transferred again")
				}
				_ = before
				return
			}
			beforeSeller, beforeBuyer := w.Actors[0].Items["parcel"], w.Actors[1].Items["parcel"]
			moneySeller, moneyBuyer := w.Actors[0].Money, w.Actors[1].Money
			boundaryRejected(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
			if w.Actors[0].Items["parcel"] != beforeSeller || w.Actors[1].Items["parcel"] != beforeBuyer || w.Actors[0].Money != moneySeller || w.Actors[1].Money != moneyBuyer {
				t.Fatal("failed settlement moved assets")
			}
		})
	}
}

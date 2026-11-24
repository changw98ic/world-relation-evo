package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPlannerReconsidersSleepWhenWaitingLowersEnergy(t *testing.T) {
	w := boundaryWorld()
	w.Tick = 18 * sim.TicksPerHour // Midnight on the first night.
	w.Actors = w.Actors[:1]
	w.Actors[0].Energy = 65
	rules := sim.DefaultPlanningRules()
	rules.Needs = []sim.NeedRule{{NeedKey: sim.NeedKey{Measure: "energy"}, Target: 65, Scale: 40, BaseWeight: 2}}
	w.Catalog.Planning = &rules
	for range 3 {
		events, err := w.Step(context.Background(), Planner{})
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.ActorID == "alice" && event.Kind == "sleep_started" {
				return
			}
		}
	}
	t.Fatalf("waiting did not lead to sleep when the energy need appeared: energy=%d, review=%+v",
		w.Actors[0].Energy, w.Actors[0].NextWake)
}

func TestHungryExhaustedWorkerCompletesRestBeforeStartingAnotherForagingTrip(t *testing.T) {
	w := sim.Village("rest-before-new-food-trip")
	a := w.Actors[14]
	a.Health, a.Energy, a.Hunger, a.Money = 100, 10, 3, 0
	a.Location, a.InsideHouse = "south_fields", ""
	a.Items = map[string]int{"water": 20}
	a.Professions["gatherer"] = 1
	a.Plan = nil
	w.Actors = []sim.Actor{a}
	w.Growth = nil
	for i := range w.Houses {
		if w.Houses[i].ID == a.HomeID {
			w.Houses[i].Condition = 0 // matches the actual long-run failure state
		}
	}
	w.InitLedgerAudit()
	rested, ate, intendedRest := false, false, false
	for step := 0; step < 3*sim.TicksPerDay; step++ {
		events, err := w.Step(context.Background(), Offline())
		if err != nil {
			t.Fatal(err)
		}
		if intendedRest && w.Actors[0].InsideHouse == a.HomeID && w.Actors[0].Energy > a.Energy {
			rested = true
		}
		for _, event := range events {
			if event.ActorID == a.ID && event.Kind == "rest_planned" {
				intendedRest = true
			}
			if event.ActorID == a.ID && event.Kind == "ate" {
				ate = true
			}
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
	}
	if !rested || !ate || w.Actors[0].Dead {
		t.Fatalf("worker repeatedly abandoned the trip home as soon as one forage became affordable: rested=%v ate=%v actor=%+v", rested, ate, w.Actors[0])
	}
}

func TestHungryRestingActorAcceptsAvailableFoodBeforeContinuingSleep(t *testing.T) {
	w := boundaryWorld()
	w.Actors[1].Resting, w.Actors[1].Hunger, w.Actors[1].Energy, w.Actors[1].Money = true, 6, 80, 0
	w.Offers = []sim.Offer{{ID: 20, Seller: "alice", Buyer: "bob", Item: "food", Quantity: 2, Price: 0, Expires: 12}}
	action, err := Offline().Decide(context.Background(), w.View("bob"))
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != "accept" || action.OfferID != 20 {
		t.Fatalf("rest intention blocked a funded immediate meal: %+v", action)
	}
	boundaryApply(t, w, "bob", action)
	if w.Actors[1].Items["food"] != 2 {
		t.Fatal("accepting food did not actually deliver it")
	}
}

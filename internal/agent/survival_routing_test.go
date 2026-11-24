package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func hungryRiverWorld(t *testing.T) *sim.World {
	t.Helper()
	p, err := pack.LoadFile("../../packs/mortal_watershed.json")
	if err != nil {
		t.Fatal(err)
	}
	p.Cohorts = nil
	w, err := p.Instantiate("hungry-river")
	if err != nil {
		t.Fatal(err)
	}
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.ID == "magistrate_lin" {
			a.Items = map[string]int{"water": 4}
			a.Hunger = 2
			a.Health = 80
			a.Energy = 100
		}
	}
	w.InitLedgerAudit()
	return w
}

func TestSurvivalRoutingRespectsVenueAccess(t *testing.T) {
	w := hungryRiverWorld(t)
	p := w.View("magistrate_lin")
	if _, ok := stepTowardDistrict(p, "qingxi_village", 0); ok {
		t.Fatal("district planner bypassed unavailable river route")
	}
	x, err := Offline().Decide(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if x.Kind == "wait" || x.Kind == "move" {
		t.Fatalf("hungry actor ignored a reachable training that enables access to food: %+v", x)
	}
}

func TestHungryActorActuallyObtainsFoodAcrossVenueSteps(t *testing.T) {
	w := hungryRiverWorld(t)
	// Exercise the same production environment, while keeping unrelated actors
	// from selling food to the subject and masking the route decision.
	for i := range w.Actors {
		if w.Actors[i].ID == "magistrate_lin" {
			w.Actors = append([]sim.Actor(nil), w.Actors[i])
			break
		}
	}
	w.InitLedgerAudit()
	obtained, ate := false, false
	// One day covers the declared guild training, river travel and extraction work in this fixture.
	for i := 0; i < int(sim.TicksPerDay); i++ {
		events, err := w.Step(context.Background(), Offline())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.ActorID == "magistrate_lin" && e.Kind == "crafted" && w.Catalog.Items[e.Payload["item"].(string)].Edible {
				obtained = true
			}
			if e.ActorID == "magistrate_lin" && e.Kind == "ate" {
				ate = true
			}
			if e.ActorID == "magistrate_lin" && e.Kind == "action_failed" {
				t.Fatalf("survival action failed: %+v", e)
			}
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
		if obtained && ate {
			return
		}
	}
	t.Fatalf("did not obtain and eat actual food: obtained=%v ate=%v actor=%+v", obtained, ate, w.Actors[0])
}

func TestSurvivalRoutingKeepsAuthorizedTravel(t *testing.T) {
	w := hungryRiverWorld(t)
	for i := range w.Actors {
		if w.Actors[i].ID == "magistrate_lin" {
			w.Actors[i].Tags = append(w.Actors[i].Tags, "water_user")
		}
	}
	p := w.View("magistrate_lin")
	action, ok := stepTowardDistrict(p, "qingxi_village", 0)
	if !ok || action.Kind != "move_venue" {
		t.Fatalf("authorized river route rejected: %+v %v", action, ok)
	}
	p.Map = sim.WorldMap{Districts: []sim.District{{ID: "a"}, {ID: "b"}}, Roads: []sim.Road{{From: "a", To: "b", Ticks: 1, EnergyCost: 2}}}
	p.Roads = []sim.RoadView{{To: "b", Ticks: 1, EnergyCost: 2}}
	p.Self.Location = "a"
	p.Self.VenueID = ""
	p.Self.InsideHouse = ""
	action, ok = stepTowardDistrict(p, "b", 0)
	if !ok || action.Kind != "move" || action.Location != "b" {
		t.Fatalf("district-only route rejected: %+v %v", action, ok)
	}
}

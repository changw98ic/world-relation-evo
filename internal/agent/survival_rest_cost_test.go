package agent

import (
	"context"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"testing"
)

func TestUrgentRestUsesTheActualCostOfLocalForaging(t *testing.T) {
	for _, c := range []struct {
		name         string
		energy, cost int
		canWork      bool
	}{
		{"recorded_case", 17, 9, true}, {"exact_energy", 9, 9, true}, {"one_short", 8, 9, false}, {"higher_world_cost", 17, 18, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := sim.Village("interrupted-rest")
			a := w.Actors[18]
			a.Resting = true
			a.Location = "north_woods"
			a.InsideHouse = ""
			a.Energy = c.energy
			a.Health = 11
			a.Hunger = 9
			a.Thirst = 0
			a.Items = map[string]int{}
			a.Professions["gatherer"] = 6
			w.Actors = []sim.Actor{a}
			w.Growth = nil
			for i := range w.Catalog.Recipes {
				if w.Catalog.Recipes[i].ID == "forage_food" {
					w.Catalog.Recipes[i].EnergyCost = c.cost
				}
			}
			w.InitLedgerAudit()
			decide := func() sim.Action {
				x, e := Offline().Decide(context.Background(), w.View(a.ID))
				if e != nil {
					t.Fatal(e)
				}
				return x
			}
			x := decide()
			if !c.canWork {
				if x.Kind == "end_rest" || x.Kind == "craft" {
					t.Fatalf("cannot pay %d energy with %d, got %v", c.cost, c.energy, x)
				}
				return
			}
			if x.Kind == "end_rest" {
				boundaryApply(t, w, a.ID, x)
				x = decide()
			}
			if x.Kind != "craft" || x.RecipeID != "forage_food" {
				t.Fatalf("food available here for %d, energy %d: got %+v", c.cost, c.energy, x)
			}
			boundaryApply(t, w, a.ID, x)
			if w.Actors[0].Energy != c.energy-c.cost {
				t.Fatal("did not settle the declared cost")
			}
			for w.Actors[0].Task != nil {
				w.Advance(a.ID)
			}
			if w.Actors[0].Items["food"] != 1 {
				t.Fatal("onsite work did not produce its actual ration")
			}
			x = decide()
			if x.Kind != "eat" {
				t.Fatal("worker did not eat actual output", x)
			}
			boundaryApply(t, w, a.ID, x)
			if w.Actors[0].Hunger != 8 || w.Actors[0].Items["food"] != 0 {
				t.Fatal("meal settlement changed")
			}
			if e := w.VerifyLedgerConservation(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

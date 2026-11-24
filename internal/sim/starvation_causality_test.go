package sim

import (
	"context"
	"testing"
)

type foodWorkChoice struct{ work bool }

func (choice foodWorkChoice) Decide(_ context.Context, p Perception) (Action, error) {
	if p.Self.Thirst >= 2 {
		return Action{Kind: "drink", Item: "water"}, nil
	}
	if p.Self.Energy == 0 {
		return Action{Kind: "sleep"}, nil
	}
	if choice.work && p.Self.Hunger > 0 {
		if p.Self.Items["grain"] > 0 {
			return Action{Kind: "eat", Item: "grain"}, nil
		}
		return Action{Kind: "craft", RecipeID: "gather_grain"}, nil
	}
	return Action{Kind: "wait"}, nil
}

// An available food source is not food already harvested or eaten. The same
// finite stock supports a worker, while deliberately waiting can cause death.
func TestStarvationFollowsActualFoodChoicesRatherThanGuaranteedSurvival(t *testing.T) {
	for _, work := range []bool{false, true} {
		name := "chooses_wait"
		if work {
			name = "chooses_work_and_eat"
		}
		t.Run(name, func(t *testing.T) {
			w := lawWorld()
			w.Actors = w.Actors[:1]
			w.Actors[0].Items["water"] = 100
			w.Catalog.Resources["grain_stand"] = ResourcePoolSpec{ID: "grain_stand"}
			w.DistrictStates = map[string]DistrictState{"home": {Resources: map[string]int64{"grain_stand": 64}}}
			w.Catalog.Recipes = []Recipe{{ID: "gather_grain", RequiredTag: "person", ResourcePool: "grain_stand", ResourceDraw: 1, Ticks: 1, EnergyCost: 1, OutputItem: "grain", OutputQty: 1, Conversion: &MaterialConversion{ExternalMassUnits: 1, ExternalNutritionUnits: 1, Source: "fixture: one finite standing grain unit funds one edible grain unit"}}}
			w.InitLedgerAudit()
			for tick := 0; tick < 8*TicksPerDay; tick++ {
				if _, _, err := w.StepRecorded(context.Background(), foodWorkChoice{work: work}); err != nil {
					t.Fatal(err)
				}
				if err := w.VerifyLedgerConservation(); err != nil {
					t.Fatal(err)
				}
			}
			if work {
				if w.Actors[0].Dead || w.DeathsByCause["starvation"] != 0 || w.DistrictStates["home"].Resources["grain_stand"] >= 64 {
					t.Fatal("legitimate paid work and eating failed to use the available food source")
				}
			} else if !w.Actors[0].Dead || w.DeathsByCause["starvation"] != 1 || w.DistrictStates["home"].Resources["grain_stand"] != 64 {
				t.Fatal("waiting was given food or survival without harvesting and eating")
			}
		})
	}
}

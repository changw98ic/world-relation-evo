package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestSalesGoalDoesNotPreventRepairOfTheActorsShelter(t *testing.T) {
	w := sim.Village("upkeep-during-sales")
	w.Tick = 12
	a := w.Actors[14]
	w.Actors = []sim.Actor{a}
	w.Growth = nil
	a = w.Actors[0]
	w.Actors[0].Items = map[string]int{"food": 20, "water": 5, "plank": 1, "timber": 2}
	w.Actors[0].Energy, w.Actors[0].Health, w.Actors[0].Hunger = 100, 100, 0
	w.Actors[0].Plan = &sim.PersonalPlan{RecipeID: "saw_planks", Item: "plank", Quantity: 2, Status: "active"}
	for i := range w.Houses {
		if w.Houses[i].ID == a.HomeID {
			w.Houses[i].Condition = 30
			w.Actors[0].Location, w.Actors[0].InsideHouse = w.Houses[i].District, w.Houses[i].ID
		}
	}
	selected, err := Offline().Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	if selected.Kind != "repair_house" || selected.HouseID != a.HomeID {
		t.Fatalf("sales plan displaced available, necessary shelter upkeep: %+v", selected)
	}
	boundaryApply(t, w, a.ID, selected)
	if w.Actors[0].Items["plank"] != 0 || w.Actors[0].Plan.Status != "active" {
		t.Fatal("repair was unpaid or erased the personal sales goal")
	}
}

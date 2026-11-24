package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestHarvestCanBeSoldInLotsBeforeEntireSeasonSalesTargetIsStocked(t *testing.T) {
	w := boundaryWorld()
	w.Tick = 12
	a := &w.Actors[0]
	a.Items["food"] = 1080
	a.Professions = map[string]int{"farmer": 1}
	a.Plan = &sim.PersonalPlan{RecipeID: "farm_grain", Item: "food", Quantity: 1200, Status: "active"}
	w.Actors[1].Items["food"] = 0
	w.Actors = w.Actors[:2]
	sim.SyncActorPlan(a, w.Tick, w.Catalog)
	selected, err := Offline().Decide(context.Background(), w.View("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if selected.Kind != "offer" || selected.Item != "food" || selected.Quantity != 2 {
		t.Fatalf("a 1200-portion seasonal sales goal blocked an affordable two-portion sale: %+v", selected)
	}
	boundaryApply(t, w, "alice", selected)
	boundaryApply(t, w, "bob", sim.Action{Kind: "accept", OfferID: w.Offers[0].ID})
	if a.Plan.Sold != 2 || a.Plan.RemainingToSell() != 1198 || w.Actors[1].Items["food"] != 2 {
		t.Fatal("small sale did not deliver real grain and advance the unchanged seasonal goal")
	}
}

func TestNextCropIsPlantedWhileExistingHarvestIsBeingSold(t *testing.T) {
	w := sim.Village("crop-and-sale")
	w.Tick = 12
	w.Actors = w.Actors[:1]
	a := &w.Actors[0]
	a.Location, a.InsideHouse = "south_fields", ""
	a.Health, a.Energy, a.Hunger = 100, 100, 0
	a.Items = map[string]int{"food": 1080, "seeds": 1, "water": 1}
	a.NeedWeights = map[string]float64{"progress": 4}
	a.Plan = &sim.PersonalPlan{RecipeID: "farm_grain", Item: "food", Quantity: 1200, Status: "active"}
	w.Growth = nil
	selected, err := Offline().Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	if selected.Kind != "craft" || selected.RecipeID != "farm_grain" {
		t.Fatalf("selling the previous harvest postponed a whole new 120-day crop: %+v", selected)
	}
	boundaryApply(t, w, a.ID, selected)
	if a.Task == nil || a.Items["seeds"] != 0 || a.Plan.Quantity != 1200 {
		t.Fatal("sowing did not pay its real inputs or erased the existing sales goal")
	}
}

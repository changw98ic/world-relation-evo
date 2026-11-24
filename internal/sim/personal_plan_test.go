package sim

import "testing"

func TestPersonalPlanCouplesToHeldInventoryAndRevertsOnStockLoss(t *testing.T) {
	// 1. Pre-existing inventory >= plan.Quantity starts directly in "sell" and completes upon sale.
	w := lawWorld()
	w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "start_plan", "abandon_plan")
	w.Catalog.Items["board"] = ItemSpec{ID: "board", MassUnits: 2, Tier: 2, Tradeable: true, BatchQty: 2, BatchPrice: 6, OfferThreshold: 6}
	w.Catalog.Recipes = []Recipe{{
		ID: "board", Profession: "worker", Ticks: 1, EnergyCost: 2,
		Inputs: map[string]int{"wood": 4}, OutputItem: "board", OutputQty: 2,
		Conversion: &MaterialConversion{Source: "fixture: four unit-mass wood become two mass-two boards"},
	}}
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Items["board"] = 2
	b.Money = 20

	lawOK(t, w.Apply("a", Action{Kind: "start_plan", RecipeID: "board"}))
	if a.Plan == nil || a.Plan.Status != "active" || a.Plan.Stage != "sell" || a.Plan.Quantity != 2 {
		t.Fatalf("expected pre-stocked start_plan to enter sell stage immediately, got %+v", a.Plan)
	}
	view := w.View("a")
	if view.ActivePlan == nil || view.ActivePlan.Stage != "sell" || view.ActivePlan.DeficitToProduce != 0 || view.ActivePlan.RemainingToSell != 2 {
		t.Fatalf("unexpected ActivePlan view for pre-stocked actor: %+v", view.ActivePlan)
	}

	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "board", Quantity: 2, Price: 6}))
	lawOK(t, w.Apply("b", Action{Kind: "accept", OfferID: w.Offers[0].ID}))
	if a.Plan.Status != "completed" || a.Plan.Stage != "done" || a.Plan.Sold != 2 || a.Plan.Produced != 0 {
		t.Fatalf("expected sale of pre-existing stock to complete plan without redundant production, got %+v", a.Plan)
	}

	// 2. Partial stock advances to "sell" once total held >= remaining, and reverts to "produce" if stock is consumed before sale.
	w2 := lawWorld()
	w2.Catalog.TagActions["person"] = append(w2.Catalog.TagActions["person"], "start_plan", "abandon_plan")
	w2.Catalog.Items["ration"] = ItemSpec{ID: "ration", MassUnits: 2, NutritionUnits: 2, Tier: 1, Edible: true, Tradeable: true, BatchQty: 2, BatchPrice: 4, OfferThreshold: 6}
	w2.Catalog.Recipes = []Recipe{{
		ID: "bake_ration", Profession: "worker", Ticks: 1, EnergyCost: 2,
		Inputs: map[string]int{"grain": 2}, OutputItem: "ration", OutputQty: 1,
		Conversion: &MaterialConversion{Source: "fixture: two grain units become one two-unit ration"},
	}}
	chef := &w2.Actors[0]
	chef.Items["ration"] = 1
	chef.Items["grain"] = 3
	chef.Hunger = 2

	lawOK(t, w2.Apply("a", Action{Kind: "start_plan", RecipeID: "bake_ration"}))
	if chef.Plan.Stage != "produce" || chef.Plan.Quantity != 2 {
		t.Fatalf("expected partial inventory (1/2) to start in produce stage, got %+v", chef.Plan)
	}

	// Crafting 1 unit brings held stock to 2 (Produced == 1 < Quantity == 2), which must advance Stage to "sell".
	lawOK(t, w2.Apply("a", Action{Kind: "craft", RecipeID: "bake_ration"}))
	if chef.Items["ration"] != 2 || chef.Plan.Produced != 1 || chef.Plan.Stage != "sell" {
		t.Fatalf("expected 1 pre-held + 1 crafted ration to reach sell stage, got items=%v plan=%+v", chef.Items, chef.Plan)
	}

	// Eating 1 ration drops held stock to 1 (< 2 remaining), reverting Stage back to "produce".
	lawOK(t, w2.Apply("a", Action{Kind: "eat", Item: "ration"}))
	if chef.Items["ration"] != 1 || chef.Plan.Stage != "produce" || chef.Plan.Status != "active" {
		t.Fatalf("expected consuming planned stock below remaining target to revert stage to produce, got items=%v plan=%+v", chef.Items, chef.Plan)
	}
}

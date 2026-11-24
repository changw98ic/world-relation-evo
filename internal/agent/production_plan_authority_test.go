package agent

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPlanUsesActualEntitlementAndHarvestActions(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "craft", "harvest")
	r := sim.Recipe{ID: "make_boards", Ticks: 1, Inputs: map[string]int{"wood": 2}, OutputItem: "boards", OutputQty: 2, Conversion: &sim.MaterialConversion{Source: "two unit masses become two boards"}}
	w.Catalog.Items["wood"] = sim.ItemSpec{ID: "wood", MassUnits: 1}
	w.Catalog.Items["boards"] = sim.ItemSpec{ID: "boards", MassUnits: 1, Tradeable: true, BatchQty: 2}
	w.Catalog.Recipes = []sim.Recipe{r}
	a := &w.Actors[0]
	a.Plan = &sim.PersonalPlan{RecipeID: r.ID, Item: r.OutputItem, Quantity: 2, Status: "active", Stage: "produce"}
	a.Items["wood"] = 2
	w.Growth = []sim.GrowthBatch{
		{ID: 10, OwnerID: "alice", District: "meeting", Item: "boards", Quantity: 2, RecipeID: "dry_boards", Batches: 1},
		{ID: 11, OwnerID: "charlie", District: "meeting", Item: "boards", Quantity: 99, Batches: 1},
	}
	check := func(expected int64, deficit int, directive, action string) sim.Action {
		t.Helper()
		p := w.View(a.ID)
		d := p.ActivePlan
		if d.NextStep != directive || d.ExpectedFromGrowth != expected || d.DeficitToProduce != deficit {
			t.Fatalf("incorrect plan for %s: %+v", directive, d)
		}
		x, ok := executeActivePlan(p, nil, p.Capabilities(), a.Money)
		if action == "" && !ok {
			return sim.Action{}
		}
		if !ok || x.Kind != action {
			t.Fatalf("directive %s, selected %+v", d.NextStep, x)
		}
		for _, opt := range Options(p) {
			if opt.Action.Kind == x.Kind && opt.Action.RecipeID == x.RecipeID && opt.Action.TargetID == x.TargetID {
				return x
			}
		}
		t.Fatalf("the directive selected an unavailable action: %+v", x)
		return sim.Action{}
	}
	check(2, 0, "await_harvest", "")
	if _, ok := resolveRecipe(w.View(a.ID), r, 0); ok {
		t.Fatal("routine work started a second recipe for the already covered lot")
	}
	w.Growth[0].Stalled = true
	check(2, 0, "care_stalled_growth", "")
	w.Growth[0].Stalled = false
	if err := w.SetGrowthCare(sim.GrowthCareAgreement{GrowthID: 10, OwnerKind: "actor", OwnerID: a.ID, CaretakerKind: "actor", CaretakerID: "bob", SupplierKind: "actor", SupplierID: "bob", SharePermille: 500, Harvest: true, OwnerAccepted: true, CaretakerAccepted: true}); err != nil {
		t.Fatal(err)
	}
	if got := w.View("bob").ExpectedGrowthStock("boards"); got != 1 {
		t.Fatalf("expected one board as caretaker payment, got %d", got)
	}
	check(1, 1, "craft:make_boards", "craft")
	w.Growth[0].Ready = true
	boundaryApply(t, w, a.ID, check(1, 1, "harvest:10", "harvest"))
	if a.Items["boards"] != 1 || w.Actors[1].Items["boards"] != 1 || w.View(a.ID).ExpectedGrowthStock("boards") != 0 {
		t.Fatalf("actual harvest disagrees with expected shares: owner=%d caretaker=%d", a.Items["boards"], w.Actors[1].Items["boards"])
	}
}

func TestPlanChecksParentsAndConsumesOnlyRecipeInputs(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "craft", "harvest")
	a := &w.Actors[0]
	r := sim.Recipe{ID: "breed", Ticks: 1, Inputs: map[string]int{"fodder": 1}, OutputItem: "kid", OutputQty: 1, Conversion: &sim.MaterialConversion{Source: "fixture: feed mass converted to young"}, Growth: &sim.GrowthRule{DurationTicks: 2, MaintenanceIntervalTicks: 1, Parents: map[string]int{"goat": 2}, Source: "fixture: two parents for two ticks"}}
	for _, item := range []string{"goat", "kid", "fodder"} {
		w.Catalog.Items[item] = sim.ItemSpec{ID: item, MassUnits: 1, BatchQty: 1}
	}
	w.Catalog.Recipes = []sim.Recipe{r}
	a.Plan = &sim.PersonalPlan{RecipeID: r.ID, Item: r.OutputItem, Quantity: 1, Status: "active", Stage: "produce"}
	a.Items["goat"], a.Items["fodder"] = 1, 1
	p := w.View(a.ID)
	if d := p.ActivePlan; d.NextStep != "acquire_inputs" || len(d.MissingInputs) != 1 || d.MissingInputs[0] != "goat:1/2" {
		t.Fatalf("missing breeding stock absent from plan: %+v", d)
	}
	start := sim.Action{Kind: "craft", RecipeID: r.ID}
	for _, opt := range Options(p) {
		if opt.Action.Kind == start.Kind && opt.Action.RecipeID == r.ID {
			t.Fatal("breeding was offered without the second parent")
		}
	}
	boundaryRejected(t, w, a.ID, start)
	a.Items["goat"]++
	p = w.View(a.ID)
	if d := p.ActivePlan; d.NextStep != "craft:breed" || p.RecipeBlockReason(r) != "" {
		t.Fatalf("exact required inputs insufficient: %+v", d)
	}
	boundaryApply(t, w, a.ID, boundaryOption(t, w, a.ID, "craft", ""))
	if a.Items["goat"] != 0 || a.Items["fodder"] != 0 || len(w.Growth) != 1 || w.Growth[0].Parents["goat"] != 2 {
		t.Fatalf("accepted breeding failed to reserve parents or pay feed: %+v", a)
	}
}

func TestPlanTravelsFromDepletedForestAndUsesExactFinalResource(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Professions = map[string]int{"woodcutter": 1}
	r, ok := w.Catalog.RecipeByID("fell_timber")
	if !ok {
		t.Fatal("missing timber recipe")
	}
	a.Plan = &sim.PersonalPlan{RecipeID: r.ID, Item: r.OutputItem, Quantity: r.OutputQty, Status: "active", Stage: "produce"}
	for i := range w.Map.Districts {
		w.Map.Districts[i].Features = []string{r.NodeFeature}
		w.Map.Districts[i].ResourceCapacities = map[string]int64{r.ResourcePool: int64(r.ResourceDraw)}
	}
	w.Map.Roads = []sim.Road{{From: "meeting", To: "remote", Ticks: 1, EnergyCost: 1}}
	w.DistrictStates = map[string]sim.DistrictState{
		"meeting": {Weather: "clear", Resources: map[string]int64{r.ResourcePool: 0}},
		"remote":  {Weather: "clear", Resources: map[string]int64{r.ResourcePool: int64(r.ResourceDraw)}},
	}
	p := w.View(a.ID)
	x, _ := executeActivePlan(p, nil, p.Capabilities(), a.Money)
	if p.ActivePlan.NextStep != "travel_to_production:remote" || x.Kind != "move" || x.Location != "remote" {
		t.Fatalf("locally exhausted forest counted as a work site: plan=%+v action=%+v", p.ActivePlan, x)
	}
	a.Location = "remote"
	p = w.View(a.ID)
	if p.ActivePlan.NextStep != "craft:"+r.ID {
		t.Fatalf("last complete draw was excluded: %+v", p.ActivePlan)
	}
	for _, option := range Options(p) {
		if option.Action.Kind == "craft" && option.Action.RecipeID == r.ID {
			boundaryApply(t, w, a.ID, option.Action)
			if w.DistrictStates["remote"].Resources[r.ResourcePool] != 0 {
				t.Fatal("last draw did not exhaust stock")
			}
			return
		}
	}
	t.Fatal("plan recommended an unavailable draw")
}

func TestPlanFindsAvailableInputAcrossRecipes(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "craft")
	a := &w.Actors[0]
	recipe := func(id, input, output string) sim.Recipe {
		w.Catalog.Items[input] = sim.ItemSpec{ID: input, MassUnits: 1, BatchQty: 1}
		w.Catalog.Items[output] = sim.ItemSpec{ID: output, MassUnits: 1, BatchQty: 1}
		return sim.Recipe{ID: id, Ticks: 1, Inputs: map[string]int{input: 1}, OutputItem: output, OutputQty: 1, Conversion: &sim.MaterialConversion{Source: "fixture: preserve one unit of material"}}
	}
	w.Catalog.Recipes = []sim.Recipe{
		recipe("fire", "shaped_clay", "pot"),
		recipe("shape", "clay", "shaped_clay"),
		recipe("recycle", "pot", "clay"),
		recipe("extract", "clay_deposit", "clay"),
	}
	a.Items["clay_deposit"] = 1
	a.Plan = &sim.PersonalPlan{RecipeID: "fire", Item: "pot", Quantity: 1, Status: "active", Stage: "produce"}
	for _, next := range []string{"extract", "shape", "fire"} {
		p := w.View(a.ID)
		x, ok := executeActivePlan(p, nil, p.Capabilities(), a.Money)
		if !ok || p.ActivePlan.NextStep != "craft:"+next || x.RecipeID != next {
			t.Fatalf("plan failed to reach %s through available inputs: %+v, %+v", next, p.ActivePlan, x)
		}
		boundaryApply(t, w, a.ID, x)
		w.Tick++
	}
	if a.Items["pot"] != 1 || w.View(a.ID).ActivePlan.Stage != "sell" {
		t.Fatalf("finished pot absent from actual saleable stock: %v", a.Items)
	}
}

package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRequiredMaterialDoesNotBecomeItsOwnSalesIntention(t *testing.T) {
	w := boundaryWorld()
	w.Tick = 12
	w.Actors = w.Actors[:1]
	w.Actors[0].Professions = map[string]int{"potter": 1}
	w.Catalog.Professions = []sim.Profession{{ID: "potter", MaxLevel: 1}}
	w.Catalog.TagProfiles["potter"] = sim.TagProfile{Produces: []string{"pottery"}, Consumes: []string{"clay"}, TargetStock: map[string]int{"pottery": 2, "clay": 2}}
	w.Catalog.Items["clay"] = sim.ItemSpec{ID: "clay", MassUnits: 1, Tradeable: true, BatchQty: 2, BatchPrice: 1}
	w.Catalog.Items["pottery"] = sim.ItemSpec{ID: "pottery", MassUnits: 1, Tradeable: true, BatchQty: 2, BatchPrice: 3}
	w.Catalog.Recipes = []sim.Recipe{
		{ID: "dig_clay", Profession: "potter", Ticks: 1, Inputs: map[string]int{"clay_deposit": 2}, OutputItem: "clay", OutputQty: 2, Conversion: &sim.MaterialConversion{Source: "fixture: two clay units"}},
		{ID: "fire_pottery", Profession: "potter", Ticks: 1, Inputs: map[string]int{"clay": 2}, OutputItem: "pottery", OutputQty: 2, Conversion: &sim.MaterialConversion{Source: "fixture: two clay units become two pots"}},
	}
	w.Catalog.Items["clay_deposit"] = sim.ItemSpec{ID: "clay_deposit", MassUnits: 1}
	w.Actors[0].Items["clay_deposit"] = 2
	w.Actors[0].NeedWeights = map[string]float64{"stock:pottery": 1.2}
	action, err := Offline().Decide(context.Background(), w.View("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != "craft" || action.RecipeID != "dig_clay" || action.Decision.Behavior.Strategy != "produce/fire_pottery" {
		t.Fatalf("making clay for pottery became an unrelated intention to sell clay: %+v", action)
	}
	boundaryApply(t, w, "alice", action)
	action, err = Offline().Decide(context.Background(), w.View("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != "craft" || action.RecipeID != "fire_pottery" || action.Decision.Behavior.Strategy != "produce/fire_pottery" {
		t.Fatalf("the production strategy did not move from material to final product: %+v", action)
	}
}

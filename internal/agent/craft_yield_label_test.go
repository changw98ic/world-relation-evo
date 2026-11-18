package agent

import (
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCraftOptionReportsWholeUnitsThatThisBatchWillDeliver(t *testing.T) {
	recipe := sim.Recipe{
		ID: "draw", Label: "Draw water", RequiredTag: "person", Ticks: 2, EnergyCost: 5,
		ResourcePool: "freshwater", ResourceDraw: 1, OutputItem: "water", OutputQty: 2,
		Conversion: &sim.MaterialConversion{ExternalMassUnits: 2, Source: "fixture: one freshwater unit funds at most two water units"},
	}
	view := sim.Perception{
		Self: sim.Actor{ID: "a", Tags: []string{"person"}, Location: "home", Energy: 100, Items: map[string]int{}},
		Map:  sim.WorldMap{Districts: []sim.District{{ID: "home"}}},
		Catalog: sim.Catalog{
			TagActions: map[string][]string{"person": {"craft", "wait"}},
			Items:      map[string]sim.ItemSpec{"water": {ID: "water", MassUnits: 1, Potable: true}},
			Recipes:    []sim.Recipe{recipe},
		},
		DistrictState:      &sim.DistrictState{Resources: map[string]int64{"freshwater": 20}},
		ExpectedCraftYield: map[string]int{"draw": 0},
	}
	label := craftLabel(t, view)
	if !strings.Contains(label, "expect 0 water") {
		t.Fatalf("option promised water this batch will not deliver: %s", label)
	}
	view.ExpectedCraftYield["draw"] = 2
	label = craftLabel(t, view)
	if !strings.Contains(label, "expect 2 water") {
		t.Fatalf("fully funded option hid the declared delivery: %s", label)
	}
}

func craftLabel(t *testing.T, view sim.Perception) string {
	t.Helper()
	for _, option := range Options(view) {
		if option.Action.RecipeID == "draw" {
			return option.Label
		}
	}
	t.Fatal("draw option was not offered")
	return ""
}

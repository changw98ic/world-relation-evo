package sim

import (
	"encoding/json"
	"testing"
)

// Fixtures are deliberately small budgets, not claims about real village yields.
// Every expected quantity in the tests comes from these declared budgets or an
// identity (a transfer has one debit and one credit), never from a yield helper.
func lawWorld() *World {
	return &World{
		ID: "laws", NextID: 1,
		Map: WorldMap{Districts: []District{{ID: "home", Features: []string{"farmland", "woodland"}}, {ID: "away"}, {ID: "beyond"}}},
		Catalog: Catalog{
			Calendar:   CalendarSpec{Seasons: []SeasonSpec{{ID: "steady", Days: 365}}},
			Lifecycle:  LifecycleSpec{ChildMaxAge: 15, ElderMinAge: 65, MaxExpectancy: 100},
			Production: ProductionSpec{FacilityCapacity: map[string]int64{"bench": 1}, FacilitySource: "fixture: one workstation", FieldAreaPerBatchM2: 1000, FieldSource: "fixture: one crop occupies 1000 square meters"},
			TagActions: map[string][]string{"person": {"wait", "craft", "harvest", "move", "speak", "offer", "accept", "sleep", "eat", "drink", "teach_knowledge", "research_tech", "adopt_facility", "borrow_loan", "accept_loan", "reject_loan", "repay_loan", "foreclose_collateral", "propose_agreement", "accept_agreement", "fulfill_agreement", "frame_suspect", "file_appeal"}},
			Items: map[string]ItemSpec{
				"wood":  {ID: "wood", MassUnits: 1, Tradeable: true, BatchQty: 3, BatchPrice: 7},
				"board": {ID: "board", MassUnits: 3},
				"seed":  {ID: "seed", MassUnits: 1},
				"water": {ID: "water", MassUnits: 1, Potable: true, ThirstRelief: 2},
				"grain": {ID: "grain", MassUnits: 1, NutritionUnits: 1, Edible: true},
			},
			Professions: []Profession{{ID: "worker", Name: "worker", MaxLevel: 1, Tier: 1, Grants: []string{"person"}}},
			Resources:   map[string]ResourcePoolSpec{"trees": {ID: "trees"}, "freshwater": {ID: "freshwater", PotableWater: true}},
			Recipes:     []Recipe{{ID: "board", RequiredTag: "person", Ticks: 3, EnergyCost: 4, Inputs: map[string]int{"wood": 3}, OutputItem: "board", OutputQty: 1, Conversion: &MaterialConversion{Source: "fixture: three unit masses become one board of mass three"}}},
		},
		Actors: []Actor{lawPerson("a"), lawPerson("b"), lawPerson("c")},
	}
}

func lawPerson(id string) Actor {
	return Actor{ID: id, Name: id, Role: "worker", Tags: []string{"person", "trader"}, Location: "home", AgeYears: 25, Health: 100, Energy: 100, Level: 1, Professions: map[string]int{"worker": 1}, Items: map[string]int{}, Trust: map[string]int{}}
}

func lawOK(t *testing.T, events []Event) {
	t.Helper()
	for _, event := range events {
		if event.Kind == "action_failed" {
			t.Fatalf("valid scenario rejected: %+v", event)
		}
	}
}

func lawRejected(t *testing.T, events []Event) {
	t.Helper()
	for _, event := range events {
		if event.Kind == "action_failed" {
			return
		}
	}
	t.Fatalf("invalid scenario was not rejected: %+v", events)
}

func lawJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func lawRestore(t *testing.T, w *World) *World {
	t.Helper()
	var restored World
	if err := json.Unmarshal(lawJSON(t, w), &restored); err != nil {
		t.Fatal(err)
	}
	return &restored
}

func lawCrop() Recipe {
	return Recipe{ID: "crop", RequiredTag: "person", NodeFeature: "farmland", Ticks: 1, Inputs: map[string]int{"seed": 1}, OutputItem: "grain", OutputQty: 3,
		Conversion: &MaterialConversion{ExternalMassUnits: 2, ExternalNutritionUnits: 3, Source: "fixture biological budget; no empirical yield claim"},
		Growth:     &GrowthRule{DurationTicks: 2, MaintenanceIntervalTicks: 1, CareWorkersPerBatch: 1, Supplies: map[string]int{"water": 1}, Source: "fixture: two supplied care ticks"}}
}

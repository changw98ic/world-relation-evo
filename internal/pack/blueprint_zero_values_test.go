package pack

import (
	"strings"
	"testing"
)

func TestBlueprintPreservesExplicitZeroAndFalse(t *testing.T) {
	raw := `{
		"id": "zero-semantics-test",
		"name": "Zero Semantics",
		"genre": "scifi_wasteland",
		"custom_items": [
			{"id": "soul_bound_relic", "name": "Soul Relic", "tier": 4, "tradeable": false, "batch_price": 100}
		],
		"districts": [
			{"id": "d1", "name": "D1", "features": ["market"]},
			{"id": "d2", "name": "D2", "features": ["market"]}
		],
		"roads": [
			{"from": "d1", "to": "d2", "ticks": 1, "energy_cost": 0, "medium": "land"}
		],
		"actors": [
			{"id": "pauper", "name": "Pauper", "role": "wanderer", "location": "d1", "money": 0}
		],
		"world_events": [
			{"id": "manual_only_event", "name": "Manual Event", "category": "artifact_emergence", "prob_permille_per_day": 0, "duration_days": 3}
		]
	}`
	bp, err := DecodeBlueprint(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("DecodeBlueprint failed: %v", err)
	}
	wp, err := CompileBlueprint(*bp)
	if err != nil {
		t.Fatalf("CompileBlueprint failed: %v", err)
	}
	if wp.Actors[0].Money != 0 {
		t.Fatalf("expected explicit money=0 to be preserved, got %d", wp.Actors[0].Money)
	}
	if wp.Map.Roads[0].EnergyCost != 0 {
		t.Fatalf("expected explicit energy_cost=0 to be preserved, got %d", wp.Map.Roads[0].EnergyCost)
	}
	if wp.Catalog.Items["soul_bound_relic"].Tradeable {
		t.Fatalf("expected explicit tradeable=false to be preserved, got true")
	}
	if wp.Catalog.WorldEvents[0].ProbPermillePerDay != 0 {
		t.Fatalf("expected explicit prob_permille_per_day=0 to be preserved, got %d", wp.Catalog.WorldEvents[0].ProbPermillePerDay)
	}
}

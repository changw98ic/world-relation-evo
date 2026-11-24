package agent

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestWiredHighTierActionsAndGeoAnomaly(t *testing.T) {
	w := sim.Village("wired-actions")
	w.Catalog.Items["crystal"] = sim.ItemSpec{
		Name:            "Energy Crystal",
		Tradeable:       true,
		DualUseCurrency: "credits",
		DualUseRate:     10,
	}
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{
		"charge": {ID: "charge", Name: "Charge", Min: 0, Max: 100},
	}
	if w.Catalog.TagActions == nil {
		w.Catalog.TagActions = map[string][]string{}
	}
	w.Catalog.TagActions["law_keeper"] = []string{"rewrite_world_law"}
	w.FlowGrid = &sim.FlowGridState{
		Nodes: []sim.FlowNode{
			{ID: "src", Kind: "generator", Generation: 10},
			{ID: "dst", Kind: "consumer", Demand: 5},
		},
		Edges: []sim.FlowEdge{
			{ID: "edge_live", FromNodeID: "src", ToNodeID: "dst", Capacity: 10},
			{ID: "edge_broken", FromNodeID: "src", ToNodeID: "dst", Capacity: 10, Severed: true},
		},
	}
	w.Map.GeoRegions = []sim.GeoRegion{
		{ID: "reg_a", Name: "Region A", ElevationM: 200},
		{ID: "reg_b", Name: "Region B", ElevationM: 50},
	}
	w.Map.GeoFlows = []sim.GeoFlowEdge{
		{FromRegion: "reg_a", ToRegion: "reg_b", Kind: "river", ConductancePermille: 800},
	}
	w.Map.Districts[0].GeoRegionID = "reg_a"
	w.Catalog.WorldEvents = []sim.WorldEventSpec{
		{
			ID:        "quake_test",
			Name:      "Fault Shift",
			Category:  "terrain_shift",
			Intensity: 60,
		},
	}

	a := &w.Actors[0]
	a.Tags = append(a.Tags, "law_keeper")
	a.Items["crystal"] = 3
	a.StateSlots = map[string]int{"charge": 10}
	w.InitLedgerAudit()

	p := w.View(a.ID)
	opts := Options(p)
	wantKinds := map[string]bool{
		"refine_currency_item": false,
		"absorb_item_slot":     false,
		"sever_flow_edge":      false,
		"repair_flow_edge":     false,
		"rewrite_world_law":    false,
	}
	for _, opt := range opts {
		if _, tracked := wantKinds[opt.Action.Kind]; tracked {
			wantKinds[opt.Action.Kind] = true
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Fatalf("expected %q to be generated in Options, missing from %d options", kind, len(opts))
		}
	}

	// Execute all 5 actions via Apply and verify ledger conservation.
	for _, act := range []sim.Action{
		{Kind: "refine_currency_item", Item: "crystal", Quantity: 1},
		{Kind: "absorb_item_slot", Item: "crystal", Quantity: 1, TargetID: "charge"},
		{Kind: "sever_flow_edge", TargetID: "edge_live"},
		{Kind: "repair_flow_edge", TargetID: "edge_broken"},
		{Kind: "rewrite_world_law", Item: "firearms_disabled", Quantity: 1},
	} {
		evs := w.Apply(a.ID, act)
		if len(evs) == 0 || evs[0].Kind == "action_failed" {
			t.Fatalf("action %+v unexpectedly failed: %+v", act, evs)
		}
	}
	if report := sim.InspectWorldConservation(w); !report.LedgerConserved {
		t.Fatalf("ledger broken after high-tier actions: %s", report.LedgerError)
	}

	// Triggering a terrain_shift world event must cascade TriggerGeoAnomaly and link root_event_id.
	wevs := w.TriggerWorldEvent("quake_test", w.Map.Districts[0].ID)
	sawGeo := false
	for _, ev := range wevs {
		if ev.Kind == "geo_anomaly_struck" {
			sawGeo = true
			if ev.Payload["root_event_id"] != wevs[0].ID {
				t.Fatalf("expected geo_anomaly_struck root_event_id=%d, got %v", wevs[0].ID, ev.Payload["root_event_id"])
			}
		}
	}
	if !sawGeo {
		t.Fatalf("expected TriggerWorldEvent(terrain_shift) to emit geo_anomaly_struck, got %+v", wevs)
	}

	// Verify Chronicle & Arcs pick up actor_married and cohort_turf_war (previously mismatched as married/turf_war).
	sampleEvents := []sim.Event{
		{ID: 1, Tick: 1, ActorID: a.ID, TargetID: w.Actors[1].ID, Kind: "actor_married", Payload: map[string]any{"text": "wed"}},
		{ID: 2, Tick: 2, ActorID: "c1", TargetID: "c2", Kind: "cohort_turf_war", Payload: map[string]any{"text": "turf clash"}},
	}
	rep := runtrace.BuildChronicle(sampleEvents, w, 0)
	if len(rep.Timeline) != 2 {
		t.Fatalf("expected both actor_married and cohort_turf_war in chronicle timeline, got %+v", rep.Timeline)
	}
	if rep.Drama.WarsAndClashes != 1 {
		t.Fatalf("expected cohort_turf_war counted in Drama.WarsAndClashes, got %+v", rep.Drama)
	}
}

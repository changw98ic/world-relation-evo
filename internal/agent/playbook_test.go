package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestLoadPlaybookValidatesExplicitDayWindows(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), []byte(`{"world_id":"local","horizon_days":30,"actors":[{"actor_id":"alice","output_file":"alice.json"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		window string
		want   [2]int
	}{
		{`"day_window":[7,3],`, [2]int{}},
		{`"day_window":[],`, [2]int{}},
		{`"day_window":[7,7],`, [2]int{7, 7}},
		{"", [2]int{1, 30}},
	} {
		raw := `{"actor_id":"alice","catalog":{"options":[{"id":"greeting",` + tt.window + `"action":{"engine_kind":"speak"}}]}}`
		if err := os.WriteFile(filepath.Join(dir, "alice.json"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		pb, err := LoadPlaybook(dir, sim.DefaultCatalog())
		if tt.want == [2]int{} {
			if err == nil || !strings.Contains(err.Error(), `option "greeting": invalid day_window`) {
				t.Fatalf("window %s: got %v", tt.window, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := pb.Actors["alice"].BaseOptions[0].DayWindow; got != tt.want {
			t.Fatalf("window %s: got %v, want %v", tt.window, got, tt.want)
		}
	}
}

func writeSyntheticPlaybookDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite := func(rel string, v any) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatalf("marshal %s: %v", rel, err)
		}
		if err := os.WriteFile(full, raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	mustWrite("summary.json", map[string]any{
		"world_id":     "test-world",
		"horizon_days": 30,
		"actors": []map[string]any{
			{"actor_id": "alice", "actor_name": "Alice", "role": "farmer", "location": "north", "output_file": "actors/alice.json"},
			{"actor_id": "bob", "actor_name": "Bob", "role": "merchant", "location": "north", "output_file": "actors/bob.json"},
		},
	})

	mustWrite("actors/alice.json", map[string]any{
		"world_id":   "test-world",
		"actor_id":   "alice",
		"actor_name": "Alice",
		"dossier": map[string]any{
			"world": map[string]any{
				"environment": sim.EnvironmentProfile{Genre: "test", CurrencyName: "credits"},
				"glossaries": map[string]any{
					"locations": map[string]string{"north": "North District", "south": "South District"},
				},
			},
			"actor": map[string]any{"role": "farmer", "location": "north"},
		},
		"catalog": map[string]any{
			"options": []map[string]any{
				{
					"id":         "opt_01",
					"category":   "主业经营",
					"title":      "Coordinate harvest with Bob",
					"day_window": []int{1, 5},
					"priority":   "urgent",
					"action": map[string]any{
						"engine_kind":      "speak",
						"target_actor_ids": []string{"bob"},
						"target_location":  "north",
						"summary":          "Discuss harvest split with Bob.",
					},
				},
				{
					"id":         "opt_02",
					"category":   "资源交易",
					"title":      "Scout southern market routes",
					"day_window": []int{1, 10},
					"priority":   "medium",
					"action": map[string]any{
						"engine_kind":      "speak",
						"target_actor_ids": []string{"bob"},
						"target_location":  "south",
						"summary":          "Ask Bob about the southern trade routes.",
					},
				},
			},
		},
	})

	mustWrite("actors/bob.json", map[string]any{
		"world_id":   "test-world",
		"actor_id":   "bob",
		"actor_name": "Bob",
		"dossier": map[string]any{
			"world": map[string]any{
				"environment": sim.EnvironmentProfile{Genre: "test", CurrencyName: "credits"},
			},
			"actor": map[string]any{"role": "merchant", "location": "north"},
		},
		"catalog": map[string]any{
			"options": []map[string]any{
				{
					"id":         "opt_01",
					"category":   "人脉结盟",
					"title":      "Offer partnership to Alice",
					"day_window": []int{1, 5},
					"priority":   "high",
					"action": map[string]any{
						"engine_kind":      "speak",
						"target_actor_ids": []string{"alice"},
						"target_location":  "north",
						"summary":          "Pitch trade terms.",
					},
				},
			},
		},
	})

	mustWrite("responses/summary.json", map[string]any{
		"scenarios": []map[string]any{
			{"scenario_id": "scn_01", "output_file": "scenarios/scn_01.json"},
		},
	})

	mustWrite("responses/scenarios/scn_01.json", map[string]any{
		"world_id": "test-world",
		"scenario": map[string]any{
			"id":               "scn_01",
			"collision_type":   "正面博弈",
			"title":            "North District Harvest Clash",
			"likelihood_score": 95,
			"likelihood_tier":  "极高",
			"day_window":       []int{1, 5},
			"shared_location":  "north",
			"participants": []map[string]any{
				{"actor_id": "alice", "actor_name": "Alice"},
				{"actor_id": "bob", "actor_name": "Bob"},
			},
			"trigger_options": []map[string]any{
				{"actor_id": "alice", "option_id": "opt_01", "title": "Coordinate harvest with Bob", "engine_kind": "speak", "target_location": "north"},
			},
		},
		"catalog": map[string]any{
			"scenario_id":       "scn_01",
			"scenario_title":    "North District Harvest Clash",
			"collision_summary": "Alice and Bob collide over harvest terms.",
			"responses": []map[string]any{
				{
					"id":                   "rsp_01",
					"responder_actor_id":   "bob",
					"responder_actor_name": "Bob",
					"stance":               "谈判妥协",
					"title":                "Counter-propose revenue share",
					"trigger_condition":    "Alice initiates harvest talk",
					"probability_hint":     "高",
					"action": map[string]any{
						"engine_kind":      "speak",
						"target_actor_ids": []string{"alice"},
						"target_location":  "north",
						"summary":          "Counter-offer fair split to Alice.",
					},
				},
				{
					"id":                   "rsp_02",
					"responder_actor_id":   "bob",
					"responder_actor_name": "Bob",
					"stance":               "暗中化解",
					"title":                "Spread counter-rumor in market",
					"trigger_condition":    "Alice pressures Bob",
					"probability_hint":     "低",
					"action": map[string]any{
						"engine_kind":      "speak",
						"target_actor_ids": []string{"alice"},
						"target_location":  "north",
						"summary":          "Quietly shift market sentiment.",
					},
				},
			},
		},
	})
	return dir
}

func testPerception(actorID, actorName, loc string, tick int64, nearby []sim.Neighbor, thirst, hunger, health, energy int) sim.Perception {
	cat := sim.DefaultCatalog()
	return sim.Perception{
		Tick:    tick,
		Clock:   sim.ClockAt(tick),
		Catalog: cat,
		Map: sim.WorldMap{
			Districts: []sim.District{
				{ID: "north", Name: "North District", Features: []string{"water_source", "market"}},
				{ID: "south", Name: "South District", Features: []string{"market"}},
			},
			Roads: []sim.Road{
				{From: "north", To: "south", Ticks: 1, EnergyCost: 2},
				{From: "south", To: "north", Ticks: 1, EnergyCost: 2},
			},
		},
		DistrictState: &sim.DistrictState{
			DistrictID: loc,
			Resources:  map[string]int64{"water_table": 100},
		},
		Roads: func() []sim.RoadView {
			if loc == "north" {
				return []sim.RoadView{{To: "south", Ticks: 1, EnergyCost: 2}}
			}
			return []sim.RoadView{{To: "north", Ticks: 1, EnergyCost: 2}}
		}(),
		Self: sim.Actor{
			ID:       actorID,
			Name:     actorName,
			Location: loc,
			Thirst:   thirst,
			Hunger:   hunger,
			Health:   health,
			Energy:   energy,
			Money:    50,
			Tags:     []string{"person", "resident", "trader"},
			Items:    map[string]int{"food": 3},
			Trust:    map[string]int{"alice": 2, "bob": 2},
		},
		Nearby: nearby,
	}
}

func TestLoadPlaybookAndReactiveChain(t *testing.T) {
	dir := writeSyntheticPlaybookDir(t)
	pb, err := LoadPlaybook(dir, sim.DefaultCatalog())
	if err != nil {
		t.Fatalf("LoadPlaybook failed: %v", err)
	}
	if len(pb.Actors) != 2 || len(pb.Scenarios) != 1 {
		t.Fatalf("unexpected playbook sizes: actors=%d scenarios=%d", len(pb.Actors), len(pb.Scenarios))
	}

	sel := NewPlaybookSelector(pb, nil)
	ctx := context.Background()

	// 1. Tier 0 survival override: thirsty Alice drinks instead of executing base option.
	pThirsty := testPerception("alice", "Alice", "north", 0, []sim.Neighbor{{ID: "bob", Name: "Bob", Health: 100}}, 3, 0, 100, 90)
	opts := Options(pThirsty)
	chosenID, err := sel.Select(ctx, pThirsty, opts)
	if err != nil {
		t.Fatalf("Select thirsty failed: %v", err)
	}
	chosen := findOptionByID(opts, chosenID)
	if chosen == nil || chosen.Action.Kind != "drink" {
		t.Fatalf("expected Tier 0 survival drink, got %+v", chosen)
	}

	// 2. Tier 2 base option: healthy Alice executes opt_01 (speak to Bob), which activates scn_01.
	pAlice := testPerception("alice", "Alice", "north", 1, []sim.Neighbor{{ID: "bob", Name: "Bob", Health: 100}}, 0, 0, 100, 90)
	optsAlice := Options(pAlice)
	aliceID, err := sel.Select(ctx, pAlice, optsAlice)
	if err != nil {
		t.Fatalf("Select alice base failed: %v", err)
	}
	aliceOpt := findOptionByID(optsAlice, aliceID)
	if aliceOpt == nil || aliceOpt.Action.Kind != "speak" || aliceOpt.Action.TargetID != "bob" {
		t.Fatalf("expected Alice speak->bob, got %+v", aliceOpt)
	}

	// 3. Tier 1 reactive counter-option: Bob sees scn_01 activated by Alice and fires a counter-option!
	pBob := testPerception("bob", "Bob", "north", 2, []sim.Neighbor{{ID: "alice", Name: "Alice", Health: 100}}, 0, 0, 100, 90)
	optsBob := Options(pBob)
	bobID, err := sel.Select(ctx, pBob, optsBob)
	if err != nil {
		t.Fatalf("Select bob counter failed: %v", err)
	}
	bobOpt := findOptionByID(optsBob, bobID)
	if bobOpt == nil || bobOpt.Action.Kind != "speak" || bobOpt.Action.TargetID != "alice" {
		t.Fatalf("expected Bob counter speak->alice, got %+v", bobOpt)
	}

	log := sel.SnapshotLog()
	if log.Stats.RulesSurvival != 1 {
		t.Fatalf("expected RulesSurvival=1, got %d", log.Stats.RulesSurvival)
	}
	if log.Stats.BaseDecisions != 1 {
		t.Fatalf("expected BaseDecisions=1, got %d", log.Stats.BaseDecisions)
	}
	if log.Stats.CounterDecisions != 1 {
		t.Fatalf("expected CounterDecisions=1, got %d", log.Stats.CounterDecisions)
	}
	if log.Stats.ScenariosActivated != 1 {
		t.Fatalf("expected ScenariosActivated=1, got %d", log.Stats.ScenariosActivated)
	}

	// 4. Alice travels south to execute the declared conversation.
	pAliceTravel := testPerception("alice", "Alice", "north", 30, nil, 0, 0, 100, 90)
	optsTravel := Options(pAliceTravel)
	travelID, err := sel.Select(ctx, pAliceTravel, optsTravel)
	if err != nil {
		t.Fatalf("Select alice travel failed: %v", err)
	}
	travelOpt := findOptionByID(optsTravel, travelID)
	if travelOpt == nil || travelOpt.Action.Kind != "move" || travelOpt.Action.Location != "south" {
		t.Fatalf("expected Alice move->south for opt_02, got %+v", travelOpt)
	}

	// After arriving, Alice executes the original action with its declared target.
	pAliceSouth := testPerception("alice", "Alice", "south", 31, []sim.Neighbor{{ID: "bob", Name: "Bob", Health: 100}}, 0, 0, 100, 88)
	optsSouth := Options(pAliceSouth)
	southID, err := sel.Select(ctx, pAliceSouth, optsSouth)
	if err != nil {
		t.Fatalf("Select alice arrived south failed: %v", err)
	}
	if option := findOptionByID(optsSouth, southID); option == nil || option.Action.Kind != "speak" || option.Action.TargetID != "bob" {
		t.Fatalf("expected Alice to speak to Bob, got %+v", option)
	}
	log2 := sel.SnapshotLog()
	if log2.Stats.TravelForBase != 1 || log2.Stats.SemanticFallbacks != 0 {
		t.Fatalf("expected TravelForBase=1 and SemanticFallbacks=0, got %+v", log2.Stats)
	}

	// 5. WriteLog round-trip.
	outPath := filepath.Join(t.TempDir(), "playbook_log.json")
	if err := sel.WriteLog(outPath); err != nil {
		t.Fatalf("WriteLog failed: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil || len(raw) < 50 {
		t.Fatalf("written log invalid: err=%v len=%d", err, len(raw))
	}
}

func TestPlaybookDeterminism(t *testing.T) {
	dir := writeSyntheticPlaybookDir(t)
	runOnce := func() PlaybookLog {
		pb, err := LoadPlaybook(dir, sim.DefaultCatalog())
		if err != nil {
			t.Fatalf("LoadPlaybook: %v", err)
		}
		sel := NewPlaybookSelector(pb, nil)
		ctx := context.Background()
		for tick := int64(0); tick < 60; tick++ {
			pA := testPerception("alice", "Alice", "north", tick, []sim.Neighbor{{ID: "bob", Name: "Bob", Health: 100}}, 0, 0, 100, 90)
			if _, err := sel.Select(ctx, pA, Options(pA)); err != nil {
				t.Fatalf("tick %d alice: %v", tick, err)
			}
			pB := testPerception("bob", "Bob", "north", tick, []sim.Neighbor{{ID: "alice", Name: "Alice", Health: 100}}, 0, 0, 100, 90)
			if _, err := sel.Select(ctx, pB, Options(pB)); err != nil {
				t.Fatalf("tick %d bob: %v", tick, err)
			}
		}
		return sel.SnapshotLog()
	}

	log1 := runOnce()
	log2 := runOnce()
	b1, _ := json.Marshal(log1)
	b2, _ := json.Marshal(log2)
	if string(b1) != string(b2) {
		t.Fatalf("PlaybookSelector is non-deterministic across identical runs")
	}
}

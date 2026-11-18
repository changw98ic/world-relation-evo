//go:build longrun

package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// One ordinary calendar year observes two 120-day crop periods even when the
// finite previous-season stock postpones the first planting. This is an
// observation window, not a product time or population limit. Deaths and absent
// harvests require causal review; neither alone is a failed physical invariant.
// Initial state, every decision and event, and final state are retained. The
// recorded decisions permit deterministic reconstruction of intermediate states.
func TestLongFoodCycles(t *testing.T) {
	days := 365
	if value := os.Getenv("WRE_FOOD_TEST_DAYS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			t.Fatal("WRE_FOOD_TEST_DAYS must be a positive observation period")
		}
		days = parsed
	}
	root := os.Getenv("WRE_LONG_ARTIFACT_DIR")
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "wre-food-cycles-")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "test-implementation"), binary, 0700); err != nil {
		t.Fatal(err)
	}
	t.Logf("FOOD_ARTIFACT_DIR=%s", root)
	for _, scenario := range []string{"village", "watershed"} {
		t.Run(scenario, func(t *testing.T) {
			var w *sim.World
			if scenario == "village" {
				w = sim.Village("rerun-village-rules")
			} else {
				p, err := pack.LoadFile("../../packs/mortal_watershed.json")
				if err != nil {
					t.Fatal(err)
				}
				w, err = p.Instantiate("rerun-watershed-rules")
				if err != nil {
					t.Fatal(err)
				}
			}
			prior := struct {
				TicksVerified       int64            `json:"ticks_verified"`
				Harvests            int              `json:"harvests"`
				FoodMatured         int64            `json:"food_matured"`
				FirstHarvest        map[string]int64 `json:"first_harvest"`
				SecondSeason        bool             `json:"second_season"`
				FirstStarvationTick int64            `json:"first_starvation_tick"`
			}{FirstHarvest: map[string]int64{}, FirstStarvationTick: -1}
			resumeRoot := os.Getenv("WRE_LONG_RESUME_ROOT")
			if resumeRoot != "" {
				expectedID := w.ID
				var restored sim.World
				for name, target := range map[string]any{"checkpoint.json": &restored, "history.json": &prior} {
					raw, err := os.ReadFile(filepath.Join(resumeRoot, scenario, name))
					if err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(raw, target); err != nil {
						t.Fatal(err)
					}
				}
				w = &restored
				w.PlaceResidents()
				if w.ID != expectedID || w.Tick != prior.TicksVerified || w.Tick < 0 || w.Tick > int64((days-1)*sim.TicksPerDay) || prior.FirstHarvest == nil {
					t.Fatal("resume checkpoint must match the scenario and recorded history and precede the last replay day")
				}
			}
			startTick := w.Tick
			dir := filepath.Join(root, scenario)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			writeState := func(name string, value any) []byte {
				t.Helper()
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
				return raw
			}
			writeState("initial.json", w)
			defer func() { writeState("final.json", w) }()
			file, err := os.OpenFile(filepath.Join(dir, "steps.ndjson.gz"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			compressed := gzip.NewWriter(file)
			defer func() {
				if err := compressed.Close(); err != nil {
					t.Error(err)
				}
				if err := file.Close(); err != nil {
					t.Error(err)
				}
			}()
			encoder := json.NewEncoder(compressed)
			var lastDay []sim.TickDecisionLog
			var checkpoint []byte
			firstStarvation := prior.FirstStarvationTick
			harvests, harvestQuantity := prior.Harvests, prior.FoodMatured
			firstHarvest := prior.FirstHarvest
			secondSeason := prior.SecondSeason
			for w.Tick < int64(days*sim.TicksPerDay) {
				if w.Tick == int64((days-1)*sim.TicksPerDay) {
					checkpoint, err = json.Marshal(w)
					if err != nil {
						t.Fatal(err)
					}
					lastDay = nil
				}
				events, decisions, err := w.StepRecorded(context.Background(), Offline())
				if err != nil {
					t.Fatal(err)
				}
				if checkpoint != nil {
					lastDay = append(lastDay, decisions)
				}
				if err := encoder.Encode(struct {
					Decisions sim.TickDecisionLog `json:"decisions"`
					Events    []sim.Event         `json:"events"`
				}{decisions, events}); err != nil {
					t.Fatal(err)
				}
				if err := w.VerifyLedgerConservation(); err != nil {
					t.Fatalf("tick %d: %v", w.Tick, err)
				}
				for _, event := range events {
					if event.Kind == "growth_matured" && event.Payload["item"] == "food" {
						if first, found := firstHarvest[event.ActorID]; found {
							if event.Tick-first >= 120*sim.TicksPerDay {
								secondSeason = true
							}
						} else {
							firstHarvest[event.ActorID] = event.Tick
						}
						harvests++
						switch quantity := event.Payload["quantity"].(type) {
						case int64:
							harvestQuantity += quantity
						case int:
							harvestQuantity += int64(quantity)
						default:
							t.Fatalf("unexpected harvest quantity %T", quantity)
						}
					}
				}
				if firstStarvation < 0 && w.DeathsByCause["starvation"] > 0 {
					firstStarvation = w.Tick
					writeState("first-starvation.json", w)
				}
				if w.Tick%sim.TicksPerDay == 0 {
					if err := encoder.Encode(map[string]any{"day": w.Tick / sim.TicksPerDay, "population": w.TotalPopulation(), "deaths": w.DeathsByCause, "harvests": harvests, "food_matured": harvestQuantity}); err != nil {
						t.Fatal(err)
					}
					if w.Tick%(30*sim.TicksPerDay) == 0 {
						writeState("checkpoint-latest.json", w)
						t.Logf("day=%d population=%d starvation=%d harvests=%d food_matured=%d", w.Tick/sim.TicksPerDay, w.TotalPopulation(), w.DeathsByCause["starvation"], harvests, harvestQuantity)
					}
				}
			}
			writeState("checkpoint-last-day.json", json.RawMessage(checkpoint))
			replayed, _, err := sim.ReplayFromCheckpoint(checkpoint, lastDay, w.Tick)
			if err != nil {
				t.Fatal(err)
			}
			replayedJSON, err := json.Marshal(replayed)
			if err != nil {
				t.Fatal(err)
			}
			finalJSON, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(finalJSON, replayedJSON) {
				t.Fatal("last day replay differs from actual settled world")
			}
			if err := os.WriteFile(filepath.Join(dir, "final-verified.json"), finalJSON, 0600); err != nil {
				t.Fatal(err)
			}
			writeState("result.json", map[string]any{"days": days, "start_tick": startTick, "end_tick": w.Tick, "resume_root": resumeRoot, "population": w.TotalPopulation(), "deaths": w.DeathsByCause, "first_starvation_tick": firstStarvation, "matured_batches": harvests, "food_matured": harvestQuantity, "second_season_observed": secondSeason, "last_day_replay_equal": true, "observation_completed": true, "simulation_correctness": "not_established", "requires_causal_review": true})
		})
	}
}

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPlannerScoresOnlyActionsAvailableToTheActor(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Tags = []string{"diner"}
	w.Catalog.TagActions = map[string][]string{"diner": {"wait", "eat"}}
	a.Thirst, a.Hunger = 2, 3
	a.Items["water"] = 1
	dir := t.TempDir()
	writer, err := runtrace.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	policy := Offline()
	policy.Trace = writer
	action, err := policy.Decide(context.Background(), w.View(a.ID))
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil || action.Kind != "eat" || action.Item != "food" {
		t.Fatalf("wanted available meal, got %+v, %v", action, err)
	}
	f, err := os.Open(filepath.Join(dir, "trace.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	var sawInput bool
	for decoder.More() {
		var entry runtrace.Entry
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry.Kind == "decision_input" {
			sawInput = true
			var data struct {
				Options []sim.Candidate
			}
			if err := json.Unmarshal(entry.Data, &data); err != nil {
				t.Fatal(err)
			}
			for _, candidate := range data.Options {
				if candidate.Action.Kind == "drink" {
					t.Fatal("an action outside the actor's capabilities entered the comparison")
				}
			}
		}
	}
	if !sawInput {
		t.Fatal("trace lost the planning input")
	}
	boundaryApply(t, w, a.ID, action)
	if a.Items["food"] != 19 {
		t.Fatal("meal was not consumed")
	}
}

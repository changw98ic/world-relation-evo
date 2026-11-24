//go:build evidence

package agent

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func recheckRecordedAction(world *sim.World, actor string, action sim.Action) (available, rejected bool) {
	choice := recordedChoiceAction(action)
	for _, option := range Options(world.View(actor)) {
		if reflect.DeepEqual(recordedChoiceAction(option.Action), choice) {
			available = true
		}
	}
	for _, outcome := range world.Apply(actor, action) {
		rejected = rejected || outcome.Kind == "action_failed"
	}
	return
}

func recordedChoiceAction(action sim.Action) sim.Action {
	switch action.Kind {
	case "speak", "transmit_message":
		action.Text, action.Statement, action.BasisEventID = "", nil, 0
	case "issue_decree":
		action.Text = ""
	case "accept":
		// The offer ID selects the trade. Rulebook citations only record why it was chosen.
		action.BasisEventID = 0
	}
	return action
}

func TestRecordedRecheckPreservesActualSpeechContent(t *testing.T) {
	w := boundaryWorld()
	action := boundaryOption(t, w, "alice", "speak", "bob")
	action.Text = "I have food to sell."
	available, rejected := recheckRecordedAction(w, "alice", action)
	if !available || rejected {
		t.Fatalf("original valid speech was changed by the review: available=%v rejected=%v", available, rejected)
	}
	// Generated wording and its citation do not replace the selected listener.
	withCitation := action
	withCitation.BasisEventID = 123
	withCitation.Statement = &sim.Statement{Topic: "item_produced", Item: "food", Quantity: 2}
	if !reflect.DeepEqual(recordedChoiceAction(withCitation), recordedChoiceAction(action)) {
		t.Fatal("generated citation was confused with a different system choice")
	}
	withCitation.TargetID = "charlie"
	if reflect.DeepEqual(recordedChoiceAction(withCitation), recordedChoiceAction(action)) {
		t.Fatal("review ignored a changed listener")
	}
}

// Recheck original failed choices against the world before any same-tick action.
// This separates stale candidates from legitimate competition for unreserved goods.
func TestRecordedCandidateRejectionsAgainstTheirOriginalWorld(t *testing.T) {
	dir := os.Getenv("WRE_EVIDENCE_TRACE_DIR")
	if dir == "" {
		t.Fatal("WRE_EVIDENCE_TRACE_DIR must name an actual trace directory")
	}
	file, err := os.Open(filepath.Join(dir, "trace.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	worlds := map[int64]json.RawMessage{}
	var findings []map[string]any
	decoder := json.NewDecoder(file)
	for {
		var entry runtrace.Entry
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if entry.Kind == "tick_begin" {
			var data struct{ World json.RawMessage }
			if err := json.Unmarshal(entry.Data, &data); err != nil {
				t.Fatal(err)
			}
			worlds[entry.Tick] = data.World
		}
		if entry.Kind != "tick_settled" {
			continue
		}
		var settled struct{ Events []sim.Event }
		if err := json.Unmarshal(entry.Data, &settled); err != nil {
			t.Fatal(err)
		}
		for _, event := range settled.Events {
			if event.Kind != "action_failed" {
				continue
			}
			var world sim.World
			if err := json.Unmarshal(worlds[entry.Tick], &world); err != nil {
				t.Fatal(err)
			}
			action := event.Action
			available, rejected := recheckRecordedAction(&world, event.ActorID, action)
			if available && rejected {
				t.Errorf("tick %d actor %s: current candidate still fails in the original world", entry.Tick, event.ActorID)
			}
			if err := world.VerifyHoldingBounds(); err != nil {
				t.Fatal(err)
			}
			classification := "not_offered_and_rejected"
			if !available && rejected {
				t.Errorf("tick %d actor %s: original rejected action is absent from current candidates and still fails; review the recorded choice and settlement", entry.Tick, event.ActorID)
			}
			if available && !rejected {
				classification = "valid_before_other_same_tick_actions"
			} else if available {
				classification = "current_candidate_still_invalid"
			} else if !rejected {
				classification = "executable_but_not_offered_needs_review"
				t.Errorf("tick %d actor %s: executable action is absent from current candidates; review perception and content fields", entry.Tick, event.ActorID)
			}
			findings = append(findings, map[string]any{"tick": entry.Tick, "actor": event.ActorID, "event_id": event.ID, "action": action, "current_candidate_available": available, "standalone_rejected": rejected, "classification": classification})
		}
	}
	output, err := os.OpenFile(filepath.Join(dir, "candidate-recheck.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(output).Encode(findings); err != nil {
		output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("rechecked %d recorded rejections", len(findings))
}

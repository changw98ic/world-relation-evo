package agent

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRecordedStepDecidesEveryIdleActorButNotOngoingTasks(t *testing.T) {
	w := sim.Village("individual-routing")
	w.Actors[0].Task = &sim.Task{Action: sim.Action{Kind: "sleep"}, TotalTicks: 16, Remaining: 15}
	w.Actors[0].Energy = 0
	expected := map[string]bool{}
	for _, a := range w.Actors {
		if !a.Dead && w.Tick >= a.JailedUntil && a.Task == nil {
			expected[a.ID] = true
		}
	}
	var mu sync.Mutex
	calls := map[string]int{}
	_, log, err := w.StepRecorded(context.Background(), boundaryDecider(func(_ context.Context, p sim.Perception) (sim.Action, error) {
		mu.Lock()
		calls[p.Self.ID]++
		mu.Unlock()
		return sim.Action{Kind: "wait"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) <= 8 || len(calls) != len(expected) || len(log.Decisions) != len(expected) {
		t.Fatalf("lost idle decisions: want=%d calls=%d recorded=%d", len(expected), len(calls), len(log.Decisions))
	}
	for id := range expected {
		if calls[id] != 1 {
			t.Fatalf("actor %s called %d times", id, calls[id])
		}
	}
	if calls[w.Actors[0].ID] != 0 {
		t.Fatal("busy actor was asked to decide again")
	}
}

func TestRecordedStepPreservesModelErrorWithoutSettlement(t *testing.T) {
	w := sim.Village("provider-failure")
	before := w.Tick
	failure := errors.New("provider fixture failure")
	target := w.Actors[len(w.Actors)-1].ID
	var mu sync.Mutex
	called := map[string]bool{}
	_, _, err := w.StepRecorded(context.Background(), boundaryDecider(func(_ context.Context, p sim.Perception) (sim.Action, error) {
		mu.Lock()
		called[p.Self.ID] = true
		mu.Unlock()
		if p.Self.ID == target {
			return sim.Action{}, failure
		}
		return sim.Action{Kind: "wait"}, nil
	}))
	if !errors.Is(err, failure) || w.Tick != before {
		t.Fatalf("provider failure hidden or settled: tick=%d err=%v", w.Tick, err)
	}
	want := map[string]bool{}
	for _, a := range w.Actors {
		want[a.ID] = true
	}
	if !reflect.DeepEqual(called, want) {
		t.Fatalf("not every idle actor reached selected decider: %d/%d", len(called), len(want))
	}
}

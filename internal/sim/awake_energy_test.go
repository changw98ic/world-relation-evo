package sim

import (
	"context"
	"testing"
)

func TestAwakeEnergySettlesBetweenDecisionsAndSkipsSleep(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	w.Actors[0].Energy = 80
	wait := Action{Kind: "wait", Decision: &BehaviorDecision{Behavior: BehaviorState{ReviewAt: 3}}}
	d := NewReplayDecider([]TickDecisionLog{
		{Tick: 0, Decisions: map[string]Action{"a": wait}},
		{Tick: 3, Decisions: map[string]Action{"a": {Kind: "sleep", Quantity: 1}}},
		{Tick: 4, Decisions: map[string]Action{"a": {Kind: "wait"}}},
	})
	step := func() []Event {
		t.Helper()
		events, err := w.Step(context.Background(), d)
		if err != nil {
			t.Fatal(err)
		}
		return events
	}

	step()
	w, err := DecodeWorldSnapshot(lawJSON(t, w))
	if err != nil {
		t.Fatal(err)
	}
	step()
	if w.Actors[0].Energy != 79 {
		t.Fatalf("awake waiting after reload: got %d energy, want 79", w.Actors[0].Energy)
	}
	step()
	gain, _ := SleepRecoveryTenths(w.Actors[0], nil, w.Tick)
	events := step()
	if len(events) == 0 || events[0].Kind != "slept" || w.Actors[0].Energy != 79+gain/10 {
		t.Fatalf("sleep must recover without an awake charge: energy=%d, events=%+v", w.Actors[0].Energy, events)
	}
	step()
	if w.Actors[0].Energy != 78+gain/10 {
		t.Fatalf("awake spending must resume on the following tick, got %d energy", w.Actors[0].Energy)
	}
}

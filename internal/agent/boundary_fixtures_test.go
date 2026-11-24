package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type boundarySelector func(context.Context, sim.Perception, []Option) (string, error)

func (f boundarySelector) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	return f(ctx, p, options)
}


func boundaryWorld() *sim.World {
	return &sim.World{ID: "boundary", Tick: 1, NextID: 100, Catalog: sim.DefaultCatalog(), Map: sim.WorldMap{Districts: []sim.District{{ID: "meeting"}, {ID: "remote"}}}, Actors: []sim.Actor{
		{ID: "alice", Name: "Alice", Location: "meeting", Tags: []string{"resident", "trader"}, Health: 100, Energy: 100, Money: 50, Items: map[string]int{"food": 20}, Trust: map[string]int{}},
		{ID: "bob", Name: "Bob", Location: "meeting", Tags: []string{"resident", "trader"}, Health: 100, Energy: 100, Money: 50, Items: map[string]int{}, Trust: map[string]int{}},
		{ID: "charlie", Name: "Charlie", Location: "meeting", Tags: []string{"resident", "trader"}, Health: 100, Energy: 100, Money: 50, Items: map[string]int{}, Trust: map[string]int{}},
	}}
}

func boundaryJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func boundaryOption(t *testing.T, w *sim.World, actor, kind, target string) sim.Action {
	t.Helper()
	for _, option := range Options(w.View(actor)) {
		if option.Action.Kind == kind && (target == "" || option.Action.TargetID == target) {
			return option.Action
		}
	}
	t.Fatalf("%s has no candidate %s -> %s", actor, kind, target)
	return sim.Action{}
}
func boundaryChoose(kind, target string) boundarySelector {
	return func(_ context.Context, _ sim.Perception, options []Option) (string, error) {
		for _, option := range options {
			if option.Action.Kind == kind && (target == "" || option.Action.TargetID == target) {
				return option.ID, nil
			}
		}
		return "", fmt.Errorf("fixture requires candidate %s -> %s", kind, target)
	}
}
func boundaryApply(t *testing.T, w *sim.World, actor string, action sim.Action) []sim.Event {
	t.Helper()
	events := w.Apply(actor, action)
	for _, event := range events {
		if event.Kind == "action_failed" {
			t.Fatalf("%s failed %s: %+v", actor, action.Kind, event.Payload)
		}
	}
	return events
}
func boundaryRejected(t *testing.T, w *sim.World, actor string, action sim.Action) {
	t.Helper()
	events := w.Apply(actor, action)
	if len(events) != 1 || events[0].Kind != "action_failed" {
		t.Fatalf("unauthorized action did not fail: %+v", events)
	}
}

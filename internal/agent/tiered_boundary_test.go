package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type boundaryDecider func(context.Context, sim.Perception) (sim.Action, error)

func (f boundaryDecider) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	return f(ctx, view)
}

func TestTieredBudgetRoutesAllActorsWhileLimitingOnlyModelExecution(t *testing.T) {
	// The caller chooses two model executions for ten eligible actors. Every
	// actor still gets a decision; the limit belongs to execution, not submission.
	const actors, budget = 10, 2
	var focalCalls, activeCalls atomic.Int64
	focal := boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) {
		focalCalls.Add(1)
		return sim.Action{Kind: "wait"}, nil
	})
	tiered := NewTieredDecider(focal, budget)
	tiered.Active = boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) {
		activeCalls.Add(1)
		return sim.Action{Kind: "wait"}, nil
	})
	for _, tick := range []int64{0, 1} {
		var group sync.WaitGroup
		results := make(chan error, actors)
		for i := 0; i < actors; i++ {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				view := sim.Perception{Tick: tick, Self: sim.Actor{ID: fmt.Sprint(i), Level: 2}, Nearby: []sim.Neighbor{{ID: "peer"}}}
				action, err := tiered.Decide(context.Background(), view)
				if err == nil && action.Kind != "wait" {
					err = errors.New("decision was lost")
				}
				results <- err
			}(i)
		}
		group.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		if focalCalls.Load() != (tick+1)*budget || activeCalls.Load() != (tick+1)*(actors-budget) {
			t.Fatal("execution budget rejected actors, exceeded model capacity, or failed to reset next tick")
		}
	}
}

func TestTieredProviderFailureCannotBecomeSuccessfulOfflineAction(t *testing.T) {
	failure := errors.New("invalid external choice")
	var focalCalls, activeCalls atomic.Int64
	tiered := NewTieredDecider(boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) {
		focalCalls.Add(1)
		return sim.Action{}, failure
	}), 1)
	tiered.Active = boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) {
		activeCalls.Add(1)
		return sim.Action{Kind: "wait"}, nil
	})
	view := sim.Perception{Self: sim.Actor{ID: "eligible", Level: 2}, Nearby: []sim.Neighbor{{ID: "peer"}}}
	_, err := tiered.Decide(context.Background(), view)
	if !errors.Is(err, failure) || activeCalls.Load() != 0 {
		t.Fatal("provider failure was hidden by a successful offline action")
	}
	if focalCalls.Load() != 1 {
		t.Fatal("a non-transient protocol error was retried")
	}
}

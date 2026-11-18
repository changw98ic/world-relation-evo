package agent

import (
	"context"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type boundaryDecider func(context.Context, sim.Perception) (sim.Action, error)

func (f boundaryDecider) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	return f(ctx, view)
}

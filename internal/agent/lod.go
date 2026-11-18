package agent

import (
	"context"
	"sync"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// TieredDecider implements a multi-resolution cognitive Level-of-Detail (LOD) decider
// for large and million-scale worlds:
//   - Tier 1 (Focal Agents): Elite/Tier-2 characters or active social/market hubs up to MaxFocalPerTick
//     are routed to the full Focal decider (e.g. Jev + ContentModel pipeline).
//   - Tier 2 (Active Agents): Remaining individual actors settle via the fast deterministic Active
//     rule decider (Offline()) in microseconds without external HTTP calls.
//   - Tier 3 (Macro Cohorts): Settled in bulk by World.Step's cohort engine.
type TieredDecider struct {
	Focal           sim.Decider
	Active          sim.Decider
	MaxFocalPerTick int
	FocalStride     int
	FocalDistricts  map[string]bool

	mu              sync.Mutex
	currentTick     int64
	focalCount      int
	totalFocalCalls int64
	prioritySet     map[string]bool
}

// SetSpotlightPriority pre-ranks hydrated focal actors by spotlight salience before a tick.
// Passing nil or an empty actor slice clears any prior spotlight override.
func (td *TieredDecider) SetSpotlightPriority(actors []sim.Actor, salience map[string]int, tick int64) {
	if td == nil {
		return
	}
	limit := td.MaxFocalPerTick
	if limit <= 0 {
		limit = 8
	}
	ordered := SalienceOrderedActorIDs(actors, salience, tick, limit)
	next := make(map[string]bool, len(ordered))
	for _, id := range ordered {
		next[id] = true
	}
	td.mu.Lock()
	if len(next) == 0 {
		td.prioritySet = nil
	} else {
		td.prioritySet = next
	}
	td.mu.Unlock()
}

// NewTieredDecider constructs a cognitive LOD decider wrapping a focal decider (such as Pipeline)
// with an offline rule fallback and a per-tick focal budget.
func NewTieredDecider(focal sim.Decider, maxFocalPerTick int, focalDistricts ...string) *TieredDecider {
	if maxFocalPerTick <= 0 {
		maxFocalPerTick = 8
	}
	var districts map[string]bool
	if len(focalDistricts) > 0 {
		districts = make(map[string]bool, len(focalDistricts))
		for _, d := range focalDistricts {
			if d != "" {
				districts[d] = true
			}
		}
	}
	return &TieredDecider{
		Focal:           focal,
		Active:          Offline(),
		MaxFocalPerTick: maxFocalPerTick,
		FocalStride:     1,
		FocalDistricts:  districts,
		currentTick:     -1,
	}
}

func (td *TieredDecider) TotalFocalCalls() int64 {
	td.mu.Lock()
	defer td.mu.Unlock()
	return td.totalFocalCalls
}

func (td *TieredDecider) Decide(ctx context.Context, p sim.Perception) (sim.Action, error) {
	active := td.Active
	if active == nil {
		active = Offline()
	}
	if td.Focal == nil {
		return active.Decide(ctx, p)
	}
	if td.FocalStride > 1 && p.Tick%int64(td.FocalStride) != 0 {
		return active.Decide(ctx, p)
	}
	if !td.qualifiesForFocal(p) {
		return active.Decide(ctx, p)
	}
	if !td.acquireFocalSlot(p.Tick) {
		return active.Decide(ctx, p)
	}
	return td.Focal.Decide(ctx, p)
}

func (td *TieredDecider) qualifiesForFocal(p sim.Perception) bool {
	a := p.Self
	if len(td.FocalDistricts) > 0 && !td.FocalDistricts[a.Location] {
		return false
	}
	td.mu.Lock()
	hasPrioritySet := len(td.prioritySet) > 0
	inPrioritySet := hasPrioritySet && td.prioritySet[a.ID]
	td.mu.Unlock()
	if hasPrioritySet {
		return inPrioritySet || p.SpotlightSalience >= 15
	}
	if p.SpotlightSalience >= 15 {
		return true
	}
	// Elite / Tier-2 advanced profession holders or high-level figures with neighbors
	for profID := range a.Professions {
		if prof, ok := p.Catalog.ProfessionByID(profID); ok && prof.Tier >= 2 {
			return true
		}
	}
	if a.Level >= 2 && len(p.Nearby) > 0 {
		return true
	}
	// Actors with active trade offers or fresh observations/rumors in social settings
	if len(p.Offers) > 0 && len(p.Nearby) > 0 {
		return true
	}
	if len(p.Nearby) > 0 {
		for i := len(a.Memories) - 1; i >= 0; i-- {
			m := a.Memories[i]
			if p.Tick-m.Tick > 8 {
				break
			}
			if m.Observation != nil || m.Statement != nil {
				return true
			}
		}
	}
	return false
}

func (td *TieredDecider) acquireFocalSlot(tick int64) bool {
	td.mu.Lock()
	defer td.mu.Unlock()
	limit := td.MaxFocalPerTick
	if limit <= 0 {
		limit = 8
	}
	if td.currentTick != tick {
		td.currentTick = tick
		td.focalCount = 0
	}
	if td.focalCount >= limit {
		return false
	}
	td.focalCount++
	td.totalFocalCalls++
	return true
}

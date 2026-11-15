package agent

import (
	"encoding/binary"
	"hash/fnv"
	"sort"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// SalienceOrderedActorIDs ranks awake, idle focal actors deterministically by spotlight salience
// descending with a per-tick FNV-1a tie-breaker so cognitive LOD budgets prioritize high-drama
// protagonists without starving lower-salience actors over time.
func SalienceOrderedActorIDs(actors []sim.Actor, salience map[string]int, tick int64, limit int) []string {
	type rankedActor struct {
		id       string
		salience int
		hash     uint64
	}
	ranked := make([]rankedActor, 0, len(actors))
	for i := range actors {
		a := &actors[i]
		if a.Dead || tick < a.JailedUntil || a.Task != nil {
			continue
		}
		sal := 0
		if salience != nil {
			sal = salience[a.ID]
		}
		ranked = append(ranked, rankedActor{
			id:       a.ID,
			salience: sal,
			hash:     spotlightTieHash(tick, a.ID),
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].salience != ranked[j].salience {
			return ranked[i].salience > ranked[j].salience
		}
		if ranked[i].hash != ranked[j].hash {
			return ranked[i].hash < ranked[j].hash
		}
		return ranked[i].id < ranked[j].id
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	out := make([]string, len(ranked))
	for i, r := range ranked {
		out[i] = r.id
	}
	return out
}

func spotlightTieHash(tick int64, actorID string) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(tick))
	h.Write(buf[:])
	h.Write([]byte(actorID))
	return h.Sum64()
}

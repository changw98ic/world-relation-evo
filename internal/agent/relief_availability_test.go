package agent

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestReliefCandidateRequiresActualRecipientsAndAllowsTheLastCoin(t *testing.T) {
	for _, needy := range []bool{false, true} {
		treasury := int64(20)
		if needy {
			treasury = 1
		}
		w := boundaryWorld()
		w.Actors = w.Actors[:2]
		w.Actors[0].Tags = append(w.Actors[0].Tags, "ruler")
		w.Map.Districts[0].PolityID = "county"
		w.Polities = &sim.PolityNetwork{Nodes: []sim.PolityNode{{ID: "county", LeaderActorID: "alice", SeatDistrictID: "meeting", Treasury: treasury}}}
		if needy {
			w.Actors[1].Money = 0
		}
		var action sim.Action
		for _, candidate := range Options(w.View("alice")) {
			if candidate.Action.Kind == "issue_decree" && candidate.Action.Item == "relief_grant" {
				action = candidate.Action
			}
		}
		if (action.Kind != "") != needy {
			t.Fatalf("needy=%v: relief candidate=%+v", needy, action)
		}
		if needy {
			if action.Quantity != 1 {
				t.Fatal("the final available coin could not be allocated")
			}
			boundaryApply(t, w, "alice", action)
			if w.Actors[1].Money != 1 || w.Polities.Nodes[0].Treasury != 0 {
				t.Fatal("relief did not reach its actual intended recipient")
			}
		}
	}
}

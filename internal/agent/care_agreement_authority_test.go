package agent

import (
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestFirstSignedCareContractSurvivesCompetingAssentAndReload(t *testing.T) {
	for _, winner := range []string{"bob", "charlie"} {
		t.Run(winner, func(t *testing.T) {
			w := boundaryWorld()
			w.Growth = []sim.GrowthBatch{{ID: 9, OwnerID: "alice", District: "meeting", Item: "food", Quantity: 10, Batches: 1}}
			accept := map[string]sim.Action{}
			for _, caretaker := range []string{"bob", "charlie"} {
				proposal := boundaryOption(t, w, "alice", "propose_growth_care", caretaker)
				boundaryApply(t, w, "alice", proposal)
				accept[caretaker] = boundaryOption(t, w, caretaker, "accept_growth_care", "alice")
			}
			boundaryApply(t, w, winner, accept[winner])
			loser := "bob"
			if winner == loser {
				loser = "charlie"
			}
			if len(w.Production.CareProposals) != 0 {
				t.Fatal("signing left competing invitations open")
			}
			// Older snapshots can contain an outstanding invitation from before signing.
			w.Production.CareProposals = append(w.Production.CareProposals, *accept[loser].Care)
			restored := new(sim.World)
			if err := json.Unmarshal([]byte(boundaryJSON(t, w)), restored); err != nil {
				t.Fatal(err)
			}
			if hasBoundaryKind(restored, loser, "accept_growth_care") || hasBoundaryKind(restored, "alice", "propose_growth_care") {
				t.Fatal("active terms were offered for replacement")
			}
			boundaryRejected(t, restored, loser, accept[loser])
			agreements := restored.Production.CareAgreements
			if len(agreements) != 1 || agreements[0].CaretakerID != winner {
				t.Fatal("stale assent overwrote active caretaker")
			}
		})
	}
}

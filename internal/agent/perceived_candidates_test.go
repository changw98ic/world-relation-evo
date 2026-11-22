package agent

import (
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestBlackmailCandidatesUseObservedRefusalsAndIgnorePrivateWallet(t *testing.T) {
	w := boundaryWorld()
	offender, victim := &w.Actors[0], &w.Actors[1]
	offender.Personality, victim.Personality = "cunning", "timid"
	w.Catalog.TagActions["coercer"] = []string{"blackmail"}
	offender.Tags = append(offender.Tags, "coercer")
	w.Secrets = []sim.ActorSecret{{
		ID: "secret", Kind: "offense", OwnerID: victim.ID, Severity: 40,
		KnownBy: []string{offender.ID}, HolderStances: map[string]string{offender.ID: "leverage"},
	}}
	blackmail := func() []sim.Action {
		var result []sim.Action
		for _, option := range Options(w.View(offender.ID)) {
			if option.Action.Kind == "blackmail" {
				result = append(result, option.Action)
			}
		}
		return result
	}
	funded := blackmail()
	victim.Money = 0
	unfunded := blackmail()
	if len(funded) != 1 || !reflect.DeepEqual(funded, unfunded) {
		t.Fatalf("private wallet changed available candidates: funded %v, unfunded %v", funded, unfunded)
	}
	events := w.Apply(offender.ID, unfunded[0])
	if len(events) == 0 || events[0].Kind != "blackmail_refused" {
		t.Fatalf("an unfunded demand must be refused at settlement: %v", events)
	}
	if got := blackmail(); len(got) != 0 {
		t.Fatalf("candidate repeated an observed refusal: %v", got)
	}
}

func TestLoanCandidateUsesVisibleTraderRoleAndAwakeState(t *testing.T) {
	w := boundaryWorld()
	borrower, lender := &w.Actors[0], &w.Actors[1]
	borrower.Money, lender.Money = 0, 0
	w.Catalog.TagActions["borrower"] = []string{"borrow_loan"}
	borrower.Tags = append(borrower.Tags, "borrower")
	if got := boundaryOption(t, w, borrower.ID, "borrow_loan", lender.ID); got.TargetID != lender.ID {
		t.Fatalf("wanted a request addressed to the visible trader, got %+v", got)
	}
	lender.Task = &sim.Task{Action: sim.Action{Kind: "sleep"}, Remaining: 2}
	for _, option := range Options(w.View(borrower.ID)) {
		if option.Action.Kind == "borrow_loan" && option.Action.TargetID == lender.ID {
			t.Fatalf("offered a request to an observably sleeping trader: %+v", option)
		}
	}
}

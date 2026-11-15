package agent

import (
	"context"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"testing"
)

func TestLoanOptionsReachBilateralSettlement(t *testing.T) {
	w := sim.Village("loan-options")
	borrower, lender := &w.Actors[0], &w.Actors[26]
	borrower.Tags = append(borrower.Tags, "trader")
	lender.Tags = append(lender.Tags, "trader")
	lender.Location, lender.InsideHouse = borrower.Location, borrower.InsideHouse
	borrower.Money, lender.Money = 0, 100
	choose := func(id, kind, target string) sim.Action {
		t.Helper()
		for _, o := range Options(w.View(id)) {
			if o.Action.Kind == kind && o.Action.TargetID == target {
				return o.Action
			}
		}
		t.Fatalf("missing %s option for %s to %s", kind, id, target)
		return sim.Action{}
	}
	request := choose(borrower.ID, "borrow_loan", lender.ID)
	w.Apply(borrower.ID, request)
	if borrower.Money != 0 || lender.Money != 100 {
		t.Fatal("request transferred assets")
	}
	w.Apply(lender.ID, choose(lender.ID, "accept_loan", borrower.ID))
	if borrower.Money != 12 || lender.Money != 88 {
		t.Fatalf("bilateral route not settled: %d/%d", borrower.Money, lender.Money)
	}
}

func TestRuleSelectorDoesNotSubstituteAgreementIDs(t *testing.T) {
	w := sim.Village("exact-agreement")
	p := w.View(w.Actors[0].ID)
	p.Self.Thirst, p.Self.Hunger, p.Self.Waste = 0, 0, 0
	p.Self.Health, p.Self.Energy = 100, 100
	p.Self.DiseaseID = ""
	p.Clock.IsNight = false
	p.Clock.Hour = 12
	p.Self.SleepTrait = "early_bird"
	p.Continuity.Agreements = []sim.Agreement{{ID: 2, SupplierID: p.Self.ID, BuyerID: "other", Item: "food", Quantity: 1, Price: 1, Status: "active", DueTick: 100}}
	p.Continuity.FulfillableAgreementIDs = []int64{2}
	p.Nearby = []sim.Neighbor{{ID: "other", Tags: []string{"trader"}}}
	p.Self.Items["food"] = 10
	p.Self.Tags = append(p.Self.Tags, "trader")
	options := []Option{{ID: "wrong", Action: sim.Action{Kind: "fulfill_agreement", TargetID: "other", Item: "food", AgreementID: 1}}, {ID: "right", Action: sim.Action{Kind: "fulfill_agreement", TargetID: "other", Item: "food", AgreementID: 2}}, {ID: "wait", Action: sim.Action{Kind: "wait"}}}
	id, err := (RuleSelector{}).Select(context.Background(), p, options)
	if err != nil || id != "right" {
		t.Fatalf("selected wrong agreement: %s %v", id, err)
	}
}

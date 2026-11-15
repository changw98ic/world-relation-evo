package agent

import (
	"context"
	"encoding/json"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"testing"
)

func continuityPolicyView() sim.Perception {
	w := sim.Village("policy")
	w.Actors[0].InsideHouse = ""
	w.Actors[1].InsideHouse = ""
	w.Actors[0].Location = "south_fields"
	w.Actors[1].Location = "south_fields"
	w.Actors[0].Hunger = 0
	w.Actors[0].Thirst = 0
	w.Actors[0].Energy = 100
	p := w.View("p01")
	p.Clock.IsNight = false
	p.Clock.Hour = 12
	return p
}

func TestRulesLoanChoicesUseSeparateCharacters(t *testing.T) {
	p := continuityPolicyView()
	p.Self.Money = 40
	p.LoanRequests = []sim.LoanRequest{{ID: 1, LenderID: p.Self.ID, BorrowerID: "p02", Principal: 12, Status: "pending"}}
	for _, tc := range []struct {
		trust, money int
		kind         string
	}{{0, 40, "accept_loan"}, {-1, 40, "reject_loan"}, {0, 20, "reject_loan"}} {
		p.Self.Trust["p02"] = tc.trust
		p.Self.Money = tc.money
		before, _ := json.Marshal(p)
		x, ok := chooseContinuity(p)
		after, _ := json.Marshal(p)
		if !ok || x.Kind != tc.kind || x.TargetID != "p02" {
			t.Fatalf("choice %+v, want %s", x, tc.kind)
		}
		if string(before) != string(after) {
			t.Fatal("policy changed world state")
		}
	}
	p.LoanRequests = nil
	p.Self.Money = 0
	p.Self.Items = map[string]int{}
	p.Self.Trust["p02"] = 0
	p.LoanQuotes = []sim.LoanRequest{{BorrowerID: p.Self.ID, LenderID: "p02", Principal: 12}}
	x, ok := chooseContinuity(p)
	if !ok || x.Kind != "borrow_loan" {
		t.Fatalf("borrower did not request: %+v", x)
	}
	p.LoanRequests = []sim.LoanRequest{{BorrowerID: p.Self.ID, LenderID: "p02", Status: "pending"}}
	if x, ok := chooseContinuity(p); ok && x.Kind == "borrow_loan" {
		t.Fatal("duplicated pending request")
	}
}

func TestRulesAgreementResponseAndDelivery(t *testing.T) {
	p := continuityPolicyView()
	p.Self.Items["food"] = 0
	p.Self.Money = 10
	p.Continuity.Agreements = []sim.Agreement{{ID: 7, BuyerID: p.Self.ID, SupplierID: "p02", Item: "food", Quantity: 2, Price: 2, Status: "proposed", ResponseBy: p.Tick + 10}}
	x, ok := chooseContinuity(p)
	if !ok || x.Kind != "accept_agreement" || x.AgreementID != 7 {
		t.Fatalf("missing acceptance %+v", x)
	}
	p.Self.Trust["p02"] = -1
	x, ok = chooseContinuity(p)
	if !ok || x.Kind != "reject_agreement" {
		t.Fatalf("missing rejection %+v", x)
	}
	p.Continuity.Agreements[0] = sim.Agreement{ID: 7, SupplierID: p.Self.ID, BuyerID: "p02", Item: "food", Quantity: 2, Price: 2, Status: "active", DueTick: p.Tick + 10}
	p.Self.Items["food"] = 3
	p.Continuity.FulfillableAgreementIDs = []int64{7}
	x, ok = chooseContinuity(p)
	if !ok || x.Kind != "fulfill_agreement" {
		t.Fatalf("missing delivery %+v", x)
	}
}

func TestRulesPlanResumesAfterUrgentNeeds(t *testing.T) {
	p := continuityPolicyView()
	p.Self.Plan = &sim.PersonalPlan{RecipeID: "farm_grain", Item: "food", Quantity: 2, Status: "active", Stage: "produce"}
	p.Self.Hunger = 1
	p.Self.Items["food"] = 1
	x, err := (rulesPolicy{}).Decide(context.Background(), p)
	if err != nil || x.Kind != "eat" {
		t.Fatalf("plan preempted hunger: %+v %v", x, err)
	}
	p.Self.Hunger = 0
	p.Self.Items["food"] = 0
	x, err = (rulesPolicy{}).Decide(context.Background(), p)
	if err != nil || x.Kind != "craft" || x.RecipeID != "farm_grain" {
		t.Fatalf("plan not resumed: %+v %v", x, err)
	}
	p.Self.Energy = 5
	x, _ = (rulesPolicy{}).Decide(context.Background(), p)
	if x.Kind == "craft" {
		t.Fatal("plan preempted exhaustion")
	}
}

func TestRulesStartsProductionPlanThroughOfflinePipeline(t *testing.T) {
	p := continuityPolicyView()
	p.Self.Plan = nil
	p.Self.Items = map[string]int{"food": 0, "water": 1, "seeds": 1}
	p.Factions = nil
	p.Nearby = nil
	p.Self.Memories = nil
	x, err := Offline().Decide(context.Background(), p)
	if err != nil || x.Kind != "start_plan" || x.RecipeID != "farm_grain" {
		t.Fatalf("offline path cannot select plan: %+v %v", x, err)
	}
}

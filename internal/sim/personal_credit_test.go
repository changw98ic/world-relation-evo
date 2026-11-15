package sim_test

import (
	"encoding/json"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"testing"
)

func TestPersonalCreditRequiresBothDecisions(t *testing.T) {
	w := sim.Village("bilateral")
	mc := sim.NewMarketCreditState()
	borrower, lender := actorByID(w, "p01"), actorByID(w, "p27")
	lender.Location, lender.InsideHouse = borrower.Location, borrower.InsideHouse
	borrower.Money, lender.Money = 0, 100
	w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID})
	if borrower.Money != 0 || lender.Money != 100 || len(mc.Loans) != 0 || len(mc.LoanRequests) != 1 {
		t.Fatal("request must not originate debt or transfer money")
	}
	w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "accept_loan", TargetID: lender.ID})
	if len(mc.Loans) != 0 {
		t.Fatal("borrower cannot accept its own request")
	}
	w.ApplyMarketCreditAction(mc, lender.ID, sim.Action{Kind: "reject_loan", TargetID: borrower.ID})
	if mc.LoanRequests[0].Status != "rejected" || borrower.Money != 0 || lender.Money != 100 {
		t.Fatal("rejection changed assets")
	}
	w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID})
	data, err := json.Marshal(mc)
	if err != nil {
		t.Fatal(err)
	}
	restored := sim.NewMarketCreditState()
	if err = json.Unmarshal(data, restored); err != nil {
		t.Fatal(err)
	}
	clone := sim.CloneMarketCreditState(restored)
	clone.LoanRequests[1].Status = "rejected"
	if restored.LoanRequests[1].Status != "pending" {
		t.Fatal("request snapshot aliases")
	}
	w.ApplyMarketCreditAction(restored, lender.ID, sim.Action{Kind: "accept_loan", TargetID: borrower.ID})
	if borrower.Money != 12 || lender.Money != 88 || len(restored.Loans) != 1 {
		t.Fatal("acceptance must settle saved request")
	}
	w.ApplyMarketCreditAction(restored, lender.ID, sim.Action{Kind: "accept_loan", TargetID: borrower.ID})
	if borrower.Money != 12 || len(restored.Loans) != 1 {
		t.Fatal("duplicate acceptance transferred twice")
	}
}

func TestDailyCreditDoesNotDecidePersonalBorrowing(t *testing.T) {
	w := sim.Village("no-automatic-borrowing")
	mc := sim.NewMarketCreditState()
	actorByID(w, "p01").Money = 0
	actorByID(w, "p27").Money = 100
	w.Tick = sim.TicksPerDay
	events := w.StepMarketAndCredit(mc)
	if len(mc.Loans) != 0 || len(mc.LoanRequests) != 0 {
		t.Fatal("daily settlement created personal debt without a decision")
	}
	for _, e := range events {
		if e.Kind == "loan_issued" {
			t.Fatal("daily settlement issued personal loan")
		}
	}
}

func TestPersonalCreditKeepsTermsUntilAcceptance(t *testing.T) {
	w := sim.Village("terms")
	mc := sim.NewMarketCreditState()
	borrower, lender := actorByID(w, "p01"), actorByID(w, "p27")
	lender.Location, lender.InsideHouse = borrower.Location, borrower.InsideHouse
	borrower.Money, lender.Money = 0, 100
	w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID})
	terms := mc.LoanRequests[0]
	lender.Money = 5
	evs, _ := w.ApplyMarketCreditAction(mc, lender.ID, sim.Action{Kind: "accept_loan", TargetID: borrower.ID})
	if evs[0].Kind != "action_failed" || borrower.Money != 0 || len(mc.Loans) != 0 {
		t.Fatal("unfunded consent moved assets")
	}
	lender.Money = 100
	mc.Spec.DailyLoanInterestPermille = 99
	w.Tick = 7
	w.ApplyMarketCreditAction(mc, lender.ID, sim.Action{Kind: "accept_loan", TargetID: borrower.ID})
	loan := mc.Loans[0]
	if loan.Principal != int64(terms.Principal) || loan.RatePermille != terms.RatePermille || loan.DueTick != 7+int64(terms.TermTicks) {
		t.Fatalf("terms silently changed: %+v", loan)
	}
}

func TestPersonalCreditBorrowerCanCancelAbsentLender(t *testing.T) {
	for _, absent := range []string{"away", "dead", "removed"} {
		t.Run(absent, func(t *testing.T) {
			w := sim.Village("cancel-loan")
			mc := sim.NewMarketCreditState()
			borrower, lender := actorByID(w, "p01"), actorByID(w, "p27")
			lender.Location, lender.InsideHouse = borrower.Location, borrower.InsideHouse
			borrower.Money, lender.Money = 0, 100
			borrowerID, lenderID := borrower.ID, lender.ID
			w.ApplyMarketCreditAction(mc, borrowerID, sim.Action{Kind: "borrow_loan", TargetID: lenderID})
			// The creditor cannot cancel in the borrower's name.
			evs, _ := w.ApplyMarketCreditAction(mc, lenderID, sim.Action{Kind: "cancel_loan", TargetID: borrowerID})
			if evs[0].Kind != "action_failed" || mc.LoanRequests[0].Status != "pending" {
				t.Fatal("wrong party cancelled request")
			}
			switch absent {
			case "away":
				lender.Location = "elsewhere"
			case "dead":
				lender.Dead = true
			case "removed":
				for i, a := range w.Actors {
					if a.ID == lenderID {
						w.Actors = append(w.Actors[:i], w.Actors[i+1:]...)
						break
					}
				}
			}
			evs, _ = w.ApplyMarketCreditAction(mc, borrowerID, sim.Action{Kind: "cancel_loan", TargetID: lenderID})
			if evs[0].Kind != "loan_cancelled" || mc.LoanRequests[0].Status != "cancelled" || len(mc.Loans) != 0 || actorByID(w, borrowerID).Money != 0 {
				t.Fatal("cancellation failed or moved assets")
			}
			// A new request is possible immediately; no timer or manual cleanup needed.
			borrower = actorByID(w, borrowerID)
			replacement := actorByID(w, "p03")
			replacement.Tags = append(replacement.Tags, "trader")
			replacement.Location, replacement.InsideHouse = borrower.Location, borrower.InsideHouse
			replacement.Money = 100
			evs, _ = w.ApplyMarketCreditAction(mc, borrowerID, sim.Action{Kind: "borrow_loan", TargetID: replacement.ID})
			if evs[0].Kind != "loan_requested" {
				t.Fatalf("cancelled request blocked new lender: %+v", evs)
			}
		})
	}
}

func TestDailyCreditDoesNotChoosePersonalRepayment(t *testing.T) {
	w := sim.Village("repayment-choice")
	mc := sim.NewMarketCreditState()
	borrower, lender := actorByID(w, "p01"), actorByID(w, "p27")
	lender.Location, lender.InsideHouse = borrower.Location, borrower.InsideHouse
	borrower.Money, lender.Money = 100, 100
	w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID})
	w.ApplyMarketCreditAction(mc, lender.ID, sim.Action{Kind: "accept_loan", TargetID: borrower.ID})
	beforeBorrower, beforeLender := borrower.Money, lender.Money
	w.Tick = int64(mc.Spec.LoanTermTicks) + 1
	events := w.StepMarketAndCredit(mc)
	if borrower.Money != beforeBorrower || lender.Money != beforeLender || mc.Loans[0].Repaid != 0 || mcTestHouseByID(w, "h01").OwnerID != borrower.ID {
		t.Fatal("daily credit made a personal repayment/foreclosure decision")
	}
	if w.Tick < mc.Loans[0].DueTick || mc.Loans[0].AccruedInterest == 0 {
		t.Fatal("daily credit failed to update debt facts")
	}
	for _, e := range events {
		if e.Kind == "loan_repaid" || e.Kind == "house_foreclosed" {
			t.Fatalf("unselected personal action: %s", e.Kind)
		}
	}
	evs, _ := w.ApplyMarketCreditAction(mc, borrower.ID, sim.Action{Kind: "repay_loan", TargetID: lender.ID})
	if evs[0].Kind != "loan_repaid" || mc.Loans[0].Status != "repaid" || borrower.Money >= beforeBorrower || lender.Money <= beforeLender {
		t.Fatal("explicit repayment failed")
	}
}

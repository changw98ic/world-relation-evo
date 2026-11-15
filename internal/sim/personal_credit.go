package sim

import "fmt"

// LoanRequest contains the engine's concrete terms, pending the lender's decision.
// No money or property moves until that character accepts these same terms.
type LoanRequest struct {
	ID              int64  `json:"id"`
	BorrowerID      string `json:"borrower_id"`
	LenderID        string `json:"lender_id"`
	Principal       int    `json:"principal"`
	RatePermille    int    `json:"rate_permille"`
	TermTicks       int    `json:"term_ticks"`
	CollateralHouse string `json:"collateral_house,omitempty"`
	RequestedTick   int64  `json:"requested_tick"`
	Status          string `json:"status"`
}

// QuotePersonalLoan computes a small subsistence loan; selection models choose
// the action and partner, not financial amounts or collateral.
func QuotePersonalLoan(borrower, lender Actor) int {
	if borrower.ID == lender.ID || borrower.Dead || lender.Dead || !ActorHasTag(lender.IdentityTags(), "trader") {
		return 0
	}
	return maxInt(0, minInt(12, lender.Money-15))
}

func (mc *MarketCreditState) PendingLoanRequests(actorID string) []LoanRequest {
	var requests []LoanRequest
	if mc == nil {
		return requests
	}
	for _, r := range mc.LoanRequests {
		if r.Status == "pending" && (r.BorrowerID == actorID || r.LenderID == actorID) {
			requests = append(requests, r)
		}
	}
	return requests
}

func (w *World) applyPersonalCredit(mc *MarketCreditState, a *Actor, x Action, spec MarketCreditSpec) []Event {
	emit := func(kind string, data map[string]any, other *Actor) []Event {
		return []Event{w.record(a, x, kind, data, other)}
	}
	fail := func(text string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": text})}
	}
	b := w.actor(x.TargetID)
	if x.Kind == "cancel_loan" {
		for i := range mc.LoanRequests {
			r := &mc.LoanRequests[i]
			if r.Status == "pending" && r.BorrowerID == a.ID && r.LenderID == x.TargetID {
				r.Status = "cancelled"
				var observer *Actor
				if b != nil && !b.Dead && SamePlace(*a, *b) {
					observer = b
				}
				return emit("loan_cancelled", map[string]any{"text": fmt.Sprintf("%s withdrew loan request #%d", a.Name, r.ID), "request": *r}, observer)
			}
		}
		return fail("no pending request from this borrower to this lender")
	}
	if b == nil || !SamePlace(*a, *b) || a.ID == b.ID || w.Tick < b.JailedUntil {
		return fail("loan partner is unavailable")
	}
	if x.Kind == "borrow_loan" {
		for _, l := range mc.Loans {
			if l.BorrowerID == a.ID && l.Status == "active" {
				return fail("borrower already has an active loan")
			}
		}
		for _, r := range mc.LoanRequests {
			if r.BorrowerID == a.ID && r.Status == "pending" {
				return fail("borrower already has a pending request")
			}
		}
		amount := QuotePersonalLoan(*a, *b)
		if amount <= 0 {
			return fail("lender cannot fund a loan")
		}
		collateral := ""
		if h := w.house(a.HomeID); h != nil && h.OwnerID == a.ID {
			collateral = h.ID
		}
		r := LoanRequest{ID: mc.allocLoanID(), BorrowerID: a.ID, LenderID: b.ID, Principal: amount, RatePermille: spec.DailyLoanInterestPermille, TermTicks: spec.LoanTermTicks, CollateralHouse: collateral, RequestedTick: w.Tick, Status: "pending"}
		mc.LoanRequests = append(mc.LoanRequests, r)
		return emit("loan_requested", map[string]any{"text": fmt.Sprintf("%s requested %d coins from %s at %d‰/day for %d ticks, collateral %s", a.Name, r.Principal, b.Name, r.RatePermille, r.TermTicks, r.CollateralHouse), "request": r}, b)
	}
	for i := range mc.LoanRequests {
		r := &mc.LoanRequests[i]
		if r.Status != "pending" || r.LenderID != a.ID || r.BorrowerID != b.ID {
			continue
		}
		if x.Kind == "reject_loan" {
			r.Status = "rejected"
			return emit("loan_rejected", map[string]any{"text": fmt.Sprintf("%s rejected %s’s loan request", a.Name, b.Name), "request": *r}, b)
		}
		if a.Money < r.Principal {
			return fail("lender has insufficient coins for agreed terms")
		}
		for _, l := range mc.Loans {
			if l.BorrowerID == b.ID && l.Status == "active" {
				return fail("borrower already has an active loan")
			}
		}
		if r.CollateralHouse != "" {
			if h := w.house(r.CollateralHouse); h == nil || h.OwnerID != b.ID {
				return fail("requested collateral is no longer owned by borrower")
			}
		}
		a.Money -= r.Principal
		b.Money += r.Principal
		r.Status = "accepted"
		loan := LoanRecord{ID: r.ID, LenderID: a.ID, BorrowerID: b.ID, Principal: int64(r.Principal), RatePermille: r.RatePermille, IssuedTick: w.Tick, DueTick: w.Tick + int64(r.TermTicks), CollateralHouse: r.CollateralHouse, Status: "active"}
		mc.Loans = append(mc.Loans, loan)
		return emit("loan_issued", map[string]any{"text": fmt.Sprintf("%s accepted %s’s request and lent %d coins, due tick %d", a.Name, b.Name, r.Principal, loan.DueTick), "loan_id": loan.ID, "request": *r, "lender_id": a.ID, "borrower_id": b.ID, "principal": loan.Principal, "rate_permille": loan.RatePermille, "due_tick": loan.DueTick, "collateral_house": loan.CollateralHouse}, b)
	}
	return fail("no pending request from this borrower to this lender")
}

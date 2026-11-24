package sim

import "testing"

// X1: an offer is a proposal. Settlement needs the named buyer and physical
// co-location, and the same goods and money must leave one holder and reach another.
func TestLawExchangeRequiresNamedConsentAndLocalAtomicDelivery(t *testing.T) {
	for _, price := range []int{0, 7} {
		w := lawWorld()
		w.Actors[0].Items["wood"], w.Actors[0].Money, w.Actors[1].Money = 2, 1, 7
		lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 2, Price: price}))
		if len(w.Offers) != 1 || w.Actors[0].Items["wood"] != 2 || w.Actors[1].Items["wood"] != 0 || w.Actors[1].Money != 7 {
			t.Fatal("proposal transferred property without buyer consent")
		}
		offer := w.Offers[0].ID
		lawRejected(t, w.Apply("c", Action{Kind: "accept", OfferID: offer}))
		w.Actors[1].Location = "away"
		lawRejected(t, w.Apply("b", Action{Kind: "accept", OfferID: offer}))
		if w.Actors[0].Items["wood"] != 2 || w.Actors[1].Items["wood"] != 0 || w.Actors[0].Money != 1 || w.Actors[1].Money != 7 {
			t.Fatal("unauthorized or remote acceptance changed economic state")
		}
		w.Actors[1].Location = "home"
		lawOK(t, w.Apply("b", Action{Kind: "accept", OfferID: offer}))
		if w.Actors[0].Items["wood"] != 0 || w.Actors[1].Items["wood"] != 2 || w.Actors[0].Money != 1+price || w.Actors[1].Money != 7-price {
			t.Fatal("accepted transfer did not have matching debit/credit or rejected exact payment/free gift")
		}
		lawRejected(t, w.Apply("b", Action{Kind: "accept", OfferID: offer}))
		if w.Actors[1].Items["wood"] != 2 || w.Actors[0].Money+w.Actors[1].Money != 8 {
			t.Fatal("same acceptance delivered twice")
		}
	}
}

func TestLawExchangeShortageCannotChargeBuyer(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"], w.Actors[1].Money = 2, 7
	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 2, Price: 7}))
	w.Actors[0].Items["wood"] = 1 // another transaction has spent one unreserved unit
	lawRejected(t, w.Apply("b", Action{Kind: "accept", OfferID: w.Offers[0].ID}))
	if w.Actors[0].Items["wood"] != 1 || w.Actors[1].Items["wood"] != 0 || w.Actors[1].Money != 7 || w.Actors[0].Money != 0 {
		t.Fatal("failed delivery charged the buyer or delivered a partial quantity")
	}
}

func TestLawExchangeTaxHasOnePayerAndOneJurisdictionReceipt(t *testing.T) {
	w := lawWorld()
	w.Map.Districts[0].PolityID = "local"
	w.Polities = &PolityNetwork{Nodes: []PolityNode{{ID: "local", MarketTaxPermille: 100}, {ID: "other", Treasury: 11}}}
	w.Actors[0].Items["wood"], w.Actors[1].Money = 1, 20
	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 1, Price: 20}))
	lawOK(t, w.Apply("b", Action{Kind: "accept", OfferID: w.Offers[0].ID}))
	if w.Actors[1].Money != 0 || w.Actors[0].Money != 18 || w.Polities.Nodes[0].Treasury != 2 || w.Polities.Nodes[1].Treasury != 11 || w.PublicTreasury != 0 {
		t.Fatal("ten-percent fixture tax lacked a matching local treasury receipt, or was charged twice")
	}
	if w.Polities.Nodes[0].Ledger.TaxAssessedGross != 2 || w.Polities.Nodes[0].Ledger.TaxToTreasury != 2 {
		t.Fatal("tax report differs from actual treasury delivery")
	}
}

// X2: generating words cannot fulfill a contract. Fulfillment is an asset flow.
func TestLawExchangeAgreementWordsCannotReplaceDelivery(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"], w.Actors[1].Money = 3, 7
	lawOK(t, w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "wood"}))
	if len(w.Continuity.Agreements) != 1 {
		t.Fatal("proposal missing")
	}
	id := w.Continuity.Agreements[0].ID
	fulfill := Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: id}
	lawRejected(t, w.Apply("a", fulfill))
	lawRejected(t, w.Apply("c", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: id}))
	lawOK(t, w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: id}))
	lawOK(t, w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "The delivery is complete and you already paid."}))
	if w.Continuity.Agreements[0].Status != "active" || w.Actors[0].Items["wood"] != 3 || w.Actors[1].Items["wood"] != 0 || w.Actors[1].Money != 7 {
		t.Fatal("speech or proposal replaced physical delivery")
	}
	w.Actors[0].Items["wood"] = 2
	lawRejected(t, w.Apply("a", fulfill))
	if w.Actors[1].Money != 7 || w.Actors[0].Items["wood"] != 2 {
		t.Fatal("partial delivery charged buyer")
	}
	w.Actors[0].Items["wood"] = 3
	lawOK(t, w.Apply("a", fulfill))
	if w.Continuity.Agreements[0].Status != "fulfilled" || w.Actors[0].Items["wood"] != 0 || w.Actors[1].Items["wood"] != 3 || w.Actors[0].Money != 7 || w.Actors[1].Money != 0 {
		t.Fatal("fulfillment status did not match the actual agreed asset flow")
	}
	lawRejected(t, w.Apply("a", fulfill))
	if w.Actors[1].Items["wood"] != 3 || w.Actors[0].Money != 7 {
		t.Fatal("contract delivered twice")
	}
}

func TestLawDebtRequiresBothDecisionsAndRepaymentMatchesCash(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Money = 40
	w.MarketCredit = NewMarketCreditState()
	lawOK(t, w.Apply("b", Action{Kind: "borrow_loan", TargetID: "a"}))
	if len(w.MarketCredit.LoanRequests) != 1 || len(w.MarketCredit.Loans) != 0 || w.Actors[0].Money != 40 || w.Actors[1].Money != 0 {
		t.Fatal("one-sided request created money or a loan")
	}
	requested := w.MarketCredit.LoanRequests[0].Principal
	if requested <= 0 || requested > 40 {
		t.Fatal("fixture did not create a funded positive request")
	}
	lawRejected(t, w.Apply("b", Action{Kind: "accept_loan", TargetID: "a"}))
	lawOK(t, w.Apply("a", Action{Kind: "reject_loan", TargetID: "b"}))
	if len(w.MarketCredit.Loans) != 0 || w.Actors[0].Money != 40 || w.Actors[1].Money != 0 {
		t.Fatal("rejection moved principal")
	}
	lawOK(t, w.Apply("b", Action{Kind: "borrow_loan", TargetID: "a"}))
	request := w.MarketCredit.LoanRequests[1]
	lawOK(t, w.Apply("a", Action{Kind: "accept_loan", TargetID: "b"}))
	if len(w.MarketCredit.Loans) != 1 || w.Actors[0].Money != 40-request.Principal || w.Actors[1].Money != request.Principal || w.MarketCredit.Loans[0].Principal != int64(request.Principal) {
		t.Fatal("accepted principal differs from the same agreed terms or cash flow")
	}
	lawOK(t, w.Apply("b", Action{Kind: "repay_loan", TargetID: "a", OfferID: request.ID, Quantity: request.Principal}))
	if w.Actors[0].Money != 40 || w.Actors[1].Money != 0 || w.MarketCredit.Loans[0].Repaid != int64(request.Principal) || w.MarketCredit.Loans[0].Status != "repaid" {
		t.Fatal("debt cancellation did not match cash actually returned")
	}
	lawRejected(t, w.Apply("b", Action{Kind: "repay_loan", OfferID: request.ID}))
}

func TestDebtSettlementUsesDeclaredTenantShare(t *testing.T) {
	for _, share := range []int{980, 1000} {
		w := lawWorld()
		w.Cohorts = []PopulationCohort{
			{ID: "debtor", District: "home", Population: 10},
			{ID: "creditor", District: "home", Population: 10},
		}
		mc := NewMarketCreditState()
		mc.Spec.MaxTenantSharePermille = share
		mc.CohortEquityPermille["debtor"] = map[string]int{"creditor": 900}
		mc.CohortDebts = []CohortDebtRecord{{ID: 1, CreditorID: "creditor", DebtorID: "debtor", Principal: 20, Status: "active"}}
		w.StepMarketAndCredit(mc)
		if got := mc.TotalAnnexedSharePermille("debtor"); got != share || mc.CohortDebts[0].Status != "annexed" {
			t.Errorf("declared share %d‰ settled to %d‰, debt status %s", share, got, mc.CohortDebts[0].Status)
		}
	}
}

func TestLawDebtClockAndRepresentationCannotInventConsentOrCollateral(t *testing.T) {
	for _, pledged := range []bool{false, true} {
		w := lawWorld()
		w.Actors[0].Money = 40
		w.MarketCredit = NewMarketCreditState()
		if pledged {
			w.Actors[1].HomeID = "property"
			w.Houses = []House{{ID: "property", District: "home", OwnerID: "b", Level: 1, Condition: 80}}
		}
		lawOK(t, w.Apply("b", Action{Kind: "borrow_loan", TargetID: "a"}))
		lawOK(t, w.Apply("a", Action{Kind: "accept_loan", TargetID: "b"}))
		loan := w.MarketCredit.Loans[0]
		if !pledged {
			w.Houses = []House{{ID: "property", District: "home", OwnerID: "b", Level: 1, Condition: 80}}
		}
		if err := w.DehydrateActor("b"); err != nil {
			t.Fatal(err)
		}
		w.Tick = loan.DueTick + 1
		w.StepMarketAndCredit(w.MarketCredit)
		if w.Houses[0].OwnerID != "b" || w.MarketCredit.Loans[0].Status != "active" || w.actor("b").Money != int(loan.Principal) {
			t.Fatal("time or storage conversion repaid, foreclosed, or killed a living borrower")
		}
		events := w.Apply("a", Action{Kind: "foreclose_collateral", TargetID: "b", OfferID: loan.ID, HouseID: "property"})
		if pledged {
			lawOK(t, events)
			if w.Houses[0].OwnerID != "a" || w.MarketCredit.Loans[0].Status != "foreclosed" {
				t.Fatal("explicit creditor action could not settle the agreed overdue collateral")
			}
		} else {
			lawRejected(t, events)
			if w.Houses[0].OwnerID != "b" || w.MarketCredit.Loans[0].Status != "active" {
				t.Fatal("an unsecured debt seized property acquired after agreement")
			}
		}
	}
}

package agent

import (
	"fmt"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func parcelWorld() *sim.World {
	w := boundaryWorld()
	w.Catalog.Items["parcel"] = sim.ItemSpec{ID: "parcel", Tradeable: true, BatchQty: 3, BatchPrice: 7, OfferThreshold: 3}
	w.Actors[0].Items = map[string]int{"parcel": 3}
	return w
}
func assertParcelAssets(t *testing.T, w *sim.World, delivered bool) {
	t.Helper()
	sellerGoods, buyerGoods, sellerMoney, buyerMoney := 3, 0, 50, 50
	if delivered {
		sellerGoods, buyerGoods, sellerMoney, buyerMoney = 0, 3, 57, 43
	}
	if w.Actors[0].Items["parcel"] != sellerGoods || w.Actors[1].Items["parcel"] != buyerGoods || w.Actors[0].Money != sellerMoney || w.Actors[1].Money != buyerMoney || w.Actors[2].Money != 50 || w.Actors[2].Items["parcel"] != 0 {
		t.Fatal("goods and payment did not reach the exact contractual owners")
	}
}

func TestTradeRequiresNamedBuyerThenDeliversExactGoodsAndPayment(t *testing.T) {
	w := parcelWorld()
	boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "offer", "bob"))
	assertParcelAssets(t, w, false)
	if len(w.Offers) != 1 {
		t.Fatal("offer was not recorded")
	}
	offer := w.Offers[0]
	if offer.Quantity != 3 || offer.Price != 7 || offer.Item != "parcel" || offer.Buyer != "bob" {
		t.Fatal("catalog terms changed during proposal")
	}
	boundaryRejected(t, w, "charlie", sim.Action{Kind: "accept", TargetID: "alice", OfferID: offer.ID})
	assertParcelAssets(t, w, false)
	boundaryApply(t, w, "bob", boundaryOption(t, w, "bob", "accept", "alice"))
	assertParcelAssets(t, w, true)
	boundaryRejected(t, w, "bob", sim.Action{Kind: "accept", TargetID: "alice", OfferID: offer.ID})
	assertParcelAssets(t, w, true)
}

func TestTradeExpiryHasAnInclusiveLastValidTick(t *testing.T) {
	for _, afterDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint(afterDeadline), func(t *testing.T) {
			w := parcelWorld()
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "offer", "bob"))
			offer := w.Offers[0]
			w.Tick = offer.Expires
			if afterDeadline {
				w.Tick++
			}
			if hasBoundaryKind(w, "bob", "accept") == afterDeadline {
				t.Fatal("candidate expiry differs from the offer's explicit deadline")
			}
			action := sim.Action{Kind: "accept", TargetID: "alice", OfferID: offer.ID}
			if afterDeadline {
				boundaryRejected(t, w, "bob", action)
			} else {
				boundaryApply(t, w, "bob", action)
			}
			assertParcelAssets(t, w, !afterDeadline)
		})
	}
}

func TestLoanProposalCannotDebitTheLenderBeforeTheirAssent(t *testing.T) {
	for _, response := range []string{"accept_loan", "reject_loan"} {
		t.Run(response, func(t *testing.T) {
			w := boundaryWorld()
			w.Actors[0].Money = 0
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "borrow_loan", "bob"))
			if w.Actors[0].Money != 0 || w.Actors[1].Money != 50 || len(w.MarketCredit.Loans) != 0 {
				t.Fatal("borrower unilaterally acquired the lender's coins")
			}
			request := w.MarketCredit.LoanRequests[0]
			boundaryRejected(t, w, "charlie", sim.Action{Kind: "accept_loan", TargetID: "alice"})
			if w.Actors[0].Money != 0 || w.Actors[1].Money != 50 || w.Actors[2].Money != 50 {
				t.Fatal("an unrelated person approved someone else's loan")
			}
			boundaryApply(t, w, "bob", boundaryOption(t, w, "bob", response, "alice"))
			if response == "accept_loan" {
				if w.Actors[0].Money != request.Principal || w.Actors[1].Money != 50-request.Principal || len(w.MarketCredit.Loans) != 1 {
					t.Fatal("accepted loan did not deliver the agreed principal between its parties")
				}
				loan := w.MarketCredit.Loans[0]
				if loan.BorrowerID != "alice" || loan.LenderID != "bob" || loan.Principal != int64(request.Principal) || loan.RatePermille != request.RatePermille || loan.DueTick != w.Tick+int64(request.TermTicks) {
					t.Fatal("loan debt differs from the terms that received assent")
				}
			} else if w.Actors[0].Money != 0 || w.Actors[1].Money != 50 || len(w.MarketCredit.Loans) != 0 {
				t.Fatal("rejected loan transferred coins or created debt")
			}
		})
	}
}

func TestLoanAssentCandidateRequiresTheLendersActualAgreedPrincipal(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint(short), func(t *testing.T) {
			w := boundaryWorld()
			w.Actors[0].Money = 0
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "borrow_loan", "bob"))
			principal := w.MarketCredit.LoanRequests[0].Principal
			w.Actors[1].Money = principal
			if short {
				w.Actors[1].Money--
			}
			if hasBoundaryKind(w, "bob", "accept_loan") == short {
				t.Error("lender's known funding and acceptance candidate disagree")
			}
			if !hasBoundaryKind(w, "bob", "reject_loan") {
				t.Fatal("unfunded lender cannot decline the proposal")
			}
			action := sim.Action{Kind: "accept_loan", TargetID: "alice"}
			if short {
				boundaryRejected(t, w, "bob", action)
				if w.Actors[0].Money != 0 || w.Actors[1].Money != principal-1 {
					t.Fatal("failed assent debited an unfunded lender")
				}
			} else {
				boundaryApply(t, w, "bob", action)
				if w.Actors[0].Money != principal || w.Actors[1].Money != 0 {
					t.Fatal("exactly funded, already quoted principal could not be delivered")
				}
			}
		})
	}
}

func TestDeliveryAgreementNeedsAssentAndPhysicalDelivery(t *testing.T) {
	w := parcelWorld()
	boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "propose_agreement", "bob"))
	assertParcelAssets(t, w, false)
	proposal := w.Continuity.Agreements[0]
	boundaryRejected(t, w, "alice", sim.Action{Kind: "fulfill_agreement", TargetID: "bob", AgreementID: proposal.ID})
	assertParcelAssets(t, w, false)
	boundaryApply(t, w, "bob", boundaryOption(t, w, "bob", "accept_agreement", "alice"))
	assertParcelAssets(t, w, false)
	w.Actors[1].Location = "remote"
	if hasBoundaryKind(w, "alice", "fulfill_agreement") {
		t.Fatal("delivery candidate crosses space without transport")
	}
	boundaryRejected(t, w, "alice", sim.Action{Kind: "fulfill_agreement", TargetID: "bob", AgreementID: proposal.ID})
	assertParcelAssets(t, w, false)
	w.Actors[1].Location = "meeting"
	boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "fulfill_agreement", "bob"))
	assertParcelAssets(t, w, true)
	if w.Continuity.Agreements[0].Status != "fulfilled" {
		t.Fatal("completed delivery remained open")
	}
}

func TestMediationNeedsBothAffectedPeopleAndPreservesProposedAmount(t *testing.T) {
	w := boundaryWorld()
	w.Relations = sim.NewRelationEvoState()
	w.Actors[2].Memories = []sim.Memory{{EventID: 9}}
	w.Relations.Grudges = []sim.GrudgeRecord{{ID: "dispute", OffenderID: "alice", AggrievedID: "bob", Intensity: 50, SourceEventID: 9, RootEventID: 9}}
	proposal := boundaryOption(t, w, "charlie", "mediate_feud", "alice")
	boundaryApply(t, w, "charlie", proposal)
	if w.Actors[0].Money != 50 || w.Actors[1].Money != 50 || w.Relations.Grudges[0].Resolved {
		t.Fatal("mediator spent a party's money without assent")
	}
	w.Relations.Grudges[0].Intensity = 90
	firstAssent := boundaryOption(t, w, "alice", "mediate_feud", "alice")
	if firstAssent.Price != proposal.Price {
		t.Fatal("proposal terms changed before assent")
	}
	boundaryApply(t, w, "alice", firstAssent)
	if w.Actors[0].Money != 50 || w.Actors[1].Money != 50 || w.Relations.Grudges[0].Resolved {
		t.Fatal("one party's assent settled a bilateral dispute")
	}
	secondAssent := boundaryOption(t, w, "bob", "mediate_feud", "alice")
	if secondAssent.Price != proposal.Price {
		t.Fatal("second party received different terms")
	}
	boundaryApply(t, w, "bob", secondAssent)
	if w.Actors[0].Money != 50-proposal.Price || w.Actors[1].Money != 50+proposal.Price || w.Actors[2].Money != 50 || !w.Relations.Grudges[0].Resolved {
		t.Fatal("settlement did not pay the aggrieved person from the assenting offender")
	}
}

func TestGrowthCareAssentGrantsOnlyTheAgreedLocalHarvestShare(t *testing.T) {
	for _, share := range []int64{0, 500, 1000} {
		t.Run(fmt.Sprint(share), func(t *testing.T) {
			w := boundaryWorld()
			for i := range w.Actors {
				w.Actors[i].Items = map[string]int{}
			}
			w.Growth = []sim.GrowthBatch{{ID: 1, OwnerID: "alice", District: "meeting", Item: "food", Quantity: 10, Batches: 1}}
			var proposal sim.Action
			for _, o := range Options(w.View("alice")) {
				if o.Action.Kind == "propose_growth_care" && o.Action.TargetID == "bob" && o.Action.Care.SharePermille == share {
					proposal = o.Action
					break
				}
			}
			if proposal.Care == nil {
				t.Fatalf("missing actual model proposal with harvest share %d", share)
			}
			proposal.Care.CaretakerAccepted = true
			boundaryApply(t, w, "alice", proposal)
			if len(w.Production.CareAgreements) != 0 || w.Production.CareProposals[0].CaretakerAccepted {
				t.Fatal("proposer forged another person's assent")
			}
			assent := boundaryOption(t, w, "bob", "accept_growth_care", "alice")
			forged := assent
			terms := *assent.Care
			terms.SharePermille = (share + 1) % 1001
			forged.Care = &terms
			boundaryRejected(t, w, "bob", forged)
			if len(w.Production.CareAgreements) != 0 {
				t.Fatal("changed terms received assent to the original proposal")
			}
			boundaryApply(t, w, "bob", assent)
			w.Growth[0].Ready = true // The fixture supplies mature goods; no agronomy is asserted here.
			w.Actors[0].Location = "remote"
			if hasBoundaryKind(w, "charlie", "harvest") {
				t.Fatal("an unrelated resident received harvest rights")
			}
			boundaryRejected(t, w, "charlie", sim.Action{Kind: "harvest", TargetID: "1"})
			boundaryApply(t, w, "bob", boundaryOption(t, w, "bob", "harvest", "1"))
			if w.Actors[1].Items["food"] != int(10*share/1000) || w.Actors[0].Items["food"] != 0 {
				t.Fatal("caretaker took the owner's share or sent goods to an absent owner")
			}
			w.Actors[0].Location = "meeting"
			w.Tick++
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "harvest", "1"))
			if w.Actors[0].Items["food"] != int(10*(1000-share)/1000) || w.Actors[2].Items["food"] != 0 || len(w.Growth) != 0 {
				t.Fatal("owner could not collect the retained local share exactly once")
			}
		})
	}
}

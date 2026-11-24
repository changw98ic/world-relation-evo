package sim

import "testing"

func TestNamedOfferCannotBeSettledByAnUnrelatedCohort(t *testing.T) {
	for _, counter := range []bool{false, true} {
		w := lawWorld()
		w.Map.Districts[0].Features = append(w.Map.Districts[0].Features, "market")
		w.Actors[0].Items["wood"] = 3
		w.Actors[1].Money = 20
		w.Catalog.TagProfiles = map[string]TagProfile{"resident": {Consumes: []string{"wood"}}}
		w.Cohorts = []PopulationCohort{{ID: "cohort", District: "home", Profession: "customer", Population: 10, Adults: 10, MoneyPool: 100, ItemPool: map[string]int64{}}}
		lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 3, Price: 7}))
		if counter {
			lawOK(t, w.Apply("b", Action{Kind: "offer", TargetID: "a", OfferID: w.Offers[0].ID, Item: "wood", Quantity: 2, Price: 5}))
		}
		before := w.Offers[0]
		w.clearRegionalCohortMarket()
		if len(w.Offers) != 1 || w.Offers[0] != before || w.Cohorts[0].ItemPool["wood"] != 0 || w.Actors[0].Items["wood"] != 3 {
			t.Fatal("unrelated cohort hijacked a named proposal")
		}
	}
}

func TestDialogueRemainsOptionalAcrossTurnsAndRespondsToTrade(t *testing.T) {
	w := lawWorld()
	lawOK(t, w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "I have completed the delivery"}))
	view := w.View("b")
	found := false
	for _, v := range view.Interactions {
		if v.Kind == "dialogue" && v.AwaitingActorID == "b" && v.ReplyOptional {
			found = true
		}
	}
	if !found {
		t.Fatal("latest speech not exposed as optional reply")
	}
	lawOK(t, w.Apply("b", Action{Kind: "wait"}))
	if len(w.View("b").Interactions) == 0 {
		t.Fatal("deferring erased dialogue")
	}
	w = lawRestore(t, w)
	if len(w.View("b").Interactions) == 0 {
		t.Fatal("snapshot erased dialogue")
	}
	w.Actors[0].Items["wood"] = 3
	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 3, Price: 7}))
	for _, v := range w.View("b").Interactions {
		if v.Kind == "dialogue" {
			t.Fatal("superseded speech remained a separate pending reply")
		}
	}
}

func TestOfferSettlementRepresentationAndExactTradeSummary(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name                           string
		sellerMoney, buyerStock, price int
		reject                         bool
	}{
		{"money_overflow", maxInt - 1, 0, 2, true},
		{"money_exact", maxInt - 2, 0, 2, false},
		{"stock_overflow", 0, maxInt, 2, true},
		{"stock_exact", 0, maxInt - 1, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			w.Actors[0].Items["wood"] = 1
			w.Actors[0].Money = tc.sellerMoney
			w.Actors[1].Money = tc.price
			w.Actors[1].Items["wood"] = tc.buyerStock
			lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 1, Price: tc.price}))
			offer := w.Offers[0]
			events := w.Apply("b", Action{Kind: "accept", OfferID: offer.ID})
			if tc.reject {
				lawRejected(t, events)
				if w.Actors[0].Money != tc.sellerMoney || w.Actors[1].Money != tc.price || w.Actors[0].Items["wood"] != 1 || w.Actors[1].Items["wood"] != tc.buyerStock {
					t.Fatal("overflow failure partially settled")
				}
			} else {
				lawOK(t, events)
				if w.Actors[0].Money != tc.sellerMoney+tc.price || w.Actors[1].Items["wood"] != tc.buyerStock+1 {
					t.Fatal("exact representable balance rejected")
				}
				summary := NewScaleSummary()
				summary.RecordEvents(events)
				if summary.ItemsTraded != 1 || summary.CoinsTraded != int64(tc.price) {
					t.Fatal("summary lost settled terms")
				}
			}
		})
	}
	w := lawWorld()
	w.Actors[0].Items["wood"] = 3
	w.Actors[1].Money = 9
	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 3, Price: 7}))
	lawOK(t, w.Apply("b", Action{Kind: "offer", OfferID: w.Offers[0].ID, TargetID: "a", Item: "wood", Quantity: 2, Price: 4}))
	events := w.Apply("a", Action{Kind: "accept", OfferID: w.Offers[0].ID})
	lawOK(t, events)
	summary := NewScaleSummary()
	summary.RecordEvents(events)
	if summary.ItemsTraded != 2 || summary.CoinsTraded != 4 || summary.TradeBatches != 1 {
		t.Fatalf("counteroffer summary is not actual settlement: %+v", summary)
	}
}

func TestBuyerCounterofferCannotBypassPurchasePermission(t *testing.T) {
	w := lawWorld()
	w.Catalog.TagActions["sell_only"] = []string{"offer", "wait"}
	w.Actors[0].Items["wood"] = 3
	lawOK(t, w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "wood", Quantity: 3, Price: 7}))
	w.Actors[1].Tags = []string{"sell_only"}
	w.Actors[1].Professions = nil
	lawRejected(t, w.Apply("b", Action{Kind: "offer", OfferID: w.Offers[0].ID, TargetID: "a", Item: "wood", Quantity: 2, Price: 1}))
}

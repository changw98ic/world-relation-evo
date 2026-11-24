package sim

import "testing"

func TestCashReliefPaysEachRepresentationOnceWithinTheActualBudget(t *testing.T) {
	for _, budget := range []int64{1, 9} {
		w := lawWorld()
		w.Map.Districts[0].PolityID = "county"
		w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "issue_decree")
		w.Polities = &PolityNetwork{Nodes: []PolityNode{{ID: "county", LeaderActorID: "a", SeatDistrictID: "home", Treasury: budget}}}
		w.Actors = w.Actors[:2]
		member := lawPerson("stored")
		w.Cohorts = []PopulationCohort{{ID: "poor", District: "home", Population: 3, Adults: 3, Members: []Actor{member}}, {ID: "rich", District: "home", Population: 2, Adults: 2, MoneyPool: 100}}
		before := w.TotalCoins()
		events := w.Apply("a", Action{Kind: "issue_decree", Item: "relief_grant", Quantity: int(budget)})
		lawOK(t, events)
		paid := int64(w.Actors[1].Money+w.Cohorts[0].Members[0].Money) + w.Cohorts[0].MoneyPool
		if paid != budget || w.Polities.Nodes[0].Treasury != 0 || w.Cohorts[1].MoneyPool != 100 || w.TotalCoins() != before {
			t.Fatalf("budget=%d: cash relief was duplicated, lost, or paid to an ineligible household", budget)
		}
		wantRecipients := min(int(budget), 4) // one actor, one stored member, two anonymous residents
		if events[0].Payload["relief_recipients"] != wantRecipients {
			t.Fatalf("recipient count includes unpaid people: %+v", events[0])
		}
	}
}

func TestCashReliefZeroRequestCannotSpendAnImplicitTwentyCoins(t *testing.T) {
	w := lawWorld()
	w.Map.Districts[0].PolityID = "county"
	w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "issue_decree")
	w.Polities = &PolityNetwork{Nodes: []PolityNode{{ID: "county", LeaderActorID: "a", Treasury: 20}}}
	for _, amount := range []int{0, -1} {
		lawRejected(t, w.Apply("a", Action{Kind: "issue_decree", Item: "relief_grant", Quantity: amount}))
		if w.Polities.Nodes[0].Treasury != 20 || w.Actors[1].Money != 0 || w.Actors[2].Money != 0 {
			t.Fatal("a nonpositive request secretly spent treasury money")
		}
	}
}

func TestFoodReliefProcurementPaysOnlyForAnAffordableSurplusLot(t *testing.T) {
	for _, price := range []int{1, 10} {
		for _, funded := range []bool{false, true} {
			w := lawWorld()
			w.Actors = w.Actors[:1]
			w.Actors[0].Hunger = 3
			w.PublicTreasury = int64(price)
			if !funded {
				w.PublicTreasury--
			}
			w.Catalog.Items["grain"] = ItemSpec{ID: "grain", Edible: true, NutritionUnits: 1, MassUnits: 1, Tradeable: true, BatchQty: 20, BatchPrice: price}
			// Four portions feed this seller for the next day. Exactly twenty
			// additional portions are available for the declared trade lot.
			w.Cohorts = []PopulationCohort{{ID: "seller", District: "home", Population: 1, Adults: 1, AvgEnergy: 21, ItemPool: map[string]int64{"grain": 24}}}
			beforeTreasury := w.PublicTreasury
			w.settleFocalRelief()
			want := 0
			if funded {
				want = 20
			}
			if w.Actors[0].Items["grain"] != want || w.Cohorts[0].ItemPool["grain"] != int64(24-want) || funded && (w.PublicTreasury != 0 || w.Cohorts[0].MoneyPool != int64(price)) || !funded && (w.PublicTreasury != beforeTreasury || w.Cohorts[0].MoneyPool != 0) {
				t.Fatalf("price=%d funded=%v: procurement did not match actual delivery and preserved subsistence", price, funded)
			}
			wantEnergy := 21
			if funded {
				wantEnergy = 0
			}
			if w.Cohorts[0].AvgEnergy != wantEnergy {
				t.Fatalf("price=%d funded=%v: only actual delivery should consume the supplier's energy", price, funded)
			}
		}
	}
}

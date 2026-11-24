package sim

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSnapshotRestoresPrivateStockToItsOwner(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Items["wood"] = math.MaxInt - 2
	w.Cohorts = []PopulationCohort{{ID: "merchant", District: "home", Population: 1, Adults: 1}}
	w.MarketCredit = NewMarketCreditState()
	w.MarketCredit.HoardedStock[a.ID] = map[string]int64{"wood": 2}
	w.MarketCredit.HoardedStock["merchant"] = map[string]int64{"grain": 1}
	w.InitLedgerAudit()
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeWorldSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Actors[0].Items["wood"] != math.MaxInt || restored.MarketCredit.HoardedStock[a.ID] != nil ||
		restored.MarketCredit.HoardedStock["merchant"]["grain"] != 1 {
		t.Fatal("personal or cohort holdings changed ownership")
	}
	if err := restored.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPostedMarketLotSettlesOnceAtTheExactCashBound(t *testing.T) {
	w := lawWorld()
	a, b := &w.Actors[0], &w.Actors[1]
	a.Location, b.Location = "home", "home"
	a.Money, b.Money = 1, 0
	w.Catalog.Items["grain"] = ItemSpec{ID: "grain", Edible: true, NutritionUnits: 1, Tradeable: true, BatchQty: 2, BatchPrice: 1}
	w.Cohorts = []PopulationCohort{{ID: "seller", District: "home", Population: 1, Adults: 1, AvgEnergy: 4,
		MoneyPool: math.MaxInt64 - 1, ItemPool: map[string]int64{"grain": 5}}}
	lot := Action{Kind: "market_buy", TargetID: "seller", Location: "home", Item: "grain", Quantity: 2, Price: 1}
	w.InitLedgerAudit()
	lawOK(t, w.Apply(a.ID, lot))
	if a.Items["grain"] != 2 || a.Money != 0 || w.Cohorts[0].MoneyPool != math.MaxInt64 || w.Cohorts[0].ItemPool["grain"] != 3 {
		t.Fatalf("the exact legal budget or public reserve was lost: %+v, %+v", a, w.Cohorts[0])
	}
	w.Cohorts[0].MoneyPool--
	b.Money++
	if events := w.Apply(b.ID, lot); len(events) != 1 || events[0].Kind != "action_failed" {
		t.Fatalf("the already sold lot settled again: %+v", events)
	}
	if b.Money != 1 || b.Items["grain"] != 0 || w.Cohorts[0].ItemPool["grain"] != 3 {
		t.Fatal("the unavailable lot charged money or sold protected stock")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestRationAccessAtLowHealthRequiresPresentBuyerSurplusAndPayment(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		wallet, treasury, stock int
		buyerDistrict, task     string
		want                    int
	}{
		{"low_health_paid", 1, 0, 6, "home", "", 2}, {"low_health_relief", 0, 1, 6, "home", "", 2}, {"unfunded", 0, 0, 6, "home", "", 0}, {"protected_stock", 1, 1, 4, "home", "", 0}, {"other_district", 1, 1, 6, "away", "", 0}, {"on_road", 1, 1, 6, "home", "move", 0}, {"between_venues", 0, 1, 6, "home", "move_venue", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			w.Actors = w.Actors[:1]
			a := &w.Actors[0]
			a.Health = 22
			a.Hunger = 6
			a.Location = tc.buyerDistrict
			a.Money = tc.wallet
			if tc.task != "" {
				a.Task = &Task{Action: Action{Kind: tc.task}, Remaining: 1}
			}
			w.PublicTreasury = int64(tc.treasury)
			w.Catalog.Items["grain"] = ItemSpec{ID: "grain", Edible: true, NutritionUnits: 1, Tradeable: true, BatchQty: 2, BatchPrice: 1}
			w.Cohorts = []PopulationCohort{{ID: "seller", District: "home", Population: 1, Adults: 1, AvgEnergy: 2, ItemPool: map[string]int64{"grain": int64(tc.stock)}}}
			w.InitLedgerAudit()
			if tc.wallet > 0 {
				w.Apply(a.ID, Action{Kind: "market_buy", TargetID: "seller", Location: "home", Item: "grain", Quantity: 2, Price: 1})
			}
			w.settleFocalRelief()
			cost := tc.want / 2
			if a.Items["grain"] != tc.want || w.Cohorts[0].ItemPool["grain"] != int64(tc.stock-tc.want) || w.Cohorts[0].MoneyPool != int64(cost) || a.Money != tc.wallet-min(tc.wallet, cost) || w.PublicTreasury != int64(tc.treasury-(cost-min(tc.wallet, cost))) || a.Hunger != 6 {
				t.Fatalf("delivery, payment or hunger wrong: actor %+v seller %+v treasury %d", a, w.Cohorts[0], w.PublicTreasury)
			}
			if e := w.VerifyLedgerConservation(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestFoodSaleRulesAlsoApplyWhenTreasuryPays(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*World, *ItemSpec)
		want      int
	}{
		{"free_to_trade", func(w *World, i *ItemSpec) {}, 2},
		{"not_tradeable", func(w *World, i *ItemSpec) { i.Tradeable = false }, 0},
		{"coin_not_accepted", func(w *World, i *ItemSpec) { i.AcceptedCurrencies = []string{"scrip"} }, 0},
		{"restricted_buyer", func(w *World, i *ItemSpec) { i.BuyerRequiredTag = "guild_member" }, 0},
		{"authorized_buyer", func(w *World, i *ItemSpec) { i.BuyerRequiredTag = "person" }, 2},
		{"embargo", func(w *World, i *ItemSpec) {
			w.Map.Districts[0].PolityID = "county"
			w.Polities = &PolityNetwork{Nodes: []PolityNode{{ID: "county", EmbargoItems: []string{"grain"}}}}
		}, 0},
	} {
		for _, public := range []bool{false, true} {
			name := tc.name + "/private"
			if public {
				name = tc.name + "/public"
			}
			t.Run(name, func(t *testing.T) {
				w := lawWorld()
				w.Actors = w.Actors[:1]
				a := &w.Actors[0]
				a.Hunger = 4
				if public {
					w.PublicTreasury = 1
				} else {
					a.Money = 1
				}
				item := ItemSpec{ID: "grain", Edible: true, Tradeable: true, BatchQty: 2, BatchPrice: 1, NutritionUnits: 1}
				tc.configure(w, &item)
				w.Catalog.Items["grain"] = item
				w.Cohorts = []PopulationCohort{{ID: "supplier", District: "home", Population: 1, Adults: 1, AvgEnergy: 2, ItemPool: map[string]int64{"grain": 6}}}
				w.InitLedgerAudit()
				var events []Event
				if public {
					events = w.settleFocalRelief()
				} else {
					events = w.Apply(a.ID, Action{Kind: "market_buy", TargetID: "supplier", Location: "home", Item: "grain", Quantity: 2, Price: 1})
				}
				if a.Items["grain"] != tc.want || w.Cohorts[0].ItemPool["grain"] != int64(6-tc.want) || w.Cohorts[0].MoneyPool != int64(tc.want/2) || int64(a.Money)+w.PublicTreasury != int64(1-tc.want/2) {
					t.Fatalf("unsettled or unauthorized procurement: %+v", events)
				}
				if e := w.VerifyLedgerConservation(); e != nil {
					t.Fatal(e)
				}
			})
		}
	}
}

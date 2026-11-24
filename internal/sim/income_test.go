package sim

import "testing"

func TestWagesStartWithCompletedWorkAndPayOnTheAgreedDate(t *testing.T) {
	for _, tc := range []struct {
		cycle PayCycle
		due   int64
	}{{PayDaily, 56}, {PayWeekly, 632}, {PayMonthly, 2840}} {
		t.Run(string(tc.cycle), func(t *testing.T) {
			w := lawWorld()
			w.PublicTreasury = 12
			w.Catalog.TagProfiles = map[string]TagProfile{"worker": {PublicTrustDuty: true}}
			w.Catalog.WorkOrders = []WorkOrder{{ID: "boards", Name: "Prepare boards", EmployerID: "treasury",
				ActionKind: "craft", CompletionKind: "crafted", RecipeID: "board", Wage: 4, PayCycle: tc.cycle}}
			alternate := w.Catalog.Recipes[0]
			alternate.ID = "alternate_board"
			w.Catalog.Recipes = append(w.Catalog.Recipes, alternate)
			w.Actors[0].Items["wood"] = 3
			w.InitLedgerAudit()
			job := Action{Kind: "craft", RecipeID: "board", WorkOrderID: "boards"}
			lawRejected(t, w.Apply("b", job))
			lawRejected(t, w.Apply("a", Action{Kind: "craft", RecipeID: alternate.ID, WorkOrderID: job.WorkOrderID}))
			lawOK(t, w.Apply("a", job))
			if len(w.EarnedWages) != 0 || w.Actors[0].Money != 0 {
				t.Fatal("incomplete or rejected work earned wages")
			}
			w.Tick = 1
			lawOK(t, w.Advance("a"))
			w.Tick = 2
			events := w.Advance("a")
			lawOK(t, events)
			if len(w.EarnedWages) != 1 || w.EarnedWages[0].DueTick != tc.due ||
				w.EarnedWages[0].WorkEventID != events[0].ID || w.Actors[0].Items["board"] != 1 {
				t.Fatalf("completed work did not create a claim: %+v, %+v", events, w.EarnedWages)
			}
			restored, err := DecodeWorldSnapshot(lawJSON(t, w))
			if err != nil {
				t.Fatal(err)
			}
			restored.Tick = tc.due - 1
			restored.stepActorPhysiologyAndLifecycle()
			if restored.Actors[0].Money != 0 || len(restored.EarnedWages) != 1 {
				t.Fatal("funds changed before the contractual payday")
			}
			restored.Tick = tc.due
			restored.stepActorPhysiologyAndLifecycle()
			restored.stepActorPhysiologyAndLifecycle()
			if restored.Actors[0].Money != 4 || restored.Actors[1].Money != 0 ||
				restored.PublicTreasury != 8 || len(restored.EarnedWages) != 0 {
				t.Fatalf("the payroll did not match the completed work: %+v", restored.EarnedWages)
			}
			if err := restored.VerifyLedgerConservation(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnpaidWageRetainsItsOriginalWorkReceipt(t *testing.T) {
	w := lawWorld()
	w.PublicTreasury = 2
	w.Actors[1].Money = 2
	w.EarnedWages = []EarnedWage{{OrderID: "boards", WorkerID: "a", EmployerID: "treasury",
		Coins: 4, PayCycle: PayWeekly, DueTick: 632, WorkEventID: 1}}
	w.NextID = 2
	w.InitLedgerAudit()
	w.Tick = 632
	events := w.settleWages()
	if len(events) != 2 || events[0].Kind != "wage_paid" || events[1].Kind != "wage_unpaid" ||
		len(w.EarnedWages) != 1 || w.EarnedWages[0].Coins != 2 || w.EarnedWages[0].WorkEventID != 1 ||
		w.Actors[0].Money != 2 || w.PublicTreasury != 0 {
		t.Fatalf("missing or incorrect arrears: %+v, %+v", events, w.EarnedWages)
	}
	w.adjustActorCurrency(&w.Actors[1], "coins", -2, false)
	w.creditTreasury(2, "fee")
	w.Tick += 7 * TicksPerDay
	w.settleWages()
	if w.Actors[0].Money != 4 || len(w.EarnedWages) != 0 {
		t.Fatal("the unpaid part of the agreed wage was lost")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestWageDebitsTheEmployersFundsAndProtectsPrivateHoldings(t *testing.T) {
	w := lawWorld()
	member := lawPerson("private member")
	member.Money = 7
	w.Cohorts = []PopulationCohort{{ID: "hall", District: "home", Population: 2, Adults: 2,
		MoneyPool: 11, Members: []Actor{member}}}
	w.EarnedWages = []EarnedWage{{OrderID: "lesson", WorkerID: "a", EmployerID: "hall", Coins: 5,
		PayCycle: PayWeekly, DueTick: 632, WorkEventID: 1}}
	w.NextID, w.Tick = 2, 632
	w.InitLedgerAudit()
	events := w.settleWages()
	c := w.Cohorts[0]
	if len(events) != 2 || events[0].Payload["payer_id"] != "hall" || w.Actors[0].Money != 4 ||
		c.MoneyPool != 7 || c.Members[0].Money != 7 || len(w.EarnedWages) != 1 || w.EarnedWages[0].Coins != 1 {
		t.Fatalf("the wage did not come from the hall's own cash: %+v", events)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestDividendsDistributeSurplusToARegisteredOwner(t *testing.T) {
	w := lawWorld()
	item := w.Catalog.Items["grain"]
	item.Tradeable, item.BatchQty, item.BatchPrice = true, 2, 10
	w.Catalog.Items["grain"] = item
	w.Actors[1].Money = 10
	w.Cohorts = []PopulationCohort{{ID: "shop", District: "home", Population: 1, Adults: 1, AvgEnergy: 3,
		MoneyPool: 20, ItemPool: map[string]int64{"grain": 9}}}
	err := w.RegisterDividends([]DividendAccount{{BusinessID: "shop", PayCycle: PayWeekly,
		Capital: 20, Ownership: map[string]int{"a": 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	w.InitLedgerAudit()
	w.Tick = 632
	if events := w.settleDividends(); len(events) != 0 {
		t.Fatal("the employer's existing capital was treated as a dividend")
	}
	w.Tick++
	lawOK(t, w.Apply("b", Action{Kind: "market_buy", TargetID: "shop", Location: "home", Item: "grain", Quantity: 2, Price: 10}))
	w.Tick = 632 + 7*TicksPerDay
	events := w.settleDividends()
	if len(events) != 1 || events[0].Kind != "dividend_paid" || w.Actors[0].Money != 10 || w.Cohorts[0].MoneyPool != 20 {
		t.Fatalf("the owned share of the business surplus was not credited: %+v", events)
	}
	w.Tick += 7 * TicksPerDay
	if events := w.settleDividends(); len(events) != 0 {
		t.Fatal("the same surplus was paid again")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

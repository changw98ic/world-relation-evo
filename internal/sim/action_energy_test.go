package sim

import "testing"

func TestMarketActionNeedsAndSpendsEnergy(t *testing.T) {
	w := lawWorld()
	grain := w.Catalog.Items["grain"]
	grain.Tradeable, grain.BatchQty, grain.BatchPrice = true, 2, 10
	w.Catalog.Items["grain"] = grain
	w.Cohorts = []PopulationCohort{{ID: "shop", District: "home", Population: 1, Adults: 1,
		AvgEnergy: 3, MoneyPool: 20, ItemPool: map[string]int64{"grain": 9}}}
	a := &w.Actors[0]
	a.Money, a.Energy = 10, 2
	w.Actors[1].Money = 10
	w.InitLedgerAudit()
	buy := Action{Kind: "market_buy", TargetID: "shop", Location: "home", Item: "grain", Quantity: 2, Price: 10}
	if w.View(a.ID).CheckAction(buy).Allowed || w.CheckAction(a.ID, buy).Allowed {
		t.Fatal("a two-unit parcel was bought with only two energy")
	}
	lawRejected(t, w.Apply(a.ID, buy))
	if a.Energy != 2 || a.Money != 10 || a.Items["grain"] != 0 {
		t.Fatal("the blocked purchase changed one of the actor's holdings")
	}
	a.Energy = 3
	if !w.View(a.ID).CheckAction(buy).Allowed || !w.CheckAction(a.ID, buy).Allowed {
		t.Fatal("exactly three energy must be enough for the purchase")
	}
	events := w.Apply(a.ID, buy)
	lawOK(t, events)
	if a.Energy != 0 || a.Money != 0 || a.Items["grain"] != 2 || payloadInt64(events[0].Payload, "energy_spent") != 3 {
		t.Fatalf("energy, money or goods were settled incorrectly: actor=%+v event=%+v", a, events[0])
	}
	if w.Cohorts[0].AvgEnergy != 0 || payloadInt64(events[0].Payload, "cohort_energy_spent") != 3 {
		t.Fatal("the seller cohort must also spend the three energy required for its lot")
	}
	lawRejected(t, w.Apply(w.Actors[1].ID, buy))
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestEveryBuiltinActionDeclaresItsEnergyRule(t *testing.T) {
	for _, kind := range RegisteredBuiltinActionKinds() {
		if builtinActionMetadataIndex[kind].referenceEnergy == 0 {
			t.Errorf("%s has no explicit energy rule", kind)
		}
	}
}

func TestSpeechSpendsEnergyAndTheListenerKeepsTheirs(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Energy = 1
	event := w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "Good morning."})
	lawOK(t, event)
	if w.Actors[0].Energy != 0 || w.Actors[1].Energy != MaxEnergy || payloadInt64(event[0].Payload, "energy_spent") != 1 {
		t.Fatal("speech should show one energy spent by its speaker")
	}
}

func TestMarketHandlingDependsOnCargoWeight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mass   int64
		count  int
		energy int
	}{
		{"one light parcel", 1, 1, 2},
		{"larger lot", 1, 4, 5},
		{"heavy cargo", 3, 4, 13},
		{"exactly full energy", MaxEnergy - 1, 1, MaxEnergy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			grain := w.Catalog.Items["grain"]
			grain.MassUnits, grain.Tradeable, grain.BatchQty, grain.BatchPrice = tc.mass, true, tc.count, 1
			w.Catalog.Items["grain"] = grain
			w.Cohorts = []PopulationCohort{{ID: "market", District: "home", Population: 1, Adults: 1,
				AvgEnergy: tc.energy, ItemPool: map[string]int64{"grain": 20}}}
			x := Action{Kind: "market_buy", TargetID: "market", Item: "grain", Location: "home", Quantity: tc.count, Price: 1}
			a := &w.Actors[0]
			a.Money, a.Energy = 1, tc.energy-1
			if w.View(a.ID).CheckAction(x).Allowed || w.CheckAction(a.ID, x).Allowed {
				t.Fatal("a buyer one point short could lift and settle the lot")
			}
			a.Energy = tc.energy
			events := w.Apply(a.ID, x)
			lawOK(t, events)
			if a.Energy != 0 || w.Cohorts[0].AvgEnergy != 0 ||
				payloadInt64(events[0].Payload, "energy_spent") != int64(tc.energy) {
				t.Fatalf("the shipment used the wrong handling budget: actor=%d cohort=%d events=%+v", a.Energy, w.Cohorts[0].AvgEnergy, events)
			}
		})
	}
}

func TestDirectTradeUsesTheSettledTermsForBothParties(t *testing.T) {
	w := lawWorld()
	grain := w.Catalog.Items["grain"]
	grain.Tradeable = true
	w.Catalog.Items["grain"] = grain
	seller, buyer := &w.Actors[0], &w.Actors[1]
	seller.Items["grain"], seller.Energy = 4, 5
	buyer.Money, buyer.Energy = 10, 5
	w.InitLedgerAudit()
	offered := w.Apply(seller.ID, Action{Kind: "offer", TargetID: buyer.ID, Item: "grain", Quantity: 4, Price: 10})
	lawOK(t, offered)
	x := Action{Kind: "accept", OfferID: offered[0].ID, Quantity: 1}
	lawRejected(t, w.Apply(buyer.ID, x))
	if seller.Items["grain"] != 4 || seller.Energy != 4 || buyer.Energy != 5 || buyer.Money != 10 {
		t.Fatal("the failed four-unit trade spent energy, money or stock")
	}
	seller.Energy = 5
	events := w.Apply(buyer.ID, x)
	lawOK(t, events)
	if buyer.Items["grain"] != 4 || seller.Energy != 0 || buyer.Energy != 0 ||
		payloadInt64(events[0].Payload, "seller_energy_spent") != 5 || payloadInt64(events[0].Payload, "buyer_energy_spent") != 5 {
		t.Fatalf("the trade used the request's quantity instead of the signed lot: %+v", events)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestCraftingFacilitySavesTimeAndEnergy(t *testing.T) {
	for _, tc := range []struct{ bonus, ticks, energy int }{{0, 4, 8}, {100, 2, 4}} {
		w := lawWorld()
		w.Catalog.Recipes[0].Ticks, w.Catalog.Recipes[0].EnergyCost = 4, 8
		w.Houses = []House{{ID: "workshop", District: "home", OwnerID: "a", Condition: MaxCondition}}
		w.Catalog.Techs = map[string]TechSpec{"bench": {ID: "bench", ProductionBonusPct: tc.bonus}}
		w.Knowledge = &KnowledgeBook{Records: map[string]*KnowledgeRecord{"bench": {AdoptedHouses: []string{"workshop"}}}}
		a := &w.Actors[0]
		a.InsideHouse, a.Items["wood"], a.Energy = "workshop", 3, tc.energy
		x := Action{Kind: "craft", RecipeID: "board"}
		view := w.View(a.ID)
		ticks, energy := view.ActionWorkEstimate(x)
		if ticks != tc.ticks || energy != tc.energy || !view.CheckAction(x).Allowed || !w.CheckAction(a.ID, x).Allowed {
			t.Fatalf("facility bonus %d: forecast (%d ticks, %d energy), want (%d, %d)", tc.bonus, ticks, energy, tc.ticks, tc.energy)
		}
		events := w.Apply(a.ID, x)
		lawOK(t, events)
		for tick := 1; tick < tc.ticks; tick++ {
			w.Tick++
			events = w.Advance(a.ID)
		}
		if a.Task != nil || a.Items["board"] != 1 || a.Energy != 0 || len(events) == 0 ||
			payloadInt64(events[0].Payload, "energy_spent_total") != int64(tc.energy) {
			t.Fatalf("facility bonus %d: energy=%d, events=%+v", tc.bonus, a.Energy, events)
		}
	}
}

func TestAnExhaustedWorkerPaysOnlyForWorkAlreadyDone(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Items["wood"], a.Energy, a.AwakeEnergyRemainder = 3, 4, 1
	lawOK(t, w.Apply(a.ID, Action{Kind: "craft", RecipeID: "board"}))
	for i := 0; i < 2; i++ {
		w.Tick++
		w.stepActorPhysiologyAndLifecycleFor(w.Actors[:1], false)
		events := w.Advance(a.ID)
		if i == 1 {
			lawRejected(t, events)
			if payloadInt64(events[0].Payload, "energy_spent_total") != 3 || payloadInt64(events[0].Payload, "work_ticks") != 2 {
				t.Fatalf("failed work charged time it never used: %+v", events)
			}
		}
	}
	if a.Task != nil || a.Energy != 0 || a.Items["wood"] != 3 || a.Items["board"] != 0 {
		t.Fatalf("interrupted job did not return materials or end: %+v", a)
	}
}

func TestTimedWorkPaysForEachProductiveInterval(t *testing.T) {
	w := lawWorld()
	w.Catalog.ActionSpecs = map[string]ActionSpec{"drill": {
		ID: "drill", Name: "Drill", TargetKind: "self", GrantedByTags: []string{"person"}, Ticks: 4, EnergyCost: 8,
	}}
	a := &w.Actors[0]
	a.Energy = 8
	lawOK(t, w.Apply(a.ID, Action{Kind: "drill"}))
	if a.Energy != 6 || a.Task == nil {
		t.Fatalf("starting four intervals of practice prepaid future effort: energy=%d", a.Energy)
	}
	w = lawRestore(t, w)
	for tick := 1; tick <= 3; tick++ {
		w.Tick++
		events := w.Advance(a.ID)
		if energy := w.Actors[0].Energy; energy != 6-2*tick {
			t.Fatalf("after practice interval %d, energy=%d", tick+1, energy)
		}
		if tick == 3 && (len(events) != 1 || payloadInt64(events[0].Payload, "energy_spent_total") != 8 ||
			payloadInt64(events[0].Payload, "work_ticks") != 4) {
			t.Fatalf("the completed drill must account for four intervals and eight points: %+v", events)
		}
	}
}

package sim

import "testing"

func TestFoodReserveCountsLivingAnonymousPeopleRatherThanStoredDeadRecords(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []Actor
		people  int
	}{
		{"all_anonymous", nil, 4},
		{"one_living_member", []Actor{{ID: "member", AgeYears: 30}}, 3},
		{"one_dead_record", []Actor{{ID: "dead", Dead: true}}, 4},
		{"living_and_dead", []Actor{{ID: "member", AgeYears: 30}, {ID: "dead", Dead: true}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			c := PopulationCohort{Population: 4, Adults: 4, Profession: "worker", Members: tc.members, ItemPool: map[string]int64{"grain": int64(tc.people * 3)}}
			if got := w.cohortFoodSurplus(c, "grain"); got != 0 {
				t.Fatalf("a stored dead record released %d portions from living people's one-day reserve", got)
			}
			c.ItemPool["grain"]++
			if got := w.cohortFoodSurplus(c, "grain"); got != 1 {
				t.Fatalf("exactly one surplus portion must remain available, got %d", got)
			}
		})
	}
}

func TestPartialCohortMealsPayOnlyTheirActualHungerRelief(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	w.Actors[0].Items["grain"] = 1
	w.Cohorts = []PopulationCohort{{
		ID:          "households",
		District:    "home",
		Population:  4,
		Adults:      4,
		AvgHunger:   1,
		ResidentSeq: 4,
		Residents: []ResidentBand{
			{Origin: "households", First: 1, Count: 3, Hunger: 1, Health: MaxHealth, Items: map[string]int{"grain": 1}},
			{Origin: "households", First: 4, Count: 1, Hunger: 1, Health: MaxHealth},
		},
	}}
	w.InitLedgerAudit()
	w.Tick = 1
	w.stepCohorts()
	if w.Cohorts[0].AvgHunger == 0 {
		t.Fatal("three portions were treated as enough to remove four people's hunger")
	}
	if w.Cohorts[0].HungerReliefRemainder != 3 {
		t.Fatal("the three paid portions were not retained as three units of relief")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
	// Deliver the one remaining portion from its actual holder to resident 4.
	w = lawRestore(t, w)
	w.Actors[0].Items["grain"]--
	w.Cohorts[0].Residents[1].Items = map[string]int{"grain": 1}
	w.Tick = 2
	w.stepCohorts()
	if w.Cohorts[0].AvgHunger != 0 || w.Cohorts[0].DigestedFood != 4 || len(w.Cohorts[0].Residents) != 1 || w.Cohorts[0].Residents[0].Count != 4 {
		t.Fatal("partial meals were forgotten instead of finishing the four-portion requirement")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPartialMealBalanceSurvivesAnonymousExpansionAndPopulationTransfers(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{ID: "people", District: "home", Profession: "worker", ProfLevel: 1, Population: 4, Adults: 4, AvgEnergy: 100, AvgHunger: 1, HungerReliefRemainder: 3}}
	for split := 0; split < 4; split++ {
		if _, err := w.HydrateActor("people"); err != nil {
			t.Fatal(err)
		}
		c := w.Cohorts[0]
		unpaid := int64(c.AvgHunger*c.Population) - c.HungerReliefRemainder
		for _, a := range w.Actors {
			unpaid += int64(a.Hunger)
		}
		if unpaid != 1 {
			t.Fatalf("split %d lost or created unmet hunger: %d", split, unpaid)
		}
		if err := c.ValidateHungerBalance(); err != nil {
			t.Fatal(err)
		}
		w = lawRestore(t, w)
	}
	w.Cohorts = []PopulationCohort{{ID: "from", Population: 5, Adults: 5, AvgHunger: 2, HungerReliefRemainder: 2}, {ID: "to", Population: 3, Adults: 3, AvgHunger: 1, HungerReliefRemainder: 1}}
	w.transferCohortEnergyAndHunger(&w.Cohorts[0], &w.Cohorts[1], 2, 5, 3)
	w.Cohorts[0].Population, w.Cohorts[0].Adults = 3, 3
	w.Cohorts[1].Population, w.Cohorts[1].Adults = 5, 5
	// The first two residents each carry one unpaid unit to the new cohort.
	for i, c := range w.Cohorts {
		if got, want := c.hungerUnits(c.Population), []int64{6, 4}[i]; got != want {
			t.Fatalf("%s unpaid hunger after transfer = %d, want %d", c.ID, got, want)
		}
		if err := c.ValidateHungerBalance(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCohortReliefBuysOnlyFundedFoodAndDoesNotEatOnDelivery(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		cash, treasury, food int64
	}{
		{"unfunded", 0, 0, 0},
		{"below_existing_public_policy_budget", 0, 27, 0},
		{"one_household_coin", 1, 0, 2},
		{"one_assessed_subsidy_coin", 0, 28, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			w.Actors = nil
			grain := w.Catalog.Items["grain"]
			grain.Tradeable, grain.BatchQty, grain.BatchPrice = true, 2, 1
			w.Catalog.Items["grain"] = grain
			w.Cohorts = []PopulationCohort{{ID: "hungry", District: "home", Population: 4, Adults: 4, AvgHunger: 3, MoneyPool: tc.cash}, {ID: "supplier", District: "home", Population: 1, Adults: 1, ItemPool: map[string]int64{"grain": 10}}}
			w.PublicTreasury = tc.treasury
			// Existing policy assesses 1/28 of public funds above its initial
			// reserve. These receipts were earned after an initial balance of zero.
			w.TreasuryLedger = TreasuryLedger{Initialized: true}
			w.InitLedgerAudit()
			w.Tick = 23
			w.stepCohortPhysiologySanitationAndLifecycle()
			if w.Cohorts[0].ItemPool["grain"] != tc.food || w.Cohorts[1].ItemPool["grain"] != 10-tc.food || w.Cohorts[1].MoneyPool != tc.food/2 {
				t.Fatalf("food moved without full payment for its actual quantity: %+v", w.Cohorts)
			}
			if w.Cohorts[0].AvgHunger != 3 || w.Cohorts[0].DigestedFood != 0 {
				t.Fatal("delivering food also counted it as eaten")
			}
			w.Tick = 24
			w.stepCohorts()
			// Partial stock is allocated to stable resident identities at the next meal, not on delivery.
			if w.Cohorts[0].DigestedFood != tc.food || w.Cohorts[0].ItemPool["grain"] != 0 {
				t.Fatalf("paid scarce food must feed exactly the funded number of residents: %+v", w.Cohorts[0])
			}
			if err := w.VerifyLedgerConservation(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHungerCreditAcceptsZeroAndTheLargestRemainderAndRejectsUnfundedCredit(t *testing.T) {
	for _, tc := range []struct {
		cohort PopulationCohort
		valid  bool
	}{
		{PopulationCohort{}, true},
		{PopulationCohort{Population: 1}, true},
		{PopulationCohort{Population: 4, AvgHunger: 1, HungerReliefRemainder: 3}, true},
		{PopulationCohort{Population: 4, AvgHunger: 1, HungerReliefRemainder: 4}, false},
		{PopulationCohort{Population: 4, AvgHunger: 1, HungerReliefRemainder: -1}, false},
		{PopulationCohort{Population: 4, HungerReliefRemainder: 1}, false},
	} {
		w := lawWorld()
		w.Cohorts = []PopulationCohort{tc.cohort}
		if err := w.VerifyHoldingBounds(); (err == nil) != tc.valid {
			t.Fatalf("valid=%v cohort=%+v err=%v", tc.valid, tc.cohort, err)
		}
		if _, _, err := ReplayFromCheckpoint(lawJSON(t, w), nil, w.Tick); (err == nil) != tc.valid {
			t.Fatalf("checkpoint changed the hunger-credit validity: valid=%v err=%v", tc.valid, err)
		}
	}
}

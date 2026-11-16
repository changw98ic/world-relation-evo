package sim

import "testing"

func resourceTestWorld(stock int64) *World {
	return &World{ID: "resource-test", NextID: 1, Catalog: Catalog{
		Calendar:    DefaultCalendarSpec(),
		Items:       map[string]ItemSpec{"timber": {ID: "timber"}, "water": {ID: "water", Potable: true}},
		Professions: []Profession{{ID: "logger", MaxLevel: 10}},
		Recipes:     []Recipe{{ID: "cut", Ticks: 1, Profession: "logger", ResourcePool: "trees", ResourceDraw: 2, OutputItem: "timber", OutputQty: 4}},
		Resources:   map[string]ResourcePoolSpec{"trees": {ID: "trees"}},
	}, DistrictStates: map[string]DistrictState{"forest": {Resources: map[string]int64{"trees": stock}, MaxResources: map[string]int64{"trees": 100}}},
		Cohorts: []PopulationCohort{{ID: "loggers", District: "forest", Profession: "logger", ProfLevel: 1, Population: 60, AvgEnergy: 100, ItemPool: map[string]int64{}}}}
}

func TestResourceProductionHonorsAvailableBatches(t *testing.T) {
	for _, stock := range []int64{0, 5, 6, 25} {
		w := resourceTestWorld(stock)
		w.InitLedgerAudit()
		events := w.runCohortProduction(&w.Cohorts[0])
		var batches int64
		for _, e := range events {
			if e.Kind == "production_started" {
				batches = e.Payload["batches"].(int64)
			}
		}
		want := (stock - 5) / 2
		if want < 0 {
			want = 0
		}
		if want > 10 {
			want = 10
		}
		if batches != want {
			t.Errorf("stock %d: batches %d, want %d", stock, batches, want)
		}
		if got := w.DistrictStates["forest"].Resources["trees"]; got != stock-want*2 {
			t.Errorf("stock %d: remaining %d", stock, got)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Errorf("stock %d: %v", stock, err)
		}
	}
}

func TestResourceZeroRegenerationStaysZero(t *testing.T) {
	w := resourceTestWorld(10)
	w.Tick = 4 // 07:00
	w.Map.Districts = []District{{ID: "forest"}}
	w.stepWorldEnvironment()
	if got := w.DistrictStates["forest"].Resources["trees"]; got != 10 {
		t.Fatalf("nonrenewable pool regenerated: %d", got)
	}
}

func TestResourceDryCohortCannotDrink(t *testing.T) {
	w := resourceTestWorld(0)
	w.Tick = 23
	w.DistrictStates = nil
	w.Map.Districts = []District{{ID: "forest", Features: []string{"residential"}}}
	w.stepCohortPhysiologySanitationAndLifecycle()
	if w.Cohorts[0].AvgThirst != 1 {
		t.Fatal("thirst was satisfied without water")
	}
}

func TestResourceMillionWorldLedger(t *testing.T) {
	w := LargeWorld("resource-million", 1000000)
	for i := 0; i < 96; i++ {
		// Exercise group production, trade, drinking, treatment, wear, and daily ecology.

		w.stepCohorts()
		w.Tick++
		w.settleShipments()
		w.AdvanceGrowth()
		w.stepWorldEnvironment()
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatalf("tick %d: %v", w.Tick, err)
		}
	}
	w.Cohorts[0].ItemPool["water"]++
	if err := w.VerifyLedgerConservation(); err == nil {
		t.Fatal("unrecorded creation was accepted")
	}
}

func TestResourceCraftReservesBeforeChargingAndCannotDoubleSpend(t *testing.T) {
	w := resourceTestWorld(7)
	w.Cohorts = nil
	w.Catalog.TagActions = map[string][]string{"worker": {"craft", "drink"}}
	w.Catalog.Recipes[0].Ticks = 2
	w.Catalog.Recipes[0].Inputs = map[string]int{"water": 1}
	w.Catalog.Recipes[0].EnergyCost = 3
	w.Catalog.Recipes[0].MoneyCost = 2
	for _, id := range []string{"a", "b"} {
		w.Actors = append(w.Actors, Actor{ID: id, Location: "forest", Tags: []string{"worker"}, Professions: map[string]int{"logger": 1}, Items: map[string]int{"water": 1}, Money: 10, Energy: 100, Health: 100})
	}
	w.InitLedgerAudit()
	w.Apply("a", Action{Kind: "craft", RecipeID: "cut"})
	if w.Actors[0].Task == nil {
		t.Fatal("exact available resource should start work")
	}
	events := w.Apply("b", Action{Kind: "craft", RecipeID: "cut"})
	if len(events) != 1 || events[0].Kind != "action_failed" {
		t.Fatal("second actor must not spend the same resources")
	}
	if a := w.Actors[1]; a.Money != 10 || a.Energy != 100 || a.Items["water"] != 1 {
		t.Fatal("rejected work charged inputs")
	}
	w.Advance("a")
	if w.Actors[0].Items["timber"] <= 0 {
		t.Fatal("funded work did not complete")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceNaturalRegenerationAndAudit(t *testing.T) {
	w := resourceTestWorld(90)
	w.Map.Districts = []District{{ID: "forest"}}
	w.Catalog.Resources["trees"] = ResourcePoolSpec{ID: "trees", RegenPermillePerDay: 200}
	w.InitLedgerAudit()
	w.Tick = 4
	w.stepWorldEnvironment()
	if got := w.DistrictStates["forest"].Resources["trees"]; got != 100 {
		t.Fatalf("capacity: %d", got)
	}
	if got := w.LedgerAudit.Ecology["forest"]["trees"].Produced; got != 10 {
		t.Fatalf("actual regeneration: %d", got)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
	w.DistrictStates["forest"].Resources["trees"]++
	if err := w.VerifyLedgerConservation(); err == nil {
		t.Fatal("unrecorded natural resource creation accepted")
	}
}

func TestResourceDrinkingConsumesAvailableWater(t *testing.T) {
	w := resourceTestWorld(6)
	w.Catalog.Resources["trees"] = ResourcePoolSpec{ID: "trees", PotableWater: true}
	w.Catalog.TagActions = map[string][]string{"worker": {"drink"}}
	w.Actors = []Actor{{ID: "a", Location: "forest", Tags: []string{"worker"}, Thirst: 2, Health: 100}}
	w.InitLedgerAudit()
	events := w.Apply("a", Action{Kind: "drink"})
	if len(events) != 1 || events[0].Kind != "drank" {
		t.Fatal("last available unit should be drinkable")
	}
	if w.DistrictStates["forest"].Resources["trees"] != 5 {
		t.Fatal("drinking did not deduct water")
	}
	w.Actors[0].Thirst = 2
	events = w.Apply("a", Action{Kind: "drink"})
	if len(events) != 1 || events[0].Kind != "action_failed" || w.Actors[0].Thirst != 2 {
		t.Fatal("empty source quenched thirst")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceNoAutomaticProductionWater(t *testing.T) {
	w := resourceTestWorld(0)
	w.Catalog = DefaultCatalog()
	w.Cohorts[0].Profession = "farmer"
	w.Cohorts[0].MoneyPool = 100
	w.InitLedgerAudit()
	w.runCohortProduction(&w.Cohorts[0])
	if w.Cohorts[0].ItemPool["water"] != 0 {
		t.Fatal("production created an uncollected water input")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestResourcePartialWaterUsesOnlyAvailableSupply(t *testing.T) {
	w := resourceTestWorld(6)
	w.Tick = 23
	w.Catalog.Resources["trees"] = ResourcePoolSpec{ID: "trees", PotableWater: true}
	w.Cohorts[0].Population = 4 // Existing demand rule: two units per six hours.
	w.Cohorts[0].ItemPool["water"] = 1
	w.InitLedgerAudit()
	w.stepCohortPhysiologySanitationAndLifecycle()
	if w.Cohorts[0].AvgThirst != 0 || w.Cohorts[0].ItemPool["water"] != 0 || w.DistrictStates["forest"].Resources["trees"] != 5 {
		t.Fatal("source plus stored water should satisfy exact demand")
	}
	w.stepCohortPhysiologySanitationAndLifecycle()
	if w.Cohorts[0].AvgThirst != 1 {
		t.Fatal("depleted supply should leave thirst unsatisfied")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceZeroWeatherMultiplierStopsRegeneration(t *testing.T) {
	w := resourceTestWorld(10)
	w.Map.Districts = []District{{ID: "forest"}}
	w.Catalog.Resources["trees"] = ResourcePoolSpec{ID: "trees", RegenPermillePerDay: 200}
	w.Catalog.Weathers = map[string]WeatherSpec{"": {ResourceRegenPct: map[string]int{"trees": 0}}}
	w.InitLedgerAudit()
	w.Tick = 4
	w.stepWorldEnvironment()
	if w.DistrictStates["forest"].Resources["trees"] != 10 {
		t.Fatal("zero weather multiplier regenerated resources")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceDrinkingCanCombineNaturalSources(t *testing.T) {
	w := resourceTestWorld(6)
	w.Tick = 23
	w.Catalog.Resources["trees"] = ResourcePoolSpec{ID: "trees", PotableWater: true}
	w.Catalog.Resources["well"] = ResourcePoolSpec{ID: "well", PotableWater: true}
	w.DistrictStates["forest"].Resources["well"] = 6
	w.DistrictStates["forest"].MaxResources["well"] = 100
	w.Cohorts[0].Population = 4
	w.InitLedgerAudit()
	w.stepCohortPhysiologySanitationAndLifecycle()
	if w.Cohorts[0].AvgThirst != 0 {
		t.Fatal("two available units across sources should satisfy demand")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

package sim

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestNutritionDailyDemandAndReserveSurviveSave(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	grain := w.Catalog.Items["grain"]
	grain.NutritionUnits = 5
	w.Catalog.Items["grain"] = grain
	a := &w.Actors[0]
	a.Hunger = 3
	a.Items["grain"] = 1
	w.InitLedgerAudit()
	lawOK(t, w.Apply(a.ID, Action{Kind: "eat", Item: "grain"}))
	if a.Hunger != 0 || a.NutritionReserve != 2 || a.Items["grain"] != 0 || a.DigestedFood != 1 {
		t.Fatalf("five units must satisfy three and retain two, consuming one item: %+v", a)
	}
	for tick := int64(1); tick <= TicksPerDay; tick++ {
		w.Tick = tick
		w.advanceActorHunger(&w.Actors[0])
		if tick == 47 {
			w = lawRestore(t, w)
		}
	}
	a = &w.Actors[0]
	if a.Hunger != 1 || a.NutritionReserve != 0 {
		t.Fatalf("one day must consume exactly three units: %+v", a)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
	for tick := int64(TicksPerDay + 1); tick <= 2*TicksPerDay; tick++ {
		w.Tick = tick
		w.advanceActorHunger(a)
	}
	if a.Hunger != 4 {
		t.Fatalf("second day used %d instead of three", a.Hunger-1)
	}
}

func TestNutritionReserveSurvivesMaterializationAndBandTransfer(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{ID: "people", District: "home", Profession: "worker", ProfLevel: 1, Population: 2, Adults: 2, AvgEnergy: 100, ResidentSeq: 2, Residents: []ResidentBand{{Origin: "people", First: 1, Count: 1, Health: 100, NutritionReserve: 2}, {Origin: "people", First: 2, Count: 1, Health: 100, NutritionReserve: 1}}}}
	w.InitLedgerAudit()
	a, err := w.HydrateActor("people")
	if err != nil {
		t.Fatal(err)
	}
	if a.NutritionReserve != 2 {
		t.Fatalf("hydration lost the selected person's balance: %+v", a)
	}
	id := a.ID
	if err := w.DehydrateActor(id); err != nil {
		t.Fatal(err)
	}
	w = lawRestore(t, w)
	a, err = w.HydrateActor("people")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != id || a.NutritionReserve != 2 {
		t.Fatalf("stored person lost identity or reserve: %+v", a)
	}
	c := &w.Cohorts[0]
	to := PopulationCohort{ID: "other"}
	c.moveResidents(&to, 1)
	if len(to.Residents) != 1 || to.Residents[0].NutritionReserve != 1 {
		t.Fatal("transfer lost reserve")
	}
	bands := appendResidentBand(nil, ResidentBand{Origin: "x", First: 1, Count: 1, Health: 100, NutritionReserve: 1})
	bands = appendResidentBand(bands, ResidentBand{Origin: "x", First: 2, Count: 1, Health: 100, NutritionReserve: 2})
	if len(bands) != 2 {
		t.Fatal("different nutritional balances merged")
	}
}

func TestNutritionMixedPartialAllocationOrderAndConservation(t *testing.T) {
	run := func(reverse bool) (*World, []ResidentBand) {
		w := lawWorld()
		w.Actors = nil
		w.Catalog.Items["corn"] = ItemSpec{ID: "corn", MassUnits: 1, NutritionUnits: 2, Edible: true}
		w.Catalog.Items["potato"] = ItemSpec{ID: "potato", MassUnits: 1, NutritionUnits: 3, Edible: true}
		bands := []ResidentBand{{Origin: "people", First: 1, Count: 2, Hunger: 3, Health: 100}, {Origin: "people", First: 3, Count: 2, Hunger: 3, Health: 90}}
		if reverse {
			bands[0], bands[1] = bands[1], bands[0]
		}
		w.Cohorts = []PopulationCohort{{ID: "people", Population: 4, Adults: 4, AvgHunger: 3, Residents: bands, ResidentSeq: 4, ItemPool: map[string]int64{"corn": 1, "potato": 2}}}
		w.InitLedgerAudit()
		w.settleResidentMeals(&w.Cohorts[0], []string{"corn", "potato"}, 1)
		c := w.Cohorts[0]
		if c.DigestedFood != 3 || c.ItemPool["corn"] != 0 || c.ItemPool["potato"] != 0 || c.hungerUnits(4) != 4 {
			t.Fatalf("partial mixed food did not deliver eight real units: %+v", c)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
		return w, c.Residents
	}
	w, a := run(false)
	_, b := run(true)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("slice order affected allocation: %+v / %+v", a, b)
	}
	restored := lawRestore(t, w)
	if !reflect.DeepEqual(restored.Cohorts[0].Residents, a) {
		t.Fatal("save lost nutritional state")
	}
}

func TestNutritionMillionPeopleStayCompact(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	grain := w.Catalog.Items["grain"]
	grain.NutritionUnits = 5
	w.Catalog.Items["grain"] = grain
	const population = 1000000
	c := PopulationCohort{ID: "people", Population: population, Adults: population, AvgHunger: 3, ItemPool: map[string]int64{"grain": population}}
	w.settleResidentMeals(&c, []string{"grain"}, 1)
	if len(c.Residents) != 1 || c.Residents[0].NutritionReserve != 2 || c.DigestedFood != population || c.ItemPool["grain"] != 0 {
		t.Fatalf("million identical meals must stay one range: %+v", c)
	}
}

func TestNutritionDoesNotMakeNonEdibleAssetsIntoFood(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Hunger = 3
	w.Catalog.Items["seed"] = ItemSpec{ID: "seed", MassUnits: 1, NutritionUnits: 5}
	a.Items["seed"] = 1
	lawRejected(t, w.Apply(a.ID, Action{Kind: "eat", Item: "seed"}))
	if a.Hunger != 3 || a.NutritionReserve != 0 || a.Items["seed"] != 1 {
		t.Fatal("nonfood nutrition became an edible meal")
	}
}

func TestNutritionAutomaticMealsSkipZeroNutritionWithoutBanningItem(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	w.Catalog.Items["a_zero"] = ItemSpec{ID: "a_zero", Edible: true, MassUnits: 1}
	w.Catalog.Items["z_meal"] = ItemSpec{ID: "z_meal", Edible: true, NutritionUnits: 3, MassUnits: 1}
	a := &w.Actors[0]
	a.Hunger = 3
	a.Items = map[string]int{"a_zero": 10, "z_meal": 1}
	w.consumeActorRation(a)
	if a.Hunger != 0 || a.Items["a_zero"] != 10 || a.Items["z_meal"] != 0 {
		t.Fatalf("automatic personal food picked empty nutrition: %+v", a)
	}
	for _, personal := range []bool{false, true} {
		c := PopulationCohort{ID: "v", Population: 2, Adults: 2, AvgHunger: 3, ItemPool: map[string]int64{"a_zero": 10, "z_meal": 2}}
		if personal {
			c.ItemPool = nil
			c.ResidentSeq = 2
			c.Residents = []ResidentBand{{Origin: "v", First: 1, Count: 2, Hunger: 3, Health: 100, Items: map[string]int{"a_zero": 10, "z_meal": 1}}}
		}
		w.settleResidentMeals(&c, []string{"a_zero", "z_meal"}, 1)
		if c.AvgHunger != 0 || c.DigestedFood != 2 {
			t.Fatalf("automatic cohort meal starved with positive nutrition: %+v", c)
		}
	}
	a.Hunger = 1
	lawOK(t, w.Apply(a.ID, Action{Kind: "eat", Item: "a_zero"}))
	if a.Hunger != 1 || a.Items["a_zero"] != 9 {
		t.Fatal("explicit zero-nutrition consumption was rejected or magically fed the actor")
	}
}

func TestNutritionRejectsUnrepresentableExternalBalancesAndRecipeBudgets(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	item := w.Catalog.Items["grain"]
	item.NutritionUnits = math.MaxInt64
	w.Catalog.Items["grain"] = item
	w.Actors[0].Items["grain"] = 1
	if err := w.VerifyHoldingBounds(); err != nil {
		t.Fatalf("one representable large unit rejected: %v", err)
	}
	w.Actors[0].Items["grain"] = 2
	if err := w.VerifyHoldingBounds(); err == nil || !strings.Contains(err.Error(), "nutrition") {
		t.Fatalf("unrepresentable product accepted: %v", err)
	}
	w.Actors[0].Items["grain"] = 1
	w.Actors[0].NutritionReserve = 1
	if err := w.VerifyHoldingBounds(); err == nil {
		t.Fatal("aggregate overflow accepted")
	}
	recipe := Recipe{ID: "overflow", OutputItem: "grain", OutputQty: 2, Inputs: map[string]int{"grain": 2}, Conversion: &MaterialConversion{Source: "fixture"}}
	if err := w.Catalog.ValidateProductionConversion(recipe); err == nil || !strings.Contains(err.Error(), "nutrition") {
		t.Fatalf("wrapped recipe budget accepted: %v", err)
	}
}

func TestNutritionRestoredWorldContinuesIdentically(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:1]
	w.Actors[0].NutritionReserve = 2
	w.Cohorts = []PopulationCohort{{ID: "v", Population: 2, Adults: 2, ResidentSeq: 2, Residents: []ResidentBand{{Origin: "v", First: 1, Count: 2, Health: 100, NutritionReserve: 2}}, ItemPool: map[string]int64{"grain": 4}}}
	w.InitLedgerAudit()
	restored := lawRestore(t, w)
	for tick := int64(1); tick <= 2*TicksPerDay; tick++ {
		for _, world := range []*World{w, restored} {
			world.Tick = tick
			world.advanceActorHunger(&world.Actors[0])
			world.settleResidentMeals(&world.Cohorts[0], []string{"grain"}, tick)
			if err := world.VerifyLedgerConservation(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !reflect.DeepEqual(lawJSON(t, w), lawJSON(t, restored)) {
		t.Fatal("restoring changed subsequent nutrition trajectory")
	}
}

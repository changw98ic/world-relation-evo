package sim

import "testing"

func TestPopulationCohortFormSpecDynamicMetabolismAndHydration(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	w := Village("cohort-form-test")
	w.Catalog.Forms = map[string]FormSpec{
		"mortal": {
			ID:             "mortal",
			Name:           "凡人",
			PowerTier:      1,
			DailyIntakePct: intPtr(100),
		},
		"fasting": {
			ID:             "fasting",
			Name:           "辟谷修士",
			PowerTier:      3,
			DailyIntakePct: intPtr(0),
			DisableHunger:  true,
			InnateTags:     []string{"cultivator"},
		},
		"devourer": {
			ID:                  "devourer",
			Name:                "气血体修",
			PowerTier:           3,
			DailyIntakePct:      intPtr(300),
			StarvationThreshold: 12,
			StarvationDamage:    intPtr(15),
			MinFoodTier:         2,
		},
	}
	w.Catalog.Items["coarse_grain"] = ItemSpec{ID: "coarse_grain", Name: "粗粮", Tier: 1, Edible: true, NutritionUnits: 1}
	w.Catalog.Items["beast_meat"] = ItemSpec{ID: "beast_meat", Name: "妖兽肉", Tier: 2, Edible: true, NutritionUnits: 3}

	d0 := w.Map.Districts[0].ID
	w.Cohorts = []PopulationCohort{
		{
			ID: "c_mortal", Name: "凡人群", District: d0, Profession: "farmer", FormID: "mortal",
			Population: 10, Adults: 10, AvgEnergy: 80, ItemPool: map[string]int64{"coarse_grain": 100},
		},
		{
			ID: "c_fast", Name: "辟谷群", District: d0, Profession: "farmer", FormID: "fasting",
			Population: 10, Adults: 10, AvgEnergy: 80, ItemPool: map[string]int64{},
		},
		{
			ID: "c_devour", Name: "体修群", District: d0, Profession: "farmer", FormID: "devourer",
			Population: 10, Adults: 10, AvgEnergy: 80, ItemPool: map[string]int64{"coarse_grain": 50, "beast_meat": 40},
		},
	}

	edible := []string{"coarse_grain", "beast_meat"}
	for tick := int64(1); tick <= TicksPerDay; tick++ {
		w.settleResidentMeals(&w.Cohorts[0], edible, tick)
		w.settleResidentMeals(&w.Cohorts[1], edible, tick)
		w.settleResidentMeals(&w.Cohorts[2], edible, tick)
	}

	// 1. Mortal cohort eats 3 units/person/day = 30 coarse_grain.
	if got := 100 - w.Cohorts[0].ItemPool["coarse_grain"]; got != 30 {
		t.Fatalf("mortal cohort consumed %d coarse_grain, want 30", got)
	}
	// 2. Fasting cohort eats 0 and takes 0 hunger/deaths despite empty ItemPool.
	if w.Cohorts[1].AvgHunger != 0 || w.Cohorts[1].Population != 10 {
		t.Fatalf("fasting cohort expected hunger=0 pop=10, got hunger=%d pop=%d", w.Cohorts[1].AvgHunger, w.Cohorts[1].Population)
	}
	// 3. Devourer cohort (300% intake = 9 units/person/day = 90 units total, MinFoodTier=2):
	// skips Tier-1 coarse_grain completely and consumes 30 Tier-2 beast_meat (30 * 3 = 90 units).
	if w.Cohorts[2].ItemPool["coarse_grain"] != 50 {
		t.Fatalf("devourer cohort (MinFoodTier=2) should not eat Tier-1 coarse_grain, remaining=%d", w.Cohorts[2].ItemPool["coarse_grain"])
	}
	if got := 40 - w.Cohorts[2].ItemPool["beast_meat"]; got != 30 {
		t.Fatalf("devourer cohort (300%% intake) consumed %d beast_meat, want 30", got)
	}

	// 4. HydrateActor preserves FormID and applies FormSpec.InnateTags cleanly.
	hydrated, err := w.HydrateActor("c_fast")
	if err != nil {
		t.Fatalf("HydrateActor(c_fast) failed: %v", err)
	}
	if hydrated.FormID != "fasting" {
		t.Fatalf("hydrated actor FormID = %q, want fasting", hydrated.FormID)
	}
	if !ActorHasTag(hydrated.Tags, "cultivator") {
		t.Fatalf("hydrated actor from fasting cohort missing innate tag 'cultivator': %+v", hydrated.Tags)
	}
}

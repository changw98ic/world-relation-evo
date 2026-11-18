package sim

import "testing"

func TestAProfessionWithoutUsableLandDoesNotLockAWholeCropSeasonOfTradeStock(t *testing.T) {
	w := lawWorld()
	w.Catalog.Professions = []Profession{{ID: "farmer", Grants: []string{"person"}}}
	crop := lawCrop()
	crop.Profession = "farmer"
	crop.Growth.DurationTicks = 120 * TicksPerDay
	w.Catalog.Recipes = []Recipe{crop}
	c := PopulationCohort{ID: "landless", District: "home", Profession: "farmer", ProfLevel: 1, Population: 1, Adults: 1, ItemPool: map[string]int64{"grain": 10}}
	if got := w.cohortFoodSurplus(c, "grain"); got != 6 {
		t.Fatalf("a producer with no right to plant could not sell six portions above its four-portion daily reserve: %d", got)
	}
	w.Production.Fields = []FieldAsset{{ID: "plot", District: "home", AreaM2: 1000, OwnerKind: "cohort", OwnerID: c.ID, UserKind: "cohort", UserID: c.ID}}
	if got := w.cohortFoodSurplus(c, "grain"); got != 0 {
		t.Fatalf("giving the qualified farmer a usable field did not restore its crop-period reserve: %d", got)
	}
}

func TestAdvancedCohortRetainsTheParentQualificationRequiredForItsProfession(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Catalog.Professions = []Profession{{ID: "worker", Grants: []string{"person"}}, {ID: "master", ParentProfession: "worker", MinParentLevel: 2}}
	w.Catalog.Recipes[0].Profession = "worker"
	w.Catalog.Recipes[0].MinProfessionLevel = 2
	w.Cohorts = []PopulationCohort{{ID: "masters", District: "home", Profession: "master", ProfLevel: 1, Population: 1, Adults: 1, AvgEnergy: 100, ItemPool: map[string]int64{"wood": 3}}}
	if events := w.runCohortProduction(&w.Cohorts[0]); len(events) != 1 || len(w.Growth) != 1 || w.Cohorts[0].ItemPool["wood"] != 0 {
		t.Fatal("starting the advanced profession erased the parent level that was required to enter it")
	}
}

func TestGenericCohortRecipeRequiresItsDeclaredPermissionIncludingInheritedGrants(t *testing.T) {
	for _, profession := range []string{"worker", "fisher", "master_fisher"} {
		w := lawEnvDrawWorld(1, 100)
		w.Actors = nil
		recipe := w.Catalog.Recipes[1]
		recipe.Profession, recipe.RequiredTag = "", "water_user"
		w.Catalog.Recipes = []Recipe{recipe}
		w.Catalog.Professions = []Profession{{ID: "worker"}, {ID: "fisher", Grants: []string{"water_user"}}, {ID: "master_fisher", ParentProfession: "fisher", MinParentLevel: 2}}
		w.Cohorts = []PopulationCohort{{ID: "workers", District: "home", Profession: profession, ProfLevel: 1, Population: 1, Adults: 1, AvgEnergy: 100}}
		events := w.runCohortProduction(&w.Cohorts[0])
		qualified := profession != "worker"
		if (len(events) == 1) != qualified || qualified && (len(w.Growth) != 1 || w.DistrictStates["home"].Resources["freshwater"] != 0) || !qualified && w.DistrictStates["home"].Resources["freshwater"] != 1 {
			t.Fatalf("profession=%s: a generic recipe ignored permission or lost an inherited grant", profession)
		}
	}
}

func TestFoodMarketFeedsAnEmptyProducerFromActualMerchantStock(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Catalog.Professions = []Profession{{ID: "farmer"}, {ID: "merchant", TradeMargin: true}}
	w.Catalog.TagProfiles = map[string]TagProfile{"farmer": {Produces: []string{"grain"}}}
	w.Catalog.Items["grain"] = ItemSpec{ID: "grain", Edible: true, HungerRelief: 1, Tradeable: true, BatchQty: 2, BatchPrice: 1}
	w.Cohorts = []PopulationCohort{
		{ID: "dealer", District: "home", Profession: "merchant", Population: 1, Adults: 1, ItemPool: map[string]int64{"grain": 12}},
		{ID: "farm", District: "home", Profession: "farmer", Population: 1, Adults: 1, AvgHunger: 3, MoneyPool: 3},
	}
	w.InitLedgerAudit()
	w.clearRegionalCohortMarket()
	if w.Cohorts[0].ItemPool["grain"] != 6 || w.Cohorts[1].ItemPool["grain"] != 6 || w.Cohorts[0].MoneyPool != 3 || w.Cohorts[1].MoneyPool != 0 {
		t.Fatalf("owning a food-producing profession prevented buying a meal, or a merchant could not resell a small real stock: %+v", w.Cohorts)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestCohortSubsistenceIsSharedByRetailReliefRentAndAdvancedFarmers(t *testing.T) {
	for _, profession := range []string{"farmer", "agronomist"} {
		w := lawWorld()
		w.Actors = w.Actors[:1]
		w.Actors[0].Money = 10
		w.Catalog.Professions = []Profession{{ID: "farmer", Grants: []string{"person"}}, {ID: "agronomist", ParentProfession: "farmer"}}
		crop := lawCrop()
		crop.Profession = "farmer"
		crop.Growth.DurationTicks = 120 * TicksPerDay
		w.Catalog.Recipes = []Recipe{crop}
		spec := w.Catalog.Items["grain"]
		spec.Tradeable, spec.BatchQty, spec.BatchPrice = true, 2, 1
		w.Catalog.Items["grain"] = spec
		w.Catalog.TagProfiles = map[string]TagProfile{"farmer": {Produces: []string{"grain"}}, "agronomist": {Produces: []string{"grain"}}}
		w.Cohorts = []PopulationCohort{{ID: "farm", District: "home", Profession: profession, ProfLevel: 1, Population: 2, Adults: 2, ItemPool: map[string]int64{"grain": 962}}, {ID: "landlord", District: "home", Population: 1, Adults: 1}}
		w.Production.Fields = []FieldAsset{{ID: "plots", District: "home", AreaM2: 2000, OwnerKind: "cohort", OwnerID: "farm", UserKind: "cohort", UserID: "farm"}}
		w.MarketCredit = NewMarketCreditState()
		w.MarketCredit.CohortEquityPermille["farm"] = map[string]int{"landlord": 1000}
		w.PublicTreasury = 10
		w.Actors[0].Hunger = 3
		w.clearFocalActorCohortTrades()
		w.stepDailyCreditAndAnnexation(w.MarketCredit, DefaultMarketCreditSpec())
		if w.Cohorts[0].ItemPool["grain"] != 962 || w.Actors[0].Items["grain"] != 0 || w.Cohorts[1].ItemPool["grain"] != 0 {
			t.Fatalf("%s lost food required before the next actual crop: %+v", profession, w.Cohorts)
		}
		w.Cohorts[0].ItemPool["grain"] += 2
		w.clearFocalActorCohortTrades()
		if w.Cohorts[0].ItemPool["grain"] != 962 || w.Actors[0].Items["grain"] != 2 {
			t.Fatalf("an exact surplus trade lot was blocked: profession=%s surplus=%d actor=%+v cohorts=%+v", profession, w.cohortFoodSurplus(w.Cohorts[0], "grain"), w.Actors[0], w.Cohorts)
		}
	}
}

func TestCohortOccupiedFieldsDoNotTriggerUnneededSeedProduction(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Catalog.Professions = []Profession{{ID: "farmer", MaxLevel: 1, Tier: 1, Grants: []string{"person"}}}
	crop := lawCrop()
	crop.Profession, crop.ID = "farmer", "farm_grain"
	crop.Growth.DurationTicks = 120 * TicksPerDay
	seed := Recipe{ID: "save_seeds", Profession: "farmer", Ticks: 1, EnergyCost: 2, Inputs: map[string]int{"grain": 1}, OutputItem: "seed", OutputQty: 1, Conversion: &MaterialConversion{Source: "fixture: stored grain selected as seed; same mass"}}
	w.Catalog.Recipes = []Recipe{crop, seed}
	w.Cohorts = []PopulationCohort{{ID: "farm", District: "home", Profession: "farmer", ProfLevel: 1, Population: 2, Adults: 2, AvgEnergy: 100, ItemPool: map[string]int64{"grain": 8, "water": 4}}}
	w.Production.Fields = []FieldAsset{{ID: "field", District: "home", AreaM2: 1000, OwnerKind: "cohort", OwnerID: "farm", UserKind: "cohort", UserID: "farm"}}
	c := &w.Cohorts[0]
	// The one available field has already been planted. More seed is not an
	// executable production need; the eight portions are the group's food.
	w.Growth = []GrowthBatch{{ID: 70, Cohort: true, OwnerID: c.ID, District: "home", RecipeID: crop.ID, FieldID: "field", AreaM2: 1000, Batches: 1, Quantity: 3, Rule: *crop.Growth}}
	if events := w.runCohortProduction(c); len(events) != 0 || c.ItemPool["grain"] != 8 {
		t.Fatalf("occupied field converted rations to unnecessary intermediates: stock=%v events=%v", c.ItemPool, events)
	}
	// A free field really does require seed. Only one field can be planted,
	// even though two workers are available.
	w.Growth = nil
	events := w.runCohortProduction(c)
	if len(events) == 0 || w.Growth[0].RecipeID != seed.ID || w.Growth[0].Batches != 1 || c.ItemPool["grain"] != 7 {
		t.Fatalf("real seed deficit for one field must fund exactly one batch: stock=%v growth=%+v", c.ItemPool, w.Growth)
	}
}

func TestCohortSowsLongCropWhileThereIsStillFoodForItsGrowthPeriod(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Catalog.Professions = []Profession{{ID: "farmer", MaxLevel: 1, Tier: 1, Grants: []string{"person"}}}
	crop := lawCrop()
	crop.ID, crop.Profession = "farm_grain", "farmer"
	crop.Growth.DurationTicks = 120 * TicksPerDay
	w.Catalog.Recipes = []Recipe{crop}
	w.Cohorts = []PopulationCohort{{ID: "farm", District: "home", Profession: "farmer", ProfLevel: 1, Population: 2, Adults: 2, AvgEnergy: 100, ItemPool: map[string]int64{"grain": 960, "seed": 1, "water": 1}}}
	w.Production.Fields = []FieldAsset{{ID: "field", District: "home", AreaM2: 1000, OwnerKind: "cohort", OwnerID: "farm", UserKind: "cohort", UserID: "farm"}}
	if events := w.runCohortProduction(&w.Cohorts[0]); len(events) != 2 || events[0].Kind != "growth_started" || events[1].Kind != "growth_work_completed" || len(w.Growth) != 1 || w.Growth[0].RecipeID != crop.ID {
		t.Fatal("120 days of food must not postpone a 120-day crop until only a few days of food remain")
	}
}

func TestCohortCareAndHarvestCanFinishAcrossSeveralWorkPeriods(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	crop := lawCrop()
	w.Catalog.Recipes = []Recipe{crop}
	w.Cohorts = []PopulationCohort{{ID: "farm", District: "home", Population: 2, Adults: 2, Wounded: 1, AvgEnergy: 100, ItemPool: map[string]int64{"water": 4}}}
	w.Growth = []GrowthBatch{{ID: 40, OwnerID: "farm", Cohort: true, District: "home", RecipeID: crop.ID, Rule: *crop.Growth, Batches: 2, Quantity: 6, Item: "grain"}}
	w.InitLedgerAudit()
	// Two fields need two worker-periods of care per interval and two to
	// harvest. One available worker can provide six periods over six ticks.
	for tick := int64(1); tick <= 6; tick++ {
		w.Tick = tick
		w.AdvanceGrowth()
		if tick < 6 && w.Cohorts[0].ItemPool["grain"] != 0 {
			t.Fatal("harvest delivered before the declared labor was completed")
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
	}
	if len(w.Growth) != 0 || w.Cohorts[0].ItemPool["grain"] != 6 || w.Cohorts[0].ItemPool["water"] != 0 {
		t.Fatalf("partial crew could never finish a paid crop: growth=%+v stock=%v", w.Growth, w.Cohorts[0].ItemPool)
	}
}

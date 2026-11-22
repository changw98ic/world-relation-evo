package sim

import "testing"

func TestP5LODPinningBarrierImmunityWorldEventShockAndHydrateConservation(t *testing.T) {
	w := Village("p5-lod")
	a := &w.Actors[0]
	a.Location = "square"
	a.InsideHouse = ""
	a.Energy = 80
	a.Health = MaxHealth

	// 1. IsActorPinnedFocal must protect high-FormSpec.PowerTier, GoldenFinger owners, and ActiveEffect sustainers.
	w.Catalog.Forms = map[string]FormSpec{
		"nascent_soul": {ID: "nascent_soul", Name: "Nascent Soul", PowerTier: 4},
		"mortal":       {ID: "mortal", Name: "Mortal", PowerTier: 1},
	}
	w.Actors[1].FormID = "nascent_soul"
	if !w.IsActorPinnedFocal(w.Actors[1]) {
		t.Fatalf("expected FormSpec PowerTier=4 actor to be pinned focal")
	}
	w.Catalog.GoldenFingers = []GoldenFingerSpec{{ID: "gf_pin", Name: "System", GrantedByTags: []string{"chosen_one"}}}
	w.Actors[2].Tags = appendUnique(w.Actors[2].Tags, "chosen_one")
	if !w.IsActorPinnedFocal(w.Actors[2]) {
		t.Fatalf("expected GoldenFinger actor to be pinned focal")
	}
	w.ActiveEffects = append(w.ActiveEffects, ActiveEffect{
		ID:             1,
		Name:           "Sect Ward",
		SustainActorID: w.Actors[3].ID,
		ExpiresTick:    w.Tick + 100,
	})
	if !w.IsActorPinnedFocal(w.Actors[3]) {
		t.Fatalf("expected ActiveEffect sustainer to be pinned focal")
	}

	// 2. Cohort attack respects FormSpec.PowerTier immunity and district ActiveEffect barriers.
	immortalCohort := PopulationCohort{
		ID:         "cohort_elders",
		Name:       "Immortal Elders",
		District:   "square",
		Profession: "farmer",
		FormID:     "nascent_soul",
		Tier:       1,
		Population: 50,
		Adults:     50,
	}
	mortalCohort := PopulationCohort{
		ID:         "cohort_militia",
		Name:       "Square Militia",
		District:   "square",
		Profession: "farmer",
		FormID:     "mortal",
		Tier:       1,
		Population: 100,
		Adults:     100,
	}
	w.Cohorts = []PopulationCohort{immortalCohort, mortalCohort}
	w.InitCohortResidents()
	// Give the first band a wounded resident and the second band a healthy resident holding 2 food.
	w.Cohorts[1].Residents = []ResidentBand{
		{Origin: "cohort_militia", First: 1, Count: 1, Health: 40},
		{Origin: "cohort_militia", First: 2, Count: 99, Health: MaxHealth, Items: map[string]int{"food": 2}},
	}
	w.Cohorts[1].Wounded = 1
	w.InitLedgerAudit()

	// Low-tier attacker (tier 1) vs nascent_soul cohort (tier 4) => immune!
	a.FormID = "mortal"
	wounded, dead, _, immune, _ := w.resolveCohortAttack(*a, &w.Cohorts[0], &AttackSpec{Tier: 1, CohortCasualties: 20}, Action{Kind: "strike"})
	if !immune || wounded != 0 || dead != 0 {
		t.Fatalf("expected tier-1 strike vs tier-4 cohort to be immune, got wounded=%d dead=%d immune=%v", wounded, dead, immune)
	}

	// High-tier attacker vs mortal cohort protected by district barrier absorbs damage first!
	w.ActiveEffects = append(w.ActiveEffects, ActiveEffect{
		ID:                        2,
		Name:                      "Guardian Dome",
		Anchor:                    EntityRef{Kind: "district", ID: "square"},
		Integrity:                 500,
		MaxIntegrity:              500,
		AbsorbDamageWithIntegrity: true,
		ExpiresTick:               w.Tick + 100,
	})
	a.FormID = "nascent_soul"
	wounded, dead, absorbed, immune, _ := w.resolveCohortAttack(*a, &w.Cohorts[1], &AttackSpec{Tier: 4, CohortCasualties: 20}, Action{Kind: "strike"})
	if immune || absorbed == 0 || dead != 0 {
		t.Fatalf("expected district barrier to absorb cohort strike, got wounded=%d dead=%d absorbed=%d immune=%v", wounded, dead, absorbed, immune)
	}

	// 3. HydrateActor pops a healthy resident (skipping wounded Residents[0]), transfers resident.Items, and keeps ledger conserved!
	hydrated, err := w.HydrateActor("cohort_militia")
	if err != nil {
		t.Fatalf("HydrateActor failed: %v", err)
	}
	if hydrated.Health != MaxHealth {
		t.Fatalf("expected HydrateActor to pop healthy resident (HP %d), got HP %d", MaxHealth, hydrated.Health)
	}
	if hydrated.Items["food"] < 2 {
		t.Fatalf("expected HydrateActor to preserve ResidentBand.Items (food>=2), got %v", hydrated.Items)
	}
	if w.Cohorts[1].Wounded != 1 {
		t.Fatalf("expected cohort Wounded count to remain 1, got %d", w.Cohorts[1].Wounded)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation broken after HydrateActor with ResidentBand.Items: %v", err)
	}

	// 4. WorldEventSpec shock casualties also strike co-district cohorts.
	w.ActiveEffects = nil
	popBefore := w.Cohorts[1].Population
	w.Catalog.WorldEvents = []WorldEventSpec{{
		ID:               "quake_test",
		Name:             "Great Quake",
		Category:         "terrain_shift",
		DurationTicks:    48,
		Intensity:        80,
		CasualtyPermille: 200,
		Damage:           15,
	}}
	_ = w.TriggerWorldEvent("quake_test", "square")
	if w.Cohorts[1].Population >= popBefore && w.Cohorts[1].Wounded <= 1 {
		t.Fatalf("expected WorldEventSpec shock to cause casualties on co-district cohort, pop=%d wounded=%d", w.Cohorts[1].Population, w.Cohorts[1].Wounded)
	}
}

func TestBlock3LODGenreNeutralConflictsImmortalCohortStateSlotsAndGini(t *testing.T) {
	w := Village("block3-lod")
	// 1. Sci-fi/custom profession IDs (zero ancient IDs) trigger cohort conflicts dynamically via tags/capabilities.
	w.Catalog.Professions = append(w.Catalog.Professions,
		Profession{ID: "cyber_enforcer", Name: "Cyber Enforcer", Tier: 1, Grants: []string{"law", "enforcer"}},
		Profession{ID: "scrap_raider", Name: "Scrap Raider", Tier: 1, Grants: []string{"outlaw", "raider"}},
		Profession{ID: "hydro_tech", Name: "Hydro Tech", Tier: 1, Grants: []string{"worker"}},
	)
	w.Catalog.TagActions["enforcer"] = []string{"arrest", "attack"}
	w.Catalog.TagActions["raider"] = []string{"raid", "attack"}
	w.Catalog.Forms = map[string]FormSpec{
		"android": {ID: "android", Name: "Android", PowerTier: 2, LifeExpectancyYears: -1},
	}
	w.Catalog.StateSlots = map[string]StateSlotSpec{
		"rad": {ID: "rad", Name: "Radiation", Min: 0, Max: 100, AmbientFieldKey: "radiation", AmbientAbsorbDivisor: 1},
	}
	w.Map.GeoRegions = []GeoRegion{{ID: "sector_1", Name: "Sector 1", AmbientFields: map[string]int{"radiation": 5}}}
	for i := range w.Map.Districts {
		w.Map.Districts[i].GeoRegionID = "sector_1"
	}
	dist := w.Map.Districts[0].ID

	w.Cohorts = []PopulationCohort{
		{ID: "c_enf", Name: "Enforcers", District: dist, Profession: "cyber_enforcer", Tier: 1, Population: 60, Adults: 60, MoneyPool: 600, ItemPool: map[string]int64{}},
		{ID: "c_raider", Name: "Raiders", District: dist, Profession: "scrap_raider", Tier: 1, Population: 60, Adults: 60, MoneyPool: 600, ItemPool: map[string]int64{}},
		{ID: "c_tech", Name: "Hydro Techs", District: dist, Profession: "hydro_tech", FormID: "android", Tier: 1, Population: 100, Adults: 50, Elders: 50, MoneyPool: 1000, ItemPool: map[string]int64{"food": 200}},
	}
	w.InitCohortResidents()
	w.InitLedgerAudit()

	// 14:00 afternoon sweep (ClockAt(0) = 06:00, so 14:00 is +8 hours = tick 32) resolves custom cyber_enforcer vs scrap_raider without any hardcoded "constable"/"bandit" IDs.
	w.Tick = 8 * TicksPerHour
	evs := w.stepCohortConflictsAt(dist)
	foundArrest := false
	for _, ev := range evs {
		if ev.Kind == "cohort_arrested" && ev.ActorID == "c_enf" && ev.TargetID == "c_raider" {
			foundArrest = true
		}
	}
	if !foundArrest {
		t.Fatalf("expected genre-neutral cohort_arrested between cyber_enforcer and scrap_raider, got %+v", evs)
	}

	// 2. Immortal/android cohort (FormSpec.LifeExpectancyYears < 0) has CohortFormLifeExpectancy == 0.
	if exp := CohortFormLifeExpectancy(w.Catalog, w.Cohorts[2]); exp != 0 {
		t.Fatalf("expected android cohort life expectancy 0 (ageless), got %d", exp)
	}

	// 3. Cohort StateSlots absorbs GeoRegion ambient field identically to individual Actors.
	w.stepStateSlotsAndForms()
	if got := w.Cohorts[2].StateSlots["rad"]; got != 5 {
		t.Fatalf("expected cohort StateSlots[rad]=5 from ambient region field, got %d", got)
	}

	// 4. Ledger conservation holds after genre-neutral conflict & payroll.
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed in Block 3: %v", err)
	}
}

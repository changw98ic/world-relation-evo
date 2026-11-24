package sim

import "testing"

func TestDeathCauseAuthorityMatchesScaleSummary(t *testing.T) {
	w := lawWorld()
	w.Actors = []Actor{
		{ID: "a1", Name: "A1", Location: "home", Health: 0},
		{ID: "a2", Name: "A2", Location: "home", Health: 0},
	}
	ev1 := w.markDeadIfZero(&w.Actors[0], nil, Action{Kind: "wait"}, "")
	ev2 := w.markDeadIfZero(&w.Actors[1], nil, Action{Kind: "wait"}, "starvation")
	w.recordDeathCause("", 1)

	s := NewScaleSummary()
	s.RecordEvents(ev1)
	s.RecordEvents(ev2)
	s.RecordEvents([]Event{{Kind: "actor_died", Payload: map[string]any{"cause": "unknown_custom_cause"}}})

	if w.DeathsByCause["combat"] != 2 || w.DeathsByCause["starvation"] != 1 || len(w.DeathsByCause) != 2 {
		t.Fatalf("unexpected World.DeathsByCause: %+v", w.DeathsByCause)
	}
	if s.DeathsByCause["combat"] != 2 || s.DeathsByCause["starvation"] != 1 || len(s.DeathsByCause) != 2 {
		t.Fatalf("unexpected ScaleSummary.DeathsByCause: %+v", s.DeathsByCause)
	}
}

func TestVerifyHoldingBoundsEnforcesResidentBandAuthority(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{
		ID:          "v",
		District:    "home",
		Population:  2,
		Adults:      2,
		AvgHunger:   1,
		ResidentSeq: 2,
		Residents:   []ResidentBand{{Origin: "v", First: 1, Count: 2, Hunger: 1, Health: MaxHealth}},
	}}
	if err := w.VerifyHoldingBounds(); err != nil {
		t.Fatalf("valid resident bands rejected: %v", err)
	}

	w.Cohorts[0].Residents[0].Count = 1
	if err := w.VerifyHoldingBounds(); err == nil {
		t.Fatal("expected population mismatch between ResidentBand and PopulationCohort to fail")
	}

	w.Cohorts[0].Residents[0].Count = 2
	w.Cohorts[0].Residents[0].Hunger = 0
	if err := w.VerifyHoldingBounds(); err == nil {
		t.Fatal("expected hunger mismatch between ResidentBand and PopulationCohort to fail")
	}
}

func TestValidatePhysicalRulesRejectsContradictoryRulesAndAcceptsBoundaries(t *testing.T) {
	base := func() Catalog {
		c := DefaultCatalog()
		return c
	}

	// Valid boundary: child_max_age + 1 == elder_min_age, elder_min_age + 1 == max_expectancy
	valid := base()
	valid.Lifecycle.ChildMaxAge = 15
	valid.Lifecycle.ElderMinAge = 16
	valid.Lifecycle.MaxExpectancy = 17
	if err := valid.ValidatePhysicalRules(nil); err != nil {
		t.Fatalf("valid lifecycle boundary rejected: %v", err)
	}

	// Contradictory lifecycle: child_max_age >= elder_min_age
	badAge := base()
	badAge.Lifecycle.ChildMaxAge = 55
	badAge.Lifecycle.ElderMinAge = 55
	if err := badAge.ValidatePhysicalRules(nil); err == nil {
		t.Fatal("expected child_max_age >= elder_min_age to fail")
	}

	// Contradictory lifecycle: elder_min_age >= max_expectancy
	badExp := base()
	badExp.Lifecycle.ElderMinAge = 78
	badExp.Lifecycle.MaxExpectancy = 78
	if err := badExp.ValidatePhysicalRules(nil); err == nil {
		t.Fatal("expected elder_min_age >= max_expectancy to fail")
	}

	// Unknown ActionSpec item cost
	badSpec := base()
	badSpec.ActionSpecs = map[string]ActionSpec{
		"test_act": {ID: "test_act", ItemCosts: map[string]int{"nonexistent_item": 1}},
	}
	if err := badSpec.ValidatePhysicalRules(nil); err == nil {
		t.Fatal("expected unknown ActionSpec item cost to fail")
	}
}

func TestCentralizedItemLedgerHelpersKeepConservation(t *testing.T) {
	w := lawWorld()
	w.Actors = []Actor{{ID: "a1", Location: "home", Items: map[string]int{"grain": 2}}}
	w.Cohorts = []PopulationCohort{{
		ID: "c1", District: "home", Population: 3, Adults: 3, ResidentSeq: 3,
		ItemPool:  map[string]int64{"grain": 5},
		Residents: []ResidentBand{{Origin: "c1", First: 1, Count: 3, Hunger: 0, Health: MaxHealth, Items: map[string]int{"grain": 1}}},
	}}
	w.InitLedgerAudit()

	w.adjustActorItem(&w.Actors[0], "grain", 3, true)
	w.adjustActorItem(&w.Actors[0], "grain", -1, true)
	w.adjustCohortItem(&w.Cohorts[0], "grain", -2, true)
	w.adjustResidentBandItem(&w.Cohorts[0].Residents[0], "grain", -1, true)

	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("centralized item helpers broke ledger conservation: %v", err)
	}
}

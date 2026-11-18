package sim

import "testing"

func TestResidentMealsAreOrderIndependentAndEquivalentToIndividualActors(t *testing.T) {
	runCohort := func(reversed bool) map[string]ResidentBand {
		w := lawWorld()
		grain := w.Catalog.Items["grain"]
		grain.HungerRelief = 4
		w.Catalog.Items["grain"] = grain

		bands := []ResidentBand{
			{Origin: "v", First: 1, Count: 2, Hunger: 3, Health: 30, Items: map[string]int{"grain": 1}},
			{Origin: "v", First: 3, Count: 2, Hunger: 3, Health: 30},
		}
		if reversed {
			bands[0], bands[1] = bands[1], bands[0]
		}
		w.Cohorts = []PopulationCohort{{
			ID:          "v",
			District:    "home",
			Population:  4,
			Adults:      4,
			ResidentSeq: 4,
			Residents:   bands,
			// Scarce pool (1 < 2 needy) must NOT favor whoever appears first in Residents.
			ItemPool: map[string]int64{"grain": 1},
		}}
		w.InitLedgerAudit()
		w.settleResidentMeals(&w.Cohorts[0], []string{"grain"}, 24)
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatalf("conservation failed (reversed=%v): %v", reversed, err)
		}
		out := map[string]ResidentBand{}
		for _, b := range w.Cohorts[0].Residents {
			for i := 0; i < b.Count; i++ {
				out[b.ID(i)] = b
			}
		}
		return out
	}

	forward := runCohort(false)
	reversed := runCohort(true)
	for id, fb := range forward {
		rb, ok := reversed[id]
		if !ok || fb.Hunger != rb.Hunger || fb.Health != rb.Health || fb.Items["grain"] != rb.Items["grain"] {
			t.Fatalf("storage order changed outcome for %s: forward=%+v reversed=%+v", id, fb, rb)
		}
	}

	// Compare batched settlement against fully expanded individual Actors.
	ind := lawWorld()
	grain := ind.Catalog.Items["grain"]
	grain.HungerRelief = 4
	ind.Catalog.Items["grain"] = grain
	actors := []Actor{
		{ID: "v_a0001", Hunger: 3, Health: 30, Items: map[string]int{"grain": 1}},
		{ID: "v_a0002", Hunger: 3, Health: 30, Items: map[string]int{"grain": 1}},
		{ID: "v_a0003", Hunger: 3, Health: 30},
		{ID: "v_a0004", Hunger: 3, Health: 30},
	}
	ind.Tick = 24
	for i := range actors {
		ind.advanceActorHunger(&actors[i])
		ind.consumeActorRation(&actors[i])
	}
	ind.stepActorPhysiologyAndLifecycleFor(actors, false)
	for _, a := range actors {
		b := forward[a.ID]
		if a.Hunger != b.Hunger || a.Health != b.Health || a.Items["grain"] != b.Items["grain"] {
			t.Fatalf("batch vs individual mismatch for %s: actor=%+v band=%+v", a.ID, a, b)
		}
	}
}

func TestWorkerRunsNeverAssignSickOrJailedResidents(t *testing.T) {
	c := PopulationCohort{
		ID:          "crew",
		Population:  4,
		ResidentSeq: 4,
		Residents: []ResidentBand{
			{Origin: "crew", First: 1, Count: 1, Health: MaxHealth, Sick: true},
			{Origin: "crew", First: 2, Count: 1, Health: MaxHealth, Jailed: true},
			{Origin: "crew", First: 3, Count: 2, Health: MaxHealth},
		},
	}
	runs, count := c.workerRuns(nil, 2)
	if count != 2 || len(runs) != 1 || runs[0].FirstID != "crew_a0003" || runs[0].Count != 2 {
		t.Fatalf("expected only healthy free workers crew_a0003..0004, got count=%d runs=%+v", count, runs)
	}
}

func TestCohortCasualtiesAndRecuperationSyncResidentBands(t *testing.T) {
	w := lawWorld()
	w.Cohorts = []PopulationCohort{{
		ID:          "crew",
		District:    "home",
		Population:  4,
		Adults:      4,
		ResidentSeq: 4,
		Residents:   []ResidentBand{{Origin: "crew", First: 1, Count: 4, Health: MaxHealth}},
	}}
	c := &w.Cohorts[0]
	w.applyCohortCasualtiesWithCause(c, 1, 0, 2, "combat")
	if c.Jailed != 2 || c.Wounded != 1 {
		t.Fatalf("expected jailed=2 wounded=1, got jailed=%d wounded=%d", c.Jailed, c.Wounded)
	}
	runs, free := c.workerRuns(nil, 4)
	if free != 2 || len(runs) != 1 || runs[0].FirstID != "crew_a0003" || runs[0].Count != 2 {
		t.Fatalf("expected only unjailed crew_a0003..0004 to work, got free=%d runs=%+v", free, runs)
	}
	woundedBands := 0
	for _, b := range c.Residents {
		if b.Health < MaxHealth {
			woundedBands += b.Count
		}
	}
	if woundedBands != 1 {
		t.Fatalf("expected 1 wounded resident band, got %d (%+v)", woundedBands, c.Residents)
	}

	// 18:00 jail docket releases 1 jailed resident (2/4 -> min 1), and resting recuperation heals 1 wounded.
	w.Tick = 12 * TicksPerHour // 06:00 + 12h = 18:00
	c.RestUntilTick = w.Tick + 1
	w.stepCohortRecuperation()
	if c.Jailed != 1 || c.Wounded != 0 {
		t.Fatalf("expected jailed=1 wounded=0 after recuperation, got jailed=%d wounded=%d", c.Jailed, c.Wounded)
	}
	_, freeAfter := c.workerRuns(nil, 4)
	if freeAfter != 3 {
		t.Fatalf("expected 3 free workers after release, got %d (%+v)", freeAfter, c.Residents)
	}
	for _, b := range c.Residents {
		if b.Health != MaxHealth {
			t.Fatalf("expected healed resident band health=%d, got %+v", MaxHealth, b)
		}
	}
}

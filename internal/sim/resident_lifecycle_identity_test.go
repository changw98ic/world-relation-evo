package sim

import "testing"

func TestPromotionMovesTheSameResidentsToNewProfession(t *testing.T) {
	w := lawWorld()
	w.Catalog.Professions = []Profession{{ID: "worker"}, {ID: "specialist", ParentProfession: "worker", Tier: 2}}
	w.Cohorts = []PopulationCohort{{ID: "workers", Profession: "worker", District: "home", Tier: 1, ProfLevel: 2, Population: 50, Adults: 50}}
	if err := w.InitCohortResidents(); err != nil {
		t.Fatal(err)
	}
	if events := w.promoteEligibleCohorts(); len(events) != 1 {
		t.Fatalf("expected a promotion, got %v", events)
	}
	src, dst := w.Cohorts[0], w.Cohorts[1]
	if src.Residents[0].First != 3 || src.Residents[0].Count != 48 || dst.Population != 2 || dst.Residents[0].ID(1) != "workers_a0002" {
		t.Fatalf("unexpected source and destination ranges %v, %v", src.Residents, dst.Residents)
	}
}

func TestDailyBirthsIssueNewIDs(t *testing.T) {
	w := lawWorld()
	w.Tick = 14 * TicksPerHour // 20:00, daily census.
	w.Cohorts = []PopulationCohort{{ID: "home", District: "home", Population: 10, Adults: 10}}
	w.Catalog.Lifecycle.AnnualBirthPermille = 1000 * CalendarAt(w.Tick, w.Catalog.Calendar).DaysInYear
	w.stepCohortPhysiologySanitationAndLifecycle()
	c := w.Cohorts[0]
	if c.CumulativeBirths != 10 || c.Population != 20 || len(c.Residents) != 1 || c.Residents[0].Count != 20 || c.Residents[0].ID(19) != "home_a0020" {
		t.Fatalf("daily census and IDs after births: %+v", c)
	}
}

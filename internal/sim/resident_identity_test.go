package sim

import "testing"

func TestResidentRangesSurviveTransferHydrationAndSnapshot(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{ID: "origin", District: "away", Profession: "worker", Population: 1000001, Members: []Actor{{ID: "origin_a0002"}}}, {ID: "destination", District: "home", Profession: "worker"}}
	if err := w.InitCohortResidents(); err != nil {
		t.Fatal(err)
	}
	src, dst := &w.Cohorts[0], &w.Cohorts[1]
	if len(src.Residents) != 1 || src.Residents[0].First != 3 || src.Residents[0].Count != 1000000 {
		t.Fatalf("starting range: %+v", src.Residents)
	}
	original := CloneCohort(*src)
	w.transferCohortEnergyAndHunger(src, dst, 2, 1000000, 0)
	src.Population -= 2
	dst.Population += 2
	if original.Residents[0].First != 3 || src.Residents[0].First != 5 {
		t.Fatal("transfer aliased clone or lost source IDs")
	}
	dst.Residents[0].Health = 80
	a, err := w.HydrateActor(dst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "origin_a0003" || a.Health != 80 {
		t.Fatalf("materialized %s, health %d", a.ID, a.Health)
	}
	if err := w.DehydrateActor(a.ID); err != nil {
		t.Fatal(err)
	}
	w = lawRestore(t, w)
	dst = &w.Cohorts[1]
	if len(dst.Members) != 1 || dst.Members[0].ID != "origin_a0003" || len(dst.Residents) != 1 || dst.Residents[0].ID(0) != "origin_a0004" {
		t.Fatalf("wrong identities after reload: %+v", dst)
	}
}

func TestResidentBirthAfterLastDeathAndRestart(t *testing.T) {
	w := lawWorld()
	w.Cohorts = []PopulationCohort{{ID: "people", Population: 1, Adults: 1}}
	c := &w.Cohorts[0]
	w.applyCohortCasualtiesWithCause(c, 0, 1, 0, "combat")
	w = lawRestore(t, w)
	c = &w.Cohorts[0]
	if len(c.Residents) != 0 || c.ResidentSeq != 1 {
		t.Fatalf("dead resident still present or ID forgotten: %+v", c)
	}
	c.issueResidents(1, 0)
	if c.Residents[0].ID(0) != "people_a0002" {
		t.Fatal("newborn reused retired ID")
	}
}

func TestResidentInitRejectsOverlappingAndIncompleteCensus(t *testing.T) {
	for _, bands := range [][]ResidentBand{{{Origin: "c", First: 1, Count: 1, Health: MaxHealth}, {Origin: "c", First: 1, Count: 1, Health: MaxHealth}}, {{Origin: "c", First: 1, Count: 1, Health: MaxHealth}}} {
		w := World{Cohorts: []PopulationCohort{{ID: "c", Population: 2, Residents: bands}}}
		if err := w.InitCohortResidents(); err == nil {
			t.Fatalf("accepted invalid census %+v", bands)
		}
	}
}

package sim

import "testing"

func TestDeepeningJailReleaseHungerReliefResidentItemConservationAndLocalPerception(t *testing.T) {
	w := Village("deepening-r1-r4")
	c := PopulationCohort{
		ID:         "cohort_square_farmers",
		Name:       "Square Farmers",
		District:   "square",
		Profession: "farmer",
		Population: 10,
		Adults:     10,
		AvgHunger:  4,
		Jailed:     2,
	}
	w.Cohorts = []PopulationCohort{c}
	w.InitCohortResidents()
	// Give anonymous residents personal items and mark 2 jailed.
	w.Cohorts[0].Residents = []ResidentBand{
		{Origin: "cohort_square_farmers", First: 1, Count: 2, Health: MaxHealth, Hunger: 4, Jailed: true, Items: map[string]int{"plank": 1}},
		{Origin: "cohort_square_farmers", First: 3, Count: 8, Health: MaxHealth, Hunger: 4, Items: map[string]int{"plank": 1}},
	}
	// Attach a jailed stored Member to verify amnesty/releaseCohortJailed never goes negative!
	stored := CloneActor(w.Actors[0])
	stored.ID = "stored_prisoner"
	stored.JailedUntil = w.Tick + 20
	w.adjustCohortMember(&w.Cohorts[0], stored, 1)
	w.Cohorts[0].Members = append(w.Cohorts[0].Members, stored)
	w.InitLedgerAudit()

	// 1. releaseCohortJailed must release only the 2 anonymous prisoners and leave the 1 stored member intact.
	released := w.releaseCohortJailed(&w.Cohorts[0], 99)
	if released != 2 {
		t.Fatalf("expected releaseCohortJailed to release 2 anonymous prisoners, got %d", released)
	}
	if w.Cohorts[0].Jailed != 1 {
		t.Fatalf("expected 1 stored jailed member to remain counted in c.Jailed, got %d", w.Cohorts[0].Jailed)
	}

	// 2. relieveCohortHunger must keep ResidentBand hunger strictly balanced.
	w.relieveCohortHunger(&w.Cohorts[0], 2)
	if err := w.VerifyHoldingBounds(); err != nil {
		t.Fatalf("VerifyHoldingBounds failed after relieveCohortHunger: %v", err)
	}

	// 3. Non-starvation cohort deaths must return ResidentBand.Items into c.ItemPool (100% ledger conserved).
	_, dead, _ := w.applyCohortCasualtiesWithCause(&w.Cohorts[0], 0, 3, 0, "combat")
	if dead != 3 {
		t.Fatalf("expected 3 combat deaths, got %d", dead)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation broken after cohort combat deaths with ResidentBand.Items: %v", err)
	}

	// 4. Local-only FlowEdges & observable sleeping state for loan candidates.
	w.FlowGrid = &FlowGridState{
		Nodes: []FlowNode{
			{ID: "n_sq", DistrictID: "square", Kind: "generator"},
			{ID: "n_far1", DistrictID: "north_woods", Kind: "generator"},
			{ID: "n_far2", DistrictID: "east_lane", Kind: "consumer"},
		},
		Edges: []FlowEdge{
			{ID: "e_local", FromNodeID: "n_sq", ToNodeID: "n_far1"},
			{ID: "e_remote", FromNodeID: "n_far1", ToNodeID: "n_far2"},
		},
	}
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Money = 2
	b.Money = 100
	b.Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 4}
	p := w.View(a.ID)
	for _, edge := range p.FlowEdges {
		if edge.ID == "e_remote" {
			t.Fatalf("Perception.FlowEdges leaked remote district edge %q", edge.ID)
		}
	}
	if peer := p.localNeighbor(b.ID); peer == nil || !peer.Sleeping {
		t.Fatalf("sleeping neighbor was not projected as asleep: %+v", peer)
	}
}

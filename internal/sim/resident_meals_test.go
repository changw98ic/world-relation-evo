package sim

import "testing"

func TestResidentDelayedRationsMatchIndividualStarvation(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		w, individual := lawWorld(), lawWorld()
		grain := w.Catalog.Items["grain"]
		grain.HungerRelief = 8
		w.Catalog.Items["grain"], individual.Catalog.Items["grain"] = grain, grain
		band1 := ResidentBand{Origin: "households", First: 1, Count: 1, Hunger: 5, Health: 20, Items: map[string]int{"grain": 1}}
		band2 := ResidentBand{Origin: "households", First: 2, Count: 1, Hunger: 5, Health: 20}
		stored := Actor{ID: "stored", AgeYears: 30, Hunger: 7, Health: MaxHealth, Items: map[string]int{"grain": 4}}
		w.Cohorts = []PopulationCohort{{ID: "households", District: "home", Population: 3, Adults: 3, Wounded: 2, AvgHunger: 5, ResidentSeq: 2, Residents: []ResidentBand{band1, band2}, Members: []Actor{stored}}}
		c := &w.Cohorts[0]
		actors := []Actor{{ID: band1.ID(0), Hunger: 5, Health: 20, Items: map[string]int{"grain": 1}}, {ID: band2.ID(0), Hunger: 5, Health: 20, Items: map[string]int{}}}
		for tick := int64(24); tick <= 48; tick++ {
			w.Tick, individual.Tick = tick-1, tick
			if delayed && tick == 25 {
				c.Residents[1].Items = map[string]int{"grain": 1}
				actors[1].Items["grain"] = 1
			}
			events := w.settleResidentMeals(c, []string{"grain"}, tick)
			for i := range actors {
				individual.advanceActorHunger(&actors[i])
				individual.consumeActorRation(&actors[i])
			}
			individual.stepActorPhysiologyAndLifecycleFor(actors, false)
			seen := map[string]ResidentBand{}
			for _, b := range c.Residents {
				for j := 0; j < b.Count; j++ {
					seen[b.ID(j)] = b
				}
			}
			for _, a := range actors {
				b, alive := seen[a.ID]
				if alive == a.Dead || alive && (a.Hunger != b.Hunger || a.Health != b.Health) {
					t.Fatalf("tick %d delayed=%v: %s actor hunger/health/dead=%d/%d/%v, band=%+v", tick, delayed, a.ID, a.Hunger, a.Health, a.Dead, b)
				}
			}
			if tick == 24 || delayed && tick == 25 {
				meal := events[0].Payload
				ranges := meal["ranges"].([]residentMealRange)
				if len(ranges) != 2 || meal["source_id"] != c.ID {
					t.Fatalf("missing exact source or ranges: %+v", meal)
				}
				if tick == 24 && (ranges[0].Intake != 1 || ranges[0].Item != "grain" || ranges[0].FirstID != actors[0].ID || ranges[1].FirstID != actors[1].ID || ranges[1].Intake != 0 || ranges[1].Reason != "no_edible_stock" || ranges[1].HealthAfter != 10) {
					t.Fatalf("one whole meal must feed one person: %+v", ranges)
				}
				if tick == 25 && (ranges[0].Reason != "not_hungry" || ranges[1].Intake != 1 || ranges[1].FirstID != actors[1].ID) {
					t.Fatalf("delayed ration: %+v", ranges)
				}
			}
			if tick == 24 {
				w = lawRestore(t, w)
				c = &w.Cohorts[0]
			}
			if tick == 48 && !delayed {
				death := events[1].Payload
				ranges := death["ranges"].([]residentMealRange)
				if death["deaths"] != 1 || len(ranges) != 1 || ranges[0].FirstID != actors[1].ID || ranges[0].HealthAfter != 0 || ranges[0].Reason != "no_edible_stock" {
					t.Fatalf("wrong starvation death evidence: %+v", death)
				}
			}
		}
		if c.Population != 3-c.CumulativeDeaths || c.CumulativeDeaths != int(w.DeathsByCause["starvation"]) || c.Members[0].Hunger != 7 || c.Members[0].Items["grain"] != 4 || c.DigestedFood != actors[0].DigestedFood+actors[1].DigestedFood {
			t.Fatalf("population or intake charged twice: %+v", c)
		}
	}
}

func TestCohortStarvationWoundsResidentsOutsideWorkforce(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{ID: "children", District: "home", Population: 2, Children: 2, AvgHunger: 5}}
	w.Tick = 23
	events := w.stepCohorts()
	summary := NewScaleSummary()
	summary.RecordEvents(events)
	c := w.Cohorts[0]
	if c.Population != 2 || c.Wounded != 2 || summary.WoundedPeople != 2 || summary.Deaths != 0 || len(c.Residents) != 1 || c.Residents[0].Health != MaxHealth-10 {
		t.Fatalf("actual hunger damage must include children: census=%+v summary=%+v", c, summary)
	}
}

func TestResidentMealEventStaysCompactAtFullAllocation(t *testing.T) {
	const population = 1000000
	w := lawWorld()
	c := PopulationCohort{ID: "village", Population: population, Adults: population, AvgHunger: 1, ItemPool: map[string]int64{"grain": population}}
	events := w.settleResidentMeals(&c, []string{"grain"}, 1)
	ranges := events[0].Payload["ranges"].([]residentMealRange)
	if len(ranges) != 1 || ranges[0].Count != population || ranges[0].Intake != 1 || c.DigestedFood != population || c.ItemPool["grain"] != 0 || len(c.Residents) != 1 || c.Residents[0].Hunger != 0 {
		t.Fatalf("full allocation must use one range: %+v", ranges)
	}
}

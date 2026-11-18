package sim

import (
	"bytes"
	"fmt"
	"testing"
)

// I1: changing the storage representation of a named living person is not a
// birth, death, transaction, amnesia, change of owner, or change of profession.
func TestLawPopulationNamedPersonSurvivesRepresentationRoundTrip(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Money, a.Energy, a.Health, a.AgeYears = 17, 37, 61, 74
	a.Items = map[string]int{"wood": 5, "grain": 2}
	a.Currencies = map[string]int64{"silver": 11}
	a.Trust["b"] = -7
	a.ProfessionXP = map[string]int{"worker": 19}
	a.Memories = []Memory{{EventID: 9, RootEventID: 8, Source: "b", Kind: "speech", Statement: &Statement{Topic: "stock", Item: "grain", Quantity: 13}}}
	w.Houses = []House{{ID: "house", District: "home", OwnerID: "a", Residents: []string{"a"}, Condition: 87}}
	before, house := lawJSON(t, *a), lawJSON(t, w.Houses)
	for round := 0; round < 4; round++ {
		if err := w.DehydrateActor("a"); err != nil {
			t.Fatal(err)
		}
		if w.TotalPopulation() != 3 || w.TotalDeaths() != 0 || w.Cohorts[0].Population != 1 || len(w.Cohorts[0].Members) != 1 || w.Cohorts[0].MoneyPool != 0 || len(w.Cohorts[0].ItemPool) != 0 {
			t.Fatal("representation created an anonymous copy or pooled personal assets")
		}
		w = lawRestore(t, w)
		restored, err := w.HydrateActor(w.Cohorts[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, lawJSON(t, *restored)) || !bytes.Equal(house, lawJSON(t, w.Houses)) || w.TotalPopulation() != 3 {
			t.Fatalf("round %d changed person or property: %+v", round, restored)
		}
	}
}

func TestLawPopulationAnonymousSplitPreservesIntegerRemainders(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Cohorts = []PopulationCohort{{ID: "group", Profession: "worker", ProfLevel: 1, District: "home", Population: 7, Adults: 7, AvgEnergy: 61, MoneyPool: 23, Currencies: map[string]int64{"silver": 31}, ItemPool: map[string]int64{"wood": 11}}}
	w.Production.EnergyRemainder = map[string]int64{"group": 3}
	for split := 1; split <= 7; split++ {
		if _, err := w.HydrateActor("group"); err != nil {
			t.Fatal(err)
		}
		c := w.Cohorts[0]
		coins, silver, wood := c.MoneyPool, c.Currencies["silver"], c.ItemPool["wood"]
		energy := int64(c.AvgEnergy*c.Population) - w.Production.EnergyRemainder["group"]
		seen := map[string]bool{}
		for _, a := range w.Actors {
			coins += int64(a.Money)
			silver += a.Currencies["silver"]
			wood += int64(a.Items["wood"])
			energy += int64(a.Energy)
			if seen[a.ID] {
				t.Fatal("one identity appeared twice")
			}
			seen[a.ID] = true
		}
		if coins != 23 || silver != 31 || wood != 11 || energy != 424 || len(w.Actors)+c.Population != 7 || w.TotalDeaths() != 0 {
			t.Fatalf("split %d changed holdings: coins=%d silver=%d wood=%d energy=%d", split, coins, silver, wood, energy)
		}
	}
}

func TestLawPopulationDeadCannotActAndEstateTransfersOnce(t *testing.T) {
	w := lawWorld()
	w.Relations = NewRelationEvoState()
	w.Relations.Kinship["a"] = &ActorKinship{ActorID: "a", ChildIDs: []string{"b"}}
	w.Actors[0].Money, w.Actors[1].Money = 17, 2
	w.Actors[0].Currencies = map[string]int64{"silver": 5}
	w.Actors[0].Items["wood"], w.Actors[1].Items["wood"] = 3, 1
	w.Actors[0].Task = &Task{Action: Action{Kind: "craft", RecipeID: "board"}, Remaining: 2, Yield: 1}
	w.Houses = []House{{ID: "home_asset", District: "home", OwnerID: "a", Condition: 71}}
	w.Actors[0].Health = 0
	deaths := w.markDeadIfZero(&w.Actors[0], nil, Action{Kind: "wait"}, "fixture injury")
	if !w.Actors[0].Dead || w.TotalPopulation() != 2 || w.TotalDeaths() != 1 || w.Actors[0].Task != nil {
		t.Fatal("death did not remove exactly one living person and their active labor")
	}
	if w.Actors[0].Money != 0 || w.Actors[1].Money != 19 || w.Actors[1].Currencies["silver"] != 5 || w.Actors[1].Items["wood"] != 4 || w.Actors[0].Items["wood"] != 0 || w.Houses[0].OwnerID != "b" || w.Houses[0].Condition != 71 {
		t.Fatal("inheritance copied, destroyed or repaired property instead of transferring ownership")
	}
	before := lawJSON(t, w)
	w.Relations.ObserveCausalRelationEvents(w, deaths[:1])
	w.markDeadIfZero(&w.Actors[0], nil, Action{}, "repeat notification")
	w.Advance("a")
	if !bytes.Equal(before, lawJSON(t, w)) {
		t.Fatal("repeated death notification or task advance settled the same estate twice")
	}
	lawRejected(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
	if w.Actors[0].Items["board"] != 0 || w.TotalDeaths() != 1 {
		t.Fatal("dead actor produced output or died twice")
	}
}

func TestLawPopulationIllnessAndImprisonmentDoNotDuplicateWorkers(t *testing.T) {
	for _, state := range []string{"healthy", "wounded", "sick", "both", "jailed"} {
		w := lawWorld()
		w.Actors = w.Actors[:1]
		w.Actors[0].ID = fmt.Sprintf("person_%s", state)
		if state == "wounded" || state == "both" {
			w.Actors[0].Health = 50
		}
		if state == "sick" || state == "both" {
			w.Actors[0].DiseaseID = "illness"
		}
		// Seed an exact member directly when jailed: entering aggregate storage is
		// forbidden while imprisoned, but saved worlds can contain such members.
		if state == "jailed" {
			w.Actors[0].JailedUntil = 10
		}
		a := w.Actors[0]
		w.Actors = nil
		c := PopulationCohort{ID: "c", District: "home", Members: []Actor{a}}
		w.adjustCohortMember(&c, a, 1)
		w.Cohorts = []PopulationCohort{c}
		anonymous := w.anonymousCohort(c)
		if anonymous.Population != 0 || anonymous.EffectivePopulation() != 0 {
			t.Fatalf("stored %s person also became anonymous labor: %+v", state, anonymous)
		}
		if w.TotalPopulation() != 1 || w.TotalDeaths() != 0 {
			t.Fatal("health status changed living identity count")
		}
		if state == "jailed" {
			if _, err := w.HydrateActor("c"); err == nil {
				t.Fatal("imprisoned exact member became a free active worker")
			}
		}
	}
}

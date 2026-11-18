package sim

import (
	"context"
	"testing"
)

// P3: representing two equivalent workers as two actors or one cohort must not
// multiply materials, shorten the declared job, or waive either worker's energy.
func TestLawProductionEquivalentWorkersHaveSameMaterialAndTimeBudget(t *testing.T) {
	individual := lawWorld()
	individual.Tick = 12 // 09:00, the configured ordinary cohort workshop shift
	individual.Actors = individual.Actors[:2]
	individual.Catalog.Recipes[0].Profession = "worker"
	for i := range individual.Actors {
		individual.Actors[i].Items["wood"] = 3
	}
	aggregate := lawRestore(t, individual)
	aggregate.Actors = nil
	aggregate.Cohorts = []PopulationCohort{{ID: "workers", Profession: "worker", ProfLevel: 1, District: "home", Population: 2, Adults: 2, AvgEnergy: 100, ItemPool: map[string]int64{"wood": 6}}}
	decider := lawDecider(func(_ context.Context, p Perception) (Action, error) {
		return Action{Kind: "craft", RecipeID: "board"}, nil
	})
	for workTick := 1; workTick <= 3; workTick++ {
		if _, err := individual.Step(context.Background(), decider); err != nil {
			t.Fatal(err)
		}
		if _, err := aggregate.Step(context.Background(), decider); err != nil {
			t.Fatal(err)
		}
		actorOutput := individual.Actors[0].Items["board"] + individual.Actors[1].Items["board"]
		cohortOutput := aggregate.Cohorts[0].ItemPool["board"]
		if workTick < 3 && (actorOutput != 0 || cohortOutput != 0) {
			t.Fatal("representation completed the three-tick job early")
		}
		if workTick == 3 && (actorOutput != 2 || cohortOutput != 2) {
			t.Fatalf("same six wood/two workers produced unequal declared output: actor=%d cohort=%d", actorOutput, cohortOutput)
		}
	}
	c := aggregate.Cohorts[0]
	if c.ItemPool["wood"] != 0 || individual.Actors[0].Items["wood"] != 0 || individual.Actors[1].Items["wood"] != 0 || c.AvgEnergy*2-int(aggregate.Production.EnergyRemainder[c.ID]) != 192 || individual.Actors[0].Energy+individual.Actors[1].Energy != 192 {
		t.Fatal("worker representation changed the six-unit material or eight-unit energy debit")
	}
}

// L1: live inventory is not shelf-stable. Food/water stock must actually pay for
// maintenance, including animals retained as parents in a growing batch.
func TestLawLivingStockConsumesSuppliesAndCannotSurviveUnfundedForever(t *testing.T) {
	for _, housed := range []string{"inventory", "retained_parent"} {
		for _, supplied := range []bool{false, true} {
			w := lawWorld()
			w.Catalog.Items["animal"] = ItemSpec{ID: "animal", MassUnits: 1, Living: &LivingRule{IntervalTicks: 1, Supplies: map[string]int64{"water": 1}, DeathAfterTicks: 2, Source: "fixture species: one water each tick; two unpaid ticks exhaust survival"}}
			w.Actors = w.Actors[:1]
			if supplied {
				w.Actors[0].Items["water"] = 4
			}
			if housed == "inventory" {
				w.Actors[0].Items["animal"] = 2
			} else {
				w.Growth = []GrowthBatch{{ID: 30, OwnerID: "a", District: "home", Parents: map[string]int64{"animal": 2}, Ready: true}}
			}
			w.maintainLivingStocks() // register the initial time of this standing stock
			for tick := int64(1); tick <= 2; tick++ {
				w.Tick = tick
				w.maintainLivingStocks()
			}
			remaining := int64(w.Actors[0].Items["animal"])
			if housed == "retained_parent" {
				remaining = w.Growth[0].Parents["animal"]
			}
			if supplied && (remaining != 2 || w.Actors[0].Items["water"] != 0) {
				t.Fatal("funded live stock did not consume its actual four-unit water budget")
			}
			if !supplied && remaining != 0 {
				t.Fatal("unfed live inventory or retained parents evaded the declared survival budget")
			}
		}
	}
}

// C1: two spellings of the same wallet still draw one wallet. Costs must be
// summed before validation; otherwise a nominally successful action overdraws it.
func TestLawCostsAliasesCannotSpendSameWalletTwice(t *testing.T) {
	for _, coins := range []int{4, 5} {
		w := lawWorld()
		w.Actors[0].Money = coins
		w.Catalog.ActionSpecs = map[string]ActionSpec{"fund": {ID: "fund", GrantedByTags: []string{"person"}, TargetKind: "self", MoneyCost: 2, CurrencyCosts: map[string]int{"coins": 3}, SlotGains: map[string]int{"focus": 1}}}
		events := w.Apply("a", Action{Kind: "fund"})
		if coins == 4 {
			lawRejected(t, events)
			if w.Actors[0].Money != 4 || w.Actors[0].StateSlots["focus"] != 0 {
				t.Fatal("insufficient combined wallet cost partially charged or granted the effect")
			}
		} else {
			lawOK(t, events)
			if w.Actors[0].Money != 0 || w.Actors[0].StateSlots["focus"] != 1 {
				t.Fatal("exact combined five-coin cost failed or paid one wallet alias twice")
			}
		}
	}
}

func TestLawCostsZeroStateRemainsExhaustedAfterSerialization(t *testing.T) {
	w := lawWorld()
	w.Catalog.StateSlots = map[string]StateSlotSpec{"focus": {ID: "focus", Min: 0, Max: 5, Default: 5}}
	w.Actors[0].StateSlots = map[string]int{"focus": 1}
	w.Catalog.ActionSpecs = map[string]ActionSpec{"focus_work": {ID: "focus_work", GrantedByTags: []string{"person"}, TargetKind: "self", SlotCosts: map[string]int{"focus": 1}, ItemCosts: map[string]int{"wood": 1}}}
	w.Actors[0].Items["wood"] = 2
	lawOK(t, w.Apply("a", Action{Kind: "focus_work"}))
	w = lawRestore(t, w)
	w.stepStateSlotsAndForms()
	lawRejected(t, w.Apply("a", Action{Kind: "focus_work"}))
	if w.Actors[0].StateSlots["focus"] != 0 || w.Actors[0].Items["wood"] != 1 {
		t.Fatal("an explicit exhausted state refilled from its initial value or charged failed work")
	}
}

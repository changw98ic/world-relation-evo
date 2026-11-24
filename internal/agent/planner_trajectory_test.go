package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPlannerValuesTheNextSkillRank(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Tags = []string{"trainee"}
	a.Skills = map[string]int{"forms": 3}
	a.SkillXP = map[string]int{"forms": 26}
	w.Catalog.TagActions["trainee"] = []string{"wait", "practice_skill"}
	w.Catalog.LevelThresholds = []int{0, 10, 25, 40}
	w.Catalog.Skills = map[string]sim.SkillSpec{"forms": {ID: "forms", MaxLevel: 4}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "skill"}, Target: 5, Scale: 5, BaseWeight: 1},
	}}
	for _, want := range []string{"practice_skill", "wait"} {
		got, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
		if err != nil || got.Kind != want || want == "practice_skill" && got.Decision.Score <= 0 {
			t.Fatalf("rank %d: chose %+v, err %v, want %s", a.Skills["forms"], got, err, want)
		}
		a.Skills["forms"], a.SkillXP["forms"] = 4, 40
	}
}

func TestPlannerValuesMaintenanceBeforeTheStateFallsBelowTarget(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Tags = []string{"caretaker"}
	w.Catalog.TagActions["caretaker"] = []string{"wait", "maintain_focus"}
	slot := sim.StateSlotSpec{ID: "focus", Min: 20, Max: 50, Default: 50, RegenPerTick: -1}
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{slot.ID: slot}
	w.Catalog.ActionSpecs = map[string]sim.ActionSpec{"maintain_focus": {
		ID: "maintain_focus", TargetKind: "self", Ticks: 4, SlotGains: map[string]int{slot.ID: 20},
	}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: 24, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "state_slot", Subject: slot.ID}, Target: 45, Scale: 10, BaseWeight: 1},
	}}
	for _, want := range []string{"maintain_focus", "wait"} {
		got, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
		if err != nil || got.Kind != want {
			t.Fatalf("change per tick %d: chose %+v, err %v, want %s", slot.RegenPerTick, got, err, want)
		}
		slot.RegenPerTick = 0
		w.Catalog.StateSlots[slot.ID] = slot
	}
}

func TestPlannerComparesFatigueBeforeTheKnownRestWindow(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Tags, a.HomeID, a.InsideHouse = []string{"restful"}, "house", "house"
	w.Houses = []sim.House{{ID: "house", District: "meeting", OwnerID: a.ID, Condition: 100}}
	w.Catalog.TagActions["restful"] = []string{"wait", "sleep"}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "energy"}, Target: 65, Scale: 40, BaseWeight: 1},
	}}
	w.Tick = 6 * sim.TicksPerHour // Noon. The next preferred sleep window begins at 22:00.
	for _, tc := range []struct {
		energy int
		want   string
	}{{95, "wait"}, {70, "sleep"}} {
		a.Energy = tc.energy
		got, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
		if err != nil || got.Kind != tc.want {
			t.Fatalf("energy %d: chose %+v, err %v, want %s", tc.energy, got, err, tc.want)
		}
	}
	w.Tick, a.Energy = sim.TicksPerDay-2, 4
	w.Catalog.TagActions["restful"] = append(w.Catalog.TagActions["restful"], "practice_skill")
	a.Skills, a.SkillXP = map[string]int{"forms": 1}, map[string]int{"forms": 0}
	w.Catalog.Skills = map[string]sim.SkillSpec{"forms": {ID: "forms", MaxLevel: 2}}
	w.Catalog.Planning.Needs = append(w.Catalog.Planning.Needs,
		sim.NeedRule{NeedKey: sim.NeedKey{Measure: "skill"}, Scale: 5, BaseWeight: 0.4})
	got, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
	if err != nil || got.Kind != "sleep" {
		t.Fatalf("a nearby clock boundary must not make exertion at four energy worthwhile: chose %+v, err %v", got, err)
	}
}

func TestPlannerUsesPreviouslyObservedShelterAcrossTheRestWindow(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Tags, a.HomeID, a.InsideHouse, a.Location = []string{"restful"}, "house", "house", "remote"
	a.SleepTrait = sim.SleepLight
	w.Houses = []sim.House{{ID: "house", District: "remote", OwnerID: a.ID, Condition: 100, Level: 3, Comfort: 3}}
	w.Map.Roads = []sim.Road{{From: "meeting", To: "remote", Ticks: 1, EnergyCost: 2}}
	w.Catalog.TagActions["restful"] = []string{"wait", "sleep", "move", "enter_house"}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "energy"}, Target: 65, Scale: 40, BaseWeight: 1},
	}}
	planner := Planner{}
	atHome, err := planner.Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	boundaryApply(t, w, a.ID, atHome)
	a.Location, a.InsideHouse, a.Energy = "meeting", "", 57
	w.Tick = sim.TicksPerDay - 1 // Fifteen minutes before dawn.
	w.Houses[0].Condition = 0    // A remote change that the actor has not seen.
	view := w.View(a.ID)
	got, err := planner.Decide(context.Background(), view)
	if err != nil || got.Kind != "move" || got.Location != "remote" || len(got.Decision.Behavior.Steps) != 2 {
		t.Fatalf("reachable shelter should beat rough sleep when already tired: chose %+v, err %v", got, err)
	}
}

func TestPlannerWorksTowardAnObservedMarketSale(t *testing.T) {
	w := boundaryWorld()
	w.Actors = w.Actors[:1]
	a := &w.Actors[0]
	a.Tags, a.Money, a.Items = []string{"maker"}, 2, map[string]int{"ore": 1}
	w.Catalog.TagActions["maker"] = []string{"wait", "craft", "market_sell"}
	w.Catalog.Items["ore"] = sim.ItemSpec{ID: "ore", MassUnits: 1}
	w.Catalog.Items["nails"] = sim.ItemSpec{ID: "nails", MassUnits: 1, Tradeable: true, BatchQty: 1, BatchPrice: 8}
	w.Catalog.Recipes = []sim.Recipe{{ID: "make_nails", OutputItem: "nails", OutputQty: 1, Ticks: 2, MoneyCost: 2,
		Inputs: map[string]int{"ore": 1}, Conversion: &sim.MaterialConversion{Source: "fixture: one unit of ore becomes one parcel of nails"}}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay,
		Needs: []sim.NeedRule{{NeedKey: sim.NeedKey{Measure: "money"}, Target: 8, Scale: 8, BaseWeight: 1}}}
	w.Cohorts = []sim.PopulationCohort{{ID: "shop", District: "meeting", Profession: "peddler", Population: 1, Adults: 1,
		AvgEnergy: 100, ItemPool: map[string]int64{}}}
	planner := Planner{}
	got, err := planner.Decide(context.Background(), w.View(a.ID))
	if err != nil || got.Kind != "wait" {
		t.Fatalf("without a funded buyer: chose %+v, err %v", got, err)
	}
	w.Cohorts[0].MoneyPool = 20
	w.InitLedgerAudit()
	var selected []string
	for range 3 {
		_, log, err := w.StepRecorded(context.Background(), planner)
		if err != nil {
			t.Fatal(err)
		}
		if action, acted := log.Decisions[a.ID]; acted {
			selected = append(selected, action.Kind)
		}
	}
	if len(selected) != 2 || selected[0] != "craft" || selected[1] != "market_sell" ||
		a.Money != 8 || a.Items["nails"] != 0 || w.Cohorts[0].MoneyPool != 12 || w.Cohorts[0].ItemPool["nails"] != 1 {
		t.Fatalf("sale did not settle: %v, producer %d coins and %d nails, shop %d coins and %d nails",
			selected, a.Money, a.Items["nails"], w.Cohorts[0].MoneyPool, w.Cohorts[0].ItemPool["nails"])
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPlannerCountsBookedWagesFromTheirPayday(t *testing.T) {
	w := boundaryWorld()
	teacher := &w.Actors[0]
	teacher.Tags, teacher.Money = []string{"teacher"}, 2
	teacher.Skills = map[string]int{"forms": 3}
	w.Catalog.Skills = map[string]sim.SkillSpec{"forms": {ID: "forms", Name: "Forms", MaxLevel: 4}}
	w.Catalog.TagActions["teacher"] = []string{"wait", "teach_knowledge"}
	w.Catalog.WorkOrders = []sim.WorkOrder{{ID: "lessons", Name: "Lessons", EmployerID: "hall",
		ActionKind: "teach_knowledge", SkillID: "forms", CompletionKind: "knowledge_taught",
		District: "meeting", Wage: 6, PayCycle: sim.PayWeekly}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "money"}, Target: 8, Scale: 8, BaseWeight: 1},
	}}
	w.Cohorts = []sim.PopulationCohort{{ID: "hall", District: "meeting", Population: 1, MoneyPool: 6}}
	planner := Planner{}
	action, err := planner.Decide(context.Background(), w.View(teacher.ID))
	if err != nil || action.Kind != "teach_knowledge" || action.WorkOrderID != "lessons" {
		t.Fatalf("before doing the work: chose %+v, err %v", action, err)
	}
	boundaryApply(t, w, teacher.ID, action)
	if teacher.Money != 2 || len(w.EarnedWages) != 1 || w.EarnedWages[0].DueTick <= w.Tick+sim.TicksPerDay {
		t.Fatalf("weekly work must record a future pay claim: cash %d, claims %+v", teacher.Money, w.EarnedWages)
	}
	if len(w.View("bob").PendingWages) != 0 {
		t.Fatal("the worker's pay claim was visible to a third party")
	}
	w.Tick++
	action, err = planner.Decide(context.Background(), w.View(teacher.ID))
	if err != nil || action.Kind != "wait" {
		t.Fatalf("the earned wage already covers the payday goal: chose %+v, err %v", action, err)
	}
}

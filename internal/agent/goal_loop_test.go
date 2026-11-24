package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestSavingsGoalValuesIncomeBeyondTheReserve(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Money, a.Items["food"] = 46, 342
	a.Ambitions = []sim.ActorAmbition{{ID: "savings", Kind: "accumulate_wealth", TargetValue: 80}}
	w.Actors[1].Items["food"] = 2
	offered := w.Apply("bob", sim.Action{Kind: "offer", TargetID: a.ID, Item: "food", Quantity: 2, Price: 5})
	if len(offered) != 1 || offered[0].Kind != "trade_offered" {
		t.Fatalf("purchase fixture did not post a quote: %+v", offered)
	}
	w.Cohorts = []sim.PopulationCohort{{ID: "shop", District: a.Location, Profession: "peddler", Population: 1, Adults: 1, AvgEnergy: 3, MoneyPool: 10}}
	p := w.View(a.ID)
	rules := p.Catalog.EffectivePlanningRules()
	rows, _, err := buildNeeds(p, rules)
	if err != nil {
		t.Fatal(err)
	}
	moneyRows := 0
	for _, row := range rows {
		if row.Measure == "money" {
			moneyRows++
			if row.level != 46 || row.Target != 80 {
				t.Fatalf("cash row = %+v, want 46 of 80", row)
			}
		}
	}
	if moneyRows != 1 {
		t.Fatalf("scored %d cash rows", moneyRows)
	}
	frame := projectFacts(p)
	buy, _ := evaluate(rows, frame.forecast(sim.Candidate{Action: sim.Action{Kind: "accept", OfferID: w.Offers[0].ID}}), rules, true)
	if buy >= 0 {
		t.Fatalf("the unnecessary purchase earned %g", buy)
	}
	action, err := (Planner{}).Decide(context.Background(), p)
	if err != nil || action.Kind != "market_sell" || action.Decision.Score <= 0 {
		t.Fatalf("available income did not advance savings: %+v, %v", action, err)
	}
	w.InitLedgerAudit()
	events := w.Apply(a.ID, action)
	if len(events) != 1 || events[0].Kind != "trade_completed" || a.Money != 46+action.Price {
		t.Fatalf("planned income was not settled: %+v, balance=%d", events, a.Money)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPostedSaleAdvancesThePeddlerCareer(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Professions = map[string]int{"peddler": 1}
	food := w.Catalog.Items["food"]
	food.Tier, food.BatchQty = 3, 2
	w.Catalog.Items["food"] = food
	target := w.Catalog.LevelThresholds[1]
	a.ProfessionXP = map[string]int{"peddler": target - 6}
	a.Ambitions = []sim.ActorAmbition{{ID: "career", Kind: "advance_profession", TargetID: "peddler", TargetValue: target}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "profession_xp"}, Scale: 6, BaseWeight: 1},
	}}
	w.Cohorts = []sim.PopulationCohort{{ID: "shop", District: a.Location, Profession: "peddler", Population: 1, Adults: 1, AvgEnergy: 3, MoneyPool: 100}}
	w.InitLedgerAudit()
	before := a.Items["food"]
	events, err := w.Step(context.Background(), Planner{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Items["food"] != before-2 || a.ProfessionXP["peddler"] != target || !a.Ambitions[0].Completed {
		t.Fatalf("the career need did not lead to a settled sale and promotion: %+v, events %v", a, events)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestInertiaDoesNotRepeatACompletedRationPurchase(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Items["food"], a.Money = 10, 42
	w.Cohorts = []sim.PopulationCohort{{ID: "shop", District: a.Location, Profession: "peddler",
		Population: 1, Adults: 1, AvgEnergy: 3, ItemPool: map[string]int64{"food": 10}}}
	view := w.View(a.ID)
	var buy sim.Action
	for _, lot := range view.MarketLots {
		if lot.Kind == "market_buy" && lot.Item == "food" {
			buy = lot
			break
		}
	}
	if buy.Kind == "" {
		t.Fatal("valid posted food lot is missing")
	}
	previous := projectFacts(view).forecast(sim.Candidate{Action: buy})
	a.Behavior = &sim.BehaviorState{Goal: previous.goal, Strategy: previous.strategy, ReviewAt: w.Tick}
	action, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
	if err != nil || action.Kind != "wait" {
		t.Fatalf("completed food plan got %+v, %v", action, err)
	}
}

func careerFixture(t *testing.T) *sim.World {
	t.Helper()
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Professions = map[string]int{"martial_artist": 1}
	a.Ambitions = []sim.ActorAmbition{{ID: "training", Kind: "advance_profession", TargetID: "martial_artist", TargetValue: 6}}
	w.Catalog.Recipes = []sim.Recipe{{
		ID: "write_manual", Profession: "martial_artist", MinProfessionLevel: 1, Ticks: 2, Inputs: map[string]int{"paper": 1},
		OutputItem: "martial_manual", OutputQty: 1, Conversion: &sim.MaterialConversion{Source: "one sheet becomes the written lesson"},
	}}
	return w
}

func TestKnownMaterialsAndMealInterruptionKeepTheSameCareerGoal(t *testing.T) {
	w := careerFixture(t)
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "nutrition"}, Target: 0, Scale: 1, BaseWeight: 20},
		{NeedKey: sim.NeedKey{Measure: "profession_xp"}, Scale: 6, BaseWeight: 1},
	}}
	w.Actors[1].Items["paper"] = 1
	w.Apply("bob", sim.Action{Kind: "offer", TargetID: "alice", Item: "paper", Quantity: 1, Price: 1})
	choose := func(want string) sim.Action {
		t.Helper()
		action, err := (Planner{}).Decide(context.Background(), w.View("alice"))
		if err != nil || action.Kind != want {
			t.Fatalf("got %+v, %v, want %s", action, err, want)
		}
		return action
	}
	a := &w.Actors[0]
	acquire := choose("accept")
	if acquire.Decision.Behavior.Goal.Measure != "profession_xp" || len(acquire.Decision.Behavior.Steps) != 1 {
		t.Fatalf("buying paper did not preserve the following lesson: %+v", acquire.Decision.Behavior)
	}
	w.Apply(a.ID, acquire)
	if a.ProfessionXP["martial_artist"] != 0 || a.Items["paper"] != 1 {
		t.Fatalf("preparation settled as training or failed to receive the paper: %+v", a)
	}
	a.Hunger = 1
	w.Tick++
	eat := choose("eat")
	w.Apply(a.ID, eat)
	if a.Behavior.Paused == nil || a.Behavior.Paused.Goal != acquire.Decision.Behavior.Goal {
		t.Fatalf("meal lost the career goal: %+v", a.Behavior)
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	w, err = sim.DecodeWorldSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	w.Tick++
	a = &w.Actors[0]
	craft := choose("craft")
	if craft.Decision.Reason != "behavior_resumed" {
		t.Fatalf("expected saved preparation to resume, got %+v", craft.Decision)
	}
	w.InitLedgerAudit()
	w.Apply(a.ID, craft)
	if a.ProfessionXP["martial_artist"] != 0 {
		t.Fatal("starting the lesson advanced its outcome")
	}
	for range 3 {
		if _, err := w.Step(context.Background(), Planner{}); err != nil {
			t.Fatal(err)
		}
	}
	if a.ProfessionXP["martial_artist"] != 6 || !a.Ambitions[0].Completed {
		t.Fatalf("settled training did not advance the career goal: %+v, %+v", a.ProfessionXP, a.Ambitions)
	}
}

func TestPreferencesDecideBetweenBeneficialCareAndTraining(t *testing.T) {
	w := careerFixture(t)
	a := &w.Actors[0]
	a.Tags = append(a.Tags, "healer")
	a.Items["paper"], a.Items["medicine"] = 1, 1
	w.Actors[1].Health = 60
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "profession_xp"}, Scale: 6, BaseWeight: 1},
		{NeedKey: sim.NeedKey{Measure: "relationship"}, Target: 20, Scale: 20, BaseWeight: 1},
	}}
	for _, choice := range []struct{ suppressed, kind string }{{"relationship", "craft"}, {"profession_xp", "heal"}} {
		a.NeedWeights = map[string]float64{choice.suppressed: 0}
		action, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
		if err != nil || action.Kind != choice.kind || action.Decision.Score <= 0 {
			t.Fatalf("same facts with zero %s weight: got %+v, %v, want %s", choice.suppressed, action, err, choice.kind)
		}
	}
}

type passiveDecider struct{}

func (passiveDecider) Decide(context.Context, sim.Perception) (sim.Action, error) {
	return sim.Action{Kind: "wait"}, nil
}

func TestObservedExpiryRetiresTheSameQuoteAndAllowsNewDemand(t *testing.T) {
	w := boundaryWorld()
	w.Actors[0].Items["paper"] = 2
	w.Actors[1].Professions = map[string]int{"scholar": 1}
	q := sim.Action{Kind: "offer", TargetID: "bob", Item: "paper", Quantity: 1, Price: 2}
	chance := func() float64 { return projectFacts(w.View("alice")).exchangeLikelihood(q, true) }
	if chance() <= 0 {
		t.Fatal("a scholar could not be approached with a documented supply")
	}
	w.Apply("alice", q)
	if len(w.Offers) != 1 || chance() != 0 {
		t.Fatal("an outstanding quote was counted as a new opportunity")
	}
	w.Tick = w.Offers[0].Expires
	if _, err := w.Step(context.Background(), passiveDecider{}); err != nil {
		t.Fatal(err)
	}
	if len(w.Offers) != 0 || chance() != 0 {
		t.Fatal("the expired quote became a fresh opportunity")
	}
	q.Price--
	if chance() <= 0 {
		t.Fatal("the old result blocked a genuine change of terms")
	}
	w.Apply("alice", q)
	response := sim.Action{Kind: "offer", TargetID: "alice", OfferID: w.Offers[0].ID, Item: q.Item, Quantity: q.Quantity, Price: 2}
	events := w.Apply("bob", response)
	if len(events) != 1 || events[0].Kind != "trade_counteroffered" {
		t.Fatalf("buyer response did not open new demand: %+v", events)
	}
	q.Price = 2
	if chance() <= 0 {
		t.Fatal("the newly requested terms did not supersede observed expiry")
	}
	events = w.Apply("alice", sim.Action{Kind: "accept", OfferID: w.Offers[0].ID})
	if len(events) != 1 || events[0].Kind != "trade_completed" || w.Actors[1].Items["paper"] != 1 {
		t.Fatalf("the new quotation did not settle: %+v", events)
	}
}

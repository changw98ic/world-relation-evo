package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPlannerUsesTheSameFactsWithDifferentNeedWeights(t *testing.T) {
	world := boundaryWorld()
	actor := &world.Actors[0]
	actor.Tags, actor.Professions = []string{"rest_and_food"}, nil
	world.Catalog.TagActions["rest_and_food"] = []string{"wait", "eat", "sleep"}
	actor.Hunger, actor.Energy, actor.Health = 4, 18, 100
	world.Catalog.Planning = &sim.PlanningRules{
		HorizonTicks: sim.TicksPerDay,
		Needs: []sim.NeedRule{
			{NeedKey: sim.NeedKey{Measure: "nutrition"}, Scale: 4, BaseWeight: 1},
			{NeedKey: sim.NeedKey{Measure: "energy"}, Target: 70, Scale: 50, BaseWeight: 1},
		},
	}
	for _, test := range []struct {
		zero string
		want string
	}{{"energy", "eat"}, {"nutrition", "sleep"}} {
		actor.NeedWeights = map[string]float64{test.zero: 0}
		action, err := (Planner{}).Decide(context.Background(), world.View(actor.ID))
		if err != nil || action.Kind != test.want {
			t.Fatalf("zero %s: got %+v, %v, want %s", test.zero, action, err, test.want)
		}
	}
}

func TestObservedHealthOfTheNeedSubjectDeterminesTreatment(t *testing.T) {
	w := boundaryWorld()
	actor, patient := &w.Actors[0], &w.Actors[1]
	actor.Tags = []string{"medic"}
	actor.Items["medicine"] = 1
	w.Catalog.TagActions["medic"] = []string{"wait", "heal"}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "health", Subject: patient.ID}, Target: 100, Scale: 50, BaseWeight: 1},
	}}
	patient.Health = 50
	for _, test := range []struct{ location, want string }{{"meeting", "heal"}, {"remote", "wait"}} {
		patient.Location = test.location
		action, err := (Planner{}).Decide(context.Background(), w.View(actor.ID))
		if err != nil || action.Kind != test.want || (action.Kind == "heal" && action.TargetID != patient.ID) {
			t.Fatalf("patient in %s: got %+v, %v, want %s", test.location, action, err, test.want)
		}
	}
}

func TestPlanningFindsWaterAndSettlesAnActualDrink(t *testing.T) {
	w := sim.Village("water-strategy")
	w.Actors = w.Actors[:1]
	a := &w.Actors[0]
	a.Location, a.InsideHouse, a.Thirst = "south_fields", "", 5
	a.Items = map[string]int{"food": 20}
	w.Tick = 1
	w.InitLedgerAudit()
	for range 8 {
		events, err := w.Step(context.Background(), Planner{})
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == "drank" {
				if !w.View(a.ID).HasPotableWater() || a.Thirst != 0 || a.Location == "south_fields" {
					t.Fatalf("water was not consumed at the destination: %s, thirst %d", a.Location, a.Thirst)
				}
				if err := w.VerifyLedgerConservation(); err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
	t.Fatal("the thirsty actor did not reach a known source and drink")
}

func TestPlannerKnownPriceChangesTheDecision(t *testing.T) {
	w := boundaryWorld()
	actor := &w.Actors[1]
	actor.Tags, actor.Professions = []string{"diner"}, nil
	w.Catalog.TagActions["diner"] = []string{"wait", "eat", "accept"}
	actor.Items, actor.Hunger, actor.Money = map[string]int{}, 3, 20
	w.Catalog.Planning = &sim.PlanningRules{
		HorizonTicks: sim.TicksPerDay,
		Needs: []sim.NeedRule{
			{NeedKey: sim.NeedKey{Measure: "nutrition"}, Scale: 8, BaseWeight: 1},
			{NeedKey: sim.NeedKey{Measure: "money"}, Target: 20, Scale: 5, BaseWeight: 1},
		},
	}
	w.Offers = []sim.Offer{{ID: 20, Seller: "alice", Buyer: actor.ID, Item: "food", Quantity: 1, Price: 0, Expires: 12}}
	for _, test := range []struct {
		price int
		want  string
	}{{0, "accept"}, {20, "wait"}} {
		w.Offers[0].Price = test.price
		view := w.View(actor.ID)
		if !view.CheckAction(sim.Action{Kind: "accept", OfferID: 20, TargetID: "alice"}).Allowed {
			t.Fatalf("offered price %d should be affordable", test.price)
		}
		action, err := (Planner{}).Decide(context.Background(), view)
		if err != nil || action.Kind != test.want {
			t.Fatalf("price %d: got %+v, %v, want %s", test.price, action, err, test.want)
		}
	}
}

func TestPlannerUsesObservedPaymentFailureUntilItSeesNewFunds(t *testing.T) {
	w := boundaryWorld()
	supplier, buyer := &w.Actors[0], &w.Actors[1]
	supplier.Money, buyer.Money = 10, 0
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "money"}, Target: 20, Scale: 20, BaseWeight: 1},
	}}
	apply := func(actor string, action sim.Action, want string) {
		t.Helper()
		events := w.Apply(actor, action)
		if len(events) == 0 || events[0].Kind != want {
			t.Fatalf("%s %s: got %+v, want %s", actor, action.Kind, events, want)
		}
	}
	choose := func(want string) sim.Action {
		t.Helper()
		action, err := (Planner{}).Decide(context.Background(), w.View("alice"))
		if err != nil || action.Kind != want {
			t.Fatalf("got %+v, %v, want %s", action, err, want)
		}
		return action
	}
	buyer.Items["food"] = 1
	apply("alice", sim.Action{Kind: "propose_agreement", TargetID: "bob", Item: "food", Quantity: 1, Price: 4}, "agreement_proposed")
	apply("bob", sim.Action{Kind: "accept_agreement", TargetID: "alice", AgreementID: 1}, "agreement_accepted")
	apply("alice", choose("fulfill_agreement"), "action_failed")
	if view := w.View("alice"); !view.ObservedPaymentFailure("bob", 4) || view.ObservedPaymentFailure("bob", 3) {
		t.Fatal("the receipt should establish only the failed price")
	}
	if w.View("charlie").ObservedPaymentFailure("bob", 4) {
		t.Fatal("the private receipt reached a third party")
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	w, err = sim.DecodeWorldSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if lower := choose("offer"); lower.Price >= 4 {
		t.Fatalf("after a failed payment the alternative must use a lower price: %+v", lower)
	}
	apply("bob", sim.Action{Kind: "offer", TargetID: "alice", Item: "food", Quantity: 1, Price: 4}, "trade_offered")
	apply("alice", sim.Action{Kind: "accept", TargetID: "bob", OfferID: w.Offers[0].ID}, "trade_completed")
	apply("alice", choose("fulfill_agreement"), "agreement_fulfilled")
}

func TestPlannerPreparesTheGoodsForASignedDelivery(t *testing.T) {
	w := boundaryWorld()
	supplier, buyer := &w.Actors[0], &w.Actors[1]
	supplier.Tags, supplier.Items, supplier.Money = []string{"supplier"}, map[string]int{"food": 2}, 10
	w.Catalog.TagActions["supplier"] = []string{"wait", "craft", "propose_agreement", "fulfill_agreement"}
	w.Catalog.Items["packed_meal"] = sim.ItemSpec{ID: "packed_meal", Tradeable: true, Edible: true, MassUnits: 1, NutritionUnits: 1}
	w.Catalog.Recipes = []sim.Recipe{{ID: "pack_meals", OutputItem: "packed_meal", OutputQty: 2,
		Ticks: 2, Inputs: map[string]int{"food": 2}, Conversion: &sim.MaterialConversion{Source: "fixture: two rations become two packed meals"}}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "obligation"}, Target: 1, Scale: 1, BaseWeight: 1},
	}}
	due := w.Tick + 2
	boundaryApply(t, w, supplier.ID, sim.Action{Kind: "propose_agreement", TargetID: buyer.ID,
		Item: "packed_meal", Quantity: 2, Price: 4, DueTick: &due})
	boundaryApply(t, w, buyer.ID, sim.Action{Kind: "accept_agreement", TargetID: supplier.ID, AgreementID: 1})
	w.InitLedgerAudit()
	var selected []string
	for range 3 {
		_, log, err := w.StepRecorded(context.Background(), Planner{})
		if err != nil {
			t.Fatal(err)
		}
		if action, acted := log.Decisions[supplier.ID]; acted {
			selected = append(selected, action.Kind)
		}
	}
	if len(selected) != 2 || selected[0] != "craft" || selected[1] != "fulfill_agreement" ||
		supplier.Money != 14 || supplier.Items["food"] != 0 || buyer.Items["packed_meal"] != 2 {
		t.Fatalf("work and signed delivery did not settle: %v, balance %d, inputs %d, delivered %d",
			selected, supplier.Money, supplier.Items["food"], buyer.Items["packed_meal"])
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPlannerAppliesInertiaOnlyWhenTheOldPlanStillWorks(t *testing.T) {
	w := boundaryWorld()
	actor := &w.Actors[0]
	actor.Tags, actor.Professions = []string{"diner"}, nil
	w.Catalog.TagActions["diner"] = []string{"wait", "eat"}
	actor.Hunger, actor.NutritionReserve = 4, 0
	actor.Items = map[string]int{"corn": 1, "potato": 1}
	w.Catalog.Items["corn"] = sim.ItemSpec{ID: "corn", Edible: true, NutritionUnits: 1}
	potato := sim.ItemSpec{ID: "potato", Edible: true}
	actor.Behavior = &sim.BehaviorState{Goal: sim.NeedKey{Measure: "nutrition"}, Strategy: "corn_plan", Version: 7,
		Steps: []sim.Action{{Kind: "eat", Item: "corn"}}, ReviewAt: w.Tick}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, InertiaMargin: 0.5,
		Needs: []sim.NeedRule{{NeedKey: sim.NeedKey{Measure: "nutrition"}, Scale: 5, BaseWeight: 1}}}
	zero := 0.0
	for _, test := range []struct {
		potato int64
		corn   int
		margin *float64
		want   string
	}{{2, 1, nil, "corn"}, {6, 1, nil, "potato"}, {2, 0, nil, "potato"}, {2, 1, &zero, "potato"}} {
		potato.NutritionUnits = test.potato
		w.Catalog.Items["potato"] = potato
		actor.Items["corn"] = test.corn
		actor.InertiaMargin = test.margin
		action, err := (Planner{}).Decide(context.Background(), w.View(actor.ID))
		if err != nil || action.Kind != "eat" || action.Item != test.want {
			t.Fatalf("potato=%d corn=%d: got %+v, %v, want %s", test.potato, test.corn, action, err, test.want)
		}
	}
}

func TestPlannerEvaluatesTransitionRiskAndSettlesTheChosenForm(t *testing.T) {
	w := boundaryWorld()
	w.Actors = w.Actors[:1]
	a := &w.Actors[0]
	a.FormID, a.Tags = "learner", []string{"learner"}
	w.Catalog.TagActions["learner"] = []string{"wait", "attempt_transition"}
	w.Catalog.Forms = map[string]sim.FormSpec{
		"learner": {ID: "learner", PowerTier: 1},
		"adept":   {ID: "adept", PowerTier: 2},
	}
	transition := sim.TransitionSpec{ID: "advance", RequiredFormID: "learner", SuccessFormID: "adept",
		Ticks: 2, ItemCosts: map[string]int{"food": 1}, FailureHealthLoss: 20}
	w.Catalog.Transitions = map[string]sim.TransitionSpec{"advance": transition}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "power_tier"}, Target: 2, Scale: 1, BaseWeight: 1},
		{NeedKey: sim.NeedKey{Measure: "health"}, Target: 100, Scale: 20, BaseWeight: 1},
	}}
	action, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
	if err != nil || action.Kind != "wait" {
		t.Fatalf("guaranteed failure should keep the current form: %+v, %v", action, err)
	}
	transition.BaseSuccessPermille = 1000
	w.Catalog.Transitions["advance"] = transition
	w.InitLedgerAudit()
	for range 2 {
		if _, err := w.Step(context.Background(), Planner{}); err != nil {
			t.Fatal(err)
		}
	}
	if a.FormID != "adept" || a.Items["food"] != 19 {
		t.Fatalf("the transition did not settle its form and material: %s, %d", a.FormID, a.Items["food"])
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduledPlanningAndPreferenceStateSurviveRestart(t *testing.T) {
	w := sim.Village("preference-restart")
	w.Actors = w.Actors[:1]
	w.Actors[0].NeedWeights = map[string]float64{"hygiene": 0}
	w.RunSeed = 17
	rules := sim.DefaultPlanningRules()
	rules.Needs[0].Volatility, rules.Needs[0].CorrelationTicks = 0.2, 20
	rules.Experiences = []sim.ExperienceRule{{Event: "ate", Need: rules.Needs[0].NeedKey, Bias: 0.2, RecoveryTicks: 20}}
	w.Catalog.Planning = &rules
	w.InitLedgerAudit()
	checkpoint, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var logs []sim.TickDecisionLog
	for range 72 {
		_, log, err := w.StepRecorded(context.Background(), Planner{})
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
		if w.Tick == 36 {
			raw, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			w, err = sim.DecodeWorldSnapshot(raw)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(w.Actors[0].Preferences) != 1 || w.Actors[0].NextWake == nil {
		t.Fatal("decision state or the next wake was lost")
	}
	restored, _, err := sim.ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(restored)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(w)
	if err != nil || string(got) != string(want) {
		t.Fatalf("recorded planning decisions did not reproduce restored world: %v", err)
	}
}

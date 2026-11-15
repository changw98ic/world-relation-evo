package sim

import (
	"encoding/json"
	"testing"
)

func continuityWorld() *World {
	c := DefaultCatalog()
	c.Recipes = []Recipe{{ID: "make_food", OutputItem: "food", OutputQty: 2, Ticks: 2}}
	c.Items["food"] = ItemSpec{ID: "food", Tradeable: true, BatchQty: 2, BatchPrice: 3}
	return &World{ID: "continuity", Catalog: c, Actors: []Actor{
		{ID: "a", Name: "Supplier", Tags: []string{"resident", "trader"}, Items: map[string]int{"food": 4}, Money: 10, Trust: map[string]int{}, Location: "village", Energy: 100},
		{ID: "b", Name: "Buyer", Tags: []string{"resident", "trader"}, Items: map[string]int{}, Money: 10, Trust: map[string]int{}, Location: "village", Energy: 100},
		{ID: "c", Name: "Other", Tags: []string{"resident", "trader"}, Items: map[string]int{}, Money: 10, Trust: map[string]int{}, Location: "elsewhere", Energy: 100},
	}}
}
func assertContinuityEvent(t *testing.T, events []Event, kind string) {
	t.Helper()
	if len(events) != 1 || events[0].Kind != kind {
		t.Fatalf("wanted %s, got %+v", kind, events)
	}
}
func TestAgreementRequiresBilateralDecisionAndActualDelivery(t *testing.T) {
	w := continuityWorld()
	a, b := &w.Actors[0], &w.Actors[1]
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "food"}), "agreement_proposed")
	g := w.Continuity.Agreements[0]
	if g.Status != "proposed" || a.Items["food"] != 4 || b.Money != 10 {
		t.Fatal("proposal changed economic state")
	}
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: g.ID}), "action_failed")
	assertContinuityEvent(t, w.Apply("c", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: g.ID}), "action_failed")
	assertContinuityEvent(t, w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: g.ID}), "agreement_accepted")
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "I have delivered everything."}), "speech")
	if w.Continuity.Agreements[0].Status != "active" || a.Items["food"] != 4 {
		t.Fatal("speech settled agreement")
	}
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: g.ID}), "agreement_fulfilled")
	if a.Items["food"] != 2 || b.Items["food"] != 2 || a.Money != 13 || b.Money != 7 || w.Continuity.Agreements[0].Status != "fulfilled" {
		t.Fatal("delivery did not settle exact catalog terms")
	}
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: g.ID}), "action_failed")
	if len(w.ContinuityFor("c").Dialogue) != 0 || len(w.ContinuityFor("c").Agreements) != 0 {
		t.Fatal("private agreement leaked")
	}
	if len(w.ContinuityFor("b").Dialogue) != 4 {
		t.Fatal("conversation did not preserve exchanges")
	}
}
func TestAgreementRejectionExpiryAndFailedDelivery(t *testing.T) {
	w := continuityWorld()
	w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "food"})
	assertContinuityEvent(t, w.Apply("b", Action{Kind: "reject_agreement", TargetID: "a", AgreementID: 1}), "agreement_rejected")
	if w.Actors[0].Items["food"] != 4 || w.Actors[1].Money != 10 {
		t.Fatal("rejection transferred assets")
	}
	w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "food"})
	w.Tick = TicksPerDay + 1
	assertContinuityEvent(t, w.AdvanceContinuity(), "agreement_expired")
	w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "food"})
	w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: 3})
	w.Actors[1].Money = 0
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "fulfill_agreement", TargetID: "b", AgreementID: 3}), "action_failed")
	if w.Actors[0].Items["food"] != 4 || w.Continuity.Agreements[2].Status != "active" {
		t.Fatal("unfunded exchange partially settled")
	}
	w.Tick = w.Continuity.Agreements[2].DueTick + 1
	assertContinuityEvent(t, w.AdvanceContinuity(), "agreement_unfulfilled")
	if w.Continuity.Agreements[2].Status != "unfulfilled" {
		t.Fatal("deadline claimed completion")
	}
}
func TestPersonalPlanSurvivesDayAndRoundTripCompletesOnlyRealProgress(t *testing.T) {
	w := continuityWorld()
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "start_plan", RecipeID: "make_food"}), "plan_started")
	w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "I have finished my production plan"})
	if w.Actors[0].Plan.Produced != 0 {
		t.Fatal("speech advanced plan")
	}
	w.Tick = TicksPerDay + 1
	// A completed task, not a proposed or attempted craft, progresses the plan.
	w.Actors[0].Task = &Task{Action: Action{Kind: "craft", RecipeID: "make_food"}, Remaining: 1, Yield: 2}
	w.Advance("a")
	if w.Actors[0].Plan.Stage != "sell" {
		t.Fatalf("craft did not advance plan: %+v", w.Actors[0].Plan)
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var restored World
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	w = &restored
	if w.Actors[0].Plan.Produced != 2 || w.Actors[0].Plan.Status != "active" {
		t.Fatal("plan lost on reload")
	}
	// An offer is not a completed sale.
	events := w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "food", Quantity: 2, Price: 3})
	if w.Actors[0].Plan.Status != "active" {
		t.Fatal("offer completed plan")
	}
	w.Apply("b", Action{Kind: "accept", OfferID: events[0].ID})
	if w.Actors[0].Plan.Status != "completed" || w.Actors[0].Plan.Sold != 2 {
		t.Fatalf("real trade failed to complete plan: %+v", w.Actors[0].Plan)
	}
}
func TestPersonalPlanMayBeInterruptedOrAbandoned(t *testing.T) {
	w := continuityWorld()
	w.Apply("a", Action{Kind: "start_plan", RecipeID: "make_food"})
	plan := ClonePersonalPlan(w.Actors[0].Plan)
	plan.Item = "other"
	if w.Actors[0].Plan.Item != "food" {
		t.Fatal("plan view aliases world")
	}
	w.Actors[0].Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 1, StartEnergy: 100}
	w.Tick = TicksPerDay
	w.Advance("a")
	if w.Actors[0].Plan.Status != "active" || w.Actors[0].Plan.Produced != 0 {
		t.Fatal("sleep erased or progressed plan")
	}
	assertContinuityEvent(t, w.Apply("a", Action{Kind: "abandon_plan"}), "plan_abandoned")
	w.Actors[0].Task = &Task{Action: Action{Kind: "craft", RecipeID: "make_food"}, Remaining: 1, Yield: 2}
	w.Advance("a")
	if w.Actors[0].Plan.Status != "abandoned" || w.Actors[0].Plan.Produced != 0 {
		t.Fatal("abandoned plan progressed")
	}
}

func TestPersonalPlanCountsPartialSalesBeforeProductionTarget(t *testing.T) {
	w := continuityWorld()
	w.Catalog.Recipes[0].OutputQty = 1
	w.Apply("a", Action{Kind: "start_plan", RecipeID: "make_food"})
	w.Actors[0].Task = &Task{Action: Action{Kind: "craft", RecipeID: "make_food"}, Remaining: 1, Yield: 1}
	w.Advance("a")
	events := w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "food", Quantity: 1, Price: 1})
	w.Apply("b", Action{Kind: "accept", OfferID: events[0].ID})
	p := w.Actors[0].Plan
	if p.Produced != 1 || p.Sold != 1 || p.Status != "active" || p.Stage != "produce" {
		t.Fatalf("partial real sale lost: %+v", p)
	}
	w.Actors[0].Task = &Task{Action: Action{Kind: "craft", RecipeID: "make_food"}, Remaining: 1, Yield: 1}
	w.Advance("a")
	if p.Stage != "sell" || p.Status != "active" {
		t.Fatalf("plan skipped remaining sale: %+v", p)
	}
	events = w.Apply("a", Action{Kind: "offer", TargetID: "b", Item: "food", Quantity: 1, Price: 1})
	w.Apply("b", Action{Kind: "accept", OfferID: events[0].ID})
	if p.Status != "completed" || p.Sold != 2 {
		t.Fatalf("partial sales failed to finish: %+v", p)
	}
}

func TestContinuityChoicesHaveExecutableTermsAndKnownCounterpart(t *testing.T) {
	w := continuityWorld()
	p := Perception{Self: w.Actors[0], Catalog: w.Catalog, Nearby: []Neighbor{{ID: "b", Tags: []string{"resident", "trader"}}, {ID: "c", Tags: []string{"resident"}}}}
	proposalCount := 0
	for _, x := range ContinuityActions(p) {
		if x.Kind == "propose_agreement" {
			proposalCount++
			if x.TargetID != "b" || x.Item != "food" || x.Quantity != 2 || x.Price != 3 {
				t.Fatalf("wrong proposal terms: %+v", x)
			}
		}
	}
	if proposalCount != 1 {
		t.Fatalf("wanted one executable proposal, got %d", proposalCount)
	}
	// A single owned item is insufficient for the catalog batch when the actor
	// cannot manufacture more. Such a proposal must not be offered to Jev.
	p.Self.Items["food"] = 1
	p.Catalog.Recipes = nil
	for _, x := range ContinuityActions(p) {
		if x.Kind == "propose_agreement" {
			t.Fatal("offered impossible batch")
		}
	}
}

func TestDialogueIsPersistedPerParticipantAndRecallsOldCounterpart(t *testing.T) {
	w := continuityWorld()
	w.ObserveContinuity(Event{ID: 1, ActorID: "a", TargetID: "c", Kind: "speech", Payload: map[string]any{"text": "Talk tomorrow"}})
	for i := int64(2); i <= 80; i++ {
		w.ObserveContinuity(Event{ID: i, ActorID: "a", TargetID: "b", Kind: "speech", Payload: map[string]any{"text": "Recent conversation"}})
	}
	if len(w.Continuity.DialogueByActor["a"]) != 80 || len(w.Continuity.DialogueByActor["b"]) != 79 || len(w.Continuity.DialogueByActor["c"]) != 1 {
		t.Fatal("dialogue copied to nonparticipants or lost")
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var restored World
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	v := restored.ContinuityFor("a")
	if len(v.Dialogue) != 32 || v.Dialogue[0].EventID != 1 || v.Dialogue[len(v.Dialogue)-1].EventID != 80 {
		t.Fatalf("old counterpart lost after restore: %+v", v.Dialogue)
	}
	v.Dialogue[0].Text = "mutated view"
	if restored.Continuity.DialogueByActor["a"][0].Text != "Talk tomorrow" {
		t.Fatal("view aliased persistent dialogue")
	}
	if len(restored.ContinuityFor("c").Dialogue) != 1 || len(restored.ContinuityFor("outsider").Dialogue) != 0 {
		t.Fatal("private dialogue leaked")
	}
}

func TestFulfillmentChoicesReflectCurrentSettlementAvailability(t *testing.T) {
	w := continuityWorld()
	w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "food"})
	w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: 1})
	choices := func() []Action {
		return ContinuityActions(Perception{Tick: w.Tick, Self: w.Actors[0], Catalog: w.Catalog, Continuity: w.ContinuityFor("a"), Nearby: []Neighbor{{ID: "b", Tags: w.Actors[1].Tags}}})
	}
	available := func() bool {
		for _, x := range choices() {
			if x.Kind == "fulfill_agreement" {
				return true
			}
		}
		return false
	}
	if !available() {
		t.Fatal("funded delivery missing from choices")
	}
	w.Actors[1].Money = 0
	if available() {
		t.Fatal("unfunded delivery still offered to selector")
	}
	w.Actors[1].Money = 10
	w.Actors[0].Items["food"] = 0
	if available() {
		t.Fatal("missing stock still offered to selector")
	}
	w.Actors[0].Items["food"] = 4
	w.Actors[1].Location = "elsewhere"
	if available() {
		t.Fatal("distant delivery still offered to selector")
	}
	w.Actors[1].Location = "village"
	w.Actors[1].Dead = true
	if available() {
		t.Fatal("dead participant delivery offered")
	}
	w.Actors[1].Dead = false
	w.Tick = w.Continuity.Agreements[0].DueTick + 1
	if available() {
		t.Fatal("expired delivery still offered to selector")
	}
}

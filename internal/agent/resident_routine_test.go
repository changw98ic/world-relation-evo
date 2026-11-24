package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestMasteredPracticeCanCauseATripAndMaintainCombatPower(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	a.Professions = map[string]int{"martial_artist": 6}
	a.StateSlots = map[string]int{"practice": 2}
	for i := range w.Catalog.Professions {
		if w.Catalog.Professions[i].ID == "martial_artist" {
			w.Catalog.Professions[i].PracticeSlot = "practice"
		}
	}
	w.Map.Districts[1].Features = []string{"training_hall"}
	w.Map.Roads = []sim.Road{{From: "meeting", To: "remote", Ticks: 1, EnergyCost: 1}}
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{"practice": {ID: "practice", Max: 10}}
	w.Catalog.ActionSpecs = map[string]sim.ActionSpec{"drill": {
		ID: "drill", Name: "Drill forms", TargetKind: "self", GrantedByTags: []string{"resident"},
		RequiredVenueFeature: "training_hall", Profession: "martial_artist", EnergyCost: 2,
		SlotGains: map[string]int{"practice": 10},
	}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "state_slot", Subject: "practice"}, Target: 10, Scale: 10, BaseWeight: 1},
	}}
	power := sim.CombatPowerWithCatalog(w.Catalog, *a)
	for range 2 {
		if _, err := w.Step(context.Background(), Planner{}); err != nil {
			t.Fatal(err)
		}
	}
	if a.Location != "remote" || a.Professions["martial_artist"] != 6 || a.StateSlots["practice"] != 10 ||
		sim.CombatPowerWithCatalog(w.Catalog, *a) <= power {
		t.Fatalf("the master did not travel to maintain existing ability: %+v", a)
	}
}

func TestConversationRequiresARealQuestionAndAnIndependentReply(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{"contact": {
		ID: "contact", Max: 10, Default: 10, ConversationGain: 10,
	}}
	w.Catalog.Planning = &sim.PlanningRules{HorizonTicks: sim.TicksPerDay, OfferAcceptancePrior: 0.4, Needs: []sim.NeedRule{
		{NeedKey: sim.NeedKey{Measure: "state_slot", Subject: "contact"}, Target: 10, Scale: 10, BaseWeight: 1},
		{NeedKey: sim.NeedKey{Measure: "conversation"}, Target: 1, Scale: 1, BaseWeight: .25},
	}}
	a := &w.Actors[0]
	a.StateSlots = map[string]int{"contact": 0}
	open, err := (Planner{}).Decide(context.Background(), w.View(a.ID))
	if err != nil || open.Kind != "speak" || open.Statement == nil || open.Statement.Topic != sim.ConversationQuestion {
		t.Fatalf("the isolated actor did not start a conversation: %+v, %v", open, err)
	}
	question := boundaryApply(t, w, a.ID, open)[0]
	if a.StateSlots["contact"] != 0 {
		t.Fatal("an unanswered question was counted as a completed conversation")
	}
	reply, err := (Planner{}).Decide(context.Background(), w.View(open.TargetID))
	if err != nil || reply.Kind != "speak" || reply.Statement == nil || reply.Statement.Topic != sim.ConversationReply ||
		reply.BasisEventID != question.ID || reply.TargetID != a.ID {
		t.Fatalf("the listener did not choose to answer the actual question: %+v, %v", reply, err)
	}
	result := boundaryApply(t, w, open.TargetID, reply)[0]
	if a.StateSlots["contact"] != 10 || result.Payload["cause_event_id"] != question.ID || a.Trust[open.TargetID] != 0 {
		t.Fatalf("the reply did not fulfil the social need through a real exchange: %+v", result)
	}
	boundaryRejected(t, w, open.TargetID, reply)
}

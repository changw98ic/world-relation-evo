package sim_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"testing"
)

func TestTradeRequiresAcceptance(t *testing.T) {
	w := sim.Village("test")
	a, b := &w.Actors[0], &w.Actors[1]
	food, money := a.Food, b.Money
	e := w.Apply(a.ID, sim.Action{Kind: "offer", TargetID: b.ID, Food: 2, Price: 2})
	if a.Food != food || b.Money != money {
		t.Fatal("proposal transferred resources")
	}
	w.Apply(b.ID, sim.Action{Kind: "accept", OfferID: e[0].ID})
	if a.Food != food-2 || b.Money != money-2 {
		t.Fatal("accepted trade not settled")
	}
}
func TestRejectKeepsResources(t *testing.T) {
	w := sim.Village("test")
	before := w.Actors[0].Food
	e := w.Apply("p01", sim.Action{Kind: "offer", TargetID: "p02", Food: 2, Price: 2})
	w.Apply("p02", sim.Action{Kind: "reject", OfferID: e[0].ID})
	if w.Actors[0].Food != before || len(w.Offers) != 0 {
		t.Fatal("rejection altered goods or left offer")
	}
}
func TestSpeechIsPrivateClaimNotFact(t *testing.T) {
	w := sim.Village("test")
	before := w.Actors[0].Food
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "I own all your food"})
	if w.Actors[0].Food != before {
		t.Fatal("speech changed inventory")
	}
	if len(w.Actors[1].Memories) != 1 || len(w.Actors[2].Memories) != 0 {
		t.Fatal("speech visibility incorrect")
	}
	if w.Actors[1].Memories[0].Source != "p01" {
		t.Fatal("lost source")
	}
}
func TestPerceptionDoesNotExposeOthersInventory(t *testing.T) {
	w := sim.Village("test")
	w.Actors[2].Location = "fields"
	p := w.View("p01")
	for _, n := range p.Nearby {
		if n.ID == "p03" {
			t.Fatal("remote actor visible")
		}
	}
	p.Self.Trust["p02"] = 90
	if w.Actors[0].Trust["p02"] != 0 {
		t.Fatal("view mutates world")
	}
}

type failing struct{}

func (failing) Decide(context.Context, sim.Perception) (sim.Action, error) {
	return sim.Action{}, errors.New("model unavailable")
}
func TestFailedDecisionDoesNotAdvance(t *testing.T) {
	w := sim.Village("test")
	b, _ := json.Marshal(w)
	_, err := w.Step(context.Background(), failing{})
	a, _ := json.Marshal(w)
	if err == nil || string(a) != string(b) {
		t.Fatal("step changed after decision failure")
	}
}
func TestVillageRunsSevenDays(t *testing.T) {
	w := sim.Village("test")
	kinds := map[string]int{}
	var last int64
	for i := 0; i < 168; i++ {
		events, err := w.Step(context.Background(), agent.Rules{})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.ID <= last || e.ActorID == "" || e.Action.Kind == "" {
				t.Fatal("event lacks action provenance")
			}
			last = e.ID
			kinds[e.Kind]++
		}
	}
	if len(w.Actors) != 30 || w.Tick != 168 {
		t.Fatal("wrong population or time")
	}
	for _, k := range []string{"speech", "trade_completed", "harvested", "ate"} {
		if kinds[k] == 0 {
			t.Fatalf("no %s", k)
		}
	}
	for _, a := range w.Actors {
		if a.Food < 0 || a.Money < 0 {
			t.Fatal("negative resources")
		}
	}
	t.Logf("events=%v", kinds)
}
func TestRuleTradeDependsOnTrust(t *testing.T) {
	w := sim.Village("test")
	w.Apply("p01", sim.Action{Kind: "offer", TargetID: "p02", Food: 2, Price: 2})
	p := w.View("p02")
	a, _ := (agent.Rules{}).Decide(context.Background(), p)
	p.Self.Trust["p01"] = -1
	b, _ := (agent.Rules{}).Decide(context.Background(), p)
	if a.Kind != "accept" || b.Kind != "reject" {
		t.Fatal("trust ignored")
	}
}

// A scripted cognition fixture verifies plumbing, not model reasoning quality.
type heardInstruction struct{}

func (heardInstruction) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	for _, m := range p.Self.Memories {
		if m.Kind == "speech" && m.Source == "p01" && m.Text == "Meet me at the well" {
			return sim.Action{Kind: "move", Location: "well"}, nil
		}
	}
	return sim.Action{Kind: "wait"}, nil
}
func TestSpeechCanDriveLaterActionWithoutGlobalKnowledge(t *testing.T) {
	w := sim.Village("test")
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "Meet me at the well"})
	if w.Actors[1].Memories[0].Kind != "speech" {
		t.Fatal("speech memory lost provenance type")
	}
	if _, err := w.Step(context.Background(), heardInstruction{}); err != nil {
		t.Fatal(err)
	}
	if w.Actors[1].Location != "well" || w.Actors[2].Location != "homes" {
		t.Fatal("heard instruction not routed to listener only")
	}
}

func TestMultiHopInformationChangesBehavior(t *testing.T) {
	w := sim.Village("chain")
	w.Actors = w.Actors[:5]
	for i := range w.Actors {
		w.Actors[i].Food = 10
		w.Actors[i].Location = "market"
	}
	w.Actors[0].Location = "fields"
	w.Actors[1].Location = "fields"
	w.Actors[2].Location = "well"
	w.Actors[3].Location = "homes"
	w.Actors[3].Food = 4
	w.Tick = 6
	fact := w.Apply("p01", sim.Action{Kind: "work"})[0]
	observedFood := w.Actors[0].Food
	firstAction, _ := (agent.Rules{}).Decide(context.Background(), w.View("p01"))
	if firstAction.Kind != "speak" || firstAction.TargetID != "p02" {
		t.Fatalf("observation not communicated: %+v", firstAction)
	}
	first := w.Apply("p01", firstAction)[0]
	w.Tick = 7
	w.Apply("p02", sim.Action{Kind: "move", Location: "well"})
	w.Tick = 9
	relayAction, _ := (agent.Rules{}).Decide(context.Background(), w.View("p02"))
	if relayAction.Kind != "speak" || relayAction.TargetID != "p03" || relayAction.BasisEventID != first.ID {
		t.Fatalf("baseline did not consume/relay heard statement: %+v", relayAction)
	}
	second := w.Apply("p02", relayAction)[0]
	w.Tick = 10
	w.Apply("p03", sim.Action{Kind: "move", Location: "homes"})
	w.Tick = 12
	// C intentionally exaggerates. The resolver records that choice, never adds noise.
	third := w.Apply("p03", sim.Action{Kind: "speak", TargetID: "p04", BasisEventID: second.ID, Text: "They produced twenty food at the fields!", Statement: &sim.Statement{Topic: "food_produced", Location: "fields", Quantity: 20}})[0]
	w.Tick = 13
	decision, _ := (agent.Rules{}).Decide(context.Background(), w.View("p04"))
	if decision.Kind != "move" || decision.Location != "fields" || decision.BasisEventID != third.ID {
		t.Fatalf("heard news did not cause action: %+v", decision)
	}
	result := w.Apply("p04", decision)[0]
	if result.Action.BasisEventID != third.ID || w.Actors[3].Location != "fields" {
		t.Fatal("causal action not settled")
	}
	chain := []sim.Event{first, second, third, result}
	parent := fact.ID
	for _, e := range chain {
		if e.Action.BasisEventID != parent || e.Payload["root_event_id"] != fact.ID {
			t.Fatalf("broken chain: %+v", e)
		}
		parent = e.ID
	}
	for i, want := range []struct {
		source string
		tick   int64
		parent int64
	}{{"p01", 6, fact.ID}, {"p02", 9, first.ID}, {"p03", 12, second.ID}} {
		a := w.Actors[i+1]
		var heard *sim.Memory
		for j := range a.Memories {
			m := &a.Memories[j]
			if m.Kind == "speech" && m.Source == want.source {
				heard = m
				break
			}
		}
		if heard == nil || heard.Tick != want.tick || heard.BasisEventID != want.parent || heard.RootEventID != fact.ID || heard.Observation != nil {
			t.Fatalf("wrong direct source/time/epistemic type: %+v", heard)
		}
	}
	if len(w.Actors[4].Memories) != 0 {
		t.Fatal("uninvolved remote villager learned news")
	}
	if w.Actors[0].Food != observedFood || fact.Payload["observation"].(sim.Observation).Quantity != 2 {
		t.Fatal("retelling changed original fact")
	}
	if first.Action.Statement.Quantity != 2 || third.Action.Statement.Quantity != 20 || third.Action.Text == first.Action.Text {
		t.Fatal("different statement contents lost")
	}
}
func TestCannotCiteUnknownMemoryOrSpeakRemotely(t *testing.T) {
	w := sim.Village("scope")
	w.Actors[0].Location = "fields"
	w.Actors[1].Location = "well"
	fact := w.Apply("p01", sim.Action{Kind: "work"})[0]
	e := w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "News", BasisEventID: fact.ID})[0]
	if e.Kind != "action_failed" || len(w.Actors[1].Memories) != 0 {
		t.Fatal("remote speech delivered")
	}
	e = w.Apply("p02", sim.Action{Kind: "move", Location: "fields", BasisEventID: fact.ID})[0]
	if e.Kind != "action_failed" || w.Actors[1].Location != "well" {
		t.Fatal("unseen fact accepted as basis")
	}
}

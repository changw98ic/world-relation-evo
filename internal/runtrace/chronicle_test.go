package runtrace

import (
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestBuildChronicleAndRenderMarkdown(t *testing.T) {
	w := sim.Village("chronicle-test")
	w.Actors[0].Ambitions = []sim.ActorAmbition{{ID: "a1", Kind: "accumulate_wealth", TargetValue: 80}}
	w.Actors[1].Dead = true
	w.Actors[1].DeathCause = "combat attack"
	w.Actors[1].DeathTick = 20

	events := []sim.Event{
		{
			ID:      10,
			Tick:    20,
			ActorID: w.Actors[1].ID,
			Kind:    "actor_died",
			Payload: map[string]any{"text": "Actor 1 was slain in wilds", "root_event_id": int64(10)},
		},
		{
			ID:       11,
			Tick:     24,
			ActorID:  w.Actors[2].ID,
			TargetID: w.Actors[0].ID,
			Kind:     "speech",
			Action: sim.Action{
				Kind:         "speak",
				TargetID:     w.Actors[0].ID,
				BasisEventID: 10,
				Statement:    &sim.Statement{Topic: "actor_died", SubjectID: "killer_x", ObjectID: w.Actors[1].ID},
			},
			Payload: map[string]any{"text": "Witness reported the slaying", "root_event_id": int64(10)},
		},
		{
			ID:       12,
			Tick:     24,
			ActorID:  w.Actors[0].ID,
			TargetID: "killer_x",
			Kind:     "grudge_formed",
			Payload:  map[string]any{"text": "Blood feud formed over reported homicide", "root_event_id": int64(10)},
		},
		{
			ID:      13,
			Tick:    40,
			ActorID: "world",
			Kind:    "world_event_triggered",
			Payload: map[string]any{"text": "Relic emerged at district_0", "root_event_id": int64(13)},
		},
		{
			ID:      14,
			Tick:    44,
			ActorID: w.Actors[0].ID,
			Kind:    "world_event_claimed",
			Payload: map[string]any{"text": "Actor 0 claimed Relic", "root_event_id": int64(13)},
		},
		{
			ID:      15,
			Tick:    50,
			ActorID: w.Actors[2].ID,
			Kind:    "party_betrayed",
			Payload: map[string]any{"text": "Actor 2 betrayed the expedition party"},
		},
		{
			ID:      16,
			Tick:    55,
			ActorID: "world",
			Kind:    "faction_war_declared",
			Payload: map[string]any{"text": "Faction A declared war on Faction B"},
		},
		{
			ID:      17,
			Tick:    60,
			ActorID: w.Actors[0].ID,
			Kind:    "secret_exposed",
			Payload: map[string]any{"text": "Actor 0 exposed covert bribe"},
		},
	}

	rep := BuildChronicle(events, w, 50)
	if rep.WorldID != "chronicle-test" {
		t.Fatalf("expected world_id chronicle-test, got %q", rep.WorldID)
	}
	if len(rep.Timeline) < 6 {
		t.Fatalf("expected >=6 major timeline entries, got %d", len(rep.Timeline))
	}
	if len(rep.PropagationChains) < 2 {
		t.Fatalf("expected >=2 causal propagation chains (death news + world event claim), got %d", len(rep.PropagationChains))
	}
	foundDeathChain := false
	for _, ch := range rep.PropagationChains {
		if ch.RootEventID == 10 && len(ch.Hops) == 2 {
			foundDeathChain = true
		}
	}
	if !foundDeathChain {
		t.Fatalf("expected death-news propagation chain with 2 hops (speech + grudge_formed), got %+v", rep.PropagationChains)
	}
	if len(rep.Biographies) != len(w.Actors) {
		t.Fatalf("expected %d biographies, got %d", len(w.Actors), len(rep.Biographies))
	}

	md := RenderChronicleMarkdown(rep)
	for _, want := range []string{
		"# World Chronicle: chronicle-test",
		"## Major Chronicle Timeline",
		"## Epistemic Propagation Chains",
		"## Actor Biographies & Fate Cards",
		"party_betrayed",
		"faction_war_declared",
		"secret_exposed",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, md)
		}
	}
}

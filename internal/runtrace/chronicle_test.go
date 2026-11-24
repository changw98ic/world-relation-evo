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

func TestChronicleDayAlignmentUnlimitedAndRemoteChannelPropagation(t *testing.T) {
	// Tick 72 is Day 2 00:00 because Tick 0 is Day 1 06:00.
	events := []sim.Event{
		{
			ID:      1,
			Tick:    72,
			ActorID: "victim",
			Kind:    "actor_died",
			Payload: map[string]any{"text": "Victim fell at midnight", "root_event_id": int64(1)},
		},
		{
			ID:      2,
			Tick:    74,
			ActorID: "scout",
			Kind:    "channel_transmitted",
			Payload: map[string]any{
				"text":          "Broadcast death news",
				"root_event_id": int64(1),
				"delivered_to":  []string{"kin_a", "kin_b"},
				"statement":     map[string]any{"topic": "actor_died"},
			},
		},
	}
	// Add 130 additional major events to verify maxTimeline=0 does not truncate at 120.
	for i := 0; i < 130; i++ {
		events = append(events, sim.Event{
			ID:      int64(10 + i),
			Tick:    int64(80 + i),
			ActorID: "a",
			Kind:    "grudge_formed",
			Payload: map[string]any{"text": "feud"},
		})
	}

	rep := BuildChronicle(events, nil, 0)
	if rep.HasFinalState {
		t.Fatal("expected HasFinalState=false when finalWorld is nil")
	}
	if len(rep.Timeline) != 131 {
		t.Fatalf("expected all 131 major events without 120-item truncation, got %d", len(rep.Timeline))
	}
	if rep.Timeline[0].Day != 2 || rep.Timeline[0].Clock != "00:00" {
		t.Fatalf("tick 72 should align with ClockAt Day 2 00:00, got Day %d %s", rep.Timeline[0].Day, rep.Timeline[0].Clock)
	}
	if len(rep.PropagationChains) != 1 || len(rep.PropagationChains[0].Hops) != 1 {
		t.Fatalf("expected channel_transmitted hop in propagation chain, got %+v", rep.PropagationChains)
	}
	if rep.PropagationChains[0].Hops[0].TargetID != "kin_a,kin_b" || rep.PropagationChains[0].Hops[0].Topic != "actor_died" {
		t.Fatalf("unexpected remote channel hop fields: %+v", rep.PropagationChains[0].Hops[0])
	}
	md := RenderChronicleMarkdown(rep)
	if !strings.Contains(md, "UNKNOWN (no final state)") {
		t.Fatalf("expected UNKNOWN ledger status when final state omitted, got:\n%s", md)
	}
}

func TestChronicleTimelineTracksCourtAndCombatCauses(t *testing.T) {
	events := []sim.Event{
		{ID: 1, Kind: "attacked"},
		{ID: 2, Kind: "secret_held"},
		{ID: 3, Kind: "secret_caught_red_handed", Payload: map[string]any{"root_event_id": int64(1), "cause_event_id": int64(1)}},
		{ID: 4, Kind: "court_warrant_issued", Action: sim.Action{BasisEventID: 3}},
		{ID: 5, Kind: "arrested", Payload: map[string]any{"root_event_id": int64(1), "cause_event_id": int64(4)}},
		{ID: 6, Kind: "court_trial_held", Payload: map[string]any{"root_event_id": int64(1), "cause_event_id": int64(5)}},
		{ID: 7, Kind: "district_control_shifted"},
	}
	report := BuildChronicle(events, nil, 6)
	if len(report.Timeline) != 6 || report.TimelineTruncated || report.Timeline[5].Kind != "district_control_shifted" {
		t.Fatalf("timeline omitted outcomes or let holding noise consume its budget: %+v", report.Timeline)
	}
	if len(report.PropagationChains) != 1 {
		t.Fatalf("expected one connected causal chain, got %+v", report.PropagationChains)
	}
	chain := report.PropagationChains[0]
	if chain.RootEventID != 1 || len(chain.Hops) != 4 || chain.Hops[3].CauseEventID != 5 {
		t.Fatalf("case chain lost the attack or direct arrest link: %+v", chain)
	}
}

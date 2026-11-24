package agent

import (
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestJoinFactionNotOfferedWhenWarring(t *testing.T) {
	w := sim.Village("no-warring-join-opt")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	a := &w.Actors[0]
	a.Guilds = []string{fA}
	a.Location = w.Factions[1].District
	a.InsideHouse = ""
	w.Diplomacy = sim.NewDiplomacyState()
	key := fA + "|" + fB
	if fA > fB {
		key = fB + "|" + fA
	}
	w.Diplomacy.Relations[key] = &sim.FactionDiplomacy{
		FactionA: fA, FactionB: fB, Stance: "at_war", RelationScore: -80,
	}

	p := w.View(a.ID)
	opts := Options(p)
	for _, o := range opts {
		if o.Action.Kind == "join_faction" && o.Action.FactionID == fB {
			t.Fatalf("Options() must not offer join_faction(%s) when at war with actor's faction", fB)
		}
	}
}

func TestDefaultSpeechNeverLeaksPrivateGoal(t *testing.T) {
	w := sim.Village("no-goal-leak")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Goal = "conceal secret escape plan"
	a.Location = b.Location
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Memories = nil

	p := w.View(a.ID)
	opts := Options(p)
	for _, o := range opts {
		if o.Action.Kind == "speak" {
			if strings.Contains(o.Action.Text, "conceal secret escape plan") || strings.Contains(o.Label, "conceal secret escape plan") {
				t.Fatalf("default speak leaked private Self.Goal: label=%q text=%q", o.Label, o.Action.Text)
			}
		}
	}
}

func TestAuthoredRulesPreserveCausalEvidence(t *testing.T) {
	w := sim.Village("rulebook-jev-bridge")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = b.Location
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Tags = append(a.Tags, "fighter")
	a.Trust = map[string]int{b.ID: -4}
	a.Memories = append(a.Memories, sim.Memory{
		Kind:     "attacked",
		Source:   b.ID,
		TargetID: a.ID,
		Tick:     w.Tick,
		EventID:  77,
		Text:     b.Name + " attacked " + a.Name,
	})

	rb := DefaultBaseRulebook(w)
	rules, err := (DefaultRuleCompiler{}).Compile(rb, w.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	options, err := (DefaultCandidateBuilder{}).Build(w.View(a.ID), rules)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range options {
		act := candidate.Action
		if act.Kind == "attack" && act.TargetID == b.ID && act.BasisEventID == 77 {
			evs := w.Apply(a.ID, act)
			if len(evs) == 0 || evs[0].Kind == "action_failed" {
				t.Fatalf("causal action could not settle: %+v", evs)
			}
			return
		}
	}
	t.Fatal("the authored reaction lost its causal event")
}

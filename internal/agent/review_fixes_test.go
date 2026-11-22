package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestRulebookEnrichesExecutableOptionsForJev(t *testing.T) {
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
	advisor := NewRulebookSelector(rb, nil)

	var capturedInstructions string
	var capturedCriteria map[string]string
	var chosenID string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			Questions map[string]struct {
				Instructions string            `json:"instructions"`
				Criteria     map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode jev request: %v", err)
			http.Error(rw, err.Error(), 400)
			return
		}
		dec := req.Questions["decision"]
		capturedInstructions = dec.Instructions
		capturedCriteria = dec.Criteria
		// Pick the rulebook-annotated attack retaliation option.
		for id, desc := range dec.Criteria {
			if strings.Contains(desc, "Rulebook reaction") && strings.Contains(desc, `"kind":"attack"`) {
				chosenID = id
				break
			}
		}
		if chosenID == "" {
			for id := range dec.Criteria {
				chosenID = id
				break
			}
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"answers": map[string]any{
				"decision": map[string]any{
					"type":   "choice",
					"choice": chosenID,
				},
			},
		})
	}))
	defer srv.Close()

	pipe := Pipeline{
		Selector: Jev{
			Endpoint:        srv.URL,
			APIKey:          "test-key",
			Name:            "jev-test",
			Client:          srv.Client(),
			RulebookAdvisor: advisor,
		},
	}

	act, err := pipe.Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatalf("Jev+Rulebook Decide failed: %v", err)
	}
	if !strings.Contains(capturedInstructions, "Matched interaction rulebook recommendations") {
		t.Fatalf("expected rulebook guidance in Jev instructions, got %q", capturedInstructions)
	}
	if chosenID == "" || !strings.Contains(capturedCriteria[chosenID], "Rulebook reaction") {
		t.Fatalf("expected Jev criteria to carry Rulebook reaction annotation, got %q", capturedCriteria[chosenID])
	}
	if act.Kind != "attack" || act.TargetID != b.ID {
		t.Fatalf("expected Jev to execute annotated retaliation attack against %s, got %+v", b.ID, act)
	}
	if act.BasisEventID != 77 {
		t.Fatalf("expected causal BasisEventID=77 stamped from matched reaction memory, got %d", act.BasisEventID)
	}
	evs := w.Apply(a.ID, act)
	if len(evs) == 0 || evs[0].Kind == "action_failed" {
		t.Fatalf("expected Jev-chosen rulebook-annotated option to execute cleanly, got %+v", evs)
	}
}

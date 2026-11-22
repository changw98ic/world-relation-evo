package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestG2DeclarativeActionExtensionRuleCompilerAndDeciderParity(t *testing.T) {
	w := sim.Village("g2-declarative-parity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 60
	a.Health = sim.MaxHealth
	b.Health = sim.MaxHealth
	a.Tags = append(a.Tags, "cultivator")
	a.StateSlots = map[string]int{"qi": 40}
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{
		"qi": {ID: "qi", Name: "Qi", Min: 0, Max: 100},
	}

	// 1. Register a brand-new composite declarative interaction WITHOUT touching World.Apply().
	w.Catalog.ActionSpecs = map[string]sim.ActionSpec{
		"spirit_palm": {
			ID:            "spirit_palm",
			Name:          "Spirit Palm",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "actor",
			EnergyCost:    5,
			SlotCosts:     map[string]int{"qi": 10},
			SlotGains:     map[string]int{"qi": 2},
			Attack: &sim.AttackSpec{
				DamageByType: map[string]int{"spirit": 18},
			},
			Ticks: 1,
		},
	}

	// 2. RuleCompiler MUST reject unknown / uncompiled new_primitive:* at compile time.
	badRB := &InteractionRulebook{
		ProactiveRules: []InteractionRule{
			{ID: "bad_1", Behavior: BehaviorSpec{EngineKind: "new_primitive:teleport_moon"}},
		},
	}
	if _, err := (DefaultRuleCompiler{}).Compile(badRB, w.Catalog); err == nil {
		t.Fatalf("expected DefaultRuleCompiler to reject unknown new_primitive:* at compile time")
	}

	// 3. RuleCompiler accepts a rule targeting our newly configured ActionSpec.
	goodRB := &InteractionRulebook{
		ProactiveRules: []InteractionRule{
			{
				ID:       "cultivator_duel",
				Name:     "Spirit Palm Exchange",
				Category: "conflict",
				Actor1:   RoleMatcher{AnyTags: []string{"cultivator"}},
				Actor2:   RoleMatcher{},
				Behavior: BehaviorSpec{EngineKind: "spirit_palm", TargetBinding: "actor2"},
				Weight:   40,
			},
		},
	}
	ruleSet, err := (DefaultRuleCompiler{}).Compile(goodRB, w.Catalog)
	if err != nil {
		t.Fatalf("expected valid ActionSpec rule to compile cleanly, got: %v", err)
	}

	// 4. CandidateBuilder produces unified legal candidates with structured rule match metadata.
	p := w.View(a.ID)
	cands1, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("CandidateBuilder.Build failed: %v", err)
	}
	cands2, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("second CandidateBuilder.Build failed: %v", err)
	}
	if !reflect.DeepEqual(cands1, cands2) {
		t.Fatalf("CandidateBuilder must be deterministic for identical perception & rules")
	}

	var matchedCand *sim.Candidate
	for i := range cands1 {
		if cands1[i].Intent.Action.Kind == "spirit_palm" && cands1[i].Intent.Action.TargetID == b.ID {
			matchedCand = &cands1[i]
			break
		}
	}
	if matchedCand == nil {
		t.Fatalf("expected spirit_palm -> %s candidate in unified CandidateBuilder output", b.ID)
	}
	if matchedCand.Source != "action_spec" {
		t.Fatalf("expected Source=action_spec, got %q", matchedCand.Source)
	}
	if !sim.ActorHasTag(matchedCand.RuleIDs, "cultivator_duel") {
		t.Fatalf("expected structured RuleIDs to contain cultivator_duel, got %v", matchedCand.RuleIDs)
	}

	// 5. Both RuleDecisionSelector and JevDecisionSelector consume the EXACT SAME candidates.
	ruleSel := RuleDecisionSelector{Selector: RuleSelector{}}
	ruleIntent, err := ruleSel.Select(context.Background(), p, cands1)
	if err != nil || ruleIntent.Action.Kind == "" {
		t.Fatalf("RuleDecisionSelector failed: intent=%+v err=%v", ruleIntent, err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"answers": map[string]any{
				"decision": map[string]any{"type": "choice", "choice": matchedCand.ID},
			},
		})
	}))
	defer srv.Close()

	jevSel := JevDecisionSelector{Jev: Jev{Endpoint: srv.URL, APIKey: "test-key", Client: srv.Client()}}
	jevIntent, err := jevSel.Select(context.Background(), p, cands1)
	if err != nil {
		t.Fatalf("JevDecisionSelector failed: %v", err)
	}
	if jevIntent.Action.Kind != "spirit_palm" || jevIntent.Action.TargetID != b.ID {
		t.Fatalf("unexpected Jev intent: %+v", jevIntent)
	}

	// 6. Execute via ActionExecutor without modifying World.Apply() and verify traceable events & state change.
	exec := sim.DefaultActionExecutor{}
	res, err := exec.Execute(w, a.ID, jevIntent)
	if err != nil || !res.Succeeded {
		t.Fatalf("DefaultActionExecutor failed on declarative spirit_palm: res=%+v err=%v", res, err)
	}
	if a.StateSlots["qi"] != 32 { // 40 - 10 + 2
		t.Fatalf("expected actor qi=32 after spirit_palm, got %d", a.StateSlots["qi"])
	}
	if b.Health >= sim.MaxHealth {
		t.Fatalf("expected target %s to take spirit_palm damage, health=%d", b.ID, b.Health)
	}
}

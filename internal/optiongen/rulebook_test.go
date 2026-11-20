package optiongen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestBuildWorldOntology_StripsAllActorNamesAndRespectsInstitutions(t *testing.T) {
	w := sim.Village("ont-test")
	// Disable racketeering to verify extort is excluded.
	f := false
	env := w.Catalog.EffectiveEnvironment()
	env.Institutions = &sim.InstitutionToggles{Racketeering: &f}
	env.PersonalityCatalog = map[string]string{
		"reclusive":  "孤僻隐世",
		"flamboyant": "张扬高调",
	}
	w.Catalog.Environment = &env
	// Give distinctive actor IDs and names to ensure none leak into ontology.
	w.Actors[0].ID = "secret_actor_alpha_99"
	w.Actors[0].Name = "绝密测试角色甲"
	w.Actors[0].Personality = "proud and ruthless"
	w.Actors[0].Hobbies = []string{"sword_dueling", "tea_gossip"}

	ont := BuildWorldOntology(w)
	raw, err := json.Marshal(ont)
	if err != nil {
		t.Fatalf("marshal ontology: %v", err)
	}
	blob := string(raw)
	if strings.Contains(blob, "secret_actor_alpha_99") || strings.Contains(blob, "绝密测试角色甲") {
		t.Fatalf("ontology leaked actor identity! blob=%s", blob)
	}
	if len(ont.Glossaries.Actors) != 0 {
		t.Fatalf("expected empty Actors glossary in ontology, got %v", ont.Glossaries.Actors)
	}
	if !strings.Contains(blob, "ruthless") || !strings.Contains(blob, "sword_dueling") || !strings.Contains(blob, "reclusive") || !strings.Contains(blob, "flamboyant") {
		t.Fatalf("expected anonymized personality_traits (including catalog) and hobbies in ontology, got traits=%v hobbies=%v",
			ont.PersonalityTraits, ont.Hobbies)
	}
	for _, act := range ont.AllowedEngineActions {
		if act == "extort" {
			t.Fatalf("extort should be excluded when racketeering is disabled")
		}
	}
	for _, out := range ont.ValidOutcomeKinds {
		if out == "extorted" || out == "extort_resisted" {
			t.Fatalf("extort outcomes should be excluded when racketeering is disabled")
		}
	}
}

func TestExpandWorldRulebook_MockDeepSeekAndAudit(t *testing.T) {
	w := sim.Village("rulebook-gen-test")
	w.Actors[0].ID = "forbidden_hero_id"
	w.Actors[0].Name = "严禁硬编码名"
	w.Actors[0].Personality = "proud and ruthless"
	w.Actors[0].Hobbies = []string{"sword_dueling", "tea_gossip"}
	f := false
	env := w.Catalog.EffectiveEnvironment()
	env.Institutions = &sim.InstitutionToggles{Racketeering: &f}
	env.PersonalityCatalog = map[string]string{
		"reclusive":  "孤僻隐世",
		"flamboyant": "张扬高调",
	}
	w.Catalog.Environment = &env

	var mu sync.Mutex
	seenDomains := map[string]bool{}
	seenCalibration := false
	var promptLeak string

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		mu.Lock()
		if strings.Contains(bodyStr, "forbidden_hero_id") || strings.Contains(bodyStr, "严禁硬编码名") {
			promptLeak = "actor identity leaked into DS prompt"
		}
		mu.Unlock()

		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		if len(req.Messages) > 1 && strings.Contains(req.Messages[1].Content, "trait_calibration") {
			mu.Lock()
			seenCalibration = true
			mu.Unlock()
			calPayload := TraitCalibrationOutput{
				Summary: "全局职业、性格与遭遇解耦标定",
				ProfessionEncounterModifiers: map[string]agent.EncounterProfile{
					"merchant": {ProactiveRate: 1.70, TargetAttraction: 1.80, DirectEngage: 1.40, WitnessEngage: 1.35},
					"farmer":   {ProactiveRate: 0.50, TargetAttraction: 0.48, DirectEngage: 0.72, WitnessEngage: 0.48},
				},
				PersonalityStanceWeights: map[string]map[string]float64{
					"ruthless":   {"强硬反制": 2.6, "积极呼应": 0.4, "避险止损": 0.45},
					"reclusive":  {"避险止损": 2.4, "暗中周旋": 1.6, "强硬反制": 0.4},
					"flamboyant": {"强硬反制": 1.9, "借力拉拢": 1.8, "避险止损": 0.45},
				},
				PersonalityEncounterModifiers: map[string]agent.EncounterProfile{
					"flamboyant": {ProactiveRate: 1.45, TargetAttraction: 1.55, DirectEngage: 1.30, WitnessEngage: 1.35},
					"reclusive":  {ProactiveRate: 0.58, TargetAttraction: 0.58, DirectEngage: 0.62, WitnessEngage: 0.48},
				},
				HobbyActionWeights: map[string]map[string]float64{
					"sword_dueling": {"attack": 2.5, "extort": 3.0}, // extort disabled -> sanitizer must strip it!
					"tea_gossip":    {"speak": 2.2},
				},
			}
			contentBytes, _ := json.Marshal(calPayload)
			env := map[string]any{
				"choices": []map[string]any{
					{"finish_reason": "stop", "message": map[string]string{"content": string(contentBytes)}},
				},
				"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 200, "total_tokens": 300},
			}
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(env)
			return
		}

		domID := "generic"
		for _, cand := range []string{"trade_economy", "enforcement_oversight", "shadow_conflict", "alliance_mentorship", "cooperation_crisis"} {
			if len(req.Messages) > 1 && strings.Contains(req.Messages[1].Content, cand) {
				domID = cand
				break
			}
		}
		mu.Lock()
		seenDomains[domID] = true
		mu.Unlock()

		// Return 1 valid proactive rule, 1 invalid/leaked proactive rule (to prove sanitizer catches it),
		// and 2 valid reaction rules (1 actor2, 1 witness), plus hobby weight tables.
		payload := DomainRulebookOutput{
			Domain:  domID,
			Summary: "通用领域规则",
			HobbyActionWeights: map[string]map[string]float64{
				"sword_dueling": {"attack": 2.5, "extort": 3.0}, // extort disabled -> sanitizer must strip it!
				"tea_gossip":    {"speak": 2.2},
			},
			ProactiveRules: []agent.InteractionRule{
				{
					ID:                    "pro_" + domID + "_01",
					Domain:                domID,
					Name:                  "通用互动报价",
					Category:              "交易",
					Priority:              "high",
					Weight:                40,
					HobbyWeights:          map[string]float64{"tea_gossip": 2.4, "sword_dueling": 0.5},
					SharedHobbyMultiplier: 1.8,
					CooldownTicks:         24,
					Actor1:                agent.RoleMatcher{Roles: []string{"*"}},
					Actor2:                agent.RoleMatcher{Roles: []string{"*"}},
					Behavior: agent.BehaviorSpec{
						EngineKind:      "speak",
						TargetBinding:   "$actor2",
						SummaryTemplate: "$actor1 在 $location 与 $actor2 商议事务",
					},
					ExpectedOutcomes: []string{"speech"},
					CausalSummary:    "发起商议后触发对方或旁观者回应",
				},
				{
					ID:     "pro_" + domID + "_leaked",
					Domain: domID,
					Name:   "故意包含硬编码名严禁硬编码名的坏规则",
					Actor1: agent.RoleMatcher{Roles: []string{"*"}},
					Actor2: agent.RoleMatcher{Roles: []string{"*"}},
					Behavior: agent.BehaviorSpec{
						EngineKind:      "speak",
						TargetBinding:   "$actor2",
						SummaryTemplate: "严禁硬编码名 说话",
					},
					ExpectedOutcomes: []string{"speech"},
				},
				{
					ID:     "pro_" + domID + "_disabled_inst",
					Domain: domID,
					Name:   "尝试调用关闭制度extort",
					Actor1: agent.RoleMatcher{Roles: []string{"*"}},
					Actor2: agent.RoleMatcher{Roles: []string{"*"}},
					Behavior: agent.BehaviorSpec{
						EngineKind:      "extort",
						TargetBinding:   "$actor2",
						SummaryTemplate: "$actor1 勒索 $actor2",
					},
					ExpectedOutcomes: []string{"extorted"},
				},
			},
			ReactionRules: []agent.CausalReactionRule{
				{
					ID:                "rxn_" + domID + "_actor2",
					Domain:            domID,
					Name:              "角色2回应商议",
					TriggerEventKinds: []string{"speech"},
					ResponderPosition: "actor2",
					ResponderMatch:    agent.RoleMatcher{Roles: []string{"*"}},
					CausalSummary:     "当事角色2收到话语后回应",
					Branches: []agent.ReactionBranch{
						{
							ID:                 "br_reply",
							Stance:             "谈判妥协",
							Title:              "当场回应",
							Weight:             50,
							PersonalityWeights: map[string]float64{"careful": 1.9, "ruthless": 0.5},
							Behavior: agent.BehaviorSpec{
								EngineKind:      "speak",
								TargetBinding:   "$actor1",
								SummaryTemplate: "$responder 回应了 $actor1",
							},
						},
					},
				},
				{
					ID:                "rxn_" + domID + "_witness",
					Domain:            domID,
					Name:              "目击者表态",
					TriggerEventKinds: []string{"speech"},
					ResponderPosition: "witness",
					ResponderMatch:    agent.RoleMatcher{Roles: []string{"*"}},
					CausalSummary:     "同场目击者闻声介入",
					Branches: []agent.ReactionBranch{
						{
							ID:                 "br_chime",
							Stance:             "借力拉拢",
							Title:              "旁观插话",
							Weight:             40,
							PersonalityWeights: map[string]float64{"proud": 1.6},
							Behavior: agent.BehaviorSpec{
								EngineKind:      "speak",
								TargetBinding:   "$actor1",
								SummaryTemplate: "$responder 旁听 $actor1 与 $actor2 交谈后插话",
							},
						},
					},
				},
			},
		}
		contentBytes, _ := json.Marshal(payload)
		env := map[string]any{
			"choices": []map[string]any{
				{"finish_reason": "stop", "message": map[string]string{"content": string(contentBytes)}},
			},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 200, "total_tokens": 300},
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(env)
	}))
	defer srv.Close()

	outDir := t.TempDir()
	client := Client{
		Endpoint: srv.URL,
		Model:    "deepseek-mock",
	}
	sum, rb, err := ExpandWorldRulebook(context.Background(), w, client, RulebookExpandConfig{
		OutDir:      outDir,
		Concurrency: 6,
	})
	if err != nil {
		t.Fatalf("ExpandWorldRulebook failed: %v", err)
	}
	if promptLeak != "" {
		t.Fatalf("%s", promptLeak)
	}
	if !seenCalibration {
		t.Fatalf("expected dedicated trait_calibration request to run")
	}
	if len(seenDomains) != 5 {
		t.Fatalf("expected all 5 generic domains requested, got %d (%v)", len(seenDomains), seenDomains)
	}
	if sum.TotalRequests != 6 {
		t.Fatalf("expected 6 total requests (5 domains + 1 trait calibration), got %d", sum.TotalRequests)
	}
	if sum.TotalProactiveRules != 5 {
		t.Fatalf("expected 5 clean proactive rules (leaked + disabled dropped), got %d (audit=%+v)", sum.TotalProactiveRules, sum.Audit)
	}
	if sum.Audit.DroppedProactiveRules != 10 { // 5 leaked + 5 disabled extort
		t.Fatalf("expected 10 dropped proactive rules (5 leaked + 5 disabled), got %+v", sum.Audit)
	}
	if sum.Audit.WitnessReactionRules != 5 || sum.Audit.Actor2ReactionRules != 5 {
		t.Fatalf("expected 5 witness + 5 actor2 reaction rules, got %+v", sum.Audit)
	}
	if sum.Audit.HobbyWeightedProactiveRules != 5 || sum.Audit.SharedHobbyProactiveRules != 5 {
		t.Fatalf("expected 5 hobby-weighted & shared-hobby proactive rules, got %+v", sum.Audit)
	}
	if sum.Audit.PersonalityWeightedBranches != 10 {
		t.Fatalf("expected 10 personality-weighted reaction branches, got %+v", sum.Audit)
	}
	if sum.Audit.ProfessionEncounterRoles != 2 || sum.Audit.PersonalityStanceTraits != 3 || sum.Audit.PersonalityEncounterTraits != 2 || sum.Audit.HobbyActionKeys != 2 {
		t.Fatalf("expected merged profession_encounter(2), personality_stance_weights(3), encounter(2) & hobby_action_weights(2), got %+v", sum.Audit)
	}
	if _, leakedExtort := rb.HobbyActionWeights["sword_dueling"]["extort"]; leakedExtort {
		t.Fatalf("sanitizer failed to strip disabled engine_kind 'extort' from hobby_action_weights")
	}
	// Verify saved rulebook.json loads via agent.LoadRulebook and contains ZERO leaked actor names!
	loaded, err := agent.LoadRulebook(filepath.Join(outDir, "rulebook.json"))
	if err != nil {
		t.Fatalf("agent.LoadRulebook failed: %v", err)
	}
	if len(loaded.ProfessionEncounterModifiers) != 2 {
		t.Fatalf("expected 2 profession encounter modifiers loaded, got %+v", loaded.ProfessionEncounterModifiers)
	}
	if len(loaded.PersonalityEncounterModifiers) != 2 {
		t.Fatalf("expected 2 personality encounter modifiers loaded, got %+v", loaded.PersonalityEncounterModifiers)
	}
	if len(loaded.ProactiveRules) != len(rb.ProactiveRules) || len(loaded.ReactionRules) != len(rb.ReactionRules) {
		t.Fatalf("loaded rulebook mismatch")
	}
	rawLoaded, _ := json.Marshal(loaded)
	if strings.Contains(string(rawLoaded), "forbidden_hero_id") || strings.Contains(string(rawLoaded), "严禁硬编码名") {
		t.Fatalf("saved rulebook.json still contained leaked actor identity!")
	}
}

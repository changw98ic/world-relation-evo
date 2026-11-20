package optiongen

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
)

func TestMineHighLikelihoodCombinationsAndExpandResponsesEndToEnd(t *testing.T) {
	loaded, err := pack.LoadFile(filepath.Join("..", "..", "packs", "xianxia_sect.json"))
	if err != nil {
		t.Fatalf("LoadFile(xianxia_sect.json): %v", err)
	}
	w, err := loaded.Instantiate("xianxia-combo-test")
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}

	// Build dossiers for 3 Xianxia actors with tightly interacting options.
	d1, err := BuildDossier(w, "xuan_zhen", 30)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := BuildDossier(w, "han_xiao", 30)
	if err != nil {
		t.Fatal(err)
	}
	d3, err := BuildDossier(w, "shen_wuyue", 30)
	if err != nil {
		t.Fatal(err)
	}

	records := []ActorExpansionRecord{
		{
			WorldID:   w.ID,
			ActorID:   "xuan_zhen",
			ActorName: "玄真道人",
			Dossier:   d1,
			Catalog: ActorOptionCatalog{
				ActorID:          "xuan_zhen",
				ActorName:        "玄真道人",
				HorizonDays:      30,
				CharacterSummary: "坐镇青云主峰，兼任长老会稽查。",
				Options: []ExpandedOption{
					{
						ID:        "opt_01",
						Category:  "竞争博弈",
						Title:     "亲查云麓灵市灰账",
						DayWindow: [2]int{1, 10},
						Priority:  "urgent",
						WhenIf:    OptionCondition{Conditions: []string{"云麓灵市账目出现亏空"}, TriggerEvent: "长老会密信示警"},
						Action: OptionActionSpec{
							EngineKind:       "audit_corruption",
							TargetActorIDs:   []string{"han_xiao"},
							TargetLocation:   "valley_bazaar",
							ItemsOrResources: map[string]any{"spirit_stone": 5},
							Summary:          "携长老会手令亲查韩霄经手的灵石账册",
						},
						PotentialCollisions: []string{"韩霄：可能暗中销毁灰账", "沈无月：需配合封锁坊市"},
					},
				},
			},
		},
		{
			WorldID:   w.ID,
			ActorID:   "han_xiao",
			ActorName: "韩霄",
			Dossier:   d2,
			Catalog: ActorOptionCatalog{
				ActorID:          "han_xiao",
				ActorName:        "韩霄",
				HorizonDays:      30,
				CharacterSummary: "掌控云麓灵市坊市流水，暗通黑市。",
				Options: []ExpandedOption{
					{
						ID:        "opt_01",
						Category:  "竞争博弈",
						Title:     "连夜调账遮掩灵石亏空",
						DayWindow: [2]int{2, 9},
						Priority:  "urgent",
						WhenIf:    OptionCondition{Conditions: []string{"听闻宗门即将查账"}, TriggerEvent: "暗线通报玄真道人动身"},
						Action: OptionActionSpec{
							EngineKind:       "bribe",
							TargetActorIDs:   []string{"xuan_zhen"},
							TargetLocation:   "valley_bazaar",
							ItemsOrResources: map[string]any{"spirit_stone": 5},
							Summary:          "在云麓灵市临时补平灵石账目并试探玄真道人口风",
						},
						PotentialCollisions: []string{"玄真道人：当场查账识破"},
					},
				},
			},
		},
		{
			WorldID:   w.ID,
			ActorID:   "shen_wuyue",
			ActorName: "沈无月",
			Dossier:   d3,
			Catalog: ActorOptionCatalog{
				ActorID:          "shen_wuyue",
				ActorName:        "沈无月",
				HorizonDays:      30,
				CharacterSummary: "执法堂干练执事，手持青云执法令牌。",
				Options: []ExpandedOption{
					{
						ID:        "opt_01",
						Category:  "人脉结盟",
						Title:     "持令牌暗控云麓灵市出口",
						DayWindow: [2]int{2, 12},
						Priority:  "high",
						WhenIf:    OptionCondition{Conditions: []string{"玄真道人下山稽查"}, TriggerEvent: "接到密令盯紧韩霄"},
						Action: OptionActionSpec{
							EngineKind:       "arrest",
							TargetActorIDs:   []string{"han_xiao", "xuan_zhen"},
							TargetLocation:   "valley_bazaar",
							ItemsOrResources: map[string]any{"enforcement_token": 1},
							Summary:          "持青云执法令牌封锁云麓灵市暗巷，协助玄真道人查控韩霄",
						},
						PotentialCollisions: []string{"韩霄：企图转移灰账被拦截"},
					},
				},
			},
		},
	}

	scenarios := MineHighLikelihoodCombinations(records, MineConfig{TopN: 3})
	if len(scenarios) == 0 {
		t.Fatal("expected at least 1 high-likelihood combination scenario")
	}
	top := scenarios[0]
	if top.LikelihoodScore < 80 || top.CollisionType != "正面博弈" {
		t.Fatalf("unexpected top scenario: score=%d type=%s title=%s", top.LikelihoodScore, top.CollisionType, top.Title)
	}
	if len(top.Participants) != 3 {
		t.Fatalf("expected 3-actor triangle scenario, got %d participants", len(top.Participants))
	}
	if top.DayWindow != [2]int{2, 9} {
		t.Fatalf("expected day overlap [2 9], got %v", top.DayWindow)
	}

	// Verify response system prompt is 100% environment-agnostic and free of ancient village leaks.
	prompt := BuildResponseSystemPrompt(d1.World, top, d1.EngineActionKinds)
	for _, must := range []string{"青云宗与云麓灵市", "灵石", "修士", "执法堂", "长老会", "保护费/强行勒索", "强硬反制", "谈判妥协"} {
		if !strings.Contains(prompt, must) {
			t.Errorf("response system prompt missing %q", must)
		}
	}
	for _, forbid := range []string{"west_cottages", "西村农舍", "村民", "农夫", "山贼", "捕快", "衙门", "铜钱"} {
		if strings.Contains(prompt, forbid) {
			t.Errorf("response system prompt leaked ancient term %q", forbid)
		}
	}

	// Mock DS server for end-to-end ExpandResponses.
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		respJSON := `{
			"scenario_id": "scn_01",
			"scenario_title": "云麓灵市查账风波",
			"collision_summary": "玄真道人亲赴云麓灵市查账，韩霄连夜平账，沈无月持令控场。",
			"stakes": ["云麓灵市抽成控制权", "执法堂威信"],
			"responses": [
				{
					"id": "rsp_01",
					"responder_actor_id": "han_xiao",
					"responder_actor_name": "韩霄",
					"stance": "谈判妥协",
					"title": "主动补缴亏空换缓查",
					"trigger_condition": "当玄真道人当场封账且沈无月已封锁暗巷时",
					"probability_hint": "高",
					"probability_rationale": "韩霄精于权衡利弊，退路被堵时会优先破财消灾",
					"counter_to_option_ids": ["opt_01"],
					"action": {
						"engine_kind": "offer",
						"target_actor_ids": ["xuan_zhen"],
						"target_location": "valley_bazaar",
						"items_or_resources": {"spirit_stone": 5},
						"summary": "当场补缴五块下品灵石并呈交副账，恳请玄真道人宽限三日复核"
					},
					"counter_effects": ["灵市风波暂缓", "韩霄短期资金吃紧"],
					"escalation_hooks": ["若玄真道人仍执意深挖暗账，韩霄将借商盟向长老会施压"]
				},
				{
					"id": "rsp_02",
					"responder_actor_id": "xuan_zhen",
					"responder_actor_name": "玄真道人",
					"stance": "强硬反制",
					"title": "当众封存灵市主账册",
					"trigger_condition": "当韩霄试图以补款拖延或暗中转移灰账时",
					"probability_hint": "高",
					"probability_rationale": "玄真道人重门规公信，绝不容坊市主事蒙混过关",
					"counter_to_option_ids": ["opt_01"],
					"action": {
						"engine_kind": "audit_corruption",
						"target_actor_ids": ["han_xiao"],
						"target_location": "valley_bazaar",
						"items_or_resources": {"enforcement_token": 1},
						"summary": "责令沈无月当场封存云麓灵市近三月主账册并暂停韩霄调账权"
					},
					"counter_effects": ["灰账证据固定", "执法堂威望上升"],
					"escalation_hooks": ["韩霄可能暗中切断玉剑峰丹药原料渠道"]
				}
			]
		}`
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"finish_reason": "stop",
					"message":       map[string]string{"content": respJSON},
				},
			},
			"usage": map[string]int{"prompt_tokens": 200, "completion_tokens": 120, "total_tokens": 320},
		})
	}))
	defer server.Close()

	outDir := t.TempDir()
	summary, err := ExpandResponses(context.Background(), records, Client{
		Endpoint:   server.URL + "/v1",
		Model:      "deepseek-test",
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	}, ResponseExpandConfig{
		OptionsDir:  "test-options-dir",
		OutDir:      outDir,
		TopN:        1,
		Concurrency: 2,
	})
	if err != nil {
		t.Fatalf("ExpandResponses failed: %v", err)
	}
	if calls.Load() != 1 || summary.TotalScenarios != 1 || summary.TotalResponses != 2 {
		t.Fatalf("unexpected summary: calls=%d summary=%+v", calls.Load(), summary)
	}
	for _, mustFile := range []string{
		"combinations.json",
		"summary.json",
		"overview.md",
		filepath.Join("scenarios", "scn_01.json"),
		filepath.Join("by_actor", "han_xiao.json"),
		filepath.Join("by_actor", "xuan_zhen.json"),
	} {
		if _, err := os.Stat(filepath.Join(outDir, mustFile)); err != nil {
			t.Fatalf("missing output %s: %v", mustFile, err)
		}
	}
	rawHan, err := os.ReadFile(filepath.Join(outDir, "by_actor", "han_xiao.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hanIdx ActorResponseIndex
	if err := json.Unmarshal(rawHan, &hanIdx); err != nil {
		t.Fatal(err)
	}
	if hanIdx.ScenarioCount != 1 || hanIdx.ResponseCount != 1 || hanIdx.Scenarios[0].Responses[0].Stance != "谈判妥协" {
		t.Fatalf("unexpected by_actor index for han_xiao: %+v", hanIdx)
	}
}

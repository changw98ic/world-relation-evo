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
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestGeneratedOptionsRejectReversedWindow(t *testing.T) {
	w := sim.Village("invalid-window")
	dossier, err := BuildDossier(w, w.Actors[0].ID, 30)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []string{`"day_window":[7,3],`, `"day_window":[1,3,7],`} {
		_, err := decodeOptionCatalog(`{"options":[{`+window+`"id":"test"}]}`, dossier)
		if err == nil || !strings.Contains(err.Error(), "invalid day_window") {
			t.Errorf("expected %s to be rejected, got %v", window, err)
		}
	}
	cat, err := decodeOptionCatalog(`{"options":[{"id":"full"},{"id":"one-day","day_window":[7,7]}]}`, dossier)
	if err != nil || cat.Options[0].DayWindow != [2]int{1, 30} || cat.Options[1].DayWindow != [2]int{7, 7} {
		t.Fatalf("valid option windows were not preserved: %+v, %v", cat, err)
	}
}

func TestExpandWorldSendsOneRequestPerCharacterWithCompleteDossier(t *testing.T) {
	w := sim.Village("test-village")
	w.Actors = w.Actors[:3]

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			rw.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}
		userContent := req.Messages[1].Content
		if !strings.Contains(userContent, `"other_actors"`) || !strings.Contains(userContent, `"horizon_days":30`) || !strings.Contains(userContent, `"engine_action_kinds"`) {
			t.Errorf("dossier missing required sections")
		}
		// Extract actor_id from the user prompt prefix.
		actorID := "p01"
		for _, a := range w.Actors {
			if strings.Contains(userContent, "角色 "+a.ID+"（") {
				actorID = a.ID
				break
			}
		}
		respJSON := `{
			"actor_id": "` + actorID + `",
			"actor_name": "测试角色",
			"horizon_days": 30,
			"character_summary": "测试角色摘要",
			"core_drives": ["生存", "获利"],
			"key_tensions": ["缺水", "粮价波动"],
			"options": [
				{
					"id": "opt_01",
					"category": "trade",
					"title": "高价售粮",
					"day_window": [1, 15],
					"priority": "high",
					"when_if": {"conditions": ["items.food >= 4", "location == market_street"]},
					"action": {"engine_kind": "offer", "target_actor_ids": ["p02"], "target_location": "market_street", "items_or_resources": {"food": 2}, "summary": "向相邻角色挂牌售粮"},
					"expected_effects": ["获得铜钱", "库存下降"],
					"potential_collisions": ["p02: 可能压价或转向他人"],
					"follow_up_hooks": ["若被拒绝则降价或转卖他人"]
				}
			]
		}`
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"finish_reason": "stop",
					"message":       map[string]string{"content": "```json\n" + respJSON + "\n```"},
				},
			},
			"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150},
		})
	}))
	defer server.Close()

	outDir := t.TempDir()
	client := Client{
		Endpoint:   server.URL + "/v1",
		Model:      "deepseek-test",
		APIKey:     "test-key",
		HTTPClient: server.Client(),
	}
	cfg := ExpandConfig{
		OutDir:      outDir,
		HorizonDays: 30,
		Concurrency: 2,
	}
	summary, err := ExpandWorld(context.Background(), w, client, cfg)
	if err != nil {
		t.Fatalf("ExpandWorld failed: %v", err)
	}
	if calls.Load() != 3 || summary.TotalActors != 3 || summary.TotalRequests != 3 || summary.TotalOptions != 3 {
		t.Fatalf("unexpected counts: calls=%d summary=%+v", calls.Load(), summary)
	}
	for _, a := range w.Actors {
		p := filepath.Join(outDir, "actors", a.ID+".json")
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("missing actor output %s: %v", p, err)
		}
		var rec ActorExpansionRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("invalid json in %s: %v", p, err)
		}
		if rec.ActorID != a.ID || len(rec.Catalog.Options) != 1 || rec.Dossier.Actor.ID != a.ID {
			t.Fatalf("unexpected record for %s: %+v", a.ID, rec)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "summary.json")); err != nil {
		t.Fatalf("missing summary.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "overview.md")); err != nil {
		t.Fatalf("missing overview.md: %v", err)
	}
	cfg.Resume = true
	resumed, err := ExpandWorld(context.Background(), w, client, cfg)
	if err != nil || resumed.TotalRequests != 0 || calls.Load() != 3 || resumed.TotalOptions != summary.TotalOptions {
		t.Fatalf("identical request did not resume: calls=%d summary=%+v err=%v", calls.Load(), resumed, err)
	}
	for _, change := range []struct {
		name   string
		update func(*sim.World, *Client, *ExpandConfig)
	}{
		{"world", func(w *sim.World, _ *Client, _ *ExpandConfig) { w.ID = "another-world" }},
		{"horizon", func(_ *sim.World, _ *Client, c *ExpandConfig) { c.HorizonDays = 1 }},
		{"model", func(_ *sim.World, c *Client, _ *ExpandConfig) { c.Model = "another-model" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			newWorld, newClient, newConfig := *w, client, cfg
			change.update(&newWorld, &newClient, &newConfig)
			if _, err := ExpandWorld(context.Background(), &newWorld, newClient, newConfig); err == nil || !strings.Contains(err.Error(), "does not match the current request") || calls.Load() != 3 {
				t.Fatalf("different %s accepted saved output or made a model call: calls=%d err=%v", change.name, calls.Load(), err)
			}
		})
	}
}

func TestBuildSystemPromptIsEnvironmentAgnosticForXianxia(t *testing.T) {
	loaded, err := pack.LoadFile(filepath.Join("..", "..", "packs", "xianxia_sect.json"))
	if err != nil {
		t.Fatalf("LoadFile(xianxia_sect.json): %v", err)
	}
	w, err := loaded.Instantiate("xianxia-prompt-test")
	if err != nil {
		t.Fatalf("Instantiate: %v", err)
	}
	dossier, err := BuildDossier(w, "lin_qingfeng", 30)
	if err != nil {
		t.Fatalf("BuildDossier: %v", err)
	}
	for _, kind := range dossier.EngineActionKinds {
		if kind == "extort" {
			t.Fatal("expected disabled racketeering action 'extort' to be filtered out of EngineActionKinds")
		}
		if kind == "eat" || kind == "sleep" || kind == "wait" {
			t.Fatalf("expected physiological action %q to be excluded from EngineActionKinds", kind)
		}
	}
	prompt := BuildSystemPrompt(dossier)
	for _, mustContain := range []string{
		"青云宗与云麓灵市",
		"灵石",
		"修士",
		"执法堂",
		"长老会",
		"保护费/强行勒索",
		"主业经营",
		"资源交易",
		"sect_summit=青云主峰",
		"qi_pill=聚气灵丹",
		"refine_qi_pill=",
	} {
		if !strings.Contains(prompt, mustContain) {
			t.Errorf("Xianxia system prompt missing expected world context %q", mustContain)
		}
	}
	for _, forbidden := range []string{
		"west_cottages", "西村农舍", "村民", "农夫", "山贼", "捕快", "衙门", "铜钱", "林禾", "陈谷", "严药", "p01", "p02",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("Xianxia system prompt leaked ancient village string %q", forbidden)
		}
	}
}

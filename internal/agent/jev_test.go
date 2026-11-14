package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestJevOfficialChoiceContract(t *testing.T) {
	options := []Option{
		{ID: "wait", Label: "Wait one hour", Action: sim.Action{Kind: "wait"}},
		{ID: "talk-p02", Label: "Talk with p02", Action: sim.Action{Kind: "speak", TargetID: "p02"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected endpoint %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing official auth/content headers")
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 3 || body["state"] == nil || body["questions"] == nil || body["model"] == nil {
			t.Errorf("wrong request envelope: %v", body)
		}
		if body["messages"] != nil {
			t.Error("Jev must not use chat-completions messages")
		}
		var state sim.Perception
		if err := json.Unmarshal(body["state"], &state); err != nil {
			t.Fatal(err)
		}
		if state.Self.ID != "p01" {
			t.Error("wrong character perception")
		}
		var questions map[string]struct {
			Type         string            `json:"type"`
			Instructions string            `json:"instructions"`
			Criteria     map[string]string `json:"criteria"`
		}
		if err := json.Unmarshal(body["questions"], &questions); err != nil {
			t.Fatal(err)
		}
		q := questions["decision"]
		if q.Type != "choice" || q.Instructions == "" || len(q.Criteria) != 2 || !strings.Contains(q.Criteria["talk-p02"], "p02") {
			t.Errorf("wrong Choice schema: %+v", q)
		}
		fmt.Fprint(w, `{"model":"jev-1.14.0","answers":{"decision":{"type":"choice","choice":"talk-p02","confidence":0.1,"probabilities":{"wait":0.4,"talk-p02":0.6}}},"usage":{"input_tokens":120,"output_tokens":10}}`)
	}))
	defer server.Close()
	j := Jev{Endpoint: server.URL + "/v1/systemone", APIKey: "test-key"}
	selected, err := j.Select(context.Background(), sim.Village("test").View("p01"), options)
	if err != nil || selected != "talk-p02" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
}

func TestJevRejectsUnknownChoiceAndMissingAnswer(t *testing.T) {
	for _, body := range []string{`{"answers":{"decision":{"type":"choice","choice":"invented"}}}`, `{"answers":{}}`, `{"answers":{"decision":{"type":"noul","noul":1}}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			j := Jev{Endpoint: server.URL, APIKey: "test-key"}
			_, err := j.Select(context.Background(), sim.Perception{}, []Option{{ID: "wait", Action: sim.Action{Kind: "wait"}}})
			if err == nil {
				t.Fatal("invalid Choice response accepted")
			}
		})
	}
}
func TestJevOptionLimitAndAPIError(t *testing.T) {
	j := Jev{APIKey: "test-key"}
	if _, err := j.Select(context.Background(), sim.Perception{}, make([]Option, 256)); err == nil {
		t.Fatal("exceeded official option limit")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "quota exceeded", 429) }))
	defer server.Close()
	j.Endpoint = server.URL
	if _, err := j.Select(context.Background(), sim.Perception{}, []Option{{ID: "wait"}}); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("lost provider error: %v", err)
	}
}

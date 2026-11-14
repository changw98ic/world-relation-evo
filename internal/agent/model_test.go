package agent

import (
	"context"
	"encoding/json"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContentModelAdapter(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Error(r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"Hello"}`}}}})
	}))
	defer s.Close()
	m := ContentModel{Endpoint: s.URL + "/v1", Name: "test"}
	c, e := m.Generate(context.Background(), sim.Village("test").View("p01"), sim.Action{Kind: "speak", TargetID: "p02"})
	if e != nil || c.Text != "Hello" {
		t.Fatalf("%+v %v", c, e)
	}
}
func TestContentModelCannotReturnActionOrState(t *testing.T) {
	for _, content := range []string{`{"text":"hello","kind":"attack"}`, `{"target_id":"p03"}`, `{"health":999}`, `{"money":999}`, `{"food":999}`, `{"price":0}`} {
		t.Run(content, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
			}))
			defer s.Close()
			_, err := (ContentModel{Endpoint: s.URL, Name: "test"}).Generate(context.Background(), sim.Village("x").View("p01"), sim.Action{Kind: "speak", TargetID: "p02"})
			if err == nil {
				t.Fatal("accepted action/state fields from content model")
			}
		})
	}
}

type chooseKind struct{ kind, target string }

func (c chooseKind) Select(_ context.Context, _ sim.Perception, options []Option) (string, error) {
	for _, o := range options {
		if o.Action.Kind == c.kind && o.Action.TargetID == c.target {
			return o.ID, nil
		}
	}
	return "missing", nil
}

type exaggeratedContent struct{}

func (exaggeratedContent) Generate(context.Context, sim.Perception, sim.Action) (Content, error) {
	return Content{Text: "I gave you 999 food and healed you completely", Statement: &sim.Statement{Topic: "food_produced", Location: "fields", Quantity: 999}}, nil
}
func TestPipelineKeepsSystemEffectsAndSelectedTarget(t *testing.T) {
	w := sim.Village("test")
	a, b := &w.Actors[0], &w.Actors[1]
	before := a.Food
	p := Pipeline{Selector: chooseKind{kind: "offer", target: b.ID}, Generator: exaggeratedContent{}}
	x, err := p.Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	if x.Kind != "offer" || x.TargetID != b.ID || x.Food != 2 || x.Price != 2 || x.Statement != nil {
		t.Fatalf("content overwrote system action: %+v", x)
	}
	w.Apply(a.ID, x)
	if a.Food != before || b.Health != 100 {
		t.Fatal("generated words altered state")
	}
}
func TestTagsDefineOptionsAndResolverPermissions(t *testing.T) {
	w := sim.Village("tags")
	w.Actors[0].Tags = []string{"resident"}
	w.Actors[1].Health = 60
	for _, o := range Options(w.View("p01")) {
		if o.Action.Kind == "attack" || o.Action.Kind == "heal" || o.Action.Kind == "offer" || o.Action.Kind == "work" {
			t.Fatalf("tag-less capability: %+v", o)
		}
	}
	if e := w.Apply("p01", sim.Action{Kind: "attack", TargetID: "p02"}); e[0].Kind != "action_failed" || w.Actors[1].Health != 60 {
		t.Fatal("resolver ignored tags")
	}
	w.Actors[0].Tags = append(w.Actors[0].Tags, "fighter")
	w.Apply("p01", sim.Action{Kind: "attack", TargetID: "p02", Text: "dealt 999 damage"})
	if w.Actors[1].Health != 50 {
		t.Fatal("damage not system calculated")
	}
	w.Actors[0].Tags = append(w.Actors[0].Tags, "healer")
	w.Actors[0].Medicine = 1
	w.Apply("p01", sim.Action{Kind: "heal", TargetID: "p02", Text: "fully healed"})
	if w.Actors[1].Health != 65 || w.Actors[0].Medicine != 0 {
		t.Fatal("healing not system calculated")
	}
	view := w.View("p01")
	view.Self.Tags[0] = "changed"
	if w.Actors[0].Tags[0] == "changed" {
		t.Fatal("tag view aliases state")
	}
}

func TestTwoProviderPipeline(t *testing.T) {
	world := sim.Village("two-provider")
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/systemone":
			calls = append(calls, "jev")
			var body struct {
				Questions map[string]struct {
					Criteria map[string]string `json:"criteria"`
				} `json:"questions"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			choice := ""
			for id, label := range body.Questions["decision"].Criteria {
				if strings.Contains(label, `"kind":"speak"`) && strings.Contains(label, `"target_id":"p02"`) {
					choice = id
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": choice}}})
		case "/v1/chat/completions":
			calls = append(calls, "content")
			var body struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			var user struct {
				Selected sim.Action `json:"selected_action"`
			}
			json.Unmarshal([]byte(body.Messages[1].Content), &user)
			if user.Selected.Kind != "speak" || user.Selected.TargetID != "p02" {
				t.Error("content model did not receive fixed selection")
			}
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"Good morning"}`}}}})
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer server.Close()
	p := Pipeline{Selector: Jev{Endpoint: server.URL + "/v1/systemone", APIKey: "test"}, Generator: ContentModel{Endpoint: server.URL + "/v1", Name: "content"}}
	action, err := p.Decide(context.Background(), world.View("p01"))
	if err != nil {
		t.Fatal(err)
	}
	events := world.Apply("p01", action)
	if len(calls) != 2 || calls[0] != "jev" || calls[1] != "content" || events[0].Kind != "speech" || events[0].TargetID != "p02" || events[0].Action.Text != "Good morning" {
		t.Fatalf("pipeline incorrect: %v %+v", calls, events)
	}
}

func TestMovementDoesNotInventCausalSource(t *testing.T) {
	w := sim.Village("cause")
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "The fields are green", Statement: &sim.Statement{Topic: "food_produced", Location: "fields", Quantity: 2}})
	for _, o := range Options(w.View("p02")) {
		if o.Action.Kind == "move" && o.Action.BasisEventID != 0 {
			t.Fatal("destination mention was silently assigned as action cause")
		}
	}
}

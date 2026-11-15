package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
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
	forbidden := []string{
		`{"text":"hello","kind":"attack"}`,
		`{"target_id":"p03"}`,
		`{"health":999}`,
		`{"energy":999}`,
		`{"money":999}`,
		`{"items":{"food":999}}`,
		`{"house_id":"h01"}`,
		`{"recipe_id":"farm_grain"}`,
		`{"price":0}`,
	}
	for _, content := range forbidden {
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
		if o.Action.Kind == c.kind && (c.target == "" || o.Action.TargetID == c.target) {
			return o.ID, nil
		}
	}
	return "missing", nil
}

type exaggeratedContent struct{}

func (exaggeratedContent) Generate(context.Context, sim.Perception, sim.Action) (Content, error) {
	return Content{
		Text:      "I gave you 999 food and healed you completely",
		Statement: &sim.Statement{Topic: "item_produced", Item: "food", Location: "south_fields", Quantity: 999},
	}, nil
}

func TestPipelineKeepsSystemEffectsAndSelectedTarget(t *testing.T) {
	w := sim.Village("test")
	a, b := &w.Actors[0], &w.Actors[1]
	a.Items["food"] = 8
	before := a.Items["food"]
	p := Pipeline{Selector: chooseKind{kind: "offer", target: b.ID}, Generator: exaggeratedContent{}}
	x, err := p.Decide(context.Background(), w.View(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	if x.Kind != "offer" || x.TargetID != b.ID || x.Item != "food" || x.Quantity != 2 || x.Price != 2 || x.Statement != nil {
		t.Fatalf("content overwrote system action: %+v", x)
	}
	w.Apply(a.ID, x)
	if a.Items["food"] != before || b.Health != 100 {
		t.Fatal("generated words altered state")
	}
}

func TestTagsDefineOptionsAndResolverPermissions(t *testing.T) {
	w := sim.Village("tags")
	w.Actors[0].Tags = []string{"resident"}
	w.Actors[0].Professions = nil
	w.Actors[1].Health = 60
	for _, o := range Options(w.View("p01")) {
		if o.Action.Kind == "attack" || o.Action.Kind == "heal" || o.Action.Kind == "offer" || o.Action.Kind == "craft" {
			t.Fatalf("tag-less capability exposed: %+v", o)
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
	w.Actors[0].Items["medicine"] = 1
	w.Apply("p01", sim.Action{Kind: "heal", TargetID: "p02", Text: "fully healed"})
	if w.Actors[1].Health != 65 || w.Actors[0].Items["medicine"] != 0 {
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
	w.Actors[0].InsideHouse = ""
	w.Actors[1].InsideHouse = ""
	w.Apply("p01", sim.Action{
		Kind: "speak", TargetID: "p02", Text: "The well is full",
		Statement: &sim.Statement{Topic: "item_produced", Item: "water", Location: "creek_well", Quantity: 2},
	})
	moves := 0
	for _, o := range Options(w.View("p02")) {
		if o.Action.Kind == "move" {
			moves++
			if o.Action.BasisEventID != 0 {
				t.Fatal("destination mention was silently assigned as action cause")
			}
		}
	}
	if moves == 0 {
		t.Fatal("expected road move options from west_cottages")
	}
}

type countingGenerator struct{ calls int }

func (g *countingGenerator) Generate(context.Context, sim.Perception, sim.Action) (Content, error) {
	g.calls++
	return Content{Text: "spoken line"}, nil
}

func TestNonSpeechSkipsContentGenerator(t *testing.T) {
	w := sim.Village("skip-gen")
	w.Actors[3].Items["food"] = 8
	gen := &countingGenerator{}
	for _, act := range []chooseKind{
		{kind: "leave_house", target: ""},
		{kind: "craft", target: ""},
		{kind: "offer", target: "p03"},
	} {
		p := Pipeline{Selector: act, Generator: gen}
		if _, err := p.Decide(context.Background(), w.View("p04")); err != nil {
			t.Fatal(err)
		}
	}
	if gen.calls != 0 {
		t.Fatalf("expected 0 content calls for non-speech actions, got %d", gen.calls)
	}
	p := Pipeline{Selector: chooseKind{kind: "speak", target: "p02"}, Generator: gen}
	x, err := p.Decide(context.Background(), w.View("p01"))
	if err != nil || gen.calls != 1 || x.Text != "spoken line" {
		t.Fatalf("speak should invoke content generator once: calls=%d action=%+v err=%v", gen.calls, x, err)
	}
}

func TestOfflineSpeechDoesNotInventIdentityBasedDistortion(t *testing.T) {
	w := sim.Village("offline-expression")
	w.Actors[0].Memories = []sim.Memory{{EventID: 1, Tick: w.Tick, Source: "p01", Observation: &sim.Observation{Kind: "item_produced", Item: "food", Location: "south_fields", Quantity: 2}}}
	for _, stock := range []int{1, 20} {
		w.Actors[0].Items["food"] = stock
		for _, target := range []string{"p02", "p15", "p30"} {
			c, err := (TemplateContent{}).Generate(context.Background(), w.View("p01"), sim.Action{Kind: "speak", TargetID: target})
			if err != nil || c.Statement == nil || c.Statement.Quantity != 2 {
				t.Fatalf("offline fixture distorted production for stock=%d target=%s: %+v %v", stock, target, c, err)
			}
		}
	}
}

func TestOfflineRetellingPreservesClaimAndSource(t *testing.T) {
	p := sim.Village("retelling").View("p01")
	original := sim.Statement{Topic: "item_produced", Item: "", Location: "south_fields", Quantity: 17}
	p.Self.Memories = []sim.Memory{{EventID: 8, RootEventID: 2, Tick: p.Tick, Kind: "speech", Source: "p03", Statement: &original}}
	c, err := (TemplateContent{}).Generate(context.Background(), p, sim.Action{Kind: "speak", TargetID: "p02"})
	if err != nil || c.BasisEventID != 8 || c.Statement == nil || *c.Statement != original || !strings.Contains(c.Text, "p03 said") {
		t.Fatalf("retelling changed claim semantics or provenance: %+v %v", c, err)
	}
}

func TestContentModelUsesSituationWithoutPrescribingStance(t *testing.T) {
	w := sim.Village("expression")
	w.Actors[0].InsideHouse = ""
	w.Actors[1].InsideHouse = ""
	w.Actors[1].Location = w.Actors[0].Location
	w.Actors[0].Hunger = 73
	w.Actors[0].Trust["p02"] = -9
	w.Actors[0].Personality = "proud but fond of old friends"
	w.Actors[0].Goal = "rebuild the household"
	var claim = 3
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Perception  sim.Perception             `json:"perception"`
			Selected    sim.Action                 `json:"selected_action"`
			Interaction map[string]json.RawMessage `json:"interaction"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Perception.Self.Hunger != 73 || payload.Perception.Self.Personality != w.Actors[0].Personality || payload.Perception.Self.Goal != w.Actors[0].Goal || payload.Perception.Self.Trust["p02"] != -9 {
			t.Error("speaker circumstances missing from model context")
		}
		if payload.Selected.Kind != "speak" || payload.Selected.TargetID != "p02" {
			t.Error("selected action changed")
		}
		for _, key := range []string{"information_stance", "stance_reason"} {
			if _, ok := payload.Interaction[key]; ok {
				t.Errorf("prescribed stance remains: %s", key)
			}
		}
		content, _ := json.Marshal(Content{Text: fmt.Sprintf("我听说出了 %d 份粮食", claim), Statement: &sim.Statement{Topic: "item_produced", Item: "food", Location: "south_fields", Quantity: claim}})
		json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}})
	}))
	defer server.Close()
	p := Pipeline{Selector: chooseKind{kind: "speak", target: "p02"}, Generator: ContentModel{Endpoint: server.URL, Name: "test"}}
	for _, q := range []int{3, 17} {
		claim = q
		before, _ := json.Marshal(w.Actors)
		action, err := p.Decide(context.Background(), w.View("p01"))
		after, _ := json.Marshal(w.Actors)
		if err != nil || action.Kind != "speak" || action.TargetID != "p02" || action.Statement == nil || action.Statement.Quantity != q {
			t.Fatalf("expression was rewritten or changed action: %+v %v", action, err)
		}
		if string(before) != string(after) {
			t.Fatal("content generation changed actor state")
		}
	}
}

type countingFocalDecider struct {
	calls atomic.Int64
}

func (c *countingFocalDecider) Decide(_ context.Context, _ sim.Perception) (sim.Action, error) {
	c.calls.Add(1)
	return sim.Action{Kind: "wait"}, nil
}

type waitActiveDecider struct{}

func (waitActiveDecider) Decide(_ context.Context, _ sim.Perception) (sim.Action, error) {
	return sim.Action{Kind: "wait"}, nil
}

func TestTieredDeciderCapsFocalCallsPerTick(t *testing.T) {
	w := sim.LargeWorld("lod-million", 1_000_000)
	// Hydrate 20 Tier-2 xiucai characters so >20 actors qualify for focal LOD
	for i := 0; i < 20; i++ {
		if _, err := w.HydrateActor("cohort_crossroads_xiucai"); err != nil {
			t.Fatal(err)
		}
	}
	focal := &countingFocalDecider{}
	td := NewTieredDecider(focal, 5)
	td.Active = waitActiveDecider{}
	for tick := 0; tick < 3; tick++ {
		if _, err := w.Step(context.Background(), td); err != nil {
			t.Fatal(err)
		}
	}
	if got := focal.calls.Load(); got != 15 {
		t.Fatalf("expected exactly 5 focal calls per tick across 3 ticks (15 total), got %d", got)
	}
}

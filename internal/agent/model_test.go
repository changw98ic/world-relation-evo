package agent

import (
	"context"
	"encoding/json"
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

func TestIdentityPairShapesExaggerationAndConservatismAcrossProfessions(t *testing.T) {
	w := sim.Village("dyad-stance")
	// Put p01 (farmer), p02 (farmer), p15 (carpenter), p30 (guard) outdoors at crossroads.
	for _, idx := range []int{0, 1, 14, 29} {
		w.Actors[idx].InsideHouse = ""
		w.Actors[idx].Location = "crossroads"
	}

	// p01 harvests 2 food at south_fields first, then returns to crossroads.
	w.Actors[0].Location = "south_fields"
	w.Apply("p01", sim.Action{Kind: "craft", RecipeID: "farm_grain"})
	for w.Actors[0].Task != nil {
		w.Tick++
		w.Advance("p01")
	}
	w.Actors[0].Location = "crossroads"
	w.Actors[0].Items["food"] = 6 // below food SurplusThreshold (8)

	tc := TemplateContent{}

	// 1. Farmer -> fellow Farmer: candid (quantity stays 2)
	cPeer, err := tc.Generate(context.Background(), w.View("p01"), sim.Action{Kind: "speak", TargetID: "p02"})
	if err != nil || cPeer.Statement == nil || cPeer.Statement.Quantity != 2 {
		t.Fatalf("farmer->farmer expected candid quantity 2, got %+v err=%v", cPeer, err)
	}

	// 2. Farmer (limited surplus, food=6 < 8) -> Carpenter buyer: conservative (2 -> 1)
	cBuyerTight, err := tc.Generate(context.Background(), w.View("p01"), sim.Action{Kind: "speak", TargetID: "p15"})
	if err != nil || cBuyerTight.Statement == nil || cBuyerTight.Statement.Quantity != 1 {
		t.Fatalf("farmer(low surplus)->carpenter expected conservative quantity 1, got %+v err=%v", cBuyerTight, err)
	}

	// 3. Farmer (ample surplus, food=10 >= 8) -> Carpenter buyer: exaggerate (2 -> 4)
	w.Actors[0].Items["food"] = 10
	cBuyerAmple, err := tc.Generate(context.Background(), w.View("p01"), sim.Action{Kind: "speak", TargetID: "p15"})
	if err != nil || cBuyerAmple.Statement == nil || cBuyerAmple.Statement.Quantity != 4 {
		t.Fatalf("farmer(ample surplus)->carpenter expected exaggerated quantity 4, got %+v err=%v", cBuyerAmple, err)
	}

	// 4. Farmer -> Armed Guard (coercive power): conservative (2 -> 1)
	cGuard, err := tc.Generate(context.Background(), w.View("p01"), sim.Action{Kind: "speak", TargetID: "p30"})
	if err != nil || cGuard.Statement == nil || cGuard.Statement.Quantity != 1 {
		t.Fatalf("farmer->guard expected conservative quantity 1, got %+v err=%v", cGuard, err)
	}

	// 5. Carpenter -> Farmer retelling food news: exaggerate (4 -> 8)
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p15", Text: cBuyerAmple.Text, BasisEventID: cBuyerAmple.BasisEventID, Statement: cBuyerAmple.Statement})
	cCarpenterToFarmer, err := tc.Generate(context.Background(), w.View("p15"), sim.Action{Kind: "speak", TargetID: "p02"})
	if err != nil || cCarpenterToFarmer.Statement == nil || cCarpenterToFarmer.Statement.Quantity != 8 {
		t.Fatalf("carpenter->farmer expected exaggerated quantity 8, got %+v err=%v", cCarpenterToFarmer, err)
	}

	// 6. Symmetric non-farmer check: Carpenter producing planks speaking to Farmer plank-buyer!
	farmerListener := sim.Neighbor{ID: "p01", Name: w.Actors[0].Name, Role: "farmer", Tags: w.Actors[0].Tags, Health: 100}
	w.Actors[14].Items["plank"] = 1 // below plank SurplusThreshold (3)
	if st, _ := CommunicationStance(w.Catalog, w.Actors[14], farmerListener, "plank"); st != "conservative" {
		t.Fatalf("carpenter(tight plank)->farmer expected conservative, got %q", st)
	}
	w.Actors[14].Items["plank"] = 4 // >= plank SurplusThreshold (3)
	if st, _ := CommunicationStance(w.Catalog, w.Actors[14], farmerListener, "plank"); st != "exaggerate" {
		t.Fatalf("carpenter(ample plank)->farmer expected exaggerate, got %q", st)
	}

	var gotStance, gotListenerRole string
	s := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		var payload struct {
			Interaction struct {
				InformationStance string       `json:"information_stance"`
				Listener          sim.Neighbor `json:"listener"`
			} `json:"interaction"`
		}
		json.Unmarshal([]byte(req.Messages[1].Content), &payload)
		gotStance = payload.Interaction.InformationStance
		gotListenerRole = payload.Interaction.Listener.Role
		json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"南亩到处都在出粮","statement":{"topic":"item_produced","item":"food","location":"south_fields","quantity":8}}`}}}})
	}))
	defer s.Close()
	_, err = (ContentModel{Endpoint: s.URL, Name: "test"}).Generate(context.Background(), w.View("p15"), sim.Action{Kind: "speak", TargetID: "p02"})
	if err != nil || gotStance != "exaggerate" || gotListenerRole != "farmer" {
		t.Fatalf("ContentModel dyad payload wrong: stance=%q listenerRole=%q err=%v", gotStance, gotListenerRole, err)
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

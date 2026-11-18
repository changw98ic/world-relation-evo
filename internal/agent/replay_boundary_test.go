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

// This uses two local protocol endpoints to prove replay never requests a new
// decision. It is not a real-provider end-to-end or language-quality test.
func TestLocalProtocolRecordingReplaysFromNonzeroTickWithoutProviderCalls(t *testing.T) {
	var choices, expressions atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/systemone":
			choices.Add(1)
			var request struct {
				Questions map[string]struct {
					Criteria map[string]string `json:"criteria"`
				} `json:"questions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				rw.WriteHeader(400)
				return
			}
			selected := ""
			for id, label := range request.Questions["decision"].Criteria {
				_, raw, found := strings.Cut(label, "; system-defined action: ")
				if !found {
					continue
				}
				var a sim.Action
				if err := json.Unmarshal([]byte(raw), &a); err != nil {
					t.Error(err)
					rw.WriteHeader(400)
					return
				}
				if a.Kind == "speak" {
					selected = id
					break
				}
			}
			if selected == "" {
				t.Error("fixture has no local speech candidate")
				rw.WriteHeader(400)
				return
			}
			_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": selected}}})
		case "/chat/completions":
			expressions.Add(1)
			_ = json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"我听说粮食已经送到。","statement":{"topic":"item_produced","item":"food","quantity":0}}`}}}})
		default:
			t.Error("unexpected local protocol endpoint")
			rw.WriteHeader(404)
		}
	}))
	defer server.Close()
	w := boundaryWorld()
	w.Tick = 37
	w.Actors = w.Actors[:2]
	checkpoint := []byte(boundaryJSON(t, w))
	pipeline := Pipeline{Selector: Jev{Endpoint: server.URL + "/systemone", APIKey: "fixture", Client: server.Client()}, Generator: ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}}
	var logs []sim.TickDecisionLog
	var originalEvents []sim.Event
	for i := 0; i < 2; i++ {
		events, log, err := w.StepRecorded(context.Background(), pipeline)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
		originalEvents = append(originalEvents, events...)
	}
	if logs[0].Tick != 37 || len(logs[0].Decisions) != 2 || choices.Load() == 0 || expressions.Load() == 0 {
		t.Fatal("the recording did not exercise both provider adapters at the checkpoint tick")
	}
	beforeChoices, beforeExpressions := choices.Load(), expressions.Load()
	server.Close()
	replayed, events, err := sim.ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if choices.Load() != beforeChoices || expressions.Load() != beforeExpressions {
		t.Fatal("replay made new model requests")
	}
	if boundaryJSON(t, replayed) != boundaryJSON(t, w) || boundaryJSON(t, events) != boundaryJSON(t, originalEvents) {
		t.Fatal("replay changed recorded state, dialogue, or event order")
	}
	if _, _, err := sim.ReplayFromCheckpoint(checkpoint, logs[1:], w.Tick); err == nil {
		t.Fatal("missing first checkpoint decision silently became a successful replay")
	}
}

func TestRecordedAndReplayedDecisionsOwnTheirNestedClaimsAndContractTerms(t *testing.T) {
	view := sim.Perception{Tick: 37, Self: sim.Actor{ID: "alice"}}
	chosen := sim.Action{Kind: "propose_growth_care", TargetID: "bob", Care: &sim.GrowthCareAgreement{GrowthID: 1, OwnerKind: "actor", OwnerID: "alice", CaretakerKind: "actor", CaretakerID: "bob", SharePermille: 500}, Statement: &sim.Statement{Quantity: 3}}
	expected := boundaryJSON(t, chosen)
	recorder := sim.NewRecordingDecider(boundaryDecider(func(context.Context, sim.Perception) (sim.Action, error) { return chosen, nil }))
	returned, err := recorder.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	returned.Care.SharePermille, returned.Statement.Quantity = 0, 999
	log, ok := recorder.LogForTick(37)
	if !ok || boundaryJSON(t, log.Decisions["alice"]) != expected {
		t.Fatal("changing the settled action rewrote the recorded decision")
	}
	log.Decisions["alice"].Care.SharePermille = 1000
	log.Decisions["alice"].Statement.Quantity = 999
	fresh, ok := recorder.LogForTick(37)
	if !ok || boundaryJSON(t, fresh.Decisions["alice"]) != expected {
		t.Fatal("changing a returned log rewrote recorder-owned contract terms")
	}
	replay := sim.NewReplayDecider([]sim.TickDecisionLog{fresh})
	fresh.Decisions["alice"].Care.SharePermille = 0
	fresh.Decisions["alice"].Statement.Quantity = 999
	first, err := replay.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, first) != expected {
		t.Fatal("changing input logs rewrote replay's recorded contract")
	}
	first.Care.SharePermille, first.Statement.Quantity = 0, 999
	again, err := replay.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, again) != expected {
		t.Fatal("a previous replay caller rewrote the next replayed decision")
	}
}

func TestLocalProviderCancellationRemainsAnError(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { requests.Add(1); _, _ = fmt.Fprint(rw, `{}`) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	view := boundaryWorld().View("alice")
	_, choiceErr := (Jev{Endpoint: server.URL, APIKey: "fixture", Client: server.Client()}).Select(ctx, view, []Option{{ID: "wait", Action: sim.Action{Kind: "wait"}}})
	_, contentErr := (ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}).Generate(ctx, view, sim.Action{Kind: "speak", TargetID: "bob"})
	if choiceErr == nil || contentErr == nil || requests.Load() != 0 {
		t.Fatal("cancelled requests succeeded or reached a provider")
	}
}

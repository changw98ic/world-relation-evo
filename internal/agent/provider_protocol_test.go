package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// All providers in this file are loopback servers. These tests verify wire
// contracts and error handling; they provide no evidence of real model quality.
func TestLocalJevLargeChoicesKeepEveryOriginalOptionReachable(t *testing.T) {
	for _, count := range []int{256, 264, 511} {
		for _, mixed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/mixed=%v", count, mixed), func(t *testing.T) {
				options := make([]Option, count)
				for i := range options {
					kind := "offer"
					if mixed && i%2 == 0 {
						kind = "speak"
					}
					options[i] = Option{ID: fmt.Sprintf("choice-%d", i), Label: "fixture", Action: sim.Action{Kind: kind, TargetID: fmt.Sprint(i)}}
				}
				chosen := options[count-1].ID
				var calls atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					call := calls.Add(1)
					var body struct {
						Questions map[string]struct{ Criteria map[string]string }
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						rw.WriteHeader(400)
						return
					}
					criteria := body.Questions["decision"].Criteria
					if len(criteria) == 0 || len(criteria) > 255 {
						t.Error("a staged question exceeded the provider's declared option range")
					}
					if call == 1 {
						for _, original := range options {
							found := 0
							for _, description := range criteria {
								if strings.Contains(description, original.ID+": ") {
									found++
								}
							}
							if found != 1 {
								t.Errorf("original option %s occurred in %d initial groups", original.ID, found)
							}
						}
					}
					selected := ""
					if _, exists := criteria[chosen]; exists {
						selected = chosen
					} else {
						for id, description := range criteria {
							if strings.Contains(description, chosen+": ") {
								selected = id
							}
						}
					}
					if selected == "" {
						t.Error("the final original option was omitted from the hierarchy")
					}
					_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": selected}}})
				}))
				defer server.Close()
				id, err := (Jev{Endpoint: server.URL, APIKey: "fixture", Client: server.Client()}).Select(context.Background(), boundaryWorld().View("alice"), options)
				if err != nil || id != chosen || calls.Load() < 2 {
					t.Fatalf("last original candidate could not be selected: choice=%s calls=%d err=%v", id, calls.Load(), err)
				}
			})
		}
	}
}

func TestLocalJevStagedChoiceFailureRemainsAnErrorWithoutRetry(t *testing.T) {
	options := make([]Option, 264)
	for i := range options {
		options[i] = Option{ID: fmt.Sprintf("choice-%d", i), Action: sim.Action{Kind: "wait"}}
	}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 2 {
			http.Error(rw, "controlled second-stage outage", 503)
			return
		}
		var body struct {
			Questions map[string]struct{ Criteria map[string]string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		for id := range body.Questions["decision"].Criteria {
			_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": id}}})
			return
		}
	}))
	defer server.Close()
	id, err := (Jev{Endpoint: server.URL, APIKey: "fixture", Client: server.Client()}).Select(context.Background(), boundaryWorld().View("alice"), options)
	if err == nil || !strings.Contains(err.Error(), "controlled second-stage outage") || id != "" || calls.Load() != 2 {
		t.Fatalf("staged error was hidden or retried: choice=%s calls=%d err=%v", id, calls.Load(), err)
	}
}

func TestLocalJevProtocolTransmitsClosedChoiceAtBothValidLimits(t *testing.T) {
	for _, count := range []int{1, 255} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			options := make([]Option, count)
			for i := range options {
				options[i] = Option{ID: fmt.Sprintf("choice-%d", i), Label: "fixture", Action: sim.Action{Kind: "wait"}}
			}
			chosen := options[count-1].ID
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/systemone" || r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("request method, endpoint, or headers violated the contract")
				}
				var envelope map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
					t.Error(err)
					rw.WriteHeader(400)
					return
				}
				if len(envelope) != 3 || envelope["messages"] != nil {
					t.Error("typed choice request has an unexpected envelope")
				}
				var model string
				if err := json.Unmarshal(envelope["model"], &model); err != nil || model != "fixture-jev" {
					t.Error("model identity was lost")
				}
				var state sim.Perception
				if err := json.Unmarshal(envelope["state"], &state); err != nil || state.Self.ID != "alice" {
					t.Error("actor perception was lost")
				}
				var questions map[string]struct {
					Type     string            `json:"type"`
					Criteria map[string]string `json:"criteria"`
				}
				if err := json.Unmarshal(envelope["questions"], &questions); err != nil {
					t.Error(err)
				}
				q := questions["decision"]
				if q.Type != "choice" || len(q.Criteria) != count {
					t.Error("closed choice alternatives were lost")
				}
				for _, option := range options {
					if !strings.Contains(q.Criteria[option.ID], `"kind":"wait"`) {
						t.Errorf("missing system action for %s", option.ID)
					}
				}
				_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": chosen}}})
			}))
			defer server.Close()
			id, err := (Jev{Endpoint: server.URL + "/systemone", APIKey: "fixture-key", Name: "fixture-jev", Client: server.Client()}).Select(context.Background(), boundaryWorld().View("alice"), options)
			if err != nil || id != chosen || calls.Load() != 1 {
				t.Fatalf("choice=%q calls=%d err=%v", id, calls.Load(), err)
			}
		})
	}
}

func TestLocalJevProtocolRejectsInvalidRequestsBeforeHTTP(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { calls.Add(1); rw.WriteHeader(500) }))
	defer server.Close()
	for _, tc := range []struct {
		name    string
		options []Option
		key     string
	}{
		{"empty", nil, "key"}, {"large_request_with_empty_identities", make([]Option, 256), "key"},
		{"empty_identity", []Option{{}}, "key"}, {"duplicate_identity", []Option{{ID: "x"}, {ID: "x"}}, "key"},
		{"missing_credential", []Option{{ID: "x"}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := (Jev{Endpoint: server.URL, APIKey: tc.key, Client: server.Client()}).Select(context.Background(), sim.Perception{}, tc.options)
			if err == nil {
				t.Fatal("invalid choice request succeeded")
			}
		})
	}
	if calls.Load() != 0 {
		t.Fatal("invalid requests reached the provider")
	}
}

func TestLocalJevProtocolErrorsCannotBecomeOfflineDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
	}{
		{"unknown_choice", `{"answers":{"decision":{"type":"choice","choice":"invented"}}}`, 200},
		{"wrong_answer_type", `{"answers":{"decision":{"type":"text","choice":"x"}}}`, 200},
		{"missing_answer", `{"answers":{}}`, 200}, {"malformed", `{`, 200},
		{"provider_busy", `busy`, 429}, {"provider_failure", `failed`, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				rw.WriteHeader(tc.status)
				_, _ = fmt.Fprint(rw, tc.response)
			}))
			defer server.Close()
			p := Pipeline{Selector: Jev{Endpoint: server.URL, APIKey: "fixture", Client: server.Client()}}
			if _, err := p.Decide(context.Background(), boundaryWorld().View("alice")); err == nil {
				t.Fatal("invalid external answer became a successful action")
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected retry or omitted provider request")
			}
		})
	}
}

func localContentServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": response}}}})
	}))
	t.Cleanup(server.Close)
	return server
}

func TestLocalContentProtocolCarriesSelectionAndMemoryIDs(t *testing.T) {
	w := boundaryWorld()
	w.Actors[0].Memories = []sim.Memory{{EventID: 17, RootEventID: 4, Source: "bob", Kind: "speech", Statement: &sim.Statement{Topic: "item_produced", Item: "food", Quantity: 3}}}
	view := w.View("alice")
	action := sim.Action{Kind: "speak", TargetID: "bob"}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("content endpoint or authentication changed")
		}
		var request struct {
			Model    string `json:"model"`
			Messages []struct{ Role, Content string }
			Format   map[string]string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		if request.Model != "fixture-content" || request.Format["type"] != "json_object" || len(request.Messages) != 2 {
			t.Error("content request envelope violated the contract")
			rw.WriteHeader(400)
			return
		}
		var payload struct {
			Perception  sim.Perception `json:"perception"`
			Selected    sim.Action     `json:"selected_action"`
			Interaction struct {
				IDs      []int64      `json:"citable_memory_event_ids"`
				Listener sim.Neighbor `json:"listener"`
			} `json:"interaction"`
		}
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &payload); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(payload.Selected, action) || payload.Perception.Self.ID != "alice" || payload.Interaction.Listener.ID != "bob" || !reflect.DeepEqual(payload.Interaction.IDs, []int64{17}) {
			t.Error("content request changed selected behavior or citable memory identities")
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"text":"我听说有三份粮食。","basis_event_id":17,"statement":{"topic":"item_produced","item":"food","quantity":3}}`}}}})
	}))
	defer server.Close()
	content, err := (ContentModel{Endpoint: server.URL + "/v1/", APIKey: "fixture-key", Name: "fixture-content", Client: server.Client()}).Generate(context.Background(), view, action)
	if err != nil || content.BasisEventID != 17 || content.Statement == nil || content.Statement.Quantity != 3 {
		t.Fatalf("content=%+v err=%v", content, err)
	}
}

func TestLocalContentProtocolRejectsBehaviorStateAndTrailingData(t *testing.T) {
	for _, response := range []string{
		`null`, `{"text":"claim","assertions":[]}`,
		`{"text":"claim","kind":"transfer_house"}`, `{"text":"claim","target_id":"charlie"}`,
		`{"text":"claim","items":{"food":100}}`, `{"text":"claim","money":100}`,
		`{"text":"claim","price":0}`, `{"text":"claim","house_id":"home"}`,
		`{"text":"claim","statement":{"quantity":3,"money":100}}`,
		`{"text":"claim"} {"kind":"attack"}`, `{"text":"claim"} garbage`,
	} {
		t.Run(response, func(t *testing.T) {
			server := localContentServer(t, response)
			_, err := (ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}).Generate(context.Background(), boundaryWorld().View("alice"), sim.Action{Kind: "speak", TargetID: "bob"})
			if err == nil {
				t.Fatal("invalid external content was accepted")
			}
		})
	}
}

func TestLocalContentProtocolRejectsUnheldCitation(t *testing.T) {
	// Root event 4 identifies provenance, but the speaker only owns memory 17.
	w := boundaryWorld()
	w.Actors[0].Memories = []sim.Memory{{EventID: 17, RootEventID: 4}}
	server := localContentServer(t, `{"text":"I witnessed it","basis_event_id":4}`)
	_, err := (ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}).Generate(context.Background(), w.View("alice"), sim.Action{Kind: "speak", TargetID: "bob"})
	if err == nil {
		t.Fatal("invented citation was silently turned into successful uncited speech")
	}
}

func TestLocalContentProtocolFailuresRemainVisible(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"busy", "busy", 429}, {"provider_failure", "failed", 500},
		{"malformed_envelope", `{`, 200}, {"missing_completion", `{"choices":[]}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				rw.WriteHeader(tc.status)
				_, _ = fmt.Fprint(rw, tc.body)
			}))
			defer server.Close()
			_, err := (ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}).Generate(context.Background(), boundaryWorld().View("alice"), sim.Action{Kind: "speak", TargetID: "bob"})
			if err == nil || calls.Load() != 1 {
				t.Fatal("provider failure became empty successful content or an unrequested retry")
			}
		})
	}
}

func TestLocalContentProtocolAllowsOptionalEmptyTextAndZeroClaim(t *testing.T) {
	server := localContentServer(t, `{"text":"","statement":{"quantity":0}}`)
	content, err := (ContentModel{Endpoint: server.URL, Name: "fixture", Client: server.Client()}).Generate(context.Background(), boundaryWorld().View("alice"), sim.Action{Kind: "speak", TargetID: "bob"})
	if err != nil || content.Text != "" || content.Statement == nil || content.Statement.Quantity != 0 {
		t.Fatalf("legal empty text/zero assertion rejected: %+v %v", content, err)
	}
}

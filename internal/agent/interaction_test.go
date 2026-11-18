package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type quoteFixture struct {
	terms sim.TradeTerms
	err   error
}

func (q quoteFixture) Generate(context.Context, sim.Perception, sim.Action) (Content, error) {
	return Content{}, errors.New("unexpected speech request")
}
func (q quoteFixture) Quote(context.Context, sim.Perception, sim.Action) (sim.TradeTerms, error) {
	return q.terms, q.err
}

func TestCounterofferCrossesTicksAndReplayWithoutTransferringBeforeAssent(t *testing.T) {
	w := parcelWorld()
	w.Actors = w.Actors[:2]
	checkpoint := []byte(boundaryJSON(t, w))
	choose := boundarySelector(func(_ context.Context, p sim.Perception, options []Option) (string, error) {
		for _, o := range options {
			if p.Tick == 1 && p.Self.ID == "alice" && !o.PrepareQuote && o.Action.Kind == "offer" {
				return o.ID, nil
			}
			if p.Tick == 2 && p.Self.ID == "bob" && (o.PrepareQuote && o.Action.OfferID != 0 || o.ID == "quote-confirm") {
				return o.ID, nil
			}
			if p.Tick == 3 && p.Self.ID == "alice" && o.Action.Kind == "accept" {
				return o.ID, nil
			}
		}
		for _, o := range options {
			if o.Action.Kind == "wait" {
				return o.ID, nil
			}
		}
		return "", fmt.Errorf("fixture missing intended action")
	})
	pipe := Pipeline{Selector: choose, Generator: quoteFixture{terms: sim.TradeTerms{Quantity: 2, Price: 4, Currency: "coins"}}}
	var logs []sim.TickDecisionLog
	var events []sim.Event
	for i := 0; i < 3; i++ {
		es, log, err := w.StepRecorded(context.Background(), pipe)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
		events = append(events, es...)
		if i < 2 {
			if w.Actors[0].Items["parcel"] != 3 || w.Actors[1].Items["parcel"] != 0 || w.Actors[0].Money != 50 || w.Actors[1].Money != 50 {
				t.Fatal("proposal transferred assets")
			}
			if len(w.Offers) != 1 {
				t.Fatal("missing cross-turn offer")
			}
			v := w.View("alice")
			found := false
			for _, x := range v.Interactions {
				if x.Kind == "offer" && x.ID == w.Offers[0].ID && x.AwaitingActorID == w.Offers[0].ResponderID() {
					found = true
				}
			}
			if !found {
				t.Fatal("response state not visible next tick")
			}
		}
	}
	if w.Actors[0].Items["parcel"] != 1 || w.Actors[1].Items["parcel"] != 2 || w.Actors[0].Money != 54 || w.Actors[1].Money != 46 || len(w.Offers) != 0 {
		t.Fatal("counteroffer did not settle exact accepted terms")
	}
	replayed, replayedEvents, err := sim.ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, replayed) != boundaryJSON(t, w) || boundaryJSON(t, replayedEvents) != boundaryJSON(t, events) {
		t.Fatal("counteroffer replay changed outcome")
	}
	// Resume at the pending-counteroffer checkpoint, not just at the run's start.
	mid, _, err := sim.ReplayFromCheckpoint(checkpoint, logs[:2], 3)
	if err != nil {
		t.Fatal(err)
	}
	resumed, _, err := sim.ReplayFromCheckpoint([]byte(boundaryJSON(t, mid)), logs[2:], w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, resumed) != boundaryJSON(t, w) {
		t.Fatal("pending proposal was not durable across restart")
	}
}

func TestCounterofferIdentityExpiryAndSettlementFailure(t *testing.T) {
	for _, scenario := range []string{"third_party", "stale", "expired", "stock_lost", "buyer_unfunded", "dead", "absent", "repeat"} {
		t.Run(scenario, func(t *testing.T) {
			w := parcelWorld()
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "offer", "bob"))
			old := w.Offers[0]
			counter := sim.Action{Kind: "offer", OfferID: old.ID, TargetID: "alice", Item: "parcel", Quantity: 2, Price: 4}
			if scenario == "third_party" {
				boundaryRejected(t, w, "charlie", counter)
				return
			}
			if scenario == "expired" {
				w.Tick = old.Expires + 1
				boundaryRejected(t, w, "bob", counter)
				return
			}
			boundaryApply(t, w, "bob", counter)
			next := w.Offers[0]
			if next.ID == old.ID || next.PreviousID != old.ID || next.Seller != "alice" || next.Buyer != "bob" || next.ResponderID() != "alice" || next.Expires != old.Expires {
				t.Fatal("counteroffer changed identity or lifetime")
			}
			if scenario == "stale" {
				boundaryRejected(t, w, "bob", sim.Action{Kind: "accept", OfferID: old.ID})
				boundaryRejected(t, w, "bob", counter)
				return
			}
			switch scenario {
			case "stock_lost":
				w.Actors[0].Items["parcel"] = 1
			case "buyer_unfunded":
				w.Actors[1].Money = 3
			case "dead":
				w.Actors[1].Dead = true
			case "absent":
				w.Actors[1].Location = "remote"
			}
			if scenario == "repeat" {
				boundaryApply(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
				before := boundaryJSON(t, w.Actors)
				boundaryRejected(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
				if w.Actors[0].Items["parcel"] != 1 || w.Actors[1].Items["parcel"] != 2 || w.Actors[1].Money != 46 {
					t.Fatal("repeated acceptance transferred again")
				}
				_ = before
				return
			}
			beforeSeller, beforeBuyer := w.Actors[0].Items["parcel"], w.Actors[1].Items["parcel"]
			moneySeller, moneyBuyer := w.Actors[0].Money, w.Actors[1].Money
			boundaryRejected(t, w, "alice", sim.Action{Kind: "accept", OfferID: next.ID})
			if w.Actors[0].Items["parcel"] != beforeSeller || w.Actors[1].Items["parcel"] != beforeBuyer || w.Actors[0].Money != moneySeller || w.Actors[1].Money != moneyBuyer {
				t.Fatal("failed settlement moved assets")
			}
		})
	}
}

func TestQuoteConfirmationCanDeclineWithoutChangingOriginalOffer(t *testing.T) {
	for _, kind := range []string{"wait", "reject"} {
		t.Run(kind, func(t *testing.T) {
			w := parcelWorld()
			boundaryApply(t, w, "alice", boundaryOption(t, w, "alice", "offer", "bob"))
			original := w.Offers[0]
			calls := 0
			selector := boundarySelector(func(_ context.Context, _ sim.Perception, os []Option) (string, error) {
				calls++
				for _, o := range os {
					if calls == 1 && o.PrepareQuote && o.Action.OfferID == original.ID || calls == 2 && o.Action.Kind == kind {
						return o.ID, nil
					}
				}
				return "", errors.New("missing optional choice")
			})
			action, err := (Pipeline{Selector: selector, Generator: quoteFixture{terms: sim.TradeTerms{Quantity: 1, Price: 0, Currency: "coins"}}}).Decide(context.Background(), w.View("bob"))
			if err != nil {
				t.Fatal(err)
			}
			if action.Kind != kind || !reflect.DeepEqual(w.Offers[0], original) {
				t.Fatal("drafting modified the live offer or forced confirmation")
			}
			boundaryApply(t, w, "bob", action)
			if kind == "wait" && !reflect.DeepEqual(w.Offers[0], original) {
				t.Fatal("deferring changed the offer")
			}
			if kind == "reject" && len(w.Offers) != 0 {
				t.Fatal("rejection did not close the original offer")
			}
			assertParcelAssets(t, w, false)
		})
	}
}

func TestStructuredQuoteProtocolRejectsMissingOrInjectedFields(t *testing.T) {
	for _, raw := range []string{`{"quantity":1,"price":0,"currency":"coins"}`, `{"quantity":1,"currency":"coins"}`, `{"quantity":1,"price":0,"currency":"coins","target_id":"charlie"}`, `{"quantity":-1,"price":0,"currency":"coins"}`, `{"quantity":1,"price":0,"currency":"coins"} {}`, `null`} {
		t.Run(raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": raw}}}})
			}))
			defer server.Close()
			w := parcelWorld()
			draft := boundaryOption(t, w, "alice", "offer", "bob")
			terms, err := (ContentModel{Endpoint: server.URL, Client: server.Client()}).Quote(context.Background(), w.View("alice"), draft)
			if raw == `{"quantity":1,"price":0,"currency":"coins"}` {
				if err != nil || terms.Price != 0 {
					t.Fatal("explicit free quote rejected", err)
				}
			} else if err == nil {
				t.Fatal("invalid quote accepted")
			}
		})
	}
}

func TestQuoteFailureDoesNotSettleOrSilentlyFallBack(t *testing.T) {
	w := parcelWorld()
	before := boundaryJSON(t, w)
	selector := boundarySelector(func(_ context.Context, _ sim.Perception, os []Option) (string, error) {
		for _, o := range os {
			if o.PrepareQuote {
				return o.ID, nil
			}
		}
		return "", errors.New("missing quote")
	})
	_, _, err := w.StepRecorded(context.Background(), Pipeline{Selector: selector, Generator: quoteFixture{err: errors.New("quote provider failed")}})
	if err == nil || boundaryJSON(t, w) != before {
		t.Fatal("quote failure was hidden or partially settled")
	}
}

func TestQuoteLocalHTTPFlowRetainsTraceAuthority(t *testing.T) {
	var requests, quotes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			quotes.Add(1)
			_ = json.NewEncoder(rw).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"quantity":2,"price":4,"currency":"coins"}`}}}})
			return
		}
		requests.Add(1)
		var req struct {
			State     sim.Perception `json:"state"`
			Questions map[string]struct {
				Criteria map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		id := ""
		criteria := req.Questions["decision"].Criteria
		if _, ok := criteria["quote-confirm"]; ok {
			id = "quote-confirm"
		} else {
			for k, label := range criteria {
				_, raw, ok := strings.Cut(label, "; system-defined action: ")
				if !ok {
					continue
				}
				var a sim.Action
				if err := json.Unmarshal([]byte(raw), &a); err != nil {
					t.Error(err)
					rw.WriteHeader(400)
					return
				}
				if req.State.Tick == 1 && req.State.Self.ID == "alice" && a.Kind == "offer" && !strings.HasPrefix(k, "q") || req.State.Tick == 2 && req.State.Self.ID == "bob" && a.OfferID != 0 && strings.HasPrefix(k, "q") || req.State.Tick == 3 && req.State.Self.ID == "alice" && a.Kind == "accept" {
					id = k
					break
				}
			}
		}
		if id == "" {
			for k, label := range criteria {
				if strings.Contains(label, `"kind":"wait"`) {
					id = k
					break
				}
			}
		}
		if id == "" {
			t.Error("missing fixture choice")
			rw.WriteHeader(400)
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": id}}})
	}))
	defer server.Close()
	dir := t.TempDir()
	writer, err := runtrace.Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := parcelWorld()
	w.Actors = w.Actors[:2]
	if err = writer.WriteJSON("initial.json", w); err != nil {
		t.Fatal(err)
	}
	record := func(kind string, tick int64, data any) {
		t.Helper()
		if err := writer.Record(kind, tick, "", data); err != nil {
			t.Fatal(err)
		}
	}
	record("run_start", w.Tick, map[string]any{"decider": "jev"})
	pipe := Pipeline{Selector: Jev{Endpoint: server.URL + "/systemone", APIKey: "fixture", Client: server.Client(), Trace: writer}, Generator: ContentModel{Endpoint: server.URL, Client: server.Client(), Trace: writer}, Trace: writer}
	for i := 0; i < 3; i++ {
		tick := w.Tick
		record("tick_begin", tick, map[string]any{"world": w})
		events, decisions, err := w.StepRecorded(context.Background(), pipe)
		if err != nil {
			t.Fatal(err)
		}
		record("tick_settled", tick, map[string]any{"world": w, "events": events, "decisions": decisions})
	}
	record("run_end", w.Tick, map[string]any{"completed": true})
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := runtrace.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range report.Findings {
		if f.Status == "violation" || f.Status == "observed_error" {
			t.Fatalf("valid quote chain failed trace authority: %+v", f)
		}
	}
	if requests.Load() != 7 || quotes.Load() != 1 || w.Actors[1].Items["parcel"] != 2 {
		t.Fatalf("wrong external calls or settlement: choices=%d quotes=%d", requests.Load(), quotes.Load())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "trace.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	initial, err := os.ReadFile(filepath.Join(dir, "initial.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, defect := range []string{"missing_both", "missing_request", "missing_response", "group_only", "wrong_choice"} {
		t.Run(defect, func(t *testing.T) {
			destination := t.TempDir()
			if err := os.WriteFile(filepath.Join(destination, "initial.json"), initial, 0600); err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			var modified bytes.Buffer
			encoder := json.NewEncoder(&modified)
			var seq uint64
			confirm := false
			for {
				var e runtrace.Entry
				if err := decoder.Decode(&e); err == io.EOF {
					break
				} else if err != nil {
					t.Fatal(err)
				}
				if e.ActorID == "bob" && e.Tick == 2 {
					if e.Kind == "quote_options" {
						confirm = true
					}
					if confirm && (e.Kind == "jev_request" || e.Kind == "jev_response") {
						if defect == "missing_both" || defect == "missing_request" && e.Kind == "jev_request" || defect == "missing_response" && e.Kind == "jev_response" {
							continue
						}
						if defect == "group_only" && e.Kind == "jev_request" {
							var data map[string]any
							if err := json.Unmarshal(e.Data, &data); err != nil {
								t.Fatal(err)
							}
							data["selection_stage"] = "group"
							e.Data, _ = json.Marshal(data)
						}
						if defect == "wrong_choice" && e.Kind == "jev_response" {
							var data map[string]any
							if err := json.Unmarshal(e.Data, &data); err != nil {
								t.Fatal(err)
							}
							data["body"] = `{"answers":{"decision":{"type":"choice","choice":"not-quote-confirm"}}}`
							e.Data, _ = json.Marshal(data)
						}
					}
				}
				seq++
				e.Seq = seq
				if err := encoder.Encode(e); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(destination, "trace.ndjson"), modified.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := runtrace.Analyze(destination)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range result.Findings {
				if f.Rule == "model_evidence" && f.ActorID == "bob" && f.Tick == 2 {
					found = true
				}
			}
			if !found {
				t.Fatal("missing or wrong final confirmation was accepted as evidenced")
			}
		})
	}
}

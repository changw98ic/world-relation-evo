package runtrace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// These are controlled counterexamples for the reviewer, not model E2E runs.
// Each completed trace differs from its valid control by one evidence defect.
func TestTraceReviewDetectsCounterexamplesEvenWhenRunCompleted(t *testing.T) {
	for _, defect := range []string{"control", "available_action", "content_authority", "individual_holdings", "zero_output", "model_evidence", "plan_availability", "agreement_terms", "care_replacement"} {
		t.Run(defect, func(t *testing.T) {
			dir := t.TempDir()
			writer, err := Create(dir)
			if err != nil {
				t.Fatal(err)
			}
			a := sim.Actor{ID: "person", Location: "place", Health: 100, Energy: 80, Items: map[string]int{"food": 1}, Memories: []sim.Memory{{EventID: 17, RootEventID: 4}}}
			initial := sim.World{ID: "counterexample", Actors: []sim.Actor{a}}
			if err := writer.WriteJSON("initial.json", initial); err != nil {
				t.Fatal(err)
			}
			record := func(kind string, data any) {
				t.Helper()
				if err := writer.Record(kind, 0, "person", data); err != nil {
					t.Fatal(err)
				}
			}
			mode := "rules"
			if defect == "model_evidence" {
				mode = "jev"
			}
			record("run_start", map[string]any{"decider": mode})
			record("tick_begin", map[string]any{"world": initial})
			selected := sim.Action{Kind: "wait"}
			p := sim.Perception{Self: a}
			if defect == "plan_availability" {
				p.ActivePlan = &sim.PlanDirectiveView{NextStep: "craft:unavailable"}
			}
			record("decision_input", map[string]any{"perception": p, "options": []map[string]any{{"id": "allowed", "action": selected}}})
			option := "allowed"
			if defect == "available_action" {
				option = "invented"
			}
			record("decision_selected", map[string]any{"option_id": option, "action": selected})
			ready := selected
			if defect == "content_authority" {
				ready.Quantity = 10
			}
			record("decision_ready", map[string]any{"action": ready})
			after := sim.World{ID: initial.ID, Tick: 1, Actors: []sim.Actor{a}}
			if defect == "individual_holdings" {
				after.Actors[0].Items = map[string]int{"food": -1}
			}
			events := []sim.Event{}
			if defect == "agreement_terms" {
				proposal := sim.Action{AgreementID: 1, TargetID: "buyer", Item: "food", Quantity: 3, Price: 7}
				after.Continuity = &sim.ContinuityState{Agreements: []sim.Agreement{{ID: 1, SupplierID: "person", BuyerID: "buyer", Item: "food", Quantity: 2, Price: 2}}}
				events = append(events, sim.Event{ID: 1, ActorID: "person", Kind: "agreement_proposed", Action: proposal})
			}
			if defect == "care_replacement" {
				for i, caretaker := range []string{"first", "second"} {
					terms := &sim.GrowthCareAgreement{GrowthID: 1, CaretakerID: caretaker}
					events = append(events, sim.Event{ID: int64(i + 1), Kind: "growth_care_accepted", Action: sim.Action{Care: terms}})
				}
			}
			if defect == "zero_output" {
				events = append(events, sim.Event{ID: 1, ActorID: "person", Kind: "crafted", Payload: map[string]any{"quantity": 0}})
			}
			record("tick_settled", map[string]any{"world": after, "events": events, "decisions": sim.TickDecisionLog{Tick: 0, Decisions: map[string]sim.Action{"person": ready}}})
			record("run_end", map[string]any{"completed": true})
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			report, err := Analyze(dir)
			if err != nil {
				t.Fatal(err)
			}
			if !report.RunCompleted || !report.TraceComplete {
				t.Fatal("the control fact is a completed trace")
			}
			if report.Correctness != "not_established" {
				t.Fatal("completion was incorrectly turned into a validity verdict")
			}
			if defect == "control" {
				if len(report.Findings) != 0 {
					t.Fatalf("valid control was rejected: %+v", report.Findings)
				}
				return
			}
			status := "violation"
			if defect == "zero_output" {
				status = "needs_review"
			} else if defect == "model_evidence" {
				status = "observed_error"
			}
			found := false
			for _, f := range report.Findings {
				if f.Rule == defect && f.Status == status && len(f.Evidence) > 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("completed run hid counterexample %s: %+v", defect, report.Findings)
			}
		})
	}
}

func TestTraceReviewAcceptsZeroDeliveryThatRetainsPaidCredit(t *testing.T) {
	dir := t.TempDir()
	writer, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := sim.Actor{ID: "person", Location: "place", Health: 100, Energy: 80}
	initial := sim.World{ID: "retained", Actors: []sim.Actor{a}}
	if err := writer.WriteJSON("initial.json", initial); err != nil {
		t.Fatal(err)
	}
	record := func(kind string, data any) {
		t.Helper()
		if err := writer.Record(kind, 0, "person", data); err != nil {
			t.Fatal(err)
		}
	}
	selected := sim.Action{Kind: "wait"}
	record("run_start", map[string]any{"decider": "rules"})
	record("tick_begin", map[string]any{"world": initial})
	record("decision_input", map[string]any{"perception": sim.Perception{Self: a}, "options": []map[string]any{{"id": "allowed", "action": selected}}})
	record("decision_selected", map[string]any{"option_id": "allowed", "action": selected})
	record("decision_ready", map[string]any{"action": selected})
	events := []sim.Event{{ID: 1, ActorID: "person", Kind: "crafted", Payload: map[string]any{"quantity": 0, "yield_credit_before": 0, "yield_credit_after": 80}}}
	record("tick_settled", map[string]any{"world": sim.World{ID: initial.ID, Tick: 1, Actors: []sim.Actor{a}}, "events": events, "decisions": sim.TickDecisionLog{Tick: 0, Decisions: map[string]sim.Action{"person": selected}}})
	record("run_end", map[string]any{"completed": true})
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range report.Findings {
		if finding.Rule == "zero_output" {
			t.Fatalf("retained funded fraction was treated as discarded output: %+v", finding)
		}
	}
}

func TestTraceReviewCannotTreatTruncationAsSuccess(t *testing.T) {
	dir := t.TempDir()
	writer, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteJSON("initial.json", sim.World{ID: "interrupted"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record("run_start", 0, "", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.RunCompleted || report.TraceComplete || len(report.Findings) == 0 {
		t.Fatalf("interruption hidden: %+v", report)
	}
	path := filepath.Join(dir, "trace.ndjson")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte("{broken")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(dir); err == nil {
		t.Fatal("corrupt evidence was turned into an empty successful review")
	}
}

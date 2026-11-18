package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
)

func TestCLIRejectsUnexpectedArgumentsBeforeOpeningAnyWorld(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid-database-url-for-argument-test")
	for _, args := range [][]string{{"run", "-h"}, {"-steps", "1", "extra"}, {"inspect", "-kind", "conservation", "extra"}, {"replay", "-to-tick", "0", "extra"}, {"audit-trace", "-trace-dir", "missing", "extra"}} {
		var out, stderr bytes.Buffer
		err := runArgs(args, &out, &stderr)
		if err == nil || !strings.Contains(err.Error(), "unexpected arguments") || out.Len() != 0 {
			t.Fatalf("args %q reached world IO or ignored an argument: %v", args, err)
		}
	}
}

func TestCLILargePopulationUsesJevForEveryIdleDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var request struct {
			Questions map[string]struct{ Criteria map[string]string }
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		for id, description := range request.Questions["decision"].Criteria {
			if strings.Contains(description, `"kind":"wait"`) {
				_ = json.NewEncoder(rw).Encode(map[string]any{"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": id}}})
				return
			}
		}
		t.Error("the local choice fixture did not receive a wait option")
		rw.WriteHeader(400)
	}))
	defer server.Close()
	for key, value := range map[string]string{"WRE_JEV_URL": server.URL, "WRE_JEV_KEY": "fixture", "WRE_JEV_MODEL": "fixture", "WRE_MODEL_URL": server.URL, "WRE_MODEL_KEY": "fixture", "WRE_MODEL": "fixture"} {
		t.Setenv(key, value)
	}
	dir := filepath.Join(t.TempDir(), "all-jev-trace")
	var out, stderr bytes.Buffer
	if err := runArgs([]string{"-offline-run", "-decider", "jev", "-pop", "31", "-steps", "1", "-pace", "0", "-quiet", "-trace-dir", dir}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	report, err := runtrace.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Decisions <= 8 || report.JevResponses != report.Decisions {
		t.Fatalf("idle actors bypassed model decisions: choices=%d recorded_decisions=%d", report.JevResponses, report.Decisions)
	}
	for _, finding := range report.Findings {
		if finding.Rule == "decision_coverage" || finding.Rule == "model_evidence" || finding.Rule == "settled_decision" {
			t.Fatalf("model routing was not fully evidenced: %+v", finding)
		}
	}
}

// Evidence contract: recorded decisions must reproduce the exported state.
// This tests replay and evidence plumbing, not whether the world's rules are true.
func TestCLIRecordedFlowCanBeReplayedWithoutModels(t *testing.T) {
	dir := t.TempDir()
	checkpoint := filepath.Join(dir, "checkpoint.json")
	decisions := filepath.Join(dir, "decisions.json")
	state := filepath.Join(dir, "state.json")
	replayed := filepath.Join(dir, "replayed.json")
	trace := filepath.Join(dir, "trace")
	var out, stderr bytes.Buffer
	if err := runArgs([]string{"-pack", "../../packs/mortal_watershed.json", "-offline-run", "-decider", "rules", "-steps", "4", "-pace", "0", "-quiet", "-trace-dir", trace, "-out-checkpoint", checkpoint, "-out-decisions", decisions, "-out-state", state}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := runArgs([]string{"replay", "-checkpoint-file", checkpoint, "-decisions-file", decisions, "-to-tick", "4", "-out-state", replayed}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	reproduction, err := os.ReadFile(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, reproduction) {
		t.Fatal("recorded decisions did not reproduce exported state")
	}
	out.Reset()
	if err := runArgs([]string{"audit-trace", "-trace-dir", trace}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	var report runtrace.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if !report.RunCompleted || !report.TraceComplete || report.TicksSettled != 4 {
		t.Fatalf("evidence incomplete: %+v", report)
	}
	if report.Correctness != "not_established" {
		t.Fatal("successful execution was incorrectly reported as simulation validity")
	}
}

// Fault injection is deliberately local. It verifies that a failed external
// dependency leaves reviewable evidence and cannot be reported as a completed run.
func TestCLIProviderFailureLeavesOriginalTraceAndFailedRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "controlled provider outage", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	for key, value := range map[string]string{
		"WRE_JEV_URL": server.URL, "WRE_JEV_KEY": "local-contract-key", "WRE_JEV_MODEL": "local-contract-model",
		"WRE_MODEL_URL": server.URL, "WRE_MODEL_KEY": "local-content-key", "WRE_MODEL": "local-content-model",
	} {
		t.Setenv(key, value)
	}
	trace := filepath.Join(t.TempDir(), "failed-trace")
	var out, stderr bytes.Buffer
	err := runArgs([]string{"-offline-run", "-decider", "jev", "-steps", "1", "-pace", "0", "-quiet", "-trace-dir", trace}, &out, &stderr)
	if err == nil {
		t.Fatal("provider failure was turned into a successful run")
	}
	report, err := runtrace.Analyze(trace)
	if err != nil {
		t.Fatal(err)
	}
	if report.RunCompleted || !report.TraceComplete || report.RunError == "" || report.JevResponses == 0 {
		t.Fatalf("failure evidence missing: %+v", report)
	}
	for _, row := range report.Timeline {
		if row.After != nil {
			t.Fatal("an aborted step must not invent an after-state")
		}
	}
	raw, err := os.ReadFile(filepath.Join(trace, "trace.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("controlled provider outage")) {
		t.Fatal("original provider response was lost")
	}
	if bytes.Contains(raw, []byte("local-contract-key")) || bytes.Contains(raw, []byte("local-content-key")) {
		t.Fatal("trace exposed credentials")
	}
}

func TestCLIRemovedModelRoutingFlagsFailBeforeWorldIO(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid-database-url-for-argument-test")
	for _, flag := range []string{"-focal-limit", "-focal-stride"} {
		var out, stderr bytes.Buffer
		err := runArgs([]string{flag, "1"}, &out, &stderr)
		if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") || out.Len() != 0 {
			t.Fatalf("obsolete routing flag %s was silently accepted: %v", flag, err)
		}
	}
}

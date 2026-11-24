package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestCLILargePopulationUsesLocalPlanning(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected model call", http.StatusBadRequest)
	}))
	defer server.Close()
	t.Setenv("WRE_JEV_URL", server.URL)
	t.Setenv("WRE_JEV_KEY", "unused-model-fixture")
	dir := filepath.Join(t.TempDir(), "planning-trace")
	var out, stderr bytes.Buffer
	if err := runArgs([]string{"-offline-run", "-pop", "31", "-steps", "4", "-pace", "0", "-quiet", "-trace-dir", dir}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	report, err := runtrace.Analyze(dir)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || report.JevResponses != 0 || report.Decisions <= 8 {
		t.Fatalf("local decisions=%d, external requests=%d", report.Decisions, calls.Load())
	}
	for _, finding := range report.Findings {
		if finding.Status == "violation" || finding.Rule == "decision_coverage" {
			t.Fatalf("missing or inconsistent planning evidence: %+v", finding)
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
	if err := runArgs([]string{"-pack", "../../packs/mortal_watershed.json", "-offline-run", "-steps", "4", "-pace", "0", "-quiet", "-trace-dir", trace, "-out-checkpoint", checkpoint, "-out-decisions", decisions, "-out-state", state}, &out, &stderr); err != nil {
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

func TestCLIRemovedModelRoutingFlagsFailBeforeWorldIO(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid-database-url-for-argument-test")
	for _, flag := range []string{"-focal-limit", "-focal-stride", "-decider", "-playbook-dir", "-rulebook", "-rulebook-overlay"} {
		var out, stderr bytes.Buffer
		err := runArgs([]string{flag, "1"}, &out, &stderr)
		if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") || out.Len() != 0 {
			t.Fatalf("obsolete routing flag %s was silently accepted: %v", flag, err)
		}
	}
}

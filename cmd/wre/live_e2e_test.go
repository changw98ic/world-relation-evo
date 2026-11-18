//go:build live

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
)

// This is an external workflow probe, not a proof that the model's behavior or
// the simulated world is reasonable. It launches the real executable twice,
// calls the configured Jev/content providers, persists to PostgreSQL, and replays
// persisted decisions. The 16+16 steps exercise a restart and the checkpoint at
// step 24; they are an observation window, not an application limit or a claim
// about long-term survival. Original evidence remains on disk after the test.
func TestLiveModelWorkflowRecordsEvidenceAcrossDatabaseRestart(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	loadDotEnv(filepath.Join(root, ".env"))
	for _, key := range []string{"WRE_JEV_KEY", "WRE_MODEL_URL", "WRE_MODEL", "DATABASE_URL"} {
		if os.Getenv(key) == "" {
			t.Fatalf("live workflow requires %s; this is a failure, not a skipped model test", key)
		}
	}
	dir := os.Getenv("WRE_LIVE_ARTIFACT_DIR")
	if dir == "" {
		dir, err = os.MkdirTemp("", "wre-live-logic-")
	} else {
		err = os.Mkdir(dir, 0700)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LIVE_ARTIFACT_DIR=%s", dir)
	worldID := fmt.Sprintf("live-logic-%d", time.Now().UnixNano())
	finished := false
	replayed := false
	defer func() {
		summary := map[string]any{"world_id": worldID, "workflow_completed": finished, "replay_equal": replayed,
			"test_failed": t.Failed(), "simulation_correctness": "not_established",
			"review_required": "Review original model calls and actor timelines; successful workflow execution is not simulation validity.",
			"scenario":        "Nine active characters from mortal_watershed; anonymous cohorts removed so every active decision uses Jev, without tiered offline decisions. Two real processes run 16 steps each, checkpoint interval 24."}
		raw, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "workflow.json"), append(raw, '\n'), 0600); err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	if deadline, ok := t.Deadline(); ok {
		cancel()
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
	}
	defer cancel()
	binary := filepath.Join(t.TempDir(), "wre")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/wre")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	p, err := pack.LoadFile(filepath.Join(root, "packs", "mortal_watershed.json"))
	if err != nil {
		t.Fatal(err)
	}
	p.ID = worldID
	p.Cohorts = nil
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(fixture, raw, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(name string, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		for suffix, data := range map[string][]byte{"stdout": stdout.Bytes(), "stderr": stderr.Bytes()} {
			if writeErr := os.WriteFile(filepath.Join(dir, name+"."+suffix), data, 0600); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		if err != nil {
			t.Fatalf("%s failed: %v; inspect %s and its trace (completion is not assumed)", name, err, dir)
		}
		return stdout.Bytes()
	}
	totalContentCalls := 0
	for _, segment := range []string{"first", "resumed"} {
		traceDir := filepath.Join(dir, segment)
		args := []string{"-world", worldID, "-decider", "jev", "-steps", "16", "-pace", "0", "-quiet", "-checkpoint-every", "24", "-trace-dir", traceDir}
		if segment == "first" {
			args = append(args, "-pack", fixture)
		}
		run(segment, args...)
		audit := run(segment+"-audit", "audit-trace", "-trace-dir", traceDir)
		if err := os.WriteFile(filepath.Join(traceDir, "review.json"), audit, 0600); err != nil {
			t.Fatal(err)
		}
		var report runtrace.Report
		if err := json.Unmarshal(audit, &report); err != nil {
			t.Fatal(err)
		}
		if !report.RunCompleted || !report.TraceComplete || report.TicksSettled != 16 || report.PersistedTicks != 16 {
			t.Fatalf("%s workflow/evidence incomplete; inspect %s", segment, traceDir)
		}
		if report.Decisions == 0 || report.JevResponses != report.Decisions {
			t.Fatalf("%s did not call Jev for every recorded active decision; inspect %s", segment, traceDir)
		}
		totalContentCalls += report.ContentResponses
		t.Logf("%s: decisions=%d content_calls=%d findings_for_review=%d; simulation validity not established", segment, report.Decisions, report.ContentResponses, len(report.Findings))
	}
	if totalContentCalls == 0 {
		t.Fatalf("workflow did not exercise the real content model; inspect %s", dir)
	}
	final, err := os.ReadFile(filepath.Join(dir, "resumed", "final.json"))
	if err != nil {
		t.Fatal(err)
	}
	replayPath := filepath.Join(dir, "replayed.json")
	run("replay", "replay", "-world", worldID, "-to-tick", "32", "-out-state", replayPath)
	reproduction, err := os.ReadFile(replayPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(final, reproduction) {
		t.Fatalf("DB checkpoint plus recorded actions did not reproduce final state; inspect %s", dir)
	}
	replayed = true
	run("inspect", "inspect", "-world", worldID, "-kind", "conservation")
	finished = true
}

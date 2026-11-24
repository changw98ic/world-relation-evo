package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCLILargeNDJSONWorksForChronicleInspectAndReplayInputs(t *testing.T) {
	dir := t.TempDir()
	const size = 16 * 1024 * 1024
	content := strings.Repeat("a", size)
	events := filepath.Join(dir, "events.ndjson")
	raw := `{"id":1,"kind":"speech","payload":{"text":"` + content + `"}}` + "\n"
	if err := os.WriteFile(events, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"inspect", "-kind", "lineage", "-event", "1", "-events-file", events},
		{"chronicle", "-events-file", events, "-format", "json"},
	} {
		if err := runArgs(args, io.Discard, io.Discard); err != nil {
			t.Fatalf("%s rejected a valid large record: %v", args[0], err)
		}
	}
	decisions := filepath.Join(dir, "decisions.ndjson")
	raw = `{"tick":0,"decisions":{"person":{"kind":"speak","text":"` + content + `"}}}` + "\n"
	if err := os.WriteFile(decisions, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	logs, err := readDecisionLogsFile(decisions)
	if err != nil || len(logs) != 1 || len(logs[0].Decisions["person"].Text) != size {
		t.Fatalf("replay did not read the full decision: logs=%d err=%v", len(logs), err)
	}
}

func TestCLIChronicleSubcommandMarkdownAndJSON(t *testing.T) {
	dir := t.TempDir()
	w := sim.Village("cli-chronicle")
	statePath := filepath.Join(dir, "state.json")
	eventsPath := filepath.Join(dir, "events.json")
	mdOutPath := filepath.Join(dir, "chronicle.md")

	stateRaw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	evs := []sim.Event{
		{ID: 1, Tick: 10, ActorID: "world", Kind: "world_event_triggered", Payload: map[string]any{"text": "Relic emerged"}},
		{ID: 2, Tick: 12, ActorID: w.Actors[0].ID, Kind: "world_event_claimed", Payload: map[string]any{"text": "Claimed relic", "root_event_id": int64(1)}},
	}
	evRaw, err := json.Marshal(evs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventsPath, evRaw, 0o644); err != nil {
		t.Fatal(err)
	}

	// Markdown to file
	var stdout, stderr bytes.Buffer
	if err := runArgs([]string{"chronicle", "-events-file", eventsPath, "-state-file", statePath, "-format", "md", "-out", mdOutPath}, &stdout, &stderr); err != nil {
		t.Fatalf("chronicle md failed: %v (stderr=%s)", err, stderr.String())
	}
	mdBytes, err := os.ReadFile(mdOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mdBytes), "# World Chronicle: cli-chronicle") || !strings.Contains(string(mdBytes), "world_event_claimed") {
		t.Fatalf("unexpected markdown output:\n%s", string(mdBytes))
	}

	// JSON to stdout
	stdout.Reset()
	stderr.Reset()
	if err := runArgs([]string{"chronicle", "-events-file", eventsPath, "-state-file", statePath, "-format", "json"}, &stdout, &stderr); err != nil {
		t.Fatalf("chronicle json failed: %v (stderr=%s)", err, stderr.String())
	}
	var rep runtrace.ChronicleReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("decode chronicle json failed: %v", err)
	}
	if rep.WorldID != "cli-chronicle" || len(rep.Timeline) != 2 {
		t.Fatalf("unexpected chronicle json report: %+v", rep)
	}
}

func TestCLIChronicleRejectsRunRecords(t *testing.T) {
	for _, payload := range []string{
		"{\"kind\":\"tick_settled\",\"tick\":8}\n",
		"[{\"id\":1,\"kind\":\"tick_settled\"}]",
		"[{\"id\":1}]",
	} {
		path := filepath.Join(t.TempDir(), "not-events.json")
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := loadEventsFromFile(path); err == nil || !strings.Contains(err.Error(), "expected a simulation event") {
			t.Fatalf("input %s should fail with an event validation error, got %v", payload, err)
		}
	}
}

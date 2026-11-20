package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

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

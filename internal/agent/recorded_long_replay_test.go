//go:build evidence

package agent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRecordedLongCheckpointMatchesTheCurrentSettlementEngine(t *testing.T) {
	dir := os.Getenv("WRE_EVIDENCE_LONG_DIR")
	if dir == "" {
		t.Fatal("WRE_EVIDENCE_LONG_DIR must name a completed long-run scenario")
	}
	result, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var outcome struct {
		LastDayReplayEqual bool `json:"last_day_replay_equal"`
	}
	if err := json.Unmarshal(result, &outcome); err != nil {
		t.Fatal(err)
	}
	if !outcome.LastDayReplayEqual {
		t.Fatal("original long-run replay was not verified")
	}
	checkpoint, err := os.ReadFile(filepath.Join(dir, "checkpoint-latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var start struct{ Tick int64 }
	if err := json.Unmarshal(checkpoint, &start); err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(filepath.Join(dir, "final-verified.json"))
	if err != nil {
		t.Fatal(err)
	}
	var target struct{ Tick int64 }
	if err := json.Unmarshal(final, &target); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, "steps.ndjson.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var logs []sim.TickDecisionLog
	decoder := json.NewDecoder(reader)
	for {
		var row struct{ Decisions *sim.TickDecisionLog }
		if err := decoder.Decode(&row); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if row.Decisions != nil && row.Decisions.Tick >= start.Tick {
			logs = append(logs, *row.Decisions)
		}
	}
	replayed, _, err := sim.ReplayFromCheckpoint(checkpoint, logs, target.Tick)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, final) {
		t.Fatal("current settlement changed the recorded long-run final state")
	}
	t.Logf("current engine reproduced ticks %d through %d exactly", start.Tick, target.Tick)
}

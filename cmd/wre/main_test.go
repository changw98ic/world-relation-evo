package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCLI_OfflinePackRun_Replay_And_Inspect(t *testing.T) {
	dir := t.TempDir()
	packPath := filepath.Join("..", "..", "packs", "mortal_watershed.json")
	cpPath := filepath.Join(dir, "checkpoint.json")
	statePath := filepath.Join(dir, "final_state.json")
	decisionsPath := filepath.Join(dir, "decisions.json")
	eventsPath := filepath.Join(dir, "events.json")
	replayedStatePath := filepath.Join(dir, "replayed_state.json")

	// 1. Run external WorldPack offline for 10 ticks while saving checkpoint, state, decisions, and events.
	var runStdout, runStderr bytes.Buffer
	err := runArgs([]string{
		"-pack", packPath,
		"-offline-run",
		"-decider", "rules",
		"-steps", "10",
		"-pace", "0",
		"-quiet",
		"-out-checkpoint", cpPath,
		"-out-state", statePath,
		"-out-decisions", decisionsPath,
		"-out-events", eventsPath,
	}, &runStdout, &runStderr)
	if err != nil {
		t.Fatalf("offline pack run failed: %v\nstderr=%s", err, runStderr.String())
	}

	// 2. Replay from checkpoint + decision logs via `wre replay` without PostgreSQL.
	var replayStdout, replayStderr bytes.Buffer
	err = runArgs([]string{
		"replay",
		"-checkpoint-file", cpPath,
		"-decisions-file", decisionsPath,
		"-to-tick", "10",
		"-out-state", replayedStatePath,
	}, &replayStdout, &replayStderr)
	if err != nil {
		t.Fatalf("wre replay failed: %v\nstderr=%s", err, replayStderr.String())
	}

	var repOut replayOutput
	if err := json.Unmarshal(replayStdout.Bytes(), &repOut); err != nil {
		t.Fatalf("decode replay stdout JSON: %v", err)
	}
	if repOut.WorldID != "mortal-watershed" || repOut.TargetTick != 10 {
		t.Fatalf("unexpected replay output header: world=%q tick=%d", repOut.WorldID, repOut.TargetTick)
	}

	origStateBytes, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	replayedStateBytes, err := os.ReadFile(replayedStatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(origStateBytes, replayedStateBytes) {
		t.Fatalf("replayed state JSON differs byte-for-byte from live run state JSON")
	}

	// 3. Exercise all 4 `wre inspect` kinds offline.
	t.Run("inspect actor", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if err := runArgs([]string{"inspect", "-state-file", statePath, "-kind", "actor", "-target", "magistrate_shen"}, &out, &errBuf); err != nil {
			t.Fatalf("inspect actor: %v", err)
		}
		var rep sim.ActorInspection
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("decode actor inspection: %v", err)
		}
		if rep.ID != "magistrate_shen" || rep.Location == "" || rep.PolityID == "" {
			t.Fatalf("unexpected actor inspection: %+v", rep)
		}
	})

	t.Run("inspect polity", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if err := runArgs([]string{"inspect", "-state-file", statePath, "-kind", "polity", "-target", "qingzhou_province"}, &out, &errBuf); err != nil {
			t.Fatalf("inspect polity: %v", err)
		}
		var rep sim.PolityInspection
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("decode polity inspection: %v", err)
		}
		if rep.ID != "qingzhou_province" || len(rep.SubordinateNodes) == 0 {
			t.Fatalf("unexpected polity inspection: %+v", rep)
		}
	})

	t.Run("inspect conservation", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		if err := runArgs([]string{"inspect", "-state-file", statePath, "-kind", "conservation"}, &out, &errBuf); err != nil {
			t.Fatalf("inspect conservation: %v", err)
		}
		var rep sim.ConservationInspection
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("decode conservation inspection: %v", err)
		}
		if rep.WorldID != "mortal-watershed" || rep.TotalPopulation <= 0 || rep.TotalCoins <= 0 {
			t.Fatalf("unexpected conservation inspection: %+v", rep)
		}
	})

	t.Run("inspect lineage json and ndjson", func(t *testing.T) {
		evRaw, err := os.ReadFile(eventsPath)
		if err != nil {
			t.Fatal(err)
		}
		var evs []sim.Event
		if err := json.Unmarshal(evRaw, &evs); err != nil {
			t.Fatal(err)
		}
		if len(evs) == 0 {
			t.Fatalf("expected events from offline run")
		}
		targetID := evs[len(evs)-1].ID

		var out, errBuf bytes.Buffer
		if err := runArgs([]string{"inspect", "-events-file", eventsPath, "-kind", "lineage", "-event", jsonNumber(targetID)}, &out, &errBuf); err != nil {
			t.Fatalf("inspect lineage (json): %v", err)
		}
		var rep sim.LineageInspection
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("decode lineage inspection: %v", err)
		}
		if rep.TargetEventID != targetID || len(rep.UpstreamChain) == 0 {
			t.Fatalf("unexpected lineage inspection: %+v", rep)
		}

		// Also verify NDJSON format works for -events-file.
		ndjsonPath := filepath.Join(dir, "events.ndjson")
		var nd bytes.Buffer
		enc := json.NewEncoder(&nd)
		for _, e := range evs {
			if err := enc.Encode(e); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(ndjsonPath, nd.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		out.Reset()
		if err := runArgs([]string{"inspect", "-events-file", ndjsonPath, "-kind", "lineage", "-event", jsonNumber(targetID)}, &out, &errBuf); err != nil {
			t.Fatalf("inspect lineage (ndjson): %v", err)
		}
	})
}

func TestCLI_ErrorCases(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := runArgs([]string{"inspect", "-kind", "unknown"}, &out, &errBuf); err == nil {
		t.Fatalf("expected error for unknown inspect kind")
	}
	if err := runArgs([]string{"replay", "-to-tick", "-1"}, &out, &errBuf); err == nil {
		t.Fatalf("expected error for negative -to-tick")
	}
	if err := runArgs([]string{"-decider", "bogus", "-offline-run"}, &out, &errBuf); err == nil {
		t.Fatalf("expected error for unknown decider")
	}
}

func jsonNumber(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

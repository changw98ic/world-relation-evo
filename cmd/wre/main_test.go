package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/pack"
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

type memReplaySource struct {
	checkpoints map[int64][]byte
	logs        map[int64]sim.TickDecisionLog
	calls       [][2]int64
}

func (m *memReplaySource) LoadNearestCheckpoint(_ context.Context, _ string, maxTick int64) (*sim.World, []byte, error) {
	bestTick := int64(-1)
	for tick := range m.checkpoints {
		if tick <= maxTick && tick > bestTick {
			bestTick = tick
		}
	}
	if bestTick < 0 {
		return nil, nil, sql.ErrNoRows
	}
	raw := m.checkpoints[bestTick]
	var w sim.World
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, nil, err
	}
	return &w, raw, nil
}

func (m *memReplaySource) LoadDecisionLogs(_ context.Context, _ string, fromTick, toTick int64) ([]sim.TickDecisionLog, error) {
	m.calls = append(m.calls, [2]int64{fromTick, toTick})
	var ticks []int64
	for tick := range m.logs {
		if tick >= fromTick && tick <= toTick {
			ticks = append(ticks, tick)
		}
	}
	sort.Slice(ticks, func(i, j int) bool { return ticks[i] < ticks[j] })
	out := make([]sim.TickDecisionLog, 0, len(ticks))
	for _, tick := range ticks {
		out = append(out, m.logs[tick])
	}
	return out, nil
}

func TestCLI_StoreReplayLoadsCheckpointTickDecisionLog(t *testing.T) {
	p, err := pack.LoadFile(filepath.Join("..", "..", "packs", "mortal_watershed.json"))
	if err != nil {
		t.Fatal(err)
	}
	w, err := p.Instantiate("store-replay-regression")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	d := agent.Offline()
	src := &memReplaySource{
		checkpoints: make(map[int64][]byte),
		logs:        make(map[int64]sim.TickDecisionLog),
	}

	cp0Raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	src.checkpoints[0] = cp0Raw

	var snap4Raw, snap8Raw []byte
	for step := 0; step < 8; step++ {
		_, tickLog, err := w.StepRecorded(ctx, d)
		if err != nil {
			t.Fatalf("StepRecorded at tick %d: %v", w.Tick, err)
		}
		src.logs[tickLog.Tick] = tickLog
		if w.Tick == 4 {
			snap4Raw, err = json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
		}
		if w.Tick == 8 {
			snap8Raw, err = json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	if len(src.logs[0].Decisions) == 0 {
		t.Fatalf("expected at least one actor decision at tick 0")
	}
	if len(src.logs[4].Decisions) == 0 {
		t.Fatalf("expected at least one actor decision at tick 4")
	}

	// 1. Replay tick 0 -> 4 before tick 4 checkpoint exists: must load [0, 3].
	cpRaw, logs, err := loadCheckpointAndLogsFromStore(ctx, src, w.ID, 4)
	if err != nil {
		t.Fatalf("loadCheckpointAndLogsFromStore(0->4): %v", err)
	}
	if !reflect.DeepEqual(src.calls, [][2]int64{{0, 3}}) {
		t.Fatalf("expected LoadDecisionLogs range [0, 3], got %v", src.calls)
	}
	replayed4, _, err := sim.ReplayFromCheckpoint(cpRaw, logs, 4)
	if err != nil {
		t.Fatalf("ReplayFromCheckpoint(0->4): %v", err)
	}
	replayed4Raw, err := json.Marshal(replayed4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap4Raw, replayed4Raw) {
		t.Fatalf("replayed tick 4 World JSON differs from live run")
	}

	// 2. Add mid-run checkpoint at tick 4; replaying to exact checkpoint tick 4 must not call LoadDecisionLogs.
	src.checkpoints[4] = snap4Raw
	src.calls = nil
	cpRawExact, logsExact, err := loadCheckpointAndLogsFromStore(ctx, src, w.ID, 4)
	if err != nil {
		t.Fatalf("loadCheckpointAndLogsFromStore(4->4): %v", err)
	}
	if len(src.calls) != 0 {
		t.Fatalf("expected no LoadDecisionLogs call when toTick == cpWorld.Tick, got %v", src.calls)
	}
	if len(logsExact) != 0 {
		t.Fatalf("expected empty logs when toTick == cpWorld.Tick, got %d", len(logsExact))
	}
	replayedExact4, _, err := sim.ReplayFromCheckpoint(cpRawExact, logsExact, 4)
	if err != nil {
		t.Fatalf("ReplayFromCheckpoint(4->4): %v", err)
	}
	replayedExact4Raw, err := json.Marshal(replayedExact4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap4Raw, replayedExact4Raw) {
		t.Fatalf("replayed exact checkpoint tick 4 World JSON differs from live run")
	}

	// 3. Replay tick 4 -> 8 from mid-run checkpoint at tick 4: must load [4, 7].
	src.calls = nil
	cpRaw8, logs8, err := loadCheckpointAndLogsFromStore(ctx, src, w.ID, 8)
	if err != nil {
		t.Fatalf("loadCheckpointAndLogsFromStore(4->8): %v", err)
	}
	if !reflect.DeepEqual(src.calls, [][2]int64{{4, 7}}) {
		t.Fatalf("expected LoadDecisionLogs range [4, 7], got %v", src.calls)
	}
	replayed8, _, err := sim.ReplayFromCheckpoint(cpRaw8, logs8, 8)
	if err != nil {
		t.Fatalf("ReplayFromCheckpoint(4->8): %v", err)
	}
	replayed8Raw, err := json.Marshal(replayed8)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap8Raw, replayed8Raw) {
		t.Fatalf("replayed tick 8 World JSON differs from live run")
	}

	// 4. Error when no checkpoint <= toTick exists.
	srcOnly4 := &memReplaySource{
		checkpoints: map[int64][]byte{4: snap4Raw},
		logs:        src.logs,
	}
	if _, _, err := loadCheckpointAndLogsFromStore(ctx, srcOnly4, w.ID, 2); err == nil {
		t.Fatalf("expected error when no checkpoint exists <= toTick")
	}
	if len(srcOnly4.calls) != 0 {
		t.Fatalf("expected no LoadDecisionLogs call when checkpoint lookup fails, got %v", srcOnly4.calls)
	}
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

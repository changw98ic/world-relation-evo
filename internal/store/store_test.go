package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPostgresAtomicSaveAndResume(t *testing.T) {
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WRE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("integration-%d", time.Now().UnixNano())
	defer func() {
		s.db.ExecContext(ctx, `DELETE FROM world_events WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	}()
	if _, err := s.Load(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing world: %v", err)
	}
	unlock, err := s.Lock(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if release, err := s.Lock(ctx, id); err == nil {
		release()
		t.Fatal("second runner acquired lock")
	}
	unlock()
	unlock, err = s.Lock(ctx, id)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	defer unlock()
	w := &sim.World{ID: id, Tick: 1, NextID: 2}
	events := []sim.Event{{ID: 1, Tick: 1, ActorID: "actor-1", TargetID: "actor-2", Kind: "test-interaction", Payload: map[string]any{"text": "hello"}}}
	if err := s.Save(ctx, w, events); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tick != 1 || got.NextID != 2 {
		t.Fatalf("bad snapshot: %+v", got)
	}
	// An event write failure must roll back the snapshot update too.
	w.Tick = 2
	if err := s.Save(ctx, w, events); err == nil {
		t.Fatal("duplicate event accepted")
	}
	got, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tick != 1 {
		t.Fatalf("partial commit: tick=%d", got.Tick)
	}
	w.NextID = 3
	events[0].ID = 2
	events[0].Tick = 2
	if err := s.Save(ctx, w, events); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM world_events WHERE world_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("events=%d", count)
	}
}

func TestPostgresPreservesTopologyHousesTasksAndLineage(t *testing.T) {
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WRE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("lineage-%d", time.Now().UnixNano())
	defer func() {
		s.db.ExecContext(ctx, `DELETE FROM world_events WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	}()
	w := sim.Village(id)
	for i := 0; i < 3; i++ {
		w.Actors[i].InsideHouse = ""
		w.Actors[i].Location = "south_fields"
	}
	events := w.Apply("p01", sim.Action{Kind: "craft", RecipeID: "farm_grain"})
	for w.Actors[0].Task != nil {
		w.Tick++
		events = append(events, w.Advance("p01")...)
	}
	root := events[len(events)-1].ID
	events = append(events, w.Apply("p01", sim.Action{
		Kind: "speak", TargetID: "p02", Text: "I harvested two grain", BasisEventID: root,
		Statement: &sim.Statement{Topic: "item_produced", Item: "food", Location: "south_fields", Quantity: 2},
	})...)
	first := events[len(events)-1].ID
	events = append(events, w.Apply("p02", sim.Action{
		Kind: "speak", TargetID: "p03", Text: "They harvested twenty grain", BasisEventID: first,
		Statement: &sim.Statement{Topic: "item_produced", Item: "food", Location: "south_fields", Quantity: 20},
	})...)
	// Also put p04 into an active multi-tick sleep task to verify ongoing Task round-trips cleanly!
	events = append(events, w.Apply("p04", sim.Action{Kind: "sleep", Quantity: 12})...)
	w.Tick++
	if err = s.Save(ctx, w, events); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, restored) {
		t.Fatal("snapshot changed topology, house assets, active task, claim or observation semantics")
	}
	var basis, gotRoot int64
	speechEventID := events[len(events)-2].ID
	if err = s.db.QueryRowContext(ctx, `SELECT (action->>'basis_event_id')::bigint,(payload->>'root_event_id')::bigint FROM world_events WHERE world_id=$1 AND id=$2`, id, speechEventID).Scan(&basis, &gotRoot); err != nil {
		t.Fatal(err)
	}
	if basis != first || gotRoot != root {
		t.Fatal("persisted lineage broken")
	}
}

func TestCompactEventsRetainsCausalAndProgressionEvents(t *testing.T) {
	raw := []sim.Event{
		{ID: 1, Kind: "moved"},
		{ID: 2, Kind: "ate"},
		{ID: 3, Kind: "speech", Action: sim.Action{Kind: "speak", Text: "filler chat"}},
		{ID: 4, Kind: "speech", Action: sim.Action{Kind: "speak", Statement: &sim.Statement{Topic: "item_produced", Item: "food"}}},
		{ID: 5, Kind: "crafted"},
		{ID: 6, Kind: "trade_completed"},
		{ID: 7, Kind: "cohort_promoted"},
	}
	compacted := CompactEvents(raw)
	if len(compacted) != 4 {
		t.Fatalf("expected 4 high-value events retained, got %d (%+v)", len(compacted), compacted)
	}
}

func TestSchemaIncludesCheckpointAndDecisionLogTables(t *testing.T) {
	for _, needle := range []string{
		"CREATE TABLE IF NOT EXISTS world_checkpoints",
		"CREATE TABLE IF NOT EXISTS world_decision_logs",
		"PRIMARY KEY (world_id, tick)",
		"state jsonb NOT NULL",
		"decisions jsonb NOT NULL",
	} {
		if !strings.Contains(schema, needle) {
			t.Fatalf("embedded schema missing %q", needle)
		}
	}
}

func TestPostgresCheckpointsDecisionLogsAndLoadEvents(t *testing.T) {
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WRE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("cp-integration-%d", time.Now().UnixNano())
	defer func() {
		s.db.ExecContext(ctx, `DELETE FROM world_decision_logs WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM world_checkpoints WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM world_events WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	}()

	w0 := &sim.World{ID: id, Tick: 0, NextID: 1}
	w5 := &sim.World{ID: id, Tick: 5, NextID: 12}
	if err := s.SaveCheckpoint(ctx, w0); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(ctx, w5); err != nil {
		t.Fatal(err)
	}
	cp, raw, err := s.LoadNearestCheckpoint(ctx, id, 4)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Tick != 0 || len(raw) == 0 {
		t.Fatalf("expected tick 0 checkpoint, got tick=%d len=%d", cp.Tick, len(raw))
	}
	cp, _, err = s.LoadNearestCheckpoint(ctx, id, 7)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Tick != 5 {
		t.Fatalf("expected tick 5 checkpoint, got %d", cp.Tick)
	}

	log1 := sim.TickDecisionLog{Tick: 1, Decisions: map[string]sim.Action{"a1": {Kind: "wait", Text: "pause"}}}
	log2 := sim.TickDecisionLog{Tick: 2, Decisions: map[string]sim.Action{"a1": {Kind: "speak", Text: "hi"}}}
	if err := s.SaveDecisionLog(ctx, id, log1); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDecisionLog(ctx, id, log2); err != nil {
		t.Fatal(err)
	}
	logs, err := s.LoadDecisionLogs(ctx, id, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Tick != 1 || logs[1].Decisions["a1"].Text != "hi" {
		t.Fatalf("unexpected logs: %+v", logs)
	}

	evs := []sim.Event{
		{ID: 1, Tick: 1, ActorID: "a1", Kind: "wait", Action: sim.Action{Kind: "wait"}, Payload: map[string]any{"x": 1}},
		{ID: 2, Tick: 2, ActorID: "a1", Kind: "speech", Action: sim.Action{Kind: "speak", Text: "hi"}, Payload: map[string]any{"text": "hi"}},
	}
	if err := s.Save(ctx, w5, evs); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadEvents(ctx, id, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[1].Action.Text != "hi" {
		t.Fatalf("unexpected loaded events: %+v", loaded)
	}
}

func TestPostgresCheckpointReplayFirstStepDecision(t *testing.T) {
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WRE_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(ctx); err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("cp-replay-%d", time.Now().UnixNano())
	defer func() {
		s.db.ExecContext(ctx, `DELETE FROM world_decision_logs WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM world_checkpoints WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM world_events WHERE world_id=$1`, id)
		s.db.ExecContext(ctx, `DELETE FROM worlds WHERE id=$1`, id)
	}()

	w := sim.Village(id)
	d := agent.Offline()
	if err := s.SaveCheckpoint(ctx, w); err != nil {
		t.Fatal(err)
	}

	verifyReplayRange := func(expectedCPTick, targetTick int64) {
		t.Helper()
		cp, cpRaw, err := s.LoadNearestCheckpoint(ctx, id, targetTick)
		if err != nil {
			t.Fatal(err)
		}
		if cp.Tick != expectedCPTick {
			t.Fatalf("expected checkpoint tick %d, got %d", expectedCPTick, cp.Tick)
		}
		logs, err := s.LoadDecisionLogs(ctx, id, cp.Tick, targetTick-1)
		if err != nil {
			t.Fatal(err)
		}
		replayed, _, err := sim.ReplayFromCheckpoint(cpRaw, logs, targetTick)
		if err != nil {
			t.Fatalf("ReplayFromCheckpoint(%d->%d): %v", cp.Tick, targetTick, err)
		}
		raw, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		var expected sim.World
		if err := json.Unmarshal(raw, &expected); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(&expected, replayed) {
			t.Fatalf("tick %d replay diverged from live run", targetTick)
		}
	}

	for step := 0; step < 4; step++ {
		_, tickLog, err := w.StepRecorded(ctx, d)
		if err != nil {
			t.Fatalf("StepRecorded tick %d: %v", w.Tick, err)
		}
		if step == 0 && len(tickLog.Decisions) == 0 {
			t.Fatalf("expected actor decisions at tick 0")
		}
		if err := s.SaveDecisionLog(ctx, id, tickLog); err != nil {
			t.Fatal(err)
		}
	}
	verifyReplayRange(0, 4)

	// Save mid-run checkpoint at tick 4 and step to tick 8.
	if err := s.SaveCheckpoint(ctx, w); err != nil {
		t.Fatal(err)
	}
	for step := 4; step < 8; step++ {
		_, tickLog, err := w.StepRecorded(ctx, d)
		if err != nil {
			t.Fatalf("StepRecorded tick %d: %v", w.Tick, err)
		}
		if step == 4 && len(tickLog.Decisions) == 0 {
			t.Fatalf("expected actor decisions at tick 4")
		}
		if err := s.SaveDecisionLog(ctx, id, tickLog); err != nil {
			t.Fatal(err)
		}
	}
	verifyReplayRange(4, 8)
}

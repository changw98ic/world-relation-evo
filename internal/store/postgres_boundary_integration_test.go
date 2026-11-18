//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// This entry point is deliberately separate from the default suite. Missing
// external configuration is a failure, never evidence of an executed DB test.
func boundaryPostgres(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv("WRE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("integration requires explicit WRE_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal("PostgreSQL connection failed; connection details are withheld")
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error("close PostgreSQL failed")
		}
	})
	if err := s.Init(ctx); err != nil {
		t.Fatalf("initialize test schema: %v", err)
	}
	return s, ctx
}
func boundaryWorldID(t *testing.T, s *Store, ctx context.Context) string {
	t.Helper()
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal(err)
	}
	id := "test-boundary-" + hex.EncodeToString(entropy[:])
	t.Cleanup(func() {
		// Every deletion is scoped to the newly generated identity. No global
		// truncation or existing-world selection is permitted in this helper.
		for _, query := range []string{
			`DELETE FROM world_decision_logs WHERE world_id=$1`,
			`DELETE FROM world_checkpoints WHERE world_id=$1`,
			`DELETE FROM world_events WHERE world_id=$1`,
			`DELETE FROM worlds WHERE id=$1`,
		} {
			if _, err := s.db.ExecContext(ctx, query, id); err != nil {
				t.Errorf("clean generated test world: %v", err)
			}
		}
	})
	return id
}
func persistedJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPostgresSnapshotAndAllEventBatchesCommitTogether(t *testing.T) {
	s, ctx := boundaryPostgres(t)
	id := boundaryWorldID(t, s, ctx)
	if _, err := s.Load(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing world must remain distinguishable from a query failure: %v", err)
	}
	w := &sim.World{ID: id, Tick: 10, NextID: 2, Actors: []sim.Actor{{ID: "owner", Money: 9, Items: map[string]int{"parcel": 3}, Plan: &sim.PersonalPlan{Item: "parcel", Quantity: 3, Stage: "sell", Status: "active"}, Memories: []sim.Memory{{EventID: 1, Source: "witness", Statement: &sim.Statement{Quantity: 30}, Observation: &sim.Observation{Quantity: 3}}}}}}
	w.Actors = append(w.Actors, sim.Actor{ID: "buyer", Money: 7, Items: map[string]int{}})
	w.Continuity = &sim.ContinuityState{NextAgreementID: 8, Agreements: []sim.Agreement{{ID: 7, SupplierID: "owner", BuyerID: "buyer", Item: "parcel", Quantity: 3, Price: 7, Status: "active", DueTick: 100}}}
	w.MarketCredit = sim.NewMarketCreditState()
	w.MarketCredit.LoanRequests = []sim.LoanRequest{{ID: 9, BorrowerID: "owner", LenderID: "buyer", Principal: 5, Status: "pending"}}
	first := sim.Event{ID: 1, Tick: 10, ActorID: "owner", Kind: "speech", Action: sim.Action{Kind: "speak", Statement: &sim.Statement{Quantity: 30}}, Payload: map[string]any{"quantity": 3}}
	if err := s.Save(ctx, w, []sim.Event{first}); err != nil {
		t.Fatal(err)
	}
	before := persistedJSON(t, w)
	// 201 records cross the current 200-row SQL batch. A duplicate in the
	// second batch must also roll back the first batch and the snapshot.
	batch := make([]sim.Event, 201)
	for i := range batch {
		batch[i] = sim.Event{ID: int64(i + 2), Tick: 11, ActorID: "owner", Kind: "delivery", Payload: map[string]any{"quantity": i}}
	}
	batch[len(batch)-1].ID = 1
	w.Tick, w.NextID, w.Actors[0].Money = 11, 203, 120
	if err := s.Save(ctx, w, batch); err == nil {
		t.Fatal("duplicate settled event unexpectedly committed")
	}
	loaded, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if persistedJSON(t, loaded) != before {
		t.Fatal("failed event insert partially committed a new snapshot or personal state")
	}
	events, err := s.LoadEvents(ctx, id, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != 1 || persistedJSON(t, events[0]) != persistedJSON(t, first) {
		t.Fatal("failed later batch left partial event history")
	}
	batch[len(batch)-1].ID = 202
	if err := s.Save(ctx, w, batch); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	events, err = s.LoadEvents(ctx, id, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if persistedJSON(t, loaded) != persistedJSON(t, w) || len(events) != 202 || events[len(events)-1].ID != 202 {
		t.Fatal("valid retry did not commit the entire snapshot and history")
	}
}

func TestPostgresWorldLockExcludesAnotherSessionAndReleases(t *testing.T) {
	s, ctx := boundaryPostgres(t)
	id := boundaryWorldID(t, s, ctx)
	otherID := boundaryWorldID(t, s, ctx)
	unlock, err := s.Lock(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()
	if second, err := s.Lock(ctx, id); err == nil {
		second()
		t.Fatal("two dedicated sessions acquired the same world's writer lock")
	}
	otherUnlock, err := s.Lock(ctx, otherID)
	if err != nil {
		t.Fatal("one world's lock blocked an independent world")
	}
	otherUnlock()
	// Force the pool to use another connection. Returning a session to the pool
	// between lock and unlock would make this contract unreliable.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	conn.Close()
	unlock()
	released = true
	reacquired, err := s.Lock(ctx, id)
	if err != nil {
		t.Fatalf("released world could not be acquired again: %v", err)
	}
	reacquired()
}

type persistedDecisionFixture struct{ calls atomic.Int64 }

func (f *persistedDecisionFixture) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	f.calls.Add(1)
	target := "buyer"
	if p.Self.ID == "buyer" {
		target = "seller"
	}
	return sim.Action{Kind: "speak", TargetID: target, Text: "three parcels are waiting", Statement: &sim.Statement{Topic: "item_produced", Item: "parcel", Quantity: 3}}, nil
}

func TestPostgresCheckpointBoundaryRestoresDecisionsWithoutCallingDecider(t *testing.T) {
	s, ctx := boundaryPostgres(t)
	id := boundaryWorldID(t, s, ctx)
	otherID := boundaryWorldID(t, s, ctx)
	w := &sim.World{ID: id, Tick: 37, NextID: 1, Catalog: sim.DefaultCatalog(), Map: sim.WorldMap{Districts: []sim.District{{ID: "meeting"}}}, Actors: []sim.Actor{
		{ID: "seller", Location: "meeting", Tags: []string{"resident"}, Health: 100, Energy: 100, Items: map[string]int{"parcel": 3}, Trust: map[string]int{}},
		{ID: "buyer", Location: "meeting", Tags: []string{"resident"}, Health: 100, Energy: 100, Money: 7, Items: map[string]int{}, Trust: map[string]int{}},
	}}
	if err := s.SaveCheckpoint(ctx, w); err != nil {
		t.Fatal(err)
	}
	decider := &persistedDecisionFixture{}
	var originalEvents []sim.Event
	for i := 0; i < 2; i++ {
		events, log, err := w.StepRecorded(ctx, decider)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.SaveDecisionLog(ctx, id, log); err != nil {
			t.Fatal(err)
		}
		originalEvents = append(originalEvents, events...)
	}
	if err := s.Save(ctx, w, originalEvents); err != nil {
		t.Fatal(err)
	}
	future := *w
	future.Tick = w.Tick + 1
	if err := s.SaveCheckpoint(ctx, &future); err != nil {
		t.Fatal(err)
	}
	cp, raw, err := s.LoadNearestCheckpoint(ctx, id, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Tick != 37 {
		t.Fatal("nearest checkpoint selection used a future state")
	}
	logs, err := s.LoadDecisionLogs(ctx, id, cp.Tick, w.Tick-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].Tick != cp.Tick {
		t.Fatal("checkpoint's first decision tick was omitted")
	}
	if err := s.SaveDecisionLog(ctx, otherID, sim.TickDecisionLog{Tick: 37, Decisions: map[string]sim.Action{"stranger": {Kind: "wait"}}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LoadNearestCheckpoint(ctx, otherID, w.Tick); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("independent world read another world's checkpoint")
	}
	otherEvents, err := s.LoadEvents(ctx, otherID, 0, -1)
	if err != nil || len(otherEvents) != 0 {
		t.Fatal("independent world read another world's events")
	}
	calls := decider.calls.Load()
	replayed, events, err := sim.ReplayFromCheckpoint(raw, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 || decider.calls.Load() != calls {
		t.Fatal("replay called the recording decider")
	}
	if persistedJSON(t, replayed) != persistedJSON(t, w) || persistedJSON(t, events) != persistedJSON(t, originalEvents) {
		t.Fatal("database-backed replay changed state or causal event history")
	}
	if _, _, err := sim.ReplayFromCheckpoint(raw, logs[1:], w.Tick); err == nil {
		t.Fatal("missing checkpoint decision silently succeeded")
	}
	storedEvents, err := s.LoadEvents(ctx, id, 37, 37)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedEvents) != 2 || storedEvents[0].Tick != 37 || storedEvents[1].Tick != 37 || storedEvents[0].ID >= storedEvents[1].ID {
		t.Fatal("event range or order differs from committed history")
	}
	otherLogs, err := s.LoadDecisionLogs(ctx, otherID, 37, 37)
	if err != nil || len(otherLogs) != 1 || !reflect.DeepEqual(otherLogs[0].Decisions, map[string]sim.Action{"stranger": {Kind: "wait"}}) {
		t.Fatal("decision log query crossed world identities")
	}
}

func TestPostgresQueryFailureCannotMasqueradeAsEmptyHistory(t *testing.T) {
	s, ctx := boundaryPostgres(t)
	// No test rows are created, so this closed-connection check needs no cleanup.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if events, err := s.LoadEvents(ctx, "test-boundary-closed", 0, -1); err == nil || len(events) != 0 {
		t.Fatal("failed event query looked like a successful empty history")
	}
	if logs, err := s.LoadDecisionLogs(ctx, "test-boundary-closed", 0, 1); err == nil || len(logs) != 0 {
		t.Fatal("failed decision query looked like a successful empty history")
	}
	if _, err := s.Load(ctx, "test-boundary-closed"); err == nil || errors.Is(err, sql.ErrNoRows) {
		t.Fatal("connection failure looked like a missing world")
	}
}

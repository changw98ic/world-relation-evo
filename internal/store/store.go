// Package store persists committed world state and its causal event history.
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/sim"
	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schema string

type Store struct{ db *sql.DB }

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Lock reserves a world for one runner. The lock belongs to this dedicated
// PostgreSQL session, not a pooled connection selected by a later query.
func (s *Store) Lock(ctx context.Context, id string) (func(), error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var locked bool
	err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, "wre:"+id).Scan(&locked)
	if err != nil || !locked {
		if err != nil {
			conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("world %q already has an active runner", id)
	}
	return func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := conn.ExecContext(releaseCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, "wre:"+id)
		if err != nil {
			// Do not return a session with a potentially held lock to the pool.
			conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}

func (s *Store) Load(ctx context.Context, id string) (*sim.World, error) {
	var data []byte
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM worlds WHERE id=$1`, id).Scan(&data); err != nil {
		return nil, err
	}
	w, err := sim.DecodeWorldSnapshot(data)
	if err != nil {
		return nil, fmt.Errorf("decode world %q: %w", id, err)
	}
	return w, nil
}

// Save commits a settled batch and its resulting snapshot together. Callers
// hold Lock for their world and never wait for model responses in this transaction.
// Events are persisted in multi-row batches to minimize database round-trips on large worlds.
func (s *Store) Save(ctx context.Context, w *sim.World, events []sim.Event) error {
	return s.SaveTick(ctx, w, events, nil, false)
}

// SaveTick atomically persists the world state, settled events, optional decision log,
// and optional replay checkpoint inside a single SQL transaction.
func (s *Store) SaveTick(ctx context.Context, w *sim.World, events []sim.Event, decisionLog *sim.TickDecisionLog, saveCheckpoint bool) error {
	if w == nil {
		return fmt.Errorf("world is nil")
	}
	state, err := json.Marshal(w)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO worlds (id,tick,state) VALUES ($1,$2,$3)
        ON CONFLICT (id) DO UPDATE SET tick=EXCLUDED.tick,state=EXCLUDED.state,updated_at=now()`, w.ID, w.Tick, string(state)); err != nil {
		return err
	}
	const batchSize = 200
	for start := 0; start < len(events); start += batchSize {
		end := start + batchSize
		if end > len(events) {
			end = len(events)
		}
		chunk := events[start:end]
		var b strings.Builder
		b.WriteString(`INSERT INTO world_events (world_id,id,tick,actor_id,target_id,kind,action,payload) VALUES `)
		args := make([]any, 0, len(chunk)*8)
		for i, e := range chunk {
			if i > 0 {
				b.WriteByte(',')
			}
			action, err := json.Marshal(e.Action)
			if err != nil {
				return err
			}
			payload, err := json.Marshal(e.Payload)
			if err != nil {
				return err
			}
			base := i * 8
			fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8)
			args = append(args, w.ID, e.ID, e.Tick, e.ActorID, e.TargetID, e.Kind, string(action), string(payload))
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return fmt.Errorf("persist world %q event batch starting at %d: %w", w.ID, chunk[0].ID, err)
		}
	}
	if decisionLog != nil {
		raw, err := json.Marshal(decisionLog)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO world_decision_logs (world_id, tick, decisions, created_at)
			VALUES ($1, $2, $3::jsonb, now())
			ON CONFLICT (world_id, tick) DO UPDATE
			SET decisions = EXCLUDED.decisions, created_at = now()`, w.ID, decisionLog.Tick, string(raw)); err != nil {
			return err
		}
	}
	if saveCheckpoint {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO world_checkpoints (world_id, tick, state, created_at)
			VALUES ($1, $2, $3::jsonb, now())
			ON CONFLICT (world_id, tick) DO UPDATE
			SET state = EXCLUDED.state, created_at = now()`, w.ID, w.Tick, string(state)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CompactEvents filters a settled event slice down to high-value causal, economic,
// and progression events (dropping routine movement, sleep, meal, and filler chat events)
// to reduce storage volume by ~85-90% on million-scale runs.
func CompactEvents(events []sim.Event) []sim.Event {
	if len(events) == 0 {
		return nil
	}
	out := make([]sim.Event, 0, len(events)/4+1)
	for _, e := range events {
		switch e.Kind {
		case "ate", "drank", "relieved", "sleep_started", "slept", "task_started", "entered_house", "left_house", "moved", "trade_offered", "trade_rejected", "cohort_socialized":
			continue
		case "speech":
			if e.Action.Statement == nil && e.Action.BasisEventID == 0 {
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// LoadNearestCheckpoint returns the latest checkpoint for worldID with tick <= maxTick,
// along with its raw JSON bytes so replay starts from the exact serialized snapshot.
func (s *Store) LoadNearestCheckpoint(ctx context.Context, worldID string, maxTick int64) (*sim.World, []byte, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT state FROM world_checkpoints
		WHERE world_id = $1 AND tick <= $2
		ORDER BY tick DESC
		LIMIT 1`, worldID, maxTick).Scan(&raw)
	if err != nil {
		return nil, nil, err
	}
	w, err := sim.DecodeWorldSnapshot(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("unmarshal checkpoint world=%q: %w", worldID, err)
	}
	return w, raw, nil
}

// LoadDecisionLogs loads ordered per-tick decision logs for worldID across [fromTick, toTick].
func (s *Store) LoadDecisionLogs(ctx context.Context, worldID string, fromTick, toTick int64) ([]sim.TickDecisionLog, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT decisions FROM world_decision_logs
		WHERE world_id = $1 AND tick >= $2 AND tick <= $3
		ORDER BY tick ASC`, worldID, fromTick, toTick)
	if err != nil {
		return nil, fmt.Errorf("query decision logs world=%q: %w", worldID, err)
	}
	defer rows.Close()
	var logs []sim.TickDecisionLog
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var entry sim.TickDecisionLog
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal decision log world=%q: %w", worldID, err)
		}
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

// LoadEvents loads persisted events for worldID ordered by (tick, id) across [fromTick, toTick].
// If toTick < 0, all events with tick >= fromTick are returned.
func (s *Store) LoadEvents(ctx context.Context, worldID string, fromTick, toTick int64) ([]sim.Event, error) {
	var rows *sql.Rows
	var err error
	if toTick < 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, tick, actor_id, target_id, kind, action, payload
			FROM world_events
			WHERE world_id = $1 AND tick >= $2
			ORDER BY tick ASC, id ASC`, worldID, fromTick)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, tick, actor_id, target_id, kind, action, payload
			FROM world_events
			WHERE world_id = $1 AND tick >= $2 AND tick <= $3
			ORDER BY tick ASC, id ASC`, worldID, fromTick, toTick)
	}
	if err != nil {
		return nil, fmt.Errorf("query events world=%q: %w", worldID, err)
	}
	defer rows.Close()
	var events []sim.Event
	for rows.Next() {
		var (
			ev         sim.Event
			actionRaw  []byte
			payloadRaw []byte
		)
		if err := rows.Scan(&ev.ID, &ev.Tick, &ev.ActorID, &ev.TargetID, &ev.Kind, &actionRaw, &payloadRaw); err != nil {
			return nil, err
		}
		if len(actionRaw) > 0 {
			if err := json.Unmarshal(actionRaw, &ev.Action); err != nil {
				return nil, fmt.Errorf("unmarshal event %d action: %w", ev.ID, err)
			}
		}
		if len(payloadRaw) > 0 {
			if err := json.Unmarshal(payloadRaw, &ev.Payload); err != nil {
				return nil, fmt.Errorf("unmarshal event %d payload: %w", ev.ID, err)
			}
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

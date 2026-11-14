// Package store persists committed world state and its causal event history.
package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"fmt"
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
	var w sim.World
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("decode world %q: %w", id, err)
	}
	return &w, nil
}

// Save commits a settled batch and its resulting snapshot together. Callers
// hold Lock for their world and never wait for model responses in this transaction.
func (s *Store) Save(ctx context.Context, w *sim.World, events []sim.Event) error {
	state, err := json.Marshal(w)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO worlds (id,tick,state) VALUES ($1,$2,$3)
        ON CONFLICT (id) DO UPDATE SET tick=EXCLUDED.tick,state=EXCLUDED.state,updated_at=now()`, w.ID, w.Tick, string(state))
	if err != nil {
		return err
	}
	for _, e := range events {
		action, err := json.Marshal(e.Action)
		if err != nil {
			return err
		}
		payload, err := json.Marshal(e.Payload)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO world_events (world_id,id,tick,actor_id,target_id,kind,action,payload)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, w.ID, e.ID, e.Tick, e.ActorID, e.TargetID, e.Kind, string(action), string(payload))
		if err != nil {
			return fmt.Errorf("persist world %q event %d: %w", w.ID, e.ID, err)
		}
	}
	return tx.Commit()
}

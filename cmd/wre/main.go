package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"github.com/changw98ic/world-relation-evo/internal/store"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	id := flag.String("world", "village-30", "world ID")
	steps := flag.Int("steps", 24, "hours to run; 0 runs continuously")
	pace := flag.Duration("pace", time.Second, "real delay per simulation hour")
	mode := flag.String("decider", "rules", "rules or model")
	flag.Parse()
	if *steps < 0 || *pace < 0 {
		return fmt.Errorf("steps and pace must be nonnegative")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var d sim.Decider
	switch *mode {
	case "rules":
		d = agent.Rules{}
	case "model":
		endpoint, name := os.Getenv("WRE_MODEL_URL"), os.Getenv("WRE_MODEL")
		if endpoint == "" || name == "" {
			return fmt.Errorf("model mode requires WRE_MODEL_URL and WRE_MODEL")
		}
		d = agent.Model{Endpoint: endpoint, Name: name, APIKey: os.Getenv("WRE_MODEL_KEY")}
	default:
		return fmt.Errorf("unknown decider %q", *mode)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required (PostgreSQL)")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.Init(ctx); err != nil {
		return err
	}
	unlock, err := db.Lock(ctx, *id)
	if err != nil {
		return err
	}
	defer unlock()
	w, err := db.Load(ctx, *id)
	if errors.Is(err, sql.ErrNoRows) {
		w = sim.Village(*id)
		err = db.Save(ctx, w, nil)
	}
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	for n := 0; *steps == 0 || n < *steps; n++ {
		events, err := w.Step(ctx, d)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err = db.Save(ctx, w, events); err != nil {
			return err
		}
		for _, e := range events {
			if err = enc.Encode(e); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "world=%s tick=%d actors=%d events=%d decider=%s\n", w.ID, w.Tick, len(w.Actors), len(events), *mode)
		if *steps != 0 && n+1 == *steps {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*pace):
		}
	}
	return nil
}

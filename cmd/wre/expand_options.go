package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/optiongen"
	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func runExpandOptions(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expand-options", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packPath := fs.String("pack", "", "path to a WorldPack JSON file (omit to use built-in village-30)")
	worldID := fs.String("world", "village-30", "world ID when not using -pack")
	days := fs.Int("days", 30, "planning horizon in simulated days")
	outDir := fs.String("out", "", "output directory for per-actor option JSON files and summary (defaults to .artifacts/options-<world>-<timestamp>)")
	concurrency := fs.Int("concurrency", 3, "maximum concurrent DS requests (still strictly 1 request per character)")
	actorFilter := fs.String("actor", "", "optional single actor ID to expand")
	resume := fs.Bool("resume", false, "reuse valid existing actors/<actor_id>.json files in -out without re-calling DS")
	maxTokens := fs.Int("max-tokens", 32768, "max_tokens per DS request")
	reasoningEffort := fs.String("reasoning-effort", "low", "optional reasoning_effort hint for reasoning models (e.g. low, medium, high, or empty)")
	modelURL := fs.String("model-url", "", "override WRE_MODEL_URL")
	modelName := fs.String("model", "", "override WRE_MODEL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *days <= 0 {
		return fmt.Errorf("-days must be positive")
	}

	endpoint := strings.TrimSpace(*modelURL)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("WRE_MODEL_URL"))
	}
	model := strings.TrimSpace(*modelName)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("WRE_MODEL"))
	}
	apiKey := strings.TrimSpace(os.Getenv("WRE_MODEL_KEY"))
	if endpoint == "" || model == "" {
		return fmt.Errorf("expand-options requires WRE_MODEL_URL and WRE_MODEL (in environment or .env)")
	}

	var w *sim.World
	if strings.TrimSpace(*packPath) != "" {
		loaded, err := pack.LoadFile(*packPath)
		if err != nil {
			return fmt.Errorf("load pack %q: %w", *packPath, err)
		}
		w, err = loaded.Instantiate("")
		if err != nil {
			return fmt.Errorf("instantiate pack %q: %w", *packPath, err)
		}
	} else {
		w = sim.Village(*worldID)
	}

	targetOut := strings.TrimSpace(*outDir)
	if targetOut == "" {
		targetOut = filepath.Join(".artifacts", fmt.Sprintf("options-%s-%s", w.ID, time.Now().Format("20060102-150405")))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	summary, err := optiongen.ExpandWorld(ctx, w, optiongen.Client{
		Endpoint:        endpoint,
		Model:           model,
		APIKey:          apiKey,
		MaxTokens:       *maxTokens,
		ReasoningEffort: strings.TrimSpace(*reasoningEffort),
	}, optiongen.ExpandConfig{
		PackPath:    *packPath,
		OutDir:      targetOut,
		HorizonDays: *days,
		Concurrency: *concurrency,
		ActorFilter: strings.TrimSpace(*actorFilter),
		Resume:      *resume,
		Progress:    stderr,
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

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

func runExpandRulebook(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expand-rulebook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packPath := fs.String("pack", "", "path to a WorldPack JSON file (omit to use built-in village-30)")
	worldID := fs.String("world", "village-30", "world ID when not using -pack")
	outDir := fs.String("out", "", "output directory for generic rulebook.json and domain audit files (defaults to .artifacts/rulebook-<world>-<timestamp>)")
	concurrency := fs.Int("concurrency", 6, "maximum concurrent DS domain and trait calibration requests")
	resume := fs.Bool("resume", false, "reuse saved domain and calibration responses only when the model request matches")
	maxTokens := fs.Int("max-tokens", 32768, "max_tokens per DS domain request")
	reasoningEffort := fs.String("reasoning-effort", "low", "optional reasoning_effort hint for reasoning models")
	modelURL := fs.String("model-url", "", "override WRE_MODEL_URL")
	modelName := fs.String("model", "", "override WRE_MODEL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
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
		return fmt.Errorf("expand-rulebook requires WRE_MODEL_URL and WRE_MODEL (in environment or .env)")
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
		targetOut = filepath.Join(".artifacts", fmt.Sprintf("rulebook-%s-%s", w.ID, time.Now().Format("20060102-150405")))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	summary, _, err := optiongen.ExpandWorldRulebook(ctx, w, optiongen.Client{
		Endpoint:        endpoint,
		Model:           model,
		APIKey:          apiKey,
		MaxTokens:       *maxTokens,
		ReasoningEffort: strings.TrimSpace(*reasoningEffort),
	}, optiongen.RulebookExpandConfig{
		OutDir:      targetOut,
		Concurrency: *concurrency,
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

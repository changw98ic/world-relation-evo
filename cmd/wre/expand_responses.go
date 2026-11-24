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

	"github.com/changw98ic/world-relation-evo/internal/optiongen"
)

func runExpandResponses(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("expand-responses", flag.ContinueOnError)
	fs.SetOutput(stderr)
	optionsDir := fs.String("options-dir", "", "directory containing expand-options output (summary.json + actors/*.json)")
	outDir := fs.String("out", "", "output directory for combination scenarios and counter-options (defaults to <options-dir>/responses)")
	topN := fs.Int("top", 12, "number of top high-likelihood multi-actor combination scenarios to expand")
	concurrency := fs.Int("concurrency", 4, "maximum concurrent DS requests")
	resume := fs.Bool("resume", false, "reuse saved scenario responses only when the model request matches")
	maxTokens := fs.Int("max-tokens", 16384, "max_tokens per DS request")
	reasoningEffort := fs.String("reasoning-effort", "low", "optional reasoning_effort hint for reasoning models (e.g. low, medium, high, or empty)")
	modelURL := fs.String("model-url", "", "override WRE_MODEL_URL")
	modelName := fs.String("model", "", "override WRE_MODEL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if strings.TrimSpace(*optionsDir) == "" {
		return fmt.Errorf("-options-dir is required")
	}
	if *topN <= 0 {
		return fmt.Errorf("-top must be positive")
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
		return fmt.Errorf("expand-responses requires WRE_MODEL_URL and WRE_MODEL (in environment or .env)")
	}

	records, _, err := optiongen.LoadExpansionRecords(*optionsDir)
	if err != nil {
		return err
	}

	targetOut := strings.TrimSpace(*outDir)
	if targetOut == "" {
		targetOut = filepath.Join(*optionsDir, "responses")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	summary, err := optiongen.ExpandResponses(ctx, records, optiongen.Client{
		Endpoint:        endpoint,
		Model:           model,
		APIKey:          apiKey,
		MaxTokens:       *maxTokens,
		ReasoningEffort: strings.TrimSpace(*reasoningEffort),
	}, optiongen.ResponseExpandConfig{
		OptionsDir:  *optionsDir,
		OutDir:      targetOut,
		TopN:        *topN,
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

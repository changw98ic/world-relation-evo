package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func loadEventsFromFile(path string) ([]sim.Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var evs []sim.Event
		if err := json.Unmarshal(trimmed, &evs); err != nil {
			return nil, fmt.Errorf("decode events JSON array: %w", err)
		}
		for i, event := range evs {
			if err := validateChronicleEvent(event); err != nil {
				return nil, fmt.Errorf("event array entry %d: %w", i+1, err)
			}
		}
		return evs, nil
	}
	// NDJSON fallback
	var evs []sim.Event
	for idx, line := range strings.Split(string(trimmed), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev sim.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("decode NDJSON event line %d: %w", idx+1, err)
		}
		if err := validateChronicleEvent(ev); err != nil {
			return nil, fmt.Errorf("event line %d: %w", idx+1, err)
		}
		evs = append(evs, ev)
	}
	return evs, nil
}

func validateChronicleEvent(event sim.Event) error {
	if event.ID <= 0 || strings.TrimSpace(event.Kind) == "" || event.Kind == "tick_settled" {
		return fmt.Errorf("expected a simulation event with a positive id and valid kind, got id=%d kind=%q", event.ID, event.Kind)
	}
	return nil
}

func loadEventsFromFiles(spec string) ([]sim.Event, error) {
	var all []sim.Event
	for _, part := range strings.Split(spec, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		evs, err := loadEventsFromFile(p)
		if err != nil {
			return nil, fmt.Errorf("load events %q: %w", p, err)
		}
		all = append(all, evs...)
	}
	return all, nil
}

func runChronicle(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("chronicle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	eventsFile := fs.String("events-file", "", "comma-separated path(s) to JSON array or NDJSON file(s) of simulation events (required)")
	stateFile := fs.String("state-file", "", "optional path to serialized World JSON snapshot")
	format := fs.String("format", "md", "output format: md|json")
	outPath := fs.String("out", "", "optional output file path (defaults to stdout)")
	maxEvents := fs.Int("max-events", 0, "maximum number of major timeline events to include (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *eventsFile == "" {
		return fmt.Errorf("chronicle requires -events-file")
	}
	events, err := loadEventsFromFiles(*eventsFile)
	if err != nil {
		return err
	}
	var finalWorld *sim.World
	if *stateFile != "" {
		rawState, err := os.ReadFile(*stateFile)
		if err != nil {
			return fmt.Errorf("read state file: %w", err)
		}
		finalWorld, err = sim.DecodeWorldSnapshot(rawState)
		if err != nil {
			return fmt.Errorf("decode state file: %w", err)
		}
	}

	report := runtrace.BuildChronicle(events, finalWorld, *maxEvents)
	var rendered []byte
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		rendered, err = json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		rendered = append(rendered, '\n')
	case "md", "markdown":
		rendered = []byte(runtrace.RenderChronicleMarkdown(report))
	default:
		return fmt.Errorf("unsupported chronicle format %q (expected md or json)", *format)
	}

	if *outPath != "" {
		if err := os.WriteFile(*outPath, rendered, 0o644); err != nil {
			return fmt.Errorf("write chronicle output %q: %w", *outPath, err)
		}
		return nil
	}
	_, err = stdout.Write(rendered)
	return err
}

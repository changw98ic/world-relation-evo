package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func init() {
	additionalCommands["blueprint"] = runBlueprint
	additionalCommands["pack"] = runPack
	additionalCommands["inject"] = runInject
	additionalCommands["query"] = runQuery
}

func runBlueprint(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "compile" {
		return fmt.Errorf("usage: wre blueprint compile -in <blueprint.json> [-out <worldpack.json>]")
	}
	fs := flag.NewFlagSet("blueprint compile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	inPath := fs.String("in", "", "path to compact Blueprint JSON file (required)")
	outPath := fs.String("out", "", "optional output path for compiled WorldPack JSON (defaults to stdout)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*inPath) == "" {
		return fmt.Errorf("blueprint compile requires -in <blueprint.json>")
	}
	bp, err := pack.LoadBlueprintFile(*inPath)
	if err != nil {
		return err
	}
	wp, err := pack.CompileBlueprint(*bp)
	if err != nil {
		return err
	}
	if *outPath != "" {
		if err := writeJSONFile(*outPath, wp); err != nil {
			return fmt.Errorf("write world pack %q: %w", *outPath, err)
		}
		return writeJSON(stdout, map[string]any{
			"ok":        true,
			"world_id":  wp.ID,
			"districts": len(wp.Map.Districts),
			"factions":  len(wp.Factions),
			"actors":    len(wp.Actors),
			"out":       *outPath,
		})
	}
	return writeJSON(stdout, wp)
}

func runPack(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "validate" {
		return fmt.Errorf("usage: wre pack validate -pack <worldpack.json>")
	}
	fs := flag.NewFlagSet("pack validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	packPath := fs.String("pack", "", "path to WorldPack JSON file (required)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if strings.TrimSpace(*packPath) == "" {
		return fmt.Errorf("pack validate requires -pack <worldpack.json>")
	}
	wp, err := pack.LoadFile(*packPath)
	if err != nil {
		return err
	}
	w, err := wp.Instantiate("")
	if err != nil {
		return fmt.Errorf("instantiate pack: %w", err)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		return fmt.Errorf("conservation check failed: %w", err)
	}
	return writeJSON(stdout, map[string]any{
		"ok":        true,
		"world_id":  w.ID,
		"genre":     wp.Genre,
		"districts": len(w.Map.Districts),
		"factions":  len(w.Factions),
		"actors":    len(w.Actors),
		"secrets":   len(w.Secrets),
		"conserved": true,
	})
}

func runInject(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("inject", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateIn := fs.String("state-in", "", "path to existing World JSON snapshot (required)")
	patchPath := fs.String("patch", "", "path to ExternalPatch JSON file (required)")
	outState := fs.String("out-state", "", "path to write updated World JSON snapshot (required)")
	outEvents := fs.String("out-events", "", "optional path to write emitted patch Events JSON array")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stateIn == "" || *patchPath == "" || *outState == "" {
		return fmt.Errorf("inject requires -state-in, -patch, and -out-state")
	}
	rawState, err := os.ReadFile(*stateIn)
	if err != nil {
		return fmt.Errorf("read -state-in %q: %w", *stateIn, err)
	}
	w, err := sim.DecodeWorldSnapshot(rawState)
	if err != nil {
		return fmt.Errorf("decode -state-in %q: %w", *stateIn, err)
	}
	rawPatch, err := os.ReadFile(*patchPath)
	if err != nil {
		return fmt.Errorf("read -patch %q: %w", *patchPath, err)
	}
	var patch sim.ExternalPatch
	dec := json.NewDecoder(strings.NewReader(string(rawPatch)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		return fmt.Errorf("decode -patch %q: %w", *patchPath, err)
	}
	events, err := w.ApplyExternalPatch(patch)
	if err != nil {
		return err
	}
	if err := writeJSONFile(*outState, w); err != nil {
		return fmt.Errorf("write -out-state %q: %w", *outState, err)
	}
	if *outEvents != "" {
		if err := writeJSONFile(*outEvents, events); err != nil {
			return fmt.Errorf("write -out-events %q: %w", *outEvents, err)
		}
	}
	return writeJSON(stdout, map[string]any{
		"ok":           true,
		"world_id":     w.ID,
		"tick":         w.Tick,
		"events_added": len(events),
		"actors":       len(w.Actors),
		"out_state":    *outState,
		"out_events":   *outEvents,
	})
}

func runQuery(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)
	eventsFile := fs.String("events-file", "", "comma-separated path(s) to simulation event JSON/NDJSON file(s) (required)")
	actorID := fs.String("actor", "", "filter events involving a specific actor ID")
	between := fs.String("between", "", "filter events involving two actor IDs separated by comma (e.g. a,b)")
	arcID := fs.Int64("arc", 0, "filter events belonging to a causal arc RootEventID")
	fromDay := fs.Int64("from-day", 0, "minimum simulated day (inclusive)")
	toDay := fs.Int64("to-day", 0, "maximum simulated day (inclusive)")
	kinds := fs.String("kind", "", "comma-separated event kinds to include")
	includeRoutine := fs.Bool("include-routine", false, "include routine physiological/wait/move events")
	limit := fs.Int("limit", 200, "maximum matched entries to output (0 = unlimited)")
	format := fs.String("format", "md", "output format: md|json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*eventsFile) == "" {
		return fmt.Errorf("query requires -events-file")
	}
	events, err := loadEventsFromFiles(*eventsFile)
	if err != nil {
		return err
	}
	var betweenA, betweenB string
	if strings.TrimSpace(*between) != "" {
		parts := strings.Split(*between, ",")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("-between requires two actor IDs separated by a comma (e.g. a,b)")
		}
		betweenA = strings.TrimSpace(parts[0])
		betweenB = strings.TrimSpace(parts[1])
	}
	var kindList []string
	if strings.TrimSpace(*kinds) != "" {
		for _, k := range strings.Split(*kinds, ",") {
			if tk := strings.TrimSpace(k); tk != "" {
				kindList = append(kindList, tk)
			}
		}
	}
	res := runtrace.QueryEvents(events, runtrace.EventQueryFilter{
		ActorID:        strings.TrimSpace(*actorID),
		BetweenA:       betweenA,
		BetweenB:       betweenB,
		ArcRootID:      *arcID,
		FromDay:        *fromDay,
		ToDay:          *toDay,
		Kinds:          kindList,
		IncludeRoutine: *includeRoutine,
		Limit:          *limit,
	})
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		return writeJSON(stdout, res)
	case "md", "markdown":
		_, err = io.WriteString(stdout, runtrace.RenderQueryMarkdown(res))
		return err
	default:
		return fmt.Errorf("unsupported query format %q (expected md or json)", *format)
	}
}

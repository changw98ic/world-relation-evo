package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
)

func runTraceAudit(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("audit-trace", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("trace-dir", "", "directory containing initial.json and trace.ndjson")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *dir == "" {
		return fmt.Errorf("audit-trace requires -trace-dir")
	}
	report, err := runtrace.Analyze(*dir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

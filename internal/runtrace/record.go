// Package runtrace records the evidence for a simulation run.
package runtrace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one ordered piece of evidence in trace.ndjson.
type Entry struct {
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Tick    int64           `json:"tick"`
	ActorID string          `json:"actor_id,omitempty"`
	Time    time.Time       `json:"time"`
	Data    json.RawMessage `json:"data"`
}

// Writer serializes concurrent records. A nil Writer explicitly disables tracing.
type Writer struct {
	mu       sync.Mutex
	dir      string
	file     *os.File
	seq      uint64
	writeErr error
	closed   bool
	closeErr error
}

// Create creates a run directory and exclusively creates its trace file.
// Existing evidence is never overwritten.
func Create(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create trace directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "trace.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create trace: %w", err)
	}
	return &Writer{dir: dir, file: file}, nil
}

// Record appends a complete JSON entry before returning. Write failures remain
// visible to subsequent callers because a partial line cannot be repaired here.
func (w *Writer) Record(kind string, tick int64, actorID string, data any) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("record trace: %w", os.ErrClosed)
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode trace data: %w", err)
	}
	entry := Entry{Seq: w.seq + 1, Kind: kind, Tick: tick, ActorID: actorID, Time: time.Now().UTC(), Data: raw}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode trace entry: %w", err)
	}
	line = append(line, '\n')
	n, err := w.file.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.writeErr = fmt.Errorf("write trace: %w", err)
		return w.writeErr
	}
	w.seq = entry.Seq
	return nil
}

// WriteJSON exclusively creates a named JSON evidence file in the run directory.
func (w *Writer) WriteJSON(name string, value any) error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return fmt.Errorf("write trace JSON: %w", os.ErrClosed)
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trace JSON %q: %w", name, err)
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(filepath.Join(w.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create trace JSON %q: %w", name, err)
	}
	n, writeErr := file.Write(raw)
	if writeErr == nil && n != len(raw) {
		writeErr = io.ErrShortWrite
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write trace JSON %q: %w", name, err)
	}
	return nil
}

// Close closes the trace and reports any earlier append failure as well.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		w.closeErr = errors.Join(w.writeErr, w.file.Close())
	}
	return w.closeErr
}

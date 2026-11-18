package runtrace

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentRecordsPreserveCompleteEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	writer, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Error(err)
		}
	})
	const workers, records = 16, 64
	type payload struct {
		Worker int    `json:"worker"`
		Index  int    `json:"index"`
		Text   string `json:"text"`
	}
	const text = "完整证据\nquoted: \"response\""
	failures := make(chan error, workers*records)
	var workersDone sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		workersDone.Add(1)
		go func(worker int) {
			defer workersDone.Done()
			for index := 0; index < records; index++ {
				if err := writer.Record("response", int64(index), fmt.Sprintf("actor-%d", worker), payload{Worker: worker, Index: index, Text: text}); err != nil {
					failures <- err
				}
			}
		}(worker)
	}
	workersDone.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if t.Failed() {
		t.FailNow()
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(dir, "trace.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	seen := make(map[[2]int]bool)
	var seq uint64
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("incomplete JSON line: %v", err)
		}
		seq++
		if entry.Seq != seq || entry.Time.IsZero() || entry.Kind != "response" {
			t.Fatalf("invalid entry metadata: %+v", entry)
		}
		var data payload
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			t.Fatal(err)
		}
		if entry.Tick != int64(data.Index) || entry.ActorID != fmt.Sprintf("actor-%d", data.Worker) || data.Text != text {
			t.Fatalf("evidence changed: entry=%+v payload=%+v", entry, data)
		}
		key := [2]int{data.Worker, data.Index}
		if seen[key] {
			t.Fatalf("duplicated evidence: %v", key)
		}
		seen[key] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if seq != workers*records || len(seen) != workers*records {
		t.Fatalf("lost records: entries=%d unique=%d want=%d", seq, len(seen), workers*records)
	}
}

func TestExistingEvidenceCannotBeOverwritten(t *testing.T) {
	dir := t.TempDir()
	writer, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.Record("original", 9, "actor", map[string]string{"body": "original trace"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "trace.ndjson")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if replacement, err := Create(dir); !errors.Is(err, os.ErrExist) {
		if replacement != nil {
			replacement.Close()
		}
		t.Fatalf("existing trace was not rejected: %v", err)
	}
	if err := writer.WriteJSON("trace.ndjson", "replacement"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("trace overwrite was not rejected: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("existing trace changed: err=%v before=%q after=%q", err, before, after)
	}
	if err := writer.WriteJSON("summary.json", map[string]string{"outcome": "original"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteJSON("summary.json", map[string]string{"outcome": "replacement"}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing JSON was not rejected: %v", err)
	}
	jsonBytes, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary map[string]string
	if err := json.Unmarshal(jsonBytes, &summary); err != nil || summary["outcome"] != "original" {
		t.Fatalf("existing JSON changed: data=%q err=%v", jsonBytes, err)
	}
}

func TestAppendFailureRemainsVisible(t *testing.T) {
	writer, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Close the backing descriptor to inject an actual filesystem write failure.
	// Callers must see it on the first record, later records, and shutdown.
	if err := writer.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record("first", 0, "", nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("first write failure hidden: %v", err)
	}
	if err := writer.Record("next", 1, "", nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("later write pretended success: %v", err)
	}
	if err := writer.Close(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("shutdown hid write failure: %v", err)
	}
}

func TestJSONAndCreationErrorsAreVisible(t *testing.T) {
	dir := t.TempDir()
	writer, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.Record("invalid", 0, "", make(chan int)); err == nil {
		t.Fatal("record encoding failure hidden")
	}
	if err := writer.WriteJSON("invalid.json", make(chan int)); err == nil {
		t.Fatal("JSON encoding failure hidden")
	}
	if _, err := os.Stat(filepath.Join(dir, "invalid.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed encoding created evidence file: %v", err)
	}
	if err := writer.WriteJSON(filepath.Join("missing", "evidence.json"), true); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("JSON creation failure hidden: %v", err)
	}
	if err := writer.Record("valid", 0, "", nil); err != nil {
		t.Fatalf("encoding error prevented later valid record: %v", err)
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if writer, err := Create(filepath.Join(blocked, "run")); err == nil {
		writer.Close()
		t.Fatal("directory creation failure hidden")
	}
}

func TestClosedAndDisabledWriters(t *testing.T) {
	var disabled *Writer
	if err := disabled.Record("disabled", 0, "", make(chan int)); err != nil {
		t.Fatal(err)
	}
	if err := disabled.WriteJSON("disabled.json", make(chan int)); err != nil {
		t.Fatal(err)
	}
	if err := disabled.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record("closed", 0, "", nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed record pretended success: %v", err)
	}
	if err := writer.WriteJSON("closed.json", nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed JSON write pretended success: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("repeat close failed: %v", err)
	}
}

package sim

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMemoryReloadPreservesAttributionWhenUpdatingOnlyTheText(t *testing.T) {
	w := lawWorld()
	for i := 0; i < 20; i++ {
		w.record(&w.Actors[0], Action{}, "work_report", map[string]any{"text": "stone"})
	}
	if err := json.Unmarshal([]byte(`{"text":"copper"}`), &w.Actors[0].Memories[0]); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`null`), &w.Actors[0].Memories[0]); err != nil {
		t.Fatal(err)
	}
	recalled := RecallMemories(w.Actors[0], MemoryCues{Terms: []string{"copper"}}, 9)
	if recalled[0].EventID != 1 || recalled[0].Source != "a" || recalled[0].RootEventID != 1 || recalled[0].Text != "copper" {
		t.Fatal("updating search data lost the original experience or ignored the updated text")
	}
}

func BenchmarkRecallRecordedHistory(b *testing.B) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Goal = "find food and timber while trading at the river market"
	for i := 0; i < 5000; i++ {
		w.record(a, Action{Kind: "speak"}, "work_report", map[string]any{"text": strings.Repeat("The worker carried timber, found food and reported at the river market. ", 5)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RecallMemories(*a, MemoryCues{Terms: []string{"food", "timber"}}, 32)
	}
}

func TestRecallUsesCurrentFactsAfterHistoryEditAndSnapshotReload(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	for i := 0; i < 20; i++ {
		text := "stone"
		if i == 0 {
			text = "copper"
		}
		w.record(a, Action{}, "work_report", map[string]any{"text": text})
	}
	a.Memories[0].Text = "stone"
	a.Memories[1].Text = "copper"
	before := lawJSON(t, w)
	recalled := RecallMemories(*a, MemoryCues{Terms: []string{"copper"}}, 9)
	if len(recalled) != 9 || recalled[0].EventID != 2 {
		t.Fatal("recall ignored the actual changed history")
	}
	reloaded := lawRestore(t, w)
	afterReload := RecallMemories(reloaded.Actors[0], MemoryCues{Terms: []string{"copper"}}, 9)
	if !bytes.Equal(lawJSON(t, recalled), lawJSON(t, afterReload)) || !bytes.Equal(before, lawJSON(t, w)) {
		t.Fatal("recall changed history or snapshot restoration changed the recalled facts")
	}
}

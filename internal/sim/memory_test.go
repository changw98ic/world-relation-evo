package sim

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestLongTermPersonalRecall(t *testing.T) {
	a := Actor{ID: "self", Goal: "recover stolen grain"}
	old := Memory{EventID: 1, RootEventID: 1, Kind: "attacked", Source: "robber", TargetID: "self", Text: "lost grain", Observation: &Observation{Kind: "attack", Item: "grain", Quantity: 3}}
	a.Memories = appendMemory(a.Memories, old)
	hearsay := Memory{EventID: 2, RootEventID: 1, BasisEventID: 1, Kind: "speech", Source: "friend", SourceTags: []string{"farmer"}, Statement: &Statement{Topic: "grain", Quantity: 9}, Text: "grain claim"}
	a.Memories = appendMemory(a.Memories, hearsay)
	for i := 3; i <= 150; i++ {
		a.Memories = appendMemory(a.Memories, Memory{EventID: int64(i), Kind: "rest", Source: "self"})
	}
	if len(a.Memories) != 150 {
		t.Fatalf("history lost: %d", len(a.Memories))
	}
	// State persistence carries the whole personal history, not just attention.
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var restored Actor
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	got := RecallMemories(restored, MemoryCues{ActorIDs: []string{"friend"}}, 32)
	if len(got) != 32 || !reflect.DeepEqual(got[0], old) || !reflect.DeepEqual(got[1], hearsay) {
		t.Fatalf("missing old relevant experiences: %+v", got)
	}
	if got[len(got)-1].EventID != 150 {
		t.Fatal("recent context lost")
	}
	if !reflect.DeepEqual(got, RecallMemories(restored, MemoryCues{ActorIDs: []string{"friend"}}, 32)) {
		t.Fatal("nondeterministic recall")
	}
	got[0].Observation.Quantity = 999
	got[1].Statement.Quantity = 999
	got[1].SourceTags[0] = "changed"
	if restored.Memories[0].Observation.Quantity != 3 || restored.Memories[1].Statement.Quantity != 9 || restored.Memories[1].SourceTags[0] != "farmer" {
		t.Fatal("recall mutated history")
	}
}

func TestRecallOnlyUsesPersonalHistory(t *testing.T) {
	a := Actor{ID: "a", Memories: []Memory{{EventID: 1, Source: "b", Kind: "speech", Statement: &Statement{Topic: "food"}}}}
	got := RecallMemories(a, MemoryCues{ActorIDs: []string{"b"}, Terms: []string{"food"}}, 32)
	if len(got) != 1 || got[0].Observation != nil || got[0].Statement == nil {
		t.Fatalf("statement semantic changed: %+v", got)
	}
	if len(RecallMemories(Actor{ID: "b"}, MemoryCues{ActorIDs: []string{"a"}}, 32)) != 0 {
		t.Fatal("leaked other actor history")
	}
	if RecallMemories(a, MemoryCues{}, 0) != nil {
		t.Fatal("zero attention should be empty")
	}
}

func TestBothPerceptionPathsRecallHistory(t *testing.T) {
	w := Village("recall")
	a := &w.Actors[0]
	a.Goal = "grain"
	a.Memories = []Memory{{EventID: 1, Source: a.ID, Kind: "observed", Observation: &Observation{Item: "grain", Quantity: 7}}}
	for i := 2; i <= 100; i++ {
		a.Memories = appendMemory(a.Memories, Memory{EventID: int64(i), Kind: "rest"})
	}
	w.Actors[1].Memories = []Memory{{EventID: 9999, Text: "private secret"}}
	regular := w.View(a.ID).Self.Memories
	indexed := w.viewIndexed(0, w.buildStepIndex()).Self.Memories
	if !reflect.DeepEqual(regular, indexed) {
		t.Fatal("indexed recall differs")
	}
	if len(regular) != MaxRecalledMemories || regular[0].EventID != 1 {
		t.Fatalf("old relevant memory missing: %+v", regular)
	}
	for _, m := range regular {
		if m.EventID == 9999 {
			t.Fatal("private knowledge leaked")
		}
	}
	regular[0].Observation.Quantity = 0
	if a.Memories[0].Observation.Quantity != 7 || len(a.Memories) != 100 {
		t.Fatal("view modified durable history")
	}
}

func TestRecallUsesActivePlanAndAgreementCues(t *testing.T) {
	a := Actor{ID: "self", Plan: &PersonalPlan{Status: "active", Item: "medicine"}, Memories: []Memory{
		{EventID: 1, Text: "medicine recipe observed"},
		{EventID: 2, Source: "supplier", Text: "a previous promise"},
	}}
	for i := 3; i <= 100; i++ {
		a.Memories = appendMemory(a.Memories, Memory{EventID: int64(i), Kind: "rest"})
	}
	p := Perception{Self: a, Continuity: ContinuityView{Agreements: []Agreement{{BuyerID: "self", SupplierID: "supplier", Status: "active", Item: "herbs"}}}}
	recallPerceptionMemories(&p)
	if p.Self.Memories[0].EventID != 1 || p.Self.Memories[1].EventID != 2 {
		t.Fatalf("plan/agreement history not recalled: %+v", p.Self.Memories)
	}
}

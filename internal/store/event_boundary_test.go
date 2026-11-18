package store

import (
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestEventCompactionKeepsCausalEconomicAndFailureEvidenceVerbatim(t *testing.T) {
	basis := sim.Event{ID: 3, Kind: "speech", ActorID: "a", Action: sim.Action{Kind: "speak", BasisEventID: 41}, Payload: map[string]any{"text": "heard news", "root_event_id": 12}}
	claim := sim.Event{ID: 4, Kind: "speech", Action: sim.Action{Kind: "speak", Statement: &sim.Statement{Quantity: 0}}}
	delivery := sim.Event{ID: 5, Kind: "agreement_fulfilled", TargetID: "buyer", Payload: map[string]any{"quantity": 3, "seller_id": "seller"}}
	failure := sim.Event{ID: 6, Kind: "action_failed", Payload: map[string]any{"text": "unavailable payment"}}
	future := sim.Event{ID: 7, Kind: "new_unrecognized_causal_event"}
	input := []sim.Event{{ID: 1, Kind: "moved"}, {ID: 2, Kind: "speech", Action: sim.Action{Text: "routine greeting"}}, basis, claim, delivery, failure, future}
	before := append([]sim.Event(nil), input...)
	want := []sim.Event{basis, claim, delivery, failure, future}
	if got := CompactEvents(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("compaction changed retained event identities, order or content: %+v", got)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("compaction rewrote its caller's settled events")
	}
	if got := CompactEvents(nil); len(got) != 0 {
		t.Fatal("empty successful event batch invented history")
	}
}

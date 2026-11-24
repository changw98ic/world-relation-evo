package store

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestEventJSONKeepsExactNumbersAndRejectsInvalidAmounts(t *testing.T) {
	const id = int64(9007199254740993)
	var event sim.Event
	if err := json.Unmarshal([]byte(`{"id":7,"kind":"trade_counteroffered","payload":{"root_event_id":9007199254740993,"previous_offer_id":9007199254740993,"case_id":9007199254740993,"quantity":2,"paid_coins":3,"other":{"id":9007199254740993,"rate":2.75}}}`), &event); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"root_event_id", "previous_offer_id", "case_id"} {
		if event.Payload[key] != id {
			t.Fatalf("%s = %v, want %d", key, event.Payload[key], id)
		}
	}
	if event.Payload["quantity"] != int64(2) || event.Payload["paid_coins"] != int64(3) ||
		!reflect.DeepEqual(event.Payload["other"], map[string]any{"id": json.Number("9007199254740993"), "rate": json.Number("2.75")}) {
		t.Fatalf("unexpected event amounts %+v", event.Payload)
	}
	for _, tc := range []struct{ field, value string }{{"quantity", "2.75"}, {"paid_coins", `"3"`}, {"case_id", "null"}} {
		raw := `{"id":7,"kind":"test","payload":{"` + tc.field + `":` + tc.value + `}}`
		if err := json.Unmarshal([]byte(raw), &event); err == nil || !strings.Contains(err.Error(), tc.field) {
			t.Errorf("expected field-specific error for %s, got %v", raw, err)
		}
	}
}

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

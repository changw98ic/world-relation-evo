package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestModelRequestsDoNotReplayEveryOldDialogueVerbatim(t *testing.T) {
	p := boundaryWorld().View("alice")
	p.Self.Memories = []sim.Memory{
		{EventID: 31, RootEventID: 31, Source: "bob", TargetID: "alice", Kind: "speech", Text: "old-dialogue-body", Statement: &sim.Statement{Topic: "item_produced", Item: "food", Quantity: 7}},
		{EventID: 32, RootEventID: 31, Source: "bob", TargetID: "alice", Kind: "speech", Text: "current-dialogue-body"},
		{EventID: 33, RootEventID: 33, Source: "alice", Kind: "crafted", Text: "witnessed-production", Observation: &sim.Observation{Kind: "item_produced", Item: "food", Quantity: 2}},
	}
	p.Continuity.Dialogue = []sim.DialogueTurn{
		{EventID: 31, SpeakerID: "bob", ListenerID: "alice", Kind: "speech", Text: "old-dialogue-body"},
		{EventID: 32, SpeakerID: "bob", ListenerID: "alice", Kind: "speech", Text: "current-dialogue-body"},
	}
	before := boundaryJSON(t, p)
	check := func(raw []byte) {
		t.Helper()
		if strings.Contains(string(raw), "old-dialogue-body") || !strings.Contains(string(raw), "current-dialogue-body") || !strings.Contains(string(raw), "witnessed-production") {
			t.Error("request must retain latest dialogue and witnessed events, not repeatedly replay old utterances")
		}
		if !strings.Contains(string(raw), "omitted_dialogue_text_event_ids") || !strings.Contains(string(raw), "item_produced") {
			t.Error("omitted text must be disclosed while structured claims and sources remain")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.URL.Path != "/systemone" {
			t.Errorf("unexpected path %s", r.URL.Path)
			rw.WriteHeader(404)
			return
		}
		check(raw)
		fmt.Fprint(rw, `{"answers":{"decision":{"type":"choice","choice":"rest"}}}`)
	}))
	defer server.Close()
	if _, err := (Jev{Endpoint: server.URL + "/systemone", APIKey: "fixture", Client: server.Client()}).Select(context.Background(), p, []Option{{ID: "rest", Action: sim.Action{Kind: "wait"}}}); err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, p) != before {
		t.Fatal("model context selection modified the original archived perception")
	}
}

func TestModelContextKeepsOpenObligationsAndDisclosesOlderSettlements(t *testing.T) {
	p := boundaryWorld().View("alice")
	for i := 1; i <= 64; i++ {
		p.Continuity.Agreements = append(p.Continuity.Agreements, sim.Agreement{ID: int64(i), SupplierID: "alice", BuyerID: "bob", Item: "food", Status: "fulfilled", ClosedTick: int64(i)})
	}
	p.Continuity.Agreements = append(p.Continuity.Agreements, sim.Agreement{ID: 100, SupplierID: "alice", BuyerID: "bob", Item: "food", Status: "active", Quantity: 2, Price: 3})
	before := boundaryJSON(t, p)
	projected := contextForModel(p)
	if len(projected.Continuity.Agreements) != 2 || projected.ArchivedClosedAgreements != 63 || projected.Continuity.Agreements[0].ID != 64 || projected.Continuity.Agreements[1].ID != 100 || projected.Continuity.Agreements[1].Quantity != 2 {
		t.Fatal("open obligations or the latest actual settlement were lost, or archived history was concealed")
	}
	if boundaryJSON(t, p) != before {
		t.Fatal("request projection rewrote persistent contract history")
	}
}

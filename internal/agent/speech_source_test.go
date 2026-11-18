package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPriceNegotiationRetainsItsMeaningAcrossTwoListeners(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.CommChannels = map[string]sim.CommChannelSpec{"test": {ID: "test"}}
	offer := boundaryOption(t, w, "alice", "propose_agreement", "bob")
	proposal := boundaryApply(t, w, "alice", offer)[0]
	text := "三份粮仍收两枚钱。"
	for _, transport := range []string{"speak", "transmit_message"} {
		bad := sim.Action{Kind: transport, TargetID: "bob", Text: text, BasisEventID: proposal.ID, Statement: &sim.Statement{Topic: "item_produced", Item: offer.Item, Quantity: 3}}
		evs := w.Apply("alice", bad)
		if len(evs) != 1 || evs[0].Kind != "action_failed" || !strings.Contains(evs[0].Payload["text"].(string), "production") {
			t.Fatalf("%s used a quote as evidence of production: %+v", transport, evs)
		}
	}
	quote := &sim.Statement{Topic: "trade_negotiation", Item: offer.Item, Location: "meeting", Quantity: 3}
	spoken := boundaryApply(t, w, "alice", sim.Action{Kind: "speak", TargetID: "bob", BasisEventID: proposal.ID, Text: text, Statement: quote})[0]
	reply, err := (TemplateContent{}).Generate(context.Background(), w.View("bob"), sim.Action{Kind: "speak", TargetID: "charlie"})
	if err != nil || reply.BasisEventID != spoken.ID || reply.Statement == nil || *reply.Statement != *quote || !strings.Contains(reply.Text, text) || strings.Contains(reply.Text, "produced") {
		t.Fatalf("counteroffer did not retain its actual terms: %+v %v", reply, err)
	}
	boundaryApply(t, w, "bob", sim.Action{Kind: "speak", TargetID: "charlie", Text: reply.Text, Statement: reply.Statement, BasisEventID: reply.BasisEventID})
	last := w.Actors[2].Memories[len(w.Actors[2].Memories)-1]
	if last.RootEventID != proposal.ID || last.Observation != nil || *last.Statement != *quote || w.Continuity.Agreements[0].Quantity != offer.Quantity || w.Actors[0].Items["food"] != 20 {
		t.Fatal("dialogue changed formal terms, assets, or provenance")
	}
}

func TestContentModelChecksMeaningOfProductionCitation(t *testing.T) {
	w := boundaryWorld()
	w.Actors[0].Memories = []sim.Memory{{EventID: 1158, Kind: "agreement_proposed", Text: "2 food for 2 coins"}}
	v := w.View("alice")
	for _, tc := range []struct {
		response string
		valid    bool
	}{
		{`{"text":"三份粮仍收两枚钱。","basis_event_id":1158,"statement":{"topic":"item_produced","item":"food","quantity":3}}`, false},
		{`{"text":"三份粮仍收两枚钱。","basis_event_id":1158,"statement":{"topic":"trade_negotiation","item":"food","quantity":3}}`, true},
		{`{"text":"我有三份粮。","statement":{"topic":"stock","item":"food","quantity":3}}`, true},
		{`{"text":"我种了三份粮。","statement":{"topic":"item_produced","item":"food","quantity":3}}`, true},
		{`{"text":"我再想想。","basis_event_id":1158}`, true},
	} {
		server := localContentServer(t, tc.response)
		_, err := (ContentModel{Endpoint: server.URL, Client: server.Client()}).Generate(context.Background(), v, sim.Action{Kind: "speak", TargetID: "bob"})
		if (err == nil) != tc.valid {
			t.Fatalf("valid=%v response=%s err=%v", tc.valid, tc.response, err)
		}
	}
}

func TestRulesPreservePartialKnowledgeOfWhatWasSeenOrHeard(t *testing.T) {
	w := boundaryWorld()
	w.Actors[0].Memories = []sim.Memory{{EventID: 42, RootEventID: 42, Kind: "speech", Source: "charlie", Text: "I have three units left", Statement: &sim.Statement{Topic: "supply", Quantity: 3, Location: "remote"}}}
	v := w.View("alice")
	action, ok := informationAction(v)
	if !ok || action.Kind != "speak" || action.Statement == nil || action.Statement.Item != "" || !strings.Contains(action.Text, "I have three units left") {
		t.Fatalf("an unspecified supply was given an item or changed on retelling: %+v", action)
	}
	report, ok := reportFromMemory(sim.Memory{EventID: 43, Source: "charlie", Observation: &sim.Observation{Kind: "item_produced", Item: "grain", Location: "field", Quantity: 2}}, v.Self, "bob")
	if !ok || !strings.Contains(report.Text, "I saw 2") || strings.Contains(report.Text, "I produced") {
		t.Fatalf("witness claimed the producer's work: %+v", report)
	}
}

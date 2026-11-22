package runtrace

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestP6ChronicleIsolationDisguiseUnverifiedClaimsAndReadOnlySnapshot(t *testing.T) {
	w := sim.Village("p6-chronicle")
	w.Catalog.Forms = map[string]sim.FormSpec{
		"mortal": {ID: "mortal", Name: "Mortal", PowerTier: 1},
		"saint":  {ID: "saint", Name: "Hidden Saint", PowerTier: 5},
	}
	w.Catalog.GoldenFingers = []sim.GoldenFingerSpec{{
		ID:            "veil",
		Name:          "Veil",
		GrantedByTags: []string{"veiled"},
		Passive:       &sim.GFPassiveSpec{DisguiseFormID: "mortal"},
	}}
	w.Actors[0].FormID = "saint"
	w.Actors[0].Tags = append(w.Actors[0].Tags, "veiled")

	events := []sim.Event{
		{
			ID:      10,
			Tick:    10,
			ActorID: "p01",
			Kind:    "speech",
			Payload: map[string]any{"text": "The granary is empty!"},
		},
		{
			ID:      11,
			Tick:    11,
			ActorID: "world",
			Kind:    "rumor_panic_triggered",
			Payload: map[string]any{
				"text":          "Rumor panic erupted over food",
				"verified":      false,
				"root_event_id": int64(10),
			},
		},
		{
			ID:       12,
			Tick:     12,
			ActorID:  "p01",
			TargetID: "faction_alpha",
			Kind:     "faction_war_declared",
			Payload:  map[string]any{"text": "War declared"},
		},
		{
			ID:       13,
			Tick:     13,
			ActorID:  "p02",
			TargetID: "p01",
			Kind:     "secret_exposed",
			Payload:  map[string]any{"text": "Exposed", "witnesses": []string{"p03"}},
		},
	}

	snap := ProjectWorldMetadata(w)
	rep := BuildChronicleReadOnly(events, snap, 50)

	// Biographies expose the observed form and omit individual wallet balances.
	bios, err := json.Marshal(rep.Biographies)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Biographies) == 0 || strings.Contains(string(bios), `"coins":`) {
		t.Fatalf("public biographies are empty or disclose private wallets: %s", bios)
	}
	for _, bio := range rep.Biographies {
		if bio.ID == w.Actors[0].ID && bio.PowerTier != 1 {
			t.Fatalf("chronicle leaked true PowerTier %d of disguised actor (want perceived 1)", bio.PowerTier)
		}
	}

	// 2. Unverified rumor arc must be explicitly tagged "[Unverified claim]".
	foundRumorArc := false
	for _, arc := range rep.Arcs {
		if arc.RootEventID == 10 {
			foundRumorArc = true
			if !strings.HasPrefix(arc.Summary, "[Unverified claim]") {
				t.Fatalf("expected unverified rumor arc to be prefixed with [Unverified claim], got %q", arc.Summary)
			}
		}
		// 3. Non-person TargetID ("faction_alpha") must not pollute Participants.
		for _, pid := range arc.Participants {
			if pid == "faction_alpha" {
				t.Fatalf("non-person faction ID polluted ChronicleArc.Participants: %v", arc.Participants)
			}
		}
	}
	if !foundRumorArc {
		t.Fatalf("expected rumor arc for root event 10")
	}

	// 4. QueryEvents finds witness p03 on secret_exposed with string-slice witnesses payload.
	qRes := QueryEvents(events, EventQueryFilter{ActorID: "p03"})
	if qRes.TotalMatched != 1 {
		t.Fatalf("expected QueryEvents to match witness p03 on secret_exposed, got %d", qRes.TotalMatched)
	}

	// 5. BuildChronicle delegates through ProjectWorldMetadata and matches BuildChronicleReadOnly identically.
	repDirect := BuildChronicle(events, w, 50)
	if !reflect.DeepEqual(repDirect, rep) {
		t.Fatalf("BuildChronicle diverged from BuildChronicleReadOnly(ProjectWorldMetadata)")
	}

	// 6. Analyze accepts explicit "ext:" extension entries while strictly rejecting unprefixed unknown kinds.
	dir := t.TempDir()
	wr, err := Create(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := wr.WriteJSON("initial.json", *w); err != nil {
		t.Fatal(err)
	}
	_ = wr.Record("run_start", 0, "", map[string]any{"decider": "rules"})
	_ = wr.Record("ext:custom_future_metric", 0, "p01", map[string]any{"score": 42})
	_ = wr.Record("run_end", 0, "", map[string]any{"completed": true})
	if err := wr.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(dir); err != nil {
		t.Fatalf("Analyze rejected explicit ext: entry kind: %v", err)
	}

	badDir := t.TempDir()
	badWr, err := Create(badDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := badWr.WriteJSON("initial.json", *w); err != nil {
		t.Fatal(err)
	}
	_ = badWr.Record("run_start", 0, "", map[string]any{"decider": "rules"})
	_ = badWr.Record("bogus_corrupt_entry", 0, "p01", map[string]any{"score": 42})
	_ = badWr.Record("run_end", 0, "", map[string]any{"completed": true})
	if err := badWr.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(badDir); err == nil {
		t.Fatalf("Analyze silently ignored unknown unprefixed entry kind; expected error")
	}
}

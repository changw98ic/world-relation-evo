package sim

import (
	"encoding/json"
	"testing"
)

func TestExternalPatchDistinguishesAbsentValuesFromZero(t *testing.T) {
	w := lawWorld()
	w.InitLedgerAudit()
	w.Factions = []Faction{{ID: "one"}, {ID: "two"}}
	w.ensureFactionDiplomacy("one", "two").RelationScore = 30
	var patch ExternalPatch
	const input = `{
		"spawn_actors": [
			{"id":"tired","energy":0},
			{"id":"deceased","health":0,"energy":0},
			{"id":"rested","health":100,"energy":100}
		],
		"set_diplomacy": [{"faction_a":"one","faction_b":"two","stance":"neutral"}]
	}`
	if err := json.Unmarshal([]byte(input), &patch); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyExternalPatch(patch); err != nil {
		t.Fatal(err)
	}
	if a := w.actor("tired"); a.Health != MaxHealth || a.Energy != 0 || a.Dead {
		t.Fatalf("omitted health or explicit zero energy was changed: %+v", a)
	}
	if a := w.actor("deceased"); a.Health != 0 || a.Energy != 0 || !a.Dead {
		t.Fatalf("explicitly dead actor was restored to full vitals: %+v", a)
	}
	if a := w.actor("rested"); a.Health != MaxHealth || a.Energy != MaxEnergy {
		t.Fatalf("valid maximum vitals changed: %+v", a)
	}
	rel := w.ensureFactionDiplomacy("one", "two")
	if rel.RelationScore != 30 {
		t.Fatalf("omitted diplomacy score changed to %d", rel.RelationScore)
	}
	patch = ExternalPatch{}
	if err := json.Unmarshal([]byte(`{"set_diplomacy":[{"faction_a":"one","faction_b":"two","relation_score":0}]}`), &patch); err != nil {
		t.Fatal(err)
	}
	events, err := w.ApplyExternalPatch(patch)
	if err != nil || rel.RelationScore != 0 || len(events) != 1 || rel.RootEventID != events[0].ID {
		t.Fatalf("zero score or update evidence was lost: %+v, %v", events, err)
	}
}

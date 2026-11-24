package pack

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func encodeBoundary(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPackInstancesDoNotShareMutableDefinitionState(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*sim.World)
	}{
		{"inventory", func(w *sim.World) { w.Actors[0].Items["grain"] = 99 }},
		{"house_residents", func(w *sim.World) { w.Houses[0].Residents[0] = "changed" }},
		{"field_rights", func(w *sim.World) { w.Production.Fields[0].UserID = "changed" }},
		{"stored_person_inventory", func(w *sim.World) { w.Cohorts[0].Members[0].Items["grain"] = 99 }},
		{"memory_observation", func(w *sim.World) { w.Actors[0].Memories[0].Observation.Quantity = 99 }},
		{"memory_statement", func(w *sim.World) { w.Actors[0].Memories[0].Statement.Quantity = 99 }},
		{"task_statement", func(w *sim.World) { w.Actors[0].Task.Action.Statement.Quantity = 99 }},
		{"task_care_terms", func(w *sim.World) { w.Actors[0].Task.Action.Care.SharePermille = 999 }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			p := rightsPack()
			p.Actors[0].Memories = []sim.Memory{{EventID: 7, Observation: &sim.Observation{Quantity: 3}, Statement: &sim.Statement{Quantity: 5}}}
			p.Actors[0].Task = &sim.Task{Action: sim.Action{Kind: "wait", Statement: &sim.Statement{Quantity: 7}, Care: &sim.GrowthCareAgreement{SharePermille: 500}}}
			beforeDefinition := encodeBoundary(t, p)
			first, err := p.Instantiate("first")
			if err != nil {
				t.Fatal(err)
			}
			second, err := p.Instantiate("second")
			if err != nil {
				t.Fatal(err)
			}
			beforeSecond := encodeBoundary(t, second)
			tc.mutate(first)
			if !reflect.DeepEqual(beforeDefinition, encodeBoundary(t, p)) {
				t.Error("changing a run rewrote the caller-owned pack")
			}
			if !reflect.DeepEqual(beforeSecond, encodeBoundary(t, second)) {
				t.Error("changing one run rewrote a separate run")
			}
		})
	}
}

// This is a loader/serialization check, not an autonomous-society quality test.
func TestShippedPackDefinitionsPreserveNamedRightsOnJSONResume(t *testing.T) {
	for _, name := range []string{"mortal_watershed.json", "xianxia_sect.json", "wasteland_sector.json"} {
		t.Run(name, func(t *testing.T) {
			p, err := LoadFile(filepath.Join("..", "..", "packs", name))
			if err != nil {
				t.Fatal(err)
			}
			before := encodeBoundary(t, p)
			w, err := p.Instantiate("")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, encodeBoundary(t, p)) {
				t.Fatal("loading changed the definition")
			}
			var resumed sim.World
			if err := json.Unmarshal(encodeBoundary(t, w), &resumed); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(encodeBoundary(t, w), encodeBoundary(t, &resumed)) {
				t.Fatal("persistent state changed during JSON resume")
			}
			for i, house := range p.Houses {
				if resumed.Houses[i].OwnerID != house.OwnerID || !reflect.DeepEqual(resumed.Houses[i].Residents, house.Residents) {
					t.Fatal("resume reassigned house rights")
				}
			}
			if p.Production != nil && !reflect.DeepEqual(resumed.Production.Fields, p.Production.Fields) {
				t.Fatal("resume reassigned declared land rights")
			}
		})
	}
}

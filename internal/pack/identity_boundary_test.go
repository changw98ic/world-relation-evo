package pack

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// The fixture declares its own people and property. Expected rights do not come
// from the loader or from a successful conservation audit.
func rightsPack() *WorldPack {
	cat := sim.DefaultCatalog()
	var features []string
	seen := map[string]bool{}
	for _, r := range cat.Recipes {
		if r.NodeFeature != "" && !seen[r.NodeFeature] {
			seen[r.NodeFeature] = true
			features = append(features, r.NodeFeature)
		}
	}
	return &WorldPack{
		ID:         "rights-fixture",
		Catalog:    cat,
		Map:        sim.WorldMap{Districts: []sim.District{{ID: "farm"}, {ID: "town", Features: features}}, Venues: []sim.Venue{{ID: "plot", DistrictID: "farm"}}},
		Actors:     []sim.Actor{{ID: "tenant", Location: "farm", VenueID: "plot", HomeID: "home", Guilds: []string{"guild"}, Tags: []string{"resident"}, Health: 100, Energy: 100, Items: map[string]int{"grain": 0}, Money: 0}},
		Cohorts:    []sim.PopulationCohort{{ID: "neighbors", District: "farm", Population: 1, Adults: 1, Members: []sim.Actor{{ID: "landlord", Location: "farm", Health: 100, Energy: 100, Items: map[string]int{"grain": 3}}}}},
		Houses:     []sim.House{{ID: "home", District: "farm", VenueID: "plot", OwnerID: "landlord", Residents: []string{"tenant"}, Level: 1, Condition: 100}},
		Factions:   []sim.Faction{{ID: "guild", District: "farm", Members: []string{"tenant"}}},
		Production: &sim.ProductionState{Fields: []sim.FieldAsset{{ID: "field", District: "farm", VenueID: "plot", AreaM2: 1, OwnerKind: "actor", OwnerID: "landlord", UserKind: "actor", UserID: "tenant"}}},
	}
}

func TestPackRightsReferToDistinctExistingPeople(t *testing.T) {
	p := rightsPack()
	w, err := p.Instantiate("separate-run")
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != "separate-run" || p.ID != "rights-fixture" {
		t.Fatal("run identity overwrote the definition")
	}
	field := w.Production.Fields[0]
	if field.OwnerID != "landlord" || field.UserID != "tenant" || w.Houses[0].OwnerID != "landlord" || w.Cohorts[0].Members[0].ID != "landlord" {
		t.Fatal("instantiation reassigned a stored person's property or use rights")
	}
	if w.Actors[0].Money != 0 || w.Actors[0].Items["grain"] != 0 {
		t.Fatal("zero holdings acquired invented assets")
	}
	p.Production.Fields[0].OwnerKind, p.Production.Fields[0].OwnerID = "cohort", "neighbors"
	if err := p.Validate(); err != nil {
		t.Fatalf("a declared cohort can own property: %v", err)
	}
}

func TestPackRejectsDanglingIdentityAndPropertyReferences(t *testing.T) {
	cases := []struct {
		name   string
		change func(*WorldPack)
	}{
		{"active_stored_identity_collision", func(p *WorldPack) { p.Cohorts[0].Members[0].ID = "tenant" }},
		{"stored_people_exceed_population", func(p *WorldPack) { p.Cohorts[0].Population = 0 }},
		{"house_owner", func(p *WorldPack) { p.Houses[0].OwnerID = "absent" }},
		{"house_resident", func(p *WorldPack) { p.Houses[0].Residents = []string{"absent"} }},
		{"actor_guild", func(p *WorldPack) { p.Actors[0].Guilds = []string{"absent"} }},
		{"faction_member", func(p *WorldPack) { p.Factions[0].Members = []string{"absent"} }},
		{"field_owner", func(p *WorldPack) { p.Production.Fields[0].OwnerID = "absent" }},
		{"field_user", func(p *WorldPack) { p.Production.Fields[0].UserID = "absent" }},
		{"wrong_holder_kind", func(p *WorldPack) { p.Production.Fields[0].OwnerKind = "faction" }},
		{"field_venue_in_another_district", func(p *WorldPack) { p.Map.Venues[0].DistrictID = "town"; p.Actors[0].VenueID = "" }},
		{"negative_named_person_stock", func(p *WorldPack) { p.Cohorts[0].Members[0].Items["grain"] = -1 }},
		{"zero_field_area", func(p *WorldPack) { p.Production.Fields[0].AreaM2 = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := rightsPack()
			tc.change(p)
			if err := p.Validate(); err == nil {
				t.Fatal("invalid external identity/property definition was accepted")
			}
			if _, err := p.Instantiate(""); err == nil {
				t.Fatal("instantiation bypassed definition validation")
			}
		})
	}
}

func TestPackDecodeRequiresOneCompleteJSONDefinition(t *testing.T) {
	raw, err := json.Marshal(rightsPack())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{` {"id":"another-world"}`, ` trailing-garbage`} {
		if _, err := Decode(strings.NewReader(string(raw) + suffix)); err == nil {
			t.Errorf("accepted content after the definition: %q", suffix)
		}
	}
}

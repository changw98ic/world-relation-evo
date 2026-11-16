package agent_test

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPipelineDeclarativeActionOptions(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.ActionSpecs = map[string]sim.ActionSpec{
		"meditate_self": {
			ID:            "meditate_self",
			Name:          "Meditate",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "self",
			EnergyCost:    5,
		},
		"ward_venue": {
			ID:                   "ward_venue",
			Name:                 "Ward Venue",
			GrantedByTags:        []string{"cultivator"},
			TargetKind:           "venue",
			RequiredVenueFeature: "spirit_vein",
			SlotCosts:            map[string]int{"qi": 3},
		},
		"blight_district": {
			ID:            "blight_district",
			Name:          "Bless District",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "district",
		},
		"palm_strike": {
			ID:            "palm_strike",
			Name:          "Palm Strike",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "actor",
		},
		"rally_cohort": {
			ID:            "rally_cohort",
			Name:          "Rally Cohort",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "cohort",
		},
		"forbidden_law": {
			ID:                   "forbidden_law",
			Name:                 "Rewrite Law",
			GrantedByTags:        []string{"cultivator"},
			TargetKind:           "self",
			AllowRewriteWorldLaw: false,
			WorldLawDelta:        map[string]int{"gravity": -1},
		},
	}

	p := sim.Perception{
		Self: sim.Actor{
			ID:         "c1",
			Name:       "Lin",
			Tags:       []string{"resident", "cultivator"},
			Location:   "d1",
			VenueID:    "v1",
			Health:     100,
			Energy:     20,
			StateSlots: map[string]int{"qi": 5},
			Items:      map[string]int{},
			Trust:      map[string]int{},
		},
		CurrentDistrict: sim.District{ID: "d1", Name: "Peak"},
		CurrentVenue:    &sim.Venue{ID: "v1", Name: "Sanctum", DistrictID: "d1", Features: []string{"spirit_vein"}},
		Nearby: []sim.Neighbor{
			{ID: "a2", Name: "Rival", Tags: []string{"resident"}},
		},
		LocalCohorts: []sim.CohortView{
			{ID: "coh1", Name: "Disciples", District: "d1", Profession: "farmer", Population: 10},
		},
		Map: sim.WorldMap{
			Districts: []sim.District{{ID: "d1", Name: "Peak"}},
			Venues:    []sim.Venue{{ID: "v1", Name: "Sanctum", DistrictID: "d1", Features: []string{"spirit_vein"}}},
		},
		Catalog: cat,
	}

	opts := agent.Options(p)
	seen := map[string]sim.Action{}
	for _, o := range opts {
		seen[o.Action.Kind] = o.Action
	}
	if _, ok := seen["meditate_self"]; !ok {
		t.Fatal("expected meditate_self option for self target")
	}
	if a, ok := seen["ward_venue"]; !ok || a.Location != "v1" {
		t.Fatalf("expected ward_venue targeting v1, got %+v", seen["ward_venue"])
	}
	if a, ok := seen["blight_district"]; !ok || a.Location != "d1" {
		t.Fatalf("expected blight_district targeting d1, got %+v", seen["blight_district"])
	}
	if a, ok := seen["palm_strike"]; !ok || a.TargetID != "a2" {
		t.Fatalf("expected palm_strike targeting a2, got %+v", seen["palm_strike"])
	}
	if a, ok := seen["rally_cohort"]; !ok || a.TargetID != "coh1" {
		t.Fatalf("expected rally_cohort targeting coh1, got %+v", seen["rally_cohort"])
	}
	if _, ok := seen["forbidden_law"]; ok {
		t.Fatal("expected unauthorized WorldLawDelta action to be omitted from Options")
	}

	// Removing required venue feature or qi cost hides ward_venue.
	p.Self.StateSlots = nil
	for _, o := range agent.Options(p) {
		if o.Action.Kind == "ward_venue" {
			t.Fatal("expected ward_venue omitted when qi cost is unmet")
		}
	}
}

package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCohortResidentCanUsePublishedDistrictRouteAfterHydrationOrRestore(t *testing.T) {
	p, err := pack.LoadFile("../../packs/mortal_watershed.json")
	if err != nil {
		t.Fatal(err)
	}
	w, err := p.Instantiate("merchant-route")
	if err != nil {
		t.Fatal(err)
	}
	a, err := w.HydrateActor("cohort_pingyuan_merchants")
	if err != nil {
		t.Fatal(err)
	}
	if a.Location != "pingyuan_county" || a.VenueID != "pingyuan_pier" {
		t.Fatalf("hydrated merchant has no local departure point: %+v", a)
	}
	id := a.ID
	// A resident saved by the earlier engine knew the district but had no venue.
	a.VenueID = ""
	a.Items = map[string]int{"water": 2}
	a.Health, a.Hunger, a.Energy, a.Money = 40, 3, 100, 0
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	w, _, err = sim.ReplayFromCheckpoint(raw, nil, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	v := w.View(id)
	if v.Self.VenueID != "pingyuan_pier" || len(v.Roads) != 0 || len(v.VenueRoutes) == 0 {
		t.Fatalf("restored location and routes disagree: venue=%q, routes=%+v", v.Self.VenueID, v.VenueRoutes)
	}
	action, err := Offline().Decide(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	if action.Kind != "join_faction" && action.Kind != "join_profession" && action.Kind != "move_venue" {
		t.Fatalf("hungry resident did not act towards food: %+v", action)
	}
	for _, ev := range w.Apply(id, action) {
		if ev.Kind == "action_failed" {
			t.Fatalf("restored resident's normal decision failed: %+v", ev)
		}
	}
}

func TestPlanUsesOnlyPublishedFirstHops(t *testing.T) {
	w := hungryRiverWorld(t)
	v := w.View("magistrate_lin")
	v.Self.VenueID = ""
	v.VenueRoutes = nil
	if x, ok := stepTowardDistrict(v, "qingxi_village", 0); ok {
		t.Fatalf("venue map without a departure offered %+v", x)
	}
	v.Map = sim.WorldMap{Districts: []sim.District{{ID: "a"}, {ID: "b"}}, Roads: []sim.Road{{From: "a", To: "b", Ticks: 1}}}
	v.Self.Location = "a"
	v.Roads = nil
	if x, ok := stepTowardDistrict(v, "b", 0); ok {
		t.Fatalf("unpublished first hop was suggested: %+v", x)
	}
}

func TestRestoreUsesCurrentHouseVenueAndPreservesExactMembers(t *testing.T) {
	w := sim.World{Map: sim.WorldMap{Venues: []sim.Venue{{ID: "dock", DistrictID: "river"}, {ID: "hall", DistrictID: "river"}}}, Houses: []sim.House{{ID: "house", District: "river", VenueID: "hall"}}, Actors: []sim.Actor{{ID: "inside", Location: "river", InsideHouse: "house"}, {ID: "outside", Location: "river", HomeID: "house"}, {ID: "already", Location: "river", VenueID: "hall"}}, Cohorts: []sim.PopulationCohort{{ID: "group", Population: 1, Members: []sim.Actor{{ID: "member", Location: "river", InsideHouse: "house"}}}}}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := sim.DecodeWorldSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Actors[0].VenueID != "hall" || restored.Actors[1].VenueID != "dock" || restored.Actors[2].VenueID != "hall" || restored.Cohorts[0].Members[0].VenueID != "hall" {
		t.Fatalf("restore did not use known current location: %+v", restored)
	}
}

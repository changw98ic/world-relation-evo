package agent

import (
	"fmt"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func hasBoundaryKind(w *sim.World, actor, kind string) bool {
	for _, option := range Options(w.View(actor)) {
		if option.Action.Kind == kind {
			return true
		}
	}
	return false
}

func costlyActionWorld() *sim.World {
	w := boundaryWorld()
	w.Catalog.ActionSpecs = map[string]sim.ActionSpec{"work": {ID: "work", TargetKind: "self", GrantedByTags: []string{"qualified"}, EnergyCost: 5, MoneyCost: 3, ItemCosts: map[string]int{"material": 2}, SlotCosts: map[string]int{"focus": 4}, CurrencyCosts: map[string]int{"credit": 7}}}
	w.Actors[0].Tags = append(w.Actors[0].Tags, "qualified")
	w.Actors[0].Energy, w.Actors[0].Money = 5, 3
	w.Actors[0].Items = map[string]int{"material": 2}
	w.Actors[0].StateSlots = map[string]int{"focus": 4}
	w.Actors[0].Currencies = map[string]int64{"credit": 7}
	return w
}

func TestCandidateAndExecutionAgreeAtDeclaredCostBoundary(t *testing.T) {
	// These amounts are declared inputs, not thresholds inferred from the code.
	w := costlyActionWorld()
	action := boundaryOption(t, w, "alice", "work", "")
	boundaryApply(t, w, "alice", action)
	a := w.Actors[0]
	if a.Energy != 0 || a.Money != 0 || a.Items["material"] != 0 || a.StateSlots["focus"] != 0 || a.Currencies["credit"] != 0 {
		t.Fatal("exactly sufficient resources were rejected or charged incorrectly")
	}
	for _, tc := range []struct {
		name   string
		change func(*sim.Actor)
	}{
		{"energy", func(a *sim.Actor) { a.Energy-- }}, {"money", func(a *sim.Actor) { a.Money-- }},
		{"item", func(a *sim.Actor) { a.Items["material"]-- }}, {"slot", func(a *sim.Actor) { a.StateSlots["focus"]-- }},
		{"currency", func(a *sim.Actor) { a.Currencies["credit"]-- }}, {"permission", func(a *sim.Actor) { a.Tags = []string{"resident"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := costlyActionWorld()
			tc.change(&w.Actors[0])
			if hasBoundaryKind(w, "alice", "work") {
				t.Fatal("unfunded or ungranted action was offered")
			}
			before := w.Actors[0]
			before.Items = map[string]int{"material": w.Actors[0].Items["material"]}
			before.StateSlots = map[string]int{"focus": w.Actors[0].StateSlots["focus"]}
			before.Currencies = map[string]int64{"credit": w.Actors[0].Currencies["credit"]}
			boundaryRejected(t, w, "alice", action)
			after := w.Actors[0]
			if before.Energy != after.Energy || before.Money != after.Money || before.Items["material"] != after.Items["material"] || before.StateSlots["focus"] != after.StateSlots["focus"] || before.Currencies["credit"] != after.Currencies["credit"] {
				t.Fatal("rejected execution charged resources")
			}
		})
	}
}

func TestDeclarativeCandidateTargetsAreExecutableAtTheirDeclaredScope(t *testing.T) {
	for _, scope := range []string{"self", "actor", "venue", "district", "cohort"} {
		t.Run(scope, func(t *testing.T) {
			w := boundaryWorld()
			w.Map.Venues = []sim.Venue{{ID: "square", DistrictID: "meeting"}}
			for i := range w.Actors {
				w.Actors[i].VenueID = "square"
			}
			w.Cohorts = []sim.PopulationCohort{{ID: "group", District: "meeting", Population: 1, Adults: 1}}
			w.Catalog.ActionSpecs = map[string]sim.ActionSpec{"mark": {ID: "mark", TargetKind: scope, GrantedByTags: []string{"resident"}}}
			action := boundaryOption(t, w, "alice", "mark", "")
			switch scope {
			case "self":
				if action.TargetID != "" || action.Location != "" {
					t.Fatal("self target was redirected")
				}
			case "actor":
				if action.TargetID != "bob" && action.TargetID != "charlie" {
					t.Fatal("actor target is outside the perceived local people")
				}
			case "venue":
				if action.Location != "square" {
					t.Fatal("venue target was redirected")
				}
			case "district":
				if action.Location != "meeting" {
					t.Fatal("district target was redirected")
				}
			case "cohort":
				if action.TargetID != "group" {
					t.Fatal("cohort target was redirected")
				}
			}
			boundaryApply(t, w, "alice", action)
		})
	}
}

func TestWorldLawCandidateFilterAndResolverUseTheSamePermission(t *testing.T) {
	for _, disabled := range []int{0, 1} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			w := boundaryWorld()
			w.Actors[0].Tags = append(w.Actors[0].Tags, "firearm")
			w.Catalog.ActionSpecs = map[string]sim.ActionSpec{"shoot": {ID: "shoot", TargetKind: "actor", GrantedByTags: []string{"firearm"}, Attack: &sim.AttackSpec{Tier: 1, DamageByType: map[string]int{"physical": 1}}}}
			w.WorldLaws = map[string]int{"firearms_disabled": disabled}
			if got := hasBoundaryKind(w, "alice", "shoot"); got != (disabled == 0) {
				t.Fatal("world law and candidate permission differ")
			}
			action := sim.Action{Kind: "shoot", TargetID: "bob"}
			if disabled == 1 {
				boundaryRejected(t, w, "alice", action)
				if w.Actors[1].Health != 100 {
					t.Fatal("forbidden attack injured its target")
				}
			} else {
				boundaryApply(t, w, "alice", action)
			}
		})
	}
}

func TestHouseEntryCandidateRequiresActualAccessAndPlace(t *testing.T) {
	for _, tc := range []struct {
		name, owner, resident, district string
		allowed                         bool
	}{
		{"owner", "alice", "", "meeting", true}, {"resident", "bob", "alice", "meeting", true},
		{"stranger", "bob", "charlie", "meeting", false}, {"remote_owner", "alice", "", "remote", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := boundaryWorld()
			h := sim.House{ID: "home", District: tc.district, OwnerID: tc.owner}
			if tc.resident != "" {
				h.Residents = []string{tc.resident}
			}
			w.Houses = []sim.House{h}
			if hasBoundaryKind(w, "alice", "enter_house") != tc.allowed {
				t.Fatal("candidate used invented house access")
			}
			action := sim.Action{Kind: "enter_house", HouseID: "home"}
			if tc.allowed {
				boundaryApply(t, w, "alice", action)
				if w.Actors[0].InsideHouse != "home" {
					t.Fatal("valid entry did not enter the house")
				}
			} else {
				boundaryRejected(t, w, "alice", action)
				if w.Actors[0].InsideHouse != "" {
					t.Fatal("rejected entry changed location")
				}
			}
		})
	}
}

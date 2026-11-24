package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestLoanCandidateAndExecutionUseConfiguredTraderTag(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "borrow_loan")
	w.MarketCredit = sim.NewMarketCreditState()
	w.MarketCredit.Spec.TraderTag = "licensed_merchant"
	lender := &w.Actors[1]
	lender.Tags, lender.Money = []string{"resident", "trader"}, 27
	request := sim.Action{Kind: "borrow_loan", TargetID: lender.ID}
	events := w.Apply("alice", request)
	if len(events) != 1 || events[0].Kind != "action_failed" || !strings.Contains(fmt.Sprint(events[0].Payload["text"]), "configured trader tag") {
		t.Fatalf("lender without the configured tag: %+v", events)
	}
	lender.Tags = []string{"resident", "licensed_merchant"}
	request = boundaryOption(t, w, "alice", "borrow_loan", lender.ID)
	events = boundaryApply(t, w, "alice", request)
	if len(events) != 1 || events[0].Kind != "loan_requested" || w.MarketCredit.LoanRequests[0].Principal != 12 || lender.Money != 27 {
		t.Fatalf("expected a 12-coin request before any payment: %+v", events)
	}
}

func TestSeventhPartyMemberCanChooseAndJoin(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "join_party")
	w.Parties = []sim.Party{{ID: "expedition", LeaderID: "bob", ExpiresTick: w.Tick + 1, MemberIDs: []string{"bob"}}}
	for i := 0; i < 5; i++ {
		member := sim.CloneActor(w.Actors[1])
		member.ID = fmt.Sprintf("member%d", i)
		w.Actors = append(w.Actors, member)
		w.Parties[0].MemberIDs = append(w.Parties[0].MemberIDs, member.ID)
	}
	join := boundaryOption(t, w, "alice", "join_party", "expedition")
	events := boundaryApply(t, w, "alice", join)
	if len(events) != 1 || events[0].Kind != "party_joined" || len(w.Parties[0].MemberIDs) != 7 {
		t.Fatalf("seventh party member failed to join: %+v", events)
	}
}

func TestTransmissionCandidatesUseKnownPeopleAsRecipients(t *testing.T) {
	w := boundaryWorld()
	a := &w.Actors[0]
	w.Actors[1].Location = "remote"
	w.Map.GeoRegions = []sim.GeoRegion{{ID: "near"}, {ID: "far", Coord: sim.GeoCoord{X: 5}}}
	w.Map.Districts[0].GeoRegionID, w.Map.Districts[1].GeoRegionID = "near", "far"
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "transmit_message")
	w.Catalog.CommChannels = map[string]sim.CommChannelSpec{
		"letter": {ID: "letter", RangeKm: 5, EnergyCost: 4},
		"signal": {ID: "signal", RangeKm: 5, Broadcast: true},
	}
	a.Memories = []sim.Memory{
		{EventID: 1, Source: a.ID, TargetID: "cultivation", Kind: "skill_leveled"},
		{EventID: 2, Source: a.ID, TargetID: "jade_pill_pavilion", Kind: "faction_joined"},
		{EventID: 3, Source: a.ID, TargetID: "bob", Kind: "speech", Text: "Hello Bob"},
	}
	view := w.View(a.ID)
	var sawLetter, sawBroadcast bool
	for _, option := range Options(view) {
		x := option.Action
		if x.Kind != "transmit_message" {
			continue
		}
		switch {
		case x.Channel == "letter" && x.TargetID == "bob":
			sawLetter = true
			events := boundaryApply(t, w, a.ID, x)
			if len(events) == 0 || events[0].Kind != "channel_transmitted" || events[0].Payload["delivered_count"] != 1 {
				t.Fatalf("letter was not delivered at the channel's range: %+v", events)
			}
		case x.Channel == "signal" && x.TargetID == "":
			sawBroadcast = true
		default:
			t.Fatalf("unexpected transmission candidate %+v", x)
		}
	}
	if !sawLetter || !sawBroadcast {
		t.Fatalf("expected a letter to Bob and an untargeted broadcast, got letter=%t broadcast=%t", sawLetter, sawBroadcast)
	}
	for _, id := range []string{"cultivation", "jade_pill_pavilion"} {
		x := sim.Action{Kind: "transmit_message", Channel: "letter", TargetID: id, Text: "Hello"}
		if view.CheckAction(x).Allowed || w.CheckAction(a.ID, x).Allowed {
			t.Fatalf("%s was accepted as a recipient", id)
		}
	}
}

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

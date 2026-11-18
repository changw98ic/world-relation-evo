package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestActivePersonalPlanUnlocksSubThresholdOfferAndExposesJevDirective(t *testing.T) {
	w := boundaryWorld()
	spec := w.Catalog.Items["plank"]
	spec.BatchQty = 2
	spec.OfferThreshold = 6
	w.Catalog.Items["plank"] = spec

	alice := &w.Actors[0]
	alice.Professions = map[string]int{"carpenter": 1}
	alice.Items["plank"] = 2 // below OfferThreshold (6), equal to BatchQty (2)

	// Without an active plan, 2 planks (< OfferThreshold 6) is not offered as surplus.
	for _, opt := range Options(w.View("alice")) {
		if opt.Action.Kind == "offer" && opt.Action.Item == "plank" {
			t.Fatalf("unexpected surplus offer below OfferThreshold without active plan: %+v", opt)
		}
	}

	// Start plan with 2 pre-existing planks: enters "sell" stage and unlocks offer candidate.
	boundaryApply(t, w, "alice", sim.Action{Kind: "start_plan", RecipeID: "saw_planks"})
	view := w.View("alice")
	if view.Self.Plan == nil || view.Self.Plan.Stage != "sell" {
		t.Fatalf("expected active plan in sell stage, got %+v", view.Self.Plan)
	}
	opts := Options(view)
	offerID := ""
	for _, opt := range opts {
		if opt.Action.Kind == "offer" && opt.Action.Item == "plank" && opt.Action.TargetID == "bob" {
			offerID = opt.ID
			if !strings.Contains(opt.Label, "[active plan sale") {
				t.Fatalf("expected active plan marker on offer label, got %q", opt.Label)
			}
			break
		}
	}
	if offerID == "" {
		t.Fatalf("expected active plan in sell stage to generate offer option below OfferThreshold")
	}

	// Verify Jev receives state.active_plan, directive instructions, and marked criteria.
	var capturedState sim.Perception
	var capturedInstructions string
	var capturedCriteria map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var payload struct {
			State     sim.Perception `json:"state"`
			Questions map[string]struct {
				Instructions string            `json:"instructions"`
				Criteria     map[string]string `json:"criteria"`
			} `json:"questions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			rw.WriteHeader(400)
			return
		}
		capturedState = payload.State
		capturedInstructions = payload.Questions["decision"].Instructions
		capturedCriteria = payload.Questions["decision"].Criteria
		_ = json.NewEncoder(rw).Encode(map[string]any{
			"answers": map[string]any{"decision": map[string]string{"type": "choice", "choice": offerID}},
		})
	}))
	defer server.Close()

	chosen, err := (Jev{Endpoint: server.URL, APIKey: "fixture", Client: server.Client()}).Select(context.Background(), view, opts)
	if err != nil || chosen != offerID {
		t.Fatalf("Jev.Select failed: chosen=%q err=%v", chosen, err)
	}
	if capturedState.ActivePlan == nil || capturedState.ActivePlan.Stage != "sell" || capturedState.ActivePlan.RemainingToSell != 2 {
		t.Fatalf("Jev state missing active_plan directive: %+v", capturedState.ActivePlan)
	}
	if !strings.Contains(capturedInstructions, "Active plan directive: stage=sell item=plank") {
		t.Fatalf("Jev instructions missing active plan summary: %q", capturedInstructions)
	}
	if !strings.Contains(capturedCriteria[offerID], "[active plan sale") {
		t.Fatalf("Jev criteria missing active plan marker: %q", capturedCriteria[offerID])
	}
}

func TestActivePersonalPlanDrivesVenueTravelProductionAndMarketSaleToCompletion(t *testing.T) {
	cat := sim.DefaultCatalog()
	plank := cat.Items["plank"]
	plank.BatchQty = 2
	plank.OfferThreshold = 8 // higher than plan quantity so only active-plan logic sells it
	cat.Items["plank"] = plank
	cat.TagProfiles["consumer_tag"] = sim.TagProfile{Consumes: []string{"plank"}, TargetStock: map[string]int{"plank": 4}}
	cat.TagActions["consumer_tag"] = []string{"wait", "accept", "reject", "accept_agreement", "reject_agreement", "eat", "drink"}

	w := &sim.World{
		ID:      "plan-venue-drive",
		Tick:    8, // daytime 08:00
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			Districts: []sim.District{
				{ID: "woods", Name: "Woods", Features: []string{"woodland"}, ResourceCapacities: map[string]int64{"timber_stand": 500}},
				{ID: "workshop_ward", Name: "Workshop Ward"},
				{ID: "bazaar", Name: "Bazaar", Features: []string{"market"}},
			},
			Venues: []sim.Venue{
				{ID: "v_woods", Name: "Timber Slope", DistrictID: "woods", Features: []string{"woodland"}},
				{ID: "v_yard", Name: "Joinery Courtyard", DistrictID: "workshop_ward"},
				{ID: "v_market", Name: "Central Market", DistrictID: "bazaar", Features: []string{"market"}},
			},
			VenueRoutes: []sim.VenueRoute{
				{FromVenue: "v_yard", ToVenue: "v_woods", Ticks: 1, EnergyCost: 2, Medium: "road"},
				{FromVenue: "v_woods", ToVenue: "v_yard", Ticks: 1, EnergyCost: 2, Medium: "road"},
				{FromVenue: "v_yard", ToVenue: "v_market", Ticks: 1, EnergyCost: 2, Medium: "road"},
				{FromVenue: "v_market", ToVenue: "v_yard", Ticks: 1, EnergyCost: 2, Medium: "road"},
			},
		},
		DistrictStates: map[string]sim.DistrictState{
			"woods":         {Weather: "clear", Resources: map[string]int64{"timber_stand": 500}, MaxResources: map[string]int64{"timber_stand": 500}},
			"workshop_ward": {Weather: "clear"},
			"bazaar":        {Weather: "clear"},
		},
		Houses: []sim.House{
			{ID: "h_joinery", Name: "Joinery Shop", District: "workshop_ward", VenueID: "v_yard", OwnerID: "artisan", Residents: []string{"artisan"}, Level: 2, Comfort: 3, Condition: 95, Facilities: []string{"carpentry_bench"}},
		},
		Actors: []sim.Actor{
			{
				ID: "artisan", Name: "Artisan", Role: "carpenter",
				Location: "workshop_ward", VenueID: "v_yard", InsideHouse: "h_joinery", HomeID: "h_joinery",
				Tags:        []string{"resident", "trader"},
				Professions: map[string]int{"woodcutter": 1, "carpenter": 1},
				Health:      100, Energy: 95, Money: 10,
				Items: map[string]int{"food": 10, "water": 6},
				Trust: map[string]int{"buyer": 2},
			},
			{
				ID: "buyer", Name: "Buyer", Role: "resident",
				Location: "bazaar", VenueID: "v_market",
				Tags:   []string{"resident", "trader", "consumer_tag"},
				Health: 100, Energy: 95, Money: 40,
				Items: map[string]int{"food": 10, "water": 6},
				Trust: map[string]int{"artisan": 2},
			},
		},
	}

	boundaryApply(t, w, "artisan", sim.Action{Kind: "start_plan", RecipeID: "saw_planks"})
	if w.Actors[0].Plan == nil || w.Actors[0].Plan.Stage != "produce" {
		t.Fatalf("expected artisan plan to start in produce stage, got %+v", w.Actors[0].Plan)
	}

	decider := Offline()
	visitedWoods := false
	craftedPlank := false
	visitedMarket := false

	for step := 0; step < 30; step++ {
		evs, err := w.Step(context.Background(), decider)
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		if w.Actors[0].VenueID == "v_woods" {
			visitedWoods = true
		}
		if w.Actors[0].VenueID == "v_market" {
			visitedMarket = true
		}
		for _, ev := range evs {
			if ev.ActorID == "artisan" && (ev.Kind == "crafted" || ev.Kind == "harvested") && ev.Action.RecipeID == "saw_planks" {
				craftedPlank = true
			}
		}
		if w.Actors[0].Plan.Status == "completed" {
			break
		}
	}

	plan := w.Actors[0].Plan
	if !visitedWoods || !craftedPlank || !visitedMarket || plan.Status != "completed" || plan.Sold < plan.Quantity {
		t.Fatalf("active plan did not drive full venue travel -> input harvest -> workshop craft -> market sale: visitedWoods=%v craftedPlank=%v visitedMarket=%v plan=%+v buyerItems=%v",
			visitedWoods, craftedPlank, visitedMarket, plan, w.Actors[1].Items)
	}
	if w.Actors[1].Items["plank"] < plan.Quantity {
		t.Fatalf("buyer did not receive planned planks: %+v", w.Actors[1].Items)
	}
}

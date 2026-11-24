package sim

import (
	"encoding/json"
	"testing"
)

func TestPlanningRulesPreserveZeroAndFullProbability(t *testing.T) {
	for _, fraction := range []float64{0, 1} {
		rules := PlanningRules{HorizonTicks: 1, OfferAcceptancePrior: fraction, Needs: []NeedRule{
			{NeedKey: NeedKey{Measure: "nutrition"}, Scale: 1, Plasticity: fraction},
		}}
		if err := rules.Validate(); err != nil {
			t.Fatalf("valid probability, plasticity and weight rejected: %v", err)
		}
	}
}

func TestRoadPredictionUsesObservedWeather(t *testing.T) {
	w := &World{
		Map: WorldMap{
			Districts: []District{{ID: "home"}, {ID: "away"}},
			Roads:     []Road{{From: "home", To: "away", Ticks: 1, EnergyCost: 2}},
		},
		Catalog: Catalog{Weathers: map[string]WeatherSpec{"storm": {TravelTickPenalty: 3}}},
		Actors: []Actor{{ID: "walker", Location: "home", Energy: 100, Health: 100, KnownFacts: map[string]KnownFact{
			"weather:away": {Subject: "away", Attribute: "weather", Value: "clear", Source: "local_observation"},
		}}},
		DistrictStates: map[string]DistrictState{"away": {Weather: "storm"}},
	}
	perceived, ok := w.View("walker").PlanRoute("away", "")
	if !ok || perceived.Ticks != 1 {
		t.Fatalf("unobserved storm leaked into route: %+v", perceived)
	}
	w.Actors[0].KnownFacts["weather:away"] = KnownFact{Subject: "away", Attribute: "weather", Value: "storm", Source: "local_observation"}
	perceived, ok = w.View("walker").PlanRoute("away", "")
	if !ok || perceived.Ticks != 4 {
		t.Fatalf("observed storm did not affect route: %+v", perceived)
	}
}

func TestAttemptToUseSeveredRouteUpdatesOnlyTheTravelersKnowledge(t *testing.T) {
	w := &World{NextID: 1, Catalog: DefaultCatalog(),
		Map: WorldMap{
			Districts: []District{{ID: "home"}, {ID: "away"}},
			Roads:     []Road{{From: "home", To: "away", Medium: "road", Ticks: 1, EnergyCost: 2}},
		},
		Actors: []Actor{
			{ID: "traveler", Tags: []string{"resident"}, Location: "home", Health: 100, Energy: 100},
			{ID: "bystander", Tags: []string{"resident"}, Location: "home", Health: 100, Energy: 100},
		},
		ActiveWorldEvents: []ActiveWorldEvent{{InstanceID: "landslide", DistrictID: "away", SeveredMedium: "road", ExpiresTick: 4}},
	}
	x := Action{Kind: "move", Location: "away"}
	if !w.View("traveler").CheckAction(x).Allowed {
		t.Fatal("undiscovered obstruction was visible to the traveler")
	}
	events := w.Apply("traveler", x)
	if len(events) != 1 || events[0].Kind != "action_failed" || w.Actors[0].Location != "home" {
		t.Fatalf("expected the attempted move to fail: %+v", events)
	}
	if _, ok := w.View("traveler").PlanRoute("away", ""); ok {
		t.Fatal("discovered obstruction did not affect the traveler's plan")
	}
	if !w.View("bystander").CheckAction(x).Allowed {
		t.Fatal("a bystander learned about the route without an observation")
	}
	w.Tick = 4
	if _, ok := w.View("traveler").PlanRoute("away", ""); !ok {
		t.Fatal("the traveler still believes the timed obstruction is active")
	}
}

func TestDelayedMessageArrivesOnlyAfterItsSavedDueTick(t *testing.T) {
	w := Village("due-message")
	w.Actors = w.Actors[:2]
	for i := range w.Actors {
		w.Actors[i].Location, w.Actors[i].InsideHouse = "crossroads", ""
		w.Actors[i].Tags = append(w.Actors[i].Tags, "radio_user")
	}
	w.Catalog.TagActions["radio_user"] = []string{"transmit_message"}
	w.Catalog.CommChannels = map[string]CommChannelSpec{
		"radio": {ID: "radio", GrantedByTags: []string{"radio_user"}, DelayTicks: 3},
	}
	first, second := w.Actors[0].ID, w.Actors[1].ID
	events := w.Apply(first, Action{Kind: "transmit_message", TargetID: second, Channel: "radio", Text: "Water here",
		Statement: &Statement{Topic: "supply", Item: "water"}})
	if len(events) != 1 || events[0].Kind != "channel_transmitted" ||
		len(w.PendingMessages) != 1 || len(w.Actors[1].Memories) != 0 {
		t.Fatalf("unexpected state before arrival: events=%+v pending=%+v", events, w.PendingMessages)
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	w, err = DecodeWorldSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	w.Tick = 2
	if arrived := w.deliverScheduledMessages(); len(arrived) != 0 || len(w.Actors[1].Memories) != 0 {
		t.Fatal("message arrived before the scheduled time")
	}
	w.Tick = 3
	arrived := w.deliverScheduledMessages()
	received := w.Actors[1].Memories
	if len(arrived) != 1 || arrived[0].Kind != "channel_delivered" || len(received) != 1 ||
		received[0].EventID != arrived[0].ID || received[0].Observation != nil || received[0].Statement == nil {
		t.Fatalf("message evidence missing after arrival: %+v %+v", arrived, received)
	}
	if len(w.deliverScheduledMessages()) != 0 {
		t.Fatal("the same delivery was applied twice")
	}
}

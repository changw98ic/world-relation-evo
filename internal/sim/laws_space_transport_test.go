package sim

import "testing"

// S1: a person cannot act at both road endpoints while travelling, and cannot
// skip an intermediate district. The road's declared time and energy are inputs.
func TestLawSpaceTravelHasTimeCostAndNoEndpointPresence(t *testing.T) {
	w := lawWorld()
	w.Map.Roads = []Road{{From: "home", To: "away", Ticks: 3, EnergyCost: 6}, {From: "away", To: "beyond", Ticks: 2, EnergyCost: 4}}
	w.Actors[2].Location = "away"
	lawRejected(t, w.Apply("a", Action{Kind: "move", Location: "beyond"}))
	if w.Actors[0].Energy != 100 || w.Actors[0].Location != "home" {
		t.Fatal("nonadjacent movement charged or teleported the traveler")
	}
	lawOK(t, w.Apply("a", Action{Kind: "move", Location: "away"}))
	if w.Actors[0].Energy != 94 || !w.Actors[0].InTransit() || w.Actors[0].Location != "home" {
		t.Fatal("departure did not pay the declared road cost and begin transit")
	}
	for _, observer := range []string{"b", "c"} {
		for _, nearby := range w.View(observer).Nearby {
			if nearby.ID == "a" {
				t.Fatal("traveler was perceived at a stationary road endpoint")
			}
		}
		lawRejected(t, w.Apply(observer, Action{Kind: "speak", TargetID: "a", Text: "Are you here?"}))
	}
	w.Tick++
	w.Advance("a")
	if !w.Actors[0].InTransit() || w.Actors[0].Location != "home" {
		t.Fatal("traveler arrived after only two of three travel ticks")
	}
	w.Tick++
	w.Advance("a")
	if w.Actors[0].InTransit() || w.Actors[0].Location != "away" || w.Actors[0].Energy != 94 {
		t.Fatal("arrival failed or charged road energy twice")
	}
	lawOK(t, w.Apply("c", Action{Kind: "speak", TargetID: "a", Text: "You have arrived."}))
	lawRejected(t, w.Apply("b", Action{Kind: "speak", TargetID: "a", Text: "Still at home?"}))
}

func TestLawSpaceHouseAndVenueBoundariesIsolateSpeech(t *testing.T) {
	for _, boundary := range []string{"house", "venue"} {
		w := lawWorld()
		if boundary == "house" {
			w.Actors[0].InsideHouse = "room"
		} else {
			w.Actors[0].VenueID = "square"
		}
		lawRejected(t, w.Apply("b", Action{Kind: "speak", TargetID: "a", Text: "private words"}))
		if len(w.Actors[0].Memories) != 0 {
			t.Fatal("speech crossed a closed co-location boundary")
		}
		w.Actors[1].InsideHouse, w.Actors[1].VenueID = w.Actors[0].InsideHouse, w.Actors[0].VenueID
		lawOK(t, w.Apply("b", Action{Kind: "speak", TargetID: "a", Text: "private words"}))
		if len(w.Actors[0].Memories) != 1 || len(w.Actors[2].Memories) != 0 {
			t.Fatal("co-occupant missed speech or outsider received it")
		}
	}
}

// S2: preparation is not a guaranteed hit on a target that has left. The funded
// successful control proves that the rejection case is not merely a disabled action.
func TestLawSpaceDelayedEffectRechecksPhysicalTarget(t *testing.T) {
	for _, depart := range []bool{false, true} {
		w := lawWorld()
		w.Catalog.ActionSpecs = map[string]ActionSpec{"strike": {ID: "strike", GrantedByTags: []string{"person"}, TargetKind: "actor", Ticks: 3, EnergyCost: 2, ItemCosts: map[string]int{"wood": 1}, Attack: &AttackSpec{DamageByType: map[string]int{"physical": 10}}}}
		w.Actors[0].Items["wood"] = 1
		lawOK(t, w.Apply("a", Action{Kind: "strike", TargetID: "b"}))
		if w.Actors[0].Items["wood"] != 0 || w.Actors[0].Energy != 98 || w.Actors[1].Health != 100 {
			t.Fatal("delayed action applied damage before paying costs and doing work")
		}
		w.Tick++
		w.Advance("a")
		if w.Actors[1].Health != 100 {
			t.Fatal("delayed damage arrived before the third work tick")
		}
		if depart {
			w.Actors[1].Location = "away"
		}
		w.Tick++
		events := w.Advance("a")
		wantHealth := 90
		if depart {
			wantHealth = 100
			lawRejected(t, events)
		} else {
			lawOK(t, events)
		}
		if w.Actors[1].Health != wantHealth || w.Actors[0].Items["wood"] != 0 || w.Actors[0].Energy != 98 {
			t.Fatal("delayed target settlement violated locality or charged preparation twice")
		}
	}
}

func lawFreightWorld() *World {
	w := lawWorld()
	w.Map.Roads = []Road{{From: "home", To: "away", Ticks: 3, FreightCapacity: 2, Medium: "land"}}
	w.Cohorts = []PopulationCohort{{ID: "owner", District: "away", Population: 1, Adults: 1, ItemPool: map[string]int64{}, MoneyPool: 7}}
	w.Growth = []GrowthBatch{{ID: 30, OwnerID: "owner", Cohort: true, District: "home", Item: "grain", Quantity: 3, Batches: 1, Ready: true, Harvested: true}}
	return w
}

// T1: stored harvest, cargo in transit, and delivered inventory are three
// representations of the same goods. Capacity governs dispatch; time governs arrival.
func TestLawTransportHarvestDeliveryKeepsCargoAndCapacity(t *testing.T) {
	w := lawFreightWorld()
	if _, err := w.DispatchHarvest(30); err != nil {
		t.Fatal(err)
	}
	if len(w.Shipments) != 1 || w.Shipments[0].Quantity != 2 || len(w.Growth) != 1 || w.Growth[0].Quantity != 1 || w.Cohorts[0].ItemPool["grain"] != 0 {
		t.Fatal("dispatch exceeded two-unit capacity or delivered cargo immediately")
	}
	if _, err := w.DispatchHarvest(30); err == nil {
		t.Fatal("second same-tick dispatch spent the same route capacity")
	}
	if w.Growth[0].Quantity+w.Shipments[0].Quantity != 3 || w.Cohorts[0].MoneyPool != 7 {
		t.Fatal("blocked dispatch lost goods or charged a second purchase")
	}
	w = lawRestore(t, w)
	if _, err := w.DispatchHarvest(30); err == nil {
		t.Fatal("saving and loading reset already spent freight capacity")
	}
	w.Tick = 2
	w.settleShipments()
	if w.Cohorts[0].ItemPool["grain"] != 0 || len(w.Shipments) != 1 {
		t.Fatal("three-tick shipment arrived early")
	}
	w.Tick = 3
	w.settleShipments()
	w.settleShipments()
	if w.Cohorts[0].ItemPool["grain"] != 2 || len(w.Shipments) != 0 || w.Growth[0].Quantity != 1 {
		t.Fatal("arrival did not transfer cargo exactly once")
	}
	if _, err := w.DispatchHarvest(30); err != nil {
		t.Fatal(err)
	}
	w.Tick = 6
	w.settleShipments()
	if w.Cohorts[0].ItemPool["grain"] != 3 || len(w.Growth) != 0 || len(w.Shipments) != 0 || w.Cohorts[0].MoneyPool != 7 {
		t.Fatal("second capacity window failed to deliver the remaining owned cargo")
	}
}

func TestLawTransportDisconnectedRouteKeepsGoodsAtOrigin(t *testing.T) {
	w := lawFreightWorld()
	w.Map.Roads = nil
	if _, err := w.DispatchHarvest(30); err == nil {
		t.Fatal("disconnected stored harvest entered transit")
	}
	if len(w.Growth) != 1 || w.Growth[0].Quantity != 3 || len(w.Shipments) != 0 || w.Cohorts[0].ItemPool["grain"] != 0 {
		t.Fatal("unreachable destination received cargo or origin lost it")
	}
}

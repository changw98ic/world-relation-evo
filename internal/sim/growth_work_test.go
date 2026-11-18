package sim

import "testing"

func TestHarvestRosterAndDelayedPropertyDelivery(t *testing.T) {
	w := lawFreightWorld()
	g := &w.Growth[0]
	g.Harvested = false
	g.Rule.CareWorkersPerBatch = 3
	g.Quantity = 5
	w.Cohorts = append(w.Cohorts, PopulationCohort{ID: "crew", District: "home", Population: 2, Residents: []ResidentBand{{Origin: "west", First: 7, Count: 1}, {Origin: "east", First: 3, Count: 1}}})
	terms := GrowthCareAgreement{GrowthID: g.ID, OwnerKind: "cohort", OwnerID: "owner", CaretakerKind: "cohort", CaretakerID: "crew", SupplierKind: "cohort", SupplierID: "crew", Harvest: true, SharePermille: 400, OwnerAccepted: true, CaretakerAccepted: true}
	if err := w.SetGrowthCare(terms); err != nil {
		t.Fatal(err)
	}
	first := w.AdvanceGrowth()
	if len(first) < 2 || first[0].Kind != "growth_work_completed" || first[0].Payload["stage_complete"] != false || w.Growth[0].HarvestWork != 2 {
		t.Fatalf("partial harvest: %+v", first)
	}
	runs := first[0].Payload["workers"].([]WorkerRun)
	if len(runs) != 2 || runs[0].FirstID != "west_a0007" || runs[1].FirstID != "east_a0003" {
		t.Fatalf("wrong workers: %+v", runs)
	}
	if again := w.AdvanceGrowth(); len(again) != 0 {
		t.Fatalf("workers reused: %+v", again)
	}
	w = lawRestore(t, w)
	w.Tick++
	events := w.AdvanceGrowth()
	entitled := map[string]int64{}
	delivered := map[string]int64{}
	record := func(events []Event) {
		for _, e := range events {
			switch e.Kind {
			case "harvest_entitled", "harvest_delivered":
				if e.Payload["source_growth_id"] != int64(30) {
					t.Fatalf("lost crop: %+v", e)
				}
				recipient := e.Payload["recipient_id"].(string)
				basis := "property"
				if recipient == "crew" {
					basis = "care_agreement"
				}
				if e.Payload["basis"] != basis {
					t.Fatal(e)
				}
				totals := entitled
				if e.Kind == "harvest_delivered" {
					totals = delivered
				}
				totals[recipient] += e.Payload["quantity"].(int64)
			}
		}
	}
	record(events)
	if entitled["owner"] != 3 || entitled["crew"] != 2 || delivered["owner"] != 0 || delivered["crew"] != 2 || w.Cohorts[1].ItemPool["grain"] != 2 || len(w.Shipments) != 1 {
		t.Fatalf("harvest distribution: %+v", events)
	}
	shipment := w.Shipments[0]
	w.Tick = shipment.ArrivalTick - 1
	record(w.settleShipments())
	if delivered["owner"] != 0 {
		t.Fatal("credited before arrival")
	}
	w.Tick++
	arrived := w.settleShipments()
	record(arrived)
	if delivered["owner"] != shipment.Quantity || arrived[len(arrived)-1].Payload["shipment_id"] != shipment.ID || w.Cohorts[0].ItemPool["grain"] != shipment.Quantity {
		t.Fatal(arrived)
	}
}

func TestAbsentCaretakerCollectsSignedShareOnce(t *testing.T) {
	w := lawWorld()
	g := GrowthBatch{ID: 30, OwnerID: "a", District: "home", Item: "grain", Quantity: 5, Batches: 1, Ready: true}
	w.Growth = []GrowthBatch{g}
	terms := GrowthCareAgreement{GrowthID: g.ID, OwnerKind: "actor", OwnerID: "a", CaretakerKind: "actor", CaretakerID: "b", SupplierKind: "actor", SupplierID: "b", Harvest: true, SharePermille: 400, OwnerAccepted: true, CaretakerAccepted: true}
	if err := w.SetGrowthCare(terms); err != nil {
		t.Fatal(err)
	}
	w.actor("b").Location = "away"
	before := w.actor("b").Items["grain"]
	events := w.harvestGrowth(0, g)
	if w.actor("b").Items["grain"] != before || len(w.Growth) != 1 || w.Growth[0].Quantity != 2 || w.Growth[0].SourceGrowthID != 30 {
		t.Fatal(events)
	}
	var earned int64
	for _, e := range events {
		if e.Payload["recipient_id"] == "b" {
			if e.Kind != "harvest_entitled" {
				t.Fatal(e)
			}
			earned += e.Payload["quantity"].(int64)
		}
	}
	if earned != 2 {
		t.Fatalf("caretaker entitlement %d", earned)
	}
	w = lawRestore(t, w)
	w.Tick++
	w.actor("b").Location = "home"
	events = w.harvestGrowth(0, w.Growth[0])
	var delivered int64
	for _, e := range events {
		if e.Kind == "harvest_entitled" {
			t.Fatal("duplicate claim", e)
		}
		if e.Kind == "harvest_delivered" {
			if e.Payload["source_growth_id"] != int64(30) || e.Payload["basis"] != "care_agreement" || e.Payload["recipient_id"] != "b" {
				t.Fatal(e)
			}
			delivered += e.Payload["quantity"].(int64)
		}
	}
	if delivered != 2 || w.actor("b").Items["grain"] != before+2 || len(w.Growth) != 0 {
		t.Fatal(events)
	}
}

func TestVolunteerHarvestCreatesNoWorkerFoodClaim(t *testing.T) {
	w := lawFreightWorld()
	g := &w.Growth[0]
	g.Harvested = false
	w.Cohorts = append(w.Cohorts, PopulationCohort{ID: "crew", District: "home", Population: 1})
	if err := w.SetGrowthCare(GrowthCareAgreement{GrowthID: g.ID, OwnerKind: "cohort", OwnerID: "owner", CaretakerKind: "cohort", CaretakerID: "crew", SupplierKind: "cohort", SupplierID: "crew", Harvest: true, OwnerAccepted: true, CaretakerAccepted: true}); err != nil {
		t.Fatal(err)
	}
	events := w.AdvanceGrowth()
	var owner int64
	for _, e := range events {
		if e.Kind == "harvest_entitled" {
			if e.Payload["recipient_id"] != "owner" {
				t.Fatal(e)
			}
			owner += e.Payload["quantity"].(int64)
		}
	}
	if owner != 3 || w.Cohorts[1].ItemPool["grain"] != 0 {
		t.Fatal(events)
	}
}

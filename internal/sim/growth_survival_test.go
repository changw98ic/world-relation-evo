package sim

import "testing"

func TestPlantSurvivalClockContinuesDuringPowerLossAndSnapshotReload(t *testing.T) {
	for _, powered := range []bool{false, true} {
		w := lawWorld()
		crop := lawCrop()
		crop.Growth.FailureAfterTicks = 2
		w.Catalog.Recipes = []Recipe{crop}
		w.Map.Venues = []Venue{{ID: "greenhouse", DistrictID: "home"}}
		w.Actors[0].VenueID = "greenhouse"
		w.Actors[0].Items["water"] = 2
		w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "support", Kind: "consumer", VenueID: "greenhouse", Active: true, Powered: powered}}}
		w.Growth = []GrowthBatch{{ID: 50, OwnerID: "a", District: "home", VenueID: "greenhouse", RecipeID: crop.ID, Rule: *crop.Growth, Batches: 1, Item: "grain", Quantity: 3}}
		w.InitLedgerAudit()
		w.Tick = 1
		w.AdvanceGrowth()
		w = lawRestore(t, w)
		w.Tick = 2
		events := w.AdvanceGrowth()
		if !w.Growth[0].Ready || powered && w.Growth[0].Quantity != 3 || !powered && w.Growth[0].Quantity != 0 {
			t.Fatalf("powered=%v: biological survival was frozen behind a production pause: %+v", powered, w.Growth[0])
		}
		want := "growth_failed"
		if powered {
			want = "growth_matured"
		}
		found := false
		for _, event := range events {
			found = found || event.Kind == want
		}
		if !found {
			t.Fatalf("powered=%v: a failed crop was counted as a harvest, or an actual harvest was lost: %+v", powered, events)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPowerLossPreservesNonlivingWorkWithoutBiologicalMortality(t *testing.T) {
	w := lawWorld()
	w.Map.Venues = []Venue{{ID: "workshop", DistrictID: "home"}}
	w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "support", Kind: "consumer", VenueID: "workshop", Active: true}}}
	w.Growth = []GrowthBatch{{ID: 50, Cohort: true, OwnerID: "workers", District: "home", VenueID: "workshop", RecipeID: "board", Rule: GrowthRule{DurationTicks: 3, MaintenanceIntervalTicks: 3}, Batches: 1, Item: "board", Quantity: 1}}
	w.Tick = 100
	w.AdvanceGrowth()
	if w.Growth[0].Ready || w.Growth[0].Quantity != 1 || w.Growth[0].ProgressTicks != 0 {
		t.Fatal("power loss destroyed or completed unfinished nonliving work")
	}
}

func TestPlantCanRecoverWhenServiceReturnsBeforeItsDeclaredDeadline(t *testing.T) {
	w := lawWorld()
	crop := lawCrop()
	crop.Growth.FailureAfterTicks = 2
	w.Catalog.Recipes = []Recipe{crop}
	w.Map.Venues = []Venue{{ID: "greenhouse", DistrictID: "home"}}
	w.Actors[0].VenueID = "greenhouse"
	w.Actors[0].Items["water"] = 2
	w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "support", Kind: "consumer", VenueID: "greenhouse", Active: true}}}
	w.Growth = []GrowthBatch{{ID: 50, OwnerID: "a", District: "home", VenueID: "greenhouse", RecipeID: crop.ID, Rule: *crop.Growth, Batches: 1, Item: "grain", Quantity: 3}}
	w.Tick = 1
	w.AdvanceGrowth()
	w = lawRestore(t, w)
	w.FlowGrid.Nodes[0].Powered = true
	for tick := int64(2); tick <= 3; tick++ {
		w.Tick = tick
		w.AdvanceGrowth()
	}
	if !w.Growth[0].Ready || w.Growth[0].Quantity != 3 || w.Growth[0].FailedTicks != 0 || w.Actors[0].Items["water"] != 0 {
		t.Fatal("an outage shorter than the declared survival period killed the plant or lost funded care")
	}
}

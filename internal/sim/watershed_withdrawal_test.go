package sim

import "testing"

func TestLawEcologyWithdrawalReducesNextDownstreamDelivery(t *testing.T) {
	for _, test := range []struct{ withdraw, downstream int64 }{{0, 10}, {960, 5}, {1920, 0}} {
		w := lawWatershed()
		w.Map.GeoFlows = []GeoFlowEdge{{FromRegion: "up", ToRegion: "down", Kind: "river", ConductancePermille: 500}}
		w.DistrictStates["up"].Resources["freshwater"] = 1920
		w.drawDistrictResource("up", "freshwater", test.withdraw)
		w.InitLedgerAudit()
		w.routeEcologicalWater([]string{"up", "down"})
		if got := w.DistrictStates["down"].Resources["freshwater"]; got != test.downstream {
			t.Fatalf("withdrawal %d delivered %d, want %d from 500/1000 per day over 96 ticks", test.withdraw, got, test.downstream)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLawEcologyFloodRequiresActualConnectedOverflow(t *testing.T) {
	for _, connected := range []bool{false, true} {
		w := lawWatershed()
		w.Actors = nil
		w.Map.GeoFlows = nil
		if connected {
			w.Map.GeoFlows = []GeoFlowEdge{{FromRegion: "up", ToRegion: "down", Kind: "river", ConductancePermille: 1000}}
		}
		w.DistrictStates["up"].Resources["freshwater"] = 1920
		w.DistrictStates["down"].MaxResources["freshwater"] = 1
		w.Catalog.Disasters = []DisasterSpec{{ID: "flood", FlowKind: "river", ProbPermillePerDay: 1000, DurationTicks: TicksPerDay}}
		w.Tick = 4 // 07:00 disaster evaluation, after this tick's water movement
		w.stepWorldEnvironment()
		if got := len(w.DistrictStates["down"].ActiveDisasters); (got == 1) != connected {
			t.Fatalf("connected=%v: floods=%d overflow=%v", connected, got, w.DistrictStates["down"].WaterOverflow)
		}
	}
}

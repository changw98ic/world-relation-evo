package sim

import (
	"reflect"
	"testing"
)

// R1: finite extraction draws local stock once. An unreachable distant stock
// cannot fund a local action, even when global totals would be sufficient.
func TestLawEcologyExtractionCannotBorrowFromRemoteOrEmptyPool(t *testing.T) {
	w := lawWorld()
	w.Catalog.Recipes = []Recipe{{ID: "cut", RequiredTag: "person", Ticks: 1, EnergyCost: 2, ResourcePool: "trees", ResourceDraw: 2, OutputItem: "wood", OutputQty: 3, Conversion: &MaterialConversion{ExternalMassUnits: 3, Source: "fixture: two finite tree units contain three wood mass units"}}}
	w.DistrictStates = map[string]DistrictState{
		"home": {Resources: map[string]int64{"trees": 3}, MaxResources: map[string]int64{"trees": 3}},
		"away": {Resources: map[string]int64{"trees": 100}, MaxResources: map[string]int64{"trees": 100}},
	}
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "cut"}))
	if w.Actors[0].Items["wood"] != 3 || w.DistrictStates["home"].Resources["trees"] != 1 {
		t.Fatal("delivered wood lacks its declared finite local source debit")
	}
	lawRejected(t, w.Apply("b", Action{Kind: "craft", RecipeID: "cut"}))
	if w.Actors[1].Items["wood"] != 0 || w.Actors[1].Energy != 100 || w.DistrictStates["home"].Resources["trees"] != 1 || w.DistrictStates["away"].Resources["trees"] != 100 {
		t.Fatal("shortage was funded by partial or distant extraction")
	}
	w.regenerateEcologicalResources(CalendarState{}, []string{"home", "away"})
	if w.DistrictStates["home"].Resources["trees"] != 1 {
		t.Fatal("nonrenewable stock regenerated without a configured source")
	}
}

func TestLawEcologyWaterAccessRequiresActualLocalWater(t *testing.T) {
	for _, stock := range []int64{0, 1} {
		w := lawWorld()
		w.Actors[0].Thirst = 2
		w.DistrictStates = map[string]DistrictState{"home": {Resources: map[string]int64{"freshwater": stock}, MaxResources: map[string]int64{"freshwater": 1}}, "away": {Resources: map[string]int64{"freshwater": 100}}}
		events := w.Apply("a", Action{Kind: "drink"})
		if stock == 0 {
			lawRejected(t, events)
			if w.Actors[0].Thirst != 2 {
				t.Fatal("dry district relieved thirst")
			}
		} else {
			lawOK(t, events)
			if w.Actors[0].Thirst != 0 || w.DistrictStates["home"].Resources["freshwater"] != 0 {
				t.Fatal("drinking did not consume the one available local water unit")
			}
		}
		if w.DistrictStates["away"].Resources["freshwater"] != 100 {
			t.Fatal("drinking spent remote water")
		}
	}
}

func lawWatershed() *World {
	w := lawWorld()
	w.Map.Districts = []District{{ID: "up", GeoRegionID: "up"}, {ID: "mid", GeoRegionID: "mid"}, {ID: "side", GeoRegionID: "side"}, {ID: "down", GeoRegionID: "down"}, {ID: "island", GeoRegionID: "island"}}
	w.Map.GeoFlows = []GeoFlowEdge{{FromRegion: "up", ToRegion: "mid", Kind: "river", ConductancePermille: 500}, {FromRegion: "up", ToRegion: "side", Kind: "aquifer", ConductancePermille: 500}, {FromRegion: "mid", ToRegion: "down", Kind: "river", ConductancePermille: 1000}}
	w.DistrictStates = map[string]DistrictState{}
	for _, id := range []string{"up", "mid", "side", "down", "island"} {
		w.DistrictStates[id] = DistrictState{Resources: map[string]int64{"freshwater": 0}, MaxResources: map[string]int64{"freshwater": 20 * TicksPerDay}}
	}
	w.DistrictStates["up"].Resources["freshwater"] = 12 * TicksPerDay
	return w
}

// R2: branching is partition, not duplication. Arrival cannot traverse a second
// edge in the same update, and each tick routes 1/TicksPerDay of daily conductance.
// Storage overflow remains explicit in the water budget.
func TestLawEcologyWatershedConservesWaterAndMovesOneEdge(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		w := lawWatershed()
		if overflow {
			w.DistrictStates["mid"].MaxResources["freshwater"] = 2
		}
		control := lawRestore(t, w)
		w.routeEcologicalWater([]string{"up", "mid", "side", "down", "island"})
		control.routeEcologicalWater([]string{"island", "down", "side", "mid", "up"})
		if !reflect.DeepEqual(w.DistrictStates, control.DistrictStates) {
			t.Fatal("water movement depends on district enumeration order")
		}
		var total int64
		for _, ds := range w.DistrictStates {
			total += ds.Resources["freshwater"] + ds.WaterOverflow["freshwater"]
		}
		wantInitial := int64(12 * TicksPerDay)
		if total != wantInitial || w.DistrictStates["up"].Resources["freshwater"] != wantInitial-12 || w.DistrictStates["side"].Resources["freshwater"] != 6 || w.DistrictStates["down"].Resources["freshwater"] != 0 || w.DistrictStates["island"].Resources["freshwater"] != 0 {
			t.Fatalf("branching duplicated, lost or teleported water: %+v", w.DistrictStates)
		}
		if overflow && (w.DistrictStates["mid"].Resources["freshwater"] != 2 || w.DistrictStates["mid"].WaterOverflow["freshwater"] != 4) {
			t.Fatal("receiving capacity concealed four units of overflow")
		}
	}
}

func TestLawEcologyHydrologyLinearPerTickRemainderAndNoMorningCliff(t *testing.T) {
	// 1. Small source stock accumulates exact integer remainders linearly across ticks without disappearing.
	small := lawWatershed()
	for _, id := range []string{"up", "mid", "side", "down", "island"} {
		ds := small.DistrictStates[id]
		ds.MaxResources["freshwater"] = 20
		ds.Resources["freshwater"] = 0
		small.DistrictStates[id] = ds
	}
	upDS := small.DistrictStates["up"]
	upDS.Resources["freshwater"] = 12
	small.DistrictStates["up"] = upDS

	for tick := 0; tick < 16; tick++ {
		small.routeEcologicalWater([]string{"up", "mid", "side", "down", "island"})
		var total int64
		for _, ds := range small.DistrictStates {
			total += ds.Resources["freshwater"] + ds.WaterOverflow["freshwater"]
		}
		if total != 12 {
			t.Fatalf("tick %d violated water conservation: total=%d", tick, total)
		}
		if tick < 15 && small.DistrictStates["up"].Resources["freshwater"] != 12 {
			t.Fatalf("tick %d transferred whole unit prematurely: up=%d", tick, small.DistrictStates["up"].Resources["freshwater"])
		}
	}
	// 16 * (12 * 500) = 96000 = 1000 * TicksPerDay -> exactly 1 unit to mid and 1 unit to side.
	if small.DistrictStates["up"].Resources["freshwater"] != 10 || small.DistrictStates["mid"].Resources["freshwater"] != 1 || small.DistrictStates["side"].Resources["freshwater"] != 1 {
		t.Fatalf("16 linear ticks did not deliver exact remainder-accumulated units: %+v", small.DistrictStates)
	}
	if len(small.DistrictStates["up"].WaterFlowRemainder) != 0 {
		t.Fatalf("exact division left stale remainder: %+v", small.DistrictStates["up"].WaterFlowRemainder)
	}

	// 2. Continuous river basin across 07:00 (tick 4) changes smoothly per tick with no batch cliff.
	basin := lawWorld()
	basin.Map.Districts = []District{{ID: "upper", GeoRegionID: "upper"}, {ID: "lower", GeoRegionID: "lower"}}
	basin.Map.GeoFlows = []GeoFlowEdge{{FromRegion: "upper", ToRegion: "lower", Kind: "river", ConductancePermille: 800}}
	basin.Catalog.Resources["freshwater"] = ResourcePoolSpec{ID: "freshwater", PotableWater: true, RegenPermillePerDay: 192}
	basin.DistrictStates = map[string]DistrictState{
		"upper": {Weather: "clear", Resources: map[string]int64{"freshwater": 27500}, MaxResources: map[string]int64{"freshwater": 27500}},
		"lower": {Weather: "clear", Resources: map[string]int64{"freshwater": 42000}, MaxResources: map[string]int64{"freshwater": 42000}},
	}
	prevUpper := basin.DistrictStates["upper"].Resources["freshwater"]
	for tick := int64(0); tick <= 12; tick++ {
		basin.Tick = tick
		basin.stepWorldEnvironment()
		curUpper := basin.DistrictStates["upper"].Resources["freshwater"]
		drop := prevUpper - curUpper
		if drop <= 0 || drop > 250 {
			t.Fatalf("tick %d produced non-linear water jump: prev=%d cur=%d drop=%d", tick, prevUpper, curUpper, drop)
		}
		prevUpper = curUpper
	}
}

func TestLawEcologyLowPoolRatioDeliversFullDeclaredYieldWhenDrawIsCovered(t *testing.T) {
	w := lawWorld()
	r := Recipe{
		ID:           "draw_water",
		RequiredTag:  "person",
		NodeFeature:  "well",
		Ticks:        1,
		EnergyCost:   2,
		ResourcePool: "freshwater",
		ResourceDraw: 2,
		OutputItem:   "water",
		OutputQty:    4,
		Conversion:   &MaterialConversion{ExternalMassUnits: 4, Source: "fixture: 2 pool units yield 4 water jars"},
	}
	w.Catalog.Items["water"] = ItemSpec{ID: "water", MassUnits: 1, Tier: 1, Potable: true, ThirstRelief: 2}
	w.Catalog.Recipes = append(w.Catalog.Recipes, r)
	w.Map.Districts[0].Features = []string{"well"}
	w.DistrictStates = map[string]DistrictState{
		"home": {
			Weather:      "clear",
			Resources:    map[string]int64{"freshwater": 2},
			MaxResources: map[string]int64{"freshwater": 10000},
		},
	}
	if mult := w.EcologicalYieldMultiplierPct("home", r); mult != 100 {
		t.Fatalf("expected 100%% ecological multiplier below 50%% capacity, got %d%%", mult)
	}
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "draw_water"}))
	if w.Actors[0].Items["water"] != 4 || w.DistrictStates["home"].Resources["freshwater"] != 0 {
		t.Fatalf("paid extraction below 50%% pool capacity throttled output: got %d water, pool=%d", w.Actors[0].Items["water"], w.DistrictStates["home"].Resources["freshwater"])
	}
}

func TestLawEcologyRecruitmentNeedsParentsAndMaturation(t *testing.T) {
	for _, parents := range []int64{0, 3} {
		w := lawWorld()
		w.Catalog.Resources["fish"] = ResourcePoolSpec{ID: "fish", Biological: true, ReproductionSource: "fish", RegenPermillePerDay: 1000, MaturityTicks: 3}
		w.DistrictStates = map[string]DistrictState{"home": {Resources: map[string]int64{"fish": parents}, MaxResources: map[string]int64{"fish": 20}}}
		w.Tick = 1
		w.regenerateEcologicalResources(CalendarState{}, []string{"home"})
		if w.DistrictStates["home"].Resources["fish"] != parents {
			t.Fatal("new recruitment was immediately harvestable")
		}
		for tick := int64(2); tick <= 4; tick++ {
			w.Tick = tick
			ds := w.DistrictStates["home"]
			w.advanceResourceRecruitment("home", &ds)
			w.DistrictStates["home"] = ds
			want := parents
			if tick == 4 {
				want = parents * 2
			} // fixture rate: one recruit per mature parent
			if ds.Resources["fish"] != want {
				t.Fatalf("parents=%d tick=%d: got %d want %d", parents, tick, ds.Resources["fish"], want)
			}
		}
	}
}

// E1: fuel is inventory before refueling, generator stock afterward, and energy
// only after conversion. One fixture fuel unit yields five energy units.
func TestLawEnergyFuelTransferConversionAndExhaustion(t *testing.T) {
	w := lawWorld()
	w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "refuel")
	w.Actors[0].Items["wood"] = 3
	w.FlowGrid = &FlowGridState{Nodes: []FlowNode{
		{ID: "g", Kind: "generator", DistrictID: "home", Active: true, Generation: 10, EnergyCapacity: 10, Source: &EnergySource{FuelItem: "wood", UnitsPerCycle: 2, EnergyPerUnit: 5, CycleTicks: 2, Source: "fixture conversion"}},
		{ID: "load", Kind: "consumer", Active: true, Demand: 5},
	}, Edges: []FlowEdge{{FromNodeID: "g", ToNodeID: "load", Capacity: 10}}}
	lawOK(t, w.Apply("a", Action{Kind: "refuel", TargetID: "g", Quantity: 2}))
	if w.Actors[0].Items["wood"] != 1 || w.FlowGrid.Nodes[0].FuelStock != 2 || w.FlowGrid.Nodes[0].StoredEnergy != 0 {
		t.Fatal("refueling created energy or lost physical fuel")
	}
	w.supplyEnergy()
	if w.FlowGrid.Nodes[0].StoredEnergy != 10 || w.FlowGrid.Nodes[0].FuelStock != 0 {
		t.Fatal("conversion did not exchange exactly two fuel units for ten energy units")
	}
	for tick := int64(0); tick <= 2; tick++ {
		w.Tick = tick
		w.stepFlowGrid()
		if tick < 2 && (!w.FlowGrid.Nodes[1].Powered || w.FlowGrid.Nodes[0].StoredEnergy != 5*(1-tick)) {
			t.Fatal("connected five-unit load did not spend five stored energy units")
		}
	}
	if w.FlowGrid.Nodes[1].Powered || w.FlowGrid.Nodes[0].StoredEnergy != 0 || w.Actors[0].Items["wood"] != 1 {
		t.Fatal("exhausted generator delivered power or secretly consumed personal inventory")
	}
}

func TestLawEnergyDisconnectedAndLossyRoutesSpendOnlyDeliverableSupply(t *testing.T) {
	for _, mode := range []string{"cut", "connected", "loss"} {
		w := lawWorld()
		w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "g", Kind: "generator", Active: true, Generation: 10, StoredEnergy: 10}, {ID: "c", Kind: "consumer", Active: true, Demand: 1}}, Edges: []FlowEdge{{FromNodeID: "g", ToNodeID: "c", Capacity: 10}}}
		if mode == "cut" {
			w.FlowGrid.Edges[0].Severed = true
		}
		if mode == "loss" {
			w.FlowGrid.Edges[0].LossPermille = 900
		}
		w.stepFlowGrid()
		wantStored := int64(9)
		if mode == "cut" {
			wantStored = 10
		}
		if mode == "loss" {
			wantStored = 0
		} // ten source units deliver one after 90% loss
		if w.FlowGrid.Nodes[0].StoredEnergy != wantStored || w.FlowGrid.Nodes[1].Powered != (mode != "cut") {
			t.Fatalf("%s: wrong delivered power/source debit: %+v", mode, w.FlowGrid.Nodes)
		}
	}
}

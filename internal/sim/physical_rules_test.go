package sim

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestPhysicalNoDisconnectedRetail(t *testing.T) {
	w := LargeWorld("physical-rules", 1000000)
	w.Map.Roads = nil
	w.Map.Districts = append(w.Map.Districts, District{ID: "isolated"})
	w.Actors = []Actor{{ID: "buyer", Location: "isolated", Health: 100, Money: 100, Items: map[string]int{}}}
	w.clearFocalActorCohortTrades()
	if len(w.Actors[0].Items) != 0 || w.Actors[0].Money != 100 {
		t.Fatal("disconnected buyer received goods or paid")
	}
}

func TestPhysicalFreightCapacityIsNotInflated(t *testing.T) {
	w := Village("physical-rules")
	for _, capacity := range []int64{0, 1, 999, 1000} {
		actual, _, _, _, _, _ := w.edgeEffectiveLogistics(Road{From: "west_cottages", To: "creek_well", Medium: "land", FreightCapacity: capacity}, CalendarAt(0, w.Catalog.Calendar))
		if actual != capacity {
			t.Errorf("capacity %d became %d", capacity, actual)
		}
	}
}

func TestPhysicalProductionRequiresMaterialSource(t *testing.T) {
	for _, r := range DefaultCatalog().Recipes {
		if r.OutputItem != "" && r.OutputQty > 0 && len(r.Inputs) == 0 && r.ResourcePool == "" {
			t.Errorf("recipe %s creates %s without material input", r.ID, r.OutputItem)
		}
	}
}

func TestPhysicalWeatherCanCloseFreightRoute(t *testing.T) {
	w := Village("physical-rules")
	w.Catalog.Weathers = map[string]WeatherSpec{"closed": {RouteCapacityPct: map[string]int{"land": 0}}}
	ds := w.DistrictStates["west_cottages"]
	ds.Weather = "closed"
	w.DistrictStates["west_cottages"] = ds
	actual, _, _, _, _, _ := w.edgeEffectiveLogistics(Road{From: "west_cottages", To: "creek_well", Medium: "land", FreightCapacity: 100}, CalendarAt(0, w.Catalog.Calendar))
	if actual != 0 {
		t.Fatalf("closed route still carries %d", actual)
	}
}

func TestPhysicalGrowthWaitsForCycleAndLocalHarvest(t *testing.T) {
	w := resourceTestWorld(100)
	w.Cohorts = nil
	w.Catalog.Items["livestock"] = ItemSpec{ID: "livestock"}
	w.Catalog.TagActions = map[string][]string{"worker": {"craft", "harvest"}}
	w.Catalog.Recipes[0].Ticks = 1
	w.Catalog.Recipes[0].Growth = &GrowthRule{DurationTicks: 4, MaintenanceIntervalTicks: 2, Supplies: map[string]int{"water": 1}, Parents: map[string]int{"livestock": 2}, Source: "test species with a four-tick cycle"}
	w.Actors = []Actor{{ID: "a", Location: "forest", Tags: []string{"worker"}, Professions: map[string]int{"logger": 1}, Health: 100, Energy: 100, Items: map[string]int{"livestock": 2, "water": 2}}}
	w.InitLedgerAudit()
	w.Apply("a", Action{Kind: "craft", RecipeID: "cut"})
	if len(w.Growth) != 1 || w.Actors[0].Items["livestock"] != 0 || w.Actors[0].Items["timber"] != 0 {
		t.Fatal("parent stock was not reserved or immature product appeared")
	}
	for tick := int64(1); tick <= 4; tick++ {
		w.Tick = tick
		w.AdvanceGrowth()
		if tick < 4 && w.Growth[0].Ready {
			t.Fatal("matured before configured cycle")
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatal(err)
		}
	}
	w.Actors[0].Location = "elsewhere"
	failed := w.Apply("a", Action{Kind: "harvest", TargetID: fmt.Sprint(w.Growth[0].ID)})
	if failed[0].Kind != "action_failed" {
		t.Fatal("remote harvest teleported goods")
	}
	w.Actors[0].Location = "forest"
	w.Apply("a", Action{Kind: "harvest", TargetID: fmt.Sprint(w.Growth[0].ID)})
	if w.Actors[0].Items["timber"] != 4 || w.Actors[0].Items["livestock"] != 2 || len(w.Growth) != 0 {
		t.Fatal("harvest or return of parents is wrong")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalGrowthCannotAdvanceWithoutSupplies(t *testing.T) {
	w := resourceTestWorld(100)
	r := w.Catalog.Recipes[0]
	r.Ticks = 1
	r.Growth = &GrowthRule{DurationTicks: 2, MaintenanceIntervalTicks: 1, Supplies: map[string]int{"water": 1}, Source: "test crop"}
	w.InitLedgerAudit()
	w.startGrowth(w.Cohorts[0].ID, "forest", "", true, r, 1, 4, nil)
	w.Tick = 10
	w.AdvanceGrowth()
	if w.Growth[0].ProgressTicks != 0 || w.Growth[0].Ready || w.Cohorts[0].ItemPool["timber"] != 0 {
		t.Fatal("growth advanced with no supply")
	}
}

func TestPhysicalCohortWorkTakesConfiguredTime(t *testing.T) {
	w := resourceTestWorld(100)
	w.Catalog.Recipes[0].Ticks = 3
	w.InitLedgerAudit()
	w.runCohortProduction(&w.Cohorts[0])
	if w.Cohorts[0].ItemPool["timber"] != 0 || len(w.Growth) != 1 {
		t.Fatal("group received finished goods immediately")
	}
	w.runCohortProduction(&w.Cohorts[0])
	if len(w.Growth) != 1 {
		t.Fatal("same workforce started overlapping work")
	}
	w.Tick = 2
	w.AdvanceGrowth()
	if w.Cohorts[0].ItemPool["timber"] != 0 {
		t.Fatal("group production finished early")
	}
	w.Tick = 3
	w.AdvanceGrowth()
	if w.Cohorts[0].ItemPool["timber"] == 0 || len(w.Growth) != 0 {
		t.Fatal("group production did not finish")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalFreightArrivesAfterTravelAndSurvivesSnapshot(t *testing.T) {
	w := Village("physical-shipment")
	w.Actors = nil
	w.Map = WorldMap{Districts: []District{{ID: "a"}, {ID: "b"}}, Roads: []Road{{From: "a", To: "b", Ticks: 3, FreightCapacity: 1, Medium: "land"}}}
	w.Cohorts = []PopulationCohort{{ID: "seller", District: "a", Profession: "woodcutter", Population: 2, ItemPool: map[string]int64{"timber": 100}}, {ID: "buyer", District: "b", Profession: "carpenter", Population: 10, MoneyPool: 100, ItemPool: map[string]int64{}}}
	w.InitLedgerAudit()
	w.clearRegionalCohortMarket()
	if len(w.Shipments) != 1 || w.Shipments[0].Quantity != 2 || w.Cohorts[1].ItemPool["timber"] != 0 {
		t.Fatal("shipment ignored capacity or arrived instantly")
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var loaded World
	if err = json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	loaded.Tick = 2
	loaded.settleShipments()
	if loaded.Cohorts[1].ItemPool["timber"] != 0 {
		t.Fatal("shipment arrived too soon")
	}
	loaded.Tick = 3
	loaded.settleShipments()
	if loaded.Cohorts[1].ItemPool["timber"] != 2 || len(loaded.Shipments) != 0 {
		t.Fatal("shipment lost on restore")
	}
	if err := loaded.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalEnergyConsumesFuelOnConfiguredCycle(t *testing.T) {
	w := &World{Catalog: Catalog{Items: map[string]ItemSpec{"fuel": {ID: "fuel"}}}, FlowGrid: &FlowGridState{
		Nodes: []FlowNode{{ID: "g", Kind: "generator", Active: true, Generation: 10, EnergyCapacity: 10, FuelStock: 2, Source: &EnergySource{FuelItem: "fuel", UnitsPerCycle: 1, EnergyPerUnit: 10, CycleTicks: 3, Source: "test fuel rating"}}, {ID: "c", Kind: "consumer", Active: true, Demand: 6}},
		Edges: []FlowEdge{{ID: "e", FromNodeID: "g", ToNodeID: "c", Capacity: 10}}}}
	w.InitLedgerAudit()
	w.stepFlowGrid()
	if w.FlowGrid.Nodes[0].FuelStock != 1 || w.FlowGrid.Nodes[0].StoredEnergy != 4 || !w.FlowGrid.Nodes[1].Powered {
		t.Fatal("first cycle did not burn fuel and supply power")
	}
	w.Tick = 1
	w.stepFlowGrid()
	if w.FlowGrid.Nodes[1].Powered || w.FlowGrid.Nodes[0].StoredEnergy != 0 || w.FlowGrid.Nodes[0].FuelStock != 1 {
		t.Fatal("power created between fuel cycles")
	}
	w.Tick = 3
	w.stepFlowGrid()
	if !w.FlowGrid.Nodes[1].Powered || w.FlowGrid.Nodes[0].FuelStock != 0 {
		t.Fatal("second cycle did not consume last fuel unit")
	}
	w.Tick = 6
	w.stepFlowGrid()
	if w.FlowGrid.Nodes[1].Powered {
		t.Fatal("exhausted source supplied full demand")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalCalendarUsesConfiguredSeasonLengths(t *testing.T) {
	c := DefaultCalendarSpec()
	if got := CalendarAt(365*TicksPerDay, c); got.Year != 2 || got.DayOfYear != 1 {
		t.Fatalf("default year: %+v", got)
	}
	custom := CalendarSpec{Seasons: []SeasonSpec{{ID: "wet", Days: 2}, {ID: "dry", Days: 3}}}
	if got := CalendarAt(2*TicksPerDay, custom); got.SeasonID != "dry" || got.DayOfSeason != 1 {
		t.Fatalf("custom season: %+v", got)
	}
}

func TestPhysicalFullEnergyStorageDoesNotBurnFuel(t *testing.T) {
	w := &World{Catalog: Catalog{Items: map[string]ItemSpec{"fuel": {ID: "fuel"}}}, FlowGrid: &FlowGridState{Nodes: []FlowNode{{ID: "g", Kind: "generator", Active: true, Generation: 10, EnergyCapacity: 10, StoredEnergy: 10, FuelStock: 1, Source: &EnergySource{FuelItem: "fuel", UnitsPerCycle: 1, EnergyPerUnit: 10, CycleTicks: 1, Source: "test fuel"}}}}}
	w.InitLedgerAudit()
	w.stepFlowGrid()
	if w.FlowGrid.Nodes[0].FuelStock != 1 || w.FlowGrid.Nodes[0].StoredEnergy != 10 {
		t.Fatal("full storage consumed fuel or exceeded capacity")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalCompostIsProcessedAfterDeclaredCycle(t *testing.T) {
	w := resourceTestWorld(100)
	w.Catalog = DefaultCatalog()
	recipe, _ := w.Catalog.RecipeByID("compost_waste")
	recipe.Growth = &GrowthRule{DurationTicks: 3, MaintenanceIntervalTicks: 3, Supplies: map[string]int{"water": 1}, Source: "three-tick test compost process"}
	w.Catalog.Recipes = []Recipe{recipe}
	w.Cohorts[0].Profession = "farmer"
	w.Cohorts[0].Population = 6
	w.Cohorts[0].ItemPool = map[string]int64{"nightsoil": 3, "water": 1}
	w.Houses = []House{{ID: "pit", District: "forest", Facilities: []string{"compost_pit"}}}
	w.InitLedgerAudit()
	w.runCohortProduction(&w.Cohorts[0])
	if w.Cohorts[0].ItemPool["compost"] != 0 || w.Cohorts[0].ItemPool["nightsoil"] != 0 {
		t.Fatal("unprocessed waste became compost or feedstock was not used")
	}
	w.Tick = int64(recipe.Ticks) + 2
	w.AdvanceGrowth()
	if w.Cohorts[0].ItemPool["compost"] != 0 {
		t.Fatal("compost matured before its configured cycle")
	}
	w.Tick++
	events := w.AdvanceGrowth()
	found := false
	for _, e := range events {
		if e.Kind == "nightsoil_composted" {
			found = true
		}
	}
	if !found || w.Cohorts[0].ItemPool["compost"] != 1 {
		t.Fatal("mature compost was not delivered")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalHungerStateDoesNotCreateWaste(t *testing.T) {
	w := resourceTestWorld(0)
	w.Catalog = DefaultCatalog()
	w.Tick = 23
	w.InitLedgerAudit()
	w.stepCohortPhysiologySanitationAndLifecycle()
	if w.DistrictStates["forest"].NightsoilPool != 0 || w.Cohorts[0].ItemPool["nightsoil"] != 0 || w.Cohorts[0].ItemPool["compost"] != 0 {
		t.Fatal("fed-looking hunger state produced matter without consumed food")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestPhysicalConfigurationRejectsUnfundedRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Catalog)
	}{
		{"missing material", func(c *Catalog) { c.Recipes[0].Inputs = nil; c.Recipes[0].ResourcePool = "" }},
		{"zero season", func(c *Catalog) { c.Calendar.Seasons[0].Days = 0 }},
		{"unknown care supply", func(c *Catalog) {
			for i := range c.Recipes {
				if c.Recipes[i].Growth != nil {
					c.Recipes[i].Growth.Supplies = map[string]int{"missing": 1}
					return
				}
			}
		}},
		{"zero growth cycle", func(c *Catalog) {
			for i := range c.Recipes {
				if c.Recipes[i].Growth != nil {
					c.Recipes[i].Growth.DurationTicks = 0
					return
				}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := DefaultCatalog()
			if err := c.ValidatePhysicalRules(nil); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&c)
			if err := c.ValidatePhysicalRules(nil); err == nil {
				t.Fatal("invalid external rule accepted")
			}
		})
	}
}

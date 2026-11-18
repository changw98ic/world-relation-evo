package sim

import (
	"fmt"
	"testing"
)

// P1: inputs and declared work precede output. Neither extra skill nor repeat
// completion may create additional matter from this nonbiological conversion.
func TestLawProductionMaterialAndWorkPrecedeOutput(t *testing.T) {
	for _, skill := range []int{1, 9} {
		t.Run(fmt.Sprint(skill), func(t *testing.T) {
			w := lawWorld()
			a := &w.Actors[0]
			a.Items["wood"], a.Professions["worker"] = 3, skill
			lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
			if a.Items["wood"] != 0 || a.Energy != 96 || a.Items["board"] != 0 || a.Task == nil {
				t.Fatalf("start must pay three wood/four energy without finished output: %+v", a)
			}
			w.Tick++
			lawOK(t, w.Advance("a"))
			if a.Items["board"] != 0 || a.Task == nil {
				t.Fatal("a three-tick job finished after only two ticks")
			}
			w.Tick++
			lawOK(t, w.Advance("a"))
			if a.Items["board"] != 1 || a.Task != nil || a.Items["wood"]+3*a.Items["board"] != 3 {
				t.Fatalf("material conversion changed its declared mass: %+v", a)
			}
			w.Advance("a")
			if a.Items["board"] != 1 || a.Energy != 96 {
				t.Fatal("completed work was paid or delivered twice")
			}
		})
	}
}

func TestLawProductionShortageHasNoPartialPayment(t *testing.T) {
	for _, missing := range []string{"material", "energy"} {
		t.Run(missing, func(t *testing.T) {
			w := lawWorld()
			a := &w.Actors[0]
			a.Items["wood"], a.Energy = 3, 4 // exact legal boundary in the control below
			if missing == "material" {
				a.Items["wood"] = 2
			} else {
				a.Energy = 3
			}
			wood, energy := a.Items["wood"], a.Energy
			lawRejected(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
			if a.Items["wood"] != wood || a.Energy != energy || a.Task != nil || a.Items["board"] != 0 {
				t.Fatal("failed start charged an unrelated available resource")
			}
			a.Items["wood"], a.Energy = 3, 4
			lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
			if a.Items["wood"] != 0 || a.Energy != 0 || a.Task == nil {
				t.Fatal("exact declared inputs and energy must start the job")
			}
		})
	}
}

func TestLawProductionWorkstationIsExclusiveAndPowerPausePreservesMaterials(t *testing.T) {
	w := lawWorld()
	w.Catalog.Recipes[0].HouseFacility = "bench"
	w.Houses = []House{{ID: "shop", District: "home", OwnerID: "a", Residents: []string{"a", "b"}, Condition: 100, Facilities: []string{"bench"}}}
	w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "load", Kind: "consumer", VenueID: "workshop", Active: true, Powered: true}}}
	w.Map.Venues = []Venue{{ID: "workshop", DistrictID: "home"}}
	w.Houses[0].VenueID = "workshop"
	for i := 0; i < 2; i++ {
		w.Actors[i].InsideHouse, w.Actors[i].VenueID, w.Actors[i].Items["wood"] = "shop", "workshop", 3
	}
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
	lawRejected(t, w.Apply("b", Action{Kind: "craft", RecipeID: "board"}))
	if w.Actors[1].Items["wood"] != 3 || w.Actors[1].Task != nil {
		t.Fatal("second worker double-booked the only bench")
	}
	w.FlowGrid.Nodes[0].Powered = false
	for i := 0; i < 2; i++ {
		w.Tick++
		w.Advance("a")
	}
	if w.Actors[0].Task == nil || w.Actors[0].Task.Remaining != 2 || w.Actors[0].Items["board"] != 0 || w.Actors[0].Items["wood"] != 0 {
		t.Fatal("unpowered work progressed or prepaid materials changed")
	}
	w.FlowGrid.Nodes[0].Powered = true
	for i := 0; i < 2; i++ {
		w.Tick++
		lawOK(t, w.Advance("a"))
	}
	if w.Actors[0].Items["board"] != 1 {
		t.Fatal("restored power failed to complete paid work")
	}
	lawOK(t, w.Apply("b", Action{Kind: "craft", RecipeID: "board"}))
	if w.Actors[1].Task == nil {
		t.Fatal("completed job did not release the bench")
	}
}

// A house is the actor's actual work site after entry, including a house whose
// location declares only its district. An outdoor venue must not hide its bench.
func TestLawProductionEnteredHouseProvidesItsOwnWorkstation(t *testing.T) {
	for _, venue := range []string{"", "workshop"} {
		t.Run("house_venue="+venue, func(t *testing.T) {
			w := lawWorld()
			w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "enter_house")
			w.Catalog.Recipes[0].HouseFacility = "bench"
			w.Map.Venues = []Venue{{ID: "workshop", DistrictID: "home"}}
			w.Houses = []House{{ID: "shop", District: "home", VenueID: venue, OwnerID: "a", Condition: 100, Facilities: []string{"bench"}}}
			w.Actors[0].VenueID = "workshop"
			w.Actors[0].Items["wood"] = 3
			lawOK(t, w.Apply("a", Action{Kind: "enter_house", HouseID: "shop"}))
			if reason := w.ProductionStartBlockReasons(w.Actors[0])["board"]; reason != "" {
				t.Fatalf("an intact, entered house has one unused bench and no power requirement: %s", reason)
			}
			lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
			for i := 0; i < 2; i++ {
				w.Tick++
				lawOK(t, w.Advance("a"))
			}
			if w.Actors[0].Items["board"] != 1 || w.Actors[0].Items["wood"] != 0 {
				t.Fatal("entry must make the actual house's funded workstation usable")
			}
		})
	}
}

// A scarce non-biological conversion keeps the funded fraction when seasonal or
// weather conditions reduce feature yield, while low pool capacity never throttles
// paid extraction once ResourceDraw is covered.
func TestLawProductionScarceYieldKeepsTheFundedFraction(t *testing.T) {
	w := lawEnvDrawWorld(20, 40)
	if got := w.View("a").ExpectedCraftYield["draw"]; got != 0 {
		t.Fatalf("candidate promised %d water when this paid batch delivers 0", got)
	}
	started := w.Apply("a", Action{Kind: "craft", RecipeID: "draw"})
	lawOK(t, started)
	if started[0].Payload["expected_yield"] != 0 || w.Actors[0].Items["water"] != 0 || w.DistrictStates["home"].Resources["freshwater"] != 19 || w.Actors[0].Energy != 95 {
		t.Fatalf("start paid the resource and energy but published a different expectation: %+v energy=%d", started, w.Actors[0].Energy)
	}
	if w.yieldCredit("actor", "a", "draw") != 0 || w.Actors[0].Task.FundedHundredths != 80 {
		t.Fatal("an unfinished batch banked or dropped its funded fraction")
	}
	w.FlowGrid = &FlowGridState{Nodes: []FlowNode{{ID: "load", Kind: "consumer", VenueID: "workshop", Active: true, Powered: false}}}
	w.Map.Venues = []Venue{{ID: "workshop", DistrictID: "home"}}
	w.Actors[0].VenueID = "workshop"
	w.Tick++
	w.Advance("a")
	if w.Actors[0].Task == nil || w.Actors[0].Items["water"] != 0 || w.yieldCredit("actor", "a", "draw") != 0 {
		t.Fatal("a paused batch delivered items or destroyed the unpaid remainder")
	}
	w.FlowGrid.Nodes[0].Powered = true
	lawOK(t, w.Advance("a"))
	if w.Actors[0].Items["water"] != 0 || w.yieldCredit("actor", "a", "draw") != 80 {
		t.Fatalf("first completion did not retain 80/100: items=%d credit=%d", w.Actors[0].Items["water"], w.yieldCredit("actor", "a", "draw"))
	}
	lawFinishDraw(t, w)
	if w.Actors[0].Items["water"] != 1 || w.yieldCredit("actor", "a", "draw") != 60 {
		t.Fatalf("second completion did not deliver the retained unit: items=%d credit=%d", w.Actors[0].Items["water"], w.yieldCredit("actor", "a", "draw"))
	}
	for i := 0; i < 3; i++ {
		lawFinishDraw(t, w)
	}
	if w.Actors[0].Items["water"] != 4 || w.yieldCredit("actor", "a", "draw") != 0 || w.DistrictStates["home"].Resources["freshwater"] != 15 || w.Actors[0].Energy != 75 {
		t.Fatalf("five paid batches did not deliver four units: items=%d credit=%d water=%d energy=%d", w.Actors[0].Items["water"], w.yieldCredit("actor", "a", "draw"), w.DistrictStates["home"].Resources["freshwater"], w.Actors[0].Energy)
	}

	for _, lowStock := range []int64{1, 20, 49} {
		full := lawEnvDrawWorld(lowStock, 100)
		if mult := full.EcologicalYieldMultiplierPct("home", full.Catalog.Recipes[1]); mult != 100 {
			t.Fatalf("stock=%d/100 throttled yield multiplier to %d%%", lowStock, mult)
		}
		if got := full.View("a").ExpectedCraftYield["draw"]; got != 2 {
			t.Fatalf("stock=%d/100 expected %d instead of full declared output 2", lowStock, got)
		}
		lawFinishDraw(t, full)
		if full.Actors[0].Items["water"] != 2 || full.yieldCredit("actor", "a", "draw") != 0 || full.DistrictStates["home"].Resources["freshwater"] != lowStock-1 {
			t.Fatalf("stock=%d/100 failed to deliver full declared two units once: items=%d credit=%d left=%d", lowStock, full.Actors[0].Items["water"], full.yieldCredit("actor", "a", "draw"), full.DistrictStates["home"].Resources["freshwater"])
		}
	}

	blocked := lawEnvDrawWorld(80, 0)
	blocked.Actors[0].Energy = 5
	before := blocked.DistrictStates["home"].Resources["freshwater"]
	lawRejected(t, blocked.Apply("a", Action{Kind: "craft", RecipeID: "draw"}))
	if blocked.Actors[0].Energy != 5 || blocked.DistrictStates["home"].Resources["freshwater"] != before || blocked.Actors[0].Items["water"] != 0 || blocked.Actors[0].Task != nil {
		t.Fatal("a zero environment multiplier paid for no output")
	}

	exact := lawEnvDrawWorld(1, 100)
	exact.Actors[0].Energy = 5
	lawOK(t, exact.Apply("a", Action{Kind: "craft", RecipeID: "draw"}))
	if exact.Actors[0].Energy != 0 || exact.DistrictStates["home"].Resources["freshwater"] != 0 || exact.Actors[0].Task == nil {
		t.Fatal("exact declared resource and energy were refused")
	}

	boosted := lawEnvDrawWorld(80, 100)
	ds := boosted.DistrictStates["home"]
	ds.MoraleBonus = 240
	boosted.DistrictStates["home"] = ds
	if got := boosted.EcologicalYieldMultiplierPct("home", boosted.Catalog.Recipes[1]); got != 180 {
		t.Fatalf("the counterexample did not reach the biological 180 percent cap: %d", got)
	}
	lawFinishDraw(t, boosted)
	if boosted.Actors[0].Items["water"] != 2 || boosted.yieldCredit("actor", "a", "draw") != 0 {
		t.Fatal("a non-biological recipe created matter above its declared output")
	}

	for _, pop := range []int{5, 7} {
		group := lawEnvDrawWorld(20, 40)
		group.Cohorts = []PopulationCohort{{ID: "crew", District: "home", Profession: "worker", ProfLevel: 1, Population: pop, AvgEnergy: 100, ItemPool: map[string]int64{}}}
		group.runCohortProduction(&group.Cohorts[0])
		wantQty, wantCredit := int64(4), int64(0)
		if pop == 7 {
			wantQty, wantCredit = 5, 60
		}
		if len(group.Growth) != 1 || group.Growth[0].Quantity != wantQty || group.yieldCredit("cohort", "crew", "draw") != wantCredit || group.DistrictStates["home"].Resources["freshwater"] != int64(20-pop) {
			t.Fatalf("cohort of %d did not match one-person batches: qty=%d credit=%d resource=%d", pop, group.Growth[0].Quantity, group.yieldCredit("cohort", "crew", "draw"), group.DistrictStates["home"].Resources["freshwater"])
		}
		group.Tick = 2
		group.AdvanceGrowth()
		if group.Cohorts[0].ItemPool["water"] != wantQty {
			t.Fatalf("cohort of %d matured a different quantity: %d", pop, group.Cohorts[0].ItemPool["water"])
		}
	}
}

func lawEnvDrawWorld(stock int64, featureYieldPct int) *World {
	w := lawWorld()
	w.Map.Districts[0].Features = append(w.Map.Districts[0].Features, "well")
	w.Catalog.Calendar.Seasons[0].FeatureYieldPct = map[string]int{"well": featureYieldPct}
	w.Catalog.Resources["freshwater"] = ResourcePoolSpec{ID: "freshwater", PotableWater: true}
	w.Catalog.Recipes = append(w.Catalog.Recipes, Recipe{
		ID: "draw", Profession: "worker", RequiredTag: "person", NodeFeature: "well", Ticks: 2, EnergyCost: 5,
		ResourcePool: "freshwater", ResourceDraw: 1, OutputItem: "water", OutputQty: 2,
		Conversion: &MaterialConversion{ExternalMassUnits: 2, Source: "fixture: one freshwater unit funds at most two water units"},
	})
	w.DistrictStates = map[string]DistrictState{"home": {Resources: map[string]int64{"freshwater": stock}, MaxResources: map[string]int64{"freshwater": 100}}}
	return w
}

func lawFinishDraw(t *testing.T, w *World) {
	t.Helper()
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "draw"}))
	w.Tick++
	lawOK(t, w.Advance("a"))
}

// P2: readiness changes biology, not land rights. The field stays occupied until harvest.
func TestLawProductionFieldOccupationEndsAtHarvest(t *testing.T) {
	w := lawWorld()
	r := lawCrop()
	w.Catalog.Recipes = []Recipe{r}
	w.Production.Fields = []FieldAsset{{ID: "field", District: "home", AreaM2: 1000, OwnerKind: "actor", OwnerID: "a", UserKind: "actor", UserID: "a"}}
	w.Actors[0].Items = map[string]int{"seed": 2, "water": 4}
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "crop"}))
	if len(w.Growth) != 1 || w.Growth[0].AreaM2 != 1000 {
		t.Fatal("exact authorized field did not fund one crop")
	}
	for tick := int64(1); tick <= 2; tick++ {
		w.Tick = tick
		w.AdvanceGrowth()
	}
	if !w.Growth[0].Ready || w.Actors[0].Items["grain"] != 0 {
		t.Fatal("maturity must precede physical harvest")
	}
	w.Tick++
	lawRejected(t, w.Apply("a", Action{Kind: "craft", RecipeID: "crop"}))
	if w.Actors[0].Items["seed"] != 1 || len(w.Growth) != 1 {
		t.Fatal("standing mature crop released land or spent seed")
	}
	lawRejected(t, w.Apply("b", Action{Kind: "harvest", TargetID: fmt.Sprint(w.Growth[0].ID)}))
	if w.Actors[1].Items["grain"] != 0 {
		t.Fatal("stranger harvested another person's crop")
	}
	lawOK(t, w.Apply("a", Action{Kind: "harvest", TargetID: fmt.Sprint(w.Growth[0].ID)}))
	if w.Actors[0].Items["grain"] != 3 || len(w.Growth) != 0 {
		t.Fatal("local authorized harvest failed to transfer standing output once")
	}
	w.Tick++
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "crop"}))
	if len(w.Growth) != 1 || w.Actors[0].Items["seed"] != 0 {
		t.Fatal("harvest did not free the same field for a second crop")
	}
}

func TestLawProductionCareRequiresConsentSuppliesAndOneWorkerPerTick(t *testing.T) {
	for _, condition := range []string{"absent_consent", "no_water", "busy", "available"} {
		t.Run(condition, func(t *testing.T) {
			w := lawWorld()
			r := lawCrop()
			w.Catalog.Recipes = []Recipe{r}
			w.Actors[0].Location = "away"
			w.Actors[1].Items["water"] = 2
			for id := int64(20); id < 22; id++ {
				w.Growth = append(w.Growth, GrowthBatch{ID: id, OwnerID: "a", District: "home", RecipeID: "crop", Rule: *r.Growth, Batches: 1, Quantity: 3, Item: "grain"})
				if condition != "absent_consent" {
					err := w.SetGrowthCare(GrowthCareAgreement{GrowthID: id, OwnerKind: "actor", OwnerID: "a", CaretakerKind: "actor", CaretakerID: "b", SupplierKind: "actor", SupplierID: "b", Harvest: true, OwnerAccepted: true, CaretakerAccepted: true})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if condition == "no_water" {
				w.Actors[1].Items["water"] = 0
			}
			if condition == "busy" {
				w.Actors[1].Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 2}
			}
			before := w.Actors[1].Items["water"]
			w.Tick = 1
			w.AdvanceGrowth()
			progress := w.Growth[0].ProgressTicks + w.Growth[1].ProgressTicks
			if condition == "available" {
				if progress != 1 || before-w.Actors[1].Items["water"] != 1 {
					t.Fatal("one available worker did not fund exactly one supplied care tick")
				}
			} else if progress != 0 || w.Actors[1].Items["water"] != before {
				t.Fatalf("unfunded or unauthorized care advanced: %+v", w.Growth)
			}
			w.AdvanceGrowth()
			if w.Growth[0].ProgressTicks+w.Growth[1].ProgressTicks != progress || w.Actors[1].Items["water"] != before-int(progress) {
				t.Fatal("repeated settlement used the same worker/tick twice")
			}
		})
	}
}

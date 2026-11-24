package sim

func (w *World) initialCropPeriodRations(population int) int {
	r, _ := w.Catalog.RecipeByID("farm_grain")
	return population * int(nutritionNeeded(r.Growth.DurationTicks+int64(r.Ticks)))
}

// The prefecture is an established retail economy. Household stores cover
// the daily resupply cycle and the journey to the nearest stocked supplier.
// Crop-period reserves owned by the cohort stay with the supplier.
func (w *World) householdFood(a Actor) (int, *PopulationCohort) {
	ticks := w.Catalog.EffectivePlanningRules().HouseholdSupplyTicks
	if ticks == 0 {
		return 0, nil
	}
	p := Perception{Self: a, Catalog: w.Catalog, Map: w.Map, Tick: w.Tick}
	var supplier *PopulationCohort
	var travel int64
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population == 0 || c.ItemPool["food"] < int64(w.Catalog.Items["food"].BatchQty) {
			continue
		}
		if c.District == a.Location {
			supplier, travel = c, 0
			break
		}
		if route, ok := p.PlanRoute(c.District, ""); ok && (supplier == nil || travel > route.Ticks) {
			supplier, travel = c, route.Ticks
		}
	}
	form, _ := w.Catalog.ActorFormSpec(a)
	return int(nutritionNeededForForm(ticks+2*travel, form)), supplier
}

func (w *World) seedHouseholdFood() {
	for i := range w.Actors {
		a := &w.Actors[i]
		keep, supplier := w.householdFood(*a)
		if excess := a.Items["food"] - keep; supplier != nil && excess > 0 {
			w.adjustActorItem(a, "food", -excess, false)
			w.adjustCohortItem(supplier, "food", int64(excess), false)
		}
	}
}

// The established village starts with last season's grain and a crop already
// halfway through its cycle. Stock units are meals; one plot feeds 30 / 12
// residents for 120 days at three nutrition units per day. These are scenario assumptions.
func (w *World) initVillageFoodCycle() {
	w.InitProductionAssets()
	r, _ := w.Catalog.RecipeByID("farm_grain")
	cycleDays := int(r.Growth.DurationTicks / TicksPerDay)
	for i := range w.Actors {
		a := &w.Actors[i]
		w.adjustActorItem(a, "food", cycleDays*int(DailyNutritionUnits)-a.Items["food"], false)
		if a.Role != "farmer" {
			continue
		}
		w.startGrowth(a.ID, "south_fields", "", false, r, 1, int64(r.OutputQty), nil)
		g := &w.Growth[len(w.Growth)-1]
		g.ProgressTicks = r.Growth.DurationTicks / 2
		g.CareUntil = g.ProgressTicks
		g.PreparationUntil = 0
	}
}

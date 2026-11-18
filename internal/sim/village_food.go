package sim

func (w *World) initialCropPeriodRations(population int) int {
	r, _ := w.Catalog.RecipeByID("farm_grain")
	return population * int((r.Growth.DurationTicks+int64(r.Ticks)+23)/24)
}

// The established village starts with last season's grain and a crop already
// halfway through its cycle. Stock units are meals; one plot feeds 30 / 12
// residents for 120 days at four meals per day. These are scenario assumptions.
func (w *World) initVillageFoodCycle() {
	w.InitProductionAssets()
	r, _ := w.Catalog.RecipeByID("farm_grain")
	cycleDays := int(r.Growth.DurationTicks / TicksPerDay)
	for i := range w.Actors {
		a := &w.Actors[i]
		a.Items["food"] = cycleDays * 4
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

package sim

func (w *World) cohortFoodRelief(c PopulationCohort) int64 {
	form, _ := w.cohortFormSpec(c)
	var relief int64
	for item, quantity := range c.ItemPool {
		spec := w.Catalog.Items[item]
		if form.AcceptsFoodItem(spec) {
			relief += quantity * spec.NutritionUnits
		}
	}
	return relief
}

func nutritionNeededForForm(ticks int64, form FormSpec) int64 {
	if form.IsHungerDisabled() {
		return 0
	}
	daily := DailyNutritionForForm(form)
	if daily <= 0 {
		return 0
	}
	return ticks/TicksPerDay*daily + (ticks%TicksPerDay*daily+TicksPerDay-1)/TicksPerDay
}

// Keep subsistence until the next crop is due. If no crop is growing, a farmer
// still needs the full declared crop period before replacement food can exist.
func (w *World) cohortFoodHorizon(c PopulationCohort) int64 {
	horizon := int64(TicksPerDay)
	profession, _ := w.Catalog.ProfessionByID(c.Profession)
	guildLevel := 1
	if guild := w.faction(c.FactionID); guild != nil {
		guildLevel = guild.Level
	}
	for _, r := range w.Catalog.Recipes {
		if cohortMeetsRecipeTraining(w.Catalog, c, profession, r) && guildLevel >= r.MinFactionLevel && r.Growth != nil && w.Catalog.Items[r.OutputItem].Edible {
			if r.NodeFeature == "farmland" {
				hasPlot := false
				for _, field := range w.Production.Fields {
					if field.UserKind == "cohort" && field.UserID == c.ID && field.District == c.District && field.AreaM2 >= w.Catalog.Production.FieldAreaPerBatchM2 {
						hasPlot = true
						break
					}
				}
				if !hasPlot {
					continue
				}
			}
			horizon = max(horizon, r.Growth.DurationTicks+int64(r.Ticks))
		}
	}
	for _, g := range w.Growth {
		if g.Cohort && g.OwnerID == c.ID && !g.Harvested && w.Catalog.Items[g.Item].Edible {
			horizon = min(horizon, max(int64(1), g.Rule.DurationTicks-g.ProgressTicks))
		}
	}
	return horizon
}

func (w *World) cohortFoodSurplus(c PopulationCohort, item string) int64 {
	form, _ := w.cohortFormSpec(c)
	needed := int64(w.anonymousCohort(c).Population) * nutritionNeededForForm(w.cohortFoodHorizon(c), form)
	for _, b := range c.Residents {
		held := b.NutritionReserve
		for id, qty := range b.Items {
			if form.AcceptsFoodItem(w.Catalog.Items[id]) {
				held += int64(qty) * w.Catalog.Items[id].NutritionUnits
			}
		}
		needed += int64(b.Count) * (int64(b.Hunger) - held)
	}
	needed = max(int64(0), needed)
	excess := max(int64(0), w.cohortFoodRelief(c)-needed)
	return min(c.ItemPool[item], excess/max(int64(1), w.Catalog.Items[item].NutritionUnits))
}

// ActorFormSpec returns the actor's FormSpec if declared in Catalog.Forms.
func (c Catalog) ActorFormSpec(a Actor) (FormSpec, bool) {
	if a.FormID == "" || c.Forms == nil {
		return FormSpec{}, false
	}
	f, ok := c.Forms[a.FormID]
	return f, ok
}

// ActorIsHungerDisabled reports whether the actor's form disables food hunger (e.g. fasting).
func (c Catalog) ActorIsHungerDisabled(a Actor) bool {
	if f, ok := c.ActorFormSpec(a); ok {
		return f.IsHungerDisabled()
	}
	return false
}

// ActorAcceptsFood reports whether spec is edible and meets the actor form's minimum food tier.
func (c Catalog) ActorAcceptsFood(a Actor, spec ItemSpec) bool {
	if f, ok := c.ActorFormSpec(a); ok {
		return f.AcceptsFoodItem(spec)
	}
	return spec.Edible && spec.NutritionUnits > 0
}

// ActorDailyNutrition returns the daily nutrition units required by the actor's form.
func (c Catalog) ActorDailyNutrition(a Actor) int64 {
	if f, ok := c.ActorFormSpec(a); ok {
		return DailyNutritionForForm(f)
	}
	return DailyNutritionUnits
}

// AvailableNutrition counts held edible items accepted by the actor's form plus absorbed surplus.
// Item quantities and prices remain in their original units.
func (c Catalog) AvailableNutrition(a Actor) int64 {
	total := a.NutritionReserve
	for item, q := range a.Items {
		if c.ActorAcceptsFood(a, c.Items[item]) {
			total += int64(q) * c.Items[item].NutritionUnits
		}
	}
	return total
}

func (w *World) incomingNutrition(cohortID string) int64 {
	var total int64
	for item, spec := range w.Catalog.Items {
		if spec.Edible {
			total += w.incomingStock(cohortID, item) * spec.NutritionUnits
		}
	}
	return total
}

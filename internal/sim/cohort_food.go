package sim

func (w *World) cohortFoodRelief(c PopulationCohort) int64 {
	var relief int64
	for item, quantity := range c.ItemPool {
		spec := w.Catalog.Items[item]
		if spec.Edible {
			relief += quantity * spec.NutritionUnits
		}
	}
	return relief
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
	needed := int64(w.anonymousCohort(c).Population) * nutritionNeeded(w.cohortFoodHorizon(c))
	for _, b := range c.Residents {
		held := b.NutritionReserve
		for id, qty := range b.Items {
			if w.Catalog.Items[id].Edible {
				held += int64(qty) * w.Catalog.Items[id].NutritionUnits
			}
		}
		needed += int64(b.Count) * (int64(b.Hunger) - held)
	}
	needed = max(int64(0), needed)
	excess := max(int64(0), w.cohortFoodRelief(c)-needed)
	return min(c.ItemPool[item], excess/max(int64(1), w.Catalog.Items[item].NutritionUnits))
}

// AvailableNutrition counts held edible items plus already absorbed surplus.
// Item quantities and prices remain in their original units.
func (c Catalog) AvailableNutrition(a Actor) int64 {
	total := a.NutritionReserve
	for item, q := range a.Items {
		if c.Items[item].Edible {
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

package sim

func (w *World) cohortFoodRelief(c PopulationCohort) int64 {
	var relief int64
	for item, quantity := range c.ItemPool {
		spec := w.Catalog.Items[item]
		if spec.Edible {
			relief += quantity * int64(max(1, spec.HungerRelief))
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
	needed := int64(w.anonymousCohort(c).Population) * ((w.cohortFoodHorizon(c) + 23) / 24)
	excess := max(int64(0), w.cohortFoodRelief(c)-needed)
	return min(c.ItemPool[item], excess/int64(max(1, w.Catalog.Items[item].HungerRelief)))
}

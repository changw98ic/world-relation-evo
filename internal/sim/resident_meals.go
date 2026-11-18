package sim

// A meal range contains only residents with identical intake and outcomes.
// First and count identify consecutive residents even after a death or transfer.
type residentMealRange struct {
	Origin       string `json:"origin"`
	First        int    `json:"first"`
	Count        int    `json:"count"`
	FirstID      string `json:"first_id"`
	Item         string `json:"item,omitempty"`
	Intake       int    `json:"units_per_person"`
	Reason       string `json:"reason,omitempty"`
	HungerBefore int    `json:"hunger_before_meal"`
	HungerAfter  int    `json:"hunger_after_meal"`
	HealthBefore int    `json:"health_before"`
	HealthAfter  int    `json:"health_after"`
}

// Cohorts advance before World.Step increments its clock. physiologyTick is the
// same end-of-step clock that exact members use for hunger and damage.
func (w *World) settleResidentMeals(c *PopulationCohort, edible []string, physiologyTick int64) []Event {
	population := w.anonymousCohort(*c).Population
	if population == 0 {
		return nil
	}
	c.ensureResidents(population)
	increment := hungerIncrement(physiologyTick, w.Catalog.Lifecycle, FormSpec{})
	if increment == 0 {
		hungry := false
		for _, b := range c.Residents {
			if b.Hunger > 0 {
				hungry = true
				break
			}
		}
		if !hungry {
			return nil
		}
	}
	damageDue := physiologyTick%(6*TicksPerHour) == 0

	// Step 1: advance hunger and count residents who are hungry without personal food.
	// Never allocate scarce ItemPool stock by slice order.
	var needy int64
	for i := range c.Residents {
		c.Residents[i].Hunger += increment
		if c.Residents[i].Hunger > 0 && residentOwnedEdibleItem(c.Residents[i], edible) == "" {
			needy += int64(c.Residents[i].Count)
		}
	}

	// Uniform pool distribution applies only when a single edible item covers every
	// resident who lacks personal stock, preventing storage-order favoritism.
	poolRationItem := ""
	if needy > 0 {
		for _, itemID := range edible {
			if c.ItemPool[itemID] >= needy {
				poolRationItem = itemID
				w.adjustCohortItem(c, itemID, -needy, true)
				break
			}
		}
	}

	var survivors []ResidentBand
	var meals, fatalities []residentMealRange
	var hunger int64
	var dead, wounded int
	for _, b := range c.Residents {
		part := cloneResidentBand(b)
		r := residentMealRange{Origin: b.Origin, First: b.First, Count: b.Count, FirstID: b.ID(0), HungerBefore: b.Hunger, HealthBefore: b.Health}
		if b.Hunger == 0 {
			r.Reason = "not_hungry"
		} else if owned := residentOwnedEdibleItem(part, edible); owned != "" {
			r.Item, r.Intake = owned, 1
			w.adjustResidentBandItem(&part, owned, -1, true)
			part.Hunger = hungerAfterRation(b.Hunger, w.Catalog.Items[owned].HungerRelief)
			c.DigestedFood += int64(part.Count)
		} else if poolRationItem != "" {
			r.Item, r.Intake = poolRationItem, 1
			part.Hunger = hungerAfterRation(b.Hunger, w.Catalog.Items[poolRationItem].HungerRelief)
			c.DigestedFood += int64(part.Count)
		} else {
			r.Reason = "no_edible_stock"
		}
		if damageDue {
			part.Health = max(0, part.Health-starvationDamage(part.Hunger, w.Catalog.Lifecycle, FormSpec{}))
		}
		r.HungerAfter, r.HealthAfter = part.Hunger, part.Health
		meals = append(meals, r)
		if part.Health == 0 {
			dead += part.Count
			fatalities = append(fatalities, r)
		} else {
			survivors = appendResidentBand(survivors, part)
			hunger += int64(part.Count) * int64(part.Hunger)
			if b.Health == MaxHealth && part.Health < MaxHealth {
				wounded += part.Count
			}
		}
	}
	if dead > 0 || wounded > 0 {
		// The census accounts for these casualties. The meal settlement owns the
		// identities and hunger of the survivors.
		wounded, _, _ = w.applyCohortCasualtiesWithCause(c, wounded, dead, 0, "starvation")
	}
	events := []Event{{
		ID: w.NextID, Tick: w.Tick, ActorID: c.ID, Kind: "cohort_meals_settled",
		Action: Action{Kind: "eat", Location: c.District},
		Payload: map[string]any{
			"source_kind": "cohort", "source_id": c.ID, "physiology_tick": physiologyTick,
			"ranges": meals, "wounded": wounded,
		},
	}}
	w.NextID++
	if dead > 0 {
		events = append(events, Event{
			ID: w.NextID, Tick: w.Tick, ActorID: c.ID, Kind: "cohort_famine_deaths",
			Action: Action{Kind: "wait", Location: c.District},
			Payload: map[string]any{
				"source_kind": "cohort", "source_id": c.ID, "physiology_tick": physiologyTick,
				"cohort_id": c.ID, "cause": "starvation", "deaths": dead, "ranges": fatalities,
			},
		})
		w.NextID++
	}
	c.Residents = survivors
	c.setHungerUnits(population-dead, hunger)
	return events
}

func residentOwnedEdibleItem(b ResidentBand, edible []string) string {
	for _, itemID := range edible {
		if b.Items[itemID] > 0 {
			return itemID
		}
	}
	return ""
}

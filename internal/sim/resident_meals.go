package sim

import "sort"

// A meal range contains only residents with identical intake and outcomes.
// First and count identify consecutive residents even after a death or transfer.
type residentMealRange struct {
	Origin          string `json:"origin"`
	First           int    `json:"first"`
	Count           int    `json:"count"`
	FirstID         string `json:"first_id"`
	Item            string `json:"item,omitempty"`
	Intake          int    `json:"units_per_person"`
	Reason          string `json:"reason,omitempty"`
	HungerBefore    int    `json:"hunger_before_meal"`
	HungerAfter     int    `json:"hunger_after_meal"`
	NutritionBefore int64  `json:"nutrition_reserve_before"`
	NutritionAfter  int64  `json:"nutrition_reserve_after"`
	HealthBefore    int    `json:"health_before"`
	HealthAfter     int    `json:"health_after"`
}

// Cohorts advance before World.Step increments its clock. physiologyTick is the
// same end-of-step clock that exact members use for hunger and damage.
func (w *World) settleResidentMeals(c *PopulationCohort, edible []string, physiologyTick int64) []Event {
	var nutritious []string
	for _, id := range edible {
		if spec := w.Catalog.Items[id]; spec.Edible && spec.NutritionUnits > 0 {
			nutritious = append(nutritious, id)
		}
	}
	sort.Strings(nutritious)
	edible = nutritious
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

	// Allocation is by need then stable identity, never storage order. Bands split
	// only at stock boundaries; a million identical residents remain one range.
	bands := append([]ResidentBand(nil), c.Residents...)
	for i := range bands {
		advanceNutrition(&bands[i].Hunger, &bands[i].NutritionReserve, increment)
	}
	sort.Slice(bands, func(i, j int) bool {
		if bands[i].Hunger != bands[j].Hunger {
			return bands[i].Hunger > bands[j].Hunger
		}
		if bands[i].Origin != bands[j].Origin {
			return bands[i].Origin < bands[j].Origin
		}
		return bands[i].First < bands[j].First
	})
	var survivors []ResidentBand
	var meals, fatalities []residentMealRange
	var hunger int64
	var dead, wounded int
	settle := func(part ResidentBand, item string, personal bool) {
		r := residentMealRange{Origin: part.Origin, First: part.First, Count: part.Count, FirstID: part.ID(0), HungerBefore: part.Hunger, HealthBefore: part.Health, NutritionBefore: part.NutritionReserve}
		if part.Hunger == 0 {
			r.Reason = "not_hungry"
		} else if item == "" {
			r.Reason = "no_edible_stock"
		} else {
			r.Item, r.Intake = item, 1
			if personal {
				w.adjustResidentBandItem(&part, item, -1, true)
			} else {
				w.adjustCohortItem(c, item, -int64(part.Count), true)
			}
			absorbNutrition(&part.Hunger, &part.NutritionReserve, w.Catalog.Items[item].NutritionUnits)
			c.DigestedFood += int64(part.Count)
		}
		if damageDue {
			part.Health = max(0, part.Health-starvationDamage(part.Hunger, w.Catalog.Lifecycle, FormSpec{}))
		}
		r.HungerAfter, r.HealthAfter, r.NutritionAfter = part.Hunger, part.Health, part.NutritionReserve
		meals = append(meals, r)
		if part.Health == 0 {
			dead += part.Count
			fatalities = append(fatalities, r)
		} else {
			survivors = append(survivors, part)
			hunger += int64(part.Count) * int64(part.Hunger)
			if r.HealthBefore == MaxHealth && part.Health < MaxHealth {
				wounded += part.Count
			}
		}
	}
	for _, b := range bands {
		if b.Hunger == 0 {
			settle(b, "", false)
			continue
		}
		if item := residentOwnedEdibleItem(b, edible); item != "" {
			settle(cloneResidentBand(b), item, true)
			continue
		}
		for _, item := range edible {
			take := int(min(int64(b.Count), c.ItemPool[item]))
			if take <= 0 {
				continue
			}
			part := cloneResidentBand(b)
			part.Count = take
			settle(part, item, false)
			b.First += take
			b.Count -= take
			if b.Count == 0 {
				break
			}
		}
		if b.Count > 0 {
			settle(b, "", false)
		}
	}
	sort.Slice(survivors, func(i, j int) bool {
		if survivors[i].Origin != survivors[j].Origin {
			return survivors[i].Origin < survivors[j].Origin
		}
		return survivors[i].First < survivors[j].First
	})
	var compact []ResidentBand
	for _, b := range survivors {
		compact = appendResidentBand(compact, b)
	}
	survivors = compact

	sort.Slice(meals, func(i, j int) bool {
		if meals[i].Origin != meals[j].Origin {
			return meals[i].Origin < meals[j].Origin
		}
		return meals[i].First < meals[j].First
	})
	sort.Slice(fatalities, func(i, j int) bool {
		if fatalities[i].Origin != fatalities[j].Origin {
			return fatalities[i].Origin < fatalities[j].Origin
		}
		return fatalities[i].First < fatalities[j].First
	})
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

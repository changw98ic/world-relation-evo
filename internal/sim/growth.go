package sim

import (
	"fmt"
	"sort"
	"strconv"
)

// GrowthRule describes a configured biological or maturation process.
// Durations are simulated ticks. Supplies are required per batch per maintenance interval.
type GrowthRule struct {
	DurationTicks            int64          `json:"duration_ticks"`
	MaintenanceIntervalTicks int64          `json:"maintenance_interval_ticks"`
	Supplies                 map[string]int `json:"supplies,omitempty"`
	ResourcePool             string         `json:"resource_pool,omitempty"`
	ResourceDraw             int64          `json:"resource_draw,omitempty"`
	Parents                  map[string]int `json:"parents,omitempty"`
	Source                   string         `json:"source"`
}

type GrowthBatch struct {
	Workers          int              `json:"workers,omitempty"`
	PreparationUntil int64            `json:"preparation_until,omitempty"`
	ID               int64            `json:"id"`
	OwnerID          string           `json:"owner_id"`
	Cohort           bool             `json:"cohort"`
	District         string           `json:"district"`
	VenueID          string           `json:"venue_id,omitempty"`
	RecipeID         string           `json:"recipe_id"`
	Rule             GrowthRule       `json:"rule"`
	Batches          int64            `json:"batches"`
	Item             string           `json:"item"`
	Quantity         int64            `json:"quantity"`
	Parents          map[string]int64 `json:"parents,omitempty"`
	ProgressTicks    int64            `json:"progress_ticks"`
	CareUntil        int64            `json:"care_until"`
	LastTick         int64            `json:"last_tick"`
	Ready            bool             `json:"ready"`
	Stalled          bool             `json:"stalled"`
}

func cloneGrowthRule(r *GrowthRule) *GrowthRule {
	if r == nil {
		return nil
	}
	c := *r
	c.Supplies = cloneIntMap(r.Supplies)
	c.Parents = cloneIntMap(r.Parents)
	return &c
}

func (w *World) startGrowth(owner, district, venue string, cohort bool, r Recipe, batches, quantity int64, parents map[string]int64) Event {
	id := w.NextID
	w.NextID++
	rule := cloneGrowthRule(r.Growth)
	kind := "growth_started"
	lastTick := w.Tick
	if rule == nil {
		rule = &GrowthRule{DurationTicks: int64(r.Ticks), MaintenanceIntervalTicks: int64(r.Ticks), Source: "configured recipe work duration"}
		kind = "production_started"
	} else if cohort {
		lastTick += int64(r.Ticks)
	}
	w.Growth = append(w.Growth, GrowthBatch{ID: id, OwnerID: owner, Cohort: cohort, District: district, VenueID: venue, RecipeID: r.ID, Rule: *rule, Batches: batches, Item: r.OutputItem, Quantity: quantity, Parents: parents, LastTick: lastTick, PreparationUntil: w.Tick + int64(r.Ticks)})
	return Event{ID: id, Tick: w.Tick, ActorID: owner, Kind: kind, Action: Action{Kind: "craft", RecipeID: r.ID}, Payload: map[string]any{"growth_id": id, "duration_ticks": rule.DurationTicks, "batches": batches}}

}

func (w *World) growthOwnerPresent(g GrowthBatch) bool {
	if g.Cohort {
		c := w.findCohort(g.OwnerID)
		return c != nil && c.Population > 0 && c.District == g.District
	}
	a := w.actor(g.OwnerID)
	return a != nil && !a.Dead && a.Location == g.District && (g.VenueID == "" || a.VenueID == g.VenueID)
}

func (w *World) growthStock(g GrowthBatch, item string) int64 {
	if g.Cohort {
		return w.findCohort(g.OwnerID).ItemPool[item]
	}
	return int64(w.actor(g.OwnerID).Items[item])
}
func (w *World) changeGrowthStock(g GrowthBatch, item string, delta int64) {
	if g.Cohort {
		c := w.findCohort(g.OwnerID)
		if c.ItemPool == nil {
			c.ItemPool = map[string]int64{}
		}
		c.ItemPool[item] += delta
		if c.ItemPool[item] == 0 {
			delete(c.ItemPool, item)
		}
	} else {
		a := w.actor(g.OwnerID)
		if a.Items == nil {
			a.Items = map[string]int{}
		}
		a.Items[item] += int(delta)
		if a.Items[item] == 0 {
			delete(a.Items, item)
		}
	}
}

// AdvanceGrowth accounts for elapsed time and each maintenance interval separately.
// Missing local supplies pause progress; paid intervals are never consumed twice after a restart.
func (w *World) AdvanceGrowth() []Event {
	var events []Event
	for i := range w.Growth {
		g := &w.Growth[i]
		elapsed := w.Tick - g.LastTick
		if elapsed <= 0 {
			continue
		}
		g.LastTick = w.Tick
		if g.Ready {
			continue
		}
		for elapsed > 0 && g.ProgressTicks < g.Rule.DurationTicks {
			if g.ProgressTicks >= g.CareUntil {
				supplied := w.growthOwnerPresent(*g)
				if supplied {
					for item, qty := range g.Rule.Supplies {
						if w.growthStock(*g, item) < int64(qty)*g.Batches {
							supplied = false
						}
					}
					if g.Rule.ResourcePool != "" && w.availableDistrictResource(g.District, g.Rule.ResourcePool) < g.Rule.ResourceDraw*g.Batches {
						supplied = false
					}
				}
				if !supplied {
					if !g.Stalled {
						events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "growth_stalled", Action: Action{Kind: "wait"}, Payload: map[string]any{"growth_id": g.ID, "reason": "local care supplies unavailable"}})
						w.NextID++
					}
					g.Stalled = true
					break
				}
				for item, qty := range g.Rule.Supplies {
					amount := int64(qty) * g.Batches
					w.changeGrowthStock(*g, item, -amount)
					w.recordLedgerItemConsumed(item, amount)
				}
				if g.Rule.ResourcePool != "" {
					w.drawDistrictResource(g.District, g.Rule.ResourcePool, g.Rule.ResourceDraw*g.Batches)
				}
				g.CareUntil += g.Rule.MaintenanceIntervalTicks
				g.Stalled = false
			}
			advance := min(elapsed, g.CareUntil-g.ProgressTicks, g.Rule.DurationTicks-g.ProgressTicks)
			g.ProgressTicks += advance
			elapsed -= advance
		}
		if g.ProgressTicks == g.Rule.DurationTicks {
			g.Ready = true
			w.recordLedgerItemProduced(g.Item, g.Quantity)
			events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "growth_matured", Action: Action{Kind: "wait", RecipeID: g.RecipeID}, Payload: map[string]any{"growth_id": g.ID, "item": g.Item, "quantity": g.Quantity}})
			w.NextID++
		}
	}
	// Group harvest is local; an individual explicitly selects the harvest action.
	for i := 0; i < len(w.Growth); {
		g := w.Growth[i]
		if g.Cohort && g.Ready && w.growthOwnerPresent(g) {
			events = append(events, w.harvestGrowth(i)...)
			continue
		}
		i++
	}
	return events
}

func (w *World) harvestGrowth(i int) []Event {
	g := w.Growth[i]
	w.changeGrowthStock(g, g.Item, g.Quantity)
	for item, qty := range g.Parents {
		w.changeGrowthStock(g, item, qty)
	}
	w.Growth = append(w.Growth[:i], w.Growth[i+1:]...)
	if len(w.Growth) == 0 {
		w.Growth = nil
	}
	r, _ := w.Catalog.RecipeByID(g.RecipeID)
	if !g.Cohort {
		a := w.actor(g.OwnerID)
		x := Action{Kind: "harvest", TargetID: strconv.FormatInt(g.ID, 10), RecipeID: g.RecipeID, Item: g.Item, Quantity: int(g.Quantity)}
		event := w.record(a, x, "harvested", map[string]any{"growth_id": g.ID, "recipe_id": g.RecipeID, "item": g.Item, "quantity": g.Quantity, "text": fmt.Sprintf("harvested %d %s", g.Quantity, g.Item), "observation": Observation{Kind: "item_produced", Item: g.Item, Location: g.District, Quantity: int(g.Quantity)}})
		return append([]Event{event}, w.grantProfessionXP(a, x, r.Profession, w.Catalog.CraftXP(r), w.Catalog.Items[g.Item].Tier)...)
	}
	c := w.findCohort(g.OwnerID)
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "cohort_crafted", Action: Action{Kind: "craft", RecipeID: g.RecipeID, Item: g.Item, Quantity: int(g.Quantity)}, Payload: map[string]any{"growth_id": g.ID, "item": g.Item, "quantity": g.Quantity, "batches": g.Batches, "recipe_id": g.RecipeID, "cohort_id": g.OwnerID}}
	w.NextID++
	events := []Event{e}
	if g.RecipeID == w.Catalog.Lifecycle.CompostRecipeID {
		events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "nightsoil_composted", Action: Action{Kind: "craft", RecipeID: g.RecipeID, Item: g.Item, Quantity: int(g.Quantity)}, Payload: map[string]any{"item": g.Item, "compost_generated": g.Quantity}})
		w.NextID++
	}
	return append(events, w.addCohortXP(c, w.Catalog.CraftXP(r), maxInt(1, w.Catalog.Items[g.Item].Tier)*c.ProfLevel)...)

}

func (w *World) visibleGrowth(actor Actor) []GrowthBatch {
	var result []GrowthBatch
	for _, g := range w.Growth {
		if !g.Cohort && g.OwnerID == actor.ID {
			g.Rule = *cloneGrowthRule(&g.Rule)
			g.Parents = cloneInt64Map(g.Parents)
			result = append(result, g)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func validateGrowth(r Recipe) error {
	g := r.Growth
	if g == nil {
		return nil
	}
	if g.DurationTicks <= 0 || g.MaintenanceIntervalTicks <= 0 || g.Source == "" {
		return fmt.Errorf("recipe %s growth requires positive duration, maintenance interval and source", r.ID)
	}
	if len(g.Supplies) == 0 && g.ResourcePool == "" {
		return fmt.Errorf("recipe %s growth requires a supply source", r.ID)
	}
	if g.ResourcePool != "" && g.ResourceDraw <= 0 {
		return fmt.Errorf("recipe %s growth resource draw must be positive", r.ID)
	}
	for item, n := range g.Supplies {
		if n <= 0 {
			return fmt.Errorf("recipe %s invalid supply %s", r.ID, item)
		}
	}
	for item, n := range g.Parents {
		if n <= 0 {
			return fmt.Errorf("recipe %s invalid parent stock %s", r.ID, item)
		}
	}
	return nil
}

func recipeStartRequirements(r Recipe) map[string]int {
	if r.Growth == nil || len(r.Growth.Parents) == 0 {
		return r.Inputs
	}
	required := map[string]int{}
	for item, qty := range r.Inputs {
		required[item] += qty
	}
	for item, qty := range r.Growth.Parents {
		required[item] += qty
	}
	return required
}

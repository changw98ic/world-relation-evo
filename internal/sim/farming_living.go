package sim

import "fmt"

// LivingRule is an explicit scenario-level husbandry model. Stock units are
// animals. Supply units are the same units as the catalog inventories. The
// preset's seven-day mortality threshold is a scenario assumption, not a
// veterinary claim. A different species must declare its own rule.
type LivingRule struct {
	IntervalTicks   int64            `json:"interval_ticks"`
	Supplies        map[string]int64 `json:"supplies"`
	ResourcePool    string           `json:"resource_pool,omitempty"`
	ResourceDraw    int64            `json:"resource_draw,omitempty"`
	DeathAfterTicks int64            `json:"death_after_ticks"`
	Source          string           `json:"source"`
}

func DefaultCattleLivingRule() *LivingRule {
	return &LivingRule{IntervalTicks: TicksPerDay, Supplies: map[string]int64{"water": 1}, ResourcePool: "pasture_grass", ResourceDraw: 2, DeathAfterTicks: 7 * TicksPerDay, Source: "scenario husbandry budget: each animal drinks one water and grazes two grass daily; seven unfed days exhaust survival budget, not veterinary calibration"}
}

type LivingCareState struct {
	HolderKind        string `json:"holder_kind"`
	HolderID          string `json:"holder_id"`
	Item              string `json:"item"`
	LastTick          int64  `json:"last_tick"`
	ShortageTicks     int64  `json:"shortage_ticks"`
	ShortageRemainder int64  `json:"shortage_remainder,omitempty"`
}

// carryLivingCare preserves elapsed maintenance and shortage debt when existing
// animals move between inventory and a production batch. Remaining stock keeps
// its own record; a split does not reset either group's clock.
func (w *World) carryLivingCare(from, to string, holder GrowthBatch, item string) {
	if w.Catalog.Items[item].Living == nil {
		return
	}
	state, ok := w.Production.LivingCare[from]
	if !ok {
		state = LivingCareState{Item: item, LastTick: w.Tick}
	}
	state.HolderKind, state.HolderID = holderKind(holder.Cohort), holder.OwnerID
	if existing, ok := w.Production.LivingCare[to]; ok {
		state.LastTick = min(state.LastTick, existing.LastTick)
		if existing.ShortageTicks > state.ShortageTicks || existing.ShortageTicks == state.ShortageTicks && existing.ShortageRemainder > state.ShortageRemainder {
			state.ShortageTicks, state.ShortageRemainder = existing.ShortageTicks, existing.ShortageRemainder
		}
	}
	if w.Production.LivingCare == nil {
		w.Production.LivingCare = map[string]LivingCareState{}
	}
	w.Production.LivingCare[to] = state
}

// maintainLivingStocks covers idle stock, retained breeding parents and mature
// live output still at the production site. The in-progress offspring's survival
// is separately tied to the growth batch's configured FailureAfterTicks.
func (w *World) maintainLivingStocks() []Event {
	var events []Event
	maintain := func(key string, holder GrowthBatch, item string, quantity int64, remove func(int64)) {
		rule := w.Catalog.Items[item].Living
		if quantity <= 0 {
			delete(w.Production.LivingCare, key)
			return
		}
		if rule == nil {
			return
		}
		state, exists := w.Production.LivingCare[key]
		if !exists {
			if w.Production.LivingCare == nil {
				w.Production.LivingCare = map[string]LivingCareState{}
			}
			w.Production.LivingCare[key] = LivingCareState{HolderKind: holderKind(holder.Cohort), HolderID: holder.OwnerID, Item: item, LastTick: w.Tick}
			return
		}
		state.HolderKind, state.HolderID = holderKind(holder.Cohort), holder.OwnerID
		for w.Tick-state.LastTick >= rule.IntervalTicks {
			state.LastTick += rule.IntervalTicks
			supplied := quantity
			if !w.growthOwnerPresent(holder) {
				supplied = 0
			}
			if supplied > 0 {
				for food, n := range rule.Supplies {
					supplied = min(supplied, w.growthStock(holder, food)/n)
				}
			}
			if rule.ResourcePool != "" {
				supplied = min(supplied, w.availableDistrictResource(holder.District, rule.ResourcePool)/rule.ResourceDraw)
			}
			for food, n := range rule.Supplies {
				used := n * supplied
				if used > 0 {
					w.changeGrowthStock(holder, food, -used, true)
				}
			}
			if rule.ResourcePool != "" && supplied > 0 {
				w.drawDistrictResource(holder.District, rule.ResourcePool, rule.ResourceDraw*supplied)
			}
			if supplied == quantity {
				state.ShortageTicks = 0
				state.ShortageRemainder = 0
			} else {
				unfed := rule.IntervalTicks*(quantity-supplied) + state.ShortageRemainder
				state.ShortageTicks += unfed / quantity
				state.ShortageRemainder = unfed % quantity
			}
			if state.ShortageTicks >= rule.DeathAfterTicks {
				remove(quantity)
				events = append(events, w.growthEvent(holder, "livestock_died", map[string]any{"item": item, "quantity": quantity, "reason": "declared feed/water survival budget exhausted"}))
				break
			}
		}
		w.Production.LivingCare[key] = state
	}
	maintainActor := func(a *Actor) {
		if a.Dead {
			return
		}
		holder := GrowthBatch{OwnerID: a.ID, District: a.Location, VenueID: a.VenueID}
		for _, item := range sortedProductionKeys(w.Catalog.Items) {
			quantity := int64(a.Items[item])
			maintain(holderKey("actor", a.ID)+"/"+item, holder, item, quantity, func(n int64) {
				w.adjustActorItem(a, item, -int(n), true)
			})
		}
	}
	for i := range w.Actors {
		maintainActor(&w.Actors[i])
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		for j := range c.Members {
			maintainActor(&c.Members[j])
		}
		holder := GrowthBatch{OwnerID: c.ID, Cohort: true, District: c.District}
		for _, item := range sortedProductionKeys(w.Catalog.Items) {
			quantity := c.ItemPool[item]
			maintain(holderKey("cohort", c.ID)+"/"+item, holder, item, quantity, func(n int64) {
				w.adjustCohortItem(c, item, -n, true)
			})
		}
	}
	for i := range w.Growth {
		g := &w.Growth[i]
		holder := w.growthCaretaker(*g)
		for _, item := range sortedProductionKeys(g.Parents) {
			quantity := g.Parents[item]
			maintain(fmt.Sprintf("growth:%d/parent/%s", g.ID, item), holder, item, quantity, func(n int64) {
				w.adjustGrowthParentItem(g, item, -n, true)
				if !g.Ready {
					g.Quantity = 0
					g.FailedTicks = g.Rule.FailureAfterTicks
					g.ProgressTicks = g.Rule.DurationTicks
				}
			})
		}
		if g.Ready {
			maintain(fmt.Sprintf("growth:%d/output", g.ID), holder, g.Item, g.Quantity, func(n int64) {
				w.adjustGrowthOutputItem(g, -n, true)
			})
		}
	}
	if len(w.Production.LivingCare) == 0 {
		w.Production.LivingCare = nil
	}
	return events
}

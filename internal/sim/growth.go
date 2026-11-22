package sim

import (
	"fmt"
	"sort"
	"strconv"
)

// GrowthRule describes a configured biological or maturation process.
// Durations are simulated ticks. Supplies are required per batch per maintenance interval.
type GrowthRule struct {
	CareWorkersPerBatch      int64          `json:"care_workers_per_batch,omitempty"`
	FailureAfterTicks        int64          `json:"failure_after_ticks,omitempty"`
	DurationTicks            int64          `json:"duration_ticks"`
	MaintenanceIntervalTicks int64          `json:"maintenance_interval_ticks"`
	Supplies                 map[string]int `json:"supplies,omitempty"`
	ResourcePool             string         `json:"resource_pool,omitempty"`
	ResourceDraw             int64          `json:"resource_draw,omitempty"`
	Parents                  map[string]int `json:"parents,omitempty"`
	Source                   string         `json:"source"`
}

type GrowthBatch struct {
	PreparationRemaining int64 `json:"preparation_remaining,omitempty"`
	// LaborBookedTick is one-based so zero means no assignment yet.
	LaborBookedTick int64 `json:"labor_booked_tick,omitempty"`
	// A stored caretaker share retains the original crop and signed terms.
	SourceGrowthID   int64            `json:"source_growth_id,omitempty"`
	CareWork         int64            `json:"care_work,omitempty"`
	HarvestWork      int64            `json:"harvest_work,omitempty"`
	WorkCredit       int64            `json:"work_credit,omitempty"`
	FieldID          string           `json:"field_id,omitempty"`
	AreaM2           int64            `json:"area_m2,omitempty"`
	HouseID          string           `json:"house_id,omitempty"`
	Harvested        bool             `json:"harvested,omitempty"`
	FailedTicks      int64            `json:"failed_ticks,omitempty"`
	EnvironmentTotal int64            `json:"environment_total,omitempty"`
	EnvironmentTicks int64            `json:"environment_ticks,omitempty"`
	FertilizerBonus  int64            `json:"fertilizer_bonus,omitempty"`
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
	fieldID, _ := w.fieldAvailable(holderKind(cohort), owner, district, venue, r)
	houseID, _ := w.productionFacility(district, venue, "", r)
	w.Growth = append(w.Growth, GrowthBatch{ID: id, OwnerID: owner, Cohort: cohort, District: district, VenueID: venue, RecipeID: r.ID, Rule: *rule, Batches: batches, Item: r.OutputItem, Quantity: quantity, Parents: parents, LastTick: lastTick, PreparationUntil: w.Tick + int64(r.Ticks), FieldID: fieldID, AreaM2: batches * w.Catalog.Production.FieldAreaPerBatchM2, HouseID: houseID, FertilizerBonus: quantity - int64(r.OutputQty)*batches})
	holder := GrowthBatch{OwnerID: owner, Cohort: cohort, District: district, VenueID: venue}
	for item := range parents {
		w.carryLivingCare(holderKey(holderKind(cohort), owner)+"/"+item, fmt.Sprintf("growth:%d/parent/%s", id, item), holder, item)
	}
	return Event{ID: id, Tick: w.Tick, ActorID: owner, Kind: kind, Action: Action{Kind: "craft", RecipeID: r.ID}, Payload: map[string]any{"growth_id": id, "duration_ticks": rule.DurationTicks, "batches": batches}}

}

func (w *World) growthOwnerPresent(g GrowthBatch) bool {
	if g.Cohort {
		c := w.findCohort(g.OwnerID)
		return c != nil && c.EffectivePopulation() > 0 && w.Tick >= c.RestUntilTick && c.District == g.District
	}
	a := w.actor(g.OwnerID)
	return a != nil && !a.Dead && !a.InTransit() && w.Tick >= a.JailedUntil && w.Tick >= a.RestUntil && a.Location == g.District && (g.VenueID == "" || a.VenueID == g.VenueID)
}

// An empty caretaker means self-care. A named caretaker supplies only local work
// and materials; ownership, harvest rights and the production site stay unchanged.
func (w *World) growthCaretaker(g GrowthBatch) GrowthBatch {
	if a := w.careAgreement(g); a != nil {
		g.OwnerID, g.Cohort = a.CaretakerID, a.CaretakerKind == "cohort"
	}
	return g
}

func (w *World) growthStock(g GrowthBatch, item string) int64 {
	if g.Cohort {
		return w.findCohort(g.OwnerID).ItemPool[item]
	}
	return int64(w.actor(g.OwnerID).Items[item])
}
func (w *World) changeGrowthStock(g GrowthBatch, item string, delta int64, trackLedger bool) {
	if g.Cohort {
		w.adjustCohortItem(w.findCohort(g.OwnerID), item, delta, trackLedger)
		return
	}
	w.adjustActorItem(w.actor(g.OwnerID), item, int(delta), trackLedger)
}

// AdvanceGrowth preserves paid care, local rights, environmental exposure and
// shortage debt. A mature unharvested crop remains on its occupied field.
func (w *World) AdvanceGrowth() []Event {
	events := w.maintainLivingStocks()
	for i := range w.Growth {
		g := &w.Growth[i]
		if g.PreparationRemaining > 0 {
			if g.LaborBookedTick == w.Tick+1 {
				continue
			}
			r, _ := w.Catalog.RecipeByID(g.RecipeID)
			if w.productionSiteOperational(g.District, g.VenueID, g.HouseID, r) {
				completed := w.completeReservedShift(g, "prepare")
				events = append(events, completed...)
				if len(completed) > 0 {
					g.PreparationRemaining--
				}
			}
			g.PreparationUntil = w.Tick + 1 + g.PreparationRemaining
			g.LastTick = g.PreparationUntil
			continue
		}
		elapsed := w.Tick - g.LastTick
		if elapsed <= 0 {
			continue
		}
		g.LastTick = w.Tick
		if g.Ready {
			continue
		}
		r, _ := w.Catalog.RecipeByID(g.RecipeID)
		if !w.productionSiteOperational(g.District, g.VenueID, g.HouseID, r) {
			if r.Growth == nil {
				g.Stalled = true
				continue
			}
			events = append(events, w.stallGrowth(g, elapsed, "production site unavailable")...)
			if g.ProgressTicks < g.Rule.DurationTicks {
				continue
			}
		}
		if r.Growth == nil {
			care := w.growthCaretaker(*g)
			if !w.growthOwnerPresent(care) {
				g.Stalled = true
				continue
			}
			worker := Actor{Location: g.District, VenueID: g.VenueID, InsideHouse: g.HouseID}
			work := elapsed*int64(100+w.FacilityProductionBonusPct(worker)) + g.WorkCredit
			if g.Workers > 0 {
				completed := w.completeReservedShift(g, "production")
				if len(completed) == 0 {
					g.Stalled = true
					continue
				}
				events = append(events, completed...)
				completed[0].Payload["periods"] = elapsed
				completed[0].Payload["stage_complete"] = g.ProgressTicks+work/100 >= g.Rule.DurationTicks
			}
			elapsed = work / 100
			g.WorkCredit = work % 100
			g.Stalled = false
		}
		for elapsed > 0 && g.ProgressTicks < g.Rule.DurationTicks {
			if g.ProgressTicks >= g.CareUntil {
				care := w.growthCaretaker(*g)
				supplied := w.growthOwnerPresent(care)
				workers := g.Rule.CareWorkersPerBatch * g.Batches
				if r.Growth == nil {
					workers = 0 // The active work reservation above owns this labor.
				}
				if supplied {
					for item, qty := range g.Rule.Supplies {
						if w.growthStock(care, item) < int64(qty)*g.Batches {
							supplied = false
						}
					}
					if g.Rule.ResourcePool != "" && w.availableDistrictResource(g.District, g.Rule.ResourcePool) < g.Rule.ResourceDraw*g.Batches {
						supplied = false
					}
				}
				if supplied && workers > g.CareWork {
					work, completed := w.completeGrowthWork(*g, care, "care", workers-g.CareWork)
					events = append(events, completed...)
					g.CareWork += work
					supplied = g.CareWork == workers
				}
				if !supplied {
					events = append(events, w.stallGrowth(g, elapsed, "local labor, agreement or supplies unavailable")...)
					break
				}
				g.CareWork = 0
				for item, qty := range g.Rule.Supplies {
					amount := int64(qty) * g.Batches
					w.changeGrowthStock(care, item, -amount, true)
				}
				if g.Rule.ResourcePool != "" {
					w.drawDistrictResource(g.District, g.Rule.ResourcePool, g.Rule.ResourceDraw*g.Batches)
				}
				g.CareUntil += g.Rule.MaintenanceIntervalTicks
				g.Stalled = false
				g.FailedTicks = 0
			}
			advance := min(elapsed, g.CareUntil-g.ProgressTicks, g.Rule.DurationTicks-g.ProgressTicks)
			if advance <= 0 {
				break
			}
			if r.Growth != nil {
				g.EnvironmentTotal += advance * int64(w.EcologicalYieldMultiplierPct(g.District, r))
				g.EnvironmentTicks += advance
			}
			g.ProgressTicks += advance
			elapsed -= advance
		}
		if g.ProgressTicks == g.Rule.DurationTicks {
			if g.EnvironmentTicks > 0 && g.Quantity > 0 {
				numerator := min(g.EnvironmentTotal, 100*g.EnvironmentTicks)
				denominator := 100 * g.EnvironmentTicks
				fertilized := int64(0)
				if r.BonusOutput > 0 {
					fertilized = g.FertilizerBonus / int64(r.BonusOutput)
				}
				g.Quantity = (g.Batches-fertilized)*(int64(r.OutputQty)*numerator/denominator) + fertilized*(int64(r.OutputQty+r.BonusOutput)*numerator/denominator)
			}
			qty := g.Quantity
			g.Quantity = 0
			g.Ready = true
			w.adjustGrowthOutputItem(g, qty, true)
			if g.Quantity > 0 {
				w.carryLivingCare("", fmt.Sprintf("growth:%d/output", g.ID), w.growthCaretaker(*g), g.Item)
			}
			kind := "growth_matured"
			if r.Growth != nil && g.Quantity == 0 {
				kind = "growth_failed"
			}
			events = append(events, w.growthEvent(*g, kind, map[string]any{"item": g.Item, "quantity": g.Quantity}))
		}
	}
	for i := 0; i < len(w.Growth); {
		g := w.Growth[i]
		care := w.growthCaretaker(g)
		if g.Harvested {
			care = g
		}
		// A finished workshop batch goes directly to its local owner's stock.
		r, _ := w.Catalog.RecipeByID(g.RecipeID)
		if g.Ready && !g.Harvested && g.Workers > 0 && r.Growth == nil && care.Cohort && care.OwnerID == g.OwnerID && w.growthOwnerPresent(g) {
			events = append(events, w.finishHarvest(i)...)
			continue
		}
		if g.Ready && care.Cohort && w.canHarvestGrowth(g, care.OwnerID, true) {
			before := len(w.Growth)
			events = append(events, w.harvestGrowth(i, care)...)
			if len(w.Growth) < before {
				continue
			}
		}
		i++
	}
	events = append(events, w.dispatchCohortHarvests()...)
	return events
}

func (w *World) stallGrowth(g *GrowthBatch, elapsed int64, reason string) []Event {
	var events []Event
	if !g.Stalled {
		events = append(events, w.growthEvent(*g, "growth_stalled", map[string]any{"reason": reason}))
	}
	g.Stalled = true
	g.FailedTicks += elapsed
	if g.Rule.FailureAfterTicks > 0 && g.FailedTicks >= g.Rule.FailureAfterTicks {
		g.Quantity = 0
		g.ProgressTicks = g.Rule.DurationTicks
	}
	return events
}

func (w *World) harvestGrowth(i int, caller GrowthBatch) []Event {
	var events []Event
	if !w.Growth[i].Harvested {
		required := max(int64(1), w.Growth[i].Rule.CareWorkersPerBatch*w.Growth[i].Batches)
		work, completed := w.completeGrowthWork(w.Growth[i], caller, "harvest", required-w.Growth[i].HarvestWork)
		events = append(events, completed...)
		w.Growth[i].HarvestWork += work
		if w.Growth[i].HarvestWork < required {
			return append(events, w.growthEvent(w.Growth[i], "harvest_progressed", map[string]any{"work_done": w.Growth[i].HarvestWork, "work_required": required}))
		}
	} else {
		_, events = w.completeGrowthWork(w.Growth[i], caller, "collect", 1)
		if len(events) == 0 {
			return nil
		}
	}
	return append(events, w.finishHarvest(i)...)
}

func (w *World) finishHarvest(i int) []Event {
	var events []Event
	g := w.Growth[i]
	care := w.growthCaretaker(g)
	if !g.Harvested {
		if agreement := w.careAgreement(g); agreement != nil && agreement.SharePermille > 0 {
			share := agreement.CaretakerShare(g.Quantity)
			if share > 0 {
				entitlement := w.harvestAllocation(g, care, "harvest_entitled", "care_agreement", share)
				entitlement.Payload["care"] = *agreement
				events = append(events, entitlement)
				if w.growthOwnerPresent(care) {
					w.carryLivingCare(fmt.Sprintf("growth:%d/output", g.ID), holderKey(holderKind(care.Cohort), care.OwnerID)+"/"+g.Item, care, g.Item)
					w.changeGrowthStock(care, g.Item, share, false)
					events = append(events, w.harvestAllocation(g, care, "harvest_delivered", "care_agreement", share))
				} else {
					// Earned compensation stays at the harvest site until collected.
					stored := g
					stored.SourceGrowthID = g.ID
					stored.ID = w.NextID
					w.NextID++
					stored.OwnerID, stored.Cohort = care.OwnerID, care.Cohort
					stored.Quantity, stored.Parents = share, nil
					stored.FieldID, stored.AreaM2, stored.HouseID = "", 0, ""
					stored.Harvested, stored.Workers, stored.Batches = true, 0, 1
					stored.Rule.CareWorkersPerBatch = 0
					w.Growth = append(w.Growth, stored)
					w.carryLivingCare(fmt.Sprintf("growth:%d/output", g.ID), fmt.Sprintf("growth:%d/output", stored.ID), stored, g.Item)
				}
			}
			g.Quantity -= share
		}
		if g.Quantity > 0 {
			events = append(events, w.harvestAllocation(g, g, "harvest_entitled", "property", g.Quantity))
		}
	}
	if !w.growthOwnerPresent(g) {
		g.Harvested = true
		w.Growth[i] = g
		return append(events, w.growthEvent(g, "harvest_stored", map[string]any{"item": g.Item, "quantity": g.Quantity, "district": g.District, "owner_id": g.OwnerID, "caretaker_id": care.OwnerID}))
	}
	outputKey := fmt.Sprintf("growth:%d/output", g.ID)
	if g.Quantity > 0 {
		w.carryLivingCare(outputKey, holderKey(holderKind(g.Cohort), g.OwnerID)+"/"+g.Item, g, g.Item)
	}
	w.changeGrowthStock(g, g.Item, g.Quantity, false)
	if g.Quantity > 0 {
		events = append(events, w.harvestAllocation(g, g, "harvest_delivered", g.harvestBasis(), g.Quantity))
	}
	delete(w.Production.LivingCare, outputKey)
	for item, qty := range g.Parents {
		key := fmt.Sprintf("growth:%d/parent/%s", g.ID, item)
		if qty > 0 {
			w.carryLivingCare(key, holderKey(holderKind(g.Cohort), g.OwnerID)+"/"+item, g, item)
		}
		w.changeGrowthStock(g, item, qty, false)
		delete(w.Production.LivingCare, key)
	}
	if len(w.Production.LivingCare) == 0 {
		w.Production.LivingCare = nil
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
		return append(append(events, event), w.grantProfessionXP(a, x, r.Profession, w.Catalog.CraftXP(r), w.Catalog.Items[g.Item].Tier)...)
	}
	c := w.findCohort(g.OwnerID)
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "cohort_crafted", Action: Action{Kind: "craft", RecipeID: g.RecipeID, Item: g.Item, Quantity: int(g.Quantity)}, Payload: map[string]any{"growth_id": g.ID, "item": g.Item, "quantity": g.Quantity, "batches": g.Batches, "recipe_id": g.RecipeID, "cohort_id": g.OwnerID}}
	w.NextID++
	events = append(events, e)
	if g.RecipeID == w.Catalog.Lifecycle.CompostRecipeID {
		events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: g.OwnerID, Kind: "nightsoil_composted", Action: Action{Kind: "craft", RecipeID: g.RecipeID, Item: g.Item, Quantity: int(g.Quantity)}, Payload: map[string]any{"item": g.Item, "compost_generated": g.Quantity}})
		w.NextID++
	}
	return append(events, w.addCohortXP(c, w.Catalog.CraftXP(r), maxInt(1, w.Catalog.Items[g.Item].Tier)*c.ProfLevel)...)

}

func (w *World) visibleGrowth(actor Actor) []GrowthBatch {
	var result []GrowthBatch
	for _, g := range w.Growth {
		if care := w.careAgreement(g); (!g.Cohort && g.OwnerID == actor.ID) || (care != nil && care.CaretakerKind == "actor" && care.CaretakerID == actor.ID) {
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
	if g.CareWorkersPerBatch < 0 || g.FailureAfterTicks < 0 {
		return fmt.Errorf("recipe %s growth labor and failure budget cannot be negative", r.ID)
	}
	if g.DurationTicks <= 0 || g.MaintenanceIntervalTicks <= 0 || g.Source == "" {
		return fmt.Errorf("recipe %s growth requires positive duration, maintenance interval and source", r.ID)
	}
	if len(g.Supplies) == 0 && g.ResourcePool == "" && len(g.Parents) == 0 {
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

func (r Recipe) StartRequirements() map[string]int {
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

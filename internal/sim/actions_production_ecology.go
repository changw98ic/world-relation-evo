package sim

import "fmt"

func registerProductionEcologyActions() {
	registerBuiltinAction("refuel", handleRefuelAction)
	registerBuiltinAction("harvest", handleHarvestAction)
	registerBuiltinAction("craft", handleCraftAction)
	registerBuiltinAction("maintain_infra", handleMaintainInfraAction)
	registerBuiltinAction("hunt_threat", handleHuntThreatAction)
	registerBuiltinAction("perform_ritual", handlePerformRitualAction)
}

func handleRefuelAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if w.FlowGrid != nil {
		for i := range w.FlowGrid.Nodes {
			n := &w.FlowGrid.Nodes[i]
			if n.ID != x.TargetID {
				continue
			}
			if n.Kind != "generator" || n.Source == nil || n.Source.FuelItem == "" || n.DistrictID != a.Location || (n.VenueID != "" && n.VenueID != a.VenueID) {
				return actionFail(tx, a, x, "generator is not locally accessible")
			}
			if x.Quantity <= 0 || a.Items[n.Source.FuelItem] < x.Quantity {
				return actionFail(tx, a, x, "insufficient declared generator fuel")
			}
			tx.moveItem(a, n.Source.FuelItem, -x.Quantity, n.ID, x.Kind)
			w.adjustFlowNodeFuel(n, int64(x.Quantity), false)
			return actionEmit(tx, a, x, "generator_refueled", fmt.Sprintf("transferred %d %s into %s", x.Quantity, n.Source.FuelItem, n.ID))
		}
	}
	return actionFail(tx, a, x, "unknown generator")
}

func handleHarvestAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	for i, g := range w.Growth {
		if fmt.Sprint(g.ID) == x.TargetID && w.canHarvestGrowth(g, a.ID, false) {
			return w.harvestGrowth(tx, i, GrowthBatch{OwnerID: a.ID, District: g.District, VenueID: g.VenueID})
		}
	}
	return actionFail(tx, a, x, "no mature owned crop or stock at this location")
}

func handleCraftAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	r, ok := w.Catalog.RecipeByID(x.RecipeID)
	if !ok {
		return actionFail(tx, a, x, "unknown recipe: "+x.RecipeID)
	}
	var yield int
	var funded int64
	if r.Growth == nil {
		whole, batch := w.previewNonBiologicalYield("actor", a.ID, a.Location, r)
		yield, funded = int(whole), batch
		if whole < 0 || int64(yield) != whole {
			return actionFail(tx, a, x, "crafted output cannot fit in the actor's inventory")
		}
	}
	fieldID, _ := w.fieldAvailable("actor", a.ID, a.Location, a.VenueID, r)
	if r.ResourcePool != "" {
		draw := int64(maxInt(1, r.ResourceDraw))
		w.drawDistrictResource(a.Location, r.ResourcePool, draw)
	}
	var prepaidCurrencies map[string]int64
	prepaidTreasury := false
	if r.MoneyCost > 0 {
		prepaidCurrencies = map[string]int64{"coins": int64(r.MoneyCost)}
		if len(w.Cohorts) > 0 {
			prepaidTreasury = true
			tx.moveCurrency(a, "coins", -int64(r.MoneyCost), "treasury", x.Kind)
			tx.creditTreasury(int64(r.MoneyCost), "fee")
		} else {
			tx.MintOrBurnCurrency(a, "coins", -int64(r.MoneyCost), x.Kind)
		}
	}
	var prepaidItems map[string]int
	for _, item := range sortedProductionKeys(r.Inputs) {
		qty := r.Inputs[item]
		if qty > 0 {
			if prepaidItems == nil {
				prepaidItems = map[string]int{}
			}
			prepaidItems[item] += qty
		}
		tx.ProduceOrConsumeItem(a, item, -qty, x.Kind)
	}
	bonus := w.Catalog.ConsumablesYieldBonus(r) && a.Items[r.BonusInput] >= 1
	if bonus {
		if prepaidItems == nil {
			prepaidItems = map[string]int{}
		}
		prepaidItems[r.BonusInput]++
		tx.ProduceOrConsumeItem(a, r.BonusInput, -1, x.Kind)
	}
	if r.Growth != nil {
		yield = w.Catalog.RecipeYield(*a, r, bonus)
	}
	w.wearProductionTool(tx, "actor", a.ID, r, 1)
	ticks := r.Ticks
	task := Task{
		Action:            x,
		TotalTicks:        ticks,
		Remaining:         ticks,
		StartedTick:       w.Tick,
		Yield:             yield,
		FundedHundredths:  funded,
		FieldID:           fieldID,
		PrepaidCurrencies: prepaidCurrencies,
		PrepaidTreasury:   prepaidTreasury,
		PrepaidItems:      prepaidItems,
	}
	tx.startTaskWork(a, &task, r.EnergyCost)
	w.advanceCraftingWork(a, &task)
	if task.Remaining <= 0 {
		task.PrepaidCurrencies, task.PrepaidItems = nil, nil
		return w.finishTaskWithTx(tx, a, task)
	}
	a.Task = &task
	expectedTicks, _ := r.WorkEstimate(w.FacilityProductionBonusPct(*a))
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s started %s (expected %d %s, %d ticks)", a.Name, r.ID, yield, r.OutputItem, expectedTicks),
		"recipe_id":       r.ID,
		"expected_yield":  yield,
		"remaining_ticks": task.Remaining,
	})}
}

func handleMaintainInfraAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if w.DistrictStates == nil {
		return actionFail(tx, a, x, "district public infrastructure is not enabled")
	}
	if a.InsideHouse != "" {
		return actionFail(tx, a, x, "step outside before maintaining district infrastructure")
	}
	ds, hasDS := w.DistrictStates[a.Location]
	spec, exists := w.Catalog.Infrastructures[x.Item]
	if !hasDS || !exists || ds.Infrastructure == nil {
		return actionFail(tx, a, x, "unknown infrastructure: "+x.Item)
	}
	curCond, present := ds.Infrastructure[x.Item]
	if !present {
		return actionFail(tx, a, x, "infrastructure is not present in "+a.Location)
	}
	if curCond >= spec.MaxCondition {
		return actionFail(tx, a, x, "infrastructure is already at full condition")
	}
	if a.Energy < spec.BuildEnergy {
		return actionFail(tx, a, x, "insufficient energy to maintain infrastructure")
	}
	for _, inItem := range sortedProductionKeys(spec.BuildInputs) {
		inQty := spec.BuildInputs[inItem]
		if a.Items[inItem] < inQty {
			return actionFail(tx, a, x, fmt.Sprintf("maintaining %s requires %d %s", spec.ID, inQty, inItem))
		}
	}
	for _, inItem := range sortedProductionKeys(spec.BuildInputs) {
		inQty := spec.BuildInputs[inItem]
		tx.ProduceOrConsumeItem(a, inItem, -inQty, x.Kind)
	}
	task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
	tx.startTaskWork(a, &task, spec.BuildEnergy)
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s started reinforcing %s in %s (3 ticks)", a.Name, spec.Name, a.Location),
		"infrastructure":  spec.ID,
		"remaining_ticks": task.Remaining,
	})}
}

func handleHuntThreatAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if w.DistrictStates == nil {
		return actionFail(tx, a, x, "no ecological threats enabled")
	}
	if a.InsideHouse != "" {
		return actionFail(tx, a, x, "step outside before hunting wild threats")
	}
	if a.Health < 50 || w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "hunter is injured or recuperating")
	}
	ds, hasDS := w.DistrictStates[a.Location]
	if !hasDS {
		return actionFail(tx, a, x, "no threat activity in current district")
	}
	chosenID := ""
	bestAct := 5
	for _, enc := range w.Catalog.Encounters {
		if x.TargetID != "" && enc.ID != x.TargetID {
			continue
		}
		if act := ds.ThreatActivity[enc.ID]; act > bestAct {
			bestAct = act
			chosenID = enc.ID
		}
	}
	if chosenID == "" {
		return actionFail(tx, a, x, "no active threat above threshold in "+a.Location)
	}
	x.TargetID = chosenID
	task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s tracked %s in %s (3 ticks)", a.Name, chosenID, a.Location),
		"threat_id":       chosenID,
		"remaining_ticks": task.Remaining,
	})}
}

func handlePerformRitualAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if !w.Catalog.EffectiveEnvironment().Institutions.RitualsEnabled() {
		return actionFail(tx, a, x, "rituals are disabled in this world environment")
	}
	if a.InsideHouse != "" {
		return actionFail(tx, a, x, "step outside to an open ritual precinct before performing ritual")
	}
	var rit *RitualSpec
	for i := range w.Catalog.Rituals {
		cand := &w.Catalog.Rituals[i]
		if x.RecipeID != "" && cand.ID != x.RecipeID {
			continue
		}
		if cand.NodeFeature != "" && !w.Map.DistrictHasFeature(a.Location, cand.NodeFeature) {
			continue
		}
		rit = cand
		break
	}
	if rit == nil {
		return actionFail(tx, a, x, "ritual cannot be performed at current location")
	}
	x.RecipeID = rit.ID
	if a.Energy < rit.EnergyCost {
		return actionFail(tx, a, x, "insufficient energy for ritual")
	}
	if a.Money < rit.MoneyCost {
		return actionFail(tx, a, x, "insufficient money for ritual")
	}
	for _, inItem := range sortedProductionKeys(rit.Inputs) {
		inQty := rit.Inputs[inItem]
		if a.Items[inItem] < inQty {
			return actionFail(tx, a, x, fmt.Sprintf("ritual %s requires %d %s", rit.ID, inQty, inItem))
		}
	}
	if rit.MoneyCost > 0 {
		if len(w.Cohorts) > 0 {
			tx.moveCurrency(a, "coins", -int64(rit.MoneyCost), "treasury", x.Kind)
			w.creditTreasury(int64(rit.MoneyCost), "fee")
		} else {
			tx.MintOrBurnCurrency(a, "coins", -int64(rit.MoneyCost), x.Kind)
		}
	}
	for _, inItem := range sortedProductionKeys(rit.Inputs) {
		inQty := rit.Inputs[inItem]
		tx.ProduceOrConsumeItem(a, inItem, -inQty, x.Kind)
	}
	ticks := rit.Ticks
	if ticks < 1 {
		ticks = 1
	}
	task := Task{Action: x, TotalTicks: ticks, Remaining: ticks - 1, StartedTick: w.Tick}
	tx.startTaskWork(a, &task, rit.EnergyCost)
	if task.Remaining <= 0 {
		return w.finishTaskWithTx(tx, a, task)
	}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s began conducting %s (%d ticks)", a.Name, rit.Name, ticks),
		"ritual_id":       rit.ID,
		"remaining_ticks": task.Remaining,
	})}
}

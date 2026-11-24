package sim

import "fmt"

func registerSurvivalActions() {
	registerBuiltinAction("wait", handleWaitAction)
	registerBuiltinAction("begin_rest", handleBeginRestAction)
	registerBuiltinAction("end_rest", handleEndRestAction)
	registerBuiltinAction("sleep", handleSleepAction)
	registerBuiltinAction("eat", handleEatAction)
	registerBuiltinAction("drink", handleDrinkAction)
	registerBuiltinAction("relieve", handleRelieveAction)
	registerBuiltinAction("medicate", handleMedicateAction)
	registerBuiltinAction("heal", handleHealAction)
}

func handleWaitAction(*WorldTx, *Actor, *Actor, Action) []Event {
	return nil
}

func handleBeginRestAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	a.Resting = true
	return actionEmit(tx, a, x, "rest_planned", "plans to reach shelter and recover energy before starting new work")
}

func handleEndRestAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	a.Resting = false
	return actionEmit(tx, a, x, "rest_abandoned", "ends the planned rest")
}

func handleSleepAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.Energy >= MaxEnergy && a.Health >= MaxHealth {
		return actionFail(tx, a, x, "already fully rested")
	}
	ticks := x.Quantity
	if ticks <= 0 {
		ticks = 16
	}
	if ticks > 32 {
		ticks = 32
	}
	x.Quantity = ticks
	startEnergy := a.Energy
	carry, factors := w.applySleepTick(tx, a, 0)
	if ticks == 1 || (a.Energy >= MaxEnergy && a.Health >= MaxHealth) {
		gained := a.Energy - startEnergy
		return []Event{tx.record(a, x, "slept", map[string]any{
			"text":          fmt.Sprintf("%s slept briefly and now has %d energy (+%d) and %d health", a.Name, a.Energy, gained, a.Health),
			"energy_gained": gained,
			"health":        a.Health,
			"sleep_factors": factors,
		})}
	}
	a.Task = &Task{
		Action:      x,
		TotalTicks:  ticks,
		Remaining:   ticks - 1,
		StartedTick: w.Tick,
		SleepTenths: carry,
		StartEnergy: startEnergy,
	}
	return []Event{tx.record(a, x, "sleep_started", map[string]any{
		"text":          fmt.Sprintf("%s lay down to sleep (%d ticks)", a.Name, ticks),
		"sleep_factors": factors,
	})}
}

func handleEatAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	itemID := x.Item
	if itemID == "" {
		itemID = bestHeldFood(w.Catalog, *a)
		x.Item = itemID
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok || !spec.Edible {
		return actionFail(tx, a, x, "item is not edible: "+itemID)
	}
	if form, hasForm := w.Catalog.ActorFormSpec(*a); hasForm && !form.MeetsMinFoodTier(spec) {
		return actionFail(tx, a, x, "food tier too low for current form: "+itemID)
	}
	if a.Items[itemID] < 1 {
		return actionFail(tx, a, x, "no "+itemID)
	}
	if a.Hunger == 0 {
		return nil
	}
	w.consumeHeldFood(tx, a, itemID)
	return actionEmit(tx, a, x, "ate", fmt.Sprintf("%s ate 1 %s", a.Name, itemID))
}

func bestHeldFood(catalog Catalog, a Actor) string {
	var best string
	for id, spec := range catalog.Items {
		if a.Items[id] <= 0 || !catalog.ActorAcceptsFood(a, spec) {
			continue
		}
		if best == "" || spec.NutritionUnits > catalog.Items[best].NutritionUnits ||
			spec.NutritionUnits == catalog.Items[best].NutritionUnits && id < best {
			best = id
		}
	}
	return best
}

func handleDrinkAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.Thirst <= 0 {
		return nil
	}
	if x.Item == "" {
		ds := w.DistrictStates[a.Location]
		if pool := w.potablePoolID(ds); pool != "" && w.drawDistrictResource(a.Location, pool, 1) == 1 {
			a.Thirst = 0
			return actionEmit(tx, a, x, "drank", fmt.Sprintf("%s drank fresh water at %s", a.Name, a.Location))
		}
	}

	itemID := x.Item
	if itemID == "" {
		itemID = w.firstCarriedPotableItem(*a)
		x.Item = itemID
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok || !spec.Potable {
		return actionFail(tx, a, x, "no potable water available")
	}
	if a.Items[itemID] < 1 {
		return actionFail(tx, a, x, "no "+itemID)
	}
	tx.ProduceOrConsumeItem(a, itemID, -1, x.Kind)
	a.Thirst -= spec.DrinkingRelief()
	if a.Thirst < 0 {
		a.Thirst = 0
	}
	w.applyItemSlotAndEnergyEffects(tx, a, spec, x.Kind)
	return actionEmit(tx, a, x, "drank", fmt.Sprintf("%s drank 1 %s", a.Name, itemID))
}

func handleRelieveAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.Waste <= 0 {
		return nil
	}
	cleanLatrine := a.InsideHouse != "" || w.DistrictStates == nil
	if !cleanLatrine && w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[a.Location]; ok {
			if cond, hasSan := w.sanitationInfraCondition(ds); hasSan && cond >= 30 {
				cleanLatrine = true
			}
		}
	}
	wasteAmt := int64(0)
	if divisor := w.Catalog.Lifecycle.WastePerFoodDivisor; divisor > 0 && w.Catalog.Lifecycle.WasteItem != "" {
		wasteAmt = a.DigestedFood / divisor
		a.DigestedFood -= wasteAmt * divisor
	}
	a.Waste = 0
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[a.Location]; ok {
			if cleanLatrine {
				w.adjustDistrictNightsoil(a.Location, w.Catalog.Lifecycle.WasteItem, wasteAmt, true)
			} else if ds.Sanitation > 10 {
				ds.Sanitation--
				w.DistrictStates[a.Location] = ds
			}
		}
	}
	return actionEmit(tx, a, x, "relieved", fmt.Sprintf("%s used sanitation facility in %s", a.Name, a.Location))
}

func handleMedicateAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.Health >= MaxHealth && a.DiseaseID == "" {
		return actionFail(tx, a, x, "actor has no injury or illness")
	}
	itemID := x.Item
	if itemID == "" {
		bestScore := 0
		for _, k := range sortedProductionKeys(w.Catalog.Items) {
			spec := w.Catalog.Items[k]
			score := spec.MedicinalHeal + spec.DiseaseCure*15
			if score > bestScore && a.Items[k] >= 1 {
				bestScore = score
				itemID = k
			}
		}
		x.Item = itemID
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) {
		return actionFail(tx, a, x, "item is not medicinal: "+itemID)
	}
	if a.Items[itemID] < 1 {
		return actionFail(tx, a, x, "no "+itemID)
	}
	tx.ProduceOrConsumeItem(a, itemID, -1, x.Kind)
	healAmt := tx.adjustHealth(a, maxInt(0, spec.MedicinalHeal), 0, x.Kind)
	restCut := int64(maxInt(24, spec.MedicinalHeal))
	if a.RestUntil > w.Tick+restCut {
		a.RestUntil -= restCut
	} else if a.RestUntil > w.Tick {
		a.RestUntil = w.Tick
	}
	curedDisease := ""
	if a.DiseaseID != "" && spec.DiseaseCure > 0 {
		a.DiseaseTicks -= spec.DiseaseCure * TicksPerDay
		if a.DiseaseTicks <= 0 {
			curedDisease = a.DiseaseID
			a.DiseaseID = ""
			a.DiseaseTicks = 0
		}
	}
	w.applyItemSlotAndEnergyEffects(tx, a, spec, x.Kind)
	ev := tx.record(a, x, "medicated", map[string]any{
		"text":          fmt.Sprintf("%s treated wounds/illness with 1 %s (+%d health -> %d)", a.Name, itemID, healAmt, a.Health),
		"item":          itemID,
		"healed":        healAmt,
		"health":        a.Health,
		"cured_disease": curedDisease,
	})
	return []Event{ev}
}

func handleHealAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) {
		return actionFail(tx, a, x, "patient is not nearby")
	}
	if b.Health >= MaxHealth && b.DiseaseID == "" {
		return actionFail(tx, a, x, "patient has no injury or illness")
	}
	itemID := x.Item
	if itemID == "" {
		for _, k := range sortedProductionKeys(w.Catalog.Items) {
			spec := w.Catalog.Items[k]
			if (spec.MedicinalHeal > 0 || spec.DiseaseCure > 0) && a.Items[k] >= 1 {
				if itemID == "" || spec.MedicinalHeal < w.Catalog.Items[itemID].MedicinalHeal {
					itemID = k
				}
			}
		}
		x.Item = itemID
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) || a.Items[itemID] < 1 {
		return actionFail(tx, a, x, "no medicine")
	}
	wantHeal := maxInt(15, spec.MedicinalHeal-5) + w.Catalog.SkillHealBonus(*a)
	amount := tx.adjustHealth(b, maxInt(0, wantHeal), 0, x.Kind)
	if b.RestUntil > w.Tick+16 {
		b.RestUntil -= 16
	}
	curedDisease := ""
	if b.DiseaseID != "" && spec.DiseaseCure > 0 {
		b.DiseaseTicks -= spec.DiseaseCure * TicksPerDay
		if b.DiseaseTicks <= 0 {
			curedDisease = b.DiseaseID
			b.DiseaseID = ""
			b.DiseaseTicks = 0
		}
	}
	tx.ProduceOrConsumeItem(a, itemID, -1, x.Kind)
	tx.MutateTrust(a, b.ID, 1)
	tx.MutateTrust(b, a.ID, 2)
	if w.actorFactionRelation(*a, *b) == "allied" {
		tx.MutateTrust(b, a.ID, 1)
	}
	ev := tx.record(a, x, "healed", map[string]any{
		"text":          fmt.Sprintf("%s healed %s for %d health", a.Name, b.Name, amount),
		"healed":        amount,
		"health":        b.Health,
		"cured_disease": curedDisease,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	events := []Event{ev}
	for _, sid := range sortedProductionKeys(a.Skills) {
		if a.Skills[sid] > 0 {
			if sk, ok := w.Catalog.Skills[sid]; ok && sk.HealBonusPerLevel > 0 {
				events = append(events, w.grantSkillXP(tx, a, x, sid, 6)...)
			}
		}
	}
	for _, prof := range w.Catalog.Professions {
		if a.Professions[prof.ID] > 0 && ActorHasTag(prof.Grants, "healer") {
			return append(events, w.grantProfessionXP(tx, a, x, prof.ID, 6, 2)...)
		}
	}
	return events
}

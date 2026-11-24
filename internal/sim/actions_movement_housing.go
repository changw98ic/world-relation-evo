package sim

import "fmt"

func registerMovementHousingActions() {
	registerBuiltinAction("move", handleMoveAction)
	registerBuiltinAction("enter_house", handleEnterHouseAction)
	registerBuiltinAction("leave_house", handleLeaveHouseAction)
	registerBuiltinAction("repair_house", handleRepairHouseAction)
	registerBuiltinAction("upgrade_house", handleUpgradeHouseAction)
	registerBuiltinAction("transfer_house", handleTransferHouseAction)
}

func handleMoveAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.InsideHouse != "" {
		return actionFail(tx, a, x, "leave house before walking village roads")
	}
	if len(w.Map.Venues) > 0 {
		return w.applyMoveDistrict(tx, a, x)
	}
	road, ok := w.Map.RoadBetween(a.Location, x.Location)
	if !ok {
		return actionFail(tx, a, x, fmt.Sprintf("no direct road from %s to %s", a.Location, x.Location))
	}
	if severed, reason := w.isRouteSevered(a.Location, x.Location, road.Medium, ""); severed {
		return actionFail(tx, a, x, reason)
	}
	effTicks, effEnergy := w.effectiveRoadCost(road)
	if a.Energy < effEnergy {
		return actionFail(tx, a, x, "insufficient energy to walk to "+x.Location)
	}
	task := Task{Action: x, TotalTicks: effTicks, Remaining: effTicks - 1, StartedTick: w.Tick}
	tx.startTaskWork(a, &task, effEnergy)
	if task.Remaining <= 0 {
		return w.finishTaskWithTx(tx, a, task)
	}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s set out toward %s (%d ticks)", a.Name, x.Location, effTicks),
		"remaining_ticks": task.Remaining,
	})}
}

func handleEnterHouseAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.InsideHouse != "" {
		return actionFail(tx, a, x, "already inside house "+a.InsideHouse)
	}
	h := w.house(x.HouseID)
	if h == nil || h.District != a.Location {
		return actionFail(tx, a, x, "house is not in current district")
	}
	if h.VenueID != "" && a.VenueID != "" && h.VenueID != a.VenueID {
		return actionFail(tx, a, x, "house is in a different venue")
	}
	if !h.CanAccess(a.ID) {
		return actionFail(tx, a, x, "actor does not have access to house "+h.ID)
	}
	venue := a.VenueID
	if venue == "" {
		venue = h.VenueID
	}
	tx.MoveActor(a, a.Location, venue, h.ID)
	return actionEmit(tx, a, x, "entered_house", a.Name+" entered "+h.Name)
}

func handleLeaveHouseAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	if a.InsideHouse == "" {
		return actionFail(tx, a, x, "actor is already outdoors")
	}
	h := w.house(a.InsideHouse)
	name, venue := a.InsideHouse, a.VenueID
	if h != nil {
		name = h.Name
		if h.VenueID != "" {
			venue = h.VenueID
		}
	}
	tx.MoveActor(a, a.Location, venue, "")
	return actionEmit(tx, a, x, "left_house", a.Name+" stepped outside from "+name)
}

func handleRepairHouseAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	env := w.Catalog.EffectiveEnvironment()
	repairItem := env.RepairItem
	h := w.house(x.HouseID)
	if h == nil || h.District != a.Location || (a.InsideHouse != "" && a.InsideHouse != h.ID) {
		return actionFail(tx, a, x, "house is not at current location")
	}
	if h.Condition >= MaxCondition {
		return actionFail(tx, a, x, "house does not need repair")
	}
	if a.Items[repairItem] < 1 {
		return actionFail(tx, a, x, "repairing a house requires 1 "+repairItem)
	}
	tx.ProduceOrConsumeItem(a, repairItem, -1, x.Kind)
	task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s started repairing %s (3 ticks)", a.Name, h.Name),
		"house_id":        h.ID,
		"remaining_ticks": task.Remaining,
	})}
}

func (c Catalog) HouseRepairGain(a Actor) int {
	builder := c.EffectiveEnvironment().BuilderProfession
	levels := a.Professions[builder]
	for _, profession := range c.Professions {
		if profession.ID != builder && (profession.ParentProfession == builder ||
			c.ProfessionGrantsCapability(profession, "repair_house") ||
			c.ProfessionGrantsCapability(profession, "upgrade_house")) {
			levels += a.Professions[profession.ID]
		}
	}
	if levels > 0 {
		return 30 + 5*levels
	}
	return 25
}

func handleUpgradeHouseAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	env := w.Catalog.EffectiveEnvironment()
	repairItem := env.RepairItem
	builderProf := env.BuilderProfession
	h := w.house(x.HouseID)
	if h == nil {
		return actionFail(tx, a, x, "unknown house: "+x.HouseID)
	}
	if h.OwnerID != a.ID {
		return actionFail(tx, a, x, "only the house owner may upgrade "+h.Name)
	}
	if a.InsideHouse != h.ID {
		return actionFail(tx, a, x, "must be inside "+h.Name+" to renovate it")
	}
	if h.Level >= 3 {
		return actionFail(tx, a, x, "house is already at maximum asset level")
	}
	if builderProf != "" && a.Professions[builderProf] < 2 {
		return actionFail(tx, a, x, "renovating a house requires "+builderProf+" profession level 2")
	}
	materials := h.Level
	coins := 4 * h.Level
	if a.Items[repairItem] < materials {
		return actionFail(tx, a, x, fmt.Sprintf("renovation requires %d %s", materials, repairItem))
	}
	if a.Money < coins {
		return actionFail(tx, a, x, fmt.Sprintf("renovation requires %d coins", coins))
	}
	tx.ProduceOrConsumeItem(a, repairItem, -materials, x.Kind)
	if coins > 0 {
		if len(w.Cohorts) > 0 {
			tx.moveCurrency(a, "coins", -int64(coins), "treasury", x.Kind)
			w.creditTreasury(int64(coins), "fee")
		} else {
			tx.MintOrBurnCurrency(a, "coins", -int64(coins), x.Kind)
		}
	}
	task := Task{Action: x, TotalTicks: 4, Remaining: 3, StartedTick: w.Tick}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s started renovating %s to level %d (4 ticks)", a.Name, h.Name, h.Level+1),
		"house_id":        h.ID,
		"remaining_ticks": task.Remaining,
	})}
}

func handleTransferHouseAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	h := w.house(x.HouseID)
	if h == nil || h.OwnerID != a.ID {
		return actionFail(tx, a, x, "actor does not own house "+x.HouseID)
	}
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "recipient is not nearby")
	}
	h.OwnerID = b.ID
	h.Residents = removeString(h.Residents, a.ID)
	if a.HomeID == h.ID {
		tx.setHome(a, "")
	}
	if a.InsideHouse == h.ID {
		tx.MoveActor(a, a.Location, a.VenueID, "")
	}
	if b.HomeID == "" {
		tx.setHome(b, h.ID)
	}
	tx.MutateTrust(a, b.ID, 2)
	tx.MutateTrust(b, a.ID, 2)
	return actionEmit(tx, a, x, "house_transferred", fmt.Sprintf("%s transferred ownership of %s to %s", a.Name, h.Name, b.Name), b)
}

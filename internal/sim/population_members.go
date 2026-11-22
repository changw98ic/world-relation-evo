package sim

import (
	"fmt"
	"sort"
)

// AvgHunger is the ceiling of anonymous hunger per person. Paid meal portions
// below one whole average point remain credited instead of feeding extra people.
func (c PopulationCohort) hungerUnits(population int) int64 {
	return int64(c.AvgHunger)*int64(population) - c.HungerReliefRemainder
}

func (c *PopulationCohort) setHungerUnits(population int, units int64) {
	if population == 0 {
		c.AvgHunger, c.HungerReliefRemainder = 0, 0
		return
	}
	c.AvgHunger = int((units + int64(population) - 1) / int64(population))
	c.HungerReliefRemainder = int64(c.AvgHunger)*int64(population) - units
}

func (c PopulationCohort) ValidateHungerBalance() error {
	population := c.Population
	for _, member := range c.Members {
		if !member.Dead {
			population--
		}
	}
	credit := c.HungerReliefRemainder
	if population < 0 || c.AvgHunger < 0 || credit < 0 || population == 0 && credit != 0 || population > 0 && credit >= int64(population) || c.AvgHunger == 0 && credit != 0 {
		return fmt.Errorf("cohort %s has invalid anonymous hunger balance: population=%d average=%d credited_portions=%d", c.ID, population, c.AvgHunger, credit)
	}
	return nil
}

func (w *World) advanceActorHunger(a *Actor) {
	if a.Dead {
		return
	}
	form, _ := w.actorFormSpec(*a)
	advanceNutrition(&a.Hunger, &a.NutritionReserve, hungerIncrement(w.Tick, w.Catalog.Lifecycle, form))
}

// DailyNutritionUnits is the shared ordinary-person demand, not an inventory item.
const DailyNutritionUnits int64 = 3

func nutritionNeeded(ticks int64) int64 {
	return ticks/TicksPerDay*DailyNutritionUnits + (ticks%TicksPerDay*DailyNutritionUnits+TicksPerDay-1)/TicksPerDay
}

// DailyNutritionForForm returns the daily nutrition units required by form
// (0 for fasting/non-eating forms, 3 for standard mortals, >3 for high-intake forms).
func DailyNutritionForForm(form FormSpec) int64 {
	if form.IsHungerDisabled() {
		return 0
	}
	pct := int64(form.EffectiveDailyIntakePct())
	if pct <= 0 {
		return 0
	}
	return maxInt64(1, (DailyNutritionUnits*pct+99)/100)
}

func hungerIncrement(tick int64, _ LifecycleSpec, form FormSpec) int {
	if tick <= 0 || form.IsHungerDisabled() {
		return 0
	}
	if form.DailyIntakePct != nil {
		intakePct := int64(*form.DailyIntakePct)
		denom := int64(TicksPerDay) * 100
		numer := DailyNutritionUnits * intakePct
		total := func(t int64) int64 {
			return (t * numer) / denom
		}
		return int(total(tick) - total(tick-1))
	}
	multiplier := int64(100)
	if form.HungerIntervalMulPct > 0 {
		multiplier = int64(form.HungerIntervalMulPct)
	}
	period := int64(TicksPerDay) * multiplier
	total := func(t int64) int64 {
		return t/period*(DailyNutritionUnits*100) + t%period*(DailyNutritionUnits*100)/period
	}
	return int(total(tick) - total(tick-1))
}

func advanceNutrition(hunger *int, reserve *int64, units int) {
	used := min(*reserve, int64(units))
	*reserve -= used
	*hunger += units - int(used)
}

func absorbNutrition(hunger *int, reserve *int64, units int64) {
	used := min(int64(*hunger), units)
	*hunger -= int(used)
	*reserve += units - used
}

func starvationDamage(hunger int, lc LifecycleSpec, form FormSpec) int {
	if form.IsHungerDisabled() {
		return 0
	}
	threshold := lc.StarvationThreshold
	if form.StarvationThreshold > 0 {
		threshold = form.StarvationThreshold
	}
	if threshold <= 0 {
		threshold = 6
	}
	dmg := lc.StarvationDamage
	if dmg <= 0 {
		dmg = 10
	}
	if form.StarvationDamage != nil {
		dmg = maxInt(0, *form.StarvationDamage)
	}
	if hunger >= threshold {
		return dmg
	}
	return 0
}

// consumeActorRation is the same finite food settlement for stored members and
// prisoners, whose actions cannot otherwise supply their meals.
func (w *World) consumeActorRation(a *Actor) []Event {
	if a.Dead || a.Hunger <= 0 {
		return nil
	}
	form, _ := w.actorFormSpec(*a)
	if form.IsHungerDisabled() {
		return nil
	}
	before := a.Hunger
	reserveBefore := a.NutritionReserve
	meal := func(item, reason string, quantity int) []Event {
		event := Event{ID: w.NextID, Tick: w.Tick, ActorID: a.ID, Kind: "personal_ration_settled", Action: Action{Kind: "eat", Item: item, Quantity: quantity}, Payload: map[string]any{"source_kind": "personal_inventory", "source_id": a.ID, "reason": reason, "hunger_before_meal": before, "hunger_after_meal": a.Hunger, "nutrition_reserve_before": reserveBefore, "nutrition_reserve_after": a.NutritionReserve, "units_per_person": quantity}}
		w.NextID++
		return []Event{event}
	}
	items := make([]string, 0, len(a.Items))
	for item, qty := range a.Items {
		if qty > 0 && form.AcceptsFoodItem(w.Catalog.Items[item]) {
			items = append(items, item)
		}
	}
	sort.Strings(items)
	if len(items) == 0 {
		return meal("", "no_edible_stock", 0)
	}
	item := items[0]
	w.consumeHeldFood(nil, a, item)
	return meal(item, "consumed", 1)
}

// consumeHeldFood settles one verified edible unit from this person's inventory.
func (w *World) consumeHeldFood(tx *WorldTx, a *Actor, item string) {
	if tx == nil {
		w.adjustActorItem(a, item, -1, true)
	} else {
		tx.ProduceOrConsumeItem(a, item, -1, "eat")
	}
	spec := w.Catalog.Items[item]
	absorbNutrition(&a.Hunger, &a.NutritionReserve, spec.NutritionUnits)
	a.DigestedFood++
	a.Waste++
	w.applyItemSlotAndEnergyEffects(tx, a, spec, "eat")
}

// applyItemSlotAndEnergyEffects applies generic energy and state-slot changes declared on an ItemSpec.
func (w *World) applyItemSlotAndEnergyEffects(tx *WorldTx, a *Actor, spec ItemSpec, reason string) {
	if a == nil {
		return
	}
	if spec.EnergyRelief != 0 {
		tx.setEnergy(a, maxInt(0, min(MaxEnergy, a.Energy+spec.EnergyRelief)), reason)
	}
	for slotID, delta := range spec.SlotChanges {
		w.addActorStateSlot(a, slotID, delta)
	}
}

// adjustCohortMember accounts for an exact member in the cohort's public census.
// Member wallets and inventories remain on the member, outside the anonymous pools.
func (w *World) adjustCohortMember(c *PopulationCohort, a Actor, delta int) {
	if a.Dead {
		return
	}
	c.Population += delta
	switch {
	case a.AgeYears <= w.Catalog.Lifecycle.ChildMaxAge:
		c.Children += delta
	case a.AgeYears >= w.Catalog.Lifecycle.ElderMinAge:
		c.Elders += delta
	default:
		c.Adults += delta
	}
	if a.Health < MaxHealth {
		c.Wounded += delta
	}
	if a.DiseaseID != "" {
		c.Sick += delta
		if a.Health < MaxHealth {
			c.WoundedSick += delta
		}
	}
	// A sentence leaves the census when the release/death settlement clears it.
	// Crossing its end tick must not make an anonymous copy release it a second time.
	if a.JailedUntil > 0 {
		c.Jailed += delta
	}
}

// anonymousCohort removes exact members from a census copy, leaving shared pools
// in place. Anonymous settlement must not consume or age those people a second time.
func (w *World) anonymousCohort(c PopulationCohort) PopulationCohort {
	for _, a := range c.Members {
		w.adjustCohortMember(&c, a, -1)
	}
	c.Members = nil
	return c
}

// reconcileCohortMemberChanges updates the census after a batch of exact-member
// changes. Callers take a shallow copy of Members before changing their values.
// Dead members become ordinary dead Actor records so kinship IDs remain resolvable.
func (w *World) reconcileCohortMemberChanges(c *PopulationCohort, before []Actor) {
	for _, a := range before {
		w.adjustCohortMember(c, a, -1)
	}
	living := c.Members[:0]
	for _, a := range c.Members {
		if a.Dead {
			// TotalDeaths counts the archived Actor; only anonymous deaths belong
			// in CumulativeDeaths.
			w.Actors = append(w.Actors, a)
			continue
		}
		w.adjustCohortMember(c, a, 1)
		living = append(living, a)
	}
	if len(living) == 0 {
		living = nil
	}
	c.Members = living
}

func (w *World) anonymousCohortEnergy(c *PopulationCohort, population int) int64 {
	return int64(c.AvgEnergy)*int64(population) - w.Production.EnergyRemainder[c.ID]
}

func (w *World) setAnonymousCohortEnergy(c *PopulationCohort, population int, energy int64) {
	if population == 0 {
		c.AvgEnergy = 0
		delete(w.Production.EnergyRemainder, c.ID)
		return
	}
	c.AvgEnergy = int((energy + int64(population) - 1) / int64(population))
	debt := int64(c.AvgEnergy)*int64(population) - energy
	if debt == 0 {
		delete(w.Production.EnergyRemainder, c.ID)
		return
	}
	if w.Production.EnergyRemainder == nil {
		w.Production.EnergyRemainder = map[string]int64{}
	}
	w.Production.EnergyRemainder[c.ID] = debt
}

// Transfer actual energy and unpaid hunger before re-expressing each anonymous
// pool as an average and its retained fractional balance.
func (w *World) transferCohortEnergyAndHunger(from, to *PopulationCohort, moved, fromPopulation, toPopulation int) {
	if moved == 0 {
		return
	}
	fromEnergy := w.anonymousCohortEnergy(from, fromPopulation)
	toEnergy := w.anonymousCohortEnergy(to, toPopulation)
	share := fromEnergy * int64(moved) / int64(fromPopulation)
	w.setAnonymousCohortEnergy(from, fromPopulation-moved, fromEnergy-share)
	w.setAnonymousCohortEnergy(to, toPopulation+moved, toEnergy+share)
	fromHunger := from.hungerUnits(fromPopulation)
	toHunger := to.hungerUnits(toPopulation)
	from.ensureResidents(fromPopulation)
	to.ensureResidents(toPopulation)
	hungerShare := from.moveResidents(to, moved)
	from.setHungerUnits(fromPopulation-moved, fromHunger-hungerShare)
	to.setHungerUnits(toPopulation+moved, toHunger+hungerShare)
}

// Population transfers also carry the corresponding anonymous currency and
// digestion stores. Personal member assets never enter these shared pools.
func (w *World) transferCohortStoredShare(from, to *PopulationCohort, moved, population int) {
	share := from.DigestedFood * int64(moved) / int64(population)
	from.DigestedFood -= share
	to.DigestedFood += share
	for currency, amount := range from.Currencies {
		share := amount * int64(moved) / int64(population)
		w.addCohortCurrency(to, currency, share)
		from.Currencies[currency] -= share
	}
}

func (w *World) transferLastCohortAssets(tx *WorldTx, c *PopulationCohort, a *Actor) {
	w.adjustActorCurrency(a, "coins", c.MoneyPool, false)
	tx.recordTransfer("transfer_currency", c.ID, a.ID, "coins", -c.MoneyPool, "cohort assets")
	c.MoneyPool = 0
	a.DigestedFood += c.DigestedFood
	c.DigestedFood = 0
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	for _, item := range sortedProductionKeys(c.ItemPool) {
		qty := c.ItemPool[item]
		a.Items[item] += int(qty)
		tx.recordTransfer("transfer_item", c.ID, a.ID, item, -qty, "cohort assets")
	}
	c.ItemPool = nil
	for _, currency := range sortedProductionKeys(c.Currencies) {
		amount := c.Currencies[currency]
		w.addActorCurrency(a, currency, amount)
		tx.recordTransfer("transfer_currency", c.ID, a.ID, currency, -amount, "cohort assets")
	}
	c.Currencies = nil
	w.transferGrowingAssets("cohort", c.ID, "actor", a.ID)
}

// settleUnclaimedActorEstate keeps heirless non-coin assets in local custody.
// A zero-population public estate is an account, not an invented inhabitant.
func (w *World) settleUnclaimedActorEstate(tx *WorldTx, deceased *Actor, houses []int) {
	var custodian *PopulationCohort
	for i := range w.Cohorts {
		if w.Cohorts[i].District == deceased.Location && w.Cohorts[i].Population > 0 {
			custodian = &w.Cohorts[i]
			break
		}
	}
	if custodian == nil {
		id := "public_estate_" + deceased.Location
		custodian = w.findCohort(id)
		if custodian == nil {
			w.Cohorts = append(w.Cohorts, PopulationCohort{ID: id, Name: "Unclaimed estate", District: deceased.Location})
			custodian = &w.Cohorts[len(w.Cohorts)-1]
		}
	}
	if len(deceased.Items) > 0 && custodian.ItemPool == nil {
		custodian.ItemPool = map[string]int64{}
	}
	for _, item := range sortedProductionKeys(deceased.Items) {
		qty := deceased.Items[item]
		custodian.ItemPool[item] += int64(qty)
		tx.recordTransfer("transfer_item", deceased.ID, custodian.ID, item, -int64(qty), "inheritance")
	}
	deceased.Items = nil
	for _, currency := range sortedProductionKeys(deceased.Currencies) {
		amount := deceased.Currencies[currency]
		w.addCohortCurrency(custodian, currency, amount)
		tx.recordTransfer("transfer_currency", deceased.ID, custodian.ID, currency, -amount, "inheritance")
	}
	deceased.Currencies = nil
	for _, hi := range houses {
		w.Houses[hi].OwnerID = custodian.ID
	}
	w.transferGrowingAssets("actor", deceased.ID, "cohort", custodian.ID)
}

func (w *World) detachCohortMembers() func() {
	members := make([][]Actor, len(w.Cohorts))
	for i := range w.Cohorts {
		members[i] = w.Cohorts[i].Members
		w.Cohorts[i] = w.anonymousCohort(w.Cohorts[i])
	}
	return func() {
		for i, stored := range members {
			w.Cohorts[i].Members = stored
			for _, a := range stored {
				w.adjustCohortMember(&w.Cohorts[i], a, 1)
			}
		}
	}
}

// drainCohortMortalityEvents reports the statuses removed by actual anonymous
// deaths. Total deaths remain on the event describing their cause.
func (w *World) drainCohortMortalityEvents() []Event {
	var events []Event
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.PendingWoundedDeaths == 0 && c.PendingSickDiedOther == 0 && c.PendingJailedDeaths == 0 {
			continue
		}
		events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: c.ID,
			Kind: "cohort_mortality_status", Action: Action{Kind: "wait"},
			Payload: map[string]any{"wounded_deaths": c.PendingWoundedDeaths,
				"sick_died_other": c.PendingSickDiedOther, "jailed_deaths": c.PendingJailedDeaths}})
		w.NextID++
		c.PendingWoundedDeaths, c.PendingSickDiedOther, c.PendingJailedDeaths = 0, 0, 0
	}
	return events
}

func reconcileActorWoundStatus(a *Actor) (wounded, healed, deaths int) {
	nowWounded := !a.Dead && a.Health < MaxHealth
	if !a.WoundedTracked && nowWounded {
		a.WoundedTracked = true
		return 1, 0, 0
	}
	if a.WoundedTracked && !a.Dead && a.Health >= MaxHealth {
		a.WoundedTracked = false
		return 0, 1, 0
	}
	if a.WoundedTracked && a.Dead {
		a.WoundedTracked = false
		return 0, 0, 1
	}
	return 0, 0, 0
}

func (w *World) storedActorCohort(id string) *PopulationCohort {
	for i := range w.Cohorts {
		for j := range w.Cohorts[i].Members {
			if w.Cohorts[i].Members[j].ID == id {
				return &w.Cohorts[i]
			}
		}
	}
	return nil
}

package sim

import (
	"fmt"
	"sort"
)

// TotalBirths returns cumulative births across macro cohorts and active households.
func (w *World) TotalBirths() int {
	return int(w.TotalBirthsCount)
}

// TotalSick returns currently diseased people across living individual Actors and macro PopulationCohorts.
func (w *World) TotalSick() int {
	sick := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].DiseaseID != "" {
			sick++
		}
	}
	for i := range w.Cohorts {
		sick += w.Cohorts[i].Sick
	}
	return sick
}

func (w *World) recordDeathCause(cause string, count int) {
	if count <= 0 {
		return
	}
	cause = canonicalDeathCause(cause)
	if w.DeathsByCause == nil {
		w.DeathsByCause = map[string]int64{}
	}
	w.DeathsByCause[cause] += int64(count)
}

// initCohortAgePyramid initializes Children, Adults, and Elders so their sum equals Population exactly.
func initCohortAgePyramid(cat Catalog, c *PopulationCohort) {
	if c == nil || c.Population <= 0 {
		c.Children, c.Adults, c.Elders = 0, 0, 0
		return
	}
	// Armed/coercive cohorts are predominantly adults; civilian cohorts hold full family pyramids.
	if cat.HasCoercivePower([]string{c.Profession}) {
		c.Children = c.Population / 20
		c.Elders = c.Population / 12
		c.Adults = c.Population - c.Children - c.Elders
		return
	}
	c.Children = (c.Population * 20) / 100
	c.Elders = (c.Population * 14) / 100
	c.Adults = c.Population - c.Children - c.Elders
}

func (w *World) potablePoolID(ds DistrictState) string {
	chosen := ""
	for id, spec := range w.Catalog.Resources {
		if spec.PotableWater && ds.AvailableResource(id) > 0 && (chosen == "" || id < chosen) {
			chosen = id
		}
	}
	return chosen
}

func (w *World) sanitationInfraCondition(ds DistrictState) (int, bool) {
	if ds.Infrastructure == nil {
		return 50, false
	}
	for infraID, spec := range w.Catalog.Infrastructures {
		if spec.SanitationRole {
			if v, ok := ds.Infrastructure[infraID]; ok {
				return v, true
			}
		}
	}
	if v, ok := ds.Infrastructure["public_latrine"]; ok {
		return v, true
	}
	return 50, false
}

// reconcileCohortAgeBrackets keeps Children + Adults + Elders == Population after any population delta.
func reconcileCohortAgeBrackets(c *PopulationCohort) {
	if c == nil {
		return
	}
	if c.Population <= 0 {
		c.Population = 0
		c.Children, c.Adults, c.Elders = 0, 0, 0
		c.WoundedSick = 0
		c.ChildAgeRemainder, c.AdultAgeRemainder = 0, 0
		if c.Wounded > 0 {
			c.Wounded = 0
		}
		if c.Sick > 0 {
			c.Sick = 0
		}
		if c.Jailed > 0 {
			c.Jailed = 0
		}
		return
	}
	sum := c.Children + c.Adults + c.Elders
	if sum == 0 {
		c.Adults = c.Population
		return
	}
	diff := c.Population - sum
	if diff > 0 {
		c.Adults += diff
	} else if diff < 0 {
		need := -diff
		if c.Adults >= need {
			c.Adults -= need
			need = 0
		} else {
			need -= c.Adults
			c.Adults = 0
		}
		if need > 0 {
			if c.Elders >= need {
				c.Elders -= need
				need = 0
			} else {
				need -= c.Elders
				c.Elders = 0
			}
		}
		if need > 0 {
			c.Children -= need
			if c.Children < 0 {
				c.Children = 0
			}
		}
	}
}

// applyCohortCasualtiesWithCause applies wounded, dead, and jailed counts to a cohort and logs dead to DeathsByCause.
func (w *World) applyCohortCasualtiesWithCause(c *PopulationCohort, wounded, dead, jailed int, cause string) (int, int, int) {
	if c == nil || c.Population <= 0 {
		return 0, 0, 0
	}
	if len(c.Members) > 0 {
		members := c.Members
		anonymous := w.anonymousCohort(*c)
		cw, cd, cj := w.applyCohortCasualtiesWithCause(&anonymous, wounded, dead, jailed, cause)
		anonymous.Members = members
		for _, a := range members {
			w.adjustCohortMember(&anonymous, a, 1)
		}
		*c = anonymous
		return cw, cd, cj
	}
	avail := c.Population
	if cause == "combat" {
		avail = maxInt(0, c.Population-c.Wounded-c.Sick+c.WoundedSick-c.Jailed)
	} else if cause == "old_age" {
		avail = c.Elders
	} else if cause == "disease" {
		avail = c.Sick
	}
	if dead > avail {
		dead = avail
	}
	if dead > 0 {
		oldPopulation := c.Population
		oldWounded, oldSick, oldJailed := c.Wounded, c.Sick, c.Jailed
		energy := w.anonymousCohortEnergy(c, oldPopulation)
		if cause != "starvation" {
			c.ensureResidents(oldPopulation)
			hunger := c.hungerUnits(oldPopulation) - c.moveResidents(nil, dead)
			c.setHungerUnits(oldPopulation-dead, hunger)
		}
		c.Population -= dead
		w.setAnonymousCohortEnergy(c, c.Population, energy-energy*int64(dead)/int64(oldPopulation))
		if cause == "disease" {
			woundedDeaths := c.WoundedSick * dead / c.Sick
			c.Sick -= dead
			c.Wounded -= woundedDeaths
			c.WoundedSick -= woundedDeaths
			c.Jailed = c.Jailed * c.Population / oldPopulation
		} else if cause != "combat" {
			if cause == "starvation" {
				// A lethal ten-point hunger hit is necessarily to an already injured resident.
				c.Wounded -= min(dead, c.Wounded)
			} else {
				c.Wounded = c.Wounded * c.Population / oldPopulation
			}
			c.Sick = c.Sick * c.Population / oldPopulation
			c.WoundedSick = min(c.WoundedSick*c.Population/oldPopulation, c.Wounded, c.Sick)
			c.Jailed = c.Jailed * c.Population / oldPopulation
		}
		c.CumulativeDeaths += dead
		if cause == "old_age" && c.Elders >= dead {
			c.Elders -= dead
		}
		reconcileCohortAgeBrackets(c)
		c.PendingWoundedDeaths += oldWounded - c.Wounded
		if cause != "disease" {
			c.PendingSickDiedOther += oldSick - c.Sick
		}
		c.PendingJailedDeaths += oldJailed - c.Jailed
		w.recordDeathCause(cause, dead)
	}
	eff := c.EffectivePopulation()
	if jailed > eff {
		jailed = eff
	}
	if jailed > 0 {
		c.ensureResidents(c.Population)
		c.updateResidentBandState(jailed, func(b ResidentBand) bool { return !b.Jailed }, func(b *ResidentBand) { b.Jailed = true })
		c.Jailed += jailed
		eff -= jailed
	}
	populationWide := cause == "disaster" || cause == "beast" || cause == "starvation"
	if populationWide {
		// Exposure includes children, elders, sick people and prisoners.
		eff = c.Population - c.Wounded
	}
	if wounded > eff {
		wounded = eff
	}
	if wounded > 0 {
		if cause != "starvation" {
			c.ensureResidents(c.Population)
			c.updateResidentBandState(wounded, func(b ResidentBand) bool { return b.Health == MaxHealth }, func(b *ResidentBand) { b.Health = maxInt(1, MaxHealth-1) })
		}
		if populationWide {
			c.WoundedSick += (c.Sick - c.WoundedSick) * wounded / eff
		}
		c.Wounded += wounded
	}
	return wounded, dead, jailed
}

func actorHasWarmClothing(cat Catalog, a Actor) bool {
	for itemID, qty := range a.Items {
		if qty > 0 && cat.Items[itemID].EquipSlot == "" && cat.Items[itemID].WarmthBonus > 0 {
			return true
		}
	}
	for _, itemID := range a.Equipped {
		if itemID != "" && cat.Items[itemID].WarmthBonus > 0 {
			return true
		}
	}
	return false
}

func actorElderAndExpectancy(lc LifecycleSpec, form *FormSpec) (elderAge, maxExp int) {
	baseElder := lc.ElderMinAge
	if baseElder <= 0 {
		baseElder = 55
	}
	baseMax := lc.MaxExpectancy
	if baseMax <= 0 {
		baseMax = 78
	}
	if form != nil && form.LifeExpectancyYears > 0 {
		maxExp = form.LifeExpectancyYears
		elderAge = (baseElder * maxExp) / baseMax
		if elderAge < 1 {
			elderAge = 1
		}
		return elderAge, maxExp
	}
	return baseElder, baseMax
}

func actorAnnualMortalityPermille(lc LifecycleSpec, form *FormSpec, ageYears int) int {
	elderAge, maxExp := actorElderAndExpectancy(lc, form)
	if ageYears < elderAge {
		return 0
	}
	baseMax := lc.MaxExpectancy
	if baseMax <= 0 {
		baseMax = 78
	}
	ramp := (ageYears - elderAge) * 12
	if form != nil && form.LifeExpectancyYears > 0 && maxExp > 0 {
		ramp = ((ageYears - elderAge) * 12 * baseMax) / maxExp
	}
	permille := lc.ElderAnnualDeathPermille + ramp
	if ageYears >= maxExp {
		permille += 350
	}
	return permille
}

// ActorAnnualMortalityPermille returns the annual old-age mortality permille for an actor of a given FormID and age.
func (w *World) ActorAnnualMortalityPermille(formID string, ageYears int) int {
	var formPtr *FormSpec
	if formID != "" {
		if f, ok := w.Catalog.Forms[formID]; ok {
			formPtr = &f
		}
	}
	return actorAnnualMortalityPermille(w.Catalog.Lifecycle, formPtr, ageYears)
}

// stepActorPhysiologyAndLifecycle settles individual Actor thirst, low-friction drinking & latrine excretion,
// cold exposure, disease progression, annual aging, household births, elder mortality, and successor hydration.
func (w *World) stepActorPhysiologyAndLifecycle() []Event {
	events := w.stepActorPhysiologyAndLifecycleFor(w.Actors, true)
	for ci := range w.Cohorts {
		c := &w.Cohorts[ci]
		for i := range c.Members {
			a := &c.Members[i]
			if a.JailedUntil > 0 && w.Tick >= a.JailedUntil {
				c.Jailed--
				a.JailedUntil = 0
				a.RestUntil = w.Tick + 48
				a.Tags = removeString(a.Tags, "outlaw")
				events = append(events, w.record(a, Action{Kind: "wait"}, "released_from_jail", map[string]any{
					"text": fmt.Sprintf("%s served their Yamen jail sentence and was released", a.Name),
				}))
			}
		}
		before := append([]Actor(nil), c.Members...)
		for i := range c.Members {
			a := &c.Members[i]
			w.advanceActorHunger(a)
			if w.Tick >= a.JailedUntil {
				events = append(events, w.consumeActorRation(a)...)
			}
		}
		events = append(events, w.stepActorPhysiologyAndLifecycleFor(c.Members, false)...)
		w.reconcileCohortMemberChanges(c, before)
	}
	clock := ClockAt(w.Tick)
	if clock.Hour == 20 && clock.Minute == 0 {
		events = append(events, w.settleDailyCivicPayrollAndRetailUpkeep(CalendarAt(w.Tick, w.Catalog.Calendar))...)
	}
	return events
}

func (w *World) stepActorPhysiologyAndLifecycleFor(actors []Actor, allowSuccessors bool) []Event {
	var events []Event
	sixHour := w.Tick%(6*TicksPerHour) == 0
	daily := w.Tick%TicksPerDay == 0
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	yearTicks := int64(cal.DaysInYear) * TicksPerDay
	if yearTicks <= 0 {
		yearTicks = 28 * TicksPerDay
	}
	annual := w.Tick > 0 && w.Tick%yearTicks == 0

	var newlyDeadIndices []int

	for i := range actors {
		a := &actors[i]
		if a.Dead {
			continue
		}
		jailed := w.Tick < a.JailedUntil

		// 1. Every 6 hours: Thirst, low-friction facility drinking & latrine settlement, starvation/dehydration checks
		if sixHour {
			if jailed {
				events = append(events, w.consumeActorRation(a)...)
			}
			a.Thirst++

			// A traveler can use carried supplies, but cannot reach a stationary
			// district water source before arriving.
			if !a.InTransit() {
				if pool := w.potablePoolID(w.DistrictStates[a.Location]); pool != "" && w.drawDistrictResource(a.Location, pool, 1) == 1 {
					a.Thirst = 0
				}
			}

			if a.Thirst >= 2 {
				// Drink carried potable item automatically before thirst becomes severe
				for itemID, qty := range a.Items {
					spec := w.Catalog.Items[itemID]
					if qty > 0 && spec.Potable {
						w.adjustActorItem(a, itemID, -1, true)
						relief := spec.ThirstRelief
						if relief <= 0 {
							relief = 2
						}
						a.Thirst -= relief
						if a.Thirst < 0 {
							a.Thirst = 0
						}
						break
					}
				}
			}

			// Low-friction excretion settlement: indoor latrine/compost_pit or public_latrine converts waste cleanly into nightsoil
			if a.Waste >= 2 && !a.InTransit() {
				cleanLatrine := false
				if a.InsideHouse != "" {
					if h := w.house(a.InsideHouse); h != nil && (h.HasFacility("compost_pit") || h.HasFacility("latrine") || h.Level >= 1) {
						cleanLatrine = true
					}
				} else if w.DistrictStates == nil {
					cleanLatrine = true
				} else if ds, ok := w.DistrictStates[a.Location]; ok {
					if cond, hasSan := w.sanitationInfraCondition(ds); hasSan && cond >= 30 {
						cleanLatrine = true
					}
				}
				if cleanLatrine {
					if divisor := w.Catalog.Lifecycle.WastePerFoodDivisor; divisor > 0 && w.Catalog.Lifecycle.WasteItem != "" {
						waste := a.DigestedFood / divisor
						a.DigestedFood -= waste * divisor
						w.adjustDistrictNightsoil(a.Location, w.Catalog.Lifecycle.WasteItem, waste, true)
					}
					a.Waste = 0
				} else if a.Waste >= 4 {
					// Forced open-air relief degrades district sanitation!
					if w.DistrictStates != nil {
						if ds, ok := w.DistrictStates[a.Location]; ok {
							ds.Sanitation -= 2
							if ds.Sanitation < 10 {
								ds.Sanitation = 10
							}
							w.DistrictStates[a.Location] = ds
						}
					}
					a.Waste = 0
				}
			}

			// Severe dehydration / starvation damage
			if a.Thirst >= 6 {
				a.Health -= 10
				if a.Health <= 0 {
					evs := w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "dehydration")
					events = append(events, evs...)
					newlyDeadIndices = append(newlyDeadIndices, i)
					continue
				}
			}
			form, _ := w.actorFormSpec(*a)
			if damage := starvationDamage(a.Hunger, w.Catalog.Lifecycle, form); damage > 0 {
				a.Health -= damage
				if a.Health <= 0 {
					evs := w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "starvation")
					events = append(events, evs...)
					newlyDeadIndices = append(newlyDeadIndices, i)
					continue
				}
			}

			// Severe weather cold exposure when outdoors without warm clothing
			if w.DistrictStates != nil && a.InsideHouse == "" {
				if ds, ok := w.DistrictStates[a.Location]; ok {
					if ws, hasW := w.Catalog.Weathers[ds.Weather]; hasW && ws.ColdDamage > 0 && !actorHasWarmClothing(w.Catalog, *a) {
						a.Energy -= ws.ColdDamage * 2
						if a.Energy < 0 {
							a.Energy = 0
						}
						a.Health -= ws.ColdDamage
						if a.Health <= 0 {
							events = append(events, w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "cold")...)
							newlyDeadIndices = append(newlyDeadIndices, i)
							continue
						}
					}
				}
			}
		}

		// 2. Daily disease progression & contagion check (active when DistrictStates is enabled or actor is already sick)
		if daily && !a.Dead {
			if a.DiseaseID != "" {
				var drain int = 8
				for _, dSpec := range w.Catalog.Diseases {
					if dSpec.ID == a.DiseaseID {
						drain = maxInt(2, dSpec.HealthDrainPerDay/2)
						break
					}
				}
				a.Health -= drain
				a.DiseaseTicks -= TicksPerDay
				if a.Health <= 0 {
					evs := w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "disease")
					events = append(events, evs...)
					newlyDeadIndices = append(newlyDeadIndices, i)
					continue
				}
				if a.DiseaseTicks <= 0 {
					curedID := a.DiseaseID
					a.DiseaseID = ""
					a.DiseaseTicks = 0
					events = append(events, w.record(a, Action{Kind: "wait"}, "recovered_disease", map[string]any{
						"text":       fmt.Sprintf("%s recovered from %s", a.Name, curedID),
						"disease_id": curedID,
					}))
				}
			} else if w.DistrictStates != nil {
				if ds, ok := w.DistrictStates[a.Location]; ok {
					ws := w.Catalog.Weathers[ds.Weather]
					for _, dSpec := range w.Catalog.Diseases {
						triggered := false
						if dSpec.MaxSanitationTrigger > 0 && ds.Sanitation <= dSpec.MaxSanitationTrigger {
							triggered = true
						}
						if dSpec.ColdTrigger && ws.ColdDamage > 0 && a.InsideHouse == "" && !actorHasWarmClothing(w.Catalog, *a) {
							triggered = true
						}
						if !triggered {
							continue
						}
						roll := int(worldHash(w.ID, w.Tick, "act_dis:"+a.ID+":"+dSpec.ID) % 1000)
						if roll < dSpec.ContagionPermille {
							a.DiseaseID = dSpec.ID
							a.DiseaseTicks = dSpec.DurationTicks
							events = append(events, w.record(a, Action{Kind: "wait"}, "contracted_disease", map[string]any{
								"text":         fmt.Sprintf("%s contracted %s in %s (sanitation %d)", a.Name, dSpec.Name, a.Location, ds.Sanitation),
								"disease_id":   dSpec.ID,
								"disease_name": dSpec.Name,
								"district":     a.Location,
							}))
							break
						}
					}
				}
			}
		}

		// 3. Annual lifecycle settlement: +1 year age, prime-adult household birth, elder attribute & natural mortality
		if annual && !a.Dead {
			a.AgeYears++
			lc := w.Catalog.Lifecycle
			var formPtr *FormSpec
			if form, ok := w.actorFormSpec(*a); ok {
				formPtr = &form
			}
			elderAge, maxExp := actorElderAndExpectancy(lc, formPtr)

			// Household birth when prime adult has home, food security, and cohorts exist to welcome the child
			edibleStock := 0
			for itemID, qty := range a.Items {
				if qty > 0 && w.Catalog.Items[itemID].Edible {
					edibleStock += qty
				}
			}
			if len(w.Cohorts) > 0 && a.HomeID != "" && a.AgeYears >= 18 && a.AgeYears < elderAge && a.Hunger <= 1 && edibleStock >= lc.FertilityMinFoodPerCapita {
				roll := int(worldHash(w.ID, w.Tick, "birth:"+a.ID) % 1000)
				if roll < lc.AnnualBirthPermille*4 {
					for ci := range w.Cohorts {
						if w.Cohorts[ci].District == a.Location {
							w.Cohorts[ci].Population++
							w.Cohorts[ci].Children++
							w.Cohorts[ci].CumulativeBirths++
							w.TotalBirthsCount++
							events = append(events, w.record(a, Action{Kind: "wait"}, "actor_child_born", map[string]any{
								"text":      fmt.Sprintf("%s welcomed a newborn child into household %s", a.Name, a.HomeID),
								"house_id":  a.HomeID,
								"cohort_id": w.Cohorts[ci].ID,
							}))
							break
						}
					}
				}
			}

			// Elder wisdom & physical frailty + natural old-age mortality
			if a.AgeYears >= elderAge {
				if a.AgeYears == elderAge {
					a.Attributes.Insight++
					if a.Attributes.Strength > 6 {
						a.Attributes.Strength--
					}
				}
				_ = maxExp
				deathPermille := actorAnnualMortalityPermille(lc, formPtr, a.AgeYears)
				roll := int(worldHash(w.ID, w.Tick, "elder_die:"+a.ID) % 1000)
				if roll < deathPermille {
					a.Health = 0
					evs := w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "old_age")
					events = append(events, evs...)
					newlyDeadIndices = append(newlyDeadIndices, i)
				}
			}
		}
	}

	// 4. Auto-hydrate successors from local cohorts for any newly deceased active focal actors
	if allowSuccessors && len(newlyDeadIndices) > 0 && len(w.Cohorts) > 0 && w.Catalog.Lifecycle.AutoHydrateSuccessor {
		events = append(events, w.hydrateSuccessorsForDead(newlyDeadIndices)...)
	}

	return events
}

func (w *World) hydrateSuccessorsForDead(deadIndices []int) []Event {
	var events []Event
	livingActors := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			livingActors++
		}
	}
	if livingActors >= 132 {
		return nil
	}
	for _, dIdx := range deadIndices {
		if livingActors >= 132 {
			break
		}
		if dIdx < 0 || dIdx >= len(w.Actors) {
			continue
		}
		deceased := w.Actors[dIdx]
		var chosenCohortID string
		for ci := range w.Cohorts {
			c := &w.Cohorts[ci]
			if c.EffectivePopulation() >= 2 && c.Profession == deceased.Role {
				chosenCohortID = c.ID
				break
			}
		}
		if chosenCohortID == "" {
			for ci := range w.Cohorts {
				if w.Cohorts[ci].EffectivePopulation() >= 5 {
					chosenCohortID = w.Cohorts[ci].ID
					break
				}
			}
		}
		if chosenCohortID == "" {
			continue
		}
		succ, err := w.HydrateActor(chosenCohortID)
		if err != nil || succ == nil {
			continue
		}
		livingActors++
		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  succ.ID,
			TargetID: deceased.ID,
			Kind:     "actor_succeeded",
			Action:   Action{Kind: "wait", Location: succ.Location},
			Payload: map[string]any{
				"text":           fmt.Sprintf("%s stepped forward from %s to succeed the late %s in %s", succ.Name, chosenCohortID, deceased.Name, succ.Location),
				"successor_id":   succ.ID,
				"predecessor_id": deceased.ID,
				"cohort_id":      chosenCohortID,
			},
		}
		w.NextID++
		events = append(events, ev)
	}
	return events
}

// stepCohortPhysiologySanitationAndLifecycle settles macro cohort water consumption, nightsoil-to-compost
// recycling, district sanitation, epidemic outbreaks, daily births & elder mortality, and annual age maturation.
func (w *World) stepCohortPhysiologySanitationAndLifecycle() []Event {
	if len(w.Cohorts) == 0 {
		return nil
	}
	restoreMembers := w.detachCohortMembers()
	defer restoreMembers()
	var events []Event
	clock := ClockAt(w.Tick)
	nextTick := w.Tick + 1
	sixHour := nextTick%(6*TicksPerHour) == 0
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	daysInYear := cal.DaysInYear
	if daysInYear <= 0 {
		daysInYear = 28
	}

	// 1. Every 6 hours: potable water consumption, dehydration/starvation check, and excretion -> nightsoil -> compost!
	if sixHour {
		popByDistrict := map[string]int{}

		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.Population <= 0 {
				continue
			}
			reconcileCohortAgeBrackets(c)
			popByDistrict[c.District] += c.Population

			// Potable water draw from local water_table or ItemPool["water"]
			c.AvgThirst++
			waterNeed := int64(maxInt(1, c.Population/2))
			remainingWater := waterNeed
			for remainingWater > 0 {
				pool := w.potablePoolID(w.DistrictStates[c.District])
				if pool == "" {
					break
				}
				remainingWater -= w.drawDistrictResource(c.District, pool, remainingWater)
			}
			waterItems := make([]string, 0)
			for itemID, spec := range w.Catalog.Items {
				if spec.Potable {
					waterItems = append(waterItems, itemID)
				}
			}
			sort.Strings(waterItems)
			for _, itemID := range waterItems {
				use := min(remainingWater, c.ItemPool[itemID])
				if use <= 0 {
					continue
				}
				w.adjustCohortItem(c, itemID, -use, true)
				remainingWater -= use
			}
			delivered := waterNeed - remainingWater
			reliefNumerator := delivered*int64(c.AvgThirst) + c.ThirstReliefRemainder
			c.AvgThirst -= int(reliefNumerator / waterNeed)
			c.ThirstReliefRemainder = reliefNumerator % waterNeed
			if c.AvgThirst == 0 {
				c.ThirstReliefRemainder = 0
			}

			// Prefecture granary anti-famine relief when food reserves exist across agricultural cohorts
			if c.AvgHunger >= 3 {
				hasActiveDisaster := false
				if w.DistrictStates != nil {
					if ds, ok := w.DistrictStates[c.District]; ok && len(ds.ActiveDisasters) > 0 {
						hasActiveDisaster = true
					}
				}
				if !hasActiveDisaster || c.AvgHunger >= 4 {
					reliefNeed := int64(c.Population)
					for itemID, spec := range w.Catalog.Items {
						if !spec.Edible || reliefNeed <= 0 {
							continue
						}
						for di := range w.Cohorts {
							donor := &w.Cohorts[di]
							if donor.District != c.District || donor.ID == c.ID || donor.Population <= 0 {
								continue
							}
							availSurplus := w.cohortFoodSurplus(*donor, itemID)
							grant := (reliefNeed + max(int64(1), spec.NutritionUnits) - 1) / max(int64(1), spec.NutritionUnits)
							if grant > availSurplus {
								grant = availSurplus
							}
							if grant <= 0 {
								continue
							}
							batchQty := int64(maxInt(1, spec.BatchQty))
							unitPrice := int64(maxInt(1, spec.BatchPrice))
							reliefCost := ((grant + batchQty - 1) / batchQty) * unitPrice
							var selfPaid, stateSubsidy int64
							if c.MoneyPool >= reliefCost {
								// Ever-Normal Granary price-stabilization sale (常平仓平粜): guild pays 80%, state subsidizes 20% when treasury has reserve
								if w.PublicTreasury > w.TreasuryLedger.InitialTreasury/2 {
									selfPaid = (reliefCost * 80) / 100
									stateSubsidy = reliefCost - selfPaid
								} else {
									selfPaid = reliefCost
								}
							} else {
								// Indigent famine relief (义仓赈济): household pays available cash, state covers deficit within prudent fiscal cap
								selfPaid = c.MoneyPool
								stateSubsidy = reliefCost - selfPaid
							}
							var maxSubsidy int64
							if reserveFloor := w.TreasuryLedger.InitialTreasury / 2; w.PublicTreasury > reserveFloor {
								maxSubsidy = (w.PublicTreasury - reserveFloor) / 28
							}
							if stateSubsidy > maxSubsidy {
								stateSubsidy = maxSubsidy
							}
							if c.MoneyPool >= reliefCost {
								selfPaid = reliefCost - stateSubsidy
							}
							// Pay only for the quantity supported by the actual assessed
							// household and public budgets. A capped subsidy is not paid money.
							grant = min(grant, (selfPaid+stateSubsidy)/unitPrice*batchQty)
							if grant == 0 {
								continue
							}
							reliefCost = ((grant + batchQty - 1) / batchQty) * unitPrice
							selfPaid = min(selfPaid, reliefCost)
							stateSubsidy = reliefCost - selfPaid
							if selfPaid > 0 {
								c.MoneyPool -= selfPaid
								donor.MoneyPool += selfPaid
							}
							if stateSubsidy > 0 {
								if paid := w.debitTreasury(stateSubsidy, "relief"); paid > 0 {
									donor.MoneyPool += paid
									if collector := w.findCollectorCohort(c.District); collector != nil && w.AvgCorruption() >= 16 {
										if emb := w.debitTreasury(maxInt64(1, paid/9), "embezzled"); emb > 0 {
											collector.MoneyPool += emb
											collector.IllicitWealth += emb
											events = append(events, Event{
												ID:      w.NextID,
												Tick:    w.Tick,
												ActorID: collector.ID,
												Kind:    "cohort_embezzled",
												Action:  Action{Kind: "wait", Location: c.District, Price: int(emb)},
												Payload: map[string]any{
													"text":            fmt.Sprintf("%s skimmed %d relief-procurement coins from granary subsidy in %s", collector.Name, emb, c.District),
													"embezzled_coins": emb,
													"relief_paid":     paid,
													"corruption_idx":  w.AvgCorruption(),
												},
											})
											w.NextID++
										}
									}
								}
							}
							w.adjustCohortItem(donor, itemID, -grant, false)
							w.adjustCohortItem(c, itemID, grant, false)
							reliefNeed -= grant * max(int64(1), spec.NutritionUnits)
							events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: c.ID, TargetID: donor.ID,
								Kind: "cohort_food_relief_purchased", Action: Action{Kind: "accept", TargetID: donor.ID, Item: itemID, Quantity: int(grant), Price: int(reliefCost)},
								Payload: map[string]any{"item": itemID, "quantity": grant, "household_paid": selfPaid, "state_paid": stateSubsidy}})
							w.NextID++
							if reliefNeed <= 0 {
								break
							}
						}
					}
				}
			}

			if c.AvgThirst >= 5 {
				thirstReq := maxInt(1, c.Population/2500)
				cw, dead, _ := w.applyCohortCasualtiesWithCause(c, thirstReq*2, thirstReq, 0, "dehydration")
				if cw > 0 || dead > 0 {
					ev := Event{
						ID: w.NextID, Tick: w.Tick, ActorID: c.ID, Kind: "cohort_dehydration_deaths",
						Action: Action{Kind: "wait", Location: c.District},
						Payload: map[string]any{
							"text":      fmt.Sprintf("%s suffered %d dehydration deaths (%d weakened) in %s", c.Name, dead, cw, c.District),
							"cohort_id": c.ID, "wounded": cw, "deaths": dead, "cause": "dehydration",
						},
					}
					w.NextID++
					events = append(events, ev)
				}
			}

			// Excretion comes from food actually consumed, including the retained fractional remainder.
			if divisor := w.Catalog.Lifecycle.WastePerFoodDivisor; divisor > 0 && w.Catalog.Lifecycle.WasteItem != "" {
				waste := c.DigestedFood / divisor
				c.DigestedFood -= waste * divisor
				w.adjustDistrictNightsoil(c.District, w.Catalog.Lifecycle.WasteItem, waste, true)
			}
		}

		// Settle district latrine sanitation & convert collected nightsoil into agricultural compost!
		if w.DistrictStates != nil {
			distKeys := make([]string, 0, len(w.DistrictStates))
			for k := range w.DistrictStates {
				distKeys = append(distKeys, k)
			}
			sort.Strings(distKeys)

			wasteItem := w.Catalog.Lifecycle.WasteItem

			for _, dID := range distKeys {
				ds := w.DistrictStates[dID]
				ns := ds.NightsoilPool
				latrineCond, _ := w.sanitationInfraCondition(ds)
				collectionPct := 55 + (latrineCond*40)/100 // up to 95% collected when public sanitation facilities are maintained!
				if collectionPct > 96 {
					collectionPct = 96
				}
				collected := (ns * int64(collectionPct)) / 100
				w.adjustDistrictNightsoil(dID, wasteItem, -(ns - collected), true)
				if wasteItem != "" && collected > 0 {
					for ci := range w.Cohorts {
						c := &w.Cohorts[ci]
						if c.Population > 0 && c.District == dID && w.Catalog.TagsConsume(cohortIdentityTags(w.Catalog, c.Profession), wasteItem) {
							w.adjustDistrictNightsoil(dID, wasteItem, -collected, false)
							w.adjustCohortItem(c, wasteItem, collected, false)
							break
						}
					}
				}
				ds = w.DistrictStates[dID]

				// Sanitation balance: crowding & uncollected waste vs latrine & clean water recovery
				dPop := popByDistrict[dID]
				crowdingPenalty := dPop / 65000
				if latrineCond >= 70 && len(ds.ActiveDisasters) == 0 {
					ds.Sanitation += 2 - crowdingPenalty
				} else {
					ds.Sanitation -= 2 + crowdingPenalty
				}
				if ds.Sanitation > 96 {
					ds.Sanitation = 96
				}
				if ds.Sanitation < 18 {
					ds.Sanitation = 18
				}
				w.DistrictStates[dID] = ds
			}

		}
	}

	// 2. Daily Civic Stipends, Household/Retail Upkeep, Epidemic Settlement, Births & Elder Mortality (at 20:00)
	if clock.Hour == 20 && clock.Minute == 0 {
		lc := w.Catalog.Lifecycle
		birthPermille := lc.AnnualBirthPermille
		if birthPermille <= 0 {
			birthPermille = 22
		}
		elderDeathPermille := lc.ElderAnnualDeathPermille
		if elderDeathPermille <= 0 {
			elderDeathPermille = 36
		}
		maxDiseaseLethality := 22
		for _, dSpec := range w.Catalog.Diseases {
			if dSpec.LethalityPermille > maxDiseaseLethality {
				maxDiseaseLethality = dSpec.LethalityPermille
			}
		}

		// Gather district healer medicine capacity for public epidemic relief
		healerMedByDist := map[string]*PopulationCohort{}
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.EffectivePopulation() <= 0 {
				continue
			}
			if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok && (ActorHasTag(prof.Grants, "healer") || w.Catalog.Capabilities(Actor{Tags: prof.Grants})["heal"]) {
				healerMedByDist[c.District] = c
			}
		}

		itemIDs := make([]string, 0, len(w.Catalog.Items))
		for k := range w.Catalog.Items {
			itemIDs = append(itemIDs, k)
		}
		sort.Strings(itemIDs)

		var dailyBirths, dailyElderDeaths, dailyNewSick, dailyCured, dailyDiseaseDeaths int
		var dailyMedUsed int64

		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.Population <= 0 {
				continue
			}
			reconcileCohortAgeBrackets(c)

			// 2a. Settle medical treatment, immune recovery, and mortality on PRE-EXISTING Sick cohort members FIRST
			if c.Sick > 0 {
				cured := 0
				maxSelfCure := maxInt(1, (c.Sick*45)/100)
				for _, itemID := range itemIDs {
					spec := w.Catalog.Items[itemID]
					if spec.DiseaseCure > 0 && c.ItemPool[itemID] > 0 && cured < maxSelfCure {
						rem := maxSelfCure - cured
						need := int64((rem + spec.DiseaseCure*2 - 1) / (spec.DiseaseCure * 2))
						if need > c.ItemPool[itemID] {
							need = c.ItemPool[itemID]
						}
						w.adjustCohortItem(c, itemID, -need, true)
						dailyMedUsed += need
						h := int(need) * spec.DiseaseCure * 2
						if h > rem {
							h = rem
						}
						cured += h
					}
				}
				// District healer clinic support on remaining sick (up to 25% daily clinic capacity)
				if c.Sick-cured > 0 {
					maxClinicCure := maxInt(1, ((c.Sick-cured)*25)/100)
					clinicCured := 0
					hc := healerMedByDist[c.District]

					if hc != nil && hc.ID != c.ID {
						for _, itemID := range itemIDs {
							spec := w.Catalog.Items[itemID]
							if spec.DiseaseCure > 0 && hc.ItemPool[itemID] > 10 && clinicCured < maxClinicCure {
								rem := maxClinicCure - clinicCured
								need := int64((rem + spec.DiseaseCure*2 - 1) / (spec.DiseaseCure * 2))
								if need > hc.ItemPool[itemID]/6 {
									need = hc.ItemPool[itemID] / 6
								}
								if need <= 0 {
									continue
								}
								fee := need * int64(maxInt(1, spec.BatchPrice))
								if c.MoneyPool >= fee {
									c.MoneyPool -= fee
									hc.MoneyPool += fee
								} else if w.PublicTreasury >= fee && fee <= maxInt64(50, w.PublicTreasury/35) {
									paidFee := w.debitTreasury(fee, "relief")
									hc.MoneyPool += paidFee
								} else {
									continue
								}
								w.adjustCohortItem(hc, itemID, -need, true)
								dailyMedUsed += need
								h := int(need) * spec.DiseaseCure * 2
								if h > rem {
									h = rem
								}
								cured += h
								clinicCured += h
							}
						}
					}
				}
				// Natural immune recovery + fractional stochastic disease mortality on remaining untreated cases
				remSick := c.Sick - cured
				if remSick > 0 {
					immuneRecover := maxInt(1, (remSick*22)/100)
					if immuneRecover > remSick {
						immuneRecover = remSick
					}
					cured += immuneRecover
					remSick -= immuneRecover
				}
				c.Sick -= cured
				c.WoundedSick = min(c.WoundedSick, c.Wounded, c.Sick)
				dailyCured += cured

				if remSick > 0 {
					numer := int64(remSick) * int64(maxDiseaseLethality)
					disDead := int(numer / 1000)
					if rem := numer % 1000; rem > 0 && int64(worldHash(w.ID, w.Tick, "cdisdead:"+c.ID)%1000) < rem {
						disDead++
					}
					if disDead > remSick {
						disDead = remSick
					}
					if disDead > 0 {
						_, actual, _ := w.applyCohortCasualtiesWithCause(c, 0, disDead, 0, "disease")
						dailyDiseaseDeaths += actual
					}
				}
			}

			// 2b. Contagious & cold-exposure NEW infections (enter c.Sick for subsequent days' progression)
			if w.DistrictStates != nil {
				if ds, ok := w.DistrictStates[c.District]; ok {
					ws := w.Catalog.Weathers[ds.Weather]
					hasWarmth := false
					for _, itemID := range itemIDs {
						spec := w.Catalog.Items[itemID]
						if spec.WarmthBonus > 0 && c.ItemPool[itemID] >= int64(maxInt(1, c.Population/5)) {
							hasWarmth = true
							break
						}
					}
					for _, dSpec := range w.Catalog.Diseases {
						triggered := false
						if dSpec.MaxSanitationTrigger > 0 && ds.Sanitation <= dSpec.MaxSanitationTrigger {
							triggered = true
						}
						if dSpec.ColdTrigger && ws.ColdDamage > 0 && !hasWarmth {
							triggered = true
						}
						if !triggered {
							continue
						}
						roll := int(worldHash(w.ID, w.Tick, "coh_dis:"+c.ID+":"+dSpec.ID) % 1000)
						if roll < dSpec.ContagionPermille*6 {
							healthy := c.EffectivePopulation()
							infected := maxInt(1, (healthy*dSpec.ContagionPermille)/1000)
							if infected > healthy/8 {
								infected = healthy / 8
							}
							if infected > 0 {
								c.Sick += infected
								dailyNewSick += infected
							}
							break
						}
					}
				}
			}

			// 2c. Daily Births (生: newborns enter Children when food-secure in non-outlaw households)
			isOutlawCohort := false
			if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok && ActorHasTag(prof.Grants, "outlaw") {
				isOutlawCohort = true
			}
			if c.Adults >= 10 && c.AvgHunger <= 1 && !isOutlawCohort {
				// Exact fractional stochastic rounding via worldHash so small and large cohorts both reproduce smoothly
				numer := int64(c.Adults) * int64(birthPermille)
				denom := int64(1000 * daysInYear)
				births := int(numer / denom)
				rem := numer % denom
				if rem > 0 && int64(worldHash(w.ID, w.Tick, "cbirth:"+c.ID)%uint64(denom)) < rem {
					births++
				}
				if births > 0 {
					c.ensureResidents(c.Population)
					c.issueResidents(births, 0)
					hunger := c.hungerUnits(c.Population)
					c.Population += births
					c.setHungerUnits(c.Population, hunger)
					c.Children += births
					c.CumulativeBirths += births
					w.TotalBirthsCount += int64(births)
					dailyBirths += births
				}
			}

			// 2d. Daily Natural Elder Mortality (老死: natural end-of-life curve among Elders)
			if c.Elders > 0 {
				numer := int64(c.Elders) * int64(elderDeathPermille)
				denom := int64(1000 * daysInYear)
				eDeaths := int(numer / denom)
				rem := numer % denom
				if rem > 0 && int64(worldHash(w.ID, w.Tick, "celder:"+c.ID)%uint64(denom)) < rem {
					eDeaths++
				}
				if eDeaths > c.Elders {
					eDeaths = c.Elders
				}
				if eDeaths > 0 {
					_, actualD, _ := w.applyCohortCasualtiesWithCause(c, 0, eDeaths, 0, "old_age")
					dailyElderDeaths += actualD
				}
			}

			// 2e. End-of-Year Age Bracket Progression (少 -> 壮 -> 老) on final day of each calendar year!
			if cal.DayOfYear == daysInYear {
				// Keep the existing 12/32-year transition rates without discarding
				// the fractional years of small populations.
				c.ChildAgeRemainder += c.Children
				c.AdultAgeRemainder += c.Adults
				matured := c.ChildAgeRemainder / 12
				retired := c.AdultAgeRemainder / 32
				c.ChildAgeRemainder %= 12
				c.AdultAgeRemainder %= 32
				if matured > c.Children {
					matured = c.Children
				}
				if retired > c.Adults {
					retired = c.Adults
				}
				c.Children -= matured
				c.Adults += matured - retired
				c.Elders += retired
				reconcileCohortAgeBrackets(c)
			}
		}

		if dailyNewSick > 0 || dailyCured > 0 || dailyDiseaseDeaths > 0 {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "cohort_epidemic",
				Action:  Action{Kind: "medicate"},
				Payload: map[string]any{
					"text":          fmt.Sprintf("Prefecture public health report: +%d newly sick, %d cured (%d medicine/tonics used), %d epidemic deaths", dailyNewSick, dailyCured, dailyMedUsed, dailyDiseaseDeaths),
					"new_sick":      dailyNewSick,
					"cured":         dailyCured,
					"medicine_used": dailyMedUsed,
					"deaths":        dailyDiseaseDeaths,
					"cause":         "disease",
				},
			}
			w.NextID++
			events = append(events, ev)
		}

		if dailyBirths > 0 || dailyElderDeaths > 0 {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "cohort_demographics",
				Action:  Action{Kind: "wait"},
				Payload: map[string]any{
					"text":         fmt.Sprintf("[%s Y%d D%d] Prefecture vital statistics: +%d newborns, %d peaceful elder deaths", cal.SeasonName, cal.Year, cal.DayOfSeason, dailyBirths, dailyElderDeaths),
					"births":       dailyBirths,
					"elder_deaths": dailyElderDeaths,
					"year":         cal.Year,
					"season":       cal.SeasonID,
					"cause":        "old_age",
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}

	return events
}

func (w *World) isCivicServiceProfession(profID string) bool {
	prof, ok := w.Catalog.ProfessionByID(profID)
	if !ok || ActorHasTag(prof.Grants, "outlaw") {
		return false
	}
	if w.Catalog.HasPublicTrustDuty([]string{profID}) {
		return true
	}
	caps := w.Catalog.Capabilities(Actor{Tags: prof.Grants})
	return caps["arrest"] || caps["heal"] || caps["perform_ritual"] || caps["spar"]
}

// settleDailyCivicPayrollAndRetailUpkeep disburses prefecture treasury stipends to non-outlaw civic
// service cohorts and focal actors (including payroll/admin embezzlement leakage), settles merchant retail
// turnover with commercial sales tax, and applies daily structural & cold-season wear on durable household goods.
func (w *World) settleDailyCivicPayrollAndRetailUpkeep(cal CalendarState) []Event {
	var events []Event
	gov := w.Catalog.EffectiveGovernance()
	stipendPerCap := int64(gov.CivicStipendPerCap)

	// 1. Civic Payroll from w.PublicTreasury to public-trust/law/healing/ritual/academy/martial posts
	if w.PublicTreasury > 0 {
		budget := w.PublicTreasury / 6
		var totalReq int64
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.Population > 0 && w.isCivicServiceProfession(c.Profession) {
				totalReq += int64(c.Population) * stipendPerCap
			}
		}
		for i := range w.Actors {
			a := &w.Actors[i]
			if !a.Dead && w.Tick >= a.JailedUntil && w.isCivicServiceProfession(a.Role) {
				totalReq += stipendPerCap + 1
			}
		}
		if totalReq > 0 && budget > 0 {
			// One shared assessment covers cohorts and focal actors, including any
			// payroll diverted to collectors. Carry integer remainders across payees.
			budget = min(budget, totalReq)
			var requestedThrough, allocatedThrough int64
			allocatePay := func(req int64) int64 {
				requestedThrough += req
				entitledThrough := requestedThrough * budget / totalReq
				pay := entitledThrough - allocatedThrough
				allocatedThrough = entitledThrough
				return pay
			}
			var totalPaid, totalEmbezzled int64
			corr := w.AvgCorruption()
			embPermille := gov.EmbezzleBasePermille + corr*2
			if embPermille > 240 {
				embPermille = 240
			}
			for i := range w.Cohorts {
				c := &w.Cohorts[i]
				if c.Population <= 0 || !w.isCivicServiceProfession(c.Profession) {
					continue
				}
				req := int64(c.Population) * stipendPerCap
				pay := allocatePay(req)
				if pay <= 0 || w.PublicTreasury <= 0 {
					continue
				}
				collector := w.findCollectorCohort(c.District)
				var embReq int64
				if collector != nil {
					embReq = (pay * int64(embPermille)) / 1000
				}
				honestPay := w.debitTreasury(pay-embReq, "payroll")
				if honestPay > 0 {
					c.MoneyPool += honestPay
					totalPaid += honestPay
				}
				if embReq > 0 && collector != nil {
					embActual := w.debitTreasury(embReq, "embezzled")
					if embActual > 0 {
						collector.MoneyPool += embActual
						collector.IllicitWealth += embActual
						totalEmbezzled += embActual
					}
				}
			}
			for i := range w.Actors {
				a := &w.Actors[i]
				if a.Dead || w.Tick < a.JailedUntil || !w.isCivicServiceProfession(a.Role) {
					continue
				}
				paid := w.debitTreasury(allocatePay(stipendPerCap+1), "payroll")
				if paid > 0 {
					a.Money += int(paid)
					totalPaid += paid
				}
			}
			if totalEmbezzled > 0 {
				embEv := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: "world",
					Kind:    "cohort_embezzled",
					Action:  Action{Kind: "wait", Price: int(totalEmbezzled)},
					Payload: map[string]any{
						"text":            fmt.Sprintf("Corrupt clerks and officers siphoned %d coins from daily public payroll & administrative disbursements (%d honest stipends paid)", totalEmbezzled, totalPaid),
						"embezzled_coins": totalEmbezzled,
						"payroll_paid":    totalPaid,
						"corruption_idx":  corr,
					},
				}
				w.NextID++
				events = append(events, embEv)
			}
		}
	}

	// 2. Daily household structural & cold-season wear on non-producer cohorts
	structuralItems := map[string]bool{}
	for _, inf := range w.Catalog.Infrastructures {
		for inItem := range inf.BuildInputs {
			structuralItems[inItem] = true
		}
	}
	isColdSeason := cal.SeasonIndex >= 0 && cal.SeasonIndex < len(w.Catalog.Calendar.Seasons) &&
		w.Catalog.Calendar.Seasons[cal.SeasonIndex].ColdExposure > 0

	itemKeys := make([]string, 0, len(w.Catalog.Items))
	for k := range w.Catalog.Items {
		itemKeys = append(itemKeys, k)
	}
	sort.Strings(itemKeys)

	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 || len(c.ItemPool) == 0 {
			continue
		}
		tags := cohortIdentityTags(w.Catalog, c.Profession)
		for _, itemID := range itemKeys {
			qty := c.ItemPool[itemID]
			if qty <= 0 || w.Catalog.TagsProduce(tags, itemID) {
				continue
			}
			spec := w.Catalog.Items[itemID]
			var wear int64
			if structuralItems[itemID] {
				wear = qty / 28
			} else if isColdSeason && spec.WarmthBonus > 0 {
				wear = qty / 18
			}
			if wear > 0 {
				w.adjustCohortItem(c, itemID, -wear, true)
			}
		}
	}

	// 3. Merchant retail turnover: TradeMargin cohorts sell consumer/craft/cultural inventory to prosperous households
	for mi := range w.Cohorts {
		m := &w.Cohorts[mi]
		if m.Population <= 0 || len(m.ItemPool) == 0 {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(m.Profession)
		if !ok || !prof.TradeMargin {
			continue
		}
		mTags := cohortIdentityTags(w.Catalog, m.Profession)
		for _, itemID := range itemKeys {
			spec := w.Catalog.Items[itemID]
			if spec.Edible || spec.Potable || w.Catalog.TagsProduce(mTags, itemID) {
				continue
			}
			floor := int64(m.Population) / 2
			if m.ItemPool[itemID] <= floor {
				continue
			}
			retailQty := (m.ItemPool[itemID] - floor) / 6
			if retailQty <= 0 {
				continue
			}
			batchQty := int64(maxInt(1, spec.BatchQty))
			unitPrice := int64(maxInt(1, spec.BatchPrice))

			// Settle each household purchase as a transfer of both coins and goods.
			remaining := retailQty
			for hi := range w.Cohorts {
				if hi == mi || remaining < batchQty {
					continue
				}
				h := &w.Cohorts[hi]
				if h.District != m.District || h.Population <= 0 {
					continue
				}
				if hp, ok := w.Catalog.ProfessionByID(h.Profession); ok && hp.TradeMargin {
					continue
				}
				surplus := h.MoneyPool - int64(h.Population)*12
				if surplus <= 0 {
					continue
				}
				budget := min(surplus, surplus/10+1)
				batches := min(remaining/batchQty, budget/unitPrice)
				if batches == 0 {
					continue
				}
				qty, cost := batches*batchQty, batches*unitPrice
				h.MoneyPool -= cost
				if h.ItemPool == nil {
					h.ItemPool = map[string]int64{}
				}
				h.ItemPool[itemID] += qty
				m.ItemPool[itemID] -= qty
				remaining -= qty
				tax := cost * int64(gov.MarketTaxPermille) / 1000
				w.collectTaxWithSkim(m.District, tax)
				m.MoneyPool += cost - tax
			}

		}
	}
	return events
}

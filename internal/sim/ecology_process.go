package sim

import (
	"sort"
	"strings"
)

// ResourceRecruitment is a born but not yet harvestable biological cohort.
// Its quantity cannot be harvested before the configured maturation tick.
type ResourceRecruitment struct {
	PoolID      string `json:"pool_id"`
	Quantity    int64  `json:"quantity"`
	MaturesTick int64  `json:"matures_tick"`
}

func (w *World) advanceResourceRecruitment(district string, ds *DistrictState) {
	kept := ds.Recruitment[:0]
	for _, r := range ds.Recruitment {
		if w.Tick < r.MaturesTick {
			kept = append(kept, r)
			continue
		}
		delta := min(r.Quantity, maxInt64(0, ds.MaxResources[r.PoolID]-ds.Resources[r.PoolID]))
		ds.Resources[r.PoolID] += delta
		w.recordEcologicalChange(district, r.PoolID, delta)
	}
	if len(kept) == 0 {
		kept = nil
	}
	ds.Recruitment = kept
}

func (w *World) stepHydrologyTick(cal CalendarState, districts []string) {
	w.routeEcologicalWater(districts)
	w.regeneratePotableWater(cal, districts)
}

func (w *World) regeneratePotableWater(cal CalendarState, districts []string) {
	denom := int64(1000 * TicksPerDay)
	for _, district := range districts {
		ds, ok := w.DistrictStates[district]
		if !ok {
			continue
		}
		for _, id := range sortedResourceKeys(ds.MaxResources) {
			spec := w.Catalog.Resources[id]
			if !spec.PotableWater {
				continue
			}
			rate := spec.RegenPermillePerDay
			if mult, ok := spec.SeasonalRegenPct[cal.SeasonID]; ok {
				rate = rate * mult / 100
			}
			if mult, ok := w.Catalog.Weathers[ds.Weather].ResourceRegenPct[id]; ok {
				rate = rate * mult / 100
			}
			capacity := ds.MaxResources[id]
			if rate <= 0 || capacity <= 0 {
				continue
			}
			num := capacity*int64(rate) + ds.WaterRegenRemainder[id]
			delta := num / denom
			rem := num % denom
			if rem > 0 {
				if ds.WaterRegenRemainder == nil {
					ds.WaterRegenRemainder = map[string]int64{}
				}
				ds.WaterRegenRemainder[id] = rem
			} else if ds.WaterRegenRemainder != nil {
				delete(ds.WaterRegenRemainder, id)
				if len(ds.WaterRegenRemainder) == 0 {
					ds.WaterRegenRemainder = nil
				}
			}
			if delta <= 0 {
				continue
			}
			stored := min(delta, maxInt64(0, capacity-ds.Resources[id]))
			if delta > stored {
				if ds.WaterOverflow == nil {
					ds.WaterOverflow = map[string]int64{}
				}
				ds.WaterOverflow[id] += delta - stored
			}
			ds.Resources[id] += stored
			w.recordEcologicalChange(district, id, stored)
		}
		w.DistrictStates[district] = ds
	}
}

func (w *World) regenerateEcologicalResources(cal CalendarState, districts []string) {
	for _, district := range districts {
		ds := w.DistrictStates[district]
		keys := make([]string, 0, len(ds.MaxResources))
		for id := range ds.MaxResources {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			spec := w.Catalog.Resources[id]
			if spec.PotableWater {
				continue
			}
			rate := spec.RegenPermillePerDay
			if mult, ok := spec.SeasonalRegenPct[cal.SeasonID]; ok {
				rate = rate * mult / 100
			}
			if mult, ok := w.Catalog.Weathers[ds.Weather].ResourceRegenPct[id]; ok {
				rate = rate * mult / 100
			}
			if rate <= 0 {
				continue
			}
			capacity := ds.MaxResources[id]
			basis := capacity
			if spec.Biological {
				basis = ds.Resources[spec.ReproductionSource]
			}
			delta := w.ecologyExpectedCount64(basis, int64(rate), 1000, "recruit:"+district+":"+id)
			if spec.Biological {
				pending := int64(0)
				for _, r := range ds.Recruitment {
					if r.PoolID == id {
						pending += r.Quantity
					}
				}
				delta = min(delta, maxInt64(0, capacity-ds.Resources[id]-pending))
				if delta > 0 {
					ds.Recruitment = append(ds.Recruitment, ResourceRecruitment{PoolID: id, Quantity: delta, MaturesTick: w.Tick + spec.MaturityTicks})
				}
				continue
			}
			stored := min(delta, maxInt64(0, capacity-ds.Resources[id]))
			ds.Resources[id] += stored
			w.recordEcologicalChange(district, id, stored)
		}
		w.DistrictStates[district] = ds
	}
}

// routeEcologicalWater advances hydrological flow linearly per tick (1/TicksPerDay
// of daily conductance) between connected regions while tracking exact integer remainders.
// Multiple outlets share available source water without duplicating or teleporting it.
func (w *World) routeEcologicalWater(districts []string) {
	type transfer struct {
		from, to, pool string
		amount         int64
	}
	var transfers []transfer
	sortedDistricts := append([]string(nil), districts...)
	sort.Strings(sortedDistricts)
	for _, from := range sortedDistricts {
		region := w.Map.GeoRegionIDForLocation(from, "")
		if region == "" {
			continue
		}
		ds := w.DistrictStates[from]
		for _, pool := range sortedResourceKeys(ds.MaxResources) {
			if !w.Catalog.Resources[pool].PotableWater {
				continue
			}
			outletWeights := map[string]int64{}
			total := int64(0)
			for _, edge := range w.Map.GeoFlows {
				if edge.FromRegion != region || (edge.Kind != "river" && edge.Kind != "aquifer") || edge.ConductancePermille <= 0 {
					continue
				}
				var targets []string
				for _, to := range sortedDistricts {
					if to != from && w.Map.GeoRegionIDForLocation(to, "") == edge.ToRegion && w.DistrictStates[to].MaxResources[pool] > 0 {
						targets = append(targets, to)
					}
				}
				if len(targets) == 0 {
					continue
				}
				// Split the edge among modeled receiving districts without multiplying conductance.
				for i, to := range targets {
					weight := int64(edge.ConductancePermille / len(targets))
					if i < edge.ConductancePermille%len(targets) {
						weight++
					}
					if weight > 0 {
						outletWeights[to] += weight
						total += weight
					}
				}
			}
			if len(outletWeights) == 0 {
				continue
			}
			prefix := pool + "->"
			curStock := ds.Resources[pool]
			if curStock <= 0 {
				clearRemainderPrefix(&ds.WaterFlowRemainder, prefix)
				continue
			}
			denom := maxInt64(1000, total) * TicksPerDay
			type candidate struct {
				to   string
				key  string
				num  int64
				want int64
				rem  int64
			}
			outletKeys := sortedResourceKeys(outletWeights)
			cands := make([]candidate, 0, len(outletKeys))
			var wantSum int64
			for _, to := range outletKeys {
				key := prefix + to
				num := curStock*outletWeights[to] + ds.WaterFlowRemainder[key]
				want := num / denom
				rem := num % denom
				cands = append(cands, candidate{to: to, key: key, num: num, want: want, rem: rem})
				wantSum += want
			}
			if wantSum > curStock {
				order := make([]int, len(cands))
				for i := range order {
					order[i] = i
				}
				sort.Slice(order, func(i, j int) bool {
					if cands[order[i]].num != cands[order[j]].num {
						return cands[order[i]].num > cands[order[j]].num
					}
					return cands[order[i]].to < cands[order[j]].to
				})
				avail := curStock
				for _, idx := range order {
					grant := min(avail, cands[idx].want)
					cands[idx].want = grant
					avail -= grant
				}
				wantSum = curStock - avail
			}
			if curStock-wantSum == 0 {
				clearRemainderPrefix(&ds.WaterFlowRemainder, prefix)
			} else {
				for _, c := range cands {
					if c.rem > 0 {
						if ds.WaterFlowRemainder == nil {
							ds.WaterFlowRemainder = map[string]int64{}
						}
						ds.WaterFlowRemainder[c.key] = c.rem
					} else if ds.WaterFlowRemainder != nil {
						delete(ds.WaterFlowRemainder, c.key)
					}
				}
				if len(ds.WaterFlowRemainder) == 0 {
					ds.WaterFlowRemainder = nil
				}
			}
			for _, c := range cands {
				if c.want > 0 {
					transfers = append(transfers, transfer{from: from, to: c.to, pool: pool, amount: c.want})
				}
			}
		}
		w.DistrictStates[from] = ds
	}
	// Withdraw all source amounts before accepting arrivals. This makes cycles
	// and map order harmless and prevents freshly arrived water moving twice.
	for _, tr := range transfers {
		ds := w.DistrictStates[tr.from]
		ds.Resources[tr.pool] -= tr.amount
		w.recordEcologicalChange(tr.from, tr.pool, -tr.amount)
		w.DistrictStates[tr.from] = ds
	}
	for _, tr := range transfers {
		ds := w.DistrictStates[tr.to]
		stored := min(tr.amount, maxInt64(0, ds.MaxResources[tr.pool]-ds.Resources[tr.pool]))
		ds.Resources[tr.pool] += stored
		w.recordEcologicalChange(tr.to, tr.pool, stored)
		if tr.amount > stored {
			if ds.WaterOverflow == nil {
				ds.WaterOverflow = map[string]int64{}
			}
			ds.WaterOverflow[tr.pool] += tr.amount - stored
		}
		w.DistrictStates[tr.to] = ds
	}
}

func clearRemainderPrefix(m *map[string]int64, prefix string) {
	if *m == nil {
		return
	}
	for k := range *m {
		if strings.HasPrefix(k, prefix) {
			delete(*m, k)
		}
	}
	if len(*m) == 0 {
		*m = nil
	}
}

func sortedResourceKeys(resources map[string]int64) []string {
	keys := make([]string, 0, len(resources))
	for id := range resources {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	return keys
}

func (w *World) hasHydrologicalOverflow(district, kind string) bool {
	region := w.Map.GeoRegionIDForLocation(district, "")
	if region == "" {
		return false
	}
	connected := false
	for _, edge := range w.Map.GeoFlows {
		if edge.Kind == kind && edge.ConductancePermille > 0 && (edge.FromRegion == region || edge.ToRegion == region) {
			connected = true
			break
		}
	}
	if !connected {
		return false
	}
	for pool, amount := range w.DistrictStates[district].WaterOverflow {
		if amount > 0 && w.Catalog.Resources[pool].PotableWater {
			return true
		}
	}
	return false
}

func (w *World) ecologyExpectedCount64(population, numerator, denominator int64, salt string) int64 {
	if population == 0 || numerator == 0 {
		return 0
	}
	n := population * numerator
	count := n / denominator
	if rem := n % denominator; rem > 0 && int64(worldHash(w.ID, w.Tick, salt)%uint64(denominator)) < rem {
		count++
	}
	return count
}

func disasterExposureRates(dis DisasterSpec, scale int, use, need int64) (wounded, dead, denominator int64) {
	// Keep the existing 3 injured / 0.5 dead per base affected resident,
	// and the existing full-coverage mitigation of 55%. Remove forced minima.
	mitigation := int64(100000)
	if need > 0 {
		mitigation -= 55000 * use / need
	}
	base := int64(dis.CasualtyPermille) * int64(scale) * mitigation
	return 6 * base, base, 2 * 1000 * 100 * 100000
}

func encounterExposureRates(enc EncounterSpec) (wounded, dead, denominator int64) {
	base := int64(enc.HazardPermille)
	return base * int64(1000-enc.LethalityPermille), base * int64(enc.LethalityPermille), 1000 * 1000
}

func (w *World) applyEcologyCohortExposure(c *PopulationCohort, population int, wn, dn, denominator int64, salt, cause string) (int, int) {
	deaths := int(w.ecologyExpectedCount64(int64(population), min(dn, denominator), denominator, salt+":dead:"+c.ID))
	injured := int(w.ecologyExpectedCount64(int64(population), min(wn, denominator-min(dn, denominator)), denominator, salt+":wounded:"+c.ID))
	injured = min(injured, population-deaths)
	cw, cd, _ := w.applyCohortCasualtiesWithCause(c, injured, deaths, 0, cause)
	return cw, cd
}

func (w *World) applyEcologyActorExposure(a *Actor, wn, dn, denominator int64, damage int, salt, cause string) (int, int, []Event) {
	if a.Dead {
		return 0, 0, nil
	}
	dn = min(dn, denominator)
	wn = min(wn, denominator-dn)
	roll := int64(worldHash(w.ID, w.Tick, salt+":"+a.ID) % uint64(denominator))
	if roll < dn {
		a.Health = 0
		return 0, 1, w.markDeadIfZero(a, nil, Action{Kind: "wait"}, cause)
	}
	if roll < dn+wn {
		// A wounded outcome is nonfatal; death has its own declared probability.
		previousHealth := a.Health
		a.Health = maxInt(1, a.Health-maxInt(1, damage))
		events := []Event{w.record(a, Action{Kind: "wait"}, "actor_environment_injured", map[string]any{
			"text":  "environmental exposure injured " + a.Name,
			"cause": cause, "previous_health": previousHealth, "health": a.Health,
		})}
		return 1, 0, append(events, w.interruptTaskOnDamage(a, cause)...)
	}
	return 0, 0, nil
}

func ecologyActorImpactCounts(events []Event) (wounded, dead int) {
	for _, event := range events {
		switch event.Kind {
		case "actor_environment_injured":
			wounded++
		case "actor_died":
			dead++
		}
	}
	return
}

func (w *World) applyDisasterActorExposure(a *Actor, dis DisasterSpec, scale int) (int, int, int64, []Event) {
	use := int64(0)
	if dis.ReliefItem != "" && a.Items[dis.ReliefItem] > 0 {
		use = 1
		w.adjustActorItem(a, dis.ReliefItem, -1, true)
	}
	wn, dn, den := disasterExposureRates(dis, scale, use, 1)
	// DisasterSpec encodes injury counts, not injury severity. One health point
	// marks the injury without inventing a new disaster damage coefficient.
	cw, cd, events := w.applyEcologyActorExposure(a, wn, dn, den, 1, "disaster:"+dis.ID, "disaster")
	return cw, cd, use, events
}

// ecologyWorkingPopulation reads actual local work reservations, not profession.
// Biological maintenance after preparation is owned by growth's care mechanism.
func (w *World) ecologyWorkingPopulation(c *PopulationCohort, recipes, features []string) int {
	workers := 0
	for _, g := range w.Growth {
		care := w.growthCaretaker(g)
		if !care.Cohort || care.OwnerID != c.ID || g.District != c.District || g.Ready || g.Stalled {
			continue
		}
		if len(recipes) > 0 && !ActorHasTag(recipes, g.RecipeID) {
			continue
		}
		r, ok := w.Catalog.RecipeByID(g.RecipeID)
		if !ok {
			continue
		}
		if len(features) > 0 && (r.NodeFeature == "" || !ActorHasTag(features, r.NodeFeature) || !w.Map.VenueOrDistrictHasFeature(g.VenueID, g.District, r.NodeFeature)) {
			continue
		}
		if r.Growth != nil && w.Tick >= g.PreparationUntil {
			continue
		}
		workers += g.Workers
	}
	return min(workers, w.anonymousCohort(*c).EffectivePopulation())
}

// A successful creature kill consumes a finite source before any drop roll.
// A zero drop probability never produces an item, including fractional expectations.
func (w *World) harvestEncounterSource(c *PopulationCohort, district string, enc EncounterSpec, attempts int64, salt string) int64 {
	quantity := w.drawEncounterLoot(district, enc, attempts, salt)
	if quantity > 0 {
		w.adjustCohortItem(c, enc.RareDropItem, quantity, true)
	}
	return quantity
}

func (w *World) drawEncounterLoot(district string, enc EncounterSpec, attempts int64, salt string) int64 {
	if enc.SourceResource == "" || enc.ResourcePerKill <= 0 || attempts <= 0 {
		return 0
	}
	ds := w.DistrictStates[district]
	kills := min(attempts, ds.AvailableResource(enc.SourceResource)/enc.ResourcePerKill, int64(maxInt(0, ds.ThreatActivity[enc.ID])))
	if kills <= 0 {
		return 0
	}
	w.drawDistrictResource(district, enc.SourceResource, kills*enc.ResourcePerKill)
	ds = w.DistrictStates[district]
	ds.ThreatActivity[enc.ID] -= int(kills)
	w.DistrictStates[district] = ds
	if enc.RareDropItem == "" || enc.RareDropPermille == 0 {
		return 0
	}
	return w.ecologyExpectedCount64(kills, int64(enc.RareDropPermille), 1000, salt+":"+enc.ID)
}

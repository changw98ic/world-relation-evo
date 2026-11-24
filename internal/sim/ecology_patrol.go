package sim

import "fmt"

// EcologyPatrol reserves real anonymous workers while they travel, work, and
// return. Work time and energy come from the catalog's extraction recipe for
// the encounter's source pool; road time/energy use ordinary movement costs.
type EcologyPatrol struct {
	CohortID      string `json:"cohort_id"`
	EncounterID   string `json:"encounter_id"`
	Home          string `json:"home"`
	Target        string `json:"target"`
	Current       string `json:"current"`
	Next          string `json:"next,omitempty"`
	Phase         string `json:"phase"`
	UntilTick     int64  `json:"until_tick"`
	Workers       int    `json:"workers"`
	RecipeID      string `json:"recipe_id"`
	CargoItem     string `json:"cargo_item,omitempty"`
	CargoQuantity int64  `json:"cargo_quantity,omitempty"`
	BountyClaim   int64  `json:"bounty_claim,omitempty"`
	Blocked       bool   `json:"blocked,omitempty"`
}

// applyEcologyPatrolExposure settles hazards at the expedition's actual site.
// Workers on a road are excluded from stationary district exposure, like Actors
// executing move. A patrol is never counted again at its home cohort's site.
func (w *World) applyEcologyPatrolExposure(district string, apply func(*PopulationCohort, int) (int, int)) (wounded, dead int) {
	for _, stateID := range sortedDistrictStateKeys(w.DistrictStates) {
		ds := w.DistrictStates[stateID]
		for i := range ds.Patrols {
			p := &ds.Patrols[i]
			if p.Current != district || p.Next != "" || p.Workers == 0 {
				continue
			}
			c := w.findCohort(p.CohortID)
			if c == nil || c.Population == 0 {
				continue
			}
			cw, cd := apply(c, p.Workers)
			p.Workers -= cd
			wounded += cw
			dead += cd
		}
		w.DistrictStates[stateID] = ds
	}
	return
}

func (w *World) ecologyReservedWorkers(cohortID string) int {
	workers := 0
	for _, ds := range w.DistrictStates {
		for _, p := range ds.Patrols {
			if p.CohortID == cohortID {
				workers += p.Workers
			}
		}
	}
	return workers
}

func (w *World) ecologyPatrolRecipe(enc EncounterSpec) (Recipe, bool) {
	for _, r := range w.Catalog.Recipes {
		if r.ResourcePool == enc.SourceResource && r.Ticks > 0 && r.Growth == nil {
			return r, true
		}
	}
	return Recipe{}, false
}

func (w *World) stepCohortThreatSuppression() []Event {
	var events []Event
	for _, district := range sortedDistrictStateKeys(w.DistrictStates) {
		for _, enc := range w.Catalog.Encounters {
			ds := w.DistrictStates[district]
			if ds.ThreatActivity[enc.ID] < enc.BaseActivity+10 {
				continue
			}
			if w.settlementRandom(w.Tick, "sup_roll:"+district+":"+enc.ID)%1000 >= 320 && ds.BountyPool == 0 {
				continue
			}
			recipe, ok := w.ecologyPatrolRecipe(enc)
			if !ok {
				continue
			}
			for ci := range w.Cohorts {
				c := &w.Cohorts[ci]
				available := int(w.productionFreeWorkers(GrowthBatch{OwnerID: c.ID, Cohort: true, District: c.District}))
				if available < 4 || w.Tick < c.RestUntilTick {
					continue
				}
				prof, ok := w.Catalog.ProfessionByID(c.Profession)
				if !ok {
					continue
				}
				matched := false
				for _, tag := range prof.Grants {
					if ActorHasTag(enc.SuppressorTags, tag) {
						matched = true
					}
				}
				if !matched {
					continue
				}
				if c.District != district {
					if _, _, ok := w.Map.NextHop(c.District, district); !ok {
						continue
					}
					if _, _, ok := w.Map.NextHop(district, c.District); !ok {
						continue
					}
				}
				// Retain the preset's existing squad size policy, bounded by actual free labor.
				squad := min(available, min(80, maxInt(3, available/20)))
				p := EcologyPatrol{CohortID: c.ID, EncounterID: enc.ID, Home: c.District, Target: district, Current: c.District, Workers: squad, RecipeID: recipe.ID, Phase: "outgoing"}
				if !w.startEcologyPatrolLeg(c, &p) {
					continue
				}
				ds = w.DistrictStates[district]
				ds.Patrols = append(ds.Patrols, p)
				w.DistrictStates[district] = ds
				events = append(events, w.ecologyPatrolEvent(p, "ecology_patrol_started", "Patrol departed; workers are reserved until they return"))
				break
			}
		}
	}
	return events
}

func (w *World) startEcologyPatrolLeg(c *PopulationCohort, p *EcologyPatrol) bool {
	destination := p.Target
	if p.Phase == "return" {
		destination = p.Home
	}
	if p.Current == destination {
		if p.Phase == "return" {
			return true
		}
		r, _ := w.Catalog.RecipeByID(p.RecipeID)
		if !w.payEcologyPatrolEnergy(c, p.Workers, r.EnergyCost) {
			return false
		}
		p.Phase, p.UntilTick = "working", w.Tick+int64(r.Ticks)
		p.Next = ""
		return true
	}
	hop, _, ok := w.Map.NextHop(p.Current, destination)
	if !ok {
		return false
	}
	ticks, energy := w.effectiveRoadCost(hop)
	if !w.payEcologyPatrolEnergy(c, p.Workers, energy) {
		return false
	}
	p.Next, p.UntilTick = hop.To, w.Tick+int64(ticks)
	return true
}

func (w *World) payEcologyPatrolEnergy(c *PopulationCohort, workers, cost int) bool {
	population := w.anonymousCohort(*c).Population
	energy := w.anonymousCohortEnergy(c, population)
	debit := int64(cost) * int64(workers)
	if energy < debit {
		return false
	}
	w.setAnonymousCohortEnergy(c, population, energy-debit)
	return true
}

func (w *World) advanceEcologyPatrols() []Event {
	var events []Event
	for _, district := range sortedDistrictStateKeys(w.DistrictStates) {
		pending := append([]EcologyPatrol(nil), w.DistrictStates[district].Patrols...)
		var kept []EcologyPatrol
		for _, p := range pending {
			c := w.findCohort(p.CohortID)
			if c == nil || c.Population <= 0 || p.Workers == 0 {
				events = append(events, w.ecologyPatrolEvent(p, "ecology_patrol_lost", "Patrol has no surviving workers or receiving cohort; undelivered cargo is lost"))
				continue
			}
			if w.Tick < p.UntilTick {
				kept = append(kept, p)
				continue
			}
			if p.Phase == "working" {
				var enc EncounterSpec
				for _, candidate := range w.Catalog.Encounters {
					if candidate.ID == p.EncounterID {
						enc = candidate
						break
					}
				}
				ds := w.DistrictStates[p.Target]
				activity := ds.ThreatActivity[enc.ID]
				suppressed := min(15, 9+p.Workers/10, maxInt(0, activity-8))
				p.CargoItem = enc.RareDropItem
				p.CargoQuantity = w.drawEncounterLoot(p.Target, enc, int64(suppressed), "patrol:"+c.ID)
				ds = w.DistrictStates[p.Target]
				// Killed creatures already reduced activity; any remaining suppression
				// represents driving creatures away, without creating extra loot.
				ds.ThreatActivity[enc.ID] = maxInt(0, activity-suppressed)
				w.DistrictStates[p.Target] = ds
				if suppressed > 0 {
					p.BountyClaim = int64(enc.BountyCoins * maxInt(1, p.Workers/6))
				}
				p.Phase = "return"
				wn, dn, den := encounterExposureRates(enc)
				cw, cd := w.applyEcologyCohortExposure(c, p.Workers, wn, dn, den, "patrol:"+enc.ID, "beast")
				p.Workers -= cd
				ev := w.ecologyPatrolEvent(p, "cohort_threat_hunted", fmt.Sprintf("Patrol completed work in %s; %d wounded, %d died; cargo returns along the road", p.Target, cw, cd))
				ev.Payload["wounded"], ev.Payload["deaths"], ev.Payload["cause"] = cw, cd, "beast"
				ev.Payload["suppressed"], ev.Payload["cargo_item"], ev.Payload["cargo_qty"] = suppressed, p.CargoItem, p.CargoQuantity
				events = append(events, ev)
			} else if p.Next != "" {
				p.Current, p.Next = p.Next, ""
			}
			if p.Phase == "return" && p.Current == p.Home {
				if p.CargoQuantity > 0 {
					w.adjustCohortItem(c, p.CargoItem, p.CargoQuantity, true)
				}
				paid := w.debitTreasury(p.BountyClaim, "bounty")
				w.addCohortCurrency(c, "coins", paid)
				ds := w.DistrictStates[p.Target]
				ds.BountyPool -= min(ds.BountyPool, int(paid))
				w.DistrictStates[p.Target] = ds
				ev := w.ecologyPatrolEvent(p, "ecology_patrol_returned", "Patrol returned with its cargo and received the available bounty")
				ev.Payload["drop_item"], ev.Payload["drop_qty"], ev.Payload["bounty_coins"] = p.CargoItem, p.CargoQuantity, paid
				events = append(events, ev)
				continue
			}
			if !w.startEcologyPatrolLeg(c, &p) {
				if !p.Blocked {
					events = append(events, w.ecologyPatrolEvent(p, "ecology_patrol_blocked", "Patrol is waiting for a usable road or sufficient travel energy"))
				}
				p.Blocked = true
			} else {
				p.Blocked = false
			}
			kept = append(kept, p)
		}
		ds := w.DistrictStates[district]
		ds.Patrols = kept
		w.DistrictStates[district] = ds
	}
	return events
}

func (w *World) ecologyPatrolEvent(p EcologyPatrol, kind, text string) Event {
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: p.CohortID, Kind: kind, Action: Action{Kind: "hunt_threat", Location: p.Current, RecipeID: p.RecipeID}, Payload: map[string]any{"text": text, "cohort_id": p.CohortID, "district": p.Target, "threat_id": p.EncounterID, "workers": p.Workers, "phase": p.Phase, "arrival_tick": p.UntilTick}}
	w.NextID++
	return e
}

func sortedDistrictStateKeys(states map[string]DistrictState) []string {
	keys := make(map[string]int64, len(states))
	for id := range states {
		keys[id] = 0
	}
	return sortedResourceKeys(keys)
}

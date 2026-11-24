package sim

import (
	"math"
	"math/bits"
)

// actionWorkRate describes the exertion for a reference number of worked
// intervals. Actual energy is settled against productive intervals elapsed.
type actionWorkRate struct {
	energy int
	ticks  int
}

func (rate actionWorkRate) energyAfter(ticks int) int {
	return ceilScaled(rate.energy, ticks, max(1, rate.ticks))
}

func ceilScaled(value, units, divisor int) int {
	if value <= 0 || units <= 0 {
		return 0
	}
	high, low := bits.Mul64(uint64(value), uint64(units))
	if high >= uint64(divisor) {
		return math.MaxInt
	}
	result, remainder := bits.Div64(high, low, uint64(divisor))
	if result >= math.MaxInt {
		return math.MaxInt
	}
	if remainder != 0 {
		result++
	}
	return int(result)
}

// Each settled lot takes one interval of transaction work plus handling of
// the actual cargo. One catalog mass unit represents one point of handling.
func (c Catalog) tradeEnergy(item string, quantity int) int {
	mass := c.Items[item].MassUnits
	if quantity <= 0 || mass <= 0 {
		return lightActionEnergy
	}
	if mass > int64((math.MaxInt-lightActionEnergy)/quantity) {
		return math.MaxInt
	}
	return lightActionEnergy + int(mass)*quantity
}

func (r Recipe) WorkEstimate(facilityBonusPct int) (ticks, energy int) {
	rate := actionWorkRate{energy: r.EnergyCost, ticks: max(1, r.Ticks)}
	ticks = ceilScaled(rate.ticks, 100, 100+facilityBonusPct)
	return ticks, rate.energyAfter(ticks)
}

func (c Catalog) actionWork(x Action, a Actor, targetNearby bool) (rate actionWorkRate, ticks int) {
	meta, builtin := builtinActionMetadataIndex[x.Kind]
	rate, ticks = actionWorkRate{energy: max(0, meta.referenceEnergy), ticks: 1}, 1
	if !builtin {
		spec := c.ActionSpecs[x.Kind]
		ticks = max(1, spec.Ticks)
		return actionWorkRate{energy: spec.EnergyCost, ticks: ticks}, ticks
	}
	switch x.Kind {
	case "market_buy", "market_sell", "accept", "fulfill_agreement":
		rate.energy = c.tradeEnergy(x.Item, x.Quantity)
	case "teach_knowledge":
		if x.TechID != "" && !targetNearby {
			rate.energy += c.CommChannels[x.Channel].EnergyCost
		} else if x.TechID == "" && x.SkillID != "" {
			// The minimum proficient teacher spends the reference effort.
			// Higher levels deliver more material in the same interval.
			rate.energy += max(0, a.Skills[x.SkillID]-2)
		}
	case "repair_house", "hunt_threat":
		ticks = 3
	case "upgrade_house":
		ticks = TicksPerHour
	case "attempt_transition":
		ticks = max(1, c.Transitions[resolveTransitionID(x)].Ticks)
		rate.ticks = TicksPerHour
		return rate, ticks
	case "maintain_infra":
		rate.energy, ticks = c.Infrastructures[x.Item].BuildEnergy, 3
	case "perform_ritual":
		for _, ritual := range c.Rituals {
			if ritual.ID == x.RecipeID {
				rate.energy, ticks = ritual.EnergyCost, max(1, ritual.Ticks)
				break
			}
		}
	case "research_tech":
		tech := c.Techs[x.TechID]
		rate.energy, ticks = tech.ResearchEnergyCost, max(1, tech.ResearchTicks)
	case "adopt_facility":
		tech := c.Techs[x.TechID]
		rate.energy, ticks = tech.AdoptEnergyCost, max(1, tech.AdoptTicks)
	case "transmit_message":
		rate.energy = c.CommChannels[x.Channel].EnergyCost
	}
	rate.ticks = ticks
	return rate, ticks
}

// ActionWorkEstimate uses only the work and cargo the actor can observe.
// The route planner supplies travel duration and effort from its route facts.
func (p Perception) ActionWorkEstimate(x Action) (ticks, energy int) {
	switch x.Kind {
	case "accept":
		for _, offer := range p.Offers {
			if offer.ID == x.OfferID {
				x.Item, x.Quantity = offer.Item, offer.Quantity
				break
			}
		}
	case "fulfill_agreement":
		x = withAgreementCargo(x, p.Continuity.Agreements)
	case "craft":
		recipe, _ := p.Catalog.RecipeByID(x.RecipeID)
		return recipe.WorkEstimate(p.CraftingSpeedBonusPct)
	case "transmit_message":
		x.Channel = p.Catalog.transmissionChannelID(p.EffectiveTags(), x.Channel)
	}
	rate, ticks := p.Catalog.actionWork(x, p.Self, x.Kind != "teach_knowledge" || p.localNeighbor(x.TargetID) != nil)
	return ticks, rate.energyAfter(ticks)
}

func (p Perception) ActionEnergyCost(x Action) int {
	_, energy := p.ActionWorkEstimate(x)
	return energy
}

func withAgreementCargo(x Action, agreements []Agreement) Action {
	for _, agreement := range agreements {
		if agreement.ID == x.AgreementID {
			x.Item, x.Quantity = agreement.Item, agreement.Quantity
			break
		}
	}
	return x
}

func (t *Task) beginWork(rate actionWorkRate) {
	if rate.energy > 0 {
		t.WorkEnergy, t.WorkPeriod, t.WorkTicks = rate.energy, rate.ticks, 1
	}
}

func (tx *WorldTx) startTaskWork(a *Actor, t *Task, energy int) {
	rate := actionWorkRate{energy: energy, ticks: max(1, t.TotalTicks)}
	t.beginWork(rate)
	tx.setEnergy(a, a.Energy-rate.energyAfter(1), t.Action.Kind)
}

func (t *Task) nextWorkEnergy() int {
	rate := actionWorkRate{energy: t.WorkEnergy, ticks: t.WorkPeriod}
	return rate.energyAfter(t.WorkTicks+1) - rate.energyAfter(t.WorkTicks)
}

func (t Task) recordWork(events []Event) {
	if t.WorkTicks == 0 || len(events) == 0 {
		return
	}
	if events[0].Payload == nil {
		events[0].Payload = make(map[string]any)
	}
	events[0].Payload["work_ticks"] = t.WorkTicks
	events[0].Payload["energy_spent_total"] = (actionWorkRate{energy: t.WorkEnergy, ticks: t.WorkPeriod}).energyAfter(t.WorkTicks)
}

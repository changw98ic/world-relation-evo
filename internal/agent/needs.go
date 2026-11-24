package agent

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func buildNeeds(p sim.Perception, rules sim.PlanningRules) ([]needValue, []sim.PreferenceState, error) {
	rules.Needs = needsWithGoals(p, rules.Needs)
	prior := make(map[sim.NeedKey]sim.PreferenceState, len(p.Self.Preferences))
	for _, state := range p.Self.Preferences {
		prior[state.Need] = state
	}
	definitions := make(map[sim.NeedKey]bool, len(rules.Needs))
	for _, rule := range rules.Needs {
		definitions[rule.NeedKey] = true
	}
	var rows []needValue
	var next []sim.PreferenceState
	for _, definition := range rules.Needs {
		for _, row := range expandNeed(p, definition) {
			if row.NeedKey != definition.NeedKey && definitions[row.NeedKey] {
				continue
			}
			if row.Minimize {
				row.Target, row.level, row.drift = -row.Target, -row.level, -row.drift
				row.lower, row.upper = -row.upper, -row.lower
			}
			key := row.NeedKey
			state, exists := prior[key]
			if !exists {
				state = sim.PreferenceState{Need: key, BaseWeight: row.BaseWeight, UpdatedTick: p.Tick}
				if w, ok := p.Self.NeedWeights[key.Measure]; ok {
					state.BaseWeight = w
				}
				if key.Subject != "" {
					if w, ok := p.Self.NeedWeights[key.Measure+":"+key.Subject]; ok {
						state.BaseWeight = w
					}
				}
			}
			persistent := advancePreference(&state, row.NeedRule, rules.Experiences, p)
			bias := 0.0
			for _, influence := range state.Influences {
				bias += influence.Strength * math.Exp(-float64(p.Tick-influence.Tick)/influence.RecoveryTicks)
			}
			row.base, row.bias, row.fluctuation = state.BaseWeight, bias, state.Fluctuation
			if state.BaseWeight != 0 {
				row.weight = state.BaseWeight * math.Exp(bias+state.Fluctuation)
			}
			if math.IsNaN(row.weight) || math.IsInf(row.weight, 0) || row.weight < 0 {
				return nil, nil, fmt.Errorf("invalid effective weight for actor %s need %+v", p.Self.ID, key)
			}
			rows = append(rows, row)
			if persistent {
				next = append(next, state)
			}
			delete(prior, key)
		}
	}
	// A temporarily absent relationship or obligation can regain relevance.
	for _, state := range p.Self.Preferences {
		if _, absent := prior[state.Need]; absent {
			next = append(next, state)
			delete(prior, state.Need)
		}
	}
	return rows, next, nil
}

func needsWithGoals(p sim.Perception, rules []sim.NeedRule) []sim.NeedRule {
	if len(p.Self.Ambitions) == 0 {
		return rules
	}
	rows := slices.Clone(rules)
	for _, goal := range p.Self.Ambitions {
		if goal.Completed {
			continue
		}
		key, target := goal.Need(p.Catalog)
		match := slices.IndexFunc(rows, func(r sim.NeedRule) bool {
			return r.NeedKey == key || key.Measure == "money" && r.NeedKey == (sim.NeedKey{Measure: "money", Subject: "coins"})
		})
		if match >= 0 {
			rows[match].Target = max(rows[match].Target, float64(target))
			continue
		}
		templates := rules
		i := slices.IndexFunc(templates, func(r sim.NeedRule) bool { return r.Measure == key.Measure && r.Subject == "" })
		if i < 0 {
			templates = sim.DefaultPlanningRules().Needs
			i = slices.IndexFunc(templates, func(r sim.NeedRule) bool { return r.Measure == key.Measure })
		}
		row := templates[i]
		row.NeedKey, row.Target = key, float64(target)
		if key.Measure == "profession_xp" {
			row.Scale = max(row.Scale, row.Target)
		}
		rows = append(rows, row)
	}
	return rows
}

func advancePreference(state *sim.PreferenceState, rule sim.NeedRule, experiences []sim.ExperienceRule, p sim.Perception) bool {
	applies := func(experience sim.ExperienceRule) bool {
		return experience.Need == state.Need || experience.Need == (sim.NeedKey{Measure: state.Need.Measure})
	}
	hasExperienceRule := slices.ContainsFunc(experiences, applies)
	for _, m := range p.Experiences {
		if m.EventID <= state.LastEventID {
			continue
		}
		for _, experience := range experiences {
			if !applies(experience) || m.Kind != experience.Event {
				continue
			}
			if experience.Bias != 0 {
				state.Influences = append(state.Influences, sim.PreferenceInfluence{
					EventID: m.EventID, Tick: m.Tick, Strength: experience.Bias, RecoveryTicks: experience.RecoveryTicks,
				})
			}
			if experience.NewBaseWeight != nil {
				state.BaseWeight += rule.Plasticity * (*experience.NewBaseWeight - state.BaseWeight)
			}
		}
		if hasExperienceRule {
			state.LastEventID = m.EventID
		}
	}
	if hasExperienceRule {
		state.LastEventID = max(state.LastEventID, p.ObservationVersion)
	}
	if dt := p.Tick - state.UpdatedTick; dt > 0 {
		if rule.Volatility != 0 || state.Fluctuation != 0 {
			rho := 0.0
			if rule.CorrelationTicks != 0 {
				rho = math.Exp(-float64(dt) / rule.CorrelationTicks)
			}
			state.Fluctuation *= rho
			if rule.Volatility != 0 {
				state.Fluctuation += rule.Volatility * math.Sqrt(1-rho*rho) * normalPreference(p.RunSeed, p.Self.ID, state.Need, p.Tick)
			}
		}
		state.UpdatedTick = p.Tick
	}
	return hasExperienceRule || rule.Volatility != 0 || state.Fluctuation != 0 || len(state.Influences) != 0
}

func expandNeed(p sim.Perception, rule sim.NeedRule) []needValue {
	a := p.Self
	row := needValue{NeedRule: rule}
	switch rule.Measure {
	case "nutrition":
		if p.Catalog.ActorIsHungerDisabled(a) {
			return nil
		}
		form := p.Catalog.Forms[a.FormID]
		row.level = float64(a.NutritionReserve) - float64(a.Hunger)
		row.drift = -float64(sim.DailyNutritionForForm(form)) / sim.TicksPerDay
	case "hydration":
		row.level = -float64(a.Thirst)
		if !canDrawFreeWater(p) {
			row.drift = -1.0 / sim.HydrationIntervalTicks
		}
	case "energy":
		row.level = float64(a.Energy)
		row.drift = -1.0 / sim.AwakeEnergyIntervalTicks
		row.lower, row.upper, row.bounded = 0, sim.MaxEnergy, true
		if row.level >= row.Target {
			row.reviewAfter = a.NextSleepWindowChange(p.Tick) - p.Tick
		}
	case "health":
		if rule.Subject == "" {
			row.level = float64(a.Health)
			break
		}
		for _, neighbor := range p.Nearby {
			if neighbor.ID == rule.Subject {
				row.level = float64(neighbor.Health)
				return []needValue{row}
			}
		}
		return nil
	case "hygiene":
		row.level = -float64(a.Waste)
	case "provisions":
		row.level = float64(p.Catalog.AvailableNutrition(a) - a.NutritionReserve)
	case "money":
		row.level = float64(a.Money)
		if rule.Subject != "" && rule.Subject != "coins" {
			row.level = float64(a.Currencies[rule.Subject])
			break
		}
		for _, wage := range p.PendingWages {
			row.reference = append(row.reference, effect{
				Need: rule.NeedKey, At: max(0, wage.DueTick-p.Tick), Change: float64(wage.Coins), shared: true,
			})
		}
		sortEffects(row.reference)
	case "housing":
		house := rule.Subject
		if house == "" {
			house = a.HomeID
		}
		for _, h := range p.AccessibleHouses {
			if h.ID == house {
				row.level = float64(h.Condition)
				return []needValue{row}
			}
		}
		if p.CurrentHouse != nil && p.CurrentHouse.ID == house {
			row.level = float64(p.CurrentHouse.Condition)
		} else {
			return nil
		}
	case "stock":
		if rule.Subject != "" {
			row.level = float64(a.Items[rule.Subject])
			return []needValue{row}
		}
		var rows []needValue
		for _, id := range sortedItemIDs(p.Catalog.Items) {
			spec := p.Catalog.Items[id]
			if a.Plan != nil && a.Plan.RemainingToSell() > 0 && a.Plan.Item == id {
				continue
			}
			if p.Catalog.ActorAcceptsFood(a, spec) {
				continue
			}
			target := p.Catalog.DesiredStock(a.IdentityTags(), id)
			if target <= 0 && !p.Catalog.TagsProduce(a.IdentityTags(), id) {
				continue
			}
			item := row
			item.Subject, item.level = id, float64(a.Items[id])
			if target > 0 {
				item.Target = float64(target)
			}
			rows = append(rows, item)
		}
		return rows
	case "progress":
		if a.Plan != nil && a.Plan.RemainingToSell() > 0 {
			row.level = productionProgress(p.Catalog, *a.Plan, a.Plan.Sold, a.Items[a.Plan.Item])
		} else {
			return nil
		}
	case "obligation":
		var rows []needValue
		for _, g := range p.Continuity.Agreements {
			if g.Status != "active" || g.SupplierID != a.ID || p.Tick > g.DueTick {
				continue
			}
			id := strconv.FormatInt(g.ID, 10)
			if rule.Subject == "" || rule.Subject == id {
				agreement := row
				agreement.Subject = id
				rows = append(rows, agreement)
			}
		}
		return rows
	case "relationship":
		if rule.Subject != "" {
			row.level = float64(a.Trust[rule.Subject])
			return []needValue{row}
		}
		var rows []needValue
		for _, neighbor := range p.Nearby {
			relation := row
			relation.Subject = neighbor.ID
			relation.level = float64(a.Trust[neighbor.ID])
			rows = append(rows, relation)
		}
		slices.SortFunc(rows, func(a, b needValue) int {
			if a.Subject < b.Subject {
				return -1
			}
			if a.Subject > b.Subject {
				return 1
			}
			return 0
		})
		return rows
	case "skill":
		if rule.Subject != "" {
			row.level = float64(a.SkillXP[rule.Subject])
			return []needValue{row}
		}
		var rows []needValue
		for _, id := range sortedIntMapKeys(a.Skills) {
			if spec, known := p.Catalog.Skills[id]; known {
				rows = appendNextRank(rows, row, id, a.Skills[id], a.SkillXP[id], spec.EffectiveMaxLevel(), p.Catalog.LevelThresholds)
			}
		}
		return rows
	case "profession_xp":
		if rule.Subject == "" {
			var rows []needValue
			for _, spec := range p.Catalog.Professions {
				if slices.ContainsFunc(a.Ambitions, func(goal sim.ActorAmbition) bool {
					return goal.Kind == "advance_profession" && goal.Completed && goal.TargetID == spec.ID
				}) {
					rows = appendNextRank(rows, row, spec.ID, a.Professions[spec.ID], a.ProfessionXP[spec.ID], spec.MaxLevel, p.Catalog.LevelThresholds)
				}
			}
			return rows
		}
		row.level = float64(a.ProfessionXP[rule.Subject])
	case "market_trade", "ambition":
		if rule.Subject == "" || rule.Target == 0 {
			return nil
		}
	case "state_slot":
		value, known := p.Catalog.StateSlotValue(a, rule.Subject)
		if !known {
			return nil
		}
		row.level = float64(value)
		row.drift = float64(p.StateSlotChangePerTick(rule.Subject))
		lower, upper := p.Catalog.StateSlots[rule.Subject].Bounds()
		row.lower, row.upper, row.bounded = float64(lower), float64(upper), true
	case "conversation":
		var rows []needValue
		for _, peer := range p.Nearby {
			if peer.Sleeping || peer.Jailed {
				continue
			}
			if question, open := p.OpenQuestions[peer.ID]; open && question.Source == peer.ID {
				answer := row
				answer.Subject = strconv.FormatInt(question.EventID, 10)
				rows = append(rows, answer)
			}
		}
		return rows
	case "power_tier":
		if rule.Target == 0 {
			return nil
		}
		row.level = float64(p.Catalog.ActorPowerTier(a))
	case "knowledge":
		if rule.Subject != "" {
			if slices.Contains(p.KnownTechs, rule.Subject) {
				row.level = 1
			}
			return []needValue{row}
		}
		var rows []needValue
		ids := make([]string, 0, len(p.Catalog.Techs))
		for id := range p.Catalog.Techs {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			tech := row
			tech.Subject = id
			if slices.Contains(p.KnownTechs, id) {
				tech.level = 1
			}
			rows = append(rows, tech)
		}
		return rows
	case "safety":
		if p.DistrictState != nil {
			if rule.Subject != "" {
				row.level = -float64(p.DistrictState.ThreatActivity[rule.Subject])
			} else {
				for _, threat := range p.Catalog.Encounters {
					row.level -= float64(p.DistrictState.ThreatActivity[threat.ID])
				}
			}
		}
	}
	return []needValue{row}
}

func appendNextRank(rows []needValue, row needValue, id string, rank, xp, cap int, thresholds []int) []needValue {
	if rank < 1 || rank >= min(cap, len(thresholds)) {
		return rows
	}
	row.Subject, row.level = id, float64(xp)
	row.Target = float64(thresholds[rank])
	row.Scale = max(row.Scale, float64(thresholds[rank]-thresholds[rank-1]))
	return append(rows, row)
}

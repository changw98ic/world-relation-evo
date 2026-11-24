package agent

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func addGoldenFingerOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	if len(p.ActiveGoldenFingers) == 0 {
		return
	}
	for _, gf := range p.ActiveGoldenFingers {
		if allowed["gf_exchange"] {
			for _, offer := range gf.AvailableShop {
				add(fmt.Sprintf("[%s] Redeem %s", gf.Name, offer.Name),
					sim.Action{Kind: "gf_exchange", Text: gf.ID, Item: offer.ID})
			}
		}
		if allowed["gf_simulate"] && gf.SimulatorReady {
			add(fmt.Sprintf("[%s] Run life/branch simulation (%s)", gf.Name, gf.SimulatorName),
				sim.Action{Kind: "gf_simulate", Text: gf.ID})
		}
	}
}

// planGoldenFingerAction autonomously executes high-value golden finger behaviors:
//   1. Emergency/progression system shop redemptions (healing when wounded, breakthrough items, stat/XP upgrades)
//   2. Life/branch simulator invocations when ready
//   3. Prophetic foresight movement (arriving early to snatch opportunities or fleeing predicted danger)
//   4. Active golden-finger quest pursuit (visiting districts, grinding skills, hunting targets)
func planGoldenFingerAction(p sim.Perception, consider func(sim.Action) bool) bool {
	if len(p.ActiveGoldenFingers) == 0 {
		return false
	}
	a := p.Self

	// 1. System Shop / Synthesis redemptions.
	for _, gf := range p.ActiveGoldenFingers {
		for _, offer := range gf.AvailableShop {
			// Priority A: emergency healing when wounded.
			if a.Health < 70 && offer.Reward.HealthDelta > 0 {
				if consider(sim.Action{Kind: "gf_exchange", Text: gf.ID, Item: offer.ID}) {
					return true
				}
			}
		}
	}

	// Collect items/skills needed for upward transitions or active quests.
	neededItems := map[string]bool{}
	selfTier := p.Catalog.ActorPowerTier(a)
	for _, tr := range p.Catalog.Transitions {
		if tr.RequiredFormID != "" && tr.RequiredFormID != a.FormID {
			continue
		}
		if toForm, ok := p.Catalog.Forms[tr.SuccessFormID]; ok && toForm.PowerTier > selfTier {
			for itemID, qty := range tr.ItemCosts {
				if a.Items[itemID] < qty {
					neededItems[itemID] = true
				}
			}
		}
	}
	for _, gf := range p.ActiveGoldenFingers {
		for _, q := range gf.ActiveQuests {
			if q.Kind == "acquire_item" && q.TargetID != "" && a.Items[q.TargetID] < max(1, q.TargetValue) {
				neededItems[q.TargetID] = true
			}
		}
	}

	for _, gf := range p.ActiveGoldenFingers {
		for _, offer := range gf.AvailableShop {
			// Priority B: breakthrough or quest items.
			for itemID, qty := range offer.Reward.Items {
				if qty > 0 && neededItems[itemID] {
					if consider(sim.Action{Kind: "gf_exchange", Text: gf.ID, Item: offer.ID}) {
						return true
					}
				}
			}
			// Priority C: permanent stat / skill XP / synthesis upgrades.
			if len(offer.Reward.Attributes) > 0 || len(offer.Reward.SkillXP) > 0 || len(offer.CostItems) > 0 {
				if consider(sim.Action{Kind: "gf_exchange", Text: gf.ID, Item: offer.ID}) {
					return true
				}
			}
		}
	}

	// 2. Ready life/branch simulator.
	if a.Health >= 50 && a.Energy >= 15 {
		for _, gf := range p.ActiveGoldenFingers {
			if gf.SimulatorReady {
				if consider(sim.Action{Kind: "gf_simulate", Text: gf.ID}) {
					return true
				}
			}
		}
	}

	// 3. Prophetic foresight positioning (opportunity interception / danger evasion).
	if a.Energy >= 20 {
		for _, gf := range p.ActiveGoldenFingers {
			for _, fs := range gf.ActiveForesights {
				if fs.Kind == "opportunity" && fs.DistrictID != "" && a.Location != fs.DistrictID {
					if step, ok := stepTowardDistrict(p, fs.DistrictID, 0); ok && consider(step) {
						return true
					}
				}
				if fs.Kind == "danger" && fs.DistrictID != "" && a.Location == fs.DistrictID && len(p.Roads) > 0 {
					if consider(sim.Action{Kind: "move", Location: p.Roads[0].To}) {
						return true
					}
				}
			}
		}
	}

	// 4. Active quest pursuit.
	if a.Health >= 50 && a.Energy >= 20 {
		nearbyMap := map[string]sim.Neighbor{}
		for _, nb := range p.Nearby {
			nearbyMap[nb.ID] = nb
		}
		for _, gf := range p.ActiveGoldenFingers {
			for _, q := range gf.ActiveQuests {
				switch q.Kind {
				case "visit_district":
					if q.TargetID != "" && a.Location != q.TargetID {
						if step, ok := stepTowardDistrict(p, q.TargetID, 0); ok && consider(step) {
							return true
						}
					}
				case "reach_skill":
					if q.TargetID != "" && a.Skills[q.TargetID] < max(2, q.TargetValue) {
						if consider(sim.Action{Kind: "practice_skill", SkillID: q.TargetID}) {
							return true
						}
					}
				case "eliminate_actor":
					if nb, ok := nearbyMap[q.TargetID]; ok && !nb.Jailed {
						if consider(sim.Action{Kind: "attack", TargetID: nb.ID}) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

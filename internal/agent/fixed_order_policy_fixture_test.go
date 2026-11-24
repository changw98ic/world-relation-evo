package agent

import (
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"sort"
)

// rulesPolicy ranks offline choices using only generic catalog recipes, professions, guilds,
// house assets, and road-graph shortest paths—never hardcoded profession branches.
type rulesPolicy struct{ options []Option }

// consider returns true when a preference matches an available system action.
func (policy rulesPolicy) preferences(p sim.Perception, consider func(sim.Action) bool) {
	p.Self.Plan = sim.ClonePersonalPlan(p.Self.Plan)
	sim.SyncActorPlan(&p.Self, p.Tick, p.Catalog)
	a := p.Self
	itemKeys := sortedItemIDs(p.Catalog.Items)

	// 1a. Physiological thirst: drink fresh water at indoor/well/canal facilities or carried potable item.
	if a.Thirst >= 2 {
		if canDrawFreeWater(p) && consider(sim.Action{Kind: "drink"}) {
			return
		}
		for _, id := range itemKeys {
			if p.Catalog.Items[id].Potable && a.Items[id] >= 1 && consider(sim.Action{Kind: "drink", Item: id}) {
				return
			}
		}
		if a.Thirst >= 3 {
			if dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "water_source"); ok && dest != a.Location {
				if step, ok := stepTowardDistrict(p, dest, 0); ok && consider(step) {
					return
				}
			}
			if home := primaryHome(p); home != nil {
				if step, ok := stepToHouse(p, home.Site()); ok && consider(step) {
					return
				}
			}
		}
	}

	// 1b. Physiological hunger: eat any available edible item accepted by the actor's form.
	if a.Hunger > 0 && !p.Catalog.ActorIsHungerDisabled(a) {
		for _, id := range itemKeys {
			spec := p.Catalog.Items[id]
			if p.Catalog.ActorAcceptsFood(a, spec) && a.Items[id] >= 1 && consider(sim.Action{Kind: "eat", Item: id}) {
				return
			}
		}
	}

	// A standing crop cannot feed somebody today. Seek reachable food before
	// returning to care work or resting because starvation has lowered health.
	if a.Hunger > 0 && !p.Catalog.ActorIsHungerDisabled(a) && totalEdibleItems(p.Catalog, a) == 0 {
		for _, o := range p.Offers {
			if o.Buyer == a.ID && o.ResponderID() == a.ID && p.OfferAcceptanceBlockReason(o) == "" && p.Catalog.ActorAcceptsFood(a, p.Catalog.Items[o.Item]) && availableMoneyAfterAgreements(p) >= o.Price && a.Trust[o.Seller] >= 0 {
				for _, n := range p.Nearby {
					if n.ID == o.Seller && !n.Jailed && consider(sim.Action{Kind: "accept", OfferID: o.ID, TargetID: o.Seller}) {
						return
					}
				}
			}
		}
		if !a.Resting || a.Energy >= sim.MaxEnergy || a.Hunger >= 5 {
			if action, ok := immediateFoodAction(p, policy.options); ok {
				if !a.Resting || a.Energy >= sim.MaxEnergy {
					if consider(action) {
						return
					}
				}
				// End a rest intention for an actual food step, including travel
				// to a source, only when that step is available at this tick.
				if a.Resting && ruleOptionID(action, policy.options) != "" {
					switch action.Kind {
					case "craft", "move", "move_venue", "leave_house", "enter_house", "join_faction", "join_profession":
						if consider(sim.Action{Kind: "end_rest"}) || consider(action) {
							return
						}
					}
				}
			}
		}
	}
	if a.Resting && a.Energy < sim.MaxEnergy && consider(planSleep(p)) {
		return
	}

	for _, g := range p.Growth {
		owner := !g.Cohort && g.OwnerID == a.ID
		caretaker, canCollect := owner, owner
		for _, agreement := range p.CareAgreements {
			if agreement.GrowthID == g.ID {
				caretaker = agreement.CaretakerKind == "actor" && agreement.CaretakerID == a.ID
				canCollect = owner || caretaker && agreement.Harvest
				break
			}
		}
		if g.Ready {
			if !canCollect {
				continue
			}
			for _, action := range growthCareActions(p) {
				if action.Kind == "harvest" && action.TargetID == fmt.Sprint(g.ID) && p.Capabilities()["harvest"] && consider(action) {
					return
				}
			}
		} else if !caretaker {
			continue
		}
		needsCare := !g.Ready && g.ProgressTicks >= g.CareUntil
		if !g.Ready && !needsCare {
			continue
		}
		if needsCare {
			supplied := true
			for item, qty := range g.Rule.Supplies {
				if int64(a.Items[item]) >= int64(qty)*g.Batches {
					continue
				}
				supplied = false
				for _, r := range p.Catalog.Recipes {
					if r.OutputItem == item && canPractice(p.Catalog, a, r) {
						if act, ok := resolveRecipe(p, r, 0); ok && consider(act) {
							return
						}
					}
				}
			}
			if !supplied {
				continue
			}
		}
		if g.VenueID != "" && g.VenueID != a.VenueID {
			if step, ok := stepTowardVenue(p, g.VenueID, 0); ok && consider(step) {
				return
			}
			continue
		}
		if g.HouseID == "" && a.InsideHouse != "" && consider(sim.Action{Kind: "leave_house"}) {
			return
		}
		if g.HouseID != "" && a.InsideHouse != g.HouseID {
			for _, h := range p.AccessibleHouses {
				if h.ID == g.HouseID {
					if step, ok := stepToHouse(p, h.Site()); ok && consider(step) {
						return
					}
					break
				}
			}
			continue
		}
	}

	for _, n := range p.EnergyNodes {
		if n.StoredEnergy == 0 && n.FuelStock == 0 && n.Source != nil && n.Source.FuelItem != "" && a.Items[n.Source.FuelItem] > 0 && consider(sim.Action{Kind: "refuel", TargetID: n.ID, Quantity: int(min(int64(a.Items[n.Source.FuelItem]), n.Source.UnitsPerCycle))}) {
			return
		}
	}
	// 1c. Sanitation & nightsoil collection: relieve waste before discomfort or disease risk builds.
	if a.Waste >= 3 && consider(sim.Action{Kind: "relieve"}) {
		return
	}

	// 2. Settle incoming trade offers based on generic item demand, funds, and directed trust.
	freeMoney := availableMoneyAfterAgreements(p)
	for _, o := range p.Offers {
		if o.ResponderID() != a.ID {
			continue
		}
		other := o.Counterparty(a.ID)
		here := false
		for _, b := range p.Nearby {
			if b.ID == other && !b.Jailed {
				here = true
				break
			}
		}
		willing := false
		if o.Buyer == a.ID {
			willing = a.Items[o.Item] < desiredBuyerStock(p, o.Item) && freeMoney >= o.Price
		} else {
			// Reuse the rule policy's existing sale intent and uncommitted inventory;
			// there is no engine-wide price floor or forced assent.
			available := availableItemAfterAgreements(p, o.Item)
			willing = available >= o.Quantity && ((a.Plan != nil && a.Plan.Status == "active" && a.Plan.Item == o.Item) || p.Catalog.TagsProduce(a.IdentityTags(), o.Item) || available >= desiredBuyerStock(p, o.Item)+o.Quantity)
		}
		if here && p.OfferAcceptanceBlockReason(o) == "" && willing && a.Trust[other] >= 0 && consider(sim.Action{Kind: "accept", OfferID: o.ID, TargetID: other}) {
			return
		}
		if consider(sim.Action{Kind: "reject", OfferID: o.ID, TargetID: other}) {
			return
		}
	}

	// 2b. Self-medication and wound/disease recuperation: treat wounds or contagious/cold illness.
	if (a.Health < 85 || a.DiseaseID != "") && p.Capabilities()["medicate"] {
		bestMed := ""
		bestScore := 0
		for _, id := range itemKeys {
			spec := p.Catalog.Items[id]
			if (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) || a.Items[id] < 1 {
				continue
			}
			score := spec.MedicinalHeal
			if a.DiseaseID != "" {
				score += spec.DiseaseCure * 20
			}
			if score > bestScore {
				bestScore = score
				bestMed = id
			}
		}
		if bestMed != "" && consider(sim.Action{Kind: "medicate", Item: bestMed}) {
			return
		}
	}
	if (a.Health < 35 || (p.Tick < a.RestUntil && a.Health < 55)) && !(hasCoLocatedHostileEnemy(p) && a.Health >= 25) {
		if consider(planSleep(p)) {
			return
		}
	}

	// 3. Sleep and circadian recovery: head home to sleep at bedtime or when exhausted.
	needNightSleep := (p.Clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, p.Clock.Hour)) && a.Energy < 85
	needExhaustionRest := a.Energy < 22
	if needNightSleep || needExhaustionRest {
		if p.Capabilities()["begin_rest"] && consider(sim.Action{Kind: "begin_rest"}) {
			return
		}
		if consider(planSleep(p)) {
			return
		}
	}
	if home := primaryHome(p); home != nil && home.Condition < 70 && a.Energy >= 12 {
		repairItem := p.Catalog.EffectiveEnvironment().RepairItem
		if availableItemAfterAgreements(p, repairItem) >= 1 {
			if action, ok := stepToRepairHouse(p, *home); ok && consider(action) {
				return
			}
		} else {
			for _, recipe := range p.Catalog.Recipes {
				if recipe.OutputItem == repairItem && canPractice(p.Catalog, a, recipe) {
					if action, ok := resolveRecipe(p, recipe, 0); ok && consider(action) {
						return
					}
				}
			}
		}
	}
	// A sales goal must not leave a harvested field idle for the next season.
	// Existing standing crops are handled above; only free land can start work.
	for _, r := range p.Catalog.Recipes {
		if r.Growth != nil && p.Catalog.Items[r.OutputItem].Edible && canPractice(p.Catalog, a, r) && a.Items[r.OutputItem] < 2*r.OutputQty {
			if action, ok := resolveRecipe(p, r, 0); ok && consider(action) {
				return
			}
		}
	}

	// 3b. Active rare artifact/relic world events take priority over routine plans when healthy and capable.
	if a.Health >= 65 && a.Energy >= 16 && len(p.ActiveWorldEvents) > 0 {
		if planPartyAction(p, consider) {
			return
		}
		for _, we := range p.ActiveWorldEvents {
			if we.Category != "artifact_emergence" || we.ClaimedBy != "" {
				continue
			}
			if we.RequiredPowerTier > 0 && p.Catalog.ActorPowerTier(a) < we.RequiredPowerTier {
				continue
			}
			if we.DistrictID == a.Location {
				if a.InsideHouse != "" && consider(sim.Action{Kind: "leave_house"}) {
					return
				}
				if consider(sim.Action{Kind: "claim_world_event", TargetID: we.InstanceID}) {
					return
				}
			} else if a.Energy >= 30 {
				if step, ok := stepTowardDistrict(p, we.DistrictID, 0); ok && consider(step) {
					return
				}
			}
		}
	}

	for _, event := range p.ActiveWorldEvents {
		if event.Category == "external_invasion" && event.DistrictID == a.Location && event.InvasionThreatID != "" &&
			consider(sim.Action{Kind: "hunt_threat", TargetID: event.InvasionThreatID}) {
			return
		}
	}

	// Continuing intentions yield to physiological needs, not to unrelated daily work.
	if action, ok := chooseContinuity(p); ok && consider(action) {
		return
	}

	// 4. Community healing if capable and carrying medicinal items.
	if p.Capabilities()["heal"] {
		if medItem := cheapestMedicinalItem(p.Catalog, a, itemKeys); medItem != "" {
			for _, b := range p.Nearby {
				if b.FactionRelation == "allied" && b.Health < sim.MaxHealth && consider(sim.Action{Kind: "heal", TargetID: b.ID, Item: medItem}) {
					return
				}
			}
			for _, b := range p.Nearby {
				if b.Health < sim.MaxHealth && consider(sim.Action{Kind: "heal", TargetID: b.ID, Item: medItem}) {
					return
				}
			}
		}
	}

	// 4b. Attempt upward form/realm breakthrough when healthy and all requirements are met.
	if planGoldenFingerAction(p, consider) {
		return
	}
	if a.Health >= 55 && a.Energy >= 25 && len(p.Catalog.Transitions) > 0 {
		selfTier := p.Catalog.ActorPowerTier(a)
		for _, opt := range policy.options {
			if opt.Action.Kind != "attempt_transition" || opt.Action.TransitionID == "" {
				continue
			}
			trans, ok := p.Catalog.Transitions[opt.Action.TransitionID]
			if !ok || trans.SuccessFormID == "" || trans.SuccessFormID == a.FormID {
				continue
			}
			targetTier := p.Catalog.Forms[trans.SuccessFormID].PowerTier
			if targetTier >= selfTier && consider(opt.Action) {
				return
			}
		}
	}

	// 4c. Train skills or run custom craft/progression ActionSpecs that advance toward upward transitions.
	if a.Health >= 55 && a.Energy >= 30 && !hasCoLocatedHostileEnemy(p) {
		neededSkills := map[string]int{}
		neededItems := map[string]int{}
		neededSlots := map[string]int{}
		selfTier := p.Catalog.ActorPowerTier(a)
		for _, tr := range p.Catalog.Transitions {
			if tr.RequiredFormID != "" && tr.RequiredFormID != a.FormID {
				continue
			}
			if toForm, ok := p.Catalog.Forms[tr.SuccessFormID]; ok && toForm.PowerTier > selfTier {
				for sid, reqLv := range tr.RequiredSkills {
					if a.Skills[sid] < reqLv && reqLv > neededSkills[sid] {
						neededSkills[sid] = reqLv
					}
				}
				for itemID, qty := range tr.ItemCosts {
					if a.Items[itemID] < qty && qty > neededItems[itemID] {
						neededItems[itemID] = qty
					}
				}
				for slotID, reqVal := range tr.RequiredSlots {
					if a.StateSlots[slotID] < reqVal && reqVal > neededSlots[slotID] {
						neededSlots[slotID] = reqVal
					}
				}
			}
		}
		if len(neededSkills) > 0 {
			for _, opt := range policy.options {
				if opt.Action.Kind == "practice_skill" && neededSkills[opt.Action.SkillID] > a.Skills[opt.Action.SkillID] {
					if consider(opt.Action) {
						return
					}
				}
			}
		}
		if len(p.Catalog.ActionSpecs) > 0 && (len(neededItems) > 0 || len(neededSlots) > 0) {
			for _, opt := range policy.options {
				spec, ok := p.Catalog.ActionSpecs[opt.Action.Kind]
				if !ok || (spec.TargetKind != "" && spec.TargetKind != "self") {
					continue
				}
				useful := false
				for itemID, gain := range spec.ItemGains {
					if gain > 0 && neededItems[itemID] > a.Items[itemID] {
						useful = true
						break
					}
				}
				if !useful {
					for slotID, gain := range spec.SlotGains {
						if gain > 0 && neededSlots[slotID] > a.StateSlots[slotID] {
							useful = true
							break
						}
					}
				}
				if useful && consider(opt.Action) {
					return
				}
			}
		}
	}

	// 6. Asset level upgrade for an owned house when a capable carpenter has materials.
	if act, ok := planHouseUpgrade(p); ok && consider(act) {
		return
	}

	// 7. Information-driven travel or rumor retelling.
	if action, ok := informationAction(p); ok && (action.Kind == "move" || action.Kind == "leave_house" || p.Tick%3 == 0) {
		if consider(action) {
			return
		}
	}

	// 8. Guild affiliation and a second profession when subsistence is secure.
	if act, ok := planGuildProgress(p); ok && consider(act) {
		return
	}

	// 8a. Credit obligations, collateral foreclosure, and peaceful feud mediation.
	if act, ok := planCreditAndRelations(p); ok && consider(act) {
		return
	}

	// 8b. Martial sparring, yamen law-enforcement arrests, bandit/gang raids, and street pickpocketing.
	if act, ok := planConflictAndLaw(p); ok && consider(act) {
		return
	}

	// 8c. Ecological, party, diplomatic, ambition & civic duties.
	if planDiplomacyAction(p, consider) {
		return
	}
	if planAmbitionAction(p, consider) {
		return
	}
	if act, ok := planEcologyRitualAndInfra(p); ok && consider(act) {
		return
	}
	// Repair any severed flow-grid edges before routine work.
	for _, edge := range p.FlowEdges {
		if edge.Severed && edge.ID != "" {
			if consider(sim.Action{Kind: "repair_flow_edge", TargetID: edge.ID}) {
				return
			}
		}
	}
	// Replenish low state slots or refine surplus dual-use currency items when beneficial.
	slotKeys := make([]string, 0, len(p.Catalog.StateSlots))
	for sid := range p.Catalog.StateSlots {
		slotKeys = append(slotKeys, sid)
	}
	sort.Strings(slotKeys)
	for _, itemID := range sortedItemIDs(p.Catalog.Items) {
		qty := a.Items[itemID]
		if qty < 2 {
			continue
		}
		spec := p.Catalog.Items[itemID]
		if spec.DualUseCurrency == "" && spec.DualUseRate <= 0 {
			continue
		}
		for _, sid := range slotKeys {
			slotSpec := p.Catalog.StateSlots[sid]
			if slotSpec.Max > 0 && a.StateSlots[sid]*2 < slotSpec.Max {
				if consider(sim.Action{Kind: "absorb_item_slot", Item: itemID, Quantity: 1, TargetID: sid}) {
					return
				}
			}
		}
		if qty >= 3 && spec.DualUseCurrency != "" && spec.DualUseRate > 0 && a.Currencies[spec.DualUseCurrency] < int64(spec.DualUseRate) {
			if consider(sim.Action{Kind: "refine_currency_item", Item: itemID, Quantity: 1}) {
				return
			}
		}
	}

	// 9. Generic profession recipe planning (inputs -> bonus inputs -> facility/district -> craft).
	if action, ok := planCatalogWork(p); ok {
		if action.Kind == "craft" && (a.Plan == nil || a.Plan.Status != "active") {
			recipe, _ := p.Catalog.RecipeByID(action.RecipeID)
			for _, candidate := range sim.ContinuityActions(p) {
				// A required intermediate is work for the final product, not a new
				// intention to sell away its own inputs.
				if candidate.Kind == "start_plan" && candidate.RecipeID == action.RecipeID && p.Catalog.TagsProduce(a.IdentityTags(), recipe.OutputItem) && consider(candidate) {
					return
				}
			}
		}
		if consider(action) {
			return
		}
	}

	// 10. Converge on the market hub to trade surplus items and exchange news.
	if marketID, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "market"); ok && a.Location != marketID {
		if step, ok := stepTowardDistrict(p, marketID, 0); ok && consider(step) {
			return
		}
	} else if a.InsideHouse != "" {
		if consider(sim.Action{Kind: "leave_house"}) {
			return
		}
	}

	if len(p.Nearby) > 0 {
		// Offer surplus tradeable goods (specialty products first) to co-located villagers.
		hasPendingOffer := false
		for _, o := range p.Offers {
			if o.Seller == a.ID {
				hasPendingOffer = true
				break
			}
		}
		if !hasPendingOffer && !recentlyMarketSaturated(a, p.Tick) {
			offerOrder := append([]string(nil), itemKeys...)
			sort.SliceStable(offerOrder, func(i, j int) bool {
				pi := p.Catalog.TagsProduce(a.IdentityTags(), offerOrder[i])
				pj := p.Catalog.TagsProduce(a.IdentityTags(), offerOrder[j])
				return pi && !pj
			})
			for _, itemID := range offerOrder {
				if p.Polity != nil && sim.ActorHasTag(p.Polity.EmbargoItems, itemID) {
					continue
				}
				spec := p.Catalog.Items[itemID]
				availQty := availableItemAfterAgreements(p, itemID)
				if !spec.Tradeable || spec.BatchQty <= 0 || availQty < spec.OfferThreshold || availQty < spec.BatchQty {
					continue
				}
				if !p.Catalog.TagsProduce(a.IdentityTags(), itemID) && availQty < desiredBuyerStock(p, itemID)+spec.BatchQty {
					continue
				}
				for _, b := range p.Nearby {
					if b.Sleeping || b.Jailed {
						continue
					}
					if p.Catalog.TagsProduce(b.Tags, itemID) {
						continue
					}
					if !p.Catalog.TagsConsume(b.Tags, itemID) {
						continue
					}
					if recentlyRejectedBy(a, b.ID, p.Tick) {
						continue
					}
					if consider(sim.Action{
						Kind: "offer", TargetID: b.ID,
						Item: itemID, Quantity: spec.BatchQty, Price: p.Catalog.SalePrice(a, itemID),
					}) {
						return
					}
				}
			}
		}

		if !recentlyChatted(a, p.Tick) {
			for idx := 0; idx < len(p.Nearby); idx++ {
				b := p.Nearby[(int(p.Tick)+idx)%len(p.Nearby)]
				if !b.Sleeping && !b.Jailed {
					if consider(sim.Action{
						Kind:     "speak",
						TargetID: b.ID,
						Text:     fmt.Sprintf("Checking in from %s; supplies=%d, energy=%d.", a.Location, totalEdibleItems(p.Catalog, a), a.Energy),
					}) {
						return
					}
				}
			}
		}
	}

	// 11. Commercial outreach: after day 1, idle traders at a market who still need target-stock items
	// periodically travel to a neighboring district to procure goods or network.
	if p.Tick >= sim.TicksPerDay && !p.Clock.IsNight && p.Clock.Hour >= 8 && p.Clock.Hour <= 16 &&
		a.InsideHouse == "" && a.Energy >= 45 && a.Health >= 75 && p.Tick >= a.RestUntil &&
		len(p.Map.Districts) > 1 && p.Capabilities()["offer"] && availableMoneyAfterAgreements(p) >= 12 &&
		p.Map.DistrictHasFeature(a.Location, "market") && !recentlyMovedDistrict(a, p.Tick, 96) {
		needsStock := false
		for _, tag := range a.IdentityTags() {
			if prof, ok := p.Catalog.TagProfiles[tag]; ok {
				for itemID, targetQty := range prof.TargetStock {
					if a.Items[itemID] < targetQty {
						needsStock = true
						break
					}
				}
			}
			if needsStock {
				break
			}
		}
		if needsStock {
			if len(p.VenueRoutes) > 0 {
				var cross []sim.VenueRouteView
				for _, vr := range p.VenueRoutes {
					if vr.ToDistrict != "" && vr.ToDistrict != a.Location && a.Energy >= vr.EnergyCost {
						cross = append(cross, vr)
					}
				}
				if len(cross) > 0 {
					vr := cross[(int(p.Tick/16)+len(a.ID))%len(cross)]
					if consider(sim.Action{Kind: "move_venue", Location: vr.ToVenue, Channel: vr.ModeID}) {
						return
					}
				}
			} else if len(p.Roads) > 0 {
				rd := p.Roads[(int(p.Tick/16)+len(a.ID))%len(p.Roads)]
				if a.Energy >= rd.EnergyCost && consider(sim.Action{Kind: "move", Location: rd.To}) {
					return
				}
			}
		}
	}

	consider(sim.Action{Kind: "wait"})
}

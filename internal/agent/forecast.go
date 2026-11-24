package agent

import (
	"maps"
	"slices"
	"strconv"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// decisionFacts indexes only the terms and observations delivered in a
// Perception. It has no access to World or to counterparties' inventories.
type decisionFacts struct {
	sim.Perception
	offers        map[int64]sim.Offer
	agreements    map[int64]sim.Agreement
	neighbors     map[string]sim.Neighbor
	growth        map[string]sim.GrowthBatch
	quotes        map[quoteKey]quoteEvidence
	awaitingReply bool
}

type quoteKey struct {
	peer, item, currency, district, venue, house string
	quantity, price                              int
	selling                                      bool
}

type quoteEvidence struct {
	accepted, resolved int
	missed, requested  int64
	open               bool
}

func termsKey(actor string, o sim.Offer) quoteKey {
	currency := o.Currency
	if currency == "" {
		currency = "coins"
	}
	return quoteKey{peer: o.Counterparty(actor), item: o.Item, currency: currency,
		district: o.District, venue: o.Venue, house: o.House, quantity: o.Quantity,
		price: o.Price, selling: actor == o.Seller}
}

func projectFacts(p sim.Perception) decisionFacts {
	facts := decisionFacts{Perception: p, offers: make(map[int64]sim.Offer, len(p.Offers)),
		agreements: make(map[int64]sim.Agreement, len(p.Continuity.Agreements)),
		neighbors:  make(map[string]sim.Neighbor, len(p.Nearby)),
		growth:     make(map[string]sim.GrowthBatch, len(p.Growth))}
	for _, offer := range p.Offers {
		facts.offers[offer.ID] = offer
	}
	for _, agreement := range p.Continuity.Agreements {
		facts.agreements[agreement.ID] = agreement
	}
	for _, neighbor := range p.Nearby {
		facts.neighbors[neighbor.ID] = neighbor
		if q, open := p.OpenQuestions[neighbor.ID]; open && q.Source == p.Self.ID && !neighbor.Sleeping && !neighbor.Jailed {
			facts.awaitingReply = true
		}
	}
	for _, growth := range p.Growth {
		facts.growth[strconv.FormatInt(growth.ID, 10)] = growth
	}
	facts.quotes = observedQuotes(p)
	return facts
}

func observedQuotes(p sim.Perception) map[quoteKey]quoteEvidence {
	quotes := make(map[quoteKey]quoteEvidence)
	for _, memory := range p.Self.Memories {
		if memory.Trade == nil {
			continue
		}
		terms := *memory.Trade
		if terms.Seller != p.Self.ID && terms.Buyer != p.Self.ID {
			continue
		}
		key := termsKey(p.Self.ID, terms)
		evidence := quotes[key]
		switch memory.Kind {
		case "trade_counteroffered":
			if terms.ProposerID() != p.Self.ID {
				evidence.requested = memory.EventID
			}
		case "trade_completed", "agreement_accepted":
			if terms.ProposerID() == p.Self.ID {
				evidence.accepted++
				evidence.resolved++
			} else {
				evidence.requested = memory.EventID
			}
		case "trade_expired", "trade_rejected", "agreement_expired", "agreement_rejected":
			if terms.ProposerID() == p.Self.ID {
				evidence.resolved++
				evidence.missed = memory.EventID
			}
		}
		quotes[key] = evidence
	}
	for _, offer := range p.Offers {
		if offer.ProposerID() == p.Self.ID && offer.Expires >= p.Tick {
			if offer.District == "" {
				offer.District, offer.Venue, offer.House = p.Self.Location, p.Self.VenueID, p.Self.InsideHouse
			}
			key := termsKey(p.Self.ID, offer)
			evidence := quotes[key]
			evidence.open = true
			quotes[key] = evidence
		}
	}
	for _, agreement := range p.Continuity.Agreements {
		if agreement.SupplierID == p.Self.ID && (agreement.Status == "proposed" || agreement.Status == "active") {
			terms := sim.Offer{Seller: p.Self.ID, Buyer: agreement.BuyerID, Item: agreement.Item, Quantity: agreement.Quantity,
				Price: agreement.Price, District: agreement.District, Venue: agreement.Venue, House: agreement.House}
			if terms.District == "" {
				terms.District, terms.Venue, terms.House = p.Self.Location, p.Self.VenueID, p.Self.InsideHouse
			}
			key := termsKey(p.Self.ID, terms)
			evidence := quotes[key]
			evidence.open = true
			quotes[key] = evidence
		}
	}
	return quotes
}

func (p decisionFacts) forecast(candidate sim.Candidate) forecast {
	x, a := candidate.Action, p.Self
	ticks, energyCost := p.ActionWorkEstimate(x)
	f := forecast{candidate: candidate, duration: int64(ticks), probability: 1}
	goal := sim.NeedKey{Measure: x.Kind, Subject: x.TargetID}
	switch x.Kind {
	case "eat":
		spec := p.Catalog.Items[x.Item]
		f.change("nutrition", "", 0, float64(spec.NutritionUnits))
		p.item(&f, x.Item, 0, -1)
		p.consumable(&f, spec)
		goal = sim.NeedKey{Measure: "nutrition"}
	case "drink":
		relief := a.Thirst
		if x.Item != "" {
			spec := p.Catalog.Items[x.Item]
			relief = spec.DrinkingRelief()
			p.item(&f, x.Item, 0, -1)
			p.consumable(&f, spec)
		}
		f.change("hydration", "", 0, float64(min(relief, a.Thirst)))
		goal = sim.NeedKey{Measure: "hydration"}
	case "sleep":
		p.sleep(&f, a, p.CurrentHouse, 0, max(x.Quantity, 1))
		goal = sim.NeedKey{Measure: "energy"}
	case "relieve":
		f.change("hygiene", "", 0, float64(a.Waste))
		goal = sim.NeedKey{Measure: "hygiene"}
	case "medicate":
		spec := p.Catalog.Items[x.Item]
		f.change("health", "", 0, float64(min(spec.MedicinalHeal, sim.MaxHealth-a.Health)))
		p.item(&f, x.Item, 0, -1)
		goal = sim.NeedKey{Measure: "health"}
	case "heal":
		spec := p.Catalog.Items[x.Item]
		target := p.neighbors[x.TargetID]
		f.change("health", target.ID, 0, float64(min(max(15, spec.MedicinalHeal-5)+p.Catalog.SkillHealBonus(a), sim.MaxHealth-target.Health)))
		f.change("relationship", target.ID, 0, 1)
		p.item(&f, x.Item, 0, -1)
		goal = sim.NeedKey{Measure: "health", Subject: target.ID}
	case "move", "move_venue", "leave_house", "enter_house", "begin_rest":
		ticks, energy := p.travel(x)
		f.duration = ticks
		energyCost += energy
	case "craft":
		energyCost = 0 // The production forecast also serves multi-step crafting.
		if recipe, ok := p.Catalog.RecipeByID(x.RecipeID); ok {
			p.craft(&f, recipe, 0, true)
			if recipe.Growth == nil {
				p.planEating(&f, recipe.OutputItem, f.duration, p.craftYield(recipe, true))
			}
			goal = p.craftingGoal(recipe)
		}
	case "harvest":
		if growth, ok := p.growth[x.TargetID]; ok {
			for _, care := range p.CareAgreements {
				if care.GrowthID == growth.ID {
					f.duration = max(1, growth.HarvestWork)
					break
				}
			}
			quantity := growth.Quantity
			for _, care := range p.CareAgreements {
				if care.GrowthID != growth.ID {
					continue
				}
				share := quantity * int64(care.SharePermille) / 1000
				if growth.OwnerID != a.ID {
					quantity = share
				} else {
					quantity -= share
				}
				break
			}
			p.item(&f, growth.Item, f.duration, float64(quantity))
			if growth.OwnerID == a.ID && !growth.Cohort {
				if recipe, known := p.Catalog.RecipeByID(growth.RecipeID); known {
					p.professionXP(&f, recipe.Profession, f.duration, p.Catalog.CraftXP(recipe))
				}
			}
			p.planEating(&f, growth.Item, f.duration, int(quantity))
			goal = p.productionGoal(growth.Item)
		}
	case "repair_house":
		p.item(&f, p.Catalog.EffectiveEnvironment().RepairItem, 0, -1)
		f.change("housing", "", f.duration, p.houseRepair(x.HouseID))
		f.change("housing", x.HouseID, f.duration, p.houseRepair(x.HouseID))
		goal = sim.NeedKey{Measure: "housing", Subject: x.HouseID}
	case "accept":
		if offer, ok := p.offers[x.OfferID]; ok {
			selling := a.ID == offer.Seller
			p.exchange(&f, selling, offer.Item, offer.Currency, offer.Quantity, offer.Price, 0)
			p.tradeXP(&f, offer.Item, offer.Quantity, 0)
			if a.ID == offer.Buyer && a.Items[offer.Item] == 0 {
				p.planEating(&f, offer.Item, 1, offer.Quantity)
			}
			goal = p.exchangeGoal(offer.Item, selling, offer.Price)
		}
	case "market_buy", "market_sell":
		selling := x.Kind == "market_sell"
		p.exchange(&f, selling, x.Item, x.Currency, x.Quantity, x.Price, 0)
		if selling {
			p.tradeXP(&f, x.Item, x.Quantity, 0)
		} else if a.Items[x.Item] == 0 {
			p.planEating(&f, x.Item, 1, x.Quantity)
		}
		goal = p.exchangeGoal(x.Item, selling, x.Price)
	case "accept_agreement", "fulfill_agreement":
		if agreement, ok := p.agreements[x.AgreementID]; ok {
			at := int64(0)
			if x.Kind == "accept_agreement" {
				// Assent is its own phase. Only value the exchange within
				// this horizon when this seller is displaying enough stock.
				at = 1
				if p.Tick+at > agreement.DueTick || a.Money < agreement.Price {
					break
				}
				if !p.observedAgreementSupply(agreement) {
					seller := p.neighbors[agreement.SupplierID]
					if !p.Catalog.TagsProduce(seller.Tags, agreement.Item) {
						break
					}
					f.probability = p.Catalog.EffectivePlanningRules().OfferAcceptancePrior
				}
				cost := p.ActionEnergyCost(sim.Action{Kind: "market_buy", Item: agreement.Item, Quantity: agreement.Quantity})
				f.change("energy", "", at, -float64(cost))
			} else if p.ObservedPaymentFailure(agreement.BuyerID, agreement.Price) {
				break
			}
			selling := a.ID == agreement.SupplierID
			p.exchange(&f, selling, agreement.Item, "coins", agreement.Quantity, agreement.Price, at)
			goal = p.exchangeGoal(agreement.Item, selling, agreement.Price)
			if x.Kind == "fulfill_agreement" {
				goal = sim.NeedKey{Measure: "obligation", Subject: strconv.FormatInt(agreement.ID, 10)}
				f.change(goal.Measure, goal.Subject, at, 1)
			}
		}
	case "start_plan":
		if recipe, ok := p.Catalog.RecipeByID(x.RecipeID); ok {
			f = p.beginProduction(f, recipe)
			goal = p.craftingGoal(recipe)
		}
	case "offer", "propose_agreement":
		selling := x.OfferID == 0 || a.ID == p.offers[x.OfferID].Seller
		if selling && (x.Currency == "" || x.Currency == "coins") && p.ObservedPaymentFailure(x.TargetID, x.Price) {
			break
		}
		at := int64(1)
		if x.Kind == "propose_agreement" {
			// Assent and later delivery are distinct phases.
			if a.Items[x.Item] < x.Quantity {
				break
			}
			at++
		}
		// The buyer evaluates the terms independently.
		if chance := p.exchangeLikelihood(x, selling); chance > 0 {
			f.probability = chance
			cost := p.ActionEnergyCost(sim.Action{Kind: "market_sell", Item: x.Item, Quantity: x.Quantity})
			f.change("energy", "", at, -float64(cost))
			p.exchange(&f, selling, x.Item, x.Currency, x.Quantity, x.Price, at)
			if x.Kind == "offer" {
				p.tradeXP(&f, x.Item, x.Quantity, at)
			}
			goal = p.exchangeGoal(x.Item, selling, x.Price)
		}
	case "practice_skill":
		f.change("skill", x.SkillID, 0, 4)
		goal = sim.NeedKey{Measure: "skill", Subject: x.SkillID}
	case "speak":
		goal = p.forecastSpeech(&f)
	case "teach_knowledge":
		if x.SkillID != "" {
			f.change("relationship", x.TargetID, 0, 1)
			p.socialContact(&f, 0)
		}
	case "research_tech":
		if spec, ok := p.Catalog.Techs[x.TechID]; ok {
			for _, item := range sortedIntMapKeys(spec.ResearchInputs) {
				p.item(&f, item, 0, -float64(spec.ResearchInputs[item]))
			}
			f.change("knowledge", x.TechID, f.duration, 1)
			goal = sim.NeedKey{Measure: "knowledge", Subject: x.TechID}
		}
	case "attempt_transition":
		if trans, ok := p.Catalog.Transitions[x.TransitionID]; ok {
			for _, item := range sortedIntMapKeys(trans.ItemCosts) {
				p.item(&f, item, 0, -float64(trans.ItemCosts[item]))
			}
			for _, slot := range sortedIntMapKeys(trans.SlotCosts) {
				f.change("state_slot", slot, 0, -float64(trans.SlotCosts[slot]))
			}
			f.unconditional = slices.Clone(f.effects)
			f.probability = float64(trans.SuccessPermille(a)) / 1000
			if next, known := p.Catalog.Forms[trans.SuccessFormID]; known {
				f.change("power_tier", "", f.duration, float64(next.PowerTier-p.Catalog.ActorPowerTier(a)))
			}
			var failed forecast
			failed.change("health", "", f.duration, -float64(min(a.Health, trans.FailureHealthLoss)))
			if next, known := p.Catalog.Forms[trans.FailureFormID]; known {
				failed.change("power_tier", "", f.duration, float64(next.PowerTier-p.Catalog.ActorPowerTier(a)))
			}
			f.failure = failed.effects
			goal = sim.NeedKey{Measure: "power_tier"}
		}
	case "claim_world_event":
		for _, event := range p.ActiveWorldEvents {
			if event.InstanceID == x.TargetID {
				p.item(&f, event.RewardItem, 0, float64(event.RewardQty))
				p.money(&f, "coins", 0, float64(event.RewardCoins))
				break
			}
		}
	case "absorb_item_slot":
		spec := p.Catalog.Items[x.Item]
		p.consumable(&f, spec)
		p.item(&f, x.Item, 0, -1)
	case "hunt_threat":
		if p.DistrictState != nil {
			reduction := float64(p.DistrictState.ThreatActivity[x.TargetID])
			f.change("safety", "", f.duration, reduction)
			f.change("safety", x.TargetID, f.duration, reduction)
		}
		goal = sim.NeedKey{Measure: "safety", Subject: x.TargetID}
	default:
		if spec, ok := p.Catalog.ActionSpecs[x.Kind]; ok {
			p.declarative(&f, spec)
			if len(spec.SlotGains) == 1 {
				for slot := range spec.SlotGains {
					goal = sim.NeedKey{Measure: "state_slot", Subject: slot}
				}
			}
		}
	}
	if energyCost != 0 {
		f.change("energy", "", 0, -float64(energyCost))
		if f.probability != 1 {
			f.unconditional = append(f.unconditional, f.effects[len(f.effects)-1])
		}
	}
	if x.WorkOrderID != "" {
		if order, known := p.Catalog.WorkOrder(x.WorkOrderID); known {
			p.money(&f, "coins", order.PayAt(p.Tick+f.duration-1)-p.Tick, float64(order.Wage))
			goal = sim.NeedKey{Measure: "money"}
			f.strategy = "work/" + order.ID + "/" + x.TargetID
		}
	}
	if f.advance || p.Self.Behavior != nil && followsStoredStep(x, p.Self.Behavior) {
		f.advance = true
		if old := p.Self.Behavior; old != nil && followsStoredStep(x, old) {
			f.strategy = old.Strategy
			goal = old.Goal
			if len(f.steps) == 0 {
				f.steps = old.Steps[1:]
			}
		}
	}
	f.advance = f.advance || len(f.effects) > 0
	f.identify(goal)
	return f
}

func (p decisionFacts) forecastSpeech(f *forecast) sim.NeedKey {
	x := f.candidate.Action
	if x.Statement != nil {
		switch x.Statement.Topic {
		case sim.ConversationQuestion:
			if p.awaitingReply {
				return sim.NeedKey{Measure: "speak", Subject: x.TargetID}
			}
			f.probability = p.Catalog.EffectivePlanningRules().OfferAcceptancePrior
			return p.socialContact(f, 1)
		case sim.ConversationReply:
			key := sim.NeedKey{Measure: "conversation", Subject: strconv.FormatInt(x.BasisEventID, 10)}
			f.change(key.Measure, key.Subject, 0, 1)
			p.socialContact(f, 0)
			return key
		}
	}
	return p.socialContact(f, 0)
}

func (p decisionFacts) socialContact(f *forecast, at int64) sim.NeedKey {
	goal := sim.NeedKey{Measure: "speak", Subject: f.candidate.Action.TargetID}
	for _, id := range slices.Sorted(maps.Keys(p.Catalog.StateSlots)) {
		spec := p.Catalog.StateSlots[id]
		if _, eligible := p.Catalog.StateSlotValue(p.Self, id); eligible && spec.ConversationGain > 0 {
			f.change("state_slot", id, at, float64(spec.ConversationGain))
			goal = sim.NeedKey{Measure: "state_slot", Subject: id}
		}
	}
	return goal
}

func (p decisionFacts) item(f *forecast, id string, at int64, quantity float64) {
	f.change("stock", id, at, quantity)
	if plan := p.Self.Plan; plan != nil && plan.RemainingToSell() > 0 && plan.Item == id {
		before := productionProgress(p.Catalog, *plan, plan.Sold, p.Self.Items[id])
		after := productionProgress(p.Catalog, *plan, plan.Sold, p.Self.Items[id]+int(quantity))
		f.change("progress", "", at, after-before)
	}
	spec := p.Catalog.Items[id]
	if p.Catalog.ActorAcceptsFood(p.Self, spec) {
		f.change("provisions", "", at, quantity*float64(spec.NutritionUnits))
	}
}

func (p decisionFacts) planEating(f *forecast, item string, at int64, quantity int) {
	spec := p.Catalog.Items[item]
	if p.Self.Hunger <= 0 || quantity <= 0 || !p.Catalog.ActorAcceptsFood(p.Self, spec) || !p.Capabilities()["eat"] {
		return
	}
	f.steps = append(f.steps, sim.Action{Kind: "eat", Item: item})
	f.change("nutrition", "", at, float64(spec.NutritionUnits))
	p.item(f, item, at, -1)
	f.duration = max(f.duration, at+1)
}

func (p decisionFacts) consumable(f *forecast, spec sim.ItemSpec) {
	f.change("energy", "", 0, float64(min(spec.EnergyRelief, sim.MaxEnergy-p.Self.Energy)))
	for _, id := range sortedIntMapKeys(spec.SlotChanges) {
		f.change("state_slot", id, 0, float64(spec.SlotChanges[id]))
	}
}

func (p decisionFacts) sleep(f *forecast, a sim.Actor, house *sim.House, begin int64, ticks int) {
	a.Energy, _ = a.EnergyAfterAwakeTicks(begin)
	carry := 0
	f.duration = begin + int64(ticks)
	energy := sim.NeedKey{Measure: "energy"}
	f.effects = append(f.effects, effect{Need: energy, At: begin, DriftChange: 1.0 / sim.AwakeEnergyIntervalTicks})
	for i := 0; i < ticks; i++ {
		at := begin + int64(i)
		gain, _ := sim.SleepRecoveryTenths(a, house, p.Tick+at)
		total := carry + gain
		delta := min(sim.MaxEnergy-a.Energy, total/10)
		f.change("energy", "", at, float64(delta))
		a.Energy += delta
		carry = total % 10
		if a.Energy == sim.MaxEnergy {
			carry = 0
		}
		if a.SleepHealsAt(p.Tick+at, house != nil) {
			f.change("health", "", at, 1)
			a.Health++
		}
		if a.Energy == sim.MaxEnergy && a.Health == sim.MaxHealth {
			f.duration = at + 1
			break
		}
	}
	f.effects = append(f.effects, effect{Need: energy, At: f.duration, DriftChange: -1.0 / sim.AwakeEnergyIntervalTicks})
}

func (p decisionFacts) travel(x sim.Action) (int64, int) {
	switch x.Kind {
	case "move":
		for _, road := range p.Roads {
			if road.To == x.Location {
				return int64(road.Ticks), road.EnergyCost
			}
		}
	case "move_venue":
		for _, route := range p.VenueRoutes {
			if route.ToVenue == x.Location && (x.Channel == "" || route.ModeID == x.Channel) {
				return int64(route.Ticks), route.EnergyCost
			}
		}
	}
	return 1, 0
}

func (p decisionFacts) craft(f *forecast, recipe sim.Recipe, start int64, local bool) {
	ticks, energy := recipe.WorkEstimate(p.CraftingSpeedBonusPct)
	f.duration = start + int64(ticks)
	f.change("energy", "", start, -float64(energy))
	p.money(f, "coins", start, -float64(recipe.MoneyCost))
	for _, item := range sortedIntMapKeys(recipe.Inputs) {
		p.item(f, item, start, -float64(recipe.Inputs[item]))
	}
	bonus := p.Catalog.ConsumablesYieldBonus(recipe) && p.Self.Items[recipe.BonusInput] > recipe.Inputs[recipe.BonusInput]
	if bonus {
		p.item(f, recipe.BonusInput, start, -1)
	}
	yield := p.craftYield(recipe, local)
	at := f.duration
	if recipe.Growth != nil {
		at += recipe.Growth.DurationTicks
		for _, item := range sortedIntMapKeys(recipe.Growth.Supplies) {
			interval := max(int64(1), recipe.Growth.MaintenanceIntervalTicks)
			p.item(f, item, f.duration, -float64(recipe.Growth.Supplies[item])*float64(recipe.Growth.DurationTicks/interval))
		}
	}
	p.item(f, recipe.OutputItem, at, float64(yield))
	if recipe.Growth == nil {
		p.professionXP(f, recipe.Profession, at, p.Catalog.CraftXP(recipe))
	}
	if recipe.SkillID != "" {
		f.change("skill", recipe.SkillID, f.duration, float64(p.Catalog.CraftXP(recipe)))
	}
}

func (p decisionFacts) professionXP(f *forecast, profession string, at int64, xp int) {
	if p.Self.Professions[profession] > 0 {
		f.change("profession_xp", profession, at, float64(xp))
		for _, spec := range p.Catalog.Professions {
			if spec.ParentProfession == profession && p.Self.Professions[spec.ID] > 0 {
				f.change("profession_xp", spec.ID, at, float64(xp))
			}
		}
	}
}

func (p decisionFacts) craftingGoal(recipe sim.Recipe) sim.NeedKey {
	goal := p.productionGoal(recipe.OutputItem)
	if goal.Measure == "progress" || goal.Measure == "provisions" && p.Self.Hunger > 0 {
		return goal
	}
	for _, ambition := range p.Self.Ambitions {
		if !ambition.Completed && ambition.Kind == "advance_profession" {
			prof, known := p.Catalog.ProfessionByID(ambition.TargetID)
			if known && (prof.ID == recipe.Profession || prof.ParentProfession == recipe.Profession) {
				return sim.NeedKey{Measure: "profession_xp", Subject: prof.ID}
			}
		}
	}
	return goal
}
func (p decisionFacts) craftYield(recipe sim.Recipe, local bool) int {
	bonus := p.Catalog.ConsumablesYieldBonus(recipe) && p.Self.Items[recipe.BonusInput] > recipe.Inputs[recipe.BonusInput]
	if local && recipe.Growth == nil {
		if expected, observed := p.ExpectedCraftYield[recipe.ID]; observed {
			return expected
		}
	}
	return p.Catalog.RecipeYield(p.Self, recipe, bonus)
}

func productionProgress(c sim.Catalog, plan sim.PersonalPlan, sold, held int) float64 {
	recipe, _ := c.RecipeByID(plan.RecipeID)
	productionWork := float64(max(recipe.Ticks, 1))
	if recipe.Growth != nil {
		productionWork += float64(recipe.Growth.DurationTicks)
	}
	// Count the fraction of remaining work covered by actual deliverable
	// stock. On sale, this fraction is replaced by completed units.
	stock := float64(min(max(0, held), max(0, plan.Quantity-sold)))
	return (float64(sold) + stock*productionWork/(productionWork+1)) / float64(plan.Quantity)
}

func (p decisionFacts) houseRepair(id string) float64 {
	for _, house := range p.AccessibleHouses {
		if house.ID == id {
			return float64(min(p.Catalog.HouseRepairGain(p.Self), sim.MaxCondition-house.Condition))
		}
	}
	return 0
}

func (p decisionFacts) money(f *forecast, currency string, at int64, amount float64) {
	if currency == "" || currency == "coins" {
		f.change("money", "", at, amount)
		f.change("money", "coins", at, amount)
	} else {
		f.change("money", currency, at, amount)
	}
}

func (p decisionFacts) exchange(f *forecast, sale bool, item, currency string, quantity, price int, at int64) {
	f.effects = append(f.effects, p.exchangeEffects(sale, item, currency, quantity, price, at)...)
}

func (p decisionFacts) tradeXP(f *forecast, item string, quantity int, at int64) {
	xp, _ := p.Catalog.TradeExperience(item, quantity)
	p.professionXP(f, "peddler", at, xp)
}

func (p decisionFacts) exchangeEffects(sale bool, item, currency string, quantity, price int, at int64) []effect {
	var exchange forecast
	stock, cash := float64(quantity), -float64(price)
	if sale {
		stock, cash = -stock, -cash
	}
	p.item(&exchange, item, at, stock)
	if sale {
		if price > 0 {
			exchange.change("market_trade", p.Self.Location, at, 1)
		}
		if plan := p.Self.Plan; plan != nil && plan.Item == item && plan.RemainingToSell() > 0 {
			held := p.Self.Items[item] - quantity
			before := productionProgress(p.Catalog, *plan, plan.Sold, held)
			after := productionProgress(p.Catalog, *plan, plan.Sold+min(quantity, plan.RemainingToSell()), held)
			exchange.change("progress", "", at, after-before)
		}
	}
	p.money(&exchange, currency, at, cash)
	return exchange.effects
}

func (p decisionFacts) observedAgreementSupply(g sim.Agreement) bool {
	for _, offer := range p.Offers {
		if offer.Seller == g.SupplierID && offer.Item == g.Item && p.Continuity.DisplayedGoods[offer.ID] >= g.Quantity {
			return true
		}
	}
	return false
}

func (p decisionFacts) exchangeLikelihood(x sim.Action, selling bool) float64 {
	currency := x.Currency
	if currency == "" {
		currency = "coins"
	}
	a := p.Self
	key := quoteKey{peer: x.TargetID, item: x.Item, currency: currency, quantity: x.Quantity, price: x.Price,
		district: a.Location, venue: a.VenueID, house: a.InsideHouse, selling: selling}
	evidence := p.quotes[key]
	// An identical message cannot advance an open negotiation or add
	// information after a missed opportunity. New terms and new interest
	// create a new choice even when the actor has previously been refused.
	if evidence.open || evidence.missed > evidence.requested {
		return 0
	}
	if x.OfferID != 0 || evidence.requested > evidence.missed {
		return p.Catalog.EffectivePlanningRules().OfferAcceptancePrior
	}
	if evidence.resolved > 0 {
		return float64(evidence.accepted) / float64(evidence.resolved)
	}
	// A response accepting a higher price for the same lot is evidence for
	// a lower price at that market. It says nothing about another good.
	if selling {
		var accepted, resolved int
		for terms, previous := range p.quotes {
			compatible := terms.price > key.price && terms.quantity <= key.quantity
			terms.price, terms.quantity = key.price, key.quantity
			if compatible && terms == key && previous.accepted > 0 {
				accepted += previous.accepted
				resolved += previous.resolved
			}
		}
		if resolved > 0 {
			return float64(accepted) / float64(resolved)
		}
	}
	// A documented craft input is a clue, not knowledge of a person's
	// inventory. Daily consumption alone is not a reason to solicit everyone.
	if n, seen := p.neighbors[x.TargetID]; seen && selling && x.Kind == "offer" {
		if a.Plan != nil && a.Plan.RemainingToSell() > 0 && a.Plan.Item == x.Item && p.Catalog.TagsConsume(n.Tags, x.Item) {
			return p.Catalog.EffectivePlanningRules().OfferAcceptancePrior
		}
		for _, recipe := range p.Catalog.Recipes {
			if recipe.Inputs[x.Item] > 0 && recipe.Profession != "" && sim.ActorHasTag(n.Tags, recipe.Profession) {
				return p.Catalog.EffectivePlanningRules().OfferAcceptancePrior
			}
		}
	}
	return 0
}

func (p decisionFacts) exchangeGoal(item string, selling bool, price int) sim.NeedKey {
	goal := p.productionGoal(item)
	if !selling || price <= 0 || goal.Measure == "progress" {
		return goal
	}
	for _, ambition := range p.Self.Ambitions {
		if !ambition.Completed && ambition.Kind == "trade_in_district" && ambition.TargetID == p.Self.Location {
			return sim.NeedKey{Measure: "market_trade", Subject: ambition.TargetID}
		}
	}
	for _, ambition := range p.Self.Ambitions {
		if !ambition.Completed && ambition.Kind == "accumulate_wealth" {
			return sim.NeedKey{Measure: "money"}
		}
	}
	return goal
}
func (p decisionFacts) productionGoal(item string) sim.NeedKey {
	if plan := p.Self.Plan; plan != nil && plan.Status == "active" && plan.Item == item {
		return sim.NeedKey{Measure: "progress", Subject: item}
	}
	if p.Catalog.ActorAcceptsFood(p.Self, p.Catalog.Items[item]) {
		return sim.NeedKey{Measure: "provisions", Subject: item}
	}
	return sim.NeedKey{Measure: "stock", Subject: item}
}

func (p decisionFacts) declarative(f *forecast, spec sim.ActionSpec) {
	f.duration = int64(max(1, spec.Ticks))
	p.money(f, "coins", 0, -float64(spec.MoneyCost))
	for _, item := range sortedIntMapKeys(spec.ItemCosts) {
		p.item(f, item, 0, -float64(spec.ItemCosts[item]))
	}
	for _, currency := range sortedIntMapKeys(spec.CurrencyCosts) {
		p.money(f, currency, 0, -float64(spec.CurrencyCosts[currency]))
	}
	for _, item := range sortedIntMapKeys(spec.ItemGains) {
		p.item(f, item, f.duration, float64(spec.ItemGains[item]))
	}
	for _, slot := range sortedIntMapKeys(spec.SlotCosts) {
		f.change("state_slot", slot, 0, -float64(spec.SlotCosts[slot]))
	}
	for _, slot := range sortedIntMapKeys(spec.SlotGains) {
		f.change("state_slot", slot, f.duration, float64(spec.SlotGains[slot]))
	}
	profession := spec.XPProfession
	if profession == "" {
		profession = spec.Profession
	}
	p.professionXP(f, profession, f.duration, spec.ProfessionXP)
	if spec.TargetKind == "" || spec.TargetKind == "self" {
		f.change("energy", "", f.duration, float64(spec.EnergyGain))
		f.change("health", "", f.duration, float64(spec.HealthGain))
	} else if spec.TargetKind == "actor" {
		f.change("energy", f.candidate.Action.TargetID, f.duration, float64(spec.EnergyGain))
		f.change("health", f.candidate.Action.TargetID, f.duration, float64(spec.HealthGain))
	}
	if spec.ThreatID != "" {
		f.change("safety", "", f.duration, float64(spec.ReduceThreat))
		f.change("safety", spec.ThreatID, f.duration, float64(spec.ReduceThreat))
	}
}

func (p decisionFacts) beginProduction(f forecast, recipe sim.Recipe) forecast {
	next := sim.Action{Kind: "craft", RecipeID: recipe.ID}
	if p.CheckAction(next).Allowed {
		f.steps = []sim.Action{next}
		p.craft(&f, recipe, 1, true)
		return f
	}
	if x, ok := resolveRecipe(p.Perception, recipe, 0); ok && p.CheckAction(x).Allowed {
		f.steps = []sim.Action{x}
	}
	return f
}

func (p decisionFacts) preparations(alternatives []forecast, base forecast) []forecast {
	if rest := p.prepareRest(base); rest.goal.Measure == "energy" {
		alternatives = append(alternatives, rest)
	}
	return alternatives
}

func (p decisionFacts) prepareRest(f forecast) forecast {
	home, ok := p.KnownHouse(p.Self.HomeID)
	if !ok || p.Self.Energy >= sim.MaxEnergy && p.Self.Health >= sim.MaxHealth {
		return f
	}
	a := p.Self
	x := f.candidate.Action
	f.effects = slices.Clone(f.effects)
	if x.Kind == "begin_rest" {
		next := planSleep(p.Perception)
		if next.Kind == "sleep" {
			f.steps = []sim.Action{next}
			p.sleep(&f, a, p.CurrentHouse, 1, next.Quantity)
		} else if next.Kind != "wait" && p.CheckAction(next).Allowed {
			hop := forecast{candidate: sim.Candidate{Action: next}, duration: 1}
			hop.duration, _ = p.travel(next)
			afterPreparation := p
			afterPreparation.Tick += f.duration
			afterPreparation.Clock = sim.ClockAt(afterPreparation.Tick)
			afterPreparation.Self.Energy, afterPreparation.Self.AwakeEnergyRemainder = a.EnergyAfterAwakeTicks(f.duration)
			projected := afterPreparation.prepareRest(hop)
			if len(projected.steps) > 0 {
				for _, e := range projected.effects {
					e.At += f.duration
					f.effects = append(f.effects, e)
				}
				f.duration += projected.duration
				f.steps = append([]sim.Action{next}, projected.steps...)
			}
		}
	} else if x.Kind == "enter_house" && x.HouseID == home.ID {
		p.sleepAfterTravel(&f, a, home.ID, f.duration, nil)
	} else if next, ok := stepToHouse(p.Perception, home); ok && actionMatchesDesired(x, next) {
		travel, energy, steps, ok := p.routeTo(home.District, home.VenueID, home.ID)
		if ok && a.Energy >= energy && actionMatchesDesired(x, steps[0]) {
			_, already := p.travel(x)
			f.change("energy", "", 0, -float64(energy-already))
			a.Energy -= energy
			p.sleepAfterTravel(&f, a, home.ID, travel, steps[1:])
		}
	}
	if len(f.steps) > 0 {
		f.identify(sim.NeedKey{Measure: "energy"})
		f.strategy = "rest/" + home.ID
		f.advance = true
	}
	return f
}

func (p decisionFacts) sleepAfterTravel(f *forecast, a sim.Actor, houseID string, travel int64, route []sim.Action) {
	arrival := a
	arrival.Energy, _ = a.EnergyAfterAwakeTicks(travel)
	ticks := sleepDuration(arrival, sim.ClockAt(p.Tick+travel))
	f.steps = append(route, sim.Action{Kind: "sleep", Quantity: ticks})
	a.InsideHouse = houseID
	p.sleep(f, a, p.ObservedShelter(houseID), travel, ticks)
}

func (p decisionFacts) routeTo(district, venue, house string) (int64, int, []sim.Action, bool) {
	a := p.Self
	var route []sim.Action
	var ticks int64
	var cost int
	if district == "" {
		return 0, 0, nil, false
	}
	if a.InsideHouse != "" && a.InsideHouse != house {
		route = append(route, sim.Action{Kind: "leave_house"})
		ticks++
	}
	if a.Location != district || venue != "" && a.VenueID != venue {
		path, ok := p.Perception.PlanRoute(district, venue)
		if !ok {
			return 0, 0, nil, false
		}
		route = append(route, path.Steps...)
		ticks += path.Ticks
		cost += path.Energy
	}
	if house != "" && a.InsideHouse != house {
		route = append(route, sim.Action{Kind: "enter_house", HouseID: house})
		ticks++
	}
	return ticks, cost, route, len(route) > 0
}

package agent

import (
	"maps"
	"slices"
	"strconv"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// A recipe strategy carries just the actor and the locally known work sites
// through the prerequisite chain. It never applies a change to World.
type recipeStrategy struct {
	facts decisionFacts
	plan  forecast
	steps []sim.Action
}

func (s recipeStrategy) branch() recipeStrategy {
	items := make(map[string]int, len(s.facts.Self.Items))
	maps.Copy(items, s.facts.Self.Items)
	s.facts.Self.Items = items
	s.plan.effects = slices.Clone(s.plan.effects)
	s.steps = slices.Clone(s.steps)
	return s
}

func (s *recipeStrategy) spendActionEnergy(action sim.Action) bool {
	cost := s.facts.ActionEnergyCost(action)
	if s.facts.Self.Energy < cost {
		return false
	}
	s.facts.Self.Energy -= cost
	s.plan.change("energy", "", s.plan.duration, -float64(cost))
	return true
}

func (s *recipeStrategy) elapseAwake(ticks int64) {
	a := &s.facts.Self
	a.Energy, a.AwakeEnergyRemainder = a.EnergyAfterAwakeTicks(ticks)
	s.plan.duration += ticks
}

func (p decisionFacts) productionStrategies(plans []forecast, candidates []sim.Candidate) []forecast {
	actor := p.Self
	byOutput := make(map[string][]sim.Recipe)
	bids := make(map[string][]sim.Action)
	for _, lot := range p.MarketLots {
		if lot.Kind == "market_sell" && actor.Items[lot.Item] < lot.Quantity {
			bids[lot.Item] = append(bids[lot.Item], lot)
		}
	}
	for _, recipe := range p.Catalog.Recipes {
		if canPractice(p.Catalog, actor, recipe) {
			byOutput[recipe.OutputItem] = append(byOutput[recipe.OutputItem], recipe)
		}
	}
	for _, recipe := range p.Catalog.Recipes {
		output := p.Catalog.Items[recipe.OutputItem]
		if !canPractice(p.Catalog, actor, recipe) && !(actor.Hunger > 0 &&
			p.Catalog.AvailableNutrition(actor)-actor.NutritionReserve == 0 &&
			p.Catalog.ActorAcceptsFood(actor, output) && recipe.Growth == nil) {
			continue
		}
		subject := p.craftingGoal(recipe)
		if actor.Plan != nil && actor.Plan.RemainingToSell() > 0 &&
			subject.Measure != "progress" && !p.Catalog.ActorAcceptsFood(actor, output) {
			continue
		}
		if !output.Edible && subject.Measure != "profession_xp" && !p.Catalog.TagsProduce(actor.IdentityTags(), recipe.OutputItem) &&
			p.Catalog.DesiredStock(actor.IdentityTags(), recipe.OutputItem) == 0 && subject.Measure != "progress" && len(bids[recipe.OutputItem]) == 0 {
			continue
		}
		if recipe.Growth != nil && p.ExpectedGrowthStock(recipe.OutputItem) > 0 {
			continue
		}
		chain := recipeStrategy{facts: p, plan: forecast{probability: 1}}
		chain = chain.branch()
		if !chain.qualify(recipe) || !chain.make(recipe, byOutput, map[string]bool{}) {
			continue
		}
		first, exists := findActionCandidate(candidates, chain.steps[0])
		if !exists {
			continue
		}
		for _, lot := range bids[recipe.OutputItem] {
			plans = chain.sellProduction(plans, first, recipe.ID, lot)
		}
		if len(chain.steps) > 1 {
			plan := chain.plan
			if recipe.Growth == nil && actor.Items[recipe.OutputItem] == 0 {
				chain.facts.planEating(&plan, recipe.OutputItem, plan.duration, chain.facts.Self.Items[recipe.OutputItem])
			}
			plan.candidate = first
			plan.steps = append(chain.steps[1:], plan.steps...)
			plan.goal, plan.strategy, plan.advance = subject, "produce/"+recipe.ID, true
			plans = append(plans, plan)
		}
	}
	return p.deliveryStrategies(plans, candidates, byOutput)
}

func (s recipeStrategy) sellProduction(plans []forecast, first sim.Candidate, recipeID string, lot sim.Action) []forecast {
	if s.facts.Self.Items[lot.Item] < lot.Quantity {
		return plans
	}
	s = s.branch()
	if !s.moveTo(lot.Location, "", "") || !s.facts.CheckAction(lot).Allowed || !s.spendActionEnergy(lot) {
		return plans
	}
	plan := s.plan
	s.facts.exchange(&plan, true, lot.Item, lot.Currency, lot.Quantity, lot.Price, plan.duration)
	s.facts.tradeXP(&plan, lot.Item, lot.Quantity, plan.duration)
	plan.duration++
	plan.steps = append(s.steps[1:], lot)
	plan.candidate, plan.advance = first, true
	plan.goal = sim.NeedKey{Measure: "money"}
	plan.strategy = "produce_sale/" + recipeID + "/" + lot.TargetID
	return append(plans, plan)
}

// A public venue feature is visible on the map. The projected action still
// needs to pass the same eligibility and resource checks at that destination.
func (p decisionFacts) routineStrategies(plans []forecast, candidates []sim.Candidate) []forecast {
	capabilities := p.Capabilities()
	for _, kind := range slices.Sorted(maps.Keys(p.Catalog.ActionSpecs)) {
		spec := p.Catalog.ActionSpecs[kind]
		if !capabilities[kind] || spec.TargetKind != "self" || len(spec.SlotGains) == 0 || spec.RequiredVenueFeature == "" {
			continue
		}
		x := sim.Action{Kind: kind}
		if p.CheckAction(x).Allowed {
			continue
		}
		for _, district := range p.Map.Districts {
			if p.Map.DistrictHasFeature(district.ID, spec.RequiredVenueFeature) {
				plans = p.routineAt(plans, candidates, x, district.ID, "")
			}
		}
		for _, venue := range p.Map.Venues {
			if !p.Map.DistrictHasFeature(venue.DistrictID, spec.RequiredVenueFeature) && sim.ActorHasTag(venue.Features, spec.RequiredVenueFeature) {
				plans = p.routineAt(plans, candidates, x, venue.DistrictID, venue.ID)
			}
		}
	}
	return plans
}

func (p decisionFacts) routineAt(plans []forecast, candidates []sim.Candidate, action sim.Action, district, venue string) []forecast {
	route := recipeStrategy{facts: p, plan: forecast{probability: 1}}
	if !route.moveTo(district, venue, "") || len(route.steps) == 0 {
		return plans
	}
	first, available := findActionCandidate(candidates, route.steps[0])
	if !available || !route.facts.CheckAction(action).Allowed {
		return plans
	}
	forecastAtSite := route.facts.forecast(sim.Candidate{Action: action})
	plan := route.plan
	for _, term := range forecastAtSite.effects {
		term.At += plan.duration
		plan.effects = append(plan.effects, term)
	}
	plan.duration += forecastAtSite.duration
	plan.candidate, plan.goal = first, forecastAtSite.goal
	plan.steps = append(route.steps[1:], action)
	plan.strategy, plan.advance = "routine/"+action.Kind+"/"+district+"/"+venue, true
	return append(plans, plan)
}

func (p decisionFacts) deliveryStrategies(plans []forecast, candidates []sim.Candidate, recipes map[string][]sim.Recipe) []forecast {
	a := p.Self
	for _, agreement := range p.Continuity.Agreements {
		if agreement.Status != "active" || agreement.SupplierID != a.ID || a.Items[agreement.Item] >= agreement.Quantity ||
			p.Tick >= agreement.DueTick || p.ObservedPaymentFailure(agreement.BuyerID, agreement.Price) {
			continue
		}
		buyer, present := p.neighbors[agreement.BuyerID]
		if !present || buyer.Jailed || buyer.Sleeping {
			continue
		}
		for _, recipe := range recipes[agreement.Item] {
			if recipe.Growth != nil {
				continue
			}
			projected := recipeStrategy{facts: p, plan: forecast{probability: 1}}
			projected = projected.branch()
			for projected.facts.Self.Items[agreement.Item] < agreement.Quantity && p.Tick+projected.plan.duration < agreement.DueTick {
				before := projected.facts.Self.Items[agreement.Item]
				if !projected.make(recipe, recipes, map[string]bool{}) || projected.facts.Self.Items[agreement.Item] <= before {
					break
				}
			}
			if projected.facts.Self.Items[agreement.Item] < agreement.Quantity ||
				!projected.moveTo(a.Location, a.VenueID, a.InsideHouse) || p.Tick+projected.plan.duration > agreement.DueTick {
				continue
			}
			first, exists := findActionCandidate(candidates, projected.steps[0])
			if !exists {
				continue
			}
			delivery := sim.Action{Kind: "fulfill_agreement", TargetID: agreement.BuyerID, AgreementID: agreement.ID}
			if !projected.spendActionEnergy(delivery) {
				continue
			}
			plan := projected.plan
			id := strconv.FormatInt(agreement.ID, 10)
			plan.candidate, plan.advance = first, true
			plan.goal = sim.NeedKey{Measure: "obligation", Subject: id}
			plan.strategy = "deliver/" + id + "/" + recipe.ID
			plan.steps = append(projected.steps[1:], delivery)
			p.exchange(&plan, true, agreement.Item, "coins", agreement.Quantity, agreement.Price, plan.duration)
			plan.change("obligation", id, plan.duration, 1)
			plan.duration++
			plans = append(plans, plan)
		}
	}
	return plans
}

func (s *recipeStrategy) qualify(recipe sim.Recipe) bool {
	if canPractice(s.facts.Catalog, s.facts.Self, recipe) {
		return true
	}
	profession, known := s.facts.Catalog.ProfessionByID(recipe.Profession)
	if !known || profession.ParentProfession != "" {
		return false
	}
	direct := s.branch()
	if direct.learn(profession) && canPractice(direct.facts.Catalog, direct.facts.Self, recipe) {
		*s = direct
		return true
	}
	var best *recipeStrategy
	for _, entry := range s.facts.Catalog.Professions {
		if entry.ID == profession.ID || len(entry.Grants) == 0 || entry.ParentProfession != "" {
			continue
		}
		through := s.branch()
		if through.learn(entry) && through.learn(profession) &&
			canPractice(through.facts.Catalog, through.facts.Self, recipe) &&
			(best == nil || through.plan.duration < best.plan.duration) {
			best = &through
		}
	}
	if best == nil {
		return false
	}
	*s = *best
	return true
}

func (s *recipeStrategy) learn(profession sim.Profession) bool {
	p, a := s.facts, &s.facts.Self
	for _, guild := range p.Factions {
		if guild.ID != profession.Guild {
			continue
		}
		joined := guild.Joined || sim.ActorHasTag(a.Guilds, guild.ID)
		if !joined && guild.JoinBlockReason != "" {
			return false
		}
		if !s.moveTo(guild.District, p.Map.DefaultVenueForDistrict(guild.District), "") {
			return false
		}
		if !joined {
			join := sim.Action{Kind: "join_faction", FactionID: guild.ID}
			if !s.spendActionEnergy(join) {
				return false
			}
			s.steps = append(s.steps, join)
			s.elapseAwake(1)
			a.Guilds = append(slices.Clone(a.Guilds), guild.ID)
			levels := make(map[string]int, len(a.GuildLevels)+1)
			maps.Copy(levels, a.GuildLevels)
			levels[guild.ID] = guild.Level
			a.GuildLevels = levels
		}
		if allowed, _ := p.Catalog.CanLearnProfession(*a, profession); !allowed {
			return false
		}
		learn := sim.Action{Kind: "join_profession", ProfessionID: profession.ID}
		if !s.spendActionEnergy(learn) {
			return false
		}
		s.steps = append(s.steps, learn)
		s.elapseAwake(1)
		levels := make(map[string]int, len(a.Professions)+1)
		maps.Copy(levels, a.Professions)
		levels[profession.ID] = 1
		a.Professions = levels
		a.Tags = append(slices.Clone(a.Tags), profession.Grants...)
		return true
	}
	return false
}

func findActionCandidate(candidates []sim.Candidate, action sim.Action) (sim.Candidate, bool) {
	for _, candidate := range candidates {
		if actionMatchesDesired(candidate.Action, action) {
			return candidate, true
		}
	}
	return sim.Candidate{}, false
}

func (s *recipeStrategy) make(recipe sim.Recipe, supplies map[string][]sim.Recipe, visiting map[string]bool) bool {
	if visiting[recipe.ID] {
		return false
	}
	if blocked, _ := sim.IsRecipeBlockedByWorldLaw(s.facts.WorldLaws, recipe); blocked {
		return false
	}
	visiting[recipe.ID] = true
	defer delete(visiting, recipe.ID)
	req := recipe.StartRequirements()
	for _, material := range sortedIntMapKeys(req) {
		for s.facts.Self.Items[material] < req[material] {
			have := s.facts.Self.Items[material]
			var best *recipeStrategy
			purchase := s.branch()
			if purchase.acquire(material) {
				best = &purchase
			}
			for _, source := range supplies[material] {
				if source.Growth != nil || visiting[source.ID] {
					continue
				}
				option := s.branch()
				if option.make(source, supplies, visiting) && option.facts.Self.Items[material] > have &&
					(best == nil || option.plan.duration < best.plan.duration) {
					best = &option
				}
			}
			if best == nil {
				return false
			}
			*s = *best
		}
	}
	if !s.reachWork(recipe) {
		return false
	}
	a := &s.facts.Self
	_, energy := recipe.WorkEstimate(s.facts.CraftingSpeedBonusPct)
	if a.Energy < energy || a.Money < recipe.MoneyCost {
		return false
	}
	local := len(s.steps) == 0
	if local && s.facts.RecipeBlockReason(recipe) != "" {
		return false
	}
	s.steps = append(s.steps, sim.Action{Kind: "craft", RecipeID: recipe.ID})
	yield := s.facts.craftYield(recipe, local)
	start := s.plan.duration
	s.facts.craft(&s.plan, recipe, start, local)
	bonus := s.facts.Catalog.ConsumablesYieldBonus(recipe) && a.Items[recipe.BonusInput] > recipe.Inputs[recipe.BonusInput]
	a.Energy -= energy
	a.Money -= recipe.MoneyCost
	for _, item := range sortedIntMapKeys(recipe.Inputs) {
		a.Items[item] -= recipe.Inputs[item]
	}
	if bonus {
		a.Items[recipe.BonusInput]--
	}
	if recipe.Growth == nil {
		a.Items[recipe.OutputItem] += yield
	}
	a.Energy, a.AwakeEnergyRemainder = a.EnergyAfterAwakeTicks(s.plan.duration - start)
	return true
}

func (s *recipeStrategy) acquire(item string) bool {
	p := s.facts
	a := &s.facts.Self
	var best sim.Action
	for _, lot := range p.MarketLots {
		if lot.Kind == "market_buy" && lot.Item == item && lot.Location == a.Location && a.Money >= lot.Price {
			best = lot
			break
		}
	}
	for _, offer := range p.Offers {
		if offer.Item != item || offer.Buyer != a.ID || offer.ResponderID() != a.ID ||
			offer.Expires < p.Tick+s.plan.duration || p.Continuity.DisplayedGoods[offer.ID] < offer.Quantity ||
			offer.District != "" && (offer.District != a.Location || offer.Venue != a.VenueID || offer.House != a.InsideHouse) {
			continue
		}
		if slices.ContainsFunc(s.steps, func(step sim.Action) bool { return step.Kind == "accept" && step.OfferID == offer.ID }) ||
			!p.CheckAction(sim.Action{Kind: "accept", OfferID: offer.ID, TargetID: offer.Seller}).Allowed {
			continue
		}
		cash := int64(a.Money)
		if offer.Currency != "" && offer.Currency != "coins" {
			cash = a.Currencies[offer.Currency]
		}
		if cash < int64(offer.Price) {
			continue
		}
		if best.Kind == "" || float64(offer.Price)/float64(offer.Quantity) < float64(best.Price)/float64(best.Quantity) {
			best = sim.Action{Kind: "accept", OfferID: offer.ID, TargetID: offer.Seller, Item: offer.Item,
				Quantity: offer.Quantity, Price: offer.Price, Currency: offer.Currency}
		}
	}
	if best.Kind == "" {
		return false
	}
	if best.Kind == "market_buy" && a.InsideHouse != "" && !s.moveTo(a.Location, a.VenueID, "") {
		return false
	}
	if best.Kind == "market_buy" && slices.ContainsFunc(s.steps, func(step sim.Action) bool {
		return step.Kind == best.Kind && step.TargetID == best.TargetID && step.Item == best.Item
	}) {
		return false
	}
	step := best
	if step.Kind == "accept" {
		step.Item, step.Currency, step.Quantity, step.Price = "", "", 0, 0
	}
	if !s.spendActionEnergy(step) {
		return false
	}
	s.steps = append(s.steps, step)
	p.exchange(&s.plan, false, best.Item, best.Currency, best.Quantity, best.Price, s.plan.duration)
	s.elapseAwake(1)
	if best.Currency == "" || best.Currency == "coins" {
		a.Money -= best.Price
	} else {
		a.Currencies = maps.Clone(a.Currencies)
		a.Currencies[best.Currency] -= int64(best.Price)
	}
	a.Items[item] += best.Quantity
	return true
}

func (p decisionFacts) marketStrategies(plans []forecast, candidates []sim.Candidate) []forecast {
	a := p.Self
	if a.InsideHouse == "" || len(p.MarketLots) == 0 {
		return plans
	}
	first, ok := findActionCandidate(candidates, sim.Action{Kind: "leave_house"})
	if !ok {
		return plans
	}
	outside := p
	outside.Self.InsideHouse = ""
	outside.Self.Energy, outside.Self.AwakeEnergyRemainder = a.EnergyAfterAwakeTicks(1)
	for _, lot := range p.MarketLots {
		if !outside.CheckAction(lot).Allowed {
			continue
		}
		selling := lot.Kind == "market_sell"
		plan := forecast{candidate: first, duration: 2, probability: 1, steps: []sim.Action{lot}, advance: true}
		plan.change("energy", "", 1, -float64(outside.ActionEnergyCost(lot)))
		outside.exchange(&plan, selling, lot.Item, "coins", lot.Quantity, lot.Price, 1)
		if !selling && a.Items[lot.Item] == 0 {
			outside.planEating(&plan, lot.Item, 2, lot.Quantity)
		}
		plan.strategy = lot.Kind + "/" + lot.TargetID + "/" + lot.Item
		plan.identify(outside.exchangeGoal(lot.Item, selling, lot.Price))
		plans = append(plans, plan)
	}
	return plans
}

func (s *recipeStrategy) reachWork(recipe sim.Recipe) bool {
	p, a := s.facts, &s.facts.Self
	district, venue, building := a.Location, a.VenueID, a.InsideHouse
	switch {
	case recipe.HouseFacility != "":
		site, known := p.KnownWorkshop(recipe.HouseFacility)
		if !known {
			return false
		}
		district, venue, building = site.District, site.VenueID, site.ID
	case recipe.NodeFeature != "":
		building = ""
		if a.VenueID != "" && len(p.Map.Venues) != 0 {
			var ok bool
			venue, ok = p.RecipeResourceVenue(recipe)
			if !ok {
				return false
			}
			site, _ := p.Map.VenueByID(venue)
			district = site.DistrictID
		} else {
			var ok bool
			district, ok = p.RecipeResourceDistrict(recipe)
			if !ok {
				return false
			}
		}
	}
	return s.moveTo(district, venue, building)
}

func (s *recipeStrategy) moveTo(district, venue, building string) bool {
	a := &s.facts.Self
	if a.Location == district && (venue == "" || a.VenueID == venue) && a.InsideHouse == building {
		return true
	}
	ticks, energy, actions, ok := s.facts.routeTo(district, venue, building)
	if !ok || a.Energy < energy {
		return false
	}
	s.plan.change("energy", "", s.plan.duration, -float64(energy))
	s.steps = append(s.steps, actions...)
	if a.Location != district {
		s.facts.DistrictState = nil
	}
	if venue == "" && a.Location != district {
		venue = s.facts.Map.DefaultVenueForDistrict(district)
	}
	if venue != "" {
		a.VenueID = venue
	}
	a.Location, a.InsideHouse = district, building
	a.Energy -= energy
	s.elapseAwake(ticks)
	s.facts.CraftingSpeedBonusPct = 0
	return true
}

func (p decisionFacts) saleStrategies(plans []forecast, candidates []sim.Candidate) []forecast {
	plans = p.tradeTripStrategies(plans, candidates)
	a := p.Self
	if a.Plan == nil || a.Plan.RemainingToSell() <= 0 || p.ActivePlan == nil || p.ActivePlan.Stage != "sell" {
		return plans
	}
	dir := p.ActivePlan
	ticks, energy, steps, ok := p.routeTo(dir.TargetDistrict, dir.TargetVenueID, "")
	if !ok || energy > a.Energy || len(steps) == 0 {
		return plans
	}
	first, exists := findActionCandidate(candidates, steps[0])
	if !exists {
		return plans
	}
	spec := p.Catalog.Items[a.Plan.Item]
	quantity := min(a.Items[a.Plan.Item], a.Plan.RemainingToSell(), max(1, spec.BatchQty))
	saleEnergy := p.ActionEnergyCost(sim.Action{Kind: "market_sell", Item: a.Plan.Item, Quantity: quantity})
	if quantity == 0 || saleEnergy > a.Energy-energy {
		return plans
	}
	sale := forecast{candidate: first, duration: ticks + 1, probability: p.Catalog.EffectivePlanningRules().OfferAcceptancePrior,
		steps: steps[1:], advance: true, strategy: "sell/" + a.Plan.Item, goal: p.productionGoal(a.Plan.Item)}
	sale.change("energy", "", 0, -float64(energy))
	sale.unconditional = slices.Clone(sale.effects)
	sale.change("energy", "", ticks, -float64(saleEnergy))
	p.exchange(&sale, true, a.Plan.Item, "coins", quantity, planBatchPrice(p.Catalog.SalePrice(a, a.Plan.Item), quantity, max(1, spec.BatchQty)), ticks+1)
	p.tradeXP(&sale, a.Plan.Item, quantity, ticks+1)
	return append(plans, sale)
}

// A published market supports an exploratory trip. The estimated exchange is
// conditional, and the actor must find an actual bid or buyer after arrival.
func (p decisionFacts) tradeTripStrategies(plans []forecast, candidates []sim.Candidate) []forecast {
	a := p.Self
	for _, goal := range a.Ambitions {
		if goal.Completed || goal.Kind != "trade_in_district" || a.Location == goal.TargetID ||
			!p.Map.DistrictHasFeature(goal.TargetID, "market") {
			continue
		}
		if _, visited := a.KnownFacts["market_visited:"+goal.TargetID]; visited {
			continue
		}
		ticks, energy, route, ok := p.routeTo(goal.TargetID, "", "")
		if !ok || energy > a.Energy || len(route) == 0 {
			continue
		}
		first, exists := findActionCandidate(candidates, route[0])
		if !exists {
			continue
		}
		for _, item := range sortedIntMapKeys(a.Items) {
			spec := p.Catalog.Items[item]
			if !spec.Tradeable || spec.BatchQty <= 0 || a.Items[item] < spec.BatchQty || spec.BatchPrice <= 0 ||
				!sim.ItemAcceptsCurrency(spec, "coins") || !p.Catalog.TagsProduce(a.IdentityTags(), item) {
				continue
			}
			saleEnergy := p.ActionEnergyCost(sim.Action{Kind: "market_sell", Item: item, Quantity: spec.BatchQty})
			if saleEnergy > a.Energy-energy {
				continue
			}
			toward := forecast{candidate: first, steps: route[1:], duration: ticks + 1,
				probability: p.Catalog.EffectivePlanningRules().OfferAcceptancePrior, advance: true,
				strategy: "visit_market/" + goal.TargetID + "/" + item}
			toward.change("energy", "", 0, -float64(energy))
			toward.unconditional = slices.Clone(toward.effects)
			toward.change("energy", "", ticks, -float64(saleEnergy))
			atMarket := p
			atMarket.Self.Location = goal.TargetID
			atMarket.exchange(&toward, true, item, "coins", spec.BatchQty, spec.BatchPrice, ticks+1)
			atMarket.tradeXP(&toward, item, spec.BatchQty, ticks+1)
			toward.identify(sim.NeedKey{Measure: "market_trade", Subject: goal.TargetID})
			plans = append(plans, toward)
		}
	}
	return plans
}

func (p decisionFacts) waterStrategies(plans []forecast, candidates []sim.Candidate, horizon int64) []forecast {
	if p.Self.Thirst <= 0 || p.HasPotableWater() {
		return plans
	}
	interval := int64(sim.HydrationIntervalTicks)
	nextUses := (p.Tick%interval + horizon) / interval
	needed := float64(p.Self.Thirst) + float64(nextUses)
	var carried float64
	for _, item := range sortedIntMapKeys(p.Self.Items) {
		spec := p.Catalog.Items[item]
		if spec.Potable {
			carried += float64(p.Self.Items[item]) * float64(spec.DrinkingRelief())
		}
	}
	if carried >= needed {
		return plans
	}
	canDrink := p.Capabilities()["drink"]
	for _, source := range p.KnownWaterSources() {
		ticks, energy, steps, reachable := p.routeTo(source, "", "")
		if !reachable || energy > p.Self.Energy {
			continue
		}
		first, exists := findActionCandidate(candidates, steps[0])
		if !exists {
			continue
		}
		plan := forecast{candidate: first, duration: ticks + 1, probability: 1,
			goal: sim.NeedKey{Measure: "hydration"}, strategy: "water/" + source, advance: true, steps: steps[1:]}
		plan.change("energy", "", 0, -float64(energy))
		reliefAt := ticks
		if !canDrink {
			arrival := p.Tick + ticks
			reliefAt = (arrival+interval-1)/interval*interval - p.Tick
			plan.duration = reliefAt + 1
		} else {
			plan.steps = append(plan.steps, sim.Action{Kind: "drink"})
		}
		// Reaching the source covers the thirst accrued during travel.
		// The actor can continue to draw from the same source at the body's
		// regular water-use intervals while staying there.
		plan.change("hydration", "", reliefAt, float64(p.Self.Thirst)+float64(reliefAt)/float64(interval))
		for at := reliefAt + interval; at < horizon; at += interval {
			plan.change("hydration", "", at, 1)
		}
		plans = append(plans, plan)
	}
	return plans
}

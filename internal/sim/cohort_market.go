package sim

import (
	"fmt"
	"math"
)

func (w *World) cohortMarketEnergy(c *PopulationCohort) (population int, balance int64) {
	population = w.anonymousCohort(*c).Population
	return population, w.anonymousCohortEnergy(c, population)
}

// Cohort market lots are standing, public terms. Seeing a lot does not reserve
// it. Settlement checks the same terms and available lot again.
func (w *World) cohortMarketLot(c *PopulationCohort, item, kind string) (Action, bool) {
	spec := w.Catalog.Items[item]
	if c == nil || c.Population <= 0 || !spec.Tradeable || spec.BatchQty <= 0 ||
		!ItemAcceptsCurrency(spec, "coins") {
		return Action{}, false
	}
	price := w.effectiveCohortTradeUnitBatchPrice(c.District, item, spec.BatchPrice)
	switch kind {
	case "market_buy":
		stock := c.ItemPool[item]
		if spec.Edible {
			stock = w.cohortFoodSurplus(*c, item)
		} else if !w.Catalog.TagsProduce(cohortIdentityTags(w.Catalog, c.Profession), item) {
			profession, _ := w.Catalog.ProfessionByID(c.Profession)
			if !profession.TradeMargin {
				return Action{}, false
			}
		} else {
			stock -= int64(c.Population) / 2
		}
		if stock < int64(spec.BatchQty) {
			return Action{}, false
		}
	case "market_sell":
		tags := cohortIdentityTags(w.Catalog, c.Profession)
		profession, _ := w.Catalog.ProfessionByID(c.Profession)
		if w.Catalog.TagsProduce(tags, item) || (!w.Catalog.TagsConsume(tags, item) && !profession.TradeMargin) ||
			c.MoneyPool < int64(price) || c.ItemPool[item] >= int64(c.Population)*4 {
			return Action{}, false
		}
	}
	if _, energy := w.cohortMarketEnergy(c); energy < int64(w.Catalog.tradeEnergy(item, spec.BatchQty)) {
		return Action{}, false
	}
	return Action{Kind: kind, TargetID: c.ID, Location: c.District, Item: item, Quantity: spec.BatchQty, Price: price}, true
}

func (w *World) projectCohortMarket(p *Perception) {
	if len(p.LocalCohorts) == 0 {
		return
	}
	a := p.Self
	tags := a.IdentityTags()
	inputs := make(map[string]bool)
	outputs := make(map[string]bool)
	for _, recipe := range w.Catalog.Recipes {
		if ok, _ := w.Catalog.ProfessionGating(a, recipe); ok {
			outputs[recipe.OutputItem] = true
			for item := range recipe.Inputs {
				inputs[item] = true
			}
		}
	}
	for _, item := range sortedProductionKeys(w.Catalog.Items) {
		spec := w.Catalog.Items[item]
		if !spec.Tradeable || w.IsItemEmbargoedAt(a.Location, a.VenueID, item) {
			continue
		}
		buy := inputs[item] || w.Catalog.ActorAcceptsFood(a, spec) || w.Catalog.DesiredStock(tags, item) > 0 ||
			spec.MedicinalHeal > 0 && (a.Health < MaxHealth || a.DiseaseID != "")
		// A standing bid is visible even before a producer makes the goods.
		// Taking the bid still requires a full parcel at settlement.
		sell := outputs[item] || a.Items[item] >= spec.BatchQty &&
			(spec.Edible || w.Catalog.TagsProduce(tags, item) || w.Catalog.DesiredStock(tags, item) > 0)
		if !buy && !sell {
			continue
		}
		mayBuy := buy && w.buyerItemBlockReason(a, item, "coins") == ""
		for _, local := range p.LocalCohorts {
			c := w.findCohort(local.ID)
			if mayBuy {
				if lot, ok := w.cohortMarketLot(c, item, "market_buy"); ok {
					p.MarketLots = append(p.MarketLots, lot)
				}
			}
			if sell {
				if lot, ok := w.cohortMarketLot(c, item, "market_sell"); ok {
					p.MarketLots = append(p.MarketLots, lot)
				}
			}
		}
	}
}

func sameMarketLot(a, b Action) bool {
	return a.Kind == b.Kind && a.TargetID == b.TargetID && a.Location == b.Location &&
		a.Item == b.Item && a.Quantity == b.Quantity && a.Price == b.Price &&
		(a.Currency == "" || a.Currency == "coins")
}

func handleCohortMarketAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	c := w.findCohort(x.TargetID)
	seller, buyer := a.ID, c.ID
	quantity, cash := -x.Quantity, x.Price
	if x.Kind == "market_buy" {
		seller, buyer = c.ID, a.ID
		quantity, cash = x.Quantity, -x.Price
		if a.Items[x.Item] > math.MaxInt-quantity || c.MoneyPool > math.MaxInt64-int64(x.Price) {
			return actionFail(tx, a, x, "market purchase exceeds a holding range")
		}
	} else if a.Money > math.MaxInt-cash || c.ItemPool[x.Item] > math.MaxInt64-int64(x.Quantity) {
		return actionFail(tx, a, x, "market sale exceeds a holding range")
	}
	undoItems, previousMoney := captureMapInt64(&c.ItemPool, x.Item), c.MoneyPool
	undoEnergyRemainder, previousEnergy := captureMapInt64(&w.Production.EnergyRemainder, c.ID), c.AvgEnergy
	tx.pushUndo(func() {
		undoItems()
		undoEnergyRemainder()
		c.MoneyPool, c.AvgEnergy = previousMoney, previousEnergy
	})
	population, energy := w.cohortMarketEnergy(c)
	effort := w.Catalog.tradeEnergy(x.Item, x.Quantity)
	w.setAnonymousCohortEnergy(c, population, energy-int64(effort))
	tx.moveItem(a, x.Item, quantity, c.ID, x.Kind)
	tx.moveCurrency(a, "coins", int64(cash), c.ID, x.Kind)
	w.adjustCohortItem(c, x.Item, -int64(quantity), false)
	w.addCohortCurrency(c, "coins", -int64(cash))
	events := []Event{tx.record(a, x, "trade_completed", map[string]any{
		"text":      fmt.Sprintf("%s and %s exchanged %d %s for %d coins", a.Name, c.Name, x.Quantity, x.Item, x.Price),
		"seller_id": seller, "buyer_id": buyer, "cohort_id": c.ID, "item": x.Item, "quantity": x.Quantity, "currency": "coins",
		"cohort_energy_spent": effort,
	})}
	if x.Kind == "market_sell" {
		xp, influence := w.Catalog.TradeExperience(x.Item, x.Quantity)
		events = append(events, w.grantProfessionXP(tx, a, x, "peddler", xp, influence)...)
	}
	return events
}

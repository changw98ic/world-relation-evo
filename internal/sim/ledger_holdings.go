package sim

import (
	"fmt"
	"sort"
)

// VerifyHoldingBounds checks individual holdings before totals are compared.
// A negative stock cannot be cancelled out by another owner's positive stock.
func (w *World) VerifyHoldingBounds() error {
	checkAmount := func(owner, asset string, amount int64) error {
		if amount < 0 {
			return fmt.Errorf("negative holding: %s %s=%d", owner, asset, amount)
		}
		return nil
	}
	checkBag := func(owner string, bag map[string]int64) error {
		keys := make([]string, 0, len(bag))
		for id := range bag {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			if err := checkAmount(owner, id, bag[id]); err != nil {
				return err
			}
		}
		return nil
	}
	checkActor := func(a Actor) error {
		owner := "actor:" + a.ID
		if err := checkAmount(owner, "coins", int64(a.Money)); err != nil {
			return err
		}
		items := make(map[string]int64, len(a.Items))
		for id, quantity := range a.Items {
			items[id] = int64(quantity)
		}
		if err := checkBag(owner, items); err != nil {
			return err
		}
		return checkBag(owner, a.Currencies)
	}
	if err := checkAmount("public treasury", "coins", w.PublicTreasury); err != nil {
		return err
	}
	for _, a := range w.Actors {
		if err := checkActor(a); err != nil {
			return err
		}
	}
	for _, c := range w.Cohorts {
		if err := checkAmount("cohort:"+c.ID, "population", int64(c.Population)); err != nil {
			return err
		}
		if err := checkAmount("cohort:"+c.ID, "wounded", int64(c.Wounded)); err != nil {
			return err
		}
		if err := checkAmount("cohort:"+c.ID, "sick", int64(c.Sick)); err != nil {
			return err
		}
		if err := checkAmount("cohort:"+c.ID, "jailed", int64(c.Jailed)); err != nil {
			return err
		}
		if err := c.ValidateHungerBalance(); err != nil {
			return err
		}
		owner := "cohort:" + c.ID
		if err := checkAmount(owner, "coins", c.MoneyPool); err != nil {
			return err
		}
		if err := checkBag(owner, c.ItemPool); err != nil {
			return err
		}
		if err := checkBag(owner, c.Currencies); err != nil {
			return err
		}
		if len(c.Residents) > 0 {
			anon := w.anonymousCohort(c)
			var bandPop int
			var bandHunger int64
			for idx, b := range c.Residents {
				if (c.ID != "" && b.Origin == "") || b.First < 1 || b.Count <= 0 {
					return fmt.Errorf("cohort %s resident band[%d] has invalid identity or count", c.ID, idx)
				}
				if b.Hunger < 0 || b.Health < 0 {
					return fmt.Errorf("cohort %s resident band[%d] has negative hunger or health", c.ID, idx)
				}
				for itemID, qty := range b.Items {
					if qty < 0 {
						return fmt.Errorf("negative holding: cohort:%s/resident_band[%d] %s=%d", c.ID, idx, itemID, qty)
					}
				}
				bandPop += b.Count
				bandHunger += int64(b.Count) * int64(b.Hunger)
			}
			if bandPop != anon.Population {
				return fmt.Errorf("cohort %s resident bands population %d != anonymous population %d", c.ID, bandPop, anon.Population)
			}
			if bandHunger != c.hungerUnits(anon.Population) {
				return fmt.Errorf("cohort %s resident bands hunger %d != cohort hunger %d", c.ID, bandHunger, c.hungerUnits(anon.Population))
			}
		}
		for _, member := range c.Members {
			if err := checkActor(member); err != nil {
				return err
			}
		}
	}
	if w.Polities != nil {
		for _, p := range w.Polities.Nodes {
			if err := checkAmount("polity:"+p.ID, "coins", p.Treasury); err != nil {
				return err
			}
			if err := checkBag("polity:"+p.ID, p.Currencies); err != nil {
				return err
			}
		}
	}
	if w.MarketCredit != nil {
		owners := make([]string, 0, len(w.MarketCredit.HoardedStock))
		for id := range w.MarketCredit.HoardedStock {
			owners = append(owners, id)
		}
		sort.Strings(owners)
		for _, owner := range owners {
			if err := checkBag("hoard:"+owner, w.MarketCredit.HoardedStock[owner]); err != nil {
				return err
			}
		}
	}
	for _, shipment := range w.Shipments {
		if err := checkAmount(fmt.Sprintf("shipment:%d", shipment.ID), shipment.Item, shipment.Quantity); err != nil {
			return err
		}
	}
	signed := make(map[int64]GrowthCareAgreement, len(w.Production.CareAgreements))
	for _, terms := range w.Production.CareAgreements {
		if err := terms.validateSigned(); err != nil {
			return err
		}
		if _, exists := signed[terms.GrowthID]; exists {
			return fmt.Errorf("multiple signed care agreements for growth %d", terms.GrowthID)
		}
		signed[terms.GrowthID] = terms
	}
	for _, growth := range w.Growth {
		if terms, ok := signed[growth.ID]; ok && (terms.OwnerID != growth.OwnerID || terms.OwnerKind != holderKind(growth.Cohort)) {
			return fmt.Errorf("care agreement owner differs from owner of growth %d", growth.ID)
		}
		owner := fmt.Sprintf("growth:%d", growth.ID)
		if err := checkAmount(owner, growth.Item, growth.Quantity); err != nil {
			return err
		}
		if err := checkBag(owner, growth.Parents); err != nil {
			return err
		}
	}
	districts := make([]string, 0, len(w.DistrictStates))
	for id := range w.DistrictStates {
		districts = append(districts, id)
	}
	sort.Strings(districts)
	for _, id := range districts {
		ds := w.DistrictStates[id]
		if err := checkBag("ecology:"+id, ds.Resources); err != nil {
			return err
		}
		if err := checkAmount("ecology:"+id, "nightsoil", ds.NightsoilPool); err != nil {
			return err
		}
	}
	if w.FlowGrid != nil {
		for _, node := range w.FlowGrid.Nodes {
			if err := checkAmount("energy:"+node.ID, "fuel", node.FuelStock); err != nil {
				return err
			}
			if err := checkAmount("energy:"+node.ID, "stored energy", node.StoredEnergy); err != nil {
				return err
			}
		}
	}
	return nil
}

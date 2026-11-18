package sim

import "fmt"

// ValidateProductionRules checks external quantities used as divisors, time
// increments and material budgets. Zero capacity is an explicit closed facility;
// zero care labor or failure duration is permitted for unattended/nonliving
// maturation. Living inventory always declares its own survival costs.
func (c Catalog) ValidateProductionRules() error {
	p := c.Production
	if p.FieldAreaPerBatchM2 < 0 || p.FieldAreaPerFarmerM2 < 0 {
		return fmt.Errorf("production field areas cannot be negative")
	}
	if p.FieldAreaPerFarmerM2 > 0 && p.FieldSource == "" {
		return fmt.Errorf("initial field allocation requires a scenario source")
	}
	if len(p.FacilityCapacity) > 0 && p.FacilitySource == "" {
		return fmt.Errorf("facility capacity requires a scenario source")
	}
	for _, id := range sortedProductionKeys(p.FacilityCapacity) {
		if id == "" || p.FacilityCapacity[id] < 0 {
			return fmt.Errorf("invalid production facility capacity %q", id)
		}
	}
	for _, id := range sortedProductionKeys(c.Items) {
		item := c.Items[id]
		if item.MassUnits < 0 || item.NutritionUnits < 0 || item.HungerRelief < 0 || item.Edible && int64(item.HungerRelief) > item.NutritionUnits {
			return fmt.Errorf("item %s has invalid material or nutrition units", id)
		}
		living := item.Living
		if living == nil {
			continue
		}
		if living.IntervalTicks <= 0 || living.DeathAfterTicks <= 0 || living.Source == "" {
			return fmt.Errorf("living item %s requires positive feeding/survival periods and a source", id)
		}
		if len(living.Supplies) == 0 && living.ResourcePool == "" {
			return fmt.Errorf("living item %s requires a declared maintenance supply", id)
		}
		for _, supply := range sortedProductionKeys(living.Supplies) {
			if _, ok := c.Items[supply]; !ok || living.Supplies[supply] <= 0 {
				return fmt.Errorf("living item %s has invalid supply %s", id, supply)
			}
		}
		if living.ResourcePool != "" {
			if _, ok := c.Resources[living.ResourcePool]; !ok || living.ResourceDraw <= 0 {
				return fmt.Errorf("living item %s has invalid resource supply %s", id, living.ResourcePool)
			}
		} else if living.ResourceDraw != 0 {
			return fmt.Errorf("living item %s resource draw has no resource pool", id)
		}
	}
	for _, r := range c.Recipes {
		if r.Growth != nil {
			if r.NodeFeature == "farmland" && (p.FieldAreaPerBatchM2 <= 0 || p.FieldSource == "") {
				return fmt.Errorf("recipe %s requires positive field area per batch and a scenario source", r.ID)
			}
		}
		for _, input := range sortedProductionKeys(r.Inputs) {
			if item, ok := c.Items[input]; !ok || item.MassUnits <= 0 {
				return fmt.Errorf("recipe %s input %s requires positive material units", r.ID, input)
			}
		}
	}
	return nil
}

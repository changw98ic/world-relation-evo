package sim

import "fmt"

// ValidatePhysicalRules validates external world definitions before any simulation.
func (c Catalog) ValidatePhysicalRules(grid *FlowGridState) error {
	for _, s := range c.Calendar.Seasons {
		if s.Days <= 0 {
			return fmt.Errorf("season %s needs a positive duration", s.ID)
		}
	}
	if c.Lifecycle.WasteItem != "" {
		if _, ok := c.Items[c.Lifecycle.WasteItem]; !ok || c.Lifecycle.WastePerFoodDivisor <= 0 {
			return fmt.Errorf("waste processing requires a declared item and positive food conversion divisor")
		}
	}
	if c.Lifecycle.CompostRecipeID != "" {
		if _, ok := c.RecipeByID(c.Lifecycle.CompostRecipeID); !ok {
			return fmt.Errorf("unknown compost recipe %s", c.Lifecycle.CompostRecipeID)
		}
	}
	for _, r := range c.Recipes {
		if r.Ticks <= 0 {
			return fmt.Errorf("recipe %s requires a positive work duration", r.ID)
		}
		if r.OutputQty > 0 && len(r.Inputs) == 0 && r.ResourcePool == "" {
			return fmt.Errorf("recipe %s has no material source", r.ID)
		}
		for item, qty := range recipeStartRequirements(r) {
			if _, ok := c.Items[item]; !ok || qty <= 0 {
				return fmt.Errorf("recipe %s invalid material %s", r.ID, item)
			}
		}
		if r.ResourcePool != "" {
			if _, ok := c.Resources[r.ResourcePool]; !ok || r.ResourceDraw <= 0 {
				return fmt.Errorf("recipe %s invalid resource source %s", r.ID, r.ResourcePool)
			}
		}
		if err := validateGrowth(r); err != nil {
			return err
		}
		if r.Growth != nil {
			for item := range r.Growth.Supplies {
				if _, ok := c.Items[item]; !ok {
					return fmt.Errorf("recipe %s unknown growth supply %s", r.ID, item)
				}
			}
			if r.Growth.ResourcePool != "" {
				if _, ok := c.Resources[r.Growth.ResourcePool]; !ok {
					return fmt.Errorf("recipe %s unknown growth resource %s", r.ID, r.Growth.ResourcePool)
				}
			}
		}
	}
	if grid == nil {
		return nil
	}
	nodes := map[string]bool{}
	for _, n := range grid.Nodes {
		if n.ID == "" || nodes[n.ID] {
			return fmt.Errorf("invalid or duplicate energy node %q", n.ID)
		}
		nodes[n.ID] = true
		if n.Generation < 0 || n.Demand < 0 || n.StoredEnergy < 0 || n.FuelStock < 0 || n.EnergyCapacity < 0 || n.StoredEnergy > n.EnergyCapacity {
			return fmt.Errorf("energy node %s has a negative amount", n.ID)
		}
		s := n.Source
		if s == nil {
			if n.FuelStock != 0 {
				return fmt.Errorf("energy node %s holds fuel without a source specification", n.ID)
			}
			continue
		}
		if n.Kind != "generator" || (s.FuelItem == "") == (s.ResourcePool == "") || s.UnitsPerCycle <= 0 || s.EnergyPerUnit <= 0 || s.CycleTicks <= 0 || s.Source == "" {
			return fmt.Errorf("energy node %s requires one source, positive conversion and cycle, and source description", n.ID)
		}
		if s.FuelItem != "" {
			if _, ok := c.Items[s.FuelItem]; !ok {
				return fmt.Errorf("energy node %s unknown fuel %s", n.ID, s.FuelItem)
			}
		}
		if s.ResourcePool != "" {
			if _, ok := c.Resources[s.ResourcePool]; !ok || n.DistrictID == "" {
				return fmt.Errorf("energy node %s unknown local resource source", n.ID)
			}
		}
	}
	for _, e := range grid.Edges {
		if !nodes[e.FromNodeID] || !nodes[e.ToNodeID] || e.Capacity < 0 || e.LossPermille < 0 || e.LossPermille > 1000 {
			return fmt.Errorf("invalid energy edge %s", e.ID)
		}
	}
	return nil
}

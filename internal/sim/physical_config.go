package sim

import "fmt"

// ValidatePhysicalRules validates external world definitions before any simulation.
func (c Catalog) ValidatePhysicalRules(grid *FlowGridState) error {
	if c.HouseWear != nil {
		rule := c.HouseWear
		if rule.ConditionLoss < 0 || rule.IntervalTicks < 0 || rule.ConditionLoss > 0 && rule.IntervalTicks == 0 || rule.Source == "" {
			return fmt.Errorf("house weathering needs a source, nonnegative loss, and a positive interval when it causes loss")
		}
	}
	if err := c.ValidateEcologyRules(); err != nil {
		return err
	}
	if err := c.ValidateProductionRules(); err != nil {
		return err
	}
	for _, s := range c.Calendar.Seasons {
		if s.Days <= 0 {
			return fmt.Errorf("season %s needs a positive duration", s.ID)
		}
	}
	if c.Lifecycle.ChildMaxAge < 0 || c.Lifecycle.ElderMinAge < 0 || c.Lifecycle.MaxExpectancy < 0 {
		return fmt.Errorf("lifecycle age thresholds cannot be negative")
	}
	if c.Lifecycle.ChildMaxAge > 0 && c.Lifecycle.ElderMinAge > 0 && c.Lifecycle.ChildMaxAge >= c.Lifecycle.ElderMinAge {
		return fmt.Errorf("lifecycle child_max_age (%d) must be less than elder_min_age (%d)", c.Lifecycle.ChildMaxAge, c.Lifecycle.ElderMinAge)
	}
	if c.Lifecycle.ElderMinAge > 0 && c.Lifecycle.MaxExpectancy > 0 && c.Lifecycle.ElderMinAge >= c.Lifecycle.MaxExpectancy {
		return fmt.Errorf("lifecycle elder_min_age (%d) must be less than max_expectancy (%d)", c.Lifecycle.ElderMinAge, c.Lifecycle.MaxExpectancy)
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
		if err := c.ValidateProductionConversion(r); err != nil {
			return err
		}
		if r.Ticks <= 0 {
			return fmt.Errorf("recipe %s requires a positive work duration", r.ID)
		}
		if r.OutputQty > 0 {
			if _, ok := c.Items[r.OutputItem]; !ok {
				return fmt.Errorf("recipe %s unknown output item %s", r.ID, r.OutputItem)
			}
			if len(r.Inputs) == 0 && r.ResourcePool == "" {
				return fmt.Errorf("recipe %s has no material source", r.ID)
			}
		}
		if r.BonusInput != "" {
			if _, ok := c.Items[r.BonusInput]; !ok {
				return fmt.Errorf("recipe %s unknown bonus input %s", r.ID, r.BonusInput)
			}
		}
		if r.Profession != "" && len(c.Professions) > 0 {
			if _, ok := c.ProfessionByID(r.Profession); !ok {
				return fmt.Errorf("recipe %s unknown profession %s", r.ID, r.Profession)
			}
		}
		for item, qty := range r.StartRequirements() {
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
	for id, tech := range c.Techs {
		for field, qty := range tech.AmbientFieldInputsPerTick {
			if field == "" || qty <= 0 {
				return fmt.Errorf("tech %s requires a named ambient operating input with positive quantity", id)
			}
		}
	}
	for k, spec := range c.ActionSpecs {
		for item, qty := range spec.ItemCosts {
			if _, ok := c.Items[item]; !ok || qty <= 0 {
				return fmt.Errorf("action_spec %s invalid item cost %s", k, item)
			}
		}
		for item, qty := range spec.ItemGains {
			if _, ok := c.Items[item]; !ok || qty <= 0 {
				return fmt.Errorf("action_spec %s invalid item gain %s", k, item)
			}
		}
		if len(c.StateSlots) > 0 {
			for slot, qty := range spec.SlotCosts {
				if _, ok := c.StateSlots[slot]; !ok || qty <= 0 {
					return fmt.Errorf("action_spec %s invalid slot cost %s", k, slot)
				}
			}
			for slot, qty := range spec.SlotGains {
				if _, ok := c.StateSlots[slot]; !ok || qty <= 0 {
					return fmt.Errorf("action_spec %s invalid slot gain %s", k, slot)
				}
			}
		}
		if len(c.Techs) > 0 {
			for _, tech := range spec.RequiredTechs {
				if _, ok := c.Techs[tech]; !ok {
					return fmt.Errorf("action_spec %s unknown required tech %s", k, tech)
				}
			}
			for _, tech := range spec.UnlockTechs {
				if _, ok := c.Techs[tech]; !ok {
					return fmt.Errorf("action_spec %s unknown unlock tech %s", k, tech)
				}
			}
		}
		if len(c.Forms) > 0 {
			for _, formID := range spec.RequiredFormIDs {
				if _, ok := c.Forms[formID]; !ok {
					return fmt.Errorf("action_spec %s unknown required form %s", k, formID)
				}
			}
		}
		if spec.ResourcePool != "" {
			if _, ok := c.Resources[spec.ResourcePool]; !ok {
				return fmt.Errorf("action_spec %s unknown resource pool %s", k, spec.ResourcePool)
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

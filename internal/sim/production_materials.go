package sim

import "fmt"

// MaterialConversion accounts for input/output in catalog material units, not
// kilograms. Missing output mass is a declared process loss. External budgets
// must name an actual finite extraction or a configured biological process.
// They are never multiplied by skill, guild level, tool ownership or technology.
type MaterialConversion struct {
	ExternalMassUnits      int64  `json:"external_mass_units,omitempty"`
	ExternalNutritionUnits int64  `json:"external_nutrition_units,omitempty"`
	Source                 string `json:"source"`
}

func withProductionMaterials(c Catalog) Catalog {
	// These are explicit scenario units. One ordinary ration carries one food
	// unit. The larger meat ration contains two; an adult cattle unit contains
	// six. Crafting/refining can discard matter, but cannot amplify these budgets.
	for id, item := range c.Items {
		item.MassUnits = 1
		if item.Edible {
			item.MassUnits = max(item.MassUnits, item.NutritionUnits)
		}
		if id == "calf" {
			item.MassUnits = 2
			item.NutritionUnits = 2
		}
		if id == "livestock" {
			item.MassUnits = 6
			item.NutritionUnits = 6
		}
		c.Items[id] = item
	}
	for i := range c.Recipes {
		r := &c.Recipes[i]
		budget := MaterialConversion{Source: "scenario conversion: declared recipe inputs -> OutputQty units; unused matter is process loss"}
		var mass, nutrition int64
		for item, n := range r.Inputs {
			mass += int64(n) * c.Items[item].MassUnits
			nutrition += int64(n) * c.Items[item].NutritionUnits
		}
		output := int64(r.OutputQty)
		if c.ConsumablesYieldBonus(*r) {
			output += int64(r.BonusOutput)
			mass += c.Items[r.BonusInput].MassUnits
			nutrition += c.Items[r.BonusInput].NutritionUnits
		}
		if r.Growth != nil || r.ResourcePool != "" {
			budget.ExternalMassUnits = max(int64(0), output*c.Items[r.OutputItem].MassUnits-mass)
			budget.ExternalNutritionUnits = max(int64(0), output*c.Items[r.OutputItem].NutritionUnits-nutrition)
			if r.Growth != nil {
				budget.Source = "configured biological assimilation from growth supplies, environment and retained parents; " + r.Growth.Source
			} else {
				budget.Source = fmt.Sprintf("finite %s extraction: %d resource units fund at most %d output units", r.ResourcePool, r.ResourceDraw, r.OutputQty)
			}
		}
		r.Conversion = &budget
	}
	return c
}

func (c Catalog) ValidateProductionConversion(r Recipe) error {
	if r.Conversion == nil {
		return fmt.Errorf("recipe %s requires declared material conversion", r.ID)
	}
	b := r.Conversion
	if b.Source == "" || b.ExternalMassUnits < 0 || b.ExternalNutritionUnits < 0 {
		return fmt.Errorf("recipe %s invalid material conversion", r.ID)
	}
	if (b.ExternalMassUnits > 0 || b.ExternalNutritionUnits > 0) && r.Growth == nil && r.ResourcePool == "" {
		return fmt.Errorf("recipe %s external material requires a declared physical source", r.ID)
	}
	var mass, nutrition int64
	for item, n := range r.Inputs {
		mass += int64(n) * c.Items[item].MassUnits
		if err := addNutrition(&nutrition, int64(n), c.Items[item].NutritionUnits); err != nil {
			return fmt.Errorf("recipe %s input %s: %w", r.ID, item, err)
		}
	}
	maximum := int64(r.OutputQty)
	if c.ConsumablesYieldBonus(r) {
		maximum += int64(r.BonusOutput)
		mass += c.Items[r.BonusInput].MassUnits
		if err := addNutrition(&nutrition, 1, c.Items[r.BonusInput].NutritionUnits); err != nil {
			return fmt.Errorf("recipe %s bonus input: %w", r.ID, err)
		}
	}
	output := c.Items[r.OutputItem]
	if output.MassUnits <= 0 {
		return fmt.Errorf("recipe %s output requires declared mass units", r.ID)
	}
	if maximum*output.MassUnits > mass+b.ExternalMassUnits {
		return fmt.Errorf("recipe %s output exceeds declared material budget", r.ID)
	}
	if err := addNutrition(&nutrition, 1, b.ExternalNutritionUnits); err != nil {
		return fmt.Errorf("recipe %s external nutrition: %w", r.ID, err)
	}
	var outputNutrition int64
	if err := addNutrition(&outputNutrition, maximum, output.NutritionUnits); err != nil {
		return fmt.Errorf("recipe %s output: %w", r.ID, err)
	}
	if outputNutrition > nutrition {
		return fmt.Errorf("recipe %s output exceeds declared nutrition budget", r.ID)
	}
	return nil
}

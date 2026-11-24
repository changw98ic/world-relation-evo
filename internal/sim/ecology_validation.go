package sim

import "fmt"

// ValidateEcologyRules validates external ecology definitions before stepping.
// Recruitment and drops must identify finite sources; natural ceremonies may
// affect social state but cannot change physical hazards.
func (c Catalog) ValidateEcologyRules() error {
	for id, resource := range c.Resources {
		if resource.RegenPermillePerDay < 0 || resource.MaturityTicks < 0 {
			return fmt.Errorf("resource %s has negative regeneration or maturity time", id)
		}
		if resource.Biological {
			if _, ok := c.Resources[resource.ReproductionSource]; !ok {
				return fmt.Errorf("biological resource %s needs a declared reproduction source", id)
			}
			if resource.RegenPermillePerDay > 0 && resource.MaturityTicks <= 0 {
				return fmt.Errorf("biological resource %s needs a positive maturity time", id)
			}
		}
	}
	for _, encounter := range c.Encounters {
		if encounter.RareDropPermille < 0 || encounter.RareDropPermille > 1000 {
			return fmt.Errorf("encounter %s drop probability must be within 0..1000", encounter.ID)
		}
		if encounter.RareDropItem != "" {
			if _, ok := c.Items[encounter.RareDropItem]; !ok {
				return fmt.Errorf("encounter %s has unknown drop item %s", encounter.ID, encounter.RareDropItem)
			}
			if _, ok := c.Resources[encounter.SourceResource]; !ok || encounter.ResourcePerKill <= 0 {
				return fmt.Errorf("encounter %s needs a finite source and positive draw per kill", encounter.ID)
			}
		}
	}
	for _, ritual := range c.Rituals {
		if !ritual.Supernatural && (ritual.DisasterReliefWeightPct != 0 || ritual.ThreatAppeaseAmount != 0) {
			return fmt.Errorf("ritual %s needs an explicit supernatural ability for physical hazard effects", ritual.ID)
		}
	}
	return nil
}

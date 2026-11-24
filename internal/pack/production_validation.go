package pack

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func (p *WorldPack) validateProductionAssets(actors, cohorts, districts map[string]bool, venues map[string]sim.Venue) error {
	if p.Production == nil {
		return nil
	}
	fields := make(map[string]bool, len(p.Production.Fields))
	holderExists := func(kind, id string) bool {
		switch kind {
		case "actor":
			return actors[id]
		case "cohort":
			return cohorts[id]
		default:
			return false
		}
	}
	for _, field := range p.Production.Fields {
		if field.ID == "" || fields[field.ID] {
			return fmt.Errorf("invalid or duplicate field id %q", field.ID)
		}
		fields[field.ID] = true
		if field.AreaM2 <= 0 || !districts[field.District] {
			return fmt.Errorf("field %q requires positive area and a declared district", field.ID)
		}
		if field.VenueID != "" {
			venue, ok := venues[field.VenueID]
			if !ok || venue.DistrictID != field.District {
				return fmt.Errorf("field %q venue does not belong to its district", field.ID)
			}
		}
		if !holderExists(field.OwnerKind, field.OwnerID) || !holderExists(field.UserKind, field.UserID) {
			return fmt.Errorf("field %q references an unknown owner or authorized user", field.ID)
		}
	}
	return nil
}

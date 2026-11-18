package sim

import (
	"fmt"
	"math"
)

// Nutrition shares the ledger's integer representation. Reject an unrepresentable
// external balance rather than wrapping it into a shortage or silently discarding it.
func addNutrition(total *int64, quantity, units int64) error {
	if quantity < 0 || units < 0 || units != 0 && quantity > math.MaxInt64/units {
		return fmt.Errorf("nutrition quantity %d × %d cannot be represented by int64", quantity, units)
	}
	product := quantity * units
	if *total > math.MaxInt64-product {
		return fmt.Errorf("nutrition total %d + %d cannot be represented by int64", *total, product)
	}
	*total += product
	return nil
}

func (c Catalog) validateHeldNutrition(items map[string]int64, reserve int64) error {
	if reserve < 0 {
		return fmt.Errorf("negative nutrition reserve: %d", reserve)
	}
	total := reserve
	for _, id := range sortedProductionKeys(items) {
		if c.Items[id].Edible {
			if err := addNutrition(&total, items[id], c.Items[id].NutritionUnits); err != nil {
				return fmt.Errorf("item %s: %w", id, err)
			}
		}
	}
	return nil
}

package pack

import (
	"path/filepath"
	"testing"
)

func TestShippedPlantsAndGrowingAnimalsDeclareCareFailurePeriods(t *testing.T) {
	for _, name := range []string{"mortal_watershed", "xianxia_sect", "wasteland_sector"} {
		p, err := LoadFile(filepath.Join("..", "..", "packs", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, recipe := range p.Catalog.Recipes {
			if recipe.Growth != nil && (recipe.NodeFeature == "farmland" || p.Catalog.Items[recipe.OutputItem].Living != nil) && recipe.Growth.FailureAfterTicks <= 0 {
				t.Errorf("%s/%s can wait forever without required care", name, recipe.ID)
			}
		}
	}
}

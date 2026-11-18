package pack

import (
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestBuiltinPacksValidateAndInstantiateWithoutContradictions(t *testing.T) {
	packs := []string{
		"mortal_watershed.json",
		"xianxia_sect.json",
		"wasteland_sector.json",
	}
	for _, name := range packs {
		t.Run(name, func(t *testing.T) {
			p, err := LoadFile(filepath.Join("..", "..", "packs", name))
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", name, err)
			}
			w, err := p.Instantiate("")
			if err != nil {
				t.Fatalf("Instantiate(%s): %v", name, err)
			}
			if err := w.VerifyHoldingBounds(); err != nil {
				t.Fatalf("VerifyHoldingBounds(%s): %v", name, err)
			}
			if err := w.VerifyLedgerConservation(); err != nil {
				t.Fatalf("VerifyLedgerConservation(%s): %v", name, err)
			}
			mapFeatures := map[string]bool{}
			for _, d := range w.Map.Districts {
				for _, f := range d.Features {
					mapFeatures[f] = true
				}
			}
			for _, v := range w.Map.Venues {
				for _, f := range v.Features {
					mapFeatures[f] = true
				}
			}
			for _, r := range w.Catalog.Recipes {
				if r.NodeFeature != "" && !mapFeatures[r.NodeFeature] {
					t.Fatalf("%s recipe %s requires missing map node_feature %q", name, r.ID, r.NodeFeature)
				}
			}
			for _, a := range w.Actors {
				_, maxExp := w.Catalog.Lifecycle.ElderMinAge, w.Catalog.Lifecycle.MaxExpectancy
				if a.FormID != "" {
					if f, ok := w.Catalog.Forms[a.FormID]; ok && f.LifeExpectancyYears > 0 {
						maxExp = f.LifeExpectancyYears
					}
				}
				if maxExp > 0 && a.AgeYears > maxExp {
					t.Fatalf("%s actor %s age %d exceeds max expectancy %d", name, a.ID, a.AgeYears, maxExp)
				}
			}
		})
	}
}

func TestCustomPackRejectsMissingRecipeNodeFeature(t *testing.T) {
	p := rightsPack()
	p.Catalog.Recipes = []sim.Recipe{{
		ID:          "bad_recipe",
		NodeFeature: "missing_feature",
		Ticks:       1,
		OutputItem:  "food",
		OutputQty:   1,
	}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected WorldPack.Validate to reject recipe with missing map node_feature")
	}
}

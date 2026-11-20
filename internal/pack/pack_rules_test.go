package pack

import (
	"context"
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

func TestNonHistoricalPacksContainNoAncientDynastyLeaks(t *testing.T) {
	ancientProfessions := map[string]bool{
		"constable": true, "chief_constable": true, "xiucai": true, "juren": true, "jinshi": true,
		"carpenter": true, "bandit": true, "gangster": true, "gang_boss": true, "yamen_runner": true,
	}
	ancientItems := map[string]bool{
		"bounty_writ": true, "nightsoil": true, "plank": true, "choice_grain": true, "sacrificial_offering": true,
	}
	ancientFeatures := map[string]bool{
		"yamen": true, "shrine": true,
	}

	for _, name := range []string{"xianxia_sect.json", "wasteland_sector.json"} {
		t.Run(name, func(t *testing.T) {
			p, err := LoadFile(filepath.Join("..", "..", "packs", name))
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", name, err)
			}
			if p.Environment == nil {
				t.Fatalf("%s must declare an EnvironmentProfile", name)
			}
			for _, prof := range p.Catalog.Professions {
				if ancientProfessions[prof.ID] {
					t.Fatalf("%s leaked ancient profession %q", name, prof.ID)
				}
			}
			for itemID := range p.Catalog.Items {
				if ancientItems[itemID] {
					t.Fatalf("%s leaked ancient item %q", name, itemID)
				}
			}
			for _, d := range p.Map.Districts {
				for _, feat := range d.Features {
					if ancientFeatures[feat] {
						t.Fatalf("%s district %s leaked ancient feature %q", name, d.ID, feat)
					}
				}
			}
			if p.Environment.EnforcementItem != "" {
				if _, ok := p.Catalog.Items[p.Environment.EnforcementItem]; !ok {
					t.Fatalf("%s environment.enforcement_item %q not in catalog.items", name, p.Environment.EnforcementItem)
				}
			}
			if p.Environment.RepairItem != "" {
				if _, ok := p.Catalog.Items[p.Environment.RepairItem]; !ok {
					t.Fatalf("%s environment.repair_item %q not in catalog.items", name, p.Environment.RepairItem)
				}
			}
			if p.Environment.BuilderProfession != "" {
				if _, ok := p.Catalog.ProfessionByID(p.Environment.BuilderProfession); !ok {
					t.Fatalf("%s environment.builder_profession %q not in catalog.professions", name, p.Environment.BuilderProfession)
				}
			}
		})
	}
}

func TestXianxiaPackSimulatesWithGenericEnvironmentAndInstitutions(t *testing.T) {
	p, err := LoadFile(filepath.Join("..", "..", "packs", "xianxia_sect.json"))
	if err != nil {
		t.Fatalf("LoadFile(xianxia_sect.json): %v", err)
	}
	w, err := p.Instantiate("xianxia-test")
	if err != nil {
		t.Fatalf("Instantiate(xianxia_sect.json): %v", err)
	}
	env := w.Catalog.EffectiveEnvironment()
	if env.Genre != "xianxia" || env.CurrencyName != "灵石" || env.EnforcementItem != "enforcement_token" || env.RepairItem != "spirit_wood" {
		t.Fatalf("unexpected xianxia environment: %+v", env)
	}
	if env.Institutions.RacketeeringEnabled() || env.Institutions.RevoltEnabled() {
		t.Fatal("xianxia_sect should disable racketeering and revolt institutions")
	}

	hasEvent := func(evs []sim.Event, kind string) bool {
		for _, ev := range evs {
			if ev.Kind == kind {
				return true
			}
		}
		return false
	}

	// 1. Extort should fail both because Xianxia tags omit it and because racketeering institution is disabled.
	extortEvs := w.Apply("xue_sha", sim.Action{Kind: "extort", TargetID: "wu_shisan"})
	if !hasEvent(extortEvs, "action_failed") {
		t.Fatalf("expected extort to fail when racketeering institution is disabled, got %+v", extortEvs)
	}
	// Even if a tag grants extort, the disabled racketeering toggle must reject it.
	w.Catalog.TagActions["rogue_outlaw"] = append(w.Catalog.TagActions["rogue_outlaw"], "extort")
	extortEvs2 := w.Apply("xue_sha", sim.Action{Kind: "extort", TargetID: "wu_shisan"})
	if !hasEvent(extortEvs2, "action_failed") {
		t.Fatalf("expected disabled racketeering toggle to block extort even if tag-granted, got %+v", extortEvs2)
	}

	// 2. Repair house should consume spirit_wood (not plank).
	var yuHomeID, yuHomeDist string
	var beforeWood int
	for i := range w.Actors {
		if w.Actors[i].ID == "yu_qing" {
			yuHomeID = w.Actors[i].HomeID
			w.Actors[i].Energy = 100
			beforeWood = w.Actors[i].Items["spirit_wood"]
			if beforeWood < 1 {
				w.Actors[i].Items["spirit_wood"] = 2
				beforeWood = 2
			}
		}
	}
	for i := range w.Houses {
		if w.Houses[i].ID == yuHomeID {
			w.Houses[i].Condition = 70
			yuHomeDist = w.Houses[i].District
		}
	}
	for i := range w.Actors {
		if w.Actors[i].ID == "yu_qing" {
			w.Actors[i].Location = yuHomeDist
			w.Actors[i].InsideHouse = ""
			w.Actors[i].VenueID = ""
		}
	}
	repEvs := w.Apply("yu_qing", sim.Action{Kind: "repair_house", HouseID: yuHomeID})
	if hasEvent(repEvs, "action_failed") {
		t.Fatalf("expected repair_house with spirit_wood to succeed, got %+v", repEvs)
	}
	for _, a := range w.Actors {
		if a.ID == "yu_qing" && a.Items["spirit_wood"] >= beforeWood {
			t.Fatalf("expected spirit_wood to be consumed on repair_house (before=%d after=%d)", beforeWood, a.Items["spirit_wood"])
		}
	}

	// 3. Arrest should work via enforcement_token by sect_enforcer (shen_wuyue) on outlaw (xue_sha).
	for i := range w.Actors {
		if w.Actors[i].ID == "shen_wuyue" {
			w.Actors[i].Location = "valley_bazaar"
			w.Actors[i].VenueID = ""
			w.Actors[i].InsideHouse = ""
			w.Actors[i].Energy = 100
		}
		if w.Actors[i].ID == "xue_sha" {
			w.Actors[i].Location = "valley_bazaar"
			w.Actors[i].VenueID = ""
			w.Actors[i].InsideHouse = ""
		}
	}
	arrEvs := w.Apply("shen_wuyue", sim.Action{Kind: "arrest", TargetID: "xue_sha"})
	if hasEvent(arrEvs, "action_failed") {
		t.Fatalf("expected sect_enforcer arrest with enforcement_token to succeed, got %+v", arrEvs)
	}

	// 4. Advance 48 ticks (1 full day) cleanly and preserve bounds + ledger conservation.
	w2, err := p.Instantiate("xianxia-sim-1day")
	if err != nil {
		t.Fatalf("Instantiate(xianxia-sim-1day): %v", err)
	}
	for tick := 0; tick < sim.TicksPerDay; tick++ {
		if _, err := w2.Step(context.Background(), packWaitDecider{}); err != nil {
			t.Fatalf("Step tick %d: %v", tick, err)
		}
	}
	if err := w2.VerifyHoldingBounds(); err != nil {
		t.Fatalf("VerifyHoldingBounds after 1 day: %v", err)
	}
	if err := w2.VerifyLedgerConservation(); err != nil {
		t.Fatalf("VerifyLedgerConservation after 1 day: %v", err)
	}
}

type packWaitDecider struct{}

func (packWaitDecider) Decide(_ context.Context, _ sim.Perception) (sim.Action, error) {
	return sim.Action{Kind: "wait"}, nil
}


package sim

import "testing"

func TestEnvironmentProfileDefaultsAndInstitutionToggles(t *testing.T) {
	var empty Catalog
	env := empty.EffectiveEnvironment()
	if env.EnforcementItem != "bounty_writ" {
		t.Fatalf("default enforcement_item = %q, want bounty_writ", env.EnforcementItem)
	}
	if env.RepairItem != "plank" {
		t.Fatalf("default repair_item = %q, want plank", env.RepairItem)
	}
	if env.BuilderProfession != "carpenter" {
		t.Fatalf("default builder_profession = %q, want carpenter", env.BuilderProfession)
	}
	if !env.Institutions.TaxationEnabled() || !env.Institutions.EnforcementEnabled() ||
		!env.Institutions.OversightEnabled() || !env.Institutions.RacketeeringEnabled() ||
		!env.Institutions.RevoltEnabled() || !env.Institutions.RitualsEnabled() {
		t.Fatal("default nil institution toggles should all be enabled for backward compatibility")
	}

	falseVal := false
	custom := Catalog{
		Environment: &EnvironmentProfile{
			Genre:             "xianxia",
			Theme:             "青云宗与云麓灵市",
			CurrencyName:      "灵石",
			ActorNoun:         "修士",
			AuthorityName:     "执法堂",
			OversightName:     "长老会",
			ForumFeatures:     []string{"sect_square", "exchange_hub"},
			EnforcementTags:   []string{"enforcer"},
			OversightTags:     []string{"elder_oversight"},
			OutlawTags:        []string{"rogue_outlaw"},
			EnforcementItem:   "enforcement_token",
			RepairItem:        "spirit_wood",
			BuilderProfession: "talisman_crafter",
			Institutions: &InstitutionToggles{
				Racketeering: &falseVal,
				Revolt:       &falseVal,
			},
		},
		Professions: []Profession{
			{ID: "spirit_thief", StealthBonusPerLevel: 5},
			{ID: "sect_enforcer", VigilanceBonusPerLevel: 6},
			{ID: "rogue_raider", ExtortionBonusPerLevel: 3},
		},
		Items: map[string]ItemSpec{
			"spirit_grain": {ID: "spirit_grain", Tradeable: true, BatchPrice: 5},
			"qi_pill":      {ID: "qi_pill", Tradeable: true, BatchPrice: 28},
		},
	}

	eff := custom.EffectiveEnvironment()
	if eff.EnforcementItem != "enforcement_token" || eff.RepairItem != "spirit_wood" || eff.BuilderProfession != "talisman_crafter" {
		t.Fatalf("unexpected custom effective environment: %+v", eff)
	}
	if eff.Institutions.RacketeeringEnabled() || eff.Institutions.RevoltEnabled() {
		t.Fatal("expected racketeering and revolt toggles to be disabled")
	}
	if !eff.Institutions.EnforcementEnabled() || !eff.Institutions.OversightEnabled() {
		t.Fatal("unspecified institution toggles should default to enabled")
	}
	if !custom.IsEnforcementTags([]string{"cultivator", "enforcer"}) || custom.IsEnforcementTags([]string{"law"}) {
		t.Fatal("IsEnforcementTags should follow configured enforcement_tags")
	}
	if !custom.IsOversightTags([]string{"elder_oversight"}) || custom.IsOversightTags([]string{"censor"}) {
		t.Fatal("IsOversightTags should follow configured oversight_tags")
	}
	if !custom.IsOutlawTags([]string{"rogue_outlaw"}) {
		t.Fatal("IsOutlawTags should match configured outlaw_tags")
	}

	w := &World{
		Catalog: custom,
		Map: WorldMap{
			Districts: []District{
				{ID: "summit", Features: []string{"sect_square"}},
				{ID: "old_yamen", Features: []string{"yamen"}},
			},
		},
	}
	if !w.isConfiguredForumDistrict("summit") || w.isConfiguredForumDistrict("old_yamen") {
		t.Fatal("isConfiguredForumDistrict should follow configured forum_features")
	}
	stealth, maxLoot := custom.StealthBonus(Actor{Professions: map[string]int{"spirit_thief": 3}})
	if stealth != 15 || maxLoot != 5 {
		t.Fatalf("StealthBonus = (%d, %d), want (15, 5)", stealth, maxLoot)
	}
	if got := custom.VigilanceBonus(Actor{Professions: map[string]int{"sect_enforcer": 2}}); got != 12 {
		t.Fatalf("VigilanceBonus = %d, want 12", got)
	}
	if got := custom.ExtortionFee(Actor{Professions: map[string]int{"rogue_raider": 3}}); got != 11 {
		t.Fatalf("ExtortionFee = %d, want 11", got)
	}
	if got := custom.SelectLootItem(Actor{Items: map[string]int{"spirit_grain": 3, "qi_pill": 1}}); got != "qi_pill" {
		t.Fatalf("SelectLootItem = %q, want qi_pill (highest value tradeable item)", got)
	}
}

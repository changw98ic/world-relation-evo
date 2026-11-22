package sim

import (
	"context"
	"testing"
)

func TestPowerTierCombatSuppressionAndBreakthrough(t *testing.T) {
	w := Village("realm-test")
	w.Catalog.Forms = map[string]FormSpec{
		"qi_refining": {
			ID:             "qi_refining",
			Name:           "炼气期",
			PowerTier:      1,
			CombatBonus:    6,
			VigilanceBonus: 2,
		},
		"foundation": {
			ID:             "foundation",
			Name:           "筑基期",
			PowerTier:      2,
			CombatBonus:    22,
			VigilanceBonus: 8,
		},
		"nascent_soul": {
			ID:             "nascent_soul",
			Name:           "元婴期",
			PowerTier:      4,
			CombatBonus:    95,
			VigilanceBonus: 35,
		},
	}
	w.Catalog.Skills = map[string]SkillSpec{
		"sword_art": {
			ID:                  "sword_art",
			Name:                "剑道",
			CombatBonusPerLevel: 4,
		},
		"medical_dao": {
			ID:                "medical_dao",
			Name:              "医道",
			HealBonusPerLevel: 6,
		},
	}
	w.Catalog.StateSlots = map[string]StateSlotSpec{
		"qi":          {ID: "qi", Min: 0, Max: 100, Default: 50},
		"cultivation": {ID: "cultivation", Min: 0, Max: 10000, Default: 0},
	}
	w.Catalog.Transitions = map[string]TransitionSpec{
		"breakthrough_foundation": {
			ID:                  "breakthrough_foundation",
			Name:                "筑基破境",
			RequiredFormID:      "qi_refining",
			RequiredSlots:       map[string]int{"cultivation": 3000, "qi": 60},
			RequiredSkills:      map[string]int{"sword_art": 2},
			SlotCosts:           map[string]int{"cultivation": 400, "qi": 40},
			Ticks:               1,
			BaseSuccessPermille: 1000,
			SuccessFormID:       "foundation",
			RewardAttributes:    map[string]int{"spirit": 2, "strength": 1},
		},
	}

	junior := Actor{
		ID:         "junior",
		Name:       "炼气弟子",
		FormID:     "qi_refining",
		Attributes: Attributes{Strength: 10, Dexterity: 10, Insight: 10, Spirit: 10},
		Skills:     map[string]int{"sword_art": 2},
		StateSlots: map[string]int{"qi": 70, "cultivation": 3200},
	}
	senior := Actor{
		ID:         "senior",
		Name:       "元婴掌门",
		FormID:     "nascent_soul",
		Attributes: Attributes{Strength: 10, Dexterity: 10, Insight: 10, Spirit: 10},
		Skills:     map[string]int{"sword_art": 6},
	}

	pJunior := CombatPowerWithCatalog(w.Catalog, junior)
	pSenior := CombatPowerWithCatalog(w.Catalog, senior)
	if pSenior < pJunior*3 {
		t.Fatalf("expected nascent_soul combat power (%d) to overwhelm qi_refining (%d) by >=3x", pSenior, pJunior)
	}
	if w.Catalog.VigilanceBonus(senior) <= w.Catalog.VigilanceBonus(junior)+20 {
		t.Fatalf("expected nascent_soul vigilance (%d) >> qi_refining (%d)", w.Catalog.VigilanceBonus(senior), w.Catalog.VigilanceBonus(junior))
	}

	// Verify breakthrough transition succeeds, upgrades FormID, raises PowerTier, and awards attributes.
	w.Actors[0].FormID = "qi_refining"
	w.Actors[0].Skills = map[string]int{"sword_art": 2}
	w.Actors[0].StateSlots = map[string]int{"qi": 70, "cultivation": 3200}
	w.Actors[0].Attributes = Attributes{Strength: 10, Dexterity: 10, Insight: 10, Spirit: 10}
	if ok, reason := CanAttemptTransition(w.Actors[0], w.Catalog.Transitions["breakthrough_foundation"]); !ok {
		t.Fatalf("expected actor to satisfy transition preconditions, got %q", reason)
	}
	evs := w.applyAttemptTransition(w.BeginTx((&w.Actors[0]).ID, ActionIntent{}), &w.Actors[0], Action{Kind: "attempt_transition", TransitionID: "breakthrough_foundation"})
	if w.Actors[0].FormID != "foundation" {
		t.Fatalf("expected FormID=foundation after breakthrough, got %q (events=%+v)", w.Actors[0].FormID, evs)
	}
	if w.Catalog.ActorPowerTier(w.Actors[0]) != 2 {
		t.Fatalf("expected PowerTier=2 after breakthrough, got %d", w.Catalog.ActorPowerTier(w.Actors[0]))
	}
	if w.Actors[0].Attributes.Spirit != 12 || w.Actors[0].Attributes.Strength != 11 {
		t.Fatalf("expected breakthrough attribute rewards (+2 spirit, +1 strength), got %+v", w.Actors[0].Attributes)
	}

	// Verify real combat attack applies realm suppression (senior hits hard, junior scratches senior and takes recoil).
	w.Actors[0].Location, w.Actors[1].Location = "market_street", "market_street"
	w.Actors[0].InsideHouse, w.Actors[1].InsideHouse = "", ""
	w.Actors[0].VenueID, w.Actors[1].VenueID = "", ""
	w.Actors[0].FormID, w.Actors[1].FormID = "qi_refining", "nascent_soul"
	w.Actors[0].Skills = map[string]int{"sword_art": 1}
	w.Actors[1].Skills = map[string]int{"sword_art": 6}
	w.Actors[0].Tags = append(w.Actors[0].Tags, "fighter")
	w.Actors[1].Tags = append(w.Actors[1].Tags, "fighter")
	w.Actors[0].Health, w.Actors[1].Health = 100, 100
	w.Actors[0].Energy, w.Actors[1].Energy = 90, 90
	w.Actors[0].RestUntil, w.Actors[1].RestUntil = 0, 0

	// Junior attacks Senior -> scratch damage (<= 5) and attacker recoil!
	w.Apply(w.Actors[0].ID, Action{Kind: "attack", TargetID: w.Actors[1].ID})
	if w.Actors[1].Health < 95 {
		t.Fatalf("expected nascent_soul to take only scratch damage from qi_refining, got health=%d", w.Actors[1].Health)
	}
	if w.Actors[0].Health >= 100 {
		t.Fatalf("expected qi_refining attacking nascent_soul to suffer recoil, got health=%d", w.Actors[0].Health)
	}

	// Senior attacks Junior -> heavy suppression damage (>= 24), capped so no instant 1-shot from 100 HP.
	w.Actors[0].Health = 100
	w.Apply(w.Actors[1].ID, Action{Kind: "attack", TargetID: w.Actors[0].ID})
	dmgToJunior := 100 - w.Actors[0].Health
	if dmgToJunior < 24 || dmgToJunior > 32 {
		t.Fatalf("expected nascent_soul attack on qi_refining to deal heavy capped damage [24,32], got %d", dmgToJunior)
	}
}

func TestIndependentSkillsCrossProfessionAccessAndMentorship(t *testing.T) {
	w := Village("skill-test")
	w.Catalog.Skills = map[string]SkillSpec{
		"talisman_craft": {
			ID:                  "talisman_craft",
			Name:                "制符",
			MaxLevel:            6,
			Ranks:               []string{"画符初学", "灵纹贯通", "符箓大家", "天符宗师"},
			RepairBonusPerLevel: 3,
		},
		"trade_craft": {
			ID:                  "trade_craft",
			Name:                "商道",
			TradeMarginPerLevel: 3,
		},
	}
	recipe := Recipe{
		ID:         "inscribe_talisman",
		Profession: "carpenter",
		SkillID:    "talisman_craft",
		OutputItem: "scroll",
		OutputQty:  1,
	}

	// Actor without carpenter profession, but WITH independent talisman_craft skill, qualifies!
	merchant := Actor{
		ID:          "m1",
		Professions: map[string]int{"peddler": 2},
		Skills:      map[string]int{"talisman_craft": 2, "trade_craft": 3},
	}
	if ok, reason := w.Catalog.ProfessionGating(merchant, recipe); !ok {
		t.Fatalf("expected independent skill to grant cross-profession recipe access, got %q", reason)
	}
	plainMerchant := Actor{
		ID:          "m2",
		Professions: map[string]int{"peddler": 2},
	}
	if ok, _ := w.Catalog.ProfessionGating(plainMerchant, recipe); ok {
		t.Fatalf("expected actor lacking both profession and skill to be gated out")
	}
	if w.Catalog.SalePrice(merchant, "food") <= w.Catalog.SalePrice(plainMerchant, "food") {
		t.Fatalf("expected trade_craft skill to boost SalePrice (%d vs %d)", w.Catalog.SalePrice(merchant, "food"), w.Catalog.SalePrice(plainMerchant, "food"))
	}

	// Skill mentorship via teach_knowledge transfers skill to co-located junior.
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location, b.Location = "market_street", "market_street"
	a.InsideHouse, b.InsideHouse = "", ""
	a.VenueID, b.VenueID = "", ""
	a.Energy = 80
	a.Skills = map[string]int{"talisman_craft": 4}
	b.Skills = nil
	evs := w.applyTeachKnowledge(w.BeginTx(a.ID, ActionIntent{}), a, Action{Kind: "teach_knowledge", SkillID: "talisman_craft", TargetID: b.ID}, b)
	if b.Skills["talisman_craft"] < 1 {
		t.Fatalf("expected student to learn talisman_craft Lv.1 via mentorship, got %+v (events=%+v)", b.Skills, evs)
	}

	// Ensure Perception exposes Neighbor FormID, PowerTier, CombatPower, and Skills consistently.
	_ = context.Background()
	view := w.View(a.ID)
	found := false
	for _, nb := range view.Nearby {
		if nb.ID == b.ID {
			found = true
			if nb.Skills["talisman_craft"] != b.Skills["talisman_craft"] {
				t.Fatalf("neighbor view missing synced skills: got %+v want %+v", nb.Skills, b.Skills)
			}
		}
	}
	if !found {
		t.Fatalf("expected %s in %s nearby list", b.ID, a.ID)
	}
}

func TestIndividualActorNonCombatHazards(t *testing.T) {
	w := Village("hazard-test")
	w.InitEcology(60)
	w.Catalog.Items["protective_charm"] = ItemSpec{
		ID:             "protective_charm",
		Name:           "护身符",
		SafetyBonusPct: 40,
	}
	w.Catalog.Skills = map[string]SkillSpec{
		"alchemy": {ID: "alchemy", Name: "炼丹", MaxLevel: 6},
	}
	w.Catalog.Accidents = []AccidentSpec{
		{
			ID:                "furnace_blast",
			Name:              "丹炉炸裂",
			Recipes:           []string{"brew_test_pill"},
			BasePermille:      1000,
			Damage:            24,
			LethalityPermille: 0,
		},
		{
			ID:                "road_landslide",
			Name:              "山道塌方",
			RouteMediums:      []string{"road"},
			BasePermille:      1000,
			Damage:            18,
			LethalityPermille: 0,
		},
	}
	w.Catalog.Encounters = []EncounterSpec{
		{
			ID:                "wild_wolf",
			Name:              "妖狼",
			Category:          "beast",
			NodeFeatures:      []string{"market"},
			BaseActivity:      50,
			HazardPermille:    1000,
			Damage:            20,
			LethalityPermille: 0,
		},
	}

	// 1. Craft accident damages novice individual spotlight Actor & skill + item reduce risk.
	a := &w.Actors[0]
	a.Health = 100
	a.Location = w.Map.Districts[0].ID
	a.InsideHouse = ""
	a.Skills = map[string]int{"alchemy": 0}
	r := Recipe{ID: "brew_test_pill", SkillID: "alchemy"}
	evs := w.checkActorCraftAccident(nil, a, Action{Kind: "craft", RecipeID: r.ID}, r)
	if len(evs) == 0 || evs[0].Kind != "workplace_accident" || a.Health >= 100 {
		t.Fatalf("expected individual spotlight actor to suffer workplace_accident, got health=%d evs=%+v", a.Health, evs)
	}

	// Skill mastery + protective charm measurably increase mitigation.
	mitNovice := w.actorRiskMitigations(Actor{Skills: map[string]int{"alchemy": 1}}, "alchemy", "")
	mitMaster := w.actorRiskMitigations(Actor{
		Skills: map[string]int{"alchemy": 6},
		Items:  map[string]int{"protective_charm": 1},
	}, "alchemy", "")
	if mitMaster.skillMitPct <= mitNovice.skillMitPct || mitMaster.itemSafetyPct != 40 {
		t.Fatalf("expected master+charm mitigations > novice, got novice=%+v master=%+v", mitNovice, mitMaster)
	}

	// 2. Fatal craft accident sets DeathCause="accident" and DeathTick.
	a.Health = 10
	a.Memories = nil
	w.Tick = 42
	evsFatal := w.checkActorCraftAccident(nil, a, Action{Kind: "craft", RecipeID: r.ID}, r)
	if !a.Dead || a.DeathCause != "accident" || a.DeathTick != 42 {
		t.Fatalf("expected fatal accident death cause='accident' tick=42, got dead=%v cause=%q tick=%d evs=%+v", a.Dead, a.DeathCause, a.DeathTick, evsFatal)
	}

	// 3. Cross-district transit hazard fires when from != to, never on same-district hop.
	b := &w.Actors[1]
	b.Dead = false
	b.Health = 100
	b.Memories = nil
	d0, d1 := w.Map.Districts[0].ID, w.Map.Districts[1].ID
	if sameEvs := w.checkActorTransitHazard(nil, b, Action{Kind: "move", Location: d0}, d0, d0, "road", "", 0); len(sameEvs) != 0 {
		t.Fatalf("expected no transit hazard on same-district hop, got %+v", sameEvs)
	}
	crossEvs := w.checkActorTransitHazard(nil, b, Action{Kind: "move", Location: d1}, d0, d1, "road", "", 0)
	if len(crossEvs) == 0 || crossEvs[0].Kind != "transit_hazard" || b.Health >= 100 {
		t.Fatalf("expected cross-district transit_hazard on spotlight actor, got health=%d evs=%+v", b.Health, crossEvs)
	}

	// 4. Daily outdoor wild encounter hits outdoor spotlight actor but spares sheltered indoor actor.
	c := &w.Actors[2]
	c.Dead = false
	c.Health = 100
	c.Memories = nil
	c.Location = d0
	c.InsideHouse = ""
	ds := w.DistrictStates[d0]
	if ds.ThreatActivity == nil {
		ds.ThreatActivity = map[string]int{}
	}
	ds.ThreatActivity["wild_wolf"] = 60
	w.DistrictStates[d0] = ds
	// Put actor 1 indoors so only outdoor actor 2 is exposed.
	b.Health = 100
	b.Location = d0
	b.InsideHouse = w.Houses[0].ID
	b.Memories = nil
	outEvs := w.stepActorOutdoorEncounters()
	if b.Health != 100 {
		t.Fatalf("expected indoor actor to be sheltered from outdoor wild_encounter, got health=%d", b.Health)
	}
	if c.Health >= 100 || len(outEvs) == 0 {
		t.Fatalf("expected outdoor spotlight actor to face wild_encounter, got health=%d evs=%+v", c.Health, outEvs)
	}
}

func TestDynamicFormIntakeScalingAndMinFoodTier(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	lc := LifecycleSpec{StarvationThreshold: 6, StarvationDamage: 10}

	mortal := FormSpec{ID: "mortal", PowerTier: 0, DailyIntakePct: intPtr(100)}
	fasting := FormSpec{ID: "foundation", PowerTier: 2, DailyIntakePct: intPtr(0)}
	bodyRefiner := FormSpec{
		ID:                  "body_refiner",
		PowerTier:           3,
		DailyIntakePct:      intPtr(300), // 3x mortal daily intake!
		StarvationThreshold: 12,
		StarvationDamage:    intPtr(15),
		MinFoodTier:         2,
	}
	nonLethal := FormSpec{
		ID:               "low_qi",
		PowerTier:        1,
		DailyIntakePct:   intPtr(35),
		StarvationDamage: intPtr(0),
	}

	sumDay := func(f FormSpec) int {
		tot := 0
		for tick := int64(1); tick <= TicksPerDay; tick++ {
			tot += hungerIncrement(tick, lc, f)
		}
		return tot
	}
	if got := sumDay(mortal); got != int(DailyNutritionUnits) {
		t.Fatalf("mortal daily hunger = %d, want %d", got, DailyNutritionUnits)
	}
	if got := sumDay(fasting); got != 0 {
		t.Fatalf("fasting form daily hunger = %d, want 0", got)
	}
	if got := sumDay(bodyRefiner); got != int(DailyNutritionUnits*3) {
		t.Fatalf("high-intake body_refiner daily hunger = %d, want %d (3x mortal)", got, DailyNutritionUnits*3)
	}
	if got := sumDay(nonLethal); got != 1 {
		t.Fatalf("35%% intake form daily hunger = %d, want 1", got)
	}

	// Per-form starvation overrides (fasting=0, nonLethal=0, bodyRefiner=0 at hunger 8, 15 at hunger 12).
	if dmg := starvationDamage(20, lc, fasting); dmg != 0 {
		t.Fatalf("fasting starvation damage = %d, want 0", dmg)
	}
	if dmg := starvationDamage(20, lc, nonLethal); dmg != 0 {
		t.Fatalf("non-lethal starvation damage = %d, want 0", dmg)
	}
	if dmg := starvationDamage(8, lc, bodyRefiner); dmg != 0 {
		t.Fatalf("bodyRefiner below custom threshold 12 should take 0 damage, got %d", dmg)
	}
	if dmg := starvationDamage(12, lc, bodyRefiner); dmg != 15 {
		t.Fatalf("bodyRefiner at custom threshold 12 should take 15 damage, got %d", dmg)
	}

	// MinFoodTier gating on eat & AvailableNutrition: body_refiner rejects Tier-1 grain, accepts Tier-2 beast meat.
	w := Village("intake-tier-test")
	w.Catalog.Forms = map[string]FormSpec{"body_refiner": bodyRefiner}
	w.Catalog.Items["coarse_grain"] = ItemSpec{ID: "coarse_grain", Name: "粗粮", Tier: 1, Edible: true, NutritionUnits: 2}
	w.Catalog.Items["beast_meat"] = ItemSpec{ID: "beast_meat", Name: "妖兽肉", Tier: 2, Edible: true, NutritionUnits: 4}
	a := &w.Actors[0]
	a.FormID = "body_refiner"
	a.Hunger = 5
	a.NutritionReserve = 0
	a.Items = map[string]int{"coarse_grain": 3, "beast_meat": 1}

	if got := w.Catalog.AvailableNutrition(*a); got != 4 {
		t.Fatalf("AvailableNutrition should count only Tier>=2 beast_meat (4), got %d", got)
	}
	evsLow := w.Apply(a.ID, Action{Kind: "eat", Item: "coarse_grain"})
	if len(evsLow) == 0 || evsLow[0].Kind != "action_failed" {
		t.Fatalf("expected Tier-1 coarse_grain to be rejected by MinFoodTier=2 form, got %+v", evsLow)
	}
	evsHigh := w.Apply(a.ID, Action{Kind: "eat", Item: "beast_meat"})
	if len(evsHigh) == 0 || evsHigh[0].Kind != "ate" || a.Hunger != 1 {
		t.Fatalf("expected Tier-2 beast_meat to satisfy body_refiner hunger (5->1), got hunger=%d evs=%+v", a.Hunger, evsHigh)
	}

	// RequiredTag on StateSlotSpec prevents non-tagged mortals from absorbing ambient qi/cultivation.
	w.Catalog.StateSlots = map[string]StateSlotSpec{
		"qi": {ID: "qi", RequiredTag: "cultivator", Min: 0, Max: 100, Default: 50, RegenPerTick: 2},
	}
	w.Actors[0].Tags = []string{"resident"} // mortal without cultivator tag
	w.Actors[0].StateSlots = nil
	w.Actors[1].Tags = []string{"resident", "cultivator"}
	w.Actors[1].StateSlots = nil
	w.stepStateSlotsAndForms()
	if w.Actors[0].StateSlots["qi"] != 0 {
		t.Fatalf("expected mortal without required_tag='cultivator' to keep qi=0, got %d", w.Actors[0].StateSlots["qi"])
	}
	if w.Actors[1].StateSlots["qi"] <= 0 {
		t.Fatalf("expected cultivator with required_tag='cultivator' to gain qi>0, got %d", w.Actors[1].StateSlots["qi"])
	}
}

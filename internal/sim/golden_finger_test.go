package sim

import (
	"testing"
)

func TestGoldenFingerRingMentorAndDisguiseAndRescue(t *testing.T) {
	w := Village("gf-ring-mentor")
	w.Catalog.Skills = map[string]SkillSpec{
		"sword_art": {ID: "sword_art", Name: "剑道", MaxLevel: 6, CombatBonusPerLevel: 5},
	}
	w.Catalog.Forms = map[string]FormSpec{
		"mortal":  {ID: "mortal", Name: "凡胎", PowerTier: 1, CombatBonus: 0},
		"nascent": {ID: "nascent", Name: "元婴", PowerTier: 5, CombatBonus: 40},
	}
	w.Catalog.Items["ancient_ring"] = ItemSpec{ID: "ancient_ring", Name: "药老残魂戒", Tier: 5, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 100}
	w.Catalog.GoldenFingers = []GoldenFingerSpec{
		{
			ID:             "gf_ancient_ring",
			Name:           "药老残魂戒",
			GrantedByItems: []string{"ancient_ring"},
			Passive: &GFPassiveSpec{
				SkillXPMultiplierPct: 250,
				CombatBonus:          15,
				DisguisePowerTier:    1,
				RescueBelowHealth:    25,
				RescueHeal:           50,
				RescueCooldownTicks:  96,
				RescueName:           "残魂代打护主",
			},
		},
	}

	hero := &w.Actors[0]
	observer := &w.Actors[1]
	hero.FormID = "nascent"
	hero.Skills = map[string]int{"sword_art": 1}
	hero.Items["ancient_ring"] = 1
	w.recordLedgerItemProduced("ancient_ring", 1)
	hero.Location = observer.Location
	hero.InsideHouse = ""
	observer.InsideHouse = ""

	// 1. Disguise: observer perceives hero as PowerTier 1 ("扮猪吃虎"), despite hero being tier 5.
	obsView := w.View(observer.ID)
	foundHero := false
	for _, nb := range obsView.Nearby {
		if nb.ID == hero.ID {
			foundHero = true
			if nb.PowerTier != 1 {
				t.Fatalf("expected disguised PowerTier 1, got %d", nb.PowerTier)
			}
		}
	}
	if !foundHero {
		t.Fatalf("expected observer to see co-located hero")
	}

	// 2. Skill XP multiplier: 250% turns 4 XP -> 10 XP.
	_ = w.Apply(hero.ID, Action{Kind: "practice_skill", SkillID: "sword_art"})
	if hero.SkillXP["sword_art"] != 10 {
		t.Fatalf("expected 2.5x skill XP (10), got %d", hero.SkillXP["sword_art"])
	}

	// 3. Ring guardian emergency rescue when HP <= 25.
	hero.Health = 18
	evs := w.stepGoldenFingers(nil)
	if hero.Health != 68 {
		t.Fatalf("expected ring rescue to heal hero from 18 to 68, got %d", hero.Health)
	}
	foundRescue := false
	for _, ev := range evs {
		if ev.Kind == "gf_rescue" {
			foundRescue = true
		}
	}
	if !foundRescue {
		t.Fatalf("expected gf_rescue event, got %+v", evs)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

func TestGoldenFingerEmotionHarvestShopSynthesisAndPocketRealm(t *testing.T) {
	w := Village("gf-emotion-shop")
	w.Catalog.Currencies = map[string]CurrencySpec{
		"emotion_pts": {ID: "emotion_pts", Name: "情绪值"},
	}
	w.Catalog.Items["spirit_herb"] = ItemSpec{ID: "spirit_herb", Name: "灵泉仙草", Tier: 3, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 20}
	w.Catalog.Items["super_pill"] = ItemSpec{ID: "super_pill", Name: "破境金丹", Tier: 5, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 120}
	w.Catalog.GoldenFingers = []GoldenFingerSpec{
		{
			ID:           "gf_emotion_sys",
			Name:         "万物情绪商城系统",
			OwnerActorID: w.Actors[0].ID,
			Passive: &GFPassiveSpec{
				DailyPocketYield: &GFRewardSpec{
					Items: map[string]int{"spirit_herb": 2},
				},
			},
			Triggers: []GFTriggerSpec{
				{
					ID:               "shock_harvest",
					Name:             "当众震惊收割情绪值",
					OnEvents:         []string{"attacked"},
					MatchRole:        "witnessed_by_others",
					ScaleByObservers: true,
					Reward: GFRewardSpec{
						Currencies: map[string]int64{"emotion_pts": 15},
					},
				},
				{
					ID:              "landmark_signin",
					Name:            "首次地标打卡签到",
					OnEvents:        []string{"moved"},
					OncePerDistrict: true,
					Reward: GFRewardSpec{
						Currencies: map[string]int64{"emotion_pts": 50},
						Coins:      20,
					},
				},
			},
			Shop: []GFShopItemSpec{
				{
					ID:           "buy_super_pill",
					Name:         "兑换破境金丹",
					CostCurrency: "emotion_pts",
					CostAmount:   30,
					CostItems:    map[string]int{"spirit_herb": 2},
					Reward: GFRewardSpec{
						Items:      map[string]int{"super_pill": 1},
						Attributes: map[string]int{"spirit": 3},
					},
					StockLimit: 2,
				},
			},
		},
	}

	hero := &w.Actors[0]
	hero.InsideHouse = ""
	// Place 2 awake co-located observers.
	w.Actors[1].Location = hero.Location
	w.Actors[1].InsideHouse = ""
	w.Actors[2].Location = hero.Location
	w.Actors[2].InsideHouse = ""

	// Tick at 06:00 triggers daily pocket yield (+2 spirit_herb) + emotion harvest from witnessed attack (2 observers * 15 = 30 emotion_pts).
	w.Tick = 0
	mockEvents := []Event{
		{ID: 9001, Tick: 0, ActorID: hero.ID, TargetID: w.Actors[1].ID, Kind: "attacked"},
		{ID: 9002, Tick: 0, ActorID: hero.ID, Kind: "moved"},
	}
	evs := w.stepGoldenFingers(mockEvents)
	if len(evs) < 3 {
		t.Fatalf("expected pocket yield + emotion harvest + landmark sign-in events, got %d", len(evs))
	}
	if hero.Items["spirit_herb"] != 2 {
		t.Fatalf("expected 2 spirit_herb from pocket yield, got %d", hero.Items["spirit_herb"])
	}
	// 2 observers * 15 (30) + 50 from landmark sign-in = 80 emotion_pts.
	if hero.Currencies["emotion_pts"] < 80 {
		t.Fatalf("expected >=80 emotion_pts, got %d", hero.Currencies["emotion_pts"])
	}

	// Redeem system shop synthesis offer (30 emotion_pts + 2 spirit_herb -> 1 super_pill + 3 spirit).
	beforeSpirit := hero.Attributes.Spirit
	exEvs := w.Apply(hero.ID, Action{Kind: "gf_exchange", Text: "gf_emotion_sys", Item: "buy_super_pill"})
	if len(exEvs) == 0 || exEvs[0].Kind != "gf_exchanged" {
		t.Fatalf("expected gf_exchanged, got %+v", exEvs)
	}
	if hero.Items["super_pill"] != 1 || hero.Items["spirit_herb"] != 0 || hero.Attributes.Spirit != beforeSpirit+3 {
		t.Fatalf("unexpected post-exchange state: pill=%d herb=%d spirit=%d", hero.Items["super_pill"], hero.Items["spirit_herb"], hero.Attributes.Spirit)
	}

	// Second move in same district must NOT re-trigger OncePerDistrict sign-in.
	coinsBefore := hero.Money
	w.stepGoldenFingers([]Event{{ID: 9003, Tick: 1, ActorID: hero.ID, Kind: "moved"}})
	if hero.Money != coinsBefore {
		t.Fatalf("expected OncePerDistrict sign-in not to fire twice in same district")
	}

	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

func TestGoldenFingerDevourMenteeRebateQuestAndSimulator(t *testing.T) {
	w := Village("gf-devour-quest-sim")
	w.Catalog.Skills = map[string]SkillSpec{
		"blade": {ID: "blade", Name: "狂刀", MaxLevel: 6},
	}
	hero := &w.Actors[0]
	mentee := &w.Actors[1]
	enemy := &w.Actors[2]
	enemy.Skills = map[string]int{"blade": 4}
	w.ensureRelations().Mentorships = append(w.ensureRelations().Mentorships, MentorshipBond{
		MentorID: hero.ID, ApprenticeID: mentee.ID, TopicID: "blade",
	})

	w.Catalog.GoldenFingers = []GoldenFingerSpec{
		{
			ID:           "gf_all_in_one",
			Name:         "诸天吞噬返还推演系统",
			OwnerActorID: hero.ID,
			Triggers: []GFTriggerSpec{
				{
					ID:               "devour_kill",
					Name:             "噬魂吞噬",
					OnEvents:         []string{"actor_died"},
					MatchRole:        "self_killer",
					DevourSkill:      true,
					DevourAttrPoints: 2,
				},
				{
					ID:        "mentee_rebate",
					Name:      "授徒万倍返还",
					OnEvents:  []string{"skill_leveled"},
					MatchRole: "mentee_or_kin",
					Reward:    GFRewardSpec{Coins: 50, SkillXP: map[string]int{"blade": 20}},
				},
			},
			Quests: []GFQuestSpec{
				{
					ID:       "q_kill_enemy",
					Title:    "斩杀宿敌",
					Kind:     "eliminate_actor",
					TargetID: enemy.ID,
					Reward:   GFRewardSpec{Coins: 100},
				},
				{
					ID:             "q_timed_fail",
					Title:          "限时试炼",
					Kind:           "visit_district",
					TargetID:       "nonexistent_zone",
					DeadlineTick:   5,
					FailurePenalty: &GFPenaltySpec{HealthLoss: 15, CoinsLoss: 10},
				},
			},
			Simulator: &GFSimulatorSpec{
				Name:             "人生模拟器",
				CooldownTicks:    24,
				CostCoins:        10,
				RewardAttributes: map[string]int{"insight": 2},
			},
		},
	}

	// 1. Kill enemy cleanly (settling estate) -> triggers devour + completes quest q_kill_enemy.
	enemy.Health = 0
	deathEvs := w.markDeadIfZero(enemy, hero, Action{Kind: "attack", TargetID: enemy.ID}, "combat")
	strBefore := hero.Attributes.Strength
	coinsBefore := hero.Money
	evs := w.stepGoldenFingers(append(deathEvs,
		Event{ID: 101, Tick: 1, ActorID: mentee.ID, Kind: "skill_leveled"},
	))
	if hero.Attributes.Strength != strBefore+2 {
		t.Fatalf("expected +2 STR devoured, got %d -> %d", strBefore, hero.Attributes.Strength)
	}
	if hero.Skills["blade"] < 1 {
		t.Fatalf("expected devoured/rebated blade skill, got %+v", hero.Skills)
	}
	if hero.Money != coinsBefore+150 { // 50 mentee rebate + 100 quest reward
		t.Fatalf("expected +150 coins (50 rebate + 100 quest), got %d -> %d", coinsBefore, hero.Money)
	}
	_ = evs

	// 2. Run life simulator.
	insBefore := hero.Attributes.Insight
	simEvs := w.Apply(hero.ID, Action{Kind: "gf_simulate", Text: "gf_all_in_one"})
	if len(simEvs) == 0 || simEvs[0].Kind != "gf_simulated" || hero.Attributes.Insight != insBefore+2 {
		t.Fatalf("expected gf_simulated +2 insight, got %+v (insight %d->%d)", simEvs, insBefore, hero.Attributes.Insight)
	}

	// 3. Timed quest expiration triggers penalty.
	w.Tick = 5
	hpBefore := hero.Health
	failEvs := w.stepGoldenFingers(nil)
	if hero.Health != hpBefore-15 {
		t.Fatalf("expected quest failure penalty -15 HP, got %d -> %d (evs=%+v)", hpBefore, hero.Health, failEvs)
	}

	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

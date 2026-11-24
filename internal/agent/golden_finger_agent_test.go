package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func intPtrGF(v int) *int { return &v }

func TestGoldenFingerEndToEndBlueprintAndAgentLoop(t *testing.T) {
	bp := pack.Blueprint{
		ID:    "gf-e2e-demo",
		Name:  "通用金手指闭环测试",
		Genre: "xianxia",
		Skills: []sim.SkillSpec{
			{ID: "qi_art", Name: "吐纳剑诀", MaxLevel: 6, CombatBonusPerLevel: 5},
		},
		Currencies: []sim.CurrencySpec{
			{ID: "sys_pts", Name: "系统积分"},
		},
		Forms: []pack.BlueprintForm{
			{ID: "qi_1", Name: "炼气一层", PowerTier: intPtrGF(1), CombatBonus: 0},
			{ID: "qi_2", Name: "筑基初期", PowerTier: intPtrGF(2), CombatBonus: 15},
		},
		Transitions: []sim.TransitionSpec{
			{
				ID:                  "break_foundation",
				Name:                "筑基破境",
				RequiredFormID:      "qi_1",
				ItemCosts:           map[string]int{"foundation_pill": 1},
				Ticks:               1,
				BaseSuccessPermille: 1000,
				SuccessFormID:       "qi_2",
			},
		},
		CustomItems: []pack.BlueprintItem{
			{ID: "foundation_pill", Name: "筑基丹", Tier: 4, BatchPrice: intPtrGF(80)},
		},
		Districts: []pack.BlueprintDistrict{
			{ID: "sect_outer", Name: "外门广场", Features: []string{"public_forum", "market"}},
			{ID: "secret_valley", Name: "秘境灵谷", Features: []string{"public_forum", "market", "relic_site"}},
		},
		GoldenFingers: []sim.GoldenFingerSpec{
			{
				ID:           "gf_signin_shop_sim",
				Name:         "诸天签到商城推演系统",
				OwnerActorID: "hero",
				Passive: &sim.GFPassiveSpec{
					SkillXPMultiplierPct: 200,
					DisguisePowerTier:    1,
					DailyPocketYield:     &sim.GFRewardSpec{Currencies: map[string]int64{"sys_pts": 40}},
				},
				Triggers: []sim.GFTriggerSpec{
					{
						ID:       "daily_signin",
						Name:     "每日签到",
						OnEvents: []string{"day_start"},
						Reward:   sim.GFRewardSpec{Currencies: map[string]int64{"sys_pts": 60}},
					},
				},
				Shop: []sim.GFShopItemSpec{
					{
						ID:           "redeem_foundation_pill",
						Name:         "兑换筑基丹",
						CostCurrency: "sys_pts",
						CostAmount:   50,
						Reward:       sim.GFRewardSpec{Items: map[string]int{"foundation_pill": 1}},
						StockLimit:   1,
					},
				},
				Quests: []sim.GFQuestSpec{
					{
						ID:       "q_visit_valley",
						Title:    "探索秘境灵谷",
						Kind:     "visit_district",
						TargetID: "secret_valley",
						Reward:   sim.GFRewardSpec{Coins: 50},
					},
				},
				Foresights: []sim.GFForesight{
					{
						ID:         "fs_valley",
						RevealTick: 0,
						TargetTick: 24,
						DistrictID: "secret_valley",
						Kind:       "opportunity",
						Summary:    "秘境灵谷即将开启机缘",
					},
				},
				Simulator: &sim.GFSimulatorSpec{
					Name:             "人生模拟器",
					CooldownTicks:    24,
					CostCoins:        5,
					RewardAttributes: map[string]int{"insight": 1},
				},
			},
		},
		Actors: []pack.BlueprintActor{
			{
				ID: "hero", Name: "林凡", Role: "disciple", Location: "sect_outer",
				Tags: []string{"fighter"}, FormID: "qi_1", Skills: map[string]int{"qi_art": 1},
				Personality: "patient studious righteous", Money: intPtr(50),
			},
			{
				ID: "rival", Name: "赵虎", Role: "disciple", Location: "sect_outer",
				Tags: []string{"fighter"}, FormID: "qi_1",
				Personality: "proud reckless", Money: intPtr(50),
			},
		},
	}

	wp, err := pack.CompileBlueprint(bp)
	if err != nil {
		t.Fatalf("CompileBlueprint failed: %v", err)
	}
	w, err := wp.Instantiate("")
	if err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	rb := DefaultBaseRulebook(w)
	sel := NewRulebookSelector(rb, nil)
	pipe := &Pipeline{Selector: sel}

	seen := map[string]int{}
	for step := 0; step < 30; step++ {
		evs, err := w.Step(context.Background(), pipe)
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		for _, ev := range evs {
			seen[ev.Kind]++
		}
	}

	if seen["gf_triggered"] == 0 {
		t.Fatalf("expected daily sign-in gf_triggered, got %+v", seen)
	}
	if seen["gf_exchanged"] == 0 {
		t.Fatalf("expected autonomous shop redemption gf_exchanged, got %+v", seen)
	}
	if seen["gf_simulated"] == 0 {
		t.Fatalf("expected autonomous simulator gf_simulated, got %+v", seen)
	}
	if seen["gf_quest_completed"] == 0 {
		t.Fatalf("expected autonomous foresight/quest completion gf_quest_completed, got %+v", seen)
	}
	if seen["transition_succeeded"] == 0 {
		t.Fatalf("expected hero to break through to qi_2 using redeemed foundation_pill, got %+v", seen)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed after golden finger simulation: %v", err)
	}
}

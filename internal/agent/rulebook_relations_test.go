package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRulebookSelector_RelationshipModulationAndProtectiveDefense(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "rel-test",
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_attack_witness",
				Domain:            "shadow_conflict",
				Name:              "目击同门遇袭",
				TriggerEventKinds: []string{"attacked"},
				ResponderPosition: "witness",
				Actor1Match:       RoleMatcher{Roles: []string{"*"}},
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				Branches: []ReactionBranch{
					{ID: "br_w_fight", Stance: "强硬反制", Title: "出手护短", Weight: 50, Behavior: BehaviorSpec{EngineKind: "attack", TargetBinding: "$actor1"}},
					{ID: "br_w_flee", Stance: "避险止损", Title: "回避自保", Weight: 50, Behavior: BehaviorSpec{EngineKind: "move", TargetBinding: "$actor1"}},
				},
			},
		},
	}

	// 1. Direct multiplier check: mentor/ally boosts 积极呼应 and suppresses 强硬反制; feud enemy boosts 强硬反制;
	// witnessing attack on a bonded ally triggers protective 强硬反制 (defendingBond=true).
	mAllyCoop, _ := computeRelationStanceMultiplier("积极呼应", 5, true, true, false, false, false)
	mAllyFight, _ := computeRelationStanceMultiplier("强硬反制", 5, true, true, false, false, false)
	if mAllyCoop <= 1.3 || mAllyFight >= 0.5 {
		t.Fatalf("expected mentor/ally to boost 积极呼应 (>1.3) and suppress 强硬反制 (<0.5), got coop=%.2f fight=%.2f", mAllyCoop, mAllyFight)
	}
	mDefFight, _ := computeRelationStanceMultiplier("强硬反制", 0, false, false, false, false, true)
	mDefFlee, _ := computeRelationStanceMultiplier("避险止损", 0, false, false, false, false, true)
	if mDefFight <= 1.5 || mDefFlee >= 0.7 {
		t.Fatalf("expected defending bonded ally to boost 强硬反制 (>1.5) and suppress 避险止损 (<0.7), got fight=%.2f flee=%.2f", mDefFight, mDefFlee)
	}
	mFeudFight, _ := computeRelationStanceMultiplier("强硬反制", -4, false, false, false, true, false)
	if mFeudFight <= 1.4 {
		t.Fatalf("expected blood-feud enemy to boost 强硬反制 (>1.4), got %.2f", mFeudFight)
	}

	// 2. End-to-end witness protective loyalty across 60 seeds: witnessing attack on your apprentice vs stranger.
	runWitness := func(isApprentice bool) int {
		fights := 0
		for seed := int64(1); seed <= 60; seed++ {
			sel := NewRulebookSelector(rb, nil)
			sel.Seed = seed
			mems := []sim.Memory{
				{EventID: 200, Tick: 20, Kind: "attacked", Source: "raider", TargetID: "victim", Text: "raider attacked victim"},
			}
			if isApprentice {
				mems = append([]sim.Memory{
					{EventID: 50, Tick: 5, Kind: "mentorship_formed", Source: "mentor", TargetID: "victim", Text: "mentor formed bond with victim"},
				}, mems...)
			}
			p := sim.Perception{
				Tick:    21,
				Clock:   sim.Clock{Day: 1, Label: "10:15"},
				Catalog: sim.DefaultCatalog(),
				Self: sim.Actor{
					ID:          "mentor",
					Name:        "师父",
					Role:        "sword_cultivator",
					Personality: "stoic",
					Health:      100,
					Energy:      90,
					Trust:       map[string]int{},
					Memories:    mems,
				},
				Nearby: []sim.Neighbor{
					{ID: "raider", Name: "劫匪", Role: "rogue_raider", Health: 90},
					{ID: "victim", Name: "徒弟", Role: "spirit_farmer", Health: 60},
				},
			}
			opts := []Option{
				{ID: "opt_attack", Action: sim.Action{Kind: "attack", TargetID: "raider"}},
				{ID: "opt_move", Action: sim.Action{Kind: "move", Location: "safe"}},
				{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
			}
			optID, err := sel.Select(context.Background(), p, opts)
			if err != nil {
				t.Fatalf("Select error: %v", err)
			}
			if optID == "opt_attack" {
				fights++
			}
		}
		return fights
	}

	defendApprentice := runWitness(true)
	defendStranger := runWitness(false)
	if defendApprentice <= defendStranger+8 {
		t.Fatalf("expected mentor witnessing attack on apprentice to fight back far more often than for stranger: apprentice=%d/60 stranger=%d/60", defendApprentice, defendStranger)
	}
}

func TestRulebookSelector_DeathNewsAndWorldEventReactions(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "env-death-test",
		PersonalityStanceWeights: map[string]map[string]float64{
			"timid":     {"避险止损": 2.4, "趁机谋利": 0.4, "秉公处置": 0.6},
			"ambitious": {"趁机谋利": 2.2, "避险止损": 0.4},
		},
	}

	// 1. Witnessing a death news memory triggers death-news relay option in RulebookSelector.
	sel := NewRulebookSelector(rb, nil)
	sel.Seed = 77
	pDeath := sim.Perception{
		Tick:    30,
		Clock:   sim.Clock{Day: 1, Label: "13:30"},
		Catalog: sim.DefaultCatalog(),
		Self: sim.Actor{
			ID:          "witness",
			Name:        "目击者",
			Role:        "market_steward",
			Personality: "sociable",
			Health:      100,
			Energy:      90,
			Memories: []sim.Memory{
				{
					EventID: 301,
					Tick:    28,
					Kind:    "actor_died",
					Source:  "victim",
					Observation: &sim.Observation{
						Kind:      "actor_died",
						SubjectID: "killer",
						ObjectID:  "victim",
						Item:      "combat",
						Location:  "valley_bazaar",
					},
				},
			},
		},
	}
	deathOpts := []Option{
		{
			ID: "opt_relay_death",
			Action: sim.Action{
				Kind:         "speak",
				TargetID:     "elder",
				BasisEventID: 301,
				Statement:    &sim.Statement{Topic: "actor_died", SubjectID: "killer", ObjectID: "victim", Item: "combat"},
			},
		},
		{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
	}
	chosen, err := sel.Select(context.Background(), pDeath, deathOpts)
	if err != nil || chosen != "opt_relay_death" {
		t.Fatalf("expected RulebookSelector to relay witnessed death news, got chosen=%q err=%v", chosen, err)
	}

	// 2. Witnessing an artifact_emergence world event prompts an ambitious actor to claim_world_event.
	sel2 := NewRulebookSelector(rb, nil)
	sel2.Seed = 88
	pWev := sim.Perception{
		Tick:    40,
		Clock:   sim.Clock{Day: 1, Label: "16:00"},
		Catalog: sim.DefaultCatalog(),
		Self: sim.Actor{
			ID:          "seeker",
			Name:        "寻宝修士",
			Role:        "sword_cultivator",
			Personality: "ambitious",
			Health:      100,
			Energy:      90,
			Memories: []sim.Memory{
				{
					EventID: 401,
					Tick:    39,
					Kind:    "world_event_triggered",
					Source:  "world",
					Observation: &sim.Observation{
						Kind:      "world_event",
						SubjectID: "ancient_spirit_treasure@39",
						ObjectID:  "artifact_emergence",
						Location:  "spirit_terraces",
					},
				},
			},
		},
	}
	wevOpts := []Option{
		{ID: "opt_claim", Action: sim.Action{Kind: "claim_world_event", TargetID: "ancient_spirit_treasure@39"}},
		{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
	}
	chosen2, err := sel2.Select(context.Background(), pWev, wevOpts)
	if err != nil || chosen2 != "opt_claim" {
		t.Fatalf("expected ambitious actor to choose claim_world_event on artifact_emergence, got chosen=%q err=%v", chosen2, err)
	}
}

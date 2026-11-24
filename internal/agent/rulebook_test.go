package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func buildTestRulebook() *InteractionRulebook {
	return &InteractionRulebook{
		WorldID: "test_generic_world",
		Theme:   "generic_test",
		ProactiveRules: []InteractionRule{
			{
				ID:            "pro_rogue_attack_merchant",
				Domain:        "shadow_conflict",
				Name:          "散修劫掠商客",
				Category:      "冲突",
				Priority:      "high",
				Weight:        50,
				CooldownTicks: 12,
				Actor1: RoleMatcher{
					AnyTags: []string{"rogue_outlaw", "attacker"},
				},
				Actor2: RoleMatcher{
					Roles: []string{"merchant", "trader"},
				},
				Behavior: BehaviorSpec{
					EngineKind:      "attack",
					TargetBinding:   "$actor2",
					SummaryTemplate: "{actor1} 逼近 {actor2} 发起突袭",
				},
				ExpectedOutcomes: []string{"attacked"},
				CausalSummary:    "劫掠者对商客动手，引发目标反击与旁观执法者介入",
			},
			{
				ID:            "pro_merchant_offer_peer",
				Domain:        "trade_economy",
				Name:          "商客主动报价交易",
				Category:      "交易",
				Priority:      "medium",
				Weight:        40,
				CooldownTicks: 12,
				Actor1: RoleMatcher{
					Roles: []string{"merchant", "trader"},
				},
				Actor2: RoleMatcher{
					Roles: []string{"*"},
				},
				Behavior: BehaviorSpec{
					EngineKind:      "offer",
					TargetBinding:   "$actor2",
					ItemRole:        "surplus",
					SummaryTemplate: "{actor1} 向 {actor2} 发出物资交易报价",
				},
				ExpectedOutcomes: []string{"trade_offered"},
				CausalSummary:    "商客发起交易报价，对方可接受、还价或拒绝",
			},
		},
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_actor2_under_attack",
				Domain:            "shadow_conflict",
				Name:              "遇袭目标紧急反制或呼救",
				TriggerEventKinds: []string{"attacked"},
				OutcomeClass:      "any",
				Actor1Match:       RoleMatcher{Roles: []string{"*"}},
				ResponderPosition: "actor2",
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				CausalSummary:     "角色2遭角色1袭击后，依据处境呼救或反击",
				Branches: []ReactionBranch{
					{
						ID:     "br_call_out",
						Stance: "借力拉拢",
						Title:  "当场呼救揭发袭击者",
						Weight: 50,
						Behavior: BehaviorSpec{
							EngineKind:      "speak",
							TargetBinding:   "$actor1",
							SummaryTemplate: "{responder} 遭 {actor1} 袭击后当众呼救揭发",
						},
						ExpectedOutcome: "speech",
					},
					{
						ID:     "br_retaliate",
						Stance: "强硬反制",
						Title:  "奋起反击袭击者",
						Weight: 20,
						Behavior: BehaviorSpec{
							EngineKind:      "attack",
							TargetBinding:   "$actor1",
							SummaryTemplate: "{responder} 遭 {actor1} 袭击后立即反击",
						},
						ExpectedOutcome: "attacked",
					},
				},
			},
			{
				ID:                "rxn_witness_enforcer_intervenes",
				Domain:            "enforcement_oversight",
				Name:              "旁观执法者目击暴行介入缉拿",
				TriggerEventKinds: []string{"attacked"},
				OutcomeClass:      "any",
				Actor1Match:       RoleMatcher{Roles: []string{"*"}},
				ResponderPosition: "witness",
				ResponderMatch:    RoleMatcher{AnyTags: []string{"law_enforcer", "enforcer", "guard"}},
				CausalSummary:     "执法者目击角色1袭击角色2，立即当场缉拿或斥责",
				Branches: []ReactionBranch{
					{
						ID:     "br_arrest_attacker",
						Stance: "强硬反制",
						Title:  "当场缉拿行凶者",
						Weight: 70,
						Behavior: BehaviorSpec{
							EngineKind:      "arrest",
							TargetBinding:   "$actor1",
							SummaryTemplate: "目击者 {responder} 见 {actor1} 袭击 {actor2}，当场出手缉拿 {actor1}",
						},
						ExpectedOutcome: "arrested",
					},
					{
						ID:     "br_rebuke_attacker",
						Stance: "谈判妥协",
						Title:  "厉声喝止行凶者",
						Weight: 30,
						Behavior: BehaviorSpec{
							EngineKind:      "speak",
							TargetBinding:   "$actor1",
							SummaryTemplate: "目击者 {responder} 见 {actor1} 袭击 {actor2}，当场喝止",
						},
						ExpectedOutcome: "speech",
					},
				},
			},
		},
	}
}

func makeGenericCastWorld(prefix string, copies int) *sim.World {
	w := sim.Village("rulebook-" + prefix)
	loc := "market_street"
	w.Actors = nil
	for c := 0; c < copies; c++ {
		suffix := ""
		if copies > 1 {
			suffix = string(rune('a' + c))
		}
		w.Actors = append(w.Actors,
			sim.Actor{
				ID:       prefix + "_rogue" + suffix,
				Name:     "流寇" + suffix,
				Role:     "rogue",
				Tags:     []string{"person", "resident", "fighter", "rogue_outlaw", "attacker"},
				Location: loc,
				Health:   100,
				Energy:   95,
				Money:    30,
				Items:    map[string]int{"food": 5, "water": 5},
				Trust:    map[string]int{},
				Attributes: sim.Attributes{
					Strength: 14, Dexterity: 12, Insight: 8, Spirit: 8,
				},
			},
			sim.Actor{
				ID:       prefix + "_merchant" + suffix,
				Name:     "商客" + suffix,
				Role:     "merchant",
				Tags:     []string{"person", "resident", "fighter", "trader", "merchant"},
				Location: loc,
				Health:   100,
				Energy:   95,
				Money:    80,
				Items:    map[string]int{"food": 8, "water": 6, "wood": 6},
				Trust:    map[string]int{},
				Attributes: sim.Attributes{
					Strength: 8, Dexterity: 9, Insight: 12, Spirit: 12,
				},
			},
			sim.Actor{
				ID:       prefix + "_guard" + suffix,
				Name:     "巡捕" + suffix,
				Role:     "guard",
				Tags:     []string{"person", "resident", "fighter", "law_enforcer", "enforcer", "guard", "constable"},
				Location: loc,
				Health:   100,
				Energy:   95,
				Money:    40,
				Items:    map[string]int{"food": 5, "water": 5},
				Trust:    map[string]int{},
				Attributes: sim.Attributes{
					Strength: 15, Dexterity: 12, Insight: 11, Spirit: 11,
				},
			},
		)
	}
	return w
}

func TestRulebookSelector_CausalChainActor1OutcomeActor2AndWitness(t *testing.T) {
	rb := buildTestRulebook()
	w := makeGenericCastWorld("castA", 1)
	sel := NewRulebookSelector(rb, nil)
	sel.Seed = 101
	pipe := Pipeline{Selector: sel}

	// Run 3 real engine ticks:
	// Tick 0: Rogue fires proactive rule -> attacks Merchant -> engine records "attacked" in Merchant (actor2) AND Guard (witness) memories.
	// Tick 1: Merchant reacts as actor2 AND Guard reacts as witness (arrest/speak targeting $actor1 = castA_rogue).
	foundAttacked := false
	for step := 0; step < 3; step++ {
		evs, err := w.Step(context.Background(), pipe)
		if err != nil {
			t.Fatalf("world step %d: %v", step, err)
		}
		for _, ev := range evs {
			if ev.Kind == "attacked" && ev.ActorID == "castA_rogue" && ev.TargetID == "castA_merchant" {
				foundAttacked = true
			}
		}
	}
	if !foundAttacked {
		t.Fatalf("expected engine to execute proactive attack from castA_rogue on castA_merchant")
	}

	snap := sel.SnapshotLog()
	if snap.Stats.ProactiveDecisions < 1 {
		t.Fatalf("expected >=1 proactive decision, got %+v", snap.Stats)
	}
	if snap.Stats.Actor2Reactions < 1 {
		t.Fatalf("expected >=1 actor2 causal reaction, got %+v", snap.Stats)
	}
	if snap.Stats.WitnessReactions < 1 {
		t.Fatalf("expected >=1 witness causal reaction, got stats=%+v guard=(loc=%s house=%s mems=%+v) rogue=(loc=%s house=%s jailed=%d)",
			snap.Stats, w.Actors[2].Location, w.Actors[2].InsideHouse, w.Actors[2].Memories,
			w.Actors[0].Location, w.Actors[0].InsideHouse, w.Actors[0].JailedUntil)
	}
	if snap.Stats.ByTriggerKind["attacked"] < 2 {
		t.Fatalf("expected both actor2 and witness triggered by 'attacked', got %+v", snap.Stats.ByTriggerKind)
	}
	foundWitnessTarget := false
	for _, ev := range snap.Events {
		if ev.Kind == "reaction" && ev.ResponderPosition == "witness" && ev.ResponderID == "castA_guard" && ev.Actor1ID == "castA_rogue" && ev.TargetID == "castA_rogue" {
			foundWitnessTarget = true
		}
	}
	if !foundWitnessTarget {
		t.Fatalf("expected witness castA_guard to dynamically bind $actor1=castA_rogue, events=%+v", snap.Events)
	}
}

func TestRulebookSelector_SameRulebookGeneralizesToRenamedDoubledCast(t *testing.T) {
	rb := buildTestRulebook()

	runCast := func(prefix string, copies int) RulebookLog {
		w := makeGenericCastWorld(prefix, copies)
		sel := NewRulebookSelector(rb, nil)
		sel.Seed = 777
		pipe := Pipeline{Selector: sel}
		for step := 0; step < 30; step++ {
			if _, err := w.Step(context.Background(), pipe); err != nil {
				t.Fatalf("step %d: %v", step, err)
			}
		}
		return sel.SnapshotLog()
	}

	logSmall := runCast("orig", 1)
	logExpanded := runCast("renamed_sect", 3) // 9 brand-new actor IDs!

	smallFired := logSmall.Stats.ProactiveDecisions + logSmall.Stats.ReactionDecisions
	expandedFired := logExpanded.Stats.ProactiveDecisions + logExpanded.Stats.ReactionDecisions
	if smallFired == 0 {
		t.Fatalf("expected rules to fire on original cast, got 0")
	}
	if expandedFired <= smallFired {
		t.Fatalf("expected expanded renamed cast to trigger more rule executions (%d <= %d)",
			expandedFired, smallFired)
	}
	if logExpanded.Stats.UniqueActorPairs < 3 {
		t.Fatalf("expected same rules to bind across >=3 distinct (actor1,actor2) pairs on renamed cast, got %d",
			logExpanded.Stats.UniqueActorPairs)
	}
	if logExpanded.Stats.AvgPairsPerFiredRule < 1.5 {
		t.Fatalf("expected cross-pair rule reuse AvgPairsPerFiredRule >= 1.5, got %.2f (%+v)",
			logExpanded.Stats.AvgPairsPerFiredRule, logExpanded.Stats.ByRuleDistinctPairs)
	}
	// Verify determinism on identical run
	logExpanded2 := runCast("renamed_sect", 3)
	if len(logExpanded.Events) != len(logExpanded2.Events) {
		t.Fatalf("non-deterministic rulebook event count: %d vs %d", len(logExpanded.Events), len(logExpanded2.Events))
	}
	for i := range logExpanded.Events {
		if logExpanded.Events[i].RuleID != logExpanded2.Events[i].RuleID ||
			logExpanded.Events[i].Actor1ID != logExpanded2.Events[i].Actor1ID ||
			logExpanded.Events[i].TargetID != logExpanded2.Events[i].TargetID ||
			logExpanded.Events[i].ExecutedKind != logExpanded2.Events[i].ExecutedKind {
			t.Fatalf("non-deterministic event[%d]: %+v vs %+v", i, logExpanded.Events[i], logExpanded2.Events[i])
		}
	}
}

func TestRulebookSelector_PersonalityModulatesReactionWeights(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "test_personality_ab",
		PersonalityStanceWeights: map[string]map[string]float64{
			"ruthless":   {"强硬反制": 2.4, "避险止损": 0.4},
			"proud":      {"强硬反制": 1.8, "避险止损": 0.5},
			"careful":    {"避险止损": 2.4, "强硬反制": 0.4},
			"benevolent": {"避险止损": 1.8, "强硬反制": 0.4},
		},
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_under_attack",
				Domain:            "shadow_conflict",
				Name:              "遭袭应对",
				TriggerEventKinds: []string{"attacked"},
				OutcomeClass:      "any",
				Actor1Match:       RoleMatcher{Roles: []string{"*"}},
				ResponderPosition: "actor2",
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				Branches: []ReactionBranch{
					{
						ID:     "br_retaliate",
						Stance: "强硬反制",
						Title:  "强硬反击",
						Weight: 30,
						PersonalityWeights: map[string]float64{
							"ruthless": 1.8,
							"careful":  0.5,
						},
						Behavior: BehaviorSpec{
							EngineKind:      "attack",
							TargetBinding:   "$actor1",
							SummaryTemplate: "{responder} 反击 {actor1}",
						},
					},
					{
						ID:     "br_withdraw",
						Stance: "避险止损",
						Title:  "口头周旋避险",
						Weight: 30,
						PersonalityWeights: map[string]float64{
							"careful":  1.8,
							"ruthless": 0.5,
						},
						Behavior: BehaviorSpec{
							EngineKind:      "speak",
							TargetBinding:   "$actor1",
							SummaryTemplate: "{responder} 劝阻并避开 {actor1}",
						},
					},
				},
			},
		},
	}

	runTrial := func(personality string) (hardCount, safeCount, weightedCount int) {
		for trial := 0; trial < 40; trial++ {
			sel := NewRulebookSelector(rb, nil)
			sel.Seed = int64(1000 + trial)
			p := sim.Perception{
				Tick:  10,
				Clock: sim.Clock{Day: 1, Label: "06:00"},
				Self: sim.Actor{
					ID:          "twin_actor",
					Name:        "双生弟子",
					Role:        "disciple",
					Tags:        []string{"person", "resident", "fighter", "disciple"},
					Personality: personality,
					Location:    "market_street",
					Health:      95,
					Energy:      90,
					Money:       50,
					Memories: []sim.Memory{
						{
							Tick:       9,
							EventID:    int64(500 + trial),
							Kind:       "attacked",
							Source:     "attacker_1",
							SourceRole: "rogue",
							TargetID:   "twin_actor",
						},
					},
				},
				Nearby: []sim.Neighbor{
					{ID: "attacker_1", Name: "袭击者", Role: "rogue", Tags: []string{"rogue"}, Health: 90},
				},
			}
			opts := []Option{
				{ID: "opt_attack", Action: sim.Action{Kind: "attack", TargetID: "attacker_1"}},
				{ID: "opt_speak", Action: sim.Action{Kind: "speak", TargetID: "attacker_1"}},
				{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
			}
			chosen, err := sel.Select(context.Background(), p, opts)
			if err != nil {
				t.Fatalf("select failed: %v", err)
			}
			snap := sel.SnapshotLog()
			weightedCount += snap.Stats.PersonalityWeightedReactions
			if chosen == "opt_attack" {
				hardCount++
			} else if chosen == "opt_speak" {
				safeCount++
			}
		}
		return hardCount, safeCount, weightedCount
	}

	ruthlessHard, ruthlessSafe, ruthlessW := runTrial("proud and ruthless")
	carefulHard, carefulSafe, carefulW := runTrial("careful and benevolent")

	if ruthlessW != 40 || carefulW != 40 {
		t.Fatalf("expected all 40 trials weighted by personality, got ruthless=%d careful=%d", ruthlessW, carefulW)
	}
	if ruthlessHard < 30 {
		t.Fatalf("expected 'proud and ruthless' to heavily favor 强硬反制(attack), got hard=%d safe=%d", ruthlessHard, ruthlessSafe)
	}
	if carefulSafe < 30 {
		t.Fatalf("expected 'careful and benevolent' to heavily favor 避险止损(speak), got hard=%d safe=%d", carefulHard, carefulSafe)
	}
}

func TestRulebookSelector_HobbiesModulateProactiveBehaviorWeightsAndSharedHobby(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "test_hobby_ab",
		HobbyActionWeights: map[string]map[string]float64{
			"sword_dueling":     {"spar": 2.5, "offer": 0.4},
			"market_bargaining": {"offer": 2.5, "spar": 0.4},
		},
		ProactiveRules: []InteractionRule{
			{
				ID:                    "pro_spar",
				Domain:                "faction_martial",
				Name:                  "切磋论剑",
				Category:              "切磋",
				Priority:              "medium",
				Weight:                30,
				HobbyWeights:          map[string]float64{"sword_dueling": 2.2, "market_bargaining": 0.4},
				SharedHobbyMultiplier: 2.5,
				CooldownTicks:         6,
				Actor1:                RoleMatcher{Roles: []string{"disciple"}},
				Actor2:                RoleMatcher{Roles: []string{"*"}},
				Behavior: BehaviorSpec{
					EngineKind:      "spar",
					TargetBinding:   "$actor2",
					SummaryTemplate: "{actor1} 邀 {actor2} 切磋",
				},
			},
			{
				ID:                    "pro_offer",
				Domain:                "trade_economy",
				Name:                  "坊市议价",
				Category:              "交易",
				Priority:              "medium",
				Weight:                30,
				HobbyWeights:          map[string]float64{"market_bargaining": 2.2, "sword_dueling": 0.4},
				SharedHobbyMultiplier: 2.5,
				CooldownTicks:         6,
				Actor1:                RoleMatcher{Roles: []string{"disciple"}},
				Actor2:                RoleMatcher{Roles: []string{"*"}},
				Behavior: BehaviorSpec{
					EngineKind:      "offer",
					TargetBinding:   "$actor2",
					SummaryTemplate: "{actor1} 向 {actor2} 报价",
				},
			},
		},
	}

	runTrial := func(hobbies []string) (sparCount, offerCount, sharedPartnerPicks, hobbyWeighted int) {
		for trial := 0; trial < 40; trial++ {
			sel := NewRulebookSelector(rb, nil)
			sel.Seed = int64(2000 + trial)
			p := sim.Perception{
				Tick:  int64(20 + trial),
				Clock: sim.Clock{Day: 1, Label: "09:00"},
				Self: sim.Actor{
					ID:       "twin_pro",
					Name:     "同门双生",
					Role:     "disciple",
					Tags:     []string{"person", "resident", "fighter", "disciple"},
					Hobbies:  hobbies,
					Location: "market_street",
					Health:   95,
					Energy:   95,
					Money:    60,
				},
				Nearby: []sim.Neighbor{
					{ID: "peer_sword", Name: "剑友", Role: "disciple", Tags: []string{"disciple"}, Hobbies: []string{"sword_dueling"}, Health: 95},
					{ID: "peer_plain", Name: "路人", Role: "disciple", Tags: []string{"disciple"}, Hobbies: []string{"tea_gossip"}, Health: 95},
				},
			}
			opts := []Option{
				{ID: "opt_spar_sword", Action: sim.Action{Kind: "spar", TargetID: "peer_sword"}},
				{ID: "opt_spar_plain", Action: sim.Action{Kind: "spar", TargetID: "peer_plain"}},
				{ID: "opt_offer_sword", Action: sim.Action{Kind: "offer", TargetID: "peer_sword"}},
				{ID: "opt_offer_plain", Action: sim.Action{Kind: "offer", TargetID: "peer_plain"}},
				{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
			}
			chosen, err := sel.Select(context.Background(), p, opts)
			if err != nil {
				t.Fatalf("select failed: %v", err)
			}
			snap := sel.SnapshotLog()
			hobbyWeighted += snap.Stats.HobbyWeightedProactives
			switch chosen {
			case "opt_spar_sword":
				sparCount++
				sharedPartnerPicks++
			case "opt_spar_plain":
				sparCount++
			case "opt_offer_sword", "opt_offer_plain":
				offerCount++
			}
		}
		return sparCount, offerCount, sharedPartnerPicks, hobbyWeighted
	}

	duelSpar, duelOffer, duelShared, duelW := runTrial([]string{"sword_dueling"})
	tradeSpar, tradeOffer, _, tradeW := runTrial([]string{"market_bargaining"})

	if duelW != 40 || tradeW != 40 {
		t.Fatalf("expected all 40 proactive decisions weighted by hobbies, got duel=%d trade=%d", duelW, tradeW)
	}
	if duelSpar < 32 {
		t.Fatalf("expected sword_dueling hobby to heavily favor spar, got spar=%d offer=%d", duelSpar, duelOffer)
	}
	if tradeOffer < 32 {
		t.Fatalf("expected market_bargaining hobby to heavily favor offer, got spar=%d offer=%d", tradeSpar, tradeOffer)
	}
	if duelShared < int(float64(duelSpar)*0.60) {
		t.Fatalf("expected shared hobby partner peer_sword to be chosen significantly more often, got shared=%d/%d", duelShared, duelSpar)
	}
}

func TestRulebookSelector_MultiRuleBranchPoolingNoPreemption(t *testing.T) {
	// Rule A has slightly higher specificity (any_tags:["cultivator"]) and only offers 积极呼应.
	// Rule B has roles:["*"] and offers 暗中周旋 heavily boosted by cunning/skeptical.
	// Previously Rule A permanently preempted Rule B; with pooled branch competition, a cunning actor
	// must overwhelmingly choose 暗中周旋 from Rule B!
	rb := &InteractionRulebook{
		WorldID: "test_pooling",
		PersonalityStanceWeights: map[string]map[string]float64{
			"cunning":   {"暗中周旋": 3.0, "积极呼应": 0.4},
			"skeptical": {"暗中周旋": 2.4, "积极呼应": 0.5},
		},
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_domain_a_tagged",
				Domain:            "alliance_mentorship",
				Name:              "同门契约邀约",
				TriggerEventKinds: []string{"agreement_proposed"},
				ResponderPosition: "actor2",
				ResponderMatch:    RoleMatcher{AnyTags: []string{"cultivator"}},
				Branches: []ReactionBranch{
					{
						ID:                 "br_accept",
						Stance:             "积极呼应",
						Title:              "爽快接约",
						Weight:             35,
						PersonalityWeights: map[string]float64{"candid": 2.0, "cunning": 0.5},
						Behavior:           BehaviorSpec{EngineKind: "accept_agreement", TargetBinding: "$actor1"},
					},
				},
			},
			{
				ID:                "rxn_domain_b_generic",
				Domain:            "trade_economy",
				Name:              "契约暗中周旋",
				TriggerEventKinds: []string{"agreement_proposed"},
				ResponderPosition: "actor2",
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				Branches: []ReactionBranch{
					{
						ID:                 "br_covert",
						Stance:             "暗中周旋",
						Title:              "暗中传讯摸底",
						Weight:             30,
						PersonalityWeights: map[string]float64{"cunning": 2.4, "skeptical": 2.0},
						Behavior:           BehaviorSpec{EngineKind: "transmit_message", TargetBinding: "$ally"},
					},
				},
			},
		},
	}

	covertPicks := 0
	for trial := 0; trial < 40; trial++ {
		sel := NewRulebookSelector(rb, nil)
		sel.Seed = int64(2000 + trial)
		p := sim.Perception{
			Tick:  20,
			Clock: sim.Clock{Day: 1, Label: "08:00"},
			Self: sim.Actor{
				ID:          "sly_actor",
				Name:        "谋士",
				Role:        "trader",
				Tags:        []string{"person", "resident", "cultivator", "trader"},
				Personality: "cunning and skeptical",
				Location:    "market_street",
				Health:      95,
				Energy:      90,
				Trust:       map[string]int{"ally_1": 3},
				Memories: []sim.Memory{
					{
						Tick:     19,
						EventID:  int64(700 + trial),
						Kind:     "agreement_proposed",
						Source:   "proposer_1",
						TargetID: "sly_actor",
					},
				},
			},
			Nearby: []sim.Neighbor{
				{ID: "proposer_1", Name: "邀约者", Role: "trader", Tags: []string{"cultivator"}, Health: 95},
				{ID: "ally_1", Name: "密友", Role: "trader", Tags: []string{"cultivator"}, Health: 95},
			},
		}
		opts := []Option{
			{ID: "opt_accept", Action: sim.Action{Kind: "accept_agreement", TargetID: "proposer_1"}},
			{ID: "opt_msg", Action: sim.Action{Kind: "transmit_message", TargetID: "ally_1"}},
			{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
		}
		chosen, err := sel.Select(context.Background(), p, opts)
		if err != nil {
			t.Fatalf("select failed: %v", err)
		}
		if chosen == "opt_msg" {
			covertPicks++
		}
	}
	if covertPicks < 32 {
		t.Fatalf("expected pooled multi-rule selection to let cunning/skeptical pick 暗中周旋 >=32/40 times, got %d/40", covertPicks)
	}
}

func TestRulebookSelector_PersonalityEncounterModifiersScaleEventFrequency(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "test_encounter_mod",
		PersonalityEncounterModifiers: map[string]EncounterProfile{
			"flamboyant":  {ProactiveRate: 1.8, TargetAttraction: 1.9, DirectEngage: 1.4, WitnessEngage: 1.6},
			"meddlesome":  {ProactiveRate: 1.5, TargetAttraction: 1.4, DirectEngage: 1.5, WitnessEngage: 2.0},
			"reclusive":   {ProactiveRate: 0.4, TargetAttraction: 0.35, DirectEngage: 0.45, WitnessEngage: 0.25},
			"low_profile": {ProactiveRate: 0.6, TargetAttraction: 0.45, DirectEngage: 0.65, WitnessEngage: 0.45},
		},
		ProactiveRules: []InteractionRule{
			{
				ID:            "pro_chat",
				Domain:        "alliance_mentorship",
				Name:          "主动寒暄",
				Category:      "结盟",
				Priority:      "medium",
				Weight:        40,
				CooldownTicks: 24,
				Actor1:        RoleMatcher{Roles: []string{"*"}},
				Actor2:        RoleMatcher{Roles: []string{"*"}},
				Behavior:      BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor2", SummaryTemplate: "$actor1 与 $actor2 搭话"},
			},
		},
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_witness_speech",
				Domain:            "alliance_mentorship",
				Name:              "旁观插话",
				TriggerEventKinds: []string{"speech"},
				ResponderPosition: "witness",
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				Branches: []ReactionBranch{
					{
						ID:       "br_chime",
						Stance:   "借力拉拢",
						Title:    "顺势插话",
						Weight:   40,
						Behavior: BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor1"},
					},
				},
			},
		},
	}

	// 1. Proactive cadence: flamboyant+meddlesome vs reclusive+low_profile over 200 ticks.
	countProactive := func(personality string) int {
		sel := NewRulebookSelector(rb, nil)
		sel.Seed = 42
		opts := []Option{
			{ID: "opt_speak", Action: sim.Action{Kind: "speak", TargetID: "peer_1"}},
			{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
		}
		for tick := int64(1); tick <= 200; tick++ {
			p := sim.Perception{
				Tick:  tick,
				Clock: sim.Clock{Day: 1, Label: "08:00"},
				Self: sim.Actor{
					ID: "a_test", Name: "测试者", Role: "disciple",
					Tags: []string{"person", "resident", "disciple"}, Personality: personality,
					Location: "market_street", Health: 95, Energy: 90,
				},
				Nearby: []sim.Neighbor{
					{ID: "peer_1", Name: "同道", Role: "disciple", Tags: []string{"disciple"}, Health: 95},
				},
			}
			_, _ = sel.Select(context.Background(), p, opts)
		}
		return sel.SnapshotLog().Stats.ProactiveDecisions
	}
	highPro := countProactive("flamboyant and meddlesome")
	lowPro := countProactive("reclusive and low_profile")
	if highPro < lowPro*2 {
		t.Fatalf("expected high-exposure proactive count >=2x low-exposure, got high=%d low=%d", highPro, lowPro)
	}

	// 2. Target attraction: a neutral actor picks between a flamboyant target and a reclusive target.
	flamTargets, recTargets := 0, 0
	for trial := 0; trial < 50; trial++ {
		sel := NewRulebookSelector(rb, nil)
		sel.Seed = int64(3000 + trial)
		p := sim.Perception{
			Tick:  10,
			Clock: sim.Clock{Day: 1, Label: "08:00"},
			Self: sim.Actor{
				ID: "neutral_a", Name: "路人甲", Role: "disciple",
				Tags: []string{"person", "resident", "disciple"}, Personality: "stoic",
				Location: "market_street", Health: 95, Energy: 90,
			},
			Nearby: []sim.Neighbor{
				{ID: "target_flam", Name: "张扬者", Role: "disciple", Tags: []string{"disciple"}, Personality: "flamboyant and meddlesome", Health: 95},
				{ID: "target_rec", Name: "隐世者", Role: "disciple", Tags: []string{"disciple"}, Personality: "reclusive and low_profile", Health: 95},
			},
		}
		opts := []Option{
			{ID: "opt_flam", Action: sim.Action{Kind: "speak", TargetID: "target_flam"}},
			{ID: "opt_rec", Action: sim.Action{Kind: "speak", TargetID: "target_rec"}},
			{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
		}
		chosen, _ := sel.Select(context.Background(), p, opts)
		if chosen == "opt_flam" {
			flamTargets++
		} else if chosen == "opt_rec" {
			recTargets++
		}
	}
	if flamTargets < 32 {
		t.Fatalf("expected flamboyant target to attract far more approaches (>=32/50), got flam=%d rec=%d", flamTargets, recTargets)
	}

	// 3. Witness engagement gate: meddlesome vs reclusive observing 50 witnessed speech events.
	countWitness := func(personality string) (engaged, suppressed int) {
		for trial := 0; trial < 50; trial++ {
			sel := NewRulebookSelector(rb, nil)
			sel.Seed = int64(4000 + trial)
			p := sim.Perception{
				Tick:  20,
				Clock: sim.Clock{Day: 1, Label: "09:00"},
				Self: sim.Actor{
					ID: "obs_actor", Name: "旁观者", Role: "disciple",
					Tags: []string{"person", "resident", "disciple"}, Personality: personality,
					Location: "market_street", Health: 95, Energy: 90,
					Memories: []sim.Memory{
						{Tick: 19, EventID: int64(900 + trial), Kind: "speech", Source: "spk_1", TargetID: "spk_2"},
					},
				},
				Nearby: []sim.Neighbor{
					{ID: "spk_1", Name: "甲", Role: "disciple", Tags: []string{"disciple"}, Health: 95},
					{ID: "spk_2", Name: "乙", Role: "disciple", Tags: []string{"disciple"}, Health: 95},
				},
			}
			opts := []Option{
				{ID: "opt_chime", Action: sim.Action{Kind: "speak", TargetID: "spk_1"}},
				{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
			}
			_, _ = sel.Select(context.Background(), p, opts)
			st := sel.SnapshotLog().Stats
			engaged += st.WitnessReactions
			suppressed += st.SuppressedWitnessReactions
		}
		return engaged, suppressed
	}
	medEng, medSup := countWitness("flamboyant and meddlesome")
	recEng, recSup := countWitness("reclusive and low_profile")
	if medEng < 38 || recSup < 28 || recEng > 22 {
		t.Fatalf("expected meddlesome to engage most witness events and reclusive to suppress most: med=(eng=%d,sup=%d) rec=(eng=%d,sup=%d)",
			medEng, medSup, recEng, recSup)
	}
}

func TestRulebookSelector_ProfessionAndPersonalityEncounterDecoupled(t *testing.T) {
	rb := &InteractionRulebook{
		WorldID: "test_prof_pers_decoupled",
		ProfessionEncounterModifiers: map[string]EncounterProfile{
			"merchant": {ProactiveRate: 1.75, TargetAttraction: 1.80, DirectEngage: 1.40, WitnessEngage: 1.35},
			"farmer":   {ProactiveRate: 0.48, TargetAttraction: 0.48, DirectEngage: 0.72, WitnessEngage: 0.48},
		},
		PersonalityEncounterModifiers: map[string]EncounterProfile{
			"flamboyant": {ProactiveRate: 1.45, TargetAttraction: 1.50, DirectEngage: 1.30, WitnessEngage: 1.35},
			"reclusive":  {ProactiveRate: 0.60, TargetAttraction: 0.60, DirectEngage: 0.65, WitnessEngage: 0.55},
		},
		ProactiveRules: []InteractionRule{
			{
				ID:            "pro_social",
				Name:          "日常寒暄",
				Category:      "结盟",
				Priority:      "medium",
				Weight:        30,
				CooldownTicks: 24,
				Actor1:        RoleMatcher{Roles: []string{"*"}},
				Actor2:        RoleMatcher{Roles: []string{"*"}},
				Behavior:      BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor2"},
			},
		},
	}

	countPro := func(role, personality string) int {
		sel := NewRulebookSelector(rb, nil)
		sel.Seed = 777
		opts := []Option{
			{ID: "opt_speak", Action: sim.Action{Kind: "speak", TargetID: "peer_1"}},
			{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
		}
		for tick := int64(1); tick <= 360; tick++ {
			p := sim.Perception{
				Tick:  tick,
				Clock: sim.Clock{Day: int(tick/96) + 1, Label: "10:00"},
				Self: sim.Actor{
					ID: "a_test", Name: "测试者", Role: role,
					Tags: []string{"person", "resident", role}, Personality: personality,
					Location: "loc_1", Health: 95, Energy: 90,
				},
				Nearby: []sim.Neighbor{
					{ID: "peer_1", Name: "同道", Role: "merchant", Tags: []string{"merchant"}, Health: 95},
				},
			}
			_, _ = sel.Select(context.Background(), p, opts)
		}
		return sel.SnapshotLog().Stats.ProactiveDecisions
	}

	reclusiveMerchant := countPro("merchant", "reclusive")
	reclusiveFarmer := countPro("farmer", "reclusive")
	flamboyantFarmer := countPro("farmer", "flamboyant")
	flamboyantMerchant := countPro("merchant", "flamboyant")

	// 1. Fixed personality (reclusive): merchant structurally interacts far more than farmer!
	if reclusiveMerchant <= reclusiveFarmer*2 {
		t.Fatalf("expected reclusive merchant >> reclusive farmer (>=2x due to profession baseline), got merch=%d farmer=%d",
			reclusiveMerchant, reclusiveFarmer)
	}
	// 2. Fixed profession (farmer): flamboyant farmer interacts far more than reclusive farmer!
	if flamboyantFarmer < int(float64(reclusiveFarmer)*1.8) {
		t.Fatalf("expected flamboyant farmer >> reclusive farmer (within-profession personality effect), got flam=%d rec=%d",
			flamboyantFarmer, reclusiveFarmer)
	}
	// 3. Anti-stereotype sanity: even a reclusive merchant has higher baseline exposure than a reclusive farmer
	//    and flamboyant merchant is the highest overall.
	if flamboyantMerchant <= reclusiveMerchant || flamboyantMerchant <= flamboyantFarmer {
		t.Fatalf("expected flamboyant merchant to be highest overall: fm=%d rm=%d ff=%d rf=%d",
			flamboyantMerchant, reclusiveMerchant, flamboyantFarmer, reclusiveFarmer)
	}
}

func TestRulebookSelector_PowerTierSuppressionAndSkillAffinity(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.Forms = map[string]sim.FormSpec{
		"qi_refining":  {ID: "qi_refining", Name: "炼气期", PowerTier: 1, CombatBonus: 6},
		"nascent_soul": {ID: "nascent_soul", Name: "元婴期", PowerTier: 4, CombatBonus: 95},
	}
	cat.Skills = map[string]sim.SkillSpec{
		"sword_art":   {ID: "sword_art", Name: "剑道", CombatBonusPerLevel: 4, ActionAffinities: []string{"spar"}},
		"trade_craft": {ID: "trade_craft", Name: "商道", TradeMarginPerLevel: 2, ActionAffinities: []string{"offer"}},
	}

	rb := &InteractionRulebook{
		WorldID: "test_power_tier",
		ReactionRules: []CausalReactionRule{
			{
				ID:                "rxn_clash",
				Domain:            "shadow_conflict",
				Name:              "遭遇挑衅应对",
				TriggerEventKinds: []string{"attacked"},
				ResponderPosition: "actor2",
				ResponderMatch:    RoleMatcher{Roles: []string{"*"}},
				Branches: []ReactionBranch{
					{
						ID:       "br_fight",
						Stance:   "强硬反制",
						Title:    "正面反击",
						Weight:   50,
						Behavior: BehaviorSpec{EngineKind: "attack", TargetBinding: "$actor1"},
					},
					{
						ID:       "br_flee",
						Stance:   "避险止损",
						Title:    "退避保命",
						Weight:   50,
						Behavior: BehaviorSpec{EngineKind: "move", TargetBinding: "$actor1"},
					},
				},
			},
		},
	}

	runRxn := func(selfForm, otherForm string, otherTier, otherPower int) (fight, flee int) {
		for trial := 0; trial < 60; trial++ {
			sel := NewRulebookSelector(rb, nil)
			sel.Seed = int64(5000 + trial)
			p := sim.Perception{
				Tick:    20,
				Clock:   sim.Clock{Day: 1, Label: "10:00"},
				Catalog: cat,
				Self: sim.Actor{
					ID:          "resp",
					Name:        "应战者",
					Role:        "sword_cultivator",
					FormID:      selfForm,
					Tags:        []string{"person", "resident", "fighter"},
					Personality: "stoic",
					Location:    "arena",
					Health:      95,
					Energy:      90,
					Memories: []sim.Memory{
						{EventID: 101, Tick: 19, Kind: "attacked", Source: "opp", TargetID: "resp", Text: "attacked"},
					},
				},
				Nearby: []sim.Neighbor{
					{ID: "opp", Name: "对手", Role: "sword_cultivator", FormID: otherForm, PowerTier: otherTier, CombatPower: otherPower, Health: 95},
				},
			}
			opts := []Option{
				{ID: "opt_attack", Action: sim.Action{Kind: "attack", TargetID: "opp"}},
				{ID: "opt_move", Action: sim.Action{Kind: "move", Location: "safe_zone"}},
				{ID: "opt_wait", Action: sim.Action{Kind: "wait"}},
			}
			chosen, err := sel.Select(context.Background(), p, opts)
			if err != nil {
				t.Fatalf("select failed: %v", err)
			}
			if chosen == "opt_attack" {
				fight++
			} else if chosen == "opt_move" {
				flee++
			}
		}
		return fight, flee
	}

	// Qi Refining facing Nascent Soul (-3 tier gap): overwhelmingly flees instead of fighting head-on!
	weakFight, weakFlee := runRxn("qi_refining", "nascent_soul", 4, 120)
	// Nascent Soul facing Qi Refining (+3 tier gap): overwhelmingly crushes head-on instead of fleeing!
	strongFight, strongFlee := runRxn("nascent_soul", "qi_refining", 1, 20)

	if weakFlee < 48 || weakFight > 12 {
		t.Fatalf("expected qi_refining facing nascent_soul to overwhelmingly flee, got fight=%d flee=%d", weakFight, weakFlee)
	}
	if strongFight < 48 || strongFlee > 12 {
		t.Fatalf("expected nascent_soul facing qi_refining to overwhelmingly confront, got fight=%d flee=%d", strongFight, strongFlee)
	}

	// Verify skill proactive multiplier boosts matching actions cleanly.
	mSword, reason := computeSkillProactiveMultiplier(cat, map[string]int{"sword_art": 5}, "spar")
	if mSword < 1.5 || reason == "" {
		t.Fatalf("expected sword_art Lv.5 to boost spar >=1.5x, got %.2f (%q)", mSword, reason)
	}
	mNone, _ := computeSkillProactiveMultiplier(cat, map[string]int{"sword_art": 5}, "offer")
	if mNone != 1.0 {
		t.Fatalf("expected sword_art not to boost unrelated offer action, got %.2f", mNone)
	}
}

func TestRulebookParallelDecisionsUseEachObserversKnowledge(t *testing.T) {
	book := &InteractionRulebook{WorldID: "observer-local", ProactiveRules: []InteractionRule{{
		ID: "contact", Priority: "high", Weight: 30,
		Actor1:   RoleMatcher{Roles: []string{"caller"}},
		Actor2:   RoleMatcher{Roles: []string{"contact"}},
		Behavior: BehaviorSpec{EngineKind: "transmit_message", TargetBinding: "$actor2"},
	}}}
	caller := testPerception("caller", "Caller", "here", 8, nil, 0, 0, 100, 100)
	caller.Self.Role = "caller"
	caller.Self.Trust = map[string]int{"peer": 3}
	peer := testPerception("peer", "Peer", "elsewhere", 8, nil, 0, 0, 100, 100)
	peer.Self.Role = "contact"
	options := []Option{
		{ID: "wait", Action: sim.Action{Kind: "wait"}},
		{ID: "contact", Action: sim.Action{Kind: "transmit_message", TargetID: "peer", Text: "Hello"}},
	}

	first := NewRulebookSelector(book, nil)
	if _, err := first.Select(context.Background(), peer, options[:1]); err != nil {
		t.Fatal(err)
	}
	expected, err := first.Select(context.Background(), caller, options)
	if err != nil || expected != "wait" {
		t.Fatalf("caller used another actor's private profile: %q, %v", expected, err)
	}

	for range 12 {
		selector := NewRulebookSelector(book, nil)
		done := make(chan error, 1)
		go func() {
			_, err := selector.Select(context.Background(), peer, options[:1])
			done <- err
		}()
		got, err := selector.Select(context.Background(), caller, options)
		if peerErr := <-done; peerErr != nil {
			t.Fatal(peerErr)
		}
		if err != nil || got != expected {
			t.Fatalf("parallel selection = %q, %v; serial selection = %q", got, err, expected)
		}
	}

	caller.Nearby = []sim.Neighbor{{ID: "peer", Role: "contact", Name: "Peer", Health: 100}}
	if got, err := NewRulebookSelector(book, nil).Select(context.Background(), caller, options); err != nil || got != "contact" {
		t.Fatalf("caller ignored a personally observed contact: %q, %v", got, err)
	}
}

func TestCommunicationOptionMatchesTheExactCitedStatement(t *testing.T) {
	death := &sim.Statement{Topic: "actor_died", SubjectID: "offender", ObjectID: "victim"}
	options := []Option{
		{ID: "supply", Action: sim.Action{Kind: "speak", TargetID: "listener", BasisEventID: 10, Statement: &sim.Statement{Topic: "supply"}}},
		{ID: "incorrect-citation", Action: sim.Action{Kind: "speak", TargetID: "listener", BasisEventID: 11, Statement: death}},
		{ID: "death", Action: sim.Action{Kind: "speak", TargetID: "listener", BasisEventID: 12, Statement: death}},
	}
	desired := sim.Action{Kind: "speak", TargetID: "listener", BasisEventID: 12, Statement: death}
	if got := ruleOptionID(desired, options); got != "death" {
		t.Fatalf("requested death report selected %q", got)
	}
}

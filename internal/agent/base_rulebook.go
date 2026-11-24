package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func isIntrigueOrCourtGenre(genre string) bool {
	g := strings.ToLower(strings.TrimSpace(genre))
	for _, kw := range []string{"court", "intrigue", "historical", "imperial", "official", "noir", "business", "corporate", "palace", "crime", "mystery", "yamen"} {
		if strings.Contains(g, kw) {
			return true
		}
	}
	return false
}

// DefaultBaseRulebook synthesizes a built-in, zero-LLM, genre-aware InteractionRulebook
// covering core social, economic, diplomatic, party, progression, and martial interactions.
// External models only need to supply a small overlay JSON (or none at all) to customize a novel world.
func DefaultBaseRulebook(w *sim.World) *InteractionRulebook {
	genre := ""
	if w != nil {
		genre = w.Catalog.EffectiveEnvironment().Genre
	}
	rb := &InteractionRulebook{
		WorldID: "builtin",
		Glossaries: PlaybookGlossaries{
			Locations:   map[string]string{},
			Professions: map[string]string{},
			Items:       map[string]string{},
			Factions:    map[string]string{},
			Recipes:     map[string]string{},
			Actors:      map[string]string{},
		},
		PersonalityStanceWeights: defaultPersonalityStanceWeights(),
		ProactiveRules:           defaultBaseProactiveRules(genre),
		ReactionRules:            defaultBaseReactionRules(genre),
	}
	if w != nil {
		rb.WorldID = w.ID
		env := w.Catalog.EffectiveEnvironment()
		rb.Environment = env
		rb.Genre = env.Genre
		rb.Theme = env.Theme
		for _, d := range w.Map.Districts {
			rb.Glossaries.Locations[d.ID] = d.Name
		}
		for _, p := range w.Catalog.Professions {
			rb.Glossaries.Professions[p.ID] = p.Name
		}
		for id, it := range w.Catalog.Items {
			rb.Glossaries.Items[id] = it.Name
		}
		for _, f := range w.Factions {
			rb.Glossaries.Factions[f.ID] = f.Name
		}
		for _, r := range w.Catalog.Recipes {
			rb.Glossaries.Recipes[r.ID] = r.Label
		}
		// Auto-register combat ActionSpecs defined by the world catalog so custom skills fire in battle.
		for id, spec := range w.Catalog.ActionSpecs {
			if spec.Attack != nil && spec.TargetKind == "actor" {
				actID := spec.ID
				if actID == "" {
					actID = id
				}
				name := spec.Name
				if name == "" {
					name = actID
				}
				rb.ProactiveRules = append(rb.ProactiveRules, InteractionRule{
					ID: "base_pro_spec_" + actID, Domain: "conflict", Name: name, Category: "战技交锋",
					Priority: "urgent", Weight: 46, CooldownTicks: 6,
					Actor1: RoleMatcher{MinHealth: intPtr(25)}, Actor2: RoleMatcher{},
					Relation:         RelationPredicate{MaxTrust: intPtr(-2), FactionRelation: "different", CoLocated: boolPtr(true)},
					Behavior:         BehaviorSpec{EngineKind: actID, TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}对{actor2}施展" + name},
					ExpectedOutcomes: []string{"attacked", "actor_died"},
				})
			}
		}
	}
	return rb
}

// MergeRulebookOverlay merges a lightweight narrative overlay onto base without mutating either input.
// Rules with matching IDs replace base rules; new IDs are appended. Maps are merged per key.
func MergeRulebookOverlay(base, overlay *InteractionRulebook) *InteractionRulebook {
	if base == nil && overlay == nil {
		return nil
	}
	if base == nil {
		cp := *overlay
		return &cp
	}
	out := *base
	if overlay == nil {
		return &out
	}
	if overlay.WorldID != "" {
		out.WorldID = overlay.WorldID
	}
	if overlay.Theme != "" {
		out.Theme = overlay.Theme
	}
	if overlay.Genre != "" {
		out.Genre = overlay.Genre
	}

	out.PersonalityStanceWeights = mergeNestedFloatMap(base.PersonalityStanceWeights, overlay.PersonalityStanceWeights)
	out.HobbyActionWeights = mergeNestedFloatMap(base.HobbyActionWeights, overlay.HobbyActionWeights)
	out.ProfessionEncounterModifiers = mergeEncounterMap(base.ProfessionEncounterModifiers, overlay.ProfessionEncounterModifiers)
	out.PersonalityEncounterModifiers = mergeEncounterMap(base.PersonalityEncounterModifiers, overlay.PersonalityEncounterModifiers)

	proIdx := make(map[string]int, len(base.ProactiveRules))
	out.ProactiveRules = append([]InteractionRule(nil), base.ProactiveRules...)
	for i, r := range out.ProactiveRules {
		proIdx[r.ID] = i
	}
	for _, r := range overlay.ProactiveRules {
		if idx, ok := proIdx[r.ID]; ok && r.ID != "" {
			out.ProactiveRules[idx] = r
		} else {
			proIdx[r.ID] = len(out.ProactiveRules)
			out.ProactiveRules = append(out.ProactiveRules, r)
		}
	}

	rxnIdx := make(map[string]int, len(base.ReactionRules))
	out.ReactionRules = append([]CausalReactionRule(nil), base.ReactionRules...)
	for i, r := range out.ReactionRules {
		rxnIdx[r.ID] = i
	}
	for _, r := range overlay.ReactionRules {
		if idx, ok := rxnIdx[r.ID]; ok && r.ID != "" {
			out.ReactionRules[idx] = r
		} else {
			rxnIdx[r.ID] = len(out.ReactionRules)
			out.ReactionRules = append(out.ReactionRules, r)
		}
	}
	return &out
}

// LoadRulebookWithOverlay loads either the built-in base rulebook ("", "builtin", "default")
// or an explicit rulebook file/dir, then optionally merges an overlay JSON file on top.
func LoadRulebookWithOverlay(rulebookPath, overlayPath string, w *sim.World) (*InteractionRulebook, error) {
	var base *InteractionRulebook
	trimmed := strings.TrimSpace(rulebookPath)
	if trimmed == "" || strings.EqualFold(trimmed, "builtin") || strings.EqualFold(trimmed, "default") {
		base = DefaultBaseRulebook(w)
	} else {
		var err error
		base, err = LoadRulebook(trimmed)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(overlayPath) == "" {
		return base, nil
	}
	raw, err := os.ReadFile(strings.TrimSpace(overlayPath))
	if err != nil {
		return nil, fmt.Errorf("read rulebook overlay %s: %w", overlayPath, err)
	}
	var overlay InteractionRulebook
	if err := json.Unmarshal(raw, &overlay); err != nil {
		return nil, fmt.Errorf("parse rulebook overlay %s: %w", overlayPath, err)
	}
	return MergeRulebookOverlay(base, &overlay), nil
}

func mergeNestedFloatMap(base, overlay map[string]map[string]float64) map[string]map[string]float64 {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]map[string]float64, len(base)+len(overlay))
	for k, sub := range base {
		cp := make(map[string]float64, len(sub))
		for sk, v := range sub {
			cp[sk] = v
		}
		out[k] = cp
	}
	for k, sub := range overlay {
		if out[k] == nil {
			out[k] = make(map[string]float64, len(sub))
		}
		for sk, v := range sub {
			out[k][sk] = v
		}
	}
	return out
}

func mergeEncounterMap(base, overlay map[string]EncounterProfile) map[string]EncounterProfile {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]EncounterProfile, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

func defaultPersonalityStanceWeights() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"ruthless":      {"强硬反制": 2.4, "趁机谋利": 1.6, "谈判妥协": 0.4, "避险止损": 0.5},
		"vengeful":      {"强硬反制": 2.6, "暗中周旋": 1.4, "谈判妥协": 0.4},
		"proud":         {"强硬反制": 1.9, "谈判妥协": 0.5, "避险止损": 0.6},
		"reckless":      {"强硬反制": 2.1, "积极呼应": 1.3, "避险止损": 0.4},
		"righteous":     {"秉公处置": 2.5, "积极呼应": 1.6, "趁机谋利": 0.3, "暗中周旋": 0.5},
		"dutiful":       {"秉公处置": 2.3, "积极呼应": 1.4, "趁机谋利": 0.4},
		"stern":         {"秉公处置": 2.2, "强硬反制": 1.4, "谈判妥协": 0.6},
		"cunning":       {"暗中周旋": 2.3, "趁机谋利": 1.9, "借力拉拢": 1.6, "强硬反制": 0.7},
		"calculating":   {"趁机谋利": 2.4, "暗中周旋": 1.8, "谈判妥协": 1.4},
		"opportunistic": {"趁机谋利": 2.3, "借力拉拢": 1.7, "避险止损": 1.3},
		"ambitious":     {"借力拉拢": 1.8, "趁机谋利": 1.7, "强硬反制": 1.4},
		"diplomatic":    {"谈判妥协": 2.3, "借力拉拢": 2.0, "强硬反制": 0.5},
		"benevolent":    {"积极呼应": 2.2, "谈判妥协": 1.5, "强硬反制": 0.4, "趁机谋利": 0.3},
		"generous":      {"积极呼应": 2.0, "借力拉拢": 1.7, "趁机谋利": 0.4},
		"sociable":      {"借力拉拢": 1.8, "积极呼应": 1.7, "谈判妥协": 1.4},
		"cautious":      {"避险止损": 2.2, "暗中周旋": 1.5, "强硬反制": 0.5},
		"timid":         {"避险止损": 2.6, "谈判妥协": 1.6, "强硬反制": 0.3},
		"patient":       {"暗中周旋": 2.1, "避险止损": 1.3, "强硬反制": 0.7},
		"inquisitive":   {"秉公处置": 1.6, "暗中周旋": 1.7, "积极呼应": 1.4},
		"skeptical":     {"暗中周旋": 1.9, "避险止损": 1.5, "积极呼应": 0.7},
		"paranoid":      {"强硬反制": 1.6, "暗中周旋": 1.8, "避险止损": 1.7, "积极呼应": 0.4},
	}
}

func defaultBaseProactiveRules(genre string) []InteractionRule {
	intrigue := isIntrigueOrCourtGenre(genre)
	rules := []InteractionRule{
		{
			ID: "base_pro_speak_ally", Domain: "social", Name: "同道交流与互通消息", Category: "人脉结盟",
			Priority: "low", Weight: 16, CooldownTicks: 24,
			Actor1: RoleMatcher{}, Actor2: RoleMatcher{},
			Relation: RelationPredicate{MinTrust: intPtr(0), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}与{actor2}交谈互通近况"},
			ExpectedOutcomes: []string{"speech"},
		},
		{
			ID: "base_pro_swear_bond", Domain: "social", Name: "志同道合立誓结交", Category: "人脉结盟",
			Priority: "high", Weight: 24, CooldownTicks: 48,
			Actor1: RoleMatcher{}, Actor2: RoleMatcher{},
			Relation: RelationPredicate{MinTrust: intPtr(2), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "swear_bond", TargetBinding: "$actor2", SummaryTemplate: "{actor1}与{actor2}在{location}缔结同盟情谊"},
			ExpectedOutcomes: []string{"bond_sworn"},
		},
		{
			ID: "base_pro_transmit_intel", Domain: "social", Name: "远程传讯联络", Category: "人脉结盟",
			Priority: "low", Weight: 14, CooldownTicks: 32,
			Actor1: RoleMatcher{}, Actor2: RoleMatcher{},
			Relation: RelationPredicate{CoLocated: boolPtr(false)},
			Behavior: BehaviorSpec{EngineKind: "transmit_message", TargetBinding: "$actor2", SummaryTemplate: "{actor1}远程传讯联络{actor2}"},
			ExpectedOutcomes: []string{"channel_transmitted", "channel_message"},
		},
		{
			ID: "base_pro_trade_surplus", Domain: "economy", Name: "互通有无与交易要约", Category: "资源交易",
			Priority: "medium", Weight: 18, CooldownTicks: 24,
			Actor1: RoleMatcher{HasSurplusGoods: boolPtr(true)}, Actor2: RoleMatcher{},
			Relation: RelationPredicate{MinTrust: intPtr(-1), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "offer", TargetBinding: "$actor2", ItemRole: "surplus", SummaryTemplate: "{actor1}在{location}向{actor2}提出交易{item}"},
			ExpectedOutcomes: []string{"trade_offered", "trade_completed"},
		},
		{
			ID: "base_pro_avenge_or_clash", Domain: "conflict", Name: "仇敌狭路相逢清算旧怨", Category: "生死血战",
			Priority: "urgent", Weight: 52, CooldownTicks: 6,
			Actor1: RoleMatcher{MinHealth: intPtr(25)}, Actor2: RoleMatcher{},
			Relation: RelationPredicate{MaxTrust: intPtr(-3), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "avenge_feud", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}与宿敌{actor2}爆发血仇清算"},
			ExpectedOutcomes: []string{"feud_avenged", "attacked", "actor_died"},
		},
		{
			ID: "base_pro_hostile_attack", Domain: "conflict", Name: "敌对阵营正面死战", Category: "生死血战",
			Priority: "urgent", Weight: 42, CooldownTicks: 8,
			Actor1: RoleMatcher{MinHealth: intPtr(30), PersonalityKeywords: []string{"ruthless", "vengeful", "reckless", "provocative", "proud", "ambitious", "dutiful", "stern"}},
			Actor2: RoleMatcher{},
			Relation: RelationPredicate{MaxTrust: intPtr(-2), FactionRelation: "different", CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "attack", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}对敌对人物{actor2}悍然出手"},
			ExpectedOutcomes: []string{"attacked", "actor_died"},
		},
		{
			ID: "base_pro_spar_rivalry", Domain: "conflict", Name: "同道切磋较技", Category: "进阶破局",
			Priority: "medium", Weight: 18, CooldownTicks: 32,
			Actor1: RoleMatcher{MinHealth: intPtr(70), PersonalityKeywords: []string{"competitive", "proud", "candid", "ambitious"}},
			Actor2: RoleMatcher{},
			Relation: RelationPredicate{MinTrust: intPtr(0), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "spar", TargetBinding: "$actor2", SummaryTemplate: "{actor1}邀{actor2}在{location}切磋试锋"},
			ExpectedOutcomes: []string{"sparred"},
		},
		{
			ID: "base_pro_party_betray", Domain: "intrigue", Name: "利欲熏心背刺临时同伙", Category: "竞争博弈",
			Priority: "medium", Weight: 20, CooldownTicks: 64,
			Actor1: RoleMatcher{PersonalityKeywords: []string{"ruthless", "opportunistic"}},
			Actor2: RoleMatcher{},
			Relation: RelationPredicate{MaxTrust: intPtr(0), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "betray_party", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在行动途中突然背刺同行者{actor2}"},
			ExpectedOutcomes: []string{"party_betrayed"},
		},
		{
			ID: "base_pro_heal_wounded", Domain: "support", Name: "救治伤患结下善缘", Category: "人脉结盟",
			Priority: "medium", Weight: 24, CooldownTicks: 20,
			Actor1: RoleMatcher{}, Actor2: RoleMatcher{IsWounded: boolPtr(true)},
			Relation: RelationPredicate{MinTrust: intPtr(1), CoLocated: boolPtr(true)},
			Behavior: BehaviorSpec{EngineKind: "heal", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}出手救治负伤的{actor2}"},
			ExpectedOutcomes: []string{"healed"},
		},
		{
			ID: "base_pro_diplomacy_war", Domain: "diplomacy", Name: "积怨爆发正式宣战", Category: "势力战争",
			Priority: "urgent", Weight: 40, CooldownTicks: 24,
			Actor1: RoleMatcher{AnyTags: []string{"ruler", "commander", "elder_oversight", "enforcer"}},
			Actor2: RoleMatcher{},
			Relation: RelationPredicate{FactionRelation: "different", MaxTrust: intPtr(-2)},
			Behavior: BehaviorSpec{EngineKind: "declare_war", TargetBinding: "$actor2", SummaryTemplate: "{actor1}决意对敌对势力正式宣战"},
			ExpectedOutcomes: []string{"faction_war_declared"},
		},
		{
			ID: "base_pro_diplomacy_ally", Domain: "diplomacy", Name: "合纵连横缔结盟约", Category: "人脉结盟",
			Priority: "high", Weight: 30, CooldownTicks: 48,
			Actor1: RoleMatcher{AnyTags: []string{"ruler", "commander", "elder_oversight", "enforcer"}},
			Actor2: RoleMatcher{},
			Relation: RelationPredicate{FactionRelation: "different", MinTrust: intPtr(1)},
			Behavior: BehaviorSpec{EngineKind: "propose_alliance", TargetBinding: "$actor2", SummaryTemplate: "{actor1}代表势力倡议结盟"},
			ExpectedOutcomes: []string{"alliance_formed"},
		},
	}

	if intrigue {
		rules = append(rules,
			InteractionRule{
				ID: "base_pro_investigate_rival", Domain: "intrigue", Name: "暗中刺探对手底细", Category: "竞争博弈",
				Priority: "high", Weight: 24, CooldownTicks: 48,
				Actor1: RoleMatcher{PersonalityKeywords: []string{"inquisitive", "skeptical", "cunning", "calculating", "ambitious", "righteous", "dutiful"}},
				Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(0), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "investigate_actor", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}暗中调查{actor2}的隐秘把柄"},
				ExpectedOutcomes: []string{"secret_discovered"},
			},
			InteractionRule{
				ID: "base_pro_blackmail_target", Domain: "intrigue", Name: "持柄要挟牟利", Category: "竞争博弈",
				Priority: "high", Weight: 24, CooldownTicks: 72,
				Actor1: RoleMatcher{PersonalityKeywords: []string{"cunning", "calculating", "opportunistic", "ruthless", "ambitious"}},
				Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(-1), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "blackmail", TargetBinding: "$actor2", SummaryTemplate: "{actor1}凭秘辛要挟{actor2}"},
				ExpectedOutcomes: []string{"blackmailed", "blackmail_refused"},
			},
			InteractionRule{
				ID: "base_pro_expose_secret", Domain: "intrigue", Name: "当众揭发隐秘丑闻", Category: "竞争博弈",
				Priority: "high", Weight: 26, CooldownTicks: 56,
				Actor1: RoleMatcher{}, Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(-1), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "expose_secret", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}公然揭发{actor2}的丑事"},
				ExpectedOutcomes: []string{"secret_exposed"},
			},
			InteractionRule{
				ID: "base_pro_bribe_influence", Domain: "intrigue", Name: "暗中行贿拉拢关键人物", Category: "竞争博弈",
				Priority: "medium", Weight: 14, CooldownTicks: 120,
				Actor1: RoleMatcher{MinMoney: intPtr(25), PersonalityKeywords: []string{"cunning", "calculating", "opportunistic", "ambitious", "diplomatic"}},
				Actor2: RoleMatcher{AnyTags: []string{"ruler", "enforcer", "elder_oversight", "law", "censor"}},
				Relation: RelationPredicate{MaxTrust: intPtr(1), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "bribe", TargetBinding: "$actor2", SummaryTemplate: "{actor1}暗中向{actor2}行贿疏通"},
				ExpectedOutcomes: []string{"bribed"},
			},
			InteractionRule{
				ID: "base_pro_enforce_audit", Domain: "order", Name: "执法监察与稽查腐败", Category: "主业经营",
				Priority: "high", Weight: 28, CooldownTicks: 24,
				Actor1: RoleMatcher{AnyTags: []string{"enforcer", "elder_oversight", "law", "censor", "ruler"}},
				Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(0), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "audit_corruption", TargetBinding: "$actor2", SummaryTemplate: "{actor1}对{actor2}展开监察稽核"},
				ExpectedOutcomes: []string{"corruption_purged", "arrested"},
			},
		)
	} else {
		// Non-intrigue action/survival/xianxia/wasteland worlds: only uncover/expose major mortal conspiracies, no petty bribe/blackmail spam.
		rules = append(rules,
			InteractionRule{
				ID: "base_pro_investigate_rival", Domain: "intrigue", Name: "暗查强敌核心黑幕", Category: "情报暗战",
				Priority: "low", Weight: 14, CooldownTicks: 96,
				Actor1: RoleMatcher{PersonalityKeywords: []string{"inquisitive", "cunning", "calculating"}},
				Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(-3), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "investigate_actor", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}暗中查探{actor2}的核心阴谋"},
				ExpectedOutcomes: []string{"secret_discovered"},
			},
			InteractionRule{
				ID: "base_pro_expose_secret", Domain: "intrigue", Name: "当众揭露惊天阴谋", Category: "情报暗战",
				Priority: "medium", Weight: 20, CooldownTicks: 96,
				Actor1: RoleMatcher{}, Actor2: RoleMatcher{},
				Relation: RelationPredicate{MaxTrust: intPtr(-3), CoLocated: boolPtr(true)},
				Behavior: BehaviorSpec{EngineKind: "expose_secret", TargetBinding: "$actor2", SummaryTemplate: "{actor1}在{location}公然揭露{actor2}的惊天阴谋"},
				ExpectedOutcomes: []string{"secret_exposed"},
			},
		)
	}
	return rules
}

func defaultBaseReactionRules(genre string) []CausalReactionRule {
	intrigue := isIntrigueOrCourtGenre(genre)
	arrestWeight := 10
	if intrigue {
		arrestWeight = 38
	}
	return []CausalReactionRule{
		{
			ID: "base_rxn_attacked_target", Domain: "conflict", Name: "遭袭当事人即时回应",
			TriggerEventKinds: []string{"attacked", "raided", "feud_avenged", "party_betrayed"},
			OutcomeClass:      "any", ResponderPosition: "actor2",
			Branches: []ReactionBranch{
				{
					ID: "retaliate", Stance: "强硬反制", Title: "当场死战反击", Weight: 58,
					Behavior: BehaviorSpec{EngineKind: "attack", TargetBinding: "$actor1", SummaryTemplate: "{responder}遭{actor1}袭击后悍然反击"},
				},
				{
					ID: "arrest_attacker", Stance: "秉公处置", Title: "依规缉拿凶手", Weight: arrestWeight,
					Behavior: BehaviorSpec{EngineKind: "arrest", TargetBinding: "$actor1", SummaryTemplate: "{responder}当场缉拿行凶的{actor1}"},
				},
				{
					ID: "call_help", Stance: "借力拉拢", Title: "传讯盟友求援", Weight: 22,
					Behavior: BehaviorSpec{EngineKind: "transmit_message", TargetBinding: "$ally", SummaryTemplate: "{responder}传讯盟友揭露{actor1}行凶"},
				},
				{
					ID: "retreat", Stance: "避险止损", Title: "暂避锋芒撤离", Weight: 18,
					Behavior: BehaviorSpec{EngineKind: "move", TargetBinding: "$self", SummaryTemplate: "{responder}权衡局势后迅速撤离{location}"},
				},
			},
		},
		{
			ID: "base_rxn_attacked_witness", Domain: "conflict", Name: "目击暴力冲突旁观者回应",
			TriggerEventKinds: []string{"attacked", "raided", "party_betrayed"},
			OutcomeClass:      "any", ResponderPosition: "witness",
			Branches: []ReactionBranch{
				{
					ID: "enforce", Stance: "秉公处置", Title: "出手制止缉凶", Weight: arrestWeight,
					Behavior: BehaviorSpec{EngineKind: "arrest", TargetBinding: "$actor1", SummaryTemplate: "{responder}目睹行凶后介入缉拿{actor1}"},
				},
				{
					ID: "aid_victim", Stance: "积极呼应", Title: "救治或声援受害者", Weight: 28,
					Behavior: BehaviorSpec{EngineKind: "heal", TargetBinding: "$actor2", SummaryTemplate: "{responder}出手照料遇袭的{actor2}"},
				},
				{
					ID: "spread_word", Stance: "暗中周旋", Title: "传播冲突消息", Weight: 22,
					Behavior: BehaviorSpec{EngineKind: "speak", TargetBinding: "$ally", SummaryTemplate: "{responder}将{actor1}袭击{actor2}之事传开"},
				},
				{
					ID: "leave_scene", Stance: "避险止损", Title: "明哲保身离开", Weight: 20,
					Behavior: BehaviorSpec{EngineKind: "move", TargetBinding: "$self", SummaryTemplate: "{responder}见势不妙悄然离开{location}"},
				},
			},
		},
		{
			ID: "base_rxn_intrigue_response", Domain: "intrigue", Name: "遭遇密谋/曝光后的反制",
			TriggerEventKinds: []string{"blackmailed", "blackmail_refused", "stolen", "bribed", "secret_exposed"},
			OutcomeClass:      "any", ResponderPosition: "actor2",
			Branches: []ReactionBranch{
				{
					ID: "strike_back", Stance: "强硬反制", Title: "撕破脸皮直接动手", Weight: 42,
					Behavior: BehaviorSpec{EngineKind: "attack", TargetBinding: "$actor1", SummaryTemplate: "{responder}被逼急后直接向{actor1}发难"},
				},
				{
					ID: "counter_expose", Stance: "秉公处置", Title: "当众反揭丑事", Weight: 26,
					Behavior: BehaviorSpec{EngineKind: "expose_secret", TargetBinding: "$actor1", SummaryTemplate: "{responder}反击揭露{actor1}的阴谋"},
				},
				{
					ID: "counter_investigate", Stance: "暗中周旋", Title: "反查对方把柄", Weight: 20,
					Behavior: BehaviorSpec{EngineKind: "investigate_actor", TargetBinding: "$actor1", SummaryTemplate: "{responder}反手暗中追查{actor1}的软肋"},
				},
				{
					ID: "negotiate_out", Stance: "谈判妥协", Title: "出言周旋缓兵", Weight: 16,
					Behavior: BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor1", SummaryTemplate: "{responder}与{actor1}虚与委蛇拖延时间"},
				},
			},
		},
		{
			ID: "base_rxn_trade_offer", Domain: "economy", Name: "面对交易与契约要约的回应",
			TriggerEventKinds: []string{"trade_offered", "agreement_proposed"},
			OutcomeClass:      "any", ResponderPosition: "actor2",
			Branches: []ReactionBranch{
				{
					ID: "accept_deal", Stance: "积极呼应", Title: "爽快成交", Weight: 40,
					Behavior: BehaviorSpec{EngineKind: "accept", TargetBinding: "$actor1", SummaryTemplate: "{responder}接受了{actor1}的交易要约"},
				},
				{
					ID: "reject_deal", Stance: "避险止损", Title: "谨慎婉拒", Weight: 22,
					Behavior: BehaviorSpec{EngineKind: "reject", TargetBinding: "$actor1", SummaryTemplate: "{responder}拒绝了{actor1}的交易提议"},
				},
			},
		},
		{
			ID: "base_rxn_social_talk", Domain: "social", Name: "面对结交与传讯的回应",
			TriggerEventKinds: []string{"speech", "channel_transmitted", "bond_sworn"},
			OutcomeClass:      "any", ResponderPosition: "actor2",
			Branches: []ReactionBranch{
				{
					ID: "reply_warm", Stance: "积极呼应", Title: "顺势交谈回应", Weight: 32,
					Behavior: BehaviorSpec{EngineKind: "speak", TargetBinding: "$actor1", SummaryTemplate: "{responder}回应了{actor1}的谈话"},
				},
				{
					ID: "bond_up", Stance: "借力拉拢", Title: "加深结盟情谊", Weight: 22,
					Behavior: BehaviorSpec{EngineKind: "swear_bond", TargetBinding: "$actor1", SummaryTemplate: "{responder}借机与{actor1}结为同盟"},
				},
			},
		},
	}
}

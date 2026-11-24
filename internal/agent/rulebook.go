package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// EncounterProfile defines how a personality trait scales an actor's event
// encounter frequency across four orthogonal causal channels (1.0 = neutral,
// <1.0 = fewer encounters / stays under the radar, >1.0 = more encounters / draws attention).
type EncounterProfile struct {
	ProactiveRate    float64 `json:"proactive_rate,omitempty"`    // scales proactive interaction initiation cadence
	TargetAttraction float64 `json:"target_attraction,omitempty"` // scales how likely other actors notice/target this actor
	DirectEngage     float64 `json:"direct_engage,omitempty"`     // scales engagement vs brushing off direct non-violent events
	WitnessEngage    float64 `json:"witness_engage,omitempty"`    // scales bystander intervention vs walking away
}

// InteractionRulebook is a fully parameterized, actor-agnostic behavioral and
// causal-reaction rule table:
//
//	Actor1(RoleMatcher, Hobbies->behavior weight, Personality->encounter cadence) --[Behavior]--> Actor2(RoleMatcher, Personality->target attraction)
//	    ==> Engine Outcome ==> Actor2 / Witness Reaction Branches (Personality->encounter gate & stance/branch weight)
//
// Zero specific actor IDs or actor names appear in any rule; bindings are resolved
// dynamically at runtime across any population size, cast rename, or cohort promotion.
type InteractionRulebook struct {
	WorldID                       string                        `json:"world_id"`
	Theme                         string                        `json:"theme,omitempty"`
	Genre                         string                        `json:"genre,omitempty"`
	Environment                   sim.EnvironmentProfile        `json:"environment"`
	Glossaries                    PlaybookGlossaries            `json:"glossaries"`
	PersonalityStanceWeights      map[string]map[string]float64 `json:"personality_stance_weights,omitempty"`      // trait -> stance -> multiplier
	ProfessionEncounterModifiers  map[string]EncounterProfile   `json:"profession_encounter_modifiers,omitempty"`  // profession -> structural daily social contact baseline
	PersonalityEncounterModifiers map[string]EncounterProfile   `json:"personality_encounter_modifiers,omitempty"` // trait -> individual within-profession encounter modifier
	HobbyActionWeights            map[string]map[string]float64 `json:"hobby_action_weights,omitempty"`            // hobby -> engine_kind -> multiplier
	ProactiveRules                []InteractionRule             `json:"proactive_rules"`
	ReactionRules                 []CausalReactionRule          `json:"reaction_rules"`
}

// RoleMatcher matches any actor dynamically by role, capability/social tags,
// personality traits, hobbies, and runtime state—never by fixed actor ID or name.
type RoleMatcher struct {
	Roles               []string `json:"roles,omitempty"`
	AnyTags             []string `json:"any_tags,omitempty"`
	ExcludeTags         []string `json:"exclude_tags,omitempty"`
	AnyHobbies          []string `json:"any_hobbies,omitempty"`
	ExcludeHobbies      []string `json:"exclude_hobbies,omitempty"`
	PersonalityKeywords []string `json:"personality_keywords,omitempty"`
	MinHealth           *int     `json:"min_health,omitempty"`
	MaxHealth           *int     `json:"max_health,omitempty"`
	MinMoney            *int     `json:"min_money,omitempty"`
	MaxMoney            *int     `json:"max_money,omitempty"`
	HasIllicitGain      *bool    `json:"has_illicit_gain,omitempty"`
	HasSurplusGoods     *bool    `json:"has_surplus_goods,omitempty"`
	IsWounded           *bool    `json:"is_wounded,omitempty"`
}

// RelationPredicate constrains the dynamic relationship between Actor 1 and Actor 2.
type RelationPredicate struct {
	MinTrust        *int   `json:"min_trust,omitempty"`
	MaxTrust        *int   `json:"max_trust,omitempty"`
	FactionRelation string `json:"faction_relation,omitempty"` // same | different | any
	CoLocated       *bool  `json:"co_located,omitempty"`
	SharedHobby     *bool  `json:"shared_hobby,omitempty"`
}

// BehaviorSpec describes a parameterized engine action whose target and item
// are bound dynamically at runtime ($actor2, $actor1, $enforcer_or_elder, $ally, $self).
type BehaviorSpec struct {
	EngineKind      string `json:"engine_kind"`
	TargetBinding   string `json:"target_binding,omitempty"` // $actor2 | $actor1 | $enforcer_or_elder | $ally | $self
	LocationFeature string `json:"location_feature,omitempty"`
	ItemRole        string `json:"item_role,omitempty"` // surplus | edible | medicinal | combat | highest_tier | any
	SummaryTemplate string `json:"summary_template"`
}

// InteractionRule defines a generic proactive interaction:
// Actor1 matching RoleMatcher performs Behavior toward any matching Actor2,
// expecting one of ExpectedOutcomes from the simulation engine.
// Actor1's hobbies (and shared hobbies with Actor2) modulate the rule's selection weight.
type InteractionRule struct {
	ID                    string             `json:"id"`
	Domain                string             `json:"domain,omitempty"`
	Name                  string             `json:"name"`
	Category              string             `json:"category"`
	Priority              string             `json:"priority"` // urgent | high | medium | low
	Weight                int                `json:"weight"`
	HobbyWeights          map[string]float64 `json:"hobby_weights,omitempty"`           // hobby_id -> weight multiplier
	SharedHobbyMultiplier float64            `json:"shared_hobby_multiplier,omitempty"` // extra multiplier when actor1 & actor2 share a hobby
	CooldownTicks         int64              `json:"cooldown_ticks,omitempty"`
	Actor1                RoleMatcher        `json:"actor1"`
	Actor2                RoleMatcher        `json:"actor2"`
	Relation              RelationPredicate  `json:"relation,omitempty"`
	LocationFeatures      []string           `json:"location_features,omitempty"`
	Behavior              BehaviorSpec       `json:"behavior"`
	ExpectedOutcomes      []string           `json:"expected_outcomes"`
	CausalSummary         string             `json:"causal_summary"`
}

// CausalReactionRule defines how Actor 2 (direct target) or a Witness reacts when
// the simulation engine settles an event (Actor 1 -> Behavior -> Actor 2 => Outcome).
type CausalReactionRule struct {
	ID                string            `json:"id"`
	Domain            string            `json:"domain,omitempty"`
	Name              string            `json:"name"`
	TriggerEventKinds []string          `json:"trigger_event_kinds"`
	OutcomeClass      string            `json:"outcome_class,omitempty"` // succeeded | failed | proposed | any
	Actor1Match       RoleMatcher       `json:"actor1_match,omitempty"`
	Actor2Match       RoleMatcher       `json:"actor2_match,omitempty"`
	ResponderPosition string            `json:"responder_position"` // actor2 | witness
	ResponderMatch    RoleMatcher       `json:"responder_match,omitempty"`
	RelationToActor1  RelationPredicate `json:"relation_to_actor1,omitempty"`
	CausalSummary     string            `json:"causal_summary"`
	Branches          []ReactionBranch  `json:"branches"`
}

// ReactionBranch is one stance-differentiated reactive choice inside a CausalReactionRule.
// The responder's personality traits modulate this branch's selection weight.
type ReactionBranch struct {
	ID                 string             `json:"id"`
	Stance             string             `json:"stance"` // 积极呼应 | 秉公处置 | 强硬反制 | 谈判妥协 | 借力拉拢 | 暗中周旋 | 趁机谋利 | 避险止损
	Title              string             `json:"title"`
	WhenResponder      RoleMatcher        `json:"when_responder,omitempty"`
	Weight             int                `json:"weight"`
	PersonalityWeights map[string]float64 `json:"personality_weights,omitempty"` // trait_keyword -> weight multiplier
	Behavior           BehaviorSpec       `json:"behavior"`
	ExpectedOutcome    string             `json:"expected_outcome,omitempty"`
}

// LoadRulebook loads a compiled generic InteractionRulebook JSON file from disk.
func LoadRulebook(path string) (*InteractionRulebook, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("rulebook path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat rulebook %s: %w", path, err)
	}
	target := path
	if info.IsDir() {
		target = filepath.Join(path, "rulebook.json")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read rulebook %s: %w", target, err)
	}
	var rb InteractionRulebook
	if err := json.Unmarshal(raw, &rb); err != nil {
		return nil, fmt.Errorf("parse rulebook %s: %w", target, err)
	}
	if len(rb.ProactiveRules) == 0 && len(rb.ReactionRules) == 0 {
		return nil, fmt.Errorf("rulebook %s contains no proactive or reaction rules", target)
	}
	for i := range rb.ProactiveRules {
		if rb.ProactiveRules[i].Weight <= 0 {
			rb.ProactiveRules[i].Weight = 20
		}
		if rb.ProactiveRules[i].CooldownTicks <= 0 {
			rb.ProactiveRules[i].CooldownTicks = 48
		}
		rb.ProactiveRules[i].Priority = strings.ToLower(strings.TrimSpace(rb.ProactiveRules[i].Priority))
		if rb.ProactiveRules[i].Behavior.TargetBinding == "" {
			rb.ProactiveRules[i].Behavior.TargetBinding = "$actor2"
		}
	}
	for i := range rb.ReactionRules {
		pos := strings.ToLower(strings.TrimSpace(rb.ReactionRules[i].ResponderPosition))
		if pos != "witness" {
			pos = "actor2"
		}
		rb.ReactionRules[i].ResponderPosition = pos
		for j := range rb.ReactionRules[i].Branches {
			if rb.ReactionRules[i].Branches[j].Weight <= 0 {
				rb.ReactionRules[i].Branches[j].Weight = 20
			}
			if rb.ReactionRules[i].Branches[j].Behavior.TargetBinding == "" {
				rb.ReactionRules[i].Branches[j].Behavior.TargetBinding = "$actor1"
			}
		}
	}
	return &rb, nil
}

var personalityStopWords = map[string]bool{
	"and": true, "or": true, "but": true, "with": true, "yet": true,
	"very": true, "quite": true, "rather": true, "the": true, "a": true,
	"an": true, "to": true, "of": true, "in": true, "for": true,
	"is": true, "are": true, "be": true, "who": true, "by": true,
	"on": true, "at": true, "from": true, "as": true, "not": true,
}

// ExtractPersonalityTraits splits an actor's personality description into
// deduplicated lowercase trait tokens (supporting both English and CJK).
func ExtractPersonalityTraits(personality string) []string {
	lower := strings.ToLower(strings.TrimSpace(personality))
	if lower == "" {
		return nil
	}
	rawTokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-'
	})
	seen := make(map[string]bool, len(rawTokens))
	out := make([]string, 0, len(rawTokens))
	for _, tok := range rawTokens {
		tok = strings.Trim(tok, "-_")
		if tok == "" || personalityStopWords[tok] || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

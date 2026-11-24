package agent

import (
	"fmt"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// RuleSet is the compiled, validated rule input consumed by CandidateBuilder.
type RuleSet struct {
	Rulebook *InteractionRulebook
	Catalog  sim.Catalog
}

// RuleCompiler compiles and validates declarative interaction rules against an ActionCatalog.
// Unknown primitives, uncataloged Action.Kinds, or invalid parameters fail at compile time.
type RuleCompiler interface {
	Compile(source *InteractionRulebook, catalog sim.Catalog) (RuleSet, error)
}

// DefaultRuleCompiler validates InteractionRulebook rules strictly against sim.Catalog
// (built-in engine actions + declarative Catalog.ActionSpecs).
type DefaultRuleCompiler struct{}

func (DefaultRuleCompiler) Compile(source *InteractionRulebook, catalog sim.Catalog) (RuleSet, error) {
	if source == nil {
		return RuleSet{Catalog: catalog}, nil
	}
	for _, r := range source.ProactiveRules {
		if err := validateRuleBehavior(r.Behavior, catalog); err != nil {
			return RuleSet{}, fmt.Errorf("proactive rule %q: %w", r.ID, err)
		}
	}
	for _, r := range source.ReactionRules {
		for j, br := range r.Branches {
			if err := validateRuleBehavior(br.Behavior, catalog); err != nil {
				return RuleSet{}, fmt.Errorf("reaction rule %q branch %d: %w", r.ID, j, err)
			}
		}
	}
	return RuleSet{Rulebook: source, Catalog: catalog}, nil
}

func validateRuleBehavior(behavior BehaviorSpec, catalog sim.Catalog) error {
	if !catalog.IsKnownActionKind(behavior.EngineKind) {
		return fmt.Errorf("unknown engine_kind %q", behavior.EngineKind)
	}
	switch strings.ToLower(strings.TrimSpace(behavior.TargetBinding)) {
	case "", "$actor1", "$actor2", "$self", "$ally", "$enforcer_or_elder":
	default:
		return fmt.Errorf("unknown target_binding %q", behavior.TargetBinding)
	}
	switch strings.ToLower(strings.TrimSpace(behavior.ItemRole)) {
	case "", "any", "surplus", "edible", "medicinal", "combat", "highest_tier":
	default:
		if _, exists := catalog.Items[behavior.ItemRole]; !exists {
			return fmt.Errorf("unknown item_role %q", behavior.ItemRole)
		}
	}
	return nil
}

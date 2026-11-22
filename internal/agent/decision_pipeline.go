package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
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

// CandidateBuilder constructs the unified legal Candidate list from Perception and RuleSet.
type CandidateBuilder interface {
	Build(view sim.Perception, rules RuleSet) ([]sim.Candidate, error)
}

// DecisionSelector chooses a single ActionIntent from pre-validated Candidates.
// Selectors (Jev, RuleSelector, RulebookSelector, PlaybookSelector) never mutate World.
type DecisionSelector interface {
	Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error)
}

// DefaultRuleCompiler validates InteractionRulebook rules strictly against sim.Catalog
// (built-in engine actions + declarative Catalog.ActionSpecs).
type DefaultRuleCompiler struct{}

func (DefaultRuleCompiler) Compile(source *InteractionRulebook, catalog sim.Catalog) (RuleSet, error) {
	if source == nil {
		return RuleSet{Catalog: catalog}, nil
	}
	for i, r := range source.ProactiveRules {
		kind := strings.TrimSpace(r.Behavior.EngineKind)
		if kind == "" {
			return RuleSet{}, fmt.Errorf("proactive rule[%d] (%s): empty engine_kind", i, r.ID)
		}
		if strings.HasPrefix(kind, "new_primitive:") {
			return RuleSet{}, fmt.Errorf("proactive rule[%d] (%s): unknown uncompiled primitive %q", i, r.ID, kind)
		}
		if !catalog.IsKnownActionKind(kind) {
			return RuleSet{}, fmt.Errorf("proactive rule[%d] (%s): unknown action kind %q", i, r.ID, kind)
		}
	}
	for i, r := range source.ReactionRules {
		for j, br := range r.Branches {
			kind := strings.TrimSpace(br.Behavior.EngineKind)
			if kind == "" {
				return RuleSet{}, fmt.Errorf("reaction rule[%d].branch[%d] (%s): empty engine_kind", i, j, r.ID)
			}
			if strings.HasPrefix(kind, "new_primitive:") {
				return RuleSet{}, fmt.Errorf("reaction rule[%d].branch[%d] (%s): unknown uncompiled primitive %q", i, j, r.ID, kind)
			}
			if !catalog.IsKnownActionKind(kind) {
				return RuleSet{}, fmt.Errorf("reaction rule[%d].branch[%d] (%s): unknown action kind %q", i, j, r.ID, kind)
			}
		}
	}
	return RuleSet{Rulebook: source, Catalog: catalog}, nil
}

// DefaultCandidateBuilder generates single-source legal Candidates from Perception
// and enriches them with structured RuleMatcher metadata (never mutating Option.Label).
type DefaultCandidateBuilder struct{}

func (DefaultCandidateBuilder) Build(view sim.Perception, rules RuleSet) ([]sim.Candidate, error) {
	opts := Options(view)
	candidates := optionsToCandidates(view.Catalog, opts)
	if rules.Rulebook != nil {
		annotateCandidatesWithRulebook(view, rules.Rulebook, opts, candidates)
	}
	return candidates, nil
}

func optionsToCandidates(catalog sim.Catalog, opts []Option) []sim.Candidate {
	candidates := make([]sim.Candidate, 0, len(opts))
	for _, opt := range opts {
		source := "builtin"
		if meta, ok := catalog.ActionMetadataByKind(opt.Action.Kind); ok && meta.Declarative {
			source = "action_spec"
		}
		candidates = append(candidates, sim.Candidate{
			ID:           opt.ID,
			Label:        opt.Label,
			Source:       source,
			BasisEventID: opt.Action.BasisEventID,
			Intent: sim.ActionIntent{
				Action:      opt.Action,
				CandidateID: opt.ID,
				Source:      source,
			},
		})
	}
	return candidates
}

// annotateCandidatesWithRulebook performs pure, read-only rule matching against the
// legal executable candidates, attaching structured RuleIDs, RuleReason, BasisEventID,
// and PriorityHint without mutating Candidate.Label or world state.
func annotateCandidatesWithRulebook(p sim.Perception, rb *InteractionRulebook, opts []Option, candidates []sim.Candidate) {
	if rb == nil || len(candidates) == 0 {
		return
	}
	a := p.Self
	idxByID := make(map[string]int, len(candidates))
	for i, c := range candidates {
		idxByID[c.ID] = i
	}
	selfProf := profileFromSelf(p)
	nearbyByID := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		nearbyByID[nb.ID] = nb
	}

	attach := func(optID, ruleID, reason string, basisEventID int64, boost float64) {
		i, ok := idxByID[optID]
		if !ok {
			return
		}
		if !sim.ActorHasTag(candidates[i].RuleIDs, ruleID) {
			candidates[i].RuleIDs = append(candidates[i].RuleIDs, ruleID)
			candidates[i].Intent.RuleIDs = append(candidates[i].Intent.RuleIDs, ruleID)
		}
		if candidates[i].RuleReason == "" {
			candidates[i].RuleReason = reason
			candidates[i].Intent.RuleReason = reason
		} else if !strings.Contains(candidates[i].RuleReason, reason) {
			candidates[i].RuleReason += "; " + reason
			candidates[i].Intent.RuleReason = candidates[i].RuleReason
		}
		if boost > candidates[i].PriorityHint {
			candidates[i].PriorityHint = boost
		}
		if basisEventID > 0 && candidates[i].Intent.Action.BasisEventID == 0 {
			for _, m := range a.Memories {
				if m.EventID == basisEventID {
					candidates[i].BasisEventID = basisEventID
					candidates[i].Intent.Action.BasisEventID = basisEventID
					break
				}
			}
		}
	}

	// Tier 1: Causal reaction rules over recent memories (last 48 ticks).
	const reactionWindowTicks = 48
	for i := len(a.Memories) - 1; i >= 0; i-- {
		mem := a.Memories[i]
		if p.Tick-mem.Tick > reactionWindowTicks {
			break
		}
		if mem.EventID == 0 || mem.Source == "" || mem.Source == "world" || mem.Source == a.ID {
			continue
		}
		pos := "witness"
		actor2ID := mem.TargetID
		if mem.TargetID == a.ID {
			pos = "actor2"
		} else if mem.TargetID == "" && mem.Observation != nil && mem.Observation.ObjectID != "" {
			actor2ID = mem.Observation.ObjectID
			if actor2ID == a.ID {
				pos = "actor2"
			}
		}
		for rIdx := range rb.ReactionRules {
			rule := &rb.ReactionRules[rIdx]
			if rule.ResponderPosition != pos || !eventKindMatches(rule.TriggerEventKinds, mem.Kind) {
				continue
			}
			if !matchesRole(rule.ResponderMatch, selfProf) {
				continue
			}
			for _, br := range rule.Branches {
				targetID := resolveTargetBinding(p, br.Behavior.TargetBinding, mem.Source, actor2ID, nearbyByID)
				opt := matchGenericBehavior(p, opts, br.Behavior, targetID)
				if opt == nil {
					continue
				}
				if mem.Kind == "faction_war_declared" && opt.Action.Kind == "attack" &&
					nearbyByID[opt.Action.TargetID].FactionRelation != "at_war" {
					continue
				}
				if !secretReactionAllowed(p, mem, opt.Action) {
					continue
				}
				title := strings.TrimSpace(br.Title)
				if title == "" {
					title = rule.Name
				}
				reason := fmt.Sprintf("reaction %s/%s: %s (cause #%d %s)", rule.ID, br.Stance, title, mem.EventID, mem.Kind)
				attach(opt.ID, rule.ID, reason, mem.EventID, float64(max(0, br.Weight)))
			}
		}
	}

	// Tier 2: Proactive role-pair rules matching executable options.
	targets := make([]actorProfileView, 0, len(p.Nearby))
	for _, nb := range p.Nearby {
		if nb.Jailed || nb.Sleeping || nb.ID == a.ID {
			continue
		}
		targets = append(targets, profileFromNeighbor(nb))
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	// Pure stateless relation check for read-only candidate annotation.
	matcher := &RulebookSelector{Rulebook: rb}
	for rIdx := range rb.ProactiveRules {
		rule := &rb.ProactiveRules[rIdx]
		if !districtMatchesFeatures(p, rule.LocationFeatures) || !matchesRole(rule.Actor1, selfProf) {
			continue
		}
		for _, tProf := range targets {
			if !matchesRole(rule.Actor2, tProf) {
				continue
			}
			if !matcher.matchesRelationLocked(p, rule.Relation, tProf.ID, true) {
				continue
			}
			targetID := resolveTargetBinding(p, rule.Behavior.TargetBinding, a.ID, tProf.ID, nearbyByID)
			opt := matchGenericBehavior(p, opts, rule.Behavior, targetID)
			if opt == nil {
				continue
			}
			reason := fmt.Sprintf("proactive %s (%s): %s -> %s", rule.ID, rule.Category, rule.Name, tProf.Name)
			attach(opt.ID, rule.ID, reason, 0, float64(max(0, rule.Weight)))
		}
	}
}

func candidatesToOptions(candidates []sim.Candidate) []Option {
	opts := make([]Option, len(candidates))
	for i, c := range candidates {
		opts[i] = Option{ID: c.ID, Label: c.Label, Action: c.Intent.Action}
	}
	return opts
}

func findCandidateByID(candidates []sim.Candidate, id string) (sim.Candidate, bool) {
	for _, c := range candidates {
		if c.ID == id {
			return c, true
		}
	}
	return sim.Candidate{}, false
}

// DecisionPipeline wires CandidateBuilder + RuleSet + DecisionSelector into sim.Decider,
// recording decision_input / decision_ready / decision_error trace entries identically to Pipeline.
type DecisionPipeline struct {
	Builder  CandidateBuilder
	Rules    RuleSet
	Selector DecisionSelector
	Trace    *runtrace.Writer
}

func (dp DecisionPipeline) Decide(ctx context.Context, p sim.Perception) (action sim.Action, err error) {
	stage := "input"
	defer func() {
		if err == nil {
			stage = "ready"
			err = dp.Trace.Record("decision_ready", p.Tick, p.Self.ID, map[string]any{"action": action})
		}
		if err != nil {
			if traceErr := dp.Trace.Record("decision_error", p.Tick, p.Self.ID, map[string]any{"stage": stage, "error": err.Error()}); traceErr != nil {
				err = errors.Join(err, traceErr)
			}
		}
	}()
	builder := dp.Builder
	if builder == nil {
		builder = DefaultCandidateBuilder{}
	}
	candidates, err := builder.Build(p, dp.Rules)
	if err != nil {
		return sim.Action{}, err
	}
	source := "selector"
	sel := dp.Selector
	switch s := sel.(type) {
	case RuleDecisionSelector:
		source = "rules"
		if dp.Trace != nil {
			s.Selector = RuleSelector{trace: dp.Trace}
			sel = s
		}
	case JevDecisionSelector:
		source = "jev"
		_, defaultBuilder := builder.(DefaultCandidateBuilder)
		if s.Jev.RulebookAdvisor != nil && defaultBuilder {
			s.rulebookMatched = dp.Rules.Rulebook == s.Jev.RulebookAdvisor.Rulebook
		}
		sel = s
	case PlaybookDecisionSelector:
		source = "playbook"
		if s.Selector != nil && dp.Trace != nil {
			s.Selector.mu.Lock()
			if s.Selector.Trace == nil {
				s.Selector.Trace = dp.Trace
			}
			s.Selector.mu.Unlock()
		}
	case RulebookDecisionSelector:
		source = "rulebook"
		if s.Selector != nil && dp.Trace != nil {
			s.Selector.mu.Lock()
			if s.Selector.Trace == nil {
				s.Selector.Trace = dp.Trace
			}
			s.Selector.mu.Unlock()
		}
	}
	if err = dp.Trace.Record("decision_input", p.Tick, p.Self.ID, map[string]any{
		"perception":      p,
		"options":         candidatesToOptions(candidates),
		"decision_source": source,
	}); err != nil {
		return sim.Action{}, err
	}
	stage = "options"
	if len(candidates) == 0 {
		return sim.Action{}, fmt.Errorf("actor %s has no actions granted by tags", p.Self.ID)
	}
	stage = "selector"
	if sel == nil {
		return sim.Action{}, fmt.Errorf("decision selector is nil")
	}
	intent, err := sel.Select(ctx, p, candidates)
	if err != nil {
		return sim.Action{}, err
	}
	stage = "selected"
	if err = dp.Trace.Record("decision_selected", p.Tick, p.Self.ID, map[string]any{
		"option_id": intent.CandidateID, "action": intent.Action,
	}); err != nil {
		return sim.Action{}, err
	}
	return intent.Action, nil
}

// optionDecisionSelector adapts selectors that identify an option by ID.
type optionDecisionSelector struct {
	selector Selector
}

func (s optionDecisionSelector) Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error) {
	if s.selector == nil {
		return sim.ActionIntent{}, fmt.Errorf("selector is nil")
	}
	id, err := s.selector.Select(ctx, view, candidatesToOptions(candidates))
	if err != nil {
		return sim.ActionIntent{}, err
	}
	candidate, ok := findCandidateByID(candidates, id)
	if !ok {
		return sim.ActionIntent{}, fmt.Errorf("selector returned unavailable option %q", id)
	}
	intent := candidate.Intent
	if intent.Action.BasisEventID == 0 {
		if source, ok := s.selector.(interface{ LastTriggerEventID(string) int64 }); ok {
			stampKnownTrigger(view, &intent, source.LastTriggerEventID(view.Self.ID))
		}
	}
	return intent, nil
}

func stampKnownTrigger(view sim.Perception, intent *sim.ActionIntent, basisID int64) {
	if intent.Action.BasisEventID != 0 || basisID <= 0 {
		return
	}
	for _, m := range view.Self.Memories {
		if m.EventID == basisID {
			intent.Action.BasisEventID = basisID
			return
		}
	}
}

type RuleDecisionSelector struct {
	Selector RuleSelector
}

func (s RuleDecisionSelector) Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error) {
	return optionDecisionSelector{s.Selector}.Select(ctx, view, candidates)
}

type RulebookDecisionSelector struct {
	Selector *RulebookSelector
}

func (s RulebookDecisionSelector) Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error) {
	if s.Selector == nil {
		return sim.ActionIntent{}, fmt.Errorf("rulebook selector is nil")
	}
	return optionDecisionSelector{s.Selector}.Select(ctx, view, candidates)
}

type PlaybookDecisionSelector struct {
	Selector *PlaybookSelector
}

func (s PlaybookDecisionSelector) Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error) {
	if s.Selector == nil {
		return sim.ActionIntent{}, fmt.Errorf("playbook selector is nil")
	}
	return optionDecisionSelector{s.Selector}.Select(ctx, view, candidates)
}

type JevDecisionSelector struct {
	Jev             Jev
	rulebookMatched bool
}

func (s JevDecisionSelector) Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error) {
	return s.Jev.selectCandidates(ctx, view, candidates, s.rulebookMatched)
}

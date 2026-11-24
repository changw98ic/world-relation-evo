package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Authoring and fault-injection fixtures exercise choices from published candidates.
type Selector interface {
	Select(context.Context, sim.Perception, []Option) (string, error)
}

type Pipeline struct {
	Selector Selector
	Trace    *runtrace.Writer
}

func (p Pipeline) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	pipeline := DecisionPipeline{Trace: p.Trace}
	switch selector := p.Selector.(type) {
	case RuleSelector:
		pipeline.Selector = RuleDecisionSelector{Selector: selector}
	case *RuleSelector:
		pipeline.Selector = RuleDecisionSelector{Selector: *selector}
	case *RulebookSelector:
		pipeline.Selector = RulebookDecisionSelector{Selector: selector}
	case *PlaybookSelector:
		pipeline.Selector = PlaybookDecisionSelector{Selector: selector}
	default:
		pipeline.Selector = optionDecisionSelector{selector}
	}
	return pipeline.Decide(ctx, view)
}

// DecisionSelector chooses a single ActionIntent from pre-validated Candidates.
// Fixture selectors never mutate World.
type DecisionSelector interface {
	Select(ctx context.Context, view sim.Perception, candidates []sim.Candidate) (sim.ActionIntent, error)
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
		"options":         candidates,
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
		"option_id": intent.CandidateID, "action": intent.Action, "rule_ids": intent.RuleIDs, "rule_reason": intent.RuleReason,
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
	id, err := s.selector.Select(ctx, view, candidates)
	if err != nil {
		return sim.ActionIntent{}, err
	}
	candidate, ok := findCandidateByID(candidates, id)
	if !ok {
		return sim.ActionIntent{}, fmt.Errorf("selector returned unavailable option %q", id)
	}
	return candidate.Intent(), nil
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

// RuleSelector supplies fixed choices for the comparison fixtures.
type RuleSelector struct{ trace *runtrace.Writer }

func (r RuleSelector) Select(_ context.Context, p sim.Perception, options []Option) (string, error) {
	var id string
	var unavailable []sim.Action
	(rulesPolicy{options: options}).preferences(p, func(desired sim.Action) bool {
		id = ruleOptionID(desired, options)
		if id == "" && r.trace != nil {
			unavailable = append(unavailable, desired)
		}
		return id != ""
	})
	if len(unavailable) > 0 {
		if err := r.trace.Record("rule_unavailable", p.Tick, p.Self.ID, map[string]any{"preferences": unavailable}); err != nil {
			return "", err
		}
	}
	if id == "" {
		return "", fmt.Errorf("no available action matches the rules for %s at tick %d", p.Self.ID, p.Tick)
	}
	return id, nil
}

func ruleOptionID(desired sim.Action, options []Option) string {
	for _, o := range options {
		if actionMatchesDesired(o.Action, desired) {
			return o.ID
		}
	}
	return ""
}

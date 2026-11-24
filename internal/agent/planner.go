package agent

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Planner compares plans in the actor's view of the world. Settlement still
// checks and applies the selected action to the authoritative state.
type Planner struct {
	Trace *runtrace.Writer
}

type needValue struct {
	sim.NeedRule
	level, drift, weight    float64
	base, bias, fluctuation float64
	lower, upper            float64
	bounded                 bool
	reviewAfter             int64
	reference               []effect
}

type effect struct {
	Need        sim.NeedKey `json:"need"`
	At          int64       `json:"at"`
	Change      float64     `json:"change"`
	DriftChange float64     `json:"drift_change,omitempty"`
	shared      bool
}

type forecast struct {
	candidate     sim.Candidate
	steps         []sim.Action
	effects       []effect
	unconditional []effect
	failure       []effect
	duration      int64
	probability   float64
	goal          sim.NeedKey
	strategy      string
	advance       bool
}

func (f *forecast) change(measure, subject string, at int64, change float64) {
	if change != 0 {
		f.effects = append(f.effects, effect{Need: sim.NeedKey{Measure: measure, Subject: subject}, At: at, Change: change})
	}
}

func (f *forecast) identify(goal sim.NeedKey) {
	f.goal = goal
	if f.strategy == "" {
		x := f.candidate.Action
		f.strategy = strings.Join([]string{x.Kind, goal.Measure, goal.Subject, x.TargetID, x.RecipeID, x.Item, x.SkillID, x.TransitionID}, "/")
	}
}

type contribution struct {
	Need          sim.NeedKey `json:"need"`
	Effect        float64     `json:"effect"`
	Weight        float64     `json:"weight"`
	BaseWeight    float64     `json:"base_weight"`
	TemporaryBias float64     `json:"temporary_bias"`
	Fluctuation   float64     `json:"fluctuation"`
}

type planScore struct {
	Candidate     string         `json:"candidate_id"`
	Strategy      string         `json:"strategy"`
	Goal          sim.NeedKey    `json:"goal"`
	Duration      int64          `json:"duration_ticks"`
	Steps         []sim.Action   `json:"next_steps,omitempty"`
	Probability   float64        `json:"probability"`
	Score         float64        `json:"score"`
	SwitchCost    float64        `json:"switch_cost"`
	Contributions []contribution `json:"contributions,omitempty"`
	Continuing    bool           `json:"continuing,omitempty"`
}

type needTrace struct {
	sim.NeedRule
	Level       float64  `json:"level"`
	Drift       float64  `json:"change_per_tick"`
	Weight      float64  `json:"effective_weight"`
	ReviewAfter int64    `json:"review_after_ticks,omitempty"`
	Reference   []effect `json:"reference_effects,omitempty"`
}

func (planner Planner) Decide(ctx context.Context, p sim.Perception) (action sim.Action, err error) {
	if err := ctx.Err(); err != nil {
		return sim.Action{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, planner.Trace.Record("decision_error", p.Tick, p.Self.ID, map[string]any{"error": err.Error()}))
		}
	}()

	rules := p.Catalog.EffectivePlanningRules()
	margin := rules.InertiaMargin
	if personal := p.Self.InertiaMargin; personal != nil {
		margin = *personal
		if math.IsNaN(margin) || math.IsInf(margin, 0) || margin < 0 {
			return sim.Action{}, fmt.Errorf("invalid inertia margin for actor %s", p.Self.ID)
		}
	}
	stored := p.Self.Behavior
	prior := stored
	resuming := stored != nil && len(stored.Steps) == 0 && stored.Paused != nil
	if resuming {
		resumed := *stored.Paused
		resumed.ObservedEvent, resumed.Version = stored.ObservedEvent, stored.Version
		prior = &resumed
		p.Self.Behavior = prior
	}
	priorCoversRisks := prior != nil && p.PlanCoversImmediateRisks(prior.Goal)
	readyMeal := prior != nil && prior.Goal.Measure != "nutrition" &&
		(len(prior.Steps) == 0 || prior.Steps[0].Kind != "eat") && p.Self.Hunger > 0 &&
		p.Catalog.AvailableNutrition(p.Self) > p.Self.NutritionReserve && p.Capabilities()["eat"]
	if priorCoversRisks && !readyMeal && len(prior.Steps) != 0 && (p.Tick < prior.ReviewAt || resuming) {
		next := prior.Steps[0]
		reason := "behavior_continued"
		steps := prior.Steps
		allowed := p.CheckAction(next).Allowed
		if !allowed {
			if repaired := repairRoute(p, steps); len(repaired) != 0 {
				steps, next, reason = repaired, repaired[0], "behavior_repaired"
				allowed = p.CheckAction(next).Allowed
			}
		}
		if allowed && (!p.BehaviorChanged || reason == "behavior_repaired") {
			if resuming && reason == "behavior_continued" {
				reason = "behavior_resumed"
			}
			next.Decision = &sim.BehaviorDecision{Behavior: *prior, Preferences: p.Self.Preferences,
				Facts: p.LocalPlanningFacts(), Reason: reason}
			next.Decision.Behavior.Steps = steps[1:]
			next.Decision.Behavior.ObservedEvent = p.ObservationVersion
			next.Decision.Behavior.ReviewAt = max(p.Tick+1, prior.ReviewAt)
			next.Decision.Behavior.ReviewReason = ""
			return next, planner.Trace.Record(reason, p.Tick, p.Self.ID, map[string]any{"action": next})
		}
	}
	cue := mealCue(p)
	if cue != "" && !p.BehaviorChanged {
		for _, kind := range []string{"drink", "eat"} {
			habit, familiar := p.Self.Habits[kind]
			if !familiar || habit.Successes < 2 || habit.Cue != cue ||
				!p.CheckAction(habit.Action).Allowed {
				continue
			}
			action = habit.Action
			goal := sim.NeedKey{Measure: "nutrition"}
			if kind == "drink" {
				goal.Measure = "hydration"
			}
			version := int64(1)
			if stored != nil {
				version = stored.Version + 1
			}
			action.Decision = &sim.BehaviorDecision{Behavior: sim.BehaviorState{
				Goal: goal, Strategy: kind + "/" + action.Item, ReviewAt: p.Tick + 1,
				ObservedEvent: p.ObservationVersion, Version: version, Paused: pausedBehavior(stored, goal),
			}, Preferences: p.Self.Preferences, Facts: p.LocalPlanningFacts(), Reason: "behavior_habit", RoutineCue: cue}
			return action, planner.Trace.Record("behavior_habit", p.Tick, p.Self.ID, map[string]any{"action": action, "evidence": habit.EventID})
		}
	}

	needs, preferences, err := buildNeeds(p, rules)
	if err != nil {
		return sim.Action{}, err
	}
	frame := projectFacts(p)
	candidates := planningCandidates(p)
	if len(candidates) == 0 {
		return sim.Action{}, fmt.Errorf("actor %s has no available action at tick %d", p.Self.ID, p.Tick)
	}
	forecasts := make([]forecast, 0, len(candidates))
	for _, candidate := range candidates {
		base := frame.forecast(candidate)
		forecasts = append(forecasts, base)
		switch candidate.Action.Kind {
		case "move", "move_venue", "leave_house", "enter_house", "begin_rest":
			forecasts = frame.preparations(forecasts, base)
		}
	}
	forecasts = frame.productionStrategies(forecasts, candidates)
	forecasts = frame.routineStrategies(forecasts, candidates)
	forecasts = frame.marketStrategies(forecasts, candidates)
	forecasts = frame.saleStrategies(forecasts, candidates)
	forecasts = frame.waterStrategies(forecasts, candidates, rules.HorizonTicks)

	var scores []planScore
	if planner.Trace != nil {
		scores = make([]planScore, len(forecasts))
	}
	best, continuing := 0, -1
	bestScore, continueScore := math.Inf(-1), math.Inf(-1)
	for i := range forecasts {
		f := &forecasts[i]
		score, contributions := evaluate(needs, *f, rules, planner.Trace != nil)
		if math.IsNaN(score) || math.IsInf(score, 0) {
			return sim.Action{}, fmt.Errorf("non-finite score for actor %s candidate %s", p.Self.ID, f.candidate.ID)
		}
		// Inertia compares viable plans. A completed purchase with no further
		// benefit is not an ongoing commitment to repeat that transaction.
		continues := priorCoversRisks && f.advance && f.goal == prior.Goal &&
			(followsStoredStep(f.candidate.Action, prior) || score > 0 && f.strategy == prior.Strategy)
		if scores != nil {
			scores[i] = planScore{Candidate: f.candidate.ID, Strategy: f.strategy, Goal: f.goal, Score: score,
				Duration: f.duration, Steps: f.steps, Probability: f.probability, Contributions: contributions, Continuing: continues}
		}
		if score > bestScore || score == bestScore && preferTiedPlan(*f, forecasts[best]) {
			best, bestScore = i, score
		}
		if continues && score > continueScore {
			continuing, continueScore = i, score
		}
	}
	reason := "highest_value"
	if continuing >= 0 && bestScore-continueScore <= margin {
		best = continuing
		reason = "retained_plan"
	}
	chosen := forecasts[best]
	action = chosen.candidate.Action
	reviewAt := p.Tick + min(chosen.duration, rules.HorizonTicks)
	reviewReason := ""
	if action.Kind == "wait" && bestScore <= 0 {
		review := p.NextPassiveReview(rules.HorizonTicks)
		reviewAt, reviewReason = review.Tick, review.Reason
	}
	version := int64(1)
	if stored != nil {
		version = stored.Version
		if chosen.goal != stored.Goal || chosen.strategy != stored.Strategy {
			version++
		}
	}
	action.Decision = &sim.BehaviorDecision{
		Behavior: sim.BehaviorState{
			Goal: chosen.goal, Strategy: chosen.strategy, Steps: chosen.steps, ObservedEvent: p.ObservationVersion,
			ReviewAt: reviewAt, ReviewReason: reviewReason, Version: version,
			Paused: pausedBehavior(stored, chosen.goal),
		},
		Preferences: preferences,
		Facts:       p.LocalPlanningFacts(),
		Reason:      reason,
		Score:       bestScore,
	}
	if best == continuing {
		action.Decision.Score = continueScore
	}
	if action.Kind == "eat" || action.Kind == "drink" {
		action.Decision.RoutineCue = cue
	}
	if planner.Trace != nil {
		matrix := make([]needTrace, 0, len(needs))
		for _, row := range needs {
			matrix = append(matrix, needTrace{NeedRule: row.NeedRule, Level: row.level, Drift: row.drift,
				Weight: row.weight, ReviewAfter: row.reviewAfter, Reference: row.reference})
		}
		if err := planner.Trace.Record("decision_input", p.Tick, p.Self.ID, map[string]any{
			"perception": p, "needs": matrix, "options": candidates, "decision_source": "planning",
		}); err != nil {
			return sim.Action{}, err
		}
	}
	if err := planner.Trace.Record("decision_selected", p.Tick, p.Self.ID, map[string]any{
		"option_id": chosen.candidate.ID, "action": chosen.candidate.Action, "rule_reason": reason,
		"observation_version": p.ObservationVersion, "horizon_ticks": rules.HorizonTicks,
		"inertia_margin": margin, "scores": scores, "woke_for": p.Self.NextWake,
	}); err != nil {
		return sim.Action{}, err
	}
	return action, planner.Trace.Record("decision_ready", p.Tick, p.Self.ID, map[string]any{"action": action})
}

func pausedBehavior(prior *sim.BehaviorState, selected sim.NeedKey) *sim.BehaviorState {
	if prior == nil || !maintenanceGoal(selected.Measure) {
		return nil
	}
	if prior.Paused != nil {
		return prior.Paused
	}
	if prior.Goal != selected && !maintenanceGoal(prior.Goal.Measure) && len(prior.Steps) > 0 {
		return prior
	}
	return nil
}

func maintenanceGoal(measure string) bool {
	switch measure {
	case "nutrition", "hydration", "provisions", "energy", "hygiene", "health", "wait":
		return true
	}
	return false
}

func mealCue(p sim.Perception) string {
	a := p.Self
	if a.Plan != nil && a.Plan.RemainingToSell() > 0 || len(p.Offers) > 0 || p.BehaviorChanged ||
		a.Behavior != nil && len(a.Behavior.Steps) != 0 {
		return ""
	}
	for _, agreement := range p.Continuity.Agreements {
		if agreement.Status == "active" || agreement.Status == "proposed" {
			return ""
		}
	}
	var cue strings.Builder
	fmt.Fprintf(&cue, "%q/%q/%q/%q/%d/%d/%d/%d/%d/%d/%t",
		a.Location, a.VenueID, a.InsideHouse, a.FormID,
		a.Hunger, a.Thirst, a.NutritionReserve, a.Energy, a.Health, a.Waste, canDrawFreeWater(p))
	for _, id := range sortedIntMapKeys(a.Items) {
		item := p.Catalog.Items[id]
		fmt.Fprintf(&cue, "/%q=%d,%d,%d", id, a.Items[id], item.NutritionUnits, item.ThirstRelief)
	}
	for _, id := range sortedIntMapKeys(p.WorldLaws) {
		fmt.Fprintf(&cue, "/%q=%d", id, p.WorldLaws[id])
	}
	return cue.String()
}

func repairRoute(p sim.Perception, steps []sim.Action) []sim.Action {
	last := 0
	for last < len(steps) && (steps[last].Kind == "move" || steps[last].Kind == "move_venue") {
		last++
	}
	if last == 0 {
		return nil
	}
	destination := steps[last-1]
	district, venue := destination.Location, ""
	if destination.Kind == "move_venue" {
		venue = destination.Location
		site, found := p.Map.VenueByID(venue)
		if !found {
			return nil
		}
		district = site.DistrictID
	}
	route, ok := p.PlanRoute(district, venue)
	if !ok || len(route.Steps) == 0 || route.Energy > p.Self.Energy ||
		p.Tick+route.Ticks >= p.Self.Behavior.ReviewAt {
		return nil
	}
	return append(route.Steps, steps[last:]...)
}

func preferTiedPlan(a, b forecast) bool {
	if a.candidate.Action.Kind == "wait" {
		return true
	}
	return b.candidate.Action.Kind != "wait" && a.duration < b.duration
}

func planningCandidates(p sim.Perception) []sim.Candidate {
	candidates := buildBaseCandidates(p)
	if plan := p.Self.Behavior; plan != nil && len(plan.Steps) > 0 {
		x := plan.Steps[0]
		if !slices.ContainsFunc(candidates, func(c sim.Candidate) bool { return actionMatchesDesired(c.Action, x) }) &&
			p.CheckAction(x).Allowed {
			candidates = append(candidates, sim.Candidate{ID: "continue", Label: "Continue the current plan", Action: x, Source: "behavior"})
		}
	}
	slices.SortStableFunc(candidates, func(a, b sim.Candidate) int {
		// Equal effect plans are ordered by duration in evaluation, then by
		// identity. Candidate generation order carries no preference.
		return strings.Compare(actionKey(a.Action), actionKey(b.Action))
	})
	return candidates
}

func actionKey(x sim.Action) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s/%s/%s/%s/%d/%d/%d/%d/%d", x.Kind, x.TargetID, x.Location,
		x.HouseID, x.RecipeID, x.SkillID, x.TransitionID, x.Item, x.Channel, x.WorkOrderID,
		x.Quantity, x.Price, x.OfferID, x.AgreementID, x.BasisEventID)
}

func followsStoredStep(x sim.Action, plan *sim.BehaviorState) bool {
	return len(plan.Steps) > 0 && actionMatchesDesired(x, plan.Steps[0])
}

func evaluate(needs []needValue, f forecast, rules sim.PlanningRules, explain bool) (float64, []contribution) {
	score, parts := evaluateEffects(needs, f.effects, rules, f.probability, explain)
	if f.probability != 1 && (len(f.unconditional) > 0 || len(f.failure) > 0) {
		certain, certainParts := evaluateEffects(needs, append(slices.Clone(f.unconditional), f.failure...), rules, 1-f.probability, explain)
		score += certain
		parts = append(parts, certainParts...)
	}
	return score, parts
}

func evaluateEffects(needs []needValue, terms []effect, rules sim.PlanningRules, probability float64, explain bool) (float64, []contribution) {
	if len(terms) == 0 || probability == 0 {
		return 0, nil
	}
	sortEffects(terms)
	horizon := float64(rules.HorizonTicks)
	var score float64
	var contributions []contribution
	for _, row := range needs {
		start := slices.IndexFunc(terms, func(term effect) bool { return term.Need == row.NeedKey })
		if start < 0 {
			continue
		}
		interval := horizon
		if row.reviewAfter > 0 {
			interval = min(interval, float64(row.reviewAfter))
		}
		improvement := probability * needImprovement(row, terms[start:], interval, rules.DiscountRate) / interval
		score += row.weight * improvement
		if explain && improvement != 0 {
			contributions = append(contributions, contribution{Need: row.NeedKey, Effect: improvement, Weight: row.weight,
				BaseWeight: row.base, TemporaryBias: row.bias, Fluctuation: row.fluctuation})
		}
	}
	return score, contributions
}

func sortEffects(terms []effect) {
	slices.SortStableFunc(terms, func(a, b effect) int {
		switch {
		case a.At < b.At:
			return -1
		case a.At > b.At:
			return 1
		default:
			return 0
		}
	})
}

func needImprovement(row needValue, terms []effect, horizon, discount float64) float64 {
	baseline := needIntegral(row, 0, horizon, discount)
	if len(row.reference) != 0 {
		baseline = needTrajectoryValue(row, row.reference, horizon, discount)
		terms = slices.Concat(row.reference, terms)
		sortEffects(terms)
	}
	return needTrajectoryValue(row, terms, horizon, discount) - baseline
}

func needTrajectoryValue(row needValue, terms []effect, horizon, discount float64) float64 {
	// Forecast preventable fatigue until the next change in rest conditions.
	// Existing fatigue still needs a recovery plan across that boundary.
	projected := row
	var last, value, terminal float64
	for _, term := range terms {
		if term.Need != row.NeedKey {
			continue
		}
		at := float64(term.At)
		if row.Measure == "energy" && at >= horizon {
			break
		}
		before := projected
		before.level = projected.clamp(projected.level + projected.drift*(at-last))
		delta, rate := term.Change, term.DriftChange
		if row.Minimize {
			delta, rate = -delta, -rate
		}
		level := projected.clamp(before.level + delta)
		if level == before.level && rate == 0 {
			continue
		}
		if last < horizon {
			value += needIntegral(projected, last, min(at, horizon), discount)
		}
		projected.level, projected.drift = level, before.drift+rate
		// Effects after the common interval contribute only their incremental
		// value there, for example when a crop ripens or wages come due.
		if at >= horizon && !term.shared {
			terminal += needIntegral(projected, at, at+horizon, discount) - needIntegral(before, at, at+horizon, discount)
		}
		last = at
	}
	if last < horizon {
		value += needIntegral(projected, last, horizon, discount)
	}
	return value + terminal
}

func (row needValue) clamp(value float64) float64 {
	if row.bounded {
		return max(row.lower, min(row.upper, value))
	}
	return value
}

// Integrate quadratic shortfall until the state hits a physical bound.
// Once it hits that bound, the state stays there until the next effect.
func needIntegral(row needValue, start, end, discount float64) float64 {
	if !row.bounded || row.drift == 0 || start >= end {
		return linearNeedIntegral(row, start, end, discount)
	}
	bound := row.lower
	if row.drift > 0 {
		bound = row.upper
	}
	crossing := start + (bound-row.level)/row.drift
	if crossing >= end {
		return linearNeedIntegral(row, start, end, discount)
	}
	crossing = max(start, crossing)
	before := linearNeedIntegral(row, start, crossing, discount)
	row.level, row.drift = bound, 0
	return before + linearNeedIntegral(row, crossing, end, discount)
}

func linearNeedIntegral(row needValue, start, end, discount float64) float64 {
	missing := row.Target - row.level
	from, until := 0.0, end-start
	if row.drift != 0 {
		crossing := missing / row.drift
		if row.drift > 0 {
			until = min(until, crossing)
		} else {
			from = max(from, crossing)
		}
	} else if missing <= 0 {
		return 0
	}
	if until <= from {
		return 0
	}
	first := (missing - row.drift*from) / row.Scale
	last := (missing - row.drift*until) / row.Scale
	value := -(until - from) * (first*first + first*last + last*last) / 3
	if discount != 0 {
		value *= math.Exp(-discount * (start + (from+until)/2))
	}
	return value
}

func normalPreference(seed int64, actor string, key sim.NeedKey, tick int64) float64 {
	uniform := func(component string) float64 {
		h := fnv.New64a()
		for _, v := range []string{strconv.FormatInt(seed, 10), actor, key.Measure, key.Subject, strconv.FormatInt(tick, 10), "preference", component} {
			fmt.Fprintf(h, "%d:%s", len(v), v)
		}
		return (float64(h.Sum64()>>12) + 0.5) / (1 << 52)
	}
	return math.Sqrt(-2*math.Log(uniform("u"))) * math.Cos(2*math.Pi*uniform("v"))
}

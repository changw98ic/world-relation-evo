package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PlaybookDecisionEvent records one playbook-driven decision (base option, reactive
// counter-option, or purposeful travel step) for replay inspection and auditing.
type PlaybookDecisionEvent struct {
	Tick           int64  `json:"tick"`
	Day            int    `json:"day"`
	Clock          string `json:"clock"`
	ActorID        string `json:"actor_id"`
	ActorName      string `json:"actor_name"`
	Location       string `json:"location"`
	Source         string `json:"source"` // counter | base | travel_for_counter | travel_for_base
	OptionID       string `json:"option_id"`
	Category       string `json:"category,omitempty"`
	ScenarioID     string `json:"scenario_id,omitempty"`
	ScenarioTitle  string `json:"scenario_title,omitempty"`
	CollisionType  string `json:"collision_type,omitempty"`
	TriggerActorID string `json:"trigger_actor_id,omitempty"`
	Stance         string `json:"stance,omitempty"`
	Probability    string `json:"probability_hint,omitempty"`
	Title          string `json:"title"`
	Summary        string `json:"summary,omitempty"`
	IntendedKind   string `json:"intended_kind"`
	ExecutedKind   string `json:"executed_kind"`
	MatchMode      string `json:"match_mode"` // exact | semantic_fallback | travel
	TargetID       string `json:"target_id,omitempty"`
	TargetLocation string `json:"target_location,omitempty"`
	RecipeID       string `json:"recipe_id,omitempty"`
	Item           string `json:"item,omitempty"`
}

type PlaybookRunStats struct {
	TotalDecisions     int            `json:"total_decisions"`
	CounterDecisions   int            `json:"counter_decisions"`
	BaseDecisions      int            `json:"base_decisions"`
	TravelForCounter   int            `json:"travel_for_counter"`
	TravelForBase      int            `json:"travel_for_base"`
	RulesSurvival      int            `json:"rules_survival"`
	RulesFallback      int            `json:"rules_fallback"`
	ExactMatches       int            `json:"exact_matches"`
	SemanticFallbacks  int            `json:"semantic_fallbacks"`
	UniqueBaseOptions  int            `json:"unique_base_options_fired"`
	TotalBaseOptions   int            `json:"total_base_options_loaded"`
	UniqueCounters     int            `json:"unique_counters_fired"`
	TotalCounters      int            `json:"total_counters_loaded"`
	ScenariosActivated int            `json:"scenarios_activated"`
	TotalScenarios     int            `json:"total_scenarios_loaded"`
	ByStance           map[string]int `json:"by_stance"`
	ByCategory         map[string]int `json:"by_category"`
	ByScenario         map[string]int `json:"by_scenario"`
	ByActorBase        map[string]int `json:"by_actor_base"`
	ByActorCounter     map[string]int `json:"by_actor_counter"`
	ByExecutedKind     map[string]int `json:"by_executed_kind"`
	ByIntendedKind     map[string]int `json:"by_intended_kind"`
}

type PlaybookLog struct {
	WorldID     string                  `json:"world_id"`
	OptionsDir  string                  `json:"options_dir"`
	HorizonDays int                     `json:"horizon_days"`
	Environment sim.EnvironmentProfile  `json:"environment"`
	Glossaries  PlaybookGlossaries      `json:"glossaries"`
	Stats       PlaybookRunStats        `json:"stats"`
	Events      []PlaybookDecisionEvent `json:"events"`
}

type scenarioActivation struct {
	ScenarioID      string
	ActivatedTick   int64
	ExpiresTick     int64
	TriggerActorID  string
	TriggerOptionID string
	TriggerTitle    string
	TriggerReason   string
	Responders      map[string]int
}

type pendingTravelIntent struct {
	IsCounter      bool
	ScenarioID     string
	BaseOption     *PlaybookBaseOption
	Counter        *PlaybookCounterEntry
	TriggerActorID string
	TargetLocation string
	ExpiresTick    int64
}

// PlaybookSelector executes a compiled offline Playbook deterministically at runtime
// with zero LLM calls, delegating physiological survival and idle economy to RuleSelector.
type PlaybookSelector struct {
	Playbook *Playbook
	Trace    *runtrace.Writer
	Seed     int64

	mu                    sync.Mutex
	baseCooldownUntil     map[string]int64
	baseFiredCount        map[string]int
	actorNextBaseTick     map[string]int64
	counterCooldownUntil  map[string]int64
	counterFiredCount     map[string]int
	scenarioActorCount    map[string]int
	scenarioCooldownUntil map[string]int64
	activeScenarios       map[string]*scenarioActivation
	everActivatedScn      map[string]bool
	travelIntents         map[string]*pendingTravelIntent
	events                []PlaybookDecisionEvent
	stats                 PlaybookRunStats
}

// NewPlaybookSelector constructs a ready-to-run deterministic PlaybookSelector.
func NewPlaybookSelector(pb *Playbook, trace *runtrace.Writer) *PlaybookSelector {
	totalBase := 0
	for _, ap := range pb.Actors {
		totalBase += len(ap.BaseOptions)
	}
	totalCounters := 0
	for _, scn := range pb.Scenarios {
		totalCounters += len(scn.Responses)
	}
	return &PlaybookSelector{
		Playbook:              pb,
		Trace:                 trace,
		baseCooldownUntil:     map[string]int64{},
		baseFiredCount:        map[string]int{},
		actorNextBaseTick:     map[string]int64{},
		counterCooldownUntil:  map[string]int64{},
		counterFiredCount:     map[string]int{},
		scenarioActorCount:    map[string]int{},
		scenarioCooldownUntil: map[string]int64{},
		activeScenarios:       map[string]*scenarioActivation{},
		everActivatedScn:      map[string]bool{},
		travelIntents:         map[string]*pendingTravelIntent{},
		stats: PlaybookRunStats{
			TotalBaseOptions: totalBase,
			TotalCounters:    totalCounters,
			TotalScenarios:   len(pb.Scenarios),
			ByStance:         map[string]int{},
			ByCategory:       map[string]int{},
			ByScenario:       map[string]int{},
			ByActorBase:      map[string]int{},
			ByActorCounter:   map[string]int{},
			ByExecutedKind:   map[string]int{},
			ByIntendedKind:   map[string]int{},
		},
	}
}

// SnapshotLog returns a deterministic snapshot of runtime telemetry and decision events.
func (s *PlaybookSelector) SnapshotLog() PlaybookLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := s.stats
	stats.UniqueBaseOptions = len(s.baseFiredCount)
	stats.UniqueCounters = len(s.counterFiredCount)
	stats.ScenariosActivated = len(s.everActivatedScn)
	stats.ByStance = cloneIntMap(s.stats.ByStance)
	stats.ByCategory = cloneIntMap(s.stats.ByCategory)
	stats.ByScenario = cloneIntMap(s.stats.ByScenario)
	stats.ByActorBase = cloneIntMap(s.stats.ByActorBase)
	stats.ByActorCounter = cloneIntMap(s.stats.ByActorCounter)
	stats.ByExecutedKind = cloneIntMap(s.stats.ByExecutedKind)
	stats.ByIntendedKind = cloneIntMap(s.stats.ByIntendedKind)

	events := make([]PlaybookDecisionEvent, len(s.events))
	copy(events, s.events)

	return PlaybookLog{
		WorldID:     s.Playbook.WorldID,
		OptionsDir:  s.Playbook.OptionsDir,
		HorizonDays: s.Playbook.HorizonDays,
		Environment: s.Playbook.Environment,
		Glossaries:  s.Playbook.Glossaries,
		Stats:       stats,
		Events:      events,
	}
}

// WriteLog writes the full PlaybookLog JSON to disk.
func (s *PlaybookSelector) WriteLog(path string) error {
	logData := s.SnapshotLog()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(logData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func cloneIntMap(src map[string]int) map[string]int {
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// Select implements Selector with a 4-tier hierarchy:
//
//	Tier 0: Physiological survival & circadian rest (RuleSelector)
//	Tier 1: Reactive combination counter-options when a scenario activates
//	Tier 2: Proactive 30-day character options (with multi-step travel intent)
//	Tier 3: Generic RuleSelector economic/social fallback
func (s *PlaybookSelector) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	if s == nil || s.Playbook == nil {
		return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalDecisions++
	day := p.Clock.Day
	if day < 1 {
		day = int(p.Tick/sim.TicksPerDay) + 1
	}

	// Tier 0: Immediate survival, wound/disease care, waste, and circadian rest.
	if needsSurvivalOverride(p) {
		s.stats.RulesSurvival++
		return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
	}

	// Expire stale scenario activations and detect organic spatial/window convergences.
	s.pruneExpiredScenarios(p.Tick)
	s.detectOrganicConvergence(p, day)

	// Tier 1: Reactive combination counter-options (highest strategic priority).
	if id, ok := s.tryReactiveCounter(p, options, day); ok {
		return id, nil
	}

	// Tier 2a: Continue an active multi-step travel intent if we have one.
	if id, ok := s.tryPendingTravelIntent(p, options, day); ok {
		return id, nil
	}

	// Tier 2b: Proactive 30-day character base option (paced so normal economy thrives in between).
	if id, ok := s.tryBaseOption(p, options, day); ok {
		return id, nil
	}

	// Tier 3: Generic RuleSelector economic/social fallback.
	s.stats.RulesFallback++
	return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
}

func hasCoLocatedHostileEnemy(p sim.Perception) bool {
	a := p.Self
	for _, nb := range p.Nearby {
		if nb.Jailed || nb.Sleeping {
			continue
		}
		if nb.FactionRelation == "at_war" || a.Trust[nb.ID] <= -3 {
			return true
		}
	}
	return false
}

func needsSurvivalOverride(p sim.Perception) bool {
	a := p.Self
	if a.Thirst >= 2 {
		return true
	}
	if a.Hunger >= 3 || (a.Hunger > 0 && totalEdibleItems(p.Catalog, a) > 0) {
		return true
	}
	if a.Waste >= 3 {
		return true
	}
	if a.DiseaseID != "" {
		return true
	}
	if a.Health < 25 || (a.Health < 45 && !hasCoLocatedHostileEnemy(p)) {
		return true
	}
	if a.Resting && a.Energy < sim.MaxEnergy {
		return true
	}
	if a.Energy < 16 {
		return true
	}
	if (p.Clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, p.Clock.Hour)) && a.Energy < 70 && !hasCoLocatedHostileEnemy(p) {
		return true
	}
	return false
}

func (s *PlaybookSelector) pruneExpiredScenarios(tick int64) {
	for sid, act := range s.activeScenarios {
		if tick > act.ExpiresTick {
			delete(s.activeScenarios, sid)
		}
	}
}

func (s *PlaybookSelector) activateScenarioLocked(scn *PlaybookScenario, tick int64, triggerActorID, triggerOptID, triggerTitle, reason string) {
	if scn == nil {
		return
	}
	existing, ok := s.activeScenarios[scn.ID]
	if !ok {
		existing = &scenarioActivation{
			ScenarioID: scn.ID,
			Responders: map[string]int{},
		}
		s.activeScenarios[scn.ID] = existing
	}
	existing.ActivatedTick = tick
	existing.ExpiresTick = tick + 2*sim.TicksPerDay
	existing.TriggerActorID = triggerActorID
	if triggerOptID != "" {
		existing.TriggerOptionID = triggerOptID
	}
	if triggerTitle != "" {
		existing.TriggerTitle = triggerTitle
	}
	existing.TriggerReason = reason
	s.everActivatedScn[scn.ID] = true
}

// detectOrganicConvergence activates a mined combination scenario when its participants
// physically encounter each other (or converge at its shared location) inside its day window.
func (s *PlaybookSelector) detectOrganicConvergence(p sim.Perception, day int) {
	a := p.Self
	scenarios := s.Playbook.ScenariosByActor[a.ID]
	if len(scenarios) == 0 {
		return
	}
	nearbySet := make(map[string]bool, len(p.Nearby))
	for _, nb := range p.Nearby {
		if !nb.Jailed && !nb.Sleeping {
			nearbySet[nb.ID] = true
		}
	}
	for _, scn := range scenarios {
		if _, alreadyActive := s.activeScenarios[scn.ID]; alreadyActive {
			continue
		}
		if day < scn.DayWindow[0] || day > scn.DayWindow[1]+1 {
			continue
		}
		// Require either a physical co-located participant or arrival at shared location during window.
		otherNearby := ""
		for _, pid := range scn.ParticipantIDs {
			if pid != a.ID && nearbySet[pid] {
				otherNearby = pid
				break
			}
		}
		if otherNearby != "" {
			s.activateScenarioLocked(scn, p.Tick, otherNearby, "", scn.Title, "co_located_window")
			continue
		}
		if scn.SharedLocation != "" && a.Location == scn.SharedLocation && !s.everActivatedScn[scn.ID] && day >= scn.DayWindow[0] {
			// Pick another participant as the catalyst so this actor can react/initiate.
			catalyst := ""
			for _, pid := range scn.ParticipantIDs {
				if pid != a.ID {
					catalyst = pid
					break
				}
			}
			if catalyst != "" {
				s.activateScenarioLocked(scn, p.Tick, catalyst, "", scn.Title, "shared_location_window")
			}
		}
	}
}

func (s *PlaybookSelector) tryReactiveCounter(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	scenarios := s.Playbook.ScenariosByActor[a.ID]
	if len(scenarios) == 0 {
		return "", false
	}

	type activeCand struct {
		scn *PlaybookScenario
		act *scenarioActivation
	}
	var cands []activeCand
	for _, scn := range scenarios {
		act, ok := s.activeScenarios[scn.ID]
		if !ok {
			continue
		}
		scnActorKey := a.ID + ":" + scn.ID
		if s.scenarioCooldownUntil[scnActorKey] > p.Tick {
			continue
		}
		if s.scenarioActorCount[scnActorKey] >= 3 {
			continue
		}
		// Prefer reacting to another actor's move; allow self-follow-up only if someone else already reacted.
		if act.TriggerActorID == a.ID && len(act.Responders) == 0 {
			continue
		}
		cands = append(cands, activeCand{scn: scn, act: act})
	}
	if len(cands) == 0 {
		return "", false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		// Unreacted scenarios first, then highest likelihood score, then most recently triggered.
		ri := s.scenarioActorCount[a.ID+":"+cands[i].scn.ID]
		rj := s.scenarioActorCount[a.ID+":"+cands[j].scn.ID]
		if ri != rj {
			return ri < rj
		}
		if cands[i].scn.LikelihoodScore != cands[j].scn.LikelihoodScore {
			return cands[i].scn.LikelihoodScore > cands[j].scn.LikelihoodScore
		}
		return cands[i].act.ActivatedTick > cands[j].act.ActivatedTick
	})

	for _, cand := range cands {
		scn := cand.scn
		counters := s.Playbook.CountersByScenarioActor[scn.ID+":"+a.ID]
		if len(counters) == 0 {
			continue
		}
		ordered := s.orderCountersDeterministically(a.ID, scn.ID, day, p.Tick, counters)
		for idx := range ordered {
			c := ordered[idx]
			cKey := a.ID + ":" + scn.ID + ":" + c.ID
			if s.counterCooldownUntil[cKey] > p.Tick {
				continue
			}
			opt, matchMode, needsTravel := matchPlaybookAction(p, options, c.Action)
			if opt != nil {
				s.commitCounterLocked(p, day, scn, cand.act, c, opt, matchMode)
				return opt.ID, true
			}
			if needsTravel && c.Action.TargetLocation != "" && c.Action.TargetLocation != a.Location {
				if step, ok := stepTowardDistrict(p, c.Action.TargetLocation, 0); ok {
					if stepID := ruleOptionID(step, options); stepID != "" {
						cCopy := c
						s.travelIntents[a.ID] = &pendingTravelIntent{
							IsCounter:      true,
							ScenarioID:     scn.ID,
							Counter:        &cCopy,
							TriggerActorID: cand.act.TriggerActorID,
							TargetLocation: c.Action.TargetLocation,
							ExpiresTick:    p.Tick + 16,
						}
						s.scenarioCooldownUntil[a.ID+":"+scn.ID] = p.Tick + 20
						s.recordTravelEventLocked(p, day, "travel_for_counter", scn, cand.act.TriggerActorID, c.ID, c.Stance, c.ProbabilityHint, "", c.Title, c.Action, step)
						return stepID, true
					}
				}
			}
		}
	}
	return "", false
}

func (s *PlaybookSelector) orderCountersDeterministically(actorID, scenarioID string, day int, tick int64, counters []PlaybookCounterEntry) []PlaybookCounterEntry {
	pool := make([]PlaybookCounterEntry, len(counters))
	copy(pool, counters)
	result := make([]PlaybookCounterEntry, 0, len(pool))
	salt := fmt.Sprintf("%s|%d|%s|%s|%d|%d", s.Playbook.WorldID, s.Seed, actorID, scenarioID, day, tick/24)
	for len(pool) > 0 {
		totalW := 0
		weights := make([]int, len(pool))
		for i, c := range pool {
			w := probabilityHintWeight(c.ProbabilityHint)
			// Prefer counter-options not yet executed in this run to explore diverse branches.
			if s.counterFiredCount[actorID+":"+scenarioID+":"+c.ID] == 0 {
				w *= 3
			}
			weights[i] = w
			totalW += w
		}
		h := fnvHash(fmt.Sprintf("%s|%d", salt, len(result)))
		pick := int(h % uint64(totalW))
		chosen := 0
		acc := 0
		for i, w := range weights {
			acc += w
			if pick < acc {
				chosen = i
				break
			}
		}
		result = append(result, pool[chosen])
		pool = append(pool[:chosen], pool[chosen+1:]...)
	}
	return result
}

func (s *PlaybookSelector) commitCounterLocked(p sim.Perception, day int, scn *PlaybookScenario, act *scenarioActivation, c PlaybookCounterEntry, opt *Option, matchMode string) {
	a := p.Self
	cKey := a.ID + ":" + scn.ID + ":" + c.ID
	scnActorKey := a.ID + ":" + scn.ID
	s.counterCooldownUntil[cKey] = p.Tick + 2*sim.TicksPerDay
	s.counterFiredCount[cKey]++
	s.scenarioCooldownUntil[scnActorKey] = p.Tick + sim.TicksPerDay
	s.scenarioActorCount[scnActorKey]++
	delete(s.travelIntents, a.ID)

	// Chain reaction: hand the scenario baton to this responder so other participants can react!
	triggerActor := act.TriggerActorID
	act.Responders[a.ID]++
	act.TriggerActorID = a.ID
	act.ActivatedTick = p.Tick
	act.ExpiresTick = p.Tick + 2*sim.TicksPerDay

	s.stats.CounterDecisions++
	if matchMode == "exact" {
		s.stats.ExactMatches++
	} else {
		s.stats.SemanticFallbacks++
	}
	if c.Stance != "" {
		s.stats.ByStance[c.Stance]++
	}
	s.stats.ByScenario[scn.ID]++
	s.stats.ByActorCounter[a.ID]++
	s.stats.ByIntendedKind[c.Action.EngineKind]++
	s.stats.ByExecutedKind[opt.Action.Kind]++

	s.events = append(s.events, PlaybookDecisionEvent{
		Tick:           p.Tick,
		Day:            day,
		Clock:          p.Clock.Label,
		ActorID:        a.ID,
		ActorName:      a.Name,
		Location:       a.Location,
		Source:         "counter",
		OptionID:       c.ID,
		ScenarioID:     scn.ID,
		ScenarioTitle:  scn.Title,
		CollisionType:  scn.CollisionType,
		TriggerActorID: triggerActor,
		Stance:         c.Stance,
		Probability:    c.ProbabilityHint,
		Title:          c.Title,
		Summary:        c.Action.Summary,
		IntendedKind:   c.Action.EngineKind,
		ExecutedKind:   opt.Action.Kind,
		MatchMode:      matchMode,
		TargetID:       opt.Action.TargetID,
		TargetLocation: opt.Action.Location,
		RecipeID:       opt.Action.RecipeID,
		Item:           opt.Action.Item,
	})
}

func (s *PlaybookSelector) tryPendingTravelIntent(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	intent, ok := s.travelIntents[a.ID]
	if !ok {
		return "", false
	}
	if p.Tick > intent.ExpiresTick {
		delete(s.travelIntents, a.ID)
		return "", false
	}
	if a.Location == intent.TargetLocation {
		delete(s.travelIntents, a.ID)
		if intent.IsCounter && intent.Counter != nil {
			scn := s.Playbook.Scenarios[intent.ScenarioID]
			act := s.activeScenarios[intent.ScenarioID]
			if scn != nil {
				if act == nil {
					s.activateScenarioLocked(scn, p.Tick, intent.TriggerActorID, "", scn.Title, "travel_arrival")
					act = s.activeScenarios[scn.ID]
				}
				if opt, matchMode, _ := matchPlaybookAction(p, options, intent.Counter.Action); opt != nil {
					s.commitCounterLocked(p, day, scn, act, *intent.Counter, opt, matchMode)
					return opt.ID, true
				}
			}
			return "", false
		}
		if intent.BaseOption != nil {
			if opt, matchMode, _ := matchPlaybookAction(p, options, intent.BaseOption.Action); opt != nil {
				s.commitBaseLocked(p, day, *intent.BaseOption, opt, matchMode)
				return opt.ID, true
			}
		}
		return "", false
	}

	// Keep moving toward target location.
	if step, ok := stepTowardDistrict(p, intent.TargetLocation, 0); ok {
		if stepID := ruleOptionID(step, options); stepID != "" {
			return stepID, true
		}
	}
	delete(s.travelIntents, a.ID)
	return "", false
}

func (s *PlaybookSelector) tryBaseOption(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	if s.actorNextBaseTick[a.ID] > p.Tick {
		return "", false
	}
	actorPB, ok := s.Playbook.Actors[a.ID]
	if !ok || len(actorPB.BaseOptions) == 0 {
		return "", false
	}

	type scoredBase struct {
		opt         PlaybookBaseOption
		score       int
		matchedOpt  *Option
		matchMode   string
		needsTravel bool
	}
	var cands []scoredBase
	for _, bo := range actorPB.BaseOptions {
		if day < bo.DayWindow[0] || day > bo.DayWindow[1] {
			continue
		}
		key := a.ID + ":" + bo.ID
		if s.baseCooldownUntil[key] > p.Tick {
			continue
		}
		maxRepeats := 1
		if bo.Priority == "urgent" || bo.Priority == "high" || bo.Category == "主业经营" || bo.Category == "资源交易" {
			maxRepeats = 2
		}
		fired := s.baseFiredCount[key]
		if fired >= maxRepeats {
			continue
		}

		matched, mode, needsTravel := matchPlaybookAction(p, options, bo.Action)
		if matched == nil && !needsTravel {
			continue
		}

		score := priorityWeight(bo.Priority)
		if fired == 0 {
			score += 80
		}
		span := bo.DayWindow[1] - bo.DayWindow[0] + 1
		if span <= 5 {
			score += 35
		} else if span <= 10 {
			score += 20
		}
		// Boost base options that are registered trigger options of mined scenarios so collisions reliably emerge.
		if len(s.Playbook.ScenariosByTriggerOption[key]) > 0 && fired == 0 {
			score += 45
		}
		if matched != nil {
			if mode == "exact" {
				score += 30
			} else {
				score += 10
			}
		} else if needsTravel {
			score += 5
		}
		// Deterministic daily jitter (0..9) for natural variety across days.
		score += int(fnvHash(fmt.Sprintf("%s|%d|%s|%s|%d", s.Playbook.WorldID, s.Seed, a.ID, bo.ID, day)) % 10)

		cands = append(cands, scoredBase{
			opt:         bo,
			score:       score,
			matchedOpt:  matched,
			matchMode:   mode,
			needsTravel: needsTravel,
		})
	}
	if len(cands) == 0 {
		return "", false
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].opt.ID < cands[j].opt.ID
	})

	for _, cand := range cands {
		if cand.matchedOpt != nil {
			s.commitBaseLocked(p, day, cand.opt, cand.matchedOpt, cand.matchMode)
			return cand.matchedOpt.ID, true
		}
		if cand.needsTravel && cand.opt.Action.TargetLocation != "" && cand.opt.Action.TargetLocation != a.Location {
			if step, ok := stepTowardDistrict(p, cand.opt.Action.TargetLocation, 0); ok {
				if stepID := ruleOptionID(step, options); stepID != "" {
					boCopy := cand.opt
					s.travelIntents[a.ID] = &pendingTravelIntent{
						IsCounter:      false,
						BaseOption:     &boCopy,
						TargetLocation: cand.opt.Action.TargetLocation,
						ExpiresTick:    p.Tick + 16,
					}
					s.actorNextBaseTick[a.ID] = p.Tick + 24
					s.recordTravelEventLocked(p, day, "travel_for_base", nil, "", cand.opt.ID, "", "", cand.opt.Category, cand.opt.Title, cand.opt.Action, step)
					return stepID, true
				}
			}
		}
	}
	return "", false
}

func (s *PlaybookSelector) commitBaseLocked(p sim.Perception, day int, bo PlaybookBaseOption, opt *Option, matchMode string) {
	a := p.Self
	key := a.ID + ":" + bo.ID
	s.baseCooldownUntil[key] = p.Tick + 2*sim.TicksPerDay
	s.baseFiredCount[key]++
	s.actorNextBaseTick[a.ID] = p.Tick + 24

	s.stats.BaseDecisions++
	if matchMode == "exact" {
		s.stats.ExactMatches++
	} else {
		s.stats.SemanticFallbacks++
	}
	if bo.Category != "" {
		s.stats.ByCategory[bo.Category]++
	}
	s.stats.ByActorBase[a.ID]++
	s.stats.ByIntendedKind[bo.Action.EngineKind]++
	s.stats.ByExecutedKind[opt.Action.Kind]++

	// Trigger any mined combination scenario linked to this base option!
	for _, scn := range s.Playbook.ScenariosByTriggerOption[key] {
		s.activateScenarioLocked(scn, p.Tick, a.ID, bo.ID, bo.Title, "trigger_option")
	}

	s.events = append(s.events, PlaybookDecisionEvent{
		Tick:           p.Tick,
		Day:            day,
		Clock:          p.Clock.Label,
		ActorID:        a.ID,
		ActorName:      a.Name,
		Location:       a.Location,
		Source:         "base",
		OptionID:       bo.ID,
		Category:       bo.Category,
		Title:          bo.Title,
		Summary:        bo.Action.Summary,
		IntendedKind:   bo.Action.EngineKind,
		ExecutedKind:   opt.Action.Kind,
		MatchMode:      matchMode,
		TargetID:       opt.Action.TargetID,
		TargetLocation: opt.Action.Location,
		RecipeID:       opt.Action.RecipeID,
		Item:           opt.Action.Item,
	})
}

func (s *PlaybookSelector) recordTravelEventLocked(p sim.Perception, day int, source string, scn *PlaybookScenario, triggerActor, optID, stance, prob, category, title string, spec PlaybookActionSpec, step sim.Action) {
	a := p.Self
	if source == "travel_for_counter" {
		s.stats.TravelForCounter++
	} else {
		s.stats.TravelForBase++
	}
	s.stats.ByExecutedKind[step.Kind]++
	ev := PlaybookDecisionEvent{
		Tick:           p.Tick,
		Day:            day,
		Clock:          p.Clock.Label,
		ActorID:        a.ID,
		ActorName:      a.Name,
		Location:       a.Location,
		Source:         source,
		OptionID:       optID,
		Category:       category,
		TriggerActorID: triggerActor,
		Stance:         stance,
		Probability:    prob,
		Title:          title,
		Summary:        spec.Summary,
		IntendedKind:   spec.EngineKind,
		ExecutedKind:   step.Kind,
		MatchMode:      "travel",
		TargetLocation: spec.TargetLocation,
	}
	if scn != nil {
		ev.ScenarioID = scn.ID
		ev.ScenarioTitle = scn.Title
		ev.CollisionType = scn.CollisionType
	}
	s.events = append(s.events, ev)
}

// matchPlaybookAction resolves a PlaybookActionSpec to a strictly legal Option from
// Options(p), returning (matchedOption, matchMode, needsTravel).
func matchPlaybookAction(p sim.Perception, options []Option, spec PlaybookActionSpec) (*Option, string, bool) {
	a := p.Self
	kind := spec.EngineKind
	if !p.Catalog.IsKnownActionKind(kind) {
		return nil, "", false
	}
	targetActors := make(map[string]bool, len(spec.TargetActorIDs))
	for _, tid := range spec.TargetActorIDs {
		if tid != "" && tid != a.ID {
			targetActors[tid] = true
		}
	}
	targetItems := make(map[string]bool, len(spec.ItemsOrResources))
	for k := range spec.ItemsOrResources {
		if k != "" {
			targetItems[k] = true
		}
	}

	// Explicit travel action.
	if kind == "move" || kind == "move_venue" {
		if spec.TargetLocation != "" && spec.TargetLocation != a.Location {
			if step, ok := stepTowardDistrict(p, spec.TargetLocation, 0); ok {
				if id := ruleOptionID(step, options); id != "" {
					if opt := findOptionByID(options, id); opt != nil {
						return opt, "exact", false
					}
				}
			}
			return nil, "", true
		}
		if opt := bestOptionForKind(options, kind, spec, targetActors, targetItems, false); opt != nil {
			return opt, "exact", false
		}
	}

	needsTarget := len(targetActors) > 0 && p.Catalog.IsTargetedActionKind(kind)
	if spec.TargetLocation == "" || spec.TargetLocation == a.Location || kind == "transmit_message" {
		if opt := bestOptionForKind(options, kind, spec, targetActors, targetItems, needsTarget); opt != nil {
			return opt, "exact", false
		}
	}
	return nil, "", spec.TargetLocation != "" && spec.TargetLocation != a.Location
}

func bestOptionForKind(options []Option, kind string, spec PlaybookActionSpec, targetActors, targetItems map[string]bool, requireTargetMatch bool) *Option {
	var best *Option
	bestScore := -1
	for i := range options {
		o := &options[i]
		if o.Action.Kind != kind {
			continue
		}
		if (requireTargetMatch && !targetActors[o.Action.TargetID]) || (spec.RecipeID != "" && o.Action.RecipeID != spec.RecipeID) {
			continue
		}
		score := 10
		if spec.RecipeID != "" && o.Action.RecipeID == spec.RecipeID {
			score += 100
		}
		if o.Action.TargetID != "" && targetActors[o.Action.TargetID] {
			score += 60
		}
		if o.Action.Item != "" && targetItems[o.Action.Item] {
			score += 40
		}
		if spec.TargetLocation != "" && o.Action.Location == spec.TargetLocation {
			score += 30
		}
		if score > bestScore {
			bestScore = score
			best = o
		}
	}
	return best
}

func findOptionByID(options []Option, id string) *Option {
	for i := range options {
		if options[i].ID == id {
			return &options[i]
		}
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

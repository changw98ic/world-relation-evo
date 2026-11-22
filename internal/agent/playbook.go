package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Playbook holds offline-expanded 30-day base character options and multi-actor
// combination counter-options compiled into a deterministic, zero-LLM runtime table.
type Playbook struct {
	WorldID                  string                            `json:"world_id"`
	OptionsDir               string                            `json:"options_dir"`
	HorizonDays              int                               `json:"horizon_days"`
	Environment              sim.EnvironmentProfile            `json:"environment"`
	Glossaries               PlaybookGlossaries                `json:"glossaries"`
	Actors                   map[string]*ActorPlaybook         `json:"actors"`
	Scenarios                map[string]*PlaybookScenario      `json:"scenarios"`
	ScenariosByActor         map[string][]*PlaybookScenario    `json:"-"`
	ScenariosByTriggerOption map[string][]*PlaybookScenario    `json:"-"`
	CountersByScenarioActor  map[string][]PlaybookCounterEntry `json:"-"`
}

// PlaybookGlossaries carries localized labels from the world pack / offline dossiers
// so runtime logs stay environment-agnostic with zero hardcoded setting names.
type PlaybookGlossaries struct {
	Actors      map[string]string `json:"actors,omitempty"`
	Locations   map[string]string `json:"locations,omitempty"`
	Items       map[string]string `json:"items,omitempty"`
	Professions map[string]string `json:"professions,omitempty"`
	Factions    map[string]string `json:"factions,omitempty"`
	Recipes     map[string]string `json:"recipes,omitempty"`
}

type ActorPlaybook struct {
	ActorID     string               `json:"actor_id"`
	ActorName   string               `json:"actor_name"`
	Role        string               `json:"role"`
	Location    string               `json:"location"`
	BaseOptions []PlaybookBaseOption `json:"base_options"`
}

type PlaybookActionSpec struct {
	EngineKind       string         `json:"engine_kind"`
	RecipeID         string         `json:"recipe_id,omitempty"`
	TargetActorIDs   []string       `json:"target_actor_ids,omitempty"`
	TargetLocation   string         `json:"target_location,omitempty"`
	ItemsOrResources map[string]any `json:"items_or_resources,omitempty"`
	Summary          string         `json:"summary"`
}

type PlaybookBaseOption struct {
	ID         string             `json:"id"`
	Category   string             `json:"category"`
	Title      string             `json:"title"`
	DayWindow  [2]int             `json:"day_window"`
	Priority   string             `json:"priority"`
	Action     PlaybookActionSpec `json:"action"`
	Conditions []string           `json:"conditions,omitempty"`
	Trigger    string             `json:"trigger_event,omitempty"`
}

type PlaybookScenario struct {
	ID              string                   `json:"id"`
	CollisionType   string                   `json:"collision_type"`
	Title           string                   `json:"title"`
	LikelihoodScore int                      `json:"likelihood_score"`
	LikelihoodTier  string                   `json:"likelihood_tier"`
	DayWindow       [2]int                   `json:"day_window"`
	SharedLocation  string                   `json:"shared_location,omitempty"`
	SharedItems     []string                 `json:"shared_items,omitempty"`
	ParticipantIDs  []string                 `json:"participant_ids"`
	TriggerOptions  []PlaybookTriggerRef     `json:"trigger_options"`
	Responses       []PlaybookCounterEntry   `json:"responses"`
	Summary         string                   `json:"collision_summary,omitempty"`
}

type PlaybookTriggerRef struct {
	ActorID        string `json:"actor_id"`
	OptionID       string `json:"option_id"`
	Title          string `json:"title"`
	EngineKind     string `json:"engine_kind"`
	TargetLocation string `json:"target_location,omitempty"`
}

type PlaybookCounterEntry struct {
	ID                 string             `json:"id"`
	ResponderActorID   string             `json:"responder_actor_id"`
	ResponderActorName string             `json:"responder_actor_name"`
	Stance             string             `json:"stance"`
	Title              string             `json:"title"`
	TriggerCondition   string             `json:"trigger_condition"`
	ProbabilityHint    string             `json:"probability_hint"`
	CounterToOptionIDs []string           `json:"counter_to_option_ids,omitempty"`
	Action             PlaybookActionSpec `json:"action"`
}

// JSON disk-loading schemas (self-contained to avoid import cycle with internal/optiongen).
type diskOptionsSummary struct {
	WorldID     string `json:"world_id"`
	HorizonDays int    `json:"horizon_days"`
	Actors      []struct {
		ActorID    string `json:"actor_id"`
		ActorName  string `json:"actor_name"`
		Role       string `json:"role"`
		Location   string `json:"location"`
		OutputFile string `json:"output_file"`
	} `json:"actors"`
}

type diskActorExpansion struct {
	WorldID   string `json:"world_id"`
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Dossier   struct {
		World struct {
			Environment sim.EnvironmentProfile `json:"environment"`
			Glossaries  PlaybookGlossaries     `json:"glossaries"`
		} `json:"world"`
		Actor struct {
			Role     string `json:"role"`
			Location string `json:"location"`
		} `json:"actor"`
	} `json:"dossier"`
	Catalog struct {
		Options []struct {
			ID        string             `json:"id"`
			Category  string             `json:"category"`
			Title     string             `json:"title"`
			DayWindow [2]int             `json:"day_window"`
			Priority  string             `json:"priority"`
			Action    PlaybookActionSpec `json:"action"`
		} `json:"options"`
	} `json:"catalog"`
}

type diskResponsesSummary struct {
	Scenarios []struct {
		ScenarioID string `json:"scenario_id"`
		OutputFile string `json:"output_file"`
	} `json:"scenarios"`
}

type diskScenarioRecord struct {
	Scenario struct {
		ID              string   `json:"id"`
		CollisionType   string   `json:"collision_type"`
		Title           string   `json:"title"`
		LikelihoodScore int      `json:"likelihood_score"`
		LikelihoodTier  string   `json:"likelihood_tier"`
		DayWindow       [2]int   `json:"day_window"`
		SharedLocation  string   `json:"shared_location"`
		SharedItems     []string `json:"shared_items"`
		Participants    []struct {
			ActorID   string `json:"actor_id"`
			ActorName string `json:"actor_name"`
		} `json:"participants"`
		TriggerOptions []PlaybookTriggerRef `json:"trigger_options"`
	} `json:"scenario"`
	Catalog struct {
		ScenarioID       string                 `json:"scenario_id"`
		ScenarioTitle    string                 `json:"scenario_title"`
		CollisionSummary string                 `json:"collision_summary"`
		Responses        []PlaybookCounterEntry `json:"responses"`
	} `json:"catalog"`
}

// LoadPlaybook loads and compiles an offline expand-options directory (and its
// optional responses/ subdirectory from expand-responses) into a runtime Playbook.
func LoadPlaybook(optionsDir string) (*Playbook, error) {
	if strings.TrimSpace(optionsDir) == "" {
		return nil, fmt.Errorf("playbook options directory is required")
	}
	summaryPath := filepath.Join(optionsDir, "summary.json")
	rawSum, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, fmt.Errorf("read playbook summary %s: %w", summaryPath, err)
	}
	var sum diskOptionsSummary
	if err := json.Unmarshal(rawSum, &sum); err != nil {
		return nil, fmt.Errorf("parse playbook summary %s: %w", summaryPath, err)
	}
	pb := &Playbook{
		WorldID:     sum.WorldID,
		OptionsDir:  optionsDir,
		HorizonDays: sum.HorizonDays,
		Glossaries: PlaybookGlossaries{
			Actors:      map[string]string{},
			Locations:   map[string]string{},
			Items:       map[string]string{},
			Professions: map[string]string{},
			Factions:    map[string]string{},
			Recipes:     map[string]string{},
		},
		Actors:                   map[string]*ActorPlaybook{},
		Scenarios:                map[string]*PlaybookScenario{},
		ScenariosByActor:         map[string][]*PlaybookScenario{},
		ScenariosByTriggerOption: map[string][]*PlaybookScenario{},
		CountersByScenarioActor:  map[string][]PlaybookCounterEntry{},
	}
	if pb.HorizonDays <= 0 {
		pb.HorizonDays = 30
	}

	envLoaded := false
	for _, brief := range sum.Actors {
		rel := brief.OutputFile
		if rel == "" {
			rel = filepath.Join("actors", brief.ActorID+".json")
		}
		actorPath := filepath.Join(optionsDir, rel)
		raw, err := os.ReadFile(actorPath)
		if err != nil {
			return nil, fmt.Errorf("read actor playbook %s: %w", actorPath, err)
		}
		var rec diskActorExpansion
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("parse actor playbook %s: %w", actorPath, err)
		}
		if !envLoaded {
			pb.Environment = rec.Dossier.World.Environment
			mergeStringMap(pb.Glossaries.Locations, rec.Dossier.World.Glossaries.Locations)
			mergeStringMap(pb.Glossaries.Items, rec.Dossier.World.Glossaries.Items)
			mergeStringMap(pb.Glossaries.Professions, rec.Dossier.World.Glossaries.Professions)
			mergeStringMap(pb.Glossaries.Factions, rec.Dossier.World.Glossaries.Factions)
			mergeStringMap(pb.Glossaries.Recipes, rec.Dossier.World.Glossaries.Recipes)
			envLoaded = true
		}
		actorID := rec.ActorID
		if actorID == "" {
			actorID = brief.ActorID
		}
		actorName := rec.ActorName
		if actorName == "" {
			actorName = brief.ActorName
		}
		pb.Glossaries.Actors[actorID] = actorName
		opts := make([]PlaybookBaseOption, 0, len(rec.Catalog.Options))
		for _, o := range rec.Catalog.Options {
			w0, w1 := o.DayWindow[0], o.DayWindow[1]
			if w0 <= 0 {
				w0 = 1
			}
			if w1 < w0 {
				w1 = pb.HorizonDays
			}
			opts = append(opts, PlaybookBaseOption{
				ID:        o.ID,
				Category:  o.Category,
				Title:     o.Title,
				DayWindow: [2]int{w0, w1},
				Priority:  strings.ToLower(strings.TrimSpace(o.Priority)),
				Action:    o.Action,
			})
		}
		sort.SliceStable(opts, func(i, j int) bool {
			if pi, pj := priorityWeight(opts[i].Priority), priorityWeight(opts[j].Priority); pi != pj {
				return pi > pj
			}
			if opts[i].DayWindow[0] != opts[j].DayWindow[0] {
				return opts[i].DayWindow[0] < opts[j].DayWindow[0]
			}
			return opts[i].ID < opts[j].ID
		})
		role := rec.Dossier.Actor.Role
		if role == "" {
			role = brief.Role
		}
		loc := rec.Dossier.Actor.Location
		if loc == "" {
			loc = brief.Location
		}
		pb.Actors[actorID] = &ActorPlaybook{
			ActorID:     actorID,
			ActorName:   actorName,
			Role:        role,
			Location:    loc,
			BaseOptions: opts,
		}
	}
	if len(pb.Actors) == 0 {
		return nil, fmt.Errorf("no actor playbooks loaded from %s", optionsDir)
	}

	// Optionally load combination counter-options from <optionsDir>/responses/
	respDir := filepath.Join(optionsDir, "responses")
	respSummaryPath := filepath.Join(respDir, "summary.json")
	if rawRespSum, err := os.ReadFile(respSummaryPath); err == nil {
		var rsum diskResponsesSummary
		if err := json.Unmarshal(rawRespSum, &rsum); err != nil {
			return nil, fmt.Errorf("parse responses summary %s: %w", respSummaryPath, err)
		}
		for _, brief := range rsum.Scenarios {
			rel := brief.OutputFile
			if rel == "" {
				rel = filepath.Join("scenarios", brief.ScenarioID+".json")
			}
			scnPath := filepath.Join(respDir, rel)
			rawScn, err := os.ReadFile(scnPath)
			if err != nil {
				return nil, fmt.Errorf("read scenario responses %s: %w", scnPath, err)
			}
			var rec diskScenarioRecord
			if err := json.Unmarshal(rawScn, &rec); err != nil {
				return nil, fmt.Errorf("parse scenario responses %s: %w", scnPath, err)
			}
			scnID := rec.Scenario.ID
			if scnID == "" {
				scnID = brief.ScenarioID
			}
			title := rec.Catalog.ScenarioTitle
			if title == "" {
				title = rec.Scenario.Title
			}
			w0, w1 := rec.Scenario.DayWindow[0], rec.Scenario.DayWindow[1]
			if w0 <= 0 {
				w0 = 1
			}
			if w1 < w0 {
				w1 = pb.HorizonDays
			}
			partIDs := make([]string, 0, len(rec.Scenario.Participants))
			seenPart := map[string]bool{}
			for _, part := range rec.Scenario.Participants {
				if part.ActorID == "" || seenPart[part.ActorID] {
					continue
				}
				seenPart[part.ActorID] = true
				partIDs = append(partIDs, part.ActorID)
				if part.ActorName != "" && pb.Glossaries.Actors[part.ActorID] == "" {
					pb.Glossaries.Actors[part.ActorID] = part.ActorName
				}
			}
			scn := &PlaybookScenario{
				ID:              scnID,
				CollisionType:   rec.Scenario.CollisionType,
				Title:           title,
				LikelihoodScore: rec.Scenario.LikelihoodScore,
				LikelihoodTier:  rec.Scenario.LikelihoodTier,
				DayWindow:       [2]int{w0, w1},
				SharedLocation:  rec.Scenario.SharedLocation,
				SharedItems:     append([]string(nil), rec.Scenario.SharedItems...),
				ParticipantIDs:  partIDs,
				TriggerOptions:  append([]PlaybookTriggerRef(nil), rec.Scenario.TriggerOptions...),
				Responses:       append([]PlaybookCounterEntry(nil), rec.Catalog.Responses...),
				Summary:         rec.Catalog.CollisionSummary,
			}
			pb.Scenarios[scn.ID] = scn
			for _, pid := range scn.ParticipantIDs {
				pb.ScenariosByActor[pid] = append(pb.ScenariosByActor[pid], scn)
			}
			for _, trig := range scn.TriggerOptions {
				key := trig.ActorID + ":" + trig.OptionID
				pb.ScenariosByTriggerOption[key] = append(pb.ScenariosByTriggerOption[key], scn)
			}
			for _, rsp := range scn.Responses {
				key := scn.ID + ":" + rsp.ResponderActorID
				pb.CountersByScenarioActor[key] = append(pb.CountersByScenarioActor[key], rsp)
			}
		}
		for pid := range pb.ScenariosByActor {
			sort.SliceStable(pb.ScenariosByActor[pid], func(i, j int) bool {
				if pb.ScenariosByActor[pid][i].LikelihoodScore != pb.ScenariosByActor[pid][j].LikelihoodScore {
					return pb.ScenariosByActor[pid][i].LikelihoodScore > pb.ScenariosByActor[pid][j].LikelihoodScore
				}
				return pb.ScenariosByActor[pid][i].ID < pb.ScenariosByActor[pid][j].ID
			})
		}
	}

	return pb, nil
}

func mergeStringMap(dst, src map[string]string) {
	for k, v := range src {
		if k != "" && v != "" {
			dst[k] = v
		}
	}
}

func priorityWeight(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "urgent":
		return 40
	case "high":
		return 30
	case "medium":
		return 20
	case "low":
		return 10
	default:
		return 15
	}
}

func probabilityHintWeight(hint string) int {
	h := strings.TrimSpace(hint)
	switch {
	case strings.Contains(h, "高") || strings.EqualFold(h, "high"):
		return 60
	case strings.Contains(h, "中") || strings.EqualFold(h, "medium"):
		return 30
	case strings.Contains(h, "低") || strings.EqualFold(h, "low"):
		return 10
	default:
		return 25
	}
}

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
	TotalDecisions      int            `json:"total_decisions"`
	CounterDecisions    int            `json:"counter_decisions"`
	BaseDecisions       int            `json:"base_decisions"`
	TravelForCounter    int            `json:"travel_for_counter"`
	TravelForBase       int            `json:"travel_for_base"`
	RulesSurvival       int            `json:"rules_survival"`
	RulesFallback       int            `json:"rules_fallback"`
	ExactMatches        int            `json:"exact_matches"`
	SemanticFallbacks   int            `json:"semantic_fallbacks"`
	UniqueBaseOptions   int            `json:"unique_base_options_fired"`
	TotalBaseOptions    int            `json:"total_base_options_loaded"`
	UniqueCounters      int            `json:"unique_counters_fired"`
	TotalCounters       int            `json:"total_counters_loaded"`
	ScenariosActivated  int            `json:"scenarios_activated"`
	TotalScenarios      int            `json:"total_scenarios_loaded"`
	ByStance            map[string]int `json:"by_stance"`
	ByCategory          map[string]int `json:"by_category"`
	ByScenario          map[string]int `json:"by_scenario"`
	ByActorBase         map[string]int `json:"by_actor_base"`
	ByActorCounter      map[string]int `json:"by_actor_counter"`
	ByExecutedKind      map[string]int `json:"by_executed_kind"`
	ByIntendedKind      map[string]int `json:"by_intended_kind"`
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
//   Tier 0: Physiological survival & circadian rest (RuleSelector)
//   Tier 1: Reactive combination counter-options when a scenario activates
//   Tier 2: Proactive 30-day character options (with multi-step travel intent)
//   Tier 3: Generic RuleSelector economic/social fallback
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
			opt, matchMode, needsTravel := matchPlaybookAction(p, options, c.Action, true)
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
				if opt, matchMode, _ := matchPlaybookAction(p, options, intent.Counter.Action, true); opt != nil {
					s.commitCounterLocked(p, day, scn, act, *intent.Counter, opt, matchMode)
					return opt.ID, true
				}
			}
			return "", false
		}
		if intent.BaseOption != nil {
			if opt, matchMode, _ := matchPlaybookAction(p, options, intent.BaseOption.Action, true); opt != nil {
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

		// Allow semantic fallback on base options when either already at target location or narrow window closing.
		allowFallback := bo.Action.TargetLocation == "" || bo.Action.TargetLocation == a.Location || (bo.DayWindow[1]-day <= 2)
		matched, mode, needsTravel := matchPlaybookAction(p, options, bo.Action, allowFallback)
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
func matchPlaybookAction(p sim.Perception, options []Option, spec PlaybookActionSpec, allowSemanticFallback bool) (*Option, string, bool) {
	a := p.Self
	kind := strings.TrimSpace(spec.EngineKind)
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

	// 1. Try exact EngineKind match.
	if !strings.HasPrefix(kind, "new_primitive:") {
		requireTargetMatch := len(targetActors) > 0 && isTargetedActionKind(kind) && spec.TargetLocation != "" && spec.TargetLocation != a.Location
		if opt := bestOptionForKind(options, kind, spec, targetActors, targetItems, requireTargetMatch); opt != nil {
			// For craft with a specific recipe_id, require matching recipe_id for exact match.
			if kind != "craft" || spec.RecipeID == "" || opt.Action.RecipeID == spec.RecipeID {
				return opt, "exact", false
			}
		}
	}

	// 2. If the action targets another district and we aren't there yet, request travel
	//    before falling back to an imprecise local action (unless transmit_message can reach directly).
	if spec.TargetLocation != "" && spec.TargetLocation != a.Location {
		if kind == "transmit_message" {
			if opt := bestOptionForKind(options, "transmit_message", spec, targetActors, targetItems, len(targetActors) > 0); opt != nil {
				return opt, "exact", false
			}
		}
		return nil, "", true
	}

	if !allowSemanticFallback {
		return nil, "", false
	}

	// 3. Semantic fallback: map new_primitive:* or precondition-blocked native kinds
	//    to the closest legal engine action while preserving full playbook telemetry.
	fallbackKinds := semanticFallbackKinds(kind)
	for _, fk := range fallbackKinds {
		if fk == "move" {
			// For evasion/scouting, pick any legal road step away from current location.
			for i := range options {
				if options[i].Action.Kind == "move" || options[i].Action.Kind == "move_venue" {
					return &options[i], "semantic_fallback", false
				}
			}
			continue
		}
		// Prefer target-actor-matched fallback first!
		if len(targetActors) > 0 && isTargetedActionKind(fk) {
			if opt := bestOptionForKind(options, fk, spec, targetActors, targetItems, true); opt != nil {
				return opt, "semantic_fallback", false
			}
		}
	}
	// Second pass: allow non-target-matched fallback (e.g., maintain_infra, craft, broadcast transmit_message, speak to nearby ally).
	for _, fk := range fallbackKinds {
		if fk == "move" {
			continue
		}
		if opt := bestOptionForKind(options, fk, spec, targetActors, targetItems, false); opt != nil {
			return opt, "semantic_fallback", false
		}
	}

	return nil, "", false
}

func isTargetedActionKind(kind string) bool {
	switch kind {
	case "speak", "transmit_message", "offer", "propose_agreement", "accept", "reject",
		"accept_agreement", "attack", "spar", "steal", "extort", "raid", "bribe",
		"audit_corruption", "arrest", "frame_suspect", "file_appeal", "heal",
		"swear_bond", "marry", "avenge_feud", "teach_knowledge", "borrow_loan":
		return true
	default:
		return false
	}
}

func bestOptionForKind(options []Option, kind string, spec PlaybookActionSpec, targetActors, targetItems map[string]bool, requireTargetMatch bool) *Option {
	var best *Option
	bestScore := -1
	for i := range options {
		o := &options[i]
		if o.Action.Kind != kind {
			continue
		}
		if requireTargetMatch && len(targetActors) > 0 && !targetActors[o.Action.TargetID] {
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

// semanticFallbackKinds maps both generic new_primitive:* verbs and precondition-gated
// native kinds to ordered legal engine action families (100% world-agnostic).
func semanticFallbackKinds(kind string) []string {
	if strings.HasPrefix(kind, "new_primitive:") {
		verb := strings.ToLower(strings.TrimPrefix(kind, "new_primitive:"))
		switch {
		case containsAny(verb, "promotion", "advance"):
			return []string{"join_profession", "join_faction", "speak", "transmit_message"}
		case containsAny(verb, "rumor", "tip", "misinformation"):
			return []string{"transmit_message", "speak"}
		case containsAny(verb, "ambush", "ward", "trap"):
			return []string{"raid", "attack", "steal", "hunt_threat", "spar", "speak"}
		case containsAny(verb, "scout", "track", "escort"):
			return []string{"speak", "transmit_message", "move"}
		case containsAny(verb, "evade", "hide", "cut_ties", "destroy"):
			return []string{"abandon_plan", "reject", "move", "transmit_message"}
		case containsAny(verb, "evidence", "inquiry", "seal"):
			return []string{"audit_corruption", "file_appeal", "issue_decree", "arrest", "transmit_message", "speak"}
		case containsAny(verb, "hoard", "stockpile", "shop", "stall", "lease", "ledger", "accounts", "treasury", "grant"):
			return []string{"start_plan", "offer", "propose_agreement", "craft", "speak", "transmit_message"}
		default:
			return []string{"speak", "transmit_message", "offer", "craft"}
		}
	}

	switch kind {
	case "speak":
		return []string{"transmit_message"}
	case "transmit_message":
		return []string{"speak"}
	case "propose_agreement":
		return []string{"offer", "speak", "transmit_message"}
	case "accept_agreement":
		return []string{"accept", "speak", "transmit_message"}
	case "accept":
		return []string{"accept_agreement", "speak"}
	case "reject":
		return []string{"abandon_plan", "transmit_message", "speak"}
	case "abandon_plan":
		return []string{"reject", "transmit_message", "speak"}
	case "offer", "bribe", "borrow_loan":
		return []string{"offer", "propose_agreement", "speak", "transmit_message"}
	case "start_plan":
		return []string{"craft", "offer", "speak"}
	case "craft":
		return []string{"craft", "maintain_infra", "start_plan"}
	case "repair_house", "upgrade_house":
		return []string{"maintain_infra", "craft", "speak"}
	case "hunt_threat":
		return []string{"maintain_infra", "spar", "speak"}
	case "audit_corruption", "file_appeal", "frame_suspect", "issue_decree":
		return []string{"issue_decree", "audit_corruption", "file_appeal", "arrest", "transmit_message", "speak"}
	case "arrest", "attack", "raid", "steal", "spar":
		return []string{"arrest", "spar", "attack", "raid", "steal", "speak", "transmit_message"}
	case "research_tech", "teach_knowledge":
		return []string{"teach_knowledge", "research_tech", "craft", "speak", "transmit_message"}
	case "join_faction", "join_profession":
		return []string{"join_faction", "join_profession", "speak", "transmit_message"}
	case "swear_bond", "mediate_feud", "heal":
		return []string{"speak", "transmit_message"}
	case "maintain_infra":
		return []string{"repair_house", "craft", "speak"}
	default:
		return []string{"speak", "transmit_message"}
	}
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

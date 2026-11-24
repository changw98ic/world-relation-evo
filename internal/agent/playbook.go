package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	ID              string                 `json:"id"`
	CollisionType   string                 `json:"collision_type"`
	Title           string                 `json:"title"`
	LikelihoodScore int                    `json:"likelihood_score"`
	LikelihoodTier  string                 `json:"likelihood_tier"`
	DayWindow       [2]int                 `json:"day_window"`
	SharedLocation  string                 `json:"shared_location,omitempty"`
	SharedItems     []string               `json:"shared_items,omitempty"`
	ParticipantIDs  []string               `json:"participant_ids"`
	TriggerOptions  []PlaybookTriggerRef   `json:"trigger_options"`
	Responses       []PlaybookCounterEntry `json:"responses"`
	Summary         string                 `json:"collision_summary,omitempty"`
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
			DayWindow []int              `json:"day_window"`
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
		DayWindow       []int    `json:"day_window"`
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
func LoadPlaybook(optionsDir string, catalog sim.Catalog) (*Playbook, error) {
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
			if err := validatePlaybookAction(o.Action, catalog); err != nil {
				return nil, fmt.Errorf("%s option %q: %w", actorPath, o.ID, err)
			}
			window, err := ResolvePlanningDayWindow(o.DayWindow, pb.HorizonDays)
			if err != nil {
				return nil, fmt.Errorf("%s option %q: %w", actorPath, o.ID, err)
			}
			opts = append(opts, PlaybookBaseOption{
				ID:        o.ID,
				Category:  o.Category,
				Title:     o.Title,
				DayWindow: window,
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
			window, err := ResolvePlanningDayWindow(rec.Scenario.DayWindow, pb.HorizonDays)
			if err != nil {
				return nil, fmt.Errorf("%s scenario %q: %w", scnPath, scnID, err)
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
				DayWindow:       window,
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
				if err := validatePlaybookAction(rsp.Action, catalog); err != nil {
					return nil, fmt.Errorf("%s response %q: %w", scnPath, rsp.ID, err)
				}
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
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read responses summary %s: %w", respSummaryPath, err)
	}

	return pb, nil
}

// ResolvePlanningDayWindow uses the planning horizon when no window is supplied.
func ResolvePlanningDayWindow(days []int, horizon int) ([2]int, error) {
	if days == nil {
		return [2]int{1, horizon}, nil
	}
	if len(days) != 2 || days[0] < 1 || days[1] < days[0] {
		return [2]int{}, fmt.Errorf("invalid day_window %v: expected a positive start and an end at or after it", days)
	}
	return [2]int{days[0], days[1]}, nil
}

func validatePlaybookAction(action PlaybookActionSpec, catalog sim.Catalog) error {
	if !catalog.IsKnownActionKind(action.EngineKind) {
		return fmt.Errorf("unknown engine_kind %q", action.EngineKind)
	}
	if action.RecipeID != "" {
		if _, known := catalog.RecipeByID(action.RecipeID); !known {
			return fmt.Errorf("unknown recipe_id %q", action.RecipeID)
		}
	}
	return nil
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

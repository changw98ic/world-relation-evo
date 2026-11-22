package runtrace

import (
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// ChronicleArc groups causally connected events sharing one RootEventID into a compact narrative episode.
type ChronicleArc struct {
	RootEventID  int64                    `json:"root_event_id"`
	RootKind     string                   `json:"root_kind"`
	StartDay     int64                    `json:"start_day"`
	EndDay       int64                    `json:"end_day"`
	StartTick    int64                    `json:"start_tick"`
	EndTick      int64                    `json:"end_tick"`
	Salience     int                      `json:"salience"`
	Summary      string                   `json:"summary"`
	Participants []string                 `json:"participants,omitempty"`
	EventCount   int                      `json:"event_count"`
	KeyEvents    []ChronicleTimelineEntry `json:"key_events"`
}

// DramaStats gives an LLM director a quick quantitative read on how much narrative drama occurred.
type DramaStats struct {
	ArcCount           int `json:"arc_count"`
	Deaths             int `json:"deaths"`
	WarsAndClashes     int `json:"wars_and_clashes"`
	AlliancesAndPeace  int `json:"alliances_and_peace"`
	SecretsAndPlots    int `json:"secrets_and_plots"`
	Betrayals          int `json:"betrayals"`
	WorldEvents        int `json:"world_events"`
	AmbitionsCompleted int `json:"ambitions_completed"`
	Transitions        int `json:"transitions"`
}

// EventQueryFilter controls selective historical drill-down for LLM chronicle writers.
type EventQueryFilter struct {
	ActorID        string
	BetweenA       string
	BetweenB       string
	ArcRootID      int64
	FromDay        int64
	ToDay          int64
	Kinds          []string
	IncludeRoutine bool
	Limit          int
}

// QueryResult holds filtered chronological entries ready for concise LLM consumption.
type QueryResult struct {
	TotalMatched int                      `json:"total_matched"`
	Truncated    bool                     `json:"truncated,omitempty"`
	Entries      []ChronicleTimelineEntry `json:"entries"`
}

var routineEventKinds = map[string]bool{
	"waited":        true,
	"ate":           true,
	"drank":         true,
	"slept":         true,
	"rested":        true,
	"rest_started":  true,
	"rest_ended":    true,
	"relieved":      true,
	"moved":         true,
	"moved_venue":   true,
	"entered_house": true,
	"left_house":    true,
	"task_started":  true,
	"action_failed": true,
}

func arcEventSalience(kind string) int {
	switch kind {
	case "actor_died", "faction_war_declared", "revolt_erupted", "party_betrayed", "faction_leader_changed":
		return 30
	case "world_event_triggered", "world_event_claimed", "secret_exposed", "peace_treaty_signed", "alliance_formed", "transition_succeeded":
		return 22
	case "blackmailed", "blackmail_refused", "secret_discovered", "grudge_formed", "feud_avenged", "vendetta_clash", "cohort_turf_war", "ambition_completed", "corruption_purged", "wrongful_conviction":
		return 15
	case "attacked", "raided", "arrested", "bribed", "party_formed", "actor_married", "bond_sworn", "actor_entered_world":
		return 10
	case "speech", "channel_message", "channel_transmitted":
		return 4
	default:
		return 2
	}
}

// BuildChronicleArcs clusters major & causal events by RootEventID into ranked narrative arcs.
func BuildChronicleArcs(sortedEvents []sim.Event) ([]ChronicleArc, DramaStats) {
	var stats DramaStats
	byRoot := map[int64]*ChronicleArc{}
	partSet := map[int64]map[string]bool{}
	eventByID := make(map[int64]sim.Event, len(sortedEvents))
	for _, e := range sortedEvents {
		eventByID[e.ID] = e
	}

	for _, e := range sortedEvents {
		switch e.Kind {
		case "actor_died":
			stats.Deaths++
		case "faction_war_declared", "faction_war_skirmish", "cohort_turf_war", "vendetta_clash", "feud_avenged":
			stats.WarsAndClashes++
		case "alliance_formed", "peace_treaty_signed":
			stats.AlliancesAndPeace++
		case "secret_discovered", "blackmailed", "blackmail_refused", "secret_exposed":
			stats.SecretsAndPlots++
		case "party_betrayed":
			stats.Betrayals++
		case "world_event_triggered", "world_event_claimed":
			stats.WorldEvents++
		case "ambition_completed":
			stats.AmbitionsCompleted++
		case "transition_succeeded":
			stats.Transitions++
		}

		if routineEventKinds[e.Kind] {
			continue
		}
		rootID := eventRootID(e)
		if rootID <= 0 {
			continue
		}
		rootEv, hasRoot := eventByID[rootID]
		// Only cluster if either this event or its root is narratively meaningful.
		if !majorChronicleKinds[e.Kind] && (!hasRoot || !majorChronicleKinds[rootEv.Kind]) {
			continue
		}
		clk := sim.ClockAt(e.Tick)
		entry := ChronicleTimelineEntry{
			Tick:     e.Tick,
			Day:      int64(clk.Day),
			Clock:    fmt.Sprintf("%02d:%02d", clk.Hour, clk.Minute),
			EventID:  e.ID,
			Kind:     e.Kind,
			ActorID:  e.ActorID,
			TargetID: e.TargetID,
			Text:     eventPayloadText(e),
		}
		arc, ok := byRoot[rootID]
		if !ok {
			rKind := e.Kind
			rSummary := entry.Text
			rTick := e.Tick
			rDay := int64(clk.Day)
			if hasRoot {
				rKind = rootEv.Kind
				rSummary = eventPayloadText(rootEv)
				rTick = rootEv.Tick
				rDay = int64(sim.ClockAt(rootEv.Tick).Day)
			}
			arc = &ChronicleArc{
				RootEventID: rootID,
				RootKind:    rKind,
				StartDay:    rDay,
				EndDay:      int64(clk.Day),
				StartTick:   rTick,
				EndTick:     e.Tick,
				Summary:     rSummary,
			}
			byRoot[rootID] = arc
			partSet[rootID] = map[string]bool{}
		}
		if e.Tick < arc.StartTick {
			arc.StartTick = e.Tick
			arc.StartDay = int64(clk.Day)
		}
		if e.Tick > arc.EndTick {
			arc.EndTick = e.Tick
			arc.EndDay = int64(clk.Day)
		}
		arc.EventCount++
		arc.Salience += arcEventSalience(e.Kind)
		if e.ActorID != "" && e.ActorID != "world" {
			partSet[rootID][e.ActorID] = true
		}
		if e.TargetID != "" && e.TargetID != "world" {
			partSet[rootID][e.TargetID] = true
		}
		arc.KeyEvents = append(arc.KeyEvents, entry)
	}

	arcs := make([]ChronicleArc, 0, len(byRoot))
	for rootID, arc := range byRoot {
		ps := make([]string, 0, len(partSet[rootID]))
		for pid := range partSet[rootID] {
			ps = append(ps, pid)
		}
		sort.Strings(ps)
		arc.Participants = ps
		arcs = append(arcs, *arc)
	}
	sort.Slice(arcs, func(i, j int) bool {
		if arcs[i].StartTick != arcs[j].StartTick {
			return arcs[i].StartTick < arcs[j].StartTick
		}
		if arcs[i].Salience != arcs[j].Salience {
			return arcs[i].Salience > arcs[j].Salience
		}
		return arcs[i].RootEventID < arcs[j].RootEventID
	})
	stats.ArcCount = len(arcs)
	return arcs, stats
}

// QueryEvents filters simulation events for targeted historical drill-down (actor life,
// bilateral history, causal arc, or day window), filtering out routine physiological noise by default.
func QueryEvents(events []sim.Event, f EventQueryFilter) QueryResult {
	sorted := append([]sim.Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Tick != sorted[j].Tick {
			return sorted[i].Tick < sorted[j].Tick
		}
		return sorted[i].ID < sorted[j].ID
	})

	kindSet := map[string]bool{}
	for _, k := range f.Kinds {
		k = strings.TrimSpace(k)
		if k != "" {
			kindSet[k] = true
		}
	}

	var res QueryResult
	for _, e := range sorted {
		if !f.IncludeRoutine && routineEventKinds[e.Kind] && len(kindSet) == 0 && f.ArcRootID == 0 {
			continue
		}
		if len(kindSet) > 0 && !kindSet[e.Kind] {
			continue
		}
		clk := sim.ClockAt(e.Tick)
		day := int64(clk.Day)
		if f.FromDay > 0 && day < f.FromDay {
			continue
		}
		if f.ToDay > 0 && day > f.ToDay {
			continue
		}
		if f.ArcRootID > 0 {
			if e.ID != f.ArcRootID && eventRootID(e) != f.ArcRootID {
				continue
			}
		}
		if f.ActorID != "" && !eventInvolvesActor(e, f.ActorID) {
			continue
		}
		if f.BetweenA != "" && f.BetweenB != "" {
			if !(eventInvolvesActor(e, f.BetweenA) && eventInvolvesActor(e, f.BetweenB)) {
				continue
			}
		}

		res.TotalMatched++
		if f.Limit > 0 && len(res.Entries) >= f.Limit {
			res.Truncated = true
			continue
		}
		res.Entries = append(res.Entries, ChronicleTimelineEntry{
			Tick:     e.Tick,
			Day:      day,
			Clock:    fmt.Sprintf("%02d:%02d", clk.Hour, clk.Minute),
			EventID:  e.ID,
			Kind:     e.Kind,
			ActorID:  e.ActorID,
			TargetID: e.TargetID,
			Text:     eventPayloadText(e),
		})
	}
	return res
}

func eventInvolvesActor(e sim.Event, actorID string) bool {
	if e.ActorID == actorID || e.TargetID == actorID {
		return true
	}
	if e.Payload == nil {
		return false
	}
	for _, k := range []string{"actor_id", "target_id", "owner_id", "victim_id", "subject_id", "leader_id", "predecessor_id", "successor_id"} {
		if s, ok := e.Payload[k].(string); ok && s == actorID {
			return true
		}
	}
	for _, k := range []string{"delivered_to", "members", "witnesses"} {
		switch raw := e.Payload[k].(type) {
		case []string:
			for _, s := range raw {
				if s == actorID {
					return true
				}
			}
		case []any:
			for _, item := range raw {
				if s, ok := item.(string); ok && s == actorID {
					return true
				}
			}
		}
	}
	return false
}

// RenderQueryMarkdown formats a QueryResult concisely for LLM reading.
func RenderQueryMarkdown(res QueryResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Historical Query Result (%d matched", res.TotalMatched)
	if res.Truncated {
		fmt.Fprintf(&b, ", showing %d", len(res.Entries))
	}
	b.WriteString(")\n\n")
	if len(res.Entries) == 0 {
		b.WriteString("_No matching events._\n")
		return b.String()
	}
	for _, item := range res.Entries {
		target := ""
		if item.TargetID != "" {
			target = " -> " + item.TargetID
		}
		fmt.Fprintf(&b, "- **Day %d %s** (`#%d`, `%s`, `%s%s`): %s\n",
			item.Day, item.Clock, item.EventID, item.Kind, item.ActorID, target, item.Text)
	}
	return b.String()
}

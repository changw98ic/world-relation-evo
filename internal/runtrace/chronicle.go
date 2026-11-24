package runtrace

import (
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// ChronicleTimelineEntry captures a major chronological milestone event in a simulated world.
type ChronicleTimelineEntry struct {
	Tick     int64  `json:"tick"`
	Day      int64  `json:"day"`
	Clock    string `json:"clock"`
	EventID  int64  `json:"event_id"`
	Kind     string `json:"kind"`
	ActorID  string `json:"actor_id"`
	TargetID string `json:"target_id,omitempty"`
	Text     string `json:"text"`
}

// PropagationHop records one step in an epistemic news/rumor/death-news causal chain.
type PropagationHop struct {
	CauseEventID int64  `json:"cause_event_id,omitempty"`
	Tick         int64  `json:"tick"`
	EventID      int64  `json:"event_id"`
	Kind         string `json:"kind"`
	ActorID      string `json:"actor_id"`
	TargetID     string `json:"target_id,omitempty"`
	Topic        string `json:"topic,omitempty"`
	Text         string `json:"text"`
}

// PropagationChain links a root event to downstream speech/channel relays and resulting reactions.
type PropagationChain struct {
	RootEventID int64            `json:"root_event_id"`
	RootTick    int64            `json:"root_tick"`
	RootKind    string           `json:"root_kind"`
	RootText    string           `json:"root_text"`
	Hops        []PropagationHop `json:"hops"`
}

// ActorBiography summarizes one focal actor's life, status, relationships, and milestones.
type ActorBiography struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	FormID          string   `json:"form_id,omitempty"`
	PowerTier       int      `json:"power_tier,omitempty"`
	Alive           bool     `json:"alive"`
	DeathCause      string   `json:"death_cause,omitempty"`
	DeathTick       int64    `json:"death_tick,omitempty"`
	Guilds          []string `json:"guilds,omitempty"`
	TopSkills       []string `json:"top_skills,omitempty"`
	ActiveAmbitions []string `json:"active_ambitions,omitempty"`
	MentorshipCount int      `json:"mentorship_count,omitempty"`
	ActiveGrudges   int      `json:"active_grudges,omitempty"`
	MilestoneCount  int      `json:"milestone_count"`
}

// FactionStanding summarizes one faction's leadership, membership, and territory.
type FactionStanding struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	LeaderID            string   `json:"leader_id,omitempty"`
	Members             int      `json:"members"`
	Level               int      `json:"level"`
	ControlledDistricts []string `json:"controlled_districts,omitempty"`
}

// CohortStanding summarizes one macro population cohort.
type CohortStanding struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	District   string `json:"district"`
	Profession string `json:"profession"`
	FormID     string `json:"form_id,omitempty"`
	Population int    `json:"population"`
	MoneyPool  int64  `json:"money_pool"`
}

// ChronicleReport is a complete, genre-agnostic narrative & analytical report of a simulation run.
type ChronicleReport struct {
	WorldID           string                   `json:"world_id"`
	Genre             string                   `json:"genre,omitempty"`
	Theme             string                   `json:"theme,omitempty"`
	HasFinalState     bool                     `json:"has_final_state"`
	TotalTicks        int64                    `json:"total_ticks"`
	TotalDays         int64                    `json:"total_days"`
	TotalEvents       int                      `json:"total_events"`
	TotalMajorEvents  int                      `json:"total_major_events"`
	TimelineTruncated bool                     `json:"timeline_truncated,omitempty"`
	LivingActors      int                      `json:"living_actors"`
	DeadActors        int                      `json:"dead_actors"`
	CohortPopulation  int                      `json:"cohort_population"`
	TotalCoins        int64                    `json:"total_coins"`
	LedgerConserved   bool                     `json:"ledger_conserved"`
	LedgerError       string                   `json:"ledger_error,omitempty"`
	ActiveParties     int                      `json:"active_parties,omitempty"`
	Drama             DramaStats               `json:"drama"`
	EventKindCounts   map[string]int           `json:"event_kind_counts"`
	Arcs              []ChronicleArc           `json:"arcs,omitempty"`
	Timeline          []ChronicleTimelineEntry `json:"timeline"`
	PropagationChains []PropagationChain       `json:"propagation_chains,omitempty"`
	Biographies       []ActorBiography         `json:"biographies,omitempty"`
	Factions          []FactionStanding        `json:"factions,omitempty"`
	Diplomacy         []sim.FactionDiplomacy   `json:"diplomacy,omitempty"`
	Cohorts           []CohortStanding         `json:"cohorts,omitempty"`
}

var majorChronicleKinds = map[string]bool{
	"world_event_triggered":     true,
	"world_event_claimed":       true,
	"disaster_struck":           true,
	"threat_surge":              true,
	"actor_died":                true,
	"actor_entered_world":       true,
	"actor_succeeded":           true,
	"actor_married":             true,
	"bond_sworn":                true,
	"revolt_erupted":            true,
	"faction_leader_changed":    true,
	"vendetta_clash":            true,
	"grudge_formed":             true,
	"feud_avenged":              true,
	"feud_mediated":             true,
	"transition_succeeded":      true,
	"skill_leveled":             true,
	"effect_spawned":            true,
	"cohort_struck":             true,
	"gf_triggered":              true,
	"gf_exchanged":              true,
	"gf_quest_completed":        true,
	"gf_quest_failed":           true,
	"gf_rescue":                 true,
	"gf_simulated":              true,
	"gf_pocket_yield":           true,
	"party_formed":              true,
	"party_betrayed":            true,
	"party_loot_split":          true,
	"party_disbanded":           true,
	"alliance_formed":           true,
	"alliance_broken":           true,
	"faction_war_declared":      true,
	"faction_war_skirmish":      true,
	"peace_treaty_signed":       true,
	"secret_discovered":         true,
	"blackmailed":               true,
	"blackmail_refused":         true,
	"secret_exposed":            true,
	"ambition_completed":        true,
	"rumor_panic_triggered":     true,
	"corruption_purged":         true,
	"wrongful_conviction":       true,
	"conviction_overturned":     true,
	"attacked":                  true,
	"raided":                    true,
	"arrested":                  true,
	"court_warrant_issued":      true,
	"court_trial_held":          true,
	"cohort_corruption_purged":  true,
	"district_control_shifted":  true,
	"cohort_turf_war":           true,
	"faction_battle":            true,
	"secret_caught_red_handed":  true,
	"geo_anomaly_struck":        true,
	"flow_cutoff":               true,
	"flow_restored":             true,
	"world_law_rewritten":       true,
	"actor_environment_injured": true,
	"inheritance_dispute":       true,
	"inheritance_settled":       true,
	"released_from_jail":        true,
	"threat_hunted":             true,
}

func chronicleTargetIsPerson(kind string) bool {
	switch kind {
	case "attacked", "raided", "arrested", "court_warrant_issued", "court_trial_held", "actor_died", "actor_succeeded",
		"faction_leader_changed", "actor_married", "bond_sworn", "grudge_formed", "feud_avenged", "feud_mediated",
		"party_betrayed", "secret_discovered", "secret_caught_red_handed", "secret_exposed", "blackmailed",
		"blackmail_refused", "corruption_purged", "wrongful_conviction", "conviction_overturned",
		"inheritance_settled", "inheritance_dispute":
		return true
	}
	return false
}

func eventPayloadText(e sim.Event) string {
	if e.Payload != nil {
		if s, ok := e.Payload["text"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	if strings.TrimSpace(e.Action.Text) != "" {
		return strings.TrimSpace(e.Action.Text)
	}
	return fmt.Sprintf("%s (%s -> %s)", e.Kind, e.ActorID, e.TargetID)
}

func eventLink(e sim.Event, key string) int64 {
	switch value := e.Payload[key].(type) {
	case int64:
		return value
	case int:
		return int64(value)
	}
	return 0
}

func eventRootID(e sim.Event) int64 {
	if root := eventLink(e, "root_event_id"); root > 0 {
		return root
	}
	if cause := eventLink(e, "cause_event_id"); cause > 0 {
		return cause
	}
	if e.Action.BasisEventID > 0 {
		return e.Action.BasisEventID
	}
	return e.ID
}

// BuildChronicle compiles a structured, genre-agnostic chronicle report from simulation events and optional final World state.
// Internally projects finalWorld into an immutable WorldMetadataSnapshot so chronicle construction never touches mutable world state.
// Passing maxTimeline <= 0 preserves all major chronological events without truncation.
func BuildChronicle(events []sim.Event, finalWorld *sim.World, maxTimeline int) ChronicleReport {
	return buildChronicleFromSnapshot(events, ProjectWorldMetadata(finalWorld), maxTimeline)
}

func buildChronicleFromSnapshot(events []sim.Event, snap *WorldMetadataSnapshot, maxTimeline int) ChronicleReport {
	sortedEvents := append([]sim.Event(nil), events...)
	sort.Slice(sortedEvents, func(i, j int) bool {
		if sortedEvents[i].Tick != sortedEvents[j].Tick {
			return sortedEvents[i].Tick < sortedEvents[j].Tick
		}
		return sortedEvents[i].ID < sortedEvents[j].ID
	})

	rep := ChronicleReport{
		HasFinalState:   snap != nil,
		TotalEvents:     len(sortedEvents),
		EventKindCounts: map[string]int{},
	}

	milestoneByActor := map[string]int{}
	eventByID := make(map[int64]sim.Event, len(sortedEvents))
	chainsByRoot := map[int64]*PropagationChain{}

	for _, e := range sortedEvents {
		eventByID[e.ID] = e
		rep.EventKindCounts[e.Kind]++
		if e.Tick > rep.TotalTicks {
			rep.TotalTicks = e.Tick
		}
		if majorChronicleKinds[e.Kind] {
			rep.TotalMajorEvents++
			if e.ActorID != "" && e.ActorID != "world" {
				milestoneByActor[e.ActorID]++
			}
			if e.TargetID != "" && e.TargetID != e.ActorID && chronicleTargetIsPerson(e.Kind) {
				milestoneByActor[e.TargetID]++
			}
			if maxTimeline <= 0 || len(rep.Timeline) < maxTimeline {
				clk := sim.ClockAt(e.Tick)
				rep.Timeline = append(rep.Timeline, ChronicleTimelineEntry{
					Tick:     e.Tick,
					Day:      int64(clk.Day),
					Clock:    fmt.Sprintf("%02d:%02d", clk.Hour, clk.Minute),
					EventID:  e.ID,
					Kind:     e.Kind,
					ActorID:  e.ActorID,
					TargetID: e.TargetID,
					Text:     eventPayloadText(e),
				})
			} else {
				rep.TimelineTruncated = true
			}
		}

	}

	// Link downstream relays and reactions to their root events.
	for _, e := range sortedEvents {
		rootID := eventRootID(e)
		if rootID == e.ID {
			continue
		}
		for root, exists := eventByID[rootID]; exists; root, exists = eventByID[rootID] {
			prior := eventRootID(root)
			if prior <= 0 || prior >= rootID {
				break
			}
			rootID = prior
		}
		chain, ok := chainsByRoot[rootID]
		if !ok {
			if rootEv, exists := eventByID[rootID]; exists {
				chain = &PropagationChain{
					RootEventID: rootEv.ID,
					RootTick:    rootEv.Tick,
					RootKind:    rootEv.Kind,
					RootText:    eventPayloadText(rootEv),
				}
				chainsByRoot[rootID] = chain
			} else {
				continue
			}
		}
		topic := ""
		if e.Action.Statement != nil {
			topic = e.Action.Statement.Topic
		} else if e.Payload != nil {
			if rawStmt, ok := e.Payload["statement"].(sim.Statement); ok {
				topic = rawStmt.Topic
			} else if rawMap, ok := e.Payload["statement"].(map[string]any); ok {
				if s, ok := rawMap["topic"].(string); ok {
					topic = s
				}
			}
		}
		targetID := e.TargetID
		if e.Kind == "speech" || e.Kind == "channel_transmitted" || e.Kind == "channel_message" {
			switch raw := e.Payload["delivered_to"].(type) {
			case []string:
				if len(raw) > 0 {
					targetID = strings.Join(raw, ",")
				}
			case []any:
				var ids []string
				for _, item := range raw {
					if s, ok := item.(string); ok && s != "" {
						ids = append(ids, s)
					}
				}
				if len(ids) > 0 {
					targetID = strings.Join(ids, ",")
				}
			}
		}
		chain.Hops = append(chain.Hops, PropagationHop{
			Tick:         e.Tick,
			EventID:      e.ID,
			CauseEventID: eventLink(e, "cause_event_id"),
			Kind:         e.Kind,
			ActorID:      e.ActorID,
			TargetID:     targetID,
			Topic:        topic,
			Text:         eventPayloadText(e),
		})
	}

	var chainIDs []int64
	for id, ch := range chainsByRoot {
		if len(ch.Hops) > 0 {
			chainIDs = append(chainIDs, id)
		}
	}
	sort.Slice(chainIDs, func(i, j int) bool { return chainIDs[i] < chainIDs[j] })
	for _, id := range chainIDs {
		rep.PropagationChains = append(rep.PropagationChains, *chainsByRoot[id])
	}
	rep.Arcs, rep.Drama = BuildChronicleArcs(sortedEvents)

	if snap != nil {
		rep.WorldID = snap.WorldID
		if snap.Tick > rep.TotalTicks {
			rep.TotalTicks = snap.Tick
		}
		rep.Genre = snap.Genre
		rep.Theme = snap.Theme
		rep.LivingActors = snap.LivingActors
		rep.DeadActors = snap.DeadActors
		rep.CohortPopulation = snap.CohortPopulation
		rep.TotalCoins = snap.TotalCoins
		rep.LedgerConserved = snap.LedgerConserved
		rep.LedgerError = snap.LedgerError
		rep.ActiveParties = snap.ActiveParties
		for _, bio := range snap.Actors {
			b := bio
			b.Guilds = append([]string(nil), bio.Guilds...)
			b.MilestoneCount = milestoneByActor[b.ID]
			rep.Biographies = append(rep.Biographies, b)
		}
		rep.Factions = append([]FactionStanding(nil), snap.Factions...)
		for i := range rep.Factions {
			rep.Factions[i].ControlledDistricts = append([]string(nil), rep.Factions[i].ControlledDistricts...)
		}
		rep.Diplomacy = append([]sim.FactionDiplomacy(nil), snap.Diplomacy...)
		rep.Cohorts = append([]CohortStanding(nil), snap.Cohorts...)
	}

	rep.TotalDays = rep.TotalTicks / sim.TicksPerDay
	if rep.TotalTicks%sim.TicksPerDay > 0 {
		rep.TotalDays++
	}
	return rep
}

// RenderChronicleMarkdown formats a ChronicleReport into a human-readable Markdown document.
func RenderChronicleMarkdown(rep ChronicleReport) string {
	var b strings.Builder
	title := rep.WorldID
	if title == "" {
		title = "Simulated World"
	}
	fmt.Fprintf(&b, "# World Chronicle: %s\n\n", title)
	if rep.Theme != "" || rep.Genre != "" {
		fmt.Fprintf(&b, "- **Theme / Genre**: %s (%s)\n", rep.Theme, rep.Genre)
	}
	fmt.Fprintf(&b, "- **Duration**: %d ticks (%d simulated days), %d total events\n", rep.TotalTicks, rep.TotalDays, rep.TotalEvents)
	fmt.Fprintf(&b, "- **Population**: %d living focal actors, %d deceased, %d cohort population\n", rep.LivingActors, rep.DeadActors, rep.CohortPopulation)
	conservedLabel := "UNKNOWN (no final state)"
	if rep.HasFinalState {
		if rep.LedgerConserved {
			conservedLabel = "CONSERVED"
		} else if rep.LedgerError != "" {
			conservedLabel = "VIOLATION: " + rep.LedgerError
		} else {
			conservedLabel = "UNVERIFIED"
		}
	}
	fmt.Fprintf(&b, "- **Economy & Ledger**: %d total coins | Status: **%s** | Active Parties: %d\n", rep.TotalCoins, conservedLabel, rep.ActiveParties)
	fmt.Fprintf(&b, "- **Drama Density**: %d arcs | deaths=%d, wars/clashes=%d, alliances/peace=%d, secrets/plots=%d, betrayals=%d, world_events=%d, ambitions_done=%d, transitions=%d\n\n",
		rep.Drama.ArcCount, rep.Drama.Deaths, rep.Drama.WarsAndClashes, rep.Drama.AlliancesAndPeace,
		rep.Drama.SecretsAndPlots, rep.Drama.Betrayals, rep.Drama.WorldEvents, rep.Drama.AmbitionsCompleted, rep.Drama.Transitions)

	if len(rep.Arcs) > 0 {
		b.WriteString("## Narrative Arcs (Clustered by RootEventID)\n\n")
		shownArcs := rep.Arcs
		if len(shownArcs) > 40 {
			ranked := make([]ChronicleArc, 0, len(rep.Arcs))
			for _, a := range rep.Arcs {
				if a.EventCount >= 2 || a.Salience >= 25 {
					ranked = append(ranked, a)
				}
			}
			if len(ranked) == 0 {
				ranked = append(ranked, rep.Arcs...)
			}
			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].Salience != ranked[j].Salience {
					return ranked[i].Salience > ranked[j].Salience
				}
				return ranked[i].StartTick < ranked[j].StartTick
			})
			if len(ranked) > 40 {
				ranked = ranked[:40]
			}
			sort.Slice(ranked, func(i, j int) bool {
				if ranked[i].StartTick != ranked[j].StartTick {
					return ranked[i].StartTick < ranked[j].StartTick
				}
				return ranked[i].RootEventID < ranked[j].RootEventID
			})
			shownArcs = ranked
			fmt.Fprintf(&b, "_Showing top %d high-salience arcs out of %d total (full list available in `-format json` or via `wre query`)._\n\n", len(shownArcs), len(rep.Arcs))
		}
		for _, arc := range shownArcs {
			parts := strings.Join(arc.Participants, ", ")
			if parts == "" {
				parts = "-"
			}
			fmt.Fprintf(&b, "- **Arc #%d** (`%s`, Day %d–%d, salience=%d, %d events, cast: %s): %s\n",
				arc.RootEventID, arc.RootKind, arc.StartDay, arc.EndDay, arc.Salience, arc.EventCount, parts, arc.Summary)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Major Chronicle Timeline\n\n")
	if len(rep.Timeline) == 0 {
		b.WriteString("_No major chronicle events recorded._\n\n")
	} else {
		if rep.TimelineTruncated {
			fmt.Fprintf(&b, "_Showing %d of %d major events (truncated by max-events limit)._\n\n", len(rep.Timeline), rep.TotalMajorEvents)
		}
		for _, item := range rep.Timeline {
			fmt.Fprintf(&b, "- **Day %d %s** (`tick %d`, `%s`): %s\n", item.Day, item.Clock, item.Tick, item.Kind, item.Text)
		}
		b.WriteString("\n")
	}

	if len(rep.PropagationChains) > 0 {
		b.WriteString("## Epistemic Propagation Chains\n\n")
		for _, ch := range rep.PropagationChains {
			fmt.Fprintf(&b, "### Root #%d (`%s` @ tick %d): %s\n", ch.RootEventID, ch.RootKind, ch.RootTick, ch.RootText)
			for _, hop := range ch.Hops {
				topicSuffix := ""
				if hop.Topic != "" {
					topicSuffix = " [topic=" + hop.Topic + "]"
				}
				fmt.Fprintf(&b, "  - tick %d `%s`%s (`%s` -> `%s`): %s\n", hop.Tick, hop.Kind, topicSuffix, hop.ActorID, hop.TargetID, hop.Text)
			}
			b.WriteString("\n")
		}
	}

	if len(rep.Factions) > 0 {
		b.WriteString("## Factions & Diplomacy\n\n")
		b.WriteString("| Faction | Leader | Members | Level | Controlled Districts |\n")
		b.WriteString("| :--- | :--- | :---: | :---: | :--- |\n")
		for _, f := range rep.Factions {
			dists := strings.Join(f.ControlledDistricts, ", ")
			if dists == "" {
				dists = "-"
			}
			leader := f.LeaderID
			if leader == "" {
				leader = "-"
			}
			fmt.Fprintf(&b, "| %s (`%s`) | %s | %d | %d | %s |\n", f.Name, f.ID, leader, f.Members, f.Level, dists)
		}
		b.WriteString("\n")
	}
	if len(rep.Diplomacy) > 0 {
		b.WriteString("### Bilateral Diplomacy\n\n")
		for _, d := range rep.Diplomacy {
			fmt.Fprintf(&b, "- `%s` <-> `%s`: **%s** (score=%d, war=%d:%d, reason=%s)\n", d.FactionA, d.FactionB, d.Stance, d.RelationScore, d.WarScoreA, d.WarScoreB, d.Reason)
		}
		b.WriteString("\n")
	}

	if len(rep.Biographies) > 0 {
		b.WriteString("## Actor Biographies & Fate Cards\n\n")
		b.WriteString("| Actor | Role | Form (Tier) | Status | Mentorships / Grudges | Ambitions |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :---: | :--- |\n")
		for _, bio := range rep.Biographies {
			status := "Alive"
			if !bio.Alive {
				status = fmt.Sprintf("Dead (%s @ tick %d)", bio.DeathCause, bio.DeathTick)
			}
			formStr := bio.FormID
			if formStr == "" {
				formStr = "default"
			}
			ambStr := strings.Join(bio.ActiveAmbitions, ", ")
			if ambStr == "" {
				ambStr = "-"
			}
			fmt.Fprintf(&b, "| %s (`%s`) | %s | %s (T%d) | %s | %d / %d | %s |\n",
				bio.Name, bio.ID, bio.Role, formStr, bio.PowerTier, status, bio.MentorshipCount, bio.ActiveGrudges, ambStr)
		}
		b.WriteString("\n")
	}

	if len(rep.Cohorts) > 0 {
		b.WriteString("## Population Cohorts\n\n")
		b.WriteString("| Cohort | District | Profession | Form | Population | Money Pool |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :---: | :---: |\n")
		for _, c := range rep.Cohorts {
			f := c.FormID
			if f == "" {
				f = "default"
			}
			fmt.Fprintf(&b, "| %s (`%s`) | %s | %s | %s | %d | %d |\n", c.Name, c.ID, c.District, c.Profession, f, c.Population, c.MoneyPool)
		}
		b.WriteString("\n")
	}
	return b.String()
}

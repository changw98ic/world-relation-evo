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
	Tick     int64  `json:"tick"`
	EventID  int64  `json:"event_id"`
	Kind     string `json:"kind"`
	ActorID  string `json:"actor_id"`
	TargetID string `json:"target_id,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Text     string `json:"text"`
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
	Coins           int      `json:"coins"`
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
	TotalTicks        int64                    `json:"total_ticks"`
	TotalDays         int64                    `json:"total_days"`
	TotalEvents       int                      `json:"total_events"`
	LivingActors      int                      `json:"living_actors"`
	DeadActors        int                      `json:"dead_actors"`
	CohortPopulation  int                      `json:"cohort_population"`
	TotalCoins        int64                    `json:"total_coins"`
	LedgerConserved   bool                     `json:"ledger_conserved"`
	LedgerError       string                   `json:"ledger_error,omitempty"`
	ActiveParties     int                      `json:"active_parties,omitempty"`
	EventKindCounts   map[string]int           `json:"event_kind_counts"`
	Timeline          []ChronicleTimelineEntry `json:"timeline"`
	PropagationChains []PropagationChain       `json:"propagation_chains,omitempty"`
	Biographies       []ActorBiography         `json:"biographies,omitempty"`
	Factions          []FactionStanding        `json:"factions,omitempty"`
	Diplomacy         []sim.FactionDiplomacy   `json:"diplomacy,omitempty"`
	Cohorts           []CohortStanding         `json:"cohorts,omitempty"`
}

var majorChronicleKinds = map[string]bool{
	"world_event_triggered": true,
	"world_event_claimed":   true,
	"disaster_struck":       true,
	"threat_surge":          true,
	"actor_died":            true,
	"revolt_erupted":        true,
	"faction_leader_changed": true,
	"turf_war":              true,
	"vendetta_clash":        true,
	"grudge_formed":         true,
	"transition_succeeded":  true,
	"party_formed":          true,
	"party_betrayed":        true,
	"party_loot_split":      true,
	"alliance_formed":       true,
	"faction_war_declared":  true,
	"faction_war_skirmish":  true,
	"peace_treaty_signed":   true,
	"secret_discovered":     true,
	"blackmailed":           true,
	"secret_exposed":        true,
	"ambition_completed":    true,
	"rumor_panic_triggered": true,
}

var chainRootKinds = map[string]bool{
	"actor_died":            true,
	"world_event_triggered": true,
	"wrongful_conviction":   true,
	"secret_exposed":        true,
	"party_betrayed":        true,
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

func eventRootID(e sim.Event) int64 {
	if e.Payload != nil {
		switch v := e.Payload["root_event_id"].(type) {
		case int64:
			if v > 0 {
				return v
			}
		case float64:
			if v > 0 {
				return int64(v)
			}
		case int:
			if v > 0 {
				return int64(v)
			}
		}
	}
	if e.Action.BasisEventID > 0 {
		return e.Action.BasisEventID
	}
	return e.ID
}

// BuildChronicle compiles a structured, genre-agnostic chronicle report from simulation events and optional final World state.
func BuildChronicle(events []sim.Event, finalWorld *sim.World, maxTimeline int) ChronicleReport {
	if maxTimeline <= 0 {
		maxTimeline = 120
	}
	sortedEvents := append([]sim.Event(nil), events...)
	sort.Slice(sortedEvents, func(i, j int) bool {
		if sortedEvents[i].Tick != sortedEvents[j].Tick {
			return sortedEvents[i].Tick < sortedEvents[j].Tick
		}
		return sortedEvents[i].ID < sortedEvents[j].ID
	})

	rep := ChronicleReport{
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
			if e.ActorID != "" && e.ActorID != "world" {
				milestoneByActor[e.ActorID]++
			}
			if e.TargetID != "" {
				milestoneByActor[e.TargetID]++
			}
			if len(rep.Timeline) < maxTimeline {
				clk := sim.ClockAt(e.Tick)
				rep.Timeline = append(rep.Timeline, ChronicleTimelineEntry{
					Tick:     e.Tick,
					Day:      e.Tick/sim.TicksPerDay + 1,
					Clock:    fmt.Sprintf("%02d:%02d", clk.Hour, clk.Minute),
					EventID:  e.ID,
					Kind:     e.Kind,
					ActorID:  e.ActorID,
					TargetID: e.TargetID,
					Text:     eventPayloadText(e),
				})
			}
		}
		if chainRootKinds[e.Kind] && e.ID > 0 {
			chainsByRoot[e.ID] = &PropagationChain{
				RootEventID: e.ID,
				RootTick:    e.Tick,
				RootKind:    e.Kind,
				RootText:    eventPayloadText(e),
			}
		}
	}

	// Link downstream relays and reactions to their root events.
	for _, e := range sortedEvents {
		isHop := e.Kind == "speech" || e.Kind == "channel_message" ||
			e.Kind == "grudge_formed" || e.Kind == "rumor_panic_triggered" ||
			e.Kind == "world_event_claimed" || e.Kind == "faction_war_declared"
		if !isHop {
			continue
		}
		rootID := eventRootID(e)
		if rootID == e.ID {
			continue
		}
		chain, ok := chainsByRoot[rootID]
		if !ok {
			if rootEv, exists := eventByID[rootID]; exists && chainRootKinds[rootEv.Kind] {
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
		}
		chain.Hops = append(chain.Hops, PropagationHop{
			Tick:     e.Tick,
			EventID:  e.ID,
			Kind:     e.Kind,
			ActorID:  e.ActorID,
			TargetID: e.TargetID,
			Topic:    topic,
			Text:     eventPayloadText(e),
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

	if finalWorld != nil {
		rep.WorldID = finalWorld.ID
		if finalWorld.Tick > rep.TotalTicks {
			rep.TotalTicks = finalWorld.Tick
		}
		if finalWorld.Catalog.Environment != nil {
			rep.Genre = finalWorld.Catalog.Environment.Genre
			rep.Theme = finalWorld.Catalog.Environment.Theme
		}
		cons := sim.InspectWorldConservation(finalWorld)
		rep.LivingActors = cons.LivingActors
		rep.DeadActors = cons.DeadActors
		rep.CohortPopulation = cons.CohortPopulation
		rep.TotalCoins = cons.TotalCoins
		rep.LedgerConserved = cons.LedgerConserved
		rep.LedgerError = cons.LedgerError
		rep.ActiveParties = len(finalWorld.Parties)

		// Mentorship and grudge counts per actor.
		mentorCount := map[string]int{}
		grudgeCount := map[string]int{}
		if finalWorld.Relations != nil {
			for _, m := range finalWorld.Relations.Mentorships {
				mentorCount[m.MentorID]++
				mentorCount[m.ApprenticeID]++
			}
			for _, g := range finalWorld.Relations.Grudges {
				if !g.Resolved {
					grudgeCount[g.AggrievedID]++
				}
			}
		}

		actorsSorted := append([]sim.Actor(nil), finalWorld.Actors...)
		sort.Slice(actorsSorted, func(i, j int) bool { return actorsSorted[i].ID < actorsSorted[j].ID })
		for _, a := range actorsSorted {
			var skillList []string
			skKeys := make([]string, 0, len(a.Skills))
			for k, lv := range a.Skills {
				if lv > 0 {
					skKeys = append(skKeys, k)
				}
			}
			sort.Strings(skKeys)
			for _, k := range skKeys {
				skillList = append(skillList, fmt.Sprintf("%s:lv%d", k, a.Skills[k]))
			}
			var ambList []string
			for _, amb := range a.Ambitions {
				status := "active"
				if amb.Completed {
					status = "done"
				}
				ambList = append(ambList, fmt.Sprintf("%s(%s)", amb.Kind, status))
			}
			rep.Biographies = append(rep.Biographies, ActorBiography{
				ID:              a.ID,
				Name:            a.Name,
				Role:            a.Role,
				FormID:          a.FormID,
				PowerTier:       finalWorld.Catalog.ActorPowerTier(a),
				Alive:           !a.Dead,
				DeathCause:      a.DeathCause,
				DeathTick:       a.DeathTick,
				Coins:           a.Money,
				Guilds:          append([]string(nil), a.Guilds...),
				TopSkills:       skillList,
				ActiveAmbitions: ambList,
				MentorshipCount: mentorCount[a.ID],
				ActiveGrudges:   grudgeCount[a.ID],
				MilestoneCount:  milestoneByActor[a.ID],
			})
		}

		// Factions
		distByFac := map[string][]string{}
		if finalWorld.Relations != nil {
			for dID, fID := range finalWorld.Relations.ControllingFaction {
				if fID != "" {
					distByFac[fID] = append(distByFac[fID], dID)
				}
			}
			for fID := range distByFac {
				sort.Strings(distByFac[fID])
			}
		}
		facs := append([]sim.Faction(nil), finalWorld.Factions...)
		sort.Slice(facs, func(i, j int) bool { return facs[i].ID < facs[j].ID })
		for _, f := range facs {
			leader := ""
			if finalWorld.Relations != nil {
				leader = finalWorld.Relations.FactionLeader[f.ID]
			}
			rep.Factions = append(rep.Factions, FactionStanding{
				ID:                  f.ID,
				Name:                f.Name,
				LeaderID:            leader,
				Members:             len(f.Members),
				Level:               f.Level,
				ControlledDistricts: distByFac[f.ID],
			})
		}

		// Diplomacy
		if finalWorld.Diplomacy != nil && len(finalWorld.Diplomacy.Relations) > 0 {
			dKeys := make([]string, 0, len(finalWorld.Diplomacy.Relations))
			for k := range finalWorld.Diplomacy.Relations {
				dKeys = append(dKeys, k)
			}
			sort.Strings(dKeys)
			for _, k := range dKeys {
				if rel := finalWorld.Diplomacy.Relations[k]; rel != nil {
					rep.Diplomacy = append(rep.Diplomacy, *rel)
				}
			}
		}

		// Cohorts
		cohorts := append([]sim.PopulationCohort(nil), finalWorld.Cohorts...)
		sort.Slice(cohorts, func(i, j int) bool { return cohorts[i].ID < cohorts[j].ID })
		for _, c := range cohorts {
			rep.Cohorts = append(rep.Cohorts, CohortStanding{
				ID:         c.ID,
				Name:       c.Name,
				District:   c.District,
				Profession: c.Profession,
				FormID:     c.FormID,
				Population: c.Population,
				MoneyPool:  c.MoneyPool,
			})
		}
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
	conservedLabel := "CONSERVED"
	if !rep.LedgerConserved && rep.LedgerError != "" {
		conservedLabel = "VIOLATION: " + rep.LedgerError
	}
	fmt.Fprintf(&b, "- **Economy & Ledger**: %d total coins | Status: **%s** | Active Parties: %d\n\n", rep.TotalCoins, conservedLabel, rep.ActiveParties)

	b.WriteString("## Major Chronicle Timeline\n\n")
	if len(rep.Timeline) == 0 {
		b.WriteString("_No major chronicle events recorded._\n\n")
	} else {
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
		b.WriteString("| Actor | Role | Form (Tier) | Status | Coins | Mentorships / Grudges | Ambitions |\n")
		b.WriteString("| :--- | :--- | :--- | :--- | :---: | :---: | :--- |\n")
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
			fmt.Fprintf(&b, "| %s (`%s`) | %s | %s (T%d) | %s | %d | %d / %d | %s |\n",
				bio.Name, bio.ID, bio.Role, formStr, bio.PowerTier, status, bio.Coins, bio.MentorshipCount, bio.ActiveGrudges, ambStr)
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

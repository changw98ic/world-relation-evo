package runtrace

import (
	"fmt"
	"sort"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// WorldMetadataSnapshot is an immutable, public-only metadata projection extracted from a World
// so narrative/chronicle generation cannot hold or mutate a live *sim.World pointer.
type WorldMetadataSnapshot struct {
	WorldID          string
	Tick             int64
	Genre            string
	Theme            string
	LivingActors     int
	DeadActors       int
	CohortPopulation int
	TotalCoins       int64
	LedgerConserved  bool
	LedgerError      string
	ActiveParties    int
	Actors           []ActorBiography
	Factions         []FactionStanding
	Diplomacy        []sim.FactionDiplomacy
	Cohorts          []CohortStanding
}

// ProjectWorldMetadata converts a sim.World into an immutable public WorldMetadataSnapshot,
// respecting active disguises (PerceivedFormID / PerceivedPowerTier).
func ProjectWorldMetadata(w *sim.World) *WorldMetadataSnapshot {
	if w == nil {
		return nil
	}
	snap := &WorldMetadataSnapshot{
		WorldID:       w.ID,
		Tick:          w.Tick,
		ActiveParties: len(w.Parties),
	}
	if w.Catalog.Environment != nil {
		snap.Genre = w.Catalog.Environment.Genre
		snap.Theme = w.Catalog.Environment.Theme
	}
	cons := sim.InspectWorldConservation(w)
	snap.LivingActors = cons.LivingActors
	snap.DeadActors = cons.DeadActors
	snap.CohortPopulation = cons.CohortPopulation
	snap.TotalCoins = cons.TotalCoins
	snap.LedgerConserved = cons.LedgerConserved
	snap.LedgerError = cons.LedgerError

	mentorCount := map[string]int{}
	grudgeCount := map[string]int{}
	if w.Relations != nil {
		for _, m := range w.Relations.Mentorships {
			mentorCount[m.MentorID]++
			mentorCount[m.ApprenticeID]++
		}
		for _, g := range w.Relations.Grudges {
			if !g.Resolved {
				grudgeCount[g.AggrievedID]++
			}
		}
	}

	actorsSorted := append([]sim.Actor(nil), w.Actors...)
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
		snap.Actors = append(snap.Actors, ActorBiography{
			ID:              a.ID,
			Name:            a.Name,
			Role:            a.Role,
			FormID:          w.Catalog.PerceivedFormID(a),
			PowerTier:       w.Catalog.PerceivedPowerTier(a),
			Alive:           !a.Dead,
			DeathCause:      a.DeathCause,
			DeathTick:       a.DeathTick,
			Guilds:          append([]string(nil), a.Guilds...),
			TopSkills:       skillList,
			ActiveAmbitions: ambList,
			MentorshipCount: mentorCount[a.ID],
			ActiveGrudges:   grudgeCount[a.ID],
		})
	}

	distByFac := map[string][]string{}
	if w.Relations != nil {
		for dID, fID := range w.Relations.ControllingFaction {
			if fID != "" {
				distByFac[fID] = append(distByFac[fID], dID)
			}
		}
		for fID := range distByFac {
			sort.Strings(distByFac[fID])
		}
	}
	facs := append([]sim.Faction(nil), w.Factions...)
	sort.Slice(facs, func(i, j int) bool { return facs[i].ID < facs[j].ID })
	for _, f := range facs {
		leader := ""
		if w.Relations != nil {
			leader = w.Relations.FactionLeader[f.ID]
		}
		snap.Factions = append(snap.Factions, FactionStanding{
			ID:                  f.ID,
			Name:                f.Name,
			LeaderID:            leader,
			Members:             len(f.Members),
			Level:               f.Level,
			ControlledDistricts: append([]string(nil), distByFac[f.ID]...),
		})
	}

	if w.Diplomacy != nil && len(w.Diplomacy.Relations) > 0 {
		dKeys := make([]string, 0, len(w.Diplomacy.Relations))
		for k := range w.Diplomacy.Relations {
			dKeys = append(dKeys, k)
		}
		sort.Strings(dKeys)
		for _, k := range dKeys {
			if rel := w.Diplomacy.Relations[k]; rel != nil {
				snap.Diplomacy = append(snap.Diplomacy, *rel)
			}
		}
	}

	cohorts := append([]sim.PopulationCohort(nil), w.Cohorts...)
	sort.Slice(cohorts, func(i, j int) bool { return cohorts[i].ID < cohorts[j].ID })
	for _, c := range cohorts {
		snap.Cohorts = append(snap.Cohorts, CohortStanding{
			ID:         c.ID,
			Name:       c.Name,
			District:   c.District,
			Profession: c.Profession,
			FormID:     c.FormID,
			Population: c.Population,
			MoneyPool:  c.MoneyPool,
		})
	}
	return snap
}

// BuildChronicleReadOnly builds a ChronicleReport purely from event evidence and an
// immutable WorldMetadataSnapshot (zero dependency on mutable *sim.World).
func BuildChronicleReadOnly(events []sim.Event, snap *WorldMetadataSnapshot, maxTimeline int) ChronicleReport {
	return buildChronicleFromSnapshot(events, snap, maxTimeline)
}

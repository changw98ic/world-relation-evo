package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// SetLastTriggerEventID records the causal trigger event ID for an actor's chosen Jev option.
func (s *RulebookSelector) SetLastTriggerEventID(actorID string, eventID int64) {
	if s == nil || actorID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastTriggerByActor == nil {
		s.lastTriggerByActor = map[string]int64{}
	}
	if eventID > 0 {
		s.lastTriggerByActor[actorID] = eventID
	} else {
		delete(s.lastTriggerByActor, actorID)
	}
}

// EnrichOptionsForJev bridges InteractionRulebook into the Jev selection pipeline:
// it evaluates all matching Tier 1 causal reaction branches and Tier 2 proactive rules
// against the legal executable options, annotating matched options with their rulebook
// intent, stance, target, and causal basis event without ever introducing unexecutable actions.
// Safe for concurrent Jev worker goroutines.
func (s *RulebookSelector) EnrichOptionsForJev(p sim.Perception, options []Option) ([]Option, []string) {
	if s == nil || s.Rulebook == nil || len(options) == 0 {
		return options, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observePerceptionLocked(p)

	a := p.Self
	out := make([]Option, len(options))
	copy(out, options)
	idxByID := make(map[string]int, len(out))
	for i, o := range out {
		idxByID[o.ID] = i
	}

	selfProf := profileFromSelf(p)
	nearbyByID := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		nearbyByID[nb.ID] = nb
	}

	var notes []string
	annotated := map[string]bool{}
	annotate := func(optID string, tag string, basisEventID int64) {
		i, ok := idxByID[optID]
		if !ok {
			return
		}
		key := optID + "|" + tag
		if annotated[key] {
			return
		}
		annotated[key] = true
		out[i].Label = out[i].Label + " [" + tag + "]"
		if basisEventID > 0 && out[i].Action.BasisEventID == 0 {
			for _, m := range a.Memories {
				if m.EventID == basisEventID {
					out[i].Action.BasisEventID = basisEventID
					break
				}
			}
		}
		if len(notes) < 8 {
			notes = append(notes, fmt.Sprintf("%s => %s", optID, tag))
		}
	}

	// 1. Tier 1: Causal reaction rules over recent memories (last 48 ticks).
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

		for rIdx := range s.Rulebook.ReactionRules {
			rule := &s.Rulebook.ReactionRules[rIdx]
			if rule.ResponderPosition != pos || !eventKindMatches(rule.TriggerEventKinds, mem.Kind) {
				continue
			}
			if !matchesRole(rule.ResponderMatch, selfProf) {
				continue
			}
			for _, br := range rule.Branches {
				targetID := resolveTargetBinding(p, br.Behavior.TargetBinding, mem.Source, actor2ID, nearbyByID)
				opt := matchGenericBehavior(p, out, br.Behavior, targetID)
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
				tag := fmt.Sprintf("Rulebook reaction %s/%s: %s (cause #%d %s)", rule.ID, br.Stance, title, mem.EventID, mem.Kind)
				annotate(opt.ID, tag, mem.EventID)
			}
		}
	}

	// 2. Tier 2: Proactive role-pair rules matching executable options.
	var targets []actorProfileView
	seenTargets := map[string]bool{}
	for _, nb := range p.Nearby {
		if nb.Jailed || nb.Sleeping || nb.ID == a.ID {
			continue
		}
		seenTargets[nb.ID] = true
		targets = append(targets, profileFromNeighbor(nb))
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	for rIdx := range s.Rulebook.ProactiveRules {
		rule := &s.Rulebook.ProactiveRules[rIdx]
		if !districtMatchesFeatures(p, rule.LocationFeatures) || !matchesRole(rule.Actor1, selfProf) {
			continue
		}
		for _, tProf := range targets {
			if !matchesRole(rule.Actor2, tProf) {
				continue
			}
			if !s.matchesRelationLocked(p, rule.Relation, tProf.ID, true) {
				continue
			}
			targetID := resolveTargetBinding(p, rule.Behavior.TargetBinding, a.ID, tProf.ID, nearbyByID)
			opt := matchGenericBehavior(p, out, rule.Behavior, targetID)
			if opt == nil {
				continue
			}
			tag := fmt.Sprintf("Rulebook proactive %s (%s): %s -> %s", rule.ID, rule.Category, rule.Name, tProf.Name)
			annotate(opt.ID, tag, 0)
		}
	}

	return out, notes
}

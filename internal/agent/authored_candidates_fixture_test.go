package agent

import (
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"sort"
	"strings"
)

// CandidateBuilder constructs the unified legal Candidate list from Perception and RuleSet.
type CandidateBuilder interface {
	Build(view sim.Perception, rules RuleSet) ([]sim.Candidate, error)
}

// DefaultCandidateBuilder generates single-source legal Candidates directly from
// Perception, Catalog (built-in + declarative ActionSpecs), and compiled RuleSet.
// When a compiled Rulebook is supplied, it both enriches matching base candidates
// and synthesizes new legal parameterized candidates directly from matching
// Rulebook BehaviorSpecs (strictly gated by Capabilities and CheckAction).
type DefaultCandidateBuilder struct{}

func (DefaultCandidateBuilder) Build(view sim.Perception, rules RuleSet) ([]sim.Candidate, error) {
	candidates := buildBaseCandidates(view)
	if rules.Rulebook != nil {
		candidates = applyRulebookCandidates(view, rules.Rulebook, candidates)
	}
	return candidates, nil
}

// applyRulebookCandidates performs pure, read-only rule evaluation over Perception
// and the compiled InteractionRulebook. For each matching reactive or proactive rule:
//   - if a base candidate already satisfies the rule's BehaviorSpec, it is enriched
//     with structured RuleIDs, RuleReason, BasisEventID, and PriorityHint;
//   - otherwise, a legal candidate is synthesized directly from the BehaviorSpec
//     and validated against Capabilities() + Perception.CheckAction() before being
//     appended as a first-class Candidate (Source: "rulebook" or "rulebook:action_spec").
func applyRulebookCandidates(p sim.Perception, rb *InteractionRulebook, candidates []sim.Candidate) []sim.Candidate {
	if rb == nil {
		return candidates
	}
	a := p.Self
	idxByID := make(map[string]int, len(candidates))
	for i, c := range candidates {
		idxByID[c.ID] = i
	}
	selfProf := profileFromSelf(p)
	nearbyByID := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		nearbyByID[nb.ID] = nb
	}
	allowed := p.Capabilities()

	attach := func(candID, ruleID, reason string, basisEventID int64, boost float64) {
		i, ok := idxByID[candID]
		if !ok {
			return
		}
		if !sim.ActorHasTag(candidates[i].RuleIDs, ruleID) {
			candidates[i].RuleIDs = append(candidates[i].RuleIDs, ruleID)
		}
		if candidates[i].RuleReason == "" {
			candidates[i].RuleReason = reason
		} else if !strings.Contains(candidates[i].RuleReason, reason) {
			candidates[i].RuleReason += "; " + reason
		}
		if boost > candidates[i].PriorityHint {
			candidates[i].PriorityHint = boost
		}
		if basisEventID > 0 && candidates[i].Action.BasisEventID == 0 {
			for _, m := range a.Memories {
				if m.EventID == basisEventID {
					candidates[i].Action.BasisEventID = basisEventID
					break
				}
			}
		}
	}

	appendOrEnrichSynthesized := func(action sim.Action, label, ruleID, reason string, basisEventID int64, boost float64) string {
		if existingID := candidateIDForAction(action, candidates); existingID != "" {
			attach(existingID, ruleID, reason, basisEventID, boost)
			return existingID
		}
		source := "rulebook"
		if meta, ok := p.Catalog.ActionMetadataByKind(action.Kind); ok && meta.Declarative {
			source = "rulebook:action_spec"
		}
		id := fmt.Sprintf("o%03d", len(candidates))
		cand := sim.Candidate{ID: id, Label: label, Source: source, Action: action}
		candidates = append(candidates, cand)
		idxByID[id] = len(candidates) - 1
		attach(id, ruleID, reason, basisEventID, boost)
		return id
	}

	matcher := NewRulebookSelector(rb, nil)
	matcher.observePerceptionLocked(p)

	// Tier 1: Causal reaction rules over recent memories (last 48 ticks).
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
		actor1 := actorProfileView{ID: mem.Source, Role: mem.SourceRole, Tags: mem.SourceTags}
		neighbor, actor1Here := nearbyByID[mem.Source]
		if actor1Here {
			actor1 = profileFromNeighbor(neighbor)
		}
		actor2 := actorProfileView{ID: actor2ID}
		if actor2ID == a.ID {
			actor2 = selfProf
		} else if neighbor, present := nearbyByID[actor2ID]; present {
			actor2 = profileFromNeighbor(neighbor)
		}
		for rIdx := range rb.ReactionRules {
			rule := &rb.ReactionRules[rIdx]
			if rule.ResponderPosition != pos || !eventKindMatches(rule.TriggerEventKinds, mem.Kind) {
				continue
			}
			if !matchesRole(rule.ResponderMatch, selfProf) || !matchesRole(rule.Actor1Match, actor1) ||
				(actor2ID != "" && !matchesRole(rule.Actor2Match, actor2)) ||
				!matcher.matchesRelationLocked(p, rule.RelationToActor1, mem.Source, actor1Here) {
				continue
			}
			for _, br := range rule.Branches {
				targetID := resolveTargetBinding(p, br.Behavior.TargetBinding, mem.Source, actor2ID, nearbyByID)
				title := strings.TrimSpace(br.Title)
				if title == "" {
					title = rule.Name
				}
				reason := fmt.Sprintf("reaction %s/%s: %s (cause #%d %s)", rule.ID, br.Stance, title, mem.EventID, mem.Kind)
				if cand := matchCandidateForBehavior(p, candidates, br.Behavior, targetID); cand != nil {
					if mem.Kind == "faction_war_declared" && cand.Action.Kind == "attack" &&
						nearbyByID[cand.Action.TargetID].FactionRelation != "at_war" {
						continue
					}
					if !secretReactionAllowed(p, mem, cand.Action) {
						continue
					}
					attach(cand.ID, rule.ID, reason, mem.EventID, float64(max(0, br.Weight)))
					continue
				}
				action, label, ok := synthesizeBehaviorCandidate(p, allowed, br.Behavior, targetID, mem.EventID, mem.Source, targetDisplayName(nearbyByID, targetID))
				if !ok {
					continue
				}
				if mem.Kind == "faction_war_declared" && action.Kind == "attack" &&
					nearbyByID[action.TargetID].FactionRelation != "at_war" {
					continue
				}
				if !secretReactionAllowed(p, mem, action) {
					continue
				}
				appendOrEnrichSynthesized(action, label, rule.ID, reason, mem.EventID, float64(max(0, br.Weight)))
			}
		}
	}

	// Tier 2: Proactive role-pair rules matching or synthesizing executable candidates.
	targets := make([]actorProfileView, 0, len(p.Nearby))
	for _, nb := range p.Nearby {
		if nb.Jailed || nb.Sleeping || nb.ID == a.ID {
			continue
		}
		targets = append(targets, profileFromNeighbor(nb))
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	for rIdx := range rb.ProactiveRules {
		rule := &rb.ProactiveRules[rIdx]
		if !districtMatchesFeatures(p, rule.LocationFeatures) || !matchesRole(rule.Actor1, selfProf) {
			continue
		}
		for _, tProf := range targets {
			if !matchesRole(rule.Actor2, tProf) {
				continue
			}
			if !matcher.matchesRelationLocked(p, rule.Relation, tProf.ID, true) {
				continue
			}
			targetID := resolveTargetBinding(p, rule.Behavior.TargetBinding, a.ID, tProf.ID, nearbyByID)
			reason := fmt.Sprintf("proactive %s (%s): %s -> %s", rule.ID, rule.Category, rule.Name, tProf.Name)
			if cand := matchCandidateForBehavior(p, candidates, rule.Behavior, targetID); cand != nil {
				attach(cand.ID, rule.ID, reason, 0, float64(max(0, rule.Weight)))
				continue
			}
			action, label, ok := synthesizeBehaviorCandidate(p, allowed, rule.Behavior, targetID, 0, a.Name, tProf.Name)
			if !ok {
				continue
			}
			appendOrEnrichSynthesized(action, label, rule.ID, reason, 0, float64(max(0, rule.Weight)))
		}
	}
	return candidates
}

func targetDisplayName(nearbyByID map[string]sim.Neighbor, targetID string) string {
	if nb, ok := nearbyByID[targetID]; ok && nb.Name != "" {
		return nb.Name
	}
	return targetID
}

// matchCandidateForBehavior selects the best existing legal Candidate satisfying a Rulebook BehaviorSpec.
func matchCandidateForBehavior(p sim.Perception, candidates []sim.Candidate, spec BehaviorSpec, targetID string) *sim.Candidate {
	index := matchBehaviorAction(p, spec, targetID, len(candidates), func(i int) sim.Action {
		return candidates[i].Action
	})
	if index < 0 {
		return nil
	}
	return &candidates[index]
}

func matchBehaviorAction(p sim.Perception, spec BehaviorSpec, targetID string, count int, actionAt func(int) sim.Action) int {
	kind := spec.EngineKind
	if (kind == "move" || kind == "move_venue") && spec.LocationFeature != "" {
		dest, _, known := p.Map.NearestDistrictWithFeature(p.Self.Location, spec.LocationFeature)
		if !known || dest == p.Self.Location {
			return -1
		}
		step, reachable := stepTowardDistrict(p, dest, 0)
		if !reachable {
			return -1
		}
		for i := 0; i < count; i++ {
			action := actionAt(i)
			if actionMatchesDesired(action, step) {
				return i
			}
		}
		return -1
	}

	needsTarget := p.Catalog.IsTargetedActionKind(kind) && (spec.TargetBinding != "" || targetID != "")
	best, bestScore := -1, int64(-1)
	for i := 0; i < count; i++ {
		action := actionAt(i)
		if action.Kind != kind || (needsTarget && (targetID == "" || action.TargetID != targetID)) {
			continue
		}
		score := int64(0)
		if role := strings.ToLower(strings.TrimSpace(spec.ItemRole)); role != "" && role != "any" {
			var eligible bool
			score, eligible = rulebookItemScore(p, action.Item, role)
			if !eligible {
				continue
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func synthesizeBehaviorCandidate(p sim.Perception, allowed map[string]bool, spec BehaviorSpec, targetID string, basisEventID int64, actor1Name, actor2Name string) (sim.Action, string, bool) {
	kind := strings.TrimSpace(spec.EngineKind)
	if kind == "" || !allowed[kind] {
		return sim.Action{}, "", false
	}
	a := p.Self
	validBasis := int64(0)
	if basisEventID > 0 {
		for _, m := range a.Memories {
			if m.EventID == basisEventID {
				validBasis = basisEventID
				break
			}
		}
	}

	var action sim.Action
	var itemName string

	if declSpec, isDecl := p.Catalog.ActionSpecs[kind]; isDecl {
		action = sim.Action{Kind: kind, BasisEventID: validBasis}
		switch declSpec.TargetKind {
		case "actor":
			if targetID == "" || targetID == a.ID {
				return sim.Action{}, "", false
			}
			action.TargetID = targetID
		case "cohort":
			if targetID != "" {
				action.TargetID = targetID
			} else {
				for _, c := range p.LocalCohorts {
					if c.District == a.Location && c.Population > 0 {
						action.TargetID = c.ID
						break
					}
				}
			}
			if action.TargetID == "" {
				return sim.Action{}, "", false
			}
		case "district":
			loc := a.Location
			if spec.LocationFeature != "" {
				if dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, spec.LocationFeature); ok {
					loc = dest
				}
			}
			action.Location = loc
		case "venue":
			if a.VenueID != "" {
				action.Location = a.VenueID
			} else {
				for _, v := range p.Map.Venues {
					if v.DistrictID == a.Location {
						action.Location = v.ID
						break
					}
				}
			}
		}
		if strings.TrimSpace(spec.SummaryTemplate) != "" {
			action.Text = renderCausalTemplate(spec.SummaryTemplate, actor1Name, actor2Name, a.Name, a.Location, "", "")
		}
	} else {
		switch kind {
		case "move", "move_venue":
			if spec.LocationFeature == "" {
				return sim.Action{}, "", false
			}
			dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, spec.LocationFeature)
			if !ok || dest == a.Location {
				return sim.Action{}, "", false
			}
			step, ok := stepTowardDistrict(p, dest, 0)
			if !ok {
				return sim.Action{}, "", false
			}
			action = step
			action.BasisEventID = validBasis
		case "speak", "transmit_message":
			if targetID == "" || targetID == a.ID {
				return sim.Action{}, "", false
			}
			action = sim.Action{Kind: kind, TargetID: targetID, BasisEventID: validBasis}
			if strings.TrimSpace(spec.SummaryTemplate) != "" {
				action.Text = renderCausalTemplate(spec.SummaryTemplate, actor1Name, actor2Name, a.Name, a.Location, "", "")
			} else {
				action = buildCommunicationAction(p, action)
			}
		case "offer":
			if targetID == "" || targetID == a.ID {
				return sim.Action{}, "", false
			}
			itemID := selectRulebookItem(p, spec.ItemRole, true)
			if itemID == "" {
				return sim.Action{}, "", false
			}
			itemSpec := p.Catalog.Items[itemID]
			held := a.Items[itemID]
			batchQty := max(1, itemSpec.BatchQty)
			qty := min(held, batchQty)
			price := planBatchPrice(max(1, itemSpec.BatchPrice), qty, batchQty)
			action = sim.Action{
				Kind:         "offer",
				TargetID:     targetID,
				Item:         itemID,
				Quantity:     qty,
				Price:        price,
				BasisEventID: validBasis,
			}
			itemName = itemID
		case "heal":
			if targetID == "" || targetID == a.ID {
				return sim.Action{}, "", false
			}
			itemID := selectRulebookItem(p, "medicinal", false)
			if itemID == "" {
				return sim.Action{}, "", false
			}
			action = sim.Action{Kind: "heal", TargetID: targetID, Item: itemID, BasisEventID: validBasis}
			itemName = itemID
		case "bribe":
			if targetID == "" || targetID == a.ID || a.Money <= 0 {
				return sim.Action{}, "", false
			}
			action = sim.Action{Kind: "bribe", TargetID: targetID, Price: min(a.Money, 4), BasisEventID: validBasis}
		default:
			meta, ok := p.Catalog.ActionMetadataByKind(kind)
			if !ok {
				return sim.Action{}, "", false
			}
			action = sim.Action{Kind: kind, BasisEventID: validBasis}
			if meta.Targeted {
				if targetID == "" || targetID == a.ID {
					return sim.Action{}, "", false
				}
				action.TargetID = targetID
			}
		}
	}

	if res := p.CheckAction(action); !res.Allowed {
		return sim.Action{}, "", false
	}
	label := renderCausalTemplate(spec.SummaryTemplate, actor1Name, actor2Name, a.Name, a.Location, itemName, "")
	if strings.TrimSpace(spec.SummaryTemplate) == "" {
		if action.TargetID != "" {
			label = fmt.Sprintf("%s -> %s", kind, actor2Name)
		} else {
			label = kind
		}
		if itemName != "" {
			label += fmt.Sprintf(" (%d %s)", max(1, action.Quantity), itemName)
		}
	}
	return action, label, true
}

func selectRulebookItem(p sim.Perception, itemRole string, requireTradeable bool) string {
	role := strings.ToLower(strings.TrimSpace(itemRole))
	itemKeys := sortedItemIDs(p.Catalog.Items)
	bestID := ""
	bestScore := int64(-1)
	for _, id := range itemKeys {
		qty := p.Self.Items[id]
		if qty <= 0 {
			continue
		}
		spec := p.Catalog.Items[id]
		if requireTradeable && !spec.Tradeable {
			continue
		}
		score, eligible := rulebookItemScore(p, id, role)
		if !eligible {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestID = id
		}
	}
	return bestID
}

func rulebookItemScore(p sim.Perception, id, role string) (int64, bool) {
	item, known := p.Catalog.Items[id]
	if !known {
		return 0, false
	}
	qty := p.Self.Items[id]
	switch role {
	case "edible":
		return 100 + item.NutritionUnits, item.Edible
	case "medicinal":
		return 100 + int64(item.MedicinalHeal) + int64(item.DiseaseCure)*10, item.MedicinalHeal > 0 || item.DiseaseCure > 0
	case "combat":
		return 100 + int64(item.CombatBonus)*10, item.CombatBonus > 0
	case "highest_tier":
		return 100 + int64(item.Tier)*20 + int64(item.BatchPrice), true
	case "surplus":
		return 100 + int64(qty), qty >= item.OfferThreshold
	case "", "any":
		return 10 + int64(item.Tier)*5 + int64(qty), true
	default:
		return 200, id == role
	}
}

func findCandidateByID(candidates []sim.Candidate, id string) (sim.Candidate, bool) {
	for _, c := range candidates {
		if c.ID == id {
			return c, true
		}
	}
	return sim.Candidate{}, false
}

package sim

import (
	"sort"
	"strings"
	"unicode"
)

// MaxRecalledMemories bounds attention, not the persisted personal history.
const MaxRecalledMemories = 32

// MemoryCues describe what the actor is considering. They are retrieval cues,
// not facts, beliefs, or changes to the actor's memories.
type MemoryCues struct {
	Terms    []string
	ActorIDs []string
}

func appendMemory(list []Memory, m Memory) []Memory { return append(list, m) }

// RecallMemories selects only this actor's experiences. Recent context is kept
// alongside older relevant and consequential experiences, in original order.
// Ranking never changes a statement into an observation or assesses its truth.
func RecallMemories(a Actor, cues MemoryCues, limit int) []Memory {
	if limit <= 0 || len(a.Memories) == 0 {
		return nil
	}
	terms := append([]string(nil), cues.Terms...)
	terms = append(terms, strings.FieldsFunc(strings.ToLower(a.Goal), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })...)
	peers := append([]string(nil), cues.ActorIDs...)
	if a.Plan != nil && a.Plan.Status == "active" {
		terms = append(terms, a.Plan.Item, a.Plan.RecipeID)
	}
	if a.Task != nil {
		terms = append(terms, a.Task.Action.Kind, a.Task.Action.Item, a.Task.Action.Location)
		peers = append(peers, a.Task.Action.TargetID)
	}
	// Normalize query terms once, not once per remembered event.
	uniqueTerms := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(strings.ToLower(term))
		if term != "" && !seen[term] {
			uniqueTerms = append(uniqueTerms, term)
			seen[term] = true
		}
	}
	peerSet := make(map[string]bool, len(peers))
	for _, peer := range peers {
		if peer != "" {
			peerSet[peer] = true
		}
	}
	limit = min(limit, len(a.Memories))
	recent := min(8, limit)
	capacity := limit - recent
	best := make([]rankedMemory, 0, capacity)
	if capacity > 0 {
		for i, m := range a.Memories[:len(a.Memories)-recent] {
			score := 0
			// Consequential first-person experiences remain salient even when distant.
			if m.Source == a.ID || m.TargetID == a.ID {
				switch m.Kind {
				case "attacked", "attack", "healed", "heal", "arrested", "arrest", "loan_issued", "loan_repaid", "wrongful_conviction", "conviction_overturned", "agreement_accepted", "agreement_fulfilled", "agreement_unfulfilled", "actor_died", "stolen", "steal_foiled", "raided", "raid_repelled", "extorted", "extort_resisted", "bribed", "corruption_purged", "medicated":
					score += 4
				}
			}
			if peerSet[m.Source] || peerSet[m.TargetID] ||
				(m.Observation != nil && (peerSet[m.Observation.SubjectID] || peerSet[m.Observation.ObjectID])) ||
				(m.Statement != nil && (peerSet[m.Statement.SubjectID] || peerSet[m.Statement.ObjectID])) {
				score += 6
			}
			if (m.Observation != nil && (m.Observation.Kind == "corruption" || m.Observation.Kind == "wrongful_conviction")) ||
				(m.Statement != nil && (m.Statement.Topic == "corruption" || m.Statement.Topic == "wrongful_conviction")) {
				score += 5
			}
			content := strings.ToLower(m.Kind + " " + m.Text)
			if m.Observation != nil {
				content += " " + strings.ToLower(m.Observation.Kind+" "+m.Observation.Item+" "+m.Observation.Location)
			}
			if m.Statement != nil {
				content += " " + strings.ToLower(m.Statement.Topic+" "+m.Statement.Item+" "+m.Statement.Location)
			}
			for _, term := range uniqueTerms {
				if strings.Contains(content, term) {
					score += 3
				}
			}
			candidate := rankedMemory{index: i, score: score}
			if len(best) < capacity {
				best = append(best, candidate)
				for child := len(best) - 1; child > 0; {
					parent := (child - 1) / 2
					if !memoryWorse(best[child], best[parent]) {
						break
					}
					best[child], best[parent] = best[parent], best[child]
					child = parent
				}
			} else if memoryWorse(best[0], candidate) {
				best[0] = candidate
				for parent := 0; ; {
					child := parent*2 + 1
					if child >= len(best) {
						break
					}
					if child+1 < len(best) && memoryWorse(best[child+1], best[child]) {
						child++
					}
					if !memoryWorse(best[child], best[parent]) {
						break
					}
					best[child], best[parent] = best[parent], best[child]
					parent = child
				}
			}
		}
	}
	indexes := make([]int, 0, limit)
	for _, m := range best {
		indexes = append(indexes, m.index)
	}
	for i := len(a.Memories) - recent; i < len(a.Memories); i++ {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	result := make([]Memory, 0, len(indexes))
	for _, i := range indexes {
		m := a.Memories[i]
		m.SourceTags = append([]string(nil), m.SourceTags...)
		if m.Observation != nil {
			o := *m.Observation
			m.Observation = &o
		}
		if m.Statement != nil {
			s := *m.Statement
			m.Statement = &s
		}
		result = append(result, m)
	}
	return result
}

// A bounded min-heap retains the strongest matches. Its root is the least
// relevant (and, on a tie, oldest) selected memory.
type rankedMemory struct{ index, score int }

func memoryWorse(a, b rankedMemory) bool {
	if a.score != b.score {
		return a.score < b.score
	}
	return a.index < b.index
}

func recallPerceptionMemories(p *Perception) {
	cues := MemoryCues{}
	for _, g := range p.Continuity.Agreements {
		if g.Status == "proposed" || g.Status == "active" {
			cues.Terms = append(cues.Terms, g.Item)
			if g.SupplierID != p.Self.ID {
				cues.ActorIDs = append(cues.ActorIDs, g.SupplierID)
			}
			if g.BuyerID != p.Self.ID {
				cues.ActorIDs = append(cues.ActorIDs, g.BuyerID)
			}
		}
	}
	for _, n := range p.Nearby {
		cues.ActorIDs = append(cues.ActorIDs, n.ID)
	}
	for _, o := range p.Offers {
		cues.ActorIDs = append(cues.ActorIDs, o.Buyer, o.Seller)
		cues.Terms = append(cues.Terms, o.Item)
	}
	p.Self.Memories = RecallMemories(p.Self, cues, MaxRecalledMemories)
}

package sim

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// ValidateCitation checks what a cited memory can be used to report. Speakers
// own their words, including mistaken quantities and uncited assertions.
func ValidateCitation(memories []Memory, basis int64, statement *Statement) error {
	if basis == 0 {
		return nil
	}
	for _, m := range memories {
		if m.EventID != basis {
			continue
		}
		if statement == nil || statement.Topic != "item_produced" {
			return nil
		}
		if m.Observation != nil && m.Observation.Kind == "item_produced" && m.Observation.Item == statement.Item {
			return nil
		}
		if m.Statement != nil && m.Statement.Topic == "item_produced" && m.Statement.Item == statement.Item {
			return nil
		}
		return fmt.Errorf("memory %d (%s) contains no production observation or reported production of %q", basis, m.Kind, statement.Item)
	}
	return fmt.Errorf("unavailable memory event %d", basis)
}

func citedRoot(memories []Memory, basis int64) int64 {
	if basis != 0 {
		for i := len(memories) - 1; i >= 0; i-- {
			m := memories[i]
			if m.EventID == basis {
				if m.RootEventID != 0 {
					return m.RootEventID
				}
				return m.EventID
			}
		}
	}
	return 0
}

// citedReport carries the original source through each relay, after checking
// that the cited memory actually supports this statement.
func citedReport(sender *Actor, x Action) (int64, string) {
	if x.BasisEventID == 0 || x.Statement == nil {
		return 0, ""
	}
	s := x.Statement
	for i := len(sender.Memories) - 1; i >= 0; i-- {
		m := sender.Memories[i]
		if m.EventID != x.BasisEventID {
			continue
		}
		if o := m.Observation; o != nil && o.Kind == s.Topic && o.SubjectID == s.SubjectID &&
			o.ObjectID == s.ObjectID && o.CaseID == s.CaseID && (s.Modality == "" || s.Modality == o.Modality) && o.Location == s.Location && o.Proceeds == s.Proceeds && (o.Item == s.Item || s.Item == "") && o.Quantity == s.Quantity {
			return m.EventID, sender.ID
		}
		if m.Statement != nil && *m.Statement == *s {
			if m.FactEventID > 0 && m.WitnessID != "" {
				return m.FactEventID, m.WitnessID
			}
			// Preserve original unverified speaker provenance (FactEventID stays 0).
			if m.WitnessID != "" {
				return 0, m.WitnessID
			}
			return 0, m.Source
		}
	}
	return 0, ""
}

// MaxRecalledMemories bounds attention, not the persisted personal history.
const MaxRecalledMemories = 32

// MemoryCues describe what the actor is considering. They are retrieval cues,
// not facts, beliefs, or changes to the actor's memories.
type MemoryCues struct {
	Terms    []string
	ActorIDs []string
}

func appendMemory(list []Memory, m Memory) []Memory { return append(list, m) }

// Rebuild derived search data once on load. Rebuilding it for every later
// decision made long checkpoint replays repeatedly allocate the whole history.
func (m *Memory) UnmarshalJSON(data []byte) error {
	type storedMemory Memory
	stored := storedMemory(*m)
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	*m = Memory(stored)
	m.search = newMemorySearchCache(*m)
	return nil
}

// These immutable search strings are derived data, shared by witnesses of the
// same event and excluded from persistence. A changed memory invalidates its
// cached key; loaded histories rebuild the same immutable data once.
type memorySearchKey struct {
	kind, channel, text string
	observation         Observation
	statement           Statement
	hasObservation      bool
	hasStatement        bool
}

type memorySearchCache struct {
	key        memorySearchKey
	normalized string
}

func searchKey(m Memory) memorySearchKey {
	key := memorySearchKey{kind: m.Kind, channel: m.Channel, text: m.Text}
	if m.Observation != nil {
		key.observation, key.hasObservation = *m.Observation, true
	}
	if m.Statement != nil {
		key.statement, key.hasStatement = *m.Statement, true
	}
	return key
}

func newMemorySearchCache(m Memory) *memorySearchCache {
	key := searchKey(m)
	content := key.kind + " " + key.channel + " " + key.text
	if key.hasObservation {
		o := key.observation
		content += " " + o.Kind + " " + o.Modality + " " + o.SubjectID + " " + o.Item + " " + o.Location
	}
	if key.hasStatement {
		s := key.statement
		content += " " + s.Topic + " " + s.SubjectID + " " + s.Item + " " + s.Location
	}
	return &memorySearchCache{key: key, normalized: strings.ToLower(content)}
}

func searchableMemory(m Memory) string {
	if m.search != nil && m.search.key == searchKey(m) {
		return m.search.normalized
	}
	return newMemorySearchCache(m).normalized
}

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
			if (m.Observation != nil && (m.Observation.Kind == "corruption" || m.Observation.Kind == "wrongful_conviction" || m.Observation.Kind == "macro_anomaly")) ||
				(m.Statement != nil && (m.Statement.Topic == "corruption" || m.Statement.Topic == "wrongful_conviction" || m.Statement.Topic == "macro_anomaly")) {
				score += 5
			}
			if len(uniqueTerms) > 0 {
				content := searchableMemory(m)
				for _, term := range uniqueTerms {
					if strings.Contains(content, term) {
						score += 3
					}
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

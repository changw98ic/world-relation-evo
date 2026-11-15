package sim

import (
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func referenceRecallMemories(a Actor, cues MemoryCues, limit int) []Memory {
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
	type ranked struct{ index, score int }
	rankedMemories := make([]ranked, 0, len(a.Memories))
	selected := make(map[int]bool)
	recent := min(8, limit, len(a.Memories))
	for i := len(a.Memories) - recent; i < len(a.Memories); i++ {
		selected[i] = true
	}
	for i, m := range a.Memories[:len(a.Memories)-recent] {
		score := 0
		// Consequential first-person experiences remain salient even when distant.
		if m.Source == a.ID || m.TargetID == a.ID {
			switch m.Kind {
			case "attacked", "attack", "healed", "heal", "arrested", "arrest", "loan_issued", "loan_repaid", "wrongful_conviction", "conviction_overturned", "agreement_accepted", "agreement_fulfilled", "agreement_unfulfilled", "actor_died", "stolen", "steal_foiled", "raided", "raid_repelled", "extorted", "extort_resisted", "bribed", "corruption_purged", "medicated":
				score += 4
			}
		}
		for _, peer := range peers {
			if peer != "" && (m.Source == peer || m.TargetID == peer) {
				score += 6
				break
			}
		}
		content := strings.ToLower(m.Kind + " " + m.Text)
		if m.Observation != nil {
			content += " " + strings.ToLower(m.Observation.Kind+" "+m.Observation.Item+" "+m.Observation.Location)
		}
		if m.Statement != nil {
			content += " " + strings.ToLower(m.Statement.Topic+" "+m.Statement.Item+" "+m.Statement.Location)
		}
		seen := map[string]bool{}
		for _, term := range terms {
			term = strings.TrimSpace(strings.ToLower(term))
			if term != "" && !seen[term] && strings.Contains(content, term) {
				score += 3
				seen[term] = true
			}
		}
		rankedMemories = append(rankedMemories, ranked{i, score})
	}
	sort.Slice(rankedMemories, func(i, j int) bool {
		if rankedMemories[i].score != rankedMemories[j].score {
			return rankedMemories[i].score > rankedMemories[j].score
		}
		return rankedMemories[i].index > rankedMemories[j].index
	})
	for _, r := range rankedMemories {
		if len(selected) >= limit {
			break
		}
		selected[r.index] = true
	}
	result := make([]Memory, 0, len(selected))
	for i, m := range a.Memories {
		if !selected[i] {
			continue
		}
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

func TestBoundedRecallMatchesFullSort(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 100; trial++ {
		a := Actor{ID: "self", Goal: "grain food", Plan: &PersonalPlan{Status: "active", Item: "medicine"}}
		kinds := []string{"rest", "attacked", "loan_issued", "speech"}
		sources := []string{"self", "friend", "stranger", ""}
		texts := []string{"grain food", "medicine", "other", "GRAIN"}
		for i := 0; i < rng.Intn(400)+trial; i++ {
			m := Memory{EventID: int64(i), Kind: kinds[rng.Intn(len(kinds))], Source: sources[rng.Intn(len(sources))], TargetID: sources[rng.Intn(len(sources))], Text: texts[rng.Intn(len(texts))]}
			if i%3 == 0 {
				m.Statement = &Statement{Topic: "food", Quantity: i}
			}
			if i%4 == 0 {
				m.Observation = &Observation{Item: "grain", Quantity: i}
			}
			a.Memories = append(a.Memories, m)
		}
		cues := MemoryCues{Terms: []string{" Grain ", "grain", "food", ""}, ActorIDs: []string{"friend", "", "friend"}}
		for _, limit := range []int{0, 1, 8, 9, 32, 100, 500} {
			got, want := RecallMemories(a, cues, limit), referenceRecallMemories(a, cues, limit)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("trial=%d limit=%d: bounded selection differs", trial, limit)
			}
		}
	}
}

func BenchmarkRecallSelection(b *testing.B) {
	a := Actor{ID: "self", Goal: "grain food"}
	for i := 0; i < 5000; i++ {
		a.Memories = append(a.Memories, Memory{EventID: int64(i), Kind: "speech", Source: "friend", Text: "grain food at market"})
	}
	cues := MemoryCues{Terms: []string{"grain", "food", " grain "}, ActorIDs: []string{"friend"}}
	b.Run("bounded", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			RecallMemories(a, cues, 32)
		}
	})
	b.Run("full_sort_reference", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			referenceRecallMemories(a, cues, 32)
		}
	})
}

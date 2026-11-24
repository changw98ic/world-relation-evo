package optiongen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MineConfig controls deterministic high-likelihood option combination mining.
type MineConfig struct {
	TopN           int // number of top high-likelihood combination scenarios to select (default 12)
	MaxPerPair     int // maximum scenarios dominated by the same unordered actor pair (default 2)
	MaxOptionReuse int // maximum times a single actor option may appear across scenarios (default 2)
}

// CombinationScenario represents a high-likelihood multi-actor convergence
// discovered from individual 30-day character option catalogs.
type CombinationScenario struct {
	ID              string                `json:"id"`
	CollisionType   string                `json:"collision_type"`
	Title           string                `json:"title"`
	LikelihoodScore int                   `json:"likelihood_score"`
	LikelihoodTier  string                `json:"likelihood_tier"`
	DayWindow       [2]int                `json:"day_window"`
	SharedLocation  string                `json:"shared_location,omitempty"`
	SharedItems     []string              `json:"shared_items,omitempty"`
	Reasons         []string              `json:"reasons"`
	Participants    []ScenarioParticipant `json:"participants"`
	TriggerOptions  []ScenarioOptionRef   `json:"trigger_options"`
}

// ScenarioParticipant captures the essential state of an actor involved in a scenario.
type ScenarioParticipant struct {
	ActorID       string         `json:"actor_id"`
	ActorName     string         `json:"actor_name"`
	Role          string         `json:"role"`
	Location      string         `json:"location"`
	Money         int            `json:"money"`
	Personality   string         `json:"personality,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	Factions      []string       `json:"factions,omitempty"`
	TrustSnapshot map[string]int `json:"trust_snapshot,omitempty"`
}

// ScenarioOptionRef points to one specific actor option participating in a combination.
type ScenarioOptionRef struct {
	ActorID             string         `json:"actor_id"`
	ActorName           string         `json:"actor_name"`
	OptionID            string         `json:"option_id"`
	Category            string         `json:"category"`
	Title               string         `json:"title"`
	DayWindow           [2]int         `json:"day_window"`
	Priority            string         `json:"priority"`
	EngineKind          string         `json:"engine_kind"`
	RecipeID            string         `json:"recipe_id,omitempty"`
	TargetActorIDs      []string       `json:"target_actor_ids,omitempty"`
	TargetLocation      string         `json:"target_location,omitempty"`
	ItemsOrResources    map[string]any `json:"items_or_resources,omitempty"`
	Conditions          []string       `json:"conditions,omitempty"`
	TriggerEvent        string         `json:"trigger_event,omitempty"`
	ActionSummary       string         `json:"action_summary"`
	PotentialCollisions []string       `json:"potential_collisions,omitempty"`
}

// LoadExpansionRecords reads summary.json and all per-actor expansion JSON files
// from an expand-options output directory in deterministic order.
func LoadExpansionRecords(optionsDir string) ([]ActorExpansionRecord, *WorldExpansionSummary, error) {
	summaryPath := filepath.Join(optionsDir, "summary.json")
	rawSum, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", summaryPath, err)
	}
	var summary WorldExpansionSummary
	if err := json.Unmarshal(rawSum, &summary); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", summaryPath, err)
	}
	records := make([]ActorExpansionRecord, 0, len(summary.Actors))
	for _, brief := range summary.Actors {
		actorPath := filepath.Join(optionsDir, "actors", brief.ActorID+".json")
		raw, err := os.ReadFile(actorPath)
		if err != nil {
			return nil, nil, fmt.Errorf("read actor record %s: %w", actorPath, err)
		}
		var rec ActorExpansionRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, nil, fmt.Errorf("parse actor record %s: %w", actorPath, err)
		}
		if rec.ActorID != brief.ActorID || rec.WorldID != summary.WorldID || rec.Dossier.World.ID != summary.WorldID || rec.Dossier.World.HorizonDays != summary.HorizonDays {
			return nil, nil, fmt.Errorf("actor record %s does not match the actor, world and planning horizon in %s", actorPath, summaryPath)
		}
		rec.Catalog.fillOmittedWindows(summary.HorizonDays)
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("no actor records found in %s", optionsDir)
	}
	return records, &summary, nil
}

type scoredPair struct {
	recA        *ActorExpansionRecord
	recB        *ActorExpansionRecord
	optA        ExpandedOption
	optB        ExpandedOption
	score       int
	dayWin      [2]int
	sharedLoc   string
	sharedItems []string
	reasons     []string
	colType     string
	pairKey     string
}

// MineHighLikelihoodCombinations deterministically analyzes all per-actor 30-day
// option catalogs, scores option pairs (and 3-actor triangles) by joint selection
// likelihood, and selects a diverse, full-cast-covering Top-N set of scenarios.
func MineHighLikelihoodCombinations(records []ActorExpansionRecord, cfg MineConfig) []CombinationScenario {
	if len(records) < 2 {
		return nil
	}
	if cfg.TopN <= 0 {
		cfg.TopN = 12
	}
	if cfg.MaxPerPair <= 0 {
		cfg.MaxPerPair = 1
	}
	if cfg.MaxOptionReuse <= 0 {
		cfg.MaxOptionReuse = 1
	}

	byID := make(map[string]*ActorExpansionRecord, len(records))
	for i := range records {
		byID[records[i].ActorID] = &records[i]
	}

	var candidates []scoredPair
	for i := 0; i < len(records); i++ {
		for j := i + 1; j < len(records); j++ {
			recA := &records[i]
			recB := &records[j]
			for _, optA := range recA.Catalog.Options {
				for _, optB := range recB.Catalog.Options {
					sp, ok := scoreOptionPair(recA, optA, recB, optB)
					if ok {
						candidates = append(candidates, sp)
					}
				}
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].dayWin[0] != candidates[j].dayWin[0] {
			return candidates[i].dayWin[0] < candidates[j].dayWin[0]
		}
		if candidates[i].pairKey != candidates[j].pairKey {
			return candidates[i].pairKey < candidates[j].pairKey
		}
		if candidates[i].optA.ID != candidates[j].optA.ID {
			return candidates[i].optA.ID < candidates[j].optA.ID
		}
		return candidates[i].optB.ID < candidates[j].optB.ID
	})

	var chosen []scoredPair
	pairCount := map[string]int{}
	optUse := map[string]int{}
	actorCovered := map[string]int{}
	typeCount := map[string]int{}

	canPick := func(sp scoredPair) bool {
		if pairCount[sp.pairKey] >= cfg.MaxPerPair {
			return false
		}
		kA := sp.recA.ActorID + ":" + sp.optA.ID
		kB := sp.recB.ActorID + ":" + sp.optB.ID
		if optUse[kA] >= cfg.MaxOptionReuse || optUse[kB] >= cfg.MaxOptionReuse {
			return false
		}
		return true
	}
	pick := func(sp scoredPair) {
		chosen = append(chosen, sp)
		pairCount[sp.pairKey]++
		optUse[sp.recA.ActorID+":"+sp.optA.ID]++
		optUse[sp.recB.ActorID+":"+sp.optB.ID]++
		actorCovered[sp.recA.ActorID]++
		actorCovered[sp.recB.ActorID]++
		typeCount[sp.colType]++
	}

	// Pass 1: prioritize covering every actor in the cast at least once with their
	// highest-likelihood cross-actor collision.
	for len(chosen) < cfg.TopN {
		bestIdx := -1
		bestEffective := -1
		for idx, sp := range candidates {
			if !canPick(sp) {
				continue
			}
			uncovered := 0
			if actorCovered[sp.recA.ActorID] == 0 {
				uncovered++
			}
			if actorCovered[sp.recB.ActorID] == 0 {
				uncovered++
			}
			if uncovered == 0 {
				continue
			}
			eff := sp.score + uncovered*25 - typeCount[sp.colType]*2
			if eff > bestEffective {
				bestEffective = eff
				bestIdx = idx
			}
		}
		if bestIdx < 0 {
			break
		}
		pick(candidates[bestIdx])
	}

	// Pass 2: fill remaining slots up to TopN with highest-scoring diverse pairs.
	for len(chosen) < cfg.TopN {
		bestIdx := -1
		bestEffective := -1
		for idx, sp := range candidates {
			if !canPick(sp) {
				continue
			}
			// Encourage balanced coverage across collision types and less-featured actors.
			eff := sp.score - typeCount[sp.colType]*4 - (actorCovered[sp.recA.ActorID]+actorCovered[sp.recB.ActorID])*2
			if eff > bestEffective {
				bestEffective = eff
				bestIdx = idx
			}
		}
		if bestIdx < 0 {
			break
		}
		pick(candidates[bestIdx])
	}

	// Sort final selection by likelihood score descending, then start day ascending.
	sort.Slice(chosen, func(i, j int) bool {
		if chosen[i].score != chosen[j].score {
			return chosen[i].score > chosen[j].score
		}
		if chosen[i].dayWin[0] != chosen[j].dayWin[0] {
			return chosen[i].dayWin[0] < chosen[j].dayWin[0]
		}
		return chosen[i].pairKey < chosen[j].pairKey
	})

	out := make([]CombinationScenario, 0, len(chosen))
	triangleOptUse := map[string]int{}
	usedTrios := map[string]bool{}
	for idx, sp := range chosen {
		scn := buildScenario(idx+1, sp, records, triangleOptUse, usedTrios, cfg.MaxOptionReuse)
		out = append(out, scn)
	}
	return out
}

func scoreOptionPair(recA *ActorExpansionRecord, optA ExpandedOption, recB *ActorExpansionRecord, optB ExpandedOption) (scoredPair, bool) {
	win, overlap := intersectDayWindow(optA.DayWindow, optB.DayWindow)
	if overlap <= 0 {
		return scoredPair{}, false
	}

	aTargetsB := containsString(optA.Action.TargetActorIDs, recB.ActorID)
	bTargetsA := containsString(optB.Action.TargetActorIDs, recA.ActorID)
	aMentionsB := collisionsMentionName(optA.PotentialCollisions, recB.ActorName)
	bMentionsA := collisionsMentionName(optB.PotentialCollisions, recA.ActorName)

	locA := effectiveLocation(optA.Action.TargetLocation, recA.Dossier.Actor.Location)
	locB := effectiveLocation(optB.Action.TargetLocation, recB.Dossier.Actor.Location)
	sharedLoc := ""
	if locA != "" && locA == locB {
		sharedLoc = locA
	}
	sharedItems := intersectItemKeys(optA.Action.ItemsOrResources, optB.Action.ItemsOrResources)

	// Must have an actual causal link (direct target, collision mention, or same location + shared resource).
	if !aTargetsB && !bTargetsA && !aMentionsB && !bMentionsA && !(sharedLoc != "" && len(sharedItems) > 0) {
		return scoredPair{}, false
	}

	score := 0
	var reasons []string

	// 1. Temporal overlap (up to 16 pts).
	spanA := optA.DayWindow[1] - optA.DayWindow[0] + 1
	spanB := optB.DayWindow[1] - optB.DayWindow[0] + 1
	minSpan := min(spanA, spanB)
	overlapPts := min(16, 4+(overlap*12)/minSpan)
	score += overlapPts

	// 2. Priority joint selection likelihood (up to 28 pts).
	pA := priorityWeight(optA.Priority)
	pB := priorityWeight(optB.Priority)
	score += (pA + pB) / 2
	if pA >= 20 && pB >= 20 {
		score += 6
		reasons = append(reasons, "双方均属高优/紧急选项，同期触发概率高")
	}

	// 3. Direct cross-actor targeting & collision mentions (up to 36 pts).
	switch {
	case aTargetsB && bTargetsA:
		score += 30
		reasons = append(reasons, fmt.Sprintf("%s与%s的行动直接互指对方", recA.ActorName, recB.ActorName))
	case aTargetsB:
		score += 18
		reasons = append(reasons, fmt.Sprintf("%s的行动直接指向%s", recA.ActorName, recB.ActorName))
	case bTargetsA:
		score += 18
		reasons = append(reasons, fmt.Sprintf("%s的行动直接指向%s", recB.ActorName, recA.ActorName))
	}
	if aMentionsB && bMentionsA {
		score += 8
		reasons = append(reasons, "双方档案碰撞预判互相点名")
	} else if aMentionsB || bMentionsA {
		score += 4
	}

	// 4. Shared location & contested resources (up to 16 pts).
	if sharedLoc != "" {
		score += 8
		locName := glossaryLookup(recA.Dossier.World.Glossaries.Locations, sharedLoc)
		reasons = append(reasons, fmt.Sprintf("同在%s交汇", locName))
	}
	if len(sharedItems) > 0 {
		score += min(8, len(sharedItems)*5)
		itemNames := make([]string, 0, len(sharedItems))
		for _, it := range sharedItems {
			itemNames = append(itemNames, glossaryLookup(recA.Dossier.World.Glossaries.Items, it))
		}
		reasons = append(reasons, fmt.Sprintf("围绕%s产生直接供需或争夺", strings.Join(itemNames, "、")))
	}

	// 5. Relationship tension (up to 8 pts).
	trustAB := recA.Dossier.Actor.Trust[recB.ActorID]
	trustBA := recB.Dossier.Actor.Trust[recA.ActorID]
	if trustAB <= -2 || trustBA <= -2 {
		score += 8
		reasons = append(reasons, "双方本就存有敌意或猜忌，极易激化")
	} else if trustAB >= 2 && trustBA >= 2 {
		score += 5
		reasons = append(reasons, "双方互信较高，联动或利益牵连紧密")
	} else if trustAB < 0 || trustBA < 0 {
		score += 4
	}

	if score > 99 {
		score = 99
	}
	if score < 45 {
		return scoredPair{}, false
	}

	pairKey := recA.ActorID + "|" + recB.ActorID
	if recB.ActorID < recA.ActorID {
		pairKey = recB.ActorID + "|" + recA.ActorID
	}

	return scoredPair{
		recA:        recA,
		recB:        recB,
		optA:        optA,
		optB:        optB,
		score:       score,
		dayWin:      win,
		sharedLoc:   sharedLoc,
		sharedItems: sharedItems,
		reasons:     reasons,
		colType:     classifyCollision(optA, optB, sharedItems),
		pairKey:     pairKey,
	}, true
}

func buildScenario(seq int, sp scoredPair, allRecords []ActorExpansionRecord, optUse map[string]int, usedTrios map[string]bool, maxOptReuse int) CombinationScenario {
	refs := []ScenarioOptionRef{
		toOptionRef(sp.recA, sp.optA),
		toOptionRef(sp.recB, sp.optB),
	}
	involvedRecs := []*ActorExpansionRecord{sp.recA, sp.recB}
	involvedIDs := map[string]bool{sp.recA.ActorID: true, sp.recB.ActorID: true}
	reasons := append([]string(nil), sp.reasons...)

	// Optionally attach 1 high-relevance third actor to form a 3-way triangle when
	// that actor has an overlapping high-priority option directly targeting A or B.
	var bestThirdRec *ActorExpansionRecord
	var bestThirdOpt ExpandedOption
	var bestThirdWin [2]int
	bestThirdScore := 0
	for i := range allRecords {
		recC := &allRecords[i]
		if involvedIDs[recC.ActorID] {
			continue
		}
		trioKey := sortedActorSetKey(sp.recA.ActorID, sp.recB.ActorID, recC.ActorID)
		if usedTrios[trioKey] {
			continue
		}
		for _, optC := range recC.Catalog.Options {
			if optUse[recC.ActorID+":"+optC.ID] >= maxOptReuse {
				continue
			}
			win3, ov := intersectDayWindow(sp.dayWin, optC.DayWindow)
			if ov <= 0 {
				continue
			}
			pC := priorityWeight(optC.Priority)
			if pC < 14 {
				continue
			}
			hitsA := containsString(optC.Action.TargetActorIDs, sp.recA.ActorID)
			hitsB := containsString(optC.Action.TargetActorIDs, sp.recB.ActorID)
			aHitsC := containsString(sp.optA.Action.TargetActorIDs, recC.ActorID) || collisionsMentionName(sp.optA.PotentialCollisions, recC.ActorName)
			bHitsC := containsString(sp.optB.Action.TargetActorIDs, recC.ActorID) || collisionsMentionName(sp.optB.PotentialCollisions, recC.ActorName)
			tScore := 0
			if hitsA && hitsB {
				tScore += 30
			} else if hitsA || hitsB {
				tScore += 16
			}
			if aHitsC {
				tScore += 10
			}
			if bHitsC {
				tScore += 10
			}
			if sp.sharedLoc != "" && effectiveLocation(optC.Action.TargetLocation, recC.Dossier.Actor.Location) == sp.sharedLoc {
				tScore += 8
			}
			if tScore < 26 {
				continue
			}
			tScore += pC / 2
			if tScore > bestThirdScore {
				bestThirdScore = tScore
				bestThirdRec = recC
				bestThirdOpt = optC
				bestThirdWin = win3
			}
		}
	}
	dayWin := sp.dayWin
	if bestThirdRec != nil {
		refs = append(refs, toOptionRef(bestThirdRec, bestThirdOpt))
		involvedRecs = append(involvedRecs, bestThirdRec)
		involvedIDs[bestThirdRec.ActorID] = true
		optUse[bestThirdRec.ActorID+":"+bestThirdOpt.ID]++
		usedTrios[sortedActorSetKey(sp.recA.ActorID, sp.recB.ActorID, bestThirdRec.ActorID)] = true
		dayWin = bestThirdWin
		reasons = append(reasons, fmt.Sprintf("%s同期卷入形成三方牵制", bestThirdRec.ActorName))
	}

	participants := make([]ScenarioParticipant, 0, len(involvedRecs))
	for _, r := range involvedRecs {
		trustSnap := map[string]int{}
		for oid := range involvedIDs {
			if oid != r.ActorID {
				if v, ok := r.Dossier.Actor.Trust[oid]; ok && v != 0 {
					trustSnap[oid] = v
				}
			}
		}
		var facs []string
		for _, f := range r.Dossier.Factions {
			if f.ActorIsMember {
				if f.Name != "" {
					facs = append(facs, f.Name)
				} else {
					facs = append(facs, f.ID)
				}
			}
		}
		participants = append(participants, ScenarioParticipant{
			ActorID:       r.ActorID,
			ActorName:     r.ActorName,
			Role:          r.Dossier.Actor.Role,
			Location:      r.Dossier.Actor.Location,
			Money:         r.Dossier.Actor.Money,
			Personality:   r.Dossier.Actor.Personality,
			Summary:       r.Catalog.CharacterSummary,
			Factions:      facs,
			TrustSnapshot: trustSnap,
		})
	}

	title := fmt.Sprintf("%s「%s」× %s「%s」", sp.recA.ActorName, sp.optA.Title, sp.recB.ActorName, sp.optB.Title)
	if bestThirdRec != nil {
		title = fmt.Sprintf("%s × %s × %s：%s", sp.recA.ActorName, sp.recB.ActorName, bestThirdRec.ActorName, sp.optA.Title)
	}

	return CombinationScenario{
		ID:              fmt.Sprintf("scn_%02d", seq),
		CollisionType:   sp.colType,
		Title:           title,
		LikelihoodScore: sp.score,
		LikelihoodTier:  likelihoodTier(sp.score),
		DayWindow:       dayWin,
		SharedLocation:  sp.sharedLoc,
		SharedItems:     sp.sharedItems,
		Reasons:         reasons,
		Participants:    participants,
		TriggerOptions:  refs,
	}
}

func toOptionRef(rec *ActorExpansionRecord, opt ExpandedOption) ScenarioOptionRef {
	return ScenarioOptionRef{
		ActorID:             rec.ActorID,
		ActorName:           rec.ActorName,
		OptionID:            opt.ID,
		Category:            opt.Category,
		Title:               opt.Title,
		DayWindow:           opt.DayWindow,
		Priority:            opt.Priority,
		EngineKind:          opt.Action.EngineKind,
		RecipeID:            opt.Action.RecipeID,
		TargetActorIDs:      opt.Action.TargetActorIDs,
		TargetLocation:      opt.Action.TargetLocation,
		ItemsOrResources:    opt.Action.ItemsOrResources,
		Conditions:          opt.WhenIf.Conditions,
		TriggerEvent:        opt.WhenIf.TriggerEvent,
		ActionSummary:       opt.Action.Summary,
		PotentialCollisions: opt.PotentialCollisions,
	}
}

func classifyCollision(a, b ExpandedOption, sharedItems []string) string {
	hostileKinds := map[string]bool{
		"arrest": true, "attack": true, "raid": true, "steal": true, "frame_suspect": true,
		"audit_corruption": true, "avenge_feud": true, "bribe": true, "claim_turf": true,
	}
	if a.Category == "竞争博弈" || b.Category == "竞争博弈" || hostileKinds[a.Action.EngineKind] || hostileKinds[b.Action.EngineKind] {
		return "正面博弈"
	}
	if a.Category == "突发危机" || b.Category == "突发危机" {
		return "危机连锁"
	}
	if a.Category == "资源交易" || b.Category == "资源交易" || len(sharedItems) > 0 {
		return "资源竞夺"
	}
	return "合纵连横"
}

func likelihoodTier(score int) string {
	switch {
	case score >= 82:
		return "极高"
	case score >= 68:
		return "高"
	default:
		return "中高"
	}
}

func priorityWeight(p string) int {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "urgent":
		return 26
	case "high":
		return 20
	case "medium":
		return 13
	default:
		return 7
	}
}

func intersectDayWindow(w1, w2 [2]int) ([2]int, int) {
	lo := max(w1[0], w2[0])
	hi := min(w1[1], w2[1])
	if hi < lo {
		return [2]int{}, 0
	}
	return [2]int{lo, hi}, hi - lo + 1
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func collisionsMentionName(cols []string, name string) bool {
	if name == "" {
		return false
	}
	for _, c := range cols {
		if strings.Contains(c, name) {
			return true
		}
	}
	return false
}

func effectiveLocation(optLoc, actorLoc string) string {
	if optLoc != "" {
		return optLoc
	}
	return actorLoc
}

func intersectItemKeys(a, b map[string]any) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []string
	for k := range a {
		if _, ok := b[k]; ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func glossaryLookup(m map[string]string, id string) string {
	if v, ok := m[id]; ok && v != "" {
		return v
	}
	return id
}

func sortedActorSetKey(ids ...string) string {
	cp := append([]string(nil), ids...)
	sort.Strings(cp)
	return strings.Join(cp, "|")
}

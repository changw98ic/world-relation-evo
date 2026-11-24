package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// RulebookDecisionEvent records one dynamically bound generic rule execution.
type RulebookDecisionEvent struct {
	Tick                     int64    `json:"tick"`
	Day                      int      `json:"day"`
	Clock                    string   `json:"clock"`
	Kind                     string   `json:"kind"` // reaction | proactive
	RuleID                   string   `json:"rule_id"`
	RuleName                 string   `json:"rule_name"`
	Category                 string   `json:"category,omitempty"`
	BranchID                 string   `json:"branch_id,omitempty"`
	Stance                   string   `json:"stance,omitempty"`
	BranchTitle              string   `json:"branch_title,omitempty"`
	ResponderPosition        string   `json:"responder_position,omitempty"` // actor2 | witness | actor1
	TriggerEventID           int64    `json:"trigger_event_id,omitempty"`
	TriggerKind              string   `json:"trigger_kind,omitempty"`
	Actor1ID                 string   `json:"actor1_id"`
	Actor1Name               string   `json:"actor1_name"`
	Actor1Role               string   `json:"actor1_role,omitempty"`
	Actor2ID                 string   `json:"actor2_id,omitempty"`
	Actor2Name               string   `json:"actor2_name,omitempty"`
	Actor2Role               string   `json:"actor2_role,omitempty"`
	ResponderID              string   `json:"responder_id"`
	ResponderName            string   `json:"responder_name"`
	ResponderRole            string   `json:"responder_role,omitempty"`
	ResponderForm            string   `json:"responder_form,omitempty"`
	ResponderPowerTier       int      `json:"responder_power_tier,omitempty"`
	TargetForm               string   `json:"target_form,omitempty"`
	TargetPowerTier          int      `json:"target_power_tier,omitempty"`
	PowerTierGap             int      `json:"power_tier_gap,omitempty"`
	PowerMultiplier          float64  `json:"power_multiplier,omitempty"`
	RelationMultiplier       float64  `json:"relation_multiplier,omitempty"`
	ResponderPersonality     string   `json:"responder_personality,omitempty"`
	TargetPersonality        string   `json:"target_personality,omitempty"`
	ResponderHobbies         []string `json:"responder_hobbies,omitempty"`
	SharedHobbies            []string `json:"shared_hobbies,omitempty"`
	BaseWeight               int      `json:"base_weight,omitempty"`
	EffectiveWeight          int      `json:"effective_weight,omitempty"`
	WeightMultiplier         float64  `json:"weight_multiplier,omitempty"`
	EncounterRate            float64  `json:"encounter_rate,omitempty"`
	ProfessionEncounterRate  float64  `json:"profession_encounter_rate,omitempty"`
	VenueEncounterRate       float64  `json:"venue_encounter_rate,omitempty"`
	PersonalityEncounterRate float64  `json:"personality_encounter_rate,omitempty"`
	TargetAttraction         float64  `json:"target_attraction,omitempty"`
	WeightReasons            []string `json:"weight_reasons,omitempty"`
	Location                 string   `json:"location"`
	IntendedKind             string   `json:"intended_kind"`
	ExecutedKind             string   `json:"executed_kind"`
	TargetID                 string   `json:"target_id,omitempty"`
	TargetName               string   `json:"target_name,omitempty"`
	Item                     string   `json:"item,omitempty"`
	RenderedSummary          string   `json:"rendered_summary"`
}

type RulebookRunStats struct {
	TotalDecisions               int                       `json:"total_decisions"`
	ProactiveDecisions           int                       `json:"proactive_decisions"`
	ReactionDecisions            int                       `json:"reaction_decisions"`
	Actor2Reactions              int                       `json:"actor2_reactions"`
	WitnessReactions             int                       `json:"witness_reactions"`
	SuppressedDirectReactions    int                       `json:"suppressed_direct_reactions"`
	SuppressedWitnessReactions   int                       `json:"suppressed_witness_reactions"`
	EncounterModulatedProactives int                       `json:"encounter_modulated_proactives"`
	TargetAttractionModulated    int                       `json:"target_attraction_modulated"`
	PersonalityWeightedReactions int                       `json:"personality_weighted_reactions"`
	PowerModulatedReactions      int                       `json:"power_modulated_reactions"`
	RelationModulatedReactions   int                       `json:"relation_modulated_reactions"`
	PowerModulatedProactives     int                       `json:"power_modulated_proactives"`
	SkillModulatedProactives     int                       `json:"skill_modulated_proactives"`
	HobbyWeightedProactives      int                       `json:"hobby_weighted_proactives"`
	SharedHobbyProactives        int                       `json:"shared_hobby_proactives"`
	RulesSurvival                int                       `json:"rules_survival"`
	RulesFallback                int                       `json:"rules_fallback"`
	UniqueProactiveRules         int                       `json:"unique_proactive_rules_fired"`
	TotalProactiveRules          int                       `json:"total_proactive_rules_loaded"`
	UniqueReactionRules          int                       `json:"unique_reaction_rules_fired"`
	TotalReactionRules           int                       `json:"total_reaction_rules_loaded"`
	UniqueActorPairs             int                       `json:"unique_actor_pairs"`
	UniqueResponders             int                       `json:"unique_responders"`
	AvgPairsPerFiredRule         float64                   `json:"avg_pairs_per_fired_rule"`
	ByStance                     map[string]int            `json:"by_stance"`
	ByCategory                   map[string]int            `json:"by_category"`
	ByTriggerKind                map[string]int            `json:"by_trigger_kind"`
	ByExecutedKind               map[string]int            `json:"by_executed_kind"`
	ByRuleCount                  map[string]int            `json:"by_rule_count"`
	ByRuleDistinctPairs          map[string]int            `json:"by_rule_distinct_pairs"`
	ByPersonalityStance          map[string]map[string]int `json:"by_personality_stance,omitempty"`
	ByHobbyAction                map[string]map[string]int `json:"by_hobby_action,omitempty"`
	ByHobbyCategory              map[string]map[string]int `json:"by_hobby_category,omitempty"`
	ByActorProactive             map[string]int            `json:"by_actor_proactive,omitempty"`
	ByActorTargeted              map[string]int            `json:"by_actor_targeted,omitempty"`
	ByActorDirectReactions       map[string]int            `json:"by_actor_direct_reactions,omitempty"`
	ByActorWitnessReactions      map[string]int            `json:"by_actor_witness_reactions,omitempty"`
	ByActorSuppressedReactions   map[string]int            `json:"by_actor_suppressed_reactions,omitempty"`
}

type RulebookLog struct {
	WorldID     string                  `json:"world_id"`
	Theme       string                  `json:"theme,omitempty"`
	Environment sim.EnvironmentProfile  `json:"environment"`
	Glossaries  PlaybookGlossaries      `json:"glossaries"`
	Stats       RulebookRunStats        `json:"stats"`
	Events      []RulebookDecisionEvent `json:"events"`
}

// RulebookSelector executes an actor-agnostic InteractionRulebook deterministically
// at runtime with zero LLM calls, reacting to real engine causal outcomes in Memory.
type RulebookSelector struct {
	Rulebook *InteractionRulebook
	Trace    *runtrace.Writer
	Seed     int64

	mu                sync.Mutex
	reactedEvents     map[string]bool  // responderID:eventID -> already handled
	proactiveCooldown map[string]int64 // actorID:ruleID -> cooldown until tick
	actorNextProTick  map[string]int64 // actorID -> next proactive decision tick
	actorNextRxnTick  map[string]int64 // actorID -> next reaction tick
	*rulebookActorKnowledge
	knowledgeByObserver map[string]*rulebookActorKnowledge
	allActorNames       map[string]string
	firedProactive      map[string]int64 // ruleID -> earliest tick fired
	firedReaction       map[string]int64 // ruleID -> earliest tick fired
	lastTriggerByActor  map[string]int64 // actorID -> causal trigger event ID for current decision
	uniquePairs         map[string]bool
	uniqueResponders    map[string]bool
	rulePairs           map[string]map[string]int64 // ruleID -> "a1->a2" -> earliest tick fired
	events              []RulebookDecisionEvent
	stats               RulebookRunStats
}

type rulebookActorKnowledge struct {
	knownActorNames         map[string]string         // dynamically observed at runtime
	knownActorRoles         map[string]string         // dynamically observed at runtime
	knownActorForms         map[string]string         // dynamically observed at runtime
	knownActorPowerTiers    map[string]int            // dynamically observed at runtime
	knownActorCombatPowers  map[string]int            // dynamically observed at runtime
	knownActorSkills        map[string]map[string]int // dynamically observed at runtime
	knownActorGuilds        map[string][]string       // dynamically observed at runtime
	knownActorHobbies       map[string][]string       // dynamically observed at runtime
	knownActorPersonalities map[string]string         // dynamically observed at runtime
}

func newRulebookActorKnowledge() *rulebookActorKnowledge {
	return &rulebookActorKnowledge{
		knownActorNames: map[string]string{}, knownActorRoles: map[string]string{},
		knownActorForms: map[string]string{}, knownActorPowerTiers: map[string]int{},
		knownActorCombatPowers: map[string]int{}, knownActorSkills: map[string]map[string]int{},
		knownActorGuilds: map[string][]string{}, knownActorHobbies: map[string][]string{},
		knownActorPersonalities: map[string]string{},
	}
}

// NewRulebookSelector constructs a zero-LLM generic RulebookSelector.
func NewRulebookSelector(rb *InteractionRulebook, trace *runtrace.Writer) *RulebookSelector {
	return &RulebookSelector{
		Rulebook:               rb,
		Trace:                  trace,
		reactedEvents:          map[string]bool{},
		proactiveCooldown:      map[string]int64{},
		actorNextProTick:       map[string]int64{},
		actorNextRxnTick:       map[string]int64{},
		rulebookActorKnowledge: newRulebookActorKnowledge(),
		knowledgeByObserver:    map[string]*rulebookActorKnowledge{},
		allActorNames:          map[string]string{},
		firedProactive:         map[string]int64{},
		firedReaction:          map[string]int64{},
		lastTriggerByActor:     map[string]int64{},
		uniquePairs:            map[string]bool{},
		uniqueResponders:       map[string]bool{},
		rulePairs:              map[string]map[string]int64{},
		stats: RulebookRunStats{
			TotalProactiveRules:        len(rb.ProactiveRules),
			TotalReactionRules:         len(rb.ReactionRules),
			ByStance:                   map[string]int{},
			ByCategory:                 map[string]int{},
			ByTriggerKind:              map[string]int{},
			ByExecutedKind:             map[string]int{},
			ByRuleCount:                map[string]int{},
			ByRuleDistinctPairs:        map[string]int{},
			ByPersonalityStance:        map[string]map[string]int{},
			ByHobbyAction:              map[string]map[string]int{},
			ByHobbyCategory:            map[string]map[string]int{},
			ByActorProactive:           map[string]int{},
			ByActorTargeted:            map[string]int{},
			ByActorDirectReactions:     map[string]int{},
			ByActorWitnessReactions:    map[string]int{},
			ByActorSuppressedReactions: map[string]int{},
		},
	}
}

func cloneNestedIntMap(in map[string]map[string]int) map[string]map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]int, len(in))
	for k, inner := range in {
		if len(inner) == 0 {
			continue
		}
		out[k] = cloneIntMap(inner)
	}
	return out
}

func incNestedInt(m map[string]map[string]int, outer, inner string) {
	if outer == "" || inner == "" {
		return
	}
	sub := m[outer]
	if sub == nil {
		sub = map[string]int{}
		m[outer] = sub
	}
	sub[inner]++
}

// SnapshotLog returns a thread-safe snapshot of generic rule execution telemetry.
func (s *RulebookSelector) SnapshotLog() RulebookLog {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.stats
	st.UniqueProactiveRules = len(s.firedProactive)
	st.UniqueReactionRules = len(s.firedReaction)
	st.UniqueActorPairs = len(s.uniquePairs)
	st.UniqueResponders = len(s.uniqueResponders)
	st.ByStance = cloneIntMap(s.stats.ByStance)
	st.ByCategory = cloneIntMap(s.stats.ByCategory)
	st.ByTriggerKind = cloneIntMap(s.stats.ByTriggerKind)
	st.ByExecutedKind = cloneIntMap(s.stats.ByExecutedKind)
	st.ByRuleCount = cloneIntMap(s.stats.ByRuleCount)
	st.ByPersonalityStance = cloneNestedIntMap(s.stats.ByPersonalityStance)
	st.ByHobbyAction = cloneNestedIntMap(s.stats.ByHobbyAction)
	st.ByHobbyCategory = cloneNestedIntMap(s.stats.ByHobbyCategory)
	st.ByActorProactive = cloneIntMap(s.stats.ByActorProactive)
	st.ByActorTargeted = cloneIntMap(s.stats.ByActorTargeted)
	st.ByActorDirectReactions = cloneIntMap(s.stats.ByActorDirectReactions)
	st.ByActorWitnessReactions = cloneIntMap(s.stats.ByActorWitnessReactions)
	st.ByActorSuppressedReactions = cloneIntMap(s.stats.ByActorSuppressedReactions)
	st.ByRuleDistinctPairs = make(map[string]int, len(s.rulePairs))
	totalPairsAcrossRules := 0
	for rid, pset := range s.rulePairs {
		st.ByRuleDistinctPairs[rid] = len(pset)
		totalPairsAcrossRules += len(pset)
	}
	if len(s.rulePairs) > 0 {
		st.AvgPairsPerFiredRule = float64(totalPairsAcrossRules) / float64(len(s.rulePairs))
	}

	evs := make([]RulebookDecisionEvent, len(s.events))
	copy(evs, s.events)
	sort.SliceStable(evs, func(i, j int) bool {
		if evs[i].Tick != evs[j].Tick {
			return evs[i].Tick < evs[j].Tick
		}
		if evs[i].ResponderID != evs[j].ResponderID {
			return evs[i].ResponderID < evs[j].ResponderID
		}
		if evs[i].Kind != evs[j].Kind {
			return evs[i].Kind < evs[j].Kind
		}
		if evs[i].RuleID != evs[j].RuleID {
			return evs[i].RuleID < evs[j].RuleID
		}
		return evs[i].TargetID < evs[j].TargetID
	})

	gloss := s.Rulebook.Glossaries
	if gloss.Actors == nil {
		gloss.Actors = map[string]string{}
	} else {
		cp := make(map[string]string, len(gloss.Actors))
		for k, v := range gloss.Actors {
			cp[k] = v
		}
		gloss.Actors = cp
	}
	for id, nm := range s.allActorNames {
		gloss.Actors[id] = nm
	}

	return RulebookLog{
		WorldID:     s.Rulebook.WorldID,
		Theme:       s.Rulebook.Theme,
		Environment: s.Rulebook.Environment,
		Glossaries:  gloss,
		Stats:       st,
		Events:      evs,
	}
}

// WriteLog serializes the RulebookLog JSON to disk.
func (s *RulebookSelector) WriteLog(path string) error {
	logData := s.SnapshotLog()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(logData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (s *RulebookSelector) observePerceptionLocked(p sim.Perception) {
	if s.knowledgeByObserver[p.Self.ID] == nil {
		s.knowledgeByObserver[p.Self.ID] = newRulebookActorKnowledge()
	}
	s.rulebookActorKnowledge = s.knowledgeByObserver[p.Self.ID]
	if p.Self.ID != "" {
		s.allActorNames[p.Self.ID] = p.Self.Name
		s.knownActorNames[p.Self.ID] = p.Self.Name
		s.knownActorRoles[p.Self.ID] = p.Self.Role
		if p.Self.FormID != "" {
			s.knownActorForms[p.Self.ID] = p.Self.FormID
		}
		if tier := p.Catalog.ActorPowerTier(p.Self); tier > 0 {
			s.knownActorPowerTiers[p.Self.ID] = tier
		}
		if cp := sim.CombatPowerWithCatalog(p.Catalog, p.Self); cp > 0 {
			s.knownActorCombatPowers[p.Self.ID] = cp
		}
		if len(p.Self.Skills) > 0 {
			s.knownActorSkills[p.Self.ID] = cloneIntMap(p.Self.Skills)
		}
		if strings.TrimSpace(p.Self.Personality) != "" {
			s.knownActorPersonalities[p.Self.ID] = p.Self.Personality
		}
		if len(p.Self.Hobbies) > 0 {
			s.knownActorHobbies[p.Self.ID] = normalizeStringList(p.Self.Hobbies)
		}
		guilds := make([]string, 0, len(p.Self.Guilds))
		seen := map[string]bool{}
		for _, g := range p.Self.Guilds {
			if g != "" && !seen[g] {
				seen[g] = true
				guilds = append(guilds, g)
			}
		}
		for _, f := range p.Factions {
			if f.Joined && f.ID != "" && !seen[f.ID] {
				seen[f.ID] = true
				guilds = append(guilds, f.ID)
			}
		}
		s.knownActorGuilds[p.Self.ID] = guilds
	}
	for _, nb := range p.Nearby {
		if nb.ID != "" {
			s.knownActorNames[nb.ID], s.knownActorRoles[nb.ID] = nb.Name, nb.Role
			s.knownActorForms[nb.ID], s.knownActorPowerTiers[nb.ID] = nb.FormID, nb.PowerTier
			s.knownActorCombatPowers[nb.ID] = nb.CombatPower
			s.knownActorSkills[nb.ID] = cloneIntMap(nb.Skills)
			s.knownActorPersonalities[nb.ID] = nb.Personality
			s.knownActorHobbies[nb.ID] = normalizeStringList(nb.Hobbies)
			s.knownActorGuilds[nb.ID] = append([]string(nil), nb.Guilds...)
		}
	}
}

func (s *RulebookSelector) actorNameLocked(id string) string {
	if id == "" {
		return ""
	}
	if nm := s.knownActorNames[id]; nm != "" {
		return nm
	}
	return id
}

func (s *RulebookSelector) actorRoleLocked(id string) string {
	if id == "" {
		return ""
	}
	return s.knownActorRoles[id]
}

func (s *RulebookSelector) actorHobbiesLocked(id string) []string {
	if id == "" {
		return nil
	}
	return s.knownActorHobbies[id]
}

func (s *RulebookSelector) actorPersonalityLocked(id string) string {
	if id == "" {
		return ""
	}
	return s.knownActorPersonalities[id]
}

// Select implements Selector with real causal memory-driven reactions and dynamic role-pair matching.
func (s *RulebookSelector) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	if s == nil || s.Rulebook == nil {
		return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.stats.TotalDecisions++
	delete(s.lastTriggerByActor, p.Self.ID)
	s.observePerceptionLocked(p)

	day := p.Clock.Day
	if day < 1 {
		day = int(p.Tick/sim.TicksPerDay) + 1
	}

	// Tier 0: Physiological survival & circadian rest.
	if needsSurvivalOverride(p) {
		s.stats.RulesSurvival++
		return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
	}

	// Work on existing legal process and coordinated movement before discretionary interactions.
	for _, action := range caseActions(p) {
		if id := ruleOptionID(action, options); id != "" {
			return id, nil
		}
	}
	if actorActiveParty(p) != nil {
		var partyOption string
		if planPartyAction(p, func(action sim.Action) bool {
			partyOption = ruleOptionID(action, options)
			return partyOption != ""
		}) {
			return partyOption, nil
		}
	}

	// Tier 1: Causal Reaction Rules triggered by real engine events in p.Self.Memories / p.Offers.
	if id, ok := s.tryCausalReactionLocked(p, options, day); ok {
		return id, nil
	}

	// Tier 1.5: Environmental crisis, major world-event & death-news reactions.
	if id, ok := s.tryEnvAndDeathReactionLocked(p, options, day); ok {
		return id, nil
	}

	// Tier 1.75: Ready upward realm/star breakthrough and golden-finger redemptions/simulations take precedence over routine chatter.
	if len(p.ActiveGoldenFingers) > 0 {
		var gfOpt string
		if planGoldenFingerAction(p, func(action sim.Action) bool {
			gfOpt = ruleOptionID(action, options)
			return gfOpt != ""
		}) {
			return gfOpt, nil
		}
	}
	if p.Self.Health >= 55 && p.Self.Energy >= 20 && len(p.Catalog.Transitions) > 0 {
		selfTier := p.Catalog.ActorPowerTier(p.Self)
		for _, opt := range options {
			if opt.Action.Kind != "attempt_transition" || opt.Action.TransitionID == "" {
				continue
			}
			if tr, ok := p.Catalog.Transitions[opt.Action.TransitionID]; ok && tr.SuccessFormID != "" && tr.SuccessFormID != p.Self.FormID {
				if p.Catalog.Forms[tr.SuccessFormID].PowerTier > selfTier {
					return opt.ID, nil
				}
			}
		}
	}

	// Tier 2: Proactive Role-Pair Interaction Rules ($actor1 --[behavior]--> $actor2).
	if id, ok := s.tryProactiveInteractionLocked(p, options, day); ok {
		return id, nil
	}

	// Tier 3: Generic economic/social baseline fallback.
	s.stats.RulesFallback++
	return RuleSelector{trace: s.Trace}.Select(ctx, p, options)
}

type actorProfileView struct {
	ID              string
	Name            string
	Role            string
	Tags            []string
	Hobbies         []string
	Personality     string
	Health          int
	Money           int
	HasIllicitGain  bool
	HasSurplusGoods bool
	HasFullState    bool
}

func profileFromSelf(p sim.Perception) actorProfileView {
	a := p.Self
	surplus := false
	for itemID, qty := range a.Items {
		spec, ok := p.Catalog.Items[itemID]
		if ok && spec.Tradeable && spec.OfferThreshold > 0 && qty >= spec.OfferThreshold {
			surplus = true
			break
		}
	}
	return actorProfileView{
		ID:              a.ID,
		Name:            a.Name,
		Role:            a.Role,
		Tags:            a.IdentityTags(),
		Hobbies:         normalizeStringList(a.Hobbies),
		Personality:     strings.ToLower(a.Personality),
		Health:          a.Health,
		Money:           a.Money,
		HasIllicitGain:  a.IllicitGain > 0,
		HasSurplusGoods: surplus,
		HasFullState:    true,
	}
}

func profileFromNeighbor(nb sim.Neighbor) actorProfileView {
	tags := append([]string(nil), nb.Tags...)
	if nb.Role != "" && !sim.ActorHasTag(tags, nb.Role) {
		tags = append(tags, nb.Role)
	}
	return actorProfileView{
		ID:           nb.ID,
		Name:         nb.Name,
		Role:         nb.Role,
		Tags:         tags,
		Hobbies:      normalizeStringList(nb.Hobbies),
		Personality:  strings.ToLower(nb.Personality),
		Health:       nb.Health,
		HasFullState: false,
	}
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s))
		if k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func actorHasHobby(hobbies []string, target string) bool {
	want := strings.ToLower(strings.TrimSpace(target))
	if want == "" {
		return false
	}
	for _, h := range hobbies {
		if strings.EqualFold(strings.TrimSpace(h), want) {
			return true
		}
	}
	return false
}

func sharedHobbies(a, b []string) []string {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[string]bool, len(a))
	for _, h := range a {
		k := strings.ToLower(strings.TrimSpace(h))
		if k != "" {
			set[k] = true
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, h := range b {
		k := strings.ToLower(strings.TrimSpace(h))
		if k != "" && set[k] && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func personalityMatchesTrait(lowerPersonality string, traits []string, key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" || lowerPersonality == "" {
		return false
	}
	for _, t := range traits {
		if t == k {
			return true
		}
	}
	return strings.Contains(lowerPersonality, k)
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func lookupFloatCI(m map[string]float64, key string) (float64, bool) {
	if len(m) == 0 || key == "" {
		return 0, false
	}
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return v, true
		}
	}
	return 0, false
}

func lookupNestedFloatMapCI(m map[string]map[string]float64, key string) (map[string]float64, bool) {
	if len(m) == 0 || key == "" {
		return nil, false
	}
	if sub, ok := m[key]; ok {
		return sub, true
	}
	for k, sub := range m {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return sub, true
		}
	}
	return nil, false
}

// computePersonalityReactionMultiplier calculates how an actor's personality traits
// modulate a ReactionBranch's weight (combining branch-level personality_weights and
// rulebook-level personality_stance_weights).
func computePersonalityReactionMultiplier(rb *InteractionRulebook, br ReactionBranch, personality string) (float64, []string) {
	lower := strings.ToLower(strings.TrimSpace(personality))
	if lower == "" {
		return 1.0, nil
	}
	traits := ExtractPersonalityTraits(lower)
	mult := 1.0
	var reasons []string

	if len(br.PersonalityWeights) > 0 {
		keys := make([]string, 0, len(br.PersonalityWeights))
		for k := range br.PersonalityWeights {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := br.PersonalityWeights[k]
			if v <= 0 || math.Abs(v-1.0) <= 1e-6 {
				continue
			}
			if personalityMatchesTrait(lower, traits, k) {
				cv := clampFloat(v, 0.2, 5.0)
				mult *= cv
				reasons = append(reasons, fmt.Sprintf("性格分支:%s×%.2f", strings.ToLower(strings.TrimSpace(k)), cv))
			}
		}
	}

	if rb != nil && len(rb.PersonalityStanceWeights) > 0 && br.Stance != "" {
		keys := make([]string, 0, len(rb.PersonalityStanceWeights))
		for k := range rb.PersonalityStanceWeights {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !personalityMatchesTrait(lower, traits, k) {
				continue
			}
			stanceMap := rb.PersonalityStanceWeights[k]
			if v := stanceMap[br.Stance]; v > 0 && math.Abs(v-1.0) > 1e-6 {
				cv := clampFloat(v, 0.2, 5.0)
				mult *= cv
				reasons = append(reasons, fmt.Sprintf("性格立场:%s→%s×%.2f", strings.ToLower(strings.TrimSpace(k)), br.Stance, cv))
			}
		}
	}

	return clampFloat(mult, 0.15, 12.0), reasons
}

// computeHobbyBehaviorMultiplier calculates how Actor1's hobbies (and shared hobbies
// with Actor2) modulate a proactive InteractionRule's selection weight.
func computeHobbyBehaviorMultiplier(rb *InteractionRulebook, rule *InteractionRule, selfHobbies, targetHobbies []string) (float64, []string, []string) {
	personalMult := 1.0
	var reasons []string
	normSelf := normalizeStringList(selfHobbies)
	shared := sharedHobbies(normSelf, targetHobbies)

	for _, h := range normSelf {
		if v, ok := lookupFloatCI(rule.HobbyWeights, h); ok && v > 0 && math.Abs(v-1.0) > 1e-6 {
			cv := clampFloat(v, 0.2, 5.0)
			personalMult *= cv
			reasons = append(reasons, fmt.Sprintf("爱好规则:%s×%.2f", h, cv))
		}
		if rb != nil && rule.Behavior.EngineKind != "" {
			if actionMap, ok := lookupNestedFloatMapCI(rb.HobbyActionWeights, h); ok {
				if v := actionMap[rule.Behavior.EngineKind]; v > 0 && math.Abs(v-1.0) > 1e-6 {
					cv := clampFloat(v, 0.2, 5.0)
					personalMult *= cv
					reasons = append(reasons, fmt.Sprintf("爱好行为:%s→%s×%.2f", h, rule.Behavior.EngineKind, cv))
				}
			}
		}
	}

	mult := clampFloat(personalMult, 0.15, 8.0)
	if len(shared) > 0 && rule.SharedHobbyMultiplier > 0 && math.Abs(rule.SharedHobbyMultiplier-1.0) > 1e-6 {
		cv := clampFloat(rule.SharedHobbyMultiplier, 0.2, 5.0)
		mult *= cv
		reasons = append(reasons, fmt.Sprintf("同好加成:%s×%.2f", strings.Join(shared, "+"), cv))
	}

	return clampFloat(mult, 0.10, 16.0), shared, reasons
}

func neutralEncounterProfile() EncounterProfile {
	return EncounterProfile{
		ProactiveRate:    1.0,
		TargetAttraction: 1.0,
		DirectEngage:     1.0,
		WitnessEngage:    1.0,
	}
}

func lookupEncounterProfileCI(m map[string]EncounterProfile, key string) (EncounterProfile, bool) {
	if len(m) == 0 || strings.TrimSpace(key) == "" {
		return EncounterProfile{}, false
	}
	if v, ok := m[key]; ok {
		return v, true
	}
	want := strings.ToLower(strings.TrimSpace(key))
	for k, v := range m {
		if strings.ToLower(strings.TrimSpace(k)) == want {
			return v, true
		}
	}
	return EncounterProfile{}, false
}

// computeProfessionEncounterProfile returns the structural daily social contact baseline
// for a profession (e.g., bazaar merchant / patrolling enforcer vs rural farmer / reclusive alchemist),
// independent of the individual actor's personality.
func computeProfessionEncounterProfile(rb *InteractionRulebook, role string) (EncounterProfile, bool) {
	out := neutralEncounterProfile()
	if rb == nil || len(rb.ProfessionEncounterModifiers) == 0 || strings.TrimSpace(role) == "" {
		return out, false
	}
	mod, ok := lookupEncounterProfileCI(rb.ProfessionEncounterModifiers, role)
	if !ok {
		return out, false
	}
	if mod.ProactiveRate > 0 {
		out.ProactiveRate = clampFloat(mod.ProactiveRate, 0.35, 2.20)
	}
	if mod.TargetAttraction > 0 {
		out.TargetAttraction = clampFloat(mod.TargetAttraction, 0.35, 2.20)
	}
	if mod.DirectEngage > 0 {
		out.DirectEngage = clampFloat(mod.DirectEngage, 0.40, 2.00)
	}
	if mod.WitnessEngage > 0 {
		out.WitnessEngage = clampFloat(mod.WitnessEngage, 0.30, 2.20)
	}
	return out, true
}

// computeVenueExposureFactor derives a genre-agnostic structural foot-traffic multiplier
// from whether the actor's current location/venue has any of the pack's public ForumFeatures.
func computeVenueExposureFactor(rb *InteractionRulebook, p sim.Perception) float64 {
	if rb == nil || len(rb.Environment.ForumFeatures) == 0 || p.Self.Location == "" {
		return 1.0
	}
	if districtMatchesFeatures(p, rb.Environment.ForumFeatures) {
		return 1.18
	}
	return 0.80
}

// computePersonalityEncounterProfile combines an actor's personality traits into a
// moderate within-profession 4-channel encounter modifier using sqrt-damped geometric
// aggregation so multi-trait personalities modulate rather than overwrite profession baselines.
func computePersonalityEncounterProfile(rb *InteractionRulebook, personality string) (EncounterProfile, []string) {
	out := neutralEncounterProfile()
	if rb == nil || len(rb.PersonalityEncounterModifiers) == 0 {
		return out, nil
	}
	lower := strings.ToLower(strings.TrimSpace(personality))
	if lower == "" {
		return out, nil
	}
	traits := ExtractPersonalityTraits(lower)
	keys := make([]string, 0, len(rb.PersonalityEncounterModifiers))
	for k := range rb.PersonalityEncounterModifiers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var (
		proLog, attrLog, dirLog, witLog float64
		proN, attrN, dirN, witN         int
		reasons                         []string
	)
	for _, k := range keys {
		if !personalityMatchesTrait(lower, traits, k) {
			continue
		}
		mod := rb.PersonalityEncounterModifiers[k]
		nk := strings.ToLower(strings.TrimSpace(k))
		if mod.ProactiveRate > 0 && math.Abs(mod.ProactiveRate-1.0) > 1e-6 {
			cv := clampFloat(mod.ProactiveRate, 0.45, 2.0)
			proLog += math.Log(cv)
			proN++
			reasons = append(reasons, fmt.Sprintf("性格遭遇(主动):%s×%.2f", nk, cv))
		}
		if mod.TargetAttraction > 0 && math.Abs(mod.TargetAttraction-1.0) > 1e-6 {
			cv := clampFloat(mod.TargetAttraction, 0.45, 2.0)
			attrLog += math.Log(cv)
			attrN++
			reasons = append(reasons, fmt.Sprintf("性格遭遇(注目):%s×%.2f", nk, cv))
		}
		if mod.DirectEngage > 0 && math.Abs(mod.DirectEngage-1.0) > 1e-6 {
			cv := clampFloat(mod.DirectEngage, 0.45, 2.0)
			dirLog += math.Log(cv)
			dirN++
			reasons = append(reasons, fmt.Sprintf("性格遭遇(当事):%s×%.2f", nk, cv))
		}
		if mod.WitnessEngage > 0 && math.Abs(mod.WitnessEngage-1.0) > 1e-6 {
			cv := clampFloat(mod.WitnessEngage, 0.40, 2.0)
			witLog += math.Log(cv)
			witN++
			reasons = append(reasons, fmt.Sprintf("性格遭遇(旁观):%s×%.2f", nk, cv))
		}
	}

	if proN > 0 {
		out.ProactiveRate = clampFloat(math.Exp(proLog/math.Sqrt(float64(proN))), 0.52, 1.65)
	}
	if attrN > 0 {
		out.TargetAttraction = clampFloat(math.Exp(attrLog/math.Sqrt(float64(attrN))), 0.52, 1.65)
	}
	if dirN > 0 {
		out.DirectEngage = clampFloat(math.Exp(dirLog/math.Sqrt(float64(dirN))), 0.50, 1.60)
	}
	if witN > 0 {
		out.WitnessEngage = clampFloat(math.Exp(witLog/math.Sqrt(float64(witN))), 0.45, 1.65)
	}
	return out, reasons
}

// effectiveEncounterBreakdown separates the three orthogonal encounter factors for full auditability.
type effectiveEncounterBreakdown struct {
	Effective   EncounterProfile
	Profession  EncounterProfile
	Personality EncounterProfile
	VenueFactor float64
	Active      bool
}

func computeEffectiveEncounterProfile(rb *InteractionRulebook, role, personality string, venueFactor float64) effectiveEncounterBreakdown {
	hasProf := rb != nil && len(rb.ProfessionEncounterModifiers) > 0
	hasPers := rb != nil && len(rb.PersonalityEncounterModifiers) > 0
	if !hasProf && !hasPers {
		n := neutralEncounterProfile()
		return effectiveEncounterBreakdown{
			Effective:   n,
			Profession:  n,
			Personality: n,
			VenueFactor: 1.0,
			Active:      false,
		}
	}
	profEnc, _ := computeProfessionEncounterProfile(rb, role)
	persEnc, _ := computePersonalityEncounterProfile(rb, personality)
	vf := venueFactor
	if vf <= 0 {
		vf = 1.0
	}
	eff := EncounterProfile{
		ProactiveRate:    clampFloat(profEnc.ProactiveRate*vf*persEnc.ProactiveRate, 0.22, 3.00),
		TargetAttraction: clampFloat(profEnc.TargetAttraction*vf*persEnc.TargetAttraction, 0.22, 3.20),
		DirectEngage:     clampFloat(profEnc.DirectEngage*persEnc.DirectEngage, 0.25, 2.50),
		WitnessEngage:    clampFloat(profEnc.WitnessEngage*vf*persEnc.WitnessEngage, 0.15, 2.60),
	}
	return effectiveEncounterBreakdown{
		Effective:   eff,
		Profession:  profEnc,
		Personality: persEnc,
		VenueFactor: vf,
		Active:      true,
	}
}

// computeRelationStanceMultiplier modulates causal reaction stance weights using runtime-observed
// mentorship bonds, sworn alliances, shared guilds, trust, blood feuds, and protective witness loyalty.
func computeRelationStanceMultiplier(stance string, trust int, sharedGuild, isMentorBond, isSwornAlly, isFeudEnemy, defendingBond bool) (float64, string) {
	if defendingBond {
		switch stance {
		case "强硬反制":
			return 1.75, "护短卫盟:强硬反制×1.75"
		case "秉公处置":
			return 1.45, "护短卫盟:秉公处置×1.45"
		case "避险止损":
			return 0.55, "护短卫盟:避险止损×0.55"
		case "趁机谋利":
			return 0.60, "护短卫盟:趁机谋利×0.60"
		}
		return 1.0, ""
	}
	if isFeudEnemy || trust <= -3 {
		switch stance {
		case "强硬反制":
			return 1.55, "宿怨敌对:强硬反制×1.55"
		case "暗中周旋":
			return 1.25, "宿怨敌对:暗中周旋×1.25"
		case "积极呼应":
			return 0.40, "宿怨敌对:积极呼应×0.40"
		case "借力拉拢":
			return 0.45, "宿怨敌对:借力拉拢×0.45"
		case "谈判妥协":
			return 0.65, "宿怨敌对:谈判妥协×0.65"
		}
		return 1.0, ""
	}
	if isMentorBond || isSwornAlly || trust >= 4 || (sharedGuild && trust >= 2) {
		switch stance {
		case "积极呼应":
			return 1.55, "师徒/盟友亲厚:积极呼应×1.55"
		case "借力拉拢":
			return 1.40, "师徒/盟友亲厚:借力拉拢×1.40"
		case "谈判妥协":
			return 1.30, "师徒/盟友亲厚:谈判妥协×1.30"
		case "强硬反制":
			return 0.35, "师徒/盟友亲厚:强硬反制×0.35"
		case "趁机谋利":
			return 0.45, "师徒/盟友亲厚:趁机谋利×0.45"
		}
	}
	return 1.0, ""
}

func inferRuntimeSocialBond(a sim.Actor, targetID string, targetGuilds []string, grudgeTargets []string) (trust int, sharedGuild, isMentorBond, isSwornAlly, isFeudEnemy bool) {
	if targetID == "" || targetID == a.ID {
		return
	}
	if a.Trust != nil {
		trust = a.Trust[targetID]
	}
	for _, g1 := range a.Guilds {
		for _, g2 := range targetGuilds {
			if g1 != "" && g1 == g2 {
				sharedGuild = true
				break
			}
		}
		if sharedGuild {
			break
		}
	}
	for _, g := range grudgeTargets {
		if g == targetID {
			isFeudEnemy = true
			break
		}
	}
	for _, m := range a.Memories {
		pairMatch := (m.Source == a.ID && m.TargetID == targetID) || (m.Source == targetID && m.TargetID == a.ID)
		if !pairMatch {
			continue
		}
		switch m.Kind {
		case "mentorship_formed", "knowledge_taught":
			isMentorBond = true
		case "bond_sworn":
			isSwornAlly = true
		case "grudge_formed":
			isFeudEnemy = true
		}
	}
	return
}

// computePowerGapStanceMultiplier modulates causal reaction stance weights based on the
// power-tier / combat-power disparity between responder (self) and trigger counterpart (other).
// Strictly genre-agnostic: works for cultivation realms, military ranks, or cybernetic tiers.
func computePowerGapStanceMultiplier(stance string, selfTier, otherTier, selfPower, otherPower int) (float64, string) {
	gap := 0
	if selfTier > 0 && otherTier > 0 {
		gap = selfTier - otherTier
	} else if selfPower > 0 && otherPower > 0 {
		ratio := float64(selfPower) / float64(otherPower)
		switch {
		case ratio <= 0.45:
			gap = -2
		case ratio <= 0.72:
			gap = -1
		case ratio >= 2.20:
			gap = 2
		case ratio >= 1.40:
			gap = 1
		}
	}
	if gap == 0 {
		return 1.0, ""
	}
	mult := 1.0
	if gap < 0 {
		// Facing a stronger counterpart: suppress reckless confrontation, boost caution/compromise/covert maneuvering.
		k := -gap
		if k > 3 {
			k = 3
		}
		switch stance {
		case "强硬反制":
			mult = []float64{1.0, 0.46, 0.24, 0.14}[k]
		case "趁机谋利":
			mult = []float64{1.0, 0.58, 0.36, 0.25}[k]
		case "避险止损":
			mult = []float64{1.0, 1.55, 2.05, 2.45}[k]
		case "谈判妥协":
			mult = []float64{1.0, 1.32, 1.55, 1.72}[k]
		case "暗中周旋":
			mult = []float64{1.0, 1.24, 1.44, 1.58}[k]
		case "借力拉拢":
			mult = []float64{1.0, 1.18, 1.30, 1.38}[k]
		}
	} else {
		// Facing a weaker counterpart: boost assertive enforcement/pressure, suppress cowering.
		k := gap
		if k > 3 {
			k = 3
		}
		switch stance {
		case "强硬反制":
			mult = []float64{1.0, 1.42, 1.75, 1.95}[k]
		case "秉公处置":
			mult = []float64{1.0, 1.28, 1.48, 1.62}[k]
		case "趁机谋利":
			mult = []float64{1.0, 1.22, 1.38, 1.48}[k]
		case "避险止损":
			mult = []float64{1.0, 0.56, 0.36, 0.25}[k]
		}
	}
	if math.Abs(mult-1.0) <= 1e-6 {
		return 1.0, ""
	}
	return mult, fmt.Sprintf("实力差(%+d):%s×%.2f", gap, stance, mult)
}

// computePowerGapProactiveMultiplier discourages suicidal hostile actions against much stronger
// targets (unless personality is reckless/provocative) and rewards senior->junior mentorship.
func computePowerGapProactiveMultiplier(actionKind string, selfTier, otherTier, selfPower, otherPower int, personality string) (float64, string) {
	gap := 0
	if selfTier > 0 && otherTier > 0 {
		gap = selfTier - otherTier
	} else if selfPower > 0 && otherPower > 0 {
		ratio := float64(selfPower) / float64(otherPower)
		switch {
		case ratio <= 0.45:
			gap = -2
		case ratio <= 0.72:
			gap = -1
		case ratio >= 2.20:
			gap = 2
		case ratio >= 1.40:
			gap = 1
		}
	}
	if gap == 0 {
		return 1.0, ""
	}
	switch actionKind {
	case "attack", "raid", "steal", "extort", "avenge_feud":
		if gap < 0 {
			k := -gap
			if k > 3 {
				k = 3
			}
			mult := []float64{1.0, 0.38, 0.16, 0.08}[k]
			lower := strings.ToLower(personality)
			if strings.Contains(lower, "reckless") || strings.Contains(lower, "provocative") {
				mult = math.Min(0.75, mult*1.6)
			}
			return mult, fmt.Sprintf("越级威慑(%+d):%s×%.2f", gap, actionKind, mult)
		}
		if gap > 0 {
			mult := clampFloat(1.0+0.18*float64(gap), 1.0, 1.45)
			return mult, fmt.Sprintf("实力压制(%+d):%s×%.2f", gap, actionKind, mult)
		}
	case "teach_knowledge":
		if gap > 0 {
			mult := clampFloat(1.0+0.25*float64(gap), 1.0, 1.65)
			return mult, fmt.Sprintf("前辈传道(%+d):×%.2f", gap, mult)
		}
	}
	return 1.0, ""
}

// computeSkillProactiveMultiplier boosts proactive actions matching the actor's independent skills.
func computeSkillProactiveMultiplier(cat sim.Catalog, skills map[string]int, actionKind string) (float64, string) {
	if len(cat.Skills) == 0 || len(skills) == 0 || actionKind == "" {
		return 1.0, ""
	}
	bestMult := 1.0
	bestName := ""
	bestLv := 0
	keys := make([]string, 0, len(skills))
	for k := range skills {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, sid := range keys {
		lv := skills[sid]
		if lv <= 0 {
			continue
		}
		spec, ok := cat.Skills[sid]
		if !ok {
			continue
		}
		matched := false
		for _, aff := range spec.ActionAffinities {
			if aff == actionKind {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		m := clampFloat(1.0+0.12*float64(lv), 1.0, 1.72)
		if m > bestMult {
			bestMult = m
			bestName = spec.Name
			if bestName == "" {
				bestName = sid
			}
			bestLv = lv
		}
	}
	if bestMult <= 1.0+1e-6 {
		return 1.0, ""
	}
	return bestMult, fmt.Sprintf("技艺加成:%s(Lv.%d)→%s×%.2f", bestName, bestLv, actionKind, bestMult)
}

func isHighSeverityTrigger(kind string) bool {
	switch kind {
	case "attacked", "raided", "stolen", "arrested", "extorted", "feud_avenged", "wrongful_conviction":
		return true
	default:
		return false
	}
}

func matchesRole(m RoleMatcher, v actorProfileView) bool {
	if len(m.Roles) > 0 && !containsWildcard(m.Roles) {
		matched := false
		for _, r := range m.Roles {
			if r == v.Role || sim.ActorHasTag(v.Tags, r) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(m.AnyTags) > 0 && !containsWildcard(m.AnyTags) {
		matched := false
		for _, t := range m.AnyTags {
			if t == v.Role || sim.ActorHasTag(v.Tags, t) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, ex := range m.ExcludeTags {
		if ex == v.Role || sim.ActorHasTag(v.Tags, ex) {
			return false
		}
	}
	if len(m.AnyHobbies) > 0 && !containsWildcard(m.AnyHobbies) {
		matched := false
		for _, h := range m.AnyHobbies {
			if actorHasHobby(v.Hobbies, h) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, ex := range m.ExcludeHobbies {
		if actorHasHobby(v.Hobbies, ex) {
			return false
		}
	}
	if len(m.PersonalityKeywords) > 0 && v.HasFullState {
		matched := false
		for _, kw := range m.PersonalityKeywords {
			if strings.Contains(v.Personality, strings.ToLower(strings.TrimSpace(kw))) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if m.MinHealth != nil && v.Health > 0 && v.Health < *m.MinHealth {
		return false
	}
	if m.MaxHealth != nil && v.Health > 0 && v.Health > *m.MaxHealth {
		return false
	}
	if m.IsWounded != nil && v.Health > 0 {
		wounded := v.Health < 85
		if wounded != *m.IsWounded {
			return false
		}
	}
	if v.HasFullState {
		if m.MinMoney != nil && v.Money < *m.MinMoney {
			return false
		}
		if m.MaxMoney != nil && v.Money > *m.MaxMoney {
			return false
		}
		if m.HasIllicitGain != nil && v.HasIllicitGain != *m.HasIllicitGain {
			return false
		}
		if m.HasSurplusGoods != nil && v.HasSurplusGoods != *m.HasSurplusGoods {
			return false
		}
	}
	return true
}

func containsWildcard(list []string) bool {
	for _, s := range list {
		if s == "*" || strings.EqualFold(s, "any") {
			return true
		}
	}
	return false
}

func (s *RulebookSelector) matchesRelationLocked(p sim.Perception, rel RelationPredicate, otherID string, coLocated bool) bool {
	if rel.CoLocated != nil && *rel.CoLocated != coLocated {
		return false
	}
	trust := p.Self.Trust[otherID]
	if rel.MinTrust != nil && trust < *rel.MinTrust {
		return false
	}
	if rel.MaxTrust != nil && trust > *rel.MaxTrust {
		return false
	}
	fRel := strings.ToLower(strings.TrimSpace(rel.FactionRelation))
	if fRel == "same" || fRel == "different" {
		shared := s.actorsShareFactionLocked(p.Self.ID, otherID)
		if fRel == "same" && !shared {
			return false
		}
		if fRel == "different" && shared {
			return false
		}
	}
	if rel.SharedHobby != nil {
		hasShared := len(sharedHobbies(p.Self.Hobbies, s.actorHobbiesLocked(otherID))) > 0
		if hasShared != *rel.SharedHobby {
			return false
		}
	}
	return true
}

func (s *RulebookSelector) actorsShareFactionLocked(aID, bID string) bool {
	ga := s.knownActorGuilds[aID]
	gb := s.knownActorGuilds[bID]
	if len(ga) == 0 || len(gb) == 0 {
		return false
	}
	set := make(map[string]bool, len(ga))
	for _, g := range ga {
		set[g] = true
	}
	for _, g := range gb {
		if set[g] {
			return true
		}
	}
	return false
}

func districtMatchesFeatures(p sim.Perception, features []string) bool {
	if len(features) == 0 {
		return true
	}
	for _, f := range features {
		if f == "" || f == "*" {
			return true
		}
		if p.Map.DistrictHasFeature(p.Self.Location, f) {
			return true
		}
		if p.Self.VenueID != "" && p.Map.VenueOrDistrictHasFeature(p.Self.VenueID, p.Self.Location, f) {
			return true
		}
	}
	return false
}

func (s *RulebookSelector) tryCausalReactionLocked(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	if s.actorNextRxnTick[a.ID] > p.Tick {
		return "", false
	}
	if len(s.Rulebook.ReactionRules) == 0 || len(a.Memories) == 0 {
		return "", false
	}

	selfProf := profileFromSelf(p)
	venueFactor := computeVenueExposureFactor(s.Rulebook, p)
	selfEncBreak := computeEffectiveEncounterProfile(s.Rulebook, a.Role, a.Personality, venueFactor)
	selfEnc := selfEncBreak.Effective
	hasEncounterMods := selfEncBreak.Active
	nearbyByID := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		nearbyByID[nb.ID] = nb
	}

	// Scan recent causal memories newest-first (within last 48 ticks = half-day window).
	const reactionWindowTicks = 48
	for i := len(a.Memories) - 1; i >= 0; i-- {
		mem := a.Memories[i]
		if p.Tick-mem.Tick > reactionWindowTicks {
			break
		}
		if mem.EventID == 0 || mem.Source == "" || mem.Source == "world" || mem.Source == a.ID {
			continue
		}
		evKey := fmt.Sprintf("%s:%d", a.ID, mem.EventID)
		if s.reactedEvents[evKey] {
			continue
		}

		// Determine responder position in this causal event: actor2 (direct target) or witness (bystander).
		pos := ""
		actor2ID := mem.TargetID
		if mem.TargetID == a.ID {
			pos = "actor2"
		} else if mem.TargetID != "" && mem.TargetID != mem.Source {
			pos = "witness"
		} else if mem.Observation != nil && mem.Observation.ObjectID != "" {
			actor2ID = mem.Observation.ObjectID
			if actor2ID == a.ID {
				pos = "actor2"
			} else {
				pos = "witness"
			}
		} else {
			// Broadcast / undirected event observed by a bystander.
			pos = "witness"
		}

		actor1Prof := actorProfileView{
			ID:          mem.Source,
			Name:        s.actorNameLocked(mem.Source),
			Role:        mem.SourceRole,
			Tags:        mem.SourceTags,
			Hobbies:     s.actorHobbiesLocked(mem.Source),
			Personality: strings.ToLower(s.actorPersonalityLocked(mem.Source)),
		}
		if nb, ok := nearbyByID[mem.Source]; ok {
			actor1Prof = profileFromNeighbor(nb)
		} else if actor1Prof.Role == "" {
			actor1Prof.Role = s.actorRoleLocked(mem.Source)
		}

		var actor2Prof actorProfileView
		if pos == "actor2" {
			actor2Prof = selfProf
		} else if nb, ok := nearbyByID[actor2ID]; ok {
			actor2Prof = profileFromNeighbor(nb)
		} else {
			actor2Prof = actorProfileView{
				ID:          actor2ID,
				Name:        s.actorNameLocked(actor2ID),
				Role:        s.actorRoleLocked(actor2ID),
				Hobbies:     s.actorHobbiesLocked(actor2ID),
				Personality: strings.ToLower(s.actorPersonalityLocked(actor2ID)),
			}
		}

		_, actor1Here := nearbyByID[mem.Source]

		// Evaluate ALL matching CausalReactionRules for this real engine outcome.
		var matchingRules []*CausalReactionRule
		for idx := range s.Rulebook.ReactionRules {
			rule := &s.Rulebook.ReactionRules[idx]
			if rule.ResponderPosition != pos {
				continue
			}
			if !eventKindMatches(rule.TriggerEventKinds, mem.Kind) {
				continue
			}
			if !matchesRole(rule.ResponderMatch, selfProf) {
				continue
			}
			if !matchesRole(rule.Actor1Match, actor1Prof) {
				continue
			}
			if actor2ID != "" && !matchesRole(rule.Actor2Match, actor2Prof) {
				continue
			}
			if !s.matchesRelationLocked(p, rule.RelationToActor1, mem.Source, actor1Here) {
				continue
			}
			matchingRules = append(matchingRules, rule)
		}
		if len(matchingRules) == 0 {
			continue
		}

		// Pool ALL executable branches across ALL matching rules into one unified
		// personality-weighted competition (prevents a generic tag from starving other domains).
		type pooledReactionCand struct {
			rule            *CausalReactionRule
			branch          ReactionBranch
			opt             *Option
			baseWeight      int
			effectiveWeight int
			sampleWeight    int
			mult            float64
			powerMult       float64
			relMult         float64
			powerTierGap    int
			targetForm      string
			targetTier      int
			reasons         []string
		}
		selfTier := p.Catalog.ActorPowerTier(a)
		selfPower := sim.CombatPowerWithCatalog(p.Catalog, a)
		otherID := mem.Source
		if otherID == a.ID {
			otherID = actor2ID
		}
		otherTier := s.knownActorPowerTiers[otherID]
		otherPower := s.knownActorCombatPowers[otherID]
		otherForm := s.knownActorForms[otherID]
		relTrust, relGuild, relMentor, relSworn, relFeud := inferRuntimeSocialBond(a, otherID, s.knownActorGuilds[otherID], p.ActiveGrudgeTargets)
		defendingBond := false
		if pos == "witness" && actor2ID != "" && actor2ID != a.ID && actor2ID != mem.Source {
			switch mem.Kind {
			case "attacked", "raided", "stolen", "wrongful_conviction", "extorted":
				vTrust, vGuild, vMentor, vSworn, _ := inferRuntimeSocialBond(a, actor2ID, s.knownActorGuilds[actor2ID], p.ActiveGrudgeTargets)
				if vMentor || vSworn || vTrust >= 3 || vGuild {
					defendingBond = true
				}
			}
		}
		firedRxnBefore := func(rid string) bool {
			t, ok := s.firedReaction[rid]
			return ok && t < p.Tick
		}
		dedup := map[string]pooledReactionCand{}
		for _, rule := range matchingRules {
			var eligible []ReactionBranch
			for _, br := range rule.Branches {
				if matchesRole(br.WhenResponder, selfProf) {
					eligible = append(eligible, br)
				}
			}
			if len(eligible) == 0 {
				eligible = append(eligible, rule.Branches...)
			}
			spec := ruleSpecificity(rule.Actor1Match) + ruleSpecificity(rule.ResponderMatch)
			specBonus := 1.0 + math.Min(0.36, 0.06*float64(spec))
			unfiredBonus := 1.0
			if !firedRxnBefore(rule.ID) {
				unfiredBonus = 1.15
			}
			for _, br := range eligible {
				targetID := resolveTargetBinding(p, br.Behavior.TargetBinding, mem.Source, actor2ID, nearbyByID)
				opt := matchGenericBehavior(p, options, br.Behavior, targetID)
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
				base := br.Weight
				if base <= 0 {
					base = 10
				}
				mult, reasons := computePersonalityReactionMultiplier(s.Rulebook, br, selfProf.Personality)
				pMult, pReason := computePowerGapStanceMultiplier(br.Stance, selfTier, otherTier, selfPower, otherPower)
				if pReason != "" {
					mult *= pMult
					reasons = append(reasons, pReason)
				}
				rMult, rReason := computeRelationStanceMultiplier(br.Stance, relTrust, relGuild, relMentor, relSworn, relFeud, defendingBond)
				if rReason != "" {
					mult *= rMult
					reasons = append(reasons, rReason)
				}
				tierGap := 0
				if selfTier > 0 && otherTier > 0 {
					tierGap = selfTier - otherTier
				}
				eff := int(math.Round(float64(base) * mult))
				if eff < 1 {
					eff = 1
				}
				sampleW := int(math.Round(float64(eff) * specBonus * unfiredBonus))
				if sampleW < 1 {
					sampleW = 1
				}
				cand := pooledReactionCand{
					rule:            rule,
					branch:          br,
					opt:             opt,
					baseWeight:      base,
					effectiveWeight: eff,
					sampleWeight:    sampleW,
					mult:            mult,
					powerMult:       pMult,
					relMult:         rMult,
					powerTierGap:    tierGap,
					targetForm:      otherForm,
					targetTier:      otherTier,
					reasons:         reasons,
				}
				dedupKey := br.Stance + "|" + opt.Action.Kind + "|" + opt.Action.TargetID
				prev, exists := dedup[dedupKey]
				if !exists || cand.sampleWeight > prev.sampleWeight || (cand.sampleWeight == prev.sampleWeight && (cand.rule.ID < prev.rule.ID || (cand.rule.ID == prev.rule.ID && cand.branch.ID < prev.branch.ID))) {
					dedup[dedupKey] = cand
				}
			}
		}
		if len(dedup) == 0 {
			continue
		}

		// Personality Encounter Gate: some personalities stay out of others' affairs or brush off
		// unsolicited social nudges, while meddlesome/provocative personalities get drawn in far more often.
		engageRate := selfEnc.DirectEngage
		if pos == "witness" {
			engageRate = selfEnc.WitnessEngage
		}
		if hasEncounterMods {
			var engageProb float64
			if pos == "witness" {
				engageProb = clampFloat(0.55*selfEnc.WitnessEngage, 0.08, 0.98)
			} else if isHighSeverityTrigger(mem.Kind) {
				engageProb = clampFloat(0.94*selfEnc.DirectEngage, 0.55, 0.99)
			} else {
				engageProb = clampFloat(0.72*selfEnc.DirectEngage, 0.20, 0.98)
			}
			gateHash := fnvHash(fmt.Sprintf("%s|%d|%s|%d|%s|encounter_gate", s.Rulebook.WorldID, s.Seed, a.ID, mem.EventID, pos))
			roll := float64(gateHash%10000) / 10000.0
			if roll >= engageProb {
				s.reactedEvents[evKey] = true
				if pos == "witness" {
					s.stats.SuppressedWitnessReactions++
				} else {
					s.stats.SuppressedDirectReactions++
				}
				s.stats.ByActorSuppressedReactions[a.ID]++
				continue
			}
		}

		pool := make([]pooledReactionCand, 0, len(dedup))
		for _, c := range dedup {
			pool = append(pool, c)
		}
		sort.SliceStable(pool, func(i, j int) bool {
			if pool[i].rule.ID != pool[j].rule.ID {
				return pool[i].rule.ID < pool[j].rule.ID
			}
			if pool[i].branch.ID != pool[j].branch.ID {
				return pool[i].branch.ID < pool[j].branch.ID
			}
			return pool[i].opt.ID < pool[j].opt.ID
		})

		totalW := 0
		for _, c := range pool {
			totalW += c.sampleWeight
		}
		pickHash := fnvHash(fmt.Sprintf("%s|%d|%s|%d|pooled_rxn", s.Rulebook.WorldID, s.Seed, a.ID, mem.EventID))
		pick := int(pickHash % uint64(totalW))
		chosen := pool[0]
		acc := 0
		for _, c := range pool {
			acc += c.sampleWeight
			if pick < acc {
				chosen = c
				break
			}
		}

		rule := chosen.rule
		br := chosen.branch
		opt := chosen.opt

		// Commit reaction!
		s.reactedEvents[evKey] = true
		rxnCD := int64(6)
		if hasEncounterMods {
			rxnCD = int64(math.Round(6.0 / clampFloat(engageRate, 0.40, 2.0)))
			if rxnCD < 3 {
				rxnCD = 3
			}
			if rxnCD > 15 {
				rxnCD = 15
			}
		}
		s.actorNextRxnTick[a.ID] = p.Tick + rxnCD
		if prev, ok := s.firedReaction[rule.ID]; !ok || p.Tick < prev {
			s.firedReaction[rule.ID] = p.Tick
		}
		s.stats.ReactionDecisions++
		if pos == "actor2" {
			s.stats.Actor2Reactions++
			s.stats.ByActorDirectReactions[a.ID]++
		} else {
			s.stats.WitnessReactions++
			s.stats.ByActorWitnessReactions[a.ID]++
		}
		if len(chosen.reasons) > 0 {
			s.stats.PersonalityWeightedReactions++
		}
		if math.Abs(chosen.powerMult-1.0) > 1e-6 {
			s.stats.PowerModulatedReactions++
		}
		if math.Abs(chosen.relMult-1.0) > 1e-6 {
			s.stats.RelationModulatedReactions++
		}
		if br.Stance != "" {
			s.stats.ByStance[br.Stance]++
			for _, tr := range ExtractPersonalityTraits(a.Personality) {
				incNestedInt(s.stats.ByPersonalityStance, tr, br.Stance)
			}
		}
		s.stats.ByTriggerKind[mem.Kind]++
		s.stats.ByExecutedKind[opt.Action.Kind]++
		s.stats.ByRuleCount[rule.ID]++
		s.recordPairLocked(rule.ID, mem.Source, a.ID, p.Tick)

		rendered := renderCausalTemplate(
			br.Behavior.SummaryTemplate,
			actor1Prof.Name, actor2Prof.Name, a.Name,
			s.locNameLocked(a.Location),
			s.itemNameLocked(opt.Action.Item),
			mem.Kind,
		)
		s.events = append(s.events, RulebookDecisionEvent{
			Tick:                 p.Tick,
			Day:                  day,
			Clock:                p.Clock.Label,
			Kind:                 "reaction",
			RuleID:               rule.ID,
			RuleName:             rule.Name,
			Category:             rule.Domain,
			BranchID:             br.ID,
			Stance:               br.Stance,
			BranchTitle:          br.Title,
			ResponderPosition:    pos,
			TriggerEventID:       mem.EventID,
			TriggerKind:          mem.Kind,
			Actor1ID:             actor1Prof.ID,
			Actor1Name:           actor1Prof.Name,
			Actor1Role:           actor1Prof.Role,
			Actor2ID:             actor2Prof.ID,
			Actor2Name:           actor2Prof.Name,
			Actor2Role:           actor2Prof.Role,
			ResponderID:          a.ID,
			ResponderName:        a.Name,
			ResponderRole:        a.Role,
			ResponderForm:        a.FormID,
			ResponderPowerTier:   selfTier,
			TargetForm:           chosen.targetForm,
			TargetPowerTier:      chosen.targetTier,
			PowerTierGap:         chosen.powerTierGap,
			PowerMultiplier:      math.Round(chosen.powerMult*1000) / 1000,
			RelationMultiplier:   math.Round(chosen.relMult*1000) / 1000,
			ResponderPersonality: a.Personality,
			ResponderHobbies:     append([]string(nil), selfProf.Hobbies...),
			BaseWeight:           chosen.baseWeight,
			EffectiveWeight:      chosen.effectiveWeight,
			WeightMultiplier:     math.Round(chosen.mult*1000) / 1000,
			EncounterRate:        math.Round(engageRate*1000) / 1000,
			ProfessionEncounterRate: math.Round(func() float64 {
				if pos == "witness" {
					return selfEncBreak.Profession.WitnessEngage
				}
				return selfEncBreak.Profession.DirectEngage
			}()*1000) / 1000,
			VenueEncounterRate: math.Round(selfEncBreak.VenueFactor*1000) / 1000,
			PersonalityEncounterRate: math.Round(func() float64 {
				if pos == "witness" {
					return selfEncBreak.Personality.WitnessEngage
				}
				return selfEncBreak.Personality.DirectEngage
			}()*1000) / 1000,
			WeightReasons:   append([]string(nil), chosen.reasons...),
			Location:        a.Location,
			IntendedKind:    br.Behavior.EngineKind,
			ExecutedKind:    opt.Action.Kind,
			TargetID:        opt.Action.TargetID,
			TargetName:      s.actorNameLocked(opt.Action.TargetID),
			Item:            opt.Action.Item,
			RenderedSummary: rendered,
		})
		if mem.EventID > 0 {
			s.lastTriggerByActor[a.ID] = mem.EventID
		}
		return opt.ID, true
	}

	return "", false
}

// tryEnvAndDeathReactionLocked lets actors respond causally to witnessed/heard death news
// (actor_died), major world events (artifact emergence, terrain shift, external invasion),
// and ecological crises (disaster_struck, threat_surge, macro_anomaly), modulated by
// personality stance weights, power tier, and runtime social bonds.
func secretReactionAllowed(p sim.Perception, m sim.Memory, action sim.Action) bool {
	if m.Observation == nil {
		return true
	}
	o := m.Observation
	if o.Kind != "secret_caught_red_handed" && o.Kind != "secret_discovered" {
		return true
	}
	for _, secret := range p.KnownSecrets {
		if secret.ID != o.ObjectID {
			continue
		}
		stance := secret.HolderStances[p.Self.ID]
		switch action.Kind {
		case "attack", "arrest", "open_case", "audit_corruption", "hold_trial", "expose_secret":
			return stance == "confront"
		case "blackmail":
			return stance == "leverage"
		case "speak", "transmit_message":
			return action.Statement == nil || secretReportAllowed(p, m.RootEventID, action.Statement)
		}
	}
	return true
}

func (s *RulebookSelector) tryEnvAndDeathReactionLocked(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	if p.Tick < s.actorNextRxnTick[a.ID] || len(a.Memories) == 0 {
		return "", false
	}
	selfTier := p.Catalog.ActorPowerTier(a)
	selfPower := sim.CombatPowerWithCatalog(p.Catalog, a)

	for i := len(a.Memories) - 1; i >= 0; i-- {
		m := a.Memories[i]
		if m.EventID <= 0 || p.Tick-m.Tick > 16 {
			continue
		}
		triggerKind := ""
		killerID := ""
		victimID := ""
		eventCat, instanceID, secretID := "", "", ""
		switch m.Kind {
		case "actor_died":
			triggerKind = "actor_died"
			victimID = m.Source
			if m.Observation != nil {
				if m.Observation.ObjectID != "" {
					victimID = m.Observation.ObjectID
				}
				killerID = m.Observation.SubjectID
			}
			if killerID == "" && m.TargetID != "" && m.TargetID != victimID {
				killerID = m.TargetID
			}
		case "world_event_triggered", "world_event_claimed":
			triggerKind = m.Kind
			if m.Observation != nil {
				eventCat, instanceID = m.Observation.ObjectID, m.Observation.SubjectID
			}
		case "disaster_struck", "threat_surge", "macro_anomaly", "wild_encounter",
			"party_betrayed", "faction_war_declared", "alliance_formed", "peace_treaty_signed",
			"secret_caught_red_handed", "witnessed_my_secret",
			"secret_discovered", "blackmailed", "blackmail_refused", "secret_exposed", "ambition_completed":
			triggerKind = m.Kind
			if m.Observation != nil && killerID == "" {
				killerID = m.Observation.SubjectID
				secretID = m.Observation.ObjectID
			}
			if m.Kind == "secret_exposed" && killerID == a.ID || m.Kind == "blackmail_refused" && m.TargetID == a.ID || m.Kind == "party_betrayed" || m.Kind == "blackmailed" {
				killerID = m.Source
			}
			if m.Kind == "blackmail_refused" && m.Source == a.ID {
				killerID = m.TargetID
			}
		case "speech", "channel_message":
			if m.Source == a.ID || m.Statement == nil {
				continue
			}
			switch m.Statement.Topic {
			case "actor_died":
				if m.FactEventID == 0 {
					continue
				}
				triggerKind = "actor_died"
				killerID = m.Statement.SubjectID
				victimID = m.Statement.ObjectID
			case "world_event", "world_event_claimed":
				triggerKind = "world_event_triggered"
				eventCat, instanceID = m.Statement.ObjectID, m.Statement.SubjectID
			case "secret_caught_red_handed", "secret_discovered", "secret_exposed":
				if m.FactEventID == 0 {
					continue
				}
				triggerKind, killerID, secretID = m.Statement.Topic, m.Statement.SubjectID, m.Statement.ObjectID
			case "macro_anomaly":
				triggerKind = "macro_anomaly"
			}
		}
		if triggerKind == "" {
			continue
		}
		evKey := fmt.Sprintf("env:%s:%d", a.ID, m.EventID)
		if s.reactedEvents[evKey] {
			continue
		}

		type envCand struct {
			opt    Option
			stance string
			weight int
			reason string
		}
		var cands []envCand
		vBonded := false
		if victimID != "" && victimID != a.ID {
			vt, vg, vm, vs, _ := inferRuntimeSocialBond(a, victimID, s.knownActorGuilds[victimID], p.ActiveGrudgeTargets)
			vBonded = vm || vs || vg || vt >= 3 || sim.ActorHasTag(p.ActiveGrudgeTargets, killerID)
		}
		killerTier := s.knownActorPowerTiers[killerID]
		killerPower := s.knownActorCombatPowers[killerID]

		stanceForSecret := ""
		for _, secret := range p.KnownSecrets {
			if secret.ID == secretID {
				stanceForSecret = secret.HolderStances[a.ID]
				break
			}
		}
		invasionThreat := ""
		for _, ev := range p.ActiveWorldEvents {
			if ev.InstanceID == instanceID && ev.DistrictID == a.Location {
				invasionThreat = ev.InvasionThreatID
				break
			}
		}
		enemy := func(id string) bool {
			for _, nb := range p.Nearby {
				if nb.ID == id {
					return nb.FactionRelation == "at_war"
				}
			}
			return false
		}

		for _, opt := range options {
			kind := opt.Action.Kind
			stance := ""
			baseW := 0
			reason := ""
			switch triggerKind {
			case "actor_died":
				switch kind {
				case "attack", "avenge_feud":
					if killerID != "" && opt.Action.TargetID == killerID && vBonded {
						stance, baseW, reason = "强硬反制", 42, "闻讯复仇"
					}
				case "arrest":
					if killerID != "" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "秉公处置", 45, "闻讯缉凶"
					}
				case "speak", "transmit_message":
					if opt.Action.Statement != nil && opt.Action.Statement.Topic == "actor_died" {
						stance, baseW, reason = "积极呼应", 36, "传递死讯"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 18, "避祸离开"
				}
			case "world_event_triggered", "world_event_claimed":
				switch kind {
				case "claim_world_event":
					if opt.Action.TargetID == instanceID {
						stance, baseW, reason = "趁机谋利", 60, "争夺发现的物资"
					}
				case "hunt_threat":
					if eventCat == "external_invasion" && opt.Action.TargetID == invasionThreat && invasionThreat != "" {
						stance, baseW, reason = "秉公处置", 48, "抵御入侵"
					}
				case "speak", "transmit_message":
					if opt.Action.Statement != nil && (opt.Action.Statement.Topic == "world_event" || opt.Action.Statement.Topic == "world_event_claimed") {
						stance, baseW, reason = "借力拉拢", 30, "扩散奇遇/异变消息"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 16, "因异变转移"
				}
			case "secret_caught_red_handed", "secret_discovered", "party_betrayed", "blackmailed", "blackmail_refused":
				switch kind {
				case "expose_secret", "open_case", "arrest", "audit_corruption":
					if stanceForSecret == "confront" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "秉公处置", 52, "撞破丑事当场揭发/缉拿"
					}
				case "attack":
					if (stanceForSecret == "confront" || secretID == "") && killerID != "" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "强硬反制", 44, "嫉恶如仇/遭背刺当场出手"
					}
				case "blackmail":
					if stanceForSecret == "leverage" && opt.Action.Item == secretID {
						stance, baseW, reason = "趁机谋利", 34, "借把柄暗中牟利"
					}
				case "speak", "transmit_message":
					if stanceForSecret == "confront" && opt.Action.Statement != nil && opt.Action.Statement.ObjectID == secretID {
						stance, baseW, reason = "积极呼应", 30, "暗中举报/传递秘辛"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 22, "装作未见悄然离场"
				}
			case "witnessed_my_secret":
				switch kind {
				case "attack":
					if killerID != "" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "强硬反制", 46, "丑事败露企图灭口"
					}
				case "bribe":
					if killerID != "" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "借力拉拢", 42, "行贿目击者封口"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 26, "事发遁走"
				}
			case "secret_exposed":
				switch kind {
				case "open_case", "arrest", "audit_corruption":
					if opt.Action.TargetID == killerID && (m.Observation == nil || m.Observation.SubjectID != a.ID) {
						stance, baseW, reason = "秉公处置", 50, "丑闻公开依法追责"
					}
				case "attack", "avenge_feud":
					if killerID != "" && opt.Action.TargetID == killerID {
						stance, baseW, reason = "强硬反制", 38, "闻丑闻出手清算"
					}
				case "speak", "transmit_message":
					if opt.Action.Statement != nil && opt.Action.Statement.Topic == "secret_exposed" && opt.Action.Statement.ObjectID == secretID {
						stance, baseW, reason = "积极呼应", 26, "议论并扩散曝光丑闻"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 18, "远离丑闻风波"
				}
			case "faction_war_declared", "alliance_formed", "peace_treaty_signed":
				switch kind {
				case "propose_alliance":
					if triggerKind == "faction_war_declared" {
						stance, baseW, reason = "借力拉拢", 46, "战事爆发寻求结盟"
					}
				case "make_peace":
					if triggerKind == "faction_war_declared" {
						stance, baseW, reason = "谈判妥协", 24, "试图斡旋停战"
					}
				case "attack":
					if triggerKind == "faction_war_declared" && enemy(opt.Action.TargetID) {
						stance, baseW, reason = "强硬反制", 36, "响应宣战正面交锋"
					}
				case "form_party", "join_party":
					if triggerKind == "faction_war_declared" {
						stance, baseW, reason = "借力拉拢", 34, "战时结伴同行"
					}
				case "speak", "transmit_message":
					stance, baseW, reason = "积极呼应", 24, "传递战和外交军情"
				case "move", "move_venue":
					if triggerKind == "faction_war_declared" {
						stance, baseW, reason = "避险止损", 24, "避开战区转移"
					}
				}
			case "ambition_completed":
				switch kind {
				case "speak", "transmit_message":
					stance, baseW, reason = "积极呼应", 22, "谈论人物志业达成"
				}
			default: // disaster_struck, threat_surge, macro_anomaly, wild_encounter
				switch kind {
				case "hunt_threat":
					stance, baseW, reason = "秉公处置", 44, "镇压兽潮/危机"
				case "maintain_infra", "repair_house", "heal":
					stance, baseW, reason = "积极呼应", 38, "抢险救灾"
				case "offer":
					stance, baseW, reason = "趁机谋利", 22, "灾时交易"
				case "speak", "transmit_message":
					if opt.Action.Statement != nil && opt.Action.Statement.Topic == "macro_anomaly" {
						stance, baseW, reason = "借力拉拢", 28, "传讯示警"
					}
				case "move", "move_venue":
					stance, baseW, reason = "避险止损", 24, "避灾转移"
				}
			}
			if baseW <= 0 {
				continue
			}
			mult := 1.0
			if s.Rulebook != nil && len(s.Rulebook.PersonalityStanceWeights) > 0 {
				for _, tr := range ExtractPersonalityTraits(a.Personality) {
					if sw := s.Rulebook.PersonalityStanceWeights[tr]; sw != nil {
						if wv := sw[stance]; wv > 0 {
							mult *= wv
						}
					}
				}
				mult = clampFloat(mult, 0.25, 3.5)
			}
			if triggerKind == "actor_died" && killerID != "" {
				pm, _ := computePowerGapStanceMultiplier(stance, selfTier, killerTier, selfPower, killerPower)
				mult *= pm
			} else if stance == "避险止损" && selfTier == 1 {
				mult *= 1.65 // low-tier mortals strongly prefer evacuating crises
			}
			eff := int(math.Round(float64(baseW) * mult))
			if eff < 1 {
				eff = 1
			}
			cands = append(cands, envCand{opt: opt, stance: stance, weight: eff, reason: reason})
		}
		if len(cands) == 0 {
			continue
		}
		sort.SliceStable(cands, func(x, y int) bool {
			if cands[x].weight != cands[y].weight {
				return cands[x].weight > cands[y].weight
			}
			return cands[x].opt.ID < cands[y].opt.ID
		})
		totalW := 0
		for _, c := range cands {
			totalW += c.weight
		}
		pick := int(fnvHash(fmt.Sprintf("%s|%d|%s|%d|env_rxn", s.Rulebook.WorldID, s.Seed, a.ID, m.EventID)) % uint64(totalW))
		chosen := cands[0]
		acc := 0
		for _, c := range cands {
			acc += c.weight
			if pick < acc {
				chosen = c
				break
			}
		}

		s.reactedEvents[evKey] = true
		s.actorNextRxnTick[a.ID] = p.Tick + 6
		s.stats.ReactionDecisions++
		s.stats.WitnessReactions++
		s.stats.ByActorWitnessReactions[a.ID]++
		s.stats.ByStance[chosen.stance]++
		s.stats.ByTriggerKind[triggerKind]++
		s.stats.ByExecutedKind[chosen.opt.Action.Kind]++
		s.events = append(s.events, RulebookDecisionEvent{
			Tick:               p.Tick,
			Day:                day,
			Clock:              p.Clock.Label,
			Kind:               "reaction",
			RuleID:             "env_" + triggerKind,
			RuleName:           chosen.reason,
			Category:           "world_environment",
			Stance:             chosen.stance,
			ResponderPosition:  "witness",
			TriggerEventID:     m.EventID,
			TriggerKind:        triggerKind,
			ResponderID:        a.ID,
			ResponderName:      a.Name,
			ResponderRole:      a.Role,
			ResponderForm:      a.FormID,
			ResponderPowerTier: selfTier,
			Location:           a.Location,
			IntendedKind:       chosen.opt.Action.Kind,
			ExecutedKind:       chosen.opt.Action.Kind,
			TargetID:           chosen.opt.Action.TargetID,
			RenderedSummary:    fmt.Sprintf("%s因%s采取[%s](%s)", a.Name, chosen.reason, chosen.stance, chosen.opt.Action.Kind),
		})
		if m.EventID > 0 {
			s.lastTriggerByActor[a.ID] = m.EventID
		}
		return chosen.opt.ID, true
	}
	return "", false
}

// LastTriggerEventID returns the causal memory EventID that motivated actorID's most recent reactive decision, if any.
func (s *RulebookSelector) LastTriggerEventID(actorID string) int64 {
	if s == nil || actorID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastTriggerByActor[actorID]
}

func (s *RulebookSelector) tryProactiveInteractionLocked(p sim.Perception, options []Option, day int) (string, bool) {
	a := p.Self
	if s.actorNextProTick[a.ID] > p.Tick {
		return "", false
	}
	if len(s.Rulebook.ProactiveRules) == 0 {
		return "", false
	}

	selfProf := profileFromSelf(p)
	venueFactor := computeVenueExposureFactor(s.Rulebook, p)
	selfEncBreak := computeEffectiveEncounterProfile(s.Rulebook, a.Role, a.Personality, venueFactor)
	selfEnc := selfEncBreak.Effective
	hasEncounterMods := selfEncBreak.Active
	nearbyByID := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		if !nb.Jailed && !nb.Sleeping {
			nearbyByID[nb.ID] = nb
		}
	}

	type proTargetCand struct {
		actor2           actorProfileView
		opt              *Option
		baseWeight       int
		effWeight        int
		sampleW          int
		mult             float64
		powerMult        float64
		powerTierGap     int
		skillModulated   bool
		targetForm       string
		targetTier       int
		targetAttraction float64
		hobbyWeighted    bool
		shared           []string
		reasons          []string
	}
	selfTier := p.Catalog.ActorPowerTier(a)
	selfPower := sim.CombatPowerWithCatalog(p.Catalog, a)
	type proRuleGroup struct {
		rule    *InteractionRule
		ruleW   int
		targets []proTargetCand
	}
	var groups []proRuleGroup

	for idx := range s.Rulebook.ProactiveRules {
		rule := &s.Rulebook.ProactiveRules[idx]
		cdKey := a.ID + ":" + rule.ID
		if s.proactiveCooldown[cdKey] > p.Tick {
			continue
		}
		if !districtMatchesFeatures(p, rule.LocationFeatures) {
			continue
		}
		if !matchesRole(rule.Actor1, selfProf) {
			continue
		}

		// Candidate Actor2 pool: all nearby awake/unjailed actors + known peers for remote transmit_message.
		var targets []actorProfileView
		seenTargets := map[string]bool{}
		for _, nb := range p.Nearby {
			if nb.Jailed || nb.Sleeping || nb.ID == a.ID {
				continue
			}
			seenTargets[nb.ID] = true
			targets = append(targets, profileFromNeighbor(nb))
		}
		if rule.Behavior.EngineKind == "transmit_message" && (rule.Relation.CoLocated == nil || !*rule.Relation.CoLocated) {
			for pid := range a.Trust {
				if pid == "" || pid == a.ID || seenTargets[pid] {
					continue
				}
				seenTargets[pid] = true
				targets = append(targets, actorProfileView{
					ID:          pid,
					Name:        s.actorNameLocked(pid),
					Role:        s.actorRoleLocked(pid),
					Tags:        []string{s.actorRoleLocked(pid)},
					Hobbies:     s.actorHobbiesLocked(pid),
					Personality: strings.ToLower(s.actorPersonalityLocked(pid)),
				})
			}
		}

		sort.SliceStable(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

		var feasible []proTargetCand
		maxEffW := 0
		baseW := priorityWeight(rule.Priority) + rule.Weight
		if baseW < 10 {
			baseW = 10
		}

		for _, tProf := range targets {
			_, coLoc := nearbyByID[tProf.ID]
			if !matchesRole(rule.Actor2, tProf) {
				continue
			}
			if !s.matchesRelationLocked(p, rule.Relation, tProf.ID, coLoc) {
				continue
			}
			targetID := resolveTargetBinding(p, rule.Behavior.TargetBinding, a.ID, tProf.ID, nearbyByID)
			opt := matchGenericBehavior(p, options, rule.Behavior, targetID)
			if opt == nil {
				continue
			}
			mult, shared, reasons := computeHobbyBehaviorMultiplier(s.Rulebook, rule, selfProf.Hobbies, tProf.Hobbies)
			hobbyWeighted := len(reasons) > 0
			if peer, ok := nearbyByID[opt.Action.TargetID]; ok {
				switch opt.Action.Kind {
				case "attack", "raid", "steal", "extort":
					if peer.FactionRelation == "allied" {
						mult *= 0.25
					} else if peer.FactionRelation == "at_war" {
						mult *= 1.5
					}
				case "heal", "offer", "form_party":
					if peer.FactionRelation == "allied" {
						mult *= 1.5
					}
				}
			}
			skillMult, skillReason := computeSkillProactiveMultiplier(p.Catalog, a.Skills, opt.Action.Kind)
			skillModulated := false
			if skillReason != "" {
				mult *= skillMult
				reasons = append(reasons, skillReason)
				skillModulated = true
			}
			tTier := s.knownActorPowerTiers[tProf.ID]
			tPower := s.knownActorCombatPowers[tProf.ID]
			tForm := s.knownActorForms[tProf.ID]
			pMult, pReason := computePowerGapProactiveMultiplier(opt.Action.Kind, selfTier, tTier, selfPower, tPower, a.Personality)
			if pReason != "" {
				mult *= pMult
				reasons = append(reasons, pReason)
			}
			tierGap := 0
			if selfTier > 0 && tTier > 0 {
				tierGap = selfTier - tTier
			}
			effW := int(math.Round(float64(baseW) * mult))
			if effW < 1 {
				effW = 1
			}
			// Target's structural profession exposure x venue foot-traffic x personality scales how
			// likely others are to notice, approach, or target them!
			tRole := tProf.Role
			if tRole == "" {
				tRole = s.actorRoleLocked(tProf.ID)
			}
			tPersonality := tProf.Personality
			if tPersonality == "" {
				tPersonality = s.actorPersonalityLocked(tProf.ID)
			}
			tVenue := 1.0
			if coLoc {
				tVenue = venueFactor
			}
			tEncBreak := computeEffectiveEncounterProfile(s.Rulebook, tRole, tPersonality, tVenue)
			tAttract := tEncBreak.Effective.TargetAttraction
			if !coLoc && hasEncounterMods {
				tAttract = clampFloat(tAttract*0.55, 0.15, 3.0)
			}
			sampleW := int(math.Round(float64(effW) * tAttract))
			if math.Abs(tAttract-1.0) > 1e-6 {
				reasons = append(reasons, fmt.Sprintf("目标注目度:职业×%.2f·性格×%.2f→×%.2f", tEncBreak.Profession.TargetAttraction, tEncBreak.Personality.TargetAttraction, tAttract))
			}
			pairKey := a.ID + "->" + tProf.ID
			if s.rulePairs[rule.ID] == nil {
				sampleW = int(math.Round(float64(sampleW) * 1.15))
			} else if firstPairTick, ok := s.rulePairs[rule.ID][pairKey]; !ok || firstPairTick >= p.Tick {
				sampleW = int(math.Round(float64(sampleW) * 1.15))
			}
			if sampleW < 1 {
				sampleW = 1
			}
			if effW > maxEffW {
				maxEffW = effW
			}
			feasible = append(feasible, proTargetCand{
				actor2:           tProf,
				opt:              opt,
				baseWeight:       baseW,
				effWeight:        effW,
				sampleW:          sampleW,
				mult:             mult,
				powerMult:        pMult,
				powerTierGap:     tierGap,
				skillModulated:   skillModulated,
				targetForm:       tForm,
				targetTier:       tTier,
				targetAttraction: tAttract,
				hobbyWeighted:    hobbyWeighted,
				shared:           shared,
				reasons:          reasons,
			})
		}
		if len(feasible) == 0 {
			continue
		}
		ruleW := maxEffW
		if firstTick, ok := s.firedProactive[rule.ID]; !ok || firstTick >= p.Tick {
			ruleW = int(math.Round(float64(ruleW) * 1.25))
		}
		if ruleW < 1 {
			ruleW = 1
		}
		groups = append(groups, proRuleGroup{
			rule:    rule,
			ruleW:   ruleW,
			targets: feasible,
		})
	}

	if len(groups) == 0 {
		return "", false
	}

	// Stage 1: Deterministic weighted sample ONE proactive rule (eliminates target-count bias
	// while letting Actor1's hobbies & shared-hobby opportunities directly scale behavior probability).
	sort.SliceStable(groups, func(i, j int) bool {
		return groups[i].rule.ID < groups[j].rule.ID
	})
	totalRuleW := 0
	for _, g := range groups {
		totalRuleW += g.ruleW
	}
	ruleHash := fnvHash(fmt.Sprintf("%s|%d|%s|%d|proactive_rule", s.Rulebook.WorldID, s.Seed, a.ID, p.Tick))
	rulePick := int(ruleHash % uint64(totalRuleW))
	chosenGroup := groups[0]
	acc := 0
	for _, g := range groups {
		acc += g.ruleW
		if rulePick < acc {
			chosenGroup = g
			break
		}
	}

	// Stage 2: Deterministic weighted sample ONE target Actor2 for the chosen rule
	// (favoring shared-hobby partners, high-attraction targets, and fresh pairs).
	targets := chosenGroup.targets
	sort.SliceStable(targets, func(i, j int) bool {
		return targets[i].actor2.ID < targets[j].actor2.ID
	})
	totalTargetW := 0
	for _, tc := range targets {
		totalTargetW += tc.sampleW
	}
	targetHash := fnvHash(fmt.Sprintf("%s|%d|%s|%s|%d|proactive_target", s.Rulebook.WorldID, s.Seed, a.ID, chosenGroup.rule.ID, p.Tick))
	targetPick := int(targetHash % uint64(totalTargetW))
	chosen := targets[0]
	acc = 0
	for _, tc := range targets {
		acc += tc.sampleW
		if targetPick < acc {
			chosen = tc
			break
		}
	}

	rule := chosenGroup.rule
	actorCD := int64(18)
	ruleCD := rule.CooldownTicks
	if hasEncounterMods && math.Abs(selfEnc.ProactiveRate-1.0) > 1e-6 {
		actorCD = int64(math.Round(18.0 / clampFloat(selfEnc.ProactiveRate, 0.22, 3.0)))
		if actorCD < 6 {
			actorCD = 6
		}
		if actorCD > 84 {
			actorCD = 84
		}
		ruleCD = int64(math.Round(float64(rule.CooldownTicks) / clampFloat(selfEnc.ProactiveRate, 0.30, 2.5)))
		if ruleCD < 10 {
			ruleCD = 10
		}
		if ruleCD > 160 {
			ruleCD = 160
		}
		s.stats.EncounterModulatedProactives++
	}
	s.proactiveCooldown[a.ID+":"+rule.ID] = p.Tick + ruleCD
	s.actorNextProTick[a.ID] = p.Tick + actorCD
	if prev, ok := s.firedProactive[rule.ID]; !ok || p.Tick < prev {
		s.firedProactive[rule.ID] = p.Tick
	}
	s.stats.ProactiveDecisions++
	s.stats.ByActorProactive[a.ID]++
	if chosen.actor2.ID != "" {
		s.stats.ByActorTargeted[chosen.actor2.ID]++
	}
	if math.Abs(chosen.targetAttraction-1.0) > 1e-6 {
		s.stats.TargetAttractionModulated++
	}
	if chosen.hobbyWeighted {
		s.stats.HobbyWeightedProactives++
	}
	if math.Abs(chosen.powerMult-1.0) > 1e-6 {
		s.stats.PowerModulatedProactives++
	}
	if chosen.skillModulated {
		s.stats.SkillModulatedProactives++
	}
	if len(chosen.shared) > 0 {
		s.stats.SharedHobbyProactives++
	}
	if rule.Category != "" {
		s.stats.ByCategory[rule.Category]++
	}
	s.stats.ByExecutedKind[chosen.opt.Action.Kind]++
	s.stats.ByRuleCount[rule.ID]++
	for _, h := range selfProf.Hobbies {
		incNestedInt(s.stats.ByHobbyAction, h, chosen.opt.Action.Kind)
		if rule.Category != "" {
			incNestedInt(s.stats.ByHobbyCategory, h, rule.Category)
		}
	}
	s.recordPairLocked(rule.ID, a.ID, chosen.actor2.ID, p.Tick)

	rendered := renderCausalTemplate(
		rule.Behavior.SummaryTemplate,
		a.Name, chosen.actor2.Name, a.Name,
		s.locNameLocked(a.Location),
		s.itemNameLocked(chosen.opt.Action.Item),
		"",
	)
	targetPers := chosen.actor2.Personality
	if targetPers == "" {
		targetPers = s.actorPersonalityLocked(chosen.actor2.ID)
	}
	s.events = append(s.events, RulebookDecisionEvent{
		Tick:                     p.Tick,
		Day:                      day,
		Clock:                    p.Clock.Label,
		Kind:                     "proactive",
		RuleID:                   rule.ID,
		RuleName:                 rule.Name,
		Category:                 rule.Category,
		ResponderPosition:        "actor1",
		Actor1ID:                 a.ID,
		Actor1Name:               a.Name,
		Actor1Role:               a.Role,
		Actor2ID:                 chosen.actor2.ID,
		Actor2Name:               chosen.actor2.Name,
		Actor2Role:               chosen.actor2.Role,
		ResponderID:              a.ID,
		ResponderName:            a.Name,
		ResponderRole:            a.Role,
		ResponderForm:            a.FormID,
		ResponderPowerTier:       selfTier,
		TargetForm:               chosen.targetForm,
		TargetPowerTier:          chosen.targetTier,
		PowerTierGap:             chosen.powerTierGap,
		PowerMultiplier:          math.Round(chosen.powerMult*1000) / 1000,
		ResponderPersonality:     a.Personality,
		TargetPersonality:        targetPers,
		ResponderHobbies:         append([]string(nil), selfProf.Hobbies...),
		SharedHobbies:            append([]string(nil), chosen.shared...),
		BaseWeight:               chosen.baseWeight,
		EffectiveWeight:          chosen.effWeight,
		WeightMultiplier:         math.Round(chosen.mult*1000) / 1000,
		EncounterRate:            math.Round(selfEnc.ProactiveRate*1000) / 1000,
		ProfessionEncounterRate:  math.Round(selfEncBreak.Profession.ProactiveRate*1000) / 1000,
		VenueEncounterRate:       math.Round(selfEncBreak.VenueFactor*1000) / 1000,
		PersonalityEncounterRate: math.Round(selfEncBreak.Personality.ProactiveRate*1000) / 1000,
		TargetAttraction:         math.Round(chosen.targetAttraction*1000) / 1000,
		WeightReasons:            append([]string(nil), chosen.reasons...),
		Location:                 a.Location,
		IntendedKind:             rule.Behavior.EngineKind,
		ExecutedKind:             chosen.opt.Action.Kind,
		TargetID:                 chosen.opt.Action.TargetID,
		TargetName:               s.actorNameLocked(chosen.opt.Action.TargetID),
		Item:                     chosen.opt.Action.Item,
		RenderedSummary:          rendered,
	})
	return chosen.opt.ID, true
}

func (s *RulebookSelector) recordPairLocked(ruleID, a1, a2 string, tick int64) {
	if a1 != "" && a2 != "" {
		pk := a1 + "->" + a2
		s.uniquePairs[pk] = true
		if s.rulePairs[ruleID] == nil {
			s.rulePairs[ruleID] = map[string]int64{}
		}
		if prev, ok := s.rulePairs[ruleID][pk]; !ok || tick < prev {
			s.rulePairs[ruleID][pk] = tick
		}
	}
	if a2 != "" {
		s.uniqueResponders[a2] = true
	}
	if a1 != "" {
		s.uniqueResponders[a1] = true
	}
}

func (s *RulebookSelector) locNameLocked(locID string) string {
	if nm := s.Rulebook.Glossaries.Locations[locID]; nm != "" {
		return nm
	}
	return locID
}

func (s *RulebookSelector) itemNameLocked(itemID string) string {
	if itemID == "" {
		return ""
	}
	if nm := s.Rulebook.Glossaries.Items[itemID]; nm != "" {
		return nm
	}
	return itemID
}

func eventKindMatches(allowed []string, kind string) bool {
	for _, k := range allowed {
		if k == "*" || k == kind {
			return true
		}
	}
	return false
}

func ruleSpecificity(m RoleMatcher) int {
	n := 0
	if len(m.Roles) > 0 && !containsWildcard(m.Roles) {
		n += 3
	}
	if len(m.AnyTags) > 0 && !containsWildcard(m.AnyTags) {
		n += 2
	}
	if len(m.AnyHobbies) > 0 && !containsWildcard(m.AnyHobbies) {
		n += 2
	}
	if len(m.PersonalityKeywords) > 0 {
		n += 2
	}
	if len(m.ExcludeTags) > 0 {
		n++
	}
	if len(m.ExcludeHobbies) > 0 {
		n++
	}
	return n
}

func resolveTargetBinding(p sim.Perception, binding, actor1ID, actor2ID string, nearbyByID map[string]sim.Neighbor) string {
	switch strings.ToLower(strings.TrimSpace(binding)) {
	case "$actor1":
		return actor1ID
	case "$actor2":
		return actor2ID
	case "$self":
		return p.Self.ID
	case "$enforcer_or_elder":
		for _, nb := range p.Nearby {
			if nb.ID == p.Self.ID || nb.Jailed || nb.Sleeping {
				continue
			}
			if p.Catalog.IsEnforcementTags(nb.Tags) || p.Catalog.IsOversightTags(nb.Tags) {
				return nb.ID
			}
		}
		return ""
	case "$ally":
		bestID, bestTrust := "", -1
		for _, nb := range p.Nearby {
			if nb.ID == p.Self.ID || nb.ID == actor1ID || nb.Jailed || nb.Sleeping {
				continue
			}
			if tr := p.Self.Trust[nb.ID]; tr > bestTrust || tr == bestTrust && bestID != "" && nb.ID < bestID {
				bestID, bestTrust = nb.ID, tr
			}
		}
		if bestID != "" {
			return bestID
		}
		for pid, tr := range p.Self.Trust {
			if pid != p.Self.ID && pid != actor1ID && (tr > bestTrust || tr == bestTrust && bestID != "" && pid < bestID) {
				bestID, bestTrust = pid, tr
			}
		}
		return bestID
	case "":
		if actor2ID != "" && actor2ID != p.Self.ID {
			return actor2ID
		}
		return actor1ID
	}
	return ""
}

func matchGenericBehavior(p sim.Perception, options []Option, spec BehaviorSpec, targetID string) *Option {
	index := matchBehaviorAction(p, spec, targetID, len(options), func(i int) sim.Action {
		return options[i].Action
	})
	if index < 0 {
		return nil
	}
	return &options[index]
}

func renderCausalTemplate(tmpl, actor1Name, actor2Name, responderName, locName, itemName, outcomeKind string) string {
	out := strings.TrimSpace(tmpl)
	if out == "" {
		out = "$responder 对 $actor1 做出应对"
	}
	a1 := nonEmptyOr(actor1Name, "角色1")
	a2 := nonEmptyOr(actor2Name, "角色2")
	resp := nonEmptyOr(responderName, "应对者")
	loc := nonEmptyOr(locName, "当地")
	item := nonEmptyOr(itemName, "物资")
	repl := strings.NewReplacer(
		"$actor1", a1,
		"{actor1}", a1,
		"$actor2", a2,
		"{actor2}", a2,
		"$responder", resp,
		"{responder}", resp,
		"$location", loc,
		"{location}", loc,
		"$item", item,
		"{item}", item,
		"$outcome", outcomeKind,
		"{outcome}", outcomeKind,
	)
	return repl.Replace(out)
}

func nonEmptyOr(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

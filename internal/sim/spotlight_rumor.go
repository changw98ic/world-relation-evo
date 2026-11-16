package sim

import (
	"fmt"
	"sort"
	"strings"
)

// SpotlightRumorSpec defines genre-decoupled configuration parameters for dynamic
// actor-cohort spotlight hydration/dehydration and bidirectional rumor/panic resonance.
type SpotlightRumorSpec struct {
	MaxFocalActors             int   `json:"max_focal_actors"`
	TargetFocalActors          int   `json:"target_focal_actors"`
	DailyPromotionCap          int   `json:"daily_promotion_cap"`
	DailyDemotionCap           int   `json:"daily_demotion_cap"`
	MinCohortPopulation        int   `json:"min_cohort_population"`
	RumorCorroborationSpeakers int   `json:"rumor_corroboration_speakers"`
	RumorTTLTicks              int64 `json:"rumor_ttl_ticks"`
	PanicDurationTicks         int64 `json:"panic_duration_ticks"`
	PanicDemandBoostPermille   int   `json:"panic_demand_boost_permille"`
}

// DefaultSpotlightRumorSpec returns the baseline spotlight and rumor resonance parameters.
func DefaultSpotlightRumorSpec() SpotlightRumorSpec {
	return SpotlightRumorSpec{
		MaxFocalActors:             128,
		TargetFocalActors:          124,
		DailyPromotionCap:          4,
		DailyDemotionCap:           4,
		MinCohortPopulation:        5,
		RumorCorroborationSpeakers: 2,
		RumorTTLTicks:              TicksPerDay * 2,
		PanicDurationTicks:         TicksPerDay,
		PanicDemandBoostPermille:   350,
	}
}

// EffectiveSpotlightRumorSpec normalizes zero/negative fields to safe defaults.
func EffectiveSpotlightRumorSpec(s SpotlightRumorSpec) SpotlightRumorSpec {
	def := DefaultSpotlightRumorSpec()
	if s.MaxFocalActors <= 0 {
		s.MaxFocalActors = def.MaxFocalActors
	}
	if s.TargetFocalActors <= 0 {
		s.TargetFocalActors = minInt(def.TargetFocalActors, s.MaxFocalActors)
	}
	if s.TargetFocalActors > s.MaxFocalActors {
		s.TargetFocalActors = s.MaxFocalActors
	}
	if s.DailyPromotionCap <= 0 {
		s.DailyPromotionCap = def.DailyPromotionCap
	}
	if s.DailyDemotionCap <= 0 {
		s.DailyDemotionCap = def.DailyDemotionCap
	}
	if s.MinCohortPopulation <= 0 {
		s.MinCohortPopulation = def.MinCohortPopulation
	}
	if s.RumorCorroborationSpeakers <= 0 {
		s.RumorCorroborationSpeakers = def.RumorCorroborationSpeakers
	}
	if s.RumorTTLTicks <= 0 {
		s.RumorTTLTicks = def.RumorTTLTicks
	}
	if s.PanicDurationTicks <= 0 {
		s.PanicDurationTicks = def.PanicDurationTicks
	}
	if s.PanicDemandBoostPermille <= 0 {
		s.PanicDemandBoostPermille = def.PanicDemandBoostPermille
	}
	return s
}

// ActiveRumor tracks an emerging rumor thread in a public forum (hub/market/shrine) district.
type ActiveRumor struct {
	Key          string   `json:"key"`
	District     string   `json:"district"`
	Topic        string   `json:"topic"`
	Item         string   `json:"item,omitempty"`
	Speakers     []string `json:"speakers"`
	FirstEventID int64    `json:"first_event_id,omitempty"`
	LastEventID  int64    `json:"last_event_id,omitempty"`
	FirstTick    int64    `json:"first_tick"`
	LastTick     int64    `json:"last_tick"`
	Intensity    int      `json:"intensity"`
	PanicFired   bool     `json:"panic_fired,omitempty"`
	PanicUntil   int64    `json:"panic_until,omitempty"`
}

// SpotlightRumorState holds dynamic spotlight salience/heat, hydrated actor tracking,
// active micro->macro rumor threads, regional panic demand multipliers, and seeded gossip dedup state.
type SpotlightRumorState struct {
	Spec                SpotlightRumorSpec        `json:"spec"`
	ActorSalience       map[string]int            `json:"actor_salience"`
	CohortHeat          map[string]int            `json:"cohort_heat"`
	HydratedCohort      map[string]string         `json:"hydrated_cohort"`
	HydratedTick        map[string]int64          `json:"hydrated_tick"`
	ActiveRumors        map[string]*ActiveRumor   `json:"active_rumors"`
	PanicDemandPermille map[string]map[string]int `json:"panic_demand_permille"`
	PanicUntilTick      map[string]int64          `json:"panic_until_tick"`
	SeededGossipEvents  map[int64]bool            `json:"seeded_gossip_events"`
	TotalPromoted       int64                     `json:"total_promoted,omitempty"`
	TotalDemoted        int64                     `json:"total_demoted,omitempty"`
	TotalPanics         int64                     `json:"total_panics,omitempty"`
	TotalGossipSeeded   int64                     `json:"total_gossip_seeded,omitempty"`
}

// NewSpotlightRumorState constructs an initialized SpotlightRumorState with default parameters.
func NewSpotlightRumorState() *SpotlightRumorState {
	return &SpotlightRumorState{
		Spec:                DefaultSpotlightRumorSpec(),
		ActorSalience:       map[string]int{},
		CohortHeat:          map[string]int{},
		HydratedCohort:      map[string]string{},
		HydratedTick:        map[string]int64{},
		ActiveRumors:        map[string]*ActiveRumor{},
		PanicDemandPermille: map[string]map[string]int{},
		PanicUntilTick:      map[string]int64{},
		SeededGossipEvents:  map[int64]bool{},
	}
}

// CloneSpotlightRumorState returns a nil-safe deep copy of SpotlightRumorState.
func CloneSpotlightRumorState(s *SpotlightRumorState) *SpotlightRumorState {
	if s == nil {
		return nil
	}
	out := *s
	if s.ActorSalience != nil {
		out.ActorSalience = cloneIntMap(s.ActorSalience)
	}
	if s.CohortHeat != nil {
		out.CohortHeat = cloneIntMap(s.CohortHeat)
	}
	if s.HydratedCohort != nil {
		out.HydratedCohort = cloneStringMap(s.HydratedCohort)
	}
	if s.HydratedTick != nil {
		out.HydratedTick = make(map[string]int64, len(s.HydratedTick))
		for k, v := range s.HydratedTick {
			out.HydratedTick[k] = v
		}
	}
	if s.ActiveRumors != nil {
		out.ActiveRumors = make(map[string]*ActiveRumor, len(s.ActiveRumors))
		for k, r := range s.ActiveRumors {
			if r == nil {
				continue
			}
			rc := *r
			if r.Speakers != nil {
				rc.Speakers = append([]string(nil), r.Speakers...)
			}
			out.ActiveRumors[k] = &rc
		}
	}
	if s.PanicDemandPermille != nil {
		out.PanicDemandPermille = make(map[string]map[string]int, len(s.PanicDemandPermille))
		for dID, items := range s.PanicDemandPermille {
			out.PanicDemandPermille[dID] = cloneIntMap(items)
		}
	}
	if s.PanicUntilTick != nil {
		out.PanicUntilTick = make(map[string]int64, len(s.PanicUntilTick))
		for k, v := range s.PanicUntilTick {
			out.PanicUntilTick[k] = v
		}
	}
	if s.SeededGossipEvents != nil {
		out.SeededGossipEvents = make(map[int64]bool, len(s.SeededGossipEvents))
		for k, v := range s.SeededGossipEvents {
			out.SeededGossipEvents[k] = v
		}
	}
	return &out
}

func (sr *SpotlightRumorState) ensureInit() {
	sr.Spec = EffectiveSpotlightRumorSpec(sr.Spec)
	if sr.ActorSalience == nil {
		sr.ActorSalience = map[string]int{}
	}
	if sr.CohortHeat == nil {
		sr.CohortHeat = map[string]int{}
	}
	if sr.HydratedCohort == nil {
		sr.HydratedCohort = map[string]string{}
	}
	if sr.HydratedTick == nil {
		sr.HydratedTick = map[string]int64{}
	}
	if sr.ActiveRumors == nil {
		sr.ActiveRumors = map[string]*ActiveRumor{}
	}
	if sr.PanicDemandPermille == nil {
		sr.PanicDemandPermille = map[string]map[string]int{}
	}
	if sr.PanicUntilTick == nil {
		sr.PanicUntilTick = map[string]int64{}
	}
	if sr.SeededGossipEvents == nil {
		sr.SeededGossipEvents = map[int64]bool{}
	}
}

// DistrictPanicDemandPermille returns the current temporary panic hoarding demand boost (in permille)
// for a district and item, or 0 if no panic is active or expired.
func (w *World) DistrictPanicDemandPermille(sr *SpotlightRumorState, districtID, itemID string) int {
	if w == nil || sr == nil || districtID == "" || itemID == "" {
		return 0
	}
	until, ok := sr.PanicUntilTick[districtID]
	if !ok || w.Tick >= until {
		return 0
	}
	items := sr.PanicDemandPermille[districtID]
	if items == nil {
		return 0
	}
	if v := items[itemID]; v > 0 {
		return v
	}
	return 0
}

// ObserveSpotlightAndRumors updates actor salience and cohort spotlight heat from settled events,
// amplifies corroborated micro speech rumors into regional panic hoarding/morale shifts, and
// seeds major macro events as citable teahouse gossip memories into awake hub/market focal actors.
func (w *World) ObserveSpotlightAndRumors(sr *SpotlightRumorState, settled []Event) []Event {
	if w == nil || sr == nil {
		return nil
	}
	sr.ensureInit()
	spec := sr.Spec

	// Expire finished district panics and stale active rumor threads.
	for dID, until := range sr.PanicUntilTick {
		if w.Tick >= until {
			delete(sr.PanicUntilTick, dID)
			delete(sr.PanicDemandPermille, dID)
		}
	}
	for k, r := range sr.ActiveRumors {
		if r == nil || w.Tick-r.LastTick > spec.RumorTTLTicks {
			delete(sr.ActiveRumors, k)
		}
	}

	cohortIndex := make(map[string]*PopulationCohort, len(w.Cohorts))
	for i := range w.Cohorts {
		cohortIndex[w.Cohorts[i].ID] = &w.Cohorts[i]
	}

	addActorSalience := func(actorID string, delta int) {
		if actorID == "" || delta <= 0 {
			return
		}
		a := w.actor(actorID)
		if a == nil || a.Dead {
			return
		}
		sr.ActorSalience[actorID] = minInt(1000, sr.ActorSalience[actorID]+delta)
	}
	addCohortHeat := func(cohortID string, delta int) {
		if cohortID == "" || delta <= 0 {
			return
		}
		if _, ok := cohortIndex[cohortID]; !ok {
			return
		}
		sr.CohortHeat[cohortID] = minInt(1000, sr.CohortHeat[cohortID]+delta)
	}

	var emitted []Event
	for _, ev := range settled {
		switch ev.Kind {
		case "gossip_seeded", "rumor_panic_triggered", "spotlight_promoted", "spotlight_demoted":
			continue
		case "wait", "slept", "action_failed":
			continue
		}

		// 1. Actor salience updates from high-drama micro events.
		switch ev.Kind {
		case "profession_joined", "actor_succeeded":
			addActorSalience(ev.ActorID, 15)
		case "faction_joined", "married", "ritual_performed", "threat_hunted":
			addActorSalience(ev.ActorID, 10)
			addActorSalience(ev.TargetID, 8)
		case "attacked", "raided", "robbed", "extorted", "extort_resisted", "turf_war", "vendetta_clash":
			addActorSalience(ev.ActorID, 14)
			addActorSalience(ev.TargetID, 12)
		case "arrested", "corruption_purged", "court_tried", "appeal_filed", "suspect_framed",
			"conviction_overturned", "wrongful_conviction", "granary_riot", "revolt_erupted":
			addActorSalience(ev.ActorID, 16)
			addActorSalience(ev.TargetID, 12)
		case "bribed", "loan_issued", "house_foreclosed", "feud_started", "feud_mediated":
			addActorSalience(ev.ActorID, 10)
			addActorSalience(ev.TargetID, 8)
		case "released_from_jail":
			addActorSalience(ev.ActorID, 6)
		case "speech":
			addActorSalience(ev.ActorID, 4)
			addActorSalience(ev.TargetID, 2)
		case "trade_completed":
			boost := 3
			if ev.Action.Price >= 20 || ev.Action.Quantity >= 5 {
				boost = 8
			}
			addActorSalience(ev.ActorID, boost)
			addActorSalience(ev.TargetID, maxInt(2, boost/2))
		}

		// 2. Cohort spotlight heat updates from macro events.
		switch ev.Kind {
		case "cohort_promoted":
			addCohortHeat(srPayloadString(ev.Payload, "from_cohort_id"), 15)
			addCohortHeat(srPayloadString(ev.Payload, "to_cohort_id"), 26)
			addCohortHeat(ev.ActorID, 15)
		case "cohort_corruption_purged":
			addCohortHeat(srPayloadString(ev.Payload, "corrupt_cohort"), 22)
			addCohortHeat(srPayloadString(ev.Payload, "oversight_cohort"), 18)
		case "cohort_racket_clashed":
			addCohortHeat(srPayloadString(ev.Payload, "gang_cohort"), 20)
			addCohortHeat(srPayloadString(ev.Payload, "victim_cohort"), 18)
		case "cohort_extorted", "cohort_bribed":
			addCohortHeat(srPayloadString(ev.Payload, "gang_cohort"), 10)
			addCohortHeat(srPayloadString(ev.Payload, "victim_cohort"), 8)
			addCohortHeat(ev.ActorID, 10)
			addCohortHeat(ev.TargetID, 8)
		case "cohort_raided", "cohort_raid_repelled", "cohort_turf_war", "cohort_arrested",
			"cohort_land_annexed", "cohort_loan_issued", "merchant_hoarded",
			"granary_riot", "tax_strike", "revolt_erupted", "revolt_suppressed":
			addCohortHeat(ev.ActorID, 16)
			addCohortHeat(ev.TargetID, 14)
			for _, key := range []string{"cohort_id", "gang_cohort", "victim_cohort", "rebel_cohort", "creditor_cohort", "debtor_cohort"} {
				addCohortHeat(srPayloadString(ev.Payload, key), 14)
			}
		case "threat_hunted":
			addCohortHeat(ev.ActorID, 18)
			addCohortHeat(srPayloadString(ev.Payload, "cohort_id"), 18)
		case "disaster_struck":
			dID := srEventDistrict(w, ev)
			if dID != "" {
				for i := range w.Cohorts {
					if w.Cohorts[i].District == dID {
						addCohortHeat(w.Cohorts[i].ID, 10)
					}
				}
			}
		}

		// 3. Micro -> Macro Rumor Amplification (strictly from interpersonal speech, never macro injection).
		if ev.Kind == "speech" && (ev.Action.Statement != nil || ev.Action.Text != "") {
			if panicEv, triggered := w.processRumorSpeech(sr, ev); triggered {
				emitted = append(emitted, panicEv)
			}
		}
	}

	return emitted
}

func (w *World) processRumorSpeech(sr *SpotlightRumorState, ev Event) (Event, bool) {
	var stmt Statement
	if ev.Action.Statement != nil {
		stmt = *ev.Action.Statement
	}

	speaker := w.actor(ev.ActorID)
	distID := ""
	if speaker != nil && speaker.Location != "" {
		distID = speaker.Location
	} else if stmt.Location != "" {
		distID = stmt.Location
	}
	if distID == "" {
		return Event{}, false
	}

	topic, item, sentiment := srClassifyRumorSpeech(stmt, ev.Action.Text)
	if sentiment == "reassurance" {
		for _, r := range sr.ActiveRumors {
			if r != nil && !r.PanicFired && r.District == distID && (item == "" || r.Item == item) && r.Intensity > 0 {
				r.Intensity--
			}
		}
		return Event{}, false
	}
	if sentiment != "panic" {
		return Event{}, false
	}

	// Rumor amplification anchors in public forums (hub/market/shrine) when the map defines any.
	if w.hasAnyPublicForumDistrict() && !w.isPublicForumDistrict(distID) {
		return Event{}, false
	}

	rumorKey := distID + "|" + topic + "|" + item
	r := sr.ActiveRumors[rumorKey]
	if r == nil {
		r = &ActiveRumor{
			Key:          rumorKey,
			District:     distID,
			Topic:        topic,
			Item:         item,
			FirstEventID: ev.ID,
			FirstTick:    w.Tick,
		}
		sr.ActiveRumors[rumorKey] = r
	}
	r.LastTick = w.Tick
	r.LastEventID = ev.ID

	alreadySpoke := false
	for _, s := range r.Speakers {
		if s == ev.ActorID {
			alreadySpoke = true
			break
		}
	}
	highTrust := w.isHighTrustPublicFigure(speaker)
	if !alreadySpoke {
		r.Speakers = append(r.Speakers, ev.ActorID)
		weight := 1
		if highTrust {
			weight = 2
		}
		r.Intensity += weight
	}

	if r.PanicFired {
		return Event{}, false
	}
	if len(r.Speakers) < sr.Spec.RumorCorroborationSpeakers && !(highTrust && r.Intensity >= sr.Spec.RumorCorroborationSpeakers) {
		return Event{}, false
	}

	r.PanicFired = true
	r.PanicUntil = w.Tick + sr.Spec.PanicDurationTicks
	sr.TotalPanics++
	sr.PanicUntilTick[distID] = r.PanicUntil
	if sr.PanicDemandPermille[distID] == nil {
		sr.PanicDemandPermille[distID] = map[string]int{}
	}
	curBoost := sr.PanicDemandPermille[distID][item]
	newBoost := minInt(800, curBoost+sr.Spec.PanicDemandBoostPermille)
	sr.PanicDemandPermille[distID][item] = newBoost

	moraleAfter := 0
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[distID]; ok {
			ds.MoraleBonus = maxInt(-25, ds.MoraleBonus-4)
			moraleAfter = ds.MoraleBonus
			w.DistrictStates[distID] = ds
		}
	}

	rootID := r.FirstEventID
	if rootID == 0 {
		rootID = ev.ID
	}
	panicEv := Event{
		ID:      w.NextID,
		Tick:    w.Tick,
		ActorID: "world",
		Kind:    "rumor_panic_triggered",
		Action: Action{
			Kind:         "speak",
			Location:     distID,
			Item:         item,
			BasisEventID: ev.ID,
			Statement:    &Statement{Topic: topic, Item: item, Location: distID, Quantity: len(r.Speakers)},
		},
		Payload: map[string]any{
			"text":                  fmt.Sprintf("Rumor panic erupted in %s over %s (%s): %d speakers corroborated; panic hoarding +%d‰, district morale=%d", distID, topic, item, len(r.Speakers), newBoost, moraleAfter),
			"district":              distID,
			"topic":                 topic,
			"item":                  item,
			"speakers":              append([]string(nil), r.Speakers...),
			"speaker_count":         len(r.Speakers),
			"demand_boost_permille": newBoost,
			"morale_bonus":          moraleAfter,
			"panic_until":           r.PanicUntil,
			"basis_event_id":        ev.ID,
			"root_event_id":         rootID,
		},
	}
	w.NextID++
	return panicEv, true
}

// StepDynamicSpotlight runs once per day when macro cohorts exist, decaying stale salience/heat,
// rotating low-salience eligible hydrated actors back into their cohorts (spotlight_demoted), and
// hydrating top-heat emergent cohort figures into focal actors (spotlight_promoted).
// It strictly preserves all 6 conservation ledgers (Population, Wounds, Epidemics, Jail, Treasury, TotalCoins).
func (w *World) StepDynamicSpotlight(sr *SpotlightRumorState) []Event {
	if w == nil || sr == nil || len(w.Cohorts) == 0 {
		return nil
	}
	sr.ensureInit()
	if w.Tick <= 0 || w.Tick%TicksPerDay != 0 {
		return nil
	}
	spec := sr.Spec

	// 1. Daily decay of salience and cohort heat; prune dead/missing actors from tracking maps.
	livingSet := make(map[string]bool, len(w.Actors))
	livingCount := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			livingSet[w.Actors[i].ID] = true
			livingCount++
		}
	}
	for id, val := range sr.ActorSalience {
		if !livingSet[id] {
			delete(sr.ActorSalience, id)
			continue
		}
		decayed := val - maxInt(2, val/5)
		if decayed <= 0 {
			delete(sr.ActorSalience, id)
		} else {
			sr.ActorSalience[id] = decayed
		}
	}
	for id := range sr.HydratedCohort {
		if !livingSet[id] {
			delete(sr.HydratedCohort, id)
			delete(sr.HydratedTick, id)
		}
	}
	for cid, val := range sr.CohortHeat {
		decayed := val - maxInt(2, val/4)
		if decayed <= 0 {
			delete(sr.CohortHeat, cid)
		} else {
			sr.CohortHeat[cid] = decayed
		}
	}

	// Build ownership and open-offer indices to guarantee zero-error conservation on dehydration.
	houseOwnedBy := make(map[string]bool, len(w.Houses))
	for _, h := range w.Houses {
		if h.OwnerID != "" {
			houseOwnedBy[h.OwnerID] = true
		}
	}
	actorHasOffer := make(map[string]bool, len(w.Offers)*2)
	for _, o := range w.Offers {
		actorHasOffer[o.Seller] = true
		actorHasOffer[o.Buyer] = true
	}

	// Identify hot cohorts eligible for promotion.
	type hotCohort struct {
		id   string
		heat int
		tier int
		lv   int
		pop  int
	}
	var hotCohorts []hotCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() < spec.MinCohortPopulation {
			continue
		}
		heat := sr.CohortHeat[c.ID]
		if heat <= 0 {
			continue
		}
		hotCohorts = append(hotCohorts, hotCohort{
			id:   c.ID,
			heat: heat,
			tier: c.Tier,
			lv:   c.ProfLevel,
			pop:  c.Population,
		})
	}
	sort.Slice(hotCohorts, func(i, j int) bool {
		if hotCohorts[i].heat != hotCohorts[j].heat {
			return hotCohorts[i].heat > hotCohorts[j].heat
		}
		if hotCohorts[i].tier != hotCohorts[j].tier {
			return hotCohorts[i].tier > hotCohorts[j].tier
		}
		if hotCohorts[i].lv != hotCohorts[j].lv {
			return hotCohorts[i].lv > hotCohorts[j].lv
		}
		if hotCohorts[i].pop != hotCohorts[j].pop {
			return hotCohorts[i].pop > hotCohorts[j].pop
		}
		return hotCohorts[i].id < hotCohorts[j].id
	})

	var events []Event

	// 2. Rotate out low-salience eligible hydrated actors when above target focal cap
	// or at MaxFocalActors while hot cohorts await promotion.
	type demoteCand struct {
		id           string
		name         string
		location     string
		role         string
		salience     int
		hydratedTick int64
	}
	var demoteCands []demoteCand
	for i := range w.Actors {
		a := w.Actors[i]
		if !w.canDehydrateSpotlightActor(sr, a, houseOwnedBy, actorHasOffer) {
			continue
		}
		demoteCands = append(demoteCands, demoteCand{
			id:           a.ID,
			name:         a.Name,
			location:     a.Location,
			role:         a.Role,
			salience:     sr.ActorSalience[a.ID],
			hydratedTick: sr.HydratedTick[a.ID],
		})
	}
	sort.Slice(demoteCands, func(i, j int) bool {
		if demoteCands[i].salience != demoteCands[j].salience {
			return demoteCands[i].salience < demoteCands[j].salience
		}
		if demoteCands[i].hydratedTick != demoteCands[j].hydratedTick {
			return demoteCands[i].hydratedTick < demoteCands[j].hydratedTick
		}
		return demoteCands[i].id < demoteCands[j].id
	})

	demoted := 0
	for _, cand := range demoteCands {
		if demoted >= spec.DailyDemotionCap {
			break
		}
		needRoom := livingCount > spec.TargetFocalActors || (livingCount >= spec.MaxFocalActors && len(hotCohorts) > 0)
		if !needRoom {
			break
		}
		sourceCohort := sr.HydratedCohort[cand.id]
		if err := w.DehydrateActor(cand.id); err != nil {
			continue
		}
		// Clean up dangling member/resident references left by DehydrateActor.
		for hi := range w.Houses {
			w.Houses[hi].Residents = removeString(w.Houses[hi].Residents, cand.id)
		}
		for fi := range w.Factions {
			w.Factions[fi].Members = removeString(w.Factions[fi].Members, cand.id)
		}
		delete(sr.ActorSalience, cand.id)
		delete(sr.HydratedCohort, cand.id)
		delete(sr.HydratedTick, cand.id)
		sr.TotalDemoted++
		livingCount--
		demoted++

		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  cand.id,
			TargetID: sourceCohort,
			Kind:     "spotlight_demoted",
			Action:   Action{Kind: "wait", Location: cand.location},
			Payload: map[string]any{
				"text":       fmt.Sprintf("%s stepped out of the spotlight back into cohort %s (%s, salience=%d)", cand.name, cand.role, cand.location, cand.salience),
				"actor_id":   cand.id,
				"cohort_id":  sourceCohort,
				"salience":   cand.salience,
				"district":   cand.location,
				"profession": cand.role,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 3. Hydrate top-heat emergent cohort figures up to MaxFocalActors.
	promoted := 0
	for _, hc := range hotCohorts {
		if promoted >= spec.DailyPromotionCap || livingCount >= spec.MaxFocalActors {
			break
		}
		// Re-verify cohort effective population in case a prior demotion/hydration mutated slice.
		var cohortPtr *PopulationCohort
		for i := range w.Cohorts {
			if w.Cohorts[i].ID == hc.id {
				cohortPtr = &w.Cohorts[i]
				break
			}
		}
		if cohortPtr == nil || cohortPtr.EffectivePopulation() < spec.MinCohortPopulation {
			continue
		}

		actor, err := w.HydrateActor(hc.id)
		if err != nil || actor == nil {
			continue
		}
		evID := w.NextID
		w.NextID++

		vividGoal := fmt.Sprintf("Emerged from %s (heat %d) in %s to lead %s affairs and speak at the hub", cohortPtr.Name, hc.heat, actor.Location, cohortPtr.Profession)
		actor.Goal = vividGoal

		sr.HydratedCohort[actor.ID] = hc.id
		sr.HydratedTick[actor.ID] = w.Tick
		sr.ActorSalience[actor.ID] = maxInt(12, minInt(60, hc.heat))
		sr.CohortHeat[hc.id] = sr.CohortHeat[hc.id] / 2
		sr.TotalPromoted++
		livingCount++
		promoted++

		ev := Event{
			ID:       evID,
			Tick:     w.Tick,
			ActorID:  actor.ID,
			TargetID: hc.id,
			Kind:     "spotlight_promoted",
			Action:   Action{Kind: "wait", Location: actor.Location, ProfessionID: cohortPtr.Profession},
			Payload: map[string]any{
				"text":       fmt.Sprintf("%s rose from %s (%s) into the focal spotlight (heat=%d)", actor.Name, cohortPtr.Name, actor.Location, hc.heat),
				"actor_id":   actor.ID,
				"cohort_id":  hc.id,
				"heat":       hc.heat,
				"district":   actor.Location,
				"profession": cohortPtr.Profession,
			},
		}
		events = append(events, ev)
	}

	return events
}

func (w *World) canDehydrateSpotlightActor(sr *SpotlightRumorState, a Actor, houseOwnedBy, actorHasOffer map[string]bool) bool {
	if len(w.Cohorts) == 0 || a.Dead || a.Task != nil {
		return false
	}
	if w.IsActorPinnedFocal(a) {
		return false
	}
	if a.Health != MaxHealth || a.WoundedTracked {
		return false
	}
	if a.DiseaseID != "" {
		return false
	}
	if a.JailedUntil != 0 && w.Tick <= a.JailedUntil {
		return false
	}
	if a.IllicitGain != 0 {
		return false
	}
	if houseOwnedBy[a.ID] || actorHasOffer[a.ID] || w.actorHasActiveCreditContract(a.ID) {
		return false
	}
	if srIsCoreBaselineActor(a.ID) {
		return false
	}
	if sr.HydratedCohort[a.ID] == "" && !strings.Contains(a.ID, "_a") {
		return false
	}
	return true
}

func srIsCoreBaselineActor(id string) bool {
	// Preserve original "p01".."p30" village baseline actors.
	if len(id) == 3 && id[0] == 'p' && id[1] >= '0' && id[1] <= '9' && id[2] >= '0' && id[2] <= '9' {
		n := int(id[1]-'0')*10 + int(id[2]-'0')
		return n >= 1 && n <= 30
	}
	return false
}

func (w *World) hasAnyPublicForumDistrict() bool {
	for _, d := range w.Map.Districts {
		if w.isPublicForumDistrict(d.ID) {
			return true
		}
	}
	return false
}

func (w *World) isPublicForumDistrict(districtID string) bool {
	return w.Map.DistrictHasFeature(districtID, "hub") ||
		w.Map.DistrictHasFeature(districtID, "market") ||
		w.Map.DistrictHasFeature(districtID, "shrine") ||
		w.Map.DistrictHasFeature(districtID, "yamen") ||
		w.Map.DistrictHasFeature(districtID, "academy")
}

func (w *World) isHighTrustPublicFigure(a *Actor) bool {
	if a == nil || a.Dead {
		return false
	}
	if a.Level >= 5 {
		return true
	}
	idTags := a.IdentityTags()
	caps := w.Catalog.Capabilities(*a)
	hasAuthorityRole := w.Catalog.HasPublicTrustDuty(idTags) ||
		caps["audit_corruption"] ||
		caps["arrest"] ||
		caps["perform_ritual"] ||
		ActorHasTag(idTags, "law") ||
		ActorHasTag(idTags, "ritual") ||
		ActorHasTag(idTags, "scholar")
	if !hasAuthorityRole && a.Level < 4 {
		return false
	}
	incomingTrust := 0
	for i := range w.Actors {
		if w.Actors[i].Dead || w.Actors[i].ID == a.ID {
			continue
		}
		if t := w.Actors[i].Trust[a.ID]; t > 0 {
			incomingTrust += t
		}
	}
	return incomingTrust >= 3
}

// srClassifyRumorSpeech evaluates both structured Statement fields and natural-language Text
// polarity so refutations/reassurances (e.g. "没有短缺，粮食充足，不要恐慌" or "no shortage, don't panic")
// are classified as "reassurance" rather than inverted into "panic".
func srClassifyRumorSpeech(stmt Statement, text string) (topic, item, sentiment string) {
	t := strings.ToLower(strings.TrimSpace(stmt.Topic))
	item = strings.TrimSpace(stmt.Item)
	if item == "" {
		item = "food"
	}

	affirmCrisis, reassureCount, textTopic := srAnalyzeSpeechCrisisPolarity(text)

	switch t {
	case "reassurance", "refutation", "calm", "abundant", "surplus", "no_shortage", "stable_supply":
		if affirmCrisis == 0 {
			return "reassurance", item, "reassurance"
		}
	case "supply", "item_produced":
		if stmt.Quantity > 0 && affirmCrisis == 0 {
			if reassureCount > 0 {
				return "reassurance", item, "reassurance"
			}
			return "", item, "neutral"
		}
	}

	if affirmCrisis == 0 && reassureCount > 0 {
		return "reassurance", item, "reassurance"
	}

	switch t {
	case "shortage", "scarcity", "famine", "food_shortage", "water_shortage",
		"hazard", "disaster", "plague", "epidemic", "panic", "hoarding",
		"threat", "scandal", "revolt", "supply_crisis", "tax_surcharge":
		return t, item, "panic"
	}
	for _, kw := range []string{"shortage", "scarcity", "hazard", "panic", "crisis", "famine", "plague", "disaster", "scandal", "revolt", "hoard"} {
		if strings.Contains(t, kw) && !strings.Contains(t, "no_") && !strings.Contains(t, "anti_") {
			return t, item, "panic"
		}
	}
	if t == "item_produced" && stmt.Quantity <= 0 && strings.TrimSpace(t) != "" {
		return "shortage", item, "panic"
	}
	if affirmCrisis > 0 {
		if textTopic == "" {
			textTopic = "shortage"
		}
		return textTopic, item, "panic"
	}
	return "", item, "neutral"
}

func srSplitSpeechClauses(s string) []string {
	seps := func(r rune) bool {
		switch r {
		case ',', '.', ';', '!', '?', ':', '\n', '，', '。', '；', '！', '？', '：', '、':
			return true
		default:
			return false
		}
	}
	raw := strings.FieldsFunc(strings.ToLower(s), seps)
	out := make([]string, 0, len(raw))
	for _, cl := range raw {
		cl = strings.TrimSpace(cl)
		if cl != "" {
			out = append(out, cl)
		}
	}
	return out
}

func srPrefixHasNegator(prefix string) bool {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return false
	}
	for _, zhNeg := range []string{
		"没有", "并无", "并非", "毫无", "绝无", "未见", "未有", "未发", "未曾", "不曾",
		"不是", "不会", "不要", "莫要", "不必", "无需", "无须", "切勿", "勿要", "别", "莫", "勿", "无", "不", "没",
	} {
		if strings.Contains(p, zhNeg) {
			return true
		}
	}
	padded := " " + p + " "
	for _, enNeg := range []string{
		" no ", " not ", " never ", " without ", " zero ", " neither ", " nor ",
		" don't ", " dont ", " doesn't ", " doesnt ", " didn't ", " didnt ",
		" isn't ", " isnt ", " aren't ", " arent ", " wasn't ", " wasnt ",
		" won't ", " wont ", " can't ", " cant ", " cannot ", " free of ", " false ", " unfounded ",
	} {
		if strings.Contains(padded, enNeg) {
			return true
		}
	}
	return false
}

func srAnalyzeSpeechCrisisPolarity(text string) (affirmCrisis int, reassure int, inferredTopic string) {
	if strings.TrimSpace(text) == "" {
		return 0, 0, ""
	}
	clauses := srSplitSpeechClauses(text)

	type crisisTerm struct {
		kw    string
		topic string
	}
	crisisTerms := []crisisTerm{
		{kw: "短缺", topic: "shortage"},
		{kw: "缺粮", topic: "shortage"},
		{kw: "断粮", topic: "shortage"},
		{kw: "匮乏", topic: "shortage"},
		{kw: "饥荒", topic: "famine"},
		{kw: "灾荒", topic: "famine"},
		{kw: "灾情", topic: "disaster"},
		{kw: "瘟疫", topic: "plague"},
		{kw: "疫病", topic: "plague"},
		{kw: "恐慌", topic: "shortage"},
		{kw: "暴乱", topic: "revolt"},
		{kw: "哗变", topic: "revolt"},
		{kw: "囤积", topic: "hoarding"},
		{kw: "哄抢", topic: "shortage"},
		{kw: "shortage", topic: "shortage"},
		{kw: "scarcity", topic: "shortage"},
		{kw: "scarce", topic: "shortage"},
		{kw: "drying up", topic: "shortage"},
		{kw: "running out", topic: "shortage"},
		{kw: "famine", topic: "famine"},
		{kw: "starving", topic: "famine"},
		{kw: "plague", topic: "plague"},
		{kw: "epidemic", topic: "plague"},
		{kw: "disaster", topic: "disaster"},
		{kw: "panic", topic: "shortage"},
		{kw: "crisis", topic: "shortage"},
		{kw: "revolt", topic: "revolt"},
	}

	abundanceTerms := []string{
		"充足", "充裕", "丰足", "充沛", "富足", "有余", "满仓", "供应正常", "供应平稳", "物价平稳",
		"plenty", "abundant", "sufficient", "ample", "enough", "well-stocked", "well stocked", "full granary",
	}

	refutationFraming := []string{
		"辟谣", "纯属谣言", "系谣言", "是谣言", "皆为谣言", "不实", "假消息", "切勿轻信", "莫信谣",
		"不缺", "未缺", "无虞", "无忧", "莫慌", "勿慌", "别慌", "不用慌", "人心安定",
		"false rumor", "unfounded rumor", "mere rumor", "debunk", "stay calm", "remain calm",
	}

	for _, cl := range clauses {
		clauseRefuted := false
		for _, rf := range refutationFraming {
			if strings.Contains(cl, rf) {
				reassure++
				clauseRefuted = true
			}
		}

		matchedCrisisInClause := false
		for _, ct := range crisisTerms {
			idx := strings.Index(cl, ct.kw)
			if idx < 0 {
				continue
			}
			matchedCrisisInClause = true
			prefix := cl[:idx]
			suffix := cl[idx+len(ct.kw):]
			postRefuted := strings.Contains(suffix, "是谣言") || strings.Contains(suffix, "属谣言") ||
				strings.Contains(suffix, "不实") || strings.Contains(suffix, "is false") || strings.Contains(suffix, "is a rumor")
			if clauseRefuted || postRefuted || srPrefixHasNegator(prefix) {
				reassure++
			} else {
				affirmCrisis++
				if inferredTopic == "" {
					inferredTopic = ct.topic
				}
			}
		}

		if !matchedCrisisInClause {
			for _, ab := range abundanceTerms {
				idx := strings.Index(cl, ab)
				if idx < 0 {
					continue
				}
				prefix := cl[:idx]
				if srPrefixHasNegator(prefix) {
					affirmCrisis++
					if inferredTopic == "" {
						inferredTopic = "shortage"
					}
				} else {
					reassure++
				}
			}
		}
	}
	return affirmCrisis, reassure, inferredTopic
}

func srEventDistrict(w *World, ev Event) string {
	if d := srPayloadString(ev.Payload, "district"); d != "" {
		return d
	}
	if ev.Action.Location != "" {
		return ev.Action.Location
	}
	if a := w.actor(ev.ActorID); a != nil && a.Location != "" {
		return a.Location
	}
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == ev.ActorID || w.Cohorts[i].ID == ev.TargetID {
			return w.Cohorts[i].District
		}
	}
	return ""
}

func srPayloadString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

package sim

import (
	"fmt"
	"maps"
	"math"
	"slices"
)

// NeedKey identifies a need and, when applicable, the object it is about.
type NeedKey struct {
	Measure string `json:"measure"`
	Subject string `json:"subject,omitempty"`
}

// NeedRule maps a quantity in world units to a bounded region of need value.
// Scale is the shortfall that corresponds to one unit of unweighted loss.
type NeedRule struct {
	NeedKey
	Target           float64 `json:"target"`
	Scale            float64 `json:"scale"`
	BaseWeight       float64 `json:"base_weight"`
	Volatility       float64 `json:"volatility,omitempty"`
	CorrelationTicks float64 `json:"correlation_ticks,omitempty"`
	Plasticity       float64 `json:"plasticity,omitempty"`
	Minimize         bool    `json:"minimize,omitempty"`
}

type ExperienceRule struct {
	Event         string   `json:"event"`
	Need          NeedKey  `json:"need"`
	Bias          float64  `json:"bias,omitempty"`
	RecoveryTicks float64  `json:"recovery_ticks,omitempty"`
	NewBaseWeight *float64 `json:"new_base_weight,omitempty"`
}

type PlanningRules struct {
	HorizonTicks         int64            `json:"horizon_ticks"`
	InertiaMargin        float64          `json:"inertia_margin"`
	DiscountRate         float64          `json:"discount_rate,omitempty"`
	OfferAcceptancePrior float64          `json:"offer_acceptance_prior,omitempty"`
	HouseholdSupplyTicks int64            `json:"household_supply_ticks,omitempty"`
	Needs                []NeedRule       `json:"needs"`
	Experiences          []ExperienceRule `json:"experiences,omitempty"`
}

// DefaultPlanningRules defines the initial value scales for the ordinary
// village. World packs can supply their own need definitions and value scales.
func DefaultPlanningRules() PlanningRules {
	return PlanningRules{
		HorizonTicks:         TicksPerDay,
		InertiaMargin:        0.05,
		OfferAcceptancePrior: 0.35,
		Needs: []NeedRule{
			{NeedKey: NeedKey{Measure: "nutrition"}, Target: 2, Scale: 4, BaseWeight: 3},
			{NeedKey: NeedKey{Measure: "hydration"}, Target: 0, Scale: 3, BaseWeight: 2},
			{NeedKey: NeedKey{Measure: "energy"}, Target: 65, Scale: 40, BaseWeight: 2},
			{NeedKey: NeedKey{Measure: "health"}, Target: 100, Scale: 40, BaseWeight: 2},
			{NeedKey: NeedKey{Measure: "hygiene"}, Target: 0, Scale: 5, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "provisions"}, Target: 8, Scale: 8, BaseWeight: 1},
			{NeedKey: NeedKey{Measure: "money"}, Target: 40, Scale: 40, BaseWeight: 1.2},
			{NeedKey: NeedKey{Measure: "housing"}, Target: 80, Scale: 50, BaseWeight: 1.4},
			{NeedKey: NeedKey{Measure: "stock"}, Target: 2, Scale: 2, BaseWeight: 0.6},
			{NeedKey: NeedKey{Measure: "progress"}, Target: 1, Scale: 1, BaseWeight: 0.7},
			{NeedKey: NeedKey{Measure: "obligation"}, Target: 1, Scale: 1, BaseWeight: 2},
			{NeedKey: NeedKey{Measure: "relationship"}, Target: 20, Scale: 40, BaseWeight: 0.15},
			{NeedKey: NeedKey{Measure: "skill"}, Scale: 5, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "profession_xp"}, Scale: 5, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "power_tier"}, Scale: 1, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "market_trade"}, Scale: 1, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "ambition"}, Scale: 1, BaseWeight: 0.4},
			{NeedKey: NeedKey{Measure: "knowledge"}, Target: 1, Scale: 1, BaseWeight: 0.45},
			{NeedKey: NeedKey{Measure: "safety"}, Target: 0, Scale: 10, BaseWeight: 1},
		},
	}
}

func (c Catalog) EffectivePlanningRules() PlanningRules {
	if c.Planning != nil {
		return *c.Planning
	}
	return DefaultPlanningRules()
}

func (r PlanningRules) Validate() error {
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	if r.HorizonTicks <= 0 || r.HouseholdSupplyTicks < 0 || !finite(r.InertiaMargin) || r.InertiaMargin < 0 ||
		!finite(r.DiscountRate) || r.DiscountRate < 0 || !finite(r.OfferAcceptancePrior) ||
		r.OfferAcceptancePrior < 0 || r.OfferAcceptancePrior > 1 {
		return fmt.Errorf("planning requires a positive horizon, finite nonnegative inertia and discount, and a probability between zero and one")
	}
	seen := map[NeedKey]bool{}
	for _, need := range r.Needs {
		switch need.Measure {
		case "nutrition", "hydration", "energy", "health", "hygiene", "provisions", "money", "housing",
			"stock", "progress", "obligation", "relationship", "skill", "safety", "state_slot", "knowledge", "power_tier",
			"profession_xp", "market_trade", "ambition", "conversation":
		default:
			return fmt.Errorf("unknown need measure %q", need.Measure)
		}
		if seen[need.NeedKey] {
			return fmt.Errorf("duplicate need %+v", need.NeedKey)
		}
		seen[need.NeedKey] = true
		if !finite(need.Target) || !finite(need.Scale) || need.Scale <= 0 ||
			!finite(need.BaseWeight) || need.BaseWeight < 0 ||
			!finite(need.Volatility) || need.Volatility < 0 ||
			!finite(need.CorrelationTicks) || need.CorrelationTicks < 0 ||
			!finite(need.Plasticity) || need.Plasticity < 0 || need.Plasticity > 1 {
			return fmt.Errorf("invalid value scale or preference parameters for %+v", need.NeedKey)
		}
	}
	for _, experience := range r.Experiences {
		if !seen[experience.Need] || experience.Event == "" || !finite(experience.Bias) ||
			!finite(experience.RecoveryTicks) || experience.RecoveryTicks < 0 ||
			experience.Bias != 0 && experience.RecoveryTicks == 0 {
			return fmt.Errorf("invalid experience rule for %+v", experience.Need)
		}
		if weight := experience.NewBaseWeight; weight != nil && (!finite(*weight) || *weight < 0) {
			return fmt.Errorf("invalid experience base weight for %+v", experience.Need)
		}
	}
	return nil
}

func clonePlanningRules(r *PlanningRules) *PlanningRules {
	if r == nil {
		return nil
	}
	out := *r
	out.Needs = slices.Clone(r.Needs)
	out.Experiences = slices.Clone(r.Experiences)
	for i := range out.Experiences {
		if weight := out.Experiences[i].NewBaseWeight; weight != nil {
			value := *weight
			out.Experiences[i].NewBaseWeight = &value
		}
	}
	return &out
}

type PreferenceInfluence struct {
	EventID       int64   `json:"event_id"`
	Tick          int64   `json:"tick"`
	Strength      float64 `json:"strength"`
	RecoveryTicks float64 `json:"recovery_ticks"`
}

type PreferenceState struct {
	Need        NeedKey               `json:"need"`
	BaseWeight  float64               `json:"base_weight"`
	Fluctuation float64               `json:"fluctuation,omitempty"`
	UpdatedTick int64                 `json:"updated_tick"`
	LastEventID int64                 `json:"last_event_id,omitempty"`
	Influences  []PreferenceInfluence `json:"influences,omitempty"`
}

const PlanningVersion = 7

func (w *World) settlementRandom(tick int64, purpose string) uint64 {
	return worldHash(w.ID, tick, fmt.Sprintf("settlement:%d:%s", w.RunSeed, purpose))
}

// BehaviorState tracks a chosen strategy across multiple settled actions.
// Steps contains the actions that have not yet been started.
type BehaviorState struct {
	Goal          NeedKey        `json:"goal"`
	Strategy      string         `json:"strategy"`
	Steps         []Action       `json:"steps,omitempty"`
	Paused        *BehaviorState `json:"paused,omitempty"`
	ObservedEvent int64          `json:"observed_event,omitempty"`
	ReviewAt      int64          `json:"review_at"`
	ReviewReason  string         `json:"review_reason,omitempty"`
	Version       int64          `json:"version"`
}

// BehaviorDecision is committed with a selected action. Replay applies the
// same state transition, including the preference update used for scoring.
type BehaviorDecision struct {
	Behavior    BehaviorState     `json:"behavior"`
	Preferences []PreferenceState `json:"preferences,omitempty"`
	Facts       []KnownFact       `json:"facts,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Score       float64           `json:"score,omitempty"`
	RoutineCue  string            `json:"routine_cue,omitempty"`
}

type HabitState struct {
	Cue       string `json:"cue"`
	Action    Action `json:"action"`
	Successes int64  `json:"successes"`
	EventID   int64  `json:"event_id"`
}

func (a *Actor) learnHabit(cue string, action Action, event Event) {
	if a.Habits == nil {
		a.Habits = make(map[string]HabitState)
	}
	previous := a.Habits[action.Kind]
	if previous.Cue != cue || previous.Action.Item != action.Item {
		previous = HabitState{Cue: cue, Action: cloneAction(action)}
	}
	previous.Successes++
	previous.EventID = event.ID
	a.Habits[action.Kind] = previous
}

type KnownFact struct {
	Subject    string `json:"subject"`
	Attribute  string `json:"attribute"`
	Value      string `json:"value,omitempty"`
	Quantity   int    `json:"quantity,omitempty"`
	EventID    int64  `json:"event_id,omitempty"`
	ObservedAt int64  `json:"observed_at"`
	Source     string `json:"source"`
}

func (a *Actor) rememberFact(fact KnownFact) {
	if a.KnownFacts == nil {
		a.KnownFacts = make(map[string]KnownFact)
	}
	a.KnownFacts[fact.Attribute+":"+fact.Subject] = fact
}

// ObservedPaymentFailure is evidence about one price, obtained by trying to
// collect payment. A lower price can still be viable.
func (p Perception) ObservedPaymentFailure(buyer string, price int) bool {
	fact, known := p.Self.KnownFacts["coin_payment_unavailable:"+buyer]
	return known && price >= fact.Quantity
}

func (w *World) observePaymentEvidence(e Event) {
	switch e.Kind {
	case "action_failed":
		observation, ok := e.Payload["observation"].(Observation)
		if e.Action.Kind == "fulfill_agreement" && ok && observation.Kind == "coin_payment_unavailable" {
			if supplier := w.actor(e.ActorID); supplier != nil {
				supplier.rememberFact(KnownFact{Subject: observation.ObjectID, Attribute: observation.Kind,
					Quantity: observation.Quantity, EventID: e.ID, ObservedAt: e.Tick, Source: observation.Modality})
			}
		}
	case "trade_completed", "agreement_fulfilled":
		if (e.Action.Currency == "" || e.Action.Currency == "coins") &&
			payloadInt64(e.Payload, "price") > payloadInt64(e.Payload, "market_tax") {
			buyerID, _ := e.Payload["buyer_id"].(string)
			sellerID, _ := e.Payload["seller_id"].(string)
			if buyer := w.actor(buyerID); buyer != nil {
				delete(buyer.KnownFacts, "coin_payment_unavailable:"+sellerID)
			}
		}
	}
}

func (p Perception) LocalPlanningFacts() []KnownFact {
	if p.Self.InTransit() {
		return nil
	}
	var facts []KnownFact
	if p.DistrictState != nil {
		water := "empty"
		if p.HasPotableWater() {
			water = "available"
		}
		facts = append(facts,
			KnownFact{Subject: p.Self.Location, Attribute: "weather", Value: p.DistrictState.Weather, ObservedAt: p.Tick, Source: "local_observation"},
			KnownFact{Subject: p.Self.Location, Attribute: "potable_water", Value: water, ObservedAt: p.Tick, Source: "local_observation"},
		)
	}
	if p.CurrentDistrict.ID != "" && ActorHasTag(p.CurrentDistrict.Features, "market") {
		facts = append(facts, KnownFact{Subject: p.Self.Location, Attribute: "market_visited",
			ObservedAt: p.Tick, Source: "local_observation"})
	}
	if house := p.CurrentHouse; house != nil {
		facts = append(facts,
			KnownFact{Subject: house.ID, Attribute: "shelter_condition", Quantity: house.Condition, ObservedAt: p.Tick, Source: "local_observation"},
			KnownFact{Subject: house.ID, Attribute: "shelter_comfort", Quantity: house.Comfort, ObservedAt: p.Tick, Source: "local_observation"},
			KnownFact{Subject: house.ID, Attribute: "shelter_level", Quantity: house.Level, ObservedAt: p.Tick, Source: "local_observation"},
		)
	}
	return facts
}

// ObservedShelter returns local conditions or the actor's last observation.
func (p Perception) ObservedShelter(id string) *House {
	for i := range p.AccessibleHouses {
		if p.AccessibleHouses[i].ID == id {
			return &p.AccessibleHouses[i]
		}
	}
	condition, sawCondition := p.Self.KnownFacts["shelter_condition:"+id]
	comfort, sawComfort := p.Self.KnownFacts["shelter_comfort:"+id]
	level, sawLevel := p.Self.KnownFacts["shelter_level:"+id]
	if !sawCondition || !sawComfort || !sawLevel {
		return nil
	}
	return &House{ID: id, Condition: condition.Quantity, Comfort: comfort.Quantity, Level: level.Quantity}
}

func (p Perception) HasPotableWater() bool {
	return p.DistrictState != nil && (&World{Catalog: p.Catalog}).potablePoolID(*p.DistrictState) != ""
}

// KnownWaterSources returns districts with a published source or a personal
// observation of water. A visit can show that a published source has run dry.
func (p Perception) KnownWaterSources() []string {
	var sources []string
	for _, district := range p.Map.Districts {
		if district.ID == p.Self.Location {
			continue
		}
		if fact, known := p.Self.KnownFacts["potable_water:"+district.ID]; known {
			if fact.Value == "available" {
				sources = append(sources, district.ID)
			}
			continue
		}
		for _, pool := range p.Catalog.Resources {
			if pool.PotableWater && slices.ContainsFunc(pool.NodeFeatures, func(feature string) bool {
				return p.Map.DistrictHasFeature(district.ID, feature)
			}) {
				sources = append(sources, district.ID)
				break
			}
		}
	}
	return sources
}

func cloneBehavior(s BehaviorState) BehaviorState {
	s.Steps = slices.Clone(s.Steps)
	for i := range s.Steps {
		s.Steps[i] = cloneAction(s.Steps[i])
	}
	if s.Paused != nil {
		paused := cloneBehavior(*s.Paused)
		s.Paused = &paused
	}
	return s
}

func clonePreferences(states []PreferenceState) []PreferenceState {
	copy := slices.Clone(states)
	for i := range copy {
		copy[i].Influences = slices.Clone(copy[i].Influences)
	}
	return copy
}

func cloneDecision(d *BehaviorDecision) *BehaviorDecision {
	if d == nil {
		return nil
	}
	copy := *d
	copy.Behavior = cloneBehavior(d.Behavior)
	copy.Preferences = clonePreferences(d.Preferences)
	copy.Facts = slices.Clone(d.Facts)
	return &copy
}

func (a *Actor) applyBehaviorDecision(d *BehaviorDecision) {
	if d != nil {
		behavior := cloneBehavior(d.Behavior)
		a.Behavior = &behavior
		a.Preferences = clonePreferences(d.Preferences)
		for _, fact := range d.Facts {
			a.rememberFact(fact)
		}
	}
}

func (a *Actor) cloneBehaviorState() {
	if a.InertiaMargin != nil {
		margin := *a.InertiaMargin
		a.InertiaMargin = &margin
	}
	if a.NextWake != nil {
		wake := *a.NextWake
		a.NextWake = &wake
	}
	if a.Behavior != nil {
		behavior := cloneBehavior(*a.Behavior)
		a.Behavior = &behavior
	}
	a.Preferences = clonePreferences(a.Preferences)
	a.NeedWeights = maps.Clone(a.NeedWeights)
	a.KnownFacts = maps.Clone(a.KnownFacts)
	a.Habits = maps.Clone(a.Habits)
	for kind, habit := range a.Habits {
		habit.Action = cloneAction(habit.Action)
		a.Habits[kind] = habit
	}
}

func decisionObservations(history []Memory, a Actor, rules *PlanningRules) (int64, []Memory, bool) {
	var version int64
	var reactions []Memory
	if len(history) > 0 {
		version = history[len(history)-1].EventID
	}
	changed := false
	var after int64
	if a.Behavior != nil {
		after = a.Behavior.ObservedEvent
	}
	for i := len(history) - 1; i >= 0 && history[i].EventID > after; i-- {
		memory := history[i]
		if a.Behavior != nil && mattersToBehavior(a.ID, memory) {
			changed = true
		}
		if rules == nil {
			if a.Behavior == nil || changed {
				break
			}
			continue
		}
		for _, rule := range rules.Experiences {
			if rule.Event == memory.Kind {
				reactions = append(reactions, Memory{EventID: memory.EventID, Tick: memory.Tick, Kind: memory.Kind})
				changed = true
				break
			}
		}
	}
	slices.Reverse(reactions)
	return version, reactions, changed
}

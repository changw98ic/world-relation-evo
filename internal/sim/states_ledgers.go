package sim

import (
	"fmt"
	"sort"
)

// StateSlotSpec declaratively defines an actor numerical state gauge (e.g. qi, radiation,
// infection, battery, sanity) with bounded range, per-tick regen/drain, ambient field
// absorption from the local GeoRegion, and automatic overflow transitions.
type StateSlotSpec struct {
	ID                   string `json:"id"`
	Name                 string `json:"name,omitempty"`
	RequiredTag          string `json:"required_tag,omitempty"`
	Min                  int    `json:"min,omitempty"`
	Max                  int    `json:"max,omitempty"`
	Default              int    `json:"default,omitempty"`
	RegenPerTick         int    `json:"regen_per_tick,omitempty"`
	AmbientFieldKey      string `json:"ambient_field_key,omitempty"`
	AmbientAbsorbDivisor int    `json:"ambient_absorb_divisor,omitempty"`
	OverflowThreshold    int    `json:"overflow_threshold,omitempty"`
	OverflowTransitionID string `json:"overflow_transition_id,omitempty"`
}

// FormSpec declaratively defines an actor's physiological/ontological form or power realm
// (e.g. mortal, qi_refining, foundation, golden_core, nascent_soul, body_refiner, cyborg) with lifespan,
// power tier, combat/vigilance scaling, bidirectional intake scaling (fasting or high-consumption),
// per-form starvation overrides, minimum food tier, per-tick slot metabolism, and innate tags.
type FormSpec struct {
	ID                   string         `json:"id"`
	Name                 string         `json:"name,omitempty"`
	PowerTier            int            `json:"power_tier,omitempty"`
	CombatBonus          int            `json:"combat_bonus,omitempty"`
	VigilanceBonus       int            `json:"vigilance_bonus,omitempty"`
	LifeExpectancyYears  int            `json:"life_expectancy_years,omitempty"`
	HungerIntervalMulPct int            `json:"hunger_interval_mul_pct,omitempty"`
	DailyIntakePct       *int           `json:"daily_intake_pct,omitempty"`
	DisableHunger        bool           `json:"disable_hunger,omitempty"`
	StarvationThreshold  int            `json:"starvation_threshold,omitempty"`
	StarvationDamage     *int           `json:"starvation_damage,omitempty"`
	MinFoodTier          int            `json:"min_food_tier,omitempty"`
	SlotMetabolism       map[string]int `json:"slot_metabolism,omitempty"`
	InnateTags           []string       `json:"innate_tags,omitempty"`
}

// IsHungerDisabled reports whether this form requires zero physiological food intake
// (e.g. fasting cultivators, pure energy beings, or synthetic bodies).
func (f FormSpec) IsHungerDisabled() bool {
	return f.DisableHunger || (f.DailyIntakePct != nil && *f.DailyIntakePct <= 0)
}

// EffectiveDailyIntakePct returns the forward daily food intake percentage relative to a
// standard mortal (100 = 1.0x, 300 = 3.0x higher intake, 35 = 0.35x lower intake, 0 = disabled).
func (f FormSpec) EffectiveDailyIntakePct() int {
	if f.IsHungerDisabled() {
		return 0
	}
	if f.DailyIntakePct != nil {
		return *f.DailyIntakePct
	}
	if f.HungerIntervalMulPct > 0 {
		pct := (10000 + f.HungerIntervalMulPct/2) / f.HungerIntervalMulPct
		if pct < 1 {
			pct = 1
		}
		return pct
	}
	return 100
}

// MeetsMinFoodTier reports whether spec meets this form's minimum food tier (if any).
func (f FormSpec) MeetsMinFoodTier(spec ItemSpec) bool {
	return f.MinFoodTier <= 0 || spec.Tier >= f.MinFoodTier
}

// AcceptsFoodItem reports whether an edible item provides nutrition and satisfies this form's minimum food tier.
func (f FormSpec) AcceptsFoodItem(spec ItemSpec) bool {
	return spec.Edible && spec.NutritionUnits > 0 && f.MeetsMinFoodTier(spec)
}

// TransitionSpec declaratively defines a form/state transition (e.g. realm breakthrough,
// cybernetic augmentation, or infection/radiation mutation) with gating, costs,
// Spirit-scaled success probability, and configurable failure memory/hostility effects.
type TransitionSpec struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name,omitempty"`
	RequiredFormID      string         `json:"required_form_id,omitempty"`
	RequiredSlots       map[string]int `json:"required_slots,omitempty"`
	RequiredSkills      map[string]int `json:"required_skills,omitempty"`
	ItemCosts           map[string]int `json:"item_costs,omitempty"`
	SlotCosts           map[string]int `json:"slot_costs,omitempty"`
	Ticks               int            `json:"ticks,omitempty"`
	BaseSuccessPermille int            `json:"base_success_permille,omitempty"`
	SpiritBonusPermille int            `json:"spirit_bonus_permille,omitempty"`
	SuccessFormID       string         `json:"success_form_id,omitempty"`
	SuccessGrantTags    []string       `json:"success_grant_tags,omitempty"`
	RewardAttributes    map[string]int `json:"reward_attributes,omitempty"`
	FailureFormID       string         `json:"failure_form_id,omitempty"`
	FailureHealthLoss   int            `json:"failure_health_loss,omitempty"`
	FailureWipeMemories bool           `json:"failure_wipe_memories,omitempty"`
	FailureHostileTag   string         `json:"failure_hostile_tag,omitempty"`
}

// CurrencySpec declaratively defines an economic currency medium (e.g. spirit_credit,
// sect_merit, syndicate_scrip) distinct from mortal "coins".
type CurrencySpec struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	BackingItem string `json:"backing_item,omitempty"`
	PolityID    string `json:"polity_id,omitempty"`
}

// FlowNode represents a node in the topological power/energy/leyline FlowGrid.
type FlowNode struct {
	EnergyCapacity int64         `json:"energy_capacity"`
	Source         *EnergySource `json:"source,omitempty"`
	StoredEnergy   int64         `json:"stored_energy"`
	FuelStock      int64         `json:"fuel_stock,omitempty"`
	NextSupplyTick int64         `json:"next_supply_tick,omitempty"`

	ID         string `json:"id"`
	VenueID    string `json:"venue_id,omitempty"`
	DistrictID string `json:"district_id,omitempty"`
	Kind       string `json:"kind"` // "generator" | "relay" | "consumer"
	Generation int    `json:"generation,omitempty"`
	Demand     int    `json:"demand,omitempty"`
	Active     bool   `json:"active"`
	Powered    bool   `json:"powered"`
}

// FlowEdge represents a directed transmission link between two FlowNodes.
type FlowEdge struct {
	ID           string `json:"id"`
	FromNodeID   string `json:"from_node_id"`
	ToNodeID     string `json:"to_node_id"`
	Capacity     int    `json:"capacity,omitempty"`
	LossPermille int    `json:"loss_permille,omitempty"`
	Severed      bool   `json:"severed,omitempty"`
}

// FlowGridState stores the topological power/flow network for powered venues and facilities.
type FlowGridState struct {
	Nodes []FlowNode `json:"nodes,omitempty"`
	Edges []FlowEdge `json:"edges,omitempty"`
}

// CloneFlowGridState returns a deep copy of g while preserving nil slices and pointers.
func CloneFlowGridState(g *FlowGridState) *FlowGridState {
	if g == nil {
		return nil
	}
	out := &FlowGridState{}
	if g.Nodes != nil {
		out.Nodes = append([]FlowNode(nil), g.Nodes...)
		for i := range out.Nodes {
			if g.Nodes[i].Source != nil {
				source := *g.Nodes[i].Source
				out.Nodes[i].Source = &source
			}
		}
	}
	if g.Edges != nil {
		out.Edges = append([]FlowEdge(nil), g.Edges...)
	}
	return out
}

// ResourceLedgerTotals records exact conservation counters for a single currency or item.
type ResourceLedgerTotals struct {
	Initial  int64 `json:"initial"`
	Produced int64 `json:"produced"`
	Consumed int64 `json:"consumed"`
}

// LedgerAuditState tracks Initial, Produced, and Consumed totals per currency and per item
// so generalized conservation (Initial + Produced - Consumed == CurrentTotalHeld) can be verified.
type LedgerAuditState struct {
	Energy     ResourceLedgerTotals                       `json:"energy"`
	Ecology    map[string]map[string]ResourceLedgerTotals `json:"ecology,omitempty"`
	Currencies map[string]ResourceLedgerTotals            `json:"currencies,omitempty"`
	Items      map[string]ResourceLedgerTotals            `json:"items,omitempty"`
}

func cloneInt64Map(m map[string]int64) map[string]int64 {
	if len(m) == 0 {
		return nil
	}
	r := make(map[string]int64, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

// CloneActor isolates every mutable part of a person's state from its source.
func CloneActor(a Actor) Actor {
	a.Ambitions = append([]ActorAmbition(nil), a.Ambitions...)
	a.KnownHomicides = append([]int64(nil), a.KnownHomicides...)
	a.Tags = append([]string(nil), a.Tags...)
	a.Guilds = append([]string(nil), a.Guilds...)
	a.Hobbies = append([]string(nil), a.Hobbies...)
	a.Professions = cloneIntMap(a.Professions)
	a.ProfessionRanks = cloneStringMap(a.ProfessionRanks)
	a.ProfessionXP = cloneIntMap(a.ProfessionXP)
	a.Skills = cloneIntMap(a.Skills)
	a.SkillXP = cloneIntMap(a.SkillXP)
	a.GuildLevels = cloneIntMap(a.GuildLevels)
	a.Items = cloneIntMap(a.Items)
	a.Equipped = cloneStringMap(a.Equipped)
	a.Trust = cloneIntMap(a.Trust)
	a.StateSlots = cloneIntMap(a.StateSlots)
	a.Currencies = cloneInt64Map(a.Currencies)
	if a.Plan != nil {
		a.Plan = ClonePersonalPlan(a.Plan)
	}
	if a.Task != nil {
		tc := *a.Task
		tc.Action = cloneAction(tc.Action)
		a.Task = &tc
	}
	if a.Memories != nil {
		a.Memories = append([]Memory(nil), a.Memories...)
		for i := range a.Memories {
			m := &a.Memories[i]
			m.SourceTags = append([]string(nil), m.SourceTags...)
			if m.Observation != nil {
				observation := *m.Observation
				m.Observation = &observation
			}
			if m.Statement != nil {
				statement := *m.Statement
				m.Statement = &statement
			}
		}
	}
	return a
}

// ActorEffectiveTags returns the actor's identity tags merged with innate tags of their current FormID.
func (c Catalog) ActorEffectiveTags(a Actor) []string {
	tags := a.IdentityTags()
	if a.FormID != "" {
		if form, ok := c.Forms[a.FormID]; ok {
			for _, tag := range form.InnateTags {
				if !ActorHasTag(tags, tag) {
					tags = append(tags, tag)
				}
			}
		}
	}
	return tags
}

func (w *World) actorFormSpec(a Actor) (FormSpec, bool) {
	if a.FormID == "" || len(w.Catalog.Forms) == 0 {
		return FormSpec{}, false
	}
	form, ok := w.Catalog.Forms[a.FormID]
	return form, ok
}

func (w *World) actorCurrencyBalance(a *Actor, currency string) int64 {
	if a == nil {
		return 0
	}
	if currency == "" || currency == "coins" {
		return int64(a.Money)
	}
	return a.Currencies[currency]
}

func (w *World) addActorCurrency(a *Actor, currency string, delta int64) {
	w.adjustActorCurrency(a, currency, delta, false)
}

// adjustActorCurrency mutates an actor's currency balance (coins or custom currency) and optionally records production/consumption in LedgerAudit.
func (w *World) adjustActorCurrency(a *Actor, currency string, delta int64, trackLedger bool) {
	if a == nil || delta == 0 {
		return
	}
	if currency == "" || currency == "coins" {
		a.Money += int(delta)
		if trackLedger {
			if delta > 0 {
				w.recordLedgerCurrencyProduced("coins", delta)
			} else {
				w.recordLedgerCurrencyConsumed("coins", -delta)
			}
		}
		return
	}
	if a.Currencies == nil {
		if delta < 0 {
			if trackLedger {
				w.recordLedgerCurrencyConsumed(currency, -delta)
			}
			return
		}
		a.Currencies = map[string]int64{}
	}
	a.Currencies[currency] += delta
	if a.Currencies[currency] == 0 {
		delete(a.Currencies, currency)
	}
	if len(a.Currencies) == 0 {
		a.Currencies = nil
	}
	if trackLedger {
		if delta > 0 {
			w.recordLedgerCurrencyProduced(currency, delta)
		} else {
			w.recordLedgerCurrencyConsumed(currency, -delta)
		}
	}
}

func (w *World) addPolityCurrency(pol *PolityNode, currency string, delta int64) {
	if pol == nil || delta == 0 {
		return
	}
	if currency == "" || currency == "coins" {
		pol.Treasury += delta
		return
	}
	if pol.Currencies == nil {
		if delta < 0 {
			return
		}
		pol.Currencies = map[string]int64{}
	}
	pol.Currencies[currency] += delta
	if pol.Currencies[currency] == 0 {
		delete(pol.Currencies, currency)
	}
	if len(pol.Currencies) == 0 {
		pol.Currencies = nil
	}
}

func (w *World) addCohortCurrency(c *PopulationCohort, currency string, delta int64) {
	if c == nil || delta == 0 {
		return
	}
	if currency == "" || currency == "coins" {
		c.MoneyPool += delta
		return
	}
	if c.Currencies == nil {
		if delta < 0 {
			return
		}
		c.Currencies = map[string]int64{}
	}
	c.Currencies[currency] += delta
	if c.Currencies[currency] == 0 {
		delete(c.Currencies, currency)
	}
	if len(c.Currencies) == 0 {
		c.Currencies = nil
	}
}

// ItemAcceptsCurrency reports whether an ItemSpec can be traded using the given currency ID.
func ItemAcceptsCurrency(spec ItemSpec, currency string) bool {
	return itemAcceptsCurrency(spec, currency)
}

func itemAcceptsCurrency(spec ItemSpec, currency string) bool {
	if len(spec.AcceptedCurrencies) == 0 {
		return true
	}
	if currency == "" {
		currency = "coins"
	}
	for _, c := range spec.AcceptedCurrencies {
		if c == currency {
			return true
		}
	}
	return false
}

func clampSlotValue(val int, spec StateSlotSpec, hasSpec bool) int {
	if !hasSpec {
		if val < 0 {
			return 0
		}
		return val
	}
	if val < spec.Min {
		val = spec.Min
	}
	if spec.Max > spec.Min && val > spec.Max {
		val = spec.Max
	}
	return val
}

func setActorSlotValue(a *Actor, slotID string, val int) {
	// An explicit zero is an exhausted slot; only an absent key is uninitialized.
	if a.StateSlots == nil {
		a.StateSlots = map[string]int{}
	}
	a.StateSlots[slotID] = val
}

func (w *World) stepStateSlotsAndForms() []Event {
	if len(w.Catalog.StateSlots) == 0 && len(w.Catalog.Forms) == 0 {
		return nil
	}
	var events []Event

	slotKeys := make([]string, 0, len(w.Catalog.StateSlots))
	for k := range w.Catalog.StateSlots {
		slotKeys = append(slotKeys, k)
	}
	sort.Strings(slotKeys)

	for i := range w.Actors {
		events = append(events, w.stepActorStateSlotsAndForm(&w.Actors[i], slotKeys)...)
	}
	for ci := range w.Cohorts {
		c := &w.Cohorts[ci]
		before := append([]Actor(nil), c.Members...)
		for mi := range c.Members {
			events = append(events, w.stepActorStateSlotsAndForm(&c.Members[mi], slotKeys)...)
		}
		w.reconcileCohortMemberChanges(c, before)
	}
	return events
}

func (w *World) stepActorStateSlotsAndForm(a *Actor, slotKeys []string) []Event {
	if a.Dead {
		return nil
	}
	var events []Event
	form, hasForm := w.actorFormSpec(*a)
	var reg GeoRegion
	hasReg := false
	if regID := w.Map.GeoRegionIDForLocation(a.Location, a.VenueID); regID != "" {
		reg, hasReg = w.Map.GeoRegionByID(regID)
	}

	// Update every slot before evaluating transitions, so prerequisites never depend on key order.
	handled := make(map[string]bool, len(slotKeys))
	var overflowTransitions []string
	actorTags := w.actorWithEffectTags(*a).Tags
	for _, slotID := range slotKeys {
		if a.Dead {
			break
		}
		handled[slotID] = true
		spec := w.Catalog.StateSlots[slotID]
		if spec.RequiredTag != "" && !ActorHasTag(actorTags, spec.RequiredTag) {
			continue
		}
		cur, existed := a.StateSlots[slotID]
		if !existed && spec.Default != 0 {
			cur = spec.Default
		}
		delta := spec.RegenPerTick
		if hasForm && form.SlotMetabolism != nil {
			delta += form.SlotMetabolism[slotID]
		}
		if spec.AmbientFieldKey != "" && hasReg && reg.AmbientFields != nil {
			ambient := reg.AmbientFields[spec.AmbientFieldKey]
			if ambient != 0 {
				div := spec.AmbientAbsorbDivisor
				if div <= 0 {
					div = 1
				}
				delta += ambient / div
			}
		}

		next := cur + delta
		overflowed := spec.OverflowThreshold > 0 && next >= spec.OverflowThreshold
		clamped := clampSlotValue(next, spec, true)
		setActorSlotValue(a, slotID, clamped)

		if overflowed && spec.OverflowTransitionID != "" {
			overflowTransitions = append(overflowTransitions, spec.OverflowTransitionID)
		}
	}

	// Apply any form SlotMetabolism entries for slots not declared in Catalog.StateSlots.
	if !a.Dead && hasForm && len(form.SlotMetabolism) > 0 {
		extraKeys := make([]string, 0, len(form.SlotMetabolism))
		for k := range form.SlotMetabolism {
			if !handled[k] {
				extraKeys = append(extraKeys, k)
			}
		}
		sort.Strings(extraKeys)
		for _, slotID := range extraKeys {
			cur := a.StateSlots[slotID]
			next := cur + form.SlotMetabolism[slotID]
			if next < 0 {
				next = 0
			}
			if a.StateSlots != nil || next != 0 {
				setActorSlotValue(a, slotID, next)
			}
		}
	}
	for _, transID := range overflowTransitions {
		if a.Dead || a.Task != nil || w.Tick < a.JailedUntil {
			break
		}
		trans := w.Catalog.Transitions[transID]
		if a.FormID != "" && (a.FormID == trans.SuccessFormID || a.FormID == trans.FailureFormID) {
			continue
		}
		if w.transitionRequirementReason(*a, trans) == "" {
			x := Action{Kind: "attempt_transition", TransitionID: transID}
			events = append(events, w.applyAttemptTransition(a, x)...)
		}
	}
	return events
}

func resolveTransitionID(x Action) string {
	if x.TransitionID != "" {
		return x.TransitionID
	}
	if x.RecipeID != "" {
		return x.RecipeID
	}
	if x.ProfessionID != "" {
		return x.ProfessionID
	}
	return x.Item
}

func (w *World) applyAttemptTransition(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	transID := resolveTransitionID(x)
	trans, ok := w.Catalog.Transitions[transID]
	if !ok {
		return fail("unknown transition: " + transID)
	}
	if reason := w.transitionRequirementReason(*a, trans); reason != "" {
		return fail(reason)
	}
	costSlotKeys := make([]string, 0, len(trans.SlotCosts))
	for k := range trans.SlotCosts {
		costSlotKeys = append(costSlotKeys, k)
	}
	sort.Strings(costSlotKeys)
	costItemKeys := make([]string, 0, len(trans.ItemCosts))
	for k := range trans.ItemCosts {
		costItemKeys = append(costItemKeys, k)
	}
	sort.Strings(costItemKeys)

	// Deduct slot and item costs upfront.
	for _, slotID := range costSlotKeys {
		cost := trans.SlotCosts[slotID]
		if cost > 0 {
			spec, hasSpec := w.Catalog.StateSlots[slotID]
			next := clampSlotValue(a.StateSlots[slotID]-cost, spec, hasSpec)
			setActorSlotValue(a, slotID, next)
		}
	}
	for _, itemID := range costItemKeys {
		qty := trans.ItemCosts[itemID]
		if qty > 0 {
			w.adjustActorItem(a, itemID, -qty, true)
		}
	}

	x.TransitionID = trans.ID
	if trans.Ticks > 1 {
		task := Task{Action: x, TotalTicks: trans.Ticks, Remaining: trans.Ticks - 1, StartedTick: w.Tick}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s began transition %s (%d ticks)", a.Name, trans.ID, trans.Ticks),
			"transition_id":   trans.ID,
			"remaining_ticks": task.Remaining,
		})}
	}
	return w.executeTransition(a, x, trans.ID)
}

func (w *World) executeTransition(a *Actor, x Action, transID string) []Event {
	if a == nil {
		return nil
	}
	trans, ok := w.Catalog.Transitions[transID]
	if !ok {
		return nil
	}
	chance := trans.BaseSuccessPermille + a.Attributes.Spirit*trans.SpiritBonusPermille
	if chance < 0 {
		chance = 0
	}
	if chance > 1000 {
		chance = 1000
	}
	succeeded := false
	if chance >= 1000 {
		succeeded = true
	} else if chance > 0 {
		roll := int(worldHash(w.ID, w.Tick, "transition:"+a.ID+":"+trans.ID) % 1000)
		succeeded = roll < chance
	}

	if succeeded {
		prevForm := a.FormID
		if trans.SuccessFormID != "" {
			a.FormID = trans.SuccessFormID
		}
		for _, tag := range trans.SuccessGrantTags {
			a.Tags = appendUnique(a.Tags, tag)
		}
		for attr, delta := range trans.RewardAttributes {
			if delta == 0 {
				continue
			}
			switch attr {
			case "strength":
				a.Attributes.Strength += delta
			case "dexterity":
				a.Attributes.Dexterity += delta
			case "insight":
				a.Attributes.Insight += delta
			case "spirit":
				a.Attributes.Spirit += delta
			}
		}
		w.refreshActor(a)
		return []Event{w.record(a, x, "transition_succeeded", map[string]any{
			"text":          fmt.Sprintf("%s succeeded transition %s -> form %s", a.Name, trans.ID, a.FormID),
			"transition_id": trans.ID,
			"previous_form": prevForm,
			"form_id":       a.FormID,
			"form_name":     w.Catalog.ActorFormName(*a),
			"power_tier":    w.Catalog.ActorPowerTier(*a),
			"granted_tags":  append([]string(nil), trans.SuccessGrantTags...),
		})}
	}

	prevForm := a.FormID
	if trans.FailureHealthLoss > 0 {
		a.Health -= trans.FailureHealthLoss
		if a.Health < 0 {
			a.Health = 0
		}
	}
	if trans.FailureFormID != "" {
		a.FormID = trans.FailureFormID
	}
	if trans.FailureHostileTag != "" {
		a.Tags = appendUnique(a.Tags, trans.FailureHostileTag)
	}
	w.refreshActor(a)
	ev := w.record(a, x, "transition_failed", map[string]any{
		"text":           fmt.Sprintf("%s failed transition %s -> form %s", a.Name, trans.ID, a.FormID),
		"transition_id":  trans.ID,
		"previous_form":  prevForm,
		"form_id":        a.FormID,
		"health_loss":    trans.FailureHealthLoss,
		"memories_wiped": trans.FailureWipeMemories,
		"hostile_tag":    trans.FailureHostileTag,
	})
	if trans.FailureWipeMemories {
		a.Memories = nil
		a.Trust = map[string]int{}
	}
	events := []Event{ev}
	events = append(events, w.markDeadIfZero(a, nil, x, "transition_failure")...)
	return events
}

func (w *World) applyRefineCurrencyItem(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	itemID := x.Item
	if itemID == "" {
		keys := make([]string, 0, len(a.Items))
		for k, qty := range a.Items {
			if qty > 0 {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			spec := w.Catalog.Items[k]
			if spec.DualUseCurrency != "" && spec.DualUseRate > 0 {
				itemID = k
				break
			}
		}
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok || spec.DualUseCurrency == "" || spec.DualUseRate <= 0 {
		return fail("item cannot be refined into currency: " + itemID)
	}
	qty := x.Quantity
	if qty <= 0 {
		qty = 1
	}
	if a.Items[itemID] < qty {
		return fail(fmt.Sprintf("insufficient %s to refine (need %d, has %d)", itemID, qty, a.Items[itemID]))
	}
	w.adjustActorItem(a, itemID, -qty, true)
	minted := int64(qty) * int64(spec.DualUseRate)
	w.adjustActorCurrency(a, spec.DualUseCurrency, minted, true)

	return []Event{w.record(a, x, "currency_item_refined", map[string]any{
		"text":            fmt.Sprintf("%s refined %d %s into %d %s", a.Name, qty, itemID, minted, spec.DualUseCurrency),
		"item":            itemID,
		"item_consumed":   qty,
		"currency":        spec.DualUseCurrency,
		"currency_minted": minted,
	})}
}

func (w *World) applyAbsorbItemSlot(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	itemID := x.Item
	if itemID == "" {
		keys := make([]string, 0, len(a.Items))
		for k, qty := range a.Items {
			if qty > 0 {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			spec := w.Catalog.Items[k]
			if spec.DualUseCurrency != "" || spec.DualUseRate > 0 {
				itemID = k
				break
			}
		}
	}
	spec, ok := w.Catalog.Items[itemID]
	if !ok {
		return fail("unknown item: " + itemID)
	}
	qty := x.Quantity
	if qty <= 0 {
		qty = 1
	}
	if a.Items[itemID] < qty {
		return fail(fmt.Sprintf("insufficient %s to absorb (need %d, has %d)", itemID, qty, a.Items[itemID]))
	}
	slotID := x.TargetID
	if slotID == "" {
		slotID = x.Channel
	}
	if slotID == "" {
		slotID = x.ProfessionID
	}
	if slotID == "" {
		if _, hasQi := w.Catalog.StateSlots["qi"]; hasQi || len(w.Catalog.StateSlots) == 0 {
			slotID = "qi"
		} else {
			keys := make([]string, 0, len(w.Catalog.StateSlots))
			for k := range w.Catalog.StateSlots {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			slotID = keys[0]
		}
	}

	gainPerUnit := spec.DualUseRate
	if gainPerUnit <= 0 {
		gainPerUnit = spec.MedicinalHeal
	}
	if gainPerUnit <= 0 {
		gainPerUnit = 10
	}

	w.adjustActorItem(a, itemID, -qty, true)

	slotSpec, hasSpec := w.Catalog.StateSlots[slotID]
	nextVal := clampSlotValue(a.StateSlots[slotID]+qty*gainPerUnit, slotSpec, hasSpec)
	setActorSlotValue(a, slotID, nextVal)

	return []Event{w.record(a, x, "item_absorbed_slot", map[string]any{
		"text":          fmt.Sprintf("%s absorbed %d %s into %s (%d)", a.Name, qty, itemID, slotID, nextVal),
		"item":          itemID,
		"item_consumed": qty,
		"slot":          slotID,
		"slot_value":    nextVal,
	})}
}

// RecomputeFlowGrid solves the World's FlowGrid topology and updates each node's Powered flag.
func (w *World) RecomputeFlowGrid() {
	if w != nil {
		recomputeFlowGrid(w.FlowGrid)
	}
}

// recomputeFlowGrid solves deterministic flow from active generators across un-severed edges,
// respecting edge Capacity and LossPermille, and updates Powered on every FlowNode.
func recomputeFlowGrid(g *FlowGridState) map[string]int64 {
	if g == nil || len(g.Nodes) == 0 {
		return nil
	}
	nodeIdx := make(map[string]int, len(g.Nodes))
	adj := make(map[string][]int, len(g.Nodes))
	for i := range g.Nodes {
		g.Nodes[i].Powered = false
		nodeIdx[g.Nodes[i].ID] = i
	}
	remCap := make([]int, len(g.Edges))
	for ei, e := range g.Edges {
		if e.Severed {
			continue
		}
		remCap[ei] = e.Capacity
		adj[e.FromNodeID] = append(adj[e.FromNodeID], ei)
	}

	remGen := make(map[string]int)
	var genIDs []string
	var consumerIDs []string
	for i := range g.Nodes {
		n := &g.Nodes[i]
		switch n.Kind {
		case "generator":
			if n.Active && n.Generation > 0 && n.StoredEnergy > 0 {
				n.Powered = true
				remGen[n.ID] = int(min(int64(n.Generation), n.StoredEnergy))
				genIDs = append(genIDs, n.ID)
			}
		case "consumer":
			consumerIDs = append(consumerIDs, n.ID)
		}
	}

	received := make(map[string]int, len(consumerIDs))
	relayReached := make(map[string]bool)

	simulatePath := func(pathEdges []int, inFlow int) (int, bool) {
		cur := inFlow
		for _, ei := range pathEdges {
			if cur <= 0 || cur > remCap[ei] {
				return 0, false
			}
			loss := g.Edges[ei].LossPermille
			if loss < 0 {
				loss = 0
			}
			if loss > 1000 {
				loss = 1000
			}
			cur = (cur * (1000 - loss)) / 1000
		}
		return cur, cur > 0
	}

	// Route power through paths that can deliver a positive integer quantity.
	for _, cid := range consumerIDs {
		cNode := g.Nodes[nodeIdx[cid]]
		need := cNode.Demand
		if need <= 0 {
			need = 1
		}
		for cNode.Active && received[cid] < need {
			// Maximum deliverable flow is monotone along a path: each edge caps
			// its input and rounds down its output after loss. Keep the best
			// positive quantity per node, so a dead path cannot hide a bypass.
			parentEdge := make(map[string]int)
			parentNode := make(map[string]string)
			sourceGen := make(map[string]string)
			deliverable := make(map[string]int)
			visited := make(map[string]bool)
			for _, gid := range genIDs {
				deliverable[gid] = remGen[gid]
				sourceGen[gid] = gid
			}
			for {
				u := ""
				best := 0
				for _, n := range g.Nodes {
					if !visited[n.ID] && deliverable[n.ID] > best {
						u, best = n.ID, deliverable[n.ID]
					}
				}
				if u == "" || u == cid {
					break
				}
				visited[u] = true
				for _, ei := range adj[u] {
					e := g.Edges[ei]
					v := e.ToNodeID
					if visited[v] || !g.Nodes[nodeIdx[v]].Active {
						continue
					}
					out := min(best, remCap[ei]) * (1000 - max(0, min(1000, e.LossPermille))) / 1000
					if out <= deliverable[v] {
						continue
					}
					deliverable[v] = out
					parentEdge[v] = ei
					parentNode[v] = u
					sourceGen[v] = sourceGen[u]
				}
			}
			if deliverable[cid] <= 0 {
				break
			}
			var pathEdges []int
			for cur := cid; cur != sourceGen[cid]; cur = parentNode[cur] {
				pathEdges = append(pathEdges, parentEdge[cur])
			}
			for l, r := 0, len(pathEdges)-1; l < r; l, r = l+1, r-1 {
				pathEdges[l], pathEdges[r] = pathEdges[r], pathEdges[l]
			}

			gid := sourceGen[cid]
			maxIn := remGen[gid]
			remNeed := need - received[cid]

			// Find minimal inFlow <= maxIn delivering >= remNeed, or max deliverable flow.
			lo, hi := 1, maxIn
			bestIn := 0
			bestOut := 0
			for lo <= hi {
				mid := lo + (hi-lo)/2
				out, ok := simulatePath(pathEdges, mid)
				if !ok {
					// Either capacity exceeded or loss reduced output to 0.
					// Check if capacity exceeded by simulating without loss check.
					overCap := false
					tmp := mid
					for _, ei := range pathEdges {
						if tmp > remCap[ei] {
							overCap = true
							break
						}
						loss := g.Edges[ei].LossPermille
						if loss < 0 {
							loss = 0
						} else if loss > 1000 {
							loss = 1000
						}
						tmp = (tmp * (1000 - loss)) / 1000
					}
					if overCap {
						hi = mid - 1
					} else {
						lo = mid + 1
					}
					continue
				}
				if out >= remNeed {
					bestIn = mid
					bestOut = out
					hi = mid - 1
				} else {
					bestIn = mid
					bestOut = out
					lo = mid + 1
				}
			}
			if bestIn <= 0 || bestOut <= 0 {
				break
			}
			curFlow := bestIn
			remGen[gid] -= bestIn
			for _, ei := range pathEdges {
				remCap[ei] -= curFlow
				loss := g.Edges[ei].LossPermille
				if loss < 0 {
					loss = 0
				} else if loss > 1000 {
					loss = 1000
				}
				curFlow = (curFlow * (1000 - loss)) / 1000
				dst := g.Edges[ei].ToNodeID
				if idx, ok := nodeIdx[dst]; ok && g.Nodes[idx].Kind == "relay" {
					relayReached[dst] = true
				}
			}
			received[cid] += curFlow
		}
	}

	// Mark any reachable relays even if no consumer sits downstream.
	var rQueue []string
	rVisited := make(map[string]bool)
	for _, gid := range genIDs {
		rVisited[gid] = true
		rQueue = append(rQueue, gid)
	}
	for head := 0; head < len(rQueue); head++ {
		u := rQueue[head]
		for _, ei := range adj[u] {
			if !g.Nodes[nodeIdx[g.Edges[ei].ToNodeID]].Active || (remCap[ei] <= 0 && !relayReached[g.Edges[ei].ToNodeID]) {
				continue
			}
			v := g.Edges[ei].ToNodeID
			if !rVisited[v] {
				rVisited[v] = true
				rQueue = append(rQueue, v)
				if idx, ok := nodeIdx[v]; ok && g.Nodes[idx].Kind == "relay" {
					relayReached[v] = true
				}
			}
		}
	}

	for i := range g.Nodes {
		n := &g.Nodes[i]
		switch n.Kind {
		case "relay":
			n.Powered = relayReached[n.ID]
		case "consumer":
			need := n.Demand
			if need <= 0 {
				need = 1
			}
			n.Powered = n.Active && received[n.ID] >= need
		}
	}
	used := map[string]int64{}
	for _, id := range genIDs {
		n := g.Nodes[nodeIdx[id]]
		used[id] = min(int64(n.Generation), n.StoredEnergy) - int64(remGen[id])
	}
	return used
}

func snapshotFlowPowered(g *FlowGridState) map[string]bool {
	if g == nil {
		return nil
	}
	out := make(map[string]bool, len(g.Nodes))
	for _, n := range g.Nodes {
		out[n.ID] = n.Powered
	}
	return out
}

func (w *World) stepFlowGrid() []Event {
	if w.FlowGrid == nil || len(w.FlowGrid.Nodes) == 0 {
		return nil
	}
	prev := snapshotFlowPowered(w.FlowGrid)
	w.supplyEnergy()
	used := recomputeFlowGrid(w.FlowGrid)
	for i := range w.FlowGrid.Nodes {
		n := &w.FlowGrid.Nodes[i]
		w.adjustFlowNodeEnergy(n, -used[n.ID], true)
	}
	var events []Event
	for _, n := range w.FlowGrid.Nodes {
		if n.Kind != "consumer" {
			continue
		}
		was := prev[n.ID]
		if was && !n.Powered {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "flow_cutoff",
				Action:  Action{Kind: "wait", TargetID: n.ID},
				Payload: map[string]any{
					"text":     fmt.Sprintf("Power flow lost at %s (venue %s)", n.ID, n.VenueID),
					"node_id":  n.ID,
					"venue_id": n.VenueID,
				},
			}
			w.NextID++
			events = append(events, ev)
		} else if !was && n.Powered {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "flow_restored",
				Action:  Action{Kind: "wait", TargetID: n.ID},
				Payload: map[string]any{
					"text":     fmt.Sprintf("Power flow restored at %s (venue %s)", n.ID, n.VenueID),
					"node_id":  n.ID,
					"venue_id": n.VenueID,
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}
	return events
}

func resolveFlowEdgeID(x Action) string {
	if x.TargetID != "" {
		return x.TargetID
	}
	if x.Item != "" {
		return x.Item
	}
	return x.RecipeID
}

func (w *World) applySeverFlowEdge(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if w.FlowGrid == nil {
		return fail("flow grid is not enabled")
	}
	edgeID := resolveFlowEdgeID(x)
	var edge *FlowEdge
	for i := range w.FlowGrid.Edges {
		if w.FlowGrid.Edges[i].ID == edgeID {
			edge = &w.FlowGrid.Edges[i]
			break
		}
	}
	if edge == nil {
		return fail("unknown flow edge: " + edgeID)
	}
	if edge.Severed {
		return fail("flow edge is already severed: " + edgeID)
	}
	prev := snapshotFlowPowered(w.FlowGrid)
	edge.Severed = true
	recomputeFlowGrid(w.FlowGrid)

	var cutoffNodes []string
	for _, n := range w.FlowGrid.Nodes {
		if n.Kind == "consumer" && prev[n.ID] && !n.Powered {
			cutoffNodes = append(cutoffNodes, n.ID)
		}
	}
	return []Event{w.record(a, x, "flow_cutoff", map[string]any{
		"text":           fmt.Sprintf("%s severed flow edge %s (affected: %v)", a.Name, edge.ID, cutoffNodes),
		"edge_id":        edge.ID,
		"affected_nodes": cutoffNodes,
	})}
}

func (w *World) applyRepairFlowEdge(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if w.FlowGrid == nil {
		return fail("flow grid is not enabled")
	}
	edgeID := resolveFlowEdgeID(x)
	var edge *FlowEdge
	for i := range w.FlowGrid.Edges {
		if w.FlowGrid.Edges[i].ID == edgeID {
			edge = &w.FlowGrid.Edges[i]
			break
		}
	}
	if edge == nil {
		return fail("unknown flow edge: " + edgeID)
	}
	if !edge.Severed {
		return fail("flow edge is not severed: " + edgeID)
	}
	prev := snapshotFlowPowered(w.FlowGrid)
	edge.Severed = false
	recomputeFlowGrid(w.FlowGrid)

	var restoredNodes []string
	for _, n := range w.FlowGrid.Nodes {
		if n.Kind == "consumer" && !prev[n.ID] && n.Powered {
			restoredNodes = append(restoredNodes, n.ID)
		}
	}
	return []Event{w.record(a, x, "flow_restored", map[string]any{
		"text":           fmt.Sprintf("%s repaired flow edge %s (restored: %v)", a.Name, edge.ID, restoredNodes),
		"edge_id":        edge.ID,
		"restored_nodes": restoredNodes,
	})}
}

func (w *World) isVenuePowerBlocked(venueID string) bool {
	if w.FlowGrid == nil || venueID == "" {
		return false
	}
	for _, n := range w.FlowGrid.Nodes {
		if n.Kind == "consumer" && n.VenueID == venueID && !n.Powered {
			return true
		}
	}
	return false
}

func (w *World) totalCurrencyHeld(currency string) int64 {
	if currency == "" || currency == "coins" {
		return w.TotalCoins()
	}
	var total int64
	if w.Polities != nil {
		for i := range w.Polities.Nodes {
			total += w.Polities.Nodes[i].Currencies[currency]
		}
	}
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			total += w.Actors[i].Currencies[currency]
		}
	}
	for i := range w.Cohorts {
		total += w.Cohorts[i].Currencies[currency]
		for _, member := range w.Cohorts[i].Members {
			if !member.Dead {
				total += member.Currencies[currency]
			}
		}
	}
	return total
}

func (w *World) totalItemHeld(itemID string) int64 {
	var total int64
	for i := range w.Actors {
		total += int64(w.Actors[i].Items[itemID])
		for _, eqID := range w.Actors[i].Equipped {
			if eqID == itemID {
				total++
			}
		}
	}
	for i := range w.Cohorts {
		total += w.Cohorts[i].ItemPool[itemID]
		for _, b := range w.Cohorts[i].Residents {
			if q := b.Items[itemID]; q > 0 {
				total += int64(b.Count) * int64(q)
			}
		}
		for _, member := range w.Cohorts[i].Members {
			total += int64(member.Items[itemID])
			for _, eqID := range member.Equipped {
				if eqID == itemID {
					total++
				}
			}
		}
	}
	if w.MarketCredit != nil {
		total += w.TotalHoardedStock(w.MarketCredit, itemID)
	}
	for _, shipment := range w.Shipments {
		if shipment.Item == itemID {
			total += shipment.Quantity
		}
	}

	for _, g := range w.Growth {
		total += g.Parents[itemID]
		if g.Ready && g.Item == itemID {
			total += g.Quantity
		}
	}

	if w.FlowGrid != nil {
		for _, n := range w.FlowGrid.Nodes {
			if n.Source != nil && n.Source.FuelItem == itemID {
				total += n.FuelStock
			}
		}
	}

	if itemID != "" && itemID == w.Catalog.Lifecycle.WasteItem {
		for _, ds := range w.DistrictStates {
			total += ds.NightsoilPool
		}
	}

	return total
}

func (w *World) allHeldCurrencies() []string {
	seen := map[string]bool{"coins": true}
	for k := range w.Catalog.Currencies {
		if k != "" {
			seen[k] = true
		}
	}
	if w.Polities != nil {
		for i := range w.Polities.Nodes {
			for k, v := range w.Polities.Nodes[i].Currencies {
				if v != 0 {
					seen[k] = true
				}
			}
		}
	}
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			for k, v := range w.Actors[i].Currencies {
				if v != 0 {
					seen[k] = true
				}
			}
		}
	}
	for i := range w.Cohorts {
		for k, v := range w.Cohorts[i].Currencies {
			if v != 0 {
				seen[k] = true
			}
		}
		for _, member := range w.Cohorts[i].Members {
			if !member.Dead {
				for id := range member.Currencies {
					seen[id] = true
				}
			}
		}
	}
	if w.LedgerAudit != nil {
		for k := range w.LedgerAudit.Currencies {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (w *World) allHeldItems() []string {
	seen := map[string]bool{}
	for i := range w.Actors {
		for k, v := range w.Actors[i].Items {
			if v != 0 {
				seen[k] = true
			}
		}
		for _, eqID := range w.Actors[i].Equipped {
			if eqID != "" {
				seen[eqID] = true
			}
		}
	}
	for i := range w.Cohorts {
		for k, v := range w.Cohorts[i].ItemPool {
			if v != 0 {
				seen[k] = true
			}
		}
		for _, b := range w.Cohorts[i].Residents {
			for k, v := range b.Items {
				if v != 0 {
					seen[k] = true
				}
			}
		}
		for _, member := range w.Cohorts[i].Members {
			for id := range member.Items {
				seen[id] = true
			}
			for _, eqID := range member.Equipped {
				if eqID != "" {
					seen[eqID] = true
				}
			}
		}
	}
	if w.MarketCredit != nil {
		for _, bag := range w.MarketCredit.HoardedStock {
			for k, v := range bag {
				if v != 0 {
					seen[k] = true
				}
			}
		}
	}
	for _, shipment := range w.Shipments {
		seen[shipment.Item] = true
	}

	for _, g := range w.Growth {
		for item := range g.Parents {
			seen[item] = true
		}
		if g.Ready {
			seen[g.Item] = true
		}
	}

	if w.FlowGrid != nil {
		for _, n := range w.FlowGrid.Nodes {
			if n.Source != nil && n.Source.FuelItem != "" {
				seen[n.Source.FuelItem] = true
			}
		}
	}

	if waste := w.Catalog.Lifecycle.WasteItem; waste != "" {
		seen[waste] = true
	}

	if w.LedgerAudit != nil {
		for k := range w.LedgerAudit.Items {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// InitLedgerAudit snapshots current currency and item holdings into w.LedgerAudit
// so subsequent mutations satisfy Initial + Produced - Consumed == CurrentTotalHeld.
func (w *World) InitLedgerAudit() {
	la := &LedgerAuditState{
		Currencies: make(map[string]ResourceLedgerTotals),
		Items:      make(map[string]ResourceLedgerTotals),
	}
	for _, cur := range w.allHeldCurrencies() {
		la.Currencies[cur] = ResourceLedgerTotals{
			Initial: w.totalCurrencyHeld(cur),
		}
	}
	for _, it := range w.allHeldItems() {
		la.Items[it] = ResourceLedgerTotals{
			Initial: w.totalItemHeld(it),
		}
	}
	if len(w.DistrictStates) > 0 {
		la.Ecology = make(map[string]map[string]ResourceLedgerTotals, len(w.DistrictStates))
		for district, ds := range w.DistrictStates {
			la.Ecology[district] = map[string]ResourceLedgerTotals{}
			for pool, amount := range ds.Resources {
				la.Ecology[district][pool] = ResourceLedgerTotals{Initial: amount}
			}
		}
	}
	la.Energy.Initial = w.storedEnergy()
	w.LedgerAudit = la
}

// VerifyLedgerConservation checks nonnegative individual holdings and exact recorded
// currency/item/resource/energy totals. Transaction delivery and physical conversion
// rules are enforced at their settlement entry points; totals alone cannot prove them.
func (w *World) VerifyLedgerConservation() error {
	if w.LedgerAudit == nil {
		return fmt.Errorf("ledger audit is not initialized")
	}
	if err := w.VerifyHoldingBounds(); err != nil {
		return err
	}
	for _, cur := range w.allHeldCurrencies() {
		entry := w.LedgerAudit.Currencies[cur]
		expected := entry.Initial + entry.Produced - entry.Consumed
		actual := w.totalCurrencyHeld(cur)
		if actual != expected {
			return fmt.Errorf("currency %q conservation error: initial=%d + produced=%d - consumed=%d = %d != actual=%d",
				cur, entry.Initial, entry.Produced, entry.Consumed, expected, actual)
		}
	}
	for _, it := range w.allHeldItems() {
		entry := w.LedgerAudit.Items[it]
		expected := entry.Initial + entry.Produced - entry.Consumed
		actual := w.totalItemHeld(it)
		if actual != expected {
			return fmt.Errorf("item %q conservation error: initial=%d + produced=%d - consumed=%d = %d != actual=%d",
				it, entry.Initial, entry.Produced, entry.Consumed, expected, actual)
		}
	}
	for district, ds := range w.DistrictStates {
		for pool, amount := range ds.Resources {
			entry := w.LedgerAudit.Ecology[district][pool]
			if expected := entry.Initial + entry.Produced - entry.Consumed; amount != expected {
				return fmt.Errorf("ecology %s/%s accounting error: expected=%d actual=%d", district, pool, expected, amount)
			}
		}
	}
	for district, pools := range w.LedgerAudit.Ecology {
		for pool, entry := range pools {
			if actual := w.DistrictStates[district].Resources[pool]; actual != entry.Initial+entry.Produced-entry.Consumed {
				return fmt.Errorf("ecology %s/%s accounting error: expected=%d actual=%d", district, pool, entry.Initial+entry.Produced-entry.Consumed, actual)
			}
		}
	}

	if e := w.LedgerAudit.Energy; w.storedEnergy() != e.Initial+e.Produced-e.Consumed {
		return fmt.Errorf("stored energy accounting mismatch")
	}

	return nil
}

func (w *World) recordLedgerItemProduced(itemID string, qty int64) {
	if w.LedgerAudit == nil || itemID == "" || qty <= 0 {
		return
	}
	if w.LedgerAudit.Items == nil {
		w.LedgerAudit.Items = map[string]ResourceLedgerTotals{}
	}
	t := w.LedgerAudit.Items[itemID]
	t.Produced += qty
	w.LedgerAudit.Items[itemID] = t
}

func (w *World) recordLedgerItemConsumed(itemID string, qty int64) {
	if w.LedgerAudit == nil || itemID == "" || qty <= 0 {
		return
	}
	if w.LedgerAudit.Items == nil {
		w.LedgerAudit.Items = map[string]ResourceLedgerTotals{}
	}
	t := w.LedgerAudit.Items[itemID]
	t.Consumed += qty
	w.LedgerAudit.Items[itemID] = t
}

// adjustActorItem mutates an actor's item stack and optionally records production/consumption in LedgerAudit in one place.
func (w *World) adjustActorItem(a *Actor, itemID string, delta int, trackLedger bool) {
	if a == nil || itemID == "" || delta == 0 {
		return
	}
	if delta > 0 {
		if a.Items == nil {
			a.Items = map[string]int{}
		}
		a.Items[itemID] += delta
		if trackLedger {
			w.recordLedgerItemProduced(itemID, int64(delta))
		}
		return
	}
	if a.Items != nil {
		a.Items[itemID] += delta
		if a.Items[itemID] <= 0 {
			delete(a.Items, itemID)
		}
	}
	if trackLedger {
		w.recordLedgerItemConsumed(itemID, int64(-delta))
	}
}

// adjustCohortItem mutates a cohort's ItemPool and optionally records production/consumption in LedgerAudit in one place.
func (w *World) adjustCohortItem(c *PopulationCohort, itemID string, delta int64, trackLedger bool) {
	if c == nil || itemID == "" || delta == 0 {
		return
	}
	if delta > 0 {
		if c.ItemPool == nil {
			c.ItemPool = map[string]int64{}
		}
		c.ItemPool[itemID] += delta
		if trackLedger {
			w.recordLedgerItemProduced(itemID, delta)
		}
		return
	}
	if c.ItemPool != nil {
		c.ItemPool[itemID] += delta
		if c.ItemPool[itemID] <= 0 {
			delete(c.ItemPool, itemID)
		}
	}
	if trackLedger {
		w.recordLedgerItemConsumed(itemID, -delta)
	}
}

// adjustResidentBandItem mutates a ResidentBand's per-person item stack and optionally records scaled production/consumption.
func (w *World) adjustResidentBandItem(b *ResidentBand, itemID string, delta int, trackLedger bool) {
	if b == nil || itemID == "" || delta == 0 || b.Count <= 0 {
		return
	}
	if delta > 0 {
		if b.Items == nil {
			b.Items = map[string]int{}
		}
		b.Items[itemID] += delta
		if trackLedger {
			w.recordLedgerItemProduced(itemID, int64(delta)*int64(b.Count))
		}
		return
	}
	if b.Items != nil {
		b.Items[itemID] += delta
		if b.Items[itemID] <= 0 {
			delete(b.Items, itemID)
		}
		if len(b.Items) == 0 {
			b.Items = nil
		}
	}
	if trackLedger {
		w.recordLedgerItemConsumed(itemID, int64(-delta)*int64(b.Count))
	}
}

// removeActorEquippedItem removes an equipped slot item from an actor and optionally records consumption in LedgerAudit.
func (w *World) removeActorEquippedItem(a *Actor, slot string, trackLedger bool) bool {
	if a == nil || slot == "" || len(a.Equipped) == 0 {
		return false
	}
	itemID, ok := a.Equipped[slot]
	if !ok || itemID == "" {
		return false
	}
	delete(a.Equipped, slot)
	if len(a.Equipped) == 0 {
		a.Equipped = nil
	}
	if trackLedger {
		w.recordLedgerItemConsumed(itemID, 1)
	}
	return true
}

// adjustFlowNodeEnergy mutates a FlowNode's StoredEnergy and optionally records production/consumption in LedgerAudit.
func (w *World) adjustFlowNodeEnergy(n *FlowNode, delta int64, trackLedger bool) {
	if n == nil || delta == 0 {
		return
	}
	n.StoredEnergy += delta
	if n.StoredEnergy < 0 {
		n.StoredEnergy = 0
	}
	if !trackLedger {
		return
	}
	if delta > 0 {
		w.recordLedgerEnergyProduced(delta)
	} else {
		w.recordLedgerEnergyConsumed(-delta)
	}
}

func (w *World) recordLedgerEnergyProduced(amt int64) {
	if w.LedgerAudit == nil || amt <= 0 {
		return
	}
	w.LedgerAudit.Energy.Produced += amt
}

func (w *World) recordLedgerEnergyConsumed(amt int64) {
	if w.LedgerAudit == nil || amt <= 0 {
		return
	}
	w.LedgerAudit.Energy.Consumed += amt
}

// adjustFlowNodeFuel mutates a FlowNode's FuelStock and optionally records production/consumption in LedgerAudit.
func (w *World) adjustFlowNodeFuel(n *FlowNode, delta int64, trackLedger bool) {
	if n == nil || delta == 0 || n.Source == nil || n.Source.FuelItem == "" {
		return
	}
	n.FuelStock += delta
	if n.FuelStock < 0 {
		n.FuelStock = 0
	}
	if !trackLedger {
		return
	}
	if delta > 0 {
		w.recordLedgerItemProduced(n.Source.FuelItem, delta)
	} else {
		w.recordLedgerItemConsumed(n.Source.FuelItem, -delta)
	}
}

// adjustDistrictNightsoil mutates a district's NightsoilPool and optionally records production/consumption in LedgerAudit.
func (w *World) adjustDistrictNightsoil(districtID, wasteItem string, delta int64, trackLedger bool) {
	if w.DistrictStates == nil || districtID == "" || wasteItem == "" || delta == 0 {
		return
	}
	ds, ok := w.DistrictStates[districtID]
	if !ok {
		return
	}
	ds.NightsoilPool += delta
	if ds.NightsoilPool < 0 {
		ds.NightsoilPool = 0
	}
	w.DistrictStates[districtID] = ds
	if !trackLedger {
		return
	}
	if delta > 0 {
		w.recordLedgerItemProduced(wasteItem, delta)
	} else {
		w.recordLedgerItemConsumed(wasteItem, -delta)
	}
}

// adjustGrowthParentItem mutates a GrowthBatch's Parents stock and optionally records production/consumption in LedgerAudit.
func (w *World) adjustGrowthParentItem(g *GrowthBatch, itemID string, delta int64, trackLedger bool) {
	if g == nil || itemID == "" || delta == 0 {
		return
	}
	if delta > 0 {
		if g.Parents == nil {
			g.Parents = map[string]int64{}
		}
		g.Parents[itemID] += delta
		if trackLedger {
			w.recordLedgerItemProduced(itemID, delta)
		}
		return
	}
	if g.Parents != nil {
		g.Parents[itemID] += delta
		if g.Parents[itemID] <= 0 {
			delete(g.Parents, itemID)
		}
	}
	if trackLedger {
		w.recordLedgerItemConsumed(itemID, -delta)
	}
}

// adjustGrowthOutputItem mutates a ready GrowthBatch's held Quantity and optionally records production/consumption in LedgerAudit.
func (w *World) adjustGrowthOutputItem(g *GrowthBatch, delta int64, trackLedger bool) {
	if g == nil || g.Item == "" || delta == 0 {
		return
	}
	g.Quantity += delta
	if g.Quantity < 0 {
		g.Quantity = 0
	}
	if !trackLedger {
		return
	}
	if delta > 0 {
		w.recordLedgerItemProduced(g.Item, delta)
	} else {
		w.recordLedgerItemConsumed(g.Item, -delta)
	}
}

func (w *World) recordLedgerCurrencyProduced(currency string, amt int64) {
	if w.LedgerAudit == nil || amt <= 0 {
		return
	}
	if currency == "" {
		currency = "coins"
	}
	if w.LedgerAudit.Currencies == nil {
		w.LedgerAudit.Currencies = map[string]ResourceLedgerTotals{}
	}
	t := w.LedgerAudit.Currencies[currency]
	t.Produced += amt
	w.LedgerAudit.Currencies[currency] = t
}

func (w *World) recordLedgerCurrencyConsumed(currency string, amt int64) {
	if w.LedgerAudit == nil || amt <= 0 {
		return
	}
	if currency == "" {
		currency = "coins"
	}
	if w.LedgerAudit.Currencies == nil {
		w.LedgerAudit.Currencies = map[string]ResourceLedgerTotals{}
	}
	t := w.LedgerAudit.Currencies[currency]
	t.Consumed += amt
	w.LedgerAudit.Currencies[currency] = t
}

// Record the amount settled by extraction, regeneration, or disaster loss.
func (w *World) recordEcologicalChange(district, pool string, delta int64) {
	if w.LedgerAudit == nil || delta == 0 {
		return
	}
	if w.LedgerAudit.Ecology == nil {
		w.LedgerAudit.Ecology = map[string]map[string]ResourceLedgerTotals{}
	}
	if w.LedgerAudit.Ecology[district] == nil {
		w.LedgerAudit.Ecology[district] = map[string]ResourceLedgerTotals{}
	}
	entry := w.LedgerAudit.Ecology[district][pool]
	if delta > 0 {
		entry.Produced += delta
	} else {
		entry.Consumed -= delta
	}
	w.LedgerAudit.Ecology[district][pool] = entry
}

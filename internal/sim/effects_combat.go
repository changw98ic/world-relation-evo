package sim

import (
	"fmt"
	"sort"
	"strings"
)

// EntityRef is a universal cross-scale target reference so capabilities, actions, and active effects
// can target an individual Actor, a macro PopulationCohort, a House/facility, a Venue, a District,
// a GeoRegion, a PolityNode, or WorldLaws without genre-specific action types.
type EntityRef struct {
	Kind string `json:"kind"` // "actor", "cohort", "house", "venue", "district", "geo_region", "polity", "world_law"
	ID   string `json:"id"`
}

// DefenseRule defines damage mitigation and tier-based immunity for a specific damage type
// (e.g. "physical", "kinetic", "emp", "spirit_fire", "radiation", "mental").
type DefenseRule struct {
	ImmunityBelowTier int `json:"immunity_below_tier,omitempty"`
	ReductionPct      int `json:"reduction_pct,omitempty"`
}

// CombatProfileSpec declaratively configures multi-dimensional combat tier, typed attack outputs,
// typed defense/immunity rules, and cross-scale cohort lethality granted by tags or carried items.
type CombatProfileSpec struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	GrantedByTags     []string               `json:"granted_by_tags,omitempty"`
	GrantedByItems    []string               `json:"granted_by_items,omitempty"`
	Tier              int                    `json:"tier"`
	AttackTypes       map[string]int         `json:"attack_types,omitempty"`
	DefenseByType     map[string]DefenseRule `json:"defense_by_type,omitempty"`
	CohortScaleDamage int                    `json:"cohort_scale_damage,omitempty"`
}

// AttackSpec describes the offensive payload of a declarative ActionSpec.
type AttackSpec struct {
	Tier             int            `json:"tier,omitempty"`
	DamageByType     map[string]int `json:"damage_by_type,omitempty"`
	IntegrityDamage  int            `json:"integrity_damage,omitempty"`
	AreaScope        string         `json:"area_scope,omitempty"` // "single", "venue", "cohort"
	CohortCasualties int            `json:"cohort_casualties,omitempty"`
}

// EffectSpec is the declarative template for spawning a sustained ActiveEffect
// (e.g. Sect Guardian Formation, Regional EMP Dome, Radiation Hazard Zone, Martial Curfew Aura).
type EffectSpec struct {
	ID                        string                 `json:"id"`
	Name                      string                 `json:"name"`
	AnchorKind                string                 `json:"anchor_kind"` // "self", "target_actor", "venue", "district"
	DurationTicks             int64                  `json:"duration_ticks"`
	MaxIntegrity              int                    `json:"max_integrity,omitempty"`
	SustainEnergyPerTick      int                    `json:"sustain_energy_per_tick,omitempty"`
	SustainSlotPerTick        map[string]int         `json:"sustain_slot_per_tick,omitempty"`
	DefenseByType             map[string]DefenseRule `json:"defense_by_type,omitempty"`
	AbsorbDamageWithIntegrity bool                   `json:"absorb_damage_with_integrity,omitempty"`
	ShieldingBonus            int                    `json:"shielding_bonus,omitempty"`
	GrantTags                 []string               `json:"grant_tags,omitempty"`
}

// ActiveEffect is a live sustained effect instance anchored to an Actor, Venue, or District.
type ActiveEffect struct {
	ID                        int64                  `json:"id"`
	SpecID                    string                 `json:"spec_id"`
	Name                      string                 `json:"name"`
	Source                    EntityRef              `json:"source"`
	Anchor                    EntityRef              `json:"anchor"`
	SustainActorID            string                 `json:"sustain_actor_id,omitempty"`
	CreatedTick               int64                  `json:"created_tick"`
	ExpiresTick               int64                  `json:"expires_tick"`
	Integrity                 int                    `json:"integrity,omitempty"`
	MaxIntegrity              int                    `json:"max_integrity,omitempty"`
	SustainEnergyPerTick      int                    `json:"sustain_energy_per_tick,omitempty"`
	SustainSlotPerTick        map[string]int         `json:"sustain_slot_per_tick,omitempty"`
	DefenseByType             map[string]DefenseRule `json:"defense_by_type,omitempty"`
	AbsorbDamageWithIntegrity bool                   `json:"absorb_damage_with_integrity,omitempty"`
	ShieldingBonus            int                    `json:"shielding_bonus,omitempty"`
	GrantTags                 []string               `json:"grant_tags,omitempty"`
}

// ActionSpec declaratively defines any world-pack action (farming, smithing, hunting, fishing,
// alchemy, chip R&D, cultivation array deployment, mech EMP strike, closed-door seclusion, cryosleep,
// battery recharge, breakthrough, or epoch law rewrite) so all genres execute through one
// Capability -> Options -> Selector -> Apply pipeline.
type ActionSpec struct {
	ID                    string           `json:"id"`
	Name                  string           `json:"name"`
	GrantedByTags         []string         `json:"granted_by_tags,omitempty"`
	TargetKind            string           `json:"target_kind"` // "self", "actor", "cohort", "venue", "district"
	RangeKm               int              `json:"range_km,omitempty"`
	RequiredVenueFeature  string           `json:"required_venue_feature,omitempty"`
	RequireOutdoors       bool             `json:"require_outdoors,omitempty"`
	RequireIndoors        bool             `json:"require_indoors,omitempty"`
	Profession            string           `json:"profession,omitempty"`
	MinProfessionLevel    int              `json:"min_profession_level,omitempty"`
	RequiredTechs         []string         `json:"required_techs,omitempty"`
	RequiredFormIDs       []string         `json:"required_form_ids,omitempty"`
	MinAttributes         map[string]int   `json:"min_attributes,omitempty"`
	MinHealth             int              `json:"min_health,omitempty"`
	ToolItem              string           `json:"tool_item,omitempty"`
	ResourcePool          string           `json:"resource_pool,omitempty"`
	ResourceDraw          int64            `json:"resource_draw,omitempty"`
	RequiredThreatID      string           `json:"required_threat_id,omitempty"`
	MinThreatActivity     int              `json:"min_threat_activity,omitempty"`
	Ticks                 int              `json:"ticks,omitempty"`
	Dormant               bool             `json:"dormant,omitempty"`
	EnergyCost            int              `json:"energy_cost,omitempty"`
	MoneyCost             int              `json:"money_cost,omitempty"`
	CurrencyCosts         map[string]int   `json:"currency_costs,omitempty"`
	ItemCosts             map[string]int   `json:"item_costs,omitempty"`
	SlotCosts             map[string]int   `json:"slot_costs,omitempty"`
	ItemGains             map[string]int   `json:"item_gains,omitempty"`
	HealthGain            int              `json:"health_gain,omitempty"`
	EnergyGain            int              `json:"energy_gain,omitempty"`
	SlotGains             map[string]int   `json:"slot_gains,omitempty"`
	XPProfession          string           `json:"xp_profession,omitempty"`
	ProfessionXP          int              `json:"profession_xp,omitempty"`
	UnlockTechs           []string         `json:"unlock_techs,omitempty"`
	ThreatID              string           `json:"threat_id,omitempty"`
	ReduceThreat          int              `json:"reduce_threat,omitempty"`
	MoraleDelta           int              `json:"morale_delta,omitempty"`
	DistrictResourceGains map[string]int64 `json:"district_resource_gains,omitempty"`
	Attack                *AttackSpec      `json:"attack,omitempty"`
	SpawnEffect           *EffectSpec      `json:"spawn_effect,omitempty"`
	TransitionID          string           `json:"transition_id,omitempty"`
	AllowRewriteWorldLaw  bool             `json:"allow_rewrite_world_law,omitempty"`
	WorldLawDelta         map[string]int   `json:"world_law_delta,omitempty"`
}

func cloneDefenseMap(in map[string]DefenseRule) map[string]DefenseRule {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]DefenseRule, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func CloneActiveEffect(e ActiveEffect) ActiveEffect {
	out := e
	out.SustainSlotPerTick = cloneIntMap(e.SustainSlotPerTick)
	out.DefenseByType = cloneDefenseMap(e.DefenseByType)
	if e.GrantTags != nil {
		out.GrantTags = append([]string(nil), e.GrantTags...)
	}
	return out
}

func CloneActionSpec(s ActionSpec) ActionSpec {
	out := s
	if s.GrantedByTags != nil {
		out.GrantedByTags = append([]string(nil), s.GrantedByTags...)
	}
	if s.RequiredTechs != nil {
		out.RequiredTechs = append([]string(nil), s.RequiredTechs...)
	}
	if s.RequiredFormIDs != nil {
		out.RequiredFormIDs = append([]string(nil), s.RequiredFormIDs...)
	}
	if s.UnlockTechs != nil {
		out.UnlockTechs = append([]string(nil), s.UnlockTechs...)
	}
	out.MinAttributes = cloneIntMap(s.MinAttributes)
	out.CurrencyCosts = cloneIntMap(s.CurrencyCosts)
	out.ItemCosts = cloneIntMap(s.ItemCosts)
	out.SlotCosts = cloneIntMap(s.SlotCosts)
	out.ItemGains = cloneIntMap(s.ItemGains)
	out.SlotGains = cloneIntMap(s.SlotGains)
	out.DistrictResourceGains = cloneInt64Map(s.DistrictResourceGains)
	out.WorldLawDelta = cloneIntMap(s.WorldLawDelta)
	if s.Attack != nil {
		ac := *s.Attack
		ac.DamageByType = cloneIntMap(s.Attack.DamageByType)
		out.Attack = &ac
	}
	if s.SpawnEffect != nil {
		ec := *s.SpawnEffect
		ec.SustainSlotPerTick = cloneIntMap(s.SpawnEffect.SustainSlotPerTick)
		ec.DefenseByType = cloneDefenseMap(s.SpawnEffect.DefenseByType)
		if s.SpawnEffect.GrantTags != nil {
			ec.GrantTags = append([]string(nil), s.SpawnEffect.GrantTags...)
		}
		out.SpawnEffect = &ec
	}
	return out
}

func CloneCombatProfileSpec(p CombatProfileSpec) CombatProfileSpec {
	out := p
	if p.GrantedByTags != nil {
		out.GrantedByTags = append([]string(nil), p.GrantedByTags...)
	}
	if p.GrantedByItems != nil {
		out.GrantedByItems = append([]string(nil), p.GrantedByItems...)
	}
	out.AttackTypes = cloneIntMap(p.AttackTypes)
	out.DefenseByType = cloneDefenseMap(p.DefenseByType)
	return out
}

func actorMatchesCombatProfile(cat Catalog, a Actor, p CombatProfileSpec) bool {
	tags := actorEffectiveTags(cat, a)
	for _, gt := range p.GrantedByTags {
		if ActorHasTag(tags, gt) {
			return true
		}
	}
	for _, itemID := range p.GrantedByItems {
		if a.Items[itemID] > 0 {
			return true
		}
	}
	return false
}

// ActorCombatProfile aggregates an actor's highest combat Tier, typed attacks, typed defenses,
// and cross-scale cohort lethality from Catalog.CombatProfiles.
func (c Catalog) ActorCombatProfile(a Actor) (tier int, attacks map[string]int, defenses map[string]DefenseRule, cohortDmg int) {
	tier = 1
	for _, cp := range c.CombatProfiles {
		if !actorMatchesCombatProfile(c, a, cp) {
			continue
		}
		if cp.Tier > tier {
			tier = cp.Tier
		}
		if cp.CohortScaleDamage > cohortDmg {
			cohortDmg = cp.CohortScaleDamage
		}
		for dt, val := range cp.AttackTypes {
			if attacks == nil {
				attacks = map[string]int{}
			}
			if val > attacks[dt] {
				attacks[dt] = val
			}
		}
		for dt, rule := range cp.DefenseByType {
			if defenses == nil {
				defenses = map[string]DefenseRule{}
			}
			cur := defenses[dt]
			if rule.ImmunityBelowTier > cur.ImmunityBelowTier {
				cur.ImmunityBelowTier = rule.ImmunityBelowTier
			}
			if rule.ReductionPct > cur.ReductionPct {
				cur.ReductionPct = rule.ReductionPct
			}
			defenses[dt] = cur
		}
	}
	return tier, attacks, defenses, cohortDmg
}

func (w *World) ActorCombatTier(a Actor) int {
	tier, _, _, _ := w.Catalog.ActorCombatProfile(a)
	return tier
}

// IsActorPinnedFocal reports whether an actor must remain an independent focal individual and never
// be merged/dehydrated into an anonymous PopulationCohort average pool.
func (w *World) IsActorPinnedFocal(a Actor) bool {
	if a.PinFocal {
		return true
	}
	if w.ActorCombatTier(a) >= 3 {
		return true
	}
	if w.Polities != nil && w.Polities.NodeLedBy(a.ID) != nil {
		return true
	}
	return false
}

func (w *World) effectAppliesToActor(eff ActiveEffect, a Actor) bool {
	switch eff.Anchor.Kind {
	case "actor":
		return eff.Anchor.ID == a.ID
	case "venue":
		return eff.Anchor.ID != "" && a.VenueID == eff.Anchor.ID
	case "district":
		return eff.Anchor.ID != "" && a.Location == eff.Anchor.ID
	default:
		return false
	}
}

func (w *World) activeShieldingBonusForActor(a Actor) int {
	bonus := 0
	for _, eff := range w.ActiveEffects {
		if eff.ShieldingBonus > 0 && w.effectAppliesToActor(eff, a) {
			bonus += eff.ShieldingBonus
		}
	}
	return bonus
}

func (w *World) activeShieldingBonusForVenue(districtID, venueID string) int {
	bonus := 0
	for _, eff := range w.ActiveEffects {
		if eff.ShieldingBonus <= 0 {
			continue
		}
		if eff.Anchor.Kind == "venue" && venueID != "" && eff.Anchor.ID == venueID {
			bonus += eff.ShieldingBonus
		} else if eff.Anchor.Kind == "district" && districtID != "" && eff.Anchor.ID == districtID {
			bonus += eff.ShieldingBonus
		}
	}
	return bonus
}

func (w *World) activeEffectGrantedTags(a Actor) []string {
	var extra []string
	for _, eff := range w.ActiveEffects {
		if len(eff.GrantTags) > 0 && w.effectAppliesToActor(eff, a) {
			for _, t := range eff.GrantTags {
				extra = appendUnique(extra, t)
			}
		}
	}
	return extra
}

func (w *World) actorEffectiveTags(a Actor) []string {
	tags := actorEffectiveTags(w.Catalog, a)
	for _, t := range w.activeEffectGrantedTags(a) {
		tags = appendUnique(tags, t)
	}
	return tags
}

func (w *World) neighborEffectiveTags(a Actor) []string {
	tags := w.Catalog.ActorEffectiveTags(a)
	for _, t := range w.activeEffectGrantedTags(a) {
		tags = appendUnique(tags, t)
	}
	return tags
}

func (w *World) actorWithEffectTags(a Actor) Actor {
	extra := w.activeEffectGrantedTags(a)
	if len(extra) == 0 {
		return a
	}
	out := a
	out.Tags = append([]string(nil), a.Tags...)
	for _, t := range extra {
		out.Tags = appendUnique(out.Tags, t)
	}
	return out
}

// ActorCapabilities reports which action kinds the actor's tags, professions, and active effects grant.
func (w *World) ActorCapabilities(a Actor) map[string]bool {
	return w.Catalog.Capabilities(w.actorWithEffectTags(a))
}

func effectAppliesToActor(eff ActiveEffect, a Actor) bool {
	switch eff.Anchor.Kind {
	case "actor":
		return eff.Anchor.ID == a.ID
	case "venue":
		return eff.Anchor.ID != "" && a.VenueID == eff.Anchor.ID
	case "district":
		return eff.Anchor.ID != "" && a.Location == eff.Anchor.ID
	default:
		return false
	}
}

// EffectiveTags returns Perception.Self's effective tags including FormSpec innate tags,
// profession grants, and any matching ActiveEffects.
func (p Perception) EffectiveTags() []string {
	tags := p.Catalog.ActorEffectiveTags(p.Self)
	for _, eff := range p.ActiveEffects {
		if len(eff.GrantTags) > 0 && effectAppliesToActor(eff, p.Self) {
			for _, t := range eff.GrantTags {
				tags = appendUnique(tags, t)
			}
		}
	}
	return tags
}

// Capabilities reports which action kinds are granted to Perception.Self, including ActiveEffects.
func (p Perception) Capabilities() map[string]bool {
	if len(p.ActiveEffects) == 0 {
		return p.Catalog.Capabilities(p.Self)
	}
	a := p.Self
	var extra []string
	for _, eff := range p.ActiveEffects {
		if len(eff.GrantTags) > 0 && effectAppliesToActor(eff, a) {
			for _, t := range eff.GrantTags {
				extra = appendUnique(extra, t)
			}
		}
	}
	if len(extra) == 0 {
		return p.Catalog.Capabilities(a)
	}
	a.Tags = append([]string(nil), a.Tags...)
	for _, t := range extra {
		a.Tags = appendUnique(a.Tags, t)
	}
	return p.Catalog.Capabilities(a)
}

// resolveMultiDimCombatHit computes net HP damage on defender `b` and barrier integrity absorption
// across active effects, returning netDamage, absorbedByBarrier, immuneBlocked, and brokenEffectEvents.
func (w *World) resolveMultiDimCombatHit(attacker Actor, defender *Actor, override *AttackSpec, x Action) (netDamage int, absorbed int, immuneBlocked bool, brokenEvents []Event) {
	atkTier, atkTypes, _, _ := w.Catalog.ActorCombatProfile(attacker)
	if override != nil {
		if override.Tier > 0 {
			atkTier = override.Tier
		}
		if len(override.DamageByType) > 0 {
			atkTypes = override.DamageByType
		}
	}
	if len(atkTypes) == 0 {
		baseDmg := 8 + (CombatPowerWithCatalog(w.Catalog, attacker)-10)/2
		if baseDmg < 10 {
			baseDmg = 10
		}
		atkTypes = map[string]int{"physical": baseDmg}
	}

	_, _, defRules, _ := w.Catalog.ActorCombatProfile(*defender)

	// Merge defense rules from equipped armor/items and ActiveEffects protecting the defender.
	mergedDef := cloneDefenseMap(defRules)
	mergeDefRule := func(dt string, rule DefenseRule) {
		if mergedDef == nil {
			mergedDef = map[string]DefenseRule{}
		}
		cur := mergedDef[dt]
		if rule.ImmunityBelowTier > cur.ImmunityBelowTier {
			cur.ImmunityBelowTier = rule.ImmunityBelowTier
		}
		if rule.ReductionPct > cur.ReductionPct {
			cur.ReductionPct = rule.ReductionPct
		}
		mergedDef[dt] = cur
	}
	for _, eqItemID := range defender.Equipped {
		if eqItemID != "" {
			if eqSpec, ok := w.Catalog.Items[eqItemID]; ok {
				for dt, rule := range eqSpec.EquipDefenseByType {
					mergeDefRule(dt, rule)
				}
			}
		}
	}
	for _, eff := range w.ActiveEffects {
		if !w.effectAppliesToActor(eff, *defender) {
			continue
		}
		for dt, rule := range eff.DefenseByType {
			mergeDefRule(dt, rule)
		}
	}

	keys := make([]string, 0, len(atkTypes))
	for dt := range atkTypes {
		keys = append(keys, dt)
	}
	sort.Strings(keys)

	totalPostDef := 0
	blockedCount := 0
	for _, dt := range keys {
		raw := atkTypes[dt]
		if raw <= 0 {
			continue
		}
		if mergedDef != nil {
			if rule, hasRule := mergedDef[dt]; hasRule {
				if rule.ImmunityBelowTier > 0 && atkTier < rule.ImmunityBelowTier {
					blockedCount++
					continue
				}
				if rule.ReductionPct > 0 {
					red := rule.ReductionPct
					if red > 95 {
						red = 95
					}
					raw = (raw * (100 - red)) / 100
					if raw < 1 {
						raw = 1
					}
				}
			}
		}
		totalPostDef += raw
	}

	if totalPostDef == 0 && blockedCount > 0 {
		immuneBlocked = true
	}

	// Barrier integrity damage & absorption on ActiveEffects protecting the defender.
	bonusIntDmg := 0
	if override != nil && override.IntegrityDamage > 0 {
		bonusIntDmg = override.IntegrityDamage
	}
	if totalPostDef > 0 || bonusIntDmg > 0 {
		kept := w.ActiveEffects[:0]
		for _, eff := range w.ActiveEffects {
			if eff.MaxIntegrity > 0 && eff.Integrity > 0 && w.effectAppliesToActor(eff, *defender) {
				if bonusIntDmg > 0 {
					if bonusIntDmg >= eff.Integrity {
						absorbed += eff.Integrity
						eff.Integrity = 0
					} else {
						absorbed += bonusIntDmg
						eff.Integrity -= bonusIntDmg
					}
				}
				if eff.AbsorbDamageWithIntegrity && totalPostDef > 0 && eff.Integrity > 0 {
					if totalPostDef <= eff.Integrity {
						absorbed += totalPostDef
						eff.Integrity -= totalPostDef
						totalPostDef = 0
					} else {
						absorbed += eff.Integrity
						totalPostDef -= eff.Integrity
						eff.Integrity = 0
					}
				}
				if eff.Integrity <= 0 {
					ev := Event{
						ID:       w.NextID,
						Tick:     w.Tick,
						ActorID:  attacker.ID,
						TargetID: defender.ID,
						Kind:     "effect_broken",
						Action:   x,
						Payload: map[string]any{
							"text":        fmt.Sprintf("%s shattered %s protecting %s", attacker.Name, eff.Name, defender.Name),
							"effect_id":   eff.ID,
							"effect_spec": eff.SpecID,
							"anchor_kind": eff.Anchor.Kind,
							"anchor_id":   eff.Anchor.ID,
						},
					}
					w.NextID++
					brokenEvents = append(brokenEvents, ev)
					continue
				}
			}
			kept = append(kept, eff)
		}
		if len(kept) == 0 {
			w.ActiveEffects = nil
		} else {
			w.ActiveEffects = kept
		}
	}

	return totalPostDef, absorbed, immuneBlocked, brokenEvents
}

// stepActiveEffects advances sustained effects per tick, deducting SustainEnergyPerTick and
// SustainSlotPerTick from SustainActorID and terminating expired or resource-exhausted effects.
func (w *World) stepActiveEffects() []Event {
	if len(w.ActiveEffects) == 0 {
		w.ActiveEffects = nil
		return nil
	}
	var events []Event
	kept := w.ActiveEffects[:0]
	for _, eff := range w.ActiveEffects {
		if eff.ExpiresTick > 0 && w.Tick >= eff.ExpiresTick {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "effect_expired",
				Action:  Action{Kind: "wait"},
				Payload: map[string]any{
					"text":        fmt.Sprintf("%s at %s:%s expired naturally", eff.Name, eff.Anchor.Kind, eff.Anchor.ID),
					"effect_id":   eff.ID,
					"effect_spec": eff.SpecID,
					"anchor_kind": eff.Anchor.Kind,
					"anchor_id":   eff.Anchor.ID,
				},
			}
			w.NextID++
			events = append(events, ev)
			continue
		}
		if eff.SustainActorID != "" {
			sustainer := w.actor(eff.SustainActorID)
			exhausted := sustainer == nil || sustainer.Dead
			if !exhausted && eff.SustainEnergyPerTick > 0 {
				if sustainer.Energy < eff.SustainEnergyPerTick {
					exhausted = true
				}
			}
			if !exhausted && len(eff.SustainSlotPerTick) > 0 {
				for slot, cost := range eff.SustainSlotPerTick {
					if sustainer.StateSlots[slot] < cost {
						exhausted = true
						break
					}
				}
			}
			if exhausted {
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: eff.SustainActorID,
					Kind:    "effect_exhausted",
					Action:  Action{Kind: "wait"},
					Payload: map[string]any{
						"text":        fmt.Sprintf("%s collapsed because sustainer %s ran out of sustaining resources", eff.Name, eff.SustainActorID),
						"effect_id":   eff.ID,
						"effect_spec": eff.SpecID,
						"anchor_kind": eff.Anchor.Kind,
						"anchor_id":   eff.Anchor.ID,
					},
				}
				w.NextID++
				events = append(events, ev)
				continue
			}
			if eff.SustainEnergyPerTick > 0 {
				sustainer.Energy -= eff.SustainEnergyPerTick
			}
			if len(eff.SustainSlotPerTick) > 0 {
				for slot, cost := range eff.SustainSlotPerTick {
					setActorSlotValue(sustainer, slot, sustainer.StateSlots[slot]-cost)
				}
			}
		}
		kept = append(kept, eff)
	}
	if len(kept) == 0 {
		w.ActiveEffects = nil
	} else {
		w.ActiveEffects = kept
	}
	return events
}

// ActorMeetsDeclarativeCosts checks whether actor `a` has enough energy, money, currencies, items,
// and state slots to execute `spec`.
func ActorMeetsDeclarativeCosts(a Actor, spec ActionSpec) (bool, string) {
	if a.Energy < spec.EnergyCost {
		return false, "insufficient energy for " + spec.ID
	}
	for curID, amt := range declarativeCurrencyCosts(spec) {
		balance := a.Currencies[curID]
		if curID == "coins" {
			balance = int64(a.Money)
		}
		if balance < amt {
			return false, fmt.Sprintf("insufficient %s (%d < %d) for %s", curID, balance, amt, spec.ID)
		}
	}
	for itemID, qty := range spec.ItemCosts {
		if a.Items[itemID] < qty {
			return false, fmt.Sprintf("requires %d %s for %s", qty, itemID, spec.ID)
		}
	}
	for slotID, amt := range spec.SlotCosts {
		if a.StateSlots[slotID] < amt {
			return false, fmt.Sprintf("insufficient state slot %s (%d < %d) for %s", slotID, a.StateSlots[slotID], amt, spec.ID)
		}
	}
	return true, ""
}

func (w *World) addActorStateSlot(a *Actor, slotID string, delta int) {
	if a == nil || slotID == "" || delta == 0 {
		return
	}
	spec, hasSpec := w.Catalog.StateSlots[slotID]
	next := clampSlotValue(a.StateSlots[slotID]+delta, spec, hasSpec)
	setActorSlotValue(a, slotID, next)
}

func (w *World) interruptTaskOnDamage(b *Actor, cause string) []Event {
	if b == nil || b.Task == nil {
		return nil
	}
	if b.Task.Dormant {
		act := b.Task.Action
		b.Task = nil
		return []Event{w.record(b, act, "dormant_interrupted", map[string]any{
			"text":        fmt.Sprintf("%s's dormant %s was interrupted by %s", b.Name, act.Kind, cause),
			"action_spec": act.Kind,
			"cause":       cause,
		})}
	}
	if cause == "combat" && b.Task.Action.Kind == "sleep" {
		b.Task = nil
	}
	return nil
}

func (w *World) deductDeclarativeCosts(a *Actor, spec ActionSpec) {
	a.Energy -= spec.EnergyCost
	for curID, amt := range declarativeCurrencyCosts(spec) {
		if amt == 0 {
			continue
		}
		if (curID == "" || curID == "coins") && len(w.Cohorts) > 0 {
			w.adjustActorCurrency(a, curID, -amt, false)
			w.creditTreasury(amt, "fee")
		} else {
			w.adjustActorCurrency(a, curID, -amt, true)
		}
	}
	for itemID, qty := range spec.ItemCosts {
		if qty > 0 && a.Items != nil {
			w.adjustActorItem(a, itemID, -qty, true)
		}
	}
	for slotID, amt := range spec.SlotCosts {
		if amt > 0 && a.StateSlots != nil {
			slotSpec, hasSpec := w.Catalog.StateSlots[slotID]
			next := clampSlotValue(a.StateSlots[slotID]-amt, slotSpec, hasSpec)
			setActorSlotValue(a, slotID, next)
		}
	}
}

// locationsWithinActionRange accepts exact co-location, declared coordinates, or
// an ordinary venue route with an explicit kilometer length. Unknown distance
// never grants range, and teleport routes do not establish physical proximity.
func (w *World) locationsWithinActionRange(fromDistrict, fromVenue, toDistrict, toVenue string, rangeKm int) bool {
	if fromDistrict == toDistrict && fromVenue == toVenue {
		return true
	}
	c1, ok1 := w.Map.GeoCoordOfLocation(fromDistrict, fromVenue)
	c2, ok2 := w.Map.GeoCoordOfLocation(toDistrict, toVenue)
	if ok1 && ok2 {
		return GeoDistanceKm(c1, c2) <= rangeKm
	}
	if fromVenue == "" || toVenue == "" {
		return false
	}
	dist := map[string]int{fromVenue: 0}
	visited := map[string]bool{}
	for {
		u := ""
		best := rangeKm
		for _, v := range w.Map.Venues {
			if d, known := dist[v.ID]; known && !visited[v.ID] && (d < best || u == "" && d == best) {
				u, best = v.ID, d
			}
		}
		if u == "" {
			return false
		}
		if u == toVenue {
			return true
		}
		visited[u] = true
		for _, route := range w.Map.VenueRoutesFrom(u) {
			if route.DistanceKm <= 0 || route.Medium == "teleport" || route.DistanceKm > rangeKm-best {
				continue
			}
			next := best + route.DistanceKm
			if old, known := dist[route.ToVenue]; !known || next < old {
				dist[route.ToVenue] = next
			}
		}
	}
}

func (w *World) declarativeTargetFailure(a Actor, x *Action, spec ActionSpec) string {
	inRange := func(district, venue string) bool {
		return w.locationsWithinActionRange(a.Location, a.VenueID, district, venue, spec.RangeKm)
	}
	switch spec.TargetKind {
	case "actor":
		b := w.actor(x.TargetID)
		if b == nil || b.Dead || b.ID == a.ID || b.InTransit() || w.Tick < b.JailedUntil {
			return "invalid or unavailable target actor"
		}
		if spec.RangeKm <= 0 {
			if !SamePlace(a, *b) {
				return "target actor is not in the same place"
			}
		} else if !SamePlace(a, *b) && !inRange(b.Location, b.VenueID) {
			return "target actor is out of range or distance is unavailable"
		}
	case "cohort":
		c := w.findCohort(x.TargetID)
		if c == nil || c.Population <= 0 {
			return "invalid or empty target cohort"
		}
		if spec.RangeKm <= 0 {
			if c.District != a.Location {
				return "target cohort is not in current district"
			}
		} else if !inRange(c.District, "") {
			return "target cohort is out of range or distance is unavailable"
		}
	case "venue":
		if x.Location == "" {
			x.Location = a.VenueID
		}
		v, ok := w.Map.VenueByID(x.Location)
		if !ok {
			return "unknown target venue: " + x.Location
		}
		if spec.RangeKm <= 0 {
			if a.VenueID != v.ID {
				return "target venue is not local"
			}
		} else if !inRange(v.DistrictID, v.ID) {
			return "target venue is out of range or distance is unavailable"
		}
	case "district":
		if x.Location == "" {
			x.Location = a.Location
		}
		if _, ok := w.Map.DistrictByID(x.Location); !ok {
			return "unknown target district: " + x.Location
		}
		if spec.RangeKm <= 0 {
			if x.Location != a.Location {
				return "target district is not local"
			}
		} else if !inRange(x.Location, "") {
			return "target district is out of range or distance is unavailable"
		}
	}
	return ""
}

// ActionSpecContext carries the read-only local state needed to evaluate an ActionSpec
// identically during candidate generation (Perception) and execution (World.Apply).
type ActionSpecContext struct {
	Actor         Actor
	Catalog       Catalog
	Map           WorldMap
	DistrictState *DistrictState
	KnownTechs    []string
}

// CheckActionSpecPrerequisites is the single canonical check for ActionSpec eligibility.
// Both option enumeration and action settlement call this check.
func CheckActionSpecPrerequisites(ctx ActionSpecContext, spec ActionSpec) string {
	a := ctx.Actor
	if len(spec.WorldLawDelta) > 0 && !spec.AllowRewriteWorldLaw {
		return "action is not authorized to rewrite fundamental world laws"
	}
	if spec.RequireOutdoors && a.InsideHouse != "" {
		return spec.ID + " requires being outdoors"
	}
	if spec.RequireIndoors && a.InsideHouse == "" {
		return spec.ID + " requires being indoors"
	}
	if spec.RequiredVenueFeature != "" && !ctx.Map.VenueOrDistrictHasFeature(a.VenueID, a.Location, spec.RequiredVenueFeature) {
		return fmt.Sprintf("%s requires location feature %s", spec.ID, spec.RequiredVenueFeature)
	}
	if spec.Profession != "" {
		minLv := spec.MinProfessionLevel
		if minLv <= 0 {
			minLv = 1
		}
		if a.Professions[spec.Profession] < minLv {
			return fmt.Sprintf("%s requires %s level %d", spec.ID, spec.Profession, minLv)
		}
	}
	if len(spec.RequiredFormIDs) > 0 && !ActorHasTag(spec.RequiredFormIDs, a.FormID) {
		return fmt.Sprintf("%s requires form %s", spec.ID, strings.Join(spec.RequiredFormIDs, " or "))
	}
	for _, techID := range spec.RequiredTechs {
		if !ActorHasTag(ctx.KnownTechs, techID) {
			return fmt.Sprintf("%s requires tech %s", spec.ID, techID)
		}
	}
	if len(spec.MinAttributes) > 0 {
		if ok, reason := a.Attributes.Meets(spec.MinAttributes); !ok {
			return reason
		}
	}
	if spec.MinHealth > 0 && a.Health < spec.MinHealth {
		return fmt.Sprintf("%s requires health >= %d", spec.ID, spec.MinHealth)
	}
	if spec.ToolItem != "" && !actorHasItemOrEquipped(a, spec.ToolItem) {
		return fmt.Sprintf("%s requires tool %s", spec.ID, spec.ToolItem)
	}
	if spec.ResourcePool != "" {
		need := spec.ResourceDraw
		if need < 1 {
			need = 1
		}
		if ctx.DistrictState == nil || ctx.DistrictState.AvailableResource(spec.ResourcePool) < need {
			return fmt.Sprintf("insufficient %s in %s", spec.ResourcePool, a.Location)
		}
	}
	if spec.RequiredThreatID != "" {
		minAct := spec.MinThreatActivity
		if minAct < 1 {
			minAct = 1
		}
		if ctx.DistrictState == nil || ctx.DistrictState.ThreatActivity[spec.RequiredThreatID] < minAct {
			return fmt.Sprintf("%s requires active threat %s (>= %d)", spec.ID, spec.RequiredThreatID, minAct)
		}
	}
	if ok, reason := ActorMeetsDeclarativeCosts(a, spec); !ok {
		return reason
	}
	return ""
}

// ActionSpecBlockReason returns a non-empty reason if actor `a` cannot execute `spec` in `w`.
func (w *World) ActionSpecBlockReason(a Actor, x Action, spec ActionSpec) string {
	var dsPtr *DistrictState
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[a.Location]; ok {
			dsPtr = &ds
		}
	}
	if reason := CheckActionSpecPrerequisites(ActionSpecContext{
		Actor:         a,
		Catalog:       w.Catalog,
		Map:           w.Map,
		DistrictState: dsPtr,
		KnownTechs:    w.ActorKnownTechs(a.ID),
	}, spec); reason != "" {
		return reason
	}
	if spec.RequiredVenueFeature != "" {
		venueID := a.VenueID
		if spec.TargetKind == "venue" && x.Location != "" {
			venueID = x.Location
		}
		if w.isVenuePowerBlocked(venueID) {
			return "venue lacks power: " + venueID
		}
	}
	if spec.TransitionID != "" {
		trans, ok := w.Catalog.Transitions[spec.TransitionID]
		if !ok {
			return "unknown transition: " + spec.TransitionID
		}
		if reason := w.transitionRequirementReason(a, trans); reason != "" {
			return reason
		}
	}
	return ""
}

// ActionSpecBlockReason returns a non-empty reason if the perceiving actor cannot execute `spec`.
func (p Perception) ActionSpecBlockReason(spec ActionSpec) string {
	return CheckActionSpecPrerequisites(ActionSpecContext{
		Actor:         p.Self,
		Catalog:       p.Catalog,
		Map:           p.Map,
		DistrictState: p.DistrictState,
		KnownTechs:    p.KnownTechs,
	}, spec)
}

func (w *World) applyDeclarativeAction(a *Actor, x Action, spec ActionSpec) []Event {
	emitFail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if reason := w.ActionSpecBlockReason(*a, x, spec); reason != "" {
		return emitFail(reason)
	}

	// Validate target before deducting costs.
	if reason := w.declarativeTargetFailure(*a, &x, spec); reason != "" {
		return emitFail(reason)
	}

	w.deductDeclarativeCosts(a, spec)
	if spec.ResourcePool != "" {
		need := spec.ResourceDraw
		if need < 1 {
			need = 1
		}
		w.drawDistrictResource(a.Location, spec.ResourcePool, need)
	}
	if spec.ToolItem != "" {
		w.wearToolItem("actor", a.ID, spec.ToolItem, 1)
	}
	ticks := spec.Ticks
	if ticks < 1 {
		ticks = 1
	}
	task := Task{
		Action:      x,
		TotalTicks:  ticks,
		Remaining:   ticks - 1,
		StartedTick: w.Tick,
		Dormant:     spec.Dormant,
	}
	if task.Remaining <= 0 {
		return w.finishDeclarativeAction(a, task, spec)
	}
	a.Task = &task
	kind := "task_started"
	if spec.Dormant {
		kind = "dormant_started"
	}
	return []Event{w.record(a, x, kind, map[string]any{
		"text":            fmt.Sprintf("%s began %s (%d ticks, dormant=%v)", a.Name, spec.Name, ticks, spec.Dormant),
		"action_spec":     spec.ID,
		"dormant":         spec.Dormant,
		"remaining_ticks": task.Remaining,
	})}
}

func (w *World) finishDeclarativeAction(a *Actor, t Task, spec ActionSpec) []Event {
	x := t.Action
	if blocked, reason := w.IsActionBlockedByWorldLaw(*a, x); blocked {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if reason := w.declarativeTargetFailure(*a, &x, spec); reason != "" {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	var events []Event

	// 1. State slot, health, energy, and item gains.
	for slotID, gain := range spec.SlotGains {
		w.addActorStateSlot(a, slotID, gain)
	}
	if spec.HealthGain != 0 {
		a.Health = maxInt(0, min(MaxHealth, a.Health+spec.HealthGain))
	}
	if spec.EnergyGain != 0 {
		a.Energy = maxInt(0, min(MaxEnergy, a.Energy+spec.EnergyGain))
	}
	var primaryItem string
	var primaryQty int
	if len(spec.ItemGains) > 0 {
		itemIDs := make([]string, 0, len(spec.ItemGains))
		for itemID := range spec.ItemGains {
			itemIDs = append(itemIDs, itemID)
		}
		sort.Strings(itemIDs)
		for _, itemID := range itemIDs {
			qty := spec.ItemGains[itemID]
			if qty <= 0 {
				continue
			}
			w.adjustActorItem(a, itemID, qty, true)
			if primaryItem == "" {
				primaryItem, primaryQty = itemID, qty
			}
		}
	}

	// 1b. District threat, morale, and resource updates.
	if w.DistrictStates != nil {
		ds, hasDS := w.DistrictStates[a.Location]
		if hasDS {
			threatTarget := spec.ThreatID
			if threatTarget == "" {
				threatTarget = spec.RequiredThreatID
			}
			if threatTarget != "" && spec.ReduceThreat > 0 && ds.ThreatActivity != nil {
				cur := ds.ThreatActivity[threatTarget] - spec.ReduceThreat
				if cur < 0 {
					cur = 0
				}
				ds.ThreatActivity[threatTarget] = cur
			}
			if spec.MoraleDelta != 0 {
				ds.MoraleBonus += spec.MoraleDelta
			}
			if len(spec.DistrictResourceGains) > 0 {
				if ds.Resources == nil {
					ds.Resources = map[string]int64{}
				}
				poolIDs := make([]string, 0, len(spec.DistrictResourceGains))
				for poolID := range spec.DistrictResourceGains {
					poolIDs = append(poolIDs, poolID)
				}
				sort.Strings(poolIDs)
				for _, poolID := range poolIDs {
					delta := spec.DistrictResourceGains[poolID]
					if delta != 0 {
						ds.Resources[poolID] = maxInt64(0, ds.Resources[poolID]+delta)
						w.recordEcologicalChange(a.Location, poolID, delta)
					}
				}
			}
			w.DistrictStates[a.Location] = ds
		}
	}

	// 1c. Tech unlocks & profession XP.
	for _, techID := range spec.UnlockTechs {
		if tech, ok := w.Catalog.Techs[techID]; ok {
			events = append(events, w.unlockTechForActor(a, x, tech)...)
		}
	}
	xpProf := spec.XPProfession
	if xpProf == "" {
		xpProf = spec.Profession
	}
	if xpProf != "" && spec.ProfessionXP > 0 {
		events = append(events, w.grantProfessionXP(a, x, xpProf, spec.ProfessionXP, 1)...)
	}

	// 2. Spawn sustained ActiveEffect if declared.
	if spec.SpawnEffect != nil {
		es := *spec.SpawnEffect
		anchor := EntityRef{Kind: "actor", ID: a.ID}
		switch es.AnchorKind {
		case "target_actor":
			anchor = EntityRef{Kind: "actor", ID: x.TargetID}
		case "venue":
			vID := x.Location
			if vID == "" {
				vID = a.VenueID
			}
			anchor = EntityRef{Kind: "venue", ID: vID}
		case "district":
			dID := x.Location
			if dID == "" {
				dID = a.Location
			}
			anchor = EntityRef{Kind: "district", ID: dID}
		}
		dur := es.DurationTicks
		if dur <= 0 {
			dur = TicksPerDay
		}
		sustainID := ""
		if es.SustainEnergyPerTick > 0 || len(es.SustainSlotPerTick) > 0 {
			sustainID = a.ID
		}
		eff := ActiveEffect{
			ID:                        w.NextID,
			SpecID:                    es.ID,
			Name:                      es.Name,
			Source:                    EntityRef{Kind: "actor", ID: a.ID},
			Anchor:                    anchor,
			SustainActorID:            sustainID,
			CreatedTick:               w.Tick,
			ExpiresTick:               w.Tick + dur,
			Integrity:                 es.MaxIntegrity,
			MaxIntegrity:              es.MaxIntegrity,
			SustainEnergyPerTick:      es.SustainEnergyPerTick,
			SustainSlotPerTick:        cloneIntMap(es.SustainSlotPerTick),
			DefenseByType:             cloneDefenseMap(es.DefenseByType),
			AbsorbDamageWithIntegrity: es.AbsorbDamageWithIntegrity,
			ShieldingBonus:            es.ShieldingBonus,
			GrantTags:                 append([]string(nil), es.GrantTags...),
		}
		w.NextID++
		w.ActiveEffects = append(w.ActiveEffects, eff)
		observers := w.awakeSamePlaceObservers(*a, a.ID)
		events = append(events, w.record(a, x, "effect_spawned", map[string]any{
			"text":        fmt.Sprintf("%s activated %s on %s:%s (integrity %d, expires tick %d)", a.Name, eff.Name, anchor.Kind, anchor.ID, eff.Integrity, eff.ExpiresTick),
			"effect_id":   eff.ID,
			"effect_spec": eff.SpecID,
			"anchor_kind": anchor.Kind,
			"anchor_id":   anchor.ID,
			"integrity":   eff.Integrity,
		}, observers...))
	}

	// 3. Multi-dimensional attack execution (single actor, venue AoE, or cross-scale PopulationCohort).
	if spec.Attack != nil {
		atk := spec.Attack
		if spec.TargetKind == "cohort" || atk.AreaScope == "cohort" {
			c := w.findCohort(x.TargetID)
			if c != nil && c.Population > 0 {
				_, _, _, profCohortDmg := w.Catalog.ActorCombatProfile(*a)
				cas := atk.CohortCasualties
				if cas <= 0 {
					cas = profCohortDmg
				}
				if cas <= 0 {
					cas = 25
				}
				wounded, dead, _ := w.applyCohortCasualtiesWithCause(c, cas*2, cas, 0, "combat")
				observers := w.awakeSamePlaceObservers(*a, a.ID)
				events = append(events, w.record(a, x, "cohort_struck", map[string]any{
					"text":        fmt.Sprintf("%s unleashed %s upon %s (%d wounded, %d killed)", a.Name, spec.Name, c.Name, wounded, dead),
					"action_spec": spec.ID,
					"cohort_id":   c.ID,
					"wounded":     wounded,
					"deaths":      dead,
				}, observers...))
			}
		} else if atk.AreaScope == "venue" {
			targetVenue := x.Location
			if targetVenue == "" {
				targetVenue = a.VenueID
			}
			var hitCount, totalDmg int
			var deathEvs []Event
			for i := range w.Actors {
				b := &w.Actors[i]
				if b.Dead || b.ID == a.ID || b.VenueID != targetVenue {
					continue
				}
				dmg, _, _, broken := w.resolveMultiDimCombatHit(*a, b, atk, x)
				events = append(events, broken...)
				if dmg > 0 {
					if dmg > b.Health {
						dmg = b.Health
					}
					b.Health -= dmg
					var interruptEvs []Event
					if b.Health > 0 {
						interruptEvs = w.interruptTaskOnDamage(b, "combat")
					}
					hitCount++
					totalDmg += dmg
					deathEvs = append(deathEvs, interruptEvs...)
					deathEvs = append(deathEvs, w.markDeadIfZero(b, a, x, "combat")...)
				}
			}
			events = append(events, w.record(a, x, "action_resolved", map[string]any{
				"text":        fmt.Sprintf("%s unleashed %s across %s (%d targets hit, %d total damage)", a.Name, spec.Name, targetVenue, hitCount, totalDmg),
				"action_spec": spec.ID,
				"targets_hit": hitCount,
				"damage":      totalDmg,
			}))
			events = append(events, deathEvs...)
		} else if b := w.actor(x.TargetID); b != nil && !b.Dead {
			dmg, absorbed, immune, broken := w.resolveMultiDimCombatHit(*a, b, atk, x)
			events = append(events, broken...)
			if dmg > b.Health {
				dmg = b.Health
			}
			b.Health -= dmg
			var interruptEvs []Event
			if dmg > 0 && b.Health > 0 {
				interruptEvs = w.interruptTaskOnDamage(b, "combat")
			}
			if b.Trust == nil {
				b.Trust = map[string]int{}
			}
			b.Trust[a.ID] -= 2
			events = append(events, w.record(a, x, "attacked", map[string]any{
				"text":           fmt.Sprintf("%s struck %s with %s (%d damage, %d absorbed, immune=%v)", a.Name, b.Name, spec.Name, dmg, absorbed, immune),
				"action_spec":    spec.ID,
				"damage":         dmg,
				"absorbed":       absorbed,
				"immune_blocked": immune,
			}, b))
			events = append(events, interruptEvs...)
			events = append(events, w.markDeadIfZero(b, a, x, "combat")...)
		}
	}

	// Phase 3 & 4 hooks: spec.TransitionID (executeTransition) & spec.AllowRewriteWorldLaw (WorldLaws).
	if spec.TransitionID != "" {
		transitionAction := Action{Kind: "attempt_transition", TransitionID: spec.TransitionID, BasisEventID: x.BasisEventID}
		events = append(events, w.applyAttemptTransition(a, transitionAction)...)
	}
	if len(spec.WorldLawDelta) > 0 {
		lawEvs, _ := w.ApplyWorldLawChange(a, x, spec.AllowRewriteWorldLaw, spec.WorldLawDelta)
		events = append(events, lawEvs...)
	}

	if primaryItem != "" || len(events) == 0 {
		payload := map[string]any{
			"text":        fmt.Sprintf("%s completed %s", a.Name, spec.Name),
			"action_spec": spec.ID,
			"dormant":     t.Dormant,
		}
		if primaryItem != "" {
			payload["item"] = primaryItem
			payload["quantity"] = primaryQty
			payload["item_gains"] = cloneIntMap(spec.ItemGains)
			payload["observation"] = Observation{Kind: "item_produced", Item: primaryItem, Location: a.Location, Quantity: primaryQty}
		}
		events = append([]Event{w.record(a, x, "action_resolved", payload)}, events...)
	}
	return events
}

func actorHasItemOrEquipped(a Actor, itemID string) bool {
	if a.Items[itemID] > 0 {
		return true
	}
	for _, eq := range a.Equipped {
		if eq == itemID {
			return true
		}
	}
	return false
}

// CanEquipItem checks whether `a` meets the WorldPack prerequisites to equip `itemID` into its declared slot.
func CanEquipItem(cat Catalog, a Actor, itemID string) (bool, string) {
	if itemID == "" {
		return false, "equip requires item"
	}
	spec, ok := cat.Items[itemID]
	if !ok {
		return false, "unknown item: " + itemID
	}
	if spec.EquipSlot == "" {
		return false, itemID + " is not equippable"
	}
	if a.Items[itemID] < 1 {
		return false, "no " + itemID + " in inventory"
	}
	if slotSpec, declared := cat.EquipSlots[spec.EquipSlot]; declared && len(slotSpec.GrantedByTags) > 0 {
		tags := cat.ActorEffectiveTags(a)
		allowed := false
		for _, gt := range slotSpec.GrantedByTags {
			if ActorHasTag(tags, gt) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, "equip slot " + spec.EquipSlot + " is not available to this actor"
		}
	}
	if len(spec.EquipRequiredForms) > 0 && !ActorHasTag(spec.EquipRequiredForms, a.FormID) {
		return false, itemID + " requires form " + strings.Join(spec.EquipRequiredForms, " or ")
	}
	if len(spec.EquipRequiredTags) > 0 {
		tags := cat.ActorEffectiveTags(a)
		for _, reqTag := range spec.EquipRequiredTags {
			if !ActorHasTag(tags, reqTag) {
				return false, itemID + " requires tag " + reqTag
			}
		}
	}
	if len(spec.EquipMinAttributes) > 0 {
		if ok, reason := a.Attributes.Meets(spec.EquipMinAttributes); !ok {
			return false, reason
		}
	}
	return true, ""
}

func (w *World) applyEquip(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if ok, reason := CanEquipItem(w.Catalog, *a, x.Item); !ok {
		return fail(reason)
	}
	spec := w.Catalog.Items[x.Item]
	if a.Equipped == nil {
		a.Equipped = map[string]string{}
	}
	prev := a.Equipped[spec.EquipSlot]
	if prev == x.Item {
		return fail(x.Item + " is already equipped in " + spec.EquipSlot)
	}
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	a.Items[x.Item]--
	if a.Items[x.Item] == 0 {
		delete(a.Items, x.Item)
	}
	if prev != "" {
		a.Items[prev]++
	}
	a.Equipped[spec.EquipSlot] = x.Item
	return []Event{w.record(a, x, "item_equipped", map[string]any{
		"text":          fmt.Sprintf("%s equipped %s into %s", a.Name, x.Item, spec.EquipSlot),
		"item":          x.Item,
		"equip_slot":    spec.EquipSlot,
		"replaced_item": prev,
	})}
}

func (w *World) applyUnequip(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	slot := x.TargetID
	if slot == "" && x.Item != "" {
		slot = w.Catalog.Items[x.Item].EquipSlot
	}
	if slot == "" || a.Equipped[slot] == "" {
		return fail("no item equipped in slot")
	}
	itemID := a.Equipped[slot]
	delete(a.Equipped, slot)
	if len(a.Equipped) == 0 {
		a.Equipped = nil
	}
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	a.Items[itemID]++
	return []Event{w.record(a, x, "item_unequipped", map[string]any{
		"text":       fmt.Sprintf("%s unequipped %s from %s", a.Name, itemID, slot),
		"item":       itemID,
		"equip_slot": slot,
	})}
}

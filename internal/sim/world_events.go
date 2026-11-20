package sim

import (
	"fmt"
	"sort"
)

// WorldEventSpec declaratively defines a major world-shaking event (artifact/relic emergence,
// terrain/route tectonic shift, or cross-district external invasion) in a 100% genre-agnostic format.
type WorldEventSpec struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Category            string           `json:"category"` // "artifact_emergence" | "terrain_shift" | "external_invasion"
	TargetFeatures      []string         `json:"target_features,omitempty"`
	ProbPermillePerDay  int              `json:"prob_permille_per_day"`
	MinTick             int64            `json:"min_tick,omitempty"`
	CooldownTicks       int64            `json:"cooldown_ticks,omitempty"`
	DurationTicks       int64            `json:"duration_ticks,omitempty"`
	Intensity           int              `json:"intensity,omitempty"`
	RewardItem          string           `json:"reward_item,omitempty"`
	RewardQty           int              `json:"reward_qty,omitempty"`
	RewardCoins         int              `json:"reward_coins,omitempty"`
	RequiredPowerTier   int              `json:"required_power_tier,omitempty"`
	SeverMedium         string           `json:"sever_medium,omitempty"`
	AddFeatures         []string         `json:"add_features,omitempty"`
	ResourceDeposits    map[string]int64 `json:"resource_deposits,omitempty"`
	InvasionThreatID    string           `json:"invasion_threat_id,omitempty"`
	InvasionThreatBoost int              `json:"invasion_threat_boost,omitempty"`
	BountyBoost         int              `json:"bounty_boost,omitempty"`
	CasualtyPermille    int              `json:"casualty_permille,omitempty"`
	Damage              int              `json:"damage,omitempty"`
}

// ActiveWorldEvent tracks a live dynamic world event instance serialized on World for deterministic replay.
type ActiveWorldEvent struct {
	InstanceID        string `json:"instance_id"`
	SpecID            string `json:"spec_id"`
	Name              string `json:"name"`
	Category          string `json:"category"`
	DistrictID        string `json:"district_id"`
	StartedTick       int64  `json:"started_tick"`
	ExpiresTick       int64  `json:"expires_tick"`
	Intensity         int    `json:"intensity"`
	RewardItem        string `json:"reward_item,omitempty"`
	RewardQty         int    `json:"reward_qty,omitempty"`
	RewardCoins       int    `json:"reward_coins,omitempty"`
	RequiredPowerTier int    `json:"required_power_tier,omitempty"`
	ClaimedBy         string `json:"claimed_by,omitempty"`
	SeveredMedium     string `json:"severed_medium,omitempty"`
	InvasionThreatID  string `json:"invasion_threat_id,omitempty"`
	RootEventID       int64  `json:"root_event_id,omitempty"`
}

func cloneWorldEventSpec(s WorldEventSpec) WorldEventSpec {
	out := s
	if s.TargetFeatures != nil {
		out.TargetFeatures = append([]string(nil), s.TargetFeatures...)
	}
	if s.AddFeatures != nil {
		out.AddFeatures = append([]string(nil), s.AddFeatures...)
	}
	if s.ResourceDeposits != nil {
		out.ResourceDeposits = cloneInt64Map(s.ResourceDeposits)
	}
	return out
}

// isRouteSevered reports whether an active terrain_shift world event currently severs
// routes of `medium` touching either endpoint unless the traveler's locomotion mode can fly over ("air").
func (w *World) isRouteSevered(fromDist, toDist, medium, modeID string) (bool, string) {
	if len(w.ActiveWorldEvents) == 0 || fromDist == "" || toDist == "" || fromDist == toDist {
		return false, ""
	}
	if medium == "" {
		medium = "road"
	}
	canFly := medium == "air"
	if !canFly && modeID != "" {
		if loc, ok := w.Catalog.LocomotionModes[modeID]; ok && ActorHasTag(loc.AllowedMediums, "air") {
			canFly = true
		}
	}
	if canFly {
		return false, ""
	}
	for _, ev := range w.ActiveWorldEvents {
		if w.Tick >= ev.ExpiresTick || ev.SeveredMedium == "" {
			continue
		}
		if ev.DistrictID != fromDist && ev.DistrictID != toDist {
			continue
		}
		if ev.SeveredMedium == medium || (ev.SeveredMedium == "road" && medium == "land") || (ev.SeveredMedium == "land" && medium == "road") {
			return true, fmt.Sprintf("route (%s) between %s and %s is severed by %s", medium, fromDist, toDist, ev.Name)
		}
	}
	return false, ""
}

// stepWorldEvents expires finished macro world events and evaluates daily triggers at 08:00.
func (w *World) stepWorldEvents() []Event {
	var events []Event
	if len(w.ActiveWorldEvents) > 0 {
		kept := w.ActiveWorldEvents[:0]
		for _, ev := range w.ActiveWorldEvents {
			if w.Tick < ev.ExpiresTick {
				kept = append(kept, ev)
			}
		}
		if len(kept) == 0 {
			w.ActiveWorldEvents = nil
		} else {
			w.ActiveWorldEvents = kept
		}
	}
	if len(w.Catalog.WorldEvents) == 0 || w.DistrictStates == nil {
		return nil
	}
	clock := ClockAt(w.Tick)
	if clock.Hour != 8 || clock.Minute != 0 {
		return nil
	}
	for _, spec := range w.Catalog.WorldEvents {
		if w.Tick < spec.MinTick || spec.ProbPermillePerDay <= 0 {
			continue
		}
		// Skip if already active or on cooldown.
		blocked := false
		for _, act := range w.ActiveWorldEvents {
			if act.SpecID == spec.ID && (w.Tick < act.ExpiresTick || (spec.CooldownTicks > 0 && w.Tick-act.StartedTick < spec.CooldownTicks)) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		roll := int(worldHash(w.ID, w.Tick, "wev:"+spec.ID) % 1000)
		if roll >= spec.ProbPermillePerDay {
			continue
		}
		var candidates []string
		for _, d := range w.Map.Districts {
			if len(spec.TargetFeatures) == 0 {
				candidates = append(candidates, d.ID)
				continue
			}
			for _, tf := range spec.TargetFeatures {
				if w.Map.DistrictHasFeature(d.ID, tf) {
					candidates = append(candidates, d.ID)
					break
				}
			}
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Strings(candidates)
		pickIdx := int(worldHash(w.ID, w.Tick, "wev_dist:"+spec.ID) % uint64(len(candidates)))
		events = append(events, w.TriggerWorldEvent(spec.ID, candidates[pickIdx])...)
		break // at most 1 major world event per day
	}
	return events
}

// TriggerWorldEvent deterministically activates a declared WorldEventSpec at districtID,
// applies its physical effects (relic spawn, terrain/route shift, or external invasion),
// and distributes epistemic eyewitness/sensory Observations strictly to qualifying observers.
func (w *World) TriggerWorldEvent(specID, districtID string) []Event {
	var spec *WorldEventSpec
	for i := range w.Catalog.WorldEvents {
		if w.Catalog.WorldEvents[i].ID == specID {
			spec = &w.Catalog.WorldEvents[i]
			break
		}
	}
	if spec == nil {
		return nil
	}
	if districtID == "" && len(w.Map.Districts) > 0 {
		districtID = w.Map.Districts[0].ID
	}
	dur := spec.DurationTicks
	if dur <= 0 {
		dur = 3 * TicksPerDay
	}
	intensity := spec.Intensity
	if intensity <= 0 {
		intensity = 55
	}

	instanceID := fmt.Sprintf("%s@%d", spec.ID, w.Tick)
	active := ActiveWorldEvent{
		InstanceID:        instanceID,
		SpecID:            spec.ID,
		Name:              spec.Name,
		Category:          spec.Category,
		DistrictID:        districtID,
		StartedTick:       w.Tick,
		ExpiresTick:       w.Tick + dur,
		Intensity:         intensity,
		RewardItem:        spec.RewardItem,
		RewardQty:         spec.RewardQty,
		RewardCoins:       spec.RewardCoins,
		RequiredPowerTier: spec.RequiredPowerTier,
		SeveredMedium:     spec.SeverMedium,
		InvasionThreatID:  spec.InvasionThreatID,
	}

	var events []Event
	// 1. Apply terrain_shift physical mutations (new features + finite resource deposits).
	if len(spec.AddFeatures) > 0 {
		for di := range w.Map.Districts {
			if w.Map.Districts[di].ID == districtID {
				for _, feat := range spec.AddFeatures {
					w.Map.Districts[di].Features = appendUnique(w.Map.Districts[di].Features, feat)
				}
				break
			}
		}
	}
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[districtID]; ok {
			if len(spec.ResourceDeposits) > 0 {
				if ds.Resources == nil {
					ds.Resources = map[string]int64{}
				}
				if ds.MaxResources == nil {
					ds.MaxResources = map[string]int64{}
				}
				rKeys := make([]string, 0, len(spec.ResourceDeposits))
				for k := range spec.ResourceDeposits {
					rKeys = append(rKeys, k)
				}
				sort.Strings(rKeys)
				for _, poolID := range rKeys {
					amt := spec.ResourceDeposits[poolID]
					if amt > 0 {
						ds.Resources[poolID] += amt
						if ds.Resources[poolID] > ds.MaxResources[poolID] {
							ds.MaxResources[poolID] = ds.Resources[poolID]
						}
						w.recordEcologicalChange(districtID, poolID, amt)
					}
				}
			}
			// 2. Apply external_invasion threat surge & bounty escalation across district(s).
			if spec.Category == "external_invasion" {
				threatID := spec.InvasionThreatID
				if threatID == "" && len(w.Catalog.Encounters) > 0 {
					threatID = w.Catalog.Encounters[0].ID
				}
				if threatID != "" {
					if ds.ThreatActivity == nil {
						ds.ThreatActivity = map[string]int{}
					}
					boost := spec.InvasionThreatBoost
					if boost <= 0 {
						boost = 45
					}
					ds.ThreatActivity[threatID] += boost
					active.InvasionThreatID = threatID
				}
				if spec.BountyBoost > 0 {
					ds.BountyPool += spec.BountyBoost
				}
			}
			w.DistrictStates[districtID] = ds
		}
	}

	// 3. Apply immediate shock casualties if declared (terrain_shift quake / external_invasion vanguard).
	wounded, dead := 0, 0
	if spec.CasualtyPermille > 0 {
		cause := "disaster"
		if spec.Category == "external_invasion" {
			cause = "beast"
		}
		dmg := maxInt(6, spec.Damage)
		for i := range w.Actors {
			a := &w.Actors[i]
			if a.Dead || a.Location != districtID || a.InTransit() {
				continue
			}
			mit := w.actorRiskMitigations(*a, "", "")
			effDmg := maxInt(3, (dmg*(100-mit.tierDefPct))/100)
			wn := int64(spec.CasualtyPermille * (100 - mit.tierDefPct/2))
			dn := int64(spec.CasualtyPermille*(100-mit.tierDefPct)) / 10
			cw, cd, impactEvs := w.applyEcologyActorExposure(a, wn, dn, 100000, effDmg, "wev:"+instanceID, cause)
			wounded += cw
			dead += cd
			events = append(events, impactEvs...)
		}
	}

	ev := Event{
		ID:      w.NextID,
		Tick:    w.Tick,
		ActorID: "world",
		Kind:    "world_event_triggered",
		Action:  Action{Kind: "wait", Location: districtID},
		Payload: map[string]any{
			"text":               fmt.Sprintf("Major world event [%s] (%s) erupted at %s (intensity %d)", spec.Name, spec.Category, districtID, intensity),
			"instance_id":        instanceID,
			"event_id":           spec.ID,
			"event_name":         spec.Name,
			"category":           spec.Category,
			"district":           districtID,
			"intensity":          intensity,
			"reward_item":        active.RewardItem,
			"reward_qty":         active.RewardQty,
			"reward_coins":       active.RewardCoins,
			"severed_medium":     active.SeveredMedium,
			"invasion_threat_id": active.InvasionThreatID,
			"wounded":            wounded,
			"deaths":             dead,
		},
	}
	w.NextID++
	active.RootEventID = ev.ID
	w.ActiveWorldEvents = append(w.ActiveWorldEvents, active)

	witnesses := w.distributeWorldEventObservations(ev, active)
	ev.Payload["witnesses"] = witnesses
	events = append([]Event{ev}, events...)
	return events
}

// distributeWorldEventObservations grants first-hand Observation{Kind:"world_event"} ONLY to
// on-site witnesses or remote observers with qualifying SensorySpec (strict epistemic isolation).
func (w *World) distributeWorldEventObservations(ev Event, active ActiveWorldEvent) []string {
	regID := w.Map.GeoRegionIDForLocation(active.DistrictID, "")
	siteCoord, hasSiteCoord := w.Map.GeoCoordOfLocation(active.DistrictID, "")
	var witnessIDs []string
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead || w.Tick < a.JailedUntil {
			continue
		}
		if a.Task != nil && (a.Task.Action.Kind == "sleep" || a.Task.Dormant) {
			continue
		}
		actorRegID := w.Map.GeoRegionIDForLocation(a.Location, a.VenueID)
		onSite := a.Location == active.DistrictID || (regID != "" && actorRegID == regID && active.Intensity >= 65)
		indoor, encShielding := w.actorEnclosure(*a)
		modality := ""
		if onSite && (!indoor || encShielding < active.Intensity) {
			modality = "eyewitness"
		} else if len(w.Catalog.SensoryModes) > 0 && hasSiteCoord {
			if actorCoord, ok := w.Map.GeoCoordOfLocation(a.Location, a.VenueID); ok {
				distKm := GeoDistanceKm(actorCoord, siteCoord)
				tags := w.actorEffectiveTags(*a)
				modeKeys := make([]string, 0, len(w.Catalog.SensoryModes))
				for k := range w.Catalog.SensoryModes {
					modeKeys = append(modeKeys, k)
				}
				sort.Strings(modeKeys)
				for _, k := range modeKeys {
					spec := w.Catalog.SensoryModes[k]
					if distKm > spec.RangeKm {
						continue
					}
					granted := false
					for _, gt := range spec.GrantedByTags {
						if ActorHasTag(tags, gt) {
							granted = true
							break
						}
					}
					if !granted || (indoor && !spec.PenetratesIndoor && encShielding > spec.PenetrationPower) {
						continue
					}
					modality = spec.ID
					break
				}
			}
		}
		if modality == "" {
			continue
		}
		obs := Observation{
			Kind:      "world_event",
			Modality:  modality,
			SubjectID: active.InstanceID,
			ObjectID:  active.Category,
			Item:      active.RewardItem,
			Location:  active.DistrictID,
			Quantity:  active.Intensity,
		}
		mem := Memory{
			Kind:        "world_event_triggered",
			Tick:        w.Tick,
			EventID:     ev.ID,
			RootEventID: ev.ID,
			Source:      "world",
			TargetID:    active.DistrictID,
			Text:        fmt.Sprintf("Witnessed %s (%s) at %s via %s", active.Name, active.Category, active.DistrictID, modality),
			Observation: &obs,
		}
		a.Memories = appendMemory(a.Memories, mem)
		witnessIDs = append(witnessIDs, a.ID)
	}
	return witnessIDs
}

// applyClaimWorldEvent lets an outdoor co-located actor claim an unclaimed artifact_emergence world event.
func (w *World) applyClaimWorldEvent(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.InsideHouse != "" {
		return fail("must step outdoors to claim world event discovery")
	}
	if a.Energy < 8 {
		return fail("insufficient energy to claim world event discovery")
	}
	var targetIdx = -1
	for i := range w.ActiveWorldEvents {
		ev := w.ActiveWorldEvents[i]
		if w.Tick >= ev.ExpiresTick || ev.Category != "artifact_emergence" || ev.ClaimedBy != "" {
			continue
		}
		if ev.DistrictID != a.Location {
			continue
		}
		if x.TargetID == "" || ev.InstanceID == x.TargetID || ev.SpecID == x.TargetID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return fail("no unclaimed artifact emergence event in current district")
	}
	ev := &w.ActiveWorldEvents[targetIdx]
	if ev.RequiredPowerTier > 0 && w.Catalog.ActorPowerTier(*a) < ev.RequiredPowerTier {
		return fail(fmt.Sprintf("requires power tier >= %d to claim %s", ev.RequiredPowerTier, ev.Name))
	}
	a.Energy -= 8
	ev.ClaimedBy = a.ID
	if ev.RewardItem != "" && ev.RewardQty > 0 {
		w.adjustActorItem(a, ev.RewardItem, ev.RewardQty, true)
	}
	if ev.RewardCoins > 0 {
		w.adjustActorCurrency(a, "coins", int64(ev.RewardCoins), true)
	}
	obs := Observation{
		Kind:      "world_event_claimed",
		Modality:  "eyewitness",
		SubjectID: a.ID,
		ObjectID:  ev.InstanceID,
		Item:      ev.RewardItem,
		Location:  a.Location,
		Quantity:  maxInt(1, ev.RewardQty),
	}
	var witnesses []*Actor
	for i := range w.Actors {
		b := &w.Actors[i]
		if b.ID != a.ID && !b.Dead && SamePlace(*a, *b) {
			witnesses = append(witnesses, b)
		}
	}
	claimEv := w.record(a, x, "world_event_claimed", map[string]any{
		"text":         fmt.Sprintf("%s claimed %s at %s (+%d %s, +%d coins)", a.Name, ev.Name, a.Location, ev.RewardQty, ev.RewardItem, ev.RewardCoins),
		"instance_id":  ev.InstanceID,
		"event_id":     ev.SpecID,
		"event_name":   ev.Name,
		"district":     a.Location,
		"reward_item":  ev.RewardItem,
		"reward_qty":   ev.RewardQty,
		"reward_coins": ev.RewardCoins,
		"observation":  obs,
	}, witnesses...)
	outEvs := []Event{claimEv}
	if ev.RewardCoins > 0 {
		outEvs = append(outEvs, w.settlePartyRewardSplit(a, ev.RewardCoins, "world_event_claimed")...)
	}
	return outEvs
}

// perceivedActiveWorldEvents returns only active world events that actor `a` is currently
// co-located with OR has observed/heard about in their own Memories (epistemic isolation).
func (w *World) perceivedActiveWorldEvents(a Actor) []ActiveWorldEvent {
	if len(w.ActiveWorldEvents) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, m := range a.Memories {
		if m.Observation != nil && m.Observation.Kind == "world_event" {
			known[m.Observation.SubjectID] = true
		}
		if m.Statement != nil && m.Statement.Topic == "world_event" {
			known[m.Statement.SubjectID] = true
		}
	}
	var out []ActiveWorldEvent
	for _, ev := range w.ActiveWorldEvents {
		if w.Tick >= ev.ExpiresTick {
			continue
		}
		if ev.DistrictID == a.Location || known[ev.InstanceID] || known[ev.SpecID] {
			out = append(out, ev)
		}
	}
	return out
}

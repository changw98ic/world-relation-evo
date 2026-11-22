package sim

import (
	"fmt"
	"sort"
	"strings"
)

// ExternalPatch defines a conservation-safe, deterministic narrative beat injection applied
// between simulation stages (e.g. Volume 1 -> Volume 2) by an external director or LLM Skill.
type ExternalPatch struct {
	Label              string                   `json:"label,omitempty"`
	TriggerWorldEvents []PatchWorldEventTrigger `json:"trigger_world_events,omitempty"`
	SpawnActors        []Actor                  `json:"spawn_actors,omitempty"`
	AddSecrets         []ActorSecret            `json:"add_secrets,omitempty"`
	AddGrudges         []GrudgeRecord           `json:"add_grudges,omitempty"`
	SetDiplomacy       []FactionDiplomacy       `json:"set_diplomacy,omitempty"`
	AddAmbitions       []PatchActorAmbition     `json:"add_ambitions,omitempty"`
	AddGoldenFingers   []GoldenFingerSpec       `json:"add_golden_fingers,omitempty"`
	AddGFQuests        []GFQuestInstance        `json:"add_gf_quests,omitempty"`
}

// PatchWorldEventTrigger fires either an already-declared Catalog.WorldEvent by SpecID
// or registers an inline WorldEventSpec and triggers it immediately at DistrictID.
type PatchWorldEventTrigger struct {
	SpecID     string          `json:"spec_id,omitempty"`
	InlineSpec *WorldEventSpec `json:"inline_spec,omitempty"`
	DistrictID string          `json:"district_id,omitempty"`
}

// PatchActorAmbition adds a new long-term ambition to an existing actor at a story turning point.
type PatchActorAmbition struct {
	ActorID     string `json:"actor_id"`
	Kind        string `json:"kind"`
	TargetID    string `json:"target_id,omitempty"`
	TargetValue int    `json:"target_value,omitempty"`
}

// ApplyExternalPatch applies an ExternalPatch to the world while strictly preserving ledger
// conservation (recording all newly introduced actor coins/items as explicit external inflow)
// and emitting formal Events with monotonic IDs for downstream chronicle reconstruction.
func (w *World) ApplyExternalPatch(patch ExternalPatch) ([]Event, error) {
	if w == nil {
		return nil, fmt.Errorf("world is nil")
	}
	var events []Event

	// 1. Spawn newly entering named actors (with conservation-tracked external inflow).
	for _, raw := range patch.SpawnActors {
		a := CloneActor(raw)
		if strings.TrimSpace(a.ID) == "" {
			return nil, fmt.Errorf("spawn_actor missing id")
		}
		if w.actor(a.ID) != nil {
			return nil, fmt.Errorf("spawn_actor %q already exists in world", a.ID)
		}
		if a.Location == "" && len(w.Map.Districts) > 0 {
			a.Location = w.Map.Districts[0].ID
		}
		if _, ok := w.Map.DistrictByID(a.Location); !ok {
			return nil, fmt.Errorf("spawn_actor %q references unknown district %q", a.ID, a.Location)
		}
		if a.Name == "" {
			a.Name = a.ID
		}
		if a.Health <= 0 {
			a.Health = MaxHealth
		}
		if a.Energy <= 0 {
			a.Energy = MaxEnergy
		}
		if !ActorHasTag(a.Tags, "resident") {
			a.Tags = append([]string{"resident"}, a.Tags...)
		}
		if a.Items == nil {
			a.Items = map[string]int{}
		}
		if a.Trust == nil {
			a.Trust = map[string]int{}
		}
		if a.Money < 0 {
			return nil, fmt.Errorf("spawn_actor %q has negative money %d", a.ID, a.Money)
		}
		// Account for external wealth & inventory inflow in LedgerAudit so conservation holds.
		if a.Money > 0 {
			w.recordLedgerCurrencyProduced("coins", int64(a.Money))
		}
		curKeys := make([]string, 0, len(a.Currencies))
		for k := range a.Currencies {
			curKeys = append(curKeys, k)
		}
		sort.Strings(curKeys)
		for _, k := range curKeys {
			if a.Currencies[k] < 0 {
				return nil, fmt.Errorf("spawn_actor %q has negative currency %s=%d", a.ID, k, a.Currencies[k])
			}
			if a.Currencies[k] > 0 {
				w.recordLedgerCurrencyProduced(k, a.Currencies[k])
			}
		}
		itemKeys := make([]string, 0, len(a.Items))
		for k := range a.Items {
			itemKeys = append(itemKeys, k)
		}
		sort.Strings(itemKeys)
		for _, k := range itemKeys {
			qty := a.Items[k]
			if qty < 0 {
				return nil, fmt.Errorf("spawn_actor %q has negative item %s=%d", a.ID, k, qty)
			}
			if qty > 0 {
				if _, ok := w.Catalog.Items[k]; !ok {
					w.Catalog.Items[k] = ItemSpec{ID: k, Name: k, Tier: 2, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 5}
				}
				w.recordLedgerItemProduced(k, int64(qty))
			}
		}
		// Sync faction membership.
		for _, gID := range a.Guilds {
			if f := w.faction(gID); f != nil && !f.HasMember(a.ID) {
				f.Members = append(f.Members, a.ID)
			}
		}
		w.Actors = append(w.Actors, a)
		ev := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: a.ID,
			Kind:    "actor_entered_world",
			Action:  Action{Kind: "wait", Location: a.Location},
			Payload: map[string]any{
				"text":     fmt.Sprintf("%s (%s) entered the stage at %s", a.Name, a.Role, a.Location),
				"actor_id": a.ID,
				"name":     a.Name,
				"role":     a.Role,
				"district": a.Location,
				"label":    patch.Label,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 2. Trigger declared or inline world events.
	for _, trig := range patch.TriggerWorldEvents {
		specID := trig.SpecID
		if trig.InlineSpec != nil {
			spec := cloneWorldEventSpec(*trig.InlineSpec)
			if spec.ID == "" {
				return nil, fmt.Errorf("inline world event spec missing id")
			}
			specID = spec.ID
			if spec.RewardItem != "" {
				if _, ok := w.Catalog.Items[spec.RewardItem]; !ok {
					w.Catalog.Items[spec.RewardItem] = ItemSpec{ID: spec.RewardItem, Name: spec.RewardItem, Tier: 4, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 40}
				}
			}
			replaced := false
			for i := range w.Catalog.WorldEvents {
				if w.Catalog.WorldEvents[i].ID == spec.ID {
					w.Catalog.WorldEvents[i] = spec
					replaced = true
					break
				}
			}
			if !replaced {
				w.Catalog.WorldEvents = append(w.Catalog.WorldEvents, spec)
			}
		}
		if specID == "" {
			return nil, fmt.Errorf("trigger_world_event requires spec_id or inline_spec")
		}
		dist := trig.DistrictID
		if dist == "" && len(w.Map.Districts) > 0 {
			dist = w.Map.Districts[0].ID
		}
		if _, ok := w.Map.DistrictByID(dist); !ok {
			return nil, fmt.Errorf("trigger_world_event %q references unknown district %q", specID, dist)
		}
		if w.DistrictStates == nil {
			w.DistrictStates = map[string]DistrictState{dist: {DistrictID: dist, Weather: "clear", Sanitation: 80}}
		}
		triggered := w.TriggerWorldEvent(specID, dist)
		if len(triggered) == 0 {
			return nil, fmt.Errorf("trigger_world_event %q did not match any WorldEventSpec", specID)
		}
		events = append(events, triggered...)
	}

	// 3. Inject secrets.
	for _, s := range patch.AddSecrets {
		if w.actor(s.OwnerID) == nil {
			return nil, fmt.Errorf("add_secret references unknown owner_id %q", s.OwnerID)
		}
		sev := s.Severity
		if sev <= 0 {
			sev = 60
		}
		_, holdEvs := w.addSecretWithWitnesses(s.OwnerID, s.Kind, s.SubjectID, sev, 0, s.KnownBy, s.WitnessedBy)
		events = append(events, holdEvs...)
	}

	// 4. Inject grudges.
	if len(patch.AddGrudges) > 0 {
		rs := w.ensureRelations()
		for _, g := range patch.AddGrudges {
			if w.actor(g.AggrievedID) == nil || w.actor(g.OffenderID) == nil {
				return nil, fmt.Errorf("add_grudge references unknown actor (%q -> %q)", g.AggrievedID, g.OffenderID)
			}
			inten := g.Intensity
			if inten <= 0 {
				inten = 60
			}
			cause := g.Cause
			if cause == "" {
				cause = "narrative_conflict"
			}
			events = append(events, rs.recordGrudge(nil, w, g.AggrievedID, g.OffenderID, cause, inten, 0, 0)...)
		}
	}

	// 5. Update bilateral faction diplomacy.
	for _, d := range patch.SetDiplomacy {
		if w.faction(d.FactionA) == nil || w.faction(d.FactionB) == nil {
			return nil, fmt.Errorf("set_diplomacy references unknown faction (%q <-> %q)", d.FactionA, d.FactionB)
		}
		rel := w.ensureFactionDiplomacy(d.FactionA, d.FactionB)
		if rel == nil {
			continue
		}
		if d.Stance != "" {
			rel.Stance = d.Stance
		}
		if d.RelationScore != 0 || d.Stance == "neutral" {
			rel.RelationScore = d.RelationScore
		}
		if d.Reason != "" {
			rel.Reason = d.Reason
		}
		evKind := "diplomacy_shifted"
		if rel.Stance == "at_war" {
			evKind = "faction_war_declared"
			rel.WarStartedTick = w.Tick
		} else if rel.Stance == "allied" {
			evKind = "alliance_formed"
		}
		ev := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: "world",
			Kind:    evKind,
			Action:  Action{Kind: "wait"},
			Payload: map[string]any{
				"text":           fmt.Sprintf("Diplomatic shift: %s <-> %s is now %s (%s)", rel.FactionA, rel.FactionB, rel.Stance, rel.Reason),
				"faction_a":      rel.FactionA,
				"faction_b":      rel.FactionB,
				"stance":         rel.Stance,
				"relation_score": rel.RelationScore,
				"reason":         rel.Reason,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 6. Add new personal ambitions to existing actors.
	for _, amb := range patch.AddAmbitions {
		a := w.actor(amb.ActorID)
		if a == nil || a.Dead {
			return nil, fmt.Errorf("add_ambition references missing or dead actor %q", amb.ActorID)
		}
		kind := normalizeAmbitionKind(amb.Kind)
		newAmb := ActorAmbition{
			ID:          fmt.Sprintf("amb_patch_%d_%s_%s", w.Tick, a.ID, kind),
			Kind:        kind,
			TargetID:    amb.TargetID,
			TargetValue: amb.TargetValue,
			StartedTick: w.Tick,
		}
		a.Ambitions = append(a.Ambitions, newAmb)
		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  a.ID,
			TargetID: amb.TargetID,
			Kind:     "ambition_adopted",
			Action:   Action{Kind: "set_ambition", Item: kind, TargetID: amb.TargetID},
			Payload: map[string]any{
				"text":        fmt.Sprintf("%s adopted ambition %s (target=%s)", a.Name, kind, amb.TargetID),
				"ambition_id": newAmb.ID,
				"kind":        kind,
				"target_id":   amb.TargetID,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 7. Register newly awakened golden fingers and dynamic system quests.
	for _, gf := range patch.AddGoldenFingers {
		if strings.TrimSpace(gf.ID) == "" {
			return nil, fmt.Errorf("add_golden_finger missing id")
		}
		replaced := false
		for i := range w.Catalog.GoldenFingers {
			if w.Catalog.GoldenFingers[i].ID == gf.ID {
				w.Catalog.GoldenFingers[i] = CloneGoldenFingerSpec(gf)
				replaced = true
				break
			}
		}
		if !replaced {
			w.Catalog.GoldenFingers = append(w.Catalog.GoldenFingers, CloneGoldenFingerSpec(gf))
		}
		actorID := gf.OwnerActorID
		if actorID == "" {
			actorID = "world"
		}
		ev := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: actorID,
			Kind:    "gf_triggered",
			Action:  Action{Kind: "wait"},
			Payload: map[string]any{
				"text":          fmt.Sprintf("Golden finger [%s] awakened for %s", gf.Name, actorID),
				"golden_finger": gf.ID,
			},
		}
		w.NextID++
		events = append(events, ev)
	}
	if len(patch.AddGFQuests) > 0 {
		st := w.ensureGoldenFingerState()
		for _, qi := range patch.AddGFQuests {
			if qi.GoldenFingerID == "" || qi.Quest.ID == "" {
				continue
			}
			st.DynamicQuests = append(st.DynamicQuests, qi)
		}
	}

	if err := w.VerifyLedgerConservation(); err != nil {
		return nil, fmt.Errorf("ledger conservation failed after patch: %w", err)
	}
	return events, nil
}

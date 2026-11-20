package sim

import (
	"fmt"
	"sort"
)

// ActorAmbition represents a long-term personal aspiration spanning days/months.
type ActorAmbition struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"` // "ascend_tier" | "avenge_blood" | "accumulate_wealth" | "expose_corrupt" | "seize_leadership" | "master_skill"
	TargetID      string `json:"target_id,omitempty"`
	TargetValue   int    `json:"target_value,omitempty"`
	Progress      int    `json:"progress,omitempty"`
	StartedTick   int64  `json:"started_tick"`
	CompletedTick int64  `json:"completed_tick,omitempty"`
	Completed     bool   `json:"completed,omitempty"`
}

// ActorSecret represents an epistemically isolated secret owned by an actor.
type ActorSecret struct {
	ID          string   `json:"id"`
	OwnerID     string   `json:"owner_id"`
	Kind        string   `json:"kind"` // "illicit_gain" | "secret_vendetta" | "covert_bribe" | "forbidden_hoard" | "hidden_plot"
	SubjectID   string   `json:"subject_id,omitempty"`
	Severity    int      `json:"severity"`
	CreatedTick int64    `json:"created_tick"`
	RootEventID int64    `json:"root_event_id,omitempty"`
	KnownBy     []string `json:"known_by,omitempty"`
	Exposed     bool     `json:"exposed,omitempty"`
	ExposedTick int64    `json:"exposed_tick,omitempty"`
}

func cloneSecret(s ActorSecret) ActorSecret {
	out := s
	if s.KnownBy != nil {
		out.KnownBy = append([]string(nil), s.KnownBy...)
	}
	return out
}

func actorKnowsSecret(s ActorSecret, actorID string) bool {
	if actorID == "" {
		return false
	}
	return s.OwnerID == actorID || s.Exposed || ActorHasTag(s.KnownBy, actorID)
}

// perceivedSecrets enforces strict epistemic isolation: an actor only sees secrets they own,
// personally discovered/witnessed, or that were publicly exposed.
func (w *World) perceivedSecrets(a Actor) []ActorSecret {
	if w == nil || len(w.Secrets) == 0 {
		return nil
	}
	var out []ActorSecret
	for _, s := range w.Secrets {
		if actorKnowsSecret(s, a.ID) {
			out = append(out, cloneSecret(s))
		}
	}
	return out
}

func normalizeAmbitionKind(raw string) string {
	switch raw {
	case "ascend_tier", "avenge_blood", "accumulate_wealth", "expose_corrupt", "seize_leadership", "master_skill":
		return raw
	default:
		return "accumulate_wealth"
	}
}

func (w *World) applySetAmbition(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	kind := normalizeAmbitionKind(x.Item)
	targetID := x.TargetID
	activeCount := 0
	for _, amb := range a.Ambitions {
		if amb.Completed {
			continue
		}
		activeCount++
		if amb.Kind == kind && amb.TargetID == targetID {
			return fail("identical active ambition already adopted")
		}
	}
	if activeCount >= 2 {
		return fail("already holds maximum of 2 active ambitions")
	}

	targetVal := x.Quantity
	switch kind {
	case "accumulate_wealth":
		if targetVal <= a.Money {
			targetVal = maxInt(a.Money+40, 80)
		}
	case "ascend_tier":
		cur := w.Catalog.ActorPowerTier(*a)
		if targetVal <= cur {
			targetVal = cur + 1
		}
	case "master_skill":
		cur := a.Skills[targetID]
		if targetVal <= cur {
			targetVal = cur + 2
		}
	default:
		if targetVal <= 0 {
			targetVal = 1
		}
	}

	amb := ActorAmbition{
		ID:          fmt.Sprintf("amb_%d_%s_%s", w.Tick, a.ID, kind),
		Kind:        kind,
		TargetID:    targetID,
		TargetValue: targetVal,
		StartedTick: w.Tick,
	}
	a.Ambitions = append(a.Ambitions, amb)
	ev := w.record(a, x, "ambition_adopted", map[string]any{
		"text":         fmt.Sprintf("%s adopted long-term ambition: %s (target=%s, goal=%d)", a.Name, amb.Kind, amb.TargetID, amb.TargetValue),
		"ambition_id":  amb.ID,
		"kind":         amb.Kind,
		"target_id":    amb.TargetID,
		"target_value": amb.TargetValue,
	})
	return []Event{ev}
}

func (w *World) applyInvestigateActor(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 5 {
		return fail("insufficient energy to investigate actor")
	}
	target := w.actor(x.TargetID)
	if target == nil || target.Dead || target.ID == a.ID || !SamePlace(*a, *target) {
		return fail("investigation target must be co-located and alive")
	}
	a.Energy -= 5

	secretIdx := -1
	for i := range w.Secrets {
		s := w.Secrets[i]
		if s.OwnerID == target.ID && !s.Exposed && !ActorHasTag(s.KnownBy, a.ID) {
			secretIdx = i
			break
		}
	}
	if secretIdx < 0 {
		return []Event{w.record(a, x, "investigation_inconclusive", map[string]any{
			"text":      fmt.Sprintf("%s investigated %s and uncovered no new secret", a.Name, target.Name),
			"target_id": target.ID,
		})}
	}

	stealth, _ := w.Catalog.StealthBonus(*target)
	invScore := a.Attributes.Insight*3 + w.Catalog.VigilanceBonus(*a)
	concealScore := target.Attributes.Dexterity*2 + stealth
	prob := 55 + (invScore-concealScore)*2
	if prob < 25 {
		prob = 25
	}
	if prob > 85 {
		prob = 85
	}
	roll := int(worldHash(w.ID, w.Tick, "inv:"+a.ID+":"+target.ID) % 100)
	if roll >= prob {
		return []Event{w.record(a, x, "investigation_inconclusive", map[string]any{
			"text":      fmt.Sprintf("%s investigated %s but found no conclusive proof", a.Name, target.Name),
			"target_id": target.ID,
		})}
	}

	s := &w.Secrets[secretIdx]
	s.KnownBy = appendUnique(s.KnownBy, a.ID)
	obs := Observation{
		Kind:      "secret_discovered",
		Modality:  "eyewitness",
		SubjectID: target.ID,
		ObjectID:  s.ID,
		Item:      s.Kind,
		Location:  a.Location,
		Quantity:  s.Severity,
	}
	ev := w.record(a, x, "secret_discovered", map[string]any{
		"text":        fmt.Sprintf("%s uncovered %s's secret (%s, severity %d)", a.Name, target.Name, s.Kind, s.Severity),
		"secret_id":   s.ID,
		"owner_id":    s.OwnerID,
		"secret_kind": s.Kind,
		"severity":    s.Severity,
		"observation": obs,
	})
	return []Event{ev}
}

func (w *World) applyBlackmail(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 4 {
		return fail("insufficient energy to blackmail")
	}
	target := w.actor(x.TargetID)
	if target == nil || target.Dead || target.ID == a.ID || !SamePlace(*a, *target) {
		return fail("blackmail target must be co-located and alive")
	}

	secretIdx := -1
	for i := range w.Secrets {
		s := w.Secrets[i]
		if s.Exposed || s.OwnerID != target.ID {
			continue
		}
		if !ActorHasTag(s.KnownBy, a.ID) {
			continue
		}
		if x.Item == "" || s.ID == x.Item || s.Kind == x.Item {
			secretIdx = i
			break
		}
	}
	if secretIdx < 0 {
		return fail("no known unexposed secret to blackmail target")
	}
	s := w.Secrets[secretIdx]
	a.Energy -= 4

	demand := maxInt(2, minInt(20, s.Severity/4))
	paid := minInt(target.Money, demand)
	if paid > 0 {
		// Exact 0-error coin transfer between existing living actors.
		target.Money -= paid
		a.Money += paid
		a.IllicitGain += paid
	}
	if target.Trust == nil {
		target.Trust = map[string]int{}
	}
	target.Trust[a.ID] -= 4

	// Blackmailing creates a reciprocal covert secret on the blackmailer known to the victim!
	w.addSecret(a.ID, "hidden_plot", target.ID, minInt(80, 30+paid*2), w.Tick, []string{a.ID, target.ID})

	observers := w.awakeSamePlaceObservers(*a, a.ID)
	obs := Observation{
		Kind:      "blackmailed",
		Modality:  "eyewitness",
		SubjectID: a.ID,
		ObjectID:  target.ID,
		Item:      s.Kind,
		Location:  a.Location,
		Quantity:  paid,
	}
	ev := w.record(a, x, "blackmailed", map[string]any{
		"text":        fmt.Sprintf("%s blackmailed %s over %s and extorted %d coins", a.Name, target.Name, s.Kind, paid),
		"secret_id":   s.ID,
		"victim_id":   target.ID,
		"paid_coins":  paid,
		"severity":    s.Severity,
		"observation": obs,
	}, observers...)
	events := []Event{ev}
	events = append(events, w.ensureRelations().recordGrudge(w, target.ID, a.ID, "blackmailed", 40, ev.ID, ev.ID)...)
	return events
}

func (w *World) applyExposeSecret(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 4 {
		return fail("insufficient energy to expose secret")
	}
	secretIdx := -1
	for i := range w.Secrets {
		s := w.Secrets[i]
		if s.Exposed {
			continue
		}
		if !ActorHasTag(s.KnownBy, a.ID) && s.OwnerID != a.ID {
			continue
		}
		if (x.Item != "" && (s.ID == x.Item || s.Kind == x.Item)) ||
			(x.Item == "" && x.TargetID != "" && s.OwnerID == x.TargetID) {
			secretIdx = i
			break
		}
	}
	if secretIdx < 0 {
		return fail("no known unexposed secret to expose")
	}
	s := &w.Secrets[secretIdx]
	a.Energy -= 4
	s.Exposed = true
	s.ExposedTick = w.Tick

	owner := w.actor(s.OwnerID)
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	for _, ob := range observers {
		if ob.ID == s.OwnerID {
			continue
		}
		if ob.Trust == nil {
			ob.Trust = map[string]int{}
		}
		ob.Trust[s.OwnerID] -= 3
	}
	if owner != nil && !owner.Dead {
		if s.Severity >= 50 && owner.IllicitGain == 0 {
			owner.IllicitGain += 5
		}
		if owner.ID != a.ID {
			if owner.Trust == nil {
				owner.Trust = map[string]int{}
			}
			owner.Trust[a.ID] -= 5
		}
	}
	obs := Observation{
		Kind:      "secret_exposed",
		Modality:  "eyewitness",
		SubjectID: s.OwnerID,
		ObjectID:  s.ID,
		Item:      s.Kind,
		Location:  a.Location,
		Quantity:  s.Severity,
	}
	ownerName := s.OwnerID
	if owner != nil {
		ownerName = owner.Name
	}
	ev := w.record(a, x, "secret_exposed", map[string]any{
		"text":        fmt.Sprintf("%s publicly exposed %s's secret (%s, severity %d) in %s", a.Name, ownerName, s.Kind, s.Severity, a.Location),
		"secret_id":   s.ID,
		"owner_id":    s.OwnerID,
		"secret_kind": s.Kind,
		"severity":    s.Severity,
		"witnesses":   len(observers),
		"observation": obs,
	}, observers...)
	events := []Event{ev}
	if owner != nil && !owner.Dead && owner.ID != a.ID {
		events = append(events, w.ensureRelations().recordGrudge(w, owner.ID, a.ID, "secret_exposed", 45, ev.ID, ev.ID)...)
	}
	return events
}

func (w *World) addSecret(ownerID, kind, subjectID string, severity int, rootEventID int64, knownBy []string) {
	if ownerID == "" || kind == "" {
		return
	}
	for i := range w.Secrets {
		s := &w.Secrets[i]
		if !s.Exposed && s.OwnerID == ownerID && s.Kind == kind && s.SubjectID == subjectID {
			for _, kb := range knownBy {
				s.KnownBy = appendUnique(s.KnownBy, kb)
			}
			if severity > s.Severity {
				s.Severity = severity
			}
			return
		}
	}
	if len(w.Secrets) >= 120 {
		return
	}
	var kb []string
	kb = appendUnique(kb, ownerID)
	for _, id := range knownBy {
		if id != "" {
			kb = appendUnique(kb, id)
		}
	}
	sort.Strings(kb)
	w.Secrets = append(w.Secrets, ActorSecret{
		ID:          fmt.Sprintf("sec_%d_%s_%d", w.Tick, ownerID, len(w.Secrets)+1),
		OwnerID:     ownerID,
		Kind:        kind,
		SubjectID:   subjectID,
		Severity:    minInt(100, maxInt(10, severity)),
		CreatedTick: w.Tick,
		RootEventID: rootEventID,
		KnownBy:     kb,
	})
}

func (w *World) stepAmbitionsAndSecrets(settled []Event) []Event {
	if w == nil {
		return nil
	}
	// 1. Record covert secrets strictly to involved actor + actual witnesses.
	for _, ev := range settled {
		switch ev.Kind {
		case "bribed":
			w.addSecret(ev.ActorID, "covert_bribe", ev.TargetID, 60, ev.ID, []string{ev.ActorID, ev.TargetID})
			if ev.TargetID != "" {
				w.addSecret(ev.TargetID, "illicit_gain", ev.ActorID, 70, ev.ID, []string{ev.TargetID, ev.ActorID})
			}
		case "robbed", "suspect_framed":
			w.addSecret(ev.ActorID, "illicit_gain", ev.TargetID, 65, ev.ID, []string{ev.ActorID, ev.TargetID})
		case "grudge_formed":
			if inten := int(payloadInt64(ev.Payload, "intensity")); inten >= 50 {
				w.addSecret(ev.ActorID, "secret_vendetta", ev.TargetID, inten, ev.ID, []string{ev.ActorID})
			}
		}
	}

	// 2. Daily 09:00 ambition seeding and progress evaluation.
	clock := ClockAt(w.Tick)
	if clock.Hour != 9 || clock.Minute != 0 {
		return nil
	}
	var events []Event
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead {
			continue
		}
		activeCount := 0
		for ai := range a.Ambitions {
			amb := &a.Ambitions[ai]
			if amb.Completed {
				continue
			}
			activeCount++
			done := false
			switch amb.Kind {
			case "accumulate_wealth":
				amb.Progress = a.Money
				done = a.Money >= amb.TargetValue
			case "ascend_tier":
				amb.Progress = w.Catalog.ActorPowerTier(*a)
				done = amb.Progress >= amb.TargetValue
			case "master_skill":
				amb.Progress = a.Skills[amb.TargetID]
				done = amb.Progress >= amb.TargetValue
			case "seize_leadership":
				if w.Relations != nil && amb.TargetID != "" && w.Relations.FactionLeader[amb.TargetID] == a.ID {
					amb.Progress = 1
					done = true
				}
			case "avenge_blood":
				if tgt := w.actor(amb.TargetID); tgt != nil && tgt.Dead {
					amb.Progress = 1
					done = true
				}
			case "expose_corrupt":
				for _, s := range w.Secrets {
					if s.Exposed && (s.OwnerID == amb.TargetID || s.ID == amb.TargetID) {
						amb.Progress = 1
						done = true
						break
					}
				}
			}
			if done {
				amb.Completed = true
				amb.CompletedTick = w.Tick
				activeCount--
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: a.ID,
					Kind:    "ambition_completed",
					Action:  Action{Kind: "set_ambition", Item: amb.Kind, TargetID: amb.TargetID},
					Payload: map[string]any{
						"text":         fmt.Sprintf("%s fulfilled long-term ambition: %s", a.Name, amb.Kind),
						"ambition_id":  amb.ID,
						"kind":         amb.Kind,
						"target_id":    amb.TargetID,
						"target_value": amb.TargetValue,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}

		if activeCount == 0 && len(a.Ambitions) < 4 {
			// Auto-seed 1 natural long-term ambition.
			seeded := false
			if w.Relations != nil {
				for _, g := range w.Relations.Grudges {
					if !g.Resolved && g.AggrievedID == a.ID && g.OffenderID != "" && g.Intensity >= 40 {
						a.Ambitions = append(a.Ambitions, ActorAmbition{
							ID:          fmt.Sprintf("amb_%d_%s_avenge", w.Tick, a.ID),
							Kind:        "avenge_blood",
							TargetID:    g.OffenderID,
							TargetValue: 1,
							StartedTick: w.Tick,
						})
						seeded = true
						break
					}
				}
			}
			if !seeded && len(w.Catalog.Transitions) > 0 {
				curTier := w.Catalog.ActorPowerTier(*a)
				hasHigher := false
				for _, f := range w.Catalog.Forms {
					if f.PowerTier > curTier {
						hasHigher = true
						break
					}
				}
				if hasHigher {
					a.Ambitions = append(a.Ambitions, ActorAmbition{
						ID:          fmt.Sprintf("amb_%d_%s_ascend", w.Tick, a.ID),
						Kind:        "ascend_tier",
						TargetValue: curTier + 1,
						Progress:    curTier,
						StartedTick: w.Tick,
					})
					seeded = true
				}
			}
			if !seeded {
				a.Ambitions = append(a.Ambitions, ActorAmbition{
					ID:          fmt.Sprintf("amb_%d_%s_wealth", w.Tick, a.ID),
					Kind:        "accumulate_wealth",
					TargetValue: maxInt(a.Money+40, 80),
					Progress:    a.Money,
					StartedTick: w.Tick,
				})
			}
		}
	}
	return events
}

package sim

import (
	"fmt"
	"sort"
	"strings"
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
	ID              string            `json:"id"`
	OwnerID         string            `json:"owner_id"`
	Kind            string            `json:"kind"` // "illicit_gain" | "secret_vendetta" | "covert_bribe" | "forbidden_hoard" | "hidden_plot" | "covert_complicity"
	SubjectID       string            `json:"subject_id,omitempty"`
	Severity        int               `json:"severity"`
	CreatedTick     int64             `json:"created_tick"`
	IllicitProceeds int               `json:"illicit_proceeds,omitempty"`
	RootEventID     int64             `json:"root_event_id,omitempty"`
	KnownBy         []string          `json:"known_by,omitempty"`
	WitnessedBy     []string          `json:"witnessed_by,omitempty"`   // eyewitnesses who caught the owner red-handed on site
	HolderStances   map[string]string `json:"holder_stances,omitempty"` // knowerID -> "shield" | "bide_time" | "fear_silence" | "leverage" | "confront"
	ExposureKnownBy []string          `json:"exposure_known_by,omitempty"`
	Exposed         bool              `json:"exposed,omitempty"`
	ExposedTick     int64             `json:"exposed_tick,omitempty"`
}

func personalityContainsAny(personality string, keywords ...string) bool {
	if personality == "" {
		return false
	}
	lower := strings.ToLower(personality)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func (w *World) formatSecretDesc(s ActorSecret) string {
	if s.SubjectID == "" {
		return s.Kind
	}
	subj := s.SubjectID
	if a := w.actor(s.SubjectID); a != nil && a.Name != "" {
		subj = a.Name
	}
	return fmt.Sprintf("%s involving %s", s.Kind, subj)
}

func CloneActorSecret(s ActorSecret) ActorSecret {
	return cloneSecret(s)
}

func cloneSecret(s ActorSecret) ActorSecret {
	out := s
	out.ExposureKnownBy = append([]string(nil), s.ExposureKnownBy...)
	if s.KnownBy != nil {
		out.KnownBy = append([]string(nil), s.KnownBy...)
	}
	if s.WitnessedBy != nil {
		out.WitnessedBy = append([]string(nil), s.WitnessedBy...)
	}
	if s.HolderStances != nil {
		out.HolderStances = cloneStringMap(s.HolderStances)
	}
	return out
}

// ComputeSecretHoldStance deterministically evaluates how a non-owner knower chooses to handle
// an unexposed secret: shielding kin/allies, biding time against a stronger foe, staying silent
// out of fear, holding quiet leverage, or confronting/exposing righteously.
func (w *World) ComputeSecretHoldStance(holder, owner Actor, s ActorSecret) string {
	if holder.ID == "" || holder.ID == owner.ID || s.Exposed {
		return ""
	}
	if s.Kind == "covert_bribe" && s.SubjectID == holder.ID && holder.IllicitGain > 0 {
		return "paid_silence"
	}
	// A briber won't publicly expose the very recipient they just bribed (self-incrimination).
	if s.Kind == "illicit_gain" && s.SubjectID == holder.ID {
		for _, other := range w.Secrets {
			if other.OwnerID == holder.ID && other.Kind == "covert_bribe" && other.SubjectID == owner.ID && !other.Exposed {
				return "paid_silence"
			}
		}
	}

	hTier := w.Catalog.ActorPowerTier(holder)
	oTier := w.Catalog.ActorPowerTier(owner)
	hPower := CombatPowerWithCatalog(w.Catalog, holder)
	oPower := CombatPowerWithCatalog(w.Catalog, owner)

	isRighteous := personalityContainsAny(holder.Personality, "righteous", "dutiful", "stern", "嫉恶", "恪职", "冷峻")
	isEnforcerOrOversight := w.Catalog.IsOversightTags(holder.IdentityTags()) || w.Catalog.IsEnforcementTags(holder.IdentityTags())
	isTimidOrLowProfile := personalityContainsAny(holder.Personality, "timid", "cautious", "low_profile", "indifferent", "aloof", "胆怯", "警觉", "韬光", "淡漠")
	isPatientOrScheming := personalityContainsAny(holder.Personality, "patient", "cunning", "calculating", "paranoid", "ambitious", "隐忍", "狡黠", "精明", "多疑", "野心")
	isOpportunist := personalityContainsAny(holder.Personality, "opportunistic", "calculating", "cunning", "见风使舵", "精明", "狡黠") || w.Catalog.IsOutlawTags(holder.IdentityTags())

	bonded := holder.Trust[owner.ID] >= 4
	if !bonded && w.Relations != nil {
		if ActorHasTag(w.Relations.mentorshipPartners(owner.ID), holder.ID) {
			bonded = true
		} else if hk := w.Relations.Kinship[holder.ID]; hk != nil {
			if hk.SpouseID == owner.ID || hk.PatronID == owner.ID ||
				ActorHasTag(hk.ParentIDs, owner.ID) || ActorHasTag(hk.ChildIDs, owner.ID) ||
				ActorHasTag(hk.SwornIDs, owner.ID) || ActorHasTag(hk.ProtegeIDs, owner.ID) {
				bonded = true
			}
		}
	}
	if bonded && !(isRighteous && isEnforcerOrOversight && s.Severity >= 75) {
		return "shield"
	}

	if isRighteous || isEnforcerOrOversight {
		return "confront"
	}

	if isTimidOrLowProfile && (oTier >= hTier+1 || oPower > hPower+12) {
		return "fear_silence"
	}

	if isPatientOrScheming && (oTier > hTier || oPower > hPower || owner.Health >= 75) {
		if isOpportunist && oTier <= hTier && owner.Health >= 65 {
			return "leverage"
		}
		return "bide_time"
	}

	if isOpportunist {
		return "leverage"
	}
	if holder.Trust[owner.ID] >= 1 {
		return "shield"
	}
	return "bide_time"
}

func (w *World) refreshSecretHolderStances(s *ActorSecret) []Event {
	if w == nil || s == nil || s.Exposed {
		return nil
	}
	owner := w.actor(s.OwnerID)
	if owner == nil {
		return nil
	}
	if s.HolderStances == nil {
		s.HolderStances = map[string]string{}
	}
	knownSorted := append([]string(nil), s.KnownBy...)
	sort.Strings(knownSorted)
	var events []Event
	for _, kid := range knownSorted {
		if kid == "" || kid == s.OwnerID {
			continue
		}
		holder := w.actor(kid)
		if holder == nil || holder.Dead {
			continue
		}
		prev := s.HolderStances[kid]
		if prev != "" {
			continue
		}
		next := w.ComputeSecretHoldStance(*holder, *owner, *s)
		if next == "" {
			continue
		}
		s.HolderStances[kid] = next
		if prev == "" && (next == "shield" || next == "bide_time" || next == "fear_silence") {
			ev := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  holder.ID,
				TargetID: owner.ID,
				Kind:     "secret_held",
				Action:   Action{Kind: "wait", TargetID: owner.ID, Item: s.ID},
				Payload: map[string]any{
					"text":        fmt.Sprintf("%s chose to hold %s's secret (%s) in silence (%s)", holder.Name, owner.Name, w.formatSecretDesc(*s), next),
					"secret_id":   s.ID,
					"owner_id":    owner.ID,
					"holder_id":   holder.ID,
					"hold_stance": next,
					"severity":    s.Severity,
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}
	return events
}

func actorKnowsSecret(s ActorSecret, actorID string) bool {
	if actorID == "" {
		return false
	}
	return s.OwnerID == actorID || ActorHasTag(s.KnownBy, actorID)
}

// perceivedSecrets enforces strict epistemic isolation: an actor only sees secrets they own,
// personally discovered, witnessed, or received through a verified report.
func (w *World) perceivedSecrets(a Actor) []ActorSecret {
	if w == nil || len(w.Secrets) == 0 {
		return nil
	}
	var out []ActorSecret
	for _, s := range w.Secrets {
		if actorKnowsSecret(s, a.ID) {
			visible := s
			visible.KnownBy = []string{a.ID}
			visible.WitnessedBy = nil
			if a.ID == s.OwnerID {
				visible.WitnessedBy = append([]string(nil), s.WitnessedBy...)
			} else if ActorHasTag(s.WitnessedBy, a.ID) {
				visible.WitnessedBy = []string{a.ID}
			}
			visible.HolderStances = nil
			if stance := s.HolderStances[a.ID]; stance != "" {
				visible.HolderStances = map[string]string{a.ID: stance}
			}
			visible.ExposureKnownBy = nil
			visible.Exposed = s.Exposed && ActorHasTag(s.ExposureKnownBy, a.ID)
			if !visible.Exposed {
				visible.ExposedTick = 0
			}
			out = append(out, visible)
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

func (w *World) applySetAmbition(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	kind := normalizeAmbitionKind(x.Item)
	targetID := x.TargetID
	activeCount := 0
	for _, amb := range a.Ambitions {
		if amb.Completed {
			if amb.Kind == kind && amb.TargetID == targetID && (kind == "master_skill" || kind == "expose_corrupt" || kind == "avenge_blood") {
				return fail("identical ambition already completed")
			}
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
	ev := tx.record(a, x, "ambition_adopted", map[string]any{
		"text":         fmt.Sprintf("%s adopted long-term ambition: %s (target=%s, goal=%d)", a.Name, amb.Kind, amb.TargetID, amb.TargetValue),
		"ambition_id":  amb.ID,
		"kind":         amb.Kind,
		"target_id":    amb.TargetID,
		"target_value": amb.TargetValue,
	})
	return []Event{ev}
}

func (w *World) applyInvestigateActor(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 5 {
		return fail("insufficient energy to investigate actor")
	}
	target := w.actor(x.TargetID)
	if target == nil || target.Dead || target.ID == a.ID || !SamePlace(*a, *target) {
		return fail("investigation target must be co-located and alive")
	}
	tx.setEnergy(a, a.Energy-5, x.Kind)

	secretIdx := -1
	for i := range w.Secrets {
		s := w.Secrets[i]
		if s.OwnerID == target.ID && !s.Exposed && !ActorHasTag(s.KnownBy, a.ID) {
			secretIdx = i
			break
		}
	}
	if secretIdx < 0 {
		return []Event{tx.record(a, x, "investigation_inconclusive", map[string]any{
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
		return []Event{tx.record(a, x, "investigation_inconclusive", map[string]any{
			"text":      fmt.Sprintf("%s investigated %s but found no conclusive proof", a.Name, target.Name),
			"target_id": target.ID,
		})}
	}

	s := &w.Secrets[secretIdx]
	s.KnownBy = appendUnique(s.KnownBy, a.ID)
	holdEvs := w.refreshSecretHolderStances(s)
	obs := Observation{
		Kind:      "secret_discovered",
		Modality:  "eyewitness",
		SubjectID: target.ID,
		ObjectID:  s.ID,
		Item:      s.Kind,
		Location:  a.Location,
		Quantity:  s.Severity,
		Proceeds:  s.IllicitProceeds,
	}
	ev := tx.record(a, x, "secret_discovered", map[string]any{
		"text":          fmt.Sprintf("%s uncovered %s's secret (%s, severity %d)", a.Name, target.Name, w.formatSecretDesc(*s), s.Severity),
		"secret_id":     s.ID,
		"owner_id":      s.OwnerID,
		"subject_id":    s.SubjectID,
		"secret_kind":   s.Kind,
		"severity":      s.Severity,
		"hold_stance":   s.HolderStances[a.ID],
		"observation":   obs,
		"root_event_id": s.RootEventID,
	})
	return append([]Event{ev}, holdEvs...)
}

func (w *World) applyBlackmail(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
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
	s := &w.Secrets[secretIdx]
	tx.setEnergy(a, a.Energy-4, x.Kind)

	if target.Trust == nil {
		target.Trust = map[string]int{}
	}
	// Continuous Offense vs Breaking-Threshold model:
	// Demand size (relative to target wealth), attitude ("subtle" | "firm" | "hostile"), and
	// exponential escalation on repeated triggers determine whether the victim treats it as a
	// minor favor (no blood grudge), complies resentfully, or snaps past their breaking threshold!
	assess := w.BlackmailAssessment(*target, *a, *s, x, true)
	demand, attitude := assess.Demand, assess.Attitude

	if assess.Outcome == "breaking_point" || target.Money < 1 {
		reason := "offense_breaking_point"
		if target.Money < 1 {
			reason = "insolvent"
		}
		tx.MutateTrust(target, a.ID, -5)
		var backlashDmg, absorbed int
		var immune bool
		var effects []Event
		if assess.Outcome == "breaking_point" {
			tPower := CombatPowerWithCatalog(w.Catalog, *target)
			backlashDmg, absorbed, immune, effects = w.applyMeleeHit(tx, target, a, x, maxInt(4, minInt(18, tPower/3)))
		}
		observers := w.awakeSamePlaceObservers(*a, a.ID)
		ev := tx.record(a, x, "blackmail_refused", map[string]any{
			"text":                fmt.Sprintf("%s snapped and refused %s's %s blackmail (%d coins) over %s (offense %d/%d; backlash -%d HP)", target.Name, a.Name, attitude, demand, w.formatSecretDesc(*s), assess.AccumulatedAfter, assess.Threshold, backlashDmg),
			"secret_id":           s.ID,
			"victim_id":           target.ID,
			"subject_id":          s.SubjectID,
			"attitude":            attitude,
			"demand":              demand,
			"single_offense":      assess.SingleOffense,
			"accumulated_offense": assess.AccumulatedAfter,
			"breaking_threshold":  assess.Threshold,
			"reason":              reason,
			"observation":         Observation{Kind: "blackmail_refused", SubjectID: a.ID, ObjectID: target.ID, SecretID: s.ID, Location: a.Location},
			"backlash_damage":     backlashDmg,
			"absorbed":            absorbed,
			"immune_blocked":      immune,
			"root_event_id":       s.RootEventID,
			"cause_event_id":      s.RootEventID,
		}, observers...)
		events := append(effects, ev)
		events = append(events, w.ensureRelations().recordGrudge(tx, w, target.ID, a.ID, "blackmail_attempt", minInt(90, 45+assess.PriorRepeats*15), ev.ID, ev.ID)...)
		events = append(events, w.finishMeleeHit(tx, target, a, x, backlashDmg, "blackmail backlash", ev)...)
		return events
	}

	paid := minInt(target.Money, demand)
	if paid > 0 {
		// Exact 0-error coin transfer via canonical primitive.
		if tx.TransferCurrency(target, a, "coins", int64(paid), x.Kind) && assess.Outcome != "minor_favor" {
			a.IllicitGain += paid
		}
	}

	observers := w.awakeSamePlaceObservers(*a, a.ID)
	obs := Observation{
		Kind:      "blackmailed",
		SecretID:  s.ID,
		Modality:  "eyewitness",
		SubjectID: a.ID,
		ObjectID:  target.ID,
		Item:      s.Kind,
		Location:  a.Location,
		Quantity:  paid,
	}

	if assess.Outcome == "minor_favor" {
		// Minor favor (e.g. "buy me a meal"): slight wariness, NO blood feud, NO criminal plot!
		tx.MutateTrust(target, a.ID, -1)
		ev := tx.record(a, x, "blackmailed", map[string]any{
			"text":                fmt.Sprintf("%s subtly leveraged %s against %s for a minor favor (%d coins; offense %d/%d)", a.Name, w.formatSecretDesc(*s), target.Name, paid, assess.AccumulatedAfter, assess.Threshold),
			"secret_id":           s.ID,
			"victim_id":           target.ID,
			"subject_id":          s.SubjectID,
			"attitude":            attitude,
			"severity_tier":       "minor_favor",
			"paid_coins":          paid,
			"single_offense":      assess.SingleOffense,
			"accumulated_offense": assess.AccumulatedAfter,
			"breaking_threshold":  assess.Threshold,
			"observation":         obs,
			"root_event_id":       s.RootEventID,
			"cause_event_id":      s.RootEventID,
		}, observers...)
		return []Event{ev}
	}

	// Substantive extortion (comply_resentful): heavy trust drop, scaled grudge, reciprocal covert secret.
	tx.MutateTrust(target, a.ID, -3)
	grudgeInt := minInt(75, maxInt(20, (assess.AccumulatedAfter*60)/maxInt(1, assess.Threshold)))
	ev := tx.record(a, x, "blackmailed", map[string]any{
		"text":                fmt.Sprintf("%s blackmailed %s (%s) over %s and extorted %d coins (offense %d/%d)", a.Name, target.Name, attitude, w.formatSecretDesc(*s), paid, assess.AccumulatedAfter, assess.Threshold),
		"secret_id":           s.ID,
		"victim_id":           target.ID,
		"subject_id":          s.SubjectID,
		"attitude":            attitude,
		"severity_tier":       "substantive",
		"paid_coins":          paid,
		"single_offense":      assess.SingleOffense,
		"accumulated_offense": assess.AccumulatedAfter,
		"breaking_threshold":  assess.Threshold,
		"severity":            s.Severity,
		"observation":         obs,
		"root_event_id":       s.RootEventID,
		"cause_event_id":      s.RootEventID,
	}, observers...)
	events := []Event{ev}
	events = append(events, w.ensureRelations().recordGrudge(tx, w, target.ID, a.ID, "blackmailed", grudgeInt, ev.ID, ev.ID)...)
	return events
}

func (w *World) applyExposeSecret(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 4 {
		return fail("insufficient energy to expose secret")
	}
	secretIdx := -1
	for i := range w.Secrets {
		s := w.Secrets[i]
		if s.Exposed && ActorHasTag(s.ExposureKnownBy, a.ID) {
			continue
		}
		if !actorKnowsSecret(s, a.ID) {
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
	tx.setEnergy(a, a.Energy-4, x.Kind)
	if !s.Exposed {
		s.Exposed = true
		s.ExposedTick = w.Tick
	}
	s.ExposureKnownBy = appendUnique(s.ExposureKnownBy, a.ID)

	owner := w.actor(s.OwnerID)
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	for _, ob := range observers {
		if ActorHasTag(s.ExposureKnownBy, ob.ID) {
			continue
		}
		s.KnownBy = appendUnique(s.KnownBy, ob.ID)
		s.ExposureKnownBy = appendUnique(s.ExposureKnownBy, ob.ID)
		if ob.ID == s.OwnerID {
			continue
		}
		if ob.Trust == nil {
			ob.Trust = map[string]int{}
		}
		tx.MutateTrust(ob, s.OwnerID, -3)
	}
	if owner != nil && !owner.Dead && ActorHasTag(s.ExposureKnownBy, owner.ID) {
		if owner.ID != a.ID {
			if owner.Trust == nil {
				owner.Trust = map[string]int{}
			}
			tx.MutateTrust(owner, a.ID, -5)
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
		Proceeds:  s.IllicitProceeds,
	}
	ownerName := s.OwnerID
	if owner != nil {
		ownerName = owner.Name
	}
	witnessIDs := make([]string, 0, len(observers))
	for _, obs := range observers {
		witnessIDs = appendUnique(witnessIDs, obs.ID)
	}
	ev := tx.record(a, x, "secret_exposed", map[string]any{
		"text":          fmt.Sprintf("%s publicly exposed %s's secret (%s, severity %d) in %s", a.Name, ownerName, w.formatSecretDesc(*s), s.Severity, a.Location),
		"secret_id":     s.ID,
		"owner_id":      s.OwnerID,
		"subject_id":    s.SubjectID,
		"secret_kind":   s.Kind,
		"severity":      s.Severity,
		"witnesses":     witnessIDs,
		"witness_count": len(observers),
		"observation":   obs,
		"root_event_id": s.RootEventID,
	}, observers...)
	events := []Event{ev}
	if owner != nil && !owner.Dead && owner.ID != a.ID && ActorHasTag(s.ExposureKnownBy, owner.ID) {
		events = append(events, w.ensureRelations().recordGrudge(tx, w, owner.ID, a.ID, "secret_exposed", 45, ev.ID, ev.ID)...)
	}
	if s.Severity >= 60 && len(s.HolderStances) > 0 {
		hKeys := make([]string, 0, len(s.HolderStances))
		for hk := range s.HolderStances {
			hKeys = append(hKeys, hk)
		}
		sort.Strings(hKeys)
		witnessIDs := append([]string{a.ID}, s.ExposureKnownBy...)
		for _, hk := range hKeys {
			if hk != a.ID && hk != s.OwnerID && s.HolderStances[hk] == "shield" {
				w.addSecret(hk, "covert_complicity", s.OwnerID, maxInt(45, s.Severity-15), ev.ID, witnessIDs)
			}
		}
	}
	return events
}

func (w *World) addSecretWithWitnesses(ownerID, kind, subjectID string, severity int, rootEventID int64, knownBy, witnessedBy []string) (*ActorSecret, []Event) {
	if ownerID == "" || kind == "" {
		return nil, nil
	}
	for i := range w.Secrets {
		s := &w.Secrets[i]
		if s.OwnerID == ownerID && s.Kind == kind && s.SubjectID == subjectID && (s.RootEventID == rootEventID || len(witnessedBy) == 0 && !s.Exposed) {
			if !s.Exposed {
				for _, kb := range knownBy {
					if kb != "" {
						s.KnownBy = appendUnique(s.KnownBy, kb)
					}
				}
				for _, wb := range witnessedBy {
					if wb != "" && wb != ownerID {
						s.WitnessedBy = appendUnique(s.WitnessedBy, wb)
						s.KnownBy = appendUnique(s.KnownBy, wb)
					}
				}
				sort.Strings(s.KnownBy)
				sort.Strings(s.WitnessedBy)
				if severity > s.Severity {
					s.Severity = severity
				}
				return s, w.refreshSecretHolderStances(s)
			}
			// Never duplicate a secret spawned from the exact same root event.
			if rootEventID != 0 && s.RootEventID == rootEventID {
				return s, nil
			}
		}
	}
	// Retain up to 200 secrets, retiring public records before active leads.
	if len(w.Secrets) >= 200 {
		retire := 0
		for i := 1; i < len(w.Secrets); i++ {
			candidate, current := w.Secrets[i], w.Secrets[retire]
			if candidate.Exposed && !current.Exposed ||
				candidate.Exposed == current.Exposed && candidate.Severity < current.Severity {
				retire = i
			}
		}
		w.Secrets = append(w.Secrets[:retire], w.Secrets[retire+1:]...)
	}
	var kb, wb []string
	kb = appendUnique(kb, ownerID)
	for _, id := range knownBy {
		if id != "" {
			kb = appendUnique(kb, id)
		}
	}
	for _, id := range witnessedBy {
		if id != "" && id != ownerID {
			wb = appendUnique(wb, id)
			kb = appendUnique(kb, id)
		}
	}
	sort.Strings(kb)
	sort.Strings(wb)
	w.Secrets = append(w.Secrets, ActorSecret{
		ID:          fmt.Sprintf("sec_%d_%s_%d", w.Tick, ownerID, w.NextID),
		OwnerID:     ownerID,
		Kind:        kind,
		SubjectID:   subjectID,
		Severity:    minInt(100, maxInt(10, severity)),
		CreatedTick: w.Tick,
		RootEventID: rootEventID,
		KnownBy:     kb,
		WitnessedBy: wb,
	})
	w.NextID++
	sec := &w.Secrets[len(w.Secrets)-1]
	return sec, w.refreshSecretHolderStances(sec)
}

func (w *World) addSecret(ownerID, kind, subjectID string, severity int, rootEventID int64, knownBy []string) {
	_, _ = w.addSecretWithWitnesses(ownerID, kind, subjectID, severity, rootEventID, knownBy, nil)
}

func secretReportTopic(topic string) bool {
	switch topic {
	case "secret_caught_red_handed", "secret_discovered", "secret_exposed":
		return true
	default:
		return false
	}
}

func (w *World) deliverSecretReport(x Action, factID int64, listeners []*Actor) []Event {
	if x.Statement == nil || factID == 0 || !secretReportTopic(x.Statement.Topic) {
		return nil
	}
	for i := range w.Secrets {
		s := &w.Secrets[i]
		if s.ID == x.Statement.ObjectID && s.OwnerID == x.Statement.SubjectID {
			for _, listener := range listeners {
				s.KnownBy = appendUnique(s.KnownBy, listener.ID)
				if x.Statement.Topic == "secret_exposed" {
					s.ExposureKnownBy = appendUnique(s.ExposureKnownBy, listener.ID)
				}
			}
			return w.refreshSecretHolderStances(s)
		}
	}
	return nil
}

// captureActionSecrets runs before another action can move or incapacitate a witness.
func (w *World) captureActionSecrets(tx *WorldTx, ev Event) []Event {
	owner := w.actor(ev.ActorID)
	if owner == nil {
		return nil
	}
	var kind string
	var severity int
	coins := int(payloadInt64(ev.Payload, "coins"))
	damage := int(payloadInt64(ev.Payload, "damage"))
	switch ev.Kind {
	case "bribed":
		kind, severity = "covert_bribe", 60
	case "blackmailed":
		if relPayloadString(ev.Payload, "severity_tier") != "substantive" {
			return nil
		}
		coins = int(payloadInt64(ev.Payload, "paid_coins"))
		kind, severity = "offense", 25+coins*2
		if coins > 0 {
			kind = "illicit_gain"
		}
	case "stolen", "raided", "extorted", "party_betrayed":
		kind, severity = "offense", 20+coins+damage*2
		if coins > 0 || ev.Kind == "stolen" {
			kind = "illicit_gain"
		}
	case "steal_foiled", "wrongful_conviction":
		kind, severity = "offense", 35
	default:
		return nil
	}

	observerIDs, recorded := extractStringSlicePayload(ev.Payload, "witnesses")
	var observers []*Actor
	if recorded {
		bystanders := make([]string, 0, len(observerIDs))
		for _, id := range observerIDs {
			if id != ev.ActorID && id != ev.TargetID {
				if observer := w.actor(id); observer != nil {
					observers = append(observers, observer)
					bystanders = append(bystanders, id)
				}
			}
		}
		observerIDs = bystanders
	} else {
		observers = w.awakeSamePlaceObservers(*owner, ev.ActorID, ev.TargetID)
		observerIDs = nil
		for _, o := range observers {
			observerIDs = append(observerIDs, o.ID)
		}
	}
	root := payloadInt64(ev.Payload, "root_event_id")
	if root == 0 {
		root = ev.ID
	}
	district := relPayloadString(ev.Payload, "district")
	if district == "" {
		district = owner.Location
	}
	var out []Event
	targetKnows := ev.Kind != "stolen"
	if ev.Kind == "stolen" {
		targetKnows = w.partyMateAvailable(w.actor(ev.TargetID))
	}
	capture := func(culprit *Actor, subjectID, secretKind string, severity, proceeds int) {
		known := []string{culprit.ID}
		if targetKnows {
			known = append(known, subjectID)
		}
		s, holds := w.addSecretWithWitnesses(culprit.ID, secretKind, subjectID, severity, root, known, observerIDs)
		s.IllicitProceeds = proceeds
		out = append(out, holds...)
		obs := Observation{Kind: "secret_caught_red_handed", Modality: "eyewitness",
			SubjectID: culprit.ID, ObjectID: s.ID, Item: s.Kind, Location: district, Quantity: s.Severity, Proceeds: proceeds}
		if len(observers) > 0 {
			caught := w.recordWithTx(tx, culprit, Action{Kind: ev.Action.Kind, TargetID: culprit.ID, Item: s.ID, BasisEventID: ev.ID},
				"secret_caught_red_handed", map[string]any{
					"text":        fmt.Sprintf("Witnesses caught %s red-handed (%s) at %s", culprit.Name, s.Kind, district),
					"observation": obs, "secret_id": s.ID, "root_event_id": root, "cause_event_id": ev.ID,
				}, observers...)
			out = append(out, caught)
			for _, observer := range observers {
				seen := Observation{Kind: "witnessed_my_secret", Modality: "eyewitness", SubjectID: observer.ID,
					ObjectID: s.ID, Item: s.Kind, Location: district, Quantity: s.Severity}
				tx.RecordMemory(culprit, Memory{Kind: seen.Kind, Tick: w.Tick,
					EventID: caught.ID, RootEventID: root, CauseEventID: ev.ID, Source: observer.ID, TargetID: culprit.ID, Observation: &seen})
			}
		}
	}
	capture(owner, ev.TargetID, kind, severity, coins)
	if ev.Kind == "bribed" {
		if recipient := w.actor(ev.TargetID); recipient != nil {
			kind := "illicit_gain"
			if relPayloadString(ev.Payload, "silenced_secret_id") != "" {
				kind = "covert_complicity"
			}
			capture(recipient, owner.ID, kind, 70, int(payloadInt64(ev.Payload, "bribe_coins")))
		}
	}
	return out
}

func (w *World) stepAmbitionsAndSecrets(settled []Event) []Event {
	var events []Event
	for _, ev := range settled {
		if ev.Kind == "grudge_formed" {
			if intensity := int(payloadInt64(ev.Payload, "intensity")); intensity >= 50 {
				_, holds := w.addSecretWithWitnesses(ev.ActorID, "secret_vendetta", ev.TargetID, intensity, ev.ID, nil, nil)
				events = append(events, holds...)
			}
		}
	}

	// 2. Daily 09:00 ambition seeding, progress evaluation, and secret stance refresh.
	clock := ClockAt(w.Tick)
	if clock.Hour != 9 || clock.Minute != 0 {
		return events
	}
	for si := range w.Secrets {
		events = append(events, w.refreshSecretHolderStances(&w.Secrets[si])...)
	}
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
				if w.Relations != nil {
					for _, g := range w.Relations.Grudges {
						if g.OffenderID == amb.TargetID && g.Resolution == "avenged" && w.Relations.matchesAggrieved(a, g) && actorKnowsAboutGrudge(*a, g) {
							amb.Progress, done = 1, true
							break
						}
					}
				}
			case "expose_corrupt":
				for _, s := range w.Secrets {
					if s.Exposed && ActorHasTag(s.ExposureKnownBy, a.ID) && (s.OwnerID == amb.TargetID || s.ID == amb.TargetID) {
						amb.Progress = 1
						done = true
						break
					}
				}
				if !done {
					for _, memory := range a.Memories {
						if memory.Kind == "corruption_purged" && memory.TargetID == amb.TargetID {
							amb.Progress, done = 1, true
							break
						}
					}
				}
			}
			if done {
				amb.Completed = true
				amb.CompletedTick = w.Tick
				activeCount--
				desc := amb.Kind
				if amb.TargetID != "" {
					desc = fmt.Sprintf("%s (%s)", amb.Kind, amb.TargetID)
				}
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: a.ID,
					Kind:    "ambition_completed",
					Action:  Action{Kind: "set_ambition", Item: amb.Kind, TargetID: amb.TargetID},
					Payload: map[string]any{
						"text":         fmt.Sprintf("%s fulfilled long-term ambition: %s", a.Name, desc),
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

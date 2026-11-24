package sim

import (
	"fmt"
	"sort"
	"strings"
)

// TechSpec declaratively defines an empirical technique or facility technology in Catalog.Techs.
// Discovery is personal (KnowledgeBook.Records[id].KnownActors) and physical effects only manifest
// where a facility is locally built/adopted (House.Facilities or Venue), never via a global tech tree.
type TechSpec struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	GrantedByTags      []string       `json:"granted_by_tags,omitempty"`
	PrerequisiteTechs  []string       `json:"prerequisite_techs,omitempty"`
	ResearchTicks      int            `json:"research_ticks,omitempty"`
	ResearchEnergyCost int            `json:"research_energy_cost,omitempty"`
	ResearchInputs     map[string]int `json:"research_inputs,omitempty"`
	FacilityName       string         `json:"facility_name,omitempty"`
	AdoptInputs        map[string]int `json:"adopt_inputs,omitempty"`
	AdoptTicks         int            `json:"adopt_ticks,omitempty"`
	AdoptEnergyCost    int            `json:"adopt_energy_cost,omitempty"`
	GrantsVenueFeature string         `json:"grants_venue_feature,omitempty"`
	// Inputs are required operating resources; deltas are independent field effects
	// such as pollution or cleanup and do not establish an operating requirement.
	AmbientFieldInputsPerTick map[string]int `json:"ambient_field_inputs_per_tick,omitempty"`
	AmbientFieldDeltaPerTick  map[string]int `json:"ambient_field_delta_per_tick,omitempty"`
	ProductionBonusPct        int            `json:"production_bonus_pct,omitempty"`
	ShareWithFaction          bool           `json:"share_with_faction,omitempty"`
}

// KnowledgeRecord tracks causal discovery, personal/archival diffusion, and physical site adoption
// for a single TechSpec without ever broadcasting knowledge globally.
type KnowledgeRecord struct {
	TechID         string           `json:"tech_id"`
	DiscovererID   string           `json:"discoverer_id,omitempty"`
	DiscoveredTick int64            `json:"discovered_tick,omitempty"`
	KnownActors    []string         `json:"known_actors,omitempty"`
	KnownFactions  []string         `json:"known_factions,omitempty"`
	AdoptedVenues  []string         `json:"adopted_venues,omitempty"`
	AdoptedHouses  []string         `json:"adopted_houses,omitempty"`
	OperatingTicks map[string]int64 `json:"operating_ticks,omitempty"`
}

// KnowledgeBook stores State A (Technical Knowledge & Local Facility Adoption) separately from
// PolityNetwork (State B) and fundamental WorldLaws (State C).
type KnowledgeBook struct {
	Records                map[string]*KnowledgeRecord `json:"records,omitempty"`
	AmbientAlertWatermarks map[string]map[string]int   `json:"ambient_alert_watermarks,omitempty"`
}

const ambientShiftThresholdStep = 10

func cloneStringSlicePreserveNil(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}

func cloneIntMapPreserveNil(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// CloneTechSpec copies a TechSpec while preserving nil slices and maps for lossless JSON round-trips.
func CloneTechSpec(spec TechSpec) TechSpec {
	out := spec
	out.GrantedByTags = cloneStringSlicePreserveNil(spec.GrantedByTags)
	out.PrerequisiteTechs = cloneStringSlicePreserveNil(spec.PrerequisiteTechs)
	out.ResearchInputs = cloneIntMapPreserveNil(spec.ResearchInputs)
	out.AdoptInputs = cloneIntMapPreserveNil(spec.AdoptInputs)
	out.AmbientFieldDeltaPerTick = cloneIntMapPreserveNil(spec.AmbientFieldDeltaPerTick)
	out.AmbientFieldInputsPerTick = cloneIntMapPreserveNil(spec.AmbientFieldInputsPerTick)
	return out
}

// CloneKnowledgeBook copies a KnowledgeBook while preserving nil maps and slices.
func CloneKnowledgeBook(kb *KnowledgeBook) *KnowledgeBook {
	if kb == nil {
		return nil
	}
	out := &KnowledgeBook{}
	if kb.Records != nil {
		out.Records = make(map[string]*KnowledgeRecord, len(kb.Records))
		for k, rec := range kb.Records {
			if rec == nil {
				out.Records[k] = nil
				continue
			}
			rc := *rec
			rc.KnownActors = cloneStringSlicePreserveNil(rec.KnownActors)
			rc.KnownFactions = cloneStringSlicePreserveNil(rec.KnownFactions)
			rc.AdoptedVenues = cloneStringSlicePreserveNil(rec.AdoptedVenues)
			rc.AdoptedHouses = cloneStringSlicePreserveNil(rec.AdoptedHouses)
			if rec.OperatingTicks != nil {
				rc.OperatingTicks = make(map[string]int64, len(rec.OperatingTicks))
				for site, tick := range rec.OperatingTicks {
					rc.OperatingTicks[site] = tick
				}
			}
			out.Records[k] = &rc
		}
	}
	if kb.AmbientAlertWatermarks != nil {
		out.AmbientAlertWatermarks = make(map[string]map[string]int, len(kb.AmbientAlertWatermarks))
		for regID, fields := range kb.AmbientAlertWatermarks {
			out.AmbientAlertWatermarks[regID] = cloneIntMapPreserveNil(fields)
		}
	}
	return out
}

func (w *World) ensureKnowledge() *KnowledgeBook {
	if w.Knowledge == nil {
		w.Knowledge = &KnowledgeBook{}
	}
	return w.Knowledge
}

// KnowsTech reports whether actorID personally knows techID (never inferred from global or rival knowledge).
func (w *World) KnowsTech(actorID, techID string) bool {
	if w == nil || actorID == "" || techID == "" || w.Knowledge == nil || w.Knowledge.Records == nil {
		return false
	}
	rec := w.Knowledge.Records[techID]
	if rec == nil {
		return false
	}
	return ActorHasTag(rec.KnownActors, actorID)
}

// ActorKnownTechs returns the sorted list of TechIDs personally known by actorID, or nil if none.
func (w *World) ActorKnownTechs(actorID string) []string {
	return actorKnownTechsFromBook(w.Knowledge, actorID)
}

func actorKnownTechsFromBook(kb *KnowledgeBook, actorID string) []string {
	if kb == nil || len(kb.Records) == 0 || actorID == "" {
		return nil
	}
	var out []string
	for techID, rec := range kb.Records {
		if rec != nil && ActorHasTag(rec.KnownActors, actorID) {
			out = append(out, techID)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

func actorMatchesTechTags(cat Catalog, a Actor, grantedByTags []string) bool {
	if len(grantedByTags) == 0 {
		return true
	}
	tags := actorEffectiveTags(cat, a)
	for _, gt := range grantedByTags {
		if ActorHasTag(tags, gt) {
			return true
		}
	}
	return false
}

// CanResearchTechWithKnown validates whether actor `a` meets tag, prerequisite, energy, and input
// requirements to research `tech`, given `knownTechs` (the actor's own known tech IDs).
func CanResearchTechWithKnown(cat Catalog, a Actor, tech TechSpec, knownTechs []string) (bool, string) {
	if tech.ID == "" {
		return false, "missing tech id"
	}
	if ActorHasTag(knownTechs, tech.ID) {
		return false, "actor already knows tech " + tech.ID
	}
	if !actorMatchesTechTags(cat, a, tech.GrantedByTags) {
		return false, "identity tags do not permit researching " + tech.ID
	}
	for _, pre := range tech.PrerequisiteTechs {
		if !ActorHasTag(knownTechs, pre) {
			return false, fmt.Sprintf("researching %s requires prerequisite tech %s", tech.ID, pre)
		}
	}
	if a.Energy < tech.ResearchEnergyCost {
		return false, fmt.Sprintf("insufficient energy to research %s", tech.ID)
	}
	for itemID, qty := range tech.ResearchInputs {
		if a.Items[itemID] < qty {
			return false, fmt.Sprintf("researching %s requires %d %s", tech.ID, qty, itemID)
		}
	}
	return true, ""
}

// CanResearchTech checks whether actor `a` in `w` can start researching `tech`.
func (w *World) CanResearchTech(a Actor, tech TechSpec) (bool, string) {
	if _, exists := w.Catalog.Techs[tech.ID]; !exists {
		return false, "unknown tech: " + tech.ID
	}
	return CanResearchTechWithKnown(w.Catalog, a, tech, w.ActorKnownTechs(a.ID))
}

func (w *World) applyResearchTech(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if x.TechID == "" {
		return fail("research_tech requires tech_id")
	}
	tech, ok := w.Catalog.Techs[x.TechID]
	if !ok {
		return fail("unknown tech: " + x.TechID)
	}
	if can, reason := w.CanResearchTech(*a, tech); !can {
		return fail(reason)
	}
	for _, itemID := range sortedProductionKeys(tech.ResearchInputs) {
		qty := tech.ResearchInputs[itemID]
		tx.ProduceOrConsumeItem(a, itemID, -qty, x.Kind)
	}
	ticks := tech.ResearchTicks
	if ticks < 1 {
		ticks = 1
	}
	task := Task{Action: x, TotalTicks: ticks, Remaining: ticks - 1, StartedTick: w.Tick}
	tx.startTaskWork(a, &task, tech.ResearchEnergyCost)
	if task.Remaining <= 0 {
		return w.finishTaskWithTx(tx, a, task)
	}
	a.Task = &task
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s began researching %s (%d ticks)", a.Name, tech.Name, ticks),
		"tech_id":         tech.ID,
		"remaining_ticks": task.Remaining,
	})}
}

func (w *World) finishResearchTech(tx *WorldTx, a *Actor, t Task) []Event {
	x := t.Action
	tech, ok := w.Catalog.Techs[x.TechID]
	if !ok {
		return nil
	}
	return w.unlockTechForActor(tx, a, x, tech)
}

// unlockTechForActor records personal discovery/unlock of tech in KnowledgeBook and emits tech_discovered.
func (w *World) unlockTechForActor(tx *WorldTx, a *Actor, x Action, tech TechSpec) []Event {
	kb := w.ensureKnowledge()
	if kb.Records == nil {
		kb.Records = make(map[string]*KnowledgeRecord)
	}
	rec := kb.Records[tech.ID]
	if rec == nil {
		rec = &KnowledgeRecord{
			TechID:         tech.ID,
			DiscovererID:   a.ID,
			DiscoveredTick: w.Tick,
		}
		kb.Records[tech.ID] = rec
	} else if rec.DiscovererID == "" {
		rec.DiscovererID = a.ID
		rec.DiscoveredTick = w.Tick
	}
	rec.KnownActors = appendUnique(rec.KnownActors, a.ID)
	if tech.ShareWithFaction {
		if x.FactionID != "" && ActorHasTag(a.Guilds, x.FactionID) {
			rec.KnownFactions = appendUnique(rec.KnownFactions, x.FactionID)
		} else if len(a.Guilds) > 0 {
			rec.KnownFactions = appendUnique(rec.KnownFactions, a.Guilds[0])
		}
	}

	obs := Observation{
		Kind:      "tech_discovered",
		SubjectID: a.ID,
		Item:      tech.ID,
		Location:  a.Location,
		Quantity:  1,
	}
	ev := tx.record(a, x, "tech_discovered", map[string]any{
		"text":          fmt.Sprintf("%s discovered %s (%s)", a.Name, tech.Name, tech.ID),
		"tech_id":       tech.ID,
		"tech_name":     tech.Name,
		"discoverer_id": a.ID,
		"observation":   obs,
	})
	return []Event{ev}
}

func (w *World) applyTeachKnowledge(tx *WorldTx, a *Actor, x Action, b *Actor) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if x.TechID == "" && x.SkillID != "" {
		return w.applyTeachSkill(tx, a, x, b)
	}
	if x.TechID == "" {
		return fail("teach_knowledge requires tech_id or skill_id")
	}
	tech, ok := w.Catalog.Techs[x.TechID]
	if !ok {
		return fail("unknown tech: " + x.TechID)
	}
	if !w.KnowsTech(a.ID, tech.ID) {
		return fail("actor does not know tech " + tech.ID)
	}
	if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil {
		return fail("student is unavailable")
	}
	if b.Task != nil && (b.Task.Action.Kind == "sleep" || b.Task.Dormant) {
		return fail("student is asleep")
	}
	if w.KnowsTech(b.ID, tech.ID) {
		return fail(b.Name + " already knows " + tech.Name)
	}

	colocated := SamePlace(*a, *b)
	usedChannel := ""
	if !colocated {
		if x.Channel == "" {
			return fail("target is not co-located and no communication channel was specified")
		}
		ch, exists := w.Catalog.CommChannels[x.Channel]
		if !exists {
			return fail("unknown communication channel: " + x.Channel)
		}
		tags := actorEffectiveTags(w.Catalog, *a)
		if len(ch.GrantedByTags) > 0 {
			granted := false
			for _, gt := range ch.GrantedByTags {
				if ActorHasTag(tags, gt) {
					granted = true
					break
				}
			}
			if !granted {
				return fail("actor lacks transmitter capability for channel " + ch.ID)
			}
		}
		if len(ch.ReceiverTags) > 0 {
			rTags := actorEffectiveTags(w.Catalog, *b)
			hasRcpt := false
			for _, rt := range ch.ReceiverTags {
				if ActorHasTag(rTags, rt) {
					hasRcpt = true
					break
				}
			}
			if !hasRcpt {
				return fail("student lacks receiver capability for channel " + ch.ID)
			}
		}
		_, senderShield := w.actorEnclosure(*a)
		if senderShield > ch.PenetrationPower {
			return fail("sender enclosure shielding blocks transmission on " + ch.ID)
		}
		_, rcptShield := w.actorEnclosure(*b)
		if rcptShield > ch.PenetrationPower {
			return fail("student enclosure shielding blocks transmission on " + ch.ID)
		}
		if !w.communicationRangeAllows(*a, *b, ch.RangeKm) {
			return fail("student is outside channel range or distance is unavailable for " + ch.ID)
		}
		usedChannel = ch.ID
	}

	if x.BasisEventID == 0 {
		for i := len(a.Memories) - 1; i >= 0; i-- {
			m := a.Memories[i]
			if m.Kind == "tech_discovered" || m.Kind == "knowledge_taught" {
				if (m.Observation != nil && m.Observation.Item == tech.ID) ||
					(m.Statement != nil && m.Statement.Item == tech.ID) {
					x.BasisEventID = m.EventID
					break
				}
			}
		}
	}

	rec := w.Knowledge.Records[tech.ID]
	rec.KnownActors = appendUnique(rec.KnownActors, b.ID)
	tx.MutateTrust(a, b.ID, 1)
	tx.MutateTrust(b, a.ID, 1)

	msgText := x.Text
	if msgText == "" {
		msgText = fmt.Sprintf("%s taught %s (%s) to %s", a.Name, tech.Name, tech.ID, b.Name)
	}

	if colocated {
		obs := Observation{
			Kind:      "knowledge_taught",
			SubjectID: a.ID,
			ObjectID:  b.ID,
			Item:      tech.ID,
			Location:  a.Location,
			Quantity:  1,
		}
		ev := tx.record(a, x, "knowledge_taught", map[string]any{
			"text":          msgText,
			"tech_id":       tech.ID,
			"tech_name":     tech.Name,
			"teacher_id":    a.ID,
			"student_id":    b.ID,
			"discoverer_id": rec.DiscovererID,
			"observation":   obs,
		}, b)
		return append([]Event{ev}, w.ensureRelations().recordMentorshipWithTx(tx, w, a, b, tech.ID)...)
	}

	// Remote channel instruction records Channel + Statement and never fabricates an Observation on student.
	e := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  a.ID,
		TargetID: b.ID,
		Kind:     "knowledge_taught",
		Action:   x,
		Payload: map[string]any{
			"text":          msgText,
			"tech_id":       tech.ID,
			"tech_name":     tech.Name,
			"teacher_id":    a.ID,
			"student_id":    b.ID,
			"discoverer_id": rec.DiscovererID,
			"channel_id":    usedChannel,
		},
	}
	w.NextID++
	root := e.ID
	if x.BasisEventID != 0 {
		for _, m := range a.Memories {
			if m.EventID == x.BasisEventID {
				root = m.RootEventID
				break
			}
		}
	}
	e.Payload["basis_event_id"] = x.BasisEventID
	e.Payload["root_event_id"] = root

	stmt := Statement{
		Topic:     "knowledge_taught",
		SubjectID: a.ID,
		ObjectID:  b.ID,
		Item:      tech.ID,
		Location:  a.Location,
		Quantity:  1,
	}
	e.Payload["statement"] = stmt

	teacherMem := Memory{
		Kind:         "knowledge_taught",
		Channel:      usedChannel,
		Tick:         w.Tick,
		EventID:      e.ID,
		Source:       a.ID,
		SourceRole:   a.Role,
		SourceTags:   append([]string(nil), a.IdentityTags()...),
		TargetID:     b.ID,
		Text:         msgText,
		BasisEventID: x.BasisEventID,
		RootEventID:  root,
		Statement:    &stmt,
	}
	tx.RecordMemory(a, teacherMem)

	studentStmt := stmt
	studentMem := Memory{
		Kind:         "knowledge_taught",
		Channel:      usedChannel,
		Tick:         w.Tick,
		EventID:      e.ID,
		Source:       a.ID,
		SourceRole:   a.Role,
		SourceTags:   append([]string(nil), a.IdentityTags()...),
		TargetID:     b.ID,
		Text:         msgText,
		BasisEventID: x.BasisEventID,
		RootEventID:  root,
		Statement:    &studentStmt,
	}
	tx.RecordMemory(b, studentMem)
	w.ObserveContinuity(e)
	return append([]Event{e}, w.ensureRelations().recordMentorshipWithTx(tx, w, a, b, tech.ID)...)
}

func (w *World) applyTeachSkill(tx *WorldTx, a *Actor, x Action, b *Actor) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	skill, ok := w.Catalog.Skills[x.SkillID]
	if !ok {
		return fail("unknown skill: " + x.SkillID)
	}
	teacherLv := a.Skills[skill.ID]
	if teacherLv < 2 {
		return fail("teacher needs at least level 2 in skill " + skill.ID)
	}
	if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil {
		return fail("student is unavailable")
	}
	if b.Task != nil && (b.Task.Action.Kind == "sleep" || b.Task.Dormant) {
		return fail("student is asleep")
	}
	studentLv := b.Skills[skill.ID]
	if studentLv >= teacherLv {
		return fail(b.Name + " is already as proficient in " + skill.Name)
	}
	if !SamePlace(*a, *b) {
		return fail("student must be co-located for skill mentorship")
	}
	tx.MutateTrust(a, b.ID, 1)
	tx.MutateTrust(b, a.ID, 2)
	xpGain := 12 + teacherLv*2
	skillEvents := w.grantSkillXP(tx, b, x, skill.ID, xpGain)
	w.recordSocialContact(a, b)
	newLv := b.Skills[skill.ID]
	msgText := x.Text
	if msgText == "" {
		msgText = fmt.Sprintf("%s mentored %s in %s (level %d -> %d)", a.Name, b.Name, skill.Name, studentLv, newLv)
	}
	obs := Observation{
		Kind:      "knowledge_taught",
		SubjectID: a.ID,
		ObjectID:  b.ID,
		Item:      skill.ID,
		Location:  a.Location,
		Quantity:  newLv,
	}
	ev := tx.record(a, x, "knowledge_taught", map[string]any{
		"text":                msgText,
		"skill_id":            skill.ID,
		"skill_name":          skill.Name,
		"teacher_id":          a.ID,
		"student_id":          b.ID,
		"teacher_skill_level": teacherLv,
		"student_skill_level": newLv,
		"xp_gained":           xpGain,
		"observation":         obs,
	}, b)
	out := append([]Event{ev}, skillEvents...)
	return append(out, w.ensureRelations().recordMentorshipWithTx(tx, w, a, b, skill.ID)...)
}

func (w *World) facilityAdoptionSiteFailure(a Actor, x Action) string {
	if x.HouseID != "" {
		h := w.house(x.HouseID)
		if h == nil || h.District != a.Location {
			return "target house is not in current district"
		}
		if h.VenueID != "" && a.VenueID != h.VenueID {
			return "must be at house venue " + h.VenueID + " to adopt facility there"
		}
		if a.InsideHouse != "" && a.InsideHouse != h.ID {
			return "must leave current house or target current house"
		}
		if !h.CanAccess(a.ID) {
			return "actor does not have access to house " + h.ID
		}
		return ""
	}
	v, ok := w.Map.VenueByID(x.Location)
	if !ok {
		return "unknown venue: " + x.Location
	}
	if a.InsideHouse != "" || a.VenueID != v.ID || (v.DistrictID != "" && v.DistrictID != a.Location) {
		return "must be at venue " + v.ID + " to adopt facility there"
	}
	return ""
}

func (w *World) applyAdoptFacility(tx *WorldTx, a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if x.TechID == "" {
		return fail("adopt_facility requires tech_id")
	}
	tech, ok := w.Catalog.Techs[x.TechID]
	if !ok {
		return fail("unknown tech: " + x.TechID)
	}
	if !w.KnowsTech(a.ID, tech.ID) {
		return fail("actor does not know tech " + tech.ID)
	}

	rec := w.Knowledge.Records[tech.ID]
	targetHouseID := x.HouseID
	if targetHouseID == "" && a.InsideHouse != "" {
		targetHouseID = a.InsideHouse
	}

	var targetVenueID string
	if targetHouseID != "" {
		x.HouseID = targetHouseID
		if reason := w.facilityAdoptionSiteFailure(*a, x); reason != "" {
			return fail(reason)
		}
		h := w.house(targetHouseID)
		if (tech.FacilityName != "" && h.HasFacility(tech.FacilityName)) || ActorHasTag(rec.AdoptedHouses, h.ID) {
			return fail("facility already adopted at house " + h.ID)
		}
		x.HouseID = h.ID
		x.Location = ""
	} else {
		if a.InsideHouse != "" {
			return fail("step outside to adopt a venue facility")
		}
		targetVenueID = x.Location
		if targetVenueID == "" {
			targetVenueID = a.VenueID
		}
		if targetVenueID == "" {
			targetVenueID = w.Map.DefaultVenueForDistrict(a.Location)
		}
		if targetVenueID == "" {
			return fail("no target house or venue for facility adoption")
		}
		x.Location = targetVenueID
		if reason := w.facilityAdoptionSiteFailure(*a, x); reason != "" {
			return fail(reason)
		}
		if ActorHasTag(rec.AdoptedVenues, targetVenueID) {
			return fail("facility already adopted at venue " + targetVenueID)
		}
		x.Location = targetVenueID
	}

	if a.Energy < tech.AdoptEnergyCost {
		return fail("insufficient energy to adopt facility " + tech.ID)
	}
	for _, itemID := range sortedProductionKeys(tech.AdoptInputs) {
		qty := tech.AdoptInputs[itemID]
		if a.Items[itemID] < qty {
			return fail(fmt.Sprintf("adopting %s requires %d %s", tech.ID, qty, itemID))
		}
	}
	var prepaidItems map[string]int
	for _, itemID := range sortedProductionKeys(tech.AdoptInputs) {
		qty := tech.AdoptInputs[itemID]
		if qty > 0 {
			if prepaidItems == nil {
				prepaidItems = map[string]int{}
			}
			prepaidItems[itemID] += qty
		}
		tx.ProduceOrConsumeItem(a, itemID, -qty, x.Kind)
	}

	ticks := tech.AdoptTicks
	if ticks < 1 {
		ticks = 1
	}
	task := Task{Action: x, TotalTicks: ticks, Remaining: ticks - 1, StartedTick: w.Tick, PrepaidItems: prepaidItems}
	tx.startTaskWork(a, &task, tech.AdoptEnergyCost)
	if task.Remaining <= 0 {
		task.PrepaidItems = nil
		return w.finishTaskWithTx(tx, a, task)
	}
	a.Task = &task
	siteLabel := targetVenueID
	if targetHouseID != "" {
		siteLabel = targetHouseID
	}
	return []Event{tx.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s began constructing %s at %s (%d ticks)", a.Name, tech.Name, siteLabel, ticks),
		"tech_id":         tech.ID,
		"remaining_ticks": task.Remaining,
	})}
}

func (w *World) finishAdoptFacility(tx *WorldTx, a *Actor, t Task) []Event {
	x := t.Action
	if reason := w.facilityAdoptionSiteFailure(*a, x); reason != "" {
		w.refundTaskPrepayments(tx, a, t)
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	tech, ok := w.Catalog.Techs[x.TechID]
	if !ok {
		return nil
	}
	kb := w.ensureKnowledge()
	if kb.Records == nil {
		kb.Records = make(map[string]*KnowledgeRecord)
	}
	rec := kb.Records[tech.ID]
	if rec == nil {
		rec = &KnowledgeRecord{
			TechID:         tech.ID,
			DiscovererID:   a.ID,
			DiscoveredTick: w.Tick,
			KnownActors:    []string{a.ID},
		}
		kb.Records[tech.ID] = rec
	}

	siteID := ""
	facilityName := tech.FacilityName
	if facilityName == "" {
		facilityName = tech.ID
	}
	if x.HouseID != "" {
		siteID = x.HouseID
		if h := w.house(x.HouseID); h != nil && tech.FacilityName != "" {
			h.Facilities = appendUnique(h.Facilities, tech.FacilityName)
		}
		rec.AdoptedHouses = appendUnique(rec.AdoptedHouses, x.HouseID)
	} else {
		venueID := x.Location
		if venueID == "" {
			venueID = a.VenueID
		}
		siteID = venueID
		rec.AdoptedVenues = appendUnique(rec.AdoptedVenues, venueID)
		if tech.GrantsVenueFeature != "" {
			for vi := range w.Map.Venues {
				if w.Map.Venues[vi].ID == venueID {
					w.Map.Venues[vi].Features = appendUnique(w.Map.Venues[vi].Features, tech.GrantsVenueFeature)
					break
				}
			}
		}
	}

	observers := w.awakeSamePlaceObservers(*a, a.ID)
	obs := Observation{
		Kind:      "facility_adopted",
		SubjectID: a.ID,
		ObjectID:  siteID,
		Item:      tech.ID,
		Location:  a.Location,
		Quantity:  1,
	}
	ev := tx.record(a, x, "facility_adopted", map[string]any{
		"text":          fmt.Sprintf("%s adopted %s (%s) at %s", a.Name, tech.Name, facilityName, siteID),
		"tech_id":       tech.ID,
		"tech_name":     tech.Name,
		"facility_name": facilityName,
		"site_id":       siteID,
		"venue_feature": tech.GrantsVenueFeature,
		"observation":   obs,
	}, observers...)
	return []Event{ev}
}

// FacilityProductionBonusPct returns the cumulative ProductionBonusPct granted ONLY by
// physically adopted facilities at the actor's current House or Venue.
func (w *World) FacilityProductionBonusPct(a Actor) int {
	if w == nil || w.Knowledge == nil || len(w.Knowledge.Records) == 0 {
		return 0
	}
	techIDs := make([]string, 0, len(w.Knowledge.Records))
	for id := range w.Knowledge.Records {
		techIDs = append(techIDs, id)
	}
	sort.Strings(techIDs)

	bonusPct := 0
	for _, id := range techIDs {
		rec := w.Knowledge.Records[id]
		if rec == nil {
			continue
		}
		tech, ok := w.Catalog.Techs[id]
		if !ok || tech.ProductionBonusPct <= 0 {
			continue
		}
		siteKey := ""
		if a.InsideHouse != "" && ActorHasTag(rec.AdoptedHouses, a.InsideHouse) {
			siteKey = "house:" + a.InsideHouse
		} else if a.InsideHouse == "" && a.VenueID != "" && ActorHasTag(rec.AdoptedVenues, a.VenueID) {
			siteKey = "venue:" + a.VenueID
		}
		if siteKey == "" || w.isVenuePowerBlocked(a.VenueID) {
			continue
		}
		if len(tech.AmbientFieldInputsPerTick) > 0 {
			if tick, ran := rec.OperatingTicks[siteKey]; !ran || tick != w.Tick {
				continue
			}
		}
		bonusPct += tech.ProductionBonusPct
	}
	return bonusPct
}

// stepAdoptedFacilities runs per-tick physical/supernatural field consequences for adopted facilities
// that declare AmbientFieldDeltaPerTick, emitting first-hand ambient_shift Observations when a
// GeoRegion's ambient field crosses a noticeable threshold.
func (w *World) stepAdoptedFacilities() []Event {
	if w == nil || w.Knowledge == nil || len(w.Knowledge.Records) == 0 || len(w.Map.GeoRegions) == 0 {
		return nil
	}
	techIDs := make([]string, 0, len(w.Knowledge.Records))
	for id := range w.Knowledge.Records {
		techIDs = append(techIDs, id)
	}
	sort.Strings(techIDs)

	type siteDelta struct {
		regID   string
		siteID  string
		field   string
		oldVal  int
		newVal  int
		applied int
	}
	var deltas []siteDelta

	for _, techID := range techIDs {
		rec := w.Knowledge.Records[techID]
		if rec == nil {
			continue
		}
		tech, ok := w.Catalog.Techs[techID]
		if !ok || (len(tech.AmbientFieldDeltaPerTick) == 0 && len(tech.AmbientFieldInputsPerTick) == 0) {
			continue
		}
		fieldSet := make(map[string]bool)
		for fk := range tech.AmbientFieldDeltaPerTick {
			fieldSet[fk] = true
		}
		for fk := range tech.AmbientFieldInputsPerTick {
			fieldSet[fk] = true
		}
		fieldKeys := make([]string, 0, len(fieldSet))
		for fk := range fieldSet {
			fieldKeys = append(fieldKeys, fk)
		}
		sort.Strings(fieldKeys)

		applyAtSite := func(siteID, siteKey, venueID, regID string) {
			if regID == "" || w.isVenuePowerBlocked(venueID) {
				return
			}
			if tick, ran := rec.OperatingTicks[siteKey]; ran && tick == w.Tick {
				return
			}
			var reg *GeoRegion
			for ri := range w.Map.GeoRegions {
				if w.Map.GeoRegions[ri].ID == regID {
					reg = &w.Map.GeoRegions[ri]
					break
				}
			}
			if reg == nil {
				return
			}
			// Check every operating input before consuming any field. A short
			// supply leaves all resources intact and grants no operating bonus.
			for fk, qty := range tech.AmbientFieldInputsPerTick {
				if reg.AmbientFields[fk] < qty {
					return
				}
			}
			if reg.AmbientFields == nil {
				reg.AmbientFields = make(map[string]int, len(fieldKeys))
			}
			if rec.OperatingTicks == nil {
				rec.OperatingTicks = make(map[string]int64)
			}
			rec.OperatingTicks[siteKey] = w.Tick
			for _, fk := range fieldKeys {
				d := tech.AmbientFieldDeltaPerTick[fk]
				if d == 0 && tech.AmbientFieldInputsPerTick[fk] == 0 {
					continue
				}
				if fk == "spirit_qi" && d > 0 && w.HasWorldLaw("qi_tide_permille") {
					d = (d * w.EffectiveWorldLaw("qi_tide_permille")) / 1000
				}
				oldVal := reg.AmbientFields[fk]
				newVal := oldVal - tech.AmbientFieldInputsPerTick[fk] + d
				if newVal < 0 {
					newVal = 0
				}
				if newVal == oldVal {
					continue
				}
				reg.AmbientFields[fk] = newVal
				deltas = append(deltas, siteDelta{
					regID:   regID,
					siteID:  siteID,
					field:   fk,
					oldVal:  oldVal,
					newVal:  newVal,
					applied: newVal - oldVal,
				})
			}
		}

		for _, venueID := range rec.AdoptedVenues {
			regID := w.Map.GeoRegionIDForLocation("", venueID)
			applyAtSite(venueID, "venue:"+venueID, venueID, regID)
		}
		for _, houseID := range rec.AdoptedHouses {
			if h := w.house(houseID); h != nil {
				regID := w.Map.GeoRegionIDForLocation(h.District, h.VenueID)
				applyAtSite(houseID, "house:"+houseID, h.VenueID, regID)
			}
		}
	}

	if len(deltas) == 0 {
		return nil
	}

	// Aggregate region-field transitions so thresholds trigger cleanly once per crossing.
	type regionFieldKey struct {
		regID string
		field string
	}
	agg := make(map[regionFieldKey]siteDelta)
	var order []regionFieldKey
	for _, sd := range deltas {
		key := regionFieldKey{regID: sd.regID, field: sd.field}
		if existing, exists := agg[key]; exists {
			existing.newVal = sd.newVal
			existing.applied += sd.applied
			agg[key] = existing
		} else {
			agg[key] = sd
			order = append(order, key)
		}
	}

	var events []Event
	for _, key := range order {
		sd := agg[key]
		if w.Knowledge.AmbientAlertWatermarks == nil {
			w.Knowledge.AmbientAlertWatermarks = make(map[string]map[string]int)
		}
		regMarks := w.Knowledge.AmbientAlertWatermarks[sd.regID]
		if regMarks == nil {
			regMarks = make(map[string]int)
			w.Knowledge.AmbientAlertWatermarks[sd.regID] = regMarks
		}
		prevWatermark, hasMark := regMarks[sd.field]
		if !hasMark {
			prevWatermark = sd.oldVal
			regMarks[sd.field] = prevWatermark
		}
		diff := sd.newVal - prevWatermark
		if diff < 0 {
			diff = -diff
		}
		depletedCross := sd.newVal == 0 && prevWatermark > 0
		if diff < ambientShiftThresholdStep && !depletedCross {
			continue
		}
		regMarks[sd.field] = sd.newVal

		ev := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: "world",
			Kind:    "ambient_shift",
			Action:  Action{Kind: "wait", Location: sd.regID, Item: sd.field},
			Payload: map[string]any{
				"text":     fmt.Sprintf("Ambient %s in %s shifted from %d to %d", sd.field, sd.regID, prevWatermark, sd.newVal),
				"region":   sd.regID,
				"site_id":  sd.siteID,
				"field":    sd.field,
				"previous": prevWatermark,
				"value":    sd.newVal,
				"delta":    sd.newVal - prevWatermark,
			},
		}
		w.NextID++
		witnesses := w.distributeAmbientShiftObservations(ev, sd.regID, sd.siteID, sd.field, prevWatermark, sd.newVal)
		ev.Payload["witness_count"] = witnesses
		events = append(events, ev)
	}
	return events
}

func (w *World) distributeAmbientShiftObservations(ev Event, regID, siteID, field string, prevVal, newVal int) int {
	reg, hasReg := w.Map.GeoRegionByID(regID)
	witnesses := 0
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead || w.Tick < a.JailedUntil || a.InTransit() {
			continue
		}
		if a.Task != nil && (a.Task.Action.Kind == "sleep" || a.Task.Dormant) {
			continue
		}
		actorRegID := w.Map.GeoRegionIDForLocation(a.Location, a.VenueID)
		inRegion := actorRegID == regID
		indoor, encShielding := w.actorEnclosure(*a)

		modality := ""
		if len(w.Catalog.SensoryModes) > 0 && hasReg {
			actorCoord, hasActorCoord := w.Map.GeoCoordOfLocation(a.Location, a.VenueID)
			if hasActorCoord {
				distKm := GeoDistanceKm(actorCoord, reg.Coord)
				tags := actorEffectiveTags(w.Catalog, *a)
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
					if !granted {
						continue
					}
					if indoor && (!spec.PenetratesIndoor || encShielding > spec.PenetrationPower) {
						continue
					}
					modality = spec.ID
					break
				}
			}
		}
		if modality == "" && inRegion && (!indoor || encShielding < 80) {
			modality = "eyewitness"
		}
		if modality == "" {
			continue
		}

		obs := Observation{
			Kind:      "ambient_shift",
			Modality:  modality,
			SubjectID: siteID,
			Item:      field,
			Location:  regID,
			Quantity:  newVal,
		}
		mem := Memory{
			Kind:        "ambient_shift",
			Tick:        w.Tick,
			EventID:     ev.ID,
			RootEventID: ev.ID,
			Source:      "world",
			Text:        fmt.Sprintf("Perceived %s shift in %s (%d -> %d via %s)", field, regID, prevVal, newVal, modality),
			Observation: &obs,
		}
		appendActorMemory(a, mem)
		witnesses++
	}
	return witnesses
}

// HasWorldLaw reports whether key is declared in World.WorldLaws or Catalog.DefaultWorldLaws.
func (w *World) HasWorldLaw(key string) bool {
	if w == nil || key == "" {
		return false
	}
	if w.WorldLaws != nil {
		if _, ok := w.WorldLaws[key]; ok {
			return true
		}
	}
	if w.Catalog.DefaultWorldLaws != nil {
		if _, ok := w.Catalog.DefaultWorldLaws[key]; ok {
			return true
		}
	}
	return false
}

// EffectiveWorldLaw returns the active value of fundamental world law `key`,
// reading World.WorldLaws[key] first and falling back to Catalog.DefaultWorldLaws[key].
func (w *World) EffectiveWorldLaw(key string) int {
	if w == nil || key == "" {
		return 0
	}
	if w.WorldLaws != nil {
		if v, ok := w.WorldLaws[key]; ok {
			return v
		}
	}
	if w.Catalog.DefaultWorldLaws != nil {
		if v, ok := w.Catalog.DefaultWorldLaws[key]; ok {
			return v
		}
	}
	return 0
}

// EffectiveWorldLaws returns a merged snapshot of Catalog.DefaultWorldLaws overridden by World.WorldLaws,
// or nil when neither defines any laws.
func (w *World) EffectiveWorldLaws() map[string]int {
	if w == nil || (len(w.Catalog.DefaultWorldLaws) == 0 && len(w.WorldLaws) == 0) {
		return nil
	}
	out := make(map[string]int, len(w.Catalog.DefaultWorldLaws)+len(w.WorldLaws))
	for k, v := range w.Catalog.DefaultWorldLaws {
		out[k] = v
	}
	for k, v := range w.WorldLaws {
		out[k] = v
	}
	return out
}

// ApplyWorldLawChange validates and applies fundamental world law mutations (State C).
// Any attempt without explicit world-pack authorization (`authorized == true`) is strictly
// rejected with an `action_failed` event and non-nil error.
func (w *World) ApplyWorldLawChange(a *Actor, x Action, authorized bool, delta map[string]int) ([]Event, error) {
	return w.applyWorldLawChangeWithTx(nil, a, x, authorized, delta)
}

func (w *World) applyWorldLawChangeWithTx(tx *WorldTx, a *Actor, x Action, authorized bool, delta map[string]int) ([]Event, error) {
	if w == nil || a == nil {
		return nil, fmt.Errorf("world or actor is nil")
	}
	if !authorized {
		ev := w.recordWithTx(tx, a, x, "action_failed", map[string]any{
			"text": fmt.Sprintf("%s lacks world-pack authorization to rewrite fundamental world laws", a.Name),
		})
		return []Event{ev}, fmt.Errorf("world law rewrite not authorized")
	}
	if len(delta) == 0 {
		ev := w.recordWithTx(tx, a, x, "action_failed", map[string]any{
			"text": "world law rewrite requires at least one law change",
		})
		return []Event{ev}, fmt.Errorf("empty world law delta")
	}
	if w.WorldLaws == nil {
		w.WorldLaws = make(map[string]int, len(delta))
	}
	keys := make([]string, 0, len(delta))
	for k := range delta {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	prev := make(map[string]int, len(keys))
	updated := make(map[string]int, len(keys))
	for _, k := range keys {
		prev[k] = w.EffectiveWorldLaw(k)
		w.WorldLaws[k] = delta[k]
		updated[k] = delta[k]
	}
	if len(w.WorldLaws) == 0 {
		w.WorldLaws = nil
	}
	summary := x.Text
	if summary == "" {
		summary = fmt.Sprintf("%s rewrote fundamental world laws (%v)", a.Name, updated)
	}
	ev := w.recordWithTx(tx, a, x, "world_law_rewritten", map[string]any{
		"text":     summary,
		"previous": prev,
		"updated":  updated,
	})
	return []Event{ev}, nil
}

func (w *World) applyRewriteWorldLaw(tx *WorldTx, a *Actor, x Action) []Event {
	// Authorization requires explicit world-pack capability mapping for "rewrite_world_law" in Catalog.TagActions.
	tags := actorEffectiveTags(w.Catalog, *a)
	authorized := false
	for _, tag := range tags {
		for _, act := range w.Catalog.TagActions[tag] {
			if act == "rewrite_world_law" {
				authorized = true
				break
			}
		}
		if authorized {
			break
		}
	}
	var delta map[string]int
	if x.Item != "" {
		delta = map[string]int{x.Item: x.Quantity}
	}
	evs, _ := w.applyWorldLawChangeWithTx(tx, a, x, authorized, delta)
	return evs
}

// IsRecipeBlockedByWorldLaw reports whether active WorldLaws prohibit crafting recipe `r`.
func IsRecipeBlockedByWorldLaw(laws map[string]int, r Recipe) (bool, string) {
	if len(laws) == 0 {
		return false, ""
	}
	if laws["firearms_disabled"] > 0 {
		if r.RequiredTag == "firearm" ||
			strings.Contains(strings.ToLower(r.ID), "firearm") ||
			strings.Contains(strings.ToLower(r.OutputItem), "firearm") {
			return true, "firearms disabled by fundamental world law"
		}
	}
	return false, ""
}

// IsActionBlockedByWorldLaw reports whether active fundamental WorldLaws block action `x`.
func (w *World) IsActionBlockedByWorldLaw(a Actor, x Action) (bool, string) {
	if w == nil {
		return false, ""
	}
	if w.EffectiveWorldLaw("firearms_disabled") > 0 {
		firearm := false
		if x.Kind == "attack" {
			firearm = strings.Contains(strings.ToLower(x.Channel), "firearm") || strings.Contains(strings.ToLower(x.Item), "firearm")
		}
		if spec, ok := w.Catalog.ActionSpecs[x.Kind]; ok && spec.Attack != nil {
			firearm = firearm || ActorHasTag(spec.GrantedByTags, "firearm") || strings.Contains(strings.ToLower(x.Channel), "firearm")
		}
		if firearm {
			return true, "firearms disabled by fundamental world law"
		}
		if x.Kind == "craft" {
			if r, ok := w.Catalog.RecipeByID(x.RecipeID); ok {
				return IsRecipeBlockedByWorldLaw(w.EffectiveWorldLaws(), r)
			}
		}
	}
	return false, ""
}

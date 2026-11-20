package sim

import (
	"fmt"
	"sort"
)

// JusticeRevoltSpec defines declarative, genre-decoupled thresholds and parameters for
// magistrate court trials, scapegoat framing, oversight appeals, grievance escalation,
// popular uprisings, and state pacification/amnesty cycles.
type JusticeRevoltSpec struct {
	CorruptionFramingThreshold int    `json:"corruption_framing_threshold,omitempty"`
	TaxStrikeThreshold         int    `json:"tax_strike_threshold,omitempty"`
	GranaryRiotThreshold       int    `json:"granary_riot_threshold,omitempty"`
	RevoltThreshold            int    `json:"revolt_threshold,omitempty"`
	AmnestyGrievanceThreshold  int    `json:"amnesty_grievance_threshold,omitempty"`
	MinRevoltSharePermille     int    `json:"min_revolt_share_permille,omitempty"`
	MaxRevoltSharePermille     int    `json:"max_revolt_share_permille,omitempty"`
	TrialSentenceTicks         int64  `json:"trial_sentence_ticks,omitempty"`
	TaxStrikeDurationTicks     int64  `json:"tax_strike_duration_ticks,omitempty"`
	RevoltCooldownTicks        int64  `json:"revolt_cooldown_ticks,omitempty"`
	AmnestyCooldownTicks       int64  `json:"amnesty_cooldown_ticks,omitempty"`
	AppealCompensationCoins    int64  `json:"appeal_compensation_coins,omitempty"`
	LawCapability              string `json:"law_capability,omitempty"`
	OversightCapability        string `json:"oversight_capability,omitempty"`
	OversightTag               string `json:"oversight_tag,omitempty"`
	OutlawTag                  string `json:"outlaw_tag,omitempty"`
}

// DefaultJusticeRevoltSpec returns the baseline declarative justice and uprising parameters.
func DefaultJusticeRevoltSpec() JusticeRevoltSpec {
	return JusticeRevoltSpec{
		CorruptionFramingThreshold: 24,
		TaxStrikeThreshold:         58,
		GranaryRiotThreshold:       68,
		RevoltThreshold:            76,
		AmnestyGrievanceThreshold:  70,
		MinRevoltSharePermille:     20, // 2% of healthy free adults
		MaxRevoltSharePermille:     60, // 6% of healthy free adults
		TrialSentenceTicks:         144,
		TaxStrikeDurationTicks:     TicksPerDay,
		RevoltCooldownTicks:        2 * TicksPerDay,
		AmnestyCooldownTicks:       3 * TicksPerDay,
		AppealCompensationCoins:    12,
		LawCapability:              "arrest",
		OversightCapability:        "audit_corruption",
		OversightTag:               "censor",
		OutlawTag:                  "outlaw",
	}
}

// EffectiveJusticeRevoltSpec fills omitted zero-value fields with DefaultJusticeRevoltSpec defaults.
func EffectiveJusticeRevoltSpec(s JusticeRevoltSpec) JusticeRevoltSpec {
	def := DefaultJusticeRevoltSpec()
	if s.CorruptionFramingThreshold <= 0 {
		s.CorruptionFramingThreshold = def.CorruptionFramingThreshold
	}
	if s.TaxStrikeThreshold <= 0 {
		s.TaxStrikeThreshold = def.TaxStrikeThreshold
	}
	if s.GranaryRiotThreshold <= 0 {
		s.GranaryRiotThreshold = def.GranaryRiotThreshold
	}
	if s.RevoltThreshold <= 0 {
		s.RevoltThreshold = def.RevoltThreshold
	}
	if s.AmnestyGrievanceThreshold <= 0 {
		s.AmnestyGrievanceThreshold = def.AmnestyGrievanceThreshold
	}
	if s.MinRevoltSharePermille <= 0 {
		s.MinRevoltSharePermille = def.MinRevoltSharePermille
	}
	if s.MaxRevoltSharePermille <= 0 {
		s.MaxRevoltSharePermille = def.MaxRevoltSharePermille
	}
	if s.MaxRevoltSharePermille < s.MinRevoltSharePermille {
		s.MaxRevoltSharePermille = s.MinRevoltSharePermille
	}
	if s.TrialSentenceTicks <= 0 {
		s.TrialSentenceTicks = def.TrialSentenceTicks
	}
	if s.TaxStrikeDurationTicks <= 0 {
		s.TaxStrikeDurationTicks = def.TaxStrikeDurationTicks
	}
	if s.RevoltCooldownTicks <= 0 {
		s.RevoltCooldownTicks = def.RevoltCooldownTicks
	}
	if s.AmnestyCooldownTicks <= 0 {
		s.AmnestyCooldownTicks = def.AmnestyCooldownTicks
	}
	if s.AppealCompensationCoins <= 0 {
		s.AppealCompensationCoins = def.AppealCompensationCoins
	}
	if s.LawCapability == "" {
		s.LawCapability = def.LawCapability
	}
	if s.OversightCapability == "" {
		s.OversightCapability = def.OversightCapability
	}
	if s.OversightTag == "" {
		s.OversightTag = def.OversightTag
	}
	if s.OutlawTag == "" {
		s.OutlawTag = def.OutlawTag
	}
	return s
}

// CourtCase records a focal or macro magistrate court hearing, possible corrupt scapegoat framing,
// and any oversight/censorate appeal outcome.
type CourtCase struct {
	ID              int64  `json:"id"`
	Tick            int64  `json:"tick"`
	District        string `json:"district"`
	Kind            string `json:"kind"`
	MagistrateID    string `json:"magistrate_id"`
	SuspectID       string `json:"suspect_id"`
	RealCulpritID   string `json:"real_culprit_id,omitempty"`
	IsCohort        bool   `json:"is_cohort,omitempty"`
	Wrongful        bool   `json:"wrongful,omitempty"`
	JailedCount     int    `json:"jailed_count,omitempty"`
	JailedUntilTick int64  `json:"jailed_until_tick,omitempty"`
	BribeShielded   bool   `json:"bribe_shielded,omitempty"`
	Status          string `json:"status"` // "convicted", "overturned", "upheld"
	AppealByID      string `json:"appeal_by_id,omitempty"`
	ResolvedTick    int64  `json:"resolved_tick,omitempty"`
}

// ActiveRevolt tracks an erupted popular uprising cohort spawned from distressed civilians.
type ActiveRevolt struct {
	ID             string `json:"id"`
	District       string `json:"district"`
	RebelCohortID  string `json:"rebel_cohort_id"`
	SourceCohortID string `json:"source_cohort_id"`
	StartedTick    int64  `json:"started_tick"`
	PeakPop        int    `json:"peak_pop"`
	Status         string `json:"status"` // "active", "suppressed", "amnestied"
	ResolvedTick   int64  `json:"resolved_tick,omitempty"`
}

// JusticeRevoltState holds JSON-serializable court dockets, district grievance indices,
// tax strikes, and active or resolved uprising cycles.
type JusticeRevoltState struct {
	NextCaseID        int64              `json:"next_case_id,omitempty"`
	NextRevoltID      int64              `json:"next_revolt_id,omitempty"`
	CourtCases        []CourtCase        `json:"court_cases,omitempty"`
	DistrictGrievance map[string]int     `json:"district_grievance"`
	TaxStrikeUntil    map[string]int64   `json:"tax_strike_until"`
	ActiveRevolts     []ActiveRevolt     `json:"active_revolts,omitempty"`
	LastAmnestyTick   int64              `json:"last_amnesty_tick,omitempty"`
	LastTrialTick     int64              `json:"last_trial_tick,omitempty"`
	Spec              *JusticeRevoltSpec `json:"spec,omitempty"`
}

// NewJusticeRevoltState constructs an initialized JusticeRevoltState.
func NewJusticeRevoltState() *JusticeRevoltState {
	return &JusticeRevoltState{
		NextCaseID:        1,
		NextRevoltID:      1,
		DistrictGrievance: map[string]int{},
		TaxStrikeUntil:    map[string]int64{},
	}
}

// CloneJusticeRevoltState returns a deep copy of s (nil-safe).
func CloneJusticeRevoltState(s *JusticeRevoltState) *JusticeRevoltState {
	if s == nil {
		return nil
	}
	out := &JusticeRevoltState{
		NextCaseID:      s.NextCaseID,
		NextRevoltID:    s.NextRevoltID,
		LastAmnestyTick: s.LastAmnestyTick,
		LastTrialTick:   s.LastTrialTick,
	}
	if len(s.CourtCases) > 0 {
		out.CourtCases = append([]CourtCase(nil), s.CourtCases...)
	}
	if s.DistrictGrievance != nil {
		out.DistrictGrievance = cloneIntMap(s.DistrictGrievance)
	}
	if s.TaxStrikeUntil != nil {
		out.TaxStrikeUntil = make(map[string]int64, len(s.TaxStrikeUntil))
		for k, v := range s.TaxStrikeUntil {
			out.TaxStrikeUntil[k] = v
		}
	}
	if len(s.ActiveRevolts) > 0 {
		out.ActiveRevolts = append([]ActiveRevolt(nil), s.ActiveRevolts...)
	}
	if s.Spec != nil {
		sp := *s.Spec
		out.Spec = &sp
	}
	return out
}

func (s *JusticeRevoltState) ensureInitialized() {
	if s.NextCaseID <= 0 {
		s.NextCaseID = 1
	}
	if s.NextRevoltID <= 0 {
		s.NextRevoltID = 1
	}
	if s.DistrictGrievance == nil {
		s.DistrictGrievance = map[string]int{}
	}
	if s.TaxStrikeUntil == nil {
		s.TaxStrikeUntil = map[string]int64{}
	}
}

func (s *JusticeRevoltState) addGrievance(districtID string, delta int) int {
	if s == nil || districtID == "" {
		return 0
	}
	s.ensureInitialized()
	g := s.DistrictGrievance[districtID] + delta
	if g < 0 {
		g = 0
	}
	if g > 100 {
		g = 100
	}
	s.DistrictGrievance[districtID] = g
	return g
}

// Grievance returns the current grievance index (0..100) for districtID.
func (s *JusticeRevoltState) Grievance(districtID string) int {
	if s == nil || s.DistrictGrievance == nil {
		return 0
	}
	return s.DistrictGrievance[districtID]
}

// IsDistrictTaxStriking reports whether a tax strike is active in districtID at tick.
func (s *JusticeRevoltState) IsDistrictTaxStriking(districtID string, tick int64) bool {
	if s == nil || s.TaxStrikeUntil == nil {
		return false
	}
	return tick < s.TaxStrikeUntil[districtID]
}

func (w *World) effectiveJusticeSpec(jr *JusticeRevoltState) JusticeRevoltSpec {
	if jr != nil && jr.Spec != nil {
		return EffectiveJusticeRevoltSpec(*jr.Spec)
	}
	return DefaultJusticeRevoltSpec()
}

func (w *World) actorCanEnforceLaw(a Actor, spec JusticeRevoltSpec) bool {
	idTags := a.IdentityTags()
	caps := w.Catalog.Capabilities(a)
	if caps[spec.LawCapability] || caps["frame_suspect"] || w.Catalog.IsEnforcementTags(idTags) {
		return true
	}
	return w.Catalog.HasCoercivePower(idTags) && w.Catalog.HasPublicTrustDuty(idTags)
}

func (w *World) actorCanAppeal(a Actor, spec JusticeRevoltSpec) bool {
	idTags := a.IdentityTags()
	caps := w.Catalog.Capabilities(a)
	if ActorHasTag(idTags, spec.OversightTag) || w.Catalog.IsOversightTags(idTags) || caps[spec.OversightCapability] || caps["file_appeal"] {
		return true
	}
	return w.Catalog.HasPublicTrustDuty(idTags)
}

func (w *World) findCohortByID(id string) *PopulationCohort {
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == id {
			return &w.Cohorts[i]
		}
	}
	return nil
}

func (w *World) isOutlawCohort(c *PopulationCohort, spec JusticeRevoltSpec) bool {
	if c == nil {
		return false
	}
	prof, ok := w.Catalog.ProfessionByID(c.Profession)
	if ok && (ActorHasTag(prof.Grants, spec.OutlawTag) || w.Catalog.IsOutlawTags(prof.Grants)) {
		return true
	}
	return false
}

func (w *World) isOversightCohort(c *PopulationCohort, spec JusticeRevoltSpec) bool {
	if c == nil || c.Population <= 0 || w.isOutlawCohort(c, spec) {
		return false
	}
	prof, ok := w.Catalog.ProfessionByID(c.Profession)
	if !ok {
		return false
	}
	if ActorHasTag(prof.Grants, spec.OversightTag) {
		return true
	}
	caps := w.Catalog.Capabilities(Actor{Tags: prof.Grants})
	if caps[spec.OversightCapability] && c.IllicitWealth == 0 {
		return true
	}
	return w.Catalog.HasPublicTrustDuty([]string{c.Profession}) && !w.isLawCollectorCohort(c)
}

func (w *World) isGarrisonCohort(c *PopulationCohort, spec JusticeRevoltSpec) bool {
	if c == nil || c.Population <= 0 || w.isOutlawCohort(c, spec) {
		return false
	}
	if w.isLawCollectorCohort(c) {
		return true
	}
	return w.Catalog.HasCoercivePower([]string{c.Profession}) && w.isCivicServiceProfession(c.Profession)
}

func (jr *JusticeRevoltState) isActiveRebelCohort(cohortID string) bool {
	if jr == nil {
		return false
	}
	for _, r := range jr.ActiveRevolts {
		if r.RebelCohortID == cohortID && r.Status == "active" {
			return true
		}
	}
	return false
}

func (jr *JusticeRevoltState) hasDistrictActiveOrRecentRevolt(districtID string, tick, cooldown int64) bool {
	if jr == nil {
		return false
	}
	for _, r := range jr.ActiveRevolts {
		if r.District != districtID {
			continue
		}
		if r.Status == "active" {
			return true
		}
		if r.ResolvedTick > 0 && tick-r.ResolvedTick < cooldown {
			return true
		}
	}
	return false
}

func (w *World) resolveRebelProfession(spec JusticeRevoltSpec, fallback string) (profID, factionID string) {
	// Prefer Tier-1 outlaw profession with coercive power, then any outlaw profession, then non-civic coercive.
	for _, p := range w.Catalog.Professions {
		if ActorHasTag(p.Grants, spec.OutlawTag) && w.Catalog.HasCoercivePower([]string{p.ID}) && p.Tier <= 1 {
			return p.ID, p.Guild
		}
	}
	for _, p := range w.Catalog.Professions {
		if ActorHasTag(p.Grants, spec.OutlawTag) {
			return p.ID, p.Guild
		}
	}
	for _, p := range w.Catalog.Professions {
		if w.Catalog.HasCoercivePower([]string{p.ID}) && !w.isCivicServiceProfession(p.ID) {
			return p.ID, p.Guild
		}
	}
	return fallback, ""
}

func (w *World) resolveEdibleItemID() string {
	if spec, ok := w.Catalog.Items["food"]; ok && spec.Edible {
		return "food"
	}
	var edibles []string
	for id, spec := range w.Catalog.Items {
		if spec.Edible {
			edibles = append(edibles, id)
		}
	}
	sort.Strings(edibles)
	if len(edibles) > 0 {
		return edibles[0]
	}
	return ""
}

// ApplyJusticeAction handles focal-actor actions ("frame_suspect", "file_appeal") for court framing
// and oversight/censorate exoneration appeals while maintaining exact jail and coin conservation.
func (w *World) ApplyJusticeAction(jr *JusticeRevoltState, actorID string, x Action) ([]Event, bool) {
	if x.Kind != "frame_suspect" && x.Kind != "file_appeal" {
		return nil, false
	}
	if jr == nil {
		return nil, true
	}
	jr.ensureInitialized()
	spec := w.effectiveJusticeSpec(jr)

	a := w.actor(actorID)
	if a == nil {
		return nil, true
	}
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Dead {
		return fail("actor is dead"), true
	}
	if w.Tick < a.JailedUntil {
		return fail("actor is imprisoned"), true
	}
	if a.Task != nil {
		return fail("actor is already busy with " + a.Task.Action.Kind), true
	}

	b := w.actor(x.TargetID)

	switch x.Kind {
	case "frame_suspect":
		if !w.Catalog.EffectiveEnvironment().Institutions.EnforcementEnabled() {
			return fail("enforcement framing is disabled in this world environment"), true
		}
		if !w.actorCanEnforceLaw(*a, spec) {
			return fail("actor lacks enforcement authority"), true
		}
		if b == nil || b.ID == a.ID || !SamePlace(*a, *b) {
			return fail("target suspect is not nearby"), true
		}
		if w.Tick < b.JailedUntil {
			return fail("target is already imprisoned"), true
		}
		bTags := b.IdentityTags()
		if ActorHasTag(bTags, spec.OutlawTag) || w.Catalog.IsOutlawTags(bTags) {
			return fail("cannot scapegoat-frame an outlaw"), true
		}
		localCorr := 0
		bribeShielded := false
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[a.Location]; ok {
				localCorr = ds.Corruption
				bribeShielded = ds.BribeShieldTicks > 0
			}
		}
		if localCorr < spec.CorruptionFramingThreshold && !bribeShielded && a.IllicitGain <= 0 {
			return fail("local corruption is too low for scapegoat framing"), true
		}
		if a.Energy < 4 {
			return fail("insufficient energy to conduct court framing"), true
		}
		a.Energy -= 4

		observers := append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)
		b.Task = nil
		b.InsideHouse = ""
		b.JailedUntil = w.Tick + spec.TrialSentenceTicks
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		b.Trust[a.ID] -= 5

		w.addDistrictCorruption(a.Location, 4)
		grievance := jr.addGrievance(a.Location, 14)

		caseID := jr.NextCaseID
		jr.NextCaseID++
		jr.CourtCases = append(jr.CourtCases, CourtCase{
			ID:              caseID,
			Tick:            w.Tick,
			District:        a.Location,
			Kind:            "framed_scapegoat",
			MagistrateID:    a.ID,
			SuspectID:       b.ID,
			IsCohort:        false,
			Wrongful:        true,
			JailedUntilTick: b.JailedUntil,
			BribeShielded:   bribeShielded,
			Status:          "convicted",
		})
		jr.LastTrialTick = w.Tick

		ev := w.record(a, x, "wrongful_conviction", map[string]any{
			"text":           fmt.Sprintf("%s framed innocent %s in %s court (case #%d, jailed until tick %d, grievance=%d)", a.Name, b.Name, a.Location, caseID, b.JailedUntil, grievance),
			"case_id":        caseID,
			"district":       a.Location,
			"magistrate_id":  a.ID,
			"suspect_id":     b.ID,
			"jailed":         1,
			"jailed_until":   b.JailedUntil,
			"bribe_shielded": bribeShielded,
			"grievance":      grievance,
			"observation": Observation{
				Kind:      "wrongful_conviction",
				SubjectID: a.ID,
				ObjectID:  b.ID,
				CaseID:    caseID,
				Location:  a.Location,
				Quantity:  1,
			},
		}, observers...)
		return []Event{ev}, true

	case "file_appeal":
		if !w.actorCanAppeal(*a, spec) {
			return fail("actor lacks oversight or public-trust standing to file appeal"), true
		}
		if a.IllicitGain > 0 {
			return fail("corrupt official cannot adjudicate appeal"), true
		}
		if a.Energy < 4 {
			return fail("insufficient energy to file appeal"), true
		}
		if x.TargetID == "" && x.BasisEventID == 0 {
			return fail("appeal must specify a known wrongful-conviction target or basis event"), true
		}
		leads := ActorKnownWrongfulLeads(*a)
		if x.BasisEventID != 0 {
			var filtered []KnownWrongfulLead
			for _, lead := range leads {
				if lead.BasisEventID == x.BasisEventID {
					filtered = append(filtered, lead)
				}
			}
			leads = filtered
		}
		if len(leads) == 0 {
			return fail("actor has no witnessed or reported knowledge of this wrongful conviction"), true
		}

		// Locate matching open wrongful conviction that the actor actually knows about AND is in the actor's current district.
		caseIdx := -1
		var matchedLead KnownWrongfulLead
		remoteDistrict := ""
		for i := len(jr.CourtCases) - 1; i >= 0; i-- {
			c := jr.CourtCases[i]
			if c.Status != "convicted" || !c.Wrongful {
				continue
			}
			if x.TargetID != "" && c.SuspectID != x.TargetID && c.MagistrateID != x.TargetID {
				continue
			}
			lead, ok := matchKnownLeadToCourtCase(leads, c)
			if !ok {
				continue
			}
			if c.District != a.Location {
				if remoteDistrict == "" {
					remoteDistrict = c.District
				}
				continue
			}
			caseIdx = i
			matchedLead = lead
			break
		}
		if caseIdx == -1 {
			if remoteDistrict != "" {
				return fail("must travel to " + remoteDistrict + " court to file appeal"), true
			}
			return fail("no known open wrongful conviction found in current district"), true
		}

		cc := &jr.CourtCases[caseIdx]
		if !w.courtPartiesPresent(*cc) {
			return fail("court parties must be present in the case district for release and enforcement"), true
		}
		a.Energy -= 4
		if x.TargetID == "" {
			x.TargetID = cc.SuspectID
		}
		if x.BasisEventID == 0 {
			x.BasisEventID = matchedLead.BasisEventID
		}
		cc.Status = "overturned"
		cc.AppealByID = a.ID
		cc.ResolvedTick = w.Tick

		released := 0
		jailedFramer := 0
		var reliefCoins int64
		var confiscatedCoins int64
		var observers []*Actor

		if !cc.IsCohort {
			if victim := w.actor(cc.SuspectID); victim != nil && !victim.Dead {
				if victim.JailedUntil > w.Tick {
					victim.JailedUntil = 0
					released = 1
				}
				reliefCoins = w.debitTreasury(spec.AppealCompensationCoins, "relief")
				victim.Money += int(reliefCoins)
				if victim.Trust == nil {
					victim.Trust = map[string]int{}
				}
				victim.Trust[a.ID] += 4
				observers = append(observers, victim)
			}
			if mag := w.actor(cc.MagistrateID); mag != nil && !mag.Dead && mag.ID != a.ID {
				if mag.IllicitGain > 0 && mag.Money > 0 {
					seize := minInt(mag.IllicitGain, mag.Money)
					mag.Money -= seize
					mag.IllicitGain -= seize
					confiscatedCoins = int64(seize)
					w.creditTreasury(confiscatedCoins, "confiscated")
				}
				if mag.JailedUntil <= w.Tick {
					mag.Task = nil
					mag.InsideHouse = ""
					mag.JailedUntil = w.Tick + spec.TrialSentenceTicks
					jailedFramer = 1
				}
				observers = append(observers, mag)
			}
		} else {
			if victimCohort := w.findCohortByID(cc.SuspectID); victimCohort != nil && victimCohort.Population > 0 {
				if cc.JailedCount > 0 && victimCohort.Jailed > 0 {
					released = minInt(victimCohort.Jailed, cc.JailedCount)
					victimCohort.Jailed -= released
				}
				reliefCoins = w.debitTreasury(int64(maxInt(1, released))*3, "relief")
				victimCohort.MoneyPool += reliefCoins
			}
			if magCohort := w.findCohortByID(cc.MagistrateID); magCohort != nil && magCohort.EffectivePopulation() >= 1 {
				if magCohort.IllicitWealth > 0 && magCohort.MoneyPool > 0 {
					seize := minInt64(magCohort.MoneyPool, minInt64(magCohort.IllicitWealth, int64(maxInt(1, released))*4))
					if seize > 0 {
						magCohort.MoneyPool -= seize
						magCohort.IllicitWealth -= seize
						confiscatedCoins = seize
						w.creditTreasury(seize, "confiscated")
					}
				}
				_, _, jailedFramer = w.applyCohortCasualtiesWithCause(magCohort, 0, 0, minInt(4, maxInt(1, released/2)), "combat")
			}
		}

		w.addDistrictCorruption(cc.District, -10)
		grievance := jr.addGrievance(cc.District, -14)

		ev := w.record(a, x, "conviction_overturned", map[string]any{
			"text":              fmt.Sprintf("%s overturned wrongful conviction #%d in %s (%d exonerated, %d corrupt framers jailed, %d relief paid, %d confiscated)", a.Name, cc.ID, cc.District, released, jailedFramer, reliefCoins, confiscatedCoins),
			"case_id":           cc.ID,
			"district":          cc.District,
			"suspect_id":        cc.SuspectID,
			"magistrate_id":     cc.MagistrateID,
			"released":          released,
			"jailed":            jailedFramer,
			"relief_coins":      reliefCoins,
			"confiscated_coins": confiscatedCoins,
			"grievance":         grievance,
		}, observers...)
		events := []Event{ev}
		if appealProf := w.Catalog.BestProfessionForCapability(*a, "file_appeal"); appealProf != "" {
			events = append(events, w.grantProfessionXP(a, x, appealProf, 8, 3)...)
		} else if auditProf := w.Catalog.BestProfessionForCapability(*a, spec.OversightCapability); auditProf != "" {
			events = append(events, w.grantProfessionXP(a, x, auditProf, 8, 3)...)
		}
		return events, true
	}

	return nil, false
}

// StepJusticeAndRevolt runs macro enforcement court trials, scapegoat framing, oversight appeals,
// district grievance tracking, tax strikes, granary riots, popular uprisings, and state
// pacification/amnesty reintegration cycles while preserving all 6 conservation ledgers.
func (w *World) StepJusticeAndRevolt(jr *JusticeRevoltState) []Event {
	if jr == nil {
		return nil
	}
	jr.ensureInitialized()
	clock := ClockAt(w.Tick)
	if clock.Minute != 0 {
		return nil
	}
	env := w.Catalog.EffectiveEnvironment()
	spec := w.effectiveJusticeSpec(jr)
	var events []Event

	// 1. 15:00 Afternoon Enforcement Docket (fair trials vs. corrupt scapegoat framing)
	if clock.Hour == 15 && env.Institutions.EnforcementEnabled() {
		events = append(events, w.stepMacroCourtTrials(jr, spec)...)
	}

	// 2. 17:00 Oversight Appeals Docket
	if clock.Hour == 17 && env.Institutions.OversightEnabled() {
		events = append(events, w.stepMacroAppeals(jr, spec)...)
	}

	// 3. 18:00 Daily District Grievance Evaluation & Escalation Ladder
	if clock.Hour == 18 && env.Institutions.RevoltEnabled() {
		events = append(events, w.stepGrievanceAndEscalation(jr, spec)...)
	}

	// 4. 20:00 Pacification, Suppression & Amnesty / Reintegration Cycle
	if clock.Hour == 20 && env.Institutions.RevoltEnabled() {
		events = append(events, w.stepPacificationAndAmnesty(jr, spec)...)
		jr.pruneResolvedHistory(w.Tick)
	}

	return events
}

func (jr *JusticeRevoltState) pruneResolvedHistory(tick int64) {
	retention := int64(14 * TicksPerDay)
	if len(jr.CourtCases) > 64 {
		kept := jr.CourtCases[:0]
		for _, c := range jr.CourtCases {
			if c.Status == "convicted" || tick-c.ResolvedTick < retention {
				kept = append(kept, c)
			}
		}
		jr.CourtCases = kept
	}
	if len(jr.ActiveRevolts) > 32 {
		kept := jr.ActiveRevolts[:0]
		for _, r := range jr.ActiveRevolts {
			if r.Status == "active" || tick-r.ResolvedTick < retention {
				kept = append(kept, r)
			}
		}
		jr.ActiveRevolts = kept
	}
}

func (w *World) sortedDistrictIDs() []string {
	seen := map[string]bool{}
	var ids []string
	for _, d := range w.Map.Districts {
		if d.ID != "" && !seen[d.ID] {
			seen[d.ID] = true
			ids = append(ids, d.ID)
		}
	}
	for id := range w.DistrictStates {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (w *World) stepMacroCourtTrials(jr *JusticeRevoltState, spec JusticeRevoltSpec) []Event {
	if len(w.Cohorts) == 0 {
		return nil
	}
	districts := w.sortedDistrictIDs()
	if len(districts) == 0 {
		return nil
	}

	// Select up to 2 districts per day for court hearings (prioritizing high-corruption/bribe-shielded districts).
	type candDistrict struct {
		id       string
		priority int
	}
	var cands []candDistrict
	for _, dID := range districts {
		prio := 0
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[dID]; ok {
				prio = ds.Corruption
				if ds.BribeShieldTicks > 0 {
					prio += 25
				}
			}
		}
		if g := jr.DistrictGrievance[dID]; g > 0 {
			prio += g / 4
		}
		cands = append(cands, candDistrict{id: dID, priority: prio})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].priority != cands[j].priority {
			return cands[i].priority > cands[j].priority
		}
		return cands[i].id < cands[j].id
	})

	// Pick top corrupt/active district plus one rotating docket district.
	selected := []string{cands[0].id}
	if len(districts) > 1 {
		rotIdx := int(w.Tick/TicksPerDay) % len(districts)
		if districts[rotIdx] != selected[0] {
			selected = append(selected, districts[rotIdx])
		}
	}

	var events []Event
	for _, dID := range selected {
		mag := w.presentLawCollector(dID)
		if mag == nil {
			continue
		}

		// Court participants must be physically present in this jurisdiction.
		var civ *PopulationCohort
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.District != dID || c.EffectivePopulation() < 4 {
				continue
			}
			if w.isOutlawCohort(c, spec) || w.isCivicServiceProfession(c.Profession) || jr.isActiveRebelCohort(c.ID) {
				continue
			}
			if civ == nil || c.EffectivePopulation() > civ.EffectivePopulation() {
				civ = c
			}
		}

		// Find an outlaw cohort already present at the court's district.
		var outlaw *PopulationCohort
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.District != dID || c.EffectivePopulation() < 3 || !w.isOutlawCohort(c, spec) {
				continue
			}
			if outlaw == nil || (c.District == dID && outlaw.District != dID) || (c.District == outlaw.District && c.EffectivePopulation() > outlaw.EffectivePopulation()) {
				outlaw = c
			}
		}

		localCorr := 0
		bribeShielded := false
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[dID]; ok {
				localCorr = ds.Corruption
				bribeShielded = ds.BribeShieldTicks > 0
			}
		}

		isCorruptFraming := (localCorr >= spec.CorruptionFramingThreshold || bribeShielded) && civ != nil && civ.EffectivePopulation() >= 4
		if isCorruptFraming {
			realCulpritID := ""
			if outlaw != nil {
				realCulpritID = outlaw.ID
			}
			jailReq := maxInt(1, minInt(12, civ.EffectivePopulation()/5000+2))
			_, _, jailed := w.applyCohortCasualtiesWithCause(civ, 0, 0, jailReq, "combat")

			caseID := jr.NextCaseID
			jr.NextCaseID++
			jr.CourtCases = append(jr.CourtCases, CourtCase{
				ID:            caseID,
				Tick:          w.Tick,
				District:      dID,
				Kind:          "framed_scapegoat",
				MagistrateID:  mag.ID,
				SuspectID:     civ.ID,
				RealCulpritID: realCulpritID,
				IsCohort:      true,
				Wrongful:      true,
				JailedCount:   jailed,
				BribeShielded: bribeShielded,
				Status:        "convicted",
			})
			jr.LastTrialTick = w.Tick

			trialEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  mag.ID,
				TargetID: civ.ID,
				Kind:     "court_trial_held",
				Action:   Action{Kind: spec.LawCapability, TargetID: civ.ID, Location: dID},
				Payload: map[string]any{
					"text":             fmt.Sprintf("Court docket #%d in %s: %s shielded culprits (%s) under corruption=%d and indicted %s", caseID, dID, mag.Name, realCulpritID, localCorr, civ.Name),
					"case_id":          caseID,
					"district":         dID,
					"magistrate":       mag.ID,
					"suspect_cohort":   civ.ID,
					"wrongful":         true,
					"jailed":           0,
					"corruption_index": localCorr,
				},
			}
			w.NextID++
			events = append(events, trialEv)

			if jailed > 0 {
				grievance := jr.addGrievance(dID, 10)
				w.addDistrictCorruption(dID, 2)
				frameEv := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  mag.ID,
					TargetID: civ.ID,
					Kind:     "wrongful_conviction",
					Action:   Action{Kind: "frame_suspect", TargetID: civ.ID, Location: dID},
					Payload: map[string]any{
						"text":           fmt.Sprintf("Wrongful conviction #%d in %s: %d innocent %s members jailed as scapegoats (real culprit: %s, grievance=%d)", caseID, dID, jailed, civ.Name, realCulpritID, grievance),
						"case_id":        caseID,
						"district":       dID,
						"magistrate_id":  mag.ID,
						"suspect_id":     civ.ID,
						"real_culprit":   realCulpritID,
						"jailed":         jailed,
						"bribe_shielded": bribeShielded,
						"grievance":      grievance,
					},
				}
				w.NextID++
				events = append(events, frameEv)
			}
			continue
		}

		if outlaw != nil && outlaw.EffectivePopulation() >= 3 {
			jailReq := maxInt(1, minInt(8, outlaw.EffectivePopulation()/40+1))
			_, _, jailed := w.applyCohortCasualtiesWithCause(outlaw, 0, 0, jailReq, "combat")

			caseID := jr.NextCaseID
			jr.NextCaseID++
			jr.CourtCases = append(jr.CourtCases, CourtCase{
				ID:           caseID,
				Tick:         w.Tick,
				District:     dID,
				Kind:         "criminal_trial",
				MagistrateID: mag.ID,
				SuspectID:    outlaw.ID,
				IsCohort:     true,
				Wrongful:     false,
				JailedCount:  jailed,
				Status:       "convicted",
			})
			jr.LastTrialTick = w.Tick
			jr.addGrievance(dID, -2)

			trialEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  mag.ID,
				TargetID: outlaw.ID,
				Kind:     "court_trial_held",
				Action:   Action{Kind: spec.LawCapability, TargetID: outlaw.ID, Location: dID},
				Payload: map[string]any{
					"text":             fmt.Sprintf("Court docket #%d in %s: %s convicted %d culprits from %s", caseID, dID, mag.Name, jailed, outlaw.Name),
					"case_id":          caseID,
					"district":         dID,
					"magistrate":       mag.ID,
					"suspect_cohort":   outlaw.ID,
					"wrongful":         false,
					"jailed":           jailed,
					"corruption_index": localCorr,
				},
			}
			w.NextID++
			events = append(events, trialEv)
		}
	}
	return events
}

func (w *World) courtPartiesPresent(cc CourtCase) bool {
	if cc.IsCohort {
		victim, magistrate := w.findCohortByID(cc.SuspectID), w.findCohortByID(cc.MagistrateID)
		return victim != nil && magistrate != nil && victim.Population > 0 && magistrate.Population > 0 && victim.District == cc.District && magistrate.District == cc.District
	}
	victim, magistrate := w.actor(cc.SuspectID), w.actor(cc.MagistrateID)
	return victim != nil && magistrate != nil && !victim.Dead && !magistrate.Dead && !victim.InTransit() && !magistrate.InTransit() && victim.Location == cc.District && magistrate.Location == cc.District
}

func (w *World) stepMacroAppeals(jr *JusticeRevoltState, spec JusticeRevoltSpec) []Event {
	var events []Event
	overturnedCount := 0
	for i := range jr.CourtCases {
		if overturnedCount >= 2 {
			break
		}
		cc := &jr.CourtCases[i]
		if cc.Status != "convicted" || !cc.Wrongful || !cc.IsCohort || !w.courtPartiesPresent(*cc) {
			continue
		}
		var censorCohort *PopulationCohort
		for j := range w.Cohorts {
			c := &w.Cohorts[j]
			if w.cohortHasJurisdiction(c, cc.District) && w.isOversightCohort(c, spec) && c.EffectivePopulation() >= 2 {
				if censorCohort == nil || c.Tier > censorCohort.Tier || c.Population > censorCohort.Population {
					censorCohort = c
				}
			}
		}
		if censorCohort == nil {
			continue
		}
		censorID, censorName := censorCohort.ID, censorCohort.Name

		cc.Status = "overturned"
		cc.AppealByID = censorID
		cc.ResolvedTick = w.Tick
		overturnedCount++

		released := 0
		jailedFramer := 0
		var reliefCoins int64
		var confiscatedCoins int64

		if !cc.IsCohort {
			if victim := w.actor(cc.SuspectID); victim != nil && !victim.Dead {
				if victim.JailedUntil > w.Tick {
					victim.JailedUntil = 0
					released = 1
				}
				reliefCoins = w.debitTreasury(spec.AppealCompensationCoins, "relief")
				victim.Money += int(reliefCoins)
			}
			if mag := w.actor(cc.MagistrateID); mag != nil && !mag.Dead {
				if mag.IllicitGain > 0 && mag.Money > 0 {
					seize := minInt(mag.IllicitGain, mag.Money)
					mag.Money -= seize
					mag.IllicitGain -= seize
					confiscatedCoins = int64(seize)
					w.creditTreasury(confiscatedCoins, "confiscated")
				}
				if mag.JailedUntil <= w.Tick {
					mag.Task = nil
					mag.InsideHouse = ""
					mag.JailedUntil = w.Tick + spec.TrialSentenceTicks
					jailedFramer = 1
				}
			}
		} else {
			if victimCohort := w.findCohortByID(cc.SuspectID); victimCohort != nil && victimCohort.Population > 0 {
				if cc.JailedCount > 0 && victimCohort.Jailed > 0 {
					released = minInt(victimCohort.Jailed, cc.JailedCount)
					victimCohort.Jailed -= released
				}
				reliefCoins = w.debitTreasury(int64(maxInt(1, released))*3, "relief")
				victimCohort.MoneyPool += reliefCoins
			}
			if magCohort := w.findCohortByID(cc.MagistrateID); magCohort != nil && magCohort.EffectivePopulation() >= 1 {
				if magCohort.IllicitWealth > 0 && magCohort.MoneyPool > 0 {
					seize := minInt64(magCohort.MoneyPool, minInt64(magCohort.IllicitWealth, int64(maxInt(1, released))*4))
					if seize > 0 {
						magCohort.MoneyPool -= seize
						magCohort.IllicitWealth -= seize
						confiscatedCoins = seize
						w.creditTreasury(seize, "confiscated")
					}
				}
				_, _, jailedFramer = w.applyCohortCasualtiesWithCause(magCohort, 0, 0, minInt(4, maxInt(1, released/2)), "combat")
			}
		}

		w.addDistrictCorruption(cc.District, -8)
		grievance := jr.addGrievance(cc.District, -12)
		if censorCohort != nil {
			events = append(events, w.addCohortXP(censorCohort, 4, 2)...)
		}

		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  censorID,
			TargetID: cc.SuspectID,
			Kind:     "conviction_overturned",
			Action:   Action{Kind: "file_appeal", TargetID: cc.SuspectID, Location: cc.District},
			Payload: map[string]any{
				"text":              fmt.Sprintf("%s overturned wrongful conviction #%d in %s (%d exonerated, %d corrupt framers jailed, %d relief paid, %d confiscated)", censorName, cc.ID, cc.District, released, jailedFramer, reliefCoins, confiscatedCoins),
				"case_id":           cc.ID,
				"district":          cc.District,
				"suspect_id":        cc.SuspectID,
				"magistrate_id":     cc.MagistrateID,
				"released":          released,
				"jailed":            jailedFramer,
				"relief_coins":      reliefCoins,
				"confiscated_coins": confiscatedCoins,
				"grievance":         grievance,
			},
		}
		w.NextID++
		events = append(events, ev)
	}
	return events
}

func (w *World) stepGrievanceAndEscalation(jr *JusticeRevoltState, spec JusticeRevoltSpec) []Event {
	districts := w.sortedDistrictIDs()
	if len(districts) == 0 {
		return nil
	}

	var events []Event
	foodItem := w.resolveEdibleItemID()

	for _, dID := range districts {
		// Gather district demographic & distress signals.
		totalPop := 0
		weightedHunger := 0
		lowCashPop := 0
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.District != dID || c.Population <= 0 || w.isOutlawCohort(c, spec) || jr.isActiveRebelCohort(c.ID) {
				continue
			}
			totalPop += c.Population
			weightedHunger += c.AvgHunger * c.Population
			if c.MoneyPool < int64(c.Population)*4 {
				lowCashPop += c.Population
			}
		}
		avgHunger := 0
		if totalPop > 0 {
			avgHunger = weightedHunger / totalPop
		} else {
			// Fallback to focal actors in districts without cohorts.
			actorCount := 0
			actorHunger := 0
			for i := range w.Actors {
				a := &w.Actors[i]
				if !a.Dead && a.Location == dID {
					actorCount++
					actorHunger += a.Hunger
				}
			}
			if actorCount > 0 {
				avgHunger = actorHunger / actorCount
			}
		}

		corr := 0
		racket := 0
		morale := 0
		disasters := 0
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[dID]; ok {
				corr = ds.Corruption
				racket = ds.RacketPressure
				morale = ds.MoraleBonus
				disasters = len(ds.ActiveDisasters)
			}
		}

		openWrongful := 0
		for _, cc := range jr.CourtCases {
			if cc.District == dID && cc.Status == "convicted" && cc.Wrongful {
				openWrongful++
			}
		}

		delta := 0
		if corr >= 35 {
			delta += 6
		} else if corr >= spec.CorruptionFramingThreshold {
			delta += 3
		} else if corr >= 15 {
			delta += 1
		} else if corr <= 10 {
			delta -= 3
		}

		if racket >= 25 {
			delta += 3
		} else if racket >= 15 {
			delta += 1
		}
		if corr >= 20 && totalPop > 0 && lowCashPop*3 >= totalPop {
			delta += 3
		}

		if avgHunger >= 4 {
			delta += 8
		} else if avgHunger >= 3 {
			delta += 4
		} else if avgHunger <= 1 {
			delta -= 3
		}

		if disasters > 0 {
			delta += minInt(8, disasters*4)
		}
		if openWrongful > 0 {
			delta += minInt(6, openWrongful*3)
		}
		if morale > 0 {
			delta -= minInt(5, maxInt(1, morale/2))
		}
		if corr < 18 && avgHunger < 3 && disasters == 0 && openWrongful == 0 {
			delta -= 3
		}

		g := jr.addGrievance(dID, delta)

		// Escalation rung 1: Merchant/Guild Tax Strike
		if g >= spec.TaxStrikeThreshold && w.Tick >= jr.TaxStrikeUntil[dID] {
			until := w.Tick + spec.TaxStrikeDurationTicks
			jr.TaxStrikeUntil[dID] = until
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "tax_strike",
				Action:  Action{Kind: "wait", Location: dID},
				Payload: map[string]any{
					"text":       fmt.Sprintf("Guilds and households in %s declared a tax strike (grievance=%d, until tick %d)", dID, g, until),
					"district":   dID,
					"grievance":  g,
					"until_tick": until,
				},
			}
			w.NextID++
			events = append(events, ev)
		}

		// Escalation rung 2: Granary Riot (hungry crowds storm surplus food stocks)
		if g >= spec.GranaryRiotThreshold && avgHunger >= 3 && foodItem != "" && len(w.Cohorts) > 0 {
			events = append(events, w.triggerGranaryRiot(jr, spec, dID, g, foodItem)...)
		}

		// Escalation rung 3: Popular Uprising / Rebel Conversion ("官逼民反")
		if g >= spec.RevoltThreshold && len(w.Cohorts) > 0 &&
			!jr.hasDistrictActiveOrRecentRevolt(dID, w.Tick, spec.RevoltCooldownTicks) {
			events = append(events, w.triggerPopularUprising(jr, spec, dID, g)...)
		}
	}

	return events
}

func (w *World) triggerGranaryRiot(jr *JusticeRevoltState, spec JusticeRevoltSpec, dID string, grievance int, foodItem string) []Event {
	var hungryIdx = -1
	var donorIdx = -1
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 || w.isOutlawCohort(c, spec) || jr.isActiveRebelCohort(c.ID) {
			continue
		}
		if c.District == dID && c.AvgHunger >= 2 && c.EffectivePopulation() >= 3 {
			if hungryIdx == -1 || c.AvgHunger > w.Cohorts[hungryIdx].AvgHunger ||
				(c.AvgHunger == w.Cohorts[hungryIdx].AvgHunger && c.Population > w.Cohorts[hungryIdx].Population) {
				hungryIdx = i
			}
		}
		surplus := c.ItemPool[foodItem] - int64(c.Population)*3
		if c.District == dID && surplus >= 4 {
			if donorIdx == -1 || c.ItemPool[foodItem] > w.Cohorts[donorIdx].ItemPool[foodItem] {
				donorIdx = i
			}
		}
	}
	if hungryIdx == -1 {
		return nil
	}

	hungry := &w.Cohorts[hungryIdx]
	var movedFood int64
	donorID := ""
	if donorIdx != -1 && donorIdx != hungryIdx {
		donor := &w.Cohorts[donorIdx]
		donorID = donor.ID
		surplus := donor.ItemPool[foodItem] - int64(donor.Population)*2
		if surplus > 0 {
			want := maxInt64(4, minInt64(int64(hungry.Population), surplus/3))
			movedFood = minInt64(want, donor.ItemPool[foodItem])
			if movedFood > 0 {
				donor.ItemPool[foodItem] -= movedFood
				if donor.ItemPool[foodItem] <= 0 {
					delete(donor.ItemPool, foodItem)
				}
				if hungry.ItemPool == nil {
					hungry.ItemPool = map[string]int64{}
				}
				hungry.ItemPool[foodItem] += movedFood
				if hungry.AvgHunger > 0 {
					hungry.AvgHunger--
				}
			}
		}
	}

	riotForce := maxInt(2, minInt(16, hungry.EffectivePopulation()/100+2))
	wHungry, dHungry, jHungry := w.applyCohortCasualtiesWithCause(hungry, maxInt(1, riotForce/3), riotForce/16, maxInt(0, riotForce/5), "combat")
	var wLaw, dLaw int
	if law := w.presentLawCollector(dID); law != nil && law.ID != hungry.ID {
		wLaw, dLaw, _ = w.applyCohortCasualtiesWithCause(law, riotForce/6, 0, 0, "combat")
	}

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  hungry.ID,
		TargetID: donorID,
		Kind:     "granary_riot",
		Action:   Action{Kind: "attack", TargetID: donorID, Location: dID, Item: foodItem, Quantity: int(movedFood)},
		Payload: map[string]any{
			"text":               fmt.Sprintf("Granary riot erupted in %s (grievance=%d): %s seized %d %s (%d wounded, %d deaths, %d jailed)", dID, grievance, hungry.Name, movedFood, foodItem, wHungry+wLaw, dHungry+dLaw, jHungry),
			"district":           dID,
			"grievance":          grievance,
			"hungry_cohort":      hungry.ID,
			"donor_cohort":       donorID,
			"food_item":          foodItem,
			"food_redistributed": movedFood,
			"wounded":            wHungry + wLaw,
			"deaths":             dHungry + dLaw,
			"jailed":             jHungry,
		},
	}
	w.NextID++
	return []Event{ev}
}

func (w *World) triggerPopularUprising(jr *JusticeRevoltState, spec JusticeRevoltSpec, dID string, grievance int) []Event {
	srcIdx := -1
	bestFree := 0
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.District != dID || c.Population <= 0 {
			continue
		}
		if w.isOutlawCohort(c, spec) || w.isCivicServiceProfession(c.Profession) || jr.isActiveRebelCohort(c.ID) {
			continue
		}
		freeHealthy := cohortFreeHealthyAdults(w.anonymousCohort(*c))
		if freeHealthy >= 4 && freeHealthy > bestFree {
			srcIdx = i
			bestFree = freeHealthy
		}
	}
	if srcIdx == -1 {
		return nil
	}

	src := &w.Cohorts[srcIdx]
	srcPopBefore := w.anonymousCohort(*src).Population
	if srcPopBefore <= 0 {
		return nil
	}

	// Scale rebel conversion fraction cleanly within [MinRevoltSharePermille..MaxRevoltSharePermille] (2%..6%).
	span := 100 - spec.RevoltThreshold
	if span < 1 {
		span = 1
	}
	excess := grievance - spec.RevoltThreshold
	if excess < 0 {
		excess = 0
	}
	sharePermille := spec.MinRevoltSharePermille + ((spec.MaxRevoltSharePermille-spec.MinRevoltSharePermille)*excess)/span
	if sharePermille > spec.MaxRevoltSharePermille {
		sharePermille = spec.MaxRevoltSharePermille
	}
	if sharePermille < spec.MinRevoltSharePermille {
		sharePermille = spec.MinRevoltSharePermille
	}

	moved := (bestFree * sharePermille) / 1000
	if moved < 2 {
		moved = 2
	}
	if moved > bestFree/2 {
		moved = bestFree / 2
	}
	if moved <= 0 {
		return nil
	}

	// Atomically deduct healthy free adults (preserving Wounded, Sick, Jailed, and age sum == Population).
	src.Population -= moved
	if src.Adults >= moved {
		src.Adults -= moved
	} else if src.Children == 0 && src.Elders == 0 {
		// Unpyramided custom cohort
	} else {
		reconcileCohortAgeBrackets(src)
	}

	coinShare := (src.MoneyPool * int64(moved)) / int64(srcPopBefore)
	if coinShare > src.MoneyPool {
		coinShare = src.MoneyPool
	}
	src.MoneyPool -= coinShare

	rebelItems := map[string]int64{}
	for itemID, qty := range src.ItemPool {
		if qty <= 0 {
			continue
		}
		share := (qty * int64(moved)) / int64(srcPopBefore)
		if share > 0 {
			src.ItemPool[itemID] -= share
			if src.ItemPool[itemID] <= 0 {
				delete(src.ItemPool, itemID)
			}
			rebelItems[itemID] = share
		}
	}

	rebelProf, rebelFaction := w.resolveRebelProfession(spec, src.Profession)
	revoltSeq := jr.NextRevoltID
	jr.NextRevoltID++
	revoltID := fmt.Sprintf("revolt_%s_%d", dID, revoltSeq)
	rebelCohortID := fmt.Sprintf("cohort_rebel_%s_%d", dID, revoltSeq)

	reb := PopulationCohort{
		ID:         rebelCohortID,
		Name:       fmt.Sprintf("%s义军营#%d", dID, revoltSeq),
		District:   dID,
		FactionID:  rebelFaction,
		Profession: rebelProf,
		Tier:       1,
		ProfLevel:  1,
		Population: moved,
		Adults:     moved,
		AvgEnergy:  MaxEnergy * 3 / 4,
		AvgHunger:  src.AvgHunger,
		MoneyPool:  coinShare,
		ItemPool:   rebelItems,
	}
	w.transferCohortEnergyAndHunger(src, &reb, moved, srcPopBefore, 0)
	w.transferCohortStoredShare(src, &reb, moved, srcPopBefore)
	w.Cohorts = append(w.Cohorts, reb)

	jr.ActiveRevolts = append(jr.ActiveRevolts, ActiveRevolt{
		ID:             revoltID,
		District:       dID,
		RebelCohortID:  rebelCohortID,
		SourceCohortID: src.ID,
		StartedTick:    w.Tick,
		PeakPop:        moved,
		Status:         "active",
	})

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  rebelCohortID,
		TargetID: src.ID,
		Kind:     "revolt_erupted",
		Action:   Action{Kind: "raid", Location: dID},
		Payload: map[string]any{
			"text":              fmt.Sprintf("Popular uprising erupted in %s (grievance=%d): %d adults from %s took up arms as %s (carrying %d coins)", dID, grievance, moved, src.Name, reb.Name, coinShare),
			"revolt_id":         revoltID,
			"district":          dID,
			"source_cohort":     src.ID,
			"rebel_cohort":      rebelCohortID,
			"converted_adults":  moved,
			"coins_transferred": coinShare,
			"grievance":         grievance,
		},
	}
	w.NextID++
	return []Event{ev}
}

func cohortFreeHealthyAdults(c PopulationCohort) int {
	avail := c.Population - c.Wounded - c.Sick + c.WoundedSick - c.Jailed
	if avail <= 0 {
		return 0
	}
	if c.Children+c.Adults+c.Elders > 0 {
		if c.Adults < avail {
			avail = c.Adults
		}
	}
	if avail < 0 {
		return 0
	}
	return avail
}

func (w *World) stepPacificationAndAmnesty(jr *JusticeRevoltState, spec JusticeRevoltSpec) []Event {
	var events []Event

	// 1. Garrison / Martial Suppression of active revolts
	for i := range jr.ActiveRevolts {
		r := &jr.ActiveRevolts[i]
		if r.Status != "active" {
			continue
		}
		reb := w.findCohortByID(r.RebelCohortID)
		if reb == nil || reb.Population <= 0 {
			r.Status = "suppressed"
			r.ResolvedTick = w.Tick
			continue
		}
		if w.Tick < r.StartedTick+12 {
			continue
		}

		// Only a garrison already present under the local governing authority can suppress rebels.
		var garrison *PopulationCohort
		for j := range w.Cohorts {
			c := &w.Cohorts[j]
			if c.ID == reb.ID || !w.cohortHasJurisdiction(c, reb.District) || !w.isGarrisonCohort(c, spec) || c.EffectivePopulation() < 3 {
				continue
			}
			if garrison == nil || (c.District == r.District && garrison.District != r.District) || c.EffectivePopulation() > garrison.EffectivePopulation() {
				garrison = c
			}
		}
		if garrison == nil || reb.EffectivePopulation() < 2 {
			continue
		}

		rebEff := reb.EffectivePopulation()
		rebForce := minInt(80, maxInt(2, rebEff/8+1))
		rebW, rebD, rebJ := w.applyCohortCasualtiesWithCause(reb, maxInt(1, rebForce/4), rebForce/16, maxInt(1, rebForce/5), "combat")
		lawW, lawD, _ := w.applyCohortCasualtiesWithCause(garrison, maxInt(1, rebForce/8), rebForce/36, 0, "combat")

		bounty := w.debitTreasury(int64(maxInt(1, rebJ+rebD))*4, "bounty")
		garrison.MoneyPool += bounty

		if reb.EffectivePopulation() < 2 {
			r.Status = "suppressed"
			r.ResolvedTick = w.Tick
		}

		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  garrison.ID,
			TargetID: reb.ID,
			Kind:     "revolt_suppressed",
			Action:   Action{Kind: "arrest", TargetID: reb.ID, Location: r.District, Price: int(bounty)},
			Payload: map[string]any{
				"text":             fmt.Sprintf("%s clashed with %s in %s (%d wounded, %d deaths, %d jailed, %d bounty; remaining rebels=%d)", garrison.Name, reb.Name, r.District, rebW+lawW, rebD+lawD, rebJ, bounty, reb.EffectivePopulation()),
				"revolt_id":        r.ID,
				"district":         r.District,
				"garrison_cohort":  garrison.ID,
				"rebel_cohort":     reb.ID,
				"wounded":          rebW + lawW,
				"deaths":           rebD + lawD,
				"jailed":           rebJ,
				"bounty_coins":     bounty,
				"remaining_rebels": reb.EffectivePopulation(),
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 2. Sovereign Amnesty & Tax-Relief Decree when instability or grievance peaks
	if w.Tick-jr.LastAmnestyTick >= spec.AmnestyCooldownTicks {
		shouldAmnesty := false
		for _, r := range jr.ActiveRevolts {
			if r.Status == "active" || (r.Status == "suppressed" && w.Tick-r.ResolvedTick < spec.AmnestyCooldownTicks) {
				shouldAmnesty = true
				break
			}
		}
		if !shouldAmnesty {
			striking := 0
			for _, until := range jr.TaxStrikeUntil {
				if w.Tick < until {
					striking++
				}
			}
			if striking >= 2 {
				shouldAmnesty = true
			}
		}
		if !shouldAmnesty {
			for _, g := range jr.DistrictGrievance {
				if g >= spec.AmnestyGrievanceThreshold {
					shouldAmnesty = true
					break
				}
			}
		}

		if shouldAmnesty {
			events = append(events, w.executeDynasticAmnesty(jr, spec)...)
		}
	}

	// 3. Reintegrate any leftover healthy rebels from already-amnestied cohorts after recovery
	for i := range jr.ActiveRevolts {
		r := &jr.ActiveRevolts[i]
		if r.Status != "amnestied" {
			continue
		}
		if evs := w.reintegrateRebels(jr, spec, r); len(evs) > 0 {
			events = append(events, evs...)
		}
	}

	return events
}

func (w *World) executeDynasticAmnesty(jr *JusticeRevoltState, spec JusticeRevoltSpec) []Event {
	jr.LastAmnestyTick = w.Tick
	var events []Event

	// Clear tax strikes & slash grievance + corruption across all districts
	for dID := range jr.TaxStrikeUntil {
		delete(jr.TaxStrikeUntil, dID)
	}
	relievedCount := 0
	for _, dID := range w.sortedDistrictIDs() {
		if jr.DistrictGrievance[dID] > 0 {
			jr.addGrievance(dID, -35)
			relievedCount++
		}
		w.addDistrictCorruption(dID, -10)
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[dID]; ok {
				ds.MoraleBonus = minInt(25, ds.MoraleBonus+5)
				w.DistrictStates[dID] = ds
			}
		}
	}

	// Pardon non-violent civilian prisoners (and partial rebel captives) with exact jail ledger tracking
	totalReleased := 0
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 || c.Jailed <= 0 {
			continue
		}
		isOutlaw := w.isOutlawCohort(c, spec)
		isRebel := false
		for _, r := range jr.ActiveRevolts {
			if r.RebelCohortID == c.ID && (r.Status == "active" || r.Status == "suppressed") {
				isRebel = true
				break
			}
		}
		if !isOutlaw {
			rel := c.Jailed
			c.Jailed = 0
			totalReleased += rel
		} else if isRebel {
			rel := maxInt(1, c.Jailed/2)
			if rel > c.Jailed {
				rel = c.Jailed
			}
			c.Jailed -= rel
			totalReleased += rel
		}
	}

	// Pardon wrongfully convicted or non-outlaw jailed focal actors
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead || a.JailedUntil <= w.Tick {
			continue
		}
		aTags := a.IdentityTags()
		if !ActorHasTag(aTags, spec.OutlawTag) && !w.Catalog.IsOutlawTags(aTags) {
			a.JailedUntil = 0
			totalReleased++
		}
	}
	// Mark open wrongful court cases whose victims were pardoned as overturned
	for i := range jr.CourtCases {
		if jr.CourtCases[i].Status == "convicted" && jr.CourtCases[i].Wrongful {
			jr.CourtCases[i].Status = "overturned"
			jr.CourtCases[i].AppealByID = "dynastic_amnesty"
			jr.CourtCases[i].ResolvedTick = w.Tick
		}
	}

	// Disburse treasury relief to poorest non-outlaw civilian cohorts
	var totalReliefPaid int64
	if w.PublicTreasury > 0 && len(w.Cohorts) > 0 {
		var needy []*PopulationCohort
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.Population <= 0 || w.isOutlawCohort(c, spec) || w.isCivicServiceProfession(c.Profession) || jr.isActiveRebelCohort(c.ID) {
				continue
			}
			if c.AvgHunger >= 2 || c.MoneyPool < int64(c.Population)*8 || jr.DistrictGrievance[c.District] >= 20 {
				needy = append(needy, c)
			}
		}
		if len(needy) > 0 {
			budget := minInt64(w.PublicTreasury/8, int64(len(needy))*400)
			perCohort := budget / int64(len(needy))
			if perCohort > 0 {
				for _, c := range needy {
					paid := w.debitTreasury(perCohort, "relief")
					c.MoneyPool += paid
					totalReliefPaid += paid
				}
			}
		}
	}

	amnestyEv := Event{
		ID:      w.NextID,
		Tick:    w.Tick,
		ActorID: "world",
		Kind:    "amnesty_decreed",
		Action:  Action{Kind: "wait"},
		Payload: map[string]any{
			"text":               fmt.Sprintf("Dynastic amnesty & tax-relief decree proclaimed across %d districts (%d prisoners pardoned, %d treasury relief disbursed)", relievedCount, totalReleased, totalReliefPaid),
			"released":           totalReleased,
			"relief_coins":       totalReliefPaid,
			"districts_relieved": relievedCount,
		},
	}
	w.NextID++
	events = append(events, amnestyEv)

	// Reintegrate active or suppressed rebel cohorts back into civilian production
	for i := range jr.ActiveRevolts {
		r := &jr.ActiveRevolts[i]
		if r.Status != "active" && r.Status != "suppressed" {
			continue
		}
		r.Status = "amnestied"
		r.ResolvedTick = w.Tick
		events = append(events, w.reintegrateRebels(jr, spec, r)...)
	}

	return events
}

func (w *World) reintegrateRebels(jr *JusticeRevoltState, spec JusticeRevoltSpec, r *ActiveRevolt) []Event {
	reb := w.findCohortByID(r.RebelCohortID)
	if reb == nil || reb.Population <= 0 {
		return nil
	}

	dst := w.findCohortByID(r.SourceCohortID)
	if dst == nil || dst.District != reb.District || dst.Population <= 0 || w.isOutlawCohort(dst, spec) {
		dst = nil
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.ID == reb.ID || c.District != reb.District || c.Population <= 0 || w.isOutlawCohort(c, spec) || w.isCivicServiceProfession(c.Profession) {
				continue
			}
			if dst == nil || (c.District == r.District && dst.District != r.District) || c.Population > dst.Population {
				dst = c
			}
		}
	}
	if dst == nil {
		return nil
	}

	// If no wounded/sick/jailed remain in reb, reintegrate 100% (including any children/elders);
	// otherwise transfer only healthy free adults so Wound/Sick/Jail ledgers stay exact.
	canFullDrain := reb.Wounded == 0 && reb.Sick == 0 && reb.Jailed == 0
	var moved int
	anonymousBefore := w.anonymousCohort(*reb).Population
	dstAnonymousBefore := w.anonymousCohort(*dst).Population
	popBefore := anonymousBefore
	if canFullDrain {
		popBefore = reb.Population
		moved = reb.Population
		dst.Population += moved
		if reb.Children+reb.Adults+reb.Elders > 0 {
			dst.Children += reb.Children
			dst.Adults += reb.Adults
			dst.Elders += reb.Elders
		} else {
			dst.Adults += moved
		}
		dst.Members = append(dst.Members, reb.Members...)
		reb.Members = nil
		dst.ChildAgeRemainder += reb.ChildAgeRemainder
		dst.AdultAgeRemainder += reb.AdultAgeRemainder
		reb.ChildAgeRemainder, reb.AdultAgeRemainder = 0, 0
		reb.Population = 0
		reb.Children, reb.Adults, reb.Elders = 0, 0, 0
		w.transferShipmentOwnership(reb.ID, dst.ID)
		w.transferGrowingAssets("cohort", reb.ID, "cohort", dst.ID)
	} else {
		freeHealthy := cohortFreeHealthyAdults(w.anonymousCohort(*reb))
		if freeHealthy <= 0 {
			return nil
		}
		moved = freeHealthy
		reb.Population -= moved
		if reb.Adults >= moved {
			reb.Adults -= moved
		} else if reb.Children+reb.Elders > 0 {
			reconcileCohortAgeBrackets(reb)
		}
		dst.Population += moved
		dst.Adults += moved
	}

	energyMoved := moved
	if canFullDrain {
		energyMoved = anonymousBefore
	}
	w.transferCohortEnergyAndHunger(reb, dst, energyMoved, anonymousBefore, dstAnonymousBefore)
	w.transferCohortStoredShare(reb, dst, moved, popBefore)

	var coinShare int64
	if canFullDrain {
		coinShare = reb.MoneyPool
	} else {
		coinShare = (reb.MoneyPool * int64(moved)) / int64(popBefore)
	}
	if coinShare > reb.MoneyPool {
		coinShare = reb.MoneyPool
	}
	if coinShare > 0 {
		reb.MoneyPool -= coinShare
		dst.MoneyPool += coinShare
	}

	if dst.ItemPool == nil && len(reb.ItemPool) > 0 {
		dst.ItemPool = map[string]int64{}
	}
	for itemID, qty := range reb.ItemPool {
		if qty <= 0 {
			continue
		}
		share := qty
		if !canFullDrain {
			share = (qty * int64(moved)) / int64(popBefore)
		}
		if share > 0 {
			reb.ItemPool[itemID] -= share
			if reb.ItemPool[itemID] <= 0 {
				delete(reb.ItemPool, itemID)
			}
			dst.ItemPool[itemID] += share
		}
	}

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  reb.ID,
		TargetID: dst.ID,
		Kind:     "rebel_reintegrated",
		Action:   Action{Kind: "wait", TargetID: dst.ID, Location: r.District},
		Payload: map[string]any{
			"text":                fmt.Sprintf("Amnestied rebels (%d from %s) laid down arms and returned to productive life in %s (transferred %d coins)", moved, reb.Name, dst.Name, coinShare),
			"revolt_id":           r.ID,
			"district":            r.District,
			"rebel_cohort":        reb.ID,
			"target_cohort":       dst.ID,
			"reintegrated_adults": moved,
			"coins_transferred":   coinShare,
		},
	}
	w.NextID++
	return []Event{ev}
}

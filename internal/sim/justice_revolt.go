package sim

import (
	"fmt"
	"sort"
)

// JusticeRevoltSpec defines declarative, genre-decoupled thresholds and parameters for
// magistrate court trials, scapegoat framing, oversight appeals, grievance escalation,
// popular uprisings, and state pacification/amnesty cycles.
type JusticeRevoltSpec struct {
	ExplicitZero               bool   `json:"explicit_zero,omitempty"`
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
	explicitFields             map[string]bool
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
	s.CorruptionFramingThreshold = resolveSpecNumber(s.CorruptionFramingThreshold, def.CorruptionFramingThreshold, s.ExplicitZero || s.explicitFields["corruption_framing_threshold"])
	s.TaxStrikeThreshold = resolveSpecNumber(s.TaxStrikeThreshold, def.TaxStrikeThreshold, s.ExplicitZero || s.explicitFields["tax_strike_threshold"])
	s.GranaryRiotThreshold = resolveSpecNumber(s.GranaryRiotThreshold, def.GranaryRiotThreshold, s.ExplicitZero || s.explicitFields["granary_riot_threshold"])
	s.RevoltThreshold = resolveSpecNumber(s.RevoltThreshold, def.RevoltThreshold, s.ExplicitZero || s.explicitFields["revolt_threshold"])
	s.AmnestyGrievanceThreshold = resolveSpecNumber(s.AmnestyGrievanceThreshold, def.AmnestyGrievanceThreshold, s.ExplicitZero || s.explicitFields["amnesty_grievance_threshold"])
	s.MinRevoltSharePermille = resolveSpecNumber(s.MinRevoltSharePermille, def.MinRevoltSharePermille, s.ExplicitZero || s.explicitFields["min_revolt_share_permille"])
	s.MaxRevoltSharePermille = resolveSpecNumber(s.MaxRevoltSharePermille, def.MaxRevoltSharePermille, s.ExplicitZero || s.explicitFields["max_revolt_share_permille"])
	s.TrialSentenceTicks = resolveSpecNumber(s.TrialSentenceTicks, def.TrialSentenceTicks, s.ExplicitZero || s.explicitFields["trial_sentence_ticks"])
	s.TaxStrikeDurationTicks = resolveSpecNumber(s.TaxStrikeDurationTicks, def.TaxStrikeDurationTicks, s.ExplicitZero || s.explicitFields["tax_strike_duration_ticks"])
	s.RevoltCooldownTicks = resolveSpecNumber(s.RevoltCooldownTicks, def.RevoltCooldownTicks, s.ExplicitZero || s.explicitFields["revolt_cooldown_ticks"])
	s.AmnestyCooldownTicks = resolveSpecNumber(s.AmnestyCooldownTicks, def.AmnestyCooldownTicks, s.ExplicitZero || s.explicitFields["amnesty_cooldown_ticks"])
	s.AppealCompensationCoins = resolveSpecNumber(s.AppealCompensationCoins, def.AppealCompensationCoins, s.ExplicitZero || s.explicitFields["appeal_compensation_coins"])
	if !s.explicitFields["max_revolt_share_permille"] && s.MaxRevoltSharePermille < s.MinRevoltSharePermille {
		s.MaxRevoltSharePermille = s.MinRevoltSharePermille
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
	BasisEventID      int64  `json:"basis_event_id,omitempty"`
	WarrantEventID    int64  `json:"warrant_event_id,omitempty"`
	ArrestEventID     int64  `json:"arrest_event_id,omitempty"`
	ConvictionEventID int64  `json:"conviction_event_id,omitempty"`
	RootEventID       int64  `json:"root_event_id,omitempty"`
	Proceeds          int    `json:"proceeds,omitempty"`
	ID                int64  `json:"id"`
	Tick              int64  `json:"tick"`
	District          string `json:"district"`
	Kind              string `json:"kind"`
	MagistrateID      string `json:"magistrate_id"`
	SuspectID         string `json:"suspect_id"`
	RealCulpritID     string `json:"real_culprit_id,omitempty"`
	IsCohort          bool   `json:"is_cohort,omitempty"`
	Wrongful          bool   `json:"wrongful,omitempty"`
	JailedCount       int    `json:"jailed_count,omitempty"`
	JailedUntilTick   int64  `json:"jailed_until_tick,omitempty"`
	BribeShielded     bool   `json:"bribe_shielded,omitempty"`
	Status            string `json:"status"` // "convicted", "overturned", "upheld"
	AppealByID        string `json:"appeal_by_id,omitempty"`
	ResolvedTick      int64  `json:"resolved_tick,omitempty"`
}

// ActiveRevolt tracks an erupted popular uprising cohort spawned from distressed civilians.
type ActiveRevolt struct {
	ID             string `json:"id"`
	District       string `json:"district"`
	RebelCohortID  string `json:"rebel_cohort_id"`
	SourceCohortID string `json:"source_cohort_id"`
	StartedTick    int64  `json:"started_tick"`
	StartedEventID int64  `json:"started_event_id,omitempty"`
	AmnestyEventID int64  `json:"amnesty_event_id,omitempty"`
	AmnestyRootID  int64  `json:"amnesty_root_event_id,omitempty"`
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
func (w *World) ApplyJusticeAction(state *JusticeRevoltState, actorID string, x Action) ([]Event, bool) {
	return w.applyJusticeSubsystem(state, actorID, x)
}

func (w *World) applyJusticeActionTx(tx *WorldTx, jr *JusticeRevoltState, a *Actor, x Action) []Event {
	if jr == nil {
		return nil
	}
	jr.ensureInitialized()
	spec := w.effectiveJusticeSpec(jr)
	fail := func(reason string) []Event { return actionFail(tx, a, x, reason) }

	b := w.actor(x.TargetID)

	switch x.Kind {
	case "frame_suspect":
		if !w.Catalog.EffectiveEnvironment().Institutions.EnforcementEnabled() {
			return fail("enforcement framing is disabled in this world environment")
		}
		if !w.actorCanEnforceLaw(*a, spec) {
			return fail("actor lacks enforcement authority")
		}
		if b == nil || b.ID == a.ID || !SamePlace(*a, *b) {
			return fail("target suspect is not nearby")
		}
		if w.Tick < b.JailedUntil {
			return fail("target is already imprisoned")
		}
		bTags := b.IdentityTags()
		if ActorHasTag(bTags, spec.OutlawTag) || w.Catalog.IsOutlawTags(bTags) {
			return fail("cannot scapegoat-frame an outlaw")
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
			return fail("local corruption is too low for scapegoat framing")
		}

		observers := append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)
		b.Task = nil
		tx.MoveActor(b, b.Location, b.VenueID, "")
		tx.setSentence(b, w.Tick+spec.TrialSentenceTicks)
		tx.MutateTrust(b, a.ID, -5)

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

		ev := tx.record(a, x, "wrongful_conviction", map[string]any{
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
		jr.CourtCases[len(jr.CourtCases)-1].ConvictionEventID = ev.ID
		jr.CourtCases[len(jr.CourtCases)-1].RootEventID = ev.ID
		return []Event{ev}

	case "file_appeal":
		if !w.actorCanAppeal(*a, spec) {
			return fail("actor lacks oversight or public-trust standing to file appeal")
		}
		if a.IllicitGain > 0 {
			return fail("corrupt official cannot adjudicate appeal")
		}
		if x.TargetID == "" && x.BasisEventID == 0 {
			return fail("appeal must specify a known wrongful-conviction target or basis event")
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
			return fail("actor has no witnessed or reported knowledge of this wrongful conviction")
		}

		// Locate matching open wrongful conviction that the actor actually knows about AND is in the actor's current district.
		caseIdx := -1
		var matchedLead KnownWrongfulLead
		remoteDistrict := ""
		for i := len(jr.CourtCases) - 1; i >= 0; i-- {
			c := jr.CourtCases[i]
			if (c.Status != "convicted" && c.Status != "served") || !c.Wrongful {
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
				return fail("must travel to " + remoteDistrict + " court to file appeal")
			}
			return fail("no known open wrongful conviction found in current district")
		}

		cc := &jr.CourtCases[caseIdx]
		if cc.IsCohort && !w.courtPartiesPresent(*cc) {
			return fail("court parties must be present in the case district for release and enforcement")
		}
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
				tx.trackStoredActor(victim.ID)
				if victim.JailedUntil > 0 {
					tx.setSentence(victim, 0)
					released = 1
				}
				reliefCoins = tx.debitTreasury(spec.AppealCompensationCoins, "relief")
				tx.moveCurrency(victim, "coins", reliefCoins, "treasury", x.Kind)
				tx.MutateTrust(victim, a.ID, 4)
				observers = append(observers, victim)
			}
			if mag := w.actor(cc.MagistrateID); mag != nil && !mag.Dead && mag.ID != a.ID {
				tx.trackStoredActor(mag.ID)
				if mag.IllicitGain > 0 && mag.Money > 0 {
					seize := minInt(mag.IllicitGain, mag.Money)
					tx.moveCurrency(mag, "coins", -int64(seize), "treasury", x.Kind)
					mag.IllicitGain -= seize
					confiscatedCoins = int64(seize)
					tx.creditTreasury(confiscatedCoins, "confiscated")
				}
				if mag.JailedUntil <= w.Tick {
					mag.Task = nil
					tx.MoveActor(mag, mag.Location, mag.VenueID, "")
					tx.setSentence(mag, w.Tick+spec.TrialSentenceTicks)
					jailedFramer = 1
				}
				observers = append(observers, mag)
			}
		} else {
			if victimCohort := w.findCohortByID(cc.SuspectID); victimCohort != nil && victimCohort.Population > 0 {
				if cc.JailedCount > 0 && victimCohort.Jailed > 0 {
					released = w.releaseCohortJailed(victimCohort, cc.JailedCount)
				}
				reliefCoins = tx.debitTreasury(int64(maxInt(1, released))*3, "relief")
				w.addCohortCurrency(victimCohort, "coins", reliefCoins)
				tx.recordTransfer("transfer_currency", "treasury", victimCohort.ID, "coins", -reliefCoins, x.Kind)
			}
			if magCohort := w.findCohortByID(cc.MagistrateID); magCohort != nil && magCohort.EffectivePopulation() >= 1 {
				if magCohort.IllicitWealth > 0 && magCohort.MoneyPool > 0 {
					seize := minInt64(magCohort.MoneyPool, minInt64(magCohort.IllicitWealth, int64(maxInt(1, released))*4))
					if seize > 0 {
						w.addCohortCurrency(magCohort, "coins", -(seize))
						tx.recordTransfer("transfer_currency", magCohort.ID, "treasury", "coins", -seize, x.Kind)
						magCohort.IllicitWealth -= seize
						confiscatedCoins = seize
						tx.creditTreasury(seize, "confiscated")
					}
				}
				_, _, jailedFramer = w.applyCohortCasualtiesWithCause(magCohort, 0, 0, minInt(4, maxInt(1, released/2)), "combat")
			}
		}

		w.addDistrictCorruption(cc.District, -10)
		grievance := jr.addGrievance(cc.District, -14)

		appealData := map[string]any{
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
			"observation": Observation{Kind: "conviction_overturned", SubjectID: a.ID, ObjectID: cc.SuspectID,
				CaseID: cc.ID, Location: cc.District},
		}
		if cc.ConvictionEventID > 0 {
			appealData["cause_event_id"] = cc.ConvictionEventID
		} else if x.BasisEventID > 0 {
			appealData["cause_event_id"] = x.BasisEventID
		}
		if cc.RootEventID > 0 {
			appealData["root_event_id"] = cc.RootEventID
		} else if cc.ConvictionEventID > 0 {
			appealData["root_event_id"] = cc.ConvictionEventID
		}
		ev := tx.record(a, x, "conviction_overturned", appealData, observers...)
		events := []Event{ev}
		if appealProf := w.Catalog.BestProfessionForCapability(*a, "file_appeal"); appealProf != "" {
			events = append(events, w.grantProfessionXP(tx, a, x, appealProf, 8, 3)...)
		} else if auditProf := w.Catalog.BestProfessionForCapability(*a, spec.OversightCapability); auditProf != "" {
			events = append(events, w.grantProfessionXP(tx, a, x, auditProf, 8, 3)...)
		}
		return events
	}

	return nil
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
			if c.Status == "wanted" || c.Status == "arrested" || c.Status == "convicted" || c.RootEventID > 0 && !c.IsCohort || tick-c.ResolvedTick < retention {
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
						"root_event_id":  trialEv.ID,
						"cause_event_id": trialEv.ID,
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
				if len(jr.CourtCases) > 0 && jr.CourtCases[len(jr.CourtCases)-1].ID == caseID {
					jr.CourtCases[len(jr.CourtCases)-1].ConvictionEventID = frameEv.ID
					jr.CourtCases[len(jr.CourtCases)-1].RootEventID = trialEv.ID
				}
				obs := Observation{Kind: "wrongful_conviction", Modality: "eyewitness",
					SubjectID: mag.ID, ObjectID: civ.ID, CaseID: caseID, Location: dID}
				for i := range w.Actors {
					witness := &w.Actors[i]
					if witness.Location == dID && witness.InsideHouse == "" && !witness.InTransit() && w.partyMateAvailable(witness) {
						appendActorMemory(witness, Memory{Kind: frameEv.Kind, EventID: frameEv.ID,
							Tick: w.Tick, Source: mag.ID, TargetID: civ.ID, RootEventID: trialEv.ID,
							CauseEventID: trialEv.ID, Observation: &obs, Text: relPayloadString(frameEv.Payload, "text")})
					}
				}
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

		if victimCohort := w.findCohortByID(cc.SuspectID); victimCohort != nil && victimCohort.Population > 0 {
			if cc.JailedCount > 0 && victimCohort.Jailed > 0 {
				released = w.releaseCohortJailed(victimCohort, cc.JailedCount)
			}
			reliefCoins = w.debitTreasury(int64(maxInt(1, released))*3, "relief")
			w.addCohortCurrency(victimCohort, "coins", reliefCoins)
		}
		if magCohort := w.findCohortByID(cc.MagistrateID); magCohort != nil && magCohort.EffectivePopulation() >= 1 {
			if magCohort.IllicitWealth > 0 && magCohort.MoneyPool > 0 {
				seize := minInt64(magCohort.MoneyPool, minInt64(magCohort.IllicitWealth, int64(maxInt(1, released))*4))
				if seize > 0 {
					w.addCohortCurrency(magCohort, "coins", -seize)
					magCohort.IllicitWealth -= seize
					confiscatedCoins = seize
					w.creditTreasury(seize, "confiscated")
				}
			}
			_, _, jailedFramer = w.applyCohortCasualtiesWithCause(magCohort, 0, 0, minInt(4, maxInt(1, released/2)), "combat")
		}

		w.addDistrictCorruption(cc.District, -8)
		grievance := jr.addGrievance(cc.District, -12)
		if censorCohort != nil {
			events = append(events, w.addCohortXP(censorCohort, 4, 2)...)
		}

		macroAppealPayload := map[string]any{
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
		}
		if cc.ConvictionEventID > 0 {
			macroAppealPayload["cause_event_id"] = cc.ConvictionEventID
		}
		if cc.RootEventID > 0 {
			macroAppealPayload["root_event_id"] = cc.RootEventID
		} else if cc.ConvictionEventID > 0 {
			macroAppealPayload["root_event_id"] = cc.ConvictionEventID
		}
		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  censorID,
			TargetID: cc.SuspectID,
			Kind:     "conviction_overturned",
			Action:   Action{Kind: "file_appeal", TargetID: cc.SuspectID, Location: cc.District},
			Payload:  macroAppealPayload,
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
				w.adjustCohortItem(donor, foodItem, -movedFood, false)
				w.adjustCohortItem(hungry, foodItem, movedFood, false)
				w.relieveCohortHunger(hungry, 1)
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
	w.addCohortCurrency(src, "coins", -(coinShare))

	rebelItems := map[string]int64{}
	for itemID, qty := range src.ItemPool {
		if qty <= 0 {
			continue
		}
		share := (qty * int64(moved)) / int64(srcPopBefore)
		if share > 0 {
			w.adjustCohortItem(src, itemID, -(share), false)
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
		StartedEventID: w.NextID,
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
			"root_event_id":     w.NextID,
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
		w.addCohortCurrency(garrison, "coins", bounty)

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
				"cause_event_id":   r.StartedEventID,
				"root_event_id":    r.StartedEventID,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// 2. Sovereign Amnesty & Tax-Relief Decree when instability or grievance peaks
	if w.Tick-jr.LastAmnestyTick >= spec.AmnestyCooldownTicks {
		shouldAmnesty := false
		var revoltCause int64
		for _, r := range jr.ActiveRevolts {
			if r.Status == "active" || (r.Status == "suppressed" && w.Tick-r.ResolvedTick < spec.AmnestyCooldownTicks) {
				shouldAmnesty = true
				revoltCause = r.StartedEventID
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
			events = append(events, w.executeDynasticAmnesty(jr, spec, revoltCause)...)
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

func (w *World) executeDynasticAmnesty(jr *JusticeRevoltState, spec JusticeRevoltSpec, cause int64) []Event {
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
		var released int
		if !isOutlaw {
			released = w.releaseCohortJailed(c, c.Jailed)
		} else if isRebel {
			anonymous := w.anonymousCohort(*c)
			released = w.releaseCohortJailed(c, maxInt(1, anonymous.Jailed/2))
		}
		totalReleased += released
		jr.settleCohortRelease(c.ID, released, w.Tick, "dynastic_amnesty")
	}

	pardoned := map[string]bool{}
	pardon := func(a *Actor) {
		if a.Dead || a.JailedUntil <= w.Tick {
			return
		}
		aTags := a.IdentityTags()
		if !ActorHasTag(aTags, spec.OutlawTag) && !w.Catalog.IsOutlawTags(aTags) {
			(*WorldTx)(nil).setSentence(a, 0)
			pardoned[a.ID] = true
			totalReleased++
		}
	}
	for i := range w.Actors {
		pardon(&w.Actors[i])
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Jailed > 0 && len(c.Members) > 0 {
			before := append([]Actor(nil), c.Members...)
			for j := range c.Members {
				pardon(&c.Members[j])
			}
			w.reconcileCohortMemberChanges(c, before)
		}
	}
	for i := range jr.CourtCases {
		cc := &jr.CourtCases[i]
		if !cc.IsCohort && pardoned[cc.SuspectID] {
			switch cc.Status {
			case "convicted", "upheld":
				cc.Status, cc.ResolvedTick = "pardoned", w.Tick
				if cc.Wrongful {
					cc.Status = "overturned"
				}
				cc.AppealByID = "dynastic_amnesty"
			case "arrested":
				cc.Status, cc.ResolvedTick = "released", w.Tick
			}
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
					w.addCohortCurrency(c, "coins", paid)
					totalReliefPaid += paid
				}
			}
		}
	}

	root := cause
	if root == 0 {
		root = w.NextID
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
			"cause_event_id":     cause,
			"root_event_id":      root,
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
		r.AmnestyEventID, r.AmnestyRootID = amnestyEv.ID, root
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
		w.addCohortCurrency(reb, "coins", -(coinShare))
		w.addCohortCurrency(dst, "coins", coinShare)
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
			w.adjustCohortItem(reb, itemID, -(share), false)
			w.adjustCohortItem(dst, itemID, share, false)
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
			"cause_event_id":      r.AmnestyEventID,
			"root_event_id":       r.AmnestyRootID,
		},
	}
	w.NextID++
	return []Event{ev}
}

func (w *World) personalCase(district, suspect, status string) *CourtCase {
	if w.JusticeRevolt != nil {
		for i := range w.JusticeRevolt.CourtCases {
			c := &w.JusticeRevolt.CourtCases[i]
			if !c.IsCohort && c.Status == status && c.District == district && c.SuspectID == suspect {
				return c
			}
		}
	}
	return nil
}

func (w *World) caseEvidence(a Actor, suspectID string, basis int64) (Memory, bool) {
	m, ok := corruptionEvidence(a, suspectID, basis)
	if !ok {
		return Memory{}, false
	}
	if w.JusticeRevolt != nil {
		for _, c := range w.JusticeRevolt.CourtCases {
			if !c.IsCohort && c.SuspectID == suspectID && c.RootEventID == m.RootEventID {
				return Memory{}, false
			}
		}
	}
	return m, true
}

func (w *World) knownLocalCases(a Actor, nearby []Neighbor) []CaseView {
	var cases []CaseView
	caps := w.ActorCapabilities(a)
	seen := map[string]bool{}
	addEvidence := func(id string, basis int64) {
		if caps["open_case"] {
			if m, ok := w.caseEvidence(a, id, basis); ok && w.mayUseSecretEvidence(a, m) {
				cases = append(cases, CaseView{SuspectID: id, Status: "reported", BasisEventID: m.EventID})
				seen[id] = true
			}
		}
	}
	for _, nb := range nearby {
		id := nb.ID
		seen[id] = true
		switch {
		case caps["arrest"] && w.personalCase(a.Location, id, "wanted") != nil:
			cases = append(cases, CaseView{SuspectID: id, Status: "wanted"})
		case (caps["audit_corruption"] || caps["hold_trial"]) && w.personalCase(a.Location, id, "arrested") != nil:
			cases = append(cases, CaseView{SuspectID: id, Status: "arrested"})
		}
		addEvidence(id, 0)
	}
	for i := len(a.Memories) - 1; i >= 0 && caps["open_case"]; i-- {
		m := a.Memories[i]
		var id, district string
		if m.Observation != nil {
			id, district = m.Observation.SubjectID, m.Observation.Location
		} else if m.Statement != nil {
			id, district = m.Statement.SubjectID, m.Statement.Location
		}
		if id != "" && !seen[id] && district == a.Location {
			addEvidence(id, m.EventID)
		}
	}
	return cases
}

func (w *World) openPersonalCase(tx *WorldTx, a, suspect *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	institutions := w.Catalog.EffectiveEnvironment().Institutions
	if !institutions.EnforcementEnabled() && !institutions.OversightEnabled() {
		return fail("courts are disabled in this world")
	}
	if suspect == nil || suspect.Dead || a.IllicitGain > 0 {
		return fail("case requires a living suspect and an available officer")
	}
	m, ok := w.caseEvidence(*a, suspect.ID, x.BasisEventID)
	if !ok {
		return fail("case requires verified evidence that has not been adjudicated")
	}
	var district string
	var proceeds int
	if m.Observation != nil {
		district, proceeds = m.Observation.Location, m.Observation.Proceeds
	} else {
		district, proceeds = m.Statement.Location, m.Statement.Proceeds
	}
	if district != a.Location {
		return fail("the evidence belongs to a different district")
	}
	if w.personalCase(district, suspect.ID, "wanted") != nil || w.personalCase(district, suspect.ID, "arrested") != nil {
		return fail("a warrant or trial is already pending for this suspect")
	}
	jr := w.ensureJusticeRevolt()
	jr.ensureInitialized()
	x.BasisEventID = m.EventID
	cc := CourtCase{ID: jr.NextCaseID, Tick: w.Tick, District: district, Kind: "criminal_trial",
		MagistrateID: a.ID, SuspectID: suspect.ID, Status: "wanted", BasisEventID: m.EventID, WarrantEventID: w.NextID,
		RootEventID: m.RootEventID, Proceeds: proceeds}
	jr.NextCaseID++
	jr.CourtCases = append(jr.CourtCases, cc)
	notice := Statement{Topic: "court_warrant_issued", SubjectID: suspect.ID, Location: district, CaseID: cc.ID}
	ev := tx.record(a, x, notice.Topic, map[string]any{"text": fmt.Sprintf("Court opened case #%d and issued a warrant for %s in %s", cc.ID, suspect.Name, district),
		"case_id": cc.ID, "district": district, "statement": notice, "cause_event_id": m.EventID})
	for i := range w.Actors {
		resident := &w.Actors[i]
		if resident.ID == a.ID || resident.Dead || resident.Location != district || resident.InTransit() {
			continue
		}
		if resident.InsideHouse != "" && resident.InsideHouse != a.InsideHouse {
			continue
		}
		if resident.Task != nil && (resident.Task.Action.Kind == "sleep" || resident.Task.Dormant) {
			continue
		}
		tx.RecordMemory(resident, Memory{Kind: ev.Kind, EventID: ev.ID, RootEventID: cc.RootEventID,
			Tick: w.Tick, Source: a.ID, TargetID: suspect.ID, Statement: &notice, Text: fmt.Sprint(ev.Payload["text"])})
	}
	return []Event{ev}
}

func (w *World) tryPersonalCase(tx *WorldTx, a, suspect *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	institutions := w.Catalog.EffectiveEnvironment().Institutions
	if x.Kind == "audit_corruption" && !institutions.OversightEnabled() ||
		x.Kind == "hold_trial" && !institutions.EnforcementEnabled() {
		return fail("the required court institution is disabled in this world")
	}
	if suspect == nil || suspect.ID == a.ID || suspect.Dead || !SamePlace(*a, *suspect) || a.IllicitGain > 0 {
		return fail("hearing requires a nearby suspect and an available oversight officer")
	}
	cc := w.personalCase(a.Location, suspect.ID, "arrested")
	if cc == nil {
		return fail("no arrested defendant in an open case")
	}
	if x.BasisEventID > 0 {
		if m, ok := corruptionEvidence(*a, suspect.ID, x.BasisEventID); !ok || m.RootEventID != cc.RootEventID {
			return fail("cited evidence does not belong to this case")
		}
	}
	seized := minInt(suspect.Money, minInt(suspect.IllicitGain, cc.Proceeds))
	tx.moveCurrency(suspect, "coins", -int64(seized), "treasury", x.Kind)
	suspect.IllicitGain -= seized
	tx.creditTreasury(int64(seized), "confiscated")
	w.addDistrictCorruption(a.Location, -8)
	spec := w.effectiveJusticeSpec(w.JusticeRevolt)
	tx.setSentence(suspect, w.Tick+spec.TrialSentenceTicks)
	cc.Status, cc.ResolvedTick, cc.JailedUntilTick = "convicted", w.Tick, suspect.JailedUntil
	var revoked []string
	for _, faction := range w.Factions {
		if w.Relations != nil && w.Relations.FactionLeader[faction.ID] == suspect.ID {
			w.setFactionLeader(faction.ID, "")
			revoked = append(revoked, faction.ID)
		}
	}
	observers := append([]*Actor{suspect}, w.awakeSamePlaceObservers(*a, a.ID, suspect.ID)...)
	ev := tx.record(a, x, "court_trial_held", map[string]any{
		"text":    fmt.Sprintf("Court convicted %s in case #%d and confiscated %d coins", suspect.Name, cc.ID, seized),
		"case_id": cc.ID, "district": cc.District, "root_event_id": cc.RootEventID, "cause_event_id": cc.ArrestEventID,
		"confiscated_coins": seized, "jailed_until": suspect.JailedUntil, "revoked_faction_offices": revoked,
	}, observers...)
	outcome := tx.record(a, x, "corruption_purged", map[string]any{
		"text": fmt.Sprintf("Case #%d adjudicated against %s", cc.ID, suspect.Name), "case_id": cc.ID,
		"root_event_id": cc.RootEventID, "cause_event_id": ev.ID,
	}, observers...)
	return []Event{ev, outcome}
}

func (w *World) settleReleasedCases(actorID string) {
	if w.JusticeRevolt == nil {
		return
	}
	for i := range w.JusticeRevolt.CourtCases {
		cc := &w.JusticeRevolt.CourtCases[i]
		if !cc.IsCohort && cc.SuspectID == actorID {
			switch cc.Status {
			case "convicted", "upheld":
				cc.Status, cc.ResolvedTick = "served", w.Tick
			case "arrested":
				cc.Status, cc.ResolvedTick = "released", w.Tick
			}
		}
	}
}

func (w *World) mayUseSecretEvidence(a Actor, m Memory) bool {
	var topic, subject, object string
	if m.Observation != nil {
		topic, subject, object = m.Observation.Kind, m.Observation.SubjectID, m.Observation.ObjectID
	} else if m.Statement != nil {
		topic, subject, object = m.Statement.Topic, m.Statement.SubjectID, m.Statement.ObjectID
	}
	for _, s := range w.Secrets {
		if secretReportTopic(topic) && s.ID == object ||
			(topic == "corruption" || topic == "wrongful_conviction") && s.RootEventID == m.RootEventID && s.OwnerID == subject {
			if ActorHasTag(s.ExposureKnownBy, a.ID) {
				return true
			}
			stance := s.HolderStances[a.ID]
			return stance == "confront" || stance == ""
		}
	}
	return true
}

func (jr *JusticeRevoltState) settleCohortRelease(cohortID string, count int, tick int64, amnesty string) {
	if jr == nil {
		return
	}
	for i := range jr.CourtCases {
		cc := &jr.CourtCases[i]
		if count == 0 {
			break
		}
		if cc.IsCohort && cc.SuspectID == cohortID && cc.Status == "convicted" {
			released := minInt(cc.JailedCount, count)
			cc.JailedCount -= released
			count -= released
			if cc.JailedCount == 0 {
				cc.Status, cc.ResolvedTick = "served", tick
				if amnesty != "" {
					cc.Status, cc.AppealByID = "pardoned", amnesty
					if cc.Wrongful {
						cc.Status = "overturned"
					}
				}
			}
		}
	}
}

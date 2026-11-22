package sim

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// RelationEvoSpec holds genre-decoupled configuration parameters for kinship,
// marriage dowries, blood-feud intensity/mediation, turf sovereignty, and monopoly charters.
type RelationEvoSpec struct {
	ExplicitZero            bool  `json:"explicit_zero,omitempty"`
	DefaultDowryCoins       int   `json:"default_dowry_coins,omitempty"`
	CohortWeddingDowryCoins int64 `json:"cohort_wedding_dowry_coins,omitempty"`
	DefaultBloodMoneyCoins  int64 `json:"default_blood_money_coins,omitempty"`
	CharterBaseFee          int64 `json:"charter_base_fee,omitempty"`
	CharterDurationTicks    int64 `json:"charter_duration_ticks,omitempty"`
	KickbackBasePermille    int   `json:"kickback_base_permille,omitempty"`
	ExclusiveMarginPermille int   `json:"exclusive_margin_permille,omitempty"`
	MaxGrudgeIntensity      int   `json:"max_grudge_intensity,omitempty"`
	VendettaTriggerMin      int   `json:"vendetta_trigger_min,omitempty"`
	explicitFields          map[string]bool
}

// DefaultRelationEvoSpec returns balanced genre-decoupled defaults for social and faction evolution.
func DefaultRelationEvoSpec() RelationEvoSpec {
	return RelationEvoSpec{
		DefaultDowryCoins:       4,
		CohortWeddingDowryCoins: 16,
		DefaultBloodMoneyCoins:  6,
		CharterBaseFee:          45,
		CharterDurationTicks:    7 * TicksPerDay,
		KickbackBasePermille:    140,
		ExclusiveMarginPermille: 150,
		MaxGrudgeIntensity:      100,
		VendettaTriggerMin:      25,
	}
}

// EffectiveRelationEvoSpec fills zero/invalid fields with DefaultRelationEvoSpec values.
func EffectiveRelationEvoSpec(s RelationEvoSpec) RelationEvoSpec {
	def := DefaultRelationEvoSpec()
	s.DefaultDowryCoins = resolveSpecNumber(s.DefaultDowryCoins, def.DefaultDowryCoins, s.ExplicitZero || s.explicitFields["default_dowry_coins"])
	s.CohortWeddingDowryCoins = resolveSpecNumber(s.CohortWeddingDowryCoins, def.CohortWeddingDowryCoins, s.ExplicitZero || s.explicitFields["cohort_wedding_dowry_coins"])
	s.DefaultBloodMoneyCoins = resolveSpecNumber(s.DefaultBloodMoneyCoins, def.DefaultBloodMoneyCoins, s.ExplicitZero || s.explicitFields["default_blood_money_coins"])
	s.CharterBaseFee = resolveSpecNumber(s.CharterBaseFee, def.CharterBaseFee, s.ExplicitZero || s.explicitFields["charter_base_fee"])
	s.CharterDurationTicks = resolveSpecNumber(s.CharterDurationTicks, def.CharterDurationTicks, s.ExplicitZero || s.explicitFields["charter_duration_ticks"])
	s.KickbackBasePermille = resolveSpecNumber(s.KickbackBasePermille, def.KickbackBasePermille, s.ExplicitZero || s.explicitFields["kickback_base_permille"])
	s.ExclusiveMarginPermille = resolveSpecNumber(s.ExclusiveMarginPermille, def.ExclusiveMarginPermille, s.ExplicitZero || s.explicitFields["exclusive_margin_permille"])
	s.MaxGrudgeIntensity = resolveSpecNumber(s.MaxGrudgeIntensity, def.MaxGrudgeIntensity, s.ExplicitZero || s.explicitFields["max_grudge_intensity"])
	s.VendettaTriggerMin = resolveSpecNumber(s.VendettaTriggerMin, def.VendettaTriggerMin, s.ExplicitZero || s.explicitFields["vendetta_trigger_min"])
	return s
}

// ActorKinship tracks an individual actor's marriage, bloodline, clan, sworn brotherhood, and patronage links.
type ActorKinship struct {
	ActorID     string   `json:"actor_id"`
	SpouseID    string   `json:"spouse_id,omitempty"`
	ParentIDs   []string `json:"parent_ids,omitempty"`
	ChildIDs    []string `json:"child_ids,omitempty"`
	ClanID      string   `json:"clan_id,omitempty"`
	SwornIDs    []string `json:"sworn_ids,omitempty"`
	PatronID    string   `json:"patron_id,omitempty"`
	ProtegeIDs  []string `json:"protege_ids,omitempty"`
	MarriedTick int64    `json:"married_tick,omitempty"`
}

func cloneActorKinship(k *ActorKinship) *ActorKinship {
	if k == nil {
		return nil
	}
	out := *k
	if k.ParentIDs != nil {
		out.ParentIDs = append([]string(nil), k.ParentIDs...)
	}
	if k.ChildIDs != nil {
		out.ChildIDs = append([]string(nil), k.ChildIDs...)
	}
	if k.SwornIDs != nil {
		out.SwornIDs = append([]string(nil), k.SwornIDs...)
	}
	if k.ProtegeIDs != nil {
		out.ProtegeIDs = append([]string(nil), k.ProtegeIDs...)
	}
	return &out
}

// ClanRecord represents a kinship lineage group anchored by surname/household/cohort ancestry.
type ClanRecord struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Surname         string         `json:"surname,omitempty"`
	DistrictID      string         `json:"district_id,omitempty"`
	PatriarchID     string         `json:"patriarch_id,omitempty"`
	MemberActorIDs  []string       `json:"member_actor_ids,omitempty"`
	MemberCohortIDs []string       `json:"member_cohort_ids,omitempty"`
	Prestige        int            `json:"prestige,omitempty"`
	Trust           map[string]int `json:"trust"`
}

func cloneClanRecord(c *ClanRecord) *ClanRecord {
	if c == nil {
		return nil
	}
	out := *c
	if c.MemberActorIDs != nil {
		out.MemberActorIDs = append([]string(nil), c.MemberActorIDs...)
	}
	if c.MemberCohortIDs != nil {
		out.MemberCohortIDs = append([]string(nil), c.MemberCohortIDs...)
	}
	if c.Trust != nil {
		out.Trust = cloneIntMap(c.Trust)
	}
	return &out
}

// GrudgeRecord records a directed causal blood feud or grievance preserving RootEventID.
type GrudgeRecord struct {
	ID                  string `json:"id"`
	AggrievedID         string `json:"aggrieved_id"`
	OffenderID          string `json:"offender_id"`
	AggrievedClanID     string `json:"aggrieved_clan_id,omitempty"`
	OffenderClanID      string `json:"offender_clan_id,omitempty"`
	Cause               string `json:"cause"`
	Intensity           int    `json:"intensity"`
	RootEventID         int64  `json:"root_event_id"`
	SourceEventID       int64  `json:"source_event_id"`
	CreatedTick         int64  `json:"created_tick"`
	LastActionTick      int64  `json:"last_action_tick,omitempty"`
	Resolved            bool   `json:"resolved,omitempty"`
	Resolution          string `json:"resolution,omitempty"`
	BloodMoneyPaid      int64  `json:"blood_money_paid,omitempty"`
	MediationProposerID string `json:"mediation_proposer_id,omitempty"`
	MediationCoins      int64  `json:"mediation_coins,omitempty"`
	OffenderAccepted    bool   `json:"offender_accepted,omitempty"`
	AggrievedAccepted   bool   `json:"aggrieved_accepted,omitempty"`
}

// DistrictSovereignty tracks which faction currently controls a district and when control shifted.
type DistrictSovereignty struct {
	DistrictID         string `json:"district_id"`
	ControllingFaction string `json:"controlling_faction,omitempty"`
	ControlSinceTick   int64  `json:"control_since_tick,omitempty"`
	LastShiftTick      int64  `json:"last_shift_tick,omitempty"`
	ContestedCount     int    `json:"contested_count,omitempty"`
	ControllerScore    int    `json:"controller_score,omitempty"`
}

// MonopolyCharter tracks an exclusive Tier-2/3 tradeable item license granted to a faction.
type MonopolyCharter struct {
	ItemID                  string `json:"item_id"`
	FactionID               string `json:"faction_id"`
	DistrictID              string `json:"district_id,omitempty"`
	GrantedTick             int64  `json:"granted_tick"`
	ExpiresTick             int64  `json:"expires_tick"`
	CharterFee              int64  `json:"charter_fee"`
	KickbackPaid            int64  `json:"kickback_paid,omitempty"`
	CollectorCohortID       string `json:"collector_cohort_id,omitempty"`
	ExclusiveMarginPermille int    `json:"exclusive_margin_permille,omitempty"`
	Active                  bool   `json:"active"`
}

// MentorshipBond tracks an established mentor-apprentice instructional relationship.
type MentorshipBond struct {
	MentorID        string `json:"mentor_id"`
	ApprenticeID    string `json:"apprentice_id"`
	TopicID         string `json:"topic_id,omitempty"`
	EstablishedTick int64  `json:"established_tick"`
	Lessons         int    `json:"lessons"`
}

// RelationEvoState holds all JSON-serializable kinship, mentorship, vendetta, patronage,
// turf sovereignty, faction leadership, and monopoly charter state.
type RelationEvoState struct {
	Spec                RelationEvoSpec                `json:"spec,omitempty"`
	Seeded              bool                           `json:"seeded,omitempty"`
	NextGrudgeSeq       int64                          `json:"next_grudge_seq,omitempty"`
	Kinship             map[string]*ActorKinship       `json:"kinship"`
	Clans               map[string]*ClanRecord         `json:"clans"`
	CohortClans         map[string]string              `json:"cohort_clans"`
	Mentorships         []MentorshipBond               `json:"mentorships,omitempty"`
	Grudges             []GrudgeRecord                 `json:"grudges,omitempty"`
	Offenses            map[string]*OffenseRecord      `json:"offenses,omitempty"`
	DistrictSovereignty map[string]DistrictSovereignty `json:"district_sovereignty"`
	ControllingFaction  map[string]string              `json:"controlling_faction"`
	FactionLeader       map[string]string              `json:"faction_leader"`
	MonopolyCharters    map[string]MonopolyCharter     `json:"monopoly_charters"`
	ObservedEventIDs    map[int64]bool                 `json:"observed_event_ids"`
}

// NewRelationEvoState creates an initialized RelationEvoState with default parameters.
func NewRelationEvoState() *RelationEvoState {
	return &RelationEvoState{
		Spec:                DefaultRelationEvoSpec(),
		Kinship:             map[string]*ActorKinship{},
		Clans:               map[string]*ClanRecord{},
		CohortClans:         map[string]string{},
		DistrictSovereignty: map[string]DistrictSovereignty{},
		ControllingFaction:  map[string]string{},
		FactionLeader:       map[string]string{},
		MonopolyCharters:    map[string]MonopolyCharter{},
		ObservedEventIDs:    map[int64]bool{},
	}
}

// CloneRelationEvoState returns a deep copy of s, safely returning nil if s is nil.
func CloneRelationEvoState(s *RelationEvoState) *RelationEvoState {
	if s == nil {
		return nil
	}
	out := *s
	if s.Kinship != nil {
		out.Kinship = make(map[string]*ActorKinship, len(s.Kinship))
		for k, v := range s.Kinship {
			out.Kinship[k] = cloneActorKinship(v)
		}
	}
	if s.Clans != nil {
		out.Clans = make(map[string]*ClanRecord, len(s.Clans))
		for k, v := range s.Clans {
			out.Clans[k] = cloneClanRecord(v)
		}
	}
	if s.CohortClans != nil {
		out.CohortClans = cloneStringMap(s.CohortClans)
	}
	if s.Mentorships != nil {
		out.Mentorships = append([]MentorshipBond(nil), s.Mentorships...)
	}
	if s.Grudges != nil {
		out.Grudges = append([]GrudgeRecord(nil), s.Grudges...)
	}
	if s.Offenses != nil {
		out.Offenses = make(map[string]*OffenseRecord, len(s.Offenses))
		for k, v := range s.Offenses {
			if v == nil {
				continue
			}
			cp := *v
			out.Offenses[k] = &cp
		}
	}
	if s.DistrictSovereignty != nil {
		out.DistrictSovereignty = make(map[string]DistrictSovereignty, len(s.DistrictSovereignty))
		for k, v := range s.DistrictSovereignty {
			out.DistrictSovereignty[k] = v
		}
	}
	if s.ControllingFaction != nil {
		out.ControllingFaction = cloneStringMap(s.ControllingFaction)
	}
	if s.FactionLeader != nil {
		out.FactionLeader = cloneStringMap(s.FactionLeader)
	}
	if s.MonopolyCharters != nil {
		out.MonopolyCharters = make(map[string]MonopolyCharter, len(s.MonopolyCharters))
		for k, v := range s.MonopolyCharters {
			out.MonopolyCharters[k] = v
		}
	}
	if s.ObservedEventIDs != nil {
		out.ObservedEventIDs = make(map[int64]bool, len(s.ObservedEventIDs))
		for k, v := range s.ObservedEventIDs {
			out.ObservedEventIDs[k] = v
		}
	}
	return &out
}

func (rs *RelationEvoState) ensureMaps() {
	if rs.Kinship == nil {
		rs.Kinship = map[string]*ActorKinship{}
	}
	if rs.Clans == nil {
		rs.Clans = map[string]*ClanRecord{}
	}
	if rs.CohortClans == nil {
		rs.CohortClans = map[string]string{}
	}
	if rs.DistrictSovereignty == nil {
		rs.DistrictSovereignty = map[string]DistrictSovereignty{}
	}
	if rs.ControllingFaction == nil {
		rs.ControllingFaction = map[string]string{}
	}
	if rs.FactionLeader == nil {
		rs.FactionLeader = map[string]string{}
	}
	if rs.MonopolyCharters == nil {
		rs.MonopolyCharters = map[string]MonopolyCharter{}
	}
	if rs.ObservedEventIDs == nil {
		rs.ObservedEventIDs = map[int64]bool{}
	}
}

func (rs *RelationEvoState) effectiveSpec() RelationEvoSpec {
	if rs == nil {
		return DefaultRelationEvoSpec()
	}
	return EffectiveRelationEvoSpec(rs.Spec)
}

func (rs *RelationEvoState) markObserved(eventID int64) bool {
	if eventID <= 0 {
		return false
	}
	if rs.ObservedEventIDs[eventID] {
		return true
	}
	rs.ObservedEventIDs[eventID] = true
	if len(rs.ObservedEventIDs) > 512 {
		cutoff := eventID - 256
		for k := range rs.ObservedEventIDs {
			if k < cutoff {
				delete(rs.ObservedEventIDs, k)
			}
		}
	}
	return false
}

// DistrictController returns the controlling faction ID for districtID, or "" if unclaimed.
func (rs *RelationEvoState) DistrictController(districtID string) string {
	if rs == nil {
		return ""
	}
	if fID := rs.ControllingFaction[districtID]; fID != "" {
		return fID
	}
	if ds, ok := rs.DistrictSovereignty[districtID]; ok {
		return ds.ControllingFaction
	}
	return ""
}

// ActiveCharterForItem returns an active MonopolyCharter for itemID at tick, if any.
func (rs *RelationEvoState) ActiveCharterForItem(itemID string, tick int64) (MonopolyCharter, bool) {
	if rs == nil || itemID == "" {
		return MonopolyCharter{}, false
	}
	ch, ok := rs.MonopolyCharters[itemID]
	if !ok || !ch.Active || ch.ExpiresTick <= tick {
		return MonopolyCharter{}, false
	}
	return ch, true
}

func extractSurname(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "clan"
	}
	fields := strings.Fields(trimmed)
	if len(fields) >= 2 {
		last := strings.TrimRight(fields[len(fields)-1], "0123456789-#_.")
		if last != "" {
			return last
		}
		first := strings.TrimRight(fields[0], "0123456789-#_.")
		if first != "" {
			return first
		}
	}
	runes := []rune(trimmed)
	if len(runes) > 0 && unicode.Is(unicode.Han, runes[0]) {
		return string(runes[0])
	}
	clean := strings.TrimRightFunc(trimmed, func(r rune) bool {
		return unicode.IsDigit(r) || r == '-' || r == '_' || r == '·' || r == '#'
	})
	if clean == "" {
		return trimmed
	}
	if idx := strings.IndexAny(clean, "-_·."); idx > 0 {
		return clean[:idx]
	}
	return clean
}

func relPayloadString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func (rs *RelationEvoState) ensureActorKinship(w *World, a Actor) *ActorKinship {
	k, ok := rs.Kinship[a.ID]
	if !ok || k == nil {
		k = &ActorKinship{ActorID: a.ID}
		rs.Kinship[a.ID] = k
	}
	if k.ClanID == "" {
		surname := extractSurname(a.Name)
		clanID := "clan_" + strings.ToLower(surname)
		if surname == "" {
			clanID = "clan_unaffiliated"
		}
		clan, exists := rs.Clans[clanID]
		if !exists || clan == nil {
			loc := a.Location
			if a.HomeID != "" {
				if h := w.house(a.HomeID); h != nil && h.District != "" {
					loc = h.District
				}
			}
			clan = &ClanRecord{
				ID:         clanID,
				Name:       surname + " Lineage",
				Surname:    surname,
				DistrictID: loc,
				Prestige:   10,
				Trust:      map[string]int{},
			}
			rs.Clans[clanID] = clan
		}
		if clan.Trust == nil {
			clan.Trust = map[string]int{}
		}
		clan.MemberActorIDs = appendUnique(clan.MemberActorIDs, a.ID)
		k.ClanID = clanID
	}
	return k
}

func (rs *RelationEvoState) refreshClanPatriarch(w *World, clan *ClanRecord) {
	if clan == nil || w == nil {
		return
	}
	if clan.PatriarchID != "" {
		if p := w.actor(clan.PatriarchID); p != nil && !p.Dead {
			return
		}
	}
	var best *Actor
	bestScore := -1
	for _, id := range clan.MemberActorIDs {
		a := w.actor(id)
		if a == nil || a.Dead {
			continue
		}
		score := a.AgeYears*2 + a.Level*6 + a.Attributes.Spirit
		if best == nil || score > bestScore || (score == bestScore && a.ID < best.ID) {
			best = a
			bestScore = score
		}
	}
	if best != nil {
		clan.PatriarchID = best.ID
	} else {
		clan.PatriarchID = ""
	}
}

func (rs *RelationEvoState) clanTrustDelta(clanA, clanB string, delta int) {
	if clanA == "" || clanB == "" || clanA == clanB {
		return
	}
	cA := rs.Clans[clanA]
	cB := rs.Clans[clanB]
	if cA != nil {
		adjustTrust(&cA.Trust, clanB, delta)
	}
	if cB != nil {
		adjustTrust(&cB.Trust, clanA, delta)
	}
}

func (rs *RelationEvoState) factionStrengthInDistrict(w *World, factionID, districtID string) int {
	f := w.faction(factionID)
	if f == nil {
		return 0
	}
	score := 0
	if f.District == districtID {
		score += 20 + f.Level*12 + f.Influence/12
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 || c.FactionID != factionID || c.District != districtID {
			continue
		}
		eff := c.EffectivePopulation()
		mult := 1 + c.Tier + c.ProfLevel/2
		if w.Catalog.HasCoercivePower([]string{c.Profession}) {
			mult += 2
		}
		score += eff * mult
	}
	for _, mID := range f.Members {
		a := w.actor(mID)
		if a == nil || a.Dead || w.Tick < a.JailedUntil {
			continue
		}
		if a.Location == districtID && !a.InTransit() {
			score += 12 + a.Level*4
		}
	}
	if leaderID := rs.FactionLeader[factionID]; leaderID != "" {
		if leader := w.actor(leaderID); leader != nil && !leader.Dead && leader.Location == districtID {
			score += 10 + leader.Level*3
		}
	}
	return score
}

// setDistrictControl updates both social history and the location record used by
// laws, taxes, perception, and relief. One district's conquest cannot replace the
// old polity's authority over its other districts.
func (w *World) setDistrictControl(rs *RelationEvoState, districtID, factionID string, score int) {
	rs.ControllingFaction[districtID] = factionID
	ds := rs.DistrictSovereignty[districtID]
	ds.DistrictID, ds.ControllingFaction = districtID, factionID
	ds.ControlSinceTick, ds.LastShiftTick = w.Tick, w.Tick
	ds.ContestedCount++
	ds.ControllerScore = score
	rs.DistrictSovereignty[districtID] = ds
	if w.Polities == nil {
		w.Polities = &PolityNetwork{}
	}
	if w.Polities.NodeByID(factionID) == nil {
		f := w.faction(factionID)
		w.Polities.Nodes = append(w.Polities.Nodes, PolityNode{ID: factionID, Name: f.Name, Level: "faction", SeatDistrictID: f.District, LeaderActorID: rs.FactionLeader[factionID]})
	}
	for i := range w.Map.Districts {
		if w.Map.Districts[i].ID == districtID {
			w.Map.Districts[i].PolityID = factionID
		}
	}
	for i := range w.Map.Venues {
		if w.Map.Venues[i].DistrictID == districtID {
			w.Map.Venues[i].PolityID = factionID
		}
	}
}

func (rs *RelationEvoState) actorLeadershipScore(w *World, a *Actor) int {
	if a == nil || a.Dead || w.Tick < a.JailedUntil {
		return -1
	}
	maxProf := 0
	for _, lv := range a.Professions {
		if lv > maxProf {
			maxProf = lv
		}
	}
	prestige := 0
	if k := rs.Kinship[a.ID]; k != nil && k.ClanID != "" {
		if clan := rs.Clans[k.ClanID]; clan != nil {
			prestige = clan.Prestige / 4
		}
		prestige += len(k.ProtegeIDs)*4 + len(k.SwornIDs)*2
	}
	penalty := 0
	if a.Health < 40 {
		penalty += 10
	}
	return a.Level*10 + maxProf*8 + a.Attributes.Spirit*2 + a.Attributes.Insight + prestige - penalty
}

// EnsureKinshipSeeded idempotently initializes actor kinship nodes, surname/cohort clans,
// district sovereignty, and faction leadership from the current World state.
func (rs *RelationEvoState) EnsureKinshipSeeded(w *World) {
	if rs == nil || w == nil {
		return
	}
	rs.ensureMaps()

	for i := range w.Actors {
		rs.ensureActorKinship(w, w.Actors[i])
	}

	// Link cohort lineages to district clans.
	clanIDs := make([]string, 0, len(rs.Clans))
	for id := range rs.Clans {
		clanIDs = append(clanIDs, id)
	}
	sort.Strings(clanIDs)

	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		if existing := rs.CohortClans[c.ID]; existing != "" && rs.Clans[existing] != nil {
			rs.Clans[existing].MemberCohortIDs = appendUnique(rs.Clans[existing].MemberCohortIDs, c.ID)
			continue
		}
		var districtClans []string
		for _, id := range clanIDs {
			if rs.Clans[id].DistrictID == c.District {
				districtClans = append(districtClans, id)
			}
		}
		var chosenClanID string
		if len(districtClans) > 0 {
			idx := int(worldHash(w.ID, 0, "cohort_clan:"+c.ID) % uint64(len(districtClans)))
			chosenClanID = districtClans[idx]
		} else {
			chosenClanID = "clan_district_" + c.District
			if rs.Clans[chosenClanID] == nil {
				rs.Clans[chosenClanID] = &ClanRecord{
					ID:         chosenClanID,
					Name:       c.District + " Lineage",
					Surname:    c.District,
					DistrictID: c.District,
					Prestige:   8,
					Trust:      map[string]int{},
				}
				clanIDs = append(clanIDs, chosenClanID)
				sort.Strings(clanIDs)
			}
		}
		rs.CohortClans[c.ID] = chosenClanID
		rs.Clans[chosenClanID].MemberCohortIDs = appendUnique(rs.Clans[chosenClanID].MemberCohortIDs, c.ID)
	}

	for _, id := range clanIDs {
		rs.refreshClanPatriarch(w, rs.Clans[id])
	}

	// Seed district sovereignty from strongest local faction if not yet assigned.
	for _, d := range w.Map.Districts {
		if _, exists := rs.DistrictSovereignty[d.ID]; exists {
			continue
		}
		if pol := w.EffectiveGoverningPolity(d.ID, ""); pol != nil {
			rs.ControllingFaction[d.ID] = pol.ID
		}
		if rs.ControllingFaction[d.ID] != "" {
			rs.DistrictSovereignty[d.ID] = DistrictSovereignty{
				DistrictID:         d.ID,
				ControllingFaction: rs.ControllingFaction[d.ID],
				ControlSinceTick:   w.Tick,
			}
			continue
		}
		bestFaction := ""
		bestScore := 0
		for i := range w.Factions {
			f := &w.Factions[i]
			sc := rs.factionStrengthInDistrict(w, f.ID, d.ID)
			if f.District != d.ID && sc < 25 {
				continue
			}
			if sc > bestScore || (sc == bestScore && f.ID < bestFaction) {
				bestScore = sc
				bestFaction = f.ID
			}
		}
		rs.ControllingFaction[d.ID] = bestFaction
		rs.DistrictSovereignty[d.ID] = DistrictSovereignty{
			DistrictID:         d.ID,
			ControllingFaction: bestFaction,
			ControlSinceTick:   w.Tick,
			ControllerScore:    bestScore,
		}
	}

	// Seed faction leadership.
	for i := range w.Factions {
		f := &w.Factions[i]
		if _, seeded := rs.FactionLeader[f.ID]; seeded {
			continue
		}
		bestID := ""
		bestScore := -1
		for _, mID := range f.Members {
			if rs.isLeaderOfOtherFaction(f.ID, mID) {
				continue
			}
			a := w.actor(mID)
			sc := rs.actorLeadershipScore(w, a)
			if sc >= 0 && (sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID))) {
				bestScore = sc
				bestID = mID
			}
		}
		if bestID != "" {
			rs.FactionLeader[f.ID] = bestID
		}
	}

	rs.Seeded = true
}

func (rs *RelationEvoState) isLeaderOfOtherFaction(factionID, actorID string) bool {
	if rs == nil || actorID == "" {
		return false
	}
	for fID, leaderID := range rs.FactionLeader {
		if fID != factionID && leaderID == actorID {
			return true
		}
	}
	return false
}

func (rs *RelationEvoState) hasUnresolvedGrudge(aggrievedID, offenderID string) bool {
	if rs == nil || aggrievedID == "" || offenderID == "" {
		return false
	}
	for _, g := range rs.Grudges {
		if !g.Resolved && g.AggrievedID == aggrievedID && g.OffenderID == offenderID {
			return true
		}
	}
	return false
}

func (rs *RelationEvoState) entityClanID(w *World, entityID string) string {
	if rs == nil || entityID == "" {
		return ""
	}
	if k := rs.Kinship[entityID]; k != nil && k.ClanID != "" {
		return k.ClanID
	}
	if cID := rs.CohortClans[entityID]; cID != "" {
		return cID
	}
	if rs.Clans[entityID] != nil {
		return entityID
	}
	_ = w
	return ""
}

func (rs *RelationEvoState) recordGrudge(tx *WorldTx, w *World, aggrievedID, offenderID, cause string, intensity int, rootEventID, sourceEventID int64) []Event {
	if rs == nil || w == nil || aggrievedID == "" || offenderID == "" || aggrievedID == offenderID {
		return nil
	}
	spec := rs.effectiveSpec()
	aggClan := rs.entityClanID(w, aggrievedID)
	offClan := rs.entityClanID(w, offenderID)

	for i := range rs.Grudges {
		g := &rs.Grudges[i]
		if g.Resolved {
			continue
		}
		if g.AggrievedID == aggrievedID && g.OffenderID == offenderID {
			g.Intensity = minInt(spec.MaxGrudgeIntensity, g.Intensity+maxInt(5, intensity/2))
			if g.RootEventID == 0 && rootEventID > 0 {
				g.RootEventID = rootEventID
			}
			g.SourceEventID = sourceEventID
			return nil
		}
	}

	rs.NextGrudgeSeq++
	if rootEventID <= 0 {
		rootEventID = sourceEventID
	}
	grudgeID := fmt.Sprintf("grudge_%d", rs.NextGrudgeSeq)
	g := GrudgeRecord{
		ID:              grudgeID,
		AggrievedID:     aggrievedID,
		OffenderID:      offenderID,
		AggrievedClanID: aggClan,
		OffenderClanID:  offClan,
		Cause:           cause,
		Intensity:       minInt(spec.MaxGrudgeIntensity, maxInt(1, intensity)),
		RootEventID:     rootEventID,
		SourceEventID:   sourceEventID,
		CreatedTick:     w.Tick,
	}
	rs.Grudges = append(rs.Grudges, g)

	if a := w.actor(aggrievedID); a != nil {
		if a.Trust == nil {
			a.Trust = map[string]int{}
		}
		tx.MutateTrust(a, offenderID, -3)
	}
	rs.clanTrustDelta(aggClan, offClan, -2)

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  aggrievedID,
		TargetID: offenderID,
		Kind:     "grudge_formed",
		Action:   Action{Kind: "speak", TargetID: offenderID, BasisEventID: sourceEventID},
		Payload: map[string]any{
			"text":              fmt.Sprintf("%s recorded a blood grievance against %s over %s (intensity %d)", aggrievedID, offenderID, cause, g.Intensity),
			"grudge_id":         g.ID,
			"aggrieved_id":      g.AggrievedID,
			"offender_id":       g.OffenderID,
			"aggrieved_clan_id": g.AggrievedClanID,
			"offender_clan_id":  g.OffenderClanID,
			"cause":             g.Cause,
			"intensity":         g.Intensity,
			"root_event_id":     g.RootEventID,
			"source_event_id":   g.SourceEventID,
		},
	}
	w.NextID++
	return []Event{ev}
}

func (rs *RelationEvoState) bondedToDeceased(listenerID, victimID string) bool {
	if ActorHasTag(rs.mentorshipPartners(victimID), listenerID) {
		return true
	}
	// The deceased's spouse link remains on record after the survivor's is cleared.
	k := rs.Kinship[listenerID]
	if vk := rs.Kinship[victimID]; vk != nil {
		if vk.SpouseID == listenerID {
			return true
		}
		if k != nil {
			for _, parent := range vk.ParentIDs {
				if ActorHasTag(k.ParentIDs, parent) {
					return true
				}
			}
		}
	}
	return k != nil && (k.SpouseID == victimID || k.PatronID == victimID ||
		ActorHasTag(k.ParentIDs, victimID) || ActorHasTag(k.ChildIDs, victimID) ||
		ActorHasTag(k.SwornIDs, victimID) || ActorHasTag(k.ProtegeIDs, victimID))
}

func (rs *RelationEvoState) recordHomicideGrudge(tx *WorldTx, w *World, aggrievedID, killerID string, rootID, deathID, reportID int64) []Event {
	listener := w.actor(aggrievedID)
	if listener == nil || listener.Dead || aggrievedID == killerID || slices.Contains(listener.KnownHomicides, deathID) {
		return nil
	}
	listener.KnownHomicides = append(listener.KnownHomicides, deathID)
	cause, intensity := "homicide", 70
	if reportID != deathID {
		cause, intensity = "homicide_reported", 65
		if listener.Trust == nil {
			listener.Trust = map[string]int{}
		}
		tx.MutateTrust(listener, killerID, -4)
	}
	return rs.recordGrudge(tx, w, aggrievedID, killerID, cause, intensity, rootID, reportID)
}

func (rs *RelationEvoState) settleDeathAndInheritance(tx *WorldTx, w *World, e Event) []Event {
	deceasedID := e.ActorID
	if deceasedID == "" {
		return nil
	}
	deceased := w.actor(deceasedID)
	if deceased == nil || deceased.EstateSettled {
		return nil
	}
	deceased.EstateSettled = true
	// Equipped items use an independent slot and can be inherited as inventory.
	if len(deceased.Equipped) > 0 {
		if deceased.Items == nil {
			deceased.Items = map[string]int{}
		}
		for _, slot := range sortedProductionKeys(deceased.Equipped) {
			item := deceased.Equipped[slot]
			deceased.Items[item]++
			tx.recordTransfer("transfer_item", deceased.ID, slot, item, 1, "inheritance")
		}
		deceased.Equipped = nil
	}
	var events []Event
	k := rs.ensureActorKinship(w, *deceased)

	// Clear spousal marriage link on surviving spouse.
	if k.SpouseID != "" {
		if sk := rs.Kinship[k.SpouseID]; sk != nil && sk.SpouseID == deceasedID {
			sk.SpouseID = ""
		}
	}

	// Refresh clan patriarch and faction leadership.
	var clan *ClanRecord
	if k.ClanID != "" {
		clan = rs.Clans[k.ClanID]
		if clan != nil && clan.PatriarchID == deceasedID {
			clan.PatriarchID = ""
			rs.refreshClanPatriarch(w, clan)
		}
	}
	for i := range w.Factions {
		f := &w.Factions[i]
		if rs.FactionLeader[f.ID] == deceasedID {
			prev := deceasedID
			bestID := ""
			bestScore := -1
			for _, mID := range f.Members {
				if rs.isLeaderOfOtherFaction(f.ID, mID) {
					continue
				}
				a := w.actor(mID)
				sc := rs.actorLeadershipScore(w, a)
				if sc >= 0 && (sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID))) {
					bestScore = sc
					bestID = mID
				}
			}
			w.setFactionLeader(f.ID, bestID)
			if bestID != "" {
				ev := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  bestID,
					TargetID: prev,
					Kind:     "faction_leader_changed",
					Action:   Action{Kind: "wait", FactionID: f.ID},
					Payload: map[string]any{
						"text":            fmt.Sprintf("%s succeeded the late %s as leader of %s", bestID, prev, f.Name),
						"faction_id":      f.ID,
						"previous_leader": prev,
						"new_leader":      bestID,
						"reason":          "succession",
						"root_event_id":   payloadInt64(e.Payload, "root_event_id"),
						"cause_event_id":  e.ID,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}
	}

	// Collect living heirs and check for low-trust inheritance disputes.
	var claimants []*Actor
	seen := map[string]bool{}
	addClaimant := func(id string) {
		if id == "" || id == deceasedID || seen[id] {
			return
		}
		if a := w.actor(id); a != nil && !a.Dead {
			seen[id] = true
			claimants = append(claimants, a)
		}
	}
	for _, id := range k.ChildIDs {
		addClaimant(id)
	}
	addClaimant(k.SpouseID)
	for _, id := range k.ProtegeIDs {
		addClaimant(id)
	}
	for _, id := range k.SwornIDs {
		addClaimant(id)
	}
	if clan != nil {
		for _, id := range clan.MemberActorIDs {
			addClaimant(id)
		}
	}

	var ownedHouses []int
	for hi := range w.Houses {
		if w.Houses[hi].OwnerID == deceasedID {
			ownedHouses = append(ownedHouses, hi)
		}
	}

	if len(claimants) > 0 {
		primaryHeir := claimants[0]
		var rivalHeir *Actor
		lowTrust := false
		for _, c := range claimants {
			if deceased.Trust != nil && deceased.Trust[c.ID] < 0 {
				lowTrust = true
			}
			if c.ID != primaryHeir.ID {
				t1 := 0
				if primaryHeir.Trust != nil {
					t1 = primaryHeir.Trust[c.ID]
				}
				t2 := 0
				if c.Trust != nil {
					t2 = c.Trust[primaryHeir.ID]
				}
				if t1 < 0 || t2 < 0 {
					lowTrust = true
					if rivalHeir == nil {
						rivalHeir = c
					}
				}
			}
		}

		// Settle estate (0-error coin & item conservation).
		inheritedCoins := deceased.Money
		if inheritedCoins > 0 {
			tx.TransferCurrency(deceased, primaryHeir, "coins", int64(inheritedCoins), "inheritance")
		}
		if len(deceased.Currencies) > 0 {
			curKeys := make([]string, 0, len(deceased.Currencies))
			for cur, amt := range deceased.Currencies {
				if amt > 0 {
					curKeys = append(curKeys, cur)
				}
			}
			sort.Strings(curKeys)
			for _, cur := range curKeys {
				amt := deceased.Currencies[cur]
				w.addActorCurrency(primaryHeir, cur, amt)
				tx.recordTransfer("transfer_currency", deceased.ID, primaryHeir.ID, cur, -amt, "inheritance")
			}
			deceased.Currencies = nil
		}
		if primaryHeir.Items == nil {
			primaryHeir.Items = map[string]int{}
		}
		itemKeys := make([]string, 0, len(deceased.Items))
		for itemID, qty := range deceased.Items {
			if qty > 0 {
				itemKeys = append(itemKeys, itemID)
			}
		}
		sort.Strings(itemKeys)
		for _, itemID := range itemKeys {
			tx.TransferItem(deceased, primaryHeir, itemID, deceased.Items[itemID], "inheritance")
		}
		w.transferGrowingAssets("actor", deceasedID, "actor", primaryHeir.ID)
		firstHouseID := ""
		for _, hi := range ownedHouses {
			w.Houses[hi].OwnerID = primaryHeir.ID
			w.Houses[hi].Residents = appendUnique(w.Houses[hi].Residents, primaryHeir.ID)
			if firstHouseID == "" {
				firstHouseID = w.Houses[hi].ID
			}
		}

		if lowTrust && rivalHeir != nil {
			tx.MutateTrust(primaryHeir, rivalHeir.ID, -2)
			tx.MutateTrust(rivalHeir, primaryHeir.ID, -2)
			dispEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  rivalHeir.ID,
				TargetID: primaryHeir.ID,
				Kind:     "inheritance_dispute",
				Action:   Action{Kind: "speak", TargetID: primaryHeir.ID, HouseID: firstHouseID, BasisEventID: e.ID},
				Payload: map[string]any{
					"text":          fmt.Sprintf("%s contested %s's inheritance of %s's estate (%d coins, house %s)", rivalHeir.Name, primaryHeir.Name, deceased.Name, inheritedCoins, firstHouseID),
					"deceased_id":   deceasedID,
					"heir_id":       primaryHeir.ID,
					"rival_id":      rivalHeir.ID,
					"house_id":      firstHouseID,
					"coins":         inheritedCoins,
					"clan_id":       k.ClanID,
					"root_event_id": e.ID,
				},
			}
			w.NextID++
			dispMem := Memory{Kind: dispEv.Kind, EventID: dispEv.ID, RootEventID: e.ID, CauseEventID: e.ID, Tick: w.Tick, Source: rivalHeir.ID, TargetID: primaryHeir.ID, Text: relPayloadString(dispEv.Payload, "text")}
			tx.RecordMemory(primaryHeir, dispMem)
			tx.RecordMemory(rivalHeir, dispMem)
			events = append(events, dispEv)
			events = append(events, rs.recordGrudge(tx, w, rivalHeir.ID, primaryHeir.ID, "inheritance_dispute", 35, e.ID, dispEv.ID)...)
		} else {
			settleEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  primaryHeir.ID,
				TargetID: deceasedID,
				Kind:     "inheritance_settled",
				Action:   Action{Kind: "wait", HouseID: firstHouseID, BasisEventID: e.ID},
				Payload: map[string]any{
					"text":          fmt.Sprintf("%s peacefully inherited %s's estate (%d coins, house %s)", primaryHeir.Name, deceased.Name, inheritedCoins, firstHouseID),
					"deceased_id":   deceasedID,
					"heir_id":       primaryHeir.ID,
					"house_id":      firstHouseID,
					"coins":         inheritedCoins,
					"clan_id":       k.ClanID,
					"root_event_id": e.ID,
				},
			}
			w.NextID++
			tx.RecordMemory(primaryHeir, Memory{Kind: settleEv.Kind, EventID: settleEv.ID, RootEventID: e.ID, CauseEventID: e.ID, Tick: w.Tick, Source: primaryHeir.ID, TargetID: deceasedID, Text: relPayloadString(settleEv.Payload, "text")})
			events = append(events, settleEv)
		}
	} else {
		// Only an estate with no living claimant escheats to the public treasury.
		if deceased.Money > 0 {
			w.creditTreasury(int64(deceased.Money), "confiscated")
			tx.recordTransfer("transfer_currency", deceased.ID, "treasury", "coins", -int64(deceased.Money), "inheritance")
			w.adjustActorCurrency(deceased, "coins", -int64(deceased.Money), false)
		}
		w.settleUnclaimedActorEstate(tx, deceased, ownedHouses)
	}

	if killerID := relPayloadString(e.Payload, "killer_id"); killerID != "" && killerID != deceasedID {
		witnesses, _ := extractStringSlicePayload(e.Payload, "witnesses")
		rootID := payloadInt64(e.Payload, "root_event_id")
		if rootID == 0 {
			rootID = e.ID
		}
		for _, id := range witnesses {
			if rs.bondedToDeceased(id, deceasedID) {
				events = append(events, rs.recordHomicideGrudge(tx, w, id, killerID, rootID, e.ID, e.ID)...)
			}
		}
	}
	return events
}

func extractStringSlicePayload(payload map[string]any, key string) ([]string, bool) {
	if payload == nil {
		return nil, false
	}
	raw, exists := payload[key]
	if !exists {
		return nil, false
	}
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, true
	}
	return nil, true
}

// RecordMentorship creates or deepens a formal mentor-apprentice bond when knowledge/skills are taught.
func (rs *RelationEvoState) RecordMentorship(w *World, mentor, apprentice *Actor, topicID string) []Event {
	return rs.recordMentorshipWithTx(nil, w, mentor, apprentice, topicID)
}

func (rs *RelationEvoState) recordMentorshipWithTx(tx *WorldTx, w *World, mentor, apprentice *Actor, topicID string) []Event {
	if rs == nil || w == nil || mentor == nil || apprentice == nil || mentor.ID == "" || apprentice.ID == "" || mentor.ID == apprentice.ID {
		return nil
	}
	rs.EnsureKinshipSeeded(w)
	for i := range rs.Mentorships {
		b := &rs.Mentorships[i]
		if b.MentorID == mentor.ID && b.ApprenticeID == apprentice.ID {
			b.Lessons++
			if b.TopicID == "" && topicID != "" {
				b.TopicID = topicID
			}
			return nil
		}
	}
	rs.Mentorships = append(rs.Mentorships, MentorshipBond{
		MentorID:        mentor.ID,
		ApprenticeID:    apprentice.ID,
		TopicID:         topicID,
		EstablishedTick: w.Tick,
		Lessons:         1,
	})
	mk := rs.ensureActorKinship(w, *mentor)
	ak := rs.ensureActorKinship(w, *apprentice)
	mk.ProtegeIDs = appendUnique(mk.ProtegeIDs, apprentice.ID)
	if ak.PatronID == "" {
		ak.PatronID = mentor.ID
	}
	if mentor.Trust == nil {
		mentor.Trust = map[string]int{}
	}
	if apprentice.Trust == nil {
		apprentice.Trust = map[string]int{}
	}
	tx.MutateTrust(mentor, apprentice.ID, 2)
	tx.MutateTrust(apprentice, mentor.ID, 3)
	ev := w.recordWithTx(tx, mentor, Action{Kind: "teach_knowledge", TargetID: apprentice.ID, SkillID: topicID}, "mentorship_formed", map[string]any{
		"text":             fmt.Sprintf("%s and %s formed a formal mentor-apprentice bond (%s)", mentor.Name, apprentice.Name, topicID),
		"mentor_id":        mentor.ID,
		"apprentice_id":    apprentice.ID,
		"topic_id":         topicID,
		"established_tick": w.Tick,
	}, apprentice)
	return []Event{ev}
}

func (rs *RelationEvoState) mentorshipPartners(actorID string) []string {
	if rs == nil || actorID == "" {
		return nil
	}
	var out []string
	for _, b := range rs.Mentorships {
		if b.MentorID == actorID && b.ApprenticeID != "" {
			out = appendUnique(out, b.ApprenticeID)
		} else if b.ApprenticeID == actorID && b.MentorID != "" {
			out = appendUnique(out, b.MentorID)
		}
	}
	return out
}

// ObserveCausalRelationEvents inspects newly settled simulation events to update lineage
// succession, settle funerals/inheritance, and record causal blood-feud grudges preserving RootEventID.
func (rs *RelationEvoState) ObserveCausalRelationEvents(w *World, events []Event) []Event {
	return rs.observeCausalRelationEventsWithTx(nil, w, events)
}

func (rs *RelationEvoState) observeCausalRelationEventsWithTx(tx *WorldTx, w *World, events []Event) []Event {
	if rs == nil || w == nil || len(events) == 0 {
		return nil
	}
	rs.EnsureKinshipSeeded(w)
	var out []Event

	for _, e := range events {
		if e.ID > 0 && rs.markObserved(e.ID) {
			continue
		}
		rootID := payloadInt64(e.Payload, "root_event_id")
		if rootID <= 0 {
			rootID = e.ID
		}
		switch e.Kind {
		case "raided":
			dmg := int(payloadInt64(e.Payload, "damage"))
			coins := int(payloadInt64(e.Payload, "coins"))
			out = append(out, rs.recordGrudge(tx, w, e.TargetID, e.ActorID, "raided", 32+dmg/2+coins, rootID, e.ID)...)
			victim := w.actor(e.TargetID)
			for _, partnerID := range rs.mentorshipPartners(e.TargetID) {
				if partnerID != e.ActorID {
					if partner := w.actor(partnerID); partner != nil && victim != nil && CanObserveActor(*partner, *victim, w.Tick) {
						tx.MutateTrust(partner, e.ActorID, -2)
					}
				}
			}
		case "attacked":
			dmg := int(payloadInt64(e.Payload, "damage"))
			out = append(out, rs.recordGrudge(tx, w, e.TargetID, e.ActorID, "attacked", 24+dmg/2, rootID, e.ID)...)
			victim := w.actor(e.TargetID)
			for _, partnerID := range rs.mentorshipPartners(e.TargetID) {
				if partnerID != e.ActorID {
					if partner := w.actor(partnerID); partner != nil && victim != nil && CanObserveActor(*partner, *victim, w.Tick) {
						tx.MutateTrust(partner, e.ActorID, -2)
					}
				}
			}
		case "extort_resisted":
			out = append(out, rs.recordGrudge(tx, w, e.TargetID, e.ActorID, "extort_resisted", 28, rootID, e.ID)...)
		case "cohort_raided":
			off := relPayloadString(e.Payload, "attacker_cohort")
			if off == "" {
				off = e.ActorID
			}
			agg := relPayloadString(e.Payload, "target_cohort")
			if agg == "" {
				agg = e.TargetID
			}
			deaths := int(payloadInt64(e.Payload, "deaths"))
			wounded := int(payloadInt64(e.Payload, "wounded"))
			out = append(out, rs.recordGrudge(tx, w, agg, off, "cohort_raided", 30+deaths*15+wounded*2, rootID, e.ID)...)
		case "cohort_racket_clashed":
			off := relPayloadString(e.Payload, "gang_cohort")
			if off == "" {
				off = e.ActorID
			}
			agg := relPayloadString(e.Payload, "target_cohort")
			if agg == "" {
				agg = e.TargetID
			}
			deaths := int(payloadInt64(e.Payload, "deaths"))
			wounded := int(payloadInt64(e.Payload, "wounded"))
			out = append(out, rs.recordGrudge(tx, w, agg, off, "cohort_racket_clashed", 28+deaths*15+wounded*2, rootID, e.ID)...)
		case "wrongful_conviction":
			agg := relPayloadString(e.Payload, "victim_id")
			if agg == "" {
				agg = relPayloadString(e.Payload, "target_cohort")
			}
			if agg == "" {
				agg = e.TargetID
			}
			off := relPayloadString(e.Payload, "framer_id")
			if off == "" {
				off = relPayloadString(e.Payload, "collector_cohort")
			}
			if off == "" {
				off = e.ActorID
			}
			out = append(out, rs.recordGrudge(tx, w, agg, off, "wrongful_conviction", 48, rootID, e.ID)...)
		case "actor_died":
			out = append(out, rs.settleDeathAndInheritance(tx, w, e)...)
		case "speech", "channel_message", "channel_transmitted":
			listeners, _ := extractStringSlicePayload(e.Payload, "delivered_to")
			stmt := e.Action.Statement
			deathID := payloadInt64(e.Payload, "fact_event_id")
			if len(listeners) == 0 || stmt == nil || stmt.Topic != "actor_died" || deathID == 0 || stmt.SubjectID == "" {
				continue
			}
			victim := w.actor(stmt.ObjectID)
			if victim == nil || victim.DeathEventID != deathID {
				continue
			}
			for _, id := range listeners {
				if listener := w.actor(id); listener != nil && listener.Trust[e.ActorID] >= -1 && rs.bondedToDeceased(id, victim.ID) {
					out = append(out, rs.recordHomicideGrudge(tx, w, id, stmt.SubjectID, rootID, deathID, e.ID)...)
				}
			}
		case "actor_succeeded":
			predID := relPayloadString(e.Payload, "predecessor_id")
			if predID == "" {
				predID = e.TargetID
			}
			succID := relPayloadString(e.Payload, "successor_id")
			if succID == "" {
				succID = e.ActorID
			}
			succ := w.actor(succID)
			if succ != nil {
				sk := rs.ensureActorKinship(w, *succ)
				if predID != "" {
					if pk := rs.Kinship[predID]; pk != nil && pk.ClanID != "" && pk.ClanID != sk.ClanID {
						if oldClan := rs.Clans[sk.ClanID]; oldClan != nil {
							oldClan.MemberActorIDs = removeString(oldClan.MemberActorIDs, succID)
						}
						sk.ClanID = pk.ClanID
						if newClan := rs.Clans[pk.ClanID]; newClan != nil {
							newClan.MemberActorIDs = appendUnique(newClan.MemberActorIDs, succID)
							rs.refreshClanPatriarch(w, newClan)
						}
					}
					sk.ParentIDs = appendUnique(sk.ParentIDs, predID)
					if pk := rs.Kinship[predID]; pk != nil {
						pk.ChildIDs = appendUnique(pk.ChildIDs, succID)
					}
				}
			}
		}
	}
	return out
}

func (rs *RelationEvoState) matchesAggrieved(a *Actor, g GrudgeRecord) bool {
	if a == nil {
		return false
	}
	if g.AggrievedID == a.ID {
		return true
	}
	k := rs.Kinship[a.ID]
	if k == nil {
		return false
	}
	if k.ClanID != "" && (g.AggrievedID == k.ClanID || g.AggrievedClanID == k.ClanID) {
		return true
	}
	if k.SpouseID == g.AggrievedID || k.PatronID == g.AggrievedID {
		return true
	}
	if ActorHasTag(k.ParentIDs, g.AggrievedID) || ActorHasTag(k.ChildIDs, g.AggrievedID) || ActorHasTag(k.SwornIDs, g.AggrievedID) || ActorHasTag(k.ProtegeIDs, g.AggrievedID) {
		return true
	}
	return false
}

func (rs *RelationEvoState) matchesOffender(b *Actor, g GrudgeRecord) bool {
	if b == nil {
		return false
	}
	if g.OffenderID == b.ID {
		return true
	}
	if k := rs.Kinship[b.ID]; k != nil && k.ClanID != "" && (g.OffenderID == k.ClanID || g.OffenderClanID == k.ClanID) {
		return true
	}
	return false
}

func (rs *RelationEvoState) findCohort(w *World, id string) *PopulationCohort {
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == id {
			return &w.Cohorts[i]
		}
	}
	return nil
}

// ApplyRelationAction handles focal actor actions: "marry", "swear_bond", "avenge_feud",
// "mediate_feud", and "claim_turf". Returns (events, true) if x.Kind is one of these actions.
func (w *World) ApplyRelationAction(state *RelationEvoState, actorID string, x Action) ([]Event, bool) {
	return w.applyRelationSubsystem(state, actorID, x)
}

func (w *World) applyRelationActionTx(tx *WorldTx, rs *RelationEvoState, a *Actor, x Action) []Event {
	emit := func(kind, text string, data map[string]any, observers ...*Actor) []Event {
		if data == nil {
			data = map[string]any{}
		}
		data["text"] = text
		return []Event{tx.record(a, x, kind, data, observers...)}
	}
	fail := func(reason string) []Event {
		return emit("action_failed", reason, nil)
	}
	if rs == nil {
		return fail("relation state unavailable")
	}
	rs.EnsureKinshipSeeded(w)
	spec := rs.effectiveSpec()
	b := w.actor(x.TargetID)

	switch x.Kind {
	case "marry":
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("prospective spouse is not nearby")
		}
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		kA := rs.ensureActorKinship(w, *a)
		kB := rs.ensureActorKinship(w, *b)
		if kA.SpouseID != "" || kB.SpouseID != "" {
			return fail("one or both actors are already married")
		}
		if ActorHasTag(kA.ParentIDs, b.ID) || ActorHasTag(kA.ChildIDs, b.ID) || ActorHasTag(kA.SwornIDs, b.ID) {
			return fail("cannot marry direct parent, child, or sworn sibling")
		}
		dowry := x.Price
		if dowry <= 0 {
			dowry = x.Quantity
		}
		if dowry <= 0 {
			dowry = minInt(a.Money/4+2, spec.DefaultDowryCoins)
		}
		if dowry < 0 {
			dowry = 0
		}
		if a.Money < dowry {
			return fail("insufficient coins for marriage dowry/bride-price")
		}
		tx.TransferCurrency(a, b, "coins", int64(dowry), x.Kind)

		kA.SpouseID = b.ID
		kB.SpouseID = a.ID
		kA.MarriedTick = w.Tick
		kB.MarriedTick = w.Tick

		mergedHouseID := a.HomeID
		if mergedHouseID != "" {
			if h := w.house(mergedHouseID); h != nil {
				h.Residents = appendUnique(h.Residents, b.ID)
				if b.HomeID == "" {
					b.HomeID = h.ID
				}
			}
		} else if b.HomeID != "" {
			mergedHouseID = b.HomeID
			if h := w.house(mergedHouseID); h != nil {
				h.Residents = appendUnique(h.Residents, a.ID)
				a.HomeID = h.ID
			}
		}

		tx.MutateTrust(a, b.ID, 4)
		tx.MutateTrust(b, a.ID, 4)
		rs.clanTrustDelta(kA.ClanID, kB.ClanID, 3)
		if clanA := rs.Clans[kA.ClanID]; clanA != nil {
			clanA.Prestige += 2
		}
		if clanB := rs.Clans[kB.ClanID]; clanB != nil {
			clanB.Prestige += 2
		}

		return emit("actor_married", fmt.Sprintf("%s and %s wed in %s (bride-price/dowry %d coins)", a.Name, b.Name, a.Location, dowry), map[string]any{
			"spouse_a":    a.ID,
			"spouse_b":    b.ID,
			"dowry_coins": dowry,
			"clan_a":      kA.ClanID,
			"clan_b":      kB.ClanID,
			"house_id":    mergedHouseID,
			"district":    a.Location,
		}, b)

	case "swear_bond":
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("target is not nearby")
		}
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		kA := rs.ensureActorKinship(w, *a)
		kB := rs.ensureActorKinship(w, *b)

		mode := strings.ToLower(strings.TrimSpace(x.Text))
		bondType := "sworn"
		if mode == "patron" || mode == "mentor" || mode == "adopt" || mode == "disciple" {
			bondType = "patron"
			if kA.PatronID == b.ID || ActorHasTag(kB.ProtegeIDs, a.ID) {
				return fail("patron bond already exists")
			}
			if kB.PatronID == a.ID || ActorHasTag(kA.ProtegeIDs, b.ID) {
				return fail("reverse patronage already exists")
			}
		} else {
			if ActorHasTag(kA.SwornIDs, b.ID) || ActorHasTag(kB.SwornIDs, a.ID) {
				return fail("sworn bond already exists")
			}
			if kA.SpouseID == b.ID || ActorHasTag(kA.ParentIDs, b.ID) || ActorHasTag(kA.ChildIDs, b.ID) {
				return fail("existing kinship supersedes sworn bond")
			}
		}

		gift := x.Price
		if gift <= 0 {
			gift = x.Quantity
		}
		if gift > 0 && a.Money < gift {
			return fail("insufficient coins for bond ceremony gift")
		}
		if gift > 0 {
			tx.TransferCurrency(a, b, "coins", int64(gift), x.Kind)
		}

		if bondType == "patron" {
			kA.PatronID = b.ID
			kB.ProtegeIDs = appendUnique(kB.ProtegeIDs, a.ID)
		} else {
			kA.SwornIDs = appendUnique(kA.SwornIDs, b.ID)
			kB.SwornIDs = appendUnique(kB.SwornIDs, a.ID)
		}

		tx.MutateTrust(a, b.ID, 3)
		tx.MutateTrust(b, a.ID, 3)
		rs.clanTrustDelta(kA.ClanID, kB.ClanID, 1)

		return emit("bond_sworn", fmt.Sprintf("%s and %s swore a %s bond in %s", a.Name, b.Name, bondType, a.Location), map[string]any{
			"bond_type":  bondType,
			"gift_coins": gift,
			"clan_a":     kA.ClanID,
			"clan_b":     kB.ClanID,
			"district":   a.Location,
		}, b)

	case "avenge_feud":
		var grudgeIdx = -1
		for i := range rs.Grudges {
			g := rs.Grudges[i]
			if g.Resolved || !actorKnowsAboutGrudge(*a, g) {
				continue
			}
			if x.Text != "" && g.ID == x.Text && rs.matchesAggrieved(a, g) {
				grudgeIdx = i
				break
			}
			if b != nil && rs.matchesAggrieved(a, g) && rs.matchesOffender(b, g) {
				grudgeIdx = i
				break
			}
		}
		if grudgeIdx < 0 {
			return fail("no active blood feud against target")
		}
		g := &rs.Grudges[grudgeIdx]
		if b == nil {
			b = w.actor(g.OffenderID)
		}
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("feud target is not nearby")
		}
		if a.Energy < 6 {
			return fail("insufficient energy to avenge feud")
		}
		if a.Health < 25 {
			return fail("too injured to fight; must recuperate")
		}
		tx.setEnergy(a, a.Energy-6, x.Kind)
		atkPower := CombatPowerWithCatalog(w.Catalog, *a)
		defPower := CombatPowerWithCatalog(w.Catalog, *b)
		x.TargetID = b.ID
		dmg, absorbed, immune, broken := w.applyMeleeHit(tx, a, b, x, maxInt(14, minInt(75, 18+(atkPower-defPower))))
		if dmg > 0 && b.Health > 0 && a.Health > 10 {
			counter := maxInt(2, minInt(22, defPower/4))
			if counter > a.Health-1 {
				counter = a.Health - 1
			}
			tx.adjustHealth(a, -counter, 0, x.Kind)
		}
		loot := 0
		if dmg > 0 {
			loot = minInt(b.Money, 3)
		}
		if loot > 0 {
			tx.TransferCurrency(b, a, "coins", int64(loot), x.Kind)
		}
		if a.Health < 65 {
			a.RestUntil = w.Tick + 4
		}
		tx.MutateTrust(b, a.ID, -3)

		g.LastActionTick = w.Tick
		if b.Health == 0 || dmg > 0 && b.Health <= MaxHealth/3 {
			g.Resolved = true
			g.Resolution = "avenged"
		} else if dmg > 0 {
			g.Intensity = maxInt(1, g.Intensity-dmg)
		}

		avengeData := map[string]any{
			"text":           fmt.Sprintf("%s avenged grudge %s against %s (-%d health, %d restitution coins)", a.Name, g.ID, b.Name, dmg, loot),
			"grudge_id":      g.ID,
			"damage":         dmg,
			"absorbed":       absorbed,
			"immune_blocked": immune,
			"coins":          loot,
			"resolved":       g.Resolved,
		}
		if g.RootEventID > 0 {
			avengeData["cause_event_id"] = g.RootEventID
			avengeData["root_event_id"] = g.RootEventID
		}
		ev := tx.record(a, x, "feud_avenged", avengeData, b)
		events := append(broken, ev)
		events = append(events, w.finishMeleeHit(tx, a, b, x, dmg, "vendetta", ev)...)
		events = append(events, w.grantCombatXP(tx, a, x, 6, 2)...)
		return events

	case "mediate_feud":
		grudgeIdx := -1
		for i := range rs.Grudges {
			g := rs.Grudges[i]
			if g.Resolved {
				continue
			}
			if x.Text != "" {
				if g.ID == x.Text {
					grudgeIdx = i
					break
				}
				continue
			}
			if x.TargetID != "" && (g.OffenderID == x.TargetID || g.AggrievedID == x.TargetID) {
				grudgeIdx = i
				break
			}
		}
		if grudgeIdx < 0 {
			return fail("no active unresolved feud found to mediate")
		}
		g := &rs.Grudges[grudgeIdx]
		if !actorKnowsAboutGrudge(*a, *g) {
			return fail("actor has no witnessed or reported knowledge of this feud")
		}
		offender, aggrieved := w.actor(g.OffenderID), w.actor(g.AggrievedID)
		for _, party := range []*Actor{offender, aggrieved} {
			if party == nil || party.Dead || w.Tick < party.JailedUntil || !SamePlace(*a, *party) || party.Task != nil && (party.Task.Action.Kind == "sleep" || party.Task.Dormant) {
				return fail("both feud parties must be present and able to communicate")
			}
		}
		if w.actorFactionRelation(*offender, *aggrieved) == "at_war" {
			return fail("cannot mediate blood feud while factions are at war")
		}

		bloodMoney := int64(x.Price)
		if bloodMoney <= 0 {
			bloodMoney = int64(x.Quantity)
		}
		if bloodMoney <= 0 {
			bloodMoney = maxInt64(2, int64(g.Intensity/5))
		}
		prevProposer := g.MediationProposerID
		prevCoins := g.MediationCoins
		prevOffAcc := g.OffenderAccepted
		prevAggAcc := g.AggrievedAccepted
		if g.MediationProposerID == "" || g.MediationCoins != bloodMoney {
			g.MediationProposerID = a.ID
			g.MediationCoins = bloodMoney
			g.OffenderAccepted, g.AggrievedAccepted = false, false
		}
		if a.ID == g.OffenderID {
			g.OffenderAccepted = true
		}
		if a.ID == g.AggrievedID {
			g.AggrievedAccepted = true
		}
		if !g.OffenderAccepted || !g.AggrievedAccepted {
			propData := map[string]any{
				"grudge_id": g.ID, "blood_money": bloodMoney, "offender_accepted": g.OffenderAccepted, "aggrieved_accepted": g.AggrievedAccepted,
			}
			if g.RootEventID > 0 {
				propData["cause_event_id"] = g.RootEventID
				propData["root_event_id"] = g.RootEventID
			}
			return emit("feud_mediation_proposed", fmt.Sprintf("%s proposed or accepted %d blood-money coins for feud %s; awaiting both parties", a.Name, bloodMoney, g.ID), propData, offender, aggrieved)
		}
		paid := bloodMoney
		if !tx.TransferCurrency(offender, aggrieved, "coins", paid, x.Kind) {
			g.MediationProposerID = prevProposer
			g.MediationCoins = prevCoins
			g.OffenderAccepted = prevOffAcc
			g.AggrievedAccepted = prevAggAcc
			return fail("offender lacks coins for the accepted blood-money settlement")
		}

		g.Resolved = true
		g.Resolution = "mediated"
		g.BloodMoneyPaid = paid
		g.LastActionTick = w.Tick

		tx.MutateTrust(offender, g.AggrievedID, 2)
		tx.MutateTrust(aggrieved, g.OffenderID, 2)
		rs.clanTrustDelta(g.AggrievedClanID, g.OffenderClanID, 2)

		medData := map[string]any{
			"text":         fmt.Sprintf("%s mediated feud %s between %s and %s (%d blood-money coins)", a.Name, g.ID, g.AggrievedID, g.OffenderID, paid),
			"grudge_id":    g.ID,
			"aggrieved_id": g.AggrievedID,
			"offender_id":  g.OffenderID,
			"blood_money":  paid,
		}
		if g.RootEventID > 0 {
			medData["cause_event_id"] = g.RootEventID
			medData["root_event_id"] = g.RootEventID
		}
		ev := tx.record(a, x, "feud_mediated", medData, offender, aggrieved)
		return []Event{ev}

	case "claim_turf":
		districtID := x.Location
		if districtID == "" {
			districtID = a.Location
		}
		if _, ok := w.Map.DistrictByID(districtID); !ok {
			return fail("unknown district: " + districtID)
		}
		if a.Location != districtID || a.InsideHouse != "" {
			return fail("must stand outdoors in " + districtID + " to claim district turf")
		}
		factionID := x.FactionID
		if factionID == "" && len(a.Guilds) > 0 {
			factionID = a.Guilds[0]
		}
		f := w.faction(factionID)
		if f == nil || !f.HasMember(a.ID) {
			return fail("actor must belong to target faction to claim turf")
		}
		prevFaction := rs.DistrictController(districtID)
		if prevFaction == factionID {
			return fail(factionID + " already holds sovereignty over " + districtID)
		}
		if a.Energy < 5 {
			return fail("insufficient energy to contest district turf")
		}
		tx.setEnergy(a, a.Energy-5, x.Kind)
		challengerScore := rs.factionStrengthInDistrict(w, factionID, districtID) + 15
		prevScore := 0
		if prevFaction != "" {
			prevScore = rs.factionStrengthInDistrict(w, prevFaction, districtID)
		}
		if prevFaction != "" && challengerScore < prevScore {
			return emit("turf_claim_repelled", fmt.Sprintf("%s's bid for %s sovereignty in %s was repelled by %s (%d vs %d)", a.Name, f.Name, districtID, prevFaction, challengerScore, prevScore), map[string]any{
				"district":         districtID,
				"challenger":       factionID,
				"controller":       prevFaction,
				"challenger_score": challengerScore,
				"controller_score": prevScore,
			})
		}

		w.setDistrictControl(rs, districtID, factionID, challengerScore)

		f.Influence += 3
		f.Level = w.Catalog.FactionLevel(f.Influence)

		return emit("district_control_shifted", fmt.Sprintf("%s established %s sovereignty over %s (replacing %s)", a.Name, f.Name, districtID, prevFaction), map[string]any{
			"district":         districtID,
			"previous_faction": prevFaction,
			"new_faction":      factionID,
			"previous_score":   prevScore,
			"new_score":        challengerScore,
		})
	}

	return nil
}

// GrantMonopolyCharter grants factionID an exclusive Tier-2/3 tradeable item license in districtID,
// collecting fee into PublicTreasury via creditTreasury(..., "fee") and paying a corrupt kickback
// to the local collector cohort (strictly conserving TotalCoins() and TreasuryLedger).
func (w *World) GrantMonopolyCharter(rs *RelationEvoState, factionID, itemID, districtID string, fee int64) ([]Event, bool) {
	if w == nil || rs == nil || factionID == "" || itemID == "" {
		return nil, false
	}
	rs.EnsureKinshipSeeded(w)
	spec := rs.effectiveSpec()

	itemSpec, ok := w.Catalog.Items[itemID]
	if !ok || !itemSpec.Tradeable || itemSpec.Tier < 2 || itemSpec.Tier > 3 {
		return nil, false
	}
	f := w.faction(factionID)
	if f == nil {
		return nil, false
	}
	if districtID == "" {
		districtID = f.District
	}
	if fee <= 0 {
		fee = spec.CharterBaseFee * int64(itemSpec.Tier-1)
	}

	corr := 15
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[districtID]; ok {
			corr = ds.Corruption
		}
	}
	collector := w.findCollectorCohort(districtID)
	var kickback int64
	collectorID := ""
	if collector != nil {
		kickPermille := spec.KickbackBasePermille + corr*2
		if kickPermille > 350 {
			kickPermille = 350
		}
		kickback = (fee * int64(kickPermille)) / 1000
		collectorID = collector.ID
	}
	totalOutlay := fee + kickback

	// Find faction cohort (or wealthy member actor) to pay totalOutlay cleanly.
	var payerCohort *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population > 0 && c.FactionID == factionID && c.MoneyPool >= totalOutlay {
			if payerCohort == nil || c.District == districtID || c.MoneyPool > payerCohort.MoneyPool {
				payerCohort = c
				if c.District == districtID {
					break
				}
			}
		}
	}
	var payerActor *Actor
	if payerCohort == nil {
		for _, mID := range f.Members {
			if a := w.actor(mID); a != nil && !a.Dead && int64(a.Money) >= totalOutlay {
				payerActor = a
				break
			}
		}
	}
	if payerCohort == nil && payerActor == nil {
		return nil, false
	}

	if payerCohort != nil {
		payerCohort.MoneyPool -= totalOutlay
	} else {
		w.adjustActorCurrency(payerActor, "coins", -totalOutlay, false)
	}
	w.creditTreasury(fee, "fee")
	if collector != nil && kickback > 0 {
		collector.MoneyPool += kickback
		collector.IllicitWealth += kickback
		w.addDistrictCorruption(districtID, 2)
	}

	expires := w.Tick + spec.CharterDurationTicks
	rs.MonopolyCharters[itemID] = MonopolyCharter{
		ItemID:                  itemID,
		FactionID:               factionID,
		DistrictID:              districtID,
		GrantedTick:             w.Tick,
		ExpiresTick:             expires,
		CharterFee:              fee,
		KickbackPaid:            kickback,
		CollectorCohortID:       collectorID,
		ExclusiveMarginPermille: spec.ExclusiveMarginPermille,
		Active:                  true,
	}

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  factionID,
		TargetID: itemID,
		Kind:     "monopoly_chartered",
		Action:   Action{Kind: "wait", FactionID: factionID, Item: itemID, Location: districtID, Price: int(fee)},
		Payload: map[string]any{
			"text":                fmt.Sprintf("%s acquired an exclusive monopoly charter for %s in %s (treasury fee %d, collector kickback %d)", f.Name, itemID, districtID, fee, kickback),
			"faction_id":          factionID,
			"item":                itemID,
			"tier":                itemSpec.Tier,
			"district":            districtID,
			"charter_fee":         fee,
			"kickback_coins":      kickback,
			"collector_cohort_id": collectorID,
			"expires_tick":        expires,
		},
	}
	w.NextID++
	return []Event{ev}, true
}

func (w *World) stepCohortWeddings(rs *RelationEvoState) []Event {
	if len(w.Cohorts) < 2 {
		return nil
	}
	spec := rs.effectiveSpec()
	var eligible []*PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() >= 20 && c.MoneyPool >= spec.CohortWeddingDowryCoins {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) < 2 {
		return nil
	}
	dayIdx := int(w.Tick / TicksPerDay)
	iA := int(worldHash(w.ID, int64(dayIdx), "wed_a") % uint64(len(eligible)))
	iB := (iA + 1 + int(worldHash(w.ID, int64(dayIdx), "wed_b")%uint64(len(eligible)-1))) % len(eligible)
	cA := eligible[iA]
	cB := eligible[iB]

	couples := minInt(maxInt(1, minInt(cA.EffectivePopulation(), cB.EffectivePopulation())/120), 6)
	dowry := minInt64(spec.CohortWeddingDowryCoins*int64(couples), cA.MoneyPool/20)
	if dowry < 0 {
		dowry = 0
	}
	cA.MoneyPool -= dowry
	cB.MoneyPool += dowry

	clanA := rs.CohortClans[cA.ID]
	clanB := rs.CohortClans[cB.ID]
	rs.clanTrustDelta(clanA, clanB, 2)
	if clA := rs.Clans[clanA]; clA != nil {
		clA.Prestige++
	}
	if clB := rs.Clans[clanB]; clB != nil {
		clB.Prestige++
	}

	ev := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  cA.ID,
		TargetID: cB.ID,
		Kind:     "cohort_wedding_held",
		Action:   Action{Kind: "marry", TargetID: cB.ID, Location: cA.District, Quantity: couples, Price: int(dowry)},
		Payload: map[string]any{
			"text":        fmt.Sprintf("%s and %s celebrated %d clan weddings between %s and %s (dowry %d coins)", cA.Name, cB.Name, couples, clanA, clanB, dowry),
			"cohort_a":    cA.ID,
			"cohort_b":    cB.ID,
			"clan_a":      clanA,
			"clan_b":      clanB,
			"couples":     couples,
			"dowry_coins": dowry,
			"district":    cA.District,
		},
	}
	w.NextID++
	return []Event{ev}
}

func (w *World) stepMacroVendettasAndMediation(rs *RelationEvoState) []Event {
	var events []Event
	spec := rs.effectiveSpec()

	processed := 0
	for i := range rs.Grudges {
		if processed >= 4 {
			break
		}
		g := &rs.Grudges[i]
		if g.Resolved || g.Intensity < spec.VendettaTriggerMin {
			continue
		}
		if w.Tick-g.CreatedTick < TicksPerDay/4 {
			continue
		}

		// Settlement requires the parties' explicit acceptance through mediate_feud.
		// Otherwise escalate to cohort vendetta clash if cohorts exist on both sides.
		cAgg := rs.findCohort(w, g.AggrievedID)
		if cAgg == nil && g.AggrievedClanID != "" {
			if cl := rs.Clans[g.AggrievedClanID]; cl != nil {
				for _, cid := range cl.MemberCohortIDs {
					if c := rs.findCohort(w, cid); c != nil && c.EffectivePopulation() >= 8 {
						cAgg = c
						break
					}
				}
			}
		}
		cOff := rs.findCohort(w, g.OffenderID)
		if cOff == nil && g.OffenderClanID != "" {
			if cl := rs.Clans[g.OffenderClanID]; cl != nil {
				for _, cid := range cl.MemberCohortIDs {
					if c := rs.findCohort(w, cid); c != nil && c.EffectivePopulation() >= 8 {
						cOff = c
						break
					}
				}
			}
		}
		if cAgg != nil && cOff != nil && cAgg.District == cOff.District && cAgg.ID != cOff.ID && !w.AreFactionsAllied(cAgg.FactionID, cOff.FactionID) && cAgg.EffectivePopulation() >= 8 && cOff.EffectivePopulation() >= 8 {
			fighters := minInt(maxInt(2, minInt(cAgg.EffectivePopulation(), cOff.EffectivePopulation())/40), 20)
			wOff, dOff, _ := w.applyCohortCasualtiesWithCause(cOff, maxInt(1, fighters/3), fighters/10, 0, "combat")
			wAgg, dAgg, _ := w.applyCohortCasualtiesWithCause(cAgg, maxInt(1, fighters/4), fighters/12, 0, "combat")

			g.LastActionTick = w.Tick
			g.Intensity = maxInt(0, g.Intensity-35)
			if g.Intensity < spec.VendettaTriggerMin {
				g.Resolved = true
				g.Resolution = "avenged"
			}

			ev := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  cAgg.ID,
				TargetID: cOff.ID,
				Kind:     "vendetta_clash",
				Action:   Action{Kind: "avenge_feud", TargetID: cOff.ID, Location: cOff.District},
				Payload: map[string]any{
					"text":             fmt.Sprintf("%s clashed with %s in vendetta over %s (%d wounded, %d deaths)", cAgg.Name, cOff.Name, g.ID, wOff+wAgg, dOff+dAgg),
					"grudge_id":        g.ID,
					"aggrieved_cohort": cAgg.ID,
					"offender_cohort":  cOff.ID,
					"wounded":          wOff + wAgg,
					"defender_wounded": wOff,
					"attacker_wounded": wAgg,
					"defender_deaths":  dOff,
					"attacker_deaths":  dAgg,
					"deaths":           dOff + dAgg,
					"cause":            "combat",
					"root_event_id":    g.RootEventID,
				},
			}
			w.NextID++
			events = append(events, ev)
			processed++
		}
	}
	return events
}

func (w *World) stepTurfLeadershipAndCharters(rs *RelationEvoState) []Event {
	var events []Event

	// 1. Expire completed monopoly charters.
	itemKeys := make([]string, 0, len(rs.MonopolyCharters))
	for itemID := range rs.MonopolyCharters {
		itemKeys = append(itemKeys, itemID)
	}
	sort.Strings(itemKeys)
	for _, itemID := range itemKeys {
		ch := rs.MonopolyCharters[itemID]
		if ch.Active && w.Tick >= ch.ExpiresTick {
			ch.Active = false
			rs.MonopolyCharters[itemID] = ch
			ev := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  ch.FactionID,
				TargetID: itemID,
				Kind:     "monopoly_revoked",
				Action:   Action{Kind: "wait", FactionID: ch.FactionID, Item: itemID, Location: ch.DistrictID},
				Payload: map[string]any{
					"text":       fmt.Sprintf("%s's monopoly charter for %s in %s expired", ch.FactionID, itemID, ch.DistrictID),
					"faction_id": ch.FactionID,
					"item":       itemID,
					"district":   ch.DistrictID,
					"reason":     "expired",
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}

	// 2. Evaluate faction leadership turnover.
	for i := range w.Factions {
		f := &w.Factions[i]
		if len(f.Members) == 0 {
			continue
		}
		curLeaderID := rs.FactionLeader[f.ID]
		curLeader := w.actor(curLeaderID)
		curScore := -1
		if curLeader != nil && !curLeader.Dead && f.HasMember(curLeaderID) {
			curScore = rs.actorLeadershipScore(w, curLeader)
		}

		bestID := ""
		bestScore := -1
		for _, mID := range f.Members {
			if mID != curLeaderID && rs.isLeaderOfOtherFaction(f.ID, mID) {
				continue
			}
			a := w.actor(mID)
			sc := rs.actorLeadershipScore(w, a)
			if sc >= 0 && (sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID))) {
				bestScore = sc
				bestID = mID
			}
		}
		if bestID != "" && bestID != curLeaderID && (curScore < 0 || bestScore >= curScore+12) {
			w.setFactionLeader(f.ID, bestID)
			reason := "challenger_ascended"
			if curScore < 0 {
				reason = "vacancy_filled"
			}
			ev := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  bestID,
				TargetID: curLeaderID,
				Kind:     "faction_leader_changed",
				Action:   Action{Kind: "wait", FactionID: f.ID},
				Payload: map[string]any{
					"text":            fmt.Sprintf("%s replaced %s as leader of %s (%d vs %d)", bestID, curLeaderID, f.Name, bestScore, curScore),
					"faction_id":      f.ID,
					"previous_leader": curLeaderID,
					"new_leader":      bestID,
					"reason":          reason,
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}

	// 3. Evaluate district sovereignty shifts once per day at 20:00.
	clock := ClockAt(w.Tick)
	if clock.Hour == 20 && clock.Minute == 0 {
		for _, d := range w.Map.Districts {
			curFaction := rs.DistrictController(d.ID)
			curScore := 0
			if curFaction != "" {
				curScore = rs.factionStrengthInDistrict(w, curFaction, d.ID)
			}
			bestFaction := ""
			bestScore := 0
			for i := range w.Factions {
				f := &w.Factions[i]
				sc := rs.factionStrengthInDistrict(w, f.ID, d.ID)
				if sc > bestScore || (sc == bestScore && (bestFaction == "" || f.ID < bestFaction)) {
					bestScore = sc
					bestFaction = f.ID
				}
			}
			if bestFaction != "" && bestFaction != curFaction && bestScore >= 20 &&
				(curFaction == "" || curScore == 0 || (bestScore >= curScore+20 && bestScore*100 >= curScore*125)) {
				w.setDistrictControl(rs, d.ID, bestFaction, bestScore)

				ev := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  bestFaction,
					TargetID: curFaction,
					Kind:     "district_control_shifted",
					Action:   Action{Kind: "claim_turf", FactionID: bestFaction, Location: d.ID},
					Payload: map[string]any{
						"text":             fmt.Sprintf("%s took sovereignty of %s from %s (%d vs %d)", bestFaction, d.ID, curFaction, bestScore, curScore),
						"district":         d.ID,
						"previous_faction": curFaction,
						"new_faction":      bestFaction,
						"previous_score":   curScore,
						"new_score":        bestScore,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}

		// 4. Daily monopoly charter auction for an unchartered Tier-2/3 tradeable item.
		var candidateItems []string
		for itemID, spec := range w.Catalog.Items {
			if spec.Tradeable && spec.Tier >= 2 && spec.Tier <= 3 {
				if _, active := rs.ActiveCharterForItem(itemID, w.Tick); !active {
					candidateItems = append(candidateItems, itemID)
				}
			}
		}
		sort.Strings(candidateItems)
		if len(candidateItems) > 0 && len(w.Factions) > 0 {
			dayIdx := int(w.Tick / TicksPerDay)
			itemID := candidateItems[int(worldHash(w.ID, int64(dayIdx), "charter_item")%uint64(len(candidateItems)))]
			// Pick strongest faction with a liquid cohort or member.
			bestFaction := ""
			bestWealth := int64(0)
			for i := range w.Factions {
				f := &w.Factions[i]
				var wSum int64
				for ci := range w.Cohorts {
					if w.Cohorts[ci].Population > 0 && w.Cohorts[ci].FactionID == f.ID {
						wSum += w.Cohorts[ci].MoneyPool
					}
				}
				if wSum > bestWealth {
					bestWealth = wSum
					bestFaction = f.ID
				}
			}
			if bestFaction != "" {
				if chEvs, ok := w.GrantMonopolyCharter(rs, bestFaction, itemID, "", 0); ok {
					events = append(events, chEvs...)
				}
			}
		}
	}

	return events
}

// StepRelationsAndTurf runs the macro daily social, kinship, vendetta/mediation,
// district sovereignty, leadership turnover, and monopoly charter pass.
func (w *World) StepRelationsAndTurf(rs *RelationEvoState) []Event {
	if w == nil || rs == nil {
		return nil
	}
	rs.EnsureKinshipSeeded(w)

	var events []Event
	clock := ClockAt(w.Tick)
	if clock.Hour == 11 && clock.Minute == 0 {
		events = append(events, w.stepCohortWeddings(rs)...)
	}
	if clock.Hour == 16 && clock.Minute == 0 {
		events = append(events, w.stepMacroVendettasAndMediation(rs)...)
	}
	events = append(events, w.stepTurfLeadershipAndCharters(rs)...)
	return events
}

func (w *World) canChallengeLeadership(a Actor, factionID string) bool {
	faction := w.faction(factionID)
	if faction == nil || !faction.HasMember(a.ID) || a.Dead || w.Tick < a.JailedUntil || a.Location != faction.District {
		return false
	}
	leader := w.Relations.FactionLeader[factionID]
	if leader == a.ID {
		return false
	}
	incumbent := w.Relations.actorLeadershipScore(w, w.actor(leader))
	return incumbent < 0 || w.Relations.actorLeadershipScore(w, &a) >= incumbent+12
}

func (w *World) applyLeadershipChallenge(tx *WorldTx, a *Actor, x Action) []Event {
	rs := w.ensureRelations()
	if !w.canChallengeLeadership(*a, x.FactionID) {
		return []Event{tx.record(a, x, "action_failed", map[string]any{"text": "challenger lacks standing at the faction seat"})}
	}
	previous := rs.FactionLeader[x.FactionID]
	w.setFactionLeader(x.FactionID, a.ID)
	x.TargetID = previous
	return []Event{tx.record(a, x, "faction_leader_changed", map[string]any{
		"text":       fmt.Sprintf("%s won the leadership of %s", a.Name, x.FactionID),
		"faction_id": x.FactionID, "previous_leader": previous, "new_leader": a.ID, "reason": "challenge",
	}, w.awakeSamePlaceObservers(*a, a.ID)...)}
}

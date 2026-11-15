package sim

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// RelationEvoSpec holds genre-decoupled configuration parameters for kinship,
// marriage dowries, blood-feud intensity/mediation, turf sovereignty, and monopoly charters.
type RelationEvoSpec struct {
	DefaultDowryCoins       int   `json:"default_dowry_coins,omitempty"`
	CohortWeddingDowryCoins int64 `json:"cohort_wedding_dowry_coins,omitempty"`
	DefaultBloodMoneyCoins  int64 `json:"default_blood_money_coins,omitempty"`
	CharterBaseFee          int64 `json:"charter_base_fee,omitempty"`
	CharterDurationTicks    int64 `json:"charter_duration_ticks,omitempty"`
	KickbackBasePermille    int   `json:"kickback_base_permille,omitempty"`
	ExclusiveMarginPermille int   `json:"exclusive_margin_permille,omitempty"`
	MaxGrudgeIntensity      int   `json:"max_grudge_intensity,omitempty"`
	VendettaTriggerMin      int   `json:"vendetta_trigger_min,omitempty"`
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
	if s.DefaultDowryCoins <= 0 {
		s.DefaultDowryCoins = def.DefaultDowryCoins
	}
	if s.CohortWeddingDowryCoins <= 0 {
		s.CohortWeddingDowryCoins = def.CohortWeddingDowryCoins
	}
	if s.DefaultBloodMoneyCoins <= 0 {
		s.DefaultBloodMoneyCoins = def.DefaultBloodMoneyCoins
	}
	if s.CharterBaseFee <= 0 {
		s.CharterBaseFee = def.CharterBaseFee
	}
	if s.CharterDurationTicks <= 0 {
		s.CharterDurationTicks = def.CharterDurationTicks
	}
	if s.KickbackBasePermille <= 0 {
		s.KickbackBasePermille = def.KickbackBasePermille
	}
	if s.ExclusiveMarginPermille <= 0 {
		s.ExclusiveMarginPermille = def.ExclusiveMarginPermille
	}
	if s.MaxGrudgeIntensity <= 0 {
		s.MaxGrudgeIntensity = def.MaxGrudgeIntensity
	}
	if s.VendettaTriggerMin <= 0 {
		s.VendettaTriggerMin = def.VendettaTriggerMin
	}
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
	ID              string `json:"id"`
	AggrievedID     string `json:"aggrieved_id"`
	OffenderID      string `json:"offender_id"`
	AggrievedClanID string `json:"aggrieved_clan_id,omitempty"`
	OffenderClanID  string `json:"offender_clan_id,omitempty"`
	Cause           string `json:"cause"`
	Intensity       int    `json:"intensity"`
	RootEventID     int64  `json:"root_event_id"`
	SourceEventID   int64  `json:"source_event_id"`
	CreatedTick     int64  `json:"created_tick"`
	LastActionTick  int64  `json:"last_action_tick,omitempty"`
	Resolved        bool   `json:"resolved,omitempty"`
	Resolution      string `json:"resolution,omitempty"`
	BloodMoneyPaid  int64  `json:"blood_money_paid,omitempty"`
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

// RelationEvoState holds all JSON-serializable kinship, vendetta, patronage, turf sovereignty,
// faction leadership, and monopoly charter state.
type RelationEvoState struct {
	Spec                RelationEvoSpec                `json:"spec,omitempty"`
	Seeded              bool                           `json:"seeded,omitempty"`
	NextGrudgeSeq       int64                          `json:"next_grudge_seq,omitempty"`
	Kinship             map[string]*ActorKinship       `json:"kinship"`
	Clans               map[string]*ClanRecord         `json:"clans"`
	CohortClans         map[string]string              `json:"cohort_clans"`
	Grudges             []GrudgeRecord                 `json:"grudges,omitempty"`
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
	if s.Grudges != nil {
		out.Grudges = append([]GrudgeRecord(nil), s.Grudges...)
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
		if cA.Trust == nil {
			cA.Trust = map[string]int{}
		}
		cA.Trust[clanB] += delta
	}
	if cB != nil {
		if cB.Trust == nil {
			cB.Trust = map[string]int{}
		}
		cB.Trust[clanA] += delta
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
	} else {
		score += f.Level*4 + f.Influence/25
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
		if a.Location == districtID {
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

func (rs *RelationEvoState) actorLeadershipScore(w *World, a *Actor) int {
	if a == nil || a.Dead {
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
	if w.Tick < a.JailedUntil {
		penalty += 25
	}
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
		cur := rs.FactionLeader[f.ID]
		if cur != "" {
			if a := w.actor(cur); a != nil && !a.Dead && f.HasMember(cur) {
				continue
			}
		}
		bestID := ""
		bestScore := -1
		for _, mID := range f.Members {
			a := w.actor(mID)
			sc := rs.actorLeadershipScore(w, a)
			if sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID)) {
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

func (rs *RelationEvoState) recordGrudge(w *World, aggrievedID, offenderID, cause string, intensity int, rootEventID, sourceEventID int64) []Event {
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
		a.Trust[offenderID] -= 3
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

func (rs *RelationEvoState) nearestLivingKin(w *World, deceasedID string) *Actor {
	k := rs.Kinship[deceasedID]
	if k != nil {
		candidates := make([]string, 0, 8)
		if k.SpouseID != "" {
			candidates = append(candidates, k.SpouseID)
		}
		candidates = append(candidates, k.ChildIDs...)
		candidates = append(candidates, k.ParentIDs...)
		candidates = append(candidates, k.ProtegeIDs...)
		candidates = append(candidates, k.SwornIDs...)
		if k.PatronID != "" {
			candidates = append(candidates, k.PatronID)
		}
		for _, id := range candidates {
			if a := w.actor(id); a != nil && !a.Dead {
				return a
			}
		}
		if clan := rs.Clans[k.ClanID]; clan != nil {
			if clan.PatriarchID != "" && clan.PatriarchID != deceasedID {
				if a := w.actor(clan.PatriarchID); a != nil && !a.Dead {
					return a
				}
			}
			for _, id := range clan.MemberActorIDs {
				if id != deceasedID {
					if a := w.actor(id); a != nil && !a.Dead {
						return a
					}
				}
			}
		}
	}
	return nil
}

func (rs *RelationEvoState) settleDeathAndInheritance(w *World, e Event) []Event {
	deceasedID := e.ActorID
	if deceasedID == "" {
		return nil
	}
	deceased := w.actor(deceasedID)
	if deceased == nil {
		return nil
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
			rs.FactionLeader[f.ID] = ""
			bestID := ""
			bestScore := -1
			for _, mID := range f.Members {
				a := w.actor(mID)
				sc := rs.actorLeadershipScore(w, a)
				if sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID)) {
					bestScore = sc
					bestID = mID
				}
			}
			if bestID != "" {
				rs.FactionLeader[f.ID] = bestID
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

	if len(claimants) > 0 && (len(ownedHouses) > 0 || deceased.Money > 0 || len(deceased.Items) > 0) {
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
			deceased.Money = 0
			primaryHeir.Money += inheritedCoins
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
			primaryHeir.Items[itemID] += deceased.Items[itemID]
			delete(deceased.Items, itemID)
		}
		firstHouseID := ""
		for _, hi := range ownedHouses {
			w.Houses[hi].OwnerID = primaryHeir.ID
			w.Houses[hi].Residents = appendUnique(w.Houses[hi].Residents, primaryHeir.ID)
			if firstHouseID == "" {
				firstHouseID = w.Houses[hi].ID
			}
		}

		if lowTrust && rivalHeir != nil {
			if primaryHeir.Trust == nil {
				primaryHeir.Trust = map[string]int{}
			}
			if rivalHeir.Trust == nil {
				rivalHeir.Trust = map[string]int{}
			}
			primaryHeir.Trust[rivalHeir.ID] -= 2
			rivalHeir.Trust[primaryHeir.ID] -= 2
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
			events = append(events, dispEv)
			events = append(events, rs.recordGrudge(w, rivalHeir.ID, primaryHeir.ID, "inheritance_dispute", 35, e.ID, dispEv.ID)...)
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
			events = append(events, settleEv)
		}
	} else if deceased.Money > 0 {
		// Heirless estate escheats to public treasury so TotalCoins() stays 100% conserved.
		w.creditTreasury(int64(deceased.Money), "confiscated")
		deceased.Money = 0
	}

	// If killed by an assailant, form a high-intensity blood-feud grudge for nearest kin or clan.
	if killerID := relPayloadString(e.Payload, "killer_id"); killerID != "" && killerID != deceasedID {
		aggrievedID := ""
		if kin := rs.nearestLivingKin(w, deceasedID); kin != nil && kin.ID != killerID {
			aggrievedID = kin.ID
		} else if k.ClanID != "" && k.ClanID != rs.entityClanID(w, killerID) {
			aggrievedID = k.ClanID
		}
		if aggrievedID != "" {
			rootID := payloadInt64(e.Payload, "root_event_id")
			if rootID <= 0 {
				rootID = e.ID
			}
			events = append(events, rs.recordGrudge(w, aggrievedID, killerID, "homicide", 70, rootID, e.ID)...)
		}
	}
	return events
}

// ObserveCausalRelationEvents inspects newly settled simulation events to update lineage
// succession, settle funerals/inheritance, and record causal blood-feud grudges preserving RootEventID.
func (rs *RelationEvoState) ObserveCausalRelationEvents(w *World, events []Event) []Event {
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
			out = append(out, rs.recordGrudge(w, e.TargetID, e.ActorID, "raided", 32+dmg/2+coins, rootID, e.ID)...)
		case "attacked":
			dmg := int(payloadInt64(e.Payload, "damage"))
			out = append(out, rs.recordGrudge(w, e.TargetID, e.ActorID, "attacked", 24+dmg/2, rootID, e.ID)...)
		case "extort_resisted":
			out = append(out, rs.recordGrudge(w, e.TargetID, e.ActorID, "extort_resisted", 28, rootID, e.ID)...)
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
			out = append(out, rs.recordGrudge(w, agg, off, "cohort_raided", 30+deaths*15+wounded*2, rootID, e.ID)...)
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
			out = append(out, rs.recordGrudge(w, agg, off, "cohort_racket_clashed", 28+deaths*15+wounded*2, rootID, e.ID)...)
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
			out = append(out, rs.recordGrudge(w, agg, off, "wrongful_conviction", 48, rootID, e.ID)...)
		case "actor_died":
			out = append(out, rs.settleDeathAndInheritance(w, e)...)
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
	if ActorHasTag(k.ParentIDs, g.AggrievedID) || ActorHasTag(k.ChildIDs, g.AggrievedID) || ActorHasTag(k.SwornIDs, g.AggrievedID) {
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

func (rs *RelationEvoState) transferBloodMoney(w *World, offenderID, aggrievedID, offenderClanID, aggrievedClanID string, requested int64) (paid int64, ok bool) {
	if requested <= 0 {
		return 0, true
	}
	// Resolve actor or cohort accounts, or clan patriarch / clan cohort fallback.
	resolvePayer := func(id, clanFallback string) (*Actor, *PopulationCohort) {
		if a := w.actor(id); a != nil && !a.Dead {
			return a, nil
		}
		if c := rs.findCohort(w, id); c != nil && c.Population > 0 {
			return nil, c
		}
		clanID := id
		if rs.Clans[clanID] == nil {
			clanID = clanFallback
		}
		if clan := rs.Clans[clanID]; clan != nil {
			if p := w.actor(clan.PatriarchID); p != nil && !p.Dead {
				return p, nil
			}
			for _, cid := range clan.MemberCohortIDs {
				if c := rs.findCohort(w, cid); c != nil && c.Population > 0 {
					return nil, c
				}
			}
		}
		return nil, nil
	}

	payActor, payCohort := resolvePayer(offenderID, offenderClanID)
	recActor, recCohort := resolvePayer(aggrievedID, aggrievedClanID)
	if (payActor == nil && payCohort == nil) || (recActor == nil && recCohort == nil) {
		return 0, false
	}
	avail := int64(0)
	if payActor != nil {
		avail = int64(payActor.Money)
	} else {
		avail = payCohort.MoneyPool
	}
	if avail < requested {
		return avail, false
	}
	if payActor != nil {
		payActor.Money -= int(requested)
	} else {
		payCohort.MoneyPool -= requested
	}
	if recActor != nil {
		recActor.Money += int(requested)
	} else {
		recCohort.MoneyPool += requested
	}
	return requested, true
}

// ApplyRelationAction handles focal actor actions: "marry", "swear_bond", "avenge_feud",
// "mediate_feud", and "claim_turf". Returns (events, true) if x.Kind is one of these actions.
func (w *World) ApplyRelationAction(rs *RelationEvoState, actorID string, x Action) ([]Event, bool) {
	switch x.Kind {
	case "marry", "swear_bond", "avenge_feud", "mediate_feud", "claim_turf":
	default:
		return nil, false
	}

	a := w.actor(actorID)
	if a == nil {
		return nil, true
	}
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	emit := func(kind, text string, data map[string]any, observers ...*Actor) []Event {
		if data == nil {
			data = map[string]any{}
		}
		data["text"] = text
		return []Event{w.record(a, x, kind, data, observers...)}
	}
	fail := func(reason string) []Event {
		return emit("action_failed", reason, nil)
	}

	if a.Dead {
		return fail("actor is dead"), true
	}
	if w.Tick < a.JailedUntil {
		return fail("actor is imprisoned in the Yamen jail"), true
	}
	if a.Task != nil {
		return fail("actor is already busy with " + a.Task.Action.Kind), true
	}
	if rs == nil {
		return fail("relation state unavailable"), true
	}
	rs.EnsureKinshipSeeded(w)
	spec := rs.effectiveSpec()
	b := w.actor(x.TargetID)

	switch x.Kind {
	case "marry":
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("prospective spouse is not nearby"), true
		}
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		kA := rs.ensureActorKinship(w, *a)
		kB := rs.ensureActorKinship(w, *b)
		if kA.SpouseID != "" || kB.SpouseID != "" {
			return fail("one or both actors are already married"), true
		}
		if ActorHasTag(kA.ParentIDs, b.ID) || ActorHasTag(kA.ChildIDs, b.ID) || ActorHasTag(kA.SwornIDs, b.ID) {
			return fail("cannot marry direct parent, child, or sworn sibling"), true
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
			return fail("insufficient coins for marriage dowry/bride-price"), true
		}
		a.Money -= dowry
		b.Money += dowry

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

		a.Trust[b.ID] += 4
		b.Trust[a.ID] += 4
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
		}, b), true

	case "swear_bond":
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("target is not nearby"), true
		}
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		kA := rs.ensureActorKinship(w, *a)
		kB := rs.ensureActorKinship(w, *b)

		gift := x.Price
		if gift <= 0 {
			gift = x.Quantity
		}
		if gift > 0 && a.Money < gift {
			return fail("insufficient coins for bond ceremony gift"), true
		}
		if gift > 0 {
			a.Money -= gift
			b.Money += gift
		}

		mode := strings.ToLower(strings.TrimSpace(x.Text))
		bondType := "sworn"
		if mode == "patron" || mode == "mentor" || mode == "adopt" || mode == "disciple" {
			bondType = "patron"
			kA.PatronID = b.ID
			kB.ProtegeIDs = appendUnique(kB.ProtegeIDs, a.ID)
		} else {
			kA.SwornIDs = appendUnique(kA.SwornIDs, b.ID)
			kB.SwornIDs = appendUnique(kB.SwornIDs, a.ID)
		}

		a.Trust[b.ID] += 3
		b.Trust[a.ID] += 3
		rs.clanTrustDelta(kA.ClanID, kB.ClanID, 1)

		return emit("bond_sworn", fmt.Sprintf("%s and %s swore a %s bond in %s", a.Name, b.Name, bondType, a.Location), map[string]any{
			"bond_type":  bondType,
			"gift_coins": gift,
			"clan_a":     kA.ClanID,
			"clan_b":     kB.ClanID,
			"district":   a.Location,
		}, b), true

	case "avenge_feud":
		var grudgeIdx = -1
		for i := range rs.Grudges {
			g := rs.Grudges[i]
			if g.Resolved {
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
			return fail("no active blood feud against target"), true
		}
		g := &rs.Grudges[grudgeIdx]
		if b == nil {
			b = w.actor(g.OffenderID)
		}
		if b == nil || b.ID == a.ID || b.Dead || w.Tick < b.JailedUntil || !SamePlace(*a, *b) {
			return fail("feud target is not nearby"), true
		}
		if a.Energy < 6 {
			return fail("insufficient energy to avenge feud"), true
		}
		if a.Health < 25 {
			return fail("too injured to fight; must recuperate"), true
		}
		a.Energy -= 6
		atkPower := CombatPowerWithCatalog(w.Catalog, *a)
		defPower := CombatPowerWithCatalog(w.Catalog, *b)
		dmg := 12 + (atkPower-defPower)/2
		if dmg < 10 {
			dmg = 10
		}
		if dmg > 35 {
			dmg = 35
		}
		if dmg > b.Health {
			dmg = b.Health
		}
		b.Health -= dmg
		if b.Task != nil && b.Task.Action.Kind == "sleep" {
			b.Task = nil
		}
		if b.Health > 0 && a.Health > 10 {
			counter := maxInt(2, defPower/4)
			if counter > a.Health-1 {
				counter = a.Health - 1
			}
			a.Health -= counter
		}
		loot := minInt(b.Money, 3)
		if loot > 0 {
			b.Money -= loot
			a.Money += loot
		}
		a.RestUntil = w.Tick + 32
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		b.Trust[a.ID] -= 3

		g.LastActionTick = w.Tick
		if b.Health == 0 || dmg >= 15 {
			g.Resolved = true
			g.Resolution = "avenged"
		} else {
			g.Intensity = maxInt(1, g.Intensity-25)
		}

		ev := w.record(a, x, "feud_avenged", map[string]any{
			"text":          fmt.Sprintf("%s avenged grudge %s against %s (-%d health, %d restitution coins)", a.Name, g.ID, b.Name, dmg, loot),
			"grudge_id":     g.ID,
			"damage":        dmg,
			"coins":         loot,
			"resolved":      g.Resolved,
			"root_event_id": g.RootEventID,
		}, b)
		if g.RootEventID > 0 {
			ev.Payload["root_event_id"] = g.RootEventID
		}
		events := []Event{ev}
		events = append(events, w.grantCombatXP(a, x, 6, 2)...)
		events = append(events, w.markDeadIfZero(b, a, x, "vendetta")...)
		return events, true

	case "mediate_feud":
		grudgeIdx := -1
		for i := range rs.Grudges {
			g := rs.Grudges[i]
			if g.Resolved {
				continue
			}
			if x.Text != "" && g.ID == x.Text {
				grudgeIdx = i
				break
			}
			if x.TargetID != "" && (g.OffenderID == x.TargetID || g.AggrievedID == x.TargetID) {
				grudgeIdx = i
				break
			}
		}
		if grudgeIdx < 0 {
			return fail("no active unresolved feud found to mediate"), true
		}
		g := &rs.Grudges[grudgeIdx]

		bloodMoney := int64(x.Price)
		if bloodMoney <= 0 {
			bloodMoney = int64(x.Quantity)
		}
		if bloodMoney <= 0 {
			bloodMoney = maxInt64(2, int64(g.Intensity/5))
		}
		paid, ok := rs.transferBloodMoney(w, g.OffenderID, g.AggrievedID, g.OffenderClanID, g.AggrievedClanID, bloodMoney)
		if !ok {
			if x.Price > 0 {
				return fail("offender lacks coins for requested blood-money settlement"), true
			}
			if paid > 0 {
				paid, _ = rs.transferBloodMoney(w, g.OffenderID, g.AggrievedID, g.OffenderClanID, g.AggrievedClanID, paid)
			} else {
				paid = 0
			}
		}

		g.Resolved = true
		g.Resolution = "mediated"
		g.BloodMoneyPaid = paid
		g.LastActionTick = w.Tick

		if offA := w.actor(g.OffenderID); offA != nil {
			if offA.Trust == nil {
				offA.Trust = map[string]int{}
			}
			offA.Trust[g.AggrievedID] += 2
		}
		if aggA := w.actor(g.AggrievedID); aggA != nil {
			if aggA.Trust == nil {
				aggA.Trust = map[string]int{}
			}
			aggA.Trust[g.OffenderID] += 2
		}
		rs.clanTrustDelta(g.AggrievedClanID, g.OffenderClanID, 2)

		ev := w.record(a, x, "feud_mediated", map[string]any{
			"text":          fmt.Sprintf("%s mediated feud %s between %s and %s (%d blood-money coins)", a.Name, g.ID, g.AggrievedID, g.OffenderID, paid),
			"grudge_id":     g.ID,
			"aggrieved_id":  g.AggrievedID,
			"offender_id":   g.OffenderID,
			"blood_money":   paid,
			"root_event_id": g.RootEventID,
		}, b)
		if g.RootEventID > 0 {
			ev.Payload["root_event_id"] = g.RootEventID
		}
		return []Event{ev}, true

	case "claim_turf":
		districtID := x.Location
		if districtID == "" {
			districtID = a.Location
		}
		if _, ok := w.Map.DistrictByID(districtID); !ok {
			return fail("unknown district: " + districtID), true
		}
		if a.Location != districtID || a.InsideHouse != "" {
			return fail("must stand outdoors in " + districtID + " to claim district turf"), true
		}
		factionID := x.FactionID
		if factionID == "" && len(a.Guilds) > 0 {
			factionID = a.Guilds[0]
		}
		f := w.faction(factionID)
		if f == nil || !f.HasMember(a.ID) {
			return fail("actor must belong to target faction to claim turf"), true
		}
		if a.Energy < 5 {
			return fail("insufficient energy to contest district turf"), true
		}
		a.Energy -= 5

		prevFaction := rs.DistrictController(districtID)
		if prevFaction == factionID {
			return fail(factionID + " already holds sovereignty over " + districtID), true
		}
		challengerScore := rs.factionStrengthInDistrict(w, factionID, districtID) + 15
		prevScore := 0
		if prevFaction != "" {
			prevScore = rs.factionStrengthInDistrict(w, prevFaction, districtID)
		}
		if prevFaction != "" && challengerScore < prevScore {
			return emit("turf_claim_repelled", fmt.Sprintf("%s's bid for %s sovereignty in %s was repelled by %s (%d vs %d)", a.Name, f.Name, districtID, prevFaction, challengerScore, prevScore), map[string]any{
				"district":          districtID,
				"challenger":        factionID,
				"controller":        prevFaction,
				"challenger_score":  challengerScore,
				"controller_score":  prevScore,
			}), true
		}

		rs.ControllingFaction[districtID] = factionID
		ds := rs.DistrictSovereignty[districtID]
		ds.DistrictID = districtID
		ds.ControllingFaction = factionID
		ds.ControlSinceTick = w.Tick
		ds.LastShiftTick = w.Tick
		ds.ContestedCount++
		ds.ControllerScore = challengerScore
		rs.DistrictSovereignty[districtID] = ds

		f.Influence += 3
		f.Level = w.Catalog.FactionLevel(f.Influence)

		return emit("district_control_shifted", fmt.Sprintf("%s established %s sovereignty over %s (replacing %s)", a.Name, f.Name, districtID, prevFaction), map[string]any{
			"district":         districtID,
			"previous_faction": prevFaction,
			"new_faction":      factionID,
			"previous_score":   prevScore,
			"new_score":        challengerScore,
		}), true
	}

	return nil, false
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
		payerActor.Money -= int(totalOutlay)
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

	// Look for an active mediator cohort (public trust duty or scholar/elder).
	var mediatorCohort *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() > 0 && w.Catalog.HasPublicTrustDuty([]string{c.Profession}) {
			mediatorCohort = c
			break
		}
	}

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

		// Try mediation first when intensity is moderate and a public-trust mediator exists.
		if mediatorCohort != nil && g.Intensity < 65 {
			bloodMoney := maxInt64(spec.DefaultBloodMoneyCoins, int64(g.Intensity/4))
			if paid, ok := rs.transferBloodMoney(w, g.OffenderID, g.AggrievedID, g.OffenderClanID, g.AggrievedClanID, bloodMoney); ok {
				g.Resolved = true
				g.Resolution = "mediated"
				g.BloodMoneyPaid = paid
				g.LastActionTick = w.Tick
				rs.clanTrustDelta(g.AggrievedClanID, g.OffenderClanID, 2)
				ev := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  mediatorCohort.ID,
					TargetID: g.OffenderID,
					Kind:     "feud_mediated",
					Action:   Action{Kind: "mediate_feud", TargetID: g.OffenderID, Location: mediatorCohort.District, Price: int(paid)},
					Payload: map[string]any{
						"text":          fmt.Sprintf("%s mediated grudge %s between %s and %s (%d blood-money coins)", mediatorCohort.Name, g.ID, g.AggrievedID, g.OffenderID, paid),
						"grudge_id":     g.ID,
						"aggrieved_id":  g.AggrievedID,
						"offender_id":   g.OffenderID,
						"blood_money":   paid,
						"root_event_id": g.RootEventID,
					},
				}
				w.NextID++
				events = append(events, ev)
				processed++
				continue
			}
		}

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
		if cAgg != nil && cOff != nil && cAgg.ID != cOff.ID && cAgg.EffectivePopulation() >= 8 && cOff.EffectivePopulation() >= 8 {
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
			a := w.actor(mID)
			sc := rs.actorLeadershipScore(w, a)
			if sc > bestScore || (sc == bestScore && (bestID == "" || mID < bestID)) {
				bestScore = sc
				bestID = mID
			}
		}
		if bestID != "" && bestID != curLeaderID && (curScore < 0 || bestScore >= curScore+12) {
			rs.FactionLeader[f.ID] = bestID
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
				rs.ControllingFaction[d.ID] = bestFaction
				ds := rs.DistrictSovereignty[d.ID]
				ds.DistrictID = d.ID
				ds.ControllingFaction = bestFaction
				ds.ControlSinceTick = w.Tick
				ds.LastShiftTick = w.Tick
				ds.ContestedCount++
				ds.ControllerScore = bestScore
				rs.DistrictSovereignty[d.ID] = ds

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

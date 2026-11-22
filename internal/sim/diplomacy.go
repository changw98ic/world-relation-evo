package sim

import (
	"fmt"
	"sort"
)

// FactionDiplomacy tracks the bilateral diplomatic stance and war score between two factions.
type FactionDiplomacy struct {
	AllianceProposer  string `json:"alliance_proposer,omitempty"`
	PeaceProposer     string `json:"peace_proposer,omitempty"`
	ContestedDistrict string `json:"contested_district,omitempty"`
	FactionA          string `json:"faction_a"`
	FactionB          string `json:"faction_b"`
	Stance            string `json:"stance"` // "allied" | "friendly" | "neutral" | "hostile" | "at_war"
	RelationScore     int    `json:"relation_score"`
	TreatyUntilTick   int64  `json:"treaty_until_tick,omitempty"`
	WarStartedTick    int64  `json:"war_started_tick,omitempty"`
	WarScoreA         int    `json:"war_score_a,omitempty"`
	WarScoreB         int    `json:"war_score_b,omitempty"`
	Reason            string `json:"reason,omitempty"`
	RootEventID       int64  `json:"root_event_id,omitempty"`
}

// DiplomacyState stores all bilateral faction diplomacy records on World.
type DiplomacyState struct {
	Relations map[string]*FactionDiplomacy `json:"relations,omitempty"`
}

func NewDiplomacyState() *DiplomacyState {
	return &DiplomacyState{Relations: map[string]*FactionDiplomacy{}}
}

func CloneDiplomacyState(d *DiplomacyState) *DiplomacyState {
	if d == nil {
		return nil
	}
	out := &DiplomacyState{Relations: make(map[string]*FactionDiplomacy, len(d.Relations))}
	for k, v := range d.Relations {
		if v == nil {
			continue
		}
		cp := *v
		out.Relations[k] = &cp
	}
	return out
}

func diplomacyKey(f1, f2 string) (key, a, b string) {
	if f1 <= f2 {
		return f1 + "|" + f2, f1, f2
	}
	return f2 + "|" + f1, f2, f1
}

func (w *World) ensureDiplomacy() *DiplomacyState {
	if w.Diplomacy == nil {
		w.Diplomacy = NewDiplomacyState()
	}
	if w.Diplomacy.Relations == nil {
		w.Diplomacy.Relations = map[string]*FactionDiplomacy{}
	}
	return w.Diplomacy
}

func (w *World) ensureFactionDiplomacy(f1, f2 string) *FactionDiplomacy {
	if f1 == "" || f2 == "" || f1 == f2 {
		return nil
	}
	ds := w.ensureDiplomacy()
	key, fa, fb := diplomacyKey(f1, f2)
	if rel, ok := ds.Relations[key]; ok && rel != nil {
		return rel
	}
	rel := &FactionDiplomacy{
		FactionA: fa,
		FactionB: fb,
		Stance:   "neutral",
	}
	ds.Relations[key] = rel
	return rel
}

// AreFactionsAllied reports whether two factions currently hold an active alliance.
func (w *World) AreFactionsAllied(f1, f2 string) bool {
	if w == nil || w.Diplomacy == nil || f1 == "" || f2 == "" || f1 == f2 {
		return false
	}
	key, _, _ := diplomacyKey(f1, f2)
	rel := w.Diplomacy.Relations[key]
	return rel != nil && rel.Stance == "allied" && (rel.TreatyUntilTick == 0 || w.Tick < rel.TreatyUntilTick)
}

// AreFactionsAtWar reports whether two factions are currently in a declared state of war.
func (w *World) AreFactionsAtWar(f1, f2 string) bool {
	if w == nil || w.Diplomacy == nil || f1 == "" || f2 == "" || f1 == f2 {
		return false
	}
	key, _, _ := diplomacyKey(f1, f2)
	rel := w.Diplomacy.Relations[key]
	return rel != nil && rel.Stance == "at_war"
}

func (w *World) actorCanLeadDiplomacy(a Actor, factionID string) bool {
	faction := w.faction(factionID)
	return faction != nil && !a.Dead && faction.HasMember(a.ID) &&
		w.Relations != nil && w.Relations.FactionLeader[factionID] == a.ID
}

func (w *World) diplomacyLeaderFactions(a Actor) []string {
	var ids []string
	for _, faction := range w.Factions {
		if w.actorCanLeadDiplomacy(a, faction.ID) {
			ids = append(ids, faction.ID)
		}
	}
	return ids
}

func (w *World) factionObservers(f1, f2, excludeID string) []*Actor {
	var out []*Actor
	fa, fb := w.faction(f1), w.faction(f2)
	for i := range w.Actors {
		b := &w.Actors[i]
		if b.Dead || b.ID == excludeID {
			continue
		}
		if ActorHasTag(b.Guilds, f1) || ActorHasTag(b.Guilds, f2) ||
			fa != nil && fa.HasMember(b.ID) || fb != nil && fb.HasMember(b.ID) {
			out = append(out, b)
		}
	}
	return out
}

// announceDiplomacy records diplomatic eyewitness observations strictly for awake, co-located
// observers (or awake actors in the event's district for world-level emergency coalitions).
// Remote or sleeping faction members must learn of diplomatic shifts through real communication.
func (w *World) announceDiplomacy(ev Event, f1, f2 string) {
	obs := Observation{
		Kind:      ev.Kind,
		Modality:  "eyewitness",
		SubjectID: f1,
		ObjectID:  f2,
		Location:  ev.Action.Location,
	}
	if existing, ok := ev.Payload["observation"].(Observation); ok {
		obs = existing
	}
	var listeners []*Actor
	if actor := w.actor(ev.ActorID); actor != nil && !actor.Dead {
		listeners = w.awakeSamePlaceObservers(*actor, actor.ID)
	} else {
		district := ev.Action.Location
		if district == "" {
			district, _ = ev.Payload["district"].(string)
		}
		if district != "" {
			for i := range w.Actors {
				b := &w.Actors[i]
				if b.Dead || b.Location != district || b.InsideHouse != "" {
					continue
				}
				if b.Task != nil && (b.Task.Action.Kind == "sleep" || b.Task.Dormant) {
					continue
				}
				listeners = append(listeners, b)
			}
		}
	}
	rootID := payloadInt64(ev.Payload, "root_event_id")
	if rootID == 0 {
		rootID = ev.ID
	}
	for _, observer := range listeners {
		obsCopy := obs
		observer.Memories = appendMemory(observer.Memories, Memory{
			Kind:        ev.Kind,
			Source:      ev.ActorID,
			TargetID:    ev.TargetID,
			Tick:        ev.Tick,
			EventID:     ev.ID,
			RootEventID: rootID,
			Observation: &obsCopy,
			Text:        relPayloadString(ev.Payload, "text"),
		})
	}
}

func (w *World) applyProposeAlliance(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 5 {
		return fail("insufficient energy to propose faction alliance")
	}
	myFac := x.FactionID
	if led := w.diplomacyLeaderFactions(*a); myFac == "" && len(led) > 0 {
		myFac = led[0]
	}
	if !w.actorCanLeadDiplomacy(*a, myFac) {
		return fail("actor lacks diplomatic authority for faction")
	}
	targetFac := x.TargetID
	if targetFac == "" || targetFac == myFac || w.faction(targetFac) == nil {
		return fail("invalid target faction for alliance")
	}
	rel := w.ensureFactionDiplomacy(myFac, targetFac)
	if rel.Stance == "at_war" {
		return fail("cannot ally while at war; sign peace first")
	}
	if w.AreFactionsAllied(myFac, targetFac) {
		return fail("factions are already allied")
	}
	minScore := -40
	if w.affectedByKnownInvasion(*a, myFac, targetFac) {
		minScore = -80
	}
	if rel.RelationScore < minScore {
		return fail("bilateral hostility too severe for alliance")
	}
	if rel.AllianceProposer == myFac {
		return fail("alliance proposal is awaiting the other faction's consent")
	}
	a.Energy -= 5
	if rel.AllianceProposer == "" {
		rel.AllianceProposer = myFac
		ev := w.record(a, x, "alliance_proposed", map[string]any{"text": fmt.Sprintf("%s proposed an alliance to %s", myFac, targetFac)})
		w.announceDiplomacy(ev, myFac, targetFac)
		return []Event{ev}
	}
	rel.AllianceProposer = ""
	rel.Stance = "allied"
	rel.RelationScore = minInt(100, maxInt(rel.RelationScore+25, 45))
	rel.TreatyUntilTick = w.Tick + 7*TicksPerDay
	reason := x.Text
	if reason == "" {
		if w.affectedByKnownInvasion(*a, myFac, targetFac) {
			reason = "external_invasion_coalition"
		} else {
			reason = "mutual_defense_pact"
		}
	}
	rel.Reason = reason

	// Boost trust between faction leaders if known.
	if w.Relations != nil {
		if otherLeader := w.actor(w.Relations.FactionLeader[targetFac]); otherLeader != nil && !otherLeader.Dead && otherLeader.ID != a.ID {
			if a.Trust == nil {
				a.Trust = map[string]int{}
			}
			if otherLeader.Trust == nil {
				otherLeader.Trust = map[string]int{}
			}
			a.Trust[otherLeader.ID]++
			otherLeader.Trust[a.ID]++
		}
	}
	obs := Observation{
		Kind:      "alliance_formed",
		Modality:  "eyewitness",
		SubjectID: myFac,
		ObjectID:  targetFac,
		Location:  a.Location,
		Quantity:  rel.RelationScore,
	}
	ev := w.record(a, x, "alliance_formed", map[string]any{
		"text":           fmt.Sprintf("%s forged an alliance between %s and %s (%s)", a.Name, myFac, targetFac, reason),
		"faction_a":      rel.FactionA,
		"faction_b":      rel.FactionB,
		"stance":         rel.Stance,
		"relation_score": rel.RelationScore,
		"reason":         reason,
		"treaty_until":   rel.TreatyUntilTick,
		"observation":    obs,
	})
	w.announceDiplomacy(ev, myFac, targetFac)
	rel.RootEventID = ev.ID
	return []Event{ev}
}

func (w *World) applyDeclareWar(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 6 {
		return fail("insufficient energy to declare faction war")
	}
	myFac := x.FactionID
	if led := w.diplomacyLeaderFactions(*a); myFac == "" && len(led) > 0 {
		myFac = led[0]
	}
	if !w.actorCanLeadDiplomacy(*a, myFac) {
		return fail("actor lacks authority to declare war")
	}
	targetFac := x.TargetID
	if targetFac == "" || targetFac == myFac || w.faction(targetFac) == nil {
		return fail("invalid target faction for war declaration")
	}
	rel := w.ensureFactionDiplomacy(myFac, targetFac)
	if rel.Stance == "at_war" {
		return fail("factions are already at war")
	}
	if w.Tick < rel.TreatyUntilTick {
		return fail("cannot declare war during active treaty or truce")
	}
	a.Energy -= 6
	rel.Stance = "at_war"
	rel.WarStartedTick = w.Tick
	rel.WarScoreA = 0
	rel.WarScoreB = 0
	rel.AllianceProposer, rel.PeaceProposer, rel.ContestedDistrict = "", "", ""
	rel.RelationScore = maxInt(-100, minInt(rel.RelationScore-35, -60))
	reason := x.Text
	if reason == "" {
		reason = "territorial_and_blood_dispute"
	}
	rel.Reason = reason

	obs := Observation{
		Kind:      "faction_war_declared",
		Modality:  "eyewitness",
		SubjectID: myFac,
		ObjectID:  targetFac,
		Location:  a.Location,
		Quantity:  -rel.RelationScore,
	}
	ev := w.record(a, x, "faction_war_declared", map[string]any{
		"text":           fmt.Sprintf("%s declared faction war: %s vs %s (%s)", a.Name, myFac, targetFac, reason),
		"faction_a":      rel.FactionA,
		"faction_b":      rel.FactionB,
		"aggressor":      myFac,
		"defender":       targetFac,
		"stance":         rel.Stance,
		"relation_score": rel.RelationScore,
		"reason":         reason,
		"observation":    obs,
	})
	w.announceDiplomacy(ev, myFac, targetFac)
	rel.RootEventID = ev.ID
	return []Event{ev}
}

func (w *World) factionTransferReparations(loserFac, winnerFac string, maxCoins int) int {
	if maxCoins <= 0 || loserFac == "" || winnerFac == "" || loserFac == winnerFac {
		return 0
	}
	var payerActor *Actor
	var payerCohort *PopulationCohort
	if w.Relations != nil {
		if lid := w.Relations.FactionLeader[loserFac]; lid != "" {
			if la := w.actor(lid); la != nil && !la.Dead && la.Money > 0 {
				payerActor = la
			}
		}
	}
	if payerActor == nil {
		for i := range w.Cohorts {
			if w.Cohorts[i].FactionID == loserFac && w.Cohorts[i].MoneyPool > 0 {
				payerCohort = &w.Cohorts[i]
				break
			}
		}
	}
	if payerActor == nil && payerCohort == nil {
		return 0
	}

	var recvActor *Actor
	var recvCohort *PopulationCohort
	if w.Relations != nil {
		if wid := w.Relations.FactionLeader[winnerFac]; wid != "" {
			if wa := w.actor(wid); wa != nil && !wa.Dead {
				recvActor = wa
			}
		}
	}
	if recvActor == nil {
		for i := range w.Cohorts {
			if w.Cohorts[i].FactionID == winnerFac && w.Cohorts[i].Population > 0 {
				recvCohort = &w.Cohorts[i]
				break
			}
		}
	}
	if recvActor == nil && recvCohort == nil {
		return 0
	}

	var avail int64
	if payerActor != nil {
		avail = int64(payerActor.Money)
	} else {
		avail = payerCohort.MoneyPool
	}
	paid := int(min(int64(maxCoins), avail/3))
	if paid <= 0 {
		return 0
	}
	if payerActor != nil {
		payerActor.Money -= paid
	} else {
		payerCohort.MoneyPool -= int64(paid)
	}
	if recvActor != nil {
		recvActor.Money += paid
	} else {
		recvCohort.MoneyPool += int64(paid)
	}
	return paid
}

func (w *World) districtFaction(district string) string {
	if w.Relations != nil {
		if controller := w.Relations.DistrictController(district); w.faction(controller) != nil {
			return controller
		}
	}
	if polity := w.EffectiveGoverningPolity(district, ""); polity != nil && w.faction(polity.ID) != nil {
		return polity.ID
	}
	for _, f := range w.Factions {
		if f.District == district {
			return f.ID
		}
	}
	return ""
}

func (w *World) neighboringDistricts(a, b string) bool {
	if _, ok := w.Map.RoadBetween(a, b); ok {
		return true
	}
	for _, route := range w.Map.VenueRoutes {
		from, _ := w.Map.VenueByID(route.FromVenue)
		to, _ := w.Map.VenueByID(route.ToVenue)
		if from.DistrictID == a && to.DistrictID == b || from.DistrictID == b && to.DistrictID == a {
			return true
		}
	}
	return false
}

func (w *World) cedeContestedDistrict(loserFac, winnerFac string) string {
	if loserFac == winnerFac || w.faction(loserFac) == nil || w.faction(winnerFac) == nil {
		return ""
	}
	var contested string
	if w.Diplomacy != nil {
		key, _, _ := diplomacyKey(loserFac, winnerFac)
		if rel := w.Diplomacy.Relations[key]; rel != nil {
			contested = rel.ContestedDistrict
		}
	}
	best, bestScore := "", 0
	for _, d := range w.Map.Districts {
		if w.districtFaction(d.ID) != loserFac {
			continue
		}
		score := 1
		if d.ID == contested {
			score = 3
		} else {
			for _, other := range w.Map.Districts {
				if w.districtFaction(other.ID) == winnerFac && w.neighboringDistricts(d.ID, other.ID) {
					score = 2
					break
				}
			}
		}
		if score > bestScore || score == bestScore && d.ID < best {
			best, bestScore = d.ID, score
		}
	}
	if best != "" {
		rs := w.ensureRelations()
		w.setDistrictControl(rs, best, winnerFac, maxInt(45, rs.DistrictSovereignty[best].ControllerScore))
	}
	return best
}

func (w *World) applyMakePeace(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 4 {
		return fail("insufficient energy to negotiate peace")
	}
	myFac := x.FactionID
	if led := w.diplomacyLeaderFactions(*a); myFac == "" && len(led) > 0 {
		myFac = led[0]
	}
	if !w.actorCanLeadDiplomacy(*a, myFac) {
		return fail("actor lacks diplomatic authority to sign peace")
	}
	targetFac := x.TargetID
	if targetFac == "" || targetFac == myFac || w.faction(targetFac) == nil {
		return fail("invalid target faction for peace treaty")
	}
	rel := w.ensureFactionDiplomacy(myFac, targetFac)
	if rel.Stance != "at_war" {
		return fail("factions are not currently at war")
	}
	if rel.PeaceProposer == myFac {
		return fail("peace proposal is awaiting the other faction's consent")
	}
	a.Energy -= 4
	if rel.PeaceProposer == "" {
		rel.PeaceProposer = myFac
		ev := w.record(a, x, "peace_proposed", map[string]any{"text": fmt.Sprintf("%s proposed peace to %s", myFac, targetFac),
			"root_event_id": rel.RootEventID})
		w.announceDiplomacy(ev, myFac, targetFac)
		return []Event{ev}
	}
	rel.PeaceProposer = ""

	victor, loser := "", ""
	scoreDelta := rel.WarScoreA - rel.WarScoreB
	if scoreDelta > 0 {
		victor, loser = rel.FactionA, rel.FactionB
	} else if scoreDelta < 0 {
		victor, loser = rel.FactionB, rel.FactionA
		scoreDelta = -scoreDelta
	}

	reparations := 0
	cededDistrict := ""
	if victor != "" && loser != "" {
		reparations = w.factionTransferReparations(loser, victor, 25)
		if scoreDelta >= 20 {
			cededDistrict = w.cedeContestedDistrict(loser, victor)
		}
	}

	rel.Stance = "neutral"
	rel.RelationScore = minInt(100, maxInt(rel.RelationScore+25, -15))
	rel.TreatyUntilTick = w.Tick + 5*TicksPerDay
	rel.WarScoreA = 0
	rel.WarScoreB = 0
	rel.Reason = "peace_treaty"

	obs := Observation{
		Kind:      "peace_treaty_signed",
		Modality:  "eyewitness",
		SubjectID: rel.FactionA,
		ObjectID:  rel.FactionB,
		Location:  a.Location,
		Quantity:  reparations,
	}
	ev := w.record(a, x, "peace_treaty_signed", map[string]any{
		"text":           fmt.Sprintf("%s signed peace between %s and %s (victor=%s, reparations=%d coins, ceded=%s)", a.Name, rel.FactionA, rel.FactionB, victor, reparations, cededDistrict),
		"faction_a":      rel.FactionA,
		"faction_b":      rel.FactionB,
		"victor":         victor,
		"loser":          loser,
		"reparations":    reparations,
		"ceded_district": cededDistrict,
		"treaty_until":   rel.TreatyUntilTick,
		"observation":    obs,
		"root_event_id":  rel.RootEventID,
	})
	w.announceDiplomacy(ev, myFac, targetFac)
	events := []Event{ev}
	if cededDistrict != "" {
		events = append(events, w.record(a, Action{Kind: "claim_turf", FactionID: victor, Location: cededDistrict},
			"district_control_shifted", map[string]any{"text": fmt.Sprintf("Peace treaty ceded %s from %s to %s", cededDistrict, loser, victor),
				"district": cededDistrict, "previous_faction": loser, "new_faction": victor,
				"root_event_id": payloadInt64(ev.Payload, "root_event_id"), "cause_event_id": ev.ID, "reason": "peace_treaty"}))
	}
	return events
}

func (w *World) factionOfEntity(id string) string {
	if c := w.findCohortByID(id); c != nil {
		return c.FactionID
	}
	if a := w.actor(id); a != nil && len(a.Guilds) > 0 {
		return a.Guilds[0]
	}
	if w.faction(id) != nil {
		return id
	}
	return ""
}

func (w *World) actorFactionRelation(a, b Actor) string {
	allied := false
	for _, f1 := range a.Guilds {
		for _, f2 := range b.Guilds {
			if w.AreFactionsAtWar(f1, f2) {
				return "at_war"
			}
			allied = allied || w.AreFactionsAllied(f1, f2)
		}
	}
	if allied {
		return "allied"
	}
	return ""
}

func (w *World) affectedByKnownInvasion(a Actor, myFac, peerFac string) bool {
	for _, event := range w.perceivedActiveWorldEvents(a) {
		if event.Category == "external_invasion" && w.factionsAffectedByInvasion(event.DistrictID, myFac, peerFac) {
			return true
		}
	}
	return false
}

func (w *World) factionsAffectedByInvasion(district, f1, f2 string) bool {
	owner := w.districtFaction(district)
	if owner != f1 && owner != f2 {
		return false
	}
	other := f1
	if owner == f1 {
		other = f2
	}
	for _, d := range w.Map.Districts {
		if w.districtFaction(d.ID) == other && w.neighboringDistricts(d.ID, district) {
			return true
		}
	}
	return false
}

func (w *World) stepDiplomacy(settled []Event) []Event {
	if len(w.Factions) < 2 {
		return nil
	}
	var events []Event
	for _, ev := range settled {
		if ev.Kind == "world_event_triggered" && relPayloadString(ev.Payload, "category") == "external_invasion" {
			district := relPayloadString(ev.Payload, "district")
			witnesses, _ := extractStringSlicePayload(ev.Payload, "witnesses")
			known := map[string]bool{}
			for _, id := range witnesses {
				if witness := w.actor(id); witness != nil {
					for _, faction := range witness.Guilds {
						known[faction] = true
					}
				}
			}
			owner := w.districtFaction(district)
			if !known[owner] {
				continue
			}
			for _, other := range sortedProductionKeys(known) {
				if other == owner || !w.factionsAffectedByInvasion(district, owner, other) ||
					w.AreFactionsAtWar(owner, other) || w.AreFactionsAllied(owner, other) {
					continue
				}
				rel := w.ensureFactionDiplomacy(owner, other)
				rel.Stance, rel.Reason, rel.AllianceProposer = "allied", "external_invasion_coalition", ""
				rel.RelationScore = maxInt(45, rel.RelationScore)
				rel.TreatyUntilTick, rel.RootEventID = w.Tick+7*TicksPerDay, ev.ID
				alliance := Event{ID: w.NextID, Tick: w.Tick, ActorID: "world", TargetID: other, Kind: "alliance_formed",
					Action: Action{Kind: "propose_alliance", FactionID: owner, TargetID: other, Location: district},
					Payload: map[string]any{"text": fmt.Sprintf("%s and %s formed an emergency coalition around %s", owner, other, district),
						"faction_a": rel.FactionA, "faction_b": rel.FactionB, "district": district,
						"root_event_id": ev.ID, "cause_event_id": ev.ID, "treaty_until": rel.TreatyUntilTick}}
				w.NextID++
				w.announceDiplomacy(alliance, owner, other)
				events = append(events, alliance)
			}
			continue
		}

		f1, f2 := "", ""
		score, counter := 0, 0
		switch ev.Kind {
		case "attacked", "feud_avenged", "raided", "raid_repelled", "party_betrayed", "cohort_raided",
			"cohort_raid_repelled", "cohort_turf_war", "vendetta_clash", "faction_battle":
			f1, f2 = w.factionOfEntity(ev.ActorID), w.factionOfEntity(ev.TargetID)
			score = int(payloadInt64(ev.Payload, "damage") + payloadInt64(ev.Payload, "wounded") + 3*payloadInt64(ev.Payload, "deaths"))
			score += int(payloadInt64(ev.Payload, "coins") + payloadInt64(ev.Payload, "defender_wounded") + 3*payloadInt64(ev.Payload, "defender_deaths"))
			counter = int(payloadInt64(ev.Payload, "recoil") + payloadInt64(ev.Payload, "attacker_wounded") + 3*payloadInt64(ev.Payload, "attacker_deaths"))
			if ev.Kind == "vendetta_clash" {
				score = int(payloadInt64(ev.Payload, "defender_wounded") + 3*payloadInt64(ev.Payload, "defender_deaths"))
			}
			if ev.Kind == "raid_repelled" {
				score, counter = 0, score
			}
		case "district_control_shifted":
			if relPayloadString(ev.Payload, "reason") == "peace_treaty" {
				continue
			}
			f1, f2 = relPayloadString(ev.Payload, "new_faction"), relPayloadString(ev.Payload, "previous_faction")
			score = 12
		default:
			continue
		}
		if f1 == "" || f2 == "" || f1 == f2 || score+counter == 0 {
			continue
		}
		rel := w.ensureFactionDiplomacy(f1, f2)
		rel.RelationScore = maxInt(-100, rel.RelationScore-maxInt(1, (score+counter)/3))
		if w.AreFactionsAllied(f1, f2) && rel.RelationScore <= -30 {
			rel.Stance = "hostile"
			rel.TreatyUntilTick = 0
			rel.Reason = "alliance_shattered_by_hostilities"
			rel.RootEventID = ev.ID
			broke := Event{ID: w.NextID, Tick: w.Tick, ActorID: "world", TargetID: f2, Kind: "alliance_broken",
				Action: Action{Kind: "declare_war", FactionID: f1, TargetID: f2},
				Payload: map[string]any{"text": fmt.Sprintf("Alliance between %s and %s collapsed after repeated bloodshed (score=%d)", rel.FactionA, rel.FactionB, rel.RelationScore),
					"faction_a": rel.FactionA, "faction_b": rel.FactionB, "relation_score": rel.RelationScore,
					"root_event_id": ev.ID, "cause_event_id": ev.ID}}
			w.NextID++
			w.announceDiplomacy(broke, f1, f2)
			events = append(events, broke)
		} else if !w.AreFactionsAtWar(f1, f2) && !w.AreFactionsAllied(f1, f2) && rel.RelationScore <= -35 {
			rel.Stance = "hostile"
		}
		if rel.Stance == "hostile" && rel.RelationScore <= -75 && w.Tick >= rel.TreatyUntilTick {
			rel.Stance, rel.Reason, rel.WarStartedTick = "at_war", ev.Kind, w.Tick
			rel.RootEventID = ev.ID
			war := Event{ID: w.NextID, Tick: w.Tick, ActorID: "world", TargetID: f2, Kind: "faction_war_declared",
				Action: Action{Kind: "declare_war", FactionID: f1, TargetID: f2},
				Payload: map[string]any{"text": fmt.Sprintf("Open war followed a clash between %s and %s", f1, f2),
					"faction_a": rel.FactionA, "faction_b": rel.FactionB, "root_event_id": ev.ID, "cause_event_id": ev.ID}}
			w.NextID++
			w.announceDiplomacy(war, f1, f2)
			events = append(events, war)
		}
		if !w.AreFactionsAtWar(f1, f2) {
			continue
		}
		if f1 == rel.FactionA {
			rel.WarScoreA += score
			rel.WarScoreB += counter
		} else {
			rel.WarScoreB += score
			rel.WarScoreA += counter
		}
		if district := srEventDistrict(w, ev); district != "" {
			rel.ContestedDistrict = district
		}
		skirmish := Event{ID: w.NextID, Tick: w.Tick, ActorID: ev.ActorID, TargetID: ev.TargetID, Kind: "faction_war_skirmish",
			Action: Action{Kind: "wait", FactionID: f1, Location: rel.ContestedDistrict},
			Payload: map[string]any{"text": fmt.Sprintf("%s and %s clashed (war score %d:%d)", f1, f2, rel.WarScoreA, rel.WarScoreB),
				"faction_a": rel.FactionA, "faction_b": rel.FactionB, "war_score_a": rel.WarScoreA, "war_score_b": rel.WarScoreB,
				"root_event_id": rel.RootEventID, "cause_event_id": ev.ID}}
		w.NextID++
		events = append(events, skirmish)
	}
	if w.Diplomacy != nil {
		for _, rel := range w.Diplomacy.Relations {
			if rel != nil && rel.Stance == "allied" && rel.TreatyUntilTick > 0 && w.Tick >= rel.TreatyUntilTick {
				rel.Stance, rel.TreatyUntilTick = "friendly", 0
			}
		}
	}
	return events
}

func actorKnowsDiplomacyUpdate(a Actor, rel FactionDiplomacy) bool {
	// Seeded initial bilateral diplomacy (no post-start RootEventID) is baseline common knowledge.
	if rel.RootEventID == 0 {
		return true
	}
	for _, m := range a.Memories {
		if m.EventID == rel.RootEventID || m.RootEventID == rel.RootEventID {
			return true
		}
		if m.Observation != nil && (m.Observation.Kind == "faction_war_declared" || m.Observation.Kind == "alliance_formed" || m.Observation.Kind == "alliance_broken" || m.Observation.Kind == "peace_treaty_signed") {
			if (m.Observation.SubjectID == rel.FactionA && m.Observation.ObjectID == rel.FactionB) ||
				(m.Observation.SubjectID == rel.FactionB && m.Observation.ObjectID == rel.FactionA) {
				if m.Tick >= rel.WarStartedTick {
					return true
				}
			}
		}
		if m.Statement != nil && (m.Statement.Topic == "faction_war_declared" || m.Statement.Topic == "alliance_formed" || m.Statement.Topic == "alliance_broken" || m.Statement.Topic == "peace_treaty_signed") {
			if (m.Statement.SubjectID == rel.FactionA && m.Statement.ObjectID == rel.FactionB) ||
				(m.Statement.SubjectID == rel.FactionB && m.Statement.ObjectID == rel.FactionA) {
				if m.Tick >= rel.WarStartedTick {
					return true
				}
			}
		}
	}
	return false
}

func (w *World) perceivedFactionDiplomacy(a Actor) []FactionDiplomacy {
	if w == nil || w.Diplomacy == nil || len(w.Diplomacy.Relations) == 0 {
		return nil
	}
	keys := make([]string, 0, len(w.Diplomacy.Relations))
	for k := range w.Diplomacy.Relations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []FactionDiplomacy
	for _, k := range keys {
		rel := w.Diplomacy.Relations[k]
		if rel == nil {
			continue
		}
		if (ActorHasTag(a.Guilds, rel.FactionA) || ActorHasTag(a.Guilds, rel.FactionB)) && actorKnowsDiplomacyUpdate(a, *rel) {
			out = append(out, *rel)
		}
	}
	return out
}

func (w *World) setFactionLeader(factionID, leaderID string) {
	rs := w.ensureRelations()
	previous := rs.FactionLeader[factionID]
	rs.FactionLeader[factionID] = leaderID
	if w.Polities == nil {
		return
	}
	for i := range w.Polities.Nodes {
		node := &w.Polities.Nodes[i]
		if node.ID == factionID || node.DeFactoControllerID == factionID ||
			node.LeaderActorID == previous && previous != "" && (node.DeJureParentID == factionID || rs.DistrictController(node.SeatDistrictID) == factionID) {
			node.LeaderActorID = leaderID
		}
	}
}

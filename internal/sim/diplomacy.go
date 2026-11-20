package sim

import (
	"fmt"
	"sort"
)

// FactionDiplomacy tracks the bilateral diplomatic stance and war score between two factions.
type FactionDiplomacy struct {
	FactionA        string `json:"faction_a"`
	FactionB        string `json:"faction_b"`
	Stance          string `json:"stance"` // "allied" | "friendly" | "neutral" | "hostile" | "at_war"
	RelationScore   int    `json:"relation_score"`
	TreatyUntilTick int64  `json:"treaty_until_tick,omitempty"`
	WarStartedTick  int64  `json:"war_started_tick,omitempty"`
	WarScoreA       int    `json:"war_score_a,omitempty"`
	WarScoreB       int    `json:"war_score_b,omitempty"`
	Reason          string `json:"reason,omitempty"`
	RootEventID     int64  `json:"root_event_id,omitempty"`
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
	return rel != nil && rel.Stance == "allied"
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
	if factionID == "" || !ActorHasTag(a.Guilds, factionID) {
		return false
	}
	if w.Relations != nil && w.Relations.FactionLeader[factionID] == a.ID {
		return true
	}
	if ActorHasTag(a.Tags, "ruler") || ActorHasTag(a.Tags, "elder_oversight") ||
		ActorHasTag(a.Tags, "commander") || ActorHasTag(a.Tags, "enforcer") {
		return true
	}
	return a.GuildLevels[factionID] >= 2
}

func (w *World) hasActiveInvasionWorldEvent() bool {
	for _, ev := range w.ActiveWorldEvents {
		if w.Tick < ev.ExpiresTick && ev.Category == "external_invasion" {
			return true
		}
	}
	return false
}

func (w *World) factionObservers(f1, f2, excludeID string) []*Actor {
	var out []*Actor
	for i := range w.Actors {
		b := &w.Actors[i]
		if b.Dead || b.ID == excludeID {
			continue
		}
		if ActorHasTag(b.Guilds, f1) || ActorHasTag(b.Guilds, f2) {
			out = append(out, b)
		}
	}
	return out
}

func (w *World) applyProposeAlliance(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 5 {
		return fail("insufficient energy to propose faction alliance")
	}
	myFac := x.FactionID
	if myFac == "" && len(a.Guilds) > 0 {
		myFac = a.Guilds[0]
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
	if rel.Stance == "allied" && w.Tick < rel.TreatyUntilTick {
		return fail("factions are already allied")
	}
	minScore := -40
	if w.hasActiveInvasionWorldEvent() {
		minScore = -80
	}
	if rel.RelationScore < minScore {
		return fail("bilateral hostility too severe for alliance")
	}
	a.Energy -= 5
	rel.Stance = "allied"
	rel.RelationScore = minInt(100, maxInt(rel.RelationScore+25, 45))
	rel.TreatyUntilTick = w.Tick + 7*TicksPerDay
	reason := x.Text
	if reason == "" {
		if w.hasActiveInvasionWorldEvent() {
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
	observers := w.factionObservers(myFac, targetFac, a.ID)
	ev := w.record(a, x, "alliance_formed", map[string]any{
		"text":           fmt.Sprintf("%s forged an alliance between %s and %s (%s)", a.Name, myFac, targetFac, reason),
		"faction_a":      rel.FactionA,
		"faction_b":      rel.FactionB,
		"stance":         rel.Stance,
		"relation_score": rel.RelationScore,
		"reason":         reason,
		"treaty_until":   rel.TreatyUntilTick,
		"observation":    obs,
	}, observers...)
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
	if myFac == "" && len(a.Guilds) > 0 {
		myFac = a.Guilds[0]
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
	if w.Tick < rel.TreatyUntilTick && rel.Stance != "hostile" {
		return fail("cannot declare war during active treaty or truce")
	}
	a.Energy -= 6
	rel.Stance = "at_war"
	rel.WarStartedTick = w.Tick
	rel.WarScoreA = 0
	rel.WarScoreB = 0
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
	observers := w.factionObservers(myFac, targetFac, a.ID)
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
	}, observers...)
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

func (w *World) cedeContestedDistrict(loserFac, winnerFac string) string {
	if w.Relations == nil || loserFac == "" || winnerFac == "" || loserFac == winnerFac {
		return ""
	}
	var controlled []string
	for dID, owner := range w.Relations.ControllingFaction {
		if owner == loserFac {
			controlled = append(controlled, dID)
		}
	}
	if len(controlled) <= 1 {
		return ""
	}
	sort.Strings(controlled)
	ceded := controlled[len(controlled)-1]
	w.Relations.ControllingFaction[ceded] = winnerFac
	if sov, ok := w.Relations.DistrictSovereignty[ceded]; ok {
		sov.ControllingFaction = winnerFac
		sov.ControlSinceTick = w.Tick
		sov.LastShiftTick = w.Tick
		sov.ControllerScore = maxInt(45, sov.ControllerScore)
		w.Relations.DistrictSovereignty[ceded] = sov
	}
	return ceded
}

func (w *World) applyMakePeace(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 4 {
		return fail("insufficient energy to negotiate peace")
	}
	myFac := x.FactionID
	if myFac == "" && len(a.Guilds) > 0 {
		myFac = a.Guilds[0]
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
	a.Energy -= 4

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
	observers := w.factionObservers(rel.FactionA, rel.FactionB, a.ID)
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
	}, observers...)
	return []Event{ev}
}

func (w *World) factionStrength(factionID string) int {
	strength := 10
	for i := range w.Actors {
		a := w.Actors[i]
		if !a.Dead && ActorHasTag(a.Guilds, factionID) {
			strength += CombatPowerWithCatalog(w.Catalog, a)
		}
	}
	for i := range w.Cohorts {
		c := w.Cohorts[i]
		if c.FactionID == factionID && c.Population > 0 {
			strength += c.Population / 10
		}
	}
	return strength
}

func (w *World) stepDiplomacy(settled []Event) []Event {
	if w == nil || len(w.Factions) < 2 {
		return nil
	}
	var events []Event
	ds := w.ensureDiplomacy()

	// 1. React to settled conflict and external invasion events.
	for _, ev := range settled {
		switch ev.Kind {
		case "turf_war", "vendetta_clash", "grudge_formed":
			a1 := w.actor(ev.ActorID)
			a2 := w.actor(ev.TargetID)
			if a1 == nil || a2 == nil || len(a1.Guilds) == 0 || len(a2.Guilds) == 0 {
				continue
			}
			f1, f2 := a1.Guilds[0], a2.Guilds[0]
			if f1 == f2 {
				continue
			}
			rel := w.ensureFactionDiplomacy(f1, f2)
			drop := 8
			if ev.Kind == "turf_war" {
				drop = 15
			}
			rel.RelationScore = maxInt(-100, rel.RelationScore-drop)
			if rel.Stance != "at_war" && rel.RelationScore <= -35 {
				rel.Stance = "hostile"
			}
			if rel.Stance == "hostile" && rel.RelationScore <= -75 && w.Tick >= rel.TreatyUntilTick {
				rel.Stance = "at_war"
				rel.WarStartedTick = w.Tick
				rel.WarScoreA = 0
				rel.WarScoreB = 0
				rel.Reason = ev.Kind
				rel.RootEventID = ev.ID
				warEv := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: "world",
					Kind:    "faction_war_declared",
					Action:  Action{Kind: "declare_war", FactionID: rel.FactionA, TargetID: rel.FactionB, BasisEventID: ev.ID},
					Payload: map[string]any{
						"text":           fmt.Sprintf("Escalating %s triggered open war between %s and %s", ev.Kind, rel.FactionA, rel.FactionB),
						"faction_a":      rel.FactionA,
						"faction_b":      rel.FactionB,
						"stance":         "at_war",
						"relation_score": rel.RelationScore,
						"reason":         ev.Kind,
						"root_event_id":  ev.ID,
					},
				}
				w.NextID++
				events = append(events, warEv)
			}
		case "world_event_triggered":
			if cat, _ := ev.Payload["category"].(string); cat == "external_invasion" {
				// Form one deterministic emergency coalition between two non-war factions.
				facIDs := make([]string, 0, len(w.Factions))
				for _, f := range w.Factions {
					facIDs = append(facIDs, f.ID)
				}
				sort.Strings(facIDs)
				formed := false
				for i := 0; i < len(facIDs) && !formed; i++ {
					for j := i + 1; j < len(facIDs) && !formed; j++ {
						rel := w.ensureFactionDiplomacy(facIDs[i], facIDs[j])
						if rel.Stance == "at_war" || rel.Stance == "allied" {
							continue
						}
						rel.Stance = "allied"
						rel.RelationScore = minInt(100, maxInt(rel.RelationScore+25, 45))
						rel.TreatyUntilTick = w.Tick + 7*TicksPerDay
						rel.Reason = "external_invasion_coalition"
						rel.RootEventID = ev.ID
						coalEv := Event{
							ID:      w.NextID,
							Tick:    w.Tick,
							ActorID: "world",
							Kind:    "alliance_formed",
							Action:  Action{Kind: "propose_alliance", FactionID: rel.FactionA, TargetID: rel.FactionB, BasisEventID: ev.ID},
							Payload: map[string]any{
								"text":           fmt.Sprintf("%s and %s formed an emergency defense coalition against external invasion", rel.FactionA, rel.FactionB),
								"faction_a":      rel.FactionA,
								"faction_b":      rel.FactionB,
								"stance":         "allied",
								"relation_score": rel.RelationScore,
								"reason":         "external_invasion_coalition",
								"treaty_until":   rel.TreatyUntilTick,
								"root_event_id":  ev.ID,
							},
						}
						w.NextID++
						events = append(events, coalEv)
						formed = true
					}
				}
			}
		}
	}

	// 2. Daily noon diplomacy maintenance & bounded war skirmishes.
	clock := ClockAt(w.Tick)
	if clock.Hour != 12 || clock.Minute != 0 || len(ds.Relations) == 0 {
		return events
	}
	keys := make([]string, 0, len(ds.Relations))
	for k := range ds.Relations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		rel := ds.Relations[k]
		if rel == nil {
			continue
		}
		if rel.Stance == "allied" && rel.TreatyUntilTick > 0 && w.Tick >= rel.TreatyUntilTick {
			rel.Stance = "friendly"
			rel.TreatyUntilTick = 0
		}
		if rel.Stance == "at_war" {
			strA := w.factionStrength(rel.FactionA) + int(worldHash(w.ID, w.Tick, "warA:"+k)%20)
			strB := w.factionStrength(rel.FactionB) + int(worldHash(w.ID, w.Tick, "warB:"+k)%20)
			winner := rel.FactionA
			gain := 8
			if strB > strA {
				winner = rel.FactionB
				rel.WarScoreB += gain
			} else {
				rel.WarScoreA += gain
			}
			skEv := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "faction_war_skirmish",
				Action:  Action{Kind: "wait", FactionID: winner},
				Payload: map[string]any{
					"text":        fmt.Sprintf("War skirmish between %s and %s: %s gained advantage (score %d:%d)", rel.FactionA, rel.FactionB, winner, rel.WarScoreA, rel.WarScoreB),
					"faction_a":   rel.FactionA,
					"faction_b":   rel.FactionB,
					"winner":      winner,
					"war_score_a": rel.WarScoreA,
					"war_score_b": rel.WarScoreB,
				},
			}
			w.NextID++
			events = append(events, skEv)
		}
	}
	return events
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
		if ActorHasTag(a.Guilds, rel.FactionA) || ActorHasTag(a.Guilds, rel.FactionB) || rel.Stance == "at_war" || rel.Stance == "allied" {
			out = append(out, *rel)
		}
	}
	return out
}

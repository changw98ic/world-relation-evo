package sim

import (
	"fmt"
	"sort"
)

// Party represents a temporary cooperative group for expeditions, escorts, threat hunts, or relic claims.
type Party struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	LeaderID       string   `json:"leader_id"`
	Purpose        string   `json:"purpose"` // "expedition" | "escort" | "hunt" | "relic_hunt"
	TargetDistrict string   `json:"target_district,omitempty"`
	MemberIDs      []string `json:"member_ids"`
	CreatedTick    int64    `json:"created_tick"`
	ExpiresTick    int64    `json:"expires_tick"`
	LootPolicy     string   `json:"loot_policy,omitempty"` // "equal" | "leader_share"
}

func CloneParty(p Party) Party {
	return cloneParty(p)
}

func cloneParty(p Party) Party {
	out := p
	if p.MemberIDs != nil {
		out.MemberIDs = append([]string(nil), p.MemberIDs...)
	}
	return out
}

// ActorParty returns a pointer to the active party containing actorID, if any.
func (w *World) ActorParty(actorID string) *Party {
	if w == nil || actorID == "" {
		return nil
	}
	for i := range w.Parties {
		if w.Tick >= w.Parties[i].ExpiresTick {
			continue
		}
		if ActorHasTag(w.Parties[i].MemberIDs, actorID) {
			return &w.Parties[i]
		}
	}
	return nil
}

// stepParties prunes dead members, transfers leadership deterministically if a leader dies,
// and disbands expired or empty parties.
func (w *World) stepParties() []Event {
	if w == nil || len(w.Parties) == 0 {
		return nil
	}
	var events []Event
	kept := make([]Party, 0, len(w.Parties))
	for _, p := range w.Parties {
		var alive []string
		for _, mid := range p.MemberIDs {
			if m := w.actor(mid); m != nil && !m.Dead {
				alive = appendUnique(alive, mid)
			}
		}
		expired := w.Tick >= p.ExpiresTick
		// Allow a newly formed solo party up to 1 day to recruit before disbanding.
		soloStale := len(alive) == 1 && w.Tick-p.CreatedTick >= TicksPerDay
		if len(alive) == 0 || expired || soloStale {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: p.LeaderID,
				Kind:    "party_disbanded",
				Action:  Action{Kind: "leave_party", TargetID: p.ID},
				Payload: map[string]any{
					"text":     fmt.Sprintf("Party %s (%s) disbanded", p.Name, p.Purpose),
					"party_id": p.ID,
					"purpose":  p.Purpose,
					"expired":  expired,
				},
			}
			w.NextID++
			events = append(events, ev)
			continue
		}
		p.MemberIDs = alive
		if !ActorHasTag(alive, p.LeaderID) {
			sorted := append([]string(nil), alive...)
			sort.Strings(sorted)
			p.LeaderID = sorted[0]
		}
		kept = append(kept, p)
	}
	if len(kept) == 0 {
		w.Parties = nil
	} else {
		w.Parties = kept
	}
	return events
}

// perceivedParties returns active parties that actor a belongs to OR whose leader is in the same district.
func (w *World) perceivedParties(a Actor) []Party {
	if w == nil || len(w.Parties) == 0 {
		return nil
	}
	var out []Party
	for _, p := range w.Parties {
		if w.Tick >= p.ExpiresTick || len(p.MemberIDs) == 0 {
			continue
		}
		visible := ActorHasTag(p.MemberIDs, a.ID)
		if !visible {
			if leader := w.actor(p.LeaderID); leader != nil && !leader.Dead && leader.Location == a.Location {
				visible = true
			}
		}
		if visible {
			out = append(out, cloneParty(p))
		}
	}
	return out
}

func normalizePartyPurpose(raw string) string {
	switch raw {
	case "expedition", "escort", "hunt", "relic_hunt":
		return raw
	default:
		return "expedition"
	}
}

func (w *World) applyFormParty(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.InsideHouse != "" {
		return fail("step outdoors to form a party")
	}
	if a.Energy < 3 {
		return fail("insufficient energy to form a party")
	}
	if w.ActorParty(a.ID) != nil {
		return fail("already belongs to an active party")
	}
	purpose := normalizePartyPurpose(x.Item)
	targetDist := x.Location
	if targetDist == "" {
		targetDist = a.Location
	}
	lootPolicy := "equal"
	if x.Text == "leader_share" {
		lootPolicy = "leader_share"
	}
	partyID := fmt.Sprintf("party_%d_%s", w.Tick, a.ID)
	members := []string{a.ID}
	var recruited *Actor
	if x.TargetID != "" && x.TargetID != a.ID {
		if b := w.actor(x.TargetID); b != nil && !b.Dead && w.Tick >= b.JailedUntil && SamePlace(*a, *b) && w.ActorParty(b.ID) == nil {
			if b.Trust[a.ID] >= 0 && a.Trust[b.ID] >= -1 {
				members = append(members, b.ID)
				recruited = b
			}
		}
	}
	a.Energy -= 3
	p := Party{
		ID:             partyID,
		Name:           fmt.Sprintf("%s's %s party", a.Name, purpose),
		LeaderID:       a.ID,
		Purpose:        purpose,
		TargetDistrict: targetDist,
		MemberIDs:      members,
		CreatedTick:    w.Tick,
		ExpiresTick:    w.Tick + 2*TicksPerDay,
		LootPolicy:     lootPolicy,
	}
	w.Parties = append(w.Parties, p)
	if recruited != nil {
		if a.Trust == nil {
			a.Trust = map[string]int{}
		}
		if recruited.Trust == nil {
			recruited.Trust = map[string]int{}
		}
		a.Trust[recruited.ID]++
		recruited.Trust[a.ID]++
	}
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	ev := w.record(a, x, "party_formed", map[string]any{
		"text":            fmt.Sprintf("%s formed %s targeting %s (%d members)", a.Name, p.Name, targetDist, len(members)),
		"party_id":        p.ID,
		"leader_id":       p.LeaderID,
		"purpose":         p.Purpose,
		"target_district": p.TargetDistrict,
		"loot_policy":     p.LootPolicy,
		"members":         append([]string(nil), members...),
	}, observers...)
	return []Event{ev}
}

func (w *World) applyJoinParty(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 2 {
		return fail("insufficient energy to join party")
	}
	if w.ActorParty(a.ID) != nil {
		return fail("already belongs to an active party")
	}
	targetIdx := -1
	for i := range w.Parties {
		p := w.Parties[i]
		if w.Tick >= p.ExpiresTick {
			continue
		}
		if p.ID == x.TargetID || p.LeaderID == x.TargetID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return fail("no matching active party to join")
	}
	p := &w.Parties[targetIdx]
	if len(p.MemberIDs) >= 6 {
		return fail("party is already full")
	}
	leader := w.actor(p.LeaderID)
	if leader == nil || leader.Dead || leader.Location != a.Location {
		return fail("must be in the same district as party leader to join")
	}
	if leader.Trust[a.ID] < -1 {
		return fail("party leader does not trust applicant")
	}
	a.Energy -= 2
	p.MemberIDs = appendUnique(p.MemberIDs, a.ID)
	if leader.Trust == nil {
		leader.Trust = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	leader.Trust[a.ID]++
	a.Trust[leader.ID]++
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	ev := w.record(a, x, "party_joined", map[string]any{
		"text":      fmt.Sprintf("%s joined %s led by %s (%d members)", a.Name, p.Name, leader.Name, len(p.MemberIDs)),
		"party_id":  p.ID,
		"leader_id": p.LeaderID,
		"purpose":   p.Purpose,
		"members":   append([]string(nil), p.MemberIDs...),
	}, observers...)
	return []Event{ev}
}

func (w *World) applyLeaveParty(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	partyIdx := -1
	for i := range w.Parties {
		if w.Tick < w.Parties[i].ExpiresTick && ActorHasTag(w.Parties[i].MemberIDs, a.ID) {
			partyIdx = i
			break
		}
	}
	if partyIdx < 0 {
		return fail("actor is not in an active party")
	}
	p := w.Parties[partyIdx]
	remaining := removeString(p.MemberIDs, a.ID)
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	ev := w.record(a, x, "party_left", map[string]any{
		"text":      fmt.Sprintf("%s left %s", a.Name, p.Name),
		"party_id":  p.ID,
		"remaining": len(remaining),
	}, observers...)
	if len(remaining) <= 1 {
		w.Parties = append(w.Parties[:partyIdx], w.Parties[partyIdx+1:]...)
	} else {
		p.MemberIDs = remaining
		if p.LeaderID == a.ID {
			sorted := append([]string(nil), remaining...)
			sort.Strings(sorted)
			p.LeaderID = sorted[0]
		}
		w.Parties[partyIdx] = p
	}
	return []Event{ev}
}

func (w *World) applyBetrayParty(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.Energy < 6 {
		return fail("insufficient energy to betray party")
	}
	partyIdx := -1
	for i := range w.Parties {
		if w.Tick < w.Parties[i].ExpiresTick && ActorHasTag(w.Parties[i].MemberIDs, a.ID) {
			partyIdx = i
			break
		}
	}
	if partyIdx < 0 {
		return fail("actor is not in an active party")
	}
	p := w.Parties[partyIdx]
	var target *Actor
	canTarget := func(peer *Actor) bool {
		return peer != nil && peer.ID != a.ID && w.Tick >= peer.JailedUntil && SamePlace(*a, *peer)
	}
	if x.TargetID != "" {
		target = w.actor(x.TargetID)
		if !ActorHasTag(p.MemberIDs, x.TargetID) || !canTarget(target) {
			return fail("betrayal target must be a co-located living party member")
		}
	} else {
		mates := append([]string(nil), p.MemberIDs...)
		sort.Strings(mates)
		for _, id := range append([]string{p.LeaderID}, mates...) {
			if peer := w.actor(id); canTarget(peer) {
				target = peer
				break
			}
		}
		if target == nil {
			return fail("betrayal requires a co-located party member")
		}
	}
	x.TargetID = target.ID
	observers := w.awakeSamePlaceObservers(*a, a.ID, target.ID)
	witnessIDs := make([]string, 0, len(observers))
	for _, peer := range observers {
		witnessIDs = append(witnessIDs, peer.ID)
	}
	observers = append(observers, target)
	a.Energy -= 6
	// Exact 0-error coin seizure from target to betrayer.
	stolen := 0
	if target.Money > 0 {
		stolen = target.Money / 3
		if stolen < 2 {
			stolen = minInt(target.Money, 2)
		}
		if stolen > 30 {
			stolen = 30
		}
		w.AssessAndRecordOffense(*target, *a, "betray_party", 30, stolen, target.Money, 0, "hostile")
		target.Money -= stolen
		a.Money += stolen
		a.IllicitGain += stolen
	} else {
		w.AssessAndRecordOffense(*target, *a, "betray_party", 30, 0, target.Money, 0, "hostile")
	}
	dmg, absorbed, immune, broken := w.applyMeleeHit(a, target, x, maxInt(6, minInt(20, CombatPowerWithCatalog(w.Catalog, *a)/3)))

	// Expel betrayer from party (or disband if <=1 remains).
	remaining := removeString(p.MemberIDs, a.ID)
	if len(remaining) <= 1 {
		w.Parties = append(w.Parties[:partyIdx], w.Parties[partyIdx+1:]...)
	} else {
		p.MemberIDs = remaining
		if p.LeaderID == a.ID {
			sorted := append([]string(nil), remaining...)
			sort.Strings(sorted)
			p.LeaderID = sorted[0]
		}
		w.Parties[partyIdx] = p
	}

	// Collapse trust for all co-located party mates and record blood grudge.
	obs := Observation{
		Kind:      "party_betrayed",
		Modality:  "eyewitness",
		SubjectID: a.ID,
		ObjectID:  target.ID,
		Item:      "coins",
		Location:  a.Location,
		Quantity:  maxInt(1, stolen),
	}
	for _, mid := range remaining {
		if m := w.actor(mid); m != nil && !m.Dead && (m.ID == target.ID || SamePlace(*a, *m)) {
			if m.Trust == nil {
				m.Trust = map[string]int{}
			}
			m.Trust[a.ID] -= 5
		}
	}
	betrayEv := w.record(a, x, "party_betrayed", map[string]any{
		"text":           fmt.Sprintf("%s betrayed %s in %s, ambushing %s (-%d HP) and seizing %d coins", a.Name, p.Name, a.Location, target.Name, dmg, stolen),
		"party_id":       p.ID,
		"victim_id":      target.ID,
		"stolen_coins":   stolen,
		"coins":          stolen,
		"absorbed":       absorbed,
		"immune_blocked": immune,
		"witnesses":      witnessIDs,
		"damage":         dmg,
		"observation":    obs,
	}, observers...)

	events := append(broken, betrayEv)
	events = append(events, w.ensureRelations().recordGrudge(w, target.ID, a.ID, "party_betrayed", 50, betrayEv.ID, betrayEv.ID)...)
	events = append(events, w.finishMeleeHit(a, target, x, dmg, "party betrayal", betrayEv)...)
	return events
}

// partyCombatSupportBonus computes cooperative combat power contributed by co-located living party mates.
func (w *World) partyCombatSupportBonus(a Actor) int {
	p := w.ActorParty(a.ID)
	if p == nil || len(p.MemberIDs) <= 1 {
		return 0
	}
	bonus := 0
	sorted := append([]string(nil), p.MemberIDs...)
	sort.Strings(sorted)
	for _, mid := range sorted {
		if mid == a.ID {
			continue
		}
		m := w.actor(mid)
		if !w.partyMateAvailable(m) {
			continue
		}
		if SamePlace(a, *m) {
			cp := CombatPowerWithCatalog(w.Catalog, *m)
			bonus += maxInt(2, cp/4)
		}
	}
	if bonus > 60 {
		bonus = 60
	}
	return bonus
}

// partyEscortRiskReductionPct returns hazard mitigation percentage granted by co-located party escorts.
func (w *World) partyEscortRiskReductionPct(a Actor, destination string) int {
	p := w.ActorParty(a.ID)
	if p == nil || len(p.MemberIDs) <= 1 {
		return 0
	}
	mates := 0
	for _, mid := range p.MemberIDs {
		if mid == a.ID {
			continue
		}
		m := w.actor(mid)
		if w.partyMateAvailable(m) && (SamePlace(a, *m) || m.InTransit() && m.Location == a.Location && m.VenueID == a.VenueID && m.Task.Action.Location == destination) {
			mates++
		}
	}
	pct := mates * 20
	if pct > 50 {
		pct = 50
	}
	return pct
}

// settlePartyRewardSplit distributes earned coins fairly among co-located living party mates
// via exact actor-to-actor transfers (0 net minting -> 100% ledger conservation).
func (w *World) settlePartyRewardSplit(a *Actor, coins int, source Event) []Event {
	if w == nil || a == nil || a.Dead || coins < 2 {
		return nil
	}
	p := w.ActorParty(a.ID)
	if p == nil || len(p.MemberIDs) <= 1 {
		return nil
	}
	sorted := append([]string(nil), p.MemberIDs...)
	sort.Strings(sorted)
	var mates []*Actor
	for _, mid := range sorted {
		if mid == a.ID {
			continue
		}
		m := w.actor(mid)
		if w.partyMateAvailable(m) && SamePlace(*a, *m) {
			mates = append(mates, m)
		}
	}
	if len(mates) == 0 {
		return nil
	}
	totalPresent := len(mates) + 1
	available := minInt(coins, a.Money)
	leaderPresent := p.LeaderID == a.ID
	for _, m := range mates {
		leaderPresent = leaderPresent || m.ID == p.LeaderID
	}
	leaderShare := p.LootPolicy == "leader_share" && leaderPresent
	each, leaderCut := available/totalPresent, 0
	if leaderShare {
		leaderCut = (available + 1) / 2
		each = (available - leaderCut) / (totalPresent - 1)
	}
	shares := make(map[string]int, len(mates))
	totalPaid := 0
	for _, m := range mates {
		cut := each
		if leaderShare && m.ID == p.LeaderID {
			cut = leaderCut
		}
		shares[m.ID] = cut
		totalPaid += cut
	}
	if totalPaid <= 0 || totalPaid > a.Money {
		return nil
	}

	a.Money -= totalPaid
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	var observers []*Actor
	for _, m := range mates {
		cut := shares[m.ID]
		if cut <= 0 {
			continue
		}
		m.Money += cut
		if m.Trust == nil {
			m.Trust = map[string]int{}
		}
		a.Trust[m.ID]++
		m.Trust[a.ID]++
		observers = append(observers, m)
	}
	ev := w.record(a, Action{Kind: "wait", TargetID: p.ID, Quantity: totalPaid}, "party_loot_split", map[string]any{
		"text":           fmt.Sprintf("%s shared %d coins (%s) across %d party members of %s", a.Name, totalPaid, source.Kind, len(mates), p.Name),
		"party_id":       p.ID,
		"reason":         source.Kind,
		"cause_event_id": source.ID,
		"root_event_id":  payloadInt64(source.Payload, "root_event_id"),
		"total_split":    totalPaid,
		"recipients":     len(mates),
		"loot_policy":    p.LootPolicy,
	}, observers...)
	return []Event{ev}
}

func (w *World) partyMateAvailable(a *Actor) bool {
	return a != nil && !a.Dead && w.Tick >= a.JailedUntil &&
		(a.Task == nil || a.Task.Action.Kind != "sleep" && !a.Task.Dormant)
}

func (w *World) partyReadyToDepart(a Actor) bool {
	p := w.ActorParty(a.ID)
	if p == nil {
		return false
	}
	for _, id := range p.MemberIDs {
		member := w.actor(id)
		if !w.partyMateAvailable(member) || !SamePlace(a, *member) {
			return false
		}
	}
	return true
}

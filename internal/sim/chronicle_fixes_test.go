package sim

import (
	"testing"
)

func TestChannelTransmittedDeathNewsFormsGrudge(t *testing.T) {
	w := Village("remote-death-news")
	w.Catalog.CommChannels = map[string]CommChannelSpec{
		"signal": {ID: "signal", Name: "Signal", RangeKm: 100, PenetrationPower: 100, Broadcast: true},
	}
	w.Map.GeoRegions = []GeoRegion{{ID: "r1", Name: "Region", Coord: GeoCoord{X: 0, Y: 0}}}
	for i := range w.Map.Districts {
		w.Map.Districts[i].GeoRegionID = "r1"
	}
	sender := &w.Actors[0]
	victim := &w.Actors[1]
	kin := &w.Actors[2]
	killer := &w.Actors[3]
	sender.Energy = 80
	sender.Location = w.Map.Districts[0].ID
	kin.Location = w.Map.Districts[1].ID
	if kin.Trust == nil {
		kin.Trust = map[string]int{}
	}
	kin.Trust[sender.ID] = 2

	rs := w.ensureRelations()
	rs.Mentorships = append(rs.Mentorships, MentorshipBond{MentorID: victim.ID, ApprenticeID: kin.ID})
	w.InitLedgerAudit()

	deathEvID := w.NextID
	w.NextID++
	victim.Dead = true
	victim.DeathEventID = deathEvID
	sender.Memories = append(sender.Memories, Memory{
		Kind:        "actor_died",
		Tick:        w.Tick,
		EventID:     deathEvID,
		FactEventID: deathEvID,
		RootEventID: deathEvID,
		Source:      victim.ID,
		Text:        "saw murder",
		Observation: &Observation{Kind: "actor_died", SubjectID: killer.ID, ObjectID: victim.ID},
	})

	evs := w.Apply(sender.ID, Action{
		Kind:         "transmit_message",
		Channel:      "signal",
		BasisEventID: deathEvID,
		Text:         "killer slew mentor",
		Statement:    &Statement{Topic: "actor_died", SubjectID: killer.ID, ObjectID: victim.ID},
	})
	if len(evs) == 0 || evs[0].Kind != "channel_transmitted" {
		t.Fatalf("expected channel_transmitted event, got %+v", evs)
	}
	relEvs := rs.ObserveCausalRelationEvents(w, evs)
	foundGrudge := false
	for _, ev := range relEvs {
		if ev.Kind == "grudge_formed" && ev.ActorID == kin.ID && ev.TargetID == killer.ID {
			foundGrudge = true
			break
		}
	}
	if !foundGrudge {
		t.Fatalf("expected kin %s to form blood feud against %s after channel_transmitted death news, got %+v", kin.ID, killer.ID, relEvs)
	}
}

func TestWorldEventIDPrecedesCasualtiesAndLinksClaimRoot(t *testing.T) {
	w := Village("wev-causal-order")
	dist := w.Map.Districts[0].ID
	w.InitEcology(30)
	for i := range w.Actors {
		w.Actors[i].Location = dist
		w.Actors[i].InsideHouse = ""
		w.Actors[i].Energy = 90
	}
	w.Catalog.WorldEvents = []WorldEventSpec{
		{
			ID:               "quake_relic",
			Name:             "Shattered Vault",
			Category:         "artifact_emergence",
			DurationTicks:    200,
			Intensity:        90,
			CasualtyPermille: 1000,
			Damage:           15,
			RewardCoins:      20,
		},
	}
	w.InitLedgerAudit()

	evs := w.TriggerWorldEvent("quake_relic", dist)
	if len(evs) < 2 {
		t.Fatalf("expected trigger event plus casualties, got %+v", evs)
	}
	trig := evs[0]
	if trig.Kind != "world_event_triggered" {
		t.Fatalf("expected first event to be world_event_triggered, got %s", trig.Kind)
	}
	for _, sub := range evs[1:] {
		if sub.ID <= trig.ID {
			t.Fatalf("casualty event ID %d (%s) should be strictly greater than trigger ID %d", sub.ID, sub.Kind, trig.ID)
		}
		if root := payloadInt64(sub.Payload, "root_event_id"); root != trig.ID {
			t.Fatalf("casualty event %s missing root_event_id=%d, got %d", sub.Kind, trig.ID, root)
		}
	}

	// Find a surviving actor at dist and claim the relic.
	var claimer *Actor
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].Location == dist {
			claimer = &w.Actors[i]
			break
		}
	}
	if claimer == nil {
		t.Fatal("expected at least one living actor to claim world event")
	}
	claimEvs := w.Apply(claimer.ID, Action{Kind: "claim_world_event"})
	if len(claimEvs) == 0 || claimEvs[0].Kind != "world_event_claimed" {
		t.Fatalf("expected world_event_claimed, got %+v", claimEvs)
	}
	if root := payloadInt64(claimEvs[0].Payload, "root_event_id"); root != trig.ID {
		t.Fatalf("expected claim event root_event_id=%d, got %d", trig.ID, root)
	}
}

func TestDeclarativeNegativeDistrictResourceGainPreservesConservation(t *testing.T) {
	w := Village("resource-clamp-ledger")
	w.InitEcology(30)
	dist := w.Map.Districts[0].ID
	w.Actors[0].Location = dist
	w.Actors[0].Energy = 80
	w.Actors[0].Tags = appendUnique(w.Actors[0].Tags, "siphon_caster")
	ds := w.DistrictStates[dist]
	if ds.Resources == nil {
		ds.Resources = map[string]int64{}
	}
	ds.Resources["ore_vein"] = 2
	w.DistrictStates[dist] = ds
	w.Catalog.ActionSpecs = map[string]ActionSpec{
		"siphon_ore": {
			ID:                    "siphon_ore",
			Name:                  "Siphon Ore",
			GrantedByTags:         []string{"siphon_caster"},
			TargetKind:            "self",
			EnergyCost:            2,
			DistrictResourceGains: map[string]int64{"ore_vein": -5},
		},
	}
	if w.Catalog.TagActions == nil {
		w.Catalog.TagActions = map[string][]string{}
	}
	w.Catalog.TagActions["siphon_caster"] = []string{"siphon_ore"}
	w.InitLedgerAudit()

	evs := w.Apply(w.Actors[0].ID, Action{Kind: "siphon_ore"})
	if len(evs) == 0 || evs[0].Kind == "action_failed" {
		t.Fatalf("expected siphon_ore to succeed, got %+v", evs)
	}
	if got := w.DistrictStates[dist].Resources["ore_vein"]; got != 0 {
		t.Fatalf("expected ore_vein clamped to 0, got %d", got)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed after clamped district resource draw: %v", err)
	}
}

func TestPotableSelectionIsDeterministic(t *testing.T) {
	w := Village("drink-determinism")
	w.Catalog.Items["tea_a"] = ItemSpec{ID: "tea_a", Potable: true, ThirstRelief: 2}
	w.Catalog.Items["water_z"] = ItemSpec{ID: "water_z", Potable: true, ThirstRelief: 3}
	a := w.Actors[0]
	a.Items = map[string]int{"water_z": 1, "tea_a": 1}

	for i := 0; i < 32; i++ {
		if got := w.firstCarriedPotableItem(a); got != "tea_a" {
			t.Fatalf("iteration %d: expected deterministic lexical first potable tea_a, got %q", i, got)
		}
	}
}

func TestDiplomacyRejectsRankAndFileInHighLevelGuild(t *testing.T) {
	w := Village("diplomacy-auth")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	w.Factions[0].Level = 3

	member := &w.Actors[0]
	member.Guilds = []string{fA}
	member.GuildLevels = map[string]int{fA: 3}
	member.Tags = []string{"resident"}
	member.Energy = 90
	w.ensureRelations().FactionLeader[fA] = w.Actors[5].ID

	evs := w.Apply(member.ID, Action{Kind: "propose_alliance", FactionID: fA, TargetID: fB})
	if len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected rank-and-file guild member to be rejected from diplomacy, got %+v", evs)
	}
}

func TestSpotlightDoesNotDemoteNamedActorWithUnderscoreA(t *testing.T) {
	if hasCohortHydratedIDSuffix("lord_arthur") {
		t.Fatal("lord_arthur must not be treated as a cohort-hydrated actor ID")
	}
	if !hasCohortHydratedIDSuffix("c_farmers_a0007") {
		t.Fatal("c_farmers_a0007 should be recognized as a cohort-hydrated actor ID")
	}
}

func TestCannotJoinHostileOrWarringFaction(t *testing.T) {
	w := Village("no-join-hostile")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	a := &w.Actors[0]
	a.Guilds = []string{fA}
	a.Location = w.Factions[1].District
	a.InsideHouse = ""
	rel := w.ensureFactionDiplomacy(fA, fB)
	rel.Stance = "hostile"
	rel.RelationScore = -60

	evs := w.Apply(a.ID, Action{Kind: "join_faction", FactionID: fB})
	if len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected joining hostile faction to fail, got %+v", evs)
	}
}

func TestAllianceBreaksWhenRelationScoreCollapses(t *testing.T) {
	w := Village("alliance-break")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	w.Actors[0].Guilds = []string{fA}
	w.Actors[1].Guilds = []string{fB}
	rel := w.ensureFactionDiplomacy(fA, fB)
	rel.Stance = "allied"
	rel.RelationScore = -28

	dipEvs := w.stepDiplomacy([]Event{
		{ID: 50, Tick: w.Tick, ActorID: w.Actors[0].ID, TargetID: w.Actors[1].ID, Kind: "attacked", Payload: map[string]any{"damage": 15}},
	})
	if w.AreFactionsAllied(fA, fB) || rel.Stance != "hostile" {
		t.Fatalf("expected alliance to collapse into hostile at score %d, got stance=%s", rel.RelationScore, rel.Stance)
	}
	foundBreak := false
	for _, ev := range dipEvs {
		if ev.Kind == "alliance_broken" {
			foundBreak = true
			break
		}
	}
	if !foundBreak {
		t.Fatalf("expected alliance_broken event, got %+v", dipEvs)
	}
}

func TestBriberDoesNotSelfIncriminateOnRecipientsIllicitGain(t *testing.T) {
	w := Village("briber-no-self-expose")
	briber := &w.Actors[0]
	recipient := &w.Actors[1]
	briber.Personality = "righteous stern"
	briber.Tags = appendUnique(briber.Tags, "enforcer")
	w.Secrets = []ActorSecret{
		{ID: "s_bribe", OwnerID: briber.ID, Kind: "covert_bribe", SubjectID: recipient.ID, Severity: 60, KnownBy: []string{briber.ID, recipient.ID}},
		{ID: "s_gain", OwnerID: recipient.ID, Kind: "illicit_gain", SubjectID: briber.ID, Severity: 70, KnownBy: []string{recipient.ID, briber.ID}},
	}
	stance := w.ComputeSecretHoldStance(*briber, *recipient, w.Secrets[1])
	if stance == "confront" {
		t.Fatalf("briber should not adopt confront stance to expose their own bribe recipient, got %q", stance)
	}
}

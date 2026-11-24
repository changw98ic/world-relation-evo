package sim

import (
	"slices"
	"testing"
)

func TestFailedTransitionRecordsDeathWhenHealthReachesZero(t *testing.T) {
	for _, tc := range []struct {
		name   string
		health int
	}{
		{name: "survives", health: 23},
		{name: "dies", health: 22},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "attempt_transition")
			w.Catalog.Transitions = map[string]TransitionSpec{"stage": {ID: "stage", Ticks: 1, FailureHealthLoss: 22}}
			w.Actors[0].Health = tc.health
			events := w.Apply("a", Action{Kind: "attempt_transition", TransitionID: "stage"})
			if len(events) == 0 || events[0].Kind != "transition_failed" || w.Actors[0].Health != tc.health-22 {
				t.Fatalf("unexpected transition result: %+v", events)
			}
			if tc.health > 22 {
				if w.Actors[0].Dead || len(events) != 1 {
					t.Fatalf("failure at 23 health should leave one health and no death: %+v", events)
				}
				return
			}
			if len(events) != 2 || events[1].Kind != "actor_died" || events[1].Payload["cause"] != "transition_failure" {
				t.Fatalf("expected death attributed to the transition: %+v", events)
			}
			requireEventCause(t, events[1], events[0])
		})
	}
}

func TestVenueStrikeRecordsCauseForActiveAndStoredActors(t *testing.T) {
	w := lawWorld()
	w.Map.Venues = []Venue{{ID: "hall", DistrictID: "home"}}
	for i := range w.Actors {
		w.Actors[i].VenueID = "hall"
	}
	w.Actors[1].Health, w.Actors[2].Health = 1, 1
	if err := w.DehydrateActor("c"); err != nil {
		t.Fatal(err)
	}
	w.Catalog.ActionSpecs = map[string]ActionSpec{
		"strike": {ID: "strike", TargetKind: "venue", GrantedByTags: []string{"person"}, Ticks: 1,
			Attack: &AttackSpec{AreaScope: "venue", DamageByType: map[string]int{"physical": 100}}},
	}
	events := w.Apply("a", Action{Kind: "strike", Location: "hall"})
	if len(events) == 0 || events[0].Kind != "action_resolved" {
		t.Fatalf("expected area strike to be recorded: %+v", events)
	}
	dead := map[string]bool{}
	for _, event := range events[1:] {
		if event.Kind == "actor_died" {
			requireEventCause(t, event, events[0])
			dead[event.ActorID] = true
		}
	}
	if !dead["b"] || !dead["c"] {
		t.Fatalf("expected active and stored actors to have linked death events: %+v", events)
	}
}

func requireEventCause(t *testing.T, event, cause Event) {
	t.Helper()
	if event.ID <= cause.ID || payloadInt64(event.Payload, "cause_event_id") != cause.ID || event.causalRoot() != cause.causalRoot() {
		t.Fatalf("event %d (%s) did not follow cause %d (%s): %+v", event.ID, event.Kind, cause.ID, cause.Kind, event.Payload)
	}
}

func TestLineageFollowsDirectCauseBeforeCitedEvidence(t *testing.T) {
	events := []Event{
		{ID: 1, Kind: "speech"},
		{ID: 2, Kind: "attacked", Action: Action{BasisEventID: 1}},
		{ID: 3, Kind: "actor_died", Action: Action{BasisEventID: 1}, Payload: map[string]any{"cause_event_id": int64(2), "root_event_id": int64(1)}},
	}
	death, err := InspectLineage(events, 3)
	if err != nil {
		t.Fatal(err)
	}
	var chain []int64
	for _, node := range death.UpstreamChain {
		chain = append(chain, node.EventID)
	}
	if !slices.Equal(chain, []int64{1, 2, 3}) {
		t.Fatalf("expected the attack before the death, got %v", chain)
	}
	attack, err := InspectLineage(events, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(attack.DescendantIDs, []int64{3}) {
		t.Fatalf("expected the death as a consequence of the attack, got %v", attack.DescendantIDs)
	}
}

func TestAnomalyExposuresKeepTheirOwnCausesWhenTheyShareAReference(t *testing.T) {
	w := lawWorld()
	w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "gf_exchange", "expose_secret")
	w.Catalog.GoldenFingers = []GoldenFingerSpec{
		{ID: "anomaly", OwnerActorID: "a", ExposureRiskPermille: 1000,
			Shop: []GFShopItemSpec{{ID: "buy", Reward: GFRewardSpec{Coins: 1}}}},
	}
	spoken := w.Apply("a", Action{Kind: "speak", TargetID: "b", Text: "The shop opened"})
	lawOK(t, spoken)
	exchange := func() Event {
		t.Helper()
		events := w.Apply("a", Action{Kind: "gf_exchange", Text: "anomaly", Item: "buy", BasisEventID: spoken[0].ID})
		for _, event := range events {
			if event.Kind == "secret_caught_red_handed" {
				requireEventCause(t, event, events[0])
				if event.causalRoot() != spoken[0].ID {
					t.Fatalf("exposure lost the cited root: %+v", event)
				}
				return event
			}
		}
		t.Fatalf("missing exposure after exchange: %+v", events)
		return Event{}
	}
	first := exchange()
	lawOK(t, w.Apply("b", Action{Kind: "expose_secret", Item: first.Payload["secret_id"].(string)}))
	second := exchange()
	if second.Payload["secret_id"] == first.Payload["secret_id"] {
		t.Fatal("a later exchange reused the exposed secret from the earlier exchange")
	}
}

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

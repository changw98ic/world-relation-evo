package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestExposureOnlyReachesItsAudience(t *testing.T) {
	w := Village("audience")
	owner, speaker, outsider := &w.Actors[0], &w.Actors[1], &w.Actors[2]
	speaker.Location, speaker.VenueID, speaker.InsideHouse = owner.Location, owner.VenueID, owner.InsideHouse
	outsider.Location, outsider.VenueID, outsider.InsideHouse = "elsewhere", "", ""
	speaker.Energy = MaxEnergy
	w.addSecret(owner.ID, "covert_bribe", speaker.ID, 80, 0, []string{speaker.ID})
	secretID := w.Secrets[0].ID
	w.Secrets[0].HolderStances = map[string]string{speaker.ID: "confront", "another_holder": "shield"}
	seen := w.View(speaker.ID).KnownSecrets
	if len(seen) != 1 || len(seen[0].HolderStances) != 1 || seen[0].HolderStances[speaker.ID] != "confront" {
		t.Errorf("secret reveals private stances: %+v", seen)
	}
	events := w.Apply(speaker.ID, Action{Kind: "expose_secret", TargetID: owner.ID, Item: secretID})
	if len(events) == 0 || events[0].Kind != "secret_exposed" {
		t.Fatalf("exposure failed: %+v", events)
	}
	if len(w.View(outsider.ID).KnownSecrets) != 0 || owner.IllicitGain != 0 {
		t.Fatalf("exposure leaked to outsider or invented proceeds: %+v, %d", w.View(outsider.ID).KnownSecrets, owner.IllicitGain)
	}
}

func TestOffenseAccumulatedAtTickZeroDecays(t *testing.T) {
	if amount, repeats := DecayedOffense(OffenseRecord{RepeatCount: 4, Accumulated: 100}, 9*TicksPerDay); amount != 50 || repeats != 2 {
		t.Fatalf("got %d offense and %d repeats, want 50 and 2", amount, repeats)
	}
}

func TestVerifiedSecretReportRunsCourtAndReplays(t *testing.T) {
	w := Village("case-replay")
	w.Actors = w.Actors[:5]
	payer, suspect, witness, officer, outsider := &w.Actors[0], &w.Actors[1], &w.Actors[2], &w.Actors[3], &w.Actors[4]
	district := w.Map.Districts[0].ID
	for _, a := range []*Actor{payer, suspect, witness, officer, outsider} {
		a.Location, a.VenueID, a.InsideHouse = district, "", ""
		a.Energy, a.Health, a.Money = MaxEnergy, MaxHealth, 45
		a.Task = nil
	}
	w.Catalog.TagActions["court_test_officer"] = []string{"arrest", "audit_corruption", "leave_house"}
	w.Catalog.TagActions["court_test_payer"] = []string{"bribe"}
	payer.Tags = appendUnique(payer.Tags, "court_test_payer")
	suspect.Tags = appendUnique(suspect.Tags, "court_test_officer")
	officer.Tags = appendUnique(officer.Tags, "court_test_officer")
	officer.Attributes.Strength = 100
	officer.InsideHouse = w.Houses[0].ID
	officer.Trust[witness.ID] = -8
	outsider.Location = w.Map.Districts[2].ID
	w.Catalog.CommChannels = map[string]CommChannelSpec{"local": {ID: "local", PenetrationPower: 100}}
	w.ensureJusticeRevolt().Spec = &JusticeRevoltSpec{TrialSentenceTicks: 2}
	w.InitLedgerAudit()

	checkpoint, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var logs []TickDecisionLog
	var liveEvents []Event
	takeTurn := func(actions map[string]Action) []Event {
		t.Helper()
		events, log, err := w.StepRecorded(context.Background(), lawDecider(func(_ context.Context, p Perception) (Action, error) {
			if a, ok := actions[p.Self.ID]; ok {
				return a, nil
			}
			return Action{Kind: "wait"}, nil
		}))
		if err != nil {
			t.Fatal(err)
		}
		logs, liveEvents = append(logs, log), append(liveEvents, events...)
		return events
	}
	find := func(events []Event, kind, actorID string) Event {
		t.Helper()
		for _, e := range events {
			if e.Kind == kind && e.ActorID == actorID {
				return e
			}
		}
		t.Fatalf("missing %s by %s in %+v", kind, actorID, events)
		return Event{}
	}

	bribeEvents := takeTurn(map[string]Action{payer.ID: {Kind: "bribe", TargetID: suspect.ID}})
	bribe := find(bribeEvents, "bribed", payer.ID)
	caughtPayer := find(bribeEvents, "secret_caught_red_handed", payer.ID)
	caught := find(bribeEvents, "secret_caught_red_handed", suspect.ID)
	if caught.ID == caughtPayer.ID || payloadInt64(caught.Payload, "cause_event_id") != bribe.ID {
		t.Fatalf("bad red-handed evidence identifiers: %+v, %+v", caughtPayer, caught)
	}
	if got := w.View(officer.ID).KnownSecrets; len(got) != 0 {
		t.Fatalf("an officer separated from the scene knows the bribe: %+v", got)
	}
	secretID := relPayloadString(caught.Payload, "secret_id")
	gainBefore := suspect.IllicitGain
	exposure := find(takeTurn(map[string]Action{
		witness.ID: {Kind: "expose_secret", TargetID: suspect.ID, Item: secretID},
	}), "secret_exposed", witness.ID)
	if suspect.IllicitGain != gainBefore || len(w.View(outsider.ID).KnownSecrets) != 0 {
		t.Fatal("exposure changed proceeds or reached a remote person")
	}
	observation := exposure.Payload["observation"].(Observation)
	report := &Statement{Topic: observation.Kind, SubjectID: observation.SubjectID, ObjectID: observation.ObjectID,
		Item: observation.Item, Location: observation.Location, Quantity: observation.Quantity, Proceeds: observation.Proceeds}
	delivery := find(takeTurn(map[string]Action{
		witness.ID: {Kind: "transmit_message", Channel: "local", TargetID: officer.ID, Text: "Reporting the observed transaction",
			Statement: report, BasisEventID: exposure.ID},
		officer.ID: {Kind: "leave_house"},
	}), "channel_transmitted", witness.ID)
	if !ActorCorruptionEvidenceWithBasis(*officer, suspect.ID, delivery.ID) || len(w.View(officer.ID).KnownSecrets) != 1 {
		t.Fatalf("delivered report did not grant evidence and secret knowledge: %+v", delivery)
	}
	find(takeTurn(map[string]Action{officer.ID: {Kind: "arrest", TargetID: witness.ID}}), "action_failed", officer.ID)
	warrant := find(takeTurn(map[string]Action{officer.ID: {Kind: "open_case", TargetID: suspect.ID, BasisEventID: delivery.ID}}), "court_warrant_issued", officer.ID)
	arrest := find(takeTurn(map[string]Action{officer.ID: {Kind: "arrest", TargetID: suspect.ID}}), "arrested", officer.ID)
	if w.JusticeRevolt.CourtCases[0].Status != "arrested" || suspect.IllicitGain != gainBefore {
		t.Fatal("arrest must update custody and leave proceeds for the hearing")
	}
	hearing := find(takeTurn(map[string]Action{officer.ID: {Kind: "audit_corruption", TargetID: suspect.ID}}), "court_trial_held", officer.ID)
	if w.JusticeRevolt.CourtCases[0].Status != "convicted" || suspect.IllicitGain != 0 ||
		payloadInt64(hearing.Payload, "confiscated_coins") != int64(gainBefore) {
		t.Fatalf("trial failed to confiscate the actual bribe: %+v", hearing)
	}
	for _, e := range []Event{warrant, arrest, hearing} {
		if root := payloadInt64(e.Payload, "root_event_id"); root != bribe.ID {
			t.Fatalf("%s traces to %d, want original bribe %d", e.Kind, root, bribe.ID)
		}
	}
	if ActorCorruptionEvidenceWithBasis(*officer, suspect.ID, delivery.ID) {
		t.Fatal("resolved evidence remains citable as a fresh charge")
	}
	find(takeTurn(nil), "released_from_jail", suspect.ID)
	if w.JusticeRevolt.CourtCases[0].Status != "served" || suspect.JailedUntil != 0 {
		t.Fatal("release left a pending conviction or custody state")
	}
	replayed, replayEvents, err := ReplayFromCheckpoint(checkpoint, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	jsonBytes := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	if !bytes.Equal(jsonBytes(w), jsonBytes(replayed)) || !bytes.Equal(jsonBytes(liveEvents), jsonBytes(replayEvents)) {
		t.Fatal("replay diverged from the state or events of the full action sequence")
	}
	if report := InspectWorldConservation(w); !report.LedgerConserved {
		t.Fatal(report.LedgerError)
	}
}

func TestDeathNewsNeedsDeliveryAndARealCitation(t *testing.T) {
	w := Village("verified-death-news")
	sender, victim, killer, kin := &w.Actors[0], &w.Actors[1], &w.Actors[2], &w.Actors[3]
	place := w.Map.Districts[0].ID
	for _, a := range []*Actor{sender, victim, killer, kin} {
		a.Location, a.VenueID, a.InsideHouse, a.Task = place, "", "", nil
		a.Energy = MaxEnergy
	}
	w.Catalog.CommChannels = map[string]CommChannelSpec{"signal": {ID: "signal", PenetrationPower: 100}}
	rs := w.ensureRelations()
	rs.Kinship[victim.ID].SpouseID, rs.Kinship[kin.ID].SpouseID = kin.ID, victim.ID
	kin.Task = &Task{Action: Action{Kind: "sleep"}}
	victim.Health = 0
	death := w.markDeadIfZero(victim, killer, Action{Kind: "attack"}, "combat")[0]
	statement := &Statement{Topic: "actor_died", SubjectID: killer.ID, ObjectID: victim.ID, Item: "combat", Location: place, Quantity: 1}
	action := Action{Kind: "transmit_message", TargetID: kin.ID, Channel: "signal", Text: "The death was witnessed", Statement: statement, BasisEventID: death.ID}
	trust := kin.Trust[killer.ID]
	memoryCount := len(kin.Memories)
	events := w.Apply(sender.ID, action)
	rs.ObserveCausalRelationEvents(w, events)
	if len(kin.Memories) != memoryCount || kin.Trust[killer.ID] != trust {
		t.Fatalf("undelivered death news took effect: %+v", events)
	}
	kin.Task = nil
	action.BasisEventID = 0
	rs.ObserveCausalRelationEvents(w, w.Apply(sender.ID, action))
	if kin.Trust[killer.ID] != trust {
		t.Fatal("an uncited rumor caused a vendetta")
	}
	action.BasisEventID = death.ID
	rs.ObserveCausalRelationEvents(w, w.Apply(sender.ID, action))
	first := kin.Trust[killer.ID]
	rs.ObserveCausalRelationEvents(w, w.Apply(sender.ID, action))
	if first >= trust || kin.Trust[killer.ID] != first || len(kin.KnownHomicides) != 1 {
		t.Fatalf("confirmed murder must form a single vendetta: trust %d -> %d -> %d", trust, first, kin.Trust[killer.ID])
	}
}

func TestConcurrentHuntsDoNotPayForAFinishedThreat(t *testing.T) {
	for _, hasEstate := range []bool{false, true} {
		w := Village("concurrent-hunt")
		w.Actors = w.Actors[:3]
		district := w.Map.Districts[0].ID
		w.Catalog.Encounters = []EncounterSpec{{ID: "hazard", BountyCoins: 12}}
		w.Catalog.TagActions["hunter_test"] = []string{"hunt_threat"}
		for i := range w.Actors {
			a := &w.Actors[i]
			a.Location, a.VenueID, a.InsideHouse = district, "", ""
			a.Energy, a.Health, a.RestUntil = MaxEnergy, MaxHealth, 0
			a.Tags = appendUnique(a.Tags, "hunter_test")
		}
		ds := w.DistrictStates[district]
		ds.ThreatActivity = map[string]int{"hazard": 6}
		ds.BountyPool = 12
		w.DistrictStates[district] = ds
		if hasEstate {
			w.Cohorts = append(w.Cohorts, PopulationCohort{ID: "public_estate_test", District: district})
		}
		w.InitLedgerAudit()
		paid, exhausted := 0, 0
		for i := 0; i < 3; i++ {
			events, err := w.Step(context.Background(), lawDecider(func(_ context.Context, _ Perception) (Action, error) {
				return Action{Kind: "hunt_threat", TargetID: "hazard"}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				if e.Kind == "threat_hunted" && payloadInt64(e.Payload, "bounty_coins") > 0 {
					paid++
				}
				if e.Kind == "action_failed" && relPayloadString(e.Payload, "text") == "the tracked threat has already been suppressed" {
					exhausted++
				}
			}
		}
		if paid != 1 || exhausted != 2 || w.DistrictStates[district].ThreatActivity["hazard"] != 0 {
			t.Fatalf("estate=%v: got %d payments and %d exhausted hunts", hasEstate, paid, exhausted)
		}
		if report := InspectWorldConservation(w); !report.LedgerConserved {
			t.Fatal(report.LedgerError)
		}
	}
}

func TestCessionAndSuccessionTransferPolityAuthority(t *testing.T) {
	w := Village("authority-transfer")
	a, successor := &w.Actors[0], &w.Actors[1]
	f1, f2 := w.Factions[0].ID, w.Factions[1].ID
	f := &w.Factions[1]
	f.Members = []string{a.ID, successor.ID}
	district := w.Map.Districts[0].ID
	w.Map.Venues = []Venue{{ID: "seat", DistrictID: district, PolityID: f1}}
	rs := w.ensureRelations()
	w.setDistrictControl(rs, district, f1, 40)
	w.setFactionLeader(f2, a.ID)
	w.ensureFactionDiplomacy(f1, f2).ContestedDistrict = district
	if got := w.cedeContestedDistrict(f1, f2); got != district {
		t.Fatalf("ceded %q; want %q", got, district)
	}
	w.Polities.Nodes = append(w.Polities.Nodes, PolityNode{ID: "local_office", DeFactoControllerID: f2, LeaderActorID: a.ID})
	if d, _ := w.Map.DistrictByID(district); d.PolityID != f2 || w.Map.Venues[0].PolityID != f2 ||
		w.EffectiveGoverningPolity(district, "seat").LeaderActorID != a.ID {
		t.Fatal("cession did not change district and venue authority")
	}
	a.Health = 0
	w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "accident")
	if got := w.Polities.NodeByID(f2).LeaderActorID; got != successor.ID ||
		w.Polities.NodeByID("local_office").LeaderActorID != successor.ID {
		t.Fatalf("polity offices did not transfer to %s: %+v", successor.ID, w.Polities.Nodes)
	}
}

func TestCoercionUsesTheSameOffenseForPreviewAndSettlement(t *testing.T) {
	w := Village("coercion-preview")
	offender, victim := &w.Actors[0], &w.Actors[1]
	district := w.Map.Districts[0].ID
	for _, a := range []*Actor{offender, victim} {
		a.Location, a.VenueID, a.InsideHouse = district, "", ""
		a.Health, a.Energy, a.Money, a.RestUntil = MaxHealth, MaxEnergy, 50, 0
	}
	w.Catalog.TagActions["test_coercion"] = []string{"blackmail", "steal", "extort"}
	offender.Tags = appendUnique(offender.Tags, "test_coercion")
	offender.Personality, victim.Personality = "cunning", "timid"
	w.addSecret(victim.ID, "offense", offender.ID, 40, 0, []string{offender.ID})
	secret := &w.Secrets[0]
	secret.HolderStances[offender.ID] = "leverage"
	action := Action{Kind: "blackmail", TargetID: victim.ID, Item: secret.ID, Text: "subtle", Price: 2}
	preview := w.BlackmailAssessment(*victim, *offender, *secret, action, false)
	events := w.Apply(offender.ID, action)
	if events[0].Kind != "blackmailed" || payloadInt64(events[0].Payload, "accumulated_offense") != int64(preview.AccumulatedAfter) ||
		w.PreviewOffense(victim.ID, offender.ID, preview.ActionKey).Accumulated != preview.AccumulatedAfter {
		t.Fatalf("blackmail preview differs from settlement: %+v vs %+v", preview, events)
	}
	for _, kind := range []string{"steal", "extort"} {
		offender.Energy, offender.RestUntil, offender.Health = MaxEnergy, 0, MaxHealth
		events := w.Apply(offender.ID, Action{Kind: kind, TargetID: victim.ID})
		if len(events) == 0 || events[0].Kind == "action_failed" {
			t.Fatalf("%s did not reach settlement: %+v", kind, events)
		}
		if rec := w.PreviewOffense(victim.ID, offender.ID, kind); rec.RepeatCount != 1 || rec.Accumulated == 0 {
			t.Fatalf("%s did not record offense at tick zero: %+v", kind, rec)
		}
	}
	for attempt := 0; ; attempt++ {
		if attempt == 8 {
			t.Fatal("repeated coercion did not reach a breaking point")
		}
		offender.Energy = MaxEnergy
		preview = w.BlackmailAssessment(*victim, *offender, *secret, action, false)
		events = w.Apply(offender.ID, action)
		if preview.Outcome == "breaking_point" {
			if events[0].Kind != "blackmail_refused" || payloadInt64(events[0].Payload, "accumulated_offense") != int64(preview.AccumulatedAfter) {
				t.Fatalf("previewed breaking point did not match settlement: %+v vs %+v", preview, events)
			}
			break
		}
		if events[0].Kind != "blackmailed" {
			t.Fatalf("blackmail failed before the previewed breaking point: %+v", events)
		}
	}
}

func TestWorldEventCooldownAndInstanceKnowledgeSurviveExpiry(t *testing.T) {
	w := Village("world-event-cooldown")
	a := &w.Actors[0]
	district := w.Map.Districts[0].ID
	a.Location, a.VenueID, a.InsideHouse = district, "", ""
	w.Catalog.WorldEvents = []WorldEventSpec{{ID: "hazard", Category: "terrain_shift", DurationTicks: 1,
		CooldownTicks: TicksPerDay, ProbPermillePerDay: 1000, SeverMedium: "road"}}
	w.TriggerWorldEvent("hazard", district)
	w.TriggerWorldEvent("hazard", district)
	if len(w.ActiveWorldEvents) != 2 || w.ActiveWorldEvents[0].InstanceID == w.ActiveWorldEvents[1].InstanceID {
		t.Fatal("same-tick instances share an ID")
	}
	if len(w.View(a.ID).Roads) != 0 {
		t.Fatal("a traveler was shown a physically severed local route")
	}
	a.Task = &Task{Action: Action{Kind: "sleep"}}
	if len(w.View(a.ID).ActiveWorldEvents) != 0 {
		t.Fatal("sleeping actor can inspect active hazards")
	}
	w.Tick = 8
	if events := w.stepWorldEvents(); len(events) != 0 || len(w.ActiveWorldEvents) != 0 {
		t.Fatalf("expired event bypassed cooldown: %+v", events)
	}
	w.Tick = 8 + TicksPerDay
	if events := w.stepWorldEvents(); len(events) == 0 {
		t.Fatal("specification never triggered after its cooldown")
	}
	a.Task = nil
	if got := w.View(a.ID).ActiveWorldEvents; len(got) != 0 {
		t.Fatalf("old instance knowledge exposed a later event: %+v", got)
	}
}

func TestViewAndIndexedViewCopySkillsEquipmentAndAmbitions(t *testing.T) {
	w := Village("isolated-views")
	a, b := &w.Actors[0], &w.Actors[1]
	b.Location, b.InsideHouse, b.VenueID = a.Location, a.InsideHouse, a.VenueID
	a.Skills, a.SkillXP = map[string]int{"work": 1}, map[string]int{"work": 2}
	a.Equipped, a.Hobbies = map[string]string{"hand": "tool"}, []string{"music"}
	a.Ambitions = []ActorAmbition{{Kind: "master_skill", TargetID: "work", TargetValue: 3}}
	b.Skills = map[string]int{"work": 4}
	direct, indexed := w.View(a.ID), w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct, indexed) {
		t.Fatal("view entry points disagree")
	}
	indexed.Self.Skills["work"], indexed.Self.SkillXP["work"] = 99, 99
	indexed.Self.Equipped["hand"], indexed.Self.Hobbies[0], indexed.Self.Ambitions[0].Kind = "changed", "changed", "changed"
	for _, nb := range indexed.Nearby {
		if nb.ID == b.ID {
			nb.Skills["work"] = 99
		}
	}
	if a.Skills["work"] != 1 || a.SkillXP["work"] != 2 || a.Equipped["hand"] != "tool" ||
		a.Hobbies[0] != "music" || a.Ambitions[0].Kind != "master_skill" || b.Skills["work"] != 4 {
		t.Fatal("indexed perception shares mutable actor state")
	}
}

func TestEquippedEstateIsTransferredOnce(t *testing.T) {
	w := Village("equipped-estate")
	victim, heir := &w.Actors[0], &w.Actors[1]
	rs := w.ensureRelations()
	rs.Kinship[victim.ID].ChildIDs = []string{heir.ID}
	victim.Equipped = map[string]string{"hand": "tool"}
	w.InitLedgerAudit()
	before := heir.Items["tool"]
	victim.Health = 0
	death := w.markDeadIfZero(victim, nil, Action{Kind: "wait"}, "accident")[0]
	rs.ObservedEventIDs = nil
	rs.ObserveCausalRelationEvents(w, []Event{death})
	if !victim.EstateSettled || len(victim.Equipped) != 0 || heir.Items["tool"] != before+1 {
		t.Fatal("estate lost equipment or transferred it more than once")
	}
	if report := InspectWorldConservation(w); !report.LedgerConserved {
		t.Fatal(report.LedgerError)
	}
}

func TestRumorRelaysFromOneSourceDoNotCountAsIndependentWitnesses(t *testing.T) {
	w := Village("rumor-provenance")
	district := ""
	for _, d := range w.Map.Districts {
		if w.isPublicForumDistrict(d.ID) {
			district = d.ID
			break
		}
	}
	if district == "" {
		t.Fatal("test requires a public forum")
	}
	for i := range w.Actors {
		w.Actors[i].Location, w.Actors[i].VenueID, w.Actors[i].InsideHouse = district, "", ""
	}
	source := &w.Actors[0]
	statement := &Statement{Topic: "shortage", Item: "food", Location: district}
	send := func(from, to *Actor, basis int64) Event {
		events := w.Apply(from.ID, Action{Kind: "speak", TargetID: to.ID, Text: "There is a shortage", Statement: statement, BasisEventID: basis})
		if len(events) == 0 || events[0].Kind != "speech" {
			t.Fatalf("relay failed: %+v", events)
		}
		return events[0]
	}
	sr := w.ensureSpotlightRumor()
	sr.Spec.RumorCorroborationSpeakers = 3
	last := send(source, &w.Actors[1], 0)
	all := []Event{last}
	for i := 1; i < 5; i++ {
		last = send(&w.Actors[i], &w.Actors[i+1], last.ID)
		all = append(all, last)
	}
	if events := w.ObserveSpotlightAndRumors(sr, all); sr.TotalPanics != 0 || len(events) != 0 {
		t.Fatalf("a single-source story triggered panic: %+v", events)
	}
	for _, rumor := range sr.ActiveRumors {
		if len(rumor.Speakers) != 1 || rumor.Speakers[0] != source.ID {
			t.Fatalf("corroboration counted repeat speakers: %+v", rumor)
		}
	}
}

func TestPartyLeaderShareConservesCoinsWithAnotherMemberClaiming(t *testing.T) {
	w := Village("party-share")
	leader, claimer, other, sleeper := &w.Actors[0], &w.Actors[1], &w.Actors[2], &w.Actors[3]
	for _, a := range []*Actor{leader, claimer, other, sleeper} {
		a.Location, a.VenueID, a.InsideHouse, a.Money = leader.Location, "", "", 0
	}
	sleeper.Task = &Task{Action: Action{Kind: "sleep"}}
	w.Parties = []Party{{ID: "party", LeaderID: leader.ID, MemberIDs: []string{leader.ID, claimer.ID, other.ID, sleeper.ID},
		LootPolicy: "leader_share", ExpiresTick: 8}}
	w.InitLedgerAudit()
	w.adjustActorCurrency(claimer, "coins", 12, true)
	event := w.record(claimer, Action{Kind: "hunt_threat"}, "threat_hunted", map[string]any{"text": "bounty earned"})
	split := w.settlePartyRewardSplit(nil, claimer, 12, event)
	if leader.Money != 6 || claimer.Money != 3 || other.Money != 3 || sleeper.Money != 0 || len(split) != 1 {
		t.Fatalf("incorrect split: leader %d, claimer %d, other %d, sleeping %d", leader.Money, claimer.Money, other.Money, sleeper.Money)
	}
	if report := InspectWorldConservation(w); !report.LedgerConserved {
		t.Fatal(report.LedgerError)
	}
}

func TestSwearBondRejectsDuplicateAndUpdatesEligibility(t *testing.T) {
	w := Village("swear-bond-dedup")
	a, b := &w.Actors[0], &w.Actors[1]
	b.Location, b.VenueID, b.InsideHouse = a.Location, a.VenueID, a.InsideHouse
	a.Money = 20
	b.Money = 10
	a.Trust[b.ID] = 5
	b.Trust[a.ID] = 5
	w.InitLedgerAudit()

	viewBefore := w.View(a.ID)
	if viewBefore.localNeighbor(b.ID) == nil || viewBefore.localNeighbor(b.ID).KinshipToSelf != "" {
		t.Fatalf("expected no kinship with %s before swearing bond, got %+v", b.ID, viewBefore.Nearby)
	}

	ev1 := w.Apply(a.ID, Action{Kind: "swear_bond", TargetID: b.ID, Price: 2})
	if len(ev1) == 0 || ev1[0].Kind != "bond_sworn" {
		t.Fatalf("expected first swear_bond to emit bond_sworn, got %+v", ev1)
	}
	trustAfterFirst := a.Trust[b.ID]
	moneyAAfterFirst, moneyBAfterFirst := a.Money, b.Money

	viewAfter := w.View(a.ID)
	if viewAfter.localNeighbor(b.ID) == nil || viewAfter.localNeighbor(b.ID).KinshipToSelf != "sworn" {
		t.Fatalf("expected %s observed as sworn after the bond, got %+v", b.ID, viewAfter.Nearby)
	}
	viewBAfter := w.View(b.ID)
	if viewBAfter.localNeighbor(a.ID) == nil || viewBAfter.localNeighbor(a.ID).KinshipToSelf != "sworn" {
		t.Fatalf("expected %s observed as sworn in the other direction, got %+v", a.ID, viewBAfter.Nearby)
	}

	// Repeating from either side must be rejected without transferring coins or inflating trust.
	ev2 := w.Apply(a.ID, Action{Kind: "swear_bond", TargetID: b.ID, Price: 2})
	if len(ev2) == 0 || ev2[0].Kind != "action_failed" {
		t.Fatalf("expected duplicate swear_bond from A to fail, got %+v", ev2)
	}
	ev3 := w.Apply(b.ID, Action{Kind: "swear_bond", TargetID: a.ID, Price: 2})
	if len(ev3) == 0 || ev3[0].Kind != "action_failed" {
		t.Fatalf("expected duplicate swear_bond from B to fail, got %+v", ev3)
	}
	if a.Trust[b.ID] != trustAfterFirst || a.Money != moneyAAfterFirst || b.Money != moneyBAfterFirst {
		t.Fatalf("duplicate swear_bond mutated state: trust=%d/%d money=(%d,%d)/(%d,%d)",
			a.Trust[b.ID], trustAfterFirst, a.Money, b.Money, moneyAAfterFirst, moneyBAfterFirst)
	}

	// Patron bond duplicate check.
	evP1 := w.Apply(a.ID, Action{Kind: "swear_bond", TargetID: b.ID, Text: "patron"})
	if len(evP1) == 0 || evP1[0].Kind != "bond_sworn" {
		t.Fatalf("expected first patron bond to succeed, got %+v", evP1)
	}
	evP2 := w.Apply(a.ID, Action{Kind: "swear_bond", TargetID: b.ID, Text: "patron"})
	if len(evP2) == 0 || evP2[0].Kind != "action_failed" {
		t.Fatalf("expected duplicate patron bond to fail, got %+v", evP2)
	}
	if report := InspectWorldConservation(w); !report.LedgerConserved {
		t.Fatal(report.LedgerError)
	}
}

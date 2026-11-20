package sim

import "testing"

func TestEpistemicDeathNewsPropagationAndDelayedVendetta(t *testing.T) {
	w := Village("epistemic-death-test")
	w.Catalog.Skills = map[string]SkillSpec{
		"sword_art": {ID: "sword_art", Name: "剑道", MaxLevel: 6},
	}

	mentor := &w.Actors[0]
	apprentice := &w.Actors[1]
	killer := &w.Actors[2]
	witness := &w.Actors[3]

	dSafe := w.Map.Districts[0].ID
	dWild := w.Map.Districts[1].ID
	for _, a := range []*Actor{mentor, apprentice, killer, witness} {
		a.InsideHouse = ""
		a.VenueID = ""
		a.Health = 100
		a.Energy = 90
		if a.Trust == nil {
			a.Trust = map[string]int{}
		}
	}
	// Establish formal mentorship while together, then move mentor far away.
	mentor.Location, apprentice.Location = dSafe, dSafe
	mentor.Skills = map[string]int{"sword_art": 4}
	apprentice.Skills = map[string]int{"sword_art": 1}
	w.applyTeachSkill(mentor, Action{Kind: "teach_knowledge", SkillID: "sword_art", TargetID: apprentice.ID}, apprentice)

	// Murder happens in dWild: witness is on-site, mentor is far away in dSafe!
	mentor.Location = dSafe
	apprentice.Location, killer.Location, witness.Location = dWild, dWild, dWild
	mentor.Trust[killer.ID] = 0
	mentor.Trust[witness.ID] = 1

	apprentice.Health = 0
	deathEvs := w.markDeadIfZero(apprentice, killer, Action{Kind: "attack", TargetID: apprentice.ID}, "combat")
	if len(deathEvs) == 0 {
		t.Fatalf("expected death events")
	}

	// 1. Epistemic isolation: distant mentor did NOT witness the murder, so NO instant omniscient grudge!
	for _, g := range w.Relations.Grudges {
		if g.AggrievedID == mentor.ID && g.OffenderID == killer.ID {
			t.Fatalf("epistemic leak: distant mentor formed omniscient grudge before hearing death news: %+v", g)
		}
	}
	// Witness on-site DID observe the death with Observation{Kind: "actor_died"}.
	var deathMem Memory
	for _, m := range witness.Memories {
		if m.Kind == "actor_died" && m.Observation != nil && m.Observation.Kind == "actor_died" {
			deathMem = m
			break
		}
	}
	if deathMem.EventID == 0 || deathMem.Observation.ObjectID != apprentice.ID || deathMem.Observation.SubjectID != killer.ID {
		t.Fatalf("expected on-site witness to receive structured actor_died Observation, got %+v", witness.Memories)
	}

	// 2. Witness travels to mentor's district and relays the death news via speech with BasisEventID & Statement.
	witness.Location = dSafe
	stmt := &Statement{
		Topic:     "actor_died",
		SubjectID: killer.ID,
		ObjectID:  apprentice.ID,
		Item:      "combat",
		Location:  dWild,
		Quantity:  1,
	}
	speakEvs := w.Apply(witness.ID, Action{
		Kind:         "speak",
		TargetID:     mentor.ID,
		Text:         "I witnessed your apprentice killed by " + killer.ID,
		Statement:    stmt,
		BasisEventID: deathMem.EventID,
	})
	w.ensureRelations().ObserveCausalRelationEvents(w, speakEvs)

	// 3. NOW mentor has heard the death news and forms the protective blood-feud grudge!
	if mentor.Trust[killer.ID] >= 0 {
		t.Fatalf("expected mentor trust toward killer to drop after hearing death news, got %d", mentor.Trust[killer.ID])
	}
	foundDelayedGrudge := false
	for _, g := range w.Relations.Grudges {
		if g.AggrievedID == mentor.ID && g.OffenderID == killer.ID && g.Cause == "homicide_reported" {
			foundDelayedGrudge = true
			break
		}
	}
	if !foundDelayedGrudge {
		t.Fatalf("expected mentor to form homicide_reported grudge after hearing witness report, got grudges=%+v", w.Relations.Grudges)
	}
}

func TestDynamicWorldEventsArtifactTerrainAndInvasion(t *testing.T) {
	w := Village("world-events-test")
	d0, d1 := w.Map.Districts[0].ID, w.Map.Districts[1].ID
	w.Catalog.Forms = map[string]FormSpec{
		"mortal":     {ID: "mortal", PowerTier: 1},
		"foundation": {ID: "foundation", PowerTier: 3},
	}
	w.Catalog.LocomotionModes = map[string]LocomotionSpec{
		"sword_flight": {ID: "sword_flight", GrantedByTags: []string{"flying_sword"}, AllowedMediums: []string{"road", "air"}, SpeedPct: 200},
	}
	w.Catalog.WorldEvents = []WorldEventSpec{
		{
			ID:                "spirit_relic",
			Name:              "太古灵宝出世",
			Category:          "artifact_emergence",
			DurationTicks:     200,
			Intensity:         60,
			RewardItem:        "food",
			RewardQty:         2,
			RewardCoins:       25,
			RequiredPowerTier: 2,
		},
		{
			ID:               "crust_rift",
			Name:             "地壳裂开",
			Category:         "terrain_shift",
			DurationTicks:    200,
			Intensity:        70,
			SeverMedium:      "road",
			AddFeatures:      []string{"spirit_mine"},
			ResourceDeposits: map[string]int64{"iron_vein": 500},
		},
		{
			ID:                  "alien_vanguard",
			Name:                "外星先遣队入侵",
			Category:            "external_invasion",
			DurationTicks:       200,
			Intensity:           80,
			InvasionThreatID:    w.Catalog.Encounters[0].ID,
			InvasionThreatBoost: 40,
			BountyBoost:         100,
		},
	}

	// 1. Artifact emergence: low-tier mortal is blocked by RequiredPowerTier=2; foundation actor claims it!
	mortal := &w.Actors[0]
	cult := &w.Actors[1]
	mortal.Location, mortal.InsideHouse, mortal.VenueID = d0, "", ""
	cult.Location, cult.InsideHouse, cult.VenueID = d0, "", ""
	mortal.FormID, cult.FormID = "mortal", "foundation"
	mortal.Energy, cult.Energy = 90, 90
	mortal.Tags = appendUnique(mortal.Tags, "resident")
	cult.Tags = appendUnique(cult.Tags, "resident")

	evs := w.TriggerWorldEvent("spirit_relic", d0)
	if len(evs) == 0 || evs[0].Kind != "world_event_triggered" {
		t.Fatalf("expected world_event_triggered, got %+v", evs)
	}
	if len(w.ActiveWorldEvents) != 1 {
		t.Fatalf("expected 1 active world event, got %d", len(w.ActiveWorldEvents))
	}
	instID := w.ActiveWorldEvents[0].InstanceID

	failEvs := w.Apply(mortal.ID, Action{Kind: "claim_world_event", TargetID: instID})
	if len(failEvs) == 0 || failEvs[0].Kind != "action_failed" {
		t.Fatalf("expected mortal (PowerTier 1) to fail claiming Tier-2 artifact, got %+v", failEvs)
	}
	beforeCoins := cult.Money
	beforeFood := cult.Items["food"]
	claimEvs := w.Apply(cult.ID, Action{Kind: "claim_world_event", TargetID: instID})
	if len(claimEvs) == 0 || claimEvs[0].Kind != "world_event_claimed" {
		t.Fatalf("expected foundation actor to claim artifact, got %+v", claimEvs)
	}
	if cult.Money != beforeCoins+25 || cult.Items["food"] != beforeFood+2 {
		t.Fatalf("expected reward (+25 coins, +2 food), got coins %d->%d food %d->%d", beforeCoins, cult.Money, beforeFood, cult.Items["food"])
	}

	// 2. Terrain shift severs road travel for walkers, while flying_sword bypasses it, and adds new feature+resource!
	w.TriggerWorldEvent("crust_rift", d0)
	if !w.Map.DistrictHasFeature(d0, "spirit_mine") {
		t.Fatalf("expected terrain_shift to add feature spirit_mine to %s", d0)
	}
	if w.DistrictStates[d0].Resources["iron_vein"] < 500 {
		t.Fatalf("expected terrain_shift to deposit 500 iron_vein, got %d", w.DistrictStates[d0].Resources["iron_vein"])
	}
	if severed, _ := w.isRouteSevered(d0, d1, "road", "walk"); !severed {
		t.Fatalf("expected road route touching %s to be severed for walkers", d0)
	}
	if severed, _ := w.isRouteSevered(d0, d1, "road", "sword_flight"); severed {
		t.Fatalf("expected air-capable sword_flight to fly over severed road fissure")
	}

	// 3. External invasion spikes district ThreatActivity and BountyPool!
	encID := w.Catalog.Encounters[0].ID
	beforeThreat := w.DistrictStates[d1].ThreatActivity[encID]
	beforeBounty := w.DistrictStates[d1].BountyPool
	w.TriggerWorldEvent("alien_vanguard", d1)
	if w.DistrictStates[d1].ThreatActivity[encID] < beforeThreat+40 || w.DistrictStates[d1].BountyPool < beforeBounty+100 {
		t.Fatalf("expected external_invasion to spike threat (+40) and bounty (+100), got threat %d->%d bounty %d->%d",
			beforeThreat, w.DistrictStates[d1].ThreatActivity[encID], beforeBounty, w.DistrictStates[d1].BountyPool)
	}

	// 4. Ledger conservation stays 100% green after all 3 world event types!
	if rep := InspectWorldConservation(w); !rep.LedgerConserved {
		t.Fatalf("conservation violated after world events: %s", rep.LedgerError)
	}
}

package sim

import "testing"

func TestDiplomacyProjectionRequiresEvidenceOfCurrentAgreement(t *testing.T) {
	a := lawPerson("observer")
	a.Memories = []Memory{{
		EventID: 10, Tick: 5,
		Observation: &Observation{Kind: "faction_war_declared", SubjectID: "alpha", ObjectID: "beta"},
	}}
	current := FactionDiplomacy{FactionA: "alpha", FactionB: "beta", Stance: "neutral", WarStartedTick: 5, RootEventID: 20}
	if actorKnowsDiplomacyUpdate(a, current) {
		t.Fatal("witnessing the old war revealed the new peace treaty")
	}
	a.Memories = append(a.Memories, Memory{EventID: 21, FactEventID: 20, Statement: &Statement{Topic: "peace_treaty_signed"}})
	if !actorKnowsDiplomacyUpdate(a, current) {
		t.Fatal("verified news of the current treaty was hidden")
	}
}

func TestP4EpistemicIsolationPartiesDisguiseDiplomacyCitationsAndRumors(t *testing.T) {
	w := Village("p4-epistemic")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""

	// 1. Party visibility requires SamePlace & awake (not district-wide).
	w.Parties = []Party{{
		ID:          "party_1",
		Name:        "Square Raiders",
		LeaderID:    b.ID,
		MemberIDs:   []string{b.ID},
		ExpiresTick: w.Tick + 100,
	}}
	// Put a inside a house -> must NOT see b's outdoor party!
	a.InsideHouse = w.Houses[0].ID
	if got := w.perceivedParties(*a); len(got) != 0 {
		t.Fatalf("indoor actor must not see outdoor party, got %+v", got)
	}
	// Put a outdoors but asleep -> must NOT see b's party!
	a.InsideHouse = ""
	a.Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 4}
	if got := w.perceivedParties(*a); len(got) != 0 {
		t.Fatalf("sleeping actor must not see outdoor party, got %+v", got)
	}
	a.Task = nil
	if got := w.perceivedParties(*a); len(got) != 1 {
		t.Fatalf("awake co-located actor should see party, got %d", len(got))
	}

	// 2. Disguised actor must NOT leak true FormSpec.InnateTags in neighborEffectiveTags.
	w.Catalog.Forms = map[string]FormSpec{
		"mortal": {ID: "mortal", Name: "Mortal", PowerTier: 1, InnateTags: []string{"commoner"}},
		"demon":  {ID: "demon", Name: "Demon Lord", PowerTier: 5, InnateTags: []string{"demon_aura", "forbidden"}},
	}
	w.Catalog.GoldenFingers = []GoldenFingerSpec{{
		ID:            "veil_gf",
		Name:          "Veil",
		GrantedByTags: []string{"veiled"},
		Passive:       &GFPassiveSpec{DisguiseFormID: "mortal"},
	}}
	b.FormID = "demon"
	b.Tags = appendUnique(b.Tags, "veiled")
	tags := w.neighborEffectiveTags(*b)
	if ActorHasTag(tags, "demon_aura") || ActorHasTag(tags, "forbidden") {
		t.Fatalf("disguised actor leaked true FormSpec innate tags: %v", tags)
	}
	if !ActorHasTag(tags, "commoner") {
		t.Fatalf("expected perceived disguise form tag 'commoner', got %v", tags)
	}

	// 3. Unseen distant war (RootEventID > 0) must NOT leak via JoinBlockReason or Nearby.FactionRelation,
	// and fabricated uncited speech must NOT unlock FactionDiplomacy.
	w.Factions = append(w.Factions,
		Faction{ID: "alpha", Name: "Alpha", District: "square", Members: []string{a.ID}},
		Faction{ID: "beta", Name: "Beta", District: "square", Members: []string{b.ID}},
	)
	a.Guilds = []string{"alpha"}
	b.Guilds = []string{"beta"}
	key, fA, fB := diplomacyKey("alpha", "beta")
	w.Diplomacy = &DiplomacyState{
		Relations: map[string]*FactionDiplomacy{
			key: {
				FactionA:       fA,
				FactionB:       fB,
				Stance:         "at_war",
				RelationScore:  -90,
				WarStartedTick: w.Tick,
				RootEventID:    999,
			},
		},
	}
	p := w.View(a.ID)
	for _, fv := range p.Factions {
		if fv.ID == "beta" && fv.JoinBlockReason != "" {
			t.Fatalf("unseen war leaked into Perception.Factions[beta].JoinBlockReason: %q", fv.JoinBlockReason)
		}
	}
	for _, nb := range p.Nearby {
		if nb.ID == b.ID && nb.FactionRelation == "at_war" {
			t.Fatalf("unseen war leaked into Perception.Nearby[%s].FactionRelation", b.ID)
		}
	}
	// Fabricated uncited speech (FactEventID == 0, WitnessID == "") must NOT unlock diplomacy!
	a.Memories = appendMemory(a.Memories, Memory{
		Kind:    "speech",
		Tick:    w.Tick,
		EventID: 1001,
		Source:  b.ID,
		Statement: &Statement{
			Topic:     "faction_war_declared",
			SubjectID: fA,
			ObjectID:  fB,
		},
	})
	if len(w.perceivedFactionDiplomacy(*a)) != 0 {
		t.Fatalf("fabricated uncited speech unlocked objective FactionDiplomacy")
	}

	// 4. citedReport must reject forged Quantity and must not stamp WitnessID on unverified relays.
	a.Memories = appendMemory(a.Memories, Memory{
		Kind:    "item_produced",
		Tick:    w.Tick,
		EventID: 2001,
		Source:  a.ID,
		Observation: &Observation{
			Kind:     "item_produced",
			Item:     "food",
			Quantity: 2,
			Location: "square",
		},
	})
	if factID, wit := citedReport(a, Action{
		Kind:         "speak",
		BasisEventID: 2001,
		Statement:    &Statement{Topic: "item_produced", Item: "food", Quantity: 999, Location: "square"},
	}); factID != 0 || wit != "" {
		t.Fatalf("citedReport certified forged quantity: factID=%d wit=%q", factID, wit)
	}
	if factID, _ := citedReport(a, Action{
		Kind:         "speak",
		BasisEventID: 1001,
		Statement:    &Statement{Topic: "faction_war_declared", SubjectID: fA, ObjectID: fB},
	}); factID != 0 {
		t.Fatalf("citedReport certified unverified relay with factID=%d", factID)
	}

	// 5. Unverified rumor speech must NOT mutate objective DistrictState.MoraleBonus.
	if w.DistrictStates == nil {
		w.DistrictStates = map[string]DistrictState{}
	}
	w.DistrictStates["square"] = DistrictState{DistrictID: "square", MoraleBonus: 10}
	sr := w.ensureSpotlightRumor()
	c := &w.Actors[2]
	c.Location = "square"
	unverifiedEv1 := Event{
		ID:      3001,
		Tick:    w.Tick,
		ActorID: a.ID,
		Kind:    "speech",
		Action:  Action{Kind: "speak", Text: "famine shortage panic! food is gone!"},
		Payload: map[string]any{"district": "square"},
	}
	unverifiedEv2 := Event{
		ID:      3002,
		Tick:    w.Tick,
		ActorID: c.ID,
		Kind:    "speech",
		Action:  Action{Kind: "speak", Text: "famine shortage panic! food is gone!"},
		Payload: map[string]any{"district": "square"},
	}
	_, _ = w.processRumorSpeech(sr, unverifiedEv1)
	_, _ = w.processRumorSpeech(sr, unverifiedEv2)
	if w.DistrictStates["square"].MoraleBonus != 10 {
		t.Fatalf("unverified rumor mutated objective DistrictState.MoraleBonus to %d", w.DistrictStates["square"].MoraleBonus)
	}
}

func TestBlock2EpistemicSealsDistrictStateWalletsDisguiseAndTelepathy(t *testing.T) {
	w := Village("block2-epistemic-seals")
	civ := &w.Actors[0]
	trader := &w.Actors[1]
	remoteMentor := &w.Actors[2]
	attacker := &w.Actors[3]

	civ.Location, trader.Location, attacker.Location = "square", "square", "square"
	civ.InsideHouse, trader.InsideHouse, attacker.InsideHouse = "", "", ""
	civ.Tags = []string{"person", "commoner"}
	trader.Tags = []string{"person", "trader"}
	remoteMentor.Location = "mill"
	remoteMentor.InsideHouse = ""

	// 1. DistrictState secret enforcement internals & hydraulic/patrol schedules must not leak to civilians.
	if w.DistrictStates == nil {
		w.DistrictStates = map[string]DistrictState{}
	}
	w.DistrictStates["square"] = DistrictState{
		DistrictID:       "square",
		Corruption:       45,
		RacketPressure:   60,
		BribeShieldTicks: 30,
		Patrols:          []EcologyPatrol{{CohortID: "patrol_1"}},
		WaterOverflow:    map[string]int64{"freshwater": 10},
	}
	pCiv := w.View(civ.ID)
	if pCiv.DistrictState == nil {
		t.Fatalf("expected local DistrictState in perception")
	}
	if pCiv.DistrictState.BribeShieldTicks != 0 || pCiv.DistrictState.Corruption != 0 || pCiv.DistrictState.RacketPressure != 0 || len(pCiv.DistrictState.Patrols) != 0 || len(pCiv.DistrictState.WaterOverflow) != 0 {
		t.Fatalf("civilian perception leaked internal DistrictState fields: %+v", *pCiv.DistrictState)
	}

	// 2. Trader eligibility facts must not depend on the lender's private wallet.
	trader.Money = 0
	pCiv = w.View(civ.ID)
	if peer := pCiv.localNeighbor(trader.ID); peer == nil || !ActorHasTag(peer.Tags, pCiv.CreditTerms.TraderTag) {
		t.Fatalf("expected the trader's observable role with an empty wallet, got %+v", peer)
	}
	// Attempting borrow_loan against penniless trader still fails at settlement.
	if evs := w.Apply(civ.ID, Action{Kind: "borrow_loan", TargetID: trader.ID}); len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected settlement borrow_loan to reject penniless trader, got %+v", evs)
	}

	// 3. Offer acceptance must not peek at buyer's private wallet when seller responds to a buy-offer.
	trader.Items["food"] = 5
	civ.Money = 0
	w.Offers = append(w.Offers, Offer{ID: 77, Seller: trader.ID, Buyer: civ.ID, ProposedBy: civ.ID, Item: "food", Quantity: 2, Price: 4, Expires: w.Tick + 10})
	pTrader := w.View(trader.ID)
	if reason := pTrader.OfferAcceptanceBlockReason(w.Offers[len(w.Offers)-1]); reason != "" {
		t.Fatalf("seller perception peeked into buyer's private wallet: %q", reason)
	}

	// 4. Cross-district mentor must not telepathically lose trust when mentee is attacked elsewhere.
	rs := w.ensureRelations()
	rs.Mentorships = append(rs.Mentorships, MentorshipBond{MentorID: remoteMentor.ID, ApprenticeID: civ.ID})
	if remoteMentor.Trust == nil {
		remoteMentor.Trust = map[string]int{}
	}
	remoteMentor.Trust[attacker.ID] = 5
	rs.ObserveCausalRelationEvents(w, []Event{{ID: 8888, Tick: w.Tick, ActorID: attacker.ID, TargetID: civ.ID, Kind: "attacked", Payload: map[string]any{"damage": 10}}})
	if remoteMentor.Trust[attacker.ID] != 5 {
		t.Fatalf("remote mentor telepathically changed trust across districts: got %d want 5", remoteMentor.Trust[attacker.ID])
	}
}

func TestRemoteAssetsExposeDeedsAndLocalAssetsExposeCondition(t *testing.T) {
	w := Village("local-asset-observation")
	a := &w.Actors[0]
	a.Location, a.InsideHouse, a.VenueID, a.HomeID = "square", "", "", "remote-house"
	w.Houses = []House{{ID: a.HomeID, District: "mill", OwnerID: a.ID, Condition: 23}}
	w.Production.Fields = []FieldAsset{{ID: "remote-field", District: "mill", AreaM2: 1000, UserKind: "actor", UserID: a.ID}}
	w.Growth = []GrowthBatch{{ID: 999, OwnerID: a.ID, District: "mill", FieldID: "remote-field", AreaM2: 500, Quantity: 8, Ready: true}}

	remote := w.View(a.ID)
	if len(remote.AccessibleHouses) != 0 || len(remote.Fields) != 0 || len(remote.Growth) != 0 {
		t.Fatalf("remote perception contains live asset data: houses=%v fields=%v growth=%v", remote.AccessibleHouses, remote.Fields, remote.Growth)
	}
	if home, known := remote.KnownHouse(a.HomeID); !known || home.District != "mill" || len(remote.FieldDeeds) != 1 {
		t.Fatalf("missing known property locations: home=%+v fields=%v", home, remote.FieldDeeds)
	}

	a.Location = "mill"
	local := w.View(a.ID)
	if len(local.AccessibleHouses) != 1 || local.AccessibleHouses[0].Condition != 23 ||
		len(local.Growth) != 1 || !local.Growth[0].Ready || local.Growth[0].Quantity != 8 ||
		len(local.Fields) != 1 || local.Fields[0].AvailableAreaM2 != 500 {
		t.Fatalf("local observations lack actual asset condition: houses=%v fields=%v growth=%v", local.AccessibleHouses, local.Fields, local.Growth)
	}
}

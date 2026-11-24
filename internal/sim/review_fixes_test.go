package sim

import (
	"testing"
)

func TestJoinFactionEligibilityParityBetweenViewAndApply(t *testing.T) {
	w := Village("join-parity")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	a := &w.Actors[0]
	a.Guilds = []string{fA}
	a.Location = w.Factions[1].District
	a.InsideHouse = ""
	rel := w.ensureFactionDiplomacy(fA, fB)
	rel.Stance = "at_war"
	rel.RelationScore = -80

	// Direct View and viewIndexed must both report JoinBlockReason for fB.
	direct := w.View(a.ID)
	indexed := w.viewIndexed(0, w.buildStepIndex())
	for _, view := range []Perception{direct, indexed} {
		found := false
		for _, fv := range view.Factions {
			if fv.ID == fB {
				found = true
				if fv.JoinBlockReason == "" {
					t.Fatalf("expected non-empty JoinBlockReason for warring faction %s", fB)
				}
			}
		}
		if !found {
			t.Fatalf("missing faction %s in view", fB)
		}
	}
	evs := w.Apply(a.ID, Action{Kind: "join_faction", FactionID: fB})
	if len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected join_faction to fail against warring faction, got %+v", evs)
	}
}

func TestDiplomacyAnnouncementRespectsLocationAndSleep(t *testing.T) {
	w := Village("diplomacy-epistemic")
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID
	leaderA := &w.Actors[0]
	remoteMember := &w.Actors[1]
	sleepingWitness := &w.Actors[2]
	awakeWitness := &w.Actors[3]

	leaderA.Guilds = []string{fA}
	leaderA.Tags = appendUnique(leaderA.Tags, "ruler")
	leaderA.Energy = 90
	leaderA.Location = w.Map.Districts[0].ID
	leaderA.InsideHouse = ""
	w.ensureRelations().FactionLeader[fA] = leaderA.ID
	w.Factions[0].Members = appendUnique(w.Factions[0].Members, leaderA.ID)
	w.Factions[0].Members = appendUnique(w.Factions[0].Members, remoteMember.ID)
	w.Factions[0].Members = appendUnique(w.Factions[0].Members, sleepingWitness.ID)

	// Remote member in a different district.
	remoteMember.Guilds = []string{fA}
	remoteMember.Location = w.Map.Districts[1].ID
	remoteMember.InsideHouse = ""

	// Co-located member who is asleep.
	sleepingWitness.Guilds = []string{fA}
	sleepingWitness.Location = leaderA.Location
	sleepingWitness.InsideHouse = ""
	sleepingWitness.Task = &Task{Action: Action{Kind: "sleep"}, Remaining: 4}

	// Awake co-located witness.
	awakeWitness.Guilds = []string{fB}
	awakeWitness.Location = leaderA.Location
	awakeWitness.InsideHouse = ""

	w.Tick = 10
	evs := w.Apply(leaderA.ID, Action{Kind: "declare_war", FactionID: fA, TargetID: fB, Text: "border_dispute"})
	if len(evs) == 0 || evs[0].Kind != "faction_war_declared" {
		t.Fatalf("expected faction_war_declared, got %+v", evs)
	}

	hasWarMem := func(a Actor) bool {
		for _, m := range a.Memories {
			if m.Kind == "faction_war_declared" {
				return true
			}
		}
		return false
	}
	if hasWarMem(*remoteMember) {
		t.Fatalf("remote faction member must not instantly receive faction_war_declared memory")
	}
	if hasWarMem(*sleepingWitness) {
		t.Fatalf("sleeping faction member must not instantly receive faction_war_declared memory")
	}
	if !hasWarMem(*awakeWitness) {
		t.Fatalf("awake co-located witness should receive faction_war_declared observation memory")
	}
	if dips := w.View(remoteMember.ID).FactionDiplomacy; len(dips) != 0 {
		t.Fatalf("remote member must not see uncommunicated war in perceivedFactionDiplomacy, got %+v", dips)
	}
	if dips := w.View(awakeWitness.ID).FactionDiplomacy; len(dips) == 0 || dips[0].Stance != "at_war" {
		t.Fatalf("awake witness should perceive the declared war, got %+v", dips)
	}
}

func TestGoldenFingerDisguiseHidesFormIDAndDeterministicMultiCurrencyPenalty(t *testing.T) {
	makeWorld := func() *World {
		w := Village("gf-disguise-det")
		w.Catalog.Forms = map[string]FormSpec{
			"mortal":  {ID: "mortal", Name: "凡胎", PowerTier: 1, CombatBonus: 0},
			"nascent": {ID: "nascent", Name: "元婴", PowerTier: 5, CombatBonus: 40},
		}
		w.Catalog.GoldenFingers = []GoldenFingerSpec{
			{
				ID:           "gf_test",
				Name:         "遮天伪装系统",
				OwnerActorID: w.Actors[0].ID,
				Passive: &GFPassiveSpec{
					DisguisePowerTier: 1,
				},
				Quests: []GFQuestSpec{
					{
						ID:           "q_multi_cur",
						Title:        "多币种惩罚任务",
						Kind:         "visit_district",
						TargetID:     "nonexistent",
						DeadlineTick: 5,
						FailurePenalty: &GFPenaltySpec{
							CurrencyLoss: map[string]int64{
								"z_pts": 10,
								"a_pts": 10,
								"m_pts": 10,
								"c_pts": 10,
							},
						},
					},
				},
			},
		}
		w.Actors[0].FormID = "nascent"
		w.Actors[0].Currencies = map[string]int64{"z_pts": 20, "a_pts": 20, "m_pts": 20, "c_pts": 20}
		w.Actors[0].Location = w.Actors[1].Location
		w.Actors[0].InsideHouse = ""
		w.Actors[1].InsideHouse = ""
		w.InitLedgerAudit()
		return w
	}

	w0 := makeWorld()
	for _, view := range []Perception{w0.View(w0.Actors[1].ID), w0.viewIndexed(1, w0.buildStepIndex())} {
		for _, nb := range view.Nearby {
			if nb.ID == w0.Actors[0].ID {
				if nb.FormID == "nascent" || nb.FormID != "mortal" {
					t.Fatalf("expected disguised FormID 'mortal' (never leaking 'nascent'), got %q", nb.FormID)
				}
				if nb.PowerTier != 1 {
					t.Fatalf("expected disguised PowerTier 1, got %d", nb.PowerTier)
				}
			}
		}
	}

	// Run 15 independent copies and confirm identical multi-currency penalty text every time.
	baseline := ""
	for run := 0; run < 15; run++ {
		w := makeWorld()
		w.Tick = 5
		evs := w.stepGoldenFingers(nil)
		if len(evs) != 1 || evs[0].Kind != "gf_quest_failed" {
			t.Fatalf("expected 1 gf_quest_failed event, got %+v", evs)
		}
		txt, _ := evs[0].Payload["text"].(string)
		if baseline == "" {
			baseline = txt
		} else if txt != baseline {
			t.Fatalf("non-deterministic multi-currency penalty text: %q vs %q", baseline, txt)
		}
	}
}

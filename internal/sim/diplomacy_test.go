package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestDiplomacyCoalitionWarSkirmishPeaceAndReplay(t *testing.T) {
	w := Village("diplomacy-test")
	if len(w.Factions) < 2 {
		t.Fatalf("expected >=2 factions in Village fixture")
	}
	fA := w.Factions[0].ID
	fB := w.Factions[1].ID

	leaderA := &w.Actors[0]
	leaderB := &w.Actors[1]
	leaderA.Guilds = []string{fA}
	leaderA.Tags = appendUnique(leaderA.Tags, "ruler")
	leaderA.Energy = 90
	leaderA.Money = 60
	leaderB.Guilds = []string{fB}
	leaderB.Tags = appendUnique(leaderB.Tags, "ruler")
	leaderB.Energy = 90
	leaderB.Money = 60
	rs := w.ensureRelations()
	w.Factions[0].Members = appendUnique(w.Factions[0].Members, leaderA.ID)
	w.Factions[1].Members = appendUnique(w.Factions[1].Members, leaderB.ID)
	rs.FactionLeader[fA] = leaderA.ID
	rs.FactionLeader[fB] = leaderB.ID
	rs.ControllingFaction[w.Map.Districts[0].ID] = fB
	rs.ControllingFaction[w.Map.Districts[1].ID] = fA
	rs.ControllingFaction[w.Map.Districts[3].ID] = fB
	leaderA.Location, leaderB.Location = w.Map.Districts[0].ID, w.Map.Districts[0].ID
	leaderA.InsideHouse, leaderB.InsideHouse = "", ""
	w.InitLedgerAudit()

	// 1. External invasion automatically triggers emergency coalition alliance.
	w.Catalog.WorldEvents = []WorldEventSpec{
		{
			ID:                  "alien_inv",
			Name:                "External Vanguard",
			Category:            "external_invasion",
			DurationTicks:       200,
			Intensity:           80,
			InvasionThreatBoost: 30,
		},
	}
	invEvs := w.TriggerWorldEvent("alien_inv", w.Map.Districts[0].ID)
	dipEvs := w.stepDiplomacy(invEvs)
	foundAlliance := false
	for _, ev := range dipEvs {
		if ev.Kind == "alliance_formed" {
			foundAlliance = true
		}
	}
	if !foundAlliance {
		t.Fatalf("expected alliance_formed during external_invasion, got %+v", dipEvs)
	}

	// Verify View vs viewIndexed parity for Perception.FactionDiplomacy.
	direct := w.View(leaderA.ID)
	indexed := w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct.FactionDiplomacy, indexed.FactionDiplomacy) || len(direct.FactionDiplomacy) == 0 {
		t.Fatalf("View vs viewIndexed diplomacy parity mismatch: direct=%+v indexed=%+v", direct.FactionDiplomacy, indexed.FactionDiplomacy)
	}

	// 2. Expire treaty, turn hostile, declare war, run skirmish, then sign peace with reparations & district cession.
	rel := w.ensureFactionDiplomacy(fA, fB)
	rel.Stance = "hostile"
	rel.RelationScore = -65
	rel.TreatyUntilTick = 0
	warEvs := w.Apply(leaderA.ID, Action{Kind: "declare_war", FactionID: fA, TargetID: fB, Text: "border_clash"})
	if len(warEvs) == 0 || warEvs[0].Kind != "faction_war_declared" {
		t.Fatalf("expected faction_war_declared, got %+v", warEvs)
	}
	if !w.AreFactionsAtWar(fA, fB) {
		t.Fatalf("expected factions %s and %s to be at_war", fA, fB)
	}

	// Simulate war score advantage for fA so fB pays reparations and cedes a district.
	if rel.FactionA == fA {
		rel.WarScoreA = 30
		rel.WarScoreB = 4
	} else {
		rel.WarScoreB = 30
		rel.WarScoreA = 4
	}
	moneyABefore := leaderA.Money
	moneyBBefore := leaderB.Money
	proposal := w.Apply(leaderA.ID, Action{Kind: "make_peace", FactionID: fA, TargetID: fB})
	if len(proposal) != 1 || proposal[0].Kind != "peace_proposed" || !w.AreFactionsAtWar(fA, fB) {
		t.Fatalf("unilateral proposal changed the war: %+v", proposal)
	}
	peaceEvs := w.Apply(leaderB.ID, Action{Kind: "make_peace", FactionID: fB, TargetID: fA})
	if len(peaceEvs) == 0 || peaceEvs[0].Kind != "peace_treaty_signed" {
		t.Fatalf("expected peace_treaty_signed, got %+v", peaceEvs)
	}
	if w.AreFactionsAtWar(fA, fB) {
		t.Fatalf("expected war to end after make_peace")
	}
	if leaderA.Money <= moneyABefore || leaderB.Money >= moneyBBefore {
		t.Fatalf("expected war reparations from loser to victor: A %d->%d B %d->%d",
			moneyABefore, leaderA.Money, moneyBBefore, leaderB.Money)
	}
	if ceded, _ := peaceEvs[0].Payload["ceded_district"].(string); ceded == "" {
		t.Fatalf("expected contested district cession on decisive war score, got payload=%+v", peaceEvs[0].Payload)
	}

	// 3. Conservation + deterministic replay.
	if cons := InspectWorldConservation(w); !cons.LedgerConserved {
		t.Fatalf("conservation violated after diplomacy war/peace: %s", cons.LedgerError)
	}
	ckpt, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	waitDecider := lawDecider(func(_ context.Context, _ Perception) (Action, error) {
		return Action{Kind: "wait"}, nil
	})
	var logs []TickDecisionLog
	for i := 0; i < 12; i++ {
		_, log, err := w.StepRecorded(context.Background(), waitDecider)
		if err != nil {
			t.Fatal(err)
		}
		logs = append(logs, log)
	}
	replayed, _, err := ReplayFromCheckpoint(ckpt, logs, w.Tick)
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(w)
	b2, _ := json.Marshal(replayed)
	if !bytes.Equal(b1, b2) {
		t.Fatalf("diplomacy replay diverged from live state")
	}
}

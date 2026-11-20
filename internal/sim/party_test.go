package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestPartyFormationLootSplitBetrayalAndReplay(t *testing.T) {
	w := Village("party-test")
	d0 := w.Map.Districts[0].ID
	w.Catalog.Forms = map[string]FormSpec{
		"mortal": {ID: "mortal", PowerTier: 2},
	}
	w.Catalog.WorldEvents = []WorldEventSpec{
		{
			ID:                "relic_site",
			Name:              "Ancient Relic",
			Category:          "artifact_emergence",
			DurationTicks:     200,
			Intensity:         60,
			RewardItem:        "food",
			RewardQty:         2,
			RewardCoins:       30,
			RequiredPowerTier: 1,
		},
	}

	leader := &w.Actors[0]
	ally := &w.Actors[1]
	betrayer := &w.Actors[2]
	for _, a := range []*Actor{leader, ally, betrayer} {
		a.Location = d0
		a.InsideHouse = ""
		a.VenueID = ""
		a.FormID = "mortal"
		a.Health = 100
		a.Energy = 90
		a.Money = 40
	}
	leader.Trust[ally.ID] = 3
	ally.Trust[leader.ID] = 3
	leader.Trust[betrayer.ID] = 1
	betrayer.Trust[leader.ID] = 0
	w.InitLedgerAudit()

	// 1. Form party with co-located ally, then betrayer joins.
	formEvs := w.Apply(leader.ID, Action{Kind: "form_party", TargetID: ally.ID, Item: "relic_hunt", Location: d0})
	if len(formEvs) == 0 || formEvs[0].Kind != "party_formed" {
		t.Fatalf("expected party_formed, got %+v", formEvs)
	}
	partyID, _ := formEvs[0].Payload["party_id"].(string)
	joinEvs := w.Apply(betrayer.ID, Action{Kind: "join_party", TargetID: partyID})
	if len(joinEvs) == 0 || joinEvs[0].Kind != "party_joined" {
		t.Fatalf("expected party_joined, got %+v", joinEvs)
	}

	// Verify View vs viewIndexed parity for Perception.Parties.
	direct := w.View(leader.ID)
	indexed := w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct.Parties, indexed.Parties) || len(direct.Parties) != 1 {
		t.Fatalf("View vs viewIndexed party parity mismatch: direct=%+v indexed=%+v", direct.Parties, indexed.Parties)
	}
	if bonus := w.partyCombatSupportBonus(*leader); bonus <= 0 {
		t.Fatalf("expected positive party combat support bonus, got %d", bonus)
	}

	// 2. Trigger artifact emergence and claim as party leader -> coins split fairly with 0-error conservation.
	allyMoneyBefore := ally.Money
	betrayerMoneyBefore := betrayer.Money
	weEvs := w.TriggerWorldEvent("relic_site", d0)
	instID, _ := weEvs[0].Payload["instance_id"].(string)
	claimEvs := w.Apply(leader.ID, Action{Kind: "claim_world_event", TargetID: instID})
	foundSplit := false
	for _, ev := range claimEvs {
		if ev.Kind == "party_loot_split" {
			foundSplit = true
		}
	}
	if !foundSplit {
		t.Fatalf("expected party_loot_split during relic claim, got %+v", claimEvs)
	}
	if ally.Money <= allyMoneyBefore || betrayer.Money <= betrayerMoneyBefore {
		t.Fatalf("expected party mates to receive coin share: ally %d->%d betrayer %d->%d",
			allyMoneyBefore, ally.Money, betrayerMoneyBefore, betrayer.Money)
	}

	// 3. Betrayer ambushes leader -> exact coin seizure, trust collapse, grudge formed, eyewitness observation.
	leaderMoneyBefore := leader.Money
	betrayEvs := w.Apply(betrayer.ID, Action{Kind: "betray_party", TargetID: leader.ID})
	foundBetray, foundGrudge := false, false
	for _, ev := range betrayEvs {
		if ev.Kind == "party_betrayed" {
			foundBetray = true
		}
		if ev.Kind == "grudge_formed" {
			foundGrudge = true
		}
	}
	if !foundBetray || !foundGrudge {
		t.Fatalf("expected party_betrayed + grudge_formed, got %+v", betrayEvs)
	}
	if leader.Money >= leaderMoneyBefore {
		t.Fatalf("expected leader to lose seized coins on betrayal: before=%d after=%d", leaderMoneyBefore, leader.Money)
	}
	if leader.Trust[betrayer.ID] >= 0 {
		t.Fatalf("expected leader trust toward betrayer to collapse, got %d", leader.Trust[betrayer.ID])
	}

	// 4. Ledger conservation + deterministic replay.
	if cons := InspectWorldConservation(w); !cons.LedgerConserved {
		t.Fatalf("conservation violated after party lifecycle: %s", cons.LedgerError)
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
		t.Fatalf("party replay diverged from live state")
	}
}

package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestAmbitionsEpistemicSecretsBlackmailExposureAndReplay(t *testing.T) {
	w := Village("ambitions-test")
	d0 := w.Map.Districts[0].ID

	investigator := &w.Actors[0]
	suspect := &w.Actors[1]
	bystander := &w.Actors[2]
	for _, a := range []*Actor{investigator, suspect, bystander} {
		a.Location = d0
		a.InsideHouse = ""
		a.VenueID = ""
		a.Energy = 95
		a.Health = 100
		a.Money = 45
	}
	investigator.Attributes.Insight = 20
	suspect.Attributes.Dexterity = 4
	w.InitLedgerAudit()

	// 1. Adopt long-term ambition and verify completion at 09:00 evaluation.
	ambEvs := w.Apply(investigator.ID, Action{Kind: "set_ambition", Item: "accumulate_wealth", Quantity: 50})
	if len(ambEvs) == 0 || ambEvs[0].Kind != "ambition_adopted" {
		t.Fatalf("expected ambition_adopted, got %+v", ambEvs)
	}
	w.adjustActorCurrency(investigator, "coins", 10, true)
	w.Tick = 12 // 09:00 (tick 0 = 06:00, +12 quarter-hours = 09:00)
	stepEvs := w.stepAmbitionsAndSecrets(nil)
	foundDone := false
	for _, ev := range stepEvs {
		if ev.Kind == "ambition_completed" && ev.ActorID == investigator.ID {
			foundDone = true
		}
	}
	if !foundDone {
		t.Fatalf("expected ambition_completed at 09:00, got %+v", stepEvs)
	}

	// 2. Plant a covert secret on suspect (known ONLY to suspect).
	w.addSecret(suspect.ID, "covert_bribe", "third_party", 72, 1, []string{suspect.ID})
	secID := w.Secrets[len(w.Secrets)-1].ID

	// Strict epistemic isolation: outsider investigator cannot see, blackmail, or expose secret before discovering it!
	if len(w.View(investigator.ID).KnownSecrets) != 0 {
		t.Fatalf("expected investigator to know 0 secrets before investigation, got %+v", w.View(investigator.ID).KnownSecrets)
	}
	failBM := w.Apply(investigator.ID, Action{Kind: "blackmail", TargetID: suspect.ID, Item: secID})
	if len(failBM) == 0 || failBM[0].Kind != "action_failed" {
		t.Fatalf("expected blackmail without knowing secret to fail, got %+v", failBM)
	}
	failExp := w.Apply(investigator.ID, Action{Kind: "expose_secret", TargetID: suspect.ID, Item: secID})
	if len(failExp) == 0 || failExp[0].Kind != "action_failed" {
		t.Fatalf("expected expose_secret without knowing secret to fail, got %+v", failExp)
	}

	// 3. Investigate until deterministic roll succeeds (prob up to 85%).
	discovered := false
	for attempt := 0; attempt < 20; attempt++ {
		w.Tick++
		investigator.Energy = 90
		evs := w.Apply(investigator.ID, Action{Kind: "investigate_actor", TargetID: suspect.ID})
		if len(evs) > 0 && evs[0].Kind == "secret_discovered" {
			discovered = true
			break
		}
	}
	if !discovered {
		t.Fatalf("expected high-insight investigator to uncover suspect secret")
	}

	// Verify View vs viewIndexed parity for Perception.KnownSecrets.
	direct := w.View(investigator.ID)
	indexed := w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct.KnownSecrets, indexed.KnownSecrets) || len(direct.KnownSecrets) == 0 {
		t.Fatalf("View vs viewIndexed KnownSecrets parity mismatch: direct=%+v indexed=%+v", direct.KnownSecrets, indexed.KnownSecrets)
	}
	// Bystander still must NOT see unexposed secret.
	if len(w.View(bystander.ID).KnownSecrets) != 0 {
		t.Fatalf("bystander must not see unexposed secret, got %+v", w.View(bystander.ID).KnownSecrets)
	}

	// 4. Blackmail transfers exact coins with 0-error ledger conservation.
	invMoneyBefore := investigator.Money
	susMoneyBefore := suspect.Money
	bmEvs := w.Apply(investigator.ID, Action{Kind: "blackmail", TargetID: suspect.ID, Item: secID})
	if len(bmEvs) == 0 || bmEvs[0].Kind != "blackmailed" {
		t.Fatalf("expected blackmailed event, got %+v", bmEvs)
	}
	if investigator.Money <= invMoneyBefore || suspect.Money >= susMoneyBefore {
		t.Fatalf("expected exact blackmail coin transfer: inv %d->%d sus %d->%d",
			invMoneyBefore, investigator.Money, susMoneyBefore, suspect.Money)
	}
	if investigator.Money-invMoneyBefore != susMoneyBefore-suspect.Money {
		t.Fatalf("coin conservation error during blackmail")
	}

	// 5. Publicly expose secret -> bystander trust drops & secret becomes visible to all.
	bystanderTrustBefore := bystander.Trust[suspect.ID]
	expEvs := w.Apply(investigator.ID, Action{Kind: "expose_secret", TargetID: suspect.ID, Item: secID})
	if len(expEvs) == 0 || expEvs[0].Kind != "secret_exposed" {
		t.Fatalf("expected secret_exposed event, got %+v", expEvs)
	}
	if bystander.Trust[suspect.ID] >= bystanderTrustBefore {
		t.Fatalf("expected bystander trust toward suspect to drop after exposure: before=%d after=%d",
			bystanderTrustBefore, bystander.Trust[suspect.ID])
	}
	if len(w.View(bystander.ID).KnownSecrets) == 0 {
		t.Fatalf("expected exposed secret to become visible to bystander")
	}

	// 6. Conservation + deterministic replay.
	if cons := InspectWorldConservation(w); !cons.LedgerConserved {
		t.Fatalf("conservation violated after ambitions/secrets: %s", cons.LedgerError)
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
		t.Fatalf("ambitions/secrets replay diverged from live state")
	}
}

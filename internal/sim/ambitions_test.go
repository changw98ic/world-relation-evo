package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestNewSecretsKeepOldUnexposedSecrets(t *testing.T) {
	w := lawWorld()
	w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], "expose_secret")
	for i := 0; i < 201; i++ {
		w.addSecret("a", "offense", fmt.Sprintf("offense-%d", i), 45, 0, []string{"b"})
	}
	if len(w.Secrets) != 201 {
		t.Fatalf("adding a secret left %d records; expected 201", len(w.Secrets))
	}
	first := w.Secrets[0].ID
	for _, s := range w.View("b").KnownSecrets {
		if s.ID == first {
			lawOK(t, w.Apply("b", Action{Kind: "expose_secret", Item: first}))
			if !w.Secrets[0].Exposed {
				t.Fatal("the original secret was not exposed")
			}
			return
		}
	}
	t.Fatal("the witness forgot an active secret after the 201st insert")
}

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

	// 4. Subtle minor favor ("buy me a meal": 2 coins out of 45) -> low offense, NO blood grudge!
	suspect.Personality = "timid and cautious"
	invMoneyBefore := investigator.Money
	susMoneyBefore := suspect.Money
	grudgesBefore := len(w.ensureRelations().Grudges)
	minorEvs := w.Apply(investigator.ID, Action{Kind: "blackmail", TargetID: suspect.ID, Item: secID, Price: 2, Text: "subtle"})
	if len(minorEvs) == 0 || minorEvs[0].Kind != "blackmailed" {
		t.Fatalf("expected minor favor blackmail to settle peacefully, got %+v", minorEvs)
	}
	if tier, _ := minorEvs[0].Payload["severity_tier"].(string); tier != "minor_favor" {
		t.Fatalf("expected severity_tier=minor_favor for subtle 2-coin request, got %q (payload=%+v)", tier, minorEvs[0].Payload)
	}
	if len(w.ensureRelations().Grudges) != grudgesBefore {
		t.Fatalf("minor meal-sized favor must NOT create a blood feud grudge!")
	}
	if investigator.Money-invMoneyBefore != 2 || susMoneyBefore-suspect.Money != 2 {
		t.Fatalf("coin conservation error during subtle favor: inv %d->%d sus %d->%d",
			invMoneyBefore, investigator.Money, susMoneyBefore, suspect.Money)
	}

	// 4b. Repeatedly triggering the SAME minor 2-coin request causes offense to escalate EXPONENTIALLY
	// (e.g. 4 -> 8 -> 16 -> 32 -> 64...) until even a timid victim crosses their breaking threshold and lashes back!
	snapped := false
	lastOffense := int(payloadInt64(minorEvs[0].Payload, "single_offense"))
	for rep := 0; rep < 8; rep++ {
		investigator.Energy = 90
		evs := w.Apply(investigator.ID, Action{Kind: "blackmail", TargetID: suspect.ID, Item: secID, Price: 2, Text: "subtle"})
		if len(evs) == 0 {
			t.Fatalf("unexpected empty events on repeat %d", rep)
		}
		curOffense := int(payloadInt64(evs[0].Payload, "single_offense"))
		if curOffense <= lastOffense {
			t.Fatalf("expected exponential offense escalation on repeat %d: prev=%d cur=%d", rep, lastOffense, curOffense)
		}
		lastOffense = curOffense
		if evs[0].Kind == "blackmail_refused" {
			snapped = true
			break
		}
	}
	if !snapped {
		t.Fatalf("expected repeated identical demands to exponentially breach breaking threshold and trigger refusal/backlash")
	}
	investigator.Health = 100

	// 4c. Hostile greedy demand on a proud/high-tier target IMMEDIATELY breaches breaking threshold on attempt #1!
	w.Catalog.Forms = map[string]FormSpec{
		"low":  {ID: "low", PowerTier: 1},
		"high": {ID: "high", PowerTier: 4},
	}
	investigator.FormID = "low"
	suspect.FormID = "high"
	suspect.Personality = "proud and vengeful"
	w.addSecret(suspect.ID, "forbidden_hoard", "vault", 80, 99, []string{suspect.ID, investigator.ID})
	sec2ID := w.Secrets[len(w.Secrets)-1].ID
	refusedEvs := w.Apply(investigator.ID, Action{Kind: "blackmail", TargetID: suspect.ID, Item: sec2ID, Price: 30, Text: "hostile"})
	if len(refusedEvs) == 0 || refusedEvs[0].Kind != "blackmail_refused" {
		t.Fatalf("expected hostile greedy demand against proud high-tier target to trigger immediate blackmail_refused, got %+v", refusedEvs)
	}
	investigator.FormID = ""
	suspect.FormID = ""
	suspect.Personality = ""

	// 4d. Verify logarithmic (lg) time decay of offense memory:
	// 0 days -> 100%, 9 days -> 50% (1 + lg(10) = 2.0), 99 days -> 33% (1 + lg(100) = 3.0).
	baseRec := OffenseRecord{RepeatCount: 4, Accumulated: 100, LastTick: 100}
	acc0, rep0 := DecayedOffense(baseRec, 100)
	acc9, rep9 := DecayedOffense(baseRec, 100+9*TicksPerDay)
	acc99, rep99 := DecayedOffense(baseRec, 100+99*TicksPerDay)
	if acc0 != 100 || rep0 != 4 || acc9 != 50 || rep9 != 2 || acc99 != 33 || rep99 != 1 {
		t.Fatalf("unexpected lg offense decay: d0=(%d,%d) d9=(%d,%d) d99=(%d,%d)", acc0, rep0, acc9, rep9, acc99, rep99)
	}

	// 5. Publicly expose secret -> bystander trust drops & secret becomes known to those who hear the disclosure.
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

func TestRedHandedWitnessConfrontationAndSecretHoldStances(t *testing.T) {
	w := Village("red-handed-test")
	d0 := w.Map.Districts[0].ID
	w.Catalog.Forms = map[string]FormSpec{
		"weak":   {ID: "weak", PowerTier: 1},
		"strong": {ID: "strong", PowerTier: 3},
	}

	culprit := &w.Actors[0]
	corruptOfficial := &w.Actors[1]
	righteousWitness := &w.Actors[2]
	kinShielder := &w.Actors[3]
	timidWitness := &w.Actors[4]
	patientSchemer := &w.Actors[5]

	for _, a := range []*Actor{culprit, corruptOfficial, righteousWitness, kinShielder, timidWitness, patientSchemer} {
		a.Location = d0
		a.InsideHouse = ""
		a.VenueID = ""
		a.Health = 100
		a.Energy = 90
		a.Money = 40
	}
	culprit.FormID = "strong"
	corruptOfficial.FormID = "strong"
	righteousWitness.FormID = "strong"
	righteousWitness.Personality = "righteous and stern"
	kinShielder.FormID = "weak"
	kinShielder.Trust[culprit.ID] = 5
	timidWitness.FormID = "weak"
	timidWitness.Personality = "timid and cautious"
	patientSchemer.FormID = "weak"
	patientSchemer.Personality = "patient and calculating"
	w.InitLedgerAudit()

	// Simulate a covert bribe caught red-handed by all 4 co-located bystanders.
	bribeEv := Event{
		ID:       900,
		Tick:     10,
		ActorID:  culprit.ID,
		TargetID: corruptOfficial.ID,
		Kind:     "bribed",
	}
	stepEvs := w.captureActionSecrets(nil, bribeEv)
	if len(w.Secrets) == 0 {
		t.Fatalf("expected bribed event to record secrets")
	}
	var bribeSec *ActorSecret
	for i := range w.Secrets {
		if w.Secrets[i].OwnerID == culprit.ID && w.Secrets[i].Kind == "covert_bribe" {
			bribeSec = &w.Secrets[i]
			break
		}
	}
	if bribeSec == nil {
		t.Fatalf("expected covert_bribe secret on culprit")
	}

	// 1. All 4 bystanders caught culprit red-handed -> in BOTH KnownBy and WitnessedBy, AND culprit noticed them!
	for _, wid := range []string{righteousWitness.ID, kinShielder.ID, timidWitness.ID, patientSchemer.ID} {
		if !ActorHasTag(bribeSec.KnownBy, wid) || !ActorHasTag(bribeSec.WitnessedBy, wid) {
			t.Fatalf("expected bystander %s in KnownBy & WitnessedBy, got known=%v witnessed=%v", wid, bribeSec.KnownBy, bribeSec.WitnessedBy)
		}
	}
	foundRedHandedMem := false
	for _, m := range righteousWitness.Memories {
		if m.Kind == "secret_caught_red_handed" && m.Observation != nil && m.Observation.SubjectID == culprit.ID {
			foundRedHandedMem = true
		}
	}
	if !foundRedHandedMem {
		t.Fatalf("expected righteous bystander to hold secret_caught_red_handed observation")
	}
	foundOwnerNoticed := false
	for _, m := range culprit.Memories {
		if m.Kind == "witnessed_my_secret" && m.Observation != nil && m.Observation.SubjectID == timidWitness.ID {
			foundOwnerNoticed = true
		}
	}
	if !foundOwnerNoticed {
		t.Fatalf("expected culprit to hold bidirectional witnessed_my_secret observation")
	}

	// 2. Verify explicit non-owner holding stances!
	if got := bribeSec.HolderStances[righteousWitness.ID]; got != "confront" {
		t.Fatalf("expected righteous witness stance=confront, got %q", got)
	}
	if got := bribeSec.HolderStances[kinShielder.ID]; got != "shield" {
		t.Fatalf("expected high-trust ally stance=shield, got %q", got)
	}
	if got := bribeSec.HolderStances[timidWitness.ID]; got != "fear_silence" {
		t.Fatalf("expected timid weaker witness stance=fear_silence, got %q", got)
	}
	if got := bribeSec.HolderStances[patientSchemer.ID]; got != "bide_time" {
		t.Fatalf("expected patient schemer weaker witness stance=bide_time, got %q", got)
	}
	foundSecretHeld := false
	for _, ev := range stepEvs {
		if ev.Kind == "secret_held" {
			foundSecretHeld = true
		}
	}
	if !foundSecretHeld {
		t.Fatalf("expected secret_held events for explicit silent holding stances, got %+v", stepEvs)
	}

	// 3. Righteous witness exposes the severe secret -> kinShielder gets implicated in covert_complicity!
	expEvs := w.Apply(righteousWitness.ID, Action{Kind: "expose_secret", TargetID: culprit.ID, Item: bribeSec.ID})
	if len(expEvs) == 0 || expEvs[0].Kind != "secret_exposed" {
		t.Fatalf("expected righteous witness to expose secret, got %+v", expEvs)
	}
	foundComplicity := false
	for _, s := range w.Secrets {
		if s.OwnerID == kinShielder.ID && s.Kind == "covert_complicity" && s.SubjectID == culprit.ID {
			foundComplicity = true
		}
	}
	if !foundComplicity {
		t.Fatalf("expected shielding ally to acquire covert_complicity secret when severe secret is exposed")
	}
	if cons := InspectWorldConservation(w); !cons.LedgerConserved {
		t.Fatalf("conservation violated: %s", cons.LedgerError)
	}
}

package sim_test

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func hasOptionKind(opts []agent.Option, kind string) bool {
	for _, o := range opts {
		if o.Action.Kind == kind {
			return true
		}
	}
	return false
}

// 1. 存活人物降级后不应被错误判定贷款违约 (LOD Outcome Invariance on Bilateral Personal Loans)
func TestDehydratedLivingBorrowerDoesNotDefaultLoan(t *testing.T) {
	setupLoanWorld := func(id string) (*sim.World, string, string) {
		w := sim.Village(id)
		borrower := actorByID(w, "p01") // owns h01
		lender := actorByID(w, "p27")   // peddler with trader tag
		borrower.Location, borrower.InsideHouse, borrower.Task, borrower.JailedUntil = "market_street", "", nil, 0
		lender.Location, lender.InsideHouse, lender.Task, lender.JailedUntil = "market_street", "", nil, 0
		borrower.Money = 20
		lender.Money = 40

		reqEvs := w.Apply(borrower.ID, sim.Action{
			Kind:     "borrow_loan",
			TargetID: lender.ID,
			Quantity: 10,
			HouseID:  "h01",
		})
		if len(reqEvs) == 0 || reqEvs[0].Kind != "loan_requested" {
			t.Fatalf("expected loan_requested, got %+v", reqEvs)
		}
		reqID := w.MarketCredit.LoanRequests[0].ID
		accEvs := w.Apply(lender.ID, sim.Action{
			Kind:     "accept_loan",
			TargetID: borrower.ID,
			OfferID:  reqID,
		})
		if len(accEvs) == 0 || accEvs[0].Kind != "loan_issued" {
			t.Fatalf("expected loan_issued after bilateral accept_loan, got %+v", accEvs)
		}
		return w, borrower.ID, lender.ID
	}

	wFocal, borrowerID, lenderID := setupLoanWorld("loan-focal")
	wDehydrated, _, _ := setupLoanWorld("loan-dehydrated")

	initPop := wDehydrated.TotalPopulation()
	initCoins := wDehydrated.TotalCoins()

	// Dehydrate the living solvent borrower into a macro cohort.
	if err := wDehydrated.DehydrateActor(borrowerID); err != nil {
		t.Fatalf("DehydrateActor failed: %v", err)
	}
	if actorByID(wDehydrated, borrowerID) != nil {
		t.Fatalf("expected borrower %s to be dehydrated into cohort", borrowerID)
	}
	if wDehydrated.TotalPopulation() != initPop || wDehydrated.TotalCoins() != initCoins {
		t.Fatalf("dehydration broke conservation: pop=%d/%d coins=%d/%d",
			wDehydrated.TotalPopulation(), initPop, wDehydrated.TotalCoins(), initCoins)
	}

	// Advance both worlds across daily credit settlement ticks.
	for day := 1; day <= 5; day++ {
		wFocal.Tick = int64(day * sim.TicksPerDay)
		wDehydrated.Tick = int64(day * sim.TicksPerDay)
		wFocal.StepMarketAndCredit(wFocal.MarketCredit)
		wDehydrated.StepMarketAndCredit(wDehydrated.MarketCredit)
	}

	focalLoan := wFocal.MarketCredit.Loans[0]
	dehydLoan := wDehydrated.MarketCredit.Loans[0]
	if dehydLoan.Status == "defaulted" {
		t.Fatalf("living dehydrated borrower was wrongly marked defaulted: %+v", dehydLoan)
	}
	if dehydLoan.Status != "active" || dehydLoan.Status != focalLoan.Status {
		t.Fatalf("LOD non-invariance on loan status: dehydrated=%q focal=%q", dehydLoan.Status, focalLoan.Status)
	}
	if dehydLoan.AccruedInterest != focalLoan.AccruedInterest {
		t.Fatalf("LOD non-invariance on accrued interest: dehydrated=%d focal=%d", dehydLoan.AccruedInterest, focalLoan.AccruedInterest)
	}

	// Lender explicitly forecloses overdue collateral in both focal and dehydrated worlds -> identical outcome.
	fEvFocal := wFocal.Apply(lenderID, sim.Action{Kind: "foreclose_collateral", TargetID: borrowerID, HouseID: "h01"})
	fEvDehyd := wDehydrated.Apply(lenderID, sim.Action{Kind: "foreclose_collateral", TargetID: borrowerID, HouseID: "h01"})
	if len(fEvFocal) == 0 || fEvFocal[0].Kind != "house_foreclosed" || len(fEvDehyd) == 0 || fEvDehyd[0].Kind != "house_foreclosed" {
		t.Fatalf("LOD divergence on collateral foreclosure: focal=%+v dehydrated=%+v", fEvFocal, fEvDehyd)
	}
	wFocal.Tick += sim.TicksPerDay
	wDehydrated.Tick += sim.TicksPerDay
	wFocal.StepMarketAndCredit(wFocal.MarketCredit)
	wDehydrated.StepMarketAndCredit(wDehydrated.MarketCredit)
	if actorByID(wDehydrated, lenderID).Money != actorByID(wFocal, lenderID).Money {
		t.Fatalf("lender ending money diverged across LOD after tenancy rent: dehydrated=%d focal=%d",
			actorByID(wDehydrated, lenderID).Money, actorByID(wFocal, lenderID).Money)
	}
	if wDehydrated.TotalCoins() != initCoins {
		t.Fatalf("coin conservation drift after dehydrated foreclosure & rent: got=%d want=%d", wDehydrated.TotalCoins(), initCoins)
	}

	// Also verify dehydrating a living LENDER routes borrower's explicit repay_loan to the lender's cohort rather than losing/defaulting it.
	wLenderDehyd, bID2, lID2 := setupLoanWorld("loan-lender-dehydrated")
	initCoins2 := wLenderDehyd.TotalCoins()
	if err := wLenderDehyd.DehydrateActor(lID2); err != nil {
		t.Fatalf("DehydrateActor(lender) failed: %v", err)
	}
	repayEvs := wLenderDehyd.Apply(bID2, sim.Action{Kind: "repay_loan"})
	if len(repayEvs) == 0 || repayEvs[0].Kind != "loan_repaid" {
		t.Fatalf("expected loan_repaid to dehydrated lender, got %+v", repayEvs)
	}
	if wLenderDehyd.MarketCredit.Loans[0].Status != "repaid" {
		t.Fatalf("expected repaid status when lender dehydrated, got %+v", wLenderDehyd.MarketCredit.Loans[0])
	}
	if wLenderDehyd.TotalCoins() != initCoins2 || wLenderDehyd.PublicTreasury != 0 {
		t.Fatalf("repayment to dehydrated lender leaked to treasury or drifted M0: coins=%d/%d treasury=%d",
			wLenderDehyd.TotalCoins(), initCoins2, wLenderDehyd.PublicTreasury)
	}
}

// 2. 辟谣不应被当成恐慌传播 (Negation & Reassurance Polarity in Rumor Classification)
func TestRefutationAndReassuranceSpeechDoesNotTriggerPanic(t *testing.T) {
	w := sim.LargeWorld("rumor-refutation", 50_000)
	sr := w.SpotlightRumor
	if sr == nil {
		sr = sim.NewSpotlightRumorState()
		w.SpotlightRumor = sr
	}
	s1 := actorByID(w, "p27")
	s2 := actorByID(w, "p28")
	s1.Location, s1.InsideHouse, s1.Task, s1.JailedUntil = "market_street", "", nil, 0
	s2.Location, s2.InsideHouse, s2.Task, s2.JailedUntil = "market_street", "", nil, 0
	moraleBefore := w.DistrictStates["market_street"].MoraleBonus

	// Exact user reproduction: two speakers say "没有短缺，粮食充足，不要恐慌", even when discussing Topic="shortage".
	ev1 := w.Apply(s1.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s2.ID,
		Text:      "没有短缺，粮食充足，不要恐慌",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	ev2 := w.Apply(s2.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s1.ID,
		Text:      "没有短缺，粮食充足，不要恐慌",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	emitted := w.ObserveSpotlightAndRumors(sr, append(ev1, ev2...))
	for _, e := range emitted {
		if e.Kind == "rumor_panic_triggered" {
			t.Fatalf("refutation speech wrongly triggered rumor_panic_triggered: %+v", e)
		}
	}
	if boost := w.DistrictPanicDemandPermille(sr, "market_street", "food"); boost != 0 {
		t.Fatalf("refutation speech wrongly boosted panic demand by +%d‰ (expected 0)", boost)
	}
	if w.DistrictStates["market_street"].MoraleBonus < moraleBefore {
		t.Fatalf("refutation speech wrongly penalized morale: before=%d after=%d", moraleBefore, w.DistrictStates["market_street"].MoraleBonus)
	}

	// English refutation must also NOT trigger panic.
	ev3 := w.Apply(s1.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s2.ID,
		Text:      "There is no shortage here, grain is abundant, do not panic.",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	ev4 := w.Apply(s2.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s1.ID,
		Text:      "Supplies are plenty, don't panic!",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	if out := w.ObserveSpotlightAndRumors(sr, append(ev3, ev4...)); len(out) != 0 {
		t.Fatalf("English reassurance wrongly triggered panic: %+v", out)
	}

	// Conversely, negated abundance + affirmative shortage ("粮食不充足，已经断粮短缺！") MUST trigger panic.
	ev5 := w.Apply(s1.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s2.ID,
		Text:      "粮食不充足，集市已经出现短缺！",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	ev6 := w.Apply(s2.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s1.ID,
		Text:      "没错，存粮匮乏快断粮了！",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street"},
	})
	panicOut := w.ObserveSpotlightAndRumors(sr, append(ev5, ev6...))
	if lastOfKind(panicOut, "rumor_panic_triggered").ID == 0 {
		t.Fatalf("expected affirmative shortage rumor to trigger panic, got %+v", panicOut)
	}
	if boost := w.DistrictPanicDemandPermille(sr, "market_street", "food"); boost != 350 {
		t.Fatalf("expected +350‰ demand boost on real panic, got %d", boost)
	}
}

// 3. 人物不能直接知道隐藏的腐败事实 (Epistemic Gating of Corruption Perception & Audit)
func TestHiddenCorruptionCannotBePerceivedOrAuditedWithoutEvidence(t *testing.T) {
	w := sim.LargeWorld("epistemic-corruption", 20_000)
	constable := jrTestFindActorByRole(w, "constable")
	gangster := jrTestFindActorByRole(w, "gangster")
	witness := jrTestFindActorByRole(w, "peddler")
	censor := jrTestFindActorByRole(w, "scholar")
	if constable == nil || gangster == nil || witness == nil || censor == nil {
		t.Fatal("missing test actors")
	}

	// Constable secretly holds IllicitGain > 0 while standing right next to censor outdoors.
	for _, a := range []*sim.Actor{constable, censor} {
		a.Location, a.InsideHouse, a.Task, a.JailedUntil = "market_street", "", nil, 0
		a.Energy, a.Health = 90, 100
		a.Memories = nil
	}
	constable.IllicitGain = 30
	constable.Money = 50
	censor.Tags = append(censor.Tags, "censor")
	censor.Professions["xiucai"] = 1
	censor.IllicitGain = 0
	initCoins := w.TotalCoins()

	// Without witnessing or hearing about corruption, censor CANNOT perceive or audit constable!
	view := w.View(censor.ID)
	opts := agent.Options(view)
	if hasOptionKind(opts, "audit_corruption") {
		t.Fatalf("omniscient leak: audit_corruption offered without epistemic evidence: %+v", opts)
	}
	dec, err := agent.Offline().Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Kind == "audit_corruption" {
		t.Fatalf("offline rules omnisciently selected audit_corruption without evidence: %+v", dec)
	}
	failAudit := w.Apply(censor.ID, sim.Action{Kind: "audit_corruption", TargetID: constable.ID})
	if len(failAudit) == 0 || failAudit[0].Kind != "action_failed" {
		t.Fatalf("expected direct audit_corruption without evidence to fail, got %+v", failAudit)
	}

	// Now gangster bribes constable INSIDE house h24 witnessed only by witness (censor is outdoors!).
	constable.InsideHouse = "h24"
	gangster.Location, gangster.InsideHouse, gangster.Task, gangster.JailedUntil = "market_street", "h24", nil, 0
	gangster.Money, gangster.Energy = 30, 80
	witness.Location, witness.InsideHouse, witness.Task, witness.JailedUntil = "market_street", "h24", nil, 0
	initCoins = w.TotalCoins()

	bribeEvs := w.Apply(gangster.ID, sim.Action{Kind: "bribe", TargetID: constable.ID})
	if len(bribeEvs) == 0 || bribeEvs[0].Kind != "bribed" {
		t.Fatalf("expected bribed event, got %+v", bribeEvs)
	}
	bribeEventID := bribeEvs[0].ID

	// Constable steps back outside next to censor; censor STILL knows nothing!
	constable.InsideHouse = ""
	if hasOptionKind(agent.Options(w.View(censor.ID)), "audit_corruption") {
		t.Fatalf("indoor bribe leaked to outdoor censor without contact")
	}

	// Witness steps outside and informs censor via speak citing BasisEventID.
	witness.InsideHouse = ""
	witnessMemID := int64(0)
	for _, m := range witness.Memories {
		if m.EventID == bribeEventID {
			witnessMemID = m.EventID
		}
	}
	if witnessMemID == 0 {
		t.Fatalf("co-located indoor witness did not record witnessed bribe observation")
	}
	spEvs := w.Apply(witness.ID, sim.Action{
		Kind:         "speak",
		TargetID:     censor.ID,
		Text:         "I saw the constable take a hush bribe inside the hall!",
		BasisEventID: witnessMemID,
		Statement: &sim.Statement{
			Topic:     "corruption",
			SubjectID: constable.ID,
			ObjectID:  gangster.ID,
			Item:      "coins",
			Location:  "market_street",
			Quantity:  5,
		},
	})
	if len(spEvs) == 0 || spEvs[0].Kind != "speech" {
		t.Fatalf("witness failed to speak corruption report: %+v", spEvs)
	}

	// Now censor holds second-hand epistemic evidence -> Options offers audit_corruption & Apply succeeds!
	afterView := w.View(censor.ID)
	if !hasOptionKind(agent.Options(afterView), "audit_corruption") {
		t.Fatalf("expected audit_corruption option after witness informed censor")
	}
	purgeEvs := w.Apply(censor.ID, sim.Action{Kind: "audit_corruption", TargetID: constable.ID})
	purged := lastOfKind(purgeEvs, "corruption_purged")
	if purged.ID == 0 {
		t.Fatalf("expected corruption_purged after receiving evidence, got %+v", purgeEvs)
	}
	if purged.Payload["root_event_id"] != bribeEventID {
		t.Fatalf("expected corruption_purged root_event_id=%d, got %v", bribeEventID, purged.Payload["root_event_id"])
	}
	if w.TotalCoins() != initCoins {
		t.Fatalf("coin conservation mismatch after epistemic audit: got=%d want=%d", w.TotalCoins(), initCoins)
	}
}

// 4. 异地或不知情冤案不能被直接翻案 (Local & Epistemic Gating of Wrongful Conviction Appeals)
func TestRemoteOrUnknownWrongfulConvictionCannotBeOverturned(t *testing.T) {
	w := sim.LargeWorld("epistemic-appeal", 20_000)
	jr := w.JusticeRevolt
	if jr == nil {
		jr = sim.NewJusticeRevoltState()
		w.JusticeRevolt = jr
	}
	constable := jrTestFindActorByRole(w, "constable")
	farmer := jrTestFindActorByRole(w, "farmer")
	witness := jrTestFindActorByRole(w, "herbalist")
	remoteCensor := jrTestFindActorByRole(w, "scholar")
	localBlindCensor := actorByID(w, "p08") // another actor granted censor tag, indoors during trial
	if constable == nil || farmer == nil || witness == nil || remoteCensor == nil || localBlindCensor == nil {
		t.Fatal("missing test actors")
	}

	// Setup trial in south_fields witnessed by witness; localBlindCensor is indoors in h17; remoteCensor is in east_lane.
	for _, a := range []*sim.Actor{constable, farmer, witness} {
		a.Location, a.InsideHouse, a.Task, a.JailedUntil = "south_fields", "", nil, 0
		a.Energy, a.Health = 90, 100
		a.Memories = nil
	}
	constable.IllicitGain = 10
	constable.Money = 20

	localBlindCensor.Location, localBlindCensor.InsideHouse, localBlindCensor.Task, localBlindCensor.JailedUntil = "south_fields", "h17", nil, 0
	localBlindCensor.Energy, localBlindCensor.Health, localBlindCensor.IllicitGain = 90, 100, 0
	localBlindCensor.Tags = append(localBlindCensor.Tags, "censor")
	localBlindCensor.Memories = nil

	remoteCensor.Location, remoteCensor.InsideHouse, remoteCensor.Task, remoteCensor.JailedUntil = "east_lane", "", nil, 0
	remoteCensor.Energy, remoteCensor.Health, remoteCensor.IllicitGain = 90, 100, 0
	remoteCensor.Tags = append(remoteCensor.Tags, "censor")
	remoteCensor.Memories = nil

	initCoins := w.TotalCoins()

	// Corrupt constable frames innocent farmer in south_fields.
	frameEvs := w.Apply(constable.ID, sim.Action{Kind: "frame_suspect", TargetID: farmer.ID})
	frameEv := lastOfKind(frameEvs, "wrongful_conviction")
	if frameEv.ID == 0 {
		t.Fatalf("expected wrongful_conviction, got %+v", frameEvs)
	}

	// 4a. Remote censor in east_lane has NO perception leak, NO option, and global fallback appeal MUST fail.
	remoteView := w.View(remoteCensor.ID)
	if remoteView.HasOpenWrongfulCase || hasOptionKind(agent.Options(remoteView), "file_appeal") {
		t.Fatalf("remote censor omnisciently perceived wrongful conviction in south_fields: view=%+v", remoteView)
	}
	if evs := w.Apply(remoteCensor.ID, sim.Action{Kind: "file_appeal"}); len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("untargeted remote file_appeal should fail (no global fallback), got %+v", evs)
	}
	if evs := w.Apply(remoteCensor.ID, sim.Action{Kind: "file_appeal", TargetID: farmer.ID}); len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("targeted remote file_appeal without knowledge/presence should fail, got %+v", evs)
	}

	// 4b. Local censor who was indoors (did not witness) steps outside into south_fields: STILL does not know!
	localBlindCensor.InsideHouse = ""
	localBlindView := w.View(localBlindCensor.ID)
	if localBlindView.HasOpenWrongfulCase || hasOptionKind(agent.Options(localBlindView), "file_appeal") {
		t.Fatalf("uninformed local censor perceived wrongful case without witnessing or hearing about it")
	}
	if evs := w.Apply(localBlindCensor.ID, sim.Action{Kind: "file_appeal", TargetID: farmer.ID}); len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("uninformed local censor should not be able to overturn unknown case, got %+v", evs)
	}

	// 4c. Witness travels to east_lane and tells remoteCensor via speak citing BasisEventID.
	witness.Location = "east_lane"
	spEvs := w.Apply(witness.ID, sim.Action{
		Kind:         "speak",
		TargetID:     remoteCensor.ID,
		Text:         "Constable framed innocent farmer in south_fields!",
		BasisEventID: frameEv.ID,
		Statement: &sim.Statement{
			Topic:     "wrongful_conviction",
			SubjectID: constable.ID,
			ObjectID:  farmer.ID,
			CaseID:    1,
			Location:  "south_fields",
			Quantity:  1,
		},
	})
	if len(spEvs) == 0 || spEvs[0].Kind != "speech" {
		t.Fatalf("witness speech failed: %+v", spEvs)
	}

	// While still in east_lane, remoteCensor knows about the case, so cannot appeal from east_lane yet—must travel!
	if evs := w.Apply(remoteCensor.ID, sim.Action{Kind: "file_appeal", TargetID: farmer.ID}); len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("appeal from wrong district should require traveling to south_fields first, got %+v", evs)
	}
	dec, err := agent.Offline().Decide(context.Background(), w.View(remoteCensor.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dec.Kind != "move" {
		t.Fatalf("expected informed remote censor to move toward south_fields court, got %+v", dec)
	}

	// Once remoteCensor arrives in south_fields, View & Options link the known case and file_appeal overturns it!
	remoteCensor.Location = "south_fields"
	arrivedView := w.View(remoteCensor.ID)
	if !arrivedView.HasOpenWrongfulCase || arrivedView.AppealTargetID != farmer.ID {
		t.Fatalf("expected informed censor at south_fields to link AppealTargetID=%s, got %+v", farmer.ID, arrivedView)
	}
	if !hasOptionKind(agent.Options(arrivedView), "file_appeal") {
		t.Fatalf("expected file_appeal in Options once informed censor arrives at court district")
	}
	appealEvs := w.Apply(remoteCensor.ID, sim.Action{Kind: "file_appeal", TargetID: farmer.ID})
	overturned := lastOfKind(appealEvs, "conviction_overturned")
	if overturned.ID == 0 {
		t.Fatalf("expected conviction_overturned, got %+v", appealEvs)
	}
	if overturned.Payload["root_event_id"] != frameEv.ID {
		t.Fatalf("expected root_event_id=%d linking back to wrongful_conviction, got %v", frameEv.ID, overturned.Payload["root_event_id"])
	}
	if farmer.JailedUntil != 0 {
		t.Fatalf("expected farmer to be released after appeal, jailed_until=%d", farmer.JailedUntil)
	}
	if w.TotalCoins() != initCoins {
		t.Fatalf("M0 coin conservation mismatch after appeal: got=%d want=%d", w.TotalCoins(), initCoins)
	}
}

// 5. 宏观事件不得直接写入市集或焦点人物记忆 (Zero Direct System/Macro Memory Injection)
func TestMacroEventsDoNotInjectMemoriesWithoutActorContact(t *testing.T) {
	w := sim.LargeWorld("no-macro-memory-injection", 100_000)
	sr := w.SpotlightRumor
	if sr == nil {
		sr = sim.NewSpotlightRumorState()
		w.SpotlightRumor = sr
	}

	for i := range w.Actors {
		w.Actors[i].Memories = nil
	}

	macroBatch := []sim.Event{
		{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: "world",
			Kind:    "disaster_struck",
			Action:  sim.Action{Kind: "wait", Location: "south_fields"},
			Payload: map[string]any{"text": "Flood in south_fields", "district": "south_fields", "wounded": 20, "deaths": 3},
		},
		{
			ID:       w.NextID + 1,
			Tick:     w.Tick,
			ActorID:  "censorate",
			TargetID: "cohort_market_gangsters",
			Kind:     "cohort_corruption_purged",
			Action:   sim.Action{Kind: "audit_corruption", Location: "market_street"},
			Payload:  map[string]any{"text": "Purged gang graft", "corrupt_cohort": "cohort_market_gangsters", "confiscated_coins": 50},
		},
		{
			ID:      w.NextID + 2,
			Tick:    w.Tick,
			ActorID: "cohort_market_chiefs",
			Kind:    "merchant_hoarded",
			Action:  sim.Action{Kind: "wait", Location: "market_street", Item: "food", Quantity: 100},
			Payload: map[string]any{"text": "Merchants hoarded grain", "district": "market_street", "item": "food", "quantity": 100},
		},
		{
			ID:      w.NextID + 3,
			Tick:    w.Tick,
			ActorID: "world",
			Kind:    "revolt_erupted",
			Action:  sim.Action{Kind: "attack", Location: "south_fields"},
			Payload: map[string]any{"text": "Revolt in south_fields", "district": "south_fields", "rebels": 80},
		},
	}
	w.NextID += 4

	emitted := w.ObserveSpotlightAndRumors(sr, macroBatch)
	if len(emitted) != 0 {
		t.Fatalf("expected zero emitted gossip events from macro batch, got %+v", emitted)
	}
	for i := range w.Actors {
		if len(w.Actors[i].Memories) != 0 {
			t.Fatalf("actor %s (%s) received system-injected memory without interpersonal contact: %+v",
				w.Actors[i].ID, w.Actors[i].Location, w.Actors[i].Memories)
		}
	}
}

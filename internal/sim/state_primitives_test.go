package sim

import (
	"math"
	"reflect"
	"testing"
)

func TestCraftOutputFitsInventoryOrReturnsPrepaidMaterials(t *testing.T) {
	for _, full := range []bool{false, true} {
		w := lawWorld()
		actor := &w.Actors[0]
		actor.Items["board"] = math.MaxInt - 1
		if full {
			actor.Items["board"]++
		}
		actor.Items["wood"] = 3
		lawOK(t, w.Apply(actor.ID, Action{Kind: "craft", RecipeID: "board"}))
		w.Tick++
		w.Advance(actor.ID)
		w.Tick++
		events := w.Advance(actor.ID)
		if full {
			lawRejected(t, events)
			if actor.Items["wood"] != 3 {
				t.Fatal("failed craft did not refund the prepaid wood")
			}
		} else {
			lawOK(t, events)
			if len(events) == 0 || events[0].Kind != "crafted" || actor.Items["wood"] != 0 {
				t.Fatalf("craft at the integer limit did not complete: %+v", events)
			}
		}
		if actor.Items["board"] != math.MaxInt {
			t.Fatalf("craft left an incorrect stock: %d", actor.Items["board"])
		}
	}
}

func TestWorldTxPrimitivesConservationAndDeathHook(t *testing.T) {
	w := Village("p2-tx-primitives")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Money = 25
	b.Money = 5
	a.Items = map[string]int{"food": 3}
	b.Items = nil
	b.Trust = nil
	w.InitLedgerAudit()

	tx := w.BeginTx(a.ID, ActionIntent{Action: Action{Kind: "wait"}})
	if !tx.TransferCurrency(a, b, "coins", 10, "payment") {
		t.Fatalf("expected coin transfer to succeed")
	}
	if a.Money != 15 || b.Money != 15 {
		t.Fatalf("unexpected balances after transfer: a=%d b=%d", a.Money, b.Money)
	}
	if !tx.TransferItem(a, b, "food", 3, "delivery") {
		t.Fatalf("expected item transfer to succeed")
	}
	if _, stillPresent := a.Items["food"]; stillPresent {
		t.Fatalf("expected zero-quantity food key to be deleted from sender inventory")
	}
	if b.Items["food"] != 3 {
		t.Fatalf("expected recipient to hold 3 food, got %d", b.Items["food"])
	}
	if !tx.MintOrBurnCurrency(a, "coins", 7, "reward") {
		t.Fatalf("expected currency mint to succeed")
	}
	if !tx.ProduceOrConsumeItem(b, "food", -1, "consume") {
		t.Fatalf("expected item consume to succeed")
	}
	tx.MutateTrust(b, a.ID, 2)
	if b.Trust[a.ID] != 2 {
		t.Fatalf("expected nil-safe MutateTrust to set trust=2, got %d", b.Trust[a.ID])
	}
	tx.MoveActor(a, "square", "", "")
	tx.RecordMemory(a, Memory{Kind: "test_mem", Tick: w.Tick, EventID: 1, Source: a.ID, Text: "hello"})
	if len(a.Memories) == 0 || a.Memories[len(a.Memories)-1].search == nil {
		t.Fatalf("expected RecordMemory to populate memory search cache")
	}

	// MutateVitals reducing health to 0 must automatically mark dead and emit actor_died.
	deathEvs := tx.MutateVitals(b, a, Action{Kind: "attack", TargetID: b.ID}, -MaxHealth, 0, "combat")
	if !b.Dead || b.Health != 0 {
		t.Fatalf("expected MutateVitals to kill actor on 0 HP, dead=%v health=%d", b.Dead, b.Health)
	}
	if len(deathEvs) == 0 {
		t.Fatalf("expected actor_died event from MutateVitals")
	}
	res := tx.Commit()
	if !res.Succeeded || len(res.Mutations) < 7 {
		t.Fatalf("expected committed tx with >=7 mutations, got succeeded=%v mutations=%d", res.Succeeded, len(res.Mutations))
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed after WorldTx primitives: %v", err)
	}
}

func TestAtomicityFixesClaimTurfAndMediateFeudAndDeclarativeSelfKill(t *testing.T) {
	w := Village("p2-atomicity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 30
	a.Health = 50
	b.Health = 80
	a.Money = 0 // offender cannot afford blood money
	b.Money = 10
	a.Tags = append(a.Tags, "gang", "martial")
	b.Trust = nil // verify nil-safe Trust writes on spar/heal/etc.

	// 1. claim_turf when faction already controls district must NOT deduct energy.
	w.Factions = append(w.Factions, Faction{ID: "iron_gang", Name: "Iron Gang", District: "square", Members: []string{a.ID}})
	a.Guilds = append(a.Guilds, "iron_gang")
	rs := w.ensureRelations()
	rs.ControllingFaction["square"] = "iron_gang"
	beforeEnergy := a.Energy
	evs := w.Apply(a.ID, Action{Kind: "claim_turf", FactionID: "iron_gang", Location: "square"})
	if len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected claim_turf failure, got %+v", evs)
	}
	if a.Energy != beforeEnergy {
		t.Fatalf("claim_turf burned energy on failure: before=%d after=%d", beforeEnergy, a.Energy)
	}

	// 2. mediate_feud when blood money transfer fails must NOT leave OffenderAccepted/AggrievedAccepted mutated.
	rs.Grudges = append(rs.Grudges, GrudgeRecord{
		ID:                  "g1",
		AggrievedID:         b.ID,
		OffenderID:          a.ID,
		Intensity:           50,
		CreatedTick:         w.Tick,
		OffenderAccepted:    true,
		MediationProposerID: a.ID,
		MediationCoins:      10,
	})
	evs = w.Apply(b.ID, Action{Kind: "mediate_feud", TargetID: a.ID, Price: 10})
	if len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected mediate_feud to fail when offender lacks coins, got %+v", evs)
	}
	if rs.Grudges[0].AggrievedAccepted {
		t.Fatalf("mediate_feud left AggrievedAccepted=true after failed settlement")
	}

	// 3. finishDeclarativeAction with negative HealthGain dropping HP to 0 must mark actor Dead.
	w.Catalog.ActionSpecs = map[string]ActionSpec{
		"blood_sacrifice": {ID: "blood_sacrifice", Name: "Blood Sacrifice", HealthGain: -100, Ticks: 1},
	}
	a.Health = 20
	evs = w.Apply(a.ID, Action{Kind: "blood_sacrifice"})
	if !a.Dead || a.Health != 0 {
		t.Fatalf("expected negative HealthGain to kill actor cleanly, dead=%v health=%d events=%+v", a.Dead, a.Health, evs)
	}

	// 4. PostTickPhaseNames exposes deterministic runtime pipeline.
	if names := w.PostTickPhaseNames(); len(names) != 26 {
		t.Fatalf("expected 26 explicit post-tick phases, got %d: %v", len(names), names)
	}
}

func TestActionExecutorRecordsTradeMutationsAndConservesLedger(t *testing.T) {
	w := lawWorld()
	seller, buyer := &w.Actors[0], &w.Actors[1]
	seller.Items["wood"], buyer.Money = 3, 7
	seller.Trust, buyer.Trust, buyer.Items = nil, nil, nil
	w.InitLedgerAudit()
	lawOK(t, w.Apply(seller.ID, Action{Kind: "offer", TargetID: buyer.ID, Item: "wood", Quantity: 3, Price: 7}))

	result, err := (DefaultActionExecutor{}).Execute(w, buyer.ID, ActionIntent{
		Action: Action{Kind: "accept", OfferID: w.Offers[0].ID},
	})
	if err != nil || !result.Succeeded || len(result.Events) != 1 || result.Events[0].Kind != "trade_completed" {
		t.Fatalf("trade did not settle through the executor: %+v, %v", result, err)
	}
	recorded := map[string]int{}
	for _, m := range result.Mutations {
		recorded[m.Primitive]++
	}
	for primitive, want := range map[string]int{"transfer_item": 1, "transfer_currency": 1, "mutate_trust": 0, "record_memory": 2} {
		if got := recorded[primitive]; got != want {
			t.Errorf("%s recorded %d times, want %d: %+v", primitive, got, want, result.Mutations)
		}
	}
	if buyer.Money != 0 || seller.Money != 7 || buyer.Items["wood"] != 3 || seller.Items["wood"] != 0 ||
		seller.Trust[buyer.ID] != 0 || buyer.Trust[seller.ID] != 0 {
		t.Fatalf("unexpected trade balances or trust: seller=%+v buyer=%+v", seller, buyer)
	}
	for _, participant := range []*Actor{seller, buyer} {
		receipt := participant.Memories[len(participant.Memories)-1].Trade
		if receipt == nil || receipt.Price != 7 || receipt.Quantity != 3 || receipt.Item != "wood" {
			t.Fatalf("%s is missing the actual trade terms: %+v", participant.ID, receipt)
		}
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

func TestActionExecutorRecordsSubsystemMutations(t *testing.T) {
	cases := []struct {
		action, event string
		setup         func(*testing.T, *World) (string, Action)
		mutations     []string
	}{
		{"accept_loan", "loan_issued", func(t *testing.T, w *World) (string, Action) {
			w.Actors[0].Money = 40
			lawOK(t, w.Apply("b", Action{Kind: "borrow_loan", TargetID: "a"}))
			return "a", Action{TargetID: "b"}
		}, []string{"transfer_currency", "record_memory"}},
		{"marry", "actor_married", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[0].Money = 12
			return "a", Action{TargetID: "b", Price: 3}
		}, []string{"transfer_currency", "mutate_trust", "record_memory"}},
		{"swear_bond", "bond_sworn", func(_ *testing.T, w *World) (string, Action) {
			return "a", Action{TargetID: "b"}
		}, []string{"mutate_trust", "record_memory"}},
		{"betray_party", "party_betrayed", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[1].Money = 20
			w.Parties = []Party{{ID: "p", LeaderID: "b", MemberIDs: []string{"a", "b"}, ExpiresTick: 10}}
			return "a", Action{TargetID: "b"}
		}, []string{"transfer_currency", "mutate_health", "mutate_energy", "mutate_trust", "record_memory"}},
		{"blackmail", "blackmailed", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[1].Money = 45
			w.Actors[1].Personality = "timid and cautious"
			w.addSecret("b", "covert_bribe", "c", 72, 1, []string{"a", "b"})
			return "a", Action{TargetID: "b", Item: w.Secrets[0].ID, Price: 2, Text: "subtle"}
		}, []string{"transfer_currency", "mutate_energy", "mutate_trust"}},
		{"fulfill_agreement", "agreement_fulfilled", func(t *testing.T, w *World) (string, Action) {
			w.Actors[0].Items["wood"], w.Actors[1].Money = 3, 7
			lawOK(t, w.Apply("a", Action{Kind: "propose_agreement", TargetID: "b", Item: "wood"}))
			id := w.Continuity.Agreements[0].ID
			lawOK(t, w.Apply("b", Action{Kind: "accept_agreement", TargetID: "a", AgreementID: id}))
			return "a", Action{TargetID: "b", AgreementID: id}
		}, []string{"transfer_currency", "transfer_item", "record_memory"}},
		{"gf_exchange", "gf_exchanged", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[0].Money, w.Actors[0].Health, w.Actors[0].Energy = 8, 90, 80
			w.Actors[0].Items["wood"] = 2
			w.Catalog.GoldenFingers = []GoldenFingerSpec{{
				ID: "shop", OwnerActorID: "a", Shop: []GFShopItemSpec{{
					ID: "remedy", CostCoins: 2, CostItems: map[string]int{"wood": 2},
					Reward: GFRewardSpec{Items: map[string]int{"board": 1}, HealthDelta: 3, EnergyDelta: 4},
				}},
			}}
			return "a", Action{Text: "shop", Item: "remedy"}
		}, []string{"mint_burn_currency", "produce_consume_item", "mutate_health", "mutate_energy"}},
		{"frame_suspect", "wrongful_conviction", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[0].Tags = append(w.Actors[0].Tags, "law")
			w.Actors[0].IllicitGain = 1
			w.Actors[0].InsideHouse, w.Actors[1].InsideHouse = "court", "court"
			return "a", Action{TargetID: "b"}
		}, []string{"mutate_energy", "move_actor", "mutate_trust", "record_memory"}},
		{"file_appeal", "conviction_overturned", func(t *testing.T, w *World) (string, Action) {
			w.Actors[0].Tags = append(w.Actors[0].Tags, "law")
			w.Actors[0].IllicitGain, w.Actors[0].Money = 1, 2
			w.PublicTreasury = 12
			lawOK(t, w.Apply("a", Action{Kind: "frame_suspect", TargetID: "b"}))
			return "c", Action{TargetID: "b"}
		}, []string{"transfer_currency", "mutate_energy", "mutate_trust", "record_memory"}},
		{"make_remedy", "action_resolved", func(_ *testing.T, w *World) (string, Action) {
			w.Actors[0].Money, w.Actors[0].Health = 4, 90
			w.Actors[0].Items["wood"], w.Actors[0].Items["seed"] = 2, 1
			w.Catalog.ActionSpecs = map[string]ActionSpec{
				"make_remedy": {ID: "make_remedy", MoneyCost: 2, EnergyCost: 3, HealthGain: 4,
					ItemCosts: map[string]int{"wood": 2, "seed": 1}, ItemGains: map[string]int{"board": 1}},
			}
			return "a", Action{}
		}, []string{"mint_burn_currency", "produce_consume_item", "mutate_health", "mutate_energy", "record_memory"}},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			run := func() ExecutionResult {
				w := lawWorld()
				w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], tc.action)
				actorID, action := tc.setup(t, w)
				action.Kind = tc.action
				w.InitLedgerAudit()
				result, err := (DefaultActionExecutor{}).Execute(w, actorID, ActionIntent{Action: action})
				if err != nil || !result.Succeeded || len(result.Events) == 0 || result.Events[0].Kind != tc.event {
					t.Fatalf("action result = %+v, %v; want %s", result, err, tc.event)
				}
				primitives := map[string]int{}
				for _, mutation := range result.Mutations {
					primitives[mutation.Primitive]++
				}
				for _, want := range tc.mutations {
					if primitives[want] == 0 {
						t.Errorf("missing %s mutation in %+v", want, result.Mutations)
					}
				}
				if primitives["emit_event"] != len(result.Events) {
					t.Errorf("recorded %d of %d events", primitives["emit_event"], len(result.Events))
				}
				if err := w.VerifyLedgerConservation(); err != nil {
					t.Fatal(err)
				}
				return result
			}
			first, second := run(), run()
			if !reflect.DeepEqual(first.Mutations, second.Mutations) {
				t.Fatalf("mutation order changed: %+v vs %+v", first.Mutations, second.Mutations)
			}
		})
	}
}

func TestExportedSubsystemActionsMatchWorldApply(t *testing.T) {
	cases := []struct {
		action    Action
		wantEvent string
		setup     func(*World)
		direct    func(*testing.T, *World, Action, bool) ([]Event, bool)
	}{
		{Action{Kind: "borrow_loan", TargetID: "b"}, "loan_requested",
			func(w *World) {
				w.MarketCredit = NewMarketCreditState()
				w.Actors[1].Money = 40
			},
			func(t *testing.T, w *World, x Action, external bool) ([]Event, bool) {
				state := w.MarketCredit
				if external {
					w.MarketCredit = NewMarketCreditState()
				}
				events, handled := w.ApplyMarketCreditAction(state, "a", x)
				if external && len(events) > 0 && events[0].Kind == "loan_requested" &&
					(len(state.LoanRequests) != 1 || len(w.MarketCredit.LoanRequests) != 0) {
					t.Fatal("loan request was not written to the explicit state")
				}
				return events, handled
			}},
		{Action{Kind: "swear_bond", TargetID: "b"}, "bond_sworn",
			func(w *World) { w.Relations = NewRelationEvoState() },
			func(t *testing.T, w *World, x Action, external bool) ([]Event, bool) {
				state := w.Relations
				if external {
					w.Relations = NewRelationEvoState()
				}
				events, handled := w.ApplyRelationAction(state, "a", x)
				if external && len(events) > 0 && events[0].Kind == "bond_sworn" &&
					(state.Kinship["a"] == nil || len(state.Kinship["a"].SwornIDs) != 1 || len(w.Relations.Kinship) != 0) {
					t.Fatal("bond was not written to the explicit state")
				}
				return events, handled
			}},
		{Action{Kind: "frame_suspect", TargetID: "b"}, "wrongful_conviction",
			func(w *World) {
				w.JusticeRevolt = NewJusticeRevoltState()
				w.Actors[0].Tags = append(w.Actors[0].Tags, "law")
				w.Actors[0].IllicitGain = 1
			},
			func(t *testing.T, w *World, x Action, external bool) ([]Event, bool) {
				state := w.JusticeRevolt
				if external {
					w.JusticeRevolt = NewJusticeRevoltState()
				}
				events, handled := w.ApplyJusticeAction(state, "a", x)
				if external && len(events) > 0 && events[0].Kind == "wrongful_conviction" &&
					(len(state.CourtCases) != 1 || len(w.JusticeRevolt.CourtCases) != 0) {
					t.Fatal("case was not written to the explicit state")
				}
				return events, handled
			}},
		{Action{Kind: "propose_agreement", TargetID: "b", Item: "wood"}, "agreement_proposed",
			func(w *World) { w.Actors[0].Items["wood"] = 3 },
			func(t *testing.T, w *World, x Action, external bool) ([]Event, bool) {
				a := w.actor("a")
				if external {
					copy := *a
					a = &copy
				}
				events := w.ApplyContinuity(a, x)
				if external && len(events) > 0 && events[0].Kind == "agreement_proposed" &&
					(len(a.Memories) != 1 || len(w.actor("a").Memories) != 0) {
					t.Fatal("proposal memory was not written to the explicit actor")
				}
				return events, true
			}},
	}
	guards := []struct {
		name  string
		block func(*World, *Action)
	}{
		{"allowed", func(*World, *Action) {}},
		{"capability", func(w *World, _ *Action) { w.Catalog = Catalog{} }},
		{"world_law", func(w *World, x *Action) {
			w.WorldLaws = map[string]int{"firearms_disabled": 1}
			w.Catalog.ActionSpecs = map[string]ActionSpec{x.Kind: {Attack: &AttackSpec{}, GrantedByTags: []string{"firearm"}}}
		}},
		{"citation", func(_ *World, x *Action) { x.BasisEventID = 100 }},
		{"stored_cohort", func(w *World, _ *Action) {
			w.Cohorts = []PopulationCohort{{ID: "stored", Population: 1, Members: []Actor{w.Actors[0]}}}
			w.Actors = w.Actors[1:]
		}},
		{"dead", func(w *World, _ *Action) { w.Actors[0].Dead = true }},
		{"custody", func(w *World, _ *Action) { w.Actors[0].JailedUntil = 1 }},
		{"busy", func(w *World, _ *Action) { w.Actors[0].Task = &Task{Action: Action{Kind: "craft"}} }},
	}
	for _, tc := range cases {
		for _, guard := range guards {
			for _, external := range []bool{false, true} {
				stateName := "world"
				if external {
					stateName = "explicit"
				}
				t.Run(tc.action.Kind+"/"+stateName+"/"+guard.name, func(t *testing.T) {
					makeWorld := func() (*World, Action) {
						w, x := lawWorld(), tc.action
						w.Catalog.TagActions["person"] = append(w.Catalog.TagActions["person"], x.Kind)
						tc.setup(w)
						guard.block(w, &x)
						return w, x
					}
					w, x := makeWorld()
					want := w.Apply("a", x)
					explicit, x := makeWorld()
					got, handled := tc.direct(t, explicit, x, external)
					if !handled || !reflect.DeepEqual(want, got) || !reflect.DeepEqual(w.Secrets, explicit.Secrets) {
						t.Fatalf("direct = %+v, %v, secrets %+v; Apply = %+v, secrets %+v", got, handled, explicit.Secrets, want, w.Secrets)
					}
					wantKind := "action_failed"
					if guard.name == "allowed" {
						wantKind = tc.wantEvent
					}
					if len(got) == 0 || got[0].Kind != wantKind {
						t.Fatalf("first event = %+v; want %s", got, wantKind)
					}
					if wantKind == "wrongful_conviction" && (len(explicit.Secrets) != 1 || explicit.Secrets[0].RootEventID != got[0].ID) {
						t.Fatalf("wrongful conviction secret missing: %+v", explicit.Secrets)
					}
				})
			}
		}
	}
}

func TestWorldTxRollbackAtomicityAcrossAllPrimitives(t *testing.T) {
	w := Village("tx-rollback-atomicity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Money = 30
	b.Money = 12
	a.Currencies = map[string]int64{"spirit_stone": 5}
	b.Currencies = nil
	a.Items = map[string]int{"food": 4}
	b.Items = nil
	a.Trust = nil
	b.Trust = nil
	a.Energy = 60
	a.Health = 90
	b.Health = 80
	a.Location = "square"
	b.Location = "square"
	w.Offers = []Offer{{ID: 1, Seller: b.ID, Buyer: a.ID, Item: "wood", Quantity: 1, Price: 2}}
	w.InitLedgerAudit()

	beforeA := CloneActor(*a)
	beforeB := CloneActor(*b)
	beforeOffers := append([]Offer(nil), w.Offers...)
	beforeNextID := w.NextID
	beforeAuditCoins := w.LedgerAudit.Currencies["coins"]
	beforeAuditFood := w.LedgerAudit.Items["food"]
	beforeDeaths := len(w.DeathsByCause)

	customDomainCounter := 10

	tx := w.BeginTx(a.ID, ActionIntent{Action: Action{Kind: "attack", TargetID: b.ID}})
	if !tx.TransferCurrency(a, b, "coins", 10, "step1") {
		t.Fatal("coin transfer failed")
	}
	if !tx.TransferCurrency(a, b, "spirit_stone", 5, "step2") {
		t.Fatal("custom currency transfer failed")
	}
	if !tx.MintOrBurnCurrency(a, "coins", 7, "step3") {
		t.Fatal("mint failed")
	}
	if !tx.TransferItem(a, b, "food", 4, "step4") {
		t.Fatal("item transfer failed")
	}
	if !tx.ProduceOrConsumeItem(b, "food", -2, "step5") {
		t.Fatal("item consume failed")
	}
	tx.MoveActor(a, "river", "", "")
	tx.MutateTrust(b, a.ID, -5)
	tx.RecordMemory(a, Memory{Kind: "temp_mem", Tick: w.Tick, EventID: 99, Source: a.ID, Text: "transient"})
	customDomainCounter = 99
	tx.OnRollback(func() { customDomainCounter = 10 })

	// Even a lethal MutateVitals mid-transaction must revert completely on Rollback.
	deathEvs := tx.MutateVitals(b, a, Action{Kind: "attack", TargetID: b.ID}, -MaxHealth, -15, "combat")
	if !b.Dead || len(deathEvs) == 0 {
		t.Fatalf("expected lethal MutateVitals before rollback, dead=%v evs=%d", b.Dead, len(deathEvs))
	}

	// Simulate mid-flight failure: rollback entire transaction.
	if !tx.Rollback() {
		t.Fatal("expected Rollback to return true")
	}
	if !tx.IsRolledBack() || tx.Rollback() {
		t.Fatal("expected Rollback to be idempotent and report IsRolledBack=true")
	}
	if tx.TransferCurrency(a, b, "coins", 1, "post-rollback") {
		t.Fatal("mutations after Rollback must be rejected")
	}
	if res := tx.Commit(); res.Succeeded {
		t.Fatalf("Commit after Rollback must not succeed: %+v", res)
	}

	if customDomainCounter != 10 {
		t.Fatalf("custom OnRollback hook did not restore state: got %d, want 10", customDomainCounter)
	}
	if !reflect.DeepEqual(*a, beforeA) {
		t.Fatalf("actor A not restored after Rollback:\ngot:  %+v\nwant: %+v", *a, beforeA)
	}
	if !reflect.DeepEqual(*b, beforeB) {
		t.Fatalf("actor B not restored after Rollback:\ngot:  %+v\nwant: %+v", *b, beforeB)
	}
	if !reflect.DeepEqual(w.Offers, beforeOffers) {
		t.Fatalf("offers not restored: got %+v, want %+v", w.Offers, beforeOffers)
	}
	if w.NextID != beforeNextID {
		t.Fatalf("NextID not restored: got %d, want %d", w.NextID, beforeNextID)
	}
	if len(w.DeathsByCause) != beforeDeaths {
		t.Fatalf("DeathsByCause not restored: %+v", w.DeathsByCause)
	}
	if w.LedgerAudit.Currencies["coins"] != beforeAuditCoins {
		t.Fatalf("LedgerAudit coins not restored: got %+v, want %+v", w.LedgerAudit.Currencies["coins"], beforeAuditCoins)
	}
	if w.LedgerAudit.Items["food"] != beforeAuditFood {
		t.Fatalf("LedgerAudit food not restored: got %+v, want %+v", w.LedgerAudit.Items["food"], beforeAuditFood)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation violated after Rollback: %v", err)
	}
}

func TestWorldTxAutoRollbackOnActionFailed(t *testing.T) {
	w := Village("tx-action-failed-rollback")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Money = 20
	b.Money = 5
	a.Items = map[string]int{"wood": 3}
	b.Items = map[string]int{}
	a.Energy = 50
	a.Health = 80
	a.Location = "square"
	a.Trust = map[string]int{}
	w.InitLedgerAudit()

	beforeMoneyA, beforeMoneyB := a.Money, b.Money
	beforeWoodA, beforeWoodB := a.Items["wood"], b.Items["wood"]
	beforeEnergy, beforeHealth := a.Energy, a.Health
	beforeLoc := a.Location
	beforeMemCount := len(a.Memories)
	domainFlag := false

	tx := w.BeginTx(a.ID, ActionIntent{Action: Action{Kind: "offer", TargetID: b.ID}})
	tx.TransferCurrency(a, b, "coins", 10, "partial")
	tx.TransferItem(a, b, "wood", 2, "partial")
	tx.setEnergy(a, 40, "partial")
	tx.adjustHealth(a, -10, 0, "partial")
	tx.MoveActor(a, "river", "", "")
	tx.MutateTrust(a, b.ID, 5)
	domainFlag = true
	tx.OnRollback(func() { domainFlag = false })

	// Handler aborts mid-way with action_failed.
	res := tx.commitEvents(actionFail(tx, a, Action{Kind: "offer", TargetID: b.ID}, "mid-way failure"))
	if res.Succeeded {
		t.Fatalf("expected action_failed to report Succeeded=false: %+v", res)
	}
	if len(res.Events) != 1 || res.Events[0].Kind != "action_failed" {
		t.Fatalf("expected single action_failed event, got %+v", res.Events)
	}
	if a.Money != beforeMoneyA || b.Money != beforeMoneyB {
		t.Fatalf("coins leaked after action_failed: a=%d b=%d", a.Money, b.Money)
	}
	if a.Items["wood"] != beforeWoodA || b.Items["wood"] != beforeWoodB {
		t.Fatalf("items leaked after action_failed: a=%d b=%d", a.Items["wood"], b.Items["wood"])
	}
	if a.Energy != beforeEnergy || a.Health != beforeHealth || a.Location != beforeLoc {
		t.Fatalf("vitals/location leaked after action_failed: energy=%d health=%d loc=%s", a.Energy, a.Health, a.Location)
	}
	if a.Trust[b.ID] != 0 {
		t.Fatalf("trust leaked after action_failed: %d", a.Trust[b.ID])
	}
	if domainFlag {
		t.Fatal("OnRollback hook did not run on action_failed")
	}
	if len(a.Memories) != beforeMemCount+1 || a.Memories[len(a.Memories)-1].Kind != "action_failed" {
		t.Fatalf("expected only action_failed memory to be recorded, got %+v", a.Memories)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation violated after action_failed rollback: %v", err)
	}
}

func TestCompoundActionValidatesCombinedItemCost(t *testing.T) {
	w := lawWorld()
	a := &w.Actors[0]
	a.Items["wood"] = 1
	w.Catalog.Transitions = map[string]TransitionSpec{"shift": {
		ID: "shift", ItemCosts: map[string]int{"wood": 1}, BaseSuccessPermille: 1000,
	}}
	w.Catalog.ActionSpecs = map[string]ActionSpec{"compound": {
		ID: "compound", GrantedByTags: []string{"person"},
		ItemCosts: map[string]int{"wood": 1}, ItemGains: map[string]int{"food": 1}, TransitionID: "shift",
	}}
	action := Action{Kind: "compound"}
	if w.CheckAction(a.ID, action).Allowed {
		t.Fatal("the same item was counted twice toward the costs")
	}
	beforeFood := a.Items["food"]
	lawRejected(t, w.Apply(a.ID, action))
	if a.Items["wood"] != 1 || a.Items["food"] != beforeFood {
		t.Fatalf("rejected action changed inventory: %v", a.Items)
	}

	a.Items["wood"]++
	if !w.CheckAction(a.ID, action).Allowed {
		t.Fatal("exact combined cost should qualify")
	}
	lawOK(t, w.Apply(a.ID, action))
	if a.Items["wood"] != 0 || a.Items["food"] != beforeFood+1 {
		t.Fatalf("valid compound action failed to settle: %v", a.Items)
	}
}

func TestFailedTransactionPublishesOnlyFailure(t *testing.T) {
	w := lawWorld()
	a, b := &w.Actors[0], &w.Actors[1]
	firstID := w.NextID
	x := Action{Kind: "speak", TargetID: b.ID, Text: "draft"}
	tx := w.BeginTx(a.ID, ActionIntent{Action: x})
	draft := tx.record(a, x, "speech", map[string]any{"text": x.Text}, b)
	evs := append([]Event{draft}, actionFail(tx, a, x, "rejected")...)
	result := tx.commitEvents(evs)
	if result.Succeeded || len(result.Events) != 1 || result.Events[0].ID != firstID ||
		len(a.Memories) != 1 || a.Memories[0].Kind != "action_failed" || len(b.Memories) != 0 {
		t.Fatalf("uncommitted event escaped: result=%+v memories=%+v", result, a.Memories)
	}
	if w.Continuity != nil && len(w.Continuity.DialogueByActor) != 0 {
		t.Fatal("uncommitted speech remained in the dialogue")
	}
	events := w.Apply(a.ID, x)
	if len(events) != 1 || events[0].ID != firstID+1 {
		t.Fatalf("the failure reused an event ID: %+v", events)
	}
}

func TestTransferCurrencyChecksReceivingBalance(t *testing.T) {
	w := lawWorld()
	a, b := &w.Actors[0], &w.Actors[1]
	maximum := int(^uint(0) >> 1)
	a.Money, b.Money = 2, maximum-1
	tx := w.BeginTx(a.ID, ActionIntent{})
	if !tx.TransferCurrency(a, b, "coins", 1, "test") || b.Money != maximum || a.Money != 1 {
		t.Fatal("transfer to the maximum representable balance failed")
	}
	if tx.TransferCurrency(a, b, "coins", 1, "test") || b.Money != maximum || a.Money != 1 {
		t.Fatal("overflowing transfer changed a balance")
	}
}

func TestTreasuryKeepsPendingTaskRefundAvailable(t *testing.T) {
	w := lawWorld()
	a, b := &w.Actors[0], &w.Actors[1]
	a.Money = 7
	w.Cohorts = []PopulationCohort{{ID: "public", District: a.Location}}
	w.PublicTreasury = 2
	w.Catalog.ActionSpecs = map[string]ActionSpec{"prepare": {
		ID: "prepare", GrantedByTags: []string{"person"}, TargetKind: "actor", Ticks: 2, MoneyCost: 7,
	}}
	w.InitLedgerAudit()
	x := Action{Kind: "prepare", TargetID: b.ID}
	lawOK(t, w.Apply(a.ID, x))
	if paid := w.debitTreasury(9, "bounty"); paid != 2 {
		t.Fatalf("spendable balance was %d, expected 2", paid)
	} else {
		b.Money += int(paid)
	}
	b.Location = "away"
	lawRejected(t, w.Advance(a.ID))
	if a.Money != 7 || w.PublicTreasury != 0 {
		t.Fatalf("prepayment refund failed: actor=%d treasury=%d", a.Money, w.PublicTreasury)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

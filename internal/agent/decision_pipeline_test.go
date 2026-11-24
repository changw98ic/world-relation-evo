package agent

import (
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestRulebookBondCandidateRequiresAPresentUnrelatedPartner(t *testing.T) {
	w := boundaryWorld()
	w.Catalog.TagActions["resident"] = append(w.Catalog.TagActions["resident"], "swear_bond")
	rules, err := (DefaultRuleCompiler{}).Compile(&InteractionRulebook{
		ReactionRules: []CausalReactionRule{{
			ID: "bond_on_speech", TriggerEventKinds: []string{"speech"}, ResponderPosition: "actor2",
			Branches: []ReactionBranch{{Behavior: BehaviorSpec{EngineKind: "swear_bond", TargetBinding: "$actor1"}}},
		}},
	}, w.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	w.Actors[1].Memories = []sim.Memory{{Kind: "speech", EventID: 7, Tick: w.Tick, Source: "alice", TargetID: "bob"}}
	offered := func() bool {
		t.Helper()
		candidates, err := (DefaultCandidateBuilder{}).Build(w.View("bob"), rules)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range candidates {
			if c.Action.Kind == "swear_bond" && c.Action.TargetID == "alice" && c.Action.BasisEventID == 7 {
				return true
			}
		}
		return false
	}
	if !offered() {
		t.Fatal("a present eligible partner was absent from the rulebook choices")
	}
	w.Actors[0].Location = "remote"
	if offered() {
		t.Fatal("a recent speech allowed bonding with someone in another district")
	}
	w.Actors[0].Location = "meeting"
	x := sim.Action{Kind: "swear_bond", TargetID: "alice"}
	if events := w.Apply("bob", x); len(events) == 0 || events[0].Kind != "bond_sworn" {
		t.Fatalf("valid bond failed: %+v", events)
	}
	if offered() || w.CheckAction("bob", x).Allowed {
		t.Fatal("an existing sworn bond remained available")
	}
}

func TestDeclarativeActionCompilerAgreesWithSettlement(t *testing.T) {
	w := sim.Village("g2-declarative-parity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 60
	a.Health = sim.MaxHealth
	b.Health = sim.MaxHealth
	a.Tags = append(a.Tags, "cultivator")
	a.StateSlots = map[string]int{"qi": 40}
	w.Catalog.StateSlots = map[string]sim.StateSlotSpec{
		"qi": {ID: "qi", Name: "Qi", Min: 0, Max: 100},
	}

	// 1. Register a brand-new composite declarative interaction WITHOUT touching World.Apply().
	w.Catalog.ActionSpecs = map[string]sim.ActionSpec{
		"spirit_palm": {
			ID:            "spirit_palm",
			Name:          "Spirit Palm",
			GrantedByTags: []string{"cultivator"},
			TargetKind:    "actor",
			EnergyCost:    5,
			SlotCosts:     map[string]int{"qi": 10},
			SlotGains:     map[string]int{"qi": 2},
			Attack: &sim.AttackSpec{
				DamageByType: map[string]int{"spirit": 18},
			},
			Ticks: 1,
		},
	}

	// 2. RuleCompiler MUST reject unknown / uncompiled new_primitive:* at compile time.
	badRB := &InteractionRulebook{
		ProactiveRules: []InteractionRule{
			{ID: "bad_1", Behavior: BehaviorSpec{EngineKind: "new_primitive:teleport_moon"}},
		},
	}
	if _, err := (DefaultRuleCompiler{}).Compile(badRB, w.Catalog); err == nil {
		t.Fatalf("expected DefaultRuleCompiler to reject unknown new_primitive:* at compile time")
	}

	// 3. RuleCompiler accepts a rule targeting our newly configured ActionSpec.
	goodRB := &InteractionRulebook{
		ProactiveRules: []InteractionRule{
			{
				ID:       "cultivator_duel",
				Name:     "Spirit Palm Exchange",
				Category: "conflict",
				Actor1:   RoleMatcher{AnyTags: []string{"cultivator"}},
				Actor2:   RoleMatcher{},
				Behavior: BehaviorSpec{EngineKind: "spirit_palm", TargetBinding: "$actor2"},
				Weight:   40,
			},
		},
	}
	ruleSet, err := (DefaultRuleCompiler{}).Compile(goodRB, w.Catalog)
	if err != nil {
		t.Fatalf("expected valid ActionSpec rule to compile cleanly, got: %v", err)
	}

	// 4. CandidateBuilder produces unified legal candidates with structured rule match metadata.
	p := w.View(a.ID)
	cands1, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("CandidateBuilder.Build failed: %v", err)
	}
	cands2, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("second CandidateBuilder.Build failed: %v", err)
	}
	if !reflect.DeepEqual(cands1, cands2) {
		t.Fatalf("CandidateBuilder must be deterministic for identical perception & rules")
	}

	var matchedCand *sim.Candidate
	for i := range cands1 {
		if cands1[i].Action.Kind == "spirit_palm" && cands1[i].Action.TargetID == b.ID {
			matchedCand = &cands1[i]
			break
		}
	}
	if matchedCand == nil {
		t.Fatalf("expected spirit_palm -> %s candidate in unified CandidateBuilder output", b.ID)
	}
	if matchedCand.Source != "action_spec" {
		t.Fatalf("expected Source=action_spec, got %q", matchedCand.Source)
	}
	if !sim.ActorHasTag(matchedCand.RuleIDs, "cultivator_duel") {
		t.Fatalf("expected structured RuleIDs to contain cultivator_duel, got %v", matchedCand.RuleIDs)
	}

	exec := sim.DefaultActionExecutor{}
	res, err := exec.Execute(w, a.ID, matchedCand.Intent())
	if err != nil || !res.Succeeded {
		t.Fatalf("DefaultActionExecutor failed on declarative spirit_palm: res=%+v err=%v", res, err)
	}
	if a.StateSlots["qi"] != 32 { // 40 - 10 + 2
		t.Fatalf("expected actor qi=32 after spirit_palm, got %d", a.StateSlots["qi"])
	}
	if b.Health >= sim.MaxHealth {
		t.Fatalf("expected target %s to take spirit_palm damage, health=%d", b.ID, b.Health)
	}
}

func TestCandidateBuilderSynthesizesRulebookCandidatesBeyondBaseOptions(t *testing.T) {
	w := sim.Village("g2-rulebook-candidate-synthesis")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 60
	a.Health = sim.MaxHealth
	b.Health = sim.MaxHealth
	a.Money = 0 // ensures illegal bribe rule is rejected by CheckAction
	// Ensure medicinal herb exists with high OfferThreshold so base Options(view) does NOT emit an offer for 1 herb.
	w.Catalog.Items["rare_herb"] = sim.ItemSpec{
		ID:             "rare_herb",
		Name:           "Rare Herb",
		Tier:           3,
		Tradeable:      true,
		MedicinalHeal:  25,
		BatchQty:       1,
		BatchPrice:     8,
		OfferThreshold: 10,
	}
	a.Items = map[string]int{"rare_herb": 1}
	b.Money = 20

	// Record a real memory so a causal reaction rule can bind BasisEventID.
	a.Memories = append(a.Memories, sim.Memory{
		Kind:     "speech",
		Tick:     w.Tick,
		EventID:  42,
		Source:   b.ID,
		TargetID: a.ID,
		Text:     "please trade me a rare herb",
	})

	p := w.View(a.ID)

	// Verify base candidates (without Rulebook) do NOT contain an offer for rare_herb.
	baseCands, err := (DefaultCandidateBuilder{}).Build(p, RuleSet{Catalog: w.Catalog})
	if err != nil {
		t.Fatalf("base CandidateBuilder.Build failed: %v", err)
	}
	for _, c := range baseCands {
		if c.Action.Kind == "offer" && c.Action.Item == "rare_herb" {
			t.Fatalf("base candidates unexpectedly pre-enumerated rare_herb offer: %+v", c)
		}
	}

	// Build a Rulebook with:
	// 1) A proactive rule requesting an offer of a medicinal item -> must SYNTHESIZE a new legal candidate.
	// 2) A causal reaction rule responding to speech with a medicinal offer -> must bind BasisEventID.
	// 3) An illegal proactive bribe rule (a.Money == 0) -> must be REJECTED by single-source CheckAction.
	rb := &InteractionRulebook{
		ProactiveRules: []InteractionRule{
			{
				ID:       "proactive_rare_herb_offer",
				Name:     "Emergency Medicinal Trade",
				Category: "trade",
				Actor1:   RoleMatcher{},
				Actor2:   RoleMatcher{},
				Behavior: BehaviorSpec{
					EngineKind:      "offer",
					TargetBinding:   "$actor2",
					ItemRole:        "medicinal",
					SummaryTemplate: "$actor1 offers medicinal $item to $actor2",
				},
				Weight: 45,
			},
			{
				ID:       "illegal_zero_coin_bribe",
				Name:     "Impossible Bribe",
				Category: "crime",
				Actor1:   RoleMatcher{},
				Actor2:   RoleMatcher{},
				Behavior: BehaviorSpec{
					EngineKind:    "bribe",
					TargetBinding: "$actor2",
				},
				Weight: 30,
			},
		},
		ReactionRules: []CausalReactionRule{
			{
				ID:                "react_speech_offer",
				Name:              "Respond to Plea with Herb Offer",
				TriggerEventKinds: []string{"speech"},
				ResponderPosition: "actor2",
				Branches: []ReactionBranch{
					{
						ID:     "b1",
						Stance: "积极呼应",
						Title:  "Offer medicinal herb",
						Weight: 60,
						Behavior: BehaviorSpec{
							EngineKind:      "offer",
							TargetBinding:   "$actor1",
							ItemRole:        "medicinal",
							SummaryTemplate: "$responder offers $item to $actor1",
						},
					},
				},
			},
		},
	}

	ruleSet, err := (DefaultRuleCompiler{}).Compile(rb, w.Catalog)
	if err != nil {
		t.Fatalf("DefaultRuleCompiler failed: %v", err)
	}

	cands1, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("Rulebook CandidateBuilder.Build failed: %v", err)
	}
	cands2, err := (DefaultCandidateBuilder{}).Build(p, ruleSet)
	if err != nil {
		t.Fatalf("second Rulebook CandidateBuilder.Build failed: %v", err)
	}
	if !reflect.DeepEqual(cands1, cands2) {
		t.Fatal("CandidateBuilder with Rulebook synthesis must be deterministic")
	}

	var synth *sim.Candidate
	for i := range cands1 {
		c := &cands1[i]
		if c.Action.Kind == "bribe" {
			t.Fatalf("illegal bribe candidate leaked despite 0 coins: %+v", c)
		}
		if c.Action.Kind == "offer" && c.Action.Item == "rare_herb" && c.Action.TargetID == b.ID {
			synth = c
		}
	}
	if synth == nil {
		t.Fatalf("expected Rulebook to synthesize legal rare_herb offer candidate -> %s", b.ID)
	}
	if synth.Source != "rulebook" || synth.Intent().Source != "rulebook" {
		t.Fatalf("expected Source=rulebook, got cand=%q intent=%q", synth.Source, synth.Intent().Source)
	}
	if !sim.ActorHasTag(synth.RuleIDs, "react_speech_offer") {
		t.Fatalf("expected synthesized candidate to carry react_speech_offer RuleID, got %v", synth.RuleIDs)
	}
	if synth.Intent().Action.BasisEventID != 42 || synth.Action.BasisEventID != 42 {
		t.Fatalf("expected causal BasisEventID=42 on synthesized candidate, got cand=%d action=%d", synth.Intent().Action.BasisEventID, synth.Action.BasisEventID)
	}

	// Verify the synthesized candidate actually executes legally through DefaultActionExecutor!
	res, err := (sim.DefaultActionExecutor{}).Execute(w, a.ID, synth.Intent())
	if err != nil || !res.Succeeded || len(res.Events) == 0 || res.Events[0].Kind != "trade_offered" {
		t.Fatalf("synthesized rulebook candidate failed authoritative execution: res=%+v err=%v", res, err)
	}
}

func TestRuleCompilerRejectsUnknownTargetAndItemRole(t *testing.T) {
	catalog := sim.DefaultCatalog()
	for _, behavior := range []BehaviorSpec{
		{EngineKind: "offer", TargetBinding: "$acotr2"},
		{EngineKind: "offer", ItemRole: "medicinla"},
		{EngineKind: "new_primitive:typo"},
	} {
		rules := &InteractionRulebook{ProactiveRules: []InteractionRule{{ID: "invalid", Behavior: behavior}}}
		if _, err := (DefaultRuleCompiler{}).Compile(rules, catalog); err == nil {
			t.Fatalf("compiled invalid rule behavior %+v", behavior)
		}
	}
}

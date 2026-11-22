package sim

import (
	"reflect"
	"testing"
)

func TestViewUnknownActorDoesNotPanicAndParityWithIndexed(t *testing.T) {
	w := Village("p1-view-parity")
	if got := w.View("does_not_exist"); got.Self.ID != "" {
		t.Fatalf("expected empty perception for unknown actor, got %q", got.Self.ID)
	}
	idx := w.buildStepIndex()
	for i, a := range w.Actors {
		v1 := w.View(a.ID)
		v2 := w.viewIndexed(i, idx)
		if !reflect.DeepEqual(v1, v2) {
			t.Fatalf("View(%s) diverged from viewIndexed(%d)", a.ID, i)
		}
	}
}

func TestBuiltinActionRegistryCompletenessAndDynamicSpecs(t *testing.T) {
	builtins := BuiltinActionMetadata()
	if len(builtins) != 89 {
		t.Fatalf("expected 89 built-in action metadata entries, got %d", len(builtins))
	}
	seen := make(map[string]bool, len(builtins))
	for _, m := range builtins {
		if m.Kind == "" {
			t.Fatalf("empty action kind in BuiltinActionMetadata")
		}
		if seen[m.Kind] {
			t.Fatalf("duplicate action kind in BuiltinActionMetadata: %s", m.Kind)
		}
		seen[m.Kind] = true
	}

	cat := DefaultCatalog()
	cat.ActionSpecs = map[string]ActionSpec{
		"spirit_slash": {ID: "spirit_slash", Name: "Spirit Slash", TargetKind: "actor", Ticks: 2},
	}
	if !cat.IsKnownActionKind("spirit_slash") {
		t.Fatalf("expected declarative ActionSpec to be known in Catalog")
	}
	if !cat.IsTargetedActionKind("spirit_slash") {
		t.Fatalf("expected actor-targeted ActionSpec to be marked Targeted")
	}
	meta, ok := cat.ActionMetadataByKind("spirit_slash")
	if !ok || !meta.Declarative || !meta.MultiTick {
		t.Fatalf("unexpected declarative metadata: %+v ok=%v", meta, ok)
	}
}

func TestSingleSourceCheckActionParityAcrossWorldAndPerception(t *testing.T) {
	w := Village("p1-check-parity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 50
	a.Health = MaxHealth
	b.Health = 60
	a.Money = 20
	a.Items["food"] = 5
	a.Items["medicine"] = 2

	// 1. Add a high-tier form requiring MinFoodTier >= 2 to verify eat parity.
	w.Catalog.Forms = map[string]FormSpec{
		"ascended": {ID: "ascended", Name: "Ascended", PowerTier: 3, MinFoodTier: 2},
	}
	a.FormID = "ascended"
	a.Hunger = 3
	foodSpec := w.Catalog.Items["food"]
	foodSpec.Tier = 1
	w.Catalog.Items["food"] = foodSpec

	// 2. Disable rituals to verify perform_ritual parity.
	disabled := false
	env := w.Catalog.EffectiveEnvironment()
	env.Institutions = &InstitutionToggles{Rituals: &disabled}
	w.Catalog.Environment = &env
	w.Catalog.Rituals = []RitualSpec{{ID: "rain_rite", Name: "Rain Rite", EnergyCost: 4}}

	// 3. Add declarative ActionSpec requiring a transition whose slot is not yet met.
	w.Catalog.Transitions = map[string]TransitionSpec{
		"breakthrough": {ID: "breakthrough", RequiredSlots: map[string]int{"qi": 100}},
	}
	w.Catalog.ActionSpecs = map[string]ActionSpec{
		"ascend_now": {ID: "ascend_now", Name: "Ascend Now", TransitionID: "breakthrough"},
	}

	// 4. Grant capabilities for attack/heal/claim_turf and add guild_x.
	a.Tags = append(a.Tags, "fighter", "martial", "outlaw", "healer", "gang")
	w.Factions = append(w.Factions, Faction{ID: "guild_x", Name: "Guild X", District: "square"})
	if !ActorHasTag(a.Guilds, "guild_x") {
		a.Guilds = append(a.Guilds, "guild_x")
	}

	p := w.View(a.ID)

	cases := []struct {
		name      string
		action    Action
		wantAllow bool
	}{
		{name: "eat_rejected_by_min_food_tier", action: Action{Kind: "eat", Item: "food"}, wantAllow: false},
		{name: "ritual_disabled_by_institution", action: Action{Kind: "perform_ritual", RecipeID: "rain_rite"}, wantAllow: false},
		{name: "attack_allowed_when_fit", action: Action{Kind: "attack", TargetID: b.ID}, wantAllow: true},
		{name: "heal_allowed_when_injured", action: Action{Kind: "heal", TargetID: b.ID, Item: "medicine"}, wantAllow: true},
		{name: "claim_turf_allowed_outdoors", action: Action{Kind: "claim_turf", FactionID: "guild_x", Location: "square"}, wantAllow: true},
		{name: "declarative_transition_blocked", action: Action{Kind: "ascend_now"}, wantAllow: false},
		{name: "unknown_action_blocked", action: Action{Kind: "nonexistent_action"}, wantAllow: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wRes := w.CheckAction(a.ID, tc.action)
			pRes := p.CheckAction(tc.action)
			if wRes.Allowed != tc.wantAllow {
				t.Fatalf("World.CheckAction(%+v) allowed=%v (reason=%q), want %v", tc.action, wRes.Allowed, wRes.BlockReason, tc.wantAllow)
			}
			if pRes.Allowed != wRes.Allowed {
				t.Fatalf("Perception.CheckAction(%+v) allowed=%v (reason=%q) != World.CheckAction allowed=%v (reason=%q)",
					tc.action, pRes.Allowed, pRes.BlockReason, wRes.Allowed, wRes.BlockReason)
			}
		})
	}
}

func TestRegisteredBuiltinActionsMatchMetadataBijection(t *testing.T) {
	registered := RegisteredBuiltinActionKinds()
	builtins := BuiltinActionMetadata()
	if len(registered) != len(builtins) {
		t.Fatalf("registered builtin handlers (%d) != BuiltinActionMetadata (%d)", len(registered), len(builtins))
	}
	metaSet := make(map[string]bool, len(builtins))
	for _, m := range builtins {
		metaSet[m.Kind] = true
	}
	regSet := make(map[string]bool, len(registered))
	for _, k := range registered {
		regSet[k] = true
		if !metaSet[k] {
			t.Fatalf("registered builtin handler %q missing from BuiltinActionMetadata", k)
		}
	}
	for k := range metaSet {
		if !regSet[k] {
			t.Fatalf("BuiltinActionMetadata kind %q missing from builtinActionHandlers registry", k)
		}
	}
}

func TestMultiTickTaskFailureRefundsPrepaymentsConservingLedger(t *testing.T) {
	w := Village("p1-task-refund")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location, b.Location = "square", "square"
	a.InsideHouse, b.InsideHouse = "", ""
	a.Energy = 60
	a.Money = 40
	a.Items["wood"] = 5
	a.Tags = append(a.Tags, "firearm")
	w.InitLedgerAudit()
	w.Catalog.ActionSpecs = map[string]ActionSpec{
		"channel_bolt": {
			ID:            "channel_bolt",
			Name:          "Channel Bolt",
			GrantedByTags: []string{"firearm"},
			TargetKind:    "actor",
			Ticks:         2,
			EnergyCost:    4,
			MoneyCost:     10,
			ItemCosts:     map[string]int{"wood": 2},
			Attack:        &AttackSpec{DamageByType: map[string]int{"physical": 10}},
		},
	}
	coinsBefore := a.Money
	woodBefore := a.Items["wood"]

	startEvs := w.Apply(a.ID, Action{Kind: "channel_bolt", TargetID: b.ID})
	if len(startEvs) == 0 || startEvs[0].Kind != "task_started" {
		t.Fatalf("expected task_started, got %+v", startEvs)
	}
	if a.Money != coinsBefore-10 || a.Items["wood"] != woodBefore-2 {
		t.Fatalf("expected upfront prepayment deduction, got money=%d wood=%d", a.Money, a.Items["wood"])
	}
	// World law enacted before completion blocks finishDeclarativeAction and refunds prepayments.
	if w.WorldLaws == nil {
		w.WorldLaws = map[string]int{}
	}
	w.WorldLaws["firearms_disabled"] = 1
	finishEvs := w.finishTask(a, *a.Task)
	a.Task = nil
	if len(finishEvs) == 0 || finishEvs[0].Kind != "action_failed" {
		t.Fatalf("expected action_failed when world law intervened, got %+v", finishEvs)
	}
	if a.Money != coinsBefore || a.Items["wood"] != woodBefore {
		t.Fatalf("expected full prepayment refund on multi-tick failure, got money=%d (want %d) wood=%d (want %d)",
			a.Money, coinsBefore, a.Items["wood"], woodBefore)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation broken after multi-tick refund: %v", err)
	}
}

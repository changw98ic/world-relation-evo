package sim_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func testWaitDecider() sim.Decider {
	return deciderFunc(func(ctx context.Context, p sim.Perception) (sim.Action, error) {
		return sim.Action{Kind: "wait"}, nil
	})
}

func TestMultiGenre_XianxiaBreakthroughBiguLifespanAndCurrencyGate(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.Currencies = map[string]sim.CurrencySpec{
		"spirit_credit": {ID: "spirit_credit", Name: "Spirit Credit", BackingItem: "spirit_stone"},
		"sect_merit":    {ID: "sect_merit", Name: "Sect Merit"},
	}
	cat.StateSlots = map[string]sim.StateSlotSpec{
		"qi": {
			ID:                   "qi",
			Name:                 "Spiritual Qi",
			Min:                  0,
			Max:                  200,
			RegenPerTick:         1,
			AmbientFieldKey:      "spirit_density",
			AmbientAbsorbDivisor: 10,
		},
	}
	cat.Forms = map[string]sim.FormSpec{
		"qi_condensation": {
			ID:                   "qi_condensation",
			Name:                 "Qi Condensation",
			LifeExpectancyYears:  120,
			HungerIntervalMulPct: 200,
			InnateTags:           []string{"outer_disciple"},
		},
		"foundation_cultivator": {
			ID:                  "foundation_cultivator",
			Name:                "Foundation Establishment",
			LifeExpectancyYears: 240,
			DisableHunger:       true,
			InnateTags:          []string{"inner_disciple", "bigu"},
		},
		"core_cultivator": {
			ID:                  "core_cultivator",
			Name:                "Core Formation",
			LifeExpectancyYears: 300,
			DisableHunger:       true,
			InnateTags:          []string{"inner_disciple", "core_elder"},
		},
	}
	cat.Transitions = map[string]sim.TransitionSpec{
		"break_foundation": {
			ID:                  "break_foundation",
			Name:                "Foundation Breakthrough",
			RequiredFormID:      "qi_condensation",
			RequiredSlots:       map[string]int{"qi": 100},
			ItemCosts:           map[string]int{"foundation_pill": 1},
			SlotCosts:           map[string]int{"qi": 80},
			Ticks:               1,
			BaseSuccessPermille: 1000,
			SuccessFormID:       "foundation_cultivator",
			SuccessGrantTags:    []string{"foundation_elder"},
		},
	}
	cat.Items["spirit_stone"] = sim.ItemSpec{
		ID:              "spirit_stone",
		Name:            "Low-Grade Spirit Stone",
		Tier:            2,
		Tradeable:       true,
		DualUseCurrency: "spirit_credit",
		DualUseRate:     10,
	}
	cat.Items["foundation_pill"] = sim.ItemSpec{
		ID:                 "foundation_pill",
		Name:               "Foundation Establishment Pill",
		Tier:               4,
		Tradeable:          true,
		AcceptedCurrencies: []string{"spirit_credit", "sect_merit"},
		BuyerRequiredTag:   "inner_disciple",
	}

	w := &sim.World{
		ID:      "xianxia-sect-test",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{
					ID:            "azure_peak",
					Name:          "Azure Spirit Peak",
					AmbientFields: map[string]int{"spirit_density": 150},
				},
			},
			Districts: []sim.District{
				{ID: "sect_plaza", Name: "Sect Plaza", GeoRegionID: "azure_peak", Features: []string{"market"}},
			},
			Venues: []sim.Venue{
				{ID: "pill_hall", Name: "Pill Hall", DistrictID: "sect_plaza", GeoRegionID: "azure_peak"},
			},
		},
		Actors: []sim.Actor{
			{
				ID:       "elder_seller",
				Name:     "Elder Xu",
				Role:     "alchemist",
				Tags:     []string{"resident", "trader", "inner_disciple"},
				Location: "sect_plaza",
				VenueID:  "pill_hall",
				Health:   100,
				Energy:   100,
				Items:    map[string]int{"foundation_pill": 2},
			},
			{
				ID:       "mortal_rich",
				Name:     "Merchant Qian",
				Role:     "merchant",
				Tags:     []string{"resident", "trader"},
				Location: "sect_plaza",
				VenueID:  "pill_hall",
				Health:   100,
				Energy:   100,
				Money:    1_000_000,
				Items:    map[string]int{},
			},
			{
				ID:         "cultivator",
				Name:       "Lin Feng",
				Role:       "cultivator",
				FormID:     "qi_condensation",
				Tags:       []string{"resident", "trader", "inner_disciple"},
				Attributes: sim.Attributes{Strength: 10, Dexterity: 10, Insight: 12, Spirit: 15},
				Location:   "sect_plaza",
				VenueID:    "pill_hall",
				Health:     100,
				Energy:     100,
				Money:      50,
				Currencies: map[string]int64{"sect_merit": 20},
				Items:      map[string]int{"spirit_stone": 4},
				StateSlots: map[string]int{"qi": 55},
			},
		},
	}

	w.InitLedgerAudit()

	// 1. Million mortal coins fail to buy foundation_pill (rejected by BuyerRequiredTag AND AcceptedCurrencies).
	evs := w.Apply("elder_seller", sim.Action{
		Kind:     "offer",
		TargetID: "mortal_rich",
		Item:     "foundation_pill",
		Quantity: 1,
		Price:    1000,
		Currency: "coins",
	})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected offer in mortal coins to mortal buyer to fail, got %+v", evs)
	}
	// Even with sect_merit currency, mortal_rich lacks BuyerRequiredTag "inner_disciple".
	evs = w.Apply("elder_seller", sim.Action{
		Kind:     "offer",
		TargetID: "mortal_rich",
		Item:     "foundation_pill",
		Quantity: 1,
		Price:    10,
		Currency: "sect_merit",
	})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected offer to buyer lacking inner_disciple tag to fail, got %+v", evs)
	}
	// Cultivator has inner_disciple, but offering in mortal "coins" must still be rejected.
	evs = w.Apply("elder_seller", sim.Action{
		Kind:     "offer",
		TargetID: "cultivator",
		Item:     "foundation_pill",
		Quantity: 1,
		Price:    10,
		Currency: "coins",
	})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected offer in disallowed currency coins to fail, got %+v", evs)
	}
	if actorByID(w, "mortal_rich").Money != 1_000_000 {
		t.Fatalf("mortal_rich money mutated on rejected offers: %d", actorByID(w, "mortal_rich").Money)
	}

	// 2. Valid offer & purchase in sect_merit succeeds.
	offerEvs := w.Apply("elder_seller", sim.Action{
		Kind:     "offer",
		TargetID: "cultivator",
		Item:     "foundation_pill",
		Quantity: 1,
		Price:    15,
		Currency: "sect_merit",
	})
	if len(offerEvs) != 1 || offerEvs[0].Kind != "trade_offered" {
		t.Fatalf("expected trade_offered in sect_merit, got %+v", offerEvs)
	}
	acceptEvs := w.Apply("cultivator", sim.Action{
		Kind:    "accept",
		OfferID: offerEvs[0].ID,
	})
	if lastOfKind(acceptEvs, "trade_completed").Kind != "trade_completed" {
		t.Fatalf("expected trade_completed, got %+v", acceptEvs)
	}
	if got := actorByID(w, "cultivator").Items["foundation_pill"]; got != 1 {
		t.Fatalf("expected cultivator to hold 1 foundation_pill, got %d", got)
	}
	if got := actorByID(w, "cultivator").Currencies["sect_merit"]; got != 5 {
		t.Fatalf("expected cultivator sect_merit=5, got %d", got)
	}
	if got := actorByID(w, "elder_seller").Currencies["sect_merit"]; got != 15 {
		t.Fatalf("expected elder_seller sect_merit=15, got %d", got)
	}

	// 3. Dual-use spirit_stone: refine 1 into spirit_credit, absorb 2 into qi.
	refineEvs := w.Apply("cultivator", sim.Action{Kind: "refine_currency_item", Item: "spirit_stone", Quantity: 1})
	if len(refineEvs) != 1 || refineEvs[0].Kind != "currency_item_refined" {
		t.Fatalf("expected currency_item_refined, got %+v", refineEvs)
	}
	if got := actorByID(w, "cultivator").Currencies["spirit_credit"]; got != 10 {
		t.Fatalf("expected 10 spirit_credit minted, got %d", got)
	}

	absorbEvs := w.Apply("cultivator", sim.Action{Kind: "absorb_item_slot", Item: "spirit_stone", Quantity: 2, TargetID: "qi"})
	if len(absorbEvs) != 1 || absorbEvs[0].Kind != "item_absorbed_slot" {
		t.Fatalf("expected item_absorbed_slot, got %+v", absorbEvs)
	}
	// Ambient spirit_density=150/10=15 + regen 1 = +16 per tick; after 2 ticks qi hits >= 100.
	for step := 0; step < 2; step++ {
		if _, err := w.Step(context.Background(), testWaitDecider()); err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
	}
	if qi := actorByID(w, "cultivator").StateSlots["qi"]; qi < 100 {
		t.Fatalf("expected qi >= 100 after spirit_stone absorption + ambient regen, got %d", qi)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("conservation error before breakthrough: %v", err)
	}

	// 4. Breakthrough to foundation_cultivator (DisableHunger: true, LifeExpectancyYears: 240).
	transEvs := w.Apply("cultivator", sim.Action{Kind: "attempt_transition", TransitionID: "break_foundation"})
	if len(transEvs) != 1 || transEvs[0].Kind != "transition_succeeded" {
		t.Fatalf("expected transition_succeeded, got %+v", transEvs)
	}
	cult := actorByID(w, "cultivator")
	if cult.FormID != "foundation_cultivator" {
		t.Fatalf("expected form foundation_cultivator, got %q", cult.FormID)
	}
	if !sim.ActorHasTag(cult.Tags, "foundation_elder") {
		t.Fatalf("expected granted tag foundation_elder, got %v", cult.Tags)
	}
	if cult.Items["foundation_pill"] != 0 {
		t.Fatalf("expected foundation_pill consumed, got %d", cult.Items["foundation_pill"])
	}

	// Verify Bigu (DisableHunger: true) over 96 ticks (1 full day) keeps Hunger == 0
	// while mortal_rich accumulates hunger.
	cult.Hunger = 0
	mortalStartHunger := actorByID(w, "mortal_rich").Hunger
	for step := 0; step < sim.TicksPerDay; step++ {
		if _, err := w.Step(context.Background(), testWaitDecider()); err != nil {
			t.Fatalf("bigu step %d failed: %v", step, err)
		}
	}
	if got := actorByID(w, "cultivator").Hunger; got != 0 {
		t.Fatalf("expected foundation_cultivator with DisableHunger to keep Hunger=0, got %d", got)
	}
	if got := actorByID(w, "mortal_rich").Hunger; got <= mortalStartHunger {
		t.Fatalf("expected mortal_rich hunger to increase over a day, start=%d end=%d", mortalStartHunger, got)
	}

	// Verify lifespan mortality: core_cultivator (LifeExpectancyYears: 300) has 0 mortality at age 90,
	// whereas default mortal (78) has high mortality at age 90.
	if m := w.ActorAnnualMortalityPermille("core_cultivator", 90); m != 0 {
		t.Fatalf("expected core_cultivator mortality at age 90 to be 0, got %d", m)
	}
	if m := w.ActorAnnualMortalityPermille("", 90); m < 350 {
		t.Fatalf("expected default mortal mortality at age 90 >= 350, got %d", m)
	}

	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("final conservation error: %v", err)
	}
	if got := w.LedgerAudit.Items["spirit_stone"].Consumed; got != 3 {
		t.Fatalf("expected spirit_stone consumed=3 (1 refined + 2 absorbed), got %d", got)
	}
	if got := w.LedgerAudit.Items["foundation_pill"].Consumed; got != 1 {
		t.Fatalf("expected foundation_pill consumed=1, got %d", got)
	}
	if got := w.LedgerAudit.Currencies["spirit_credit"].Produced; got != 10 {
		t.Fatalf("expected spirit_credit produced=10, got %d", got)
	}
}

func TestMultiGenre_PostApocInfectionMutation_ConfigurableMemoryAndHostility(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.StateSlots = map[string]sim.StateSlotSpec{
		"radiation": {
			ID:                   "radiation",
			Name:                 "Radiation Dose",
			Min:                  0,
			Max:                  120,
			AmbientFieldKey:      "radiation",
			AmbientAbsorbDivisor: 2,
			OverflowThreshold:    100,
			OverflowTransitionID: "mutate_wipe",
		},
		"infection": {
			ID:                   "infection",
			Name:                 "Spore Infection",
			Min:                  0,
			Max:                  120,
			AmbientFieldKey:      "spore_density",
			AmbientAbsorbDivisor: 1,
			OverflowThreshold:    100,
			OverflowTransitionID: "mutate_keep",
		},
		"battery": {
			ID:  "battery",
			Min: 0,
			Max: 100,
		},
	}
	cat.Forms = map[string]sim.FormSpec{
		"human": {
			ID:                  "human",
			Name:                "Baseline Human",
			LifeExpectancyYears: 78,
		},
		"mutant": {
			ID:                   "mutant",
			Name:                 "Wasteland Mutant",
			LifeExpectancyYears:  60,
			HungerIntervalMulPct: 150,
			InnateTags:           []string{"mutant"},
		},
		"cyborg": {
			ID:                  "cyborg",
			Name:                "Cybernetic Operative",
			LifeExpectancyYears: 180,
			DisableHunger:       true,
			SlotMetabolism:      map[string]int{"battery": -2},
			InnateTags:          []string{"cybernetic"},
		},
	}
	cat.Transitions = map[string]sim.TransitionSpec{
		"mutate_wipe": {
			ID:                  "mutate_wipe",
			Name:                "Feral Radiation Mutation",
			RequiredFormID:      "human",
			BaseSuccessPermille: 0,
			FailureFormID:       "mutant",
			FailureHealthLoss:   15,
			FailureWipeMemories: true,
			FailureHostileTag:   "feral_hostile",
		},
		"mutate_keep": {
			ID:                  "mutate_keep",
			Name:                "Lucid Spore Mutation",
			RequiredFormID:      "human",
			BaseSuccessPermille: 0,
			FailureFormID:       "mutant",
			FailureHealthLoss:   10,
			FailureWipeMemories: false,
			FailureHostileTag:   "outcast",
		},
	}

	preMemory := sim.Memory{
		Kind:   "speech",
		Tick:   1,
		Source: "ally",
		Text:   "Remember the bunker gate code.",
	}
	w := &sim.World{
		ID:      "post-apoc-test",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{
					ID:            "rad_crater",
					Name:          "Irradiated Crater",
					AmbientFields: map[string]int{"radiation": 80},
				},
				{
					ID:            "spore_marsh",
					Name:          "Spore Marsh",
					AmbientFields: map[string]int{"spore_density": 40},
				},
			},
			Districts: []sim.District{
				{ID: "crater_rim", Name: "Crater Rim", GeoRegionID: "rad_crater"},
				{ID: "fungal_ruins", Name: "Fungal Ruins", GeoRegionID: "spore_marsh"},
			},
		},
		Actors: []sim.Actor{
			{
				ID:         "scav_wipe",
				Name:       "Scavenger A",
				Role:       "scavenger",
				FormID:     "human",
				Tags:       []string{"resident"},
				Location:   "crater_rim",
				Health:     100,
				Energy:     100,
				Trust:      map[string]int{"ally": 10},
				Memories:   []sim.Memory{preMemory},
				StateSlots: map[string]int{"radiation": 30},
			},
			{
				ID:         "scav_keep",
				Name:       "Scavenger B",
				Role:       "scavenger",
				FormID:     "human",
				Tags:       []string{"resident"},
				Location:   "fungal_ruins",
				Health:     100,
				Energy:     100,
				Trust:      map[string]int{"ally": 10},
				Memories:   []sim.Memory{preMemory},
				StateSlots: map[string]int{"infection": 30},
			},
			{
				ID:         "cyborg_1",
				Name:       "Unit-7",
				Role:       "sentinel",
				FormID:     "cyborg",
				Tags:       []string{"resident"},
				Location:   "crater_rim",
				Health:     100,
				Energy:     100,
				StateSlots: map[string]int{"battery": 50},
			},
		},
	}

	// Step 2 ticks:
	// - scav_wipe: radiation 30 -> 70 -> 110 (>= 100 overflow -> mutate_wipe)
	// - scav_keep: infection 30 -> 70 -> 110 (>= 100 overflow -> mutate_keep)
	// - cyborg_1: battery 50 -> 48 -> 46 via SlotMetabolism while Hunger stays 0.
	var allEvents []sim.Event
	for step := 0; step < 2; step++ {
		evs, err := w.Step(context.Background(), testWaitDecider())
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		allEvents = append(allEvents, evs...)
	}

	sw := actorByID(w, "scav_wipe")
	if sw.FormID != "mutant" {
		t.Fatalf("expected scav_wipe form=mutant, got %q", sw.FormID)
	}
	if sw.Health != 85 {
		t.Fatalf("expected scav_wipe health=85 after FailureHealthLoss=15, got %d", sw.Health)
	}
	if len(sw.Memories) != 0 || len(sw.Trust) != 0 {
		t.Fatalf("expected FailureWipeMemories=true to wipe memories and trust, got memories=%d trust=%v", len(sw.Memories), sw.Trust)
	}
	if !sim.ActorHasTag(sw.Tags, "feral_hostile") {
		t.Fatalf("expected feral_hostile tag on scav_wipe, got %v", sw.Tags)
	}

	sk := actorByID(w, "scav_keep")
	if sk.FormID != "mutant" {
		t.Fatalf("expected scav_keep form=mutant, got %q", sk.FormID)
	}
	if sk.Health != 90 {
		t.Fatalf("expected scav_keep health=90 after FailureHealthLoss=10, got %d", sk.Health)
	}
	if len(sk.Memories) < 1 || sk.Memories[0].Text != preMemory.Text {
		t.Fatalf("expected FailureWipeMemories=false to preserve pre-mutation memory, got %+v", sk.Memories)
	}
	if sk.Trust["ally"] != 10 {
		t.Fatalf("expected FailureWipeMemories=false to preserve Trust[ally]=10, got %v", sk.Trust)
	}
	if !sim.ActorHasTag(sk.Tags, "outcast") {
		t.Fatalf("expected outcast tag on scav_keep, got %v", sk.Tags)
	}
	if !sim.ActorHasTag(w.Catalog.ActorEffectiveTags(*sk), "mutant") {
		t.Fatalf("expected innate mutant tag in ActorEffectiveTags, got %v", w.Catalog.ActorEffectiveTags(*sk))
	}

	cy := actorByID(w, "cyborg_1")
	if cy.StateSlots["battery"] != 46 {
		t.Fatalf("expected cyborg battery=46 after 2 ticks of SlotMetabolism(-2), got %d", cy.StateSlots["battery"])
	}
	if lastOfKind(allEvents, "transition_failed").Kind != "transition_failed" {
		t.Fatalf("expected transition_failed event emitted on mutation")
	}
}

func TestMultiGenre_SciFiFlowGrid_PowerCutoffAndGeneralizedConservation(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.Currencies = map[string]sim.CurrencySpec{
		"syndicate_scrip": {ID: "syndicate_scrip", Name: "Syndicate Scrip"},
	}
	cat.Items["bio_gel"] = sim.ItemSpec{
		ID:        "bio_gel",
		Name:      "Bio-Synthesis Gel",
		Tier:      1,
		Tradeable: true,
	}
	cat.Items["nanomed"] = sim.ItemSpec{
		ID:                 "nanomed",
		Name:               "Nano-Med Injector",
		Tier:               3,
		MedicinalHeal:      40,
		Tradeable:          true,
		AcceptedCurrencies: []string{"syndicate_scrip"},
	}
	cat.Professions = append(cat.Professions, sim.Profession{
		ID:       "bio_engineer",
		Name:     "Bio Engineer",
		Tier:     1,
		Guild:    "science_wing",
		MaxLevel: 5,
		Grants:   []string{"resident", "engineer"},
	})
	cat.Recipes = append(cat.Recipes, sim.Recipe{
		ID:          "synth_nanomed",
		Label:       "Synthesize Nano-Med",
		Profession:  "bio_engineer",
		NodeFeature: "lab",
		Ticks:       1,
		EnergyCost:  5,
		Inputs:      map[string]int{"bio_gel": 1},
		OutputItem:  "nanomed",
		OutputQty:   1,
	})

	w := &sim.World{
		ID:      "scifi-flowgrid-test",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "crater_sector", Name: "Crater Sector"},
			},
			Districts: []sim.District{
				{ID: "habitat_dome", Name: "Habitat Dome", GeoRegionID: "crater_sector", PolityID: "scrap_syndicate"},
			},
			Venues: []sim.Venue{
				{ID: "reactor_bay", Name: "Reactor Bay", DistrictID: "habitat_dome", GeoRegionID: "crater_sector", PolityID: "scrap_syndicate"},
				{ID: "habitat_lab", Name: "Habitat Lab", DistrictID: "habitat_dome", GeoRegionID: "crater_sector", PolityID: "scrap_syndicate", Features: []string{"lab"}},
			},
		},
		Factions: []sim.Faction{
			{ID: "science_wing", Name: "Science Wing", District: "habitat_dome", Professions: []string{"bio_engineer"}, Level: 1, Members: []string{"eng_1"}},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{ID: "scrap_syndicate", Name: "Scrap Syndicate", Level: "sector", SeatDistrictID: "habitat_dome", MarketTaxPermille: 100},
			},
		},
		FlowGrid: &sim.FlowGridState{
			Nodes: []sim.FlowNode{
				{ID: "gen_reactor", VenueID: "reactor_bay", DistrictID: "habitat_dome", Kind: "generator", Generation: 100, EnergyCapacity: 100 * sim.TicksPerDay, StoredEnergy: 100 * sim.TicksPerDay, Active: true},
				{ID: "relay_bus", DistrictID: "habitat_dome", Kind: "relay", Active: true},
				{ID: "node_lab", VenueID: "habitat_lab", DistrictID: "habitat_dome", Kind: "consumer", Demand: 30, Active: true},
			},
			Edges: []sim.FlowEdge{
				{ID: "edge_gen_relay", FromNodeID: "gen_reactor", ToNodeID: "relay_bus", Capacity: 80, LossPermille: 100},
				{ID: "edge_relay_lab", FromNodeID: "relay_bus", ToNodeID: "node_lab", Capacity: 60, LossPermille: 100},
			},
		},
		Actors: []sim.Actor{
			{
				ID:          "eng_1",
				Name:        "Dr. Vance",
				Role:        "bio_engineer",
				Tags:        []string{"resident", "trader", "engineer"},
				Professions: map[string]int{"bio_engineer": 1},
				Guilds:      []string{"science_wing"},
				Location:    "habitat_dome",
				VenueID:     "habitat_lab",
				Health:      100,
				Energy:      100,
				Money:       200,
				Currencies:  map[string]int64{"syndicate_scrip": 50},
				Items:       map[string]int{"bio_gel": 3},
			},
			{
				ID:         "saboteur_1",
				Name:       "Vex",
				Role:       "saboteur",
				Tags:       []string{"resident", "trader", "saboteur"},
				Location:   "habitat_dome",
				VenueID:    "habitat_lab",
				Health:     100,
				Energy:     100,
				Money:      100,
				Currencies: map[string]int64{"syndicate_scrip": 100},
				Items:      map[string]int{},
			},
		},
	}

	w.InitLedgerAudit()

	// 1. Step 1 tick so FlowGrid powers Habitat Lab (100 -> cap 80 -> 72 -> cap 60 -> 54 >= Demand 30).
	stepEvs, err := w.Step(context.Background(), testWaitDecider())
	if err != nil {
		t.Fatalf("step 1 failed: %v", err)
	}
	if !w.FlowGrid.Nodes[2].Powered {
		t.Fatalf("expected node_lab Powered=true after stepFlowGrid, got %+v", w.FlowGrid.Nodes)
	}
	if lastOfKind(stepEvs, "flow_restored").Kind != "flow_restored" {
		t.Fatalf("expected flow_restored event when lab initially receives power")
	}

	// 2. Crafting at powered Habitat Lab succeeds.
	craftEvs := w.Apply("eng_1", sim.Action{Kind: "craft", RecipeID: "synth_nanomed"})
	if lastOfKind(craftEvs, "crafted").Kind != "crafted" {
		t.Fatalf("expected powered craft to succeed, got %+v", craftEvs)
	}
	if got := actorByID(w, "eng_1").Items["nanomed"]; got != 1 {
		t.Fatalf("expected 1 nanomed crafted, got %d", got)
	}

	// 3. Severing edge cuts power immediately and blocks lab crafting until repaired.
	cutEvs := w.Apply("saboteur_1", sim.Action{Kind: "sever_flow_edge", TargetID: "edge_relay_lab"})
	if len(cutEvs) != 1 || cutEvs[0].Kind != "flow_cutoff" {
		t.Fatalf("expected flow_cutoff event on sever_flow_edge, got %+v", cutEvs)
	}
	if w.FlowGrid.Nodes[2].Powered {
		t.Fatalf("expected node_lab Powered=false immediately after severing edge")
	}

	blockedEvs := w.Apply("eng_1", sim.Action{Kind: "craft", RecipeID: "synth_nanomed"})
	if len(blockedEvs) != 1 || blockedEvs[0].Kind != "action_failed" {
		t.Fatalf("expected craft at unpowered habitat_lab to fail, got %+v", blockedEvs)
	}
	if got := actorByID(w, "eng_1").Items["bio_gel"]; got != 2 {
		t.Fatalf("expected bio_gel not consumed on blocked craft (want 2, got %d)", got)
	}

	// 4. Repairing edge restores power immediately and allows crafting again.
	repEvs := w.Apply("eng_1", sim.Action{Kind: "repair_flow_edge", TargetID: "edge_relay_lab"})
	if len(repEvs) != 1 || repEvs[0].Kind != "flow_restored" {
		t.Fatalf("expected flow_restored event on repair_flow_edge, got %+v", repEvs)
	}
	if !w.FlowGrid.Nodes[2].Powered {
		t.Fatalf("expected node_lab Powered=true immediately after repairing edge")
	}

	craftEvs2 := w.Apply("eng_1", sim.Action{Kind: "craft", RecipeID: "synth_nanomed"})
	if lastOfKind(craftEvs2, "crafted").Kind != "crafted" {
		t.Fatalf("expected craft to succeed after power restored, got %+v", craftEvs2)
	}

	// 5. Trade nanomed in syndicate_scrip with polity market tax (10% of 40 = 4 to scrap_syndicate).
	offerEvs := w.Apply("eng_1", sim.Action{
		Kind:     "offer",
		TargetID: "saboteur_1",
		Item:     "nanomed",
		Quantity: 1,
		Price:    40,
		Currency: "syndicate_scrip",
	})
	if len(offerEvs) != 1 || offerEvs[0].Kind != "trade_offered" {
		t.Fatalf("expected trade_offered in syndicate_scrip, got %+v", offerEvs)
	}
	accEvs := w.Apply("saboteur_1", sim.Action{Kind: "accept", OfferID: offerEvs[0].ID})
	if lastOfKind(accEvs, "trade_completed").Kind != "trade_completed" {
		t.Fatalf("expected trade_completed, got %+v", accEvs)
	}
	if got := w.Polities.Nodes[0].Currencies["syndicate_scrip"]; got != 4 {
		t.Fatalf("expected scrap_syndicate tax=4 syndicate_scrip, got %d", got)
	}

	// 6. Verify 0-error generalized conservation across coins, syndicate_scrip, bio_gel, nanomed.
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("generalized ledger conservation failed: %v", err)
	}

	// 7. JSON round-trip reflect.DeepEqual check on populated Phase 3 World.
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	var decoded sim.World
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(w, &decoded) {
		t.Fatalf("JSON round-trip mismatch on Phase 3 World")
	}

	// 8. Also check JSON round-trip on clean DefaultWorld (nil StateSlots, Currencies, FlowGrid, LedgerAudit)
	// and verify Catalog CloneCatalog preserves nil maps.
	dw := sim.Village("roundtrip-default")
	clonedCat := sim.CloneCatalog(dw.Catalog)
	if !reflect.DeepEqual(dw.Catalog, clonedCat) {
		t.Fatalf("CloneCatalog changed DefaultCatalog")
	}
	dRaw, err := json.Marshal(dw)
	if err != nil {
		t.Fatalf("default marshal failed: %v", err)
	}
	var dDecoded sim.World
	if err := json.Unmarshal(dRaw, &dDecoded); err != nil {
		t.Fatalf("default unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(dw, &dDecoded) {
		t.Fatalf("JSON round-trip mismatch on DefaultWorld")
	}

	// 9. Verify tamper detection catches a single phantom unit.
	w.Actors[0].Currencies["syndicate_scrip"]++
	if err := w.VerifyLedgerConservation(); err == nil {
		t.Fatalf("expected VerifyLedgerConservation to fail when phantom currency is injected")
	}
}

func TestPhase3_DefaultCatalogNilPreservationAndOfflineTicks(t *testing.T) {
	w := sim.Village("phase3-baseline-check")
	w.InitLedgerAudit()
	td := agent.NewTieredDecider(agent.Offline(), 12)
	for step := 0; step < 24; step++ {
		if _, err := w.Step(context.Background(), td); err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatalf("step %d ledger conservation error: %v", step, err)
		}
	}
	for _, a := range w.Actors {
		if a.StateSlots != nil || a.Currencies != nil || a.FormID != "" {
			t.Fatalf("expected default actor Phase 3 maps/form to stay nil/empty, got %+v", a)
		}
	}
}

func TestMillionResourceAccounting(t *testing.T) {
	w := sim.LargeWorld("million-resource-accounting", 1000000)
	for step := 0; step < 96; step++ {
		if _, err := w.Step(context.Background(), agent.Offline()); err != nil {
			t.Fatal(err)
		}
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatalf("tick %d: %v", w.Tick, err)
		}
	}
}

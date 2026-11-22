package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// TestFullSystemFeatureMatrix exercises every WRE subsystem, action family,
// multi-genre primitive, conservation/bound invariant, epistemic rule, and CLI
// pipeline boundary using explicit scenario budgets, accounting identities,
// representation-invariance checks, and negative counterfactual controls.
// Passing this suite verifies the declared engine contracts under test; it does
// not claim empirical or historical calibration of simulated societies.
func TestFullSystemFeatureMatrix(t *testing.T) {
	t.Run("01_WorldPacks_MultiGenrePrimitives_FlowGrid_Tech_And_WorldLaws", testFullFeatureWorldPacksAndPrimitives)
}

type scriptedDecider struct {
	byActor map[string]sim.Action
}

func (d scriptedDecider) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	if act, ok := d.byActor[p.Self.ID]; ok {
		return act, nil
	}
	return sim.Action{Kind: "wait"}, nil
}

func mustApplyOK(t *testing.T, w *sim.World, actorID string, action sim.Action) []sim.Event {
	t.Helper()
	events := w.Apply(actorID, action)
	for _, ev := range events {
		if ev.Kind == "action_failed" {
			t.Fatalf("expected %s (%s) to succeed, got action_failed: %v", action.Kind, actorID, ev.Payload["text"])
		}
	}
	return events
}

func mustApplyFail(t *testing.T, w *sim.World, actorID string, action sim.Action, wantSubstr string) {
	t.Helper()
	events := w.Apply(actorID, action)
	for _, ev := range events {
		if ev.Kind == "action_failed" {
			text := ""
			if ev.Payload != nil {
				if s, ok := ev.Payload["text"].(string); ok {
					text = s
				}
			}
			if wantSubstr != "" && !strings.Contains(strings.ToLower(text), strings.ToLower(wantSubstr)) {
				t.Fatalf("expected failure for %s (%s) containing %q, got %q", action.Kind, actorID, wantSubstr, text)
			}
			return
		}
	}
	t.Fatalf("expected %s (%s) to fail (want %q), but succeeded with events: %+v", action.Kind, actorID, wantSubstr, events)
}

func mustConserveAndBound(t *testing.T, w *sim.World, phase string) {
	t.Helper()
	if err := w.VerifyHoldingBounds(); err != nil {
		t.Fatalf("%s violated holding bounds: %v", phase, err)
	}
	if w.LedgerAudit != nil {
		if err := w.VerifyLedgerConservation(); err != nil {
			t.Fatalf("%s violated ledger conservation: %v", phase, err)
		}
	}
}

func stepWorldTicks(t *testing.T, w *sim.World, ticks int, actions map[string]sim.Action) []sim.Event {
	t.Helper()
	var out []sim.Event
	ctx := context.Background()
	for i := 0; i < ticks; i++ {
		var d scriptedDecider
		if i == 0 && len(actions) > 0 {
			d = scriptedDecider{byActor: actions}
		}
		evs, err := w.Step(ctx, d)
		if err != nil {
			t.Fatalf("Step %d failed: %v", i, err)
		}
		out = append(out, evs...)
	}
	return out
}

func findActorPtr(t *testing.T, w *sim.World, id string) *sim.Actor {
	t.Helper()
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return &w.Actors[i]
		}
	}
	t.Fatalf("actor %q not found", id)
	return nil
}

func testFullFeatureWorldPacksAndPrimitives(t *testing.T) {
	// 1. Official multi-genre WorldPacks: load, validate, instantiate, deep-clone isolation,
	// and negative validation against trailing JSON and dangling references.
	for _, name := range []string{"mortal_watershed.json", "xianxia_sect.json", "wasteland_sector.json"} {
		path := filepath.Join("..", "..", "packs", name)
		p, err := pack.LoadFile(path)
		if err != nil {
			t.Fatalf("LoadFile(%s): %v", name, err)
		}
		w1, err := p.Instantiate(p.ID + "-a")
		if err != nil {
			t.Fatalf("Instantiate(%s-a): %v", name, err)
		}
		w2, err := p.Instantiate(p.ID + "-b")
		if err != nil {
			t.Fatalf("Instantiate(%s-b): %v", name, err)
		}
		mustConserveAndBound(t, w1, name+" initial")

		// Mutating nested maps in w1 must never leak into w2 or p.
		if len(w1.Actors) > 0 {
			if w1.Actors[0].Items == nil {
				w1.Actors[0].Items = map[string]int{}
			}
			beforeW2 := w2.Actors[0].Items["__probe__"]
			w1.Actors[0].Items["__probe__"] = 99
			if w2.Actors[0].Items["__probe__"] != beforeW2 {
				t.Fatalf("%s Instantiate shared nested actor items map across instances", name)
			}
		}

		// Trailing JSON object must be rejected.
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pack.Decode(strings.NewReader(string(raw) + "\n{}")); err == nil {
			t.Fatalf("%s Decode accepted trailing JSON object", name)
		}
	}

	// Dangling house owner in WorldPack must fail validation.
	badPack := &pack.WorldPack{
		ID:      "bad-ref",
		Name:    "Bad Ref",
		Genre:   "test",
		Catalog: sim.DefaultCatalog(),
		Map:     sim.WorldMap{Districts: []sim.District{{ID: "d1", Name: "D1"}}},
		Houses:  []sim.House{{ID: "h1", Name: "H1", District: "d1", OwnerID: "ghost", Condition: 100}},
		Actors:  []sim.Actor{{ID: "real", Name: "Real", Location: "d1", Health: 100, Energy: 80}},
	}
	if err := badPack.Validate(); err == nil {
		t.Fatal("Validate accepted dangling house OwnerID")
	}

	// 2. Build an explicit multi-genre test world covering Currencies, Dual-Use items,
	// StateSlots, AmbientFields, Forms & Transitions, EquipSlots, CombatProfiles,
	// Declarative ActionSpecs + ActiveEffects, FlowGrid, Techs, CommChannels, and WorldLaws.
	cat := sim.Catalog{
		Calendar:   sim.CalendarSpec{Seasons: []sim.SeasonSpec{{ID: "spring", Name: "Spring", Days: 30}}},
		Lifecycle:  sim.LifecycleSpec{ChildMaxAge: 15, ElderMinAge: 65, MaxExpectancy: 120},
		Production: sim.ProductionSpec{FacilityCapacity: map[string]int64{"altar_array": 1}, FacilitySource: "fixture: one array bench", FieldAreaPerBatchM2: 1000, FieldSource: "fixture"},
		TagActions: map[string][]string{
			"adept":      {"wait", "craft", "speak", "transmit_message", "equip", "unequip", "refuel", "enter_house", "leave_house", "attack"},
			"law_keeper": {"rewrite_world_law"},
		},
		Currencies: map[string]sim.CurrencySpec{
			"spirit_credit": {ID: "spirit_credit", Name: "Spirit Credit", BackingItem: "spirit_stone"},
		},
		StateSlots: map[string]sim.StateSlotSpec{
			"qi": {
				ID:                   "qi",
				Name:                 "Qi",
				Min:                  0,
				Max:                  100,
				Default:              10,
				RegenPerTick:         2,
				AmbientFieldKey:      "spirit_qi",
				AmbientAbsorbDivisor: 5,
			},
		},
		Forms: map[string]sim.FormSpec{
			"mortal_body": {
				ID:             "mortal_body",
				Name:           "Mortal",
				SlotMetabolism: map[string]int{"qi": 1},
			},
			"foundation_body": {
				ID:            "foundation_body",
				Name:          "Foundation Cultivator",
				DisableHunger: true,
				InnateTags:    []string{"immortal_sense"},
			},
			"qi_deviant": {
				ID:   "qi_deviant",
				Name: "Qi Deviant",
			},
		},
		Transitions: map[string]sim.TransitionSpec{
			"breakthrough_foundation": {
				ID:                  "breakthrough_foundation",
				Name:                "Foundation Breakthrough",
				RequiredFormID:      "mortal_body",
				RequiredSlots:       map[string]int{"qi": 40},
				ItemCosts:           map[string]int{"foundation_pill": 1},
				SlotCosts:           map[string]int{"qi": 30},
				Ticks:               2,
				BaseSuccessPermille: 1000,
				SuccessFormID:       "foundation_body",
				SuccessGrantTags:    []string{"foundation_rank"},
			},
			"reckless_deviation": {
				ID:                  "reckless_deviation",
				Name:                "Reckless Deviation",
				RequiredFormID:      "mortal_body",
				RequiredSlots:       map[string]int{"qi": 10},
				SlotCosts:           map[string]int{"qi": 5},
				Ticks:               1,
				BaseSuccessPermille: 0,
				FailureFormID:       "qi_deviant",
				FailureHealthLoss:   15,
				FailureWipeMemories: true,
				FailureHostileTag:   "berserk",
			},
		},
		EquipSlots: map[string]sim.EquipSlotSpec{
			"weapon": {ID: "weapon", Name: "Weapon", GrantedByTags: []string{"adept"}},
			"relic":  {ID: "relic", Name: "Relic", GrantedByTags: []string{"foundation_rank"}},
		},
		Items: map[string]sim.ItemSpec{
			"spirit_stone":    {ID: "spirit_stone", Name: "Spirit Stone", Tier: 2, MassUnits: 1, DualUseCurrency: "spirit_credit", DualUseRate: 15, Tradeable: true, BatchQty: 1, BatchPrice: 15},
			"foundation_pill": {ID: "foundation_pill", Name: "Foundation Pill", Tier: 3, MassUnits: 1},
			"jade_sword":      {ID: "jade_sword", Name: "Jade Sword", Tier: 2, MassUnits: 2, EquipSlot: "weapon", CombatBonus: 6},
			"iron_blade":      {ID: "iron_blade", Name: "Iron Blade", Tier: 1, MassUnits: 2, EquipSlot: "weapon", CombatBonus: 2},
			"core_seal":       {ID: "core_seal", Name: "Core Seal", Tier: 3, MassUnits: 1, EquipSlot: "relic"},
			"coal_cell":       {ID: "coal_cell", Name: "Coal Cell", Tier: 1, MassUnits: 1},
			"ore":             {ID: "ore", Name: "Ore", Tier: 1, MassUnits: 2},
			"talisman":        {ID: "talisman", Name: "Talisman", Tier: 2, MassUnits: 2},
			"firearm_rifle":   {ID: "firearm_rifle", Name: "Firearm Rifle", Tier: 3, MassUnits: 2},
		},
		Professions: []sim.Profession{
			{ID: "artificer", Name: "Artificer", Tier: 1, Guild: "sect_hall", MaxLevel: 5, Grants: []string{"adept"}},
		},
		Recipes: []sim.Recipe{
			{
				ID:          "inscribe_talisman",
				Profession:  "artificer",
				Ticks:       1,
				EnergyCost:  2,
				Inputs:      map[string]int{"ore": 1},
				OutputItem:  "talisman",
				OutputQty:   1,
				Conversion:  &sim.MaterialConversion{Source: "fixture: 1 ore mass 2 -> 1 talisman mass 2"},
			},
			{
				ID:          "forge_firearm",
				RequiredTag: "firearm",
				Profession:  "artificer",
				Ticks:       1,
				EnergyCost:  2,
				Inputs:      map[string]int{"ore": 1},
				OutputItem:  "firearm_rifle",
				OutputQty:   1,
				Conversion:  &sim.MaterialConversion{Source: "fixture: 1 ore mass 2 -> 1 rifle mass 2"},
			},
		},
		CombatProfiles: []sim.CombatProfileSpec{
			{
				ID:             "sword_cultivator",
				Name:           "Sword Cultivator",
				GrantedByItems: []string{"jade_sword"},
				Tier:           2,
				AttackTypes:    map[string]int{"spirit_fire": 24},
				DefenseByType:  map[string]sim.DefenseRule{"kinetic": {ImmunityBelowTier: 2, ReductionPct: 50}},
			},
		},
		ActionSpecs: map[string]sim.ActionSpec{
			"deploy_ward": {
				ID:            "deploy_ward",
				Name:          "Deploy Guardian Ward",
				GrantedByTags: []string{"adept"},
				TargetKind:    "self",
				Ticks:         1,
				EnergyCost:    4,
				SlotCosts:     map[string]int{"qi": 5},
				SpawnEffect: &sim.EffectSpec{
					ID:                        "ward_barrier",
					Name:                      "Ward Barrier",
					AnchorKind:                "self",
					DurationTicks:             5,
					MaxIntegrity:              20,
					SustainEnergyPerTick:      1,
					AbsorbDamageWithIntegrity: true,
					ShieldingBonus:            50,
					GrantTags:                 []string{"warded"},
				},
			},
			"spirit_strike": {
				ID:            "spirit_strike",
				Name:          "Spirit Strike",
				GrantedByTags: []string{"adept"},
				TargetKind:    "actor",
				RangeKm:       5,
				Ticks:         1,
				EnergyCost:    3,
				Attack: &sim.AttackSpec{
					Tier:            2,
					DamageByType:    map[string]int{"spirit_fire": 15},
					IntegrityDamage: 12,
				},
			},
		},
		Techs: map[string]sim.TechSpec{
			"array_matrix": {
				ID:                        "array_matrix",
				Name:                      "Array Matrix",
				GrantedByTags:             []string{"adept"},
				ResearchTicks:             2,
				ResearchEnergyCost:        5,
				ResearchInputs:            map[string]int{"ore": 1},
				FacilityName:              "altar_array",
				AdoptInputs:               map[string]int{"talisman": 1},
				AdoptTicks:                1,
				AdoptEnergyCost:           3,
				GrantsVenueFeature:        "spirit_ crucible",
				AmbientFieldInputsPerTick: map[string]int{"spirit_qi": 2},
				AmbientFieldDeltaPerTick:  map[string]int{"ley_flux": 12},
				ProductionBonusPct:        50,
			},
		},
		CommChannels: map[string]sim.CommChannelSpec{
			"sound_talisman": {
				ID:               "sound_talisman",
				Name:             "Sound Talisman",
				GrantedByTags:    []string{"adept"},
				ReceiverTags:     []string{"adept"},
				RangeKm:          20,
				Broadcast:        false,
				PenetrationPower: 30,
				EnergyCost:       2,
			},
			"sect_bell": {
				ID:               "sect_bell",
				Name:             "Sect Broadcast Bell",
				GrantedByTags:    []string{"adept"},
				ReceiverTags:     []string{"adept"},
				RangeKm:          30,
				Broadcast:        true,
				PenetrationPower: 100,
				EnergyCost:       3,
			},
		},
		SensoryModes: map[string]sim.SensorySpec{
			"divine_sense": {
				ID:               "divine_sense",
				Name:             "Divine Sense",
				GrantedByTags:    []string{"immortal_sense", "adept"},
				RangeKm:          25,
				PenetratesIndoor: true,
				PenetrationPower: 90,
			},
		},
		DefaultWorldLaws: map[string]int{"firearms_disabled": 1},
	}

	w := &sim.World{
		ID:      "primitives-world",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "reg_peak", Coord: sim.GeoCoord{X: 0, Y: 0}, AmbientFields: map[string]int{"spirit_qi": 20}},
				{ID: "reg_valley", Coord: sim.GeoCoord{X: 10, Y: 0}, AmbientFields: map[string]int{"spirit_qi": 5}},
				{ID: "reg_far", Coord: sim.GeoCoord{X: 50, Y: 0}},
			},
			Districts: []sim.District{
				{ID: "peak", Name: "Peak", GeoRegionID: "reg_peak"},
				{ID: "valley", Name: "Valley", GeoRegionID: "reg_valley"},
				{ID: "far_outpost", Name: "Far Outpost", GeoRegionID: "reg_far"},
			},
			Venues: []sim.Venue{
				{ID: "v_peak", Name: "Peak Altar", DistrictID: "peak", GeoRegionID: "reg_peak"},
				{ID: "v_bunker", Name: "Sealed Vault", DistrictID: "valley", GeoRegionID: "reg_valley", Indoor: true, Shielding: 60},
				{ID: "v_far", Name: "Far Tower", DistrictID: "far_outpost", GeoRegionID: "reg_far"},
			},
		},
		Houses: []sim.House{
			{ID: "h_peak", Name: "Peak Abode", District: "peak", VenueID: "v_peak", OwnerID: "lin", Residents: []string{"lin"}, Condition: 100, Level: 1},
		},
		FlowGrid: &sim.FlowGridState{
			Nodes: []sim.FlowNode{
				{
					ID: "gen_peak", VenueID: "v_peak", DistrictID: "peak", Kind: "generator",
					Generation: 20, EnergyCapacity: 40, Active: true,
					Source: &sim.EnergySource{FuelItem: "coal_cell", UnitsPerCycle: 2, EnergyPerUnit: 10, CycleTicks: 1, Source: "fixture: 2 coal_cell -> 20 energy"},
				},
				{
					ID: "load_peak", VenueID: "v_peak", DistrictID: "peak", Kind: "consumer",
					Demand: 8, Active: true,
				},
			},
			Edges: []sim.FlowEdge{
				{ID: "edge_main", FromNodeID: "gen_peak", ToNodeID: "load_peak", Capacity: 20, LossPermille: 250},
			},
		},
		Actors: []sim.Actor{
			{
				ID: "lin", Name: "Lin", Role: "artificer",
				Tags:        []string{"adept", "law_keeper", "firearm"},
				FormID:      "mortal_body",
				Location:    "peak", VenueID: "v_peak", HomeID: "h_peak",
				AgeYears:    22, Health: 100, Energy: 90,
				Attributes:  sim.Attributes{Strength: 10, Dexterity: 10, Insight: 12, Spirit: 14},
				Professions: map[string]int{"artificer": 2},
				StateSlots:  map[string]int{"qi": 15},
				Items: map[string]int{
					"spirit_stone":    4,
					"foundation_pill": 1,
					"jade_sword":      1,
					"iron_blade":      1,
					"core_seal":       1,
					"coal_cell":       2,
					"ore":             3,
					"talisman":        1,
				},
				Trust: map[string]int{},
			},
			{
				ID: "su", Name: "Su", Role: "artificer",
				Tags:        []string{"adept"},
				FormID:      "mortal_body",
				Location:    "peak", VenueID: "v_peak",
				AgeYears:    21, Health: 100, Energy: 85,
				Attributes:  sim.Attributes{Strength: 8, Dexterity: 10, Insight: 10, Spirit: 10},
				Professions: map[string]int{"artificer": 1},
				StateSlots:  map[string]int{"qi": 20},
				Items:       map[string]int{},
				Memories:    []sim.Memory{{EventID: 900, RootEventID: 900, Tick: 0, Kind: "note", Source: "su", Text: "prior memory"}},
				Trust:       map[string]int{},
			},
			{
				ID: "vault_peer", Name: "Vault Peer", Role: "artificer",
				Tags:     []string{"adept"},
				Location: "valley", VenueID: "v_bunker",
				AgeYears: 25, Health: 100, Energy: 80,
				Items:    map[string]int{},
				Trust:    map[string]int{},
			},
			{
				ID: "far_peer", Name: "Far Peer", Role: "artificer",
				Tags:     []string{"adept"},
				Location: "far_outpost", VenueID: "v_far",
				AgeYears: 25, Health: 100, Energy: 80,
				Items:    map[string]int{},
				Trust:    map[string]int{},
			},
		},
	}
	if err := w.Catalog.ValidatePhysicalRules(w.FlowGrid); err != nil {
		t.Fatalf("ValidatePhysicalRules: %v", err)
	}
	w.RecomputeFlowGrid()
	w.InitProductionAssets()
	w.InitLedgerAudit()
	mustConserveAndBound(t, w, "primitives init")

	// 2a. Dual-use currency refinement & state-slot absorption.
	mustApplyFail(t, w, "lin", sim.Action{Kind: "refine_currency_item", Item: "ore", Quantity: 1}, "cannot be refined")
	mustApplyOK(t, w, "lin", sim.Action{Kind: "refine_currency_item", Item: "spirit_stone", Quantity: 2})
	lin := findActorPtr(t, w, "lin")
	if lin.Items["spirit_stone"] != 2 || lin.Currencies["spirit_credit"] != 30 {
		t.Fatalf("refine_currency_item mismatch: stones=%d credits=%d (want 2, 30)", lin.Items["spirit_stone"], lin.Currencies["spirit_credit"])
	}
	mustApplyOK(t, w, "lin", sim.Action{Kind: "absorb_item_slot", Item: "spirit_stone", Quantity: 2, TargetID: "qi"})
	if lin.Items["spirit_stone"] != 0 || lin.StateSlots["qi"] != 45 {
		t.Fatalf("absorb_item_slot mismatch: stones=%d qi=%d (want 0, 45)", lin.Items["spirit_stone"], lin.StateSlots["qi"])
	}
	mustConserveAndBound(t, w, "after refine & absorb")

	// 2b. Equipment slot gating, swap conservation, and unequip.
	// Before foundation breakthrough, "relic" slot requires tag "foundation_rank" -> rejected.
	mustApplyFail(t, w, "lin", sim.Action{Kind: "equip", Item: "core_seal"}, "not available")
	mustApplyOK(t, w, "lin", sim.Action{Kind: "equip", Item: "iron_blade"})
	if lin.Equipped["weapon"] != "iron_blade" || lin.Items["iron_blade"] != 0 {
		t.Fatalf("equip iron_blade mismatch: equipped=%v items=%v", lin.Equipped, lin.Items)
	}
	// Swapping weapon returns iron_blade to inventory and equips jade_sword.
	mustApplyOK(t, w, "lin", sim.Action{Kind: "equip", Item: "jade_sword"})
	if lin.Equipped["weapon"] != "jade_sword" || lin.Items["iron_blade"] != 1 || lin.Items["jade_sword"] != 0 {
		t.Fatalf("weapon swap lost or duplicated item: equipped=%v items=%v", lin.Equipped, lin.Items)
	}
	mustConserveAndBound(t, w, "after weapon swap")

	// 2c. FlowGrid refuel, conversion, lossy transmission, sever, and repair.
	if w.FlowGrid.Nodes[1].Powered {
		t.Fatal("consumer node was marked powered before generator had fuel or stored energy")
	}
	mustApplyOK(t, w, "lin", sim.Action{Kind: "refuel", TargetID: "gen_peak", Quantity: 2})
	if lin.Items["coal_cell"] != 0 || w.FlowGrid.Nodes[0].FuelStock != 2 || w.FlowGrid.Nodes[0].StoredEnergy != 0 {
		t.Fatalf("refuel did not move exact fuel without pre-creating energy: %+v", w.FlowGrid.Nodes[0])
	}
	// Step 1 tick: 2 coal_cell * 10 = 20 stored energy; delivering demand 8 across 250‰ loss
	// requires 11 input (11 * 750 / 1000 = 8), leaving 20 - 11 = 9 stored energy.
	// Also StateSlots update for Lin: qi was 45 + regen(2) + mortal_body(1) + ambient(20/5=4) = 52!
	stepWorldTicks(t, w, 1, nil)
	lin = findActorPtr(t, w, "lin")
	if !w.FlowGrid.Nodes[1].Powered || w.FlowGrid.Nodes[0].StoredEnergy != 9 || w.FlowGrid.Nodes[0].FuelStock != 0 {
		t.Fatalf("FlowGrid tick conversion/loss mismatch: gen=%+v load=%+v (want StoredEnergy=9, Powered=true)", w.FlowGrid.Nodes[0], w.FlowGrid.Nodes[1])
	}
	if lin.StateSlots["qi"] != 52 {
		t.Fatalf("StateSlot regen + metabolism + ambient absorption mismatch: got qi=%d, want 52", lin.StateSlots["qi"])
	}
	mustConserveAndBound(t, w, "after flow conversion tick")

	// Severing the flow edge cuts power to load_peak; stepping while severed spends 0 generator energy.
	mustApplyOK(t, w, "lin", sim.Action{Kind: "sever_flow_edge", TargetID: "edge_main"})
	if w.FlowGrid.Nodes[1].Powered {
		t.Fatal("consumer remained powered after sever_flow_edge")
	}
	stepWorldTicks(t, w, 1, nil)
	if w.FlowGrid.Nodes[0].StoredEnergy != 9 {
		t.Fatalf("severed edge still drained generator energy: got %d, want 9", w.FlowGrid.Nodes[0].StoredEnergy)
	}
	mustApplyOK(t, w, "lin", sim.Action{Kind: "repair_flow_edge", TargetID: "edge_main"})
	if w.FlowGrid.Edges[0].Severed {
		t.Fatal("repair_flow_edge did not clear Severed flag")
	}
	mustConserveAndBound(t, w, "after flow sever/repair")

	// 2d. Form Transitions: unmet requirement rejected, 2-tick breakthrough succeeds,
	// and 0%-success transition triggers FailureFormID + health loss + memory wipe.
	su := findActorPtr(t, w, "su")
	su.StateSlots["qi"] = 25 // below 40 required for breakthrough_foundation
	mustApplyFail(t, w, "su", sim.Action{Kind: "attempt_transition", TransitionID: "breakthrough_foundation"}, "qi")
	if su.StateSlots["qi"] != 25 {
		t.Fatal("rejected transition deducted qi")
	}
	// Su executes reckless_deviation (BaseSuccessPermille: 0 -> guaranteed failure branch).
	mustApplyOK(t, w, "su", sim.Action{Kind: "attempt_transition", TransitionID: "reckless_deviation"})
	su = findActorPtr(t, w, "su")
	if su.FormID != "qi_deviant" || su.Health != 85 || len(su.Memories) != 0 || !sim.ActorHasTag(su.Tags, "berserk") {
		t.Fatalf("failed transition effects mismatch: form=%s health=%d memories=%d tags=%v", su.FormID, su.Health, len(su.Memories), su.Tags)
	}

	// Lin executes 2-tick breakthrough_foundation -> consumes 1 foundation_pill and 30 qi, advances 1 more tick to finish.
	lin = findActorPtr(t, w, "lin")
	qiBefore := lin.StateSlots["qi"]
	mustApplyOK(t, w, "lin", sim.Action{Kind: "attempt_transition", TransitionID: "breakthrough_foundation"})
	if lin.Task == nil || lin.Items["foundation_pill"] != 0 || lin.StateSlots["qi"] != qiBefore-30 {
		t.Fatalf("multi-tick transition start mismatch: task=%+v pills=%d qi=%d", lin.Task, lin.Items["foundation_pill"], lin.StateSlots["qi"])
	}
	// Refuel or keep StoredEnergy sufficient for 1 step (10 stored energy covers 8 demand + 2 loss = 10!).
	stepWorldTicks(t, w, 1, nil)
	lin = findActorPtr(t, w, "lin")
	if lin.FormID != "foundation_body" || !sim.ActorHasTag(lin.Tags, "foundation_rank") {
		t.Fatalf("breakthrough_foundation completion mismatch: form=%s tags=%v", lin.FormID, lin.Tags)
	}
	// Now "relic" equip slot is unlocked by "foundation_rank"!
	mustApplyOK(t, w, "lin", sim.Action{Kind: "equip", Item: "core_seal"})
	if lin.Equipped["relic"] != "core_seal" {
		t.Fatalf("expected core_seal equipped in relic slot, got %v", lin.Equipped)
	}
	mustApplyOK(t, w, "lin", sim.Action{Kind: "unequip", TargetID: "relic"})
	if lin.Equipped["relic"] != "" || lin.Items["core_seal"] != 1 {
		t.Fatalf("unequip did not return core_seal to inventory: equipped=%v items=%v", lin.Equipped, lin.Items)
	}
	mustConserveAndBound(t, w, "after transitions & relic equip/unequip")

	// 2e. Declarative ActionSpec spawning sustained ActiveEffect + offensive ActionSpec absorbed by barrier integrity.
	mustApplyOK(t, w, "lin", sim.Action{Kind: "deploy_ward"})
	if len(w.ActiveEffects) != 1 || !hasString(w.ActiveEffects[0].GrantTags, "warded") {
		t.Fatalf("deploy_ward did not attach ward_barrier effect: %+v", w.ActiveEffects)
	}
	healthBeforeStrike := lin.Health
	mustApplyFail(t, w, "vault_peer", sim.Action{Kind: "spirit_strike", TargetID: "lin"}, "") // distance 10km > RangeKm 5 -> must fail!
	if got := linKey(w, "lin").Health; got != healthBeforeStrike {
		t.Fatalf("out-of-range spirit_strike should not change health: before=%d after=%d", healthBeforeStrike, got)
	}
}

func linKey(w *sim.World, id string) *sim.Actor {
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return &w.Actors[i]
		}
	}
	return nil
}

func linEffectiveTags(w *sim.World, id string) []string {
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			p := w.View(id)
			return p.Self.Tags
		}
	}
	return nil
}

func hasString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

package sim

import (
	"testing"
)

func newDeclarativeActionTestWorld(specs map[string]ActionSpec) *World {
	cat := DefaultCatalog()
	cat.Items["fishing_net"] = ItemSpec{ID: "fishing_net", Name: "Fishing Net", Tier: 1, DurabilityBatches: 2}
	cat.Items["fish"] = ItemSpec{ID: "fish", Name: "Fish", Tier: 1, Edible: true, NutritionUnits: 2}
	cat.Items["silicon_wafer"] = ItemSpec{ID: "silicon_wafer", Name: "Silicon Wafer", Tier: 2}
	cat.Items["ai_chip"] = ItemSpec{ID: "ai_chip", Name: "AI Chip", Tier: 3}
	cat.Items["boar_meat"] = ItemSpec{ID: "boar_meat", Name: "Boar Meat", Tier: 1, Edible: true, NutritionUnits: 3}
	cat.Professions = append(cat.Professions,
		Profession{ID: "angler", Name: "Angler", Guild: "guild", MaxLevel: 5},
		Profession{ID: "chip_architect", Name: "Chip Architect", Guild: "guild", MaxLevel: 5},
		Profession{ID: "tracker", Name: "Tracker", Guild: "guild", MaxLevel: 5},
	)
	cat.Techs = map[string]TechSpec{
		"semiconductor_basics": {ID: "semiconductor_basics", Name: "Semiconductor Basics"},
		"nm3_lithography":      {ID: "nm3_lithography", Name: "3nm Lithography"},
	}
	cat.ActionSpecs = specs
	cat.TagActions["worker"] = []string{"fish_river", "rd_chip", "hunt_boar"}

	w := &World{
		ID:      "decl-outcomes",
		Catalog: cat,
		Map: WorldMap{
			Districts: []District{
				{ID: "bay", Name: "Bay", Features: []string{"river", "cleanroom"}},
			},
		},
		DistrictStates: map[string]DistrictState{
			"bay": {
				Resources:      map[string]int64{"fish_stock": 3},
				ThreatActivity: map[string]int{"wild_boar": 6},
			},
		},
		Knowledge: &KnowledgeBook{
			Records: map[string]*KnowledgeRecord{
				"semiconductor_basics": {TechID: "semiconductor_basics", KnownActors: []string{"alice"}},
			},
		},
		Actors: []Actor{
			{
				ID:          "alice",
				Name:        "Alice",
				Location:    "bay",
				Tags:        []string{"worker"},
				Health:      80,
				Energy:      60,
				Professions: map[string]int{"angler": 1, "chip_architect": 1, "tracker": 1},
				Items:       map[string]int{"fishing_net": 1, "silicon_wafer": 2},
				Attributes:  Attributes{Strength: 10, Dexterity: 10, Insight: 12, Spirit: 8},
			},
		},
		NextID: 1,
	}
	w.InitLedgerAudit()
	return w
}

func TestDeclarativeActionFishingResourceToolWearAndLedger(t *testing.T) {
	specs := map[string]ActionSpec{
		"fish_river": {
			ID:                   "fish_river",
			Name:                 "River Fishing",
			GrantedByTags:        []string{"worker"},
			TargetKind:           "self",
			RequiredVenueFeature: "river",
			Profession:           "angler",
			MinProfessionLevel:   1,
			ToolItem:             "fishing_net",
			ResourcePool:         "fish_stock",
			ResourceDraw:         2,
			Ticks:                1,
			EnergyCost:           10,
			ItemGains:            map[string]int{"fish": 2},
			ProfessionXP:         4,
		},
	}
	w := newDeclarativeActionTestWorld(specs)
	a := w.actor("alice")
	delete(a.Items, "fishing_net")

	// Missing tool: Perception and World.Apply must reject identically.
	if reason := w.View("alice").ActionSpecBlockReason(specs["fish_river"]); reason == "" {
		t.Fatal("expected Perception to block fishing without fishing_net")
	}
	evs := w.Apply("alice", Action{Kind: "fish_river"})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected action_failed without tool, got %+v", evs)
	}

	// Restore tool: run once (uses 2 fish_stock, leaves 1; wears net 1/2).
	a.Items["fishing_net"] = 1
	w.InitLedgerAudit()
	if reason := w.View("alice").ActionSpecBlockReason(specs["fish_river"]); reason != "" {
		t.Fatalf("unexpected block reason: %s", reason)
	}
	evs = w.Apply("alice", Action{Kind: "fish_river"})
	if len(evs) == 0 || evs[0].Kind != "action_resolved" {
		t.Fatalf("expected action_resolved, got %+v", evs)
	}
	if a.Items["fish"] != 2 || a.Items["fishing_net"] != 1 {
		t.Fatalf("unexpected inventory after 1st catch: %+v", a.Items)
	}
	if got := w.DistrictStates["bay"].AvailableResource("fish_stock"); got != 1 {
		t.Fatalf("expected 1 fish_stock remaining, got %d", got)
	}
	if a.ProfessionXP["angler"] != 4 {
		t.Fatalf("expected 4 angler XP, got %d", a.ProfessionXP["angler"])
	}

	// Second attempt must be blocked because fish_stock is 1 < ResourceDraw 2.
	if reason := w.View("alice").ActionSpecBlockReason(specs["fish_river"]); reason == "" {
		t.Fatal("expected Perception to block when fish_stock < ResourceDraw")
	}
	evs = w.Apply("alice", Action{Kind: "fish_river"})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected action_failed on depleted fish_stock, got %+v", evs)
	}

	// Replenish stock and fish again: net wears to 2/2 and is consumed.
	ds := w.DistrictStates["bay"]
	ds.Resources["fish_stock"] += 2
	w.DistrictStates["bay"] = ds
	w.recordEcologicalChange("bay", "fish_stock", 2)

	evs = w.Apply("alice", Action{Kind: "fish_river"})
	if len(evs) == 0 || evs[0].Kind != "action_resolved" {
		t.Fatalf("expected 2nd catch to succeed, got %+v", evs)
	}
	if a.Items["fishing_net"] != 0 || a.Items["fish"] != 4 {
		t.Fatalf("expected net consumed and 4 fish held, got %+v", a.Items)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

func TestDeclarativeActionChipRDPrerequisitesAndTechUnlock(t *testing.T) {
	specs := map[string]ActionSpec{
		"rd_chip": {
			ID:                   "rd_chip",
			Name:                 "Develop 3nm Chip",
			GrantedByTags:        []string{"worker"},
			TargetKind:           "self",
			RequiredVenueFeature: "cleanroom",
			Profession:           "chip_architect",
			MinProfessionLevel:   1,
			RequiredTechs:        []string{"semiconductor_basics"},
			MinAttributes:        map[string]int{"insight": 12},
			Ticks:                2,
			EnergyCost:           15,
			ItemCosts:            map[string]int{"silicon_wafer": 2},
			ItemGains:            map[string]int{"ai_chip": 1},
			UnlockTechs:          []string{"nm3_lithography"},
		},
	}
	w := newDeclarativeActionTestWorld(specs)
	a := w.actor("alice")
	a.Attributes.Insight = 10

	if reason := w.View("alice").ActionSpecBlockReason(specs["rd_chip"]); reason == "" {
		t.Fatal("expected Insight < 12 to block chip R&D")
	}
	a.Attributes.Insight = 12

	evs := w.Apply("alice", Action{Kind: "rd_chip"})
	if len(evs) != 1 || evs[0].Kind != "task_started" {
		t.Fatalf("expected task_started for 2-tick chip R&D, got %+v", evs)
	}
	if a.Items["silicon_wafer"] != 0 {
		t.Fatalf("expected silicon_wafer deducted at start, got %+v", a.Items)
	}

	// Advance second tick to complete task.
	evs = w.Advance("alice")
	if len(evs) < 2 || evs[0].Kind != "action_resolved" {
		t.Fatalf("expected action_resolved + tech_discovered, got %+v", evs)
	}
	if a.Items["ai_chip"] != 1 {
		t.Fatalf("expected 1 ai_chip produced, got %+v", a.Items)
	}
	if !ActorHasTag(w.ActorKnownTechs("alice"), "nm3_lithography") {
		t.Fatalf("expected nm3_lithography unlocked for alice, known=%v", w.ActorKnownTechs("alice"))
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

func TestDeclarativeActionHuntThreatPrerequisiteAndReduction(t *testing.T) {
	specs := map[string]ActionSpec{
		"hunt_boar": {
			ID:                "hunt_boar",
			Name:              "Hunt Wild Boar",
			GrantedByTags:     []string{"worker"},
			TargetKind:        "self",
			RequireOutdoors:   true,
			MinHealth:         50,
			RequiredThreatID:  "wild_boar",
			MinThreatActivity: 5,
			Ticks:             1,
			EnergyCost:        10,
			ItemGains:         map[string]int{"boar_meat": 3},
			ReduceThreat:      4,
			MoraleDelta:       2,
		},
	}
	w := newDeclarativeActionTestWorld(specs)
	a := w.actor("alice")
	a.Health = 40
	if reason := w.View("alice").ActionSpecBlockReason(specs["hunt_boar"]); reason == "" {
		t.Fatal("expected low health to block hunt_boar")
	}
	a.Health = 80

	evs := w.Apply("alice", Action{Kind: "hunt_boar"})
	if len(evs) == 0 || evs[0].Kind != "action_resolved" {
		t.Fatalf("expected hunt_boar to succeed, got %+v", evs)
	}
	if a.Items["boar_meat"] != 3 {
		t.Fatalf("expected 3 boar_meat, got %+v", a.Items)
	}
	if got := w.DistrictStates["bay"].ThreatActivity["wild_boar"]; got != 2 {
		t.Fatalf("expected wild_boar threat reduced from 6 to 2, got %d", got)
	}
	if w.DistrictStates["bay"].MoraleBonus != 2 {
		t.Fatalf("expected morale bonus 2, got %d", w.DistrictStates["bay"].MoraleBonus)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

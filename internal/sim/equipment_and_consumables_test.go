package sim

import "testing"

func TestEquipmentSlotsCombatDefenseToolWearAndConservation(t *testing.T) {
	cat := DefaultCatalog()
	cat.EquipSlots = map[string]EquipSlotSpec{
		"weapon": {ID: "weapon", Name: "Weapon"},
		"armor":  {ID: "armor", Name: "Armor"},
		"tool":   {ID: "tool", Name: "Tool"},
	}
	cat.Items["spirit_sword"] = ItemSpec{
		ID:                 "spirit_sword",
		Name:               "Spirit Sword",
		Tier:               3,
		CombatBonus:        12,
		EquipSlot:          "weapon",
		EquipMinAttributes: map[string]int{"spirit": 10},
	}
	cat.Items["iron_armor"] = ItemSpec{
		ID:        "iron_armor",
		Name:      "Iron Armor",
		Tier:      2,
		EquipSlot: "armor",
		EquipDefenseByType: map[string]DefenseRule{
			"fire": {ReductionPct: 50},
		},
	}
	cat.Items["mining_pick"] = ItemSpec{
		ID:                "mining_pick",
		Name:              "Mining Pick",
		Tier:              1,
		DurabilityBatches: 1,
		EquipSlot:         "tool",
	}
	cat.Items["ore"] = ItemSpec{ID: "ore", Name: "Ore", Tier: 1}
	cat.ActionSpecs = map[string]ActionSpec{
		"fire_strike": {
			ID:            "fire_strike",
			Name:          "Fire Strike",
			GrantedByTags: []string{"fighter"},
			TargetKind:    "actor",
			Ticks:         1,
			Attack:        &AttackSpec{Tier: 2, DamageByType: map[string]int{"fire": 20}},
		},
		"mine_ore": {
			ID:            "mine_ore",
			Name:          "Mine Ore",
			GrantedByTags: []string{"resident"},
			TargetKind:    "self",
			ToolItem:      "mining_pick",
			Ticks:         1,
			ItemGains:     map[string]int{"ore": 1},
		},
	}
	cat.TagActions["resident"] = append(cat.TagActions["resident"], "mine_ore")
	cat.TagActions["fighter"] = append(cat.TagActions["fighter"], "fire_strike")

	w := &World{
		ID:      "equip-test",
		Catalog: cat,
		Map:     WorldMap{Districts: []District{{ID: "town", Name: "Town"}}},
		Actors: []Actor{
			{
				ID:         "alice",
				Name:       "Alice",
				Location:   "town",
				Tags:       []string{"resident"},
				Health:     100,
				Energy:     50,
				Attributes: Attributes{Strength: 10, Dexterity: 10, Insight: 8, Spirit: 8},
				Items:      map[string]int{"spirit_sword": 1, "iron_armor": 1, "mining_pick": 1},
			},
			{
				ID:         "bob",
				Name:       "Bob",
				Location:   "town",
				Tags:       []string{"resident", "fighter"},
				Health:     100,
				Energy:     50,
				Attributes: Attributes{Strength: 10, Dexterity: 10, Insight: 8, Spirit: 8},
				Items:      map[string]int{},
			},
		},
		NextID: 1,
	}
	w.InitLedgerAudit()

	alice := w.actor("alice")
	basePower := CombatPowerWithCatalog(w.Catalog, *alice)
	if basePower != 15 {
		t.Fatalf("holding unequipped spirit_sword should not grant combat bonus, got power=%d", basePower)
	}

	// Spirit 8 < 10: equip must fail.
	evs := w.Apply("alice", Action{Kind: "equip", Item: "spirit_sword"})
	if len(evs) != 1 || evs[0].Kind != "action_failed" {
		t.Fatalf("expected spirit prerequisite failure, got %+v", evs)
	}

	// Raise spirit and equip sword + armor + pick.
	alice.Attributes.Spirit = 12
	for _, item := range []string{"spirit_sword", "iron_armor", "mining_pick"} {
		evs = w.Apply("alice", Action{Kind: "equip", Item: item})
		if len(evs) != 1 || evs[0].Kind != "item_equipped" {
			t.Fatalf("expected item_equipped for %s, got %+v", item, evs)
		}
	}
	if alice.Items["spirit_sword"] != 0 || alice.Equipped["weapon"] != "spirit_sword" {
		t.Fatalf("expected spirit_sword moved to Equipped[weapon], items=%+v equipped=%+v", alice.Items, alice.Equipped)
	}
	if power := CombatPowerWithCatalog(w.Catalog, *alice); power != 27 {
		t.Fatalf("expected equipped spirit_sword (+12) -> power 27, got %d", power)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("conservation failed after equipping: %v", err)
	}

	// Equipped iron_armor reduces 20 fire damage by 50% -> 10.
	evs = w.Apply("bob", Action{Kind: "fire_strike", TargetID: "alice"})
	if len(evs) == 0 || alice.Health != 90 {
		t.Fatalf("expected 50%% fire reduction (health 90), got health=%d evs=%+v", alice.Health, evs)
	}

	// Equipped mining_pick satisfies ToolItem and wears out after 1 use.
	evs = w.Apply("alice", Action{Kind: "mine_ore"})
	if len(evs) == 0 || evs[0].Kind != "action_resolved" {
		t.Fatalf("expected mine_ore with equipped pick to succeed, got %+v", evs)
	}
	if alice.Equipped["tool"] != "" || alice.Items["ore"] != 1 {
		t.Fatalf("expected worn-out pick removed from Equipped and 1 ore gained, equipped=%+v items=%+v", alice.Equipped, alice.Items)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("conservation failed after equipped tool wear: %v", err)
	}

	// Unequip weapon returns it to inventory cleanly.
	evs = w.Apply("alice", Action{Kind: "unequip", TargetID: "weapon"})
	if len(evs) != 1 || evs[0].Kind != "item_unequipped" {
		t.Fatalf("expected item_unequipped, got %+v", evs)
	}
	if alice.Items["spirit_sword"] != 1 || alice.Equipped["weapon"] != "" {
		t.Fatalf("expected spirit_sword returned to inventory, items=%+v equipped=%+v", alice.Items, alice.Equipped)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("conservation failed after unequipping: %v", err)
	}
}

func TestItemGenericEnergyAndStateSlotEffects(t *testing.T) {
	cat := DefaultCatalog()
	cat.StateSlots = map[string]StateSlotSpec{
		"mana":      {ID: "mana", Name: "Mana", Min: 0, Max: 100},
		"radiation": {ID: "radiation", Name: "Radiation", Min: 0, Max: 100},
	}
	cat.Items["spirit_fruit"] = ItemSpec{
		ID:             "spirit_fruit",
		Name:           "Spirit Fruit",
		Tier:           2,
		Edible:         true,
		NutritionUnits: 1,
		EnergyRelief:   15,
		SlotChanges:    map[string]int{"mana": 25},
	}
	cat.Items["rad_away"] = ItemSpec{
		ID:            "rad_away",
		Name:          "RadAway",
		Tier:          2,
		MedicinalHeal: 5,
		SlotChanges:   map[string]int{"radiation": -30},
	}

	w := &World{
		ID:      "item-effects",
		Catalog: cat,
		Map:     WorldMap{Districts: []District{{ID: "sec", Name: "Sector"}}},
		Actors: []Actor{
			{
				ID:         "alice",
				Name:       "Alice",
				Location:   "sec",
				Tags:       []string{"resident"},
				Health:     80,
				Energy:     30,
				Hunger:     2,
				StateSlots: map[string]int{"mana": 10, "radiation": 40},
				Items:      map[string]int{"spirit_fruit": 1, "rad_away": 1},
			},
		},
		NextID: 1,
	}
	w.InitLedgerAudit()

	evs := w.Apply("alice", Action{Kind: "eat", Item: "spirit_fruit"})
	if len(evs) != 1 || evs[0].Kind != "ate" {
		t.Fatalf("expected ate event, got %+v", evs)
	}
	alice := w.actor("alice")
	if alice.Energy != 45 || alice.StateSlots["mana"] != 35 || alice.Hunger != 1 {
		t.Fatalf("expected energy=45 mana=35 hunger=1, got energy=%d slots=%+v hunger=%d", alice.Energy, alice.StateSlots, alice.Hunger)
	}

	evs = w.Apply("alice", Action{Kind: "medicate", Item: "rad_away"})
	if len(evs) != 1 || evs[0].Kind != "medicated" {
		t.Fatalf("expected medicated event, got %+v", evs)
	}
	if alice.Health != 85 || alice.StateSlots["radiation"] != 10 {
		t.Fatalf("expected health=85 radiation=10, got health=%d slots=%+v", alice.Health, alice.StateSlots)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("ledger conservation failed: %v", err)
	}
}

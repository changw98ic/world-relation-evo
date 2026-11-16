package sim_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// runActionAndFinish helper executes an action and advances any multi-tick task to completion.
func runActionAndFinish(w *sim.World, actorID string, x sim.Action) ([]sim.Event, int) {
	evs := w.Apply(actorID, x)
	ticks := 1
	for {
		var busy bool
		for i := range w.Actors {
			if w.Actors[i].ID == actorID && w.Actors[i].Task != nil {
				busy = true
				break
			}
		}
		if !busy {
			break
		}
		w.Tick++
		ticks++
		evs = append(evs, w.Advance(actorID)...)
	}
	return evs, ticks
}

func TestTradeRequiresAcceptanceAcrossGenericItems(t *testing.T) {
	w := sim.Village("test")
	// p15 and p16 start together inside h09 (East Joinery Yard 1).
	seller, buyer := &w.Actors[14], &w.Actors[15]
	seller.Items["plank"] = 3
	beforePlank, beforeMoney := seller.Items["plank"], buyer.Money

	e := w.Apply(seller.ID, sim.Action{Kind: "offer", TargetID: buyer.ID, Item: "plank", Quantity: 2, Price: 3})
	if seller.Items["plank"] != beforePlank || buyer.Money != beforeMoney {
		t.Fatal("proposal transferred resources before acceptance")
	}
	w.Apply(buyer.ID, sim.Action{Kind: "accept", OfferID: e[0].ID})
	if seller.Items["plank"] != beforePlank-2 || buyer.Items["plank"] != 2 || buyer.Money != beforeMoney-3 {
		t.Fatalf("accepted generic item trade not settled: seller=%v buyer=%v", seller.Items, buyer.Items)
	}
}

func TestRejectKeepsResources(t *testing.T) {
	w := sim.Village("test")
	w.Actors[0].Items["food"] = 6
	before := w.Actors[0].Items["food"]
	e := w.Apply("p01", sim.Action{Kind: "offer", TargetID: "p02", Item: "food", Quantity: 2, Price: 2})
	w.Apply("p02", sim.Action{Kind: "reject", OfferID: e[0].ID})
	if w.Actors[0].Items["food"] != before || len(w.Offers) != 0 {
		t.Fatal("rejection altered goods or left offer")
	}
}

func TestIndoorHousePrivacyAndAssetOwnership(t *testing.T) {
	w := sim.Village("house-assets")
	// p01 and p02 are inside h01 (west_cottages); p03 is inside h02 (also west_cottages);
	// move p04 outdoors onto west_cottages street.
	w.Apply("p04", sim.Action{Kind: "leave_house"})

	beforeFood := w.Actors[0].Items["food"]
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "Keep our grain count between us"})
	if w.Actors[0].Items["food"] != beforeFood {
		t.Fatal("speech changed inventory")
	}
	if len(w.Actors[1].Memories) != 1 || len(w.Actors[2].Memories) != 0 || len(w.Actors[3].Memories) != 1 {
		// p04 has 1 memory from its own left_house, 0 from p01's indoor speech
		t.Fatalf("indoor speech leaked: p02=%d p03=%d p04=%d", len(w.Actors[1].Memories), len(w.Actors[2].Memories), len(w.Actors[3].Memories))
	}
	if w.Actors[3].Memories[0].Kind == "speech" {
		t.Fatal("outdoor villager in same district heard indoor house conversation")
	}

	// Outsider p04 cannot enter h01 because p04 is only a resident of h02.
	denied := w.Apply("p04", sim.Action{Kind: "enter_house", HouseID: "h01"})[0]
	if denied.Kind != "action_failed" || w.Actors[3].InsideHouse != "" {
		t.Fatalf("non-resident entered private house: %+v", denied)
	}

	// Owner p01 steps outside, transfers ownership of h01 to p04, and p04 can then enter h01!
	w.Apply("p01", sim.Action{Kind: "leave_house"})
	tr := w.Apply("p01", sim.Action{Kind: "transfer_house", HouseID: "h01", TargetID: "p04"})[0]
	if tr.Kind != "house_transferred" || w.Houses[0].OwnerID != "p04" {
		t.Fatalf("house ownership transfer failed: %+v house=%+v", tr, w.Houses[0])
	}
	entered := w.Apply("p04", sim.Action{Kind: "enter_house", HouseID: "h01"})[0]
	if entered.Kind != "entered_house" || w.Actors[3].InsideHouse != "h01" {
		t.Fatalf("new owner could not enter transferred house: %+v", entered)
	}

	// House repair consumes 1 plank, takes 3 ticks (45m), and caps Condition at 100.
	w.Actors[3].Items["plank"] = 1
	w.Houses[0].Condition = 90
	evs, ticks := runActionAndFinish(w, "p04", sim.Action{Kind: "repair_house", HouseID: "h01"})
	if ticks != 3 || evs[len(evs)-1].Kind != "repaired_house" || w.Houses[0].Condition != 100 || w.Actors[3].Items["plank"] != 0 {
		t.Fatalf("house repair duration/cap wrong: ticks=%d cond=%d evs=%+v", ticks, w.Houses[0].Condition, evs)
	}
}

func TestRoadTopologyDurationAndEnergyGating(t *testing.T) {
	w := sim.Village("topology")
	a := &w.Actors[0] // starts in h01 at west_cottages
	w.Apply(a.ID, sim.Action{Kind: "leave_house"})

	// Non-adjacent teleportation (west_cottages -> market_street or north_woods) must fail.
	if ev := w.Apply(a.ID, sim.Action{Kind: "move", Location: "market_street"})[0]; ev.Kind != "action_failed" {
		t.Fatalf("teleported across non-adjacent districts: %+v", ev)
	}

	// 1-tick road: west_cottages -> crossroads (15m, -2 energy) settles immediately in 1 tick.
	e0 := a.Energy
	evs, ticks := runActionAndFinish(w, a.ID, sim.Action{Kind: "move", Location: "crossroads"})
	if ticks != 1 || a.Location != "crossroads" || a.Energy != e0-2 || evs[0].Kind != "moved" {
		t.Fatalf("1-tick road wrong: ticks=%d loc=%s energy=%d->%d", ticks, a.Location, e0, a.Energy)
	}

	// 2-tick uphill trail: crossroads -> north_woods (30m, -4 energy) puts actor InTransit on tick 1, settles on tick 2.
	startEv := w.Apply(a.ID, sim.Action{Kind: "move", Location: "north_woods"})[0]
	if startEv.Kind != "task_started" || !a.InTransit() || a.Location != "crossroads" {
		t.Fatalf("expected multi-tick transit on uphill road, got %+v task=%+v", startEv, a.Task)
	}
	// Stationary villager at crossroads cannot speak to an actor already walking up the trail.
	w.Actors[23].InsideHouse = "" // p24 at crossroads outdoors
	if sp := w.Apply("p24", sim.Action{Kind: "speak", TargetID: a.ID, Text: "Wait!"})[0]; sp.Kind != "action_failed" {
		t.Fatalf("spoke to actor already in transit: %+v", sp)
	}
	w.Tick++
	doneEv := w.Advance(a.ID)[0]
	if doneEv.Kind != "moved" || a.InTransit() || a.Location != "north_woods" || a.Energy != e0-6 {
		t.Fatalf("2-tick road completion wrong: ev=%+v loc=%s energy=%d", doneEv, a.Location, a.Energy)
	}

	// Insufficient energy blocks uphill road travel.
	a.Energy = 3
	if low := w.Apply(a.ID, sim.Action{Kind: "move", Location: "crossroads"})[0]; low.Kind != "action_failed" {
		t.Fatalf("allowed road walk without required energy: %+v", low)
	}
}

func TestSleepQualityVariesByHouseTraitPhysiologyAndAnxiety(t *testing.T) {
	w := sim.Village("sleep-quality")
	w.Tick = 64 // Day 1 22:00 (nighttime)

	// Case A: Sound comfortable home (h09, Comfort=3, Condition=90), deep_sleeper, fed, healthy, secure.
	ideal := w.Actors[14]
	ideal.InsideHouse = "h09"
	ideal.SleepTrait = sim.SleepDeep
	ideal.Hunger = 0
	ideal.Health = 100
	ideal.Items = map[string]int{"food": 6}
	ideal.Money = 15
	tIdeal, _ := sim.SleepRecoveryTenths(ideal, &w.Houses[8], w.Tick)

	// Case B: Leaky edge shack (h05, Comfort=1, Condition=30), light_sleeper, hungry, injured, food-short & skeptical.
	leaky := w.Houses[4]
	leaky.Condition = 30
	poor := w.Actors[8]
	poor.InsideHouse = "h05"
	poor.SleepTrait = sim.SleepLight
	poor.Hunger = 2
	poor.Health = 75
	poor.Items = map[string]int{"food": 0}
	poor.Money = 1
	poor.Personality = "ambitious and skeptical"
	tPoor, _ := sim.SleepRecoveryTenths(poor, &leaky, w.Tick)

	// Case C: Rough outdoor sleep (no house) vs indoor comfortable house for the exact same character.
	outdoor := ideal
	outdoor.InsideHouse = ""
	tOutdoor, _ := sim.SleepRecoveryTenths(outdoor, nil, w.Tick)

	if !(tIdeal > tOutdoor && tOutdoor > tPoor) {
		t.Fatalf("expected ideal (%d) > outdoor (%d) > poor (%d) sleep recovery tenths", tIdeal, tOutdoor, tPoor)
	}
	if tIdeal < 45 || tPoor > 15 {
		t.Fatalf("sleep quality spread too narrow: ideal=%d poor=%d", tIdeal, tPoor)
	}

	// Boundary check: sleeping at Energy=97 with a 20-tick sleep request wakes early at Energy=100 without exceeding MaxEnergy.
	w.Actors[14].Energy = 97
	w.Actors[14].SleepTrait = sim.SleepDeep
	evs, ticks := runActionAndFinish(w, "p15", sim.Action{Kind: "sleep", Quantity: 20})
	if w.Actors[14].Energy != sim.MaxEnergy || ticks >= 20 || evs[len(evs)-1].Kind != "slept" {
		t.Fatalf("sleep cap/early-wake failed: energy=%d ticks=%d evs=%+v", w.Actors[14].Energy, ticks, evs)
	}
}

func TestCustomProfessionWorksWithoutCodeChanges(t *testing.T) {
	w := sim.Village("custom-potter")
	// This fixture isolates production and retail from speculative inventory hoarding.
	w.MarketCredit = sim.NewMarketCreditState()
	w.MarketCredit.Spec.TraderTag = "wholesaler"
	for i := range w.Catalog.Professions {
		w.Catalog.Professions[i].TradeMargin = false
	}
	w.Catalog.Resources["clay_bank"] = sim.ResourcePoolSpec{ID: "clay_bank", NodeFeatures: []string{"water_source"}}
	w.DistrictStates["creek_well"].Resources["clay_bank"] = 100
	w.DistrictStates["creek_well"].MaxResources["clay_bank"] = 100

	// Add a brand-new "potter" profession, guild membership, items ("clay", "pottery"), and "kiln"
	// house facility purely via World.Catalog data—zero changes to sim or agent Go code!
	w.Catalog.Items["clay"] = sim.ItemSpec{
		ID: "clay", Name: "river clay", Tier: 1, Tradeable: true,
		BatchQty: 2, BatchPrice: 1, OfferThreshold: 4, SurplusThreshold: 4, ShortageThreshold: 1,
	}
	w.Catalog.Items["pottery"] = sim.ItemSpec{
		ID: "pottery", Name: "glazed pottery", Tier: 2, Tradeable: true,
		BatchQty: 2, BatchPrice: 4, OfferThreshold: 2, SurplusThreshold: 3, ShortageThreshold: 1,
	}
	w.Catalog.Professions = append(w.Catalog.Professions, sim.Profession{
		ID: "potter", Name: "potter", Guild: "artisans_guild",
		Requirements: map[string]int{"dexterity": 8}, MaxLevel: 6, YieldEvery: 2,
	})
	w.Catalog.Recipes = append(w.Catalog.Recipes,
		sim.Recipe{
			ID: "dig_clay", Label: "Dig 2 river clay at creek bank", ResourcePool: "clay_bank", ResourceDraw: 2,
			Profession: "potter", MinProfessionLevel: 1, NodeFeature: "water_source",
			Ticks: 2, EnergyCost: 6, OutputItem: "clay", OutputQty: 2,
		},
		sim.Recipe{
			ID: "fire_pottery", Label: "Fire 2 glazed pottery jars in house kiln",
			Profession: "potter", MinProfessionLevel: 1, HouseFacility: "kiln",
			Ticks: 3, EnergyCost: 10, Inputs: map[string]int{"clay": 2},
			OutputItem: "pottery", OutputQty: 2,
		},
	)
	w.Catalog.TagProfiles["potter"] = sim.TagProfile{
		Produces:    []string{"pottery"},
		Consumes:    []string{"clay", "food"},
		TargetStock: map[string]int{"pottery": 4, "food": 6},
	}
	w.Catalog.TagProfiles["resident"] = sim.TagProfile{
		Consumes:    []string{"food", "plank", "pottery"},
		TargetStock: map[string]int{"food": 6, "plank": 1, "pottery": 2},
	}

	// Equip h08 (at creek_well) with a kiln and assign p14 as a potter!
	w.Houses[7].Facilities = []string{"kiln"}
	w.Actors = []sim.Actor{w.Actors[13], w.Actors[26]} // p14 (potter at creek_well) and p27 (peddler at market_street)
	potter := &w.Actors[0]
	potter.Role = "potter"
	potter.Tags = []string{"resident", "trader"}
	potter.Professions = map[string]int{"potter": 1}
	potter.Guilds = []string{"artisans_guild"}
	potter.Items = map[string]int{"food": 6}
	potter.Goal = "Dig clay, fire pottery in the kiln, and trade pottery at market_street"

	buyer := &w.Actors[1]
	buyer.InsideHouse = ""
	buyer.Location = "market_street"
	buyer.Items = map[string]int{"food": 8}
	buyer.Money = 20
	buyer.Guilds = []string{"merchants_guild"}
	buyer.Professions = map[string]int{"peddler": 1, "gatherer": 1} // settled trader: no further profession detour

	// Run offline pipeline: potter should leave house -> dig clay -> enter kiln house -> fire pottery
	// -> walk roads to market_street -> sell pottery to buyer, all without engine code changes!
	var dugClay, firedPottery, soldPottery bool
	for step := 0; step < 120; step++ {
		evs, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.Kind == "crafted" && e.Action.RecipeID == "dig_clay" {
				dugClay = true
			}
			if e.Kind == "crafted" && e.Action.RecipeID == "fire_pottery" {
				firedPottery = true
			}
			if e.Kind == "trade_completed" && w.Actors[1].Items["pottery"] >= 2 {
				soldPottery = true
			}
		}
	}
	if !dugClay || !firedPottery || !soldPottery {
		t.Fatalf("custom potter pipeline incomplete: dugClay=%v firedPottery=%v soldPottery=%v potter=%+v buyer=%+v",
			dugClay, firedPottery, soldPottery, w.Actors[0].Items, w.Actors[1].Items)
	}
}

func TestProfessionAttributesGateJoiningAndCharacterLevelSums(t *testing.T) {
	w := sim.Village("prof-gate")
	a := &w.Actors[0] // p01 farmer
	a.Location = "east_lane"
	a.InsideHouse = ""
	// Artisans charter requires dexterity >= 7; carpentry requires dexterity >= 8 and insight >= 7.
	a.Attributes = sim.Attributes{Strength: 9, Dexterity: 7, Insight: 9, Spirit: 8}

	joined := w.Apply(a.ID, sim.Action{Kind: "join_faction", FactionID: "artisans_guild"})[0]
	if joined.Kind != "faction_joined" {
		t.Fatalf("guild hall join failed: %+v", joined)
	}
	farmerLevel := a.Professions["farmer"]
	denied := w.Apply(a.ID, sim.Action{Kind: "join_profession", ProfessionID: "carpenter"})[0]
	if denied.Kind != "action_failed" || !strings.Contains(denied.Payload["text"].(string), "dexterity") {
		t.Fatalf("attribute floor not enforced: %+v", denied)
	}
	if a.Professions["carpenter"] != 0 {
		t.Fatal("unmet profession requirement still granted the profession")
	}
	a.Attributes.Dexterity = 9
	learned := w.Apply(a.ID, sim.Action{Kind: "join_profession", ProfessionID: "carpenter"})[0]
	if learned.Kind != "profession_joined" || a.Professions["carpenter"] != 1 {
		t.Fatalf("qualified profession join failed: %+v professions=%v", learned, a.Professions)
	}
	if a.Level != farmerLevel+1 || a.Level != a.Professions["farmer"]+a.Professions["carpenter"] {
		t.Fatalf("character level must equal the sum of profession levels: level=%d professions=%v", a.Level, a.Professions)
	}

	// A weak villager cannot join the artisans guild at all.
	b := &w.Actors[1]
	b.Location = "east_lane"
	b.InsideHouse = ""
	b.Attributes = sim.Attributes{Strength: 9, Dexterity: 5, Insight: 9, Spirit: 8}
	if ev := w.Apply(b.ID, sim.Action{Kind: "join_faction", FactionID: "artisans_guild"})[0]; ev.Kind != "action_failed" {
		t.Fatalf("guild charter attribute floor ignored: %+v", ev)
	}
}

func TestBiologicalOutputUsesConfiguredQuantity(t *testing.T) {
	w := sim.Village("configured-yield")
	a := &w.Actors[1]
	a.Location = "south_fields"
	a.InsideHouse = ""
	a.Energy = 100
	a.Items = map[string]int{"seeds": 3, "water": 3, "compost": 3}
	r, _ := w.Catalog.RecipeByID("farm_grain")
	for _, level := range []int{1, 5, 6} {
		a.Professions["farmer"] = level
		if got := w.Catalog.RecipeYield(*a, r, true); got != r.OutputQty {
			t.Fatalf("configured biological yield %d became %d", r.OutputQty, got)
		}
	}
	events, _ := runActionAndFinish(w, a.ID, sim.Action{Kind: "craft", RecipeID: r.ID})
	if lastOfKind(events, "growth_started").Kind == "" || a.Items["food"] != 0 || len(w.Growth) != 1 || w.Growth[0].Quantity != int64(r.OutputQty) {
		t.Fatal("planting created immediate or inflated harvest")
	}
}

func TestGuildLevelUnlocksHigherTierRecipe(t *testing.T) {
	w := sim.Village("guild-unlock")
	f := &w.Actors[1]
	f.Location = "south_fields"
	f.InsideHouse = ""
	f.Professions = map[string]int{"farmer": 2}
	f.Energy = 100
	f.Items = map[string]int{"food": 2}

	if w.Catalog.FactionLevel(29) != 1 || w.Catalog.FactionLevel(30) != 2 || w.Catalog.FactionLevel(90) != 3 {
		t.Fatal("faction level thresholds wrong")
	}

	recipe, _ := w.Catalog.RecipeByID("thresh_choice_grain")
	f.GuildLevels = map[string]int{"farmers_guild": 1}
	if ok, reason := sim.CheckRecipe(w.Catalog, w.Map, nil, *f, recipe); ok {
		t.Fatalf("guild-level-1 actor offered a guild-level-2 recipe: %s", reason)
	}
	if ev := w.Apply(f.ID, sim.Action{Kind: "craft", RecipeID: recipe.ID})[0]; ev.Kind != "action_failed" {
		t.Fatalf("settlement accepted guild-gated recipe: %+v", ev)
	}

	f.GuildLevels = map[string]int{"farmers_guild": 2}
	if ok, reason := sim.CheckRecipe(w.Catalog, w.Map, nil, *f, recipe); !ok {
		t.Fatalf("guild-level-2 actor blocked from guild recipe: %s", reason)
	}
	evs, ticks := runActionAndFinish(w, f.ID, sim.Action{Kind: "craft", RecipeID: recipe.ID})
	if ticks != 4 || lastOfKind(evs, "crafted").Kind != "crafted" || f.Items["choice_grain"] != 2 {
		t.Fatalf("guild recipe settlement wrong: ticks=%d items=%v evs=%+v", ticks, f.Items, evs)
	}
}

func TestHouseAssetLevelUpgradeRequiresCarpenter(t *testing.T) {
	w := sim.Village("house-upgrade")
	h := &w.Houses[13] // h14 Crossroads Watch Lodge: level 1, comfort 2, owner p26
	owner := actorByID(w, "p26")
	if owner == nil || h.OwnerID != owner.ID {
		t.Fatal("fixture owner mismatch")
	}
	owner.Location = "crossroads"
	owner.InsideHouse = h.ID
	owner.Energy = 80
	owner.Money = 20
	owner.Items = map[string]int{"food": 5, "plank": 3}
	owner.Professions = map[string]int{"peddler": 1, "carpenter": 1}

	if ev := w.Apply(owner.ID, sim.Action{Kind: "upgrade_house", HouseID: h.ID})[0]; ev.Kind != "action_failed" || !strings.Contains(ev.Payload["text"].(string), "carpenter") {
		t.Fatalf("asset upgrade allowed without carpenter level 2: %+v", ev)
	}
	owner.Professions["carpenter"] = 2
	evs, ticks := runActionAndFinish(w, owner.ID, sim.Action{Kind: "upgrade_house", HouseID: h.ID})
	if ticks != 4 || lastOfKind(evs, "house_upgraded").Kind != "house_upgraded" {
		t.Fatalf("house renovation did not settle: ticks=%d evs=%+v", ticks, evs)
	}
	if h.Level != 2 || h.Comfort != 3 || h.Condition != sim.MaxCondition || owner.Items["plank"] != 2 || owner.Money != 16 {
		t.Fatalf("asset upgrade effects wrong: level=%d comfort=%d cond=%d planks=%d money=%d", h.Level, h.Comfort, h.Condition, owner.Items["plank"], owner.Money)
	}
	// Level 3 is the asset cap.
	runActionAndFinish(w, owner.ID, sim.Action{Kind: "upgrade_house", HouseID: h.ID})
	if h.Level != 3 {
		t.Fatalf("second upgrade failed: level=%d", h.Level)
	}
	if ev := w.Apply(owner.ID, sim.Action{Kind: "upgrade_house", HouseID: h.ID})[0]; ev.Kind != "action_failed" {
		t.Fatalf("upgraded beyond asset level cap: %+v", ev)
	}
}

func TestItemTierDrivesExperienceAndGuildInfluence(t *testing.T) {
	w := sim.Village("xp")
	green, _ := w.Catalog.RecipeByID("farm_grain")
	plank, _ := w.Catalog.RecipeByID("saw_planks")
	tools, _ := w.Catalog.RecipeByID("forge_tools")
	if w.Catalog.CraftXP(green) != 4 || w.Catalog.CraftXP(plank) != 6 || w.Catalog.CraftXP(tools) != 12 {
		t.Fatalf("tier-scaled craft xp wrong: %d %d %d", w.Catalog.CraftXP(green), w.Catalog.CraftXP(plank), w.Catalog.CraftXP(tools))
	}

	// p19 blacksmith works the forge at home with 4 xp; +12 xp crosses the level-2 threshold (8).
	sm := actorByID(w, "p19")
	sm.Items = map[string]int{"food": 4, "ore": 1, "timber": 1}
	sm.Energy = 100
	sm.Professions = map[string]int{"blacksmith": 1}
	sm.ProfessionXP = map[string]int{"blacksmith": 4}
	beforeInfluence := factionByID(w, "artisans_guild").Influence
	evs, _ := runActionAndFinish(w, sm.ID, sim.Action{Kind: "craft", RecipeID: "forge_tools"})
	if lastOfKind(evs, "profession_leveled").Kind != "profession_leveled" || sm.Professions["blacksmith"] != 2 {
		t.Fatalf("tier-weighted xp did not level the profession: xp=%d professions=%v evs=%+v", sm.ProfessionXP["blacksmith"], sm.Professions, evs)
	}
	if factionByID(w, "artisans_guild").Influence <= beforeInfluence || sm.Level != 2 {
		t.Fatalf("guild influence or character level not refreshed: influence=%d level=%d", factionByID(w, "artisans_guild").Influence, sm.Level)
	}
}

func TestPeddlerMarginGrowsWithProfessionLevel(t *testing.T) {
	w := sim.Village("margin")
	p := &w.Actors[24] // p25 peddler
	p.Professions = map[string]int{"peddler": 1}
	if got := w.Catalog.SalePrice(*p, "food"); got != 2 {
		t.Fatalf("novice peddler margin wrong: %d", got)
	}
	p.Professions["peddler"] = 5
	if got := w.Catalog.SalePrice(*p, "food"); got != 4 {
		t.Fatalf("seasoned peddler margin wrong: %d", got)
	}
}

// lastOfKind returns the most recent settled event of the given kind (or a zero event).
func lastOfKind(events []sim.Event, kind string) sim.Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return events[i]
		}
	}
	return sim.Event{Kind: "missing_" + kind}
}

func actorByID(w *sim.World, id string) *sim.Actor {
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return &w.Actors[i]
		}
	}
	return nil
}

func factionByID(w *sim.World, id string) *sim.Faction {
	for i := range w.Factions {
		if w.Factions[i].ID == id {
			return &w.Factions[i]
		}
	}
	return nil
}

func TestPerceptionDoesNotExposeOthersInventory(t *testing.T) {
	w := sim.Village("test")
	p := w.View("p01")
	for _, n := range p.Nearby {
		if n.ID == "p03" {
			t.Fatal("villager in another house visible")
		}
	}
	p.Self.Trust["p02"] = 90
	p.Self.Items["food"] = 999
	if w.Actors[0].Trust["p02"] == 90 || w.Actors[0].Items["food"] == 999 {
		t.Fatal("view mutates world")
	}
}

type failing struct{}

func (failing) Decide(context.Context, sim.Perception) (sim.Action, error) {
	return sim.Action{}, errors.New("model unavailable")
}

func TestFailedDecisionDoesNotAdvance(t *testing.T) {
	w := sim.Village("test")
	b, _ := json.Marshal(w)
	_, err := w.Step(context.Background(), failing{})
	a, _ := json.Marshal(w)
	if err == nil || string(a) != string(b) {
		t.Fatal("step changed after decision failure")
	}
}

func TestVillageRunsSevenDays(t *testing.T) {
	w := sim.Village("seven-days")
	kinds := map[string]int{}
	recipes := map[string]int{}
	failReasons := map[string]int{}
	var last int64
	totalTicks := 7 * sim.TicksPerDay // 672 quarter-hour ticks = 7 full days
	for i := 0; i < totalTicks; i++ {
		events, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			if e.ID <= last || e.ActorID == "" || e.Action.Kind == "" {
				t.Fatal("event lacks action provenance")
			}
			last = e.ID
			kinds[e.Kind]++
			if e.Kind == "crafted" {
				recipes[e.Action.RecipeID]++
			}
			if e.Kind == "action_failed" {
				failReasons[e.Payload["text"].(string)]++
			}
		}
	}
	// Other actors may finish the same repair after this tick's decisions were made.
	delete(failReasons, "infrastructure is already at full condition")
	if len(failReasons) != 0 {
		t.Fatalf("unexpected action_failed events across 7 days: %v", failReasons)
	}
	if len(w.Actors) != 30 || w.Tick != int64(totalTicks) {
		t.Fatalf("wrong population or tick: actors=%d tick=%d", len(w.Actors), w.Tick)
	}
	for _, k := range []string{"left_house", "entered_house", "moved", "crafted", "speech", "trade_completed", "ate", "slept", "repaired_house", "growth_started", "faction_joined", "profession_joined"} {
		if kinds[k] == 0 {
			t.Fatalf("expected event kind %s across 7 days; got %v", k, kinds)
		}
	}
	for _, r := range []string{"farm_grain", "make_compost"} {
		if recipes[r] != 0 {
			t.Fatalf("long-cycle recipe %s produced before maturity", r)
		}
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}

	for _, a := range w.Actors {
		if a.Money < 0 || a.Energy < 0 || a.Energy > sim.MaxEnergy || a.Health < 0 || a.Health > sim.MaxHealth {
			t.Fatalf("actor %s out of bounds: money=%d energy=%d health=%d", a.ID, a.Money, a.Energy, a.Health)
		}
		if len(w.View(a.ID).Self.Memories) > sim.MaxRecalledMemories {
			t.Fatalf("actor %s memories exceeded cap %d: %d", a.ID, sim.MaxRecalledMemories, len(w.View(a.ID).Self.Memories))
		}
		for item, qty := range a.Items {
			if qty < 0 {
				t.Fatalf("actor %s negative item %s=%d", a.ID, item, qty)
			}
		}
	}
	for _, h := range w.Houses {
		if h.Condition < 0 || h.Condition > sim.MaxCondition {
			t.Fatalf("house %s condition out of bounds: %d", h.ID, h.Condition)
		}
	}
	t.Logf("events=%v recipes=%v", kinds, recipes)
}

func TestRuleTradeDependsOnTrust(t *testing.T) {
	w := sim.Village("trust")
	w.Actors[0].Items["food"] = 8
	w.Actors[1].Items["food"] = 2
	w.Apply("p01", sim.Action{Kind: "offer", TargetID: "p02", Item: "food", Quantity: 2, Price: 2})
	p := w.View("p02")
	a, _ := agent.Offline().Decide(context.Background(), p)
	p.Self.Trust["p01"] = -1
	b, _ := agent.Offline().Decide(context.Background(), p)
	if a.Kind != "accept" || b.Kind != "reject" {
		t.Fatalf("trust ignored: accept=%+v reject=%+v", a, b)
	}
}

type heardInstruction struct{}

func (heardInstruction) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	for _, m := range p.Self.Memories {
		if m.Kind == "speech" && m.Source == "p01" && m.Text == "Meet me at creek_well" {
			if p.Self.InsideHouse != "" {
				return sim.Action{Kind: "leave_house"}, nil
			}
			return sim.Action{Kind: "move", Location: "creek_well"}, nil
		}
	}
	return sim.Action{Kind: "wait"}, nil
}

func TestSpeechCanDriveLaterActionWithoutGlobalKnowledge(t *testing.T) {
	w := sim.Village("test")
	// Put p01, p02, p03 outdoors at west_cottages; p01 speaks only to p02.
	for i := 0; i < 3; i++ {
		w.Actors[i].InsideHouse = ""
		w.Actors[i].Location = "west_cottages"
	}
	w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "Meet me at creek_well"})
	if w.Actors[1].Memories[0].Kind != "speech" {
		t.Fatal("speech memory lost provenance type")
	}
	if _, err := w.Step(context.Background(), heardInstruction{}); err != nil {
		t.Fatal(err)
	}
	if w.Actors[1].Location != "creek_well" || w.Actors[2].Location != "west_cottages" {
		t.Fatalf("heard instruction not routed to listener only: p02=%s p03=%s", w.Actors[1].Location, w.Actors[2].Location)
	}
}

func TestMultiHopInformationChangesBehavior(t *testing.T) {
	w := sim.Village("chain")
	w.Actors[0].Professions["gatherer"] = 1
	w.Actors = w.Actors[:5]
	for i := range w.Actors {
		w.Actors[i].InsideHouse = ""
		w.Actors[i].Energy = 90
		w.Actors[i].Items = map[string]int{"food": 10, "seeds": 2, "water": 2}
		w.Actors[i].Location = "market_street"
	}
	w.Actors[0].Location = "south_fields"
	w.Actors[1].Location = "south_fields"
	w.Actors[2].Location = "creek_well"
	w.Actors[3].Location = "crossroads"
	w.Actors[3].Items = map[string]int{"food": 4}
	w.Actors[3].Role = "peddler"
	w.Actors[3].Tags = []string{"resident", "trader"} // non-gatherer/non-farmer normally heads to market_street
	w.Actors[3].Professions = map[string]int{"peddler": 1}
	w.Actors[3].Guilds = nil

	// A gathers existing wild food at south_fields (3 ticks).
	w.Tick = 4
	evs, _ := runActionAndFinish(w, "p01", sim.Action{Kind: "craft", RecipeID: "forage_food"})
	fact := lastOfKind(evs, "crafted") // settled at Tick 6
	observedFood := w.Actors[0].Items["food"]

	firstAction, _ := agent.Offline().Decide(context.Background(), w.View("p01"))
	if firstAction.Kind != "speak" || firstAction.TargetID != "p02" {
		t.Fatalf("observation not communicated: %+v", firstAction)
	}
	first := w.Apply("p01", firstAction)[0]

	// B walks road south_fields -> creek_well and relays news to C.
	w.Tick = 7
	w.Apply("p02", sim.Action{Kind: "move", Location: "creek_well"})
	w.Tick = 9
	relayAction, _ := agent.Offline().Decide(context.Background(), w.View("p02"))
	if relayAction.Kind != "speak" || relayAction.TargetID != "p03" || relayAction.BasisEventID != first.ID {
		t.Fatalf("baseline did not consume/relay heard statement: %+v", relayAction)
	}
	second := w.Apply("p02", relayAction)[0]

	// C walks road creek_well -> crossroads and intentionally exaggerates to D.
	w.Tick = 10
	w.Apply("p03", sim.Action{Kind: "move", Location: "crossroads"})
	w.Tick = 12
	third := w.Apply("p03", sim.Action{
		Kind:         "speak",
		TargetID:     "p04",
		BasisEventID: second.ID,
		Text:         "They harvested twenty grain at south_fields!",
		Statement:    &sim.Statement{Topic: "item_produced", Item: "food", Location: "south_fields", Quantity: 20},
	})[0]

	// D at crossroads acts on the rumor by taking the road hop toward south_fields (creek_well),
	// whereas without the rumor D walks toward market_street!
	w.Tick = 13
	decision, _ := agent.Offline().Decide(context.Background(), w.View("p04"))
	if decision.Kind != "move" || decision.Location != "creek_well" {
		t.Fatalf("heard news did not route actor along road toward south_fields: %+v", decision)
	}
	withoutNews := w.View("p04")
	withoutNews.Self.Memories = nil
	alternative, _ := agent.Offline().Decide(context.Background(), withoutNews)
	if alternative.Location == decision.Location {
		t.Fatalf("news did not change baseline destination: with=%+v without=%+v", decision, alternative)
	}
	result := w.Apply("p04", decision)[0]
	// System movement candidates never invent a causal basis, so the settled move carries none.
	if result.Kind != "moved" || result.Action.BasisEventID != 0 || w.Actors[3].Location != "creek_well" {
		t.Fatalf("causal action not settled: %+v", result)
	}

	chain := []sim.Event{first, second, third}
	parent := fact.ID
	for _, e := range chain {
		if e.Action.BasisEventID != parent || e.Payload["root_event_id"] != fact.ID {
			t.Fatalf("broken chain: %+v", e)
		}
		parent = e.ID
	}
	for i, want := range []struct {
		source string
		tick   int64
		parent int64
	}{{"p01", 6, fact.ID}, {"p02", 9, first.ID}, {"p03", 12, second.ID}} {
		a := w.Actors[i+1]
		var heard *sim.Memory
		for j := range a.Memories {
			m := &a.Memories[j]
			if m.Kind == "speech" && m.Source == want.source {
				heard = m
				break
			}
		}
		if heard == nil || heard.Tick != want.tick || heard.BasisEventID != want.parent || heard.RootEventID != fact.ID || heard.Observation != nil {
			t.Fatalf("wrong direct source/time/epistemic type: %+v", heard)
		}
	}
	if len(w.Actors[4].Memories) != 0 {
		t.Fatal("uninvolved remote villager learned news")
	}
	if w.Actors[0].Items["food"] != observedFood || fact.Payload["observation"].(sim.Observation).Quantity != 1 {
		t.Fatal("retelling changed original fact")
	}
	if first.Action.Statement.Quantity != 1 || third.Action.Statement.Quantity != 20 || third.Action.Text == first.Action.Text {
		t.Fatal("different statement contents lost")
	}
}

func TestCannotCiteUnknownMemoryOrSpeakRemotely(t *testing.T) {
	w := sim.Village("scope")
	w.Actors[0].Professions["gatherer"] = 1
	w.Actors[0].InsideHouse = ""
	w.Actors[0].Location = "south_fields"
	w.Actors[1].InsideHouse = ""
	w.Actors[1].Location = "creek_well"
	evs, _ := runActionAndFinish(w, "p01", sim.Action{Kind: "craft", RecipeID: "forage_food"})
	fact := lastOfKind(evs, "crafted")

	e := w.Apply("p01", sim.Action{Kind: "speak", TargetID: "p02", Text: "News", BasisEventID: fact.ID})[0]
	if e.Kind != "action_failed" || len(w.Actors[1].Memories) != 0 {
		t.Fatal("remote speech delivered")
	}
	e = w.Apply("p02", sim.Action{Kind: "move", Location: "south_fields", BasisEventID: fact.ID})[0]
	if e.Kind != "action_failed" || w.Actors[1].Location != "creek_well" {
		t.Fatal("unseen fact accepted as basis")
	}
}

func TestFarmerDiversifiedTasksAndCompostYield(t *testing.T) {
	w := sim.Village("farming-diversity")
	chosen := map[string]int{}
	for i := 0; i < 12; i++ {
		act, err := agent.Offline().Decide(context.Background(), w.View(w.Actors[i].ID))
		if err != nil {
			t.Fatal(err)
		}
		key := act.Kind
		if act.Kind == "craft" || act.Kind == "start_plan" {
			key = "craft:" + act.RecipeID
		} else if act.Kind == "repair_house" {
			key = "repair_house"
		}
		chosen[key]++
	}
	// At 06:00 AM, compost-needing farmers choose compost production (possibly
	// by starting its persistent plan), a leaky-shack farmer repairs the roof,
	// and others travel. These are initial intentions, not completed output.
	for _, want := range []string{"craft:make_compost", "repair_house", "leave_house"} {
		if chosen[want] == 0 {
			t.Fatalf("farmers lacked initial diversity; missing %s in %v", want, chosen)
		}
	}

	// The test world declares a short crop cycle; production still waits and consumes care inputs.
	for i := range w.Catalog.Recipes {
		if w.Catalog.Recipes[i].ID == "farm_grain" {
			w.Catalog.Recipes[i].Growth = &sim.GrowthRule{DurationTicks: 2, MaintenanceIntervalTicks: 1, Supplies: map[string]int{"water": 1}, Source: "two-tick test crop"}
		}
	}
	f := &w.Actors[1]
	f.InsideHouse = ""
	f.Location = "south_fields"
	if ev := w.Apply(f.ID, sim.Action{Kind: "craft", RecipeID: "farm_grain"})[0]; ev.Kind != "action_failed" {
		t.Fatal("farming accepted missing seeds")
	}
	beforeGrain := f.Items["food"]
	if ev := w.Apply(f.ID, sim.Action{Kind: "craft", RecipeID: "save_seeds"})[0]; ev.Kind != "crafted" || f.Items["seeds"] != 1 || f.Items["food"] != beforeGrain-1 {
		t.Fatal("seed selection did not consume stored grain")
	}
	f.Items["water"] = 3
	f.Energy = 90
	beforeFood := f.Items["food"]
	evs, ticks := runActionAndFinish(w, f.ID, sim.Action{Kind: "craft", RecipeID: "farm_grain"})
	if ticks != 4 || lastOfKind(evs, "growth_started").Kind == "" || f.Items["food"] != beforeFood {
		t.Fatal("planting bypassed maturation")
	}
	w.Tick++
	w.AdvanceGrowth()
	if w.Growth[0].Ready {
		t.Fatal("crop matured too early")
	}
	w.Tick++
	w.AdvanceGrowth()
	evs = w.Apply(f.ID, sim.Action{Kind: "harvest", TargetID: fmt.Sprint(w.Growth[0].ID)})
	if lastOfKind(evs, "harvested").Kind == "" || f.Items["food"] != beforeFood+2 || f.Items["water"] != 0 {
		t.Fatal("configured crop harvest or care consumption is wrong")
	}
}

func TestNearbyOrderDebiasedAcrossActors(t *testing.T) {
	w := sim.Village("debias")
	// Gather everyone onto crossroads square to test 30-way neighbor ordering de-biasing.
	for i := range w.Actors {
		w.Actors[i].InsideHouse = ""
		w.Actors[i].Location = "crossroads"
	}
	firstTargets := map[string]bool{}
	for _, a := range w.Actors {
		v1 := w.View(a.ID)
		v2 := w.View(a.ID)
		if len(v1.Nearby) == 0 {
			t.Fatal("expected nearby villagers at crossroads")
		}
		if v1.Nearby[0].ID != v2.Nearby[0].ID {
			t.Fatal("View must remain deterministic for the same actor and tick")
		}
		firstTargets[v1.Nearby[0].ID] = true
		v1.Nearby[0].Tags[0] = "mutated"
		if w.View(a.ID).Nearby[0].Tags[0] == "mutated" {
			t.Fatal("Neighbor.Tags aliased world state")
		}
	}
	if len(firstTargets) < 8 {
		t.Fatalf("expected diverse initial first neighbors across 30 actors, got %d (%v)", len(firstTargets), firstTargets)
	}
}

func TestCareerProgressionScholarToXiucaiAndBlacksmithRanks(t *testing.T) {
	w := sim.Village("career-progression")

	// 1. Scholar (读书人 -> 秀才) progression at scholars_guild (书院, crossroads)
	scholar := actorByID(w, "p29")
	if scholar == nil || scholar.Role != "scholar" {
		t.Fatalf("expected p29 to be seeded as scholar, got %+v", scholar)
	}
	scholar.InsideHouse = ""
	scholar.Location = "crossroads"
	scholar.Energy = 100
	scholar.Professions = map[string]int{"scholar": 1}
	scholar.ProfessionXP = nil
	scholar.Items["paper"] = 2
	scholar.Items["ink"] = 2
	w.Apply(scholar.ID, sim.Action{Kind: "wait"}) // refresh derived fields

	// At scholar level 1, cannot yet advance to xiucai (requires scholar level >= 2) or compile_classic.
	if ev := w.Apply(scholar.ID, sim.Action{Kind: "join_profession", ProfessionID: "xiucai"})[0]; ev.Kind != "action_failed" {
		t.Fatalf("expected xiucai advancement to require scholar level 2, got %+v", ev)
	}
	if ev := w.Apply(scholar.ID, sim.Action{Kind: "craft", RecipeID: "compile_classic"})[0]; ev.Kind != "action_failed" {
		t.Fatalf("expected compile_classic to require xiucai profession, got %+v", ev)
	}

	// Transcribe scrolls twice (+6 XP each -> 12 XP >= 8 threshold for level 2).
	runActionAndFinish(w, scholar.ID, sim.Action{Kind: "craft", RecipeID: "transcribe_scroll"})
	evs, _ := runActionAndFinish(w, scholar.ID, sim.Action{Kind: "craft", RecipeID: "transcribe_scroll"})
	if scholar.Professions["scholar"] != 2 || scholar.ProfessionRanks["scholar"] != "通经书生" {
		t.Fatalf("expected scholar level 2 (通经书生), got profs=%v ranks=%v evs=%+v", scholar.Professions, scholar.ProfessionRanks, evs)
	}

	// Now advance from 读书人 (scholar) to 秀才 (xiucai) at the crossroads academy guild hall!
	adv := w.Apply(scholar.ID, sim.Action{Kind: "join_profession", ProfessionID: "xiucai"})[0]
	if adv.Kind != "profession_joined" || scholar.Professions["xiucai"] != 1 || scholar.ProfessionRanks["xiucai"] != "新科秀才" {
		t.Fatalf("failed to advance from scholar to xiucai: ev=%+v profs=%v ranks=%v", adv, scholar.Professions, scholar.ProfessionRanks)
	}
	// Character level must equal sum of base + advanced profession levels (2 + 1 = 3).
	if scholar.Level != 3 {
		t.Fatalf("expected character level 3 (scholar 2 + xiucai 1), got %d", scholar.Level)
	}

	// Advancing to xiucai boosts transcribe_scroll yield (1 base + 1 xiucai BaseYieldBonus = 2) and unlocks compile_classic!
	scrollRecipe, _ := w.Catalog.RecipeByID("transcribe_scroll")
	if got := w.Catalog.RecipeYield(*scholar, scrollRecipe, false); got != 2 {
		t.Fatalf("expected xiucai advancement to boost scroll yield to 2, got %d", got)
	}
	scholar.Energy = 100
	compileEvs, _ := runActionAndFinish(w, scholar.ID, sim.Action{Kind: "craft", RecipeID: "compile_classic"})
	if lastOfKind(compileEvs, "crafted").Kind != "crafted" || scholar.Items["classic_tome"] < 1 {
		t.Fatalf("xiucai failed to compile classic_tome: items=%v evs=%+v", scholar.Items, compileEvs)
	}

	// 2. Blacksmith intra-profession stage titles (锻造学徒 -> 初级铁匠 -> 匠师) and Tier-2 master_smith advancement
	sm := actorByID(w, "p19")
	sm.Professions = map[string]int{"blacksmith": 1}
	sm.ProfessionXP = nil
	sm.Energy = 100
	sm.Items = map[string]int{"food": 6, "ore": 6, "timber": 6}
	blacksmithProf, _ := w.Catalog.ProfessionByID("blacksmith")
	if blacksmithProf.RankTitle(1) != "锻造学徒" || blacksmithProf.RankTitle(2) != "初级铁匠" || blacksmithProf.RankTitle(4) != "匠师" {
		t.Fatalf("blacksmith rank titles wrong: lv1=%s lv2=%s lv4=%s",
			blacksmithProf.RankTitle(1), blacksmithProf.RankTitle(2), blacksmithProf.RankTitle(4))
	}

	// Forge tools once (+12 XP -> crosses level 2 threshold 8 -> 初级铁匠).
	runActionAndFinish(w, sm.ID, sim.Action{Kind: "craft", RecipeID: "forge_tools"})
	if sm.Professions["blacksmith"] != 2 || sm.ProfessionRanks["blacksmith"] != "初级铁匠" {
		t.Fatalf("expected blacksmith level 2 (初级铁匠), got profs=%v ranks=%v", sm.Professions, sm.ProfessionRanks)
	}

	// Step outside onto east_lane and advance into Tier-2 master_smith (匠师).
	w.Apply(sm.ID, sim.Action{Kind: "leave_house"})
	msEv := w.Apply(sm.ID, sim.Action{Kind: "join_profession", ProfessionID: "master_smith"})[0]
	if msEv.Kind != "profession_joined" || sm.Professions["master_smith"] != 1 || sm.Level != 3 {
		t.Fatalf("master_smith advancement failed: ev=%+v profs=%v level=%d", msEv, sm.Professions, sm.Level)
	}
	forgeRecipe, _ := w.Catalog.RecipeByID("forge_tools")
	if got := w.Catalog.RecipeYield(*sm, forgeRecipe, false); got < 2 {
		t.Fatalf("expected master_smith to raise forge_tools yield to >=2, got %d", got)
	}
}

func TestSnapshotJSONRoundTripPreservesAllFields(t *testing.T) {
	w := sim.Village("roundtrip")
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var restored sim.World
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatal("fresh Village snapshot did not round-trip identically through JSON")
	}
}

func TestMillionPopulationSimulationAndHydration(t *testing.T) {
	w := sim.LargeWorld("million-prefecture", 1_000_000)
	if got := w.TotalPopulation(); got != 1_000_000 {
		t.Fatalf("expected initial total population 1,000,000, got %d", got)
	}
	if len(w.Actors) != 120 || len(w.Cohorts) == 0 {
		t.Fatalf("expected 120 active individual actors and macro cohorts, got actors=%d cohorts=%d", len(w.Actors), len(w.Cohorts))
	}

	// Verify 3,000,000 scale initialization as well
	w3m := sim.LargeWorld("three-million", 3_000_000)
	if got := w3m.TotalPopulation(); got != 3_000_000 {
		t.Fatalf("expected 3,000,000 population, got %d", got)
	}

	// Hydrate an individual from the Tier-2 xiucai cohort and verify population conservation
	hydrated, err := w.HydrateActor("cohort_crossroads_xiucai")
	if err != nil {
		t.Fatal(err)
	}
	hydratedID := hydrated.ID
	if hydrated.Professions["xiucai"] < 1 || hydrated.Professions["scholar"] < 2 || hydrated.Level < 3 {
		t.Fatalf("hydrated xiucai actor missing expected professions/level: %+v", hydrated)
	}
	if got := w.TotalPopulation(); got != 1_000_000 {
		t.Fatalf("hydration broke population conservation: %d", got)
	}

	// Dehydrate the actor back into its macro cohort and verify population conservation
	if err := w.DehydrateActor(hydratedID); err != nil {
		t.Fatal(err)
	}
	if got := w.TotalPopulation(); got != 1_000_000 {
		t.Fatalf("dehydration broke population conservation: %d", got)
	}

	// Run 1 full simulated day (96 ticks) across 1,000,000 total population
	kinds := map[string]int{}
	summary := sim.NewScaleSummary()
	for step := 0; step < sim.TicksPerDay; step++ {
		evs, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatal(err)
		}
		summary.RecordEvents(evs)
		for _, e := range evs {
			kinds[e.Kind]++
		}
	}
	if got, want := w.TotalPopulation()+w.TotalDeaths(), 1_000_000+w.TotalBirths(); got != want {
		t.Fatalf("expected living + deaths == 1,000,000 + births after 1 day, got living=%d deaths=%d births=%d sum=%d want=%d",
			w.TotalPopulation(), w.TotalDeaths(), w.TotalBirths(), got, want)
	}
	if w.TotalDeaths() <= 0 || w.TotalDeaths() > 500 {
		t.Fatalf("expected realistic non-zero daily multi-cause mortality (1..500), got %d (by_cause=%v)", w.TotalDeaths(), w.DeathsByCause)
	}
	for _, want := range []string{
		"crafted", "trade_completed", "cohort_crafted", "cohort_traded", "cohort_socialized", "cohort_leveled", "cohort_promoted", "faction_leveled",
		"cohort_sparred", "cohort_raided", "cohort_raid_repelled", "cohort_stolen", "cohort_turf_war", "cohort_arrested",
		"cohort_recuperated", "cohort_released",
	} {
		if kinds[want] == 0 {
			t.Fatalf("expected %s event during 1,000,000-population day; got %v", want, kinds)
		}
	}
	if summary.CraftBatches <= 0 || summary.ItemsProduced <= 0 || summary.TradeBatches <= 0 || summary.SocialInteractions <= 0 {
		t.Fatalf("expected production, trade and social activity under declared resource constraints, got %+v", summary)
	}
	// Conflicts must remain realistic (hundreds of daily incidents/bouts, NOT hundreds of thousands)
	if summary.CombatBatches <= 0 || summary.CombatBatches > 2_000 || summary.LootCoins > 5_000 {
		t.Fatalf("expected realistic bounded daily conflict/crime volume, got combat_batches=%d loot_coins=%d summary=%+v", summary.CombatBatches, summary.LootCoins, summary)
	}
	if summary.RaidIncidents == 0 || summary.TheftBatches == 0 || summary.TurfWarIncidents == 0 || summary.SparBatches == 0 || summary.ArrestBatches == 0 {
		t.Fatalf("expected non-zero raid/theft/turf/spar/arrest activity, got %+v", summary)
	}
	if summary.WoundedPeople == 0 || summary.HealedPeople == 0 || summary.JailedPeople == 0 || summary.ReleasedPeople == 0 || summary.Deaths == 0 {
		t.Fatalf("expected non-zero wounded, healed, jailed, released, and deaths in 1M day, got %+v", summary)
	}
	if gotW, wantW := summary.WoundedPeople, summary.HealedPeople+summary.WoundedDeaths+int64(w.TotalWounded()); gotW != wantW {
		t.Fatalf("wound ledger mismatch after 1M day: added=%d != healed(%d)+wounded_deaths(%d)+active(%d)=%d",
			gotW, summary.HealedPeople, summary.WoundedDeaths, w.TotalWounded(), wantW)
	}
	if gotS, wantS := summary.EpidemicCases, summary.CuredPeople+summary.DeathsByCause["disease"]+summary.SickDiedOther+int64(w.TotalSick()); gotS != wantS {
		t.Fatalf("epidemic ledger mismatch after 1M day: cases=%d != cured(%d)+disease_deaths(%d)+other(%d)+active(%d)=%d",
			gotS, summary.CuredPeople, summary.DeathsByCause["disease"], summary.SickDiedOther, w.TotalSick(), wantS)
	}
	if gotJ, wantJ := summary.JailedPeople, summary.ReleasedPeople+summary.JailedDeaths+int64(w.TotalJailed()); gotJ != wantJ {
		t.Fatalf("jail ledger mismatch after 1M day: jailed=%d != released(%d)+jailed_deaths(%d)+active(%d)=%d",
			gotJ, summary.ReleasedPeople, summary.JailedDeaths, w.TotalJailed(), wantJ)
	}

	// Verify JSON snapshot round-trip on the 1,000,000-population world
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var restored sim.World
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatal("1,000,000-population world did not round-trip identically through JSON")
	}
	t.Logf("1M population 24h (96 ticks) living=%d wounded=%d jailed=%d dead=%d craft_batches=%d items_produced=%d trade_batches=%d items_traded=%d coins_traded=%d incidents(raid=%d,turf=%d,theft=%d,arrest=%d,spar=%d) casualties(wounded=%d,healed=%d,jailed=%d,released=%d,deaths=%d,med=%d) loot_coins=%d bounty_coins=%d",
		w.TotalPopulation(), w.TotalWounded(), w.TotalJailed(), w.TotalDeaths(),
		summary.CraftBatches, summary.ItemsProduced, summary.TradeBatches, summary.ItemsTraded, summary.CoinsTraded,
		summary.RaidIncidents, summary.TurfWarIncidents, summary.TheftBatches, summary.ArrestBatches, summary.SparBatches,
		summary.WoundedPeople, summary.HealedPeople, summary.JailedPeople, summary.ReleasedPeople, summary.Deaths, summary.MedicineUsed,
		summary.LootCoins, summary.BountyCoins)
}

func TestMartialHallYamenBanditThiefGangInteractions(t *testing.T) {
	w := sim.LargeWorld("conflict-test", 1000)

	// 1. Hydrate a martial artist (武师), constable (捕快), bandit (山贼), thief (小偷), and gangster (帮众)
	martial, err := w.HydrateActor("cohort_east_martial_artists")
	if err != nil {
		t.Fatal(err)
	}
	constable, err := w.HydrateActor("cohort_crossroads_constables")
	if err != nil {
		t.Fatal(err)
	}
	bandit, err := w.HydrateActor("cohort_woods_bandits")
	if err != nil {
		t.Fatal(err)
	}
	thief, err := w.HydrateActor("cohort_market_thieves")
	if err != nil {
		t.Fatal(err)
	}
	gangster, err := w.HydrateActor("cohort_market_gangsters")
	if err != nil {
		t.Fatal(err)
	}

	martialID, constableID, banditID, thiefID, gangsterID := martial.ID, constable.ID, bandit.ID, thief.ID, gangster.ID

	// Co-locate everyone at market_street outdoors with a merchant target (p27)
	for _, id := range []string{martialID, constableID, banditID, thiefID, gangsterID, "p27"} {
		a := actorByID(w, id)
		a.Location = "market_street"
		a.InsideHouse = ""
		a.Energy = 100
		a.Health = 100
	}
	merchant := actorByID(w, "p27")
	merchant.Money = 40
	merchant.Items["choice_grain"] = 4
	merchant.Items["medicine"] = 2

	// 2. 武馆演武切磋 (Sparring between martial artist and yamen constable + recuperation cooldown)
	sparEv := w.Apply(martialID, sim.Action{Kind: "spar", TargetID: constableID})[0]
	if sparEv.Kind != "sparred" {
		t.Fatalf("expected sparred event between martial artist and constable, got %+v", sparEv)
	}
	if actorByID(w, martialID).Trust[constableID] <= 0 || actorByID(w, constableID).Trust[martialID] <= 0 {
		t.Fatal("sparring should build mutual trust between fighters")
	}
	// Immediate repeat sparring must be blocked while recuperating!
	if again := w.Apply(martialID, sim.Action{Kind: "spar", TargetID: constableID})[0]; again.Kind != "action_failed" {
		t.Fatalf("expected immediate repeat spar during recuperation to fail, got %+v", again)
	}

	// 3. 小偷顺手牵羊 (Thief pickpocketing merchant)
	beforeThiefCoins := actorByID(w, thiefID).Money
	stealEv := w.Apply(thiefID, sim.Action{Kind: "steal", TargetID: "p27"})[0]
	if stealEv.Kind != "stolen" || actorByID(w, thiefID).Money <= beforeThiefCoins {
		t.Fatalf("expected thief to pickpocket coins/items from merchant, got %+v", stealEv)
	}

	// 4. 山贼与帮派劫掠攻伐 + 伤后服药休养 (Raids, Recuperation Cooldown & Medicate)
	beforeBanditCoins := actorByID(w, banditID).Money
	raidEv := w.Apply(banditID, sim.Action{Kind: "raid", TargetID: "p27"})[0]
	if raidEv.Kind != "raided" || actorByID(w, banditID).Money <= beforeBanditCoins {
		t.Fatalf("expected bandit raid against merchant to succeed, got %+v", raidEv)
	}
	// Bandit cannot launch back-to-back raids without recuperating first!
	if immediateRaid := w.Apply(banditID, sim.Action{Kind: "raid", TargetID: "p27"})[0]; immediateRaid.Kind != "action_failed" {
		t.Fatalf("expected back-to-back raid without recuperation to fail, got %+v", immediateRaid)
	}
	gangRaidEv := w.Apply(gangsterID, sim.Action{Kind: "raid", TargetID: "p27"})[0]
	if gangRaidEv.Kind != "raided" {
		t.Fatalf("expected gangster extortion raid against merchant to succeed, got %+v", gangRaidEv)
	}

	// Injured merchant treats own wounds using medicate!
	injuredHP := actorByID(w, "p27").Health
	actorByID(w, "p27").Items["medicine"] = 1
	medEv := w.Apply("p27", sim.Action{Kind: "medicate", Item: "medicine"})[0]
	if medEv.Kind != "medicated" || actorByID(w, "p27").Health <= injuredHP {
		t.Fatalf("expected injured merchant to medicate and recover health, got %+v hp=%d->%d", medEv, injuredHP, actorByID(w, "p27").Health)
	}

	// Advance time past recuperation cooldown and promote martial artist to Tier-2 武学宗师 (martial_master)
	w.Tick += 96
	ma := actorByID(w, martialID)
	ma.Location = "east_lane"
	ma.Attributes = sim.Attributes{Strength: 13, Dexterity: 12, Insight: 10, Spirit: 10}
	advMa := w.Apply(martialID, sim.Action{Kind: "join_profession", ProfessionID: "martial_master"})[0]
	if advMa.Kind != "profession_joined" || ma.Professions["martial_master"] < 1 {
		t.Fatalf("expected martial_artist to advance to martial_master, got %+v", advMa)
	}
	ma.Professions["martial_master"] = 4
	ma.Location = "market_street"

	// Bandit trying to raid the Tier-2 Martial Master gets repelled and wounded!
	actorByID(w, banditID).Energy = 100
	actorByID(w, banditID).Health = 90
	repelEv := w.Apply(banditID, sim.Action{Kind: "raid", TargetID: martialID})[0]
	if repelEv.Kind != "raid_repelled" || actorByID(w, banditID).Health >= 90 {
		t.Fatalf("expected weaker bandit raid against martial master to be repelled with injury, got %+v", repelEv)
	}

	// 5. 府衙捕快签发海捕公文、缉拿盗匪入狱与致死判定 (Yamen Constable Arrests, Imprisonment & Lethal Clash)
	con := actorByID(w, constableID)
	con.Location = "crossroads"
	con.Attributes = sim.Attributes{Strength: 12, Dexterity: 11, Insight: 11, Spirit: 10}
	advCon := w.Apply(constableID, sim.Action{Kind: "join_profession", ProfessionID: "chief_constable"})[0]
	if advCon.Kind != "profession_joined" || con.Professions["chief_constable"] < 1 {
		t.Fatalf("expected constable to advance to chief_constable, got %+v", advCon)
	}
	con.Energy = 100
	con.Items["paper"] = 1
	con.Items["ink"] = 1
	writEvs, _ := runActionAndFinish(w, constableID, sim.Action{Kind: "craft", RecipeID: "issue_bounty_writ"})
	if lastOfKind(writEvs, "crafted").Kind != "crafted" || con.Items["bounty_writ"] < 1 {
		t.Fatalf("expected constable to issue bounty_writ at yamen, got %+v", writEvs)
	}
	con.Location = "market_street"
	con.Energy = 100
	con.Health = 100
	con.RestUntil = 0
	beforeBounty := con.Money
	arrestEv := w.Apply(constableID, sim.Action{Kind: "arrest", TargetID: banditID})[0]
	if arrestEv.Kind != "arrested" || actorByID(w, constableID).Money <= beforeBounty || actorByID(w, banditID).JailedUntil <= w.Tick {
		t.Fatalf("expected chief constable to arrest and imprison bandit, got %+v jailedUntil=%d", arrestEv, actorByID(w, banditID).JailedUntil)
	}
	// Imprisoned bandit cannot act while serving sentence in Yamen jail!
	actorByID(w, banditID).Energy = 100
	if jailedTry := w.Apply(banditID, sim.Action{Kind: "steal", TargetID: "p27"})[0]; jailedTry.Kind != "action_failed" {
		t.Fatalf("expected imprisoned bandit action to fail, got %+v", jailedTry)
	}
	// Cannot arrest a law-abiding resident
	actorByID(w, constableID).Energy = 100
	actorByID(w, constableID).RestUntil = 0
	failArrest := w.Apply(constableID, sim.Action{Kind: "arrest", TargetID: "p27"})[0]
	if failArrest.Kind != "action_failed" {
		t.Fatalf("expected arresting law-abiding merchant to fail, got %+v", failArrest)
	}

	// 6. Lethal combat causes real death (Dead == true, actor_died, TotalDeaths increments)
	beforeLiving := w.TotalPopulation()
	g := actorByID(w, gangsterID)
	g.Health = 12
	ma.Energy = 100
	ma.Health = 100
	killEvs := w.Apply(martialID, sim.Action{Kind: "attack", TargetID: gangsterID})
	if !actorByID(w, gangsterID).Dead || lastOfKind(killEvs, "actor_died").Kind != "actor_died" {
		t.Fatalf("expected Critically wounded gangster to die from lethal attack, got dead=%v evs=%+v", actorByID(w, gangsterID).Dead, killEvs)
	}
	if w.TotalPopulation() != beforeLiving-1 || w.TotalDeaths() < 1 {
		t.Fatalf("expected living population to drop by 1 and deaths >= 1, got pop=%d->%d deaths=%d", beforeLiving, w.TotalPopulation(), w.TotalDeaths())
	}
}

func TestHighDensitySpatialIndexAttentionCap(t *testing.T) {
	w := sim.LargeWorld("high-density", 5000)
	// Hydrate 60 extra actors at crossroads so >60 actors are co-located in the same district
	for i := 0; i < 60; i++ {
		a, err := w.HydrateActor("cohort_crossroads_scholars")
		if err != nil {
			t.Fatal(err)
		}
		a.Location = "crossroads"
		a.InsideHouse = ""
	}
	var mu sync.Mutex
	var seenMax int
	probe := deciderFunc(func(_ context.Context, p sim.Perception) (sim.Action, error) {
		mu.Lock()
		if len(p.Nearby) > seenMax {
			seenMax = len(p.Nearby)
		}
		mu.Unlock()
		return sim.Action{Kind: "wait"}, nil
	})
	if _, err := w.Step(context.Background(), probe); err != nil {
		t.Fatal(err)
	}
	if seenMax != sim.MaxNearbyView {
		t.Fatalf("expected high-density district Nearby to be capped at %d, got %d", sim.MaxNearbyView, seenMax)
	}
}

type deciderFunc func(context.Context, sim.Perception) (sim.Action, error)

func (f deciderFunc) Decide(ctx context.Context, p sim.Perception) (sim.Action, error) {
	return f(ctx, p)
}

func TestWorldEvolutionFullYearEcologyLogisticsAndLifecycle(t *testing.T) {
	const initialPop = 50_000
	w := sim.LargeWorld("full-year-test", initialPop)
	for i := range w.Catalog.Calendar.Seasons {
		w.Catalog.Calendar.Seasons[i].Days = 7
	}
	for i := range w.Catalog.Recipes {
		if g := w.Catalog.Recipes[i].Growth; g != nil {
			g.DurationTicks = sim.TicksPerDay
			g.MaintenanceIntervalTicks = sim.TicksPerDay
			g.Source = "one-day biological cycle in accelerated integration fixture"
		}
	}

	initialCoins := w.TotalCoins()
	summary := sim.NewScaleSummary()
	seenSeasons := map[string]bool{}
	seenWeathers := map[string]bool{}

	// Force one active focal actor to critically low health + severe illness early so we deterministically verify AutoHydrateSuccessor too!
	w.Actors[0].DiseaseID = "dysentery"
	w.Actors[0].DiseaseTicks = 10 * sim.TicksPerDay
	w.Actors[0].Health = 4
	w.Actors[0].Items = map[string]int{"food": 4}

	totalTicks := 28 * sim.TicksPerDay // 1 complete 4-season year (2,688 ticks) -> rolls into Year 2 Spring!
	for step := 0; step < totalTicks; step++ {
		evs, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		summary.RecordEvents(evs)
		cal := sim.CalendarAt(w.Tick, w.Catalog.Calendar)
		seenSeasons[cal.SeasonID] = true
		for _, ds := range w.DistrictStates {
			if ds.Weather != "" {
				seenWeathers[ds.Weather] = true
			}
		}
		if got, want := w.TotalPopulation()+w.TotalDeaths(), initialPop+w.TotalBirths(); got != want {
			t.Fatalf("demographic conservation violated at tick %d: living=%d + deaths=%d (%d) != initial(%d) + births=%d (%d)",
				w.Tick, w.TotalPopulation(), w.TotalDeaths(), got, initialPop, w.TotalBirths(), want)
		}
	}

	// 1. Calendar & Weather verification
	for _, s := range []string{"spring", "summer", "autumn", "winter"} {
		if !seenSeasons[s] {
			t.Fatalf("expected full year to visit season %s, visited %v", s, seenSeasons)
		}
	}
	if endCal := sim.CalendarAt(w.Tick, w.Catalog.Calendar); endCal.Year != 2 {
		t.Fatalf("expected calendar to advance to Year 2 after 28 days, got %+v", endCal)
	}
	if len(seenWeathers) < 3 {
		t.Fatalf("expected diverse regional weather transitions across 4 seasons, saw %v", seenWeathers)
	}

	// 2. Multi-medium freight & Grand Canal (漕运) verification
	if summary.WaterwayBatches == 0 || summary.WaterwayItems == 0 || summary.LandFreightBatches == 0 || summary.FreightFees == 0 {
		t.Fatalf("expected active waterway (漕运) and land freight with carrier fees, got waterway_batches=%d waterway_items=%d land_batches=%d fees=%d",
			summary.WaterwayBatches, summary.WaterwayItems, summary.LandFreightBatches, summary.FreightFees)
	}

	// 3. Full primary supply chains (农牧渔林矿猎祠) & ecological resource pool regeneration
	for _, item := range []string{"food", "livestock", "meat", "fish", "timber", "ore", "hide", "winter_coat", "sacrificial_offering", "compost", "medicine"} {
		if summary.ProducedByItem[item] == 0 {
			t.Fatalf("expected supply chain item %s to be produced over full year; produced=%v", item, summary.ProducedByItem)
		}
	}
	for dID, ds := range w.DistrictStates {
		for poolID, maxCap := range ds.MaxResources {
			if maxCap > 0 && (ds.Resources[poolID] <= 0 || ds.Resources[poolID] > maxCap) {
				t.Fatalf("district %s pool %s out of ecological carrying bounds: %d / %d", dID, poolID, ds.Resources[poolID], maxCap)
			}
		}
	}

	// 4. Disasters, public infrastructure maintenance, wild beasts/anomalies, and shrine rituals
	if summary.Disasters == 0 || summary.InfraRepairs == 0 {
		t.Fatalf("expected regional disasters and public infrastructure repairs over full year, got disasters=%d infra_repairs=%d", summary.Disasters, summary.InfraRepairs)
	}
	if summary.WildEncounters+summary.ThreatSurges == 0 || summary.ThreatHunts == 0 {
		t.Fatalf("expected wild beast/anomaly activity and hunter/martial suppression, got encounters=%d surges=%d hunts=%d",
			summary.WildEncounters, summary.ThreatSurges, summary.ThreatHunts)
	}
	if summary.Rituals == 0 {
		t.Fatalf("expected seasonal/crisis shrine rituals over full year, got 0")
	}

	// 5. Physiology, sanitation nightsoil-to-compost loop, epidemics & cures, births, multi-cause mortality & succession
	if summary.CompostRecycled == 0 {
		t.Fatalf("expected sanitation nightsoil recycling into compost, got 0")
	}
	if summary.EpidemicCases == 0 || summary.CuredPeople == 0 || w.DeathsByCause["disease"] == 0 {
		t.Fatalf("expected contagious/cold illness cases, medical cures, and non-zero disease deaths over full year, got cases=%d cured=%d deaths_by_cause=%v", summary.EpidemicCases, summary.CuredPeople, w.DeathsByCause)
	}
	if w.TotalBirths() == 0 || w.DeathsByCause["old_age"] == 0 || len(w.DeathsByCause) < 4 {
		t.Fatalf("expected births, elder mortality, and >=4 distinct death causes over full year; births=%d deaths_by_cause=%v", w.TotalBirths(), w.DeathsByCause)
	}
	if summary.Successions == 0 || summary.Successions > int64(summary.EventCounts["actor_died"]) {
		t.Fatalf("expected successor count to be positive and no greater than focal deaths, got %d (deaths_by_cause=%v)", summary.Successions, w.DeathsByCause)
	}
	if gotW, wantW := summary.WoundedPeople, summary.HealedPeople+summary.WoundedDeaths+int64(w.TotalWounded()); gotW != wantW {
		t.Fatalf("full-year wound ledger mismatch: added=%d != healed(%d)+wounded_deaths(%d)+active(%d)=%d",
			gotW, summary.HealedPeople, summary.WoundedDeaths, w.TotalWounded(), wantW)
	}
	// Note: w.Actors[0] was seeded with DiseaseID="dysentery" before Step 0 (initialSick = 1)
	if gotS, wantS := int64(1)+summary.EpidemicCases, summary.CuredPeople+summary.DeathsByCause["disease"]+summary.SickDiedOther+int64(w.TotalSick()); gotS != wantS {
		t.Fatalf("full-year epidemic ledger mismatch: initial(1)+cases(%d)=%d != cured(%d)+disease_deaths(%d)+other(%d)+active(%d)=%d",
			summary.EpidemicCases, gotS, summary.CuredPeople, summary.DeathsByCause["disease"], summary.SickDiedOther, w.TotalSick(), wantS)
	}
	if gotJ, wantJ := summary.JailedPeople, summary.ReleasedPeople+summary.JailedDeaths+int64(w.TotalJailed()); gotJ != wantJ {
		t.Fatalf("full-year jail ledger mismatch: jailed=%d != released(%d)+jailed_deaths(%d)+active(%d)=%d",
			gotJ, summary.ReleasedPeople, summary.JailedDeaths, w.TotalJailed(), wantJ)
	}
	if gotCoins := w.TotalCoins(); gotCoins != initialCoins {
		t.Fatalf("full-year M0 coin conservation mismatch: ending=%d != initial=%d (diff=%d)", gotCoins, initialCoins, gotCoins-initialCoins)
	}
	tl := w.TreasuryLedger
	if gotGross, wantGross := tl.TaxAssessedGross, tl.TaxToTreasury+tl.TaxSkimmed; gotGross != wantGross {
		t.Fatalf("full-year tax assessment split mismatch: gross=%d != net(%d)+skimmed(%d)=%d", gotGross, tl.TaxToTreasury, tl.TaxSkimmed, wantGross)
	}
	wantTreasury := tl.InitialTreasury + tl.TaxToTreasury + tl.StateFeesIn + tl.ConfiscatedIn - tl.PayrollPaid - tl.ReliefAndInfraOut - tl.BountiesOut - tl.TreasuryEmbezzled
	if w.PublicTreasury != wantTreasury {
		t.Fatalf("full-year treasury ledger mismatch: public_treasury=%d != expected=%d (%+v)", w.PublicTreasury, wantTreasury, tl)
	}

	t.Logf("Full-year 50k summary: pop=%d births=%d deaths=%d(%v) successions=%d waterway_items=%d freight_fees=%d disasters=%d infra_repairs=%d hunts=%d rituals=%d compost=%d epidemics=%d cured=%d",
		w.TotalPopulation(), w.TotalBirths(), w.TotalDeaths(), w.DeathsByCause, summary.Successions,
		summary.WaterwayItems, summary.FreightFees, summary.Disasters, summary.InfraRepairs,
		summary.ThreatHunts, summary.Rituals, summary.CompostRecycled, summary.EpidemicCases, summary.CuredPeople)
}

func TestGenreDecoupledSciFiColonyPackWithoutCodeChanges(t *testing.T) {
	// Build a 100% Sci-Fi Orbital Colony simulation pack purely through Catalog + WorldMap data structures,
	// proving zero hardcoded historical setting dependencies in the simulation engine!
	sciFiMap := sim.WorldMap{
		Districts: []sim.District{
			{
				ID: "orbital_hub", Name: "天穹核心环", Terrain: "station_core",
				Features:           []string{"market", "water_source", "reactor_shrine", "residential"},
				ResourceCapacities: map[string]int64{"coolant_reservoir": 5000},
			},
			{
				ID: "hydro_dome", Name: "生态水培穹顶", Terrain: "bio_sector",
				Features:           []string{"hydro_bay", "water_source"},
				ResourceCapacities: map[string]int64{"biomass_vat": 8000, "coolant_reservoir": 4000},
			},
			{
				ID: "ice_asteroid", Name: "碎冰矿星带", Terrain: "deep_space",
				Features:           []string{"mining_rig"},
				ResourceCapacities: map[string]int64{"asteroid_vein": 8000},
			},
		},
		Roads: []sim.Road{
			{From: "orbital_hub", To: "hydro_dome", Ticks: 1, EnergyCost: 2, Medium: "shuttle_corridor", FreightCapacity: 400, FreightFeePermille: 80, BaseLossPermille: 10, CarrierTag: "shuttle_pilot"},
			{From: "orbital_hub", To: "ice_asteroid", Ticks: 2, EnergyCost: 3, Medium: "shuttle_corridor", FreightCapacity: 400, FreightFeePermille: 100, BaseLossPermille: 20, CarrierTag: "shuttle_pilot"},
		},
	}

	sciFiCat := sim.Catalog{
		TagActions: map[string][]string{
			"colonist": {"wait", "move", "enter_house", "leave_house", "sleep", "eat", "drink", "relieve", "medicate", "speak", "maintain_infra", "join_faction", "join_profession"},
			"trader":   {"offer", "accept", "reject"},
			"ranger":   {"attack", "hunt_threat"},
			"medic":    {"heal"},
			"psion":    {"perform_ritual"},
		},
		LevelThresholds:        []int{0, 8, 20, 40},
		FactionLevelThresholds: []int{0, 30, 90},
		Calendar: sim.CalendarSpec{
			Seasons: []sim.SeasonSpec{
				{ID: "perihelion", Days: 3, Name: "近日季", WeatherWeights: map[string]int{"nominal": 60, "solar_flare": 40}, RouteCapacityPct: map[string]int{"shuttle_corridor": 100}},
				{ID: "aphelion", Days: 3, Name: "远日寒季", ColdExposure: 2, WeatherWeights: map[string]int{"nominal": 50, "cryo_drift": 50}, RouteCapacityPct: map[string]int{"shuttle_corridor": 80}},
			},
		},
		Weathers: map[string]sim.WeatherSpec{
			"nominal":     {ID: "nominal", Name: "轨道平稳"},
			"solar_flare": {ID: "solar_flare", Name: "太阳耀斑风暴", TravelEnergyPenalty: 2, RouteCapacityPct: map[string]int{"shuttle_corridor": 70}, AccidentMultiplierPct: 140},
			"cryo_drift":  {ID: "cryo_drift", Name: "深空极寒流", ColdDamage: 4, TravelTickPenalty: 1},
		},
		Resources: map[string]sim.ResourcePoolSpec{
			"coolant_reservoir": {ID: "coolant_reservoir", Name: "循环净水池", PotableWater: true, RegenPermillePerDay: 80, DepletionFloorPct: 10},
			"biomass_vat":       {ID: "biomass_vat", Name: "藻类生物质槽", NodeFeatures: []string{"hydro_bay"}, RegenPermillePerDay: 70, DepletionFloorPct: 15},
			"asteroid_vein":     {ID: "asteroid_vein", Name: "小行星钛矿脉", NodeFeatures: []string{"mining_rig"}, RegenPermillePerDay: 25, DepletionFloorPct: 10},
		},
		Items: map[string]sim.ItemSpec{
			"synth_ration":  {ID: "synth_ration", Tier: 1, BatchPrice: 2, Edible: true, HungerRelief: 2, Tradeable: true, BatchQty: 2, OfferThreshold: 6, ShortageThreshold: 2, SurplusThreshold: 6},
			"coolant_water": {ID: "coolant_water", Tier: 1, BatchPrice: 1, Potable: true, ThirstRelief: 2, Tradeable: true, BatchQty: 2, OfferThreshold: 6},
			"alloy_ore":     {ID: "alloy_ore", Tier: 1, BatchPrice: 3, Tradeable: true, BatchQty: 2, OfferThreshold: 4, ShortageThreshold: 2, SurplusThreshold: 6},
			"organic_waste": {ID: "organic_waste", Tier: 1},
			"bio_nutrient":  {ID: "bio_nutrient", Tier: 1, BatchPrice: 2, Tradeable: true, BatchQty: 2, OfferThreshold: 4},
			"nano_med":      {ID: "nano_med", Tier: 2, BatchPrice: 5, MedicinalHeal: 30, DiseaseCure: 2, Tradeable: true, BatchQty: 1, OfferThreshold: 3},
			"thermal_suit":  {ID: "thermal_suit", Tier: 2, BatchPrice: 6, WarmthBonus: 2, Tradeable: true, BatchQty: 1, OfferThreshold: 3},
			"void_crystal":  {ID: "void_crystal", Tier: 3, BatchPrice: 10, Tradeable: true, BatchQty: 1, OfferThreshold: 2},
			"core_cell":     {ID: "core_cell", Tier: 2, BatchPrice: 4, Tradeable: true, BatchQty: 1, OfferThreshold: 3},
		},
		Professions: []sim.Profession{
			{ID: "hydro_tech", Name: "水培技师", Tier: 1, Guild: "bio_corps", Grants: []string{"colonist", "trader", "bio_worker"}, Ranks: []string{"见习技师", "高级技师"}},
			{ID: "void_miner", Name: "深空矿工", Tier: 1, Guild: "mining_syndicate", Grants: []string{"colonist", "trader", "miner_worker"}, Ranks: []string{"采矿员", "深空钻探师"}},
			{ID: "shuttle_captain", Name: "穿梭舰长", Tier: 1, Guild: "transit_fleet", Grants: []string{"colonist", "trader", "shuttle_pilot"}, Ranks: []string{"副驾驶", "轨道舰长"}},
			{ID: "exo_ranger", Name: "外骨骼游骑兵", Tier: 1, Guild: "defense_wing", CombatBonusPerLevel: 3, Grants: []string{"colonist", "trader", "ranger"}, Ranks: []string{"巡逻兵", "星环游骑兵"}},
			{ID: "harmonic_psion", Name: "反应堆谐律官", Tier: 1, Guild: "core_order", Grants: []string{"colonist", "trader", "psion", "medic"}, Ranks: []string{"谐律学徒", "反应堆司祭"}},
		},
		Recipes: []sim.Recipe{
			{ID: "recycle_waste", Profession: "hydro_tech", Inputs: map[string]int{"organic_waste": 3}, OutputItem: "bio_nutrient", OutputQty: 1, Ticks: 2, Growth: &sim.GrowthRule{DurationTicks: 96, MaintenanceIntervalTicks: 96, ResourcePool: "coolant_reservoir", ResourceDraw: 1, Source: "test colony one-day recycling process"}},
			{ID: "cultivate_ration", Label: "Cultivate synth rations", Profession: "hydro_tech", NodeFeature: "hydro_bay", ResourcePool: "biomass_vat", ResourceDraw: 2, BonusInput: "bio_nutrient", BonusOutput: 2, OutputItem: "synth_ration", OutputQty: 4, EnergyCost: 8, Ticks: 3},
			{ID: "extract_alloy", Label: "Extract asteroid alloy", Profession: "void_miner", NodeFeature: "mining_rig", ResourcePool: "asteroid_vein", ResourceDraw: 2, OutputItem: "alloy_ore", OutputQty: 3, EnergyCost: 10, Ticks: 3},
			{ID: "charge_core_cell", Label: "Charge harmonic core cell", Profession: "harmonic_psion", Inputs: map[string]int{"alloy_ore": 1}, OutputItem: "core_cell", OutputQty: 2, EnergyCost: 8, Ticks: 2},
			{ID: "synthesize_nanomed", Label: "Synthesize nano-medicine", Profession: "harmonic_psion", Inputs: map[string]int{"alloy_ore": 1}, OutputItem: "nano_med", OutputQty: 2, EnergyCost: 8, Ticks: 2},
		},
		TagProfiles: map[string]sim.TagProfile{
			"colonist":     {TargetStock: map[string]int{"synth_ration": 8, "nano_med": 1}, Consumes: []string{"synth_ration", "nano_med"}},
			"bio_worker":   {Produces: []string{"synth_ration"}, Consumes: []string{"bio_nutrient", "organic_waste"}, TargetStock: map[string]int{"synth_ration": 16, "bio_nutrient": 4}},
			"miner_worker": {Produces: []string{"alloy_ore"}, TargetStock: map[string]int{"alloy_ore": 12, "synth_ration": 8}},
			"psion":        {Produces: []string{"core_cell", "nano_med"}, Consumes: []string{"alloy_ore"}, TargetStock: map[string]int{"core_cell": 4, "nano_med": 4, "alloy_ore": 4}, PublicTrustDuty: true},
			"ranger":       {CoercivePower: true, TargetStock: map[string]int{"synth_ration": 8, "nano_med": 2}},
		},
		Infrastructures: map[string]sim.InfrastructureSpec{
			"shield_grid":  {ID: "shield_grid", Name: "磁暴偏导护盾", NodeFeatures: []string{"market", "hydro_bay", "mining_rig"}, MaxCondition: 100, DecayPerDay: 8, BuildInputs: map[string]int{"alloy_ore": 1}, BuildEnergy: 8},
			"recycler_bay": {ID: "recycler_bay", Name: "有机废料再生舱", SanitationRole: true, NodeFeatures: []string{"market", "hydro_bay", "mining_rig"}, MaxCondition: 100, DecayPerDay: 4, BuildInputs: map[string]int{"alloy_ore": 1}, BuildEnergy: 6},
		},
		Disasters: []sim.DisasterSpec{
			{ID: "hull_microbreach", Name: "微陨石船壳渗漏", TriggerWeathers: []string{"solar_flare"}, TargetFeatures: []string{"mining_rig", "hydro_bay"}, ProbPermillePerDay: 450, DurationTicks: 48, CasualtyPermille: 4, MitigatedByInfra: "shield_grid", ReliefItem: "synth_ration"},
		},
		Encounters: []sim.EncounterSpec{
			{ID: "void_parasite", Name: "虚空噬能体", Category: "anomaly", NodeFeatures: []string{"mining_rig"}, BaseActivity: 18, GrowthPerDay: 8, SurgeThreshold: 30, HazardPermille: 40, Damage: 12, RareDropItem: "void_crystal", RareDropPermille: 300, SuppressorTags: []string{"ranger"}, BountyCoins: 6},
		},
		Rituals: []sim.RitualSpec{
			{ID: "reactor_harmony_rite", Name: "反应堆谐律大典", NodeFeature: "reactor_shrine", RequiredTag: "psion", ScheduledSeason: "perihelion", CrisisTrigger: true, Inputs: map[string]int{"core_cell": 1}, EnergyCost: 10, Ticks: 2, MoraleBoost: 10, TrustBoost: 2, DisasterReliefWeightPct: 30, ThreatAppeaseAmount: 10},
		},
		Diseases: []sim.DiseaseSpec{
			{ID: "rad_sickness", Name: "辐射衰弱症", MaxSanitationTrigger: 55, ColdTrigger: true, ContagionPermille: 25, DurationTicks: 192, HealthDrainPerDay: 8, LethalityPermille: 15},
		},
		Lifecycle: sim.LifecycleSpec{
			ChildMaxAge: 15, ElderMinAge: 60, MaxExpectancy: 90,
			FertilityMinFoodPerCapita: 2, AnnualBirthPermille: 24, ElderAnnualDeathPermille: 30,
			CompostItem: "bio_nutrient", WasteItem: "organic_waste", WastePerFoodDivisor: 4, CompostRecipeID: "recycle_waste", AutoHydrateSuccessor: true,
		},
	}

	w := &sim.World{
		ID:      "scifi-orbital-colony",
		Map:     sciFiMap,
		Catalog: sciFiCat,
		Factions: []sim.Faction{
			{ID: "bio_corps", Name: "生态水培总署", District: "hydro_dome", Professions: []string{"hydro_tech"}, Level: 1},
			{ID: "mining_syndicate", Name: "深空矿业联合体", District: "ice_asteroid", Professions: []string{"void_miner"}, Level: 1},
			{ID: "transit_fleet", Name: "星环穿梭舰队", District: "orbital_hub", Professions: []string{"shuttle_captain"}, Level: 1},
			{ID: "defense_wing", Name: "轨道防卫军", District: "ice_asteroid", Professions: []string{"exo_ranger"}, Level: 1},
			{ID: "core_order", Name: "核心谐律院", District: "orbital_hub", Professions: []string{"harmonic_psion"}, Level: 1},
		},
		Houses: []sim.House{
			{ID: "hab_01", Name: "一号居住舱", District: "orbital_hub", OwnerID: "c01", Residents: []string{"c01", "c02"}, Level: 2, Comfort: 2, Condition: 95},
		},
		Actors: []sim.Actor{
			{
				ID: "c01", Name: "林舰长", Role: "harmonic_psion", Tags: []string{"colonist", "trader"},
				Attributes:  sim.Attributes{Strength: 9, Dexterity: 9, Insight: 12, Spirit: 12},
				Professions: map[string]int{"harmonic_psion": 2}, Guilds: []string{"core_order"},
				Location: "orbital_hub", HomeID: "hab_01", AgeYears: 32, Health: 100, Energy: 95, Money: 30,
				Items: map[string]int{"synth_ration": 8, "core_cell": 2, "nano_med": 2, "alloy_ore": 2},
				Trust: map[string]int{},
			},
			{
				ID: "c02", Name: "赵游骑", Role: "exo_ranger", Tags: []string{"colonist", "trader"},
				Attributes:  sim.Attributes{Strength: 13, Dexterity: 12, Insight: 9, Spirit: 10},
				Professions: map[string]int{"exo_ranger": 2}, Guilds: []string{"defense_wing"},
				Location: "ice_asteroid", HomeID: "hab_01", AgeYears: 28, Health: 100, Energy: 95, Money: 20,
				Items: map[string]int{"synth_ration": 8, "nano_med": 2, "thermal_suit": 1},
				Trust: map[string]int{},
			},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "cohort_hydro", Name: "水培技师群", District: "hydro_dome", FactionID: "bio_corps", Profession: "hydro_tech", Tier: 1, ProfLevel: 2, Population: 2000, Children: 400, Adults: 1400, Elders: 200, AvgEnergy: 85, MoneyPool: 8000, ItemPool: map[string]int64{"synth_ration": 12000, "bio_nutrient": 1000}},
			{ID: "cohort_miners", Name: "深空矿工群", District: "ice_asteroid", FactionID: "mining_syndicate", Profession: "void_miner", Tier: 1, ProfLevel: 2, Population: 1500, Children: 250, Adults: 1100, Elders: 150, AvgEnergy: 85, MoneyPool: 9000, ItemPool: map[string]int64{"synth_ration": 6000, "alloy_ore": 4000}},
			{ID: "cohort_pilots", Name: "穿梭舰员群", District: "orbital_hub", FactionID: "transit_fleet", Profession: "shuttle_captain", Tier: 1, ProfLevel: 2, Population: 500, Children: 80, Adults: 370, Elders: 50, AvgEnergy: 85, MoneyPool: 5000, ItemPool: map[string]int64{"synth_ration": 2500}},
			{ID: "cohort_rangers", Name: "星环游骑兵群", District: "ice_asteroid", FactionID: "defense_wing", Profession: "exo_ranger", Tier: 1, ProfLevel: 2, Population: 500, Children: 70, Adults: 390, Elders: 40, AvgEnergy: 85, MoneyPool: 5000, ItemPool: map[string]int64{"synth_ration": 3000, "thermal_suit": 500}},
			{ID: "cohort_psions", Name: "反应堆司祭群", District: "orbital_hub", FactionID: "core_order", Profession: "harmonic_psion", Tier: 1, ProfLevel: 2, Population: 498, Children: 80, Adults: 370, Elders: 48, AvgEnergy: 85, MoneyPool: 6000, ItemPool: map[string]int64{"synth_ration": 3000, "core_cell": 300, "nano_med": 600}},
		},
	}
	w.InitEcology(5000)
	initialPop := w.TotalPopulation()

	summary := sim.NewScaleSummary()
	for step := 0; step < 7*sim.TicksPerDay; step++ {
		evs, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatalf("sci-fi step %d failed: %v", step, err)
		}
		summary.RecordEvents(evs)
	}

	if got, want := w.TotalPopulation()+w.TotalDeaths(), initialPop+w.TotalBirths(); got != want {
		t.Fatalf("sci-fi demographic conservation failed: living=%d + deaths=%d != initial=%d + births=%d",
			w.TotalPopulation(), w.TotalDeaths(), initialPop, w.TotalBirths())
	}
	if summary.ProducedByItem["synth_ration"] == 0 || summary.ProducedByItem["alloy_ore"] == 0 {
		t.Fatalf("expected sci-fi colony to produce synth_ration and alloy_ore, got %v", summary.ProducedByItem)
	}
	if summary.LandFreightBatches == 0 || summary.FreightFees == 0 {
		t.Fatalf("expected shuttle_corridor inter-sector freight & shuttle_pilot fees, got batches=%d fees=%d", summary.LandFreightBatches, summary.FreightFees)
	}
	if summary.CompostRecycled == 0 {
		t.Fatalf("expected recycler_bay to recycle organic waste into bio_nutrient, got 0")
	}
	if summary.ThreatHunts == 0 || summary.Rituals == 0 {
		t.Fatalf("expected exo_ranger void_parasite hunts and reactor_harmony_rite rituals, got hunts=%d rituals=%d", summary.ThreatHunts, summary.Rituals)
	}
	t.Logf("Sci-Fi Orbital Colony 7d summary: pop=%d births=%d deaths=%d(%v) produced=%v shuttle_batches=%d freight_fees=%d bio_nutrient_recycled=%d hunts=%d rituals=%d",
		w.TotalPopulation(), w.TotalBirths(), w.TotalDeaths(), w.DeathsByCause, summary.ProducedByItem,
		summary.LandFreightBatches, summary.FreightFees, summary.CompostRecycled, summary.ThreatHunts, summary.Rituals)
}

func TestTaxationCorruptionProtectionRacketAndPurgeConservation(t *testing.T) {
	// Part 1: Focal-actor micro interactions (extort -> bribe collusion -> censorate audit_corruption)
	mv := sim.LargeWorld("micro-gov", 1000)
	gHyd, err := mv.HydrateActor("cohort_market_gangsters")
	if err != nil {
		t.Fatal(err)
	}
	cHyd, err := mv.HydrateActor("cohort_crossroads_constables")
	if err != nil {
		t.Fatal(err)
	}
	gangster := actorByID(mv, gHyd.ID)
	constable := actorByID(mv, cHyd.ID)
	peddler := actorByID(mv, "p27")
	xiucai := actorByID(mv, "p29")
	if gangster == nil || peddler == nil || constable == nil || xiucai == nil {
		t.Fatal("missing expected focal roles")
	}
	// Promote scholar to xiucai via official join_profession so censor capability is granted declaratively via Catalog
	xiucai.InsideHouse = ""
	xiucai.Location = "crossroads"
	xiucai.Professions["scholar"] = 2
	factionByID(mv, "scholars_guild").Level = 2
	if joinEvs := mv.Apply(xiucai.ID, sim.Action{Kind: "join_profession", ProfessionID: "xiucai"}); lastOfKind(joinEvs, "profession_joined").Kind != "profession_joined" {
		t.Fatalf("expected xiucai promotion, got %v", joinEvs)
	}

	for _, a := range []*sim.Actor{gangster, peddler, constable, xiucai} {
		a.InsideHouse = ""
		a.Location = "market_street"
		a.Energy = 90
		a.Health = 100
	}
	peddler.Money = 30
	gangster.Money = 25
	constable.Money = 10
	microInitialCoins := mv.TotalCoins()

	// 1a. Gangster extorts protection fee from peddler
	evs := mv.Apply(gangster.ID, sim.Action{Kind: "extort", TargetID: peddler.ID})
	if last := lastOfKind(evs, "extorted"); last.Kind != "extorted" {
		t.Fatalf("expected extorted event, got %v", evs)
	}
	if gangster.Money <= 25 || peddler.Money >= 30 {
		t.Fatalf("expected protection fee transfer from peddler to gangster: gangster=%d peddler=%d", gangster.Money, peddler.Money)
	}

	// 1b. Gangster bribes constable; constable accumulates IllicitGain & skips arrest in offline rules
	evs = mv.Apply(gangster.ID, sim.Action{Kind: "bribe", TargetID: constable.ID})
	if last := lastOfKind(evs, "bribed"); last.Kind != "bribed" {
		t.Fatalf("expected bribed event, got %v", evs)
	}
	if constable.IllicitGain <= 0 || constable.Trust[gangster.ID] <= 0 {
		t.Fatalf("expected constable illicit gain and positive collusion trust, got illicit=%d trust=%d", constable.IllicitGain, constable.Trust[gangster.ID])
	}
	dec, err := agent.Offline().Decide(context.Background(), mv.View(constable.ID))
	if err != nil {
		t.Fatal(err)
	}
	if dec.Kind == "arrest" && dec.TargetID == gangster.ID {
		t.Fatalf("bribed constable should look the other way rather than immediately arresting briber: %+v", dec)
	}

	// 1c. Xiucai (censor tag) audits corrupt constable, jailing him & confiscating illicit gains to treasury
	beforeTreasury := mv.PublicTreasury
	evs = mv.Apply(xiucai.ID, sim.Action{Kind: "audit_corruption", TargetID: constable.ID})
	if last := lastOfKind(evs, "corruption_purged"); last.Kind != "corruption_purged" {
		t.Fatalf("expected corruption_purged event, got %v", evs)
	}
	if constable.IllicitGain != 0 || constable.JailedUntil <= mv.Tick || mv.PublicTreasury <= beforeTreasury {
		t.Fatalf("expected corrupt constable purged, jailed, and treasury credited: illicit=%d jailed_until=%d treasury=%d->%d",
			constable.IllicitGain, constable.JailedUntil, beforeTreasury, mv.PublicTreasury)
	}
	if got := mv.TotalCoins(); got != microInitialCoins {
		t.Fatalf("micro governance violated coin conservation: got=%d want=%d", got, microInitialCoins)
	}

	// Part 2: 1,000,000-population 30-day (2,880 ticks) macro fiscal, corruption, racket, and 6-ledger conservation
	const initialPop = 1_000_000
	w := sim.LargeWorld("million-gov-30d", initialPop)
	initialCoins := w.TotalCoins()
	summary := sim.NewScaleSummary()

	const totalTicks = 30 * sim.TicksPerDay
	for step := 0; step < totalTicks; step++ {
		stepEvs, err := w.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		summary.RecordEvents(stepEvs)
		if step%sim.TicksPerDay == sim.TicksPerDay-1 {
			if got := w.TotalCoins(); got != initialCoins {
				t.Fatalf("day %d M0 coin conservation drift: got=%d != initial=%d (diff=%d)",
					(step+1)/sim.TicksPerDay, got, initialCoins, got-initialCoins)
			}
		}
	}

	// Verify all 6 zero-error conservation ledgers:
	// 1. Population
	if got, want := w.TotalPopulation()+w.TotalDeaths(), initialPop+w.TotalBirths(); got != want {
		t.Fatalf("1M 30d population ledger mismatch: %d != %d", got, want)
	}
	// 2. Wounds
	if got, want := summary.WoundedPeople, summary.HealedPeople+summary.WoundedDeaths+int64(w.TotalWounded()); got != want {
		t.Fatalf("1M 30d wound ledger mismatch: %d != %d", got, want)
	}
	// 3. Epidemics
	if got, want := summary.EpidemicCases, summary.CuredPeople+w.DeathsByCause["disease"]+summary.SickDiedOther+int64(w.TotalSick()); got != want {
		t.Fatalf("1M 30d epidemic ledger mismatch: %d != %d", got, want)
	}
	// 4. Jail (including corrupt officials jailed & back-door bribe releases)
	if got, want := summary.JailedPeople, summary.ReleasedPeople+summary.JailedDeaths+int64(w.TotalJailed()); got != want {
		t.Fatalf("1M 30d jail ledger mismatch: jailed=%d != released(%d)+jailed_deaths(%d)+active(%d)=%d",
			got, summary.ReleasedPeople, summary.JailedDeaths, w.TotalJailed(), want)
	}
	// 5. Public Treasury & Tax Assessment Split
	tl := w.TreasuryLedger
	if got, want := tl.TaxAssessedGross, tl.TaxToTreasury+tl.TaxSkimmed; got != want {
		t.Fatalf("1M 30d tax split mismatch: gross=%d != net(%d)+skimmed(%d)=%d", got, tl.TaxToTreasury, tl.TaxSkimmed, want)
	}
	wantTreasury := tl.InitialTreasury + tl.TaxToTreasury + tl.StateFeesIn + tl.ConfiscatedIn - tl.PayrollPaid - tl.ReliefAndInfraOut - tl.BountiesOut - tl.TreasuryEmbezzled
	if w.PublicTreasury != wantTreasury {
		t.Fatalf("1M 30d treasury ledger mismatch: public_treasury=%d != expected=%d (%+v)", w.PublicTreasury, wantTreasury, tl)
	}
	// 6. Global M0 Money Supply
	if got := w.TotalCoins(); got != initialCoins {
		t.Fatalf("1M 30d M0 coin conservation mismatch: got=%d != initial=%d (diff=%d)", got, initialCoins, got-initialCoins)
	}

	// Verify meaningful multi-pillar taxation, corruption/bribery/purge cycles, and gang protection rackets
	if tl.TaxAssessedGross < 10_000_000 || tl.TaxToTreasury < 8_000_000 {
		t.Fatalf("expected realistic multi-million 30d tax revenue on 1M pop, got gross=%d net=%d", tl.TaxAssessedGross, tl.TaxToTreasury)
	}
	if tl.TaxSkimmed == 0 || summary.EmbezzledCoins == 0 || summary.BribeIncidents == 0 || summary.BribeCoins == 0 {
		t.Fatalf("expected active tax skimming, payroll/relief embezzlement, and underworld bribery; got skimmed=%d embezzled=%d bribes=%d(%d coins)",
			tl.TaxSkimmed, summary.EmbezzledCoins, summary.BribeIncidents, summary.BribeCoins)
	}
	if summary.AuditPurges == 0 || summary.CorruptionJailed == 0 || summary.ConfiscatedCoins == 0 {
		t.Fatalf("expected censorate anti-corruption purges jailing corrupt officers and confiscating illicit wealth; got purges=%d jailed=%d confiscated=%d",
			summary.AuditPurges, summary.CorruptionJailed, summary.ConfiscatedCoins)
	}
	if summary.RacketIncidents == 0 || summary.ProtectionCoins == 0 || summary.RacketClashes == 0 {
		t.Fatalf("expected gang protection rackets and refusal clashes; got incidents=%d clashes=%d protection_coins=%d",
			summary.RacketIncidents, summary.RacketClashes, summary.ProtectionCoins)
	}

	t.Logf("1M 30d Fiscal & Governance Summary:\n  M0 Coins: initial=%d ending=%d\n  Treasury: init=%d + tax_net=%d (gross=%d, skimmed=%d, surcharge=%d) + fees_in=%d + confiscated_in=%d - payroll=%d - relief=%d - bounties=%d - embezzled=%d => ending=%d\n  Corruption: avg_idx=%d illicit_held=%d embezzled_total=%d bribes=%d(coins=%d, bailed=%d) purges=%d(jailed=%d, confiscated=%d)\n  Protection Rackets: incidents=%d batches=%d clashes=%d protection_coins=%d\n  Coins Traded=%d Raid=%d Turf=%d Theft=%d Arrest=%d",
		initialCoins, w.TotalCoins(),
		tl.InitialTreasury, tl.TaxToTreasury, tl.TaxAssessedGross, tl.TaxSkimmed, summary.TaxSurcharge, tl.StateFeesIn, tl.ConfiscatedIn,
		tl.PayrollPaid, tl.ReliefAndInfraOut, tl.BountiesOut, tl.TreasuryEmbezzled, w.PublicTreasury,
		w.AvgCorruption(), w.TotalIllicitWealth(), summary.EmbezzledCoins, summary.BribeIncidents, summary.BribeCoins, summary.BribeReleased,
		summary.AuditPurges, summary.CorruptionJailed, summary.ConfiscatedCoins,
		summary.RacketIncidents, summary.RacketBatches, summary.RacketClashes, summary.ProtectionCoins,
		summary.CoinsTraded, summary.RaidIncidents, summary.TurfWarIncidents, summary.TheftBatches, summary.ArrestBatches)
}

func TestIntegratedFourPillarsMillionPopConservation(t *testing.T) {
	const initialPop = 1_000_000
	w := sim.LargeWorld("integrated-four-pillars-1m", initialPop)
	initialCoins := w.TotalCoins()

	// Seed localized governance stress in south_fields so trials, framing, appeals, strikes/riots/revolts,
	// market hoarding, credit/foreclosures, weddings/feuds/turf, and spotlight rotation all trigger naturally.
	if w.DistrictStates != nil {
		ds := w.DistrictStates["south_fields"]
		ds.Corruption = 40
		ds.BribeShieldTicks = 72
		ds.RacketPressure = 24
		w.DistrictStates["south_fields"] = ds
	}
	for ci := range w.Cohorts {
		c := &w.Cohorts[ci]
		if c.District == "south_fields" && c.Profession == "farmer" {
			c.AvgHunger = 4
			if c.ItemPool != nil && c.ItemPool["food"] > int64(c.Population) {
				c.ItemPool["food"] = int64(c.Population) / 2
			}
			break
		}
	}

	td := agent.NewTieredDecider(agent.Offline(), 12)
	summary := sim.NewScaleSummary()
	totalTicks := 8 * sim.TicksPerDay
	for step := 0; step < totalTicks; step++ {
		if summary.RevoltsErupted == 0 && w.DistrictStates != nil {
			ds := w.DistrictStates["south_fields"]
			if ds.Corruption < 38 {
				ds.Corruption = 38
			}
			ds.BribeShieldTicks = max(ds.BribeShieldTicks, 24)
			w.DistrictStates["south_fields"] = ds
			for ci := range w.Cohorts {
				if w.Cohorts[ci].District == "south_fields" && w.Cohorts[ci].AvgHunger < 4 {
					w.Cohorts[ci].AvgHunger = 4
				}
			}
		}
		if w.SpotlightRumor != nil {
			td.SetSpotlightPriority(w.Actors, w.SpotlightRumor.ActorSalience, w.Tick)
		}
		evs, err := w.Step(context.Background(), td)
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		summary.RecordEvents(evs)

		// Strict per-tick Population and M0 Coin conservation
		if got, want := w.TotalPopulation()+w.TotalDeaths(), initialPop+w.TotalBirths(); got != want {
			t.Fatalf("tick %d population ledger mismatch: pop(%d)+deaths(%d)=%d != init(%d)+births(%d)=%d",
				w.Tick, w.TotalPopulation(), w.TotalDeaths(), got, initialPop, w.TotalBirths(), want)
		}
		if gotCoins := w.TotalCoins(); gotCoins != initialCoins {
			t.Fatalf("tick %d total coins mismatch: got=%d want=%d", w.Tick, gotCoins, initialCoins)
		}
	}

	// Verify all 6 ledgers at 0-error
	if gotW, wantW := summary.WoundedPeople, summary.HealedPeople+summary.WoundedDeaths+int64(w.TotalWounded()); gotW != wantW {
		t.Fatalf("wound ledger mismatch: wounded=%d != healed(%d)+wounded_deaths(%d)+active(%d)=%d",
			gotW, summary.HealedPeople, summary.WoundedDeaths, w.TotalWounded(), wantW)
	}
	if gotS, wantS := summary.EpidemicCases, summary.CuredPeople+summary.DeathsByCause["disease"]+summary.SickDiedOther+int64(w.TotalSick()); gotS != wantS {
		t.Fatalf("epidemic ledger mismatch: cases=%d != cured(%d)+disease_deaths(%d)+other(%d)+active(%d)=%d",
			gotS, summary.CuredPeople, summary.DeathsByCause["disease"], summary.SickDiedOther, w.TotalSick(), wantS)
	}
	if gotJ, wantJ := summary.JailedPeople, summary.ReleasedPeople+summary.JailedDeaths+int64(w.TotalJailed()); gotJ != wantJ {
		t.Fatalf("jail ledger mismatch: jailed=%d != released(%d)+jailed_deaths(%d)+active(%d)=%d",
			gotJ, summary.ReleasedPeople, summary.JailedDeaths, w.TotalJailed(), wantJ)
	}
	tl := w.TreasuryLedger
	if got, want := tl.TaxAssessedGross, tl.TaxToTreasury+tl.TaxSkimmed; got != want {
		t.Fatalf("tax split mismatch: gross=%d != net(%d)+skimmed(%d)=%d", got, tl.TaxToTreasury, tl.TaxSkimmed, want)
	}
	expectedTreasury := tl.InitialTreasury + tl.TaxToTreasury + tl.StateFeesIn + tl.ConfiscatedIn -
		tl.PayrollPaid - tl.ReliefAndInfraOut - tl.BountiesOut - tl.TreasuryEmbezzled
	if w.PublicTreasury != expectedTreasury {
		t.Fatalf("treasury ledger mismatch: actual=%d != expected=%d (ledger=%+v)", w.PublicTreasury, expectedTreasury, tl)
	}

	// Verify all 4 pillars activated and integrated
	if w.MarketCredit == nil || len(w.MarketCredit.DistrictPrices) == 0 || summary.HoardEvents == 0 || summary.LoansIssued == 0 {
		t.Fatalf("expected active market & credit pillar (prices/hoarding/loans), got mc=%v hoards=%d loans=%d",
			w.MarketCredit != nil, summary.HoardEvents, summary.LoansIssued)
	}
	gini := w.WealthGiniPermille(w.MarketCredit)
	if gini <= 0 {
		t.Fatalf("expected non-zero wealth Gini index, got %d", gini)
	}
	if w.Relations == nil || summary.Marriages == 0 || len(w.Relations.DistrictSovereignty) == 0 || len(w.Relations.MonopolyCharters) == 0 {
		t.Fatalf("expected active relations, kinship, turf & charter pillar, got marriages=%d turf=%d charters=%d",
			summary.Marriages, summary.TurfShifts, summary.MonopolyCharters)
	}
	if w.JusticeRevolt == nil || summary.CourtTrials == 0 || summary.WrongfulConvictions == 0 || summary.AppealsOverturned == 0 ||
		summary.TaxStrikes+summary.GranaryRiots+summary.RevoltsErupted == 0 {
		t.Fatalf("expected active justice & revolt pillar, got trials=%d wrongful=%d overturned=%d strikes=%d riots=%d revolts=%d",
			summary.CourtTrials, summary.WrongfulConvictions, summary.AppealsOverturned, summary.TaxStrikes, summary.GranaryRiots, summary.RevoltsErupted)
	}
	if w.SpotlightRumor == nil || summary.SpotlightPromotions == 0 || summary.GossipSeeded != 0 {
		t.Fatalf("expected active spotlight pillar with zero direct macro gossip injection, got promoted=%d demoted=%d panics=%d gossip=%d",
			summary.SpotlightPromotions, summary.SpotlightDemotions, summary.RumorPanics, summary.GossipSeeded)
	}

	// Verify JSON snapshot round-trip with all 4 pillars populated
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	var restored sim.World
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatal("integrated 1,000,000-population world did not round-trip identically through JSON")
	}

	t.Logf("Integrated 1M 8-Day Summary: gini=%d‰ focal_calls=%d market(hoards=%d,loans=%d,repaid=%d,foreclosed=%d,annexed=%d) relations(marriages=%d,bonds=%d,grudges=%d,feuds=%d,vendettas=%d,turf_shifts=%d,charters=%d) justice(trials=%d,wrongful=%d,overturned=%d,strikes=%d,riots=%d,revolts=%d,amnesties=%d,reintegrated=%d) spotlight(promoted=%d,demoted=%d,panics=%d,gossip=%d)",
		gini, td.TotalFocalCalls(),
		summary.HoardEvents, summary.LoansIssued, summary.LoansRepaid, summary.HousesForeclosed, summary.CohortLandsAnnexed,
		summary.Marriages, summary.BondsSworn, summary.GrudgesFormed, summary.FeudsAvenged+summary.FeudsMediated, summary.VendettaClashes, summary.TurfShifts, summary.MonopolyCharters,
		summary.CourtTrials, summary.WrongfulConvictions, summary.AppealsOverturned, summary.TaxStrikes, summary.GranaryRiots, summary.RevoltsErupted, summary.AmnestiesDecreed, summary.RebelsReintegrated,
		summary.SpotlightPromotions, summary.SpotlightDemotions, summary.RumorPanics, summary.GossipSeeded)
}

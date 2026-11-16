package sim_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestMultiGenre_XianxiaSpiritArray_DiscoveryTeachBuildAndAmbientDrain(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.Recipes = append(cat.Recipes,
		sim.Recipe{
			ID:          "refine_spirit_pill",
			Label:       "Refine Spirit Pill",
			RequiredTag: "cultivator",
			Ticks:       1,
			EnergyCost:  2,
			Inputs:      map[string]int{"herb": 1},
			OutputItem:  "medicine",
			OutputQty:   2,
		},
		sim.Recipe{
			ID:          "spirit_array_elixir",
			Label:       "Condense Array Elixir",
			RequiredTag: "cultivator",
			NodeFeature: "spirit_array",
			Ticks:       1,
			EnergyCost:  2,
			Inputs:      map[string]int{"herb": 1},
			OutputItem:  "medicine",
			OutputQty:   4,
		},
	)
	cat.SensoryModes = map[string]sim.SensorySpec{
		"divine_sense": {
			ID:               "divine_sense",
			Name:             "Divine Sense",
			GrantedByTags:    []string{"sensory_cultivator"},
			RangeKm:          15,
			PenetrationPower: 50,
		},
	}
	cat.Techs = map[string]sim.TechSpec{
		"spirit_gathering_array": {
			ID:                       "spirit_gathering_array",
			Name:                     "Spirit Gathering Array",
			GrantedByTags:            []string{"sect_elder"},
			ResearchTicks:            2,
			ResearchEnergyCost:       6,
			ResearchInputs:           map[string]int{"ore": 1},
			FacilityName:             "spirit_gathering_array",
			AdoptTicks:               2,
			AdoptEnergyCost:          4,
			AdoptInputs:              map[string]int{"ore": 2},
			GrantsVenueFeature:       "spirit_array",
			AmbientFieldDeltaPerTick: map[string]int{"spirit_qi": -5},
			ProductionBonusPct:       50,
		},
	}

	w := &sim.World{
		ID:      "xianxia-knowledge",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{
					ID:            "spirit_peak_region",
					Name:          "Azure Cloud Peak",
					Coord:         sim.GeoCoord{X: 0, Y: 0},
					ElevationM:    1800,
					AmbientFields: map[string]int{"spirit_qi": 100},
				},
				{
					ID:            "rogue_valley_region",
					Name:          "Misty Hollow",
					Coord:         sim.GeoCoord{X: 8, Y: 0},
					ElevationM:    300,
					AmbientFields: map[string]int{"spirit_qi": 60},
				},
			},
			Districts: []sim.District{
				{ID: "sect_mountain", Name: "Azure Sect Mountain", GeoRegionID: "spirit_peak_region", Features: []string{"shrine"}},
				{ID: "rogue_valley", Name: "Misty Hollow", GeoRegionID: "rogue_valley_region", Features: []string{"herb_patch"}},
			},
			Roads: []sim.Road{
				{From: "sect_mountain", To: "rogue_valley", Ticks: 2, EnergyCost: 3},
			},
			Venues: []sim.Venue{
				{ID: "sect_altar", Name: "Peak Spirit Altar", DistrictID: "sect_mountain", GeoRegionID: "spirit_peak_region", Kind: "altar", Features: []string{"shrine"}},
				{ID: "rival_herb_garden", Name: "Rogue Herb Garden", DistrictID: "rogue_valley", GeoRegionID: "rogue_valley_region", Kind: "garden", Features: []string{"herb_patch"}},
			},
		},
		Actors: []sim.Actor{
			{
				ID: "elder", Name: "Elder Qing", Role: "elder",
				Tags:     []string{"resident", "sect_elder", "cultivator"},
				Location: "sect_mountain", VenueID: "sect_altar",
				Health: 100, Energy: 90, Money: 20,
				Items: map[string]int{"ore": 3, "herb": 4, "food": 6},
				Trust: map[string]int{}, Goal: "research formation arrays",
			},
			{
				ID: "disciple", Name: "Disciple Lin", Role: "disciple",
				Tags:     []string{"resident", "cultivator"},
				Location: "sect_mountain", VenueID: "sect_altar",
				Health: 100, Energy: 90, Money: 10,
				Items: map[string]int{"ore": 3, "herb": 6, "food": 6},
				Trust: map[string]int{}, Goal: "cultivate and build sect arrays",
			},
			{
				ID: "rogue", Name: "Rogue Han", Role: "cultivator",
				Tags:     []string{"resident", "cultivator", "sensory_cultivator"},
				Location: "rogue_valley", VenueID: "rival_herb_garden",
				Health: 100, Energy: 90, Money: 10,
				Items: map[string]int{"ore": 3, "herb": 6, "food": 6},
				Trust: map[string]int{}, Goal: "guard the hollow",
			},
			{
				ID: "bystander", Name: "Acolyte Zhou", Role: "acolyte",
				Tags:     []string{"resident"},
				Location: "sect_mountain", VenueID: "sect_altar",
				Health: 100, Energy: 90, Money: 5,
				Items: map[string]int{"food": 6},
				Trust: map[string]int{}, Goal: "sweep the altar",
			},
		},
	}

	// 1. Pre-discovery candidate isolation: Elder sees research_tech, Rogue does NOT.
	hasOption := func(opts []agent.Option, kind, techID string) bool {
		for _, o := range opts {
			if o.Action.Kind == kind && o.Action.TechID == techID {
				return true
			}
		}
		return false
	}
	if !hasOption(agent.Options(w.View("elder")), "research_tech", "spirit_gathering_array") {
		t.Fatalf("expected elder to see research_tech option for spirit_gathering_array")
	}
	rogueView := w.View("rogue")
	if len(rogueView.KnownTechs) != 0 {
		t.Fatalf("expected rogue KnownTechs to be empty before discovery, got %v", rogueView.KnownTechs)
	}
	rogueOpts := agent.Options(rogueView)
	if hasOption(rogueOpts, "research_tech", "spirit_gathering_array") ||
		hasOption(rogueOpts, "teach_knowledge", "spirit_gathering_array") ||
		hasOption(rogueOpts, "adopt_facility", "spirit_gathering_array") {
		t.Fatalf("rogue cultivator must not see restricted sect tech candidates")
	}

	// 2. Elder researches spirit_gathering_array (2 ticks).
	startEvs := w.Apply("elder", sim.Action{Kind: "research_tech", TechID: "spirit_gathering_array"})
	if len(startEvs) != 1 || startEvs[0].Kind != "task_started" {
		t.Fatalf("expected task_started for 2-tick research, got %+v", startEvs)
	}
	discEvs := w.Advance("elder")
	if len(discEvs) != 1 || discEvs[0].Kind != "tech_discovered" {
		t.Fatalf("expected tech_discovered on completion, got %+v", discEvs)
	}
	discoveryEventID := discEvs[0].ID

	if !w.KnowsTech("elder", "spirit_gathering_array") {
		t.Fatalf("expected elder to know spirit_gathering_array after research")
	}
	if w.KnowsTech("disciple", "spirit_gathering_array") || w.KnowsTech("rogue", "spirit_gathering_array") {
		t.Fatalf("knowledge must NOT spread globally or to sectmates/rivals on discovery")
	}
	if len(w.View("rogue").KnownTechs) != 0 {
		t.Fatalf("rogue Perception.KnownTechs must remain empty after elder's secret discovery")
	}

	// Verify elder got first-hand Observation on discovery.
	elderObsFound := false
	for _, m := range w.Actors[0].Memories {
		if m.Kind == "tech_discovered" && m.Observation != nil && m.Observation.Item == "spirit_gathering_array" {
			elderObsFound = true
		}
	}
	if !elderObsFound {
		t.Fatalf("expected elder to record first-hand Observation for tech_discovered")
	}

	// Rival venue gets 0 production bonus and lacks spirit_array feature.
	rogueCraft := w.Apply("rogue", sim.Action{Kind: "craft", RecipeID: "refine_spirit_pill"})
	if len(rogueCraft) != 1 || rogueCraft[0].Kind != "crafted" || rogueCraft[0].Payload["quantity"] != 2 {
		t.Fatalf("expected rogue base craft quantity 2 (0%% bonus), got %+v", rogueCraft)
	}
	if failElixir := w.Apply("rogue", sim.Action{Kind: "craft", RecipeID: "spirit_array_elixir"}); len(failElixir) != 1 || failElixir[0].Kind != "action_failed" {
		t.Fatalf("expected rogue to fail spirit_array_elixir without adopted array, got %+v", failElixir)
	}

	// 3. Elder teaches disciple (co-located), establishing causal lineage.
	teachEvs := w.Apply("elder", sim.Action{Kind: "teach_knowledge", TechID: "spirit_gathering_array", TargetID: "disciple"})
	if len(teachEvs) != 1 || teachEvs[0].Kind != "knowledge_taught" {
		t.Fatalf("expected knowledge_taught event, got %+v", teachEvs)
	}
	if teachEvs[0].Payload["basis_event_id"] != discoveryEventID || teachEvs[0].Payload["root_event_id"] != discoveryEventID {
		t.Fatalf("expected causal lineage to point to discovery event %d, got basis=%v root=%v",
			discoveryEventID, teachEvs[0].Payload["basis_event_id"], teachEvs[0].Payload["root_event_id"])
	}
	if !w.KnowsTech("disciple", "spirit_gathering_array") || w.KnowsTech("rogue", "spirit_gathering_array") {
		t.Fatalf("expected disciple (and not rogue) to know spirit_gathering_array after teaching")
	}
	if !hasOption(agent.Options(w.View("disciple")), "adopt_facility", "spirit_gathering_array") {
		t.Fatalf("expected disciple Options to expose adopt_facility after learning tech")
	}

	// 4. Disciple adopts spirit_gathering_array at sect_altar (2 ticks), witnessed by bystander.
	adoptStart := w.Apply("disciple", sim.Action{Kind: "adopt_facility", TechID: "spirit_gathering_array"})
	if len(adoptStart) != 1 || adoptStart[0].Kind != "task_started" {
		t.Fatalf("expected task_started for adopt_facility, got %+v", adoptStart)
	}
	adoptDone := w.Advance("disciple")
	if len(adoptDone) != 1 || adoptDone[0].Kind != "facility_adopted" {
		t.Fatalf("expected facility_adopted event, got %+v", adoptDone)
	}

	sectAltar, _ := w.Map.VenueByID("sect_altar")
	if !sim.ActorHasTag(sectAltar.Features, "spirit_array") {
		t.Fatalf("expected sect_altar to gain GrantsVenueFeature 'spirit_array', got %v", sectAltar.Features)
	}
	rivalGarden, _ := w.Map.VenueByID("rival_herb_garden")
	if sim.ActorHasTag(rivalGarden.Features, "spirit_array") {
		t.Fatalf("rival_herb_garden must NOT gain spirit_array feature")
	}
	bystanderWitnessed := false
	for _, m := range w.Actors[3].Memories {
		if m.Kind == "facility_adopted" && m.Observation != nil && m.Observation.ObjectID == "sect_altar" {
			bystanderWitnessed = true
		}
	}
	if !bystanderWitnessed {
		t.Fatalf("expected co-located bystander to witness facility_adopted Observation")
	}

	// Disciple crafting at sect_altar gets +50% ProductionBonusPct (2 -> 3) AND unlocks spirit_array_elixir.
	sectCraft := w.Apply("disciple", sim.Action{Kind: "craft", RecipeID: "refine_spirit_pill"})
	if len(sectCraft) != 1 || sectCraft[0].Kind != "crafted" || sectCraft[0].Payload["quantity"] != 3 {
		t.Fatalf("expected disciple at sect_altar to receive +50%% bonus yield (3), got %+v", sectCraft)
	}
	sectElixir := w.Apply("disciple", sim.Action{Kind: "craft", RecipeID: "spirit_array_elixir"})
	if len(sectElixir) != 1 || sectElixir[0].Kind != "crafted" {
		t.Fatalf("expected disciple at sect_altar to craft spirit_array_elixir, got %+v", sectElixir)
	}

	// 5. Step world over 4 ticks: spirit_qi drains (-5/tick => 100 -> 80) and rogue perceives ambient_shift via divine_sense!
	waitDecider := deciderFunc(func(_ context.Context, _ sim.Perception) (sim.Action, error) {
		return sim.Action{Kind: "wait"}, nil
	})
	var shiftEvents []sim.Event
	for tick := 0; tick < 4; tick++ {
		evs, err := w.Step(context.Background(), waitDecider)
		if err != nil {
			t.Fatalf("Step %d failed: %v", tick, err)
		}
		for _, ev := range evs {
			if ev.Kind == "ambient_shift" {
				shiftEvents = append(shiftEvents, ev)
			}
		}
	}
	peakReg, _ := w.Map.GeoRegionByID("spirit_peak_region")
	if peakReg.AmbientFields["spirit_qi"] != 80 {
		t.Fatalf("expected spirit_peak_region spirit_qi to drain from 100 to 80 over 4 ticks, got %d", peakReg.AmbientFields["spirit_qi"])
	}
	if len(shiftEvents) == 0 {
		t.Fatalf("expected ambient_shift event when spirit_qi crossed threshold")
	}
	rogueSensedShift := false
	for _, m := range w.Actors[2].Memories {
		if m.Kind == "ambient_shift" && m.Observation != nil && m.Observation.Modality == "divine_sense" && m.Observation.Item == "spirit_qi" {
			rogueSensedShift = true
		}
	}
	if !rogueSensedShift {
		t.Fatalf("expected nearby rogue cultivator with divine_sense (8km <= 15km) to receive first-hand ambient_shift Observation")
	}

	// 6. Lossless JSON round-trip with reflect.DeepEqual.
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var restored sim.World
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatalf("World JSON round-trip mismatch in Xianxia test")
	}
}

func TestMultiGenre_SciFiTechDiffusion_AndImmutableWorldLaws(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.TagActions["epoch_arbiter"] = []string{"rewrite_world_law"}
	cat.DefaultWorldLaws = map[string]int{
		"dharma_age":        1,
		"emp_storm_law":     0,
		"firearms_disabled": 0,
	}
	cat.Techs = map[string]sim.TechSpec{
		"fusion_microcell": {
			ID:                       "fusion_microcell",
			Name:                     "Fusion Microcell",
			GrantedByTags:            []string{"scientist"},
			ResearchTicks:            1,
			ResearchEnergyCost:       5,
			FacilityName:             "fission_forge",
			AdoptTicks:               1,
			AdoptEnergyCost:          4,
			GrantsVenueFeature:       "fusion_grid",
			AmbientFieldDeltaPerTick: map[string]int{"radiation": 5},
			ProductionBonusPct:       100,
		},
	}
	cat.CommChannels = map[string]sim.CommChannelSpec{
		"quantum_link": {
			ID:               "quantum_link",
			Name:             "Quantum Comms",
			GrantedByTags:    []string{"comms_officer"},
			ReceiverTags:     []string{"comms_officer"},
			RangeKm:          100,
			PenetrationPower: 50,
			EnergyCost:       2,
		},
	}
	cat.Recipes = append(cat.Recipes, sim.Recipe{
		ID:          "craft_firearm_rifle",
		Label:       "Assemble Rail Rifle",
		RequiredTag: "gunsmith",
		Ticks:       1,
		EnergyCost:  2,
		OutputItem:  "tools",
		OutputQty:   1,
	})

	w := &sim.World{
		ID:      "scifi-knowledge-laws",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "orbital_ring", Name: "Orbital Ring", Coord: sim.GeoCoord{X: 0, Y: 0}},
				{ID: "reactor_crater", Name: "Reactor Crater", Coord: sim.GeoCoord{X: 20, Y: 0}, AmbientFields: map[string]int{"radiation": 10}},
			},
			Districts: []sim.District{
				{ID: "orbital_hub", Name: "Orbital Hub", GeoRegionID: "orbital_ring", PolityID: "orbital_command", Features: []string{"market"}},
				{ID: "scrap_basin", Name: "Scrap Basin", GeoRegionID: "reactor_crater", PolityID: "seceded_syndicate", Features: []string{"market"}},
			},
			Venues: []sim.Venue{
				{ID: "orbital_lab", Name: "Orbital Lab", DistrictID: "orbital_hub", GeoRegionID: "orbital_ring", PolityID: "orbital_command", Features: []string{"market"}},
				{ID: "command_deck", Name: "Command Deck", DistrictID: "orbital_hub", GeoRegionID: "orbital_ring", PolityID: "orbital_command"},
				{ID: "surface_yard", Name: "Syndicate Yard", DistrictID: "scrap_basin", GeoRegionID: "reactor_crater", PolityID: "seceded_syndicate", Features: []string{"market"}},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{
					ID: "orbital_command", Name: "Orbital Command", Level: "sector",
					SeatDistrictID: "orbital_hub", SeatVenueID: "command_deck",
					LeaderActorID: "admiral",
				},
				{
					ID: "seceded_syndicate", Name: "Scrap Syndicate", Level: "district",
					DeJureParentID:      "orbital_command",
					DeFactoControllerID: "syndicate_warlord",
					SeatDistrictID:      "scrap_basin", SeatVenueID: "surface_yard",
					LeaderActorID: "warlord",
				},
			},
		},
		Houses: []sim.House{
			{ID: "workshop_h1", Name: "Crater Workshop", District: "scrap_basin", VenueID: "surface_yard", OwnerID: "engineer", Residents: []string{"engineer"}, Level: 1, Condition: 100},
			{ID: "workshop_h2", Name: "Other Bunkhouse", District: "scrap_basin", VenueID: "surface_yard", OwnerID: "warlord", Residents: []string{"warlord"}, Level: 1, Condition: 100},
		},
		Actors: []sim.Actor{
			{
				ID: "scientist", Name: "Dr. Chen", Role: "scientist",
				Tags:     []string{"resident", "scientist", "comms_officer"},
				Location: "orbital_hub", VenueID: "orbital_lab",
				Health: 100, Energy: 90, Money: 20, Items: map[string]int{"food": 5}, Trust: map[string]int{},
			},
			{
				ID: "engineer", Name: "Tech Mara", Role: "engineer",
				Tags:     []string{"resident", "comms_officer"},
				Location: "scrap_basin", VenueID: "surface_yard", HomeID: "workshop_h1",
				Health: 100, Energy: 90, Money: 20, Items: map[string]int{"food": 5}, Trust: map[string]int{},
			},
			{
				ID: "admiral", Name: "Admiral Vance", Role: "commander",
				Tags:     []string{"resident", "ruler"},
				Location: "orbital_hub", VenueID: "command_deck",
				Health: 100, Energy: 90, Money: 50, Items: map[string]int{"food": 5}, Trust: map[string]int{},
			},
			{
				ID: "warlord", Name: "Boss Krag", Role: "warlord",
				Tags:     []string{"resident", "ruler", "gunsmith"},
				Location: "scrap_basin", VenueID: "surface_yard",
				Health: 100, Energy: 90, Money: 50, Items: map[string]int{"food": 5, "tools": 2}, Trust: map[string]int{},
			},
			{
				ID: "epoch_sentinel", Name: "Epoch Core", Role: "arbiter",
				Tags:     []string{"resident", "epoch_arbiter"},
				Location: "orbital_hub", VenueID: "command_deck",
				Health: 100, Energy: 90, Money: 0, Items: map[string]int{"food": 5}, Trust: map[string]int{},
			},
		},
	}

	// Verify initial nil WorldLaws & nil Knowledge JSON round-trip preserves nil.
	rawInit, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("initial json.Marshal failed: %v", err)
	}
	var restoredInit sim.World
	if err := json.Unmarshal(rawInit, &restoredInit); err != nil {
		t.Fatalf("initial json.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(w, &restoredInit) {
		t.Fatalf("initial World with nil Knowledge and nil WorldLaws failed reflect.DeepEqual JSON round-trip")
	}

	// 1. Scientist discovers fusion_microcell; WorldLaws remain untouched.
	discEvs := w.Apply("scientist", sim.Action{Kind: "research_tech", TechID: "fusion_microcell"})
	if len(discEvs) != 1 || discEvs[0].Kind != "tech_discovered" {
		t.Fatalf("expected immediate 1-tick tech_discovered, got %+v", discEvs)
	}
	if w.WorldLaws != nil || w.EffectiveWorldLaw("dharma_age") != 1 || w.EffectiveWorldLaw("emp_storm_law") != 0 {
		t.Fatalf("research_tech must never mutate WorldLaws")
	}

	// 2. Scientist remotely teaches engineer over quantum_link (remote channel: Statement only, no Observation on student).
	teachEvs := w.Apply("scientist", sim.Action{
		Kind:     "teach_knowledge",
		TechID:   "fusion_microcell",
		TargetID: "engineer",
		Channel:  "quantum_link",
	})
	if len(teachEvs) != 1 || teachEvs[0].Kind != "knowledge_taught" {
		t.Fatalf("expected remote knowledge_taught event, got %+v", teachEvs)
	}
	if !w.KnowsTech("engineer", "fusion_microcell") {
		t.Fatalf("expected engineer to know fusion_microcell after quantum_link instruction")
	}
	engMem := w.Actors[1].Memories[len(w.Actors[1].Memories)-1]
	if engMem.Channel != "quantum_link" || engMem.Observation != nil || engMem.Statement == nil {
		t.Fatalf("remote channel teaching must set Channel + Statement and NEVER Observation, got %+v", engMem)
	}

	// 3. Engineer adopts fission_forge inside workshop_h1; other houses unaffected; radiation rises over ticks.
	adoptEvs := w.Apply("engineer", sim.Action{Kind: "adopt_facility", TechID: "fusion_microcell", HouseID: "workshop_h1"})
	if len(adoptEvs) != 1 || adoptEvs[0].Kind != "facility_adopted" {
		t.Fatalf("expected facility_adopted for house workshop_h1, got %+v", adoptEvs)
	}
	if !w.Houses[0].HasFacility("fission_forge") || w.Houses[1].HasFacility("fission_forge") {
		t.Fatalf("expected ONLY workshop_h1 to gain fission_forge, got h1=%v h2=%v", w.Houses[0].Facilities, w.Houses[1].Facilities)
	}

	waitDecider := deciderFunc(func(_ context.Context, _ sim.Perception) (sim.Action, error) {
		return sim.Action{Kind: "wait"}, nil
	})
	for i := 0; i < 3; i++ {
		if _, err := w.Step(context.Background(), waitDecider); err != nil {
			t.Fatalf("Step %d failed: %v", i, err)
		}
	}
	craterReg, _ := w.Map.GeoRegionByID("reactor_crater")
	if craterReg.AmbientFields["radiation"] != 25 { // 10 + 3*5
		t.Fatalf("expected reactor_crater radiation to rise to 25, got %d", craterReg.AmbientFields["radiation"])
	}

	// 4. State B isolation: issue_decree respects DeFactoControllerID and CANNOT mutate KnowledgeBook or WorldLaws.
	decreeEv := w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_command", Item: "embargo:tools", Quantity: 1})
	if len(decreeEv) != 1 || decreeEv[0].Kind != "decree_issued" {
		t.Fatalf("expected admiral embargo decree to succeed, got %+v", decreeEv)
	}
	if !w.IsItemEmbargoedAt("orbital_hub", "orbital_lab", "tools") {
		t.Fatalf("expected loyal orbital_hub to enforce orbital_command embargo")
	}
	if w.IsItemEmbargoedAt("scrap_basin", "surface_yard", "tools") {
		t.Fatalf("expected seceded scrap_basin (DeFactoControllerID=syndicate_warlord) to ignore parent DeJure embargo")
	}
	if badDecree := w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_command", Item: "world_law:dharma_age", Quantity: 0}); len(badDecree) != 1 || badDecree[0].Kind != "action_failed" {
		t.Fatalf("issue_decree must NOT alter WorldLaws, got %+v", badDecree)
	}
	if badTechDecree := w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_command", Item: "tech:fusion_microcell", Quantity: 1}); len(badTechDecree) != 1 || badTechDecree[0].Kind != "action_failed" {
		t.Fatalf("issue_decree must NOT alter KnowledgeBook, got %+v", badTechDecree)
	}

	// 5. State C immutability: unauthorized world-law rewrites emit action_failed; authorized epoch action succeeds.
	if unauthApply := w.Apply("scientist", sim.Action{Kind: "rewrite_world_law", Item: "dharma_age", Quantity: 0}); len(unauthApply) != 1 || unauthApply[0].Kind != "action_failed" {
		t.Fatalf("expected unauthorized scientist rewrite_world_law to fail, got %+v", unauthApply)
	}
	if unauthRuler := w.Apply("admiral", sim.Action{Kind: "rewrite_world_law", Item: "emp_storm_law", Quantity: 1}); len(unauthRuler) != 1 || unauthRuler[0].Kind != "action_failed" {
		t.Fatalf("expected unauthorized ruler rewrite_world_law to fail, got %+v", unauthRuler)
	}
	failHelperEvs, failHelperErr := w.ApplyWorldLawChange(&w.Actors[0], sim.Action{Kind: "rewrite_world_law"}, false, map[string]int{"dharma_age": 99, "emp_storm_law": 1})
	if failHelperErr == nil || len(failHelperEvs) != 1 || failHelperEvs[0].Kind != "action_failed" {
		t.Fatalf("expected ApplyWorldLawChange(..., authorized=false) to emit action_failed and error, got evs=%+v err=%v", failHelperEvs, failHelperErr)
	}
	if w.EffectiveWorldLaw("dharma_age") != 1 || w.EffectiveWorldLaw("emp_storm_law") != 0 {
		t.Fatalf("unauthorized attempts altered WorldLaws: dharma=%d emp=%d", w.EffectiveWorldLaw("dharma_age"), w.EffectiveWorldLaw("emp_storm_law"))
	}

	// Authorized world-pack rewrite via ApplyWorldLawChange and via epoch_arbiter tag.
	authEvs, authErr := w.ApplyWorldLawChange(&w.Actors[4], sim.Action{Kind: "rewrite_world_law"}, true, map[string]int{
		"dharma_age":    0,
		"emp_storm_law": 1,
	})
	if authErr != nil || len(authEvs) != 1 || authEvs[0].Kind != "world_law_rewritten" {
		t.Fatalf("expected authorized ApplyWorldLawChange to emit world_law_rewritten, got evs=%+v err=%v", authEvs, authErr)
	}
	authTagEvs := w.Apply("epoch_sentinel", sim.Action{Kind: "rewrite_world_law", Item: "firearms_disabled", Quantity: 1})
	if len(authTagEvs) != 1 || authTagEvs[0].Kind != "world_law_rewritten" {
		t.Fatalf("expected epoch_arbiter tag to authorize rewrite_world_law, got %+v", authTagEvs)
	}
	if w.EffectiveWorldLaw("dharma_age") != 0 || w.EffectiveWorldLaw("emp_storm_law") != 1 || w.EffectiveWorldLaw("firearms_disabled") != 1 {
		t.Fatalf("expected updated WorldLaws, got %+v", w.WorldLaws)
	}

	// Declarative gameplay enforcement: firearms_disabled=1 blocks firearm crafting & firearm attacks.
	if blockedCraft := w.Apply("warlord", sim.Action{Kind: "craft", RecipeID: "craft_firearm_rifle"}); len(blockedCraft) != 1 || blockedCraft[0].Kind != "action_failed" {
		t.Fatalf("expected firearms_disabled world law to block craft_firearm_rifle, got %+v", blockedCraft)
	}
	if blockedAttack := w.Apply("warlord", sim.Action{Kind: "attack", TargetID: "engineer", Item: "firearm"}); len(blockedAttack) != 1 || blockedAttack[0].Kind != "action_failed" {
		t.Fatalf("expected firearms_disabled world law to block firearm attack, got %+v", blockedAttack)
	}

	// Final lossless JSON round-trip check.
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var restored sim.World
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatalf("World JSON round-trip mismatch in Sci-Fi test")
	}
}

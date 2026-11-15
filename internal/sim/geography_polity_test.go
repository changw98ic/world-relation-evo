package sim_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type fixedOptionSelector struct {
	match func(agent.Option) bool
}

func (s fixedOptionSelector) Select(_ context.Context, _ sim.Perception, opts []agent.Option) (string, error) {
	for _, o := range opts {
		if s.match(o) {
			return o.ID, nil
		}
	}
	return opts[0].ID, nil
}

func TestMultiGenre_MortalWatershed_VenueIsolationAndCrossBorderFlood(t *testing.T) {
	cat := sim.DefaultCatalog()
	w := &sim.World{
		ID:      "mortal-watershed",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "high_peak", Name: "Cloud Ridge Peak", Coord: sim.GeoCoord{X: 0, Y: 20}, ElevationM: 900, Terrain: "mountain"},
				{ID: "upper_gorge", Name: "Upper Qing Gorge", Coord: sim.GeoCoord{X: 0, Y: 10}, ElevationM: 420, Terrain: "valley", Resources: map[string]int64{"soil_fertility": 100}},
				{ID: "lower_plain", Name: "Lower Plain Basin", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 90, Terrain: "plain", Resources: map[string]int64{"soil_fertility": 200}},
			},
			GeoFlows: []sim.GeoFlowEdge{
				// River flows downhill from upper_gorge (420m) -> lower_plain (90m) across county borders.
				{FromRegion: "upper_gorge", ToRegion: "lower_plain", Kind: "river", ConductancePermille: 800},
				// Misconfigured uphill edge (420m -> 900m) must be rejected by river gravity rule.
				{FromRegion: "upper_gorge", ToRegion: "high_peak", Kind: "river", ConductancePermille: 900},
			},
			Districts: []sim.District{
				{ID: "mountain_hermitage", Name: "Peak Hermitage", GeoRegionID: "high_peak", PolityID: "upstream_county", Features: []string{"woodland"}},
				{ID: "upstream_county", Name: "Qingxi County", GeoRegionID: "upper_gorge", PolityID: "upstream_county", Features: []string{"farmland", "market", "yamen"}},
				{ID: "downstream_county", Name: "Pingyuan County", GeoRegionID: "lower_plain", PolityID: "downstream_county", Features: []string{"river_bank", "market"}},
			},
			Roads: []sim.Road{
				{From: "mountain_hermitage", To: "upstream_county", Ticks: 3, EnergyCost: 4},
				{From: "upstream_county", To: "downstream_county", Ticks: 3, EnergyCost: 4},
			},
			Venues: []sim.Venue{
				{ID: "hermit_hut_yard", Name: "Hermit Yard", DistrictID: "mountain_hermitage", GeoRegionID: "high_peak", PolityID: "upstream_county", Kind: "courtyard", Features: []string{"woodland"}},
				{ID: "east_village_paddy", Name: "East Village Rice Paddy", DistrictID: "upstream_county", GeoRegionID: "upper_gorge", PolityID: "upstream_county", Kind: "farmland", Features: []string{"farmland"}},
				{ID: "west_village_teahouse", Name: "West Village Teahouse", DistrictID: "upstream_county", GeoRegionID: "upper_gorge", PolityID: "upstream_county", Kind: "teahouse", Features: []string{"market"}},
				{ID: "river_market_wharf", Name: "Pingyuan River Wharf", DistrictID: "downstream_county", GeoRegionID: "lower_plain", PolityID: "downstream_county", Kind: "wharf", Features: []string{"river_bank", "market"}},
			},
			VenueRoutes: []sim.VenueRoute{
				{FromVenue: "east_village_paddy", ToVenue: "west_village_teahouse", DistanceKm: 4, Ticks: 1, EnergyCost: 2, Medium: "road"},
				{FromVenue: "west_village_teahouse", ToVenue: "river_market_wharf", DistanceKm: 10, Ticks: 2, EnergyCost: 3, Medium: "road"},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{ID: "qingzhou_pref", Name: "Qingzhou Prefecture", Level: "prefecture"},
				{ID: "upstream_county", Name: "Qingxi County", Level: "county", DeJureParentID: "qingzhou_pref"},
				{ID: "downstream_county", Name: "Pingyuan County", Level: "county", DeJureParentID: "qingzhou_pref"},
			},
		},
		DistrictStates: map[string]sim.DistrictState{
			"upstream_county": {
				Weather:   "rain",
				Resources: map[string]int64{"soil_fertility": 100},
			},
			"downstream_county": {
				Weather:        "clear",
				Resources:      map[string]int64{"soil_fertility": 200},
				Infrastructure: map[string]int{"river_levee": 80}, // 48% flood mitigation!
			},
		},
		Actors: []sim.Actor{
			{
				ID: "farmer_a", Name: "Lin", Role: "farmer",
				Tags:        []string{"resident", "trader"},
				Professions: map[string]int{"farmer": 2},
				Location:    "upstream_county", VenueID: "west_village_teahouse",
				Health: 100, Energy: 90, Money: 20,
				Items: map[string]int{"seeds": 3, "water": 3, "food": 5},
				Trust: map[string]int{}, Goal: "tend crops",
			},
			{
				ID: "tea_guest", Name: "Zhao", Role: "peddler",
				Tags:        []string{"resident", "trader"},
				Professions: map[string]int{"peddler": 1},
				Location:    "upstream_county", VenueID: "east_village_paddy",
				Health: 100, Energy: 90, Money: 25,
				Items: map[string]int{"food": 5},
				Trust: map[string]int{}, Goal: "trade goods",
			},
			{
				ID: "wharf_worker", Name: "Sun", Role: "boatman",
				Tags:        []string{"resident"},
				Professions: map[string]int{"boatman": 1},
				Location:    "downstream_county", VenueID: "river_market_wharf",
				Health: 100, Energy: 90, Money: 15,
				Items: map[string]int{"food": 5},
				Trust: map[string]int{}, Goal: "watch the wharf",
			},
			{
				ID: "peak_hermit", Name: "Gu", Role: "herbalist",
				Tags:        []string{"resident"},
				Professions: map[string]int{"herbalist": 2},
				Location:    "mountain_hermitage", VenueID: "hermit_hut_yard",
				Health: 100, Energy: 90, Money: 10,
				Items: map[string]int{"food": 5},
				Trust: map[string]int{}, Goal: "gather herbs",
			},
		},
	}

	// 1. Sub-county venue isolation: both farmer_a and tea_guest are outdoors in upstream_county,
	//    but in different villages/venues -> SamePlace MUST be false and Nearby MUST be empty.
	if sim.SamePlace(w.Actors[0], w.Actors[1]) {
		t.Fatalf("expected outdoor actors in different venues of the same county not to share SamePlace")
	}
	if got := len(w.View("farmer_a").Nearby); got != 0 {
		t.Fatalf("expected 0 nearby actors in west_village_teahouse, got %d", got)
	}
	failSpeak := w.Apply("farmer_a", sim.Action{Kind: "speak", TargetID: "tea_guest", Text: "hello across county"})
	if len(failSpeak) == 0 || failSpeak[0].Kind != "action_failed" {
		t.Fatalf("expected cross-venue speak to fail, got %+v", failSpeak)
	}

	// 2. Venue-scoped recipe features: farmer_a at west_village_teahouse (market only) cannot farm_grain
	//    until moving to east_village_paddy (farmland).
	farmRecipe, ok := w.Catalog.RecipeByID("farm_grain")
	if !ok {
		t.Fatalf("missing farm_grain recipe")
	}
	if ok, _ := sim.CheckRecipe(w.Catalog, w.Map, nil, w.Actors[0], farmRecipe); ok {
		t.Fatalf("expected farm_grain to be rejected at teahouse venue lacking farmland feature")
	}
	moveEvs := w.Apply("farmer_a", sim.Action{Kind: "move_venue", Location: "east_village_paddy"})
	if len(moveEvs) != 1 || moveEvs[0].Kind != "moved_venue" {
		t.Fatalf("expected 1-tick move_venue to complete, got %+v", moveEvs)
	}
	if !sim.SamePlace(w.Actors[0], w.Actors[1]) {
		t.Fatalf("expected farmer_a and tea_guest to share SamePlace after arriving at east_village_paddy")
	}
	if ok, reason := sim.CheckRecipe(w.Catalog, w.Map, nil, w.Actors[0], farmRecipe); !ok {
		t.Fatalf("expected farm_grain to be allowed at east_village_paddy, got %s", reason)
	}

	// Move tea_guest back to west_village_teahouse and put tea_guest to sleep before flood hits,
	// so farmer_a is awake at east_village_paddy while tea_guest sleeps.
	w.Apply("tea_guest", sim.Action{Kind: "move_venue", Location: "west_village_teahouse"})
	w.Actors[1].Energy = 60
	w.Apply("tea_guest", sim.Action{Kind: "sleep", Quantity: 8})

	// 3. Trigger mountain flash flood in upper_gorge: must cascade downstream across county border
	//    into lower_plain (downstream_county), mitigate by river_levee, and NOT flow uphill to high_peak!
	anomEvs := w.TriggerGeoAnomaly(sim.GeoAnomalySpec{
		ID:                "mountain_flash_flood",
		Name:              "Qing Gorge Flash Flood",
		OriginRegionID:    "upper_gorge",
		FlowKind:          "river",
		Intensity:         80,
		DurationTicks:     96,
		MitigatedByInfra:  "river_levee",
		ResourceDamagePct: map[string]int{"soil_fertility": 40},
		ActorHealthDamage: 25,
	})
	if len(anomEvs) != 2 {
		t.Fatalf("expected flood to strike upper_gorge and downstream lower_plain (2 regions, ignoring uphill high_peak), got %d events: %+v", len(anomEvs), anomEvs)
	}
	originEventID := anomEvs[0].ID
	// Downstream levee (condition 80 -> 48% mitigation) reduces raw downstream intensity (80*0.8=64) to ~33.
	downInt, _ := anomEvs[1].Payload["intensity"].(int)
	if downInt >= 64 {
		t.Fatalf("expected downstream river_levee to mitigate flood intensity below 64, got %d", downInt)
	}

	// Verify first-hand witness Observations:
	// - farmer_a (awake in upper_gorge) and wharf_worker (awake in downstream lower_plain) have eyewitness Observation
	// - tea_guest (asleep) and peak_hermit (unaffected high_peak) have ZERO anomaly memories!
	farmerMem := w.Actors[0].Memories[len(w.Actors[0].Memories)-1]
	if farmerMem.Observation == nil || farmerMem.Observation.Kind != "macro_anomaly" || farmerMem.Observation.Modality != "eyewitness" {
		t.Fatalf("expected farmer_a to record eyewitness macro_anomaly Observation, got %+v", farmerMem)
	}
	wharfMem := w.Actors[2].Memories[len(w.Actors[2].Memories)-1]
	if wharfMem.Observation == nil || wharfMem.Observation.Kind != "macro_anomaly" {
		t.Fatalf("expected downstream wharf_worker to record eyewitness macro_anomaly Observation, got %+v", wharfMem)
	}
	for _, m := range w.Actors[1].Memories {
		if m.Kind == "macro_anomaly" {
			t.Fatalf("sleeping actor tea_guest must not receive eyewitness Observation, got %+v", m)
		}
	}
	if len(w.Actors[3].Memories) != 0 {
		t.Fatalf("unaffected uphill peak_hermit must have 0 memories, got %+v", w.Actors[3].Memories)
	}

	// 4. Wake tea_guest, reunite farmer_a with tea_guest at west_village_teahouse, and verify
	//    autonomous speech propagation via Offline() pipeline preserving RootEventID lineage.
	w.Actors[1].Task = nil
	w.Actors[0].Health = 100
	w.Apply("farmer_a", sim.Action{Kind: "move_venue", Location: "west_village_teahouse"})

	decided, err := agent.Offline().Decide(context.Background(), w.View("farmer_a"))
	if err != nil {
		t.Fatalf("Offline().Decide failed: %v", err)
	}
	if decided.Kind != "speak" || decided.TargetID != "tea_guest" || decided.Statement == nil || decided.Statement.Topic != "macro_anomaly" {
		t.Fatalf("expected farmer_a to relay witnessed flood to tea_guest via speak, got %+v", decided)
	}
	w.Apply("farmer_a", decided)
	heard := w.Actors[1].Memories[len(w.Actors[1].Memories)-1]
	if heard.Kind != "speech" || heard.Observation != nil || heard.Statement == nil || heard.Statement.Topic != "macro_anomaly" || heard.RootEventID != originEventID {
		t.Fatalf("expected tea_guest to hold second-hand Statement (Observation==nil) with RootEventID=%d, got %+v", originEventID, heard)
	}
}

func TestMultiGenre_XianxiaSectDomain_DivineSenseSwordFlightAndEpistemicFirewall(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.LocomotionModes = map[string]sim.LocomotionSpec{
		"sword_flight": {
			ID:             "sword_flight",
			Name:           "Flying Sword",
			GrantedByTags:  []string{"flying_sword"},
			AllowedMediums: []string{"road", "river", "air"},
			SpeedPct:       300,
			EnergyPct:      50,
			BypassWeather:  true,
		},
	}
	cat.SensoryModes = map[string]sim.SensorySpec{
		"divine_sense": {
			ID:               "divine_sense",
			Name:             "Nascent Soul Divine Sense",
			GrantedByTags:    []string{"nascent_soul"},
			RangeKm:          20,
			PenetratesIndoor: true,
			PenetrationPower: 50,
		},
	}

	w := &sim.World{
		ID:      "xianxia-sect-domain",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "jade_summit", Name: "Jade Summit", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 1800, AmbientFields: map[string]int{"qi_density": 95}},
				{ID: "spirit_valley", Name: "Outer Spirit Valley", Coord: sim.GeoCoord{X: 12, Y: 0}, ElevationM: 900, AmbientFields: map[string]int{"qi_density": 60}},
				{ID: "mortal_foothills", Name: "Mortal Foothills", Coord: sim.GeoCoord{X: 30, Y: 0}, ElevationM: 150, AmbientFields: map[string]int{"qi_density": 15}},
			},
			Districts: []sim.District{
				{ID: "sect_core", Name: "Azure Cloud Sect Core", GeoRegionID: "jade_summit", PolityID: "azure_cloud_sect", Features: []string{"hub"}},
				{ID: "outer_valley", Name: "Outer Herb Valley", GeoRegionID: "spirit_valley", PolityID: "azure_cloud_sect", Features: []string{"herb_patch"}},
				{ID: "market_town", Name: "Cloudfoot Market Town", GeoRegionID: "mortal_foothills", PolityID: "cloudfoot_town", Features: []string{"market"}},
			},
			Roads: []sim.Road{
				{From: "sect_core", To: "outer_valley", Ticks: 6, EnergyCost: 8},
				{From: "outer_valley", To: "market_town", Ticks: 6, EnergyCost: 8},
			},
			Venues: []sim.Venue{
				{ID: "grand_peak_hall", Name: "Grand Peak Hall", DistrictID: "sect_core", GeoRegionID: "jade_summit", PolityID: "azure_cloud_sect", Kind: "sect_hall"},
				{ID: "herb_terrace", Name: "Spirit Herb Terrace", DistrictID: "outer_valley", GeoRegionID: "spirit_valley", PolityID: "azure_cloud_sect", Kind: "terrace"},
				{ID: "warded_alchemy_vault", Name: "Warded Alchemy Sanctum", DistrictID: "outer_valley", GeoRegionID: "spirit_valley", PolityID: "azure_cloud_sect", Kind: "sanctum", Indoor: true, Shielding: 75},
				{ID: "foot_market_bazaar", Name: "Cloudfoot Bazaar", DistrictID: "market_town", GeoRegionID: "mortal_foothills", PolityID: "cloudfoot_town", Kind: "market", Features: []string{"market"}},
			},
			VenueRoutes: []sim.VenueRoute{
				{FromVenue: "grand_peak_hall", ToVenue: "herb_terrace", DistanceKm: 12, Ticks: 6, EnergyCost: 8, Medium: "road"},
				{FromVenue: "grand_peak_hall", ToVenue: "warded_alchemy_vault", DistanceKm: 12, Ticks: 3, EnergyCost: 4, Medium: "air"},
				{FromVenue: "herb_terrace", ToVenue: "foot_market_bazaar", DistanceKm: 18, Ticks: 6, EnergyCost: 8, Medium: "road"},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{
					ID: "azure_cloud_sect", Name: "Azure Cloud Sect", Level: "sect_domain",
					SeatDistrictID: "sect_core", SeatVenueID: "grand_peak_hall",
					LeaderActorID: "sect_master", Treasury: 120,
				},
				{
					ID: "cloudfoot_town", Name: "Cloudfoot Market Town", Level: "market_town",
					DeJureParentID: "azure_cloud_sect",
					SeatDistrictID: "market_town", SeatVenueID: "foot_market_bazaar",
				},
			},
		},
		DistrictStates: map[string]sim.DistrictState{
			"sect_core": {Weather: "storm"}, // storm adds travel penalty for non-bypassing walkers
		},
		Actors: []sim.Actor{
			{
				ID: "sect_master", Name: "Patriarch Xuan", Role: "scholar",
				Tags:     []string{"resident", "sect_master", "nascent_soul", "flying_sword", "censor"},
				Location: "sect_core", VenueID: "grand_peak_hall",
				Health: 100, Energy: 100, Money: 50,
				Items: map[string]int{"food": 5}, Trust: map[string]int{}, Goal: "govern sect",
			},
			{
				ID: "mortal_servant", Name: "A-Fu", Role: "gatherer",
				Tags:     []string{"resident"},
				Location: "sect_core", VenueID: "grand_peak_hall",
				Health: 100, Energy: 100, Money: 10,
				Items: map[string]int{"food": 5}, Trust: map[string]int{}, Goal: "sweep hall",
			},
			{
				ID: "corrupt_deacon", Name: "Deacon Ma", Role: "constable",
				Tags:        []string{"resident", "law", "trader"},
				Professions: map[string]int{"constable": 2},
				Location:    "market_town", VenueID: "foot_market_bazaar",
				Health: 100, Energy: 90, Money: 20,
				Items: map[string]int{"herb": 6, "food": 4}, Trust: map[string]int{}, Goal: "patrol bazaar",
			},
			{
				ID: "rogue_smuggler", Name: "Black Fox", Role: "thief",
				Tags:        []string{"resident", "thief", "outlaw", "trader"},
				Professions: map[string]int{"thief": 2},
				Location:    "market_town", VenueID: "foot_market_bazaar",
				Health: 100, Energy: 90, Money: 40,
				Items: map[string]int{"food": 4}, Trust: map[string]int{}, Goal: "buy herbs",
			},
		},
	}

	initialCoins := w.TotalCoins()

	// 1. Locomotion decoupling: sword_flight vs mortal walking on the same VenueRoutes.
	masterView := w.View("sect_master")
	servantView := w.View("mortal_servant")
	if len(masterView.VenueRoutes) != 2 {
		t.Fatalf("expected sect_master with flying_sword to access both road and air routes (2), got %d", len(masterView.VenueRoutes))
	}
	if len(servantView.VenueRoutes) != 1 {
		t.Fatalf("expected mortal_servant to access only the road route (air chasm blocked), got %d", len(servantView.VenueRoutes))
	}
	if masterView.VenueRoutes[0].ModeID != "sword_flight" || masterView.VenueRoutes[0].Ticks != 2 {
		t.Fatalf("expected sword_flight 300%% speed to cut 6-tick route to 2 ticks bypassing storm, got %+v", masterView.VenueRoutes[0])
	}
	if servantView.VenueRoutes[0].ModeID != "walk" || servantView.VenueRoutes[0].Ticks <= 6 {
		t.Fatalf("expected mortal_servant walking in storm to take >6 ticks, got %+v", servantView.VenueRoutes[0])
	}

	// 2. Bounded divine_sense vs Warded Formation Shielding:
	//    - spirit_valley (12km <= 20km, shielding 0) -> sensed via divine_sense!
	//    - warded_alchemy_vault (12km <= 20km, shielding 75 > penetration 50) -> blocked!
	//    - foot_market_bazaar (30km > 20km) -> out of range!
	if mod, ok := w.CanSenseVenueRemotely(w.Actors[0], "outer_valley", "herb_terrace"); !ok || mod != "divine_sense" {
		t.Fatalf("expected divine_sense to reach herb_terrace at 12km, got (%q, %v)", mod, ok)
	}
	if mod, ok := w.CanSenseVenueRemotely(w.Actors[0], "outer_valley", "warded_alchemy_vault"); ok {
		t.Fatalf("expected warded_alchemy_vault (shielding 75 > 50) to block divine_sense, got (%q, %v)", mod, ok)
	}
	if mod, ok := w.CanSenseVenueRemotely(w.Actors[0], "market_town", "foot_market_bazaar"); ok {
		t.Fatalf("expected foot_market_bazaar (30km > 20km) to be beyond divine_sense range, got (%q, %v)", mod, ok)
	}

	// Trigger a leyline surge in spirit_valley (12km away): sect_master senses it via divine_sense,
	// while mortal_servant standing right beside him at grand_peak_hall perceives nothing!
	w.TriggerGeoAnomaly(sim.GeoAnomalySpec{
		ID:                "leyline_surge",
		Name:              "Spirit Vein Surge",
		OriginRegionID:    "spirit_valley",
		FlowKind:          "leyline",
		Intensity:         60,
		AmbientFieldDelta: map[string]int{"qi_density": 30},
	})
	if len(w.Actors[0].Memories) != 1 || w.Actors[0].Memories[0].Observation == nil || w.Actors[0].Memories[0].Observation.Modality != "divine_sense" {
		t.Fatalf("expected sect_master to record divine_sense Observation of leyline_surge, got %+v", w.Actors[0].Memories)
	}
	if len(w.Actors[1].Memories) != 0 {
		t.Fatalf("expected mortal_servant at 12km without divine_sense to have 0 memories, got %+v", w.Actors[1].Memories)
	}

	// 3. Jev-selected dynamic decree via Pipeline + subordinate jurisdiction inheritance + M0 conservation.
	pipe := agent.Pipeline{
		Selector: fixedOptionSelector{match: func(o agent.Option) bool {
			return o.Action.Kind == "issue_decree" && o.Action.Item == "market_tax" && o.Action.Quantity == 50
		}},
		Generator: agent.TemplateContent{},
	}
	decreeAct, err := pipe.Decide(context.Background(), w.View("sect_master"))
	if err != nil {
		t.Fatalf("Pipeline.Decide for sect_master failed: %v", err)
	}
	if decreeAct.Kind != "issue_decree" || decreeAct.Text == "" {
		t.Fatalf("expected Pipeline to select issue_decree and populate proclamation Text, got %+v", decreeAct)
	}
	decEvs := w.Apply("sect_master", decreeAct)
	if len(decEvs) != 1 || decEvs[0].Kind != "decree_issued" {
		t.Fatalf("expected decree_issued event, got %+v", decEvs)
	}
	// Subordinate cloudfoot_town inherits the 5% (50 permille) market tax from azure_cloud_sect!
	if got := w.EffectiveMarketTaxPermille("market_town", "foot_market_bazaar"); got != 50 {
		t.Fatalf("expected subordinate cloudfoot_town to inherit 50 permille market tax, got %d", got)
	}

	// Execute a 20-coin herb trade in foot_market_bazaar -> 1 coin market tax collected into polity treasury.
	offerEvs := w.Apply("corrupt_deacon", sim.Action{Kind: "offer", TargetID: "rogue_smuggler", Item: "herb", Quantity: 2, Price: 20})
	offerID := offerEvs[0].ID
	w.Apply("rogue_smuggler", sim.Action{Kind: "accept", OfferID: offerID})
	townNode := w.Polities.NodeByID("cloudfoot_town")
	if townNode.Treasury != 1 {
		t.Fatalf("expected 1 coin market tax remitted to cloudfoot_town treasury, got %d", townNode.Treasury)
	}

	// 4. Epistemic Firewall: rogue_smuggler bribes corrupt_deacon in foot_market_bazaar (30km away, no witnesses).
	//    sect_master governs the parent PolityNode, yet MUST NOT automatically know about the private bribe!
	bribeEvs := w.Apply("rogue_smuggler", sim.Action{Kind: "bribe", TargetID: "corrupt_deacon"})
	if len(bribeEvs) == 0 || bribeEvs[0].Kind != "bribed" {
		t.Fatalf("expected hush bribe to succeed in bazaar, got %+v", bribeEvs)
	}
	if _, hasEv := sim.ActorCorruptionEvidence(w.Actors[0], "corrupt_deacon"); hasEv {
		t.Fatalf("epistemic firewall violated: sect_master gained corruption evidence of remote subordinate bazaar without witnessing or hearing it")
	}

	if afterCoins := w.TotalCoins(); afterCoins != initialCoins {
		t.Fatalf("coin conservation violated: initial=%d after=%d", initialCoins, afterCoins)
	}
}

func TestMultiGenre_SciFiWasteland_DeFactoSecessionBunkerShieldingAndRadioLineage(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.CommChannels = map[string]sim.CommChannelSpec{
		"encrypted_radio": {
			ID:               "encrypted_radio",
			Name:             "Encrypted Tac-Net Radio",
			GrantedByTags:    []string{"radio_tx"},
			ReceiverTags:     []string{"radio_rx"},
			RangeKm:          120,
			Broadcast:        true,
			PenetrationPower: 60,
			EnergyCost:       2,
		},
	}

	w := &sim.World{
		ID:      "scifi-wasteland",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "orbital_anchor", Name: "Sub-Orbital Spire Zone", Coord: sim.GeoCoord{X: 0, Y: 80}, ElevationM: 500, AmbientFields: map[string]int{"radiation": 0}},
				{ID: "crater_wastes", Name: "Glassed Crater Wastes", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 120, AmbientFields: map[string]int{"radiation": 20}},
				{ID: "rebel_basin", Name: "Scrap Basin Sector", Coord: sim.GeoCoord{X: 30, Y: 0}, ElevationM: 100, AmbientFields: map[string]int{"radiation": 5}},
				{ID: "loyal_plateau", Name: "Aegis Dome Plateau", Coord: sim.GeoCoord{X: 60, Y: 0}, ElevationM: 260, AmbientFields: map[string]int{"radiation": 0}},
			},
			GeoFlows: []sim.GeoFlowEdge{
				{FromRegion: "crater_wastes", ToRegion: "rebel_basin", Kind: "wind_corridor", ConductancePermille: 800},
			},
			Districts: []sim.District{
				{ID: "spire_hq", Name: "Orbital Spire HQ", GeoRegionID: "orbital_anchor", PolityID: "orbital_authority", Features: []string{"hub"}},
				{ID: "loyal_dome", Name: "Aegis Loyal Dome", GeoRegionID: "loyal_plateau", PolityID: "loyal_dome_sector", Features: []string{"market"}},
				{ID: "scrap_sector", Name: "Rebel Scrap Sector", GeoRegionID: "rebel_basin", PolityID: "rebel_scrap_sector", Features: []string{"market"}},
			},
			Roads: []sim.Road{
				{From: "spire_hq", To: "loyal_dome", Ticks: 4, EnergyCost: 4},
				{From: "loyal_dome", To: "scrap_sector", Ticks: 4, EnergyCost: 4},
			},
			Venues: []sim.Venue{
				{ID: "command_deck", Name: "Spire Command Deck", DistrictID: "spire_hq", GeoRegionID: "orbital_anchor", PolityID: "orbital_authority", Kind: "command"},
				{ID: "dome_plaza", Name: "Aegis Dome Plaza", DistrictID: "loyal_dome", GeoRegionID: "loyal_plateau", PolityID: "loyal_dome_sector", Kind: "plaza", Features: []string{"market"}},
				{ID: "surface_scrapyard", Name: "Exposed Surface Scrapyard", DistrictID: "scrap_sector", GeoRegionID: "rebel_basin", PolityID: "rebel_scrap_sector", Kind: "outpost", Features: []string{"market"}, Shielding: 0},
				{ID: "sealed_vault_bunker", Name: "Lead-Lined Deep Vault", DistrictID: "scrap_sector", GeoRegionID: "rebel_basin", PolityID: "rebel_scrap_sector", Kind: "bunker", Indoor: true, Shielding: 90},
			},
			VenueRoutes: []sim.VenueRoute{
				{FromVenue: "surface_scrapyard", ToVenue: "sealed_vault_bunker", DistanceKm: 1, Ticks: 1, EnergyCost: 1, Medium: "road"},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{
					ID: "orbital_authority", Name: "Orbital Authority", Level: "orbital_command",
					SeatDistrictID: "spire_hq", SeatVenueID: "command_deck",
					LeaderActorID: "admiral", Treasury: 100,
				},
				{
					ID: "loyal_dome_sector", Name: "Aegis Dome Sector", Level: "surface_sector",
					DeJureParentID: "orbital_authority",
				},
				{
					ID: "iron_raider_front", Name: "Iron Raider Warlord Front", Level: "warlord_faction",
				},
				{
					// Statutory child of orbital_authority, but de-facto controlled by iron_raider_front!
					ID: "rebel_scrap_sector", Name: "Sector-7 Scrap Zone", Level: "surface_sector",
					DeJureParentID:      "orbital_authority",
					DeFactoControllerID: "iron_raider_front",
				},
			},
		},
		Actors: []sim.Actor{
			{
				ID: "admiral", Name: "Admiral Vance", Role: "captain",
				Tags:     []string{"resident", "commander"},
				Location: "spire_hq", VenueID: "command_deck",
				Health: 100, Energy: 100, Money: 50,
				Items: map[string]int{"food": 5}, Trust: map[string]int{}, Goal: "enforce orbital law",
			},
			{
				ID: "loyal_vendor", Name: "Kira", Role: "peddler",
				Tags:     []string{"resident", "trader", "radio_rx"},
				Location: "loyal_dome", VenueID: "dome_plaza",
				Health: 100, Energy: 90, Money: 5, // needy (<12 coins) -> eligible for orbital relief!
				Items: map[string]int{"weapon": 3, "food": 4}, Trust: map[string]int{}, Goal: "trade in dome",
			},
			{
				ID: "loyal_citizen", Name: "Toma", Role: "miner",
				Tags:     []string{"resident", "trader"}, // NO radio_rx tag!
				Location: "loyal_dome", VenueID: "dome_plaza",
				Health: 100, Energy: 90, Money: 30,
				Items: map[string]int{"food": 4}, Trust: map[string]int{}, Goal: "work dome",
			},
			{
				ID: "surface_scout", Name: "Rex", Role: "hunter",
				Tags:     []string{"resident", "trader", "radio_tx", "radio_rx"},
				Location: "scrap_sector", VenueID: "surface_scrapyard",
				Health: 100, Energy: 90, Money: 4, // needy, but in rebel sector -> must NOT get orbital relief!
				Items: map[string]int{"weapon": 3, "food": 4}, Trust: map[string]int{}, Goal: "scout wastes",
			},
			{
				ID: "rebel_buyer", Name: "Nyx", Role: "bandit",
				Tags:     []string{"resident", "trader"},
				Location: "scrap_sector", VenueID: "surface_scrapyard",
				Health: 100, Energy: 90, Money: 30,
				Items: map[string]int{"food": 4}, Trust: map[string]int{}, Goal: "arm rebels",
			},
			{
				ID: "vault_dweller", Name: "Doc Eli", Role: "healer",
				Tags:     []string{"resident", "radio_rx"},
				Location: "scrap_sector", VenueID: "sealed_vault_bunker",
				Health: 100, Energy: 90, Money: 20,
				Items: map[string]int{"food": 5}, Trust: map[string]int{}, Goal: "maintain bunker",
			},
		},
	}

	initialCoins := w.TotalCoins()

	// 1. DeJure vs DeFacto jurisdiction split:
	//    Admiral Vance issues a contraband embargo on "weapon" and a 20-coin relief_grant.
	w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_authority", Item: "embargo:weapon", Quantity: 1})
	w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_authority", Item: "relief_grant", Quantity: 20})

	// Loyal dome obeys orbital embargo & receives relief coins:
	if !w.IsItemEmbargoedAt("loyal_dome", "dome_plaza", "weapon") {
		t.Fatalf("expected loyal_dome_sector to enforce orbital weapon embargo")
	}
	embargoFail := w.Apply("loyal_vendor", sim.Action{Kind: "offer", TargetID: "loyal_citizen", Item: "weapon", Quantity: 1, Price: 6})
	if len(embargoFail) == 0 || embargoFail[0].Kind != "action_failed" {
		t.Fatalf("expected weapon offer in loyal_dome to be blocked by embargo, got %+v", embargoFail)
	}
	if w.Actors[1].Money != 25 { // 5 + 20 relief grant
		t.Fatalf("expected needy loyal_vendor to receive 20 relief coins (5->25), got %d", w.Actors[1].Money)
	}
	if w.Actors[3].Money != 4 {
		t.Fatalf("expected rebel surface_scout NOT to receive orbital relief grant, got %d", w.Actors[3].Money)
	}

	// Rebel scrap sector (DeFactoControllerID = iron_raider_front) ignores orbital weapon embargo!
	if w.IsItemEmbargoedAt("scrap_sector", "surface_scrapyard", "weapon") {
		t.Fatalf("expected breakaway rebel_scrap_sector to ignore statutory orbital embargo")
	}
	rebelOffer := w.Apply("surface_scout", sim.Action{Kind: "offer", TargetID: "rebel_buyer", Item: "weapon", Quantity: 1, Price: 6})
	if len(rebelOffer) != 1 || rebelOffer[0].Kind != "trade_offered" {
		t.Fatalf("expected rebel weapon offer to succeed, got %+v", rebelOffer)
	}
	w.Apply("rebel_buyer", sim.Action{Kind: "accept", OfferID: rebelOffer[0].ID})

	// 2. Radiation storm drifts along wind_corridor from crater_wastes -> rebel_basin:
	//    - Exposed surface_scout takes radiation damage & gets eyewitness Observation
	//    - Sealed bunker vault_dweller (Shielding 90 >= 64) takes 0 damage & gets 0 eyewitness memories!
	radEvs := w.TriggerGeoAnomaly(sim.GeoAnomalySpec{
		ID:                "fallout_plume",
		Name:              "Glass Crater Fallout Storm",
		OriginRegionID:    "crater_wastes",
		FlowKind:          "wind_corridor",
		Intensity:         80,
		AmbientFieldDelta: map[string]int{"radiation": 50},
		ActorHealthDamage: 40,
	})
	if len(radEvs) != 2 {
		t.Fatalf("expected fallout_plume to drift from crater_wastes to rebel_basin (2 regions), got %d", len(radEvs))
	}
	rebelAnomalyEventID := radEvs[1].ID
	rebelReg, _ := w.Map.GeoRegionByID("rebel_basin")
	if rebelReg.AmbientFields["radiation"] <= 5 {
		t.Fatalf("expected rebel_basin ambient radiation to rise after fallout drift, got %d", rebelReg.AmbientFields["radiation"])
	}
	if w.Actors[3].Health >= 100 {
		t.Fatalf("expected exposed surface_scout to suffer radiation health damage, got %d", w.Actors[3].Health)
	}
	if w.Actors[5].Health != 100 || len(w.Actors[5].Memories) != 0 {
		t.Fatalf("expected sealed_vault_bunker (shielding 90) to completely protect vault_dweller and block eyewitnessing, health=%d mem=%+v", w.Actors[5].Health, w.Actors[5].Memories)
	}

	// 3. Encrypted radio broadcast from surface_scout:
	//    - Select transmit_message via Pipeline so TemplateContent generates report from witnessed fallout_plume
	//    - loyal_vendor at dome_plaza (30km <= 120km, has radio_rx, shielding 0 <= 60) receives channel_message (Observation == nil!)
	//    - loyal_citizen at dome_plaza (no radio_rx) receives NOTHING
	//    - vault_dweller in sealed_vault_bunker (has radio_rx, but bunker shielding 90 > penetration 60) receives NOTHING!
	radioPipe := agent.Pipeline{
		Selector: fixedOptionSelector{match: func(o agent.Option) bool {
			return o.Action.Kind == "transmit_message" && o.Action.Channel == "encrypted_radio"
		}},
		Generator: agent.TemplateContent{},
	}
	txAct, err := radioPipe.Decide(context.Background(), w.View("surface_scout"))
	if err != nil {
		t.Fatalf("Pipeline.Decide for radio transmission failed: %v", err)
	}
	txEvs := w.Apply("surface_scout", txAct)
	if len(txEvs) != 1 || txEvs[0].Kind != "channel_transmitted" {
		t.Fatalf("expected channel_transmitted event, got %+v", txEvs)
	}

	gotRadio := false
	for _, m := range w.Actors[1].Memories {
		if m.Kind == "channel_message" {
			gotRadio = true
			if m.Channel != "encrypted_radio" {
				t.Fatalf("expected Channel='encrypted_radio', got %q", m.Channel)
			}
			if m.Observation != nil {
				t.Fatalf("remote radio message MUST NOT populate first-hand Observation, got %+v", m.Observation)
			}
			if m.Statement == nil || m.Statement.Topic != "macro_anomaly" || m.Statement.SubjectID != "fallout_plume" {
				t.Fatalf("expected structured macro_anomaly Statement over radio, got %+v", m.Statement)
			}
			if m.RootEventID != rebelAnomalyEventID {
				t.Fatalf("expected radio message RootEventID=%d to trace back to fallout event, got %d", rebelAnomalyEventID, m.RootEventID)
			}
		}
	}
	if !gotRadio {
		t.Fatalf("expected radio-equipped loyal_vendor within 120km to receive encrypted_radio broadcast")
	}
	for _, m := range w.Actors[2].Memories {
		if m.Kind == "channel_message" {
			t.Fatalf("unequipped loyal_citizen without radio_rx must not receive encrypted_radio message: %+v", m)
		}
	}
	if len(w.Actors[5].Memories) != 0 {
		t.Fatalf("bunker-shielded vault_dweller (shielding 90 > penetration 60) must not receive radio wave: %+v", w.Actors[5].Memories)
	}

	// 4. Verify strict coin conservation & lossless JSON round-trip (reflect.DeepEqual).
	if afterCoins := w.TotalCoins(); afterCoins != initialCoins {
		t.Fatalf("coin conservation violated across decrees and wasteland trades: initial=%d after=%d", initialCoins, afterCoins)
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal(w) failed: %v", err)
	}
	var restored sim.World
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("json.Unmarshal(w) failed: %v", err)
	}
	if !reflect.DeepEqual(w, &restored) {
		t.Fatalf("World JSON round-trip mismatch after Phase 1 three-layer geography & polity additions")
	}
}

func TestMultiGenre_ConcurrentStepWithVenuesAndPolities(t *testing.T) {
	w := sim.Village("test-county-village")
	// Upgrade Village's crossroads into two distinct sub-district Venues (yamen_courtyard and crossroads_square)
	// governed by a county PolityNode, verifying parallel Step() worker goroutines and placeKey venue buckets.
	w.Map.GeoRegions = []sim.GeoRegion{
		{ID: "valley_basin", Name: "Green Valley Basin", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 120},
	}
	for i := range w.Map.Districts {
		w.Map.Districts[i].GeoRegionID = "valley_basin"
		w.Map.Districts[i].PolityID = "green_valley_county"
	}
	w.Map.Venues = []sim.Venue{
		{ID: "yamen_courtyard", Name: "Yamen Courtyard", DistrictID: "crossroads", GeoRegionID: "valley_basin", PolityID: "green_valley_county", Kind: "yamen_hall", Features: []string{"hub", "yamen"}},
		{ID: "crossroads_square", Name: "Crossroads Square", DistrictID: "crossroads", GeoRegionID: "valley_basin", PolityID: "green_valley_county", Kind: "square", Features: []string{"hub", "residential", "academy"}},
	}
	w.Map.VenueRoutes = []sim.VenueRoute{
		{FromVenue: "yamen_courtyard", ToVenue: "crossroads_square", DistanceKm: 1, Ticks: 1, EnergyCost: 1, Medium: "road"},
	}
	w.Polities = &sim.PolityNetwork{
		Nodes: []sim.PolityNode{
			{
				ID: "green_valley_county", Name: "Green Valley County", Level: "county",
				SeatDistrictID: "crossroads", SeatVenueID: "yamen_courtyard",
				LeaderActorID: w.Actors[0].ID, Treasury: 40,
			},
		},
	}
	w.Actors[0].Tags = append(w.Actors[0].Tags, "magistrate")
	for i := range w.Actors {
		if w.Actors[i].Location == "crossroads" && w.Actors[i].InsideHouse == "" {
			if i%2 == 0 {
				w.Actors[i].VenueID = "yamen_courtyard"
			} else {
				w.Actors[i].VenueID = "crossroads_square"
			}
		}
	}

	decider := agent.Offline()
	for step := 0; step < 24; step++ {
		if _, err := w.Step(context.Background(), decider); err != nil {
			t.Fatalf("Step %d failed: %v", step, err)
		}
	}
}


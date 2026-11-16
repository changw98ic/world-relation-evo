package sim_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type countingDecider struct {
	mu           sync.Mutex
	callsByActor map[string]int
	nextAction   func(sim.Perception) sim.Action
}

func (c *countingDecider) calls(actorID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.callsByActor[actorID]
}

func ecFindEvent(events []sim.Event, kind string) *sim.Event {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func (c *countingDecider) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	c.mu.Lock()
	if c.callsByActor == nil {
		c.callsByActor = map[string]int{}
	}
	c.callsByActor[p.Self.ID]++
	c.mu.Unlock()
	if c.nextAction != nil {
		return c.nextAction(p), nil
	}
	return sim.Action{Kind: "wait"}, nil
}

func TestMultiGenre_XianxiaGuardianFormation_SustainAbsorbAndShatter(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.SensoryModes = map[string]sim.SensorySpec{
		"divine_sense": {
			ID:               "divine_sense",
			Name:             "Divine Sense",
			GrantedByTags:    []string{"array_master"},
			RangeKm:          50,
			PenetrationPower: 50,
		},
	}
	cat.CombatProfiles = []sim.CombatProfileSpec{
		{
			ID:            "mortal_blade",
			Name:          "Mortal Blade",
			GrantedByTags: []string{"mortal_fighter"},
			Tier:          1,
			AttackTypes:   map[string]int{"physical": 18},
		},
		{
			ID:            "spirit_fire_caster",
			Name:          "Spirit Fire",
			GrantedByTags: []string{"spirit_fire_caster"},
			Tier:          3,
			AttackTypes:   map[string]int{"spirit_fire": 30},
		},
	}
	cat.ActionSpecs = map[string]sim.ActionSpec{
		"deploy_guardian_array": {
			ID:                   "deploy_guardian_array",
			Name:                 "Deploy Guardian Array",
			GrantedByTags:        []string{"array_master"},
			TargetKind:           "venue",
			RequiredVenueFeature: "spirit_vein",
			SlotCosts:            map[string]int{"qi": 5},
			SpawnEffect: &sim.EffectSpec{
				ID:                        "guardian_array",
				Name:                      "Sect Guardian Array",
				AnchorKind:                "venue",
				DurationTicks:             10,
				MaxIntegrity:              50,
				SustainSlotPerTick:        map[string]int{"qi": 3},
				DefenseByType:             map[string]sim.DefenseRule{"physical": {ImmunityBelowTier: 3}},
				AbsorbDamageWithIntegrity: true,
				ShieldingBonus:            70,
				GrantTags:                 []string{"array_blessed"},
			},
		},
		"array_sanctum_prayer": {
			ID:            "array_sanctum_prayer",
			Name:          "Array Sanctum Blessing",
			GrantedByTags: []string{"array_blessed"},
			TargetKind:    "self",
			SlotGains:     map[string]int{"qi": 2},
		},
	}

	makeWorld := func(initialQi int) *sim.World {
		return &sim.World{
			ID:      "xianxia-array",
			NextID:  1,
			Catalog: cat,
			Map: sim.WorldMap{
				GeoRegions: []sim.GeoRegion{
					{ID: "jade_peak", Name: "Jade Peak", Coord: sim.GeoCoord{X: 0, Y: 0}},
				},
				Districts: []sim.District{
					{ID: "sect_summit", Name: "Sect Summit", GeoRegionID: "jade_peak", Features: []string{"market"}},
					{ID: "outer_ridge", Name: "Outer Ridge", GeoRegionID: "jade_peak", Features: []string{"woodland"}},
				},
				Venues: []sim.Venue{
					{ID: "sanctum_hall", Name: "Sanctum Hall", DistrictID: "sect_summit", GeoRegionID: "jade_peak", Features: []string{"spirit_vein"}, Shielding: 10},
					{ID: "lookout_post", Name: "Lookout Post", DistrictID: "outer_ridge", GeoRegionID: "jade_peak"},
				},
			},
			Actors: []sim.Actor{
				{
					ID: "elder", Name: "Array Elder", Role: "elder",
					Tags: []string{"resident", "fighter", "array_master"}, Location: "sect_summit", VenueID: "sanctum_hall",
					Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
					StateSlots: map[string]int{"qi": initialQi},
					Items:      map[string]int{}, Trust: map[string]int{},
				},
				{
					ID: "bandit", Name: "Mortal Blade", Role: "bandit",
					Tags: []string{"resident", "fighter", "mortal_fighter"}, Location: "sect_summit", VenueID: "sanctum_hall",
					Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
					Items: map[string]int{}, Trust: map[string]int{},
				},
				{
					ID: "mage", Name: "Crimson Cultivator", Role: "mage",
					Tags: []string{"resident", "fighter", "spirit_fire_caster"}, Location: "sect_summit", VenueID: "sanctum_hall",
					Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
					Items: map[string]int{}, Trust: map[string]int{},
				},
				{
					ID: "scout", Name: "Remote Scout", Role: "scout",
					Tags: []string{"resident", "array_master"}, Location: "outer_ridge", VenueID: "lookout_post",
					Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
					Items: map[string]int{}, Trust: map[string]int{},
				},
			},
		}
	}

	// Part A: Deploy Guardian Array via Options -> Apply, test shielding, Tier-1 immunity, and spirit-fire shatter.
	w := makeWorld(20)
	scout := *actorByID(w, "scout")
	if _, canSenseBefore := w.CanSenseVenueRemotely(scout, "sect_summit", "sanctum_hall"); !canSenseBefore {
		t.Fatal("expected remote divine_sense to penetrate unwarded sanctum_hall (shielding 10 <= 50)")
	}

	opts := agent.Options(w.View("elder"))
	var deployOpt *agent.Option
	for i := range opts {
		if opts[i].Action.Kind == "deploy_guardian_array" {
			deployOpt = &opts[i]
			break
		}
	}
	if deployOpt == nil {
		t.Fatalf("expected deploy_guardian_array in agent.Options, got %+v", opts)
	}
	evs := w.Apply("elder", deployOpt.Action)
	if ecFindEvent(evs, "effect_spawned") == nil {
		t.Fatalf("expected effect_spawned event, got %+v", evs)
	}
	if len(w.ActiveEffects) != 1 || w.ActiveEffects[0].Integrity != 50 {
		t.Fatalf("expected 1 active effect with integrity 50, got %+v", w.ActiveEffects)
	}
	if gotQi := actorByID(w, "elder").StateSlots["qi"]; gotQi != 15 {
		t.Fatalf("expected elder qi 15 after 5-qi deployment, got %d", gotQi)
	}

	// ActiveEffect GrantTags should grant "array_sanctum_prayer" capability in both World and Perception.
	if !w.ActorCapabilities(*actorByID(w, "elder"))["array_sanctum_prayer"] {
		t.Fatal("expected ActiveEffect GrantTags to grant array_sanctum_prayer on World")
	}
	if !w.View("elder").Capabilities()["array_sanctum_prayer"] {
		t.Fatal("expected ActiveEffect GrantTags to grant array_sanctum_prayer on Perception")
	}
	// ActiveEffect ShieldingBonus (+70) + Venue.Shielding (10) = 80 > PenetrationPower (50) blocks remote sensing.
	if _, canSenseDuring := w.CanSenseVenueRemotely(scout, "sect_summit", "sanctum_hall"); canSenseDuring {
		t.Fatal("expected Guardian Array shielding bonus (10+70=80) to block remote divine_sense (50)")
	}

	// Tier-1 mortal blade strikes elder -> blocked by ImmunityBelowTier: 3, 0 HP lost, 0 integrity lost.
	mortalEvs := w.Apply("bandit", sim.Action{Kind: "attack", TargetID: "elder"})
	mortalAtk := ecFindEvent(mortalEvs, "attacked")
	if mortalAtk == nil || mortalAtk.Payload["immune_blocked"] != true || mortalAtk.Payload["damage"] != 0 {
		t.Fatalf("expected Tier-1 mortal blade blocked with 0 damage, got %+v", mortalAtk)
	}
	if actorByID(w, "elder").Health != 100 || w.ActiveEffects[0].Integrity != 50 {
		t.Fatalf("expected elder HP=100 and barrier Integrity=50 after Tier-1 attack, got hp=%d int=%d",
			actorByID(w, "elder").Health, w.ActiveEffects[0].Integrity)
	}

	// Tier-3 spirit fire hit #1 (30 dmg): absorbed completely by barrier (50 -> 20).
	fire1 := w.Apply("mage", sim.Action{Kind: "attack", TargetID: "elder"})
	atk1 := ecFindEvent(fire1, "attacked")
	if atk1 == nil || atk1.Payload["damage"] != 0 || atk1.Payload["absorbed"] != 30 {
		t.Fatalf("expected first spirit-fire hit absorbed (30), got %+v", atk1)
	}
	if actorByID(w, "elder").Health != 100 || w.ActiveEffects[0].Integrity != 20 {
		t.Fatalf("expected elder HP=100 and barrier Integrity=20, got hp=%d int=%d",
			actorByID(w, "elder").Health, w.ActiveEffects[0].Integrity)
	}

	// Tier-3 spirit fire hit #2 (30 dmg): absorbs 20, shatters barrier (effect_broken), deals 10 HP damage!
	fire2 := w.Apply("mage", sim.Action{Kind: "attack", TargetID: "elder"})
	if ecFindEvent(fire2, "effect_broken") == nil {
		t.Fatalf("expected effect_broken event when barrier shattered, got %+v", fire2)
	}
	atk2 := ecFindEvent(fire2, "attacked")
	if atk2 == nil || atk2.Payload["damage"] != 10 || atk2.Payload["absorbed"] != 20 {
		t.Fatalf("expected second spirit-fire hit to deal 10 damage after 20 absorbed, got %+v", atk2)
	}
	if actorByID(w, "elder").Health != 90 {
		t.Fatalf("expected elder HP=90 after barrier shatter, got %d", actorByID(w, "elder").Health)
	}
	if w.ActiveEffects != nil {
		t.Fatalf("expected ActiveEffects reset to nil after shatter, got %+v", w.ActiveEffects)
	}

	// Part B: Sustain exhaustion when qi hits 0.
	w2 := makeWorld(9) // 5 to deploy -> 4 remaining; sustains 1 tick (4 -> 1), then collapses next tick!
	w2.Apply("elder", sim.Action{Kind: "deploy_guardian_array", Location: "sanctum_hall"})
	if len(w2.ActiveEffects) != 1 {
		t.Fatalf("expected active effect spawned in w2")
	}
	idleDecider := &countingDecider{}
	evs1, err := w2.Step(context.Background(), idleDecider)
	if err != nil {
		t.Fatalf("step 1 error: %v", err)
	}
	if ecFindEvent(evs1, "effect_exhausted") != nil || len(w2.ActiveEffects) != 1 {
		t.Fatalf("expected effect still sustained after tick 1 with 1 qi left, got active=%d evs=%+v", len(w2.ActiveEffects), evs1)
	}
	if got := actorByID(w2, "elder").StateSlots["qi"]; got != 1 {
		t.Fatalf("expected elder qi=1 after 1 sustain tick, got %d", got)
	}
	evs2, err := w2.Step(context.Background(), idleDecider)
	if err != nil {
		t.Fatalf("step 2 error: %v", err)
	}
	if ecFindEvent(evs2, "effect_exhausted") == nil {
		t.Fatalf("expected effect_exhausted when qi fell below SustainSlotPerTick (1 < 3), got %+v", evs2)
	}
	if w2.ActiveEffects != nil {
		t.Fatalf("expected ActiveEffects reset to nil after exhaustion, got %+v", w2.ActiveEffects)
	}
}

func TestMultiGenre_SciFiMech_KineticImmunityVsEMPVulnerability(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.CombatProfiles = []sim.CombatProfileSpec{
		{
			ID:            "kinetic_rifle",
			Name:          "Kinetic Rifle",
			GrantedByTags: []string{"rifle_trooper"},
			Tier:          1,
			AttackTypes:   map[string]int{"kinetic": 25},
		},
		{
			ID:                "heavy_mech",
			Name:              "Heavy Mech Chassis",
			GrantedByTags:     []string{"heavy_mech"},
			Tier:              3,
			DefenseByType:     map[string]sim.DefenseRule{"kinetic": {ImmunityBelowTier: 3}},
			CohortScaleDamage: 20,
		},
	}
	cat.ActionSpecs = map[string]sim.ActionSpec{
		"fire_emp_warhead": {
			ID:            "fire_emp_warhead",
			Name:          "Fire EMP Warhead",
			GrantedByTags: []string{"emp_gunner"},
			TargetKind:    "actor",
			EnergyCost:    8,
			Attack: &sim.AttackSpec{
				Tier:         3,
				DamageByType: map[string]int{"emp": 40},
			},
		},
		"mech_barrage": {
			ID:            "mech_barrage",
			Name:          "Mech Rocket Barrage",
			GrantedByTags: []string{"heavy_mech"},
			TargetKind:    "cohort",
			EnergyCost:    10,
			Attack: &sim.AttackSpec{
				Tier:             3,
				AreaScope:        "cohort",
				CohortCasualties: 15,
			},
		},
	}

	w := &sim.World{
		ID:      "scifi-mech",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			Districts: []sim.District{
				{ID: "sector_7", Name: "Sector 7", Features: []string{"market"}},
			},
		},
		Cohorts: []sim.PopulationCohort{
			{
				ID: "raider_squad", Name: "Scrap Raiders", District: "sector_7",
				Profession: "bandit", Tier: 1, Population: 100,
			},
		},
		Actors: []sim.Actor{
			{
				ID: "trooper", Name: "Rifle Trooper", Role: "guard",
				Tags: []string{"resident", "fighter", "rifle_trooper"}, Location: "sector_7",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
			{
				ID: "emp_spec", Name: "EMP Specialist", Role: "guard",
				Tags: []string{"resident", "emp_gunner"}, Location: "sector_7",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
			{
				ID: "mech", Name: "Titan Mech-01", Role: "mech",
				Tags: []string{"resident", "fighter", "heavy_mech"}, Location: "sector_7",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
		},
	}

	// Tier-1 kinetic rifle vs Tier-3 Heavy Mech -> 0 damage, immune_blocked=true.
	kinEvs := w.Apply("trooper", sim.Action{Kind: "attack", TargetID: "mech"})
	kinAtk := ecFindEvent(kinEvs, "attacked")
	if kinAtk == nil || kinAtk.Payload["immune_blocked"] != true || kinAtk.Payload["damage"] != 0 {
		t.Fatalf("expected Tier-1 kinetic attack immune-blocked against Heavy Mech, got %+v", kinAtk)
	}
	if actorByID(w, "mech").Health != 100 {
		t.Fatalf("expected mech HP=100 after kinetic hit, got %d", actorByID(w, "mech").Health)
	}

	// EMP warhead vs Heavy Mech -> full 40 damage.
	empOpts := agent.Options(w.View("emp_spec"))
	foundEMP := false
	for _, o := range empOpts {
		if o.Action.Kind == "fire_emp_warhead" && o.Action.TargetID == "mech" {
			foundEMP = true
			break
		}
	}
	if !foundEMP {
		t.Fatalf("expected fire_emp_warhead option targeting mech in agent.Options")
	}
	empEvs := w.Apply("emp_spec", sim.Action{Kind: "fire_emp_warhead", TargetID: "mech"})
	empAtk := ecFindEvent(empEvs, "attacked")
	if empAtk == nil || empAtk.Payload["damage"] != 40 || empAtk.Payload["immune_blocked"] != false {
		t.Fatalf("expected EMP warhead to deal full 40 damage, got %+v", empAtk)
	}
	if actorByID(w, "mech").Health != 60 {
		t.Fatalf("expected mech HP=60 after EMP warhead, got %d", actorByID(w, "mech").Health)
	}

	// Cross-scale cohort strike via agent.Options -> World.Apply preserves population + deaths conservation.
	initPop := w.TotalPopulation()
	mechOpts := agent.Options(w.View("mech"))
	foundCohortOpt := false
	for _, o := range mechOpts {
		if o.Action.Kind == "mech_barrage" && o.Action.TargetID == "raider_squad" {
			foundCohortOpt = true
			break
		}
	}
	if !foundCohortOpt {
		t.Fatalf("expected mech_barrage option targeting raider_squad in agent.Options")
	}
	barrageEvs := w.Apply("mech", sim.Action{Kind: "mech_barrage", TargetID: "raider_squad"})
	struck := ecFindEvent(barrageEvs, "cohort_struck")
	if struck == nil {
		t.Fatalf("expected cohort_struck event, got %+v", barrageEvs)
	}
	if w.TotalPopulation()+w.TotalDeaths() != initPop {
		t.Fatalf("population conservation violated: pop=%d deaths=%d want=%d", w.TotalPopulation(), w.TotalDeaths(), initPop)
	}
}

func TestMultiGenre_PinnedHighTierDormantSeclusion_ZeroModelCallsAndNoDehydration(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.CombatProfiles = []sim.CombatProfileSpec{
		{
			ID:            "nascent_soul",
			Name:          "Nascent Soul Patriarch",
			GrantedByTags: []string{"nascent_soul"},
			Tier:          3,
		},
	}
	cat.ActionSpecs = map[string]sim.ActionSpec{
		"closed_door_seclusion": {
			ID:            "closed_door_seclusion",
			Name:          "Closed-Door Seclusion",
			GrantedByTags: []string{"nascent_soul"},
			TargetKind:    "self",
			Ticks:         20,
			Dormant:       true,
			SlotGains:     map[string]int{"qi": 25},
		},
	}

	w := &sim.World{
		ID:      "xianxia-seclusion",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "azure_reg", Name: "Azure Region", Coord: sim.GeoCoord{X: 0, Y: 0}},
			},
			Districts: []sim.District{
				{ID: "azure_peak", Name: "Azure Peak", GeoRegionID: "azure_reg", Features: []string{"water_source"}},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{ID: "azure_sect", Name: "Azure Sect", LeaderActorID: "sect_leader"},
			},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "outer_disciples", Name: "Outer Disciples", District: "azure_peak", Profession: "farmer", Tier: 1, Population: 50},
		},
		Actors: []sim.Actor{
			{
				ID: "patriarch", Name: "Patriarch Han", Role: "farmer",
				Tags: []string{"resident", "nascent_soul"}, Location: "azure_peak",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
			{
				ID: "pinned_guest", Name: "Pinned Envoy", Role: "farmer",
				PinFocal: true,
				Tags:     []string{"resident"}, Location: "azure_peak",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
			{
				ID: "sect_leader", Name: "Sect Master Lin", Role: "farmer",
				Tags: []string{"resident"}, Location: "azure_peak",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
			{
				ID: "assassin", Name: "Shadow Blade", Role: "bandit",
				Tags: []string{"resident", "fighter"}, Location: "azure_peak",
				Health: 100, Energy: 100, SleepTrait: sim.SleepEarlyBird,
				Items: map[string]int{}, Trust: map[string]int{},
			},
		},
	}

	// 1. Pinned focal retention: Tier>=3, PinFocal, and Polity Leader cannot be dehydrated.
	for _, pinnedID := range []string{"patriarch", "pinned_guest", "sect_leader"} {
		a := *actorByID(w, pinnedID)
		if !w.IsActorPinnedFocal(a) {
			t.Fatalf("expected IsActorPinnedFocal(%s) == true", pinnedID)
		}
		if err := w.DehydrateActor(pinnedID); err == nil {
			t.Fatalf("expected DehydrateActor(%s) to fail for pinned actor", pinnedID)
		}
	}
	if w.IsActorPinnedFocal(*actorByID(w, "assassin")) {
		t.Fatal("expected unpinned Tier-1 non-leader actor not to be pinned focal")
	}

	// 2. Dormant 20-tick seclusion: 1 Decide call to start on tick 0, then 0 Decide calls during ticks 1..19.
	decider := &countingDecider{
		nextAction: func(p sim.Perception) sim.Action {
			if p.Self.ID == "patriarch" && p.Tick == 0 {
				return sim.Action{Kind: "closed_door_seclusion"}
			}
			return sim.Action{Kind: "wait"}
		},
	}
	evs0, err := w.Step(context.Background(), decider)
	if err != nil {
		t.Fatalf("tick 0 step error: %v", err)
	}
	if ecFindEvent(evs0, "dormant_started") == nil {
		t.Fatalf("expected dormant_started event on tick 0, got %+v", evs0)
	}
	if pat := actorByID(w, "patriarch"); pat.Task == nil || !pat.Task.Dormant || pat.Task.Remaining != 19 {
		t.Fatalf("expected patriarch in dormant task with 19 remaining ticks, got %+v", pat.Task)
	}

	// Verify reflect.DeepEqual JSON round-trip while Dormant task is active (and empty StateSlots/Currencies/ActiveEffects are nil).
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json marshal world: %v", err)
	}
	var decoded sim.World
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json unmarshal world: %v", err)
	}
	decoded.Catalog = sim.CloneCatalog(decoded.Catalog)
	if !reflect.DeepEqual(w, &decoded) {
		t.Fatalf("JSON round-trip mismatch while dormant:\ngot:  %+v\nwant: %+v", &decoded, w)
	}

	callsAfterStart := decider.calls("patriarch")
	if callsAfterStart != 1 {
		t.Fatalf("expected 1 decider call for patriarch on start tick, got %d", callsAfterStart)
	}
	var finalEvs []sim.Event
	for step := 0; step < 19; step++ {
		evs, err := w.Step(context.Background(), decider)
		if err != nil {
			t.Fatalf("seclusion step %d error: %v", step, err)
		}
		finalEvs = append(finalEvs, evs...)
	}
	if decider.calls("patriarch") != 1 {
		t.Fatalf("expected 0 Decider calls during 19 dormant ticks, total calls=%d", decider.calls("patriarch"))
	}
	if pat := actorByID(w, "patriarch"); pat.Task != nil || pat.StateSlots["qi"] != 25 {
		t.Fatalf("expected seclusion finished with task=nil and qi=25, got task=%+v slots=%+v", pat.Task, pat.StateSlots)
	}
	if ecFindEvent(finalEvs, "action_resolved") == nil {
		t.Fatalf("expected action_resolved on seclusion completion")
	}

	// 3. Early wake on combat damage (dmg > 0): interrupts dormant task immediately and resumes Decide on next tick.
	w.Apply("patriarch", sim.Action{Kind: "closed_door_seclusion"})
	if actorByID(w, "patriarch").Task == nil || !actorByID(w, "patriarch").Task.Dormant {
		t.Fatal("expected patriarch back in dormant task")
	}
	atkEvs := w.Apply("assassin", sim.Action{Kind: "attack", TargetID: "patriarch"})
	if ecFindEvent(atkEvs, "dormant_interrupted") == nil {
		t.Fatalf("expected dormant_interrupted event on combat hit, got %+v", atkEvs)
	}
	if actorByID(w, "patriarch").Task != nil {
		t.Fatalf("expected patriarch task cleared after damage, got %+v", actorByID(w, "patriarch").Task)
	}
	beforeWakeCalls := decider.calls("patriarch")
	if _, err := w.Step(context.Background(), decider); err != nil {
		t.Fatalf("post-interrupt step error: %v", err)
	}
	if decider.calls("patriarch") != beforeWakeCalls+1 {
		t.Fatalf("expected patriarch to resume Decide after dormant interruption")
	}

	// 4. Early wake on local disaster / geo-anomaly damaging actor in their district.
	w.Apply("patriarch", sim.Action{Kind: "closed_door_seclusion"})
	if actorByID(w, "patriarch").Task == nil || !actorByID(w, "patriarch").Task.Dormant {
		t.Fatal("expected patriarch in dormant task before disaster")
	}
	disEvs := w.TriggerGeoAnomaly(sim.GeoAnomalySpec{
		ID:                "spirit_quake",
		Name:              "Spirit Vein Tremor",
		OriginRegionID:    "azure_reg",
		Intensity:         50,
		ActorHealthDamage: 10,
	})
	if ecFindEvent(disEvs, "dormant_interrupted") == nil {
		t.Fatalf("expected dormant_interrupted event when local disaster hits dormant actor, got %+v", disEvs)
	}
	if actorByID(w, "patriarch").Task != nil {
		t.Fatalf("expected patriarch task cleared after disaster, got %+v", actorByID(w, "patriarch").Task)
	}
}

func TestCrossPhase_ActionSpecSynergiesAndConservation(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.StateSlots = map[string]sim.StateSlotSpec{
		"qi": {ID: "qi", Name: "Spiritual Qi", Min: 0, Max: 100, Default: 50},
	}
	cat.Forms = map[string]sim.FormSpec{
		"mortal":   {ID: "mortal", Name: "Mortal"},
		"ascended": {ID: "ascended", Name: "Ascended", InnateTags: []string{"immortal"}},
	}
	cat.Transitions = map[string]sim.TransitionSpec{
		"breakthrough": {
			ID:                  "breakthrough",
			Name:                "Realm Ascension",
			RequiredFormID:      "mortal",
			BaseSuccessPermille: 1000,
			SuccessFormID:       "ascended",
		},
	}
	cat.Currencies = map[string]sim.CurrencySpec{
		"sect_merit": {ID: "sect_merit", Name: "Sect Merit"},
	}
	cat.DefaultWorldLaws = map[string]int{"qi_cap": 100}
	cat.ActionSpecs = map[string]sim.ActionSpec{
		"ascension_rite": {
			ID:                   "ascension_rite",
			Name:                 "Ascension Rite",
			GrantedByTags:        []string{"cultivator"},
			TargetKind:           "self",
			RequiredVenueFeature: "powered_altar",
			MoneyCost:            10,
			CurrencyCosts:        map[string]int{"sect_merit": 25},
			ItemCosts:            map[string]int{"herb": 1},
			SlotGains:            map[string]int{"qi": 80},
			TransitionID:         "breakthrough",
			AllowRewriteWorldLaw: true,
			WorldLawDelta:        map[string]int{"qi_cap": 200},
		},
	}

	w := &sim.World{
		ID:      "cross-phase",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			Districts: []sim.District{{ID: "peak", Name: "Peak", Features: []string{"hub"}}},
			Venues: []sim.Venue{
				{ID: "altar", Name: "Spirit Altar", DistrictID: "peak", Kind: "sanctum", Features: []string{"powered_altar"}},
			},
		},
		Actors: []sim.Actor{
			{
				ID: "adept", Name: "Adept Lin", Role: "cultivator",
				Tags:       []string{"resident", "cultivator"},
				Location:   "peak",
				VenueID:    "altar",
				Health:     100,
				Energy:     100,
				Money:      50,
				FormID:     "mortal",
				StateSlots: map[string]int{"qi": 60},
				Currencies: map[string]int64{"sect_merit": 40},
				Items:      map[string]int{"herb": 2, "food": 2},
				Trust:      map[string]int{},
			},
		},
		FlowGrid: &sim.FlowGridState{
			Nodes: []sim.FlowNode{
				{ID: "gen", VenueID: "altar", DistrictID: "peak", Kind: "generator", Generation: 50, EnergyCapacity: 50, StoredEnergy: 50, Active: false},
				{ID: "altar_node", VenueID: "altar", DistrictID: "peak", Kind: "consumer", Demand: 20, Active: true},
			},
			Edges: []sim.FlowEdge{
				{ID: "e1", FromNodeID: "gen", ToNodeID: "altar_node", Capacity: 50},
			},
		},
	}
	w.RecomputeFlowGrid()
	w.InitLedgerAudit()

	// 1. Unpowered FlowGrid blocks declarative ActionSpec requiring venue feature.
	failEvs := w.Apply("adept", sim.Action{Kind: "ascension_rite"})
	if ecFindEvent(failEvs, "action_failed") == nil {
		t.Fatalf("expected action_failed when venue FlowGrid node is unpowered, got %+v", failEvs)
	}

	// 2. Power the FlowGrid and run the rite: verifies ledger conservation, clamping, TransitionID, and WorldLawDelta.
	w.FlowGrid.Nodes[0].Active = true
	w.RecomputeFlowGrid()
	okEvs := w.Apply("adept", sim.Action{Kind: "ascension_rite"})
	if ecFindEvent(okEvs, "action_failed") != nil {
		t.Fatalf("expected ascension_rite to succeed once powered, got %+v", okEvs)
	}
	adept := actorByID(w, "adept")
	if adept.FormID != "ascended" {
		t.Fatalf("expected TransitionID breakthrough to set FormID=ascended, got %q", adept.FormID)
	}
	if adept.StateSlots["qi"] != 100 {
		t.Fatalf("expected qi clamped at StateSlots['qi'].Max=100, got %d", adept.StateSlots["qi"])
	}
	if w.EffectiveWorldLaw("qi_cap") != 200 {
		t.Fatalf("expected WorldLawDelta to update qi_cap to 200, got %+v", w.WorldLaws)
	}
	if adept.Money != 40 || w.PublicTreasury != 0 || adept.Currencies["sect_merit"] != 15 || adept.Items["herb"] != 1 {
		t.Fatalf("unexpected post-rite balances: money=%d treasury=%d merit=%d herb=%d", adept.Money, w.PublicTreasury, adept.Currencies["sect_merit"], adept.Items["herb"])
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatalf("expected 0-error LedgerAudit conservation after ActionSpec costs, got %v", err)
	}
}

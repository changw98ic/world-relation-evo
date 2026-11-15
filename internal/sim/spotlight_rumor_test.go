package sim_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func srTestFindMemoryByEventID(a sim.Actor, eventID int64) (sim.Memory, bool) {
	for _, m := range a.Memories {
		if m.EventID == eventID {
			return m, true
		}
	}
	return sim.Memory{}, false
}

func TestSpotlightRumorDynamicHydrationPanicAndGossipLineage(t *testing.T) {
	// 1. Dynamic Cohort Spotlight Promotion & Demotion + Conservation Guards
	w := sim.LargeWorld("sr-dynamic", 1_000_000)
	sr := sim.NewSpotlightRumorState()
	sr.Spec.MaxFocalActors = 123
	sr.Spec.TargetFocalActors = 121
	sr.Spec.DailyPromotionCap = 3
	sr.Spec.DailyDemotionCap = 3

	initPop := w.TotalPopulation()
	initCoins := w.TotalCoins()
	initWounded := w.TotalWounded()
	initSick := w.TotalSick()
	initJailed := w.TotalJailed()
	initTreasury := w.PublicTreasury

	// Feed macro drama events to heat specific cohorts.
	w.Tick = sim.TicksPerDay
	drama := []sim.Event{
		{
			ID:      w.NextID,
			Tick:    w.Tick - 10,
			ActorID: "cohort_crossroads_scholars",
			Kind:    "cohort_promoted",
			Payload: map[string]any{
				"from_cohort_id": "cohort_crossroads_scholars",
				"to_cohort_id":   "cohort_crossroads_xiucai",
				"promoted_count": 4,
			},
		},
		{
			ID:       w.NextID + 1,
			Tick:     w.Tick - 8,
			ActorID:  "censorate",
			TargetID: "cohort_market_gangsters",
			Kind:     "cohort_corruption_purged",
			Action:   sim.Action{Kind: "audit", Location: "market_street"},
			Payload: map[string]any{
				"text":              "Censorate purged market_street graft",
				"oversight_cohort":  "cohort_crossroads_constables",
				"corrupt_cohort":    "cohort_market_gangsters",
				"confiscated_coins": 40,
			},
		},
		{
			ID:       w.NextID + 2,
			Tick:     w.Tick - 5,
			ActorID:  "cohort_market_gangsters",
			TargetID: "cohort_market_peddlers",
			Kind:     "cohort_racket_clashed",
			Action:   sim.Action{Kind: "extort", Location: "market_street", Item: "food", Quantity: 5},
			Payload: map[string]any{
				"text":          "Market peddlers resisted extortion",
				"gang_cohort":   "cohort_market_gangsters",
				"victim_cohort": "cohort_market_peddlers",
				"smashed_item":  "food",
				"smashed_qty":   5,
			},
		},
	}
	w.NextID += 3

	obsEvs := w.ObserveSpotlightAndRumors(sr, drama)
	if sr.CohortHeat["cohort_crossroads_xiucai"] <= 0 || sr.CohortHeat["cohort_market_gangsters"] <= 0 {
		t.Fatalf("expected cohort heat to rise from macro events, got %v", sr.CohortHeat)
	}
	if len(obsEvs) != 0 {
		t.Fatalf("expected macro events not to inject synthetic gossip events into focal actors, got %+v", obsEvs)
	}

	// Day 1 spotlight step: promotes 3 top-heat figures (120 -> 123) without injecting synthetic memories.
	promEvs := w.StepDynamicSpotlight(sr)
	promotedCount := 0
	var promotedIDs []string
	for _, ev := range promEvs {
		if ev.Kind == "spotlight_promoted" {
			promotedCount++
			promotedIDs = append(promotedIDs, ev.ActorID)
		}
	}
	if promotedCount != 3 {
		t.Fatalf("expected 3 spotlight_promoted events, got %d (%+v)", promotedCount, promEvs)
	}
	for _, id := range promotedIDs {
		a := actorByID(w, id)
		if a == nil || a.Goal == "" {
			t.Fatalf("promoted actor %s missing vivid goal: %+v", id, a)
		}
		if len(a.Memories) != 0 {
			t.Fatalf("promoted actor %s should not start with system-injected memories: %+v", id, a.Memories)
		}
	}

	// Verify conservation after promotion.
	if w.TotalPopulation() != initPop || w.TotalCoins() != initCoins || w.TotalWounded() != initWounded ||
		w.TotalSick() != initSick || w.TotalJailed() != initJailed || w.PublicTreasury != initTreasury {
		t.Fatalf("conservation broken after promotion: pop=%d/%d coins=%d/%d wounded=%d/%d sick=%d/%d jailed=%d/%d treasury=%d/%d",
			w.TotalPopulation(), initPop, w.TotalCoins(), initCoins, w.TotalWounded(), initWounded,
			w.TotalSick(), initSick, w.TotalJailed(), initJailed, w.PublicTreasury, initTreasury)
	}

	// Protect two promoted actors with conservation blockers (wounded / open offer) and verify only the clean idle one can demote!
	blockedWounded := promotedIDs[0]
	blockedOffer := promotedIDs[1]
	eligibleIdle := promotedIDs[2]
	if a := actorByID(w, blockedWounded); a != nil {
		a.Health = sim.MaxHealth - 10
		a.WoundedTracked = true
	}
	if a := actorByID(w, blockedOffer); a != nil {
		w.Offers = append(w.Offers, sim.Offer{
			ID: 999999, Seller: a.ID, Buyer: "p01", Item: "food", Quantity: 1, Price: 1, Expires: w.Tick + 50,
		})
	}
	// Make eligibleIdle have lowest salience so it rotates out, and boost another cohort for Day 2 promotion.
	sr.ActorSalience[eligibleIdle] = 1
	sr.CohortHeat["cohort_east_martial_artists"] = 90
	w.Tick = sim.TicksPerDay * 2

	rotEvs := w.StepDynamicSpotlight(sr)
	var demotedIDs []string
	var day2Promoted []string
	for _, ev := range rotEvs {
		if ev.Kind == "spotlight_demoted" {
			demotedIDs = append(demotedIDs, ev.ActorID)
		} else if ev.Kind == "spotlight_promoted" {
			day2Promoted = append(day2Promoted, ev.ActorID)
		}
	}
	if len(demotedIDs) == 0 || demotedIDs[0] != eligibleIdle {
		t.Fatalf("expected clean idle actor %s to be demoted first, got demoted=%v", eligibleIdle, demotedIDs)
	}
	if actorByID(w, blockedWounded) == nil || actorByID(w, blockedOffer) == nil {
		t.Fatalf("protected wounded or offer-holding actors were dehydrated")
	}
	if len(day2Promoted) == 0 {
		t.Fatalf("expected new hot cohort promotion after demotion slot opened, got %+v", rotEvs)
	}
	// Core baseline p01..p30 must remain intact.
	for i := 1; i <= 30; i++ {
		id := "p" + string([]byte{byte('0' + i/10), byte('0' + i%10)})
		if actorByID(w, id) == nil {
			t.Fatalf("core actor %s was dehydrated", id)
		}
	}

	// Restore test perturbations so conservation baseline remains exact before multi-day run.
	if a := actorByID(w, blockedWounded); a != nil {
		a.Health = sim.MaxHealth
		a.WoundedTracked = false
	}
	w.Offers = nil
	if w.TotalPopulation() != initPop || w.TotalCoins() != initCoins || w.TotalWounded() != initWounded ||
		w.TotalSick() != initSick || w.TotalJailed() != initJailed || w.PublicTreasury != initTreasury {
		t.Fatalf("conservation broken after rotation: pop=%d/%d coins=%d/%d wounded=%d/%d sick=%d/%d jailed=%d/%d treasury=%d/%d",
			w.TotalPopulation(), initPop, w.TotalCoins(), initCoins, w.TotalWounded(), initWounded,
			w.TotalSick(), initSick, w.TotalJailed(), initJailed, w.PublicTreasury, initTreasury)
	}

	// 2. Micro -> Macro Rumor Panic Resonance
	s1, s2 := actorByID(w, "p27"), actorByID(w, "p28")
	s1.Location, s1.InsideHouse, s1.Task, s1.JailedUntil = "market_street", "", nil, 0
	s2.Location, s2.InsideHouse, s2.Task, s2.JailedUntil = "market_street", "", nil, 0
	moraleBefore := w.DistrictStates["market_street"].MoraleBonus

	sp1 := w.Apply(s1.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s2.ID,
		Text:      "Grain shortage is coming to Market Street!",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street", Quantity: 0},
	})
	if len(sp1) == 0 || sp1[0].Kind != "speech" {
		t.Fatalf("first rumor speech failed: %+v", sp1)
	}
	panicCheck1 := w.ObserveSpotlightAndRumors(sr, sp1)
	for _, ev := range panicCheck1 {
		if ev.Kind == "rumor_panic_triggered" {
			t.Fatalf("single normal speaker should not trigger panic before corroboration: %+v", panicCheck1)
		}
	}

	// Second distinct speaker corroborates the shortage rumor.
	sp2 := w.Apply(s2.ID, sim.Action{
		Kind:      "speak",
		TargetID:  s1.ID,
		Text:      "Yes, everyone says food stocks are drying up!",
		Statement: &sim.Statement{Topic: "shortage", Item: "food", Location: "market_street", Quantity: 0},
	})
	if len(sp2) == 0 || sp2[0].Kind != "speech" {
		t.Fatalf("second rumor speech failed: %+v", sp2)
	}
	panicCheck2 := w.ObserveSpotlightAndRumors(sr, sp2)
	panicEv := lastOfKind(panicCheck2, "rumor_panic_triggered")
	if panicEv.ID == 0 {
		t.Fatalf("expected rumor_panic_triggered after corroboration, got %+v", panicCheck2)
	}
	if boost := w.DistrictPanicDemandPermille(sr, "market_street", "food"); boost <= 0 {
		t.Fatalf("expected positive panic hoarding demand boost, got %d", boost)
	}
	if w.DistrictStates["market_street"].MoraleBonus >= moraleBefore {
		t.Fatalf("expected district morale to shift downward on panic: before=%d after=%d", moraleBefore, w.DistrictStates["market_street"].MoraleBonus)
	}

	// 3. Epistemic Guard: Macro Disaster Does NOT Inject Memories; Witness -> Speak -> Listener Preserves Lineage
	hubA := jrTestFindActorByRole(w, "gangster")
	hubB := actorByID(w, "p25")
	hubC := jrTestFindActorByRole(w, "constable")
	if hubA == nil || hubB == nil || hubC == nil {
		t.Fatal("missing hub test actors")
	}
	hubA.Location, hubA.InsideHouse, hubA.Task, hubA.JailedUntil = "crossroads", "", nil, 0
	hubB.Location, hubB.InsideHouse, hubB.Task, hubB.JailedUntil = "crossroads", "", nil, 0
	hubC.Location, hubC.InsideHouse, hubC.Task, hubC.JailedUntil = "crossroads", "", nil, 0
	hubA.Money = 20
	hubA.Energy = 80
	sr.ActorSalience[hubA.ID] = 500

	macroDisaster := sim.Event{
		ID:      w.NextID,
		Tick:    w.Tick,
		ActorID: "world",
		Kind:    "disaster_struck",
		Action:  sim.Action{Kind: "wait", Location: "south_fields"},
		Payload: map[string]any{
			"text":     "Flash flood struck south_fields",
			"district": "south_fields",
			"wounded":  12,
			"deaths":   2,
		},
	}
	w.NextID++
	gossipEvs := w.ObserveSpotlightAndRumors(sr, []sim.Event{macroDisaster})
	if len(gossipEvs) != 0 {
		t.Fatalf("expected zero gossip_seeded events from macro disaster, got %+v", gossipEvs)
	}
	if _, injected := srTestFindMemoryByEventID(*hubA, macroDisaster.ID); injected {
		t.Fatalf("macro event %d was directly injected into focal actor %s memory without contact", macroDisaster.ID, hubA.ID)
	}

	// Instead, interpersonal interaction produces first-hand memory on hubA, which hubA relays via speak to hubB.
	bribeEvs := w.Apply(hubA.ID, sim.Action{Kind: "bribe", TargetID: hubC.ID})
	if len(bribeEvs) == 0 || bribeEvs[0].Kind != "bribed" {
		t.Fatalf("expected first-hand bribed interaction, got %+v", bribeEvs)
	}
	rootEvID := bribeEvs[0].ID
	speakGossip := w.Apply(hubA.ID, sim.Action{
		Kind:         "speak",
		TargetID:     hubB.ID,
		Text:         "Officer " + hubC.Name + " took a hush bribe at Crossroads.",
		BasisEventID: rootEvID,
		Statement:    &sim.Statement{Topic: "corruption", SubjectID: hubC.ID, Item: "coins", Location: "crossroads", Quantity: 4},
	})
	if len(speakGossip) == 0 || speakGossip[0].Kind != "speech" {
		t.Fatalf("focal actor failed to cite first-hand interaction via BasisEventID: %+v", speakGossip)
	}
	if speakGossip[0].Action.BasisEventID != rootEvID || speakGossip[0].Payload["root_event_id"] != rootEvID {
		t.Fatalf("speech lineage broken: %+v", speakGossip[0])
	}
	heardMem, heardOK := srTestFindMemoryByEventID(*hubB, speakGossip[0].ID)
	if !heardOK || heardMem.RootEventID != rootEvID || heardMem.BasisEventID != rootEvID {
		t.Fatalf("listener did not inherit root_event_id lineage: ok=%v mem=%+v", heardOK, heardMem)
	}

	// 4. agent.SalienceOrderedActorIDs ranking, filtering, determinism & tick rotation
	testActors := []sim.Actor{
		{ID: "dead_a", Dead: true},
		{ID: "jailed_a", JailedUntil: w.Tick + 10},
		{ID: "busy_a", Task: &sim.Task{Action: sim.Action{Kind: "sleep"}}},
		{ID: "high_a"},
		{ID: "mid_a"},
		{ID: "low_1"},
		{ID: "low_2"},
		{ID: "low_3"},
	}
	sal := map[string]int{"dead_a": 999, "jailed_a": 999, "busy_a": 999, "high_a": 80, "mid_a": 30}
	order1 := agent.SalienceOrderedActorIDs(testActors, sal, 100, 4)
	order1Repeat := agent.SalienceOrderedActorIDs(testActors, sal, 100, 4)
	if !reflect.DeepEqual(order1, order1Repeat) {
		t.Fatalf("SalienceOrderedActorIDs not deterministic: %v vs %v", order1, order1Repeat)
	}
	if len(order1) != 4 || order1[0] != "high_a" || order1[1] != "mid_a" {
		t.Fatalf("expected high_a, mid_a first and dead/jailed/busy excluded, got %v", order1)
	}
	// Check across ticks that equal-salience actors rotate so none starves.
	seenZero := map[string]bool{}
	for tick := int64(1); tick <= 30; tick++ {
		ids := agent.SalienceOrderedActorIDs(testActors, sal, tick, 3)
		if len(ids) == 3 {
			seenZero[ids[2]] = true
		}
	}
	if len(seenZero) < 2 {
		t.Fatalf("expected FNV tie-break rotation across ticks to prevent starvation, saw %v", seenZero)
	}

	// 5. High-trust public figure triggers panic on a single corroborated speech, and panic expires cleanly.
	priest := actorByID(w, "p0052")
	listener := actorByID(w, "p0053")
	if priest != nil && listener != nil {
		priest.Location, priest.InsideHouse, priest.Task, priest.JailedUntil = "grand_shrine", "", nil, 0
		listener.Location, listener.InsideHouse, listener.Task, listener.JailedUntil = "grand_shrine", "", nil, 0
		priest.Professions["high_priest"] = 2
		// Give incoming trust from 3 neighbors so priest qualifies as a high-trust public figure.
		for _, trusterID := range []string{"p01", "p02", "p03"} {
			if tr := actorByID(w, trusterID); tr != nil {
				if tr.Trust == nil {
					tr.Trust = map[string]int{}
				}
				tr.Trust[priest.ID] = 2
			}
		}
		spPriest := w.Apply(priest.ID, sim.Action{
			Kind:      "speak",
			TargetID:  listener.ID,
			Text:      "A plague warning has reached the Grand Shrine!",
			Statement: &sim.Statement{Topic: "plague", Item: "medicine", Location: "grand_shrine", Quantity: 1},
		})
		if len(spPriest) == 0 || spPriest[0].Kind != "speech" {
			t.Fatalf("priest speech failed: %+v", spPriest)
		}
		panicPriest := w.ObserveSpotlightAndRumors(sr, spPriest)
		if lastOfKind(panicPriest, "rumor_panic_triggered").ID == 0 {
			t.Fatalf("expected high-trust public figure to trigger rumor_panic_triggered, got %+v", panicPriest)
		}
		if w.DistrictPanicDemandPermille(sr, "grand_shrine", "medicine") <= 0 {
			t.Fatalf("expected medicine panic demand boost in grand_shrine")
		}
		// Advance past PanicDurationTicks and verify panic expires cleanly.
		w.Tick += sr.Spec.PanicDurationTicks + 1
		w.ObserveSpotlightAndRumors(sr, nil)
		if boost := w.DistrictPanicDemandPermille(sr, "grand_shrine", "medicine"); boost != 0 {
			t.Fatalf("expected panic boost to expire after PanicDurationTicks, got %d", boost)
		}
	}

	// Nil-safety checks.
	if got := w.ObserveSpotlightAndRumors(nil, drama); got != nil {
		t.Fatalf("expected nil on nil state, got %+v", got)
	}
	if got := w.StepDynamicSpotlight(nil); got != nil {
		t.Fatalf("expected nil on nil state, got %+v", got)
	}
	if got := w.DistrictPanicDemandPermille(nil, "market_street", "food"); got != 0 {
		t.Fatalf("expected 0 on nil state, got %d", got)
	}
	if sim.CloneSpotlightRumorState(nil) != nil {
		t.Fatal("expected nil clone on nil state")
	}
	if ids := agent.SalienceOrderedActorIDs(testActors, nil, 10, 2); len(ids) != 2 {
		t.Fatalf("expected nil salience map to work cleanly, got %v", ids)
	}

	// 6. JSON round-trip and CloneSpotlightRumorState
	cloned := sim.CloneSpotlightRumorState(sr)
	if !reflect.DeepEqual(sr, cloned) {
		t.Fatalf("CloneSpotlightRumorState mismatch")
	}
	raw, err := json.Marshal(sr)
	if err != nil {
		t.Fatal(err)
	}
	var decoded sim.SpotlightRumorState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sr, &decoded) {
		t.Fatalf("JSON round-trip mismatch:\nwant=%+v\ngot=%+v", sr, &decoded)
	}
}

func TestSpotlightRumorThreeDayMillionPopConservation(t *testing.T) {
	w := sim.LargeWorld("sr-3day-1m", 1_000_000)
	sr := sim.NewSpotlightRumorState()
	sr.Spec.MaxFocalActors = 126
	sr.Spec.TargetFocalActors = 122

	initPop := w.TotalPopulation()
	initCoins := w.TotalCoins()
	initTreasury := w.PublicTreasury

	summary := sim.NewScaleSummary()
	kinds := map[string]int{}

	for day := 0; day < 3; day++ {
		for tick := 0; tick < sim.TicksPerDay; tick++ {
			evs, err := w.Step(context.Background(), agent.Offline())
			if err != nil {
				t.Fatal(err)
			}
			obsEvs := w.ObserveSpotlightAndRumors(sr, evs)
			spotEvs := w.StepDynamicSpotlight(sr)

			allEvs := make([]sim.Event, 0, len(evs)+len(obsEvs)+len(spotEvs))
			allEvs = append(allEvs, evs...)
			allEvs = append(allEvs, obsEvs...)
			allEvs = append(allEvs, spotEvs...)

			summary.RecordEvents(allEvs)
			for _, e := range allEvs {
				kinds[e.Kind]++
			}
		}
	}

	if kinds["spotlight_promoted"] == 0 || kinds["spotlight_demoted"] == 0 {
		t.Fatalf("expected both spotlight_promoted and spotlight_demoted over 3 days at 1M pop, got promoted=%d demoted=%d",
			kinds["spotlight_promoted"], kinds["spotlight_demoted"])
	}
	if kinds["gossip_seeded"] != 0 {
		t.Fatalf("expected zero macro gossip_seeded memory injections over 3 days, got %d", kinds["gossip_seeded"])
	}
	if kinds["speech"] == 0 {
		t.Fatalf("expected interpersonal speech propagation over 3 days, got %v", kinds)
	}

	// Verify strict 0-error conservation across all 6 ledgers.
	if got, want := w.TotalPopulation()+w.TotalDeaths(), initPop+w.TotalBirths(); got != want {
		t.Fatalf("population ledger mismatch: living(%d)+deaths(%d)=%d != init(%d)+births(%d)=%d",
			w.TotalPopulation(), w.TotalDeaths(), got, initPop, w.TotalBirths(), want)
	}
	if gotW, wantW := summary.WoundedPeople, summary.HealedPeople+summary.WoundedDeaths+int64(w.TotalWounded()); gotW != wantW {
		t.Fatalf("wound ledger mismatch: added=%d != healed(%d)+wounded_deaths(%d)+active(%d)=%d",
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
	if gotT, wantT := initTreasury+tl.TaxToTreasury+tl.StateFeesIn+tl.ConfiscatedIn, w.PublicTreasury+tl.PayrollPaid+tl.ReliefAndInfraOut+tl.BountiesOut+tl.TreasuryEmbezzled; gotT != wantT {
		t.Fatalf("treasury ledger mismatch: in=%d != out+bal=%d", gotT, wantT)
	}
	if w.TotalCoins() != initCoins {
		t.Fatalf("TotalCoins mismatch: got=%d want=%d", w.TotalCoins(), initCoins)
	}
}

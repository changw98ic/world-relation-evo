package sim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// noisyStatefulMockDecider simulates a non-deterministic Jev + ContentModel decider whose
// chosen actions, generated speech texts, and structured statements vary with every call.
type noisyStatefulMockDecider struct {
	inner sim.Decider
	mu    sync.Mutex
	calls atomic.Int64
	seq   int64
}

func (n *noisyStatefulMockDecider) Decide(ctx context.Context, p sim.Perception) (sim.Action, error) {
	n.calls.Add(1)
	act, err := n.inner.Decide(ctx, p)
	if err != nil {
		return sim.Action{}, err
	}
	n.mu.Lock()
	n.seq++
	callSeq := n.seq
	n.mu.Unlock()

	// Inject non-deterministic/stateful LLM-like text variations and occasional noisy speech.
	if act.Kind == "speak" {
		act.Text = fmt.Sprintf("[LLM-gen#%d tick=%d] %s", callSeq, p.Tick, act.Text)
		if act.Statement == nil {
			act.Statement = &sim.Statement{
				Topic:    "gossip",
				Item:     "food",
				Location: p.Self.Location,
				Quantity: int(callSeq%7) + 1,
			}
		}
	} else if act.Kind == "wait" && len(p.Nearby) > 0 && callSeq%5 == 0 {
		target := p.Nearby[int(callSeq)%len(p.Nearby)]
		if !target.Sleeping && !target.Jailed {
			act = sim.Action{
				Kind:     "speak",
				TargetID: target.ID,
				Text:     fmt.Sprintf("Noisy LLM remark #%d at tick %d", callSeq, p.Tick),
				Statement: &sim.Statement{
					Topic:    "supply",
					Item:     "food",
					Location: p.Self.Location,
					Quantity: int(callSeq%5) + 2,
				},
			}
		}
	}
	return act, nil
}

func TestReplayFromCheckpoint_ByteIdenticalAndZeroLiveCalls(t *testing.T) {
	// Test on a rich world with Venues, GeoRegions, Polities, Ecology, MarketCredit, and Cohorts.
	w0 := sim.LargeWorld("replay-test", 200)
	w0.Map.GeoRegions = []sim.GeoRegion{
		{ID: "reg_main", Name: "Main Basin", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 120},
	}
	for i := range w0.Map.Districts {
		w0.Map.Districts[i].GeoRegionID = "reg_main"
		w0.Map.Districts[i].PolityID = "pref_main"
	}
	w0.Map.Venues = []sim.Venue{
		{ID: "venue_crossroads", Name: "Crossroads Plaza", DistrictID: "crossroads", GeoRegionID: "reg_main", PolityID: "pref_main", Kind: "hub", Features: []string{"hub", "yamen", "academy"}},
		{ID: "venue_market", Name: "Grand Market", DistrictID: "market_street", GeoRegionID: "reg_main", PolityID: "pref_main", Kind: "market", Features: []string{"market"}},
	}
	w0.Polities = &sim.PolityNetwork{
		Nodes: []sim.PolityNode{
			{ID: "pref_main", Name: "Qingzhou Prefecture", Level: "prefecture", SeatDistrictID: "crossroads", SeatVenueID: "venue_crossroads", LeaderActorID: w0.Actors[0].ID, Treasury: 50},
		},
	}

	// Step 2 ticks to reach an interesting mid-run checkpoint with active tasks/states, then serialize checkpoint.
	noisy := &noisyStatefulMockDecider{inner: agent.Offline()}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if _, err := w0.Step(ctx, noisy); err != nil {
			t.Fatalf("warmup step %d failed: %v", i, err)
		}
	}

	cpBytes, err := json.Marshal(w0)
	if err != nil {
		t.Fatalf("marshal checkpoint: %v", err)
	}
	// Restore live world from checkpoint bytes so both live and replay start from the exact serialized snapshot.
	var wLive sim.World
	if err := json.Unmarshal(cpBytes, &wLive); err != nil {
		t.Fatalf("unmarshal live world: %v", err)
	}

	const replayTicks = 20
	targetTick := wLive.Tick + replayTicks
	var liveEvents []sim.Event
	var logs []sim.TickDecisionLog
	for i := 0; i < replayTicks; i++ {
		evs, logEntry, err := wLive.StepRecorded(ctx, noisy)
		if err != nil {
			t.Fatalf("StepRecorded tick %d failed: %v", wLive.Tick, err)
		}
		liveEvents = append(liveEvents, evs...)
		logs = append(logs, logEntry)
	}
	if len(liveEvents) == 0 {
		t.Fatalf("expected events during 20 live ticks")
	}

	liveEndBytes, err := json.Marshal(&wLive)
	if err != nil {
		t.Fatalf("marshal live end state: %v", err)
	}
	callsBeforeReplay := noisy.calls.Load()
	if callsBeforeReplay == 0 {
		t.Fatalf("expected live noisy decider to have been called")
	}

	// Replay from checkpointJSON using ONLY persisted logs (zero calls to noisy decider).
	wReplay, replayEvents, err := sim.ReplayFromCheckpoint(cpBytes, logs, targetTick)
	if err != nil {
		t.Fatalf("ReplayFromCheckpoint failed: %v", err)
	}

	if gotCalls := noisy.calls.Load(); gotCalls != callsBeforeReplay {
		t.Fatalf("live decider was called during replay! before=%d after=%d", callsBeforeReplay, gotCalls)
	}

	replayEndBytes, err := json.Marshal(wReplay)
	if err != nil {
		t.Fatalf("marshal replay end state: %v", err)
	}
	if !bytes.Equal(liveEndBytes, replayEndBytes) {
		t.Fatalf("replay world JSON differs from live world JSON:\nlive len=%d replay len=%d", len(liveEndBytes), len(replayEndBytes))
	}
	if !reflect.DeepEqual(&wLive, wReplay) {
		t.Fatalf("replay World struct does not DeepEqual live World")
	}
	if !reflect.DeepEqual(liveEvents, replayEvents) {
		t.Fatalf("replay Events (%d) differ from live Events (%d)", len(replayEvents), len(liveEvents))
	}

	// Verify clear error when a required actor decision is missing from logs.
	brokenLogs := append([]sim.TickDecisionLog(nil), logs...)
	firstTickDecisions := make(map[string]sim.Action, len(brokenLogs[0].Decisions))
	var omittedActor string
	for k, v := range brokenLogs[0].Decisions {
		if omittedActor == "" {
			omittedActor = k
			continue
		}
		firstTickDecisions[k] = v
	}
	brokenLogs[0].Decisions = firstTickDecisions
	if omittedActor != "" {
		_, _, err := sim.ReplayFromCheckpoint(cpBytes, brokenLogs, targetTick)
		if err == nil || !strings.Contains(err.Error(), omittedActor) {
			t.Fatalf("expected clear error naming omitted actor %q, got: %v", omittedActor, err)
		}
	}

	// Verify clear error when an entire tick log is missing.
	if _, _, err := sim.ReplayFromCheckpoint(cpBytes, logs[1:], targetTick); err == nil {
		t.Fatalf("expected error when starting tick log is missing")
	}
}

func TestInspectQueries_ActorLineagePolityConservation(t *testing.T) {
	cat := sim.DefaultCatalog()
	cat.CommChannels = map[string]sim.CommChannelSpec{
		"radio_broadcast": {
			ID:               "radio_broadcast",
			Name:             "Wasteland Radio",
			GrantedByTags:    []string{"radio_transceiver"},
			ReceiverTags:     []string{"radio_transceiver"},
			RangeKm:          100,
			Broadcast:        true,
			PenetrationPower: 60,
			EnergyCost:       1,
		},
	}

	cat.Forms = map[string]sim.FormSpec{
		"cyborg_commander": {ID: "cyborg_commander", Name: "Cyborg Commander", InnateTags: []string{"cybernetic"}},
	}
	cat.StateSlots = map[string]sim.StateSlotSpec{
		"battery": {ID: "battery", Min: 0, Max: 100, Default: 80},
	}
	cat.Currencies = map[string]sim.CurrencySpec{
		"orbital_scrip": {ID: "orbital_scrip", Name: "Orbital Scrip", PolityID: "orbital_authority"},
	}
	cat.Techs = map[string]sim.TechSpec{
		"microgrid_relay": {ID: "microgrid_relay", Name: "Microgrid Relay"},
	}

	w := &sim.World{
		ID:      "inspect-domain",
		NextID:  1,
		Catalog: cat,
		Map: sim.WorldMap{
			GeoRegions: []sim.GeoRegion{
				{ID: "crater", Name: "Irradiated Crater", Coord: sim.GeoCoord{X: 0, Y: 0}, ElevationM: 100},
				{ID: "plateau", Name: "Aegis Plateau", Coord: sim.GeoCoord{X: 20, Y: 0}, ElevationM: 250},
			},
			Districts: []sim.District{
				{ID: "scrap_dist", Name: "Scrap Sector", GeoRegionID: "crater", PolityID: "scrap_sector", Features: []string{"market"}},
				{ID: "spire_dist", Name: "Spire Command", GeoRegionID: "plateau", PolityID: "orbital_authority", Features: []string{"hub"}},
			},
			Roads: []sim.Road{
				{From: "scrap_dist", To: "spire_dist", Ticks: 2, EnergyCost: 2},
			},
			Venues: []sim.Venue{
				{ID: "scrap_yard", Name: "Surface Scrapyard", DistrictID: "scrap_dist", GeoRegionID: "crater", PolityID: "scrap_sector", Kind: "outpost", Features: []string{"market"}},
				{ID: "bunker_vault", Name: "Shielded Bunker", DistrictID: "scrap_dist", GeoRegionID: "crater", PolityID: "scrap_sector", Kind: "bunker", Indoor: true, Shielding: 80},
				{ID: "command_deck", Name: "Command Deck", DistrictID: "spire_dist", GeoRegionID: "plateau", PolityID: "orbital_authority", Kind: "command", Features: []string{"hub"}},
			},
		},
		Polities: &sim.PolityNetwork{
			Nodes: []sim.PolityNode{
				{
					ID: "orbital_authority", Name: "Orbital Authority", Level: "orbital",
					SeatDistrictID: "spire_dist", SeatVenueID: "command_deck",
					LeaderActorID: "admiral", Treasury: 120,
				},
				{
					ID: "scrap_syndicate", Name: "Scrap Syndicate", Level: "syndicate",
					LeaderActorID: "boss", Treasury: 45,
				},
				{
					ID: "scrap_sector", Name: "Scrap Sector", Level: "sector",
					DeJureParentID:      "orbital_authority",
					DeFactoControllerID: "scrap_syndicate",
					SeatDistrictID:      "scrap_dist", SeatVenueID: "scrap_yard",
				},
			},
		},
		Houses: []sim.House{
			{ID: "h1", Name: "Command Quarters", District: "spire_dist", OwnerID: "admiral", Residents: []string{"admiral"}, Level: 2, Comfort: 3, Condition: 90},
		},
		Actors: []sim.Actor{
			{
				ID: "admiral", Name: "Admiral Vance", Role: "commander",
				Tags: []string{"resident", "ruler", "radio_transceiver"},
				Location: "spire_dist", VenueID: "command_deck",
				Health: 100, Energy: 100, Money: 80,
				FormID:     "cyborg_commander",
				StateSlots: map[string]int{"battery": 85},
				Currencies: map[string]int64{"orbital_scrip": 150},
				Items: map[string]int{"food": 5, "radio_transceiver": 1}, Trust: map[string]int{},
			},
			{
				ID: "scout", Name: "Scout Rex", Role: "hunter",
				Tags: []string{"resident", "trader", "radio_transceiver"},
				Location: "scrap_dist", VenueID: "scrap_yard",
				Health: 100, Energy: 100, Money: 25,
				Items: map[string]int{"food": 4, "radio_transceiver": 1}, Trust: map[string]int{},
			},
			{
				ID: "boss", Name: "Boss Nyx", Role: "trader",
				Tags: []string{"resident", "trader"},
				Location: "scrap_dist", VenueID: "scrap_yard",
				Health: 100, Energy: 100, Money: 40,
				Items: map[string]int{"food": 4}, Trust: map[string]int{},
			},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "c_scav", Name: "Scrap Scavengers", District: "scrap_dist", Profession: "miner", Population: 50, Adults: 40, Children: 5, Elders: 5, MoneyPool: 300},
		},
		Knowledge: &sim.KnowledgeBook{
			Records: map[string]*sim.KnowledgeRecord{
				"microgrid_relay": {TechID: "microgrid_relay", DiscovererID: "admiral", KnownActors: []string{"admiral"}},
			},
		},
		PublicTreasury: 100,
	}
	w.InitLedgerAudit()

	var events []sim.Event
	// Root event: anomaly in crater (witnessed by scout at scrap_yard).
	anomEvs := w.TriggerGeoAnomaly(sim.GeoAnomalySpec{
		ID:                "rad_storm",
		Name:              "Crater Fallout Storm",
		OriginRegionID:    "crater",
		Intensity:         60,
		ActorHealthDamage: 5,
	})
	events = append(events, anomEvs...)
	rootID := anomEvs[0].ID

	// Hop 1: scout transmits radio broadcast citing rootID -> admiral receives channel_message.
	txEvs := w.Apply("scout", sim.Action{
		Kind:         "transmit_message",
		Channel:      "radio_broadcast",
		BasisEventID: rootID,
		Text:         "Fallout storm hit scrap_yard",
		Statement:    &sim.Statement{Topic: "macro_anomaly", SubjectID: "rad_storm", Location: "scrap_dist", Quantity: 60},
	})
	events = append(events, txEvs...)
	txID := txEvs[0].ID

	// Hop 2: scout speaks locally to boss citing txID.
	spEvs := w.Apply("scout", sim.Action{
		Kind:         "speak",
		TargetID:     "boss",
		BasisEventID: txID,
		Text:         "Told orbital command about the storm",
		Statement:    &sim.Statement{Topic: "macro_anomaly", SubjectID: "rad_storm", Location: "scrap_dist", Quantity: 60},
	})
	events = append(events, spEvs...)
	spID := spEvs[0].ID

	// Decree + offer to verify polity & conservation inspection.
	decEvs := w.Apply("admiral", sim.Action{Kind: "issue_decree", FactionID: "orbital_authority", Item: "market_tax", Quantity: 75})
	events = append(events, decEvs...)
	w.Apply("scout", sim.Action{Kind: "offer", TargetID: "boss", Item: "food", Quantity: 1, Price: 5})

	// 1. InspectActor: check admiral (channel_message with Statement & ChannelID, Observation==nil)
	// and scout (eyewitness Observation).
	admiralRep, err := sim.InspectActor(w, "admiral")
	if err != nil {
		t.Fatalf("InspectActor(admiral) failed: %v", err)
	}
	if admiralRep.GeoRegionID != "plateau" || admiralRep.PolityID != "orbital_authority" || admiralRep.VenueID != "command_deck" {
		t.Fatalf("unexpected admiral placement: %+v", admiralRep)
	}
	if admiralRep.FormID != "cyborg_commander" || admiralRep.StateSlots["battery"] != 85 || admiralRep.Currencies["orbital_scrip"] != 150 || len(admiralRep.KnownTechs) != 1 || admiralRep.KnownTechs[0] != "microgrid_relay" {
		t.Fatalf("expected integrated Phase 3-4 fields on InspectActor, got %+v", admiralRep)
	}
	foundChannelMem := false
	for _, m := range admiralRep.Memories {
		if m.Kind == "channel_message" {
			foundChannelMem = true
			if m.HasObservation || !m.HasStatement || m.ChannelID != "radio_broadcast" || m.RootEventID != rootID {
				t.Fatalf("bad channel memory epistemic fields: %+v", m)
			}
		}
	}
	if !foundChannelMem {
		t.Fatalf("expected channel_message memory on admiral")
	}

	scoutRep, err := sim.InspectActor(w, "scout")
	if err != nil {
		t.Fatalf("InspectActor(scout) failed: %v", err)
	}
	if len(scoutRep.Memories) == 0 || !scoutRep.Memories[0].HasObservation || scoutRep.Memories[0].HasStatement {
		t.Fatalf("expected first-hand Observation on scout, got %+v", scoutRep.Memories)
	}
	if _, err := sim.InspectActor(w, "nonexistent"); err == nil {
		t.Fatalf("expected error for unknown actor")
	}

	// 2. InspectLineage: trace from leaf spID back to rootID and from rootID downstream.
	linLeaf, err := sim.InspectLineage(events, spID)
	if err != nil {
		t.Fatalf("InspectLineage(%d) failed: %v", spID, err)
	}
	if linLeaf.RootEventID != rootID || len(linLeaf.UpstreamChain) != 3 {
		t.Fatalf("expected 3-hop chain [%d -> %d -> %d], got root=%d chain=%+v", rootID, txID, spID, linLeaf.RootEventID, linLeaf.UpstreamChain)
	}
	if linLeaf.UpstreamChain[0].EventID != rootID || linLeaf.UpstreamChain[1].EventID != txID || linLeaf.UpstreamChain[2].EventID != spID {
		t.Fatalf("unexpected chain order: %+v", linLeaf.UpstreamChain)
	}

	linRoot, err := sim.InspectLineage(events, rootID)
	if err != nil {
		t.Fatalf("InspectLineage(%d) failed: %v", rootID, err)
	}
	if linRoot.DownstreamTree == nil || len(linRoot.DownstreamTree.Children) != 1 || linRoot.DownstreamTree.Children[0].Node.EventID != txID {
		t.Fatalf("unexpected downstream tree from root: %+v", linRoot.DownstreamTree)
	}
	if len(linRoot.DescendantIDs) != 2 {
		t.Fatalf("expected 2 descendants [%d, %d], got %v", txID, spID, linRoot.DescendantIDs)
	}
	if _, err := sim.InspectLineage(events, 999999); err == nil {
		t.Fatalf("expected error for unknown event ID")
	}

	// 3. InspectPolity: check DeJure vs DeFacto split on scrap_sector, decrees on orbital_authority.
	scrapPol, err := sim.InspectPolity(w, "scrap_sector")
	if err != nil {
		t.Fatalf("InspectPolity(scrap_sector) failed: %v", err)
	}
	if scrapPol.DeJureParentID != "orbital_authority" || scrapPol.DeFactoControllerID != "scrap_syndicate" || scrapPol.EffectiveRulerID != "scrap_syndicate" {
		t.Fatalf("expected scrap_sector DeJure=orbital_authority, DeFacto=scrap_syndicate, got %+v", scrapPol)
	}
	if len(scrapPol.Venues) != 2 {
		t.Fatalf("expected 2 venues under scrap_sector, got %d", len(scrapPol.Venues))
	}
	orbPol, err := sim.InspectPolity(w, "orbital_authority")
	if err != nil {
		t.Fatalf("InspectPolity(orbital_authority) failed: %v", err)
	}
	if len(orbPol.ActiveDecrees) != 1 || orbPol.LeaderActorID != "admiral" || len(orbPol.DeJureChildren) != 1 || len(orbPol.DeFactoSubjects) != 0 {
		t.Fatalf("unexpected orbital_authority inspection: %+v", orbPol)
	}

	// 4. InspectWorldConservation: verify population, M0 coins, treasury, offers, houses, cohorts.
	cons := sim.InspectWorldConservation(w)
	if cons.TotalPopulation != 53 || cons.LivingActors != 3 || cons.CohortPopulation != 50 {
		t.Fatalf("unexpected population conservation: %+v", cons)
	}
	if cons.TotalCoins != w.TotalCoins() || cons.TotalCoins != 80+25+40+300+100+120+45 {
		t.Fatalf("unexpected coin conservation: got=%d want=%d", cons.TotalCoins, w.TotalCoins())
	}
	if !cons.LedgerConserved || cons.Currencies["orbital_scrip"] != 150 {
		t.Fatalf("expected conserved Phase 3 ledger and orbital_scrip=150, got %+v", cons)
	}
	if cons.ActiveOffers != 1 || cons.HouseCount != 1 || cons.CohortCount != 1 {
		t.Fatalf("unexpected offers/houses/cohorts: %+v", cons)
	}
}

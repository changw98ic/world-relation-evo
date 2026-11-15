package sim_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func relTestFindEvent(events []sim.Event, kind string) (sim.Event, bool) {
	for _, e := range events {
		if e.Kind == kind {
			return e, true
		}
	}
	return sim.Event{}, false
}

func relTestSumWoundedDeaths(events []sim.Event, kind string) (wounded, deaths int) {
	for _, e := range events {
		if e.Kind != kind {
			continue
		}
		if w, ok := e.Payload["wounded"].(int); ok {
			wounded += w
		} else if w64, ok := e.Payload["wounded"].(int64); ok {
			wounded += int(w64)
		}
		if d, ok := e.Payload["deaths"].(int); ok {
			deaths += d
		} else if d64, ok := e.Payload["deaths"].(int64); ok {
			deaths += int(d64)
		}
	}
	return wounded, deaths
}

func TestRelationsKinshipMarriageVendettaTurfAndMonopoly(t *testing.T) {
	t.Run("MarriageDowryHouseSharingAndBigamyGuard", func(t *testing.T) {
		w := sim.Village("rel-marry")
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)
		initialCoins := w.TotalCoins()

		// p01 (林禾) and p02 (陈谷) start co-located inside h01 (owned by p01).
		p01 := actorByID(w, "p01")
		p02 := actorByID(w, "p02")
		p03 := actorByID(w, "p03")
		if p01 == nil || p02 == nil || p03 == nil {
			t.Fatal("missing baseline actors")
		}
		// Place p03 inside h01 temporarily to test bigamy rejection after p01 marries p02.
		p03.InsideHouse = "h01"
		p03.Location = p01.Location

		evs, handled := w.ApplyRelationAction(rs, p01.ID, sim.Action{
			Kind:     "marry",
			TargetID: p02.ID,
			Price:    5,
		})
		if !handled || len(evs) != 1 || evs[0].Kind != "actor_married" {
			t.Fatalf("expected actor_married event, got handled=%v evs=%+v", handled, evs)
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("marriage broke M0 coin conservation: got=%d want=%d", w.TotalCoins(), initialCoins)
		}
		if rs.Kinship[p01.ID].SpouseID != p02.ID || rs.Kinship[p02.ID].SpouseID != p01.ID {
			t.Fatalf("spouse links not set symmetrically: %+v vs %+v", rs.Kinship[p01.ID], rs.Kinship[p02.ID])
		}
		clanA := rs.Kinship[p01.ID].ClanID
		clanB := rs.Kinship[p02.ID].ClanID
		if clanA == "" || clanB == "" || clanA == clanB {
			t.Fatalf("expected distinct surname clans for inter-clan marriage, got %q and %q", clanA, clanB)
		}
		if rs.Clans[clanA].Trust[clanB] <= 0 || rs.Clans[clanB].Trust[clanA] <= 0 {
			t.Fatalf("expected positive inter-clan trust after wedding, got A->B=%d B->A=%d",
				rs.Clans[clanA].Trust[clanB], rs.Clans[clanB].Trust[clanA])
		}

		// Ensure house residence merged if spouse had separate home.
		p04 := actorByID(w, "p04")
		p04.InsideHouse = ""
		p04.Location = "west_cottages"
		p05 := actorByID(w, "p05")
		p05.InsideHouse = ""
		p05.Location = "west_cottages"
		evs2, _ := w.ApplyRelationAction(rs, p04.ID, sim.Action{Kind: "marry", TargetID: p05.ID, Price: 3})
		if evs2[0].Kind != "actor_married" {
			t.Fatalf("expected second marriage to succeed, got %+v", evs2)
		}
		h02 := w.Houses[1] // h02 is p04's home
		if !h02.CanAccess(p05.ID) {
			t.Fatalf("expected spouse %s to gain access to %s residents=%v", p05.ID, h02.ID, h02.Residents)
		}

		// Bigamy must fail cleanly.
		failEvs, handled := w.ApplyRelationAction(rs, p01.ID, sim.Action{Kind: "marry", TargetID: p03.ID, Price: 2})
		if !handled || len(failEvs) == 0 || failEvs[0].Kind != "action_failed" {
			t.Fatalf("expected bigamy attempt to fail, got %+v", failEvs)
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin conservation mismatch after marriages: got=%d want=%d", w.TotalCoins(), initialCoins)
		}
	})

	t.Run("SwornBondPatronageAndLineageInheritanceWithDisputes", func(t *testing.T) {
		w := sim.Village("rel-inherit")
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)
		initialCoins := w.TotalCoins()

		// Case 1: Peaceful inheritance via marriage + sworn/patron bond.
		p01 := actorByID(w, "p01")
		p02 := actorByID(w, "p02")
		p01.Items["tools"] = 2
		p01MoneyBefore := p01.Money

		w.ApplyRelationAction(rs, p01.ID, sim.Action{Kind: "marry", TargetID: p02.ID, Price: 2})
		bondEvs, ok := w.ApplyRelationAction(rs, p02.ID, sim.Action{Kind: "swear_bond", TargetID: p01.ID, Text: "patron", Price: 1})
		if !ok || bondEvs[0].Kind != "bond_sworn" {
			t.Fatalf("expected bond_sworn event, got %+v", bondEvs)
		}
		if rs.Kinship[p02.ID].PatronID != p01.ID {
			t.Fatalf("expected p01 as patron of p02, got %q", rs.Kinship[p02.ID].PatronID)
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin mismatch after bond gift: got=%d want=%d", w.TotalCoins(), initialCoins)
		}

		// p01 dies naturally -> p02 inherits h01, money, and tools peacefully.
		p01.Health = 0
		p01.Dead = true
		p01.Money = p01MoneyBefore - 1 // exact current money after -2 dowry +1 gift
		deathEv := sim.Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: p01.ID,
			Kind:    "actor_died",
			Payload: map[string]any{"cause": "old_age"},
		}
		w.NextID++
		inhEvs := rs.ObserveCausalRelationEvents(w, []sim.Event{deathEv})
		settled, found := relTestFindEvent(inhEvs, "inheritance_settled")
		if !found {
			t.Fatalf("expected inheritance_settled event, got %+v", inhEvs)
		}
		if settled.Payload["heir_id"] != p02.ID {
			t.Fatalf("expected spouse p02 to inherit, got %v", settled.Payload["heir_id"])
		}
		if w.Houses[0].OwnerID != p02.ID {
			t.Fatalf("expected h01 owner transferred to p02, got %q", w.Houses[0].OwnerID)
		}
		if p02.Items["tools"] < 2 {
			t.Fatalf("expected inherited tools >= 2, got %d", p02.Items["tools"])
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin conservation broken after inheritance: got=%d want=%d", w.TotalCoins(), initialCoins)
		}

		// Verify actor_succeeded integrates successor into predecessor's clan and lineage.
		succ := actorByID(w, "p10")
		if succ != nil {
			succEv := sim.Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  succ.ID,
				TargetID: p01.ID,
				Kind:     "actor_succeeded",
				Payload: map[string]any{
					"predecessor_id": p01.ID,
					"successor_id":   succ.ID,
				},
			}
			w.NextID++
			rs.ObserveCausalRelationEvents(w, []sim.Event{succEv})
			if rs.Kinship[succ.ID].ClanID != rs.Kinship[p01.ID].ClanID {
				t.Fatalf("expected successor %s to inherit clan %q, got %q", succ.ID, rs.Kinship[p01.ID].ClanID, rs.Kinship[succ.ID].ClanID)
			}
		}

		// Case 2: Low family trust triggers inheritance_dispute and forms a grudge.
		p03 := actorByID(w, "p03") // owns h02
		p04 := actorByID(w, "p04")
		p05 := actorByID(w, "p05")
		// Make p04 and p05 sworn brothers of p03 with mutual hostility.
		p03.InsideHouse = ""
		p04.InsideHouse = ""
		p05.InsideHouse = ""
		p05.Location = p03.Location
		w.ApplyRelationAction(rs, p03.ID, sim.Action{Kind: "swear_bond", TargetID: p04.ID})
		w.ApplyRelationAction(rs, p03.ID, sim.Action{Kind: "swear_bond", TargetID: p05.ID})
		p04.Trust[p05.ID] = -3
		p05.Trust[p04.ID] = -3
		p03.Health = 0
		p03.Dead = true
		deathEv2 := sim.Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: p03.ID,
			Kind:    "actor_died",
			Payload: map[string]any{"cause": "old_age"},
		}
		w.NextID++
		dispEvs := rs.ObserveCausalRelationEvents(w, []sim.Event{deathEv2})
		if _, found := relTestFindEvent(dispEvs, "inheritance_dispute"); !found {
			t.Fatalf("expected inheritance_dispute when claimants distrust each other, got %+v", dispEvs)
		}
		if _, found := relTestFindEvent(dispEvs, "grudge_formed"); !found {
			t.Fatalf("expected grudge_formed from inheritance dispute, got %+v", dispEvs)
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin conservation broken after disputed inheritance: got=%d want=%d", w.TotalCoins(), initialCoins)
		}
	})

	t.Run("CausalGrudgeVendettaRetaliationAndBloodMoneyMediation", func(t *testing.T) {
		w := sim.Village("rel-feud")
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)

		// Place p30 (guard) and p19 (blacksmith) outdoors in market_street.
		raider := actorByID(w, "p30")
		victim := actorByID(w, "p19")
		mediator := actorByID(w, "p29") // scholar
		raider.InsideHouse = ""
		raider.Location = "market_street"
		victim.InsideHouse = ""
		victim.Location = "market_street"
		mediator.InsideHouse = ""
		mediator.Location = "market_street"
		raider.Money = 25
		victim.Money = 10
		initialCoins := w.TotalCoins()

		rootRaid := sim.Event{
			ID:       501,
			Tick:     w.Tick,
			ActorID:  raider.ID,
			TargetID: victim.ID,
			Kind:     "raided",
			Payload: map[string]any{
				"damage":        20,
				"coins":         4,
				"root_event_id": int64(501),
			},
		}
		grudgeEvs := rs.ObserveCausalRelationEvents(w, []sim.Event{rootRaid})
		gEv, found := relTestFindEvent(grudgeEvs, "grudge_formed")
		if !found {
			t.Fatalf("expected grudge_formed from raided event, got %+v", grudgeEvs)
		}
		if gEv.Payload["root_event_id"] != int64(501) {
			t.Fatalf("expected root_event_id=501 preserved, got %v", gEv.Payload["root_event_id"])
		}

		// Victim mediates first grudge with blood money via scholar mediator.
		medEvs, handled := w.ApplyRelationAction(rs, mediator.ID, sim.Action{
			Kind:     "mediate_feud",
			TargetID: raider.ID,
			Price:    6,
		})
		if !handled || len(medEvs) == 0 || medEvs[0].Kind != "feud_mediated" {
			t.Fatalf("expected feud_mediated event, got handled=%v evs=%+v", handled, medEvs)
		}
		if raider.Money != 19 || victim.Money != 16 {
			t.Fatalf("expected 6 blood-money coins transferred raider(25->19) victim(10->16), got %d / %d", raider.Money, victim.Money)
		}
		if medEvs[0].Payload["root_event_id"] != int64(501) {
			t.Fatalf("expected root_event_id=501 preserved in mediation, got %v", medEvs[0].Payload["root_event_id"])
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin conservation broken after blood-money mediation: got=%d want=%d", w.TotalCoins(), initialCoins)
		}

		// Now test homicide -> kin blood feud retaliation (avenge_feud).
		p01 := actorByID(w, "p01")
		p02 := actorByID(w, "p02")
		p01.InsideHouse = ""
		p01.Location = "market_street"
		p02.InsideHouse = ""
		p02.Location = "market_street"
		w.ApplyRelationAction(rs, p01.ID, sim.Action{Kind: "marry", TargetID: p02.ID, Price: 2})
		p01.Health = 0
		p01.Dead = true
		murderEv := sim.Event{
			ID:       601,
			Tick:     w.Tick,
			ActorID:  p01.ID,
			TargetID: raider.ID,
			Kind:     "actor_died",
			Payload: map[string]any{
				"killer_id":     raider.ID,
				"cause":         "combat",
				"root_event_id": int64(601),
			},
		}
		feudEvs := rs.ObserveCausalRelationEvents(w, []sim.Event{murderEv})
		if _, found := relTestFindEvent(feudEvs, "grudge_formed"); !found {
			t.Fatalf("expected homicide grudge_formed for spouse p02, got %+v", feudEvs)
		}
		raiderHealthBefore := raider.Health
		avEvs, handled := w.ApplyRelationAction(rs, p02.ID, sim.Action{
			Kind:     "avenge_feud",
			TargetID: raider.ID,
		})
		if !handled || len(avEvs) == 0 || avEvs[0].Kind != "feud_avenged" {
			t.Fatalf("expected feud_avenged event, got %+v", avEvs)
		}
		if raider.Health >= raiderHealthBefore {
			t.Fatalf("expected raider health to drop after avenge_feud: %d -> %d", raiderHealthBefore, raider.Health)
		}
		if avEvs[0].Payload["root_event_id"] != int64(601) {
			t.Fatalf("expected root_event_id=601 preserved in avenge_feud, got %v", avEvs[0].Payload["root_event_id"])
		}
		if w.TotalCoins() != initialCoins {
			t.Fatalf("coin conservation broken after avenge_feud: got=%d want=%d", w.TotalCoins(), initialCoins)
		}
	})

	t.Run("LargeWorldMacroWeddingsVendettaTurfShiftLeadershipAndMonopolyCharter", func(t *testing.T) {
		w := sim.LargeWorld("rel-large", 10000)
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)
		initialCoins := w.TotalCoins()
		initialPop := w.TotalPopulation()

		merchFaction := factionByID(w, "merchants_guild")
		gangFaction := factionByID(w, "gang_guild")
		if merchFaction == nil || gangFaction == nil {
			t.Fatal("missing merchants_guild or gang_guild")
		}
		// Boost gang_guild cohorts in market_street so it dominates market_street sovereignty.
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.FactionID == "gang_guild" && c.District == "market_street" {
				c.Population += 200
				c.Adults += 200
				initialPop += 200
			}
		}
		// Boost a challenger member in merchants_guild to trigger leadership turnover.
		if len(merchFaction.Members) >= 2 {
			challenger := actorByID(w, merchFaction.Members[len(merchFaction.Members)-1])
			if challenger != nil {
				challenger.Level += 6
				challenger.Attributes.Spirit += 8
			}
		}

		// Seed a wrongful_conviction event and a high-intensity cohort_raided event to test macro mediation & vendetta_clash.
		var farmerCohort, banditCohort, peddlerCohort *sim.PopulationCohort
		for i := range w.Cohorts {
			switch w.Cohorts[i].Profession {
			case "farmer":
				if farmerCohort == nil {
					farmerCohort = &w.Cohorts[i]
				}
			case "bandit":
				if banditCohort == nil {
					banditCohort = &w.Cohorts[i]
				}
			case "peddler":
				if peddlerCohort == nil {
					peddlerCohort = &w.Cohorts[i]
				}
			}
		}
		if farmerCohort == nil || banditCohort == nil || peddlerCohort == nil {
			t.Fatal("missing expected cohorts in LargeWorld")
		}
		banditCohort.MoneyPool += 500
		initialCoins += 500

		rs.ObserveCausalRelationEvents(w, []sim.Event{
			{
				ID:       901,
				Tick:     0,
				ActorID:  "yamen_collector",
				TargetID: peddlerCohort.ID,
				Kind:     "wrongful_conviction",
				Payload: map[string]any{
					"framer_id":     banditCohort.ID,
					"victim_id":     peddlerCohort.ID,
					"root_event_id": int64(901),
				},
			},
			{
				ID:       902,
				Tick:     0,
				ActorID:  banditCohort.ID,
				TargetID: farmerCohort.ID,
				Kind:     "cohort_raided",
				Payload: map[string]any{
					"attacker_cohort": banditCohort.ID,
					"target_cohort":   farmerCohort.ID,
					"deaths":          3,
					"wounded":         8,
					"root_event_id":   int64(902),
				},
			},
		})

		// Advance 1 full day calling StepRelationsAndTurf each tick.
		var allEvents []sim.Event
		for tick := 0; tick < sim.TicksPerDay; tick++ {
			w.Tick = int64(tick)
			evs := w.StepRelationsAndTurf(rs)
			allEvents = append(allEvents, evs...)
		}

		if _, ok := relTestFindEvent(allEvents, "cohort_wedding_held"); !ok {
			t.Fatalf("expected cohort_wedding_held over 1 day, got events=%d", len(allEvents))
		}
		if _, ok := relTestFindEvent(allEvents, "feud_mediated"); !ok {
			t.Fatalf("expected feud_mediated for moderate wrongful_conviction grudge, got events=%+v", allEvents)
		}
		if _, ok := relTestFindEvent(allEvents, "vendetta_clash"); !ok {
			t.Fatalf("expected vendetta_clash for high-intensity cohort_raided grudge, got events=%+v", allEvents)
		}
		if _, ok := relTestFindEvent(allEvents, "district_control_shifted"); !ok {
			t.Fatalf("expected district_control_shifted in LargeWorld, got events=%+v", allEvents)
		}
		if _, ok := relTestFindEvent(allEvents, "faction_leader_changed"); !ok {
			t.Fatalf("expected faction_leader_changed when challenger outscored incumbent, got events=%+v", allEvents)
		}
		if _, ok := relTestFindEvent(allEvents, "monopoly_chartered"); !ok {
			t.Fatalf("expected monopoly_chartered auctioned during daily step, got events=%+v", allEvents)
		}

		// Verify explicit GrantMonopolyCharter treasury fee + kickback conservation.
		beforeTreasury := w.PublicTreasury
		beforeIllicit := w.TotalIllicitWealth()
		chEvs, ok := w.GrantMonopolyCharter(rs, "merchants_guild", "choice_grain", "market_street", 50)
		if !ok || len(chEvs) != 1 || chEvs[0].Kind != "monopoly_chartered" {
			t.Fatalf("expected explicit GrantMonopolyCharter to succeed, got ok=%v evs=%+v", ok, chEvs)
		}
		if w.PublicTreasury != beforeTreasury+50 {
			t.Fatalf("expected PublicTreasury +50 fee, got %d -> %d", beforeTreasury, w.PublicTreasury)
		}
		if w.TotalIllicitWealth() <= beforeIllicit {
			t.Fatalf("expected corrupt collector kickback to increase TotalIllicitWealth: %d -> %d", beforeIllicit, w.TotalIllicitWealth())
		}
		if ch, active := rs.ActiveCharterForItem("choice_grain", w.Tick); !active || ch.FactionID != "merchants_guild" {
			t.Fatalf("expected active choice_grain monopoly charter for merchants_guild, got %+v active=%v", ch, active)
		}

		// Advance past charter expiration and verify monopoly_revoked emits cleanly.
		chExpired, _ := rs.ActiveCharterForItem("choice_grain", w.Tick)
		savedTick := w.Tick
		w.Tick = chExpired.ExpiresTick
		revEvs := w.StepRelationsAndTurf(rs)
		if rev, ok := relTestFindEvent(revEvs, "monopoly_revoked"); !ok || rev.Payload["item"] != "choice_grain" {
			t.Fatalf("expected monopoly_revoked on expiration, got %+v", revEvs)
		}
		if _, active := rs.ActiveCharterForItem("choice_grain", w.Tick); active {
			t.Fatal("expected choice_grain monopoly charter to be inactive after expiration")
		}
		w.Tick = savedTick

		// Verify strict zero-error invariants: TotalCoins, Population + Deaths, TreasuryLedger, Wounded ledger.
		if w.TotalCoins() != initialCoins {
			t.Fatalf("M0 coin conservation mismatch: got=%d want=%d", w.TotalCoins(), initialCoins)
		}
		if w.TotalPopulation()+w.TotalDeaths() != initialPop+w.TotalBirths() {
			t.Fatalf("population conservation mismatch: pop=%d deaths=%d init=%d births=%d",
				w.TotalPopulation(), w.TotalDeaths(), initialPop, w.TotalBirths())
		}
		tl := w.TreasuryLedger
		wantTreasury := tl.InitialTreasury + tl.TaxToTreasury + tl.StateFeesIn + tl.ConfiscatedIn -
			tl.PayrollPaid - tl.ReliefAndInfraOut - tl.BountiesOut - tl.TreasuryEmbezzled
		if w.PublicTreasury != wantTreasury {
			t.Fatalf("TreasuryLedger mismatch: treasury=%d want=%d ledger=%+v", w.PublicTreasury, wantTreasury, tl)
		}
		vendWounded, vendDeaths := relTestSumWoundedDeaths(allEvents, "vendetta_clash")
		if vendWounded != w.TotalWounded() {
			t.Fatalf("vendetta wounded mismatch: reported=%d actual_world_wounded=%d", vendWounded, w.TotalWounded())
		}
		if vendDeaths != w.TotalDeaths() || int64(vendDeaths) != w.DeathsByCause["combat"] {
			t.Fatalf("vendetta deaths mismatch: reported=%d total_deaths=%d combat_cause=%d", vendDeaths, w.TotalDeaths(), w.DeathsByCause["combat"])
		}

		// Verify focal claim_turf action (both repelled against entrenched controller and succeeding on unclaimed turf).
		if len(merchFaction.Members) > 0 {
			claimer := actorByID(w, merchFaction.Members[0])
			claimer.InsideHouse = ""
			claimer.Location = "west_cottages"
			claimer.Energy = 50
			repelEvs, handled := w.ApplyRelationAction(rs, claimer.ID, sim.Action{
				Kind:      "claim_turf",
				FactionID: "merchants_guild",
				Location:  "west_cottages",
			})
			if !handled || repelEvs[0].Kind != "turf_claim_repelled" {
				t.Fatalf("expected focal claim_turf against strong farmers_guild to repel, got %+v", repelEvs)
			}

			rs.ControllingFaction["west_cottages"] = ""
			ds := rs.DistrictSovereignty["west_cottages"]
			ds.ControllingFaction = ""
			rs.DistrictSovereignty["west_cottages"] = ds
			claimEvs, handled := w.ApplyRelationAction(rs, claimer.ID, sim.Action{
				Kind:      "claim_turf",
				FactionID: "merchants_guild",
				Location:  "west_cottages",
			})
			if !handled || claimEvs[0].Kind != "district_control_shifted" || rs.DistrictController("west_cottages") != "merchants_guild" {
				t.Fatalf("expected focal claim_turf to claim unclaimed west_cottages, got %+v controller=%q", claimEvs, rs.DistrictController("west_cottages"))
			}
		}
	})

	t.Run("JSONRoundTripCloneAndNilSafety", func(t *testing.T) {
		if sim.CloneRelationEvoState(nil) != nil {
			t.Fatal("CloneRelationEvoState(nil) must return nil")
		}
		var nilRS *sim.RelationEvoState
		nilRS.EnsureKinshipSeeded(nil)
		if nilRS.ObserveCausalRelationEvents(nil, nil) != nil {
			t.Fatal("expected nil on nil state ObserveCausalRelationEvents")
		}
		if nilRS.DistrictController("market_street") != "" {
			t.Fatal("expected empty DistrictController on nil state")
		}

		w := sim.LargeWorld("rel-json", 1000)
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)
		w.GrantMonopolyCharter(rs, "merchants_guild", "choice_grain", "market_street", 30)
		rs.ObserveCausalRelationEvents(w, []sim.Event{{ID: 11, Tick: 1, ActorID: "p01", TargetID: "p02", Kind: "raided", Payload: map[string]any{"damage": 12}}})

		cloned := sim.CloneRelationEvoState(rs)
		raw, err := json.Marshal(cloned)
		if err != nil {
			t.Fatalf("marshal RelationEvoState failed: %v", err)
		}
		var decoded sim.RelationEvoState
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal RelationEvoState failed: %v", err)
		}
		reEncoded, err := json.Marshal(&decoded)
		if err != nil {
			t.Fatalf("re-marshal decoded RelationEvoState failed: %v", err)
		}
		if string(raw) != string(reEncoded) {
			t.Fatalf("JSON round-trip mismatch:\nwant=%s\ngot=%s", string(raw), string(reEncoded))
		}
		if !reflect.DeepEqual(rs.ControllingFaction, decoded.ControllingFaction) ||
			!reflect.DeepEqual(rs.FactionLeader, decoded.FactionLeader) ||
			!reflect.DeepEqual(rs.MonopolyCharters, decoded.MonopolyCharters) ||
			!reflect.DeepEqual(rs.Grudges, decoded.Grudges) {
			t.Fatalf("decoded maps/slices mismatch after JSON round-trip")
		}
	})

	t.Run("GenreDecoupledSciFiPack", func(t *testing.T) {
		cat := sim.DefaultCatalog()
		cat.Items = map[string]sim.ItemSpec{
			"ration_pack": {ID: "ration_pack", Name: "Hydroponic Rations", Tier: 1, Edible: true, HungerRelief: 1, Tradeable: true, BatchQty: 2, BatchPrice: 2},
			"fusion_cell": {ID: "fusion_cell", Name: "Fusion Cell", Tier: 2, Tradeable: true, BatchQty: 1, BatchPrice: 10},
		}
		w := &sim.World{
			ID:     "orbital-station",
			NextID: 1,
			Map: sim.WorldMap{
				Districts: []sim.District{
					{ID: "docking_ring", Name: "Docking Ring", Features: []string{"market", "canal_wharf"}},
				},
			},
			Catalog: cat,
			Factions: []sim.Faction{
				{ID: "dock_union", Name: "Dockers Union", District: "docking_ring", Members: []string{"s1", "s2"}, Level: 2, Influence: 60},
			},
			Actors: []sim.Actor{
				{ID: "s1", Name: "Elena Vance", Role: "peddler", Location: "docking_ring", AgeYears: 30, Level: 2, Health: 100, Energy: 90, Money: 120, Guilds: []string{"dock_union"}, Professions: map[string]int{"peddler": 2}, Trust: map[string]int{}, Items: map[string]int{}},
				{ID: "s2", Name: "Marcus Vance", Role: "guard", Location: "docking_ring", AgeYears: 28, Level: 1, Health: 100, Energy: 90, Money: 40, Guilds: []string{"dock_union"}, Professions: map[string]int{"guard": 1}, Trust: map[string]int{}, Items: map[string]int{}},
			},
			DistrictStates: map[string]sim.DistrictState{
				"docking_ring": {DistrictID: "docking_ring", Sanitation: 80, Corruption: 10},
			},
		}
		rs := sim.NewRelationEvoState()
		rs.EnsureKinshipSeeded(w)
		if rs.Kinship["s1"].ClanID != rs.Kinship["s2"].ClanID {
			t.Fatalf("expected shared surname Vance to map to same clan: %q vs %q", rs.Kinship["s1"].ClanID, rs.Kinship["s2"].ClanID)
		}
		evs, ok := w.GrantMonopolyCharter(rs, "dock_union", "fusion_cell", "docking_ring", 25)
		if !ok || len(evs) != 1 || evs[0].Kind != "monopoly_chartered" {
			t.Fatalf("expected genre-decoupled sci-fi monopoly charter to succeed, got ok=%v evs=%+v", ok, evs)
		}
		if w.TotalCoins() != 160 {
			t.Fatalf("expected sci-fi TotalCoins=160 conserved, got %d", w.TotalCoins())
		}
	})
}

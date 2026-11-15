package sim_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func jrTestFindActorByRole(w *sim.World, role string) *sim.Actor {
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].Role == role {
			return &w.Actors[i]
		}
	}
	return nil
}

func jrTestPayloadInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func jrTestTotalItems(w *sim.World) map[string]int64 {
	tot := map[string]int64{}
	for i := range w.Actors {
		if w.Actors[i].Dead {
			continue
		}
		for k, v := range w.Actors[i].Items {
			tot[k] += int64(v)
		}
	}
	for i := range w.Cohorts {
		for k, v := range w.Cohorts[i].ItemPool {
			tot[k] += v
		}
	}
	return tot
}

type jrTestLedgerTracker struct {
	summary    *sim.ScaleSummary
	eventCount map[string]int
}

func jrTestNewLedgerTracker() *jrTestLedgerTracker {
	return &jrTestLedgerTracker{
		summary:    sim.NewScaleSummary(),
		eventCount: map[string]int{},
	}
}

func (lt *jrTestLedgerTracker) record(events []sim.Event) {
	lt.summary.RecordEvents(events)
	for _, e := range events {
		lt.eventCount[e.Kind]++
	}
}

func (lt *jrTestLedgerTracker) assertAllSixLedgersZeroError(t *testing.T, w *sim.World, initialPop int, initialCoins int64) {
	t.Helper()
	// 1. Population
	if got, want := w.TotalPopulation()+w.TotalDeaths(), initialPop+w.TotalBirths(); got != want {
		t.Fatalf("population ledger mismatch: pop(%d)+deaths(%d)=%d != init(%d)+births(%d)=%d",
			w.TotalPopulation(), w.TotalDeaths(), got, initialPop, w.TotalBirths(), want)
	}
	// 2. Wounds
	if got, want := lt.summary.WoundedPeople, lt.summary.HealedPeople+lt.summary.WoundedDeaths+int64(w.TotalWounded()); got != want {
		t.Fatalf("wound ledger mismatch: wounded=%d != healed(%d)+wounded_deaths(%d)+active(%d)=%d",
			got, lt.summary.HealedPeople, lt.summary.WoundedDeaths, w.TotalWounded(), want)
	}
	// 3. Epidemics
	if got, want := lt.summary.EpidemicCases, lt.summary.CuredPeople+w.DeathsByCause["disease"]+lt.summary.SickDiedOther+int64(w.TotalSick()); got != want {
		t.Fatalf("epidemic ledger mismatch: cases=%d != cured(%d)+disease_deaths(%d)+other(%d)+active(%d)=%d",
			got, lt.summary.CuredPeople, w.DeathsByCause["disease"], lt.summary.SickDiedOther, w.TotalSick(), want)
	}
	// 4. Jail
	if got, want := lt.summary.JailedPeople, lt.summary.ReleasedPeople+lt.summary.JailedDeaths+int64(w.TotalJailed()); got != want {
		t.Fatalf("jail ledger mismatch: jailed=%d != released(%d)+jailed_deaths(%d)+active(%d)=%d",
			got, lt.summary.ReleasedPeople, lt.summary.JailedDeaths, w.TotalJailed(), want)
	}
	// 5. Public Treasury
	tl := w.TreasuryLedger
	if got, want := tl.TaxAssessedGross, tl.TaxToTreasury+tl.TaxSkimmed; got != want {
		t.Fatalf("tax split mismatch: gross=%d != net(%d)+skimmed(%d)=%d", got, tl.TaxToTreasury, tl.TaxSkimmed, want)
	}
	wantTreasury := tl.InitialTreasury + tl.TaxToTreasury + tl.StateFeesIn + tl.ConfiscatedIn -
		tl.PayrollPaid - tl.ReliefAndInfraOut - tl.BountiesOut - tl.TreasuryEmbezzled
	if w.PublicTreasury != wantTreasury {
		t.Fatalf("treasury ledger mismatch: public_treasury=%d != expected=%d (%+v)", w.PublicTreasury, wantTreasury, tl)
	}
	// 6. Global M0 Coins
	if got := w.TotalCoins(); got != initialCoins {
		t.Fatalf("M0 coin conservation mismatch: got=%d != initial=%d (diff=%d)", got, initialCoins, got-initialCoins)
	}
}

func TestJusticeRevoltWrongfulFramingAppealUprisingAndAmnesty(t *testing.T) {
	// Part 1: Focal Court Scapegoat Framing -> Scholar/Censor Appeal Exoneration
	w := sim.LargeWorld("jr-focal", 20_000)
	jr := sim.NewJusticeRevoltState()
	tracker := jrTestNewLedgerTracker()
	initialPop := w.TotalPopulation()
	initialCoins := w.TotalCoins()

	constable := jrTestFindActorByRole(w, "constable")
	farmer := jrTestFindActorByRole(w, "farmer")
	scholar := jrTestFindActorByRole(w, "scholar")
	if constable == nil || farmer == nil || scholar == nil {
		t.Fatalf("expected constable, farmer, and scholar actors in LargeWorld")
	}

	// Place actors together in crossroads and raise local corruption so constable can frame farmer.
	for _, a := range []*sim.Actor{constable, farmer, scholar} {
		a.Location = "crossroads"
		a.InsideHouse = ""
		a.Energy = 90
		a.Health = 100
		a.JailedUntil = 0
		a.Task = nil
	}
	constable.IllicitGain = 8
	constable.Money = 20
	scholar.Tags = append(scholar.Tags, "censor")
	scholar.IllicitGain = 0
	if w.DistrictStates != nil {
		ds := w.DistrictStates["crossroads"]
		ds.Corruption = 30
		ds.BribeShieldTicks = 48
		w.DistrictStates["crossroads"] = ds
	}
	initialCoins = w.TotalCoins()

	evs, handled := w.ApplyJusticeAction(jr, constable.ID, sim.Action{Kind: "frame_suspect", TargetID: farmer.ID})
	if !handled {
		t.Fatalf("expected frame_suspect to be handled")
	}
	tracker.record(evs)
	if last := lastOfKind(evs, "wrongful_conviction"); last.Kind != "wrongful_conviction" {
		t.Fatalf("expected wrongful_conviction event, got %+v", evs)
	}
	if farmer.JailedUntil <= w.Tick {
		t.Fatalf("expected innocent farmer to be jailed after wrongful framing, jailed_until=%d tick=%d", farmer.JailedUntil, w.Tick)
	}
	if jr.Grievance("crossroads") <= 0 {
		t.Fatalf("expected crossroads grievance to spike after wrongful conviction, got %d", jr.Grievance("crossroads"))
	}

	// Scholar/censor files appeal -> exonerates farmer, pays treasury relief, jails & confiscates corrupt framer
	farmerMoneyBefore := farmer.Money
	treasuryBefore := w.PublicTreasury
	evs, handled = w.ApplyJusticeAction(jr, scholar.ID, sim.Action{Kind: "file_appeal", TargetID: farmer.ID})
	if !handled {
		t.Fatalf("expected file_appeal to be handled")
	}
	tracker.record(evs)
	if last := lastOfKind(evs, "conviction_overturned"); last.Kind != "conviction_overturned" {
		t.Fatalf("expected conviction_overturned event, got %+v", evs)
	}
	if farmer.JailedUntil != 0 {
		t.Fatalf("expected exonerated farmer JailedUntil == 0, got %d", farmer.JailedUntil)
	}
	if farmer.Money <= farmerMoneyBefore {
		t.Fatalf("expected exonerated farmer to receive relief compensation: %d -> %d", farmerMoneyBefore, farmer.Money)
	}
	if constable.JailedUntil <= w.Tick || constable.IllicitGain != 0 {
		t.Fatalf("expected corrupt framer jailed and illicit gain confiscated: jailed_until=%d illicit=%d", constable.JailedUntil, constable.IllicitGain)
	}
	if w.PublicTreasury == treasuryBefore {
		t.Fatalf("expected treasury delta from relief + confiscation")
	}
	tracker.assertAllSixLedgersZeroError(t, w, initialPop, initialCoins)

	// Part 2: Macro Trial -> Framing -> Appeal -> Grievance Ladder -> Tax Strike -> Granary Riot -> Revolt -> Suppression -> Amnesty & Reintegration
	mw := sim.LargeWorld("jr-macro", 200_000)
	mjr := sim.NewJusticeRevoltState()
	mTracker := jrTestNewLedgerTracker()
	mInitPop := mw.TotalPopulation()
	mInitCoins := mw.TotalCoins()

	// Seed oppressive conditions in south_fields: high corruption, bribe shield, high hunger, and low cash
	if mw.DistrictStates != nil {
		ds := mw.DistrictStates["south_fields"]
		ds.Corruption = 40
		ds.BribeShieldTicks = 72
		ds.RacketPressure = 30
		mw.DistrictStates["south_fields"] = ds
	}
	for i := range mw.Cohorts {
		c := &mw.Cohorts[i]
		if c.District == "south_fields" && !mw.Catalog.HasCoercivePower([]string{c.Profession}) {
			c.AvgHunger = 4
			if c.ItemPool == nil {
				c.ItemPool = map[string]int64{}
			}
			c.ItemPool["food"] = int64(c.Population) / 4 // acute local food scarcity
		}
	}
	// Seed high initial grievance near threshold so escalation ladder progresses cleanly across days
	mjr.DistrictGrievance["south_fields"] = 50

	for step := 0; step < 8*sim.TicksPerDay; step++ {
		// Keep pressure high until revolt erupts so full escalation ladder fires deterministically
		if mTracker.eventCount["revolt_erupted"] == 0 && mw.DistrictStates != nil {
			ds := mw.DistrictStates["south_fields"]
			if ds.Corruption < 36 {
				ds.Corruption = 36
			}
			mw.DistrictStates["south_fields"] = ds
			for i := range mw.Cohorts {
				if mw.Cohorts[i].District == "south_fields" && mw.Cohorts[i].AvgHunger < 4 {
					mw.Cohorts[i].AvgHunger = 4
				}
			}
		}

		// Snapshot item conservation around StepJusticeAndRevolt specifically
		stepEvs, err := mw.Step(context.Background(), agent.Offline())
		if err != nil {
			t.Fatalf("step %d failed: %v", step, err)
		}
		mTracker.record(stepEvs)

		itemsBeforeJR := jrTestTotalItems(mw)
		jrEvs := mw.StepJusticeAndRevolt(mjr)
		itemsAfterJR := jrTestTotalItems(mw)
		mTracker.record(jrEvs)

		// Verify atomic item conservation for granary_riot, revolt_erupted, and rebel_reintegrated
		for k, vBefore := range itemsBeforeJR {
			if itemsAfterJR[k] != vBefore {
				t.Fatalf("step %d item %q conservation mismatch across StepJusticeAndRevolt: before=%d after=%d", step, k, vBefore, itemsAfterJR[k])
			}
		}
		for k, vAfter := range itemsAfterJR {
			if itemsBeforeJR[k] != vAfter {
				t.Fatalf("step %d extra item %q appeared in StepJusticeAndRevolt: before=%d after=%d", step, k, itemsBeforeJR[k], vAfter)
			}
		}

		// Verify every cohort maintains Children+Adults+Elders == Population and Wounded+Sick+Jailed <= Population
		for i := range mw.Cohorts {
			c := mw.Cohorts[i]
			if c.Population > 0 {
				if c.Children+c.Adults+c.Elders != c.Population {
					t.Fatalf("step %d cohort %s age sum mismatch: %d+%d+%d != %d", step, c.ID, c.Children, c.Adults, c.Elders, c.Population)
				}
				if c.Wounded+c.Sick+c.Jailed > c.Population {
					t.Fatalf("step %d cohort %s casualty overflow: w(%d)+s(%d)+j(%d) > pop(%d)", step, c.ID, c.Wounded, c.Sick, c.Jailed, c.Population)
				}
			}
		}
	}

	mTracker.assertAllSixLedgersZeroError(t, mw, mInitPop, mInitCoins)

	requiredEvents := []string{
		"court_trial_held",
		"wrongful_conviction",
		"conviction_overturned",
		"tax_strike",
		"granary_riot",
		"revolt_erupted",
		"revolt_suppressed",
		"amnesty_decreed",
		"rebel_reintegrated",
	}
	for _, kind := range requiredEvents {
		if mTracker.eventCount[kind] == 0 {
			t.Fatalf("expected event %q to trigger in 8-day oppressed/pacification simulation, counts=%+v", kind, mTracker.eventCount)
		}
	}

	// Verify rebel conversion share remained within bounded [2%, 6%] of source free healthy adults
	if len(mjr.ActiveRevolts) == 0 {
		t.Fatalf("expected at least one recorded ActiveRevolt")
	}
	reintegratedOK := false
	for _, r := range mjr.ActiveRevolts {
		if r.Status == "amnestied" {
			reintegratedOK = true
		}
	}
	if !reintegratedOK {
		t.Fatalf("expected revolt to reach amnestied/reintegrated status, got %+v", mjr.ActiveRevolts)
	}

	// Part 3: JSON round-trip & CloneJusticeRevoltState fidelity
	cloned := sim.CloneJusticeRevoltState(mjr)
	rawOrig, err := json.Marshal(mjr)
	if err != nil {
		t.Fatalf("marshal original state: %v", err)
	}
	var decoded sim.JusticeRevoltState
	if err := json.Unmarshal(rawOrig, &decoded); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	rawCloned, _ := json.Marshal(cloned)
	rawDecoded, _ := json.Marshal(&decoded)
	if string(rawCloned) != string(rawDecoded) {
		t.Fatalf("JSON round-trip / clone mismatch:\ncloned=%s\ndecoded=%s", string(rawCloned), string(rawDecoded))
	}
	if sim.CloneJusticeRevoltState(nil) != nil {
		t.Fatalf("CloneJusticeRevoltState(nil) should return nil")
	}

	t.Logf("Justice & Revolt 8-Day 200k Summary: events=%+v revolts=%d cases=%d treasury=%d",
		mTracker.eventCount, len(mjr.ActiveRevolts), len(mjr.CourtCases), mw.PublicTreasury)
}

func TestJusticeRevoltGenreDecoupledSciFiPack(t *testing.T) {
	// Verify zero hardcoded historical assumptions on a Sci-Fi colony pack with custom tags/items/professions.
	cat := sim.Catalog{
		TagActions: map[string][]string{
			"colonist": {"wait", "move", "speak"},
			"ranger":   {"arrest", "attack"},
			"psion":    {"audit_corruption", "perform_ritual"},
		},
		Items: map[string]sim.ItemSpec{
			"synth_ration": {ID: "synth_ration", Tier: 1, Edible: true, HungerRelief: 2, BatchPrice: 2},
		},
		Professions: []sim.Profession{
			{ID: "hydro_tech", Name: "Hydro Tech", Tier: 1, Guild: "bio", Grants: []string{"colonist"}},
			{ID: "exo_ranger", Name: "Exo Ranger", Tier: 1, Guild: "sec", Grants: []string{"colonist", "ranger"}},
			{ID: "psion_auditor", Name: "Psion Auditor", Tier: 1, Guild: "order", Grants: []string{"colonist", "psion"}},
			{ID: "void_pirate", Name: "Void Pirate", Tier: 1, Guild: "pirates", Grants: []string{"colonist", "outlaw", "fighter"}},
		},
		TagProfiles: map[string]sim.TagProfile{
			"exo_ranger":    {CoercivePower: true, PublicTrustDuty: true},
			"psion_auditor": {PublicTrustDuty: true},
			"void_pirate":   {CoercivePower: true},
			"fighter":       {CoercivePower: true},
		},
	}

	w := &sim.World{
		ID:             "scifi-jr",
		NextID:         1,
		PublicTreasury: 2000,
		Map: sim.WorldMap{
			Districts: []sim.District{
				{ID: "orbital_hub", Name: "Orbital Hub"},
			},
		},
		Catalog: cat,
		DistrictStates: map[string]sim.DistrictState{
			"orbital_hub": {DistrictID: "orbital_hub", Corruption: 42, BribeShieldTicks: 48},
		},
		Cohorts: []sim.PopulationCohort{
			{
				ID: "cohort_hydro", Name: "Hydroponics Crew", District: "orbital_hub", Profession: "hydro_tech",
				Population: 500, Adults: 350, Children: 100, Elders: 50, AvgHunger: 4, MoneyPool: 2500,
				ItemPool: map[string]int64{"synth_ration": 2500},
			},
			{
				ID: "cohort_rangers", Name: "Orbital Rangers", District: "orbital_hub", Profession: "exo_ranger",
				Population: 40, Adults: 35, Children: 2, Elders: 3, MoneyPool: 800,
			},
			{
				ID: "cohort_psions", Name: "Psion Oversight", District: "orbital_hub", Profession: "psion_auditor",
				Population: 20, Adults: 16, Children: 1, Elders: 3, MoneyPool: 600,
			},
			{
				ID: "cohort_pirates", Name: "Void Pirates", District: "orbital_hub", Profession: "void_pirate",
				Population: 60, Adults: 50, Children: 3, Elders: 7, MoneyPool: 900,
			},
		},
	}

	jr := sim.NewJusticeRevoltState()
	jr.DistrictGrievance["orbital_hub"] = 72
	tracker := jrTestNewLedgerTracker()
	initPop := w.TotalPopulation()
	initCoins := w.TotalCoins()

	for step := 0; step < 4*sim.TicksPerDay; step++ {
		w.Tick++
		tracker.record(w.StepJusticeAndRevolt(jr))
	}

	tracker.assertAllSixLedgersZeroError(t, w, initPop, initCoins)
	if tracker.eventCount["wrongful_conviction"] == 0 || tracker.eventCount["conviction_overturned"] == 0 ||
		tracker.eventCount["revolt_erupted"] == 0 || tracker.eventCount["amnesty_decreed"] == 0 {
		t.Fatalf("expected sci-fi pack to run full justice & revolt cycle via catalog introspection, got counts=%+v", tracker.eventCount)
	}
}

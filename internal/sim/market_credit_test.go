package sim_test

import (
	"encoding/json"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func mcTestCohortByID(w *sim.World, id string) *sim.PopulationCohort {
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == id {
			return &w.Cohorts[i]
		}
	}
	return nil
}

func mcTestHouseByID(w *sim.World, id string) *sim.House {
	for i := range w.Houses {
		if w.Houses[i].ID == id {
			return &w.Houses[i]
		}
	}
	return nil
}

func TestMarketCreditDynamicPricingHoardingUsuryAndAnnexation(t *testing.T) {
	// 1. Spec defaults, nil-safety, cloning, and JSON round-trip
	def := sim.DefaultMarketCreditSpec()
	eff := sim.EffectiveMarketCreditSpec(sim.MarketCreditSpec{})
	if eff != def {
		t.Fatalf("expected EffectiveMarketCreditSpec empty to equal default: got=%+v want=%+v", eff, def)
	}
	if sim.CloneMarketCreditState(nil) != nil {
		t.Fatal("expected CloneMarketCreditState(nil) == nil")
	}

	wNil := sim.Village("nil-check")
	if p := wNil.EffectiveMarketPrice(nil, "market_street", "food", 2); p != 2 {
		t.Fatalf("nil MarketCreditState should return base price 2, got %d", p)
	}
	if evs := wNil.StepMarketAndCredit(nil); len(evs) != 0 {
		t.Fatalf("nil StepMarketAndCredit should emit 0 events, got %d", len(evs))
	}
	if evs, handled := wNil.ApplyMarketCreditAction(nil, "p01", sim.Action{Kind: "borrow_loan"}); !handled || len(evs) == 0 || evs[0].Kind != "action_failed" {
		t.Fatalf("nil ApplyMarketCreditAction should handle borrow_loan with action_failed, got handled=%v evs=%+v", handled, evs)
	}
	if _, handled := wNil.ApplyMarketCreditAction(nil, "p01", sim.Action{Kind: "wait"}); handled {
		t.Fatal("unrelated action should return handled=false")
	}

	// 2. Dynamic pricing: bumper surplus (谷贱伤农) vs scarcity + disaster + canal disruption (米珠薪桂)
	w := sim.LargeWorld("mc-pricing", 2000)
	mc := sim.NewMarketCreditState()
	baseFoodPrice := w.Catalog.Items["food"].BatchPrice

	// Baseline equilibrium
	w.UpdateDistrictPrices(mc)
	eqPermille := mc.DistrictPrices["south_fields"]["food"]
	if eqPermille <= 0 {
		t.Fatalf("expected positive equilibrium permille, got %d", eqPermille)
	}

	// 2a. Bumper harvest surplus: flood south_fields and world cohorts with food -> price drops toward MinPricePermille
	for i := range w.Cohorts {
		w.Cohorts[i].ItemPool["food"] = int64(w.Cohorts[i].Population) * 40
	}
	w.UpdateDistrictPrices(mc)
	glutPermille := mc.DistrictPrices["south_fields"]["food"]
	glutPrice := w.EffectiveMarketPrice(mc, "south_fields", "food", baseFoodPrice)
	if glutPermille >= 900 || glutPrice > baseFoodPrice {
		t.Fatalf("expected bumper surplus price drop (<900 permille), got permille=%d price=%d (base=%d)", glutPermille, glutPrice, baseFoodPrice)
	}

	// 2b. Scarcity + active drought disaster -> surge well above base price
	for i := range w.Cohorts {
		w.Cohorts[i].ItemPool["food"] = int64(w.Cohorts[i].Population) / 2
	}
	for i := range w.Actors {
		w.Actors[i].Items["food"] = 0
	}
	dsSouth := w.DistrictStates["south_fields"]
	dsSouth.ActiveDisasters = []sim.ActiveDisaster{{ID: "drought", Name: "亢旱缺水", StartedTick: w.Tick, ExpiresTick: w.Tick + 200}}
	w.DistrictStates["south_fields"] = dsSouth

	w.UpdateDistrictPrices(mc)
	scarcityPermille := mc.DistrictPrices["south_fields"]["food"]
	scarcityPrice := w.EffectiveMarketPrice(mc, "south_fields", "food", baseFoodPrice)
	if scarcityPermille <= 1800 || scarcityPrice <= baseFoodPrice {
		t.Fatalf("expected scarcity+disaster price surge (>1800 permille), got permille=%d price=%d (base=%d)", scarcityPermille, scarcityPrice, baseFoodPrice)
	}

	// 2c. Canal / waterway route disruption shock on market_street
	dsSouth.ActiveDisasters = nil
	w.DistrictStates["south_fields"] = dsSouth
	for i := range w.Cohorts {
		w.Cohorts[i].ItemPool["food"] = int64(w.Cohorts[i].Population) * 8
	}
	w.UpdateDistrictPrices(mc)
	calmWharfPermille := mc.DistrictPrices["market_street"]["food"]

	dsWharf := w.DistrictStates["jade_river_wharf"]
	dsWharf.Weather = "storm"
	dsWharf.ActiveDisasters = []sim.ActiveDisaster{{ID: "river_flood", Name: "河决洪涝", StartedTick: w.Tick, ExpiresTick: w.Tick + 200}}
	w.DistrictStates["jade_river_wharf"] = dsWharf
	dsMarket := w.DistrictStates["market_street"]
	dsMarket.Weather = "frost_snow"
	w.DistrictStates["market_street"] = dsMarket

	w.UpdateDistrictPrices(mc)
	disruptedWharfPermille := mc.DistrictPrices["market_street"]["food"]
	if disruptedWharfPermille <= calmWharfPermille {
		t.Fatalf("expected canal route disruption to raise market_street price: calm=%d disrupted=%d", calmWharfPermille, disruptedWharfPermille)
	}

	// 2d. Genre-decoupled pricing on non-village catalog
	sciWorld := &sim.World{
		ID: "scifi-pricing",
		Map: sim.WorldMap{
			Districts: []sim.District{{ID: "hydro_dome", Name: "Hydro Dome"}, {ID: "ore_ring", Name: "Ore Ring"}},
			Roads:     []sim.Road{{From: "hydro_dome", To: "ore_ring", Ticks: 1, EnergyCost: 2, Medium: "shuttle_corridor", FreightCapacity: 100000}},
		},
		Catalog: sim.Catalog{
			Items: map[string]sim.ItemSpec{
				"synth_ration": {ID: "synth_ration", Tier: 1, Edible: true, HungerRelief: 2, Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 4},
			},
			TagProfiles: map[string]sim.TagProfile{
				"colonist": {Consumes: []string{"synth_ration"}, TargetStock: map[string]int{"synth_ration": 8}},
			},
			Weathers: map[string]sim.WeatherSpec{
				"solar_flare": {ID: "solar_flare", RouteCapacityPct: map[string]int{"shuttle_corridor": 40}, RouteHazardPermille: map[string]int{"shuttle_corridor": 30}},
			},
			Disasters: []sim.DisasterSpec{
				{ID: "hull_microbreach", ReliefItem: "synth_ration"},
			},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "c_colonists", District: "ore_ring", Profession: "colonist", Population: 500, ItemPool: map[string]int64{"synth_ration": 200}},
		},
		DistrictStates: map[string]sim.DistrictState{
			"ore_ring": {DistrictID: "ore_ring", Weather: "solar_flare", ActiveDisasters: []sim.ActiveDisaster{{ID: "hull_microbreach", ExpiresTick: 100}}},
		},
	}
	sciMC := sim.NewMarketCreditState()
	sciWorld.UpdateDistrictPrices(sciMC)
	if sciPrice := sciWorld.EffectiveMarketPrice(sciMC, "ore_ring", "synth_ration", 2); sciPrice <= 2 {
		t.Fatalf("expected genre-decoupled sci-fi scarcity+disaster price > 2, got %d (permille=%d)", sciPrice, sciMC.DistrictPrices["ore_ring"]["synth_ration"])
	}

	// 3. Merchant Hoarding during rising scarcity & Dump when price peaks
	wh := sim.LargeWorld("mc-hoard", 1500)
	hmc := sim.NewMarketCreditState()
	hmc.Spec = sim.MarketCreditSpec{
		HoardTriggerPermille:   1150,
		HoardReleasePermille:   1850,
		HoardSurplusMultiplier: 2,
	}
	peddlerCohort := mcTestCohortByID(wh, "cohort_market_peddlers")
	if peddlerCohort == nil {
		t.Fatal("missing cohort_market_peddlers")
	}
	// Set moderate scarcity across market_street (~1250-1500 permille) while merchant cohort holds extra food
	for i := range wh.Cohorts {
		wh.Cohorts[i].ItemPool["food"] = int64(wh.Cohorts[i].Population) * 4
	}
	for i := range wh.Actors {
		wh.Actors[i].Items["food"] = 2
	}
	peddlerCohort.ItemPool["food"] = int64(peddlerCohort.Population) * 7
	traderActor := actorByID(wh, "p27")
	if traderActor == nil {
		t.Fatal("missing p27 trader actor")
	}
	traderActor.Location = "market_street"
	traderActor.InsideHouse = ""
	traderActor.Money = 40
	traderActor.Items["food"] = 20

	// Clear any weather/disaster first so price is in moderate scarcity band [1150, 1850)
	for dID, ds := range wh.DistrictStates {
		ds.Weather = "clear"
		ds.ActiveDisasters = nil
		wh.DistrictStates[dID] = ds
	}
	// Make market_street slightly short on food overall so price triggers hoarding without immediate peak dump
	for i := range wh.Cohorts {
		if wh.Cohorts[i].District == "market_street" && wh.Cohorts[i].ID != peddlerCohort.ID {
			wh.Cohorts[i].ItemPool["food"] = 0
		}
	}
	wh.UpdateDistrictPrices(hmc)
	pBeforeHoard := hmc.DistrictPrices["market_street"]["food"]
	if pBeforeHoard < 1150 || pBeforeHoard >= 1850 {
		t.Fatalf("test setup expected market_street food permille in [1150, 1850), got %d", pBeforeHoard)
	}

	beforePeddlerStock := peddlerCohort.ItemPool["food"]
	beforeActorStock := traderActor.Items["food"]
	hoardEvs := wh.StepMarketAndCredit(hmc)
	if ev := lastOfKind(hoardEvs, "merchant_hoarded"); ev.Kind != "merchant_hoarded" {
		t.Fatalf("expected merchant_hoarded event, got %+v", hoardEvs)
	}
	hoardedTotal := wh.TotalHoardedStock(hmc, "food")
	if hoardedTotal <= 0 {
		t.Fatalf("expected positive hoarded food stock, got %d", hoardedTotal)
	}
	if peddlerCohort.ItemPool["food"]+hmc.HoardedStock[peddlerCohort.ID]["food"] != beforePeddlerStock {
		t.Fatalf("cohort item conservation failed during hoarding: pool=%d hoarded=%d before=%d",
			peddlerCohort.ItemPool["food"], hmc.HoardedStock[peddlerCohort.ID]["food"], beforePeddlerStock)
	}
	if traderActor.Items["food"]+int(hmc.HoardedStock[traderActor.ID]["food"]) != beforeActorStock {
		t.Fatalf("actor item conservation failed during hoarding: items=%d hoarded=%d before=%d",
			traderActor.Items["food"], hmc.HoardedStock[traderActor.ID]["food"], beforeActorStock)
	}

	// Spike price past HoardReleasePermille via severe famine + disaster -> merchants dump hoarded stock!
	for i := range wh.Cohorts {
		if wh.Cohorts[i].ID != peddlerCohort.ID {
			wh.Cohorts[i].ItemPool["food"] = 0
		}
	}
	dsMkt := wh.DistrictStates["market_street"]
	dsMkt.ActiveDisasters = []sim.ActiveDisaster{{ID: "drought", Name: "亢旱缺水", StartedTick: wh.Tick, ExpiresTick: wh.Tick + 200}}
	wh.DistrictStates["market_street"] = dsMkt
	wh.Tick += 4
	releaseEvs := wh.StepMarketAndCredit(hmc)
	if ev := lastOfKind(releaseEvs, "hoard_released"); ev.Kind != "hoard_released" {
		t.Fatalf("expected hoard_released event at price peak (%d permille), got %+v", hmc.DistrictPrices["market_street"]["food"], releaseEvs)
	}
	if got := wh.TotalHoardedStock(hmc, "food"); got != 0 {
		t.Fatalf("expected all hoarded food released at peak, remaining=%d", got)
	}
	if peddlerCohort.ItemPool["food"] != beforePeddlerStock || traderActor.Items["food"] != beforeActorStock {
		t.Fatalf("expected exact item conservation after hoard release: cohort=%d(want %d) actor=%d(want %d)",
			peddlerCohort.ItemPool["food"], beforePeddlerStock, traderActor.Items["food"], beforeActorStock)
	}

	// 4. Focal Actor Usury Loans, Repayment, Collateral Foreclosure, and Daily Tenant Rent
	wf := sim.Village("mc-focal-credit")
	fmc := sim.NewMarketCreditState()
	initialFocalCoins := wf.TotalCoins()

	lender := actorByID(wf, "p27")
	borrower := actorByID(wf, "p01")
	borrower2 := actorByID(wf, "p03")
	if lender == nil || borrower == nil || borrower2 == nil {
		t.Fatal("missing focal test actors")
	}
	lender.Money = 120
	borrower.Money = 5
	borrower2.Money = 10
	initialFocalCoins = wf.TotalCoins()

	// 4a. Solvent focal loan & repayment
	evs, ok := wf.ApplyMarketCreditAction(fmc, borrower2.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID, Quantity: 20, HouseID: "none"})
	if !ok || lastOfKind(evs, "loan_issued").Kind != "loan_issued" {
		t.Fatalf("expected loan_issued, got ok=%v evs=%+v", ok, evs)
	}
	if borrower2.Money != 30 || lender.Money != 100 || wf.TotalCoins() != initialFocalCoins {
		t.Fatalf("loan issuance coin mismatch: borrower2=%d lender=%d total=%d(want %d)", borrower2.Money, lender.Money, wf.TotalCoins(), initialFocalCoins)
	}
	evs, ok = wf.ApplyMarketCreditAction(fmc, borrower2.ID, sim.Action{Kind: "repay_loan", TargetID: lender.ID, Quantity: 20})
	if !ok || lastOfKind(evs, "loan_repaid").Kind != "loan_repaid" {
		t.Fatalf("expected loan_repaid, got ok=%v evs=%+v", ok, evs)
	}
	if fmc.Loans[0].Status != "repaid" || wf.TotalCoins() != initialFocalCoins {
		t.Fatalf("expected repaid loan & exact conservation: loan=%+v total=%d", fmc.Loans[0], wf.TotalCoins())
	}

	// 4b. Collateralized loan -> overdue -> manual foreclose_collateral -> tenant keeps access & pays daily rent
	h01 := mcTestHouseByID(wf, "h01")
	if h01 == nil || h01.OwnerID != borrower.ID {
		t.Fatalf("expected p01 to own h01 initially, got %+v", h01)
	}
	evs, ok = wf.ApplyMarketCreditAction(fmc, borrower.ID, sim.Action{Kind: "borrow_loan", TargetID: lender.ID, Quantity: 25, HouseID: "h01"})
	if !ok || lastOfKind(evs, "loan_issued").Kind != "loan_issued" {
		t.Fatalf("expected collateralized loan_issued, got %+v", evs)
	}
	// Premature foreclosure must fail
	if earlyEvs, _ := wf.ApplyMarketCreditAction(fmc, lender.ID, sim.Action{Kind: "foreclose_collateral", TargetID: borrower.ID, HouseID: "h01"}); earlyEvs[0].Kind != "action_failed" {
		t.Fatalf("expected premature foreclosure to fail, got %+v", earlyEvs)
	}
	// Drain borrower cash so it defaults past LoanTermTicks, then test explicit foreclose_collateral
	lender.Money += borrower.Money
	borrower.Money = 0
	wf.Tick = int64(fmc.Spec.LoanTermTicks)
	foreEvs, ok := wf.ApplyMarketCreditAction(fmc, lender.ID, sim.Action{Kind: "foreclose_collateral", TargetID: borrower.ID, HouseID: "h01"})
	if !ok || lastOfKind(foreEvs, "house_foreclosed").Kind != "house_foreclosed" {
		t.Fatalf("expected house_foreclosed, got ok=%v evs=%+v", ok, foreEvs)
	}
	if h01.OwnerID != lender.ID {
		t.Fatalf("expected h01 OwnerID transferred to %s, got %s", lender.ID, h01.OwnerID)
	}
	if !h01.CanAccess(borrower.ID) || !h01.CanAccess(lender.ID) {
		t.Fatalf("expected both foreclosed tenant %s and new owner %s to retain house access", borrower.ID, lender.ID)
	}

	// Give tenant coins and run daily step -> must pay daily rent to new owner while conserving M0!
	borrower.Money = 20
	initialFocalCoins = wf.TotalCoins()
	lenderBeforeRent := lender.Money
	wf.Tick += sim.TicksPerDay
	rentEvs := wf.StepMarketAndCredit(fmc)
	if ev := lastOfKind(rentEvs, "house_rent_paid"); ev.Kind != "house_rent_paid" {
		t.Fatalf("expected house_rent_paid event, got %+v", rentEvs)
	}
	if lender.Money <= lenderBeforeRent || borrower.Money >= 20 || wf.TotalCoins() != initialFocalCoins {
		t.Fatalf("expected daily tenant rent transfer conserving coins: lender=%d(was %d) borrower=%d total=%d(want %d)",
			lender.Money, lenderBeforeRent, borrower.Money, wf.TotalCoins(), initialFocalCoins)
	}

	// 5. Macro PopulationCohort Usury Credit, Interest, Default -> Land Annexation + Daily Rent + Gini + JSON
	wm := sim.LargeWorld("mc-macro-annex", 4000)
	mmc := sim.NewMarketCreditState()
	initialMacroCoins := wm.TotalCoins()
	initialMacroPop := wm.TotalPopulation()

	// Set equal wealth first to test Gini == 0 baseline on uniform cohort
	wEqual := &sim.World{
		ID: "gini-eq",
		Actors: []sim.Actor{
			{ID: "a1", Money: 10},
			{ID: "a2", Money: 10},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "c1", Population: 100, MoneyPool: 1000},
		},
	}
	if g := wEqual.WealthGiniPermille(nil); g != 0 {
		t.Fatalf("expected uniform wealth Gini == 0, got %d", g)
	}
	wSkew := &sim.World{
		ID: "gini-skew",
		Actors: []sim.Actor{
			{ID: "a1", Money: 1000},
		},
		Cohorts: []sim.PopulationCohort{
			{ID: "c1", Population: 99, MoneyPool: 0},
		},
	}
	if g := wSkew.WealthGiniPermille(nil); g < 950 {
		t.Fatalf("expected extreme concentration Gini >= 950, got %d", g)
	}

	// Make south farmers cash-distressed while market peddlers are liquid creditors
	farmers := mcTestCohortByID(wm, "cohort_south_farmers")
	merchants := mcTestCohortByID(wm, "cohort_market_peddlers")
	chiefs := mcTestCohortByID(wm, "cohort_market_chiefs")
	if farmers == nil || merchants == nil || chiefs == nil {
		t.Fatal("missing expected macro cohorts")
	}
	// Keep other cohorts above distress floor so test isolates farmer borrowing cleanly
	for i := range wm.Cohorts {
		c := &wm.Cohorts[i]
		c.MoneyPool = int64(c.Population) * 10
	}
	farmers.MoneyPool = int64(farmers.Population) // distressed (1 coin/cap < 6/cap floor)
	farmers.ItemPool["food"] = int64(farmers.Population) * 8
	merchants.MoneyPool = int64(merchants.Population) * 80
	chiefs.MoneyPool = int64(chiefs.Population) * 80
	initialMacroCoins = wm.TotalCoins()
	giniBefore := wm.WealthGiniPermille(mmc)

	// Day 0: StepMarketAndCredit issues cohort usury loan to distressed farmers
	var allMacroEvs []sim.Event
	allMacroEvs = append(allMacroEvs, wm.StepMarketAndCredit(mmc)...)
	if ev := lastOfKind(allMacroEvs, "cohort_loan_issued"); ev.Kind != "cohort_loan_issued" {
		t.Fatalf("expected cohort_loan_issued for distressed farmers, got %+v", allMacroEvs)
	}
	if got := wm.TotalCoins(); got != initialMacroCoins {
		t.Fatalf("M0 coin drift after cohort loan issuance: got=%d want=%d", got, initialMacroCoins)
	}

	// Drain farmer cash each day so they cannot repay before LoanTermTicks -> forces default & land annexation!
	// (Shift cash to another non-borrowing cohort to keep M0 conserved!)
	sinkCohort := mcTestCohortByID(wm, "cohort_crossroads_scholars")
	if sinkCohort == nil {
		t.Fatal("missing sink cohort")
	}
	for day := 1; day <= 6; day++ {
		if farmers.MoneyPool > int64(farmers.Population) {
			excess := farmers.MoneyPool - int64(farmers.Population)
			farmers.MoneyPool -= excess
			sinkCohort.MoneyPool += excess
		}
		wm.Tick = int64(day) * sim.TicksPerDay
		stepEvs := wm.StepMarketAndCredit(mmc)
		allMacroEvs = append(allMacroEvs, stepEvs...)
		if got := wm.TotalCoins(); got != initialMacroCoins {
			t.Fatalf("day %d M0 coin drift: got=%d want=%d", day, got, initialMacroCoins)
		}
	}

	if ev := lastOfKind(allMacroEvs, "cohort_land_annexed"); ev.Kind != "cohort_land_annexed" {
		t.Fatalf("expected cohort_land_annexed after overdue insolvency, got events=%+v", allMacroEvs)
	}
	annexShare := mmc.AnnexedSharePermille[farmers.ID]
	creditorID := mmc.CreditorCohort[farmers.ID]
	if annexShare <= 0 || annexShare > mmc.Spec.MaxTenantSharePermille || creditorID == "" {
		t.Fatalf("unexpected annexation state for farmers: share=%d creditor=%s", annexShare, creditorID)
	}

	// Next day, give farmers harvest cash & grain surplus -> must pay daily rent (cash & output) to creditor cohort!
	if sinkCohort.MoneyPool >= int64(farmers.Population)*10 {
		give := int64(farmers.Population) * 10
		sinkCohort.MoneyPool -= give
		farmers.MoneyPool += give
	}
	farmers.ItemPool["food"] = int64(farmers.Population) * 10
	creditor := mcTestCohortByID(wm, creditorID)
	creditorCoinsBefore := creditor.MoneyPool
	creditorFoodBefore := creditor.ItemPool["food"]

	wm.Tick += sim.TicksPerDay
	rentDayEvs := wm.StepMarketAndCredit(mmc)
	if ev := lastOfKind(rentDayEvs, "cohort_rent_paid"); ev.Kind != "cohort_rent_paid" {
		t.Fatalf("expected cohort_rent_paid event, got %+v", rentDayEvs)
	}
	if creditor.MoneyPool <= creditorCoinsBefore || creditor.ItemPool["food"] <= creditorFoodBefore {
		t.Fatalf("expected creditor %s to receive cash and food rent: coins %d->%d food %d->%d",
			creditorID, creditorCoinsBefore, creditor.MoneyPool, creditorFoodBefore, creditor.ItemPool["food"])
	}
	if got := wm.TotalCoins(); got != initialMacroCoins {
		t.Fatalf("M0 coin drift after rent payment: got=%d want=%d", got, initialMacroCoins)
	}
	if got := wm.TotalPopulation(); got != initialMacroPop {
		t.Fatalf("population drift: got=%d want=%d", got, initialMacroPop)
	}
	giniAfter := wm.WealthGiniPermille(mmc)
	if giniAfter <= 0 || giniAfter == giniBefore {
		t.Logf("Gini changed from %d to %d permille", giniBefore, giniAfter)
	}

	// 6. Clone isolation and JSON serialization round-trip of MarketCreditState
	cloned := sim.CloneMarketCreditState(mmc)
	raw, err := json.Marshal(cloned)
	if err != nil {
		t.Fatalf("marshal MarketCreditState failed: %v", err)
	}
	var decoded sim.MarketCreditState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal MarketCreditState failed: %v", err)
	}
	if decoded.AnnexedSharePermille[farmers.ID] != annexShare || decoded.CreditorCohort[farmers.ID] != creditorID {
		t.Fatalf("JSON round-trip lost annexation state: got share=%d creditor=%s", decoded.AnnexedSharePermille[farmers.ID], decoded.CreditorCohort[farmers.ID])
	}
	if len(decoded.CohortDebts) != len(mmc.CohortDebts) || decoded.NextLoanID != mmc.NextLoanID {
		t.Fatalf("JSON round-trip lost cohort debts or NextLoanID: got=%d/%d want=%d/%d",
			len(decoded.CohortDebts), decoded.NextLoanID, len(mmc.CohortDebts), mmc.NextLoanID)
	}
	cloned.AnnexedSharePermille[farmers.ID] = 999
	if mmc.AnnexedSharePermille[farmers.ID] == 999 {
		t.Fatal("CloneMarketCreditState aliased AnnexedSharePermille map")
	}

	// 7. Also test automatic focal house foreclosure inside StepMarketAndCredit
	wa := sim.Village("mc-auto-foreclose")
	amc := sim.NewMarketCreditState()
	aLender := actorByID(wa, "p27")
	aBorrower := actorByID(wa, "p01")
	aLender.Money = 100
	aBorrower.Money = 2
	initAutoCoins := wa.TotalCoins()
	if _, ok := wa.ApplyMarketCreditAction(amc, aBorrower.ID, sim.Action{Kind: "borrow_loan", TargetID: aLender.ID, Quantity: 20, HouseID: "h01"}); !ok {
		t.Fatal("expected borrow_loan handled")
	}
	aLender.Money += aBorrower.Money
	aBorrower.Money = 0
	wa.Tick = int64(amc.Spec.LoanTermTicks) + 1
	autoEvs := wa.StepMarketAndCredit(amc)
	if ev := lastOfKind(autoEvs, "house_foreclosed"); ev.Kind != "house_foreclosed" {
		t.Fatalf("expected automatic house_foreclosed in StepMarketAndCredit, got %+v", autoEvs)
	}
	if h := mcTestHouseByID(wa, "h01"); h == nil || h.OwnerID != aLender.ID {
		t.Fatalf("expected h01 foreclosed to %s, got %+v", aLender.ID, h)
	}
	if wa.TotalCoins() != initAutoCoins {
		t.Fatalf("M0 coin drift on automatic foreclosure: got=%d want=%d", wa.TotalCoins(), initAutoCoins)
	}
}

func TestMarketCreditSolventRepaymentAndMultiDayConservation(t *testing.T) {
	w := sim.LargeWorld("mc-solvent-14d", 10000)
	mc := sim.NewMarketCreditState()
	initialCoins := w.TotalCoins()
	initialPop := w.TotalPopulation()

	farmers := mcTestCohortByID(w, "cohort_south_farmers")
	merchants := mcTestCohortByID(w, "cohort_market_peddlers")
	if farmers == nil || merchants == nil {
		t.Fatal("missing expected cohorts")
	}
	for i := range w.Cohorts {
		w.Cohorts[i].MoneyPool = int64(w.Cohorts[i].Population) * 12
	}
	farmers.MoneyPool = int64(farmers.Population) // distressed on Day 0
	merchants.MoneyPool = int64(merchants.Population) * 60
	rich := mcTestCohortByID(w, "cohort_east_carpenters")
	if rich == nil {
		t.Fatal("missing cohort_east_carpenters")
	}
	rich.MoneyPool = int64(farmers.Population) * 30
	initialCoins = w.TotalCoins()

	var allEvs []sim.Event
	// Day 0: farmers borrow
	allEvs = append(allEvs, w.StepMarketAndCredit(mc)...)
	if ev := lastOfKind(allEvs, "cohort_loan_issued"); ev.Kind != "cohort_loan_issued" {
		t.Fatalf("expected cohort_loan_issued, got %+v", allEvs)
	}

	// On Day 2, give farmers bumper cash (transferred from rich cohort) so they repay principal + accrued interest!
	for day := 1; day <= 14; day++ {
		if day == 2 {
			transfer := int64(farmers.Population) * 20
			rich.MoneyPool -= transfer
			farmers.MoneyPool += transfer
		}
		for tickInDay := 0; tickInDay < sim.TicksPerDay; tickInDay++ {
			allEvs = append(allEvs, w.StepMarketAndCredit(mc)...)
			if got := w.TotalCoins(); got != initialCoins {
				t.Fatalf("day %d tick %d M0 coin drift: got=%d want=%d", day, w.Tick, got, initialCoins)
			}
			w.Tick++
		}
	}

	repaidEv := lastOfKind(allEvs, "cohort_loan_repaid")
	if repaidEv.Kind != "cohort_loan_repaid" {
		t.Fatalf("expected solvent cohort_loan_repaid event, got %+v", allEvs)
	}
	// Verify accrued interest caused total repaid to exceed principal on at least one debt
	sawInterest := false
	for _, d := range mc.CohortDebts {
		if d.DebtorID == farmers.ID && d.Status == "repaid" && d.AccruedInterest > 0 && d.Repaid > d.Principal {
			sawInterest = true
			break
		}
	}
	if !sawInterest {
		t.Fatalf("expected repaid cohort debt with accrued usury interest > 0, debts=%+v", mc.CohortDebts)
	}
	if got := w.TotalPopulation(); got != initialPop {
		t.Fatalf("population mismatch: got=%d want=%d", got, initialPop)
	}
	if got := w.TotalCoins(); got != initialCoins {
		t.Fatalf("final M0 coin mismatch: got=%d want=%d", got, initialCoins)
	}
}


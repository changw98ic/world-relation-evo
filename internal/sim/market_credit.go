package sim

import (
	"fmt"
	"math"
	"sort"
)

// MarketCreditSpec declares genre-decoupled parameters for dynamic supply/demand pricing,
// speculative hoarding, usury credit, collateral foreclosure, and land/workshop annexation.
type MarketCreditSpec struct {
	ExplicitZero              bool   `json:"explicit_zero,omitempty"`
	MinPricePermille          int    `json:"min_price_permille,omitempty"`
	MaxPricePermille          int    `json:"max_price_permille,omitempty"`
	HoardSurplusMultiplier    int    `json:"hoard_surplus_multiplier,omitempty"`
	HoardTriggerPermille      int    `json:"hoard_trigger_permille,omitempty"`
	HoardReleasePermille      int    `json:"hoard_release_permille,omitempty"`
	DailyLoanInterestPermille int    `json:"daily_loan_interest_permille,omitempty"`
	LoanTermTicks             int    `json:"loan_term_ticks,omitempty"`
	MaxTenantSharePermille    int    `json:"max_tenant_share_permille,omitempty"`
	DailyRentPermille         int    `json:"daily_rent_permille,omitempty"`
	HouseDailyRentPerLevel    int    `json:"house_daily_rent_per_level,omitempty"`
	SubsistenceFloorPerCap    int    `json:"subsistence_floor_per_cap,omitempty"`
	CreditorReservePerCap     int    `json:"creditor_reserve_per_cap,omitempty"`
	TraderTag                 string `json:"trader_tag,omitempty"`
	explicitFields            map[string]bool
}

// DefaultMarketCreditSpec returns balanced defaults for dynamic pricing, hoarding, usury, and annexation.
func DefaultMarketCreditSpec() MarketCreditSpec {
	return MarketCreditSpec{
		MinPricePermille:          550,
		MaxPricePermille:          2600,
		HoardSurplusMultiplier:    2,
		HoardTriggerPermille:      1150,
		HoardReleasePermille:      1650,
		DailyLoanInterestPermille: 18,
		LoanTermTicks:             5 * TicksPerDay,
		MaxTenantSharePermille:    650,
		DailyRentPermille:         140,
		HouseDailyRentPerLevel:    2,
		SubsistenceFloorPerCap:    6,
		CreditorReservePerCap:     12,
		TraderTag:                 "trader",
	}
}

// EffectiveMarketCreditSpec fills omitted fields from DefaultMarketCreditSpec.
func EffectiveMarketCreditSpec(s MarketCreditSpec) MarketCreditSpec {
	def := DefaultMarketCreditSpec()
	s.MinPricePermille = resolveSpecNumber(s.MinPricePermille, def.MinPricePermille, s.ExplicitZero || s.explicitFields["min_price_permille"])
	s.MaxPricePermille = resolveSpecNumber(s.MaxPricePermille, def.MaxPricePermille, s.ExplicitZero || s.explicitFields["max_price_permille"])
	s.HoardSurplusMultiplier = resolveSpecNumber(s.HoardSurplusMultiplier, def.HoardSurplusMultiplier, s.ExplicitZero || s.explicitFields["hoard_surplus_multiplier"])
	s.HoardTriggerPermille = resolveSpecNumber(s.HoardTriggerPermille, def.HoardTriggerPermille, s.ExplicitZero || s.explicitFields["hoard_trigger_permille"])
	s.HoardReleasePermille = resolveSpecNumber(s.HoardReleasePermille, def.HoardReleasePermille, s.ExplicitZero || s.explicitFields["hoard_release_permille"])
	s.DailyLoanInterestPermille = resolveSpecNumber(s.DailyLoanInterestPermille, def.DailyLoanInterestPermille, s.ExplicitZero || s.explicitFields["daily_loan_interest_permille"])
	s.LoanTermTicks = resolveSpecNumber(s.LoanTermTicks, def.LoanTermTicks, s.ExplicitZero || s.explicitFields["loan_term_ticks"])
	s.MaxTenantSharePermille = resolveSpecNumber(s.MaxTenantSharePermille, def.MaxTenantSharePermille, s.ExplicitZero || s.explicitFields["max_tenant_share_permille"])
	s.DailyRentPermille = resolveSpecNumber(s.DailyRentPermille, def.DailyRentPermille, s.ExplicitZero || s.explicitFields["daily_rent_permille"])
	s.HouseDailyRentPerLevel = resolveSpecNumber(s.HouseDailyRentPerLevel, def.HouseDailyRentPerLevel, s.ExplicitZero || s.explicitFields["house_daily_rent_per_level"])
	s.SubsistenceFloorPerCap = resolveSpecNumber(s.SubsistenceFloorPerCap, def.SubsistenceFloorPerCap, s.ExplicitZero || s.explicitFields["subsistence_floor_per_cap"])
	s.CreditorReservePerCap = resolveSpecNumber(s.CreditorReservePerCap, def.CreditorReservePerCap, s.ExplicitZero || s.explicitFields["creditor_reserve_per_cap"])
	if !s.explicitFields["max_price_permille"] && s.MaxPricePermille < s.MinPricePermille {
		s.MaxPricePermille = s.MinPricePermille
	}
	if !s.ExplicitZero && !s.explicitFields["hoard_release_permille"] && s.HoardReleasePermille < s.HoardTriggerPermille {
		s.HoardReleasePermille = s.HoardTriggerPermille + 200
	}
	if s.TraderTag == "" {
		s.TraderTag = def.TraderTag
	}
	return s
}

// LoanRecord tracks an individual Actor's usury loan and optional property collateral.
type LoanRecord struct {
	ID                  int64  `json:"id"`
	LenderID            string `json:"lender_id"`
	LenderCohortID      string `json:"lender_cohort_id,omitempty"`
	BorrowerID          string `json:"borrower_id"`
	BorrowerCohortID    string `json:"borrower_cohort_id,omitempty"`
	BorrowerCohortFunds int64  `json:"borrower_cohort_funds,omitempty"`
	Principal           int64  `json:"principal"`
	AccruedInterest     int64  `json:"accrued_interest"`
	Repaid              int64  `json:"repaid,omitempty"`
	RatePermille        int    `json:"rate_permille"`
	IssuedTick          int64  `json:"issued_tick"`
	IssuedEventID       int64  `json:"issued_event_id,omitempty"`
	RootEventID         int64  `json:"root_event_id,omitempty"`
	DueTick             int64  `json:"due_tick"`
	CollateralHouse     string `json:"collateral_house,omitempty"`
	Status              string `json:"status"` // "active", "repaid", "foreclosed", "defaulted"
}

// CohortDebtRecord tracks macro usury debt between a debtor PopulationCohort and a creditor PopulationCohort.
type CohortDebtRecord struct {
	ID              int64  `json:"id"`
	CreditorID      string `json:"creditor_id"`
	DebtorID        string `json:"debtor_id"`
	Principal       int64  `json:"principal"`
	AccruedInterest int64  `json:"accrued_interest"`
	Repaid          int64  `json:"repaid,omitempty"`
	RatePermille    int    `json:"rate_permille"`
	IssuedTick      int64  `json:"issued_tick"`
	DueTick         int64  `json:"due_tick"`
	Status          string `json:"status"` // "active", "repaid", "annexed", "defaulted"
}

// ForeclosedTenancy tracks a former homeowner remaining in a foreclosed House as a rent-paying tenant.
type ForeclosedTenancy struct {
	HouseID           string `json:"house_id"`
	OwnerID           string `json:"owner_id"`
	OwnerCohortID     string `json:"owner_cohort_id,omitempty"`
	TenantID          string `json:"tenant_id"`
	TenantCohortID    string `json:"tenant_cohort_id,omitempty"`
	TenantCohortFunds int64  `json:"tenant_cohort_funds,omitempty"`
	DailyRent         int64  `json:"daily_rent"`
	SinceTick         int64  `json:"since_tick"`
	LastRentTick      int64  `json:"last_rent_tick"`
}

// MarketCreditState holds dynamic district prices, hoarded stocks, usury loan ledgers, and property/land annexation state.
type MarketCreditState struct {
	Spec           MarketCreditSpec            `json:"spec,omitempty"`
	DistrictPrices map[string]map[string]int   `json:"district_prices"`
	HoardedStock   map[string]map[string]int64 `json:"hoarded_stock"`
	LoanRequests   []LoanRequest               `json:"loan_requests,omitempty"`
	Loans          []LoanRecord                `json:"loans,omitempty"`
	CohortDebts    []CohortDebtRecord          `json:"cohort_debts,omitempty"`
	// CohortEquityPermille records each creditor's share, keyed by debtor then creditor.
	CohortEquityPermille map[string]map[string]int    `json:"cohort_equity_permille"`
	ForeclosedTenancies  map[string]ForeclosedTenancy `json:"foreclosed_tenancies"`
	NextLoanID           int64                        `json:"next_loan_id,omitempty"`
	LastPriceUpdateTick  int64                        `json:"last_price_update_tick,omitempty"`
	LastCreditDay        int                          `json:"last_credit_day,omitempty"`
}

// NewMarketCreditState constructs an initialized MarketCreditState with default spec.
func NewMarketCreditState() *MarketCreditState {
	return &MarketCreditState{
		Spec:                 DefaultMarketCreditSpec(),
		DistrictPrices:       map[string]map[string]int{},
		HoardedStock:         map[string]map[string]int64{},
		CohortEquityPermille: map[string]map[string]int{},
		ForeclosedTenancies:  map[string]ForeclosedTenancy{},
		NextLoanID:           1,
		LastCreditDay:        -1,
	}
}

func (mc *MarketCreditState) ensureInit() MarketCreditSpec {
	if mc.DistrictPrices == nil {
		mc.DistrictPrices = map[string]map[string]int{}
	}
	if mc.HoardedStock == nil {
		mc.HoardedStock = map[string]map[string]int64{}
	}
	if mc.CohortEquityPermille == nil {
		mc.CohortEquityPermille = map[string]map[string]int{}
	}
	if mc.ForeclosedTenancies == nil {
		mc.ForeclosedTenancies = map[string]ForeclosedTenancy{}
	}
	if mc.NextLoanID <= 0 {
		mc.NextLoanID = 1
	}
	return EffectiveMarketCreditSpec(mc.Spec)
}

func (mc *MarketCreditState) allocLoanID() int64 {
	if mc.NextLoanID <= 0 {
		mc.NextLoanID = 1
	}
	id := mc.NextLoanID
	mc.NextLoanID++
	return id
}

// TotalAnnexedSharePermille derives the total from the individual creditors' registered shares.
func (mc *MarketCreditState) TotalAnnexedSharePermille(debtorID string) int {
	var total int
	for _, share := range mc.CohortEquityPermille[debtorID] {
		total += share
	}
	return total
}

// TotalHoardedStock returns total hoarded units of itemID across all holders (or all items if itemID == "").
func (w *World) TotalHoardedStock(mc *MarketCreditState, itemID string) int64 {
	if mc == nil || len(mc.HoardedStock) == 0 {
		return 0
	}
	var total int64
	for _, bag := range mc.HoardedStock {
		if itemID == "" {
			for _, qty := range bag {
				total += qty
			}
		} else {
			total += bag[itemID]
		}
	}
	return total
}

// EffectiveMarketPrice returns the dynamically scaled market price for itemID in districtID, clamped >= 1.
func (w *World) EffectiveMarketPrice(mc *MarketCreditState, districtID, itemID string, basePrice int) int {
	if basePrice <= 0 {
		if spec, ok := w.Catalog.Items[itemID]; ok && spec.BatchPrice > 0 {
			basePrice = spec.BatchPrice
		} else {
			basePrice = 1
		}
	}
	if mc == nil {
		return maxInt(1, basePrice)
	}
	spec := EffectiveMarketCreditSpec(mc.Spec)
	permille := 0
	if mc.DistrictPrices != nil {
		if dMap, ok := mc.DistrictPrices[districtID]; ok {
			permille = dMap[itemID]
		}
	}
	if permille <= 0 {
		permille = w.computeDistrictItemPricePermille(spec, districtID, itemID)
	}
	if permille < spec.MinPricePermille {
		permille = spec.MinPricePermille
	}
	if permille > spec.MaxPricePermille {
		permille = spec.MaxPricePermille
	}
	scaled := (basePrice*permille + 500) / 1000
	if scaled < 1 {
		scaled = 1
	}
	return scaled
}

// UpdateDistrictPrices recalculates DistrictPrices across all districts and tradeable catalog items
// from supply vs DesiredStock/population demand, active disasters, weather, and freight/canal disruptions.
func (w *World) UpdateDistrictPrices(mc *MarketCreditState) {
	if mc == nil {
		return
	}
	spec := mc.ensureInit()

	itemIDs := make([]string, 0, len(w.Catalog.Items))
	for id, it := range w.Catalog.Items {
		if it.Tradeable {
			itemIDs = append(itemIDs, id)
		}
	}
	sort.Strings(itemIDs)

	// Pre-aggregate supply and demand per district and world-wide in one linear pass.
	localSupply := make(map[string]map[string]int64, len(w.Map.Districts))
	localDemand := make(map[string]map[string]int64, len(w.Map.Districts))
	worldSupply := make(map[string]int64, len(itemIDs))
	worldDemand := make(map[string]int64, len(itemIDs))

	ensureBucket := func(m map[string]map[string]int64, dID string) map[string]int64 {
		b := m[dID]
		if b == nil {
			b = make(map[string]int64, len(itemIDs))
			m[dID] = b
		}
		return b
	}

	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead || a.Location == "" {
			continue
		}
		supB := ensureBucket(localSupply, a.Location)
		demB := ensureBucket(localDemand, a.Location)
		aTags := a.IdentityTags()
		for _, itemID := range itemIDs {
			q := int64(a.Items[itemID])
			if q > 0 {
				supB[itemID] += q
				worldSupply[itemID] += q
			}
			want := int64(w.Catalog.DesiredStock(aTags, itemID))
			if want <= 0 {
				it := w.Catalog.Items[itemID]
				if it.Edible || it.Potable || w.Catalog.TagsConsume(aTags, itemID) {
					want = int64(maxInt(1, it.OfferThreshold))
				}
			}
			if want > 0 {
				demB[itemID] += want
				worldDemand[itemID] += want
			}
		}
	}

	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 || c.District == "" {
			continue
		}
		supB := ensureBucket(localSupply, c.District)
		demB := ensureBucket(localDemand, c.District)
		cTags := cohortIdentityTags(w.Catalog, c.Profession)
		for _, itemID := range itemIDs {
			q := c.ItemPool[itemID]
			if q > 0 {
				supB[itemID] += q
				worldSupply[itemID] += q
			}
			it := w.Catalog.Items[itemID]
			perCap := w.Catalog.DesiredStock(cTags, itemID)
			if perCap <= 0 && (it.Edible || it.Potable) {
				perCap = w.Catalog.DesiredStock([]string{"resident"}, itemID)
				if perCap <= 0 {
					perCap = maxInt(2, it.OfferThreshold)
				}
			} else if perCap <= 0 && w.Catalog.TagsConsume(cTags, itemID) {
				perCap = maxInt(1, it.ShortageThreshold)
			}
			if perCap > 0 {
				want := int64(c.Population) * int64(perCap)
				demB[itemID] += want
				worldDemand[itemID] += want
			}
		}
	}

	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	for _, d := range w.Map.Districts {
		dMap := mc.DistrictPrices[d.ID]
		if dMap == nil {
			dMap = make(map[string]int, len(itemIDs))
			mc.DistrictPrices[d.ID] = dMap
		}
		routeShock := w.districtRouteDisruptionPermille(d.ID, cal)
		for _, itemID := range itemIDs {
			permille := w.scoreItemPricePermille(
				spec, d.ID, itemID,
				localSupply[d.ID][itemID], localDemand[d.ID][itemID],
				worldSupply[itemID], worldDemand[itemID],
				routeShock,
			)
			dMap[itemID] = permille
		}
	}
	mc.LastPriceUpdateTick = w.Tick
}

func (w *World) computeDistrictItemPricePermille(spec MarketCreditSpec, districtID, itemID string) int {
	var lSup, lDem, wSup, wDem int64
	it := w.Catalog.Items[itemID]
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead {
			continue
		}
		q := int64(a.Items[itemID])
		aTags := a.IdentityTags()
		want := int64(w.Catalog.DesiredStock(aTags, itemID))
		if want <= 0 && (it.Edible || it.Potable || w.Catalog.TagsConsume(aTags, itemID)) {
			want = int64(maxInt(1, it.OfferThreshold))
		}
		wSup += q
		wDem += want
		if a.Location == districtID {
			lSup += q
			lDem += want
		}
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		q := c.ItemPool[itemID]
		cTags := cohortIdentityTags(w.Catalog, c.Profession)
		perCap := w.Catalog.DesiredStock(cTags, itemID)
		if perCap <= 0 && (it.Edible || it.Potable) {
			perCap = w.Catalog.DesiredStock([]string{"resident"}, itemID)
			if perCap <= 0 {
				perCap = maxInt(2, it.OfferThreshold)
			}
		} else if perCap <= 0 && w.Catalog.TagsConsume(cTags, itemID) {
			perCap = maxInt(1, it.ShortageThreshold)
		}
		want := int64(c.Population) * int64(perCap)
		wSup += q
		wDem += want
		if c.District == districtID {
			lSup += q
			lDem += want
		}
	}
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	routeShock := w.districtRouteDisruptionPermille(districtID, cal)
	return w.scoreItemPricePermille(spec, districtID, itemID, lSup, lDem, wSup, wDem, routeShock)
}

func (w *World) districtRouteDisruptionPermille(districtID string, cal CalendarState) int {
	edges := w.Map.NeighborsOf(districtID)
	if len(edges) == 0 {
		return 0
	}
	maxShock := 0
	disruptedCount := 0
	waterwayShock := 0
	for _, r := range edges {
		capBatches, _, _, hazardPermille, medium, _ := w.edgeEffectiveLogistics(r, cal)
		nomCap := r.FreightCapacity
		if nomCap <= 0 {
			continue
		}
		edgeShock := 0
		if capBatches < nomCap {
			pctDrop := int(((nomCap - capBatches) * 100) / nomCap)
			if pctDrop > 15 {
				edgeShock += pctDrop * 5
			}
		}
		if hazardPermille > 0 {
			edgeShock += hazardPermille * 3
		}
		// Spillover from active disasters in the neighboring terminal district.
		if w.DistrictStates != nil {
			if nds, ok := w.DistrictStates[r.To]; ok && len(nds.ActiveDisasters) > 0 {
				edgeShock += 120
			}
		}
		if medium == "waterway" && edgeShock > waterwayShock {
			waterwayShock = edgeShock
		}
		if edgeShock > 60 {
			disruptedCount++
		}
		if edgeShock > maxShock {
			maxShock = edgeShock
		}
	}
	shock := maxShock
	if waterwayShock > 0 {
		shock += waterwayShock / 3
	}
	if disruptedCount == len(edges) && len(edges) > 1 {
		shock += 150
	}
	if shock > 650 {
		shock = 650
	}
	return shock
}

func (w *World) scoreItemPricePermille(
	spec MarketCreditSpec,
	districtID, itemID string,
	localSupply, localDemand, worldSupply, worldDemand int64,
	routeShock int,
) int {
	var worldRatio int64 = 1000
	if worldDemand > 0 {
		worldRatio = (worldSupply * 1000) / worldDemand
	}
	supplyRatio := worldRatio
	if localDemand > 0 {
		localRatio := (localSupply * 1000) / localDemand
		supplyRatio = (localRatio*7 + worldRatio*3) / 10
	}

	permille := 1000
	if supplyRatio < 1000 {
		deficit := int(1000 - supplyRatio)
		permille += (deficit * 1100) / 1000
	} else if supplyRatio > 1000 {
		surplus := int(supplyRatio - 1000)
		maxDrop := 1000 - spec.MinPricePermille
		drop := (surplus * 225) / 1000
		if drop > maxDrop {
			drop = maxDrop
		}
		permille -= drop
	}

	it := w.Catalog.Items[itemID]
	isEssential := it.Edible || it.Potable || it.MedicinalHeal > 0 || it.DiseaseCure > 0 || it.WarmthBonus > 0

	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[districtID]; ok {
			for _, ad := range ds.ActiveDisasters {
				shock := 200
				if isEssential {
					shock = 420
				}
				for _, disSpec := range w.Catalog.Disasters {
					if disSpec.ID == ad.ID && disSpec.ReliefItem == itemID {
						shock = 520
						break
					}
				}
				permille += shock
			}
			if ws, hasW := w.Catalog.Weathers[ds.Weather]; hasW {
				if ws.ColdDamage > 0 && (it.WarmthBonus > 0 || it.Edible) {
					permille += 120
				} else {
					for _, yPct := range ws.FeatureYieldPct {
						if yPct > 0 && yPct < 80 && isEssential {
							permille += 100
							break
						}
					}
				}
			}
		}
	}

	if routeShock > 0 {
		if isEssential {
			permille += routeShock
		} else {
			permille += routeShock / 2
		}
	}

	if permille < spec.MinPricePermille {
		permille = spec.MinPricePermille
	}
	if permille > spec.MaxPricePermille {
		permille = spec.MaxPricePermille
	}
	return permille
}

func (w *World) isMerchantCohort(c *PopulationCohort, spec MarketCreditSpec) bool {
	if c == nil || c.Population <= 0 {
		return false
	}
	if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok {
		if prof.TradeMargin {
			return true
		}
		if spec.TraderTag != "" && ActorHasTag(prof.Grants, spec.TraderTag) {
			return true
		}
	}
	return false
}

// Older saves separated a person's hoard from their other property. Restore
// that property into the single inventory used by personal decisions.
func (w *World) mergePersonalHoardedStock() error {
	if w.MarketCredit == nil {
		return nil
	}
	for id, stock := range w.MarketCredit.HoardedStock {
		a := w.actor(id)
		if a == nil {
			continue
		}
		for item, quantity := range stock {
			held := a.Items[item]
			if quantity < 0 || held < 0 || quantity > int64(math.MaxInt-held) {
				return fmt.Errorf("restore hoard for actor %s: inventory of %s is out of range", id, item)
			}
			w.adjustActorItem(a, item, int(quantity), false)
		}
		delete(w.MarketCredit.HoardedStock, id)
	}
	return nil
}

func (w *World) stepMerchantHoarding(mc *MarketCreditState, spec MarketCreditSpec) []Event {
	var events []Event

	itemIDs := make([]string, 0, len(w.Catalog.Items))
	for id, it := range w.Catalog.Items {
		if it.Tradeable {
			itemIDs = append(itemIDs, id)
		}
	}
	sort.Strings(itemIDs)

	addHoard := func(holderID, itemID string, qty int64) {
		if qty <= 0 {
			return
		}
		bag := mc.HoardedStock[holderID]
		if bag == nil {
			bag = map[string]int64{}
			mc.HoardedStock[holderID] = bag
		}
		bag[itemID] += qty
	}

	// Merchant cohorts manage their pooled stock. Individuals choose when to
	// sell from the inventory they own.
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if !w.isMerchantCohort(c, spec) {
			continue
		}
		for _, itemID := range itemIDs {
			it := w.Catalog.Items[itemID]
			pricePermille := mc.DistrictPrices[c.District][itemID]
			if pricePermille <= 0 {
				pricePermille = 1000
			}
			curHoarded := mc.HoardedStock[c.ID][itemID]
			// Release hoarded stock on price peak, distress liquidation, or supply glut recovery.
			if curHoarded > 0 && (pricePermille >= spec.HoardReleasePermille || c.AvgHunger >= 2 || c.MoneyPool <= int64(c.Population) || pricePermille <= 900) {
				reason := "price_peak"
				if pricePermille < spec.HoardReleasePermille {
					if c.AvgHunger >= 2 || c.MoneyPool <= int64(c.Population) {
						reason = "distress_liquidation"
					} else {
						reason = "glut_clear"
					}
				}
				w.adjustCohortItem(c, itemID, curHoarded, false)
				delete(mc.HoardedStock[c.ID], itemID)
				if len(mc.HoardedStock[c.ID]) == 0 {
					delete(mc.HoardedStock, c.ID)
				}
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: c.ID,
					Kind:    "hoard_released",
					Action:  Action{Kind: "offer", Location: c.District, Item: itemID, Quantity: int(curHoarded)},
					Payload: map[string]any{
						"text":           fmt.Sprintf("%s released %d hoarded %s in %s (%s at %d‰ price)", c.Name, curHoarded, itemID, c.District, reason, pricePermille),
						"holder_id":      c.ID,
						"district":       c.District,
						"item":           itemID,
						"quantity":       curHoarded,
						"price_permille": pricePermille,
						"reason":         reason,
					},
				}
				w.NextID++
				events = append(events, ev)
				continue
			}

			// Withhold stock during rising scarcity.
			if pricePermille >= spec.HoardTriggerPermille && pricePermille < spec.HoardReleasePermille {
				reserve := int64(c.Population)
				if it.Edible {
					reserve = int64(c.Population) * 2
				}
				excess := c.ItemPool[itemID] - reserve
				if excess <= 0 {
					continue
				}
				capQty := int64(spec.HoardSurplusMultiplier) * maxInt64(1, int64(c.Population)*2)
				room := capQty - curHoarded
				if room <= 0 {
					continue
				}
				hoardQty := excess / 2
				if hoardQty < 1 {
					hoardQty = excess
				}
				if hoardQty > room {
					hoardQty = room
				}
				if hoardQty <= 0 {
					continue
				}
				w.adjustCohortItem(c, itemID, -(hoardQty), false)
				addHoard(c.ID, itemID, hoardQty)
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: c.ID,
					Kind:    "merchant_hoarded",
					Action:  Action{Kind: "wait", Location: c.District, Item: itemID, Quantity: int(hoardQty)},
					Payload: map[string]any{
						"text":           fmt.Sprintf("%s hoarded %d %s in %s during scarcity (%d‰ price)", c.Name, hoardQty, itemID, c.District, pricePermille),
						"holder_id":      c.ID,
						"district":       c.District,
						"item":           itemID,
						"quantity":       hoardQty,
						"total_hoarded":  curHoarded + hoardQty,
						"price_permille": pricePermille,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}
	}

	return events
}

// ApplyMarketCreditAction handles focal Actor usury credit actions:
// Requests, bilateral acceptance/rejection, borrower cancellation, repayment and foreclosure.
func (w *World) ApplyMarketCreditAction(state *MarketCreditState, actorID string, x Action) ([]Event, bool) {
	return w.applyMarketCreditSubsystem(state, actorID, x)
}

func (w *World) applyMarketCreditActionTx(tx *WorldTx, mc *MarketCreditState, a *Actor, x Action) []Event {
	emit := func(kind, text string, data map[string]any, observers ...*Actor) []Event {
		if data == nil {
			data = map[string]any{}
		}
		data["text"] = text
		return []Event{tx.record(a, x, kind, data, observers...)}
	}
	fail := func(reason string) []Event {
		return emit("action_failed", reason, nil)
	}
	if mc == nil {
		return fail("market credit ledger unavailable")
	}
	spec := mc.ensureInit()

	switch x.Kind {
	case "borrow_loan", "accept_loan", "reject_loan", "cancel_loan":
		return w.applyPersonalCredit(tx, mc, a, x, spec)

	case "repay_loan":
		idx := -1
		for i := range mc.Loans {
			l := &mc.Loans[i]
			if l.Status != "active" || l.BorrowerID != a.ID {
				continue
			}
			if x.OfferID > 0 && l.ID != x.OfferID {
				continue
			}
			if x.TargetID != "" && l.LenderID != x.TargetID {
				continue
			}
			idx = i
			break
		}
		if idx < 0 {
			return fail("no matching active loan to repay")
		}
		l := &mc.Loans[idx]
		owed := l.Principal + l.AccruedInterest - l.Repaid
		if owed <= 0 {
			l.Status = "repaid"
			return fail("loan is already settled")
		}
		if a.Money <= 0 {
			return fail("insufficient coins to repay loan")
		}
		pay := int64(x.Quantity)
		if pay <= 0 {
			pay = int64(x.Price)
		}
		if pay <= 0 || pay > owed {
			pay = owed
		}
		if pay > int64(a.Money) {
			pay = int64(a.Money)
		}
		lender := w.actor(l.LenderID)
		if lender != nil && !lender.Dead {
			tx.TransferCurrency(a, lender, "coins", pay, x.Kind)
			tx.MutateTrust(lender, a.ID, 1)
		} else if lc := w.findCohort(l.LenderCohortID); lc != nil && lc.Population > 0 {
			tx.moveCurrency(a, "coins", -pay, lc.ID, x.Kind)
			w.addCohortCurrency(lc, "coins", pay)
		} else {
			tx.moveCurrency(a, "coins", -pay, "treasury", x.Kind)
			w.creditTreasury(pay, "fee")
		}
		l.Repaid += pay
		rem := l.Principal + l.AccruedInterest - l.Repaid
		if rem <= 0 {
			rem = 0
			l.Status = "repaid"
		}
		var obs []*Actor
		if lender != nil && !lender.Dead {
			obs = append(obs, lender)
		}
		repayData := map[string]any{
			"loan_id":        l.ID,
			"lender_id":      l.LenderID,
			"borrower_id":    l.BorrowerID,
			"coins":          pay,
			"remaining_owed": rem,
			"status":         l.Status,
		}
		if l.IssuedEventID > 0 {
			repayData["cause_event_id"] = l.IssuedEventID
		}
		if l.RootEventID > 0 {
			repayData["root_event_id"] = l.RootEventID
		}
		return emit("loan_repaid", fmt.Sprintf("%s repaid %d coins on loan #%d (remaining %d, status %s)", a.Name, pay, l.ID, rem, l.Status), repayData, obs...)

	case "foreclose_collateral":
		idx := -1
		for i := range mc.Loans {
			l := &mc.Loans[i]
			if l.Status != "active" || l.LenderID != a.ID {
				continue
			}
			if x.OfferID > 0 && l.ID != x.OfferID {
				continue
			}
			if x.TargetID != "" && l.BorrowerID != x.TargetID {
				continue
			}
			if x.HouseID != "" && l.CollateralHouse != "" && l.CollateralHouse != x.HouseID {
				continue
			}
			idx = i
			break
		}
		if idx < 0 {
			return fail("no matching active loan to foreclose")
		}
		l := &mc.Loans[idx]
		if w.Tick < l.DueTick {
			return fail("loan is not overdue yet")
		}
		rem := l.Principal + l.AccruedInterest - l.Repaid
		if rem <= 0 {
			l.Status = "repaid"
			return fail("loan has no remaining balance")
		}
		borrower := w.actor(l.BorrowerID)
		borrowerCohort := w.findCohort(l.BorrowerCohortID)
		if l.CollateralHouse == "" {
			return fail("loan has no agreed collateral house")
		}
		h := w.house(l.CollateralHouse)
		borrowerAlive := (borrower != nil && !borrower.Dead) || (borrower == nil && borrowerCohort != nil && borrowerCohort.Population > 0)
		if h == nil || !borrowerAlive || h.OwnerID != l.BorrowerID {
			return fail("no foreclosable collateral house owned by borrower")
		}

		oldOwner := h.OwnerID
		h.OwnerID = a.ID
		h.Residents = appendUnique(h.Residents, l.BorrowerID)
		dailyRent := int64(maxInt(1, h.Level) * spec.HouseDailyRentPerLevel)
		tenancy := ForeclosedTenancy{
			HouseID:      h.ID,
			OwnerID:      a.ID,
			TenantID:     l.BorrowerID,
			DailyRent:    dailyRent,
			SinceTick:    w.Tick,
			LastRentTick: w.Tick,
		}
		if borrower == nil && borrowerCohort != nil {
			tenancy.TenantCohortID = borrowerCohort.ID
			tenancy.TenantCohortFunds = l.BorrowerCohortFunds
		}
		mc.ForeclosedTenancies[h.ID] = tenancy
		l.Status = "foreclosed"
		borrowerName := l.BorrowerID
		var obs []*Actor
		if borrower != nil {
			borrowerName = borrower.Name
			tx.MutateTrust(borrower, a.ID, -3)
			tx.MutateTrust(a, borrower.ID, -1)
			obs = append(obs, borrower)
		}
		forecloseData := map[string]any{
			"loan_id":      l.ID,
			"house_id":     h.ID,
			"old_owner_id": oldOwner,
			"new_owner_id": a.ID,
			"tenant_id":    l.BorrowerID,
			"daily_rent":   dailyRent,
		}
		if l.IssuedEventID > 0 {
			forecloseData["cause_event_id"] = l.IssuedEventID
		}
		if l.RootEventID > 0 {
			forecloseData["root_event_id"] = l.RootEventID
		}
		return emit("house_foreclosed", fmt.Sprintf("%s foreclosed %s from %s over overdue loan #%d (tenant rent %d/day)", a.Name, h.Name, borrowerName, l.ID, dailyRent), forecloseData, obs...)
	}

	return nil
}

// StepMarketAndCredit advances dynamic district pricing, merchant hoarding/release,
// daily interest and overdue facts, aggregate cohort credit, and rent collection.
func (w *World) StepMarketAndCredit(mc *MarketCreditState) []Event {
	if mc == nil {
		return nil
	}
	spec := mc.ensureInit()
	var events []Event

	// Hourly (every 4 ticks) or initial price & hoarding pass.
	if w.Tick == 0 || w.Tick%4 == 0 || w.Tick-mc.LastPriceUpdateTick >= 4 || len(mc.DistrictPrices) == 0 {
		w.UpdateDistrictPrices(mc)
		events = append(events, w.stepMerchantHoarding(mc, spec)...)
	}

	// Daily credit, interest, rent, default foreclosure, and new distress lending pass.
	day := int(w.Tick / TicksPerDay)
	if day != mc.LastCreditDay {
		mc.LastCreditDay = day
		events = append(events, w.stepDailyCreditAndAnnexation(mc, spec)...)
	}
	return events
}

func (w *World) findCohort(id string) *PopulationCohort {
	if id == "" {
		return nil
	}
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == id {
			return &w.Cohorts[i]
		}
	}
	return nil
}

func (w *World) actorHasActiveCreditContract(actorID string) bool {
	if actorID == "" {
		return false
	}
	if a := w.actor(actorID); a != nil && a.Plan != nil && a.Plan.Status == "active" {
		return true
	}
	if w.Continuity != nil {
		for _, ag := range w.Continuity.Agreements {
			if (ag.Status == "proposed" || ag.Status == "active") && (ag.SupplierID == actorID || ag.BuyerID == actorID) {
				return true
			}
		}
	}
	mc := w.MarketCredit
	if mc == nil {
		return false
	}
	for _, r := range mc.LoanRequests {
		if r.Status == "pending" && (r.BorrowerID == actorID || r.LenderID == actorID) {
			return true
		}
	}
	for i := range mc.Loans {
		l := &mc.Loans[i]
		if l.Status == "active" && (l.BorrowerID == actorID || l.LenderID == actorID) {
			return true
		}
	}
	for _, ten := range mc.ForeclosedTenancies {
		if ten.OwnerID == actorID || ten.TenantID == actorID {
			return true
		}
	}
	return false
}

func (w *World) stepDailyCreditAndAnnexation(mc *MarketCreditState, spec MarketCreditSpec) []Event {
	var events []Event

	// 1. Accrue daily interest on active focal and cohort loans.
	for i := range mc.Loans {
		l := &mc.Loans[i]
		if l.Status != "active" {
			continue
		}
		owed := l.Principal + l.AccruedInterest - l.Repaid
		if owed <= 0 {
			l.Status = "repaid"
			continue
		}
		interest := (owed*int64(l.RatePermille) + 500) / 1000
		if interest < 1 {
			interest = 1
		}
		l.AccruedInterest += interest
	}
	for i := range mc.CohortDebts {
		d := &mc.CohortDebts[i]
		if d.Status != "active" {
			continue
		}
		owed := d.Principal + d.AccruedInterest - d.Repaid
		if owed <= 0 {
			d.Status = "repaid"
			continue
		}
		interest := (owed*int64(d.RatePermille) + 500) / 1000
		if interest < 1 {
			interest = 1
		}
		d.AccruedInterest += interest
	}

	// 2. Collect daily tenant rent on foreclosed houses (focal or dehydrated actors).
	houseIDs := make([]string, 0, len(mc.ForeclosedTenancies))
	for hID := range mc.ForeclosedTenancies {
		houseIDs = append(houseIDs, hID)
	}
	sort.Strings(houseIDs)
	for _, hID := range houseIDs {
		ten := mc.ForeclosedTenancies[hID]
		h := w.house(hID)
		if h == nil || h.OwnerID == "" || h.OwnerID == ten.TenantID {
			delete(mc.ForeclosedTenancies, hID)
			continue
		}
		owner := w.actor(h.OwnerID)
		ownerCohort := w.findCohort(ten.OwnerCohortID)
		tenant := w.actor(ten.TenantID)
		tenantCohort := w.findCohort(ten.TenantCohortID)
		ownerAlive := (owner != nil && !owner.Dead) || (owner == nil && ownerCohort != nil && ownerCohort.Population > 0)
		tenantAlive := (tenant != nil && !tenant.Dead) || (tenant == nil && tenantCohort != nil && tenantCohort.Population > 0)
		if !ownerAlive || !tenantAlive {
			delete(mc.ForeclosedTenancies, hID)
			continue
		}
		if owner != nil {
			ten.OwnerID = owner.ID
		}
		rent := ten.DailyRent
		if rent < 1 {
			rent = int64(maxInt(1, h.Level) * spec.HouseDailyRentPerLevel)
		}
		availRent := int64(0)
		if tenant != nil {
			availRent = int64(tenant.Money)
		} else if tenantCohort != nil {
			availRent = minInt64(ten.TenantCohortFunds, tenantCohort.MoneyPool)
		}
		if availRent > 0 {
			paid := minInt64(rent, availRent)
			if paid > 0 {
				switch {
				case tenant != nil && owner != nil:
					transferActorCoinsClean(tenant, owner, int(paid))
				case tenant != nil:
					w.adjustActorCurrency(tenant, "coins", -paid, false)
					w.addCohortCurrency(ownerCohort, "coins", paid)
				case owner != nil:
					w.addCohortCurrency(tenantCohort, "coins", -(paid))
					ten.TenantCohortFunds -= paid
					w.adjustActorCurrency(owner, "coins", paid, false)
				default:
					w.addCohortCurrency(tenantCohort, "coins", -(paid))
					ten.TenantCohortFunds -= paid
					w.addCohortCurrency(ownerCohort, "coins", paid)
				}
				ten.LastRentTick = w.Tick
				if tenant != nil {
					var obs []*Actor
					ownerName := ten.OwnerID
					if owner != nil {
						ownerName = owner.Name
						obs = append(obs, owner)
					}
					ev := w.record(tenant, Action{Kind: "wait", TargetID: ten.OwnerID, HouseID: h.ID, Price: int(paid)}, "house_rent_paid", map[string]any{
						"text":      fmt.Sprintf("%s paid %d coins daily tenant rent for %s to %s", tenant.Name, paid, h.Name, ownerName),
						"house_id":  h.ID,
						"owner_id":  ten.OwnerID,
						"tenant_id": tenant.ID,
						"coins":     paid,
					}, obs...)
					events = append(events, ev)
				} else {
					ev := Event{
						ID:       w.NextID,
						Tick:     w.Tick,
						ActorID:  ten.TenantID,
						TargetID: ten.OwnerID,
						Kind:     "house_rent_paid",
						Action:   Action{Kind: "wait", TargetID: ten.OwnerID, HouseID: h.ID, Price: int(paid)},
						Payload: map[string]any{
							"text":      fmt.Sprintf("%s (in %s) paid %d coins daily tenant rent for %s", ten.TenantID, tenantCohort.Name, paid, h.Name),
							"house_id":  h.ID,
							"owner_id":  ten.OwnerID,
							"tenant_id": ten.TenantID,
							"coins":     paid,
						},
					}
					w.NextID++
					events = append(events, ev)
				}
			}
		}
		mc.ForeclosedTenancies[hID] = ten
	}

	// 3. Assess rent once per debtor, then divide the same cash and output assessment
	// between the registered creditors. Splitting a claim never raises total rent.
	debtorIDs := make([]string, 0, len(mc.CohortEquityPermille))
	for dID := range mc.CohortEquityPermille {
		debtorIDs = append(debtorIDs, dID)
	}
	sort.Strings(debtorIDs)
	for _, dID := range debtorIDs {
		shares := mc.CohortEquityPermille[dID]
		sharePermille := mc.TotalAnnexedSharePermille(dID)
		debtor := w.findCohort(dID)
		if sharePermille <= 0 || debtor == nil || debtor.Population <= 0 {
			continue
		}
		creditorIDs := make([]string, 0, len(shares))
		for cID := range shares {
			creditorIDs = append(creditorIDs, cID)
		}
		sort.Strings(creditorIDs)

		var rentCoins int64
		surplusCoins := debtor.MoneyPool - int64(debtor.Population)*2
		if surplusCoins > 0 {
			rentCoins = (surplusCoins * int64(sharePermille) * int64(spec.DailyRentPermille)) / 1000000
			if rentCoins < 1 && surplusCoins >= 10 {
				rentCoins = 1
			}
			rentCoins = minInt64(rentCoins, debtor.MoneyPool)
		}

		var rentItem string
		var rentQty int64
		dTags := cohortIdentityTags(w.Catalog, debtor.Profession)
		itemKeys := make([]string, 0, len(debtor.ItemPool))
		for k, v := range debtor.ItemPool {
			if v > 0 {
				itemKeys = append(itemKeys, k)
			}
		}
		sort.Strings(itemKeys)
		for _, itemID := range itemKeys {
			if !w.Catalog.TagsProduce(dTags, itemID) {
				continue
			}
			excess := debtor.ItemPool[itemID] - int64(debtor.Population)*2
			if w.Catalog.Items[itemID].Edible {
				excess = w.cohortFoodSurplus(*debtor, itemID)
			}
			if excess <= 0 {
				continue
			}
			qty := (excess * int64(sharePermille) * int64(spec.DailyRentPermille)) / 1000000
			if qty < 1 && excess >= 10 {
				qty = 1
			}
			if qty > 0 {
				rentItem = itemID
				rentQty = minInt64(qty, debtor.ItemPool[itemID])
				break
			}
		}

		// Integer remainders carry across sorted creditors so every assessed unit
		// has one recipient, with deterministic rounding and no duplicated minimum.
		var cumulativeShare int64
		var allocatedCoins, allocatedQty int64
		for _, cID := range creditorIDs {
			cumulativeShare += int64(shares[cID])
			coinsThrough := rentCoins * cumulativeShare / int64(sharePermille)
			qtyThrough := rentQty * cumulativeShare / int64(sharePermille)
			coins, qty := coinsThrough-allocatedCoins, qtyThrough-allocatedQty
			allocatedCoins, allocatedQty = coinsThrough, qtyThrough
			creditor := w.findCohort(cID)
			if creditor == nil || creditor.Population <= 0 || debtor.ID == creditor.ID {
				continue // An unavailable creditor's entitlement stays with the debtor.
			}
			w.addCohortCurrency(debtor, "coins", -(coins))
			w.addCohortCurrency(creditor, "coins", coins)
			if qty > 0 {
				w.adjustCohortItem(debtor, rentItem, -qty, false)
				w.adjustCohortItem(creditor, rentItem, qty, false)
			}
			if coins > 0 || qty > 0 {
				ev := Event{
					ID: w.NextID, Tick: w.Tick, ActorID: debtor.ID, TargetID: creditor.ID,
					Kind:   "cohort_rent_paid",
					Action: Action{Kind: "wait", TargetID: creditor.ID, Item: rentItem, Quantity: int(qty), Price: int(coins)},
					Payload: map[string]any{
						"text":          fmt.Sprintf("%s paid land/workshop rent (%d coins, %d %s) on %d‰ annexed share to %s", debtor.Name, coins, qty, rentItem, shares[cID], creditor.Name),
						"debtor_cohort": debtor.ID, "creditor_cohort": creditor.ID,
						"share_permille": shares[cID], "coins": coins, "item": rentItem, "item_qty": qty,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}
	}

	// 4. Update personal debt facts only. Repayment and foreclosure require
	// borrower/creditor actions; the clock never chooses either on their behalf.
	// A living borrower dehydrated into a PopulationCohort is NOT dead and MUST NOT default.
	for i := range mc.Loans {
		l := &mc.Loans[i]
		if l.Status != "active" {
			continue
		}
		borrower := w.actor(l.BorrowerID)
		borrowerCohort := w.findCohort(l.BorrowerCohortID)
		if borrower != nil && borrower.Dead {
			l.Status = "defaulted"
			continue
		}
		if borrower == nil && (borrowerCohort == nil || borrowerCohort.Population <= 0) {
			l.Status = "defaulted"
			continue
		}
		owed := l.Principal + l.AccruedInterest - l.Repaid
		if owed <= 0 {
			l.Status = "repaid"
		}
	}

	// 5. Settle macro PopulationCohort debt repayments and overdue land/workshop annexation.
	for i := range mc.CohortDebts {
		d := &mc.CohortDebts[i]
		if d.Status != "active" {
			continue
		}
		debtor := w.findCohort(d.DebtorID)
		creditor := w.findCohort(d.CreditorID)
		if debtor == nil || debtor.Population <= 0 {
			d.Status = "defaulted"
			continue
		}
		owed := d.Principal + d.AccruedInterest - d.Repaid
		reserve := int64(debtor.Population) * 2
		if w.Tick >= d.DueTick {
			reserve = int64(debtor.Population)
		}
		if owed > 0 && debtor.MoneyPool > reserve && (w.Tick >= d.DueTick || debtor.MoneyPool >= owed+int64(debtor.Population)*4) {
			pay := minInt64(owed, debtor.MoneyPool-reserve)
			if pay > 0 {
				w.addCohortCurrency(debtor, "coins", -(pay))
				if creditor != nil && creditor.Population > 0 {
					w.addCohortCurrency(creditor, "coins", pay)
				} else {
					w.creditTreasury(pay, "fee")
				}
				d.Repaid += pay
				rem := d.Principal + d.AccruedInterest - d.Repaid
				if rem <= 0 {
					rem = 0
					d.Status = "repaid"
				}
				ev := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  debtor.ID,
					TargetID: d.CreditorID,
					Kind:     "cohort_loan_repaid",
					Action:   Action{Kind: "repay_loan", TargetID: d.CreditorID, OfferID: d.ID, Price: int(pay)},
					Payload: map[string]any{
						"text":            fmt.Sprintf("%s repaid %d coins on usury debt #%d (remaining %d, status %s)", debtor.Name, pay, d.ID, rem, d.Status),
						"loan_id":         d.ID,
						"creditor_cohort": d.CreditorID,
						"debtor_cohort":   debtor.ID,
						"coins":           pay,
						"remaining_owed":  rem,
						"status":          d.Status,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}
		if d.Status == "active" && w.Tick >= d.DueTick {
			rem := d.Principal + d.AccruedInterest - d.Repaid
			if rem <= 0 {
				d.Status = "repaid"
				continue
			}
			if creditor != nil && creditor.Population > 0 {
				addShare := int((rem * 250) / maxInt64(1, int64(debtor.Population)*4))
				if addShare < 100 {
					addShare = 100
				}
				if addShare > 350 {
					addShare = 350
				}
				curShare := mc.TotalAnnexedSharePermille(debtor.ID)
				newShare := minInt(spec.MaxTenantSharePermille, curShare+addShare)
				delta := newShare - curShare
				if delta <= 0 {
					continue // No equity remains to settle this debt.
				}
				if mc.CohortEquityPermille[debtor.ID] == nil {
					mc.CohortEquityPermille[debtor.ID] = map[string]int{}
				}
				mc.CohortEquityPermille[debtor.ID][creditor.ID] += delta
				d.Status = "annexed"
				ev := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  creditor.ID,
					TargetID: debtor.ID,
					Kind:     "cohort_land_annexed",
					Action:   Action{Kind: "foreclose_collateral", TargetID: debtor.ID, OfferID: d.ID},
					Payload: map[string]any{
						"text":                   fmt.Sprintf("%s annexed %d‰ land/workshop equity from %s over %d unpaid debt (total annexed %d‰)", creditor.Name, delta, debtor.Name, rem, newShare),
						"loan_id":                d.ID,
						"creditor_cohort":        creditor.ID,
						"debtor_cohort":          debtor.ID,
						"unpaid_debt":            rem,
						"annexed_delta_permille": delta,
						"total_annexed_permille": newShare,
					},
				}
				w.NextID++
				events = append(events, ev)
			} else {
				d.Status = "defaulted"
			}
		}
	}

	// 6. Issue new usury loans to distressed cohorts from liquid merchant/elite cohorts.
	gov := w.Catalog.EffectiveGovernance()
	activeCohortLoans := make(map[string]int, len(w.Cohorts))
	for _, d := range mc.CohortDebts {
		if d.Status == "active" {
			activeCohortLoans[d.DebtorID]++
		}
	}
	var merchantCreditors []*PopulationCohort
	var eliteCreditors []*PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok && ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		reserve := int64(c.Population) * int64(spec.CreditorReservePerCap)
		if c.MoneyPool <= reserve+20 {
			continue
		}
		if w.isMerchantCohort(c, spec) {
			merchantCreditors = append(merchantCreditors, c)
		} else if c.Tier >= 2 || c.MoneyPool > reserve*2 {
			eliteCreditors = append(eliteCreditors, c)
		}
	}
	sort.Slice(merchantCreditors, func(i, j int) bool {
		if merchantCreditors[i].MoneyPool != merchantCreditors[j].MoneyPool {
			return merchantCreditors[i].MoneyPool > merchantCreditors[j].MoneyPool
		}
		return merchantCreditors[i].ID < merchantCreditors[j].ID
	})
	sort.Slice(eliteCreditors, func(i, j int) bool {
		if eliteCreditors[i].MoneyPool != eliteCreditors[j].MoneyPool {
			return eliteCreditors[i].MoneyPool > eliteCreditors[j].MoneyPool
		}
		return eliteCreditors[i].ID < eliteCreditors[j].ID
	})
	creditors := append(merchantCreditors, eliteCreditors...)

	for i := range w.Cohorts {
		borrower := &w.Cohorts[i]
		if borrower.Population <= 0 || activeCohortLoans[borrower.ID] >= 2 {
			continue
		}
		if w.isMerchantCohort(borrower, spec) {
			continue
		}
		if prof, ok := w.Catalog.ProfessionByID(borrower.Profession); ok && ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		floor := int64(borrower.Population) * int64(spec.SubsistenceFloorPerCap)
		distressed := borrower.MoneyPool < floor || (borrower.AvgHunger >= 1 && borrower.MoneyPool < floor+int64(borrower.Population)*2)
		if !distressed {
			continue
		}
		targetCash := floor + int64(borrower.Population)*2
		need := targetCash - borrower.MoneyPool
		if need < 10 {
			need = 10
		}

		// Prefer a liquid creditor in the same district, otherwise the richest creditor.
		var chosen *PopulationCohort
		for _, cr := range creditors {
			if cr.ID == borrower.ID {
				continue
			}
			reserve := int64(cr.Population) * int64(spec.CreditorReservePerCap)
			if cr.MoneyPool-reserve < 10 {
				continue
			}
			if cr.District == borrower.District {
				chosen = cr
				break
			}
			if chosen == nil {
				chosen = cr
			}
		}
		if chosen == nil {
			continue
		}
		avail := chosen.MoneyPool - int64(chosen.Population)*int64(spec.CreditorReservePerCap)
		loanAmt := need
		if loanAmt > avail/2 && avail >= 20 {
			loanAmt = avail / 2
		}
		if loanAmt > avail {
			loanAmt = avail
		}
		if loanAmt <= 0 {
			continue
		}
		w.addCohortCurrency(chosen, "coins", -(loanAmt))
		w.addCohortCurrency(borrower, "coins", loanAmt)
		loanID := mc.allocLoanID()
		dueTick := w.Tick + int64(spec.LoanTermTicks)
		mc.CohortDebts = append(mc.CohortDebts, CohortDebtRecord{
			ID:           loanID,
			CreditorID:   chosen.ID,
			DebtorID:     borrower.ID,
			Principal:    loanAmt,
			RatePermille: spec.DailyLoanInterestPermille,
			IssuedTick:   w.Tick,
			DueTick:      dueTick,
			Status:       "active",
		})
		activeCohortLoans[borrower.ID]++
		ev := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  borrower.ID,
			TargetID: chosen.ID,
			Kind:     "cohort_loan_issued",
			Action:   Action{Kind: "borrow_loan", TargetID: chosen.ID, Price: int(loanAmt)},
			Payload: map[string]any{
				"text":            fmt.Sprintf("%s borrowed %d coins at %d‰/day usury from %s (due tick %d)", borrower.Name, loanAmt, spec.DailyLoanInterestPermille, chosen.Name, dueTick),
				"loan_id":         loanID,
				"creditor_cohort": chosen.ID,
				"debtor_cohort":   borrower.ID,
				"principal":       loanAmt,
				"rate_permille":   spec.DailyLoanInterestPermille,
				"due_tick":        dueTick,
			},
		}
		w.NextID++
		events = append(events, ev)
	}

	// Focal actors originate loans only through bilateral character actions.

	return events
}

// WealthGiniPermille computes the population-weighted Gini coefficient (0..1000)
// across all living individual Actors and macro PopulationCohorts.
func (w *World) WealthGiniPermille(mc *MarketCreditState) int {
	type wealthBucket struct {
		pop    int64
		wealth int64
	}
	buckets := make([]wealthBucket, 0, len(w.Actors)+len(w.Cohorts))
	var totalPop, totalWealth int64

	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead {
			continue
		}
		wealth := int64(maxInt(0, a.Money))
		buckets = append(buckets, wealthBucket{pop: 1, wealth: wealth})
		totalPop++
		totalWealth += wealth
	}
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		anon := w.anonymousCohort(*c)
		if anon.Population > 0 {
			wealth := maxInt64(0, c.MoneyPool)
			p := int64(anon.Population)
			buckets = append(buckets, wealthBucket{pop: p, wealth: wealth})
			totalPop += p
			totalWealth += wealth
		}
		for _, m := range c.Members {
			if m.Dead {
				continue
			}
			mw := int64(maxInt(0, m.Money))
			buckets = append(buckets, wealthBucket{pop: 1, wealth: mw})
			totalPop++
			totalWealth += mw
		}
	}

	if totalPop <= 0 || totalWealth <= 0 || len(buckets) <= 1 {
		return 0
	}

	sort.Slice(buckets, func(i, j int) bool {
		lhs := float64(buckets[i].wealth) * float64(buckets[j].pop)
		rhs := float64(buckets[j].wealth) * float64(buckets[i].pop)
		if lhs != rhs {
			return lhs < rhs
		}
		return buckets[i].pop < buckets[j].pop
	})

	var cumWealth float64
	var weightedArea float64
	for _, b := range buckets {
		wVal := float64(b.wealth)
		pVal := float64(b.pop)
		weightedArea += pVal * (cumWealth + 0.5*wVal)
		cumWealth += wVal
	}
	gini := 1.0 - (2.0*weightedArea)/(float64(totalPop)*cumWealth)
	permille := int(gini*1000.0 + 0.5)
	if permille < 0 {
		return 0
	}
	if permille > 1000 {
		return 1000
	}
	return permille
}

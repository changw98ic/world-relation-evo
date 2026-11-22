package sim

import (
	"fmt"
	"sort"
)

// GovernanceSpec defines declarative fiscal taxation, institutional corruption & censorate oversight,
// and syndicate protection-racket mechanics without hardcoding historical setting names.
type GovernanceSpec struct {
	MarketTaxPermille        int    `json:"market_tax_permille,omitempty"`
	TransitCustomsPermille   int    `json:"transit_customs_permille,omitempty"`
	DailyPollTaxPermille     int    `json:"daily_poll_tax_permille,omitempty"`
	SubsistenceReservePerCap int    `json:"subsistence_reserve_per_cap,omitempty"`
	CivicStipendPerCap       int    `json:"civic_stipend_per_cap,omitempty"`
	TaxSkimBasePermille      int    `json:"tax_skim_base_permille,omitempty"`
	EmbezzleBasePermille     int    `json:"embezzle_base_permille,omitempty"`
	AuditThreshold           int    `json:"audit_threshold,omitempty"`
	RacketRatePermille       int    `json:"racket_rate_permille,omitempty"`
	BribeSharePermille       int    `json:"bribe_share_permille,omitempty"`
	LawCapability            string `json:"law_capability,omitempty"`
	AuditCapability          string `json:"audit_capability,omitempty"`
	RacketTag                string `json:"racket_tag,omitempty"`
	OutlawTag                string `json:"outlaw_tag,omitempty"`
}

func DefaultGovernanceSpec() GovernanceSpec {
	return GovernanceSpec{
		MarketTaxPermille:        90,  // 9.0% commercial wholesale & retail tax (商税与铺面牙税)
		TransitCustomsPermille:   55,  // 5.5% cross-district waterway/pass octroi duty (漕关船钞与过境厘金)
		DailyPollTaxPermille:     24,  // 2.4%/day household, land & guild surplus tax above subsistence floor (夏秋丁户田赋)
		SubsistenceReservePerCap: 8,   // 8 coins/capita untaxed household livelihood buffer
		CivicStipendPerCap:       3,   // 3 coins/capita/day nominal public-service stipend
		TaxSkimBasePermille:      135, // 13.5% baseline tax-collection skimming before reaching treasury (征税截留)
		EmbezzleBasePermille:     110, // 11.0% baseline payroll/relief embezzlement by corrupt apparatus (克扣饷赈)
		AuditThreshold:           34,  // district corruption index (0..100) triggering censorate anti-corruption purge
		RacketRatePermille:       22,  // 2.2% per sweep syndicate protection fee on commercial/wharf surplus (帮派保护费/水头钱)
		BribeSharePermille:       260, // 26.0% of syndicate surplus illicit cash paid as hush bribes to law apparatus
		LawCapability:            "arrest",
		AuditCapability:          "audit_corruption",
		RacketTag:                "gang",
		OutlawTag:                "outlaw",
	}
}

// EffectiveGovernance returns the catalog's configured GovernanceSpec filled with safe defaults for omitted fields.
func (c Catalog) EffectiveGovernance() GovernanceSpec {
	g := c.Governance
	def := DefaultGovernanceSpec()
	if g.MarketTaxPermille <= 0 {
		g.MarketTaxPermille = def.MarketTaxPermille
	}
	if g.TransitCustomsPermille <= 0 {
		g.TransitCustomsPermille = def.TransitCustomsPermille
	}
	if g.DailyPollTaxPermille <= 0 {
		g.DailyPollTaxPermille = def.DailyPollTaxPermille
	}
	if g.SubsistenceReservePerCap <= 0 {
		g.SubsistenceReservePerCap = def.SubsistenceReservePerCap
	}
	if g.CivicStipendPerCap <= 0 {
		g.CivicStipendPerCap = def.CivicStipendPerCap
	}
	if g.TaxSkimBasePermille <= 0 {
		g.TaxSkimBasePermille = def.TaxSkimBasePermille
	}
	if g.EmbezzleBasePermille <= 0 {
		g.EmbezzleBasePermille = def.EmbezzleBasePermille
	}
	if g.AuditThreshold <= 0 {
		g.AuditThreshold = def.AuditThreshold
	}
	if g.RacketRatePermille <= 0 {
		g.RacketRatePermille = def.RacketRatePermille
	}
	if g.BribeSharePermille <= 0 {
		g.BribeSharePermille = def.BribeSharePermille
	}
	if g.LawCapability == "" {
		g.LawCapability = def.LawCapability
	}
	if g.AuditCapability == "" {
		g.AuditCapability = def.AuditCapability
	}
	if g.RacketTag == "" {
		g.RacketTag = def.RacketTag
	}
	if g.OutlawTag == "" {
		g.OutlawTag = def.OutlawTag
	}
	return g
}

// TreasuryLedger maintains an exact zero-error accounting ledger for w.PublicTreasury:
// InitialTreasury + TaxToTreasury + StateFeesIn + ConfiscatedIn == PublicTreasury + PayrollPaid + ReliefAndInfraOut + BountiesOut + TreasuryEmbezzled.
type TreasuryLedger struct {
	Initialized       bool  `json:"initialized,omitempty"`
	InitialTreasury   int64 `json:"initial_treasury,omitempty"`
	TaxAssessedGross  int64 `json:"tax_assessed_gross,omitempty"`
	TaxToTreasury     int64 `json:"tax_to_treasury,omitempty"`
	TaxSkimmed        int64 `json:"tax_skimmed,omitempty"`
	TaxSurcharge      int64 `json:"tax_surcharge,omitempty"`
	StateFeesIn       int64 `json:"state_fees_in,omitempty"`
	ConfiscatedIn     int64 `json:"confiscated_in,omitempty"`
	PayrollPaid       int64 `json:"payroll_paid,omitempty"`
	ReliefAndInfraOut int64 `json:"relief_and_infra_out,omitempty"`
	BountiesOut       int64 `json:"bounties_out,omitempty"`
	TreasuryEmbezzled int64 `json:"treasury_embezzled,omitempty"`
}

func (w *World) ensureTreasuryLedger() {
	if !w.TreasuryLedger.Initialized {
		w.TreasuryLedger.Initialized = true
		w.TreasuryLedger.InitialTreasury = w.PublicTreasury
	}
}

func (w *World) creditTreasury(amount int64, category string) {
	if amount <= 0 {
		return
	}
	w.ensureTreasuryLedger()
	w.PublicTreasury += amount
	switch category {
	case "tax":
		w.TreasuryLedger.TaxToTreasury += amount
	case "confiscated":
		w.TreasuryLedger.ConfiscatedIn += amount
	default:
		w.TreasuryLedger.StateFeesIn += amount
	}
}

func (w *World) debitTreasury(amount int64, category string) int64 {
	if amount <= 0 {
		return 0
	}
	w.ensureTreasuryLedger()
	if w.PublicTreasury <= 0 {
		return 0
	}
	paid := amount
	if paid > w.PublicTreasury {
		paid = w.PublicTreasury
	}
	w.PublicTreasury -= paid
	switch category {
	case "payroll":
		w.TreasuryLedger.PayrollPaid += paid
	case "bounty":
		w.TreasuryLedger.BountiesOut += paid
	case "embezzled":
		w.TreasuryLedger.TreasuryEmbezzled += paid
	default:
		w.TreasuryLedger.ReliefAndInfraOut += paid
	}
	return paid
}

// TotalCoins returns the exact total coin supply across living Actors, PopulationCohorts, PublicTreasury, and PolityNode treasuries.
func (w *World) TotalCoins() int64 {
	total := w.PublicTreasury
	if w.Polities != nil {
		for i := range w.Polities.Nodes {
			total += w.Polities.Nodes[i].Treasury
		}
	}
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			total += int64(w.Actors[i].Money)
		}
	}
	for i := range w.Cohorts {
		total += w.Cohorts[i].MoneyPool
		for _, member := range w.Cohorts[i].Members {
			if !member.Dead {
				total += int64(member.Money)
			}
		}
	}
	return total
}

// TotalIllicitWealth returns unconfiscated corrupt wealth held across cohorts and living actors.
func (w *World) TotalIllicitWealth() int64 {
	var total int64
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].IllicitGain > 0 {
			total += int64(w.Actors[i].IllicitGain)
		}
	}
	for i := range w.Cohorts {
		if w.Cohorts[i].IllicitWealth > 0 {
			total += w.Cohorts[i].IllicitWealth
		}
	}
	return total
}

// AvgCorruption returns the mean corruption index (0..100) across all districts.
func (w *World) AvgCorruption() int {
	if len(w.DistrictStates) == 0 {
		return 0
	}
	sum := 0
	for _, ds := range w.DistrictStates {
		sum += ds.Corruption
	}
	return sum / len(w.DistrictStates)
}

func (w *World) isLawCollectorCohort(c *PopulationCohort) bool {
	if c == nil || c.Population <= 0 {
		return false
	}
	gov := w.Catalog.EffectiveGovernance()
	prof, ok := w.Catalog.ProfessionByID(c.Profession)
	if !ok || ActorHasTag(prof.Grants, gov.OutlawTag) {
		return false
	}
	caps := w.Catalog.Capabilities(Actor{Tags: prof.Grants})
	if caps[gov.LawCapability] {
		return true
	}
	return w.Catalog.HasCoercivePower([]string{c.Profession}) && w.Catalog.HasPublicTrustDuty([]string{c.Profession})
}

func (w *World) findCollectorCohort(districtID string) *PopulationCohort {
	var fallback *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if !w.isLawCollectorCohort(c) {
			continue
		}
		if c.District == districtID {
			return c
		}
		if fallback == nil || c.Population > fallback.Population {
			fallback = c
		}
	}
	return fallback
}

func (w *World) addDistrictCorruption(districtID string, delta int) {
	if w.DistrictStates == nil || districtID == "" {
		return
	}
	ds, ok := w.DistrictStates[districtID]
	if !ok {
		return
	}
	ds.Corruption += delta
	if ds.Corruption < 0 {
		ds.Corruption = 0
	}
	if ds.Corruption > 100 {
		ds.Corruption = 100
	}
	w.DistrictStates[districtID] = ds
}

// collectTaxWithSkim splits an already-deducted gross tax amount into corrupt collector skimming
// (if a law/tax collector cohort exists) and net remittance into w.PublicTreasury.
func (w *World) collectTaxWithSkim(districtID string, grossTax int64) (netTreasury, skimmed int64) {
	if grossTax <= 0 {
		return 0, 0
	}
	w.ensureTreasuryLedger()
	w.TreasuryLedger.TaxAssessedGross += grossTax

	gov := w.Catalog.EffectiveGovernance()
	collector := w.findCollectorCohort(districtID)
	if collector != nil {
		corr := 15
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[districtID]; ok {
				corr = ds.Corruption
			}
		}
		skimPermille := gov.TaxSkimBasePermille + corr*2
		if skimPermille > 280 {
			skimPermille = 280
		}
		skimmed = (grossTax * int64(skimPermille)) / 1000
		if skimmed > 0 {
			collector.MoneyPool += skimmed
			collector.IllicitWealth += skimmed
			w.TreasuryLedger.TaxSkimmed += skimmed
		}
	}
	netTreasury = grossTax - skimmed
	if netTreasury > 0 {
		w.creditTreasury(netTreasury, "tax")
	}
	return netTreasury, skimmed
}

// stepCohortDailyTaxation runs once per day at 12:00 to assess household, land, and guild surplus taxes
// across non-civic, non-outlaw cohorts, complete with corrupt collector "fire-meltage" surcharges (火耗加派) and skimming.
func (w *World) stepCohortDailyTaxation() []Event {
	if !w.Catalog.EffectiveEnvironment().Institutions.TaxationEnabled() {
		return nil
	}
	clock := ClockAt(w.Tick)
	if clock.Hour != 12 || clock.Minute != 0 || len(w.Cohorts) == 0 {
		return nil
	}
	gov := w.Catalog.EffectiveGovernance()
	var totalAssessed, totalNetTreasury, totalSkimmed, totalSurcharge int64
	var taxedCohorts, taxedPopulation int

	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(c.Profession)
		if ok && ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		if w.isCivicServiceProfession(c.Profession) {
			continue
		}
		subsistence := int64(c.Population) * int64(gov.SubsistenceReservePerCap)
		taxable := c.MoneyPool - subsistence
		if taxable <= 0 {
			continue
		}
		pollPermille, targetPolity := w.EffectivePollTaxPermille(c.District, gov.DailyPollTaxPermille)
		if pollPermille <= 0 {
			continue
		}
		assessed := (taxable * int64(pollPermille)) / 1000
		if w.JusticeRevolt != nil && w.JusticeRevolt.IsDistrictTaxStriking(c.District, w.Tick) {
			assessed /= 4
		}
		if assessed < 1 {
			continue
		}

		// Corrupt collectors levy extra meltage/assessment surcharges (火耗羡余) when local corruption is elevated
		var surcharge int64
		collector := w.findCollectorCohort(c.District)
		if collector != nil && w.DistrictStates != nil {
			if ds, hasDS := w.DistrictStates[c.District]; hasDS && ds.Corruption >= 12 {
				surPermille := 90 + ds.Corruption*3 // 9%..39% surcharge on top of official tax
				if surPermille > 350 {
					surPermille = 350
				}
				surcharge = (assessed * int64(surPermille)) / 1000
				if assessed+surcharge > taxable/4 {
					surcharge = maxInt64(0, taxable/4-assessed)
				}
			}
		}
		totalDue := assessed + surcharge
		if totalDue > c.MoneyPool {
			totalDue = c.MoneyPool
			if assessed > totalDue {
				assessed = totalDue
			}
			surcharge = totalDue - assessed
		}
		if totalDue <= 0 {
			continue
		}

		c.MoneyPool -= totalDue
		var netT, skimmed int64
		if targetPolity != nil {
			targetPolity.Ledger.TaxAssessedGross += assessed
			if collector != nil {
				corr := 15
				if w.DistrictStates != nil {
					if ds, ok := w.DistrictStates[c.District]; ok {
						corr = ds.Corruption
					}
				}
				skimPermille := gov.TaxSkimBasePermille + corr*2
				if skimPermille > 280 {
					skimPermille = 280
				}
				skimmed = (assessed * int64(skimPermille)) / 1000
				if skimmed > 0 {
					collector.MoneyPool += skimmed
					collector.IllicitWealth += skimmed
					targetPolity.Ledger.TaxSkimmed += skimmed
				}
			}
			netT = assessed - skimmed
			if netT > 0 {
				targetPolity.Treasury += netT
				targetPolity.Ledger.TaxToTreasury += netT
			}
		} else {
			netT, skimmed = w.collectTaxWithSkim(c.District, assessed)
		}
		if surcharge > 0 && collector != nil {
			collector.MoneyPool += surcharge
			collector.IllicitWealth += surcharge
			if targetPolity != nil {
				targetPolity.Ledger.TaxSurcharge += surcharge
			} else {
				w.TreasuryLedger.TaxSurcharge += surcharge
			}
		} else if surcharge > 0 {
			if targetPolity != nil {
				targetPolity.Treasury += surcharge
				targetPolity.Ledger.TaxToTreasury += surcharge
			} else {
				w.creditTreasury(surcharge, "tax")
			}
			netT += surcharge
		}
		totalAssessed += assessed
		totalNetTreasury += netT
		totalSkimmed += skimmed
		totalSurcharge += surcharge
		taxedCohorts++
		taxedPopulation += c.Population
	}

	// Drift corruption upward in districts where tax skimming & meltage surcharges occurred
	if totalSkimmed+totalSurcharge > 0 && w.DistrictStates != nil {
		distIDs := make([]string, 0, len(w.DistrictStates))
		for k := range w.DistrictStates {
			distIDs = append(distIDs, k)
		}
		sort.Strings(distIDs)
		for _, dID := range distIDs {
			w.addDistrictCorruption(dID, 4)
		}
	}

	if totalAssessed == 0 && totalSurcharge == 0 {
		return nil
	}
	ev := Event{
		ID:      w.NextID,
		Tick:    w.Tick,
		ActorID: "world",
		Kind:    "cohort_tax_collected",
		Action:  Action{Kind: "wait", Price: int(totalNetTreasury)},
		Payload: map[string]any{
			"text":             fmt.Sprintf("Prefecture daily fiscal levy across %d guilds (%d taxpayers): %d official tax assessed (%d remitted to treasury, %d skimmed by collectors) + %d corrupt surcharge extorted", taxedCohorts, taxedPopulation, totalAssessed, totalNetTreasury, totalSkimmed, totalSurcharge),
			"taxed_cohorts":    taxedCohorts,
			"taxed_population": taxedPopulation,
			"gross_tax":        totalAssessed,
			"net_treasury":     totalNetTreasury,
			"tax_skimmed":      totalSkimmed,
			"surcharge_coins":  totalSurcharge,
			"treasury_balance": w.PublicTreasury,
		},
	}
	w.NextID++
	return []Event{ev}
}

// stepCohortProtectionRackets models morning market (10:00) and afternoon wharf (16:00) syndicate protection-fee
// collection ("例钱 / 码头水头规费"), temporary anti-pickpocket turf protection for paying stalls, and violent
// stall-smashing / Martial Hall counter-clashes when guilds refuse extortion.
func (w *World) stepCohortProtectionRackets() []Event {
	if !w.Catalog.EffectiveEnvironment().Institutions.RacketeeringEnabled() {
		return nil
	}
	clock := ClockAt(w.Tick)
	if (clock.Hour != 10 && clock.Hour != 16) || clock.Minute != 0 || len(w.Cohorts) == 0 {
		return nil
	}
	gov := w.Catalog.EffectiveGovernance()

	// Gather active syndicate racket cohorts (e.g. gangster & gang_boss)
	var primaryGang, bossGang *PopulationCohort
	gangForce := 0
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		eff := c.EffectivePopulation()
		if eff < 4 {
			continue
		}
		if w.Tick < c.RestUntilTick {
			eff = maxInt(4, (eff*6)/10) // beat-level racketeers still collect routine protection fees while main brawlers recuperate
		}
		prof, ok := w.Catalog.ProfessionByID(c.Profession)
		if !ok || !ActorHasTag(prof.Grants, gov.RacketTag) {
			continue
		}
		if c.Tier >= 2 {
			bossGang = c
			gangForce += eff * 2
		} else {
			if primaryGang == nil || c.Population > primaryGang.Population {
				primaryGang = c
			}
			gangForce += eff
		}
	}
	if primaryGang == nil && bossGang != nil {
		primaryGang = bossGang
	}
	if primaryGang == nil || gangForce < 6 {
		return nil
	}

	var events []Event
	var totalProtectionCoins int64
	var totalBatches int64
	var protectedTargets int
	maxSweepCap := int64(gangForce) * 14
	if maxSweepCap < 120 {
		maxSweepCap = 120
	}
	perGuildCap := maxInt64(40, maxSweepCap/4)

	// Find Martial Hall / Watch defender cohort that may guard refusing merchants
	var defender *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() < 4 || w.Tick < c.RestUntilTick {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(c.Profession)
		if !ok || ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		if ActorHasTag(prof.Grants, "martial") || ActorHasTag(prof.Grants, "law") {
			if defender == nil || ActorHasTag(prof.Grants, "martial") {
				defender = c
			}
		}
	}

	for i := range w.Cohorts {
		if totalProtectionCoins >= maxSweepCap {
			break
		}
		victim := &w.Cohorts[i]
		if victim.Population < 20 || victim.ID == primaryGang.ID || (bossGang != nil && victim.ID == bossGang.ID) {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(victim.Profession)
		if !ok || ActorHasTag(prof.Grants, gov.OutlawTag) || w.isCivicServiceProfession(victim.Profession) || w.Catalog.HasCoercivePower([]string{victim.Profession}) {
			continue
		}
		// 10:00 sweeps market/craft stalls; 16:00 sweeps canal wharves, boatmen, fishers & merchants
		isWharfOrMerchant := prof.TradeMargin || ActorHasTag(prof.Grants, "water_carrier") || ActorHasTag(prof.Grants, "water_user")
		if clock.Hour == 16 && !isWharfOrMerchant {
			continue
		}
		if clock.Hour == 10 && !prof.TradeMargin && cohortProducesEdible(w.Catalog, victim.Profession) {
			continue
		}

		reserve := int64(victim.Population) * int64(gov.SubsistenceReservePerCap+2)
		surplus := victim.MoneyPool - reserve
		if surplus <= 40 {
			continue
		}

		demand := (surplus * int64(gov.RacketRatePermille)) / 1000
		if demand < 4 {
			demand = 4
		}
		if demand > perGuildCap {
			demand = perGuildCap
		}
		remCap := maxSweepCap - totalProtectionCoins
		if demand > remCap {
			demand = remCap
		}
		if demand <= 0 {
			continue
		}

		roll := int(worldHash(w.ID, w.Tick, "racket:"+primaryGang.ID+":"+victim.ID) % 1000)
		// ~16% chance a guild refuses extortion and triggers stall-smashing / Martial Hall escort clash!
		if roll < 160 {
			enforcers := maxInt(4, minInt(48, gangForce/35))
			smashBatches := int64(maxInt(1, enforcers/6))
			seizedCoins := demand / 2
			if seizedCoins > victim.MoneyPool/40 {
				seizedCoins = victim.MoneyPool / 40
			}
			if seizedCoins > 0 {
				victim.MoneyPool -= seizedCoins
				primaryGang.MoneyPool += seizedCoins
			}
			// Smash/seize a small batch of shop stock
			var smashedItem string
			var smashedQty int64
			itemIDs := make([]string, 0, len(victim.ItemPool))
			for k, q := range victim.ItemPool {
				if q >= 8 {
					itemIDs = append(itemIDs, k)
				}
			}
			sort.Strings(itemIDs)
			if len(itemIDs) > 0 {
				smashedItem = itemIDs[0]
				smashedQty = minInt64(smashBatches*2, victim.ItemPool[smashedItem]/25)
				if smashedQty > 0 {
					looted := smashedQty / 2 // half looted, half ruined in stall smash
					ruined := smashedQty - looted
					w.adjustCohortItem(victim, smashedItem, -looted, false)
					w.adjustCohortItem(primaryGang, smashedItem, looted, false)
					w.adjustCohortItem(victim, smashedItem, -ruined, true)
				}
			}

			vicW, vicD, _ := w.applyCohortCasualtiesWithCause(victim, maxInt(1, enforcers/8), roll%2, 0, "combat")
			var gangW, gangD, gangJailed, defW int
			if defender != nil {
				gangW, gangD, gangJailed = w.applyCohortCasualtiesWithCause(primaryGang, maxInt(1, enforcers/5), enforcers/45, maxInt(0, enforcers/12), "combat")
				defW, _, _ = w.applyCohortCasualtiesWithCause(defender, enforcers/14, 0, 0, "combat")
			} else {
				gangW, _, _ = w.applyCohortCasualtiesWithCause(primaryGang, maxInt(1, enforcers/12), 0, 0, "combat")
			}
			w.addDistrictCorruption(victim.District, 1)
			if w.DistrictStates != nil {
				if ds, hasDS := w.DistrictStates[victim.District]; hasDS {
					ds.RacketPressure = minInt(100, ds.RacketPressure+4)
					w.DistrictStates[victim.District] = ds
				}
			}

			clashEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  primaryGang.ID,
				TargetID: victim.ID,
				Kind:     "cohort_racket_clashed",
				Action:   Action{Kind: "extort", TargetID: victim.ID, Location: victim.District, Item: smashedItem, Quantity: int(smashedQty), Price: int(seizedCoins)},
				Payload: map[string]any{
					"text":             fmt.Sprintf("%s refused %s protection racket in %s: enforcers smashed %d stalls (seized %d coins, %d %s damaged; %d wounded, %d killed, %d gangsters detained)", victim.Name, primaryGang.Name, victim.District, smashBatches, seizedCoins, smashedQty, smashedItem, vicW+gangW+defW, vicD+gangD, gangJailed),
					"gang_cohort":      primaryGang.ID,
					"victim_cohort":    victim.ID,
					"batches":          smashBatches,
					"protection_coins": seizedCoins,
					"smashed_item":     smashedItem,
					"smashed_qty":      smashedQty,
					"wounded":          vicW + gangW + defW,
					"deaths":           vicD + gangD,
					"jailed":           gangJailed,
				},
			}
			w.NextID++
			events = append(events, clashEv)
			totalProtectionCoins += seizedCoins
			continue
		}

		// Peaceful protection-fee compliance ("交例钱保平安")
		victim.MoneyPool -= demand
		if bossGang != nil && bossGang.ID != primaryGang.ID {
			bossCut := demand / 4
			bossGang.MoneyPool += bossCut
			primaryGang.MoneyPool += (demand - bossCut)
		} else {
			primaryGang.MoneyPool += demand
		}
		victim.ProtectedUntilTick = w.Tick + TicksPerDay
		stallBatches := maxInt64(1, int64(victim.Population)/250)
		totalProtectionCoins += demand
		totalBatches += stallBatches
		protectedTargets++
		if w.DistrictStates != nil {
			if ds, hasDS := w.DistrictStates[victim.District]; hasDS {
				ds.RacketPressure = minInt(100, ds.RacketPressure+3)
				w.DistrictStates[victim.District] = ds
			}
		}
	}

	if protectedTargets > 0 && totalProtectionCoins > 0 {
		extEv := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: primaryGang.ID,
			Kind:    "cohort_extorted",
			Action:  Action{Kind: "extort", Location: primaryGang.District, Price: int(totalProtectionCoins)},
			Payload: map[string]any{
				"text":              fmt.Sprintf("%s collected %d protection-fee coins across %d commercial/wharf guilds (%d stall/barge batches under gang turf umbrella)", primaryGang.Name, totalProtectionCoins, protectedTargets, totalBatches),
				"gang_cohort":       primaryGang.ID,
				"protected_cohorts": protectedTargets,
				"batches":           totalBatches,
				"protection_coins":  totalProtectionCoins,
			},
		}
		w.NextID++
		events = append(events, extEv)
		events = append(events, w.addCohortXP(primaryGang, 3, 2)...)
		events = append(events, w.addCohortXP(bossGang, 4, 2)...)
	}
	return events
}

// stepCohortBriberyAndCollusion settles evening (19:00) underworld payoffs to corrupt law-enforcement cohorts,
// buying advance patrol tip-offs (BribeShieldTicks) and early back-door jail releases (买通牢头捞人).
func (w *World) stepCohortBriberyAndCollusion() []Event {
	clock := ClockAt(w.Tick)
	if clock.Hour != 19 || clock.Minute != 0 || len(w.Cohorts) == 0 {
		return nil
	}
	gov := w.Catalog.EffectiveGovernance()
	var events []Event

	for i := range w.Cohorts {
		outlaw := &w.Cohorts[i]
		if outlaw.Population <= 0 {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(outlaw.Profession)
		if !ok || !ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		// Syndicates & outlaw bosses with liquid surplus bribe local law enforcement
		reserve := int64(outlaw.Population) * 8
		surplus := outlaw.MoneyPool - reserve
		if surplus < 40 {
			continue
		}
		lawTarget := w.findCollectorCohort(outlaw.District)
		if lawTarget == nil {
			continue
		}
		bribe := (surplus * int64(gov.BribeSharePermille)) / 1000
		maxBribe := int64(maxInt(150, outlaw.Population*4))
		if bribe > maxBribe {
			bribe = maxBribe
		}
		if bribe < 15 {
			continue
		}

		outlaw.MoneyPool -= bribe
		lawTarget.MoneyPool += bribe
		lawTarget.IllicitWealth += bribe

		// Collusion effects: 18-hour patrol tip-off shield + corruption increase + early back-door jail release
		w.addDistrictCorruption(lawTarget.District, 6)
		if outlaw.District != lawTarget.District {
			w.addDistrictCorruption(outlaw.District, 4)
		}
		if w.DistrictStates != nil {
			for _, dID := range []string{outlaw.District, lawTarget.District} {
				if ds, hasDS := w.DistrictStates[dID]; hasDS {
					if ds.BribeShieldTicks < 72 {
						ds.BribeShieldTicks = 72
					}
					w.DistrictStates[dID] = ds
				}
			}
		}

		bBailed := 0
		if outlaw.Jailed > 0 && bribe >= 30 {
			bBailed = minInt(outlaw.Jailed, maxInt(1, outlaw.Jailed/5))
			outlaw.Jailed -= bBailed
		}

		bribeEv := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  outlaw.ID,
			TargetID: lawTarget.ID,
			Kind:     "cohort_bribed",
			Action:   Action{Kind: "bribe", TargetID: lawTarget.ID, Location: lawTarget.District, Price: int(bribe)},
			Payload: map[string]any{
				"text":           fmt.Sprintf("%s funneled %d hush-bribe coins to %s (%d jailed members quietly bailed out; patrol tip-off shield active)", outlaw.Name, bribe, lawTarget.Name, bBailed),
				"outlaw_cohort":  outlaw.ID,
				"law_cohort":     lawTarget.ID,
				"bribe_coins":    bribe,
				"bribe_released": bBailed,
			},
		}
		w.NextID++
		events = append(events, bribeEv)
	}
	return events
}

// stepCohortAntiCorruptionAudit runs Censorate / Public-Trust anti-corruption inspections at 16:00 when
// corruption breaches AuditThreshold or on the 3-day supervisory docket, jailing corrupt personnel and
// confiscating IllicitWealth back into w.PublicTreasury.
func (w *World) stepCohortAntiCorruptionAudit() []Event {
	if !w.Catalog.EffectiveEnvironment().Institutions.OversightEnabled() {
		return nil
	}
	clock := ClockAt(w.Tick)
	if clock.Hour != 16 || clock.Minute != 0 || len(w.Cohorts) == 0 {
		return nil
	}
	gov := w.Catalog.EffectiveGovernance()
	routineDocket := (w.Tick/TicksPerDay)%3 == 2

	// Identify public-trust oversight cohort from configured oversight tags/capabilities
	var censor *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() < 2 {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(c.Profession)
		if !ok || w.Catalog.IsOutlawTags(prof.Grants) || ActorHasTag(prof.Grants, gov.OutlawTag) {
			continue
		}
		caps := w.Catalog.Capabilities(Actor{Tags: prof.Grants})
		if w.Catalog.IsOversightTags(prof.Grants) || (caps[gov.AuditCapability] && c.IllicitWealth == 0) || (w.Catalog.HasPublicTrustDuty([]string{c.Profession}) && !w.isLawCollectorCohort(c)) {
			if censor == nil || c.Tier > censor.Tier || c.Population > censor.Population {
				censor = c
			}
		}
	}

	var events []Event
	avgCorr := w.AvgCorruption()
	purgedDistricts := map[string]bool{}
	for i := range w.Cohorts {
		target := &w.Cohorts[i]
		if target.Population <= 0 || target.IllicitWealth < 120 {
			continue
		}
		localCorr := avgCorr
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[target.District]; ok && ds.Corruption > localCorr {
				localCorr = ds.Corruption
			}
		}
		if localCorr < gov.AuditThreshold && !(routineDocket && localCorr >= 22 && target.IllicitWealth >= int64(maxInt(800, target.Population*5))) {
			continue
		}

		// Seize 72% of accumulated illicit wealth (capped at target.MoneyPool) back into PublicTreasury!
		seize := (target.IllicitWealth * 720) / 1000
		if seize < 1 {
			seize = 1
		}
		if seize > target.MoneyPool {
			seize = target.MoneyPool
		}
		target.MoneyPool -= seize
		if seize >= target.IllicitWealth {
			target.IllicitWealth = 0
		} else {
			target.IllicitWealth -= seize
		}
		// Remaining hidden illicit trail cleared by half after audit restructuring
		target.IllicitWealth /= 2
		w.creditTreasury(seize, "confiscated")
		purgedDistricts[target.District] = true

		// Arrest and jail corrupt officers/clerks proportional to illicit scale
		jailReq := maxInt(2, minInt(45, int(seize/6000)+3))
		wReq := jailReq / 4
		wounded, _, jailed := w.applyCohortCasualtiesWithCause(target, wReq, 0, jailReq, "combat")

		actorID := "censorate"
		censorName := "Prefecture Censorate"
		if censor != nil {
			actorID = censor.ID
			censorName = censor.Name
			events = append(events, w.addCohortXP(censor, 5, 3)...)
		}

		jr := w.ensureJusticeRevolt()
		jr.ensureInitialized()
		caseID := jr.NextCaseID
		jr.NextCaseID++
		trial := Event{ID: w.NextID, Tick: w.Tick, ActorID: actorID, TargetID: target.ID, Kind: "court_trial_held",
			Action: Action{Kind: gov.AuditCapability, TargetID: target.ID, Location: target.District},
			Payload: map[string]any{"text": fmt.Sprintf("Case #%d found %d illicit coins held by %s", caseID, seize, target.Name),
				"district": target.District, "case_id": caseID, "suspect_cohort": target.ID, "jailed": jailed}}
		w.NextID++
		jr.CourtCases = append(jr.CourtCases, CourtCase{ID: caseID, Tick: w.Tick, District: target.District,
			Kind: "corruption_trial", MagistrateID: actorID, SuspectID: target.ID, IsCohort: true, Status: "convicted",
			JailedCount: jailed, RootEventID: trial.ID})
		events = append(events, trial)
		purgeEv := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  actorID,
			TargetID: target.ID,
			Kind:     "cohort_corruption_purged",
			Action:   Action{Kind: gov.AuditCapability, TargetID: target.ID, Location: target.District, Price: int(seize)},
			Payload: map[string]any{
				"text":              fmt.Sprintf("%s audited %s for graft & collusion: %d corrupt officers imprisoned (%d injured resisting), %d illicit coins confiscated back to Public Treasury", censorName, target.Name, jailed, wounded, seize),
				"case_id":           caseID,
				"root_event_id":     trial.ID,
				"cause_event_id":    trial.ID,
				"oversight_cohort":  actorID,
				"corrupt_cohort":    target.ID,
				"jailed":            jailed,
				"wounded":           wounded,
				"confiscated_coins": seize,
				"treasury_balance":  w.PublicTreasury,
			},
		}
		w.NextID++
		events = append(events, purgeEv)
	}
	if len(purgedDistricts) > 0 && w.DistrictStates != nil {
		for dID, ds := range w.DistrictStates {
			if purgedDistricts[dID] {
				ds.Corruption = maxInt(8, ds.Corruption-16)
				ds.BribeShieldTicks = 0
				ds.MoraleBonus = minInt(25, ds.MoraleBonus+3)
			} else {
				ds.Corruption = maxInt(8, ds.Corruption-6)
				if ds.BribeShieldTicks > 24 {
					ds.BribeShieldTicks -= 24
				}
			}
			w.DistrictStates[dID] = ds
		}
	}
	return events
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

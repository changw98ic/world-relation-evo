package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"github.com/changw98ic/world-relation-evo/internal/store"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func loadDotEnv(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	loadDotEnv(".env")
	id := flag.String("world", "village-30", "world ID")
	pop := flag.Int("pop", 30, "initial total population when creating a new world (e.g. 30 or 1000000)")
	steps := flag.Int("steps", 24, "quarter-hour ticks (15m each) to run; 0 runs continuously")
	pace := flag.Duration("pace", time.Second, "real delay per simulation tick")
	mode := flag.String("decider", "jev", "jev (production) or rules (offline test)")
	focalLimit := flag.Int("focal-limit", 0, "max focal Jev/LLM calls per tick (0 auto-caps at 8 when pop > 30)")
	focalStride := flag.Int("focal-stride", 1, "run focal Jev/LLM calls every N ticks when tiered LOD is active")
	compact := flag.Bool("compact-events", false, "persist only high-value causal/economic/progression events")
	quiet := flag.Bool("quiet", false, "suppress per-event stdout NDJSON and log periodic day summaries to stderr")
	flag.Parse()
	if *steps < 0 || *pace < 0 || *pop <= 0 {
		return fmt.Errorf("steps, pace, and pop must be positive/nonnegative")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var d sim.Decider
	switch *mode {
	case "rules":
		d = agent.Offline()
	case "jev":
		endpoint, name := os.Getenv("WRE_MODEL_URL"), os.Getenv("WRE_MODEL")
		if endpoint == "" || name == "" {
			return fmt.Errorf("jev mode requires WRE_MODEL_URL and WRE_MODEL")
		}
		jevURL := os.Getenv("WRE_JEV_URL")
		if jevURL == "" {
			jevURL = "https://api.typesafe.ai/v1/systemone"
		}
		jevName := os.Getenv("WRE_JEV_MODEL")
		if jevName == "" {
			jevName = "jev-latest"
		}
		key := os.Getenv("WRE_JEV_KEY")
		if key == "" {
			return fmt.Errorf("jev mode requires WRE_JEV_KEY")
		}
		d = agent.Pipeline{Selector: agent.Jev{Endpoint: jevURL, Name: jevName, APIKey: key}, Generator: agent.ContentModel{Endpoint: endpoint, Name: name, APIKey: os.Getenv("WRE_MODEL_KEY")}}
	default:
		return fmt.Errorf("unknown decider %q", *mode)
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required (PostgreSQL)")
	}
	db, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.Init(ctx); err != nil {
		return err
	}
	unlock, err := db.Lock(ctx, *id)
	if err != nil {
		return err
	}
	defer unlock()
	w, err := db.Load(ctx, *id)
	if errors.Is(err, sql.ErrNoRows) {
		w = sim.LargeWorld(*id, *pop)
		err = db.Save(ctx, w, nil)
	}
	if err != nil {
		return err
	}
	var td *agent.TieredDecider
	if *mode == "jev" && (*focalLimit > 0 || *focalStride > 1 || w.TotalPopulation() > 30) {
		td = agent.NewTieredDecider(d, *focalLimit)
		if *focalStride > 1 {
			td.FocalStride = *focalStride
		}
		d = td
	}
	enc := json.NewEncoder(os.Stdout)
	summary := sim.NewScaleSummary()
	for n := 0; *steps == 0 || n < *steps; n++ {
		if td != nil && w.SpotlightRumor != nil {
			td.SetSpotlightPriority(w.Actors, w.SpotlightRumor.ActorSalience, w.Tick)
		}
		events, err := w.Step(ctx, d)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		summary.RecordEvents(events)
		persisted := events
		if *compact {
			persisted = store.CompactEvents(events)
		}
		if err = db.Save(ctx, w, persisted); err != nil {
			return err
		}
		if !*quiet {
			for _, e := range persisted {
				if err = enc.Encode(e); err != nil {
					return err
				}
			}
			fmt.Fprintf(os.Stderr, "world=%s tick=%d clock=%q pop=%d actors=%d cohorts=%d events=%d decider=%s\n", w.ID, w.Tick, sim.ClockAt(w.Tick).Label, w.TotalPopulation(), len(w.Actors), len(w.Cohorts), len(persisted), *mode)
		} else if w.Tick%sim.TicksPerDay == 0 || (*steps != 0 && n+1 == *steps) {
			focalCalls := int64(0)
			if td != nil {
				focalCalls = td.TotalFocalCalls()
			}
			cal := sim.CalendarAt(w.Tick, w.Catalog.Calendar)
			tl := w.TreasuryLedger
			gini := 0
			if w.MarketCredit != nil {
				gini = w.WealthGiniPermille(w.MarketCredit)
			}
			fmt.Fprintf(os.Stderr, "world=%s tick=%d clock=%q cal=Y%d-%s-D%d pop=%d(births=%d,sick=%d,wounded=%d,jailed=%d,dead=%d,successions=%d) total_coins=%d treasury=%d fiscal(init=%d,tax_gross=%d,tax_net=%d,tax_skim=%d,surcharge=%d,fees_in=%d,confiscated_in=%d,payroll_out=%d,relief_out=%d,bounty_out=%d,embezzled_out=%d) corruption(avg_idx=%d,illicit_held=%d,embezzled=%d,bribes=%d,bribe_coins=%d,bribe_released=%d,purges=%d,jailed=%d,confiscated=%d) racket(incidents=%d,batches=%d,clashes=%d,protection_coins=%d) deaths_by_cause=%v focal_calls=%d craft_batches=%d items_produced=%d trade_batches=%d items_traded=%d coins_traded=%d logistics(waterway_batches=%d,waterway_items=%d,land_batches=%d,freight_fees=%d,loss_items=%d,shipwrecks=%d) ecology(disasters=%d,infra_repairs=%d,accidents=%d,encounters=%d,surges=%d,threat_hunts=%d,rituals=%d,compost_recycled=%d) wounds(added=%d,healed=%d,wounded_deaths=%d,active=%d) epidemics(cases=%d,cured=%d,disease_deaths=%d,other_deaths=%d,active=%d) jail(jailed=%d,released=%d,jailed_deaths=%d,active=%d) incidents(raid=%d,turf_war=%d,theft=%d,arrest=%d,spar=%d) medicine_used=%d loot_coins=%d bounty_coins=%d social_interactions=%d promoted_people=%d market(gini=%d,hoards=%d,loans=%d,repaid=%d,foreclosed=%d,annexed=%d) relations(marriages=%d,bonds=%d,grudges=%d,feuds=%d,vendettas=%d,turf_shifts=%d,charters=%d) justice(trials=%d,wrongful=%d,overturned=%d,strikes=%d,riots=%d,revolts=%d,amnesties=%d,reintegrated=%d) spotlight(promoted=%d,demoted=%d,rumor_panics=%d,gossip=%d) produced_by_item=%v\n",
				w.ID, w.Tick, sim.ClockAt(w.Tick).Label, cal.Year, cal.SeasonID, cal.DayOfSeason,
				w.TotalPopulation(), w.TotalBirths(), w.TotalSick(), w.TotalWounded(), w.TotalJailed(), w.TotalDeaths(), summary.Successions,
				w.TotalCoins(), w.PublicTreasury,
				tl.InitialTreasury, tl.TaxAssessedGross, tl.TaxToTreasury, tl.TaxSkimmed, summary.TaxSurcharge, tl.StateFeesIn, tl.ConfiscatedIn, tl.PayrollPaid, tl.ReliefAndInfraOut, tl.BountiesOut, tl.TreasuryEmbezzled,
				w.AvgCorruption(), w.TotalIllicitWealth(), summary.EmbezzledCoins, summary.BribeIncidents, summary.BribeCoins, summary.BribeReleased, summary.AuditPurges, summary.CorruptionJailed, summary.ConfiscatedCoins,
				summary.RacketIncidents, summary.RacketBatches, summary.RacketClashes, summary.ProtectionCoins,
				w.DeathsByCause, focalCalls,
				summary.CraftBatches, summary.ItemsProduced, summary.TradeBatches, summary.ItemsTraded, summary.CoinsTraded,
				summary.WaterwayBatches, summary.WaterwayItems, summary.LandFreightBatches, summary.FreightFees, summary.TransitLossItems, summary.Shipwrecks,
				summary.Disasters, summary.InfraRepairs, summary.WorkplaceAccidents, summary.WildEncounters, summary.ThreatSurges, summary.ThreatHunts, summary.Rituals, summary.CompostRecycled,
				summary.WoundedPeople, summary.HealedPeople, summary.WoundedDeaths, w.TotalWounded(),
				summary.EpidemicCases, summary.CuredPeople, w.DeathsByCause["disease"], summary.SickDiedOther, w.TotalSick(),
				summary.JailedPeople, summary.ReleasedPeople, summary.JailedDeaths, w.TotalJailed(),
				summary.RaidIncidents, summary.TurfWarIncidents, summary.TheftBatches, summary.ArrestBatches, summary.SparBatches,
				summary.MedicineUsed, summary.LootCoins, summary.BountyCoins, summary.SocialInteractions, summary.PromotedPeople,
				gini, summary.HoardEvents, summary.LoansIssued, summary.LoansRepaid, summary.HousesForeclosed, summary.CohortLandsAnnexed,
				summary.Marriages, summary.BondsSworn, summary.GrudgesFormed, summary.FeudsAvenged+summary.FeudsMediated, summary.VendettaClashes, summary.TurfShifts, summary.MonopolyCharters,
				summary.CourtTrials, summary.WrongfulConvictions, summary.AppealsOverturned, summary.TaxStrikes, summary.GranaryRiots, summary.RevoltsErupted, summary.AmnestiesDecreed, summary.RebelsReintegrated,
				summary.SpotlightPromotions, summary.SpotlightDemotions, summary.RumorPanics, summary.GossipSeeded,
				summary.ProducedByItem)
		}
		if *steps != 0 && n+1 == *steps {
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*pace):
		}
	}
	return nil
}

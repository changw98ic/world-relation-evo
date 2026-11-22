package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
	"github.com/changw98ic/world-relation-evo/internal/store"
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
	if err := runArgs(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var additionalCommands = map[string]func([]string, io.Writer, io.Writer) error{}

func runArgs(args []string, stdout, stderr io.Writer) error {
	loadDotEnv(".env")
	if len(args) > 0 {
		switch args[0] {
		case "inspect":
			return runInspect(args[1:], stdout, stderr)
		case "replay":
			return runReplay(args[1:], stdout, stderr)
		case "audit-trace":
			return runTraceAudit(args[1:], stdout, stderr)
		case "expand-options":
			return runExpandOptions(args[1:], stdout, stderr)
		case "expand-responses":
			return runExpandResponses(args[1:], stdout, stderr)
		case "expand-rulebook":
			return runExpandRulebook(args[1:], stdout, stderr)
		case "chronicle":
			return runChronicle(args[1:], stdout, stderr)
		default:
			if command := additionalCommands[args[0]]; command != nil {
				return command(args[1:], stdout, stderr)
			}
		}
	}
	return runSim(args, stdout, stderr)
}

func runSim(args []string, stdout, stderr io.Writer) (runErr error) {
	fs := flag.NewFlagSet("wre", flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("world", "village-30", "world ID")
	pop := fs.Int("pop", 30, "initial total population when creating a new world (e.g. 30 or 1000000)")
	steps := fs.Int("steps", 24, "quarter-hour ticks (15m each) to run; 0 runs continuously")
	pace := fs.Duration("pace", time.Second, "real delay per simulation tick")
	mode := fs.String("decider", "jev", "jev (production), rules (offline baseline), playbook (per-actor compiled), or rulebook (actor-agnostic generic causal rules)")
	playbookDir := fs.String("playbook-dir", "", "path to offline expand-options output directory (required when -decider=playbook)")
	rulebookPath := fs.String("rulebook", "", "path to rulebook.json/dir or 'builtin' for built-in generic base rulebook when -decider=rulebook")
	rulebookOverlay := fs.String("rulebook-overlay", "", "optional lightweight overlay JSON merged onto -rulebook (or builtin base rulebook)")
	seed := fs.Int64("seed", 0, "deterministic RNG seed for rulebook/stochastic selectors")
	compact := fs.Bool("compact-events", false, "persist only high-value causal/economic/progression events")
	quiet := fs.Bool("quiet", false, "suppress per-event stdout NDJSON and log periodic day summaries to stderr")
	packPath := fs.String("pack", "", "path to an external WorldPack JSON file to initialize a new world")
	stateIn := fs.String("state-in", "", "path to an existing World JSON snapshot to resume offline (requires -offline-run)")
	checkpointEvery := fs.Int("checkpoint-every", 24, "save deterministic replay checkpoints every N ticks (0 disables periodic DB checkpoints)")
	offlineRun := fs.Bool("offline-run", false, "run without PostgreSQL (useful with -pack or offline testing)")
	outState := fs.String("out-state", "", "optional path to write final World JSON snapshot")
	outCheckpoint := fs.String("out-checkpoint", "", "optional path to write initial World checkpoint JSON before stepping")
	outDecisions := fs.String("out-decisions", "", "optional path to write recorded per-tick TickDecisionLog JSON array")
	outEvents := fs.String("out-events", "", "optional path to write emitted events JSON array")
	outPlaybookLog := fs.String("out-playbook-log", "", "optional path to write PlaybookSelector execution telemetry and decision log JSON")
	outRulebookLog := fs.String("out-rulebook-log", "", "optional path to write RulebookSelector execution telemetry and causal log JSON")
	traceDir := fs.String("trace-dir", "", "write original model calls, perceptions and settled world states to a new evidence directory")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *steps < 0 || *pace < 0 || *pop <= 0 || *checkpointEvery < 0 {
		return fmt.Errorf("steps, pace, pop, and checkpoint-every must be valid nonnegative/positive values")
	}
	worldFlagSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "world" {
			worldFlagSet = true
		}
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var evidence *runtrace.Writer
	var traceTick int64
	var completedSteps int
	if *traceDir != "" {
		var err error
		evidence, err = runtrace.Create(*traceDir)
		if err != nil {
			return err
		}
		defer func() {
			message := ""
			if runErr != nil {
				message = runErr.Error()
			} else if ctx.Err() != nil {
				message = ctx.Err().Error()
			}
			endErr := evidence.Record("run_end", traceTick, "", map[string]any{
				"completed": runErr == nil && *steps > 0 && completedSteps == *steps,
				"error":     message,
			})
			runErr = errors.Join(runErr, endErr, evidence.Close())
		}()
	}
	if err := evidence.Record("run_start", 0, "", map[string]any{"decider": *mode, "requested_steps": *steps}); err != nil {
		return err
	}

	var (
		loadedPack *pack.WorldPack
		err        error
	)
	effectiveID := *id
	if *packPath != "" {
		loadedPack, err = pack.LoadFile(*packPath)
		if err != nil {
			return err
		}
		if !worldFlagSet {
			effectiveID = loadedPack.ID
		}
	}

	var (
		db     *store.Store
		unlock func()
		w      *sim.World
	)
	dsn := os.Getenv("DATABASE_URL")
	if !*offlineRun && dsn == "" {
		return fmt.Errorf("DATABASE_URL is required (PostgreSQL); pass -offline-run for DB-free execution")
	}
	if *stateIn != "" && !*offlineRun {
		return fmt.Errorf("-state-in requires -offline-run")
	}
	if !*offlineRun {
		db, err = store.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		if err = db.Init(ctx); err != nil {
			return err
		}
		unlock, err = db.Lock(ctx, effectiveID)
		if err != nil {
			return err
		}
		defer unlock()
		w, err = db.Load(ctx, effectiveID)
		if errors.Is(err, sql.ErrNoRows) {
			if loadedPack != nil {
				override := ""
				if worldFlagSet {
					override = effectiveID
				}
				w, err = loadedPack.Instantiate(override)
				if err != nil {
					return err
				}
			} else {
				w = sim.LargeWorld(effectiveID, *pop)
			}
			if err = db.SaveTick(ctx, w, nil, nil, *checkpointEvery > 0); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if *checkpointEvery > 0 {
			if _, _, cpErr := db.LoadNearestCheckpoint(ctx, w.ID, w.Tick); errors.Is(cpErr, sql.ErrNoRows) {
				if err = db.SaveTick(ctx, w, nil, nil, true); err != nil {
					return err
				}
			} else if cpErr != nil {
				return cpErr
			}
		}
	} else {
		if *stateIn != "" {
			rawState, readErr := os.ReadFile(*stateIn)
			if readErr != nil {
				return fmt.Errorf("read -state-in %q: %w", *stateIn, readErr)
			}
			w, err = sim.DecodeWorldSnapshot(rawState)
			if err != nil {
				return fmt.Errorf("decode -state-in %q: %w", *stateIn, err)
			}
		} else if loadedPack != nil {
			override := ""
			if worldFlagSet {
				override = effectiveID
			}
			w, err = loadedPack.Instantiate(override)
			if err != nil {
				return err
			}
		} else {
			w = sim.LargeWorld(effectiveID, *pop)
		}
	}

	d, playbookSel, rulebookSel, err := buildDecider(*mode, *playbookDir, *rulebookPath, *rulebookOverlay, w, evidence)
	if err != nil {
		return err
	}
	if rulebookSel != nil && *seed != 0 {
		rulebookSel.Seed = *seed
	}

	if err := w.Catalog.ValidatePhysicalRules(w.FlowGrid); err != nil {
		return err
	}
	if err := w.VerifyHoldingBounds(); err != nil {
		return fmt.Errorf("invalid initial world holdings: %w", err)
	}
	traceTick = w.Tick
	if evidence != nil {
		if err := evidence.WriteJSON("initial.json", w); err != nil {
			return err
		}
	}
	if *outCheckpoint != "" {
		if err := writeJSONFile(*outCheckpoint, w); err != nil {
			return fmt.Errorf("write checkpoint file: %w", err)
		}
	}

	if err := evidence.Record("run_world", w.Tick, "", map[string]any{"world_id": w.ID, "population": w.TotalPopulation(), "decider": *mode}); err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	summary := sim.NewScaleSummary()
	var (
		recordedLogs []sim.TickDecisionLog
		allEvents    []sim.Event
	)

	for n := 0; *steps == 0 || n < *steps; n++ {
		traceTick = w.Tick
		if err := evidence.Record("tick_begin", traceTick, "", map[string]any{"world": w}); err != nil {
			return err
		}
		events, tickLog, err := w.StepRecorded(ctx, d)
		if err != nil {
			traceErr := evidence.Record("step_error", traceTick, "", map[string]any{"error": err.Error()})
			if ctx.Err() != nil {
				return traceErr
			}
			return errors.Join(err, traceErr)
		}
		if err := evidence.Record("tick_settled", traceTick, "", map[string]any{"world": w, "events": events, "decisions": tickLog}); err != nil {
			return err
		}
		traceTick = w.Tick
		if err := w.VerifyLedgerConservation(); err != nil {
			return fmt.Errorf("world %q tick %d accounting: %w", w.ID, w.Tick, err)
		}
		if *outDecisions != "" {
			recordedLogs = append(recordedLogs, tickLog)
		}
		if *outEvents != "" {
			allEvents = append(allEvents, events...)
		}
		summary.RecordEvents(events)
		persisted := events
		if *compact {
			persisted = store.CompactEvents(events)
		}
		if db != nil {
			shouldCheckpoint := *checkpointEvery > 0 && w.Tick%int64(*checkpointEvery) == 0
			if err = db.SaveTick(ctx, w, persisted, &tickLog, shouldCheckpoint); err != nil {
				return err
			}
			if err := evidence.Record("tick_persisted", tickLog.Tick, "", map[string]any{"world_id": w.ID, "tick": w.Tick}); err != nil {
				return err
			}
		}
		completedSteps++
		if !*quiet {
			for _, e := range persisted {
				if err = enc.Encode(e); err != nil {
					return err
				}
			}
			fmt.Fprintf(stderr, "world=%s tick=%d clock=%q pop=%d actors=%d cohorts=%d events=%d decider=%s\n", w.ID, w.Tick, sim.ClockAt(w.Tick).Label, w.TotalPopulation(), len(w.Actors), len(w.Cohorts), len(persisted), *mode)
		} else if w.Tick%sim.TicksPerDay == 0 || (*steps != 0 && n+1 == *steps) {
			cal := sim.CalendarAt(w.Tick, w.Catalog.Calendar)
			tl := w.TreasuryLedger
			gini := 0
			if w.MarketCredit != nil {
				gini = w.WealthGiniPermille(w.MarketCredit)
			}
			fmt.Fprintf(stderr, "world=%s tick=%d clock=%q cal=Y%d-%s-D%d pop=%d(births=%d,sick=%d,wounded=%d,jailed=%d,dead=%d,successions=%d) total_coins=%d treasury=%d fiscal(init=%d,tax_gross=%d,tax_net=%d,tax_skim=%d,surcharge=%d,fees_in=%d,confiscated_in=%d,payroll_out=%d,relief_out=%d,bounty_out=%d,embezzled_out=%d) corruption(avg_idx=%d,illicit_held=%d,embezzled=%d,bribes=%d,bribe_coins=%d,bribe_released=%d,purges=%d,jailed=%d,confiscated=%d) racket(incidents=%d,batches=%d,clashes=%d,protection_coins=%d) deaths_by_cause=%v craft_batches=%d items_produced=%d trade_batches=%d items_traded=%d coins_traded=%d logistics(waterway_batches=%d,waterway_items=%d,land_batches=%d,freight_fees=%d,loss_items=%d,shipwrecks=%d) ecology(disasters=%d,infra_repairs=%d,accidents=%d,encounters=%d,surges=%d,threat_hunts=%d,rituals=%d,compost_recycled=%d) wounds(added=%d,healed=%d,wounded_deaths=%d,active=%d) epidemics(cases=%d,cured=%d,disease_deaths=%d,other_deaths=%d,active=%d) jail(jailed=%d,released=%d,jailed_deaths=%d,active=%d) incidents(raid=%d,turf_war=%d,theft=%d,arrest=%d,spar=%d) medicine_used=%d loot_coins=%d bounty_coins=%d social_interactions=%d promoted_people=%d market(gini=%d,hoards=%d,loans=%d,repaid=%d,foreclosed=%d,annexed=%d) relations(marriages=%d,bonds=%d,grudges=%d,feuds=%d,vendettas=%d,turf_shifts=%d,charters=%d) justice(trials=%d,wrongful=%d,overturned=%d,strikes=%d,riots=%d,revolts=%d,amnesties=%d,reintegrated=%d) spotlight(promoted=%d,demoted=%d,rumor_panics=%d,gossip=%d) produced_by_item=%v\n",
				w.ID, w.Tick, sim.ClockAt(w.Tick).Label, cal.Year, cal.SeasonID, cal.DayOfSeason,
				w.TotalPopulation(), w.TotalBirths(), w.TotalSick(), w.TotalWounded(), w.TotalJailed(), w.TotalDeaths(), summary.Successions,
				w.TotalCoins(), w.PublicTreasury,
				tl.InitialTreasury, tl.TaxAssessedGross, tl.TaxToTreasury, tl.TaxSkimmed, summary.TaxSurcharge, tl.StateFeesIn, tl.ConfiscatedIn, tl.PayrollPaid, tl.ReliefAndInfraOut, tl.BountiesOut, tl.TreasuryEmbezzled,
				w.AvgCorruption(), w.TotalIllicitWealth(), summary.EmbezzledCoins, summary.BribeIncidents, summary.BribeCoins, summary.BribeReleased, summary.AuditPurges, summary.CorruptionJailed, summary.ConfiscatedCoins,
				summary.RacketIncidents, summary.RacketBatches, summary.RacketClashes, summary.ProtectionCoins,
				w.DeathsByCause,
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
		if *pace > 0 {
			select {
			case <-ctx.Done():
				break
			case <-time.After(*pace):
			}
		}
		if ctx.Err() != nil {
			break
		}
	}

	if *outState != "" {
		if err := writeJSONFile(*outState, w); err != nil {
			return fmt.Errorf("write state file: %w", err)
		}
	}
	if *outDecisions != "" {
		if err := writeJSONFile(*outDecisions, recordedLogs); err != nil {
			return fmt.Errorf("write decisions file: %w", err)
		}
	}
	if *outEvents != "" {
		if err := writeJSONFile(*outEvents, allEvents); err != nil {
			return fmt.Errorf("write events file: %w", err)
		}
	}
	if *outPlaybookLog != "" && playbookSel != nil {
		if err := playbookSel.WriteLog(*outPlaybookLog); err != nil {
			return fmt.Errorf("write playbook log file: %w", err)
		}
	}
	if playbookSel != nil {
		snap := playbookSel.SnapshotLog()
		fmt.Fprintf(stderr, "playbook_summary world=%s total=%d base=%d(%d/%d unique) counter=%d(%d/%d unique) scenarios=%d/%d travel(base=%d,counter=%d) exact=%d fallback=%d survival=%d rules_fallback=%d\n",
			snap.WorldID, snap.Stats.TotalDecisions,
			snap.Stats.BaseDecisions, snap.Stats.UniqueBaseOptions, snap.Stats.TotalBaseOptions,
			snap.Stats.CounterDecisions, snap.Stats.UniqueCounters, snap.Stats.TotalCounters,
			snap.Stats.ScenariosActivated, snap.Stats.TotalScenarios,
			snap.Stats.TravelForBase, snap.Stats.TravelForCounter,
			snap.Stats.ExactMatches, snap.Stats.SemanticFallbacks,
			snap.Stats.RulesSurvival, snap.Stats.RulesFallback)
	}
	if *outRulebookLog != "" && rulebookSel != nil {
		if err := rulebookSel.WriteLog(*outRulebookLog); err != nil {
			return fmt.Errorf("write rulebook log file: %w", err)
		}
	}
	if rulebookSel != nil {
		snap := rulebookSel.SnapshotLog()
		fmt.Fprintf(stderr, "rulebook_summary world=%s total=%d proactive=%d(%d/%d unique) reaction=%d(actor2=%d,witness=%d,%d/%d unique) pairs=%d avg_pairs_per_rule=%.2f responders=%d survival=%d rules_fallback=%d\n",
			snap.WorldID, snap.Stats.TotalDecisions,
			snap.Stats.ProactiveDecisions, snap.Stats.UniqueProactiveRules, snap.Stats.TotalProactiveRules,
			snap.Stats.ReactionDecisions, snap.Stats.Actor2Reactions, snap.Stats.WitnessReactions,
			snap.Stats.UniqueReactionRules, snap.Stats.TotalReactionRules,
			snap.Stats.UniqueActorPairs, snap.Stats.AvgPairsPerFiredRule, snap.Stats.UniqueResponders,
			snap.Stats.RulesSurvival, snap.Stats.RulesFallback)
	}
	if evidence != nil {
		if err := evidence.WriteJSON("final.json", w); err != nil {
			return err
		}
	}
	return nil
}

func buildDecider(mode, playbookDir, rulebookPath, rulebookOverlay string, w *sim.World, evidence *runtrace.Writer) (sim.Decider, *agent.PlaybookSelector, *agent.RulebookSelector, error) {
	switch mode {
	case "rules":
		return agent.DecisionPipeline{
			Builder:  agent.DefaultCandidateBuilder{},
			Selector: agent.RuleDecisionSelector{},
			Trace:    evidence,
		}, nil, nil, nil
	case "playbook":
		if strings.TrimSpace(playbookDir) == "" {
			return nil, nil, nil, fmt.Errorf("playbook mode requires -playbook-dir <expand-options-dir>")
		}
		pb, err := agent.LoadPlaybook(playbookDir)
		if err != nil {
			return nil, nil, nil, err
		}
		sel := agent.NewPlaybookSelector(pb, evidence)
		return agent.DecisionPipeline{
			Builder:  agent.DefaultCandidateBuilder{},
			Selector: agent.PlaybookDecisionSelector{Selector: sel},
			Trace:    evidence,
		}, sel, nil, nil
	case "rulebook":
		rb, err := agent.LoadRulebookWithOverlay(rulebookPath, rulebookOverlay, w)
		if err != nil {
			return nil, nil, nil, err
		}
		sel := agent.NewRulebookSelector(rb, evidence)
		return agent.DecisionPipeline{
			Builder:  agent.DefaultCandidateBuilder{},
			Selector: agent.RulebookDecisionSelector{Selector: sel},
			Trace:    evidence,
		}, nil, sel, nil
	case "jev":
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
			return nil, nil, nil, fmt.Errorf("jev mode requires WRE_JEV_KEY")
		}
		var advisor *agent.RulebookSelector
		var ruleSet agent.RuleSet
		if strings.TrimSpace(rulebookPath) != "" || strings.TrimSpace(rulebookOverlay) != "" {
			rb, err := agent.LoadRulebookWithOverlay(rulebookPath, rulebookOverlay, w)
			if err != nil {
				return nil, nil, nil, err
			}
			compiled, err := (agent.DefaultRuleCompiler{}).Compile(rb, w.Catalog)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("compile rulebook for jev: %w", err)
			}
			ruleSet = compiled
			advisor = agent.NewRulebookSelector(rb, evidence)
		}
		return agent.DecisionPipeline{
			Builder: agent.DefaultCandidateBuilder{},
			Rules:   ruleSet,
			Selector: agent.JevDecisionSelector{
				Jev: agent.Jev{Endpoint: jevURL, Name: jevName, APIKey: key, Trace: evidence, RulebookAdvisor: advisor},
			},
			Trace: evidence,
		}, nil, advisor, nil
	default:
		return nil, nil, nil, fmt.Errorf("unknown decider %q", mode)
	}
}

func runInspect(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stateFile := fs.String("state-file", "", "path to serialized World JSON snapshot")
	eventsFile := fs.String("events-file", "", "path to JSON array or NDJSON file of events (for kind=lineage)")
	worldID := fs.String("world", "", "world ID in PostgreSQL (used when -state-file / -events-file are omitted)")
	kind := fs.String("kind", "", "inspection kind: actor|lineage|polity|conservation")
	target := fs.String("target", "", "actor ID or polity ID")
	eventID := fs.Int64("event", 0, "event ID (for kind=lineage)")
	fromTick := fs.Int64("from-tick", 0, "starting tick when loading events from DB")
	toTick := fs.Int64("to-tick", -1, "ending tick when loading events from DB (-1 for all)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *kind == "" {
		return fmt.Errorf("inspect requires -kind (actor|lineage|polity|conservation)")
	}

	ctx := context.Background()
	var db *store.Store
	openDB := func() (*store.Store, error) {
		if db != nil {
			return db, nil
		}
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			return nil, fmt.Errorf("DATABASE_URL is required when inspecting from DB")
		}
		s, err := store.Open(ctx, dsn)
		if err != nil {
			return nil, err
		}
		db = s
		return db, nil
	}
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()

	loadWorld := func() (*sim.World, error) {
		if *stateFile != "" {
			raw, err := os.ReadFile(*stateFile)
			if err != nil {
				return nil, fmt.Errorf("read state-file %q: %w", *stateFile, err)
			}
			w, err := sim.DecodeWorldSnapshot(raw)
			if err != nil {
				return nil, fmt.Errorf("decode state-file %q: %w", *stateFile, err)
			}
			return w, nil
		}
		if *worldID == "" {
			return nil, fmt.Errorf("inspect requires -state-file or -world")
		}
		s, err := openDB()
		if err != nil {
			return nil, err
		}
		return s.Load(ctx, *worldID)
	}

	var out any
	switch *kind {
	case "actor":
		if *target == "" {
			return fmt.Errorf("inspect -kind actor requires -target <actor_id>")
		}
		w, err := loadWorld()
		if err != nil {
			return err
		}
		out, err = sim.InspectActor(w, *target)
		if err != nil {
			return err
		}
	case "polity":
		if *target == "" {
			return fmt.Errorf("inspect -kind polity requires -target <polity_id>")
		}
		w, err := loadWorld()
		if err != nil {
			return err
		}
		out, err = sim.InspectPolity(w, *target)
		if err != nil {
			return err
		}
	case "conservation":
		w, err := loadWorld()
		if err != nil {
			return err
		}
		out = sim.InspectWorldConservation(w)
	case "lineage":
		if *eventID <= 0 {
			return fmt.Errorf("inspect -kind lineage requires -event <id>")
		}
		var (
			events []sim.Event
			err    error
		)
		if *eventsFile != "" {
			events, err = readEventsFile(*eventsFile)
			if err != nil {
				return err
			}
		} else {
			if *worldID == "" {
				return fmt.Errorf("inspect -kind lineage requires -events-file or -world")
			}
			s, err := openDB()
			if err != nil {
				return err
			}
			events, err = s.LoadEvents(ctx, *worldID, *fromTick, *toTick)
			if err != nil {
				return err
			}
		}
		out, err = sim.InspectLineage(events, *eventID)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown inspect kind %q (expected actor|lineage|polity|conservation)", *kind)
	}
	return writeJSON(stdout, out)
}

type replayOutput struct {
	WorldID    string      `json:"world_id"`
	TargetTick int64       `json:"target_tick"`
	EventCount int         `json:"event_count"`
	World      *sim.World  `json:"world"`
	Events     []sim.Event `json:"events"`
}

type replaySource interface {
	LoadNearestCheckpoint(ctx context.Context, worldID string, maxTick int64) (*sim.World, []byte, error)
	LoadDecisionLogs(ctx context.Context, worldID string, fromTick, toTick int64) ([]sim.TickDecisionLog, error)
}

func loadCheckpointAndLogsFromStore(ctx context.Context, src replaySource, worldID string, toTick int64) ([]byte, []sim.TickDecisionLog, error) {
	cpWorld, raw, err := src.LoadNearestCheckpoint(ctx, worldID, toTick)
	if err != nil {
		return nil, nil, fmt.Errorf("load nearest checkpoint for %q <= %d: %w", worldID, toTick, err)
	}
	var logs []sim.TickDecisionLog
	if toTick > cpWorld.Tick {
		logs, err = src.LoadDecisionLogs(ctx, worldID, cpWorld.Tick, toTick-1)
		if err != nil {
			return nil, nil, err
		}
	}
	return raw, logs, nil
}

func runReplay(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(stderr)
	worldID := fs.String("world", "", "world ID in PostgreSQL")
	checkpointFile := fs.String("checkpoint-file", "", "path to World checkpoint JSON file")
	decisionsFile := fs.String("decisions-file", "", "path to JSON array or NDJSON file of TickDecisionLog")
	toTick := fs.Int64("to-tick", -1, "target tick to replay up to (required)")
	outState := fs.String("out-state", "", "optional path to write replayed World JSON")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %q", fs.Args())
	}
	if *toTick < 0 {
		return fmt.Errorf("replay requires -to-tick >= 0")
	}

	var (
		cpRaw []byte
		logs  []sim.TickDecisionLog
		err   error
	)
	if *checkpointFile != "" {
		cpRaw, err = os.ReadFile(*checkpointFile)
		if err != nil {
			return fmt.Errorf("read checkpoint-file %q: %w", *checkpointFile, err)
		}
		if *decisionsFile != "" {
			logs, err = readDecisionLogsFile(*decisionsFile)
			if err != nil {
				return err
			}
		}
	} else {
		if *worldID == "" {
			return fmt.Errorf("replay requires -checkpoint-file or -world")
		}
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			return fmt.Errorf("DATABASE_URL is required when replaying from DB")
		}
		ctx := context.Background()
		db, err := store.Open(ctx, dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		cpRaw, logs, err = loadCheckpointAndLogsFromStore(ctx, db, *worldID, *toTick)
		if err != nil {
			return err
		}
	}

	replayed, events, err := sim.ReplayFromCheckpoint(cpRaw, logs, *toTick)
	if err != nil {
		return err
	}
	if *outState != "" {
		if err := writeJSONFile(*outState, replayed); err != nil {
			return fmt.Errorf("write out-state %q: %w", *outState, err)
		}
	}
	return writeJSON(stdout, replayOutput{
		WorldID:    replayed.ID,
		TargetTick: replayed.Tick,
		EventCount: len(events),
		World:      replayed,
		Events:     events,
	})
}

func readEventsFile(path string) ([]sim.Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read events-file %q: %w", path, err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var events []sim.Event
		if err := json.Unmarshal(trimmed, &events); err != nil {
			return nil, fmt.Errorf("decode events array %q: %w", path, err)
		}
		return events, nil
	}
	var events []sim.Event
	sc := bufio.NewScanner(bytes.NewReader(trimmed))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev sim.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("decode NDJSON event in %q: %w", path, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func readDecisionLogsFile(path string) ([]sim.TickDecisionLog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read decisions-file %q: %w", path, err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var logs []sim.TickDecisionLog
		if err := json.Unmarshal(trimmed, &logs); err != nil {
			return nil, fmt.Errorf("decode decision logs array %q: %w", path, err)
		}
		return logs, nil
	}
	var logs []sim.TickDecisionLog
	sc := bufio.NewScanner(bytes.NewReader(trimmed))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var entry sim.TickDecisionLog
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("decode NDJSON decision log in %q: %w", path, err)
		}
		logs = append(logs, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func writeJSON(w io.Writer, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = w.Write(raw)
	return err
}

func writeJSONFile(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

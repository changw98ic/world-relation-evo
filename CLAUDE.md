# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Requires Go 1.23+ and PostgreSQL 17 (or compatible).

### Build, Lint, and Test

```sh
go build ./cmd/wre
go vet ./...
go test ./...
go test -race ./...

# Run a single test
go test ./internal/sim -run TestCustomProfessionWorksWithoutCodeChanges -v

# Run PostgreSQL persistence tests (skipped when WRE_TEST_DATABASE_URL is unset)
WRE_TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/store -v
```

### Local Database and Simulation Runner

```sh
export WRE_DB_PASSWORD=local-development-password
docker compose up -d
export DATABASE_URL="postgres://wre:${WRE_DB_PASSWORD}@localhost:5432/wre?sslmode=disable"

# Offline deterministic baseline (672 quarter-hour ticks = 7 simulated days; -steps 0 runs until SIGINT/SIGTERM)
go run ./cmd/wre -decider rules -world village-30 -steps 672 -pace 0s

# Million-scale multi-resolution world (-pop 1000000 creates 120 active actors + 19 macro cohorts conserving 1,000,000 total inhabitants)
go run ./cmd/wre -decider rules -world prefecture-1m -pop 1000000 -compact-events -steps 96 -pace 0s

# Production two-provider pipeline (-decider jev is the default)
export WRE_JEV_KEY=your-jev-key
export WRE_JEV_MODEL=jev-latest                     # default: jev-latest
export WRE_JEV_URL=https://api.typesafe.ai/v1/systemone # optional override; complete endpoint URL
export WRE_MODEL_URL=https://your-provider.example/v1
export WRE_MODEL=your-model
export WRE_MODEL_KEY=your-key
go run ./cmd/wre -world model-village -decider jev -steps 24 -pace 0s
```

Each simulation tick represents 15 minutes (`4` ticks/hour, `96` ticks/day; `Tick 0` = Day 1 `06:00`). Actors executing multi-tick tasks (`move` along longer roads, `craft`, `repair_house`, `sleep`) advance via `World.Advance` without issuing Jev calls while busy, and only `speak` invokes `ContentModel`.

## Architecture and Core Invariants

WRE is a headless village simulation (`cmd/wre`, `internal/sim`, `internal/agent`, `internal/store`) with no UI, renderer, or scripted plot/event generator. Every emitted event originates from a character action settled by `sim.World.Apply` or `sim.World.Advance`; background hunger (`Tick%24 == 0`, every 6h) and daily house weathering (`Tick%96 == 0`) advance state without fabricating plot events.

### 1. Spatial Topology, Standalone House Assets, Energy & Sleep (`internal/sim`)

- **Road graph (`topology.go`)**: Village districts (`west_cottages`, `creek_well`, `south_fields`, `crossroads`, `market_street`, `east_lane`, `north_woods`) are connected by explicit `Road` segments with `Ticks` (`15m` units) and `EnergyCost`. Non-adjacent travel requires traversing intermediate districts; actors mid-transit (`Actor.InTransit()`) are isolated from stationary co-location checks.
- **Standalone `House` assets (`house.go`)**: `World.Houses` stores independent property assets (`ID`, `District`, `OwnerID`, `Residents`, asset `Level` `1–3`, `Comfort`, `Condition`, `Facilities` such as `compost_pit`, `carpentry_bench`, `forge`, `herb_rack`). Entering a house (`InsideHouse`) isolates perception, speech, and trade to co-occupants of that house. Owners/residents can `repair_house` with `plank`s, owners with carpenter level `>= 2` can renovate houses (`upgrade_house`), or owners can `transfer_house` ownership to a co-located villager.
- **Energy & heterogeneous sleep (`SleepRecoveryTenths`)**: Actions spend `Energy` (`0–100`). Multi-tick `sleep` restores energy per quarter-hour from four factors: house `Level`/`Comfort`/`Condition` (vs. rough outdoor exposure), personal `SleepTrait` (`deep_sleeper`, `light_sleeper`, `early_bird`, `night_owl`) + time-of-day alignment, physiological `Hunger`/injury pain (`Health < 100`), and food/coin scarcity + temperament anxiety, waking automatically when `Energy` reaches `100`.

### 2. Data-Driven Factions, Professions, Progression & 4-Stage Cognition/Physics Separation (`internal/sim/catalog.go`, `internal/agent`)

1. **Declarative catalog, guilds, career progression & candidate generation (`sim.Catalog`, `sim.CheckRecipe`, `agent.Options`)**:
   - `World.Factions` defines village guilds (`farmers_guild` 农会, `artisans_guild` 匠作行, `healers_guild` 药行, `merchants_guild` 商帮, `watch_guild` 巡守, `scholars_guild` 书院) anchored at district halls; member crafting raises `Faction.Influence` and `Faction.Level`.
   - `World.Catalog` defines `TagActions`, graded `Items` (`Tier 1–5`, `Edible`, `Tradeable`, batch size/price, surplus/shortage thresholds), `Professions` (Tier-1 base professions and Tier-2 advanced professions gated by `ParentProfession`, `MinParentLevel`, `MinFactionLevel`, and `Attributes`, plus level-indexed stage `Ranks` such as `锻造学徒` → `初级铁匠` → `匠师` and `读书人` → `秀才`), `Recipes`, and economic/social `TagProfiles`.
   - `Actor.Level` is strictly the sum of all held profession levels (`Actor.Professions`). Higher profession levels and advanced career promotions raise recipe yields (`Catalog.RecipeYield`) and merchant margins (`Catalog.SalePrice`). Adding a new profession requires only `Catalog` data—zero profession-specific branches in `sim` or `agent`.
2. **Action selection (`agent.Selector`)**: Selects only an `Option.ID` from system-generated candidates (`1–255` options).
   - `agent.Jev` (`internal/agent/jev.go`) posts `{model, state, questions.decision}` to TypeSafe System One's typed `choice` API—never chat-completions `messages`—and rejects unknown option IDs or non-`choice` answers.
   - `agent.RuleSelector` + `rulesPolicy` (`internal/agent/rules.go`) is the explicit `-decider rules` offline baseline, planning over `Catalog.Recipes`, `WorldMap.NextHop`, and `AccessibleHouses`. The engine never silently falls back from `jev` to `rules`.
3. **Content generation (`agent.Generator`, `speak` only)**: Runs only when `selected.Kind == "speak"`. `agent.CommunicationStance` computes `candid`, `conservative`, or `exaggerate` generically from `Catalog.TagProfiles` (`Produces`, `Consumes`, `CoercivePower`, `PublicTrustDuty`), item stock, and directed `Trust`. `agent.ContentModel` (`internal/agent/model.go`) enforces `response_format: json_object` and `json.Decoder.DisallowUnknownFields()`, accepting only `{text, basis_event_id, statement}`.
4. **Concurrent perception, serial settlement (`sim.World.Step`, `sim.World.Apply`, `sim.World.Advance`)**:
   - `World.View(id)` deep-copies isolated state and sorts `Nearby` deterministically by FNV-1a `(viewerID, tick, neighborID)`.
   - `World.Step` dispatches up to 4 worker goroutines for idle actors (`Task == nil`) against the **same** pre-step snapshot; any `Decide` error aborts the step before mutating `World`.
   - Settlement runs in deterministic `w.Actors` order (`Advance` for ongoing tasks, `Apply` for newly chosen actions). Multi-item trades require two separate actions: seller `offer` (`12`-tick expiry) followed by co-located buyer `accept`.

### 3. Epistemic Separation, Lineage, and Persistence (`internal/sim/world.go`, `internal/store`)

- **Observations vs. Statements**: Completed `craft` tasks populate `Memory.Observation` (`{kind, item, location, quantity}`). `speak` delivers a private memory only to speaker and `TargetID` listener with an optional `Memory.Statement` (`{topic, item, location, quantity}`), which never overwrites inventories or `Observation`s.
- **Causal chain (`BasisEventID` and `RootEventID`)**: `World.Apply` verifies `BasisEventID` matches an actual `Memory.EventID` held by the actor (emitting `action_failed` otherwise) and propagates `RootEventID`. `agent.Options` never auto-attaches `BasisEventID` to movement candidates.
- **PostgreSQL single-runner persistence**: `Store.Lock` pins a session-level `pg_try_advisory_lock` on a dedicated `*sql.Conn` (marking `driver.ErrBadConn` if unlock fails); `Store.Save` commits the `worlds` JSONB snapshot and multi-row batched `world_events` inserts in one transaction before `cmd/wre` streams NDJSON events to `stdout`.
- **Million-scale multi-resolution engine (`internal/sim/scale.go`, `internal/agent/lod.go`)**: `sim.LargeWorld(id, pop)` scales to 1,000,000–3,000,000+ population using an $O(1)$ spatial step index (`buildStepIndex` / `viewIndexed`, `MaxNearbyView = 32`), macro `PopulationCohort` bulk production/promotion/market clearing (`stepCohorts`), exact population-conserving `HydrateActor` / `DehydrateActor`, `agent.TieredDecider` per-tick focal LLM budgeting, and `store.CompactEvents`.

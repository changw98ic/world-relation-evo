# World Relation Evo (WRE)

A headless village simulation written in Go with PostgreSQL persistence.
Characters choose actions; settled actions and interactions produce events. There is no plot/event generator, map renderer, or UI.

## Run

Requires Go 1.23+ and PostgreSQL 17 (or compatible). For local development:

```sh
export WRE_DB_PASSWORD=local-development-password
# Uses the official PostgreSQL image; data persists in a named volume.
docker compose up -d
export DATABASE_URL="postgres://wre:${WRE_DB_PASSWORD}@localhost:5432/wre?sslmode=disable"
go run ./cmd/wre -decider rules -world village-30 -steps 672 -pace 0s
# Run a 1,000,000-population multi-resolution world with event compaction:
go run ./cmd/wre -decider rules -world prefecture-1m -pop 1000000 -compact-events -steps 96 -pace 0s
# Resume the same world continuously; Ctrl-C stops it.
go run ./cmd/wre -decider rules -world village-30 -steps 0 -pace 1s
```

The first run creates 30 named villagers and 16 standalone houses across 7 road-connected districts. Each tick is 15 simulated minutes (`96` ticks per day, starting at Day 1 `06:00`).
JSON events are written to stdout **after** the corresponding state and events commit. Status goes to stderr. PostgreSQL stores the complete snapshot and append-only action events together; a restart resumes the saved tick and any in-progress multi-tick tasks. Different world IDs are separate simulations. One runner holds a PostgreSQL advisory lock for each active world.

## Decision modes

`-decider rules` is a deterministic, profession-agnostic offline baseline for exercising the engine. It plans over the declarative `World.Catalog` (`Items`, `Recipes`, `TagProfiles`), navigates multi-hop roads via shortest travel time, maintains standalone `House` assets with planks, eats edible items when hungry, sleeps in its home during its circadian window or when low on energy, trades surplus goods at `market_street`, and relays or acts on item supply reports.

The production pipeline (`-decider jev`, the default) is:

**identity tags + catalog → system-defined behavior/target candidates → Jev choice → independent LLM speech content (`speak` only) → system settlement → events/state.**

Jev selects only a candidate ID for idle characters (`Task == nil`); characters mid-way through a multi-tick road walk, craft recipe, house repair, or sleep advance deterministically without redundant model calls. Only `speak` invokes the content model, which receives the fixed listener, citable memory IDs, and the dyadic supply/demand/authority stance (`candid`, `conservative`, or `exaggerate`), returning only words, assertions, and memory references. It cannot change behavior, target, movement route, house ownership, recipe, price, transferred quantities, damage, or healing.

Configure Jev and a separate content model:

```sh
export WRE_JEV_KEY=your-jev-key
export WRE_JEV_MODEL=jev-latest
# Optional complete endpoint: WRE_JEV_URL=https://api.typesafe.ai/v1/systemone
export WRE_MODEL_URL=https://your-provider.example/v1
export WRE_MODEL=your-model
export WRE_MODEL_KEY=your-key
go run ./cmd/wre -world model-village -decider jev -steps 24 -pace 0s
```

Endpoint and model are explicit; the engine never silently substitutes a model or falls back to rules. Up to four idle decisions run concurrently from the same starting snapshot, then ongoing tasks and newly chosen actions settle in stable character order. Nearby villagers are ordered deterministically per `(viewer, tick)` so candidates do not crowd low actor IDs. A failed model request stops the step before state changes. Official Jev contract: [System One](https://docs.typesafe.ai/api/system-one), [Choice](https://docs.typesafe.ai/primitives/choice).

## World mechanics and semantics

- **Physical road topology & indoor privacy**: Districts (`west_cottages`, `creek_well`, `south_fields`, `crossroads`, `market_street`, `east_lane`, `north_woods`) are linked by explicit road segments with travel duration (`15m`–`30m`) and energy cost. Houses are anchored in distinct districts; entering a house isolates perception, speech, and trade to co-occupants of that specific house.
- **Standalone house assets & asset levels**: Houses (`World.Houses`) carry `OwnerID`, `Residents`, asset `Level` (`1–3`), `Comfort` (`1–3`), structural `Condition` (`0–100`, weathering `-2`/day), and indoor `Facilities` (`compost_pit`, `carpentry_bench`, `forge`, `herb_rack`). Residents repair worn houses using `plank`s (`repair_house`), owners with carpenter level `>= 2` can renovate houses to higher asset levels (`upgrade_house`), and owners can transfer property deeds (`transfer_house`).
- **Energy & heterogeneous sleep quality**: Road travel, crafting, repairs, and combat spend `Energy` (`0–100`). Multi-tick `sleep` computes per-tick recovery from four factors: house asset level, comfort, and roof condition (vs. rough outdoor exposure), personal `SleepTrait` (`deep_sleeper`, `light_sleeper`, `early_bird`, `night_owl`) and time of day, physiological `Hunger` and injury pain (`Health < 100`), and food/coin scarcity plus temperament anxiety.
- **Factions, multi-profession character levels, item tiers & career progression**:
  - **Factions (`World.Factions`)**: Six village guilds (`farmers_guild` 农会, `artisans_guild` 匠作行, `healers_guild` 药行, `merchants_guild` 商帮, `watch_guild` 巡守, `scholars_guild` 书院) are anchored at their hall districts and level up via member production influence.
  - **Base & advanced professions + rank progression**: `Catalog.Professions` defines both Tier-1 base professions (e.g., `scholar` 读书人, `blacksmith` 铁匠, `carpenter` 木匠, `farmer` 农夫) and Tier-2 advanced professions unlocked by parent profession mastery and attributes (e.g., `scholar` 读书人 → `xiucai` 秀才, `blacksmith` 铁匠 → `master_smith` 匠师, `carpenter` → `master_carpenter` 大木匠师, `farmer` → `agronomist` 农师, `herbalist` → `alchemist` 丹药师, `peddler` → `merchant_chief` 大掌柜, `guard` → `captain` 都头). Each profession also advances through stage titles as its level grows (e.g., `锻造学徒` → `初级铁匠` → `匠师`).
  - **Character level & scaling yields**: A character's `Level` equals the sum of all held profession levels (`Professions`). Higher profession levels and advanced career promotions directly raise crafting output yields (`Catalog.RecipeYield`), merchant sale margins (`Catalog.SalePrice`), and house repair restoration. Items carry grades (`Tier 1–5`) that scale crafting XP and guild influence.
- **Epistemic separation & causal lineage**: Completed production records a first-hand `Observation`, while `speak` records a private `Statement` for speaker and listener only. `basis_event_id` must reference an actual `Memory.EventID` held by the actor; each settled event/memory propagates `root_event_id` without certifying truth or auto-attaching causes to movement options.
- **Million-scale multi-resolution population (`LargeWorld` & `PopulationCohort`)**: Worlds scale from 30 villagers up to 1,000,000–3,000,000+ inhabitants via `sim.LargeWorld(id, pop)`, combining an $O(1)$ spatially indexed active `Actor` core (with attention-capped `MaxNearbyView = 32` perception), `agent.TieredDecider` cognitive LOD budgeting (`-focal-limit`), vectorized `PopulationCohort` production/promotion/market clearing, bidirectional `HydrateActor` / `DehydrateActor` conservation, and multi-row batched PostgreSQL event persistence (`-compact-events`).

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
# Real PostgreSQL persistence tests (otherwise explicitly skipped):
WRE_TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/store -v
```

## Layout

- `internal/sim`: declarative item/recipe/profile catalog, district road graph, standalone house assets & sleep physiology, local perception, multi-tick settlement
- `internal/agent`: catalog-driven candidate builder, Jev selector, dyadic content generator, and profession-agnostic offline baseline
- `internal/store`: PostgreSQL snapshots, events, and per-world session-pinned advisory lock
- `cmd/wre`: runner CLI and committed NDJSON event stream

No license has been selected yet.

### Personal continuity

Actors can select persistent production-and-sale plans (`start_plan` / `abandon_plan`).
Plans survive sleeping, travel, day changes and PostgreSQL snapshots. Completed
craft tasks and actual sales advance their counters; speech and offered trades do
not. The current plan type is production followed by sale, not arbitrary prose goals.

`propose_agreement` creates catalog-priced future delivery terms. The specified
buyer chooses `accept_agreement` or `reject_agreement`; only `fulfill_agreement`
settles goods and payment. Pending proposals expire and overdue agreements become
unfulfilled. Participants retain their exchanges and see their own open agreements
in subsequent decisions. These are goods-for-payment commitments, not an automatic
natural-language contract parser.

Personal credit uses `borrow_loan` → lender `accept_loan` / `reject_loan`. A request
records system-quoted principal, interest, term and collateral without moving assets.
Daily settlement accrues interest but no longer originates personal loans, repays them,
or forecloses collateral on a character’s behalf. Repayment and foreclosure are explicit
character actions; pending requests can be withdrawn with `cancel_loan`. Both Jev and offline character
policies use these explicit actions; the engine only settles their consequences.

Personal memories are retained in snapshots. Each decision recalls at most 32,
combining recent context with relevant older experiences, plans and counterparties.
Observations remain observations and statements remain attributed claims. The content
model receives the character's situation without a preassigned honesty stance or
fixed quantity distortion. Offline text is a deterministic test fixture, not a claim
of model-quality realism.

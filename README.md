# World Relation Evo (WRE)

A headless world simulation written in Go, with historical, xianxia, and science-fiction world packs. Run in memory or persist worlds in PostgreSQL.
Characters choose actions; settled actions and interactions produce events. There is no plot/event generator, map renderer, or UI.

## Quick start

Requires Go 1.23+. Run commands from the repository root. Try the built-in
30-character village with rule-based decisions and in-memory state:

```sh
go run ./cmd/wre -offline-run -decider rules -steps 4 -pace 0s -quiet
```

The final status line on stderr reports `tick=4`. Omit `-quiet` to emit one JSON
event per line on stdout. Each tick advances 15 simulated minutes; a day has 96
ticks. The built-in village starts at Day 1, 06:00.

`-offline-run` disables database access; `-decider rules` disables model calls.
Use both for a run that needs neither PostgreSQL nor model credentials. In-memory
runs start a new world each time. Export files as shown below to inspect or replay
one later.

## Run with PostgreSQL

The supplied Compose service uses PostgreSQL 17 and requires Docker Compose.
Data persists in a named volume:

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

Without `-pack`, a new world uses the built-in village and the requested population.
An existing world resumes its saved state and in-progress tasks; `-pop` only
controls creation. Different world IDs are separate simulations. A PostgreSQL
session lock allows one simulation runner per world.

World snapshots and each batch of events commit in one transaction. Decision logs
and replay checkpoints are saved separately. JSON events reach stdout after these
writes succeed. `-checkpoint-every` defaults to 24 ticks; `0` disables periodic
DB checkpoints. `-compact-events` filters persisted and stdout events to selected
causal, economic, and progression events. It does not filter `-out-events` exports.

## World packs

A WorldPack JSON file defines the catalog, geography, characters, population groups,
and optional systems such as polities, ecology, energy networks, and knowledge.
The loader validates references before creating the world.

| Pack | Setting |
| --- | --- |
| [mortal_watershed.json](packs/mortal_watershed.json) | Historical |
| [xianxia_sect.json](packs/xianxia_sect.json) | Xianxia |
| [wasteland_sector.json](packs/wasteland_sector.json) | Science-fiction wasteland |

```sh
go run ./cmd/wre -offline-run -decider rules \
  -pack packs/xianxia_sect.json -steps 32 -pace 0s -quiet
```

The pack supplies the world ID unless `-world` overrides it. With PostgreSQL, packs
initialize new worlds; loading an existing world resumes its saved snapshot.

## Export, inspect, and replay

Record a short offline run. These output files are overwritten if they already exist:

```sh
go run ./cmd/wre -offline-run -decider rules -steps 4 -pace 0s -quiet \
  -out-checkpoint /tmp/wre-checkpoint.json \
  -out-decisions /tmp/wre-decisions.json \
  -out-state /tmp/wre-state.json \
  -out-events /tmp/wre-events.json

go run ./cmd/wre inspect -state-file /tmp/wre-state.json -kind conservation

go run ./cmd/wre replay -checkpoint-file /tmp/wre-checkpoint.json \
  -decisions-file /tmp/wre-decisions.json -to-tick 4 \
  -out-state /tmp/wre-replayed.json > /tmp/wre-replay-result.json

cmp /tmp/wre-state.json /tmp/wre-replayed.json
```

A successful comparison produces no output. Replay uses recorded actions to
recompute state and events without live model calls. The initial checkpoint and
decision logs must cover the requested target tick. Exports are JSON; event and
decision-log readers also accept newline-delimited JSON.

`inspect` supports `conservation`, `actor`, `polity`, and `lineage`. Use `-target`
with an actor or polity ID from the snapshot. For lineage, use `-events-file` and
`-event` with an event ID. Inspection returns JSON on stdout.

For stored worlds, use `inspect -world <world-id> -kind conservation` or
`replay -world <world-id> -to-tick <tick>` with `DATABASE_URL` set. Database replay
loads the nearest checkpoint at or before the target and its subsequent decision
logs.

## Decision modes

`-decider rules` is a deterministic, profession-agnostic offline baseline for exercising the engine. It plans over the declarative `World.Catalog` (`Items`, `Recipes`, `TagProfiles`), navigates multi-hop roads via shortest travel time, maintains standalone `House` assets with planks, eats edible items when hungry, sleeps in its home during its circadian window or when low on energy, trades surplus goods at `market_street`, and relays or acts on item supply reports.

The production pipeline (`-decider jev`, the default) is:

**identity tags + catalog → system-defined action candidates → Jev choice → text generation when needed → system settlement → events/state.**

Jev selects a candidate ID for an idle character. Ongoing tasks advance without
another decision. TypeSafe's [Choice protocol](https://docs.typesafe.ai/primitives/choice)
allows at most 255 alternatives per question. Larger action sets use successive
group and concrete-action choices. Every original action remains reachable; all
stages are recorded. A focal character's decision can therefore make multiple Jev
requests. `-focal-limit` counts focal characters per tick. The content model runs for `speak`, `transmit_message`, and
`issue_decree`. It receives the selected action and the character's perception,
including remembered experiences. It supplies words and, for communication,
assertions and memory references. Decrees use only its text. The selected behavior,
target, route, recipe, decree parameters, and trade terms remain system-controlled.

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

The CLI reads `.env` from the working directory; existing nonempty environment
variables take precedence. `WRE_MODEL_URL` and `WRE_MODEL` are required in Jev mode.
Jev's URL and model default to the values shown above. Add `-offline-run` to use
model decisions without PostgreSQL.

Idle decisions run concurrently from the same starting view. The worker count is
the CPU count bounded to 4–32, further limited by the number of idle actors.
Settlement advances ongoing tasks and non-movement actions first, then relocation
actions, in actor order. A decision error stops the step before settlement.

When population exceeds 30, `-focal-limit` is positive, or `-focal-stride` exceeds 1,
`TieredDecider` routes a
limited number of characters to Jev and the remaining active characters to rules.
`-focal-limit` controls that budget (default 8 when tiering is active);
`-focal-stride` controls how often focal decisions run. Population groups advance
in bulk in the simulation engine. This is the configured execution policy; errors
from a selected model call still propagate.

## World mechanics and semantics

- **Physical road topology & indoor privacy**: Districts (`west_cottages`, `creek_well`, `south_fields`, `crossroads`, `market_street`, `east_lane`, `north_woods`) are linked by explicit road segments with travel duration (`15m`–`30m`) and energy cost. Houses are anchored in distinct districts; entering a house isolates perception, speech, and trade to co-occupants of that specific house.
- **Standalone house assets & asset levels**: Houses (`World.Houses`) carry `OwnerID`, `Residents`, asset `Level` (`1–3`), `Comfort` (`1–3`), `Condition` (`0–100`, weathering declared by `catalog.house_wear`), and indoor `Facilities` (`compost_pit`, `carpentry_bench`, `forge`, `herb_rack`). Residents repair worn houses using `plank`s (`repair_house`), owners with carpenter level `>= 2` can renovate houses to higher asset levels (`upgrade_house`), and owners can transfer property deeds (`transfer_house`).
- **Energy & heterogeneous sleep quality**: Road travel, crafting, repairs, and combat spend `Energy` (`0–100`). Multi-tick `sleep` computes per-tick recovery from four factors: house asset level, comfort, and roof condition (vs. rough outdoor exposure), personal `SleepTrait` (`deep_sleeper`, `light_sleeper`, `early_bird`, `night_owl`) and time of day, physiological `Hunger` and injury pain (`Health < 100`), and food/coin scarcity plus temperament anxiety.
- **Factions, multi-profession character levels, item tiers & career progression**:
  - **Factions (`World.Factions`)**: Six village guilds (`farmers_guild` 农会, `artisans_guild` 匠作行, `healers_guild` 药行, `merchants_guild` 商帮, `watch_guild` 巡守, `scholars_guild` 书院) are anchored at their hall districts and level up via member production influence.
  - **Base & advanced professions + rank progression**: `Catalog.Professions` defines both Tier-1 base professions (e.g., `scholar` 读书人, `blacksmith` 铁匠, `carpenter` 木匠, `farmer` 农夫) and Tier-2 advanced professions unlocked by parent profession mastery and attributes (e.g., `scholar` 读书人 → `xiucai` 秀才, `blacksmith` 铁匠 → `master_smith` 匠师, `carpenter` → `master_carpenter` 大木匠师, `farmer` → `agronomist` 农师, `herbalist` → `alchemist` 丹药师, `peddler` → `merchant_chief` 大掌柜, `guard` → `captain` 都头). Each profession also advances through stage titles as its level grows (e.g., `锻造学徒` → `初级铁匠` → `匠师`).
  - **Character level & scaling yields**: A character's `Level` equals the sum of all held profession levels (`Professions`). Skills and facilities can improve work speed; the declared material budget bounds output. Merchant sale margins (`Catalog.SalePrice`) and house repair restoration also depend on profession levels. Items carry grades (`Tier 1–5`) that scale crafting XP and guild influence.
- **Epistemic separation & causal lineage**: Completed production records a first-hand `Observation`, while communication records attributed `Statement` memories. Perception and witness rules determine who can observe an event. `basis_event_id` must reference an actual `Memory.EventID` held by the actor; each settled event/memory propagates `root_event_id` without certifying truth or auto-attaching causes to movement options.
- **Million-scale multi-resolution population (`LargeWorld` & `PopulationCohort`)**: Worlds scale from 30 villagers up to 1,000,000–3,000,000+ inhabitants via `sim.LargeWorld(id, pop)`, combining a spatially indexed active `Actor` core (with attention-capped `MaxNearbyView = 32` perception), `agent.TieredDecider` cognitive LOD budgeting (`-focal-limit`), vectorized `PopulationCohort` production/promotion/market clearing, bidirectional `HydrateActor` / `DehydrateActor` conservation, and multi-row batched PostgreSQL event persistence (`-compact-events`).

Additional systems cover regional ecology, lifecycle, markets and personal credit,
relationships, governance and justice, and rumor propagation. World packs can define
state gauges and body forms, transitions, combat effects, currencies, and energy
networks. Knowledge and observations remain separate from world laws.

## Resource accounting

Harvesting uses finite district resource pools. The final available unit can be
extracted; there is no hidden reserve. A group completes only the batches supported by
its inputs, money, and available resources. An individual reserves resource inputs
when starting a craft task. Drinking, including automatic drinking, deducts actual
water from a local source or carried inventory.

Resource renewal uses its declared source, rate, season, weather and capacity.
Biological pools require surviving reproduction sources and elapsed maturation.
Water moves through configured rivers and aquifers; only declared inputs can add
water. Explicit zero rates or multipliers remain zero. Ore does not regenerate.
These scenario units do not establish a calibrated ecosystem.

New built-in worlds and WorldPacks initialize accounting for held items, currencies,
and district resource pools. Production, consumption, extraction, regeneration,
transport losses, and disaster losses record their own amounts. Transfers move
holdings between owners. The checker compares these entries with current stocks:
`initial + produced - consumed = current`. It does not infer entries from final
inventory differences or require natural resources to stay constant.

The simulation CLI checks the accounts before saving each tick. A mismatch stops
the run with an error. A historical snapshot without an initialized audit cannot
establish the past source of its stocks and will fail this check; use a new world
ID to run with accounting from creation. `inspect -kind conservation` reports the
check result and its error when unavailable or inconsistent.

## Production sources and cycles

World rules live in `catalog.recipes`, `catalog.resources`, `catalog.calendar`,
and `flow_grid.nodes` in each WorldPack. Item names identify the crop or species;
the engine does not select a growth rate from the world's genre.

A recipe declares starting `inputs` and/or a local `resource_pool` with
`resource_draw`. `ticks` is the preparation/work time. Biological production also
requires a `growth` rule. For example, ordinary watershed wheat uses:

```json
{
  "duration_ticks": 11520,
  "maintenance_interval_ticks": 96,
  "care_workers_per_batch": 1,
  "resource_pool": "water_table",
  "resource_draw": 1,
  "failure_after_ticks": 672,
  "source": "120-day wheat preset; daily local irrigation and field labor; seven-day continuous care-shortage survival budget is a scenario assumption"
}
```

One tick is 15 minutes. `duration_ticks` is the required growth time after
preparation. Every `maintenance_interval_ticks`, each batch consumes its declared
`supplies` and, if configured, `resource_pool` / `resource_draw`. Missing local
care stops progress. A declared `failure_after_ticks` ends a biological batch
after continuous unmet care; a failed crop records `growth_failed` and produces
no harvest. `parents` reserves existing breeding animals until harvest;
it does not consume them or make them available for another breeding batch.
The completed output quantity comes from the recipe, not a profession multiplier.
Individuals collect mature output at its production site with `harvest`.
Population groups collect locally after maturation. Owners care for their own
batches until the owner proposes terms with `propose_growth_care` and the named
caretaker agrees with `accept_growth_care`. Signing records the local supplier,
harvest permission and share in `production.care_agreements` and closes all other
invitations for the same batch. Care, harvest, delivery and each participant's
view read those signed terms. The model can offer unpaid care, an equal split,
or the whole harvest to the caretaker. An explicit proposal can specify any
integer share from 0 to 1000 per thousand. Settlement pays whole units from the
actual harvest and gives the remainder to the owner.
An absent owner retains title to locally stored harvest. Authorized deliveries
use the same timed, capacity-limited shipments as other freight. Individuals
collect their own harvest at its actual site. A remote inventory cannot supply
care.

`production.fields` records persistent land area, location, ownership and use
rights. Sowing reserves that land until harvest. Initialization supplies one
1000 m² field per farmer, with one 1000 m² crop batch per field; these are declared
scenario budgets, not agronomic calibration. Land is not created when work starts.
Installed facilities likewise declare workstation capacities; their default is
one serial workstation per facility. Broken facilities provide no working capacity.
Preparation, care, harvest and patrols share actual available labor.

A person stored in `cohorts[].members` keeps their identity, age, thirst, injuries,
professions, knowledge, wallet, inventory and property rights. Anonymous group
stocks exclude those private holdings. Their food, water, ongoing tasks and living
animals still advance. Storing or restoring a person does not create property or
consent. Ordinary group crafts wait for their declared work duration.

The built-in 30-person village represents an established settlement. Each person
starts with 480 stored meal units (120 days at the engine's four hunger increments
per day). Its 12 farmers own 12 half-grown wheat plots, initially 60 days into the
120-day cycle. One configured plot yields 1,200 meal units: 30 residents × four
meals × 120 days / 12 plots. This is a scenario food budget, not a measured yield
per seed or per hectare. Seed and irrigation inputs describe batch supplies, not
individual grains or litres. Initial stocks and standing crops are recorded before
the first ledger baseline. Ongoing harvests still require local care and elapsed
time; the rule policy prepares a following crop while the current harvest is used.
When hungry and out of edible stock, the rule policy first considers available
food offers, then reachable immediate food production. It can travel to a guild
and learn an eligible extraction profession through normal actions. Hunger-related
low health does not force endless rest while food can be obtained. Recipe travel
requires both the location feature and placement of the required resource pool;
visible local exhaustion is respected.

The ordinary watershed starts anonymous households with a finite 180-day store
from the previous season: 720 meal units per person. This covers the 120-day crop
and actual preparation, care and harvest delays. The stock is recorded at creation
and never automatically replenished. Its irrigation consumes local `water_table`
stock and daily field labor. These are explicit scenario budgets.

House wear is also declared: `interval_ticks`, `condition_loss` and `source` in
`catalog.house_wear`. Ordinary defaults lose four condition points per 365-day year.
The 25-year linear maintenance proxy is a scenario assumption, using the NPS
[wood shingle roof service-life discussion](https://www.nps.gov/orgs/1739/upload/preservation-brief-19-wood-shingle-roofs.pdf)
as a reference; it is not a universal building lifespan. Explicit zero loss is
allowed. Disasters and direct damage settle separately.

The supplied ordinary-world defaults are wheat (120 days), cattle gestation
(283 days), calf rearing (450 days), and compost maturation (60 days). These are
preset assumptions, not universal values. The period references are
[FAO crop growing periods](https://www.fao.org/4/S2022E/s2022e07.htm),
[UNL cattle gestation](https://beef.unl.edu/faq/pregnant-cows/),
[Penn State heifer development](https://extension.psu.edu/dairy-heifer-production),
and [Minnesota Extension composting](https://extension.umn.edu/garden-and-home/yard-and-garden/gardening-in-minnesota/composting-in-home-gardens).
The 450-day preset converts 15 months to 30-day months. Feed and water quantities
are abstract inventory units and remain world-design parameters. `source` records
the reference or fictional rule behind a configured cycle.

The default calendar totals 365 days: each entry in `calendar.seasons` declares
its own positive `days`. This is a seasonal calendar, not a Gregorian date or
leap-year implementation. Xianxia and science-fiction packs explicitly declare
their crop cycles, inputs and energy supplies; accelerated cycles need their own
rule rather than a genre-dependent engine shortcut.

Waste production uses food actually eaten, with fractional amounts retained until
`lifecycle.waste_per_food_divisor` is reached. Collected `waste_item` is a real
stock. `compost_recipe_id` processes it through the same timed production system.
Anonymous households retain partial meal relief in `hunger_relief_remainder`:
three portions cannot satisfy four one-portion needs. This balance survives
snapshots and population transfers. It is separate from `digested_food`, which
tracks actual consumed food for the waste cycle.

Cross-district purchases create persisted `shipments`. The seller loses the goods
at dispatch, and the buyer receives them after the route's travel time. Cargo is
counted as held stock while in transit. Route capacity is used as declared,
including zero; a zero weather/season capacity closes the route. Automatic local
retail does not supply characters across disconnected or distant districts.

Generators hold `stored_energy` up to `energy_capacity`. Their `source` declares
exactly one `fuel_item` or local `resource_pool`, plus `units_per_cycle`,
`energy_per_unit`, `cycle_ticks` and a descriptive `source`. Generation consumes
that actual stock; distribution deducts delivered energy and network losses.
`refuel` moves carried fuel into a local generator. Full storage does not burn more
fuel. Fuel, stored energy, growing batches, reserved parents and cargo survive
snapshots and replay and participate in the corresponding accounting checks.

Each production item declares `mass_units` and, when applicable,
`nutrition_units`. Each recipe declares `conversion`, including any external
material/nutrition budget and its physical source. These are scenario units, not
kilograms or calories. The output must fit that budget. Skills, guilds, durable
tools and facilities cannot multiply material. Consumable fertilizer may improve
biological output within the declared maximum budget. Tools accumulate fractional
wear across batches. Facility technology speeds actual work instead of adding
unfunded output. Its `ambient_field_inputs_per_tick` names positive operating
inputs. Only a fully supplied, powered operating tick earns that speed gain;
`ambient_field_delta_per_tick` describes separate pollution or cleanup effects.
Power loss pauses active work without charging its inputs again. A biological
batch's declared survival clock continues while its production site is unavailable.

Biological resource pools declare `biological`, `reproduction_source` and
`maturity_ticks`. New recruits need a surviving source and become available only
after maturity. The supplied maturity periods are derived from existing model
regeneration rates; they are not measured tree ages. Drinking water needs a real
potable source. River/aquifer flows move actual water, and river floods require
hydrological overflow. Hunting loot consumes its declared finite source; zero
probability stays zero. Ordinary rituals change social state. Physical disaster
relief by ritual requires an explicit `supernatural` rule.

Living-stock rules declare feeding intervals, water/feed quantities and
`death_after_ticks`. The supplied cattle budget is one water and two pasture units
per day, with death after seven days of continuous shortage. These are explicit
scenario assumptions, not veterinary thresholds. Biological batches may also
declare `failure_after_ticks`; all shipped growing crops and animals have an
explicit finite care-shortage period. These periods are scenario budgets rather
than measured species mortality thresholds. Care shortages accumulate rather
than resetting when the owner changes representation.

Pack loading rejects missing conversion budgets or physical sources, unknown
materials, nonpositive cycles, invalid energy conversions, invalid network edges,
unknown land-right holders, duplicate identities and negative initial holdings.
Existing custom packs must adopt the current fields; use a new world ID after
updating their definitions. Total accounting also rejects a negative individual
holding even if another holder offsets it. These checks verify the configured
model and its declared flows; they do not establish a physically calibrated
ecosystem. The audit repair record is in
[the simulation audit fixes](docs/simulation-audit-fixes-2026-11-16.md).

## Personal continuity

Actors can select persistent production-and-sale plans (`start_plan` / `abandon_plan`).
Plans survive sleeping, travel, day changes and PostgreSQL snapshots. Completed
craft tasks and actual sales advance their counters; speech and offered trades do
not. The plan sells one catalog trade lot as soon as that lot is available, while
retaining the full seasonal sales target. The rule policy starts the following
crop on available authorized land while selling the previous harvest. The current
plan type is production followed by sale, not arbitrary prose goals.

A delivery proposal records the supplier, buyer, item, quantity, total price in
coins and due tick in one agreement. The catalog supplies a suggested quote.
An explicit proposal can set its own quantity, total price (including zero) and
deadline. The buyer can accept or reject the recorded terms. Fulfillment uses
those same terms and checks that the people can meet, the trade is permitted,
and the goods and money are available. Pending proposals expire and overdue agreements become
unfulfilled. Participants retain their exchanges and see their own open agreements
in subsequent decisions. These are goods-for-payment commitments, not an automatic
natural-language contract parser.

Personal credit uses `borrow_loan` → lender `accept_loan` / `reject_loan`. A request
records system-quoted principal, interest, term and collateral without moving assets.
Daily settlement accrues interest but no longer originates personal loans, repays them,
or forecloses collateral on a character’s behalf. Repayment and foreclosure are explicit
character actions; pending requests can be withdrawn with `cancel_loan`. Both Jev and offline character
policies use these explicit actions; the engine only settles their consequences.
Unsecured loans cannot seize a house that was never pledged. Group equity is
stored in `cohort_equity_permille` as debtor → creditor → share. Multiple creditors
retain separate shares and receive their own portion of rent; a later creditor
cannot replace an earlier one. Wages share one treasury budget. Food procurement pays only against goods actually
delivered and funded. A capped subsidy reduces the purchased quantity; unpaid
goods stay with their owner. Delivery adds inventory; hunger decreases only when
that inventory is eaten. A `relief_grant` decree transfers its requested positive cash amount
to eligible residents; it does not create grain. Named people, stored members and
anonymous residents are counted separately. Retail, food relief procurement and
in-kind rent share the same reserve for food needed until the next crop. A crop
profession needs actual usable land before its full crop period sets that reserve.
The anonymous reserve counts living residents; retained records of dead members
do not reduce living residents' food reserves.
Advanced population groups retain their parent profession's entry qualification
and inherited permissions when selecting work.

Personal memories are retained in snapshots. Each decision recalls at most 32,
combining recent context with relevant older experiences, plans and counterparties.
Model requests retain the latest text in each directed conversation, plus recalled
structured claims, observations and source IDs. Omitted older text and archived
closed agreements are disclosed; full history remains in snapshots and traces.
Open agreements remain complete. Observations remain observations and statements
remain attributed claims. The content
model receives the character's situation without a preassigned honesty stance or
fixed quantity distortion. Offline text is a deterministic test fixture, not a claim
of model-quality realism.

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
```

The tests were rebuilt around declared budgets, actual delivery, ownership,
consent, spatial locality, and information provenance. Each rule has a valid
control and a counterexample. Default tests use memory and local HTTP fixtures;
they do not implicitly call real models or PostgreSQL.

Select real database tests explicitly. Missing configuration fails rather than
skipping the requested checks:

```sh
WRE_TEST_DATABASE_URL="$DATABASE_URL" \
  go test -tags integration ./internal/store -count=1 -v
```

The real model workflow builds and starts the executable, calls the configured
Jev and content providers, saves to PostgreSQL, restarts from that database, and
replays persisted decisions. Configure the model variables above and
`DATABASE_URL`, then run:

```sh
go test -tags live ./cmd/wre \
  -run '^TestLiveModelWorkflowRecordsEvidenceAcrossDatabaseRestart$' \
  -count=1 -v -timeout 30m
```

Every simulation mode can record original evidence in a new directory:

```sh
go run ./cmd/wre -offline-run -decider rules -steps 4 -pace 0 -quiet \
  -trace-dir /tmp/wre-evidence-new-run

go run ./cmd/wre audit-trace -trace-dir /tmp/wre-evidence-new-run \
  > /tmp/wre-evidence-review.json
```

Trace includes original model request/response bodies, perceptions, candidates,
actions, settled state, events, persistence acknowledgments, and failures.
Tiered runs record both model and rule decisions with their `decision_source`;
Rule decisions record which preferred actions were unavailable before the person
chose an available action. Trace review compares immediate work instructions to
the actual choices, delivery proposals to stored contracts, and accepted care
agreements to the previously signed caretaker for the same crop.
Existing evidence is not overwritten. Authentication headers and configured
credentials are not recorded as fields. Review reports keep simulation
correctness as `not_established`, including when execution and replay succeed.

Long food-cycle observation records every decision and event, checks the ledger
each tick, records actual harvests and deaths, and replays the last day against
the final state:

```sh
go test -tags longrun ./internal/agent -run '^TestLongFoodCycles$' -count=1 -v -timeout 0
```

The default observation is `365` simulated days. Finite previous-season stores
can postpone the first planting, so two crop durations alone do not guarantee
that two harvests are observable. These rule-driven runs do not call models.
Starvation is an outcome to investigate, not a failed invariant by itself.
Review work, income, access, choices and actual food settlement before attributing
a death to behavior or an engine defect. The controlled starvation test includes
both a character who leaves available food uncollected and a character who works
and eats; their different outcomes must remain possible.
The November 17 village and watershed observations each have 35,040 consecutive
tick records. The November 17 build reproduced both final states from day 360.
The repair review records actual deaths, harvests, retained historical test failures,
and the scope of these checks. Physical and social calibration remains unestablished.
See the [November 17 repair review](docs/testing/full-repair-2026-11-17.md) and
the [November 18 world-rules review](docs/testing/world-rules-2026-11-18.md) for
the contract, harvest, movement and starvation findings and their replay scope.

The actual 2026-11-16 model run completed 32 ticks with 254 Jev and 115 content
responses, but trace review found zero-output water production, ungrounded task
narration, and repeated travel requiring investigation. See the
[testing rules and commands](docs/testing/README.md) and the
[real-run findings and evidence](docs/testing/live-trace-review-2026-11-16.md).
Workflow completion is not evidence that the simulated world is valid.

## Layout

- `cmd/wre`: simulation, inspection, and replay commands
- `internal/sim`: world state, perception, action settlement, population groups, world systems, and replay
- `internal/agent`: action candidates, Jev selection, content generation, rule decisions, and model-call budgeting
- `internal/pack`: WorldPack loading, validation, and world creation
- `internal/store`: PostgreSQL snapshots, events, decision logs, checkpoints, and world locks
- `packs`: reference world definitions

No license has been selected yet.

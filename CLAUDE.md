# Development guide

WRE is a headless Go simulation. It has no renderer or scripted plot generator.
Run commands from the repository root. Go 1.23+ is required.

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/wre
```

Default tests use memory and local HTTP protocol fixtures. Explicit external checks:

```sh
WRE_TEST_DATABASE_URL="$DATABASE_URL" go test -tags integration ./internal/store -count=1 -v
go test -tags live ./cmd/wre -run '^TestLiveRuleWorkflowRecordsEvidenceAcrossDatabaseRestart$' -count=1 -v -timeout 30m
go test -tags longrun ./internal/agent -run '^TestLongFoodCycles$' -count=1 -v -timeout 0
```

The live test requires PostgreSQL.
It starts two real simulation processes and compares database replay with final
state. The long test records rule-driven village and watershed food cycles for
365 simulated days by default. Set `WRE_FOOD_TEST_DAYS` for a shorter observation.
Missing external configuration is a test failure.

## Engine boundaries

- One tick is 15 minutes; there are 96 ticks per day. Tick zero is 06:00.
- `internal/sim` owns state, settlement, population groups, perception, and
  scheduled wakes. `internal/agent` projects outcomes and scores them against
  the actor's needs. `internal/pack` validates world definitions.
- Actors with due choices use the local planner. The planner can continue or
  repair a viable plan before generating alternatives. Settlement processes
  actions in a stable order and checks their conditions against true state.
- `internal/store` commits world state, actors, cohorts, decisions, events and
  any due checkpoint in one transaction.

- Material and nutrition budgets bound output. Skill and facilities cannot create
  extra material. Biological growth requires declared sources, elapsed time and
  local care. Preparation, care, harvest and patrols share available workers.
- Fields preserve actual area, location, ownership and use rights. A stored person
  preserves identity and private assets. Anonymous stocks exclude member assets.
- Rivers move actual water. Biological renewal requires surviving sources and
  maturation. Resource zero remains depleted. Extracting the final unit is legal.
- `catalog.house_wear` defines interval, loss and source. Ordinary defaults use
  four points per 365-day year; disasters and direct damage are separate.
- Production plans sell available catalog lots toward their full target. The
  next crop can start on free authorized land before the previous harvest sells.
- An observation records experience; a statement remains an attributed claim.
  A memory reference must name an event actually held by that person.
- Actor records retain observations, preferences, active plans, habits and the
  next scheduled wake. Trace and replay evidence expose failures and omissions.

## Evidence

Use `-trace-dir <new-directory>` and then
`go run ./cmd/wre audit-trace -trace-dir <directory>` for review. Original evidence
must remain intact. A balanced ledger, passing test or completed workflow proves
only what that check observes. It does not establish a calibrated society.

Read [testing rules](docs/testing/README.md),
[the rerun review](docs/testing/rerun-review-2026-11-17.md), and
[the full repair review](docs/testing/full-repair-2026-11-17.md) for current status.
The 73-entry audit report is historical; each claim needs current evidence.

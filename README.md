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
go run ./cmd/wre -decider rules -world village-30 -steps 168 -pace 0s
# Resume the same world continuously; Ctrl-C stops it.
go run ./cmd/wre -decider rules -world village-30 -steps 0 -pace 1s
```

The first run creates 30 named villagers. Each tick is one simulated hour.
JSON events are written to stdout **after** the corresponding state and events commit. Status goes to stderr. PostgreSQL stores the complete snapshot and append-only action events together; a restart resumes the saved tick. Different world IDs are separate simulations. One runner holds a PostgreSQL advisory lock for each active world.

## Decision modes

`-decider rules` is a deterministic offline baseline for exercising the engine. It models subsistence, surplus trade and direct speech, **not open-ended AI intelligence**. Farmers harvest 2 food per work action, other villagers can gather 1. The initial economy has finite coins, no minting or scripted disasters. Characters sleep/wait, eat, work, travel, make trade offers, accept/reject and speak. The offline policy also retells recent observed/heard food-production news and uses trusted reports to choose where to seek food. It uses explicit structured claims, not general natural-language understanding. Social sharing is scheduled at three-hour intervals so it does not displace all subsistence work.

The production pipeline (`-decider jev`, the default) is:

**identity tags → system-defined behavior/target candidates → Jev choice → independent LLM content → system settlement → events/state.**

Jev selects only a candidate ID. Its official typed Choice request does not generate prose. The content model receives the already-selected action and target and returns only words, assertions and memory references. It cannot change behavior, target, movement destination, price, transferred quantities, damage or healing.

Tag grants: `resident` permits waiting, moving, eating and talking; `farmer`/`gatherer` permit work; `trader` permits selling proposals and buying/rejecting offers; `fighter` permits attacks; `healer` permits treatment. The seed includes a healer and a fighter. The system calculates fixed initial food yields, sale terms (2 food/2 coins), attack damage (10), treatment (15 health, consumes one medicine), hunger and trade effects. These are prototype world rules, not model outputs.

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

Endpoint/model are explicit; the engine never silently substitutes a model or falls back to rules. Four decisions run concurrently from the same starting snapshot, then actions settle in stable character order. A failed model request stops the step before state changes. One full simulated day makes 720 Jev selections and up to 720 independent content-generation calls (waiting skips generation), so start with a few steps when testing providers. Official Jev contract: [System One](https://docs.typesafe.ai/api/system-one), [Choice](https://docs.typesafe.ai/primitives/choice).

A character sees its own state, goals, personality, trust, last 32 memories, nearby identities and its trade offers, not everyone else's inventory or global history. Full memories remain in the saved world. This first slice has recent-memory context, not semantic long-term retrieval.

## Semantics

- Actions are intentions. Only the resolver changes world state.
- Selling requires an offer and a separate buyer acceptance. An offer alone transfers nothing.
- Speech records exactly what was said, who said it and to whom. It creates a heard-memory for the listener, not a new objective fact. Third parties do not automatically learn it.
- A direct `observation` is separate from a speaker's `statement`. A statement's location/quantity records the speaker's assertion; it never overwrites production or inventory. The speaker chooses the wording and any exaggeration, with no automatic distortion parameter.
- `basis_event_id` references a memory actually available to the acting character. Each resulting event/memory keeps its root event ID and direct source; following event links reconstructs the whole transmission chain. Root provenance does not certify truth. Movement candidates do not infer causal sources merely because a message mentions the destination; the offline behavior test uses a with/without-message comparison instead. A listener sees the immediate message, not the original world's fact payload.
- The multi-hop test traces A's harvest → A tells B → B tells C → C exaggerates to D → D seeks food at the fields. It verifies exact speakers, times, changed quantities, causal links and ignorance of an uninvolved villager.
- Events retain their initiating action and actor. Background hunger accumulation advances physiology, not fabricated plot events.
- Trade updates directed trust independently of money and food. Other relationship types and explicit beliefs are not yet implemented.
- Travel takes one tick between the four abstract locations; this is a small village, not geographic simulation.
- The offline policy uses simple preferences. Jev selects supported behaviors/targets and the independent model writes content. Complex negotiation, farming seasons, detailed injury physiology, reproduction and organizations are later scope.

## Tests

```sh
go test ./...
go test -race ./...
go vet ./...
# Real PostgreSQL persistence tests (otherwise explicitly skipped):
WRE_TEST_DATABASE_URL="$DATABASE_URL" go test ./internal/store -v
```

Tests cover bilateral trades, private speech, perception isolation, decision failure, seven virtual days with 30 villagers, relationship-dependent decisions and separate mock HTTP Jev/content providers. Tests also verify tag grants and that content cannot supply actions or state fields. These do not establish real-model emergent behavior; run and inspect a model-backed world separately.

## Layout

- `internal/sim`: world data, local perception, action settlement, simulation clock
- `internal/agent`: candidate builder, Jev selector, content generator, and explicit offline equivalents
- `internal/store`: PostgreSQL snapshots, events and per-world runner lock
- `cmd/wre`: background runner and JSON event output

No license has been selected yet.

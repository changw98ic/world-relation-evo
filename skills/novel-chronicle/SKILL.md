---
name: novel-chronicle
description: Turn any novel outline into a conserved, deterministic multi-stage simulation using the WRE CLI, then write a grounded, detailed chronicle strictly anchored to simulated events and causal arcs.
---

# Novel Outline -> Simulated World Chronicle (WRE Skill)

Use this workflow whenever a user provides a novel outline (any genre: xianxia, historical court intrigue, sci-fi, modern, fantasy) and wants to simulate how the world actually evolves and compile a detailed chronicle.

## Core Principles

1. **Never fabricate simulated history.** Every plot turn, death, alliance, betrayal, war, or secret exposure written into the final chronicle MUST cite real simulated evidence (`[Day X, #EventID]`).
2. **Use `blueprint.json` (~150 lines), not raw 3,000-line `WorldPack`.** Let `wre blueprint compile` auto-generate survival food/water, lodging, connected roads, communication channels, and ledger conservation scaffolding.
3. **Simulate in narrative volumes/stages.** Run Stage 1 -> inspect `chronicle` -> optionally inject Stage 2 external events or newly entering characters via `wre inject` -> resume offline via `-state-in`.
4. **Drill down before writing.** Use `wre chronicle` for macro arcs/biographies, then `wre query` (by `-actor`, `-between`, `-arc`, `-from-day/-to-day`) to inspect exact causal chains without physiological noise.

---

## Step-by-Step Workflow

Create a dedicated working directory (e.g. `.runs/<novel-id>/`) for all intermediate files.

### Step 1: Compile the Novel Outline into `blueprint.json`

Extract the outline into a compact JSON blueprint with:
- World metadata (`id`, `name`, `genre`, `summary`, `currency_name`, `actor_noun`, `food_item_id/name`, `supply_days`)
- `districts` (and optional `roads`; omitted roads auto-connect)
- `factions` (with `leader_id` so leaders automatically receive diplomatic authority)
- `actors` (with `personality` keywords like `ruthless`, `cunning`, `righteous`, `patient`, `ambitious`, `cautious`, `diplomatic`, `vengeful`, `inquisitive`; initial `trust` map; initial `ambitions` such as `avenge_blood`, `expose_corrupt`, `ascend_tier`, `seize_leadership`, `accumulate_wealth`)
- Initial `mentorships`, `kinships`, `grudges`, `diplomacy` (`allied`/`friendly`/`neutral`/`hostile`/`at_war`), `secrets` (`hidden_plot`/`covert_bribe`/`illicit_gain`/`secret_vendetta`)
- Optional `forms` & `transitions` (for power tiers / breakthroughs) and `world_events` (`artifact_emergence` | `terrain_shift` | `external_invasion`)

Compile and validate:
```sh
go run ./cmd/wre blueprint compile -in .runs/<id>/blueprint.json -out .runs/<id>/pack.json
```
If validation returns an error, fix the reported field in `blueprint.json` and re-run until `{"ok": true}` is returned.

### Step 2: Run Stage 1 Simulation (Zero External API Key Needed)

Use the built-in generic rulebook (`-decider rulebook -rulebook builtin`). Optionally pass `-rulebook-overlay overlay.json` if the story needs bespoke rules or stance weights:
```sh
go run ./cmd/wre -offline-run -pack .runs/<id>/pack.json \
  -decider rulebook -rulebook builtin -seed 42 \
  -steps 2880 -pace 0s -quiet \
  -out-state .runs/<id>/stage1_state.json \
  -out-events .runs/<id>/stage1_events.json
```
*(96 ticks = 1 day; 2,880 ticks = 30 days; 8,640 ticks = 90 days.)*

Inspect Stage 1 results & drama density:
```sh
go run ./cmd/wre chronicle \
  -events-file .runs/<id>/stage1_events.json \
  -state-file .runs/<id>/stage1_state.json \
  -format md
```
If `Drama Density` is too quiet (few arcs/conflicts), adjust initial `trust`, `grudges`, `secrets`, `diplomacy`, or `world_events` and re-run.

### Step 3: Optional Multi-Volume Stage Injection & Resume

Between volumes, inject objective external turning points (new world event, newly arriving named character, newly planted secret/grudge, diplomatic rupture, or updated ambition) in a conservation-safe `beat2.json`:
```json
{
  "label": "volume_2_turning_point",
  "spawn_actors": [
    {
      "id": "envoy_zhou",
      "name": "周钦差",
      "role": "envoy",
      "location": "palace",
      "tags": ["resident", "elder_oversight", "enforcer"],
      "personality": "stern inquisitive",
      "money": 120,
      "items": {"ration": 300, "drinking_water": 200}
    }
  ],
  "trigger_world_events": [
    {
      "district_id": "palace",
      "inline_spec": {
        "id": "ancient_vault",
        "name": "禁地秘匣现世",
        "category": "artifact_emergence",
        "duration_ticks": 480,
        "intensity": 85,
        "reward_item": "jade_edict",
        "reward_qty": 1,
        "reward_coins": 80
      }
    }
  ]
}
```
Apply the patch and resume offline:
```sh
go run ./cmd/wre inject \
  -state-in .runs/<id>/stage1_state.json \
  -patch .runs/<id>/beat2.json \
  -out-state .runs/<id>/stage1_patched.json \
  -out-events .runs/<id>/beat2_events.json

go run ./cmd/wre -offline-run -state-in .runs/<id>/stage1_patched.json \
  -decider rulebook -rulebook builtin -seed 43 \
  -steps 2880 -pace 0s -quiet \
  -out-state .runs/<id>/stage2_state.json \
  -out-events .runs/<id>/stage2_events.json
```

### Step 4: Merge Chronicles & Drill Down into Historical Evidence

Generate the full multi-stage chronicle index:
```sh
go run ./cmd/wre chronicle \
  -events-file .runs/<id>/stage1_events.json,.runs/<id>/beat2_events.json,.runs/<id>/stage2_events.json \
  -state-file .runs/<id>/stage2_state.json \
  -format md -out .runs/<id>/chronicle_raw.md
```
Then drill down into key protagonists, rivalries, and high-salience arcs using `wre query` (noise-filtered by default):
```sh
# One protagonist's life trajectory
go run ./cmd/wre query -events-file .runs/<id>/stage1_events.json,.runs/<id>/beat2_events.json,.runs/<id>/stage2_events.json -actor <actor_id>

# Bilateral history between two rivals or allies
go run ./cmd/wre query -events-file ... -between <actorA>,<actorB>

# Full causal chain of a major arc
go run ./cmd/wre query -events-file ... -arc <root_event_id>
```

### Step 5: Write the Detailed Chronicle (Grounded Output Format)

Compose the final chronicle in the novel's narrative voice with four sections:
1. **纪年总纲（Chronological Annals）**: Organized by year/season/day, summarizing each major arc with `[Day X, #EventID]`.
2. **重大事件本末（Major Event Chronicles）**: Deep narrative reconstruction of top arcs (causes, rumors/relays, turning points, aftermath).
3. **势力与外交兴衰志（Factions & Diplomacy）**: Territory shifts, alliances, wars, treaties, and leadership changes.
4. **核心人物列传（Protagonist Biographies）**: Personal ambitions, grudges, secrets held/exposed, relationships, and final fate.

**Strict Grounding Rule**: Never claim a character died, betrayed, allied, or achieved a goal unless backed by queried events or the final state snapshot. Clearly separate literary atmosphere from factual citations.


---

## Universal Golden Finger Module (`golden_fingers`)

Declare any web-novel golden finger purely via JSON inside `blueprint.json` (`"golden_fingers": [...]`) or mid-story via `wre inject` (`"add_golden_fingers": [...]`, `"add_gf_quests": [...]`). All rewards and redemptions are automatically tracked in `LedgerAudit` for strict conservation.

### Composable Building Blocks (`sim.GoldenFingerSpec`)

1. **Binding**: `owner_actor_id` (soul-bound), `granted_by_tags`, or `granted_by_items` (lootable/transferable ring or pendant).
2. **`passive`**:
   - `skill_xp_multiplier_pct` (e.g., `250` = 2.5× proficiency panel)
   - `combat_bonus` (flat combat empowerment)
   - `disguise_power_tier` (e.g., `1` = 扮猪吃虎: outsiders perceive lower tier/combat power)
   - `daily_pocket_yield` (随身空间 / 灵泉农场 daily items/coins/currencies)
   - `rescue_below_health` / `rescue_heal` / `rescue_cooldown_ticks` / `rescue_name` (戒指老爷爷护主保命)
3. **`triggers`** (reactive & scheduled hooks):
   - Daily sign-in: `on_events: ["day_start"]`
   - Landmark check-in: `on_events: ["moved"]`, `once_per_district: true`, optional `required_district_features`
   - Emotion/shock harvest: `match_role: "witnessed_by_others"`, `scale_by_observers: true`, `reward.currencies`
   - Kill devouring: `on_events: ["actor_died"]`, `match_role: "self_killer"`, `devour_skill: true`, `devour_attr_points: 2`
   - Mentee/kin rebate: `on_events: ["transition_succeeded", "skill_leveled"]`, `match_role: "mentee_or_kin"`
4. **`shop`** (系统商城 / 万界兑换 / 万物合成): executed autonomously via `gf_exchange` (`cost_currency`/`cost_coins`/`cost_items` -> `reward`, optional `stock_limit` & `cooldown_ticks`).
5. **`quests`** (任务系统 / 无限流主支线): `kind` in `reach_form | reach_skill | eliminate_actor | acquire_item | visit_district | accumulate_currency`, with `reward`, optional `deadline_tick`, and `failure_penalty` (`health_loss`, `coins_loss`, `currency_loss`, `lethal`).
6. **`foresights`** (重生先知 / 剧透记忆) & **`simulator`** (人生模拟器 via `gf_simulate`): agents proactively position ahead of predicted `opportunity` / flee `danger`, and invoke branch simulations for foresight + retained attributes/XP.
7. **`exposure_risk_permille`**: overt miracles in front of witnesses can seed a discoverable `forbidden_hoard` secret!

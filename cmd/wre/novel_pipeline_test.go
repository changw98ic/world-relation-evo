package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
)

func TestEndToEndNovelOutlineCLISkillPipeline(t *testing.T) {
	dir := t.TempDir()
	bpPath := filepath.Join(dir, "blueprint.json")
	packPath := filepath.Join(dir, "pack.json")
	overlayPath := filepath.Join(dir, "overlay.json")
	s1State := filepath.Join(dir, "stage1_state.json")
	s1Events := filepath.Join(dir, "stage1_events.json")
	patchPath := filepath.Join(dir, "beat2.json")
	s1Patched := filepath.Join(dir, "stage1_patched.json")
	beat2Events := filepath.Join(dir, "beat2_events.json")
	s2State := filepath.Join(dir, "stage2_state.json")
	s2Events := filepath.Join(dir, "stage2_events.json")
	chronicleMD := filepath.Join(dir, "chronicle.md")

	// 1. Compact LLM-authored Blueprint (~90 lines JSON for a court-intrigue novel outline).
	bpJSON := `{
  "id": "imperial-shadow",
  "name": "北燕暗潮",
  "genre": "historical_court",
  "summary": "北燕朝堂太子党与宰相党暗斗，密探与御史穿梭于皇城与西市之间。",
  "currency_name": "银两",
  "actor_noun": "朝臣",
  "food_item_id": "court_meal",
  "food_item_name": "官膳",
  "water_item_id": "tea_water",
  "water_item_name": "清茶",
  "supply_days": 120,
  "districts": [
    {"id": "palace", "name": "太极宫", "features": ["public_forum", "authority_seat"], "controlling_faction": "crown_party"},
    {"id": "west_market", "name": "西市坊", "features": ["market", "public_forum"], "controlling_faction": "chancellor_party"}
  ],
  "factions": [
    {"id": "crown_party", "name": "东宫党", "district": "palace", "leader_id": "prince_li"},
    {"id": "chancellor_party", "name": "相府党", "district": "west_market", "leader_id": "chancellor_wei"}
  ],
  "actors": [
    {
      "id": "prince_li", "name": "太子李承", "role": "prince", "location": "palace",
      "faction": "crown_party", "tags": ["ruler", "fighter"],
      "personality": "proud righteous ambitious", "money": 120,
      "trust": {"chancellor_wei": -4, "censor_han": 3},
      "ambitions": [{"kind": "expose_corrupt", "target_id": "chancellor_wei"}]
    },
    {
      "id": "chancellor_wei", "name": "丞相魏严", "role": "chancellor", "location": "palace",
      "faction": "chancellor_party", "tags": ["ruler"],
      "personality": "cunning ruthless calculating", "money": 180,
      "trust": {"prince_li": -4, "spy_mo": 2}
    },
    {
      "id": "censor_han", "name": "御史韩铮", "role": "censor", "location": "palace",
      "faction": "crown_party", "tags": ["elder_oversight", "enforcer"],
      "personality": "stern dutiful inquisitive", "money": 80,
      "trust": {"prince_li": 3, "chancellor_wei": -2}
    },
    {
      "id": "spy_mo", "name": "密探墨影", "role": "spy", "location": "palace",
      "faction": "chancellor_party", "tags": ["outlaw", "fighter"],
      "personality": "cunning opportunistic reckless", "money": 70,
      "trust": {"chancellor_wei": 2, "prince_li": -3}
    }
  ],
  "grudges": [
    {"aggrieved_id": "prince_li", "offender_id": "chancellor_wei", "cause": "east_palace_poisoning", "intensity": 70}
  ],
  "diplomacy": [
    {"faction_a": "crown_party", "faction_b": "chancellor_party", "stance": "hostile", "relation_score": -70, "reason": "heir_succession"}
  ],
  "secrets": [
    {"owner_id": "chancellor_wei", "kind": "hidden_plot", "subject_id": "prince_li", "severity": 85, "known_by": ["chancellor_wei", "censor_han"]}
  ]
}`
	if err := os.WriteFile(bpPath, []byte(bpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Lightweight overlay adjusting intrigue weight.
	overlayJSON := `{
  "personality_stance_weights": {
    "stern": {"秉公处置": 3.0}
  }
}`
	if err := os.WriteFile(overlayPath, []byte(overlayJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	runCLI := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if err := runArgs(args, &stdout, &stderr); err != nil {
			t.Fatalf("wre %v failed: %v\nstderr=%s\nstdout=%s", args, err, stderr.String(), stdout.String())
		}
		return stdout.String()
	}

	// 2. Compile blueprint & validate pack.
	compileOut := runCLI("blueprint", "compile", "-in", bpPath, "-out", packPath)
	if !strings.Contains(compileOut, `"ok": true`) {
		t.Fatalf("unexpected blueprint compile output: %s", compileOut)
	}
	valOut := runCLI("pack", "validate", "-pack", packPath)
	if !strings.Contains(valOut, `"conserved": true`) {
		t.Fatalf("unexpected pack validate output: %s", valOut)
	}

	// 3. Stage 1 offline run using built-in BaseRulebook + lightweight overlay (2 days = 192 ticks).
	runCLI(
		"-offline-run", "-pack", packPath,
		"-seed", "42", "-steps", "192", "-pace", "0s", "-quiet",
		"-out-state", s1State, "-out-events", s1Events,
	)

	// 4. Stage 2 director patch: inject an arriving imperial envoy + trigger secret seal emergence.
	patchJSON := `{
  "label": "volume_2_envoy_and_seal",
  "spawn_actors": [
    {
      "id": "envoy_zhou",
      "name": "钦差周肃",
      "role": "envoy",
      "location": "palace",
      "tags": ["resident", "elder_oversight", "enforcer"],
      "personality": "righteous stern inquisitive",
      "money": 100,
      "items": {"court_meal": 200, "tea_water": 150}
    }
  ],
  "trigger_world_events": [
    {
      "district_id": "palace",
      "inline_spec": {
        "id": "imperial_jade_seal",
        "name": "先帝遗诏玉匣现世",
        "category": "artifact_emergence",
        "duration_ticks": 384,
        "intensity": 85,
        "reward_item": "jade_edict",
        "reward_qty": 1,
        "reward_coins": 50
      }
    }
  ]
}`
	if err := os.WriteFile(patchPath, []byte(patchJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	injectOut := runCLI("inject", "-state-in", s1State, "-patch", patchPath, "-out-state", s1Patched, "-out-events", beat2Events)
	if !strings.Contains(injectOut, `"ok": true`) {
		t.Fatalf("unexpected inject output: %s", injectOut)
	}

	// 5. Stage 2 offline resume from patched snapshot (2 more days = 192 ticks).
	runCLI(
		"-offline-run", "-state-in", s1Patched,
		"-steps", "192", "-pace", "0s", "-quiet",
		"-out-state", s2State, "-out-events", s2Events,
	)

	// 6. Merge multi-stage events into Chronicle (JSON + Markdown).
	mergedEventsArg := strings.Join([]string{s1Events, beat2Events, s2Events}, ",")
	chronJSON := runCLI("chronicle", "-events-file", mergedEventsArg, "-state-file", s2State, "-format", "json")
	var rep runtrace.ChronicleReport
	if err := json.Unmarshal([]byte(chronJSON), &rep); err != nil {
		t.Fatalf("decode merged chronicle json: %v", err)
	}
	if !rep.HasFinalState || !rep.LedgerConserved {
		t.Fatalf("expected conserved final chronicle, got has_final=%v conserved=%v err=%q", rep.HasFinalState, rep.LedgerConserved, rep.LedgerError)
	}
	if rep.WorldID != "imperial-shadow" || rep.TotalTicks != 384 {
		t.Fatalf("expected world imperial-shadow at tick 384, got world=%q tick=%d", rep.WorldID, rep.TotalTicks)
	}
	if rep.Drama.ArcCount == 0 || rep.Drama.WorldEvents == 0 {
		t.Fatalf("expected meaningful narrative arcs and world events, got drama=%+v", rep.Drama)
	}
	foundEnvoy := false
	for _, bio := range rep.Biographies {
		if bio.ID == "envoy_zhou" {
			foundEnvoy = true
			break
		}
	}
	if !foundEnvoy {
		t.Fatalf("spawned volume-2 actor envoy_zhou missing from final biographies")
	}

	runCLI("chronicle", "-events-file", mergedEventsArg, "-state-file", s2State, "-format", "md", "-out", chronicleMD)
	mdRaw, err := os.ReadFile(chronicleMD)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mdRaw), "## Narrative Arcs (Clustered by RootEventID)") {
		t.Fatalf("expected Narrative Arcs section in rendered chronicle markdown:\n%s", string(mdRaw))
	}

	// 7. Historical drill-down via `wre query` (actor trajectory & arc causal chain).
	qActorJSON := runCLI("query", "-events-file", mergedEventsArg, "-actor", "envoy_zhou", "-format", "json")
	var qActor runtrace.QueryResult
	if err := json.Unmarshal([]byte(qActorJSON), &qActor); err != nil {
		t.Fatalf("decode actor query json: %v", err)
	}
	if qActor.TotalMatched == 0 {
		t.Fatalf("expected query -actor envoy_zhou to return at least entry event")
	}
	firstArcID := rep.Arcs[0].RootEventID
	qArcMD := runCLI("query", "-events-file", mergedEventsArg, "-arc", jsonInt64String(firstArcID), "-format", "md")
	if !strings.Contains(qArcMD, "# Historical Query Result") {
		t.Fatalf("unexpected query -arc markdown output:\n%s", qArcMD)
	}
}

func jsonInt64String(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

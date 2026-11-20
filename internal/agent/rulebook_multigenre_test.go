package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func buildGenericTestRulebook(worldID string) *agent.InteractionRulebook {
	return &agent.InteractionRulebook{
		WorldID: worldID,
		PersonalityStanceWeights: map[string]map[string]float64{
			"ambitious":   {"趁机谋利": 2.0, "借力拉拢": 1.4},
			"righteous":   {"秉公处置": 2.2, "强硬反制": 1.4},
			"cautious":    {"避险止损": 2.0, "隐忍观察": 1.5},
			"calculating": {"趁机谋利": 1.8, "借力拉拢": 1.5},
			"gregarious":  {"积极呼应": 1.8, "借力拉拢": 1.6},
		},
	}
}

func TestRulebookMultiGenreLiveBenchReplayAndChronicle(t *testing.T) {
	packs := []string{
		"xianxia_sect.json",
		"mortal_watershed.json",
		"wasteland_sector.json",
	}
	const ticks = 96
	ctx := context.Background()

	for _, file := range packs {
		t.Run(file, func(t *testing.T) {
			p, err := pack.LoadFile(filepath.Join("..", "..", "packs", file))
			if err != nil {
				t.Fatalf("LoadFile(%s): %v", file, err)
			}
			w, err := p.Instantiate("rb-bench-" + strings.TrimSuffix(file, ".json"))
			if err != nil {
				t.Fatalf("Instantiate(%s): %v", file, err)
			}
			ckpt, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}

			sel := agent.NewRulebookSelector(buildGenericTestRulebook(w.ID), nil)
			sel.Seed = 42
			decider := agent.Pipeline{Selector: sel}

			logs := make([]sim.TickDecisionLog, 0, ticks)
			var allEvents []sim.Event
			for step := 0; step < ticks; step++ {
				evs, log, err := w.StepRecorded(ctx, decider)
				if err != nil {
					t.Fatalf("%s rulebook step %d failed: %v", file, step, err)
				}
				logs = append(logs, log)
				allEvents = append(allEvents, evs...)
			}

			// 1. 100% ledger conservation
			cons := sim.InspectWorldConservation(w)
			if !cons.LedgerConserved {
				t.Fatalf("%s rulebook conservation failed: %s", file, cons.LedgerError)
			}

			// 2. Byte-identical deterministic replay
			replayed, _, err := sim.ReplayFromCheckpoint(ckpt, logs, ticks)
			if err != nil {
				t.Fatalf("%s replay failed: %v", file, err)
			}
			b1, _ := json.Marshal(w)
			b2, _ := json.Marshal(replayed)
			if !bytes.Equal(b1, b2) {
				t.Fatalf("%s rulebook replay diverged from live state", file)
			}

			// 3. Chronicle generation succeeds cleanly on real multi-genre output
			rep := runtrace.BuildChronicle(allEvents, w, 100)
			md := runtrace.RenderChronicleMarkdown(rep)
			if rep.WorldID != w.ID || !rep.LedgerConserved || !strings.Contains(md, "# World Chronicle: "+w.ID) {
				t.Fatalf("%s chronicle validation failed: rep=%+v", file, rep)
			}
		})
	}
}

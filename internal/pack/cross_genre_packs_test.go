package pack_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/pack"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestCrossGenreWorldPacksFormsSkillsAndConservation(t *testing.T) {
	cases := []struct {
		file            string
		wantFastingForm string
		wantHighIntake  string
		wantMinSkills   int
	}{
		{
			file:            "xianxia_sect.json",
			wantFastingForm: "foundation",
			wantHighIntake:  "",
			wantMinSkills:   8,
		},
		{
			file:            "wasteland_sector.json",
			wantFastingForm: "combat_android",
			wantHighIntake:  "bio_mutant",
			wantMinSkills:   5,
		},
		{
			file:            "mortal_watershed.json",
			wantFastingForm: "",
			wantHighIntake:  "martial_master",
			wantMinSkills:   5,
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "packs", tc.file)
			p, err := pack.LoadFile(path)
			if err != nil {
				t.Fatalf("LoadFile(%s) failed: %v", tc.file, err)
			}
			w, err := p.Instantiate("")
			if err != nil {
				t.Fatalf("Instantiate(%s) failed: %v", tc.file, err)
			}
			if len(w.Catalog.Skills) < tc.wantMinSkills {
				t.Fatalf("%s expected >=%d independent skills, got %d", tc.file, tc.wantMinSkills, len(w.Catalog.Skills))
			}
			if tc.wantFastingForm != "" {
				f, ok := w.Catalog.Forms[tc.wantFastingForm]
				if !ok || !f.IsHungerDisabled() {
					t.Fatalf("%s expected fasting form %q with IsHungerDisabled=true, got %+v (ok=%v)", tc.file, tc.wantFastingForm, f, ok)
				}
			}
			if tc.wantHighIntake != "" {
				f, ok := w.Catalog.Forms[tc.wantHighIntake]
				if !ok || f.EffectiveDailyIntakePct() <= 150 {
					t.Fatalf("%s expected high-intake form %q (>150%%), got %d%% (ok=%v)", tc.file, tc.wantHighIntake, f.EffectiveDailyIntakePct(), ok)
				}
			}

			// Step 48 ticks (half day) using rules decider and verify zero conservation violations.
			pipe := agent.Offline()
			for step := 0; step < 48; step++ {
				if _, err := w.Step(context.Background(), pipe); err != nil {
					t.Fatalf("%s Step tick %d failed: %v", tc.file, step, err)
				}
			}
			report := sim.InspectWorldConservation(w)
			if !report.LedgerConserved {
				t.Fatalf("%s conservation failed after 48 ticks: %s (%+v)", tc.file, report.LedgerError, report)
			}
		})
	}
}

func TestCrossGenreMultiDayReplayDeterminism(t *testing.T) {
	p, err := pack.LoadFile(filepath.Join("..", "..", "packs", "mortal_watershed.json"))
	if err != nil {
		t.Fatal(err)
	}
	w1, err := p.Instantiate("replay-det-mortal")
	if err != nil {
		t.Fatal(err)
	}
	ckpt, err := json.Marshal(w1)
	if err != nil {
		t.Fatal(err)
	}
	pipe := agent.Offline()
	ctx := context.Background()
	const steps = 192
	logs := make([]sim.TickDecisionLog, 0, steps)
	for tick := 0; tick < steps; tick++ {
		_, log, err := w1.StepRecorded(ctx, pipe)
		if err != nil {
			t.Fatalf("w1 step %d: %v", tick, err)
		}
		logs = append(logs, log)
	}
	w2, _, err := sim.ReplayFromCheckpoint(ckpt, logs, steps)
	if err != nil {
		t.Fatalf("ReplayFromCheckpoint failed: %v", err)
	}
	j1, _ := json.Marshal(w1)
	j2, _ := json.Marshal(w2)
	if !bytes.Equal(j1, j2) {
		t.Fatalf("replay diverged after %d ticks", steps)
	}
}

package agent

import (
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestOptionsRespectsSingleSourceCheckActionAndNoDuplicatePracticeSkill(t *testing.T) {
	w := sim.Village("p1-agent-parity")
	a := &w.Actors[0]
	b := &w.Actors[1]
	a.Location = "square"
	b.Location = "square"
	a.InsideHouse = ""
	b.InsideHouse = ""
	a.Energy = 60
	a.Health = sim.MaxHealth
	a.Hunger = 3
	a.Money = 20
	a.Items["food"] = 4

	// Configure MinFoodTier = 2 so tier-1 food is unacceptable.
	w.Catalog.Forms = map[string]sim.FormSpec{
		"immortal": {ID: "immortal", Name: "Immortal", PowerTier: 3, MinFoodTier: 2},
	}
	a.FormID = "immortal"
	food := w.Catalog.Items["food"]
	food.Tier = 1
	w.Catalog.Items["food"] = food

	// Configure a skill + matching master_skill ambition to verify no duplicate practice_skill option.
	w.Catalog.Skills = map[string]sim.SkillSpec{
		"swordsmanship": {ID: "swordsmanship", Name: "Swordsmanship", MaxLevel: 5},
	}
	a.Skills = map[string]int{"swordsmanship": 1}
	a.Ambitions = []sim.ActorAmbition{{ID: "amb1", Kind: "master_skill", TargetID: "swordsmanship", TargetValue: 5}}

	// Put actor b to sleep so bribe against sleeping enforcer is blocked.
	b.Task = &sim.Task{Action: sim.Action{Kind: "sleep"}, Remaining: 4}

	p := w.View(a.ID)
	opts := Options(p)

	practiceCount := 0
	for _, opt := range opts {
		if opt.Action.Kind == "eat" && opt.Action.Item == "food" {
			t.Fatalf("Options emitted unacceptable eat food for high-tier form: %+v", opt)
		}
		if opt.Action.Kind == "bribe" && opt.Action.TargetID == b.ID {
			t.Fatalf("Options emitted bribe against sleeping target: %+v", opt)
		}
		if opt.Action.Kind == "practice_skill" && opt.Action.SkillID == "swordsmanship" {
			practiceCount++
		}
		if res := p.CheckAction(opt.Action); !res.Allowed {
			t.Fatalf("Options emitted action blocked by Perception.CheckAction: opt=%+v reason=%q", opt, res.BlockReason)
		}
	}
	if practiceCount != 1 {
		t.Fatalf("expected exactly 1 practice_skill option for swordsmanship, got %d", practiceCount)
	}
}

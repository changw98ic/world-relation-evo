package agent

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func addDiplomacyOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	for _, myFac := range p.LedFactions {
		addFactionDiplomacyOptions(p, myFac, allowed, add)
	}
}

func addFactionDiplomacyOptions(p sim.Perception, myFac string, allowed map[string]bool, add func(string, sim.Action)) {
	a := p.Self
	relByOther := map[string]sim.FactionDiplomacy{}
	for _, d := range p.FactionDiplomacy {
		if d.FactionA == myFac {
			relByOther[d.FactionB] = d
		} else if d.FactionB == myFac {
			relByOther[d.FactionA] = d
		}
	}
	hasInvasion := false
	for _, we := range p.ActiveWorldEvents {
		if we.Category == "external_invasion" {
			hasInvasion = true
			break
		}
	}
	for _, f := range p.Factions {
		if f.ID == myFac {
			continue
		}
		rel, hasRel := relByOther[f.ID]
		stance := "neutral"
		if hasRel {
			stance = rel.Stance
		}
		if allowed["make_peace"] && stance == "at_war" && rel.PeaceProposer != myFac && a.Energy >= 4 {
			add(fmt.Sprintf("Negotiate peace treaty between %s and %s (-4 energy)", myFac, f.ID),
				sim.Action{Kind: "make_peace", FactionID: myFac, TargetID: f.ID})
		}
		if allowed["propose_alliance"] && stance != "at_war" && (stance != "allied" || rel.TreatyUntilTick > 0 && p.Tick >= rel.TreatyUntilTick) && rel.AllianceProposer != myFac && a.Energy >= 5 {
			if hasInvasion || !hasRel || rel.RelationScore >= -40 {
				add(fmt.Sprintf("Propose alliance between %s and %s (-5 energy)", myFac, f.ID),
					sim.Action{Kind: "propose_alliance", FactionID: myFac, TargetID: f.ID})
			}
		}
		if allowed["declare_war"] && stance != "at_war" && a.Energy >= 6 && p.Tick >= rel.TreatyUntilTick {
			add(fmt.Sprintf("Declare faction war: %s against %s (-6 energy)", myFac, f.ID),
				sim.Action{Kind: "declare_war", FactionID: myFac, TargetID: f.ID})
		}
	}
}

func planDiplomacyAction(p sim.Perception, consider func(sim.Action) bool) bool {
	for _, myFac := range p.LedFactions {
		for _, rel := range p.FactionDiplomacy {
			other := rel.FactionA
			if other == myFac {
				other = rel.FactionB
			} else if rel.FactionB != myFac {
				continue
			}
			if rel.PeaceProposer != myFac && rel.Stance == "at_war" &&
				(rel.PeaceProposer != "" || p.Tick-rel.WarStartedTick >= 3*sim.TicksPerDay || p.Self.Health < 65) &&
				consider(sim.Action{Kind: "make_peace", FactionID: myFac, TargetID: other}) {
				return true
			}
			if rel.AllianceProposer != "" && rel.AllianceProposer != myFac &&
				consider(sim.Action{Kind: "propose_alliance", FactionID: myFac, TargetID: other}) {
				return true
			}
			if rel.Stance == "hostile" && rel.RelationScore <= -60 && p.Self.Health >= 75 &&
				p.Tick >= rel.TreatyUntilTick && consider(sim.Action{Kind: "declare_war", FactionID: myFac, TargetID: other}) {
				return true
			}
		}
	}
	return false
}

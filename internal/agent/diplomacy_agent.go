package agent

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func canActDiplomatically(a sim.Actor) (string, bool) {
	if len(a.Guilds) == 0 {
		return "", false
	}
	facID := a.Guilds[0]
	if sim.ActorHasTag(a.Tags, "ruler") || sim.ActorHasTag(a.Tags, "elder_oversight") ||
		sim.ActorHasTag(a.Tags, "commander") || sim.ActorHasTag(a.Tags, "enforcer") ||
		a.GuildLevels[facID] >= 2 {
		return facID, true
	}
	return "", false
}

func addDiplomacyOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	a := p.Self
	myFac, ok := canActDiplomatically(a)
	if !ok {
		return
	}
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
		if allowed["make_peace"] && stance == "at_war" && a.Energy >= 4 {
			add(fmt.Sprintf("Negotiate peace treaty between %s and %s (-4 energy)", myFac, f.ID),
				sim.Action{Kind: "make_peace", FactionID: myFac, TargetID: f.ID})
		}
		if allowed["propose_alliance"] && stance != "at_war" && stance != "allied" && a.Energy >= 5 {
			if hasInvasion || !hasRel || rel.RelationScore >= -40 {
				add(fmt.Sprintf("Propose alliance between %s and %s (-5 energy)", myFac, f.ID),
					sim.Action{Kind: "propose_alliance", FactionID: myFac, TargetID: f.ID})
			}
		}
		if allowed["declare_war"] && stance == "hostile" && a.Energy >= 6 && p.Tick >= rel.TreatyUntilTick {
			add(fmt.Sprintf("Declare faction war: %s against %s (-6 energy)", myFac, f.ID),
				sim.Action{Kind: "declare_war", FactionID: myFac, TargetID: f.ID})
		}
	}
}

func planDiplomacyAction(p sim.Perception, consider func(sim.Action) bool) bool {
	a := p.Self
	myFac, ok := canActDiplomatically(a)
	if !ok || a.Energy < 12 {
		return false
	}
	hasInvasion := false
	for _, we := range p.ActiveWorldEvents {
		if we.Category == "external_invasion" {
			hasInvasion = true
			break
		}
	}
	for _, rel := range p.FactionDiplomacy {
		other := ""
		if rel.FactionA == myFac {
			other = rel.FactionB
		} else if rel.FactionB == myFac {
			other = rel.FactionA
		} else {
			continue
		}
		if rel.Stance == "at_war" && (p.Tick-rel.WarStartedTick >= 3*sim.TicksPerDay || a.Health < 65) {
			if consider(sim.Action{Kind: "make_peace", FactionID: myFac, TargetID: other}) {
				return true
			}
		}
		if rel.Stance == "hostile" && rel.RelationScore <= -60 && a.Health >= 75 && a.Energy >= 25 && p.Tick >= rel.TreatyUntilTick {
			if consider(sim.Action{Kind: "declare_war", FactionID: myFac, TargetID: other}) {
				return true
			}
		}
	}
	if hasInvasion {
		for _, f := range p.Factions {
			if f.ID == myFac {
				continue
			}
			if consider(sim.Action{Kind: "propose_alliance", FactionID: myFac, TargetID: f.ID}) {
				return true
			}
		}
	}
	return false
}

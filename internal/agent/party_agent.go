package agent

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func actorActiveParty(p sim.Perception) *sim.Party {
	for i := range p.Parties {
		if sim.ActorHasTag(p.Parties[i].MemberIDs, p.Self.ID) {
			return &p.Parties[i]
		}
	}
	return nil
}

func addPartyOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	a := p.Self
	myParty := actorActiveParty(p)
	if myParty == nil {
		if allowed["form_party"] && a.InsideHouse == "" && a.Energy >= 3 && a.Health >= 60 {
			purpose := ""
			targetDist := a.Location
			for _, we := range p.ActiveWorldEvents {
				if we.Category == "artifact_emergence" && we.ClaimedBy == "" {
					purpose = "relic_hunt"
					targetDist = we.DistrictID
					break
				}
			}
			if purpose == "" && p.DistrictState != nil {
				for _, act := range p.DistrictState.ThreatActivity {
					if act >= 20 {
						purpose = "hunt"
						targetDist = a.Location
						break
					}
				}
			}
			if purpose != "" {
				for _, nb := range p.Nearby {
					if nb.Jailed || nb.Sleeping || a.Trust[nb.ID] < 0 {
						continue
					}
					add(fmt.Sprintf("Form %s party with %s targeting %s (-3 energy)", purpose, nb.Name, targetDist),
						sim.Action{Kind: "form_party", TargetID: nb.ID, Item: purpose, Location: targetDist})
					break
				}
			}
		}
		if allowed["join_party"] && a.Energy >= 2 && a.Health >= 55 {
			for _, party := range p.Parties {
				if len(party.MemberIDs) >= 6 || a.Trust[party.LeaderID] < -1 {
					continue
				}
				add(fmt.Sprintf("Join %s (%s, %d members)", party.Name, party.Purpose, len(party.MemberIDs)),
					sim.Action{Kind: "join_party", TargetID: party.ID})
			}
		}
		return
	}

	if allowed["leave_party"] {
		add(fmt.Sprintf("Leave %s", myParty.Name), sim.Action{Kind: "leave_party", TargetID: myParty.ID})
	}
	if allowed["betray_party"] && a.Energy >= 6 && a.Health >= 50 {
		traits := ExtractPersonalityTraits(a.Personality)
		opportunistic := sim.ActorHasTag(traits, "ruthless") || sim.ActorHasTag(traits, "calculating") ||
			sim.ActorHasTag(traits, "cunning") || sim.ActorHasTag(traits, "opportunistic") ||
			p.Catalog.IsOutlawTags(a.IdentityTags())
		for _, nb := range p.Nearby {
			if nb.Jailed || !sim.ActorHasTag(myParty.MemberIDs, nb.ID) {
				continue
			}
			if a.Trust[nb.ID] <= 0 || opportunistic {
				add(fmt.Sprintf("Betray party and ambush %s (-6 energy)", nb.Name),
					sim.Action{Kind: "betray_party", TargetID: nb.ID})
			}
		}
	}
}

func planPartyAction(p sim.Perception, consider func(sim.Action) bool) bool {
	a := p.Self
	if a.Health < 60 || a.Energy < 18 {
		return false
	}
	myParty := actorActiveParty(p)
	if myParty == nil {
		// Join an existing trusted co-located party first.
		for _, party := range p.Parties {
			if len(party.MemberIDs) < 6 && a.Trust[party.LeaderID] >= 0 {
				if consider(sim.Action{Kind: "join_party", TargetID: party.ID}) {
					return true
				}
			}
		}
		// Form a party with a trusted co-located ally if an artifact event or high threat exists.
		purpose := ""
		targetDist := a.Location
		for _, we := range p.ActiveWorldEvents {
			if we.Category == "artifact_emergence" && we.ClaimedBy == "" {
				purpose = "relic_hunt"
				targetDist = we.DistrictID
				break
			}
		}
		if purpose == "" && p.DistrictState != nil {
			for _, act := range p.DistrictState.ThreatActivity {
				if act >= 25 {
					purpose = "hunt"
					targetDist = a.Location
					break
				}
			}
		}
		if purpose != "" && a.InsideHouse == "" {
			for _, nb := range p.Nearby {
				if nb.Jailed || nb.Sleeping || a.Trust[nb.ID] < 0 {
					continue
				}
				if consider(sim.Action{Kind: "form_party", TargetID: nb.ID, Item: purpose, Location: targetDist}) {
					return true
				}
			}
		}
		return false
	}

	// Follow party target district for coordinated expedition/escort.
	if myParty.TargetDistrict != "" && myParty.TargetDistrict != a.Location && a.Energy >= 30 {
		if step, ok := stepTowardDistrict(p, myParty.TargetDistrict, 0); ok && consider(step) {
			return true
		}
	}
	return false
}

package agent

import (
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func addAmbitionOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	a := p.Self
	activeCount := 0
	for _, amb := range a.Ambitions {
		if !amb.Completed {
			activeCount++
		}
	}
	if allowed["set_ambition"] && activeCount < 2 {
		add("Adopt long-term wealth accumulation ambition", sim.Action{Kind: "set_ambition", Item: "accumulate_wealth"})
	}

	nearbySet := make(map[string]bool, len(p.Nearby))
	for _, nb := range p.Nearby {
		nearbySet[nb.ID] = true
	}

	if allowed["investigate_actor"] && a.Energy >= 5 {
		traits := ExtractPersonalityTraits(a.Personality)
		inquisitive := p.Catalog.IsOversightTags(a.IdentityTags()) ||
			p.Catalog.IsEnforcementTags(a.IdentityTags()) ||
			sim.ActorHasTag(traits, "inquisitive") || sim.ActorHasTag(traits, "skeptical")
		for _, nb := range p.Nearby {
			if nb.Jailed || nb.Sleeping {
				continue
			}
			if inquisitive || a.Trust[nb.ID] < 0 {
				add(fmt.Sprintf("Investigate %s for hidden secrets (-5 energy)", nb.Name),
					sim.Action{Kind: "investigate_actor", TargetID: nb.ID})
			}
		}
	}

	for _, s := range p.KnownSecrets {
		if s.Exposed || s.OwnerID == a.ID || !sim.ActorHasTag(s.KnownBy, a.ID) {
			continue
		}
		if allowed["expose_secret"] && a.Energy >= 4 {
			add(fmt.Sprintf("Publicly expose %s's secret (%s, severity %d) (-4 energy)", s.OwnerID, s.Kind, s.Severity),
				sim.Action{Kind: "expose_secret", TargetID: s.OwnerID, Item: s.ID})
		}
		if allowed["blackmail"] && a.Energy >= 4 && nearbySet[s.OwnerID] {
			add(fmt.Sprintf("Blackmail %s over %s (-4 energy)", s.OwnerID, s.Kind),
				sim.Action{Kind: "blackmail", TargetID: s.OwnerID, Item: s.ID})
		}
	}
}

func planAmbitionAction(p sim.Perception, consider func(sim.Action) bool) bool {
	a := p.Self
	if a.Energy < 12 || a.Health < 55 {
		return false
	}
	traits := ExtractPersonalityTraits(a.Personality)
	isOversight := p.Catalog.IsOversightTags(a.IdentityTags()) || p.Catalog.IsEnforcementTags(a.IdentityTags()) || sim.ActorHasTag(traits, "righteous")
	isOpportunist := sim.ActorHasTag(traits, "calculating") || sim.ActorHasTag(traits, "cunning") ||
		sim.ActorHasTag(traits, "opportunistic") || p.Catalog.IsOutlawTags(a.IdentityTags())

	nearbySet := make(map[string]bool, len(p.Nearby))
	for _, nb := range p.Nearby {
		if !nb.Jailed && !nb.Sleeping {
			nearbySet[nb.ID] = true
		}
	}

	// Act on known unexposed secrets of others.
	for _, s := range p.KnownSecrets {
		if s.Exposed || s.OwnerID == a.ID || !sim.ActorHasTag(s.KnownBy, a.ID) {
			continue
		}
		if isOpportunist && !isOversight && nearbySet[s.OwnerID] {
			if consider(sim.Action{Kind: "blackmail", TargetID: s.OwnerID, Item: s.ID}) {
				return true
			}
		}
		if len(p.Nearby) > 0 && (isOversight || a.Trust[s.OwnerID] < 0) {
			if consider(sim.Action{Kind: "expose_secret", TargetID: s.OwnerID, Item: s.ID}) {
				return true
			}
		}
	}

	// Periodically investigate low-trust co-located suspects.
	if p.Tick%8 == 0 && (isOversight || sim.ActorHasTag(traits, "inquisitive")) {
		for _, nb := range p.Nearby {
			if !nb.Jailed && !nb.Sleeping && a.Trust[nb.ID] < 0 {
				if consider(sim.Action{Kind: "investigate_actor", TargetID: nb.ID}) {
					return true
				}
			}
		}
	}
	return false
}

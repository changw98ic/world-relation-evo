package agent

import (
	"fmt"
	"sort"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func addAmbitionOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	a := p.Self
	activeCount := 0
	hasAmbition := func(kind, targetID string) bool {
		for _, amb := range a.Ambitions {
			if amb.Kind != kind || (targetID != "" && amb.TargetID != targetID) {
				continue
			}
			if !amb.Completed || kind == "master_skill" || kind == "expose_corrupt" || kind == "avenge_blood" {
				return true
			}
		}
		return false
	}
	for _, amb := range a.Ambitions {
		if !amb.Completed {
			activeCount++
		}
	}
	if allowed["set_ambition"] && activeCount < 2 {
		if !hasAmbition("accumulate_wealth", "") {
			add("Adopt long-term wealth accumulation ambition", sim.Action{Kind: "set_ambition", Item: "accumulate_wealth"})
		}
		for _, targetID := range p.ActiveGrudgeTargets {
			if targetID != "" && !hasAmbition("avenge_blood", targetID) {
				add(fmt.Sprintf("Adopt blood vengeance ambition against %s", targetID),
					sim.Action{Kind: "set_ambition", Item: "avenge_blood", TargetID: targetID})
				break
			}
		}
		if len(p.Catalog.Transitions) > 0 && !hasAmbition("ascend_tier", "") {
			curTier := p.Catalog.ActorPowerTier(a)
			for _, f := range p.Catalog.Forms {
				if f.PowerTier > curTier {
					add(fmt.Sprintf("Adopt ascension ambition to reach tier %d", curTier+1),
						sim.Action{Kind: "set_ambition", Item: "ascend_tier", Quantity: curTier + 1})
					break
				}
			}
		}
		for _, s := range p.KnownSecrets {
			if !s.Exposed && s.HolderStances[a.ID] == "confront" && s.OwnerID != "" && s.OwnerID != a.ID && !hasAmbition("expose_corrupt", s.OwnerID) {
				add(fmt.Sprintf("Adopt ambition to expose %s's wrongdoing", s.OwnerID),
					sim.Action{Kind: "set_ambition", Item: "expose_corrupt", TargetID: s.OwnerID})
				break
			}
		}
		skills := make([]string, 0, len(a.Skills))
		for id := range a.Skills {
			skills = append(skills, id)
		}
		sort.Strings(skills)
		for _, id := range skills {
			if spec, ok := p.Catalog.Skills[id]; ok && a.Skills[id] < spec.EffectiveMaxLevel() && !hasAmbition("master_skill", id) {
				add("Work toward mastery of "+id, sim.Action{Kind: "set_ambition", Item: "master_skill", TargetID: id, Quantity: spec.EffectiveMaxLevel()})
			}
		}
		for _, factionID := range a.Guilds {
			if !sim.ActorHasTag(p.LedFactions, factionID) && !hasAmbition("seize_leadership", factionID) {
				add(fmt.Sprintf("Adopt ambition to win leadership of %s", factionID),
					sim.Action{Kind: "set_ambition", Item: "seize_leadership", TargetID: factionID})
			}
		}
	}

	ambitionTargets := make(map[string]bool, len(a.Ambitions))
	for _, amb := range a.Ambitions {
		if !amb.Completed && amb.TargetID != "" && (amb.Kind == "avenge_blood" || amb.Kind == "expose_corrupt") {
			ambitionTargets[amb.TargetID] = true
		}
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
			if inquisitive || a.Trust[nb.ID] < 0 || ambitionTargets[nb.ID] {
				add(fmt.Sprintf("Investigate %s for hidden secrets (-5 energy)", nb.Name),
					sim.Action{Kind: "investigate_actor", TargetID: nb.ID})
			}
		}
	}

	for _, s := range p.KnownSecrets {
		if s.Exposed || s.OwnerID == a.ID || !sim.ActorHasTag(s.KnownBy, a.ID) {
			continue
		}
		if allowed["expose_secret"] && a.Energy >= 4 && s.HolderStances[a.ID] == "confront" {
			hasListener := false
			for _, nb := range p.Nearby {
				if nb.ID != a.ID && nb.ID != s.OwnerID && !nb.Jailed && !nb.Sleeping {
					hasListener = true
					break
				}
			}
			if !hasListener {
				continue
			}
			add(fmt.Sprintf("Publicly expose %s's secret (%s, severity %d) (-4 energy)", s.OwnerID, s.Kind, s.Severity),
				sim.Action{Kind: "expose_secret", TargetID: s.OwnerID, Item: s.ID})
		}
	}
	for _, factionID := range p.LeadershipChallenges {
		add("Challenge for leadership of "+factionID, sim.Action{Kind: "challenge_leadership", FactionID: factionID})
	}
	for _, ambition := range a.Ambitions {
		if !ambition.Completed && ambition.Kind == "master_skill" && a.Skills[ambition.TargetID] > 0 && a.Skills[ambition.TargetID] < ambition.TargetValue && a.Energy >= 4 {
			add("Practise "+ambition.TargetID, sim.Action{Kind: "practice_skill", SkillID: ambition.TargetID})
		}
	}
	for _, action := range p.BlackmailActions {
		add(fmt.Sprintf("Leverage secret against %s (%s, %d coins)", action.TargetID, action.Text, action.Price), action)
	}
}

func planAmbitionAction(p sim.Perception, consider func(sim.Action) bool) bool {
	a := p.Self
	if a.Energy < 12 || a.Health < 55 {
		return false
	}
	traits := ExtractPersonalityTraits(a.Personality)
	isOversight := p.Catalog.IsOversightTags(a.IdentityTags()) || p.Catalog.IsEnforcementTags(a.IdentityTags()) || sim.ActorHasTag(traits, "righteous")

	nearbyMap := make(map[string]sim.Neighbor, len(p.Nearby))
	for _, nb := range p.Nearby {
		if !nb.Jailed && !nb.Sleeping {
			nearbyMap[nb.ID] = nb
		}
	}
	selfTier := p.Catalog.ActorPowerTier(a)
	isRighteous := sim.ActorHasTag(traits, "righteous") || sim.ActorHasTag(traits, "dutiful") || sim.ActorHasTag(traits, "stern")
	isRuthless := sim.ActorHasTag(traits, "ruthless") || sim.ActorHasTag(traits, "vengeful") || sim.ActorHasTag(traits, "paranoid") || p.Catalog.IsOutlawTags(a.IdentityTags())

	// 1. Witness silencing / hush-money when the secret's owner knows a bystander caught them red-handed.
	if isRuthless && a.Health >= 60 {
		for _, s := range p.KnownSecrets {
			if s.Exposed || s.OwnerID != a.ID || len(s.WitnessedBy) == 0 {
				continue
			}
			for _, wid := range s.WitnessedBy {
				nb, here := nearbyMap[wid]
				if !here || a.Trust[wid] >= 2 {
					continue
				}
				if p.Catalog.IsEnforcementTags(nb.Tags) && a.Money >= 6 {
					if consider(sim.Action{Kind: "bribe", TargetID: wid}) {
						return true
					}
				}
				if nb.PowerTier <= selfTier && consider(sim.Action{Kind: "attack", TargetID: wid}) {
					return true
				}
			}
		}
	}

	// 2. Act on known unexposed secrets of others strictly according to explicit HoldStance.
	for _, s := range p.KnownSecrets {
		if s.Exposed || s.OwnerID == a.ID || !sim.ActorHasTag(s.KnownBy, a.ID) {
			continue
		}
		stance := ""
		if s.HolderStances != nil {
			stance = s.HolderStances[a.ID]
		}
		nb, ownerHere := nearbyMap[s.OwnerID]

		switch stance {
		case "shield", "bide_time", "paid_silence":
			// Deliberately protecting kin/ally: never expose or blackmail.
			continue

		case "fear_silence":
			// Too afraid of a stronger owner to speak out; quietly leave if co-located to avoid being silenced.
			if ownerHere && a.Energy >= 16 {
				if len(p.Roads) > 0 && consider(sim.Action{Kind: "move", Location: p.Roads[0].To}) {
					return true
				}
				if len(p.VenueRoutes) > 0 && consider(sim.Action{Kind: "move_venue", Location: p.VenueRoutes[0].ToVenue}) {
					return true
				}
			}
			continue

		case "confront":
			// Righteous / enforcer caught red-handed or discovered severe wrongdoing -> immediate confrontation!
			selfComplicit := s.Kind == "covert_bribe" && s.SubjectID == a.ID && a.IllicitGain > 0
			if selfComplicit {
				continue
			}
			if ownerHere {
				if nb.PowerTier >= selfTier+2 {
					// Outclassed righteous witness backs off to report to higher authority instead of suiciding.
					if len(p.Roads) > 0 && consider(sim.Action{Kind: "move", Location: p.Roads[0].To}) {
						return true
					}
					continue
				}
				if consider(sim.Action{Kind: "arrest", TargetID: s.OwnerID}) {
					return true
				}
				if isRighteous && s.Severity >= 60 && a.Health >= 65 && selfTier >= nb.PowerTier && sim.ActorHasTag(s.WitnessedBy, a.ID) {
					if consider(sim.Action{Kind: "attack", TargetID: s.OwnerID}) {
						return true
					}
				}
			}
			if len(p.Nearby) > 0 && consider(sim.Action{Kind: "expose_secret", TargetID: s.OwnerID, Item: s.ID}) {
				return true
			}
			continue

		}
	}
	for _, action := range p.BlackmailActions {
		if consider(action) {
			return true
		}
	}

	// 3. Actively pursue declared personal ambitions (avenge_blood, expose_corrupt, ascend_tier).
	for _, amb := range a.Ambitions {
		if amb.Completed {
			continue
		}
		switch amb.Kind {
		case "avenge_blood":
			if nb, here := nearbyMap[amb.TargetID]; here && a.Health >= 60 {
				if nb.PowerTier <= selfTier+1 {
					if consider(sim.Action{Kind: "avenge_feud", TargetID: amb.TargetID}) {
						return true
					}
					if consider(sim.Action{Kind: "attack", TargetID: amb.TargetID}) {
						return true
					}
				}
				if consider(sim.Action{Kind: "investigate_actor", TargetID: amb.TargetID}) {
					return true
				}
			}
		case "expose_corrupt":
			for _, s := range p.KnownSecrets {
				if !s.Exposed && s.HolderStances[a.ID] == "confront" && s.OwnerID == amb.TargetID && sim.ActorHasTag(s.KnownBy, a.ID) && len(p.Nearby) > 0 {
					if consider(sim.Action{Kind: "expose_secret", TargetID: s.OwnerID, Item: s.ID}) {
						return true
					}
				}
			}
			if _, here := nearbyMap[amb.TargetID]; here {
				if consider(sim.Action{Kind: "investigate_actor", TargetID: amb.TargetID}) {
					return true
				}
			}
		case "master_skill":
			if a.Skills[amb.TargetID] < amb.TargetValue && consider(sim.Action{Kind: "practice_skill", SkillID: amb.TargetID}) {
				return true
			}
		case "seize_leadership":
			if consider(sim.Action{Kind: "challenge_leadership", FactionID: amb.TargetID}) {
				return true
			}
			for _, faction := range p.Factions {
				if faction.ID == amb.TargetID && faction.District != a.Location {
					if step, ok := stepTowardDistrict(p, faction.District, 0); ok && consider(step) {
						return true
					}
				}
			}
		case "ascend_tier":
			keys := make([]string, 0, len(p.Catalog.Transitions))
			for id := range p.Catalog.Transitions {
				keys = append(keys, id)
			}
			sort.Strings(keys)
			for _, id := range keys {
				tr := p.Catalog.Transitions[id]
				if tr.RequiredFormID != "" && tr.RequiredFormID != a.FormID {
					continue
				}
				if toForm, ok := p.Catalog.Forms[tr.SuccessFormID]; ok && toForm.PowerTier > selfTier {
					if consider(sim.Action{Kind: "attempt_transition", TransitionID: tr.ID}) {
						return true
					}
				}
			}
		}
	}

	if sim.ActorHasTag(traits, "ambitious") {
		for _, factionID := range p.LeadershipChallenges {
			if consider(sim.Action{Kind: "set_ambition", Item: "seize_leadership", TargetID: factionID}) {
				return true
			}
		}
	}
	if sim.ActorHasTag(traits, "studious") || sim.ActorHasTag(traits, "dutiful") {
		skills := make([]string, 0, len(a.Skills))
		for skillID := range a.Skills {
			skills = append(skills, skillID)
		}
		sort.Strings(skills)
		for _, skillID := range skills {
			if spec, ok := p.Catalog.Skills[skillID]; ok && a.Skills[skillID] < spec.EffectiveMaxLevel() &&
				consider(sim.Action{Kind: "set_ambition", Item: "master_skill", TargetID: skillID, Quantity: spec.EffectiveMaxLevel()}) {
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

package agent

import (
	"strconv"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func growthCareActions(p sim.Perception) []sim.Action {
	a := p.Self
	signed := make(map[int64]sim.GrowthCareAgreement, len(p.CareAgreements))
	for _, care := range p.CareAgreements {
		signed[care.GrowthID] = care
	}
	var actions []sim.Action
	for _, g := range p.Growth {
		if !p.AtGrowthSite(g) {
			continue
		}
		id := strconv.FormatInt(g.ID, 10)
		care, hasCare := signed[g.ID]
		if p.CanHarvestObservedGrowth(g) {
			actions = append(actions, sim.Action{Kind: "harvest", TargetID: id})
		}
		if g.Cohort && g.Ready && g.Harvested && g.Quantity > 0 && care.Harvest && care.CaretakerKind == "actor" && care.CaretakerID == a.ID {
			actions = append(actions, sim.Action{Kind: "dispatch_harvest", TargetID: id})
		}
		if g.Harvested || hasCare {
			continue
		}
		pending := map[string]bool{}
		for _, proposal := range p.Continuity.CareProposals {
			if proposal.GrowthID != g.ID {
				continue
			}
			pending[proposal.CaretakerID] = true
			ownerKind := "actor"
			if g.Cohort {
				ownerKind = "cohort"
			}
			if proposal.CaretakerKind == "actor" && proposal.CaretakerID == a.ID && proposal.OwnerID == g.OwnerID && proposal.OwnerKind == ownerKind {
				terms := proposal
				actions = append(actions, sim.Action{Kind: "accept_growth_care", TargetID: proposal.OwnerID, Care: &terms})
			}
		}
		if g.Cohort || g.OwnerID != a.ID || a.Task != nil && (a.Task.Action.Kind == "sleep" || a.Task.Dormant) {
			continue
		}
		for _, peer := range p.Nearby {
			if peer.Sleeping || peer.Jailed || peer.Resting || pending[peer.ID] {
				continue
			}
			for _, share := range []int64{0, 500, 1000} {
				terms := sim.GrowthCareAgreement{
					GrowthID: g.ID, OwnerKind: "actor", OwnerID: a.ID, CaretakerKind: "actor",
					CaretakerID: peer.ID, SupplierKind: "actor", SupplierID: peer.ID, Harvest: true, SharePermille: share,
				}
				actions = append(actions, sim.Action{Kind: "propose_growth_care", TargetID: peer.ID, Care: &terms})
			}
		}
	}
	return actions
}

func caseActions(p sim.Perception) []sim.Action {
	a := p.Self
	caps := p.Capabilities()
	inst := p.Catalog.EffectiveEnvironment().Institutions
	var actions []sim.Action
	acted := map[string]bool{}
	for _, entry := range p.LocalCases {
		if acted[entry.SuspectID] {
			continue
		}
		var kind string
		switch entry.Status {
		case "wanted":
			if inst.EnforcementEnabled() && caps["arrest"] && a.Energy >= 7 && a.Health >= 45 && p.Tick >= a.RestUntil {
				for _, n := range p.Nearby {
					if n.ID == entry.SuspectID && !n.Jailed {
						kind = "arrest"
						break
					}
				}
			}
		case "arrested":
			if a.Energy >= 6 && a.IllicitGain == 0 {
				if inst.OversightEnabled() && caps["audit_corruption"] {
					kind = "audit_corruption"
				} else if inst.EnforcementEnabled() && caps["hold_trial"] {
					kind = "hold_trial"
				}
			}
		case "reported":
			if caps["open_case"] && a.IllicitGain == 0 && a.Energy >= 4 {
				kind = "open_case"
			}
		}
		if kind != "" {
			actions = append(actions, sim.Action{Kind: kind, TargetID: entry.SuspectID, BasisEventID: entry.BasisEventID})
			acted[entry.SuspectID] = true
		}
	}
	return actions
}

func appealAction(p sim.Perception) (sim.Action, bool) {
	if len(p.KnownWrongfulCases) == 0 {
		return sim.Action{}, false
	}
	lead := p.KnownWrongfulCases[0]
	return sim.Action{Kind: "file_appeal", TargetID: lead.SuspectID, Location: lead.District, BasisEventID: lead.BasisEventID}, true
}

func mediationActions(p sim.Perception) []sim.Action {
	var actions []sim.Action
	for _, feud := range p.KnownFeuds {
		price := max(2, feud.Intensity/5)
		if feud.MediationPending {
			switch p.Self.ID {
			case feud.OffenderID:
				if feud.OffenderAccepted && !feud.AggrievedAccepted {
					continue
				}
			case feud.AggrievedID:
				if feud.AggrievedAccepted && !feud.OffenderAccepted {
					continue
				}
			default:
				continue
			}
			price = int(feud.MediationCoins)
			if int64(price) != feud.MediationCoins {
				continue
			}
		}
		actions = append(actions, sim.Action{Kind: "mediate_feud", Text: feud.ID, TargetID: feud.OffenderID, Price: price})
	}
	return actions
}

func leadershipChallenges(p sim.Perception) []string {
	var factions []string
	for _, id := range p.Self.Guilds {
		for _, f := range p.Factions {
			if f.ID == id && f.LeadershipContestOpen {
				factions = append(factions, id)
				break
			}
		}
	}
	return factions
}

func canRequestLoan(n sim.Neighbor, spec sim.MarketCreditSpec) bool {
	return !n.Jailed && !n.Sleeping && sim.ActorHasTag(n.Tags, spec.TraderTag)
}

func blackmailActions(p sim.Perception) []sim.Action {
	a := p.Self
	if a.Energy < 4 {
		return nil
	}
	here := map[string]sim.Neighbor{}
	for _, nb := range p.Nearby {
		if !nb.Sleeping && !nb.Jailed {
			here[nb.ID] = nb
		}
	}
	var actions []sim.Action
	for _, secret := range p.KnownSecrets {
		victim, present := here[secret.OwnerID]
		if secret.Exposed || !present || secret.HolderStances[a.ID] != "leverage" {
			continue
		}
		x := sim.Action{Kind: "blackmail", TargetID: victim.ID, Item: secret.ID, Text: "firm", Price: max(2, secret.Severity/5)}
		personality := strings.ToLower(a.Personality)
		switch {
		case containsTrait(personality, "cautious", "cunning", "diplomatic", "警觉", "狡黠"):
			x.Text, x.Price = "subtle", 2
		case containsTrait(personality, "ruthless", "reckless", "狠辣", "鲁莽"):
			x.Text, x.Price = "hostile", max(6, secret.Severity/3)
		}

		// Earlier demands and refusals are known to the offender. Estimate only
		// the minimum offense attainable, since the victim's means are private.
		var history sim.OffenseRecord
		refused := false
		for _, memory := range a.Memories {
			if memory.Source != a.ID || memory.TargetID != victim.ID || memory.Observation == nil ||
				memory.Observation.SecretID != secret.ID {
				continue
			}
			if memory.Kind == "blackmailed" || memory.Kind == "blackmail_refused" {
				_, repeats := sim.DecayedOffense(history, memory.Tick)
				history.RepeatCount, history.LastTick = repeats+1, memory.Tick
				refused = memory.Kind == "blackmail_refused"
			}
		}
		_, repeats := sim.DecayedOffense(history, p.Tick)
		if refused && repeats > 0 {
			continue
		}
		threshold := p.ObservedOffenseThreshold(victim, secret.Severity)
		if sim.BlackmailMinimumOffense(x.Price, x.Text, repeats) >= threshold {
			x.Text, x.Price = "subtle", 2
		}
		if sim.BlackmailMinimumOffense(x.Price, x.Text, repeats) < threshold {
			actions = append(actions, x)
		}
	}
	return actions
}

func containsTrait(personality string, traits ...string) bool {
	for _, trait := range traits {
		if strings.Contains(personality, trait) {
			return true
		}
	}
	return false
}

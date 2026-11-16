package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// rulesPolicy ranks offline choices using only generic catalog recipes, professions, guilds,
// house assets, and road-graph shortest paths—never hardcoded profession branches.
type rulesPolicy struct{}

func (rulesPolicy) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	a := p.Self
	itemKeys := sortedItemIDs(p.Catalog.Items)

	// 1a. Physiological thirst: drink fresh water at indoor/well/canal facilities or carried potable item.
	if a.Thirst >= 2 {
		if canDrawFreeWater(p) {
			return sim.Action{Kind: "drink"}, nil
		}
		for _, id := range itemKeys {
			if p.Catalog.Items[id].Potable && a.Items[id] >= 1 {
				return sim.Action{Kind: "drink", Item: id}, nil
			}
		}
		if a.Thirst >= 3 {
			if dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "water_source"); ok && dest != a.Location {
				if step, ok := stepTowardDistrict(p, dest, 0); ok {
					return step, nil
				}
			}
			if home := primaryHome(p); home != nil {
				if step, ok := stepToHouse(p, *home); ok {
					return step, nil
				}
			}
		}
	}

	// 1b. Physiological hunger: eat any available edible item.
	if a.Hunger > 0 {
		for _, id := range itemKeys {
			spec := p.Catalog.Items[id]
			if spec.Edible && a.Items[id] >= 1 {
				return sim.Action{Kind: "eat", Item: id}, nil
			}
		}
	}

	for _, g := range p.Growth {
		needsCare := !g.Ready && g.ProgressTicks >= g.CareUntil
		if !g.Ready && !needsCare {
			continue
		}
		if needsCare {
			supplied := true
			for item, qty := range g.Rule.Supplies {
				if int64(a.Items[item]) >= int64(qty)*g.Batches {
					continue
				}
				supplied = false
				for _, r := range p.Catalog.Recipes {
					if r.OutputItem == item && canPractice(p.Catalog, a, r) {
						if act, ok := resolveRecipe(p, r, 0); ok {
							return act, nil
						}
					}
				}
			}
			if !supplied {
				continue
			}
		}
		if g.District != a.Location {
			if step, ok := stepTowardDistrict(p, g.District, 0); ok {
				return step, nil
			}
			continue
		}
		if g.VenueID != "" && g.VenueID != a.VenueID {
			if route, _, mode, ok := p.Map.NextVenueHopForActor(p.Catalog, nil, a, a.VenueID, g.VenueID); ok {
				return sim.Action{Kind: "move_venue", Location: route.ToVenue, Channel: mode}, nil
			}
			continue
		}
		if g.Ready {
			return sim.Action{Kind: "harvest", TargetID: fmt.Sprint(g.ID)}, nil
		}
	}

	for _, n := range p.EnergyNodes {
		if n.StoredEnergy == 0 && n.FuelStock == 0 && n.Source != nil && n.Source.FuelItem != "" && a.Items[n.Source.FuelItem] > 0 {
			return sim.Action{Kind: "refuel", TargetID: n.ID, Quantity: int(min(int64(a.Items[n.Source.FuelItem]), n.Source.UnitsPerCycle))}, nil
		}
	}
	// 1c. Sanitation & nightsoil collection: relieve waste before discomfort or disease risk builds.
	if a.Waste >= 3 {
		return sim.Action{Kind: "relieve"}, nil
	}

	// 2. Settle incoming trade offers based on generic item demand, funds, and directed trust.
	freeMoney := availableMoneyAfterAgreements(p)
	for _, o := range p.Offers {
		if o.Buyer != a.ID {
			continue
		}
		sellerHere := false
		for _, b := range p.Nearby {
			if b.ID == o.Seller && !b.Jailed {
				sellerHere = true
				break
			}
		}
		want := desiredBuyerStock(p, o.Item)
		if sellerHere && a.Items[o.Item] < want && freeMoney >= o.Price && a.Trust[o.Seller] >= 0 {
			return sim.Action{Kind: "accept", OfferID: o.ID, TargetID: o.Seller}, nil
		}
		return sim.Action{Kind: "reject", OfferID: o.ID, TargetID: o.Seller}, nil
	}

	// 2b. Self-medication and wound/disease recuperation: treat wounds or contagious/cold illness.
	if (a.Health < 85 || a.DiseaseID != "") && p.Capabilities()["medicate"] {
		bestMed := ""
		bestScore := 0
		for _, id := range itemKeys {
			spec := p.Catalog.Items[id]
			if (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) || a.Items[id] < 1 {
				continue
			}
			score := spec.MedicinalHeal
			if a.DiseaseID != "" {
				score += spec.DiseaseCure * 20
			}
			if score > bestScore {
				bestScore = score
				bestMed = id
			}
		}
		if bestMed != "" {
			return sim.Action{Kind: "medicate", Item: bestMed}, nil
		}
	}
	if a.Health < 60 || (p.Tick < a.RestUntil && a.Health < 85) {
		return planSleep(p), nil
	}

	// 3. Sleep and circadian recovery: head home to sleep at bedtime or when exhausted.
	needNightSleep := (p.Clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, p.Clock.Hour)) && a.Energy < 85
	needExhaustionRest := a.Energy < 22
	if needNightSleep || needExhaustionRest {
		return planSleep(p), nil
	}

	// Continuing intentions yield to physiological needs, not to unrelated daily work.
	if action, ok := chooseContinuity(p); ok {
		return action, nil
	}

	// 4. Community healing if capable and carrying medicinal items.
	if p.Capabilities()["heal"] {
		if medItem := cheapestMedicinalItem(p.Catalog, a, itemKeys); medItem != "" {
			for _, b := range p.Nearby {
				if b.Health < sim.MaxHealth {
					return sim.Action{Kind: "heal", TargetID: b.ID, Item: medItem}, nil
				}
			}
		}
	}

	// 5. Standalone house asset maintenance when planks are on hand and roof condition drops.
	if home := primaryHome(p); home != nil && home.Condition < 70 && availableItemAfterAgreements(p, "plank") >= 1 && a.Energy >= 12 {
		if act, ok := stepToRepairHouse(p, *home); ok {
			return act, nil
		}
	}

	// 6. Asset level upgrade for an owned house when a capable carpenter has materials.
	if act, ok := planHouseUpgrade(p); ok {
		return act, nil
	}

	// 7. Information-driven travel or rumor retelling.
	if action, ok := informationAction(p); ok && (action.Kind == "move" || action.Kind == "leave_house" || p.Tick%3 == 0) {
		return action, nil
	}

	// 8. Guild affiliation and a second profession when subsistence is secure.
	if act, ok := planGuildProgress(p); ok {
		return act, nil
	}

	// 8a. Credit obligations, collateral foreclosure, and peaceful feud mediation.
	if act, ok := planCreditAndRelations(p); ok {
		return act, nil
	}

	// 8b. Martial sparring, yamen law-enforcement arrests, bandit/gang raids, and street pickpocketing.
	if act, ok := planConflictAndLaw(p); ok {
		return act, nil
	}

	// 8c. Ecological & civic duties: seasonal/crisis shrine rituals, threat hunting, and public-works repair.
	if act, ok := planEcologyRitualAndInfra(p); ok {
		return act, nil
	}

	// 9. Generic profession recipe planning (inputs -> bonus inputs -> facility/district -> craft).
	if action, ok := planCatalogWork(p); ok {
		if action.Kind == "craft" && (a.Plan == nil || a.Plan.Status != "active") {
			for _, candidate := range sim.ContinuityActions(p) {
				if candidate.Kind == "start_plan" && candidate.RecipeID == action.RecipeID {
					return candidate, nil
				}
			}
		}
		return action, nil
	}

	// 10. Converge on the market hub to trade surplus items and exchange news.
	if marketID, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "market"); ok && a.Location != marketID {
		if step, ok := stepTowardDistrict(p, marketID, 0); ok {
			return step, nil
		}
	} else if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, nil
	}

	if len(p.Nearby) > 0 {
		// Offer surplus tradeable goods (specialty products first) to co-located villagers.
		hasPendingOffer := false
		for _, o := range p.Offers {
			if o.Seller == a.ID {
				hasPendingOffer = true
				break
			}
		}
		if !hasPendingOffer && !recentlyMarketSaturated(a, p.Tick) {
			offerOrder := append([]string(nil), itemKeys...)
			sort.SliceStable(offerOrder, func(i, j int) bool {
				pi := p.Catalog.TagsProduce(a.IdentityTags(), offerOrder[i])
				pj := p.Catalog.TagsProduce(a.IdentityTags(), offerOrder[j])
				return pi && !pj
			})
			for _, itemID := range offerOrder {
				if p.Polity != nil && sim.ActorHasTag(p.Polity.EmbargoItems, itemID) {
					continue
				}
				spec := p.Catalog.Items[itemID]
				availQty := availableItemAfterAgreements(p, itemID)
				if !spec.Tradeable || spec.BatchQty <= 0 || availQty < spec.OfferThreshold || availQty < spec.BatchQty {
					continue
				}
				if !p.Catalog.TagsProduce(a.IdentityTags(), itemID) && availQty < desiredBuyerStock(p, itemID)+spec.BatchQty {
					continue
				}
				for _, b := range p.Nearby {
					if b.Sleeping || b.Jailed {
						continue
					}
					if p.Catalog.TagsProduce(b.Tags, itemID) {
						continue
					}
					if !p.Catalog.TagsConsume(b.Tags, itemID) {
						continue
					}
					if recentlyRejectedBy(a, b.ID, p.Tick) {
						continue
					}
					return sim.Action{
						Kind: "offer", TargetID: b.ID,
						Item: itemID, Quantity: spec.BatchQty, Price: p.Catalog.SalePrice(a, itemID),
					}, nil
				}
			}
		}

		if !recentlyChatted(a, p.Tick) {
			for idx := 0; idx < len(p.Nearby); idx++ {
				b := p.Nearby[(int(p.Tick)+idx)%len(p.Nearby)]
				if !b.Sleeping && !b.Jailed {
					return sim.Action{
						Kind:     "speak",
						TargetID: b.ID,
						Text:     fmt.Sprintf("I have %d food, %d energy, and am trying to %s.", a.Items["food"], a.Energy, a.Goal),
					}, nil
				}
			}
		}
	}

	return sim.Action{Kind: "wait"}, nil
}

func recentlyChatted(self sim.Actor, tick int64) bool {
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if tick-m.Tick > 16 {
			break
		}
		if m.Kind == "speech" && m.Source == self.ID {
			return true
		}
	}
	return false
}

func recentlyRejectedBy(self sim.Actor, buyerID string, tick int64) bool {
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if tick-m.Tick > 48 {
			break
		}
		if m.Kind == "trade_rejected" && m.Source == buyerID {
			return true
		}
	}
	return false
}

func recentlyMarketSaturated(self sim.Actor, tick int64) bool {
	rejections := 0
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if tick-m.Tick > 16 {
			break
		}
		if m.Kind == "trade_rejected" && m.TargetID == self.ID {
			rejections++
			if rejections >= 2 {
				return true
			}
		}
	}
	return false
}

// canPractice reports whether the actor's profession levels, guild standing, and tags permit the recipe.
func canPractice(cat sim.Catalog, a sim.Actor, r sim.Recipe) bool {
	ok, _ := cat.ProfessionGating(a, r)
	return ok
}

func desiredBuyerStock(p sim.Perception, item string) int {
	a := p.Self
	want := p.Catalog.DesiredStock(a.IdentityTags(), item)
	spec := p.Catalog.Items[item]
	if spec.Edible && want < 8 && !p.Catalog.TagsProduce(a.IdentityTags(), item) {
		want = 8
	}
	if item == "plank" {
		if home := primaryHome(p); home != nil && home.Condition < 80 && want < 2 {
			want = 2
		}
	}
	if p.Catalog.TagsConsume(a.IdentityTags(), item) && want < 2 {
		want = 2
	}
	return want
}

func primaryHome(p sim.Perception) *sim.House {
	a := p.Self
	for i := range p.AccessibleHouses {
		if p.AccessibleHouses[i].ID == a.HomeID {
			return &p.AccessibleHouses[i]
		}
	}
	var best *sim.House
	for i := range p.AccessibleHouses {
		h := &p.AccessibleHouses[i]
		if best == nil || h.Comfort*100+h.Condition > best.Comfort*100+best.Condition {
			best = h
		}
	}
	return best
}

func planSleep(p sim.Perception) sim.Action {
	a := p.Self
	ticks := 8
	if p.Clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, p.Clock.Hour) || a.Energy < 40 || a.Health < 70 {
		ticks = 20
	}
	home := primaryHome(p)
	if home != nil {
		if a.InsideHouse == home.ID {
			return sim.Action{Kind: "sleep", Quantity: ticks}
		}
		if a.InsideHouse != "" {
			return sim.Action{Kind: "leave_house"}
		}
		if a.Location == home.District {
			return sim.Action{Kind: "enter_house", HouseID: home.ID}
		}
		if hop, _, ok := p.Map.NextHop(a.Location, home.District); ok {
			effEnergy := hop.EnergyCost
			for _, rv := range p.Roads {
				if rv.To == hop.To {
					effEnergy = rv.EnergyCost
					break
				}
			}
			if a.Energy >= effEnergy {
				return sim.Action{Kind: "move", Location: hop.To}
			}
		}
	}
	if a.Energy < 35 || a.Health < 55 || p.Clock.IsNight {
		return sim.Action{Kind: "sleep", Quantity: ticks}
	}
	return sim.Action{Kind: "wait"}
}

func stepTowardDistrict(p sim.Perception, targetDistrict string, basisEventID int64) (sim.Action, bool) {
	a := p.Self
	if a.Location == targetDistrict {
		if a.InsideHouse != "" {
			return sim.Action{Kind: "leave_house", BasisEventID: basisEventID}, true
		}
		return sim.Action{}, false
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house", BasisEventID: basisEventID}, true
	}
	hop, _, ok := p.Map.NextHop(a.Location, targetDistrict)
	if !ok {
		return sim.Action{}, false
	}
	effEnergy := hop.EnergyCost
	for _, rv := range p.Roads {
		if rv.To == hop.To {
			effEnergy = rv.EnergyCost
			break
		}
	}
	if a.Energy < effEnergy {
		return planSleep(p), true
	}
	return sim.Action{Kind: "move", Location: hop.To, BasisEventID: basisEventID}, true
}

func stepToHouse(p sim.Perception, h sim.House) (sim.Action, bool) {
	a := p.Self
	if a.InsideHouse == h.ID {
		return sim.Action{}, false
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, true
	}
	if a.Location == h.District {
		return sim.Action{Kind: "enter_house", HouseID: h.ID}, true
	}
	return stepTowardDistrict(p, h.District, 0)
}

func stepToRepairHouse(p sim.Perception, h sim.House) (sim.Action, bool) {
	a := p.Self
	if a.Location == h.District && (a.InsideHouse == "" || a.InsideHouse == h.ID) {
		return sim.Action{Kind: "repair_house", HouseID: h.ID}, true
	}
	return stepToHouse(p, h)
}

// planHouseUpgrade renovates an owned house when a carpenter of at least level 2 has materials.
func planHouseUpgrade(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	home := primaryHome(p)
	if home == nil || home.ID != a.HomeID || home.OwnerID != a.ID || home.Level >= 3 {
		return sim.Action{}, false
	}
	if a.Professions["carpenter"] < 2 || a.Energy < 20 {
		return sim.Action{}, false
	}
	if availableItemAfterAgreements(p, "plank") < home.Level || availableMoneyAfterAgreements(p) < 4*home.Level {
		return sim.Action{}, false
	}
	if a.InsideHouse == home.ID {
		return sim.Action{Kind: "upgrade_house", HouseID: home.ID}, true
	}
	return stepToHouse(p, *home)
}

func professionHasReachableWork(p sim.Perception, prof sim.Profession) bool {
	if prof.CombatBonusPerLevel > 0 {
		return true
	}
	for _, rit := range p.Catalog.Rituals {
		if rit.RequiredTag != "" && sim.ActorHasTag(prof.Grants, rit.RequiredTag) {
			if rit.NodeFeature == "" {
				return true
			}
			if _, _, ok := p.Map.NearestDistrictWithFeature(p.Self.Location, rit.NodeFeature); ok {
				return true
			}
		}
	}
	for _, r := range p.Catalog.Recipes {
		if r.Profession != prof.ID {
			continue
		}
		if r.NodeFeature == "" {
			return true
		}
		if _, _, ok := p.Map.NearestDistrictWithFeature(p.Self.Location, r.NodeFeature); ok {
			return true
		}
	}
	return false
}

// planGuildProgress joins a guild hall in the current district, advances into eligible Tier 2+ higher
// professions (e.g. scholar -> xiucai, blacksmith -> master_smith), or learns a second base profession
// so character level equals the sum of multiple profession levels.
func planGuildProgress(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	edibleQty := totalEdibleItems(p.Catalog, a)
	if p.Clock.Hour < 8 || p.Clock.Hour >= 18 || a.Energy < 30 || edibleQty < 2 {
		return sim.Action{}, false
	}
	// If the actor already qualifies for a Tier-2+ career promotion in any joined guild, travel to its hall and promote.
	for _, f := range p.Factions {
		if !f.Joined {
			continue
		}
		for _, profID := range f.Professions {
			prof, ok := p.Catalog.ProfessionByID(profID)
			if !ok || prof.ParentProfession == "" {
				continue
			}
			if ok, _ := p.Catalog.CanLearnProfession(a, prof); ok {
				if a.InsideHouse == "" && a.Location == f.District {
					return sim.Action{Kind: "join_profession", ProfessionID: prof.ID}, true
				}
				return stepTowardDistrict(p, f.District, 0)
			}
		}
	}
	if a.InsideHouse != "" || a.Energy < 50 || edibleQty < 3 {
		return sim.Action{}, false
	}
	for _, f := range p.Factions {
		if f.District != a.Location {
			continue
		}
		if !f.Joined {
			if p.Catalog.HasCoercivePower(f.Professions) {
				roleMatch := false
				for _, pid := range f.Professions {
					if a.Role == pid {
						roleMatch = true
						break
					}
				}
				if !roleMatch {
					continue
				}
			}
			if ok, _ := a.Attributes.Meets(f.Requirements); ok {
				return sim.Action{Kind: "join_faction", FactionID: f.ID}, true
			}
			continue
		}
		if len(a.Professions) >= 2 || (a.Role != "" && a.Professions[a.Role] < 2) {
			continue
		}
		for _, profID := range f.Professions {
			prof, ok := p.Catalog.ProfessionByID(profID)
			if !ok || prof.ParentProfession != "" || !professionHasReachableWork(p, prof) {
				continue
			}
			if ok, _ := p.Catalog.CanLearnProfession(a, prof); ok {
				return sim.Action{Kind: "join_profession", ProfessionID: prof.ID}, true
			}
		}
	}
	return sim.Action{}, false
}

func recentlyConflictActed(self sim.Actor, tick int64, window int64) bool {
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if tick-m.Tick > window {
			break
		}
		switch m.Kind {
		case "attacked", "sparred", "stolen", "steal_foiled", "raided", "raid_repelled", "arrested", "arrest_resisted", "extorted", "extort_resisted", "bribed", "corruption_purged":
			if m.Source == self.ID {
				return true
			}
		}
	}
	return false
}

func planCreditAndRelations(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	if a.Energy < 10 || a.Health < 40 || p.Tick < a.RestUntil {
		return sim.Action{}, false
	}
	allowed := p.Capabilities()
	freeMoney := availableMoneyAfterAgreements(p)
	if allowed["foreclose_collateral"] && p.OverdueBorrowerID != "" && p.OverdueHouseID != "" {
		return sim.Action{Kind: "foreclose_collateral", TargetID: p.OverdueBorrowerID, HouseID: p.OverdueHouseID}, true
	}
	if allowed["repay_loan"] && p.HasActiveLoan && (freeMoney == a.Money && a.Money >= 6 || freeMoney >= 18) && a.Hunger <= 3 {
		return sim.Action{Kind: "repay_loan"}, true
	}
	if allowed["mediate_feud"] && p.MediateTargetID != "" && a.Energy >= 12 && a.Health >= 50 && freeMoney >= 4 && !recentlyConflictActed(a, p.Tick, 24) {
		for _, b := range p.Nearby {
			if !b.Jailed && b.ID == p.MediateTargetID {
				return sim.Action{Kind: "mediate_feud", TargetID: b.ID}, true
			}
		}
	}
	return sim.Action{}, false
}

func planConflictAndLaw(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	if a.Energy < 28 || a.Health < 65 || p.Tick < a.RestUntil || recentlyConflictActed(a, p.Tick, 24) {
		return sim.Action{}, false
	}
	allowed := p.Capabilities()

	// 0. Upright censors and honest law officers audit & purge co-located officers when backed by epistemic evidence.
	if allowed["audit_corruption"] && a.IllicitGain == 0 && a.Energy >= 12 {
		for _, b := range p.Nearby {
			if basisID, hasEv := sim.ActorCorruptionEvidence(a, b.ID); !b.Jailed && hasEv {
				return sim.Action{Kind: "audit_corruption", TargetID: b.ID, BasisEventID: basisID}, true
			}
		}
	}

	// 0b. Upright censors file censorate appeals for known local wrongful convictions (or travel to the court district they heard about).
	if allowed["file_appeal"] && a.IllicitGain == 0 && a.Energy >= 10 {
		if p.HasOpenWrongfulCase && p.AppealTargetID != "" {
			return sim.Action{
				Kind:         "file_appeal",
				TargetID:     p.AppealTargetID,
				Location:     a.Location,
				BasisEventID: p.AppealBasisEventID,
			}, true
		}
		if a.Energy >= 16 {
			for _, lead := range sim.ActorKnownWrongfulLeads(a) {
				if lead.District != "" && lead.District != a.Location {
					visitedAfter := false
					for _, past := range a.Memories {
						if past.EventID > lead.BasisEventID && past.Kind == "moved" && strings.HasSuffix(past.Text, "to "+lead.District) {
							visitedAfter = true
							break
						}
					}
					if !visitedAfter {
						if step, ok := stepTowardDistrict(p, lead.District, lead.BasisEventID); ok {
							return step, true
						}
					}
				}
			}
		}
	}

	// 1. Yamen constables and watch guards arrest co-located outlaws (unless bribed to look the other way!).
	if allowed["arrest"] && a.Energy >= 12 {
		for _, b := range p.Nearby {
			if a.IllicitGain > 0 && a.Trust[b.ID] >= 3 {
				continue
			}
			if !b.Jailed && b.Health > 0 && (sim.ActorHasTag(b.Tags, "outlaw") || sim.ActorHasTag(b.Tags, "raider") || sim.ActorHasTag(b.Tags, "thief") || sim.ActorHasTag(b.Tags, "gang")) {
				return sim.Action{Kind: "arrest", TargetID: b.ID}, true
			}
		}
	}

	// 1b. Solvent outlaws bribe nearby law officers to buy protection from arrest.
	if allowed["bribe"] && availableMoneyAfterAgreements(p) >= 14 && a.Energy >= 10 {
		for _, b := range p.Nearby {
			if !b.Jailed && sim.ActorHasTag(b.Tags, "law") && a.Trust[b.ID] < 3 {
				return sim.Action{Kind: "bribe", TargetID: b.ID}, true
			}
		}
	}

	// 1c. Fighters/avengers pursue active blood-feud targets when co-located and healthy.
	if allowed["avenge_feud"] && len(p.ActiveGrudgeTargets) > 0 && a.Energy >= 18 && a.Health >= 55 {
		for _, targetID := range p.ActiveGrudgeTargets {
			for _, b := range p.Nearby {
				if !b.Jailed && b.Health > 0 && b.ID == targetID {
					return sim.Action{Kind: "avenge_feud", TargetID: b.ID}, true
				}
			}
		}
	}

	// 2. Martial artists spar with co-located fighters, guards, or constables to hone skills.
	if allowed["spar"] && a.Energy >= 40 && a.Health >= 75 && !p.Clock.IsNight {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && b.Health >= 70 && sim.ActorHasTag(b.Tags, "fighter") && !sim.ActorHasTag(b.Tags, "outlaw") {
				return sim.Action{Kind: "spar", TargetID: b.ID}, true
			}
		}
	}

	// 3. Thieves pickpocket non-outlaw neighbors at the market or crossroads.
	if allowed["steal"] && a.Energy >= 25 && a.Health >= 65 {
		for _, b := range p.Nearby {
			if !b.Jailed && !sim.ActorHasTag(b.Tags, "outlaw") && !sim.ActorHasTag(b.Tags, "law") {
				return sim.Action{Kind: "steal", TargetID: b.ID}, true
			}
		}
	}

	// 3b. Gangsters collect daytime protection fees from non-outlaw traders/artisans before resorting to bloody raids.
	if allowed["extort"] && a.Energy >= 30 && a.Health >= 65 && !p.Clock.IsNight {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && !sim.ActorHasTag(b.Tags, "outlaw") && !sim.ActorHasTag(b.Tags, "law") {
				return sim.Action{Kind: "extort", TargetID: b.ID}, true
			}
		}
	}

	// 4. Bandits and gang members raid non-outlaw targets when healthy.
	if allowed["raid"] && a.Energy >= 40 && a.Health >= 75 {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && b.Health > 35 && !sim.ActorHasTag(b.Tags, "outlaw") {
				return sim.Action{Kind: "raid", TargetID: b.ID}, true
			}
		}
	}

	return sim.Action{}, false
}

func planEcologyRitualAndInfra(p sim.Perception) (sim.Action, bool) {
	if p.DistrictState == nil {
		return sim.Action{}, false
	}
	a := p.Self
	allowed := p.Capabilities()

	if allowed["perform_ritual"] && a.Energy >= 20 {
		seasonTag := fmt.Sprintf("Y%d-%s", p.Calendar.Year, p.Calendar.SeasonID)
		for _, rit := range p.Catalog.Rituals {
			seasonalDue := rit.ScheduledSeason != "" && rit.ScheduledSeason == p.Calendar.SeasonID && p.DistrictState.LastRitualSeason != seasonTag
			crisisDue := rit.ScheduledSeason == "" && len(p.DistrictState.ActiveDisasters) > 0
			if !seasonalDue && !crisisDue {
				continue
			}
			if availableMoneyAfterAgreements(p) < rit.MoneyCost {
				continue
			}
			if !holdsAllItems(a.Items, rit.Inputs) {
				inKeys := make([]string, 0, len(rit.Inputs))
				for k := range rit.Inputs {
					inKeys = append(inKeys, k)
				}
				sort.Strings(inKeys)
				for _, inItem := range inKeys {
					if a.Items[inItem] >= rit.Inputs[inItem] {
						continue
					}
					for _, sub := range p.Catalog.Recipes {
						if sub.OutputItem == inItem && canPractice(p.Catalog, a, sub) {
							if act, ok := resolveRecipe(p, sub, 0); ok {
								return act, true
							}
						}
					}
				}
				continue
			}
			if rit.NodeFeature != "" && !p.Map.DistrictHasFeature(a.Location, rit.NodeFeature) {
				if dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, rit.NodeFeature); ok {
					if step, ok := stepTowardDistrict(p, dest, 0); ok {
						return step, true
					}
				}
				continue
			}
			if a.InsideHouse != "" {
				return sim.Action{Kind: "leave_house"}, true
			}
			if a.Energy < rit.EnergyCost {
				return planSleep(p), true
			}
			return sim.Action{Kind: "perform_ritual", RecipeID: rit.ID}, true
		}
	}

	if allowed["hunt_threat"] && a.Energy >= 25 && a.Health >= 70 && p.Tick >= a.RestUntil {
		bestID := ""
		bestAct := 0
		for _, enc := range p.Catalog.Encounters {
			thresh := enc.BaseActivity + 6
			if thresh < 10 {
				thresh = 10
			}
			if act := p.DistrictState.ThreatActivity[enc.ID]; act >= thresh && act > bestAct {
				bestAct = act
				bestID = enc.ID
			}
		}
		if bestID != "" {
			if a.InsideHouse != "" {
				return sim.Action{Kind: "leave_house"}, true
			}
			return sim.Action{Kind: "hunt_threat", TargetID: bestID}, true
		}
	}

	if allowed["maintain_infra"] && a.Energy >= 25 {
		infraKeys := make([]string, 0, len(p.DistrictState.Infrastructure))
		for k := range p.DistrictState.Infrastructure {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, infraID := range infraKeys {
			cond := p.DistrictState.Infrastructure[infraID]
			spec, exists := p.Catalog.Infrastructures[infraID]
			if !exists || cond >= (spec.MaxCondition*65)/100 || a.Energy < spec.BuildEnergy || !holdsAllItems(a.Items, spec.BuildInputs) {
				continue
			}
			if a.InsideHouse != "" {
				return sim.Action{Kind: "leave_house"}, true
			}
			return sim.Action{Kind: "maintain_infra", Item: infraID}, true
		}
	}

	return sim.Action{}, false
}

func planCatalogWork(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	// Urgent subsistence first if edible stock is running low.
	if totalEdibleItems(p.Catalog, a) < 3 {
		for _, r := range p.Catalog.Recipes {
			if p.Catalog.Items[r.OutputItem].Edible && canPractice(p.Catalog, a, r) {
				if act, ok := resolveRecipe(p, r, 0); ok {
					return act, true
				}
			}
		}
	}

	// Otherwise produce items below the actor's identity-profile target stock.
	for _, r := range p.Catalog.Recipes {
		if !canPractice(p.Catalog, a, r) {
			continue
		}
		target := p.Catalog.DesiredStock(a.IdentityTags(), r.OutputItem)
		if target <= 0 || a.Items[r.OutputItem] >= target {
			continue
		}
		if act, ok := resolveRecipe(p, r, 0); ok {
			return act, true
		}
	}
	return sim.Action{}, false
}

func resolveRecipe(p sim.Perception, r sim.Recipe, depth int) (sim.Action, bool) {
	if depth > 3 {
		return sim.Action{}, false
	}
	if blocked, _ := sim.IsRecipeBlockedByWorldLaw(p.WorldLaws, r); blocked {
		return sim.Action{}, false
	}
	if r.Growth != nil {
		target := max(r.OutputQty, desiredBuyerStock(p, r.OutputItem))
		if p.Self.Plan != nil && p.Self.Plan.Status == "active" && p.Self.Plan.RecipeID == r.ID {
			target = max(r.OutputQty, p.Self.Plan.Quantity-p.Self.Plan.Produced)
		}
		var pending int64
		for _, g := range p.Growth {
			if g.RecipeID == r.ID {
				pending += g.Quantity
			}
		}
		if pending >= int64(target) {
			return sim.Action{}, false
		}
	}

	a := p.Self

	// Check required inputs in deterministic key order; recursively produce missing inputs if capable.
	if len(r.Inputs) > 0 {
		inKeys := make([]string, 0, len(r.Inputs))
		for k := range r.Inputs {
			inKeys = append(inKeys, k)
		}
		sort.Strings(inKeys)
		for _, inItem := range inKeys {
			if a.Items[inItem] >= r.Inputs[inItem] {
				continue
			}
			for _, sub := range p.Catalog.Recipes {
				if sub.OutputItem == inItem && canPractice(p.Catalog, a, sub) {
					if act, ok := resolveRecipe(p, sub, depth+1); ok {
						return act, true
					}
				}
			}
			return sim.Action{}, false
		}
	}

	// Prepare consumable bonus input (e.g., compost) when already at an equipped home or reasonably stocked.
	if r.BonusInput != "" && !r.DurableBonus && a.Items[r.BonusInput] < 1 {
		for _, sub := range p.Catalog.Recipes {
			if sub.OutputItem == r.BonusInput && canPractice(p.Catalog, a, sub) {
				atBonusHouse := sub.HouseFacility != "" && p.CurrentHouse != nil && p.CurrentHouse.HasFacility(sub.HouseFacility)
				if atBonusHouse || a.Items[r.OutputItem] >= 6 {
					if act, ok := resolveRecipe(p, sub, depth+1); ok {
						return act, true
					}
				}
			}
		}
	}

	if availableMoneyAfterAgreements(p) < r.MoneyCost {
		return sim.Action{}, false
	}
	if a.Energy < r.EnergyCost {
		return planSleep(p), true
	}

	if r.HouseFacility != "" {
		var facilityHouse *sim.House
		for i := range p.AccessibleHouses {
			if p.AccessibleHouses[i].HasFacility(r.HouseFacility) {
				facilityHouse = &p.AccessibleHouses[i]
				break
			}
		}
		if facilityHouse == nil {
			return sim.Action{}, false
		}
		if a.InsideHouse == facilityHouse.ID {
			return sim.Action{Kind: "craft", RecipeID: r.ID}, true
		}
		return stepToHouse(p, *facilityHouse)
	}

	if r.NodeFeature != "" {
		if a.InsideHouse == "" && p.Map.DistrictHasFeature(a.Location, r.NodeFeature) {
			return sim.Action{Kind: "craft", RecipeID: r.ID}, true
		}
		dest, _, ok := p.Map.NearestDistrictWithFeature(a.Location, r.NodeFeature)
		if !ok {
			return sim.Action{}, false
		}
		return stepTowardDistrict(p, dest, 0)
	}

	return sim.Action{Kind: "craft", RecipeID: r.ID}, true
}

// informationAction acts on or relays recent structured supply observations and heard statements.
func informationAction(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	for i := len(a.Memories) - 1; i >= 0; i-- {
		m := a.Memories[i]
		if p.Tick-m.Tick > sim.TicksPerDay {
			continue
		}
		var claim sim.Statement
		text := ""
		if m.Observation != nil {
			switch m.Observation.Kind {
			case "item_produced":
				claim = sim.Statement{
					Topic:    "item_produced",
					Item:     m.Observation.Item,
					Location: m.Observation.Location,
					Quantity: m.Observation.Quantity,
				}
				text = fmt.Sprintf("I produced %d %s at %s.", claim.Quantity, claim.Item, claim.Location)
			case "macro_anomaly":
				mod := m.Observation.Modality
				if mod == "" {
					mod = "eyewitness"
				}
				claim = sim.Statement{
					Topic:     "macro_anomaly",
					SubjectID: m.Observation.SubjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
				}
				text = fmt.Sprintf("I observed %s (%s) at %s (intensity %d).", claim.SubjectID, mod, claim.Location, claim.Quantity)
			case "corruption":
				if m.Observation.SubjectID == "" || m.Observation.SubjectID == a.ID || a.IllicitGain > 0 {
					continue
				}
				claim = sim.Statement{
					Topic:     "corruption",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      "coins",
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
				}
				text = fmt.Sprintf("I witnessed %s taking %d illicit bribe coins at %s.", claim.SubjectID, claim.Quantity, claim.Location)
			case "wrongful_conviction":
				if m.Observation.ObjectID == "" || m.Observation.SubjectID == a.ID {
					continue
				}
				claim = sim.Statement{
					Topic:     "wrongful_conviction",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					CaseID:    m.Observation.CaseID,
					Location:  m.Observation.Location,
					Quantity:  1,
				}
				text = fmt.Sprintf("I witnessed %s wrongfully framing %s in %s (case #%d).", claim.SubjectID, claim.ObjectID, claim.Location, claim.CaseID)
			default:
				continue
			}
		} else if (m.Kind == "speech" || m.Kind == "channel_message") && m.Source != a.ID && m.Statement != nil {
			claim = *m.Statement
			if claim.Item == "" && (claim.Topic == "item_produced" || claim.Topic == "supply") {
				claim.Item = "food"
			}
			text = fmt.Sprintf("%s told me: %s", m.Source, m.Text)
		} else {
			continue
		}

		if claim.Topic == "wrongful_conviction" && claim.Location != "" && a.Location != claim.Location && a.IllicitGain == 0 && a.Energy >= 16 && p.Capabilities()["file_appeal"] {
			visitedAfter := false
			for _, past := range a.Memories {
				if past.Tick > m.Tick && past.Kind == "moved" && strings.HasSuffix(past.Text, "to "+claim.Location) {
					visitedAfter = true
					break
				}
			}
			if !visitedAfter {
				if step, ok := stepTowardDistrict(p, claim.Location, m.EventID); ok {
					return step, true
				}
			}
		}

		isSupplyTopic := claim.Topic == "item_produced" || claim.Topic == "supply" || claim.Topic == "surplus"
		if isSupplyTopic && m.Kind == "speech" && p.Tick-m.Tick <= 8 && a.Trust[m.Source] >= 0 && claim.Location != "" && a.Location != claim.Location {
			spec := p.Catalog.Items[claim.Item]
			threshold := spec.ShortageThreshold + 2
			if threshold <= 2 {
				threshold = 4
			}
			want := desiredBuyerStock(p, claim.Item)
			if a.Items[claim.Item] < threshold && a.Items[claim.Item] < want && !p.Catalog.TagsProduce(a.IdentityTags(), claim.Item) {
				visitedAfter := false
				for _, past := range a.Memories {
					if past.Tick > m.Tick && past.Kind == "moved" && strings.HasSuffix(past.Text, "to "+claim.Location) {
						visitedAfter = true
						break
					}
				}
				if !visitedAfter {
					if step, ok := stepTowardDistrict(p, claim.Location, m.EventID); ok {
						return step, true
					}
				}
			}
		}

		for _, b := range p.Nearby {
			if b.ID == m.Source || b.ID == claim.SubjectID || b.Sleeping || b.Jailed {
				continue
			}
			told := false
			for _, past := range a.Memories {
				if past.Kind == "speech" && past.Source == a.ID && past.TargetID == b.ID && past.RootEventID == m.RootEventID {
					told = true
					break
				}
			}
			if told {
				continue
			}
			return sim.Action{Kind: "speak", TargetID: b.ID, Text: text, Statement: &claim, BasisEventID: m.EventID}, true
		}
	}
	return sim.Action{}, false
}

func availableMoneyAfterAgreements(p sim.Perception) int {
	reserved := 0
	for _, g := range p.Continuity.Agreements {
		if g.Status == "active" && g.BuyerID == p.Self.ID && p.Tick <= g.DueTick {
			reserved += g.Price
		}
	}
	if p.Self.Money <= reserved {
		return 0
	}
	return p.Self.Money - reserved
}

func availableItemAfterAgreements(p sim.Perception, itemID string) int {
	reserved := 0
	for _, g := range p.Continuity.Agreements {
		if g.Status == "active" && g.SupplierID == p.Self.ID && g.Item == itemID && p.Tick <= g.DueTick {
			reserved += g.Quantity
		}
	}
	if p.Self.Items[itemID] <= reserved {
		return 0
	}
	return p.Self.Items[itemID] - reserved
}

// chooseContinuity is the offline character policy, not a world settlement rule.
// It returns one choice; only World.Apply performs transfers or records commitments.
func chooseContinuity(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	allowed := p.Capabilities()
	freeMoney := availableMoneyAfterAgreements(p)
	nearby := func(id string) bool {
		for _, n := range p.Nearby {
			if n.ID == id && !n.Sleeping && !n.Jailed {
				return true
			}
		}
		return false
	}
	for _, r := range p.LoanRequests {
		if r.Status != "pending" || r.LenderID != a.ID || !nearby(r.BorrowerID) {
			continue
		}
		kind := "reject_loan"
		if a.Trust[r.BorrowerID] >= 0 && freeMoney-r.Principal >= 15 {
			kind = "accept_loan"
		}
		if allowed[kind] {
			return sim.Action{Kind: kind, TargetID: r.BorrowerID}, true
		}
	}
	actions := sim.ContinuityActions(p)
	for _, x := range actions {
		if x.Kind == "accept_agreement" && allowed[x.Kind] {
			if a.Trust[x.TargetID] >= 0 && freeMoney >= x.Price && a.Items[x.Item] < desiredBuyerStock(p, x.Item) {
				return x, true
			}
			return sim.Action{Kind: "reject_agreement", TargetID: x.TargetID, AgreementID: x.AgreementID}, true
		}
		if x.Kind == "fulfill_agreement" && allowed[x.Kind] {
			return x, true
		}
	}
	// Produce the goods already promised before taking up a new line of work.
	for _, g := range p.Continuity.Agreements {
		if g.Status != "active" || g.SupplierID != a.ID || p.Tick > g.DueTick || a.Items[g.Item] >= g.Quantity {
			continue
		}
		for _, r := range p.Catalog.Recipes {
			if r.OutputItem == g.Item && canPractice(p.Catalog, a, r) {
				if x, ok := resolveRecipe(p, r, 0); ok {
					return x, true
				}
			}
		}
	}
	if allowed["borrow_loan"] && !p.HasActiveLoan && a.Money < 2 && totalEdibleItems(p.Catalog, a) < 3 {
		pending := false
		for _, r := range p.LoanRequests {
			if r.BorrowerID == a.ID && r.Status == "pending" {
				pending = true
			}
		}
		if !pending {
			for _, q := range p.LoanQuotes {
				if q.Principal > 0 && nearby(q.LenderID) && a.Trust[q.LenderID] >= 0 {
					return sim.Action{Kind: "borrow_loan", TargetID: q.LenderID}, true
				}
			}
		}
	}
	if a.Plan != nil && a.Plan.Status == "active" {
		if a.Plan.Stage == "produce" {
			if r, ok := p.Catalog.RecipeByID(a.Plan.RecipeID); ok && canPractice(p.Catalog, a, r) {
				return resolveRecipe(p, r, 0)
			}
		}
		// Propose a continuing sale when there is a compatible buyer, rather than
		// dropping the objective after production or manufacturing a buyer response.
		for _, x := range actions {
			if x.Kind != "propose_agreement" || x.Item != a.Plan.Item || !allowed[x.Kind] || a.Trust[x.TargetID] < 0 {
				continue
			}
			for _, n := range p.Nearby {
				if n.ID == x.TargetID && p.Catalog.TagsConsume(n.Tags, x.Item) && !p.Catalog.TagsProduce(n.Tags, x.Item) && !recentlyRejectedBy(a, n.ID, p.Tick) {
					return x, true
				}
			}
		}
	}
	return sim.Action{}, false
}

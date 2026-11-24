package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func recentlyMovedDistrict(self sim.Actor, tick int64, window int64) bool {
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if tick-m.Tick > window {
			break
		}
		if (m.Kind == "moved" || m.Kind == "moved_venue") && m.Source == self.ID {
			return true
		}
	}
	return false
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
	if spec.Edible && spec.NutritionUnits > 0 {
		daily := p.Catalog.ActorDailyNutrition(a)
		if daily <= 0 || !p.Catalog.ActorAcceptsFood(a, spec) {
			return want
		}
		need := max(int64(0), 2*daily+int64(a.Hunger)-p.Catalog.AvailableNutrition(a))
		return a.Items[item] + int((need+spec.NutritionUnits-1)/spec.NutritionUnits)
	}
	if item == "plank" {
		if home := primaryHome(p); home != nil && home.Condition < 80 && want < 2 {
			want = 2
		}
	}
	if p.Catalog.TagsConsume(a.IdentityTags(), item) && want < 2 {
		want = 2
	}
	if a.Plan != nil && a.Plan.Status == "active" && a.Plan.Stage == "produce" {
		if r, ok := p.Catalog.RecipeByID(a.Plan.RecipeID); ok {
			want = max(want, r.StartRequirements()[item])
		}
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

func sleepDuration(a sim.Actor, clock sim.Clock) int {
	if clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, clock.Hour) || a.Energy < 40 || a.Health < 70 {
		return 20
	}
	return 8
}

func planSleep(p sim.Perception) sim.Action {
	a := p.Self
	ticks := sleepDuration(a, p.Clock)
	home := primaryHome(p)
	if deed, known := p.KnownHouse(a.HomeID); known && deed.District != a.Location {
		if step, ok := stepToHouseNoSleep(p, deed); ok {
			return step
		}
	}
	if home != nil {
		if a.InsideHouse == home.ID {
			return sim.Action{Kind: "sleep", Quantity: ticks}
		}
		if step, ok := stepToHouseNoSleep(p, home.Site()); ok {
			return step
		}
	}
	if a.Energy < 35 || a.Health < 55 || p.Clock.IsNight {
		return sim.Action{Kind: "sleep", Quantity: ticks}
	}
	return sim.Action{Kind: "wait"}
}

func stepToHouseNoSleep(p sim.Perception, h sim.HouseSite) (sim.Action, bool) {
	a := p.Self
	if a.InsideHouse == h.ID {
		return sim.Action{}, false
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, true
	}
	if h.VenueID != "" && a.VenueID != "" && a.VenueID != h.VenueID {
		for _, vr := range p.VenueRoutes {
			if (vr.ToVenue == h.VenueID || sim.ActorHasTag(vr.LeadsToward, h.VenueID)) && a.Energy >= vr.EnergyCost {
				return sim.Action{Kind: "move_venue", Location: vr.ToVenue, Channel: vr.ModeID}, true
			}
		}
		return sim.Action{}, false
	}
	if a.Location == h.District {
		return sim.Action{Kind: "enter_house", HouseID: h.ID}, true
	}
	if hop, _, ok := p.Map.NextHop(a.Location, h.District); ok {
		for _, rv := range p.Roads {
			if rv.To == hop.To && a.Energy >= rv.EnergyCost {
				return sim.Action{Kind: "move", Location: hop.To}, true
			}
		}
	}
	return sim.Action{}, false
}

func stepTowardVenue(p sim.Perception, targetVenue string, basisEventID int64) (sim.Action, bool) {
	a := p.Self
	if targetVenue == "" {
		return sim.Action{}, false
	}
	if a.VenueID == targetVenue {
		if a.InsideHouse != "" {
			return sim.Action{Kind: "leave_house", BasisEventID: basisEventID}, true
		}
		return sim.Action{}, false
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house", BasisEventID: basisEventID}, true
	}
	var best *sim.VenueRouteView
	for i := range p.VenueRoutes {
		vr := &p.VenueRoutes[i]
		if vr.ToVenue == targetVenue || sim.ActorHasTag(vr.LeadsToward, targetVenue) {
			if best == nil || vr.Ticks < best.Ticks || (vr.Ticks == best.Ticks && vr.ToVenue == targetVenue) {
				best = vr
			}
		}
	}
	if best == nil {
		return sim.Action{}, false
	}
	if a.Energy < best.EnergyCost {
		return planSleep(p), true
	}
	return sim.Action{Kind: "move_venue", Location: best.ToVenue, Channel: best.ModeID, BasisEventID: basisEventID}, true
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
	if len(p.Map.Venues) > 0 {
		for _, hop := range p.VenueRoutes {
			if sim.ActorHasTag(hop.LeadsTowardDistricts, targetDistrict) {
				if a.Energy < hop.EnergyCost {
					return planSleep(p), true
				}
				return sim.Action{Kind: "move_venue", Location: hop.ToVenue, Channel: hop.ModeID, BasisEventID: basisEventID}, true
			}
		}
		return sim.Action{}, false
	}
	var next *sim.RoadView
	for i := range p.Roads {
		road := &p.Roads[i]
		if sim.ActorHasTag(road.LeadsToward, targetDistrict) {
			next = road
			break
		}
		if road.To == targetDistrict {
			next = road
		}
	}
	if next == nil {
		return sim.Action{}, false
	}
	if a.Energy < next.EnergyCost {
		return planSleep(p), true
	}
	return sim.Action{Kind: "move", Location: next.To, BasisEventID: basisEventID}, true
}

func stepToHouse(p sim.Perception, h sim.HouseSite) (sim.Action, bool) {
	a := p.Self
	if a.InsideHouse == h.ID {
		return sim.Action{}, false
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, true
	}
	if h.VenueID != "" && a.VenueID != "" && a.VenueID != h.VenueID {
		return stepTowardVenue(p, h.VenueID, 0)
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
	return stepToHouse(p, h.Site())
}

// planHouseUpgrade renovates an owned dwelling when the configured builder profession has materials.
func planHouseUpgrade(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	home := primaryHome(p)
	if home == nil || home.ID != a.HomeID || home.OwnerID != a.ID || home.Level >= 3 {
		return sim.Action{}, false
	}
	env := p.Catalog.EffectiveEnvironment()
	if (env.BuilderProfession != "" && a.Professions[env.BuilderProfession] < 2) || a.Energy < 20 {
		return sim.Action{}, false
	}
	if availableItemAfterAgreements(p, env.RepairItem) < home.Level || availableMoneyAfterAgreements(p) < 4*home.Level {
		return sim.Action{}, false
	}
	if a.InsideHouse == home.ID {
		return sim.Action{Kind: "upgrade_house", HouseID: home.ID}, true
	}
	return stepToHouse(p, home.Site())
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
			if f.JoinBlockReason == "" {
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
	if allowed["mediate_feud"] && a.Energy >= 12 && a.Health >= 50 && !recentlyConflictActed(a, p.Tick, 24) {
		nearbyRel := map[string]string{}
		for _, nb := range p.Nearby {
			nearbyRel[nb.ID] = nb.FactionRelation
		}
		for _, action := range mediationActions(p) {
			if rel := nearbyRel[action.TargetID]; rel == "at_war" || rel == "hostile" {
				continue
			}
			if a.Trust[action.TargetID] <= -3 {
				continue
			}
			if action.TargetID != a.ID || freeMoney >= action.Price {
				return action, true
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
	for _, action := range caseActions(p) {
		if allowed[action.Kind] {
			return action, true
		}
	}

	// 0b. Upright censors file censorate appeals for known local wrongful convictions (or travel to the court district they heard about).
	if allowed["file_appeal"] && a.IllicitGain == 0 && a.Energy >= 10 {
		if appeal, ok := appealAction(p); ok {
			return appeal, true
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

	env := p.Catalog.EffectiveEnvironment()

	// 1. Enforcement personnel detain co-located violators (unless bribed to look the other way!).

	// 1b. Solvent outlaws bribe nearby enforcers to buy protection from detention.
	if allowed["bribe"] && availableMoneyAfterAgreements(p) >= 14 && a.Energy >= 10 {
		for _, b := range p.Nearby {
			if !b.Jailed && p.Catalog.IsEnforcementTags(b.Tags) && a.Trust[b.ID] < 3 {
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

	// 2. Combatants spar with co-located non-outlaw fighters to hone skills.
	if allowed["spar"] && a.Energy >= 40 && a.Health >= 75 && !p.Clock.IsNight {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && b.Health >= 70 && sim.ActorHasTag(b.Tags, "fighter") && !p.Catalog.IsOutlawTags(b.Tags) {
				return sim.Action{Kind: "spar", TargetID: b.ID}, true
			}
		}
	}

	// 3. Covert actors steal from non-outlaw, non-enforcer neighbors.
	if allowed["steal"] && a.Energy >= 25 && a.Health >= 65 {
		for _, b := range p.Nearby {
			if !b.Jailed && !p.Catalog.IsOutlawTags(b.Tags) && !p.Catalog.IsEnforcementTags(b.Tags) {
				return sim.Action{Kind: "steal", TargetID: b.ID}, true
			}
		}
	}

	// 3b. Coercers collect daytime protection fees from non-outlaw targets before resorting to raids.
	if env.Institutions.RacketeeringEnabled() && allowed["extort"] && a.Energy >= 30 && a.Health >= 65 && !p.Clock.IsNight {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && !p.Catalog.IsOutlawTags(b.Tags) && !p.Catalog.IsEnforcementTags(b.Tags) {
				return sim.Action{Kind: "extort", TargetID: b.ID}, true
			}
		}
	}

	// 4. Raiders plunder non-outlaw targets when healthy.
	if allowed["raid"] && a.Energy >= 40 && a.Health >= 75 {
		for _, b := range p.Nearby {
			if !b.Jailed && !b.Sleeping && b.Health > 35 && !p.Catalog.IsOutlawTags(b.Tags) {
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
	// Urgent subsistence first if edible stock is running low (skipped for fasting forms).
	if daily := p.Catalog.ActorDailyNutrition(a); daily > 0 && totalEdibleItems(p.Catalog, a) < int(daily) {
		for _, r := range p.Catalog.Recipes {
			if p.Catalog.ActorAcceptsFood(a, p.Catalog.Items[r.OutputItem]) && canPractice(p.Catalog, a, r) {
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
		// Keep the next crop growing while the previous harvest is being eaten.
		if r.Growth != nil && p.Catalog.Items[r.OutputItem].Edible {
			target = max(target, 2*r.OutputQty)
		}
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
	// Existing batches cover planned demand, while quick meals can still meet
	// immediate hunger during a long growing season.
	if r.Growth != nil || !p.Catalog.Items[r.OutputItem].Edible {
		if d := p.ActivePlan; d != nil && d.Item == r.OutputItem && d.Stage == "produce" {
			if d.DeficitToProduce == 0 {
				return sim.Action{}, false
			}
		} else {
			target := max(r.OutputQty, desiredBuyerStock(p, r.OutputItem))
			if expected := p.ExpectedGrowthStock(r.OutputItem); expected > 0 && expected >= int64(target-p.Self.Items[r.OutputItem]) {
				return sim.Action{}, false
			}
		}
	}

	// Before producing planting inputs, make sure there is land to use them on.
	if r.Growth != nil && r.NodeFeature == "farmland" {
		if len(p.Map.Venues) > 0 && p.Self.VenueID != "" {
			if _, ok := p.RecipeResourceVenue(r); !ok {
				return sim.Action{}, false
			}
		} else if _, ok := p.RecipeResourceDistrict(r); !ok {
			return sim.Action{}, false
		}
	}

	a := p.Self

	// Check required inputs in deterministic key order; recursively produce missing inputs if capable.
	requirements := r.StartRequirements()
	if len(requirements) > 0 {
		inKeys := make([]string, 0, len(requirements))
		for k := range requirements {
			inKeys = append(inKeys, k)
		}
		sort.Strings(inKeys)
		for _, inItem := range inKeys {
			if a.Items[inItem] >= requirements[inItem] {
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

	if availableMoneyAfterAgreements(p) < r.MoneyCost {
		return sim.Action{}, false
	}
	if a.Energy < r.EnergyCost {
		if p.Capabilities()["begin_rest"] {
			return sim.Action{Kind: "begin_rest"}, true
		}
		return planSleep(p), true
	}

	if r.HouseFacility != "" {
		facilityHouse, known := p.KnownWorkshop(r.HouseFacility)
		if !known {
			return sim.Action{}, false
		}
		if a.InsideHouse == facilityHouse.ID {
			return sim.Action{Kind: "craft", RecipeID: r.ID}, p.RecipeBlockReason(r) == ""
		}
		return stepToHouse(p, facilityHouse)
	}

	if r.NodeFeature != "" {
		if len(p.Map.Venues) > 0 && a.VenueID != "" {
			destVenue, ok := p.RecipeResourceVenue(r)
			if !ok {
				return sim.Action{}, false
			}
			if a.InsideHouse == "" && a.VenueID == destVenue {
				return sim.Action{Kind: "craft", RecipeID: r.ID}, p.RecipeBlockReason(r) == ""
			}
			return stepTowardVenue(p, destVenue, 0)
		}
		dest, ok := p.RecipeResourceDistrict(r)
		if !ok {
			return sim.Action{}, false
		}
		if a.InsideHouse == "" && a.Location == dest {
			return sim.Action{Kind: "craft", RecipeID: r.ID}, p.RecipeBlockReason(r) == ""
		}
		return stepTowardDistrict(p, dest, 0)
	}

	return sim.Action{Kind: "craft", RecipeID: r.ID}, p.RecipeBlockReason(r) == ""
}

// informationAction acts on or relays recent structured supply observations and heard statements.
func informationAction(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	for i := len(a.Memories) - 1; i >= 0; i-- {
		m := a.Memories[i]
		if p.Tick-m.Tick > sim.TicksPerDay {
			continue
		}
		report, ok := reportFromMemory(m, a, "")
		if !ok || !secretReportAllowed(p, m.RootEventID, report.Statement) {
			continue
		}
		claim := report.Statement

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
		if isSupplyTopic && claim.Item != "" && m.Kind == "speech" && p.Tick-m.Tick <= 8 && a.Trust[m.Source] >= 0 && claim.Location != "" && a.Location != claim.Location {
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
			// Skip retelling stale hearsay beyond 8 hours (first-hand observations still carry 1 day).
			if m.Observation == nil && p.Tick-m.Tick > 32 {
				continue
			}
			told := false
			for _, past := range a.Memories {
				if past.RootEventID == m.RootEventID && (past.Kind == "speech" || past.Kind == "channel_message") {
					// Already told b, OR b already told us about this exact root event!
					if (past.Source == a.ID && past.TargetID == b.ID) || (past.Source == b.ID && past.TargetID == a.ID) {
						told = true
						break
					}
				}
			}
			if told {
				continue
			}
			return sim.Action{Kind: "speak", TargetID: b.ID, Text: report.Text, Statement: claim, BasisEventID: report.BasisEventID}, true
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
			for _, peer := range p.Nearby {
				if canRequestLoan(peer, p.CreditTerms) && nearby(peer.ID) && a.Trust[peer.ID] >= 0 {
					return sim.Action{Kind: "borrow_loan", TargetID: peer.ID}, true
				}
			}
		}
	}
	if a.Plan != nil && a.Plan.Status == "active" {
		return executeActivePlan(p, actions, allowed, freeMoney)
	}
	return sim.Action{}, false
}

func executeActivePlan(p sim.Perception, continuityActions []sim.Action, allowed map[string]bool, freeMoney int) (sim.Action, bool) {
	a := p.Self
	plan := a.Plan
	if plan == nil || plan.Status != "active" {
		return sim.Action{}, false
	}
	rem := plan.RemainingToSell()
	if rem <= 0 {
		return sim.Action{}, false
	}
	dir := p.ActivePlan
	if dir == nil {
		dir = sim.BuildPlanDirective(p)
	}
	if dir.Stage == "produce" {
		switch {
		case dir.NextStep == "await_harvest", dir.NextStep == "care_stalled_growth":
			return sim.Action{}, false
		case len(dir.ReadyHarvestIDs) > 0:
			return sim.Action{Kind: "harvest", TargetID: fmt.Sprint(dir.ReadyHarvestIDs[0])}, true
		case strings.HasPrefix(dir.NextStep, "abandon_"):
			return sim.Action{Kind: "abandon_plan"}, allowed["abandon_plan"]
		case dir.NextStep == "acquire_inputs":
			if freeMoney > 0 {
				return stepToMarket(p)
			}
			return sim.Action{}, false
		case dir.NextStep == "production_blocked":
			if r, ok := p.Catalog.RecipeByID(dir.ActiveRecipeID); ok && a.Energy < r.EnergyCost {
				if allowed["begin_rest"] {
					return sim.Action{Kind: "begin_rest"}, true
				}
				return planSleep(p), true
			}
			return sim.Action{}, false
		case strings.HasPrefix(dir.NextStep, "craft:"):
			r, _ := p.Catalog.RecipeByID(dir.ActiveRecipeID)
			return sim.Action{Kind: "craft", RecipeID: r.ID}, freeMoney >= r.MoneyCost
		case strings.HasPrefix(dir.NextStep, "leave_house"):
			return sim.Action{Kind: "leave_house"}, true
		case strings.HasPrefix(dir.NextStep, "enter_workshop:"):
			return sim.Action{Kind: "enter_house", HouseID: dir.TargetHouseID}, true
		case dir.TargetVenueID != "" && a.VenueID != "":
			return stepTowardVenue(p, dir.TargetVenueID, 0)
		case dir.TargetDistrict != "":
			return stepTowardDistrict(p, dir.TargetDistrict, 0)
		}
		return sim.Action{}, false
	}

	// Stage == "sell": actual held stock covers the remaining quantity to sell.
	spec, hasSpec := p.Catalog.Items[plan.Item]
	embargoed := p.Polity != nil && sim.ActorHasTag(p.Polity.EmbargoItems, plan.Item)
	if embargoed {
		if allowed["abandon_plan"] {
			return sim.Action{Kind: "abandon_plan"}, true
		}
		return sim.Action{}, false
	}

	hasPendingPlanOfferHere := false
	for _, o := range p.Offers {
		if o.Seller == a.ID && o.Item == plan.Item {
			for _, nb := range p.Nearby {
				if nb.ID == o.Buyer && !nb.Jailed && !nb.Sleeping {
					hasPendingPlanOfferHere = true
					break
				}
			}
		}
	}

	if hasSpec && spec.Tradeable && spec.BatchQty > 0 && allowed["offer"] && !hasPendingPlanOfferHere {
		qty := min(spec.BatchQty, rem)
		if qty > 0 && a.Items[plan.Item] >= qty {
			cur := "coins"
			if len(spec.AcceptedCurrencies) > 0 && !sim.ItemAcceptsCurrency(spec, "coins") {
				cur = spec.AcceptedCurrencies[0]
			}
			actCur := ""
			if cur != "coins" {
				actCur = cur
			}
			price := planBatchPrice(p.Catalog.SalePrice(a, plan.Item), qty, spec.BatchQty)
			canOfferTo := func(nb sim.Neighbor, requireConsume bool) bool {
				if nb.Sleeping || nb.Jailed || recentlyRejectedBy(a, nb.ID, p.Tick) {
					return false
				}
				if spec.BuyerRequiredTag != "" && !sim.ActorHasTag(nb.Tags, spec.BuyerRequiredTag) {
					return false
				}
				if p.Catalog.TagsProduce(nb.Tags, plan.Item) {
					return false
				}
				if !p.Catalog.Capabilities(sim.Actor{Tags: nb.Tags})["accept"] {
					return false
				}
				if requireConsume && !p.Catalog.TagsConsume(nb.Tags, plan.Item) {
					return false
				}
				for _, o := range p.Offers {
					if o.Seller == a.ID && o.Buyer == nb.ID && o.Item == plan.Item {
						return false
					}
				}
				return true
			}
			for _, requireConsume := range []bool{true, false} {
				for _, nb := range p.Nearby {
					if canOfferTo(nb, requireConsume) {
						return sim.Action{
							Kind: "offer", TargetID: nb.ID,
							Item: plan.Item, Quantity: qty, Price: price, Currency: actCur,
						}, true
					}
				}
			}
		}
	}

	for _, x := range continuityActions {
		if x.Kind != "propose_agreement" || x.Item != plan.Item || !allowed[x.Kind] || a.Trust[x.TargetID] < 0 {
			continue
		}
		for _, n := range p.Nearby {
			if n.ID == x.TargetID && !p.Catalog.TagsProduce(n.Tags, x.Item) && (p.Catalog.TagsConsume(n.Tags, x.Item) || p.Catalog.Capabilities(sim.Actor{Tags: n.Tags})["accept_agreement"]) && !recentlyRejectedBy(a, n.ID, p.Tick) {
				return x, true
			}
		}
	}

	if hasPendingPlanOfferHere {
		return sim.Action{Kind: "wait"}, true
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, true
	}
	if step, ok := stepToMarket(p); ok {
		return step, true
	}
	if recentlyMarketSaturated(a, p.Tick) {
		if len(p.VenueRoutes) > 0 {
			vr := p.VenueRoutes[int(p.Tick)%len(p.VenueRoutes)]
			if a.Energy >= vr.EnergyCost {
				return sim.Action{Kind: "move_venue", Location: vr.ToVenue, Channel: vr.ModeID}, true
			}
		} else if len(p.Roads) > 0 {
			rd := p.Roads[int(p.Tick)%len(p.Roads)]
			if a.Energy >= rd.EnergyCost {
				return sim.Action{Kind: "move", Location: rd.To}, true
			}
		}
	}
	return sim.Action{}, false
}

func stepToMarket(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	if len(p.Map.Venues) > 0 && a.VenueID != "" {
		if a.InsideHouse == "" && p.Map.VenueOrDistrictHasFeature(a.VenueID, a.Location, "market") {
			return sim.Action{}, false
		}
		bestVenue := ""
		bestCost := int(^uint(0) >> 1)
		for _, v := range p.Map.Venues {
			if !p.Map.VenueOrDistrictHasFeature(v.ID, v.DistrictID, "market") {
				continue
			}
			if v.ID == a.VenueID {
				if a.InsideHouse != "" {
					return sim.Action{Kind: "leave_house"}, true
				}
				return sim.Action{}, false
			}
			if _, cost, _, ok := p.Map.NextVenueHopForActor(p.Catalog, nil, a, a.VenueID, v.ID); ok && cost < bestCost {
				bestVenue, bestCost = v.ID, cost
			}
		}
		if bestVenue != "" {
			return stepTowardVenue(p, bestVenue, 0)
		}
	}
	if marketID, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "market"); ok && a.Location != marketID {
		return stepTowardDistrict(p, marketID, 0)
	}
	if a.InsideHouse != "" {
		return sim.Action{Kind: "leave_house"}, true
	}
	return sim.Action{}, false
}

// Use existing extraction recipes, profession entry rules and real travel.
// This neither grants a skill nor supplies inventory as a side effect.
func immediateFoodAction(p sim.Perception, options []Option) (sim.Action, bool) {
	// Work offered here already satisfies energy, material and site rules.
	// Prefer obtaining a meal here before looking for a distant source.
	for _, option := range options {
		if option.Action.Kind == "craft" {
			r, _ := p.Catalog.RecipeByID(option.Action.RecipeID)
			if p.Catalog.ActorAcceptsFood(p.Self, p.Catalog.Items[r.OutputItem]) && r.Growth == nil && r.ResourcePool != "" && len(r.Inputs) == 0 {
				return option.Action, true
			}
		}
	}
	for _, r := range p.Catalog.Recipes {
		if !p.Catalog.ActorAcceptsFood(p.Self, p.Catalog.Items[r.OutputItem]) || r.Growth != nil || len(r.Inputs) > 0 || r.ResourcePool == "" {
			continue
		}
		if r.NodeFeature != "" {
			if _, _, ok := p.Map.NearestDistrictWithFeature(p.Self.Location, r.NodeFeature); !ok {
				continue
			}
		}
		if canPractice(p.Catalog, p.Self, r) {
			if x, ok := resolveRecipe(p, r, 0); ok {
				return x, true
			}
			continue
		}
		prof, ok := p.Catalog.ProfessionByID(r.Profession)
		if !ok || prof.ParentProfession != "" {
			continue
		}
		if ok, _ := p.Catalog.CanLearnProfession(p.Self, prof); !ok {
			continue
		}
		for _, f := range p.Factions {
			if f.ID != prof.Guild {
				continue
			}
			if !f.Joined {
				if f.JoinBlockReason != "" {
					continue
				}
			}
			if p.Self.Location != f.District || p.Self.InsideHouse != "" {
				if step, ok := stepTowardDistrict(p, f.District, 0); ok {
					return step, true
				}
				if step, ok := learnTravelProfession(p, f.District); ok {
					return step, true
				}
				continue
			}
			if !f.Joined {
				return sim.Action{Kind: "join_faction", FactionID: f.ID}, true
			}
			return sim.Action{Kind: "join_profession", ProfessionID: prof.ID}, true
		}
	}
	return sim.Action{}, false
}

// Learn through the existing guild actions when a profession's declared grants
// make the required journey possible. The temporary actor is used only to plan;
// the real actor gets no ability until joining and learning actually settle.
func learnTravelProfession(p sim.Perception, targetDistrict string) (sim.Action, bool) {
	if p.Self.VenueID == "" {
		return sim.Action{}, false
	}
	for _, prof := range p.Catalog.Professions {
		if len(prof.Grants) == 0 {
			continue
		}
		if ok, _ := p.Catalog.CanLearnProfession(p.Self, prof); !ok {
			continue
		}
		future := p.Self
		future.Tags = append(append([]string(nil), future.Tags...), prof.Grants...)
		enables := false
		for _, v := range p.Map.Venues {
			if v.DistrictID != targetDistrict {
				continue
			}
			if _, _, _, ok := p.Map.NextVenueHopForActor(p.Catalog, nil, future, future.VenueID, v.ID); ok {
				enables = true
				break
			}
		}
		if !enables {
			continue
		}
		for _, guild := range p.Factions {
			if guild.ID != prof.Guild {
				continue
			}
			if !guild.Joined {
				if guild.JoinBlockReason != "" {
					continue
				}
			}
			if guild.District != p.Self.Location || p.Self.InsideHouse != "" {
				if step, ok := stepTowardDistrict(p, guild.District, 0); ok {
					return step, true
				}
				continue
			}
			if !guild.Joined {
				return sim.Action{Kind: "join_faction", FactionID: guild.ID}, true
			}
			return sim.Action{Kind: "join_profession", ProfessionID: prof.ID}, true
		}
	}
	return sim.Action{}, false
}

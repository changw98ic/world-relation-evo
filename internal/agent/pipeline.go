package agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/runtrace"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type Option struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Action sim.Action `json:"action"`
}

type Selector interface {
	Select(context.Context, sim.Perception, []Option) (string, error)
}

type Pipeline struct {
	Selector Selector
	Trace    *runtrace.Writer
}

func (p Pipeline) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	pipeline := DecisionPipeline{Trace: p.Trace}
	switch selector := p.Selector.(type) {
	case RuleSelector:
		pipeline.Selector = RuleDecisionSelector{Selector: selector}
	case *RuleSelector:
		pipeline.Selector = RuleDecisionSelector{Selector: *selector}
	case Jev:
		pipeline.Selector = JevDecisionSelector{Jev: selector}
	case *Jev:
		pipeline.Selector = JevDecisionSelector{Jev: *selector}
	case *RulebookSelector:
		pipeline.Selector = RulebookDecisionSelector{Selector: selector}
	case *PlaybookSelector:
		pipeline.Selector = PlaybookDecisionSelector{Selector: selector}
	default:
		pipeline.Selector = optionDecisionSelector{selector}
	}
	return pipeline.Decide(ctx, view)
}

// modelRequestError preserves the original error chain without exposing endpoint
// credentials through Pipeline's decision_error record.
type modelRequestError struct {
	cause   error
	message string
}

func (e modelRequestError) Error() string { return e.message }
func (e modelRequestError) Unwrap() error { return e.cause }

func recordModelError(trace *runtrace.Writer, kind string, p sim.Perception, endpoint, apiKey string, err error) error {
	if err == nil || trace == nil {
		return err
	}
	message := err.Error()
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		// net/http includes the full request URL in url.Error.Error(). Omit it.
		message = requestErr.Op + ": " + requestErr.Err.Error()
	}
	for _, secret := range []string{endpoint, apiKey} {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	safeErr := modelRequestError{cause: err, message: message}
	return errors.Join(safeErr, trace.Record(kind, p.Tick, p.Self.ID, map[string]any{"error": message}))
}

func requirementSummary(req map[string]int) string {
	if len(req) == 0 {
		return "no attribute minimum"
	}
	keys := make([]string, 0, len(req))
	for k := range req {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s>=%d", k, req[k]))
	}
	return strings.Join(parts, ", ")
}

func Options(p sim.Perception) []Option {
	p.Self.Plan = sim.ClonePersonalPlan(p.Self.Plan)
	sim.SyncActorPlan(&p.Self, p.Tick, p.Catalog)
	if p.ActivePlan == nil && p.Self.Plan != nil && p.Self.Plan.Status == "active" {
		p.ActivePlan = sim.BuildPlanDirective(p)
	}
	a := p.Self
	dir := p.ActivePlan
	allowed := p.Capabilities()
	var options []Option
	add := func(label string, action sim.Action) {
		if !allowed[action.Kind] {
			return
		}
		if res := p.CheckAction(action); !res.Allowed {
			return
		}
		if action.Text == "" && (action.Kind == "speak" || action.Kind == "transmit_message" || action.Kind == "issue_decree") {
			action = buildCommunicationAction(p, action)
		}
		options = append(options, Option{ID: fmt.Sprintf("o%03d", len(options)), Label: label, Action: action})
	}

	for _, n := range p.EnergyNodes {
		if n.Source != nil && n.Source.FuelItem != "" && a.Items[n.Source.FuelItem] > 0 {
			qty := min(int64(a.Items[n.Source.FuelItem]), n.Source.UnitsPerCycle)
			add(fmt.Sprintf("Refuel %s with %d %s", n.ID, qty, n.Source.FuelItem), sim.Action{Kind: "refuel", TargetID: n.ID, Quantity: int(qty)})
		}
	}
	for _, action := range growthCareActions(p) {
		label := action.Kind + " " + action.TargetID
		if action.Care != nil {
			label = fmt.Sprintf("%s for growth %d: %s supplies care and materials for %d/1000 of the harvest; owner %s keeps the balance", action.Kind, action.Care.GrowthID, action.Care.CaretakerID, action.Care.SharePermille, action.Care.OwnerID)
		} else if action.Kind == "harvest" && dir != nil {
			for _, hid := range dir.ReadyHarvestIDs {
				if fmt.Sprint(hid) == action.TargetID {
					label += " [active plan harvest]"
					break
				}
			}
		}
		add(label, action)
	}
	add("Wait 15m and catch breath (+1 energy)", sim.Action{Kind: "wait"})
	if a.Resting {
		add("End the planned rest", sim.Action{Kind: "end_rest"})
	} else if a.Energy < sim.MaxEnergy {
		add("Plan rest: reach shelter, then recover energy before resuming work", sim.Action{Kind: "begin_rest"})
	}

	if a.InsideHouse != "" {
		houseName := a.InsideHouse
		if p.CurrentHouse != nil {
			houseName = p.CurrentHouse.Name
		}
		leaveLabel := fmt.Sprintf("Step outside from %s onto %s (15m)", houseName, a.Location)
		if dir != nil && strings.HasPrefix(dir.NextStep, "leave_house") {
			leaveLabel += " [active plan: " + dir.NextStep + "]"
		}
		add(leaveLabel, sim.Action{Kind: "leave_house"})
		if h := p.CurrentHouse; h != nil && h.OwnerID == a.ID && h.Level < 3 {
			env := p.Catalog.EffectiveEnvironment()
			repairItem := env.RepairItem
			builderProf := env.BuilderProfession
			materials := h.Level
			coins := 4 * h.Level
			if (builderProf == "" || a.Professions[builderProf] >= 2) && a.Items[repairItem] >= materials && a.Money >= coins && a.Energy >= 12 {
				add(fmt.Sprintf("Renovate %s to asset level %d (%d %s + %d coins, 60m, -12 energy)", h.Name, h.Level+1, materials, repairItem, coins), sim.Action{
					Kind: "upgrade_house", HouseID: h.ID,
				})
			}
		}
	} else {
		for _, f := range p.Factions {
			if f.District != a.Location {
				continue
			}
			if !f.Joined {
				if f.JoinBlockReason == "" {
					add(fmt.Sprintf("Join %s guild (guild level %d; charter %s)", f.Name, f.Level, requirementSummary(f.Requirements)), sim.Action{
						Kind: "join_faction", FactionID: f.ID,
					})
				}
				continue
			}
			for _, profID := range f.Professions {
				prof, ok := p.Catalog.ProfessionByID(profID)
				if !ok {
					continue
				}
				if ok, _ := p.Catalog.CanLearnProfession(a, prof); !ok {
					continue
				}
				if prof.ParentProfession != "" {
					add(fmt.Sprintf("Advance from %s to %s (%s, tier %d; requires %s lv>=%d, %s)",
						prof.ParentProfession, prof.Name, prof.RankTitle(1), prof.Tier, prof.ParentProfession, prof.MinParentLevel, requirementSummary(prof.Requirements)),
						sim.Action{Kind: "join_profession", ProfessionID: prof.ID})
				} else {
					add(fmt.Sprintf("Learn %s (%s, tier %d; requires %s)",
						prof.Name, prof.RankTitle(1), prof.Tier, requirementSummary(prof.Requirements)),
						sim.Action{Kind: "join_profession", ProfessionID: prof.ID})
				}
			}
		}
		for _, h := range p.AccessibleHouses {
			if h.District == a.Location && (h.VenueID == "" || a.VenueID == "" || h.VenueID == a.VenueID) {
				enterLabel := fmt.Sprintf("Enter %s (level %d, comfort %d, condition %d/100)", h.Name, h.Level, h.Comfort, h.Condition)
				if dir != nil && dir.TargetHouseID == h.ID {
					enterLabel += " [active plan workshop]"
				}
				add(enterLabel, sim.Action{Kind: "enter_house", HouseID: h.ID})
			}
		}
		for _, r := range p.Roads {
			if a.Energy < r.EnergyCost {
				continue
			}
			label := fmt.Sprintf("Walk road to %s (%dm, -%d energy", r.To, r.Ticks*15, r.EnergyCost)
			if len(r.LeadsToward) > 0 {
				label += "; leads toward " + strings.Join(r.LeadsToward, ", ")
			}
			label += ")"
			if dir != nil && dir.TargetDistrict != "" && (r.To == dir.TargetDistrict || sim.ActorHasTag(r.LeadsToward, dir.TargetDistrict)) {
				label += " [active plan route -> " + dir.TargetDistrict + "]"
			}
			add(label, sim.Action{Kind: "move", Location: r.To})
		}
		for _, vr := range p.VenueRoutes {
			if a.Energy < vr.EnergyCost {
				continue
			}
			modeLabel := vr.ModeID
			if modeLabel == "" {
				modeLabel = "walk"
			}
			label := fmt.Sprintf("Travel (%s) to venue %s (%dm, -%d energy", modeLabel, vr.ToVenue, vr.Ticks*15, vr.EnergyCost)
			if len(vr.LeadsToward) > 0 {
				label += "; leads toward " + strings.Join(vr.LeadsToward, ", ")
			}
			label += ")"
			if dir != nil {
				if dir.TargetVenueID != "" && (vr.ToVenue == dir.TargetVenueID || sim.ActorHasTag(vr.LeadsToward, dir.TargetVenueID)) {
					label += " [active plan route -> " + dir.TargetVenueID + "]"
				} else if dir.TargetVenueID == "" && dir.TargetDistrict != "" && vr.ToDistrict == dir.TargetDistrict {
					label += " [active plan route -> " + dir.TargetDistrict + "]"
				}
			}
			add(label, sim.Action{Kind: "move_venue", Location: vr.ToVenue, Channel: vr.ModeID})
		}
		if p.Polity != nil && p.Polity.LedPolityID != "" && a.Energy >= 4 &&
			(p.Polity.LedPolitySeatDistrict == "" || a.Location == p.Polity.LedPolitySeatDistrict) &&
			(p.Polity.LedPolitySeatVenue == "" || a.VenueID == "" || a.VenueID == p.Polity.LedPolitySeatVenue) {
			pid := p.Polity.LedPolityID
			pname := p.Polity.LedPolityName
			if p.Polity.LedPolityMarketTax != 50 {
				add(fmt.Sprintf("Promulgate 5%% market tax decree in %s (-4 energy)", pname),
					sim.Action{Kind: "issue_decree", FactionID: pid, Item: "market_tax", Quantity: 50})
			}
			if p.Polity.LedPolityMarketTax > 0 {
				add(fmt.Sprintf("Abolish market tax decree in %s (-4 energy)", pname),
					sim.Action{Kind: "issue_decree", FactionID: pid, Item: "market_tax", Quantity: -1})
			}
			if !p.Polity.LedPolityCurfew {
				add(fmt.Sprintf("Enact night curfew decree across %s (-4 energy)", pname),
					sim.Action{Kind: "issue_decree", FactionID: pid, Item: "curfew_on", Quantity: 1})
			} else {
				add(fmt.Sprintf("Lift night curfew decree across %s (-4 energy)", pname),
					sim.Action{Kind: "issue_decree", FactionID: pid, Item: "curfew_off", Quantity: 0})
			}
			if p.Polity.LedPolityTreasury > 0 && p.Polity.ReliefGrantAvailable {
				grant := 20
				if int64(grant) > p.Polity.LedPolityTreasury {
					grant = int(p.Polity.LedPolityTreasury)
				}
				add(fmt.Sprintf("Promulgate treasury disaster/livelihood relief grant (%d coins) in %s (-4 energy)", grant, pname),
					sim.Action{Kind: "issue_decree", FactionID: pid, Item: "relief_grant", Quantity: grant})
			}
			for _, id := range sortedItemIDs(p.Catalog.Items) {
				spec := p.Catalog.Items[id]
				if spec.Tradeable && (spec.CombatBonus > 0 || spec.Tier >= 4) {
					add(fmt.Sprintf("Promulgate contraband embargo on %s across %s (-4 energy)", id, pname),
						sim.Action{Kind: "issue_decree", FactionID: pid, Item: "embargo:" + id, Quantity: 1})
				}
			}
		}
	}

	if a.Energy < sim.MaxEnergy || a.Health < sim.MaxHealth {
		sleepTicks := 8
		if p.Clock.IsNight || sim.SleepWindowAligned(a.SleepTrait, p.Clock.Hour) || a.Energy < 40 || a.Health < 70 {
			sleepTicks = 20
		}
		if p.CurrentHouse != nil {
			add(fmt.Sprintf("Sleep and recuperate in %s (comfort %d, condition %d/100, up to %dm)", p.CurrentHouse.Name, p.CurrentHouse.Comfort, p.CurrentHouse.Condition, sleepTicks*15), sim.Action{Kind: "sleep", Quantity: sleepTicks})
		} else if a.Energy < 35 || a.Health < 55 || p.Clock.IsNight {
			add(fmt.Sprintf("Sleep rough outdoors at %s (reduced recovery, up to %dm)", a.Location, sleepTicks*15), sim.Action{Kind: "sleep", Quantity: sleepTicks})
		}
	}

	itemKeys := sortedItemIDs(p.Catalog.Items)
	if a.Thirst >= 2 {
		if canDrawFreeWater(p) {
			add(fmt.Sprintf("Drink fresh water at %s (-%d thirst)", a.Location, a.Thirst), sim.Action{Kind: "drink"})
		} else {
			for _, itemID := range itemKeys {
				spec := p.Catalog.Items[itemID]
				if spec.Potable && a.Items[itemID] >= 1 {
					add(fmt.Sprintf("Drink 1 %s (-%d thirst)", itemID, spec.ThirstRelief), sim.Action{Kind: "drink", Item: itemID})
				}
			}
		}
	}
	if a.Hunger > 0 {
		for _, itemID := range itemKeys {
			spec := p.Catalog.Items[itemID]
			if spec.Edible && a.Items[itemID] >= 1 && p.Catalog.ActorAcceptsFood(a, spec) {
				add(fmt.Sprintf("Eat 1 %s (%d nutrition units; surplus is retained)", itemID, spec.NutritionUnits), sim.Action{Kind: "eat", Item: itemID})
			}
		}
	}
	if a.Waste >= 3 {
		add(fmt.Sprintf("Use sanitation facility at %s (clear %d waste)", a.Location, a.Waste), sim.Action{Kind: "relieve"})
	}
	if a.Health < sim.MaxHealth || a.DiseaseID != "" {
		for _, itemID := range itemKeys {
			spec := p.Catalog.Items[itemID]
			if (spec.MedicinalHeal > 0 || spec.DiseaseCure > 0) && a.Items[itemID] >= 1 {
				add(fmt.Sprintf("Treat own wounds/illness with 1 %s (+%d health, cure %d)", itemID, spec.MedicinalHeal, spec.DiseaseCure), sim.Action{Kind: "medicate", Item: itemID})
			}
		}
	}

	if a.InsideHouse == "" && p.DistrictState != nil {
		infraKeys := make([]string, 0, len(p.DistrictState.Infrastructure))
		for k := range p.DistrictState.Infrastructure {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, infraID := range infraKeys {
			cond := p.DistrictState.Infrastructure[infraID]
			spec, exists := p.Catalog.Infrastructures[infraID]
			if !exists || cond >= spec.MaxCondition || a.Energy < spec.BuildEnergy || !holdsAllItems(a.Items, spec.BuildInputs) {
				continue
			}
			add(fmt.Sprintf("Reinforce %s in %s (45m, -%d energy, condition %d/%d)", spec.Name, a.Location, spec.BuildEnergy, cond, spec.MaxCondition),
				sim.Action{Kind: "maintain_infra", Item: infraID})
		}

		if a.Energy >= 10 && a.Health >= 50 && p.Tick >= a.RestUntil {
			for _, threat := range p.Catalog.Encounters {
				if act := p.DistrictState.ThreatActivity[threat.ID]; act > 5 {
					add(fmt.Sprintf("Hunt %s in %s (45m, -10 energy, threat %d, bounty up to %d coins)", threat.Name, a.Location, act, threat.BountyCoins),
						sim.Action{Kind: "hunt_threat", TargetID: threat.ID})
				}
			}
		}

		for _, rit := range p.Catalog.Rituals {
			if rit.NodeFeature != "" && !p.Map.DistrictHasFeature(a.Location, rit.NodeFeature) {
				continue
			}
			if a.Energy < rit.EnergyCost || a.Money < rit.MoneyCost || !holdsAllItems(a.Items, rit.Inputs) {
				continue
			}
			add(fmt.Sprintf("Conduct %s at %s (%dm, -%d energy, +%d morale)", rit.Name, a.Location, rit.Ticks*15, rit.EnergyCost, rit.MoraleBoost),
				sim.Action{Kind: "perform_ritual", RecipeID: rit.ID})
		}

		if a.Energy >= 8 {
			for _, we := range p.ActiveWorldEvents {
				if we.Category != "artifact_emergence" || we.ClaimedBy != "" || we.DistrictID != a.Location {
					continue
				}
				if we.RequiredPowerTier > 0 && p.Catalog.ActorPowerTier(a) < we.RequiredPowerTier {
					continue
				}
				add(fmt.Sprintf("Claim %s at %s (-8 energy; reward +%d %s, +%d coins)", we.Name, a.Location, we.RewardQty, we.RewardItem, we.RewardCoins),
					sim.Action{Kind: "claim_world_event", TargetID: we.InstanceID})
			}
		}
	}

	for _, r := range p.Catalog.Recipes {
		if p.RecipeBlockReason(r) == "" {
			bonus := r.BonusInput != "" && a.Items[r.BonusInput] >= 1
			expected := p.Catalog.RecipeYield(a, r, bonus)
			if r.Growth == nil {
				if whole, ok := p.ExpectedCraftYield[r.ID]; ok {
					expected = whole
				}
			}
			tier := p.Catalog.Items[r.OutputItem].Tier
			mastery := ""
			if r.Profession != "" {
				rank := a.ProfessionRanks[r.Profession]
				if rank != "" {
					mastery = fmt.Sprintf(", %s lv%d %s", r.Profession, a.Professions[r.Profession], rank)
				} else {
					mastery = fmt.Sprintf(", %s lv%d", r.Profession, a.Professions[r.Profession])
				}
			}
			duration := fmt.Sprintf("%dm work", r.Ticks*15)
			if r.Growth != nil {
				duration += fmt.Sprintf(", %dm maturation, care every %dm", r.Growth.DurationTicks*15, r.Growth.MaintenanceIntervalTicks*15)
			}
			craftLabel := fmt.Sprintf("%s (%s, -%d energy%s, tier %d, expect %d %s, you hold %d)",
				r.Label, duration, r.EnergyCost, mastery, tier, expected, r.OutputItem, a.Items[r.OutputItem])
			if dir != nil && dir.NextStep == "craft:"+r.ID {
				if r.ID == dir.RecipeID {
					craftLabel += fmt.Sprintf(" [active plan target: need %d more %s]", dir.DeficitToProduce, dir.Item)
				} else if r.ID == dir.ActiveRecipeID {
					craftLabel += fmt.Sprintf(" [active plan input for %s]", dir.RecipeID)
				}
			}
			add(craftLabel, sim.Action{Kind: "craft", RecipeID: r.ID})
		}
	}

	for _, h := range p.AccessibleHouses {
		if h.District == a.Location && (a.InsideHouse == "" || a.InsideHouse == h.ID) {
			repairItem := p.Catalog.EffectiveEnvironment().RepairItem
			if h.Condition < sim.MaxCondition && a.Energy >= 8 && a.Items[repairItem] >= 1 {
				add(fmt.Sprintf("Repair %s with 1 %s (45m, -8 energy, condition %d/100)", h.Name, repairItem, h.Condition), sim.Action{Kind: "repair_house", HouseID: h.ID})
			}
		}
	}

	embargoed := map[string]bool{}
	if p.Polity != nil {
		for _, emb := range p.Polity.EmbargoItems {
			embargoed[emb] = true
		}
	}
	type offerCandidate struct {
		spec     sim.ItemSpec
		qty      int
		price    int
		planSale bool
	}
	var offerItems []offerCandidate
	if allowed["offer"] {
		for _, itemID := range itemKeys {
			if embargoed[itemID] {
				continue
			}
			spec := p.Catalog.Items[itemID]
			if !spec.Tradeable || spec.BatchQty <= 0 {
				continue
			}
			if a.Plan != nil && a.Plan.Status == "active" && itemID == a.Plan.Item && a.Plan.RemainingToSell() > 0 {
				qty := min(spec.BatchQty, a.Plan.RemainingToSell())
				if qty > 0 && a.Items[itemID] >= qty {
					price := planBatchPrice(p.Catalog.SalePrice(a, spec.ID), qty, spec.BatchQty)
					offerItems = append(offerItems, offerCandidate{spec: spec, qty: qty, price: price, planSale: true})
					continue
				}
			}
			if a.Items[itemID] >= spec.OfferThreshold && a.Items[itemID] >= spec.BatchQty {
				offerItems = append(offerItems, offerCandidate{spec: spec, qty: spec.BatchQty, price: p.Catalog.SalePrice(a, spec.ID)})
			}
		}
	}

	grudgeTarget := make(map[string]bool, len(p.ActiveGrudgeTargets))
	for _, id := range p.ActiveGrudgeTargets {
		grudgeTarget[id] = true
	}
	if p.HasActiveLoan && a.Money > 0 {
		add("Repay active usury loan", sim.Action{Kind: "repay_loan"})
	}
	if p.OverdueBorrowerID != "" && p.OverdueHouseID != "" {
		add("Foreclose overdue collateral deed "+p.OverdueHouseID, sim.Action{
			Kind: "foreclose_collateral", TargetID: p.OverdueBorrowerID, HouseID: p.OverdueHouseID,
		})
	}
	env := p.Catalog.EffectiveEnvironment()
	if appeal, ok := appealAction(p); ok && env.Institutions.OversightEnabled() && a.IllicitGain == 0 && a.Energy >= 4 {
		add(fmt.Sprintf("File %s appeal for %s (-4 energy)", env.OversightName, appeal.TargetID), appeal)
	}
	if a.InsideHouse == "" && len(a.Guilds) > 0 && p.DistrictController == "" && a.Energy >= 5 {
		add("Claim district sovereignty for "+a.Guilds[0], sim.Action{Kind: "claim_turf", FactionID: a.Guilds[0], Location: a.Location})
	}

	if report, ok := informationAction(p); ok && report.Kind == "speak" && ruleOptionID(report, options) == "" {
		add(report.Text, report)
	}
	for _, action := range caseActions(p) {
		add(fmt.Sprintf("%s: %s", action.Kind, action.TargetID), action)
	}
	medItem := cheapestMedicinalItem(p.Catalog, a, itemKeys)
	for _, b := range p.Nearby {
		if b.Health < sim.MaxHealth && medItem != "" {
			add(fmt.Sprintf("Treat %s with 1 %s", b.Name, medItem), sim.Action{Kind: "heal", TargetID: b.ID, Item: medItem})
		}
		if b.Jailed {
			continue
		}
		if !b.Sleeping {
			add(fmt.Sprintf("Talk to %s (%s, %s, level %d)", b.Name, b.ID, b.Role, b.Level), sim.Action{Kind: "speak", TargetID: b.ID})
		}
		for _, cand := range offerItems {
			spec := cand.spec
			if spec.BuyerRequiredTag != "" && !sim.ActorHasTag(b.Tags, spec.BuyerRequiredTag) {
				continue
			}
			cur := "coins"
			if len(spec.AcceptedCurrencies) > 0 && !sim.ItemAcceptsCurrency(spec, "coins") {
				cur = spec.AcceptedCurrencies[0]
			}
			pending := false
			for _, o := range p.Offers {
				if o.Seller == a.ID && o.Buyer == b.ID && o.Item == spec.ID {
					pending = true
					break
				}
			}
			if !pending {
				actCur := ""
				if cur != "coins" {
					actCur = cur
				}
				offerLabel := fmt.Sprintf("Offer %d %s (tier %d) to %s for %d %s", cand.qty, spec.ID, spec.Tier, b.Name, cand.price, cur)
				if cand.planSale {
					offerLabel += fmt.Sprintf(" [active plan sale: %d %s remaining]", a.Plan.RemainingToSell(), spec.ID)
				}
				add(offerLabel, sim.Action{
					Kind: "offer", TargetID: b.ID, Item: spec.ID, Quantity: cand.qty, Price: cand.price, Currency: actCur,
				})
			}
		}
		bIsOutlaw := p.Catalog.IsOutlawTags(b.Tags)
		bIsEnforcer := p.Catalog.IsEnforcementTags(b.Tags)
		bIsOversight := p.Catalog.IsOversightTags(b.Tags)
		if b.Health > 0 && a.Energy >= 6 && a.Health >= 25 {
			add("Attack "+b.Name+" (-6 energy)", sim.Action{Kind: "attack", TargetID: b.ID})
		}
		if a.Health >= 65 && b.Health >= 65 && a.Energy >= 6 && p.Tick >= a.RestUntil && !b.Sleeping {
			add("Spar with "+b.Name+" (-6 energy)", sim.Action{Kind: "spar", TargetID: b.ID})
		}
		if a.Health >= 45 && a.Energy >= 5 && p.Tick >= a.RestUntil && !bIsOutlaw {
			add("Steal from "+b.Name+" (-5 energy)", sim.Action{Kind: "steal", TargetID: b.ID})
		}
		if env.Institutions.RacketeeringEnabled() && a.Health >= 50 && a.Energy >= 6 && p.Tick >= a.RestUntil && !bIsOutlaw && !b.Sleeping && !b.Jailed && b.FactionRelation != "allied" {
			add("Extort coercion fee from "+b.Name+" (-6 energy)", sim.Action{Kind: "extort", TargetID: b.ID})
		}
		if a.Health >= 55 && b.Health > 20 && a.Energy >= 8 && p.Tick >= a.RestUntil && !bIsOutlaw {
			add("Raid and plunder "+b.Name+" (-8 energy)", sim.Action{Kind: "raid", TargetID: b.ID})
		}
		if a.Money >= 4 && a.Energy >= 3 && !b.Sleeping {
			if bIsEnforcer {
				add(fmt.Sprintf("Offer hush bribe to %s %s (-3 energy)", env.AuthorityName, b.Name), sim.Action{Kind: "bribe", TargetID: b.ID})
			} else {
				for _, sec := range p.KnownSecrets {
					if sec.OwnerID == a.ID && !sec.Exposed && sim.ActorHasTag(sec.WitnessedBy, b.ID) {
						add(fmt.Sprintf("Offer hush bribe to witness %s (-3 energy)", b.Name), sim.Action{Kind: "bribe", TargetID: b.ID, Item: sec.ID})
						break
					}
				}
			}
		}
		if p.CurrentHouse != nil && p.CurrentHouse.OwnerID == a.ID {
			add(fmt.Sprintf("Transfer ownership deed of %s to %s (permanent property transfer)", p.CurrentHouse.Name, b.Name), sim.Action{
				Kind: "transfer_house", TargetID: b.ID, HouseID: p.CurrentHouse.ID,
			})
		}

		if !p.Married && !b.Married && b.KinshipToSelf == "" {
			add(fmt.Sprintf("Propose marriage alliance to %s", b.Name), sim.Action{
				Kind: "marry", TargetID: b.ID,
			})
		}
		if a.Energy >= 2 && a.Trust[b.ID] >= 3 && b.KinshipToSelf == "" {
			add(fmt.Sprintf("Swear kinship/patronage bond with %s", b.Name), sim.Action{
				Kind: "swear_bond", TargetID: b.ID,
			})
		}
		if grudgeTarget[b.ID] && a.Health >= 25 && a.Energy >= 6 && b.Health > 0 {
			add(fmt.Sprintf("Avenge blood feud against %s (-6 energy)", b.Name), sim.Action{
				Kind: "avenge_feud", TargetID: b.ID,
			})
		}
		if env.Institutions.EnforcementEnabled() && !bIsEnforcer && !bIsOversight && !bIsOutlaw {
			add(fmt.Sprintf("Frame %s in %s docket (-4 energy)", b.Name, env.AuthorityName), sim.Action{
				Kind: "frame_suspect", TargetID: b.ID,
			})
		}
	}

	for _, action := range mediationActions(p) {
		add(fmt.Sprintf("Propose or accept %d coins to settle known feud %s", action.Price, action.Text), action)
	}
	selfEffTags := p.EffectiveTags()
	for _, o := range p.Offers {
		if o.ResponderID() != a.ID {
			continue
		}
		other := o.Counterparty(a.ID)
		if p.OfferAcceptanceBlockReason(o) == "" {
			for _, b := range p.Nearby {
				if b.ID == other && !b.Jailed {
					add(fmt.Sprintf("Accept offer %d: buyer %s pays %d %s to seller %s for %d %s", o.ID, o.Buyer, o.Price, o.Currency, o.Seller, o.Quantity, o.Item), sim.Action{Kind: "accept", TargetID: other, OfferID: o.ID})
				}
			}
		}
		add(fmt.Sprintf("Reject offer %d (%d %s)", o.ID, o.Quantity, o.Item), sim.Action{Kind: "reject", TargetID: other, OfferID: o.ID})
	}

	for _, action := range sim.ContinuityActions(p) {
		add(continuityOptionLabel(p, action, dir), action)
	}
	pendingBorrow := false
	for _, request := range p.LoanRequests {
		if request.BorrowerID == a.ID {
			pendingBorrow = true
			add("Cancel pending loan request to "+request.LenderID, sim.Action{Kind: "cancel_loan", TargetID: request.LenderID})
		}
		if request.LenderID == a.ID {
			for _, nb := range p.Nearby {
				if nb.ID == request.BorrowerID && !nb.Jailed {
					if a.Money >= request.Principal {
						add(fmt.Sprintf("Accept loan request from %s: %d coins, interest %d per mille/day, term %d ticks, collateral %s", nb.Name, request.Principal, request.RatePermille, request.TermTicks, request.CollateralHouse), sim.Action{Kind: "accept_loan", TargetID: nb.ID})
					}
					add("Reject loan request from "+nb.Name, sim.Action{Kind: "reject_loan", TargetID: nb.ID})
				}
			}
		}
	}
	if !p.HasActiveLoan && !pendingBorrow && a.Money < 10 {
		collateral := ""
		if deed, known := p.KnownHouse(a.HomeID); known && deed.OwnerID == a.ID {
			collateral = deed.ID
		}
		for _, peer := range p.Nearby {
			if canRequestLoan(peer, p.CreditTerms) {
				add(fmt.Sprintf("Request 12 coins from %s: interest %d per mille/day, term %d ticks, collateral %s", peer.ID, p.CreditTerms.DailyLoanInterestPermille, p.CreditTerms.LoanTermTicks, collateral), sim.Action{Kind: "borrow_loan", TargetID: peer.ID})
			}
		}
	}
	if len(p.Catalog.CommChannels) > 0 {
		chKeys := make([]string, 0, len(p.Catalog.CommChannels))
		for k := range p.Catalog.CommChannels {
			chKeys = append(chKeys, k)
		}
		sort.Strings(chKeys)
		tags := selfEffTags
		for _, chID := range chKeys {
			ch := p.Catalog.CommChannels[chID]
			if a.Energy < ch.EnergyCost {
				continue
			}
			if len(ch.GrantedByTags) > 0 {
				granted := false
				for _, gt := range ch.GrantedByTags {
					if sim.ActorHasTag(tags, gt) {
						granted = true
						break
					}
				}
				if !granted {
					continue
				}
			}
			if ch.Broadcast {
				add(fmt.Sprintf("Broadcast message over %s (range %dkm, -%d energy)", ch.Name, ch.RangeKm, ch.EnergyCost),
					sim.Action{Kind: "transmit_message", Channel: ch.ID})
			} else {
				peerSet := map[string]bool{}
				for peerID := range a.Trust {
					if peerID != "" && peerID != a.ID {
						peerSet[peerID] = true
					}
				}
				for _, m := range a.Memories {
					if m.Source != "" && m.Source != "world" && m.Source != a.ID {
						peerSet[m.Source] = true
					}
					if m.TargetID != "" && m.TargetID != a.ID {
						peerSet[m.TargetID] = true
					}
				}
				peers := make([]string, 0, len(peerSet))
				for pid := range peerSet {
					peers = append(peers, pid)
				}
				sort.Strings(peers)
				for _, pid := range peers {
					add(fmt.Sprintf("Transmit message to %s via %s (-%d energy)", pid, ch.Name, ch.EnergyCost),
						sim.Action{Kind: "transmit_message", Channel: ch.ID, TargetID: pid})
				}
			}
		}
	}

	// Technical knowledge discovery, teaching, and local facility adoption (strictly scoped to eligible/known techs).
	if len(p.Catalog.Techs) > 0 {
		techKeys := make([]string, 0, len(p.Catalog.Techs))
		for k := range p.Catalog.Techs {
			techKeys = append(techKeys, k)
		}
		sort.Strings(techKeys)
		for _, techID := range techKeys {
			tech := p.Catalog.Techs[techID]
			if ok, _ := sim.CanResearchTechWithKnown(p.Catalog, a, tech, p.KnownTechs); ok {
				ticks := tech.ResearchTicks
				if ticks < 1 {
					ticks = 1
				}
				add(fmt.Sprintf("Research %s (%dm, -%d energy)", tech.Name, ticks*15, tech.ResearchEnergyCost),
					sim.Action{Kind: "research_tech", TechID: tech.ID})
			}
		}

		knownSorted := append([]string(nil), p.KnownTechs...)
		sort.Strings(knownSorted)
		effTags := selfEffTags
		nearbySet := make(map[string]bool, len(p.Nearby))
		for _, nb := range p.Nearby {
			nearbySet[nb.ID] = true
		}

		for _, techID := range knownSorted {
			tech, exists := p.Catalog.Techs[techID]
			if !exists {
				continue
			}
			for _, nb := range p.Nearby {
				if nb.Jailed || nb.Sleeping {
					continue
				}
				add(fmt.Sprintf("Teach %s to %s", tech.Name, nb.Name),
					sim.Action{Kind: "teach_knowledge", TechID: tech.ID, TargetID: nb.ID})
			}
			if len(p.Catalog.CommChannels) > 0 {
				chKeys := make([]string, 0, len(p.Catalog.CommChannels))
				for k := range p.Catalog.CommChannels {
					chKeys = append(chKeys, k)
				}
				sort.Strings(chKeys)
				peerSet := map[string]bool{}
				for peerID := range a.Trust {
					if peerID != "" && peerID != a.ID && !nearbySet[peerID] {
						peerSet[peerID] = true
					}
				}
				for _, m := range a.Memories {
					if m.Source != "" && m.Source != "world" && m.Source != a.ID && !nearbySet[m.Source] {
						peerSet[m.Source] = true
					}
					if m.TargetID != "" && m.TargetID != a.ID && !nearbySet[m.TargetID] {
						peerSet[m.TargetID] = true
					}
				}
				peers := make([]string, 0, len(peerSet))
				for pid := range peerSet {
					peers = append(peers, pid)
				}
				sort.Strings(peers)
				for _, chID := range chKeys {
					ch := p.Catalog.CommChannels[chID]
					if a.Energy < ch.EnergyCost {
						continue
					}
					if len(ch.GrantedByTags) > 0 {
						granted := false
						for _, gt := range ch.GrantedByTags {
							if sim.ActorHasTag(effTags, gt) {
								granted = true
								break
							}
						}
						if !granted {
							continue
						}
					}
					for _, pid := range peers {
						add(fmt.Sprintf("Teach %s to %s via %s (-%d energy)", tech.Name, pid, ch.Name, ch.EnergyCost),
							sim.Action{Kind: "teach_knowledge", TechID: tech.ID, TargetID: pid, Channel: ch.ID})
					}
				}
			}

			if a.Energy >= tech.AdoptEnergyCost && holdsAllItems(a.Items, tech.AdoptInputs) {
				adoptTicks := tech.AdoptTicks
				if adoptTicks < 1 {
					adoptTicks = 1
				}
				if a.InsideHouse != "" {
					if p.CurrentHouse != nil && p.CurrentHouse.CanAccess(a.ID) {
						if tech.FacilityName == "" || !p.CurrentHouse.HasFacility(tech.FacilityName) {
							add(fmt.Sprintf("Adopt %s in %s (%dm)", tech.Name, p.CurrentHouse.Name, adoptTicks*15),
								sim.Action{Kind: "adopt_facility", TechID: tech.ID, HouseID: p.CurrentHouse.ID})
						}
					}
				} else {
					if a.VenueID != "" {
						if p.CurrentVenue == nil || tech.GrantsVenueFeature == "" || !sim.ActorHasTag(p.CurrentVenue.Features, tech.GrantsVenueFeature) {
							add(fmt.Sprintf("Adopt %s at venue %s (%dm)", tech.Name, a.VenueID, adoptTicks*15),
								sim.Action{Kind: "adopt_facility", TechID: tech.ID, Location: a.VenueID})
						}
					}
					for _, h := range p.AccessibleHouses {
						if h.District == a.Location && (h.VenueID == "" || a.VenueID == "" || h.VenueID == a.VenueID) {
							if tech.FacilityName == "" || !h.HasFacility(tech.FacilityName) {
								add(fmt.Sprintf("Adopt %s in %s (%dm)", tech.Name, h.Name, adoptTicks*15),
									sim.Action{Kind: "adopt_facility", TechID: tech.ID, HouseID: h.ID})
							}
						}
					}
				}
			}
		}
	}
	// Dual-use currency refinement & state-slot absorption.
	if allowed["refine_currency_item"] || allowed["absorb_item_slot"] {
		slotKeys := make([]string, 0, len(p.Catalog.StateSlots))
		for sid := range p.Catalog.StateSlots {
			slotKeys = append(slotKeys, sid)
		}
		sort.Strings(slotKeys)
		for _, itemID := range sortedItemIDs(p.Catalog.Items) {
			if a.Items[itemID] <= 0 {
				continue
			}
			spec := p.Catalog.Items[itemID]
			if allowed["refine_currency_item"] && spec.DualUseCurrency != "" && spec.DualUseRate > 0 {
				add(fmt.Sprintf("Refine 1 %s into %d %s", itemID, spec.DualUseRate, spec.DualUseCurrency),
					sim.Action{Kind: "refine_currency_item", Item: itemID, Quantity: 1})
			}
			if allowed["absorb_item_slot"] && (spec.DualUseCurrency != "" || spec.DualUseRate > 0) {
				if len(slotKeys) == 0 {
					add(fmt.Sprintf("Absorb 1 %s to replenish qi", itemID),
						sim.Action{Kind: "absorb_item_slot", Item: itemID, Quantity: 1, TargetID: "qi"})
				} else {
					for _, sid := range slotKeys {
						slotSpec := p.Catalog.StateSlots[sid]
						if slotSpec.Max <= 0 || a.StateSlots[sid] < slotSpec.Max {
							add(fmt.Sprintf("Absorb 1 %s to replenish %s", itemID, sid),
								sim.Action{Kind: "absorb_item_slot", Item: itemID, Quantity: 1, TargetID: sid})
						}
					}
				}
			}
		}
	}

	// Flow-grid edge repair and sabotage.
	if len(p.FlowEdges) > 0 && (allowed["repair_flow_edge"] || allowed["sever_flow_edge"]) {
		for _, edge := range p.FlowEdges {
			if edge.ID == "" {
				continue
			}
			if edge.Severed && allowed["repair_flow_edge"] {
				add(fmt.Sprintf("Repair severed flow edge %s (%s -> %s)", edge.ID, edge.FromNodeID, edge.ToNodeID),
					sim.Action{Kind: "repair_flow_edge", TargetID: edge.ID})
			} else if !edge.Severed && allowed["sever_flow_edge"] {
				add(fmt.Sprintf("Sever flow edge %s (%s -> %s)", edge.ID, edge.FromNodeID, edge.ToNodeID),
					sim.Action{Kind: "sever_flow_edge", TargetID: edge.ID})
			}
		}
	}

	// Fundamental world-law rewriting (explicitly gated by Catalog.TagActions["..."]["rewrite_world_law"]).
	if allowed["rewrite_world_law"] {
		authorizedLaw := false
		for _, tag := range selfEffTags {
			if sim.ActorHasTag(p.Catalog.TagActions[tag], "rewrite_world_law") {
				authorizedLaw = true
				break
			}
		}
		if authorizedLaw {
			lawKeys := []string{"firearms_disabled"}
			for k := range p.WorldLaws {
				if !sim.ActorHasTag(lawKeys, k) {
					lawKeys = append(lawKeys, k)
				}
			}
			sort.Strings(lawKeys)
			for _, law := range lawKeys {
				nextVal := 1
				if p.WorldLaws[law] > 0 {
					nextVal = 0
				}
				add(fmt.Sprintf("Rewrite world law %s to %d", law, nextVal),
					sim.Action{Kind: "rewrite_world_law", Item: law, Quantity: nextVal})
			}
		}
	}

	// Independent skill practice (proficiency grinding) when actor has a known skill below max level.
	if len(p.Catalog.Skills) > 0 && allowed["practice_skill"] && a.Energy >= 4 {
		skillKeys := make([]string, 0, len(a.Skills))
		for sid, lv := range a.Skills {
			if lv >= 1 {
				skillKeys = append(skillKeys, sid)
			}
		}
		sort.Strings(skillKeys)
		for _, sid := range skillKeys {
			sk, ok := p.Catalog.Skills[sid]
			if !ok || a.Skills[sid] >= sk.EffectiveMaxLevel() {
				continue
			}
			add(fmt.Sprintf("Practice %s (Lv.%d/%d, -4 energy)", sk.Name, a.Skills[sid], sk.EffectiveMaxLevel()),
				sim.Action{Kind: "practice_skill", SkillID: sid})
		}
	}

	// Independent skill mentorship (teacher level >= 2 and strictly higher than nearby student).
	if len(p.Catalog.Skills) > 0 && allowed["teach_knowledge"] && a.Energy >= 4 {
		skillKeys := make([]string, 0, len(a.Skills))
		for sid, lv := range a.Skills {
			if lv >= 2 {
				skillKeys = append(skillKeys, sid)
			}
		}
		sort.Strings(skillKeys)
		for _, sid := range skillKeys {
			sk, ok := p.Catalog.Skills[sid]
			if !ok {
				continue
			}
			teacherLv := a.Skills[sid]
			for _, nb := range p.Nearby {
				if nb.Jailed || nb.Sleeping {
					continue
				}
				if nb.Skills[sid] < teacherLv {
					add(fmt.Sprintf("Mentor %s in %s (Lv.%d -> Lv.%d, -4 energy)", nb.Name, sk.Name, nb.Skills[sid], min(teacherLv, nb.Skills[sid]+1)),
						sim.Action{Kind: "teach_knowledge", SkillID: sid, TargetID: nb.ID})
				}
			}
		}
	}

	// Declarative form/realm transitions (e.g. cultivation breakthroughs) when all preconditions & costs are satisfied.
	if len(p.Catalog.Transitions) > 0 && allowed["attempt_transition"] {
		transKeys := make([]string, 0, len(p.Catalog.Transitions))
		for k := range p.Catalog.Transitions {
			transKeys = append(transKeys, k)
		}
		sort.Strings(transKeys)
		for _, tid := range transKeys {
			trans := p.Catalog.Transitions[tid]
			if trans.ID == "" {
				trans.ID = tid
			}
			if trans.SuccessFormID != "" && a.FormID == trans.SuccessFormID {
				continue
			}
			if ok, _ := sim.CanAttemptTransition(a, trans); ok {
				name := trans.Name
				if name == "" {
					name = trans.ID
				}
				ticks := trans.Ticks
				if ticks < 1 {
					ticks = 1
				}
				add(fmt.Sprintf("Attempt transition %s (%dm)", name, ticks*15),
					sim.Action{Kind: "attempt_transition", TransitionID: trans.ID})
			}
		}
	}

	addDeclarativeActionOptions(p, allowed, add)
	addGoldenFingerOptions(p, allowed, add)
	for _, itemID := range itemKeys {
		if ok, _ := sim.CanEquipItem(p.Catalog, a, itemID); ok {
			slot := p.Catalog.Items[itemID].EquipSlot
			if a.Equipped[slot] != itemID {
				add(fmt.Sprintf("Equip %s into %s", itemID, slot), sim.Action{Kind: "equip", Item: itemID})
			}
		}
	}
	if len(a.Equipped) > 0 {
		slots := make([]string, 0, len(a.Equipped))
		for slot := range a.Equipped {
			slots = append(slots, slot)
		}
		sort.Strings(slots)
		for _, slot := range slots {
			if itemID := a.Equipped[slot]; itemID != "" {
				add(fmt.Sprintf("Unequip %s from %s", itemID, slot), sim.Action{Kind: "unequip", TargetID: slot, Item: itemID})
			}
		}
	}
	addPartyOptions(p, allowed, add)
	addDiplomacyOptions(p, allowed, add)
	addAmbitionOptions(p, allowed, add)
	return options
}

func actionSpecCostSummary(spec sim.ActionSpec) string {
	var parts []string
	if spec.Ticks > 1 {
		parts = append(parts, fmt.Sprintf("%dt", spec.Ticks))
	}
	if spec.Dormant {
		parts = append(parts, "dormant")
	}
	if spec.EnergyCost > 0 {
		parts = append(parts, fmt.Sprintf("-%d energy", spec.EnergyCost))
	}
	if spec.MoneyCost > 0 {
		parts = append(parts, fmt.Sprintf("-%d coins", spec.MoneyCost))
	}
	for _, k := range sortedIntMapKeys(spec.SlotCosts) {
		if spec.SlotCosts[k] > 0 {
			parts = append(parts, fmt.Sprintf("-%d %s", spec.SlotCosts[k], k))
		}
	}
	for _, k := range sortedIntMapKeys(spec.CurrencyCosts) {
		if spec.CurrencyCosts[k] > 0 {
			parts = append(parts, fmt.Sprintf("-%d %s", spec.CurrencyCosts[k], k))
		}
	}
	for _, k := range sortedIntMapKeys(spec.ItemCosts) {
		if spec.ItemCosts[k] > 0 {
			parts = append(parts, fmt.Sprintf("-%d %s", spec.ItemCosts[k], k))
		}
	}
	if spec.ResourcePool != "" {
		draw := spec.ResourceDraw
		if draw < 1 {
			draw = 1
		}
		parts = append(parts, fmt.Sprintf("-%d %s", draw, spec.ResourcePool))
	}
	for _, k := range sortedIntMapKeys(spec.ItemGains) {
		if spec.ItemGains[k] > 0 {
			parts = append(parts, fmt.Sprintf("+%d %s", spec.ItemGains[k], k))
		}
	}
	if len(parts) == 0 {
		return "instant"
	}
	return strings.Join(parts, ", ")
}

func sortedIntMapKeys(m map[string]int) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func addDeclarativeActionOptions(p sim.Perception, allowed map[string]bool, add func(string, sim.Action)) {
	if len(p.Catalog.ActionSpecs) == 0 {
		return
	}
	a := p.Self
	specKeys := make([]string, 0, len(p.Catalog.ActionSpecs))
	for k := range p.Catalog.ActionSpecs {
		specKeys = append(specKeys, k)
	}
	sort.Strings(specKeys)
	for _, k := range specKeys {
		spec := p.Catalog.ActionSpecs[k]
		actionID := spec.ID
		if actionID == "" {
			actionID = k
		}
		if actionID == "" || !allowed[actionID] {
			continue
		}
		if p.ActionSpecBlockReason(spec) != "" {
			continue
		}
		name := spec.Name
		if name == "" {
			name = actionID
		}
		summary := actionSpecCostSummary(spec)
		switch spec.TargetKind {
		case "", "self":
			add(fmt.Sprintf("%s (%s)", name, summary), sim.Action{Kind: actionID})
		case "venue":
			if spec.RangeKm <= 0 {
				if a.VenueID != "" {
					add(fmt.Sprintf("%s at %s (%s)", name, a.VenueID, summary), sim.Action{Kind: actionID, Location: a.VenueID})
				} else {
					for _, v := range p.Map.Venues {
						if v.DistrictID == a.Location {
							add(fmt.Sprintf("%s at %s (%s)", name, v.Name, summary), sim.Action{Kind: actionID, Location: v.ID})
						}
					}
				}
			} else if srcCoord, ok := p.Map.GeoCoordOfLocation(a.Location, a.VenueID); ok {
				for _, v := range p.Map.Venues {
					if dstCoord, ok2 := p.Map.GeoCoordOfLocation(v.DistrictID, v.ID); ok2 && sim.GeoDistanceKm(srcCoord, dstCoord) <= spec.RangeKm {
						add(fmt.Sprintf("%s at %s (%s)", name, v.Name, summary), sim.Action{Kind: actionID, Location: v.ID})
					}
				}
			} else if a.VenueID != "" {
				add(fmt.Sprintf("%s at %s (%s)", name, a.VenueID, summary), sim.Action{Kind: actionID, Location: a.VenueID})
			}
		case "district":
			if spec.RangeKm <= 0 {
				if a.Location != "" {
					add(fmt.Sprintf("%s in %s (%s)", name, a.Location, summary), sim.Action{Kind: actionID, Location: a.Location})
				}
			} else if srcCoord, ok := p.Map.GeoCoordOfLocation(a.Location, a.VenueID); ok {
				for _, d := range p.Map.Districts {
					if dstCoord, ok2 := p.Map.GeoCoordOfLocation(d.ID, ""); ok2 && sim.GeoDistanceKm(srcCoord, dstCoord) <= spec.RangeKm {
						add(fmt.Sprintf("%s in %s (%s)", name, d.Name, summary), sim.Action{Kind: actionID, Location: d.ID})
					}
				}
			} else if a.Location != "" {
				add(fmt.Sprintf("%s in %s (%s)", name, a.Location, summary), sim.Action{Kind: actionID, Location: a.Location})
			}
		case "actor":
			for _, b := range p.Nearby {
				if !b.Jailed {
					add(fmt.Sprintf("%s -> %s (%s)", name, b.Name, summary), sim.Action{Kind: actionID, TargetID: b.ID})
				}
			}
		case "cohort":
			for _, c := range p.LocalCohorts {
				if c.District == a.Location && c.Population > 0 {
					add(fmt.Sprintf("%s -> %s (%s)", name, c.Name, summary), sim.Action{Kind: actionID, TargetID: c.ID})
				}
			}
		}
	}
}

func sortedItemIDs(items map[string]sim.ItemSpec) []string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func planBatchPrice(batchPrice, qty, batchQty int) int {
	if batchPrice <= 0 || qty <= 0 {
		return 0
	}
	if batchQty <= 0 || qty >= batchQty {
		return batchPrice
	}
	return max(1, (batchPrice*qty+batchQty-1)/batchQty)
}

func continuityOptionLabel(p sim.Perception, action sim.Action, dir *sim.PlanDirectiveView) string {
	a := p.Self
	switch action.Kind {
	case "start_plan":
		if r, ok := p.Catalog.RecipeByID(action.RecipeID); ok {
			qty := max(r.OutputQty, p.Catalog.Items[r.OutputItem].BatchQty)
			return fmt.Sprintf("Start personal plan: produce and sell %d %s via %s (you hold %d)", qty, r.OutputItem, r.ID, a.Items[r.OutputItem])
		}
	case "abandon_plan":
		if a.Plan != nil {
			return fmt.Sprintf("Abandon active plan (%s: sold %d/%d %s, you hold %d)", a.Plan.RecipeID, a.Plan.Sold, a.Plan.Quantity, a.Plan.Item, a.Items[a.Plan.Item])
		}
	case "propose_agreement":
		label := fmt.Sprintf("Propose delivery agreement to %s: %d %s for %d coins", action.TargetID, action.Quantity, action.Item, action.Price)
		if dir != nil && action.Item == dir.Item && dir.RemainingToSell > 0 {
			label += fmt.Sprintf(" [active plan sale: %d %s remaining]", dir.RemainingToSell, dir.Item)
		}
		return label
	case "fulfill_agreement":
		label := fmt.Sprintf("Fulfill delivery agreement #%d to %s: %d %s for %d coins", action.AgreementID, action.TargetID, action.Quantity, action.Item, action.Price)
		if dir != nil && action.Item == dir.Item && dir.RemainingToSell > 0 {
			label += " [active plan delivery]"
		}
		return label
	case "accept_agreement":
		return fmt.Sprintf("Accept delivery agreement #%d from %s: %d %s for %d coins", action.AgreementID, action.TargetID, action.Quantity, action.Item, action.Price)
	case "reject_agreement":
		return fmt.Sprintf("Reject delivery agreement #%d from %s", action.AgreementID, action.TargetID)
	}
	return fmt.Sprintf("%s: target=%s recipe=%s item=%s quantity=%d price=%d agreement=%d", action.Kind, action.TargetID, action.RecipeID, action.Item, action.Quantity, action.Price, action.AgreementID)
}

func holdsAllItems(inv map[string]int, req map[string]int) bool {
	for itemID, qty := range req {
		if inv[itemID] < qty {
			return false
		}
	}
	return true
}

func canDrawFreeWater(p sim.Perception) bool {
	if p.DistrictState == nil {
		return false
	}
	for id, spec := range p.Catalog.Resources {
		if spec.PotableWater && p.DistrictState.AvailableResource(id) > 0 {
			return true
		}
	}
	return false
}

func cheapestMedicinalItem(cat sim.Catalog, a sim.Actor, itemKeys []string) string {
	best := ""
	bestHeal := 0
	for _, id := range itemKeys {
		spec := cat.Items[id]
		if (spec.MedicinalHeal > 0 || spec.DiseaseCure > 0) && a.Items[id] >= 1 {
			if best == "" || spec.MedicinalHeal < bestHeal {
				best = id
				bestHeal = spec.MedicinalHeal
			}
		}
	}
	return best
}

func totalEdibleItems(cat sim.Catalog, a sim.Actor) int {
	return int(cat.AvailableNutrition(a))
}

// RuleSelector is explicitly the offline baseline, never a Jev substitute.
type RuleSelector struct{ trace *runtrace.Writer }

func (r RuleSelector) Select(_ context.Context, p sim.Perception, options []Option) (string, error) {
	var id string
	var unavailable []sim.Action
	(rulesPolicy{options: options}).preferences(p, func(desired sim.Action) bool {
		id = ruleOptionID(desired, options)
		if id == "" && r.trace != nil {
			unavailable = append(unavailable, desired)
		}
		return id != ""
	})
	if len(unavailable) > 0 {
		if err := r.trace.Record("rule_unavailable", p.Tick, p.Self.ID, map[string]any{"preferences": unavailable}); err != nil {
			return "", err
		}
	}
	if id == "" {
		return "", fmt.Errorf("no available action matches the rules for %s at tick %d", p.Self.ID, p.Tick)
	}
	return id, nil
}

func ruleOptionID(desired sim.Action, options []Option) string {
	for _, o := range options {
		a := o.Action
		if a.Kind == desired.Kind &&
			((desired.Kind != "speak" && desired.Kind != "transmit_message" && desired.Kind != "channel_message") ||
				desired.Statement == nil && desired.BasisEventID == 0 ||
				a.BasisEventID == desired.BasisEventID && sameStatement(a.Statement, desired.Statement)) &&
			(desired.Channel == "" || a.Channel == desired.Channel) &&
			a.Location == desired.Location &&
			a.HouseID == desired.HouseID &&
			a.FactionID == desired.FactionID &&
			a.ProfessionID == desired.ProfessionID &&
			a.RecipeID == desired.RecipeID &&
			(desired.TechID == "" || a.TechID == desired.TechID) &&
			(desired.SkillID == "" || a.SkillID == desired.SkillID) &&
			(desired.Currency == "" || a.Currency == desired.Currency) &&
			(desired.TransitionID == "" || a.TransitionID == desired.TransitionID) &&
			a.OfferID == desired.OfferID &&
			a.AgreementID == desired.AgreementID &&
			(desired.Kind != "mediate_feud" || a.Text == desired.Text && a.Price == desired.Price) &&
			((desired.Kind != "gf_exchange" && desired.Kind != "gf_simulate") || desired.Text == "" || a.Text == desired.Text) &&
			((desired.Kind != "propose_agreement" && desired.Kind != "offer" && desired.Kind != "rewrite_world_law") || (a.Price == desired.Price && a.Quantity == desired.Quantity)) &&
			(desired.Care == nil || a.Care != nil && *a.Care == *desired.Care) &&
			(desired.Item == "" || a.Item == desired.Item) &&
			(a.TargetID == desired.TargetID || a.OfferID != 0) {
			return o.ID
		}
	}
	return ""
}

func sameStatement(a, b *sim.Statement) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

type speechDraft struct {
	Text         string
	Statement    *sim.Statement
	BasisEventID int64
}

// buildCommunicationAction attaches deterministic structured speech, channel
// transmission, or decree text directly onto system-defined candidate actions.
func buildCommunicationAction(p sim.Perception, a sim.Action) sim.Action {
	if a.Kind == "issue_decree" {
		polName := a.FactionID
		if p.Polity != nil && p.Polity.LedPolityName != "" {
			polName = p.Polity.LedPolityName
		}
		a.Text = fmt.Sprintf("Proclamation of %s by %s: enact %s (%d).", polName, p.Self.Name, a.Item, a.Quantity)
		return a
	}
	if a.Kind != "speak" && a.Kind != "transmit_message" {
		return a
	}
	alreadyToldTarget := func(rootID int64) bool {
		if rootID == 0 {
			return false
		}
		for _, past := range p.Self.Memories {
			if (past.Kind == "speech" || past.Kind == "channel_message") && past.Source == p.Self.ID && past.RootEventID == rootID {
				if a.TargetID == "" || past.TargetID == a.TargetID {
					return true
				}
			}
		}
		return false
	}
	for i := len(p.Self.Memories) - 1; i >= 0; i-- {
		m := p.Self.Memories[i]
		if p.Tick-m.Tick > sim.TicksPerDay || alreadyToldTarget(m.RootEventID) {
			continue
		}
		if draft, ok := reportFromMemory(m, p.Self, a.TargetID); ok && secretReportAllowed(p, m.RootEventID, draft.Statement) {
			a.Text = draft.Text
			a.Statement = draft.Statement
			a.BasisEventID = draft.BasisEventID
			return a
		}
	}
	a.Text = fmt.Sprintf("Checking in from %s; supplies=%d, energy=%d.", p.Self.Location, totalEdibleItems(p.Catalog, p.Self), p.Self.Energy)
	return a
}

// reportFromMemory gives planning and speech the same account of one memory.
func secretReportAllowed(p sim.Perception, root int64, stmt *sim.Statement) bool {
	switch stmt.Topic {
	case "secret_caught_red_handed", "secret_discovered", "secret_exposed":
		for _, secret := range p.KnownSecrets {
			if secret.ID == stmt.ObjectID {
				return secret.Exposed || secret.HolderStances[p.Self.ID] == "confront"
			}
		}
		return false
	case "corruption", "wrongful_conviction":
		for _, secret := range p.KnownSecrets {
			if secret.RootEventID == root && secret.OwnerID == stmt.SubjectID {
				return secret.Exposed || secret.HolderStances[p.Self.ID] == "confront"
			}
		}
	}
	return true
}

func reportFromMemory(m sim.Memory, self sim.Actor, listenerID string) (speechDraft, bool) {
	if m.Observation != nil {
		switch m.Observation.Kind {
		case "item_produced":
			q := m.Observation.Quantity
			text := fmt.Sprintf("I saw %d %s produced at %s.", q, m.Observation.Item, m.Observation.Location)
			if m.Source == self.ID {
				text = fmt.Sprintf("I produced %d %s at %s.", q, m.Observation.Item, m.Observation.Location)
			}
			return speechDraft{
				Text:         text,
				BasisEventID: m.EventID,
				Statement:    &sim.Statement{Topic: "item_produced", Item: m.Observation.Item, Location: m.Observation.Location, Quantity: q},
			}, true
		case "macro_anomaly":
			mod := m.Observation.Modality
			if mod == "" {
				mod = "eyewitness"
			}
			return speechDraft{
				Text:         fmt.Sprintf("I observed %s (%s) at %s (intensity %d).", m.Observation.SubjectID, mod, m.Observation.Location, m.Observation.Quantity),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "macro_anomaly",
					SubjectID: m.Observation.SubjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		case "unlawful_attack":
			o := m.Observation
			if o.SubjectID == self.ID || o.SubjectID == listenerID {
				return speechDraft{}, false
			}
			return speechDraft{Text: fmt.Sprintf("I saw %s attack %s at %s.", o.SubjectID, o.ObjectID, o.Location), BasisEventID: m.EventID,
				Statement: &sim.Statement{Topic: o.Kind, SubjectID: o.SubjectID, ObjectID: o.ObjectID, Location: o.Location, Quantity: o.Quantity}}, true
		case "corruption":
			if m.Observation.SubjectID == "" || m.Observation.SubjectID == self.ID || m.Observation.SubjectID == listenerID || self.IllicitGain > 0 {
				return speechDraft{}, false
			}
			return speechDraft{
				Text:         fmt.Sprintf("I witnessed %s taking %d illicit bribe coins at %s.", m.Observation.SubjectID, m.Observation.Quantity, m.Observation.Location),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "corruption",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		case "wrongful_conviction":
			if m.Observation.ObjectID == "" || m.Observation.SubjectID == self.ID || m.Observation.SubjectID == listenerID {
				return speechDraft{}, false
			}
			return speechDraft{
				Text:         fmt.Sprintf("I witnessed %s wrongfully framing %s in %s (case #%d).", m.Observation.SubjectID, m.Observation.ObjectID, m.Observation.Location, m.Observation.CaseID),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "wrongful_conviction",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					CaseID:    m.Observation.CaseID,
					Location:  m.Observation.Location,
					Quantity:  1,
				},
			}, true
		case "ambient_shift":
			mod := m.Observation.Modality
			if mod == "" {
				mod = "eyewitness"
			}
			return speechDraft{
				Text:         fmt.Sprintf("I perceived %s shift (%s) at %s (level %d).", m.Observation.Item, mod, m.Observation.Location, m.Observation.Quantity),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "ambient_shift",
					SubjectID: m.Observation.SubjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		case "facility_adopted":
			return speechDraft{
				Text:         fmt.Sprintf("I witnessed %s adopting %s at %s.", m.Observation.SubjectID, m.Observation.Item, m.Observation.ObjectID),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "facility_adopted",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  1,
				},
			}, true
		case "faction_war_declared", "alliance_formed", "alliance_broken", "peace_treaty_signed":
			o := m.Observation
			if o.SubjectID == "" || o.ObjectID == "" {
				return speechDraft{}, false
			}
			text := m.Text
			if text == "" {
				text = fmt.Sprintf("I witnessed %s between %s and %s at %s.", o.Kind, o.SubjectID, o.ObjectID, o.Location)
			}
			return speechDraft{
				Text:         text,
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     o.Kind,
					SubjectID: o.SubjectID,
					ObjectID:  o.ObjectID,
					Location:  o.Location,
					Quantity:  o.Quantity,
				},
			}, true
		case "actor_died":
			if m.Observation.ObjectID == "" {
				return speechDraft{}, false
			}
			// Never accuse the listener to their own face as the killer.
			if m.Observation.SubjectID != "" && m.Observation.SubjectID == listenerID {
				return speechDraft{}, false
			}
			mod := m.Observation.Modality
			prefix := "I witnessed"
			if mod != "" && mod != "eyewitness" {
				prefix = "I perceived via " + mod + " that"
			}
			text := fmt.Sprintf("%s %s die from %s at %s.", prefix, m.Observation.ObjectID, m.Observation.Item, m.Observation.Location)
			if m.Observation.SubjectID != "" {
				text = fmt.Sprintf("%s %s was killed by %s (%s) at %s.", prefix, m.Observation.ObjectID, m.Observation.SubjectID, m.Observation.Item, m.Observation.Location)
			}
			return speechDraft{
				Text:         text,
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "actor_died",
					Modality:  mod,
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  1,
				},
			}, true
		case "world_event":
			mod := m.Observation.Modality
			if mod == "" {
				mod = "eyewitness"
			}
			return speechDraft{
				Text:         fmt.Sprintf("I witnessed world event %s (%s) at %s via %s.", m.Observation.SubjectID, m.Observation.ObjectID, m.Observation.Location, mod),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "world_event",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		case "world_event_claimed":
			if m.Observation.SubjectID == listenerID {
				return speechDraft{}, false
			}
			return speechDraft{
				Text:         fmt.Sprintf("I saw %s claim %s (%d %s) at %s.", m.Observation.SubjectID, m.Observation.ObjectID, m.Observation.Quantity, m.Observation.Item, m.Observation.Location),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     "world_event_claimed",
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		case "secret_caught_red_handed", "secret_exposed", "secret_discovered":
			if m.Observation.SubjectID == listenerID {
				return speechDraft{}, false
			}
			return speechDraft{
				Text:         fmt.Sprintf("I witnessed %s's secret (%s) at %s.", m.Observation.SubjectID, m.Observation.Item, m.Observation.Location),
				BasisEventID: m.EventID,
				Statement: &sim.Statement{
					Topic:     m.Observation.Kind,
					SubjectID: m.Observation.SubjectID,
					ObjectID:  m.Observation.ObjectID,
					Item:      m.Observation.Item,
					Location:  m.Observation.Location,
					Quantity:  m.Observation.Quantity,
					Proceeds:  m.Observation.Proceeds,
				},
			}, true
		}
	}
	if (m.Kind == "speech" || m.Kind == "channel_message") && m.Source != self.ID && m.Source != listenerID && m.Statement != nil {
		v := *m.Statement
		if v.SubjectID != "" && v.SubjectID == listenerID {
			return speechDraft{}, false
		}
		// Retell the speaker's core words with a single clean attribution prefix
		// (avoiding infinite "A said: B said: C said: ..." chains).
		core := m.Text
		if idx := strings.LastIndex(core, " said: "); idx >= 0 {
			core = core[idx+len(" said: "):]
		}
		text := fmt.Sprintf("%s said: %s", m.Source, core)
		if core == "" {
			text = fmt.Sprintf("%s reported %s: %d %s at %s.", m.Source, v.Topic, v.Quantity, v.Item, v.Location)
		}
		return speechDraft{Text: text, BasisEventID: m.EventID, Statement: &v}, true
	}
	return speechDraft{}, false
}

func Offline() Pipeline { return Pipeline{Selector: RuleSelector{}} }

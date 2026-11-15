package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

type Content struct {
	Text         string         `json:"text"`
	Statement    *sim.Statement `json:"statement,omitempty"`
	BasisEventID int64          `json:"basis_event_id,omitempty"`
}

type Generator interface {
	Generate(context.Context, sim.Perception, sim.Action) (Content, error)
}

type Pipeline struct {
	Selector  Selector
	Generator Generator
}

func (p Pipeline) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	options := Options(view)
	if len(options) == 0 {
		return sim.Action{}, fmt.Errorf("actor %s has no actions granted by tags", view.Self.ID)
	}
	id, err := p.Selector.Select(ctx, view, options)
	if err != nil {
		return sim.Action{}, err
	}
	var selected *sim.Action
	for _, o := range options {
		if o.ID == id {
			v := o.Action
			selected = &v
			break
		}
	}
	if selected == nil {
		return sim.Action{}, fmt.Errorf("selector returned unavailable option %q", id)
	}
	if selected.Kind != "speak" && selected.Kind != "transmit_message" && selected.Kind != "issue_decree" {
		return *selected, nil
	}
	content, err := p.Generator.Generate(ctx, view, *selected)
	if err != nil {
		return sim.Action{}, err
	}
	// Content cannot replace the selected behavior, target, route, house, profession, guild, recipe, decree parameters, or trade terms.
	if selected.Kind == "issue_decree" {
		if content.Text != "" {
			selected.Text = content.Text
		}
		return *selected, nil
	}
	selected.Text = content.Text
	selected.Statement = content.Statement
	selected.BasisEventID = content.BasisEventID
	return *selected, nil
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
	a := p.Self
	allowed := p.Catalog.Capabilities(a)
	var options []Option
	add := func(label string, action sim.Action) {
		if allowed[action.Kind] {
			options = append(options, Option{ID: fmt.Sprintf("o%03d", len(options)), Label: label, Action: action})
		}
	}

	add("Wait 15m and catch breath (+1 energy)", sim.Action{Kind: "wait"})

	if a.InsideHouse != "" {
		houseName := a.InsideHouse
		if p.CurrentHouse != nil {
			houseName = p.CurrentHouse.Name
		}
		add(fmt.Sprintf("Step outside from %s onto %s (15m)", houseName, a.Location), sim.Action{Kind: "leave_house"})
		if h := p.CurrentHouse; h != nil && h.OwnerID == a.ID && h.Level < 3 {
			planks := h.Level
			coins := 4 * h.Level
			if a.Professions["carpenter"] >= 2 && a.Items["plank"] >= planks && a.Money >= coins && a.Energy >= 12 {
				add(fmt.Sprintf("Renovate %s to asset level %d (%d planks + %d coins, 60m, -12 energy)", h.Name, h.Level+1, planks, coins), sim.Action{
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
				if ok, _ := a.Attributes.Meets(f.Requirements); ok {
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
				add(fmt.Sprintf("Enter %s (level %d, comfort %d, condition %d/100)", h.Name, h.Level, h.Comfort, h.Condition), sim.Action{Kind: "enter_house", HouseID: h.ID})
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
			add(label, sim.Action{Kind: "move_venue", Location: vr.ToVenue, Channel: vr.ModeID})
		}
		if p.Polity != nil && p.Polity.LedPolityID != "" && a.Energy >= 4 &&
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
			if p.Polity.LedPolityTreasury >= 10 {
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
			if spec.Edible && a.Items[itemID] >= 1 {
				add(fmt.Sprintf("Eat 1 %s (-%d hunger)", itemID, spec.HungerRelief), sim.Action{Kind: "eat", Item: itemID})
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
			bestThreatID := ""
			bestThreatName := ""
			bestThreatAct := 5
			bestBounty := 0
			for _, enc := range p.Catalog.Encounters {
				if act := p.DistrictState.ThreatActivity[enc.ID]; act > bestThreatAct {
					bestThreatAct = act
					bestThreatID = enc.ID
					bestThreatName = enc.Name
					bestBounty = enc.BountyCoins
				}
			}
			if bestThreatID != "" {
				add(fmt.Sprintf("Hunt %s in %s (45m, -10 energy, threat %d, bounty %d+ coins)", bestThreatName, a.Location, bestThreatAct, bestBounty),
					sim.Action{Kind: "hunt_threat", TargetID: bestThreatID})
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
	}

	for _, r := range p.Catalog.Recipes {
		if ok, _ := sim.CheckRecipe(p.Catalog, p.Map, p.CurrentHouse, a, r); ok {
			bonus := r.BonusInput != "" && a.Items[r.BonusInput] >= 1
			expected := p.Catalog.RecipeYield(a, r, bonus)
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
			add(fmt.Sprintf("%s (%dm, -%d energy%s, tier %d, expect %d %s, you hold %d)",
				r.Label, r.Ticks*15, r.EnergyCost, mastery, tier, expected, r.OutputItem, a.Items[r.OutputItem]),
				sim.Action{Kind: "craft", RecipeID: r.ID})
		}
	}

	for _, h := range p.AccessibleHouses {
		if h.District == a.Location && (a.InsideHouse == "" || a.InsideHouse == h.ID) {
			if h.Condition < sim.MaxCondition && a.Energy >= 8 && a.Items["plank"] >= 1 {
				add(fmt.Sprintf("Repair %s with 1 plank (45m, -8 energy, condition %d/100)", h.Name, h.Condition), sim.Action{Kind: "repair_house", HouseID: h.ID})
			}
		}
	}

	embargoed := map[string]bool{}
	if p.Polity != nil {
		for _, emb := range p.Polity.EmbargoItems {
			embargoed[emb] = true
		}
	}
	var surplusItems []sim.ItemSpec
	for _, itemID := range itemKeys {
		if embargoed[itemID] {
			continue
		}
		spec := p.Catalog.Items[itemID]
		if spec.Tradeable && spec.BatchQty > 0 && a.Items[itemID] >= spec.OfferThreshold {
			surplusItems = append(surplusItems, spec)
		}
	}

	eligibleSpouse := make(map[string]bool, len(p.EligibleSpouseIDs))
	for _, id := range p.EligibleSpouseIDs {
		eligibleSpouse[id] = true
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
	if p.HasOpenWrongfulCase && p.AppealTargetID != "" && a.IllicitGain == 0 && a.Energy >= 6 {
		add("File censorate appeal for "+p.AppealTargetID+" (-6 energy)", sim.Action{
			Kind: "file_appeal", TargetID: p.AppealTargetID, Location: a.Location, BasisEventID: p.AppealBasisEventID,
		})
	}
	if len(a.Guilds) > 0 && p.DistrictController == "" && a.Energy >= 6 {
		add("Claim district sovereignty for "+a.Guilds[0], sim.Action{Kind: "claim_turf", FactionID: a.Guilds[0], Location: a.Location})
	}

	medItem := cheapestMedicinalItem(p.Catalog, a, itemKeys)
	for _, b := range p.Nearby {
		if b.Health < sim.MaxHealth && medItem != "" {
			add(fmt.Sprintf("Treat %s with 1 %s", b.Name, medItem), sim.Action{Kind: "heal", TargetID: b.ID, Item: medItem})
		}
		if b.Jailed {
			continue
		}
		add(fmt.Sprintf("Talk to %s (%s, %s, level %d)", b.Name, b.ID, b.Role, b.Level), sim.Action{Kind: "speak", TargetID: b.ID})
		for _, spec := range surplusItems {
			pending := false
			for _, o := range p.Offers {
				if o.Seller == a.ID && o.Buyer == b.ID && o.Item == spec.ID {
					pending = true
					break
				}
			}
			if !pending {
				add(fmt.Sprintf("Offer %d %s (tier %d) to %s for %d coins", spec.BatchQty, spec.ID, spec.Tier, b.Name, p.Catalog.SalePrice(a, spec.ID)), sim.Action{
					Kind: "offer", TargetID: b.ID, Item: spec.ID, Quantity: spec.BatchQty, Price: p.Catalog.SalePrice(a, spec.ID),
				})
			}
		}
		if b.Health > 0 && a.Energy >= 6 && a.Health >= 25 {
			add("Attack "+b.Name+" (-6 energy)", sim.Action{Kind: "attack", TargetID: b.ID})
		}
		if a.Health >= 65 && b.Health >= 65 && a.Energy >= 6 && p.Tick >= a.RestUntil && !b.Sleeping {
			add("Spar with "+b.Name+" in martial arts (-6 energy)", sim.Action{Kind: "spar", TargetID: b.ID})
		}
		if a.Health >= 45 && a.Energy >= 5 && p.Tick >= a.RestUntil && !sim.ActorHasTag(b.Tags, "outlaw") {
			add("Pickpocket "+b.Name+" (-5 energy)", sim.Action{Kind: "steal", TargetID: b.ID})
		}
		if a.Health >= 50 && a.Energy >= 6 && p.Tick >= a.RestUntil && !sim.ActorHasTag(b.Tags, "outlaw") {
			add("Collect gang protection fee from "+b.Name+" (-6 energy)", sim.Action{Kind: "extort", TargetID: b.ID})
		}
		if a.Health >= 55 && b.Health > 20 && a.Energy >= 8 && p.Tick >= a.RestUntil && !sim.ActorHasTag(b.Tags, "outlaw") {
			add("Raid and plunder "+b.Name+" (-8 energy)", sim.Action{Kind: "raid", TargetID: b.ID})
		}
		if a.Money >= 4 && a.Energy >= 3 && sim.ActorHasTag(b.Tags, "law") {
			add("Slip hush bribe to law officer "+b.Name+" (-3 energy)", sim.Action{Kind: "bribe", TargetID: b.ID})
		}
		if basisID, hasEv := sim.ActorCorruptionEvidence(a, b.ID); a.IllicitGain == 0 && hasEv && a.Energy >= 6 {
			add("Audit and expose corrupt officer "+b.Name+" (-6 energy)", sim.Action{
				Kind: "audit_corruption", TargetID: b.ID, BasisEventID: basisID,
			})
		}
		if a.Health >= 45 && b.Health > 0 && a.Energy >= 7 && p.Tick >= a.RestUntil && (sim.ActorHasTag(b.Tags, "outlaw") || sim.ActorHasTag(b.Tags, "raider") || sim.ActorHasTag(b.Tags, "thief") || sim.ActorHasTag(b.Tags, "gang") || a.Trust[b.ID] < 0) {
			add("Arrest outlaw "+b.Name+" for yamen bounty (-7 energy)", sim.Action{Kind: "arrest", TargetID: b.ID})
		}
		if p.CurrentHouse != nil && p.CurrentHouse.OwnerID == a.ID {
			add(fmt.Sprintf("Transfer ownership deed of %s to %s (permanent property transfer)", p.CurrentHouse.Name, b.Name), sim.Action{
				Kind: "transfer_house", TargetID: b.ID, HouseID: p.CurrentHouse.ID,
			})
		}

		if !p.Married && eligibleSpouse[b.ID] {
			add(fmt.Sprintf("Propose marriage alliance to %s", b.Name), sim.Action{
				Kind: "marry", TargetID: b.ID,
			})
		}
		if a.Energy >= 2 && a.Trust[b.ID] >= 3 {
			add(fmt.Sprintf("Swear kinship/patronage bond with %s", b.Name), sim.Action{
				Kind: "swear_bond", TargetID: b.ID,
			})
		}
		if grudgeTarget[b.ID] && a.Health >= 25 && a.Energy >= 6 && b.Health > 0 {
			add(fmt.Sprintf("Avenge blood feud against %s (-6 energy)", b.Name), sim.Action{
				Kind: "avenge_feud", TargetID: b.ID,
			})
		}
		if p.MediateTargetID != "" && b.ID == p.MediateTargetID && a.Money >= 4 && a.Energy >= 4 {
			add(fmt.Sprintf("Mediate blood feud with %s", b.Name), sim.Action{
				Kind: "mediate_feud", TargetID: b.ID,
			})
		}
		if (a.IllicitGain > 0 || p.DistrictGrievance >= 35) && a.Energy >= 5 && !sim.ActorHasTag(b.Tags, "law") && !sim.ActorHasTag(b.Tags, "censor") && !sim.ActorHasTag(b.Tags, "outlaw") {
			add(fmt.Sprintf("Frame %s in scapegoat court docket (-5 energy)", b.Name), sim.Action{
				Kind: "frame_suspect", TargetID: b.ID,
			})
		}
	}

	for _, o := range p.Offers {
		if o.Buyer != a.ID {
			continue
		}
		if a.Money >= o.Price {
			for _, b := range p.Nearby {
				if b.ID == o.Seller && !b.Jailed {
					add(fmt.Sprintf("Buy %d %s from %s for %d coins (offer %d)", o.Quantity, o.Item, b.Name, o.Price, o.ID), sim.Action{
						Kind: "accept", TargetID: o.Seller, OfferID: o.ID,
					})
				}
			}
		}
		add(fmt.Sprintf("Reject offer %d (%d %s)", o.ID, o.Quantity, o.Item), sim.Action{Kind: "reject", TargetID: o.Seller, OfferID: o.ID})
	}
	for _, action := range sim.ContinuityActions(p) {
		add(fmt.Sprintf("%s: target=%s recipe=%s item=%s quantity=%d price=%d agreement=%d", action.Kind, action.TargetID, action.RecipeID, action.Item, action.Quantity, action.Price, action.AgreementID), action)
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
					add(fmt.Sprintf("Accept loan request from %s: %d coins, interest %d per mille/day, term %d ticks, collateral %s", nb.Name, request.Principal, request.RatePermille, request.TermTicks, request.CollateralHouse), sim.Action{Kind: "accept_loan", TargetID: nb.ID})
					add("Reject loan request from "+nb.Name, sim.Action{Kind: "reject_loan", TargetID: nb.ID})
				}
			}
		}
	}
	if !p.HasActiveLoan && !pendingBorrow && a.Money < 10 {
		for _, quote := range p.LoanQuotes {
			add(fmt.Sprintf("Request %d coins from %s: interest %d per mille/day, term %d ticks, collateral %s", quote.Principal, quote.LenderID, quote.RatePermille, quote.TermTicks, quote.CollateralHouse), sim.Action{Kind: "borrow_loan", TargetID: quote.LenderID})
		}
	}
	if len(p.Catalog.CommChannels) > 0 {
		chKeys := make([]string, 0, len(p.Catalog.CommChannels))
		for k := range p.Catalog.CommChannels {
			chKeys = append(chKeys, k)
		}
		sort.Strings(chKeys)
		tags := a.IdentityTags()
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
	return options
}

func sortedItemIDs(items map[string]sim.ItemSpec) []string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
	a := p.Self
	nearWell := a.InsideHouse != "" ||
		p.Map.DistrictHasFeature(a.Location, "water_source") ||
		p.Map.DistrictHasFeature(a.Location, "canal_wharf")
	if !nearWell {
		return false
	}
	if a.InsideHouse == "" && p.DistrictState != nil {
		for resID, rs := range p.Catalog.Resources {
			if rs.PotableWater {
				if maxCap := p.DistrictState.MaxResources[resID]; maxCap > 0 {
					if p.DistrictState.Resources[resID] <= maxCap/20 {
						return false
					}
				}
				break
			}
		}
	}
	return true
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
	total := 0
	for id, qty := range a.Items {
		if qty > 0 && cat.Items[id].Edible {
			total += qty
		}
	}
	return total
}

func defaultEdibleItem(cat sim.Catalog) string {
	if spec, ok := cat.Items["food"]; ok && spec.Edible {
		return "food"
	}
	for _, id := range sortedItemIDs(cat.Items) {
		if cat.Items[id].Edible {
			return id
		}
	}
	return "food"
}

// RuleSelector is explicitly the offline baseline, never a Jev substitute.
type RuleSelector struct{}

func (RuleSelector) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	desired, err := (rulesPolicy{}).Decide(ctx, p)
	if err != nil {
		return "", err
	}
	for _, o := range options {
		a := o.Action
		if a.Kind == desired.Kind &&
			(desired.Channel == "" || a.Channel == desired.Channel) &&
			a.Location == desired.Location &&
			a.HouseID == desired.HouseID &&
			a.FactionID == desired.FactionID &&
			a.ProfessionID == desired.ProfessionID &&
			a.RecipeID == desired.RecipeID &&
			a.OfferID == desired.OfferID &&
			a.AgreementID == desired.AgreementID &&
			(desired.Item == "" || a.Item == desired.Item) &&
			(a.TargetID == desired.TargetID || a.OfferID != 0) {
			return o.ID, nil
		}
	}
	for _, o := range options {
		if o.Action.Kind == "wait" {
			return o.ID, nil
		}
	}
	return "", fmt.Errorf("no matching offline choice")
}

// PrimaryTopicItem determines which item a speaker's recent memories or role focus on for contextual expression.
func PrimaryTopicItem(cat sim.Catalog, self sim.Actor, listener sim.Neighbor) string {
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if m.Observation != nil && m.Observation.Item != "" {
			return m.Observation.Item
		}
		if m.Kind == "speech" && m.Source != self.ID && m.Source != listener.ID && m.Statement != nil && m.Statement.Item != "" {
			return m.Statement.Item
		}
	}
	for _, t := range self.IdentityTags() {
		if prods := cat.TagProfiles[t].Produces; len(prods) > 0 {
			return prods[0]
		}
	}
	return "food"
}

func findNeighbor(nearby []sim.Neighbor, id string) (sim.Neighbor, bool) {
	for _, b := range nearby {
		if b.ID == id {
			return b, true
		}
	}
	return sim.Neighbor{ID: id}, false
}

// TemplateContent is a deterministic offline fixture, not a simulation of character expression.
// It preserves witnessed/reported quantities; the content model chooses how a character speaks.
type TemplateContent struct{}

func (TemplateContent) Generate(_ context.Context, p sim.Perception, a sim.Action) (Content, error) {
	if a.Kind == "issue_decree" {
		polName := a.FactionID
		if p.Polity != nil && p.Polity.LedPolityName != "" {
			polName = p.Polity.LedPolityName
		}
		return Content{
			Text: fmt.Sprintf("Proclamation of %s by %s: enact %s (%d).", polName, p.Self.Name, a.Item, a.Quantity),
		}, nil
	}
	if a.Kind != "speak" && a.Kind != "transmit_message" {
		return Content{}, nil
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
		if m.Observation != nil {
			switch m.Observation.Kind {
			case "item_produced":
				q := m.Observation.Quantity
				return Content{
					Text:         fmt.Sprintf("I produced %d %s at %s.", q, m.Observation.Item, m.Observation.Location),
					BasisEventID: m.EventID,
					Statement:    &sim.Statement{Topic: "item_produced", Item: m.Observation.Item, Location: m.Observation.Location, Quantity: q},
				}, nil
			case "macro_anomaly":
				mod := m.Observation.Modality
				if mod == "" {
					mod = "eyewitness"
				}
				return Content{
					Text:         fmt.Sprintf("I observed %s (%s) at %s (intensity %d).", m.Observation.SubjectID, mod, m.Observation.Location, m.Observation.Quantity),
					BasisEventID: m.EventID,
					Statement: &sim.Statement{
						Topic:     "macro_anomaly",
						SubjectID: m.Observation.SubjectID,
						Item:      m.Observation.Item,
						Location:  m.Observation.Location,
						Quantity:  m.Observation.Quantity,
					},
				}, nil
			case "corruption":
				if m.Observation.SubjectID == "" || m.Observation.SubjectID == p.Self.ID || m.Observation.SubjectID == a.TargetID || p.Self.IllicitGain > 0 {
					continue
				}
				return Content{
					Text:         fmt.Sprintf("I witnessed %s taking %d illicit bribe coins at %s.", m.Observation.SubjectID, m.Observation.Quantity, m.Observation.Location),
					BasisEventID: m.EventID,
					Statement: &sim.Statement{
						Topic:     "corruption",
						SubjectID: m.Observation.SubjectID,
						ObjectID:  m.Observation.ObjectID,
						Item:      "coins",
						Location:  m.Observation.Location,
						Quantity:  m.Observation.Quantity,
					},
				}, nil
			case "wrongful_conviction":
				if m.Observation.ObjectID == "" || m.Observation.SubjectID == p.Self.ID || m.Observation.SubjectID == a.TargetID {
					continue
				}
				return Content{
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
				}, nil
			}
		}
		if (m.Kind == "speech" || m.Kind == "channel_message") && m.Source != p.Self.ID && m.Source != a.TargetID && m.Statement != nil {
			v := *m.Statement
			if v.SubjectID != "" && v.SubjectID == a.TargetID {
				continue
			}
			switch v.Topic {
			case "macro_anomaly":
				return Content{
					Text:         fmt.Sprintf("%s reported anomaly %s at %s (intensity %d).", m.Source, v.SubjectID, v.Location, v.Quantity),
					BasisEventID: m.EventID,
					Statement:    &v,
				}, nil
			case "corruption":
				return Content{
					Text:         fmt.Sprintf("%s said %s took illicit bribes (%d coins) at %s.", m.Source, v.SubjectID, v.Quantity, v.Location),
					BasisEventID: m.EventID,
					Statement:    &v,
				}, nil
			case "wrongful_conviction":
				return Content{
					Text:         fmt.Sprintf("%s said %s wrongfully framed %s in %s (case #%d).", m.Source, v.SubjectID, v.ObjectID, v.Location, v.CaseID),
					BasisEventID: m.EventID,
					Statement:    &v,
				}, nil
			default:
				return Content{
					Text:         fmt.Sprintf("%s said %d %s was produced at %s.", m.Source, v.Quantity, v.Item, v.Location),
					BasisEventID: m.EventID,
					Statement:    &v,
				}, nil
			}
		}
	}
	return Content{Text: fmt.Sprintf("I have %d food, %d energy, and am trying to %s.", p.Self.Items["food"], p.Self.Energy, p.Self.Goal)}, nil
}

func Offline() Pipeline { return Pipeline{Selector: RuleSelector{}, Generator: TemplateContent{}} }

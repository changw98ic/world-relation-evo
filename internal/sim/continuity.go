package sim

import (
	"fmt"
	"sort"
)

// PersonalPlan records an actor's chosen production-and-sale objective. It is
// independent of Task: sleeping, travelling or eating does not erase the goal.
type PersonalPlan struct {
	RecipeID     string `json:"recipe_id"`
	Item         string `json:"item"`
	Quantity     int    `json:"quantity"`
	Produced     int    `json:"produced"`
	Sold         int    `json:"sold"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	StartedTick  int64  `json:"started_tick"`
	FinishedTick int64  `json:"finished_tick,omitempty"`
}

func ClonePersonalPlan(p *PersonalPlan) *PersonalPlan {
	if p == nil {
		return nil
	}
	q := *p
	return &q
}

// RemainingToSell returns how many units of p.Item still need to be sold to complete an active plan.
func (p *PersonalPlan) RemainingToSell() int {
	if p == nil || p.Status != "active" {
		return 0
	}
	if p.Sold >= p.Quantity {
		return 0
	}
	return p.Quantity - p.Sold
}

// SyncActorPlan couples an active PersonalPlan's Stage and completion status to
// the actor's actual held inventory (a.Items[p.Item]) and completed sales (p.Sold).
func SyncActorPlan(a *Actor, tick int64, cat Catalog) {
	if a == nil || a.Plan == nil || a.Plan.Status != "active" {
		return
	}
	p := a.Plan
	if a.Dead {
		p.Status = "failed"
		p.FinishedTick = tick
		return
	}
	rem := p.Quantity - p.Sold
	if rem <= 0 {
		p.Status = "completed"
		p.Stage = "done"
		p.FinishedTick = tick
		return
	}
	p.Stage = p.inventoryStage(cat, a.Items[p.Item])
}

func (p PersonalPlan) nextSaleLot(cat Catalog) int {
	return min(p.RemainingToSell(), max(1, cat.Items[p.Item].BatchQty))
}

func (p PersonalPlan) inventoryStage(cat Catalog, held int) string {
	if held >= p.nextSaleLot(cat) {
		return "sell"
	}
	return "produce"
}

// growthProceeds returns this actor's expected share of the specified item.
func (p Perception) growthProceeds(g GrowthBatch, item string) int64 {
	owner := !g.Cohort && g.OwnerID == p.Self.ID
	var amount int64
	if owner {
		amount = g.Parents[item]
	}
	if g.Item != item {
		return amount
	}
	output := g.Quantity
	if !g.Harvested {
		for _, agreement := range p.CareAgreements {
			if agreement.GrowthID == g.ID {
				share := agreement.CaretakerShare(output)
				output -= share
				if agreement.CaretakerKind == "actor" && agreement.CaretakerID == p.Self.ID {
					amount += share
				}
				break
			}
		}
	}
	if owner {
		amount += output
	}
	return amount
}

func (p Perception) ExpectedGrowthStock(item string) int64 {
	var expected int64
	for _, g := range p.Growth {
		expected += p.growthProceeds(g, item)
	}
	return expected
}

// PlanDirectiveView summarizes an active PersonalPlan's inventory-coupled progress and immediate step.
type PlanDirectiveView struct {
	RecipeID           string   `json:"recipe_id"`
	Item               string   `json:"item"`
	Stage              string   `json:"stage"`
	TargetQuantity     int      `json:"target_quantity"`
	SoldQuantity       int      `json:"sold_quantity"`
	RemainingToSell    int      `json:"remaining_to_sell"`
	HeldQuantity       int      `json:"held_quantity"`
	DeficitToProduce   int      `json:"deficit_to_produce"`
	ExpectedFromGrowth int64    `json:"expected_from_growth,omitempty"`
	BlockReason        string   `json:"block_reason,omitempty"`
	MissingInputs      []string `json:"missing_inputs,omitempty"`
	ReadyHarvestIDs    []int64  `json:"ready_harvest_ids,omitempty"`
	ActiveRecipeID     string   `json:"active_recipe_id,omitempty"`
	NextStep           string   `json:"next_step"`
	TargetDistrict     string   `json:"target_district,omitempty"`
	TargetVenueID      string   `json:"target_venue_id,omitempty"`
	TargetHouseID      string   `json:"target_house_id,omitempty"`
}

// BuildPlanDirective derives the read-only tactical plan state for Perception without mutating world state.
func BuildPlanDirective(p Perception) *PlanDirectiveView {
	a := p.Self
	if a.Plan == nil || a.Plan.Status != "active" {
		return nil
	}
	rem := maxInt(0, a.Plan.Quantity-a.Plan.Sold)
	held := a.Items[a.Plan.Item]
	stage := a.Plan.inventoryStage(p.Catalog, held)
	nextLot := a.Plan.nextSaleLot(p.Catalog)
	dir := &PlanDirectiveView{
		RecipeID:         a.Plan.RecipeID,
		Item:             a.Plan.Item,
		Stage:            stage,
		TargetQuantity:   a.Plan.Quantity,
		SoldQuantity:     a.Plan.Sold,
		RemainingToSell:  rem,
		HeldQuantity:     held,
		DeficitToProduce: maxInt(0, nextLot-held),
	}
	if stage == "sell" {
		for _, id := range p.Continuity.FulfillableAgreementIDs {
			for _, g := range p.Continuity.Agreements {
				if g.ID == id && g.Item == a.Plan.Item {
					dir.NextStep = "fulfill_agreement"
					return dir
				}
			}
		}
		hasLocalBuyer := false
		for _, nb := range p.Nearby {
			if !nb.Jailed && !nb.Sleeping && !p.Catalog.TagsProduce(nb.Tags, a.Plan.Item) {
				hasLocalBuyer = true
				break
			}
		}
		if hasLocalBuyer {
			dir.NextStep = "offer_or_propose_sale"
			return dir
		}
		if len(p.Map.Venues) > 0 && a.VenueID != "" {
			for _, v := range p.Map.Venues {
				if p.Map.VenueOrDistrictHasFeature(v.ID, v.DistrictID, "market") {
					dir.TargetVenueID = v.ID
					dir.TargetDistrict = v.DistrictID
					if a.InsideHouse != "" {
						dir.NextStep = "leave_house_for_market"
					} else if a.VenueID != v.ID {
						dir.NextStep = "travel_to_market_venue:" + v.ID
					} else {
						dir.NextStep = "sell_at_market"
					}
					return dir
				}
			}
		}
		if marketID, _, ok := p.Map.NearestDistrictWithFeature(a.Location, "market"); ok {
			dir.TargetDistrict = marketID
			if a.InsideHouse != "" {
				dir.NextStep = "leave_house_for_market"
			} else if a.Location != marketID {
				dir.NextStep = "travel_to_market:" + marketID
			} else {
				dir.NextStep = "sell_at_market"
			}
			return dir
		}
		if a.InsideHouse != "" {
			dir.NextStep = "leave_house_to_find_buyer"
		} else {
			dir.NextStep = "offer_or_propose_sale"
		}
		return dir
	}

	dir.ExpectedFromGrowth = p.ExpectedGrowthStock(dir.Item)
	growthStep := dir.planGrowth(p, dir.Item, dir.ExpectedFromGrowth >= int64(dir.DeficitToProduce))
	dir.DeficitToProduce -= int(min(int64(dir.DeficitToProduce), dir.ExpectedFromGrowth))
	if growthStep {
		return dir
	}

	r, ok := p.Catalog.RecipeByID(a.Plan.RecipeID)
	if !ok {
		dir.NextStep = "abandon_invalid_plan"
		return dir
	}
	if blocked, reason := IsRecipeBlockedByWorldLaw(p.WorldLaws, r); blocked {
		dir.BlockReason = reason
		dir.NextStep = "abandon_ineligible_plan"
		return dir
	}
	if can, _ := p.Catalog.ProfessionGating(a, r); !can {
		dir.NextStep = "abandon_ineligible_plan"
		return dir
	}
	return dir.planRecipeStart(p, r, map[string]bool{})
}

func (dir *PlanDirectiveView) planRecipeStart(p Perception, workRecipe Recipe, visiting map[string]bool) *PlanDirectiveView {
	a := p.Self
	requirements := workRecipe.StartRequirements()
	var missing []string
	for _, item := range sortedProductionKeys(requirements) {
		if a.Items[item] < requirements[item] {
			missing = append(missing, item)
			dir.MissingInputs = append(dir.MissingInputs, fmt.Sprintf("%s:%d/%d", item, a.Items[item], requirements[item]))
		}
	}
	if len(missing) > 0 {
		visiting[workRecipe.ID] = true
		defer delete(visiting, workRecipe.ID)
		var pending *PlanDirectiveView
		for _, item := range missing {
			if dir.planGrowth(p, item, p.ExpectedGrowthStock(item) >= int64(requirements[item]-a.Items[item])) {
				return dir
			}
			for _, sub := range p.Catalog.Recipes {
				if sub.OutputItem != item || visiting[sub.ID] {
					continue
				}
				if ok, _ := p.Catalog.ProfessionGating(a, sub); !ok {
					continue
				}
				if blocked, _ := IsRecipeBlockedByWorldLaw(p.WorldLaws, sub); blocked {
					continue
				}
				branch := *dir
				branch.MissingInputs = append([]string(nil), dir.MissingInputs...)
				next := branch.planRecipeStart(p, sub, visiting)
				if next.NextStep != "production_blocked" && next.NextStep != "acquire_inputs" {
					return next
				}
				if pending == nil {
					pending = next
				}
			}
		}
		if pending != nil {
			return pending
		}
		dir.NextStep = "acquire_inputs"
		return dir
	}
	dir.ActiveRecipeID = workRecipe.ID
	if workRecipe.HouseFacility != "" {
		for _, h := range p.AccessibleHouses {
			if h.HasFacility(workRecipe.HouseFacility) {
				dir.TargetHouseID = h.ID
				dir.TargetDistrict = h.District
				dir.TargetVenueID = h.VenueID
				if a.InsideHouse == h.ID {
					dir.setCraftStep(p, workRecipe)
				} else if a.InsideHouse != "" {
					dir.NextStep = "leave_house_for_workshop:" + h.ID
				} else if a.Location == h.District && (h.VenueID == "" || a.VenueID == "" || h.VenueID == a.VenueID) {
					dir.NextStep = "enter_workshop:" + h.ID
				} else {
					dir.NextStep = "travel_to_workshop:" + h.ID
				}
				return dir
			}
		}
	}
	if workRecipe.NodeFeature != "" {
		if len(p.Map.Venues) > 0 && a.VenueID != "" {
			if venue, ok := p.RecipeResourceVenue(workRecipe); ok {
				dir.TargetVenueID = venue
				v, _ := p.Map.VenueByID(venue)
				dir.TargetDistrict = v.DistrictID
				if a.InsideHouse != "" {
					dir.NextStep = "leave_house_for_production"
				} else if a.VenueID != venue {
					dir.NextStep = "travel_to_production_venue:" + venue
				} else {
					dir.setCraftStep(p, workRecipe)
				}
				return dir
			}
		} else if dest, ok := p.RecipeResourceDistrict(workRecipe); ok {
			dir.TargetDistrict = dest
			if a.InsideHouse != "" {
				dir.NextStep = "leave_house_for_production"
			} else if a.Location != dest {
				dir.NextStep = "travel_to_production:" + dest
			} else {
				dir.setCraftStep(p, workRecipe)
			}
			return dir
		}
	}
	dir.setCraftStep(p, workRecipe)
	return dir
}

func (dir *PlanDirectiveView) setCraftStep(p Perception, r Recipe) {
	dir.BlockReason = p.RecipeBlockReason(r)
	if !p.Capabilities()["craft"] {
		dir.BlockReason = "identity tags do not grant action: craft"
	}
	if dir.BlockReason != "" {
		dir.NextStep = "production_blocked"
		return
	}
	dir.NextStep = "craft:" + r.ID
}

func (dir *PlanDirectiveView) planGrowth(p Perception, item string, covered bool) bool {
	for _, g := range p.Growth {
		amount := p.growthProceeds(g, item)
		if !g.Ready || amount == 0 {
			continue
		}
		id := fmt.Sprint(g.ID)
		for _, action := range p.GrowthCareActions {
			if action.Kind == "harvest" && action.TargetID == id && p.Capabilities()["harvest"] {
				dir.ReadyHarvestIDs = append(dir.ReadyHarvestIDs, g.ID)
				dir.NextStep = "harvest:" + id
				return true
			}
		}
		canCollect := !g.Cohort && g.OwnerID == p.Self.ID
		if !canCollect && !g.Harvested {
			for _, terms := range p.CareAgreements {
				if terms.GrowthID == g.ID && terms.Harvest && terms.CaretakerKind == "actor" && terms.CaretakerID == p.Self.ID {
					canCollect = true
					break
				}
			}
		}
		if canCollect && p.Capabilities()["harvest"] && (p.Self.Location != g.District || g.VenueID != "" && p.Self.VenueID != g.VenueID) {
			dir.TargetDistrict, dir.TargetVenueID = g.District, g.VenueID
			dir.NextStep = "travel_to_harvest"
		}
	}
	if dir.NextStep == "travel_to_harvest" {
		return true
	}
	if covered {
		for _, g := range p.Growth {
			if g.Stalled && !g.Ready && p.growthProceeds(g, item) > 0 {
				dir.NextStep = "care_stalled_growth"
				return true
			}
		}
		dir.NextStep = "await_harvest"
		return true
	}
	return false
}

type Agreement struct {
	ID           int64  `json:"id"`
	SupplierID   string `json:"supplier_id"`
	BuyerID      string `json:"buyer_id"`
	Item         string `json:"item"`
	Quantity     int    `json:"quantity"`
	Price        int    `json:"price"`
	ProposedTick int64  `json:"proposed_tick"`
	ResponseBy   int64  `json:"response_by"`
	DueTick      int64  `json:"due_tick"`
	AcceptedTick int64  `json:"accepted_tick,omitempty"`
	ClosedTick   int64  `json:"closed_tick,omitempty"`
	Status       string `json:"status"`
}

// Terms copied into an event or option are a snapshot of the contract as signed.
func (g Agreement) withTerms(x Action) Action {
	x.AgreementID, x.Item, x.Quantity, x.Price = g.ID, g.Item, g.Quantity, g.Price
	x.DueTick = &g.DueTick
	return x
}

// A reference alone identifies the terms. If a caller repeats them, it must
// name the same item, lot, total price and deadline as the stored proposal.
func (g Agreement) matchesAction(x Action) bool {
	return (x.Item == "" || x.Item == g.Item) &&
		(x.Quantity == 0 && x.Price == 0 || x.Quantity == g.Quantity && x.Price == g.Price) &&
		(x.DueTick == nil || *x.DueTick == g.DueTick) &&
		(x.Currency == "" || x.Currency == "coins")
}

func (w *World) agreementDeliveryBlockReason(g Agreement, supplier, buyer *Actor) string {
	if g.Status != "active" || w.Tick > g.DueTick {
		return "agreement is not active or its delivery deadline has passed"
	}
	if supplier == nil || buyer == nil || !SamePlace(*supplier, *buyer) || w.Tick < supplier.JailedUntil || w.Tick < buyer.JailedUntil || (buyer.Task != nil && (buyer.Task.Action.Kind == "sleep" || buyer.Task.Dormant)) {
		return "agreement participants are unavailable for delivery at the same place"
	}
	if reason := w.buyerItemBlockReason(*buyer, g.Item, "coins"); reason != "" {
		return reason
	}
	if supplier.Items[g.Item] < g.Quantity || buyer.Money < g.Price {
		return "delivery goods or payment unavailable"
	}
	return ""
}

// DialogueTurn keeps actual exchanges, including their event identity. Content
// is speech, never an instruction to mutate a contract or complete a plan.
type DialogueTurn struct {
	EventID     int64  `json:"event_id"`
	Tick        int64  `json:"tick"`
	SpeakerID   string `json:"speaker_id"`
	ListenerID  string `json:"listener_id"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	AgreementID int64  `json:"agreement_id,omitempty"`
}
type ContinuityState struct {
	NextAgreementID int64                     `json:"next_agreement_id"`
	Agreements      []Agreement               `json:"agreements,omitempty"`
	DialogueByActor map[string][]DialogueTurn `json:"dialogue_by_actor,omitempty"`
}
type ContinuityView struct {
	FulfillableAgreementIDs []int64        `json:"fulfillable_agreement_ids,omitempty"`
	Agreements              []Agreement    `json:"agreements,omitempty"`
	Dialogue                []DialogueTurn `json:"dialogue,omitempty"`
}

func (w *World) continuity() *ContinuityState {
	if w.Continuity == nil {
		w.Continuity = &ContinuityState{}
	}
	return w.Continuity
}
func (w *World) ContinuityFor(id string) ContinuityView {
	if w.Continuity == nil {
		return ContinuityView{}
	}
	return w.continuityForAgreements(id, w.Continuity.Agreements)
}

func (w *World) continuityForAgreements(id string, agreements []Agreement) ContinuityView {
	var v ContinuityView
	if w.Continuity == nil {
		return v
	}
	for _, a := range agreements {
		if a.SupplierID == id || a.BuyerID == id {
			v.Agreements = append(v.Agreements, a)
			if a.SupplierID == id && a.Status == "active" {
				supplier, buyer := w.actor(a.SupplierID), w.actor(a.BuyerID)
				if w.agreementDeliveryBlockReason(a, supplier, buyer) == "" {
					v.FulfillableAgreementIDs = append(v.FulfillableAgreementIDs, a.ID)
				}
			}
		}
	}
	// Read only this actor's own persisted exchanges, not the world's history.
	dialogue := w.Continuity.DialogueByActor[id]
	// Preserve the latest exchange with each counterpart, then fill with recent
	// turns. This avoids one busy conversation hiding an unfinished older one.
	selected := map[int]bool{}
	peers := map[string]bool{}
	for i := len(dialogue) - 1; i >= 0; i-- {
		d := dialogue[i]
		peer := ""
		if d.SpeakerID == id {
			peer = d.ListenerID
		} else if d.ListenerID == id {
			peer = d.SpeakerID
		}
		if peer != "" && !peers[peer] {
			selected[i] = true
			peers[peer] = true
		}
	}
	for i := len(dialogue) - 1; i >= 0 && len(selected) < 32; i-- {
		d := dialogue[i]
		if d.SpeakerID == id || d.ListenerID == id {
			selected[i] = true
		}
	}
	indexes := make([]int, 0, len(selected))
	for i := range selected {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	for _, i := range indexes {
		v.Dialogue = append(v.Dialogue, dialogue[i])
	}
	return v
}

func defaultDeliveryDeadline(proposalTick int64) int64 {
	return proposalTick + 3*TicksPerDay
}

// The catalog supplies the suggested lot for a new coin-denominated agreement.
func agreementQuote(cat Catalog, item string) (int, int, bool) {
	s, ok := cat.Items[item]
	if !ok || !s.Tradeable || !ItemAcceptsCurrency(s, "coins") || s.BatchQty <= 0 || s.BatchPrice < 0 {
		return 0, 0, false
	}
	return s.BatchQty, s.BatchPrice, true
}
func canProduce(cat Catalog, a Actor, item string) bool {
	for _, r := range cat.Recipes {
		if r.OutputItem == item {
			if ok, _ := cat.ProfessionGating(a, r); ok {
				return true
			}
		}
	}
	return false
}

func (w *World) ApplyContinuity(a *Actor, x Action) []Event {
	emit := func(kind, text string, b *Actor) []Event {
		return []Event{w.record(a, x, kind, map[string]any{"text": text}, b)}
	}
	fail := func(text string) []Event { return emit("action_failed", text, nil) }
	switch x.Kind {
	case "start_plan":
		if !w.Catalog.Capabilities(*a)["offer"] {
			return fail("production-and-sale plan requires trading capability")
		}
		if a.Plan != nil && a.Plan.Status == "active" {
			return fail("abandon the current plan before choosing another")
		}
		r, ok := w.Catalog.RecipeByID(x.RecipeID)
		if !ok {
			return fail("unknown plan recipe")
		}
		if ok, _ := w.Catalog.ProfessionGating(*a, r); !ok {
			return fail("recipe is not available to this actor")
		}
		s := w.Catalog.Items[r.OutputItem]
		if !s.Tradeable || r.OutputQty <= 0 {
			return fail("plan needs a tradeable production output")
		}
		a.Plan = &PersonalPlan{RecipeID: r.ID, Item: r.OutputItem, Quantity: maxInt(r.OutputQty, s.BatchQty), Status: "active", StartedTick: w.Tick}
		SyncActorPlan(a, w.Tick, w.Catalog)
		return emit("plan_started", fmt.Sprintf("%s plans to produce and sell %d %s (stage=%s, held=%d)", a.Name, a.Plan.Quantity, a.Plan.Item, a.Plan.Stage, a.Items[a.Plan.Item]), nil)
	case "abandon_plan":
		if a.Plan == nil || a.Plan.Status != "active" {
			return fail("no active plan")
		}
		a.Plan.Status = "abandoned"
		a.Plan.FinishedTick = w.Tick
		return emit("plan_abandoned", a.Name+" abandoned the production plan", nil)
	case "propose_agreement":
		b := w.actor(x.TargetID)
		if b == nil || b.Dead || b.ID == a.ID || !SamePlace(*a, *b) || !w.Catalog.Capabilities(*b)["accept_agreement"] {
			return fail("agreement counterpart is not nearby")
		}
		item := w.Catalog.Items[x.Item]
		if !item.Tradeable || !ItemAcceptsCurrency(item, "coins") || (x.Currency != "" && x.Currency != "coins") {
			return fail("item is unavailable for a coin delivery agreement")
		}
		q, price := x.Quantity, x.Price
		if q == 0 && price == 0 {
			// An item-only request selects the catalog lot. An explicit lot
			// with a zero total price promises free delivery.
			var ok bool
			q, price, ok = agreementQuote(w.Catalog, x.Item)
			if !ok {
				return fail("item has no suggested agreement terms")
			}
		}
		if q <= 0 || price < 0 {
			return fail("delivery requires a positive quantity and nonnegative total price")
		}
		due := defaultDeliveryDeadline(w.Tick)
		if x.DueTick != nil {
			due = *x.DueTick
		}
		if due < w.Tick {
			return fail("delivery deadline precedes proposal")
		}
		if a.Items[x.Item] < q && !canProduce(w.Catalog, *a, x.Item) {
			return fail("supplier neither owns nor can produce the promised item")
		}
		state := w.continuity()
		for _, g := range state.Agreements {
			if g.SupplierID == a.ID && g.BuyerID == b.ID && g.Item == x.Item && (g.Status == "proposed" || g.Status == "active") {
				return fail("this delivery already has an open proposal")
			}
		}
		state.NextAgreementID++
		g := Agreement{ID: state.NextAgreementID, SupplierID: a.ID, BuyerID: b.ID, Item: x.Item, Quantity: q, Price: price, ProposedTick: w.Tick, ResponseBy: min(w.Tick+TicksPerDay, due), DueTick: due, Status: "proposed"}
		state.Agreements = append(state.Agreements, g)
		x = g.withTerms(x)
		return emit("agreement_proposed", fmt.Sprintf("%s proposes to deliver %d %s to %s for %d coins by tick %d", a.Name, g.Quantity, g.Item, b.Name, g.Price, g.DueTick), b)
	case "accept_agreement", "reject_agreement", "fulfill_agreement":
		state := w.continuity()
		for i := range state.Agreements {
			g := &state.Agreements[i]
			if g.ID != x.AgreementID {
				continue
			}
			supplier, buyer := w.actor(g.SupplierID), w.actor(g.BuyerID)
			if !g.matchesAction(x) {
				return fail("action terms differ from the stored agreement")
			}
			x = g.withTerms(x)
			if x.Kind == "accept_agreement" || x.Kind == "reject_agreement" {
				if supplier == nil || buyer == nil || !SamePlace(*supplier, *buyer) || w.Tick < supplier.JailedUntil || (supplier.Task != nil && (supplier.Task.Action.Kind == "sleep" || supplier.Task.Dormant)) {
					return fail("agreement participants are unavailable to respond at the same place")
				}
				if a.ID != g.BuyerID || x.TargetID != g.SupplierID || g.Status != "proposed" || w.Tick > g.ResponseBy {
					return fail("no pending proposal for this recipient")
				}
				if x.Kind == "reject_agreement" {
					g.Status = "rejected"
					g.ClosedTick = w.Tick
					return emit("agreement_rejected", a.Name+" rejected the delivery proposal", supplier)
				}
				g.Status = "active"
				g.AcceptedTick = w.Tick
				return emit("agreement_accepted", a.Name+" accepted the delivery and payment terms", supplier)
			}
			if a.ID != g.SupplierID || x.TargetID != g.BuyerID {
				return fail("delivery requires the named supplier and buyer")
			}
			if reason := w.agreementDeliveryBlockReason(*g, supplier, buyer); reason != "" {
				return fail(reason)
			}
			if buyer.Items == nil {
				buyer.Items = map[string]int{}
			}
			supplier.Items[g.Item] -= g.Quantity
			buyer.Items[g.Item] += g.Quantity
			buyer.Money -= g.Price
			supplier.Money += g.Price
			g.Status = "fulfilled"
			if buyer.Trust == nil {
				buyer.Trust = map[string]int{}
			}
			if supplier.Trust == nil {
				supplier.Trust = map[string]int{}
			}
			buyer.Trust[supplier.ID]++
			supplier.Trust[buyer.ID]++
			g.ClosedTick = w.Tick
			return []Event{w.record(a, x, "agreement_fulfilled", map[string]any{"text": fmt.Sprintf("%s delivered %d %s to %s for %d coins", supplier.Name, g.Quantity, g.Item, buyer.Name, g.Price), "item": g.Item, "quantity": g.Quantity, "price": g.Price, "seller_id": supplier.ID, "buyer_id": buyer.ID}, buyer)}
		}
		return fail("agreement not found")
	}
	return nil
}

// ObserveContinuity runs after a real event is recorded. It does not emit more
// events, so the record hook cannot recurse.
func (w *World) ObserveContinuity(e Event) {
	if e.Kind == "speech" || e.Kind == "trade_offered" || e.Kind == "trade_counteroffered" || e.Kind == "trade_completed" || e.Kind == "trade_rejected" || e.Kind == "trade_expired" || e.Kind == "agreement_proposed" || e.Kind == "agreement_accepted" || e.Kind == "agreement_rejected" || e.Kind == "agreement_fulfilled" {
		s := w.continuity()
		if s.DialogueByActor == nil {
			s.DialogueByActor = map[string][]DialogueTurn{}
		}
		turn := DialogueTurn{EventID: e.ID, Tick: e.Tick, SpeakerID: e.ActorID, ListenerID: e.TargetID, Kind: e.Kind, Text: fmt.Sprint(e.Payload["text"]), AgreementID: e.Action.AgreementID}
		s.DialogueByActor[e.ActorID] = append(s.DialogueByActor[e.ActorID], turn)
		if e.TargetID != "" && e.TargetID != e.ActorID {
			s.DialogueByActor[e.TargetID] = append(s.DialogueByActor[e.TargetID], turn)
		}
	}
	if e.Kind == "actor_died" {
		if a := w.actor(e.ActorID); a != nil && a.Plan != nil && a.Plan.Status == "active" {
			a.Plan.Status = "failed"
			a.Plan.FinishedTick = w.Tick
		}
		return
	}
	quantity := payloadQuantity(e.Payload["quantity"])
	if e.Kind == "crafted" || e.Kind == "harvested" {
		if a := w.actor(e.ActorID); a != nil && a.Plan != nil && a.Plan.Status == "active" && e.Action.RecipeID == a.Plan.RecipeID {
			a.Plan.Produced += quantity
		}
	}
	sellerID := ""
	if e.Kind == "trade_completed" || e.Kind == "agreement_fulfilled" {
		sellerID = e.ActorID
		if e.Kind == "trade_completed" {
			sellerID, _ = e.Payload["seller_id"].(string)
		}
		if a := w.actor(sellerID); a != nil && a.Plan != nil && a.Plan.Status == "active" && e.Payload["item"] == a.Plan.Item {
			a.Plan.Sold += quantity
		}
	}
	for _, id := range []string{e.ActorID, e.TargetID, sellerID} {
		if id != "" {
			SyncActorPlan(w.actor(id), w.Tick, w.Catalog)
		}
	}
}
func payloadQuantity(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func (w *World) AdvanceContinuity() []Event {
	if w.Continuity == nil {
		return nil
	}
	var events []Event
	for i := range w.Continuity.Agreements {
		g := &w.Continuity.Agreements[i]
		kind := ""
		if g.Status == "proposed" && w.Tick > g.ResponseBy {
			g.Status = "expired"
			kind = "agreement_expired"
		}
		if g.Status == "active" && w.Tick > g.DueTick {
			g.Status = "unfulfilled"
			kind = "agreement_unfulfilled"
		}
		if kind == "" {
			continue
		}
		g.ClosedTick = w.Tick
		a, b := w.actor(g.SupplierID), w.actor(g.BuyerID)
		if a != nil {
			events = append(events, w.record(a, g.withTerms(Action{Kind: "agreement_deadline", TargetID: g.BuyerID}), kind, map[string]any{"text": fmt.Sprintf("delivery agreement %d %s", g.ID, g.Status)}, b))
		}
	}
	return events
}

// ContinuityActions adds choices; it never schedules a task behind the selector.
func ContinuityActions(p Perception) []Action {
	var out []Action
	a := p.Self
	if a.Plan != nil && a.Plan.Status == "active" {
		out = append(out, Action{Kind: "abandon_plan"})
	} else if p.Catalog.Capabilities(a)["offer"] {
		for _, r := range p.Catalog.Recipes {
			if ok, _ := p.Catalog.ProfessionGating(a, r); ok && p.Catalog.Items[r.OutputItem].Tradeable {
				out = append(out, Action{Kind: "start_plan", RecipeID: r.ID})
			}
		}
	}
	items := map[string]bool{}
	for item, q := range a.Items {
		if q > 0 {
			items[item] = true
		}
	}
	for _, r := range p.Catalog.Recipes {
		if ok, _ := p.Catalog.ProfessionGating(a, r); ok {
			items[r.OutputItem] = true
		}
	}
	keys := make([]string, 0, len(items))
	for item := range items {
		if q, _, ok := agreementQuote(p.Catalog, item); ok && (a.Items[item] >= q || canProduce(p.Catalog, a, item)) {
			keys = append(keys, item)
		}
	}
	sort.Strings(keys)
	type agreementPair struct {
		buyer string
		item  string
	}
	var openPairs map[agreementPair]bool
	if len(keys) > 0 {
		for _, g := range p.Continuity.Agreements {
			if g.SupplierID == a.ID && (g.Status == "proposed" || g.Status == "active") {
				if openPairs == nil {
					openPairs = make(map[agreementPair]bool)
				}
				openPairs[agreementPair{buyer: g.BuyerID, item: g.Item}] = true
			}
		}
	}
	nearActive := make(map[string]bool, len(p.Nearby))
	for _, b := range p.Nearby {
		if !b.Sleeping && !b.Jailed {
			nearActive[b.ID] = true
		} else {
			continue
		}
		if len(keys) == 0 || !tagsAllowAction(p.Catalog, b.Tags, "accept_agreement") {
			continue
		}
		for _, item := range keys {
			if openPairs[agreementPair{buyer: b.ID, item: item}] {
				continue
			}
			spec := p.Catalog.Items[item]
			due := defaultDeliveryDeadline(p.Tick)
			out = append(out, Action{Kind: "propose_agreement", TargetID: b.ID, Item: item, Quantity: spec.BatchQty, Price: spec.BatchPrice, DueTick: &due})
		}
	}
	for _, g := range p.Continuity.Agreements {
		if g.Status != "proposed" && g.Status != "active" {
			continue
		}
		other := g.SupplierID
		if a.ID == g.SupplierID {
			other = g.BuyerID
		}
		if !nearActive[other] {
			continue
		}
		if g.Status == "proposed" && g.BuyerID == a.ID && p.Tick <= g.ResponseBy {
			out = append(out, g.withTerms(Action{Kind: "accept_agreement", TargetID: other}), g.withTerms(Action{Kind: "reject_agreement", TargetID: other}))
		}
		fulfillable := false
		for _, id := range p.Continuity.FulfillableAgreementIDs {
			if id == g.ID {
				fulfillable = true
				break
			}
		}
		if fulfillable && g.Status == "active" && g.SupplierID == a.ID && p.Tick <= g.DueTick && a.Items[g.Item] >= g.Quantity {
			out = append(out, g.withTerms(Action{Kind: "fulfill_agreement", TargetID: other}))
		}
	}
	return out
}

func tagsAllowAction(c Catalog, tags []string, action string) bool {
	for _, tag := range tags {
		for _, act := range c.TagActions[tag] {
			if act == action {
				return true
			}
		}
	}
	if spec, ok := c.ActionSpecs[action]; ok {
		if len(spec.GrantedByTags) == 0 {
			return true
		}
		for _, gt := range spec.GrantedByTags {
			if ActorHasTag(tags, gt) {
				return true
			}
		}
	}
	return false
}

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
			if a.SupplierID == id && a.Status == "active" && w.Tick <= a.DueTick {
				supplier, buyer := w.actor(a.SupplierID), w.actor(a.BuyerID)
				if supplier != nil && buyer != nil && !supplier.Dead && !buyer.Dead && SamePlace(*supplier, *buyer) && supplier.Items[a.Item] >= a.Quantity && buyer.Money >= a.Price && w.Tick >= supplier.JailedUntil && w.Tick >= buyer.JailedUntil && !(buyer.Task != nil && buyer.Task.Action.Kind == "sleep") {
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

// Agreement terms come from the catalog's explicit batch terms, not generated
// prose. A supplier may promise future production even with no stock today.
func agreementQuote(cat Catalog, item string) (int, int, bool) {
	s, ok := cat.Items[item]
	if !ok || !s.Tradeable || s.BatchQty <= 0 || s.BatchPrice < 0 {
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
		a.Plan = &PersonalPlan{RecipeID: r.ID, Item: r.OutputItem, Quantity: maxInt(r.OutputQty, s.BatchQty), Stage: "produce", Status: "active", StartedTick: w.Tick}
		return emit("plan_started", fmt.Sprintf("%s plans to produce and sell %d %s", a.Name, a.Plan.Quantity, a.Plan.Item), nil)
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
		q, p, ok := agreementQuote(w.Catalog, x.Item)
		if !ok {
			return fail("item has no agreement terms")
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
		g := Agreement{ID: state.NextAgreementID, SupplierID: a.ID, BuyerID: b.ID, Item: x.Item, Quantity: q, Price: p, ProposedTick: w.Tick, ResponseBy: w.Tick + TicksPerDay, DueTick: w.Tick + 3*TicksPerDay, Status: "proposed"}
		state.Agreements = append(state.Agreements, g)
		x.AgreementID = g.ID
		return emit("agreement_proposed", fmt.Sprintf("%s proposes to deliver %d %s to %s for %d coins by tick %d", a.Name, q, x.Item, b.Name, p, g.DueTick), b)
	case "accept_agreement", "reject_agreement", "fulfill_agreement":
		state := w.continuity()
		for i := range state.Agreements {
			g := &state.Agreements[i]
			if g.ID != x.AgreementID {
				continue
			}
			supplier, buyer := w.actor(g.SupplierID), w.actor(g.BuyerID)
			if supplier == nil || buyer == nil || supplier.Dead || buyer.Dead {
				return fail("agreement participant unavailable")
			}
			if !SamePlace(*supplier, *buyer) {
				return fail("agreement participants are not together")
			}
			if x.Kind == "accept_agreement" || x.Kind == "reject_agreement" {
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
			if a.ID != g.SupplierID || x.TargetID != g.BuyerID || g.Status != "active" || w.Tick > g.DueTick {
				return fail("no active delivery for this supplier")
			}
			if supplier.Items[g.Item] < g.Quantity || buyer.Money < g.Price {
				return fail("delivery goods or payment unavailable")
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
			return []Event{w.record(a, x, "agreement_fulfilled", map[string]any{"text": fmt.Sprintf("%s delivered %d %s to %s for %d coins", supplier.Name, g.Quantity, g.Item, buyer.Name, g.Price), "item": g.Item, "quantity": g.Quantity, "seller_id": supplier.ID}, buyer)}
		}
		return fail("agreement not found")
	}
	return nil
}

// ObserveContinuity runs after a real event is recorded. It does not emit more
// events, so the record hook cannot recurse.
func (w *World) ObserveContinuity(e Event) {
	if e.Kind == "speech" || e.Kind == "agreement_proposed" || e.Kind == "agreement_accepted" || e.Kind == "agreement_rejected" || e.Kind == "agreement_fulfilled" {
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
	if e.Kind != "crafted" && e.Kind != "harvested" && e.Kind != "trade_completed" && e.Kind != "agreement_fulfilled" && e.Kind != "actor_died" {
		return
	}
	owner := e.ActorID
	if e.Kind == "trade_completed" {
		owner, _ = e.Payload["seller_id"].(string)
	}
	a := w.actor(owner)
	if a == nil || a.Plan == nil || a.Plan.Status != "active" {
		return
	}
	p := a.Plan
	if e.Kind == "actor_died" {
		p.Status = "failed"
		p.FinishedTick = w.Tick
		return
	}
	quantity := payloadQuantity(e.Payload["quantity"])
	if (e.Kind == "crafted" || e.Kind == "harvested") && e.Action.RecipeID == p.RecipeID {
		p.Produced += quantity
		if p.Produced >= p.Quantity {
			p.Stage = "sell"
			if p.Sold >= p.Quantity {
				p.Status = "completed"
				p.Stage = "done"
				p.FinishedTick = w.Tick
			}
		}
	}
	if (e.Kind == "trade_completed" || e.Kind == "agreement_fulfilled") && e.Payload["item"] == p.Item {
		p.Sold += quantity
		if p.Sold >= p.Quantity && p.Produced >= p.Quantity {
			p.Status = "completed"
			p.Stage = "done"
			p.FinishedTick = w.Tick
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
			events = append(events, w.record(a, Action{Kind: "agreement_deadline", TargetID: g.BuyerID, AgreementID: g.ID}, kind, map[string]any{"text": fmt.Sprintf("delivery agreement %d %s", g.ID, g.Status)}, b))
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
			out = append(out, Action{Kind: "propose_agreement", TargetID: b.ID, Item: item, Quantity: spec.BatchQty, Price: spec.BatchPrice})
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
			out = append(out, Action{Kind: "accept_agreement", TargetID: other, AgreementID: g.ID, Item: g.Item, Quantity: g.Quantity, Price: g.Price}, Action{Kind: "reject_agreement", TargetID: other, AgreementID: g.ID})
		}
		fulfillable := false
		for _, id := range p.Continuity.FulfillableAgreementIDs {
			if id == g.ID {
				fulfillable = true
				break
			}
		}
		if fulfillable && g.Status == "active" && g.SupplierID == a.ID && p.Tick <= g.DueTick && a.Items[g.Item] >= g.Quantity {
			out = append(out, Action{Kind: "fulfill_agreement", TargetID: other, AgreementID: g.ID, Item: g.Item, Quantity: g.Quantity, Price: g.Price})
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

package sim

import "fmt"

// TradeTerms are an explicit proposal, never a claim extracted from speech.
type TradeTerms struct {
	Quantity int    `json:"quantity"`
	Price    int    `json:"price"`
	Currency string `json:"currency"`
}

func (o Offer) ProposerID() string {
	if o.ProposedBy != "" {
		return o.ProposedBy
	}
	return o.Seller
}
func (o Offer) ResponderID() string {
	if o.ProposerID() == o.Seller {
		return o.Buyer
	}
	return o.Seller
}
func (o Offer) Counterparty(id string) string {
	if id == o.Seller {
		return o.Buyer
	}
	return o.Seller
}

// QuoteAction binds editable terms to a selected proposal's unchanged parties,
// item and predecessor. Physical availability is checked again at settlement.
func QuoteAction(p Perception, draft Action, terms TradeTerms) (Action, error) {
	if draft.Kind != "offer" {
		return Action{}, fmt.Errorf("only an offer can request trade terms")
	}
	if terms.Quantity <= 0 || terms.Price < 0 {
		return Action{}, fmt.Errorf("quote requires positive quantity and nonnegative total price")
	}
	currency := terms.Currency
	if currency == "" {
		currency = "coins"
	}
	spec, ok := p.Catalog.Items[draft.Item]
	if !ok || !spec.Tradeable || !ItemAcceptsCurrency(spec, currency) {
		return Action{}, fmt.Errorf("item %s cannot be quoted in %s", draft.Item, currency)
	}
	if draft.OfferID == 0 {
		if p.Self.Items[draft.Item] < terms.Quantity {
			return Action{}, fmt.Errorf("seller does not hold the proposed quantity")
		}
	} else {
		found := false
		for _, o := range p.Offers {
			if o.ID != draft.OfferID {
				continue
			}
			if o.ResponderID() != p.Self.ID || o.Counterparty(p.Self.ID) != draft.TargetID || o.Item != draft.Item || o.Expires < p.Tick {
				return Action{}, fmt.Errorf("counteroffer does not match an open proposal for this actor")
			}
			if o.Seller == p.Self.ID && p.Self.Items[o.Item] < terms.Quantity {
				return Action{}, fmt.Errorf("seller does not hold the proposed quantity")
			}
			found = true
			break
		}
		if !found {
			return Action{}, fmt.Errorf("original offer is no longer available")
		}
	}
	draft.Quantity, draft.Price, draft.Currency = terms.Quantity, terms.Price, currency
	if currency == "coins" {
		draft.Currency = ""
	}
	return draft, nil
}

func (w *World) counterOffer(a *Actor, x Action) []Event {
	fail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason}, w.actor(x.TargetID))}
	}
	for i, previous := range w.Offers {
		if previous.ID != x.OfferID {
			continue
		}
		if previous.ResponderID() != a.ID || x.TargetID != previous.Counterparty(a.ID) || x.Item != previous.Item || previous.Expires < w.Tick {
			return fail("counteroffer must answer the current proposal without changing its parties or item")
		}
		if a.ID == previous.Buyer && !w.ActorCapabilities(*a)["accept"] {
			return fail("buyer identity does not grant purchase assent")
		}
		other := w.actor(x.TargetID)
		if other == nil || other.Dead || w.Tick < other.JailedUntil || !SamePlace(*a, *other) {
			return fail("counterparty is not available at the same place")
		}
		p := Perception{Self: *a, Tick: w.Tick, Catalog: w.Catalog, Offers: []Offer{previous}}
		valid, err := QuoteAction(p, x, TradeTerms{Quantity: x.Quantity, Price: x.Price, Currency: x.Currency})
		if err != nil {
			return fail(err.Error())
		}
		if w.IsItemEmbargoedAt(a.Location, a.VenueID, x.Item) {
			return fail("item is embargoed by local polity decree: " + x.Item)
		}
		buyer := w.actor(previous.Buyer)
		currency := valid.Currency
		if currency == "" {
			currency = "coins"
		}
		if buyer == nil {
			return fail("buyer is no longer available")
		}
		if reason := w.buyerItemBlockReason(*buyer, x.Item, currency); reason != "" {
			return fail(reason)
		}
		e := w.record(a, valid, "trade_counteroffered", map[string]any{"text": fmt.Sprintf("%s proposes %d %s for %d %s in reply to offer %d", a.Name, valid.Quantity, valid.Item, valid.Price, currency, previous.ID), "previous_offer_id": previous.ID}, other)
		next := previous
		next.ID, next.PreviousID, next.ProposedBy = e.ID, previous.ID, a.ID
		next.Quantity, next.Price, next.Currency = valid.Quantity, valid.Price, valid.Currency
		w.Offers[i] = next
		return []Event{e}
	}
	return fail("original offer not found")
}

// Interactions expose cross-turn state without scheduling a response. A latest
// utterance is an opportunity to reply, not an obligation or a settled promise.
type InteractionView struct {
	Kind            string `json:"kind"`
	ID              int64  `json:"id"`
	CounterpartyID  string `json:"counterparty_id"`
	AwaitingActorID string `json:"awaiting_actor_id,omitempty"`
	Expires         int64  `json:"expires,omitempty"`
	BlockReason     string `json:"block_reason,omitempty"`
	ReplyOptional   bool   `json:"reply_optional"`
}

func BuildInteractionViews(p Perception) []InteractionView {
	var out []InteractionView
	available := func(id string) bool {
		for _, n := range p.Nearby {
			if n.ID == id && !n.Jailed && !n.Sleeping {
				return true
			}
		}
		return false
	}
	for _, o := range p.Offers {
		if o.Expires < p.Tick {
			continue
		}
		v := InteractionView{Kind: "offer", ID: o.ID, CounterpartyID: o.Counterparty(p.Self.ID), AwaitingActorID: o.ResponderID(), Expires: o.Expires, ReplyOptional: true}
		if !available(v.CounterpartyID) {
			v.BlockReason = "counterparty is not available at the same place"
		} else {
			v.BlockReason = p.OfferBlockReasons[o.ID]
		}
		out = append(out, v)
	}
	for _, g := range p.Continuity.Agreements {
		if g.Status != "proposed" && g.Status != "active" {
			continue
		}
		other := g.SupplierID
		if p.Self.ID == other {
			other = g.BuyerID
		}
		v := InteractionView{Kind: "agreement", ID: g.ID, CounterpartyID: other, AwaitingActorID: g.BuyerID, Expires: g.ResponseBy, ReplyOptional: true}
		if g.Status == "active" {
			v.AwaitingActorID = g.SupplierID
			v.Expires = g.DueTick
		}
		if !available(other) {
			v.BlockReason = "counterparty is not available at the same place"
		}
		out = append(out, v)
	}
	latest := map[string]int64{}
	for _, d := range p.Continuity.Dialogue {
		other := d.SpeakerID
		if other == p.Self.ID {
			other = d.ListenerID
		}
		if d.EventID > latest[other] {
			latest[other] = d.EventID
		}
	}
	for _, d := range p.Continuity.Dialogue {
		if d.Kind != "speech" {
			continue
		}
		other := d.SpeakerID
		if other == p.Self.ID {
			other = d.ListenerID
		}
		if latest[other] != d.EventID {
			continue
		}
		v := InteractionView{Kind: "dialogue", ID: d.EventID, CounterpartyID: other, AwaitingActorID: d.ListenerID, ReplyOptional: true}
		if !available(other) {
			v.BlockReason = "counterparty is not available at the same place"
		}
		out = append(out, v)
	}
	return out
}

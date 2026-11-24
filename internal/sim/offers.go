package sim

import (
	"fmt"
	"math"
)

// perceivedOfferBlockReason evaluates only publicly observable and self-owned
// offer constraints for Perception, never peeking at a counterparty's private
// wallet or private inventory before an accept attempt is submitted.
// OfferAcceptanceBlockReason checks the viewer's own holdings and the goods on display.
func (p Perception) OfferAcceptanceBlockReason(offer Offer) string {
	a := p.Self
	if offer.Expires < p.Tick {
		return "offer has expired"
	}
	peer := p.localNeighbor(offer.Counterparty(a.ID))
	if peer == nil || peer.Jailed || p.Tick < a.JailedUntil {
		return "seller is not available at the buyer's location"
	}
	currency := offer.Currency
	if currency == "" {
		currency = "coins"
	}
	spec := p.Catalog.Items[offer.Item]
	if !spec.Tradeable {
		return fmt.Sprintf("item %s is not tradeable", offer.Item)
	}
	if !itemAcceptsCurrency(spec, currency) {
		return fmt.Sprintf("item %s does not accept currency %s", offer.Item, currency)
	}
	buyerTags := peer.Tags
	if a.ID == offer.Buyer {
		buyerTags = p.EffectiveTags()
	}
	if spec.BuyerRequiredTag != "" && !ActorHasTag(buyerTags, spec.BuyerRequiredTag) {
		return fmt.Sprintf("buyer %s lacks required tag %s for %s", offer.Buyer, spec.BuyerRequiredTag, offer.Item)
	}
	if p.Polity != nil && ActorHasTag(p.Polity.EmbargoItems, offer.Item) {
		return "item is embargoed by local polity decree: " + offer.Item
	}
	quantity := p.Continuity.DisplayedGoods[offer.ID]
	if a.ID == offer.Seller {
		quantity = a.Items[offer.Item]
	}
	if quantity < offer.Quantity {
		return "seller no longer holds the offered quantity"
	}
	balance := a.Currencies[currency]
	if currency == "coins" {
		balance = int64(a.Money)
	}
	if a.ID == offer.Buyer && balance < int64(offer.Price) {
		return "buyer cannot pay the offered price"
	}
	return ""
}

func (w *World) offerSettlementBlockReason(buyer Actor, offer Offer) string {
	seller := w.actor(offer.Seller)
	if offer.Expires < w.Tick {
		return "offer has expired"
	}
	if buyer.Dead || seller == nil || seller.Dead || !SamePlace(buyer, *seller) || w.Tick < seller.JailedUntil || w.Tick < buyer.JailedUntil {
		return "seller is not available at the buyer's location"
	}
	effort := w.Catalog.tradeEnergy(offer.Item, offer.Quantity)
	if buyer.Energy < effort || seller.Energy < effort {
		return "one participant cannot handle this lot with their remaining energy"
	}
	if seller.Items[offer.Item] < offer.Quantity {
		return "seller no longer holds the offered quantity"
	}
	currency := offer.Currency
	if currency == "" {
		currency = "coins"
	}
	if reason := w.buyerItemBlockReason(buyer, offer.Item, currency); reason != "" {
		return reason
	}
	if w.actorCurrencyBalance(&buyer, currency) < int64(offer.Price) {
		return "buyer cannot pay the offered price"
	}

	if offer.Quantity > int(^uint(0)>>1)-buyer.Items[offer.Item] {
		return "buyer item balance would exceed the integer representation"
	}
	tax := w.offerTax(buyer, offer)
	net := int64(offer.Price - tax)
	if currency == "coins" {
		if int64(seller.Money) > int64(int(^uint(0)>>1))-net {
			return "seller coin balance would exceed the integer representation"
		}
	} else if seller.Currencies[currency] > math.MaxInt64-net {
		return "seller currency balance would exceed the integer representation"
	}
	if tax > 0 {
		amount := int64(tax)
		pol := w.EffectiveGoverningPolity(buyer.Location, buyer.VenueID)
		if pol != nil {
			if currency == "coins" {
				if pol.Treasury > math.MaxInt64-amount || pol.Ledger.TaxAssessedGross > math.MaxInt64-amount || pol.Ledger.TaxToTreasury > math.MaxInt64-amount {
					return "polity tax balance would exceed the integer representation"
				}
			} else if pol.Currencies[currency] > math.MaxInt64-amount {
				return "polity currency balance would exceed the integer representation"
			}
		} else if w.PublicTreasury > math.MaxInt64-amount || w.TreasuryLedger.TaxToTreasury > math.MaxInt64-amount {
			return "public tax balance would exceed the integer representation"
		}
	}
	return ""
}

func (w *World) offerTax(buyer Actor, offer Offer) int {
	if offer.Price <= 0 {
		return 0
	}
	if offer.Currency != "" && offer.Currency != "coins" && w.EffectiveGoverningPolity(buyer.Location, buyer.VenueID) == nil {
		return 0
	}
	rate := min(w.EffectiveMarketTaxPermille(buyer.Location, buyer.VenueID), 1000)
	if rate <= 0 {
		return 0
	}
	return (offer.Price/1000)*rate + (offer.Price%1000)*rate/1000
}

// A change of payer does not change the local restrictions on an item's buyer.
func (w *World) buyerItemBlockReason(buyer Actor, item, currency string) string {
	spec := w.Catalog.Items[item]
	if !spec.Tradeable {
		return fmt.Sprintf("item %s is not tradeable", item)
	}
	if !itemAcceptsCurrency(spec, currency) {
		return fmt.Sprintf("item %s does not accept currency %s", item, currency)
	}
	if spec.BuyerRequiredTag != "" && !ActorHasTag(w.actorEffectiveTags(buyer), spec.BuyerRequiredTag) {
		return fmt.Sprintf("buyer %s lacks required tag %s for %s", buyer.ID, spec.BuyerRequiredTag, item)
	}
	if w.IsItemEmbargoedAt(buyer.Location, buyer.VenueID, item) {
		return "item is embargoed by local polity decree: " + item
	}
	return ""
}

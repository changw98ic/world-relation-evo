package sim

import "fmt"

// Perception and serial settlement use the same current delivery conditions.
// A simultaneous competing action can still invalidate an otherwise valid offer.
func (w *World) offerSettlementBlockReason(buyer Actor, offer Offer) string {
	seller := w.actor(offer.Seller)
	if offer.Expires < w.Tick {
		return "offer has expired"
	}
	if seller == nil || !SamePlace(buyer, *seller) || w.Tick < seller.JailedUntil {
		return "seller is not available at the buyer's location"
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
	return ""
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

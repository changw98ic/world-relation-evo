package sim

import "fmt"

// Shipment holds purchased cargo until the route's travel time has elapsed.
// Money changes owners at dispatch; cargo remains part of held inventory in transit.
type Shipment struct {
	ID          int64  `json:"id"`
	BuyerID     string `json:"buyer_id"`
	Item        string `json:"item"`
	Quantity    int64  `json:"quantity"`
	From        string `json:"from"`
	To          string `json:"to"`
	ArrivalTick int64  `json:"arrival_tick"`
}

func (w *World) incomingStock(buyerID, item string) int64 {
	var total int64
	for _, s := range w.Shipments {
		if s.BuyerID == buyerID && s.Item == item {
			total += s.Quantity
		}
	}
	return total
}

func (w *World) settleShipments() []Event {
	var events []Event
	pending := w.Shipments[:0]
	for _, s := range w.Shipments {
		if w.Tick < s.ArrivalTick {
			pending = append(pending, s)
			continue
		}
		buyer := w.findCohort(s.BuyerID)
		if buyer == nil || buyer.District != s.To {
			panic(fmt.Sprintf("shipment %d has no destination owner %s at %s", s.ID, s.BuyerID, s.To))
		}
		if buyer.ItemPool == nil {
			buyer.ItemPool = map[string]int64{}
		}
		buyer.ItemPool[s.Item] += s.Quantity
		events = append(events, Event{ID: w.NextID, Tick: w.Tick, ActorID: s.BuyerID, Kind: "shipment_arrived", Action: Action{Kind: "accept", Item: s.Item, Quantity: int(s.Quantity), Location: s.To}, Payload: map[string]any{"shipment_id": s.ID, "from": s.From, "to": s.To, "quantity": s.Quantity, "item": s.Item}})
		w.NextID++
	}
	w.Shipments = pending
	if len(w.Shipments) == 0 {
		w.Shipments = nil
	}
	return events
}

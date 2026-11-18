package sim

import "fmt"

// dispatchCohortHarvests schedules the consenting local caretaker's stored cargo.
// A blocked route leaves the cargo in place and reports the failed dispatch.
func (w *World) dispatchCohortHarvests() []Event {
	var events []Event
	for i := 0; i < len(w.Growth); {
		g := w.Growth[i]
		a := w.careAgreement(g)
		care := w.growthCaretaker(g)
		if !g.Ready || !g.Harvested || !g.Cohort || g.Quantity == 0 || a == nil || a.CaretakerKind != "cohort" || !a.Harvest || !w.growthOwnerPresent(care) {
			i++
			continue
		}
		before := len(w.Growth)
		dispatched, err := w.DispatchHarvest(g.ID)
		if err != nil {
			events = append(events, w.growthEvent(g, "harvest_delivery_blocked", map[string]any{"reason": err.Error(), "item": g.Item, "quantity": g.Quantity}))
			i++
			continue
		}
		events = append(events, dispatched...)
		if len(w.Growth) == before {
			i++
		}
	}
	return events
}

// DispatchHarvest moves goods already harvested into the existing in-transit
// ledger. It does not pay a second purchase price: the owner already owns them.
// An individual owner instead collects at the stored district through harvest.
func (w *World) DispatchHarvest(growthID int64) ([]Event, error) {
	for i := range w.Growth {
		g := &w.Growth[i]
		if g.ID != growthID {
			continue
		}
		if !g.Ready || !g.Harvested || !g.Cohort {
			return nil, fmt.Errorf("harvest is not stored cargo owned by a cohort")
		}
		owner := w.findCohort(g.OwnerID)
		if owner == nil || owner.Population <= 0 {
			return nil, fmt.Errorf("harvest has no living destination owner")
		}
		route, ok := w.planFreightRoute(g.District, owner.District, g.Item, CalendarAt(w.Tick, w.Catalog.Calendar))
		if !ok {
			return nil, fmt.Errorf("no open freight route from stored harvest to owner")
		}
		if w.IsItemEmbargoedAt(g.District, g.VenueID, g.Item) || w.IsItemEmbargoedAt(owner.District, "", g.Item) {
			return nil, fmt.Errorf("harvest delivery is embargoed")
		}
		// Regional trade and harvest cargo share each edge's declared batch budget.
		batchQty := int64(max(1, w.Catalog.Items[g.Item].BatchQty))
		capacity := g.Quantity
		if len(route.edges) > 0 {
			capacity = min(capacity, w.freightRouteAvailableBatches(route)*batchQty)
		}
		if capacity <= 0 {
			return nil, fmt.Errorf("freight route has no remaining declared capacity")
		}
		if len(route.edges) > 0 {
			w.reserveFreightRoute(route, (capacity+batchQty-1)/batchQty)
		}
		// Parents remain under local husbandry until their own physical delivery or
		// collection; dispatching crop cargo never teleports retained animals.
		shipmentID := w.NextID
		w.Shipments = append(w.Shipments, Shipment{ID: shipmentID, GrowthID: g.ID, SourceGrowthID: g.harvestSource(), HarvestBasis: g.harvestBasis(), BuyerID: g.OwnerID, Item: g.Item, Quantity: capacity, From: g.District, To: owner.District, ArrivalTick: w.Tick + route.ticks})
		w.NextID++
		g.Quantity -= capacity
		e := w.growthEvent(*g, "harvest_dispatched", map[string]any{"source_growth_id": g.harvestSource(), "shipment_id": shipmentID, "item": g.Item, "quantity": capacity, "from": g.District, "to": owner.District, "arrival_tick": w.Tick + route.ticks})
		if g.Quantity == 0 && len(g.Parents) == 0 {
			w.Growth = append(w.Growth[:i], w.Growth[i+1:]...)
			if len(w.Growth) == 0 {
				w.Growth = nil
			}
		}
		return []Event{e}, nil
	}
	return nil, fmt.Errorf("unknown stored harvest")
}

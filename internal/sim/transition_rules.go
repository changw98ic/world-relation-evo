package sim

import (
	"fmt"
	"sort"
)

// transitionRequirementReason is shared by explicit and threshold-triggered transitions.
// Preconditions and costs always refer to the same pre-payment state.
func (w *World) transitionRequirementReason(a Actor, trans TransitionSpec) string {
	if trans.RequiredFormID != "" && a.FormID != trans.RequiredFormID {
		return fmt.Sprintf("transition %s requires form %s (has %s)", trans.ID, trans.RequiredFormID, a.FormID)
	}
	keys := make([]string, 0, len(trans.RequiredSlots))
	for id := range trans.RequiredSlots {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if a.StateSlots[id] < trans.RequiredSlots[id] {
			return fmt.Sprintf("transition %s requires %s >= %d (has %d)", trans.ID, id, trans.RequiredSlots[id], a.StateSlots[id])
		}
	}
	keys = keys[:0]
	for id := range trans.SlotCosts {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		cost := trans.SlotCosts[id]
		if a.StateSlots[id]-cost < 0 {
			return fmt.Sprintf("transition %s requires %d %s", trans.ID, cost, id)
		}
	}
	keys = keys[:0]
	for id := range trans.ItemCosts {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if a.Items[id] < trans.ItemCosts[id] {
			return fmt.Sprintf("transition %s requires %d %s", trans.ID, trans.ItemCosts[id], id)
		}
	}
	return ""
}

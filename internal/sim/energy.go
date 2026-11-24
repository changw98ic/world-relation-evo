package sim

// EnergySource converts a configured amount of fuel or local environmental resource
// into stored energy at the configured interval. It never creates an implicit fuel supply.
type EnergySource struct {
	FuelItem      string `json:"fuel_item,omitempty"`
	ResourcePool  string `json:"resource_pool,omitempty"`
	UnitsPerCycle int64  `json:"units_per_cycle"`
	EnergyPerUnit int64  `json:"energy_per_unit"`
	CycleTicks    int64  `json:"cycle_ticks"`
	Source        string `json:"source"`
}

func (w *World) storedEnergy() int64 {
	var total int64
	if w.FlowGrid != nil {
		for _, n := range w.FlowGrid.Nodes {
			total += n.StoredEnergy
		}
	}
	return total
}

func (w *World) supplyEnergy() {
	for i := range w.FlowGrid.Nodes {
		n := &w.FlowGrid.Nodes[i]
		s := n.Source
		if n.Kind != "generator" || !n.Active || s == nil || w.Tick < n.NextSupplyTick {
			continue
		}
		units := min(s.UnitsPerCycle, (n.EnergyCapacity-n.StoredEnergy)/s.EnergyPerUnit)
		if s.FuelItem != "" {
			units = min(units, n.FuelStock)
		} else {
			units = min(units, w.availableDistrictResource(n.DistrictID, s.ResourcePool))
		}
		if units <= 0 {
			continue
		}
		if s.FuelItem != "" {
			w.adjustFlowNodeFuel(n, -units, true)
		} else {
			w.drawDistrictResource(n.DistrictID, s.ResourcePool, units)
		}
		energy := units * s.EnergyPerUnit
		w.adjustFlowNodeEnergy(n, energy, true)
		n.NextSupplyTick = w.Tick + s.CycleTicks
	}
}

func (w *World) localEnergyNodes(a Actor) []FlowNode {
	var nodes []FlowNode
	if w.FlowGrid == nil {
		return nil
	}
	for _, n := range w.FlowGrid.Nodes {
		if n.Kind == "generator" && n.DistrictID == a.Location && (n.VenueID == "" || n.VenueID == a.VenueID) {
			if n.Source != nil {
				source := *n.Source
				n.Source = &source
			}
			nodes = append(nodes, n)
		}
	}
	return nodes
}

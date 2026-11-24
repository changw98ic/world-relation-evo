package sim

// District is an outdoor node on the world road/waterway graph.
type District struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	GeoRegionID        string           `json:"geo_region_id,omitempty"`
	PolityID           string           `json:"polity_id,omitempty"`
	Terrain            string           `json:"terrain,omitempty"`
	Features           []string         `json:"features,omitempty"`
	ResourceCapacities map[string]int64 `json:"resource_capacities,omitempty"`
}

// Road is an undirected physical or waterway route between two districts with travel ticks (15m each),
// personal energy cost, and optional bulk freight parameters (e.g. waterway canal vs mountain pass).
type Road struct {
	From               string `json:"from"`
	To                 string `json:"to"`
	Ticks              int    `json:"ticks"`
	EnergyCost         int    `json:"energy_cost"`
	Medium             string `json:"medium,omitempty"`
	FreightCapacity    int64  `json:"freight_capacity,omitempty"`
	FreightFeePermille int    `json:"freight_fee_permille,omitempty"`
	BaseLossPermille   int    `json:"base_loss_permille,omitempty"`
	CarrierTag         string `json:"carrier_tag,omitempty"`
}

// WorldMap defines all connected village districts, roads, natural geography regions/flows, and sub-district venues.
type WorldMap struct {
	Districts   []District    `json:"districts"`
	Roads       []Road        `json:"roads"`
	GeoRegions  []GeoRegion   `json:"geo_regions,omitempty"`
	GeoFlows    []GeoFlowEdge `json:"geo_flows,omitempty"`
	Venues      []Venue       `json:"venues,omitempty"`
	VenueRoutes []VenueRoute  `json:"venue_routes,omitempty"`
}

func DefaultMap() WorldMap {
	return WorldMap{
		Districts: []District{
			{ID: "west_cottages", Name: "West Cottages", Features: []string{"residential"}},
			{ID: "creek_well", Name: "Creek Well", Features: []string{"water_source", "residential"}},
			{ID: "south_fields", Name: "South Fields", Features: []string{"farmland", "forage_ground"}},
			{ID: "crossroads", Name: "Village Crossroads", Features: []string{"hub", "residential", "academy", "yamen"}},
			{ID: "market_street", Name: "Market Street", Features: []string{"market", "residential", "gang_turf"}},
			{ID: "east_lane", Name: "East Artisan Lane", Features: []string{"residential", "workshop_row", "martial_hall"}},
			{ID: "north_woods", Name: "North Woods", Features: []string{"woodland", "herb_patch", "forage_ground", "bandit_camp"}},
		},
		Roads: []Road{
			{From: "west_cottages", To: "creek_well", Ticks: 1, EnergyCost: 2},
			{From: "west_cottages", To: "crossroads", Ticks: 1, EnergyCost: 2},
			{From: "creek_well", To: "south_fields", Ticks: 1, EnergyCost: 3},
			{From: "creek_well", To: "crossroads", Ticks: 1, EnergyCost: 2},
			{From: "crossroads", To: "market_street", Ticks: 1, EnergyCost: 2},
			{From: "crossroads", To: "east_lane", Ticks: 1, EnergyCost: 2},
			{From: "east_lane", To: "market_street", Ticks: 1, EnergyCost: 2},
			{From: "crossroads", To: "north_woods", Ticks: 2, EnergyCost: 4},
			{From: "east_lane", To: "north_woods", Ticks: 2, EnergyCost: 4},
		},
	}
}

func (m WorldMap) DistrictByID(id string) (District, bool) {
	for _, d := range m.Districts {
		if d.ID == id {
			return d, true
		}
	}
	return District{}, false
}

func (m WorldMap) DistrictHasFeature(id, feature string) bool {
	d, ok := m.DistrictByID(id)
	if !ok {
		return false
	}
	for _, f := range d.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// RoadBetween returns the directed road segment from -> to if physically connected.
func (m WorldMap) RoadBetween(from, to string) (Road, bool) {
	for _, r := range m.Roads {
		if r.From == from && r.To == to {
			return r, true
		}
		if r.To == from && r.From == to {
			rev := r
			rev.From = from
			rev.To = to
			return rev, true
		}
	}
	return Road{}, false
}

// NeighborsOf returns all road segments leaving district from in deterministic map order.
func (m WorldMap) NeighborsOf(from string) []Road {
	var out []Road
	for _, r := range m.Roads {
		if r.From == from {
			out = append(out, r)
		} else if r.To == from {
			rev := r
			rev.From = from
			rev.To = r.From
			out = append(out, rev)
		}
	}
	return out
}

// NextHop computes the shortest-time road hop from -> to using Dijkstra over road ticks.
func (m WorldMap) NextHop(from, to string) (Road, int, bool) {
	if from == to {
		return Road{}, 0, false
	}
	const inf = 1 << 29
	dist := make(map[string]int, len(m.Districts))
	firstHop := make(map[string]Road, len(m.Districts))
	visited := make(map[string]bool, len(m.Districts))
	for _, d := range m.Districts {
		dist[d.ID] = inf
	}
	if _, ok := dist[from]; !ok {
		return Road{}, 0, false
	}
	if _, ok := dist[to]; !ok {
		return Road{}, 0, false
	}
	dist[from] = 0

	for range m.Districts {
		u := ""
		best := inf
		for _, d := range m.Districts {
			if !visited[d.ID] && dist[d.ID] < best {
				best = dist[d.ID]
				u = d.ID
			}
		}
		if u == "" || u == to {
			break
		}
		visited[u] = true
		for _, edge := range m.NeighborsOf(u) {
			cand := dist[u] + edge.Ticks
			if cand < dist[edge.To] {
				dist[edge.To] = cand
				if u == from {
					firstHop[edge.To] = edge
				} else {
					firstHop[edge.To] = firstHop[u]
				}
			}
		}
	}
	if dist[to] >= inf {
		return Road{}, 0, false
	}
	return firstHop[to], dist[to], true
}

// NearestDistrictWithFeature finds the reachable district offering feature with minimum travel ticks.
func (m WorldMap) NearestDistrictWithFeature(from, feature string) (string, int, bool) {
	if m.DistrictHasFeature(from, feature) {
		return from, 0, true
	}
	bestID := ""
	bestTicks := 1 << 29
	for _, d := range m.Districts {
		if !m.DistrictHasFeature(d.ID, feature) {
			continue
		}
		if _, total, ok := m.NextHop(from, d.ID); ok && total < bestTicks {
			bestTicks = total
			bestID = d.ID
		}
	}
	if bestID == "" {
		return "", 0, false
	}
	return bestID, bestTicks, true
}

// LeadsToward lists non-adjacent districts whose shortest route from `from` starts with `firstStep`.
func (m WorldMap) LeadsToward(from, firstStep string) []string {
	var targets []string
	for _, d := range m.Districts {
		if d.ID == from || d.ID == firstStep {
			continue
		}
		if hop, _, ok := m.NextHop(from, d.ID); ok && hop.To == firstStep {
			targets = append(targets, d.ID)
		}
	}
	return targets
}

func CloneMap(m WorldMap) WorldMap {
	out := WorldMap{
		Districts: make([]District, len(m.Districts)),
		Roads:     append([]Road(nil), m.Roads...),
	}
	for i, d := range m.Districts {
		dc := d
		if d.Features != nil {
			dc.Features = append([]string(nil), d.Features...)
		}
		if d.ResourceCapacities != nil {
			dc.ResourceCapacities = make(map[string]int64, len(d.ResourceCapacities))
			for k, v := range d.ResourceCapacities {
				dc.ResourceCapacities[k] = v
			}
		}
		out.Districts[i] = dc
	}
	if m.GeoRegions != nil {
		out.GeoRegions = make([]GeoRegion, len(m.GeoRegions))
		for i, r := range m.GeoRegions {
			rc := r
			if r.Resources != nil {
				rc.Resources = make(map[string]int64, len(r.Resources))
				for k, v := range r.Resources {
					rc.Resources[k] = v
				}
			}
			if r.AmbientFields != nil {
				rc.AmbientFields = cloneIntMap(r.AmbientFields)
			}
			out.GeoRegions[i] = rc
		}
	}
	if m.GeoFlows != nil {
		out.GeoFlows = append([]GeoFlowEdge(nil), m.GeoFlows...)
	}
	if m.Venues != nil {
		out.Venues = make([]Venue, len(m.Venues))
		for i, v := range m.Venues {
			vc := v
			if v.Features != nil {
				vc.Features = append([]string(nil), v.Features...)
			}
			out.Venues[i] = vc
		}
	}
	if m.VenueRoutes != nil {
		out.VenueRoutes = append([]VenueRoute(nil), m.VenueRoutes...)
	}
	return out
}

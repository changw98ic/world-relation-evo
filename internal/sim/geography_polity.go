package sim

import (
	"fmt"
	"sort"
	"strings"
)

// GeoCoord represents a 2D integer kilometer coordinate in the natural geography layer.
type GeoCoord struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// GeoDistanceKm computes deterministic integer Euclidean floor distance in kilometers.
func GeoDistanceKm(a, b GeoCoord) int {
	dx := int64(a.X - b.X)
	dy := int64(a.Y - b.Y)
	sq := dx*dx + dy*dy
	if sq <= 0 {
		return 0
	}
	// Integer square root via Newton's method.
	x := sq
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + sq/x) / 2
	}
	return int(x)
}

// GeoRegion is Layer 1 (Natural Geography): watershed/basin/sector cells with coordinates,
// elevation, terrain, climate, natural resource pools, and ambient physical/supernatural fields.
type GeoRegion struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Coord         GeoCoord         `json:"coord"`
	AreaSqKm      int              `json:"area_sq_km,omitempty"`
	ElevationM    int              `json:"elevation_m,omitempty"`
	Terrain       string           `json:"terrain,omitempty"`
	Climate       string           `json:"climate,omitempty"`
	Resources     map[string]int64 `json:"resources,omitempty"`
	AmbientFields map[string]int   `json:"ambient_fields,omitempty"`
}

// GeoFlowEdge is a directed physical flux link between two GeoRegions (e.g. river downstream,
// prevailing wind corridor, underground aquifer, or spirit leyline). Natural anomalies propagate
// along matching flow edges regardless of administrative Polity borders.
type GeoFlowEdge struct {
	FromRegion          string `json:"from_region"`
	ToRegion            string `json:"to_region"`
	Kind                string `json:"kind"` // e.g. "river", "wind_corridor", "leyline", "aquifer"
	ConductancePermille int    `json:"conductance_permille"`
}

// GeoAnomalySpec describes a natural or supernatural macro anomaly originating in a GeoRegion.
type GeoAnomalySpec struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	OriginRegionID    string         `json:"origin_region_id"`
	FlowKind          string         `json:"flow_kind,omitempty"`
	Intensity         int            `json:"intensity"`
	DurationTicks     int64          `json:"duration_ticks,omitempty"`
	MitigatedByInfra  string         `json:"mitigated_by_infra,omitempty"`
	ResourceDamagePct map[string]int `json:"resource_damage_pct,omitempty"`
	AmbientFieldDelta map[string]int `json:"ambient_field_delta,omitempty"`
	HouseDamage       int            `json:"house_damage,omitempty"`
	ActorHealthDamage int            `json:"actor_health_damage,omitempty"`
}

// Venue is Layer 2 (Places & Transport): a concrete sub-district place such as a village square,
// courtyard, teahouse, paddy field, mine shaft, sect altar, or sealed bunker.
// Co-located interaction (SamePlace) requires sharing the same VenueID when venues are configured.
type Venue struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DistrictID  string   `json:"district_id,omitempty"`
	GeoRegionID string   `json:"geo_region_id,omitempty"`
	PolityID    string   `json:"polity_id,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Features    []string `json:"features,omitempty"`
	Shielding   int      `json:"shielding,omitempty"`
	Indoor      bool     `json:"indoor,omitempty"`
}

// VenueRoute connects two Venues via a transport medium (road, footpath, river, sea, air, teleport).
type VenueRoute struct {
	FromVenue   string `json:"from_venue"`
	ToVenue     string `json:"to_venue"`
	DistanceKm  int    `json:"distance_km,omitempty"`
	Ticks       int    `json:"ticks"`
	EnergyCost  int    `json:"energy_cost"`
	Medium      string `json:"medium,omitempty"`
	RequiredTag string `json:"required_tag,omitempty"`
}

// VenueRouteView exposes a reachable venue hop in an actor's Perception, already adjusted for
// terrain, weather, and the actor's best available LocomotionSpec.
type VenueRouteView struct {
	ToVenue              string   `json:"to_venue"`
	ToDistrict           string   `json:"to_district,omitempty"`
	Medium               string   `json:"medium,omitempty"`
	ModeID               string   `json:"mode_id,omitempty"`
	Ticks                int      `json:"ticks"`
	EnergyCost           int      `json:"energy_cost"`
	LeadsToward          []string `json:"leads_toward,omitempty"`
	LeadsTowardDistricts []string `json:"leads_toward_districts,omitempty"`
}

// LocomotionSpec declaratively defines a movement mode (e.g. walking, horse, boat, sword_flight,
// rover, teleport_array) without hardcoding genre-specific movement rules.
type LocomotionSpec struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	GrantedByTags  []string `json:"granted_by_tags,omitempty"`
	AllowedMediums []string `json:"allowed_mediums,omitempty"`
	SpeedPct       int      `json:"speed_pct,omitempty"`
	EnergyPct      int      `json:"energy_pct,omitempty"`
	BypassWeather  bool     `json:"bypass_weather,omitempty"`
}

// SensorySpec declaratively defines an observation modality beyond ordinary eyesight
// (e.g. divine_sense, orbital_sensor, seismic_array), bounded by range and shielding penetration.
type SensorySpec struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	GrantedByTags    []string `json:"granted_by_tags,omitempty"`
	RangeKm          int      `json:"range_km"`
	PenetratesIndoor bool     `json:"penetrates_indoor,omitempty"`
	PenetrationPower int      `json:"penetration_power,omitempty"`
}

// CommChannelSpec declaratively defines a remote communication channel (e.g. sound_talisman,
// encrypted_radio, public_pa, courier_beacon). Channel messages always record Channel + Statement
// and never impersonate a first-hand eyewitness Observation.
type CommChannelSpec struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	GrantedByTags    []string `json:"granted_by_tags,omitempty"`
	ReceiverTags     []string `json:"receiver_tags,omitempty"`
	RangeKm          int      `json:"range_km"`
	Broadcast        bool     `json:"broadcast,omitempty"`
	PenetrationPower int      `json:"penetration_power,omitempty"`
	EnergyCost       int      `json:"energy_cost,omitempty"`
}

// PolityDecree is an enacted policy or administrative order on a PolityNode.
type PolityDecree struct {
	ID         int64  `json:"id"`
	PolityID   string `json:"polity_id"`
	IssuerID   string `json:"issuer_id"`
	Kind       string `json:"kind"` // "market_tax", "poll_tax", "curfew_on", "curfew_off", "relief_grant", "embargo"
	Item       string `json:"item,omitempty"`
	Value      int    `json:"value,omitempty"`
	Text       string `json:"text,omitempty"`
	IssuedTick int64  `json:"issued_tick"`
}

// PolityNode is Layer 3 (Administrative & Faction Control): a WorldPack-configurable jurisdiction
// (province/county/town/village, sect domain/market town, orbital command/surface sector).
// Statutory hierarchy (DeJureParentID) and actual territorial control (DeFactoControllerID) are
// stored independently so breakaway warlords, occupied towns, or rebel sectors do not inherit
// decrees from a nominal superior they no longer obey.
type PolityNode struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Level               string           `json:"level"`
	DeJureParentID      string           `json:"de_jure_parent_id,omitempty"`
	DeFactoControllerID string           `json:"de_facto_controller_id,omitempty"`
	SeatDistrictID      string           `json:"seat_district_id,omitempty"`
	SeatVenueID         string           `json:"seat_venue_id,omitempty"`
	LeaderActorID       string           `json:"leader_actor_id,omitempty"`
	Treasury            int64            `json:"treasury,omitempty"`
	Currencies          map[string]int64 `json:"currencies,omitempty"`
	Ledger              TreasuryLedger   `json:"ledger,omitempty"`
	MarketTaxPermille   int              `json:"market_tax_permille,omitempty"`
	PollTaxPermille     int              `json:"poll_tax_permille,omitempty"`
	Curfew              bool             `json:"curfew,omitempty"`
	EmbargoItems        []string         `json:"embargo_items,omitempty"`
	Decrees             []PolityDecree   `json:"decrees,omitempty"`
}

// PolityNetwork holds all multi-level administrative and faction jurisdictions in the world.
type PolityNetwork struct {
	Nodes []PolityNode `json:"nodes,omitempty"`
}

// PolityPerceptionView exposes public administrative facts at the actor's current location
// plus leadership authority if the actor leads a polity—never leaking private remote events.
type PolityPerceptionView struct {
	ReliefGrantAvailable  bool     `json:"relief_grant_available"`
	LedPolitySeatDistrict string   `json:"led_polity_seat_district,omitempty"`
	LocalPolityID         string   `json:"local_polity_id,omitempty"`
	LocalPolityName       string   `json:"local_polity_name,omitempty"`
	LocalPolityLevel      string   `json:"local_polity_level,omitempty"`
	EffectiveRulerID      string   `json:"effective_ruler_id,omitempty"`
	MarketTaxPermille     int      `json:"market_tax_permille,omitempty"`
	Curfew                bool     `json:"curfew,omitempty"`
	EmbargoItems          []string `json:"embargo_items,omitempty"`
	LedPolityID           string   `json:"led_polity_id,omitempty"`
	LedPolityName         string   `json:"led_polity_name,omitempty"`
	LedPolityTreasury     int64    `json:"led_polity_treasury,omitempty"`
	LedPolitySeatVenue    string   `json:"led_polity_seat_venue,omitempty"`
	LedPolityCurfew       bool     `json:"led_polity_curfew,omitempty"`
	LedPolityMarketTax    int      `json:"led_polity_market_tax,omitempty"`
}

func ClonePolityNetwork(net *PolityNetwork) *PolityNetwork {
	if net == nil {
		return nil
	}
	out := &PolityNetwork{}
	if net.Nodes != nil {
		out.Nodes = make([]PolityNode, len(net.Nodes))
		for i, n := range net.Nodes {
			nc := n
			nc.Currencies = cloneInt64Map(n.Currencies)
			if n.EmbargoItems != nil {
				nc.EmbargoItems = append([]string(nil), n.EmbargoItems...)
			}
			if n.Decrees != nil {
				nc.Decrees = append([]PolityDecree(nil), n.Decrees...)
			}
			out.Nodes[i] = nc
		}
	}
	return out
}

func (net *PolityNetwork) NodeByID(id string) *PolityNode {
	if net == nil || id == "" {
		return nil
	}
	for i := range net.Nodes {
		if net.Nodes[i].ID == id {
			return &net.Nodes[i]
		}
	}
	return nil
}

func (net *PolityNetwork) NodeLedBy(actorID string) *PolityNode {
	if net == nil || actorID == "" {
		return nil
	}
	for i := range net.Nodes {
		if net.Nodes[i].LeaderActorID == actorID {
			return &net.Nodes[i]
		}
	}
	return nil
}

// EffectivePolityChain returns the ordered chain of PolityNodes whose laws/decrees are actually
// enforced at basePolityID, from most local to highest obeyed superior.
// If a node's DeFactoControllerID points to a different polity (breakaway/occupation), statutory
// DeJureParentID inheritance stops and switches to the de-facto controller chain instead.
func (net *PolityNetwork) EffectivePolityChain(basePolityID string) []PolityNode {
	if net == nil || basePolityID == "" {
		return nil
	}
	var chain []PolityNode
	seen := make(map[string]bool, 8)
	curID := basePolityID
	for curID != "" && !seen[curID] {
		seen[curID] = true
		node := net.NodeByID(curID)
		if node == nil {
			break
		}
		if node.DeFactoControllerID != "" && node.DeFactoControllerID != node.ID && node.DeFactoControllerID != node.DeJureParentID {
			// Territory is de-facto controlled by another polity/faction rather than its statutory superior.
			if ctrl := net.NodeByID(node.DeFactoControllerID); ctrl != nil {
				curID = ctrl.ID
				continue
			}
			// Controlled by a non-polity rebel force: local node stands alone without obeying DeJureParentID.
			chain = append(chain, *node)
			break
		}
		chain = append(chain, *node)
		curID = node.DeJureParentID
	}
	return chain
}

// PolityGovernsNode reports whether rulerPolityID has effective de-facto jurisdiction over targetPolityID.
func (net *PolityNetwork) PolityGovernsNode(rulerPolityID, targetPolityID string) bool {
	if net == nil || rulerPolityID == "" || targetPolityID == "" {
		return false
	}
	for _, n := range net.EffectivePolityChain(targetPolityID) {
		if n.ID == rulerPolityID {
			return true
		}
	}
	return false
}

func (m WorldMap) GeoRegionByID(id string) (GeoRegion, bool) {
	for _, r := range m.GeoRegions {
		if r.ID == id {
			return r, true
		}
	}
	return GeoRegion{}, false
}

func (m WorldMap) VenueByID(id string) (Venue, bool) {
	for _, v := range m.Venues {
		if v.ID == id {
			return v, true
		}
	}
	return Venue{}, false
}

func (m WorldMap) DefaultVenueForDistrict(districtID string) string {
	for _, v := range m.Venues {
		if v.DistrictID == districtID {
			return v.ID
		}
	}
	return ""
}

// PlaceResidents initializes missing locations at creation or when resuming a
// snapshot. The current house locates an indoor resident; otherwise the map's
// declared district entry locates an outdoor resident. Existing positions and
// historical records of the dead are retained.
func (w *World) PlaceResidents() {
	if len(w.Map.Venues) == 0 {
		return
	}
	for i := range w.Actors {
		w.placeResident(&w.Actors[i])
	}
	for i := range w.Cohorts {
		for j := range w.Cohorts[i].Members {
			w.placeResident(&w.Cohorts[i].Members[j])
		}
	}
}

func (w *World) placeResident(a *Actor) {
	if a.VenueID != "" || a.Dead {
		return
	}
	if a.InsideHouse != "" {
		if h := w.house(a.InsideHouse); h != nil && h.District == a.Location && h.VenueID != "" {
			a.VenueID = h.VenueID
			return
		}
	}
	a.VenueID = w.Map.DefaultVenueForDistrict(a.Location)
}

func (m WorldMap) VenueOrDistrictHasFeature(venueID, districtID, feature string) bool {
	if venueID != "" {
		if v, ok := m.VenueByID(venueID); ok {
			if len(v.Features) > 0 {
				for _, f := range v.Features {
					if f == feature {
						return true
					}
				}
				return false
			}
			if v.DistrictID != "" {
				districtID = v.DistrictID
			}
		}
	}
	return m.DistrictHasFeature(districtID, feature)
}

func (m WorldMap) GeoRegionIDForLocation(districtID, venueID string) string {
	if venueID != "" {
		if v, ok := m.VenueByID(venueID); ok {
			if v.GeoRegionID != "" {
				return v.GeoRegionID
			}
			if v.DistrictID != "" {
				districtID = v.DistrictID
			}
		}
	}
	if districtID != "" {
		if d, ok := m.DistrictByID(districtID); ok && d.GeoRegionID != "" {
			return d.GeoRegionID
		}
	}
	return ""
}

func (m WorldMap) GeoCoordOfLocation(districtID, venueID string) (GeoCoord, bool) {
	regID := m.GeoRegionIDForLocation(districtID, venueID)
	if regID == "" {
		return GeoCoord{}, false
	}
	reg, ok := m.GeoRegionByID(regID)
	if !ok {
		return GeoCoord{}, false
	}
	return reg.Coord, true
}

func (m WorldMap) PolityIDForLocation(districtID, venueID string) string {
	if venueID != "" {
		if v, ok := m.VenueByID(venueID); ok {
			if v.PolityID != "" {
				return v.PolityID
			}
			if v.DistrictID != "" {
				districtID = v.DistrictID
			}
		}
	}
	if districtID != "" {
		if d, ok := m.DistrictByID(districtID); ok && d.PolityID != "" {
			return d.PolityID
		}
	}
	return ""
}

func (m WorldMap) VenueRoutesFrom(fromVenue string) []VenueRoute {
	if fromVenue == "" || len(m.VenueRoutes) == 0 {
		return nil
	}
	var out []VenueRoute
	for _, r := range m.VenueRoutes {
		if r.FromVenue == fromVenue {
			out = append(out, r)
		} else if r.ToVenue == fromVenue {
			rev := r
			rev.FromVenue = fromVenue
			rev.ToVenue = r.FromVenue
			out = append(out, rev)
		}
	}
	return out
}

func (m WorldMap) VenueRouteBetween(fromVenue, toVenue string) (VenueRoute, bool) {
	for _, r := range m.VenueRoutes {
		if r.FromVenue == fromVenue && r.ToVenue == toVenue {
			return r, true
		}
		if r.ToVenue == fromVenue && r.FromVenue == toVenue {
			rev := r
			rev.FromVenue = fromVenue
			rev.ToVenue = toVenue
			return rev, true
		}
	}
	return VenueRoute{}, false
}

func actorEffectiveTags(cat Catalog, a Actor) []string {
	tags := cat.ActorEffectiveTags(a)
	if len(a.Professions) == 0 {
		return tags
	}
	for _, prof := range cat.Professions {
		if a.Professions[prof.ID] > 0 {
			for _, g := range prof.Grants {
				if !ActorHasTag(tags, g) {
					tags = append(tags, g)
				}
			}
		}
	}
	return tags
}

func locomotionCostForActor(cat Catalog, ds *DistrictState, a Actor, r VenueRoute) (ticks int, energy int, modeID string, ok bool) {
	tags := actorEffectiveTags(cat, a)
	if r.RequiredTag != "" && !ActorHasTag(tags, r.RequiredTag) {
		return 0, 0, "", false
	}
	medium := r.Medium
	if medium == "" {
		medium = "road"
	}
	baseTicks := maxInt(1, r.Ticks)
	baseEnergy := maxInt(0, r.EnergyCost)

	bestTicks := 1 << 29
	bestEnergy := 0
	bestMode := ""

	tryMode := func(id string, speedPct, energyPct int, bypassWeather bool) {
		if speedPct <= 0 {
			speedPct = 100
		}
		if energyPct <= 0 {
			energyPct = 100
		}
		t := (baseTicks*100 + speedPct - 1) / speedPct
		if t < 1 {
			t = 1
		}
		e := (baseEnergy * energyPct) / 100
		if baseEnergy > 0 && e < 1 {
			e = 1
		}
		if !bypassWeather && ds != nil {
			if ws, exists := cat.Weathers[ds.Weather]; exists {
				if ws.TravelTickPenalty > 0 {
					t += ws.TravelTickPenalty
				}
				capPct := ws.RouteCapacityPct[medium]
				if capPct > 0 && capPct < 100 {
					t = maxInt(1, (t*100+capPct-1)/capPct)
				}
				if ws.TravelEnergyPenalty > 0 {
					e += ws.TravelEnergyPenalty
				}
			}
		}
		if t < bestTicks || (t == bestTicks && (e < bestEnergy || (e == bestEnergy && id < bestMode))) {
			bestTicks = t
			bestEnergy = e
			bestMode = id
		}
	}

	// Baseline pedestrian/boat access on standard ground/footpath/water routes.
	if medium == "road" || medium == "footpath" {
		tryMode("walk", 100, 100, false)
	} else if medium == "river" || medium == "water" {
		if ActorHasTag(tags, "water_carrier") || ActorHasTag(tags, "water_user") {
			tryMode("boat", 100, 100, false)
		}
	}

	if len(cat.LocomotionModes) > 0 {
		modeKeys := make([]string, 0, len(cat.LocomotionModes))
		for k := range cat.LocomotionModes {
			modeKeys = append(modeKeys, k)
		}
		sort.Strings(modeKeys)
		for _, k := range modeKeys {
			spec := cat.LocomotionModes[k]
			if len(spec.GrantedByTags) > 0 {
				granted := false
				for _, gt := range spec.GrantedByTags {
					if ActorHasTag(tags, gt) {
						granted = true
						break
					}
				}
				if !granted {
					continue
				}
			}
			allowed := len(spec.AllowedMediums) == 0
			for _, am := range spec.AllowedMediums {
				if am == medium || am == "*" {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
			tryMode(spec.ID, spec.SpeedPct, spec.EnergyPct, spec.BypassWeather)
		}
	}

	if bestMode == "" {
		return 0, 0, "", false
	}
	return bestTicks, bestEnergy, bestMode, true
}

func (m WorldMap) NextVenueHopForActor(cat Catalog, dsByDistrict map[string]DistrictState, a Actor, fromVenue, toVenue string) (VenueRoute, int, string, bool) {
	if fromVenue == "" || toVenue == "" || fromVenue == toVenue || len(m.Venues) == 0 {
		return VenueRoute{}, 0, "", false
	}
	const inf = 1 << 29
	dist := make(map[string]int, len(m.Venues))
	firstHop := make(map[string]VenueRoute, len(m.Venues))
	firstMode := make(map[string]string, len(m.Venues))
	visited := make(map[string]bool, len(m.Venues))
	venueDistrict := make(map[string]string, len(m.Venues))
	for _, v := range m.Venues {
		dist[v.ID] = inf
		venueDistrict[v.ID] = v.DistrictID
	}
	if _, ok := dist[fromVenue]; !ok {
		return VenueRoute{}, 0, "", false
	}
	if _, ok := dist[toVenue]; !ok {
		return VenueRoute{}, 0, "", false
	}
	dist[fromVenue] = 0

	for range m.Venues {
		u := ""
		best := inf
		for _, v := range m.Venues {
			if !visited[v.ID] && dist[v.ID] < best {
				best = dist[v.ID]
				u = v.ID
			}
		}
		if u == "" || u == toVenue {
			break
		}
		visited[u] = true
		var dsPtr *DistrictState
		if dsByDistrict != nil {
			if ds, ok := dsByDistrict[venueDistrict[u]]; ok {
				dsCopy := ds
				dsPtr = &dsCopy
			}
		}
		for _, edge := range m.VenueRoutesFrom(u) {
			hopTicks, _, modeID, passable := locomotionCostForActor(cat, dsPtr, a, edge)
			if !passable {
				continue
			}
			cand := dist[u] + hopTicks
			if cand < dist[edge.ToVenue] {
				dist[edge.ToVenue] = cand
				if u == fromVenue {
					firstHop[edge.ToVenue] = edge
					firstMode[edge.ToVenue] = modeID
				} else {
					firstHop[edge.ToVenue] = firstHop[u]
					firstMode[edge.ToVenue] = firstMode[u]
				}
			}
		}
	}
	if dist[toVenue] >= inf {
		return VenueRoute{}, 0, "", false
	}
	return firstHop[toVenue], dist[toVenue], firstMode[toVenue], true
}

func buildVenueRouteViews(m WorldMap, cat Catalog, dsByDistrict map[string]DistrictState, a Actor) []VenueRouteView {
	if a.InsideHouse != "" || a.VenueID == "" || len(m.VenueRoutes) == 0 {
		return nil
	}
	edges := m.VenueRoutesFrom(a.VenueID)
	if len(edges) == 0 {
		return nil
	}
	var dsPtr *DistrictState
	if dsByDistrict != nil {
		if ds, ok := dsByDistrict[a.Location]; ok {
			dsCopy := ds
			dsPtr = &dsCopy
		}
	}
	var views []VenueRouteView
	for _, r := range edges {
		ticks, energy, modeID, ok := locomotionCostForActor(cat, dsPtr, a, r)
		if !ok {
			continue
		}
		destV, _ := m.VenueByID(r.ToVenue)
		views = append(views, VenueRouteView{
			ToVenue:    r.ToVenue,
			ToDistrict: destV.DistrictID,
			Medium:     r.Medium,
			ModeID:     modeID,
			Ticks:      ticks,
			EnergyCost: energy,
		})
	}
	// One shortest-path result supplies both venue and district directions.
	// A district is reached at its nearest accessible venue.
	type districtPath struct{ viewIndex, ticks int }
	best := make(map[string]districtPath)
	for _, dest := range m.Venues {
		hop, ticks, _, reachable := m.NextVenueHopForActor(cat, dsByDistrict, a, a.VenueID, dest.ID)
		if !reachable {
			continue
		}
		for i := range views {
			if views[i].ToVenue != hop.ToVenue {
				continue
			}
			if dest.ID != hop.ToVenue {
				views[i].LeadsToward = append(views[i].LeadsToward, dest.ID)
			}
			if prior, exists := best[dest.DistrictID]; !exists || ticks < prior.ticks {
				best[dest.DistrictID] = districtPath{i, ticks}
			}
			break
		}
	}
	for _, district := range m.Districts {
		if path, found := best[district.ID]; found && district.ID != a.Location {
			view := &views[path.viewIndex]
			view.LeadsTowardDistricts = append(view.LeadsTowardDistricts, district.ID)
		}
	}
	return views
}

func buildPolityPerceptionView(m WorldMap, net *PolityNetwork, a Actor) *PolityPerceptionView {
	if net == nil || len(net.Nodes) == 0 {
		return nil
	}
	localID := m.PolityIDForLocation(a.Location, a.VenueID)
	led := net.NodeLedBy(a.ID)
	if localID == "" && led == nil {
		return nil
	}
	view := &PolityPerceptionView{}
	if localID != "" {
		chain := net.EffectivePolityChain(localID)
		if len(chain) > 0 {
			local := chain[0]
			top := chain[len(chain)-1]
			view.LocalPolityID = local.ID
			view.LocalPolityName = local.Name
			view.LocalPolityLevel = local.Level
			view.EffectiveRulerID = top.ID
			embargoSet := map[string]bool{}
			// Inherit from top superior down to local polity so explicit local settings override defaults,
			// while superior curfews and embargoes apply across obeyed subordinate jurisdictions.
			for idx := len(chain) - 1; idx >= 0; idx-- {
				n := chain[idx]
				if n.MarketTaxPermille > 0 {
					view.MarketTaxPermille = n.MarketTaxPermille
				} else if n.MarketTaxPermille < 0 {
					view.MarketTaxPermille = 0
				}
				if n.Curfew {
					view.Curfew = true
				}
				for _, item := range n.EmbargoItems {
					embargoSet[item] = true
				}
			}
			if len(embargoSet) > 0 {
				view.EmbargoItems = make([]string, 0, len(embargoSet))
				for k := range embargoSet {
					view.EmbargoItems = append(view.EmbargoItems, k)
				}
				sort.Strings(view.EmbargoItems)
			}
		}
	}
	if led != nil {
		view.LedPolityID = led.ID
		view.LedPolityName = led.Name
		view.LedPolityTreasury = led.Treasury
		view.LedPolitySeatVenue = led.SeatVenueID
		view.LedPolitySeatDistrict = led.SeatDistrictID
		view.LedPolityCurfew = led.Curfew
		view.LedPolityMarketTax = led.MarketTaxPermille
	}
	return view
}

// cohortHasJurisdiction requires physical presence. A cohort explicitly affiliated
// with a polity can enforce law only where that polity actually governs.
func (w *World) cohortHasJurisdiction(c *PopulationCohort, districtID string) bool {
	if c == nil || c.District != districtID || c.EffectivePopulation() <= 0 {
		return false
	}
	if w.Polities != nil && w.Polities.NodeByID(c.FactionID) != nil {
		for _, node := range w.effectivePolityChainAt(districtID, "") {
			if node.ID == c.FactionID {
				return true
			}
		}
		return false
	}
	return true
}

func (w *World) presentLawCollector(districtID string) *PopulationCohort {
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if w.cohortHasJurisdiction(c, districtID) && w.isLawCollectorCohort(c) {
			return c
		}
	}
	return nil
}

func (w *World) effectivePolityChainAt(districtID, venueID string) []PolityNode {
	if w.Polities == nil {
		return nil
	}
	pid := w.Map.PolityIDForLocation(districtID, venueID)
	if pid == "" {
		return nil
	}
	return w.Polities.EffectivePolityChain(pid)
}

// EffectiveGoverningPolity returns the immediate effective PolityNode for a district/venue.
func (w *World) EffectiveGoverningPolity(districtID, venueID string) *PolityNode {
	chain := w.effectivePolityChainAt(districtID, venueID)
	if len(chain) == 0 {
		return nil
	}
	return w.Polities.NodeByID(chain[0].ID)
}

func (w *World) EffectiveMarketTaxPermille(districtID, venueID string) int {
	chain := w.effectivePolityChainAt(districtID, venueID)
	if len(chain) == 0 {
		return 0
	}
	rate := 0
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].MarketTaxPermille > 0 {
			rate = chain[i].MarketTaxPermille
		} else if chain[i].MarketTaxPermille < 0 {
			rate = 0
		}
	}
	return rate
}

func (w *World) EffectivePollTaxPermille(districtID string, fallback int) (rate int, targetPolity *PolityNode) {
	chain := w.effectivePolityChainAt(districtID, "")
	if len(chain) == 0 {
		return fallback, nil
	}
	rate = fallback
	for i := len(chain) - 1; i >= 0; i-- {
		if chain[i].PollTaxPermille > 0 {
			rate = chain[i].PollTaxPermille
		} else if chain[i].PollTaxPermille < 0 {
			rate = 0
		}
	}
	return rate, w.Polities.NodeByID(chain[0].ID)
}

func (w *World) IsCurfewActiveAt(districtID, venueID string) bool {
	for _, n := range w.effectivePolityChainAt(districtID, venueID) {
		if n.Curfew {
			return true
		}
	}
	return false
}

func (w *World) IsItemEmbargoedAt(districtID, venueID, itemID string) bool {
	if itemID == "" {
		return false
	}
	for _, n := range w.effectivePolityChainAt(districtID, venueID) {
		for _, emb := range n.EmbargoItems {
			if emb == itemID {
				return true
			}
		}
	}
	return false
}

func (w *World) actorEnclosure(a Actor) (indoor bool, shielding int) {
	if a.VenueID != "" {
		if v, ok := w.Map.VenueByID(a.VenueID); ok {
			if v.Indoor {
				indoor = true
			}
			shielding += v.Shielding
		}
	}
	if a.InsideHouse != "" {
		indoor = true
		if h := w.house(a.InsideHouse); h != nil {
			if h.Shielding > 0 {
				shielding += h.Shielding
			} else {
				shielding += 10 + h.Level*5
			}
		}
	}
	shielding += w.activeShieldingBonusForActor(a)
	return indoor, shielding
}

func (w *World) siteMaxShielding(districtID, venueID string) (indoor bool, shielding int) {
	bonus := w.activeShieldingBonusForVenue(districtID, venueID)
	if venueID != "" {
		if v, ok := w.Map.VenueByID(venueID); ok {
			return v.Indoor, v.Shielding + bonus
		}
	}
	return false, bonus
}

// distributeAnomalyObservations evaluates every living awake actor against the physical site and
// shielding of a macro anomaly, granting a first-hand Observation ONLY to:
//  1. On-site witnesses not sealed inside an enclosure stronger than the anomaly intensity; or
//  2. Remote observers equipped/tagged with a SensorySpec whose RangeKm reaches the site and whose
//     PenetrationPower overcomes the site's shielding.
func (w *World) distributeAnomalyObservations(ev Event, anomalyID, flowKind, regionID, districtID string, intensity int) int {
	if intensity <= 0 {
		intensity = 25
	}
	var siteCoord GeoCoord
	hasSiteCoord := false
	if regionID != "" {
		if reg, ok := w.Map.GeoRegionByID(regionID); ok {
			siteCoord = reg.Coord
			hasSiteCoord = true
		}
	} else if districtID != "" {
		siteCoord, hasSiteCoord = w.Map.GeoCoordOfLocation(districtID, "")
	}
	siteLabel := districtID
	if siteLabel == "" {
		siteLabel = regionID
	}

	witnessCount := 0
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead || w.Tick < a.JailedUntil {
			continue
		}
		if a.Task != nil && (a.Task.Action.Kind == "sleep" || a.Task.Dormant) {
			continue
		}
		actorRegID := w.Map.GeoRegionIDForLocation(a.Location, a.VenueID)
		onSite := (districtID != "" && a.Location == districtID) || (regionID != "" && actorRegID == regionID)
		indoor, encShielding := w.actorEnclosure(*a)

		modality := ""
		if onSite && (!indoor || encShielding < intensity) {
			modality = "eyewitness"
		} else if len(w.Catalog.SensoryModes) > 0 && hasSiteCoord {
			actorCoord, hasActorCoord := w.Map.GeoCoordOfLocation(a.Location, a.VenueID)
			if hasActorCoord {
				distKm := GeoDistanceKm(actorCoord, siteCoord)
				tags := w.actorEffectiveTags(*a)
				modeKeys := make([]string, 0, len(w.Catalog.SensoryModes))
				for k := range w.Catalog.SensoryModes {
					modeKeys = append(modeKeys, k)
				}
				sort.Strings(modeKeys)
				for _, k := range modeKeys {
					spec := w.Catalog.SensoryModes[k]
					if distKm > spec.RangeKm {
						continue
					}
					granted := false
					for _, gt := range spec.GrantedByTags {
						if ActorHasTag(tags, gt) {
							granted = true
							break
						}
					}
					if !granted {
						continue
					}
					// Outgoing sensory scan must also penetrate the observer's own sealed enclosure if inside one.
					if indoor && !spec.PenetratesIndoor && encShielding > spec.PenetrationPower {
						continue
					}
					modality = spec.ID
					break
				}
			}
		}
		if modality == "" {
			continue
		}

		obs := Observation{
			Kind:      "macro_anomaly",
			Modality:  modality,
			SubjectID: anomalyID,
			Item:      flowKind,
			Location:  siteLabel,
			Quantity:  intensity,
		}
		mem := Memory{
			Kind:        "macro_anomaly",
			Tick:        w.Tick,
			EventID:     ev.ID,
			RootEventID: ev.ID,
			Source:      "world",
			Text:        fmt.Sprintf("Observed %s at %s via %s (intensity %d)", anomalyID, siteLabel, modality, intensity),
			Observation: &obs,
		}
		a.Memories = appendMemory(a.Memories, mem)
		witnessCount++
	}
	return witnessCount
}

// CanSenseVenueRemotely checks whether observer `a` has a SensorySpec able to observe `targetVenueID`
// from `a`'s current location, respecting both distance and target venue shielding.
func (w *World) CanSenseVenueRemotely(a Actor, targetDistrictID, targetVenueID string) (string, bool) {
	if a.Dead || len(w.Catalog.SensoryModes) == 0 {
		return "", false
	}
	srcCoord, ok1 := w.Map.GeoCoordOfLocation(a.Location, a.VenueID)
	dstCoord, ok2 := w.Map.GeoCoordOfLocation(targetDistrictID, targetVenueID)
	if !ok1 || !ok2 {
		return "", false
	}
	distKm := GeoDistanceKm(srcCoord, dstCoord)
	targetIndoor, targetShield := w.siteMaxShielding(targetDistrictID, targetVenueID)
	observerIndoor, observerShield := w.actorEnclosure(a)
	tags := w.actorEffectiveTags(a)

	modeKeys := make([]string, 0, len(w.Catalog.SensoryModes))
	for k := range w.Catalog.SensoryModes {
		modeKeys = append(modeKeys, k)
	}
	sort.Strings(modeKeys)
	for _, k := range modeKeys {
		spec := w.Catalog.SensoryModes[k]
		if distKm > spec.RangeKm {
			continue
		}
		granted := false
		for _, gt := range spec.GrantedByTags {
			if ActorHasTag(tags, gt) {
				granted = true
				break
			}
		}
		if !granted {
			continue
		}
		if (targetIndoor || observerIndoor) && !spec.PenetratesIndoor {
			continue
		}
		if observerShield > spec.PenetrationPower || targetShield > spec.PenetrationPower {
			continue
		}
		return spec.ID, true
	}
	return "", false
}

// TriggerGeoAnomaly executes a natural/supernatural anomaly at spec.OriginRegionID and cascades it
// physically along matching GeoFlows (e.g. downstream river flood across county borders or fallout
// along a wind corridor), applying infrastructure & shielding mitigation and recording first-hand
// Observations strictly on qualifying witnesses.
func (w *World) TriggerGeoAnomaly(spec GeoAnomalySpec) []Event {
	if spec.OriginRegionID == "" || spec.Intensity <= 0 {
		return nil
	}
	regIdxByID := make(map[string]int, len(w.Map.GeoRegions))
	for i, r := range w.Map.GeoRegions {
		regIdxByID[r.ID] = i
	}
	if _, exists := regIdxByID[spec.OriginRegionID]; !exists {
		return nil
	}

	type queueItem struct {
		regionID  string
		intensity int
		hop       int
	}
	bestIntensity := map[string]int{spec.OriginRegionID: spec.Intensity}
	visitOrder := []string{spec.OriginRegionID}
	queue := []queueItem{{regionID: spec.OriginRegionID, intensity: spec.Intensity, hop: 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if spec.FlowKind == "" {
			continue
		}
		srcReg := w.Map.GeoRegions[regIdxByID[cur.regionID]]
		for _, edge := range w.Map.GeoFlows {
			if edge.FromRegion != cur.regionID || edge.Kind != spec.FlowKind || edge.ConductancePermille <= 0 {
				continue
			}
			dstIdx, ok := regIdxByID[edge.ToRegion]
			if !ok {
				continue
			}
			dstReg := w.Map.GeoRegions[dstIdx]
			// Gravity check for surface water floods: rivers do not flow uphill.
			if edge.Kind == "river" && dstReg.ElevationM > srcReg.ElevationM {
				continue
			}
			nextInt := (cur.intensity * edge.ConductancePermille) / 1000
			if nextInt < 15 {
				continue
			}
			if prev, seen := bestIntensity[edge.ToRegion]; !seen || nextInt > prev {
				if !seen {
					visitOrder = append(visitOrder, edge.ToRegion)
				}
				bestIntensity[edge.ToRegion] = nextInt
				queue = append(queue, queueItem{regionID: edge.ToRegion, intensity: nextInt, hop: cur.hop + 1})
			}
		}
	}

	dur := spec.DurationTicks
	if dur <= 0 {
		dur = TicksPerDay
	}

	var events []Event
	for _, regID := range visitOrder {
		rawInt := bestIntensity[regID]
		rIdx := regIdxByID[regID]
		reg := &w.Map.GeoRegions[rIdx]

		if len(spec.AmbientFieldDelta) > 0 {
			if reg.AmbientFields == nil {
				reg.AmbientFields = make(map[string]int, len(spec.AmbientFieldDelta))
			}
			for fk, delta := range spec.AmbientFieldDelta {
				scaledDelta := (delta * rawInt) / spec.Intensity
				reg.AmbientFields[fk] += scaledDelta
			}
		}
		for poolID, lossPct := range spec.ResourceDamagePct {
			if reg.Resources != nil && reg.Resources[poolID] > 0 {
				effLoss := (lossPct * rawInt) / 100
				if effLoss > 95 {
					effLoss = 95
				}
				reg.Resources[poolID] -= (reg.Resources[poolID] * int64(effLoss)) / 100
				if reg.Resources[poolID] < 1 {
					reg.Resources[poolID] = 1
				}
			}
		}

		// Collect all Districts anchored to this GeoRegion.
		var affectedDistricts []string
		for _, d := range w.Map.Districts {
			if d.GeoRegionID == regID {
				affectedDistricts = append(affectedDistricts, d.ID)
			}
		}
		primaryDistrict := ""
		if len(affectedDistricts) > 0 {
			primaryDistrict = affectedDistricts[0]
		}

		maxMitigationPct := 0
		for _, dID := range affectedDistricts {
			infraMitPct := 0
			if w.DistrictStates != nil {
				if ds, ok := w.DistrictStates[dID]; ok {
					if spec.MitigatedByInfra != "" && ds.Infrastructure != nil {
						infraCond := ds.Infrastructure[spec.MitigatedByInfra]
						infraMitPct = (infraCond * 60) / 100
					}
					ds.ActiveDisasters = append(ds.ActiveDisasters, ActiveDisaster{
						ID:          spec.ID,
						Name:        spec.Name,
						StartedTick: w.Tick,
						ExpiresTick: w.Tick + dur,
					})
					dmgScale := maxInt(25, 100-infraMitPct)
					for poolID, lossPct := range spec.ResourceDamagePct {
						if ds.Resources != nil && ds.Resources[poolID] > 0 {
							effLoss := (lossPct * rawInt * dmgScale) / 10000
							loss := min(ds.Resources[poolID], ds.Resources[poolID]*int64(effLoss)/100)
							ds.Resources[poolID] -= loss
							w.recordEcologicalChange(dID, poolID, -loss)
						}
					}
					w.DistrictStates[dID] = ds
				}
			}
			if infraMitPct > maxMitigationPct {
				maxMitigationPct = infraMitPct
			}
			dmgScale := maxInt(25, 100-infraMitPct)
			if spec.HouseDamage > 0 {
				for hi := range w.Houses {
					if w.Houses[hi].District != dID {
						continue
					}
					shield := w.Houses[hi].Shielding
					if w.Houses[hi].VenueID != "" {
						if v, ok := w.Map.VenueByID(w.Houses[hi].VenueID); ok {
							shield += v.Shielding
						}
					}
					shield += w.activeShieldingBonusForVenue(dID, w.Houses[hi].VenueID)
					if shield >= rawInt {
						continue
					}
					hd := maxInt(1, (spec.HouseDamage*rawInt*dmgScale)/10000)
					w.Houses[hi].Condition -= hd
					if w.Houses[hi].Condition < 0 {
						w.Houses[hi].Condition = 0
					}
				}
			}
		}

		effInt := (rawInt * maxInt(25, 100-maxMitigationPct)) / 100
		if effInt < 1 {
			effInt = 1
		}

		woundedActors := 0
		deadActors := 0
		var deathEvents []Event
		if spec.ActorHealthDamage > 0 {
			for ai := range w.Actors {
				a := &w.Actors[ai]
				if a.Dead {
					continue
				}
				if w.Map.GeoRegionIDForLocation(a.Location, a.VenueID) != regID {
					continue
				}
				_, shield := w.actorEnclosure(*a)
				if shield >= effInt {
					continue
				}
				netPct := maxInt(0, effInt-shield)
				dmg := (spec.ActorHealthDamage * netPct) / 100
				if dmg < 1 && netPct > 0 {
					dmg = 1
				}
				if dmg <= 0 {
					continue
				}
				if dmg >= a.Health {
					a.Health = 0
					deadActors++
					deathEvents = append(deathEvents, w.markDeadIfZero(a, nil, Action{Kind: "wait", Location: a.Location}, "disaster")...)
				} else {
					a.Health -= dmg
					woundedActors++
					deathEvents = append(deathEvents, w.interruptTaskOnDamage(a, "disaster")...)
				}
			}
		}

		ev := Event{
			ID:      w.NextID,
			Tick:    w.Tick,
			ActorID: "world",
			Kind:    "geo_anomaly_struck",
			Action:  Action{Kind: "wait", Location: primaryDistrict},
			Payload: map[string]any{
				"text":           fmt.Sprintf("%s swept %s (intensity %d, infra mitigation %d%%, %d injured, %d perished)", spec.Name, reg.Name, effInt, maxMitigationPct, woundedActors, deadActors),
				"anomaly_id":     spec.ID,
				"anomaly_name":   spec.Name,
				"geo_region_id":  regID,
				"district":       primaryDistrict,
				"flow_kind":      spec.FlowKind,
				"intensity":      effInt,
				"mitigation_pct": maxMitigationPct,
				"wounded":        woundedActors,
				"deaths":         deadActors,
			},
		}
		w.NextID++
		witnesses := w.distributeAnomalyObservations(ev, spec.ID, spec.FlowKind, regID, primaryDistrict, effInt)
		ev.Payload["witnesses"] = witnesses
		events = append(events, ev)
		events = append(events, deathEvents...)
	}
	return events
}

// communicationRangeAllows uses verified coordinates for finite remote channels.
// An exact shared place needs no coordinates; range zero is the channel's district-local mode.
func (w *World) communicationRangeAllows(sender, recipient Actor, rangeKm int) bool {
	if rangeKm == 0 {
		return sender.Location == recipient.Location
	}
	if SamePlace(sender, recipient) {
		return true
	}
	src, hasSrc := w.Map.GeoCoordOfLocation(sender.Location, sender.VenueID)
	dst, hasDst := w.Map.GeoCoordOfLocation(recipient.Location, recipient.VenueID)
	return hasSrc && hasDst && GeoDistanceKm(src, dst) <= rangeKm
}

// perceivedActiveEffects exposes only present effects that the observer can perceive.
// Only an effect's owner receives its internal strength and upkeep state.
func (w *World) perceivedActiveEffects(a Actor, effects []ActiveEffect) []ActiveEffect {
	var visible []ActiveEffect
	for _, eff := range effects {
		if eff.ExpiresTick > 0 && w.Tick >= eff.ExpiresTick {
			continue
		}
		owned := eff.Source.Kind == "actor" && eff.Source.ID == a.ID || eff.SustainActorID == a.ID
		if owned {
			visible = append(visible, CloneActiveEffect(eff))
			continue
		}
		if w.effectAppliesToActor(eff, a) && len(eff.GrantTags) > 0 {
			visible = append(visible, ActiveEffect{ID: eff.ID, SpecID: eff.SpecID, Name: eff.Name, Anchor: eff.Anchor, GrantTags: append([]string(nil), eff.GrantTags...)})
			continue
		}
		if a.Dead || a.InTransit() || a.Task != nil && (a.Task.Action.Kind == "sleep" || a.Task.Dormant) {
			continue
		}
		district, venue := "", ""
		local := false
		targetIndoor, targetShield := false, 0
		switch eff.Anchor.Kind {
		case "actor":
			target := w.actor(eff.Anchor.ID)
			if target == nil || target.Dead || target.InTransit() {
				continue
			}
			district, venue = target.Location, target.VenueID
			targetIndoor, targetShield = w.actorEnclosure(*target)
			local = SamePlace(a, *target)
		case "venue":
			site, ok := w.Map.VenueByID(eff.Anchor.ID)
			if !ok {
				continue
			}
			district, venue = site.DistrictID, site.ID
			local = a.InsideHouse == "" && a.VenueID == venue
		case "district":
			district = eff.Anchor.ID
			indoor, _ := w.actorEnclosure(a)
			local = !indoor && a.Location == district
		}
		if !local {
			mode, sensed := w.CanSenseVenueRemotely(a, district, venue)
			if !sensed {
				continue
			}
			spec := w.Catalog.SensoryModes[mode]
			if targetIndoor && !spec.PenetratesIndoor || targetShield > spec.PenetrationPower {
				continue
			}
		}
		visible = append(visible, ActiveEffect{ID: eff.ID, SpecID: eff.SpecID, Name: eff.Name, Anchor: eff.Anchor})
	}
	return visible
}

// applyMoveDistrict follows the venue graph when places are configured, so district
// travel cannot bypass a boat, flight, or other declared route capability.
func (w *World) applyMoveDistrict(a *Actor, x Action) []Event {
	from := a.VenueID
	traveler := w.actorWithEffectTags(*a)
	var best *VenueRouteView
	for _, hop := range buildVenueRouteViews(w.Map, w.Catalog, w.DistrictStates, traveler) {
		if hop.ToDistrict == x.Location && (best == nil || hop.Ticks < best.Ticks || hop.Ticks == best.Ticks && hop.ToVenue < best.ToVenue) {
			candidate := hop
			best = &candidate
		}
	}
	if best == nil {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": fmt.Sprintf("no traversable venue route from %s to district %s; travel through connected venues first", from, x.Location)})}
	}
	x.Kind, x.Location = "move_venue", best.ToVenue
	return w.applyMoveVenue(a, x)
}

func (w *World) applyMoveVenue(a *Actor, x Action) []Event {
	emitFail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if a.InsideHouse != "" {
		return emitFail("leave house before traveling between venues")
	}
	destVenue, ok := w.Map.VenueByID(x.Location)
	if !ok {
		return emitFail("unknown destination venue: " + x.Location)
	}
	fromVenue := a.VenueID
	if fromVenue == "" {
		return emitFail("actor has no current venue to depart from")
	}
	route, connected := w.Map.VenueRouteBetween(fromVenue, destVenue.ID)
	if !connected {
		return emitFail(fmt.Sprintf("no direct venue route from %s to %s", fromVenue, destVenue.ID))
	}
	var dsPtr *DistrictState
	if w.DistrictStates != nil {
		if ds, exists := w.DistrictStates[a.Location]; exists {
			dsCopy := ds
			dsPtr = &dsCopy
		}
	}
	effTicks, effEnergy, modeID, passable := locomotionCostForActor(w.Catalog, dsPtr, w.actorWithEffectTags(*a), route)
	if !passable {
		return emitFail(fmt.Sprintf("cannot traverse %s route from %s to %s without required locomotion capability", route.Medium, fromVenue, destVenue.ID))
	}
	if severed, reason := w.isRouteSevered(a.Location, destVenue.DistrictID, route.Medium, modeID); severed {
		return emitFail(reason)
	}
	if a.Energy < effEnergy {
		return emitFail("insufficient energy to travel to " + destVenue.ID)
	}
	a.Energy -= effEnergy
	x.Channel = modeID
	task := Task{Action: x, TotalTicks: effTicks, Remaining: effTicks - 1, StartedTick: w.Tick}
	if task.Remaining <= 0 {
		return w.finishMoveVenue(a, task)
	}
	a.Task = &task
	return []Event{w.record(a, x, "task_started", map[string]any{
		"text":            fmt.Sprintf("%s set out via %s toward %s (%d ticks)", a.Name, modeID, destVenue.ID, effTicks),
		"venue_id":        destVenue.ID,
		"locomotion_mode": modeID,
		"remaining_ticks": task.Remaining,
	})}
}

func (w *World) finishMoveVenue(a *Actor, t Task) []Event {
	x := t.Action
	destVenue, ok := w.Map.VenueByID(x.Location)
	if !ok {
		return nil
	}
	fromVenue := a.VenueID
	fromDistrict := a.Location
	medium := "road"
	if route, connected := w.Map.VenueRouteBetween(fromVenue, destVenue.ID); connected && route.Medium != "" {
		medium = route.Medium
	}
	a.VenueID = destVenue.ID
	if destVenue.DistrictID != "" {
		a.Location = destVenue.DistrictID
	}
	modeID := x.Channel
	if modeID == "" {
		modeID = "walk"
	}
	events := []Event{w.record(a, x, "moved_venue", map[string]any{
		"text":            fmt.Sprintf("%s traveled via %s to %s", a.Name, modeID, destVenue.ID),
		"venue_id":        destVenue.ID,
		"district":        a.Location,
		"locomotion_mode": modeID,
	})}
	return append(events, w.checkActorTransitHazard(a, x, fromDistrict, a.Location, medium, modeID)...)
}

func (w *World) applyIssueDecree(a *Actor, x Action) []Event {
	emitFail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if w.Polities == nil {
		return emitFail("no polity network configured")
	}
	var node *PolityNode
	if x.FactionID != "" {
		node = w.Polities.NodeByID(x.FactionID)
	} else {
		node = w.Polities.NodeLedBy(a.ID)
	}
	if node == nil || node.LeaderActorID != a.ID {
		return emitFail("actor does not hold executive authority over requested polity")
	}
	if node.SeatVenueID != "" && a.VenueID != "" && a.VenueID != node.SeatVenueID {
		return emitFail("ruler must be at administrative seat venue " + node.SeatVenueID + " to promulgate decrees")
	}
	if node.SeatDistrictID != "" && a.Location != node.SeatDistrictID {
		return emitFail("ruler must be in seat district " + node.SeatDistrictID + " to promulgate decrees")
	}
	if a.Energy < 4 {
		return emitFail("insufficient energy to issue decree")
	}

	kind := x.Item
	embargoTarget := ""
	if strings.HasPrefix(kind, "embargo:") {
		embargoTarget = strings.TrimPrefix(kind, "embargo:")
		kind = "embargo"
	}

	decreeVal := x.Quantity
	var reliefDistributed int64
	var reliefRecipients int

	switch kind {
	case "market_tax":
		if decreeVal < -1 || decreeVal > 500 {
			return emitFail("market tax permille out of valid bounds")
		}
		node.MarketTaxPermille = decreeVal
	case "poll_tax":
		if decreeVal < -1 || decreeVal > 500 {
			return emitFail("poll tax permille out of valid bounds")
		}
		node.PollTaxPermille = decreeVal
	case "curfew_on":
		node.Curfew = true
		decreeVal = 1
	case "curfew_off":
		node.Curfew = false
		decreeVal = 0
	case "embargo":
		if embargoTarget == "" {
			return emitFail("embargo decree requires target item")
		}
		node.EmbargoItems = appendUnique(node.EmbargoItems, embargoTarget)
	case "relief_grant":
		budget := int64(decreeVal)
		if budget <= 0 {
			return emitFail("relief grant must request a positive cash amount")
		}
		if node.Treasury < budget {
			budget = node.Treasury
		}
		if budget <= 0 {
			return emitFail("polity treasury lacks funds for relief grant")
		}
		needyActors, needyCohorts := w.reliefRecipients(node, a.ID)
		people := int64(len(needyActors))
		for _, cohort := range needyCohorts {
			people += int64(cohort.Population - len(cohort.Members))
		}
		if people == 0 {
			return emitFail("no eligible needy residents in governed jurisdiction for relief grant")
		}
		perPerson, remainder := budget/people, budget%people
		for _, recipient := range needyActors {
			amount := perPerson
			if remainder > 0 {
				amount++
				remainder--
			}
			recipient.Money += int(amount)
			reliefDistributed += amount
			if amount > 0 {
				reliefRecipients++
			}
		}
		for _, cohort := range needyCohorts {
			population := int64(cohort.Population - len(cohort.Members))
			extra := min(remainder, population)
			amount := perPerson*population + extra
			remainder -= extra
			cohort.MoneyPool += amount
			reliefDistributed += amount
			if perPerson > 0 {
				reliefRecipients += int(population)
			} else {
				reliefRecipients += int(extra)
			}
		}
		node.Treasury -= reliefDistributed
		node.Ledger.ReliefAndInfraOut += reliefDistributed
		if reliefDistributed == 0 {
			return emitFail("no eligible needy residents in governed jurisdiction for relief grant")
		}
		decreeVal = int(reliefDistributed)
	default:
		return emitFail("unsupported decree kind: " + x.Item)
	}

	a.Energy -= 4
	proclamation := x.Text
	if proclamation == "" {
		proclamation = fmt.Sprintf("%s promulgated %s decree (%s=%d) for %s", a.Name, kind, embargoTarget, decreeVal, node.Name)
	}
	decree := PolityDecree{
		ID:         w.NextID,
		PolityID:   node.ID,
		IssuerID:   a.ID,
		Kind:       kind,
		Item:       embargoTarget,
		Value:      decreeVal,
		Text:       proclamation,
		IssuedTick: w.Tick,
	}
	node.Decrees = append(node.Decrees, decree)
	x.FactionID = node.ID
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	ev := w.record(a, x, "decree_issued", map[string]any{
		"text":               proclamation,
		"polity_id":          node.ID,
		"polity_name":        node.Name,
		"decree_kind":        kind,
		"embargo_item":       embargoTarget,
		"value":              decreeVal,
		"relief_distributed": reliefDistributed,
		"relief_recipients":  reliefRecipients,
		"treasury_remaining": node.Treasury,
	}, observers...)
	return []Event{ev}
}

func (w *World) applyTransmitMessage(a *Actor, x Action) []Event {
	emitFail := func(reason string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": reason})}
	}
	if len(w.Catalog.CommChannels) == 0 {
		return emitFail("no communication channels configured in world catalog")
	}
	chID := x.Channel
	tags := w.actorEffectiveTags(*a)
	if chID == "" {
		keys := make([]string, 0, len(w.Catalog.CommChannels))
		for k := range w.Catalog.CommChannels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			spec := w.Catalog.CommChannels[k]
			for _, gt := range spec.GrantedByTags {
				if ActorHasTag(tags, gt) {
					chID = k
					break
				}
			}
			if chID != "" {
				break
			}
		}
	}
	ch, exists := w.Catalog.CommChannels[chID]
	if !exists {
		return emitFail("unknown communication channel: " + chID)
	}
	if len(ch.GrantedByTags) > 0 {
		granted := false
		for _, gt := range ch.GrantedByTags {
			if ActorHasTag(tags, gt) {
				granted = true
				break
			}
		}
		if !granted {
			return emitFail("actor lacks transmitter capability for channel " + ch.ID)
		}
	}
	if !ch.Broadcast && x.TargetID == "" {
		return emitFail("point-to-point channel requires target recipient")
	}
	if x.Text == "" {
		return emitFail("empty channel message")
	}
	if a.Energy < ch.EnergyCost {
		return emitFail("insufficient energy to transmit over " + ch.ID)
	}
	_, senderShield := w.actorEnclosure(*a)
	if senderShield > ch.PenetrationPower {
		return emitFail("sender enclosure shielding blocks outgoing transmission on " + ch.ID)
	}
	if !ch.Broadcast {
		target := w.actor(x.TargetID)
		if target == nil || target.Dead {
			return emitFail("unknown or dead channel recipient")
		}
		if !w.communicationRangeAllows(*a, *target, ch.RangeKm) {
			return emitFail("recipient is outside the channel range or its distance cannot be verified")
		}
	}

	a.Energy -= ch.EnergyCost
	x.Channel = ch.ID

	var recipients []*Actor
	for i := range w.Actors {
		rcpt := &w.Actors[i]
		if rcpt.Dead || rcpt.ID == a.ID || w.Tick < rcpt.JailedUntil {
			continue
		}
		if !ch.Broadcast && rcpt.ID != x.TargetID {
			continue
		}
		if rcpt.Task != nil && (rcpt.Task.Action.Kind == "sleep" || rcpt.Task.Dormant) {
			continue
		}
		if len(ch.ReceiverTags) > 0 {
			rTags := w.actorEffectiveTags(*rcpt)
			hasReceiver := false
			for _, rt := range ch.ReceiverTags {
				if ActorHasTag(rTags, rt) {
					hasReceiver = true
					break
				}
			}
			if !hasReceiver {
				continue
			}
		}
		if !w.communicationRangeAllows(*a, *rcpt, ch.RangeKm) {
			continue
		}
		_, rcptShield := w.actorEnclosure(*rcpt)
		if rcptShield > ch.PenetrationPower {
			continue
		}
		recipients = append(recipients, rcpt)
	}

	e := Event{
		ID:       w.NextID,
		Tick:     w.Tick,
		ActorID:  a.ID,
		TargetID: x.TargetID,
		Kind:     "channel_transmitted",
		Action:   x,
		Payload: map[string]any{
			"text":       x.Text,
			"channel_id": ch.ID,
		},
	}
	w.NextID++
	root := e.ID
	if x.BasisEventID != 0 {
		for _, m := range a.Memories {
			if m.EventID == x.BasisEventID {
				root = m.RootEventID
				break
			}
		}
	}
	e.Payload["basis_event_id"] = x.BasisEventID
	e.Payload["root_event_id"] = root

	var stmtPtr *Statement
	if x.Statement != nil {
		sc := *x.Statement
		stmtPtr = &sc
		e.Payload["statement"] = sc
	}

	senderMem := Memory{
		Kind:         "channel_message",
		Channel:      ch.ID,
		Tick:         w.Tick,
		EventID:      e.ID,
		Source:       a.ID,
		SourceRole:   a.Role,
		SourceTags:   append([]string(nil), a.IdentityTags()...),
		TargetID:     x.TargetID,
		Text:         x.Text,
		BasisEventID: x.BasisEventID,
		RootEventID:  root,
		Statement:    stmtPtr,
	}
	a.Memories = appendMemory(a.Memories, senderMem)

	deliveredIDs := make([]string, 0, len(recipients))
	for _, rcpt := range recipients {
		var rcptStmt *Statement
		if stmtPtr != nil {
			sc := *stmtPtr
			rcptStmt = &sc
		}
		// Remote channel delivery explicitly sets Channel + Statement and NEVER sets Observation.
		rcptMem := Memory{
			Kind:         "channel_message",
			Channel:      ch.ID,
			Tick:         w.Tick,
			EventID:      e.ID,
			Source:       a.ID,
			SourceRole:   a.Role,
			SourceTags:   append([]string(nil), a.IdentityTags()...),
			TargetID:     rcpt.ID,
			Text:         x.Text,
			BasisEventID: x.BasisEventID,
			RootEventID:  root,
			Statement:    rcptStmt,
		}
		rcpt.Memories = appendMemory(rcpt.Memories, rcptMem)
		deliveredIDs = append(deliveredIDs, rcpt.ID)
	}
	e.Payload["delivered_count"] = len(deliveredIDs)
	if len(deliveredIDs) > 0 {
		e.Payload["delivered_to"] = deliveredIDs
	}
	w.ObserveContinuity(e)
	return []Event{e}
}

package sim

// PrefectureMap returns the 11-district mountain-river-canal topology for large-scale worlds,
// combining northern mountains/mines/pastures with southern/central river plains and Grand Canal waterways.
func PrefectureMap() WorldMap {
	return WorldMap{
		Districts: []District{
			{ID: "west_cottages", Name: "西庄民坊", Terrain: "plains", Features: []string{"residential", "farmland"}},
			{ID: "creek_well", Name: "清溪井坊", Terrain: "river_basin", Features: []string{"water_source", "residential"}},
			{ID: "south_fields", Name: "南畴水田灌区", Terrain: "river_basin", Features: []string{"farmland", "forage_ground", "water_source"}},
			{ID: "jade_river_wharf", Name: "玉河渔港漕码头", Terrain: "river_basin", Features: []string{"water_source", "fishery_waters", "canal_wharf", "market", "residential"}},
			{ID: "crossroads", Name: "府城十字街衙署", Terrain: "urban", Features: []string{"hub", "residential", "academy", "yamen"}},
			{ID: "market_street", Name: "通济闹市漕埠", Terrain: "urban", Features: []string{"market", "canal_wharf", "residential", "gang_turf"}},
			{ID: "east_lane", Name: "东坊百工武馆", Terrain: "urban", Features: []string{"residential", "workshop_row", "martial_hall"}},
			{ID: "grand_shrine", Name: "社稷山川坛祠", Terrain: "plains", Features: []string{"shrine", "residential"}},
			{ID: "north_pasture", Name: "北麓水草牧场", Terrain: "steppe", Features: []string{"pasture", "forage_ground", "hunting_ground"}},
			{ID: "north_woods", Name: "苍山林场猎苑", Terrain: "forest_mountain", Features: []string{"woodland", "herb_patch", "forage_ground", "hunting_ground", "bandit_camp"}},
			{ID: "deep_iron_mine", Name: "黑石深井矿山", Terrain: "mountain", Features: []string{"mine_shaft", "woodland", "hunting_ground"}},
		},
		Roads: []Road{
			// Waterway Canal & River Network (漕运黄金水道: 大运量、低运费)
			{From: "south_fields", To: "jade_river_wharf", Ticks: 1, EnergyCost: 2, Medium: "waterway", FreightCapacity: 650000, FreightFeePermille: 20, BaseLossPermille: 3, CarrierTag: "water_carrier"},
			{From: "jade_river_wharf", To: "market_street", Ticks: 1, EnergyCost: 2, Medium: "waterway", FreightCapacity: 800000, FreightFeePermille: 20, BaseLossPermille: 3, CarrierTag: "water_carrier"},
			{From: "market_street", To: "crossroads", Ticks: 1, EnergyCost: 2, Medium: "waterway", FreightCapacity: 550000, FreightFeePermille: 20, BaseLossPermille: 2, CarrierTag: "water_carrier"},

			// Flatland Prefecture Official Roads (平原官道: 中运量、中运费)
			{From: "west_cottages", To: "creek_well", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 220000, FreightFeePermille: 45, BaseLossPermille: 5, CarrierTag: "land_carrier"},
			{From: "west_cottages", To: "crossroads", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 240000, FreightFeePermille: 45, BaseLossPermille: 5, CarrierTag: "land_carrier"},
			{From: "creek_well", To: "south_fields", Ticks: 1, EnergyCost: 3, Medium: "land", FreightCapacity: 240000, FreightFeePermille: 45, BaseLossPermille: 5, CarrierTag: "land_carrier"},
			{From: "creek_well", To: "crossroads", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 220000, FreightFeePermille: 45, BaseLossPermille: 5, CarrierTag: "land_carrier"},
			{From: "crossroads", To: "east_lane", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 260000, FreightFeePermille: 45, BaseLossPermille: 4, CarrierTag: "land_carrier"},
			{From: "east_lane", To: "market_street", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 280000, FreightFeePermille: 40, BaseLossPermille: 4, CarrierTag: "land_carrier"},
			{From: "crossroads", To: "grand_shrine", Ticks: 1, EnergyCost: 2, Medium: "land", FreightCapacity: 200000, FreightFeePermille: 40, BaseLossPermille: 3, CarrierTag: "land_carrier"},
			{From: "west_cottages", To: "north_pasture", Ticks: 2, EnergyCost: 3, Medium: "land", FreightCapacity: 200000, FreightFeePermille: 55, BaseLossPermille: 6, CarrierTag: "land_carrier"},

			// Mountain Passes & Forest Trails (崇山栈道: 险峻限重、高运费、易遭山洪猛兽劫道)
			{From: "crossroads", To: "north_woods", Ticks: 2, EnergyCost: 4, Medium: "mountain_pass", FreightCapacity: 150000, FreightFeePermille: 75, BaseLossPermille: 9, CarrierTag: "land_carrier"},
			{From: "east_lane", To: "north_woods", Ticks: 2, EnergyCost: 4, Medium: "mountain_pass", FreightCapacity: 160000, FreightFeePermille: 75, BaseLossPermille: 9, CarrierTag: "land_carrier"},
			{From: "north_pasture", To: "north_woods", Ticks: 2, EnergyCost: 4, Medium: "mountain_pass", FreightCapacity: 130000, FreightFeePermille: 80, BaseLossPermille: 10, CarrierTag: "land_carrier"},
			{From: "north_woods", To: "deep_iron_mine", Ticks: 2, EnergyCost: 5, Medium: "mountain_pass", FreightCapacity: 150000, FreightFeePermille: 85, BaseLossPermille: 12, CarrierTag: "land_carrier"},
			{From: "east_lane", To: "deep_iron_mine", Ticks: 3, EnergyCost: 5, Medium: "mountain_pass", FreightCapacity: 140000, FreightFeePermille: 85, BaseLossPermille: 10, CarrierTag: "land_carrier"},
		},
	}
}

// PrefectureFactions returns the full guild roster for large mountain-river worlds,
// adding waterways/canal-fishery, mountain hunters, and shrine ritual guilds alongside existing guilds.
func PrefectureFactions() []Faction {
	base := DefaultFactions()
	for i := range base {
		if base[i].ID == "farmers_guild" {
			base[i].Professions = []string{"farmer", "gatherer", "agronomist", "herder", "ranch_master"}
		}
	}
	extra := []Faction{
		{ID: "waterways_guild", Name: "漕渔总帮", District: "jade_river_wharf", Professions: []string{"fisher", "fleet_master", "boatman", "canal_master"}, Requirements: map[string]int{"strength": 7, "spirit": 7}},
		{ID: "hunters_guild", Name: "巡山猎行", District: "north_woods", Professions: []string{"hunter", "master_hunter"}, Requirements: map[string]int{"strength": 8, "dexterity": 8}},
		{ID: "shrine_guild", Name: "社稷奉常祠", District: "grand_shrine", Professions: []string{"ritualist", "high_priest"}, Requirements: map[string]int{"insight": 8, "spirit": 8}},
	}
	return append(base, extra...)
}

type freightRoutePlan struct {
	ticks          int64
	edges          []Road
	edgeKeys       []string
	feePermille    int
	lossPermille   int
	hazardPermille int
	primaryMedium  string
	carrierTag     string
}

// FreightCapacityUsage records all dispatches sharing a road during one tick.
// Persisting this state prevents a restored world from reusing the same capacity.
type FreightCapacityUsage struct {
	Tick        int64            `json:"tick"`
	UsedBatches map[string]int64 `json:"used_batches"`
}

// freightRouteAvailableBatches returns the remaining bottleneck capacity of a
// non-local route, using the road's existing seasonal and weather adjustments.
func (w *World) freightRouteAvailableBatches(plan freightRoutePlan) int64 {
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	var available int64
	for i, edge := range plan.edges {
		capacity, _, _, _, _, _ := w.edgeEffectiveLogistics(edge, cal)
		if w.FreightUsage != nil && w.FreightUsage.Tick == w.Tick {
			capacity -= w.FreightUsage.UsedBatches[plan.edgeKeys[i]]
		}
		if i == 0 || capacity < available {
			available = capacity
		}
	}
	return max(int64(0), available)
}

// reserveFreightRoute consumes batches already bounded by the available route
// capacity. Market clearing and harvest delivery both use this same record.
func (w *World) reserveFreightRoute(plan freightRoutePlan, batches int64) {
	if w.FreightUsage == nil || w.FreightUsage.Tick != w.Tick {
		w.FreightUsage = &FreightCapacityUsage{Tick: w.Tick, UsedBatches: make(map[string]int64)}
	}
	for _, edgeKey := range plan.edgeKeys {
		w.FreightUsage.UsedBatches[edgeKey] += batches
	}
}

func canonicalEdgeKey(a, b string) string {
	if a < b {
		return a + "<->" + b
	}
	return b + "<->" + a
}

func (w *World) edgeEffectiveLogistics(r Road, cal CalendarState) (capBatches int64, feePermille, lossPermille, hazardPermille int, medium, carrier string) {
	medium = r.Medium
	if medium == "" {
		medium = "land"
	}
	capBatches = r.FreightCapacity
	feePermille = r.FreightFeePermille

	lossPermille = r.BaseLossPermille
	carrier = r.CarrierTag
	if carrier == "" {
		carrier = "land_carrier"
	}

	if cal.SeasonIndex >= 0 && cal.SeasonIndex < len(w.Catalog.Calendar.Seasons) {
		season := w.Catalog.Calendar.Seasons[cal.SeasonIndex]
		if rPct, ok := season.RouteCapacityPct[medium]; ok {
			capBatches = (capBatches * int64(rPct)) / 100
		} else if medium == "waterway" && season.CanalCapacityPct > 0 {
			capBatches = (capBatches * int64(season.CanalCapacityPct)) / 100
		}
	}
	if w.DistrictStates != nil {
		for _, ep := range []string{r.From, r.To} {
			if ds, ok := w.DistrictStates[ep]; ok {
				if ws, hasW := w.Catalog.Weathers[ds.Weather]; hasW {
					if rPct, ok := ws.RouteCapacityPct[medium]; ok {
						capBatches = (capBatches * int64(rPct)) / 100
					}
					if h := ws.RouteHazardPermille[medium]; h > hazardPermille {
						hazardPermille = h
					}
				}
			}
		}
	}
	lossPermille += hazardPermille / 3

	return capBatches, feePermille, lossPermille, hazardPermille, medium, carrier
}

// planFreightRoute finds the lowest-fee permitted path for this cargo item.
// Embargoed jurisdictions are excluded during routing so an open bypass remains usable.
func (w *World) planFreightRoute(from, to, itemID string, cal CalendarState) (freightRoutePlan, bool) {
	if w.IsItemEmbargoedAt(from, "", itemID) || w.IsItemEmbargoedAt(to, "", itemID) {
		return freightRoutePlan{}, false
	}
	if from == to {
		return freightRoutePlan{primaryMedium: "local"}, true
	}
	const inf = 1 << 29
	cost := make(map[string]int, len(w.Map.Districts))
	prevEdge := make(map[string]Road, len(w.Map.Districts))
	visited := make(map[string]bool, len(w.Map.Districts))
	for _, d := range w.Map.Districts {
		cost[d.ID] = inf
	}
	if _, ok := cost[from]; !ok {
		return freightRoutePlan{}, false
	}
	if _, ok := cost[to]; !ok {
		return freightRoutePlan{}, false
	}
	cost[from] = 0

	for range w.Map.Districts {
		u := ""
		best := inf
		for _, d := range w.Map.Districts {
			if !visited[d.ID] && cost[d.ID] < best {
				best = cost[d.ID]
				u = d.ID
			}
		}
		if u == "" || u == to {
			break
		}
		visited[u] = true
		for _, edge := range w.Map.NeighborsOf(u) {
			if w.IsItemEmbargoedAt(edge.To, "", itemID) {
				continue
			}
			capacity, fee, _, _, _, _ := w.edgeEffectiveLogistics(edge, cal)
			if capacity <= 0 || edge.Ticks <= 0 {
				continue
			}
			if cost[u]+fee < cost[edge.To] {
				cost[edge.To] = cost[u] + fee
				prevEdge[edge.To] = edge
			}
		}
	}
	if cost[to] >= inf {
		return freightRoutePlan{}, false
	}

	var rev []Road
	cur := to
	for cur != from {
		e, ok := prevEdge[cur]
		if !ok {
			return freightRoutePlan{}, false
		}
		rev = append(rev, e)
		cur = e.From
	}
	plan := freightRoutePlan{
		edges:         make([]Road, len(rev)),
		edgeKeys:      make([]string, len(rev)),
		primaryMedium: "land",
		carrierTag:    "land_carrier",
	}
	waterwaySteps := 0
	for i := range rev {
		edge := rev[len(rev)-1-i]
		plan.edges[i] = edge
		ticks, _ := w.effectiveRoadCost(edge)
		plan.ticks += int64(ticks)
		plan.edgeKeys[i] = canonicalEdgeKey(edge.From, edge.To)
		_, fee, loss, haz, med, car := w.edgeEffectiveLogistics(edge, cal)
		plan.feePermille += fee
		plan.lossPermille += loss
		if haz > plan.hazardPermille {
			plan.hazardPermille = haz
		}
		if med == "waterway" {
			waterwaySteps++
			plan.carrierTag = car
		} else if plan.primaryMedium != "waterway" {
			if med != "land" {
				plan.primaryMedium = med
			}
			plan.carrierTag = car
		}
	}
	if waterwaySteps > 0 {
		plan.primaryMedium = "waterway"
	}
	if plan.lossPermille > 150 {
		plan.lossPermille = 150
	}
	return plan, true
}

// payFreightToCarriers distributes transport fees to cohorts holding carrierTag (e.g. boatmen for waterway, peddlers for land).
func (w *World) payFreightToCarriers(carrierTag string, feeCoins int64) *PopulationCohort {
	if feeCoins <= 0 {
		return nil
	}
	var carriers []*PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok {
			if ActorHasTag(prof.Grants, carrierTag) || (carrierTag == "land_carrier" && prof.TradeMargin) {
				carriers = append(carriers, c)
			}
		}
	}
	if len(carriers) == 0 {
		return nil
	}
	share := feeCoins / int64(len(carriers))
	rem := feeCoins % int64(len(carriers))
	for idx, c := range carriers {
		w.addCohortCurrency(c, "coins", share)
		if idx == 0 {
			w.addCohortCurrency(c, "coins", rem)
		}
	}
	return carriers[0]
}

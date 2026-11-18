package sim

import (
	"fmt"
	"runtime"
	"sort"
)

// MaxNearbyView caps the number of co-located neighbors materialized into a single Actor's Perception
// so high-density districts (hundreds or thousands of co-located actors) remain O(1) per actor.
// Set to 32 so all 29 neighbors in the 30-villager baseline are always visible without truncation.
const MaxNearbyView = 32

// PopulationCohort models a macro demographic segment of homogeneous villagers
// (grouped by district, faction, profession, and profession level) for million-scale simulation.
// Cohorts produce and consume catalog items in bulk, contribute influence to their faction,
// advance in profession XP/level/rank, promote into Tier-2 professions, and can dynamically
// hydrate/dehydrate individual Actors on demand.
type PopulationCohort struct {
	Residents   []ResidentBand `json:"residents,omitempty"`
	ResidentSeq int            `json:"resident_seq,omitempty"`
	// Members retain the identities and exact state of previously materialized people.
	// Their assets belong to these records, not to MoneyPool or ItemPool.
	Members               []Actor `json:"members,omitempty"`
	PendingWoundedDeaths  int     `json:"pending_wounded_deaths,omitempty"`
	PendingSickDiedOther  int     `json:"pending_sick_died_other,omitempty"`
	PendingJailedDeaths   int     `json:"pending_jailed_deaths,omitempty"`
	WoundedSick           int     `json:"wounded_sick,omitempty"`
	ChildAgeRemainder     int     `json:"child_age_remainder,omitempty"`
	AdultAgeRemainder     int     `json:"adult_age_remainder,omitempty"`
	ThirstReliefRemainder int64   `json:"thirst_relief_remainder,omitempty"`
	HungerReliefRemainder int64   `json:"hunger_relief_remainder,omitempty"`
	DigestedFood          int64   `json:"digested_food,omitempty"`
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	District              string  `json:"district"`
	FactionID             string  `json:"faction_id,omitempty"`
	Profession            string  `json:"profession"`
	Tier                  int     `json:"tier"`
	ProfLevel             int     `json:"prof_level"`
	Rank                  string  `json:"rank,omitempty"`
	Population            int     `json:"population"`
	Children              int     `json:"children,omitempty"`
	Adults                int     `json:"adults,omitempty"`
	Elders                int     `json:"elders,omitempty"`
	Wounded               int     `json:"wounded,omitempty"`
	Sick                  int     `json:"sick,omitempty"`
	Jailed                int     `json:"jailed,omitempty"`
	CumulativeDeaths      int     `json:"cumulative_deaths,omitempty"`
	CumulativeBirths      int     `json:"cumulative_births,omitempty"`
	RestUntilTick         int64   `json:"rest_until_tick,omitempty"`
	// Average physiology belongs to anonymous residents; Members use their own values.
	AvgEnergy          int              `json:"avg_energy"`
	AvgHunger          int              `json:"avg_hunger"`
	AvgThirst          int              `json:"avg_thirst,omitempty"`
	AvgXP              int              `json:"avg_xp,omitempty"`
	MoneyPool          int64            `json:"money_pool"`
	Currencies         map[string]int64 `json:"currencies,omitempty"`
	IllicitWealth      int64            `json:"illicit_wealth,omitempty"`
	ProtectedUntilTick int64            `json:"protected_until_tick,omitempty"`
	ItemPool           map[string]int64 `json:"item_pool,omitempty"`
	HydratedSeq        int              `json:"hydrated_seq,omitempty"`
}

func CloneCohort(c PopulationCohort) PopulationCohort {
	if c.Residents != nil {
		res := make([]ResidentBand, len(c.Residents))
		for i, b := range c.Residents {
			res[i] = cloneResidentBand(b)
		}
		c.Residents = res
	}
	c.Currencies = cloneInt64Map(c.Currencies)
	c.ItemPool = cloneInt64Map(c.ItemPool)
	if c.Members != nil {
		c.Members = append([]Actor(nil), c.Members...)
		for i := range c.Members {
			c.Members[i] = CloneActor(c.Members[i])
		}
	}
	return c
}

// EffectivePopulation returns healthy, free working-age cohort members able to work, patrol, or fight.
func (c PopulationCohort) EffectivePopulation() int {
	workforce := c.Population
	if c.Children+c.Adults+c.Elders > 0 {
		workforce = c.Adults + c.Elders/2
	}
	eff := workforce - c.Wounded - c.Sick + c.WoundedSick - c.Jailed
	if eff < 0 {
		return 0
	}
	return eff
}

// TotalPopulation returns the combined living population of active individual Actors and macro PopulationCohorts.
func (w *World) TotalPopulation() int {
	total := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead {
			total++
		}
	}
	for i := range w.Cohorts {
		total += w.Cohorts[i].Population
	}
	return total
}

// TotalDeaths returns cumulative deaths across individual Actors and macro PopulationCohorts.
func (w *World) TotalDeaths() int {
	deaths := 0
	for i := range w.Actors {
		if w.Actors[i].Dead {
			deaths++
		}
	}
	for i := range w.Cohorts {
		deaths += w.Cohorts[i].CumulativeDeaths
	}
	return deaths
}

// TotalWounded returns currently injured people across living individual Actors and macro PopulationCohorts.
func (w *World) TotalWounded() int {
	wounded := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].Health < MaxHealth {
			wounded++
		}
	}
	for i := range w.Cohorts {
		wounded += w.Cohorts[i].Wounded
	}
	return wounded
}

// TotalJailed returns currently imprisoned people across living individual Actors and macro PopulationCohorts.
func (w *World) TotalJailed() int {
	jailed := 0
	for i := range w.Actors {
		if !w.Actors[i].Dead && w.Actors[i].JailedUntil > w.Tick {
			jailed++
		}
	}
	for i := range w.Cohorts {
		jailed += w.Cohorts[i].Jailed
	}
	return jailed
}

type placeKey struct {
	location    string
	venueID     string
	insideHouse string
}

type stepIndex struct {
	clock             Clock
	calendar          CalendarState
	mapSnapshot       WorldMap
	catalogSnapshot   Catalog
	politiesSnapshot  *PolityNetwork
	knowledgeSnapshot *KnowledgeBook
	worldLawsSnapshot map[string]int
	districtByID      map[string]District
	venueByID         map[string]Venue
	districtStates    map[string]DistrictState
	roadsByDistrict   map[string][]RoadView
	houseByID         map[string]House
	accessibleByActor map[string][]House
	factionsBase      []FactionView
	factionMembers    map[string]map[string]bool
	placeBuckets      map[placeKey][]int
	neighborSnap      []Neighbor
	offersByActor     map[string][]Offer
	activeEffectsSnap []ActiveEffect
	cohortsByDistrict map[string][]CohortView
	agreementsByActor map[string][]Agreement
}

func (w *World) buildStepIndex() *stepIndex {
	idx := &stepIndex{
		clock:             ClockAt(w.Tick),
		calendar:          CalendarAt(w.Tick, w.Catalog.Calendar),
		mapSnapshot:       CloneMap(w.Map),
		catalogSnapshot:   CloneCatalog(w.Catalog),
		politiesSnapshot:  ClonePolityNetwork(w.Polities),
		knowledgeSnapshot: CloneKnowledgeBook(w.Knowledge),
		worldLawsSnapshot: w.EffectiveWorldLaws(),
		districtByID:      make(map[string]District, len(w.Map.Districts)),
		venueByID:         make(map[string]Venue, len(w.Map.Venues)),
		roadsByDistrict:   make(map[string][]RoadView, len(w.Map.Districts)),
		houseByID:         make(map[string]House, len(w.Houses)),
		accessibleByActor: make(map[string][]House, len(w.Actors)),
		factionsBase:      make([]FactionView, len(w.Factions)),
		factionMembers:    make(map[string]map[string]bool, len(w.Factions)),
		placeBuckets:      make(map[placeKey][]int, len(w.Map.Districts)*2),
		neighborSnap:      make([]Neighbor, len(w.Actors)),
		offersByActor:     make(map[string][]Offer),
	}
	if w.Continuity != nil && len(w.Continuity.Agreements) > 0 {
		counts := make(map[string]int, len(w.Actors))
		for _, ag := range w.Continuity.Agreements {
			counts[ag.SupplierID]++
			if ag.BuyerID != ag.SupplierID {
				counts[ag.BuyerID]++
			}
		}
		idx.agreementsByActor = make(map[string][]Agreement, len(counts))
		for id, n := range counts {
			idx.agreementsByActor[id] = make([]Agreement, 0, n)
		}
		for _, ag := range w.Continuity.Agreements {
			idx.agreementsByActor[ag.SupplierID] = append(idx.agreementsByActor[ag.SupplierID], ag)
			if ag.BuyerID != ag.SupplierID {
				idx.agreementsByActor[ag.BuyerID] = append(idx.agreementsByActor[ag.BuyerID], ag)
			}
		}
	}
	if len(w.ActiveEffects) > 0 {
		idx.activeEffectsSnap = make([]ActiveEffect, len(w.ActiveEffects))
		for i, eff := range w.ActiveEffects {
			idx.activeEffectsSnap[i] = CloneActiveEffect(eff)
		}
	}
	if len(w.Cohorts) > 0 {
		idx.cohortsByDistrict = make(map[string][]CohortView, len(w.Map.Districts))
		for _, c := range w.Cohorts {
			if c.Population > 0 {
				idx.cohortsByDistrict[c.District] = append(idx.cohortsByDistrict[c.District], CohortView{
					ID:         c.ID,
					Name:       c.Name,
					District:   c.District,
					Profession: c.Profession,
					Population: c.Population,
				})
			}
		}
	}
	if w.DistrictStates != nil {
		idx.districtStates = make(map[string]DistrictState, len(w.DistrictStates))
		for k, ds := range w.DistrictStates {
			idx.districtStates[k] = CloneDistrictState(ds)
		}
	}

	for _, d := range w.Map.Districts {
		dc := d
		if d.Features != nil {
			dc.Features = append([]string(nil), d.Features...)
		}
		idx.districtByID[d.ID] = dc
		neighbors := w.Map.NeighborsOf(d.ID)
		if len(neighbors) > 0 {
			views := make([]RoadView, len(neighbors))
			for i, r := range neighbors {
				effTicks, effEnergy := w.effectiveRoadCost(r)
				views[i] = RoadView{
					To:          r.To,
					Ticks:       effTicks,
					EnergyCost:  effEnergy,
					LeadsToward: w.Map.LeadsToward(d.ID, r.To),
				}
			}
			idx.roadsByDistrict[d.ID] = views
		}
	}
	for _, v := range w.Map.Venues {
		vc := v
		if v.Features != nil {
			vc.Features = append([]string(nil), v.Features...)
		}
		idx.venueByID[v.ID] = vc
	}

	for _, h := range w.Houses {
		hc := CloneHouse(h)
		idx.houseByID[h.ID] = hc
		seen := make(map[string]bool, len(h.Residents)+1)
		if h.OwnerID != "" {
			seen[h.OwnerID] = true
			idx.accessibleByActor[h.OwnerID] = append(idx.accessibleByActor[h.OwnerID], hc)
		}
		for _, r := range h.Residents {
			if !seen[r] {
				seen[r] = true
				idx.accessibleByActor[r] = append(idx.accessibleByActor[r], hc)
			}
		}
	}

	for i, f := range w.Factions {
		idx.factionsBase[i] = FactionView{
			ID:           f.ID,
			Name:         f.Name,
			District:     f.District,
			Level:        f.Level,
			Professions:  append([]string(nil), f.Professions...),
			Requirements: cloneIntMap(f.Requirements),
			Members:      len(f.Members),
		}
		mset := make(map[string]bool, len(f.Members))
		for _, m := range f.Members {
			mset[m] = true
		}
		idx.factionMembers[f.ID] = mset
	}

	hasFormOrEffect := len(w.Catalog.Forms) > 0 || len(w.ActiveEffects) > 0
	for i := range w.Actors {
		a := &w.Actors[i]
		if a.Dead {
			continue
		}
		var nbTags []string
		if hasFormOrEffect {
			nbTags = w.neighborEffectiveTags(*a)
		} else {
			nbTags = a.IdentityTags()
		}
		idx.neighborSnap[i] = Neighbor{
			ID:       a.ID,
			Name:     a.Name,
			Role:     a.Role,
			Tags:     nbTags,
			Level:    a.Level,
			Health:   a.Health,
			Sleeping: a.Task != nil && a.Task.Action.Kind == "sleep",
			Jailed:   a.JailedUntil > w.Tick,
		}
		if !a.InTransit() {
			venueBucket := ""
			if a.InsideHouse == "" {
				venueBucket = a.VenueID
			}
			pk := placeKey{location: a.Location, venueID: venueBucket, insideHouse: a.InsideHouse}
			idx.placeBuckets[pk] = append(idx.placeBuckets[pk], i)
		}
	}

	for _, o := range w.Offers {
		if o.Expires >= w.Tick {
			idx.offersByActor[o.Buyer] = append(idx.offersByActor[o.Buyer], o)
			if o.Seller != o.Buyer {
				idx.offersByActor[o.Seller] = append(idx.offersByActor[o.Seller], o)
			}
		}
	}

	return idx
}

func (w *World) viewIndexed(actorIdx int, idx *stepIndex) Perception {
	a := w.Actors[actorIdx]
	id := a.ID
	a.Tags = append([]string(nil), a.Tags...)
	a.Guilds = append([]string(nil), a.Guilds...)
	a.Professions = cloneIntMap(a.Professions)
	a.ProfessionRanks = cloneStringMap(a.ProfessionRanks)
	a.ProfessionXP = cloneIntMap(a.ProfessionXP)
	a.GuildLevels = cloneIntMap(a.GuildLevels)
	a.Items = cloneIntMap(a.Items)
	a.Trust = cloneIntMap(a.Trust)
	a.StateSlots = cloneIntMap(a.StateSlots)
	a.Currencies = cloneInt64Map(a.Currencies)
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	if a.Task != nil {
		tc := *a.Task
		if tc.Action.Statement != nil {
			sc := *tc.Action.Statement
			tc.Action.Statement = &sc
		}
		a.Task = &tc
	}

	dist := idx.districtByID[a.Location]
	if dist.Features != nil {
		dist.Features = append([]string(nil), dist.Features...)
	}
	p := Perception{
		EnergyNodes:     w.localEnergyNodes(a),
		Growth:          w.visibleGrowth(a),
		Tick:            w.Tick,
		Clock:           idx.clock,
		Calendar:        idx.calendar,
		Self:            a,
		CurrentDistrict: dist,
		Map:             CloneMap(idx.mapSnapshot),
		Catalog:         CloneCatalog(idx.catalogSnapshot),
	}
	p.ActiveEffects = w.perceivedActiveEffects(a, idx.activeEffectsSnap)
	if cohorts := idx.cohortsByDistrict[a.Location]; len(cohorts) > 0 {
		p.LocalCohorts = append([]CohortView(nil), cohorts...)
	}
	if idx.districtStates != nil {
		if ds, ok := idx.districtStates[a.Location]; ok {
			dsc := CloneDistrictState(ds)
			p.DistrictState = &dsc
		}
	}

	if a.InsideHouse != "" {
		if h, ok := idx.houseByID[a.InsideHouse]; ok {
			hc := CloneHouse(h)
			p.CurrentHouse = &hc
		}
	}
	if a.VenueID != "" {
		if v, ok := idx.venueByID[a.VenueID]; ok {
			vc := v
			if v.Features != nil {
				vc.Features = append([]string(nil), v.Features...)
			}
			p.CurrentVenue = &vc
		}
	}
	p.Polity = buildPolityPerceptionView(p.Map, idx.politiesSnapshot, a)
	p.KnownTechs = actorKnownTechsFromBook(idx.knowledgeSnapshot, id)
	p.WorldLaws = cloneIntMap(idx.worldLawsSnapshot)
	if houses := idx.accessibleByActor[id]; len(houses) > 0 {
		p.AccessibleHouses = make([]House, len(houses))
		for i, h := range houses {
			p.AccessibleHouses[i] = CloneHouse(h)
		}
	}
	if len(idx.factionsBase) > 0 {
		p.Factions = make([]FactionView, len(idx.factionsBase))
		for i, fb := range idx.factionsBase {
			fv := fb
			fv.Professions = append([]string(nil), fb.Professions...)
			fv.Requirements = cloneIntMap(fb.Requirements)
			fv.Joined = idx.factionMembers[fb.ID][id]
			p.Factions[i] = fv
		}
	}
	if a.InsideHouse == "" {
		if roads := idx.roadsByDistrict[a.Location]; len(roads) > 0 {
			p.Roads = make([]RoadView, len(roads))
			for i, r := range roads {
				rv := r
				if r.LeadsToward != nil {
					rv.LeadsToward = append([]string(nil), r.LeadsToward...)
				}
				p.Roads[i] = rv
			}
		}
		if len(p.Map.Venues) > 0 {
			p.Roads = nil
			p.VenueRoutes = buildVenueRouteViews(p.Map, p.Catalog, idx.districtStates, w.actorWithEffectTags(a))
		}
	}

	if !a.InTransit() {
		venueBucket := ""
		if a.InsideHouse == "" {
			venueBucket = a.VenueID
		}
		bucket := idx.placeBuckets[placeKey{location: a.Location, venueID: venueBucket, insideHouse: a.InsideHouse}]
		if len(bucket) > 1 {
			if len(bucket)-1 <= MaxNearbyView {
				p.Nearby = make([]Neighbor, 0, len(bucket)-1)
				for _, bi := range bucket {
					if bi == actorIdx {
						continue
					}
					nb := idx.neighborSnap[bi]
					nb.Tags = append([]string(nil), nb.Tags...)
					p.Nearby = append(p.Nearby, nb)
				}
			} else {
				// Attention-capped O(1) neighbor selection for high-density districts:
				// 1. Counterparties with active offers
				// 2. Trusted/known acquaintances
				// 3. Deterministic tick-rotated sample from the co-located bucket
				selected := make([]Neighbor, 0, MaxNearbyView)
				picked := make(map[string]bool, MaxNearbyView)
				offerPeers := make(map[string]bool)
				for _, o := range idx.offersByActor[id] {
					offerPeers[o.Seller] = true
					offerPeers[o.Buyer] = true
				}
				for _, bi := range bucket {
					if bi == actorIdx {
						continue
					}
					nb := idx.neighborSnap[bi]
					if offerPeers[nb.ID] || a.Trust[nb.ID] != 0 {
						nb.Tags = append([]string(nil), nb.Tags...)
						selected = append(selected, nb)
						picked[nb.ID] = true
						if len(selected) >= MaxNearbyView {
							break
						}
					}
				}
				if len(selected) < MaxNearbyView {
					offset := int(neighborOrderKey(id, w.Tick, "bucket") % uint64(len(bucket)))
					for step := 0; step < len(bucket) && len(selected) < MaxNearbyView; step++ {
						bi := bucket[(offset+step)%len(bucket)]
						if bi == actorIdx {
							continue
						}
						nb := idx.neighborSnap[bi]
						if picked[nb.ID] {
							continue
						}
						nb.Tags = append([]string(nil), nb.Tags...)
						selected = append(selected, nb)
						picked[nb.ID] = true
					}
				}
				p.Nearby = selected
			}
			sort.Slice(p.Nearby, func(i, j int) bool {
				ki := neighborOrderKey(id, w.Tick, p.Nearby[i].ID)
				kj := neighborOrderKey(id, w.Tick, p.Nearby[j].ID)
				if ki != kj {
					return ki < kj
				}
				return p.Nearby[i].ID < p.Nearby[j].ID
			})
		}
	}

	if offers := idx.offersByActor[id]; len(offers) > 0 {
		p.Offers = append([]Offer(nil), offers...)
	}
	w.applyPerceptionEvoHintsWithAgreements(&p, idx.agreementsByActor[id])
	recallPerceptionMemories(&p)
	return p
}

func stepWorkerCount(idleCount int) int {
	if idleCount <= 0 {
		return 0
	}
	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}
	if workers > 32 {
		workers = 32
	}
	if idleCount < workers {
		return idleCount
	}
	return workers
}

// LargeWorld constructs a multi-resolution world targeting totalPopulation (e.g. 30,000 to 3,000,000+).
// If targetPopulation <= 30, it returns the standard 30-villager Village(id).
// For larger populations, it seeds an active individual Actor core alongside macro PopulationCohorts
// distributed across all 6 factions, 7 districts, and 18 Tier-1/Tier-2 professions so the exact
// totalPopulation is conserved while each tick settles in milliseconds.
func LargeWorld(id string, targetPopulation int) *World {
	w := Village(id)
	if targetPopulation <= len(w.Actors) {
		return w
	}

	// Upgrade topology and guild roster to the 11-district mountain-river-canal prefecture!
	w.Map = PrefectureMap()
	w.Factions = PrefectureFactions()
	for i := range w.Actors {
		a := &w.Actors[i]
		a.Guilds = nil
		if prof, ok := w.Catalog.ProfessionByID(a.Role); ok {
			if f := w.faction(prof.Guild); f != nil {
				f.Members = appendUnique(f.Members, a.ID)
				a.Guilds = appendUnique(a.Guilds, f.ID)
			}
		}
	}

	// Expand active individual roster up to 120 focal/active agents (or 10% of targetPopulation for smaller scales)
	targetActive := 120
	if targetPopulation/10 < targetActive {
		targetActive = targetPopulation / 10
	}
	if targetActive < len(w.Actors) {
		targetActive = len(w.Actors)
	}

	extraHouses := []House{
		{ID: "h17", Name: "South Granary Courtyard", District: "south_fields", OwnerID: "p31", Residents: []string{"p31", "p32", "p33", "p34"}, Level: 2, Comfort: 2, Condition: 88, Facilities: []string{"compost_pit"}},
		{ID: "h18", Name: "East Master Smithy", District: "east_lane", OwnerID: "p35", Residents: []string{"p35", "p36", "p37", "p38"}, Level: 3, Comfort: 3, Condition: 92, Facilities: []string{"forge", "carpentry_bench"}},
		{ID: "h19", Name: "Crossroads Imperial Academy Hall", District: "crossroads", OwnerID: "p39", Residents: []string{"p39", "p40", "p41", "p42"}, Level: 3, Comfort: 3, Condition: 95},
		{ID: "h20", Name: "Market Guild Exchange", District: "market_street", OwnerID: "p43", Residents: []string{"p43", "p44", "p45", "p46"}, Level: 3, Comfort: 3, Condition: 94, Facilities: []string{"herb_rack"}},
		{ID: "h21", Name: "East Zhenwu Martial Hall", District: "east_lane", OwnerID: "p0041", Residents: []string{"p0041", "p0056"}, Level: 3, Comfort: 3, Condition: 94, Facilities: []string{"martial_yard", "forge"}},
		{ID: "h22", Name: "Crossroads Prefecture Yamen", District: "crossroads", OwnerID: "p0042", Residents: []string{"p0042", "p0057"}, Level: 3, Comfort: 3, Condition: 96, Facilities: []string{"yamen_court"}},
		{ID: "h23", Name: "North Woods Blackwind Stronghold", District: "north_woods", OwnerID: "p0043", Residents: []string{"p0043", "p0058"}, Level: 2, Comfort: 2, Condition: 85, Facilities: []string{"bandit_stronghold"}},
		{ID: "h24", Name: "Market Azure Dragon Gang Hall", District: "market_street", OwnerID: "p0045", Residents: []string{"p0045", "p0044"}, Level: 2, Comfort: 2, Condition: 88, Facilities: []string{"gang_hall"}},
		{ID: "h25", Name: "Jade River Wharf Boathouse", District: "jade_river_wharf", OwnerID: "p0046", Residents: []string{"p0046", "p0047"}, Level: 2, Comfort: 3, Condition: 90, Facilities: []string{"carpentry_bench"}},
		{ID: "h26", Name: "North Steppe Ranch Yurt", District: "north_pasture", OwnerID: "p0048", Residents: []string{"p0048", "p0049"}, Level: 2, Comfort: 2, Condition: 86, Facilities: []string{"compost_pit"}},
		{ID: "h27", Name: "Blackrock Mine Barracks", District: "deep_iron_mine", OwnerID: "p0050", Residents: []string{"p0050", "p0051"}, Level: 2, Comfort: 2, Condition: 84, Facilities: []string{"forge"}},
		{ID: "h28", Name: "Grand Altar Precinct", District: "grand_shrine", OwnerID: "p0052", Residents: []string{"p0052", "p0053"}, Level: 3, Comfort: 3, Condition: 95, Facilities: []string{"herb_rack"}},
	}
	if targetActive > len(w.Actors) {
		w.Houses = append(w.Houses, extraHouses...)
	}

	surnames := []string{"林", "陈", "王", "李", "赵", "周", "吴", "郑", "孙", "钱", "冯", "褚", "卫", "蒋", "沈", "韩", "杨", "朱", "秦", "尤", "许", "何", "吕", "施", "张", "孔", "曹", "严", "华", "金"}
	given := []string{"文", "武", "明", "远", "清", "平", "安", "宁", "宏", "博", "雅", "正", "志", "成", "康", "泰", "瑞", "祥", "恒", "达"}
	roles := []string{
		"farmer", "farmer", "herder", "fisher", "carpenter",
		"blacksmith", "herbalist", "peddler", "boatman", "scholar",
		"martial_artist", "constable", "bandit", "thief", "gangster",
		"farmer", "hunter", "woodcutter", "miner", "ritualist",
		"carpenter", "blacksmith", "herbalist", "healer", "peddler",
		"scholar", "guard", "fisher", "herder", "hunter",
	}
	sleepTraits := []string{SleepDeep, SleepEarlyBird, SleepLight, SleepNightOwl}
	personalities := []string{"careful and candid", "sociable and generous", "ambitious and skeptical"}

	for idx := len(w.Actors); idx < targetActive; idx++ {
		actorID := fmt.Sprintf("p%04d", idx+1)
		role := roles[idx%len(roles)]
		home := w.Houses[idx%len(w.Houses)]
		w.Houses[idx%len(w.Houses)].Residents = appendUnique(w.Houses[idx%len(w.Houses)].Residents, actorID)
		name := surnames[idx%len(surnames)] + given[(idx/len(surnames))%len(given)] + fmt.Sprintf("%d", idx+1)
		tags := []string{"resident", "trader"}
		if prof, ok := w.Catalog.ProfessionByID(role); ok {
			for _, g := range prof.Grants {
				tags = appendUnique(tags, g)
			}
		}
		if w.Catalog.TagsConsume([]string{role}, "water") {
			tags = appendUnique(tags, "water_user")
		}
		attrs := seedAttributes(role, idx)
		profs := map[string]int{role: 1 + idx%3}
		// This established settlement starts with last season's crop-period
		// reserve, like Village's standing-crop initialization.
		items := map[string]int{"food": w.initialCropPeriodRations(1), "water": 2}
		switch role {
		case "farmer":
			items["seeds"] = 3
			items["compost"] = 2
		case "herder":
			items["livestock"] = 2
			items["winter_coat"] = 1
		case "fisher":
			items["tools"] = 1
		case "boatman":
			items["plank"] = 1
			items["tools"] = 1
		case "hunter":
			items["weapon"] = 1
			items["tools"] = 1
			items["winter_coat"] = 1
		case "ritualist":
			items["sacrificial_offering"] = 2
			items["herb"] = 2
		case "carpenter":
			items["timber"] = 2
			items["plank"] = 2
		case "blacksmith":
			items["ore"] = 2
			items["timber"] = 2
			items["tools"] = 1
		case "miner":
			items["tools"] = 1
			items["ore"] = 2
			items["winter_coat"] = 1
		case "herbalist", "healer":
			items["herb"] = 2
			items["medicine"] = 2
		case "scholar":
			items["scroll"] = 2
		case "martial_artist":
			items["weapon"] = 1
			items["martial_manual"] = 1
			items["ore"] = 1
			items["timber"] = 1
		case "constable", "guard":
			items["weapon"] = 1
			items["bounty_writ"] = 1
		case "bandit", "gangster":
			items["weapon"] = 1
		}
		loc := home.District
		if prof, ok := w.Catalog.ProfessionByID(role); ok {
			if f := w.faction(prof.Guild); f != nil && f.District != "" {
				loc = f.District
			}
		}
		a := Actor{
			ID:          actorID,
			Name:        name,
			Role:        role,
			Tags:        tags,
			Attributes:  attrs,
			Professions: profs,
			Location:    loc,
			InsideHouse: "",
			HomeID:      home.ID,
			AgeYears:    18 + (idx*7)%48,
			Health:      MaxHealth,
			Energy:      85,
			Hunger:      0,
			SleepTrait:  sleepTraits[idx%len(sleepTraits)],
			Money:       20,
			Items:       items,
			Goal:        "Practice " + role + " craft, advance in guild rank, and trade across the prefecture",
			Personality: personalities[idx%len(personalities)],
			Trust:       map[string]int{},
		}
		if prof, ok := w.Catalog.ProfessionByID(role); ok {
			if f := w.faction(prof.Guild); f != nil {
				f.Members = appendUnique(f.Members, a.ID)
				a.Guilds = appendUnique(a.Guilds, f.ID)
			}
		}
		w.Actors = append(w.Actors, a)
	}

	remaining := targetPopulation - len(w.Actors)
	type cohortSpec struct {
		id        string
		name      string
		district  string
		faction   string
		prof      string
		tier      int
		profLevel int
		weight    int // parts per 10,000 (~98.6% civilian primary/craft/logistics/ritual, ~1.4% watch/yamen/martial/outlaws)
	}
	specs := []cohortSpec{
		{id: "cohort_south_farmers", name: "南畴佃农群", district: "south_fields", faction: "farmers_guild", prof: "farmer", tier: 1, profLevel: 1, weight: 2039},
		{id: "cohort_west_farmers", name: "西庄熟农群", district: "west_cottages", faction: "farmers_guild", prof: "farmer", tier: 1, profLevel: 2, weight: 1150},
		{id: "cohort_south_agronomists", name: "司农农师营", district: "south_fields", faction: "farmers_guild", prof: "agronomist", tier: 2, profLevel: 2, weight: 180},
		{id: "cohort_pasture_herders", name: "北麓牧户群", district: "north_pasture", faction: "farmers_guild", prof: "herder", tier: 1, profLevel: 2, weight: 480},
		{id: "cohort_pasture_masters", name: "北苑牧正堂", district: "north_pasture", faction: "farmers_guild", prof: "ranch_master", tier: 2, profLevel: 2, weight: 90},
		{id: "cohort_wharf_fishers", name: "玉河渔户帮", district: "jade_river_wharf", faction: "waterways_guild", prof: "fisher", tier: 1, profLevel: 2, weight: 480},
		{id: "cohort_wharf_fleet_masters", name: "渔港船把头堂", district: "jade_river_wharf", faction: "waterways_guild", prof: "fleet_master", tier: 2, profLevel: 2, weight: 90},
		{id: "cohort_wharf_boatmen", name: "通济漕工帮", district: "jade_river_wharf", faction: "waterways_guild", prof: "boatman", tier: 1, profLevel: 2, weight: 380},
		{id: "cohort_wharf_canal_masters", name: "漕运总舵师堂", district: "jade_river_wharf", faction: "waterways_guild", prof: "canal_master", tier: 2, profLevel: 2, weight: 80},
		{id: "cohort_woods_gatherers", name: "北林采食群", district: "north_woods", faction: "farmers_guild", prof: "gatherer", tier: 1, profLevel: 1, weight: 340},
		{id: "cohort_woods_hunters", name: "苍山猎户营", district: "north_woods", faction: "hunters_guild", prof: "hunter", tier: 1, profLevel: 2, weight: 280},
		{id: "cohort_woods_master_hunters", name: "镇山猎首堂", district: "north_woods", faction: "hunters_guild", prof: "master_hunter", tier: 2, profLevel: 2, weight: 60},
		{id: "cohort_woods_woodcutters", name: "苍山樵夫帮", district: "north_woods", faction: "artisans_guild", prof: "woodcutter", tier: 1, profLevel: 2, weight: 680},
		{id: "cohort_east_carpenters", name: "东坊木作群", district: "east_lane", faction: "artisans_guild", prof: "carpenter", tier: 1, profLevel: 2, weight: 480},
		{id: "cohort_east_master_carpenters", name: "营造大木匠群", district: "east_lane", faction: "artisans_guild", prof: "master_carpenter", tier: 2, profLevel: 2, weight: 120},
		{id: "cohort_mine_miners", name: "黑石深井矿工帮", district: "deep_iron_mine", faction: "artisans_guild", prof: "miner", tier: 1, profLevel: 2, weight: 560},
		{id: "cohort_east_blacksmiths", name: "东坊铁匠铺群", district: "east_lane", faction: "artisans_guild", prof: "blacksmith", tier: 1, profLevel: 2, weight: 440},
		{id: "cohort_east_master_smiths", name: "精锻匠师堂", district: "east_lane", faction: "artisans_guild", prof: "master_smith", tier: 2, profLevel: 2, weight: 120},
		{id: "cohort_market_herbalists", name: "药市街采药群", district: "market_street", faction: "healers_guild", prof: "herbalist", tier: 1, profLevel: 2, weight: 420},
		{id: "cohort_market_healers", name: "杏林医者群", district: "market_street", faction: "healers_guild", prof: "healer", tier: 1, profLevel: 2, weight: 240},
		{id: "cohort_market_alchemists", name: "丹药房丹师群", district: "market_street", faction: "healers_guild", prof: "alchemist", tier: 2, profLevel: 2, weight: 100},
		{id: "cohort_market_peddlers", name: "商帮陆运行商群", district: "market_street", faction: "merchants_guild", prof: "peddler", tier: 1, profLevel: 2, weight: 460},
		{id: "cohort_market_chiefs", name: "商帮大掌柜堂", district: "market_street", faction: "merchants_guild", prof: "merchant_chief", tier: 2, profLevel: 2, weight: 110},
		{id: "cohort_crossroads_scholars", name: "书院读书人群", district: "crossroads", faction: "scholars_guild", prof: "scholar", tier: 1, profLevel: 2, weight: 260},
		{id: "cohort_crossroads_xiucai", name: "书院秀才林", district: "crossroads", faction: "scholars_guild", prof: "xiucai", tier: 2, profLevel: 2, weight: 90},
		{id: "cohort_shrine_ritualists", name: "社稷奉祀祝官群", district: "grand_shrine", faction: "shrine_guild", prof: "ritualist", tier: 1, profLevel: 2, weight: 105},
		{id: "cohort_shrine_high_priests", name: "奉常大祭酒堂", district: "grand_shrine", faction: "shrine_guild", prof: "high_priest", tier: 2, profLevel: 2, weight: 28},
		{id: "cohort_crossroads_guards", name: "巡守武侯营", district: "crossroads", faction: "watch_guild", prof: "guard", tier: 1, profLevel: 2, weight: 28},
		{id: "cohort_crossroads_captains", name: "巡守都头府", district: "crossroads", faction: "watch_guild", prof: "captain", tier: 2, profLevel: 2, weight: 7},
		{id: "cohort_east_martial_artists", name: "振武馆武师群", district: "east_lane", faction: "martial_guild", prof: "martial_artist", tier: 1, profLevel: 2, weight: 25},
		{id: "cohort_east_martial_masters", name: "振武馆宗师堂", district: "east_lane", faction: "martial_guild", prof: "martial_master", tier: 2, profLevel: 2, weight: 7},
		{id: "cohort_crossroads_constables", name: "府衙捕快营", district: "crossroads", faction: "yamen_guild", prof: "constable", tier: 1, profLevel: 2, weight: 14},
		{id: "cohort_crossroads_chief_constables", name: "六扇门总捕府", district: "crossroads", faction: "yamen_guild", prof: "chief_constable", tier: 2, profLevel: 2, weight: 4},
		{id: "cohort_woods_bandits", name: "黑风寨山贼群", district: "north_woods", faction: "bandits_guild", prof: "bandit", tier: 1, profLevel: 2, weight: 16},
		{id: "cohort_woods_bandit_leaders", name: "黑风寨大王堂", district: "north_woods", faction: "bandits_guild", prof: "bandit_leader", tier: 2, profLevel: 2, weight: 4},
		{id: "cohort_market_thieves", name: "闹市窃贼帮", district: "market_street", faction: "gang_guild", prof: "thief", tier: 1, profLevel: 2, weight: 12},
		{id: "cohort_market_master_thieves", name: "千手盗帅盟", district: "market_street", faction: "gang_guild", prof: "master_thief", tier: 2, profLevel: 2, weight: 2},
		{id: "cohort_market_gangsters", name: "青龙帮刀客群", district: "market_street", faction: "gang_guild", prof: "gangster", tier: 1, profLevel: 2, weight: 15},
		{id: "cohort_market_gang_bosses", name: "青龙帮堂主堂", district: "market_street", faction: "gang_guild", prof: "gang_boss", tier: 2, profLevel: 2, weight: 4},
	}

	pops := make([]int, len(specs))
	allocated := 0
	for i, sp := range specs {
		p := (remaining * sp.weight) / 10000
		if p < 2 && remaining >= len(specs)*4 {
			p = 2
		}
		pops[i] = p
		allocated += p
	}
	pops[0] += remaining - allocated

	w.Cohorts = make([]PopulationCohort, len(specs))
	for i, sp := range specs {
		pop := pops[i]
		prof, _ := w.Catalog.ProfessionByID(sp.prof)
		xp := 0
		if sp.profLevel-1 < len(w.Catalog.LevelThresholds) {
			xp = w.Catalog.LevelThresholds[sp.profLevel-1]
		}
		itemPool := map[string]int64{
			"food":  int64(w.initialCropPeriodRations(pop)),
			"water": int64(pop) * 2,
		}
		if sp.district == "north_pasture" || sp.district == "north_woods" || sp.district == "deep_iron_mine" {
			itemPool["winter_coat"] = int64(maxInt(1, pop/3))
		}
		switch sp.prof {
		case "farmer", "agronomist":
			itemPool["seeds"] = int64(pop) * 2
			itemPool["compost"] = int64(pop) * 2
		case "herder", "ranch_master":
			itemPool["livestock"] = int64(pop) * 2
			itemPool["hide"] = int64(pop)
		case "fisher", "fleet_master":
			itemPool["tools"] = int64(pop)
			itemPool["fish"] = int64(pop) * 2
		case "boatman", "canal_master":
			itemPool["plank"] = int64(pop)
			itemPool["tools"] = int64(pop)
		case "hunter", "master_hunter":
			itemPool["weapon"] = int64(pop)
			itemPool["tools"] = int64(pop)
			itemPool["hide"] = int64(pop)
		case "ritualist", "high_priest":
			itemPool["sacrificial_offering"] = int64(pop) * 2
			itemPool["herb"] = int64(pop) * 2
		case "woodcutter":
			itemPool["timber"] = int64(pop) * 2
		case "carpenter", "master_carpenter":
			itemPool["timber"] = int64(pop) * 2
			itemPool["plank"] = int64(pop) * 2
		case "miner":
			itemPool["ore"] = int64(pop) * 2
			itemPool["tools"] = int64(pop)
		case "blacksmith", "master_smith":
			itemPool["ore"] = int64(pop) * 2
			itemPool["timber"] = int64(pop) * 2
			itemPool["tools"] = int64(pop)
		case "herbalist", "alchemist", "healer":
			itemPool["herb"] = int64(pop) * 2
			itemPool["medicine"] = int64(pop)
		case "scholar", "xiucai":
			itemPool["scroll"] = int64(pop)
		case "martial_artist", "martial_master":
			itemPool["weapon"] = int64(pop)
			itemPool["martial_manual"] = int64(pop)
			itemPool["medicine"] = int64(pop / 2)
			itemPool["ore"] = int64(pop)
			itemPool["timber"] = int64(pop)
		case "constable", "chief_constable", "guard", "captain":
			itemPool["weapon"] = int64(pop)
			itemPool["bounty_writ"] = int64(pop)
			itemPool["medicine"] = int64(pop / 2)
		case "bandit", "bandit_leader", "gangster", "gang_boss":
			itemPool["weapon"] = int64(pop)
			itemPool["medicine"] = int64(pop / 2)
		}
		w.Cohorts[i] = PopulationCohort{
			ID:         sp.id,
			Name:       sp.name,
			District:   sp.district,
			FactionID:  sp.faction,
			Profession: sp.prof,
			Tier:       sp.tier,
			ProfLevel:  sp.profLevel,
			Rank:       prof.RankTitle(sp.profLevel),
			Population: pop,
			AvgEnergy:  84,
			AvgHunger:  0,
			AvgXP:      xp,
			MoneyPool:  int64(pop) * 16,
			ItemPool:   itemPool,
		}
		initCohortAgePyramid(w.Catalog, &w.Cohorts[i])
	}

	// Recompute initial faction influence and initialize dynamic district ecology!
	for i := range w.Factions {
		f := &w.Factions[i]
		influence := 0
		for _, memberID := range f.Members {
			if actor := w.actor(memberID); actor != nil {
				for _, profID := range f.Professions {
					influence += 2 * actor.Professions[profID]
				}
			}
		}
		f.Influence = influence
		f.Level = w.Catalog.FactionLevel(influence)
	}
	for i := range w.Actors {
		w.refreshActor(&w.Actors[i])
	}
	w.PublicTreasury = int64(targetPopulation) * 6
	w.TreasuryLedger = TreasuryLedger{
		Initialized:     true,
		InitialTreasury: w.PublicTreasury,
	}
	if err := w.InitCohortResidents(); err != nil {
		panic(err)
	}
	w.InitEcology(targetPopulation)
	w.InitProductionAssets()
	w.InitLedgerAudit()
	return w
}

// HydrateActor materializes one individual Actor from the specified macro PopulationCohort,
// transferring its per-capita share of MoneyPool and ItemPool and preserving TotalPopulation().
func (w *World) HydrateActor(cohortID string) (*Actor, error) {
	var c *PopulationCohort
	for i := range w.Cohorts {
		if w.Cohorts[i].ID == cohortID {
			c = &w.Cohorts[i]
			break
		}
	}
	if c == nil {
		return nil, fmt.Errorf("unknown cohort: %s", cohortID)
	}
	reconcileCohortAgeBrackets(c)
	if len(c.Members) > 0 {
		for i := range c.Members {
			if w.Tick < c.Members[i].JailedUntil {
				continue
			}
			a := CloneActor(c.Members[i])
			w.placeResident(&a)
			w.adjustCohortMember(c, a, -1)
			c.Members = append(c.Members[:i], c.Members[i+1:]...)
			if len(c.Members) == 0 {
				c.Members = nil
			}
			if c.Population == 0 {
				w.transferLastCohortAssets(c, &a)
			}
			w.Actors = append(w.Actors, a)
			return &w.Actors[len(w.Actors)-1], nil
		}
	}
	anonymous := w.anonymousCohort(*c)
	if anonymous.Population-anonymous.Wounded-anonymous.Sick+anonymous.WoundedSick-anonymous.Jailed < 1 {
		return nil, fmt.Errorf("cohort %s has no healthy free population to hydrate", cohortID)
	}

	oldPop := int64(anonymous.Population)
	c.ensureResidents(anonymous.Population)
	resident := c.Residents[0]
	actorID := resident.ID(0)
	c.moveResidents(nil, 1)
	c.HydratedSeq++
	money := int(c.MoneyPool / oldPop)
	if money < 0 {
		money = 0
	}
	c.MoneyPool -= int64(money)

	items := map[string]int{}
	for item, totalQty := range c.ItemPool {
		share := int(totalQty / oldPop)
		if share > 0 {
			items[item] = share
			c.ItemPool[item] -= int64(share)
			if c.ItemPool[item] <= 0 {
				delete(c.ItemPool, item)
			}
		}
	}
	var actorCurrencies map[string]int64
	for cur, totalAmt := range c.Currencies {
		share := totalAmt / oldPop
		if share > 0 {
			if actorCurrencies == nil {
				actorCurrencies = map[string]int64{}
			}
			actorCurrencies[cur] = share
			c.Currencies[cur] -= share
			if c.Currencies[cur] <= 0 {
				delete(c.Currencies, cur)
			}
		}
	}
	if len(c.Currencies) == 0 {
		c.Currencies = nil
	}
	digestedShare := c.DigestedFood / oldPop
	c.DigestedFood -= digestedShare
	totalHunger := c.hungerUnits(anonymous.Population)
	hunger := resident.Hunger
	c.setHungerUnits(anonymous.Population-1, totalHunger-int64(hunger))
	totalEnergy := w.anonymousCohortEnergy(c, anonymous.Population)
	energy := int(totalEnergy / oldPop)
	w.setAnonymousCohortEnergy(c, anonymous.Population-1, totalEnergy-int64(energy))
	reconcileCohortAgeBrackets(c)
	age := w.Catalog.Lifecycle.ChildMaxAge + 1
	switch {
	case anonymous.Adults > 0:
		c.Adults--
		age += c.HydratedSeq % maxInt(1, w.Catalog.Lifecycle.ElderMinAge-age)
	case anonymous.Elders > 0:
		c.Elders--
		age = w.Catalog.Lifecycle.ElderMinAge + c.HydratedSeq%maxInt(1, w.Catalog.Lifecycle.MaxExpectancy-w.Catalog.Lifecycle.ElderMinAge)
	default:
		c.Children--
		age = c.HydratedSeq % maxInt(1, w.Catalog.Lifecycle.ChildMaxAge+1)
	}
	c.Population--

	prof, _ := w.Catalog.ProfessionByID(c.Profession)
	baseRole := c.Profession
	if prof.ParentProfession != "" {
		baseRole = prof.ParentProfession
	}
	attrs := seedAttributes(baseRole, c.HydratedSeq)
	for reqAttr, minVal := range prof.Requirements {
		switch reqAttr {
		case "strength":
			if attrs.Strength < minVal {
				attrs.Strength = minVal
			}
		case "dexterity":
			if attrs.Dexterity < minVal {
				attrs.Dexterity = minVal
			}
		case "insight":
			if attrs.Insight < minVal {
				attrs.Insight = minVal
			}
		case "spirit":
			if attrs.Spirit < minVal {
				attrs.Spirit = minVal
			}
		}
	}

	tags := []string{"resident", "trader"}
	for _, g := range prof.Grants {
		tags = appendUnique(tags, g)
	}
	if w.Catalog.TagsConsume([]string{baseRole}, "water") {
		tags = appendUnique(tags, "water_user")
	}

	profs := map[string]int{c.Profession: c.ProfLevel}
	xpMap := map[string]int{}
	if c.AvgXP > 0 {
		xpMap[c.Profession] = c.AvgXP
	}
	if prof.ParentProfession != "" {
		parentLv := prof.MinParentLevel
		if parentLv < 2 {
			parentLv = 2
		}
		profs[prof.ParentProfession] = parentLv
		if parentLv-1 < len(w.Catalog.LevelThresholds) {
			xpMap[prof.ParentProfession] = w.Catalog.LevelThresholds[parentLv-1]
		}
	}
	if len(xpMap) == 0 {
		xpMap = nil
	}

	var guilds []string
	if c.FactionID != "" {
		if f := w.faction(c.FactionID); f != nil {
			f.Members = appendUnique(f.Members, actorID)
			guilds = append(guilds, f.ID)
		}
	}

	var homeID string
	for i := range w.Houses {
		if w.Houses[i].District == c.District {
			homeID = w.Houses[i].ID
			w.Houses[i].Residents = appendUnique(w.Houses[i].Residents, actorID)
			break
		}
	}

	a := Actor{DigestedFood: digestedShare, NutritionReserve: resident.NutritionReserve,
		ID:           actorID,
		Name:         fmt.Sprintf("%s·%d", c.Name, c.HydratedSeq),
		Role:         baseRole,
		Tags:         tags,
		Attributes:   attrs,
		Professions:  profs,
		ProfessionXP: xpMap,
		Guilds:       guilds,
		Location:     c.District,
		InsideHouse:  "",
		HomeID:       homeID,
		AgeYears:     age,
		Health:       resident.Health,
		Energy:       energy,
		Hunger:       hunger,
		Thirst:       c.AvgThirst,
		SleepTrait:   SleepEarlyBird,
		Money:        money,
		Currencies:   actorCurrencies,
		Items:        items,
		Goal:         fmt.Sprintf("Hydrated from %s to practice %s and trade", c.Name, c.Profession),
		Personality:  "careful and candid",
		Trust:        map[string]int{},
	}
	w.placeResident(&a)
	w.refreshActor(&a)
	if c.Population == 0 {
		w.transferLastCohortAssets(c, &a)
	}
	w.Actors = append(w.Actors, a)
	return &w.Actors[len(w.Actors)-1], nil
}

// DehydrateActor absorbs an idle individual Actor back into its matching macro PopulationCohort
// (or creates one if none matches), conserving TotalPopulation(), money, and inventory.
func (w *World) DehydrateActor(actorID string) error {
	idx := -1
	for i := range w.Actors {
		if w.Actors[i].ID == actorID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown actor: %s", actorID)
	}
	a := w.Actors[idx]
	if a.Dead {
		return fmt.Errorf("cannot dehydrate dead actor %s", actorID)
	}
	if w.Tick < a.JailedUntil {
		return fmt.Errorf("cannot dehydrate jailed actor %s", actorID)
	}
	if a.Task != nil {
		return fmt.Errorf("cannot dehydrate busy actor %s", actorID)
	}
	if w.IsActorPinnedFocal(a) {
		return fmt.Errorf("cannot dehydrate pinned/high-tier/leader focal actor %s", actorID)
	}

	profID := a.Role
	profLv := a.Professions[profID]
	if profLv < 1 {
		for k, lv := range a.Professions {
			if lv > profLv {
				profID = k
				profLv = lv
			}
		}
	}
	if profLv < 1 {
		profLv = 1
	}

	var target *PopulationCohort
	for i := range w.Cohorts {
		if w.Cohorts[i].Profession == profID && w.Cohorts[i].District == a.Location {
			target = &w.Cohorts[i]
			break
		}
	}

	if target == nil {
		prof, _ := w.Catalog.ProfessionByID(profID)
		tier := prof.Tier
		if tier < 1 {
			tier = 1
		}
		w.Cohorts = append(w.Cohorts, PopulationCohort{
			ID:         "cohort_" + a.Location + "_" + profID,
			Name:       prof.Name + "群",
			District:   a.Location,
			FactionID:  prof.Guild,
			Profession: profID,
			Tier:       tier,
			ProfLevel:  profLv,
			Rank:       prof.RankTitle(profLv),
			AvgEnergy:  a.Energy,
			AvgHunger:  a.Hunger,
			AvgThirst:  a.Thirst,
		})
		target = &w.Cohorts[len(w.Cohorts)-1]
	}

	reconcileCohortAgeBrackets(target)
	w.adjustCohortMember(target, a, 1)
	target.Members = append(target.Members, CloneActor(a))
	// The stable actor ID continues to own credit contracts and personal assets.
	w.transferGrowingAssets("actor", actorID, "cohort", target.ID)

	// Remove pending offers involving this actor
	filtered := w.Offers[:0]
	for _, o := range w.Offers {
		if o.Seller != actorID && o.Buyer != actorID {
			filtered = append(filtered, o)
		}
	}
	w.Offers = filtered

	w.Actors = append(w.Actors[:idx], w.Actors[idx+1:]...)
	return nil
}

// stepCohorts advances macro PopulationCohorts and their compact resident ranges,
// handling circadian recovery, subsistence consumption, vectorized catalog recipe production,
// faction influence & cohort career promotion, and regional macro-micro trade clearing.
func (w *World) stepCohorts() []Event {
	if len(w.Cohorts) == 0 {
		return nil
	}
	var events []Event
	clock := ClockAt(w.Tick)
	nextTick := w.Tick + 1

	edibleKeys := make([]string, 0, 8)
	for itemID, spec := range w.Catalog.Items {
		if spec.Edible {
			edibleKeys = append(edibleKeys, itemID)
		}
	}
	sort.Strings(edibleKeys) // Same item order as consumeActorRation.

	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		if c.ItemPool == nil {
			c.ItemPool = map[string]int64{}
		}

		// 1. Circadian recovery at night
		if clock.IsNight {
			c.AvgEnergy += 12
			if c.AvgEnergy > MaxEnergy {
				c.AvgEnergy = MaxEnergy
			}
		}

		// 2. Exact members eat from their own inventories during actor physiology.
		events = append(events, w.settleResidentMeals(c, edibleKeys, nextTick)...)

		// 3. Staggered daytime bulk production & social exchange (only healthy, free, non-recuperating members)
		eff := c.EffectivePopulation()
		isCoerciveCraft := w.Catalog.HasCoercivePower([]string{c.Profession})
		isEdibleProducer := cohortProducesEdible(w.Catalog, c.Profession)
		if eff > 0 && w.Tick >= c.RestUntilTick && !clock.IsNight {
			if isCoerciveCraft {
				if clock.Hour == 10 && clock.Minute == 0 {
					events = append(events, w.runCohortProduction(c)...)
				}
			} else if isEdibleProducer {
				// Agricultural/pastoral/fishery/hunting food-chain shifts (8 shifts/day feeding 4 meals/capita/day)
				isFoodShiftHour := clock.Hour == 6 || clock.Hour == 8 || clock.Hour == 10 || clock.Hour == 11 ||
					clock.Hour == 13 || clock.Hour == 15 || clock.Hour == 17 || clock.Hour == 19
				if isFoodShiftHour && clock.Minute == (i%4)*15 {
					events = append(events, w.runCohortProduction(c)...)
				}
			} else {
				// Industrial, craft, medicinal & scholarly workshop shifts (2 shifts/day: morning & afternoon)
				if (clock.Hour == 9 || clock.Hour == 15) && clock.Minute == (i%4)*15 {
					events = append(events, w.runCohortProduction(c)...)
				}
			}
		}
		if eff > 0 && !clock.IsNight && int(w.Tick%4) == (i+2)%4 {
			socialBatches := int64(eff / 6)
			if socialBatches < 1 {
				socialBatches = 1
			}
			e := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: c.ID,
				Kind:    "cohort_socialized",
				Action:  Action{Kind: "speak", Location: c.District},
				Payload: map[string]any{
					"text":       fmt.Sprintf("%s exchanged %d local supply and guild reports in %s", c.Name, socialBatches, c.District),
					"cohort_id":  c.ID,
					"district":   c.District,
					"batches":    socialBatches,
					"population": c.Population,
				},
			}
			w.NextID++
			events = append(events, e)
		}
	}

	// 3b. Water, sanitation, nightsoil->compost recycling, epidemic checks, births & elder mortality
	events = append(events, w.stepCohortPhysiologySanitationAndLifecycle()...)

	// 4. Regional Macro-Micro Market Clearinghouse, Fiscal Taxation, Protection Rackets & Law/Oversight Engine
	if w.Tick%4 == 0 {
		events = append(events, w.clearRegionalCohortMarket()...)
		events = append(events, w.stepCohortDailyTaxation()...)
		events = append(events, w.stepCohortProtectionRackets()...)
		events = append(events, w.stepCohortConflicts()...)
		events = append(events, w.stepCohortBriberyAndCollusion()...)
		events = append(events, w.stepCohortAntiCorruptionAudit()...)
	}

	// 4b. Wound recuperation (herbal medicine/tonic treatment & rest) and Yamen jail sentence completion
	if w.Tick%(6*TicksPerHour) == 0 {
		events = append(events, w.stepCohortRecuperation()...)
	}

	// 5. Daily Tier-1 -> Tier-2 cohort graduation check
	if nextTick%TicksPerDay == 0 {
		events = append(events, w.promoteEligibleCohorts()...)
	}

	for i := range w.Cohorts {
		if len(w.Cohorts[i].ItemPool) == 0 {
			w.Cohorts[i].ItemPool = nil
		}
	}
	return events
}

func (w *World) selectCohortRecipe(c *PopulationCohort, prof Profession, guildLevel int, batches int64) (*Recipe, int64) {
	canUse := func(r *Recipe) bool {
		if w.EcologicalYieldMultiplierPct(c.District, *r) == 0 {
			return false
		}
		if err := w.Catalog.ValidateProductionConversion(*r); err != nil {
			return false
		}
		if r.NodeFeature != "" && !w.Map.DistrictHasFeature(c.District, r.NodeFeature) {
			return false
		}
		if !w.productionSiteOperational(c.District, "", "", *r) {
			return false
		}
		if _, free := w.productionFacility(c.District, "", "", *r); free < 1 {
			return false
		}
		if _, free := w.fieldAvailable("cohort", c.ID, c.District, "", *r); free < 1 {
			return false
		}

		return cohortMeetsRecipeTraining(w.Catalog, *c, prof, *r) && (r.MinFactionLevel == 0 || guildLevel >= r.MinFactionLevel)
	}
	hasInputs := func(r *Recipe) bool {
		return w.cohortRecipeBatches(c, *r, batches) > 0
	}

	findOutputRecipe := func(item string) *Recipe {
		for idx := range w.Catalog.Recipes {
			r := &w.Catalog.Recipes[idx]
			if r.OutputItem == item && canUse(r) && hasInputs(r) {
				return r
			}
		}
		return nil
	}

	// Collect eligible target recipes for this cohort (respecting realistic inventory caps to avoid runaway gluts)
	var candidates []*Recipe
	for idx := range w.Catalog.Recipes {
		r := &w.Catalog.Recipes[idx]
		if !canUse(r) {
			continue
		}
		// Skip pure intermediate input recipes (seeds, compost, herb) as top-level targets when final product recipes exist
		if r.ID == "save_seeds" || r.ID == "make_compost" || r.ID == "gather_herbs" {
			continue
		}
		outSpec := w.Catalog.Items[r.OutputItem]
		if outSpec.Potable && c.ItemPool[r.OutputItem] >= int64(c.Population)*2 {
			continue
		}
		if !outSpec.Edible && !outSpec.Potable {
			capPerCapita := int64(5)
			if outSpec.MedicinalHeal > 0 || outSpec.DiseaseCure > 0 {
				capPerCapita = 3
			}
			if c.ItemPool[r.OutputItem] >= int64(c.Population)*capPerCapita {
				continue
			}
		} else if outSpec.Edible {
			foodTarget := int64(c.Population) * 18
			if r.Growth != nil {
				// Planting must precede exhaustion by
				// the whole configured growth period, not an arbitrary few meals.
				foodTarget = max(foodTarget, int64(c.Population)*nutritionNeeded(r.Growth.DurationTicks+int64(r.Ticks)))
			}
			if w.cohortFoodRelief(*c) > foodTarget {
				continue
			}
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 {
		return nil, 0
	}
	// Rotate across eligible candidates (and prioritize farm_grain when food stock is below 5 per capita)
	target := candidates[int(w.Tick/4)%len(candidates)]
	if (c.Profession == "farmer" || c.Profession == "agronomist") && c.ItemPool["food"] < int64(c.Population)*5 {
		for _, cand := range candidates {
			if cand.ID == "farm_grain" {
				target = cand
				break
			}
		}
	}
	// Fund inputs only for work that fits the actual field, workstation, energy,
	// and extraction budgets. Extra workers do not imply extra planting demand.
	targetBatches := w.cohortRecipeCapacity(c, *target, batches)

	// Resolve missing required inputs first (e.g. save_seeds, make_compost, gather_herbs, brew_medicine, forge_tools, transcribe_scroll)
	inKeys := make([]string, 0, len(target.Inputs))
	for k := range target.Inputs {
		inKeys = append(inKeys, k)
	}
	sort.Strings(inKeys)
	for _, inItem := range inKeys {
		reqQty := int64(target.Inputs[inItem]) * targetBatches
		if c.ItemPool[inItem] < reqQty {
			if sub := findOutputRecipe(inItem); sub != nil {
				deficit := reqQty - c.ItemPool[inItem]
				needed := (deficit + int64(sub.OutputQty) - 1) / int64(sub.OutputQty)
				return sub, w.cohortRecipeBatches(c, *sub, min(batches, needed))
			}
		}
	}
	if hasInputs(target) {
		return target, w.cohortRecipeBatches(c, *target, targetBatches)
	}
	for _, cand := range candidates {
		if hasInputs(cand) {
			return cand, w.cohortRecipeBatches(c, *cand, batches)
		}
	}
	return nil, 0
}

// cohortRecipeBatches limits work to complete batches funded by existing stocks.
func (w *World) cohortRecipeBatches(c *PopulationCohort, r Recipe, requested int64) int64 {
	batches := w.cohortRecipeCapacity(c, r, requested)
	for item, qty := range r.StartRequirements() {
		if qty > 0 {
			batches = min(batches, c.ItemPool[item]/int64(qty))
		}
	}
	return batches
}

func (w *World) cohortRecipeCapacity(c *PopulationCohort, r Recipe, requested int64) int64 {
	if w.EcologicalYieldMultiplierPct(c.District, r) == 0 {
		return 0
	}
	batches := requested
	if c.AvgEnergy < r.EnergyCost {
		return 0
	}
	if r.EnergyCost > 0 {
		energy := int64(c.AvgEnergy)*int64(c.Population-len(c.Members)) - w.Production.EnergyRemainder[c.ID]
		batches = min(batches, max(int64(0), energy/int64(r.EnergyCost)))
	}
	_, facilityFree := w.productionFacility(c.District, "", "", r)
	_, landFree := w.fieldAvailable("cohort", c.ID, c.District, "", r)
	batches = min(batches, facilityFree, landFree)
	if r.MoneyCost > 0 {
		batches = min(batches, c.MoneyPool/int64(r.MoneyCost))
	}
	if r.ResourcePool != "" {
		batches = min(batches, w.availableDistrictResource(c.District, r.ResourcePool)/int64(maxInt(1, r.ResourceDraw)))
	}
	return batches
}

func (w *World) runCohortProduction(c *PopulationCohort) []Event {
	prof, ok := w.Catalog.ProfessionByID(c.Profession)
	if !ok {
		return nil
	}
	f := w.faction(c.FactionID)
	guildLevel := 1
	if f != nil {
		guildLevel = f.Level
	}

	eff := w.productionFreeWorkers(GrowthBatch{OwnerID: c.ID, Cohort: true, District: c.District})
	if eff <= 0 {
		return nil
	}
	// One worker starts one batch, exactly as an individual actor does.
	batches := eff
	chosen, batches := w.selectCohortRecipe(c, prof, guildLevel, batches)
	if chosen == nil {
		return nil
	}
	batches = w.cohortRecipeBatches(c, *chosen, batches)
	if batches <= 0 {
		return nil
	}
	houseID, _ := w.productionFacility(c.District, "", "", *chosen)
	if chosen.MoneyCost > 0 {
		spent := int64(chosen.MoneyCost) * batches
		c.MoneyPool -= spent
		w.creditTreasury(spent, "fee")
	}
	for inItem, inQty := range chosen.Inputs {
		w.adjustCohortItem(c, inItem, -int64(inQty)*batches, true)
	}

	bonusBatches := int64(0)
	if w.Catalog.ConsumablesYieldBonus(*chosen) {
		bonusBatches = min(batches, c.ItemPool[chosen.BonusInput])
		w.adjustCohortItem(c, chosen.BonusInput, -bonusBatches, true)
	}
	var produced int64
	if chosen.Growth == nil {
		funded := fundedYieldHundredths(chosen.OutputQty, w.EcologicalYieldMultiplierPct(c.District, *chosen), batches)
		var next int64
		produced, next = settleYieldCredit(w.yieldCredit("cohort", c.ID, chosen.ID), funded)
		w.setYieldCredit("cohort", c.ID, chosen.ID, next)
	} else {
		produced = int64(w.Catalog.RecipeYield(Actor{}, *chosen, false))*batches + bonusBatches*int64(chosen.BonusOutput)
	}
	w.wearProductionTool("cohort", c.ID, *chosen, batches)
	if chosen.ResourcePool != "" {
		w.drawDistrictResource(c.District, chosen.ResourcePool, batches*int64(maxInt(1, chosen.ResourceDraw)))
	}
	parents := map[string]int64{}
	if chosen.Growth != nil {
		for item, qty := range chosen.Growth.Parents {
			amount := int64(qty) * batches
			w.adjustCohortItem(c, item, -amount, false)
			parents[item] = amount
		}
	}
	if len(parents) == 0 {
		parents = nil
	}
	// Charge the same per-worker cost, retaining sub-point average costs.
	if chosen.EnergyCost > 0 {
		if w.Production.EnergyRemainder == nil {
			w.Production.EnergyRemainder = map[string]int64{}
		}
		energy := int64(chosen.EnergyCost)*batches + w.Production.EnergyRemainder[c.ID]
		anonymousPopulation := int64(c.Population - len(c.Members))
		c.AvgEnergy -= int(energy / anonymousPopulation)
		w.Production.EnergyRemainder[c.ID] = energy % anonymousPopulation
	}
	event := w.startGrowth(c.ID, c.District, "", true, *chosen, batches, produced, parents)
	g := &w.Growth[len(w.Growth)-1]
	g.Workers = int(batches)
	g.HouseID = houseID
	if chosen.Growth != nil {
		g.PreparationRemaining = int64(chosen.Ticks)
		completed := w.completeReservedShift(g, "prepare")
		if len(completed) > 0 {
			g.PreparationRemaining--
		}
		return append([]Event{event}, completed...)
	}
	return []Event{event}
}

func (w *World) effectiveCohortTradeUnitBatchPrice(districtID, itemID string, basePrice int) int {
	price := w.EffectiveMarketPrice(w.MarketCredit, districtID, itemID, basePrice)
	if w.SpotlightRumor != nil {
		if panicPermille := w.DistrictPanicDemandPermille(w.SpotlightRumor, districtID, itemID); panicPermille > 1000 {
			price = (price*(1000+(panicPermille-1000)/2) + 500) / 1000
		}
	}
	if w.Relations != nil {
		if ch, ok := w.Relations.ActiveCharterForItem(itemID, w.Tick); ok && ch.ExclusiveMarginPermille > 0 {
			price = (price*(1000+ch.ExclusiveMarginPermille) + 500) / 1000
		}
	}
	if price < 1 {
		price = 1
	}
	return price
}

func (w *World) clearRegionalCohortMarket() []Event {
	var events []Event

	// 1. Clear open individual Actor offers at market districts against macro Cohort demand
	remainingOffers := make([]Offer, 0, len(w.Offers))
	for _, o := range w.Offers {
		seller := w.actor(o.Seller)
		spec := w.Catalog.Items[o.Item]
		if o.Buyer != "" || o.ProposerID() != o.Seller || (o.Currency != "" && o.Currency != "coins") || !itemAcceptsCurrency(spec, "coins") || spec.BuyerRequiredTag != "" {
			remainingOffers = append(remainingOffers, o)
			continue
		}
		if seller == nil || o.Expires < w.Tick || seller.Items[o.Item] < o.Quantity || !w.Map.DistrictHasFeature(seller.Location, "market") {
			remainingOffers = append(remainingOffers, o)
			continue
		}
		var buyerCohort *PopulationCohort
		for i := range w.Cohorts {
			c := &w.Cohorts[i]
			if c.District != seller.Location || c.Population <= 0 || c.MoneyPool < int64(o.Price) {
				continue
			}
			if w.Catalog.TagsConsume([]string{"resident", c.Profession}, o.Item) && !w.Catalog.TagsProduce([]string{c.Profession}, o.Item) {
				if c.ItemPool[o.Item] < int64(c.Population)*3 {
					buyerCohort = c
					break
				}
			}
		}
		if buyerCohort == nil {
			remainingOffers = append(remainingOffers, o)
			continue
		}
		// Settle individual Seller -> macro Cohort trade
		seller.Items[o.Item] -= o.Quantity
		if seller.Items[o.Item] == 0 {
			delete(seller.Items, o.Item)
		}
		seller.Money += o.Price
		if buyerCohort.ItemPool == nil {
			buyerCohort.ItemPool = map[string]int64{}
		}
		buyerCohort.ItemPool[o.Item] += int64(o.Quantity)
		buyerCohort.MoneyPool -= int64(o.Price)
		act := Action{Kind: "accept", OfferID: o.ID, TargetID: seller.ID, Item: o.Item, Quantity: o.Quantity, Price: o.Price}
		e := w.record(seller, act, "trade_completed", map[string]any{
			"text":      fmt.Sprintf("%s bought %d %s from %s for %d coins", buyerCohort.Name, o.Quantity, o.Item, seller.Name, o.Price),
			"cohort_id": buyerCohort.ID,
			"seller_id": seller.ID,
			"buyer_id":  buyerCohort.ID,
			"item":      o.Item,
			"quantity":  o.Quantity,
		})
		events = append(events, e)
		tier := w.Catalog.Items[o.Item].Tier
		if tier < 1 {
			tier = 1
		}
		events = append(events, w.grantProfessionXP(seller, act, "peddler", tier*o.Quantity, tier)...)
	}
	w.Offers = remainingOffers

	// 1b. Bidirectional Focal Actor <-> Macro Cohort Retail/Wholesale Bridge:
	//     Present outdoor actors can obtain rations from local cohorts, get medicine,
	//     and sell surplus crafted output to demanding/merchant cohorts without starving in isolation.
	events = append(events, w.clearFocalActorCohortTrades()...)

	// 2. Two-Stage Inter-Cohort Logistics Clearing:
	//    Pass 0 clears local same-district trades at zero freight fee & zero transit loss;
	//    Pass 1 clears cross-district trades over multi-medium routes (waterway / land / mountain_pass)
	//    with capacity limits, carrier freight fees, transit loss, and storm shipping hazards.
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	routeCache := make(map[string]freightRoutePlan, 64)
	routeValid := make(map[string]bool, 64)
	getRoute := func(from, to, itemID string) (freightRoutePlan, bool) {
		key := itemID + ":" + from + "->" + to
		if valid, seen := routeValid[key]; seen {
			return routeCache[key], valid
		}
		plan, ok := w.planFreightRoute(from, to, itemID, cal)
		routeCache[key] = plan
		routeValid[key] = ok
		return plan, ok
	}

	itemKeys := make([]string, 0, len(w.Catalog.Items))
	for k := range w.Catalog.Items {
		itemKeys = append(itemKeys, k)
	}
	sort.Strings(itemKeys)

	for _, itemID := range itemKeys {
		spec := w.Catalog.Items[itemID]
		if !spec.Tradeable || spec.BatchQty <= 0 {
			continue
		}
		for si := range w.Cohorts {
			seller := &w.Cohorts[si]
			sellerTags := cohortIdentityTags(w.Catalog, seller.Profession)
			profession, _ := w.Catalog.ProfessionByID(seller.Profession)
			if seller.Population <= 0 || !w.Catalog.TagsProduce(sellerTags, itemID) && !profession.TradeMargin {
				continue
			}
			reserve := int64(seller.Population) / 2
			if spec.Edible {
				reserve = seller.ItemPool[itemID] - w.cohortFoodSurplus(*seller, itemID)
			}
			surplus := seller.ItemPool[itemID] - reserve
			if surplus < int64(spec.BatchQty) {
				continue
			}
			for pass := 0; pass < 2; pass++ {
				for bi := range w.Cohorts {
					if si == bi {
						continue
					}
					buyer := &w.Cohorts[bi]
					buyerTags := cohortIdentityTags(w.Catalog, buyer.Profession)
					if buyer.Population <= 0 || !spec.Edible && w.Catalog.TagsProduce(buyerTags, itemID) {
						continue
					}
					if pass == 0 && buyer.District != seller.District {
						continue
					}
					if pass == 1 && buyer.District == seller.District {
						continue
					}
					if !spec.Edible && !w.Catalog.TagsConsume(buyerTags, itemID) {
						continue
					}
					want := int64(buyer.Population) * 2
					if spec.Edible {
						if spec.NutritionUnits == 0 {
							continue
						}
						units := max(int64(0), int64(w.anonymousCohort(*buyer).Population)*DailyNutritionUnits*2-w.cohortFoodRelief(*buyer)-w.incomingNutrition(buyer.ID))
						want = buyer.ItemPool[itemID] + w.incomingStock(buyer.ID, itemID) + (units+spec.NutritionUnits-1)/spec.NutritionUnits
					}
					if w.SpotlightRumor != nil {
						if panicPermille := w.DistrictPanicDemandPermille(w.SpotlightRumor, buyer.District, itemID); panicPermille > 1000 {
							want = (want*int64(panicPermille) + 500) / 1000
						}
					}
					deficit := want - buyer.ItemPool[itemID] - w.incomingStock(buyer.ID, itemID)
					if deficit <= 0 {
						continue
					}
					var plan freightRoutePlan
					if pass == 0 {
						if w.IsItemEmbargoedAt(buyer.District, "", itemID) {
							continue
						}
						plan = freightRoutePlan{primaryMedium: "local"}
					} else {
						var ok bool
						plan, ok = getRoute(seller.District, buyer.District, itemID)
						if !ok {
							continue
						}
					}
					batches := deficit / int64(spec.BatchQty)
					maxSellBatches := surplus / int64(spec.BatchQty)
					if batches > maxSellBatches {
						batches = maxSellBatches
					}
					if pass == 1 {
						batches = min(batches, w.freightRouteAvailableBatches(plan))
					}
					unitBatchPrice := int64(w.effectiveCohortTradeUnitBatchPrice(buyer.District, itemID, spec.BatchPrice))
					effBatchCost := unitBatchPrice + (unitBatchPrice*int64(plan.feePermille)+999)/1000
					if effBatchCost < 1 {
						effBatchCost = 1
					}
					if maxAfford := buyer.MoneyPool / effBatchCost; batches > maxAfford {
						batches = maxAfford
					}
					if batches <= 0 {
						continue
					}

					qty := batches * int64(spec.BatchQty)
					cost := batches * unitBatchPrice
					var freightFee, lossQty int64
					if pass == 1 {
						w.reserveFreightRoute(plan, batches)
						if plan.feePermille > 0 {
							freightFee = (cost * int64(plan.feePermille)) / 1000
							if freightFee < 1 && buyer.MoneyPool > cost {
								freightFee = 1
							}
							if cost+freightFee > buyer.MoneyPool {
								freightFee = buyer.MoneyPool - cost
							}
						}
						lossQty = (qty * int64(plan.lossPermille)) / 1000
					}

					carrier := w.payFreightToCarriers(plan.carrierTag, freightFee)
					if carrier == nil {
						// No carrier cohort was paid; waive freight fee so M0 coins remain strictly conserved.
						freightFee = 0
					}
					if pass == 1 && plan.hazardPermille >= 18 {
						roll := int(worldHash(w.ID, w.Tick, fmt.Sprintf("ship_acc:%s:%s:%s", seller.ID, buyer.ID, itemID)) % 1000)
						if roll < plan.hazardPermille/2 {
							wreckLoss := maxInt64(1, qty/5)
							if lossQty+wreckLoss > qty {
								lossQty = qty
							} else {
								lossQty += wreckLoss
							}
							var cw, cd int
							if carrier != nil {
								cw, cd, _ = w.applyCohortCasualtiesWithCause(carrier, 2, roll%2, 0, "accident")
							}
							accEv := Event{
								ID:      w.NextID,
								Tick:    w.Tick,
								ActorID: seller.ID,
								Kind:    "freight_accident",
								Action:  Action{Kind: "move", Location: buyer.District, Item: itemID, Quantity: int(wreckLoss)},
								Payload: map[string]any{
									"text":       fmt.Sprintf("Freight convoy/barge (%s) carrying %s from %s to %s suffered storm/route wreck (-%d %s, %d injured, %d dead)", plan.primaryMedium, itemID, seller.District, buyer.District, wreckLoss, itemID, cw, cd),
									"medium":     plan.primaryMedium,
									"from":       seller.District,
									"to":         buyer.District,
									"item":       itemID,
									"cargo_lost": wreckLoss,
									"wounded":    cw,
									"deaths":     cd,
									"cause":      "accident",
								},
							}
							w.NextID++
							events = append(events, accEv)
						}
					}

					deliveredQty := qty - lossQty
					if deliveredQty < 0 {
						deliveredQty = 0
					}
					w.adjustCohortItem(seller, itemID, -lossQty, true)
					w.adjustCohortItem(seller, itemID, -(qty - lossQty), false)
					if deliveredQty > 0 {
						if pass == 0 {
							w.adjustCohortItem(buyer, itemID, deliveredQty, false)
						} else {
							w.Shipments = append(w.Shipments, Shipment{ID: w.NextID, BuyerID: buyer.ID, Item: itemID, Quantity: deliveredQty, From: seller.District, To: buyer.District, ArrivalTick: w.Tick + plan.ticks})
							w.NextID++
						}
					}
					// Market tax follows the buyer's place of settlement, exactly as a personal accept.
					marketTax := cost * int64(w.EffectiveMarketTaxPermille(buyer.District, "")) / 1000
					if marketTax > cost {
						marketTax = cost
					}
					creditTax := func(pol *PolityNode, tax int64) {
						if tax <= 0 {
							return
						}
						if pol == nil {
							w.ensureTreasuryLedger()
							w.TreasuryLedger.TaxAssessedGross += tax
							w.creditTreasury(tax, "tax")
						} else {
							pol.Treasury += tax
							pol.Ledger.TaxAssessedGross += tax
							pol.Ledger.TaxToTreasury += tax
						}
					}
					creditTax(w.EffectiveGoverningPolity(buyer.District, ""), marketTax)
					grossTax := marketTax
					var transitTax int64
					for _, edge := range plan.edges {
						fromPol := w.EffectiveGoverningPolity(edge.From, "")
						toPol := w.EffectiveGoverningPolity(edge.To, "")
						if fromPol != nil && toPol != nil && fromPol.ID == toPol.ID {
							continue
						}
						tax := cost * int64(w.Catalog.EffectiveGovernance().TransitCustomsPermille) / 1000
						if tax > cost-grossTax {
							tax = cost - grossTax
						}
						creditTax(toPol, tax)
						grossTax += tax
						transitTax += tax
					}
					netTax, skimmedTax := grossTax, int64(0)
					seller.MoneyPool += (cost - grossTax)
					buyer.MoneyPool -= (cost + freightFee)
					surplus -= qty

					e := Event{
						ID:       w.NextID,
						Tick:     w.Tick,
						ActorID:  seller.ID,
						TargetID: buyer.ID,
						Kind:     "cohort_traded",
						Action:   Action{Kind: "accept", TargetID: seller.ID, Item: itemID, Quantity: int(deliveredQty), Price: int(cost)},
						Payload: map[string]any{
							"text":          fmt.Sprintf("%s bought %d %s (%d batches via %s, fee %d, tax %d, loss %d) from %s for %d coins", buyer.Name, deliveredQty, itemID, batches, plan.primaryMedium, freightFee, grossTax, lossQty, seller.Name, cost),
							"seller_cohort": seller.ID,
							"buyer_cohort":  buyer.ID,
							"item":          itemID,
							"batches":       batches,
							"quantity":      deliveredQty,
							"shipped_qty":   qty,
							"loss_qty":      lossQty,
							"coins":         cost,
							"tax_assessed":  grossTax,
							"market_tax":    marketTax,
							"transit_tax":   transitTax,
							"tax_treasury":  netTax,
							"tax_skimmed":   skimmedTax,
							"freight_fee":   freightFee,
							"medium":        plan.primaryMedium,
						},
					}
					w.NextID++
					events = append(events, e)

					if surplus < int64(spec.BatchQty)*10 {
						break
					}
				}
				if surplus < int64(spec.BatchQty)*10 {
					break
				}
			}
		}
	}

	// Grant carrier & brokerage cohorts (waterway boatmen and land peddlers) XP & guild influence
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		prof, ok := w.Catalog.ProfessionByID(c.Profession)
		if !ok {
			continue
		}
		if prof.TradeMargin || ActorHasTag(prof.Grants, "water_carrier") || ActorHasTag(prof.Grants, "land_carrier") {
			c.AvgXP += 4
			if nextLv := w.Catalog.ProfessionLevel(c.AvgXP); nextLv > c.ProfLevel && nextLv <= prof.MaxLevel {
				c.ProfLevel = nextLv
				c.Rank = prof.RankTitle(nextLv)
			}
			if f := w.faction(c.FactionID); f != nil {
				f.Influence += 2 * c.ProfLevel
				if next := w.Catalog.FactionLevel(f.Influence); next > f.Level {
					f.Level = next
				}
			}
		}
	}

	return events
}

func cohortProducesEdible(cat Catalog, profID string) bool {
	for _, rec := range cat.Recipes {
		if rec.Profession == profID && cat.Items[rec.OutputItem].Edible {
			return true
		}
	}
	if prof, ok := cat.ProfessionByID(profID); ok && prof.ParentProfession != "" {
		for _, rec := range cat.Recipes {
			if rec.Profession == prof.ParentProfession && cat.Items[rec.OutputItem].Edible {
				return true
			}
		}
	}
	return false
}

func (w *World) clearFocalActorCohortTrades() []Event {
	var events []Event
	itemKeys := make([]string, 0, len(w.Catalog.Items))
	for k := range w.Catalog.Items {
		itemKeys = append(itemKeys, k)
	}
	sort.Strings(itemKeys)

	for ai := range w.Actors {
		a := &w.Actors[ai]
		if a.Dead || w.Tick < a.JailedUntil || a.InTransit() || a.InsideHouse != "" {
			continue
		}

		// A. Buy essential edible rations from local/surplus cohort if low on food
		edibleQty := a.NutritionReserve
		medQty := 0
		for _, k := range itemKeys {
			q := a.Items[k]
			if q <= 0 {
				continue
			}
			sp := w.Catalog.Items[k]
			if sp.Edible {
				edibleQty += int64(q) * sp.NutritionUnits
			}
			if sp.MedicinalHeal > 0 || sp.DiseaseCure > 0 {
				medQty += q
			}
		}
		if edibleQty < DailyNutritionUnits+int64(a.Hunger) {
			if sale, ok := w.buyLocalCohortRation(a, itemKeys, false); ok {
				events = append(events, sale)
			} else if a.Hunger >= 3 && w.PublicTreasury > 0 {
				if relief, ok := w.buyLocalCohortRation(a, itemKeys, true); ok {
					events = append(events, relief)
				}
			}
		}

		if a.Health < 25 {
			continue
		}

		// B. Buy medicinal dose if injured/ill and holding no medicine
		if (a.Health < 85 || a.DiseaseID != "") && medQty == 0 {
			for _, k := range itemKeys {
				sp := w.Catalog.Items[k]
				if (sp.MedicinalHeal <= 0 && sp.DiseaseCure <= 0) || !sp.Tradeable || sp.BatchQty <= 0 {
					continue
				}
				price := w.effectiveCohortTradeUnitBatchPrice(a.Location, k, maxInt(1, sp.BatchPrice))
				if a.Money < price {
					continue
				}
				for ci := range w.Cohorts {
					c := &w.Cohorts[ci]
					if c.District != a.Location {
						continue
					}
					if c.Population > 0 && c.ItemPool[k] > int64(maxInt(10, c.Population/2)) && c.ItemPool[k] >= int64(sp.BatchQty) {
						a.Money -= price
						if a.Items == nil {
							a.Items = map[string]int{}
						}
						a.Items[k] += sp.BatchQty
						c.ItemPool[k] -= int64(sp.BatchQty)
						c.MoneyPool += int64(price)
						act := Action{Kind: "accept", TargetID: c.ID, Item: k, Quantity: sp.BatchQty, Price: price}
						events = append(events, w.record(a, act, "trade_completed", map[string]any{
							"text":      fmt.Sprintf("%s bought %d %s from %s for %d coins", a.Name, sp.BatchQty, k, c.Name, price),
							"cohort_id": c.ID,
							"seller_id": c.ID,
							"buyer_id":  a.ID,
							"item":      k,
							"quantity":  sp.BatchQty,
						}))
						break
					}
				}
				if a.Items[k] > 0 {
					break
				}
			}
		}

		// C. Sell surplus crafted goods to demanding or merchant cohorts
		aTags := a.IdentityTags()
		for _, k := range itemKeys {
			sp := w.Catalog.Items[k]
			if !sp.Tradeable || sp.BatchQty <= 0 || !w.Catalog.TagsProduce(aTags, k) {
				continue
			}
			minHold := maxInt(4, sp.OfferThreshold+sp.BatchQty)
			if sp.Edible {
				minHold = 10
			}
			if a.Items[k] < minHold || a.Items[k] < sp.BatchQty {
				continue
			}
			price := w.Catalog.SalePrice(*a, k)
			if price < 1 {
				price = 1
			}
			var buyerCohort *PopulationCohort
			for ci := range w.Cohorts {
				c := &w.Cohorts[ci]
				if c.District != a.Location {
					continue
				}
				if c.Population <= 0 || c.MoneyPool < int64(price) {
					continue
				}
				cTags := cohortIdentityTags(w.Catalog, c.Profession)
				if w.Catalog.TagsProduce(cTags, k) {
					continue
				}
				isMerchant := false
				if cp, ok := w.Catalog.ProfessionByID(c.Profession); ok && cp.TradeMargin {
					isMerchant = true
				}
				if (w.Catalog.TagsConsume(cTags, k) || isMerchant) && c.ItemPool[k] < int64(c.Population)*4 {
					buyerCohort = c
					if c.District == a.Location {
						break
					}
				}
			}
			if buyerCohort != nil {
				a.Items[k] -= sp.BatchQty
				if a.Items[k] <= 0 {
					delete(a.Items, k)
				}
				a.Money += price
				if buyerCohort.ItemPool == nil {
					buyerCohort.ItemPool = map[string]int64{}
				}
				buyerCohort.ItemPool[k] += int64(sp.BatchQty)
				buyerCohort.MoneyPool -= int64(price)
				act := Action{Kind: "accept", TargetID: a.ID, Item: k, Quantity: sp.BatchQty, Price: price}
				events = append(events, w.record(a, act, "trade_completed", map[string]any{
					"text":      fmt.Sprintf("%s bought %d %s from %s for %d coins", buyerCohort.Name, sp.BatchQty, k, a.Name, price),
					"cohort_id": buyerCohort.ID,
					"seller_id": a.ID,
					"buyer_id":  buyerCohort.ID,
					"item":      k,
					"quantity":  sp.BatchQty,
				}))
				tier := maxInt(1, sp.Tier)
				events = append(events, w.grantProfessionXP(a, act, "peddler", tier*sp.BatchQty, tier)...)
				break
			}
		}
	}
	return events
}

// buyLocalCohortRation settles one whole local lot from its actual supplier.
// Privately paid purchases and public relief use the same stock, price and rights.
func (w *World) buyLocalCohortRation(a *Actor, itemKeys []string, public bool) (Event, bool) {
	budget := int64(a.Money)
	if public {
		budget = w.PublicTreasury
	}
	for _, item := range itemKeys {
		spec := w.Catalog.Items[item]
		if !spec.Edible || spec.BatchQty <= 0 || w.buyerItemBlockReason(*a, item, "coins") != "" {
			continue
		}
		price := w.effectiveCohortTradeUnitBatchPrice(a.Location, item, maxInt(1, spec.BatchPrice))
		if int64(price) > budget {
			continue
		}
		for i := range w.Cohorts {
			seller := &w.Cohorts[i]
			if seller.District != a.Location || seller.Population <= 0 || w.cohortFoodSurplus(*seller, item) < int64(spec.BatchQty) {
				continue
			}
			paid := int64(price)
			if public {
				paid = w.debitTreasury(paid, "relief")
			} else {
				a.Money -= price
			}
			seller.MoneyPool += paid
			seller.ItemPool[item] -= int64(spec.BatchQty)
			if a.Items == nil {
				a.Items = map[string]int{}
			}
			a.Items[item] += spec.BatchQty
			act := Action{Kind: "accept", TargetID: seller.ID, Item: item, Quantity: spec.BatchQty, Price: int(paid)}
			payload := map[string]any{"seller_id": seller.ID, "item": item, "quantity": spec.BatchQty}
			if public {
				payload["text"] = fmt.Sprintf("%s received %d %s bought from %s with %d treasury coins", a.Name, spec.BatchQty, item, seller.Name, paid)
				payload["recipient_id"], payload["coins"] = a.ID, paid
				return w.record(a, act, "food_relief_delivered", payload), true
			}
			payload["text"] = fmt.Sprintf("%s bought %d %s from %s for %d coins", a.Name, spec.BatchQty, item, seller.Name, paid)
			payload["buyer_id"], payload["cohort_id"] = a.ID, seller.ID
			return w.record(a, act, "trade_completed", payload), true
		}
	}
	return Event{}, false
}

func cohortIdentityTags(cat Catalog, profID string) []string {
	tags := []string{"resident", profID}
	if prof, ok := cat.ProfessionByID(profID); ok {
		for _, tag := range prof.Grants {
			tags = appendUnique(tags, tag)
		}
		if parent, ok := cat.ProfessionByID(prof.ParentProfession); ok {
			tags = appendUnique(tags, parent.ID)
			for _, tag := range parent.Grants {
				tags = appendUnique(tags, tag)
			}
		}
	}
	return tags
}

func cohortMeetsRecipeTraining(cat Catalog, c PopulationCohort, profession Profession, recipe Recipe) bool {
	if recipe.Profession != "" && recipe.Profession != c.Profession && (profession.ParentProfession == "" || recipe.Profession != profession.ParentProfession) {
		return false
	}
	if recipe.RequiredTag != "" && !ActorHasTag(cohortIdentityTags(cat, c.Profession), recipe.RequiredTag) {
		return false
	}
	level := c.ProfLevel
	if recipe.Profession != "" && recipe.Profession == profession.ParentProfession {
		level = max(level, profession.MinParentLevel)
	}
	return recipe.Profession == "" || level >= max(1, recipe.MinProfessionLevel)
}

func (w *World) promoteEligibleCohorts() []Event {
	var events []Event
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		reconcileCohortAgeBrackets(c)
		anonymous := w.anonymousCohort(*c)
		if c.Tier != 1 || anonymous.EffectivePopulation() < 40 {
			continue
		}
		for _, adv := range w.Catalog.Professions {
			if adv.ParentProfession != c.Profession {
				continue
			}
			reqLv := adv.MinParentLevel
			if reqLv < 2 {
				reqLv = 2
			}
			if c.ProfLevel < reqLv {
				continue
			}
			if adv.MinFactionLevel > 0 {
				if f := w.faction(adv.Guild); f == nil || f.Level < adv.MinFactionLevel {
					continue
				}
			}
			gradCount := anonymous.EffectivePopulation() / 25
			if gradCount < 1 {
				gradCount = 1
			}
			if gradCount > 5000 {
				gradCount = 5000
			}

			var dst *PopulationCohort
			for j := range w.Cohorts {
				if w.Cohorts[j].Profession == adv.ID && w.Cohorts[j].District == c.District {
					dst = &w.Cohorts[j]
					break
				}
			}
			if dst == nil {
				w.Cohorts = append(w.Cohorts, PopulationCohort{
					ID:         "cohort_" + c.District + "_" + adv.ID,
					Name:       adv.Name + "群",
					District:   c.District,
					FactionID:  adv.Guild,
					Profession: adv.ID,
					Tier:       adv.Tier,
					ProfLevel:  1,
					Rank:       adv.RankTitle(1),
					AvgEnergy:  c.AvgEnergy,
					AvgHunger:  c.AvgHunger,
					ItemPool:   map[string]int64{},
				})
				dst = &w.Cohorts[len(w.Cohorts)-1]
				// Refresh pointer c after slice append
				c = &w.Cohorts[i]
			}
			if dst.ItemPool == nil {
				dst.ItemPool = map[string]int64{}
			}

			oldPop := int64(anonymous.Population)
			moneyShare := (c.MoneyPool * int64(gradCount)) / oldPop
			c.MoneyPool -= moneyShare
			dst.MoneyPool += moneyShare
			for item, qty := range c.ItemPool {
				share := (qty * int64(gradCount)) / oldPop
				if share > 0 {
					w.adjustCohortItem(c, item, -share, false)
					w.adjustCohortItem(dst, item, share, false)
				}
			}
			w.transferCohortEnergyAndHunger(c, dst, gradCount, anonymous.Population, w.anonymousCohort(*dst).Population)
			w.transferCohortStoredShare(c, dst, gradCount, anonymous.Population)
			adultGraduates := min(gradCount, anonymous.Adults)
			elderGraduates := gradCount - adultGraduates
			c.Population -= gradCount
			c.Adults -= adultGraduates
			c.Elders -= elderGraduates
			dst.Population += gradCount
			dst.Adults += adultGraduates
			dst.Elders += elderGraduates
			reconcileCohortAgeBrackets(c)
			reconcileCohortAgeBrackets(dst)

			e := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: c.ID,
				Kind:    "cohort_promoted",
				Action:  Action{Kind: "join_profession", ProfessionID: adv.ID},
				Payload: map[string]any{
					"text":              fmt.Sprintf("%d villagers from %s promoted from %s to %s (%s)", gradCount, c.Name, c.Profession, adv.Name, dst.Rank),
					"from_cohort_id":    c.ID,
					"to_cohort_id":      dst.ID,
					"promoted_count":    gradCount,
					"parent_profession": c.Profession,
					"profession":        adv.ID,
					"rank":              dst.Rank,
				},
			}
			w.NextID++
			events = append(events, e)
			break
		}
	}
	return events
}

func (w *World) addCohortXP(c *PopulationCohort, xpGain, influenceGain int) []Event {
	if c == nil || c.Population <= 0 {
		return nil
	}
	var events []Event
	c.AvgXP += xpGain
	prof, ok := w.Catalog.ProfessionByID(c.Profession)
	if ok {
		newLevel := w.Catalog.ProfessionLevel(c.AvgXP)
		if newLevel > prof.MaxLevel {
			newLevel = prof.MaxLevel
		}
		if newLevel > c.ProfLevel {
			oldRank := c.Rank
			c.ProfLevel = newLevel
			c.Rank = prof.RankTitle(newLevel)
			e := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: c.ID,
				Kind:    "cohort_leveled",
				Action:  Action{Kind: "craft", ProfessionID: c.Profession},
				Payload: map[string]any{
					"text":          fmt.Sprintf("%s (%d villagers) advanced %s to level %d (%s)", c.Name, c.Population, c.Profession, c.ProfLevel, c.Rank),
					"cohort_id":     c.ID,
					"population":    c.Population,
					"profession":    c.Profession,
					"level":         c.ProfLevel,
					"rank":          c.Rank,
					"previous_rank": oldRank,
				},
			}
			w.NextID++
			events = append(events, e)
		}
	}
	if f := w.faction(c.FactionID); f != nil && influenceGain > 0 {
		f.Influence += influenceGain * c.ProfLevel
		if next := w.Catalog.FactionLevel(f.Influence); next > f.Level {
			f.Level = next
			e := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: c.ID,
				Kind:    "faction_leveled",
				Action:  Action{Kind: "craft", FactionID: f.ID},
				Payload: map[string]any{
					"text":       fmt.Sprintf("%s now stands at level %d", f.Name, next),
					"faction_id": f.ID,
					"level":      next,
					"influence":  f.Influence,
				},
			}
			w.NextID++
			events = append(events, e)
		}
	}
	return events
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// stepCohortRecuperation treats wounded cohort members declaratively using Catalog medicinal items and rest,
// and releases jailed prisoners who have completed their prefecture jail sentences.
func (w *World) stepCohortRecuperation() []Event {
	restoreMembers := w.detachCohortMembers()
	defer restoreMembers()
	var events []Event
	clock := ClockAt(w.Tick)
	medKeys := make([]string, 0, 4)
	for itemID, spec := range w.Catalog.Items {
		if spec.MedicinalHeal > 0 {
			medKeys = append(medKeys, itemID)
		}
	}
	sort.Slice(medKeys, func(i, j int) bool {
		hi := w.Catalog.Items[medKeys[i]].MedicinalHeal
		hj := w.Catalog.Items[medKeys[j]].MedicinalHeal
		if hi != hj {
			return hi > hj
		}
		return medKeys[i] < medKeys[j]
	})
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		if c.Wounded > 0 {
			rem := c.Wounded
			var totalMedUsed int64
			healed := 0
			for _, medID := range medKeys {
				avail := c.ItemPool[medID]
				if avail <= 0 || rem <= 0 {
					continue
				}
				perUnit := w.Catalog.Items[medID].MedicinalHeal / 10
				if perUnit < 2 {
					perUnit = 2
				}
				need := int64((rem + perUnit - 1) / perUnit)
				if need > avail {
					need = avail
				}
				w.adjustCohortItem(c, medID, -need, true)
				totalMedUsed += need
				h := int(need) * perUnit
				if h > rem {
					h = rem
				}
				healed += h
				rem -= h
			}
			if rem > 0 {
				restHeal := 0
				if clock.IsNight || w.Tick < c.RestUntilTick {
					restHeal = rem / 5
					if restHeal < 1 {
						restHeal = 1
					}
				} else {
					restHeal = rem / 12
				}
				if restHeal > rem {
					restHeal = rem
				}
				healed += restHeal
			}
			if healed > c.Wounded {
				healed = c.Wounded
			}
			if healed > 0 {
				c.ensureResidents(c.Population)
				c.updateResidentBandState(healed, func(b ResidentBand) bool { return b.Health < MaxHealth }, func(b *ResidentBand) { b.Health = MaxHealth })
				c.Wounded -= healed
				c.WoundedSick = min(c.WoundedSick, c.Wounded, c.Sick)
				e := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: c.ID,
					Kind:    "cohort_recuperated",
					Action:  Action{Kind: "medicate", Location: c.District},
					Payload: map[string]any{
						"text":              fmt.Sprintf("%s recuperated %d wounded members (%d remaining wounded; used %d medicinal doses)", c.Name, healed, c.Wounded, totalMedUsed),
						"cohort_id":         c.ID,
						"healed":            healed,
						"remaining_wounded": c.Wounded,
						"medicine_used":     totalMedUsed,
					},
				}
				w.NextID++
				events = append(events, e)
			}
		}

		// Daily magistrate jail docket at 18:00: prisoners who served sentences are released.
		if clock.Hour == 18 && c.Jailed > 0 {
			released := c.Jailed / 4
			if released < 1 {
				released = 1
			}
			c.ensureResidents(c.Population)
			c.updateResidentBandState(released, func(b ResidentBand) bool { return b.Jailed }, func(b *ResidentBand) { b.Jailed = false })
			c.Jailed -= released
			e := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: c.ID,
				Kind:    "cohort_released",
				Action:  Action{Kind: "wait", Location: c.District},
				Payload: map[string]any{
					"text":             fmt.Sprintf("Yamen jail released %d sentence-completed members back to %s (%d still imprisoned)", released, c.Name, c.Jailed),
					"cohort_id":        c.ID,
					"released":         released,
					"remaining_jailed": c.Jailed,
				},
			}
			w.NextID++
			events = append(events, e)
		}
	}
	return events
}

// stepCohortConflicts executes realistically scheduled prefecture-scale martial morning drills,
// mountain bandit ambushes & escort counterattacks, peak-hour market pickpocketing, nighttime gang
// turf wars, and daily Yamen law-enforcement sweeps—complete with wounds, jailings, deaths, and multi-day recuperation.
func (w *World) stepCohortConflicts() []Event {
	var events []Event
	for _, district := range w.sortedDistrictIDs() {
		events = append(events, w.stepCohortConflictsAt(district)...)
	}
	return events
}

func (w *World) stepCohortConflictsAt(district string) []Event {
	var events []Event
	clock := ClockAt(w.Tick)
	findCohort := func(prof string) *PopulationCohort {
		for i := range w.Cohorts {
			if w.Cohorts[i].District == district && w.Cohorts[i].Profession == prof && w.Cohorts[i].Population > 0 {
				if w.isLawCollectorCohort(&w.Cohorts[i]) && !w.cohortHasJurisdiction(&w.Cohorts[i], district) {
					continue
				}
				return &w.Cohorts[i]
			}
		}
		return nil
	}

	martial := findCohort("martial_artist")
	martialMaster := findCohort("martial_master")
	constables := findCohort("constable")
	chiefConstables := findCohort("chief_constable")
	guards := findCohort("guard")
	captains := findCohort("captain")
	bandits := findCohort("bandit")
	banditLeaders := findCohort("bandit_leader")
	thieves := findCohort("thief")
	masterThieves := findCohort("master_thief")
	gangsters := findCohort("gangster")
	gangBosses := findCohort("gang_boss")
	peddlers := findCohort("peddler")
	farmers := findCohort("farmer")
	woodcutters := findCohort("woodcutter")

	// 1. 08:00 晨间武馆演武切磋 (Morning Martial Hall Sparring — once per day, non-lethal)
	if clock.Hour == 8 && martial != nil && w.Tick >= martial.RestUntilTick && martial.EffectivePopulation() >= 4 {
		partner := guards
		if (w.Tick/TicksPerDay)%2 == 1 && constables != nil && constables.EffectivePopulation() >= 4 {
			partner = constables
		}
		if partner != nil && partner.EffectivePopulation() >= 4 {
			sparBatches := int64(martial.EffectivePopulation() / 30)
			if martialMaster != nil {
				sparBatches += int64(martialMaster.EffectivePopulation() / 20)
			}
			if sparBatches < 1 {
				sparBatches = 1
			}
			bruised, _, _ := w.applyCohortCasualtiesWithCause(martial, int(sparBatches/25), 0, 0, "combat")
			e := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  martial.ID,
				TargetID: partner.ID,
				Kind:     "cohort_sparred",
				Action:   Action{Kind: "spar", TargetID: partner.ID, Location: martial.District},
				Payload: map[string]any{
					"text":            fmt.Sprintf("%s and %s held %d morning sparring bouts in %s (%d minor sprains)", martial.Name, partner.Name, sparBatches, martial.District, bruised),
					"attacker_cohort": martial.ID,
					"defender_cohort": partner.ID,
					"batches":         sparBatches,
					"wounded":         bruised,
				},
			}
			w.NextID++
			events = append(events, e)
			events = append(events, w.addCohortXP(martial, 4, 2)...)
			events = append(events, w.addCohortXP(martialMaster, 4, 2)...)
			events = append(events, w.addCohortXP(partner, 3, 1)...)
		}
	}

	// 2. 06:00 / 17:00 山贼下山劫道 vs 武馆/巡守护镖反击 (Mountain Bandit Ambush & Escort Counterattack)
	if (clock.Hour == 6 || clock.Hour == 17) && bandits != nil && w.Tick >= bandits.RestUntilTick &&
		bandits.EffectivePopulation() >= 4 && bandits.EffectivePopulation()*100 >= bandits.Population*65 {
		target := woodcutters
		dayIdx := int(w.Tick/TicksPerDay) % 3
		if dayIdx == 1 && farmers != nil {
			target = farmers
		} else if dayIdx == 2 && peddlers != nil {
			target = peddlers
		}
		if target != nil && target.EffectivePopulation() >= 10 {
			raiderSquad := bandits.EffectivePopulation() / 25
			if banditLeaders != nil {
				raiderSquad += banditLeaders.EffectivePopulation() / 15
			}
			if raiderSquad < 4 {
				raiderSquad = 4
			}
			if raiderSquad > 85 {
				raiderSquad = 85
			}
			succeedBatches := int64(raiderSquad / 14)
			if succeedBatches < 1 {
				succeedBatches = 1
			}
			repelledBatches := int64(raiderSquad / 16)
			if repelledBatches < 1 {
				repelledBatches = 1
			}

			civWounded, civDead, _ := w.applyCohortCasualtiesWithCause(target, maxInt(1, raiderSquad/12), raiderSquad/55, 0, "combat")
			lootCoins := succeedBatches * 18
			if lootCoins > target.MoneyPool/50 {
				lootCoins = target.MoneyPool / 50
			}
			if lootCoins > 0 {
				target.MoneyPool -= lootCoins
				bandits.MoneyPool += lootCoins
			}
			lootItem := "food"
			if target.Profession == "woodcutter" && target.ItemPool["timber"] >= succeedBatches*6 {
				lootItem = "timber"
			}
			lootQty := succeedBatches * 6
			if lootQty > target.ItemPool[lootItem]/30 {
				lootQty = target.ItemPool[lootItem] / 30
			}
			if lootQty > 0 {
				w.adjustCohortItem(target, lootItem, -lootQty, false)
				w.adjustCohortItem(bandits, lootItem, lootQty, false)
			}

			raidEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  bandits.ID,
				TargetID: target.ID,
				Kind:     "cohort_raided",
				Action:   Action{Kind: "raid", TargetID: target.ID, Item: lootItem, Quantity: int(lootQty), Price: int(lootCoins)},
				Payload: map[string]any{
					"text":            fmt.Sprintf("%s dispatched %d raiders to ambush %s (%d caravan batches hit, %d %s & %d coins looted; caravan suffered %d wounded, %d dead)", bandits.Name, raiderSquad, target.Name, succeedBatches, lootQty, lootItem, lootCoins, civWounded, civDead),
					"attacker_cohort": bandits.ID,
					"target_cohort":   target.ID,
					"raider_squad":    raiderSquad,
					"batches":         succeedBatches,
					"item":            lootItem,
					"quantity":        lootQty,
					"coins":           lootCoins,
					"wounded":         civWounded,
					"deaths":          civDead,
				},
			}
			w.NextID++
			events = append(events, raidEv)
			events = append(events, w.addCohortXP(bandits, 5, 2)...)
			events = append(events, w.addCohortXP(banditLeaders, 6, 2)...)

			defender := martial
			if defender == nil || defender.EffectivePopulation() < 4 {
				defender = guards
			}
			if defender != nil && defender.EffectivePopulation() >= 4 {
				banditWounded, banditDead, _ := w.applyCohortCasualtiesWithCause(bandits, maxInt(2, raiderSquad/4), maxInt(1, raiderSquad/26), 0, "combat")
				defWounded, defDead, _ := w.applyCohortCasualtiesWithCause(defender, maxInt(1, raiderSquad/9), raiderSquad/65, 0, "combat")
				repelEv := Event{
					ID:       w.NextID,
					Tick:     w.Tick,
					ActorID:  defender.ID,
					TargetID: bandits.ID,
					Kind:     "cohort_raid_repelled",
					Action:   Action{Kind: "attack", TargetID: bandits.ID, Location: target.District},
					Payload: map[string]any{
						"text":            fmt.Sprintf("%s fought off %d bandit ambush squads from %s (bandits: %d wounded, %d killed; escorts: %d wounded, %d killed)", defender.Name, repelledBatches, bandits.Name, banditWounded, banditDead, defWounded, defDead),
						"defender_cohort": defender.ID,
						"attacker_cohort": bandits.ID,
						"batches":         repelledBatches,
						"wounded":         banditWounded + defWounded,
						"deaths":          banditDead + defDead,
					},
				}
				w.NextID++
				events = append(events, repelEv)
				events = append(events, w.addCohortXP(defender, 5, 2)...)
			}
			// Bandits retreat to stronghold to treat wounds and regroup for ~18-30 hours
			cooldown := int64(72)
			if bandits.Wounded > bandits.Population/5 {
				cooldown = 120
			}
			bandits.RestUntilTick = w.Tick + cooldown
			if banditLeaders != nil {
				banditLeaders.RestUntilTick = w.Tick + cooldown
			}
		}
	}

	// 3. 11:00 / 15:00 闹市小偷扒窃 (Peak-Hour Market Pickpocketing — deterred if merchants paid gang protection fees!)
	if (clock.Hour == 11 || clock.Hour == 15) && thieves != nil && peddlers != nil &&
		w.Tick >= thieves.RestUntilTick && thieves.EffectivePopulation() >= 3 {
		pickpockets := thieves.EffectivePopulation() / 38
		if masterThieves != nil {
			pickpockets += masterThieves.EffectivePopulation() / 20
		}
		protectedByGang := peddlers.ProtectedUntilTick > w.Tick
		if protectedByGang {
			pickpockets /= 2
		}
		if pickpockets < 1 {
			pickpockets = 1
		}
		if pickpockets > 42 {
			pickpockets = 42
		}
		caughtDiv := 7
		if protectedByGang {
			caughtDiv = 4
		}
		caught := pickpockets / caughtDiv
		woundedThieves, _, jailedThieves := w.applyCohortCasualtiesWithCause(thieves, caught, 0, maxInt(0, caught/2), "combat")
		succeeded := pickpockets - caught
		if succeeded < 1 {
			succeeded = 1
		}
		theftBatches := int64(succeeded)
		stolenCoins := theftBatches * 2
		if stolenCoins > peddlers.MoneyPool/80 {
			stolenCoins = peddlers.MoneyPool / 80
		}
		if stolenCoins > 0 {
			peddlers.MoneyPool -= stolenCoins
			thieves.MoneyPool += stolenCoins
		}
		stolenQty := theftBatches / 2
		if stolenQty > peddlers.ItemPool["food"]/80 {
			stolenQty = peddlers.ItemPool["food"] / 80
		}
		if stolenQty > 0 {
			peddlers.ItemPool["food"] -= stolenQty
			if thieves.ItemPool == nil {
				thieves.ItemPool = map[string]int64{}
			}
			thieves.ItemPool["food"] += stolenQty
		}
		if thieves.Jailed+thieves.Wounded > thieves.Population/3 {
			thieves.RestUntilTick = w.Tick + 48
		}
		stealEv := Event{
			ID:       w.NextID,
			Tick:     w.Tick,
			ActorID:  thieves.ID,
			TargetID: peddlers.ID,
			Kind:     "cohort_stolen",
			Action:   Action{Kind: "steal", TargetID: peddlers.ID, Item: "food", Quantity: int(stolenQty), Price: int(stolenCoins)},
			Payload: map[string]any{
				"text":              fmt.Sprintf("%s committed %d street thefts in %s (%d coins, %d food stolen; %d thieves beaten & %d jailed; gang_protected=%v)", thieves.Name, theftBatches, thieves.District, stolenCoins, stolenQty, woundedThieves, jailedThieves, protectedByGang),
				"thief_cohort":      thieves.ID,
				"victim_cohort":     peddlers.ID,
				"batches":           theftBatches,
				"coins":             stolenCoins,
				"quantity":          stolenQty,
				"wounded":           woundedThieves,
				"jailed":            jailedThieves,
				"protected_by_gang": protectedByGang,
			},
		}
		w.NextID++
		events = append(events, stealEv)
		events = append(events, w.addCohortXP(thieves, 3, 1)...)
		events = append(events, w.addCohortXP(masterThieves, 4, 2)...)
	}

	// 4. 21:00 夜间帮派堂口争抢保护费地盘火并 (Nighttime Underworld Gang Turf War — followed by multi-day recuperation!)
	if clock.Hour == 21 && gangsters != nil && w.Tick >= gangsters.RestUntilTick &&
		gangsters.EffectivePopulation() >= 4 && gangsters.EffectivePopulation()*10 >= gangsters.Population*7 {
		rival := bandits
		if (w.Tick/TicksPerDay)%2 == 0 && martial != nil && martial.EffectivePopulation() >= 4 {
			rival = martial
		}
		if rival != nil && rival.EffectivePopulation() >= 4 {
			fighterCount := gangsters.EffectivePopulation() / 18
			if gangBosses != nil {
				fighterCount += gangBosses.EffectivePopulation() / 12
			}
			if fighterCount < 4 {
				fighterCount = 4
			}
			if fighterCount > 110 {
				fighterCount = 110
			}
			warBatches := int64(fighterCount / 10)
			if warBatches < 1 {
				warBatches = 1
			}

			gangWounded, gangDead, _ := w.applyCohortCasualtiesWithCause(gangsters, maxInt(2, fighterCount/3), maxInt(1, fighterCount/22), 0, "combat")
			rivalWounded, rivalDead, _ := w.applyCohortCasualtiesWithCause(rival, maxInt(1, fighterCount/4), fighterCount/28, 0, "combat")

			if w.DistrictStates != nil {
				if ds, hasDS := w.DistrictStates[gangsters.District]; hasDS {
					ds.RacketPressure = minInt(100, ds.RacketPressure+8)
					w.DistrictStates[gangsters.District] = ds
				}
			}

			// Multi-day recuperation (48 hours = 192 ticks) after a major bloody turf war!
			gangsters.RestUntilTick = w.Tick + 192
			if gangBosses != nil {
				gangBosses.RestUntilTick = w.Tick + 192
			}

			turfEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  gangsters.ID,
				TargetID: rival.ID,
				Kind:     "cohort_turf_war",
				Action:   Action{Kind: "raid", TargetID: rival.ID, Location: gangsters.District},
				Payload: map[string]any{
					"text":         fmt.Sprintf("%s (%d fighters) fought a racket-turf war with %s in %s (%d wounded, %d killed; entering 2-day recuperation)", gangsters.Name, fighterCount, rival.Name, gangsters.District, gangWounded+rivalWounded, gangDead+rivalDead),
					"gang_cohort":  gangsters.ID,
					"rival_cohort": rival.ID,
					"fighters":     fighterCount,
					"batches":      warBatches,
					"coins":        int64(0),
					"wounded":      gangWounded + rivalWounded,
					"deaths":       gangDead + rivalDead,
				},
			}
			w.NextID++
			events = append(events, turfEv)
			events = append(events, w.addCohortXP(gangsters, 6, 3)...)
			events = append(events, w.addCohortXP(gangBosses, 8, 3)...)
		}
	}

	// 5. 14:00 午后府衙联合巡守缉盗剿匪 (Daily Prefecture Yamen Sweep — blunted if outlaws bribed corrupt patrols!)
	if clock.Hour == 14 && constables != nil && w.Tick >= constables.RestUntilTick && constables.EffectivePopulation() >= 3 {
		var outlaw *PopulationCohort
		switch int(w.Tick/TicksPerDay) % 3 {
		case 0:
			outlaw = bandits
		case 1:
			outlaw = thieves
		default:
			outlaw = gangsters
		}
		if outlaw == nil || outlaw.EffectivePopulation() < 2 {
			for _, cand := range []*PopulationCohort{bandits, gangsters, thieves} {
				if cand != nil && cand.EffectivePopulation() >= 2 {
					outlaw = cand
					break
				}
			}
		}
		if outlaw != nil && outlaw.EffectivePopulation() >= 2 {
			patrolSize := constables.EffectivePopulation() / 18
			if chiefConstables != nil {
				patrolSize += chiefConstables.EffectivePopulation() / 10
			}
			if captains != nil {
				patrolSize += captains.EffectivePopulation() / 14
			}
			if patrolSize < 3 {
				patrolSize = 3
			}
			if patrolSize > 95 {
				patrolSize = 95
			}
			collusionShielded := false
			if w.DistrictStates != nil {
				if ds, hasDS := w.DistrictStates[outlaw.District]; hasDS && ds.BribeShieldTicks > 0 {
					collusionShielded = true
					patrolSize = maxInt(2, (patrolSize*42)/100) // Corrupt tip-offs let most gangsters slip away!
				}
			}
			writNeed := int64(maxInt(1, patrolSize/8))
			if constables.ItemPool["bounty_writ"] >= writNeed {
				w.adjustCohortItem(constables, "bounty_writ", -writNeed, true)
				patrolSize += patrolSize / 6
			}

			capturedTarget := maxInt(1, patrolSize/4)
			var outlawWoundedReq, outlawDeadReq, lawWoundedReq, lawDeadReq int
			if outlaw.Profession == "thief" {
				outlawWoundedReq = maxInt(1, patrolSize/12)
				lawWoundedReq = patrolSize / 30
			} else {
				outlawWoundedReq = maxInt(1, patrolSize/6)
				outlawDeadReq = patrolSize / 32
				lawWoundedReq = maxInt(1, patrolSize/12)
				lawDeadReq = patrolSize / 75
			}
			outW, outD, jailedCount := w.applyCohortCasualtiesWithCause(outlaw, outlawWoundedReq, outlawDeadReq, capturedTarget, "combat")
			lawW, lawD, _ := w.applyCohortCasualtiesWithCause(constables, lawWoundedReq, lawDeadReq, 0, "combat")

			arrestBatches := int64(maxInt(1, jailedCount))
			confiscated := arrestBatches * 4
			if confiscated > outlaw.MoneyPool/25 {
				confiscated = outlaw.MoneyPool / 25
			}
			if confiscated > 0 {
				outlaw.MoneyPool -= confiscated
				treasuryShare := confiscated / 2
				victimRestitution := confiscated - treasuryShare
				w.creditTreasury(treasuryShare, "confiscated")
				if peddlers != nil {
					peddlers.MoneyPool += victimRestitution
				} else if farmers != nil {
					farmers.MoneyPool += victimRestitution
				} else {
					constables.MoneyPool += victimRestitution
				}
			}
			officialBounty := w.debitTreasury(arrestBatches*3, "bounty")
			constables.MoneyPool += officialBounty
			bountyCoins := officialBounty
			if outlaw.RestUntilTick < w.Tick+64 {
				outlaw.RestUntilTick = w.Tick + 64
			}

			arrestEv := Event{
				ID:       w.NextID,
				Tick:     w.Tick,
				ActorID:  constables.ID,
				TargetID: outlaw.ID,
				Kind:     "cohort_arrested",
				Action:   Action{Kind: "arrest", TargetID: outlaw.ID, Location: outlaw.District, Price: int(bountyCoins)},
				Payload: map[string]any{
					"text":               fmt.Sprintf("%s swept %s (collusion_shielded=%v): %d arrested & imprisoned, %d wounded, %d killed in resistance (confiscated %d coins, %d bounty)", constables.Name, outlaw.Name, collusionShielded, jailedCount, outW+lawW, outD+lawD, confiscated, bountyCoins),
					"yamen_cohort":       constables.ID,
					"outlaw_cohort":      outlaw.ID,
					"batches":            arrestBatches,
					"jailed":             jailedCount,
					"wounded":            outW + lawW,
					"deaths":             outD + lawD,
					"confiscated":        confiscated,
					"coins":              bountyCoins,
					"collusion_shielded": collusionShielded,
				},
			}
			w.NextID++
			events = append(events, arrestEv)
			events = append(events, w.addCohortXP(constables, 5, 2)...)
			events = append(events, w.addCohortXP(chiefConstables, 6, 3)...)
			events = append(events, w.addCohortXP(captains, 3, 1)...)
		}
	}

	return events
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ScaleSummary aggregates both individual Actor events and macro PopulationCohort
// batch/item/casualty volumes so million-scale simulations report true population-wide output, crime, and casualties.
type ScaleSummary struct {
	EventCounts        map[string]int   `json:"event_counts"`
	CraftBatches       int64            `json:"craft_batches"`
	ItemsProduced      int64            `json:"items_produced"`
	ProducedByItem     map[string]int64 `json:"produced_by_item"`
	TradeBatches       int64            `json:"trade_batches"`
	ItemsTraded        int64            `json:"items_traded"`
	CoinsTraded        int64            `json:"coins_traded"`
	WaterwayBatches    int64            `json:"waterway_batches"`
	WaterwayItems      int64            `json:"waterway_items"`
	LandFreightBatches int64            `json:"land_freight_batches"`
	FreightFees        int64            `json:"freight_fees"`
	TransitLossItems   int64            `json:"transit_loss_items"`
	Shipwrecks         int64            `json:"shipwrecks"`
	SocialInteractions int64            `json:"social_interactions"`
	PromotedPeople     int64            `json:"promoted_people"`
	CombatBatches      int64            `json:"combat_batches"`
	RaidIncidents      int64            `json:"raid_incidents"`
	RaidBatches        int64            `json:"raid_batches"`
	TheftBatches       int64            `json:"theft_batches"`
	TurfWarIncidents   int64            `json:"turf_war_incidents"`
	TurfWarBatches     int64            `json:"turf_war_batches"`
	SparBatches        int64            `json:"spar_batches"`
	ArrestSweepCount   int64            `json:"arrest_sweep_count"`
	ArrestBatches      int64            `json:"arrest_batches"`
	WoundedPeople      int64            `json:"wounded_people"`
	HealedPeople       int64            `json:"healed_people"`
	WoundedDeaths      int64            `json:"wounded_deaths"`
	JailedPeople       int64            `json:"jailed_people"`
	ReleasedPeople     int64            `json:"released_people"`
	JailedDeaths       int64            `json:"jailed_deaths"`
	Births             int64            `json:"births"`
	Deaths             int64            `json:"deaths"`
	DeathsByCause      map[string]int64 `json:"deaths_by_cause"`
	Successions        int64            `json:"successions"`
	MedicineUsed       int64            `json:"medicine_used"`
	LootCoins          int64            `json:"loot_coins"`
	BountyCoins        int64            `json:"bounty_coins"`
	Disasters          int64            `json:"disasters"`
	InfraRepairs       int64            `json:"infra_repairs"`
	WorkplaceAccidents int64            `json:"workplace_accidents"`
	WildEncounters     int64            `json:"wild_encounters"`
	ThreatSurges       int64            `json:"threat_surges"`
	ThreatHunts        int64            `json:"threat_hunts"`
	Rituals            int64            `json:"rituals"`
	EpidemicCases      int64            `json:"epidemic_cases"`
	CuredPeople        int64            `json:"cured_people"`
	SickDiedOther      int64            `json:"sick_died_other"`
	CompostRecycled    int64            `json:"compost_recycled"`
	TaxAssessed        int64            `json:"tax_assessed"`
	TaxToTreasury      int64            `json:"tax_to_treasury"`
	TaxSkimmed         int64            `json:"tax_skimmed"`
	TaxSurcharge       int64            `json:"tax_surcharge"`
	EmbezzledCoins     int64            `json:"embezzled_coins"`
	BribeIncidents     int64            `json:"bribe_incidents"`
	BribeCoins         int64            `json:"bribe_coins"`
	BribeReleased      int64            `json:"bribe_released"`
	AuditPurges        int64            `json:"audit_purges"`
	CorruptionJailed   int64            `json:"corruption_jailed"`
	ConfiscatedCoins   int64            `json:"confiscated_coins"`
	RacketIncidents    int64            `json:"racket_incidents"`
	RacketBatches      int64            `json:"racket_batches"`
	RacketClashes      int64            `json:"racket_clashes"`
	ProtectionCoins    int64            `json:"protection_coins"`

	// Pillar 1: Market & Credit
	HoardEvents        int64 `json:"hoard_events"`
	HoardReleased      int64 `json:"hoard_released"`
	LoansIssued        int64 `json:"loans_issued"`
	LoanPrincipalCoins int64 `json:"loan_principal_coins"`
	LoansRepaid        int64 `json:"loans_repaid"`
	LoanRepaidCoins    int64 `json:"loan_repaid_coins"`
	HousesForeclosed   int64 `json:"houses_foreclosed"`
	HouseRentCoins     int64 `json:"house_rent_coins"`
	CohortLandsAnnexed int64 `json:"cohort_lands_annexed"`
	CohortRentCoins    int64 `json:"cohort_rent_coins"`

	// Pillar 2: Relations, Kinship & Sovereignty
	Marriages            int64 `json:"marriages"`
	BondsSworn           int64 `json:"bonds_sworn"`
	InheritancesSettled  int64 `json:"inheritances_settled"`
	InheritanceDisputes  int64 `json:"inheritance_disputes"`
	GrudgesFormed        int64 `json:"grudges_formed"`
	FeudsAvenged         int64 `json:"feuds_avenged"`
	FeudsMediated        int64 `json:"feuds_mediated"`
	VendettaClashes      int64 `json:"vendetta_clashes"`
	TurfShifts           int64 `json:"turf_shifts"`
	FactionLeaderChanges int64 `json:"faction_leader_changes"`
	MonopolyCharters     int64 `json:"monopoly_charters"`

	// Pillar 3: Justice & Peasant Revolt
	CourtTrials         int64 `json:"court_trials"`
	WrongfulConvictions int64 `json:"wrongful_convictions"`
	AppealsOverturned   int64 `json:"appeals_overturned"`
	TaxStrikes          int64 `json:"tax_strikes"`
	GranaryRiots        int64 `json:"granary_riots"`
	RevoltsErupted      int64 `json:"revolts_erupted"`
	RevoltsSuppressed   int64 `json:"revolts_suppressed"`
	AmnestiesDecreed    int64 `json:"amnesties_decreed"`
	RebelsReintegrated  int64 `json:"rebels_reintegrated"`

	// Pillar 4: Spotlight & Rumor
	SpotlightPromotions int64 `json:"spotlight_promotions"`
	SpotlightDemotions  int64 `json:"spotlight_demotions"`
	RumorPanics         int64 `json:"rumor_panics"`
	GossipSeeded        int64 `json:"gossip_seeded"`
}

func NewScaleSummary() *ScaleSummary {
	return &ScaleSummary{
		EventCounts:    map[string]int{},
		ProducedByItem: map[string]int64{},
		DeathsByCause:  map[string]int64{},
	}
}

func payloadInt64(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func (s *ScaleSummary) RecordEvents(events []Event) {
	if s.DeathsByCause == nil {
		s.DeathsByCause = map[string]int64{}
	}
	for _, e := range events {
		s.EventCounts[e.Kind]++
		switch e.Kind {
		case "crafted", "harvested":
			s.CraftBatches++
			qty := payloadInt64(e.Payload, "quantity")
			if qty <= 0 {
				qty = 1
			}
			s.ItemsProduced += qty
			if item, ok := e.Payload["item"].(string); ok && item != "" {
				s.ProducedByItem[item] += qty
			}
		case "cohort_crafted":
			batches := payloadInt64(e.Payload, "batches")
			qty := payloadInt64(e.Payload, "quantity")
			s.CraftBatches += batches
			s.ItemsProduced += qty
			if item, ok := e.Payload["item"].(string); ok && item != "" {
				s.ProducedByItem[item] += qty
			}
		case "trade_completed":
			s.TradeBatches++
			qty := int64(e.Action.Quantity)
			if qty <= 0 {
				qty = 1
			}
			s.ItemsTraded += qty
			if e.Action.Currency == "" || e.Action.Currency == "coins" {
				s.CoinsTraded += int64(e.Action.Price)
			}
		case "cohort_traded":
			b := payloadInt64(e.Payload, "batches")
			q := payloadInt64(e.Payload, "quantity")
			s.TradeBatches += b
			s.ItemsTraded += q
			s.CoinsTraded += payloadInt64(e.Payload, "coins")
			s.TaxAssessed += payloadInt64(e.Payload, "tax_assessed")
			s.TaxToTreasury += payloadInt64(e.Payload, "tax_treasury")
			s.TaxSkimmed += payloadInt64(e.Payload, "tax_skimmed")
			s.FreightFees += payloadInt64(e.Payload, "freight_fee")
			s.TransitLossItems += payloadInt64(e.Payload, "loss_qty")
			if med, _ := e.Payload["medium"].(string); med == "waterway" {
				s.WaterwayBatches += b
				s.WaterwayItems += q
			} else if med != "" && med != "local" {
				s.LandFreightBatches += b
			}
		case "freight_accident":
			s.Shipwrecks++
			s.WorkplaceAccidents++
			s.TransitLossItems += payloadInt64(e.Payload, "cargo_lost")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["accident"] += d
			}
		case "speech":
			s.SocialInteractions++
		case "cohort_socialized":
			s.SocialInteractions += payloadInt64(e.Payload, "batches")
		case "profession_joined":
			s.PromotedPeople++
		case "cohort_promoted":
			s.PromotedPeople += payloadInt64(e.Payload, "promoted_count")
		case "actor_died":
			s.Deaths++
			cause, _ := e.Payload["cause"].(string)
			cause = canonicalDeathCause(cause)
			s.DeathsByCause[cause]++
			if wasSick, _ := e.Payload["was_sick"].(bool); wasSick && cause != "disease" {
				s.SickDiedOther++
			}
			if wasJailed, _ := e.Payload["was_jailed"].(bool); wasJailed {
				s.JailedDeaths++
			}
		case "cohort_mortality_status":
			s.WoundedDeaths += payloadInt64(e.Payload, "wounded_deaths")
			s.SickDiedOther += payloadInt64(e.Payload, "sick_died_other")
			s.JailedDeaths += payloadInt64(e.Payload, "jailed_deaths")
		case "actor_health_reconciled":
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			s.HealedPeople += payloadInt64(e.Payload, "healed")
			s.WoundedDeaths += payloadInt64(e.Payload, "wounded_deaths")
		case "actor_child_born":
			s.Births++
		case "actor_succeeded":
			s.Successions++
		case "cohort_demographics":
			s.Births += payloadInt64(e.Payload, "births")
			ed := payloadInt64(e.Payload, "elder_deaths")
			s.Deaths += ed
			if ed > 0 {
				s.DeathsByCause["old_age"] += ed
			}
		case "cohort_meals_settled":
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
		case "cohort_famine_deaths":
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["starvation"] += d
			}
		case "cohort_dehydration_deaths":
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["dehydration"] += d
			}
		case "cohort_epidemic":
			s.EpidemicCases += payloadInt64(e.Payload, "new_sick")
			cured := payloadInt64(e.Payload, "cured")
			s.CuredPeople += cured
			s.MedicineUsed += payloadInt64(e.Payload, "medicine_used")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["disease"] += d
			}
		case "contracted_disease":
			s.EpidemicCases++
		case "recovered_disease":
			s.CuredPeople++
		case "nightsoil_composted":
			s.CompostRecycled += payloadInt64(e.Payload, "compost_generated")
		case "disaster_struck":
			s.Disasters++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded") - payloadInt64(e.Payload, "actor_wounded")
			d := payloadInt64(e.Payload, "deaths") - payloadInt64(e.Payload, "actor_deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["disaster"] += d
			}
		case "infra_maintained":
			s.InfraRepairs++
		case "workplace_accident":
			s.WorkplaceAccidents++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["accident"] += d
			}
		case "wild_encounter":
			s.WildEncounters++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["beast"] += d
			}
		case "threat_surge":
			s.ThreatSurges++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded") - payloadInt64(e.Payload, "actor_wounded")
			d := payloadInt64(e.Payload, "deaths") - payloadInt64(e.Payload, "actor_deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["beast"] += d
			}
		case "cohort_threat_hunted", "threat_hunted":
			s.ThreatHunts++
			s.BountyCoins += payloadInt64(e.Payload, "bounty_coins")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["beast"] += d
			}
		case "cohort_ritual", "ritual_performed":
			s.Rituals++
		case "medicated", "healed":
			s.MedicineUsed++
			if cd, _ := e.Payload["cured_disease"].(string); cd != "" {
				s.CuredPeople++
			}
		case "released_from_jail":
			s.ReleasedPeople++
		case "cohort_recuperated":
			s.HealedPeople += payloadInt64(e.Payload, "healed")
			s.MedicineUsed += payloadInt64(e.Payload, "medicine_used") + payloadInt64(e.Payload, "tonic_used")
		case "cohort_released":
			s.ReleasedPeople += payloadInt64(e.Payload, "released")
		case "attacked":
			s.CombatBatches++
		case "sparred":
			s.CombatBatches++
			s.SparBatches++
		case "cohort_sparred":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.SparBatches += b
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
		case "raided":
			s.CombatBatches++
			s.RaidIncidents++
			s.RaidBatches++
			s.LootCoins += payloadInt64(e.Payload, "coins")
		case "raid_repelled":
			s.CombatBatches++
			s.RaidBatches++
		case "cohort_raided":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.RaidIncidents++
			s.RaidBatches += b
			s.LootCoins += payloadInt64(e.Payload, "coins")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "cohort_raid_repelled":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.RaidBatches += b
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "stolen", "steal_foiled":
			s.CombatBatches++
			s.TheftBatches++
			s.LootCoins += payloadInt64(e.Payload, "coins")
		case "cohort_stolen":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.TheftBatches += b
			s.LootCoins += payloadInt64(e.Payload, "coins")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
		case "cohort_turf_war":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.TurfWarIncidents++
			s.TurfWarBatches += b
			s.LootCoins += payloadInt64(e.Payload, "coins")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "arrested", "arrest_resisted":
			s.CombatBatches++
			s.ArrestBatches++
			s.BountyCoins += payloadInt64(e.Payload, "coins")
			if e.Kind == "arrested" && payloadInt64(e.Payload, "jailed_until") > 0 {
				s.JailedPeople++
			}
		case "cohort_arrested":
			b := payloadInt64(e.Payload, "batches")
			s.CombatBatches += b
			s.ArrestSweepCount++
			s.ArrestBatches += b
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
			s.BountyCoins += payloadInt64(e.Payload, "coins")
		case "cohort_tax_collected":
			s.TaxAssessed += payloadInt64(e.Payload, "gross_tax")
			s.TaxToTreasury += payloadInt64(e.Payload, "net_treasury")
			s.TaxSkimmed += payloadInt64(e.Payload, "tax_skimmed")
			s.TaxSurcharge += payloadInt64(e.Payload, "surcharge_coins")
		case "cohort_embezzled":
			s.EmbezzledCoins += payloadInt64(e.Payload, "embezzled_coins")
		case "extorted":
			s.RacketIncidents++
			s.RacketBatches++
			s.ProtectionCoins += payloadInt64(e.Payload, "protection_coins")
		case "extort_resisted":
			s.RacketClashes++
			s.CombatBatches++
		case "cohort_extorted":
			s.RacketIncidents++
			s.RacketBatches += payloadInt64(e.Payload, "batches")
			s.ProtectionCoins += payloadInt64(e.Payload, "protection_coins")
		case "cohort_racket_clashed":
			b := payloadInt64(e.Payload, "batches")
			s.RacketIncidents++
			s.RacketClashes++
			s.RacketBatches += b
			s.CombatBatches += b
			s.ProtectionCoins += payloadInt64(e.Payload, "protection_coins")
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "bribed":
			s.BribeIncidents++
			s.BribeCoins += payloadInt64(e.Payload, "bribe_coins")
		case "cohort_bribed":
			s.BribeIncidents++
			s.BribeCoins += payloadInt64(e.Payload, "bribe_coins")
			rel := payloadInt64(e.Payload, "bribe_released")
			s.BribeReleased += rel
			s.ReleasedPeople += rel
		case "corruption_purged":
			s.AuditPurges++
			s.ConfiscatedCoins += payloadInt64(e.Payload, "confiscated_coins")
			if payloadInt64(e.Payload, "jailed_until") > 0 {
				s.JailedPeople++
				s.CorruptionJailed++
			}
		case "cohort_corruption_purged":
			s.AuditPurges++
			s.ConfiscatedCoins += payloadInt64(e.Payload, "confiscated_coins")
			j := payloadInt64(e.Payload, "jailed")
			s.JailedPeople += j
			s.CorruptionJailed += j
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")

		// Pillar 1: Market & Credit
		case "merchant_hoarded":
			s.HoardEvents++
		case "hoard_released":
			s.HoardReleased++
		case "loan_issued", "cohort_loan_issued":
			s.LoansIssued++
			s.LoanPrincipalCoins += payloadInt64(e.Payload, "principal")
		case "loan_repaid", "cohort_loan_repaid":
			s.LoansRepaid++
			s.LoanRepaidCoins += payloadInt64(e.Payload, "coins")
		case "house_foreclosed":
			s.HousesForeclosed++
		case "house_rent_paid":
			s.HouseRentCoins += payloadInt64(e.Payload, "coins")
		case "cohort_land_annexed":
			s.CohortLandsAnnexed++
		case "cohort_rent_paid":
			s.CohortRentCoins += payloadInt64(e.Payload, "coins")

		// Pillar 2: Relations, Kinship & Sovereignty
		case "actor_married":
			s.Marriages++
			s.SocialInteractions++
		case "cohort_wedding_held":
			couples := payloadInt64(e.Payload, "couples")
			if couples <= 0 {
				couples = 1
			}
			s.Marriages += couples
			s.SocialInteractions += couples
		case "bond_sworn":
			s.BondsSworn++
			s.SocialInteractions++
		case "inheritance_settled":
			s.InheritancesSettled++
		case "inheritance_dispute":
			s.InheritanceDisputes++
		case "grudge_formed":
			s.GrudgesFormed++
		case "feud_avenged":
			s.FeudsAvenged++
			s.CombatBatches++
			s.LootCoins += payloadInt64(e.Payload, "coins")
		case "feud_mediated":
			s.FeudsMediated++
			s.SocialInteractions++
		case "vendetta_clash":
			s.VendettaClashes++
			s.CombatBatches++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "district_control_shifted":
			s.TurfShifts++
		case "faction_leader_changed":
			s.FactionLeaderChanges++
		case "monopoly_chartered":
			s.MonopolyCharters++

		// Pillar 3: Justice & Peasant Revolt
		case "court_trial_held":
			s.CourtTrials++
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
		case "wrongful_conviction":
			s.WrongfulConvictions++
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
		case "conviction_overturned":
			s.AppealsOverturned++
			s.ReleasedPeople += payloadInt64(e.Payload, "released")
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
			s.ConfiscatedCoins += payloadInt64(e.Payload, "confiscated_coins")
		case "tax_strike":
			s.TaxStrikes++
		case "granary_riot":
			s.GranaryRiots++
			s.CombatBatches++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
		case "revolt_erupted":
			s.RevoltsErupted++
		case "revolt_suppressed":
			s.RevoltsSuppressed++
			s.CombatBatches++
			s.WoundedPeople += payloadInt64(e.Payload, "wounded")
			s.JailedPeople += payloadInt64(e.Payload, "jailed")
			d := payloadInt64(e.Payload, "deaths")
			s.Deaths += d
			if d > 0 {
				s.DeathsByCause["combat"] += d
			}
			s.BountyCoins += payloadInt64(e.Payload, "bounty_coins")
		case "amnesty_decreed":
			s.AmnestiesDecreed++
			s.ReleasedPeople += payloadInt64(e.Payload, "released")
		case "rebel_reintegrated":
			s.RebelsReintegrated += payloadInt64(e.Payload, "reintegrated_adults")

		// Pillar 4: Spotlight & Rumor
		case "spotlight_promoted":
			s.SpotlightPromotions++
		case "spotlight_demoted":
			s.SpotlightDemotions++
		case "rumor_panic_triggered":
			s.RumorPanics++
		case "gossip_seeded":
			s.GossipSeeded++
		}
	}
}

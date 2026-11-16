package sim

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"
)

// SeasonSpec defines a named season in a world's declarative calendar.
type SeasonSpec struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	FeatureYieldPct  map[string]int `json:"feature_yield_pct,omitempty"`
	WeatherWeights   map[string]int `json:"weather_weights,omitempty"`
	ColdExposure     int            `json:"cold_exposure,omitempty"`
	CanalCapacityPct int            `json:"canal_capacity_pct,omitempty"`
	RouteCapacityPct map[string]int `json:"route_capacity_pct,omitempty"`
}

// CalendarSpec defines configurable season length and annual cycle (e.g. 7 days/season = 28 days/year, or 365 days/year).
type CalendarSpec struct {
	DaysPerSeason int          `json:"days_per_season"`
	Seasons       []SeasonSpec `json:"seasons,omitempty"`
}

// CalendarState is the derived calendar date for any quarter-hour simulation tick.
type CalendarState struct {
	Year        int    `json:"year"`
	SeasonIndex int    `json:"season_index"`
	SeasonID    string `json:"season_id"`
	SeasonName  string `json:"season_name"`
	DayOfSeason int    `json:"day_of_season"`
	DayOfYear   int    `json:"day_of_year"`
	DaysInYear  int    `json:"days_in_year"`
}

// CalendarAt computes the year, season, and day within season from tick and CalendarSpec.
func CalendarAt(tick int64, spec CalendarSpec) CalendarState {
	clock := ClockAt(tick)
	daysPerSeason := spec.DaysPerSeason
	if daysPerSeason <= 0 {
		daysPerSeason = 7
	}
	numSeasons := len(spec.Seasons)
	if numSeasons == 0 {
		daysInYear := daysPerSeason * 4
		zeroDay := clock.Day - 1
		year := zeroDay/daysInYear + 1
		dayOfYear := zeroDay%daysInYear + 1
		sIdx := (dayOfYear - 1) / daysPerSeason
		return CalendarState{
			Year:        year,
			SeasonIndex: sIdx,
			SeasonID:    "default",
			SeasonName:  "Default",
			DayOfSeason: (dayOfYear-1)%daysPerSeason + 1,
			DayOfYear:   dayOfYear,
			DaysInYear:  daysInYear,
		}
	}
	daysInYear := daysPerSeason * numSeasons
	zeroDay := clock.Day - 1
	year := zeroDay/daysInYear + 1
	dayOfYear := zeroDay%daysInYear + 1
	sIdx := (dayOfYear - 1) / daysPerSeason
	if sIdx >= numSeasons {
		sIdx = numSeasons - 1
	}
	s := spec.Seasons[sIdx]
	return CalendarState{
		Year:        year,
		SeasonIndex: sIdx,
		SeasonID:    s.ID,
		SeasonName:  s.Name,
		DayOfSeason: (dayOfYear-1)%daysPerSeason + 1,
		DayOfYear:   dayOfYear,
		DaysInYear:  daysInYear,
	}
}

// WeatherSpec defines a declarative weather state and its physical effects on yields, travel, shipping, and exposure.
type WeatherSpec struct {
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	TravelTickPenalty     int            `json:"travel_tick_penalty,omitempty"`
	TravelEnergyPenalty   int            `json:"travel_energy_penalty,omitempty"`
	RouteHazardPermille   map[string]int `json:"route_hazard_permille,omitempty"`
	RouteCapacityPct      map[string]int `json:"route_capacity_pct,omitempty"`
	FeatureYieldPct       map[string]int `json:"feature_yield_pct,omitempty"`
	ResourceRegenPct      map[string]int `json:"resource_regen_pct,omitempty"`
	AccidentMultiplierPct int            `json:"accident_multiplier_pct,omitempty"`
	ColdDamage            int            `json:"cold_damage,omitempty"`
}

// ResourcePoolSpec defines a finite regional ecological pool with daily regeneration and depletion floor.
type ResourcePoolSpec struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	NodeFeatures        []string       `json:"node_features,omitempty"`
	PotableWater        bool           `json:"potable_water,omitempty"`
	RegenPermillePerDay int            `json:"regen_permille_per_day"`
	SeasonalRegenPct    map[string]int `json:"seasonal_regen_pct,omitempty"`
	DepletionFloorPct   int            `json:"depletion_floor_pct,omitempty"`
}

// EncounterSpec defines a declarative wild creature, spirit/anomaly, or hazard entity inhabiting specific district features.
type EncounterSpec struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	NodeFeatures      []string `json:"node_features"`
	BaseActivity      int      `json:"base_activity"`
	GrowthPerDay      int      `json:"growth_per_day"`
	SurgeThreshold    int      `json:"surge_threshold"`
	HazardPermille    int      `json:"hazard_permille"`
	Damage            int      `json:"damage"`
	LethalityPermille int      `json:"lethality_permille,omitempty"`
	RareDropItem      string   `json:"rare_drop_item,omitempty"`
	RareDropPermille  int      `json:"rare_drop_permille,omitempty"`
	SuppressorTags    []string `json:"suppressor_tags,omitempty"`
	BountyCoins       int      `json:"bounty_coins,omitempty"`
}

// DisasterSpec defines a regional natural hazard, its weather/seasonal triggers, and engineering mitigations.
type DisasterSpec struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	TriggerWeathers     []string       `json:"trigger_weathers,omitempty"`
	TriggerSeasons      []string       `json:"trigger_seasons,omitempty"`
	TargetFeatures      []string       `json:"target_features,omitempty"`
	ProbPermillePerDay  int            `json:"prob_permille_per_day"`
	DurationTicks       int64          `json:"duration_ticks"`
	ResourceDamagePct   map[string]int `json:"resource_damage_pct,omitempty"`
	HouseDamage         int            `json:"house_damage,omitempty"`
	SanitationDrop      int            `json:"sanitation_drop,omitempty"`
	CasualtyPermille    int            `json:"casualty_permille,omitempty"`
	MitigatedByInfra    string         `json:"mitigated_by_infra,omitempty"`
	ReliefItem          string         `json:"relief_item,omitempty"`
}

// AccidentSpec defines a declarative workplace or transit accident and its tool/infrastructure safety mitigations.
type AccidentSpec struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	NodeFeatures      []string `json:"node_features,omitempty"`
	Recipes           []string `json:"recipes,omitempty"`
	RouteMediums      []string `json:"route_mediums,omitempty"`
	BasePermille      int      `json:"base_permille"`
	SafetyInfra       string   `json:"safety_infra,omitempty"`
	Damage            int      `json:"damage"`
	LethalityPermille int      `json:"lethality_permille,omitempty"`
}

// InfrastructureSpec defines buildable/maintainable public works in a district (e.g. levee, well, mine shoring, public latrine, granary).
type InfrastructureSpec struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	NodeFeature    string         `json:"node_feature,omitempty"`
	NodeFeatures   []string       `json:"node_features,omitempty"`
	SanitationRole bool           `json:"sanitation_role,omitempty"`
	MaxCondition   int            `json:"max_condition"`
	DecayPerDay    int            `json:"decay_per_day"`
	BuildInputs    map[string]int `json:"build_inputs,omitempty"`
	BuildEnergy    int            `json:"build_energy"`
}

// RitualSpec defines a seasonal or crisis ceremony with configurable social morale and optional anomaly/disaster appeasement weights.
type RitualSpec struct {
	ID                      string         `json:"id"`
	Name                    string         `json:"name"`
	NodeFeature             string         `json:"node_feature"`
	RequiredTag             string         `json:"required_tag,omitempty"`
	ScheduledSeason         string         `json:"scheduled_season,omitempty"`
	CrisisTrigger           bool           `json:"crisis_trigger,omitempty"`
	Inputs                  map[string]int `json:"inputs,omitempty"`
	MoneyCost               int            `json:"money_cost,omitempty"`
	EnergyCost              int            `json:"energy_cost"`
	Ticks                   int            `json:"ticks"`
	MoraleBoost             int            `json:"morale_boost,omitempty"`
	TrustBoost              int            `json:"trust_boost,omitempty"`
	DisasterReliefWeightPct int            `json:"disaster_relief_weight_pct,omitempty"`
	ThreatAppeaseAmount     int            `json:"threat_appease_amount,omitempty"`
}

// DiseaseSpec defines a contagious or environmental illness driven by sanitation, water quality, or cold exposure.
type DiseaseSpec struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	MaxSanitationTrigger int    `json:"max_sanitation_trigger,omitempty"`
	ColdTrigger          bool   `json:"cold_trigger,omitempty"`
	ContagionPermille    int    `json:"contagion_permille"`
	DurationTicks        int    `json:"duration_ticks"`
	HealthDrainPerDay    int    `json:"health_drain_per_day"`
	LethalityPermille    int    `json:"lethality_permille"`
}

// LifecycleSpec defines age brackets, fertility, aging, natural elder mortality, and successor hydration rules.
type LifecycleSpec struct {
	ChildMaxAge               int    `json:"child_max_age"`
	ElderMinAge               int    `json:"elder_min_age"`
	MaxExpectancy             int    `json:"max_expectancy"`
	FertilityMinFoodPerCapita int    `json:"fertility_min_food_per_capita"`
	AnnualBirthPermille       int    `json:"annual_birth_permille"`
	ElderAnnualDeathPermille  int    `json:"elder_annual_death_permille"`
	CompostItem               string `json:"compost_item,omitempty"`
	AutoHydrateSuccessor      bool   `json:"auto_hydrate_successor,omitempty"`
}

// EcologySpec configures initial ecological state for an external WorldPack.
type EcologySpec struct {
	Enabled        bool                     `json:"enabled,omitempty"`
	ScalePop       int                      `json:"scale_pop,omitempty"`
	DistrictStates map[string]DistrictState `json:"district_states,omitempty"`
}

// ActiveDisaster tracks an ongoing regional disaster in a district until ExpiresTick.
type ActiveDisaster struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StartedTick int64  `json:"started_tick"`
	ExpiresTick int64  `json:"expires_tick"`
}

// DistrictState holds live ecological, weather, sanitation, infrastructure, threat, and disaster state for one district.
type DistrictState struct {
	DistrictID      string           `json:"district_id"`
	Weather         string           `json:"weather"`
	Sanitation      int              `json:"sanitation"`
	NightsoilPool   int64            `json:"nightsoil_pool,omitempty"`
	MoraleBonus      int              `json:"morale_bonus,omitempty"`
	BountyPool       int              `json:"bounty_pool,omitempty"`
	Corruption       int              `json:"corruption,omitempty"`
	RacketPressure   int              `json:"racket_pressure,omitempty"`
	BribeShieldTicks int              `json:"bribe_shield_ticks,omitempty"`
	LastRitualSeason string           `json:"last_ritual_season,omitempty"`
	Resources       map[string]int64 `json:"resources,omitempty"`
	MaxResources    map[string]int64 `json:"max_resources,omitempty"`
	Infrastructure  map[string]int   `json:"infrastructure,omitempty"`
	ThreatActivity  map[string]int   `json:"threat_activity,omitempty"`
	ActiveDisasters []ActiveDisaster `json:"active_disasters,omitempty"`
}

func CloneDistrictState(ds DistrictState) DistrictState {
	out := ds
	if ds.Resources != nil {
		out.Resources = make(map[string]int64, len(ds.Resources))
		for k, v := range ds.Resources {
			out.Resources[k] = v
		}
	}
	if ds.MaxResources != nil {
		out.MaxResources = make(map[string]int64, len(ds.MaxResources))
		for k, v := range ds.MaxResources {
			out.MaxResources[k] = v
		}
	}
	if ds.Infrastructure != nil {
		out.Infrastructure = cloneIntMap(ds.Infrastructure)
	}
	if ds.ThreatActivity != nil {
		out.ThreatActivity = cloneIntMap(ds.ThreatActivity)
	}
	if ds.ActiveDisasters != nil {
		out.ActiveDisasters = append([]ActiveDisaster(nil), ds.ActiveDisasters...)
	}
	return out
}

func worldHash(worldID string, tick int64, salt string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(worldID))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(tick))
	h.Write(buf[:])
	h.Write([]byte(salt))
	return h.Sum64()
}

func DefaultCalendarSpec() CalendarSpec {
	return CalendarSpec{
		DaysPerSeason: 7, // 4 seasons * 7 days = 28 days per condensed year
		Seasons: []SeasonSpec{
			{
				ID:   "spring",
				Name: "春季",
				FeatureYieldPct: map[string]int{
					"farmland": 115, "herb_patch": 120, "pasture": 120, "fishery_waters": 110, "woodland": 105,
				},
				WeatherWeights:   map[string]int{"clear": 45, "rain": 35, "dense_fog": 12, "storm": 8},
				CanalCapacityPct: 100,
			},
			{
				ID:   "summer",
				Name: "夏季",
				FeatureYieldPct: map[string]int{
					"farmland": 110, "fishery_waters": 125, "woodland": 115, "pasture": 110,
				},
				WeatherWeights:   map[string]int{"clear": 38, "rain": 28, "storm": 18, "drought_heat": 16},
				CanalCapacityPct: 110,
			},
			{
				ID:   "autumn",
				Name: "秋季",
				FeatureYieldPct: map[string]int{
					"farmland": 130, "hunting_ground": 125, "forage_ground": 120, "pasture": 105,
				},
				WeatherWeights:   map[string]int{"clear": 55, "rain": 20, "dense_fog": 15, "storm": 10},
				CanalCapacityPct: 100,
			},
			{
				ID:   "winter",
				Name: "冬季",
				FeatureYieldPct: map[string]int{
					"farmland": 55, "pasture": 65, "herb_patch": 60, "fishery_waters": 75, "hunting_ground": 110, "mine_shaft": 100,
				},
				WeatherWeights:   map[string]int{"clear": 35, "frost_snow": 45, "dense_fog": 12, "storm": 8},
				ColdExposure:     2,
				CanalCapacityPct: 55,
			},
		},
	}
}

func DefaultWeatherSpecs() map[string]WeatherSpec {
	return map[string]WeatherSpec{
		"clear": {
			ID:                    "clear",
			Name:                  "晴朗",
			AccidentMultiplierPct: 100,
		},
		"rain": {
			ID:                    "rain",
			Name:                  "甘霖细雨",
			TravelEnergyPenalty:   1,
			RouteHazardPermille:   map[string]int{"mountain_pass": 8, "waterway": 5, "land": 2},
			FeatureYieldPct:       map[string]int{"farmland": 115, "herb_patch": 115, "pasture": 110},
			ResourceRegenPct:      map[string]int{"soil_fertility": 125, "water_table": 130, "pasture_grass": 120, "herb_bed": 125},
			AccidentMultiplierPct: 120,
		},
		"storm": {
			ID:                    "storm",
			Name:                  "狂风暴雨",
			TravelTickPenalty:     1,
			TravelEnergyPenalty:   2,
			RouteHazardPermille:   map[string]int{"waterway": 45, "mountain_pass": 35, "land": 10},
			RouteCapacityPct:      map[string]int{"waterway": 65, "mountain_pass": 60, "land": 80},
			FeatureYieldPct:       map[string]int{"farmland": 80, "fishery_waters": 65, "woodland": 75, "mine_shaft": 75},
			ResourceRegenPct:      map[string]int{"water_table": 140},
			AccidentMultiplierPct: 185,
		},
		"drought_heat": {
			ID:                    "drought_heat",
			Name:                  "酷暑伏旱",
			TravelEnergyPenalty:   2,
			RouteCapacityPct:      map[string]int{"waterway": 60, "land": 95},
			FeatureYieldPct:       map[string]int{"farmland": 65, "pasture": 70, "fishery_waters": 80},
			ResourceRegenPct:      map[string]int{"soil_fertility": 55, "water_table": 50, "pasture_grass": 60},
			AccidentMultiplierPct: 125,
		},
		"frost_snow": {
			ID:                    "frost_snow",
			Name:                  "霜雪严寒",
			TravelTickPenalty:     1,
			TravelEnergyPenalty:   2,
			RouteHazardPermille:   map[string]int{"mountain_pass": 30, "waterway": 20, "land": 12},
			RouteCapacityPct:      map[string]int{"waterway": 50, "mountain_pass": 55, "land": 80},
			FeatureYieldPct:       map[string]int{"farmland": 50, "pasture": 60, "herb_patch": 55},
			AccidentMultiplierPct: 150,
			ColdDamage:            3,
		},
		"dense_fog": {
			ID:                    "dense_fog",
			Name:                  "江山浓雾",
			TravelTickPenalty:     1,
			RouteHazardPermille:   map[string]int{"waterway": 22, "mountain_pass": 18, "land": 5},
			RouteCapacityPct:      map[string]int{"waterway": 80, "mountain_pass": 80},
			AccidentMultiplierPct: 130,
		},
	}
}

func DefaultResourceSpecs() map[string]ResourcePoolSpec {
	return map[string]ResourcePoolSpec{
		"soil_fertility": {
			ID: "soil_fertility", Name: "水田地力", NodeFeatures: []string{"farmland"}, RegenPermillePerDay: 90,
			SeasonalRegenPct: map[string]int{"spring": 130, "summer": 115, "autumn": 100, "winter": 70}, DepletionFloorPct: 35,
		},
		"pasture_grass": {
			ID: "pasture_grass", Name: "牧场水草", NodeFeatures: []string{"pasture"}, RegenPermillePerDay: 110,
			SeasonalRegenPct: map[string]int{"spring": 140, "summer": 125, "autumn": 95, "winter": 50}, DepletionFloorPct: 30,
		},
		"fish_stock": {
			ID: "fish_stock", Name: "江河鱼汛", NodeFeatures: []string{"fishery_waters"}, RegenPermillePerDay: 105,
			SeasonalRegenPct: map[string]int{"spring": 120, "summer": 135, "autumn": 110, "winter": 65}, DepletionFloorPct: 30,
		},
		"timber_stand": {
			ID: "timber_stand", Name: "山林蓄木", NodeFeatures: []string{"woodland"}, RegenPermillePerDay: 75,
			SeasonalRegenPct: map[string]int{"spring": 125, "summer": 120, "autumn": 90, "winter": 55}, DepletionFloorPct: 35,
		},
		"ore_vein": {
			ID: "ore_vein", Name: "矿脉储量", NodeFeatures: []string{"mine_shaft"}, RegenPermillePerDay: 55,
			SeasonalRegenPct: map[string]int{"spring": 100, "summer": 100, "autumn": 100, "winter": 100}, DepletionFloorPct: 40,
		},
		"game_herd": {
			ID: "game_herd", Name: "山野兽群", NodeFeatures: []string{"hunting_ground"}, RegenPermillePerDay: 95,
			SeasonalRegenPct: map[string]int{"spring": 125, "summer": 115, "autumn": 120, "winter": 75}, DepletionFloorPct: 25,
		},
		"herb_bed": {
			ID: "herb_bed", Name: "灵药山苗", NodeFeatures: []string{"herb_patch"}, RegenPermillePerDay: 95,
			SeasonalRegenPct: map[string]int{"spring": 135, "summer": 120, "autumn": 100, "winter": 50}, DepletionFloorPct: 30,
		},
		"water_table": {
			ID: "water_table", Name: "河井水源", NodeFeatures: []string{"water_source", "canal_wharf"}, PotableWater: true, RegenPermillePerDay: 160,
			SeasonalRegenPct: map[string]int{"spring": 120, "summer": 125, "autumn": 100, "winter": 85}, DepletionFloorPct: 40,
		},
	}
}

func DefaultEncounterSpecs() []EncounterSpec {
	return []EncounterSpec{
		{
			ID: "mountain_beast", Name: "深山猛虎豺狼", Category: "beast",
			NodeFeatures: []string{"hunting_ground", "woodland"},
			BaseActivity: 24, GrowthPerDay: 8, SurgeThreshold: 56,
			HazardPermille: 18, Damage: 18, LethalityPermille: 55,
			RareDropItem: "hide", RareDropPermille: 420,
			SuppressorTags: []string{"hunter", "martial", "law"}, BountyCoins: 8,
		},
		{
			ID: "mountain_anomaly", Name: "苍山精怪木魅", Category: "anomaly",
			NodeFeatures: []string{"mine_shaft", "herb_patch"},
			BaseActivity: 18, GrowthPerDay: 7, SurgeThreshold: 52,
			HazardPermille: 14, Damage: 20, LethalityPermille: 50,
			RareDropItem: "rare_herb", RareDropPermille: 480,
			SuppressorTags: []string{"hunter", "martial", "ritual"}, BountyCoins: 12,
		},
		{
			ID: "river_fiend", Name: "大河水怪蛟鼍", Category: "anomaly",
			NodeFeatures: []string{"fishery_waters", "canal_wharf"},
			BaseActivity: 20, GrowthPerDay: 7, SurgeThreshold: 54,
			HazardPermille: 15, Damage: 18, LethalityPermille: 50,
			RareDropItem: "rare_herb", RareDropPermille: 350,
			SuppressorTags: []string{"hunter", "law", "martial"}, BountyCoins: 10,
		},
	}
}

func DefaultDisasterSpecs() []DisasterSpec {
	return []DisasterSpec{
		{
			ID: "river_flood", Name: "河决洪涝",
			TriggerWeathers: []string{"storm", "rain"}, TriggerSeasons: []string{"summer", "spring"},
			TargetFeatures: []string{"farmland", "canal_wharf", "fishery_waters"},
			ProbPermillePerDay: 95, DurationTicks: 144,
			ResourceDamagePct: map[string]int{"soil_fertility": 15}, HouseDamage: 6, SanitationDrop: 24,
			CasualtyPermille: 2, MitigatedByInfra: "levee", ReliefItem: "food",
		},
		{
			ID: "drought", Name: "亢旱缺水",
			TriggerWeathers: []string{"drought_heat"}, TriggerSeasons: []string{"summer", "autumn"},
			TargetFeatures: []string{"farmland", "pasture"},
			ProbPermillePerDay: 110, DurationTicks: 192,
			ResourceDamagePct: map[string]int{"soil_fertility": 18, "water_table": 28, "pasture_grass": 20},
			SanitationDrop: 10, CasualtyPermille: 2, MitigatedByInfra: "irrigation_well", ReliefItem: "water",
		},
		{
			ID: "locust_swarm", Name: "飞蝗蔽日",
			TriggerWeathers: []string{"drought_heat", "clear"}, TriggerSeasons: []string{"summer", "autumn"},
			TargetFeatures: []string{"farmland", "pasture"},
			ProbPermillePerDay: 55, DurationTicks: 96,
			ResourceDamagePct: map[string]int{"soil_fertility": 20, "pasture_grass": 18},
			CasualtyPermille: 1, MitigatedByInfra: "granary", ReliefItem: "food",
		},
		{
			ID: "wildfire", Name: "山林大火",
			TriggerWeathers: []string{"drought_heat", "clear"}, TriggerSeasons: []string{"autumn", "summer"},
			TargetFeatures: []string{"woodland", "hunting_ground"},
			ProbPermillePerDay: 45, DurationTicks: 96,
			ResourceDamagePct: map[string]int{"timber_stand": 22, "herb_bed": 20}, HouseDamage: 5,
			CasualtyPermille: 2, MitigatedByInfra: "irrigation_well",
		},
		{
			ID: "blizzard_cold", Name: "严冬暴雪寒潮",
			TriggerWeathers: []string{"frost_snow"}, TriggerSeasons: []string{"winter"},
			TargetFeatures: []string{"residential", "pasture", "farmland"},
			ProbPermillePerDay: 120, DurationTicks: 144,
			ResourceDamagePct: map[string]int{"pasture_grass": 15}, HouseDamage: 4,
			CasualtyPermille: 2, MitigatedByInfra: "granary", ReliefItem: "winter_coat",
		},
	}
}

func DefaultAccidentSpecs() []AccidentSpec {
	return []AccidentSpec{
		{
			ID: "mine_collapse", Name: "矿井塌方透水",
			NodeFeatures: []string{"mine_shaft"}, Recipes: []string{"mine_ore", "deep_shaft_ore"},
			BasePermille: 16, SafetyInfra: "mine_shoring", Damage: 24, LethalityPermille: 60,
		},
		{
			ID: "logging_rockfall", Name: "伐木滚石工伤",
			NodeFeatures: []string{"woodland"}, Recipes: []string{"fell_timber"},
			BasePermille: 12, Damage: 18, LethalityPermille: 35,
		},
		{
			ID: "canal_shipwreck", Name: "漕船风浪覆没",
			RouteMediums: []string{"waterway"}, NodeFeatures: []string{"fishery_waters", "canal_wharf"},
			Recipes: []string{"net_fish"},
			BasePermille: 12, SafetyInfra: "levee", Damage: 20, LethalityPermille: 50,
		},
		{
			ID: "workshop_fire", Name: "炉坊走水烫伤",
			Recipes: []string{"forge_tools", "forge_fine_tools", "temper_weapon"},
			BasePermille: 8, Damage: 15, LethalityPermille: 20,
		},
	}
}

func DefaultInfrastructureSpecs() map[string]InfrastructureSpec {
	return map[string]InfrastructureSpec{
		"levee": {
			ID: "levee", Name: "防洪河堤漕闸", NodeFeature: "canal_wharf", NodeFeatures: []string{"canal_wharf", "farmland"},
			MaxCondition: 100, DecayPerDay: 3, BuildInputs: map[string]int{"plank": 1, "timber": 1}, BuildEnergy: 10,
		},
		"irrigation_well": {
			ID: "irrigation_well", Name: "抗旱深井水渠", NodeFeature: "farmland", NodeFeatures: []string{"farmland", "pasture"},
			MaxCondition: 100, DecayPerDay: 2, BuildInputs: map[string]int{"plank": 1}, BuildEnergy: 8,
		},
		"mine_shoring": {
			ID: "mine_shoring", Name: "矿道木石支护", NodeFeature: "mine_shaft", NodeFeatures: []string{"mine_shaft"},
			MaxCondition: 100, DecayPerDay: 4, BuildInputs: map[string]int{"timber": 2}, BuildEnergy: 10,
		},
		"public_latrine": {
			ID: "public_latrine", Name: "坊市公厕积肥池", NodeFeature: "residential", NodeFeatures: []string{"residential", "market"}, SanitationRole: true,
			MaxCondition: 100, DecayPerDay: 3, BuildInputs: map[string]int{"plank": 1}, BuildEnergy: 7,
		},
		"granary": {
			ID: "granary", Name: "常平义仓", NodeFeature: "shrine", NodeFeatures: []string{"shrine", "hub"},
			MaxCondition: 100, DecayPerDay: 2, BuildInputs: map[string]int{"plank": 2}, BuildEnergy: 9,
		},
	}
}

func DefaultRitualSpecs() []RitualSpec {
	return []RitualSpec{
		{
			ID: "spring_plow_rite", Name: "春祈社稷劝农祭", NodeFeature: "shrine", RequiredTag: "ritual",
			ScheduledSeason: "spring", Inputs: map[string]int{"sacrificial_offering": 1}, EnergyCost: 8, Ticks: 3,
			MoraleBoost: 12, TrustBoost: 1, DisasterReliefWeightPct: 25, ThreatAppeaseAmount: 8,
		},
		{
			ID: "summer_river_rite", Name: "夏仲安澜镇水祭", NodeFeature: "shrine", RequiredTag: "ritual",
			ScheduledSeason: "summer", Inputs: map[string]int{"sacrificial_offering": 1}, EnergyCost: 8, Ticks: 3,
			MoraleBoost: 12, TrustBoost: 1, DisasterReliefWeightPct: 30, ThreatAppeaseAmount: 14,
		},
		{
			ID: "autumn_harvest_rite", Name: "秋报万宝丰年祭", NodeFeature: "shrine", RequiredTag: "ritual",
			ScheduledSeason: "autumn", Inputs: map[string]int{"sacrificial_offering": 1}, EnergyCost: 8, Ticks: 3,
			MoraleBoost: 15, TrustBoost: 2, DisasterReliefWeightPct: 25, ThreatAppeaseAmount: 10,
		},
		{
			ID: "winter_hearth_rite", Name: "冬腊祀祖迎岁祭", NodeFeature: "shrine", RequiredTag: "ritual",
			ScheduledSeason: "winter", Inputs: map[string]int{"sacrificial_offering": 1}, EnergyCost: 8, Ticks: 3,
			MoraleBoost: 15, TrustBoost: 2, DisasterReliefWeightPct: 25, ThreatAppeaseAmount: 10,
		},
		{
			ID: "crisis_appeasement_rite", Name: "禳灾祈安太牢大祭", NodeFeature: "shrine", RequiredTag: "ritual",
			CrisisTrigger: true, Inputs: map[string]int{"sacrificial_offering": 1, "choice_grain": 1}, EnergyCost: 10, Ticks: 4,
			MoraleBoost: 18, TrustBoost: 2, DisasterReliefWeightPct: 35, ThreatAppeaseAmount: 18,
		},
	}
}

func DefaultDiseaseSpecs() []DiseaseSpec {
	return []DiseaseSpec{
		{
			ID: "dysentery", Name: "秽水痢疾", MaxSanitationTrigger: 68,
			ContagionPermille: 14, DurationTicks: 192, HealthDrainPerDay: 12, LethalityPermille: 22,
		},
		{
			ID: "cold_fever", Name: "风寒伤寒", ColdTrigger: true,
			ContagionPermille: 12, DurationTicks: 144, HealthDrainPerDay: 10, LethalityPermille: 16,
		},
		{
			ID: "miasma_plague", Name: "灾后时疫", MaxSanitationTrigger: 48,
			ContagionPermille: 22, DurationTicks: 240, HealthDrainPerDay: 18, LethalityPermille: 38,
		},
	}
}

func DefaultLifecycleSpec() LifecycleSpec {
	return LifecycleSpec{
		ChildMaxAge:               15,
		ElderMinAge:               55,
		MaxExpectancy:             78,
		FertilityMinFoodPerCapita: 3,
		AnnualBirthPermille:       22,
		ElderAnnualDeathPermille:  36,
		CompostItem:               "compost",
		AutoHydrateSuccessor:      true,
	}
}

// InitEcology initializes DistrictStates across w.Map scaled to scalePop so weather, carrying capacity,
// encounters, disasters, infrastructure, sanitation, and rituals operate dynamically.
func (w *World) InitEcology(scalePop int) {
	if scalePop < 30 {
		scalePop = 30
	}
	w.DistrictStates = make(map[string]DistrictState, len(w.Map.Districts))
	scaleFactor := int64(scalePop / 30)
	if scaleFactor < 1 {
		scaleFactor = 1
	}

	for _, d := range w.Map.Districts {
		ds := DistrictState{
			DistrictID:     d.ID,
			Weather:        "clear",
			Sanitation:     86,
			Resources:      map[string]int64{},
			MaxResources:   map[string]int64{},
			Infrastructure: map[string]int{},
			ThreatActivity: map[string]int{},
		}
		// Seed explicit capacities from District or derive declaratively from Catalog.Resources NodeFeatures
		for poolID, capVal := range d.ResourceCapacities {
			ds.MaxResources[poolID] = capVal
			ds.Resources[poolID] = capVal
		}
		resKeys := make([]string, 0, len(w.Catalog.Resources))
		for k := range w.Catalog.Resources {
			resKeys = append(resKeys, k)
		}
		sort.Strings(resKeys)
		for _, poolID := range resKeys {
			if ds.MaxResources[poolID] > 0 {
				continue
			}
			spec := w.Catalog.Resources[poolID]
			for _, feat := range spec.NodeFeatures {
				if w.Map.DistrictHasFeature(d.ID, feat) {
					capVal := int64(2500) * scaleFactor
					ds.MaxResources[poolID] = capVal
					ds.Resources[poolID] = capVal
					break
				}
			}
			if spec.PotableWater && ds.MaxResources[poolID] == 0 {
				capVal := int64(800) * scaleFactor
				ds.MaxResources[poolID] = capVal
				ds.Resources[poolID] = capVal
			}
		}
		// Seed district infrastructure declaratively from Catalog.Infrastructures
		infraKeys := make([]string, 0, len(w.Catalog.Infrastructures))
		for k := range w.Catalog.Infrastructures {
			infraKeys = append(infraKeys, k)
		}
		sort.Strings(infraKeys)
		for _, infraID := range infraKeys {
			spec := w.Catalog.Infrastructures[infraID]
			matched := spec.NodeFeature == "" && len(spec.NodeFeatures) == 0
			if spec.NodeFeature != "" && w.Map.DistrictHasFeature(d.ID, spec.NodeFeature) {
				matched = true
			}
			for _, feat := range spec.NodeFeatures {
				if w.Map.DistrictHasFeature(d.ID, feat) {
					matched = true
					break
				}
			}
			if matched {
				ds.Infrastructure[infraID] = (spec.MaxCondition * 85) / 100
			}
		}
		// Seed encounter threat activity for matching features
		for _, enc := range w.Catalog.Encounters {
			matched := false
			for _, feat := range enc.NodeFeatures {
				if w.Map.DistrictHasFeature(d.ID, feat) {
					matched = true
					break
				}
			}
			if matched {
				ds.ThreatActivity[enc.ID] = enc.BaseActivity
			}
		}
		if len(ds.Resources) == 0 {
			ds.Resources = nil
			ds.MaxResources = nil
		}
		if len(ds.Infrastructure) == 0 {
			ds.Infrastructure = nil
		}
		if len(ds.ThreatActivity) == 0 {
			ds.ThreatActivity = nil
		}
		ds.Corruption = 14
		w.DistrictStates[d.ID] = ds
	}
}

func (w *World) districtState(districtID string) (DistrictState, bool) {
	if w.DistrictStates == nil {
		return DistrictState{}, false
	}
	ds, ok := w.DistrictStates[districtID]
	return ds, ok
}

// EcologicalYieldMultiplierPct computes the combined season, weather, disaster, and resource-pool
// carrying-capacity yield multiplier (100 = 1.0x) for recipe r in districtID.
// When w.DistrictStates is nil (legacy small-village mode), it returns 100 so deterministic unit test yields are preserved.
func (w *World) EcologicalYieldMultiplierPct(districtID string, r Recipe) int {
	if w.DistrictStates == nil {
		return 100
	}
	ds, ok := w.DistrictStates[districtID]
	if !ok {
		return 100
	}
	pct := 100
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)
	if cal.SeasonIndex >= 0 && cal.SeasonIndex < len(w.Catalog.Calendar.Seasons) {
		season := w.Catalog.Calendar.Seasons[cal.SeasonIndex]
		if r.NodeFeature != "" {
			if mult := season.FeatureYieldPct[r.NodeFeature]; mult > 0 {
				pct = (pct * mult) / 100
			}
		}
	}
	if ws, ok := w.Catalog.Weathers[ds.Weather]; ok && r.NodeFeature != "" {
		if mult := ws.FeatureYieldPct[r.NodeFeature]; mult > 0 {
			pct = (pct * mult) / 100
		}
	}
	if len(ds.ActiveDisasters) > 0 && r.NodeFeature != "" {
		pct = (pct * 75) / 100
	}
	if r.ResourcePool != "" && ds.MaxResources != nil && ds.MaxResources[r.ResourcePool] > 0 {
		cur := ds.Resources[r.ResourcePool]
		maxCap := ds.MaxResources[r.ResourcePool]
		// Ecological carrying capacity: full yield above 50% capacity; linear scarcity below 50% down to DepletionFloorPct.
		if cur < maxCap/2 {
			floor := w.Catalog.Resources[r.ResourcePool].DepletionFloorPct
			if floor <= 0 {
				floor = 25
			}
			ratioPct := int((cur * 200) / maxCap) // 0..100
			if ratioPct < floor {
				ratioPct = floor
			}
			pct = (pct * ratioPct) / 100
		}
	}
	if ds.MoraleBonus > 0 {
		pct += ds.MoraleBonus / 3
	}
	if pct < 25 {
		pct = 25
	}
	if pct > 180 {
		pct = 180
	}
	return pct
}

// drawDistrictResource deducts resource harvest from the district's finite ecological pool (down to 5% reserve).
func (w *World) drawDistrictResource(districtID, poolID string, amount int64) {
	if w.DistrictStates == nil || poolID == "" || amount <= 0 {
		return
	}
	ds, ok := w.DistrictStates[districtID]
	if !ok || ds.Resources == nil || ds.MaxResources[poolID] <= 0 {
		return
	}
	minReserve := ds.MaxResources[poolID] / 20
	cur := ds.Resources[poolID] - amount
	if cur < minReserve {
		cur = minReserve
	}
	ds.Resources[poolID] = cur
	w.DistrictStates[districtID] = ds
}

// pickSeasonWeather selects the next weather deterministically from seasonal weights.
func (w *World) pickSeasonWeather(districtID string, cal CalendarState) string {
	if len(w.Catalog.Weathers) == 0 {
		return "clear"
	}
	var weights map[string]int
	if cal.SeasonIndex >= 0 && cal.SeasonIndex < len(w.Catalog.Calendar.Seasons) {
		weights = w.Catalog.Calendar.Seasons[cal.SeasonIndex].WeatherWeights
	}
	keys := make([]string, 0, len(w.Catalog.Weathers))
	total := 0
	for k := range w.Catalog.Weathers {
		wt := 10
		if len(weights) > 0 {
			wt = weights[k]
		}
		if wt > 0 {
			keys = append(keys, k)
			total += wt
		}
	}
	if len(keys) == 0 || total <= 0 {
		return "clear"
	}
	sort.Strings(keys)
	roll := int(worldHash(w.ID, w.Tick, "weather:"+districtID) % uint64(total))
	acc := 0
	for _, k := range keys {
		wt := 10
		if len(weights) > 0 {
			wt = weights[k]
		}
		acc += wt
		if roll < acc {
			return k
		}
	}
	return keys[0]
}

// stepWorldEnvironment advances calendar weather, resource regeneration, infrastructure decay/repair,
// natural disasters & granary relief, beast/anomaly encounters & suppression, and seasonal/crisis rituals.
func (w *World) stepWorldEnvironment() []Event {
	if w.DistrictStates == nil {
		return nil
	}
	var events []Event
	clock := ClockAt(w.Tick)
	cal := CalendarAt(w.Tick, w.Catalog.Calendar)

	distIDs := make([]string, 0, len(w.Map.Districts))
	for _, d := range w.Map.Districts {
		distIDs = append(distIDs, d.ID)
		if ds, ok := w.DistrictStates[d.ID]; ok && ds.BribeShieldTicks > 0 {
			ds.BribeShieldTicks--
			w.DistrictStates[d.ID] = ds
		}
	}

	// 1. Weather state machine transition every 12 hours (06:00 and 18:00)
	if (clock.Hour == 6 || clock.Hour == 18) && clock.Minute == 0 && w.Tick > 0 {
		for _, dID := range distIDs {
			ds, ok := w.DistrictStates[dID]
			if !ok {
				continue
			}
			nextW := w.pickSeasonWeather(dID, cal)
			if nextW != ds.Weather {
				oldW := ds.Weather
				ds.Weather = nextW
				w.DistrictStates[dID] = ds
				ws := w.Catalog.Weathers[nextW]
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: "world",
					Kind:    "weather_changed",
					Action:  Action{Kind: "wait", Location: dID},
					Payload: map[string]any{
						"text":             fmt.Sprintf("[%s Y%d] %s weather shifted from %s to %s (%s)", cal.SeasonName, cal.Year, dID, oldW, nextW, ws.Name),
						"district":         dID,
						"previous_weather": oldW,
						"weather":          nextW,
						"season":           cal.SeasonID,
						"year":             cal.Year,
					},
				}
				w.NextID++
				events = append(events, ev)
			}
		}
	}

	// 2. Daily ecological regeneration, infrastructure maintenance, disasters, threats, and rituals (at 07:00)
	if clock.Hour == 7 && clock.Minute == 0 {
		for _, dID := range distIDs {
			ds, ok := w.DistrictStates[dID]
			if !ok {
				continue
			}

			// 2a. Expire finished disasters
			if len(ds.ActiveDisasters) > 0 {
				kept := ds.ActiveDisasters[:0]
				for _, ad := range ds.ActiveDisasters {
					if w.Tick < ad.ExpiresTick {
						kept = append(kept, ad)
					}
				}
				if len(kept) == 0 {
					ds.ActiveDisasters = nil
				} else {
					ds.ActiveDisasters = kept
				}
			}

			// 2b. Regenerate ecological resource pools
			for poolID, maxCap := range ds.MaxResources {
				cur := ds.Resources[poolID]
				if cur >= maxCap {
					continue
				}
				spec := w.Catalog.Resources[poolID]
				regenPermille := spec.RegenPermillePerDay
				if regenPermille <= 0 {
					regenPermille = 50
				}
				if mult := spec.SeasonalRegenPct[cal.SeasonID]; mult > 0 {
					regenPermille = (regenPermille * mult) / 100
				}
				if ws, hasW := w.Catalog.Weathers[ds.Weather]; hasW {
					if mult := ws.ResourceRegenPct[poolID]; mult > 0 {
						regenPermille = (regenPermille * mult) / 100
					}
				}
				delta := (maxCap * int64(regenPermille)) / 1000
				if delta < 1 {
					delta = 1
				}
				cur += delta
				if cur > maxCap {
					cur = maxCap
				}
				ds.Resources[poolID] = cur
			}

			// 2c. Infrastructure natural wear & cohort public-works engineering repair (修堤、凿井、支护矿道、修公厕、修义仓)
			infraKeys := make([]string, 0, len(ds.Infrastructure))
			for infraID := range ds.Infrastructure {
				infraKeys = append(infraKeys, infraID)
			}
			sort.Strings(infraKeys)
			for _, infraID := range infraKeys {
				cond := ds.Infrastructure[infraID]
				spec, exists := w.Catalog.Infrastructures[infraID]
				if !exists {
					continue
				}
				cond -= spec.DecayPerDay
				if cond < 0 {
					cond = 0
				}
				if cond < (spec.MaxCondition*75)/100 {
					if repaired := w.runCohortInfraRepair(dID, infraID, spec, cond); repaired > cond {
						events = append(events, Event{
							ID:      w.NextID,
							Tick:    w.Tick,
							ActorID: "guild_works_" + dID,
							Kind:    "infra_maintained",
							Action:  Action{Kind: "maintain_infra", Location: dID, Item: infraID},
							Payload: map[string]any{
								"text":           fmt.Sprintf("Public works crews in %s reinforced %s (%d -> %d/%d)", dID, spec.Name, cond, repaired, spec.MaxCondition),
								"district":       dID,
								"infrastructure": infraID,
								"condition":      repaired,
							},
						})
						w.NextID++
						cond = repaired
					}
				}
				ds.Infrastructure[infraID] = cond
			}

			// 2d. Decay temporary ritual morale buff slightly each day
			if ds.MoraleBonus > 0 {
				ds.MoraleBonus -= 2
				if ds.MoraleBonus < 0 {
					ds.MoraleBonus = 0
				}
			}

			// 2e. Check regional disaster triggers & apply engineering mitigation + granary relief
			for _, dis := range w.Catalog.Disasters {
				if len(ds.ActiveDisasters) >= 1 {
					break
				}
				if len(dis.TriggerSeasons) > 0 && !ActorHasTag(dis.TriggerSeasons, cal.SeasonID) {
					continue
				}
				if len(dis.TriggerWeathers) > 0 && !ActorHasTag(dis.TriggerWeathers, ds.Weather) {
					continue
				}
				featMatch := len(dis.TargetFeatures) == 0
				for _, tf := range dis.TargetFeatures {
					if w.Map.DistrictHasFeature(dID, tf) {
						featMatch = true
						break
					}
				}
				if !featMatch {
					continue
				}
				prob := dis.ProbPermillePerDay
				infraMitigationPct := 0
				if dis.MitigatedByInfra != "" && ds.Infrastructure != nil {
					infraCond := ds.Infrastructure[dis.MitigatedByInfra]
					infraMitigationPct = (infraCond * 60) / 100 // well-maintained levees/wells cut disaster risk & damage up to 60%!
					prob = (prob * (100 - infraMitigationPct/2)) / 100
				}
				roll := int(worldHash(w.ID, w.Tick, "disaster:"+dID+":"+dis.ID) % 1000)
				if roll >= prob {
					continue
				}
				dur := dis.DurationTicks
				if dur <= 0 {
					dur = 96
				}
				ds.ActiveDisasters = append(ds.ActiveDisasters, ActiveDisaster{
					ID:          dis.ID,
					Name:        dis.Name,
					StartedTick: w.Tick,
					ExpiresTick: w.Tick + dur,
				})
				dmgScalePct := 100 - infraMitigationPct
				if dmgScalePct < 25 {
					dmgScalePct = 25
				}
				for poolID, lossPct := range dis.ResourceDamagePct {
					if ds.Resources != nil && ds.Resources[poolID] > 0 {
						effLoss := (lossPct * dmgScalePct) / 100
						ds.Resources[poolID] -= (ds.Resources[poolID] * int64(effLoss)) / 100
						if ds.Resources[poolID] < 1 {
							ds.Resources[poolID] = 1
						}
					}
				}
				if dis.SanitationDrop > 0 {
					ds.Sanitation -= (dis.SanitationDrop * dmgScalePct) / 100
					if ds.Sanitation < 15 {
						ds.Sanitation = 15
					}
				}
				if dis.HouseDamage > 0 {
					hd := maxInt(1, (dis.HouseDamage*dmgScalePct)/100)
					for hi := range w.Houses {
						if w.Houses[hi].District == dID {
							w.Houses[hi].Condition -= hd
							if w.Houses[hi].Condition < 0 {
								w.Houses[hi].Condition = 0
							}
						}
					}
				}
				wounded, dead, reliefUsed := w.applyDisasterImpactAndRelief(dID, dis, dmgScalePct)
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: "world",
					Kind:    "disaster_struck",
					Action:  Action{Kind: "wait", Location: dID},
					Payload: map[string]any{
						"text":            fmt.Sprintf("%s struck %s (infra mitigation %d%%, relief %d %s distributed; %d injured, %d perished)", dis.Name, dID, infraMitigationPct, reliefUsed, dis.ReliefItem, wounded, dead),
						"district":        dID,
						"disaster_id":     dis.ID,
						"disaster_name":   dis.Name,
						"mitigation_pct":  infraMitigationPct,
						"relief_used":     reliefUsed,
						"wounded":         wounded,
						"deaths":          dead,
						"cause":           "disaster",
					},
				}
				w.NextID++
				witnesses := w.distributeAnomalyObservations(ev, dis.ID, "disaster", w.Map.GeoRegionIDForLocation(dID, ""), dID, maxInt(20, dmgScalePct))
				ev.Payload["witnesses"] = witnesses
				events = append(events, ev)
				for ai := range w.Actors {
					act := &w.Actors[ai]
					if !act.Dead && act.Location == dID && act.Task != nil && act.Task.Dormant {
						events = append(events, w.interruptTaskOnDamage(act, "disaster")...)
					}
				}
			}

			// 2f. Beast & Anomaly ecological growth, surge raids (兽潮/水怪袭扰), official bounties, and hunter/martial suppression
			for _, enc := range w.Catalog.Encounters {
				act, tracked := ds.ThreatActivity[enc.ID]
				if !tracked {
					continue
				}
				growth := enc.GrowthPerDay
				if cal.SeasonIndex >= 0 && cal.SeasonIndex < len(w.Catalog.Calendar.Seasons) && w.Catalog.Calendar.Seasons[cal.SeasonIndex].ColdExposure > 0 && enc.Category == "beast" {
					growth += 3 // hungry creatures descend during harsh/cold seasons
				}
				if len(ds.ActiveDisasters) > 0 && enc.Category == "anomaly" {
					growth += 4 // anomalies stir during floods/droughts
				}
				act += growth
				if act >= enc.SurgeThreshold {
					// Surge raid on district workers/boats + Yamen/Guild bounty posted!
					surgeW, surgeD := w.applyThreatSurge(dID, enc)
					ds.BountyPool += enc.BountyCoins * 10
					ev := Event{
						ID:      w.NextID,
						Tick:    w.Tick,
						ActorID: "world",
						Kind:    "threat_surge",
						Action:  Action{Kind: "attack", Location: dID},
						Payload: map[string]any{
							"text":        fmt.Sprintf("%s surged in %s (activity %d >= %d; %d injured, %d killed; official bounty raised to %d coins)", enc.Name, dID, act, enc.SurgeThreshold, surgeW, surgeD, ds.BountyPool),
							"district":    dID,
							"threat_id":   enc.ID,
							"threat_name": enc.Name,
							"category":    enc.Category,
							"wounded":     surgeW,
							"deaths":      surgeD,
							"bounty_pool": ds.BountyPool,
							"cause":       "beast",
						},
					}
					w.NextID++
					witnesses := w.distributeAnomalyObservations(ev, enc.ID, enc.Category, w.Map.GeoRegionIDForLocation(dID, ""), dID, maxInt(25, act))
					ev.Payload["witnesses"] = witnesses
					events = append(events, ev)
					act = enc.SurgeThreshold - 10
				}
				ds.ThreatActivity[enc.ID] = act
			}

			w.DistrictStates[dID] = ds
		}

		// 2g. Hunter, Martial, and Law-Enforcement Threat Suppression Expeditions (猎户巡山/武馆府衙除害领赏)
		events = append(events, w.stepCohortThreatSuppression()...)

		// 2h. Shrine Seasonal & Crisis Rituals (岁时常祭与禳灾大祭)
		events = append(events, w.stepCohortRituals(cal)...)
	}

	// 3. Daytime mountain/river worker wild encounters & workplace safety accidents (checked at 12:00 noon)
	if clock.Hour == 12 && clock.Minute == 0 {
		events = append(events, w.stepCohortEncountersAndAccidents()...)
	}

	return events
}

func (w *World) runCohortInfraRepair(districtID, infraID string, spec InfrastructureSpec, currentCond int) int {
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.Population <= 0 {
			continue
		}
		producesMaterial := false
		for item := range spec.BuildInputs {
			if w.Catalog.TagsProduce([]string{c.Profession}, item) {
				producesMaterial = true
				break
			}
		}
		if c.District != districtID && !producesMaterial {
			continue
		}
		canPay := true
		for item, qty := range spec.BuildInputs {
			if c.ItemPool[item] < int64(qty)*5 {
				canPay = false
				break
			}
		}
		if !canPay {
			continue
		}
		for item, qty := range spec.BuildInputs {
			c.ItemPool[item] -= int64(qty) * 5
			if c.ItemPool[item] <= 0 {
				delete(c.ItemPool, item)
			}
		}
		next := currentCond + 22
		if next > spec.MaxCondition {
			next = spec.MaxCondition
		}
		return next
	}
	return currentCond
}

func (w *World) applyDisasterImpactAndRelief(districtID string, dis DisasterSpec, dmgScalePct int) (totalWounded, totalDead int, reliefUsed int64) {
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.District != districtID || c.Population <= 0 {
			continue
		}
		// Granary / stockpiled relief item protects residents from disaster casualties!
		reliefMitigation := 100
		if dis.ReliefItem != "" && c.ItemPool[dis.ReliefItem] > 0 {
			need := int64(maxInt(1, c.Population/8))
			use := need
			if use > c.ItemPool[dis.ReliefItem] {
				use = c.ItemPool[dis.ReliefItem]
			}
			c.ItemPool[dis.ReliefItem] -= use
			if c.ItemPool[dis.ReliefItem] <= 0 {
				delete(c.ItemPool, dis.ReliefItem)
			}
			reliefUsed += use
			reliefMitigation = 45
		}
		if dis.CasualtyPermille > 0 {
			numer := int64(c.Population) * int64(dis.CasualtyPermille) * int64(dmgScalePct) * int64(reliefMitigation)
			denom := int64(1000 * 100 * 100)
			rawAffected := int(numer / denom)
			if rem := numer % denom; rem > 0 && int64(worldHash(w.ID, w.Tick, "dis_cas:"+c.ID+":"+dis.ID)%uint64(denom)) < rem {
				rawAffected++
			}
			wReq := maxInt(2, rawAffected*3)
			dReq := (rawAffected + 1) / 2
			if rawAffected > 0 && dReq == 0 {
				dReq = 1
			}
			cw, cd, _ := w.applyCohortCasualtiesWithCause(c, wReq, dReq, 0, "disaster")
			totalWounded += cw
			totalDead += cd
		}
	}
	return totalWounded, totalDead, reliefUsed
}

func (w *World) applyThreatSurge(districtID string, enc EncounterSpec) (wounded, dead int) {
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.District != districtID || c.Population <= 0 {
			continue
		}
		wReq := maxInt(2, c.Population/900)
		dReq := maxInt(1, wReq/8)
		cw, cd, _ := w.applyCohortCasualtiesWithCause(c, wReq, dReq, 0, "beast")
		wounded += cw
		dead += cd
	}
	return wounded, dead
}

func (w *World) stepCohortThreatSuppression() []Event {
	var events []Event
	distIDs := make([]string, 0, len(w.DistrictStates))
	for k := range w.DistrictStates {
		distIDs = append(distIDs, k)
	}
	sort.Strings(distIDs)

	for _, dID := range distIDs {
		ds := w.DistrictStates[dID]
		for _, enc := range w.Catalog.Encounters {
			act := ds.ThreatActivity[enc.ID]
			if act < enc.BaseActivity+10 {
				continue
			}
			// Patrols sweep wilderness corridors periodically rather than every single morning everywhere,
			// allowing seasonal/storm surges to occasionally break through when activity spikes!
			patrolRoll := int(worldHash(w.ID, w.Tick, "sup_roll:"+dID+":"+enc.ID) % 1000)
			if patrolRoll >= 320 && ds.BountyPool == 0 {
				continue
			}
			// Find a suppressor cohort (hunter / master_hunter preferred, or martial/law)
			var hunter *PopulationCohort
			for i := range w.Cohorts {
				c := &w.Cohorts[i]
				if c.EffectivePopulation() < 4 || w.Tick < c.RestUntilTick {
					continue
				}
				prof, ok := w.Catalog.ProfessionByID(c.Profession)
				if !ok {
					continue
				}
				matched := false
				for _, g := range prof.Grants {
					if ActorHasTag(enc.SuppressorTags, g) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
				if c.District == dID {
					hunter = c
					break
				}
				if hunter == nil {
					hunter = c
				}
			}
			if hunter == nil {
				continue
			}
			squad := hunter.EffectivePopulation() / 20
			if squad < 3 {
				squad = 3
			}
			if squad > 80 {
				squad = 80
			}
			suppressed := 9 + squad/10
			if suppressed > 15 {
				suppressed = 15
			}
			if suppressed > act-8 {
				suppressed = act - 8
			}
			if suppressed <= 0 {
				continue
			}
			ds.ThreatActivity[enc.ID] = act - suppressed
			baseBounty := int64(enc.BountyCoins * maxInt(1, squad/6))
			bounty := w.debitTreasury(baseBounty, "bounty")
			if ds.BountyPool > 0 {
				claim := ds.BountyPool
				if claim > int(baseBounty) {
					claim = int(baseBounty)
				}
				paidClaim := w.debitTreasury(int64(claim), "bounty")
				ds.BountyPool -= int(paidClaim)
				bounty += paidClaim
			}
			hunter.MoneyPool += bounty
			var dropQty int64
			if enc.RareDropItem != "" {
				dropQty = int64(maxInt(1, (squad*enc.RareDropPermille)/1000))
				if hunter.ItemPool == nil {
					hunter.ItemPool = map[string]int64{}
				}
				hunter.ItemPool[enc.RareDropItem] += dropQty
			}
			wHunt := maxInt(1, squad/12)
			dNumer := int64(wHunt) * int64(maxInt(25, enc.LethalityPermille))
			dHunt := int(dNumer / 1000)
			if rem := dNumer % 1000; rem > 0 && int64(worldHash(w.ID, w.Tick, "hunt_d:"+hunter.ID+":"+enc.ID)%1000) < rem {
				dHunt++
			}
			hw, hd, _ := w.applyCohortCasualtiesWithCause(hunter, wHunt, dHunt, 0, "beast")
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: hunter.ID,
				Kind:    "cohort_threat_hunted",
				Action:  Action{Kind: "hunt_threat", Location: dID, Item: enc.RareDropItem, Quantity: int(dropQty), Price: int(bounty)},
				Payload: map[string]any{
					"text":            fmt.Sprintf("%s dispatched %d hunters/guards into %s to suppress %s (-%d threat, +%d %s, +%d bounty coins; %d wounded, %d killed)", hunter.Name, squad, dID, enc.Name, suppressed, dropQty, enc.RareDropItem, bounty, hw, hd),
					"cohort_id":       hunter.ID,
					"district":        dID,
					"threat_id":       enc.ID,
					"threat_name":     enc.Name,
					"suppressed":      suppressed,
					"drop_item":       enc.RareDropItem,
					"drop_qty":        dropQty,
					"bounty_coins":    bounty,
					"wounded":         hw,
					"deaths":          hd,
					"cause":           "beast",
				},
			}
			w.NextID++
			events = append(events, ev)
			events = append(events, w.addCohortXP(hunter, 5, 2)...)
		}
		w.DistrictStates[dID] = ds
	}
	return events
}

func (w *World) stepCohortRituals(cal CalendarState) []Event {
	var events []Event
	if len(w.Catalog.Rituals) == 0 {
		return nil
	}
	// Locate ritualist / high_priest cohort (or shrine district cohort)
	var priests *PopulationCohort
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		if c.EffectivePopulation() < 2 {
			continue
		}
		if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok && w.Catalog.Capabilities(Actor{Tags: prof.Grants})["perform_ritual"] {
			priests = c
			break
		}
	}
	if priests == nil {
		return nil
	}

	distIDs := make([]string, 0, len(w.DistrictStates))
	for k := range w.DistrictStates {
		distIDs = append(distIDs, k)
	}
	sort.Strings(distIDs)

	// 1. Seasonal Grand Rites at shrine district (Day 2 seasonal opening & Day 5 mid-season observance)
	shrineDist := priests.District
	shrineState, hasShrine := w.DistrictStates[shrineDist]
	seasonTag := fmt.Sprintf("Y%d-%s", cal.Year, cal.SeasonID)
	dueScheduled := hasShrine && ((cal.DayOfSeason >= 2 && shrineState.LastRitualSeason != seasonTag) || cal.DayOfSeason == 5)
	if dueScheduled {
		for _, rit := range w.Catalog.Rituals {
			if rit.ScheduledSeason != cal.SeasonID {
				continue
			}
			scaleBatches := int64(maxInt(1, priests.EffectivePopulation()/15))
			canAfford := true
			for inItem, inQty := range rit.Inputs {
				if priests.ItemPool[inItem] < int64(inQty)*scaleBatches {
					canAfford = false
					break
				}
			}
			if !canAfford {
				continue
			}
			for inItem, inQty := range rit.Inputs {
				priests.ItemPool[inItem] -= int64(inQty) * scaleBatches
				if priests.ItemPool[inItem] <= 0 {
					delete(priests.ItemPool, inItem)
				}
			}
			shrineState.LastRitualSeason = seasonTag
			w.DistrictStates[shrineDist] = shrineState

			// Apply morale boost across all districts & optional threat appeasement
			for _, dID := range distIDs {
				ds := w.DistrictStates[dID]
				ds.MoraleBonus += rit.MoraleBoost
				if ds.MoraleBonus > 30 {
					ds.MoraleBonus = 30
				}
				if rit.DisasterReliefWeightPct > 0 && rit.ThreatAppeaseAmount > 0 {
					appease := (rit.ThreatAppeaseAmount * rit.DisasterReliefWeightPct) / 100
					for encID, act := range ds.ThreatActivity {
						if act > appease {
							ds.ThreatActivity[encID] = act - appease
						}
					}
				}
				w.DistrictStates[dID] = ds
			}
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: priests.ID,
				Kind:    "cohort_ritual",
				Action:  Action{Kind: "perform_ritual", Location: shrineDist, RecipeID: rit.ID},
				Payload: map[string]any{
					"text":          fmt.Sprintf("%s conducted %s (%s) at %s consuming %d offerings (+%d prefecture morale)", priests.Name, rit.Name, seasonTag, shrineDist, scaleBatches, rit.MoraleBoost),
					"cohort_id":     priests.ID,
					"ritual_id":     rit.ID,
					"ritual_name":   rit.Name,
					"district":      shrineDist,
					"season":        cal.SeasonID,
					"batches":       scaleBatches,
					"morale_boost":  rit.MoraleBoost,
				},
			}
			w.NextID++
			events = append(events, ev)
			events = append(events, w.addCohortXP(priests, 6, 3)...)
			break
		}
	}

	// 2. Crisis Appeasement Rite when any district suffers an active disaster
	for _, dID := range distIDs {
		ds := w.DistrictStates[dID]
		if len(ds.ActiveDisasters) == 0 {
			continue
		}
		for _, rit := range w.Catalog.Rituals {
			if !rit.CrisisTrigger {
				continue
			}
			canAfford := true
			for inItem, inQty := range rit.Inputs {
				if priests.ItemPool[inItem] < int64(inQty)*4 {
					canAfford = false
					break
				}
			}
			if !canAfford {
				continue
			}
			for inItem, inQty := range rit.Inputs {
				priests.ItemPool[inItem] -= int64(inQty) * 4
				if priests.ItemPool[inItem] <= 0 {
					delete(priests.ItemPool, inItem)
				}
			}
			ds.MoraleBonus += rit.MoraleBoost
			if rit.DisasterReliefWeightPct > 0 {
				for ai := range ds.ActiveDisasters {
					rem := ds.ActiveDisasters[ai].ExpiresTick - w.Tick
					cut := (rem * int64(rit.DisasterReliefWeightPct)) / 100
					ds.ActiveDisasters[ai].ExpiresTick -= cut
				}
			}
			w.DistrictStates[dID] = ds
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: priests.ID,
				Kind:    "cohort_ritual",
				Action:  Action{Kind: "perform_ritual", Location: dID, RecipeID: rit.ID},
				Payload: map[string]any{
					"text":         fmt.Sprintf("%s held crisis ceremony %s for disaster-stricken %s (+%d morale, relief weight %d%%)", priests.Name, rit.Name, dID, rit.MoraleBoost, rit.DisasterReliefWeightPct),
					"cohort_id":    priests.ID,
					"ritual_id":    rit.ID,
					"ritual_name":  rit.Name,
					"district":     dID,
					"batches":      int64(4),
					"morale_boost": rit.MoraleBoost,
				},
			}
			w.NextID++
			events = append(events, ev)
			break
		}
	}
	return events
}

// stepCohortEncountersAndAccidents resolves daytime workplace accidents (mining, logging, fishing, forging)
// and wild beast/anomaly encounters during primary resource extraction.
func (w *World) stepCohortEncountersAndAccidents() []Event {
	var events []Event
	for i := range w.Cohorts {
		c := &w.Cohorts[i]
		eff := c.EffectivePopulation()
		if eff < 15 {
			continue
		}
		ds, hasDS := w.DistrictStates[c.District]
		if !hasDS {
			continue
		}
		ws := w.Catalog.Weathers[ds.Weather]
		weatherMult := ws.AccidentMultiplierPct
		if weatherMult <= 0 {
			weatherMult = 100
		}

		// 1. Check workplace accidents matching this cohort's district features or recipes
		for _, acc := range w.Catalog.Accidents {
			matched := false
			for _, recID := range acc.Recipes {
				if r, ok := w.Catalog.RecipeByID(recID); ok && r.Profession == c.Profession {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			prob := (acc.BasePermille * weatherMult) / 100
			// Declarative safety items (e.g. fine_tools, tools, exo_suit) reduce workplace accident rate!
			bestSafetyBonus := 0
			for itemID, itemSpec := range w.Catalog.Items {
				if itemSpec.SafetyBonusPct > bestSafetyBonus && c.ItemPool[itemID] >= int64(maxInt(1, eff/10)) {
					bestSafetyBonus = itemSpec.SafetyBonusPct
				}
			}
			if bestSafetyBonus > 0 {
				prob = (prob * (100 - bestSafetyBonus)) / 100
			}
			if acc.SafetyInfra != "" && ds.Infrastructure != nil {
				cond := ds.Infrastructure[acc.SafetyInfra]
				prob = (prob * (100 - cond/2)) / 100
			}
			if prob < 1 {
				prob = 1
			}
			roll := int(worldHash(w.ID, w.Tick, "acc:"+c.ID+":"+acc.ID) % 1000)
			if roll < prob {
				wReq := maxInt(1, eff/450)
				dNumer := int64(wReq) * int64(acc.LethalityPermille)
				dReq := int(dNumer / 1000)
				if rem := dNumer % 1000; rem > 0 && int64(worldHash(w.ID, w.Tick, "accd:"+c.ID+":"+acc.ID)%1000) < rem {
					dReq++
				}
				cw, cd, _ := w.applyCohortCasualtiesWithCause(c, wReq, dReq, 0, "accident")
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: c.ID,
					Kind:    "workplace_accident",
					Action:  Action{Kind: "craft", Location: c.District},
					Payload: map[string]any{
						"text":          fmt.Sprintf("%s experienced %s in %s under %s weather (%d injured, %d killed)", c.Name, acc.Name, c.District, ds.Weather, cw, cd),
						"cohort_id":     c.ID,
						"district":      c.District,
						"accident_id":   acc.ID,
						"accident_name": acc.Name,
						"weather":       ds.Weather,
						"wounded":       cw,
						"deaths":        cd,
						"cause":         "accident",
					},
				}
				w.NextID++
				events = append(events, ev)
				break
			}
		}

		// 2. Check wild beast/anomaly encounters while working in feature-matching outdoor sites
		for _, enc := range w.Catalog.Encounters {
			act := ds.ThreatActivity[enc.ID]
			if act <= 0 {
				continue
			}
			worksAtSite := false
			if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok {
				for _, g := range prof.Grants {
					if ActorHasTag(enc.SuppressorTags, g) {
						worksAtSite = true
						break
					}
				}
			}
			if !worksAtSite {
				for _, rec := range w.Catalog.Recipes {
					if rec.Profession == c.Profession && rec.NodeFeature != "" && ActorHasTag(enc.NodeFeatures, rec.NodeFeature) {
						worksAtSite = true
						break
					}
				}
			}
			if !worksAtSite {
				continue
			}
			prob := (enc.HazardPermille * act) / 30
			roll := int(worldHash(w.ID, w.Tick, "enc:"+c.ID+":"+enc.ID) % 1000)
			if roll < prob {
				wReq := maxInt(1, eff/500)
				dNumer := int64(wReq) * int64(enc.LethalityPermille)
				dReq := int(dNumer / 1000)
				if rem := dNumer % 1000; rem > 0 && int64(worldHash(w.ID, w.Tick, "encd:"+c.ID+":"+enc.ID)%1000) < rem {
					dReq++
				}
				cw, cd, _ := w.applyCohortCasualtiesWithCause(c, wReq, dReq, 0, "beast")
				var dropQty int64
				if enc.RareDropItem != "" {
					dropQty = int64(maxInt(1, wReq/2))
					if c.ItemPool == nil {
						c.ItemPool = map[string]int64{}
					}
					c.ItemPool[enc.RareDropItem] += dropQty
				}
				ev := Event{
					ID:      w.NextID,
					Tick:    w.Tick,
					ActorID: c.ID,
					Kind:    "wild_encounter",
					Action:  Action{Kind: "craft", Location: c.District, Item: enc.RareDropItem, Quantity: int(dropQty)},
					Payload: map[string]any{
						"text":        fmt.Sprintf("%s encountered %s in %s (%d injured, %d killed; harvested %d %s)", c.Name, enc.Name, c.District, cw, cd, dropQty, enc.RareDropItem),
						"cohort_id":   c.ID,
						"district":    c.District,
						"threat_id":   enc.ID,
						"threat_name": enc.Name,
						"drop_item":   enc.RareDropItem,
						"drop_qty":    dropQty,
						"wounded":     cw,
						"deaths":      cd,
						"cause":       "beast",
					},
				}
				w.NextID++
				events = append(events, ev)
				break
			}
		}
	}
	return events
}

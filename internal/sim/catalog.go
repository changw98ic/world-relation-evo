package sim

import (
	"fmt"
	"sort"
)

// Attributes is the character panel. Professions declare minimum values.
type Attributes struct {
	Strength  int `json:"strength"`
	Dexterity int `json:"dexterity"`
	Insight   int `json:"insight"`
	Spirit    int `json:"spirit"`
}

func (a Attributes) value(name string) (int, bool) {
	switch name {
	case "strength":
		return a.Strength, true
	case "dexterity":
		return a.Dexterity, true
	case "insight":
		return a.Insight, true
	case "spirit":
		return a.Spirit, true
	}
	return 0, false
}

// Meets reports whether every required attribute reaches its minimum.
func (a Attributes) Meets(req map[string]int) (bool, string) {
	if len(req) == 0 {
		return true, ""
	}
	keys := make([]string, 0, len(req))
	for k := range req {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		got, ok := a.value(k)
		if !ok {
			return false, "unknown attribute " + k
		}
		if got < req[k] {
			return false, fmt.Sprintf("requires %s >= %d (has %d)", k, req[k], got)
		}
	}
	return true, ""
}

// EquipSlotSpec declares an equipment slot available to qualifying actors in a world pack.
type EquipSlotSpec struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	GrantedByTags []string `json:"granted_by_tags,omitempty"`
}

// ItemSpec defines generic item properties. Tier is the goods grade (1–5).
type ItemSpec struct {
	MassUnits          int64                  `json:"mass_units,omitempty"`
	NutritionUnits     int64                  `json:"nutrition_units,omitempty"`
	Living             *LivingRule            `json:"living,omitempty"`
	DurabilityBatches  int64                  `json:"durability_batches,omitempty"`
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Tier               int                    `json:"tier"`
	Edible             bool                   `json:"edible,omitempty"`
	Potable            bool                   `json:"potable,omitempty"`
	ThirstRelief       int                    `json:"thirst_relief,omitempty"`
	EnergyRelief       int                    `json:"energy_relief,omitempty"`
	SlotChanges        map[string]int         `json:"slot_changes,omitempty"`
	WarmthBonus        int                    `json:"warmth_bonus,omitempty"`
	MedicinalHeal      int                    `json:"medicinal_heal,omitempty"`
	DiseaseCure        int                    `json:"disease_cure,omitempty"`
	SafetyBonusPct     int                    `json:"safety_bonus_pct,omitempty"`
	CombatBonus        int                    `json:"combat_bonus,omitempty"`
	EquipSlot          string                 `json:"equip_slot,omitempty"`
	EquipRequiredTags  []string               `json:"equip_required_tags,omitempty"`
	EquipMinAttributes map[string]int         `json:"equip_min_attributes,omitempty"`
	EquipRequiredForms []string               `json:"equip_required_forms,omitempty"`
	EquipDefenseByType map[string]DefenseRule `json:"equip_defense_by_type,omitempty"`
	EquipSpeedBonusPct int                    `json:"equip_speed_bonus_pct,omitempty"`
	EquipYieldBonusPct int                    `json:"equip_yield_bonus_pct,omitempty"`
	Tradeable          bool                   `json:"tradeable,omitempty"`
	BatchQty           int                    `json:"batch_qty,omitempty"`
	BatchPrice         int                    `json:"batch_price,omitempty"`
	OfferThreshold     int                    `json:"offer_threshold,omitempty"`
	SurplusThreshold   int                    `json:"surplus_threshold,omitempty"`
	ShortageThreshold  int                    `json:"shortage_threshold,omitempty"`
	AcceptedCurrencies []string               `json:"accepted_currencies,omitempty"`
	BuyerRequiredTag   string                 `json:"buyer_required_tag,omitempty"`
	DualUseCurrency    string                 `json:"dual_use_currency,omitempty"`
	DualUseRate        int                    `json:"dual_use_rate,omitempty"`
}

// Profession is a leveled trade (Tier 1 base or Tier 2+ advanced) with attribute entry requirements,
// optional parent profession prerequisite for career advancement, progression rank titles, and a governing guild.
type Profession struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Tier                int            `json:"tier,omitempty"`
	Guild               string         `json:"guild"`
	ParentProfession    string         `json:"parent_profession,omitempty"`
	MinParentLevel      int            `json:"min_parent_level,omitempty"`
	MinFactionLevel     int            `json:"min_faction_level,omitempty"`
	Requirements        map[string]int `json:"requirements,omitempty"`
	MaxLevel               int            `json:"max_level"`
	YieldEvery             int            `json:"yield_every,omitempty"`
	BaseYieldBonus         int            `json:"base_yield_bonus,omitempty"`
	CombatBonusPerLevel    int            `json:"combat_bonus_per_level,omitempty"`
	StealthBonusPerLevel   int            `json:"stealth_bonus_per_level,omitempty"`
	VigilanceBonusPerLevel int            `json:"vigilance_bonus_per_level,omitempty"`
	ExtortionBonusPerLevel int            `json:"extortion_bonus_per_level,omitempty"`
	TradeMargin            bool           `json:"trade_margin,omitempty"`
	Grants                 []string       `json:"grants,omitempty"`
	Ranks                  []string       `json:"ranks,omitempty"`
}

// RankTitle returns the stage title for a given profession level (e.g., 锻造学徒 -> 初级铁匠 -> 匠师).
func (p Profession) RankTitle(level int) string {
	if len(p.Ranks) == 0 {
		return p.Name
	}
	if level <= 1 {
		return p.Ranks[0]
	}
	idx := level / 2
	if idx >= len(p.Ranks) {
		idx = len(p.Ranks) - 1
	}
	return p.Ranks[idx]
}

// Faction is a village guild with a level driven by member production influence.
type Faction struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	District     string         `json:"district"`
	Professions  []string       `json:"professions"`
	Requirements map[string]int `json:"requirements,omitempty"`
	Members      []string       `json:"members,omitempty"`
	Level        int            `json:"level"`
	Influence    int            `json:"influence"`
}

func (f *Faction) HasMember(id string) bool {
	for _, m := range f.Members {
		if m == id {
			return true
		}
	}
	return false
}

// SkillSpec declaratively defines an actor craft/technique skill completely decoupled from Profession.
// Actors may hold and practice multiple skills across professions and transmit them through mentorship.
type SkillSpec struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Category               string   `json:"category,omitempty"`
	MaxLevel               int      `json:"max_level,omitempty"`
	Ranks                  []string `json:"ranks,omitempty"`
	CombatBonusPerLevel    int      `json:"combat_bonus_per_level,omitempty"`
	StealthBonusPerLevel   int      `json:"stealth_bonus_per_level,omitempty"`
	VigilanceBonusPerLevel int      `json:"vigilance_bonus_per_level,omitempty"`
	HealBonusPerLevel      int      `json:"heal_bonus_per_level,omitempty"`
	RepairBonusPerLevel    int      `json:"repair_bonus_per_level,omitempty"`
	TradeMarginPerLevel    int      `json:"trade_margin_per_level,omitempty"`
	ActionAffinities       []string `json:"action_affinities,omitempty"`
}

// RankTitle returns the stage title for a given skill level (e.g., 初窥门径 -> 登堂入室 -> 炉火纯青 -> 一代宗师).
func (s SkillSpec) RankTitle(level int) string {
	if len(s.Ranks) == 0 {
		return s.Name
	}
	if level <= 1 {
		return s.Ranks[0]
	}
	idx := (level - 1) * len(s.Ranks) / max(1, s.EffectiveMaxLevel())
	if idx >= len(s.Ranks) {
		idx = len(s.Ranks) - 1
	}
	return s.Ranks[idx]
}

// EffectiveMaxLevel returns the skill's configured max level or default 6.
func (s SkillSpec) EffectiveMaxLevel() int {
	if s.MaxLevel > 0 {
		return s.MaxLevel
	}
	return 6
}

// Recipe defines a production action with profession, skill, guild, and location requirements.
type Recipe struct {
	Conversion         *MaterialConversion `json:"conversion,omitempty"`
	Growth             *GrowthRule         `json:"growth,omitempty"`
	ID                 string              `json:"id"`
	Label              string              `json:"label"`
	Profession         string              `json:"profession,omitempty"`
	SkillID            string              `json:"skill_id,omitempty"`
	MinSkillLevel      int                 `json:"min_skill_level,omitempty"`
	RequiredTag        string              `json:"required_tag,omitempty"`
	MinProfessionLevel int                 `json:"min_profession_level,omitempty"`
	MinFactionLevel    int                 `json:"min_faction_level,omitempty"`
	NodeFeature        string              `json:"node_feature,omitempty"`
	HouseFacility      string              `json:"house_facility,omitempty"`
	ResourcePool       string              `json:"resource_pool,omitempty"`
	ResourceDraw       int                 `json:"resource_draw,omitempty"`
	Ticks              int                 `json:"ticks"`
	EnergyCost         int                 `json:"energy_cost"`
	MoneyCost          int                 `json:"money_cost,omitempty"`
	Inputs             map[string]int      `json:"inputs,omitempty"`
	BonusInput         string              `json:"bonus_input,omitempty"`
	BonusOutput        int                 `json:"bonus_output,omitempty"`
	DurableBonus       bool                `json:"durable_bonus,omitempty"`
	OutputItem         string              `json:"output_item"`
	OutputQty          int                 `json:"output_qty"`
}

// TagProfile describes the economic and social role of an identity tag or profession.
type TagProfile struct {
	Produces        []string       `json:"produces,omitempty"`
	Consumes        []string       `json:"consumes,omitempty"`
	TargetStock     map[string]int `json:"target_stock,omitempty"`
	CoercivePower   bool           `json:"coercive_power,omitempty"`
	PublicTrustDuty bool           `json:"public_trust_duty,omitempty"`
}

// Catalog holds all declarative rules for capabilities, items, professions, recipes, ecology, logistics, rituals, and lifecycle.
type Catalog struct {
	HouseWear              *HouseWearRule                `json:"house_wear,omitempty"`
	Production             ProductionSpec                `json:"production"`
	TagActions             map[string][]string           `json:"tag_actions"`
	Items                  map[string]ItemSpec           `json:"items"`
	Professions            []Profession                  `json:"professions"`
	Skills                 map[string]SkillSpec          `json:"skills,omitempty"`
	Recipes                []Recipe                      `json:"recipes"`
	TagProfiles            map[string]TagProfile         `json:"tag_profiles"`
	LevelThresholds        []int                         `json:"level_thresholds"`
	FactionLevelThresholds []int                         `json:"faction_level_thresholds"`
	Calendar               CalendarSpec                  `json:"calendar"`
	Weathers               map[string]WeatherSpec        `json:"weathers,omitempty"`
	Resources              map[string]ResourcePoolSpec   `json:"resources,omitempty"`
	Encounters             []EncounterSpec               `json:"encounters,omitempty"`
	Disasters              []DisasterSpec                `json:"disasters,omitempty"`
	Accidents              []AccidentSpec                `json:"accidents,omitempty"`
	WorldEvents            []WorldEventSpec              `json:"world_events,omitempty"`
	Infrastructures        map[string]InfrastructureSpec `json:"infrastructures,omitempty"`
	Rituals                []RitualSpec                  `json:"rituals,omitempty"`
	Diseases               []DiseaseSpec                 `json:"diseases,omitempty"`
	Lifecycle              LifecycleSpec                 `json:"lifecycle"`
	Governance             GovernanceSpec                `json:"governance,omitempty"`
	LocomotionModes        map[string]LocomotionSpec     `json:"locomotion_modes,omitempty"`
	SensoryModes           map[string]SensorySpec        `json:"sensory_modes,omitempty"`
	CommChannels           map[string]CommChannelSpec    `json:"comm_channels,omitempty"`
	CombatProfiles         []CombatProfileSpec           `json:"combat_profiles,omitempty"`
	ActionSpecs            map[string]ActionSpec         `json:"action_specs,omitempty"`
	EquipSlots             map[string]EquipSlotSpec      `json:"equip_slots,omitempty"`
	StateSlots             map[string]StateSlotSpec      `json:"state_slots,omitempty"`
	Forms                  map[string]FormSpec           `json:"forms,omitempty"`
	Transitions            map[string]TransitionSpec     `json:"transitions,omitempty"`
	Currencies             map[string]CurrencySpec       `json:"currencies,omitempty"`
	Techs                  map[string]TechSpec           `json:"techs,omitempty"`
	DefaultWorldLaws       map[string]int                `json:"default_world_laws,omitempty"`
	Environment            *EnvironmentProfile           `json:"environment,omitempty"`
}

func DefaultCatalog() Catalog {
	catalog := Catalog{
		HouseWear:  func() *HouseWearRule { rule := DefaultHouseWearRule(); return &rule }(),
		Production: DefaultProductionSpec(),
		TagActions: map[string][]string{
			"resident": {"equip", "unequip", "refuel", "harvest", "claim_world_event", "form_party", "join_party", "leave_party", "betray_party", "propose_alliance", "declare_war", "make_peace", "set_ambition", "investigate_actor", "blackmail", "expose_secret", "propose_growth_care", "accept_growth_care", "dispatch_harvest", "start_plan", "abandon_plan", "wait", "move", "move_venue", "enter_house", "leave_house", "sleep", "begin_rest", "end_rest", "eat", "drink", "relieve", "medicate", "speak", "repair_house", "transfer_house", "upgrade_house", "maintain_infra", "join_faction", "join_profession", "marry", "swear_bond", "mediate_feud"},
			"trader":   {"propose_agreement", "accept_agreement", "reject_agreement", "fulfill_agreement", "accept_loan", "reject_loan", "cancel_loan", "offer", "accept", "reject", "borrow_loan", "repay_loan", "foreclose_collateral"},
			"fighter":  {"attack", "avenge_feud", "claim_turf"},
			"healer":   {"heal"},
			"martial":  {"spar", "hunt_threat", "avenge_feud", "claim_turf"},
			"hunter":   {"hunt_threat"},
			"thief":    {"steal", "bribe"},
			"raider":   {"raid", "bribe", "avenge_feud", "claim_turf"},
			"gang":     {"raid", "extort", "bribe", "avenge_feud", "claim_turf"},
			"law":      {"arrest", "hunt_threat", "audit_corruption", "claim_turf", "frame_suspect"},
			"censor":   {"audit_corruption", "file_appeal"},
			"ritual":   {"perform_ritual"},
			"ruler":    {"issue_decree"},
		},
		LevelThresholds:        []int{0, 8, 20, 40, 70, 110},
		FactionLevelThresholds: []int{0, 30, 90, 200, 360},
		Calendar:               DefaultCalendarSpec(),
		Weathers:               DefaultWeatherSpecs(),
		Resources:              DefaultResourceSpecs(),
		Encounters:             DefaultEncounterSpecs(),
		Disasters:              DefaultDisasterSpecs(),
		Accidents:              DefaultAccidentSpecs(),
		Infrastructures:        DefaultInfrastructureSpecs(),
		Rituals:                DefaultRitualSpecs(),
		Diseases:               DefaultDiseaseSpecs(),
		Lifecycle:              DefaultLifecycleSpec(),
		Governance:             DefaultGovernanceSpec(),
		Items: map[string]ItemSpec{
			"nightsoil": {ID: "nightsoil", Name: "collected organic waste", Tier: 1},
			"paper":     {ID: "paper", Name: "paper sheets", Tier: 1, Tradeable: true, BatchQty: 1, BatchPrice: 1},
			"ink":       {ID: "ink", Name: "writing ink", Tier: 1, Tradeable: true, BatchQty: 1, BatchPrice: 1},
			"calf":      {ID: "calf", Name: "cattle calf", Tier: 1, Tradeable: true, BatchQty: 1, BatchPrice: 2, Living: DefaultCattleLivingRule()},

			"food": {ID: "food", Name: "grain rations", Tier: 1, Edible: true, NutritionUnits: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 6, SurplusThreshold: 8, ShortageThreshold: 4},
			"seeds": {ID: "seeds", Name: "wheat seeds", Tier: 1,
				Tradeable: true, BatchQty: 3, BatchPrice: 1, OfferThreshold: 6, SurplusThreshold: 6, ShortageThreshold: 1},
			"water": {ID: "water", Name: "well water", Tier: 1, Potable: true, ThirstRelief: 2,
				Tradeable: true, BatchQty: 2, BatchPrice: 1, OfferThreshold: 6, SurplusThreshold: 5, ShortageThreshold: 1},
			"compost": {ID: "compost", Name: "household compost", Tier: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 1, OfferThreshold: 5, SurplusThreshold: 4, ShortageThreshold: 1},
			"timber": {ID: "timber", Name: "rough timber", Tier: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 4, SurplusThreshold: 5, ShortageThreshold: 2},
			"plank": {ID: "plank", Name: "dressed planks", Tier: 2,
				Tradeable: true, BatchQty: 2, BatchPrice: 3, OfferThreshold: 3, SurplusThreshold: 3, ShortageThreshold: 1},
			"ore": {ID: "ore", Name: "ironstone ore", Tier: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 4, SurplusThreshold: 4, ShortageThreshold: 1},
			"tools": {ID: "tools", Name: "hand tools", Tier: 3, SafetyBonusPct: 20, DurabilityBatches: 24,
				Tradeable: true, BatchQty: 1, BatchPrice: 4, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"herb": {ID: "herb", Name: "mountain herbs", Tier: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 4, SurplusThreshold: 4, ShortageThreshold: 2},
			"medicine": {ID: "medicine", Name: "herbal medicine", Tier: 3, MedicinalHeal: 20, DiseaseCure: 1,
				Tradeable: true, BatchQty: 1, BatchPrice: 3, OfferThreshold: 3, SurplusThreshold: 3, ShortageThreshold: 1},
			"choice_grain": {ID: "choice_grain", Name: "choice grain", Tier: 2, Edible: true, NutritionUnits: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 4, OfferThreshold: 4, SurplusThreshold: 5, ShortageThreshold: 2},
			"fine_tools": {ID: "fine_tools", Name: "fine tool sets", Tier: 4, SafetyBonusPct: 40, CombatBonus: 2, DurabilityBatches: 24,
				Tradeable: true, BatchQty: 1, BatchPrice: 8, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"tonic": {ID: "tonic", Name: "herbal tonics", Tier: 4, MedicinalHeal: 35, DiseaseCure: 2,
				Tradeable: true, BatchQty: 1, BatchPrice: 8, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"scroll": {ID: "scroll", Name: "copied scroll", Tier: 2,
				Tradeable: true, BatchQty: 1, BatchPrice: 3, OfferThreshold: 2, SurplusThreshold: 3, ShortageThreshold: 1},
			"classic_tome": {ID: "classic_tome", Name: "annotated classic", Tier: 4,
				Tradeable: true, BatchQty: 1, BatchPrice: 8, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"weapon": {ID: "weapon", Name: "steel blades", Tier: 3, CombatBonus: 5,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"martial_manual": {ID: "martial_manual", Name: "boxing manual", Tier: 3, CombatBonus: 2,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"bounty_writ": {ID: "bounty_writ", Name: "yamen bounty writ", Tier: 3,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"livestock": {ID: "livestock", Name: "adult cattle", Tier: 2, Living: DefaultCattleLivingRule(),
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 3, SurplusThreshold: 4, ShortageThreshold: 1},
			"meat": {ID: "meat", Name: "cured meat", Tier: 2, Edible: true, NutritionUnits: 2,
				Tradeable: true, BatchQty: 2, BatchPrice: 4, OfferThreshold: 4, SurplusThreshold: 5, ShortageThreshold: 2},
			"fish": {ID: "fish", Name: "river fish", Tier: 1, Edible: true, NutritionUnits: 1,
				Tradeable: true, BatchQty: 2, BatchPrice: 2, OfferThreshold: 5, SurplusThreshold: 6, ShortageThreshold: 2},
			"hide": {ID: "hide", Name: "cured beast hide", Tier: 2,
				Tradeable: true, BatchQty: 2, BatchPrice: 3, OfferThreshold: 3, SurplusThreshold: 4, ShortageThreshold: 1},
			"winter_coat": {ID: "winter_coat", Name: "quilted fur coat", Tier: 3, WarmthBonus: 3,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 2, ShortageThreshold: 1},
			"rare_herb": {ID: "rare_herb", Name: "spirit mountain ginseng", Tier: 3,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 3, ShortageThreshold: 1},
			"sacrificial_offering": {ID: "sacrificial_offering", Name: "ritual grain & incense offering", Tier: 3,
				Tradeable: true, BatchQty: 1, BatchPrice: 5, OfferThreshold: 2, SurplusThreshold: 3, ShortageThreshold: 1},
		},
		Professions: []Profession{
			// Tier 1: Base professions (基础职业) with stage rank progression (职业阶位称号)
			{ID: "farmer", Name: "crop farmer", Tier: 1, Guild: "farmers_guild", Requirements: map[string]int{"strength": 8, "spirit": 6}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"农耕学徒", "初级农夫", "司农把式"}},
			{ID: "gatherer", Name: "forager", Tier: 1, Guild: "farmers_guild", Requirements: map[string]int{"dexterity": 6}, MaxLevel: 6, YieldEvery: 3, Ranks: []string{"采食学徒", "初级采食者", "山野熟手"}},
			{ID: "herder", Name: "牧民", Tier: 1, Guild: "farmers_guild", Requirements: map[string]int{"strength": 7, "spirit": 7}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"放牧学徒", "熟手牧夫", "司牧把式"}},
			{ID: "fisher", Name: "渔民", Tier: 1, Guild: "waterways_guild", Requirements: map[string]int{"strength": 7, "dexterity": 7}, MaxLevel: 6, YieldEvery: 2, Grants: []string{"water_user"}, Ranks: []string{"渔舟学徒", "江河渔夫", "老渔把式"}},
			{ID: "boatman", Name: "漕工船夫", Tier: 1, Guild: "waterways_guild", Requirements: map[string]int{"strength": 8, "spirit": 7}, MaxLevel: 6, YieldEvery: 2, TradeMargin: true, Grants: []string{"water_carrier", "water_user"}, Ranks: []string{"纤夫水手", "漕船艄公", "老舵头"}},
			{ID: "hunter", Name: "猎户", Tier: 1, Guild: "hunters_guild", Requirements: map[string]int{"strength": 8, "dexterity": 8}, MaxLevel: 6, YieldEvery: 2, CombatBonusPerLevel: 2, Grants: []string{"fighter", "hunter"}, Ranks: []string{"巡林学徒", "进山猎手", "熟山老猎"}},
			{ID: "ritualist", Name: "祠祝巫祝", Tier: 1, Guild: "shrine_guild", Requirements: map[string]int{"insight": 8, "spirit": 8}, MaxLevel: 6, YieldEvery: 2, Grants: []string{"ritual"}, Ranks: []string{"洒扫庙祝", "司礼祝官", "奉常大祝"}},
			{ID: "woodcutter", Name: "woodcutter", Tier: 1, Guild: "artisans_guild", Requirements: map[string]int{"strength": 8}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"伐木学徒", "初级樵夫", "林木匠师"}},
			{ID: "carpenter", Name: "carpenter", Tier: 1, Guild: "artisans_guild", Requirements: map[string]int{"dexterity": 8, "insight": 7}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"木工学徒", "初级木匠", "营造匠师"}},
			{ID: "miner", Name: "miner", Tier: 1, Guild: "artisans_guild", Requirements: map[string]int{"strength": 8}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"采矿学徒", "初级矿工", "寻脉匠师"}},
			{ID: "blacksmith", Name: "blacksmith", Tier: 1, Guild: "artisans_guild", Requirements: map[string]int{"strength": 10, "insight": 8}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"锻造学徒", "初级铁匠", "匠师"}},
			{ID: "herbalist", Name: "herbalist", Tier: 1, Guild: "healers_guild", Requirements: map[string]int{"insight": 9, "spirit": 7}, MaxLevel: 6, YieldEvery: 2, Grants: []string{"water_user"}, Ranks: []string{"采药学徒", "初级药师", "司药匠师"}},
			{ID: "healer", Name: "healer", Tier: 1, Guild: "healers_guild", Requirements: map[string]int{"insight": 10, "spirit": 9}, MaxLevel: 6, YieldEvery: 3, Grants: []string{"healer", "water_user"}, Ranks: []string{"医馆学徒", "初级医者", "杏林医师"}},
			{ID: "peddler", Name: "peddler", Tier: 1, Guild: "merchants_guild", Requirements: map[string]int{"insight": 7, "spirit": 8}, MaxLevel: 6, YieldEvery: 3, TradeMargin: true, Grants: []string{"land_carrier"}, Ranks: []string{"商铺学徒", "初级行商", "行商掌柜"}},
			{ID: "guard", Name: "guard", Tier: 1, Guild: "watch_guild", Requirements: map[string]int{"strength": 10, "spirit": 8}, MaxLevel: 6, YieldEvery: 3, CombatBonusPerLevel: 3, Grants: []string{"fighter", "law"}, Ranks: []string{"巡防学徒", "初级武侯", "巡守教头"}},
			{ID: "scholar", Name: "读书人", Tier: 1, Guild: "scholars_guild", Requirements: map[string]int{"insight": 9, "spirit": 7}, MaxLevel: 6, YieldEvery: 2, Ranks: []string{"蒙学读书人", "通经书生", "宿儒"}},
			{ID: "martial_artist", Name: "武师", Tier: 1, Guild: "martial_guild", Requirements: map[string]int{"strength": 9, "dexterity": 8}, MaxLevel: 6, YieldEvery: 2, CombatBonusPerLevel: 3, Grants: []string{"fighter", "martial"}, Ranks: []string{"武馆学徒", "趟子手", "武馆拳师"}},
			{ID: "constable", Name: "捕快", Tier: 1, Guild: "yamen_guild", Requirements: map[string]int{"strength": 9, "dexterity": 8, "insight": 8}, MaxLevel: 6, YieldEvery: 2, CombatBonusPerLevel: 3, Grants: []string{"fighter", "law"}, Ranks: []string{"白役差人", "府衙捕快", "刑名缉捕"}},
			{ID: "bandit", Name: "山贼", Tier: 1, Guild: "bandits_guild", Requirements: map[string]int{"strength": 9, "dexterity": 7}, MaxLevel: 6, YieldEvery: 2, CombatBonusPerLevel: 3, Grants: []string{"fighter", "outlaw", "raider"}, Ranks: []string{"落草喽啰", "山寨悍匪", "巡山头目"}},
			{ID: "thief", Name: "小偷", Tier: 1, Guild: "gang_guild", Requirements: map[string]int{"dexterity": 9, "insight": 7}, MaxLevel: 6, YieldEvery: 2, Grants: []string{"thief", "outlaw"}, Ranks: []string{"摸包小贼", "穿窬惯窃", "妙手空空"}},
			{ID: "gangster", Name: "帮众", Tier: 1, Guild: "gang_guild", Requirements: map[string]int{"strength": 8, "spirit": 8}, MaxLevel: 6, YieldEvery: 2, CombatBonusPerLevel: 3, Grants: []string{"fighter", "outlaw", "gang"}, Ranks: []string{"帮派外围", "红棍帮众", "堂口香主"}},
			// Tier 2: Advanced professions (高级进阶职业) unlocked by parent profession mastery
			{ID: "xiucai", Name: "秀才", Tier: 2, Guild: "scholars_guild", ParentProfession: "scholar", MinParentLevel: 2, Requirements: map[string]int{"insight": 11, "spirit": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Grants: []string{"censor"}, Ranks: []string{"新科秀才", "凛膳秀才", "贡生学士"}},
			{ID: "master_smith", Name: "匠师", Tier: 2, Guild: "artisans_guild", ParentProfession: "blacksmith", MinParentLevel: 2, Requirements: map[string]int{"strength": 11, "insight": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Ranks: []string{"初阶匠师", "精锻匠师", "大匠师"}},
			{ID: "master_carpenter", Name: "大木匠师", Tier: 2, Guild: "artisans_guild", ParentProfession: "carpenter", MinParentLevel: 2, Requirements: map[string]int{"dexterity": 10, "insight": 8}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Ranks: []string{"司木匠师", "营造大匠", "都料匠师"}},
			{ID: "agronomist", Name: "农师", Tier: 2, Guild: "farmers_guild", ParentProfession: "farmer", MinParentLevel: 2, Requirements: map[string]int{"strength": 10, "insight": 8}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Ranks: []string{"初阶农师", "司稼农师", "大司农"}},
			{ID: "ranch_master", Name: "牧正", Tier: 2, Guild: "farmers_guild", ParentProfession: "herder", MinParentLevel: 2, Requirements: map[string]int{"strength": 9, "spirit": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Ranks: []string{"苑囿副正", "北牧监正", "太仆牧师"}},
			{ID: "fleet_master", Name: "渔帮总舵", Tier: 2, Guild: "waterways_guild", ParentProfession: "fisher", MinParentLevel: 2, Requirements: map[string]int{"strength": 9, "dexterity": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Grants: []string{"water_user"}, Ranks: []string{"渔队把头", "江防渔总", "水泊总舵"}},
			{ID: "canal_master", Name: "漕帮总舵", Tier: 2, Guild: "waterways_guild", ParentProfession: "boatman", MinParentLevel: 2, Requirements: map[string]int{"strength": 9, "insight": 8, "spirit": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, TradeMargin: true, Grants: []string{"water_carrier", "water_user"}, Ranks: []string{"运河押运官", "漕帮香主", "通济总漕"}},
			{ID: "master_hunter", Name: "猎王山神卫", Tier: 2, Guild: "hunters_guild", ParentProfession: "hunter", MinParentLevel: 2, Requirements: map[string]int{"strength": 10, "dexterity": 10}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 2, Grants: []string{"fighter", "hunter"}, Ranks: []string{"巡山总猎", "镇山猎头", "百兽猎王"}},
			{ID: "high_priest", Name: "大司祝", Tier: 2, Guild: "shrine_guild", ParentProfession: "ritualist", MinParentLevel: 2, Requirements: map[string]int{"insight": 10, "spirit": 10}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Grants: []string{"ritual"}, Ranks: []string{"太祝令", "奉常博士", "社稷大宗伯"}},
			{ID: "alchemist", Name: "丹药师", Tier: 2, Guild: "healers_guild", ParentProfession: "herbalist", MinParentLevel: 2, Requirements: map[string]int{"insight": 11, "spirit": 8}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Grants: []string{"water_user"}, Ranks: []string{"司药丹师", "炼药宗师", "国医丹师"}},
			{ID: "merchant_chief", Name: "大掌柜", Tier: 2, Guild: "merchants_guild", ParentProfession: "peddler", MinParentLevel: 2, Requirements: map[string]int{"insight": 9, "spirit": 10}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, TradeMargin: true, Grants: []string{"land_carrier"}, Ranks: []string{"分号掌柜", "大掌柜", "商帮总管"}},
			{ID: "captain", Name: "都头", Tier: 2, Guild: "watch_guild", ParentProfession: "guard", MinParentLevel: 2, Requirements: map[string]int{"strength": 12, "spirit": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 3, Grants: []string{"fighter", "law"}, Ranks: []string{"副都头", "巡防都头", "总巡检"}},
			{ID: "martial_master", Name: "武学宗师", Tier: 2, Guild: "martial_guild", ParentProfession: "martial_artist", MinParentLevel: 2, Requirements: map[string]int{"strength": 11, "dexterity": 9, "spirit": 8}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 3, Grants: []string{"fighter", "martial"}, Ranks: []string{"内家拳师", "武馆总教习", "武学大宗师"}},
			{ID: "chief_constable", Name: "总捕头", Tier: 2, Guild: "yamen_guild", ParentProfession: "constable", MinParentLevel: 2, Requirements: map[string]int{"strength": 11, "dexterity": 9, "insight": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 3, Grants: []string{"fighter", "law", "censor"}, Ranks: []string{"铁血捕头", "六扇门总捕", "提刑按察使"}},
			{ID: "bandit_leader", Name: "山大王", Tier: 2, Guild: "bandits_guild", ParentProfession: "bandit", MinParentLevel: 2, Requirements: map[string]int{"strength": 11, "dexterity": 8, "spirit": 8}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 3, Grants: []string{"fighter", "outlaw", "raider"}, Ranks: []string{"分寨瓢把子", "黑风寨主", "绿林总瓢把子"}},
			{ID: "master_thief", Name: "盗帅", Tier: 2, Guild: "gang_guild", ParentProfession: "thief", MinParentLevel: 2, Requirements: map[string]int{"dexterity": 11, "insight": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, Grants: []string{"thief", "outlaw"}, Ranks: []string{"飞贼头目", "夜行盗圣", "千手盗帅"}},
			{ID: "gang_boss", Name: "帮主", Tier: 2, Guild: "gang_guild", ParentProfession: "gangster", MinParentLevel: 2, Requirements: map[string]int{"strength": 10, "insight": 8, "spirit": 9}, MaxLevel: 6, YieldEvery: 2, BaseYieldBonus: 1, CombatBonusPerLevel: 3, Grants: []string{"fighter", "outlaw", "gang"}, Ranks: []string{"分堂舵主", "双花大红棍", "帮派龙头"}},
		},
		Recipes: []Recipe{
			{ID: "compost_waste", Label: "Mature collected organic waste into compost", Profession: "farmer", MinProfessionLevel: 1, HouseFacility: "compost_pit", Ticks: 2, EnergyCost: 6, Inputs: map[string]int{"nightsoil": 3}, OutputItem: "compost", OutputQty: 1, Growth: &GrowthRule{CareWorkersPerBatch: 1, FailureAfterTicks: 7 * TicksPerDay, DurationTicks: 60 * TicksPerDay, MaintenanceIntervalTicks: TicksPerDay, Supplies: map[string]int{"water": 1}, Source: "UMN Extension: well-managed warm-season compost 2-4 months; preset uses 60 days"}},

			{ID: "make_paper", Label: "Prepare paper from timber pulp", Profession: "scholar", MinProfessionLevel: 1, NodeFeature: "academy", Ticks: 3, EnergyCost: 8, Inputs: map[string]int{"timber": 1, "water": 1}, OutputItem: "paper", OutputQty: 1},
			{ID: "make_ink", Label: "Prepare writing ink from plant pigment", Profession: "scholar", MinProfessionLevel: 1, NodeFeature: "academy", Ticks: 3, EnergyCost: 8, Inputs: map[string]int{"herb": 1, "water": 1}, OutputItem: "ink", OutputQty: 1},
			{ID: "raise_livestock", Label: "Raise juvenile stock to breeding maturity", Profession: "herder", MinProfessionLevel: 1, NodeFeature: "pasture", Ticks: 3, EnergyCost: 11, Inputs: map[string]int{"calf": 1}, Growth: &GrowthRule{CareWorkersPerBatch: 1, FailureAfterTicks: 7 * TicksPerDay, DurationTicks: 450 * TicksPerDay, MaintenanceIntervalTicks: TicksPerDay, Supplies: map[string]int{"water": 1}, ResourcePool: "pasture_grass", ResourceDraw: 2, Source: "Penn State Extension: heifers ready for breeding at 13-15 months; preset uses 450 days"}, OutputItem: "livestock", OutputQty: 1},

			{
				ID: "save_seeds", Label: "Select crop seed from stored grain",
				Profession: "farmer", MinProfessionLevel: 1,
				Ticks: 1, EnergyCost: 2, Inputs: map[string]int{"food": 1},
				OutputItem: "seeds", OutputQty: 1,
			},
			{
				ID: "fetch_water", Label: "Draw 2 buckets of water at the creek well",
				RequiredTag: "water_user", NodeFeature: "water_source",
				ResourcePool: "water_table", ResourceDraw: 1,
				Ticks: 2, EnergyCost: 5,
				OutputItem: "water", OutputQty: 2,
			},
			{
				ID: "make_compost", Label: "Turn 2 compost in the yard compost pit",
				Profession: "farmer", MinProfessionLevel: 1, HouseFacility: "compost_pit",
				Ticks: 2, EnergyCost: 6, Inputs: map[string]int{"food": 2},
				Growth:     &GrowthRule{CareWorkersPerBatch: 1, FailureAfterTicks: 7 * TicksPerDay, DurationTicks: 60 * TicksPerDay, MaintenanceIntervalTicks: TicksPerDay, Supplies: map[string]int{"water": 1}, Source: "UMN Extension: well-managed warm-season compost 2-4 months; preset uses 60 days"},
				OutputItem: "compost", OutputQty: 2,
			},
			{
				ID: "farm_grain", Label: "Irrigate, tend, and harvest grain in the fields",
				Profession: "farmer", MinProfessionLevel: 1, NodeFeature: "farmland",
				ResourcePool: "soil_fertility", ResourceDraw: 2,
				Ticks: 4, EnergyCost: 14,
				Inputs:     map[string]int{"seeds": 1, "water": 1},
				BonusInput: "compost", BonusOutput: 1,
				OutputItem: "food", OutputQty: 1200,
				Growth: &GrowthRule{CareWorkersPerBatch: 1, FailureAfterTicks: 7 * TicksPerDay, DurationTicks: 120 * TicksPerDay, MaintenanceIntervalTicks: TicksPerDay, Supplies: map[string]int{"water": 1}, Source: "FAO crop water needs: wheat 120-150 days; preset uses 120 days"},
			},
			{
				ID: "thresh_choice_grain", Label: "Thresh 2 choice grain for the guild stores",
				Profession: "farmer", MinProfessionLevel: 2, MinFactionLevel: 2, NodeFeature: "farmland",

				Ticks: 4, EnergyCost: 14,
				Inputs:     map[string]int{"food": 2},
				OutputItem: "choice_grain", OutputQty: 2,
			},
			{
				ID: "forage_food", Label: "Forage wild food from field margins and woods",
				Profession: "gatherer", MinProfessionLevel: 1, NodeFeature: "forage_ground",
				ResourcePool: "soil_fertility", ResourceDraw: 1,
				Ticks: 3, EnergyCost: 9,
				OutputItem: "food", OutputQty: 1,
			},
			{
				ID: "fell_timber", Label: "Fell and trim 2 timber in the north woods",
				Profession: "woodcutter", MinProfessionLevel: 1, NodeFeature: "woodland",
				ResourcePool: "timber_stand", ResourceDraw: 2,
				Ticks: 3, EnergyCost: 12,
				BonusInput: "tools", BonusOutput: 1, DurableBonus: true,
				OutputItem: "timber", OutputQty: 2,
			},
			{
				ID: "saw_planks", Label: "Saw 2 timber into 2 roof planks at carpentry bench",
				Profession: "carpenter", MinProfessionLevel: 1, HouseFacility: "carpentry_bench",
				Ticks: 3, EnergyCost: 10,
				Inputs:     map[string]int{"timber": 2},
				BonusInput: "tools", BonusOutput: 1, DurableBonus: true,
				OutputItem: "plank", OutputQty: 2,
			},
			{
				ID: "sew_winter_coat", Label: "Craft 1 quilted winter fur coat from 2 cured hides",
				Profession: "carpenter", MinProfessionLevel: 2, HouseFacility: "carpentry_bench",
				Ticks: 3, EnergyCost: 10,
				Inputs:     map[string]int{"hide": 2},
				OutputItem: "winter_coat", OutputQty: 1,
			},
			{
				ID: "mine_ore", Label: "Collect 2 ironstone ore from rocky woodland",
				Profession: "miner", MinProfessionLevel: 1, NodeFeature: "woodland",
				ResourcePool: "ore_vein", ResourceDraw: 2,
				Ticks: 3, EnergyCost: 13,
				OutputItem: "ore", OutputQty: 2,
			},
			{
				ID: "deep_shaft_ore", Label: "Excavate 3 rich ironstone ore from deep mountain shaft",
				Profession: "miner", MinProfessionLevel: 1, NodeFeature: "mine_shaft",
				ResourcePool: "ore_vein", ResourceDraw: 3,
				Ticks: 3, EnergyCost: 13,
				BonusInput: "tools", BonusOutput: 1, DurableBonus: true,
				OutputItem: "ore", OutputQty: 3,
			},
			{
				ID: "forge_tools", Label: "Forge durable hand tools from ore and timber",
				Profession: "blacksmith", MinProfessionLevel: 1, HouseFacility: "forge",
				Ticks: 4, EnergyCost: 14,
				Inputs:     map[string]int{"ore": 1, "timber": 1},
				OutputItem: "tools", OutputQty: 1,
			},
			{
				ID: "forge_fine_tools", Label: "Forge a masterwork fine tool set for the guild",
				Profession: "blacksmith", MinProfessionLevel: 3, MinFactionLevel: 3, HouseFacility: "forge",
				Ticks: 4, EnergyCost: 16,
				Inputs:     map[string]int{"tools": 1, "ore": 1},
				OutputItem: "fine_tools", OutputQty: 1,
			},
			{
				ID: "gather_herbs", Label: "Gather 2 mountain herbs in the north woods",
				Profession: "herbalist", MinProfessionLevel: 1, NodeFeature: "herb_patch",
				ResourcePool: "herb_bed", ResourceDraw: 2,
				Ticks: 2, EnergyCost: 7,
				OutputItem: "herb", OutputQty: 2,
			},
			{
				ID: "brew_medicine", Label: "Steep herbs and well water into 2 herbal medicines",
				Profession: "herbalist", MinProfessionLevel: 1, HouseFacility: "herb_rack",
				Ticks: 3, EnergyCost: 8,
				Inputs:     map[string]int{"herb": 2, "water": 1},
				OutputItem: "medicine", OutputQty: 2,
			},
			{
				ID: "distill_tonic", Label: "Distill 2 potent tonics at the herb rack",
				Profession: "herbalist", MinProfessionLevel: 3, MinFactionLevel: 2, HouseFacility: "herb_rack",
				Ticks: 3, EnergyCost: 10,
				Inputs:     map[string]int{"medicine": 2, "herb": 1},
				OutputItem: "tonic", OutputQty: 2,
			},
			{
				ID: "refine_elixir_tonic", Label: "Refine 3 spirit tonics from rare mountain herbs",
				Profession: "alchemist", MinProfessionLevel: 1, HouseFacility: "herb_rack",
				Ticks: 3, EnergyCost: 10,
				Inputs:     map[string]int{"rare_herb": 1, "water": 2},
				OutputItem: "tonic", OutputQty: 3,
			},
			{
				ID: "transcribe_scroll", Label: "Transcribe 1 classical scroll at the academy",
				Profession: "scholar", MinProfessionLevel: 1, NodeFeature: "academy",
				Ticks: 3, EnergyCost: 8,
				OutputItem: "scroll", OutputQty: 1,
				Inputs: map[string]int{"paper": 1, "ink": 1},
			},
			{
				ID: "compile_classic", Label: "Compile 1 annotated classic treatise as a xiucai",
				Profession: "xiucai", MinProfessionLevel: 1, NodeFeature: "academy",
				Ticks: 4, EnergyCost: 11,
				Inputs:     map[string]int{"scroll": 1},
				OutputItem: "classic_tome", OutputQty: 1,
			},
			{
				ID: "practice_martial_arts", Label: "Drill forms and compile 1 boxing manual at the martial hall",
				Profession: "martial_artist", MinProfessionLevel: 1, NodeFeature: "martial_hall",
				Ticks: 3, EnergyCost: 10,
				BonusInput: "weapon", BonusOutput: 1, DurableBonus: true,
				OutputItem: "martial_manual", OutputQty: 1,
				Inputs: map[string]int{"paper": 1, "ink": 1},
			},
			{
				ID: "temper_weapon", Label: "Temper 1 steel blade from ore and timber at the martial hall",
				Profession: "martial_artist", MinProfessionLevel: 2, NodeFeature: "martial_hall",
				Ticks: 4, EnergyCost: 13,
				Inputs:     map[string]int{"ore": 1, "timber": 1},
				OutputItem: "weapon", OutputQty: 1,
			},
			{
				ID: "issue_bounty_writ", Label: "Investigate cases and issue 1 yamen bounty writ",
				Profession: "constable", MinProfessionLevel: 1, NodeFeature: "yamen",
				Ticks: 3, EnergyCost: 9,
				BonusInput: "scroll", BonusOutput: 1,
				OutputItem: "bounty_writ", OutputQty: 1,
				Inputs: map[string]int{"paper": 1, "ink": 1},
			},
			{
				ID: "graze_livestock", Label: "Graze and breed 1 pasture livestock herd",
				Profession: "herder", MinProfessionLevel: 1, NodeFeature: "pasture",
				ResourcePool: "pasture_grass", ResourceDraw: 2,
				Ticks: 3, EnergyCost: 11,
				BonusInput: "water", BonusOutput: 1,
				OutputItem: "calf", OutputQty: 1,
				Growth: &GrowthRule{CareWorkersPerBatch: 1, FailureAfterTicks: 7 * TicksPerDay, DurationTicks: 283 * TicksPerDay, MaintenanceIntervalTicks: TicksPerDay, Parents: map[string]int{"livestock": 2}, Source: "UNL Beef: cattle gestation typically 283 days; breeding pair and care inputs declared by this preset"},
			},
			{
				ID: "butcher_stock", Label: "Dress 1 livestock into 3 cured meat rations",
				Profession: "herder", MinProfessionLevel: 1, NodeFeature: "pasture",
				Ticks: 3, EnergyCost: 10,
				Inputs:     map[string]int{"livestock": 1},
				OutputItem: "meat", OutputQty: 3,
			},
			{
				ID: "tan_pasture_hide", Label: "Cure 2 hides from pasture stock",
				Profession: "herder", MinProfessionLevel: 2, NodeFeature: "pasture",
				Ticks: 3, EnergyCost: 9,
				Inputs:     map[string]int{"livestock": 1},
				OutputItem: "hide", OutputQty: 2,
			},
			{
				ID: "net_fish", Label: "Cast nets and land 3 river fish at fishery waters",
				Profession: "fisher", MinProfessionLevel: 1, NodeFeature: "fishery_waters",
				ResourcePool: "fish_stock", ResourceDraw: 2,
				Ticks: 3, EnergyCost: 10,
				BonusInput: "tools", BonusOutput: 1, DurableBonus: true,
				OutputItem: "fish", OutputQty: 3,
			},
			{
				ID: "haul_canal_water", Label: "Haul 4 barrels of clean river water at canal wharf",
				Profession: "boatman", MinProfessionLevel: 1, NodeFeature: "canal_wharf",
				ResourcePool: "water_table", ResourceDraw: 2,
				Ticks: 2, EnergyCost: 7,
				OutputItem: "water", OutputQty: 4,
			},
			{
				ID: "hunt_game", Label: "Track and hunt wild game for 2 cured meat rations",
				Profession: "hunter", MinProfessionLevel: 1, NodeFeature: "hunting_ground",
				ResourcePool: "game_herd", ResourceDraw: 2,
				Ticks: 3, EnergyCost: 12,
				BonusInput: "weapon", BonusOutput: 1, DurableBonus: true,
				OutputItem: "meat", OutputQty: 2,
			},
			{
				ID: "trap_pelts", Label: "Trap mountain beasts for 2 cured hides",
				Profession: "hunter", MinProfessionLevel: 1, NodeFeature: "hunting_ground",
				ResourcePool: "game_herd", ResourceDraw: 1,
				Ticks: 3, EnergyCost: 11,
				BonusInput: "tools", BonusOutput: 1, DurableBonus: true,
				OutputItem: "hide", OutputQty: 2,
			},
			{
				ID: "forage_rare_herb", Label: "Scale cliffs in hunting grounds for 1 spirit mountain ginseng",
				Profession: "hunter", MinProfessionLevel: 2, NodeFeature: "hunting_ground",
				ResourcePool: "herb_bed", ResourceDraw: 2,
				Ticks: 4, EnergyCost: 12,
				OutputItem: "rare_herb", OutputQty: 1,
			},
			{
				ID: "prepare_offering", Label: "Consecrate 2 sacrificial offerings at the grand shrine",
				Profession: "ritualist", MinProfessionLevel: 1, NodeFeature: "shrine",
				Ticks: 3, EnergyCost: 8,
				Inputs:     map[string]int{"food": 2},
				BonusInput: "rare_herb", BonusOutput: 1,
				OutputItem: "sacrificial_offering", OutputQty: 2,
			},
		},
		TagProfiles: map[string]TagProfile{
			"resident": {
				Consumes:    []string{"food", "water", "choice_grain", "meat", "fish", "plank", "winter_coat"},
				TargetStock: map[string]int{"food": 6, "plank": 1},
			},
			"farmer": {
				Produces:    []string{"food", "choice_grain"},
				Consumes:    []string{"nightsoil", "seeds", "water", "compost", "tools"},
				TargetStock: map[string]int{"food": 12, "choice_grain": 4},
			},
			"agronomist": {
				Produces:    []string{"food", "choice_grain"},
				Consumes:    []string{"nightsoil", "seeds", "water", "compost", "tools"},
				TargetStock: map[string]int{"food": 14, "choice_grain": 6},
			},
			"gatherer": {
				Produces:    []string{"food"},
				TargetStock: map[string]int{"food": 5},
			},
			"woodcutter": {
				Produces:    []string{"timber"},
				Consumes:    []string{"food", "tools"},
				TargetStock: map[string]int{"timber": 6, "food": 6},
			},
			"carpenter": {
				Produces:    []string{"plank", "winter_coat"},
				Consumes:    []string{"timber", "hide", "food", "tools"},
				TargetStock: map[string]int{"plank": 5, "winter_coat": 1, "timber": 2, "food": 6},
			},
			"master_carpenter": {
				Produces:    []string{"plank", "winter_coat"},
				Consumes:    []string{"timber", "hide", "food", "tools"},
				TargetStock: map[string]int{"plank": 6, "winter_coat": 2, "timber": 2, "food": 6},
			},
			"miner": {
				Produces:    []string{"ore"},
				Consumes:    []string{"food", "tools"},
				TargetStock: map[string]int{"ore": 5, "food": 6},
			},
			"blacksmith": {
				Produces:    []string{"tools", "fine_tools"},
				Consumes:    []string{"ore", "timber", "food"},
				TargetStock: map[string]int{"tools": 3, "fine_tools": 2, "ore": 2, "timber": 2, "food": 6},
			},
			"master_smith": {
				Produces:    []string{"tools", "fine_tools"},
				Consumes:    []string{"ore", "timber", "food"},
				TargetStock: map[string]int{"tools": 4, "fine_tools": 2, "ore": 2, "timber": 2, "food": 6},
			},
			"herbalist": {
				Produces:    []string{"herb", "medicine", "tonic"},
				Consumes:    []string{"water", "food"},
				TargetStock: map[string]int{"medicine": 4, "tonic": 2, "herb": 2, "food": 6},
			},
			"alchemist": {
				Produces:    []string{"herb", "medicine", "tonic"},
				Consumes:    []string{"water", "rare_herb", "food"},
				TargetStock: map[string]int{"medicine": 4, "tonic": 3, "herb": 2, "food": 6},
			},
			"healer": {
				Consumes:        []string{"medicine", "tonic", "food"},
				TargetStock:     map[string]int{"medicine": 4, "tonic": 2, "food": 6},
				PublicTrustDuty: true,
			},
			"herder": {
				Produces:    []string{"calf", "livestock", "meat", "hide"},
				Consumes:    []string{"calf", "water", "food"},
				TargetStock: map[string]int{"livestock": 3, "meat": 6, "hide": 3, "food": 6},
			},
			"ranch_master": {
				Produces:    []string{"calf", "livestock", "meat", "hide"},
				Consumes:    []string{"calf", "water", "food"},
				TargetStock: map[string]int{"livestock": 4, "meat": 8, "hide": 4, "food": 6},
			},
			"fisher": {
				Produces:    []string{"fish"},
				Consumes:    []string{"tools", "food"},
				TargetStock: map[string]int{"fish": 8, "food": 6},
			},
			"fleet_master": {
				Produces:    []string{"fish"},
				Consumes:    []string{"tools", "fine_tools", "food"},
				TargetStock: map[string]int{"fish": 10, "food": 6},
			},
			"boatman": {
				Produces:    []string{"water"},
				Consumes:    []string{"food", "fish", "plank", "tools"},
				TargetStock: map[string]int{"water": 6, "food": 6, "plank": 2},
			},
			"canal_master": {
				Produces:    []string{"water"},
				Consumes:    []string{"food", "choice_grain", "fish", "plank", "fine_tools"},
				TargetStock: map[string]int{"water": 8, "food": 8, "plank": 2},
			},
			"hunter": {
				Produces:      []string{"meat", "hide", "rare_herb"},
				Consumes:      []string{"weapon", "tools", "medicine", "food"},
				TargetStock:   map[string]int{"meat": 6, "hide": 4, "rare_herb": 2, "food": 6},
				CoercivePower: true,
			},
			"master_hunter": {
				Produces:      []string{"meat", "hide", "rare_herb"},
				Consumes:      []string{"weapon", "fine_tools", "tonic", "medicine", "food"},
				TargetStock:   map[string]int{"meat": 8, "hide": 5, "rare_herb": 3, "food": 6},
				CoercivePower: true,
			},
			"ritualist": {
				Produces:        []string{"sacrificial_offering"},
				Consumes:        []string{"food", "choice_grain", "rare_herb", "scroll"},
				TargetStock:     map[string]int{"sacrificial_offering": 4, "food": 6},
				PublicTrustDuty: true,
			},
			"high_priest": {
				Produces:        []string{"sacrificial_offering"},
				Consumes:        []string{"food", "choice_grain", "rare_herb", "classic_tome"},
				TargetStock:     map[string]int{"sacrificial_offering": 6, "food": 6},
				PublicTrustDuty: true,
			},
			"fighter": {
				Consumes:      []string{"food", "tools", "weapon"},
				TargetStock:   map[string]int{"food": 6},
				CoercivePower: true,
			},
			"peddler": {
				Consumes:    []string{"food", "choice_grain", "meat", "fish", "hide", "winter_coat", "rare_herb", "sacrificial_offering", "paper", "ink", "timber", "plank", "ore", "tools", "fine_tools", "herb", "medicine", "tonic", "scroll", "classic_tome", "weapon", "martial_manual"},
				TargetStock: map[string]int{"food": 8, "choice_grain": 2, "timber": 2, "plank": 2, "ore": 2, "tools": 1, "medicine": 2, "scroll": 2},
			},
			"merchant_chief": {
				Consumes:    []string{"food", "choice_grain", "meat", "fish", "hide", "winter_coat", "rare_herb", "sacrificial_offering", "paper", "ink", "timber", "plank", "ore", "tools", "fine_tools", "herb", "medicine", "tonic", "scroll", "classic_tome", "weapon", "martial_manual"},
				TargetStock: map[string]int{"food": 8, "choice_grain": 2, "timber": 2, "plank": 2, "ore": 2, "tools": 1, "fine_tools": 1, "medicine": 2, "tonic": 1, "scroll": 2, "classic_tome": 1},
			},
			"guard": {
				Consumes:      []string{"food", "tools", "weapon", "bounty_writ", "medicine"},
				TargetStock:   map[string]int{"food": 6, "medicine": 1},
				CoercivePower: true,
			},
			"captain": {
				Consumes:      []string{"food", "tools", "fine_tools", "weapon", "bounty_writ", "medicine", "tonic"},
				TargetStock:   map[string]int{"food": 6, "medicine": 2, "tonic": 1},
				CoercivePower: true,
			},
			"scholar": {
				Produces:        []string{"paper", "ink", "scroll"},
				Consumes:        []string{"timber", "water", "herb", "food"},
				TargetStock:     map[string]int{"scroll": 3, "food": 6},
				PublicTrustDuty: true,
			},
			"xiucai": {
				Produces:        []string{"paper", "ink", "scroll", "classic_tome"},
				Consumes:        []string{"timber", "water", "herb", "food", "scroll"},
				TargetStock:     map[string]int{"classic_tome": 2, "scroll": 2, "food": 6},
				PublicTrustDuty: true,
			},
			"martial_artist": {
				Produces:      []string{"martial_manual", "weapon"},
				Consumes:      []string{"paper", "ink", "food", "ore", "timber", "medicine", "tonic"},
				TargetStock:   map[string]int{"martial_manual": 3, "weapon": 2, "food": 6, "medicine": 2},
				CoercivePower: true,
			},
			"martial_master": {
				Produces:      []string{"martial_manual", "weapon"},
				Consumes:      []string{"paper", "ink", "food", "ore", "timber", "medicine", "tonic"},
				TargetStock:   map[string]int{"martial_manual": 4, "weapon": 3, "food": 6, "tonic": 2},
				CoercivePower: true,
			},
			"constable": {
				Produces:        []string{"bounty_writ"},
				Consumes:        []string{"paper", "ink", "food", "scroll", "weapon", "medicine"},
				TargetStock:     map[string]int{"bounty_writ": 3, "weapon": 1, "food": 6},
				CoercivePower:   true,
				PublicTrustDuty: true,
			},
			"chief_constable": {
				Produces:        []string{"bounty_writ"},
				Consumes:        []string{"paper", "ink", "food", "scroll", "weapon", "medicine", "tonic"},
				TargetStock:     map[string]int{"bounty_writ": 4, "weapon": 2, "food": 6},
				CoercivePower:   true,
				PublicTrustDuty: true,
			},
			"bandit": {

				Consumes:      []string{"food", "choice_grain", "weapon", "medicine"},
				TargetStock:   map[string]int{"food": 8, "weapon": 1, "medicine": 2},
				CoercivePower: true,
			},
			"bandit_leader": {

				Consumes:      []string{"food", "choice_grain", "weapon", "medicine", "tonic"},
				TargetStock:   map[string]int{"food": 10, "weapon": 2, "tonic": 1},
				CoercivePower: true,
			},
			"thief": {
				Consumes:    []string{"food", "choice_grain", "medicine"},
				TargetStock: map[string]int{"food": 6, "medicine": 1},
			},
			"master_thief": {
				Consumes:    []string{"food", "choice_grain", "medicine", "tonic"},
				TargetStock: map[string]int{"food": 6, "tonic": 1},
			},
			"gangster": {
				Consumes:      []string{"food", "choice_grain", "weapon", "medicine", "martial_manual"},
				TargetStock:   map[string]int{"food": 6, "weapon": 1, "medicine": 2},
				CoercivePower: true,
			},
			"gang_boss": {
				Consumes:      []string{"food", "choice_grain", "weapon", "medicine", "tonic", "martial_manual"},
				TargetStock:   map[string]int{"food": 8, "weapon": 2, "tonic": 1},
				CoercivePower: true,
			},
		},
	}
	return withProductionMaterials(catalog)
}

// DefaultFactions returns the village guilds. Members and levels are seeded by Village.
func DefaultFactions() []Faction {
	return []Faction{
		{ID: "farmers_guild", Name: "农会", District: "south_fields", Professions: []string{"farmer", "gatherer", "agronomist"}, Requirements: map[string]int{"strength": 6}},
		{ID: "artisans_guild", Name: "匠作行", District: "east_lane", Professions: []string{"woodcutter", "carpenter", "miner", "blacksmith", "master_smith", "master_carpenter"}, Requirements: map[string]int{"dexterity": 7}},
		{ID: "healers_guild", Name: "药行", District: "market_street", Professions: []string{"herbalist", "healer", "alchemist"}, Requirements: map[string]int{"insight": 8}},
		{ID: "merchants_guild", Name: "商帮", District: "market_street", Professions: []string{"peddler", "merchant_chief"}, Requirements: map[string]int{"spirit": 7}},
		{ID: "watch_guild", Name: "巡守", District: "crossroads", Professions: []string{"guard", "captain"}, Requirements: map[string]int{"strength": 8}},
		{ID: "scholars_guild", Name: "书院", District: "crossroads", Professions: []string{"scholar", "xiucai"}, Requirements: map[string]int{"insight": 8}},
		{ID: "martial_guild", Name: "武馆", District: "east_lane", Professions: []string{"martial_artist", "martial_master"}, Requirements: map[string]int{"strength": 8, "dexterity": 7}},
		{ID: "yamen_guild", Name: "府衙", District: "crossroads", Professions: []string{"constable", "chief_constable"}, Requirements: map[string]int{"strength": 8, "insight": 8}},
		{ID: "bandits_guild", Name: "山贼山寨", District: "north_woods", Professions: []string{"bandit", "bandit_leader"}, Requirements: map[string]int{"strength": 8, "dexterity": 7}},
		{ID: "gang_guild", Name: "帮派堂口", District: "market_street", Professions: []string{"thief", "master_thief", "gangster", "gang_boss"}, Requirements: map[string]int{"dexterity": 7, "spirit": 7}},
	}
}

func ActorHasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func (c Catalog) ProfessionByID(id string) (Profession, bool) {
	for _, p := range c.Professions {
		if p.ID == id {
			return p, true
		}
	}
	return Profession{}, false
}

func (c Catalog) RecipeByID(id string) (Recipe, bool) {
	for _, r := range c.Recipes {
		if r.ID == id {
			return r, true
		}
	}
	return Recipe{}, false
}

// ProfessionLevel maps accumulated profession experience to a level capped by the catalog thresholds.
func (c Catalog) ProfessionLevel(xp int) int {
	level := 1
	for i, threshold := range c.LevelThresholds {
		if xp >= threshold {
			level = i + 1
		}
	}
	return level
}

// FactionLevel maps accumulated guild influence to a level capped by the catalog thresholds.
func (c Catalog) FactionLevel(influence int) int {
	level := 1
	for i, threshold := range c.FactionLevelThresholds {
		if influence >= threshold {
			level = i + 1
		}
	}
	return level
}

// Capabilities reports which action kinds the actor's tags and professions grant.
func (c Catalog) Capabilities(a Actor) map[string]bool {
	result := map[string]bool{}
	idTags := c.EffectiveTags(a)
	for _, tag := range idTags {
		for _, action := range c.TagActions[tag] {
			result[action] = true
		}
	}
	if len(a.Professions) > 0 {
		result["craft"] = true
		result["harvest"] = true
	}
	for _, r := range c.Recipes {
		if r.RequiredTag != "" && ActorHasTag(idTags, r.RequiredTag) {
			result["craft"] = true
			result["harvest"] = true
			break
		}
	}
	for _, rit := range c.Rituals {
		if rit.RequiredTag != "" && ActorHasTag(idTags, rit.RequiredTag) {
			result["perform_ritual"] = true
			break
		}
	}
	for _, enc := range c.Encounters {
		for _, st := range enc.SuppressorTags {
			if ActorHasTag(idTags, st) {
				result["hunt_threat"] = true
				break
			}
		}
	}
	if result["move"] {
		result["move_venue"] = true
	}
	for _, execTag := range []string{"ruler", "governor", "magistrate", "sect_master", "commander", "polity_leader"} {
		if ActorHasTag(idTags, execTag) {
			result["issue_decree"] = true
			break
		}
	}
	for _, ch := range c.CommChannels {
		if len(ch.GrantedByTags) == 0 {
			result["transmit_message"] = true
			break
		}
		for _, gt := range ch.GrantedByTags {
			if ActorHasTag(idTags, gt) {
				result["transmit_message"] = true
				break
			}
		}
	}
	for k, spec := range c.ActionSpecs {
		actionID := spec.ID
		if actionID == "" {
			actionID = k
		}
		if actionID == "" {
			continue
		}
		if len(spec.GrantedByTags) == 0 {
			result[actionID] = true
			continue
		}
		for _, gt := range spec.GrantedByTags {
			if ActorHasTag(idTags, gt) {
				result[actionID] = true
				break
			}
		}
	}
	if len(c.Transitions) > 0 {
		result["attempt_transition"] = true
	}
	if len(c.StateSlots) > 0 || len(c.Currencies) > 0 {
		result["refine_currency_item"] = true
		result["absorb_item_slot"] = true
	} else {
		for _, spec := range c.Items {
			if spec.DualUseCurrency != "" || spec.DualUseRate > 0 {
				result["refine_currency_item"] = true
				result["absorb_item_slot"] = true
				break
			}
		}
	}
	if result["wait"] || ActorHasTag(idTags, "resident") || ActorHasTag(idTags, "engineer") || ActorHasTag(idTags, "saboteur") || result["attack"] || result["craft"] {
		result["sever_flow_edge"] = true
		result["repair_flow_edge"] = true
	}
	if len(c.Skills) > 0 {
		result["teach_knowledge"] = true
	}
	if len(c.Techs) > 0 {
		result["teach_knowledge"] = true
		result["adopt_facility"] = true
		for _, tech := range c.Techs {
			if len(tech.GrantedByTags) == 0 {
				result["research_tech"] = true
				break
			}
			for _, gt := range tech.GrantedByTags {
				if ActorHasTag(idTags, gt) {
					result["research_tech"] = true
					break
				}
			}
			if result["research_tech"] {
				break
			}
		}
	}
	return result
}

// EffectiveTags returns the actor's identity tags combined with form innate tags and active profession grants.
func (c Catalog) EffectiveTags(a Actor) []string {
	return actorEffectiveTags(c, a)
}

// CanLearnProfession checks whether actor `a` meets the attribute, parent profession, and guild level requirements
// to learn a base profession or advance into a higher-tier profession.
func (c Catalog) CanLearnProfession(a Actor, prof Profession) (bool, string) {
	if _, held := a.Professions[prof.ID]; held {
		return false, "already practices " + prof.Name
	}
	if prof.ParentProfession != "" {
		reqLevel := prof.MinParentLevel
		if reqLevel < 1 {
			reqLevel = 2
		}
		if a.Professions[prof.ParentProfession] < reqLevel {
			return false, fmt.Sprintf("advancing to %s requires %s level %d (has %d)", prof.ID, prof.ParentProfession, reqLevel, a.Professions[prof.ParentProfession])
		}
	}
	if prof.MinFactionLevel > 0 && a.GuildLevels[prof.Guild] < prof.MinFactionLevel {
		return false, fmt.Sprintf("learning %s requires guild %s level %d", prof.ID, prof.Guild, prof.MinFactionLevel)
	}
	if ok, reason := a.Attributes.Meets(prof.Requirements); !ok {
		return false, "profession requirements unmet: " + reason
	}
	return true, ""
}

// ProfessionGating reports whether the actor's profession levels, independent skill levels, and guild standing qualify for the recipe.
func (c Catalog) ProfessionGating(a Actor, r Recipe) (bool, string) {
	hasSkillAccess := false
	if r.SkillID != "" {
		reqSkill := r.MinSkillLevel
		if reqSkill < 1 {
			reqSkill = 1
		}
		if a.Skills[r.SkillID] >= reqSkill {
			hasSkillAccess = true
		}
	}
	if r.Profession != "" {
		level, held := a.Professions[r.Profession]
		required := r.MinProfessionLevel
		if required < 1 {
			required = 1
		}
		profOK := held && level >= required
		if profOK && r.MinFactionLevel > 0 {
			prof, _ := c.ProfessionByID(r.Profession)
			if a.GuildLevels[prof.Guild] < r.MinFactionLevel {
				profOK = false
			}
		}
		if !profOK && !hasSkillAccess {
			if !held {
				return false, "actor does not practice profession: " + r.Profession
			}
			if level < required {
				return false, fmt.Sprintf("recipe %s requires %s level %d", r.ID, r.Profession, required)
			}
			prof, _ := c.ProfessionByID(r.Profession)
			return false, fmt.Sprintf("recipe %s requires guild %s level %d", r.ID, prof.Guild, r.MinFactionLevel)
		}
	} else if r.SkillID != "" && !hasSkillAccess {
		reqSkill := r.MinSkillLevel
		if reqSkill < 1 {
			reqSkill = 1
		}
		return false, fmt.Sprintf("recipe %s requires skill %s level %d", r.ID, r.SkillID, reqSkill)
	}
	if r.RequiredTag != "" && !ActorHasTag(c.ActorEffectiveTags(a), r.RequiredTag) {
		return false, "recipe " + r.ID + " requires tag: " + r.RequiredTag
	}
	return true, ""
}

// ActorPowerTier returns the ordered strength/realm tier of an actor's current FormID (0 if unset).
func (c Catalog) ActorPowerTier(a Actor) int {
	if a.FormID == "" || len(c.Forms) == 0 {
		return 0
	}
	if form, ok := c.Forms[a.FormID]; ok {
		return form.PowerTier
	}
	return 0
}

// ActorFormName returns the display name of an actor's current FormID.
func (c Catalog) ActorFormName(a Actor) string {
	if a.FormID == "" {
		return ""
	}
	if form, ok := c.Forms[a.FormID]; ok && form.Name != "" {
		return form.Name
	}
	return a.FormID
}

// SkillHealBonus returns the total extra health restored by an actor's healing skills.
func (c Catalog) SkillHealBonus(a Actor) int {
	bonus := 0
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := c.Skills[sid]; ok && sk.HealBonusPerLevel > 0 {
			bonus += lv * sk.HealBonusPerLevel
		}
	}
	return bonus
}

// SkillRepairBonus returns the total extra infrastructure/house condition restored by an actor's repair/array skills.
func (c Catalog) SkillRepairBonus(a Actor) int {
	bonus := 0
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := c.Skills[sid]; ok && sk.RepairBonusPerLevel > 0 {
			bonus += lv * sk.RepairBonusPerLevel
		}
	}
	return bonus
}

// SkillLevel maps accumulated skill XP onto the catalog's level thresholds.
func (c Catalog) SkillLevel(xp int) int {
	return c.ProfessionLevel(xp)
}

// RecipeYield is the declared material conversion per batch. Skill and guild
// standing grant access and experience, never additional matter or nutrition.
// A biological bonus is an explicit addition to the configured growth budget.
func (c Catalog) RecipeYield(_ Actor, r Recipe, bonus bool) int {
	yield := r.OutputQty
	if bonus && c.ConsumablesYieldBonus(r) {
		yield += r.BonusOutput
	}
	return yield
}

func (c Catalog) ConsumablesYieldBonus(r Recipe) bool {
	return r.Growth != nil && r.BonusInput != "" && !r.DurableBonus && r.BonusOutput > 0
}

// CraftXP returns the profession experience earned by completing a recipe.
func (c Catalog) CraftXP(r Recipe) int {
	tier := c.Items[r.OutputItem].Tier
	if tier < 1 {
		tier = 1
	}
	ticks := r.Ticks
	if ticks < 1 {
		ticks = 1
	}
	return tier * ticks
}

// SalePrice returns the asking price for a batch, raised by merchant profession levels and advanced merchant rank.
func (c Catalog) SalePrice(a Actor, itemID string) int {
	spec, ok := c.Items[itemID]
	if !ok {
		return 0
	}
	price := spec.BatchPrice
	for _, prof := range c.Professions {
		if !prof.TradeMargin {
			continue
		}
		if level := a.Professions[prof.ID]; level > 0 {
			price += prof.BaseYieldBonus
			if level > 1 {
				price += (level - 1) / 2
			}
		}
	}
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := c.Skills[sid]; ok && sk.TradeMarginPerLevel > 0 {
			price += (lv * sk.TradeMarginPerLevel) / 3
		}
	}
	return price
}

func (c Catalog) TagsProduce(tags []string, item string) bool {
	for _, t := range tags {
		for _, p := range c.TagProfiles[t].Produces {
			if p == item {
				return true
			}
		}
	}
	return false
}

func (c Catalog) TagsConsume(tags []string, item string) bool {
	for _, t := range tags {
		for _, want := range c.TagProfiles[t].Consumes {
			if want == item {
				return true
			}
		}
	}
	return false
}

func (c Catalog) HasCoercivePower(tags []string) bool {
	for _, t := range tags {
		if c.TagProfiles[t].CoercivePower {
			return true
		}
	}
	return false
}

func (c Catalog) HasPublicTrustDuty(tags []string) bool {
	for _, t := range tags {
		if c.TagProfiles[t].PublicTrustDuty {
			return true
		}
	}
	return false
}

func (c Catalog) DesiredStock(tags []string, item string) int {
	best := 0
	for _, t := range tags {
		if v := c.TagProfiles[t].TargetStock[item]; v > best {
			best = v
		}
	}
	return best
}

func CloneCatalog(c Catalog) Catalog {
	out := Catalog{
		Production:             c.Production,
		TagActions:             make(map[string][]string, len(c.TagActions)),
		Items:                  make(map[string]ItemSpec, len(c.Items)),
		Professions:            make([]Profession, len(c.Professions)),
		Recipes:                make([]Recipe, len(c.Recipes)),
		TagProfiles:            make(map[string]TagProfile, len(c.TagProfiles)),
		LevelThresholds:        append([]int(nil), c.LevelThresholds...),
		FactionLevelThresholds: append([]int(nil), c.FactionLevelThresholds...),
		Calendar:               CalendarSpec{},
		Lifecycle:              c.Lifecycle,
		Governance:             c.Governance,
		Environment:            CloneEnvironmentProfile(c.Environment),
	}
	if c.HouseWear != nil {
		rule := *c.HouseWear
		out.HouseWear = &rule
	}
	out.Production.FacilityCapacity = cloneInt64Map(c.Production.FacilityCapacity)
	for k, v := range c.TagActions {
		out.TagActions[k] = append([]string(nil), v...)
	}
	for k, v := range c.Items {
		ic := v
		if v.Living != nil {
			living := *v.Living
			living.Supplies = cloneInt64Map(v.Living.Supplies)
			ic.Living = &living
		}
		if v.AcceptedCurrencies != nil {
			ic.AcceptedCurrencies = append([]string(nil), v.AcceptedCurrencies...)
		}
		ic.SlotChanges = cloneIntMap(v.SlotChanges)
		if v.EquipRequiredTags != nil {
			ic.EquipRequiredTags = append([]string(nil), v.EquipRequiredTags...)
		}
		ic.EquipMinAttributes = cloneIntMap(v.EquipMinAttributes)
		if v.EquipRequiredForms != nil {
			ic.EquipRequiredForms = append([]string(nil), v.EquipRequiredForms...)
		}
		ic.EquipDefenseByType = cloneDefenseMap(v.EquipDefenseByType)
		out.Items[k] = ic
	}
	for i, p := range c.Professions {
		pc := p
		if p.Requirements != nil {
			pc.Requirements = make(map[string]int, len(p.Requirements))
			for k, v := range p.Requirements {
				pc.Requirements[k] = v
			}
		}
		pc.Grants = append([]string(nil), p.Grants...)
		pc.Ranks = append([]string(nil), p.Ranks...)
		out.Professions[i] = pc
	}
	for i, r := range c.Recipes {
		rc := r
		if r.Conversion != nil {
			conversion := *r.Conversion
			rc.Conversion = &conversion
		}
		rc.Growth = cloneGrowthRule(r.Growth)
		if r.Inputs != nil {
			rc.Inputs = make(map[string]int, len(r.Inputs))
			for k, v := range r.Inputs {
				rc.Inputs[k] = v
			}
		}
		out.Recipes[i] = rc
	}
	for k, p := range c.TagProfiles {
		pc := p
		if p.Produces != nil {
			pc.Produces = append([]string(nil), p.Produces...)
		}
		if p.Consumes != nil {
			pc.Consumes = append([]string(nil), p.Consumes...)
		}
		if p.TargetStock != nil {
			pc.TargetStock = make(map[string]int, len(p.TargetStock))
			for sk, sv := range p.TargetStock {
				pc.TargetStock[sk] = sv
			}
		}
		out.TagProfiles[k] = pc
	}
	if c.Calendar.Seasons != nil {
		out.Calendar.Seasons = make([]SeasonSpec, len(c.Calendar.Seasons))
		for i, s := range c.Calendar.Seasons {
			sc := s
			sc.FeatureYieldPct = cloneIntMap(s.FeatureYieldPct)
			sc.WeatherWeights = cloneIntMap(s.WeatherWeights)
			sc.RouteCapacityPct = cloneIntMap(s.RouteCapacityPct)
			out.Calendar.Seasons[i] = sc
		}
	}
	if c.Weathers != nil {
		out.Weathers = make(map[string]WeatherSpec, len(c.Weathers))
		for k, ws := range c.Weathers {
			wc := ws
			wc.RouteHazardPermille = cloneIntMap(ws.RouteHazardPermille)
			wc.RouteCapacityPct = cloneIntMap(ws.RouteCapacityPct)
			wc.FeatureYieldPct = cloneIntMap(ws.FeatureYieldPct)
			wc.ResourceRegenPct = cloneIntMap(ws.ResourceRegenPct)
			out.Weathers[k] = wc
		}
	}
	if c.Resources != nil {
		out.Resources = make(map[string]ResourcePoolSpec, len(c.Resources))
		for k, rs := range c.Resources {
			rc := rs
			rc.NodeFeatures = append([]string(nil), rs.NodeFeatures...)
			rc.SeasonalRegenPct = cloneIntMap(rs.SeasonalRegenPct)
			out.Resources[k] = rc
		}
	}
	if c.Encounters != nil {
		out.Encounters = make([]EncounterSpec, len(c.Encounters))
		for i, enc := range c.Encounters {
			ec := enc
			ec.NodeFeatures = append([]string(nil), enc.NodeFeatures...)
			ec.SuppressorTags = append([]string(nil), enc.SuppressorTags...)
			out.Encounters[i] = ec
		}
	}
	if c.Disasters != nil {
		out.Disasters = make([]DisasterSpec, len(c.Disasters))
		for i, dis := range c.Disasters {
			dc := dis
			dc.TriggerWeathers = append([]string(nil), dis.TriggerWeathers...)
			dc.TriggerSeasons = append([]string(nil), dis.TriggerSeasons...)
			dc.TargetFeatures = append([]string(nil), dis.TargetFeatures...)
			dc.ResourceDamagePct = cloneIntMap(dis.ResourceDamagePct)
			if dis.YieldPct != nil {
				yp := *dis.YieldPct
				dc.YieldPct = &yp
			}
			out.Disasters[i] = dc
		}
	}
	if c.Accidents != nil {
		out.Accidents = make([]AccidentSpec, len(c.Accidents))
		for i, acc := range c.Accidents {
			ac := acc
			ac.NodeFeatures = append([]string(nil), acc.NodeFeatures...)
			ac.Recipes = append([]string(nil), acc.Recipes...)
			ac.RouteMediums = append([]string(nil), acc.RouteMediums...)
			out.Accidents[i] = ac
		}
	}
	if c.Infrastructures != nil {
		out.Infrastructures = make(map[string]InfrastructureSpec, len(c.Infrastructures))
		for k, inf := range c.Infrastructures {
			ic := inf
			ic.NodeFeatures = append([]string(nil), inf.NodeFeatures...)
			ic.BuildInputs = cloneIntMap(inf.BuildInputs)
			out.Infrastructures[k] = ic
		}
	}
	if c.Rituals != nil {
		out.Rituals = make([]RitualSpec, len(c.Rituals))
		for i, rit := range c.Rituals {
			rc := rit
			rc.Inputs = cloneIntMap(rit.Inputs)
			out.Rituals[i] = rc
		}
	}
	if c.Diseases != nil {
		out.Diseases = append([]DiseaseSpec(nil), c.Diseases...)
	}
	if c.WorldEvents != nil {
		out.WorldEvents = make([]WorldEventSpec, len(c.WorldEvents))
		for i, we := range c.WorldEvents {
			out.WorldEvents[i] = cloneWorldEventSpec(we)
		}
	}
	if c.LocomotionModes != nil {
		out.LocomotionModes = make(map[string]LocomotionSpec, len(c.LocomotionModes))
		for k, lm := range c.LocomotionModes {
			lc := lm
			if lm.GrantedByTags != nil {
				lc.GrantedByTags = append([]string(nil), lm.GrantedByTags...)
			}
			if lm.AllowedMediums != nil {
				lc.AllowedMediums = append([]string(nil), lm.AllowedMediums...)
			}
			out.LocomotionModes[k] = lc
		}
	}
	if c.SensoryModes != nil {
		out.SensoryModes = make(map[string]SensorySpec, len(c.SensoryModes))
		for k, sm := range c.SensoryModes {
			sc := sm
			if sm.GrantedByTags != nil {
				sc.GrantedByTags = append([]string(nil), sm.GrantedByTags...)
			}
			out.SensoryModes[k] = sc
		}
	}
	if c.CommChannels != nil {
		out.CommChannels = make(map[string]CommChannelSpec, len(c.CommChannels))
		for k, cc := range c.CommChannels {
			cp := cc
			if cc.GrantedByTags != nil {
				cp.GrantedByTags = append([]string(nil), cc.GrantedByTags...)
			}
			if cc.ReceiverTags != nil {
				cp.ReceiverTags = append([]string(nil), cc.ReceiverTags...)
			}
			out.CommChannels[k] = cp
		}
	}
	if c.CombatProfiles != nil {
		out.CombatProfiles = make([]CombatProfileSpec, len(c.CombatProfiles))
		for i, cp := range c.CombatProfiles {
			out.CombatProfiles[i] = CloneCombatProfileSpec(cp)
		}
	}
	if c.ActionSpecs != nil {
		out.ActionSpecs = make(map[string]ActionSpec, len(c.ActionSpecs))
		for k, spec := range c.ActionSpecs {
			out.ActionSpecs[k] = CloneActionSpec(spec)
		}
	}
	if c.EquipSlots != nil {
		out.EquipSlots = make(map[string]EquipSlotSpec, len(c.EquipSlots))
		for k, es := range c.EquipSlots {
			ec := es
			if es.GrantedByTags != nil {
				ec.GrantedByTags = append([]string(nil), es.GrantedByTags...)
			}
			out.EquipSlots[k] = ec
		}
	}
	if c.StateSlots != nil {
		out.StateSlots = make(map[string]StateSlotSpec, len(c.StateSlots))
		for k, v := range c.StateSlots {
			out.StateSlots[k] = v
		}
	}
	if c.Forms != nil {
		out.Forms = make(map[string]FormSpec, len(c.Forms))
		for k, f := range c.Forms {
			fc := f
			if f.DailyIntakePct != nil {
				v := *f.DailyIntakePct
				fc.DailyIntakePct = &v
			}
			if f.StarvationDamage != nil {
				v := *f.StarvationDamage
				fc.StarvationDamage = &v
			}
			if f.SlotMetabolism != nil {
				fc.SlotMetabolism = cloneIntMap(f.SlotMetabolism)
			}
			if f.InnateTags != nil {
				fc.InnateTags = append([]string(nil), f.InnateTags...)
			}
			out.Forms[k] = fc
		}
	}
	if c.Skills != nil {
		out.Skills = make(map[string]SkillSpec, len(c.Skills))
		for k, sk := range c.Skills {
			sc := sk
			if sk.Ranks != nil {
				sc.Ranks = append([]string(nil), sk.Ranks...)
			}
			if sk.ActionAffinities != nil {
				sc.ActionAffinities = append([]string(nil), sk.ActionAffinities...)
			}
			out.Skills[k] = sc
		}
	}
	if c.Transitions != nil {
		out.Transitions = make(map[string]TransitionSpec, len(c.Transitions))
		for k, t := range c.Transitions {
			tc := t
			if t.RequiredSlots != nil {
				tc.RequiredSlots = cloneIntMap(t.RequiredSlots)
			}
			if t.RequiredSkills != nil {
				tc.RequiredSkills = cloneIntMap(t.RequiredSkills)
			}
			if t.ItemCosts != nil {
				tc.ItemCosts = cloneIntMap(t.ItemCosts)
			}
			if t.SlotCosts != nil {
				tc.SlotCosts = cloneIntMap(t.SlotCosts)
			}
			if t.SuccessGrantTags != nil {
				tc.SuccessGrantTags = append([]string(nil), t.SuccessGrantTags...)
			}
			if t.RewardAttributes != nil {
				tc.RewardAttributes = cloneIntMap(t.RewardAttributes)
			}
			out.Transitions[k] = tc
		}
	}
	if c.Currencies != nil {
		out.Currencies = make(map[string]CurrencySpec, len(c.Currencies))
		for k, v := range c.Currencies {
			out.Currencies[k] = v
		}
	}
	if c.Techs != nil {
		out.Techs = make(map[string]TechSpec, len(c.Techs))
		for k, ts := range c.Techs {
			out.Techs[k] = CloneTechSpec(ts)
		}
	}
	if c.DefaultWorldLaws != nil {
		out.DefaultWorldLaws = cloneIntMapPreserveNil(c.DefaultWorldLaws)
	}
	return out
}

func CloneFactions(factions []Faction) []Faction {
	out := make([]Faction, len(factions))
	for i, f := range factions {
		fc := f
		fc.Professions = append([]string(nil), f.Professions...)
		fc.Members = append([]string(nil), f.Members...)
		if f.Requirements != nil {
			fc.Requirements = make(map[string]int, len(f.Requirements))
			for k, v := range f.Requirements {
				fc.Requirements[k] = v
			}
		}
		out[i] = fc
	}
	return out
}

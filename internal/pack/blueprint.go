package pack

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Blueprint is a compact, LLM-friendly world specification (~100-250 lines JSON) that compiles
// into a strictly validated, conservation-balanced WorldPack with automatic survival scaffolding.
type Blueprint struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Genre         string                    `json:"genre"`
	Summary       string                    `json:"summary,omitempty"`
	CurrencyName  string                    `json:"currency_name,omitempty"`
	ActorNoun     string                    `json:"actor_noun,omitempty"`
	AuthorityName string                    `json:"authority_name,omitempty"`
	OversightName string                    `json:"oversight_name,omitempty"`
	FoodItemID    string                    `json:"food_item_id,omitempty"`
	FoodItemName  string                    `json:"food_item_name,omitempty"`
	WaterItemID   string                    `json:"water_item_id,omitempty"`
	WaterItemName string                    `json:"water_item_name,omitempty"`
	SupplyDays    int                       `json:"supply_days,omitempty"`
	Institutions   *sim.InstitutionToggles `json:"institutions,omitempty"`
	Forms          []BlueprintForm         `json:"forms,omitempty"`
	Skills         []sim.SkillSpec         `json:"skills,omitempty"`
	StateSlots     []sim.StateSlotSpec     `json:"state_slots,omitempty"`
	ActionSpecs    []sim.ActionSpec        `json:"action_specs,omitempty"`
	CombatProfiles []sim.CombatProfileSpec `json:"combat_profiles,omitempty"`
	Currencies     []sim.CurrencySpec      `json:"currencies,omitempty"`
	GoldenFingers  []sim.GoldenFingerSpec  `json:"golden_fingers,omitempty"`
	Transitions    []sim.TransitionSpec    `json:"transitions,omitempty"`
	CustomItems   []BlueprintItem           `json:"custom_items,omitempty"`
	Districts     []BlueprintDistrict       `json:"districts"`
	Roads         []BlueprintRoad           `json:"roads,omitempty"`
	Factions      []BlueprintFaction        `json:"factions,omitempty"`
	Actors        []BlueprintActor          `json:"actors"`
	Mentorships   []sim.MentorshipBond      `json:"mentorships,omitempty"`
	Kinships      []sim.ActorKinship        `json:"kinships,omitempty"`
	Grudges       []BlueprintGrudge         `json:"grudges,omitempty"`
	Diplomacy     []BlueprintDiplomacy      `json:"diplomacy,omitempty"`
	Secrets       []BlueprintSecret         `json:"secrets,omitempty"`
	WorldEvents   []BlueprintWorldEvent     `json:"world_events,omitempty"`
}

type BlueprintForm struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	PowerTier      int      `json:"power_tier"`
	CombatBonus    int      `json:"combat_bonus,omitempty"`
	VigilanceBonus int      `json:"vigilance_bonus,omitempty"`
	InnateTags     []string `json:"innate_tags,omitempty"`
}

type BlueprintItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tier           int    `json:"tier,omitempty"`
	Edible         bool   `json:"edible,omitempty"`
	Potable        bool   `json:"potable,omitempty"`
	Tradeable      *bool  `json:"tradeable,omitempty"`
	BatchPrice     int    `json:"batch_price,omitempty"`
	NutritionUnits int64  `json:"nutrition_units,omitempty"`
	MassUnits      int64  `json:"mass_units,omitempty"`
	CombatBonus    int    `json:"combat_bonus,omitempty"`
	EquipSlot      string `json:"equip_slot,omitempty"`
}

type BlueprintDistrict struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Terrain            string   `json:"terrain,omitempty"`
	Features           []string `json:"features,omitempty"`
	ControllingFaction string   `json:"controlling_faction,omitempty"`
}

type BlueprintRoad struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Ticks      int    `json:"ticks,omitempty"`
	EnergyCost *int   `json:"energy_cost,omitempty"`
	Medium     string `json:"medium,omitempty"`
}

type BlueprintFaction struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	District string `json:"district,omitempty"`
	LeaderID string `json:"leader_id,omitempty"`
	Level    int    `json:"level,omitempty"`
}

type BlueprintAmbition struct {
	Kind        string `json:"kind"` // accumulate_wealth | ascend_tier | avenge_blood | expose_corrupt | seize_leadership | master_skill
	TargetID    string `json:"target_id,omitempty"`
	TargetValue int    `json:"target_value,omitempty"`
}

type BlueprintActor struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Role        string              `json:"role"`
	Location    string              `json:"location"`
	Faction     string              `json:"faction,omitempty"`
	Factions    []string            `json:"factions,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Personality string              `json:"personality,omitempty"`
	Hobbies     []string            `json:"hobbies,omitempty"`
	FormID      string              `json:"form_id,omitempty"`
	Attributes  *sim.Attributes     `json:"attributes,omitempty"`
	AgeYears    int                 `json:"age_years,omitempty"`
	Money       *int                `json:"money,omitempty"`
	Goal        string              `json:"goal,omitempty"`
	Trust       map[string]int      `json:"trust,omitempty"`
	Items       map[string]int      `json:"items,omitempty"`
	Skills      map[string]int      `json:"skills,omitempty"`
	StateSlots  map[string]int      `json:"state_slots,omitempty"`
	Ambitions   []BlueprintAmbition `json:"ambitions,omitempty"`
	Memories    []sim.Memory        `json:"memories,omitempty"`
	PinFocal    *bool               `json:"pin_focal,omitempty"`
}

type BlueprintGrudge struct {
	AggrievedID string `json:"aggrieved_id"`
	OffenderID  string `json:"offender_id"`
	Cause       string `json:"cause"`
	Intensity   int    `json:"intensity,omitempty"`
}

type BlueprintDiplomacy struct {
	FactionA      string `json:"faction_a"`
	FactionB      string `json:"faction_b"`
	Stance        string `json:"stance"` // allied | friendly | neutral | hostile | at_war
	RelationScore int    `json:"relation_score,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type BlueprintSecret struct {
	OwnerID   string   `json:"owner_id"`
	Kind      string   `json:"kind"` // illicit_gain | secret_vendetta | covert_bribe | forbidden_hoard | hidden_plot
	SubjectID string   `json:"subject_id,omitempty"`
	Severity  int      `json:"severity,omitempty"`
	KnownBy   []string `json:"known_by,omitempty"`
}

type BlueprintWorldEvent struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Category           string   `json:"category"` // artifact_emergence | terrain_shift | external_invasion
	TargetFeatures     []string `json:"target_features,omitempty"`
	ProbPermillePerDay *int     `json:"prob_permille_per_day,omitempty"`
	MinDay             int      `json:"min_day,omitempty"`
	CooldownDays       int      `json:"cooldown_days,omitempty"`
	DurationDays       int      `json:"duration_days,omitempty"`
	Intensity          int      `json:"intensity,omitempty"`
	RewardItem         string   `json:"reward_item,omitempty"`
	RewardQty          *int     `json:"reward_qty,omitempty"`
	RewardCoins        int      `json:"reward_coins,omitempty"`
	RequiredPowerTier  int      `json:"required_power_tier,omitempty"`
	CasualtyPermille   int      `json:"casualty_permille,omitempty"`
	Damage             int      `json:"damage,omitempty"`
}

// LoadBlueprintFile decodes a Blueprint JSON file with strict unknown-field checking.
func LoadBlueprintFile(path string) (*Blueprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open blueprint %q: %w", path, err)
	}
	defer f.Close()
	return DecodeBlueprint(f)
}

// DecodeBlueprint parses a Blueprint JSON stream, rejecting unknown fields so LLMs get immediate feedback.
func DecodeBlueprint(r io.Reader) (*Blueprint, error) {
	var bp Blueprint
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bp); err != nil {
		return nil, fmt.Errorf("decode blueprint JSON: %w", err)
	}
	return &bp, nil
}

// CompileBlueprint converts a compact Blueprint into a fully populated, conservation-valid WorldPack.
func CompileBlueprint(bp Blueprint) (*WorldPack, error) {
	if strings.TrimSpace(bp.ID) == "" {
		return nil, fmt.Errorf("blueprint.id is required")
	}
	if len(bp.Districts) == 0 {
		return nil, fmt.Errorf("blueprint must define at least one district")
	}
	if len(bp.Actors) == 0 {
		return nil, fmt.Errorf("blueprint must define at least one actor")
	}

	name := strings.TrimSpace(bp.Name)
	if name == "" {
		name = bp.ID
	}
	genre := strings.TrimSpace(bp.Genre)
	if genre == "" {
		genre = "custom"
	}
	currency := strings.TrimSpace(bp.CurrencyName)
	if currency == "" {
		currency = "钱币"
	}
	actorNoun := strings.TrimSpace(bp.ActorNoun)
	if actorNoun == "" {
		actorNoun = "人物"
	}
	authority := strings.TrimSpace(bp.AuthorityName)
	if authority == "" {
		authority = "执法机构"
	}
	oversight := strings.TrimSpace(bp.OversightName)
	if oversight == "" {
		oversight = "监察机构"
	}
	foodID := strings.TrimSpace(bp.FoodItemID)
	if foodID == "" {
		foodID = "ration"
	}
	foodName := strings.TrimSpace(bp.FoodItemName)
	if foodName == "" {
		foodName = "口粮"
	}
	waterID := strings.TrimSpace(bp.WaterItemID)
	if waterID == "" {
		waterID = "drinking_water"
	}
	waterName := strings.TrimSpace(bp.WaterItemName)
	if waterName == "" {
		waterName = "饮水"
	}
	supplyDays := bp.SupplyDays
	if supplyDays <= 0 {
		supplyDays = 180
	}

	// 1. Environment & Catalog scaffolding.
	env := &sim.EnvironmentProfile{
		Genre:           genre,
		Theme:           name,
		Summary:         bp.Summary,
		CurrencyName:    currency,
		ActorNoun:       actorNoun,
		AuthorityName:   authority,
		OversightName:   oversight,
		ForumFeatures:   []string{"public_forum", "authority_seat", "market", "hub"},
		EnforcementTags: []string{"enforcer", "law"},
		OversightTags:   []string{"elder_oversight", "censor"},
		OutlawTags:      []string{"outlaw", "raider", "thief"},
	}
	if bp.Institutions != nil {
		inst := *bp.Institutions
		env.Institutions = &inst
	}

	tagActions := map[string][]string{
		"resident": {
			"equip", "unequip", "claim_world_event",
			"form_party", "join_party", "leave_party", "betray_party",
			"propose_alliance", "declare_war", "make_peace",
			"set_ambition", "investigate_actor", "blackmail", "expose_secret",
			"attempt_transition", "transmit_message",
			"wait", "move", "move_venue", "enter_house", "leave_house",
			"sleep", "begin_rest", "end_rest", "eat", "drink", "relieve", "medicate",
			"speak", "join_faction", "marry", "swear_bond", "mediate_feud",
			"offer", "accept", "reject", "heal", "spar", "attack", "avenge_feud",
			"bribe", "practice_skill", "teach_knowledge",
		},
		"trader":          {"propose_agreement", "accept_agreement", "reject_agreement", "fulfill_agreement", "offer", "accept", "reject", "borrow_loan", "repay_loan", "accept_loan", "reject_loan"},
		"fighter":         {"attack", "spar", "avenge_feud", "claim_turf", "hunt_threat"},
		"enforcer":        {"arrest", "audit_corruption", "hunt_threat", "claim_turf"},
		"law":             {"arrest", "audit_corruption", "hunt_threat", "claim_turf"},
		"elder_oversight": {"audit_corruption", "file_appeal", "issue_decree"},
		"censor":          {"audit_corruption", "file_appeal"},
		"ruler":           {"issue_decree", "propose_alliance", "declare_war", "make_peace", "claim_turf"},
		"commander":       {"propose_alliance", "declare_war", "make_peace", "attack", "claim_turf"},
		"outlaw":          {"steal", "raid", "extort", "bribe", "attack", "avenge_feud"},
		"healer":          {"heal", "medicate"},
	}

	items := map[string]sim.ItemSpec{
		foodID: {
			ID: foodID, Name: foodName, Tier: 1, Edible: true, Tradeable: true,
			MassUnits: 1, NutritionUnits: 1, BatchQty: 2, BatchPrice: 2,
			OfferThreshold: 12, SurplusThreshold: 18, ShortageThreshold: 6,
		},
		waterID: {
			ID: waterID, Name: waterName, Tier: 1, Potable: true, ThirstRelief: 2, Tradeable: true,
			MassUnits: 1, BatchQty: 2, BatchPrice: 1,
			OfferThreshold: 12, SurplusThreshold: 18, ShortageThreshold: 6,
		},
		"medicine_pack": {
			ID: "medicine_pack", Name: "疗伤药剂", Tier: 2, Tradeable: true,
			MedicinalHeal: 25, DiseaseCure: 1,
			MassUnits: 1, BatchQty: 1, BatchPrice: 6, OfferThreshold: 4, SurplusThreshold: 6,
		},
	}
	for _, ci := range bp.CustomItems {
		if strings.TrimSpace(ci.ID) == "" {
			continue
		}
		mass := ci.MassUnits
		if mass <= 0 {
			mass = 1
		}
		tier := ci.Tier
		if tier <= 0 {
			tier = 2
		}
		price := ci.BatchPrice
		if price <= 0 {
			price = tier * 5
		}
		nm := ci.Name
		if nm == "" {
			nm = ci.ID
		}
		tradeable := true
		if ci.Tradeable != nil {
			tradeable = *ci.Tradeable
		}
		items[ci.ID] = sim.ItemSpec{
			ID: ci.ID, Name: nm, Tier: tier,
			Edible: ci.Edible, Potable: ci.Potable,
			Tradeable: tradeable, MassUnits: mass, NutritionUnits: ci.NutritionUnits,
			CombatBonus: ci.CombatBonus, EquipSlot: ci.EquipSlot,
			BatchQty: 1, BatchPrice: price, OfferThreshold: 2, SurplusThreshold: 3,
		}
	}

	forms := map[string]sim.FormSpec{}
	for _, f := range bp.Forms {
		if strings.TrimSpace(f.ID) == "" {
			continue
		}
		tier := f.PowerTier
		if tier <= 0 {
			tier = 1
		}
		nm := f.Name
		if nm == "" {
			nm = f.ID
		}
		forms[f.ID] = sim.FormSpec{
			ID:             f.ID,
			Name:           nm,
			PowerTier:      tier,
			CombatBonus:    f.CombatBonus,
			VigilanceBonus: f.VigilanceBonus,
			InnateTags:     append([]string(nil), f.InnateTags...),
		}
	}

	// Ensure at least one world communication channel so cross-district news & rumors propagate.
	commChannels := map[string]sim.CommChannelSpec{
		"messenger": {
			ID:               "messenger",
			Name:             "驿传讯息",
			RangeKm:          200,
			PenetrationPower: 100,
			EnergyCost:       2,
			Broadcast:        false,
		},
	}

	// Ensure a minimal potable district resource pool so actors don't die of thirst when traveling.
	resources := map[string]sim.ResourcePoolSpec{
		"fresh_water": {
			ID: "fresh_water", Name: "水源",
			RegenPermillePerDay: 100, PotableWater: true,
		},
	}

	// 2. Map: GeoRegion + Districts + connected Roads.
	worldMap := sim.WorldMap{
		GeoRegions: []sim.GeoRegion{
			{ID: "main_region", Name: name, Coord: sim.GeoCoord{X: 0, Y: 0}},
		},
		Districts: make([]sim.District, 0, len(bp.Districts)),
	}
	districtSet := make(map[string]bool, len(bp.Districts))
	relations := sim.NewRelationEvoState()
	for _, d := range bp.Districts {
		if strings.TrimSpace(d.ID) == "" {
			return nil, fmt.Errorf("district has empty id")
		}
		if districtSet[d.ID] {
			return nil, fmt.Errorf("duplicate district id %q", d.ID)
		}
		districtSet[d.ID] = true
		nm := d.Name
		if nm == "" {
			nm = d.ID
		}
		feats := append([]string(nil), d.Features...)
		if len(feats) == 0 {
			feats = []string{"public_forum", "market"}
		}
		worldMap.Districts = append(worldMap.Districts, sim.District{
			ID:          d.ID,
			Name:        nm,
			Terrain:     d.Terrain,
			GeoRegionID: "main_region",
			Features:    feats,
		})
		if d.ControllingFaction != "" {
			relations.ControllingFaction[d.ID] = d.ControllingFaction
		}
	}
	if len(bp.Roads) > 0 {
		for _, r := range bp.Roads {
			ticks := r.Ticks
			if ticks <= 0 {
				ticks = 1
			}
			ecost := 2
			if r.EnergyCost != nil {
				ecost = *r.EnergyCost
				if ecost < 0 {
					ecost = 0
				}
			}
			med := r.Medium
			if med == "" {
				med = "land"
			}
			worldMap.Roads = append(worldMap.Roads, sim.Road{
				From: r.From, To: r.To, Ticks: ticks, EnergyCost: ecost, Medium: med,
			})
		}
	} else if len(worldMap.Districts) > 1 {
		// Auto-connect districts in a clean ring/chain so no actor is ever stranded.
		for i := 0; i < len(worldMap.Districts)-1; i++ {
			worldMap.Roads = append(worldMap.Roads, sim.Road{
				From: worldMap.Districts[i].ID, To: worldMap.Districts[i+1].ID,
				Ticks: 1, EnergyCost: 2, Medium: "land",
			})
		}
		if len(worldMap.Districts) > 2 {
			worldMap.Roads = append(worldMap.Roads, sim.Road{
				From: worldMap.Districts[len(worldMap.Districts)-1].ID, To: worldMap.Districts[0].ID,
				Ticks: 2, EnergyCost: 3, Medium: "land",
			})
		}
	}

	// 3. Factions.
	factions := make([]sim.Faction, 0, len(bp.Factions))
	factionSet := make(map[string]bool, len(bp.Factions))
	for _, f := range bp.Factions {
		if strings.TrimSpace(f.ID) == "" {
			return nil, fmt.Errorf("faction has empty id")
		}
		if factionSet[f.ID] {
			return nil, fmt.Errorf("duplicate faction id %q", f.ID)
		}
		factionSet[f.ID] = true
		nm := f.Name
		if nm == "" {
			nm = f.ID
		}
		dist := f.District
		if dist == "" {
			dist = worldMap.Districts[0].ID
		}
		lv := f.Level
		if lv <= 0 {
			lv = 1
		}
		factions = append(factions, sim.Faction{
			ID:       f.ID,
			Name:     nm,
			District: dist,
			Level:    lv,
		})
		if f.LeaderID != "" {
			relations.FactionLeader[f.ID] = f.LeaderID
		}
	}

	// 4. Actors & default lodging houses + balanced survival food/water stores.
	actors := make([]sim.Actor, 0, len(bp.Actors))
	houses := make([]sim.House, 0, len(bp.Actors))
	foodStock := supplyDays * 3 // 3 nutrition units/day
	waterStock := supplyDays * 2
	for i, ba := range bp.Actors {
		if strings.TrimSpace(ba.ID) == "" {
			return nil, fmt.Errorf("actors[%d] has empty id", i)
		}
		loc := ba.Location
		if loc == "" {
			loc = worldMap.Districts[0].ID
		}
		nm := ba.Name
		if nm == "" {
			nm = ba.ID
		}
		role := ba.Role
		if role == "" {
			role = "resident"
		}
		tags := []string{"resident"}
		for _, t := range ba.Tags {
			if t != "" && !sim.ActorHasTag(tags, t) {
				tags = append(tags, t)
			}
		}
		var guilds []string
		if ba.Faction != "" {
			guilds = append(guilds, ba.Faction)
		}
		for _, g := range ba.Factions {
			if g != "" && !sim.ActorHasTag(guilds, g) {
				guilds = append(guilds, g)
			}
		}
		// Automatically mark declared faction leaders with ruler tag so diplomacy/decrees work.
		for _, f := range bp.Factions {
			if f.LeaderID == ba.ID && !sim.ActorHasTag(tags, "ruler") {
				tags = append(tags, "ruler")
			}
		}

		actorItems := map[string]int{
			foodID:          foodStock,
			waterID:         waterStock,
			"medicine_pack": 2,
		}
		for k, v := range ba.Items {
			if v > 0 {
				actorItems[k] += v
				if _, exists := items[k]; !exists {
					items[k] = sim.ItemSpec{ID: k, Name: k, Tier: 2, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 5}
				}
			}
		}
		money := 50
		if ba.Money != nil {
			money = *ba.Money
			if money < 0 {
				money = 0
			}
		}
		age := ba.AgeYears
		if age <= 0 {
			age = 24 + (i % 20)
		}
		pin := true
		if ba.PinFocal != nil {
			pin = *ba.PinFocal
		}
		var ambitions []sim.ActorAmbition
		for ai, amb := range ba.Ambitions {
			if strings.TrimSpace(amb.Kind) == "" {
				continue
			}
			ambitions = append(ambitions, sim.ActorAmbition{
				ID:          fmt.Sprintf("amb_init_%s_%d", ba.ID, ai+1),
				Kind:        amb.Kind,
				TargetID:    amb.TargetID,
				TargetValue: amb.TargetValue,
				StartedTick: 0,
			})
		}

		houseID := "house_" + ba.ID
		houses = append(houses, sim.House{
			ID:        houseID,
			Name:      nm + "居所",
			District:  loc,
			OwnerID:   ba.ID,
			Residents: []string{ba.ID},
			Level:     1,
			Comfort:   2,
			Condition: 100,
		})

		attrs := sim.Attributes{Strength: 8, Dexterity: 8, Insight: 10, Spirit: 8}
		if sim.ActorHasTag(tags, "fighter") || sim.ActorHasTag(tags, "commander") {
			attrs.Strength = 16
			attrs.Dexterity = 14
		}
		if sim.ActorHasTag(tags, "outlaw") {
			attrs.Strength = maxIntBP(attrs.Strength, 14)
			attrs.Dexterity = maxIntBP(attrs.Dexterity, 15)
		}
		if sim.ActorHasTag(tags, "enforcer") || sim.ActorHasTag(tags, "law") {
			attrs.Strength = maxIntBP(attrs.Strength, 13)
			attrs.Insight = maxIntBP(attrs.Insight, 13)
		}
		if sim.ActorHasTag(tags, "ruler") || sim.ActorHasTag(tags, "elder_oversight") || sim.ActorHasTag(tags, "censor") {
			attrs.Insight = maxIntBP(attrs.Insight, 14)
			attrs.Spirit = maxIntBP(attrs.Spirit, 12)
		}
		if ba.Attributes != nil {
			attrs = *ba.Attributes
		}

		actors = append(actors, sim.Actor{
			ID:          ba.ID,
			Name:        nm,
			Role:        role,
			Tags:        tags,
			Attributes:  attrs,
			Guilds:      guilds,
			Location:    loc,
			HomeID:      houseID,
			AgeYears:    age,
			Health:      100,
			Energy:      100,
			Money:       money,
			Items:       actorItems,
			Skills:      ba.Skills,
			FormID:      ba.FormID,
			StateSlots:  ba.StateSlots,
			Goal:        ba.Goal,
			Personality: ba.Personality,
			Hobbies:     append([]string(nil), ba.Hobbies...),
			Trust:       ba.Trust,
			Memories:    ba.Memories,
			Ambitions:   ambitions,
			PinFocal:    pin,
		})
	}

	// 5. Initial Relations, Diplomacy, Secrets, WorldEvents.
	relations.Mentorships = append([]sim.MentorshipBond(nil), bp.Mentorships...)
	for _, k := range bp.Kinships {
		if k.ActorID == "" {
			continue
		}
		kc := k
		relations.Kinship[k.ActorID] = &kc
	}
	for i, g := range bp.Grudges {
		if g.AggrievedID == "" || g.OffenderID == "" {
			continue
		}
		inten := g.Intensity
		if inten <= 0 {
			inten = 65
		}
		relations.Grudges = append(relations.Grudges, sim.GrudgeRecord{
			ID:          fmt.Sprintf("grudge_init_%d", i+1),
			AggrievedID: g.AggrievedID,
			OffenderID:  g.OffenderID,
			Cause:       g.Cause,
			Intensity:   inten,
		})
	}

	var diplomacy *sim.DiplomacyState
	if len(bp.Diplomacy) > 0 {
		diplomacy = sim.NewDiplomacyState()
		for _, d := range bp.Diplomacy {
			if d.FactionA == "" || d.FactionB == "" || d.FactionA == d.FactionB {
				continue
			}
			fa, fb := d.FactionA, d.FactionB
			if fa > fb {
				fa, fb = fb, fa
			}
			stance := d.Stance
			if stance == "" {
				stance = "neutral"
			}
			score := d.RelationScore
			if score == 0 {
				switch stance {
				case "allied":
					score = 65
				case "friendly":
					score = 30
				case "hostile":
					score = -65
				case "at_war":
					score = -85
				}
			}
			diplomacy.Relations[fa+"|"+fb] = &sim.FactionDiplomacy{
				FactionA:      fa,
				FactionB:      fb,
				Stance:        stance,
				RelationScore: score,
				Reason:        d.Reason,
			}
		}
	}

	var secrets []sim.ActorSecret
	for i, s := range bp.Secrets {
		if s.OwnerID == "" || s.Kind == "" {
			continue
		}
		sev := s.Severity
		if sev <= 0 {
			sev = 65
		}
		secrets = append(secrets, sim.ActorSecret{
			ID:        fmt.Sprintf("sec_init_%d_%s", i+1, s.OwnerID),
			OwnerID:   s.OwnerID,
			Kind:      s.Kind,
			SubjectID: s.SubjectID,
			Severity:  sev,
			KnownBy:   append([]string(nil), s.KnownBy...),
		})
	}

	var worldEvents []sim.WorldEventSpec
	for _, we := range bp.WorldEvents {
		if we.ID == "" || we.Category == "" {
			continue
		}
		if we.RewardItem != "" {
			if _, ok := items[we.RewardItem]; !ok {
				items[we.RewardItem] = sim.ItemSpec{
					ID: we.RewardItem, Name: we.RewardItem, Tier: 4, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 40,
				}
			}
		}
		prob := 80
		if we.ProbPermillePerDay != nil {
			prob = *we.ProbPermillePerDay
			if prob < 0 {
				prob = 0
			}
		}
		rewardQty := 0
		if we.RewardQty != nil {
			rewardQty = *we.RewardQty
			if rewardQty < 0 {
				rewardQty = 0
			}
		} else if we.RewardItem != "" {
			rewardQty = 1
		}
		durDays := we.DurationDays
		if durDays <= 0 {
			durDays = 5
		}
		inten := we.Intensity
		if inten <= 0 {
			inten = 70
		}
		worldEvents = append(worldEvents, sim.WorldEventSpec{
			ID:                 we.ID,
			Name:               we.Name,
			Category:           we.Category,
			TargetFeatures:     append([]string(nil), we.TargetFeatures...),
			ProbPermillePerDay: prob,
			MinTick:            int64(maxIntBP(0, we.MinDay-1)) * sim.TicksPerDay,
			CooldownTicks:      int64(maxIntBP(0, we.CooldownDays)) * sim.TicksPerDay,
			DurationTicks:      int64(durDays) * sim.TicksPerDay,
			Intensity:          inten,
			RewardItem:         we.RewardItem,
			RewardQty:          rewardQty,
			RewardCoins:        we.RewardCoins,
			RequiredPowerTier:  we.RequiredPowerTier,
			CasualtyPermille:   we.CasualtyPermille,
			Damage:             we.Damage,
		})
	}

	// Ensure deterministic district states with abundant water.
	distStates := make(map[string]sim.DistrictState, len(worldMap.Districts))
	for _, d := range worldMap.Districts {
		distStates[d.ID] = sim.DistrictState{
			DistrictID:   d.ID,
			Weather:      "clear",
			Sanitation:   80,
			Resources:    map[string]int64{"fresh_water": 50000},
			MaxResources: map[string]int64{"fresh_water": 50000},
		}
	}

	var transitions map[string]sim.TransitionSpec
	if len(bp.Transitions) > 0 {
		transitions = make(map[string]sim.TransitionSpec, len(bp.Transitions))
		for _, tr := range bp.Transitions {
			if tr.ID != "" {
				transitions[tr.ID] = tr
			}
		}
	}

	var skills map[string]sim.SkillSpec
	if len(bp.Skills) > 0 {
		skills = make(map[string]sim.SkillSpec, len(bp.Skills))
		for _, sk := range bp.Skills {
			if sk.ID != "" {
				skills[sk.ID] = sk
			}
		}
	}

	var stateSlots map[string]sim.StateSlotSpec
	if len(bp.StateSlots) > 0 {
		stateSlots = make(map[string]sim.StateSlotSpec, len(bp.StateSlots))
		for _, sl := range bp.StateSlots {
			if sl.ID != "" {
				stateSlots[sl.ID] = sl
			}
		}
	}

	var actionSpecs map[string]sim.ActionSpec
	if len(bp.ActionSpecs) > 0 {
		actionSpecs = make(map[string]sim.ActionSpec, len(bp.ActionSpecs))
		for _, as := range bp.ActionSpecs {
			if as.ID != "" {
				actionSpecs[as.ID] = as
			}
		}
	}

	var combatProfiles []sim.CombatProfileSpec
	if len(bp.CombatProfiles) > 0 {
		combatProfiles = append([]sim.CombatProfileSpec(nil), bp.CombatProfiles...)
	}

	var currencies map[string]sim.CurrencySpec
	if len(bp.Currencies) > 0 {
		currencies = make(map[string]sim.CurrencySpec, len(bp.Currencies))
		for _, cur := range bp.Currencies {
			if cur.ID != "" {
				currencies[cur.ID] = cur
			}
		}
	}

	var goldenFingers []sim.GoldenFingerSpec
	if len(bp.GoldenFingers) > 0 {
		goldenFingers = make([]sim.GoldenFingerSpec, 0, len(bp.GoldenFingers))
		for _, gf := range bp.GoldenFingers {
			if gf.ID != "" {
				goldenFingers = append(goldenFingers, sim.CloneGoldenFingerSpec(gf))
			}
		}
	}

	// Deterministic sort of transitions/forms not needed for map, but keep clean catalog.
	cat := sim.Catalog{
		Environment:    env,
		TagActions:     tagActions,
		Items:          items,
		Resources:      resources,
		CommChannels:   commChannels,
		Forms:          forms,
		Skills:         skills,
		StateSlots:     stateSlots,
		ActionSpecs:    actionSpecs,
		CombatProfiles: combatProfiles,
		Currencies:     currencies,
		GoldenFingers:  goldenFingers,
		Transitions:    transitions,
		WorldEvents:    worldEvents,
		HouseWear:      &sim.HouseWearRule{IntervalTicks: 365 * sim.TicksPerDay, ConditionLoss: 0, Source: "blueprint_disabled_wear"},
	}

	wp := &WorldPack{
		ID:             bp.ID,
		Name:           name,
		Genre:          genre,
		Description:    bp.Summary,
		PublicTreasury: 500,
		Environment:    env,
		Catalog:        cat,
		Map:            worldMap,
		Houses:         houses,
		Factions:       factions,
		Actors:         actors,
		Ecology: &sim.EcologySpec{
			Enabled:        true,
			ScalePop:       maxIntBP(30, len(actors)),
			DistrictStates: distStates,
		},
		Relations: relations,
		Diplomacy: diplomacy,
		Secrets:   secrets,
	}

	// Smoke-validate both schema and full instantiation + conservation before returning!
	w, err := wp.Instantiate("")
	if err != nil {
		return nil, fmt.Errorf("compiled world pack failed validation/instantiation: %w", err)
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		return nil, fmt.Errorf("compiled world pack failed ledger conservation: %w", err)
	}
	_ = sort.Strings
	return wp, nil
}

func maxIntBP(a, b int) int {
	if a > b {
		return a
	}
	return b
}

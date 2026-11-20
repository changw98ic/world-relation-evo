package optiongen

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// CharacterDossier is the complete, self-contained description of one character
// and the world context sent to DeepSeek for offline 30-day option expansion.
type CharacterDossier struct {
	World                WorldContextSummary   `json:"world"`
	Actor                sim.Actor             `json:"actor"`
	Home                 *sim.House            `json:"home,omitempty"`
	OwnedHouses          []sim.House           `json:"owned_houses,omitempty"`
	CurrentDistrict      DistrictSummary       `json:"current_district"`
	ActorFields          []sim.FieldAsset      `json:"actor_fields,omitempty"`
	ActorGrowthBatches   []sim.GrowthBatch     `json:"actor_growth_batches,omitempty"`
	Polities             []PolitySummary       `json:"polities,omitempty"`
	Factions             []FactionSummary      `json:"factions,omitempty"`
	OtherActors          []OtherActorSummary   `json:"other_actors"`
	Districts            []DistrictSummary     `json:"districts"`
	Roads                []sim.Road            `json:"roads,omitempty"`
	Venues               []sim.Venue           `json:"venues,omitempty"`
	Catalog              CatalogSummary        `json:"catalog"`
	EngineActionKinds    []string              `json:"engine_action_kinds"`
	Tick0BaselineOptions []BaselineOptionBrief `json:"tick0_baseline_options,omitempty"`
}

type WorldContextSummary struct {
	ID             string                 `json:"id"`
	Tick           int64                  `json:"tick"`
	TicksPerDay    int                    `json:"ticks_per_day"`
	HorizonDays    int                    `json:"horizon_days"`
	HorizonTicks   int                    `json:"horizon_ticks"`
	Clock          string                 `json:"clock"`
	Year           int                    `json:"year"`
	SeasonID       string                 `json:"season_id"`
	DayOfSeason    int                    `json:"day_of_season"`
	PublicTreasury int64                  `json:"public_treasury"`
	WorldLaws      map[string]int         `json:"world_laws,omitempty"`
	Environment    sim.EnvironmentProfile `json:"environment"`
	Glossaries     WorldGlossaries        `json:"glossaries"`
}

// WorldGlossaries provides localized display names for every ID in the loaded world
// so both prompts and generic viewers require zero hardcoded setting lookup tables.
type WorldGlossaries struct {
	Locations   map[string]string `json:"locations,omitempty"`
	Items       map[string]string `json:"items,omitempty"`
	Professions map[string]string `json:"professions,omitempty"`
	Factions    map[string]string `json:"factions,omitempty"`
	Recipes     map[string]string `json:"recipes,omitempty"`
}

type DistrictSummary struct {
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	GeoRegionID        string           `json:"geo_region_id,omitempty"`
	PolityID           string           `json:"polity_id,omitempty"`
	Terrain            string           `json:"terrain,omitempty"`
	Features           []string         `json:"features,omitempty"`
	Weather            string           `json:"weather,omitempty"`
	Sanitation         int              `json:"sanitation,omitempty"`
	Corruption         int              `json:"corruption,omitempty"`
	RacketPressure     int              `json:"racket_pressure,omitempty"`
	Resources          map[string]int64 `json:"resources,omitempty"`
	ResourceCapacities map[string]int64 `json:"resource_capacities,omitempty"`
	Infrastructure     map[string]int   `json:"infrastructure,omitempty"`
	ThreatActivity     map[string]int   `json:"threat_activity,omitempty"`
	ActiveDisasters    int              `json:"active_disasters,omitempty"`
}

type PolitySummary struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Level               string   `json:"level"`
	DeJureParentID      string   `json:"de_jure_parent_id,omitempty"`
	DeFactoControllerID string   `json:"de_facto_controller_id,omitempty"`
	SeatDistrictID      string   `json:"seat_district_id,omitempty"`
	SeatVenueID         string   `json:"seat_venue_id,omitempty"`
	LeaderActorID       string   `json:"leader_actor_id,omitempty"`
	Treasury            int64    `json:"treasury,omitempty"`
	MarketTaxPermille   int      `json:"market_tax_permille,omitempty"`
	PollTaxPermille     int      `json:"poll_tax_permille,omitempty"`
	Curfew              bool     `json:"curfew,omitempty"`
	EmbargoItems        []string `json:"embargo_items,omitempty"`
	ActorIsLeader       bool     `json:"actor_is_leader,omitempty"`
}

type FactionSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	District      string   `json:"district"`
	Professions   []string `json:"professions,omitempty"`
	Members       []string `json:"members,omitempty"`
	Level         int      `json:"level"`
	Influence     int      `json:"influence"`
	ActorIsMember bool     `json:"actor_is_member,omitempty"`
}

type OtherActorSummary struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Role             string         `json:"role"`
	Tags             []string       `json:"tags,omitempty"`
	Professions      map[string]int `json:"professions,omitempty"`
	Guilds           []string       `json:"guilds,omitempty"`
	Location         string         `json:"location"`
	VenueID          string         `json:"venue_id,omitempty"`
	HomeID           string         `json:"home_id,omitempty"`
	AgeYears         int            `json:"age_years,omitempty"`
	Money            int            `json:"money"`
	Items            map[string]int `json:"items,omitempty"`
	Goal             string         `json:"goal,omitempty"`
	Personality      string         `json:"personality,omitempty"`
	CoLocated        bool           `json:"co_located,omitempty"`
	MyTrustToThem    int            `json:"my_trust_to_them,omitempty"`
	TheirTrustToMe   int            `json:"their_trust_to_me,omitempty"`
	LeadsPolityIDs   []string       `json:"leads_polity_ids,omitempty"`
}

type CatalogSummary struct {
	Items        []ItemSummary        `json:"items"`
	Recipes      []RecipeSummary      `json:"recipes"`
	Professions  []ProfessionSummary  `json:"professions"`
	CommChannels []sim.CommChannelSpec `json:"comm_channels,omitempty"`
}

type ItemSummary struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Tier           int    `json:"tier,omitempty"`
	Edible         bool   `json:"edible,omitempty"`
	Potable        bool   `json:"potable,omitempty"`
	NutritionUnits int64  `json:"nutrition_units,omitempty"`
	MedicinalHeal  int    `json:"medicinal_heal,omitempty"`
	DiseaseCure    int    `json:"disease_cure,omitempty"`
	Tradeable      bool   `json:"tradeable,omitempty"`
	BatchQty       int    `json:"batch_qty,omitempty"`
	BatchPrice     int    `json:"batch_price,omitempty"`
}

type RecipeSummary struct {
	ID                 string         `json:"id"`
	Label              string         `json:"label"`
	Profession         string         `json:"profession,omitempty"`
	RequiredTag        string         `json:"required_tag,omitempty"`
	MinProfessionLevel int            `json:"min_profession_level,omitempty"`
	NodeFeature        string         `json:"node_feature,omitempty"`
	HouseFacility      string         `json:"house_facility,omitempty"`
	ResourcePool       string         `json:"resource_pool,omitempty"`
	Ticks              int            `json:"ticks"`
	GrowthDays         int            `json:"growth_days,omitempty"`
	Inputs             map[string]int `json:"inputs,omitempty"`
	OutputItem         string         `json:"output_item"`
	OutputQty          int            `json:"output_qty"`
}

type ProfessionSummary struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Tier             int      `json:"tier,omitempty"`
	Guild            string   `json:"guild,omitempty"`
	ParentProfession string   `json:"parent_profession,omitempty"`
	MinParentLevel   int      `json:"min_parent_level,omitempty"`
	Ranks            []string `json:"ranks,omitempty"`
}

type BaselineOptionBrief struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	TargetID string `json:"target_id,omitempty"`
	Location string `json:"location,omitempty"`
	RecipeID string `json:"recipe_id,omitempty"`
	Item     string `json:"item,omitempty"`
}

// ActorOptionCatalog is the structured 30-day option set returned by DeepSeek for one character.
type ActorOptionCatalog struct {
	ActorID          string           `json:"actor_id"`
	ActorName        string           `json:"actor_name"`
	HorizonDays      int              `json:"horizon_days"`
	CharacterSummary string           `json:"character_summary"`
	CoreDrives       []string         `json:"core_drives"`
	KeyTensions      []string         `json:"key_tensions"`
	Options          []ExpandedOption `json:"options"`
}

type ExpandedOption struct {
	ID                  string           `json:"id"`
	Category            string           `json:"category"`
	Title               string           `json:"title"`
	DayWindow           [2]int           `json:"day_window"`
	Priority            string           `json:"priority"`
	WhenIf              OptionCondition  `json:"when_if"`
	Action              OptionActionSpec `json:"action"`
	ExpectedEffects     []string         `json:"expected_effects"`
	PotentialCollisions []string         `json:"potential_collisions"`
	FollowUpHooks       []string         `json:"follow_up_hooks"`
}

type OptionCondition struct {
	Conditions   []string `json:"conditions"`
	TriggerEvent string   `json:"trigger_event,omitempty"`
}

func (c *OptionCondition) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '[' {
		var list []string
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		c.Conditions = list
		return nil
	}
	if trimmed[0] == '"' {
		var single string
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return err
		}
		if single != "" {
			c.Conditions = []string{single}
		}
		return nil
	}
	type alias OptionCondition
	var out alias
	if err := json.Unmarshal(trimmed, &out); err != nil {
		return err
	}
	*c = OptionCondition(out)
	return nil
}

type OptionActionSpec struct {
	EngineKind       string         `json:"engine_kind"`
	RecipeID         string         `json:"recipe_id,omitempty"`
	TargetActorIDs   []string       `json:"target_actor_ids,omitempty"`
	TargetLocation   string         `json:"target_location,omitempty"`
	ItemsOrResources map[string]any `json:"items_or_resources,omitempty"`
	Summary          string         `json:"summary"`
}

// ActorExpansionRecord stores the full audit trail for one actor's DS call.
type ActorExpansionRecord struct {
	WorldID          string             `json:"world_id"`
	ActorID          string             `json:"actor_id"`
	ActorName        string             `json:"actor_name"`
	Model            string             `json:"model"`
	Endpoint         string             `json:"endpoint"`
	RequestedAt      time.Time          `json:"requested_at"`
	DurationMS       int64              `json:"duration_ms"`
	PromptTokens     int                `json:"prompt_tokens,omitempty"`
	CompletionTokens int                `json:"completion_tokens,omitempty"`
	TotalTokens      int                `json:"total_tokens,omitempty"`
	Dossier          CharacterDossier   `json:"dossier"`
	Catalog          ActorOptionCatalog `json:"catalog"`
	RawResponse      string             `json:"raw_response"`
}

// WorldExpansionSummary aggregates results across all characters in a world.
type WorldExpansionSummary struct {
	WorldID        string                 `json:"world_id"`
	PackPath       string                 `json:"pack_path,omitempty"`
	HorizonDays    int                    `json:"horizon_days"`
	Model          string                 `json:"model"`
	Endpoint       string                 `json:"endpoint"`
	StartedAt      time.Time              `json:"started_at"`
	CompletedAt    time.Time              `json:"completed_at"`
	TotalActors    int                    `json:"total_actors"`
	TotalRequests  int                    `json:"total_requests"`
	TotalOptions   int                    `json:"total_options"`
	TotalTokens    int                    `json:"total_tokens"`
	Actors         []ActorExpansionBrief  `json:"actors"`
}

type ActorExpansionBrief struct {
	ActorID       string         `json:"actor_id"`
	ActorName     string         `json:"actor_name"`
	Role          string         `json:"role"`
	Location      string         `json:"location"`
	OptionCount   int            `json:"option_count"`
	ByCategory    map[string]int `json:"by_category"`
	DurationMS    int64          `json:"duration_ms"`
	TotalTokens   int            `json:"total_tokens,omitempty"`
	OutputFile    string         `json:"output_file"`
	NewPrimitives []string       `json:"new_primitives,omitempty"`
}

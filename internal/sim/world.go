package sim

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
)

// Observation is a first-hand material outcome witnessed by the acting character, co-located observer, or bounded sensory scan.
type Observation struct {
	Kind      string `json:"kind"`
	Modality  string `json:"modality,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	ObjectID  string `json:"object_id,omitempty"`
	CaseID    int64  `json:"case_id,omitempty"`
	Item      string `json:"item"`
	Location  string `json:"location"`
	Quantity  int    `json:"quantity"`
}

// Statement is a speaker's structured assertion inside speech (never an objective world fact).
type Statement struct {
	Topic     string `json:"topic"`
	SubjectID string `json:"subject_id,omitempty"`
	ObjectID  string `json:"object_id,omitempty"`
	CaseID    int64  `json:"case_id,omitempty"`
	Item      string `json:"item"`
	Location  string `json:"location"`
	Quantity  int    `json:"quantity"`
}

type Memory struct {
	search       *memorySearchCache
	BasisEventID int64        `json:"basis_event_id,omitempty"`
	RootEventID  int64        `json:"root_event_id"`
	Observation  *Observation `json:"observation,omitempty"`
	Statement    *Statement   `json:"statement,omitempty"`
	Kind         string       `json:"kind"`
	Channel      string       `json:"channel,omitempty"`
	Tick         int64        `json:"tick"`
	EventID      int64        `json:"event_id"`
	Source       string       `json:"source"`
	SourceRole   string       `json:"source_role,omitempty"`
	SourceTags   []string     `json:"source_tags,omitempty"`
	TargetID     string       `json:"target_id,omitempty"`
	Text         string       `json:"text"`
}

type Offer struct {
	ProposedBy string `json:"proposed_by,omitempty"`
	PreviousID int64  `json:"previous_id,omitempty"`
	ID         int64  `json:"id"`
	Seller     string `json:"seller"`
	Buyer      string `json:"buyer"`
	Item       string `json:"item"`
	Quantity   int    `json:"quantity"`
	Price      int    `json:"price"`
	Currency   string `json:"currency,omitempty"`
	Expires    int64  `json:"expires"`
}

// Task tracks a multi-tick action currently in progress so busy actors advance deterministically across quarter-hours.
type Task struct {
	Action           Action `json:"action"`
	TotalTicks       int    `json:"total_ticks"`
	Remaining        int    `json:"remaining_ticks"`
	WorkCredit       int    `json:"work_credit,omitempty"` // hundredths of a crafting work tick
	StartedTick      int64  `json:"started_tick"`
	Yield            int    `json:"yield,omitempty"`
	FundedHundredths int64  `json:"funded_hundredths,omitempty"`
	FieldID          string `json:"field_id,omitempty"`
	SleepTenths      int    `json:"sleep_tenths,omitempty"`
	StartEnergy      int    `json:"start_energy,omitempty"`
	Dormant          bool   `json:"dormant,omitempty"`
}

type Actor struct {
	Resting          bool              `json:"resting,omitempty"`
	DigestedFood     int64             `json:"digested_food,omitempty"`
	Plan             *PersonalPlan     `json:"plan,omitempty"`
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Role             string            `json:"role"`
	Tags             []string          `json:"tags"`
	Attributes       Attributes        `json:"attributes"`
	Professions      map[string]int    `json:"professions"`
	ProfessionRanks  map[string]string `json:"profession_ranks,omitempty"`
	ProfessionXP     map[string]int    `json:"profession_xp,omitempty"`
	Level            int               `json:"level"`
	Guilds           []string          `json:"guilds,omitempty"`
	GuildLevels      map[string]int    `json:"guild_levels,omitempty"`
	Location         string            `json:"location"`
	VenueID          string            `json:"venue_id,omitempty"`
	InsideHouse      string            `json:"inside_house,omitempty"`
	HomeID           string            `json:"home_id,omitempty"`
	AgeYears         int               `json:"age_years,omitempty"`
	Health           int               `json:"health"`
	Energy           int               `json:"energy"`
	NutritionReserve int64             `json:"nutrition_reserve,omitempty"`
	Hunger           int               `json:"hunger"`
	Thirst           int               `json:"thirst,omitempty"`
	Waste            int               `json:"waste,omitempty"`
	DiseaseID        string            `json:"disease_id,omitempty"`
	DiseaseTicks     int               `json:"disease_ticks,omitempty"`
	Dead             bool              `json:"dead,omitempty"`
	WoundedTracked   bool              `json:"wounded_tracked,omitempty"`
	JailedUntil      int64             `json:"jailed_until,omitempty"`
	RestUntil        int64             `json:"rest_until,omitempty"`
	SleepTrait       string            `json:"sleep_trait"`
	Money            int               `json:"money"`
	Currencies       map[string]int64  `json:"currencies,omitempty"`
	IllicitGain      int               `json:"illicit_gain,omitempty"`
	Items            map[string]int    `json:"items"`
	Equipped         map[string]string `json:"equipped,omitempty"`
	PinFocal         bool              `json:"pin_focal,omitempty"`
	FormID           string            `json:"form_id,omitempty"`
	StateSlots       map[string]int    `json:"state_slots,omitempty"`
	Goal             string            `json:"goal"`
	Personality      string            `json:"personality"`
	Trust            map[string]int    `json:"trust"`
	Task             *Task             `json:"task,omitempty"`
	Memories         []Memory          `json:"memories,omitempty"`
}

// IdentityTags returns the actor's social tags plus held profession IDs for economic profile lookups.
func (a Actor) IdentityTags() []string {
	tags := append([]string(nil), a.Tags...)
	profs := make([]string, 0, len(a.Professions))
	for id := range a.Professions {
		profs = append(profs, id)
	}
	sort.Strings(profs)
	return append(tags, profs...)
}

func (a Actor) InTransit() bool {
	return a.Task != nil && (a.Task.Action.Kind == "move" || a.Task.Action.Kind == "move_venue")
}

func SamePlace(a, b Actor) bool {
	if a.Dead || b.Dead || a.InTransit() || b.InTransit() {
		return false
	}
	if a.Location != b.Location || a.InsideHouse != b.InsideHouse {
		return false
	}
	if a.InsideHouse == "" && (a.VenueID != "" || b.VenueID != "") {
		return a.VenueID == b.VenueID
	}
	return true
}

type World struct {
	Production       ProductionState          `json:"production"`
	Growth           []GrowthBatch            `json:"growth,omitempty"`
	Shipments        []Shipment               `json:"shipments,omitempty"`
	FreightUsage     *FreightCapacityUsage    `json:"freight_usage,omitempty"`
	Continuity       *ContinuityState         `json:"continuity,omitempty"`
	Polities         *PolityNetwork           `json:"polities,omitempty"`
	ID               string                   `json:"id"`
	Tick             int64                    `json:"tick"`
	NextID           int64                    `json:"next_id"`
	Map              WorldMap                 `json:"map"`
	Catalog          Catalog                  `json:"catalog"`
	Factions         []Faction                `json:"factions"`
	Houses           []House                  `json:"houses"`
	Actors           []Actor                  `json:"actors"`
	Offers           []Offer                  `json:"offers,omitempty"`
	Cohorts          []PopulationCohort       `json:"cohorts,omitempty"`
	DistrictStates   map[string]DistrictState `json:"district_states,omitempty"`
	DeathsByCause    map[string]int64         `json:"deaths_by_cause,omitempty"`
	TotalBirthsCount int64                    `json:"total_births_count,omitempty"`
	PublicTreasury   int64                    `json:"public_treasury,omitempty"`
	TreasuryLedger   TreasuryLedger           `json:"treasury_ledger,omitempty"`
	MarketCredit     *MarketCreditState       `json:"market_credit,omitempty"`
	Relations        *RelationEvoState        `json:"relations,omitempty"`
	JusticeRevolt    *JusticeRevoltState      `json:"justice_revolt,omitempty"`
	SpotlightRumor   *SpotlightRumorState     `json:"spotlight_rumor,omitempty"`
	ActiveEffects    []ActiveEffect           `json:"active_effects,omitempty"`
	FlowGrid         *FlowGridState           `json:"flow_grid,omitempty"`
	LedgerAudit      *LedgerAuditState        `json:"ledger_audit,omitempty"`
	Knowledge        *KnowledgeBook           `json:"knowledge,omitempty"`
	WorldLaws        map[string]int           `json:"world_laws,omitempty"`
}

type Action struct {
	DueTick      *int64               `json:"due_tick,omitempty"` // Optional delivery deadline; tick zero is a valid deadline.
	AgreementID  int64                `json:"agreement_id,omitempty"`
	BasisEventID int64                `json:"basis_event_id,omitempty"`
	Statement    *Statement           `json:"statement,omitempty"`
	Care         *GrowthCareAgreement `json:"care,omitempty"`
	Kind         string               `json:"kind"`
	Channel      string               `json:"channel,omitempty"`
	TargetID     string               `json:"target_id,omitempty"`
	Location     string               `json:"location,omitempty"`
	HouseID      string               `json:"house_id,omitempty"`
	FactionID    string               `json:"faction_id,omitempty"`
	ProfessionID string               `json:"profession_id,omitempty"`
	RecipeID     string               `json:"recipe_id,omitempty"`
	TechID       string               `json:"tech_id,omitempty"`
	Item         string               `json:"item,omitempty"`
	Quantity     int                  `json:"quantity,omitempty"`
	Price        int                  `json:"price,omitempty"`
	Currency     string               `json:"currency,omitempty"`
	TransitionID string               `json:"transition_id,omitempty"`
	OfferID      int64                `json:"offer_id,omitempty"`
	Text         string               `json:"text,omitempty"`
}

type Event struct {
	ID       int64          `json:"id"`
	Tick     int64          `json:"tick"`
	ActorID  string         `json:"actor_id"`
	TargetID string         `json:"target_id,omitempty"`
	Kind     string         `json:"kind"`
	Action   Action         `json:"action"`
	Payload  map[string]any `json:"payload"`
}

type Neighbor struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Role     string   `json:"role"`
	Tags     []string `json:"tags"`
	Level    int      `json:"level"`
	Health   int      `json:"health"`
	Sleeping bool     `json:"sleeping,omitempty"`
	Jailed   bool     `json:"jailed,omitempty"`
}

type RoadView struct {
	To          string   `json:"to"`
	Ticks       int      `json:"ticks"`
	EnergyCost  int      `json:"energy_cost"`
	LeadsToward []string `json:"leads_toward,omitempty"`
}

// FactionView is the public summary of a guild visible to every actor.
type FactionView struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	District     string         `json:"district"`
	Level        int            `json:"level"`
	Professions  []string       `json:"professions"`
	Requirements map[string]int `json:"requirements,omitempty"`
	Joined       bool           `json:"joined"`
	Members      int            `json:"members"`
}

type CohortView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	District   string `json:"district"`
	Profession string `json:"profession"`
	Population int    `json:"population"`
}

type Perception struct {
	CareAgreements         []GrowthCareAgreement `json:"care_agreements,omitempty"`
	Fields                 []FieldView           `json:"fields,omitempty"`
	EnergyNodes            []FlowNode            `json:"energy_nodes,omitempty"`
	Growth                 []GrowthBatch         `json:"growth,omitempty"`
	Continuity             ContinuityView        `json:"continuity"`
	ActivePlan             *PlanDirectiveView    `json:"active_plan,omitempty"`
	LoanRequests           []LoanRequest         `json:"loan_requests,omitempty"`
	LoanQuotes             []LoanRequest         `json:"loan_quotes,omitempty"`
	Tick                   int64                 `json:"tick"`
	Clock                  Clock                 `json:"clock"`
	Calendar               CalendarState         `json:"calendar"`
	Self                   Actor                 `json:"self"`
	CurrentDistrict        District              `json:"current_district"`
	CurrentVenue           *Venue                `json:"current_venue,omitempty"`
	DistrictState          *DistrictState        `json:"district_state,omitempty"`
	CurrentHouse           *House                `json:"current_house,omitempty"`
	AccessibleHouses       []House               `json:"accessible_houses,omitempty"`
	Factions               []FactionView         `json:"factions,omitempty"`
	Roads                  []RoadView            `json:"roads,omitempty"`
	VenueRoutes            []VenueRouteView      `json:"venue_routes,omitempty"`
	Polity                 *PolityPerceptionView `json:"polity,omitempty"`
	Nearby                 []Neighbor            `json:"nearby,omitempty"`
	LocalCohorts           []CohortView          `json:"local_cohorts,omitempty"`
	ActiveEffects          []ActiveEffect        `json:"active_effects,omitempty"`
	Offers                 []Offer               `json:"offers,omitempty"`
	Interactions           []InteractionView     `json:"interactions,omitempty"`
	OfferBlockReasons      map[int64]string      `json:"offer_block_reasons,omitempty"`
	SpotlightSalience      int                   `json:"spotlight_salience,omitempty"`
	HasActiveLoan          bool                  `json:"has_active_loan,omitempty"`
	OverdueBorrowerID      string                `json:"overdue_borrower_id,omitempty"`
	OverdueHouseID         string                `json:"overdue_house_id,omitempty"`
	Married                bool                  `json:"married,omitempty"`
	EligibleSpouseIDs      []string              `json:"eligible_spouse_ids,omitempty"`
	ActiveGrudgeTargets    []string              `json:"active_grudge_targets,omitempty"`
	MediationActions       []Action              `json:"mediation_actions,omitempty"`
	GrowthCareActions      []Action              `json:"growth_care_actions,omitempty"`
	ExpectedCraftYield     map[string]int        `json:"expected_craft_yield,omitempty"`
	ProductionBlockReasons map[string]string     `json:"production_block_reasons,omitempty"`
	DistrictController     string                `json:"district_controller,omitempty"`
	HasOpenWrongfulCase    bool                  `json:"has_open_wrongful_case,omitempty"`
	AppealTargetID         string                `json:"appeal_target_id,omitempty"`
	AppealBasisEventID     int64                 `json:"appeal_basis_event_id,omitempty"`
	DistrictGrievance      int                   `json:"district_grievance,omitempty"`
	KnownTechs             []string              `json:"known_techs,omitempty"`
	WorldLaws              map[string]int        `json:"world_laws,omitempty"`
	Map                    WorldMap              `json:"-"`
	Catalog                Catalog               `json:"-"`
}

type Decider interface {
	Decide(context.Context, Perception) (Action, error)
}

func seedAttributes(role string, i int) Attributes {
	switch role {
	case "farmer":
		return Attributes{Strength: 9 + i%5, Dexterity: 6 + i%3, Insight: 7 + i%3, Spirit: 8 + i%3}
	case "woodcutter":
		return Attributes{Strength: 10 + i%4, Dexterity: 7 + i%3, Insight: 6 + i%3, Spirit: 8 + i%3}
	case "carpenter":
		return Attributes{Strength: 8 + i%3, Dexterity: 9 + i%4, Insight: 8 + i%3, Spirit: 7 + i%3}
	case "blacksmith":
		return Attributes{Strength: 11 + i%3, Dexterity: 7 + i%3, Insight: 9 + i%3, Spirit: 8 + i%3}
	case "miner":
		return Attributes{Strength: 9 + i%4, Dexterity: 7 + i%3, Insight: 6 + i%3, Spirit: 8 + i%3}
	case "herbalist":
		return Attributes{Strength: 7 + i%3, Dexterity: 7 + i%3, Insight: 10 + i%4, Spirit: 8 + i%3}
	case "healer":
		return Attributes{Strength: 7, Dexterity: 7 + i%3, Insight: 12 + i%2, Spirit: 10 + i%3}
	case "peddler":
		return Attributes{Strength: 7 + i%3, Dexterity: 7 + i%3, Insight: 8 + i%3, Spirit: 9 + i%3}
	case "scholar":
		return Attributes{Strength: 7 + i%2, Dexterity: 8 + i%2, Insight: 11 + i%3, Spirit: 9 + i%3}
	case "guard":
		return Attributes{Strength: 11 + i%3, Dexterity: 8 + i%3, Insight: 7 + i%3, Spirit: 9 + i%3}
	case "martial_artist", "martial_master":
		return Attributes{Strength: 11 + i%3, Dexterity: 10 + i%3, Insight: 8 + i%3, Spirit: 9 + i%3}
	case "constable", "chief_constable":
		return Attributes{Strength: 11 + i%3, Dexterity: 9 + i%3, Insight: 10 + i%3, Spirit: 9 + i%3}
	case "bandit", "bandit_leader":
		return Attributes{Strength: 11 + i%3, Dexterity: 9 + i%3, Insight: 7 + i%3, Spirit: 8 + i%3}
	case "thief", "master_thief":
		return Attributes{Strength: 7 + i%3, Dexterity: 11 + i%3, Insight: 9 + i%3, Spirit: 8 + i%3}
	case "gangster", "gang_boss":
		return Attributes{Strength: 10 + i%3, Dexterity: 8 + i%3, Insight: 8 + i%3, Spirit: 10 + i%3}
	case "herder", "ranch_master":
		return Attributes{Strength: 9 + i%4, Dexterity: 8 + i%3, Insight: 7 + i%3, Spirit: 9 + i%3}
	case "fisher", "fleet_master", "boatman", "canal_master":
		return Attributes{Strength: 9 + i%4, Dexterity: 8 + i%3, Insight: 7 + i%3, Spirit: 9 + i%3}
	case "hunter", "master_hunter":
		return Attributes{Strength: 10 + i%4, Dexterity: 10 + i%3, Insight: 8 + i%3, Spirit: 8 + i%3}
	case "ritualist", "high_priest":
		return Attributes{Strength: 7 + i%2, Dexterity: 7 + i%3, Insight: 10 + i%3, Spirit: 11 + i%3}
	default:
		return Attributes{Strength: 8 + i%4, Dexterity: 8 + i%4, Insight: 8 + i%4, Spirit: 8 + i%4}
	}
}

// CombatPowerWithCatalog computes an actor's combat effectiveness declaratively from physical attributes,
// Catalog profession CombatBonusPerLevel, and Catalog item CombatBonus.
func CombatPowerWithCatalog(cat Catalog, a Actor) int {
	power := a.Attributes.Strength + a.Attributes.Dexterity/2
	for pid, lv := range a.Professions {
		if lv <= 0 {
			continue
		}
		if prof, ok := cat.ProfessionByID(pid); ok && prof.CombatBonusPerLevel > 0 {
			power += lv * prof.CombatBonusPerLevel
		}
	}
	for itemID, qty := range a.Items {
		if qty > 0 {
			if spec, ok := cat.Items[itemID]; ok && spec.EquipSlot == "" && spec.CombatBonus > 0 {
				power += spec.CombatBonus
			}
		}
	}
	for _, itemID := range a.Equipped {
		if itemID != "" {
			if spec, ok := cat.Items[itemID]; ok && spec.EquipSlot != "" && spec.CombatBonus > 0 {
				power += spec.CombatBonus
			}
		}
	}
	return power
}

// CombatPower returns an actor's combat effectiveness against DefaultCatalog.
func CombatPower(a Actor) int {
	return CombatPowerWithCatalog(DefaultCatalog(), a)
}

func (w *World) grantCombatXP(a *Actor, x Action, xp, influence int) []Event {
	for _, prof := range w.Catalog.Professions {
		if prof.Tier == 1 && prof.CombatBonusPerLevel > 0 && a.Professions[prof.ID] > 0 {
			return w.grantProfessionXP(a, x, prof.ID, xp, influence)
		}
	}
	for _, prof := range w.Catalog.Professions {
		if prof.CombatBonusPerLevel > 0 && a.Professions[prof.ID] > 0 {
			return w.grantProfessionXP(a, x, prof.ID, xp, influence)
		}
	}
	return nil
}

func (w *World) effectiveRoadCost(r Road) (int, int) {
	ticks := r.Ticks
	energy := r.EnergyCost
	if w.DistrictStates != nil {
		var tickPen, energyPen int
		for _, ep := range []string{r.From, r.To} {
			if ds, ok := w.DistrictStates[ep]; ok {
				if ws, hasW := w.Catalog.Weathers[ds.Weather]; hasW {
					if ws.TravelTickPenalty > tickPen {
						tickPen = ws.TravelTickPenalty
					}
					if ws.TravelEnergyPenalty > energyPen {
						energyPen = ws.TravelEnergyPenalty
					}
				}
			}
		}
		ticks += tickPen
		energy += energyPen
	}
	if ticks < 1 {
		ticks = 1
	}
	if energy < 0 {
		energy = 0
	}
	return ticks, energy
}

func Village(id string) *World {
	w := &World{
		ID:       id,
		NextID:   1,
		Map:      DefaultMap(),
		Catalog:  DefaultCatalog(),
		Factions: DefaultFactions(),
		Houses: []House{
			{ID: "h01", Name: "West Farmhouse 1", District: "west_cottages", OwnerID: "p01", Residents: []string{"p01", "p02"}, Level: 1, Comfort: 2, Condition: 84, Facilities: []string{"compost_pit"}},
			{ID: "h02", Name: "West Farmhouse 2", District: "west_cottages", OwnerID: "p03", Residents: []string{"p03", "p04"}, Level: 2, Comfort: 2, Condition: 78, Facilities: []string{"compost_pit"}},
			{ID: "h03", Name: "West Farmhouse 3", District: "west_cottages", OwnerID: "p05", Residents: []string{"p05", "p06"}, Level: 1, Comfort: 2, Condition: 74, Facilities: []string{"compost_pit"}},
			{ID: "h04", Name: "West Farmhouse 4", District: "west_cottages", OwnerID: "p07", Residents: []string{"p07", "p08"}, Level: 1, Comfort: 1, Condition: 62, Facilities: []string{"compost_pit"}},
			{ID: "h05", Name: "West Edge Shack", District: "west_cottages", OwnerID: "p09", Residents: []string{"p09"}, Level: 1, Comfort: 1, Condition: 46, Facilities: []string{"compost_pit"}},
			{ID: "h06", Name: "Creekside Cottage 1", District: "creek_well", OwnerID: "p10", Residents: []string{"p10", "p11"}, Level: 2, Comfort: 2, Condition: 86, Facilities: []string{"compost_pit"}},
			{ID: "h07", Name: "Creekside Cottage 2", District: "creek_well", OwnerID: "p12", Residents: []string{"p12", "p13"}, Level: 1, Comfort: 2, Condition: 80, Facilities: []string{"compost_pit"}},
			{ID: "h08", Name: "Creek Woodshed Cabin", District: "creek_well", OwnerID: "p14", Residents: []string{"p14"}, Level: 1, Comfort: 1, Condition: 54},
			{ID: "h09", Name: "East Joinery Yard 1", District: "east_lane", OwnerID: "p15", Residents: []string{"p15", "p16"}, Level: 2, Comfort: 3, Condition: 90, Facilities: []string{"carpentry_bench"}},
			{ID: "h10", Name: "East Joinery Yard 2", District: "east_lane", OwnerID: "p17", Residents: []string{"p17", "p18"}, Level: 2, Comfort: 3, Condition: 88, Facilities: []string{"carpentry_bench"}},
			{ID: "h11", Name: "East Smithy House", District: "east_lane", OwnerID: "p19", Residents: []string{"p19", "p20", "p21"}, Level: 2, Comfort: 2, Condition: 82, Facilities: []string{"forge"}},
			{ID: "h12", Name: "East Herb Courtyard", District: "east_lane", OwnerID: "p22", Residents: []string{"p22", "p23"}, Level: 3, Comfort: 3, Condition: 91, Facilities: []string{"herb_rack"}},
			{ID: "h13", Name: "Crossroads Timber Hut", District: "crossroads", OwnerID: "p24", Residents: []string{"p24", "p25"}, Level: 1, Comfort: 1, Condition: 48},
			{ID: "h14", Name: "Crossroads Watch Lodge", District: "crossroads", OwnerID: "p26", Residents: []string{"p26", "p29"}, Level: 1, Comfort: 2, Condition: 79},
			{ID: "h15", Name: "Market Apothecary", District: "market_street", OwnerID: "p28", Residents: []string{"p28"}, Level: 3, Comfort: 3, Condition: 94, Facilities: []string{"herb_rack"}},
			{ID: "h16", Name: "Market Merchant House", District: "market_street", OwnerID: "p27", Residents: []string{"p27", "p30"}, Level: 2, Comfort: 3, Condition: 92},
		},
	}

	homeByActor := map[string]House{}
	for _, h := range w.Houses {
		for _, r := range h.Residents {
			homeByActor[r] = h
		}
	}

	names := []string{
		"林禾", "陈谷", "王麦", "李田", "赵耕", "周穗", "吴苗", "郑稻", "孙粟", "钱垄",
		"冯耘", "褚桑", "卫豆", "蒋黍", "沈稷", "韩仓", "杨米", "朱秤", "秦布", "尤盐",
		"许柴", "何薪", "吕泉", "施陶", "张木", "孔石", "曹鱼", "严药", "华织", "金铁",
	}
	personalities := []string{"careful and candid", "sociable and generous", "ambitious and skeptical"}
	sleepTraits := []string{SleepDeep, SleepEarlyBird, SleepLight, SleepNightOwl}

	for i, name := range names {
		actorID := fmt.Sprintf("p%02d", i+1)
		home := homeByActor[actorID]
		items := map[string]int{"food": 4 + i%3}
		money := 12 + i%5
		energy := 78 + (i*5)%20
		var role, goal string
		var tags []string

		switch {
		case i < 12:
			role = "farmer"
			tags = []string{"resident", "trader", "water_user"}
			switch i % 4 {
			case 0:
				items["seeds"] = 2
				items["water"] = 2
				if i > 0 {
					items["compost"] = 1
				}
				goal = "Walk to south_fields to irrigate and harvest grain, then trade surplus at market_street"
			case 1:
				items["water"] = 2
				items["compost"] = 1
				goal = "Walk to market_street to buy crop seeds, then cultivate grain in south_fields"
			case 2:
				items["seeds"] = 2
				items["compost"] = 1
				goal = "Draw water at creek_well first, then tend grain in south_fields"
			case 3:
				items["seeds"] = 2
				items["water"] = 2
				goal = "Turn compost at home compost_pit first to boost harvest, then farm in south_fields"
			}
			if i == 8 {
				items["plank"] = 1
				goal = "Repair the leaky roof of West Edge Shack with planks, then farm grain"
			}
		case i <= 13:
			role = "woodcutter"
			tags = []string{"resident", "trader"}
			items["food"] = 5
			if i == 12 {
				items["tools"] = 1
			}
			goal = "Hike to north_woods to fell timber, then trade timber to carpenters in east_lane or market_street"
		case i <= 16:
			role = "carpenter"
			tags = []string{"resident", "trader"}
			items["food"] = 3 + i%2
			items["timber"] = 2 * ((i + 1) % 2)
			if i == 14 {
				items["tools"] = 1
			}
			money = 15 + i%3
			goal = "Saw timber into roof planks at home carpentry_bench and trade planks for grain at market_street"
		case i == 17:
			role = "woodcutter"
			tags = []string{"resident", "trader"}
			items["food"] = 4
			items["tools"] = 1
			goal = "Cut timber in north_woods to supply East Artisan Lane workshops and buy food"
		case i <= 19:
			role = "blacksmith"
			tags = []string{"resident", "trader"}
			items["food"] = 4
			if i == 18 {
				items["ore"] = 1
				items["timber"] = 1
			}
			money = 16
			goal = "Gather ore in north_woods, forge durable hand tools at the smithy, and sell tools at market_street"
		case i == 20:
			role = "miner"
			tags = []string{"resident", "trader"}
			items["food"] = 4
			items["ore"] = 2
			goal = "Collect ironstone ore in north_woods and trade with blacksmiths and peddlers"
		case i <= 22:
			role = "herbalist"
			tags = []string{"resident", "trader", "water_user"}
			items["food"] = 4
			if i == 21 {
				items["herb"] = 2
				items["water"] = 1
			} else {
				items["medicine"] = 2
			}
			money = 14
			goal = "Gather herbs in north_woods, draw water at creek_well, brew medicine at home herb_rack, and care for villagers"
		case i == 23:
			role = "gatherer"
			tags = []string{"resident", "trader"}
			items["food"] = 2
			items["plank"] = 1
			goal = "Forage wild food and fell timber in north_woods while keeping Crossroads Timber Hut repaired"
		case i <= 26:
			role = "peddler"
			tags = []string{"resident", "trader"}
			items["food"] = 7 - (i % 2)
			if i == 26 {
				items["plank"] = 2
			}
			money = 18
			goal = "Broker grain, planks, tools, and harvest news across crossroads and market_street"
		case i == 27:
			role = "healer"
			tags = []string{"resident", "trader", "healer", "water_user"}
			items["food"] = 5
			items["medicine"] = 4
			items["herb"] = 2
			money = 16
			goal = "Treat injured villagers at market_street and crossroads, brew medicine, and trade fairly"
		case i == 28:
			role = "scholar"
			tags = []string{"resident", "trader"}
			items["food"] = 5
			items["scroll"] = 1
			money = 16
			goal = "Study and transcribe scrolls at the crossroads academy, advance from scholar to xiucai, and trade scrolls at market_street"
		default:
			role = "guard"
			tags = []string{"resident", "trader", "fighter"}
			items["food"] = 5
			money = 16
			goal = "Patrol market_street, crossroads, and creek_well to keep order and secure daily rations"
		}

		trust := map[string]int{}
		for _, r := range home.Residents {
			if r != actorID {
				trust[r] = 1
			}
		}

		w.Actors = append(w.Actors, Actor{
			ID:          actorID,
			Name:        name,
			Role:        role,
			Tags:        tags,
			Attributes:  seedAttributes(role, i),
			Professions: map[string]int{role: 1 + i%3},
			Location:    home.District,
			InsideHouse: home.ID,
			HomeID:      home.ID,
			AgeYears:    20 + i%26,
			Health:      MaxHealth,
			Energy:      energy,
			Hunger:      0,
			SleepTrait:  sleepTraits[i%len(sleepTraits)],
			Money:       money,
			Items:       items,
			Goal:        goal,
			Personality: personalities[i%len(personalities)],
			Trust:       trust,
		})
	}

	for i := range w.Actors {
		a := &w.Actors[i]
		if prof, ok := w.Catalog.ProfessionByID(a.Role); ok {
			if f := w.faction(prof.Guild); f != nil {
				f.Members = append(f.Members, a.ID)
				a.Guilds = append(a.Guilds, f.ID)
			}
		}
	}
	for i := range w.Factions {
		f := &w.Factions[i]
		influence := 0
		for _, memberID := range f.Members {
			actor := w.actor(memberID)
			for _, profID := range f.Professions {
				influence += 2 * actor.Professions[profID]
			}
		}
		f.Influence = influence
		f.Level = w.Catalog.FactionLevel(influence)
	}
	for i := range w.Actors {
		w.refreshActor(&w.Actors[i])
	}
	w.InitEcology(len(w.Actors))
	w.initVillageFoodCycle()
	w.InitLedgerAudit()
	return w
}

func (w *World) actor(id string) *Actor {
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return &w.Actors[i]
		}
	}
	if c := w.storedActorCohort(id); c != nil {
		for i := range c.Members {
			if c.Members[i].ID == id {
				return &c.Members[i]
			}
		}
	}
	return nil
}

func (w *World) house(id string) *House {
	for i := range w.Houses {
		if w.Houses[i].ID == id {
			return &w.Houses[i]
		}
	}
	return nil
}

func (w *World) faction(id string) *Faction {
	for i := range w.Factions {
		if w.Factions[i].ID == id {
			return &w.Factions[i]
		}
	}
	return nil
}

// refreshActor recomputes the derived character level (sum of profession levels), profession stage titles, and guild standing.
func (w *World) refreshActor(a *Actor) {
	level := 0
	for _, l := range a.Professions {
		level += l
	}
	a.Level = level
	if len(a.Professions) == 0 {
		a.ProfessionRanks = nil
	} else {
		if a.ProfessionRanks != nil {
			clear(a.ProfessionRanks)
		} else {
			a.ProfessionRanks = make(map[string]string, len(a.Professions))
		}
		for profID, lv := range a.Professions {
			if prof, ok := w.Catalog.ProfessionByID(profID); ok {
				a.ProfessionRanks[profID] = prof.RankTitle(lv)
			} else {
				a.ProfessionRanks[profID] = profID
			}
		}
	}
	if len(a.Guilds) == 0 {
		a.GuildLevels = nil
	} else {
		if a.GuildLevels != nil {
			clear(a.GuildLevels)
		} else {
			a.GuildLevels = make(map[string]int, len(a.Guilds))
		}
		for _, guildID := range a.Guilds {
			if f := w.faction(guildID); f != nil {
				a.GuildLevels[guildID] = f.Level
			}
		}
	}
	if len(a.ProfessionXP) == 0 {
		a.ProfessionXP = nil
	}
	SyncActorPlan(a, w.Tick, w.Catalog)
}

// CheckRecipe evaluates the catalog, inventory and location requirements for one batch.
func CheckRecipe(cat Catalog, worldMap WorldMap, currentHouse *House, a Actor, r Recipe) (bool, string) {
	if err := cat.ValidateProductionConversion(r); err != nil {
		return false, err.Error()
	}
	if ok, reason := cat.ProfessionGating(a, r); !ok {
		return false, reason
	}
	if r.HouseFacility != "" {
		if a.InsideHouse == "" || currentHouse == nil || currentHouse.ID != a.InsideHouse || !currentHouse.HasFacility(r.HouseFacility) {
			return false, "recipe " + r.ID + " requires indoor facility: " + r.HouseFacility
		}
	}
	if r.NodeFeature != "" {
		if a.InsideHouse != "" {
			return false, "recipe " + r.ID + " requires stepping outside to district with " + r.NodeFeature
		}
		if a.VenueID != "" {
			if !worldMap.VenueOrDistrictHasFeature(a.VenueID, a.Location, r.NodeFeature) {
				return false, "recipe " + r.ID + " requires venue/district feature: " + r.NodeFeature
			}
		} else if !worldMap.DistrictHasFeature(a.Location, r.NodeFeature) {
			return false, "recipe " + r.ID + " requires district feature: " + r.NodeFeature
		}
	}
	if a.Energy < r.EnergyCost {
		return false, "insufficient energy for " + r.ID
	}
	if a.Money < r.MoneyCost {
		return false, "insufficient money for " + r.ID
	}
	requirements := r.StartRequirements()
	for _, item := range sortedProductionKeys(requirements) {
		qty := requirements[item]
		if a.Items[item] < qty {
			return false, fmt.Sprintf("recipe %s requires %d %s", r.ID, qty, item)
		}
	}
	return true, ""
}

func neighborOrderKey(viewerID string, tick int64, neighborID string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(viewerID))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(tick))
	h.Write(buf[:])
	h.Write([]byte(neighborID))
	return h.Sum64()
}

func cloneIntMap(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	r := make(map[string]int, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

func cloneStringMap(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	r := make(map[string]string, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

func appendUnique(list []string, value string) []string {
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}

func (w *World) View(id string) Perception {
	a := CloneActor(*w.actor(id))
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}

	dist, _ := w.Map.DistrictByID(a.Location)
	if dist.Features != nil {
		dist.Features = append([]string(nil), dist.Features...)
	}
	p := Perception{
		EnergyNodes:     w.localEnergyNodes(a),
		Growth:          w.visibleGrowth(a),
		Tick:            w.Tick,
		Clock:           ClockAt(w.Tick),
		Calendar:        CalendarAt(w.Tick, w.Catalog.Calendar),
		Self:            a,
		CurrentDistrict: dist,
		Map:             CloneMap(w.Map),
		Catalog:         CloneCatalog(w.Catalog),
	}
	p.ActiveEffects = w.perceivedActiveEffects(a, w.ActiveEffects)
	for _, c := range w.Cohorts {
		if c.Population > 0 && c.District == a.Location {
			p.LocalCohorts = append(p.LocalCohorts, CohortView{
				ID:         c.ID,
				Name:       c.Name,
				District:   c.District,
				Profession: c.Profession,
				Population: c.Population,
			})
		}
	}
	if w.DistrictStates != nil {
		if ds, ok := w.DistrictStates[a.Location]; ok {
			dsc := CloneDistrictState(ds)
			p.DistrictState = &dsc
		}
	}

	if a.InsideHouse != "" {
		if h := w.house(a.InsideHouse); h != nil {
			hc := CloneHouse(*h)
			p.CurrentHouse = &hc
		}
	}
	if a.VenueID != "" {
		if v, ok := w.Map.VenueByID(a.VenueID); ok {
			vc := v
			if v.Features != nil {
				vc.Features = append([]string(nil), v.Features...)
			}
			p.CurrentVenue = &vc
		}
	}
	p.Polity = buildPolityPerceptionView(p.Map, ClonePolityNetwork(w.Polities), a)
	p.KnownTechs = w.ActorKnownTechs(id)
	p.WorldLaws = w.EffectiveWorldLaws()
	for _, h := range w.Houses {
		if h.CanAccess(id) {
			p.AccessibleHouses = append(p.AccessibleHouses, CloneHouse(h))
		}
	}
	for _, f := range w.Factions {
		p.Factions = append(p.Factions, FactionView{
			ID:           f.ID,
			Name:         f.Name,
			District:     f.District,
			Level:        f.Level,
			Professions:  append([]string(nil), f.Professions...),
			Requirements: cloneIntMap(f.Requirements),
			Joined:       f.HasMember(id),
			Members:      len(f.Members),
		})
	}

	if a.InsideHouse == "" {
		for _, r := range w.Map.NeighborsOf(a.Location) {
			effTicks, effEnergy := w.effectiveRoadCost(r)
			p.Roads = append(p.Roads, RoadView{
				To:          r.To,
				Ticks:       effTicks,
				EnergyCost:  effEnergy,
				LeadsToward: w.Map.LeadsToward(a.Location, r.To),
			})
		}
		if len(w.Map.Venues) > 0 {
			p.Roads = nil
		}
		p.VenueRoutes = buildVenueRouteViews(p.Map, p.Catalog, w.DistrictStates, w.actorWithEffectTags(a))
	}

	for _, b := range w.Actors {
		if b.ID != id && SamePlace(a, b) {
			p.Nearby = append(p.Nearby, Neighbor{
				ID:       b.ID,
				Name:     b.Name,
				Role:     b.Role,
				Tags:     append([]string(nil), w.neighborEffectiveTags(b)...),
				Level:    b.Level,
				Health:   b.Health,
				Sleeping: b.Task != nil && b.Task.Action.Kind == "sleep",
				Jailed:   b.JailedUntil > w.Tick,
			})
		}
	}
	sort.Slice(p.Nearby, func(i, j int) bool {
		ki := neighborOrderKey(id, w.Tick, p.Nearby[i].ID)
		kj := neighborOrderKey(id, w.Tick, p.Nearby[j].ID)
		if ki != kj {
			return ki < kj
		}
		return p.Nearby[i].ID < p.Nearby[j].ID
	})

	for _, o := range w.Offers {
		if (o.Buyer == id || o.Seller == id) && o.Expires >= w.Tick {
			p.Offers = append(p.Offers, o)
		}
	}
	w.applyPerceptionEvoHints(&p)
	recallPerceptionMemories(&p)
	return p
}

func (w *World) ensureMarketCredit() *MarketCreditState {
	if w.MarketCredit == nil {
		w.MarketCredit = NewMarketCreditState()
	}
	return w.MarketCredit
}

func (w *World) ensureRelations() *RelationEvoState {
	if w.Relations == nil {
		w.Relations = NewRelationEvoState()
	}
	w.Relations.EnsureKinshipSeeded(w)
	return w.Relations
}

func (w *World) ensureJusticeRevolt() *JusticeRevoltState {
	if w.JusticeRevolt == nil {
		w.JusticeRevolt = NewJusticeRevoltState()
	}
	return w.JusticeRevolt
}

func (w *World) ensureSpotlightRumor() *SpotlightRumorState {
	if w.SpotlightRumor == nil {
		w.SpotlightRumor = NewSpotlightRumorState()
	}
	return w.SpotlightRumor
}

// applyPerceptionEvoHints populates read-only tactical context fields on p from active
// evolutionary subsystems without mutating any world state (safe for concurrent view workers).
func (w *World) applyPerceptionEvoHints(p *Perception) {
	var agreements []Agreement
	if w.Continuity != nil {
		agreements = w.Continuity.Agreements
	}
	w.applyPerceptionEvoHintsWithAgreements(p, agreements)
}

func (w *World) applyPerceptionEvoHintsWithAgreements(p *Perception, agreements []Agreement) {
	if p.Polity != nil && p.Polity.LedPolityID != "" {
		actors, cohorts := w.reliefRecipients(w.Polities.NodeByID(p.Polity.LedPolityID), p.Self.ID)
		p.Polity.ReliefGrantAvailable = len(actors) > 0 || len(cohorts) > 0
	}
	p.ExpectedCraftYield = map[string]int{}
	for _, recipe := range w.Catalog.Recipes {
		if recipe.Growth == nil {
			whole, _ := w.previewNonBiologicalYield("actor", p.Self.ID, p.Self.Location, recipe)
			p.ExpectedCraftYield[recipe.ID] = int(whole)
		}
	}
	for _, field := range w.Production.Fields {
		if field.UserKind == "actor" && field.UserID == p.Self.ID {
			p.Fields = append(p.Fields, FieldView{FieldAsset: field, AvailableAreaM2: w.availableFieldArea(field)})
		}
	}
	p.OfferBlockReasons = make(map[int64]string)
	for _, offer := range p.Offers {
		if offer.ResponderID() == p.Self.ID {
			if buyer := w.actor(offer.Buyer); buyer != nil {
				if reason := w.offerSettlementBlockReason(*buyer, offer); reason != "" {
					p.OfferBlockReasons[offer.ID] = reason
				}
			}
		}
	}
	if p == nil {
		return
	}
	p.Self.Plan = ClonePersonalPlan(p.Self.Plan)
	SyncActorPlan(&p.Self, w.Tick, w.Catalog)
	p.Continuity = w.continuityForAgreements(p.Self.ID, agreements)
	p.Interactions = BuildInteractionViews(*p)
	a := p.Self
	for _, g := range p.Growth {
		if terms := w.careAgreement(g); terms != nil {
			p.CareAgreements = append(p.CareAgreements, *terms)
		}
	}
	p.GrowthCareActions = w.growthCareActions(a)
	p.ProductionBlockReasons = w.ProductionStartBlockReasons(a)
	p.ActivePlan = BuildPlanDirective(*p)
	p.LoanRequests = w.MarketCredit.PendingLoanRequests(a.ID)
	spec := DefaultMarketCreditSpec()
	if w.MarketCredit != nil {
		spec = EffectiveMarketCreditSpec(w.MarketCredit.Spec)
	}
	for _, nb := range p.Nearby {
		if lender := w.actor(nb.ID); lender != nil && !nb.Jailed {
			if amount := QuotePersonalLoan(a, *lender); amount > 0 {
				collateral := ""
				if h := w.house(a.HomeID); h != nil && h.OwnerID == a.ID {
					collateral = h.ID
				}
				p.LoanQuotes = append(p.LoanQuotes, LoanRequest{BorrowerID: a.ID, LenderID: lender.ID, Principal: amount, RatePermille: spec.DailyLoanInterestPermille, TermTicks: spec.LoanTermTicks, CollateralHouse: collateral})
			}
		}
	}
	if w.SpotlightRumor != nil && w.SpotlightRumor.ActorSalience != nil {
		p.SpotlightSalience = w.SpotlightRumor.ActorSalience[a.ID]
	}
	if w.MarketCredit != nil {
		for _, l := range w.MarketCredit.Loans {
			if l.Status != "active" {
				continue
			}
			if l.BorrowerID == a.ID {
				p.HasActiveLoan = true
			}
			if p.OverdueBorrowerID == "" && l.LenderID == a.ID && w.Tick >= l.DueTick && l.Principal+l.AccruedInterest-l.Repaid > 0 {
				if l.CollateralHouse != "" {
					borrower := w.actor(l.BorrowerID)
					borrowerCohort := w.findCohort(l.BorrowerCohortID)
					borrowerAlive := (borrower != nil && !borrower.Dead) || (borrower == nil && borrowerCohort != nil && borrowerCohort.Population > 0)
					if h := w.house(l.CollateralHouse); h != nil && borrowerAlive && h.OwnerID == l.BorrowerID {
						p.OverdueBorrowerID = l.BorrowerID
						p.OverdueHouseID = h.ID
					}
				}
			}
		}
	}
	if w.Relations != nil {
		p.DistrictController = w.Relations.DistrictController(a.Location)
		kA := w.Relations.Kinship[a.ID]
		if kA != nil && kA.SpouseID != "" {
			p.Married = true
		} else if len(p.Nearby) > 0 {
			for _, nb := range p.Nearby {
				if nb.Jailed {
					continue
				}
				kB := w.Relations.Kinship[nb.ID]
				if kB != nil && kB.SpouseID != "" {
					continue
				}
				if kA != nil && (ActorHasTag(kA.ParentIDs, nb.ID) || ActorHasTag(kA.ChildIDs, nb.ID) || ActorHasTag(kA.SwornIDs, nb.ID)) {
					continue
				}
				p.EligibleSpouseIDs = append(p.EligibleSpouseIDs, nb.ID)
			}
		}
		p.MediationActions = w.mediationActions(a)
		for _, g := range w.Relations.Grudges {
			if g.Resolved {
				continue
			}
			if g.AggrievedID == a.ID && g.OffenderID != "" {
				p.ActiveGrudgeTargets = appendUnique(p.ActiveGrudgeTargets, g.OffenderID)
			}
		}
	}
	if w.JusticeRevolt != nil {
		p.DistrictGrievance = w.JusticeRevolt.Grievance(a.Location)
		leads := ActorKnownWrongfulLeads(a)
		if len(leads) > 0 {
			for i := len(w.JusticeRevolt.CourtCases) - 1; i >= 0; i-- {
				cc := w.JusticeRevolt.CourtCases[i]
				if cc.Status != "convicted" || !cc.Wrongful || cc.District != a.Location {
					continue
				}
				if lead, ok := matchKnownLeadToCourtCase(leads, cc); ok {
					p.HasOpenWrongfulCase = true
					p.AppealTargetID = cc.SuspectID
					p.AppealBasisEventID = lead.BasisEventID
					break
				}
			}
		}
	}
}

// KnownWrongfulLead represents a wrongful conviction that an Actor has personally witnessed
// or heard about through interpersonal speech, with its citable BasisEventID.
type KnownWrongfulLead struct {
	BasisEventID int64  `json:"basis_event_id"`
	CaseID       int64  `json:"case_id,omitempty"`
	SuspectID    string `json:"suspect_id,omitempty"`
	MagistrateID string `json:"magistrate_id,omitempty"`
	District     string `json:"district,omitempty"`
}

func actorKnowsAboutGrudge(self Actor, g GrudgeRecord) bool {
	if self.ID == g.AggrievedID || self.ID == g.OffenderID {
		return true
	}
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if (g.SourceEventID != 0 && (m.EventID == g.SourceEventID || m.RootEventID == g.SourceEventID)) ||
			(g.RootEventID != 0 && (m.EventID == g.RootEventID || m.RootEventID == g.RootEventID)) {
			return true
		}
		if (m.Source == g.OffenderID && m.TargetID == g.AggrievedID) || (m.Source == g.AggrievedID && m.TargetID == g.OffenderID) {
			return true
		}
	}
	return false
}

func memoryImplicatesCorruption(m Memory, selfID, suspectID string) bool {
	if suspectID == "" || m.Kind == "action_failed" {
		return false
	}
	if m.Observation != nil && (m.Observation.Kind == "corruption" || m.Observation.Kind == "wrongful_conviction") && m.Observation.SubjectID == suspectID {
		return true
	}
	if m.Kind == "bribed" && m.TargetID == suspectID {
		return true
	}
	if m.Kind == "wrongful_conviction" && m.Source == suspectID {
		return true
	}
	if m.Kind == "speech" && m.Source != selfID && m.Statement != nil {
		switch m.Statement.Topic {
		case "corruption", "bribe", "scandal", "wrongful_conviction":
			if m.Statement.SubjectID == suspectID {
				return true
			}
		}
	}
	return false
}

func isAuditRefutingMemory(m Memory, suspectID string) bool {
	if m.TargetID != suspectID {
		return false
	}
	if m.Kind == "corruption_purged" || m.Kind == "audit_cleared" {
		return true
	}
	return m.Kind == "action_failed" && strings.Contains(m.Text, "no illicit gain")
}

// ActorCorruptionEvidence reports whether self possesses witnessed or spoken epistemic evidence
// implicating suspectID in corruption/framing that has not already been audited since that memory.
func ActorCorruptionEvidence(self Actor, suspectID string) (int64, bool) {
	if suspectID == "" || suspectID == self.ID {
		return 0, false
	}
	latestResolvedTick := int64(-1)
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if isAuditRefutingMemory(m, suspectID) {
			if m.Tick > latestResolvedTick {
				latestResolvedTick = m.Tick
			}
		}
	}
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if latestResolvedTick >= 0 && m.Tick < latestResolvedTick {
			continue
		}
		if latestResolvedTick >= 0 && m.Tick == latestResolvedTick {
			laterResolved := false
			for j := i + 1; j < len(self.Memories); j++ {
				if isAuditRefutingMemory(self.Memories[j], suspectID) {
					laterResolved = true
					break
				}
			}
			if laterResolved {
				continue
			}
		}
		if memoryImplicatesCorruption(m, self.ID, suspectID) {
			return m.EventID, true
		}
	}
	return 0, false
}

// ActorCorruptionEvidenceWithBasis verifies that basisEventID in self.Memories implicates suspectID.
func ActorCorruptionEvidenceWithBasis(self Actor, suspectID string, basisEventID int64) bool {
	if basisEventID == 0 {
		_, ok := ActorCorruptionEvidence(self, suspectID)
		return ok
	}
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if m.EventID == basisEventID {
			return memoryImplicatesCorruption(m, self.ID, suspectID)
		}
	}
	return false
}

// ActorKnownWrongfulLeads extracts wrongful-conviction leads personally witnessed or heard via speech by self.
func ActorKnownWrongfulLeads(self Actor) []KnownWrongfulLead {
	if len(self.Memories) == 0 {
		return nil
	}
	var out []KnownWrongfulLead
	seenSuspect := map[string]bool{}
	seenCase := map[int64]bool{}
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		var lead KnownWrongfulLead
		valid := false
		if m.Observation != nil && m.Observation.Kind == "wrongful_conviction" {
			lead = KnownWrongfulLead{
				BasisEventID: m.EventID,
				CaseID:       m.Observation.CaseID,
				MagistrateID: m.Observation.SubjectID,
				SuspectID:    m.Observation.ObjectID,
				District:     m.Observation.Location,
			}
			valid = true
		} else if m.Kind == "wrongful_conviction" {
			lead = KnownWrongfulLead{
				BasisEventID: m.EventID,
				MagistrateID: m.Source,
				SuspectID:    m.TargetID,
			}
			valid = true
		} else if m.Kind == "speech" && m.Source != self.ID && m.Statement != nil && m.Statement.Topic == "wrongful_conviction" {
			lead = KnownWrongfulLead{
				BasisEventID: m.EventID,
				CaseID:       m.Statement.CaseID,
				MagistrateID: m.Statement.SubjectID,
				SuspectID:    m.Statement.ObjectID,
				District:     m.Statement.Location,
			}
			valid = lead.CaseID > 0 || lead.SuspectID != "" || lead.MagistrateID != ""
		}
		if !valid {
			continue
		}
		resolvedLater := false
		for j := i + 1; j < len(self.Memories); j++ {
			mj := self.Memories[j]
			if mj.Kind == "conviction_overturned" && (lead.SuspectID != "" && mj.TargetID == lead.SuspectID) {
				resolvedLater = true
				break
			}
			if mj.Kind == "action_failed" && lead.SuspectID != "" && mj.TargetID == lead.SuspectID && strings.Contains(mj.Text, "no open wrongful conviction") {
				resolvedLater = true
				break
			}
		}
		if resolvedLater {
			continue
		}
		if lead.CaseID > 0 && seenCase[lead.CaseID] {
			continue
		}
		if lead.SuspectID != "" && seenSuspect[lead.SuspectID] {
			continue
		}
		if lead.CaseID > 0 {
			seenCase[lead.CaseID] = true
		}
		if lead.SuspectID != "" {
			seenSuspect[lead.SuspectID] = true
		}
		out = append(out, lead)
	}
	return out
}

func matchKnownLeadToCourtCase(leads []KnownWrongfulLead, cc CourtCase) (KnownWrongfulLead, bool) {
	for _, lead := range leads {
		if lead.CaseID > 0 && lead.CaseID == cc.ID {
			return lead, true
		}
		if lead.SuspectID != "" && lead.SuspectID == cc.SuspectID && (lead.District == "" || lead.District == cc.District) {
			return lead, true
		}
		if lead.CaseID == 0 && lead.SuspectID == "" && lead.MagistrateID != "" && lead.MagistrateID == cc.MagistrateID && (lead.District == "" || lead.District == cc.District) {
			return lead, true
		}
	}
	return KnownWrongfulLead{}, false
}

func (w *World) awakeSamePlaceObservers(placeActor Actor, excludeIDs ...string) []*Actor {
	excl := make(map[string]bool, len(excludeIDs))
	for _, id := range excludeIDs {
		if id != "" {
			excl[id] = true
		}
	}
	var out []*Actor
	for i := range w.Actors {
		c := &w.Actors[i]
		if c.Dead || excl[c.ID] || w.Tick < c.JailedUntil {
			continue
		}
		if c.Task != nil && (c.Task.Action.Kind == "sleep" || c.Task.Dormant) {
			continue
		}
		if SamePlace(placeActor, *c) {
			out = append(out, c)
		}
	}
	return out
}

func isRelocationAction(kind string) bool {
	return kind == "move" || kind == "move_venue" || kind == "leave_house" || kind == "enter_house"
}

// Step evaluates decisions concurrently for all idle actors against the same snapshot,
// then settles ongoing multi-tick tasks and newly chosen actions in deterministic actor order.
// In-place actions settle before departures (leave_house, enter_house, move) so co-located villagers
// who perceived each other at the start of the tick can complete speech and trades before someone walks away.
func (w *World) Step(ctx context.Context, d Decider) ([]Event, error) {
	actions := make([]Action, len(w.Actors))
	errs := make([]error, len(w.Actors))
	wasBusy := make([]bool, len(w.Actors))
	var idle []int
	for i := range w.Actors {
		if w.Actors[i].Dead || w.Tick < w.Actors[i].JailedUntil || w.Actors[i].Task != nil {
			wasBusy[i] = true
		} else {
			idle = append(idle, i)
		}
	}

	if len(idle) > 0 {
		idx := w.buildStepIndex()
		jobs := make(chan int)
		var workers sync.WaitGroup
		workerCount := stepWorkerCount(len(idle))
		for worker := 0; worker < workerCount; worker++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for i := range jobs {
					actions[i], errs[i] = d.Decide(ctx, w.viewIndexed(i, idx))
				}
			}()
		}
		for _, i := range idle {
			jobs <- i
		}
		close(jobs)
		workers.Wait()
		for _, i := range idle {
			if errs[i] != nil {
				return nil, fmt.Errorf("actor %s: %w", w.Actors[i].ID, errs[i])
			}
		}
	}

	events := w.stepFlowGrid()
	for ci := range w.Cohorts {
		c := &w.Cohorts[ci]
		before := append([]Actor(nil), c.Members...)
		for i := range c.Members {
			if w.Tick >= c.Members[i].JailedUntil && c.Members[i].Task != nil {
				events = append(events, w.Advance(c.Members[i].ID)...)
			}
		}
		w.reconcileCohortMemberChanges(c, before)
	}
	for i := range w.Actors {
		if w.Actors[i].Dead {
			continue
		}
		if w.Tick < w.Actors[i].JailedUntil {
			if w.Actors[i].Energy < MaxEnergy {
				w.Actors[i].Energy++
			}
			if w.Tick%4 == 0 && w.Actors[i].Health < MaxHealth && w.Actors[i].Hunger <= 2 && w.Actors[i].Thirst <= 1 {
				w.Actors[i].Health++
			}
			continue
		}
		if wasBusy[i] {
			if w.Actors[i].Task != nil {
				events = append(events, w.Advance(w.Actors[i].ID)...)
			}
		} else if !isRelocationAction(actions[i].Kind) {
			events = append(events, w.Apply(w.Actors[i].ID, actions[i])...)
		}
	}
	for i := range w.Actors {
		if !w.Actors[i].Dead && !wasBusy[i] && isRelocationAction(actions[i].Kind) {
			events = append(events, w.Apply(w.Actors[i].ID, actions[i])...)
		}
	}
	events = append(events, w.StepMarketAndCredit(w.ensureMarketCredit())...)
	if len(w.Cohorts) > 0 {
		events = append(events, w.stepCohorts()...)
	}

	w.Tick++
	events = append(events, w.settleShipments()...)
	events = append(events, w.AdvanceGrowth()...)
	events = append(events, w.stepActiveEffects()...)
	events = append(events, w.AdvanceContinuity()...)
	// Physiological hunger accumulates once every 6 hours (24 quarter-hour ticks),
	// scaled or disabled by each actor's FormSpec.
	for i := range w.Actors {
		w.advanceActorHunger(&w.Actors[i])
	}
	// Advance regional ecology/weather/disasters/encounters/rituals and actor lifecycle/physiology.
	events = append(events, w.stepWorldEnvironment()...)
	events = append(events, w.stepAdoptedFacilities()...)
	events = append(events, w.stepStateSlotsAndForms()...)
	events = append(events, w.stepActorPhysiologyAndLifecycle()...)
	// Release prisoners who have completed their Yamen jail sentence.
	for i := range w.Actors {
		a := &w.Actors[i]
		if !a.Dead && a.JailedUntil > 0 && w.Tick >= a.JailedUntil {
			a.JailedUntil = 0
			a.RestUntil = w.Tick + 48
			a.Tags = removeString(a.Tags, "outlaw")
			events = append(events, w.record(a, Action{Kind: "wait"}, "released_from_jail", map[string]any{
				"text": fmt.Sprintf("%s served their Yamen jail sentence and was released", a.Name),
			}))
		}
	}
	w.weatherHouses()

	var activeOffers []Offer
	for _, o := range w.Offers {
		if o.Expires >= w.Tick {
			activeOffers = append(activeOffers, o)
		} else if proposer := w.actor(o.ProposerID()); proposer != nil {
			action := Action{Kind: "offer_expired", OfferID: o.ID, TargetID: o.ResponderID(), Item: o.Item, Quantity: o.Quantity, Price: o.Price, Currency: o.Currency}
			events = append(events, w.record(proposer, action, "trade_expired", map[string]any{"text": fmt.Sprintf("offer %d expired without assent", o.ID)}, w.actor(o.ResponderID())))
		}
	}
	w.Offers = activeOffers

	events = append(events, w.StepRelationsAndTurf(w.ensureRelations())...)
	events = append(events, w.StepJusticeAndRevolt(w.ensureJusticeRevolt())...)
	events = append(events, w.ensureRelations().ObserveCausalRelationEvents(w, events)...)

	for i := range w.Actors {
		w.refreshActor(&w.Actors[i])
	}
	if len(w.Cohorts) > 0 {
		var actorWounded, actorHealed, actorWoundedDeaths int
		for i := range w.Actors {
			wounded, healed, deaths := reconcileActorWoundStatus(&w.Actors[i])
			actorWounded += wounded
			actorHealed += healed
			actorWoundedDeaths += deaths
		}
		for ci := range w.Cohorts {
			for i := range w.Cohorts[ci].Members {
				wounded, healed, deaths := reconcileActorWoundStatus(&w.Cohorts[ci].Members[i])
				actorWounded += wounded
				actorHealed += healed
				actorWoundedDeaths += deaths
			}
		}
		if actorWounded > 0 || actorHealed > 0 || actorWoundedDeaths > 0 {
			ev := Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: "world",
				Kind:    "actor_health_reconciled",
				Action:  Action{Kind: "wait"},
				Payload: map[string]any{
					"text":           fmt.Sprintf("Focal actor health ledger: +%d wounded, %d healed, %d wounded deaths", actorWounded, actorHealed, actorWoundedDeaths),
					"wounded":        actorWounded,
					"healed":         actorHealed,
					"wounded_deaths": actorWoundedDeaths,
				},
			}
			w.NextID++
			events = append(events, ev)
		}
	}
	events = append(events, w.drainCohortMortalityEvents()...)
	events = append(events, w.ObserveSpotlightAndRumors(w.ensureSpotlightRumor(), events)...)
	events = append(events, w.StepDynamicSpotlight(w.ensureSpotlightRumor())...)
	return events, nil
}

func (w *World) record(a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: a.ID, TargetID: x.TargetID, Kind: kind, Action: x, Payload: data}
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
	data["basis_event_id"] = x.BasisEventID
	data["root_event_id"] = root
	m := Memory{
		Kind:         kind,
		Tick:         w.Tick,
		EventID:      e.ID,
		Source:       a.ID,
		SourceRole:   a.Role,
		SourceTags:   append([]string(nil), a.IdentityTags()...),
		TargetID:     x.TargetID,
		Text:         fmt.Sprint(data["text"]),
		BasisEventID: x.BasisEventID,
		RootEventID:  root,
	}
	if kind == "speech" && x.Statement != nil {
		v := *x.Statement
		m.Statement = &v
		data["statement"] = v
	}
	if obs, ok := data["observation"].(Observation); ok {
		v := obs
		m.Observation = &v
	}
	m.search = newMemorySearchCache(m)
	a.Memories = appendMemory(a.Memories, m)
	for _, b := range observers {
		if b != nil && b.ID != a.ID {
			b.Memories = appendMemory(b.Memories, m)
		}
	}
	w.ObserveContinuity(e)
	return e
}

// grantProfessionXP advances profession experience (and optional guild influence) with level-up and rank-advancement events.
func (w *World) grantProfessionXP(a *Actor, x Action, profID string, xp, influence int) []Event {
	prof, ok := w.Catalog.ProfessionByID(profID)
	if !ok {
		return nil
	}
	oldLevel, held := a.Professions[profID]
	if !held {
		return nil
	}
	if a.ProfessionXP == nil {
		a.ProfessionXP = map[string]int{}
	}
	a.ProfessionXP[profID] += xp
	var events []Event
	level := w.Catalog.ProfessionLevel(a.ProfessionXP[profID])
	if level > prof.MaxLevel {
		level = prof.MaxLevel
	}
	if level > oldLevel {
		oldRank := prof.RankTitle(oldLevel)
		newRank := prof.RankTitle(level)
		a.Professions[profID] = level
		w.refreshActor(a)
		events = append(events, w.record(a, x, "profession_leveled", map[string]any{
			"text":            fmt.Sprintf("%s reached %s level %d (%s)", a.Name, profID, level, newRank),
			"profession":      profID,
			"level":           level,
			"rank":            newRank,
			"previous_rank":   oldRank,
			"character_level": a.Level,
			"xp":              a.ProfessionXP[profID],
		}))
	}
	for _, adv := range w.Catalog.Professions {
		if adv.ParentProfession == profID {
			if advOld, hasAdv := a.Professions[adv.ID]; hasAdv {
				a.ProfessionXP[adv.ID] += xp
				advLevel := w.Catalog.ProfessionLevel(a.ProfessionXP[adv.ID])
				if advLevel > adv.MaxLevel {
					advLevel = adv.MaxLevel
				}
				if advLevel > advOld {
					oldRank := adv.RankTitle(advOld)
					newRank := adv.RankTitle(advLevel)
					a.Professions[adv.ID] = advLevel
					w.refreshActor(a)
					events = append(events, w.record(a, x, "profession_leveled", map[string]any{
						"text":            fmt.Sprintf("%s reached %s level %d (%s)", a.Name, adv.ID, advLevel, newRank),
						"profession":      adv.ID,
						"level":           advLevel,
						"rank":            newRank,
						"previous_rank":   oldRank,
						"character_level": a.Level,
						"xp":              a.ProfessionXP[adv.ID],
					}))
				}
			}
		}
	}
	if f := w.faction(prof.Guild); f != nil && f.HasMember(a.ID) && influence > 0 {
		f.Influence += influence * a.Professions[profID]
		if next := w.Catalog.FactionLevel(f.Influence); next > f.Level {
			f.Level = next
			events = append(events, w.record(a, x, "faction_leveled", map[string]any{
				"text":       fmt.Sprintf("%s now stands at level %d", f.Name, next),
				"faction_id": f.ID,
				"level":      next,
				"influence":  f.Influence,
			}))
		}
	}
	w.refreshActor(a)
	return events
}

func removeString(slice []string, val string) []string {
	if len(slice) == 0 {
		return slice
	}
	out := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != val {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func canonicalDeathCause(cause string) string {
	switch cause {
	case "old_age", "disease", "disaster", "accident", "beast", "starvation", "dehydration", "cold", "combat":
		return cause
	default:
		return "combat"
	}
}

func (w *World) markDeadIfZero(target *Actor, killer *Actor, x Action, cause string) []Event {
	if target == nil || target.Dead || target.Health > 0 {
		return nil
	}
	wasSick := target.DiseaseID != ""
	wasJailed := target.JailedUntil > w.Tick
	if c := w.storedActorCohort(target.ID); c != nil && c.Population == 1 {
		w.transferLastCohortAssets(c, target)
	}
	target.Health = 0
	target.Dead = true
	target.DiseaseID = ""
	target.DiseaseTicks = 0
	target.Task = nil
	target.JailedUntil = 0
	target.IllicitGain = 0
	filtered := w.Offers[:0]
	for _, o := range w.Offers {
		if o.Seller != target.ID && o.Buyer != target.ID {
			filtered = append(filtered, o)
		}
	}
	w.Offers = filtered
	if len(w.Offers) == 0 {
		w.Offers = nil
	}
	canon := canonicalDeathCause(cause)
	w.recordDeathCause(canon, 1)
	killerID := ""
	killerName := "natural or environmental causes"
	if killer != nil {
		killerID = killer.ID
		killerName = killer.Name
	}
	ev := w.record(target, x, "actor_died", map[string]any{
		"text":       fmt.Sprintf("%s died from %s (%s)", target.Name, cause, killerName),
		"killer_id":  killerID,
		"cause":      canon,
		"detail":     cause,
		"was_sick":   wasSick,
		"was_jailed": wasJailed,
	}, killer)
	// Settle inheritance before deciding where any heirless estate goes. Observe
	// marks this death event, so the end-of-step observer cannot settle it twice.
	return append([]Event{ev}, w.ensureRelations().ObserveCausalRelationEvents(w, []Event{ev})...)
}

func (w *World) applySleepTick(a *Actor, carryTenths int) (newCarry int, factors string) {
	var h *House
	if a.InsideHouse != "" {
		h = w.house(a.InsideHouse)
	}
	tenths, notes := SleepRecoveryTenths(*a, h, w.Tick)
	total := carryTenths + tenths
	gain := total / 10
	rem := total % 10
	a.Energy += gain
	if a.Energy >= MaxEnergy {
		a.Energy = MaxEnergy
		rem = 0
		a.Resting = false
	}
	if a.Health < MaxHealth && !a.Dead && a.Hunger <= 2 {
		if h != nil {
			a.Health++
		} else if w.Tick%2 == 0 {
			a.Health++
		}
		if a.Health > MaxHealth {
			a.Health = MaxHealth
		}
	}
	return rem, notes
}

// Advance progresses an actor's ongoing multi-tick Task by one 15-minute tick.
func (w *World) Advance(id string) []Event {
	a := w.actor(id)
	if a == nil || a.Dead || a.Task == nil {
		return nil
	}
	t := a.Task
	x := t.Action
	if c := w.storedActorCohort(x.TargetID); c != nil && x.TargetID != id {
		before := append([]Actor(nil), c.Members...)
		defer w.reconcileCohortMemberChanges(c, before)
	}

	if x.Kind == "sleep" {
		carry, factors := w.applySleepTick(a, t.SleepTenths)
		t.SleepTenths = carry
		t.Remaining--
		if t.Remaining <= 0 || (a.Energy >= MaxEnergy && a.Health >= MaxHealth) {
			gained := a.Energy - t.StartEnergy
			a.Task = nil
			return []Event{w.record(a, x, "slept", map[string]any{
				"text":          fmt.Sprintf("%s woke up with %d energy (+%d) and %d health", a.Name, a.Energy, gained, a.Health),
				"energy_gained": gained,
				"health":        a.Health,
				"sleep_factors": factors,
			})}
		}
		return nil
	}

	if x.Kind == "adopt_facility" {
		if reason := w.facilityAdoptionSiteFailure(*a, x); reason != "" {
			return []Event{w.record(a, x, "task_paused", map[string]any{"text": reason, "remaining_ticks": t.Remaining})}
		}
	}
	if x.Kind == "craft" && !w.craftingTaskOperational(a, t) {
		return []Event{w.record(a, x, "task_paused", map[string]any{
			"text":            "crafting paused because the production site is unavailable or unpowered",
			"remaining_ticks": t.Remaining,
		})}
	}
	if x.Kind == "craft" {
		w.advanceCraftingWork(a, t)
	} else {
		t.Remaining--
	}
	if t.Remaining > 0 {
		return nil
	}
	a.Task = nil
	return w.finishTask(a, *t)
}

// craftingTaskOperational checks sustained site requirements without consuming the
// task's prepaid materials again or discarding them while work is paused.
func (w *World) craftingTaskOperational(a *Actor, t *Task) bool {
	r, ok := w.Catalog.RecipeByID(t.Action.RecipeID)
	return ok && w.productionSiteOperational(a.Location, a.VenueID, a.InsideHouse, r)
}

// advanceCraftingWork grants acceleration only for this funded operating tick.
// Fractional work survives a pause without granting future unfunded acceleration.
func (w *World) advanceCraftingWork(a *Actor, t *Task) {
	progress := 100 + t.WorkCredit + w.FacilityProductionBonusPct(*a)
	t.Remaining -= progress / 100
	t.WorkCredit = progress % 100
}

func (w *World) finishTask(a *Actor, t Task) []Event {
	x := t.Action
	switch x.Kind {
	case "move":
		a.Location = x.Location
		if a.VenueID != "" {
			if v, ok := w.Map.VenueByID(a.VenueID); !ok || (v.DistrictID != "" && v.DistrictID != x.Location) {
				a.VenueID = w.Map.DefaultVenueForDistrict(x.Location)
			}
		}
		return []Event{w.record(a, x, "moved", map[string]any{
			"text": a.Name + " walked along the road to " + x.Location,
		})}
	case "move_venue":
		return w.finishMoveVenue(a, t)
	case "craft":
		r, ok := w.Catalog.RecipeByID(x.RecipeID)
		if !ok {
			return nil
		}
		if r.Growth != nil {
			parents := map[string]int64{}
			for item, qty := range r.Growth.Parents {
				if a.Items[item] < qty {
					return []Event{w.record(a, x, "action_failed", map[string]any{"text": "breeding stock no longer available"})}
				}
			}
			for item, qty := range r.Growth.Parents {
				w.adjustActorItem(a, item, -qty, false)
				parents[item] = int64(qty)
			}
			if len(parents) == 0 {
				parents = nil
			}
			event := w.startGrowth(a.ID, a.Location, a.VenueID, false, r, 1, int64(t.Yield), parents)
			g := &w.Growth[len(w.Growth)-1]
			g.FieldID = t.FieldID
			g.HouseID = a.InsideHouse
			g.PreparationUntil = w.Tick
			event.Payload["stage"] = "prepare"
			event.Payload["stage_complete"] = true
			event.Payload["field_id"] = g.FieldID
			event.Payload["workers"] = []WorkerRun{{FirstID: a.ID, Count: 1}}
			return []Event{event}
		}
		before := w.yieldCredit("actor", a.ID, r.ID)
		delivered, next := settleYieldCredit(before, t.FundedHundredths)
		w.setYieldCredit("actor", a.ID, r.ID, next)
		t.Yield = int(delivered)
		w.adjustActorItem(a, r.OutputItem, t.Yield, true)
		tier := w.Catalog.Items[r.OutputItem].Tier
		if tier < 1 {
			tier = 1
		}
		obs := Observation{Kind: "item_produced", Item: r.OutputItem, Location: a.Location, Quantity: t.Yield}
		events := []Event{w.record(a, x, "crafted", map[string]any{
			"text":                fmt.Sprintf("%s completed %s (+%d %s)", a.Name, r.ID, t.Yield, r.OutputItem),
			"recipe_id":           r.ID,
			"item":                r.OutputItem,
			"tier":                tier,
			"quantity":            t.Yield,
			"yield_credit_before": before,
			"yield_credit_after":  next,
			"observation":         obs,
		})}
		return append(events, w.grantProfessionXP(a, x, r.Profession, w.Catalog.CraftXP(r), tier)...)
	case "maintain_infra":
		if w.DistrictStates == nil {
			return nil
		}
		ds, ok := w.DistrictStates[a.Location]
		spec, exists := w.Catalog.Infrastructures[x.Item]
		if !ok || !exists || ds.Infrastructure == nil {
			return nil
		}
		cond := ds.Infrastructure[x.Item] + 20
		if cond > spec.MaxCondition {
			cond = spec.MaxCondition
		}
		ds.Infrastructure[x.Item] = cond
		w.DistrictStates[a.Location] = ds
		events := []Event{w.record(a, x, "infra_maintained", map[string]any{
			"text":           fmt.Sprintf("%s reinforced %s in %s to condition %d/%d", a.Name, spec.Name, a.Location, cond, spec.MaxCondition),
			"district":       a.Location,
			"infrastructure": x.Item,
			"condition":      cond,
		})}
		for profID := range a.Professions {
			return append(events, w.grantProfessionXP(a, x, profID, 6, 2)...)
		}
		return events
	case "hunt_threat":
		if w.DistrictStates == nil {
			return nil
		}
		ds, ok := w.DistrictStates[a.Location]
		if !ok {
			return nil
		}
		var enc *EncounterSpec
		for i := range w.Catalog.Encounters {
			if w.Catalog.Encounters[i].ID == x.TargetID {
				enc = &w.Catalog.Encounters[i]
				break
			}
		}
		if enc == nil {
			return nil
		}
		suppressed := 8 + CombatPowerWithCatalog(w.Catalog, *a)/4
		curAct := ds.ThreatActivity[enc.ID]
		if suppressed > curAct {
			suppressed = curAct
		}
		dropQty := w.drawEncounterLoot(a.Location, *enc, int64(minInt(1, suppressed)), "act_hunt:"+a.ID)
		ds = w.DistrictStates[a.Location]
		ds.ThreatActivity[enc.ID] = curAct - suppressed
		bounty := enc.BountyCoins
		if ds.BountyPool > 0 {
			claim := ds.BountyPool
			if claim > enc.BountyCoins {
				claim = enc.BountyCoins
			}
			ds.BountyPool -= claim
			bounty += claim
		}
		if len(w.Cohorts) > 0 {
			paid := int(w.debitTreasury(int64(bounty), "bounty"))
			if ds.BountyPool > 0 && paid < bounty {
				ds.BountyPool += (bounty - paid)
			}
			bounty = paid
		}
		w.DistrictStates[a.Location] = ds
		w.adjustActorCurrency(a, "coins", int64(bounty), len(w.Cohorts) == 0)
		if dropQty > 0 {
			w.adjustActorItem(a, enc.RareDropItem, int(dropQty), true)
		}
		if a.Health > 20 {
			a.Health -= 4
		}
		a.RestUntil = w.Tick + 24
		events := []Event{w.record(a, x, "threat_hunted", map[string]any{
			"text":         fmt.Sprintf("%s hunted %s in %s (-%d threat, +%d bounty coins, +%d %s)", a.Name, enc.Name, a.Location, suppressed, bounty, dropQty, enc.RareDropItem),
			"district":     a.Location,
			"threat_id":    enc.ID,
			"threat_name":  enc.Name,
			"suppressed":   suppressed,
			"bounty_coins": bounty,
			"drop_item":    enc.RareDropItem,
			"drop_qty":     dropQty,
		})}
		return append(events, w.grantCombatXP(a, x, 8, 3)...)
	case "perform_ritual":
		var rit *RitualSpec
		for i := range w.Catalog.Rituals {
			if w.Catalog.Rituals[i].ID == x.RecipeID {
				rit = &w.Catalog.Rituals[i]
				break
			}
		}
		if rit == nil {
			return nil
		}
		cal := CalendarAt(w.Tick, w.Catalog.Calendar)
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[a.Location]; ok {
				ds.MoraleBonus += rit.MoraleBoost
				if ds.MoraleBonus > 30 {
					ds.MoraleBonus = 30
				}
				if rit.ScheduledSeason != "" {
					ds.LastRitualSeason = fmt.Sprintf("Y%d-%s", cal.Year, cal.SeasonID)
				}
				if rit.Supernatural && rit.DisasterReliefWeightPct > 0 {
					appease := (rit.ThreatAppeaseAmount * rit.DisasterReliefWeightPct) / 100
					for encID, act := range ds.ThreatActivity {
						if act > appease {
							ds.ThreatActivity[encID] = act - appease
						}
					}
					for ai := range ds.ActiveDisasters {
						rem := ds.ActiveDisasters[ai].ExpiresTick - w.Tick
						ds.ActiveDisasters[ai].ExpiresTick -= (rem * int64(rit.DisasterReliefWeightPct)) / 100
					}
				}
				w.DistrictStates[a.Location] = ds
			}
		}
		if rit.TrustBoost > 0 {
			for bi := range w.Actors {
				if w.Actors[bi].ID != a.ID && SamePlace(*a, w.Actors[bi]) {
					a.Trust[w.Actors[bi].ID] += rit.TrustBoost
					w.Actors[bi].Trust[a.ID] += rit.TrustBoost
				}
			}
		}
		events := []Event{w.record(a, x, "ritual_performed", map[string]any{
			"text":         fmt.Sprintf("%s performed %s at %s (+%d morale)", a.Name, rit.Name, a.Location, rit.MoraleBoost),
			"ritual_id":    rit.ID,
			"ritual_name":  rit.Name,
			"district":     a.Location,
			"morale_boost": rit.MoraleBoost,
		})}
		for _, prof := range w.Catalog.Professions {
			if a.Professions[prof.ID] > 0 && (rit.RequiredTag == "" || ActorHasTag(prof.Grants, rit.RequiredTag)) {
				return append(events, w.grantProfessionXP(a, x, prof.ID, 8, 3)...)
			}
		}
		return events
	case "repair_house":
		h := w.house(x.HouseID)
		if h == nil {
			return nil
		}
		restore := 25
		if carp := a.Professions["carpenter"] + a.Professions["master_carpenter"]; carp > 0 {
			restore = 30 + 5*carp
		}
		h.Condition += restore
		if h.Condition > MaxCondition {
			h.Condition = MaxCondition
		}
		events := []Event{w.record(a, x, "repaired_house", map[string]any{
			"text":      fmt.Sprintf("%s repaired %s to condition %d", a.Name, h.Name, h.Condition),
			"house_id":  h.ID,
			"condition": h.Condition,
		})}
		return append(events, w.grantProfessionXP(a, x, "carpenter", 6, 2)...)
	case "upgrade_house":
		h := w.house(x.HouseID)
		if h == nil {
			return nil
		}
		h.Level++
		if h.Level > 3 {
			h.Level = 3
		}
		if h.Comfort < 3 {
			h.Comfort++
		}
		h.Condition = MaxCondition
		events := []Event{w.record(a, x, "house_upgraded", map[string]any{
			"text":      fmt.Sprintf("%s upgraded %s to level %d (comfort %d)", a.Name, h.Name, h.Level, h.Comfort),
			"house_id":  h.ID,
			"level":     h.Level,
			"comfort":   h.Comfort,
			"condition": h.Condition,
		})}
		return append(events, w.grantProfessionXP(a, x, "carpenter", 12, 3)...)
	case "attempt_transition":
		return w.executeTransition(a, x, resolveTransitionID(x))
	case "research_tech":
		return w.finishResearchTech(a, t)
	case "adopt_facility":
		return w.finishAdoptFacility(a, t)
	default:
		if spec, ok := w.Catalog.ActionSpecs[x.Kind]; ok {
			return w.finishDeclarativeAction(a, t, spec)
		}
		return nil
	}
}

func (w *World) Apply(id string, x Action) []Event {
	a := w.actor(id)
	if a == nil {
		return nil
	}
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	emit := func(kind, text string, observers ...*Actor) []Event {
		return []Event{w.record(a, x, kind, map[string]any{"text": text}, observers...)}
	}
	fail := func(reason string) []Event { return emit("action_failed", reason) }

	if w.storedActorCohort(id) != nil {
		return fail("actor must be hydrated before taking an individual action")
	}
	if c := w.storedActorCohort(x.TargetID); c != nil {
		before := append([]Actor(nil), c.Members...)
		defer w.reconcileCohortMemberChanges(c, before)
	}
	if a.Dead {
		return fail("actor is dead")
	}
	if w.Tick < a.JailedUntil {
		return fail("actor is imprisoned in the Yamen jail")
	}
	if a.Task != nil {
		return fail("actor is already busy with " + a.Task.Action.Kind)
	}
	if !w.ActorCapabilities(*a)[x.Kind] {
		return fail("identity tags do not grant action: " + x.Kind)
	}
	if blocked, reason := w.IsActionBlockedByWorldLaw(*a, x); blocked {
		return fail(reason)
	}
	if err := ValidateCitation(a.Memories, x.BasisEventID, x.Statement); err != nil {
		x.BasisEventID = 0
		return fail(err.Error())
	}

	b := w.actor(x.TargetID)

	switch x.Kind {
	case "refuel":
		if w.FlowGrid != nil {
			for i := range w.FlowGrid.Nodes {
				n := &w.FlowGrid.Nodes[i]
				if n.ID != x.TargetID {
					continue
				}
				if n.Kind != "generator" || n.Source == nil || n.Source.FuelItem == "" || n.DistrictID != a.Location || (n.VenueID != "" && n.VenueID != a.VenueID) {
					return fail("generator is not locally accessible")
				}
				if x.Quantity <= 0 || a.Items[n.Source.FuelItem] < x.Quantity {
					return fail("insufficient declared generator fuel")
				}
				w.adjustActorItem(a, n.Source.FuelItem, -x.Quantity, false)
				w.adjustFlowNodeFuel(n, int64(x.Quantity), false)
				return emit("generator_refueled", fmt.Sprintf("transferred %d %s into %s", x.Quantity, n.Source.FuelItem, n.ID))
			}
		}
		return fail("unknown generator")
	case "propose_growth_care", "accept_growth_care", "dispatch_harvest":
		return w.applyGrowthCareAction(a, x)
	case "harvest":
		for i, g := range w.Growth {
			if fmt.Sprint(g.ID) == x.TargetID && w.canHarvestGrowth(g, a.ID, false) {
				return w.harvestGrowth(i, GrowthBatch{OwnerID: a.ID, District: g.District, VenueID: g.VenueID})
			}
		}
		return fail("no mature owned crop or stock at this location")
	case "wait":
		if a.Energy < MaxEnergy {
			a.Energy++
		}
		return nil
	case "begin_rest":
		a.Resting = true
		return emit("rest_planned", "plans to reach shelter and recover energy before starting new work")
	case "end_rest":
		a.Resting = false
		return emit("rest_abandoned", "ends the planned rest")

	case "join_faction":
		f := w.faction(x.FactionID)
		if f == nil {
			return fail("unknown faction: " + x.FactionID)
		}
		if f.HasMember(id) {
			return fail("already a member of " + f.Name)
		}
		if a.InsideHouse != "" || a.Location != f.District {
			return fail("joining " + f.Name + " requires standing at the guild hall in " + f.District)
		}
		if ok, reason := a.Attributes.Meets(f.Requirements); !ok {
			return fail("guild charter unmet: " + reason)
		}
		f.Members = append(f.Members, id)
		a.Guilds = appendUnique(a.Guilds, f.ID)
		w.refreshActor(a)
		return emit("faction_joined", fmt.Sprintf("%s joined %s (level %d)", a.Name, f.Name, f.Level))

	case "join_profession":
		prof, ok := w.Catalog.ProfessionByID(x.ProfessionID)
		if !ok {
			return fail("unknown profession: " + x.ProfessionID)
		}
		f := w.faction(prof.Guild)
		if f == nil || !f.HasMember(id) {
			return fail("must join the " + prof.Guild + " guild before learning " + prof.Name)
		}
		if a.InsideHouse != "" || a.Location != f.District {
			return fail("learning " + prof.Name + " requires the guild hall in " + f.District)
		}
		if ok, reason := w.Catalog.CanLearnProfession(*a, prof); !ok {
			return fail(reason)
		}
		if a.Professions == nil {
			a.Professions = map[string]int{}
		}
		a.Professions[prof.ID] = 1
		for _, grant := range prof.Grants {
			a.Tags = appendUnique(a.Tags, grant)
		}
		w.refreshActor(a)
		rank := prof.RankTitle(1)
		payload := map[string]any{
			"text":            fmt.Sprintf("%s learned %s (%s, tier %d, level 1, character level %d)", a.Name, prof.Name, rank, prof.Tier, a.Level),
			"profession":      prof.ID,
			"tier":            prof.Tier,
			"rank":            rank,
			"character_level": a.Level,
		}
		if prof.ParentProfession != "" {
			payload["parent_profession"] = prof.ParentProfession
			payload["text"] = fmt.Sprintf("%s advanced from %s to %s (%s, tier %d, level 1, character level %d)", a.Name, prof.ParentProfession, prof.Name, rank, prof.Tier, a.Level)
		}
		return []Event{w.record(a, x, "profession_joined", payload)}

	case "enter_house":
		if a.InsideHouse != "" {
			return fail("already inside house " + a.InsideHouse)
		}
		h := w.house(x.HouseID)
		if h == nil || h.District != a.Location {
			return fail("house is not in current district")
		}
		if h.VenueID != "" && a.VenueID != "" && h.VenueID != a.VenueID {
			return fail("house is in a different venue")
		}
		if !h.CanAccess(a.ID) {
			return fail("actor does not have access to house " + h.ID)
		}
		if a.VenueID == "" && h.VenueID != "" {
			a.VenueID = h.VenueID
		}
		a.InsideHouse = h.ID
		return emit("entered_house", a.Name+" entered "+h.Name)

	case "leave_house":
		if a.InsideHouse == "" {
			return fail("actor is already outdoors")
		}
		h := w.house(a.InsideHouse)
		name := a.InsideHouse
		if h != nil {
			name = h.Name
			if h.VenueID != "" {
				a.VenueID = h.VenueID
			}
		}
		a.InsideHouse = ""
		return emit("left_house", a.Name+" stepped outside from "+name)

	case "move_venue":
		return w.applyMoveVenue(a, x)

	case "issue_decree":
		return w.applyIssueDecree(a, x)

	case "transmit_message":
		return w.applyTransmitMessage(a, x)

	case "move":
		if a.InsideHouse != "" {
			return fail("leave house before walking village roads")
		}
		if len(w.Map.Venues) > 0 {
			return w.applyMoveDistrict(a, x)
		}
		road, ok := w.Map.RoadBetween(a.Location, x.Location)
		if !ok {
			return fail(fmt.Sprintf("no direct road from %s to %s", a.Location, x.Location))
		}
		effTicks, effEnergy := w.effectiveRoadCost(road)
		if a.Energy < effEnergy {
			return fail("insufficient energy to walk to " + x.Location)
		}
		a.Energy -= effEnergy
		task := Task{Action: x, TotalTicks: effTicks, Remaining: effTicks - 1, StartedTick: w.Tick}
		if task.Remaining <= 0 {
			return w.finishTask(a, task)
		}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s set out toward %s (%d ticks)", a.Name, x.Location, effTicks),
			"remaining_ticks": task.Remaining,
		})}

	case "sleep":
		if a.Energy >= MaxEnergy && a.Health >= MaxHealth {
			return fail("already fully rested")
		}
		ticks := x.Quantity
		if ticks <= 0 {
			ticks = 16
		}
		if ticks > 32 {
			ticks = 32
		}
		x.Quantity = ticks
		startEnergy := a.Energy
		carry, factors := w.applySleepTick(a, 0)
		if ticks == 1 || (a.Energy >= MaxEnergy && a.Health >= MaxHealth) {
			gained := a.Energy - startEnergy
			return []Event{w.record(a, x, "slept", map[string]any{
				"text":          fmt.Sprintf("%s slept briefly and now has %d energy (+%d) and %d health", a.Name, a.Energy, gained, a.Health),
				"energy_gained": gained,
				"health":        a.Health,
				"sleep_factors": factors,
			})}
		}
		a.Task = &Task{
			Action:      x,
			TotalTicks:  ticks,
			Remaining:   ticks - 1,
			StartedTick: w.Tick,
			SleepTenths: carry,
			StartEnergy: startEnergy,
		}
		return []Event{w.record(a, x, "sleep_started", map[string]any{
			"text":          fmt.Sprintf("%s lay down to sleep (%d ticks)", a.Name, ticks),
			"sleep_factors": factors,
		})}

	case "eat":
		itemID := x.Item
		if itemID == "" {
			for k, spec := range w.Catalog.Items {
				if spec.Edible && a.Items[k] > 0 {
					if itemID == "" || spec.NutritionUnits > w.Catalog.Items[itemID].NutritionUnits || spec.NutritionUnits == w.Catalog.Items[itemID].NutritionUnits && k < itemID {
						itemID = k
					}
				}
			}
			if itemID == "" {
				itemID = "food"
			}
			x.Item = itemID
		}
		spec, ok := w.Catalog.Items[itemID]
		if !ok || !spec.Edible {
			return fail("item is not edible: " + itemID)
		}
		if a.Items[itemID] < 1 {
			return fail("no " + itemID)
		}
		if a.Hunger == 0 {
			return nil
		}
		w.consumeHeldFood(a, itemID)
		return emit("ate", fmt.Sprintf("%s ate 1 %s", a.Name, itemID))

	case "drink":
		if a.Thirst <= 0 {
			return nil
		}
		if x.Item == "" {
			ds := w.DistrictStates[a.Location]
			if pool := w.potablePoolID(ds); pool != "" && w.drawDistrictResource(a.Location, pool, 1) == 1 {
				a.Thirst = 0
				return emit("drank", fmt.Sprintf("%s drank fresh water at %s", a.Name, a.Location))
			}
		}

		itemID := x.Item
		if itemID == "" {
			for k, spec := range w.Catalog.Items {
				if spec.Potable && a.Items[k] > 0 {
					itemID = k
					break
				}
			}
			x.Item = itemID
		}
		spec, ok := w.Catalog.Items[itemID]
		if !ok || !spec.Potable {
			return fail("no potable water available")
		}
		if a.Items[itemID] < 1 {
			return fail("no " + itemID)
		}
		w.adjustActorItem(a, itemID, -1, true)
		relief := spec.ThirstRelief
		if relief <= 0 {
			relief = 2
		}
		a.Thirst -= relief
		if a.Thirst < 0 {
			a.Thirst = 0
		}
		w.applyItemSlotAndEnergyEffects(a, spec)
		return emit("drank", fmt.Sprintf("%s drank 1 %s", a.Name, itemID))

	case "relieve":
		if a.Waste <= 0 {
			return nil
		}
		cleanLatrine := a.InsideHouse != "" || w.DistrictStates == nil
		if !cleanLatrine && w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[a.Location]; ok {
				if cond, hasSan := w.sanitationInfraCondition(ds); hasSan && cond >= 30 {
					cleanLatrine = true
				}
			}
		}
		wasteAmt := int64(0)
		if divisor := w.Catalog.Lifecycle.WastePerFoodDivisor; divisor > 0 && w.Catalog.Lifecycle.WasteItem != "" {
			wasteAmt = a.DigestedFood / divisor
			a.DigestedFood -= wasteAmt * divisor
		}
		a.Waste = 0
		if w.DistrictStates != nil {
			if ds, ok := w.DistrictStates[a.Location]; ok {
				if cleanLatrine {
					w.adjustDistrictNightsoil(a.Location, w.Catalog.Lifecycle.WasteItem, int64(wasteAmt), true)
				} else if ds.Sanitation > 10 {
					ds.Sanitation--
					w.DistrictStates[a.Location] = ds
				}
			}
		}
		return emit("relieved", fmt.Sprintf("%s used sanitation facility in %s", a.Name, a.Location))

	case "medicate":
		if a.Health >= MaxHealth && a.DiseaseID == "" {
			return fail("actor has no injury or illness")
		}
		itemID := x.Item
		if itemID == "" {
			bestScore := 0
			for k, spec := range w.Catalog.Items {
				score := spec.MedicinalHeal + spec.DiseaseCure*15
				if score > bestScore && a.Items[k] >= 1 {
					bestScore = score
					itemID = k
				}
			}
			x.Item = itemID
		}
		spec, ok := w.Catalog.Items[itemID]
		if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) {
			return fail("item is not medicinal: " + itemID)
		}
		if a.Items[itemID] < 1 {
			return fail("no " + itemID)
		}
		w.adjustActorItem(a, itemID, -1, true)
		healAmt := spec.MedicinalHeal
		if healAmt > MaxHealth-a.Health {
			healAmt = MaxHealth - a.Health
		}
		if healAmt < 0 {
			healAmt = 0
		}
		a.Health += healAmt
		restCut := int64(maxInt(24, spec.MedicinalHeal))
		if a.RestUntil > w.Tick+restCut {
			a.RestUntil -= restCut
		} else if a.RestUntil > w.Tick {
			a.RestUntil = w.Tick
		}
		curedDisease := ""
		if a.DiseaseID != "" && spec.DiseaseCure > 0 {
			a.DiseaseTicks -= spec.DiseaseCure * TicksPerDay
			if a.DiseaseTicks <= 0 {
				curedDisease = a.DiseaseID
				a.DiseaseID = ""
				a.DiseaseTicks = 0
			}
		}
		w.applyItemSlotAndEnergyEffects(a, spec)
		ev := w.record(a, x, "medicated", map[string]any{
			"text":          fmt.Sprintf("%s treated wounds/illness with 1 %s (+%d health -> %d)", a.Name, itemID, healAmt, a.Health),
			"item":          itemID,
			"healed":        healAmt,
			"health":        a.Health,
			"cured_disease": curedDisease,
		})
		return []Event{ev}

	case "equip":
		return w.applyEquip(a, x)

	case "unequip":
		return w.applyUnequip(a, x)

	case "craft":
		r, ok := w.Catalog.RecipeByID(x.RecipeID)
		if !ok {
			return fail("unknown recipe: " + x.RecipeID)
		}
		if reason := recipeBlockReason(w.Catalog, w.Map, w.house(a.InsideHouse), *a, r, w.productionStartBlockReason(*a, r), w.availableDistrictResource(a.Location, r.ResourcePool)); reason != "" {
			return fail(reason)
		}
		fieldID, _ := w.fieldAvailable("actor", a.ID, a.Location, a.VenueID, r)
		if r.ResourcePool != "" {
			draw := int64(maxInt(1, r.ResourceDraw))
			w.drawDistrictResource(a.Location, r.ResourcePool, draw)
		}
		a.Energy -= r.EnergyCost
		if r.MoneyCost > 0 {
			if len(w.Cohorts) > 0 {
				w.adjustActorCurrency(a, "coins", -int64(r.MoneyCost), false)
				w.creditTreasury(int64(r.MoneyCost), "fee")
			} else {
				w.adjustActorCurrency(a, "coins", -int64(r.MoneyCost), true)
			}
		}
		for item, qty := range r.Inputs {
			w.adjustActorItem(a, item, -qty, true)
		}
		bonus := w.Catalog.ConsumablesYieldBonus(r) && a.Items[r.BonusInput] >= 1
		if bonus {
			w.adjustActorItem(a, r.BonusInput, -1, true)
		}
		yield := w.Catalog.RecipeYield(*a, r, bonus)
		var funded int64
		if r.Growth == nil {
			whole, batch := w.previewNonBiologicalYield("actor", a.ID, a.Location, r)
			yield, funded = int(whole), batch
		}
		w.wearProductionTool("actor", a.ID, r, 1)
		ticks := r.Ticks
		task := Task{Action: x, TotalTicks: ticks, Remaining: ticks, StartedTick: w.Tick, Yield: yield, FundedHundredths: funded, FieldID: fieldID}
		w.advanceCraftingWork(a, &task)
		if task.Remaining <= 0 {
			return w.finishTask(a, task)
		}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s started %s (expected %d %s, %d ticks)", a.Name, r.ID, yield, r.OutputItem, ticks),
			"recipe_id":       r.ID,
			"expected_yield":  yield,
			"remaining_ticks": task.Remaining,
		})}

	case "maintain_infra":
		if w.DistrictStates == nil {
			return fail("district public infrastructure is not enabled")
		}
		if a.InsideHouse != "" {
			return fail("step outside before maintaining district infrastructure")
		}
		ds, hasDS := w.DistrictStates[a.Location]
		spec, exists := w.Catalog.Infrastructures[x.Item]
		if !hasDS || !exists || ds.Infrastructure == nil {
			return fail("unknown infrastructure: " + x.Item)
		}
		curCond, present := ds.Infrastructure[x.Item]
		if !present {
			return fail("infrastructure is not present in " + a.Location)
		}
		if curCond >= spec.MaxCondition {
			return fail("infrastructure is already at full condition")
		}
		if a.Energy < spec.BuildEnergy {
			return fail("insufficient energy to maintain infrastructure")
		}
		for inItem, inQty := range spec.BuildInputs {
			if a.Items[inItem] < inQty {
				return fail(fmt.Sprintf("maintaining %s requires %d %s", spec.ID, inQty, inItem))
			}
		}
		a.Energy -= spec.BuildEnergy
		for inItem, inQty := range spec.BuildInputs {
			w.adjustActorItem(a, inItem, -inQty, true)
		}
		task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s started reinforcing %s in %s (3 ticks)", a.Name, spec.Name, a.Location),
			"infrastructure":  spec.ID,
			"remaining_ticks": task.Remaining,
		})}

	case "hunt_threat":
		if w.DistrictStates == nil {
			return fail("no ecological threats enabled")
		}
		if a.InsideHouse != "" {
			return fail("step outside before hunting wild threats")
		}
		if a.Energy < 10 {
			return fail("insufficient energy to hunt threat")
		}
		if a.Health < 50 || w.Tick < a.RestUntil {
			return fail("hunter is injured or recuperating")
		}
		ds, hasDS := w.DistrictStates[a.Location]
		if !hasDS {
			return fail("no threat activity in current district")
		}
		chosenID := ""
		bestAct := 5
		for _, enc := range w.Catalog.Encounters {
			if x.TargetID != "" && enc.ID != x.TargetID {
				continue
			}
			if act := ds.ThreatActivity[enc.ID]; act > bestAct {
				bestAct = act
				chosenID = enc.ID
			}
		}
		if chosenID == "" {
			return fail("no active threat above threshold in " + a.Location)
		}
		x.TargetID = chosenID
		a.Energy -= 10
		task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s tracked %s in %s (3 ticks)", a.Name, chosenID, a.Location),
			"threat_id":       chosenID,
			"remaining_ticks": task.Remaining,
		})}

	case "perform_ritual":
		if a.InsideHouse != "" {
			return fail("step outside to shrine precinct before performing ritual")
		}
		var rit *RitualSpec
		for i := range w.Catalog.Rituals {
			cand := &w.Catalog.Rituals[i]
			if x.RecipeID != "" && cand.ID != x.RecipeID {
				continue
			}
			if cand.NodeFeature != "" && !w.Map.DistrictHasFeature(a.Location, cand.NodeFeature) {
				continue
			}
			rit = cand
			break
		}
		if rit == nil {
			return fail("ritual cannot be performed at current location")
		}
		x.RecipeID = rit.ID
		if a.Energy < rit.EnergyCost {
			return fail("insufficient energy for ritual")
		}
		if a.Money < rit.MoneyCost {
			return fail("insufficient money for ritual")
		}
		for inItem, inQty := range rit.Inputs {
			if a.Items[inItem] < inQty {
				return fail(fmt.Sprintf("ritual %s requires %d %s", rit.ID, inQty, inItem))
			}
		}
		a.Energy -= rit.EnergyCost
		if rit.MoneyCost > 0 {
			if len(w.Cohorts) > 0 {
				w.adjustActorCurrency(a, "coins", -int64(rit.MoneyCost), false)
				w.creditTreasury(int64(rit.MoneyCost), "fee")
			} else {
				w.adjustActorCurrency(a, "coins", -int64(rit.MoneyCost), true)
			}
		}
		for inItem, inQty := range rit.Inputs {
			w.adjustActorItem(a, inItem, -inQty, true)
		}
		ticks := rit.Ticks
		if ticks < 1 {
			ticks = 1
		}
		task := Task{Action: x, TotalTicks: ticks, Remaining: ticks - 1, StartedTick: w.Tick}
		if task.Remaining <= 0 {
			return w.finishTask(a, task)
		}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s began conducting %s (%d ticks)", a.Name, rit.Name, ticks),
			"ritual_id":       rit.ID,
			"remaining_ticks": task.Remaining,
		})}

	case "repair_house":
		h := w.house(x.HouseID)
		if h == nil || h.District != a.Location || (a.InsideHouse != "" && a.InsideHouse != h.ID) {
			return fail("house is not at current location")
		}
		if h.Condition >= MaxCondition {
			return fail("house does not need repair")
		}
		if a.Energy < 8 {
			return fail("insufficient energy to repair house")
		}
		if a.Items["plank"] < 1 {
			return fail("repairing a house requires 1 plank")
		}
		a.Energy -= 8
		w.adjustActorItem(a, "plank", -1, true)
		task := Task{Action: x, TotalTicks: 3, Remaining: 2, StartedTick: w.Tick}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s started repairing %s (3 ticks)", a.Name, h.Name),
			"house_id":        h.ID,
			"remaining_ticks": task.Remaining,
		})}

	case "upgrade_house":
		h := w.house(x.HouseID)
		if h == nil {
			return fail("unknown house: " + x.HouseID)
		}
		if h.OwnerID != a.ID {
			return fail("only the house owner may upgrade " + h.Name)
		}
		if a.InsideHouse != h.ID {
			return fail("must be inside " + h.Name + " to renovate it")
		}
		if h.Level >= 3 {
			return fail("house is already at maximum asset level")
		}
		if a.Professions["carpenter"] < 2 {
			return fail("renovating a house requires carpenter profession level 2")
		}
		planks := h.Level
		coins := 4 * h.Level
		if a.Items["plank"] < planks {
			return fail(fmt.Sprintf("renovation requires %d planks", planks))
		}
		if a.Money < coins {
			return fail(fmt.Sprintf("renovation requires %d coins", coins))
		}
		if a.Energy < 12 {
			return fail("insufficient energy to renovate")
		}
		w.adjustActorItem(a, "plank", -planks, true)
		if coins > 0 {
			if len(w.Cohorts) > 0 {
				w.adjustActorCurrency(a, "coins", -int64(coins), false)
				w.creditTreasury(int64(coins), "fee")
			} else {
				w.adjustActorCurrency(a, "coins", -int64(coins), true)
			}
		}
		a.Energy -= 12
		task := Task{Action: x, TotalTicks: 4, Remaining: 3, StartedTick: w.Tick}
		a.Task = &task
		return []Event{w.record(a, x, "task_started", map[string]any{
			"text":            fmt.Sprintf("%s started renovating %s to level %d (4 ticks)", a.Name, h.Name, h.Level+1),
			"house_id":        h.ID,
			"remaining_ticks": task.Remaining,
		})}

	case "transfer_house":
		h := w.house(x.HouseID)
		if h == nil || h.OwnerID != a.ID {
			return fail("actor does not own house " + x.HouseID)
		}
		if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("recipient is not nearby")
		}
		h.OwnerID = b.ID
		if !h.CanAccess(b.ID) {
			h.Residents = append(h.Residents, b.ID)
		}
		if b.HomeID == "" {
			b.HomeID = h.ID
		}
		a.Trust[b.ID] += 2
		b.Trust[a.ID] += 2
		return emit("house_transferred", fmt.Sprintf("%s transferred ownership of %s to %s", a.Name, h.Name, b.Name), b)

	case "attack":
		if blocked, reason := w.IsActionBlockedByWorldLaw(*a, x); blocked {
			return fail(reason)
		}
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("target is not nearby")
		}
		if a.Energy < 6 {
			return fail("insufficient energy to attack")
		}
		if a.Health < 25 {
			return fail("too injured to fight; must recuperate")
		}
		a.Energy -= 6
		if b.Trust == nil {
			b.Trust = map[string]int{}
		}
		if len(w.Catalog.CombatProfiles) > 0 || len(w.ActiveEffects) > 0 {
			dmg, absorbed, immune, broken := w.resolveMultiDimCombatHit(*a, b, nil, x)
			if dmg > b.Health {
				dmg = b.Health
			}
			b.Health -= dmg
			var interruptEvs []Event
			if dmg > 0 && b.Health > 0 {
				interruptEvs = w.interruptTaskOnDamage(b, "combat")
			}
			a.RestUntil = w.Tick + 32
			b.Trust[a.ID] -= 2
			ev := w.record(a, x, "attacked", map[string]any{
				"text":           fmt.Sprintf("%s attacked %s (%d damage, %d absorbed, immune=%v)", a.Name, b.Name, dmg, absorbed, immune),
				"damage":         dmg,
				"absorbed":       absorbed,
				"immune_blocked": immune,
			}, b)
			events := append(broken, ev)
			events = append(events, interruptEvs...)
			events = append(events, w.grantCombatXP(a, x, 4, 1)...)
			events = append(events, w.markDeadIfZero(b, a, x, "combat attack")...)
			return events
		}
		damage := 8 + (CombatPowerWithCatalog(w.Catalog, *a)-10)/2
		if damage < 10 {
			damage = 10
		}
		if damage > b.Health {
			damage = b.Health
		}
		b.Health -= damage
		var interruptEvs []Event
		if damage > 0 && b.Health > 0 {
			interruptEvs = w.interruptTaskOnDamage(b, "combat")
		}
		a.RestUntil = w.Tick + 32
		b.Trust[a.ID] -= 2
		ev := w.record(a, x, "attacked", map[string]any{
			"text":   fmt.Sprintf("%s attacked %s for %d damage", a.Name, b.Name, damage),
			"damage": damage,
		}, b)
		events := []Event{ev}
		events = append(events, interruptEvs...)
		events = append(events, w.grantCombatXP(a, x, 4, 1)...)
		events = append(events, w.markDeadIfZero(b, a, x, "combat attack")...)
		return events

	case "spar":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("sparring partner is not nearby")
		}
		if a.Energy < 6 {
			return fail("insufficient energy to spar")
		}
		if w.Tick < a.RestUntil {
			return fail("still recuperating from recent bout")
		}
		if a.Health < 65 || b.Health < 65 {
			return fail("fighter is injured and must recuperate before sparring")
		}
		a.Energy -= 6
		if a.Health > 60 {
			a.Health -= 2
		}
		if b.Health > 60 {
			b.Health -= 2
		}
		a.RestUntil = w.Tick + 48
		if b.RestUntil < w.Tick+24 {
			b.RestUntil = w.Tick + 24
		}
		a.Trust[b.ID]++
		b.Trust[a.ID]++
		atkPower := CombatPowerWithCatalog(w.Catalog, *a)
		defPower := CombatPowerWithCatalog(w.Catalog, *b)
		ev := w.record(a, x, "sparred", map[string]any{
			"text":           fmt.Sprintf("%s sparred with %s in martial arts practice (power %d vs %d)", a.Name, b.Name, atkPower, defPower),
			"attacker_power": atkPower,
			"defender_power": defPower,
		}, b)
		events := []Event{ev}
		events = append(events, w.grantCombatXP(a, x, 6, 2)...)
		events = append(events, w.grantCombatXP(b, x, 4, 1)...)
		return events

	case "steal":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("target is not nearby")
		}
		if a.Energy < 5 {
			return fail("insufficient energy to steal")
		}
		if a.Health < 45 || w.Tick < a.RestUntil {
			return fail("thief is injured or laying low to recuperate")
		}
		a.Energy -= 5
		stealth := a.Attributes.Dexterity + a.Professions["thief"]*3 + a.Professions["master_thief"]*4
		if b.Task != nil && b.Task.Action.Kind == "sleep" {
			stealth += 4
		}
		vigilance := b.Attributes.Insight + b.Professions["constable"]*3 + b.Professions["chief_constable"]*4 + b.Professions["martial_artist"]*2 + b.Professions["guard"]*2
		if ClockAt(w.Tick).IsNight && w.IsCurfewActiveAt(a.Location, a.VenueID) {
			vigilance += 8
		}
		if stealth >= vigilance && (b.Money > 0 || len(b.Items) > 0) {
			maxCoins := 2 + a.Professions["thief"] + a.Professions["master_thief"]*2
			stolenCoins := b.Money
			if stolenCoins > maxCoins {
				stolenCoins = maxCoins
			}
			b.Money -= stolenCoins
			a.Money += stolenCoins

			stolenItem := ""
			stolenQty := 0
			for _, candidate := range []string{"choice_grain", "medicine", "tonic", "scroll", "tools", "plank", "food"} {
				if b.Items[candidate] > 0 {
					stolenItem = candidate
					stolenQty = 1
					w.adjustActorItem(b, candidate, -1, false)
					w.adjustActorItem(a, candidate, 1, false)
					break
				}
			}
			a.Tags = appendUnique(a.Tags, "outlaw")
			a.RestUntil = w.Tick + 32
			b.Trust[a.ID] -= 2
			ev := w.record(a, x, "stolen", map[string]any{
				"text":     fmt.Sprintf("%s pickpocketed %d coins and %d %s from %s", a.Name, stolenCoins, stolenQty, stolenItem, b.Name),
				"coins":    stolenCoins,
				"item":     stolenItem,
				"quantity": stolenQty,
			}, b)
			return append([]Event{ev}, w.grantProfessionXP(a, x, "thief", 6, 2)...)
		}
		dmg := 12
		if dmg >= a.Health {
			dmg = a.Health - 1
		}
		if dmg > 0 {
			a.Health -= dmg
		}
		a.Tags = appendUnique(a.Tags, "outlaw")
		a.RestUntil = w.Tick + 64
		b.Trust[a.ID] -= 3
		ev := w.record(a, x, "steal_foiled", map[string]any{
			"text":   fmt.Sprintf("%s was caught trying to steal from %s (-%d health)", a.Name, b.Name, dmg),
			"damage": dmg,
		}, b)
		return append([]Event{ev}, w.grantProfessionXP(a, x, "thief", 2, 1)...)

	case "raid":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("target is not nearby")
		}
		if a.Energy < 8 {
			return fail("insufficient energy to raid")
		}
		if a.Health < 55 || w.Tick < a.RestUntil {
			return fail("raider is injured or recuperating after recent raid")
		}
		a.Energy -= 8
		atkPower := CombatPowerWithCatalog(w.Catalog, *a)
		defPower := CombatPowerWithCatalog(w.Catalog, *b)
		profID := "bandit"
		if a.Professions["gangster"] > a.Professions["bandit"] {
			profID = "gangster"
		}
		a.Tags = appendUnique(a.Tags, "outlaw")
		if atkPower >= defPower {
			dmg := 14 + (atkPower-defPower)/2
			if dmg > 35 {
				dmg = 35
			}
			if dmg > b.Health {
				dmg = b.Health
			}
			b.Health -= dmg
			if b.Task != nil && b.Task.Action.Kind == "sleep" {
				b.Task = nil
			}
			if defPower >= 10 && a.Health > 30 {
				a.Health -= 4
			}
			a.RestUntil = w.Tick + 64
			if b.RestUntil < w.Tick+48 {
				b.RestUntil = w.Tick + 48
			}
			maxCoins := 3 + a.Professions[profID]*2
			lootCoins := b.Money
			if lootCoins > maxCoins {
				lootCoins = maxCoins
			}
			b.Money -= lootCoins
			a.Money += lootCoins

			lootItem := ""
			lootQty := 0
			for _, candidate := range []string{"choice_grain", "food", "plank", "tools", "medicine", "timber", "ore"} {
				if b.Items[candidate] > 0 {
					lootItem = candidate
					lootQty = 1
					w.adjustActorItem(b, candidate, -1, false)
					w.adjustActorItem(a, candidate, 1, false)
					break
				}
			}
			b.Trust[a.ID] -= 4
			ev := w.record(a, x, "raided", map[string]any{
				"text":     fmt.Sprintf("%s raided %s for %d coins and %d %s (%d damage)", a.Name, b.Name, lootCoins, lootQty, lootItem, dmg),
				"coins":    lootCoins,
				"item":     lootItem,
				"quantity": lootQty,
				"damage":   dmg,
			}, b)
			events := []Event{ev}
			events = append(events, w.grantProfessionXP(a, x, profID, 8, 3)...)
			events = append(events, w.markDeadIfZero(b, a, x, "armed raid")...)
			return events
		}
		dmg := 16 + (defPower-atkPower)/2
		if dmg > 40 {
			dmg = 40
		}
		if dmg > a.Health {
			dmg = a.Health
		}
		a.Health -= dmg
		a.RestUntil = w.Tick + 96
		b.Trust[a.ID] -= 3
		ev := w.record(a, x, "raid_repelled", map[string]any{
			"text":   fmt.Sprintf("%s repelled a raid by %s (-%d health to attacker)", b.Name, a.Name, dmg),
			"damage": dmg,
		}, b)
		events := []Event{ev}
		events = append(events, w.grantCombatXP(b, x, 6, 2)...)
		events = append(events, w.markDeadIfZero(a, b, x, "repelled raid clash")...)
		return events

	case "extort":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("target is not nearby")
		}
		if ActorHasTag(b.IdentityTags(), "outlaw") {
			return fail("cannot extort fellow outlaw")
		}
		if a.Energy < 6 {
			return fail("insufficient energy to extort")
		}
		if a.Health < 50 || w.Tick < a.RestUntil {
			return fail("enforcer is injured or resting")
		}
		a.Energy -= 6
		a.Tags = appendUnique(a.Tags, "outlaw")
		atkPower := CombatPowerWithCatalog(w.Catalog, *a) + 2
		defPower := CombatPowerWithCatalog(w.Catalog, *b)
		if atkPower >= defPower && b.Money >= 2 {
			fee := 2 + a.Professions["gangster"] + a.Professions["gang_boss"]*2
			if fee > b.Money/3+1 {
				fee = b.Money/3 + 1
			}
			if fee > b.Money {
				fee = b.Money
			}
			b.Money -= fee
			a.Money += fee
			b.Trust[a.ID] -= 2
			a.RestUntil = w.Tick + 32
			w.addDistrictCorruption(a.Location, 1)
			ev := w.record(a, x, "extorted", map[string]any{
				"text":             fmt.Sprintf("%s collected %d protection-fee coins from %s in %s", a.Name, fee, b.Name, a.Location),
				"protection_coins": fee,
				"district":         a.Location,
			}, b)
			events := []Event{ev}
			gangProf := "gangster"
			if a.Professions["gang_boss"] > 0 {
				gangProf = "gang_boss"
			}
			return append(events, w.grantProfessionXP(a, x, gangProf, 5, 2)...)
		}
		dmg := 8
		if dmg > a.Health-1 {
			dmg = maxInt(0, a.Health-1)
		}
		a.Health -= dmg
		a.RestUntil = w.Tick + 48
		b.Trust[a.ID] -= 2
		ev := w.record(a, x, "extort_resisted", map[string]any{
			"text":   fmt.Sprintf("%s refused protection extortion from %s (-%d health to enforcer)", b.Name, a.Name, dmg),
			"damage": dmg,
		}, b)
		return []Event{ev}

	case "bribe":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("official is not nearby")
		}
		if !w.Catalog.Capabilities(*b)["arrest"] {
			return fail("target does not hold law-enforcement authority")
		}
		if a.Energy < 3 {
			return fail("insufficient energy")
		}
		if a.Money < 4 {
			return fail("insufficient coins to offer bribe")
		}
		a.Energy -= 3
		bribeCoins := minInt(a.Money/3+2, 10)
		if bribeCoins > a.Money {
			bribeCoins = a.Money
		}
		a.Money -= bribeCoins
		b.Money += bribeCoins
		b.IllicitGain += bribeCoins
		a.Trust[b.ID] += 2
		b.Trust[a.ID] += 3
		w.addDistrictCorruption(a.Location, 2)
		if w.DistrictStates != nil {
			if ds, hasDS := w.DistrictStates[a.Location]; hasDS {
				if ds.BribeShieldTicks < 48 {
					ds.BribeShieldTicks = 48
				}
				w.DistrictStates[a.Location] = ds
			}
		}
		observers := append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)
		ev := w.record(a, x, "bribed", map[string]any{
			"text":        fmt.Sprintf("%s slipped %d hush-bribe coins to %s in %s", a.Name, bribeCoins, b.Name, a.Location),
			"bribe_coins": bribeCoins,
			"district":    a.Location,
			"observation": Observation{
				Kind:      "corruption",
				SubjectID: b.ID,
				ObjectID:  a.ID,
				Item:      "coins",
				Location:  a.Location,
				Quantity:  bribeCoins,
			},
		}, observers...)
		return []Event{ev}

	case "audit_corruption":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("suspect is not nearby")
		}
		if a.IllicitGain > 0 {
			return fail("corrupt officer cannot conduct anti-corruption audit")
		}
		if a.Energy < 6 {
			return fail("insufficient energy to conduct audit")
		}
		basisID, hasEv := ActorCorruptionEvidence(*a, b.ID)
		if !hasEv {
			return fail("no witnessed or reported corruption evidence against target")
		}
		if x.BasisEventID != 0 && !ActorCorruptionEvidenceWithBasis(*a, b.ID, x.BasisEventID) {
			return fail("basis event does not implicate target in corruption")
		}
		if x.BasisEventID == 0 {
			x.BasisEventID = basisID
		}
		if b.IllicitGain <= 0 {
			return fail("target holds no illicit corrupt gains")
		}
		a.Energy -= 6
		seized := minInt(b.IllicitGain, b.Money)
		b.Money -= seized
		b.IllicitGain = 0
		reward := maxInt(1, seized/5)
		if reward > seized {
			reward = seized
		}
		toTreasury := seized - reward
		a.Money += reward
		if toTreasury > 0 {
			w.creditTreasury(int64(toTreasury), "confiscated")
		}
		w.addDistrictCorruption(a.Location, -8)
		jailedUntil := int64(0)
		if seized >= 4 && b.Health > 0 {
			b.Task = nil
			b.InsideHouse = ""
			b.JailedUntil = w.Tick + 144
			jailedUntil = b.JailedUntil
		}
		observers := append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)
		ev := w.record(a, x, "corruption_purged", map[string]any{
			"text":              fmt.Sprintf("%s audited and exposed %s for corruption, confiscating %d illicit coins (%d to treasury, %d audit reward)", a.Name, b.Name, seized, toTreasury, reward),
			"confiscated_coins": toTreasury,
			"reward_coins":      reward,
			"jailed_until":      jailedUntil,
		}, observers...)
		events := []Event{ev}
		for _, profID := range []string{"xiucai", "chief_constable", "scholar", "constable"} {
			if a.Professions[profID] > 0 {
				events = append(events, w.grantProfessionXP(a, x, profID, 8, 3)...)
				break
			}
		}
		return events

	case "arrest":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("target is not nearby")
		}
		bTags := b.IdentityTags()
		if !ActorHasTag(bTags, "outlaw") && !ActorHasTag(bTags, "raider") && !ActorHasTag(bTags, "thief") && !ActorHasTag(bTags, "gang") && a.Trust[b.ID] >= 0 {
			return fail("target is a law-abiding resident")
		}
		if a.Energy < 7 {
			return fail("insufficient energy to arrest")
		}
		if a.Health < 45 || w.Tick < a.RestUntil {
			return fail("constable is injured or resting from recent patrol clash")
		}
		a.Energy -= 7
		lawPower := CombatPowerWithCatalog(w.Catalog, *a) + 3
		usedWrit := false
		if a.Items["bounty_writ"] > 0 {
			lawPower += 5
			usedWrit = true
		}
		outlawPower := CombatPowerWithCatalog(w.Catalog, *b)
		lawProf := "constable"
		if a.Professions["constable"] == 0 && a.Professions["guard"] > 0 {
			lawProf = "guard"
		}
		if lawPower >= outlawPower {
			dmg := 12
			if b.Health <= dmg && outlawPower < 18 {
				dmg = b.Health - 1
			}
			if dmg > b.Health {
				dmg = b.Health
			}
			if dmg > 0 {
				b.Health -= dmg
			}
			b.Task = nil
			b.InsideHouse = ""
			a.RestUntil = w.Tick + 16
			confiscated := b.Money / 2
			if confiscated > 8 {
				confiscated = 8
			}
			w.adjustActorCurrency(b, "coins", -int64(confiscated), false)
			baseOfficial := 4
			if usedWrit {
				baseOfficial += 3
				w.adjustActorItem(a, "bounty_writ", -1, true)
			}
			if len(w.Cohorts) > 0 {
				baseOfficial = int(w.debitTreasury(int64(baseOfficial), "bounty"))
			}
			w.adjustActorCurrency(a, "coins", int64(baseOfficial), len(w.Cohorts) == 0)
			w.adjustActorCurrency(a, "coins", int64(confiscated), false)
			bounty := baseOfficial + confiscated
			if b.Health > 0 {
				b.JailedUntil = w.Tick + 192
				filtered := w.Offers[:0]
				for _, o := range w.Offers {
					if o.Seller != b.ID && o.Buyer != b.ID {
						filtered = append(filtered, o)
					}
				}
				w.Offers = filtered
				if len(w.Offers) == 0 {
					w.Offers = nil
				}
			}
			ev := w.record(a, x, "arrested", map[string]any{
				"text":         fmt.Sprintf("%s arrested outlaw %s, imprisoning until tick %d, confiscating %d coins and claiming %d bounty (%d damage)", a.Name, b.Name, b.JailedUntil, confiscated, bounty, dmg),
				"coins":        bounty,
				"confiscated":  confiscated,
				"damage":       dmg,
				"jailed_until": b.JailedUntil,
			}, b)
			events := []Event{ev}
			events = append(events, w.grantProfessionXP(a, x, lawProf, 8, 3)...)
			events = append(events, w.markDeadIfZero(b, a, x, "violent arrest resistance")...)
			return events
		}
		dmg := 14
		if dmg > a.Health {
			dmg = a.Health
		}
		a.Health -= dmg
		a.RestUntil = w.Tick + 64
		ev := w.record(a, x, "arrest_resisted", map[string]any{
			"text":   fmt.Sprintf("outlaw %s resisted arrest by %s (-%d health)", b.Name, a.Name, dmg),
			"damage": dmg,
		}, b)
		events := []Event{ev}
		events = append(events, w.grantProfessionXP(a, x, lawProf, 3, 1)...)
		events = append(events, w.markDeadIfZero(a, b, x, "resisting outlaw counterattack")...)
		return events

	case "heal":
		if b == nil || b.ID == id || !SamePlace(*a, *b) {
			return fail("patient is not nearby")
		}
		if b.Health >= MaxHealth && b.DiseaseID == "" {
			return fail("patient has no injury or illness")
		}
		itemID := x.Item
		if itemID == "" {
			for k, spec := range w.Catalog.Items {
				if (spec.MedicinalHeal > 0 || spec.DiseaseCure > 0) && a.Items[k] >= 1 {
					if itemID == "" || spec.MedicinalHeal < w.Catalog.Items[itemID].MedicinalHeal {
						itemID = k
					}
				}
			}
			x.Item = itemID
		}
		spec, ok := w.Catalog.Items[itemID]
		if !ok || (spec.MedicinalHeal <= 0 && spec.DiseaseCure <= 0) || a.Items[itemID] < 1 {
			return fail("no medicine")
		}
		amount := maxInt(15, spec.MedicinalHeal-5)
		if amount > MaxHealth-b.Health {
			amount = MaxHealth - b.Health
		}
		if amount < 0 {
			amount = 0
		}
		b.Health += amount
		if b.RestUntil > w.Tick+16 {
			b.RestUntil -= 16
		}
		curedDisease := ""
		if b.DiseaseID != "" && spec.DiseaseCure > 0 {
			b.DiseaseTicks -= spec.DiseaseCure * TicksPerDay
			if b.DiseaseTicks <= 0 {
				curedDisease = b.DiseaseID
				b.DiseaseID = ""
				b.DiseaseTicks = 0
			}
		}
		w.adjustActorItem(a, itemID, -1, true)
		a.Trust[b.ID]++
		b.Trust[a.ID] += 2
		ev := w.record(a, x, "healed", map[string]any{
			"text":          fmt.Sprintf("%s healed %s for %d health", a.Name, b.Name, amount),
			"healed":        amount,
			"health":        b.Health,
			"cured_disease": curedDisease,
		}, b)
		events := []Event{ev}
		for _, prof := range w.Catalog.Professions {
			if a.Professions[prof.ID] > 0 && ActorHasTag(prof.Grants, "healer") {
				return append(events, w.grantProfessionXP(a, x, prof.ID, 6, 2)...)
			}
		}
		return events

	case "speak":
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("listener is not nearby")
		}
		if x.Text == "" {
			return fail("empty speech")
		}
		return emit("speech", x.Text, b)

	case "offer":
		if x.OfferID != 0 {
			return w.counterOffer(a, x)
		}
		if b == nil || b.ID == id || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
			return fail("buyer is not nearby")
		}
		spec, ok := w.Catalog.Items[x.Item]
		if !ok || !spec.Tradeable {
			return fail("item is not tradeable: " + x.Item)
		}
		if w.IsItemEmbargoedAt(a.Location, a.VenueID, x.Item) {
			return fail("item is embargoed by local polity decree: " + x.Item)
		}
		cur := x.Currency
		if cur == "" {
			cur = "coins"
		}
		if !itemAcceptsCurrency(spec, cur) {
			return fail(fmt.Sprintf("item %s does not accept currency %s", x.Item, cur))
		}
		if spec.BuyerRequiredTag != "" && !ActorHasTag(w.actorEffectiveTags(*b), spec.BuyerRequiredTag) {
			return fail(fmt.Sprintf("buyer %s lacks required tag %s for %s", b.ID, spec.BuyerRequiredTag, x.Item))
		}
		if x.Quantity <= 0 || x.Price < 0 || a.Items[x.Item] < x.Quantity {
			return fail("invalid offer or insufficient item stock")
		}
		offerText := fmt.Sprintf("%s offers %d %s (tier %d) for %d %s", a.Name, x.Quantity, x.Item, spec.Tier, x.Price, cur)
		e := w.record(a, x, "trade_offered", map[string]any{
			"text":     offerText,
			"currency": cur,
		}, b)
		w.Offers = append(w.Offers, Offer{
			ID:       e.ID,
			Seller:   id,
			Buyer:    b.ID,
			Item:     x.Item,
			Quantity: x.Quantity,
			Price:    x.Price,
			Currency: x.Currency,
			Expires:  w.Tick + 12,
		})
		return []Event{e}

	case "accept", "reject":
		for i, o := range w.Offers {
			if o.ID != x.OfferID || o.ResponderID() != id {
				continue
			}
			seller := w.actor(o.Seller)
			buyer := w.actor(o.Buyer)
			x.TargetID = o.ProposerID()
			if x.Kind == "reject" {
				w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
				if len(w.Offers) == 0 {
					w.Offers = nil
				}
				return emit("trade_rejected", a.Name+" rejected offer", w.actor(o.ProposerID()))
			}
			cur := o.Currency
			if cur == "" {
				cur = "coins"
			}
			if buyer == nil {
				return fail("buyer is no longer available")
			}
			if reason := w.offerSettlementBlockReason(*buyer, o); reason != "" {
				return fail(reason)
			}
			w.adjustActorItem(seller, o.Item, -o.Quantity, false)
			w.adjustActorItem(buyer, o.Item, o.Quantity, false)
			taxCoins := w.offerTax(*buyer, o)
			pol := w.EffectiveGoverningPolity(buyer.Location, buyer.VenueID)
			if cur == "coins" {
				buyer.Money -= o.Price
				seller.Money += (o.Price - taxCoins)
				if taxCoins > 0 {
					if pol != nil {
						pol.Treasury += int64(taxCoins)
						pol.Ledger.TaxAssessedGross += int64(taxCoins)
						pol.Ledger.TaxToTreasury += int64(taxCoins)
					} else {
						w.creditTreasury(int64(taxCoins), "tax")
					}
				}
			} else {
				if pol == nil {
					taxCoins = 0
				}
				w.addActorCurrency(buyer, cur, -int64(o.Price))
				w.addActorCurrency(seller, cur, int64(o.Price-taxCoins))
				if taxCoins > 0 && pol != nil {
					w.addPolityCurrency(pol, cur, int64(taxCoins))
				}
			}
			if buyer.Trust == nil {
				buyer.Trust = map[string]int{}
			}
			if seller.Trust == nil {
				seller.Trust = map[string]int{}
			}
			buyer.Trust[seller.ID]++
			seller.Trust[buyer.ID]++
			w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
			if len(w.Offers) == 0 {
				w.Offers = nil
			}
			x.Item, x.Quantity, x.Price, x.Currency = o.Item, o.Quantity, o.Price, o.Currency
			events := []Event{w.record(a, x, "trade_completed", map[string]any{
				"text":       fmt.Sprintf("%s bought %d %s from %s for %d %s", buyer.Name, o.Quantity, o.Item, seller.Name, o.Price, cur),
				"item":       o.Item,
				"quantity":   o.Quantity,
				"seller_id":  seller.ID,
				"price":      o.Price,
				"currency":   cur,
				"market_tax": taxCoins,
			}, w.actor(o.ProposerID()))}
			tier := w.Catalog.Items[o.Item].Tier
			if tier < 1 {
				tier = 1
			}
			events = append(events, w.grantProfessionXP(seller, x, "peddler", tier*o.Quantity, tier)...)
			return append(events, w.grantProfessionXP(buyer, x, "peddler", tier*o.Quantity, tier)...)
		}
		return fail("offer not found")

	case "attempt_transition":
		return w.applyAttemptTransition(a, x)

	case "refine_currency_item":
		return w.applyRefineCurrencyItem(a, x)

	case "absorb_item_slot":
		return w.applyAbsorbItemSlot(a, x)

	case "sever_flow_edge":
		return w.applySeverFlowEdge(a, x)

	case "repair_flow_edge":
		return w.applyRepairFlowEdge(a, x)

	case "research_tech":
		return w.applyResearchTech(a, x)

	case "teach_knowledge":
		return w.applyTeachKnowledge(a, x, b)

	case "adopt_facility":
		return w.applyAdoptFacility(a, x)

	case "rewrite_world_law":
		return w.applyRewriteWorldLaw(a, x)

	case "start_plan", "abandon_plan", "propose_agreement", "accept_agreement", "reject_agreement", "fulfill_agreement":
		return w.ApplyContinuity(a, x)

	case "borrow_loan", "accept_loan", "reject_loan", "cancel_loan", "repay_loan", "foreclose_collateral":
		if evs, ok := w.ApplyMarketCreditAction(w.ensureMarketCredit(), id, x); ok {
			return evs
		}
		return fail("market credit action failed: " + x.Kind)

	case "marry", "swear_bond", "avenge_feud", "mediate_feud", "claim_turf":
		if evs, ok := w.ApplyRelationAction(w.ensureRelations(), id, x); ok {
			return evs
		}
		return fail("relation action failed: " + x.Kind)

	case "frame_suspect", "file_appeal":
		if evs, ok := w.ApplyJusticeAction(w.ensureJusticeRevolt(), id, x); ok {
			return evs
		}
		return fail("justice action failed: " + x.Kind)

	default:
		if spec, ok := w.Catalog.ActionSpecs[x.Kind]; ok {
			return w.applyDeclarativeAction(a, x, spec)
		}
		return fail("unknown action: " + x.Kind)
	}
}

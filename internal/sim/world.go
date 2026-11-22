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
	SecretID  string `json:"secret_id,omitempty"`
	Kind      string `json:"kind"`
	Modality  string `json:"modality,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	ObjectID  string `json:"object_id,omitempty"`
	CaseID    int64  `json:"case_id,omitempty"`
	Item      string `json:"item"`
	Location  string `json:"location"`
	Quantity  int    `json:"quantity"`
	Proceeds  int    `json:"proceeds,omitempty"`
}

// Statement is a speaker's structured assertion inside speech (never an objective world fact).
type Statement struct {
	Topic     string `json:"topic"`
	Modality  string `json:"modality,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	ObjectID  string `json:"object_id,omitempty"`
	CaseID    int64  `json:"case_id,omitempty"`
	Item      string `json:"item"`
	Location  string `json:"location"`
	Quantity  int    `json:"quantity"`
	Proceeds  int    `json:"proceeds,omitempty"`
}

type Memory struct {
	search       *memorySearchCache
	BasisEventID int64        `json:"basis_event_id,omitempty"`
	FactEventID  int64        `json:"fact_event_id,omitempty"`
	CauseEventID int64        `json:"cause_event_id,omitempty"`
	WitnessID    string       `json:"witness_id,omitempty"`
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
	Action            Action           `json:"action"`
	TotalTicks        int              `json:"total_ticks"`
	Remaining         int              `json:"remaining_ticks"`
	WorkCredit        int              `json:"work_credit,omitempty"` // hundredths of a crafting work tick
	StartedTick       int64            `json:"started_tick"`
	Yield             int              `json:"yield,omitempty"`
	FundedHundredths  int64            `json:"funded_hundredths,omitempty"`
	FieldID           string           `json:"field_id,omitempty"`
	SleepTenths       int              `json:"sleep_tenths,omitempty"`
	StartEnergy       int              `json:"start_energy,omitempty"`
	Dormant           bool             `json:"dormant,omitempty"`
	PrepaidCurrencies map[string]int64 `json:"prepaid_currencies,omitempty"`
	PrepaidTreasury   bool             `json:"prepaid_treasury,omitempty"`
	PrepaidItems      map[string]int   `json:"prepaid_items,omitempty"`
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
	Skills           map[string]int    `json:"skills,omitempty"`
	SkillXP          map[string]int    `json:"skill_xp,omitempty"`
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
	DeathCause       string            `json:"death_cause,omitempty"`
	DeathTick        int64             `json:"death_tick,omitempty"`
	DeathEventID     int64             `json:"death_event_id,omitempty"`
	EstateSettled    bool              `json:"estate_settled,omitempty"`
	KnownHomicides   []int64           `json:"known_homicides,omitempty"`
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
	Hobbies          []string          `json:"hobbies,omitempty"`
	Trust            map[string]int    `json:"trust"`
	Task             *Task             `json:"task,omitempty"`
	Memories         []Memory          `json:"memories,omitempty"`
	Ambitions        []ActorAmbition   `json:"ambitions,omitempty"`
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
	Production          ProductionState          `json:"production"`
	Growth              []GrowthBatch            `json:"growth,omitempty"`
	Shipments           []Shipment               `json:"shipments,omitempty"`
	FreightUsage        *FreightCapacityUsage    `json:"freight_usage,omitempty"`
	Continuity          *ContinuityState         `json:"continuity,omitempty"`
	Polities            *PolityNetwork           `json:"polities,omitempty"`
	ID                  string                   `json:"id"`
	Tick                int64                    `json:"tick"`
	NextID              int64                    `json:"next_id"`
	Map                 WorldMap                 `json:"map"`
	Catalog             Catalog                  `json:"catalog"`
	Factions            []Faction                `json:"factions"`
	Houses              []House                  `json:"houses"`
	Actors              []Actor                  `json:"actors"`
	Offers              []Offer                  `json:"offers,omitempty"`
	Cohorts             []PopulationCohort       `json:"cohorts,omitempty"`
	DistrictStates      map[string]DistrictState `json:"district_states,omitempty"`
	DeathsByCause       map[string]int64         `json:"deaths_by_cause,omitempty"`
	TotalBirthsCount    int64                    `json:"total_births_count,omitempty"`
	PublicTreasury      int64                    `json:"public_treasury,omitempty"`
	TreasuryLedger      TreasuryLedger           `json:"treasury_ledger,omitempty"`
	MarketCredit        *MarketCreditState       `json:"market_credit,omitempty"`
	Relations           *RelationEvoState        `json:"relations,omitempty"`
	JusticeRevolt       *JusticeRevoltState      `json:"justice_revolt,omitempty"`
	SpotlightRumor      *SpotlightRumorState     `json:"spotlight_rumor,omitempty"`
	ActiveEffects       []ActiveEffect           `json:"active_effects,omitempty"`
	ActiveWorldEvents   []ActiveWorldEvent       `json:"active_world_events,omitempty"`
	WorldEventLastEnded map[string]int64         `json:"world_event_last_ended,omitempty"`
	Parties             []Party                  `json:"parties,omitempty"`
	Diplomacy           *DiplomacyState          `json:"diplomacy,omitempty"`
	Secrets             []ActorSecret            `json:"secrets,omitempty"`
	FlowGrid            *FlowGridState           `json:"flow_grid,omitempty"`
	LedgerAudit         *LedgerAuditState        `json:"ledger_audit,omitempty"`
	Knowledge           *KnowledgeBook           `json:"knowledge,omitempty"`
	GoldenFinger        *GoldenFingerState       `json:"golden_finger,omitempty"`
	WorldLaws           map[string]int           `json:"world_laws,omitempty"`
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
	SkillID      string               `json:"skill_id,omitempty"`
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
	Married         bool           `json:"married,omitempty"`
	KinshipToSelf   string         `json:"kinship_to_self,omitempty"`
	Resting         bool           `json:"resting,omitempty"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Role            string         `json:"role"`
	FormID          string         `json:"form_id,omitempty"`
	PowerTier       int            `json:"power_tier,omitempty"`
	CombatPower     int            `json:"combat_power,omitempty"`
	Guilds          []string       `json:"guilds,omitempty"`
	Skills          map[string]int `json:"skills,omitempty"`
	Tags            []string       `json:"tags"`
	Personality     string         `json:"personality,omitempty"`
	Hobbies         []string       `json:"hobbies,omitempty"`
	Level           int            `json:"level"`
	Health          int            `json:"health"`
	Sleeping        bool           `json:"sleeping,omitempty"`
	Jailed          bool           `json:"jailed,omitempty"`
	FactionRelation string         `json:"faction_relation,omitempty"`
}

type RoadView struct {
	To          string   `json:"to"`
	Ticks       int      `json:"ticks"`
	EnergyCost  int      `json:"energy_cost"`
	LeadsToward []string `json:"leads_toward,omitempty"`
}

// FactionView is the public summary of a guild visible to every actor.
type FactionView struct {
	LeadershipContestOpen bool           `json:"leadership_contest_open,omitempty"`
	ID                    string         `json:"id"`
	Name                  string         `json:"name"`
	District              string         `json:"district"`
	Level                 int            `json:"level"`
	Professions           []string       `json:"professions"`
	Requirements          map[string]int `json:"requirements,omitempty"`
	Joined                bool           `json:"joined"`
	JoinBlockReason       string         `json:"join_block_reason,omitempty"`
	Members               int            `json:"members"`
}

type CohortView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	District   string `json:"district"`
	Profession string `json:"profession"`
	Population int    `json:"population"`
}

type Perception struct {
	LocalCases             []CaseView            `json:"local_cases,omitempty"`
	KnownWrongfulCases     []KnownWrongfulLead   `json:"known_wrongful_cases,omitempty"`
	KnownFeuds             []FeudView            `json:"known_feuds,omitempty"`
	CreditTerms            MarketCreditSpec      `json:"credit_terms"`
	KnownHouses            []HouseSite           `json:"known_houses,omitempty"`
	LaborUsed              int64                 `json:"labor_used,omitempty"`
	CareAgreements         []GrowthCareAgreement `json:"care_agreements,omitempty"`
	FieldDeeds             []FieldAsset          `json:"field_deeds,omitempty"`
	Fields                 []FieldView           `json:"fields,omitempty"`
	EnergyNodes            []FlowNode            `json:"energy_nodes,omitempty"`
	Growth                 []GrowthBatch         `json:"growth,omitempty"`
	Continuity             ContinuityView        `json:"continuity"`
	ActivePlan             *PlanDirectiveView    `json:"active_plan,omitempty"`
	LoanRequests           []LoanRequest         `json:"loan_requests,omitempty"`
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
	ActiveWorldEvents      []ActiveWorldEvent    `json:"active_world_events,omitempty"`
	Parties                []Party               `json:"parties,omitempty"`
	LedFactions            []string              `json:"led_factions,omitempty"`
	FactionDiplomacy       []FactionDiplomacy    `json:"faction_diplomacy,omitempty"`
	PartyReady             bool                  `json:"party_ready,omitempty"`
	KnownSecrets           []ActorSecret         `json:"known_secrets,omitempty"`
	Offers                 []Offer               `json:"offers,omitempty"`
	Interactions           []InteractionView     `json:"interactions,omitempty"`
	SpotlightSalience      int                   `json:"spotlight_salience,omitempty"`
	HasActiveLoan          bool                  `json:"has_active_loan,omitempty"`
	OverdueBorrowerID      string                `json:"overdue_borrower_id,omitempty"`
	OverdueHouseID         string                `json:"overdue_house_id,omitempty"`
	Married                bool                  `json:"married,omitempty"`
	ActiveGrudgeTargets    []string              `json:"active_grudge_targets,omitempty"`
	ActiveGoldenFingers    []GoldenFingerView    `json:"active_golden_fingers,omitempty"`
	ExpectedCraftYield     map[string]int        `json:"expected_craft_yield,omitempty"`
	ProductionBlockReasons map[string]string     `json:"production_block_reasons,omitempty"`
	DistrictController     string                `json:"district_controller,omitempty"`
	DistrictGrievance      int                   `json:"district_grievance,omitempty"`
	KnownTechs             []string              `json:"known_techs,omitempty"`
	WorldLaws              map[string]int        `json:"world_laws,omitempty"`
	FlowEdges              []FlowEdge            `json:"flow_edges,omitempty"`
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
// FormSpec CombatBonus (cultivation/power realm), independent SkillSpec CombatBonusPerLevel,
// Catalog profession CombatBonusPerLevel, and Catalog item CombatBonus.
func CombatPowerWithCatalog(cat Catalog, a Actor) int {
	power := a.Attributes.Strength + a.Attributes.Dexterity/2
	if a.FormID != "" {
		if form, ok := cat.Forms[a.FormID]; ok && form.CombatBonus > 0 {
			power += form.CombatBonus
		}
	}
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := cat.Skills[sid]; ok && sk.CombatBonusPerLevel > 0 {
			power += lv * sk.CombatBonusPerLevel
		}
	}
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
	power += cat.GoldenFingerCombatBonus(a)
	return power
}

// CombatPower returns an actor's combat effectiveness against DefaultCatalog.
func CombatPower(a Actor) int {
	return CombatPowerWithCatalog(DefaultCatalog(), a)
}

func (w *World) grantCombatXP(tx *WorldTx, a *Actor, x Action, xp, influence int) []Event {
	var events []Event
	for _, sid := range sortedProductionKeys(a.Skills) {
		if a.Skills[sid] <= 0 {
			continue
		}
		if sk, ok := w.Catalog.Skills[sid]; ok && sk.CombatBonusPerLevel > 0 {
			events = append(events, w.grantSkillXP(tx, a, x, sid, xp)...)
		}
	}
	for _, prof := range w.Catalog.Professions {
		if prof.Tier == 1 && prof.CombatBonusPerLevel > 0 && a.Professions[prof.ID] > 0 {
			return append(events, w.grantProfessionXP(tx, a, x, prof.ID, xp, influence)...)
		}
	}
	for _, prof := range w.Catalog.Professions {
		if prof.CombatBonusPerLevel > 0 && a.Professions[prof.ID] > 0 {
			return append(events, w.grantProfessionXP(tx, a, x, prof.ID, xp, influence)...)
		}
	}
	return events
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

// FactionJoinEligibilityReason returns a non-empty block reason if actor a is ineligible to join
// faction f (already a member, charter attributes unmet, or f is at war / hostile with any of a's guilds).
// Both Perception/Options and World.Apply("join_faction") use this single source of truth.
func (w *World) FactionJoinEligibilityReason(a Actor, f Faction) string {
	if f.HasMember(a.ID) || ActorHasTag(a.Guilds, f.ID) {
		return "already a member of " + f.Name
	}
	if ok, reason := a.Attributes.Meets(f.Requirements); !ok {
		return "guild charter unmet: " + reason
	}
	for _, gID := range a.Guilds {
		if gID == f.ID {
			continue
		}
		if w.AreFactionsAtWar(gID, f.ID) {
			return "cannot join a faction at war with " + gID
		}
		if w.Diplomacy != nil {
			key, _, _ := diplomacyKey(gID, f.ID)
			if rel := w.Diplomacy.Relations[key]; rel != nil && (rel.Stance == "hostile" || rel.RelationScore <= -30) {
				return "cannot join hostile faction " + f.Name
			}
		}
	}
	return ""
}

// PerceivedFactionJoinBlockReason mirrors FactionJoinEligibilityReason for Perception,
// gating war/hostility blocks on diplomacy updates actually known to actor a.
// World.Apply("join_faction") still enforces authoritative FactionJoinEligibilityReason.
func (w *World) PerceivedFactionJoinBlockReason(a Actor, f Faction) string {
	if f.HasMember(a.ID) || ActorHasTag(a.Guilds, f.ID) {
		return "already a member of " + f.Name
	}
	if ok, reason := a.Attributes.Meets(f.Requirements); !ok {
		return "guild charter unmet: " + reason
	}
	if w.Diplomacy != nil {
		for _, gID := range a.Guilds {
			if gID == f.ID {
				continue
			}
			key, _, _ := diplomacyKey(gID, f.ID)
			rel := w.Diplomacy.Relations[key]
			if rel == nil || !actorKnowsDiplomacyUpdate(a, *rel) {
				continue
			}
			if rel.Stance == "at_war" {
				return "cannot join a faction at war with " + gID
			}
			if rel.Stance == "hostile" || rel.RelationScore <= -30 {
				return "cannot join hostile faction " + f.Name
			}
		}
	}
	return ""
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
	for i := range w.Actors {
		if w.Actors[i].ID == id {
			return w.viewIndexed(i, w.buildStepIndex())
		}
	}
	found := w.actor(id)
	if found == nil {
		return Perception{}
	}
	return w.buildPerception(*found, -1, w.buildStepIndex())
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

func (w *World) projectPerceptionContext(p *Perception, agreements []Agreement) {
	if p == nil {
		return
	}
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
		if field.UserKind != "actor" || field.UserID != p.Self.ID {
			continue
		}
		if field.District == p.Self.Location && !p.Self.InTransit() {
			p.Fields = append(p.Fields, FieldView{FieldAsset: field, AvailableAreaM2: w.availableFieldArea(field)})
		} else {
			p.FieldDeeds = append(p.FieldDeeds, field)
		}
	}
	p.Self.Plan = ClonePersonalPlan(p.Self.Plan)
	SyncActorPlan(&p.Self, w.Tick, w.Catalog)
	p.Continuity = w.continuityForAgreements(p.Self.ID, agreements)
	a := p.Self

	p.LoanRequests = w.MarketCredit.PendingLoanRequests(a.ID)
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
		if kA := w.Relations.Kinship[a.ID]; kA != nil && kA.SpouseID != "" {
			p.Married = true
		}
		for _, g := range w.Relations.Grudges {
			if g.Resolved {
				continue
			}
			if g.OffenderID != "" && w.Relations.matchesAggrieved(&a, g) && actorKnowsAboutGrudge(a, g) {
				p.ActiveGrudgeTargets = appendUnique(p.ActiveGrudgeTargets, g.OffenderID)
			}
		}
	}
	p.LedFactions = w.diplomacyLeaderFactions(a)
	p.PartyReady = w.partyReadyToDepart(a)
	p.FlowEdges = w.localFlowEdges(a)
	for i := range p.Nearby {
		if peer := w.actor(p.Nearby[i].ID); peer != nil {
			p.Nearby[i].FactionRelation = w.perceivedActorFactionRelation(a, *peer)
		}
	}
	if w.JusticeRevolt != nil {
		p.DistrictGrievance = w.JusticeRevolt.Grievance(a.Location)
	}
	w.projectLocalFacts(p)
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
	}
	return false
}

func criminalSecretKind(kind string) bool {
	switch kind {
	case "covert_bribe", "illicit_gain", "covert_complicity", "offense":
		return true
	}
	return false
}

func memoryImplicatesCorruption(m Memory, selfID, suspectID string) bool {
	if suspectID == "" || suspectID == selfID {
		return false
	}
	if o := m.Observation; o != nil && o.SubjectID == suspectID {
		return o.Kind == "unlawful_attack" || o.Kind == "corruption" || o.Kind == "wrongful_conviction" ||
			secretReportTopic(o.Kind) && criminalSecretKind(o.Item)
	}
	if (m.Kind == "speech" || m.Kind == "channel_message") && m.Source != selfID && m.FactEventID > 0 && m.Statement != nil {
		report := m.Statement
		return report.SubjectID == suspectID && (report.Topic == "unlawful_attack" || report.Topic == "corruption" || report.Topic == "wrongful_conviction" ||
			secretReportTopic(report.Topic) && criminalSecretKind(report.Item))
	}
	return false
}

func ActorCorruptionEvidenceWithBasis(self Actor, suspectID string, basisEventID int64) bool {
	_, ok := corruptionEvidence(self, suspectID, basisEventID)
	return ok
}

func corruptionEvidence(self Actor, suspectID string, basisID int64) (Memory, bool) {
	settled := map[int64]bool{}
	for _, m := range self.Memories {
		if m.Kind == "corruption_purged" && m.TargetID == suspectID {
			settled[m.RootEventID] = true
		}
	}
	for i := len(self.Memories) - 1; i >= 0; i-- {
		m := self.Memories[i]
		if !settled[m.RootEventID] && (basisID == 0 || m.EventID == basisID) && memoryImplicatesCorruption(m, self.ID, suspectID) {
			return m, true
		}
	}
	return Memory{}, false
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
		} else if (m.Kind == "speech" || m.Kind == "channel_message") && m.Source != self.ID && m.FactEventID > 0 && m.Statement != nil && m.Statement.Topic == "wrongful_conviction" {
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
			if mj.Kind == "conviction_overturned" && ((lead.CaseID > 0 && mj.Observation != nil && mj.Observation.CaseID == lead.CaseID) ||
				lead.CaseID == 0 && lead.SuspectID != "" && mj.TargetID == lead.SuspectID) {
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
		if lead.CaseID == 0 && lead.SuspectID != "" && seenSuspect[lead.SuspectID] {
			continue
		}
		if lead.CaseID > 0 {
			seenCase[lead.CaseID] = true
		}
		if lead.CaseID == 0 && lead.SuspectID != "" {
			seenSuspect[lead.SuspectID] = true
		}
		out = append(out, lead)
	}
	return out
}

func matchKnownLeadToCourtCase(leads []KnownWrongfulLead, cc CourtCase) (KnownWrongfulLead, bool) {
	for _, lead := range leads {
		if lead.CaseID > 0 {
			if lead.CaseID == cc.ID {
				return lead, true
			}
			continue
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
				adjustActorHealthBounded(&w.Actors[i], 1, 0)
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
	for _, phase := range w.postTickPhases() {
		if phaseEvs := phase.Run(events); len(phaseEvs) > 0 {
			events = append(events, phaseEvs...)
		}
	}
	return events, nil
}

func (w *World) record(a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	return w.recordWithTx(nil, a, x, kind, data, observers...)
}

func (w *World) recordWithTx(tx *WorldTx, a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	e := Event{ID: w.NextID, Tick: w.Tick, ActorID: a.ID, TargetID: x.TargetID, Kind: kind, Action: x, Payload: data}
	w.NextID++
	root := payloadInt64(data, "root_event_id")
	if root == 0 {
		root = citedRoot(a.Memories, x.BasisEventID)
	}
	if root == 0 {
		root = e.ID
	}
	data["basis_event_id"] = x.BasisEventID
	data["root_event_id"] = root
	switch kind {
	case "bribed", "stolen", "raided", "steal_foiled", "wrongful_conviction", "extorted", "party_betrayed", "blackmailed":
		if _, exists := data["witnesses"]; !exists {
			viewers := w.awakeSamePlaceObservers(*a, a.ID)
			witnesses := make([]string, 0, len(viewers))
			for _, actor := range viewers {
				witnesses = append(witnesses, actor.ID)
			}
			data["witnesses"] = witnesses
			data["district"] = a.Location
		}
	}
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
		CauseEventID: payloadInt64(data, "cause_event_id"),
		RootEventID:  root,
	}
	if kind == "speech" && x.Statement != nil {
		v := *x.Statement
		m.Statement = &v
		data["statement"] = v
		m.FactEventID, m.WitnessID = citedReport(a, x)
		data["report_source_id"] = m.WitnessID
		data["district"] = a.Location
		data["fact_event_id"], data["witness_id"] = m.FactEventID, m.WitnessID
	}
	if obs, ok := data["observation"].(Observation); ok {
		v := obs
		m.Observation = &v
	}
	m.search = newMemorySearchCache(m)
	tx.RecordMemory(a, m)
	for _, b := range observers {
		if b != nil && b.ID != a.ID {
			tx.RecordMemory(b, m)
		}
	}
	w.ObserveContinuity(e)
	return e
}

// grantSkillXP advances an actor's independent craft/technique skill XP, unlocking level 1 if needed
// and emitting a skill_leveled event whenever a threshold is crossed.
func (w *World) grantSkillXP(tx *WorldTx, a *Actor, x Action, skillID string, xp int) []Event {
	if a == nil || skillID == "" || xp <= 0 {
		return nil
	}
	spec, ok := w.Catalog.Skills[skillID]
	if !ok {
		return nil
	}
	if mult := w.Catalog.GoldenFingerSkillXPMultiplier(*a); mult > 100 {
		xp = maxInt(1, (xp*mult+50)/100)
	}
	if a.Skills == nil {
		a.Skills = map[string]int{}
	}
	if a.SkillXP == nil {
		a.SkillXP = map[string]int{}
	}
	oldLevel := a.Skills[skillID]
	if oldLevel == 0 {
		a.Skills[skillID] = 1
		a.SkillXP[skillID] += xp
		return []Event{w.recordWithTx(tx, a, x, "skill_leveled", map[string]any{
			"text":       fmt.Sprintf("%s learned skill %s level 1 (%s)", a.Name, spec.Name, spec.RankTitle(1)),
			"skill_id":   skillID,
			"skill_name": spec.Name,
			"level":      1,
			"rank":       spec.RankTitle(1),
			"xp":         a.SkillXP[skillID],
		})}
	}
	a.SkillXP[skillID] += xp
	maxLv := spec.EffectiveMaxLevel()
	level := w.Catalog.SkillLevel(a.SkillXP[skillID])
	if level < oldLevel {
		level = oldLevel
	}
	if level > maxLv {
		level = maxLv
	}
	if level > oldLevel {
		oldRank := spec.RankTitle(oldLevel)
		newRank := spec.RankTitle(level)
		a.Skills[skillID] = level
		return []Event{w.recordWithTx(tx, a, x, "skill_leveled", map[string]any{
			"text":          fmt.Sprintf("%s reached skill %s level %d (%s)", a.Name, spec.Name, level, newRank),
			"skill_id":      skillID,
			"skill_name":    spec.Name,
			"level":         level,
			"rank":          newRank,
			"previous_rank": oldRank,
			"xp":            a.SkillXP[skillID],
		})}
	}
	return nil
}

// grantProfessionXP advances profession experience (and optional guild influence) with level-up and rank-advancement events.
func (w *World) grantProfessionXP(tx *WorldTx, a *Actor, x Action, profID string, xp, influence int) []Event {
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
		events = append(events, w.recordWithTx(tx, a, x, "profession_leveled", map[string]any{
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
					events = append(events, w.recordWithTx(tx, a, x, "profession_leveled", map[string]any{
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
			events = append(events, w.recordWithTx(tx, a, x, "faction_leveled", map[string]any{
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

func (w *World) markDeadIfZero(target *Actor, killer *Actor, x Action, cause string, direct ...Event) []Event {
	return w.markDeadIfZeroWithTx(nil, target, killer, x, cause, direct...)
}

func (w *World) markDeadIfZeroWithTx(tx *WorldTx, target *Actor, killer *Actor, x Action, cause string, direct ...Event) []Event {
	if target == nil || target.Dead || target.Health > 0 {
		return nil
	}
	deathData := map[string]any{}
	if len(direct) > 0 {
		deathData["cause_event_id"] = direct[0].ID
		root := payloadInt64(direct[0].Payload, "root_event_id")
		if root == 0 {
			root = direct[0].ID
		}
		deathData["root_event_id"] = root
	}
	wasSick := target.DiseaseID != ""
	wasJailed := target.JailedUntil > w.Tick
	targetSnapshot := *target
	if c := w.storedActorCohort(target.ID); c != nil && c.Population == 1 {
		w.transferLastCohortAssets(tx, c, target)
	}
	tx.setHealth(target, 0, 0, cause)
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
	target.DeathCause = canon
	target.DeathTick = w.Tick
	w.recordDeathCause(canon, 1)
	killerID := ""
	killerName := "natural or environmental causes"
	if killer != nil {
		killerID = killer.ID
		killerName = killer.Name
	}
	// Collect epistemic eyewitnesses (same place) and remote sensory observers before marking Dead.
	var observers []*Actor
	var witnessIDs []string
	if killer != nil && killer.ID != target.ID {
		observers = append(observers, killer)
	}
	for i := range w.Actors {
		o := &w.Actors[i]
		if o.ID == target.ID || o.Dead || w.Tick < o.JailedUntil {
			continue
		}
		if o.Task != nil && (o.Task.Action.Kind == "sleep" || o.Task.Dormant) {
			continue
		}
		if SamePlace(targetSnapshot, *o) {
			if killer == nil || o.ID != killer.ID {
				observers = append(observers, o)
			}
			witnessIDs = append(witnessIDs, o.ID)
		}
	}
	obs := Observation{
		Kind:      "actor_died",
		Modality:  "eyewitness",
		SubjectID: killerID,
		ObjectID:  target.ID,
		Item:      canon,
		Location:  target.Location,
		Quantity:  1,
	}
	x.TargetID = killerID
	for key, value := range map[string]any{
		"text":        fmt.Sprintf("%s died from %s (%s)", target.Name, cause, killerName),
		"killer_id":   killerID,
		"cause":       canon,
		"detail":      cause,
		"district":    target.Location,
		"was_sick":    wasSick,
		"was_jailed":  wasJailed,
		"witnesses":   witnessIDs,
		"observation": obs,
	} {
		deathData[key] = value
	}
	ev := w.recordWithTx(tx, target, x, "actor_died", deathData, observers...)
	target.DeathEventID = ev.ID
	// Also notify qualifying remote sensory observers (e.g. divine_sense / orbital_sensor) respecting shielding.
	if len(w.Catalog.SensoryModes) > 0 {
		for i := range w.Actors {
			o := &w.Actors[i]
			if o.ID == target.ID || (killer != nil && o.ID == killer.ID) || o.Dead || w.Tick < o.JailedUntil || ActorHasTag(witnessIDs, o.ID) {
				continue
			}
			if o.Task != nil && (o.Task.Action.Kind == "sleep" || o.Task.Dormant) {
				continue
			}
			if mod, ok := w.CanSenseVenueRemotely(*o, target.Location, target.VenueID); ok {
				sObs := obs
				sObs.Modality = mod
				mem := Memory{
					Kind:         "actor_died",
					Tick:         w.Tick,
					EventID:      ev.ID,
					RootEventID:  payloadInt64(ev.Payload, "root_event_id"),
					CauseEventID: payloadInt64(ev.Payload, "cause_event_id"),
					Source:       target.ID,
					SourceRole:   target.Role,
					TargetID:     killerID,
					Text:         fmt.Sprintf("Perceived via %s: %s died from %s (%s)", mod, target.Name, cause, killerName),
					Observation:  &sObs,
				}
				tx.RecordMemory(o, mem)
			}
		}
	}
	// Settle inheritance before deciding where any heirless estate goes. Observe
	// marks this death event, so the end-of-step observer cannot settle it twice.
	return append([]Event{ev}, w.ensureRelations().observeCausalRelationEventsWithTx(tx, w, []Event{ev})...)
}

func (w *World) applySleepTick(tx *WorldTx, a *Actor, carryTenths int) (newCarry int, factors string) {
	var h *House
	if a.InsideHouse != "" {
		h = w.house(a.InsideHouse)
	}
	tenths, notes := SleepRecoveryTenths(*a, h, w.Tick)
	total := carryTenths + tenths
	gain := total / 10
	rem := total % 10
	tx.setEnergy(a, min(MaxEnergy, a.Energy+gain), "sleep")
	if a.Energy >= MaxEnergy {
		rem = 0
		a.Resting = false
	}
	if a.Health < MaxHealth && !a.Dead && a.Hunger <= 2 {
		if h != nil {
			if w.Tick%4 == 0 {
				tx.adjustHealth(a, 1, 0, "sleep")
			}
		} else if w.Tick%8 == 0 {
			tx.adjustHealth(a, 1, 0, "sleep")
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
		carry, factors := w.applySleepTick(nil, a, t.SleepTenths)
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
	tx := w.BeginTx(a.ID, ActionIntent{Action: t.Action})
	return tx.commitEvents(w.finishTaskWithTx(tx, a, t)).Events
}

func (w *World) finishTaskWithTx(tx *WorldTx, a *Actor, t Task) []Event {
	x := t.Action
	switch x.Kind {
	case "move":
		fromDistrict := a.Location
		medium := "road"
		if road, ok := w.Map.RoadBetween(fromDistrict, x.Location); ok && road.Medium != "" {
			medium = road.Medium
		}
		if closed, reason := w.isRouteSevered(fromDistrict, x.Location, medium, ""); closed {
			return []Event{tx.record(a, x, "travel_interrupted", map[string]any{"text": reason})}
		}
		escort := w.partyEscortRiskReductionPct(*a, x.Location)
		venue := a.VenueID
		if venue != "" {
			if v, ok := w.Map.VenueByID(venue); !ok || (v.DistrictID != "" && v.DistrictID != x.Location) {
				venue = w.Map.DefaultVenueForDistrict(x.Location)
			}
		}
		tx.MoveActor(a, x.Location, venue, a.InsideHouse)
		events := []Event{tx.record(a, x, "moved", map[string]any{
			"text": a.Name + " walked along the road to " + x.Location,
		})}
		return append(events, w.checkActorTransitHazard(tx, a, x, fromDistrict, a.Location, medium, "", escort)...)
	case "move_venue":
		return w.finishMoveVenue(tx, a, t)
	case "craft":
		r, ok := w.Catalog.RecipeByID(x.RecipeID)
		if !ok {
			return nil
		}
		if r.Growth != nil {
			parents := map[string]int64{}
			for item, qty := range r.Growth.Parents {
				if a.Items[item] < qty {
					w.refundTaskPrepayments(tx, a, t)
					return []Event{tx.record(a, x, "action_failed", map[string]any{"text": "breeding stock no longer available"})}
				}
			}
			for _, item := range sortedProductionKeys(r.Growth.Parents) {
				qty := r.Growth.Parents[item]
				tx.moveItem(a, item, -qty, "growth", x.Kind)
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
			events := []Event{event}
			return append(events, w.checkActorCraftAccident(tx, a, x, r)...)
		}
		before := w.yieldCredit("actor", a.ID, r.ID)
		delivered, next := settleYieldCredit(before, t.FundedHundredths)
		w.setYieldCredit("actor", a.ID, r.ID, next)
		t.Yield = int(delivered)
		tx.ProduceOrConsumeItem(a, r.OutputItem, t.Yield, x.Kind)
		tier := w.Catalog.Items[r.OutputItem].Tier
		if tier < 1 {
			tier = 1
		}
		obs := Observation{Kind: "item_produced", Item: r.OutputItem, Location: a.Location, Quantity: t.Yield}
		events := []Event{tx.record(a, x, "crafted", map[string]any{
			"text":                fmt.Sprintf("%s completed %s (+%d %s)", a.Name, r.ID, t.Yield, r.OutputItem),
			"recipe_id":           r.ID,
			"item":                r.OutputItem,
			"tier":                tier,
			"quantity":            t.Yield,
			"yield_credit_before": before,
			"yield_credit_after":  next,
			"observation":         obs,
		})}
		if r.SkillID != "" {
			events = append(events, w.grantSkillXP(tx, a, x, r.SkillID, w.Catalog.CraftXP(r))...)
		}
		events = append(events, w.grantProfessionXP(tx, a, x, r.Profession, w.Catalog.CraftXP(r), tier)...)
		return append(events, w.checkActorCraftAccident(tx, a, x, r)...)
	case "maintain_infra":
		if w.DistrictStates == nil {
			return nil
		}
		ds, ok := w.DistrictStates[a.Location]
		spec, exists := w.Catalog.Infrastructures[x.Item]
		if !ok || !exists || ds.Infrastructure == nil {
			return nil
		}
		repairGain := 20 + w.Catalog.SkillRepairBonus(*a)
		cond := ds.Infrastructure[x.Item] + repairGain
		if cond > spec.MaxCondition {
			cond = spec.MaxCondition
		}
		ds.Infrastructure[x.Item] = cond
		w.DistrictStates[a.Location] = ds
		events := []Event{tx.record(a, x, "infra_maintained", map[string]any{
			"text":           fmt.Sprintf("%s reinforced %s in %s to condition %d/%d", a.Name, spec.Name, a.Location, cond, spec.MaxCondition),
			"district":       a.Location,
			"infrastructure": x.Item,
			"condition":      cond,
		})}
		for _, sid := range sortedProductionKeys(a.Skills) {
			if a.Skills[sid] > 0 {
				if sk, ok := w.Catalog.Skills[sid]; ok && sk.RepairBonusPerLevel > 0 {
					events = append(events, w.grantSkillXP(tx, a, x, sid, 6)...)
				}
			}
		}
		if a.Role != "" && a.Professions[a.Role] > 0 {
			return append(events, w.grantProfessionXP(tx, a, x, a.Role, 6, 2)...)
		}
		for _, profID := range sortedProductionKeys(a.Professions) {
			if a.Professions[profID] > 0 {
				return append(events, w.grantProfessionXP(tx, a, x, profID, 6, 2)...)
			}
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
		curAct := ds.ThreatActivity[enc.ID]
		if curAct <= 0 {
			return []Event{tx.record(a, x, "action_failed", map[string]any{"text": "the tracked threat has already been suppressed"})}
		}
		suppressed := minInt(curAct, 8+(CombatPowerWithCatalog(w.Catalog, *a)+w.partyCombatSupportBonus(*a))/4)
		dropQty := w.drawEncounterLoot(a.Location, *enc, 1, "act_hunt:"+a.ID)
		ds = w.DistrictStates[a.Location]
		ds.ThreatActivity[enc.ID] = curAct - suppressed
		base := enc.BountyCoins
		bonus := minInt(ds.BountyPool, base)
		bounty := base + bonus
		funded := w.hasActiveCohorts()
		if funded {
			bounty = int(w.debitTreasury(int64(bounty), "bounty"))
		}
		ds.BountyPool -= maxInt(0, bounty-base)
		w.DistrictStates[a.Location] = ds
		if funded {
			tx.moveCurrency(a, "coins", int64(bounty), "treasury", x.Kind)
		} else {
			tx.MintOrBurnCurrency(a, "coins", int64(bounty), x.Kind)
		}
		if dropQty > 0 {
			tx.ProduceOrConsumeItem(a, enc.RareDropItem, int(dropQty), x.Kind)
		}
		if a.Health > 20 {
			tx.adjustHealth(a, -4, 0, x.Kind)
		}
		a.RestUntil = w.Tick + 24
		data := map[string]any{
			"text":         fmt.Sprintf("%s hunted %s in %s (-%d threat, +%d bounty coins, +%d %s)", a.Name, enc.Name, a.Location, suppressed, bounty, dropQty, enc.RareDropItem),
			"district":     a.Location,
			"threat_id":    enc.ID,
			"threat_name":  enc.Name,
			"suppressed":   suppressed,
			"bounty_coins": bounty,
			"drop_item":    enc.RareDropItem,
			"drop_qty":     dropQty,
		}
		for i := len(w.ActiveWorldEvents) - 1; i >= 0; i-- {
			incident := w.ActiveWorldEvents[i]
			if incident.DistrictID == a.Location && incident.InvasionThreatID == enc.ID && w.Tick < incident.ExpiresTick {
				data["cause_event_id"], data["root_event_id"] = incident.RootEventID, incident.RootEventID
				break
			}
		}
		events := []Event{tx.record(a, x, "threat_hunted", data)}
		events = append(events, w.settlePartyRewardSplit(tx, a, bounty, events[0])...)
		return append(events, w.grantCombatXP(tx, a, x, 8, 3)...)
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
					tx.MutateTrust(a, w.Actors[bi].ID, rit.TrustBoost)
					tx.MutateTrust(&w.Actors[bi], a.ID, rit.TrustBoost)
				}
			}
		}
		events := []Event{tx.record(a, x, "ritual_performed", map[string]any{
			"text":         fmt.Sprintf("%s performed %s at %s (+%d morale)", a.Name, rit.Name, a.Location, rit.MoraleBoost),
			"ritual_id":    rit.ID,
			"ritual_name":  rit.Name,
			"district":     a.Location,
			"morale_boost": rit.MoraleBoost,
		})}
		for _, prof := range w.Catalog.Professions {
			if a.Professions[prof.ID] > 0 && (rit.RequiredTag == "" || ActorHasTag(prof.Grants, rit.RequiredTag)) {
				return append(events, w.grantProfessionXP(tx, a, x, prof.ID, 8, 3)...)
			}
		}
		return events
	case "repair_house":
		h := w.house(x.HouseID)
		if h == nil {
			return nil
		}
		builderProf := w.Catalog.EffectiveEnvironment().BuilderProfession
		restore := 25
		builderLevels := a.Professions[builderProf]
		for _, prof := range w.Catalog.Professions {
			if prof.ID != builderProf && (prof.ParentProfession == builderProf ||
				w.Catalog.ProfessionGrantsCapability(prof, "repair_house") ||
				w.Catalog.ProfessionGrantsCapability(prof, "upgrade_house")) {
				builderLevels += a.Professions[prof.ID]
			}
		}
		if builderLevels > 0 {
			restore = 30 + 5*builderLevels
		}
		h.Condition += restore
		if h.Condition > MaxCondition {
			h.Condition = MaxCondition
		}
		events := []Event{tx.record(a, x, "repaired_house", map[string]any{
			"text":      fmt.Sprintf("%s repaired %s to condition %d", a.Name, h.Name, h.Condition),
			"house_id":  h.ID,
			"condition": h.Condition,
		})}
		if builderProf != "" {
			return append(events, w.grantProfessionXP(tx, a, x, builderProf, 6, 2)...)
		}
		return events
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
		events := []Event{tx.record(a, x, "house_upgraded", map[string]any{
			"text":      fmt.Sprintf("%s upgraded %s to level %d (comfort %d)", a.Name, h.Name, h.Level, h.Comfort),
			"house_id":  h.ID,
			"level":     h.Level,
			"comfort":   h.Comfort,
			"condition": h.Condition,
		})}
		if builderProf := w.Catalog.EffectiveEnvironment().BuilderProfession; builderProf != "" {
			return append(events, w.grantProfessionXP(tx, a, x, builderProf, 12, 3)...)
		}
		return events
	case "attempt_transition":
		return w.executeTransition(tx, a, x, resolveTransitionID(x))
	case "research_tech":
		return w.finishResearchTech(tx, a, t)
	case "adopt_facility":
		return w.finishAdoptFacility(tx, a, t)
	default:
		if spec, ok := w.Catalog.ActionSpecs[x.Kind]; ok {
			return w.finishDeclarativeAction(tx, a, t, spec)
		}
		return nil
	}
}

func (w *World) Apply(id string, x Action) []Event {
	result := (DefaultActionExecutor{}).execute(w, id, ActionIntent{Action: x})
	return result.Events
}

func (w *World) apply(tx *WorldTx, a *Actor, x Action) []Event {
	id := a.ID
	if a.Items == nil {
		a.Items = map[string]int{}
	}
	if a.Trust == nil {
		a.Trust = map[string]int{}
	}
	emit := func(kind, text string, observers ...*Actor) []Event {
		return []Event{tx.record(a, x, kind, map[string]any{"text": text}, observers...)}
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
		return fail("actor is in custody")
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
	if handler, ok := builtinActionHandlers[x.Kind]; ok {
		return handler(tx, a, b, x)
	}
	if spec, ok := w.Catalog.ActionSpecs[x.Kind]; ok {
		return w.applyDeclarativeAction(tx, a, x, spec)
	}
	return fail("unknown action: " + x.Kind)
}

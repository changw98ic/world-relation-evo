// Package pack loads, validates, and instantiates external genre-decoupled WorldPack JSON definitions.
package pack

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

const worldPackSchemaVersion = 2

// WorldPack is an external JSON world definition supporting arbitrary genres (historical, xianxia, sci-fi, etc.).
type WorldPack struct {
	SchemaVersion    int                     `json:"schema_version,omitempty"`
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	Genre            string                  `json:"genre"`
	Description      string                  `json:"description,omitempty"`
	SeedTick         int64                   `json:"seed_tick,omitempty"`
	PublicTreasury   int64                   `json:"public_treasury,omitempty"`
	Catalog          sim.Catalog             `json:"catalog"`
	Map              sim.WorldMap            `json:"map"`
	Polities         *sim.PolityNetwork      `json:"polities,omitempty"`
	Houses           []sim.House             `json:"houses,omitempty"`
	Factions         []sim.Faction           `json:"factions,omitempty"`
	Actors           []sim.Actor             `json:"actors"`
	Cohorts          []sim.PopulationCohort  `json:"cohorts,omitempty"`
	Ecology          *sim.EcologySpec        `json:"ecology,omitempty"`
	Production       *sim.ProductionState    `json:"production,omitempty"`
	Governance       *sim.GovernanceSpec     `json:"governance,omitempty"`
	DividendAccounts []sim.DividendAccount   `json:"dividend_accounts,omitempty"`
	MarketCredit     *sim.MarketCreditSpec   `json:"market_credit,omitempty"`
	JusticeRevolt    *sim.JusticeRevoltSpec  `json:"justice_revolt,omitempty"`
	SpotlightRumor   *sim.SpotlightRumorSpec `json:"spotlight_rumor,omitempty"`
	FlowGrid         *sim.FlowGridState      `json:"flow_grid,omitempty"`
	WorldLaws        map[string]int          `json:"world_laws,omitempty"`
	Knowledge        *sim.KnowledgeBook      `json:"knowledge,omitempty"`
	Environment      *sim.EnvironmentProfile `json:"environment,omitempty"`
	Relations        *sim.RelationEvoState   `json:"relations,omitempty"`
	Diplomacy        *sim.DiplomacyState     `json:"diplomacy,omitempty"`
	Secrets          []sim.ActorSecret       `json:"secrets,omitempty"`
	Parties          []sim.Party             `json:"parties,omitempty"`
}

// LoadFile reads and validates a WorldPack JSON file from disk.
func LoadFile(path string) (*WorldPack, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open world pack %q: %w", path, err)
	}
	defer f.Close()
	p, err := Decode(f)
	if err != nil {
		return nil, fmt.Errorf("load world pack %q: %w", path, err)
	}
	return p, nil
}

// Decode parses JSON from r into a WorldPack and validates its referential integrity.
func Decode(r io.Reader) (*WorldPack, error) {
	var p WorldPack
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode world pack JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("world pack trailing data: %w", err)
		}
		return nil, fmt.Errorf("world pack must contain exactly one JSON definition")
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	p.SchemaVersion = worldPackSchemaVersion
	return &p, nil
}

// Validate checks uniqueness and referential integrity across geography, venues, polities, houses, factions, actors, and cohorts.
func (p *WorldPack) Validate() error {
	if p == nil {
		return fmt.Errorf("world pack is nil")
	}
	if p.SchemaVersion < 0 || p.SchemaVersion > worldPackSchemaVersion {
		return fmt.Errorf("unsupported world pack schema_version %d", p.SchemaVersion)
	}
	if p.ID == "" {
		return fmt.Errorf("world pack id is required")
	}
	if len(p.Map.Districts) == 0 {
		return fmt.Errorf("world pack %q must define at least one district", p.ID)
	}

	if err := normalizeCatalog(p.Catalog).ValidatePhysicalRules(p.FlowGrid); err != nil {
		return err
	}
	geoRegions := make(map[string]bool, len(p.Map.GeoRegions))
	for i, r := range p.Map.GeoRegions {
		if r.ID == "" {
			return fmt.Errorf("geo_region[%d] has empty id", i)
		}
		if geoRegions[r.ID] {
			return fmt.Errorf("duplicate geo_region id %q", r.ID)
		}
		geoRegions[r.ID] = true
	}
	for i, gf := range p.Map.GeoFlows {
		if !geoRegions[gf.FromRegion] {
			return fmt.Errorf("geo_flow[%d] references unknown from_region %q", i, gf.FromRegion)
		}
		if !geoRegions[gf.ToRegion] {
			return fmt.Errorf("geo_flow[%d] references unknown to_region %q", i, gf.ToRegion)
		}
	}

	districts := make(map[string]bool, len(p.Map.Districts))
	for i, d := range p.Map.Districts {
		if d.ID == "" {
			return fmt.Errorf("district[%d] has empty id", i)
		}
		if districts[d.ID] {
			return fmt.Errorf("duplicate district id %q", d.ID)
		}
		districts[d.ID] = true
		if d.GeoRegionID != "" && !geoRegions[d.GeoRegionID] {
			return fmt.Errorf("district %q references unknown geo_region_id %q", d.ID, d.GeoRegionID)
		}
	}
	for i, r := range p.Map.Roads {
		if !districts[r.From] {
			return fmt.Errorf("road[%d] references unknown from district %q", i, r.From)
		}
		if !districts[r.To] {
			return fmt.Errorf("road[%d] references unknown to district %q", i, r.To)
		}
	}

	venues := make(map[string]sim.Venue, len(p.Map.Venues))
	for i, v := range p.Map.Venues {
		if v.ID == "" {
			return fmt.Errorf("venue[%d] has empty id", i)
		}
		if _, dup := venues[v.ID]; dup {
			return fmt.Errorf("duplicate venue id %q", v.ID)
		}
		if v.DistrictID == "" {
			return fmt.Errorf("venue %q has empty district_id", v.ID)
		}
		if !districts[v.DistrictID] {
			return fmt.Errorf("venue %q references unknown district_id %q", v.ID, v.DistrictID)
		}
		if v.GeoRegionID != "" && !geoRegions[v.GeoRegionID] {
			return fmt.Errorf("venue %q references unknown geo_region_id %q", v.ID, v.GeoRegionID)
		}
		venues[v.ID] = v
	}
	for i, vr := range p.Map.VenueRoutes {
		if _, ok := venues[vr.FromVenue]; !ok {
			return fmt.Errorf("venue_route[%d] references unknown from_venue %q", i, vr.FromVenue)
		}
		if _, ok := venues[vr.ToVenue]; !ok {
			return fmt.Errorf("venue_route[%d] references unknown to_venue %q", i, vr.ToVenue)
		}
	}

	polities := make(map[string]bool)
	if p.Polities != nil {
		for i, n := range p.Polities.Nodes {
			if n.ID == "" {
				return fmt.Errorf("polity[%d] has empty id", i)
			}
			if polities[n.ID] {
				return fmt.Errorf("duplicate polity id %q", n.ID)
			}
			polities[n.ID] = true
		}
		for _, n := range p.Polities.Nodes {
			if n.DeJureParentID != "" && !polities[n.DeJureParentID] {
				return fmt.Errorf("polity %q references unknown de_jure_parent_id %q", n.ID, n.DeJureParentID)
			}
			if n.DeFactoControllerID != "" && !polities[n.DeFactoControllerID] {
				return fmt.Errorf("polity %q references unknown de_facto_controller_id %q", n.ID, n.DeFactoControllerID)
			}
			if n.SeatDistrictID != "" && !districts[n.SeatDistrictID] {
				return fmt.Errorf("polity %q references unknown seat_district_id %q", n.ID, n.SeatDistrictID)
			}
			if n.SeatVenueID != "" {
				if _, ok := venues[n.SeatVenueID]; !ok {
					return fmt.Errorf("polity %q references unknown seat_venue_id %q", n.ID, n.SeatVenueID)
				}
			}
		}
	}
	for _, d := range p.Map.Districts {
		if d.PolityID != "" && !polities[d.PolityID] {
			return fmt.Errorf("district %q references unknown polity_id %q", d.ID, d.PolityID)
		}
	}
	for _, v := range p.Map.Venues {
		if v.PolityID != "" && !polities[v.PolityID] {
			return fmt.Errorf("venue %q references unknown polity_id %q", v.ID, v.PolityID)
		}
	}

	factions := make(map[string]bool, len(p.Factions))
	for i, f := range p.Factions {
		if f.ID == "" {
			return fmt.Errorf("faction[%d] has empty id", i)
		}
		if factions[f.ID] {
			return fmt.Errorf("duplicate faction id %q", f.ID)
		}
		factions[f.ID] = true
		if f.District != "" && !districts[f.District] {
			return fmt.Errorf("faction %q references unknown district %q", f.ID, f.District)
		}
	}

	houses := make(map[string]bool, len(p.Houses))
	for i, h := range p.Houses {
		if h.ID == "" {
			return fmt.Errorf("house[%d] has empty id", i)
		}
		if houses[h.ID] {
			return fmt.Errorf("duplicate house id %q", h.ID)
		}
		houses[h.ID] = true
		if !districts[h.District] {
			return fmt.Errorf("house %q references unknown district %q", h.ID, h.District)
		}
		if h.VenueID != "" {
			if _, ok := venues[h.VenueID]; !ok {
				return fmt.Errorf("house %q references unknown venue_id %q", h.ID, h.VenueID)
			}
		}
	}

	allActors := append([]sim.Actor(nil), p.Actors...)
	for _, cohort := range p.Cohorts {
		allActors = append(allActors, cohort.Members...)
	}
	actors := make(map[string]bool, len(allActors))
	for i, a := range allActors {
		if a.ID == "" {
			return fmt.Errorf("actor[%d] has empty id", i)
		}
		if actors[a.ID] {
			return fmt.Errorf("duplicate actor id %q", a.ID)
		}
		actors[a.ID] = true
		if a.Location == "" || !districts[a.Location] {
			return fmt.Errorf("actor %q has invalid location %q", a.ID, a.Location)
		}
		if a.VenueID != "" {
			v, ok := venues[a.VenueID]
			if !ok {
				return fmt.Errorf("actor %q references unknown venue_id %q", a.ID, a.VenueID)
			}
			if v.DistrictID != "" && v.DistrictID != a.Location {
				return fmt.Errorf("actor %q venue_id %q belongs to district %q, not actor location %q", a.ID, a.VenueID, v.DistrictID, a.Location)
			}
		}
		if a.HomeID != "" && !houses[a.HomeID] {
			return fmt.Errorf("actor %q references unknown home_id %q", a.ID, a.HomeID)
		}
		if a.InsideHouse != "" && !houses[a.InsideHouse] {
			return fmt.Errorf("actor %q references unknown inside_house %q", a.ID, a.InsideHouse)
		}
		if a.FormID != "" {
			if _, ok := p.Catalog.Forms[a.FormID]; !ok {
				return fmt.Errorf("actor %q references unknown form_id %q", a.ID, a.FormID)
			}
		}
		if len(p.Catalog.Professions) > 0 {
			for pid := range a.Professions {
				if _, ok := p.Catalog.ProfessionByID(pid); !ok {
					return fmt.Errorf("actor %q references unknown profession %q", a.ID, pid)
				}
			}
		}
		if len(p.Catalog.Skills) > 0 {
			for sid := range a.Skills {
				if _, ok := p.Catalog.Skills[sid]; !ok {
					return fmt.Errorf("actor %q references unknown skill %q", a.ID, sid)
				}
			}
		}
		if len(p.Catalog.EquipSlots) > 0 {
			for slot := range a.Equipped {
				hasSlot := false
				for _, s := range p.Catalog.EquipSlots {
					if s.ID == slot {
						hasSlot = true
						break
					}
				}
				if !hasSlot {
					return fmt.Errorf("actor %q equipped item in undeclared slot %q", a.ID, slot)
				}
			}
		}
	}

	for _, h := range p.Houses {
		if h.OwnerID != "" && !actors[h.OwnerID] {
			return fmt.Errorf("house %q references unknown owner %q", h.ID, h.OwnerID)
		}
		for _, id := range h.Residents {
			if !actors[id] {
				return fmt.Errorf("house %q references unknown resident %q", h.ID, id)
			}
		}
	}
	for _, a := range allActors {
		for _, id := range a.Guilds {
			if !factions[id] {
				return fmt.Errorf("actor %q references unknown guild %q", a.ID, id)
			}
		}
	}
	for _, f := range p.Factions {
		for _, id := range f.Members {
			if !actors[id] {
				return fmt.Errorf("faction %q references unknown member %q", f.ID, id)
			}
		}
	}
	if p.Polities != nil {
		for _, n := range p.Polities.Nodes {
			if n.LeaderActorID != "" && !actors[n.LeaderActorID] {
				return fmt.Errorf("polity %q references unknown leader_actor_id %q", n.ID, n.LeaderActorID)
			}
		}
	}

	cohorts := make(map[string]bool, len(p.Cohorts))
	for i, c := range p.Cohorts {
		if c.ID == "" {
			return fmt.Errorf("cohort[%d] has empty id", i)
		}
		if cohorts[c.ID] {
			return fmt.Errorf("duplicate cohort id %q", c.ID)
		}
		cohorts[c.ID] = true
		livingMembers := 0
		for _, member := range c.Members {
			if !member.Dead {
				livingMembers++
			}
		}
		if c.Population < livingMembers {
			return fmt.Errorf("cohort %q population %d is below its %d living members", c.ID, c.Population, livingMembers)
		}
		if !districts[c.District] {
			return fmt.Errorf("cohort %q references unknown district %q", c.ID, c.District)
		}
		if c.FactionID != "" && !factions[c.FactionID] {
			return fmt.Errorf("cohort %q references unknown faction_id %q", c.ID, c.FactionID)
		}
		if c.Profession != "" && len(p.Catalog.Professions) > 0 {
			if _, ok := p.Catalog.ProfessionByID(c.Profession); !ok {
				return fmt.Errorf("cohort %q references unknown profession %q", c.ID, c.Profession)
			}
		}
		if c.FormID != "" && len(p.Catalog.Forms) > 0 {
			if _, ok := p.Catalog.Forms[c.FormID]; !ok {
				return fmt.Errorf("cohort %q references unknown form_id %q", c.ID, c.FormID)
			}
		}
		if ageSum := c.Children + c.Adults + c.Elders; ageSum > 0 && ageSum != c.Population {
			return fmt.Errorf("cohort %q age brackets sum %d != population %d", c.ID, ageSum, c.Population)
		}
	}

	if err := p.validateProductionAssets(actors, cohorts, districts, venues); err != nil {
		return err
	}
	mapFeatures := make(map[string]bool)
	for _, d := range p.Map.Districts {
		for _, f := range d.Features {
			mapFeatures[f] = true
		}
	}
	for _, v := range p.Map.Venues {
		for _, f := range v.Features {
			mapFeatures[f] = true
		}
	}
	for _, r := range p.Catalog.Recipes {
		if r.NodeFeature != "" && !mapFeatures[r.NodeFeature] {
			return fmt.Errorf("recipe %q requires missing map node_feature %q", r.ID, r.NodeFeature)
		}
		if r.SkillID != "" && len(p.Catalog.Skills) > 0 {
			if _, ok := p.Catalog.Skills[r.SkillID]; !ok {
				return fmt.Errorf("recipe %q references unknown skill_id %q", r.ID, r.SkillID)
			}
		}
	}
	initial := &sim.World{Actors: p.Actors, Cohorts: p.Cohorts, PublicTreasury: p.PublicTreasury, Polities: p.Polities, FlowGrid: p.FlowGrid}
	if p.Ecology != nil {
		initial.DistrictStates = make(map[string]sim.DistrictState, len(p.Ecology.DistrictStates))
		for id, override := range p.Ecology.DistrictStates {
			initial.DistrictStates[id] = override.DistrictState
		}
	}
	if err := initial.VerifyHoldingBounds(); err != nil {
		return fmt.Errorf("invalid initial holdings: %w", err)
	}
	if p.Ecology != nil {
		for dID := range p.Ecology.DistrictStates {
			if !districts[dID] {
				return fmt.Errorf("ecology district_states references unknown district %q", dID)
			}
		}
	}

	if p.FlowGrid != nil {
		flowNodes := make(map[string]bool, len(p.FlowGrid.Nodes))
		for i, n := range p.FlowGrid.Nodes {
			if n.ID == "" {
				return fmt.Errorf("flow_grid.nodes[%d] has empty id", i)
			}
			if flowNodes[n.ID] {
				return fmt.Errorf("duplicate flow_grid node id %q", n.ID)
			}
			flowNodes[n.ID] = true
			if n.VenueID != "" {
				if _, ok := venues[n.VenueID]; !ok {
					return fmt.Errorf("flow_grid node %q references unknown venue_id %q", n.ID, n.VenueID)
				}
			}
			if n.DistrictID != "" && !districts[n.DistrictID] {
				return fmt.Errorf("flow_grid node %q references unknown district_id %q", n.ID, n.DistrictID)
			}
		}
		for i, e := range p.FlowGrid.Edges {
			if !flowNodes[e.FromNodeID] {
				return fmt.Errorf("flow_grid.edges[%d] references unknown from_node_id %q", i, e.FromNodeID)
			}
			if !flowNodes[e.ToNodeID] {
				return fmt.Errorf("flow_grid.edges[%d] references unknown to_node_id %q", i, e.ToNodeID)
			}
		}
	}

	if p.Knowledge != nil {
		for k, rec := range p.Knowledge.Records {
			if rec == nil {
				continue
			}
			techID := rec.TechID
			if techID == "" {
				techID = k
			}
			if _, ok := p.Catalog.Techs[techID]; !ok {
				return fmt.Errorf("knowledge record %q references unknown tech %q", k, techID)
			}
			for _, aid := range rec.KnownActors {
				if !actors[aid] {
					return fmt.Errorf("knowledge record %q references unknown actor %q", k, aid)
				}
			}
			for _, fid := range rec.KnownFactions {
				if !factions[fid] {
					return fmt.Errorf("knowledge record %q references unknown faction %q", k, fid)
				}
			}
			for _, vid := range rec.AdoptedVenues {
				if _, ok := venues[vid]; !ok {
					return fmt.Errorf("knowledge record %q references unknown venue %q", k, vid)
				}
			}
			for _, hid := range rec.AdoptedHouses {
				if !houses[hid] {
					return fmt.Errorf("knowledge record %q references unknown house %q", k, hid)
				}
			}
		}
	}

	if p.Relations != nil {
		for aid, k := range p.Relations.Kinship {
			if !actors[aid] {
				return fmt.Errorf("relations.kinship references unknown actor %q", aid)
			}
			if k == nil {
				continue
			}
			if k.SpouseID != "" && !actors[k.SpouseID] {
				return fmt.Errorf("relations.kinship[%q] references unknown spouse %q", aid, k.SpouseID)
			}
			if k.PatronID != "" && !actors[k.PatronID] {
				return fmt.Errorf("relations.kinship[%q] references unknown patron %q", aid, k.PatronID)
			}
			for _, pid := range k.ParentIDs {
				if !actors[pid] {
					return fmt.Errorf("relations.kinship[%q] references unknown parent %q", aid, pid)
				}
			}
			for _, cid := range k.ChildIDs {
				if !actors[cid] {
					return fmt.Errorf("relations.kinship[%q] references unknown child %q", aid, cid)
				}
			}
			for _, sid := range k.SwornIDs {
				if !actors[sid] {
					return fmt.Errorf("relations.kinship[%q] references unknown sworn %q", aid, sid)
				}
			}
			for _, pid := range k.ProtegeIDs {
				if !actors[pid] {
					return fmt.Errorf("relations.kinship[%q] references unknown protege %q", aid, pid)
				}
			}
		}
		for i, m := range p.Relations.Mentorships {
			if !actors[m.MentorID] {
				return fmt.Errorf("relations.mentorships[%d] references unknown mentor %q", i, m.MentorID)
			}
			if !actors[m.ApprenticeID] {
				return fmt.Errorf("relations.mentorships[%d] references unknown apprentice %q", i, m.ApprenticeID)
			}
		}
		for i, g := range p.Relations.Grudges {
			if g.AggrievedID != "" && !actors[g.AggrievedID] {
				return fmt.Errorf("relations.grudges[%d] references unknown aggrieved_id %q", i, g.AggrievedID)
			}
			if g.OffenderID != "" && !actors[g.OffenderID] {
				return fmt.Errorf("relations.grudges[%d] references unknown offender_id %q", i, g.OffenderID)
			}
		}
		for fid, lid := range p.Relations.FactionLeader {
			if !factions[fid] {
				return fmt.Errorf("relations.faction_leader references unknown faction %q", fid)
			}
			if lid != "" && !actors[lid] {
				return fmt.Errorf("relations.faction_leader[%q] references unknown leader %q", fid, lid)
			}
		}
		for did, fid := range p.Relations.ControllingFaction {
			if !districts[did] {
				return fmt.Errorf("relations.controlling_faction references unknown district %q", did)
			}
			if fid != "" && !factions[fid] {
				return fmt.Errorf("relations.controlling_faction[%q] references unknown faction %q", did, fid)
			}
		}
	}
	if p.Diplomacy != nil {
		for k, rel := range p.Diplomacy.Relations {
			if rel == nil {
				continue
			}
			if !factions[rel.FactionA] {
				return fmt.Errorf("diplomacy.relations[%q] references unknown faction_a %q", k, rel.FactionA)
			}
			if !factions[rel.FactionB] {
				return fmt.Errorf("diplomacy.relations[%q] references unknown faction_b %q", k, rel.FactionB)
			}
		}
	}
	for i, s := range p.Secrets {
		if s.OwnerID == "" || !actors[s.OwnerID] {
			return fmt.Errorf("secrets[%d] references unknown owner_id %q", i, s.OwnerID)
		}
		for _, kb := range s.KnownBy {
			if kb != "" && !actors[kb] {
				return fmt.Errorf("secrets[%d] references unknown known_by actor %q", i, kb)
			}
		}
		for _, wb := range s.WitnessedBy {
			if wb != "" && !actors[wb] {
				return fmt.Errorf("secrets[%d] references unknown witnessed_by actor %q", i, wb)
			}
		}
	}
	for i, party := range p.Parties {
		if party.LeaderID == "" || !actors[party.LeaderID] {
			return fmt.Errorf("parties[%d] references unknown leader_id %q", i, party.LeaderID)
		}
		if party.TargetDistrict != "" && !districts[party.TargetDistrict] {
			return fmt.Errorf("parties[%d] references unknown target_district %q", i, party.TargetDistrict)
		}
		for _, mid := range party.MemberIDs {
			if !actors[mid] {
				return fmt.Errorf("parties[%d] references unknown member_id %q", i, mid)
			}
		}
	}

	return nil
}

// Instantiate validates p and constructs a ready-to-step *sim.World with all runtime state
// initialized safely for immediate World.Step execution and lossless JSON round-trip.
func (p *WorldPack) Instantiate(worldIDOverride string) (*sim.World, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	worldID := p.ID
	if worldIDOverride != "" {
		worldID = worldIDOverride
	}

	cat := normalizeCatalog(sim.CloneCatalog(p.Catalog))
	if p.Environment != nil {
		cat.Environment = sim.CloneEnvironmentProfile(p.Environment)
	}
	if cat.Environment == nil && (p.Genre != "" || p.Name != "" || p.Description != "") {
		cat.Environment = &sim.EnvironmentProfile{}
	}
	if cat.Environment != nil {
		if cat.Environment.Genre == "" {
			cat.Environment.Genre = p.Genre
		}
		if cat.Environment.Theme == "" {
			cat.Environment.Theme = p.Name
		}
		if cat.Environment.Summary == "" {
			cat.Environment.Summary = p.Description
		}
	}
	if p.Governance != nil {
		cat.Governance = *p.Governance
	}
	cat.Governance = cat.EffectiveGovernance()

	worldMap := normalizeMap(sim.CloneMap(p.Map))
	polities := normalizePolities(sim.ClonePolityNetwork(p.Polities))

	factions := sim.CloneFactions(p.Factions)
	if factions == nil {
		factions = []sim.Faction{}
	}
	for i := range factions {
		if len(factions[i].Members) == 0 {
			factions[i].Members = nil
		}
		if len(factions[i].Requirements) == 0 {
			factions[i].Requirements = nil
		}
		if factions[i].Professions == nil {
			factions[i].Professions = []string{}
		}
	}

	houses := make([]sim.House, len(p.Houses))
	for i, h := range p.Houses {
		hc := sim.CloneHouse(h)
		if len(hc.Residents) == 0 {
			hc.Residents = nil
		}
		if len(hc.Facilities) == 0 {
			hc.Facilities = nil
		}
		houses[i] = hc
	}

	sleepTraits := []string{sim.SleepDeep, sim.SleepEarlyBird, sim.SleepLight, sim.SleepNightOwl}
	actors := make([]sim.Actor, len(p.Actors))
	for i, a := range p.Actors {
		ac := sim.CloneActor(a)
		if ac.Tags == nil {
			ac.Tags = []string{}
		} else {
			ac.Tags = append([]string(nil), ac.Tags...)
		}
		if ac.Professions == nil {
			ac.Professions = map[string]int{}
		} else {
			cp := make(map[string]int, len(ac.Professions))
			for k, v := range ac.Professions {
				cp[k] = v
			}
			ac.Professions = cp
		}
		if len(ac.ProfessionRanks) == 0 {
			ac.ProfessionRanks = nil
		} else {
			cp := make(map[string]string, len(ac.ProfessionRanks))
			for k, v := range ac.ProfessionRanks {
				cp[k] = v
			}
			ac.ProfessionRanks = cp
		}
		if len(ac.ProfessionXP) == 0 {
			ac.ProfessionXP = nil
		} else {
			cp := make(map[string]int, len(ac.ProfessionXP))
			for k, v := range ac.ProfessionXP {
				cp[k] = v
			}
			ac.ProfessionXP = cp
		}
		if len(ac.Skills) == 0 {
			ac.Skills = nil
		} else {
			cp := make(map[string]int, len(ac.Skills))
			for k, v := range ac.Skills {
				cp[k] = v
			}
			ac.Skills = cp
		}
		if len(ac.SkillXP) == 0 {
			ac.SkillXP = nil
		} else {
			cp := make(map[string]int, len(ac.SkillXP))
			for k, v := range ac.SkillXP {
				cp[k] = v
			}
			ac.SkillXP = cp
		}
		if len(ac.Guilds) == 0 {
			ac.Guilds = nil
		} else {
			ac.Guilds = append([]string(nil), ac.Guilds...)
		}
		if len(ac.GuildLevels) == 0 {
			ac.GuildLevels = nil
		} else {
			cp := make(map[string]int, len(ac.GuildLevels))
			for k, v := range ac.GuildLevels {
				cp[k] = v
			}
			ac.GuildLevels = cp
		}
		if ac.Items == nil {
			ac.Items = map[string]int{}
		} else {
			cp := make(map[string]int, len(ac.Items))
			for k, v := range ac.Items {
				if v > 0 {
					cp[k] = v
				}
			}
			ac.Items = cp
		}
		if ac.Trust == nil {
			ac.Trust = map[string]int{}
		} else {
			cp := make(map[string]int, len(ac.Trust))
			for k, v := range ac.Trust {
				cp[k] = v
			}
			ac.Trust = cp
		}
		if len(ac.Memories) == 0 {
			ac.Memories = nil
		} else {
			ac.Memories = append([]sim.Memory(nil), ac.Memories...)
		}
		if ac.Plan != nil {
			ac.Plan = sim.ClonePersonalPlan(ac.Plan)
		}
		if ac.Task != nil {
			tc := *ac.Task
			ac.Task = &tc
		}
		if ac.SleepTrait == "" {
			ac.SleepTrait = sleepTraits[i%len(sleepTraits)]
		}
		if len(ac.Currencies) == 0 {
			ac.Currencies = nil
		} else {
			cp := make(map[string]int64, len(ac.Currencies))
			for k, v := range ac.Currencies {
				cp[k] = v
			}
			ac.Currencies = cp
		}
		if len(ac.StateSlots) == 0 {
			ac.StateSlots = nil
		} else {
			cp := make(map[string]int, len(ac.StateSlots))
			for k, v := range ac.StateSlots {
				cp[k] = v
			}
			ac.StateSlots = cp
		}
		actors[i] = ac
	}

	var cohorts []sim.PopulationCohort
	if len(p.Cohorts) > 0 {
		cohorts = make([]sim.PopulationCohort, len(p.Cohorts))
		for i, c := range p.Cohorts {
			cc := sim.CloneCohort(c)
			if cc.Population > 0 && cc.Children+cc.Adults+cc.Elders == 0 {
				cc.Children = (cc.Population * 20) / 100
				cc.Elders = (cc.Population * 14) / 100
				cc.Adults = cc.Population - cc.Children - cc.Elders
			}
			cohorts[i] = cc
		}
	}

	nextID := int64(1)
	bumpNextID := func(mems []sim.Memory) {
		for _, m := range mems {
			if m.EventID >= nextID {
				nextID = m.EventID + 1
			}
			if m.RootEventID >= nextID {
				nextID = m.RootEventID + 1
			}
			if m.BasisEventID >= nextID {
				nextID = m.BasisEventID + 1
			}
		}
	}
	for _, a := range actors {
		bumpNextID(a.Memories)
	}
	for _, c := range cohorts {
		for _, m := range c.Members {
			bumpNextID(m.Memories)
		}
	}

	relations := sim.NewRelationEvoState()
	if p.Relations != nil {
		if cloned := sim.CloneRelationEvoState(p.Relations); cloned != nil {
			relations = cloned
			if relations.Kinship == nil {
				relations.Kinship = map[string]*sim.ActorKinship{}
			}
			if relations.Clans == nil {
				relations.Clans = map[string]*sim.ClanRecord{}
			}
			if relations.FactionLeader == nil {
				relations.FactionLeader = map[string]string{}
			}
			if relations.ControllingFaction == nil {
				relations.ControllingFaction = map[string]string{}
			}
			if relations.CohortClans == nil {
				relations.CohortClans = map[string]string{}
			}
			if relations.DistrictSovereignty == nil {
				relations.DistrictSovereignty = map[string]sim.DistrictSovereignty{}
			}
			if relations.MonopolyCharters == nil {
				relations.MonopolyCharters = map[string]sim.MonopolyCharter{}
			}
			if relations.ObservedEventIDs == nil {
				relations.ObservedEventIDs = map[int64]bool{}
			}
			relations.Spec = sim.EffectiveRelationEvoSpec(relations.Spec)
		}
	}

	var diplomacy *sim.DiplomacyState
	if p.Diplomacy != nil {
		diplomacy = sim.CloneDiplomacyState(p.Diplomacy)
	}

	var secrets []sim.ActorSecret
	if len(p.Secrets) > 0 {
		secrets = make([]sim.ActorSecret, len(p.Secrets))
		for i, s := range p.Secrets {
			sc := sim.CloneActorSecret(s)
			if sc.ID == "" {
				sc.ID = fmt.Sprintf("sec_init_%d_%s", i+1, sc.OwnerID)
			}
			if sc.OwnerID != "" && !sim.ActorHasTag(sc.KnownBy, sc.OwnerID) {
				sc.KnownBy = append([]string{sc.OwnerID}, sc.KnownBy...)
			}
			if sc.Severity <= 0 {
				sc.Severity = 50
			}
			if sc.RootEventID >= nextID {
				nextID = sc.RootEventID + 1
			}
			secrets[i] = sc
		}
	}

	var parties []sim.Party
	if len(p.Parties) > 0 {
		parties = make([]sim.Party, len(p.Parties))
		for i, party := range p.Parties {
			pc := sim.CloneParty(party)
			if pc.ID == "" {
				pc.ID = fmt.Sprintf("party_init_%d_%s", i+1, pc.LeaderID)
			}
			if pc.ExpiresTick <= p.SeedTick {
				pc.ExpiresTick = p.SeedTick + 7*sim.TicksPerDay
			}
			parties[i] = pc
		}
	}

	w := &sim.World{
		ID:             worldID,
		Tick:           p.SeedTick,
		NextID:         nextID,
		Map:            worldMap,
		Catalog:        cat,
		Polities:       polities,
		Factions:       factions,
		Houses:         houses,
		Actors:         actors,
		Cohorts:        cohorts,
		PublicTreasury: p.PublicTreasury,
		TreasuryLedger: sim.TreasuryLedger{
			Initialized:     true,
			InitialTreasury: p.PublicTreasury,
		},
		Continuity:     &sim.ContinuityState{NextAgreementID: 1},
		MarketCredit:   sim.NewMarketCreditState(),
		Relations:      relations,
		Diplomacy:      diplomacy,
		Secrets:        secrets,
		Parties:        parties,
		JusticeRevolt:  sim.NewJusticeRevoltState(),
		SpotlightRumor: sim.NewSpotlightRumorState(),
		FlowGrid:       sim.CloneFlowGridState(p.FlowGrid),
		Knowledge:      sim.CloneKnowledgeBook(p.Knowledge),
	}
	w.PlaceResidents()
	if err := w.RegisterDividends(p.DividendAccounts); err != nil {
		return nil, err
	}
	if p.Production != nil {
		w.Production = sim.CloneProductionState(*p.Production)
	} else {
		w.InitProductionAssets()
	}
	if len(p.WorldLaws) > 0 {
		w.WorldLaws = make(map[string]int, len(p.WorldLaws))
		for k, v := range p.WorldLaws {
			w.WorldLaws[k] = v
		}
	}

	if p.MarketCredit != nil {
		w.MarketCredit.Spec = sim.EffectiveMarketCreditSpec(*p.MarketCredit)
	}
	if p.JusticeRevolt != nil {
		spec := sim.EffectiveJusticeRevoltSpec(*p.JusticeRevolt)
		w.JusticeRevolt.Spec = &spec
	}
	if p.SpotlightRumor != nil {
		w.SpotlightRumor.Spec = sim.EffectiveSpotlightRumorSpec(*p.SpotlightRumor)
	}

	// Synchronize faction membership lists and actor Guilds slices consistently.
	factionByID := make(map[string]*sim.Faction, len(w.Factions))
	for i := range w.Factions {
		factionByID[w.Factions[i].ID] = &w.Factions[i]
	}
	actorByID := make(map[string]*sim.Actor, len(w.Actors))
	for i := range w.Actors {
		actorByID[w.Actors[i].ID] = &w.Actors[i]
	}
	for i := range w.Actors {
		a := &w.Actors[i]
		for _, gID := range a.Guilds {
			if f, ok := factionByID[gID]; ok {
				if !f.HasMember(a.ID) {
					f.Members = append(f.Members, a.ID)
				}
			}
		}
	}
	for i := range w.Factions {
		f := &w.Factions[i]
		for _, mID := range f.Members {
			if a, ok := actorByID[mID]; ok {
				has := false
				for _, gID := range a.Guilds {
					if gID == f.ID {
						has = true
						break
					}
				}
				if !has {
					a.Guilds = append(a.Guilds, f.ID)
				}
			}
		}
	}

	// Initialize ecology if configured, if macro cohorts are present, or if declarative WorldEvents require DistrictStates.
	initEco := len(w.Cohorts) > 0 || len(w.Catalog.WorldEvents) > 0
	scalePop := w.TotalPopulation()
	if p.Ecology != nil {
		if p.Ecology.Enabled != nil {
			initEco = *p.Ecology.Enabled
		} else if p.Ecology.ScalePop > 0 || len(p.Ecology.DistrictStates) > 0 {
			initEco = true
		}
		if p.Ecology.ScalePop > 0 {
			scalePop = p.Ecology.ScalePop
		}
	}
	if initEco {
		if scalePop < 30 {
			scalePop = 30
		}
		w.InitEcology(scalePop)
		if p.Ecology != nil && len(p.Ecology.DistrictStates) > 0 {
			distIDs := make([]string, 0, len(p.Ecology.DistrictStates))
			for dID := range p.Ecology.DistrictStates {
				distIDs = append(distIDs, dID)
			}
			sort.Strings(distIDs)
			for _, dID := range distIDs {
				ov := p.Ecology.DistrictStates[dID]
				base := w.DistrictStates[dID]
				base.DistrictID = dID
				if ov.Weather != "" {
					base.Weather = ov.Weather
				}
				if ov.Sanitation != nil {
					base.Sanitation = *ov.Sanitation
				}
				base.Corruption = ov.Corruption
				if ov.MoraleBonus != 0 {
					base.MoraleBonus = ov.MoraleBonus
				}
				if ov.BountyPool > 0 {
					base.BountyPool = ov.BountyPool
				}
				if len(ov.Resources) > 0 {
					if base.Resources == nil {
						base.Resources = map[string]int64{}
					}
					if base.MaxResources == nil {
						base.MaxResources = map[string]int64{}
					}
					for k, v := range ov.Resources {
						base.Resources[k] = v
						if base.MaxResources[k] < v {
							base.MaxResources[k] = v
						}
					}
				}
				if len(ov.MaxResources) > 0 {
					if base.MaxResources == nil {
						base.MaxResources = map[string]int64{}
					}
					for k, v := range ov.MaxResources {
						base.MaxResources[k] = v
					}
				}
				if len(ov.Infrastructure) > 0 {
					if base.Infrastructure == nil {
						base.Infrastructure = map[string]int{}
					}
					for k, v := range ov.Infrastructure {
						base.Infrastructure[k] = v
					}
				}
				if len(ov.ThreatActivity) > 0 {
					if base.ThreatActivity == nil {
						base.ThreatActivity = map[string]int{}
					}
					for k, v := range ov.ThreatActivity {
						base.ThreatActivity[k] = v
					}
				}
				if len(ov.ActiveDisasters) > 0 {
					base.ActiveDisasters = append([]sim.ActiveDisaster(nil), ov.ActiveDisasters...)
				}
				w.DistrictStates[dID] = base
			}
		}
	}

	if w.FlowGrid != nil {
		w.RecomputeFlowGrid()
	}
	if err := w.InitCohortResidents(); err != nil {
		return nil, err
	}
	w.InitLedgerAudit()

	return w, nil
}

func normalizeCatalog(cat sim.Catalog) sim.Catalog {
	def := sim.DefaultCatalog()
	if cat.TagActions == nil {
		cat.TagActions = def.TagActions
	}
	if cat.Items == nil {
		cat.Items = map[string]sim.ItemSpec{}
	}
	if cat.Professions == nil {
		cat.Professions = []sim.Profession{}
	}
	for i := range cat.Professions {
		if len(cat.Professions[i].Requirements) == 0 {
			cat.Professions[i].Requirements = nil
		}
		if len(cat.Professions[i].Grants) == 0 {
			cat.Professions[i].Grants = nil
		}
		if len(cat.Professions[i].Ranks) == 0 {
			cat.Professions[i].Ranks = nil
		}
	}
	if cat.Recipes == nil {
		cat.Recipes = []sim.Recipe{}
	}
	for i := range cat.Recipes {
		if len(cat.Recipes[i].Inputs) == 0 {
			cat.Recipes[i].Inputs = nil
		}
	}
	if cat.TagProfiles == nil {
		cat.TagProfiles = map[string]sim.TagProfile{}
	}
	for k, tp := range cat.TagProfiles {
		if len(tp.Produces) == 0 {
			tp.Produces = nil
		}
		if len(tp.Consumes) == 0 {
			tp.Consumes = nil
		}
		if len(tp.TargetStock) == 0 {
			tp.TargetStock = nil
		}
		cat.TagProfiles[k] = tp
	}
	if len(cat.LevelThresholds) == 0 {
		cat.LevelThresholds = append([]int(nil), def.LevelThresholds...)
	}
	if len(cat.FactionLevelThresholds) == 0 {
		cat.FactionLevelThresholds = append([]int(nil), def.FactionLevelThresholds...)
	}
	if cat.Calendar.Seasons == nil {
		cat.Calendar = sim.DefaultCalendarSpec()
	}
	if len(cat.Weathers) == 0 {
		cat.Weathers = sim.DefaultWeatherSpecs()
	}
	if cat.Lifecycle == (sim.LifecycleSpec{}) {
		lc := sim.DefaultLifecycleSpec()
		if _, hasWaste := cat.Items[lc.WasteItem]; !hasWaste {
			lc.WasteItem = ""
			lc.WastePerFoodDivisor = 0
		}
		hasCompost := false
		for _, r := range cat.Recipes {
			if r.ID == lc.CompostRecipeID {
				hasCompost = true
				break
			}
		}
		if !hasCompost {
			lc.CompostRecipeID = ""
		}
		cat.Lifecycle = lc
	} else if cat.Lifecycle.MaxExpectancy == 0 {
		cat.Lifecycle.Ageless = true
	}
	if len(cat.Resources) == 0 {
		cat.Resources = nil
	}
	if len(cat.Encounters) == 0 {
		cat.Encounters = nil
	}
	if len(cat.Disasters) == 0 {
		cat.Disasters = nil
	}
	if len(cat.Accidents) == 0 {
		cat.Accidents = nil
	}
	if len(cat.Infrastructures) == 0 {
		cat.Infrastructures = nil
	}
	if len(cat.Rituals) == 0 {
		cat.Rituals = nil
	}
	if len(cat.Diseases) == 0 {
		cat.Diseases = nil
	}
	if len(cat.LocomotionModes) == 0 {
		cat.LocomotionModes = nil
	}
	if len(cat.SensoryModes) == 0 {
		cat.SensoryModes = nil
	}
	if len(cat.CommChannels) == 0 {
		cat.CommChannels = nil
	}
	if len(cat.CombatProfiles) == 0 {
		cat.CombatProfiles = nil
	}
	if len(cat.ActionSpecs) == 0 {
		cat.ActionSpecs = nil
	}
	if len(cat.StateSlots) == 0 {
		cat.StateSlots = nil
	}
	if len(cat.Forms) == 0 {
		cat.Forms = nil
	}
	if len(cat.Transitions) == 0 {
		cat.Transitions = nil
	}
	if len(cat.Currencies) == 0 {
		cat.Currencies = nil
	}
	if len(cat.Techs) == 0 {
		cat.Techs = nil
	}
	if len(cat.DefaultWorldLaws) == 0 {
		cat.DefaultWorldLaws = nil
	}
	return sim.CloneCatalog(cat)
}

func normalizeMap(m sim.WorldMap) sim.WorldMap {
	if m.Districts == nil {
		m.Districts = []sim.District{}
	}
	for i := range m.Districts {
		if len(m.Districts[i].Features) == 0 {
			m.Districts[i].Features = nil
		}
		if len(m.Districts[i].ResourceCapacities) == 0 {
			m.Districts[i].ResourceCapacities = nil
		}
	}
	if m.Roads == nil {
		m.Roads = []sim.Road{}
	}
	if len(m.GeoRegions) == 0 {
		m.GeoRegions = nil
	} else {
		for i := range m.GeoRegions {
			if len(m.GeoRegions[i].Resources) == 0 {
				m.GeoRegions[i].Resources = nil
			}
			if len(m.GeoRegions[i].AmbientFields) == 0 {
				m.GeoRegions[i].AmbientFields = nil
			}
		}
	}
	if len(m.GeoFlows) == 0 {
		m.GeoFlows = nil
	}
	if len(m.Venues) == 0 {
		m.Venues = nil
	} else {
		for i := range m.Venues {
			if len(m.Venues[i].Features) == 0 {
				m.Venues[i].Features = nil
			}
		}
	}
	if len(m.VenueRoutes) == 0 {
		m.VenueRoutes = nil
	}
	return m
}

func normalizePolities(net *sim.PolityNetwork) *sim.PolityNetwork {
	if net == nil {
		return nil
	}
	if len(net.Nodes) == 0 {
		net.Nodes = nil
		return net
	}
	for i := range net.Nodes {
		if len(net.Nodes[i].EmbargoItems) == 0 {
			net.Nodes[i].EmbargoItems = nil
		}
		if len(net.Nodes[i].Decrees) == 0 {
			net.Nodes[i].Decrees = nil
		}
		if len(net.Nodes[i].Currencies) == 0 {
			net.Nodes[i].Currencies = nil
		}
	}
	return net
}

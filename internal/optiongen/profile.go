package optiongen

import (
	"fmt"
	"sort"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

var engineActionKinds = []string{
	"abandon_plan",
	"accept",
	"accept_agreement",
	"accept_growth_care",
	"accept_loan",
	"adopt_facility",
	"arrest",
	"attack",
	"audit_corruption",
	"avenge_feud",
	"begin_rest",
	"borrow_loan",
	"bribe",
	"cancel_loan",
	"claim_turf",
	"craft",
	"drink",
	"eat",
	"end_rest",
	"enter_house",
	"equip",
	"extort",
	"file_appeal",
	"foreclose_collateral",
	"frame_suspect",
	"fulfill_agreement",
	"harvest",
	"heal",
	"hunt_threat",
	"issue_decree",
	"join_faction",
	"join_profession",
	"leave_house",
	"maintain_infra",
	"marry",
	"medicate",
	"move",
	"move_venue",
	"offer",
	"perform_ritual",
	"propose_agreement",
	"propose_growth_care",
	"raid",
	"refuel",
	"reject",
	"reject_agreement",
	"reject_loan",
	"relieve",
	"repair_house",
	"repay_loan",
	"research_tech",
	"shoot",
	"sleep",
	"spar",
	"speak",
	"start_plan",
	"steal",
	"swear_bond",
	"teach_knowledge",
	"transfer_house",
	"transmit_message",
	"unequip",
	"upgrade_house",
	"wait",
}

// BuildDossier assembles the complete description of a single actor together with
// all relevant world, social, political, spatial, and catalog context.
func BuildDossier(w *sim.World, actorID string, horizonDays int) (CharacterDossier, error) {
	if w == nil {
		return CharacterDossier{}, fmt.Errorf("world is nil")
	}
	if horizonDays <= 0 {
		horizonDays = 30
	}
	var target *sim.Actor
	for i := range w.Actors {
		if w.Actors[i].ID == actorID {
			a := w.Actors[i]
			target = &a
			break
		}
	}
	if target == nil {
		return CharacterDossier{}, fmt.Errorf("actor %q not found in world %q", actorID, w.ID)
	}

	cal := sim.CalendarAt(w.Tick, w.Catalog.Calendar)
	env := w.Catalog.EffectiveEnvironment()
	worldSummary := WorldContextSummary{
		ID:             w.ID,
		Tick:           w.Tick,
		TicksPerDay:    sim.TicksPerDay,
		HorizonDays:    horizonDays,
		HorizonTicks:   horizonDays * sim.TicksPerDay,
		Clock:          sim.ClockAt(w.Tick).Label,
		Year:           cal.Year,
		SeasonID:       cal.SeasonID,
		DayOfSeason:    cal.DayOfSeason,
		PublicTreasury: w.PublicTreasury,
		WorldLaws:      w.WorldLaws,
		Environment:    env,
		Glossaries:     buildGlossaries(w),
	}

	var home *sim.House
	var ownedHouses []sim.House
	for _, h := range w.Houses {
		if h.ID == target.HomeID {
			hc := h
			home = &hc
		}
		if h.OwnerID == target.ID {
			ownedHouses = append(ownedHouses, h)
		}
	}

	districts := make([]DistrictSummary, 0, len(w.Map.Districts))
	var currentDistrict DistrictSummary
	for _, d := range w.Map.Districts {
		ds := DistrictSummary{
			ID:                 d.ID,
			Name:               d.Name,
			GeoRegionID:        d.GeoRegionID,
			PolityID:           d.PolityID,
			Terrain:            d.Terrain,
			Features:           d.Features,
			ResourceCapacities: d.ResourceCapacities,
		}
		if st, ok := w.DistrictStates[d.ID]; ok {
			ds.Weather = st.Weather
			ds.Sanitation = st.Sanitation
			ds.Corruption = st.Corruption
			ds.RacketPressure = st.RacketPressure
			ds.Resources = st.Resources
			ds.Infrastructure = st.Infrastructure
			ds.ThreatActivity = st.ThreatActivity
			ds.ActiveDisasters = len(st.ActiveDisasters)
		}
		districts = append(districts, ds)
		if d.ID == target.Location {
			currentDistrict = ds
		}
	}

	var actorFields []sim.FieldAsset
	for _, f := range w.Production.Fields {
		if f.OwnerID == target.ID || f.UserID == target.ID {
			actorFields = append(actorFields, f)
		}
	}

	var actorGrowth []sim.GrowthBatch
	for _, g := range w.Growth {
		if !g.Cohort && g.OwnerID == target.ID {
			actorGrowth = append(actorGrowth, g)
		}
	}

	politiesByLeader := map[string][]string{}
	var polities []PolitySummary
	if w.Polities != nil {
		for _, node := range w.Polities.Nodes {
			if node.LeaderActorID != "" {
				politiesByLeader[node.LeaderActorID] = append(politiesByLeader[node.LeaderActorID], node.ID)
			}
			polities = append(polities, PolitySummary{
				ID:                  node.ID,
				Name:                node.Name,
				Level:               node.Level,
				DeJureParentID:      node.DeJureParentID,
				DeFactoControllerID: node.DeFactoControllerID,
				SeatDistrictID:      node.SeatDistrictID,
				SeatVenueID:         node.SeatVenueID,
				LeaderActorID:       node.LeaderActorID,
				Treasury:            node.Treasury,
				MarketTaxPermille:   node.MarketTaxPermille,
				PollTaxPermille:     node.PollTaxPermille,
				Curfew:              node.Curfew,
				EmbargoItems:        node.EmbargoItems,
				ActorIsLeader:       node.LeaderActorID == target.ID,
			})
		}
	}

	factions := make([]FactionSummary, 0, len(w.Factions))
	for _, f := range w.Factions {
		isMember := false
		for _, m := range f.Members {
			if m == target.ID {
				isMember = true
				break
			}
		}
		factions = append(factions, FactionSummary{
			ID:            f.ID,
			Name:          f.Name,
			District:      f.District,
			Professions:   f.Professions,
			Members:       f.Members,
			Level:         f.Level,
			Influence:     f.Influence,
			ActorIsMember: isMember,
		})
	}

	otherActors := make([]OtherActorSummary, 0, max(0, len(w.Actors)-1))
	for _, other := range w.Actors {
		if other.ID == target.ID || other.Dead {
			continue
		}
		otherActors = append(otherActors, OtherActorSummary{
			ID:             other.ID,
			Name:           other.Name,
			Role:           other.Role,
			Tags:           other.Tags,
			Professions:    other.Professions,
			Guilds:         other.Guilds,
			Location:       other.Location,
			VenueID:        other.VenueID,
			HomeID:         other.HomeID,
			AgeYears:       other.AgeYears,
			Money:          other.Money,
			Items:          other.Items,
			Goal:           other.Goal,
			Personality:    other.Personality,
			CoLocated:      sim.SamePlace(*target, other),
			MyTrustToThem:  target.Trust[other.ID],
			TheirTrustToMe: other.Trust[target.ID],
			LeadsPolityIDs: politiesByLeader[other.ID],
		})
	}

	catalog := summarizeCatalog(w.Catalog)

	view := w.View(target.ID)
	rawOpts := agent.Options(view)
	baseline := make([]BaselineOptionBrief, 0, len(rawOpts))
	for _, opt := range rawOpts {
		switch opt.Action.Kind {
		case "eat", "drink", "sleep", "wait", "begin_rest", "end_rest", "enter_house", "leave_house":
			continue
		}
		baseline = append(baseline, BaselineOptionBrief{
			ID:       opt.ID,
			Label:    opt.Label,
			Kind:     opt.Action.Kind,
			TargetID: opt.Action.TargetID,
			Location: opt.Action.Location,
			RecipeID: opt.Action.RecipeID,
			Item:     opt.Action.Item,
		})
	}

	return CharacterDossier{
		World:                worldSummary,
		Actor:                *target,
		Home:                 home,
		OwnedHouses:          ownedHouses,
		CurrentDistrict:      currentDistrict,
		ActorFields:          actorFields,
		ActorGrowthBatches:   actorGrowth,
		Polities:             polities,
		Factions:             factions,
		OtherActors:          otherActors,
		Districts:            districts,
		Roads:                w.Map.Roads,
		Venues:               w.Map.Venues,
		Catalog:              catalog,
		EngineActionKinds:    activeEngineActionKinds(w.Catalog),
		Tick0BaselineOptions: baseline,
	}, nil
}

func buildGlossaries(w *sim.World) WorldGlossaries {
	g := WorldGlossaries{
		Locations:   make(map[string]string, len(w.Map.Districts)),
		Items:       make(map[string]string, len(w.Catalog.Items)),
		Professions: make(map[string]string, len(w.Catalog.Professions)),
		Factions:    make(map[string]string, len(w.Factions)),
		Recipes:     make(map[string]string, len(w.Catalog.Recipes)),
	}
	for _, d := range w.Map.Districts {
		name := d.Name
		if name == "" {
			name = d.ID
		}
		g.Locations[d.ID] = name
	}
	for id, spec := range w.Catalog.Items {
		name := spec.Name
		if name == "" {
			name = id
		}
		g.Items[id] = name
	}
	for _, p := range w.Catalog.Professions {
		name := p.Name
		if name == "" {
			name = p.ID
		}
		g.Professions[p.ID] = name
	}
	for _, f := range w.Factions {
		name := f.Name
		if name == "" {
			name = f.ID
		}
		g.Factions[f.ID] = name
	}
	for _, r := range w.Catalog.Recipes {
		name := r.Label
		if name == "" {
			name = r.ID
		}
		g.Recipes[r.ID] = name
	}
	return g
}

func activeEngineActionKinds(c sim.Catalog) []string {
	env := c.EffectiveEnvironment()
	inst := env.Institutions
	disabled := map[string]bool{
		// Physiological/idle actions are handled automatically by the engine.
		"eat": true, "drink": true, "sleep": true, "wait": true,
		"begin_rest": true, "end_rest": true, "relieve": true,
	}
	if !inst.RacketeeringEnabled() {
		disabled["extort"] = true
	}
	if !inst.EnforcementEnabled() {
		disabled["arrest"] = true
	}
	if !inst.OversightEnabled() {
		disabled["audit_corruption"] = true
		disabled["file_appeal"] = true
	}
	if !inst.RitualsEnabled() {
		disabled["perform_ritual"] = true
	}

	seen := map[string]bool{}
	if len(c.TagActions) > 0 {
		for _, acts := range c.TagActions {
			for _, act := range acts {
				if act != "" && !disabled[act] {
					seen[act] = true
				}
			}
		}
		for actID := range c.ActionSpecs {
			if actID != "" && !disabled[actID] {
				seen[actID] = true
			}
		}
	}
	if len(seen) == 0 {
		for _, act := range engineActionKinds {
			if !disabled[act] {
				seen[act] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for act := range seen {
		out = append(out, act)
	}
	sort.Strings(out)
	return out
}

func summarizeCatalog(c sim.Catalog) CatalogSummary {
	itemKeys := make([]string, 0, len(c.Items))
	for k := range c.Items {
		itemKeys = append(itemKeys, k)
	}
	sort.Strings(itemKeys)
	items := make([]ItemSummary, 0, len(itemKeys))
	for _, k := range itemKeys {
		spec := c.Items[k]
		items = append(items, ItemSummary{
			ID:             spec.ID,
			Name:           spec.Name,
			Tier:           spec.Tier,
			Edible:         spec.Edible,
			Potable:        spec.Potable,
			NutritionUnits: spec.NutritionUnits,
			MedicinalHeal:  spec.MedicinalHeal,
			DiseaseCure:    spec.DiseaseCure,
			Tradeable:      spec.Tradeable,
			BatchQty:       spec.BatchQty,
			BatchPrice:     spec.BatchPrice,
		})
	}

	recipes := make([]RecipeSummary, 0, len(c.Recipes))
	for _, r := range c.Recipes {
		growthDays := 0
		if r.Growth != nil && r.Growth.DurationTicks > 0 {
			growthDays = int(r.Growth.DurationTicks / sim.TicksPerDay)
		}
		recipes = append(recipes, RecipeSummary{
			ID:                 r.ID,
			Label:              r.Label,
			Profession:         r.Profession,
			RequiredTag:        r.RequiredTag,
			MinProfessionLevel: r.MinProfessionLevel,
			NodeFeature:        r.NodeFeature,
			HouseFacility:      r.HouseFacility,
			ResourcePool:       r.ResourcePool,
			Ticks:              r.Ticks,
			GrowthDays:         growthDays,
			Inputs:             r.Inputs,
			OutputItem:         r.OutputItem,
			OutputQty:          r.OutputQty,
		})
	}

	profs := make([]ProfessionSummary, 0, len(c.Professions))
	for _, p := range c.Professions {
		profs = append(profs, ProfessionSummary{
			ID:               p.ID,
			Name:             p.Name,
			Tier:             p.Tier,
			Guild:            p.Guild,
			ParentProfession: p.ParentProfession,
			MinParentLevel:   p.MinParentLevel,
			Ranks:            p.Ranks,
		})
	}

	chKeys := make([]string, 0, len(c.CommChannels))
	for k := range c.CommChannels {
		chKeys = append(chKeys, k)
	}
	sort.Strings(chKeys)
	channels := make([]sim.CommChannelSpec, 0, len(chKeys))
	for _, k := range chKeys {
		channels = append(channels, c.CommChannels[k])
	}

	return CatalogSummary{
		Items:        items,
		Recipes:      recipes,
		Professions:  profs,
		CommChannels: channels,
	}
}

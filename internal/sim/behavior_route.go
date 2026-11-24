package sim

type PlannedRoute struct {
	Steps  []Action
	Ticks  int64
	Energy int
}

// PlanRoute uses published topology, witnessed route changes and the weather
// visible at the current location. Execution reevaluates each hop on arrival.
func (p Perception) PlanRoute(district, venue string) (PlannedRoute, bool) {
	actor := p.Self
	known := World{Tick: p.Tick, Map: p.Map, Catalog: p.Catalog, ActiveWorldEvents: p.ActiveWorldEvents}
	known.DistrictStates = p.knownWeather()
	var result PlannedRoute
	if len(p.Map.Venues) != 0 && actor.VenueID != "" {
		if venue == "" {
			venue = p.Map.DefaultVenueForDistrict(district)
		}
		routeMap := p.Map
		routeMap.VenueRoutes = make([]VenueRoute, 0, len(p.Map.VenueRoutes))
		for _, r := range p.Map.VenueRoutes {
			from, _ := p.Map.VenueByID(r.FromVenue)
			to, _ := p.Map.VenueByID(r.ToVenue)
			if severed, _ := known.isRouteSevered(from.DistrictID, to.DistrictID, r.Medium, ""); !severed {
				routeMap.VenueRoutes = append(routeMap.VenueRoutes, r)
			}
		}
		for here := actor.VenueID; here != venue; {
			hop, _, mode, ok := routeMap.NextVenueHopForActor(p.Catalog, known.DistrictStates, actor, here, venue)
			if !ok {
				return PlannedRoute{}, false
			}
			from, _ := p.Map.VenueByID(here)
			weather, visible := known.DistrictStates[from.DistrictID]
			var local *DistrictState
			if visible {
				local = &weather
			}
			ticks, energy, _, _ := locomotionCostForActor(p.Catalog, local, actor, hop)
			result.Steps = append(result.Steps, Action{Kind: "move_venue", Location: hop.ToVenue, Channel: mode})
			result.Ticks += int64(ticks)
			result.Energy += energy
			here = hop.ToVenue
		}
		return result, true
	}

	routeMap := p.Map
	routeMap.Roads = make([]Road, 0, len(p.Map.Roads))
	for _, road := range p.Map.Roads {
		if severed, _ := known.isRouteSevered(road.From, road.To, road.Medium, ""); !severed {
			road.Ticks, road.EnergyCost = known.effectiveRoadCost(road)
			routeMap.Roads = append(routeMap.Roads, road)
		}
	}
	for here := actor.Location; here != district; {
		hop, _, ok := routeMap.NextHop(here, district)
		if !ok {
			return PlannedRoute{}, false
		}
		result.Steps = append(result.Steps, Action{Kind: "move", Location: hop.To})
		result.Ticks += int64(hop.Ticks)
		result.Energy += hop.EnergyCost
		here = hop.To
	}
	return result, true
}

func (p Perception) knownWeather() map[string]DistrictState {
	states := make(map[string]DistrictState, len(p.Self.KnownFacts)+1)
	for _, fact := range p.Self.KnownFacts {
		if fact.Attribute == "weather" {
			states[fact.Subject] = DistrictState{Weather: fact.Value}
		}
	}
	if p.DistrictState != nil {
		states[p.Self.Location] = *p.DistrictState
	}
	return states
}

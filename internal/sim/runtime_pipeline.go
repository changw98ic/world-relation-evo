package sim

import "fmt"

// TickPhase is one explicit, named stage in the World.Step runtime pipeline.
// Extracting the post-tick world subsystems into an ordered slice of TickPhase
// makes the execution order inspectable, testable, and decoupled from inline code.
type TickPhase struct {
	Name string
	Run  func(eventsSoFar []Event) []Event
}

func (w *World) preTickPhases(due []int, actions []Action, wasBusy []bool) []TickPhase {
	return []TickPhase{
		{Name: "flow_grid", Run: func(_ []Event) []Event { return w.stepFlowGrid() }},
		{Name: "stored_actor_tasks", Run: func(_ []Event) []Event {
			var tasks []string
			for _, c := range w.Cohorts {
				for _, a := range c.Members {
					if w.Tick >= a.JailedUntil && a.Task != nil {
						tasks = append(tasks, a.ID)
					}
				}
			}
			var events []Event
			for _, id := range tasks {
				events = append(events, w.Advance(id)...)
			}
			return events
		}},
		{Name: "due_actor_tasks", Run: func(_ []Event) []Event {
			var events []Event
			for _, i := range due {
				a := &w.Actors[i]
				if !a.Dead && w.Tick >= a.JailedUntil && a.Task != nil {
					events = append(events, w.Advance(a.ID)...)
				}
			}
			return events
		}},
		{Name: "actor_actions", Run: func(_ []Event) []Event {
			var events []Event
			for n, i := range due {
				if w.Actors[i].Dead {
					continue
				}
				if w.Tick < w.Actors[i].JailedUntil {
					if w.Actors[i].Energy < MaxEnergy {
						setActorEnergy(&w.Actors[i], w.Actors[i].Energy+1)
					}
					if w.Tick%4 == 0 && w.Actors[i].Health < MaxHealth && w.Actors[i].Hunger <= 2 && w.Actors[i].Thirst <= 1 {
						adjustActorHealthBounded(&w.Actors[i], 1, 0)
					}
					continue
				}
				if !wasBusy[n] && !isRelocationAction(actions[n].Kind) {
					events = append(events, w.Apply(w.Actors[i].ID, actions[n])...)
				}
			}
			return events
		}},
		{Name: "actor_relocation", Run: func(_ []Event) []Event {
			var events []Event
			for n, i := range due {
				if !w.Actors[i].Dead && !wasBusy[n] && isRelocationAction(actions[n].Kind) {
					events = append(events, w.Apply(w.Actors[i].ID, actions[n])...)
				}
			}
			return events
		}},
		{Name: "market_and_credit", Run: func(_ []Event) []Event { return w.StepMarketAndCredit(w.ensureMarketCredit()) }},
		{Name: "cohorts", Run: func(_ []Event) []Event {
			if len(w.Cohorts) > 0 {
				return w.stepCohorts()
			}
			return nil
		}},
	}
}

func runTickPhases(events []Event, phases []TickPhase) []Event {
	for _, phase := range phases {
		events = append(events, phase.Run(events)...)
	}
	return events
}

// postTickPhases constructs the canonical ordered subsystem stages executed after
// w.Tick++ during World.Step, preserving exact deterministic settlement order.
func (w *World) postTickPhases() []TickPhase {
	return []TickPhase{
		{Name: "arrived_messages", Run: func(_ []Event) []Event { return w.deliverScheduledMessages() }},
		{Name: "shipments", Run: func(_ []Event) []Event { return w.settleShipments() }},
		{Name: "growth", Run: func(_ []Event) []Event { return w.AdvanceGrowth() }},
		{Name: "active_effects", Run: func(_ []Event) []Event { return w.stepActiveEffects() }},
		{Name: "continuity", Run: func(_ []Event) []Event { return w.AdvanceContinuity() }},
		{Name: "actor_hunger", Run: func(_ []Event) []Event {
			for i := range w.Actors {
				w.advanceActorHunger(&w.Actors[i])
			}
			return nil
		}},
		{Name: "world_environment", Run: func(_ []Event) []Event { return w.stepWorldEnvironment() }},
		{Name: "world_events", Run: func(_ []Event) []Event { return w.stepWorldEvents() }},
		{Name: "adopted_facilities", Run: func(_ []Event) []Event { return w.stepAdoptedFacilities() }},
		{Name: "state_slots_and_forms", Run: func(_ []Event) []Event { return w.stepStateSlotsAndForms() }},
		{Name: "actor_physiology_and_lifecycle", Run: func(_ []Event) []Event { return w.stepActorPhysiologyAndLifecycle() }},
		{Name: "prisoner_release", Run: func(_ []Event) []Event { return w.stepPrisonerReleases() }},
		{Name: "house_weathering", Run: func(_ []Event) []Event {
			w.weatherHouses()
			return nil
		}},
		{Name: "offer_expiration", Run: func(_ []Event) []Event { return w.stepOfferExpirations() }},
		{Name: "relations_and_turf", Run: func(_ []Event) []Event { return w.StepRelationsAndTurf(w.ensureRelations()) }},
		{Name: "justice_and_revolt", Run: func(_ []Event) []Event { return w.StepJusticeAndRevolt(w.ensureJusticeRevolt()) }},
		{Name: "causal_relations", Run: func(evs []Event) []Event { return w.ensureRelations().ObserveCausalRelationEvents(w, evs) }},
		{Name: "parties", Run: func(_ []Event) []Event { return w.stepParties() }},
		{Name: "diplomacy", Run: func(evs []Event) []Event { return w.stepDiplomacy(evs) }},
		{Name: "ambitions_and_secrets", Run: func(evs []Event) []Event { return w.stepAmbitionsAndSecrets(evs) }},
		{Name: "golden_fingers", Run: func(evs []Event) []Event { return w.stepGoldenFingers(evs) }},
		{Name: "refresh_actors", Run: func(_ []Event) []Event {
			for i := range w.Actors {
				w.refreshActor(&w.Actors[i])
			}
			return nil
		}},
		{Name: "wound_reconciliation", Run: func(_ []Event) []Event { return w.stepWoundReconciliation() }},
		{Name: "cohort_mortality_drain", Run: func(_ []Event) []Event { return w.drainCohortMortalityEvents() }},
		{Name: "spotlight_and_rumors", Run: func(evs []Event) []Event { return w.ObserveSpotlightAndRumors(w.ensureSpotlightRumor(), evs) }},
		{Name: "dynamic_spotlight", Run: func(_ []Event) []Event { return w.StepDynamicSpotlight(w.ensureSpotlightRumor()) }},
	}
}

// PostTickPhaseNames returns the ordered names of all post-increment tick phases.
func (w *World) PostTickPhaseNames() []string {
	phases := w.postTickPhases()
	names := make([]string, len(phases))
	for i, ph := range phases {
		names[i] = ph.Name
	}
	return names
}

func (w *World) stepPrisonerReleases() []Event {
	var events []Event
	for i := range w.Actors {
		events = append(events, w.releaseCompletedSentence(&w.Actors[i])...)
	}
	return events
}

func (w *World) releaseCompletedSentence(a *Actor) []Event {
	if a.Dead || a.JailedUntil <= 0 || w.Tick < a.JailedUntil {
		return nil
	}
	(*WorldTx)(nil).setSentence(a, 0)
	a.RestUntil = w.Tick + 48
	w.settleReleasedCases(a.ID)
	return []Event{w.record(a, Action{Kind: "wait"}, "released_from_jail", map[string]any{
		"text": fmt.Sprintf("%s served their sentence and was released", a.Name),
	})}
}

func (w *World) stepOfferExpirations() []Event {
	var events []Event
	var activeOffers []Offer
	for _, o := range w.Offers {
		if o.Expires >= w.Tick {
			activeOffers = append(activeOffers, o)
		} else if proposer := w.actor(o.ProposerID()); proposer != nil {
			action := Action{Kind: "offer_expired", OfferID: o.ID, TargetID: o.ResponderID(), Item: o.Item, Quantity: o.Quantity, Price: o.Price, Currency: o.Currency}
			events = append(events, w.record(proposer, action, "trade_expired", map[string]any{
				"text":           fmt.Sprintf("offer %d expired without assent", o.ID),
				"cause_event_id": o.ID,
				"root_event_id":  o.causalRoot(),
				"trade":          o,
			}, w.actor(o.ResponderID())))
		}
	}
	w.Offers = activeOffers
	return events
}

func (w *World) stepWoundReconciliation() []Event {
	if len(w.Cohorts) == 0 {
		return nil
	}
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
	if actorWounded == 0 && actorHealed == 0 && actorWoundedDeaths == 0 {
		return nil
	}
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
	return []Event{ev}
}

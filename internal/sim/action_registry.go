package sim

import (
	"slices"
	"sort"
)

// BuiltinActionHandler executes one built-in action kind after World.apply entry guards pass.
type BuiltinActionHandler func(tx *WorldTx, a, b *Actor, x Action) []Event

var builtinActionHandlers = map[string]BuiltinActionHandler{}

func registerBuiltinAction(kind string, h BuiltinActionHandler) {
	builtinActionHandlers[kind] = h
}

func actionEmit(tx *WorldTx, a *Actor, x Action, kind, text string, observers ...*Actor) []Event {
	return []Event{tx.record(a, x, kind, map[string]any{"text": text}, observers...)}
}

func actionFail(tx *WorldTx, a *Actor, x Action, reason string) []Event {
	return actionEmit(tx, a, x, "action_failed", reason)
}

// RegisteredBuiltinActionKinds returns all sorted built-in action kinds registered in the ActionRegistry.
func RegisteredBuiltinActionKinds() []string {
	keys := make([]string, 0, len(builtinActionHandlers))
	for k := range builtinActionHandlers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	registerSurvivalActions()
	registerMovementHousingActions()
	registerProductionEcologyActions()
	registerCombatCrimeActions()
	registerTradeSocialActions()
	registerDomainSubsystemActions()
}

func registerDomainSubsystemActions() {
	registerBuiltinAction("move_venue", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyMoveVenue(tx, a, x) })
	registerBuiltinAction("issue_decree", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyIssueDecree(tx, a, x) })
	registerBuiltinAction("transmit_message", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyTransmitMessage(tx, a, x) })
	registerBuiltinAction("claim_world_event", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyClaimWorldEvent(tx, a, x) })

	registerBuiltinAction("form_party", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyFormParty(tx, a, x) })
	registerBuiltinAction("join_party", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyJoinParty(tx, a, x) })
	registerBuiltinAction("leave_party", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyLeaveParty(tx, a, x) })
	registerBuiltinAction("betray_party", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyBetrayParty(tx, a, x) })

	registerBuiltinAction("propose_alliance", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyProposeAlliance(tx, a, x) })
	registerBuiltinAction("declare_war", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyDeclareWar(tx, a, x) })
	registerBuiltinAction("make_peace", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyMakePeace(tx, a, x) })

	registerBuiltinAction("set_ambition", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applySetAmbition(tx, a, x) })
	registerBuiltinAction("challenge_leadership", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyLeadershipChallenge(tx, a, x) })
	registerBuiltinAction("investigate_actor", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyInvestigateActor(tx, a, x) })
	registerBuiltinAction("blackmail", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyBlackmail(tx, a, x) })
	registerBuiltinAction("expose_secret", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyExposeSecret(tx, a, x) })

	registerBuiltinAction("gf_exchange", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyGFExchange(tx, a, x) })
	registerBuiltinAction("gf_simulate", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyGFSimulate(tx, a, x) })

	registerBuiltinAction("equip", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyEquip(tx, a, x) })
	registerBuiltinAction("unequip", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyUnequip(tx, a, x) })

	registerBuiltinAction("open_case", func(tx *WorldTx, a, b *Actor, x Action) []Event { return tx.World.openPersonalCase(tx, a, b, x) })
	registerBuiltinAction("audit_corruption", func(tx *WorldTx, a, b *Actor, x Action) []Event { return tx.World.tryPersonalCase(tx, a, b, x) })
	registerBuiltinAction("hold_trial", func(tx *WorldTx, a, b *Actor, x Action) []Event { return tx.World.tryPersonalCase(tx, a, b, x) })

	registerBuiltinAction("attempt_transition", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyAttemptTransition(tx, a, x) })
	registerBuiltinAction("refine_currency_item", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyRefineCurrencyItem(tx, a, x) })
	registerBuiltinAction("absorb_item_slot", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyAbsorbItemSlot(tx, a, x) })
	registerBuiltinAction("sever_flow_edge", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applySeverFlowEdge(tx, a, x) })
	registerBuiltinAction("repair_flow_edge", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyRepairFlowEdge(tx, a, x) })

	registerBuiltinAction("research_tech", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyResearchTech(tx, a, x) })
	registerBuiltinAction("teach_knowledge", func(tx *WorldTx, a, b *Actor, x Action) []Event { return tx.World.applyTeachKnowledge(tx, a, x, b) })
	registerBuiltinAction("adopt_facility", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyAdoptFacility(tx, a, x) })
	registerBuiltinAction("rewrite_world_law", func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyRewriteWorldLaw(tx, a, x) })

	for _, kind := range []string{"propose_growth_care", "accept_growth_care", "dispatch_harvest"} {
		registerBuiltinAction(kind, func(tx *WorldTx, a, _ *Actor, x Action) []Event { return tx.World.applyGrowthCareAction(tx, a, x) })
	}

	for _, kind := range continuityActionKinds {
		registerBuiltinAction(kind, func(tx *WorldTx, a, _ *Actor, x Action) []Event {
			return tx.World.applyContinuityTx(tx, a, x)
		})
	}
	for _, kind := range marketCreditActionKinds {
		registerBuiltinAction(kind, func(tx *WorldTx, a, _ *Actor, x Action) []Event {
			state := tx.marketCreditOverride
			if !tx.hasExplicitSubsystem {
				state = tx.World.ensureMarketCredit()
			}
			return tx.World.applyMarketCreditActionTx(tx, state, a, x)
		})
	}
	for _, kind := range relationActionKinds {
		registerBuiltinAction(kind, func(tx *WorldTx, a, _ *Actor, x Action) []Event {
			state := tx.relationsOverride
			if !tx.hasExplicitSubsystem {
				state = tx.World.ensureRelations()
			}
			return tx.World.applyRelationActionTx(tx, state, a, x)
		})
	}
	for _, kind := range justiceActionKinds {
		registerBuiltinAction(kind, func(tx *WorldTx, a, _ *Actor, x Action) []Event {
			state := tx.justiceRevoltOverride
			if !tx.hasExplicitSubsystem {
				state = tx.World.ensureJusticeRevolt()
			}
			return tx.World.applyJusticeActionTx(tx, state, a, x)
		})
	}
}

var (
	continuityActionKinds   = []string{"start_plan", "abandon_plan", "propose_agreement", "accept_agreement", "reject_agreement", "fulfill_agreement"}
	marketCreditActionKinds = []string{"borrow_loan", "accept_loan", "reject_loan", "cancel_loan", "repay_loan", "foreclose_collateral"}
	relationActionKinds     = []string{"marry", "swear_bond", "avenge_feud", "mediate_feud", "claim_turf"}
	justiceActionKinds      = []string{"frame_suspect", "file_appeal"}
)

func (w *World) applyMarketCreditSubsystem(state *MarketCreditState, actorID string, x Action) ([]Event, bool) {
	if !slices.Contains(marketCreditActionKinds, x.Kind) {
		return nil, false
	}
	if state != nil && state == w.MarketCredit {
		return w.Apply(actorID, x), true
	}
	tx := w.BeginTx(actorID, ActionIntent{Action: x})
	tx.marketCreditOverride = state
	tx.hasExplicitSubsystem = true
	return (DefaultActionExecutor{}).executeTx(tx, w.actor(actorID)).Events, true
}

func (w *World) applyRelationSubsystem(state *RelationEvoState, actorID string, x Action) ([]Event, bool) {
	if !slices.Contains(relationActionKinds, x.Kind) {
		return nil, false
	}
	if state != nil && state == w.Relations {
		return w.Apply(actorID, x), true
	}
	tx := w.BeginTx(actorID, ActionIntent{Action: x})
	tx.relationsOverride = state
	tx.hasExplicitSubsystem = true
	return (DefaultActionExecutor{}).executeTx(tx, w.actor(actorID)).Events, true
}

func (w *World) applyJusticeSubsystem(state *JusticeRevoltState, actorID string, x Action) ([]Event, bool) {
	if !slices.Contains(justiceActionKinds, x.Kind) {
		return nil, false
	}
	if state != nil && state == w.JusticeRevolt {
		return w.Apply(actorID, x), true
	}
	tx := w.BeginTx(actorID, ActionIntent{Action: x})
	tx.justiceRevoltOverride = state
	tx.hasExplicitSubsystem = true
	return (DefaultActionExecutor{}).executeTx(tx, w.actor(actorID)).Events, true
}

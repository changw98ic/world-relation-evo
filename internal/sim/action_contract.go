package sim

import "sort"

// ActionIntent wraps an Action with decision/rule provenance for unified
// candidate -> decision -> execution tracing.
type ActionIntent struct {
	Action      Action   `json:"action"`
	CandidateID string   `json:"candidate_id,omitempty"`
	RuleIDs     []string `json:"rule_ids,omitempty"`
	RuleReason  string   `json:"rule_reason,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// ActionCheckResult is the single-source precondition verdict shared by
// candidate construction (Perception) and authoritative execution (World).
type ActionCheckResult struct {
	Allowed          bool   `json:"allowed"`
	BlockReason      string `json:"block_reason,omitempty"`
	routeUnavailable bool
	workRate         actionWorkRate
}

func AllowAction() ActionCheckResult {
	return ActionCheckResult{Allowed: true}
}

func BlockAction(reason string) ActionCheckResult {
	if reason == "" {
		reason = "action blocked"
	}
	return ActionCheckResult{Allowed: false, BlockReason: reason}
}

// Candidate is an action and its parameters as viewed by the actor.
type Candidate struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Action       Action   `json:"action"`
	Source       string   `json:"source,omitempty"`
	RuleIDs      []string `json:"rule_ids,omitempty"`
	RuleReason   string   `json:"rule_reason,omitempty"`
	PriorityHint float64  `json:"priority_hint,omitempty"`
}

func (c Candidate) Intent() ActionIntent {
	return ActionIntent{Action: c.Action, CandidateID: c.ID, RuleIDs: c.RuleIDs, RuleReason: c.RuleReason, Source: c.Source}
}

// StateMutationRecord records an atomic state mutation applied via StatePrimitives.
type StateMutationRecord struct {
	Primitive string `json:"primitive"`
	ActorID   string `json:"actor_id,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	Resource  string `json:"resource,omitempty"`
	Delta     int64  `json:"delta,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// ExecutionResult is the structured settlement outcome returned by ActionExecutor.
type ExecutionResult struct {
	Succeeded bool                  `json:"succeeded"`
	Reason    string                `json:"reason,omitempty"`
	Events    []Event               `json:"events,omitempty"`
	Mutations []StateMutationRecord `json:"mutations,omitempty"`
}

// ActionMetadata is the single authoritative descriptor for every engine built-in
// and declarative action kind.
type ActionMetadata struct {
	Kind            string `json:"kind"`
	Category        string `json:"category"`
	Targeted        bool   `json:"targeted,omitempty"`
	Relocation      bool   `json:"relocation,omitempty"`
	MultiTick       bool   `json:"multi_tick,omitempty"`
	Builtin         bool   `json:"builtin,omitempty"`
	Declarative     bool   `json:"declarative,omitempty"`
	referenceEnergy int
}

const (
	timeOnlyEnergy        = -1 // Recovery, time passing, or a change of intent.
	energyFromActionFacts = -2 // A route, recipe, facility, or communication channel owns the amount.
	lightActionEnergy     = 1  // One brief interaction or piece of paperwork.
	handlingActionEnergy  = 2  // A transaction, transfer, or handling job.
)

// ActionExecutor executes an ActionIntent against the authoritative World.
type ActionExecutor interface {
	Check(world *World, actorID string, intent ActionIntent) ActionCheckResult
	Execute(world *World, actorID string, intent ActionIntent) (ExecutionResult, error)
}

var builtinActionMetadataList = []ActionMetadata{
	// Survival & Rest
	{Kind: "wait", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "begin_rest", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "end_rest", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "sleep", Category: "survival", MultiTick: true, Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "eat", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "drink", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "relieve", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "medicate", Category: "survival", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "heal", Category: "survival", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},

	// Movement & Housing
	{Kind: "move", Category: "movement", Relocation: true, MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "move_venue", Category: "movement", Relocation: true, MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "enter_house", Category: "movement", Relocation: true, Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "leave_house", Category: "movement", Relocation: true, Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "repair_house", Category: "housing", MultiTick: true, Builtin: true, referenceEnergy: 8},
	{Kind: "upgrade_house", Category: "housing", MultiTick: true, Builtin: true, referenceEnergy: 12},
	{Kind: "transfer_house", Category: "housing", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},

	// Production, Growth & Energy
	{Kind: "craft", Category: "production", MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "refuel", Category: "production", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "harvest", Category: "production", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "propose_growth_care", Category: "production", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "accept_growth_care", Category: "production", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "dispatch_harvest", Category: "production", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "maintain_infra", Category: "ecology", MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "hunt_threat", Category: "ecology", Targeted: true, MultiTick: true, Builtin: true, referenceEnergy: 10},
	{Kind: "perform_ritual", Category: "ecology", MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "claim_world_event", Category: "world_event", Targeted: true, Builtin: true, referenceEnergy: 8},

	// Economy, Trade, Credit & Continuity
	{Kind: "offer", Category: "economy", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "accept", Category: "economy", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "reject", Category: "economy", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "market_buy", Category: "economy", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "market_sell", Category: "economy", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "start_plan", Category: "continuity", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "abandon_plan", Category: "continuity", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "propose_agreement", Category: "continuity", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "accept_agreement", Category: "continuity", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "reject_agreement", Category: "continuity", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "fulfill_agreement", Category: "continuity", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "borrow_loan", Category: "credit", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "accept_loan", Category: "credit", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "reject_loan", Category: "credit", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "cancel_loan", Category: "credit", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "repay_loan", Category: "credit", Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "foreclose_collateral", Category: "credit", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},

	// Factions, Professions, Skills & Equipment
	{Kind: "join_faction", Category: "faction", Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "join_profession", Category: "progression", Builtin: true, referenceEnergy: 4},
	{Kind: "practice_skill", Category: "progression", Builtin: true, referenceEnergy: 4},
	{Kind: "equip", Category: "equipment", Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "unequip", Category: "equipment", Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "attempt_transition", Category: "progression", MultiTick: true, Builtin: true, referenceEnergy: 4},
	{Kind: "refine_currency_item", Category: "progression", Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "absorb_item_slot", Category: "progression", Builtin: true, referenceEnergy: handlingActionEnergy},

	// Flow, Knowledge & World Laws
	{Kind: "sever_flow_edge", Category: "infrastructure", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "repair_flow_edge", Category: "infrastructure", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "research_tech", Category: "knowledge", MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "teach_knowledge", Category: "knowledge", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "adopt_facility", Category: "knowledge", MultiTick: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "rewrite_world_law", Category: "world_law", Builtin: true, referenceEnergy: 8},

	// Communication & Governance
	{Kind: "speak", Category: "communication", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "transmit_message", Category: "communication", Targeted: true, Builtin: true, referenceEnergy: energyFromActionFacts},
	{Kind: "issue_decree", Category: "polity", Builtin: true, referenceEnergy: 4},

	// Combat, Crime & Coercion
	{Kind: "attack", Category: "combat", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "spar", Category: "combat", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "steal", Category: "crime", Targeted: true, Builtin: true, referenceEnergy: 5},
	{Kind: "raid", Category: "crime", Targeted: true, Builtin: true, referenceEnergy: 8},
	{Kind: "extort", Category: "crime", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "bribe", Category: "crime", Targeted: true, Builtin: true, referenceEnergy: 3},

	// Justice & Enforcement
	{Kind: "open_case", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "arrest", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 7},
	{Kind: "audit_corruption", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "hold_trial", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "frame_suspect", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "file_appeal", Category: "justice", Targeted: true, Builtin: true, referenceEnergy: 4},

	// Relations, Kinship & Turf
	{Kind: "marry", Category: "relations", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "swear_bond", Category: "relations", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "avenge_feud", Category: "relations", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "mediate_feud", Category: "relations", Targeted: true, Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "claim_turf", Category: "faction", Builtin: true, referenceEnergy: 5},

	// Parties
	{Kind: "form_party", Category: "party", Targeted: true, Builtin: true, referenceEnergy: 3},
	{Kind: "join_party", Category: "party", Targeted: true, Builtin: true, referenceEnergy: 2},
	{Kind: "leave_party", Category: "party", Targeted: true, Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "betray_party", Category: "party", Targeted: true, Builtin: true, referenceEnergy: 6},

	// Diplomacy
	{Kind: "propose_alliance", Category: "diplomacy", Targeted: true, Builtin: true, referenceEnergy: 5},
	{Kind: "declare_war", Category: "diplomacy", Targeted: true, Builtin: true, referenceEnergy: 6},
	{Kind: "make_peace", Category: "diplomacy", Targeted: true, Builtin: true, referenceEnergy: 4},

	// Ambitions & Secrets
	{Kind: "set_ambition", Category: "ambition", Builtin: true, referenceEnergy: timeOnlyEnergy},
	{Kind: "challenge_leadership", Category: "faction", Builtin: true, referenceEnergy: handlingActionEnergy},
	{Kind: "investigate_actor", Category: "intrigue", Targeted: true, Builtin: true, referenceEnergy: 5},
	{Kind: "blackmail", Category: "intrigue", Targeted: true, Builtin: true, referenceEnergy: 4},
	{Kind: "expose_secret", Category: "intrigue", Targeted: true, Builtin: true, referenceEnergy: 4},

	// Golden Finger
	{Kind: "gf_exchange", Category: "golden_finger", Builtin: true, referenceEnergy: lightActionEnergy},
	{Kind: "gf_simulate", Category: "golden_finger", Builtin: true, referenceEnergy: lightActionEnergy},
}

var builtinActionMetadataIndex = func() map[string]ActionMetadata {
	m := make(map[string]ActionMetadata, len(builtinActionMetadataList))
	for _, meta := range builtinActionMetadataList {
		m[meta.Kind] = meta
	}
	return m
}()

// BuiltinActionMetadata returns a copy of the engine's built-in action descriptors.
func BuiltinActionMetadata() []ActionMetadata {
	out := make([]ActionMetadata, len(builtinActionMetadataList))
	copy(out, builtinActionMetadataList)
	return out
}

// ActionMetadataList returns the unified catalog of built-in + declarative ActionSpec metadata.
func (c Catalog) ActionMetadataList() []ActionMetadata {
	out := BuiltinActionMetadata()
	if len(c.ActionSpecs) == 0 {
		return out
	}
	keys := make([]string, 0, len(c.ActionSpecs))
	for k := range c.ActionSpecs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		spec := c.ActionSpecs[k]
		kind := spec.ID
		if kind == "" {
			kind = k
		}
		if kind == "" {
			continue
		}
		if _, exists := builtinActionMetadataIndex[kind]; exists {
			continue
		}
		out = append(out, ActionMetadata{
			Kind:        kind,
			Category:    "declarative",
			Targeted:    spec.TargetKind == "actor" || spec.TargetKind == "cohort",
			MultiTick:   spec.Ticks > 1,
			Declarative: true,
		})
	}
	return out
}

// ActionMetadataByKind looks up metadata for any built-in or declarative action kind.
func (c Catalog) ActionMetadataByKind(kind string) (ActionMetadata, bool) {
	if meta, ok := builtinActionMetadataIndex[kind]; ok {
		return meta, true
	}
	if spec, ok := c.ActionSpecs[kind]; ok {
		id := spec.ID
		if id == "" {
			id = kind
		}
		return ActionMetadata{
			Kind:        id,
			Category:    "declarative",
			Targeted:    spec.TargetKind == "actor" || spec.TargetKind == "cohort",
			MultiTick:   spec.Ticks > 1,
			Declarative: true,
		}, true
	}
	return ActionMetadata{}, false
}

// IsKnownActionKind reports whether kind is either a built-in engine action or a catalog ActionSpec.
func (c Catalog) IsKnownActionKind(kind string) bool {
	_, ok := c.ActionMetadataByKind(kind)
	return ok
}

// IsTargetedActionKind reports whether kind expects a target entity/party/cohort/actor.
func (c Catalog) IsTargetedActionKind(kind string) bool {
	meta, ok := c.ActionMetadataByKind(kind)
	return ok && meta.Targeted
}

// ActionMetadataList returns the world's unified action catalog (built-ins + ActionSpecs).
func (w *World) ActionMetadataList() []ActionMetadata {
	if w == nil {
		return BuiltinActionMetadata()
	}
	return w.Catalog.ActionMetadataList()
}

// ActionMetadataList returns the perceived unified action catalog (built-ins + ActionSpecs).
func (p Perception) ActionMetadataList() []ActionMetadata {
	return p.Catalog.ActionMetadataList()
}

// DefaultActionExecutor checks actions and settles them through a WorldTx.
type DefaultActionExecutor struct{}

func (DefaultActionExecutor) Check(world *World, actorID string, intent ActionIntent) ActionCheckResult {
	if world == nil {
		return BlockAction("world is nil")
	}
	return world.CheckAction(actorID, intent.Action)
}

func (DefaultActionExecutor) Execute(world *World, actorID string, intent ActionIntent) (ExecutionResult, error) {
	if world == nil {
		return ExecutionResult{Succeeded: false, Reason: "world is nil"}, nil
	}
	return DefaultActionExecutor{}.execute(world, actorID, intent), nil
}

func (e DefaultActionExecutor) execute(world *World, actorID string, intent ActionIntent) ExecutionResult {
	return e.executeTx(world.BeginTx(actorID, intent), world.actor(actorID))
}

func (DefaultActionExecutor) executeTx(tx *WorldTx, a *Actor) ExecutionResult {
	if a == nil {
		return ExecutionResult{Reason: "unknown actor: " + tx.ActorID}
	}
	tx.trackStoredActor(a.ID)
	tx.trackStoredActor(tx.Intent.Action.TargetID)
	return tx.commitEvents(tx.World.apply(tx, a, tx.Intent.Action))
}

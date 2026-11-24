package sim

import (
	"fmt"
	"math"
	"strings"
)

// WorldTx provides an explicit, traceable, rollback-capable state-mutation boundary
// around World modifications. All core state mutations (currency, items, vitals,
// movement, trust, memories, and events) compose through WorldTx so ledger
// conservation, death checks, nil-map safety, secret capture, and failure atomicity
// are enforced in one canonical place.
type WorldTx struct {
	World     *World
	ActorID   string
	Intent    ActionIntent
	Mutations []StateMutationRecord
	Events    []Event

	// Optional explicit subsystem states supplied by exported legacy entry points (strongly typed, no `any`).
	marketCreditOverride  *MarketCreditState
	relationsOverride     *RelationEvoState
	justiceRevoltOverride *JusticeRevoltState
	hasExplicitSubsystem  bool

	undo       []func()
	refunds    []func() []string
	cohorts    []cohortCensusSnapshot
	committed  bool
	rolledBack bool
}

type cohortCensusSnapshot struct {
	id      string
	members []Actor
}

func (tx *WorldTx) trackStoredActor(id string) {
	if id != "" {
		if c := tx.World.storedActorCohort(id); c != nil {
			tx.trackCohort(c)
		}
	}
}

func (tx *WorldTx) trackCohort(c *PopulationCohort) {
	for _, snapshot := range tx.cohorts {
		if snapshot.id == c.ID {
			return
		}
	}
	tx.cohorts = append(tx.cohorts, cohortCensusSnapshot{c.ID, append([]Actor(nil), c.Members...)})
}

// BeginTx starts a new mutation transaction bound to actorID and intent.
func (w *World) BeginTx(actorID string, intent ActionIntent) *WorldTx {
	return &WorldTx{
		World:   w,
		ActorID: actorID,
		Intent:  intent,
	}
}

func (tx *WorldTx) relationState() *RelationEvoState {
	if tx.relationsOverride != nil {
		return tx.relationsOverride
	}
	return tx.World.Relations
}

// IsRolledBack reports whether this transaction has been rolled back.
func (tx *WorldTx) IsRolledBack() bool {
	return tx != nil && tx.rolledBack
}

// IsCommitted reports whether this transaction has been committed.
func (tx *WorldTx) IsCommitted() bool {
	return tx != nil && tx.committed
}

// OnRollback registers a custom compensating rollback hook that runs in LIFO order
// if Rollback is called or if Commit settles an action_failed outcome.
func (tx *WorldTx) OnRollback(fn func()) {
	tx.pushUndo(fn)
}

func (tx *WorldTx) pushUndo(fn func()) {
	if tx != nil && fn != nil {
		tx.undo = append(tx.undo, fn)
	}
}

func (tx *WorldTx) canMutate() bool {
	return tx == nil || (!tx.committed && !tx.rolledBack)
}

// Rollback reverts every state mutation recorded in this transaction in exact LIFO
// order, restoring World and LedgerAudit to their pre-transaction state.
// Calling Rollback on a nil, already-committed, or already-rolled-back transaction
// returns false without side effects.
func (tx *WorldTx) Rollback() bool {
	if tx == nil || tx.committed || tx.rolledBack {
		return false
	}
	tx.undoMutations()
	tx.refunds = nil
	tx.cohorts = nil
	tx.rolledBack = true
	return true
}

func (tx *WorldTx) undoMutations() {
	for i := len(tx.undo) - 1; i >= 0; i-- {
		tx.undo[i]()
	}
	tx.undo = nil
	tx.Mutations = nil
	tx.Events = nil
}

func captureMapInt(m *map[string]int, key string) func() {
	wasNil := *m == nil
	prevVal, existed := (*m)[key]
	return func() {
		if wasNil {
			*m = nil
			return
		}
		if *m == nil {
			*m = map[string]int{}
		}
		if !existed {
			delete(*m, key)
			return
		}
		(*m)[key] = prevVal
	}
}

func captureMapInt64(m *map[string]int64, key string) func() {
	wasNil := *m == nil
	prevVal, existed := (*m)[key]
	return func() {
		if wasNil {
			*m = nil
			return
		}
		if *m == nil {
			*m = map[string]int64{}
		}
		if !existed {
			delete(*m, key)
			return
		}
		(*m)[key] = prevVal
	}
}

func captureLedgerCurrency(w *World, currency string) func() {
	if w == nil || w.LedgerAudit == nil {
		return nil
	}
	if currency == "" {
		currency = "coins"
	}
	audit := w.LedgerAudit
	wasNil := audit.Currencies == nil
	prev, existed := audit.Currencies[currency]
	return func() {
		if wasNil {
			audit.Currencies = nil
			return
		}
		if audit.Currencies == nil {
			audit.Currencies = map[string]ResourceLedgerTotals{}
		}
		if !existed {
			delete(audit.Currencies, currency)
			return
		}
		audit.Currencies[currency] = prev
	}
}

func captureLedgerItem(w *World, itemID string) func() {
	if w == nil || w.LedgerAudit == nil || itemID == "" {
		return nil
	}
	audit := w.LedgerAudit
	wasNil := audit.Items == nil
	prev, existed := audit.Items[itemID]
	return func() {
		if wasNil {
			audit.Items = nil
			return
		}
		if audit.Items == nil {
			audit.Items = map[string]ResourceLedgerTotals{}
		}
		if !existed {
			delete(audit.Items, itemID)
			return
		}
		audit.Items[itemID] = prev
	}
}

func (tx *WorldTx) recordMutation(rec StateMutationRecord) {
	if tx == nil {
		return
	}
	tx.Mutations = append(tx.Mutations, rec)
}

// TransferCurrency atomically transfers amount of currency ("coins" or custom)
// from `from` to `to` without net creation/destruction.
func (tx *WorldTx) TransferCurrency(from, to *Actor, currency string, amount int64, reason string) bool {
	if !tx.canMutate() || from == nil || to == nil || amount <= 0 {
		return false
	}
	if currency == "" || currency == "coins" {
		fromMoney, toMoney := from.Money, to.Money
		if int64(int(amount)) != amount || !transferActorCoinsClean(from, to, int(amount)) {
			return false
		}
		tx.pushUndo(func() {
			from.Money = fromMoney
			to.Money = toMoney
		})
		tx.recordMutation(StateMutationRecord{
			Primitive: "transfer_currency",
			ActorID:   from.ID,
			TargetID:  to.ID,
			Resource:  "coins",
			Delta:     amount,
			Detail:    reason,
		})
		return true
	}
	if from.Currencies[currency] < amount || (from != to && to.Currencies[currency] > math.MaxInt64-amount) {
		return false
	}
	undoFrom := captureMapInt64(&from.Currencies, currency)
	undoTo := captureMapInt64(&to.Currencies, currency)
	tx.World.adjustActorCurrency(from, currency, -amount, false)
	tx.World.adjustActorCurrency(to, currency, amount, false)
	tx.pushUndo(func() {
		undoTo()
		undoFrom()
	})
	tx.recordMutation(StateMutationRecord{
		Primitive: "transfer_currency",
		ActorID:   from.ID,
		TargetID:  to.ID,
		Resource:  currency,
		Delta:     amount,
		Detail:    reason,
	})
	return true
}

// MintOrBurnCurrency adjusts an actor's currency balance and records ledger conservation.
func (tx *WorldTx) MintOrBurnCurrency(a *Actor, currency string, delta int64, reason string) bool {
	if tx == nil || tx.World == nil || !tx.canMutate() || a == nil || delta == 0 {
		return false
	}
	if currency == "" {
		currency = "coins"
	}
	balance, maximum := tx.World.actorCurrencyBalance(a, currency), int64(math.MaxInt64)
	if currency == "coins" {
		maximum = int64(math.MaxInt)
	}
	if delta < -balance || delta > maximum-balance {
		return false
	}
	undoLedger := captureLedgerCurrency(tx.World, currency)
	var undoActor func()
	if currency == "coins" {
		prevMoney := a.Money
		undoActor = func() { a.Money = prevMoney }
	} else {
		undoActor = captureMapInt64(&a.Currencies, currency)
	}
	tx.World.adjustActorCurrency(a, currency, delta, true)
	if currency != "coins" && a.Currencies != nil && a.Currencies[currency] <= 0 {
		delete(a.Currencies, currency)
		if len(a.Currencies) == 0 {
			a.Currencies = nil
		}
	}
	tx.pushUndo(func() {
		undoActor()
		if undoLedger != nil {
			undoLedger()
		}
	})
	tx.recordMutation(StateMutationRecord{
		Primitive: "mint_burn_currency",
		ActorID:   a.ID,
		Resource:  currency,
		Delta:     delta,
		Detail:    reason,
	})
	return true
}

// TransferItem atomically transfers qty units of itemID from `from` to `to`,
// removing zero-valued keys to keep inventory maps clean.
func (tx *WorldTx) TransferItem(from, to *Actor, itemID string, qty int, reason string) bool {
	if !tx.canMutate() || from == nil || to == nil {
		return false
	}
	undoFrom := captureMapInt(&from.Items, itemID)
	undoTo := captureMapInt(&to.Items, itemID)
	if !transferActorItemClean(from, to, itemID, qty) {
		return false
	}
	tx.pushUndo(func() {
		undoTo()
		undoFrom()
	})
	tx.recordMutation(StateMutationRecord{
		Primitive: "transfer_item",
		ActorID:   from.ID,
		TargetID:  to.ID,
		Resource:  itemID,
		Delta:     int64(qty),
		Detail:    reason,
	})
	return true
}

// ProduceOrConsumeItem adjusts an actor's inventory with ledger conservation accounting.
func (tx *WorldTx) ProduceOrConsumeItem(a *Actor, itemID string, delta int, reason string) bool {
	if tx == nil || tx.World == nil || !tx.canMutate() || a == nil || itemID == "" || delta == 0 {
		return false
	}
	if current := a.Items[itemID]; delta < -current || delta > math.MaxInt-current {
		return false
	}
	undoItem := captureMapInt(&a.Items, itemID)
	undoLedger := captureLedgerItem(tx.World, itemID)
	tx.World.adjustActorItem(a, itemID, delta, true)
	tx.pushUndo(func() {
		undoItem()
		if undoLedger != nil {
			undoLedger()
		}
	})
	tx.recordMutation(StateMutationRecord{
		Primitive: "produce_consume_item",
		ActorID:   a.ID,
		Resource:  itemID,
		Delta:     int64(delta),
		Detail:    reason,
	})
	return true
}

func (tx *WorldTx) creditTreasury(amount int64, category string) {
	w := tx.World
	previous, ledger := w.PublicTreasury, w.TreasuryLedger
	w.creditTreasury(amount, category)
	tx.pushUndo(func() { w.PublicTreasury, w.TreasuryLedger = previous, ledger })
}

func (tx *WorldTx) debitTreasury(amount int64, category string) int64 {
	w := tx.World
	previous, ledger := w.PublicTreasury, w.TreasuryLedger
	paid := w.debitTreasury(amount, category)
	tx.pushUndo(func() { w.PublicTreasury, w.TreasuryLedger = previous, ledger })
	return paid
}

// moveCurrency records the actor side of a transfer with a treasury or cohort.
// The caller settles the named counterparty, whose balance has its own rules.
func (tx *WorldTx) moveCurrency(a *Actor, currency string, delta int64, counterparty, reason string) {
	if !tx.canMutate() || a == nil || delta == 0 {
		return
	}
	var undoActor func()
	if currency == "" || currency == "coins" {
		prevMoney := a.Money
		undoActor = func() { a.Money = prevMoney }
	} else {
		undoActor = captureMapInt64(&a.Currencies, currency)
	}
	tx.World.adjustActorCurrency(a, currency, delta, false)
	tx.pushUndo(undoActor)
	if currency == "" {
		currency = "coins"
	}
	tx.recordTransfer("transfer_currency", a.ID, counterparty, currency, delta, reason)
}

// moveItem moves items between inventory and another holding in the same ledger.
func (tx *WorldTx) moveItem(a *Actor, itemID string, delta int, counterparty, reason string) {
	if !tx.canMutate() || a == nil || delta == 0 {
		return
	}
	undoItem := captureMapInt(&a.Items, itemID)
	tx.World.adjustActorItem(a, itemID, delta, false)
	tx.pushUndo(undoItem)
	tx.recordTransfer("transfer_item", a.ID, counterparty, itemID, int64(delta), reason)
}

func (tx *WorldTx) recordTransfer(primitive, actorID, targetID, resource string, delta int64, detail string) {
	if delta == 0 {
		return
	}
	if delta > 0 {
		actorID, targetID = targetID, actorID
	} else {
		delta = -delta
	}
	tx.recordMutation(StateMutationRecord{
		Primitive: primitive, ActorID: actorID, TargetID: targetID, Resource: resource, Delta: delta, Detail: detail,
	})
}

func (tx *WorldTx) setEnergy(a *Actor, next int, reason string) {
	if !tx.canMutate() || a == nil {
		return
	}
	prev := a.Energy
	delta := next - prev
	if delta == 0 {
		return
	}
	setActorEnergy(a, next)
	tx.pushUndo(func() { setActorEnergy(a, prev) })
	tx.recordMutation(StateMutationRecord{
		Primitive: "mutate_energy", ActorID: a.ID, Resource: "energy", Delta: int64(delta), Detail: reason,
	})
}

func setActorEnergy(a *Actor, energy int) {
	a.Energy = energy
}

func (tx *WorldTx) setSentence(a *Actor, until int64) {
	previous := a.JailedUntil
	if until == previous {
		return
	}
	a.JailedUntil = until
	tx.pushUndo(func() { a.JailedUntil = previous })
	tx.recordMutation(StateMutationRecord{
		Primitive: "set_sentence", ActorID: a.ID, Delta: until - previous,
	})
}

func (tx *WorldTx) setHome(a *Actor, houseID string) {
	previous := a.HomeID
	a.HomeID = houseID
	tx.pushUndo(func() { a.HomeID = previous })
	tx.recordMutation(StateMutationRecord{
		Primitive: "set_home", ActorID: a.ID, TargetID: houseID,
	})
}

func (tx *WorldTx) setEquipped(a *Actor, slot, itemID string) {
	previous := a.Equipped[slot]
	setActorEquippedItem(a, slot, itemID)
	tx.pushUndo(func() { setActorEquippedItem(a, slot, previous) })
	tx.recordMutation(StateMutationRecord{
		Primitive: "set_equipped", ActorID: a.ID, TargetID: slot, Resource: itemID,
	})
}

func setActorEquippedItem(a *Actor, slot, itemID string) {
	if itemID == "" {
		delete(a.Equipped, slot)
		if len(a.Equipped) == 0 {
			a.Equipped = nil
		}
		return
	}
	if a.Equipped == nil {
		a.Equipped = map[string]string{}
	}
	a.Equipped[slot] = itemID
}

// MutateVitals applies clamped health and energy deltas to target and automatically
// invokes markDeadIfZero if Health reaches 0.
func (tx *WorldTx) MutateVitals(target, killer *Actor, x Action, healthDelta, energyDelta int, cause string, sourceEvents ...Event) []Event {
	if tx == nil || tx.World == nil || !tx.canMutate() || target == nil {
		return nil
	}
	if energyDelta != 0 {
		tx.setEnergy(target, maxInt(0, min(MaxEnergy, target.Energy+energyDelta)), cause)
	}
	var deathEvs []Event
	if healthDelta != 0 {
		tx.adjustHealth(target, healthDelta, 0, cause)
		if target.Health == 0 && !target.Dead {
			prevTarget := CloneActor(*target)
			var prevOffers []Offer
			if tx.World.Offers != nil {
				prevOffers = append([]Offer(nil), tx.World.Offers...)
			}
			var prevDeaths map[string]int64
			if tx.World.DeathsByCause != nil {
				prevDeaths = make(map[string]int64, len(tx.World.DeathsByCause))
				for k, v := range tx.World.DeathsByCause {
					prevDeaths[k] = v
				}
			}
			var prevHouses []House
			if tx.World.Houses != nil {
				prevHouses = make([]House, len(tx.World.Houses))
				for i, h := range tx.World.Houses {
					prevHouses[i] = CloneHouse(h)
				}
			}
			var prevCohorts []PopulationCohort
			if tx.World.Cohorts != nil {
				prevCohorts = make([]PopulationCohort, len(tx.World.Cohorts))
				for i, c := range tx.World.Cohorts {
					prevCohorts[i] = CloneCohort(c)
				}
			}
			prevFactions := CloneFactions(tx.World.Factions)
			prevProduction := CloneProductionState(tx.World.Production)
			prevRelations := CloneRelationEvoState(tx.World.Relations)
			prevPolities := ClonePolityNetwork(tx.World.Polities)
			prevPublicTreasury := tx.World.PublicTreasury
			prevTreasuryLedger := tx.World.TreasuryLedger
			prevNextID := tx.World.NextID
			prevEvLen := len(tx.Events)
			tx.pushUndo(func() {
				*target = prevTarget
				tx.World.Offers = prevOffers
				tx.World.DeathsByCause = prevDeaths
				tx.World.Houses = prevHouses
				tx.World.Cohorts = prevCohorts
				tx.World.Factions = prevFactions
				tx.World.Production = prevProduction
				tx.World.Relations = prevRelations
				tx.World.Polities = prevPolities
				tx.World.PublicTreasury = prevPublicTreasury
				tx.World.TreasuryLedger = prevTreasuryLedger
				tx.World.NextID = prevNextID
				if prevEvLen <= len(tx.Events) {
					tx.Events = tx.Events[:prevEvLen]
				}
			})
			deathEvs = tx.World.markDeadIfZeroWithTx(tx, target, killer, x, cause, sourceEvents...)
			for _, ev := range deathEvs {
				tx.EmitEvent(ev)
			}
		}
	}
	return deathEvs
}

// MoveActor updates an actor's district, venue, and indoor house state atomically.
func (tx *WorldTx) MoveActor(a *Actor, districtID, venueID, insideHouse string) {
	if !tx.canMutate() || a == nil {
		return
	}
	if districtID == "" {
		districtID = a.Location
	}
	if a.Location == districtID && a.VenueID == venueID && a.InsideHouse == insideHouse {
		return
	}
	prevLoc, prevVenue, prevHouse := a.Location, a.VenueID, a.InsideHouse
	a.Location, a.VenueID, a.InsideHouse = districtID, venueID, insideHouse
	tx.pushUndo(func() {
		a.Location, a.VenueID, a.InsideHouse = prevLoc, prevVenue, prevHouse
	})
	tx.recordMutation(StateMutationRecord{
		Primitive: "move_actor",
		ActorID:   a.ID,
		Detail:    fmt.Sprintf("district=%s venue=%s house=%s", a.Location, a.VenueID, a.InsideHouse),
	})
}

// MutateTrust adjusts from.Trust[peerID] safely (initializing nil maps).
func (tx *WorldTx) MutateTrust(from *Actor, peerID string, delta int) {
	if !tx.canMutate() || from == nil {
		return
	}
	undoTrust := captureMapInt(&from.Trust, peerID)
	if !adjustActorTrust(from, peerID, delta) {
		return
	}
	tx.pushUndo(undoTrust)
	tx.recordTrust(from.ID, peerID, delta)
}

func (tx *WorldTx) recordTrust(actorID, peerID string, delta int) {
	tx.recordMutation(StateMutationRecord{
		Primitive: "mutate_trust",
		ActorID:   actorID,
		TargetID:  peerID,
		Resource:  "trust",
		Delta:     int64(delta),
	})
}

// RecordMemory appends a memory to actor a, guaranteeing search-cache initialization.
func (tx *WorldTx) RecordMemory(a *Actor, mem Memory) {
	if !tx.canMutate() || a == nil {
		return
	}
	wasNil := a.Memories == nil
	prevLen := len(a.Memories)
	appendActorMemory(a, mem)
	tx.pushUndo(func() {
		if wasNil {
			a.Memories = nil
			return
		}
		if prevLen <= len(a.Memories) {
			a.Memories = a.Memories[:prevLen]
		}
	})
	if tx != nil && tx.World != nil {
		before := a.NextWake
		tx.World.observeForBehavior(a, mem)
		if before != a.NextWake {
			tx.pushUndo(func() {
				a.NextWake = before
				if schedule := tx.World.scheduler; schedule != nil {
					schedule.put(a.ID, before.Tick)
				}
			})
		}
	}
	tx.recordMutation(StateMutationRecord{
		Primitive: "record_memory",
		ActorID:   a.ID,
		Detail:    mem.Kind,
	})
}

func (tx *WorldTx) clearActorMemoriesAndTrust(a *Actor) {
	if !tx.canMutate() || a == nil {
		return
	}
	var prevMems []Memory
	if a.Memories != nil {
		prevMems = append([]Memory(nil), a.Memories...)
	}
	var prevTrust map[string]int
	if a.Trust != nil {
		prevTrust = make(map[string]int, len(a.Trust))
		for k, v := range a.Trust {
			prevTrust[k] = v
		}
	}
	if len(a.Memories) > 0 {
		tx.recordMutation(StateMutationRecord{Primitive: "clear_memory", ActorID: a.ID, Delta: -int64(len(a.Memories))})
	}
	for _, peerID := range sortedProductionKeys(a.Trust) {
		if delta := -a.Trust[peerID]; delta != 0 {
			tx.recordTrust(a.ID, peerID, delta)
		}
	}
	a.Memories = nil
	a.Trust = map[string]int{}
	tx.pushUndo(func() {
		a.Memories = prevMems
		a.Trust = prevTrust
	})
}

// EmitEvent appends an event into the transaction's pending event stream.
func (tx *WorldTx) EmitEvent(ev Event) {
	if tx == nil || !tx.canMutate() {
		return
	}
	tx.Events = append(tx.Events, ev)
}

// Action handlers return the complete, ordered event stream for the action.
func (tx *WorldTx) commitEvents(events []Event) ExecutionResult {
	tx.Events = events
	return tx.Commit()
}

// Commit finalizes all transaction events, rolls back any partial state mutations if
// the outcome is action_failed, runs secret capture, and returns ExecutionResult.
func (tx *WorldTx) Commit() ExecutionResult {
	if tx == nil {
		return ExecutionResult{}
	}
	if tx.rolledBack {
		return ExecutionResult{Succeeded: false, Reason: "transaction rolled back"}
	}
	for _, ev := range tx.Events {
		if ev.Kind != "action_failed" {
			continue
		}
		tx.undoMutations()
		var refundErrors []string
		for _, refund := range tx.refunds {
			refundErrors = append(refundErrors, refund()...)
		}
		if len(refundErrors) > 0 {
			ev.Payload["refund_errors"] = refundErrors
			ev.Payload["text"] = fmt.Sprintf("%s (refund failed: %s)", ev.Payload["text"], strings.Join(refundErrors, ", "))
		}
		if ev.ID == 0 {
			ev = tx.World.record(tx.World.actor(ev.ActorID), ev.Action, ev.Kind, ev.Payload)
		}
		tx.Events = []Event{ev}
		break
	}
	var energySpent int64
	var workKind string
	for _, mutation := range tx.Mutations {
		if mutation.ActorID == tx.ActorID && mutation.Primitive == "mutate_energy" && mutation.Delta < 0 {
			energySpent -= mutation.Delta
			if workKind == "" {
				workKind = mutation.Detail
			}
		}
	}
	if energySpent > 0 && tx.Intent.Action.Kind != "" {
		for i := range tx.Events {
			ev := &tx.Events[i]
			if ev.ActorID == tx.ActorID && (ev.Action.Kind == tx.Intent.Action.Kind || ev.Action.Kind == workKind) {
				if ev.Payload == nil {
					ev.Payload = make(map[string]any)
				}
				ev.Payload["energy_spent"] = energySpent
				break
			}
		}
	}
	var followups []Event
	if tx.World != nil {
		for _, ev := range tx.Events {
			tx.World.observePaymentEvidence(ev)
			followups = append(followups, tx.World.captureActionSecrets(tx, ev)...)
			followups = append(followups, tx.World.earnWage(ev)...)
		}
	}
	allEvents := append(tx.Events, followups...)
	for _, ev := range allEvents {
		tx.recordMutation(StateMutationRecord{Primitive: "emit_event", ActorID: ev.ActorID, TargetID: ev.TargetID, Detail: ev.Kind})
	}
	succeeded := true
	reason := ""
	if len(allEvents) > 0 && allEvents[0].Kind == "action_failed" {
		succeeded = false
		reason, _ = allEvents[0].Payload["text"].(string)
	}
	for _, snapshot := range tx.cohorts {
		c := tx.World.findCohortByID(snapshot.id)
		tx.World.reconcileCohortMemberChanges(c, snapshot.members)
	}
	tx.committed = true
	tx.undo = nil
	tx.refunds = nil
	tx.cohorts = nil
	return ExecutionResult{
		Succeeded: succeeded,
		Reason:    reason,
		Events:    allEvents,
		Mutations: tx.Mutations,
	}
}

// Shared nil-safe, zero-key-clean helpers used across internal/sim handlers.

func adjustActorTrust(a *Actor, peerID string, delta int) bool {
	if a == nil {
		return false
	}
	return adjustTrust(&a.Trust, peerID, delta)
}

func adjustTrust(trust *map[string]int, peerID string, delta int) bool {
	if peerID == "" || delta == 0 {
		return false
	}
	if *trust == nil {
		*trust = map[string]int{}
	}
	(*trust)[peerID] += delta
	return true
}

func transferActorItemClean(from, to *Actor, itemID string, qty int) bool {
	if from == nil || to == nil || itemID == "" || qty <= 0 {
		return false
	}
	if from.Items[itemID] < qty || (from != to && to.Items[itemID] > math.MaxInt-qty) {
		return false
	}
	if to.Items == nil {
		to.Items = map[string]int{}
	}
	from.Items[itemID] -= qty
	if from.Items[itemID] <= 0 {
		delete(from.Items, itemID)
	}
	to.Items[itemID] += qty
	return true
}

func transferActorCoinsClean(from, to *Actor, amount int) bool {
	if from == nil || to == nil || amount <= 0 {
		return false
	}
	if from.Money < amount || (from != to && to.Money > math.MaxInt-amount) {
		return false
	}
	from.Money -= amount
	to.Money += amount
	return true
}

// adjustActorHealthBounded applies a clamped health delta in [minFloor, MaxHealth]
// and returns the actual delta applied.
func adjustActorHealthBounded(a *Actor, delta, minFloor int) int {
	if a == nil || delta == 0 {
		return 0
	}
	return setActorHealthBounded(a, a.Health+delta, minFloor)
}

func setActorHealthBounded(a *Actor, health, floor int) int {
	next := min(MaxHealth, max(0, floor, health))
	applied := next - a.Health
	a.Health = next
	return applied
}

func setResidentBandHealthBounded(b *ResidentBand, health int) {
	b.Health = min(MaxHealth, max(0, health))
}

// creditPolityTax credits market/trade tax coins cleanly to pol.Treasury or public treasury.
func (w *World) creditPolityTax(pol *PolityNode, taxCoins int64) {
	if taxCoins <= 0 {
		return
	}
	if pol != nil {
		w.addPolityCurrency(pol, "coins", taxCoins)
		pol.Ledger.TaxAssessedGross += taxCoins
		pol.Ledger.TaxToTreasury += taxCoins
		return
	}
	w.creditTreasury(taxCoins, "tax")
}

// appendActorMemory appends mem to actor a while guaranteeing its search cache is initialized.
func appendActorMemory(a *Actor, mem Memory) {
	if a == nil {
		return
	}
	if mem.search == nil {
		mem.search = newMemorySearchCache(mem)
	}
	a.Memories = appendMemory(a.Memories, mem)
}

// refundTaskPrepayments returns prepaid currencies and items recorded on a multi-tick Task
// when completion fails before its effects commit, preserving all conservation ledgers.
func (w *World) refundTaskPrepayments(tx *WorldTx, a *Actor, t Task) {
	if w == nil || a == nil {
		return
	}
	tx.refunds = append(tx.refunds, func() []string {
		var failures []string
		for _, curID := range sortedProductionKeys(t.PrepaidCurrencies) {
			amount := t.PrepaidCurrencies[curID]
			if amount <= 0 {
				continue
			}
			if (curID == "" || curID == "coins") && t.PrepaidTreasury {
				if w.spendableTreasury() < amount || int64(a.Money) > int64(math.MaxInt)-amount {
					failures = append(failures, fmt.Sprintf("%d coins", amount))
					continue
				}
				w.debitTreasury(amount, "refund")
				tx.moveCurrency(a, curID, amount, "treasury", "task refund")
			} else if !tx.MintOrBurnCurrency(a, curID, amount, "task refund") {
				failures = append(failures, fmt.Sprintf("%d %s", amount, curID))
			}
		}
		for _, itemID := range sortedProductionKeys(t.PrepaidItems) {
			if qty := t.PrepaidItems[itemID]; qty > 0 && !tx.ProduceOrConsumeItem(a, itemID, qty, "task refund") {
				failures = append(failures, fmt.Sprintf("%d %s", qty, itemID))
			}
		}
		return failures
	})
}

func (tx *WorldTx) record(a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	return tx.World.recordWithTx(tx, a, x, kind, data, observers...)
}

// adjustHealth leaves death settlement to handlers that must first record the
// event that caused the injury.
func (tx *WorldTx) adjustHealth(a *Actor, delta, floor int, cause string) int {
	if !tx.canMutate() || a == nil {
		return 0
	}
	prev := a.Health
	applied := adjustActorHealthBounded(a, delta, floor)
	if applied != 0 {
		tx.pushUndo(func() { a.Health = prev })
		tx.recordHealth(a, applied, cause)
	}
	return applied
}

func (tx *WorldTx) setHealth(a *Actor, health, floor int, cause string) {
	if !tx.canMutate() || a == nil {
		return
	}
	prev := a.Health
	applied := setActorHealthBounded(a, health, floor)
	if applied != 0 {
		tx.pushUndo(func() { a.Health = prev })
		tx.recordHealth(a, applied, cause)
	}
}

func (tx *WorldTx) recordHealth(a *Actor, applied int, cause string) {
	if applied != 0 {
		tx.recordMutation(StateMutationRecord{
			Primitive: "mutate_health",
			ActorID:   a.ID,
			Resource:  "health",
			Delta:     int64(applied),
			Detail:    cause,
		})
	}
}

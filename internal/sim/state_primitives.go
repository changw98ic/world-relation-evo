package sim

import (
	"fmt"
	"math"
	"sort"
)

// SafeInt64ToInt converts an int64 to int safely without wrap-around truncation.
func SafeInt64ToInt(v int64) int {
	if v > int64(math.MaxInt) {
		return math.MaxInt
	}
	if v < int64(math.MinInt) {
		return math.MinInt
	}
	return int(v)
}

// WorldTx provides an explicit, traceable state-mutation boundary around World
// modifications. All core state mutations (currency, items, vitals, movement,
// trust, memories, and events) can be composed through WorldTx so ledger
// conservation, death checks, nil-map safety, and secret capture are enforced
// in one canonical place.
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
}

// BeginTx starts a new mutation transaction bound to actorID and intent.
func (w *World) BeginTx(actorID string, intent ActionIntent) *WorldTx {
	return &WorldTx{
		World:   w,
		ActorID: actorID,
		Intent:  intent,
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
	if from == nil || to == nil || amount <= 0 {
		return false
	}
	if currency == "" || currency == "coins" {
		if int64(int(amount)) != amount || !transferActorCoinsClean(from, to, int(amount)) {
			return false
		}
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
	if from.Currencies[currency] < amount {
		return false
	}
	tx.World.adjustActorCurrency(from, currency, -amount, false)
	tx.World.adjustActorCurrency(to, currency, amount, false)
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
	if tx == nil || tx.World == nil || a == nil || delta == 0 {
		return false
	}
	if currency == "" {
		currency = "coins"
	}
	if delta < 0 {
		if currency == "coins" {
			if int64(a.Money) < -delta {
				return false
			}
		} else if a.Currencies[currency] < -delta {
			return false
		}
	}
	tx.World.adjustActorCurrency(a, currency, delta, true)
	if currency != "coins" && a.Currencies != nil && a.Currencies[currency] <= 0 {
		delete(a.Currencies, currency)
	}
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
	if !transferActorItemClean(from, to, itemID, qty) {
		return false
	}
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
	if tx == nil || tx.World == nil || a == nil || itemID == "" || delta == 0 {
		return false
	}
	if delta < 0 && a.Items[itemID] < -delta {
		return false
	}
	tx.World.adjustActorItem(a, itemID, delta, true)
	tx.recordMutation(StateMutationRecord{
		Primitive: "produce_consume_item",
		ActorID:   a.ID,
		Resource:  itemID,
		Delta:     int64(delta),
		Detail:    reason,
	})
	return true
}

// moveCurrency records the actor side of a transfer with a treasury or cohort.
// The caller settles the named counterparty, whose balance has its own rules.
func (tx *WorldTx) moveCurrency(a *Actor, currency string, delta int64, counterparty, reason string) {
	if delta == 0 {
		return
	}
	tx.World.adjustActorCurrency(a, currency, delta, false)
	if currency == "" {
		currency = "coins"
	}
	tx.recordTransfer("transfer_currency", a.ID, counterparty, currency, delta, reason)
}

// moveItem moves items between inventory and another holding in the same ledger.
func (tx *WorldTx) moveItem(a *Actor, itemID string, delta int, counterparty, reason string) {
	if delta == 0 {
		return
	}
	tx.World.adjustActorItem(a, itemID, delta, false)
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
	delta := next - a.Energy
	if delta == 0 {
		return
	}
	a.Energy = next
	tx.recordMutation(StateMutationRecord{
		Primitive: "mutate_energy", ActorID: a.ID, Resource: "energy", Delta: int64(delta), Detail: reason,
	})
}

// MutateVitals applies clamped health and energy deltas to target and automatically
// invokes markDeadIfZero if Health reaches 0.
func (tx *WorldTx) MutateVitals(target, killer *Actor, x Action, healthDelta, energyDelta int, cause string, sourceEvents ...Event) []Event {
	if tx == nil || tx.World == nil || target == nil {
		return nil
	}
	if energyDelta != 0 {
		tx.setEnergy(target, maxInt(0, min(MaxEnergy, target.Energy+energyDelta)), cause)
	}
	var deathEvs []Event
	if healthDelta != 0 {
		tx.adjustHealth(target, healthDelta, 0, cause)
		if target.Health == 0 && !target.Dead {
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
	if a == nil {
		return
	}
	if districtID == "" {
		districtID = a.Location
	}
	if a.Location == districtID && a.VenueID == venueID && a.InsideHouse == insideHouse {
		return
	}
	a.Location, a.VenueID, a.InsideHouse = districtID, venueID, insideHouse
	tx.recordMutation(StateMutationRecord{
		Primitive: "move_actor",
		ActorID:   a.ID,
		Detail:    fmt.Sprintf("district=%s venue=%s house=%s", a.Location, a.VenueID, a.InsideHouse),
	})
}

// MutateTrust adjusts from.Trust[peerID] safely (initializing nil maps).
func (tx *WorldTx) MutateTrust(from *Actor, peerID string, delta int) {
	if !adjustActorTrust(from, peerID, delta) {
		return
	}
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
	if a == nil {
		return
	}
	appendActorMemory(a, mem)
	tx.recordMutation(StateMutationRecord{
		Primitive: "record_memory",
		ActorID:   a.ID,
		Detail:    mem.Kind,
	})
}

func (tx *WorldTx) clearActorMemoriesAndTrust(a *Actor) {
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
}

// EmitEvent appends an event into the transaction's pending event stream.
func (tx *WorldTx) EmitEvent(ev Event) {
	if tx == nil {
		return
	}
	tx.Events = append(tx.Events, ev)
}

// Action handlers return the complete, ordered event stream for the action.
func (tx *WorldTx) commitEvents(events []Event) ExecutionResult {
	tx.Events = events
	return tx.Commit()
}

// Commit finalizes all transaction events, runs secret capture, and returns ExecutionResult.
func (tx *WorldTx) Commit() ExecutionResult {
	if tx == nil {
		return ExecutionResult{}
	}
	var secretEvents []Event
	if tx.World != nil {
		for _, ev := range tx.Events {
			secretEvents = append(secretEvents, tx.World.captureActionSecrets(tx, ev)...)
		}
	}
	allEvents := append(tx.Events, secretEvents...)
	for _, ev := range allEvents {
		tx.recordMutation(StateMutationRecord{Primitive: "emit_event", ActorID: ev.ActorID, TargetID: ev.TargetID, Detail: ev.Kind})
	}
	succeeded := true
	reason := ""
	if len(allEvents) > 0 && allEvents[0].Kind == "action_failed" {
		succeeded = false
		if text, ok := allEvents[0].Payload["text"].(string); ok {
			reason = text
		} else if raw := allEvents[0].Payload["text"]; raw != nil {
			reason = fmt.Sprint(raw)
		}
	}
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
	if from.Items[itemID] < qty {
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
	if from.Money < amount {
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
		pol.Treasury += taxCoins
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
	if len(t.PrepaidCurrencies) > 0 {
		curIDs := make([]string, 0, len(t.PrepaidCurrencies))
		for k := range t.PrepaidCurrencies {
			curIDs = append(curIDs, k)
		}
		sort.Strings(curIDs)
		for _, curID := range curIDs {
			amt := t.PrepaidCurrencies[curID]
			if amt <= 0 {
				continue
			}
			if (curID == "" || curID == "coins") && t.PrepaidTreasury {
				refunded := w.debitTreasury(amt, "refund")
				if refunded > 0 {
					tx.moveCurrency(a, curID, refunded, "treasury", "task refund")
				}
			} else {
				tx.MintOrBurnCurrency(a, curID, amt, "task refund")
			}
		}
	}
	if len(t.PrepaidItems) > 0 {
		itemIDs := make([]string, 0, len(t.PrepaidItems))
		for k := range t.PrepaidItems {
			itemIDs = append(itemIDs, k)
		}
		sort.Strings(itemIDs)
		for _, itemID := range itemIDs {
			qty := t.PrepaidItems[itemID]
			if qty > 0 {
				tx.ProduceOrConsumeItem(a, itemID, qty, "task refund")
			}
		}
	}
}

func (tx *WorldTx) record(a *Actor, x Action, kind string, data map[string]any, observers ...*Actor) Event {
	return tx.World.recordWithTx(tx, a, x, kind, data, observers...)
}

// adjustHealth leaves death settlement to handlers that must first record the
// event that caused the injury.
func (tx *WorldTx) adjustHealth(a *Actor, delta, floor int, cause string) int {
	applied := adjustActorHealthBounded(a, delta, floor)
	tx.recordHealth(a, applied, cause)
	return applied
}

func (tx *WorldTx) setHealth(a *Actor, health, floor int, cause string) {
	tx.recordHealth(a, setActorHealthBounded(a, health, floor), cause)
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

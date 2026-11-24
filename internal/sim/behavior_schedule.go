package sim

import (
	"container/heap"
	"sort"
)

// ScheduledWake is the durable deadline for an actor's next behavior event.
// The heap is rebuilt from these records after restoring a snapshot.
type ScheduledWake struct {
	Tick    int64  `json:"tick"`
	Reason  string `json:"reason"`
	Version int64  `json:"version"`
}

type wakeEntry struct {
	id    string
	tick  int64
	index int
}

type wakeQueue []*wakeEntry

func (q wakeQueue) Len() int { return len(q) }
func (q wakeQueue) Less(i, j int) bool {
	if q[i].tick != q[j].tick {
		return q[i].tick < q[j].tick
	}
	return q[i].id < q[j].id
}
func (q wakeQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index, q[j].index = i, j
}
func (q *wakeQueue) Push(x any) {
	entry := x.(*wakeEntry)
	entry.index = len(*q)
	*q = append(*q, entry)
}
func (q *wakeQueue) Pop() any {
	index := len(*q) - 1
	entry := (*q)[index]
	(*q)[index] = nil
	*q = (*q)[:index]
	return entry
}

type behaviorSchedule struct {
	queue   wakeQueue
	pending map[string]*wakeEntry
	actors  map[string]int
	floor   int64
}

func (w *World) behaviorScheduler() *behaviorSchedule {
	if w.scheduler != nil && len(w.scheduler.actors) == len(w.Actors) {
		return w.scheduler
	}
	s := &behaviorSchedule{actors: make(map[string]int, len(w.Actors)), pending: make(map[string]*wakeEntry, len(w.Actors)), floor: w.Tick}
	for i := range w.Actors {
		a := &w.Actors[i]
		s.actors[a.ID] = i
		if !a.Dead {
			at := w.Tick
			if a.NextWake != nil {
				at = a.NextWake.Tick
			}
			entry := &wakeEntry{id: a.ID, tick: at, index: len(s.queue)}
			s.pending[a.ID] = entry
			s.queue = append(s.queue, entry)
		}
	}
	heap.Init(&s.queue)
	w.scheduler = s
	return s
}

func (s *behaviorSchedule) put(id string, tick int64) {
	if _, active := s.actors[id]; !active {
		return
	}
	if entry := s.pending[id]; entry != nil {
		entry.tick = tick
		heap.Fix(&s.queue, entry.index)
		return
	}
	entry := &wakeEntry{id: id, tick: tick}
	s.pending[id] = entry
	heap.Push(&s.queue, entry)
}

func (s *behaviorSchedule) due(tick int64) []int {
	var due []int
	for len(s.queue) != 0 && s.queue[0].tick <= tick {
		entry := heap.Pop(&s.queue).(*wakeEntry)
		delete(s.pending, entry.id)
		due = append(due, s.actors[entry.id])
	}
	// The world resolves actions in roster order within each logical phase.
	sort.Ints(due)
	return due
}

func (w *World) setWake(a *Actor, at int64, reason string) {
	if a.Dead {
		a.NextWake = nil
		return
	}
	if w.scheduler != nil {
		at = max(at, w.scheduler.floor)
	}
	if scheduled := a.NextWake; scheduled != nil && scheduled.Tick <= at {
		return
	}
	var version int64
	if a.Behavior != nil {
		version = a.Behavior.Version
	}
	a.NextWake = &ScheduledWake{Tick: at, Reason: reason, Version: version}
	if s := w.scheduler; s != nil {
		if _, active := s.actors[a.ID]; active {
			s.put(a.ID, at)
		}
	}
}

func (w *World) acknowledgeWake(a *Actor, action Action) {
	if a.Task != nil {
		w.setWake(a, w.Tick+1, "task_progress")
		return
	}
	if action.Kind == "wait" && action.Decision != nil {
		reason := action.Decision.Behavior.ReviewReason
		if reason == "" {
			reason = "review_due"
		}
		w.setWake(a, max(w.Tick+1, action.Decision.Behavior.ReviewAt), reason)
		return
	}
	w.setWake(a, w.Tick+1, "action_complete")
}

func (w *World) observeForBehavior(a *Actor, memory Memory) {
	if a.Dead || a.NextWake == nil {
		return
	}
	if mattersToBehavior(a.ID, memory) {
		w.setWake(a, w.Tick, "observation")
		return
	}
	if planning := w.Catalog.Planning; planning != nil {
		for _, rule := range planning.Experiences {
			if rule.Event == memory.Kind {
				w.setWake(a, w.Tick, "experience")
				return
			}
		}
	}
}

func mattersToBehavior(actorID string, memory Memory) bool {
	if memory.Trade != nil && memory.Trade.ProposerID() == actorID {
		switch memory.Kind {
		case "trade_expired", "trade_rejected", "agreement_expired", "agreement_rejected", "agreement_unfulfilled":
			return true
		}
	}
	if memory.Source == actorID {
		return false
	}
	switch memory.Kind {
	case "trade_offered", "trade_counteroffered", "agreement_proposed", "agreement_accepted", "loan_requested",
		"trade_completed", "agreement_fulfilled", "channel_message", "world_event", "arrested", "attacked":
		return true
	}
	return memory.Observation != nil || memory.Statement != nil && memory.Statement.Topic != ""
}

// NextPassiveReview returns the next change the actor can anticipate while
// waiting, using the same nutrition and physiology boundaries as settlement.
func (p Perception) NextPassiveReview(horizon int64) ScheduledWake {
	tick := p.Tick
	next := ScheduledWake{Tick: tick + horizon, Reason: "review_due"}
	consider := func(at int64, reason string) {
		if at < next.Tick {
			next.Tick, next.Reason = at, reason
		}
	}
	sixHours := int64(HydrationIntervalTicks)
	consider((tick/sixHours+1)*sixHours, "hydration_due")
	if p.Self.RestUntil > tick {
		consider(p.Self.RestUntil, "recovery_complete")
	}
	for _, need := range p.Catalog.EffectivePlanningRules().Needs {
		if need.Minimize {
			continue
		}
		switch need.Measure {
		case "energy":
			if need.Subject == "" {
				consider(p.Self.NextSleepWindowChange(tick), "sleep_window_changes")
			}
			energy := float64(p.Self.Energy)
			if need.Subject == "" && need.Target > 0 && energy >= need.Target {
				points := int64(energy-need.Target) + 1
				ticks := points*AwakeEnergyIntervalTicks - p.Self.AwakeEnergyRemainder
				consider(tick+ticks, "energy_need")
			}
		case "state_slot":
			slot := p.Catalog.StateSlots[need.Subject]
			value, exists := p.Catalog.StateSlotValue(p.Self, need.Subject)
			if rate := p.StateSlotChangePerTick(need.Subject); exists && rate < 0 && float64(value) >= need.Target && float64(slot.Min) < need.Target {
				consider(tick+int64((float64(value)-need.Target)/float64(-rate))+1, "state_need")
			}
		}
	}
	form := p.Catalog.Forms[p.Self.FormID]
	if !form.IsHungerDisabled() {
		numerator, divisor := nutritionRate(form)
		if numerator > 0 {
			consumed := tick*numerator/divisor + 1
			boundary := (consumed*divisor + numerator - 1) / numerator
			if boundary > tick {
				consider(boundary, "meal_due")
			}
		}
	}
	for _, offer := range p.Offers {
		if offer.Expires >= tick {
			consider(offer.Expires+1, "quote_response_due")
		}
	}
	for _, wage := range p.PendingWages {
		if wage.DueTick > tick {
			consider(wage.DueTick, "payday")
		}
	}
	for _, gap := range p.Continuity.Agreements {
		switch gap.Status {
		case "proposed":
			if gap.ResponseBy >= tick {
				consider(gap.ResponseBy+1, "agreement_response_due")
			}
		case "active":
			if gap.DueTick >= tick {
				consider(gap.DueTick+1, "delivery_due")
			}
		}
	}
	for _, event := range p.ActiveWorldEvents {
		if event.ExpiresTick > tick {
			consider(event.ExpiresTick, "local_event_changes")
		}
	}
	return next
}

func (p Perception) PlanCoversImmediateRisks(goal NeedKey) bool {
	if dehydrationDamage(p.Self.Thirst) > 0 && goal.Measure != "hydration" {
		return false
	}
	return starvationDamage(p.Self.Hunger, p.Catalog.Lifecycle, p.Catalog.Forms[p.Self.FormID]) == 0 ||
		goal.Measure == "nutrition" || goal.Measure == "provisions"
}

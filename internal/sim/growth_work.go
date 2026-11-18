package sim

// WorkerRun names a contiguous set of residents without expanding large cohorts.
// Actor work has a single ID. Resident runs also retain the identity coordinates.
type WorkerRun struct {
	FirstID string `json:"first_id"`
	Origin  string `json:"origin,omitempty"`
	First   int    `json:"first,omitempty"`
	Count   int    `json:"count"`
}

// A worker keeps their identity if residents move or their physiology bands
// split between assignments. Subtract occupied intervals by origin and ordinal.
func (c *PopulationCohort) workerRuns(used []WorkerRun, limit int) ([]WorkerRun, int) {
	var runs []WorkerRun
	left := limit
	for _, band := range c.Residents {
		if band.Sick || band.Jailed {
			continue
		}
		for first, end := band.First, band.First+band.Count; first < end && left > 0; {
			next, skip := end, first
			for _, busy := range used {
				if busy.Origin != band.Origin || busy.First+busy.Count <= first {
					continue
				}
				if busy.First <= first {
					skip = max(skip, busy.First+busy.Count)
				} else {
					next = min(next, busy.First)
				}
			}
			if skip > first {
				first = skip
				continue
			}
			n := min(left, next-first)
			runs = append(runs, WorkerRun{FirstID: band.ID(first - band.First), Origin: band.Origin, First: first, Count: n})
			left -= n
			first += n
		}
		if left == 0 {
			break
		}
	}
	return runs, limit - left
}

func (w *World) completeGrowthWork(g, caller GrowthBatch, stage string, need int64) (int64, []Event) {
	count := min(need, w.productionFreeWorkers(caller))
	if count == 0 {
		return 0, nil
	}
	var runs []WorkerRun
	if caller.Cohort {
		c := w.findCohort(caller.OwnerID)
		c.ensureResidents(w.anonymousCohort(*c).Population)
		var used []WorkerRun
		if w.Production.LaborTick == w.Tick {
			used = w.Production.WorkerRoster
		}
		var actual int
		runs, actual = c.workerRuns(used, int(count))
		count = int64(actual)
		if count == 0 || ((stage == "prepare" || stage == "production") && count < need) {
			return 0, nil
		}
	} else {
		runs = []WorkerRun{{FirstID: caller.OwnerID, Count: 1}}
	}
	w.reserveProductionWorkers(caller, count)
	w.Production.WorkerRoster = append(w.Production.WorkerRoster, runs...)
	e := w.growthEvent(g, "growth_work_completed", map[string]any{"worker_holder_id": caller.OwnerID, "worker_holder_kind": holderKind(caller.Cohort), "stage": stage, "workers": runs, "work_units": count, "stage_complete": count == need, "district": g.District, "field_id": g.FieldID})
	return count, []Event{e}
}

// Convert this batch's standing reservation into a settled shift. A partially
// staffed batch remains pending; other batches keep their reserved capacity.
func (w *World) completeReservedShift(g *GrowthBatch, stage string) []Event {
	caller := w.growthCaretaker(*g)
	previous := g.LaborBookedTick
	g.LaborBookedTick = w.Tick + 1
	if w.productionFreeWorkers(caller) < int64(g.Workers) {
		g.LaborBookedTick = previous
		return nil
	}
	_, events := w.completeGrowthWork(*g, caller, stage, int64(g.Workers))
	if len(events) == 0 {
		g.LaborBookedTick = previous
	} else if stage == "prepare" {
		events[0].Payload["stage_complete"] = g.PreparationRemaining == 1
	}
	return events
}

func (g GrowthBatch) harvestSource() int64 {
	if g.SourceGrowthID != 0 {
		return g.SourceGrowthID
	}
	return g.ID
}

func (g GrowthBatch) harvestBasis() string {
	if g.SourceGrowthID != 0 {
		return "care_agreement"
	}
	return "property"
}

func (w *World) harvestAllocation(g, recipient GrowthBatch, kind, basis string, qty int64) Event {
	return w.growthEvent(g, kind, map[string]any{"source_growth_id": g.harvestSource(), "recipient_kind": holderKind(recipient.Cohort), "recipient_id": recipient.OwnerID, "basis": basis, "item": g.Item, "quantity": qty})
}

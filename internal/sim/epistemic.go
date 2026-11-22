package sim

// CanObserveActor is the canonical epistemic visibility gate between two actors.
// An observer can perceive a target directly only when both are alive, the observer
// is awake and not jailed/in-transit, and both share the exact same place
// (district, venue, and indoor house).
func CanObserveActor(observer, target Actor, tick int64) bool {
	if observer.Dead || target.Dead || observer.ID == target.ID {
		return false
	}
	if tick < observer.JailedUntil || observer.InTransit() {
		return false
	}
	if observer.Task != nil && (observer.Task.Action.Kind == "sleep" || observer.Task.Dormant) {
		return false
	}
	return SamePlace(observer, target)
}

// sanitizeDistrictStateForActor projects DistrictState for Perception, stripping
// internal simulation/hydraulic/patrol schedules for all actors and hiding covert
// enforcement internals (BribeShieldTicks, Corruption, RacketPressure) from
// ordinary actors without enforcement or oversight standing.
func (w *World) sanitizeDistrictStateForActor(a Actor, ds DistrictState) DistrictState {
	out := CloneDistrictState(ds)
	out.Patrols = nil
	out.Recruitment = nil
	out.WaterOverflow = nil
	out.WaterRegenRemainder = nil
	out.WaterFlowRemainder = nil

	tags := a.IdentityTags()
	hasOfficialAccess := w.Catalog.IsEnforcementTags(tags) || w.Catalog.IsOversightTags(tags) || w.ActorCapabilities(a)["frame_suspect"]
	if !hasOfficialAccess {
		out.BribeShieldTicks = 0
		out.Corruption = 0
		out.RacketPressure = 0
	}
	return out
}

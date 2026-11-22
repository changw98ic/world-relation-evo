package agent

// SetLastTriggerEventID records the causal trigger event ID for an actor's chosen Jev option.
func (s *RulebookSelector) SetLastTriggerEventID(actorID string, eventID int64) {
	if s == nil || actorID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastTriggerByActor == nil {
		s.lastTriggerByActor = map[string]int64{}
	}
	if eventID > 0 {
		s.lastTriggerByActor[actorID] = eventID
	} else {
		delete(s.lastTriggerByActor, actorID)
	}
}

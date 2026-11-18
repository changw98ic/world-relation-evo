package sim

// mediationActions exposes only known feuds whose parties can talk here.
// Accepting an existing proposal uses exactly the amount already offered.
func (w *World) mediationActions(a Actor) []Action {
	if w.Relations == nil {
		return nil
	}
	var actions []Action
	for _, g := range w.Relations.Grudges {
		if g.Resolved || !actorKnowsAboutGrudge(a, g) {
			continue
		}
		partiesPresent := true
		for _, id := range []string{g.OffenderID, g.AggrievedID} {
			party := w.actor(id)
			if party == nil || party.Dead || w.Tick < party.JailedUntil || !SamePlace(a, *party) || party.Task != nil && (party.Task.Action.Kind == "sleep" || party.Task.Dormant) {
				partiesPresent = false
				break
			}
		}
		if !partiesPresent {
			continue
		}
		amount := int64(maxInt(2, g.Intensity/5))
		if g.MediationProposerID != "" {
			amount = g.MediationCoins
			switch a.ID {
			case g.OffenderID:
				if g.OffenderAccepted && !g.AggrievedAccepted { continue }
			case g.AggrievedID:
				if g.AggrievedAccepted && !g.OffenderAccepted { continue }
			default:
				continue
			}
		}
		actions = append(actions, Action{Kind: "mediate_feud", Text: g.ID, TargetID: g.OffenderID, Price: int(amount)})
	}
	return actions
}

package agent

import "github.com/changw98ic/world-relation-evo/internal/sim"

type modelContext struct {
	sim.Perception
	OmittedDialogueTextEventIDs []int64 `json:"omitted_dialogue_text_event_ids,omitempty"`
	ArchivedClosedAgreements    int     `json:"archived_closed_agreement_count,omitempty"`
}

// Send the latest utterance in each directed conversation, plus the recalled
// history's structured claims, observations, event IDs and provenance. Replaying
// every rephrasing of a long conversation caused the actual Jev context failure.
// This is a request projection: archived memories and trace inputs stay intact.
func contextForModel(p sim.Perception) modelContext {
	p.Self.Memories = append([]sim.Memory(nil), p.Self.Memories...)
	result := modelContext{Perception: p}
	result.Continuity.Dialogue = append([]sim.DialogueTurn(nil), p.Continuity.Dialogue...)
	type conversation struct{ source, target, channel string }
	latest := map[conversation]int{}
	textPresent := map[int64]bool{}
	for i, memory := range p.Self.Memories {
		if memory.Kind == "speech" {
			latest[conversation{memory.Source, memory.TargetID, memory.Channel}] = i
		}
	}
	for i := range result.Self.Memories {
		memory := &result.Self.Memories[i]
		if memory.Kind == "speech" && latest[conversation{memory.Source, memory.TargetID, memory.Channel}] != i {
			memory.Text = ""
			result.OmittedDialogueTextEventIDs = append(result.OmittedDialogueTextEventIDs, memory.EventID)
		}
		if memory.Text != "" {
			textPresent[memory.EventID] = true
		}
	}
	latestTurn := map[conversation]int{}
	for i, turn := range result.Continuity.Dialogue {
		latestTurn[conversation{turn.SpeakerID, turn.ListenerID, turn.Kind}] = i
	}
	for i := range result.Continuity.Dialogue {
		turn := &result.Continuity.Dialogue[i]
		if textPresent[turn.EventID] {
			turn.Text = ""
		} else if latestTurn[conversation{turn.SpeakerID, turn.ListenerID, turn.Kind}] != i && turn.Text != "" {
			turn.Text = ""
			result.OmittedDialogueTextEventIDs = append(result.OmittedDialogueTextEventIDs, turn.EventID)
		}
	}
	// Open obligations remain complete. For concluded dealings, retain the
	// latest outcome per counterparty and item; earlier settlements remain in
	// the persistent world archive rather than growing every future request.
	type dealings struct{ supplier, buyer, item string }
	lastClosed := map[dealings]int{}
	for i, agreement := range p.Continuity.Agreements {
		if agreement.Status != "proposed" && agreement.Status != "active" {
			key := dealings{agreement.SupplierID, agreement.BuyerID, agreement.Item}
			previous, found := lastClosed[key]
			if !found || agreement.ClosedTick >= p.Continuity.Agreements[previous].ClosedTick {
				lastClosed[key] = i
			}
		}
	}
	result.Continuity.Agreements = nil
	for i, agreement := range p.Continuity.Agreements {
		if agreement.Status == "proposed" || agreement.Status == "active" || lastClosed[dealings{agreement.SupplierID, agreement.BuyerID, agreement.Item}] == i {
			result.Continuity.Agreements = append(result.Continuity.Agreements, agreement)
		} else {
			result.ArchivedClosedAgreements++
		}
	}
	return result
}

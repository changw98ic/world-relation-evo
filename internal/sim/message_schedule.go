package sim

import (
	"slices"
	"sort"
)

// ScheduledMessage is a transmission whose recipients have yet to receive it.
type ScheduledMessage struct {
	DueTick    int64    `json:"due_tick"`
	ID         int64    `json:"id"`
	Message    Memory   `json:"message"`
	Recipients []string `json:"recipients"`
	Action     Action   `json:"action"`
}

func (w *World) scheduleMessage(tx *WorldTx, message ScheduledMessage) {
	i := sort.Search(len(w.PendingMessages), func(i int) bool {
		return w.PendingMessages[i].DueTick > message.DueTick
	})
	w.PendingMessages = slices.Insert(w.PendingMessages, i, message)
	tx.pushUndo(func() {
		w.PendingMessages = slices.Delete(w.PendingMessages, i, i+1)
		if len(w.PendingMessages) == 0 {
			w.PendingMessages = nil
		}
	})
}

func (w *World) deliverScheduledMessages() []Event {
	var events []Event
	last := 0
	for _, message := range w.PendingMessages {
		if message.DueTick > w.Tick {
			break
		}
		last++
		tx := w.BeginTx(message.Message.Source, ActionIntent{Action: message.Action})
		var received []string
		var recipients []*Actor
		eventID := w.NextID
		w.NextID++
		for _, id := range message.Recipients {
			a := w.actor(id)
			if a == nil || a.Dead {
				continue
			}
			memory := message.Message
			memory.EventID, memory.Tick, memory.TargetID = eventID, w.Tick, id
			tx.trackStoredActor(id)
			tx.RecordMemory(a, memory)
			recipients = append(recipients, a)
			received = append(received, id)
		}
		event := Event{ID: eventID, Tick: w.Tick, Kind: "channel_delivered", ActorID: message.Message.Source,
			TargetID: message.Action.TargetID, Action: message.Action, Payload: map[string]any{
				"cause_event_id": message.ID, "root_event_id": message.Message.RootEventID,
				"fact_event_id": message.Message.FactEventID, "witness_id": message.Message.WitnessID,
				"delivered_to": received, "delivered_count": len(received),
			}}
		events = append(events, tx.commitEvents([]Event{event}).Events...)
		events = append(events, w.deliverSecretReport(message.Action, message.Message.FactEventID, recipients)...)
	}
	if last == len(w.PendingMessages) {
		w.PendingMessages = nil
	} else if last > 0 {
		w.PendingMessages = slices.Delete(w.PendingMessages, 0, last)
	}
	return events
}

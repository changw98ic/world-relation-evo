package sim

const (
	ConversationQuestion = "conversation_question"
	ConversationReply    = "conversation_reply"
)

// OpenQuestionWith uses the last personal exchange, so a reply closes the
// question for both people. A bystander cannot answer on a participant's behalf.
func (a Actor) OpenQuestionWith(peer string) (Memory, bool) {
	for i := len(a.Memories) - 1; i >= 0; i-- {
		m := a.Memories[i]
		pair := m.Source == a.ID && m.TargetID == peer || m.Source == peer && m.TargetID == a.ID
		if m.Kind == "speech" && pair {
			return m, m.Statement != nil && m.Statement.Topic == ConversationQuestion
		}
	}
	return Memory{}, false
}

func (a Actor) openQuestions() map[string]Memory {
	var questions map[string]Memory
	for _, turn := range a.Memories {
		if turn.Kind != "speech" {
			continue
		}
		var peer string
		switch {
		case turn.Source == a.ID:
			peer = turn.TargetID
		case turn.TargetID == a.ID:
			peer = turn.Source
		default:
			continue
		}
		if turn.Statement != nil && turn.Statement.Topic == ConversationQuestion {
			if questions == nil {
				questions = make(map[string]Memory)
			}
			questions[peer] = turn
		} else {
			delete(questions, peer)
		}
	}
	return questions
}

func (c Catalog) HasConversationNeeds() bool {
	for _, slot := range c.StateSlots {
		if slot.ConversationGain > 0 {
			return true
		}
	}
	return false
}

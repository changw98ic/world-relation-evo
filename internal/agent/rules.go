package agent

import (
	"context"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// Rules is an offline baseline, not a claim of LLM-driven character intelligence.
type Rules struct{}

func (Rules) Decide(_ context.Context, p sim.Perception) (sim.Action, error) {
	a := p.Self
	if a.Hunger > 0 && a.Food > 0 {
		return sim.Action{Kind: "eat"}, nil
	}
	for _, o := range p.Offers {
		if o.Buyer == a.ID {
			kind := "reject"
			if a.Food < 8 && a.Money >= o.Price && a.Trust[o.Seller] >= 0 {
				kind = "accept"
			}
			return sim.Action{Kind: kind, OfferID: o.ID}, nil
		}
	}
	hour := p.Tick % 24
	if hour < 6 {
		return sim.Action{Kind: "wait"}, nil
	}
	if action, ok := informationAction(p); ok && (action.Kind == "move" || p.Tick%3 == 0) {
		return action, nil
	}
	if a.Food < 3 || a.Role == "farmer" && a.Food < 12 {
		if a.Location != "fields" {
			return sim.Action{Kind: "move", Location: "fields"}, nil
		}
		return sim.Action{Kind: "work"}, nil
	}
	if a.Location != "market" {
		return sim.Action{Kind: "move", Location: "market"}, nil
	}
	if len(p.Nearby) > 0 {
		for _, b := range p.Nearby {
			if a.Food > 7 && b.Role != "farmer" {
				pending := false
				for _, o := range p.Offers {
					if o.Seller == a.ID && o.Buyer == b.ID {
						pending = true
					}
				}
				if !pending {
					return sim.Action{Kind: "offer", TargetID: b.ID, Food: 2, Price: 2}, nil
				}
			}
		}
		b := p.Nearby[int(p.Tick)%len(p.Nearby)]
		msg := fmt.Sprintf("I have %d food and am trying to %s.", a.Food, a.Goal)
		return sim.Action{Kind: "speak", TargetID: b.ID, Text: msg}, nil
	}
	return sim.Action{Kind: "wait"}, nil
}

// This explicit baseline policy reads reported food opportunities. It is not a
// natural-language parser: structured statements remain claims, not observations.
func informationAction(p sim.Perception) (sim.Action, bool) {
	a := p.Self
	for i := len(a.Memories) - 1; i >= 0; i-- {
		m := a.Memories[i]
		if p.Tick-m.Tick > 24 {
			continue
		}
		var claim sim.Statement
		if m.Observation != nil && m.Observation.Kind == "food_produced" {
			claim = sim.Statement{Topic: "food_produced", Location: m.Observation.Location, Quantity: m.Observation.Quantity}
		} else if m.Kind == "speech" && m.Source != a.ID && m.Statement != nil && m.Statement.Topic == "food_produced" {
			claim = *m.Statement
		} else {
			continue
		}
		if m.Kind == "speech" && a.Food < 6 && a.Trust[m.Source] >= 0 && claim.Location == "fields" && a.Location != "fields" {
			return sim.Action{Kind: "move", Location: claim.Location, BasisEventID: m.EventID}, true
		}
		for _, b := range p.Nearby {
			if b.ID == m.Source {
				continue
			}
			told := false
			for _, past := range a.Memories {
				if past.Kind == "speech" && past.Source == a.ID && past.TargetID == b.ID && past.RootEventID == m.RootEventID {
					told = true
					break
				}
			}
			if told {
				continue
			}
			text := fmt.Sprintf("I produced %d food at %s.", claim.Quantity, claim.Location)
			if m.Kind == "speech" {
				text = fmt.Sprintf("%s told me: %s", m.Source, m.Text)
			}
			return sim.Action{Kind: "speak", TargetID: b.ID, Text: text, Statement: &claim, BasisEventID: m.EventID}, true
		}
	}
	return sim.Action{}, false
}

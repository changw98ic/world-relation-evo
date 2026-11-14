package agent

import (
	"context"
	"fmt"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type Option struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Action sim.Action `json:"action"`
}
type Selector interface {
	Select(context.Context, sim.Perception, []Option) (string, error)
}
type Content struct {
	Text         string         `json:"text"`
	Statement    *sim.Statement `json:"statement,omitempty"`
	BasisEventID int64          `json:"basis_event_id,omitempty"`
}
type Generator interface {
	Generate(context.Context, sim.Perception, sim.Action) (Content, error)
}
type Pipeline struct {
	Selector  Selector
	Generator Generator
}

func (p Pipeline) Decide(ctx context.Context, view sim.Perception) (sim.Action, error) {
	options := Options(view)
	if len(options) == 0 {
		return sim.Action{}, fmt.Errorf("actor %s has no actions granted by tags", view.Self.ID)
	}
	id, err := p.Selector.Select(ctx, view, options)
	if err != nil {
		return sim.Action{}, err
	}
	var selected *sim.Action
	for _, o := range options {
		if o.ID == id {
			v := o.Action
			selected = &v
			break
		}
	}
	if selected == nil {
		return sim.Action{}, fmt.Errorf("selector returned unavailable option %q", id)
	}
	if selected.Kind == "wait" {
		return *selected, nil
	}
	content, err := p.Generator.Generate(ctx, view, *selected)
	if err != nil {
		return sim.Action{}, err
	}
	// Content cannot replace the selected behavior, target, route, or resource terms.
	selected.Text = content.Text
	if selected.Kind == "speak" {
		selected.Statement = content.Statement
		selected.BasisEventID = content.BasisEventID
	}
	return *selected, nil
}
func Options(p sim.Perception) []Option {
	a := p.Self
	allowed := sim.Capabilities(a)
	var options []Option
	add := func(label string, action sim.Action) {
		if allowed[action.Kind] {
			options = append(options, Option{ID: fmt.Sprintf("o%03d", len(options)), Label: label, Action: action})
		}
	}
	add("Wait", sim.Action{Kind: "wait"})
	for _, l := range p.Locations {
		if l != a.Location {
			add("Move to "+l, sim.Action{Kind: "move", Location: l})
		}
	}
	if a.Food > 0 && a.Hunger > 0 {
		add("Eat one food", sim.Action{Kind: "eat"})
	}
	if a.Location == "fields" {
		add("Produce food through your work", sim.Action{Kind: "work"})
	}
	for _, b := range p.Nearby {
		add("Talk to "+b.Name+" ("+b.ID+")", sim.Action{Kind: "speak", TargetID: b.ID})
		if a.Food >= 2 {
			pending := false
			for _, o := range p.Offers {
				if o.Seller == a.ID && o.Buyer == b.ID {
					pending = true
				}
			}
			if !pending {
				add("Offer to sell 2 food to "+b.Name+" for 2 coins", sim.Action{Kind: "offer", TargetID: b.ID, Food: 2, Price: 2})
			}
		}
		if b.Health > 0 {
			add("Attack "+b.Name, sim.Action{Kind: "attack", TargetID: b.ID})
		}
		if b.Health < 100 && a.Medicine > 0 {
			add("Treat "+b.Name, sim.Action{Kind: "heal", TargetID: b.ID})
		}
	}
	for _, o := range p.Offers {
		if o.Buyer != a.ID {
			continue
		}
		if a.Money >= o.Price {
			for _, b := range p.Nearby {
				if b.ID == o.Seller {
					add(fmt.Sprintf("Buy from %s: accept offer %d", b.Name, o.ID), sim.Action{Kind: "accept", TargetID: o.Seller, OfferID: o.ID})
				}
			}
		}
		add(fmt.Sprintf("Reject offer %d", o.ID), sim.Action{Kind: "reject", TargetID: o.Seller, OfferID: o.ID})
	}
	return options
}

// RuleSelector is explicitly the offline baseline, never a Jev substitute.
type RuleSelector struct{}

func (RuleSelector) Select(ctx context.Context, p sim.Perception, options []Option) (string, error) {
	desired, err := (rulesPolicy{}).Decide(ctx, p)
	if err != nil {
		return "", err
	}
	for _, o := range options {
		a := o.Action
		if a.Kind == desired.Kind && a.Location == desired.Location && a.OfferID == desired.OfferID && (a.TargetID == desired.TargetID || a.OfferID != 0) {
			return o.ID, nil
		}
	}
	for _, o := range options {
		if o.Action.Kind == "wait" {
			return o.ID, nil
		}
	}
	return "", fmt.Errorf("no matching offline choice")
}

type TemplateContent struct{}

func (TemplateContent) Generate(_ context.Context, p sim.Perception, a sim.Action) (Content, error) {
	if a.Kind != "speak" {
		return Content{}, nil
	}
	// The content module selects a known source and wording after the interlocutor is fixed.
	for i := len(p.Self.Memories) - 1; i >= 0; i-- {
		m := p.Self.Memories[i]
		if p.Tick-m.Tick > 24 {
			continue
		}
		if m.Observation != nil && m.Observation.Kind == "food_produced" {
			return Content{Text: fmt.Sprintf("I produced %d food at %s.", m.Observation.Quantity, m.Observation.Location), BasisEventID: m.EventID, Statement: &sim.Statement{Topic: "food_produced", Location: m.Observation.Location, Quantity: m.Observation.Quantity}}, nil
		}
		if m.Kind == "speech" && m.Source != p.Self.ID && m.Source != a.TargetID && m.Statement != nil {
			v := *m.Statement
			return Content{Text: fmt.Sprintf("%s told me: %s", m.Source, m.Text), BasisEventID: m.EventID, Statement: &v}, nil
		}
	}
	return Content{Text: fmt.Sprintf("I have %d food and am trying to %s.", p.Self.Food, p.Self.Goal)}, nil
}

func Offline() Pipeline { return Pipeline{Selector: RuleSelector{}, Generator: TemplateContent{}} }

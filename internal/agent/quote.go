package agent

import (
	"context"
	"fmt"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

type QuoteGenerator interface {
	Quote(context.Context, sim.Perception, sim.Action) (sim.TradeTerms, error)
}

func withQuoteOptions(p sim.Perception, options []Option) []Option {
	add := func(draft sim.Action, label string) {
		options = append(options, Option{ID: fmt.Sprintf("q%03d", len(options)), Label: label, Action: draft, PrepareQuote: true})
	}
	for _, o := range options {
		if o.Action.Kind == "offer" && o.Action.OfferID == 0 {
			add(o.Action, "Draft different quantity, total price or currency for: "+o.Label)
		}
	}
	if !p.Capabilities()["offer"] {
		return options
	}
	rules := sim.World{Catalog: p.Catalog, WorldLaws: p.WorldLaws}
	for _, o := range p.Offers {
		if o.ResponderID() != p.Self.ID || o.Expires < p.Tick || o.Buyer == p.Self.ID && !p.Capabilities()["accept"] {
			continue
		}
		other := o.Counterparty(p.Self.ID)
		for _, n := range p.Nearby {
			if n.ID != other || n.Jailed || n.Sleeping {
				continue
			}
			draft := sim.Action{Kind: "offer", OfferID: o.ID, TargetID: other, Item: o.Item, Quantity: o.Quantity, Price: o.Price, Currency: o.Currency}
			if blocked, _ := rules.IsActionBlockedByWorldLaw(p.Self, draft); blocked {
				continue
			}
			add(draft, fmt.Sprintf("Draft a counteroffer to %s for offer %d: buyer remains %s, seller remains %s; replaces this offer only if you confirm", other, o.ID, o.Buyer, o.Seller))
		}
	}
	return options
}

func (p Pipeline) resolveQuote(ctx context.Context, view sim.Perception, selector Selector, optionID string, draft sim.Action, original []Option) (sim.Action, string, error) {
	generator, ok := p.Generator.(QuoteGenerator)
	if !ok {
		return sim.Action{}, "", fmt.Errorf("selected quote requires a quote generator")
	}
	if err := p.Trace.Record("quote_selected", view.Tick, view.Self.ID, map[string]any{"option_id": optionID, "action": draft}); err != nil {
		return sim.Action{}, "", err
	}
	terms, err := generator.Quote(ctx, view, draft)
	if err != nil {
		return sim.Action{}, "", err
	}
	proposal, err := sim.QuoteAction(view, draft, terms)
	if err != nil {
		return sim.Action{}, "", err
	}
	choices := []Option{{ID: "quote-confirm", Label: fmt.Sprintf("Submit explicit proposal to %s: %d %s for %d %s; no goods or payment move yet", proposal.TargetID, proposal.Quantity, proposal.Item, proposal.Price, terms.Currency), Action: proposal}}
	// Keep every ordinary alternative: confirmation must not force a trade or a wait.
	for _, o := range original {
		if !o.PrepareQuote {
			choices = append(choices, o)
		}
	}
	if err = p.Trace.Record("quote_options", view.Tick, view.Self.ID, map[string]any{"draft_option_id": optionID, "terms": terms, "options": choices}); err != nil {
		return sim.Action{}, "", err
	}
	id, err := selector.Select(ctx, view, choices)
	if err != nil {
		return sim.Action{}, "", err
	}
	for _, o := range choices {
		if o.ID == id {
			return o.Action, id, nil
		}
	}
	return sim.Action{}, "", fmt.Errorf("quote confirmation returned unavailable option %q", id)
}

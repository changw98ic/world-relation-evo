package agent

import (
	"context"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestOfferCandidatesUseCurrentDeliverableStock(t *testing.T) {
	for _, stock := range []int{0, 1, 2} {
		w := boundaryWorld()
		w.Actors[0].Items["food"] = stock
		w.Actors[1].Money = 0 // A free transfer is a valid boundary.
		w.Offers = []sim.Offer{{ID: 77, Seller: "alice", Buyer: "bob", Item: "food", Quantity: 2, Price: 0, Expires: w.Tick + 12}}
		check := func(view sim.Perception) {
			t.Helper()
			accept, reject := false, false
			for _, option := range Options(view) {
				if option.Action.OfferID == 77 {
					accept = accept || option.Action.Kind == "accept"
					reject = reject || option.Action.Kind == "reject"
				}
			}
			if accept != (stock == 2) || !reject {
				t.Fatalf("stock=%d: accept=%v reject=%v; two units must actually be deliverable", stock, accept, reject)
			}
		}
		check(w.View("bob"))
		// Step uses the indexed perception path, which must expose the same facts.
		pipeline := Pipeline{Selector: boundarySelector(func(_ context.Context, p sim.Perception, _ []Option) (string, error) {
			if p.Self.ID == "bob" {
				check(p)
			}
			for _, option := range Options(p) {
				if option.Action.Kind == "wait" {
					return option.ID, nil
				}
			}
			panic("fixture has no wait action")
		})}
		if _, err := w.Step(context.Background(), pipeline); err != nil {
			t.Fatal(err)
		}
		if stock == 2 {
			boundaryApply(t, w, "bob", sim.Action{Kind: "accept", OfferID: 77})
			if w.Actors[0].Items["food"] != 0 || w.Actors[1].Items["food"] != 2 {
				t.Fatal("exact stock free transfer did not deliver two units")
			}
		}
	}
}

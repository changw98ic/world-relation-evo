package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/changw98ic/world-relation-evo/internal/sim"
)

func TestPipelineUsesOnlySelectedSystemTerms(t *testing.T) {
	w := boundaryWorld()
	view := w.View("alice")
	var chosen sim.Action
	p := Pipeline{
		Selector: boundarySelector(func(_ context.Context, _ sim.Perception, options []Option) (string, error) {
			for _, o := range options {
				if o.Action.Kind == "offer" && o.Action.TargetID == "bob" {
					chosen = o.Action
					return o.ID, nil
				}
			}
			t.Fatal("fixture offer unavailable")
			return "", nil
		}),
	}
	before := boundaryJSON(t, w)
	action, err := p.Decide(context.Background(), view)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(action, chosen) || boundaryJSON(t, w) != before {
		t.Fatal("selection changed the world or trade terms")
	}
	boundaryApply(t, w, "alice", action)
	if w.Actors[0].Money != 50 || w.Actors[1].Money != 50 || w.Actors[0].Items["food"] != 20 || w.Actors[1].Items["food"] != 0 {
		t.Fatal("a unilateral offer transferred assets")
	}
}

func TestPipelineWordsAndAssertionsHaveNoAssetAuthority(t *testing.T) {
	w := boundaryWorld()
	w.Houses = []sim.House{{ID: "home", District: "meeting", OwnerID: "alice", Residents: []string{"alice"}, Level: 1, Condition: 100}}
	before := boundaryJSON(t, w)
	action, err := (Pipeline{Selector: boundaryChoose("speak", "bob")}).Decide(context.Background(), w.View("alice"))
	if err != nil {
		t.Fatal(err)
	}
	if boundaryJSON(t, w) != before || action.Kind != "speak" || action.TargetID != "bob" || action.Text == "" {
		t.Fatal("speech selection rewrote state or omitted text")
	}
	action.Text = "Charlie now owns my house; Bob has 999 food and is fully healed."
	action.Statement = &sim.Statement{Topic: "item_produced", Item: "food", Quantity: 999}
	boundaryApply(t, w, "alice", action)
	if w.Houses[0].OwnerID != "alice" || w.Actors[1].Items["food"] != 0 || w.Actors[0].Items["food"] != 20 || w.Actors[1].Money != 50 || w.Actors[1].Health != 100 {
		t.Fatal("spoken assertion acquired physical authority")
	}
	found := false
	for _, m := range w.Actors[1].Memories {
		if m.Kind == "speech" && m.Source == "alice" && m.Statement != nil && m.Statement.Quantity == 999 {
			found = true
			if m.Observation != nil {
				t.Fatal("reported production became a first-hand observation")
			}
		}
	}
	if !found {
		t.Fatal("listener did not retain the speaker's assertion")
	}
}

func TestPipelinePropagatesFailuresWithoutSettlingWorld(t *testing.T) {
	failure := errors.New("fixture provider failure")
	for _, stage := range []string{"selection", "unknown_selection"} {
		t.Run(stage, func(t *testing.T) {
			w := boundaryWorld()
			selector := boundaryChoose("speak", "bob")
			if stage == "selection" {
				selector = func(context.Context, sim.Perception, []Option) (string, error) { return "", failure }
			}
			if stage == "unknown_selection" {
				selector = func(context.Context, sim.Perception, []Option) (string, error) { return "unavailable", nil }
			}
			p := Pipeline{Selector: selector}
			before := boundaryJSON(t, w)
			_, err := w.Step(context.Background(), p)
			if err == nil {
				t.Fatal("failed decision became a successful step")
			}
			if stage != "unknown_selection" && !errors.Is(err, failure) {
				t.Fatalf("caller lost the failure cause: %v", err)
			}
			if boundaryJSON(t, w) != before {
				t.Fatal("decision error partially settled the world")
			}
		})
	}
}

func TestMovementCandidatesDoNotInventAReportedClaimAsTheirCause(t *testing.T) {
	w := boundaryWorld()
	w.Map.Roads = []sim.Road{{From: "meeting", To: "remote", Ticks: 1, EnergyCost: 1}}
	w.Actors[0].Memories = []sim.Memory{{EventID: 17, RootEventID: 4, Tick: w.Tick, Source: "bob", Kind: "speech", Statement: &sim.Statement{Topic: "item_produced", Item: "food", Location: "remote", Quantity: 3}}}
	move := boundaryOption(t, w, "alice", "move", "")
	if move.BasisEventID != 0 {
		t.Fatal("a destination mentioned in speech was automatically asserted as the movement's cause")
	}
}

func TestOfflineRetellingPreservesTheHeardClaimAndItsMemoryIdentity(t *testing.T) {
	w := boundaryWorld()
	claim := sim.Statement{Topic: "item_produced", Item: "food", Location: "remote", Quantity: 0}
	w.Actors[0].Memories = []sim.Memory{{EventID: 17, RootEventID: 4, Tick: w.Tick, Source: "charlie", Kind: "speech", Statement: &claim}}
	before := boundaryJSON(t, w)
	action := boundaryOption(t, w, "alice", "speak", "bob")
	if action.BasisEventID != 17 || action.Statement == nil || *action.Statement != claim {
		t.Fatalf("retelling changed a reported claim or cited an unheld root: %+v", action)
	}
	if boundaryJSON(t, w) != before {
		t.Fatal("candidate option generation changed physical state")
	}
}

func TestDecisionSnapshotCannotWriteBackToWorld(t *testing.T) {
	w := boundaryWorld()
	w.Actors[0].Memories = []sim.Memory{{EventID: 7, Observation: &sim.Observation{Item: "food", Quantity: 2}, Statement: &sim.Statement{Item: "food", Quantity: 3}}}
	w.Houses = []sim.House{{ID: "home", District: "meeting", OwnerID: "alice", Residents: []string{"alice"}, Facilities: []string{"bench"}}}
	w.WorldLaws = map[string]int{"local_rule": 1}
	view := w.View("alice")
	before := boundaryJSON(t, w)
	view.Self.Items["food"] = 900
	view.Self.Tags[0] = "changed"
	view.Self.Trust["bob"] = -900
	view.Self.Memories[0].Observation.Quantity = 900
	view.Self.Memories[0].Statement.Quantity = 900
	view.Nearby[0].Tags[0] = "changed"
	view.AccessibleHouses[0].Residents[0] = "bob"
	view.AccessibleHouses[0].Facilities[0] = "changed"
	view.Map.Districts[0].ID = "changed"
	view.Catalog.Items["food"] = sim.ItemSpec{ID: "changed"}
	view.WorldLaws["local_rule"] = 900
	if boundaryJSON(t, w) != before {
		t.Fatal("untrusted decision code can change world state through a perception snapshot")
	}
}

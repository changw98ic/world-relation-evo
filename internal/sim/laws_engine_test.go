package sim

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
)

type lawDecider func(context.Context, Perception) (Action, error)

func (d lawDecider) Decide(ctx context.Context, p Perception) (Action, error) { return d(ctx, p) }

// D1: a failed set of decisions has no partial settlement. All successful
// decisions see the same pre-step snapshot, not another actor's proposed action.
func TestLawEngineDecisionFailureLeavesWholeWorldUnchanged(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"] = 3
	w.Actors[2].Task = &Task{Action: Action{Kind: "move", Location: "away"}, Remaining: 1}
	before := lawJSON(t, w)
	providerError := errors.New("fixture provider unavailable")
	events, err := w.Step(context.Background(), lawDecider(func(_ context.Context, p Perception) (Action, error) {
		if p.Self.ID == "b" {
			return Action{}, providerError
		}
		return Action{Kind: "craft", RecipeID: "board"}, nil
	}))
	if !errors.Is(err, providerError) || len(events) != 0 || !bytes.Equal(before, lawJSON(t, w)) {
		t.Fatal("decision error was hidden or partially settled work, travel, time, money or environment")
	}
}

func TestLawEnginePerceptionIsAnIsolatedCopy(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"] = 3
	w.Actors[0].Memories = []Memory{{EventID: 1, Source: "a", Observation: &Observation{Item: "wood", Quantity: 3}, Statement: &Statement{Topic: "grain", Quantity: 9}}}
	w.Map.Roads = []Road{{From: "home", To: "away", Ticks: 2, EnergyCost: 4}}
	before := lawJSON(t, w)
	p := w.View("a")
	p.Self.Items["wood"] = 99
	p.Self.Professions["worker"] = 99
	p.Self.Memories[0].Observation.Quantity = 99
	p.Self.Memories[0].Statement.Quantity = 99
	p.Catalog.Recipes[0].Inputs["wood"] = 99
	p.Map.Roads[0].EnergyCost = 99
	if !bytes.Equal(before, lawJSON(t, w)) {
		t.Fatal("a character's perception alias changed physical world or stored evidence")
	}
}

func TestLawEngineDecisionsSharePreSettlementState(t *testing.T) {
	w := lawWorld()
	w.Actors = w.Actors[:2]
	w.Catalog.ActionSpecs = map[string]ActionSpec{"strike": {ID: "strike", GrantedByTags: []string{"person"}, TargetKind: "actor", Attack: &AttackSpec{DamageByType: map[string]int{"physical": 10}}}}
	var mu sync.Mutex
	seen := map[string]Perception{}
	_, err := w.Step(context.Background(), lawDecider(func(_ context.Context, p Perception) (Action, error) {
		mu.Lock()
		seen[p.Self.ID] = p
		mu.Unlock()
		if p.Self.ID == "a" {
			return Action{Kind: "strike", TargetID: "b"}, nil
		}
		return Action{Kind: "wait"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen["a"].Tick != 0 || seen["b"].Tick != 0 || seen["b"].Self.Health != 100 || w.Actors[1].Health != 90 || w.Tick != 1 {
		t.Fatal("victim decided after observing a hit from the same settlement tick, or funded hit did not settle")
	}
	for _, memory := range seen["b"].Self.Memories {
		if memory.Observation != nil && memory.Observation.Kind == "attack" {
			t.Fatal("victim observed a future hit in the pre-step snapshot")
		}
	}
}

// A deliberately injected unlogged asset is a negative control for accounting.
// Passing this audit says only that the declared ledger agrees with holdings.
func TestLawEngineLedgerDetectsAnUnfundedAssetPerturbation(t *testing.T) {
	w := lawWorld()
	w.Actors[0].Items["wood"] = 3
	w.InitLedgerAudit()
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
	w.Actors[1].Items["wood"]++
	if err := w.VerifyLedgerConservation(); err == nil {
		t.Fatal("audit did not detect an actual item created without a debit or production record")
	}
	w.Actors[1].Items["wood"]--
	lawOK(t, w.Apply("a", Action{Kind: "craft", RecipeID: "board"}))
	for i := 0; i < 2; i++ {
		w.Tick++
		w.Advance("a")
	}
	if w.Actors[0].Items["board"] != 1 || w.Actors[0].Items["wood"] != 0 {
		t.Fatal("fixture conversion failed its independent holdings check")
	}
	if err := w.VerifyLedgerConservation(); err != nil {
		t.Fatal(err)
	}
}

// S1: a traveler cannot reach a fixed endpoint water source, but can drink
// carried water. The stationary control proves the source itself works.
func TestLawSpaceTravelCannotDrinkStationaryEndpointWater(t *testing.T) {
	for _, tc := range []struct {
		name            string
		travel, carried bool
		wantWater       int64
		wantThirst      int
	}{
		{name: "stationary", wantWater: 0, wantThirst: 0},
		{name: "in_transit", travel: true, wantWater: 1, wantThirst: 1},
		{name: "in_transit_carried_water", travel: true, carried: true, wantWater: 1, wantThirst: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := lawWorld()
			w.Actors = w.Actors[:1]
			w.Tick = 22
			w.Map.Roads = []Road{{From: "home", To: "away", Ticks: 5, EnergyCost: 6}}
			w.DistrictStates = map[string]DistrictState{"home": {Resources: map[string]int64{"freshwater": 1}, MaxResources: map[string]int64{"freshwater": 1}}}
			if tc.carried {
				w.Actors[0].Thirst = 1
				w.Actors[0].Items["water"] = 1
			}
			if tc.travel {
				lawOK(t, w.Apply("a", Action{Kind: "move", Location: "away"}))
			}
			for i := 0; i < 2; i++ {
				if _, err := w.Step(context.Background(), lawDecider(func(context.Context, Perception) (Action, error) { return Action{Kind: "wait"}, nil })); err != nil {
					t.Fatal(err)
				}
			}
			if w.Tick != 24 || tc.travel && (!w.Actors[0].InTransit() || w.Actors[0].Task.Remaining != 2) {
				t.Fatal("fixture did not reach the six-hour check while two travel ticks remained")
			}
			water, thirst := w.DistrictStates["home"].Resources["freshwater"], w.Actors[0].Thirst
			if water != tc.wantWater || thirst != tc.wantThirst || w.Actors[0].Items["water"] != 0 {
				t.Fatalf("water access violated stationary/carried source budget: origin_water=%d thirst=%d carried_water=%d; want %d/%d/0", water, thirst, w.Actors[0].Items["water"], tc.wantWater, tc.wantThirst)
			}
		})
	}
}

// S1: waste and digested food are carried with the traveler. Neither can be
// deposited in an endpoint's fixed latrine before the traveler arrives there.
func TestLawSpaceTravelCannotUseStationaryEndpointSanitation(t *testing.T) {
	for _, travel := range []bool{false, true} {
		name := "stationary"
		if travel {
			name = "in_transit"
		}
		t.Run(name, func(t *testing.T) {
			w := lawWorld()
			w.Actors = w.Actors[:1]
			w.Tick = 22
			w.Map.Roads = []Road{{From: "home", To: "away", Ticks: 5, EnergyCost: 6}}
			w.Catalog.Lifecycle.WasteItem = "nightsoil"
			w.Catalog.Lifecycle.WastePerFoodDivisor = 2
			w.Catalog.Items["nightsoil"] = ItemSpec{ID: "nightsoil", MassUnits: 1}
			w.DistrictStates = map[string]DistrictState{"home": {Sanitation: 80, Infrastructure: map[string]int{"public_latrine": 100}, NightsoilPool: 3}}
			w.Actors[0].Waste, w.Actors[0].DigestedFood = 2, 4
			if travel {
				lawOK(t, w.Apply("a", Action{Kind: "move", Location: "away"}))
			}
			for i := 0; i < 2; i++ {
				if _, err := w.Step(context.Background(), lawDecider(func(context.Context, Perception) (Action, error) { return Action{Kind: "wait"}, nil })); err != nil {
					t.Fatal(err)
				}
			}
			ds, actor := w.DistrictStates["home"], w.Actors[0]
			if w.Tick != 24 || travel && (!actor.InTransit() || actor.Task.Remaining != 2) {
				t.Fatal("fixture did not remain in transit through the sanitation check")
			}
			if travel {
				if ds.NightsoilPool != 3 || ds.Sanitation != 80 || actor.Waste != 2 || actor.DigestedFood != 4 {
					t.Fatalf("traveler used stationary origin latrine: nightsoil=%d sanitation=%d waste=%d digested_food=%d; want 3/80/2/4", ds.NightsoilPool, ds.Sanitation, actor.Waste, actor.DigestedFood)
				}
			} else if ds.NightsoilPool != 5 || ds.Sanitation != 80 || actor.Waste != 0 || actor.DigestedFood != 0 {
				t.Fatalf("stationary latrine did not conserve the declared four-food/two-nightsoil budget: nightsoil=%d sanitation=%d waste=%d digested_food=%d", ds.NightsoilPool, ds.Sanitation, actor.Waste, actor.DigestedFood)
			}
		})
	}
}

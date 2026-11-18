package sim

import (
	"context"
	"reflect"
	"testing"
)

func TestIndexedPerceptionContainsTheSameLocalSupplyAndYieldFacts(t *testing.T) {
	w := lawWorld()
	w.Cohorts = []PopulationCohort{{ID: "neighbors", District: "home", Population: 2, Adults: 2, Profession: "worker"}}
	direct := w.View("a")
	indexed := w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct.LocalCohorts, indexed.LocalCohorts) || !reflect.DeepEqual(direct.ExpectedCraftYield, indexed.ExpectedCraftYield) {
		t.Fatalf("the actual stepping path hid supply or promised different output: direct=%+v/%v indexed=%+v/%v", direct.LocalCohorts, direct.ExpectedCraftYield, indexed.LocalCohorts, indexed.ExpectedCraftYield)
	}
}

func TestConcurrentActorPerceptionsCannotRewriteEachOthersCatalog(t *testing.T) {
	w := lawWorld()
	changed := make(chan struct{})
	_, err := w.Step(context.Background(), lawDecider(func(_ context.Context, p Perception) (Action, error) {
		if p.Self.ID == "a" {
			spec := p.Catalog.Items["grain"]
			spec.HungerRelief = 999
			p.Catalog.Items["grain"] = spec
			close(changed)
		} else {
			<-changed
			if p.Catalog.Items["grain"].HungerRelief != 1 {
				t.Error("one actor's perception rewrote another actor's input")
			}
		}
		return Action{Kind: "wait"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if w.Catalog.Items["grain"].HungerRelief != 1 {
		t.Fatal("actor perception rewrote the physical catalog")
	}
}

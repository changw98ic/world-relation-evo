package sim

import (
	"context"
	"reflect"
	"testing"
)

func TestIndexedPerceptionContainsTheSameLocalSupplyAndYieldFacts(t *testing.T) {
	w := lawWorld()
	w.Cohorts = []PopulationCohort{{ID: "neighbors", District: "home", Population: 2, Adults: 2, Profession: "worker"}}
	w.ActiveWorldEvents = []ActiveWorldEvent{{
		InstanceID:  "relic@1",
		SpecID:      "relic",
		Name:        "Ancient Relic",
		Category:    "artifact_emergence",
		DistrictID:  "home",
		StartedTick: 0,
		ExpiresTick: 96,
		Intensity:   65,
	}}
	direct := w.View("a")
	indexed := w.viewIndexed(0, w.buildStepIndex())
	if !reflect.DeepEqual(direct.LocalCohorts, indexed.LocalCohorts) || !reflect.DeepEqual(direct.ExpectedCraftYield, indexed.ExpectedCraftYield) || !reflect.DeepEqual(direct.ActiveWorldEvents, indexed.ActiveWorldEvents) {
		t.Fatalf("the actual stepping path hid supply, yield, or world events: direct=%+v/%v/%+v indexed=%+v/%v/%+v", direct.LocalCohorts, direct.ExpectedCraftYield, direct.ActiveWorldEvents, indexed.LocalCohorts, indexed.ExpectedCraftYield, indexed.ActiveWorldEvents)
	}
}

func TestConcurrentActorPerceptionsCannotRewriteEachOthersCatalog(t *testing.T) {
	w := lawWorld()
	changed := make(chan struct{})
	_, err := w.Step(context.Background(), lawDecider(func(_ context.Context, p Perception) (Action, error) {
		if p.Self.ID == "a" {
			spec := p.Catalog.Items["grain"]
			spec.NutritionUnits = 999
			p.Catalog.Items["grain"] = spec
			close(changed)
		} else {
			<-changed
			if p.Catalog.Items["grain"].NutritionUnits != 1 {
				t.Error("one actor's perception rewrote another actor's input")
			}
		}
		return Action{Kind: "wait"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if w.Catalog.Items["grain"].NutritionUnits != 1 {
		t.Fatal("actor perception rewrote the physical catalog")
	}
}

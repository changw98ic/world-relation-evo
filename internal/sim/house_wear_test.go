package sim

import (
	"context"
	"testing"
)

func TestOrdinaryHouseWeatheringUsesYearsRatherThanFiftyDays(t *testing.T) {
	w := lawWorld()
	w.Actors = nil
	w.Houses = []House{{ID: "roof", District: "home", Condition: 100}}
	for tick := 0; tick < 50*TicksPerDay; tick++ {
		if _, err := w.Step(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if w.Houses[0].Condition != 100 {
		t.Fatalf("ordinary shelter lost %d/100 condition in fifty days without a disaster or damage event", 100-w.Houses[0].Condition)
	}
}

func TestHouseWeatheringRespectsItsConfiguredIntervalAndExplicitZero(t *testing.T) {
	for _, loss := range []int{0, 2, 100} {
		w := lawWorld()
		w.Houses = []House{{ID: "roof", Condition: 100}}
		w.Catalog.HouseWear = &HouseWearRule{IntervalTicks: 3, ConditionLoss: loss, Source: "fixture: declared roof loss every three ticks"}
		if err := w.Catalog.ValidatePhysicalRules(nil); err != nil {
			t.Fatal(err)
		}
		w.Tick = 2
		w.weatherHouses()
		if w.Houses[0].Condition != 100 {
			t.Fatal("wear was charged before its declared period")
		}
		w = lawRestore(t, w)
		w.Tick = 3
		w.weatherHouses()
		if w.Houses[0].Condition != 100-loss {
			t.Fatal("interval boundary lost or duplicated the declared wear")
		}
	}
}

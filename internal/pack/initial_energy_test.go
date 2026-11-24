package pack

import "testing"

func TestPackPreservesExplicitlyExhaustedCohortEnergy(t *testing.T) {
	p := rightsPack()
	p.Cohorts[0].AvgEnergy = 0
	w, err := p.Instantiate("")
	if err != nil {
		t.Fatal(err)
	}
	if w.Cohorts[0].AvgEnergy != 0 {
		t.Fatal("loading a declared zero-energy cohort created usable energy")
	}
}

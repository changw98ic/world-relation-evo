package sim

// This is the existing cash-relief eligibility policy, applied to named people
// regardless of representation and to anonymous households on a per-person basis.
func (w *World) reliefRecipients(node *PolityNode, donorID string) (people []*Actor, groups []*PopulationCohort) {
	consider := func(person *Actor) {
		if person.Dead || person.ID == donorID || !w.Polities.PolityGovernsNode(node.ID, w.Map.PolityIDForLocation(person.Location, person.VenueID)) {
			return
		}
		if person.Money < 12 || person.Hunger > 0 || person.Health < MaxHealth {
			people = append(people, person)
		}
	}
	for i := range w.Actors {
		consider(&w.Actors[i])
	}
	for ci := range w.Cohorts {
		cohort := &w.Cohorts[ci]
		for i := range cohort.Members {
			consider(&cohort.Members[i])
		}
		anonymous := w.anonymousCohort(*cohort)
		if anonymous.Population > 0 && w.Polities.PolityGovernsNode(node.ID, w.Map.PolityIDForLocation(cohort.District, "")) && (anonymous.MoneyPool < int64(anonymous.Population)*12 || anonymous.AvgHunger > 0 || anonymous.Wounded > 0 || anonymous.Sick > 0) {
			groups = append(groups, cohort)
		}
	}
	return people, groups
}

package sim

import "testing"

func TestMentorshipBondFormationAndProtectiveVendetta(t *testing.T) {
	w := Village("mentorship-test")
	w.Catalog.Skills = map[string]SkillSpec{
		"sword_art": {ID: "sword_art", Name: "剑道", MaxLevel: 6, CombatBonusPerLevel: 4},
	}

	mentor := &w.Actors[0]
	apprentice := &w.Actors[1]
	killer := &w.Actors[2]
	for _, a := range []*Actor{mentor, apprentice, killer} {
		a.Location = w.Map.Districts[0].ID
		a.InsideHouse = ""
		a.VenueID = ""
		a.Energy = 90
		a.Health = 100
		if a.Trust == nil {
			a.Trust = map[string]int{}
		}
	}
	mentor.Skills = map[string]int{"sword_art": 4}
	apprentice.Skills = map[string]int{"sword_art": 1}
	mentor.Trust[killer.ID] = 0

	// 1. Teaching a skill establishes a formal MentorshipBond and emits mentorship_formed.
	evs := w.applyTeachSkill(mentor, Action{Kind: "teach_knowledge", SkillID: "sword_art", TargetID: apprentice.ID}, apprentice)
	foundFormed := false
	for _, ev := range evs {
		if ev.Kind == "mentorship_formed" {
			foundFormed = true
		}
	}
	if !foundFormed {
		t.Fatalf("expected mentorship_formed event after applyTeachSkill, got %+v", evs)
	}
	if len(w.ensureRelations().Mentorships) != 1 || w.Relations.Mentorships[0].MentorID != mentor.ID || w.Relations.Mentorships[0].ApprenticeID != apprentice.ID {
		t.Fatalf("expected formal MentorshipBond recorded, got %+v", w.Relations.Mentorships)
	}

	// 2. Subsequent lesson increments Lessons without duplicating the bond.
	mentor.Energy = 90
	w.applyTeachSkill(mentor, Action{Kind: "teach_knowledge", SkillID: "sword_art", TargetID: apprentice.ID}, apprentice)
	if len(w.Relations.Mentorships) != 1 || w.Relations.Mentorships[0].Lessons != 2 {
		t.Fatalf("expected single MentorshipBond with Lessons=2, got %+v", w.Relations.Mentorships)
	}

	// 3. Killing the apprentice triggers protective blood-feud grudge & trust drop for the surviving mentor!
	apprentice.Health = 0
	deathEvs := w.markDeadIfZero(apprentice, killer, Action{Kind: "attack", TargetID: apprentice.ID}, "combat")
	if len(deathEvs) == 0 {
		t.Fatalf("expected death events")
	}
	if mentor.Trust[killer.ID] >= 0 {
		t.Fatalf("expected mentor trust toward apprentice's killer to plummet (<0), got %d", mentor.Trust[killer.ID])
	}
	hasMentorGrudge := false
	for _, g := range w.Relations.Grudges {
		if g.AggrievedID == mentor.ID && g.OffenderID == killer.ID && g.Intensity >= 50 {
			hasMentorGrudge = true
			break
		}
	}
	if !hasMentorGrudge {
		t.Fatalf("expected surviving mentor to form high-intensity vendetta grudge against killer, got grudges=%+v", w.Relations.Grudges)
	}
}

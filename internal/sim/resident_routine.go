package sim

const martialForms = "martial_forms"

// The prefecture has a public martial hall and a standing teaching contract.
// Fourteen days is its practice retention curve. Residents begin to value
// refreshing that practice after two days away from the hall.
func (w *World) configureResidentRoutine() {
	const practiceSlot = "martial_practice"
	const contactSlot = "social_contact"
	const practiceAction = "drill_martial_forms"
	const retention = 14 * TicksPerDay
	c := &w.Catalog
	if c.StateSlots == nil {
		c.StateSlots = make(map[string]StateSlotSpec)
	}
	c.StateSlots[practiceSlot] = StateSlotSpec{ID: practiceSlot, Name: "武艺熟练度", RequiredTag: "martial_artist",
		Max: retention, Default: retention, RegenPerTick: -1}
	c.StateSlots[contactSlot] = StateSlotSpec{ID: contactSlot, Name: "近期交往",
		Max: 2 * TicksPerDay, Default: 2 * TicksPerDay, RegenPerTick: -1, ConversationGain: 2 * TicksPerDay}
	for i := range c.Professions {
		if c.Professions[i].ID == "martial_artist" || c.Professions[i].ParentProfession == "martial_artist" {
			c.Professions[i].PracticeSlot = practiceSlot
		}
	}
	if c.Skills == nil {
		c.Skills = make(map[string]SkillSpec)
	}
	c.Skills[martialForms] = SkillSpec{ID: martialForms, Name: "拳术", Category: "martial", MaxLevel: 6}
	c.TagActions["martial_artist"] = appendUnique(c.TagActions["martial_artist"], "teach_knowledge")
	if c.ActionSpecs == nil {
		c.ActionSpecs = make(map[string]ActionSpec)
	}
	c.ActionSpecs[practiceAction] = ActionSpec{
		ID: practiceAction, Name: "练拳保持熟练", TargetKind: "self", GrantedByTags: []string{"martial_artist"},
		Profession: "martial_artist", RequiredVenueFeature: "martial_hall", RequireOutdoors: true,
		Ticks: TicksPerHour, EnergyCost: 8, SlotGains: map[string]int{practiceSlot: retention},
		ProfessionXP: 2,
	}
	for _, hall := range w.Cohorts {
		if hall.Profession == "martial_artist" && w.Map.DistrictHasFeature(hall.District, "martial_hall") {
			c.WorkOrders = append(c.WorkOrders, WorkOrder{
				ID: hall.ID + "/lessons", Name: "武馆授课", EmployerID: hall.ID, ActionKind: "teach_knowledge",
				CompletionKind: "knowledge_taught", RequiredFeature: "martial_hall", District: hall.District,
				SkillID: martialForms, Wage: 3, PayCycle: PayWeekly,
			})
		}
	}
	rules := c.EffectivePlanningRules()
	rules.Needs = append(rules.Needs,
		NeedRule{NeedKey: NeedKey{Measure: "state_slot", Subject: practiceSlot}, Target: retention - 2*TicksPerDay, Scale: TicksPerDay, BaseWeight: 0.7},
		NeedRule{NeedKey: NeedKey{Measure: "state_slot", Subject: contactSlot}, Target: TicksPerDay, Scale: TicksPerDay, BaseWeight: 0.4},
		NeedRule{NeedKey: NeedKey{Measure: "conversation"}, Target: 1, Scale: 1, BaseWeight: 0.25},
	)
	c.Planning = &rules
}

func (w *World) seedResidentPractice(a *Actor) {
	level := min(a.Professions["martial_artist"], len(w.Catalog.LevelThresholds))
	skill, available := w.Catalog.Skills[martialForms]
	if !available || level < 1 || a.Skills[martialForms] >= level {
		return
	}
	level = min(level, skill.EffectiveMaxLevel())
	if a.Skills == nil {
		a.Skills = make(map[string]int)
	}
	if a.SkillXP == nil {
		a.SkillXP = make(map[string]int)
	}
	a.Skills[martialForms] = level
	a.SkillXP[martialForms] = w.Catalog.LevelThresholds[level-1]
}

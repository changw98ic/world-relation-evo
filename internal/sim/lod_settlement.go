package sim

import "fmt"

// resolveCohortAttack applies shared cross-LOD combat laws when a high-tier actor
// or declarative ActionSpec strikes an aggregated PopulationCohort:
//   - Respects cohort FormSpec.PowerTier suppression & immunity
//   - Absorbs incoming damage through active district-anchored barriers (ActiveEffect)
//   - Applies scaled casualties to the cohort only with the remaining unabsorbed force
func (w *World) resolveCohortAttack(attacker Actor, c *PopulationCohort, atk *AttackSpec, x Action) (wounded, dead, absorbed int, immune bool, brokenEvents []Event) {
	if w == nil || c == nil || c.Population <= 0 || atk == nil {
		return 0, 0, 0, false, nil
	}
	atkTier := atk.Tier
	if t := w.ActorCombatTier(attacker); t > atkTier {
		atkTier = t
	}
	if t := w.Catalog.ActorPowerTier(attacker); t > atkTier {
		atkTier = t
	}
	defTier := c.Tier
	if c.FormID != "" {
		if form, ok := w.Catalog.Forms[c.FormID]; ok && form.PowerTier > defTier {
			defTier = form.PowerTier
		}
	}
	// Tier suppression: a cohort whose form tier exceeds attacker tier by >= 2 is immune to low-tier assaults.
	if defTier >= atkTier+2 {
		return 0, 0, 0, true, nil
	}

	_, _, _, profCohortDmg := w.Catalog.ActorCombatProfile(attacker)
	cas := atk.CohortCasualties
	if cas <= 0 {
		cas = profCohortDmg
	}
	if cas <= 0 {
		baseDmg := 0
		for _, d := range atk.DamageByType {
			if d > 0 {
				baseDmg += d
			}
		}
		if baseDmg > 0 {
			cas = maxInt(1, baseDmg/10)
		}
	}
	if cas <= 0 {
		return 0, 0, 0, false, nil
	}
	// Scale casualties by tier advantage (high-tier power against lower-tier cohorts remains decisive).
	if atkTier > defTier {
		cas += (atkTier - defTier) * (cas / 2)
	} else if defTier > atkTier {
		cas = maxInt(1, cas/2)
	}

	// District-anchored ActiveEffect barriers absorb cohort strike impact using their Integrity.
	totalImpact := cas
	for i := range w.ActiveEffects {
		eff := &w.ActiveEffects[i]
		if eff.ExpiresTick > 0 && w.Tick >= eff.ExpiresTick {
			continue
		}
		if eff.Anchor.Kind != "district" || eff.Anchor.ID != c.District {
			continue
		}
		if !eff.AbsorbDamageWithIntegrity || eff.Integrity <= 0 {
			continue
		}
		// Barrier blocks proportionally up to its remaining integrity.
		absorb := minInt(totalImpact, eff.Integrity)
		eff.Integrity -= absorb
		absorbed += absorb
		totalImpact -= absorb
		if eff.Integrity <= 0 {
			brokenEvents = append(brokenEvents, Event{
				ID:      w.NextID,
				Tick:    w.Tick,
				ActorID: attacker.ID,
				Kind:    "effect_broken",
				Action:  x,
				Payload: map[string]any{
					"text":        fmt.Sprintf("%s shattered barrier %s protecting %s", attacker.Name, eff.Name, c.District),
					"effect_id":   eff.ID,
					"effect_spec": eff.SpecID,
					"district":    c.District,
				},
			})
			w.NextID++
		}
		if totalImpact <= 0 {
			break
		}
	}
	if totalImpact <= 0 {
		return 0, 0, absorbed, false, brokenEvents
	}

	wounded, dead, _ = w.applyCohortCasualtiesWithCause(c, totalImpact*2, totalImpact, 0, "combat")
	return wounded, dead, absorbed, false, brokenEvents
}

// releaseCohortJailed releases up to want anonymous jailed residents from cohort c,
// detaching stored Members first and keeping ResidentBand.Jailed synchronized so
// c.Jailed can never go negative when stored Members later complete their sentences.
func (w *World) releaseCohortJailed(c *PopulationCohort, want int) int {
	if c == nil || want <= 0 {
		return 0
	}
	restore := w.detachCohortMembers()
	defer restore()
	if c.Jailed <= 0 {
		return 0
	}
	released := minInt(c.Jailed, want)
	if released <= 0 {
		return 0
	}
	c.ensureResidents(c.Population)
	released = c.updateResidentBandState(released, func(b ResidentBand) bool { return b.Jailed }, func(b *ResidentBand) { b.Jailed = false })
	c.Jailed -= released
	return released
}

// relieveCohortHunger reduces anonymous cohort hunger by delta while keeping
// ResidentBand.Hunger, HungerReliefRemainder, and AvgHunger strictly synchronized.
func (w *World) relieveCohortHunger(c *PopulationCohort, delta int) {
	if c == nil || delta <= 0 {
		return
	}
	restore := w.detachCohortMembers()
	defer restore()
	if c.Population <= 0 {
		return
	}
	c.ensureResidents(c.Population)
	var totalHunger int64
	for i := range c.Residents {
		if c.Residents[i].Hunger > 0 {
			c.Residents[i].Hunger = maxInt(0, c.Residents[i].Hunger-delta)
		}
		totalHunger += int64(c.Residents[i].Count) * int64(c.Residents[i].Hunger)
	}
	c.setHungerUnits(c.Population, totalHunger)
}

// SettlementTaxAssessment computes the canonical poll tax assessment shared across
// individual actors and macro cohorts, halving/quartering consistently during tax strikes.
func SettlementTaxAssessment(taxable int64, pollPermille int, taxStriking bool) int64 {
	if taxable <= 0 || pollPermille <= 0 {
		return 0
	}
	assessed := (taxable * int64(pollPermille)) / 1000
	if taxStriking {
		assessed /= 4
	}
	return assessed
}

// SettlementCorruptionSurcharge computes corrupt collector surcharge ("meltage") shared
// across LOD boundaries, capped at a safe fraction of taxable surplus.
func SettlementCorruptionSurcharge(assessed, taxable int64, corruption int) int64 {
	if assessed <= 0 || taxable <= 0 || corruption < 12 {
		return 0
	}
	surPermille := 90 + corruption*3
	if surPermille > 350 {
		surPermille = 350
	}
	surcharge := (assessed * int64(surPermille)) / 1000
	if assessed+surcharge > taxable/4 {
		surcharge = maxInt64(0, taxable/4-assessed)
	}
	return surcharge
}

// CohortFormLifeExpectancy resolves the effective life expectancy (in years) for a
// PopulationCohort by consulting its FormSpec.LifeExpectancyYears first (matching individual
// Actor lifespan semantics), then falling back to Catalog.Lifecycle.MaxExpectancy.
// A return value <= 0 denotes an ageless / immortal / synthetic form.
func CohortFormLifeExpectancy(cat Catalog, c PopulationCohort) int {
	if cat.Lifecycle.Ageless {
		return 0
	}
	if c.FormID != "" {
		if form, ok := cat.Forms[c.FormID]; ok {
			if form.LifeExpectancyYears < 0 {
				return 0
			}
			if form.LifeExpectancyYears > 0 {
				return form.LifeExpectancyYears
			}
		}
	}
	if cat.Lifecycle.MaxExpectancy < 0 {
		return 0
	}
	if cat.Lifecycle.MaxExpectancy > 0 {
		return cat.Lifecycle.MaxExpectancy
	}
	return DefaultLifecycleSpec().MaxExpectancy
}

// cohortEffectiveTagsAndCaps returns the combined profession + form tags and action capabilities
// for a PopulationCohort so macro conflict/governance systems never hardcode genre profession IDs.
func (w *World) cohortEffectiveTagsAndCaps(c PopulationCohort) ([]string, map[string]bool) {
	var tags []string
	if prof, ok := w.Catalog.ProfessionByID(c.Profession); ok {
		tags = append(tags, prof.Grants...)
	}
	if c.FormID != "" {
		if form, ok := w.Catalog.Forms[c.FormID]; ok {
			tags = append(tags, form.InnateTags...)
		}
	}
	caps := w.Catalog.Capabilities(Actor{Tags: tags})
	return tags, caps
}

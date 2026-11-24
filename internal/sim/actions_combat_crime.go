package sim

import "fmt"

func registerCombatCrimeActions() {
	registerBuiltinAction("attack", handleAttackAction)
	registerBuiltinAction("spar", handleSparAction)
	registerBuiltinAction("steal", handleStealAction)
	registerBuiltinAction("raid", handleRaidAction)
	registerBuiltinAction("extort", handleExtortAction)
	registerBuiltinAction("bribe", handleBribeAction)
	registerBuiltinAction("arrest", handleArrestAction)
}

func handleAttackAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if blocked, reason := w.IsActionBlockedByWorldLaw(*a, x); blocked {
		return actionFail(tx, a, x, reason)
	}
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "target is not nearby")
	}
	if a.Health < 25 {
		return actionFail(tx, a, x, "too injured to fight; must recuperate")
	}
	atkPower := CombatPowerWithCatalog(w.Catalog, *a)
	defPower := CombatPowerWithCatalog(w.Catalog, *b)
	stance := w.actorFactionRelation(*a, *b)
	hasGrudge := w.Relations != nil && w.Relations.hasUnresolvedGrudge(a.ID, b.ID)
	base := 0
	profiles := len(w.Catalog.CombatProfiles) > 0 || len(w.ActiveEffects) > 0
	if !profiles {
		if (stance == "at_war" || hasGrudge) && atkPower >= defPower-10 {
			base = maxInt(14, minInt(65, 22+(atkPower-defPower)))
		} else {
			base = maxInt(3, minInt(28, 10+(atkPower-defPower)/2))
		}
	}
	damage, absorbed, immune, effects := w.applyMeleeHit(tx, a, b, x, base)
	recoil := 0
	if !profiles && damage > 0 && defPower >= atkPower+20 && b.Health > 0 && a.Health > 1 {
		recoil = -tx.adjustHealth(a, -minInt(14, (defPower-atkPower)/5), 1, x.Kind)
	}
	if a.Health < 65 {
		a.RestUntil = w.Tick + 4
	}
	if stance == "allied" {
		tx.MutateTrust(b, a.ID, -4)
	} else {
		tx.MutateTrust(b, a.ID, -2)
	}
	data := map[string]any{
		"text":   fmt.Sprintf("%s attacked %s (%d damage, %d absorbed, immune=%v)", a.Name, b.Name, damage, absorbed, immune),
		"damage": damage, "attacker_power": atkPower, "defender_power": defPower,
		"recoil": recoil, "absorbed": absorbed, "immune_blocked": immune,
		"district": a.Location, "faction_relation": stance,
	}
	if stance != "at_war" {
		data["observation"] = Observation{Kind: "unlawful_attack", Modality: "eyewitness",
			SubjectID: a.ID, ObjectID: b.ID, Location: a.Location, Quantity: damage}
	}
	ev := tx.record(a, x, "attacked", data, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	events := append(effects, ev)
	events = append(events, w.finishMeleeHit(tx, a, b, x, damage, "combat attack", ev)...)
	return append(events, w.grantCombatXP(tx, a, x, 4, 1)...)
}

func handleSparAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "sparring partner is not nearby")
	}
	if w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "still recuperating from recent bout")
	}
	if a.Health < 65 || b.Health < 65 {
		return actionFail(tx, a, x, "fighter is injured and must recuperate before sparring")
	}
	if a.Health > 60 {
		tx.adjustHealth(a, -2, 60, x.Kind)
	}
	if b.Health > 60 {
		tx.adjustHealth(b, -2, 60, x.Kind)
	}
	a.RestUntil = w.Tick + 48
	if b.RestUntil < w.Tick+24 {
		b.RestUntil = w.Tick + 24
	}
	tx.MutateTrust(a, b.ID, 1)
	tx.MutateTrust(b, a.ID, 1)
	atkPower := CombatPowerWithCatalog(w.Catalog, *a)
	defPower := CombatPowerWithCatalog(w.Catalog, *b)
	ev := tx.record(a, x, "sparred", map[string]any{
		"text":           fmt.Sprintf("%s sparred with %s in martial arts practice (power %d vs %d)", a.Name, b.Name, atkPower, defPower),
		"attacker_power": atkPower,
		"defender_power": defPower,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	events := []Event{ev}
	events = append(events, w.grantCombatXP(tx, a, x, 6, 2)...)
	events = append(events, w.grantCombatXP(tx, b, x, 4, 1)...)
	return events
}

func handleStealAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "target is not nearby")
	}
	if a.Health < 45 || w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "thief is injured or laying low to recuperate")
	}
	stealth, maxCoins := w.Catalog.StealthBonus(*a)
	if b.Task != nil && b.Task.Action.Kind == "sleep" {
		stealth += 4
	}
	vigilance := w.Catalog.VigilanceBonus(*b)
	if ClockAt(w.Tick).IsNight && w.IsCurfewActiveAt(a.Location, a.VenueID) {
		vigilance += 8
	}
	stealProf := w.Catalog.BestProfessionForCapability(*a, "steal")
	stolenCoins := minInt(b.Money, maxCoins)
	couldSteal := stealth >= vigilance && (b.Money > 0 || len(b.Items) > 0)
	victimAwake := w.partyMateAvailable(b)
	if couldSteal && victimAwake {
		preview := w.previewOffenseAssessment(*b, *a, "steal", 14, stolenCoins, b.Money, 0, "firm")
		couldSteal = preview.Outcome != "breaking_point"
	}
	demand := stolenCoins
	if !couldSteal && !victimAwake {
		demand = 0
	}
	offense := w.AssessAndRecordOffense(*b, *a, "steal", 14, demand, b.Money, 0, "firm")
	if couldSteal {
		if tx.TransferCurrency(b, a, "coins", int64(stolenCoins), x.Kind) {
			a.IllicitGain += stolenCoins
		}
		stolenItem := w.Catalog.SelectLootItem(*b)
		stolenQty := 0
		if stolenItem != "" {
			stolenQty = 1
			tx.TransferItem(b, a, stolenItem, 1, x.Kind)
		}
		w.Catalog.markOutlaw(a)
		a.RestUntil = w.Tick + 32
		tx.MutateTrust(b, a.ID, -2)
		var victimObservers []*Actor
		if w.partyMateAvailable(b) {
			victimObservers = append(victimObservers, b)
		}
		ev := tx.record(a, x, "stolen", map[string]any{
			"text":     fmt.Sprintf("%s stole %d coins and %d %s from %s", a.Name, stolenCoins, stolenQty, stolenItem, b.Name),
			"coins":    stolenCoins,
			"item":     stolenItem,
			"quantity": stolenQty,
			"offense":  offense,
		}, victimObservers...)
		events := []Event{ev}
		for _, sid := range sortedProductionKeys(a.Skills) {
			if a.Skills[sid] > 0 {
				if sk, ok := w.Catalog.Skills[sid]; ok && sk.StealthBonusPerLevel > 0 {
					events = append(events, w.grantSkillXP(tx, a, x, sid, 6)...)
				}
			}
		}
		if stealProf != "" {
			return append(events, w.grantProfessionXP(tx, a, x, stealProf, 6, 2)...)
		}
		return events
	}
	dmg := -tx.adjustHealth(a, -12, 1, x.Kind)
	w.Catalog.markOutlaw(a)
	a.RestUntil = w.Tick + 64
	tx.MutateTrust(b, a.ID, -3)
	ev := tx.record(a, x, "steal_foiled", map[string]any{
		"text":    fmt.Sprintf("%s was caught trying to steal from %s (-%d health)", a.Name, b.Name, dmg),
		"damage":  dmg,
		"offense": offense,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	if stealProf != "" {
		return append([]Event{ev}, w.grantProfessionXP(tx, a, x, stealProf, 2, 1)...)
	}
	return []Event{ev}
}

func handleRaidAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "target is not nearby")
	}
	if a.Health < 55 || w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "raider is injured or recuperating after recent raid")
	}
	atkPower := CombatPowerWithCatalog(w.Catalog, *a)
	defPower := CombatPowerWithCatalog(w.Catalog, *b)
	profID := w.Catalog.BestProfessionForCapability(*a, "raid")
	w.Catalog.markOutlaw(a)
	if atkPower >= defPower {
		wantDmg := minInt(35, 14+(atkPower-defPower)/2)
		dmg := -tx.adjustHealth(b, -wantDmg, 0, x.Kind)
		if b.Task != nil && b.Task.Action.Kind == "sleep" {
			b.Task = nil
		}
		if defPower >= 10 && a.Health > 30 {
			tx.adjustHealth(a, -4, 30, x.Kind)
		}
		a.RestUntil = w.Tick + 64
		if b.RestUntil < w.Tick+48 {
			b.RestUntil = w.Tick + 48
		}
		maxCoins := 3 + a.Professions[profID]*2
		lootCoins := minInt(b.Money, maxCoins)
		offense := w.AssessAndRecordOffense(*b, *a, "raid", 25+dmg, lootCoins, b.Money, 0, "hostile")
		if tx.TransferCurrency(b, a, "coins", int64(lootCoins), x.Kind) {
			a.IllicitGain += lootCoins
		}
		lootItem := w.Catalog.SelectLootItem(*b)
		lootQty := 0
		if lootItem != "" {
			lootQty = 1
			tx.TransferItem(b, a, lootItem, 1, x.Kind)
		}
		tx.MutateTrust(b, a.ID, -4)
		ev := tx.record(a, x, "raided", map[string]any{
			"text":     fmt.Sprintf("%s raided %s for %d coins and %d %s (%d damage)", a.Name, b.Name, lootCoins, lootQty, lootItem, dmg),
			"coins":    lootCoins,
			"item":     lootItem,
			"quantity": lootQty,
			"offense":  offense,
			"damage":   dmg,
		}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
		events := []Event{ev}
		if profID != "" {
			events = append(events, w.grantProfessionXP(tx, a, x, profID, 8, 3)...)
		}
		events = append(events, w.markDeadIfZeroWithTx(tx, b, a, x, "armed raid", ev)...)
		return events
	}
	w.AssessAndRecordOffense(*b, *a, "raid", 25, 0, b.Money, 0, "hostile")
	wantDmg := minInt(40, 16+(defPower-atkPower)/2)
	dmg := -tx.adjustHealth(a, -wantDmg, 0, x.Kind)
	a.RestUntil = w.Tick + 96
	tx.MutateTrust(b, a.ID, -3)
	ev := tx.record(a, x, "raid_repelled", map[string]any{
		"text":   fmt.Sprintf("%s repelled a raid by %s (-%d health to attacker)", b.Name, a.Name, dmg),
		"damage": dmg,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	events := []Event{ev}
	events = append(events, w.grantCombatXP(tx, b, x, 6, 2)...)
	events = append(events, w.markDeadIfZeroWithTx(tx, a, b, x, "repelled raid clash", ev)...)
	return events
}

func handleExtortAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	env := w.Catalog.EffectiveEnvironment()
	if !env.Institutions.RacketeeringEnabled() {
		return actionFail(tx, a, x, "protection extortion is disabled in this world environment")
	}
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "target is not nearby")
	}
	if w.Catalog.IsOutlawTags(b.IdentityTags()) {
		return actionFail(tx, a, x, "cannot extort fellow outlaw")
	}
	if a.Health < 50 || w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "enforcer is injured or resting")
	}
	w.Catalog.markOutlaw(a)
	atkPower := CombatPowerWithCatalog(w.Catalog, *a) + 2
	defPower := CombatPowerWithCatalog(w.Catalog, *b)
	fee := w.extortionDemand(*a, *b)
	offense := w.AssessAndRecordOffense(*b, *a, "extort", 15, fee, b.Money, 0, "firm")
	if atkPower >= defPower && b.Money >= 2 && offense.Outcome != "breaking_point" {
		if tx.TransferCurrency(b, a, "coins", int64(fee), x.Kind) {
			a.IllicitGain += fee
		}
		tx.MutateTrust(b, a.ID, -2)
		a.RestUntil = w.Tick + 32
		w.addDistrictCorruption(a.Location, 1)
		ev := tx.record(a, x, "extorted", map[string]any{
			"text":             fmt.Sprintf("%s collected %d coercion-fee coins from %s in %s", a.Name, fee, b.Name, a.Location),
			"protection_coins": fee,
			"coins":            fee,
			"offense":          offense,
			"district":         a.Location,
		}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
		events := []Event{ev}
		if extProf := w.Catalog.BestProfessionForCapability(*a, "extort"); extProf != "" {
			events = append(events, w.grantProfessionXP(tx, a, x, extProf, 5, 2)...)
		}
		return events
	}
	dmg := -tx.adjustHealth(a, -8, 1, x.Kind)
	a.RestUntil = w.Tick + 48
	tx.MutateTrust(b, a.ID, -2)
	ev := tx.record(a, x, "extort_resisted", map[string]any{
		"text":    fmt.Sprintf("%s refused extortion from %s (-%d health to attacker)", b.Name, a.Name, dmg),
		"damage":  dmg,
		"offense": offense,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	return []Event{ev}
}

func handleBribeAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "official is not nearby")
	}
	var hushSecret *ActorSecret
	for i := range w.Secrets {
		secret := &w.Secrets[i]
		if secret.OwnerID == a.ID && !secret.Exposed && ActorHasTag(secret.WitnessedBy, b.ID) &&
			(x.Item == "" || x.Item == secret.ID) {
			hushSecret = secret
			break
		}
	}
	if hushSecret == nil && !w.ActorCapabilities(*b)["arrest"] {
		return actionFail(tx, a, x, "recipient is neither an official nor a witness to the secret")
	}
	if !w.partyMateAvailable(b) {
		return actionFail(tx, a, x, "recipient cannot accept the bribe")
	}
	if a.Money < 4 {
		return actionFail(tx, a, x, "insufficient coins to offer bribe")
	}
	bribeCoins := minInt(a.Money, minInt(a.Money/3+2, 10))
	if !tx.TransferCurrency(a, b, "coins", int64(bribeCoins), x.Kind) {
		return actionFail(tx, a, x, "recipient cannot hold the bribe")
	}
	b.IllicitGain += bribeCoins
	tx.MutateTrust(a, b.ID, 2)
	tx.MutateTrust(b, a.ID, 3)
	w.addDistrictCorruption(a.Location, 2)
	if w.DistrictStates != nil {
		if ds, hasDS := w.DistrictStates[a.Location]; hasDS {
			if ds.BribeShieldTicks < 48 {
				ds.BribeShieldTicks = 48
			}
			w.DistrictStates[a.Location] = ds
		}
	}
	observers := append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)
	ev := tx.record(a, x, "bribed", map[string]any{
		"text":        fmt.Sprintf("%s slipped %d hush-bribe coins to %s in %s", a.Name, bribeCoins, b.Name, a.Location),
		"bribe_coins": bribeCoins,
		"district":    a.Location,
		"observation": Observation{
			Kind:      "corruption",
			SubjectID: b.ID,
			ObjectID:  a.ID,
			Item:      "coins",
			Location:  a.Location,
			Quantity:  bribeCoins,
			Proceeds:  bribeCoins,
		},
	}, observers...)
	if hushSecret != nil {
		hushSecret.HolderStances[b.ID] = "paid_silence"
		ev.Payload["silenced_secret_id"] = hushSecret.ID
	}
	return []Event{ev}
}

func handleArrestAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	env := w.Catalog.EffectiveEnvironment()
	if !env.Institutions.EnforcementEnabled() {
		return actionFail(tx, a, x, "enforcement detention is disabled in this world environment")
	}
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "target is not nearby")
	}
	cc := w.personalCase(a.Location, b.ID, "wanted")
	if cc == nil {
		return actionFail(tx, a, x, "no open warrant in this district")
	}
	x.BasisEventID = cc.BasisEventID
	if a.Health < 45 || w.Tick < a.RestUntil {
		return actionFail(tx, a, x, "enforcer is injured or resting from recent patrol clash")
	}
	lawPower := CombatPowerWithCatalog(w.Catalog, *a) + 3
	usedWrit := false
	if env.EnforcementItem != "" && a.Items[env.EnforcementItem] > 0 {
		lawPower += 5
		usedWrit = true
	}
	outlawPower := CombatPowerWithCatalog(w.Catalog, *b)
	lawProf := w.Catalog.BestProfessionForCapability(*a, "arrest")
	if lawPower >= outlawPower {
		dmg := -tx.adjustHealth(b, -12, 1, x.Kind)
		b.Task = nil
		tx.MoveActor(b, b.Location, b.VenueID, "")
		a.RestUntil = w.Tick + 16
		baseOfficial := int64(4)
		if usedWrit {
			baseOfficial += 3
			tx.ProduceOrConsumeItem(a, env.EnforcementItem, -1, x.Kind)
		}
		if w.hasActiveCohorts() {
			baseOfficial = tx.debitTreasury(baseOfficial, "bounty")
			tx.moveCurrency(a, "coins", baseOfficial, "treasury", x.Kind)
		} else {
			tx.MintOrBurnCurrency(a, "coins", baseOfficial, x.Kind)
		}
		if b.Health > 0 {
			tx.setSentence(b, w.Tick+w.effectiveJusticeSpec(w.JusticeRevolt).TrialSentenceTicks)
			cc.Status = "arrested"
			cc.ArrestEventID = w.NextID
			cc.JailedUntilTick = b.JailedUntil
			filtered := w.Offers[:0]
			for _, o := range w.Offers {
				if o.Seller != b.ID && o.Buyer != b.ID {
					filtered = append(filtered, o)
				}
			}
			w.Offers = filtered
			if len(w.Offers) == 0 {
				w.Offers = nil
			}
		}
		ev := tx.record(a, x, "arrested", map[string]any{
			"text":           fmt.Sprintf("%s detained %s, confining until tick %d and claiming %d bounty (%d damage)", a.Name, b.Name, b.JailedUntil, baseOfficial, dmg),
			"coins":          baseOfficial,
			"case_id":        cc.ID,
			"root_event_id":  cc.RootEventID,
			"cause_event_id": cc.WarrantEventID,
			"damage":         dmg,
			"jailed_until":   b.JailedUntil,
		}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
		events := []Event{ev}
		if lawProf != "" {
			events = append(events, w.grantProfessionXP(tx, a, x, lawProf, 8, 3)...)
		}
		events = append(events, w.markDeadIfZeroWithTx(tx, b, a, x, "violent arrest resistance", ev)...)
		return events
	}
	dmg := -tx.adjustHealth(a, -14, 0, x.Kind)
	a.RestUntil = w.Tick + 64
	ev := tx.record(a, x, "arrest_resisted", map[string]any{
		"text":   fmt.Sprintf("%s resisted detention by %s (-%d health)", b.Name, a.Name, dmg),
		"damage": dmg,
	}, append([]*Actor{b}, w.awakeSamePlaceObservers(*a, a.ID, b.ID)...)...)
	events := []Event{ev}
	events = append(events, w.grantProfessionXP(tx, a, x, lawProf, 3, 1)...)
	events = append(events, w.markDeadIfZeroWithTx(tx, a, b, x, "resisting outlaw counterattack", ev)...)
	return events
}

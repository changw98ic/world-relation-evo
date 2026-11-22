package sim

import "fmt"

func registerTradeSocialActions() {
	registerBuiltinAction("join_faction", handleJoinFactionAction)
	registerBuiltinAction("join_profession", handleJoinProfessionAction)
	registerBuiltinAction("practice_skill", handlePracticeSkillAction)
	registerBuiltinAction("speak", handleSpeakAction)
	registerBuiltinAction("offer", handleOfferAction)
	registerBuiltinAction("accept", handleAcceptOrRejectOfferAction)
	registerBuiltinAction("reject", handleAcceptOrRejectOfferAction)
}

func handleJoinFactionAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	f := w.faction(x.FactionID)
	if f == nil {
		return actionFail(tx, a, x, "unknown faction: "+x.FactionID)
	}
	if reason := w.FactionJoinEligibilityReason(*a, *f); reason != "" {
		return actionFail(tx, a, x, reason)
	}
	if a.InsideHouse != "" || a.Location != f.District {
		return actionFail(tx, a, x, "joining "+f.Name+" requires standing at the guild hall in "+f.District)
	}
	f.Members = append(f.Members, a.ID)
	a.Guilds = appendUnique(a.Guilds, f.ID)
	w.refreshActor(a)
	return actionEmit(tx, a, x, "faction_joined", fmt.Sprintf("%s joined %s (level %d)", a.Name, f.Name, f.Level))
}

func handleJoinProfessionAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	prof, ok := w.Catalog.ProfessionByID(x.ProfessionID)
	if !ok {
		return actionFail(tx, a, x, "unknown profession: "+x.ProfessionID)
	}
	f := w.faction(prof.Guild)
	if f == nil || !f.HasMember(a.ID) {
		return actionFail(tx, a, x, "must join the "+prof.Guild+" guild before learning "+prof.Name)
	}
	if a.InsideHouse != "" || a.Location != f.District {
		return actionFail(tx, a, x, "learning "+prof.Name+" requires the guild hall in "+f.District)
	}
	if ok, reason := w.Catalog.CanLearnProfession(*a, prof); !ok {
		return actionFail(tx, a, x, reason)
	}
	if a.Professions == nil {
		a.Professions = map[string]int{}
	}
	a.Professions[prof.ID] = 1
	for _, grant := range prof.Grants {
		a.Tags = appendUnique(a.Tags, grant)
	}
	w.refreshActor(a)
	rank := prof.RankTitle(1)
	payload := map[string]any{
		"text":            fmt.Sprintf("%s learned %s (%s, tier %d, level 1, character level %d)", a.Name, prof.Name, rank, prof.Tier, a.Level),
		"profession":      prof.ID,
		"tier":            prof.Tier,
		"rank":            rank,
		"character_level": a.Level,
	}
	if prof.ParentProfession != "" {
		payload["parent_profession"] = prof.ParentProfession
		payload["text"] = fmt.Sprintf("%s advanced from %s to %s (%s, tier %d, level 1, character level %d)", a.Name, prof.ParentProfession, prof.Name, rank, prof.Tier, a.Level)
	}
	return []Event{tx.record(a, x, "profession_joined", payload)}
}

func handlePracticeSkillAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	spec, ok := w.Catalog.Skills[x.SkillID]
	if !ok || a.Skills[x.SkillID] == 0 || a.Skills[x.SkillID] >= spec.EffectiveMaxLevel() || a.Energy < 4 {
		return actionFail(tx, a, x, "practice requires a known skill below its maximum level and four energy")
	}
	tx.setEnergy(a, a.Energy-4, x.Kind)
	events := actionEmit(tx, a, x, "skill_practiced", fmt.Sprintf("%s practised %s", a.Name, spec.Name))
	return append(events, w.grantSkillXP(tx, a, x, x.SkillID, 4)...)
}

func handleSpeakAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "listener is not nearby")
	}
	if x.Text == "" {
		return actionFail(tx, a, x, "empty speech")
	}
	if b.Task != nil && (b.Task.Action.Kind == "sleep" || b.Task.Dormant) {
		return actionFail(tx, a, x, "listener cannot hear speech")
	}
	ev := tx.record(a, x, "speech", map[string]any{"text": x.Text, "delivered_to": []string{b.ID}}, b)
	return append([]Event{ev}, w.deliverSecretReport(x, payloadInt64(ev.Payload, "fact_event_id"), []*Actor{b})...)
}

func handleOfferAction(tx *WorldTx, a, b *Actor, x Action) []Event {
	w := tx.World
	if x.OfferID != 0 {
		return w.counterOffer(tx, a, x)
	}
	if b == nil || b.ID == a.ID || !SamePlace(*a, *b) || w.Tick < b.JailedUntil {
		return actionFail(tx, a, x, "buyer is not nearby")
	}
	spec, ok := w.Catalog.Items[x.Item]
	if !ok || !spec.Tradeable {
		return actionFail(tx, a, x, "item is not tradeable: "+x.Item)
	}
	if w.IsItemEmbargoedAt(a.Location, a.VenueID, x.Item) {
		return actionFail(tx, a, x, "item is embargoed by local polity decree: "+x.Item)
	}
	cur := x.Currency
	if cur == "" {
		cur = "coins"
	}
	if !itemAcceptsCurrency(spec, cur) {
		return actionFail(tx, a, x, fmt.Sprintf("item %s does not accept currency %s", x.Item, cur))
	}
	if spec.BuyerRequiredTag != "" && !ActorHasTag(w.actorEffectiveTags(*b), spec.BuyerRequiredTag) {
		return actionFail(tx, a, x, fmt.Sprintf("buyer %s lacks required tag %s for %s", b.ID, spec.BuyerRequiredTag, x.Item))
	}
	if x.Quantity <= 0 || x.Price < 0 || a.Items[x.Item] < x.Quantity {
		return actionFail(tx, a, x, "invalid offer or insufficient item stock")
	}
	offerText := fmt.Sprintf("%s offers %d %s (tier %d) for %d %s", a.Name, x.Quantity, x.Item, spec.Tier, x.Price, cur)
	e := tx.record(a, x, "trade_offered", map[string]any{
		"text":     offerText,
		"currency": cur,
	}, b)
	w.Offers = append(w.Offers, Offer{
		ID:       e.ID,
		Seller:   a.ID,
		Buyer:    b.ID,
		Item:     x.Item,
		Quantity: x.Quantity,
		Price:    x.Price,
		Currency: x.Currency,
		Expires:  w.Tick + 12,
	})
	return []Event{e}
}

func handleAcceptOrRejectOfferAction(tx *WorldTx, a, _ *Actor, x Action) []Event {
	w := tx.World
	for i, o := range w.Offers {
		if o.ID != x.OfferID || o.ResponderID() != a.ID {
			continue
		}
		seller := w.actor(o.Seller)
		buyer := w.actor(o.Buyer)
		x.TargetID = o.ProposerID()
		if x.Kind == "reject" {
			w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
			if len(w.Offers) == 0 {
				w.Offers = nil
			}
			return []Event{tx.record(a, x, "trade_rejected", map[string]any{
				"text":           a.Name + " rejected offer",
				"cause_event_id": o.ID,
				"root_event_id":  o.ID,
			}, w.actor(o.ProposerID()))}
		}
		cur := o.Currency
		if cur == "" {
			cur = "coins"
		}
		if buyer == nil {
			return actionFail(tx, a, x, "buyer is no longer available")
		}
		if reason := w.offerSettlementBlockReason(*buyer, o); reason != "" {
			return actionFail(tx, a, x, reason)
		}
		tx.TransferItem(seller, buyer, o.Item, o.Quantity, x.Kind)
		taxCoins := w.offerTax(*buyer, o)
		pol := w.EffectiveGoverningPolity(buyer.Location, buyer.VenueID)
		if cur != "coins" && pol == nil {
			taxCoins = 0
		}
		if netSeller := o.Price - taxCoins; netSeller > 0 {
			tx.TransferCurrency(buyer, seller, cur, int64(netSeller), x.Kind)
		}
		if taxCoins > 0 {
			account := "treasury"
			if pol != nil {
				account = pol.ID
			}
			tx.moveCurrency(buyer, cur, -int64(taxCoins), account, x.Kind)
			if cur == "coins" {
				w.creditPolityTax(pol, int64(taxCoins))
			} else {
				w.addPolityCurrency(pol, cur, int64(taxCoins))
			}
		}
		tx.MutateTrust(buyer, seller.ID, 1)
		tx.MutateTrust(seller, buyer.ID, 1)
		w.Offers = append(w.Offers[:i], w.Offers[i+1:]...)
		if len(w.Offers) == 0 {
			w.Offers = nil
		}
		x.Item, x.Quantity, x.Price, x.Currency = o.Item, o.Quantity, o.Price, o.Currency
		if w.actorFactionRelation(*buyer, *seller) == "allied" {
			tx.MutateTrust(buyer, seller.ID, 1)
			tx.MutateTrust(seller, buyer.ID, 1)
		}
		events := []Event{tx.record(a, x, "trade_completed", map[string]any{
			"text":           fmt.Sprintf("%s bought %d %s from %s for %d %s", buyer.Name, o.Quantity, o.Item, seller.Name, o.Price, cur),
			"item":           o.Item,
			"quantity":       o.Quantity,
			"seller_id":      seller.ID,
			"price":          o.Price,
			"currency":       cur,
			"market_tax":     taxCoins,
			"cause_event_id": o.ID,
			"root_event_id":  o.ID,
		}, w.actor(o.ProposerID()))}
		tier := w.Catalog.Items[o.Item].Tier
		if tier < 1 {
			tier = 1
		}
		if _, hasPeddler := w.Catalog.ProfessionByID("peddler"); hasPeddler {
			events = append(events, w.grantProfessionXP(tx, seller, x, "peddler", tier*o.Quantity, tier)...)
			events = append(events, w.grantProfessionXP(tx, buyer, x, "peddler", tier*o.Quantity, tier)...)
		}
		return events
	}
	return actionFail(tx, a, x, "offer not found")
}

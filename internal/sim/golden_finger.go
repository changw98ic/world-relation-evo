package sim

import (
	"fmt"
	"sort"
	"strings"
)

// GoldenFingerSpec declaratively configures any web-novel "golden finger" (cheat/anomaly system)
// as a composable bundle of orthogonal engine primitives — passive modifiers, reactive event triggers,
// personal exchange/synthesis shop, quest/mission engine, foresight/prophet intel, and life simulator.
//
// Binding modes (any combination; if multiple fields are set, any match activates the golden finger):
//   - OwnerActorID: soul-bound to one named actor.
//   - GrantedByTags: active for actors carrying any matching identity/effect tag.
//   - GrantedByItems: active for any actor currently possessing a matching item (lootable/transferable,
//     e.g., an ancient mentor ring or pocket-realm jade pendant).
type GoldenFingerSpec struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description,omitempty"`
	OwnerActorID         string           `json:"owner_actor_id,omitempty"`
	GrantedByTags        []string         `json:"granted_by_tags,omitempty"`
	GrantedByItems       []string         `json:"granted_by_items,omitempty"`
	ExposureRiskPermille int              `json:"exposure_risk_permille,omitempty"`
	ExposureSecretKind   string           `json:"exposure_secret_kind,omitempty"` // default: "forbidden_hoard"
	Passive              *GFPassiveSpec   `json:"passive,omitempty"`
	Triggers             []GFTriggerSpec  `json:"triggers,omitempty"`
	Shop                 []GFShopItemSpec `json:"shop,omitempty"`
	Quests               []GFQuestSpec    `json:"quests,omitempty"`
	Foresights           []GFForesight    `json:"foresights,omitempty"`
	Simulator            *GFSimulatorSpec `json:"simulator,omitempty"`
}

// GFPassiveSpec models always-on golden-finger traits:
//   - Proficiency/panel XP multiplier (SkillXPMultiplierPct, e.g., 250 = 2.5x)
//   - Combat/defense empowerment (CombatBonus)
//   - Disguise / "playing pig to eat tiger" (DisguisePowerTier hides true realm from neighbors)
//   - Pocket realm / spirit-spring daily yield (DailyPocketYield)
//   - Ring-spirit / guardian emergency rescue (RescueBelowHealth / RescueHeal / RescueCooldownTicks)
type GFPassiveSpec struct {
	SkillXPMultiplierPct int           `json:"skill_xp_multiplier_pct,omitempty"`
	CombatBonus          int           `json:"combat_bonus,omitempty"`
	DisguisePowerTier    int           `json:"disguise_power_tier,omitempty"`
	DisguiseFormID       string        `json:"disguise_form_id,omitempty"`
	DailyPocketYield     *GFRewardSpec `json:"daily_pocket_yield,omitempty"`
	RescueBelowHealth    int           `json:"rescue_below_health,omitempty"`
	RescueHeal           int           `json:"rescue_heal,omitempty"`
	RescueCooldownTicks  int64         `json:"rescue_cooldown_ticks,omitempty"`
	RescueName           string        `json:"rescue_name,omitempty"`
}

// GFRewardSpec is a conservation-safe reward payload granted by triggers, shop exchanges, quests,
// pocket-realm yields, or life-simulator runs.
type GFRewardSpec struct {
	Coins       int              `json:"coins,omitempty"`
	Currencies  map[string]int64 `json:"currencies,omitempty"`
	Items       map[string]int   `json:"items,omitempty"`
	SkillXP     map[string]int   `json:"skill_xp,omitempty"`
	StateSlots  map[string]int   `json:"state_slots,omitempty"`
	Attributes  map[string]int   `json:"attributes,omitempty"` // strength|dexterity|insight|spirit
	HealthDelta int              `json:"health_delta,omitempty"`
	EnergyDelta int              `json:"energy_delta,omitempty"`
	GrantTags   []string         `json:"grant_tags,omitempty"`
}

// GFTriggerSpec declaratively reacts to simulation events or daily ticks, unifying:
//   - Daily & landmark sign-in/check-in (OnEvents:["day_start"] or ["moved"] + OncePerDistrict)
//   - Emotion/shock/resentment harvesting (MatchRole:"witnessed_by_others" + ScaleByObservers:true)
//   - Kill devouring / looting (OnEvents:["actor_died"], MatchRole:"self_killer", DevourSkill/DevourAttrPoints)
//   - Mentorship / investment / kinship multiplier rebate (MatchRole:"mentee_or_kin")
type GFTriggerSpec struct {
	ID                       string       `json:"id"`
	Name                     string       `json:"name"`
	OnEvents                 []string     `json:"on_events"` // event kinds or "day_start"
	MatchRole                string       `json:"match_role,omitempty"` // "self_actor"|"self_target"|"self_killer"|"witnessed_by_others"|"mentee_or_kin"|"any"
	RequiredDistrictFeatures []string     `json:"required_district_features,omitempty"`
	OncePerDistrict          bool         `json:"once_per_district,omitempty"`
	CooldownTicks            int64        `json:"cooldown_ticks,omitempty"`
	MaxTriggers              int          `json:"max_triggers,omitempty"`
	ScaleByObservers         bool         `json:"scale_by_observers,omitempty"`
	DevourSkill              bool         `json:"devour_skill,omitempty"`
	DevourAttrPoints         int          `json:"devour_attr_points,omitempty"`
	Reward                   GFRewardSpec `json:"reward,omitempty"`
}

// GFShopItemSpec defines one offer in a golden finger's system shop, multiverse exchange, or
// item-synthesis/upgrade forge (executed via Action{Kind:"gf_exchange", Text:gfID, Item:offerID}).
type GFShopItemSpec struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	CostCurrency  string         `json:"cost_currency,omitempty"`
	CostAmount    int64          `json:"cost_amount,omitempty"`
	CostCoins     int            `json:"cost_coins,omitempty"`
	CostItems     map[string]int `json:"cost_items,omitempty"`
	Reward        GFRewardSpec   `json:"reward"`
	StockLimit    int            `json:"stock_limit,omitempty"`
	CooldownTicks int64          `json:"cooldown_ticks,omitempty"`
}

// GFPenaltySpec defines failure penalties for timed system quests or infinite-flow missions.
type GFPenaltySpec struct {
	HealthLoss   int              `json:"health_loss,omitempty"`
	CoinsLoss    int              `json:"coins_loss,omitempty"`
	CurrencyLoss map[string]int64 `json:"currency_loss,omitempty"`
	Lethal       bool             `json:"lethal,omitempty"`
}

// GFQuestSpec defines a declarative mission issued by a golden finger (quest system / infinite flow).
// Supported Kind values:
//   - "reach_form": TargetID = form_id (or TargetValue = power_tier)
//   - "reach_skill": TargetID = skill_id, TargetValue = required level
//   - "eliminate_actor": TargetID = actor_id (must be dead)
//   - "acquire_item": TargetID = item_id, TargetValue = quantity (default 1)
//   - "visit_district": TargetID = district_id
//   - "accumulate_currency": TargetID = currency_id (or "coins"), TargetValue = amount
type GFQuestSpec struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Kind           string         `json:"kind"`
	TargetID       string         `json:"target_id,omitempty"`
	TargetValue    int            `json:"target_value,omitempty"`
	DeadlineTick   int64          `json:"deadline_tick,omitempty"`
	Reward         GFRewardSpec   `json:"reward,omitempty"`
	FailurePenalty *GFPenaltySpec `json:"failure_penalty,omitempty"`
}

// GFForesight defines prophetic / regressor memory intel revealed to the host during [RevealTick, TargetTick].
type GFForesight struct {
	ID            string `json:"id"`
	RevealTick    int64  `json:"reveal_tick,omitempty"`
	TargetTick    int64  `json:"target_tick,omitempty"`
	DistrictID    string `json:"district_id,omitempty"`
	Kind          string `json:"kind"` // "opportunity" | "danger"
	Summary       string `json:"summary"`
	TargetActorID string `json:"target_actor_id,omitempty"`
}

// GFSimulatorSpec configures a life-simulator / branch-deduction cheat executed via Action{Kind:"gf_simulate"}.
type GFSimulatorSpec struct {
	Name             string           `json:"name,omitempty"`
	CooldownTicks    int64            `json:"cooldown_ticks,omitempty"`
	CostCoins        int              `json:"cost_coins,omitempty"`
	CostCurrency     string           `json:"cost_currency,omitempty"`
	CostAmount       int64            `json:"cost_amount,omitempty"`
	RewardSkillXP    map[string]int   `json:"reward_skill_xp,omitempty"`
	RewardAttributes map[string]int   `json:"reward_attributes,omitempty"`
	RewardCurrencies map[string]int64 `json:"reward_currencies,omitempty"`
}

// GoldenFingerState stores deterministic runtime progress for all golden fingers on World.
type GoldenFingerState struct {
	TriggerCounts    map[string]int    `json:"trigger_counts,omitempty"`     // key: actor|gf|trigger
	TriggerLastTick  map[string]int64  `json:"trigger_last_tick,omitempty"`  // key: actor|gf|trigger
	VisitedDistricts map[string]bool   `json:"visited_districts,omitempty"`  // key: actor|gf|trigger|district
	ShopBoughtCounts map[string]int    `json:"shop_bought_counts,omitempty"` // key: actor|gf|offer
	ShopLastTick     map[string]int64  `json:"shop_last_tick,omitempty"`     // key: actor|gf|offer
	QuestStatus      map[string]string `json:"quest_status,omitempty"`       // key: actor|gf|quest -> "completed"|"failed"
	RescueLastTick   map[string]int64  `json:"rescue_last_tick,omitempty"`   // key: actor|gf
	PocketLastDay    map[string]int64  `json:"pocket_last_day,omitempty"`    // key: actor|gf
	SimLastTick      map[string]int64  `json:"sim_last_tick,omitempty"`      // key: actor|gf
	DynamicQuests    []GFQuestInstance `json:"dynamic_quests,omitempty"`     // quests added mid-run via ExternalPatch
}

// GFQuestInstance binds a dynamically injected quest to a specific golden finger ID.
type GFQuestInstance struct {
	GoldenFingerID string      `json:"golden_finger_id"`
	Quest          GFQuestSpec `json:"quest"`
}

// GoldenFingerView is the perception-safe snapshot exposed to an actor's Agent/Decider.
type GoldenFingerView struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	ActiveQuests     []GFQuestSpec    `json:"active_quests,omitempty"`
	ActiveForesights []GFForesight    `json:"active_foresights,omitempty"`
	AvailableShop    []GFShopItemSpec `json:"available_shop,omitempty"`
	SimulatorReady   bool             `json:"simulator_ready,omitempty"`
	SimulatorName    string           `json:"simulator_name,omitempty"`
}

func NewGoldenFingerState() *GoldenFingerState {
	return &GoldenFingerState{
		TriggerCounts:    map[string]int{},
		TriggerLastTick:  map[string]int64{},
		VisitedDistricts: map[string]bool{},
		ShopBoughtCounts: map[string]int{},
		ShopLastTick:     map[string]int64{},
		QuestStatus:      map[string]string{},
		RescueLastTick:   map[string]int64{},
		PocketLastDay:    map[string]int64{},
		SimLastTick:      map[string]int64{},
	}
}

func (w *World) ensureGoldenFingerState() *GoldenFingerState {
	if w.GoldenFinger == nil {
		w.GoldenFinger = NewGoldenFingerState()
	}
	st := w.GoldenFinger
	if st.TriggerCounts == nil {
		st.TriggerCounts = map[string]int{}
	}
	if st.TriggerLastTick == nil {
		st.TriggerLastTick = map[string]int64{}
	}
	if st.VisitedDistricts == nil {
		st.VisitedDistricts = map[string]bool{}
	}
	if st.ShopBoughtCounts == nil {
		st.ShopBoughtCounts = map[string]int{}
	}
	if st.ShopLastTick == nil {
		st.ShopLastTick = map[string]int64{}
	}
	if st.QuestStatus == nil {
		st.QuestStatus = map[string]string{}
	}
	if st.RescueLastTick == nil {
		st.RescueLastTick = map[string]int64{}
	}
	if st.PocketLastDay == nil {
		st.PocketLastDay = map[string]int64{}
	}
	if st.SimLastTick == nil {
		st.SimLastTick = map[string]int64{}
	}
	return st
}

func CloneGFRewardSpec(r GFRewardSpec) GFRewardSpec {
	out := r
	out.Currencies = cloneInt64Map(r.Currencies)
	out.Items = cloneIntMap(r.Items)
	out.SkillXP = cloneIntMap(r.SkillXP)
	out.StateSlots = cloneIntMap(r.StateSlots)
	out.Attributes = cloneIntMap(r.Attributes)
	if r.GrantTags != nil {
		out.GrantTags = append([]string(nil), r.GrantTags...)
	}
	return out
}

func CloneGoldenFingerSpec(g GoldenFingerSpec) GoldenFingerSpec {
	out := g
	if g.GrantedByTags != nil {
		out.GrantedByTags = append([]string(nil), g.GrantedByTags...)
	}
	if g.GrantedByItems != nil {
		out.GrantedByItems = append([]string(nil), g.GrantedByItems...)
	}
	if g.Passive != nil {
		pc := *g.Passive
		if g.Passive.DailyPocketYield != nil {
			yc := CloneGFRewardSpec(*g.Passive.DailyPocketYield)
			pc.DailyPocketYield = &yc
		}
		out.Passive = &pc
	}
	if g.Triggers != nil {
		out.Triggers = make([]GFTriggerSpec, len(g.Triggers))
		for i, tr := range g.Triggers {
			tc := tr
			if tr.OnEvents != nil {
				tc.OnEvents = append([]string(nil), tr.OnEvents...)
			}
			if tr.RequiredDistrictFeatures != nil {
				tc.RequiredDistrictFeatures = append([]string(nil), tr.RequiredDistrictFeatures...)
			}
			tc.Reward = CloneGFRewardSpec(tr.Reward)
			out.Triggers[i] = tc
		}
	}
	if g.Shop != nil {
		out.Shop = make([]GFShopItemSpec, len(g.Shop))
		for i, sh := range g.Shop {
			sc := sh
			sc.CostItems = cloneIntMap(sh.CostItems)
			sc.Reward = CloneGFRewardSpec(sh.Reward)
			out.Shop[i] = sc
		}
	}
	if g.Quests != nil {
		out.Quests = make([]GFQuestSpec, len(g.Quests))
		for i, q := range g.Quests {
			qc := q
			qc.Reward = CloneGFRewardSpec(q.Reward)
			if q.FailurePenalty != nil {
				fp := *q.FailurePenalty
				fp.CurrencyLoss = cloneInt64Map(q.FailurePenalty.CurrencyLoss)
				qc.FailurePenalty = &fp
			}
			out.Quests[i] = qc
		}
	}
	if g.Foresights != nil {
		out.Foresights = append([]GFForesight(nil), g.Foresights...)
	}
	if g.Simulator != nil {
		sc := *g.Simulator
		sc.RewardSkillXP = cloneIntMap(g.Simulator.RewardSkillXP)
		sc.RewardAttributes = cloneIntMap(g.Simulator.RewardAttributes)
		sc.RewardCurrencies = cloneInt64Map(g.Simulator.RewardCurrencies)
		out.Simulator = &sc
	}
	return out
}

// ActorHasGoldenFinger reports whether spec is currently active for actor a.
func (c Catalog) ActorHasGoldenFinger(a Actor, spec GoldenFingerSpec) bool {
	if a.Dead {
		return false
	}
	if spec.OwnerActorID != "" && a.ID == spec.OwnerActorID {
		return true
	}
	if len(spec.GrantedByTags) > 0 {
		tags := actorEffectiveTags(c, a)
		for _, gt := range spec.GrantedByTags {
			if ActorHasTag(tags, gt) {
				return true
			}
		}
	}
	for _, itemID := range spec.GrantedByItems {
		if a.Items[itemID] > 0 {
			return true
		}
	}
	return false
}

// ActiveGoldenFingersFor returns all GoldenFingerSpecs currently active for actor a.
func (c Catalog) ActiveGoldenFingersFor(a Actor) []GoldenFingerSpec {
	if len(c.GoldenFingers) == 0 || a.Dead {
		return nil
	}
	var out []GoldenFingerSpec
	for _, gf := range c.GoldenFingers {
		if c.ActorHasGoldenFinger(a, gf) {
			out = append(out, gf)
		}
	}
	return out
}

// GoldenFingerSkillXPMultiplier returns the highest skill XP multiplier percentage (default 100).
func (c Catalog) GoldenFingerSkillXPMultiplier(a Actor) int {
	mult := 100
	for _, gf := range c.ActiveGoldenFingersFor(a) {
		if gf.Passive != nil && gf.Passive.SkillXPMultiplierPct > mult {
			mult = gf.Passive.SkillXPMultiplierPct
		}
	}
	return mult
}

// GoldenFingerCombatBonus sums passive combat bonuses from all active golden fingers.
func (c Catalog) GoldenFingerCombatBonus(a Actor) int {
	bonus := 0
	for _, gf := range c.ActiveGoldenFingersFor(a) {
		if gf.Passive != nil && gf.Passive.CombatBonus > 0 {
			bonus += gf.Passive.CombatBonus
		}
	}
	return bonus
}

// PerceivedFormID returns the actor's apparent FormID to outsiders, honoring DisguiseFormID / DisguisePowerTier.
func (c Catalog) PerceivedFormID(a Actor) string {
	trueTier := c.ActorPowerTier(a)
	disguiseTier := 0
	for _, gf := range c.ActiveGoldenFingersFor(a) {
		if gf.Passive == nil {
			continue
		}
		if gf.Passive.DisguiseFormID != "" {
			return gf.Passive.DisguiseFormID
		}
		if gf.Passive.DisguisePowerTier > 0 && gf.Passive.DisguisePowerTier < trueTier {
			if disguiseTier == 0 || gf.Passive.DisguisePowerTier < disguiseTier {
				disguiseTier = gf.Passive.DisguisePowerTier
			}
		}
	}
	if disguiseTier == 0 {
		return a.FormID
	}
	// Deterministically select a matching lower-tier form so the true high-tier FormID is never leaked.
	formKeys := make([]string, 0, len(c.Forms))
	for k := range c.Forms {
		formKeys = append(formKeys, k)
	}
	sort.Strings(formKeys)
	bestID := ""
	bestTier := -1
	for _, k := range formKeys {
		f := c.Forms[k]
		if f.PowerTier <= disguiseTier && f.PowerTier > bestTier {
			bestTier = f.PowerTier
			bestID = f.ID
			if bestID == "" {
				bestID = k
			}
		}
	}
	return bestID
}

// PerceivedPowerTier returns the actor's apparent power tier to outsiders, honoring DisguisePowerTier ("扮猪吃虎").
func (c Catalog) PerceivedPowerTier(a Actor) int {
	tier := c.ActorPowerTier(a)
	for _, gf := range c.ActiveGoldenFingersFor(a) {
		if gf.Passive != nil && gf.Passive.DisguisePowerTier > 0 && gf.Passive.DisguisePowerTier < tier {
			tier = gf.Passive.DisguisePowerTier
		}
	}
	return tier
}

// PerceivedCombatPower returns the actor's apparent combat power to outsiders when disguised.
func (c Catalog) PerceivedCombatPower(a Actor) int {
	power := CombatPowerWithCatalog(c, a)
	for _, gf := range c.ActiveGoldenFingersFor(a) {
		if gf.Passive != nil && gf.Passive.DisguisePowerTier > 0 && gf.Passive.DisguisePowerTier < c.ActorPowerTier(a) {
			disguised := 10 + gf.Passive.DisguisePowerTier*6
			if disguised < power {
				power = disguised
			}
		}
	}
	return power
}

func (w *World) goldenFingerQuestsFor(gf GoldenFingerSpec) []GFQuestSpec {
	out := append([]GFQuestSpec(nil), gf.Quests...)
	if w.GoldenFinger != nil {
		for _, dq := range w.GoldenFinger.DynamicQuests {
			if dq.GoldenFingerID == gf.ID {
				out = append(out, dq.Quest)
			}
		}
	}
	return out
}

func (w *World) canAffordGFShopOffer(a Actor, gf GoldenFingerSpec, offer GFShopItemSpec) bool {
	st := w.GoldenFinger
	key := a.ID + "|" + gf.ID + "|" + offer.ID
	if st != nil {
		if offer.StockLimit > 0 && st.ShopBoughtCounts[key] >= offer.StockLimit {
			return false
		}
		if offer.CooldownTicks > 0 {
			if last, ok := st.ShopLastTick[key]; ok && w.Tick-last < offer.CooldownTicks {
				return false
			}
		}
	}
	if offer.CostCoins > 0 && a.Money < offer.CostCoins {
		return false
	}
	if offer.CostCurrency != "" && offer.CostAmount > 0 && a.Currencies[offer.CostCurrency] < offer.CostAmount {
		return false
	}
	for itemID, qty := range offer.CostItems {
		if qty > 0 && a.Items[itemID] < qty {
			return false
		}
	}
	return true
}

func (w *World) canRunGFSimulator(a Actor, gf GoldenFingerSpec) bool {
	if gf.Simulator == nil {
		return false
	}
	simSpec := gf.Simulator
	if w.GoldenFinger != nil && simSpec.CooldownTicks > 0 {
		key := a.ID + "|" + gf.ID
		if last, ok := w.GoldenFinger.SimLastTick[key]; ok && w.Tick-last < simSpec.CooldownTicks {
			return false
		}
	}
	if simSpec.CostCoins > 0 && a.Money < simSpec.CostCoins {
		return false
	}
	if simSpec.CostCurrency != "" && simSpec.CostAmount > 0 && a.Currencies[simSpec.CostCurrency] < simSpec.CostAmount {
		return false
	}
	return true
}

// goldenFingerViewsFor builds the perception-safe GoldenFingerView list for actor a.
func (w *World) goldenFingerViewsFor(a Actor) []GoldenFingerView {
	active := w.Catalog.ActiveGoldenFingersFor(a)
	if len(active) == 0 {
		return nil
	}
	st := w.GoldenFinger
	views := make([]GoldenFingerView, 0, len(active))
	for _, gf := range active {
		v := GoldenFingerView{
			ID:   gf.ID,
			Name: gf.Name,
		}
		for _, q := range w.goldenFingerQuestsFor(gf) {
			qKey := a.ID + "|" + gf.ID + "|" + q.ID
			if st == nil || st.QuestStatus[qKey] == "" {
				v.ActiveQuests = append(v.ActiveQuests, q)
			}
		}
		for _, fs := range gf.Foresights {
			if w.Tick >= fs.RevealTick && (fs.TargetTick <= 0 || w.Tick <= fs.TargetTick) {
				v.ActiveForesights = append(v.ActiveForesights, fs)
			}
		}
		for _, sh := range gf.Shop {
			if w.canAffordGFShopOffer(a, gf, sh) {
				v.AvailableShop = append(v.AvailableShop, sh)
			}
		}
		if gf.Simulator != nil && w.canRunGFSimulator(a, gf) {
			v.SimulatorReady = true
			v.SimulatorName = gf.Simulator.Name
			if v.SimulatorName == "" {
				v.SimulatorName = gf.Name + "推演"
			}
		}
		views = append(views, v)
	}
	return views
}

// applyGFReward deterministically applies a GFRewardSpec to actor a with strict ledger conservation.
func (w *World) applyGFReward(a *Actor, x Action, r GFRewardSpec, multiplier int) (string, []Event) {
	if multiplier < 1 {
		multiplier = 1
	}
	var parts []string
	var extraEvents []Event

	if r.Coins > 0 {
		gain := r.Coins * multiplier
		a.Money += gain
		w.recordLedgerCurrencyProduced("coins", int64(gain))
		parts = append(parts, fmt.Sprintf("+%d coins", gain))
	}
	if len(r.Currencies) > 0 {
		if a.Currencies == nil {
			a.Currencies = map[string]int64{}
		}
		keys := make([]string, 0, len(r.Currencies))
		for k := range r.Currencies {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, cur := range keys {
			amt := r.Currencies[cur] * int64(multiplier)
			if amt <= 0 {
				continue
			}
			a.Currencies[cur] += amt
			w.recordLedgerCurrencyProduced(cur, amt)
			parts = append(parts, fmt.Sprintf("+%d %s", amt, cur))
		}
	}
	if len(r.Items) > 0 {
		if a.Items == nil {
			a.Items = map[string]int{}
		}
		keys := sortedProductionKeys(r.Items)
		for _, itemID := range keys {
			qty := r.Items[itemID] * multiplier
			if qty <= 0 {
				continue
			}
			if _, ok := w.Catalog.Items[itemID]; !ok {
				w.Catalog.Items[itemID] = ItemSpec{ID: itemID, Name: itemID, Tier: 3, Tradeable: true, MassUnits: 1, BatchQty: 1, BatchPrice: 20}
			}
			a.Items[itemID] += qty
			w.recordLedgerItemProduced(itemID, int64(qty))
			parts = append(parts, fmt.Sprintf("+%d %s", qty, itemID))
		}
	}
	if len(r.SkillXP) > 0 {
		keys := sortedProductionKeys(r.SkillXP)
		for _, sid := range keys {
			xp := r.SkillXP[sid] * multiplier
			if xp <= 0 {
				continue
			}
			extraEvents = append(extraEvents, w.grantSkillXP(a, x, sid, xp)...)
			parts = append(parts, fmt.Sprintf("+%d %s XP", xp, sid))
		}
	}
	if len(r.StateSlots) > 0 {
		if a.StateSlots == nil {
			a.StateSlots = map[string]int{}
		}
		keys := sortedProductionKeys(r.StateSlots)
		for _, sl := range keys {
			delta := r.StateSlots[sl] * multiplier
			if delta == 0 {
				continue
			}
			next := a.StateSlots[sl] + delta
			if spec, ok := w.Catalog.StateSlots[sl]; ok {
				if next < spec.Min {
					next = spec.Min
				}
				if spec.Max > 0 && next > spec.Max {
					next = spec.Max
				}
			}
			a.StateSlots[sl] = next
			parts = append(parts, fmt.Sprintf("%+d %s", delta, sl))
		}
	}
	if len(r.Attributes) > 0 {
		keys := sortedProductionKeys(r.Attributes)
		for _, attr := range keys {
			delta := r.Attributes[attr] * multiplier
			if delta == 0 {
				continue
			}
			switch strings.ToLower(attr) {
			case "strength":
				a.Attributes.Strength += delta
			case "dexterity":
				a.Attributes.Dexterity += delta
			case "insight":
				a.Attributes.Insight += delta
			case "spirit":
				a.Attributes.Spirit += delta
			}
			parts = append(parts, fmt.Sprintf("%+d %s", delta, attr))
		}
	}
	if r.HealthDelta != 0 {
		a.Health = maxInt(1, minInt(MaxHealth, a.Health+r.HealthDelta*multiplier))
		parts = append(parts, fmt.Sprintf("%+d HP", r.HealthDelta*multiplier))
	}
	if r.EnergyDelta != 0 {
		a.Energy = maxInt(0, minInt(MaxEnergy, a.Energy+r.EnergyDelta*multiplier))
		parts = append(parts, fmt.Sprintf("%+d energy", r.EnergyDelta*multiplier))
	}
	for _, tag := range r.GrantTags {
		if tag != "" {
			a.Tags = appendUnique(a.Tags, tag)
		}
	}
	return strings.Join(parts, ", "), extraEvents
}

func (w *World) maybeExposeGoldenFinger(a *Actor, gf GoldenFingerSpec, causeEventID int64) []Event {
	if gf.ExposureRiskPermille <= 0 {
		return nil
	}
	observers := w.awakeSamePlaceObservers(*a, a.ID)
	if len(observers) == 0 {
		return nil
	}
	roll := int(worldHash(w.ID, w.Tick, "gf_expose:"+a.ID+":"+gf.ID) % 1000)
	if roll >= gf.ExposureRiskPermille {
		return nil
	}
	kind := gf.ExposureSecretKind
	if kind == "" {
		kind = "forbidden_hoard"
	}
	for _, s := range w.Secrets {
		if s.OwnerID == a.ID && s.Kind == kind && s.SubjectID == gf.ID && !s.Exposed {
			return nil
		}
	}
	knownBy := []string{a.ID}
	for _, obs := range observers {
		knownBy = appendUnique(knownBy, obs.ID)
	}
	sec := ActorSecret{
		ID:          fmt.Sprintf("sec_gf_%d_%s", w.NextID, a.ID),
		OwnerID:     a.ID,
		Kind:        kind,
		SubjectID:   gf.ID,
		Severity:    85,
		CreatedTick: w.Tick,
		KnownBy:     knownBy,
	}
	w.Secrets = append(w.Secrets, sec)
	ev := w.record(a, Action{Kind: "wait", BasisEventID: causeEventID}, "secret_caught_red_handed", map[string]any{
		"text":           fmt.Sprintf("Witnesses glimpsed %s's hidden anomaly [%s] at %s", a.Name, gf.Name, a.Location),
		"secret_id":      sec.ID,
		"secret_kind":    kind,
		"golden_finger":  gf.ID,
		"witnesses":      len(observers),
		"cause_event_id": causeEventID,
	}, observers...)
	return []Event{ev}
}

// applyGFExchange executes Action{Kind:"gf_exchange", Text:gfID, Item:offerID}.
func (w *World) applyGFExchange(a *Actor, x Action) []Event {
	fail := func(msg string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": msg})}
	}
	var matchedGF *GoldenFingerSpec
	var matchedOffer *GFShopItemSpec
	for _, gf := range w.Catalog.ActiveGoldenFingersFor(*a) {
		if x.Text != "" && gf.ID != x.Text {
			continue
		}
		for i := range gf.Shop {
			if gf.Shop[i].ID == x.Item {
				gCopy := gf
				oCopy := gf.Shop[i]
				matchedGF = &gCopy
				matchedOffer = &oCopy
				break
			}
		}
		if matchedOffer != nil {
			break
		}
	}
	if matchedGF == nil || matchedOffer == nil {
		return fail("golden finger shop offer not available")
	}
	if !w.canAffordGFShopOffer(*a, *matchedGF, *matchedOffer) {
		return fail("cannot afford or on cooldown for " + matchedOffer.Name)
	}
	st := w.ensureGoldenFingerState()
	key := a.ID + "|" + matchedGF.ID + "|" + matchedOffer.ID

	if matchedOffer.CostCoins > 0 {
		a.Money -= matchedOffer.CostCoins
		w.recordLedgerCurrencyConsumed("coins", int64(matchedOffer.CostCoins))
	}
	if matchedOffer.CostCurrency != "" && matchedOffer.CostAmount > 0 {
		a.Currencies[matchedOffer.CostCurrency] -= matchedOffer.CostAmount
		w.recordLedgerCurrencyConsumed(matchedOffer.CostCurrency, matchedOffer.CostAmount)
	}
	for _, itemID := range sortedProductionKeys(matchedOffer.CostItems) {
		qty := matchedOffer.CostItems[itemID]
		if qty > 0 {
			a.Items[itemID] -= qty
			w.recordLedgerItemConsumed(itemID, int64(qty))
		}
	}

	st.ShopBoughtCounts[key]++
	st.ShopLastTick[key] = w.Tick

	rewardText, extra := w.applyGFReward(a, x, matchedOffer.Reward, 1)
	ev := w.record(a, x, "gf_exchanged", map[string]any{
		"text":          fmt.Sprintf("%s redeemed [%s] via %s (%s)", a.Name, matchedOffer.Name, matchedGF.Name, rewardText),
		"golden_finger": matchedGF.ID,
		"offer_id":      matchedOffer.ID,
		"offer_name":    matchedOffer.Name,
		"reward":        rewardText,
	})
	events := append([]Event{ev}, extra...)
	events = append(events, w.maybeExposeGoldenFinger(a, *matchedGF, ev.ID)...)
	return events
}

// applyGFSimulate executes Action{Kind:"gf_simulate", Text:gfID} (life simulator / branch deduction).
func (w *World) applyGFSimulate(a *Actor, x Action) []Event {
	fail := func(msg string) []Event {
		return []Event{w.record(a, x, "action_failed", map[string]any{"text": msg})}
	}
	var matchedGF *GoldenFingerSpec
	for _, gf := range w.Catalog.ActiveGoldenFingersFor(*a) {
		if x.Text != "" && gf.ID != x.Text {
			continue
		}
		if gf.Simulator != nil {
			gCopy := gf
			matchedGF = &gCopy
			break
		}
	}
	if matchedGF == nil || matchedGF.Simulator == nil {
		return fail("no active simulator golden finger")
	}
	if !w.canRunGFSimulator(*a, *matchedGF) {
		return fail("simulator is on cooldown or unaffordable")
	}
	simSpec := matchedGF.Simulator
	if simSpec.CostCoins > 0 {
		a.Money -= simSpec.CostCoins
		w.recordLedgerCurrencyConsumed("coins", int64(simSpec.CostCoins))
	}
	if simSpec.CostCurrency != "" && simSpec.CostAmount > 0 {
		a.Currencies[simSpec.CostCurrency] -= simSpec.CostAmount
		w.recordLedgerCurrencyConsumed(simSpec.CostCurrency, simSpec.CostAmount)
	}
	st := w.ensureGoldenFingerState()
	st.SimLastTick[a.ID+"|"+matchedGF.ID] = w.Tick

	// Deduce immediate opportunities and mortal threats across the world.
	var insights []string
	for _, we := range w.ActiveWorldEvents {
		if we.ClaimedBy == "" && w.Tick < we.ExpiresTick {
			insights = append(insights, fmt.Sprintf("opportunity:%s@%s", we.Name, we.DistrictID))
		}
	}
	if w.Relations != nil {
		for _, g := range w.Relations.Grudges {
			if !g.Resolved && g.OffenderID == a.ID {
				if foe := w.actor(g.AggrievedID); foe != nil && !foe.Dead {
					insights = append(insights, fmt.Sprintf("threat:%s@%s", foe.Name, foe.Location))
				}
			}
		}
	}
	if len(insights) == 0 {
		insights = append(insights, "path_clear_for_cultivation")
	}

	rewardSpec := GFRewardSpec{
		SkillXP:    simSpec.RewardSkillXP,
		Attributes: simSpec.RewardAttributes,
		Currencies: simSpec.RewardCurrencies,
	}
	rewardText, extra := w.applyGFReward(a, x, rewardSpec, 1)
	simName := simSpec.Name
	if simName == "" {
		simName = matchedGF.Name
	}
	ev := w.record(a, x, "gf_simulated", map[string]any{
		"text":          fmt.Sprintf("%s ran [%s], foresaw (%s) and retained (%s)", a.Name, simName, strings.Join(insights, "; "), rewardText),
		"golden_finger": matchedGF.ID,
		"insights":      insights,
		"reward":        rewardText,
	})
	return append([]Event{ev}, extra...)
}

func (w *World) isMenteeOrKin(hostID, otherID string) bool {
	if hostID == "" || otherID == "" || hostID == otherID || w.Relations == nil {
		return false
	}
	for _, m := range w.Relations.Mentorships {
		if m.MentorID == hostID && m.ApprenticeID == otherID {
			return true
		}
	}
	if k := w.Relations.Kinship[hostID]; k != nil {
		if k.SpouseID == otherID || ActorHasTag(k.ChildIDs, otherID) || ActorHasTag(k.SwornIDs, otherID) {
			return true
		}
	}
	return false
}

func (w *World) isGFQuestSatisfied(a Actor, q GFQuestSpec) bool {
	switch q.Kind {
	case "reach_form":
		if q.TargetID != "" && a.FormID == q.TargetID {
			return true
		}
		if q.TargetValue > 0 && w.Catalog.ActorPowerTier(a) >= q.TargetValue {
			return true
		}
	case "reach_skill":
		req := q.TargetValue
		if req <= 0 {
			req = 2
		}
		return a.Skills[q.TargetID] >= req
	case "eliminate_actor":
		if t := w.actor(q.TargetID); t != nil && t.Dead {
			return true
		}
	case "acquire_item":
		req := q.TargetValue
		if req <= 0 {
			req = 1
		}
		return a.Items[q.TargetID] >= req
	case "visit_district":
		return a.Location == q.TargetID
	case "accumulate_currency":
		req := int64(q.TargetValue)
		if req <= 0 {
			req = 100
		}
		if q.TargetID == "" || q.TargetID == "coins" {
			return int64(a.Money) >= req
		}
		return a.Currencies[q.TargetID] >= req
	}
	return false
}

// stepGoldenFingers evaluates all declarative golden-finger passives, reactive triggers,
// emergency rescues, pocket-realm daily yields, and quest completions/expirations for the current tick.
func (w *World) stepGoldenFingers(tickEvents []Event) []Event {
	if len(w.Catalog.GoldenFingers) == 0 {
		return nil
	}
	st := w.ensureGoldenFingerState()
	dayNum := w.Tick / TicksPerDay

	var out []Event
	for ai := range w.Actors {
		a := &w.Actors[ai]
		if a.Dead {
			continue
		}
		activeGFs := w.Catalog.ActiveGoldenFingersFor(*a)
		if len(activeGFs) == 0 {
			continue
		}
		for _, gf := range activeGFs {
			// 1. Passive emergency guardian rescue (e.g. ancient ring spirit intervention).
			if gf.Passive != nil && gf.Passive.RescueBelowHealth > 0 && a.Health > 0 && a.Health <= gf.Passive.RescueBelowHealth {
				rKey := a.ID + "|" + gf.ID
				cd := gf.Passive.RescueCooldownTicks
				if cd <= 0 {
					cd = TicksPerDay
				}
				if last, ok := st.RescueLastTick[rKey]; !ok || w.Tick-last >= cd {
					st.RescueLastTick[rKey] = w.Tick
					heal := gf.Passive.RescueHeal
					if heal <= 0 {
						heal = 45
					}
					a.Health = minInt(MaxHealth, a.Health+heal)
					a.Energy = minInt(MaxEnergy, a.Energy+20)
					rName := gf.Passive.RescueName
					if rName == "" {
						rName = gf.Name + "护主"
					}
					ev := w.record(a, Action{Kind: "wait"}, "gf_rescue", map[string]any{
						"text":          fmt.Sprintf("[%s] intervened to rescue %s (+%d HP, now %d HP)", rName, a.Name, heal, a.Health),
						"golden_finger": gf.ID,
						"heal":          heal,
					})
					out = append(out, ev)
					out = append(out, w.maybeExposeGoldenFinger(a, gf, ev.ID)...)
				}
			}

			// 2. Passive daily pocket-realm / spirit-spring yield (once per simulated day).
			if gf.Passive != nil && gf.Passive.DailyPocketYield != nil {
				pKey := a.ID + "|" + gf.ID
				if lastDay, ok := st.PocketLastDay[pKey]; !ok || lastDay < dayNum {
					st.PocketLastDay[pKey] = dayNum
					rewardText, extra := w.applyGFReward(a, Action{Kind: "wait"}, *gf.Passive.DailyPocketYield, 1)
					if rewardText != "" {
						ev := w.record(a, Action{Kind: "wait"}, "gf_pocket_yield", map[string]any{
							"text":          fmt.Sprintf("%s harvested [%s] pocket realm yield (%s)", a.Name, gf.Name, rewardText),
							"golden_finger": gf.ID,
							"reward":        rewardText,
						})
						out = append(out, ev)
						out = append(out, extra...)
					}
				}
			}

			// 3. Reactive & scheduled triggers (sign-in, landmark check-in, emotion harvest, kill devour, mentee rebate).
			for _, tr := range gf.Triggers {
				tKey := a.ID + "|" + gf.ID + "|" + tr.ID
				if tr.MaxTriggers > 0 && st.TriggerCounts[tKey] >= tr.MaxTriggers {
					continue
				}
				if tr.CooldownTicks > 0 {
					if last, ok := st.TriggerLastTick[tKey]; ok && w.Tick-last < tr.CooldownTicks {
						continue
					}
				}
				// Check district feature restriction if any.
				if len(tr.RequiredDistrictFeatures) > 0 {
					featOK := false
					for _, feat := range tr.RequiredDistrictFeatures {
						if w.Map.DistrictHasFeature(a.Location, feat) {
							featOK = true
							break
						}
					}
					if !featOK {
						continue
					}
				}
				if tr.OncePerDistrict && st.VisitedDistricts[tKey+"|"+a.Location] {
					continue
				}

				// 3a. Daily check-in trigger ("day_start": fires once on each new simulated day).
				if ActorHasTag(tr.OnEvents, "day_start") {
					if lastTick, fired := st.TriggerLastTick[tKey]; !fired || lastTick/TicksPerDay < dayNum {
						st.TriggerCounts[tKey]++
						st.TriggerLastTick[tKey] = w.Tick
						if tr.OncePerDistrict {
							st.VisitedDistricts[tKey+"|"+a.Location] = true
						}
						rewardText, extra := w.applyGFReward(a, Action{Kind: "wait"}, tr.Reward, 1)
						ev := w.record(a, Action{Kind: "wait"}, "gf_triggered", map[string]any{
							"text":          fmt.Sprintf("%s triggered [%s / %s] at %s (%s)", a.Name, gf.Name, tr.Name, a.Location, rewardText),
							"golden_finger": gf.ID,
							"trigger_id":    tr.ID,
							"reward":        rewardText,
						})
						out = append(out, ev)
						out = append(out, extra...)
						continue
					}
				}

				// 3b. Event-driven triggers.
				for _, evSrc := range tickEvents {
					if !ActorHasTag(tr.OnEvents, evSrc.Kind) {
						continue
					}
					if tr.MaxTriggers > 0 && st.TriggerCounts[tKey] >= tr.MaxTriggers {
						break
					}
					if tr.OncePerDistrict && st.VisitedDistricts[tKey+"|"+a.Location] {
						break
					}
					matched := false
					mult := 1
					var victim *Actor

					role := tr.MatchRole
					if role == "" {
						role = "self_actor"
					}
					switch role {
					case "any":
						matched = true
					case "self_actor":
						matched = evSrc.ActorID == a.ID
					case "self_target":
						matched = evSrc.TargetID == a.ID
					case "self_killer":
						if evSrc.Kind == "actor_died" {
							killerID, _ := evSrc.Payload["killer_id"].(string)
							if killerID == a.ID || evSrc.TargetID == a.ID {
								matched = true
								victim = w.actor(evSrc.ActorID)
							}
						}
					case "witnessed_by_others":
						if evSrc.ActorID == a.ID {
							obsCount := len(w.awakeSamePlaceObservers(*a, a.ID))
							if obsCount > 0 {
								matched = true
								if tr.ScaleByObservers {
									mult = obsCount
								}
							}
						}
					case "mentee_or_kin":
						if w.isMenteeOrKin(a.ID, evSrc.ActorID) {
							matched = true
						}
					}
					if !matched {
						continue
					}

					st.TriggerCounts[tKey]++
					st.TriggerLastTick[tKey] = w.Tick
					if tr.OncePerDistrict {
						st.VisitedDistricts[tKey+"|"+a.Location] = true
					}
					actionCtx := Action{Kind: "wait", BasisEventID: evSrc.ID}
					rewardText, extra := w.applyGFReward(a, actionCtx, tr.Reward, mult)
					var devourParts []string
					if victim != nil {
						if tr.DevourAttrPoints > 0 {
							a.Attributes.Strength += tr.DevourAttrPoints
							a.Attributes.Spirit += tr.DevourAttrPoints
							devourParts = append(devourParts, fmt.Sprintf("devoured +%d STR/+%d SPI from %s", tr.DevourAttrPoints, tr.DevourAttrPoints, victim.Name))
						}
						if tr.DevourSkill && len(victim.Skills) > 0 {
							bestSkill := ""
							bestLv := 0
							for _, sid := range sortedProductionKeys(victim.Skills) {
								if victim.Skills[sid] > bestLv {
									bestLv = victim.Skills[sid]
									bestSkill = sid
								}
							}
							if bestSkill != "" {
								extra = append(extra, w.grantSkillXP(a, actionCtx, bestSkill, 12)...)
								devourParts = append(devourParts, fmt.Sprintf("devoured %s essence", bestSkill))
							}
						}
					}
					summaryParts := []string{}
					if rewardText != "" {
						summaryParts = append(summaryParts, rewardText)
					}
					summaryParts = append(summaryParts, devourParts...)
					ev := w.record(a, actionCtx, "gf_triggered", map[string]any{
						"text":           fmt.Sprintf("%s triggered [%s / %s] (%s)", a.Name, gf.Name, tr.Name, strings.Join(summaryParts, "; ")),
						"golden_finger":  gf.ID,
						"trigger_id":     tr.ID,
						"cause_event_id": evSrc.ID,
						"root_event_id":  evSrc.ID,
						"multiplier":     mult,
						"reward":         strings.Join(summaryParts, "; "),
					})
					out = append(out, ev)
					out = append(out, extra...)
					out = append(out, w.maybeExposeGoldenFinger(a, gf, ev.ID)...)
					break // fire each trigger at most once per tick
				}
			}

			// 4. Quest / Mission evaluation (completion rewards & deadline penalties).
			for _, q := range w.goldenFingerQuestsFor(gf) {
				qKey := a.ID + "|" + gf.ID + "|" + q.ID
				if st.QuestStatus[qKey] != "" {
					continue
				}
				if w.isGFQuestSatisfied(*a, q) {
					st.QuestStatus[qKey] = "completed"
					rewardText, extra := w.applyGFReward(a, Action{Kind: "wait"}, q.Reward, 1)
					ev := w.record(a, Action{Kind: "wait"}, "gf_quest_completed", map[string]any{
						"text":          fmt.Sprintf("%s completed [%s] quest '%s' (%s)", a.Name, gf.Name, q.Title, rewardText),
						"golden_finger": gf.ID,
						"quest_id":      q.ID,
						"quest_title":   q.Title,
						"reward":        rewardText,
					})
					out = append(out, ev)
					out = append(out, extra...)
					continue
				}
				if q.DeadlineTick > 0 && w.Tick >= q.DeadlineTick {
					st.QuestStatus[qKey] = "failed"
					var penParts []string
					var deathEvs []Event
					if q.FailurePenalty != nil {
						fp := q.FailurePenalty
						if fp.CoinsLoss > 0 {
							loss := minInt(a.Money, fp.CoinsLoss)
							a.Money -= loss
							w.recordLedgerCurrencyConsumed("coins", int64(loss))
							penParts = append(penParts, fmt.Sprintf("-%d coins", loss))
						}
						if len(fp.CurrencyLoss) > 0 {
							curKeys := make([]string, 0, len(fp.CurrencyLoss))
							for cur := range fp.CurrencyLoss {
								curKeys = append(curKeys, cur)
							}
							sort.Strings(curKeys)
							for _, cur := range curKeys {
								amt := fp.CurrencyLoss[cur]
								if amt > 0 && a.Currencies[cur] > 0 {
									loss := amt
									if loss > a.Currencies[cur] {
										loss = a.Currencies[cur]
									}
									a.Currencies[cur] -= loss
									w.recordLedgerCurrencyConsumed(cur, loss)
									penParts = append(penParts, fmt.Sprintf("-%d %s", loss, cur))
								}
							}
						}
						if fp.Lethal {
							a.Health = 0
							penParts = append(penParts, "system_erasure")
						} else if fp.HealthLoss > 0 {
							a.Health = maxInt(0, a.Health-fp.HealthLoss)
							penParts = append(penParts, fmt.Sprintf("-%d HP", fp.HealthLoss))
						}
					}
					ev := w.record(a, Action{Kind: "wait"}, "gf_quest_failed", map[string]any{
						"text":          fmt.Sprintf("%s failed [%s] quest '%s' (%s)", a.Name, gf.Name, q.Title, strings.Join(penParts, ", ")),
						"golden_finger": gf.ID,
						"quest_id":      q.ID,
						"quest_title":   q.Title,
						"penalty":       strings.Join(penParts, ", "),
					})
					out = append(out, ev)
					if a.Health == 0 {
						deathEvs = w.markDeadIfZero(a, nil, Action{Kind: "wait"}, "system_erasure", ev)
						out = append(out, deathEvs...)
						break
					}
				}
			}
		}
	}
	return out
}

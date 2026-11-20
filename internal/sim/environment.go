package sim

import "sort"

// InstitutionToggles lets a WorldPack selectively enable or disable macro-institutional
// subsystems (e.g. taxation, enforcement courts, oversight audits, syndicate rackets,
// revolt escalation, rituals). Nil fields default to true for backward compatibility.
type InstitutionToggles struct {
	Taxation     *bool `json:"taxation,omitempty"`
	Enforcement  *bool `json:"enforcement,omitempty"`
	Oversight    *bool `json:"oversight,omitempty"`
	Racketeering *bool `json:"racketeering,omitempty"`
	Revolt       *bool `json:"revolt,omitempty"`
	Rituals      *bool `json:"rituals,omitempty"`
}

func (t *InstitutionToggles) TaxationEnabled() bool {
	return t == nil || t.Taxation == nil || *t.Taxation
}

func (t *InstitutionToggles) EnforcementEnabled() bool {
	return t == nil || t.Enforcement == nil || *t.Enforcement
}

func (t *InstitutionToggles) OversightEnabled() bool {
	return t == nil || t.Oversight == nil || *t.Oversight
}

func (t *InstitutionToggles) RacketeeringEnabled() bool {
	return t == nil || t.Racketeering == nil || *t.Racketeering
}

func (t *InstitutionToggles) RevoltEnabled() bool {
	return t == nil || t.Revolt == nil || *t.Revolt
}

func (t *InstitutionToggles) RitualsEnabled() bool {
	return t == nil || t.Rituals == nil || *t.Rituals
}

// EnvironmentProfile declares genre-neutral setting metadata, institutional switches,
// semantic tags, forum features, and display terminology so simulation settlement,
// candidate generation, and offline option expansion never hardcode any era or theme.
type EnvironmentProfile struct {
	Genre            string              `json:"genre,omitempty"`
	Theme            string              `json:"theme,omitempty"`
	Summary          string              `json:"summary,omitempty"`
	CurrencyName     string              `json:"currency_name,omitempty"`
	ActorNoun        string              `json:"actor_noun,omitempty"`
	AuthorityName    string              `json:"authority_name,omitempty"`
	OversightName    string              `json:"oversight_name,omitempty"`
	ForumFeatures    []string            `json:"forum_features,omitempty"`
	EnforcementTags  []string            `json:"enforcement_tags,omitempty"`
	OversightTags    []string            `json:"oversight_tags,omitempty"`
	OutlawTags        []string            `json:"outlaw_tags,omitempty"`
	EnforcementItem   string              `json:"enforcement_item,omitempty"`
	RepairItem        string              `json:"repair_item,omitempty"`
	BuilderProfession string              `json:"builder_profession,omitempty"`
	Institutions       *InstitutionToggles `json:"institutions,omitempty"`
	ActionLabels       map[string]string   `json:"action_labels,omitempty"`
	Terminology        map[string]string   `json:"terminology,omitempty"`
	PersonalityCatalog map[string]string   `json:"personality_catalog,omitempty"`
	HobbyCatalog       map[string]string   `json:"hobby_catalog,omitempty"`
}

// CloneEnvironmentProfile returns a deep copy of an optional EnvironmentProfile.
func CloneEnvironmentProfile(src *EnvironmentProfile) *EnvironmentProfile {
	if src == nil {
		return nil
	}
	cp := *src
	cp.ForumFeatures = append([]string(nil), src.ForumFeatures...)
	cp.EnforcementTags = append([]string(nil), src.EnforcementTags...)
	cp.OversightTags = append([]string(nil), src.OversightTags...)
	cp.OutlawTags = append([]string(nil), src.OutlawTags...)
	if src.Institutions != nil {
		t := *src.Institutions
		if src.Institutions.Taxation != nil {
			v := *src.Institutions.Taxation
			t.Taxation = &v
		}
		if src.Institutions.Enforcement != nil {
			v := *src.Institutions.Enforcement
			t.Enforcement = &v
		}
		if src.Institutions.Oversight != nil {
			v := *src.Institutions.Oversight
			t.Oversight = &v
		}
		if src.Institutions.Racketeering != nil {
			v := *src.Institutions.Racketeering
			t.Racketeering = &v
		}
		if src.Institutions.Revolt != nil {
			v := *src.Institutions.Revolt
			t.Revolt = &v
		}
		if src.Institutions.Rituals != nil {
			v := *src.Institutions.Rituals
			t.Rituals = &v
		}
		cp.Institutions = &t
	}
	if len(src.ActionLabels) > 0 {
		cp.ActionLabels = make(map[string]string, len(src.ActionLabels))
		for k, v := range src.ActionLabels {
			cp.ActionLabels[k] = v
		}
	}
	if len(src.Terminology) > 0 {
		cp.Terminology = make(map[string]string, len(src.Terminology))
		for k, v := range src.Terminology {
			cp.Terminology[k] = v
		}
	}
	if len(src.PersonalityCatalog) > 0 {
		cp.PersonalityCatalog = make(map[string]string, len(src.PersonalityCatalog))
		for k, v := range src.PersonalityCatalog {
			cp.PersonalityCatalog[k] = v
		}
	}
	if len(src.HobbyCatalog) > 0 {
		cp.HobbyCatalog = make(map[string]string, len(src.HobbyCatalog))
		for k, v := range src.HobbyCatalog {
			cp.HobbyCatalog[k] = v
		}
	}
	return &cp
}

// DefaultEnvironmentProfile returns backward-compatible neutral defaults when a world
// omits an explicit EnvironmentProfile.
func DefaultEnvironmentProfile() EnvironmentProfile {
	return EnvironmentProfile{
		Genre:             "generic",
		CurrencyName:      "coins",
		ActorNoun:         "resident",
		AuthorityName:     "authority",
		OversightName:     "oversight",
		ForumFeatures:     []string{"public_forum", "authority_seat", "ritual_site", "hub", "market", "academy", "shrine", "yamen"},
		EnforcementTags:   []string{"law"},
		OversightTags:     []string{"censor"},
		OutlawTags:        []string{"outlaw", "raider", "thief", "gang"},
		EnforcementItem:   "bounty_writ",
		RepairItem:        "plank",
		BuilderProfession: "carpenter",
	}
}

// EffectiveEnvironment returns the catalog's EnvironmentProfile with omitted fields
// populated from backward-compatible defaults.
func (c Catalog) EffectiveEnvironment() EnvironmentProfile {
	def := DefaultEnvironmentProfile()
	if c.Environment == nil {
		return def
	}
	env := *c.Environment
	if env.Genre == "" {
		env.Genre = def.Genre
	}
	if env.CurrencyName == "" {
		env.CurrencyName = def.CurrencyName
	}
	if env.ActorNoun == "" {
		env.ActorNoun = def.ActorNoun
	}
	if env.AuthorityName == "" {
		env.AuthorityName = def.AuthorityName
	}
	if env.OversightName == "" {
		env.OversightName = def.OversightName
	}
	if len(env.ForumFeatures) == 0 {
		env.ForumFeatures = def.ForumFeatures
	}
	if len(env.EnforcementTags) == 0 {
		env.EnforcementTags = def.EnforcementTags
	}
	if len(env.OversightTags) == 0 {
		env.OversightTags = def.OversightTags
	}
	if len(env.OutlawTags) == 0 {
		env.OutlawTags = def.OutlawTags
	}
	if env.EnforcementItem == "" {
		env.EnforcementItem = def.EnforcementItem
	}
	if env.RepairItem == "" {
		env.RepairItem = def.RepairItem
	}
	if env.BuilderProfession == "" {
		env.BuilderProfession = def.BuilderProfession
	}
	return env
}

// PrimaryOutlawTag returns the canonical outlaw tag configured for this environment.
func (e EnvironmentProfile) PrimaryOutlawTag() string {
	if len(e.OutlawTags) > 0 && e.OutlawTags[0] != "" {
		return e.OutlawTags[0]
	}
	return "outlaw"
}

// PrimaryEnforcementTag returns the canonical enforcement tag configured for this environment.
func (e EnvironmentProfile) PrimaryEnforcementTag() string {
	if len(e.EnforcementTags) > 0 && e.EnforcementTags[0] != "" {
		return e.EnforcementTags[0]
	}
	return "law"
}

// PrimaryOversightTag returns the canonical oversight tag configured for this environment.
func (e EnvironmentProfile) PrimaryOversightTag() string {
	if len(e.OversightTags) > 0 && e.OversightTags[0] != "" {
		return e.OversightTags[0]
	}
	return "censor"
}

// IsOutlawTags reports whether any tag in tags marks an outlaw/hostile-violator role
// under this catalog's environment configuration.
func (c Catalog) IsOutlawTags(tags []string) bool {
	env := c.EffectiveEnvironment()
	for _, want := range env.OutlawTags {
		if ActorHasTag(tags, want) {
			return true
		}
	}
	return false
}

// IsEnforcementTags reports whether any tag in tags marks an enforcement role.
func (c Catalog) IsEnforcementTags(tags []string) bool {
	env := c.EffectiveEnvironment()
	for _, want := range env.EnforcementTags {
		if ActorHasTag(tags, want) {
			return true
		}
	}
	return false
}

// IsOversightTags reports whether any tag in tags marks an oversight/audit role.
func (c Catalog) IsOversightTags(tags []string) bool {
	env := c.EffectiveEnvironment()
	for _, want := range env.OversightTags {
		if ActorHasTag(tags, want) {
			return true
		}
	}
	return false
}

// IsPublicForumDistrict reports whether districtID carries any configured forum feature
// (or universal public_forum / authority_seat / exchange_hub tags).
func (w *World) isConfiguredForumDistrict(districtID string) bool {
	env := w.Catalog.EffectiveEnvironment()
	for _, feat := range env.ForumFeatures {
		if w.Map.DistrictHasFeature(districtID, feat) {
			return true
		}
	}
	return w.Map.DistrictHasFeature(districtID, "public_forum") ||
		w.Map.DistrictHasFeature(districtID, "authority_seat") ||
		w.Map.DistrictHasFeature(districtID, "exchange_hub")
}

// ProfessionGrantsCapability reports whether profession p grants the given action capability
// through Catalog.TagActions.
func (c Catalog) ProfessionGrantsCapability(p Profession, capability string) bool {
	if capability == "" {
		return false
	}
	for _, tag := range p.Grants {
		for _, act := range c.TagActions[tag] {
			if act == capability {
				return true
			}
		}
	}
	return false
}

// StealthBonus computes an actor's setting-agnostic stealth score from Dexterity and
// any leveled professions with StealthBonusPerLevel or the "steal" capability.
func (c Catalog) StealthBonus(a Actor) (stealth int, maxLootCoins int) {
	stealth = a.Attributes.Dexterity
	maxLootCoins = 2
	for profID, lv := range a.Professions {
		if lv <= 0 {
			continue
		}
		p, ok := c.ProfessionByID(profID)
		if !ok {
			continue
		}
		bonus := p.StealthBonusPerLevel
		if bonus <= 0 && c.ProfessionGrantsCapability(p, "steal") {
			if p.Tier >= 2 {
				bonus = 4
			} else {
				bonus = 3
			}
		}
		if bonus > 0 {
			stealth += lv * bonus
			if p.Tier >= 2 {
				maxLootCoins += lv * 2
			} else {
				maxLootCoins += lv
			}
		}
	}
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := c.Skills[sid]; ok && sk.StealthBonusPerLevel > 0 {
			stealth += lv * sk.StealthBonusPerLevel
			maxLootCoins += (lv + 1) / 2
		}
	}
	return stealth, maxLootCoins
}

// VigilanceBonus computes an actor's setting-agnostic anti-theft vigilance from Insight,
// FormSpec VigilanceBonus, independent skills, and leveled professions.
func (c Catalog) VigilanceBonus(a Actor) int {
	vigilance := a.Attributes.Insight
	if a.FormID != "" {
		if form, ok := c.Forms[a.FormID]; ok && form.VigilanceBonus > 0 {
			vigilance += form.VigilanceBonus
		}
	}
	for sid, lv := range a.Skills {
		if lv <= 0 {
			continue
		}
		if sk, ok := c.Skills[sid]; ok && sk.VigilanceBonusPerLevel > 0 {
			vigilance += lv * sk.VigilanceBonusPerLevel
		}
	}
	for profID, lv := range a.Professions {
		if lv <= 0 {
			continue
		}
		p, ok := c.ProfessionByID(profID)
		if !ok {
			continue
		}
		bonus := p.VigilanceBonusPerLevel
		if bonus <= 0 {
			switch {
			case c.ProfessionGrantsCapability(p, "arrest"):
				if p.Tier >= 2 {
					bonus = 4
				} else {
					bonus = 3
				}
			case c.ProfessionGrantsCapability(p, "spar") || p.CombatBonusPerLevel > 0 && !c.IsOutlawTags(p.Grants):
				bonus = 2
			}
		}
		vigilance += lv * bonus
	}
	return vigilance
}

// ExtortionFee computes the setting-agnostic coercion/protection fee from any leveled
// professions with ExtortionBonusPerLevel or the "extort" capability.
func (c Catalog) ExtortionFee(a Actor) int {
	fee := 2
	for profID, lv := range a.Professions {
		if lv <= 0 {
			continue
		}
		p, ok := c.ProfessionByID(profID)
		if !ok {
			continue
		}
		bonus := p.ExtortionBonusPerLevel
		if bonus <= 0 && c.ProfessionGrantsCapability(p, "extort") {
			if p.Tier >= 2 {
				bonus = 2
			} else {
				bonus = 1
			}
		}
		fee += lv * bonus
	}
	return fee
}

// BestProfessionForCapability selects the highest-tier/highest-level profession an actor
// actually holds that grants the given capability, avoiding hardcoded era profession IDs.
func (c Catalog) BestProfessionForCapability(a Actor, capability string) string {
	bestID := ""
	bestTier := -1
	bestLv := -1
	keys := make([]string, 0, len(a.Professions))
	for k := range a.Professions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, profID := range keys {
		lv := a.Professions[profID]
		if lv <= 0 {
			continue
		}
		p, ok := c.ProfessionByID(profID)
		if !ok || !c.ProfessionGrantsCapability(p, capability) {
			continue
		}
		if p.Tier > bestTier || (p.Tier == bestTier && lv > bestLv) {
			bestID = profID
			bestTier = p.Tier
			bestLv = lv
		}
	}
	if bestID != "" {
		return bestID
	}
	// Fallback to the actor's primary role if it exists in their professions.
	if a.Role != "" && a.Professions[a.Role] > 0 {
		return a.Role
	}
	return ""
}

// SelectLootItem deterministically chooses the highest-value tradeable item currently
// carried by target b, avoiding hardcoded ancient item ID lists.
func (c Catalog) SelectLootItem(b Actor) string {
	bestItem := ""
	bestScore := -1
	keys := make([]string, 0, len(b.Items))
	for k, qty := range b.Items {
		if qty > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, itemID := range keys {
		spec, ok := c.Items[itemID]
		if !ok || !spec.Tradeable {
			continue
		}
		score := spec.Tier*100 + spec.BatchPrice
		if score > bestScore {
			bestScore = score
			bestItem = itemID
		}
	}
	return bestItem
}

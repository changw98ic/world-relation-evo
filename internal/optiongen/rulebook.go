package optiongen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/changw98ic/world-relation-evo/internal/agent"
	"github.com/changw98ic/world-relation-evo/internal/sim"
)

// RulebookDomainSpec describes one generic interaction domain synthesized offline by DeepSeek.
type RulebookDomainSpec struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	FocusActions    []string `json:"focus_actions"`
	FocusOutcomes   []string `json:"focus_outcomes"`
	MinProactive    int      `json:"min_proactive"`
	MinReaction     int      `json:"min_reaction"`
	RequireWitness  bool     `json:"require_witness"`
}

// RoleArchetypeSummary summarizes a role class across the world WITHOUT any actor IDs or names.
type RoleArchetypeSummary struct {
	Role         string   `json:"role"`
	Population   int      `json:"population"`
	CommonTags   []string `json:"common_tags"`
	Capabilities []string `json:"capabilities"`
	Professions  []string `json:"professions,omitempty"`
}

// WorldOntology is the 100% actor-anonymized structural description of a world passed to DeepSeek.
// No specific actor IDs or actor names ever appear in this structure.
type WorldOntology struct {
	WorldID              string                   `json:"world_id"`
	Environment          sim.EnvironmentProfile   `json:"environment"`
	Glossaries           agent.PlaybookGlossaries `json:"glossaries"`
	RoleArchetypes       []RoleArchetypeSummary   `json:"role_archetypes"`
	PersonalityTraits    []string                 `json:"personality_traits,omitempty"`
	PersonalityLabels    map[string]string        `json:"personality_labels,omitempty"`
	Hobbies              []string                 `json:"hobbies,omitempty"`
	HobbyLabels          map[string]string        `json:"hobby_labels,omitempty"`
	DistrictFeatures     []string                 `json:"district_features"`
	AllowedEngineActions []string                 `json:"allowed_engine_actions"`
	ValidOutcomeKinds    []string                 `json:"valid_outcome_kinds"`
	DisabledInstitutions []string                 `json:"disabled_institutions,omitempty"`
}

// DefaultReactionStances defines the 8 orthogonal causal reaction stances used across genres.
var DefaultReactionStances = []string{
	"积极呼应",
	"秉公处置",
	"强硬反制",
	"谈判妥协",
	"借力拉拢",
	"暗中周旋",
	"趁机谋利",
	"避险止损",
}

// DomainRulebookOutput is the JSON payload returned by DeepSeek for a single generic domain.
type DomainRulebookOutput struct {
	Domain                        string                            `json:"domain"`
	Summary                       string                            `json:"summary"`
	PersonalityStanceWeights      map[string]map[string]float64     `json:"personality_stance_weights,omitempty"`
	ProfessionEncounterModifiers  map[string]agent.EncounterProfile `json:"profession_encounter_modifiers,omitempty"`
	PersonalityEncounterModifiers map[string]agent.EncounterProfile `json:"personality_encounter_modifiers,omitempty"`
	HobbyActionWeights            map[string]map[string]float64     `json:"hobby_action_weights,omitempty"`
	ProactiveRules                []agent.InteractionRule           `json:"proactive_rules"`
	ReactionRules                 []agent.CausalReactionRule        `json:"reaction_rules"`
}

// TraitCalibrationOutput is the global profession, personality & hobby calibration table synthesized
// offline once per world (100% actor-anonymized), keeping structural profession exposure strictly
// decoupled from within-profession personality encounter modifiers and 8-stance weights.
type TraitCalibrationOutput struct {
	Summary                       string                            `json:"summary"`
	ProfessionEncounterModifiers  map[string]agent.EncounterProfile `json:"profession_encounter_modifiers"`
	PersonalityStanceWeights      map[string]map[string]float64     `json:"personality_stance_weights"`
	PersonalityEncounterModifiers map[string]agent.EncounterProfile `json:"personality_encounter_modifiers"`
	HobbyActionWeights            map[string]map[string]float64     `json:"hobby_action_weights"`
}

// TraitCalibrationRecord stores the full offline audit trail for the global trait calibration request.
type TraitCalibrationRecord struct {
	WorldID          string                 `json:"world_id"`
	Model            string                 `json:"model"`
	Endpoint         string                 `json:"endpoint"`
	RequestedAt      time.Time              `json:"requested_at"`
	DurationMS       int64                  `json:"duration_ms"`
	PromptTokens     int                    `json:"prompt_tokens,omitempty"`
	CompletionTokens int                    `json:"completion_tokens,omitempty"`
	TotalTokens      int                    `json:"total_tokens,omitempty"`
	Output           TraitCalibrationOutput `json:"output"`
	RawResponse      string                 `json:"raw_response"`
}

// DomainRulebookRecord stores the full offline synthesis audit trail for one domain.
type DomainRulebookRecord struct {
	WorldID          string               `json:"world_id"`
	Domain           RulebookDomainSpec   `json:"domain"`
	Model            string               `json:"model"`
	Endpoint         string               `json:"endpoint"`
	RequestedAt      time.Time            `json:"requested_at"`
	DurationMS       int64                `json:"duration_ms"`
	PromptTokens     int                  `json:"prompt_tokens,omitempty"`
	CompletionTokens int                  `json:"completion_tokens,omitempty"`
	TotalTokens      int                  `json:"total_tokens,omitempty"`
	Output           DomainRulebookOutput `json:"output"`
	RawResponse      string               `json:"raw_response"`
}

// RulebookAuditReport records programmatic safety & generality checks on the generated rulebook.
type RulebookAuditReport struct {
	ActorLeakCount              int      `json:"actor_leak_count"`
	ActorLeaks                  []string `json:"actor_leaks,omitempty"`
	DroppedProactiveRules       int      `json:"dropped_proactive_rules"`
	DroppedReactionRules        int      `json:"dropped_reaction_rules"`
	DroppedReactionBranches     int      `json:"dropped_reaction_branches"`
	WitnessReactionRules        int      `json:"witness_reaction_rules"`
	Actor2ReactionRules         int      `json:"actor2_reaction_rules"`
	PersonalityWeightedBranches int      `json:"personality_weighted_branches"`
	HobbyWeightedProactiveRules int      `json:"hobby_weighted_proactive_rules"`
	SharedHobbyProactiveRules   int      `json:"shared_hobby_proactive_rules"`
	ProfessionEncounterRoles    int      `json:"profession_encounter_roles"`
	PersonalityStanceTraits     int      `json:"personality_stance_traits"`
	PersonalityEncounterTraits  int      `json:"personality_encounter_traits"`
	HobbyActionKeys             int      `json:"hobby_action_keys"`
}

// RulebookExpandSummary summarizes an offline expand-rulebook synthesis run.
type RulebookExpandSummary struct {
	WorldID             string               `json:"world_id"`
	Theme               string               `json:"theme"`
	Genre               string               `json:"genre"`
	Model               string               `json:"model"`
	Endpoint            string               `json:"endpoint"`
	StartedAt           time.Time            `json:"started_at"`
	CompletedAt         time.Time            `json:"completed_at"`
	Domains             int                  `json:"domains"`
	TotalRequests       int                  `json:"total_requests"`
	TotalTokens         int                  `json:"total_tokens"`
	TotalProactiveRules int                  `json:"total_proactive_rules"`
	TotalReactionRules  int                  `json:"total_reaction_rules"`
	TotalBranches       int                  `json:"total_branches"`
	Audit               RulebookAuditReport  `json:"audit"`
	RulebookFile        string               `json:"rulebook_file"`
	DomainFiles         map[string]string    `json:"domain_files"`
}

// RulebookExpandConfig configures ExpandWorldRulebook.
type RulebookExpandConfig struct {
	OutDir      string
	Concurrency int
	Resume      bool
	Progress    io.Writer
}

var baseInteractiveEngineActions = []string{
	"offer",
	"accept",
	"reject",
	"propose_agreement",
	"accept_agreement",
	"reject_agreement",
	"fulfill_agreement",
	"speak",
	"transmit_message",
	"attack",
	"spar",
	"steal",
	"raid",
	"extort",
	"bribe",
	"arrest",
	"audit_corruption",
	"file_appeal",
	"issue_decree",
	"heal",
	"swear_bond",
	"teach_knowledge",
	"avenge_feud",
	"hunt_threat",
	"maintain_infra",
	"repair_house",
	"perform_ritual",
	"move",
}

var baseEngineOutcomeKinds = []string{
	"trade_offered",
	"trade_completed",
	"trade_rejected",
	"trade_counteroffered",
	"agreement_proposed",
	"agreement_accepted",
	"agreement_rejected",
	"agreement_fulfilled",
	"attacked",
	"sparred",
	"stolen",
	"steal_foiled",
	"raided",
	"raid_repelled",
	"extorted",
	"extort_resisted",
	"bribed",
	"arrested",
	"arrest_resisted",
	"corruption_purged",
	"wrongful_conviction",
	"conviction_overturned",
	"decree_issued",
	"speech",
	"channel_message",
	"bond_sworn",
	"grudge_formed",
	"feud_avenged",
	"feud_mediated",
	"healed",
	"knowledge_taught",
	"threat_hunted",
	"infra_maintained",
	"repaired_house",
}

// BuildWorldOntology extracts a strictly actor-anonymized structural ontology from a World.
func BuildWorldOntology(w *sim.World) WorldOntology {
	env := w.Catalog.EffectiveEnvironment()
	rawGloss := buildGlossaries(w)
	// Intentionally keep Actors glossary EMPTY so DeepSeek never sees any fixed character ID or name!
	gloss := agent.PlaybookGlossaries{
		Locations:   rawGloss.Locations,
		Professions: rawGloss.Professions,
		Items:       rawGloss.Items,
		Factions:    rawGloss.Factions,
		Recipes:     rawGloss.Recipes,
		Actors:      map[string]string{},
	}

	var disabled []string
	if !env.Institutions.TaxationEnabled() {
		disabled = append(disabled, "税赋征收")
	}
	if !env.Institutions.EnforcementEnabled() {
		disabled = append(disabled, "强制缉拿/执法")
	}
	if !env.Institutions.OversightEnabled() {
		disabled = append(disabled, "监察审计/申诉")
	}
	if !env.Institutions.RacketeeringEnabled() {
		disabled = append(disabled, "保护费/强行勒索")
	}
	if !env.Institutions.RevoltEnabled() {
		disabled = append(disabled, "民变暴动")
	}
	if !env.Institutions.RitualsEnabled() {
		disabled = append(disabled, "公开仪式/祭典")
	}

	// Filter interactive actions by world institutions and actual catalog capabilities.
	worldCaps := map[string]bool{}
	for _, a := range w.Actors {
		for capKind := range w.Catalog.Capabilities(a) {
			worldCaps[capKind] = true
		}
	}
	var allowedActions []string
	for _, k := range baseInteractiveEngineActions {
		if k == "extort" && !env.Institutions.RacketeeringEnabled() {
			continue
		}
		if k == "arrest" && !env.Institutions.EnforcementEnabled() {
			continue
		}
		if (k == "audit_corruption" || k == "file_appeal") && !env.Institutions.OversightEnabled() {
			continue
		}
		if k == "perform_ritual" && !env.Institutions.RitualsEnabled() {
			continue
		}
		if len(worldCaps) > 0 && !worldCaps[k] && k != "move" && k != "speak" && k != "offer" && k != "accept" && k != "reject" {
			continue
		}
		allowedActions = append(allowedActions, k)
	}

	// Filter valid engine outcome kinds by world institutions.
	var validOutcomes []string
	for _, o := range baseEngineOutcomeKinds {
		if (o == "extorted" || o == "extort_resisted") && !env.Institutions.RacketeeringEnabled() {
			continue
		}
		if (o == "arrested" || o == "arrest_resisted") && !env.Institutions.EnforcementEnabled() {
			continue
		}
		if (o == "corruption_purged" || o == "wrongful_conviction" || o == "conviction_overturned") && !env.Institutions.OversightEnabled() {
			continue
		}
		validOutcomes = append(validOutcomes, o)
	}

	// Aggregate role archetypes WITHOUT exposing any individual actor ID or name.
	type roleAgg struct {
		count int
		tags  map[string]int
		caps  map[string]bool
		profs map[string]bool
	}
	byRole := map[string]*roleAgg{}
	for _, a := range w.Actors {
		r := strings.TrimSpace(a.Role)
		if r == "" {
			r = "generic"
		}
		agg := byRole[r]
		if agg == nil {
			agg = &roleAgg{tags: map[string]int{}, caps: map[string]bool{}, profs: map[string]bool{}}
			byRole[r] = agg
		}
		agg.count++
		for _, t := range a.IdentityTags() {
			if t == "" || strings.HasPrefix(t, "actor:") {
				continue
			}
			agg.tags[t]++
		}
		for c := range w.Catalog.Capabilities(a) {
			agg.caps[c] = true
		}
		for pID, lv := range a.Professions {
			if lv > 0 {
				agg.profs[pID] = true
			}
		}
	}

	roleKeys := make([]string, 0, len(byRole))
	for k := range byRole {
		roleKeys = append(roleKeys, k)
	}
	sort.Strings(roleKeys)

	allowedSet := map[string]bool{}
	for _, k := range allowedActions {
		allowedSet[k] = true
	}

	archetypes := make([]RoleArchetypeSummary, 0, len(roleKeys))
	for _, rk := range roleKeys {
		agg := byRole[rk]
		tags := make([]string, 0, len(agg.tags))
		for t := range agg.tags {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		caps := make([]string, 0, len(agg.caps))
		for c := range agg.caps {
			if allowedSet[c] {
				caps = append(caps, c)
			}
		}
		sort.Strings(caps)
		profs := make([]string, 0, len(agg.profs))
		for pID := range agg.profs {
			profs = append(profs, pID)
		}
		sort.Strings(profs)
		archetypes = append(archetypes, RoleArchetypeSummary{
			Role:         rk,
			Population:   agg.count,
			CommonTags:   tags,
			Capabilities: caps,
			Professions:  profs,
		})
	}

	// Collect strictly anonymized personality trait vocabulary and hobby vocabulary across
	// both the world's EnvironmentProfile catalogs and the current actor population.
	traitSet := map[string]bool{}
	hobbySet := map[string]bool{}
	for k := range env.PersonalityCatalog {
		nk := strings.ToLower(strings.TrimSpace(k))
		if nk != "" {
			traitSet[nk] = true
		}
	}
	for k := range env.HobbyCatalog {
		nk := strings.ToLower(strings.TrimSpace(k))
		if nk != "" {
			hobbySet[nk] = true
		}
	}
	for _, a := range w.Actors {
		for _, tr := range agent.ExtractPersonalityTraits(a.Personality) {
			traitSet[tr] = true
		}
		for _, h := range a.Hobbies {
			hk := strings.ToLower(strings.TrimSpace(h))
			if hk != "" {
				hobbySet[hk] = true
			}
		}
	}
	traits := make([]string, 0, len(traitSet))
	for tr := range traitSet {
		traits = append(traits, tr)
	}
	sort.Strings(traits)

	hobbies := make([]string, 0, len(hobbySet))
	for h := range hobbySet {
		hobbies = append(hobbies, h)
	}
	sort.Strings(hobbies)

	traitLabels := map[string]string{}
	hobbyLabels := map[string]string{}
	for _, tr := range traits {
		if lbl := strings.TrimSpace(env.PersonalityCatalog[tr]); lbl != "" {
			traitLabels[tr] = lbl
		} else if lbl := strings.TrimSpace(env.Terminology[tr]); lbl != "" {
			traitLabels[tr] = lbl
		}
	}
	for _, h := range hobbies {
		if lbl := strings.TrimSpace(env.HobbyCatalog[h]); lbl != "" {
			hobbyLabels[h] = lbl
		} else if lbl := strings.TrimSpace(env.Terminology[h]); lbl != "" {
			hobbyLabels[h] = lbl
		}
	}

	featSet := map[string]bool{}
	for _, d := range w.Map.Districts {
		for _, f := range d.Features {
			if strings.TrimSpace(f) != "" {
				featSet[f] = true
			}
		}
	}
	for _, v := range w.Map.Venues {
		for _, f := range v.Features {
			if strings.TrimSpace(f) != "" {
				featSet[f] = true
			}
		}
	}
	feats := make([]string, 0, len(featSet))
	for f := range featSet {
		feats = append(feats, f)
	}
	sort.Strings(feats)

	return WorldOntology{
		WorldID:              w.ID,
		Environment:          env,
		Glossaries:           gloss,
		RoleArchetypes:       archetypes,
		PersonalityTraits:    traits,
		PersonalityLabels:    traitLabels,
		Hobbies:              hobbies,
		HobbyLabels:          hobbyLabels,
		DistrictFeatures:     feats,
		AllowedEngineActions: allowedActions,
		ValidOutcomeKinds:    validOutcomes,
		DisabledInstitutions: disabled,
	}
}

// DefaultRulebookDomains returns the 5 generic cross-role interaction domains adapted to the world's ontology.
func DefaultRulebookDomains(ont WorldOntology) []RulebookDomainSpec {
	allowedAct := map[string]bool{}
	for _, a := range ont.AllowedEngineActions {
		allowedAct[a] = true
	}
	allowedOut := map[string]bool{}
	for _, o := range ont.ValidOutcomeKinds {
		allowedOut[o] = true
	}
	filterAct := func(list ...string) []string {
		var out []string
		for _, k := range list {
			if allowedAct[k] {
				out = append(out, k)
			}
		}
		return out
	}
	filterOut := func(list ...string) []string {
		var out []string
		for _, k := range list {
			if allowedOut[k] {
				out = append(out, k)
			}
		}
		return out
	}

	return []RulebookDomainSpec{
		{
			ID:             "trade_economy",
			Title:          "资源流通与契约博弈",
			Description:    "任意供给/生产/商贸身份的角色1与需要资源或有余货的角色2之间发起报价、交易、长期供货契约，以及报价/契约达成、还价或拒绝后的连锁反应。",
			FocusActions:   filterAct("offer", "accept", "reject", "propose_agreement", "accept_agreement", "reject_agreement", "fulfill_agreement", "speak", "transmit_message"),
			FocusOutcomes:  filterOut("trade_offered", "trade_completed", "trade_rejected", "trade_counteroffered", "agreement_proposed", "agreement_accepted", "agreement_rejected", "agreement_fulfilled"),
			MinProactive:   6,
			MinReaction:    6,
			RequireWitness: false,
		},
		{
			ID:             "enforcement_oversight",
			Title:          "秩序执法、行贿与监察问责",
			Description:    "拥有执法/秩序或监察/长老职能的角色1与违规者、可疑对象或持权者角色2之间的巡查、缉拿、行贿拉拢、监察清查与申诉平反，以及旁观执法者/同门目击后的即时干预。",
			FocusActions:   filterAct("arrest", "audit_corruption", "bribe", "file_appeal", "issue_decree", "speak", "transmit_message"),
			FocusOutcomes:  filterOut("bribed", "corruption_purged", "arrested", "arrest_resisted", "wrongful_conviction", "conviction_overturned", "decree_issued", "steal_foiled"),
			MinProactive:   5,
			MinReaction:    6,
			RequireWitness: true,
		},
		{
			ID:             "shadow_conflict",
			Title:          "切磋较量、暗中窃取与公开冲突",
			Description:    "任意角色1向角色2发起切磋、偷窃、袭扰、强夺或复仇，引擎结算成功/失败/被抓现行后，目标角色2与同场目击者依据身份和处境作出强硬反制、呼救揭发、止损避险或趁机介入。",
			FocusActions:   filterAct("spar", "attack", "steal", "raid", "extort", "avenge_feud", "arrest", "speak", "heal"),
			FocusOutcomes:  filterOut("sparred", "attacked", "stolen", "steal_foiled", "raided", "raid_repelled", "extorted", "extort_resisted", "grudge_formed", "feud_avenged"),
			MinProactive:   6,
			MinReaction:    7,
			RequireWitness: true,
		},
		{
			ID:             "alliance_mentorship",
			Title:          "传道授业、结盟示好与声望传播",
			Description:    "前辈/师长/高阶身份与后辈/同道之间的指点传艺、结义立誓、公开喊话、远程传讯与拉拢结盟，以及收到示好、传讯或公开言论后的响应与表态。",
			FocusActions:   filterAct("teach_knowledge", "swear_bond", "speak", "transmit_message", "offer", "propose_agreement", "heal"),
			FocusOutcomes:  filterOut("speech", "channel_message", "bond_sworn", "knowledge_taught", "feud_mediated"),
			MinProactive:   6,
			MinReaction:    6,
			RequireWitness: true,
		},
		{
			ID:             "cooperation_crisis",
			Title:          "疗伤救治、除患护阵与灾险协作",
			Description:    "医者/炼丹/护法/工匠等角色对受伤、遇险或处于动荡地点的角色2施救、巡猎清患、维护阵法设施，以及受助者与旁观者目击善举或危机后的回报、传讯与驰援。",
			FocusActions:   filterAct("heal", "hunt_threat", "maintain_infra", "repair_house", "offer", "speak", "transmit_message", "swear_bond"),
			FocusOutcomes:  filterOut("healed", "threat_hunted", "infra_maintained", "repaired_house", "attacked", "raided"),
			MinProactive:   5,
			MinReaction:    5,
			RequireWitness: true,
		},
	}
}

// BuildRulebookSystemPrompt constructs a strictly actor-agnostic, world-ontology-aware prompt.
func BuildRulebookSystemPrompt(ont WorldOntology, domain RulebookDomainSpec) string {
	env := ont.Environment
	theme := nonEmptyStr(env.Theme, ont.WorldID)
	actorNoun := nonEmptyStr(env.ActorNoun, "角色")
	currency := nonEmptyStr(env.CurrencyName, "货币")
	authority := nonEmptyStr(env.AuthorityName, "秩序机构")
	oversight := nonEmptyStr(env.OversightName, "监察机构")

	var wb strings.Builder
	fmt.Fprintf(&wb, "- 世界主题：%s（题材：%s）\n", theme, env.Genre)
	if env.Summary != "" {
		fmt.Fprintf(&wb, "- 世界生态：%s\n", env.Summary)
	}
	fmt.Fprintf(&wb, "- 货币单位：%s｜角色称谓：%s｜秩序机构：%s｜监察机构：%s\n", currency, actorNoun, authority, oversight)
	if len(ont.DisabledInstitutions) > 0 {
		fmt.Fprintf(&wb, "- 本世界已关闭制度：%s（严禁生成涉及已关闭制度的规则或措辞！）\n", strings.Join(ont.DisabledInstitutions, "、"))
	}
	if s := formatGlossaryLine(ont.Glossaries.Locations); s != "" {
		fmt.Fprintf(&wb, "- 地点对照：%s\n", s)
	}
	if s := formatGlossaryLine(ont.Glossaries.Professions); s != "" {
		fmt.Fprintf(&wb, "- 职业/身份对照：%s\n", s)
	}
	if s := formatGlossaryLine(ont.Glossaries.Items); s != "" {
		fmt.Fprintf(&wb, "- 物资/道具对照：%s\n", s)
	}
	if s := formatGlossaryLine(ont.Glossaries.Factions); s != "" {
		fmt.Fprintf(&wb, "- 势力/组织对照：%s\n", s)
	}
	if len(ont.DistrictFeatures) > 0 {
		fmt.Fprintf(&wb, "- 地点特征标签（location_features 可选值）：%s\n", strings.Join(ont.DistrictFeatures, ", "))
	}
	if len(ont.PersonalityTraits) > 0 {
		parts := make([]string, 0, len(ont.PersonalityTraits))
		for _, tr := range ont.PersonalityTraits {
			if lbl := ont.PersonalityLabels[tr]; lbl != "" {
				parts = append(parts, fmt.Sprintf("%s(%s)", tr, lbl))
			} else {
				parts = append(parts, tr)
			}
		}
		fmt.Fprintf(&wb, "- 性格词表（personality_traits，用于修改【应对分支权重】）：%s\n", strings.Join(parts, "、"))
	}
	if len(ont.Hobbies) > 0 {
		parts := make([]string, 0, len(ont.Hobbies))
		for _, h := range ont.Hobbies {
			if lbl := ont.HobbyLabels[h]; lbl != "" {
				parts = append(parts, fmt.Sprintf("%s(%s)", h, lbl))
			} else {
				parts = append(parts, h)
			}
		}
		fmt.Fprintf(&wb, "- 爱好词表（hobbies，用于修改【主动行为权重】与同好加成）：%s\n", strings.Join(parts, "、"))
	}

	return fmt.Sprintf(`你是一名通用的多主体社会模拟因果规则架构师。你的任务不是为某几个固定名字的人物写死剧本，而是为当前世界观编写**100%% 角色无关、可泛化到任意人数与任意新角色**的通用交互与因果反应规则表：

核心范式：
  角色1（按身份/标签匹配；**爱好修改主动行为权重**，性格决定遭遇频率） --[通用行为]--> 角色2（按身份/标签/关系匹配）
      ==> 引擎真实结算结果（TriggerEventKind）
      ==> 角色2 或 同场目击者（Witness）按 8 大正交立场分支应对（**性格修改应对分支权重**）

当前世界设定（必须100%%贴合，严禁串台到其他题材或已关闭制度）：
%s
本次需要设计的交互领域：%s（%s）
领域说明：%s

【8 大正交应对立场（reaction_rules.branches[].stance 必须严格取自以下 8 项）】
1. 积极呼应：正向承接、爽快接受、致谢回报、赞同附和（仁厚/坦直/热情/豪爽/谦逊偏爱）
2. 秉公处置：依规执法、提请监察、公堂申诉、按章立约（恪职/冷峻/刚正/嫉恶如仇/沉稳偏爱）
3. 强硬反制：正面回击、武力压制、针锋相对、立威报复（狠辣/孤傲/好胜/睚眦必报/莽撞/挑衅偏爱）
4. 谈判妥协：还价磋商、各退一步、话术缓冲、讲和斡旋（圆滑/审慎/豁达/随和/节俭偏爱）
5. 借力拉拢：呼朋引伴、拉拢盟友、上告靠山、抱团造势（热络/广结善缘/仗义/好管闲事偏爱）
6. 暗中周旋：表面不动声色、私下传讯、暗线打探、迂回化解（狡黠/多疑/城府/隐忍/草木皆兵偏爱）
7. 趁机谋利：顺水推舟、坐地起价、落井下石、趁乱渔利（精明重利/野心/见风使舵/狠辣偏爱）
8. 避险止损：抽身退避、割肉保命、闭门谢客、暂避锋芒（警觉避险/胆怯/孤僻隐世/韬光养晦/淡漠偏爱）

【严苛的通用性铁律（违者整条作废）】
1. **绝对禁止出现任何具体人名或具体角色ID**：规则面向「任意符合条件的角色1」和「任意符合条件的角色2/目击者」，模板文案里只能使用占位符 $actor1、$actor2、$responder、$location、$item！
2. **必须严格使用引擎真实动作与真实结算事件**：
   - behavior.engine_kind 只能从以下允许列表中选择：%s
   - expected_outcomes 和 trigger_event_kinds 只能从以下真实引擎结果事件中选择：%s
   - 本领域重点推荐动作：%s
   - 本领域重点推荐结果事件：%s
3. **动态目标绑定 target_binding** 只能使用以下 5 个变量之一：
   - "$actor2"（主动规则中指向被作用的角色2）
   - "$actor1"（反应规则中指向发起原行为的角色1）
   - "$enforcer_or_elder"（同场或已知的秩序执法/监察/长老角色，适合举报求援）
   - "$ally"（同场或已知的最高信任同道）
   - "$self"（自身，仅用于无目标动作如 maintain_infra/hunt_threat/issue_decree）
4. **RoleMatcher 字段说明**（留空或使用 ["*"] 表示任意角色都可匹配；不要写得过于苛刻导致运行时无人匹配！）：
   - roles: 角色职业/身份数组，必须取自世界本体 role_archetypes 中的 role，或 ["*"]
   - any_tags: 标签数组，取自 role_archetypes 中的 common_tags（满足任一即可），或留空
   - exclude_tags: 排除标签数组
   - min_health / max_health / min_money / max_money / has_illicit_gain / has_surplus_goods / is_wounded（按需选填）
   - **重要**：不要在 actor1 或 when_responder 中用 personality_keywords / any_hobbies 做硬性过滤（否则会让很多角色永远不触发），而要通过下面的**权重乘数**来区分不同性格与爱好的倾向差异！
5. **性格与爱好正交权重设计（核心验收项，必须逐条认真填写！）**：
   - **爱好修改主动行为权重**：
     * 每条 proactive_rule 必须填写 `+"`hobby_weights`"+`（从世界本体的 hobbies 词表中挑选 2~5 个相关爱好，契合该行为的给 `+"`1.8~3.0`"+` 高乘数，与该行为气质相悖的给 `+"`0.35~0.65`"+` 低乘数）以及 `+"`shared_hobby_multiplier`"+`（当角色1与角色2拥有共同爱好时的额外倍率，社交/切磋/交易/协作类给 `+"`1.5~2.2`"+`，纯敌对突袭可给 `+"`1.0`"+` 或 `+"`0.7`"+`）。
     * 同时在顶层输出本领域相关的 `+"`hobby_action_weights`"+`（映射 `+"`hobby_id -> engine_kind -> multiplier`"+`）。
   - **性格修改应对分支权重**：
     * 每条 reaction_rule 的每个 branch 必须填写 `+"`personality_weights`"+`（从世界本体的 personality_traits 词表中挑选 3~5 个最关键性格词，符合该立场与动作的给 `+"`1.7~2.8`"+` 高乘数，相反性格给 `+"`0.35~0.65`"+` 低乘数）。
     * 全局性格表会由独立标定任务统一生成，因此本领域输出中**无需重复枚举全部性格词表**，只需把篇幅集中用于设计更丰富的 3~4 分支因果反应规则！
6. **立场均衡覆盖与篇幅控制要求**：
   - proactive_rules 生成 %d~6 条（精炼覆盖核心常规互动与利益/冲突摩擦，不要超过 6 条）：每条必须明确 actor1、actor2、relation、hobby_weights、shared_hobby_multiplier、behavior、expected_outcomes、causal_summary（1句精炼中文）。
   - reaction_rules 生成 %d~7 条（精炼覆盖核心真实引擎结果，不要超过 7 条）：必须同时包含 responder_position="actor2"（当事受害/受邀方）与 responder_position="witness"（同场目击旁观者）两类规则！
   - 每条 reaction_rule 包含 **3~4 个不同立场**的 branches（从 8 大立场中选择，且各分支基础 weight 保持在 25~40 的均衡区间，由性格权重拉开胜负）：
     * 正向/商业/社交高频事件（如 trade_offered, trade_rejected, agreement_proposed, healed, knowledge_taught, channel_message, speech, sparred）必须积极覆盖「积极呼应」「暗中周旋」「趁机谋利」「谈判妥协」「借力拉拢」等立场，严禁只写冲突选项！
     * 执法/违规/冲突事件必须区分「秉公处置」（按规矩执法/审计/申诉）与「强硬反制」（私力报复/武力压制），并兼顾「暗中周旋」「趁机谋利」「避险止损」！
7. **所有中文描述必须是贴合「%s」世界观的自然简体中文人话**，禁止在中文文案中夹杂英文变量名（除 $actor1/$actor2/$responder/$location/$item 占位符外）。

请严格返回合法 JSON，结构如下：
{
  "domain": "%s",
  "summary": "该领域通用因果博弈逻辑概述（1-2句中文）",
  "hobby_action_weights": {
    "sword_dueling": {"spar": 2.6, "attack": 1.4, "offer": 0.5},
    "market_bargaining": {"offer": 2.6, "propose_agreement": 2.0, "spar": 0.45}
  },
  "proactive_rules": [
    {
      "id": "pro_%s_01",
      "domain": "%s",
      "name": "规则中文名（不含具体人名）",
      "category": "交易|执法|冲突|结盟|协作",
      "priority": "urgent|high|medium|low",
      "weight": 35,
      "hobby_weights": {"market_bargaining": 2.5, "sword_dueling": 0.5},
      "shared_hobby_multiplier": 1.8,
      "cooldown_ticks": 48,
      "actor1": {"roles": ["..."], "any_tags": ["..."]},
      "actor2": {"roles": ["*"], "exclude_tags": []},
      "relation": {"co_located": true, "faction_relation": "any"},
      "location_features": [],
      "behavior": {
        "engine_kind": "offer",
        "target_binding": "$actor2",
        "item_role": "surplus",
        "summary_template": "$actor1 在 $location 向 $actor2 发出 $item 交易报价"
      },
      "expected_outcomes": ["trade_offered"],
      "causal_summary": "通用因果说明：角色1执行该行为后在引擎产生何种结果，将诱发角色2与旁观者何种连锁反应"
    }
  ],
  "reaction_rules": [
    {
      "id": "rxn_%s_01",
      "domain": "%s",
      "name": "因果反应规则中文名（不含具体人名）",
      "trigger_event_kinds": ["attacked"],
      "outcome_class": "succeeded|failed|proposed|any",
      "actor1_match": {"roles": ["*"]},
      "actor2_match": {"roles": ["*"]},
      "responder_position": "actor2|witness",
      "responder_match": {"roles": ["*"], "any_tags": []},
      "relation_to_actor1": {"co_located": true},
      "causal_summary": "目击或遭遇该结果后的通用因果分支逻辑",
      "branches": [
        {
          "id": "br_01",
          "stance": "秉公处置",
          "title": "分支中文标题",
          "when_responder": {},
          "weight": 35,
          "personality_weights": {"dutiful": 2.5, "righteous": 2.2, "stern": 1.8, "cunning": 0.5},
          "behavior": {
            "engine_kind": "arrest",
            "target_binding": "$actor1",
            "summary_template": "$responder 目击 $actor1 伤害 $actor2，依规当场出手缉拿 $actor1"
          },
          "expected_outcome": "arrested"
        }
      ]
    }
  ]
}`,
		wb.String(),
		domain.Title, domain.ID, domain.Description,
		strings.Join(ont.AllowedEngineActions, ", "),
		strings.Join(ont.ValidOutcomeKinds, ", "),
		strings.Join(domain.FocusActions, ", "),
		strings.Join(domain.FocusOutcomes, ", "),
		domain.MinProactive,
		domain.MinReaction,
		theme,
		domain.ID, domain.ID, domain.ID, domain.ID, domain.ID,
	)
}

// BuildTraitCalibrationSystemPrompt builds the prompt for the dedicated global profession,
// personality & hobby calibration call that synthesizes decoupled structural profession exposure,
// within-profession personality encounter modifiers, 8-stance weights, and hobby->action weights
// across 100% of the world's anonymized vocabularies.
func BuildTraitCalibrationSystemPrompt(ont WorldOntology) string {
	env := ont.Environment
	theme := nonEmptyStr(env.Theme, ont.WorldID)

	var profsList []string
	profIDs := make([]string, 0, len(ont.Glossaries.Professions))
	for pid := range ont.Glossaries.Professions {
		profIDs = append(profIDs, pid)
	}
	sort.Strings(profIDs)
	for _, pid := range profIDs {
		if lbl := ont.Glossaries.Professions[pid]; lbl != "" {
			profsList = append(profsList, fmt.Sprintf("%s(%s)", pid, lbl))
		} else {
			profsList = append(profsList, pid)
		}
	}

	var traitsList []string
	for _, tr := range ont.PersonalityTraits {
		if lbl := ont.PersonalityLabels[tr]; lbl != "" {
			traitsList = append(traitsList, fmt.Sprintf("%s(%s)", tr, lbl))
		} else {
			traitsList = append(traitsList, tr)
		}
	}
	var hobbiesList []string
	for _, h := range ont.Hobbies {
		if lbl := ont.HobbyLabels[h]; lbl != "" {
			hobbiesList = append(hobbiesList, fmt.Sprintf("%s(%s)", h, lbl))
		} else {
			hobbiesList = append(hobbiesList, h)
		}
	}

	return fmt.Sprintf(`你是一名通用的多主体社会模拟社会学与行为权重标定师。请为世界观「%s」（题材：%s）一次性完成全局、100%%角色无关的四大正交权重表标定。

【核心架构原则：职业接触面 与 性格修正 必须严格解耦！】
- 现实与修仙社会中，一个人每天遇到多少事件 = 职业/岗位天然人际接触面 × 场所客流 × 性格个体修正！
- 商人/坊市执事/执法巡卫本身每天就要面对海量客流与公事，即便性格内向低调也依然交集频密；灵植夫/偏远农户/闭关丹师/隐匿窃修本身岗位接触人就少，即便性格张扬热络，其基础接触面也低于坊市商人！
- 严禁把“职业导致的接触量差异”混进性格里，也严禁让性格波动完全吞没职业基础差异！

【待标定词表（必须 100%% 全覆盖，不可遗漏任何一项！）】
- 职业词表（共 %d 项）：%s
- 性格词表（共 %d 项）：%s
- 爱好词表（共 %d 项）：%s
- 允许的引擎主动行为（engine_kind）：%s
- 8 大正交应对立场：%s

【表1：profession_encounter_modifiers（职业/岗位 → 天然日常接触面基线，仅看岗位职责不看性格！）】
- 覆盖词表中**每一个职业 ID**，标定 4 个岗位暴露通道倍率（范围 0.40~1.90，1.0 表示普通中等接触面）：
  * proactive_rate：岗位本身日常需要主动与人打交道/巡查/做买卖/传令的频率（商贩/坊市执事/执法巡卫/客栈掌柜给 1.45~1.85；器师/长老/剑修给 0.85~1.20；灵植夫/农户/闭关炼丹师/矿工/暗巷窃修给 0.42~0.72）
  * target_attraction：岗位本身的可见度与被他人找上门的需求度（坊市掌柜/核心资源持有者/执法巡卫给 1.45~1.90；灵植夫/偏远杂役/潜行窃修给 0.42~0.68）
  * direct_engage：岗位职责要求当面回应他人交涉/交易/公务的程度（坊市掌柜/执法执事给 1.25~1.50；偏居耕作/闭关潜修给 0.65~0.85）
  * witness_engage：岗位身处公共场合且负有介入义务的程度（执法巡卫/掌门长老/坊市掌柜给 1.30~1.75；灵植夫/闭关丹师/暗巷窃修给 0.40~0.70）

【表2：personality_encounter_modifiers（性格 → 同职业内部的个体遭遇修正，幅度克制且与职业正交！）】
- 覆盖词表中**每一个性格词**，仅刻画“同一职业里，张扬者 vs 孤僻者”的个体偏差，范围严格控制在 **0.55~1.60**（不要给极端 0.25 或 2.3，以免吞没职业差异！）：
  * proactive_rate：同职业内主动串门/挑事的偏差（张扬高调/好管闲事/广结善缘/桀骜挑衅/鲁莽/好奇探秘给 1.25~1.55；孤僻隐世/韬光养晦/胆怯畏事/淡漠/清冷疏离给 0.55~0.75）
  * target_attraction：同职业内引人注目/招惹是非的偏差（张扬高调/桀骜挑衅/广结善缘/孤傲给 1.25~1.60；孤僻隐世/韬光养晦/清冷疏离/谦逊/淡漠给 0.55~0.75）
  * direct_engage：同职业内面对非致命交涉/挑衅时的接茬意愿（好管闲事/热络/挑衅/睚眦必报给 1.20~1.50；孤僻隐世/淡漠/清冷疏离/韬光养晦/胆怯给 0.55~0.75）
  * witness_engage：同职业内目击旁人变故时插手管闲事的偏差（好管闲事/嫉恶如仇/好奇探秘/恪职给 1.25~1.60；淡漠置身事外/孤僻隐世/胆怯畏事/韬光养晦/清冷给 0.45~0.70）

【表3：personality_stance_weights（性格 → 8大应对立场权重乘数）】
- 覆盖词表中**每一个性格词**，给出它在 8 大立场上的倍率（范围 0.30~3.20，中性项可省略或写 1.0）：
  * 积极呼应：仁厚/坦直/热情/豪爽/谦逊/随和/豁达大幅提高（1.8~2.8），狠辣/多疑/草木皆兵/淡漠/冷峻大幅压低（0.35~0.6）
  * 秉公处置：恪职/嫉恶如仇/冷峻/沉稳/坦直大幅提高（1.8~2.8），狡黠/见风使舵/狠辣/鲁莽压低（0.4~0.65）
  * 强硬反制：狠辣/睚眦必报/孤傲/好胜/桀骜挑衅/鲁莽大幅提高（2.0~3.0），胆怯/仁厚/审慎/圆滑/隐世大幅压低（0.30~0.55）
  * 谈判妥协：圆滑斡旋/审慎/豁达/随和/节俭/谦逊/沉稳提高（1.7~2.6），鲁莽/挑衅/狠辣/睚眦必报压低（0.35~0.6）
  * 借力拉拢：广结善缘/热络/仗义慷慨/好管闲事/圆滑/野心提高（1.7~2.6），孤僻隐世/清冷疏离/淡漠/多疑压低（0.35~0.6）
  * 暗中周旋：狡黠/城府/多疑/草木皆兵/隐忍沉潜/韬光养晦/精明重利大幅提高（2.0~3.0），鲁莽/坦直/张扬高调压低（0.35~0.6）
  * 趁机谋利：精明重利/见风使舵/进取野心/狡黠/狠辣大幅提高（2.0~3.0），仁厚/恪职/嫉恶如仇/淡漠/谦逊压低（0.30~0.55）
  * 避险止损：警觉避险/胆怯畏事/孤僻隐世/韬光养晦/审慎/淡漠/清冷大幅提高（2.0~3.0），鲁莽/好胜/桀骜挑衅/张扬/狠辣压低（0.30~0.55）

【表4：hobby_action_weights（爱好 → 主动行为 engine_kind 权重乘数）】
- 覆盖词表中**每一个爱好**，只使用上面列出的允许 engine_kind，契合行为给 1.8~3.0，不契合行为给 0.35~0.65。

【铁律】
- 严禁出现任何具体人名或角色 ID！
- 严格返回合法 JSON，结构如下：
{
  "summary": "职业接触面、性格遭遇修正、8立场与爱好行为标定简述（1-2句中文）",
  "profession_encounter_modifiers": {
    "market_steward": {"proactive_rate": 1.70, "target_attraction": 1.80, "direct_engage": 1.40, "witness_engage": 1.35},
    "spirit_farmer": {"proactive_rate": 0.50, "target_attraction": 0.48, "direct_engage": 0.72, "witness_engage": 0.48}
  },
  "personality_encounter_modifiers": {
    "flamboyant": {"proactive_rate": 1.45, "target_attraction": 1.55, "direct_engage": 1.30, "witness_engage": 1.35},
    "reclusive": {"proactive_rate": 0.58, "target_attraction": 0.58, "direct_engage": 0.62, "witness_engage": 0.48}
  },
  "personality_stance_weights": {
    "ruthless": {"强硬反制": 2.8, "趁机谋利": 2.0, "积极呼应": 0.4, "避险止损": 0.4}
  },
  "hobby_action_weights": {
    "sword_dueling": {"spar": 2.8, "attack": 1.5, "offer": 0.45}
  }
}`,
		theme, env.Genre,
		len(profsList), strings.Join(profsList, "、"),
		len(traitsList), strings.Join(traitsList, "、"),
		len(hobbiesList), strings.Join(hobbiesList, "、"),
		strings.Join(ont.AllowedEngineActions, ", "),
		strings.Join(DefaultReactionStances, " | "),
	)
}

// GenerateTraitCalibration sends ONE anonymized DeepSeek request to calibrate all personality
// stance weights, 4-channel event encounter modifiers, and hobby action weights for the world.
func (c Client) GenerateTraitCalibration(ctx context.Context, ont WorldOntology) (TraitCalibrationRecord, error) {
	url, err := c.chatEndpoint()
	if err != nil {
		return TraitCalibrationRecord{}, err
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return TraitCalibrationRecord{}, fmt.Errorf("optiongen: model is required")
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 32768
	}
	temp := c.Temperature
	if temp <= 0 {
		temp = 0.3
	}

	sysPrompt := BuildTraitCalibrationSystemPrompt(ont)
	userPayload := map[string]any{
		"world_id":               ont.WorldID,
		"theme":                  ont.Environment.Theme,
		"genre":                  ont.Environment.Genre,
		"personality_traits":     ont.PersonalityTraits,
		"personality_labels":     ont.PersonalityLabels,
		"hobbies":                ont.Hobbies,
		"hobby_labels":           ont.HobbyLabels,
		"allowed_engine_actions": ont.AllowedEngineActions,
		"stances":                DefaultReactionStances,
	}
	rawUser, _ := json.Marshal(userPayload)
	userPrompt := fmt.Sprintf("请根据以下匿名世界词表（不含任何具体人物姓名与ID），为全部 %d 个性格词与 %d 个爱好生成完整的 trait_calibration JSON：\n\n%s",
		len(ont.PersonalityTraits), len(ont.Hobbies), string(rawUser))

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature":     temp,
		"max_tokens":      maxTokens,
		"response_format": map[string]string{"type": "json_object"},
	}
	if effort := strings.TrimSpace(c.ReasoningEffort); effort != "" {
		reqBody["reasoning_effort"] = effort
	}
	rawReq, err := json.Marshal(reqBody)
	if err != nil {
		return TraitCalibrationRecord{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}

	started := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawReq))
	if err != nil {
		return TraitCalibrationRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return TraitCalibrationRecord{}, fmt.Errorf("request ds for trait_calibration: %w", err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return TraitCalibrationRecord{}, fmt.Errorf("read ds response for trait_calibration: %w", err)
	}
	durationMS := time.Since(started).Milliseconds()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(rawResp)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return TraitCalibrationRecord{}, fmt.Errorf("ds http %d for trait_calibration: %s", resp.StatusCode, snippet)
	}

	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rawResp, &envelope); err != nil {
		return TraitCalibrationRecord{}, fmt.Errorf("decode ds envelope for trait_calibration: %w", err)
	}
	if len(envelope.Choices) == 0 {
		return TraitCalibrationRecord{}, fmt.Errorf("ds returned empty choices for trait_calibration")
	}

	content := envelope.Choices[0].Message.Content
	out, err := decodeTraitCalibrationOutput(content)
	if err != nil {
		tail := content
		if len(tail) > 240 {
			tail = "..." + tail[len(tail)-240:]
		}
		return TraitCalibrationRecord{}, fmt.Errorf("parse ds trait_calibration (finish_reason=%s len=%d): %w | tail=%q",
			envelope.Choices[0].FinishReason, len(content), err, tail)
	}

	return TraitCalibrationRecord{
		WorldID:          ont.WorldID,
		Model:            model,
		Endpoint:         url,
		RequestedAt:      started,
		DurationMS:       durationMS,
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		TotalTokens:      envelope.Usage.TotalTokens,
		Output:           out,
		RawResponse:      content,
	}, nil
}

func decodeTraitCalibrationOutput(raw string) (TraitCalibrationOutput, error) {
	cleaned := extractJSONPayload(raw)
	var out TraitCalibrationOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return TraitCalibrationOutput{}, err
	}
	if len(out.PersonalityStanceWeights) == 0 && len(out.PersonalityEncounterModifiers) == 0 && len(out.HobbyActionWeights) == 0 {
		return TraitCalibrationOutput{}, fmt.Errorf("trait_calibration returned empty weight tables")
	}
	return out, nil
}

// GenerateDomainRules sends ONE DeepSeek request for one generic interaction domain using ONLY anonymized WorldOntology.
func (c Client) GenerateDomainRules(ctx context.Context, ont WorldOntology, domain RulebookDomainSpec) (DomainRulebookRecord, error) {
	url, err := c.chatEndpoint()
	if err != nil {
		return DomainRulebookRecord{}, err
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return DomainRulebookRecord{}, fmt.Errorf("optiongen: model is required")
	}

	ontJSON, err := json.Marshal(ont)
	if err != nil {
		return DomainRulebookRecord{}, fmt.Errorf("marshal world ontology: %w", err)
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 32768
	}
	temp := c.Temperature
	if temp <= 0 {
		temp = 0.35
	}

	sysPrompt := BuildRulebookSystemPrompt(ont, domain)
	userPrompt := fmt.Sprintf(
		"以下是当前世界的匿名本体结构（仅包含职业原型、标签、能力、地点特征与词表，不包含任何具体人物姓名与ID）。请为领域「%s」（%s）生成 100%% 通用的 proactive_rules 与 reaction_rules JSON：\n\n%s",
		domain.Title, domain.ID, string(ontJSON),
	)

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sysPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature":     temp,
		"max_tokens":      maxTokens,
		"response_format": map[string]string{"type": "json_object"},
	}
	if effort := strings.TrimSpace(c.ReasoningEffort); effort != "" {
		reqBody["reasoning_effort"] = effort
	}
	rawReq, err := json.Marshal(reqBody)
	if err != nil {
		return DomainRulebookRecord{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}

	started := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawReq))
	if err != nil {
		return DomainRulebookRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return DomainRulebookRecord{}, fmt.Errorf("request ds for domain %s: %w", domain.ID, err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return DomainRulebookRecord{}, fmt.Errorf("read ds response for domain %s: %w", domain.ID, err)
	}
	durationMS := time.Since(started).Milliseconds()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(rawResp)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return DomainRulebookRecord{}, fmt.Errorf("ds http %d for domain %s: %s", resp.StatusCode, domain.ID, snippet)
	}

	var envelope struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rawResp, &envelope); err != nil {
		return DomainRulebookRecord{}, fmt.Errorf("decode ds envelope for domain %s: %w", domain.ID, err)
	}
	if len(envelope.Choices) == 0 {
		return DomainRulebookRecord{}, fmt.Errorf("ds returned empty choices for domain %s", domain.ID)
	}

	content := envelope.Choices[0].Message.Content
	out, err := decodeDomainRulebookOutput(content, domain.ID)
	if err != nil {
		tail := content
		if len(tail) > 240 {
			tail = "..." + tail[len(tail)-240:]
		}
		return DomainRulebookRecord{}, fmt.Errorf("parse ds rulebook for domain %s (finish_reason=%s len=%d): %w | tail=%q",
			domain.ID, envelope.Choices[0].FinishReason, len(content), err, tail)
	}

	return DomainRulebookRecord{
		WorldID:          ont.WorldID,
		Domain:           domain,
		Model:            model,
		Endpoint:         url,
		RequestedAt:      started,
		DurationMS:       durationMS,
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		TotalTokens:      envelope.Usage.TotalTokens,
		Output:           out,
		RawResponse:      content,
	}, nil
}

func decodeDomainRulebookOutput(raw, domainID string) (DomainRulebookOutput, error) {
	cleaned := extractJSONPayload(raw)
	var out DomainRulebookOutput
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return DomainRulebookOutput{}, err
	}
	if out.Domain == "" {
		out.Domain = domainID
	}
	if len(out.ProactiveRules) == 0 && len(out.ReactionRules) == 0 {
		return DomainRulebookOutput{}, fmt.Errorf("domain %s returned 0 proactive and 0 reaction rules", domainID)
	}
	return out, nil
}

func sanitizeWeightMap(in map[string]float64, allowedKeys map[string]bool) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		nk := strings.ToLower(strings.TrimSpace(k))
		if nk == "" || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if len(allowedKeys) > 0 && !allowedKeys[nk] {
			continue
		}
		if v < 0.2 {
			v = 0.2
		}
		if v > 5.0 {
			v = 5.0
		}
		out[nk] = math.Round(v*100) / 100
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeNestedWeightMap(in map[string]map[string]float64, allowedOuter, allowedInner map[string]bool, lowerInner bool) map[string]map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]float64, len(in))
	for okKey, inner := range in {
		nk := strings.ToLower(strings.TrimSpace(okKey))
		if nk == "" || len(inner) == 0 {
			continue
		}
		if len(allowedOuter) > 0 && !allowedOuter[nk] {
			continue
		}
		cleanInner := map[string]float64{}
		for ik, v := range inner {
			cik := strings.TrimSpace(ik)
			if lowerInner {
				cik = strings.ToLower(cik)
			}
			if cik == "" || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			if len(allowedInner) > 0 && !allowedInner[cik] {
				continue
			}
			if v < 0.2 {
				v = 0.2
			}
			if v > 5.0 {
				v = 5.0
			}
			cleanInner[cik] = math.Round(v*100) / 100
		}
		if len(cleanInner) > 0 {
			out[nk] = cleanInner
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeNestedWeightMaps(maps []map[string]map[string]float64) map[string]map[string]float64 {
	acc := map[string]map[string][]float64{}
	for _, m := range maps {
		for outer, inner := range m {
			okKey := strings.ToLower(strings.TrimSpace(outer))
			if okKey == "" {
				continue
			}
			if acc[okKey] == nil {
				acc[okKey] = map[string][]float64{}
			}
			for ik, v := range inner {
				ikKey := strings.TrimSpace(ik)
				if ikKey == "" || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
					continue
				}
				acc[okKey][ikKey] = append(acc[okKey][ikKey], v)
			}
		}
	}
	if len(acc) == 0 {
		return nil
	}
	out := make(map[string]map[string]float64, len(acc))
	for outer, inner := range acc {
		out[outer] = make(map[string]float64, len(inner))
		for ik, vals := range inner {
			prod := 1.0
			for _, v := range vals {
				if v < 0.2 {
					v = 0.2
				}
				if v > 5.0 {
					v = 5.0
				}
				prod *= v
			}
			gm := math.Pow(prod, 1.0/float64(len(vals)))
			out[outer][ik] = math.Round(gm*100) / 100
		}
	}
	return out
}

func sanitizeEncounterFloat(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v < 0.25 {
		v = 0.25
	}
	if v > 3.0 {
		v = 3.0
	}
	return math.Round(v*100) / 100
}

func sanitizeEncounterModifiers(in map[string]agent.EncounterProfile) map[string]agent.EncounterProfile {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]agent.EncounterProfile, len(in))
	for k, ep := range in {
		nk := strings.ToLower(strings.TrimSpace(k))
		if nk == "" {
			continue
		}
		clean := agent.EncounterProfile{
			ProactiveRate:    sanitizeEncounterFloat(ep.ProactiveRate),
			TargetAttraction: sanitizeEncounterFloat(ep.TargetAttraction),
			DirectEngage:     sanitizeEncounterFloat(ep.DirectEngage),
			WitnessEngage:    sanitizeEncounterFloat(ep.WitnessEngage),
		}
		if clean.ProactiveRate == 0 && clean.TargetAttraction == 0 && clean.DirectEngage == 0 && clean.WitnessEngage == 0 {
			continue
		}
		out[nk] = clean
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeEncounterModifierMaps(maps []map[string]agent.EncounterProfile) map[string]agent.EncounterProfile {
	type bucket struct {
		pro []float64
		att []float64
		dir []float64
		wit []float64
	}
	acc := map[string]*bucket{}
	for _, m := range maps {
		for k, ep := range m {
			nk := strings.ToLower(strings.TrimSpace(k))
			if nk == "" {
				continue
			}
			b := acc[nk]
			if b == nil {
				b = &bucket{}
				acc[nk] = b
			}
			if v := sanitizeEncounterFloat(ep.ProactiveRate); v > 0 {
				b.pro = append(b.pro, v)
			}
			if v := sanitizeEncounterFloat(ep.TargetAttraction); v > 0 {
				b.att = append(b.att, v)
			}
			if v := sanitizeEncounterFloat(ep.DirectEngage); v > 0 {
				b.dir = append(b.dir, v)
			}
			if v := sanitizeEncounterFloat(ep.WitnessEngage); v > 0 {
				b.wit = append(b.wit, v)
			}
		}
	}
	if len(acc) == 0 {
		return nil
	}
	gmean := func(vals []float64) float64 {
		if len(vals) == 0 {
			return 0
		}
		prod := 1.0
		for _, v := range vals {
			prod *= v
		}
		return math.Round(math.Pow(prod, 1.0/float64(len(vals)))*100) / 100
	}
	out := make(map[string]agent.EncounterProfile, len(acc))
	for k, b := range acc {
		ep := agent.EncounterProfile{
			ProactiveRate:    gmean(b.pro),
			TargetAttraction: gmean(b.att),
			DirectEngage:     gmean(b.dir),
			WitnessEngage:    gmean(b.wit),
		}
		if ep.ProactiveRate > 0 || ep.TargetAttraction > 0 || ep.DirectEngage > 0 || ep.WitnessEngage > 0 {
			out[k] = ep
		}
	}
	return out
}

// SanitizeAndAuditRulebook validates engine actions, outcome kinds, personality/hobby weights,
// encounter modifiers, and guarantees zero actor ID/name leaks.
func SanitizeAndAuditRulebook(rb *agent.InteractionRulebook, ont WorldOntology, w *sim.World) RulebookAuditReport {
	allowedAct := map[string]bool{}
	for _, k := range ont.AllowedEngineActions {
		allowedAct[k] = true
	}
	validOut := map[string]bool{}
	for _, o := range ont.ValidOutcomeKinds {
		validOut[o] = true
	}

	// Build forbidden actor identity tokens from the actual world cast (never sent to DS).
	type forbidden struct {
		token string
		label string
	}
	var forbiddenTokens []forbidden
	if w != nil {
		for _, a := range w.Actors {
			if len(strings.TrimSpace(a.ID)) >= 3 {
				forbiddenTokens = append(forbiddenTokens, forbidden{token: a.ID, label: "actor_id:" + a.ID})
			}
			if len([]rune(strings.TrimSpace(a.Name))) >= 2 {
				forbiddenTokens = append(forbiddenTokens, forbidden{token: a.Name, label: "actor_name:" + a.Name})
			}
		}
	}

	checkActorLeaks := func(blob string) []string {
		var leaks []string
		for _, f := range forbiddenTokens {
			if strings.Contains(blob, f.token) {
				leaks = append(leaks, f.label)
			}
		}
		return leaks
	}

	var report RulebookAuditReport

	// Sanitize global personality->stance, personality->encounter, and hobby->action tables.
	rb.PersonalityStanceWeights = sanitizeNestedWeightMap(rb.PersonalityStanceWeights, nil, nil, false)
	if len(rb.PersonalityStanceWeights) > 0 {
		if raw, err := json.Marshal(rb.PersonalityStanceWeights); err == nil {
			if leaks := checkActorLeaks(string(raw)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, "personality_stance_weights->"+l)
				}
				rb.PersonalityStanceWeights = nil
			}
		}
	}
	rb.ProfessionEncounterModifiers = sanitizeEncounterModifiers(rb.ProfessionEncounterModifiers)
	if len(rb.ProfessionEncounterModifiers) > 0 {
		if raw, err := json.Marshal(rb.ProfessionEncounterModifiers); err == nil {
			if leaks := checkActorLeaks(string(raw)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, "profession_encounter_modifiers->"+l)
				}
				rb.ProfessionEncounterModifiers = nil
			}
		}
	}
	rb.PersonalityEncounterModifiers = sanitizeEncounterModifiers(rb.PersonalityEncounterModifiers)
	if len(rb.PersonalityEncounterModifiers) > 0 {
		if raw, err := json.Marshal(rb.PersonalityEncounterModifiers); err == nil {
			if leaks := checkActorLeaks(string(raw)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, "personality_encounter_modifiers->"+l)
				}
				rb.PersonalityEncounterModifiers = nil
			}
		}
	}
	rb.HobbyActionWeights = sanitizeNestedWeightMap(rb.HobbyActionWeights, nil, allowedAct, true)
	if len(rb.HobbyActionWeights) > 0 {
		if raw, err := json.Marshal(rb.HobbyActionWeights); err == nil {
			if leaks := checkActorLeaks(string(raw)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, "hobby_action_weights->"+l)
				}
				rb.HobbyActionWeights = nil
			}
		}
	}
	report.ProfessionEncounterRoles = len(rb.ProfessionEncounterModifiers)
	report.PersonalityStanceTraits = len(rb.PersonalityStanceWeights)
	report.PersonalityEncounterTraits = len(rb.PersonalityEncounterModifiers)
	report.HobbyActionKeys = len(rb.HobbyActionWeights)

	seenProIDs := map[string]bool{}
	cleanPro := make([]agent.InteractionRule, 0, len(rb.ProactiveRules))
	for i, r := range rb.ProactiveRules {
		r.Behavior.EngineKind = strings.TrimSpace(r.Behavior.EngineKind)
		if !allowedAct[r.Behavior.EngineKind] {
			report.DroppedProactiveRules++
			continue
		}
		// Filter expected outcomes to real engine kinds.
		var keptOut []string
		for _, o := range r.ExpectedOutcomes {
			o = strings.TrimSpace(o)
			if validOut[o] {
				keptOut = append(keptOut, o)
			}
		}
		r.ExpectedOutcomes = keptOut
		if r.ID == "" || seenProIDs[r.ID] {
			r.ID = fmt.Sprintf("pro_%s_%02d", nonEmptyStr(r.Domain, "gen"), i+1)
		}
		seenProIDs[r.ID] = true
		if r.Weight <= 0 {
			r.Weight = 25
		}
		if r.CooldownTicks <= 0 {
			r.CooldownTicks = 48
		}
		if r.Behavior.TargetBinding == "" {
			r.Behavior.TargetBinding = "$actor2"
		}
		r.HobbyWeights = sanitizeWeightMap(r.HobbyWeights, nil)
		if r.SharedHobbyMultiplier < 0 || math.IsNaN(r.SharedHobbyMultiplier) || math.IsInf(r.SharedHobbyMultiplier, 0) {
			r.SharedHobbyMultiplier = 0
		} else if r.SharedHobbyMultiplier > 0 {
			if r.SharedHobbyMultiplier < 0.2 {
				r.SharedHobbyMultiplier = 0.2
			}
			if r.SharedHobbyMultiplier > 5.0 {
				r.SharedHobbyMultiplier = 5.0
			}
			r.SharedHobbyMultiplier = math.Round(r.SharedHobbyMultiplier*100) / 100
		}
		if rawRule, err := json.Marshal(r); err == nil {
			if leaks := checkActorLeaks(string(rawRule)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, r.ID+"->"+l)
				}
				report.DroppedProactiveRules++
				continue
			}
		}
		if len(r.HobbyWeights) > 0 {
			report.HobbyWeightedProactiveRules++
		}
		if r.SharedHobbyMultiplier > 0 && math.Abs(r.SharedHobbyMultiplier-1.0) > 1e-6 {
			report.SharedHobbyProactiveRules++
		}
		cleanPro = append(cleanPro, r)
	}
	rb.ProactiveRules = cleanPro

	seenRxnIDs := map[string]bool{}
	cleanRxn := make([]agent.CausalReactionRule, 0, len(rb.ReactionRules))
	for i, r := range rb.ReactionRules {
		var keptTriggers []string
		for _, tk := range r.TriggerEventKinds {
			tk = strings.TrimSpace(tk)
			if tk == "*" || validOut[tk] {
				keptTriggers = append(keptTriggers, tk)
			}
		}
		if len(keptTriggers) == 0 {
			report.DroppedReactionRules++
			continue
		}
		r.TriggerEventKinds = keptTriggers

		pos := strings.ToLower(strings.TrimSpace(r.ResponderPosition))
		if pos != "witness" {
			pos = "actor2"
		}
		r.ResponderPosition = pos

		cleanBranches := make([]agent.ReactionBranch, 0, len(r.Branches))
		for j, br := range r.Branches {
			br.Behavior.EngineKind = strings.TrimSpace(br.Behavior.EngineKind)
			if !allowedAct[br.Behavior.EngineKind] {
				report.DroppedReactionBranches++
				continue
			}
			if br.ID == "" {
				br.ID = fmt.Sprintf("br_%02d", j+1)
			}
			if br.Weight <= 0 {
				br.Weight = 25
			}
			if br.Behavior.TargetBinding == "" {
				br.Behavior.TargetBinding = "$actor1"
			}
			br.PersonalityWeights = sanitizeWeightMap(br.PersonalityWeights, nil)
			if rawBr, err := json.Marshal(br); err == nil {
				if leaks := checkActorLeaks(string(rawBr)); len(leaks) > 0 {
					for _, l := range leaks {
						report.ActorLeaks = append(report.ActorLeaks, r.ID+"/"+br.ID+"->"+l)
					}
					report.DroppedReactionBranches++
					continue
				}
			}
			cleanBranches = append(cleanBranches, br)
		}
		if len(cleanBranches) == 0 {
			report.DroppedReactionRules++
			continue
		}
		r.Branches = cleanBranches
		if r.ID == "" || seenRxnIDs[r.ID] {
			r.ID = fmt.Sprintf("rxn_%s_%02d", nonEmptyStr(r.Domain, "gen"), i+1)
		}
		seenRxnIDs[r.ID] = true
		if rawRule, err := json.Marshal(r); err == nil {
			if leaks := checkActorLeaks(string(rawRule)); len(leaks) > 0 {
				for _, l := range leaks {
					report.ActorLeaks = append(report.ActorLeaks, r.ID+"->"+l)
				}
				report.DroppedReactionRules++
				continue
			}
		}
		if r.ResponderPosition == "witness" {
			report.WitnessReactionRules++
		} else {
			report.Actor2ReactionRules++
		}
		for _, br := range r.Branches {
			if len(br.PersonalityWeights) > 0 {
				report.PersonalityWeightedBranches++
			}
		}
		cleanRxn = append(cleanRxn, r)
	}
	rb.ReactionRules = cleanRxn
	report.ActorLeakCount = len(report.ActorLeaks)
	return report
}

// ExpandWorldRulebook synthesizes a 100% actor-agnostic InteractionRulebook for a WorldPack
// by calling DeepSeek concurrently for the 5 generic interaction domains + 1 global trait/encounter calibration.
func ExpandWorldRulebook(ctx context.Context, w *sim.World, client Client, cfg RulebookExpandConfig) (RulebookExpandSummary, *agent.InteractionRulebook, error) {
	if w == nil {
		return RulebookExpandSummary{}, nil, fmt.Errorf("world is nil")
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		return RulebookExpandSummary{}, nil, fmt.Errorf("out directory is required")
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 6
	}
	if err := os.MkdirAll(filepath.Join(cfg.OutDir, "domains"), 0o755); err != nil {
		return RulebookExpandSummary{}, nil, err
	}

	ont := BuildWorldOntology(w)
	domains := DefaultRulebookDomains(ont)
	started := time.Now().UTC()

	type domainResult struct {
		idx int
		rec DomainRulebookRecord
		err error
	}
	type calResult struct {
		rec TraitCalibrationRecord
		err error
	}

	sem := make(chan struct{}, concurrency)
	resCh := make(chan domainResult, len(domains))
	calCh := make(chan calResult, 1)
	var wg sync.WaitGroup
	var logMu sync.Mutex

	logProgress := func(format string, args ...any) {
		if cfg.Progress == nil {
			return
		}
		logMu.Lock()
		defer logMu.Unlock()
		fmt.Fprintf(cfg.Progress, format, args...)
	}

	// Launch dedicated global trait & encounter calibration concurrently alongside domains.
	wg.Add(1)
	go func() {
		defer wg.Done()
		calFile := filepath.Join(cfg.OutDir, "domains", "_trait_calibration.json")
		if cfg.Resume {
			if raw, err := os.ReadFile(calFile); err == nil {
				var cached TraitCalibrationRecord
				if json.Unmarshal(raw, &cached) == nil && len(cached.Output.ProfessionEncounterModifiers) > 0 && (len(cached.Output.PersonalityStanceWeights) > 0 || len(cached.Output.PersonalityEncounterModifiers) > 0) {
					logProgress("[rulebook cal] resume cached trait_calibration (%d professions, %d stance traits, %d encounter traits, %d hobbies)\n",
						len(cached.Output.ProfessionEncounterModifiers), len(cached.Output.PersonalityStanceWeights), len(cached.Output.PersonalityEncounterModifiers), len(cached.Output.HobbyActionWeights))
					calCh <- calResult{rec: cached}
					return
				}
			}
		}

		sem <- struct{}{}
		defer func() { <-sem }()

		logProgress("[rulebook cal] requesting DS for global profession/trait/encounter calibration (%d professions, %d traits, %d hobbies)...\n",
			len(ont.Glossaries.Professions), len(ont.PersonalityTraits), len(ont.Hobbies))
		var rec TraitCalibrationRecord
		var lastErr error
		for attempt := 1; attempt <= 2; attempt++ {
			rec, lastErr = client.GenerateTraitCalibration(ctx, ont)
			if lastErr == nil {
				break
			}
			if attempt < 2 && ctx.Err() == nil {
				logProgress("[rulebook cal] retry trait_calibration after error: %v\n", lastErr)
				time.Sleep(2 * time.Second)
			}
		}
		if lastErr != nil {
			calCh <- calResult{err: lastErr}
			return
		}
		if raw, err := json.MarshalIndent(rec, "", "  "); err == nil {
			_ = os.WriteFile(calFile, raw, 0o644)
		}
		logProgress("[rulebook cal] done trait_calibration: %d professions, %d stance traits, %d encounter traits, %d hobbies (%dms, %d tokens)\n",
			len(rec.Output.ProfessionEncounterModifiers), len(rec.Output.PersonalityStanceWeights), len(rec.Output.PersonalityEncounterModifiers), len(rec.Output.HobbyActionWeights), rec.DurationMS, rec.TotalTokens)
		calCh <- calResult{rec: rec}
	}()

	for i, d := range domains {
		wg.Add(1)
		go func(idx int, dom RulebookDomainSpec) {
			defer wg.Done()
			domFile := filepath.Join(cfg.OutDir, "domains", dom.ID+".json")
			if cfg.Resume {
				if raw, err := os.ReadFile(domFile); err == nil {
					var cached DomainRulebookRecord
					if json.Unmarshal(raw, &cached) == nil && (len(cached.Output.ProactiveRules) > 0 || len(cached.Output.ReactionRules) > 0) {
						logProgress("[rulebook %d/%d] resume cached %s (%d proactive, %d reaction)\n",
							idx+1, len(domains), dom.ID, len(cached.Output.ProactiveRules), len(cached.Output.ReactionRules))
						resCh <- domainResult{idx: idx, rec: cached}
						return
					}
				}
			}

			sem <- struct{}{}
			defer func() { <-sem }()

			logProgress("[rulebook %d/%d] requesting DS for generic domain %s (%s)...\n",
				idx+1, len(domains), dom.ID, dom.Title)

			var rec DomainRulebookRecord
			var lastErr error
			for attempt := 1; attempt <= 2; attempt++ {
				rec, lastErr = client.GenerateDomainRules(ctx, ont, dom)
				if lastErr == nil {
					break
				}
				if attempt < 2 && ctx.Err() == nil {
					logProgress("[rulebook %d/%d] retry %s after error: %v\n", idx+1, len(domains), dom.ID, lastErr)
					time.Sleep(2 * time.Second)
				}
			}
			if lastErr != nil {
				resCh <- domainResult{idx: idx, err: lastErr}
				return
			}
			if raw, err := json.MarshalIndent(rec, "", "  "); err == nil {
				_ = os.WriteFile(domFile, raw, 0o644)
			}
			logProgress("[rulebook %d/%d] done %s: %d proactive, %d reaction (%dms, %d tokens)\n",
				idx+1, len(domains), dom.ID, len(rec.Output.ProactiveRules), len(rec.Output.ReactionRules), rec.DurationMS, rec.TotalTokens)
			resCh <- domainResult{idx: idx, rec: rec}
		}(i, d)
	}

	wg.Wait()
	close(resCh)
	close(calCh)

	calRes := <-calCh
	records := make([]DomainRulebookRecord, len(domains))
	firstErr := calRes.err
	for r := range resCh {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		records[r.idx] = r.rec
	}
	if firstErr != nil {
		return RulebookExpandSummary{}, nil, firstErr
	}

	rb := &agent.InteractionRulebook{
		WorldID:     ont.WorldID,
		Theme:       ont.Environment.Theme,
		Genre:       ont.Environment.Genre,
		Environment: ont.Environment,
		Glossaries:  ont.Glossaries,
	}

	totalTokens := calRes.rec.TotalTokens
	domainFiles := make(map[string]string, len(records)+1)
	domainFiles["_trait_calibration"] = filepath.Join("domains", "_trait_calibration.json")
	var professionEncounterMaps []map[string]agent.EncounterProfile
	var personalityMaps []map[string]map[string]float64
	var encounterMaps []map[string]agent.EncounterProfile
	var hobbyMaps []map[string]map[string]float64

	if len(calRes.rec.Output.ProfessionEncounterModifiers) > 0 {
		professionEncounterMaps = append(professionEncounterMaps, calRes.rec.Output.ProfessionEncounterModifiers)
	}
	if len(calRes.rec.Output.PersonalityStanceWeights) > 0 {
		personalityMaps = append(personalityMaps, calRes.rec.Output.PersonalityStanceWeights)
	}
	if len(calRes.rec.Output.PersonalityEncounterModifiers) > 0 {
		encounterMaps = append(encounterMaps, calRes.rec.Output.PersonalityEncounterModifiers)
	}
	if len(calRes.rec.Output.HobbyActionWeights) > 0 {
		hobbyMaps = append(hobbyMaps, calRes.rec.Output.HobbyActionWeights)
	}

	for _, rec := range records {
		totalTokens += rec.TotalTokens
		domainFiles[rec.Domain.ID] = filepath.Join("domains", rec.Domain.ID+".json")
		if len(rec.Output.ProfessionEncounterModifiers) > 0 {
			professionEncounterMaps = append(professionEncounterMaps, rec.Output.ProfessionEncounterModifiers)
		}
		if len(rec.Output.PersonalityStanceWeights) > 0 {
			personalityMaps = append(personalityMaps, rec.Output.PersonalityStanceWeights)
		}
		if len(rec.Output.PersonalityEncounterModifiers) > 0 {
			encounterMaps = append(encounterMaps, rec.Output.PersonalityEncounterModifiers)
		}
		if len(rec.Output.HobbyActionWeights) > 0 {
			hobbyMaps = append(hobbyMaps, rec.Output.HobbyActionWeights)
		}
		for _, pr := range rec.Output.ProactiveRules {
			if pr.Domain == "" {
				pr.Domain = rec.Domain.ID
			}
			rb.ProactiveRules = append(rb.ProactiveRules, pr)
		}
		for _, rr := range rec.Output.ReactionRules {
			if rr.Domain == "" {
				rr.Domain = rec.Domain.ID
			}
			rb.ReactionRules = append(rb.ReactionRules, rr)
		}
	}
	rb.ProfessionEncounterModifiers = mergeEncounterModifierMaps(professionEncounterMaps)
	rb.PersonalityStanceWeights = mergeNestedWeightMaps(personalityMaps)
	rb.PersonalityEncounterModifiers = mergeEncounterModifierMaps(encounterMaps)
	rb.HobbyActionWeights = mergeNestedWeightMaps(hobbyMaps)

	audit := SanitizeAndAuditRulebook(rb, ont, w)

	totalBranches := 0
	for _, rr := range rb.ReactionRules {
		totalBranches += len(rr.Branches)
	}

	rulebookPath := filepath.Join(cfg.OutDir, "rulebook.json")
	rbRaw, err := json.MarshalIndent(rb, "", "  ")
	if err != nil {
		return RulebookExpandSummary{}, nil, err
	}
	if err := os.WriteFile(rulebookPath, rbRaw, 0o644); err != nil {
		return RulebookExpandSummary{}, nil, err
	}

	summary := RulebookExpandSummary{
		WorldID:             ont.WorldID,
		Theme:               ont.Environment.Theme,
		Genre:               ont.Environment.Genre,
		Model:               client.Model,
		Endpoint:            client.Endpoint,
		StartedAt:           started,
		CompletedAt:         time.Now().UTC(),
		Domains:             len(domains),
		TotalRequests:       len(domains) + 1,
		TotalTokens:         totalTokens,
		TotalProactiveRules: len(rb.ProactiveRules),
		TotalReactionRules:  len(rb.ReactionRules),
		TotalBranches:       totalBranches,
		Audit:               audit,
		RulebookFile:        "rulebook.json",
		DomainFiles:         domainFiles,
	}
	sumRaw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return RulebookExpandSummary{}, nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "summary.json"), sumRaw, 0o644); err != nil {
		return RulebookExpandSummary{}, nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "overview.md"), []byte(renderRulebookOverviewMD(summary, rb)), 0o644); err != nil {
		return RulebookExpandSummary{}, nil, err
	}

	return summary, rb, nil
}

func renderRulebookOverviewMD(sum RulebookExpandSummary, rb *agent.InteractionRulebook) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 通用角色交互因果规则库 — %s (%s)\n\n", nonEmptyStr(sum.Theme, sum.WorldID), sum.WorldID)
	fmt.Fprintf(&b, "- 离线合成请求数：%d 次（%d 个交互领域 + 1 次全局职业/性格/爱好解耦标定，与人口规模无关）\n", sum.TotalRequests, sum.Domains)
	fmt.Fprintf(&b, "- 通用主动交互规则：%d 条（含爱好权重：%d 条｜含同好加成：%d 条｜全局爱好→行为表：%d 项）\n",
		sum.TotalProactiveRules, sum.Audit.HobbyWeightedProactiveRules, sum.Audit.SharedHobbyProactiveRules, sum.Audit.HobbyActionKeys)
	fmt.Fprintf(&b, "- 通用因果反应规则：%d 条（当事角色2：%d｜同场目击者：%d｜总反应分支：%d｜含性格权重分支：%d｜职业暴露基线：%d 项｜性格→8立场：%d 项｜性格→遭遇修正：%d 项）\n",
		sum.TotalReactionRules, sum.Audit.Actor2ReactionRules, sum.Audit.WitnessReactionRules, sum.TotalBranches,
		sum.Audit.PersonalityWeightedBranches, sum.Audit.ProfessionEncounterRoles, sum.Audit.PersonalityStanceTraits, sum.Audit.PersonalityEncounterTraits)
	fmt.Fprintf(&b, "- 角色名/ID硬编码泄漏审计：%d 处（已自动剔除/零泄漏）\n\n", sum.Audit.ActorLeakCount)

	b.WriteString("## 主动交互规则（角色1[爱好权重] → 行为 → 角色2 ⇒ 引擎结果）\n\n")
	for _, r := range rb.ProactiveRules {
		fmt.Fprintf(&b, "- **[%s] %s**（%s｜`%s` → `%s`｜爱好权重:%d｜同好:×%.2f）：%s\n",
			r.ID, r.Name, r.Category, r.Behavior.EngineKind, strings.Join(r.ExpectedOutcomes, "/"),
			len(r.HobbyWeights), r.SharedHobbyMultiplier, r.CausalSummary)
	}
	b.WriteString("\n## 因果反应规则（引擎结果 ⇒ 角色2/目击者[性格权重]分支应对）\n\n")
	for _, r := range rb.ReactionRules {
		fmt.Fprintf(&b, "- **[%s] %s**（触发：`%s`｜立场位置：`%s`｜分支数：%d）：%s\n",
			r.ID, r.Name, strings.Join(r.TriggerEventKinds, "/"), r.ResponderPosition, len(r.Branches), r.CausalSummary)
	}
	return b.String()
}

func nonEmptyStr(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

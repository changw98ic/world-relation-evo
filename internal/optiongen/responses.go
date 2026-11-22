package optiongen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CounterOption is a reactive decision branch generated for one participant
// when a high-likelihood cross-actor combination scenario triggers.
type CounterOption struct {
	ID                   string           `json:"id"`
	ResponderActorID     string           `json:"responder_actor_id"`
	ResponderActorName   string           `json:"responder_actor_name"`
	Stance               string           `json:"stance"`
	Title                string           `json:"title"`
	TriggerCondition     string           `json:"trigger_condition"`
	ProbabilityHint      string           `json:"probability_hint"`
	ProbabilityRationale string           `json:"probability_rationale"`
	CounterToOptionIDs   []string         `json:"counter_to_option_ids,omitempty"`
	Action               OptionActionSpec `json:"action"`
	CounterEffects       []string         `json:"counter_effects"`
	EscalationHooks      []string         `json:"escalation_hooks"`
}

// ScenarioResponseCatalog is the structured counter-option set returned by DeepSeek
// for a single high-likelihood multi-actor combination scenario.
type ScenarioResponseCatalog struct {
	ScenarioID       string          `json:"scenario_id"`
	ScenarioTitle    string          `json:"scenario_title"`
	CollisionSummary string          `json:"collision_summary"`
	Stakes           []string        `json:"stakes"`
	Responses        []CounterOption `json:"responses"`
}

// ScenarioResponseRecord stores the full audit trail for one scenario's DS call.
type ScenarioResponseRecord struct {
	WorldID          string                  `json:"world_id"`
	Scenario         CombinationScenario     `json:"scenario"`
	Model            string                  `json:"model"`
	Endpoint         string                  `json:"endpoint"`
	RequestedAt      time.Time               `json:"requested_at"`
	DurationMS       int64                   `json:"duration_ms"`
	PromptTokens     int                     `json:"prompt_tokens,omitempty"`
	CompletionTokens int                     `json:"completion_tokens,omitempty"`
	TotalTokens      int                     `json:"total_tokens,omitempty"`
	Catalog          ScenarioResponseCatalog `json:"catalog"`
	RawResponse      string                  `json:"raw_response"`
}

// ActorResponseIndex aggregates all reactive counter-options that belong to one actor
// across every mined combination scenario, ready for downstream if-else rule compilation.
type ActorResponseIndex struct {
	WorldID       string                    `json:"world_id"`
	ActorID       string                    `json:"actor_id"`
	ActorName     string                    `json:"actor_name"`
	Role          string                    `json:"role"`
	Location      string                    `json:"location"`
	ScenarioCount int                       `json:"scenario_count"`
	ResponseCount int                       `json:"response_count"`
	Scenarios     []ActorScenarioCounterSet `json:"scenarios"`
}

type ActorScenarioCounterSet struct {
	ScenarioID       string          `json:"scenario_id"`
	ScenarioTitle    string          `json:"scenario_title"`
	CollisionType    string          `json:"collision_type"`
	LikelihoodScore  int             `json:"likelihood_score"`
	LikelihoodTier   string          `json:"likelihood_tier"`
	DayWindow        [2]int          `json:"day_window"`
	CollisionSummary string          `json:"collision_summary"`
	Responses        []CounterOption `json:"responses"`
}

// WorldResponseSummary summarizes the full combination-response expansion run.
type WorldResponseSummary struct {
	WorldID        string                  `json:"world_id"`
	OptionsDir     string                  `json:"options_dir"`
	Model          string                  `json:"model"`
	Endpoint       string                  `json:"endpoint"`
	StartedAt      time.Time               `json:"started_at"`
	CompletedAt    time.Time               `json:"completed_at"`
	TotalScenarios int                     `json:"total_scenarios"`
	TotalRequests  int                     `json:"total_requests"`
	TotalResponses int                     `json:"total_responses"`
	TotalTokens    int                     `json:"total_tokens"`
	ByStance       map[string]int          `json:"by_stance"`
	ByCollision    map[string]int          `json:"by_collision"`
	Scenarios      []ScenarioResponseBrief `json:"scenarios"`
}

type ScenarioResponseBrief struct {
	ScenarioID      string         `json:"scenario_id"`
	Title           string         `json:"title"`
	CollisionType   string         `json:"collision_type"`
	LikelihoodScore int            `json:"likelihood_score"`
	LikelihoodTier  string         `json:"likelihood_tier"`
	DayWindow       [2]int         `json:"day_window"`
	Participants    []string       `json:"participants"`
	ResponseCount   int            `json:"response_count"`
	ByStance        map[string]int `json:"by_stance"`
	DurationMS      int64          `json:"duration_ms"`
	TotalTokens     int            `json:"total_tokens,omitempty"`
	OutputFile      string         `json:"output_file"`
}

// ResponseExpandConfig controls offline combination-driven counter-option generation.
type ResponseExpandConfig struct {
	OptionsDir  string
	OutDir      string
	TopN        int
	Concurrency int
	Resume      bool
	Progress    io.Writer
}

// BuildResponseSystemPrompt builds a setting-agnostic system prompt for generating
// reactive counter-options from a mined multi-actor combination scenario.
func BuildResponseSystemPrompt(world WorldContextSummary, scenario CombinationScenario, actionKinds []string) string {
	env := world.Environment
	theme := env.Theme
	if theme == "" {
		theme = world.ID
	}
	actorNoun := env.ActorNoun
	if actorNoun == "" {
		actorNoun = "角色"
	}
	currency := env.CurrencyName
	if currency == "" {
		currency = "货币"
	}
	authority := env.AuthorityName
	if authority == "" {
		authority = "秩序机构"
	}
	oversight := env.OversightName
	if oversight == "" {
		oversight = "监察机构"
	}

	var worldDesc strings.Builder
	fmt.Fprintf(&worldDesc, "- 世界主题：%s（题材：%s）\n", theme, env.Genre)
	if env.Summary != "" {
		fmt.Fprintf(&worldDesc, "- 世界生态：%s\n", env.Summary)
	}
	fmt.Fprintf(&worldDesc, "- 货币单位：%s｜角色称谓：%s｜执法/秩序机构：%s｜监察/问责机构：%s\n",
		currency, actorNoun, authority, oversight)

	var disabledInst []string
	if !env.Institutions.TaxationEnabled() {
		disabledInst = append(disabledInst, "税赋征收")
	}
	if !env.Institutions.EnforcementEnabled() {
		disabledInst = append(disabledInst, "强制缉拿/执法")
	}
	if !env.Institutions.OversightEnabled() {
		disabledInst = append(disabledInst, "监察审计/申诉")
	}
	if !env.Institutions.RacketeeringEnabled() {
		disabledInst = append(disabledInst, "保护费/强行勒索")
	}
	if !env.Institutions.RevoltEnabled() {
		disabledInst = append(disabledInst, "民变暴动")
	}
	if !env.Institutions.RitualsEnabled() {
		disabledInst = append(disabledInst, "公开仪式/祭典")
	}
	if len(disabledInst) > 0 {
		fmt.Fprintf(&worldDesc, "- 本世界已关闭制度：%s（严禁围绕这些已关闭制度设计应对选项）\n", strings.Join(disabledInst, "、"))
	}

	gloss := world.Glossaries
	if s := formatGlossaryLine(gloss.Locations); s != "" {
		fmt.Fprintf(&worldDesc, "- 地点中英对照：%s\n", s)
	}
	if s := formatGlossaryLine(gloss.Professions); s != "" {
		fmt.Fprintf(&worldDesc, "- 职业/身份对照：%s\n", s)
	}
	if s := formatGlossaryLine(gloss.Items); s != "" {
		fmt.Fprintf(&worldDesc, "- 物资/道具对照：%s\n", s)
	}
	if s := formatGlossaryLine(gloss.Factions); s != "" {
		fmt.Fprintf(&worldDesc, "- 势力/组织对照：%s\n", s)
	}
	if s := formatGlossaryLine(gloss.Recipes); s != "" {
		fmt.Fprintf(&worldDesc, "- 已有生产/炼制配方（使用 craft 并在 action.recipe_id 填入配方ID）：%s\n", s)
	}
	if len(actionKinds) > 0 {
		fmt.Fprintf(&worldDesc, "- 当前世界可用引擎动作（engine_action_kinds）：%s\n", strings.Join(actionKinds, "，"))
	}

	sampleRespID := "actor_a"
	sampleRespName := "某位当事" + actorNoun
	sampleTargetID := "actor_b"
	sampleLoc := scenario.SharedLocation
	if len(scenario.Participants) > 0 {
		sampleRespID = scenario.Participants[0].ActorID
		sampleRespName = scenario.Participants[0].ActorName
		if sampleLoc == "" {
			sampleLoc = scenario.Participants[0].Location
		}
	}
	if len(scenario.Participants) > 1 {
		sampleTargetID = scenario.Participants[1].ActorID
	}
	perActorTarget := "2 到 3 条"
	totalTarget := len(scenario.Participants) * 3
	if totalTarget < 6 {
		totalTarget = 6
	}

	return fmt.Sprintf(`你是一个通用世界模拟引擎的“组合碰撞应对选项设计师”。
你的任务：基于一个已经挖掘出的高概率多%s选项组合局面，为该局面中**每一位涉事%s**分别设计 %s 差异化的“应对选项”（Counter-Options），共约 %d 条，使其可直接编译为底层引擎的响应式 if-else 分支。请直接高效地输出合法 JSON，无需冗长内部推演。

当前世界环境与术语表（必须严格遵循本世界设定，严禁串味或引入不属于本世界的词汇）：
%s
严格要求（违反任意一条都视为不合格）：
1. **100%% 贴合当前世界观的简体中文人话**：
   - scenario_title、collision_summary、stakes、title、trigger_condition、probability_rationale、action.summary、counter_effects、escalation_hooks **必须 100%% 使用自然、通顺且贴合本世界观的简体中文**。
   - 提及人物必须写档案中的**真实中文姓名**，提及地点/物品/职业/势力必须写上方对照表中的**中文名**，提及货币必须写“%s”。
   - **严禁**在中文可读文本中出现英文变量名、英文蛇形标识符、表达式语法或任何内部编号（如 opt_01、scn_01、rsp_01 等）。
   - 只有底层机器字段（scenario_id、responder_actor_id、counter_to_option_ids、action.engine_kind、action.recipe_id、action.target_actor_ids、action.target_location、action.items_or_resources）使用档案中的真实 ID。
2. **必须真正“应对组合碰撞”，而非重复原选项**：
   - 每条应对选项都必须针对“对方已经亮出或即将落地某个动作”的组合态势，给出具有博弈深度的后手反应。
   - 同一位涉事%s的 2 到 3 条应对选项，必须在 stance（应对姿态）上拉开明显差异，只从以下 5 种标准中文姿态选一：
     `+"`强硬反制`"+` | `+"`谈判妥协`"+` | `+"`暗中化解`"+` | `+"`借力拉拢`"+` | `+"`避险止损`"+`
   - 结合该%s的真实性格、手头筹码与人脉关系，如实评估每条应对的选中概率（高/中/低）并给出一句动机依据。
3. **动作原语与配方映射规范**：
   - 若应对动作涉及上方“已有生产/炼制配方”，action.engine_kind **必须填 "craft"**，并在 action.recipe_id 填入对应配方 ID。
   - 只允许使用上方 engine_action_kinds 列出的动作，包括已声明的 ActionSpecs。超出该列表的策略请拆成已有动作。
   - 严禁凭空捏造档案中不存在的人物、地点、物品或组织。

输出格式（必须且只能返回一个合法 JSON 对象，不要附加任何解释）：
{
  "scenario_id": %q,
  "scenario_title": "精炼中文局面标题（18字以内，不含英文或编号）",
  "collision_summary": "两三句白话讲清这几位%s为何会在该时间窗撞在一起、核心冲突或联动点是什么",
  "stakes": ["各方核心赌注或利害1", "各方核心赌注或利害2"],
  "responses": [
    {
      "id": "rsp_01",
      "responder_actor_id": %q,
      "responder_actor_name": %q,
      "stance": "强硬反制|谈判妥协|暗中化解|借力拉拢|避险止损",
      "title": "简短醒目的中文应对标题（12字以内）",
      "trigger_condition": "用自然中文写清何时触发该应对（例如：当某人亲赴某地采取某行动且己方某条件成立时）",
      "probability_hint": "高|中|低",
      "probability_rationale": "结合该%s性格、处境与筹码，用一句自然中文说明为何该应对选中概率高/中/低",
      "counter_to_option_ids": ["对方触发选项的真实option_id"],
      "action": {
        "engine_kind": "对应 engine_action_kinds 列出的动作名",
        "recipe_id": "可选：仅当 engine_kind 为 craft 时填入档案中真实的 recipe_id",
        "target_actor_ids": [%q],
        "target_location": %q,
        "items_or_resources": {"真实item_id": 1},
        "summary": "用自然中文说明具体如何后手应对、找谁配合、拿什么筹码"
      },
      "counter_effects": ["执行后的直接局势变化1", "直接局势变化2"],
      "escalation_hooks": ["若对方继续加码或识破后的二阶演化1", "二阶演化2"]
    }
  ]
}`,
		actorNoun, actorNoun, perActorTarget, totalTarget,
		worldDesc.String(),
		currency,
		actorNoun, actorNoun,
		scenario.ID,
		actorNoun,
		sampleRespID, sampleRespName,
		actorNoun,
		sampleTargetID, sampleLoc,
	)
}

// ExpandScenarioResponses sends one DS request for a single CombinationScenario
// and returns the full ScenarioResponseRecord.
func (c Client) ExpandScenarioResponses(
	ctx context.Context,
	world WorldContextSummary,
	scenario CombinationScenario,
	actionKinds []string,
	rosterBrief []map[string]string,
) (ScenarioResponseRecord, error) {
	url, err := c.chatEndpoint()
	if err != nil {
		return ScenarioResponseRecord{}, err
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return ScenarioResponseRecord{}, errors.New("WRE_MODEL (model) is required")
	}

	payload := map[string]any{
		"world_id":            world.ID,
		"horizon_days":        world.HorizonDays,
		"scenario":            scenario,
		"world_actor_roster":  rosterBrief,
		"engine_action_kinds": actionKinds,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return ScenarioResponseRecord{}, fmt.Errorf("marshal scenario payload %s: %w", scenario.ID, err)
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 16384
	}
	temp := c.Temperature
	if temp <= 0 {
		temp = 0.4
	}

	sysPrompt := BuildResponseSystemPrompt(world, scenario, actionKinds)
	userPrompt := fmt.Sprintf(
		"以下是高概率跨角色组合局面 %s（综合发生概率 %d/100，时间窗第 %d–%d 天，碰撞类型：%s）的完整上下文。请严格使用贴合本世界观的简体中文人话（严禁英文变量或内部编号），为每一位涉事角色生成 2 到 3 条差异化应对选项（JSON）：\n\n%s",
		scenario.ID, scenario.LikelihoodScore, scenario.DayWindow[0], scenario.DayWindow[1], scenario.CollisionType, string(payloadJSON),
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
		return ScenarioResponseRecord{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}

	started := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawReq))
	if err != nil {
		return ScenarioResponseRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ScenarioResponseRecord{}, fmt.Errorf("request ds for %s: %w", scenario.ID, err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return ScenarioResponseRecord{}, fmt.Errorf("read ds response for %s: %w", scenario.ID, err)
	}
	durationMS := time.Since(started).Milliseconds()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(rawResp)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return ScenarioResponseRecord{}, fmt.Errorf("ds http %d for %s: %s", resp.StatusCode, scenario.ID, snippet)
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
		return ScenarioResponseRecord{}, fmt.Errorf("decode ds envelope for %s: %w", scenario.ID, err)
	}
	if len(envelope.Choices) == 0 {
		return ScenarioResponseRecord{}, fmt.Errorf("ds returned empty choices for %s", scenario.ID)
	}

	content := envelope.Choices[0].Message.Content
	cat, err := decodeResponseCatalog(content, scenario)
	if err != nil {
		tail := content
		if len(tail) > 240 {
			tail = "..." + tail[len(tail)-240:]
		}
		return ScenarioResponseRecord{}, fmt.Errorf("parse ds responses for %s (finish_reason=%s len=%d tokens=%d/%d): %w | tail=%q",
			scenario.ID, envelope.Choices[0].FinishReason, len(content),
			envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, err, tail)
	}

	return ScenarioResponseRecord{
		WorldID:          world.ID,
		Scenario:         scenario,
		Model:            model,
		Endpoint:         url,
		RequestedAt:      started,
		DurationMS:       durationMS,
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		TotalTokens:      envelope.Usage.TotalTokens,
		Catalog:          cat,
		RawResponse:      content,
	}, nil
}

func decodeResponseCatalog(raw string, scenario CombinationScenario) (ScenarioResponseCatalog, error) {
	cleaned := extractJSONPayload(raw)
	if cleaned == "" {
		return ScenarioResponseCatalog{}, errors.New("empty json payload")
	}
	var cat ScenarioResponseCatalog
	if err := json.Unmarshal([]byte(cleaned), &cat); err != nil {
		return ScenarioResponseCatalog{}, err
	}
	if len(cat.Responses) == 0 {
		return ScenarioResponseCatalog{}, errors.New("returned responses list is empty")
	}
	if cat.ScenarioID == "" {
		cat.ScenarioID = scenario.ID
	}
	if cat.ScenarioTitle == "" {
		cat.ScenarioTitle = scenario.Title
	}
	nameByID := map[string]string{}
	for _, p := range scenario.Participants {
		nameByID[p.ActorID] = p.ActorName
	}
	for i := range cat.Responses {
		if cat.Responses[i].ID == "" {
			cat.Responses[i].ID = fmt.Sprintf("rsp_%02d", i+1)
		}
		if cat.Responses[i].ResponderActorName == "" {
			cat.Responses[i].ResponderActorName = nameByID[cat.Responses[i].ResponderActorID]
		}
	}
	return cat, nil
}

// ExpandResponses mines the top high-likelihood multi-actor combinations from
// existing character option records, requests DS once per combination scenario
// to generate reactive counter-options, and persists both per-scenario and
// per-actor indexed outputs.
func ExpandResponses(ctx context.Context, records []ActorExpansionRecord, client Client, cfg ResponseExpandConfig) (*WorldResponseSummary, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("need at least 2 actor records to mine option combinations")
	}
	if cfg.TopN <= 0 {
		cfg.TopN = 12
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if strings.TrimSpace(cfg.OutDir) == "" {
		return nil, fmt.Errorf("output directory is required")
	}

	scenariosDir := filepath.Join(cfg.OutDir, "scenarios")
	byActorDir := filepath.Join(cfg.OutDir, "by_actor")
	if err := os.MkdirAll(scenariosDir, 0o755); err != nil {
		return nil, fmt.Errorf("create scenarios dir: %w", err)
	}
	if err := os.MkdirAll(byActorDir, 0o755); err != nil {
		return nil, fmt.Errorf("create by_actor dir: %w", err)
	}

	worldCtx := records[0].Dossier.World
	actionKinds := records[0].Dossier.EngineActionKinds
	rosterBrief := make([]map[string]string, 0, len(records))
	for _, r := range records {
		rosterBrief = append(rosterBrief, map[string]string{
			"id":       r.ActorID,
			"name":     r.ActorName,
			"role":     glossaryLookup(worldCtx.Glossaries.Professions, r.Dossier.Actor.Role),
			"location": glossaryLookup(worldCtx.Glossaries.Locations, r.Dossier.Actor.Location),
		})
	}

	scenarios := MineHighLikelihoodCombinations(records, MineConfig{TopN: cfg.TopN})
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no high-likelihood cross-actor combinations discovered")
	}
	if err := writeJSONFile(filepath.Join(cfg.OutDir, "combinations.json"), scenarios); err != nil {
		return nil, err
	}

	cachedBySig := map[string]ScenarioResponseRecord{}
	if cfg.Resume {
		cachedBySig = loadCachedScenarioResponses(scenariosDir)
	}

	started := time.Now().UTC()
	respRecords := make([]ScenarioResponseRecord, len(scenarios))
	errs := make([]error, len(scenarios))

	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup
	var logMu sync.Mutex

	for i, scn := range scenarios {
		wg.Add(1)
		go func(idx int, s CombinationScenario) {
			defer wg.Done()

			outPath := filepath.Join(scenariosDir, s.ID+".json")
			if cfg.Resume {
				if existing, ok := cachedBySig[ScenarioTriggerSignature(s)]; ok {
					existing.Scenario = s
					existing.Catalog.ScenarioID = s.ID
					_ = writeJSONFile(outPath, existing)
					respRecords[idx] = existing
					if cfg.Progress != nil {
						logMu.Lock()
						fmt.Fprintf(cfg.Progress, "[%d/%d] resumed %s (%s): %d responses (matched signature)\n",
							idx+1, len(scenarios), s.ID, s.Title, len(existing.Catalog.Responses))
						logMu.Unlock()
					}
					return
				}
			}

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}

			if cfg.Progress != nil {
				logMu.Lock()
				fmt.Fprintf(cfg.Progress, "[%d/%d] requesting ds for %s (score=%d, %s: %s)...\n",
					idx+1, len(scenarios), s.ID, s.LikelihoodScore, s.CollisionType, s.Title)
				logMu.Unlock()
			}

			rec, err := client.ExpandScenarioResponses(ctx, worldCtx, s, actionKinds, rosterBrief)
			if err != nil {
				errs[idx] = err
				if cfg.Progress != nil {
					logMu.Lock()
					fmt.Fprintf(cfg.Progress, "[%d/%d] ERROR %s: %v\n", idx+1, len(scenarios), s.ID, err)
					logMu.Unlock()
				}
				return
			}
			if err := writeJSONFile(outPath, rec); err != nil {
				errs[idx] = err
				return
			}
			respRecords[idx] = rec

			if cfg.Progress != nil {
				logMu.Lock()
				fmt.Fprintf(cfg.Progress, "[%d/%d] done %s: %d responses, %d tokens, %dms -> %s\n",
					idx+1, len(scenarios), s.ID, len(rec.Catalog.Responses), rec.TotalTokens, rec.DurationMS, outPath)
				logMu.Unlock()
			}
		}(i, scn)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("scenario %s failed: %w", scenarios[i].ID, err)
		}
	}

	endpointURL, _ := client.chatEndpoint()
	summary := &WorldResponseSummary{
		WorldID:        worldCtx.ID,
		OptionsDir:     cfg.OptionsDir,
		Model:          client.Model,
		Endpoint:       endpointURL,
		StartedAt:      started,
		CompletedAt:    time.Now().UTC(),
		TotalScenarios: len(scenarios),
		TotalRequests:  len(scenarios),
		ByStance:       map[string]int{},
		ByCollision:    map[string]int{},
		Scenarios:      make([]ScenarioResponseBrief, 0, len(scenarios)),
	}

	actorIndexes := make(map[string]*ActorResponseIndex, len(records))
	for _, r := range records {
		actorIndexes[r.ActorID] = &ActorResponseIndex{
			WorldID:   worldCtx.ID,
			ActorID:   r.ActorID,
			ActorName: r.ActorName,
			Role:      r.Dossier.Actor.Role,
			Location:  r.Dossier.Actor.Location,
		}
	}

	for _, rec := range respRecords {
		byStance := map[string]int{}
		groupedByActor := map[string][]CounterOption{}
		for _, rsp := range rec.Catalog.Responses {
			st := rsp.Stance
			if st == "" {
				st = "未分类"
			}
			byStance[st]++
			summary.ByStance[st]++
			groupedByActor[rsp.ResponderActorID] = append(groupedByActor[rsp.ResponderActorID], rsp)
		}
		summary.ByCollision[rec.Scenario.CollisionType]++
		summary.TotalResponses += len(rec.Catalog.Responses)
		summary.TotalTokens += rec.TotalTokens

		partNames := make([]string, 0, len(rec.Scenario.Participants))
		for _, p := range rec.Scenario.Participants {
			partNames = append(partNames, p.ActorName)
		}
		summary.Scenarios = append(summary.Scenarios, ScenarioResponseBrief{
			ScenarioID:      rec.Scenario.ID,
			Title:           rec.Catalog.ScenarioTitle,
			CollisionType:   rec.Scenario.CollisionType,
			LikelihoodScore: rec.Scenario.LikelihoodScore,
			LikelihoodTier:  rec.Scenario.LikelihoodTier,
			DayWindow:       rec.Scenario.DayWindow,
			Participants:    partNames,
			ResponseCount:   len(rec.Catalog.Responses),
			ByStance:        byStance,
			DurationMS:      rec.DurationMS,
			TotalTokens:     rec.TotalTokens,
			OutputFile:      filepath.Join("scenarios", rec.Scenario.ID+".json"),
		})

		actorIDs := make([]string, 0, len(groupedByActor))
		for aid := range groupedByActor {
			actorIDs = append(actorIDs, aid)
		}
		sort.Strings(actorIDs)
		for _, aid := range actorIDs {
			idxObj, ok := actorIndexes[aid]
			if !ok {
				continue
			}
			list := groupedByActor[aid]
			idxObj.ScenarioCount++
			idxObj.ResponseCount += len(list)
			idxObj.Scenarios = append(idxObj.Scenarios, ActorScenarioCounterSet{
				ScenarioID:       rec.Scenario.ID,
				ScenarioTitle:    rec.Catalog.ScenarioTitle,
				CollisionType:    rec.Scenario.CollisionType,
				LikelihoodScore:  rec.Scenario.LikelihoodScore,
				LikelihoodTier:   rec.Scenario.LikelihoodTier,
				DayWindow:        rec.Scenario.DayWindow,
				CollisionSummary: rec.Catalog.CollisionSummary,
				Responses:        list,
			})
		}
	}

	for _, r := range records {
		if err := writeJSONFile(filepath.Join(byActorDir, r.ActorID+".json"), actorIndexes[r.ActorID]); err != nil {
			return nil, err
		}
	}
	if err := writeJSONFile(filepath.Join(cfg.OutDir, "summary.json"), summary); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "overview.md"), []byte(renderResponseOverviewMarkdown(summary, respRecords)), 0o644); err != nil {
		return nil, err
	}
	return summary, nil
}

func loadCachedScenarioResponses(scenariosDir string) map[string]ScenarioResponseRecord {
	out := map[string]ScenarioResponseRecord{}
	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(scenariosDir, entry.Name()))
		if err != nil {
			continue
		}
		var rec ScenarioResponseRecord
		if err := json.Unmarshal(raw, &rec); err != nil || len(rec.Catalog.Responses) == 0 {
			continue
		}
		sig := ScenarioTriggerSignature(rec.Scenario)
		if sig != "" {
			out[sig] = rec
		}
	}
	return out
}

func renderResponseOverviewMarkdown(s *WorldResponseSummary, records []ScenarioResponseRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — 高概率选项组合碰撞与应对总览\n\n", s.WorldID)
	fmt.Fprintf(&b, "- 模型：`%s`\n", s.Model)
	fmt.Fprintf(&b, "- 高概率组合局面数 / DS 请求数：`%d` / `%d`\n", s.TotalScenarios, s.TotalRequests)
	fmt.Fprintf(&b, "- 生成应对选项总数：`%d`\n", s.TotalResponses)
	fmt.Fprintf(&b, "- 总 Token 消耗：`%d`\n\n", s.TotalTokens)

	b.WriteString("| 局面 ID | 碰撞类型 | 概率评分 | 时间窗 | 涉事角色 | 局面标题 | 应对数 |\n")
	b.WriteString("| --- | --- | ---: | --- | --- | --- | ---: |\n")
	for _, sc := range s.Scenarios {
		fmt.Fprintf(&b, "| `%s` | %s | %d (%s) | D%d-D%d | %s | %s | %d |\n",
			sc.ScenarioID, sc.CollisionType, sc.LikelihoodScore, sc.LikelihoodTier,
			sc.DayWindow[0], sc.DayWindow[1], strings.Join(sc.Participants, "、"),
			escapePipe(sc.Title), sc.ResponseCount)
	}

	for _, rec := range records {
		fmt.Fprintf(&b, "\n## `%s` — %s（%s · 概率 %d）\n\n",
			rec.Scenario.ID, rec.Catalog.ScenarioTitle, rec.Scenario.CollisionType, rec.Scenario.LikelihoodScore)
		if rec.Catalog.CollisionSummary != "" {
			fmt.Fprintf(&b, "> %s\n\n", rec.Catalog.CollisionSummary)
		}
		b.WriteString("| 应对者 | 姿态 | 选中概率 | 标题 | 触发条件 | 动作原语 |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, rsp := range rec.Catalog.Responses {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | `%s` |\n",
				rsp.ResponderActorName, rsp.Stance, rsp.ProbabilityHint,
				escapePipe(rsp.Title), escapePipe(rsp.TriggerCondition), rsp.Action.EngineKind)
		}
	}
	return b.String()
}

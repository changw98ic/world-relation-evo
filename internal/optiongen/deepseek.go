package optiongen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Client calls an OpenAI-compatible DeepSeek endpoint once per character to
// expand that character's complete 30-day behavioral option catalog offline.
type Client struct {
	Endpoint        string
	Model           string
	APIKey          string
	HTTPClient      *http.Client
	MaxTokens       int
	Temperature     float64
	ReasoningEffort string
}

// BuildSystemPrompt constructs a setting-agnostic, world-aware system prompt
// strictly from the loaded world's EnvironmentProfile and localized Glossaries.
func BuildSystemPrompt(dossier CharacterDossier) string {
	horizon := dossier.World.HorizonDays
	if horizon <= 0 {
		horizon = 30
	}
	env := dossier.World.Environment
	theme := env.Theme
	if theme == "" {
		theme = dossier.World.ID
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
		fmt.Fprintf(&worldDesc, "- 本世界已关闭制度：%s（严禁围绕这些已关闭制度编造选项）\n", strings.Join(disabledInst, "、"))
	}

	gloss := dossier.World.Glossaries
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
	if s := formatGlossaryLine(env.Terminology); s != "" {
		fmt.Fprintf(&worldDesc, "- 术语与性格译法参考：%s\n", s)
	}

	sampleActorID := dossier.Actor.ID
	sampleActorName := dossier.Actor.Name
	sampleOtherID := ""
	sampleOtherName := "某位同行"
	if len(dossier.OtherActors) > 0 {
		sampleOtherID = dossier.OtherActors[0].ID
		sampleOtherName = dossier.OtherActors[0].Name
	}
	sampleLocID := dossier.Actor.Location
	if sampleLocID == "" && len(dossier.Districts) > 0 {
		sampleLocID = dossier.Districts[0].ID
	}

	return fmt.Sprintf(`你是一个通用世界模拟引擎的“离线行为选项扩充设计师”。
你的任务：完全基于下方提供的世界设定与单个%s的完整档案，为该%s在未来 %d 天内（第 1 天到第 %d 天）列出所有有决策价值的“实质行为选项”（可转化为 if-else 条件分支）。请直接高效地输出合法 JSON，无需冗长内部推演。

当前世界环境与术语表（必须严格遵循本世界设定，严禁串味或引入不属于本世界的词汇）：
%s
严格要求（违反任意一条都视为不合格）：
1. **100%% 贴合当前世界观的简体中文人话**：
   - character_summary、core_drives、key_tensions、title、when_if.conditions、when_if.trigger_event、action.summary、expected_effects、potential_collisions、follow_up_hooks **必须 100%% 使用自然、通顺且贴合本世界观的简体中文**。
   - 提及人物必须写档案中的**真实中文姓名**（如“%s”），提及地点/物品/职业/势力必须写上方对照表中的**中文名**，提及货币必须写“%s”。
   - **严禁**在这些中文可读文本中出现英文变量名、英文蛇形标识符、表达式语法（如 items.xxx >= 2、location == ...）或内部选项编号（如 opt_01、opt_12）。
   - 只有底层机器字段（actor_id、action.engine_kind、action.recipe_id、action.target_actor_ids、action.target_location、action.items_or_resources）使用档案中的真实英文/编号 ID。
2. **严禁列通用生理废话**：
   - **禁止**把“饿了进食”“渴了饮水”“困了睡觉”“累了休息”作为独立选项（这些由底层生理规则自动处理，不占选项名额）。
   - 只有真正影响资源、资产、人脉、声望、权力、安危或剧情走向的实质决策才列入。
3. **贴合该%s独一无二的处境**：
   - 充分利用该%s的性格、年龄、住所与同住者、初始物资缺口、所属势力/派系、职业晋升路径、特殊能力与标签，以及同世界其他 %d 位%s之间的真实互补、上下级、信任或敌对关系。
   - **严禁**凭空捏造档案中不存在的人物、地点或组织。
4. **动作原语与配方映射规范**：
   - 如果某条选项执行的是上方“已有生产/炼制配方”中的工序，action.engine_kind **必须填 "craft"**，并在 action.recipe_id 填入对应配方 ID（严禁把已有配方写成 new_primitive:...）。
   - 优先使用 engine_action_kinds 中已有的动作名；只有确实超出引擎现有动作集合的高阶策略动作，才可使用 "new_primitive:英文蛇形名"。
   - 每条选项的 when_if.trigger_event 都请用一句精炼中文写明触发时机或外部事件，不要留空。
5. **覆盖 %d 天内完整博弈维度**（category 只能从以下 6 个标准中文类别中选一）：
   - 主业经营（本职生产、采集、炼制、修习、设施/据点修缮升级、长期计划）
   - 资源交易（买卖议价、囤货调剂、借贷赊账、供货契约、跨区套利）
   - 人脉结盟（拜访同行/同门/邻里、交换情报、传讯联络、立誓结盟、传授或求助）
   - 竞争博弈（利益争夺、流言试探、行贿拉拢、构陷或举报、潜行窃取、袭扰复仇）
   - 突发危机（物资断供、据点损毁、伤病灾害、强敌威胁、违约追责、内部风波）
   - 进阶破局（加入新势力、职业进阶或转职、研习新技术/功法、购置或转让据点）
6. **数量与质量**：为该%s生成 **20 到 26 条**精悍、互不重复、覆盖前中后 %d 天节奏的高质量选项。每条都要明确写清“什么情况下做（if）→ 具体怎么做 → 预期后果 → 会牵动/冲突到哪位具体%s → 后续分支”。

输出格式（必须且只能返回一个合法 JSON 对象，不要附加任何解释）：
{
  "actor_id": "%s",
  "actor_name": "%s",
  "horizon_days": %d,
  "character_summary": "一两句白话概括该%s的当下处境、优势短板与未来%d天核心看点",
  "core_drives": ["核心诉求1", "核心诉求2"],
  "key_tensions": ["核心隐患或矛盾1", "核心隐患或矛盾2"],
  "options": [
    {
      "id": "opt_01",
      "category": "主业经营|资源交易|人脉结盟|竞争博弈|突发危机|进阶破局",
      "title": "简短醒目的中文标题（12字以内）",
      "day_window": [1, 10],
      "priority": "urgent|high|medium|low",
      "when_if": {
        "conditions": ["自然中文条件1（严禁英文变量或编号）", "自然中文条件2"],
        "trigger_event": "触发这件事的外部事件或时机（自然中文，必填）"
      },
      "action": {
        "engine_kind": "对应engine_action_kinds里的已有动作名，或 new_primitive:英文蛇形名",
        "recipe_id": "可选：仅当 engine_kind 为 craft 时填入档案中真实的 recipe_id",
        "target_actor_ids": [%q],
        "target_location": %q,
        "items_or_resources": {"真实item_id": 1},
        "summary": "用自然中文说明具体做什么、找谁、拿什么筹码或怎么谈"
      },
      "expected_effects": ["自然中文预期后果1", "自然中文预期后果2"],
      "potential_collisions": ["%s：可能产生的冲突、竞价、掣肘或合作点"],
      "follow_up_hooks": ["若对方答应/拒绝/翻脸后的下一步分支1", "后续分支2"]
    }
  ]
}`,
		actorNoun, actorNoun, horizon, horizon,
		worldDesc.String(),
		sampleOtherName, currency,
		actorNoun, actorNoun, len(dossier.OtherActors), actorNoun,
		horizon,
		actorNoun, horizon, actorNoun,
		sampleActorID, sampleActorName, horizon,
		actorNoun, horizon,
		sampleOtherID, sampleLocID,
		sampleOtherName,
	)
}

func formatGlossaryLine(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(parts, "，")
}

func (c Client) chatEndpoint() (string, error) {
	base := strings.TrimSpace(c.Endpoint)
	if base == "" {
		return "", errors.New("WRE_MODEL_URL (endpoint) is required")
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/chat/completions") {
		return base, nil
	}
	return base + "/chat/completions", nil
}

// ExpandActor sends exactly one request to DeepSeek for the given CharacterDossier
// and returns the full ActorExpansionRecord.
func (c Client) ExpandActor(ctx context.Context, dossier CharacterDossier) (ActorExpansionRecord, error) {
	url, err := c.chatEndpoint()
	if err != nil {
		return ActorExpansionRecord{}, err
	}
	model := strings.TrimSpace(c.Model)
	if model == "" {
		return ActorExpansionRecord{}, errors.New("WRE_MODEL (model) is required")
	}

	horizon := dossier.World.HorizonDays
	if horizon <= 0 {
		horizon = 30
	}
	dossierJSON, err := json.Marshal(dossier)
	if err != nil {
		return ActorExpansionRecord{}, fmt.Errorf("marshal dossier for %s: %w", dossier.Actor.ID, err)
	}

	maxTokens := c.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 32768
	}
	temp := c.Temperature
	if temp <= 0 {
		temp = 0.4
	}

	sysPrompt := BuildSystemPrompt(dossier)
	actorNoun := dossier.World.Environment.ActorNoun
	if actorNoun == "" {
		actorNoun = "角色"
	}
	userPrompt := fmt.Sprintf(
		"以下是角色 %s（%s，%s）的完整世界档案。请严格贴合本世界观并使用简体中文人话（严禁把吃喝睡休息列为选项，严禁在中文描述中夹杂英文变量名或内部选项编号），给出该%s在未来 %d 天内所有有决策价值的实质行为选项（JSON）：\n\n%s",
		dossier.Actor.ID, dossier.Actor.Name, actorNoun, actorNoun, horizon, string(dossierJSON),
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
		return ActorExpansionRecord{}, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Minute}
	}

	started := time.Now().UTC()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawReq))
	if err != nil {
		return ActorExpansionRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return ActorExpansionRecord{}, fmt.Errorf("request ds for %s: %w", dossier.Actor.ID, err)
	}
	defer resp.Body.Close()

	rawResp, err := io.ReadAll(resp.Body)
	if err != nil {
		return ActorExpansionRecord{}, fmt.Errorf("read ds response for %s: %w", dossier.Actor.ID, err)
	}
	durationMS := time.Since(started).Milliseconds()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := string(rawResp)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return ActorExpansionRecord{}, fmt.Errorf("ds http %d for %s: %s", resp.StatusCode, dossier.Actor.ID, snippet)
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
		return ActorExpansionRecord{}, fmt.Errorf("decode ds envelope for %s: %w", dossier.Actor.ID, err)
	}
	if len(envelope.Choices) == 0 {
		return ActorExpansionRecord{}, fmt.Errorf("ds returned empty choices for %s", dossier.Actor.ID)
	}

	content := envelope.Choices[0].Message.Content
	catalog, err := decodeOptionCatalog(content, dossier)
	if err != nil {
		tail := content
		if len(tail) > 240 {
			tail = "..." + tail[len(tail)-240:]
		}
		return ActorExpansionRecord{}, fmt.Errorf("parse ds catalog for %s (finish_reason=%s len=%d tokens=%d/%d): %w | tail=%q",
			dossier.Actor.ID, envelope.Choices[0].FinishReason, len(content),
			envelope.Usage.PromptTokens, envelope.Usage.CompletionTokens, err, tail)
	}

	return ActorExpansionRecord{
		WorldID:          dossier.World.ID,
		ActorID:          dossier.Actor.ID,
		ActorName:        dossier.Actor.Name,
		Model:            model,
		Endpoint:         url,
		RequestedAt:      started,
		DurationMS:       durationMS,
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		TotalTokens:      envelope.Usage.TotalTokens,
		Dossier:          dossier,
		Catalog:          catalog,
		RawResponse:      content,
	}, nil
}

func decodeOptionCatalog(raw string, dossier CharacterDossier) (ActorOptionCatalog, error) {
	cleaned := extractJSONPayload(raw)
	if cleaned == "" {
		return ActorOptionCatalog{}, errors.New("empty json payload")
	}
	var cat ActorOptionCatalog
	if err := json.Unmarshal([]byte(cleaned), &cat); err != nil {
		return ActorOptionCatalog{}, err
	}
	if len(cat.Options) == 0 {
		return ActorOptionCatalog{}, errors.New("returned options list is empty")
	}
	if cat.ActorID == "" {
		cat.ActorID = dossier.Actor.ID
	}
	if cat.ActorName == "" {
		cat.ActorName = dossier.Actor.Name
	}
	if cat.HorizonDays <= 0 {
		cat.HorizonDays = dossier.World.HorizonDays
	}
	return cat, nil
}

func extractJSONPayload(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start >= 0 && end > start {
		return strings.TrimSpace(s[start : end+1])
	}
	return s
}

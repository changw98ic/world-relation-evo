# WRE 架构重构迁移矩阵与实施契约（`migration-matrix.md`）

> 基线：`dev@58a14c9`（分支 `refactor/architecture-p0-p7`）。
> 原则：以 **8 阶段（P0–P7）+ 4 验收关口（G1–G4）** 推进；**P1–P3 为第一轮核心重构**，先建立可扩展的统一行为与执行内核，再按依赖顺序完成认知、LOD、编译/叙事与清理。

---

## 一、核心最小接口契约（结合现有 Go 类型精确落定）

在保持 `sim.World`、`sim.Action`、`sim.Perception`、`sim.Event` 外部 JSON 兼容的同时，建立 4 个核心接口，消灭规则、候选、决策、执行四者混杂：

```go
// 1. 统一动作定义与执行内核（P1–P2，internal/sim）
type ActionContext struct {
    World   *World
    ActorID string
    Intent  ActionIntent // 兼容包裹 sim.Action + 元数据（CandidateID, RuleIDs, BasisEventID）
}

type ActionCheckResult struct {
    Allowed     bool
    BlockReason string
}

type ExecutionResult struct {
    Succeeded bool
    Reason    string
    Events    []Event
    Mutations []StateMutationRecord // 可追溯的原语变更摘要（用于守恒/审计/因果）
}

type ActionHandler interface {
    Kind() string
    Check(w *World, actorID string, intent ActionIntent) ActionCheckResult
    CheckPerceived(view Perception, intent ActionIntent) ActionCheckResult // 候选期单源无副作用校验
    Execute(tx *WorldTx, actorID string, intent ActionIntent) (ExecutionResult, error)
}

type ActionExecutor interface {
    Check(world *World, intent ActionIntent) ActionCheckResult
    Execute(world *World, intent ActionIntent) (ExecutionResult, error)
}

// 2. 候选构造与规则编译（P1 & P3，internal/sim + internal/agent + internal/optiongen）
type Candidate struct {
    ID           string
    Label        string
    Intent       ActionIntent
    Source       string   // "builtin" | "action_spec" | "rulebook" | "playbook"
    RuleIDs      []string // 命中的声明式规则 ID
    RuleReason   string
    BasisEventID int64
    PriorityHint float64
}

type CandidateBuilder interface {
    Build(view Perception, rules RuleSet) ([]Candidate, error)
}

type RuleCompiler interface {
    Compile(source RuleSource, catalog ActionCatalog) (RuleSet, error)
}

// 3. 纯决策器（P3，internal/agent：Jev / RuleSelector / RulebookDecider / PlaybookDecider 共享）
type DecisionSelector interface {
    Select(ctx context.Context, view Perception, candidates []Candidate) (ActionIntent, error)
}
```

### 四个契约铁律
1. **单源条件**：`ActionHandler.CheckPerceived`（仅基于 `Perception` 已知信息）与 `ActionHandler.Check`（基于真实 `World`）定义在同一注册单元内；候选生成与规则决策器禁止私自重抄精力/血量/权限/外交判断。
2. **原子事务边界 `WorldTx`**：所有状态变更经 `StatePrimitives`（`TransferCurrency`/`MintBurnCurrency`/`TransferItem`/`ProduceConsumeItem`/`MutateVitals`/`MoveActor`/`MutateTrust`/`RecordKnowledge`/`EmitEvent`）记录并提交；先全量校验、再批量变更，失败动作零部分结算。
3. **编译期严格校验**：`RuleCompiler` 面对未知原语（含 `new_primitive:*`）、未注册 `Action.Kind`、非法参数或无效条件，必须在编译期报错返回，禁止运行时静默降级。
4. **纯选择决策**：`Jev`、`RuleSelector`、`RulebookDecider`、`PlaybookDecider` 只接收相同 `([]Candidate)` 返回一个 `ActionIntent`，不直接修改世界，也不通过修改 `Option.Label` 文本来暗渡规则。

---

## 二、全仓模块迁移归属矩阵（确保零遗漏）

| 现有模块 / 文件 | 当前职责 | 目标模块归属 | 迁移阶段 | 兼容策略 |
|---|---|---|---|---|
| `sim/world.go` (`Action`, `apply`, `Apply`) + 6 个子分发器 | 89 个内置动作 + 声明式动作分散分发 | `sim` 统一 `ActionRegistry` + `ActionExecutor` + 领域 `ActionHandler` 注册 | **P1–P2** | `World.Apply(id, x)` 保留为 `ActionExecutor.Execute` 的兼容薄壳；旧导出 `ApplyContinuity`/`ApplyMarketCreditAction`/`ApplyRelationAction`/`ApplyJusticeAction` 改为委托 `World.Apply`（补齐守卫与秘密捕获） |
| `sim/world.go` (`World.Step`, `Advance`, `record`, `markDeadIfZero`) | 31 阶段编排、状态直接修改、事件生成 | `sim` `WorldRuntime`（显式 `TickPhase` 管线）+ `WorldTx` + `StatePrimitives` + `EventCommit` | **P2** | 保持 31 阶段既有逻辑顺序与确定性；修复 `finishDeclarativeAction` 漏死、`claim_turf`/`mediate_feud` 部分结算与 7 处 nil `Trust` panic |
| `sim/states_ledgers.go` (`adjust*`, `VerifyLedgerConservation`) + `governance.go` (`creditTreasury`/`debitTreasury`) | 分散的货币/物品/财政记账 | `sim` `StatePrimitives` 唯一写入层（统一 `Actor`/`Cohort`/`Polity`/`PublicTreasury` 账本） | **P2** | 全仓直接写 `Money`/`Items`/`Health`/`Energy`/`Location`/`Trust`/`Memories` 全部收口至原语 |
| `sim/world.go` (`View`, `applyPerceptionEvoHintsWithAgreements`) + `sim/scale.go` (`viewIndexed`) | 双路径感知 + 15 个动作候选字段混杂 | `sim` 单路径 `KnowledgeProjection.BuildPerception(actorIdx, lookup)`；剥离 `(d)` 候选构造至 `CandidateBuilder` | **P1 (合并双路径) → P3 (剥离候选) → P4 (认知封堵)** | `World.View(id)` 保持签名不变；旧 `Perception` 动作字段在 P3 过渡期可按需由统一构造器填充并在 P7 清理 |
| `sim/effects_combat.go` (`ActionSpec`, `applyDeclarativeAction`, `finishDeclarativeAction`) | 声明式动作执行与战斗 | 接入统一 `ActionRegistry`（动态注册 `Catalog.ActionSpecs`）+ 复用 `StatePrimitives` | **P1–P2** | 修复 `Perception.ActionSpecBlockReason` 漏 `isVenuePowerBlocked`/`TransitionID`、`finishDeclarativeAction` 漏 `markDeadIfZero` 与 `captureActionSecrets` |
| `agent/pipeline.go`, `*_agent.go` (`Options`) | 候选动作生成 + 40+ 重复前置条件 | `agent` 统一 `CandidateBuilder`（消费 `sim.ActionRegistry` + `RuleSet`） | **P1 & P3** | `agent.Options(p)` 保留为 `CandidateBuilder.Build` 适配层，消灭重复 `practice_skill` 与 22 处条件漂移 |
| `agent/rulebook.go`, `rulebook_jev.go`, `base_rulebook.go`, `playbook.go`, `rules.go`, `jev.go` | 规则匹配、文本标签增强与决策选择混杂 | 拆分为 `RuleMatcher` + `CandidateBuilder` + 纯 `DecisionSelector`（`Jev`/`Rules`/`Rulebook`/`Playbook`） | **P3** | 废弃 `EnrichOptionsForJev` 文本拼接，改为结构化 `Candidate`；保留 `sim.Decider` 适配层 |
| `optiongen/profile.go`, `rulebook.go`, `expand.go`, `deepseek.go`, `responses.go` | 离线展开与审计（含 2 份分叉动作白名单） | `RuleCompiler`（直接从 `sim.ActionCatalog` 读取动作能力与 `ActionSpecs`，编译期拒收未知原语） | **P3 & P6** | 删除幽灵 `"shoot"` 与硬编码白名单分叉 |
| `sim/memory.go`, `diplomacy.go`, `party.go`, `ambitions.go`, `spotlight_rumor.go`, `world_events.go`, `geography_polity.go`, `knowledge_laws.go` | 事实、观察、声明、传闻、秘密与记忆混写 | `KnowledgeProjection` + `Observation`（统一 `CanObserveActor`）+ `Communication`（严格 `FactEventID` 验证） | **P4** | 封堵全部 10 类上帝视角泄漏、谣言改客观真相、远程感知 `&&` bug 与 13 处因果断链 |
| `sim/scale.go`, `governance.go`, `ecology.go`, `lifecycle.go`, `population_members.go`, `resident_bands.go`, `cohort_food.go` | 三层 LOD 独立规则、古代硬编码、守恒漏洞 | `PopulationRuntime` + `LODProjection` + `CohortSettlement`（共享 `SettlementLaws`） | **P5** | 修复 6 项 P0 守恒 bug、5 项跨 LOD 超凡力量失效、`lifecycle.go:732` 随机 map 顺序，去除古代硬编码 |
| `internal/pack/blueprint.go`, `pack.go`, `production_validation.go` + `sim` 各 `Effective*Spec` | 零值覆盖、双层默认回填、题材硬编码 | `WorldPackCompiler`（带 `schema_version` 与旧包迁移适配器，显式 `0`/`false`/`[]` 零值保真） | **P6** | 兼容 `packs/mortal_watershed.json`、`xianxia_sect.json`、`wasteland_sector.json` 及历史 Blueprint |
| `internal/runtrace/chronicle.go`, `query.go`, `analyze.go`, `record.go` + `cmd/wre/chronicle.go`, `novel_cli.go` | 直接读 `*sim.World`、谣言当事实、`"witnesses"` 类型冲突 | `ChronicleProjection`（只消费 `[]sim.Event` + 不可变 `WorldMetadataSnapshot`，区分事实/声明/谣言） | **P6** | 保持 `wre chronicle` / `wre query` CLI 参数与输出格式兼容，修复客观/传闻混写 |
| `internal/store/store.go`, `schema.sql` + `cmd/wre/main.go` | 无迁移、非单事务、inspect 绕过解码 | 统一 `DecodeWorldSnapshot`、单事务 `SaveTick`、增量 schema 迁移与外键 | **P6–P7** | 向后兼容读取旧 PG 快照与事件行 |

---

## 三、八阶段与四个验收关口执行计划

### 关口 G1（P0–P1）：基线冻结 + 统一行为契约
- **P0（已完成）**：锁定 `dev@58a14c9`，切出 `refactor/architecture-p0-p7`，5 路并行审计完成并交付 4 份基线文档（`architecture-current.md`, `dependency-map.md`, `state-mutation-map.md`, `migration-matrix.md`）。
- **P1（下一步立即执行）**：
  1. 合并 `World.View()` 与 `World.viewIndexed()` 为单一 `buildPerception`，修复 `View(unknownID)` nil panic 与 `VenueRoutes`/`Nearby` 漂移，补充 100% parity 单测。
  2. 新增 `internal/sim/action_contract.go` + `internal/sim/action_registry.go`：定义 `ActionIntent`、`ActionCheckResult`、`ActionHandler`、`ActionRegistry`（注册全部 89 个内置动作 + 动态 `Catalog.ActionSpecs`）。
  3. 建立单源条件求值器（`CheckAction` / `CheckPerceivedAction`），优先覆盖代表性高频动作与已知高危漂移动作：`join_faction`、`offer`/`accept`/`reject`、`attack`/`avenge_feud`/`spar`/`steal`/`raid`/`extort`/`bribe`、`move`/`move_venue`/`enter_house`/`leave_house`、`heal`/`medicate`/`eat`/`drink`/`sleep`/`relieve`、`claim_turf`、`frame_suspect`/`file_appeal`、`form_party`/`join_party`/`betray_party`、`propose_alliance`/`declare_war`/`make_peace`、`ActionSpec`（补齐 `isVenuePowerBlocked` 与 `TransitionID`）、`issue_decree`/`transmit_message`/`teach_knowledge`/`adopt_facility`/`rewrite_world_law`。
  4. 让 `agent/pipeline.go` 与 `*_agent.go` 调用统一单源条件检查，消除 22 处已知 `agent`/`sim` 条件漂移与 `practice_skill` 重复生成。
  - **G1 验收**：全仓 `go test -race ./... && go vet ./...` 通过；新增契约单测验证代表性动作在 `agent` 候选与 `sim` 执行使用同一条件源。

### 关口 G2（P2–P3）：世界执行内核 + 统一规则与决策
- **P2（世界执行内核）**：
  1. 新增 `internal/sim/state_primitives.go` + `internal/sim/world_tx.go`：实现 6 大原子原语（货币、物品、生命精力、位移、关系信任、记忆知识）与 `WorldTx` / `ExecutionResult` / `EventCommit`。
  2. 将 `world.go` 与全部子系统直接状态写入逐步迁入原语；收口 6 个子分发器统一走 `ActionExecutor`，统一补齐 `captureActionSecrets`（含 `Advance` 多 Tick 完成）；修复 `finishDeclarativeAction` 漏 `markDeadIfZero`、`claim_turf`/`mediate_feud` 部分结算与 7 处 nil `Trust` panic。
  3. 将 `World.Step` 的 31 个阶段显式化为 `TickPhase` 管线。
- **P3（统一规则与决策）**：
  1. 抽离统一 `ActionCatalog`（唯一动作元数据来源，替换 4 处硬编码动作清单）。
  2. 拆分 `Rulebook` / `Playbook` / `Catalog.ActionSpecs` 进入统一 `RuleCompiler` → `RuleMatcher` → `CandidateBuilder` 链，结构化输出 `[]Candidate`（含 `RuleIDs`/`BasisEventID`）。
  3. 重构 `Jev`、`RuleSelector`、`RulebookSelector`、`PlaybookSelector` 实现统一 `DecisionSelector`，废弃 `EnrichOptionsForJev` 文本标签耦合；修复 `optiongen` 编译期校验（拒收未知原语，接入 `ActionSpecs`）。
  - **G2 验收（证明第一轮核心重构三大里程碑）**：
    1. 新增一个基于已有原语的声明式复合互动，**无需修改 `World.Apply()` 主分发逻辑**；
    2. `Jev` 与本地规则决策器在同一世界状态下看到完全一致的合法 `[]Candidate`，行为条件与结果语义一致；
    3. 失败动作零部分结算，成功动作的状态变化可追溯至动作 → 原语 → 事件因果链。

### 关口 G3（P4–P5）：人物认知隔离 + 统一群体与 LOD 结算
- **P4（认知与传播）**：落实 `CanObserveActor` 统一观察原语；封堵 `FactionJoinEligibilityReason`/`Nearby.FactionRelation`/`Neighbor.Tags`/`perceivedParties`/私有资产透视/跨区直写等 10 类上帝视角；硬化 `ValidateCitation`/`citedReport`/谣言系统（谣言不得直接篡改客观世界状态）；修复远程感知 `&&` bug 与 13 处因果断链。
- **P5（群体与 LOD）**：修复 6 项 P0 人口/物品/牢狱/饥饿守恒漏洞；抽离个体与群体共享的 `SettlementLaws`；修复 `IsActorPinnedFocal`、`cohort_struck` 护盾与阶位免疫、`venue` AoE 与 `WorldEventSpec` 跨 LOD 作用、不朽形态寿命及 `lifecycle.go:732` 随机 map 顺序。
- **G3 验收**：认知隔离测试、人口/货币/物品守恒测试、跨 LOD 超凡作用测试、确定性回放全部通过。

### 关口 G4（P6–P7）：内容编译/叙事隔离 + 清理固化与全量验收
- **P6（世界包、编译与叙事）**：引入 `schema_version` 与旧包兼容适配器；补全 `Blueprint`/`WorldPack`/`sim` 零值保真指针；清除古代官府与 `"food"`/`"carpenter"` 等硬编码题材泄露；`BuildChronicle` 改为只消费事件证据 + 不可变元数据快照，严格区分客观事实与未证实谣言/指控；修复 `"witnesses"` 字段类型冲突；统一 `DecodeWorldSnapshot` 与单事务 `SaveTick`。
- **P7（清理与验收）**：删除被替代的重复代码与死分支；更新文档与 CLI；执行 10 项最终验收（普通测试、`go vet`、`-race`、确定性回放、守恒、认知、LOD、旧包兼容、多题材长程模拟、Jev 集成/未配置凭据显式标明）并输出最终回归与差异报告。

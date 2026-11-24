# WRE 现状架构全景审计（`architecture-current.md`）

> **基线锁定**：`dev` / `origin/dev` HEAD = `58a14c91a623569b33e849693e2301215c182eda`
> **重构分支**：`refactor/architecture-p0-p7`
> **基线验证**：`go vet ./...` 通过；`go test -race ./...` 全仓 7 个包全绿。

---

## 1. 系统总体架构与五大结构性病灶

当前 WRE 全仓共 **7 个 Go 包、191 个 `.go` 文件、74,964 LOC**（生产代码 59,524 LOC + 测试 15,440 LOC）。顶层包级依赖图已无循环依赖，但内部存在五大结构性耦合：

1. **世界状态、规则、决策、执行职责混杂**
   - `internal/sim/world.go`（3,916 LOC）同时承载世界状态定义、31 阶段主循环编排（`Step`）、多 Tick 结算（`Advance`）、89 个内置动作 + 声明式动作分发（`apply`）、感知组装（`View` + `applyPerceptionEvoHintsWithAgreements`）与部分业务规则。
   - `internal/agent`（9,649 生产 LOC）把候选动作构造（`Options()` + 4 个子模块）、规则匹配（`InteractionRulebook` / `Playbook`）与决策选择（`RuleSelector` / `RulebookSelector` / `PlaybookSelector` / `Jev`）缠绕在一起。
2. **相同业务条件在不同模块重复实现且已出现严重漂移**
   - `w.apply()` 支持 89 个内置 `Action.Kind` + 声明式 `Catalog.ActionSpecs`；`agent.Options()` 生成 74 个 `Action.Kind`。
   - 全仓仅约 15 处前置校验通过 `sim` 单源函数共享，其余 **40+ 个动作**在 `agent/pipeline.go`、`agent/rules.go`、`agent/*_agent.go` 与 `sim` 之间双写甚至三写，已验证出至少 **22 处真实语义漂移**（如 `frame_suspect` 在 `agent` 检查 `DistrictGrievance>=35` 而 `sim` 检查 `Corruption>=24 || bribeShielded`；`join_party` 在 `agent` 查自己对队长信任而 `sim` 查队长对自己信任；`claim_turf` 在 `agent` 漏查 `InsideHouse==""`；`eat` 漏查 `ActorAcceptsFood`；`ActionSpecBlockReason` 在 `Perception` 版漏查 `isVenuePowerBlocked` 与 `TransitionID`）。
3. **行为扩展依赖修改 Go 代码，声明式规则能力与编译校验不足**
   - 存在 **4 份互相分叉的硬编码动作清单**：`sim/catalog.go:276-290` (`TagActions`)、`optiongen/profile.go:11-76` (`engineActionKinds`，含幽灵动作 `"shoot"`)、`optiongen/rulebook.go:170-210` (`baseInteractiveEngineActions`，漏掉 25+ 内置动作与全部 `Catalog.ActionSpecs` 导致 `SanitizeAndAuditRulebook` 静默丢弃声明式动作规则)、`agent/playbook.go:1305-1315` (`isTargetedActionKind`)。
   - `optiongen` 对 `new_primitive:*` 未在编译期报错，而是由 `playbook.go:1360-1423` 在运行时静默降级为 `speak`/`offer`/`craft`/`transmit_message`。
4. **Actor、Stored Member、PopulationCohort 与世界事件缺少统一结算边界**
   - 实际存在 **三层 LOD**：活跃个体 `w.Actors`、已脱水但保留个体的 `c.Members`、匿名聚合人群 `PopulationCohort`（含 `ResidentBand`）。
   - 宏观人群有 10+ 个独立 `stepCohort*` 结算函数，硬编码古代官府税制/火耗/保护费/15 个古代职业 ID，且与个体规则分叉；高阶力量（`ActiveEffect` 阵法护盾、`FormSpec.PowerTier`、场域 AoE、`WorldEventSpec` 初始冲击、`StateSlots`、`GoldenFinger`）在跨 LOD 时存在显著失效或被抹除。
5. **模拟、知识传播、世界包与编年史相互耦合**
   - `World.View()` 与 `World.viewIndexed()` 双路径维护（存在 14 处漂移及 `View(unknownID)` nil panic）；`sim.Perception` 60 个字段中超过 **30 个字段**存在上帝视角泄漏或动作候选逻辑混入。
   - 谣言系统（`spotlight_rumor.go:300-453`）允许未核验/伪造言论直接扣减客观 `DistrictState.MoraleBonus`；`BuildChronicle` 直接读取可变 `*sim.World`（含伪装前真实阶位、私密野心与未揭露秘密）并把未证实谣言渲染为客观事件弧摘要。

---

## 2. 主循环 `World.Step()` 31 阶段真实执行序（`internal/sim/world.go:1477-1648`）

> **关键发现（时钟撕裂）**：Pass 1–6 运行在 `w.Tick == T`（自增前），Pass 7–31 运行在 `w.Tick == T+1`（自增后）。`StepMarketAndCredit` 与 `stepCohorts` 看到的时间戳比环境、生命周期、关系、司法早 1 tick。

| 阶段 | 源码位置 | 职责 | 关键问题 / 观察 |
|---|---|---|---|
| Pass 0 | `world.go:1478-1514` | 并发决策（`buildStepIndex` + 4..32 worker 调用 `viewIndexed` & `d.Decide`） | 只读阶段，出错即中止；但 `viewIndexed` 短暂别名共享活体 `a.Memories` |
| Pass 1 | `world.go:1516` | `stepFlowGrid()`（供能、边流、场馆供电） | — |
| Pass 2 | `world.go:1517-1526` | 推进存储队列成员 `c.Members` 进行中 `Task` + `reconcileCohortMemberChanges` | `Advance` 不触发 `captureActionSecrets` |
| Pass 3 | `world.go:1527-1547` | 活跃个体：在押回血回精力 / `Advance` 忙碌任务 / 执行非位移 `Apply` | 在押恢复直接改 `Energy`/`Health` |
| Pass 4 | `world.go:1548-1552` | 活跃个体：执行位移类 `Apply`（`move`/`move_venue`/`leave_house`/`enter_house`） | 与非位移解耦以减少同 tick 错位 |
| Pass 5 | `world.go:1553` | `StepMarketAndCredit`（屯粮/价格/租金/借贷/兼并） | 跳过 `c.Members`；死后释放囤货可能滞留尸体 |
| Pass 6 | `world.go:1554-1556` | `stepCohorts()`（餐食/生产/生理/市场/税收/保护费/冲突/贿赂/反腐/康复/晋升） | 大量硬编码古代职业/税制；未统一 `detachCohortMembers` |
| **Tick++** | `world.go:1558` | **`w.Tick++`** | 时钟边界 |
| Pass 7–10 | `world.go:1559-1562` | `settleShipments` → `AdvanceGrowth` → `stepActiveEffects` → `AdvanceContinuity` | `stepActiveEffects` 危害伤害跳过 `c.Members` 和队列 |
| Pass 11 | `world.go:1565-1567` | 活跃个体饥饿推进 `advanceActorHunger` | `c.Members` 饥饿推迟到 Pass 16 |
| Pass 12 | `world.go:1569` | `stepWorldEnvironment`（天气/水文/资源/灾害/威胁/巡逻/祭祀/遭遇事故） | 灾害暴露漏传 `cause_event_id`；`stepCohortRituals` 忽略 `RitualsEnabled` |
| Pass 13–15 | `world.go:1570-1572` | `stepWorldEvents` → `stepAdoptedFacilities` → `stepStateSlotsAndForms` | `WorldEventSpec` 冲击跳过队列与 `c.Members`；队列无 `StateSlots` |
| Pass 16 | `world.go:1573` | `stepActorPhysiologyAndLifecycle`（个体+成员伤病死亡/生育/继承补员 + 20:00 俸禄） | 活跃个体生育未调 `c.issueResidents` 破坏 `ResidentBand` 守恒；20:00 俸禄漏扣 `Members` |
| Pass 17–19 | `world.go:1575-1597` | 活跃囚犯释放 → 房屋风化 → `Offers` 过期 | `c.Members` 释放未调 `settleReleasedCases` |
| Pass 20–26 | `world.go:1599-1605` | `StepRelationsAndTurf` → `StepJusticeAndRevolt` → `ObserveCausalRelationEvents` → `stepParties` → `stepDiplomacy` → `stepAmbitionsAndSecrets` → `stepGoldenFingers` | 多处直接改 `c.Jailed`/`Trust`/`Grudges` 且断因果链 |
| Pass 27–31 | `world.go:1607-1646` | `refreshActor`（仅活跃）→ `reconcileActorWoundStatus` → `drainCohortMortalityEvents` → `ObserveSpotlightAndRumors` → `StepDynamicSpotlight` | `IsActorPinnedFocal` 漏 `FormSpec.PowerTier`/`GoldenFinger`/`SustainActorID` |

---

## 3. 动作执行与六大子分发器现状

`World.Apply(id, x)` (`world.go:2387`) 包装 `w.apply(id, x)` 并追加 `captureActionSecrets`。但 `w.apply` 内部混合了内联实现、单函数委托以及 **6 个协议不一致的子分发器**：

| 子分发器 | 位置 | 导出 | 签名形状 | 核心问题 |
|---|---|---|---|---|
| `applyGrowthCareAction` | `growth_care_actions.go:70` | 否 | `(a *Actor, x Action) []Event` | 仅靠 `w.apply` 守卫 |
| `ApplyContinuity` | `continuity.go:534` | **是** | `(a *Actor, x Action) []Event` | 外部直调绕过能力/世界律/引用/秘密捕获 |
| `ApplyMarketCreditAction` | `market_credit.go:808` | **是** | `(mc, actorID, x) ([]Event, bool)` | 重复 `w.actor` 与部分守卫；绕过能力/世界律/秘密捕获 |
| `ApplyRelationAction` | `relations_kinship.go:1457` | **是** | `(rs, actorID, x) ([]Event, bool)` | 同上；`claim_turf`/`mediate_feud` 存在先改状态后失败 |
| `ApplyJusticeAction` | `justice_revolt.go:359` | **是** | `(jr, actorID, x) ([]Event, bool)` | 仅含 `frame_suspect`/`file_appeal`，与 `open_case`/`hold_trial`/`arrest` 割裂 |
| `applyDeclarativeAction` | `effects_combat.go:972` | 否 | `(a, x, spec) []Event` | `finishDeclarativeAction` 在 `Advance` 中绕过 `captureActionSecrets` 且负 `HealthGain` 漏调 `markDeadIfZero` |

---

## 4. `Perception`（60 字段）与认知/传播链现状

- **双路径冗余**：`World.View(id)` (`world.go:922`) 对活跃角色转发到 `viewIndexed`，但保留了 140 行独立回退实现供脱水成员使用；二者在 `MaxNearbyView`（32 上限 + 显著性加权）、`VenueRoutes` 条件、切片克隆和 `View(unknownID)` panic 上存在明显分叉。
- **60 字段分类统计**（详见 `state-mutation-map.md`）：
  - 纯局部/自省字段 `(a)`：约 12 个（`Tick`/`Clock`/`Calendar`/`CurrentVenue`/`CurrentHouse`/`LedFactions`/`ActiveEffects`/`HasActiveLoan` 等）。
  - 认知门控但存残余漏洞 `(b)`：约 8 个（`ActiveWorldEvents`/`FactionDiplomacy`/`KnownSecrets`/`KnownTechs`/`ActiveGrudgeTargets` 等）。
  - **客观全局真相泄漏 `(c)`**：超过 **28 个字段**（典型：`Factions[i].JoinBlockReason` 与 `Nearby[i].FactionRelation` 绕过 `actorKnowsDiplomacyUpdate`；`Nearby[i].Tags` 泄露伪装前真实 `FormID`；`DistrictState` 暴露 `BribeShieldTicks`/`Corruption`/`Patrols`；`ExtortionTargets`/`BlackmailActions`/`LoanQuotes`/`OfferBlockReasons`/`FulfillableAgreementIDs` 读取对方私有钱包、仇怨和未伪装战力；`perceivedParties` 缺 `SamePlace`/清醒校验；`FlowEdges`/`Map`/`AccessibleHouses`/`Growth`/`Roads`/`VenueRoutes` 泄漏跨区实时态）。
  - **动作候选逻辑混入 `(d)`**：15 个字段（`ActivePlan`, `CaseActions`, `BlackmailActions`, `MediationActions`, `GrowthCareActions`, `ExtortionTargets`, `LeadershipChallenges`, `EligibleSpouseIDs`, `EligibleBondIDs`, `OfferBlockReasons`, `ProductionBlockReasons`, `ExpectedCraftYield`, `Interactions`, `PartyReady`, `AppealTargetID`/`AppealBasisEventID`）。
- **传播与谣言链缺陷**：
  - `ValidateCitation` 仅校验 `Topic == "item_produced"`；`citedReport` 漏验 `o.Quantity == s.Quantity` 且未核验转述也会盖章 `WitnessID`。
  - `processRumorSpeech` 把 `witness_id` 混淆为发言者身份，且未验证 `FactEventID > 0` 即直接扣减 `DistrictState.MoraleBonus -= 4`。
  - 远程感知（`world_events.go:408`, `geography_polity.go:965`, `knowledge_laws.go:1024`）室内穿透判断把 `||` 误写为 `&&`。

---

## 5. 世界包编译、叙事与持久化现状

- **`internal/pack` 零值保真缺口**：`Blueprint` 有 12+ 个非指针数值/切片字段在显式传 `0`/`[]` 时被覆盖；`sim` 的 5 大配置结构体（`GovernanceSpec` 10 字段、`MarketCreditSpec` 12 字段、`RelationEvoSpec` 9 字段、`JusticeRevoltSpec` 12 字段、`SpotlightRumorSpec` 9 字段）及 `LifecycleSpec`/`EcologySpec.Enabled`/`DistrictState` 覆盖逻辑均存在 `<= 0` 默认回填，无法配置 `0%` 税率/利息/无寿命限制/无生态。
- **硬编码题材残留**：`blueprint.go`、`environment.go`、`governance.go`、`scale.go`、`production.go`、`house.go:205`（`a.Items["food"]` 导致非 `"food"` 口粮题材永久扣睡眠恢复）、`world.go`（`"carpenter"`/`"peddler"`）仍含大量古代县域/中文默认语义。
- **`internal/runtrace` 叙事隔离缺口**：`BuildChronicle` 直接持有可读可变 `*sim.World`（绕过伪装读取 `ActorPowerTier`、读私密野心与未曝光秘密）；`BuildChronicleArcs` 直接把 `rumor_panic_triggered` 对应的未核验发言文本输出为客观事件弧摘要；`eventInvolvesActor` 与 `secret_exposed` 的 `"witnesses"` 字段存在类型冲突（`[]string` vs `int`）。
- **`internal/store` 持久化风险**：`worlds` / `world_checkpoints` / `world_decision_logs` 无 `schema_version` 与表迁移机制、缺外键、`Save` 与 `SaveDecisionLog`/`SaveCheckpoint` 非单事务、`wre inspect -state-file` 绕过 `DecodeWorldSnapshot`。

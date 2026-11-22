# WRE 全仓状态写入路径、原子性、认知泄漏与守恒缺陷登记册（`state-mutation-map.md`）

> 基线：`dev@58a14c9`。本文件登记全部已核实的世界状态直接写入点、部分结算漏洞、感知上帝视角泄漏、人口/资源守恒缺陷及零值覆盖点，作为 **P1–P6** 的逐项销号清单。

---

## 一、核心状态写入点总登记册（`internal/sim/` 全量 54 个非测试文件）

| 状态域 | 现有规范辅助函数 | 绕过辅助函数的直接写入分布 | 已知钩子遗漏 / 缺陷 |
|---|---|---|---|
| **`Actor.Money` / `Currencies`** | `adjustActorCurrency` (`states_ledgers.go:1265`), `addActorCurrency` (`:1244`) | **15 个文件、56 处直接写入**：`world.go` (3278, 3381, 3455, 3518, 3806), `scale.go` (1115, 1635, 2035, 2107, 2156), `lifecycle.go` (1285), `continuity.go` (650), `personal_credit.go` (111), `market_credit.go` (894, 1093, 1159), `relations_kinship.go` (1014, 1110, 1443, 1530, 1611, 1684, 1950), `justice_revolt.go` (560, 990, 1969), `geography_polity.go` (1542), `ambitions.go` (471), `diplomacy.go` (401), `party.go` (353, 514), `population_members.go` (301), `golden_finger.go` (545, 736, 791, 1144) | `golden_finger.go` 手动配对 `recordLedgerCurrency*`；`replay_inspect.go:748-752` 漏计 `c.Members` 硬币导致报告失真；尸体遗留货币被 `TotalCoins` 静默丢弃 |
| **`Actor.Health`** | 无统一扣血/治疗原语（仅死亡入口 `markDeadIfZero` at `world.go:1875`） | **13 个文件、52 处直接写入**：`world.go` (1536, 2014, 2253, 2826, 3177, 3217, 3321, 3364, 3416, 3478, 3589, 3638, 3679), `effects_combat.go` (605, 826, **1037-1039**), `ecology.go` (1630, 1731, 1798, 1883), `ecology_process.go` (375, 381), `lifecycle.go` (433, 443, 460, 481, 578), `states_ledgers.go` (451, 588, 608), `relations_kinship.go` (1672, 1677), `party.go` (349), `golden_finger.go` (641, 1168, 1184), `scale.go` (719, 1110) | **P0 致命漏洞**：`effects_combat.go:1037-1039` (`finishDeclarativeAction`) 负 `HealthGain` 可把 `a.Health` 降到 `0` 而**不调用 `markDeadIfZero`**，制造 `Health==0 && !Dead` 活死人 |
| **`Actor.Energy`** | 无统一原语 | **18 个文件、60+ 处直接写入**：`world.go`, `scale.go`, `lifecycle.go`, `knowledge_laws.go`, `effects_combat.go`, `states_ledgers.go`, `geography_polity.go`, `market_credit.go`, `relations_kinship.go`, `justice_revolt.go`, `ambitions.go`, `diplomacy.go`, `party.go`, `world_events.go`, `golden_finger.go`, `population_members.go`, `growth_work.go` | `relations_kinship.go:1839` (`claim_turf`) 在失败校验前先扣 5 精力 |
| **`Actor.Location` / `VenueID` / `InsideHouse` / `HomeID`** | 无统一移动原语 | **7 个文件、32 处直接写入**：`world.go` (2107, 2549, 2563, 3137, 3591), `scale.go` (715, 1106), `population_members.go` (386), `geography_polity.go` (715), `relations_kinship.go` (1034, 1546), `market_credit.go` (988, 1207) | `transfer_house` (`world.go:3132`) 未清理原房主 `HomeID` 与 `InsideHouse`；`arrest` (`3591`) 清 `InsideHouse` 但不移入监牢 |
| **`Actor.Items` / `Equipped`** | `adjustActorItem` (`states_ledgers.go:1526`), `removeActorEquippedItem` (`:1607`) | **12 个文件、26 处直接写入**：`scale.go` (1117, 1631, 2036, 2103, 2160), `continuity.go` (**648**), `market_credit.go` (647, 698, 724), `relations_kinship.go` (885, 1042, 1115), `population_members.go` (309, 343), `effects_combat.go` (1371, 1406), `golden_finger.go` (581, **746**) | `continuity.go:648` 与 `golden_finger.go:746` 未删除 `0` 数量键，污染 `len(b.Items) > 0` 判断；`HydrateActor` 与群体死亡直接丢弃 `ResidentBand.Items` 破坏物品守恒 |
| **`Actor.Trust`** | 无统一关系原语 | **10 个文件、42 处直接写入**：`world.go`, `scale.go`, `knowledge_laws.go`, `continuity.go`, `relations_kinship.go`, `ambitions.go`, `party.go`, `offense.go`, `market_credit.go`, `justice_revolt.go` | **7 处 nil-map panic 隐患**（`world.go:2316,3140,3227,3522,3694`, `knowledge_laws.go:397,532`） |
| **`Actor.Memories`** | `w.record` → `appendMemory` (`world.go:1650`, `memory.go:86`) | **10 个文件直接拼接**：`world.go:1987`, `knowledge_laws.go:480,497,1056`, `geography_polity.go:1345,1365,1464`, `ambitions.go:428,719,795`, `diplomacy.go:177,760`, `justice_revolt.go:883,1940`, `party.go:555`, `world_events.go:320,428`, `relations_kinship.go:1072,1202`, `golden_finger.go:1110` | 多处缺失 `search` 索引缓存、`SourceRole`/`SourceTags`/`CauseEventID`/`FactEventID`；`FailureWipeMemories` (`states_ledgers.go:669`) 未同步清除 `Secrets.KnownBy`/`Knowledge.KnownActors` |
| **`World.Factions` / `Houses`** | 无统一成员/产权原语 | `world.go`, `scale.go`, `house.go`, `ecology.go`, `knowledge_laws.go`, `market_credit.go`, `relations_kinship.go`, `spotlight_rumor.go` | 直接调用 `DehydrateActor` (`scale.go:1133`) 遗漏清理 `Factions.Members` 与 `Houses.Residents`；`HydrateActor` Path A 未恢复它们 |
| **`World.Secrets`** | `addSecretWithWitnesses` (`ambitions.go:640`), `captureActionSecrets` (`:740`) | `ambitions.go:554`, `world.go:3548`, **`golden_finger.go:690`** | `golden_finger.go:690` 绕过 200 上限/去重/持有人立场刷新且写 `RootEventID: 0`；`Advance` 与子分发器直调绕过 `captureActionSecrets` |
| **`World.Ledger` / `Treasury` / `Polities.Treasury`** | `recordLedger*` (`states_ledgers.go`), `creditTreasury`/`debitTreasury` (`governance.go:112-159`) | `pol.Treasury` 在 `world.go:3810`, `scale.go:1863`, `governance.go:373-411`, `geography_polity.go:1543`, `relations_kinship.go:1947`, `justice_revolt.go:567` 多处内联修改 | 缺统一 `creditPolityTreasury`/`transferTreasury` 原语；`governance.go:408-416` 无官吏加派时账本不对称 |

---

## 二、部分结算与原子性漏洞清单（P2 重点销号）

1. **[高] `claim_turf` (`relations_kinship.go:1839-1844`)**：先执行 `a.Energy -= 5`，后检查 `prevFaction == factionID` 失败返回 → 失败扣精力。
2. **[高] `mediate_feud` (`relations_kinship.go:1764-1783`)**：先写入 `OffenderAccepted = true` / `AggrievedAccepted = true` / `MediationCoins`，后调 `transferBloodMoney` 失败返回 → 血仇未解但双方接受标记被污染。
3. **[高] `finishDeclarativeAction` (`effects_combat.go:1034-1275`)**：完成全部伤害、死亡、物品与效果变更后，最后才执行 `applyAttemptTransition` / `ApplyWorldLawChange`，失败时前期变更无法回滚。
4. **[高] 多 Tick 动作前置扣费无补偿**：`craft`（成长资产缺亲本 `world.go:2126`）、`adopt_facility`（`knowledge_laws.go:682`）、`hunt_threat`（威胁已归零 `world.go:2232`）、`applyDeclarativeAction`（完成时被世界律阻止或目标消失 `effects_combat.go:1025`）。
5. **[中] 校验失败前污染集合**：`marry`/`swear_bond` 先调 `ensureActorKinship`；`propose_alliance`/`declare_war`/`make_peace` 先调 `ensureFactionDiplomacy` 插入中立记录；`repay_loan`/`foreclose_collateral` 在 fail 分支改写 `l.Status = "repaid"`。
6. **[中] 7 处目标 `Trust` nil-map panic**：在已扣物品/货币/精力后直接写 `b.Trust[a.ID]`（`world.go:2316,3140,3227,3522,3694`, `knowledge_laws.go:397,532`）。

---

## 三、上帝视角泄漏与因果断链清单（P4 重点销号）

### 3.1 必须封堵的 10 类认知泄漏
1. **外交泄漏**：`FactionJoinEligibilityReason` (`world.go:764-786`) 与 `Nearby[i].FactionRelation` (`world.go:1256`, `diplomacy.go:586-600`) 直查 `w.Diplomacy`；`actorKnowsDiplomacyUpdate` (`diplomacy.go:757-781`) 在 `RootEventID == 0` 时无条件放行且对 `m.Statement` 未要求 `FactEventID > 0`。
2. **伪装破功**：`Neighbor.Tags` (`effects_combat.go:336-342`) 合并真实 `a.FormID` 的 `InnateTags` 而非 `PerceivedFormID`；`ExtortionTargets`/`BlackmailActions` 内部读取真实未伪装 `ActorPowerTier`/`CombatPower`。
3. **私有资产/仇怨透视**：`ExtortionTargets`、`BlackmailActions`、`LoanQuotes`（`personal_credit.go:25` 读 `lender.Money - 15`）、`OfferBlockReasons`、`Continuity.FulfillableAgreementIDs` 直接读取对手私有余额/库存/仇怨。
4. **`perceivedParties` 越界** (`party.go:101-122`)：仅比对 `leader.Location == a.Location`，缺 `SamePlace`（室内外/场馆隔离）与清醒检查。
5. **`DistrictState` 与 `FlowEdges` 全量裸露** (`world.go:967,1243`)：暴露 `BribeShieldTicks`、`Corruption`、`RacketPressure`、`Patrols` 与全图 `FlowEdges`。
6. **跨区实时状态透视**：`AccessibleHouses`、`Growth`/`Fields`、`Roads`/`VenueRoutes`（读取目标区实时天气与未目击断路）、`Polity.ReliefGrantAvailable`（全辖区扫描）、`LeadershipChallenges`（读远方首领私有属性/血量）、`OverdueBorrowerID`（读远方生死与产权）。
7. **无传播跳的跨区状态/记忆注入**：`ObserveCausalRelationEvents` 跨区扣师徒信任；`settleDeathAndInheritance` 跨区产生 `inheritance_dispute` 血仇直接进 `ActiveGrudgeTargets`；`openPersonalCase` 全区广播忽略室内/睡眠；`repay_loan`/`foreclose_collateral`/`house_rent_paid`/`file_appeal`/`trade_rejected` 跨区直塞记忆。
8. **远程感知逻辑反转 bug**：`world_events.go:408`、`geography_polity.go:965`、`knowledge_laws.go:1024` 把 `if indoor && (!spec.PenetratesIndoor || encShielding > spec.PenetrationPower)` 误写为 `&&`。
9. **谣言直接篡改客观真相**：`spotlight_rumor.go:300-453` 不查 `FactEventID > 0` 即扣减 `DistrictState.MoraleBonus -= 4`，且把 `witness_id` 误当发言人身份。
10. **13 处事件因果断链**：灾害致伤死（`ecology_process.go:407` 漏传 `disaster_struck`）、声明式战斗击杀（`effects_combat.go:1230,1263`）、突破失败致死（`states_ledgers.go:674`）、勒索与秘密发现/曝光（`ambitions.go:447,601`）、金手指曝光（`golden_finger.go:681`）、结盟/和谈/冤案申诉/起义大赦/血仇调解/小队生命周期/交易借贷协议全生命周期均缺失 `cause_event_id` 或 `root_event_id`。

---

## 四、LOD、守恒与跨粒度超凡力量缺陷清单（P5 重点销号）

| 级别 | 位置 | 缺陷 |
|---|---|---|
| **P0** | `scale.go:958-979` (`HydrateActor`) | 无过滤弹出 `Residents[0]`：永久销毁 `ResidentBand.Items`（破物品守恒）；伤员/囚犯水合时不减 `c.Wounded`/`c.Jailed`（双重计数） |
| **P0** | `resident_meals.go:98-100`, `lifecycle.go:187-191` | 饿死与非饿死群体伤亡直接丢弃 `ResidentBand.Items` 未记账本消耗（破物品守恒） |
| **P0** | `lifecycle.go:549-562` | 活跃个体生育加 `c.Population++` 但未调 `c.issueResidents(1, 0)` 与精力重算，直接破坏 `ResidentBand` 人口守恒与重载 |
| **P0** | `governance.go:757`, `justice_revolt.go:587,1011,1605` | 直接修改 `c.Jailed` 绕过 `Members` 与 `ResidentBand.Jailed`，大赦后存储成员刑满释放可把 `c.Jailed` 减成 **`-1`** |
| **P0** | `justice_revolt.go:1249-1251` (`triggerGranaryRiot`) | 直接 `hungry.AvgHunger--` 不同步 `Residents`/`HungerReliefRemainder`，破坏饥饿守恒 |
| **P0** | `effects_combat.go:1183-1239`, `world_events.go:310-334` | `cohort_struck` 绕过区级 `ActiveEffect` 护盾与群体 `PowerTier` 免疫；`venue` AoE 与 `WorldEventSpec` 冲击只遍历 `w.Actors`，群体与 `c.Members` 零伤亡 |
| **P0** | `effects_combat.go:265-276` (`IsActorPinnedFocal`) | 漏查 `FormSpec.PowerTier`、`GoldenFinger`、`ActiveEffect.SustainActorID`，高阶主角可能被脱水冻结 |
| **P1** | `lifecycle.go:732` | 饥荒救济遍历 Go map `w.Catalog.Items` 顺序随机，破坏确定性回放 |
| **P1** | `lifecycle.go:1110-1148`, `scale.go:1174,2250` | 群体寿命无视 `FormSpec.LifeExpectancyYears`；晋升与脱水丢失 `FormID` |
| **P1** | `governance.go:326`, `lifecycle.go:1221`, `market_credit.go:665,1598` | 税收/俸禄/基尼未调 `detachCohortMembers()`，用含 `Members` 的人口乘不含 `Members` 的资金池 |

---

## 五、配置零值保真、叙事与持久化缺陷清单（P6–P7 重点销号）

1. **零值覆盖**：`blueprint.go` 12+ 个字段、`pack.go` (`Ecology.Enabled`, `DistrictState` `Sanitation:0`/`Corruption:0`, `LifecycleSpec.MaxExpectancy:0`)、`sim` 5 大配置结构体的 52 个 `<= 0` 回填字段、`ExternalPatch` (`Health:0`/`Energy:0`/`RelationScore:0`)。
2. **非通用题材硬编码**：`house.go:205` (`a.Items["food"]`)、`world.go` (`carpenter`/`peddler`/`food`)、`scale.go:2536` (15 个古代职业 ID)、`governance.go` (火耗/保护费/御史台)、`production.go` (`farmer`/`farmland`)、`spotlight_rumor.go:746` (`p01..p30`)。
3. **叙事隔离**：`BuildChronicle` 接收可变 `*sim.World`、泄露真实 `ActorPowerTier` 与私密状态；`BuildChronicleArcs` 把未证实谣言与非人物 `TargetID` 混入客观事件弧；`secret_exposed` 的 `"witnesses"` 写了整型计数导致 `wre query -actor` 失效。
4. **持久化**：补齐 `schema_version`、单事务 `SaveTick`、外键约束、统一 `DecodeWorldSnapshot` 入口、`runtrace.Analyze` 可扩展 entry kind。

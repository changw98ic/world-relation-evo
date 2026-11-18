# 模拟测试的独立判据

以下测试数量和清理说明对应 2026-11-16。后续新增反例、隔离历史探针复查及长周期运行的当前状态见[全量修复记录](full-repair-2026-11-17.md)。

本次重建删除了 `internal/sim` 的全部 30 个旧测试文件。新体系有 9 个文件，其中 1 个只放小场景和辅助函数，8 个放规则测试，共 42 个顶层测试。没有把旧文件改名保留，没有继续用全年或百万人口长跑作为正确性的证据。

旧测试持久备份：`.artifacts/original-tests-20261116.tar`。备份只用于核查清理前的内容，不是持续测试的输入。

## 如何得出预期结果

预期结果来自四类依据：

1. 恒等关系。例如，实际交付两份货物，卖方必须少两份，买方必须多两份。
2. 场景明确声明的预算。例如，三份质量为一的木料，制成一块质量为三的木板；工作需要三刻；每人支付四点体力。这些数值用来检验预算是否执行，不是历史或现实参数。
3. 改变表示后应保持的事实。例如，将具名人物存入群体再取出，不构成出生、死亡、财产转让或失忆。
4. 扰动和反事实对照。例如，只切断道路、拿走一份材料、取消同意、移动目标，观察受影响的结果是否相应消失。合法对照必须成功，避免把“功能完全不工作”误认成正确拒绝。

测试不调用 `RecipeYield`、`SalePrice`、`CombatPower` 或同一个结算函数来计算 expected。小场景不使用其他包的测试辅助函数，不导入 `internal/agent`。`lawRestore` 只做 JSON 保存和载入，用来检验表示改变，不充当经济计算依据。

事件只用于确认失败被公开报告，或关联实际发生的消息和记忆。成功仍须检查人物、库存、钱、工时、地点或权利的变化。单独出现一个事件不构成通过的依据。

## 规则及验证边界

### R1：有限资源只能从实际所在地取用

- **规则**：一次取用必须扣减实际本地资源；不足时不能从远处补齐，也不能只扣一部分成本后假装成功。没有再生来源的资源不能自行增加。
- **为何成立**：本地有限库存是可用量的物理边界。远处的树木和水没有运输，就不能成为本地材料或饮水。
- **可推翻它的反例**：本地只剩一单位树木，配方需要两单位，但角色仍得到三份木料；或者干旱地区借用远处水源消除口渴。
- **验证边界**：只验证明确场景下的有限库存、本地性和失败原子性。不证明单位树木含多少木料，也不证明人的饮水需求量正确。
- **用例**：`TestLawEcologyExtractionCannotBorrowFromRemoteOrEmptyPool`、`TestLawEcologyWaterAccessRequiresActualLocalWater`。

### R2：水系分流、容量和生物成熟必须有因果先后

- **规则**：分支共享同一份水量。一次水系更新只走一条边。超过接收容量的水必须显式留在溢出量中。生物新增量需要亲本，且成熟前不能作为可捕获库存。
- **为何成立**：分流不是复制。更改地区遍历顺序不能改变水量或让刚到达的水多走一条边。新增生物与成熟生物是不同状态。
- **可推翻它的反例**：12 单位水沿两个分支各送 12 单位；中游刚收水就在同次更新送到下游；无亲本仍产生鱼；幼体在成熟时间前进入可捕获量。
- **验证边界**：验证整数水量和显式成熟时点，不验证降雨量、蒸发、河流速度、地下水、物种繁殖率或真实洪灾概率。溢出被记录不代表洪灾模型已经合理。
- **用例**：`TestLawEcologyWatershedConservesWaterAndMovesOneEdge`、`TestLawEcologyRecruitmentNeedsParentsAndMaturation`。

### E1：燃料、储能和用能必须分开结算

- **规则**：加料只转移燃料；转换按声明比例消耗燃料并增加储能；用电和线路损失共同扣减来源储能。线路切断时不能继续供电，也不能扣除未交付的能量。
- **为何成立**：同一份燃料不能同时留在个人物品中和发电机中。功率状态必须由当前可交付能量支持。
- **可推翻它的反例**：两份燃料既保留在背包又变成十份储能；储能耗尽后负载仍有电；损失 90% 的线路只花一份能量就交付一份。
- **验证边界**：只验证给定整数转换比例、连通性、损失和耗尽。没有检验真实热效率、交流电、多个电源调度最优性或生物体力与热量的换算。
- **用例**：`TestLawEnergyFuelTransferConversionAndExhaustion`、`TestLawEnergyDisconnectedAndLossyRoutesSpendOnlyDeliverableSupply`。

### P1：材料、工作位置和工时先于产物

- **规则**：制成品出现前必须支付声明的材料和体力，并完成声明的工时。高技能不能扩大非生物配方的质量预算。一个工位不能同时给两名工人使用。断电期间工作不推进，也不能重复收费。
- **为何成立**：生产是有输入和时间的转换。输入不足、停电或工位占用是能区分有效生产和凭空产物的独立条件。
- **可推翻它的反例**：三刻工作第一刻就交货；缺一份材料仍扣体力；两人占用唯一工位；停电期间推进任务；恢复供电再次扣材料。
- **验证边界**：检验声明预算，不证明三刻工时、技能曲线、工具磨损率、工厂效率或真实成本合理。
- **用例**：`TestLawProductionMaterialAndWorkPrecedeOutput`、`TestLawProductionShortageHasNoPartialPayment`、`TestLawProductionWorkstationIsExclusiveAndPowerPausePreservesMaterials`、`TestLawProductionEnteredHouseProvidesItsOwnWorkstation`。

### P2：土地占用、照料劳力和授权是不同约束

- **规则**：成熟但未收获的作物仍占用土地。陌生人不能直接收获。照料必须得到同意，并有实际水料和可用照料者。同一名照料者同一刻只能承担场景声明的一份工作。
- **为何成立**：成熟不会使作物从地里消失。所有权不会自动给他人同意。有人在场、有人有水、有一名空闲劳力，三者不能互相替代。
- **可推翻它的反例**：成熟就能在同一面积重播；只有同乡身份就能花别人的水；睡眠中的照料者同时照料两批作物。
- **验证边界**：没有验证真实耕地需求、季节生长期、肥力、天气产量或照料方式。`SetGrowthCare` 用于构造明确双方同意的场景；提议和接受的模型选择由其他测试范围负责。
- **用例**：`TestLawProductionFieldOccupationEndsAtHarvest`、`TestLawProductionCareRequiresConsentSuppliesAndOneWorkerPerTick`。

### P3、L1、C1：表示、活体存货和资源别名不改变预算

- **规则**：同样两名工人以个人或群体运行，六份木料、三刻工时、八点体力应得到同样产物。活体存货和保留亲本必须实际消耗饲养物资。`MoneyCost` 与 `CurrencyCosts[coins]` 指向同一钱包，必须先合并支付条件。明确为零的状态值不能在保存载入后恢复为默认值。
- **为何成立**：表示改变不增加劳力、材料或能量。动物是持续需要维护的资产。一个钱包或一个状态储备不能因为写法不同而被重复使用。
- **可推翻它的反例**：群体生产比同样工人提前交付；亲本放进生产批次后不再需要饮水；四枚钱支付总共五枚成本；状态归零后靠保存载入重新获得五点。
- **验证边界**：验证精确小群体与声明预算，不证明大群体调度、饲养死亡阈值或数值状态的现实含义正确。活体用例只检查完全供给和完全缺供，不涵盖部分饲养时不同个体的存活分布。
- **用例**：`TestLawProductionEquivalentWorkersHaveSameMaterialAndTimeBudget`、`TestLawLivingStockConsumesSuppliesAndCannotSurviveUnfundedForever`、`TestLawCostsAliasesCannotSpendSameWalletTwice`、`TestLawCostsZeroStateRemainsExhaustedAfterSerialization`。

### I1：具名人口、身份和继承不因存储方式而改变

- **规则**：具名人物放入群体再取出，其身份、年龄、技能、钱、货物、记忆和房产不变。匿名人口拆分应保留整数余量。具名成员不能同时被算作匿名劳力。死亡只发生一次；遗产从死者移给继承人，不能复制或顺便修好房产。死者不能继续完成生产。
- **为何成立**：存储表示不是世界事件。死亡改变存活人数，不自动销毁或复制财产。受伤、患病和坐牢是同一人的状态，不是新增人口。
- **可推翻它的反例**：每次取出群体成员都给新钱包；整数除法丢失最后一份钱或体力；受伤又患病的人被扣两次人口；重复通知死亡再次发遗产；死者的工作继续出货。
- **验证边界**：不证明亲属优先顺序、寿命、生育率、养老负担、迁移率、人口晋升分布或继承制度的现实合理性。完整人口迁移和自动聚焦策略没有被这组小场景覆盖。
- **用例**：`TestLawPopulationNamedPersonSurvivesRepresentationRoundTrip`、`TestLawPopulationAnonymousSplitPreservesIntegerRemainders`、`TestLawPopulationDeadCannotActAndEstateTransfersOnce`、`TestLawPopulationIllnessAndImprisonmentDoNotDuplicateWorkers`。

### X1、X2：同意、话语和实际交付是三件事

- **规则**：报价和提议不移动资产。指定买方才可接受；实际交易需要同地可交付的货物和价款。价格为零和恰好支付全部现金是合法输入。交货不足不能收钱。合同只能通过实际货款流变成已履约，文字不能代替交付。税款必须有对应的本地金库收入。
- **为何成立**：同意来自当事人的动作，履约来自资产流。双方总钱相等并不能单独证明收款人正确，因此还检查各方及对应金库。
- **可推翻它的反例**：第三人接受另一人的报价；远处收货；口头说“已付款”就标记履约；缺一份货仍扣全价；税款既入本地库又入公共库；免费赠予被拒绝。
- **验证边界**：不证明价格、税率、供需、市场匹配、公平性、贫富分布、财政持续性或真实契约制度。税用例中的 10% 是明确测试预算，不是合理税率的结论。
- **用例**：`TestLawExchangeRequiresNamedConsentAndLocalAtomicDelivery`、`TestLawExchangeShortageCannotChargeBuyer`、`TestLawExchangeTaxHasOnePayerAndOneJurisdictionReceipt`、`TestLawExchangeAgreementWordsCannotReplaceDelivery`。

### D2：债务必须对应同意的条款和现金流

- **规则**：借款申请不能自行放款；放款方要接受同一份条款；还款记账要与实际返还的钱一致。经过时间或人物被存入群体不能代替本人还款，不能误判其死亡。无抵押贷款不能没收后来得到的房屋；有约定抵押时，债权人的明确执行动作才可转移逾期抵押物。
- **为何成立**：债务是经同意形成的权利义务，借款本金和还款是可追踪现金流。时钟推进和存储改变不提供新的同意。
- **可推翻它的反例**：申请就创造现金；本人接受自己的申请；借款人被存入群体就违约；无抵押债权指向后来买到的任意房屋。
- **验证边界**：不证明利率、最低贷款金额、租金、信用风险、群体自动借贷、土地兼并或违约处置政策合理。
- **用例**：`TestLawDebtRequiresBothDecisionsAndRepaymentMatchesCash`、`TestLawDebtClockAndRepresentationCannotInventConsentOrCollateral`。

### S1、S2、T1：位置和在途资产不能跳过实际移动

- **规则**：不能跳过中间地区。人物在路上时不在两个静止端点，不能取用端点固定水源或厕所，但可以喝随身携带的水。房屋和场所边界隔离交谈。延迟效果必须在完成时重新确认目标在场。货物发出后属于在途资产，路程结束前不能进入收货库存。发货共享道路容量，保存载入不能恢复已用容量；断路时货留原地。
- **为何成立**：地点、旅行和交付是物理条件，不因持有者或动作已经被选择而消失。成功的原地对照排除“动作完全失效”的假通过。
- **可推翻它的反例**：路人同时与出发地、目的地的人交谈；目标已经走远仍被延迟攻击；同一刻两次使用全部道路容量；保存载入提前收货；没有路仍减少原地货物。
- **验证边界**：没有验证真实路速、负重、距离估算、地形能耗、天气损失、多路线最优分配或交通事故概率。水系与行政边界的完整交互也没有覆盖。
- **用例**：`TestLawSpaceTravelHasTimeCostAndNoEndpointPresence`、`TestLawSpaceHouseAndVenueBoundariesIsolateSpeech`、`TestLawSpaceDelayedEffectRechecksPhysicalTarget`、`TestLawTransportHarvestDeliveryKeepsCargoAndCapacity`、`TestLawTransportDisconnectedRouteKeepsGoodsAtOrigin`、`TestLawSpaceTravelCannotDrinkStationaryEndpointWater`、`TestLawSpaceTravelCannotUseStationaryEndpointSanitation`。

### K1、J1：个人信息必须有来源、渠道和行动条件

- **规则**：传话保留说话者、直接依据和最初来源，不升级为亲眼观察，不改变库存。回忆只选择自己的历史且不反过来修改历史。研究要先付成本并完成工时；发现不自动教会所有人，也不等于造好设施。远程传授要有测得且允许的距离。法院有案卷，不代表每个人知道案情；上诉要有自己的目击或转述线索，并到对应法院。
- **为何成立**：全局存在的信息不是人物已经收到的信息。知道技术与拥有设施是不同事实。司法行为必须使用该人物实际可得的证据和空间条件。
- **可推翻它的反例**：没听过的事件编号被当作证据；传话两次变成目击；说“有 999 份木料”就改库存；远程未知距离仍授课；未获知冤案的人直接用全局案卷上诉。
- **验证边界**：不评判话语真假，不证明回忆排序最像人、社会信任模型准确、传播概率合理、审判制度公正或任何内容模型理解了这些规则。实际模型的完整 trace 由主线程另行检查。
- **用例**：`TestLawInformationSpeechPreservesSourceWithoutCreatingGoods`、`TestLawInformationRecallSelectsOnlyOwnedHistoryAndDoesNotMutateIt`、`TestLawInformationResearchCostsWorkBeforePersonalDiscovery`、`TestLawInformationTeachingRequiresKnowledgeAndMeasuredChannel`、`TestLawJusticeAppealUsesPersonalEvidenceAndLocalCourt`。

### D1：失败不能伪装成成功，核算不能代替现实验证

- **规则**：任一人物决策失败，这一步不能部分结算任何人物、时间或环境。所有人物基于结算前状态决策。修改感知副本不能反向改世界。故意增加一个未记账物品时，核算必须报错；正常转换同时通过独立库存预算和核算。
- **为何成立**：一步失败就不应让前几个角色先花钱或让在途人物先抵达。感知是输入副本，不是对世界的写权限。总账只核对声明与持有量。
- **可推翻它的反例**：第二个人决策报错，第一人已经扣料；受击者在同一步决策时已经看到本步攻击造成的伤害；改感知中配方的输入量直接改变生产；未记账资产增加仍通过核算。
- **验证边界**：不证明全部并发交错、所有外部错误、持久化事务、完整重放、真实模型决策、百万规模性能或长期稳定性。核算通过不能证明生产、价格、生态或社会行为符合现实。
- **用例**：`TestLawEngineDecisionFailureLeavesWholeWorldUnchanged`、`TestLawEnginePerceptionIsAnIsolatedCopy`、`TestLawEngineDecisionsSharePreSettlementState`、`TestLawEngineLedgerDetectsAnUnfundedAssetPerturbation`。

## 30 个旧文件的清理去向

下表中的“重建”指旧文件已删除，其所属问题改用上面的规则和新场景验证。不是保留旧断言。未重建的部分明确列出，不能因新测试通过就声称仍被覆盖。

| 已删除旧文件 | 去向 | 本次没有验证的部分 |
| --- | --- | --- |
| `agriculture_production_test.go` | P2、L1、T1；土地占用、照料、实际交付和活体成本 | 肥料增产比例、天气产量、全部农业资产迁移 |
| `continuity_test.go` | X2；双方同意和实际履约 | 所有个人计划阶段、对话裁剪策略、截止日策略 |
| `ecology_audit_fix_test.go` | R1、R2、S1；有限水系、生物成熟和本地访问 | 灾害伤亡统计分布、跨区巡猎、概率标定 |
| `economy_audit_regression_test.go` | D2、X1；抵押同意、税款流和实际交付 | 多债权人分摊、工资及生活开支政策 |
| `economy_tax_regression_test.go` | X1；税款实际进入唯一对应金库 | 群体市场全部税种、海关税率、财政合理性 |
| `freight_capacity_test.go` | T1；有限容量、保存后容量和延迟交付 | 多条共享边的全部路由组合、全图最优发运 |
| `memory_reference_test.go` | K1、I1；自己的历史及保存不变 | 旧排序参考实现与所有排序权重 |
| `memory_test.go` | K1；来源、选取和副本隔离 | 旧实现复写的排序比较器和大量随机排序输入 |
| `physical_rules_test.go` | R1、E1、P1、P2、D2、T1 | 百万人口长测、全部物理配置矩阵 |
| `population_energy_test.go` | I1、P3；拆分余量和等效工人体力预算 | 晋升与大规模人口结构转移 |
| `population_migration_test.go` | I1；身份和整数资产余量 | 所有迁移方向、迁移率及其长期分布 |
| `population_regression_test.go` | I1、D2；身份、死亡、财产、存活借款人 | 所有家庭出生、寿命和继任者选择 |
| `population_runtime_test.go` | I1、P3；具名成员不会重复算劳力 | 全部成员后台行为与群体策略组合 |
| `production_audit_regression_test.go` | P1、E1、S2、K1；停电、损失、延迟目标、授课 | 全部加速设施输入组合、建设停工与恢复矩阵 |
| `production_validation_test.go` | 删除旧配置拒绝矩阵；本目录不把它算作现实规则测试 | 外部配置矩阵由 pack 测试范围另行处理；本文件不宣称已覆盖 |
| `resource_accounting_test.go` | R1、E1、P1、D1；独立预算加核算负例 | 全部资源持有者组合与长期财政流 |
| `root_audit_fix_test.go` | C1、S2；钱包别名、状态耗尽、延迟生效 | 所有状态转换失败分支及世界规则改写 |
| `space_society_regression_test.go` | S1、T1、J1；空间边界、交付、知情上诉 | 所有政体、领土、社会变动组合 |
| `effects_combat_test.go` | S2、C1、I1；延迟攻击、成本和死亡停止工作 | 全部伤害类型、防御、持续效果、跨尺度伤亡 |
| `epistemic_regression_test.go` | K1、D2、J1；来源、存活身份、个人司法线索 | 所有宏观传闻、群众知识和行为选择 |
| `geography_polity_test.go` | R2、S1、X1；水系、移动边界、本地税款 | 所有统治链、独立地区、法令继承和政体干预 |
| `justice_revolt_test.go` | J1；知情与不知情、在场与远处的上诉对照 | 冤案概率、民怨、税罢工和起义动力学 |
| `knowledge_laws_test.go` | K1、P1；研究工时、个人传播与设施分离 | 全部世界规则覆写、技术扩散率及规则权能 |
| `market_credit_test.go` | X1、D2；资产流、债务同意、抵押边界 | 价格动态、囤积、兼并、租金与自动群体借贷 |
| `personal_credit_test.go` | D2；同意、现金还款、未约定抵押不可没收 | 全部贷款取消、期限和利率边界 |
| `relations_kinship_test.go` | I1、X2；继承唯一性和协议交付 | 婚姻、嫁妆、复仇、宗族关系及垄断制度 |
| `replay_inspect_test.go` | D1、I1、T1；局部保存不变与一步失败原子性 | 完整决策日志重放及命令行恢复由主线程处理；本目录没有重建它们 |
| `spotlight_rumor_test.go` | I1、K1；具名人物存取与消息来源 | 自动聚焦阈值、流言恐慌传播及所有聚焦切换 |
| `states_ledgers_test.go` | C1、E1、D1；共享成本、状态耗尽、燃料和核算负例 | 所有形态、寿命、货币精炼、吸收和长期状态演化 |
| `world_test.go` | 拆入 R1 至 D1 的小场景；不再用巨型综合断言作判据 | 旧文件未被上述具体用例列出的功能均不宣称保留覆盖 |

## 实跑与缺陷

2026-11-16，主线程修复两处源头后，本子任务完成了最终实跑：

| 实际执行 | 结果 |
| --- | --- |
| `go test ./internal/sim` | 通过，0.088s |
| `go test -race ./internal/sim` | 通过，1.136s，未报告并发数据冲突 |
| `go test ./internal/sim -list '^TestLaw'` | 确认 41 个顶层规则测试 |
| `go list -json ./internal/sim` | 确认只有上面的 9 个新测试文件，没有旧测试文件 |
| 对 9 个新文件运行 `gofmt -l` | 无输出，格式化完成 |

以下保留两个缺陷先失败、再修复的过程。完整运行不再有默认失败用例。

**饮水缺陷及修复验证**：

- 实际 `Apply(move)` 后两次 `World.Step`：初始 tick 为 22，道路需走 5 刻，出发地固定水源有 1 单位；到 tick 24 时人物仍剩 2 刻旅途。
- 修复前得到 `origin_water=0 thirst=0 remaining_travel=2`；独立预期是 `origin_water=1 thirst=1`。静止对照正常取到本地水。
- 主线程在 `internal/sim/lifecycle.go` 自动饮水分支排除了 `InTransit()`。本子任务实际复跑原 40 个测试，全部通过（3.109s），包括加强后的同一步决策快照用例。
- 后续新增“旅途中有随身水”对照，静止取水、旅途不能取端点水、旅途中仍可饮用随身水这三组均通过。修复没有把随身饮水一并禁用。

**卫生设施缺陷及修复验证**：

- 同样的 5 刻旅途，在 tick 24 时仍剩 2 刻。初始出发地厕所夜土池为 3，卫生值为 80；人物排泄负担为 2，已消化食物为 4；场景明确每 2 食物可转 1 夜土。
- 静止对照正确完成转换，结果为夜土池 5、卫生值 80、排泄负担 0、已消化食物 0。
- 旅途组竟得到同样结果；独立预期应保留 `nightsoil=3 sanitation=80 waste=2 digested_food=4`，因为人物无法在出发地固定厕所完成转换。
- 新增对照时，定向测试和完整 `go test ./internal/sim` 均实际失败；完整运行只报告这个失败。
- 源头是 `internal/sim/lifecycle.go` 自动卫生设施结算按 `a.Location` 操作，没有排除 `InTransit()`。主线程在该分支加入 `!a.InTransit()`。本子任务没有修改运行时、跳过测试或修改预期。
- 修复后，完整 sim 测试和带并发检测的完整 sim 测试均通过。静止卫生设施和随身饮水对照仍通过。

**真实 trace 后的补充验证**：

- 独立用例 `TestLawProductionEnteredHouseProvidesItsOwnWorkstation` 先复现：进入完好房屋后，仅因房屋没有单独声明场所身份，实际空闲工位仍被排除。声明场所的合法对照通过。
- 主线程修复 `productionFacility`：已有明确当前房屋时，不再用室外场所过滤该房屋的工位。两个对照均通过，实际材料、工时和成品也符合声明预算。
- 最终全仓普通和带真实数据库的并发检查均通过，其中 sim 为 42 个顶层测试、15 个子用例。上表的 41 项是补入该反例之前的历史清点。
- 真实模型已完成 32 步。人工审查发现了零产出取水、只讲话未见办事进展、重复往返等问题。详见[真实运行审查](live-trace-review-2026-11-16.md)。工位修复发生在实跑之后，未据此声称模型行为已经改善。

## 仍需真实 trace 回答的问题

小场景证明不了人物在真实模型选择下会形成合理生产、饮食、交易、照料、迁移和司法行为。必须查看实际候选项、选中的动作、动作失败原因、供给和生活变化。尤其要检查：账目虽然平衡但没有食物来源；人物只睡觉或闲逛；双方持续提出没有对方同意的交易；生产地点与人物旅行脱节；有资源却缺少可执行动作；宏观人口自动行为绕过个人同意。完整流程成功返回只证明流程运行结束，不回答这些问题。

本子任务新增运行时检查、限制、兜底、重试：**无**。本子任务只改测试和本文档。上面的两处运行时修复由主线程处理。测试中所有数值是明确的小场景预算，不新增产品限制。

# 模拟审计修复记录（2026-11-16）

> 历史记录：本文记述上一阶段 73 条审计记录的修复和当时的测试结果。随后用户要求清理全部测试，本文所引用的旧测试已被重建替代，不能继续用作当前覆盖证明，也不能据此宣称模拟合理。当前依据、真实数据库与模型实跑结果见[新的测试说明](testing/README.md)；原始 trace 暴露的问题见[真实运行审查](testing/live-trace-review-2026-11-16.md)。下文“数据库未配置”等描述只对应旧阶段。

本记录对应原审计的 **73 条原始记录**。每条都关联具体改动和实际运行的回归测试。重复记录分别保留原编号；这些记录不表示 73 个独立缺陷。人口组原先没有编号，此处按原顺序命名为 POP-01 至 POP-12。

明确重复关系：AG10 与 E2；PROD-01 与 AG6；E7 与 POP-02。

`tick` 是模拟的一步，等于 15 分钟。“收起”指把具名人物保存为群体成员；这不会抹去其身份和私产。“群体”中的匿名人口与具名成员分别结算，合计人数保持不变。

## 实际验证

- `go build -o /tmp/wre-audit-fixed ./cmd/wre`：通过。
- `go vet ./...`：通过。
- `go test ./internal/agent -count=1`：通过，4.003秒。
- `go test ./internal/sim -run '^TestRootAudit|^TestPopulation|^TestAgricultureProduction|^TestProduction|^TestEcologyAudit|^TestEconomy|^TestSpaceSociety|^TestFreight' -count=1`：全部匹配回归通过，0.333秒。
- `go test ./internal/sim ./internal/agent ./internal/pack -run 'Physical|GrowthCare|External|Reference|WorldLaw' -count=1`：三个包全部匹配测试通过；含真实照料选择到结算、外部参数边界、世界包32tick运行和严格保存恢复。
- `go test -json ./...`：全仓5个包通过，392个测试及子用例通过，6个可选测试跳过；internal/sim 307.965秒。退出码0。
- `go test -race -json -timeout 30m ./...`：全仓5个包通过，392个测试及子用例通过，6个可选测试跳过；internal/sim 1201.981秒。退出码0，未发现并发数据竞争。
- `逐条比对正式清单与两轮完整测试日志`：73条记录列出的命名回归全部出现在普通测试和并发竞争检测的通过结果中，没有遗漏或仅凭名称推定通过的测试。
- `git diff --check`：通过。

未设置WRE_TEST_DATABASE_URL。PostgreSQL专用测试会按项目约定跳过；本轮不把数据库持久化集成记为通过。250天可选粮食周期测试未启用。

## 逐条对应

### 自然资源与气候灾害

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| ECO-01 | 普通灾害和兽潮只伤群体，已展开人物免疫。<br>**修复：**人物、收起的人物和匿名群体按相同暴露率受灾，人数只记一次。 | `TestEcologyAudit01ActorsAndMembersShareDisasterExposure`<br>`TestEcologyAudit01ChildrenHaveSameDisasterExposure`<br>`TestEcologyAudit01ThreatSurgeActorsAndMembers` |
| ECO-02 | 一份救济水保护整个千人群体。<br>**修复：**减灾效果按实际物资覆盖人数计算；一份物资只保护对应人数。 | `TestEcologyAudit02ReliefCoverageFollowsAmount` |
| ECO-03 | 猎人瞬间剿兽至不连通地区。<br>**修复：**巡山保存真实去程、工作、回程和驻地，并占用共同劳动力。战利品随队返回。 | `TestEcologyAudit03PatrolExposureUsesActualLocation`<br>`TestEcologyAudit03PatrolTravelsWorksAndReturns` |
| ECO-04 | 野外遭遇掉落不扣来源且漏记产出。<br>**修复：**人物和群体狩猎共用有限动物来源扣除及真实产出记账。 | `TestEcologyAudit04EncounterConsumesFiniteSourceAndAccountsOutput`<br>`TestEcologyAudit04IndividualHuntUsesSourceAndLedger` |
| ECO-05 | 零掉落概率仍掉落。<br>**修复：**按声明概率处理掉落，取消强制最少一份；零概率没有掉落。 | `TestEcologyAudit05ZeroAndFractionalDrops` |
| ECO-06 | 停工休养仍遭受职业事故。<br>**修复：**职业事故只作用于实际地点中正在参与对应工作的人员。 | `TestEcologyAudit06AccidentsRequireActualAvailableWork` |
| ECO-07 | 灾害到期后继续影响产量。<br>**修复：**灾害在声明到期的那个tick停止生效。 | `TestEcologyAudit07DisasterExpiresAtItsExactTick` |
| ECO-08 | 显式零天气产量被忽略。<br>**修复：**明确的零天气或季节倍率保留为停产；个人和群体在扣费前停止开工。 | `TestEcologyAudit08ExplicitZeroYieldStopsProduction`<br>`TestEcologyAudit08ZeroYieldBlocksIndividualAndCohortWork` |
| ECO-09 | 计算受灾人数为零仍至少伤两人。<br>**修复：**取消每群体保底伤亡；零暴露产生零伤亡。 | `TestEcologyAudit09ZeroExposureHasNoMinimumInjuries` |
| ECO-10 | 林地可采集标志没有对应资源。<br>**修复：**可采集地形与实际资源来源同步，林地有声明的采集来源。 | `TestEcologyAudit10ForageFeatureHasSource` |
| ECO-11 | 生物补充不依赖繁殖来源及成熟期。<br>**修复：**生物资源依赖声明亲本，新增个体先等待成熟，灭绝不再自行逆转。 | `TestEcologyAudit11ReproductionRequiresParentsAndMaturation` |
| ECO-12 | 所有地区自动生成可饮用水源。<br>**修复：**取消全地区自动饮水；只有真实水源或显式配置的存量可饮用。 | `TestEcologyAudit12WaterNeedsDeclaredSource` |
| ECO-13 | 日常天气补水与洪水不连接真实水系。<br>**修复：**真实水量沿河流或含水层移动；上游取水影响下游，洪水需要实际溢流。 | `TestEcologyAudit13FloodRequiresConnectedOverflow`<br>`TestEcologyAudit13UpstreamExtractionChangesDownstreamWater` |
| ECO-14 | 所有资源保留5%不可开采。<br>**修复：**移除隐藏的5%禁采份额，最后一单位资源可以正常使用。 | `TestEcologyAudit14LastResourceUnitCanBeExtracted` |
| ECO-15 | 默认仪式直接改变灾害持续时间和普通野兽活动。<br>**修复：**普通仪式只改变社会状态；物理减灾和安抚野兽需要显式超自然规则。 | `TestEcologyAudit15IndividualOrdinaryRitualCannotChangeNature`<br>`TestEcologyAudit15RitualObjectiveEffectsRequireExplicitCapability` |

### 农业、畜牧与土地

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| AG1 | 异地所有者的作物成熟后没有收获出口。<br>**修复：**异地收获保留当地货权；授权群体经共享路容运输，个人到实际地点领取。 | `TestAgricultureProductionAbsentCaretakerShareStaysLocal`<br>`TestAgricultureProductionAutomaticRemoteDelivery`<br>`TestAgricultureProductionHarvestSharesFreightCapacity`<br>`TestAgricultureProductionRemoteHarvestAndDelivery`<br>`TestAgricultureProductionStoredOwnerUsesOwnLabor`<br>`TestFreightCapacitySharesEdgesAndSurvivesSnapshot`<br>`TestFreightMarketSharesCapacityAcrossCallsAndOtherDispatches` |
| AG2 | 展开群体最后一人会把在产作物留给零人口群体。<br>**修复：**最后匿名成员展开时迁移既有作物、田产和亲畜；具名成员仍保留个人产权。 | `TestAgricultureProductionTransferPreservesAssetsAndCare`<br>`TestAgricultureProductionTransferRetainsAnimalBatchDebt`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner`<br>`TestPopulationLastAnonymousHydrationTransfersGrowingAssets` |
| AG3 | 死亡继承遗漏在产作物与保留亲畜。<br>**修复：**死亡继承同时迁移在产作物、田产权利、保留亲畜及照料债务。 | `TestAgricultureProductionTransferPreservesAssetsAndCare`<br>`TestAgricultureProductionTransferRetainsAnimalBatchDebt`<br>`TestPopulationFix02InheritancePrecedesEscheat`<br>`TestPopulationFix02StoredHeirAndUnclaimedEstate`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce` |
| AG4 | 田产面积、占用与使用权没有进入农业结算。<br>**修复：**田块保存面积、地点、所有权和使用权；播种占地，收获前不能重复使用。 | `TestAgricultureProductionFieldAreaAndPersistence`<br>`TestAgricultureProductionZeroInitialLandBudget`<br>`TestPopulationLastAnonymousHydrationTransfersGrowingAssets` |
| AG5 | 被监禁或零劳动人口仍能照料和自动收获。<br>**修复：**照料和收获使用真实可用劳动，监禁、休养和零劳力不能继续劳动。 | `TestAgricultureProductionCareUsesLaborAndExplicitConsent`<br>`TestAgricultureProductionEnergyExcludesStoredMembers`<br>`TestAgricultureProductionStoredOwnerUsesOwnLabor`<br>`TestPhysicalCohortWorkTakesConfiguredTime` |
| AG6 | 生物配方消耗肥料与工具却丢弃增产结果。<br>**修复：**实际肥料投入进入生物配方的声明产量预算；不再扣肥后丢弃结果。 | `TestAgricultureProductionFertilizerEnvironmentAndSnapshot`<br>`TestAgricultureProductionPartialFertilizerAndBatchParity`<br>`TestBiologicalOutputUsesConfiguredQuantity`<br>`TestFarmerDiversifiedTasksAndCompostYield` |
| AG7 | 农业环境产量规则被固定产量覆盖。<br>**修复：**按生长过程的实际环境计算减产，个人和群体采用相同的逐批取整。 | `TestAgricultureProductionFertilizerEnvironmentAndSnapshot`<br>`TestAgricultureProductionPartialFertilizerAndBatchParity`<br>`TestEcologyAudit08ZeroYieldBlocksIndividualAndCohortWork` |
| AG8 | 家畜作为库存没有持续生存成本，缺料生物只暂停。<br>**修复：**库存牲畜持续吃喝；缺料累积短缺时间并按声明规则死亡。 | `TestAgricultureProductionAnimalClockSurvivesReservationAndReturn`<br>`TestAgricultureProductionEmptyLivingCareSnapshot`<br>`TestAgricultureProductionLivingStockCostsAndDeath`<br>`TestAgricultureProductionParentDeathPreservesMatureGoods`<br>`TestAgricultureProductionTransferRetainsAnimalBatchDebt`<br>`TestPhysicalGrowthCannotAdvanceWithoutSupplies` |
| AG9 | 公会自动接管照料没有同意、报酬或补给归属协议。<br>**修复：**照料协议明确双方接受、当地补给人、收获权和分成；公会身份不等于同意。 | `TestAgricultureProductionAbsentCaretakerShareStaysLocal`<br>`TestAgricultureProductionCareUsesLaborAndExplicitConsent`<br>`TestAgricultureProductionRemoteHarvestAndDelivery`<br>`TestPhysicalRemoteOwnerCannotSupplyAbsentLocalCaretaker`<br>`TestPhysicalRemoteOwnerUsesLocalCaretakerAfterDehydration`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner` |
| AG10 | 第二个兼并债权人覆盖第一个债权人的收益权。<br>**修复：**同E2：按债权人分别保存收益份额，不再覆盖先前债权人。 | `TestEconomyAnnexationCapPreservesEarlierCreditors`<br>`TestEconomyMultipleCreditorsRetainSharesAndReceiveRent`<br>`TestEconomyRentRoundingDoesNotMultiplyMinimum` |

### 人物、人口、家庭与粒度转换

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| POP-01 | 群体恢复供水后仍持续脱水死亡。<br>**修复：**供水恢复后解除脱水状态，按实际饮水及余数推进。 | `TestPopulationFix01WaterRestoresExistingThirst` |
| POP-02 | 死亡处理抢先转走继承现金。<br>**修复：**死亡先按真实继承关系分配现金和资产，剩余部分才进入公共处置。 | `TestPopulationFix02InheritancePrecedesEscheat`<br>`TestPopulationFix02StoredHeirAndUnclaimedEstate`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce` |
| POP-03 | 自动降级使已婚配偶永久失联。<br>**修复：**收起后保留具名成员和亲属引用，配偶仍能被正常找到。 | `TestPopulationFix03MarriageKeepsResolvableIdentity`<br>`TestPopulationStoredMembersSurviveCohortPromotion`<br>`TestPopulationStoredMembersSurviveReintegration` |
| POP-04 | LOD 把老人转成年轻人并清除口渴。<br>**修复：**年龄、口渴及其他个人状态原样保存；匿名人口的生理平均不受表示转换影响。 | `TestPhysicalDigestedFoodSurvivesHydrationRoundTrip`<br>`TestPopulationAnonymousDeathsSettleEnergyRemainder`<br>`TestPopulationAnonymousEnergySurvivesPromotionAndUprising`<br>`TestPopulationAnonymousEnergySurvivesReintegration`<br>`TestPopulationAnonymousHydrationPreservesPaidEnergy`<br>`TestPopulationFix04AgeAndPhysiologySurviveLOD`<br>`TestPopulationNamedLODLeavesAnonymousEnergyUnchanged`<br>`TestPopulationStoredMembersSurviveCohortPromotion`<br>`TestPopulationUprisingOnlyMovesAnonymousAdults` |
| POP-05 | 病伤人口被排除在饥荒和老年死亡之外。<br>**修复：**病伤人口也参与真实饥荒、老年及其他死亡，并分别记清状态死亡数。 | `TestPopulationFix05SickAndWoundedCanDie`<br>`TestPopulationMortalityReportsRemovedStatuses`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce`<br>`TestSpotlightRumorThreeDayMillionPopConservation` |
| POP-06 | 新生儿进入异区同职业群体。<br>**修复：**新生儿只进入实际出生地区的相应群体。 | `TestPopulationFix06NewbornStaysInLocalDistrict` |
| POP-07 | 直接合并病伤人物重复扣除同一个人。<br>**修复：**病与伤的重叠人数单独保存，同一个人不会重复扣除劳动人口。 | `TestPopulationFix07WoundedSickOverlapCountsOnce`<br>`TestPopulationMortalityReportsRemovedStatuses`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce` |
| POP-08 | 小群体年龄层永远停止成熟。<br>**修复：**小群体保留年龄推进余数，人数不足整批时仍会逐步长大和衰老。 | `TestPopulationFix08SmallCohortsKeepAgeRemainders`<br>`TestPopulationStoredMembersSurviveReintegration` |
| POP-09 | 监禁无需食物和水仍能恢复。<br>**修复：**监禁人员仍需要食物和水，恢复受真实生理状态约束。 | `TestPopulationFix09PrisonersConsumeRealRations`<br>`TestPopulationStoredSentenceExpiresExactlyOnce` |
| POP-10 | 冷伤在临死前停止。<br>**修复：**低体温继续作用于濒死者，不再因低健康值免疫。 | `TestPopulationFix10ColdExposureCanKill`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce` |
| POP-11 | LOD 清除身体形态和资源槽，改变生理规则并凭空补充资源。<br>**修复：**收起和展开保存身体形态、明确零值资源槽及个人生理规则。 | `TestPopulationEmptyMembersHaveCanonicalJSON`<br>`TestPopulationFix11FormAndResourceSlotsSurvivePersistentLOD`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner`<br>`TestPopulationLastAnonymousHydrationTransfersGrowingAssets`<br>`TestPopulationStoredActorMustHydrateBeforeIndividualExecution` |
| POP-12 | LOD 丢失个人知识、附加职业与职业经验。<br>**修复：**个人知识、附加职业及经验保存在成员身份中，不再重建为职业模板。 | `TestPopulationFix12KnowledgeAndProfessionXPSurviveLOD`<br>`TestPopulationStoredMembersSurviveCohortPromotion`<br>`TestPopulationStoredMembersSurviveReintegration`<br>`TestPopulationUprisingOnlyMovesAnonymousAdults` |

### 生产、设施、能源与技术

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| PROD-01 | 生长配方吃掉增产材料，但增产量被强制清零。<br>**修复：**同AG6：实际投入的肥料增益进入生物产出预算。 | `TestAgricultureProductionFertilizerEnvironmentAndSnapshot`<br>`TestAgricultureProductionPartialFertilizerAndBatchParity`<br>`TestBiologicalOutputUsesConfiguredQuantity` |
| PROD-02 | 群体与个体的同等级同配方产量不同。<br>**修复：**相同投入与配方对个人和群体使用同一材料预算和逐批环境计算。 | `TestAgricultureProductionMaterialAndNutritionBudgets`<br>`TestAgricultureProductionParityEnergyAndFacilityCapacity`<br>`TestAgricultureProductionPartialFertilizerAndBatchParity` |
| PROD-03 | 制作技能直接增加物品件数，缺少物质和营养约束。<br>**修复：**物品声明质量及营养单位，配方声明转换预算；技能、工具和设施不再凭空加件。 | `TestAgricultureProductionMaterialAndNutritionBudgets`<br>`TestCareerProgressionScholarToXiucaiAndBlacksmithRanks`<br>`TestCustomProfessionWorksWithoutCodeChanges`<br>`TestGenreDecoupledSciFiColonyPackWithoutCodeChanges` |
| PROD-04 | 电网遇到无效最短路径后放弃有效旁路。<br>**修复：**电网按实际可送达量排除无效路径，并继续使用有效旁路。 | `TestProductionPowerBypassesUndeliverablePaths` |
| PROD-05 | 设施停电只阻止开工，已开工生产照常完成。<br>**修复：**长任务每tick检查供电，断电暂停有效工作，恢复后继续且不重复扣料。 | `TestProductionCraftPausesThroughPowerCut` |
| PROD-06 | 有射程但缺坐标时跳过范围限制。<br>**修复：**有限射程需要可验证的同地、坐标或实际距离，缺坐标不能视为无限射程。 | `TestProductionFiniteAttackRangeNeedsKnownDistance`<br>`TestProductionKnowledgeTeachingNeedsKnownChannelDistance` |
| PROD-07 | 多tick攻击只在开始检查距离，结算能命中已离开的目标。<br>**修复：**延迟攻击在结算前重新检查目标存活、位置和距离；失效则明确失败。 | `TestProductionDelayedAttackRevalidatesTarget`<br>`TestRootAuditFirearmLawCoversDeclarativeAndDelayedActions` |
| PROD-08 | 群体生产不检查配方所需体力，合法高耗能配方会扣到负值。<br>**修复：**群体开工批数受实际体力支持，不会把合法高耗能工作扣成负数。 | `TestAgricultureProductionEnergyExcludesStoredMembers`<br>`TestAgricultureProductionParityEnergyAndFacilityCapacity`<br>`TestAgricultureProductionShiftUsesActualEnergyCost` |
| PROD-09 | 设施没有并行容量，0耐久房屋设施仍可生产。<br>**修复：**设施使用声明的工位容量；损坏、无电或已满的工位不能再开工。 | `TestAgricultureProductionCohortPowerPauseAndRestore`<br>`TestAgricultureProductionParityEnergyAndFacilityCapacity`<br>`TestPhysicalCohortWorkTakesConfiguredTime`<br>`TestPhysicalCompostIsProcessedAfterDeclaredCycle` |
| PROD-10 | 工具磨损按批次向上取整，规模越小工具消耗率越高。<br>**修复：**工具磨损跨批次保留余数，拆成小批次不会额外消耗工具。 | `TestAgricultureProductionToolWearIndependentOfBatchSize`<br>`TestAgricultureProductionTransferPreservesAssetsAndCare` |
| PROD-11 | 研发与建设实际消耗材料，但完全漏记物品消耗账。<br>**修复：**研究和建设的真实材料消耗进入物品账。 | `TestProductionResearchAndBuildRecordMaterialConsumption` |
| PROD-12 | 采用房屋设施只检查同地区，允许跨地点施工。<br>**修复：**施工必须处于实际地点；离开现场后暂停，返回后可以继续。 | `TestProductionHouseConstructionRequiresActualSite` |
| PROD-13 | 消耗环境资源的设施耗尽资源后仍保留全部增产效果。<br>**修复：**运行投入按tick完整支付后才获得加速；耗尽或停电后停止当期增益。 | `TestMultiGenre_SciFiTechDiffusion_AndImmutableWorldLaws`<br>`TestMultiGenre_XianxiaSpiritArray_DiscoveryTeachBuildAndAmbientDrain`<br>`TestProductionCraftAccelerationStopsWhenOperatingInputsRunOut`<br>`TestProductionFacilityConsumesOnlyFundedPoweredOperatingTicks`<br>`TestProductionFacilityInputDefinitionValidation`<br>`TestProductionFacilityInputsCompeteAndEffectsAreIndependent` |

### 经济、产权与财政

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| E1 | 无抵押贷款可以没收未约定房屋。<br>**修复：**抵押执行只使用贷款双方原先确认的房屋，无抵押不能临时指定房屋。 | `TestEconomyUnsecuredLoanCannotSeizeLaterHouse`<br>`TestMarketCreditDynamicPricingHoardingUsuryAndAnnexation` |
| E2 | 第二债权人覆盖第一债权人的产权。<br>**修复：**每个债权人的份额单独保存，现金与实物租金按各自份额分配。 | `TestEconomyAnnexationCapPreservesEarlierCreditors`<br>`TestEconomyMultipleCreditorsRetainSharesAndReceiveRent`<br>`TestEconomyRentRoundingDoesNotMultiplyMinimum` |
| E3 | 救济支付了余额但没有粮食交付。<br>**修复：**救济采购同时交货付款，不足完整批价时保留国库资金。 | `TestEconomyReliefFindsAffordableFoodWithExistingRation`<br>`TestEconomyReliefPaysOnlyForDeliveredBatch` |
| E4 | 药品销售可使库存负数。<br>**修复：**销售检查实际整批库存，库存正好够时允许卖完，库存不够时不扣货。 | `TestEconomyAutomaticFoodSaleCannotExceedActorStock`<br>`TestEconomyMedicineSaleHonorsEntireAvailableInventory`<br>`TestEconomyReliefPaysOnlyForDeliveredBatch` |
| E5 | 人物工资忽略财政预算。<br>**修复：**人物与群体共用原有工资预算，全部支付与截留合计不超过该预算。 | `TestEconomyActorOnlyWorldPaysCivicSalaryDaily`<br>`TestEconomyPayrollDiversionStaysInsideBudget`<br>`TestEconomyPayrollSharesOneBudget` |
| E6 | 群体购买人物商品没有更新生产销售计划。<br>**修复：**群体实际买入人物商品后记录真实卖方和交付数量，销售计划随成交推进。 | `TestEconomyCohortPurchaseCompletesActorProductionPlan` |
| E7 | 有群体时先转走死者金币导致子女继承为零。<br>**修复：**同POP-02：现金先按继承关系分配，不能先被群体统一转走。 | `TestPopulationFix02InheritancePrecedesEscheat`<br>`TestPopulationFix02StoredHeirAndUnclaimedEstate`<br>`TestPopulationLODLeavesProductionRightsOnNamedOwner`<br>`TestPopulationStoredMemberHealthAndDeathCountOnce` |

### 空间、物流、信息与治理

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| SS-01 | 同一地图的地区移动可绕过场所路线能力与耗时。<br>**修复：**地区移动复用实际场所路线，保留能力要求、能量与旅行工时。 | `TestMultiGenre_MortalWatershed_VenueIsolationAndCrossBorderFlood`<br>`TestMultiGenre_XianxiaSectDomain_DivineSenseSwordFlightAndEpistemicFirewall`<br>`TestSpaceSocietyDistrictMoveUsesVenueRoutes` |
| SS-02 | 暴动可以从没有道路连接的远区粮仓直接抢粮。<br>**修复：**暴动只抢当地实际存在的粮仓，不能即时取得异地库存。 | `TestSpaceSocietyRiotOnlySeizesLocalGranary` |
| SS-03 | 宏观战斗、镇压、法院绕过空间和实际辖区。<br>**修复：**宏观战斗、法院、申诉和镇压需要真实在场人员与当前辖区。 | `TestAgricultureProductionEmptyLivingCareSnapshot`<br>`TestIntegratedFourPillarsMillionPopConservation`<br>`TestJusticeRevoltWrongfulFramingAppealUprisingAndAmnesty`<br>`TestMillionPopulationSimulationAndHydration`<br>`TestRelationsKinshipMarriageVendettaTurfAndMonopoly`<br>`TestSpaceSocietyCourtAndSuppressionRequirePresentAuthority`<br>`TestSpaceSocietyMacroCombatRequiresPresence`<br>`TestSpaceSocietyMacroSparringRequiresPresence`<br>`TestSpaceSocietyPromotionKeepsGraduatesInTheirDistrict`<br>`TestSpaceSocietyScheduledConflictsRequirePresence`<br>`TestSpaceSocietyVendettaAndMacroMediationCannotReachRemoteParties` |
| SS-04 | 任何居民可对未知远处仇怨强制扣款结案。<br>**修复：**调解需要已知仇怨、实际沟通及双方同额接受，第三人只能提出方案。 | `TestMediationOptionsReachBilateralSettlement`<br>`TestRelationsKinshipMarriageVendettaTurfAndMonopoly`<br>`TestSpaceSocietyMediationNeedsKnowledgePresenceAndBothAcceptances`<br>`TestSpaceSocietyVendettaAndMacroMediationCannotReachRemoteParties` |
| SS-05 | 夺取主权后旧政权法律继续控制领地。<br>**修复：**控制权变更同步实际辖区，税法、宵禁与禁运立即读取新控制方。 | `TestSpaceSocietyTerritorialControlChangesLawAndPerception` |
| SS-06 | 有限距离通信在缺少坐标时变成无限距离。<br>**修复：**有限距离通信和教学共用距离判断，不能因缺坐标变成无限距离。 | `TestProductionKnowledgeTeachingNeedsKnownChannelDistance`<br>`TestSpaceSocietyCommunicationRequiresVerifiedRange` |
| SS-07 | 人物与两种模型都收到远处实时活动效果的完整状态。<br>**修复：**两种感知入口只提供可见效果；他人的内部强度和维持来源不再泄露。 | `TestSpaceSocietyEffectAppearancePreservesPersonalCapabilities`<br>`TestSpaceSocietyEffectsExposeOnlyPerceivedAppearance` |
| SS-08 | 远距感知公共方法不检查观察者自身屏蔽。<br>**修复：**远感同时检查观察者自身和目标的室内屏蔽及穿透能力。 | `TestSpaceSocietyRemoteSenseChecksObserverEnclosureAtMaximumRange` |
| SS-09 | 宏观贸易直接绕过属地禁运。<br>**修复：**宏观交易和货运遵守相同属地禁运规则，并保留合法绕行。 | `TestFreightCapacitySharesEdgesAndSurvivesSnapshot`<br>`TestFreightMarketSharesCapacityAcrossCallsAndOtherDispatches`<br>`TestSnapshotJSONRoundTripPreservesAllFields`<br>`TestSpaceSocietyFreightHonorsEndpointAndTransitEmbargoes`<br>`TestSpaceSocietyFreightKeepsLegalBypassAroundEmbargo` |
| SS-10 | 宏观货运采用另一税率和收税主体。<br>**修复：**宏观市场按实际属地税率收税，通行税单列并进入真实财政账户。 | `TestSpaceSocietyFreightUsesPolityMarketTax` |
| SS-11 | 叛军并回平民群体后在途货物仍交给零人口旧群体。<br>**修复：**同地完全并群同步迁移已付款在途货权和农业资产，货物仍会真实到达。 | `TestSpaceSocietyAmnestyTransfersIncomingCargo` |

### 主线程交叉复核

| 原编号 | 原问题与修复后的行为 | 回归测试 |
|---|---|---|
| ROOT-01 | 资源槽耗尽后按默认值自动补满，零再生也生效。<br>**修复：**明确的零值表示耗尽，只有缺少键才初始化，保存恢复后不补满。 | `TestRootAuditExhaustedSlotStaysZero`<br>`TestRootAuditSustainedEffectDoesNotRefillExhaustedSlot` |
| ROOT-02 | 自动阈值转换绕过同一转换声明的材料、条件与工时。<br>**修复：**主动、自动和声明式转换共用前置条件、实际扣料及转换工时。 | `TestRootAuditAutomaticTransitionDoesNotReplaceBusyTask`<br>`TestRootAuditAutomaticTransitionUsesPrerequisitesCostsAndTime`<br>`TestRootAuditDeclarativeTransitionUsesItsOwnCostsAndDuration` |
| ROOT-03 | 同一动作的两份金币成本分开校验，合计可透支。<br>**修复：**同币种成本先合计，再按同一结果校验和扣款。 | `TestRootAuditCombinedCoinCosts` |
| ROOT-04 | 总量账本不检查持有人负库存，也不证明交付或物理单位守恒。<br>**修复：**先检查每个持有人非负，再比较总量；交付及材料预算由对应结算规则检查。 | `TestGrowthCareCandidatesRequireSeparateAssentAndPreserveLocalGoods`<br>`TestProductionExternalBudgetsPreserveExplicitZeroAndPositiveBoundaries`<br>`TestProductionExternalBudgetsRejectInvalidConsumptionDefinitions`<br>`TestProductionPackValidatesRightsMembersAndHoldings`<br>`TestRootAuditNegativeHoldingsCannotCancelAcrossOwners` |
| ROOT-05 | 同一禁枪世界法则能被声明式射击动作绕过。<br>**修复：**内置与声明式枪械动作共用禁枪法则，候选和延迟结算均遵守。 | `TestRootAuditFirearmLawCoversDeclarativeAndDelayedActions`<br>`TestWorldLawBlocksDeclarativeFirearmCandidate` |

## 新增检查、资源约束与行为收紧

这些约束来自原审计的可复现错误、当前执行路径和明确配置。各项原始依据与实现位置同时保存在相邻 JSON 文件。没有新增兜底或重试。没有新增网络超时、自动重试次数或请求数量上限。

| 检查或资源约束 | 依据 | 可能影响的原有操作与合法边界 |
|---|---|---|
| 生物亲本、成熟期和区域容量 | ECO-11 的无亲本即时补充；使用明确的资源与繁殖配置。 | 没有亲本或尚未成熟时不能采出新增个体；存活亲本繁殖及按期成熟已验证。默认成熟期从既有再生率推导，不声称是真实树龄。 |
| 真实水源、水系流量和洪水溢流 | ECO-12、13 的凭空饮水和脱离水系洪水。 | 无来源不能饮水；最后一单位、多个来源合用、上游取水和真实溢洪均已验证。 |
| 狩猎来源、概率、实际工作地点与救济覆盖 | ECO-01 至 06、09 的表示免疫、远程劳动、无限掉落和一份救济覆盖全群体。 | 不在现场或未实际工作者不能参与该劳动结算；零概率、零暴露、零物资和满额合法边界已验证。 |
| 仪式物理效果需要显式超自然规则 | ECO-15 中普通世界祈祷直接消除客观灾害。 | 普通仪式保留社会效果；声明超自然规则的仪式仍可按其配置改变物理环境。 |
| 田块面积、使用权、劳动和设施工位 | AG4、AG5、PROD-08、09 的无限播种、监禁劳动和无容量设施。 | 土地被占、无权使用、工位已满或劳力不足时不能开工；正好足额的批次仍能执行。容量由资源所有者统一负责。 |
| 照料双方接受、补给归属、收获权和分成 | AG9 中公会自动签约；AG1 中货权和交付不清。 | 公会成员不再被强制代劳；0‰ 和 1000‰ 分成、所有者离开后接受既有提议、保存恢复均已验证。 |
| 在途货物共同占用逐边运力 | AG1 与 SS-09、11 的真实交付要求。 | 市场和收获不能分别超用同一道路容量；已付款货物受阻会保留并明确报告，恢复路线后可继续正常调度。 |
| 持续饲养与缺料死亡 | AG8 中牲畜无需生存成本。 | 家畜不再无限暂停而免于饥渴；充足和短缺条件均有回归。每天一水两草、持续短缺七天的默认值是明确场景预算，不是兽医阈值。 |
| 质量与营养预算、肥料上限、工位与田地预算 | PROD-01 至 03、09 的无来源增产和无限产能。 | 技能、工具、公会和技术不再加出无材料的件数；肥料在声明预算内生效。默认每设施一工位，每农民及每作物批次1000平方米均是场景预算。 |
| 供电、运行投入与施工现场逐tick有效 | PROD-05、12、13 中断电、离场或耗尽资源仍持续受益。 | 断电或离场暂停有效工作，投入保留；正好够一次完整运行时可执行，恢复后继续。没有新增停工超时。 |
| 有限射程、通信距离、实际在场及屏蔽 | PROD-06、07 与 SS-01 至 03、06、08 的距离和空间绕过。 | 缺距离证据不能跨地点攻击或通信；同地、最大允许距离、恰好穿透屏蔽、合法路线和合法绕行均已验证。 |
| 调解知识、沟通与双方同额接受 | SS-04 的第三人强制扣款。 | 第三人只能提议；双方接受后才付钱结案。模型候选到实际付款的完整流程已验证。 |
| 实际辖区、属地税与禁运 | SS-03、05、09、10 的失地执法和宏观绕法。 | 失去辖区不能继续执法；合法当地执行与不受禁的绕行仍通过。税收总额、实收和截留分别闭合。 |
| 已约定抵押物、多债权人份额和实际整批预算 | E1 至 E5 的财产覆盖、空交付、负库存和超财政工资。 | 无抵押不能扣房；不足整批时不成交；库存或预算正好够时通过。保留既有份额上限与每日国库六分之一工资预算。 |
| 资源零值、转换前置条件及合计成本 | ROOT-01 至 03 的免费补满、免费转换和合计透支。 | 耗尽不能重填；忙碌人物不启动第二个自动转换；准确足额成本、完整工时及保存恢复均已验证。 |
| 持有人非负及外部资产引用检查 | ROOT-04 中负库存可被他人余额抵消；新增田产与成员需有真实身份。 | 负库存、重复身份和无效田产权利会明确报错；零库存、合法成员及合法权利通过。没有库存数量上限。 |
| 外部饲养、材料和生产参数 | 饲养间隔0会使当前循环无法推进，0供给会成为除数；负质量会破坏转换预算。 | 饲养间隔、存活期和实际消耗须为正并有来源；输入材料须有正质量。明确0工位、非食物0营养、无需人工的0照料工和非活体无限期成熟仍合法，没有新增上限。最小正数与各类合法零值均已验证。 |
| 统一枪械法则及结算时复核 | ROOT-05 的声明式入口和延迟结算绕过。 | 禁枪时不生成或执行射击候选；法律允许时仍可开工。普通通信不会仅因通道名称含firearm被当成攻击。 |

## 测试场景迁移

自定义职业与科幻配方补上了明确的材料单位和转换预算。旧“技能自动增加件数”断言改为检查材料允许的真实产量。跨区瞬间法院、切磋、镇压等旧必现事件改用同地正向与异地反向测试；司法集成场景配置实际在场人员。原有人数、病伤、财政、货物、守恒及保存往返断言继续执行。30天税收测试取消没有标定依据的“至少一千万”收入断言，改为要求真实征税与实收发生；逐项总额、实收、截留、国库和钱币守恒继续严格核对。初始田块处于长周期且已占满时，七天内不再要求无地新播。

## 范围

本轮修复原审计记录中的实现错误和缺失机制。测试证明这些配置和执行路径满足已声明的规则。质量、营养、田块、饲养、繁殖和环境参数仍是世界设计数据；通过总账并不能证明现实世界的公斤、热量或多年生态已经校准。

未提交或推送代码。保留任务开始时已有的未提交工作。

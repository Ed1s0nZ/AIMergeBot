# PR审计优化实施计划

输入：Confirmed R3；设计design.md；分支codex/audit-workbench-optimization。阶段F3，生产代码尚未修改。

| 切片 | 需求 | 文件/边界 | 验证与交付 |
| --- | --- | --- | --- |
| 1 上下文批量工具 | 001、002、012 | 新context_repository_batch.go及测试，agent_investigation.go，策略版本 | 固定SHA、编号行、未知ID、撤权、取消、范围及字节预算测试；回归上下文工具；文档新增工具；无DB迁移 |
| 2 PR调查与风险知识 | 003、004、006 | Agent提示、分组交接、独立知识模块 | 风险引入/修复及反证样例；预算与检查点回归；策略版本更新 |
| 3 PR因果链 | 005、006、012 | Finding/Investigation契约、验证、持久化及前端类型 | 逐边观察资格、BASE删除、跨仓库双方与未知边测试；历史JSON兼容 |
| 4 审查工作台 | 007、008 | Review事务/迁移、HTTP、独立React组件 | 并发首写、版本冲突、权限、草稿保存、键盘、加载错误与浏览器验证；记录旧调用方迁移 |
| 5 读取效率 | 009、012 | 状态读取HTTP、详情加载、Git清单缓存 | 无变化传输、变化恢复、撤权、提交隔离及有界资源测试；实测再报告效果 |
| 6 SARIF | 010 | 独立导出模块与授权路由 | 标准schema校验、BASE/HEAD来源、缺口、不确定边和访问控制 |
| 7 质量及完整验收 | 011、全部 | evaluation及验收报告、CHANGELOG | 新增语言和跨仓库PR对；独立真值；实际模型、成本、未完成及失败记录；逐REQ完成审计 |

每个切片开工前细化所需契约、字段及回滚；局部完成不标记目标完成。代码与实施说明按切片测试后提交推送；综合验收另有证据报告。用户README及docs/images不纳入本功能提交。

首切片检查：go test ./internal/platform -run Context；新增批量相关测试；完成后go test ./...。涉及UI再运行类型检查、构建与实际界面验证；涉及并发/事务运行race；不每次重复无关全量检查。

回滚：每切片可独立回退代码；迁移采用保留数据的增量方式，复核版本保护不能通过降级客户端静默绕过。保持旧报告可读；无自动合并、部署或发布。真实评测如缺外部条件明确报告缺口，不以unit test代替质量验证。

独立检视 AWO-REV-002（8e6bf31）：固定 context 读取擦除 ErrRepositoryUnavailable，违背既有执行器停止契约。局部修复仅保留安全 sentinel，不传播原错误文本；普通缺失文件保持可恢复。以真实 SDK 本地 HTTP 计数验证单组及分组首轮 context 失败，后续组停止及停止原因；不改变预算与语言范围。

### v36 计划状态收尾（P10 / F3）

依据真实恢复回归704/705：四项计划checked但仅record_hypothesis，状态仍investigating；源码reader与update门禁未失败。Confirmed R3 AC003/006/011，已有设计和调用契约允许此局部反馈迭代。Workflow Gate：允许，缺少状态反馈而非新产品决策；不涉及UI/API持久化迁移。Maintainability Gate：导航96行、职责单一；agent_investigation仅局部工具说明，保持现有校验，不广泛重构。

新增server-owned导航unresolved_ledger_count（纯计数、不插入模型claim/id），明确record永远创建investigating、必须显式update；supported/rejected相对实际假设而非是否报漏洞，兼容性结论不能靠标记rejected伪造反证。必要上下文未知保持investigating；四项checked不能自动收尾，不自动制造关系。工具说明同样明确创建状态；策略v36，历史不重写，预算不变。测试覆盖状态混合、隐私投影与实际SDK工具请求中的反馈；真实同集后续仅称回归。

### 复核引用失败安全分类（P10 / F3）

真实v36 701只有invalid_observation摘要，原始模型引用未存储，具体失效ID不可重构；不得猜测历史原因。Confirmed R3 AC006/011/012允许局部诊断改进。Workflow Gate允许：已有固定来源/隐私契约，无新增产品权限、UI或存储迁移；Maintainability Gate低风险：来源验证和复核agent局部返回类型替换，新独立helper单职责，不广泛重构。

保持原验证Error字符串及全部接受/拒绝条件，增加私有typed失败原因，运行记录code细分为未知引用、非复核阶段、非源工具、读取失败、输出损坏、快照不符、空源码；只输出有限枚举，不记录原引用ID/模型理由/源码/底层错误。复核仍unavailable，不自动删除坏引用、不重试、不增预算。测试覆盖各类别与实际SDK错误回执路径，历史不重写。无提示语义改变，策略v36保持。

### v37 记录错误显式纠正（P10 / F3）

输入Confirmed R3 AC006/011/012、首次801/804/805/806恢复缺口；已有design新增契约。Workflow Gate允许已有错误恢复的局部实现，不涉及UI/DB写权限；Maintainability Gate：新recording_corrections helper单职责，agent_investigation/verification仅注册/过滤局部连接，不广泛重构。新增helper、工具注册、只读过滤、策略；测试成功纠正、跨产物/源码/快照/逆序拒绝、批次原子性、历史保留/非证据、SDK及时刷新与预算/失败留存回归，race/vet；真实回归另冻结，旧首次样本不再称独立验收。源码失败/分页/语义极性不通过本工具自动解除，未验证事项仍保留。

### v38 合格纠正导航（P10 / F3）

提取recordingCorrectionKeyLocked，复用单对校验于resolver及新recording_correction_navigation.go纯投影；primaryRecordingProgress新增eligible_recording_corrections（最多4），提示显式处理，保持pending由resolver唯一删除。规范编号采用observation-正整数或group-正整数-observation-正整数；只限制系统导航候选，不改工具输入契约。新增导航边界/隐私/幂等状态测试及实际SDK请求断言；运行原resolver回归、导航、SDK、只读/压缩及race/vet，随后全量Go检查。更新recording-corrections、implementation、CHANGELOG。单切片可回退导航而保留v37 resolver，无DB迁移。先提交推送此设计计划，再改代码；完成后按pr-review技能进行独立新上下文检视。不自动合并或部署。

### v39 主仓库路径预导航（P10/F3）

先设计计划提交推送。实现helper调用tools.list page1，ctx/checkpoint/executor停止传播；agent初始user明确PR manifest非整个仓库与HEAD清单非source。复用正常计数及ID；有限primary inventory投影不含paths/modeltext，不新增source资格。测试首模型请求真实list回执/未修改源、拒绝列表作为证据、partial恢复、普通失败、执行器/取消/持久化失败0模型调用，以及组预算。现有受控SDK需按新增真实首导航ID更新，不改真值/门禁/预算。定向/race、全量/vet与独立边界review；后续802式回归检视是否实际查关联源及停止虚构单文件仓库，不追求全部completed。回滚单helper+初始连接即可，旧报告只读。

V39-001修复计划：grouped_agent child Progress wrapper串行化并记首error；child返回后立即停止，保存当前失败与后组未处理说明。独立原始反例转换为普通失败/ErrConflict回归，断言callback仅1次、SDK HTTP0次、原list trace保留、后组unprocessed和error identity；增加先成功后失败检查点覆盖。Workflow/Maintainability Gate：已有Confirmed R3持久化安全范围局部修复，单函数连接，无DB/UI迁移。修复后重新定向/race、全量/vet，独立复查绑定新SHA；dda3eda真实回归不得改称修复后执行。

### v40 状态对应动作导航（P10/F3）

Workflow Gate：Confirmed R3/design/source及真实802回执具备，缺口是记录动作未采用，不是新权限或产品决策；允许局部反馈迭代。Maintainability Gate：primary_progress_navigation104行、agent331行，新增纯action helper单职责，旧导航只接入；低风险narrow_fix，无广泛重构。生命周期分支codex/audit-quality-loop，先提交推送设计计划，再源/测试，再记录实际效应及独立边界检查。

实现有限action纯helper及固定说明；primaryRecordingProgress仅添一个内部system字段，用现有合法来源/待办投影选择，不改变记录门禁；核心prompt与record_hypothesis说明明确每次PR行为检查包括no-findings。测试各动作优先顺序、未知gap安全降级、完整状态但真实unknown仍保留、源/模型text不入system；真实SDK第一源读后第二请求提示record_changed_behavior，工具调用后按服务器状态刷新，末轮预算优先。原记录/计划/纠正/分组/停止测试、race、full/vet；真模型focus802原15/80/240，无重跑。真实效果失败保留，AER-001仍open直到原验收充分。

### v41 调查最终覆盖一致性（P10/F3）

Workflow Gate：Confirmed R3、v40原始回执与具体producer/consumer遗漏已具备；允许只读调查结果可靠性修复，无新产品/权限决策。Maintainability Gate：新单职责helper及agent331行一处连接，prRecordingGaps保留共用，低风险narrow_fix，不广泛重构。

先设计计划提交推送，再调查PR coverage/helper和agent正常结果连接；测试无finding已解析四项checked/support或reject调查缺链接时仍明确未完成、完整结构只表示字段齐全、inferred不能因unresolved_edges空消失；metadataOnly正例和错阶段/仓库/快照/重复/损坏/混源/变正文反例。实际SDK通过现有record/update及final[]覆盖；实际worker持久化status与报告无告警不当completed，原计划/finding metadata/分组/序列/SARIF回归、race/vet/full。独立边界review新SHA。真实旧802回执离线只验证新增projection，不称新模型/不重写原status；不盲重跑模型为取得completed。原Claim极性/跨项目关系仍需继续优化。

### v42 命题评估适配（P10/F3）

Workflow Gate：Confirmed R3 AC003/006/011/012及真实802失败证据具备；已有设计契约，允许，无新权限/产品决定。Maintainability Gate：agent_investigation约200行，导航单职责；低风险adapter_extraction，独立输入helper，原校验复用，无广泛重构。生命周期分支codex/audit-quality-loop；设计6e2d357先推送，本计划提交推送后实现。新增DTO/枚举映射和schema modifier；ledgerChange薄委托保存真实提交trace并限制新旧输入字节，原update保持。模型schema仅隐藏根status、保留plan.status；有限导航说明改新assessment，策略v42。测试枚举、矛盾、兼容、预算、真实源码资格、实际SDK schema与回执；race/full/vet；冻结SHA独立新上下文review，再原provider/预算802回归。失败与未完成保留，不把已知回归称盲测，不自动合并部署。回退适配注册及导航即可，历史结构保持。

### v42 剩余质量缺口取证（P9/F5准备）

上一轮为progress：输入契约源码、真实802及独立检视改变权威状态。Workflow Gate：Confirmed R3 AC002/003/005/011、冻结源码b19ff4677f475d15bd5ab7d04e0da7c27bdd5807及旧v37 803/805缺口齐备；允许仅评测，不改生产契约/权限/UI。Maintainability Gate：无代码改动，现有CLI只读审计固定fixture；不新增prompt迭代。依原独立首次corpus，选803跨项目正例和805兼容负例各一次原deepseek-chat/15steps80tools240回归；冻结当前源码和原真值，分别检查双方源码、主PR锚点、source-linked关系、实际Claim极性及未知边；失败/incomplete/成本完整保留，不改真值/预算，不盲重跑、不执行样本。首次已知样本称回归，不能外推生产准确率；确认需求不要求所有样本completed。本轮结果将决定具体必要改进或更新AC覆盖，不以只检查工具存在关闭AER-001。先提交推送本计划，再执行。

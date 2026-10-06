# PR审计优化实施计划

输入：Confirmed R3；设计design.md；当前实施分支codex/audit-quality-loop。以下保留原切片计划；既有实现与证据见implementation.md及verification-matrix.md。当前v45契约基础已完成，执行接入与真实质量验证仍进行中。

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

### v43 计划身份反馈（P10/F3）

Workflow Gate：Confirmed R3 AC003/006/011/012、真实805原始冲突及design具备，允许局部错误恢复；无新权限/UI决定。Maintainability Gate：investigation_plan约140行，单计划校验职责，新增单职责错误helper；低风险narrow_fix，不需先广泛重构。分支codex/audit-quality-loop，设计22ce281已推送；本计划先提交推送再实现。保留旧校验顺序与拒绝规则，收集identity冲突，返回有界原tuple JSON，异常数据safe fallback；schema question复制说明。测试多冲突、遗漏、更改kind、非法/超限旧数据、注入字符串仅tool输出、门禁不松、ledger未变、合法恢复与显式resolver原历史，实际SDK错误及source flag/ID校验；race/full/vet、冻结SHA独立review（pr-review re-review-gates要求契约/恢复新上下文）。不重复803/805真实请求直到新源码与边界验证完成；真实极性仍open。不自动合并部署。

### v44 空/未知ID反馈（P10/F3）

Workflow Gate：Confirmed R3 AC003/006/011/012、v43真实空ID及F2设计齐备，允许局部恢复，不改权限/UI契约。Maintainability Gate：agent_investigation约200行连接1处、types只字段schema描述、新helper单职责；低风险narrow_fix，无广泛重构。分支codex/audit-quality-loop；design80c1f0d先推送，本计划先提交推送。新增未知IDerror helper仅投影当前ledger键，有限排序/JSON编码/80bytes/UTF8/NUL/30条/8000bytes上限fallback，保留全部旧验证和拒绝。字段说明同record返回ID；测试空/未知/多记录不推断、异常datafallback/注入仅tool、source不能提升、wrongID不写ledger、复制ID后仍需计划来源门禁，SDK未知ID→复制明确ID→合法更新→显式resolver保持history。race/full/vet，冻结源码独立新上下文（pr-review恢复契约要求），不盲重跑模型。AER-001极性仍open，无合并部署。

### v45 复核预算基础切片（P10/F3）

完整目标与新契约见claim-verification-design.md（7bf1616）；本切片只是其必要基础，命题复核/UI/实际质量未实现，不代表目标完成。Workflow Gate：Confirmed R3/F2设计/现有verification实现及实际极性缺口齐备；允许实现共享预算基础，无新权限/产品决定。Maintainability Gate：verification_agent195行包含模型初始化/调度/结果校验，私有helper抽取预算降低耦合；medium风险，zero_behavior_refactor，旧wrapper保留模型初始化后才创建pool的顺序，nil pool兼容，无跨UI变更。策略v44保持，未来feature接入后才v45。

抽取verification_budget.go的phase ctx/cancel、60秒与deadline−10秒、40工具余量、单项min10及consume；现有verifyFindings薄wrapper调用withBudget(nil)，原模型选择、callback、source资格、单项15秒/graph8、排序/缺口/persistence保持。新pool可复用但本切片尚无第二consumer，不能声称已实现共享命题复核。测试余量/耗尽/overcount/取消/短deadline，以及同一pool跨两次finding调用不可额外消费；旧finding/context/checkpoint/model usage/停止/race/full/vet。独立预算边界fresh-context review按pr-review公共信任预算改动要求；源码freeze，未运行样本/真实API，不用实现foundation代替原AC。单helper可回退，后续继续完成契约/parser/runner/UI/SARIF/实际评测。先计划提交推送再代码。


v45当前切片状态：共享预算基础3f493d3、server-owned/parser/provenance及CVR-001修复814c60a已完成切片验证。下一步执行器接入：resolved调查稳定顺序、finding优先同pool、fresh prompt不给首审判断/理由、实际模型与压缩usage正确归入复核价目、取消/检查点/故障停止、未完成coverage、worker/SARIF持久化；随后UI和当前源码真实模型回归。不得以契约切片批准代替整体完成或自动合并。


### v45 工作台消费接入 Gate（执行前）

Workflow Gate：P10/F3；Confirmed R3 AC006/007/010/012及claim-verification-design.md已有五类复核状态/历史空态/360px/键盘/source导航契约。Go optional ClaimVerification已固定，当前执行源码986b336；允许第4切片，无新产品选择，保持已有coverage组件风格。待验证的是实际界面与真实原模型效果，不是API字段设计。

Maintainability Gate：run-detail现有页面约600行，API类型单一契约模块；只增加独立ClaimVerificationPanel委派及初审命题文案，不把逻辑堆进页面。risk medium（frontend/typed契约/状态/长ID导航），无需广泛重构，允许adapter消费；ObservationLinks复用只调整长ID换行。新组件接收server-owned optional review与调查/任务状态；旧报告缺失显示历史/未执行，进行中/取消/分歧/未知/关闭分开。

改动文件：frontend/src/claim-verification.tsx、api.ts、run-detail.tsx、settings.tsx、model-usage.tsx、observation-links.css；config.example.yaml及README更新共享范围说明。必要检查：tsc/build/embedded build、360px及键盘按钮直达fresh源码实际浏览器检查；现有类型/构建无额外测试框架。构建产物按仓库既有策略提交，不触碰原工作区用户README修改。分支codex/audit-quality-loop，前端源码与证据独立冻结检视，不继承后端986b336报告为整体批准。

### v45 EXR-001 取消检查点闭环（执行前 Gate）

Workflow Gate：P10/F3，Confirmed R3 AC006/010/012、命题复核设计和986b336独立检视EXR-001已具备。全量Go在986b336通过（platform194.834s），但真实Store取消fence反例证明完成wrapper太晚；允许局部修复，不改取消权限、租约、旧报告或真值。

Maintainability Gate：audit_claim_completion约36行、public入口薄wrapper；medium（检查点/分组/取消边界），narrow_fix，无广泛重构。新增纯检查点投影与progress decorator，public Audit/AuditGroups复制auditor配置后接入，覆盖primary及group/supplemental所有原progress路径。投影只复制investigations和coverage切片后给resolved且nil review添加unavailable或disabled；原ledger、未完成调查、合法已完成review、原Claim/status/evidence不变。不得把placeholder写回实际主审账本或遗留到成功final结果；无需在每种取消SQL中重写结果，也不回填历史。

测试实际Store checkpoint→Cancel→late checkpoint ErrConflict，确认取消结果带显式未完成review且SARIF存在该项/执行非成功；恢复也保留；disabled无unknown verdict、in-progress不伪装review、原切片不变、合法review不覆盖、group已命名ID保留。公共SDK/worker及group/recovery/cancel定向race、full/vet/diffcheck；冻结修复SHA后EXR-001独立re-review。随后继续原已计划UI/真实原模型回归，AER-001保持open。分支codex/audit-quality-loop，无自动合并部署。

UI验证诱发修复：360px实际RunDetail已显示五态和命题极性；Return source按钮能打开正确固定关联源码，route保持#/runs/45、scrollWidth=360，但焦点停留原复核按钮，Tab无法顺着目标证据继续。AC007键盘source导航需要目标focus，因此ObservationLinks复用FindingWorkbench的原生focus模式：开details/滚动后focus其summary（preventScroll），不改变路由、原记录或权限；无广泛UI重构。下次实际Return+Tab复证后记录。

### v45 原模型已知兼容负例回归准备

P9/F5；Confirmed R3 AC002/003/005/006/011及原805错误极性/空ID回执具备，允许验证已经实现的v44恢复/v45复核，不更改真值或执行预算。后端dfc6340 full/CI/scoped APPROVE已完成；当前UI冻结提交后在该精确HEAD用原config、原deepseek-chat/temperature0.1/15steps80tools240s，原corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b，805只运行一次。外部新唯一目录记录CLI revision/策略、固定BASE/HEAD及授权下游、原始checkpoint/trace/失败/未完成/usage/token成本和manifest，不展示credentials、不执行样本/PoC。不把known regression当blind准确率、不追求全部completed、不提高预算或盲重跑。观察实际初审命题是否resolved、极性、fresh独立复核/共享预算及来源关系；若仍不满足据具体证据继续原范围，不关闭AER-001。

### v45 成对真实复核覆盖准备（执行前）

805一次真实回归在53331c8完成，fresh独立true确实识别primary rejected的实际命题错误，但只有兼容负例，不能外推guard负例或有条件漏洞finding优先共享预算。P9/F5，Confirmed R3 AC001/002/004/005/006/011及原802/803独立首次corpus/已有回归具备；允许按原deepseek-chat/temperature0.1/15steps80tools240s各跑一次known regression（802保护恢复负例、803 TS→PHP有条件风险正例）。不新增样本训练/改真值/提高预算，不盲重跑，不要求两项completed。分别检查主PR双侧、授权固定下游/关联、命题极性、finding锚点、fresh复核/预算优先、真实未知/失败、阶段完整usage。新唯一外部目录冻结文档CLI head及与53331c8/dfc6340源码关系并保护archive，之后逐项R3审计。AER-001保持open，无自动合并部署。

### v46 PR上下文原子记录适配（执行前）

P10/F3；design de20cfc先提交推送，Confirmed R3和v45真实遗漏齐备，Workflow允许，Maintainability medium/adapter_extraction。新增pr_context_recording.go typed DTO和锁内复制/合并/验证/保存；agent_investigation仅注册；primary_recording_action定向三种导航；recording_corrections独立family匹配；runs_store策略46。测试真实Go固定源双侧及context、错误来源/side/claim/size/无context/原子失败、保留计划/状态/字段及不自动填ID、同family显式纠正/历史、实际SDK schema与record→补context→正常final保存；原调查/PRcoverage/finding/correction/verification/group/cancel/recovery定向race、vet/full，精确CI。冻结SHA后按pr-review公共契约/恢复独立fresh-context检视。后续原预算真实802/803各一次用于适配效果，先证明工程边界后调用；不增加预算/执行样本/隐藏失败/自动main合并部署。文档/CHANGELOG随实现及验证提交；新family不跨artifact解除旧错误；rollback保持旧工具路径。

### v47 原决策预算内收尾适配（执行前）

P10/F3，design13fd1dd先推送；Confirmed R3和真实v46早停足够，Workflow允许，Maintainability medium/adapter_extraction。新增primary_finalization_model.go（WithTools共享counter/callback委托/硬cap/一次finalization/源状态门禁/早draft合法candidate经原validator保留）；primary_round_budget提示抽纯helper复用，agent.go一处连接，compression/源/Store不改；policy47，文档CHANGELOG。实际SDK覆盖early final→补记录→final与失败/ctxcancel/最后工具阻断、WithTools重新绑定同cap、完整/nil源/无余量/重复final不续、usage无双callbacks/原token/compression/group/取消回归；race/full/vet/精确CI和冻结独立公共预算/信任边界检视后原802/803各一次。保留当前error/recovery/gap；禁止自动source/cited/status/summary同步、扩大15/80/240或复核pool，无部署合并。回退adapter连接和policy，旧record工具仍用。

### v48 独立来源要求与子断言覆盖（执行前）

P10/F3，设计734acf2先推送；Confirmed R3/真实803必要context漏读及full/未核实载荷矛盾具备，Workflow允许，Maintainability narrow low-medium。claim_verification_sources.go提取排序去重context ID helper供原门禁和模型User要求共用；claim_verification_agent.go内部payload显式source_requirements，固定prompt明确每个context fresh证据与必要caller/contract；agent.go与verification_agent.go固定条件/具体子断言提示，不提高预算、不生成源、不新增public字段或按语言/关键字判断。policy48、相关SDK与来源测试、文档CHANGELOG；race/full/vet/CI和冻结独立trust/modelinput检视后原803一次known regression。保留原记录和失败/unknown，真实字段仍无法保证语义遵从时不关闭AER-001。无自动merge/deploy，rollback旧payload/prompt及policy不降低原sourcegate。

### v49 关系schema/字段诊断适配（执行前）

P10/F3；design24d3618已先推送，Confirmed R3与v48重复空to失败输入足够，Workflow允许，Maintainability low-medium narrow。pr_investigation.go共享DTO标签声明原非空/长度/枚举/source数上限，validation逐字段有界错误/index/未知edge处理说明，不回显untrusted值；agent_investigation仅工具说明，policy49。实际SDK nested schema和失败/纠正路径、字段negative/正例/atomic/无secret echo、v48实际失败离线请求反馈和显式修正产物；原PR/source/recording/history/取消/finding/收尾回归race/full/vet/精确CI。冻结公共工具schema/反馈边界fresh-context检视后原803一次，原预算/真值/源不变，不盲重跑或以契约测试代替真实采用。文档CHANGELOG随实现/实际效果更新，失败保留；无自动merge/deploy。

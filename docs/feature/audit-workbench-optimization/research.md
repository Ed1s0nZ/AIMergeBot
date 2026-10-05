# 成熟代码审计与Agent调查实践调研

> 2026-10-06定位修正：本项目为PR增量审计。入口侦察、知识加载、跨仓库搜索和链路核查均由PR改动驱动，结论必须解释BASE/HEAD变化与风险的因果关系。全仓库取证不等于默认全项目扫描；最新需求为R3。

日期：2026-10-06；本地应用基线cbfc430，当前分支接入文档65d7623。

## 结论与范围

### PR审计补充依据（2026-10-06）

- [reviewdog过滤模式](https://github.com/reviewdog/reviewdog#filter-mode)区分变更行、diff上下文、变更文件与不过滤，并说明不同评论API支持范围不同。可借鉴语言无关诊断接入与评论位置回退；但行位置过滤不证明风险由改动引入。AIMergeBot应分别核查PR因果归属和可评论位置，保留删除BASE锚点，并允许将跨文件证据放在报告中，而非强行绑定无关变更行。
- [Google代码审查实践](https://google.github.io/eng-practices/review/reviewer/looking-for.html)要求结合文件及系统上下文检查功能、复杂度、并发和有效测试，并区分风格偏好。可借鉴按改动类型选择检查问题：接口变化核查调用方，状态变化核查不变量，并发变化核查竞态，测试变化核查断言有效性。默认自动评论聚焦有因果证据的可执行问题，不能把个人风格偏好包装为漏洞。

这些来源与既有PR-Agent、SARIF及论文研究共同支持PR增量审计定位。DeepAudit仅为参考之一；不因某项目采用RAG或多Agent就将其架构作为本项目目标。以上未运行外部工具，也未测量实际降噪效果。

用户明确要求保持不限制语言、可跨项目的Agent审计主旨。最值得借鉴的是有界的上下文检索、入口到危险操作的证据链、反证闭环、分层质量评测与便于人工核查的工作台。语言分析器不是审计准入条件；编译、执行仓库代码及运行模型生成脚本不进入默认路径。

本报告区分来源事实、本地代码事实和建议。未安装或运行外部分析引擎，未调用真实模型，未测量集成后的效果。论文成绩不能当作AIMergeBot成绩，来源成熟度不以star数判断。下面的研究原型不等同于成熟生产产品。

## 论文证据

| 来源与阅读版本 | 方法及证据 | 可借鉴内容 | 应用限制 |
| --- | --- | --- | --- |
| [IRIS，2405.17238v3](https://arxiv.org/html/2405.17238v3)，§3、§5、§7、附录C.7 | 模型推断污点规格，CodeQL寻找路径，再做上下文分诊；错误分析包括副作用、缺失库使用关系与无法用简单污点描述的问题。 | 将候选、源码事实、路径假设和反证分别记录；工具分析与模型解释相互校验。 | Java、四类风险，不能外推所有语言；模型过滤仍可能遗漏真实风险。不能把“CodeQL查询库开源”等同于CLI私有仓库使用无限制。 |
| [LLMDFA，2402.10754v1](https://arxiv.org/html/2402.10754v1)，§3与§4.5；[更新版摘要](https://arxiv.org/abs/2402.10754) | 将分析拆成源/汇提取、函数数据流摘要、路径可行性检查。初版与更新版标题和评测范围有差异。 | 把大调查分解成小的可核查事实；保护条件单独检验。 | 不照搬自动生成并执行脚本；默认仍只读、不编译。版本间数值不混用。 |
| [JitVul，2503.03586v1](https://arxiv.org/html/2503.03586v1)，§4.2、§5.1、§5.4 | 成对评测漏洞版与修复版，比较F1和pairwise accuracy；ReAct与普通模型在不同指标上表现不一致。 | 同时测正例与近似负例，避免“两个版本都报风险”获得虚高召回；对检索/预算做消融。 | 预选函数与成对分类不能充分证明自主全仓库定位能力；添加上下文不保证效果变好。 |
| [PrimeVul，2403.18624v2](https://arxiv.org/html/2403.18624v2)，§IV | 时间拆分、重复样本控制、成对行为分类及低误报约束。 | 按项目、时间与修复对隔离调试/验收样本；记录双报、双漏与反转。 | C/C++函数数据集不覆盖多语言、跨项目或业务逻辑。公开CVE还可能存在模型训练污染。 |
| [CORRECT，2504.13474v1](https://arxiv.org/html/2504.13474v1) | 使用丰富上下文，评估预测和理由；论文采用模型裁判。 | 结论对也需要检查理由是否与保护条件、数据路径一致。 | 模型裁判不是独立真值；本项目需人工复核裁判分歧，不能自动以同一模型自证质量。 |
| [VulnGym，2608.02001v1](https://arxiv.org/html/2608.02001v1)，数据构建、Evaluation Framework与Metrics | 区分自主端到端检测、入口定位、关键操作定位及路径构建；提供行级证据和人工核查流程。 | 把“找不到代码”“连错链路”“忽略防护”“判断错后果”拆开评估；有提示诊断与无提示验收分开。 | 新近预印本，尚未在本机复现；不能将单仓库数据集直接当跨仓库证明，也不能假定已排除训练污染。 |

## 开源项目与成熟工具

| 项目/一手来源 | 实际能力或方法 | 对本项目的建议 | 集成约束 |
| --- | --- | --- | --- |
| [CodeQL路径查询](https://codeql.github.com/docs/writing-codeql-queries/creating-path-queries/) | 用source、sink及连接路径解释结果。 | 链路中每个节点保留源码位置与来源，缺边显式标注。 | 只借鉴结果结构；[CLI授权](https://docs.github.com/en/code-security/concepts/code-scanning/codeql/codeql-cli)与MIT查询库不是同一许可。 |
| [Joern CPG](https://docs.joern.io/code-property-graph/)、[数据流步骤](https://docs.joern.io/cpgql/data-flow-steps/) | AST、控制流与数据依赖组织成图，支持数据流遍历。 | 调查账本可以组织为事实与边，避免只保留自由文本结论。 | CPG有语言前端边界；跨文件静态图不能自动证明跨服务请求链。 |
| [Semgrep污点分析](https://docs.semgrep.dev/writing-rules/data-flow/taint-mode/overview) | 明确source、sink、sanitizer、传播和trace。文档将跨函数/跨文件分析标为Pro。 | 每类风险调查要求查找保护条件和反证；规则输出只作为候选。 | 社区版、Pro和规则许可分开判断；不能承诺免费开源跨文件污点能力。 |
| [PR-Agent差异处理](https://github.com/The-PR-Agent/pr-agent/blob/main/pr_agent/algo/pr_processing.py)、[审查入口](https://github.com/The-PR-Agent/pr-agent/blob/main/pr_agent/tools/pr_reviewer.py) | 有界diff上下文、分块与增量审查入口。 | 明确未纳入范围、版本与预算；保持补审与完整审计区别。 | 源码观察不是安全准确率证明，不能照搬自动发布或修复建议。 |
| [Aider repository map](https://aider.chat/docs/repomap.html) | 用文件与依赖关系排序，在token预算内提供相关仓库地图。 | 增加便于导航的固定仓库概览和相关文件候选。 | AST支持有边界；无解析器时保留目录/文本路径，并明确词法候选不是真实调用关系。 |
| [DefectDojo去重](https://docs.defectdojo.com/triage_findings/finding_deduplication/about_deduplication/) | 区分工具ID、字段哈希、重复记录和原始发现。 | 保留来源与观察，合并展示时不丢原证据；人工确认关联。 | 不借用自动继承误报或修复状态；不同HEAD的保护条件可能变化。 |
| [OSV-Scanner](https://github.com/google/osv-scanner)、[官方说明](https://google.github.io/osv-scanner/) | 依赖清单与漏洞数据库关联。 | 后续可为锁文件变更提供确定性的版本候选。 | 默认不向新增服务发送私有依赖；版本匹配不证明可达和可利用。 |
| [Trivy](https://github.com/aquasecurity/trivy) | 覆盖漏洞、错误配置、秘密与SBOM等扫描。 | 后续可补充CI/容器/配置变更专项线索。 | 工具命中仍需本次变更和源码条件核查，不能自动晋级supported。 |
| [Dependency-Track](https://github.com/DependencyTrack/dependency-track) | 围绕组件与SBOM管理供应链风险。 | 依赖风险与代码风险分别展示，再关联实际使用证据。 | 不引入一个独立平台作为核心审计必需依赖。 |
| [OSS-Fuzz-gen](https://github.com/google/oss-fuzz-gen) | 生成fuzz目标，以构建、崩溃和覆盖衡量结果。 | 未来单独设计隔离验证，分开记录模型猜测与运行证据。 | 默认只读约束下不执行；这是后续能力，未做本机验证。 |
| [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/os/sarif-v2.1.0-os.html) | 标准化规则、位置、codeFlows及版本化指纹。 | 加入有权限检查的结果导出，保留固定版本及不完整覆盖说明。 | codeFlows只能表达已保存且核验的链路，不能从时序图补造真实调用边。 |

### 检索时仓库快照

2026-10-06通过公开GitHub API读取默认分支HEAD和license元数据。用于定位阅读版本，不是依赖锁定或许可证法律审查；未证明本地安装可运行。

| 仓库 | 默认分支SHA | API许可标识 |
| --- | --- | --- |
| github/codeql | 5478a913a4e243d3c2742bfac0fade7ebaf54a16 | MIT，查询仓库 |
| joernio/joern | 7b8d928c1b96dffa08505830912063eddfd06201 | Apache-2.0 |
| semgrep/semgrep | bcaca11e919e6d4dc94992865f77adcfb1216131 | LGPL-2.1 |
| The-PR-Agent/pr-agent | 906860201fe8da6b09405f85f176b15bab3cfb69 | MIT |
| DefectDojo/django-DefectDojo | 258344266af19de33d721e511f2fc34834f67bdb | BSD-3-Clause |
| google/osv-scanner | 1b86129be1f0285c403616cb362ac188de676c6e | Apache-2.0 |
| aquasecurity/trivy | 8f815546c7b57dc11229b0098463dff0cfc9ed2c | Apache-2.0 |
| DependencyTrack/dependency-track | f6be6a1a1fdfad029e886b4702a0fe7570f750c5 | Apache-2.0 |
| google/oss-fuzz-gen | c0982c5d40a7e93ce70fd319705804b9a29954d0 | Apache-2.0 |

IRIS与PrimeVul元数据API读取失败，未获得固定SHA；正文通过作者论文和公开仓库页面核实。不将未核实项补成成功。

## 本地能力与缺口对照

| 本地事实 | 证据文件 | 缺口与建议 |
| --- | --- | --- |
| 16个主仓库工具，授权上下文时增加4个工具 | internal/platform/agent_investigation.go:register | 现有批量读取仅主仓库；关联搜索逐仓库逐query。新增统一跨仓库批量检索/读取，减少模型往返但保持每来源证据。README的14个工具描述落后于当前注册实现。 |
| 关联仓库固定SHA，每次通过contextReader授权 | internal/platform/context_repository_tools.go | 批量操作必须沿用授权，每项返回仓库、版本、失败与游标，不允许任意URL。 |
| 调查账本含claim、evidence、counterevidence、状态和观察引用 | internal/platform/agent_investigation.go；types.go | 尚无结构化入口、危险操作、保护条件及跨仓库连接事实。可增加可核查风险链路，推断边独立显示。 |
| 发现校验主仓库变更锚点，独立复核新上下文 | finding_registry.go；finding_verification.go | 精确片段匹配只是来源校验。UI需要分别显示源码来源、模型支持、反证和未验证运行条件。 |
| 分组按目录、文件数、字节数，汇总不自动升级发现 | audit_groups.go；grouped_agent.go | 先做影响面导航与跨组未解决问题清单，再用评测判断关系导向分组是否收益更高。 |
| 分页连续前缀检查，未完成工具有覆盖限制 | pagination_coverage.go；agent_explore.go | 批量工具需要逐项保留连续分页状态，不能一项成功使整批成为完整。 |
| 11例自建语料，有固定Git历史入口；历史真实模型评测为v12 | internal/evaluation；evaluation/README.md；evaluation-round4.md | 增加多语言、跨项目、近似负例、入口/关键操作/链路标注。旧结果不能证明v14；评测README首次连接失败描述需补充后续轮次。 |
| 详情有证据、时序图、覆盖、用量、补审与历史关联 | frontend/src/run-detail.tsx与相关组件 | 提供发现导航/筛选、集中证据入口、结构化覆盖与调查链路；保持移动端和权限状态。 |
| 两秒全量轮询，复核表单初始化后不跟随远端决定 | run-detail.tsx；routes.go；runs_store.go | 加入复核冲突保护与草稿恢复，再优化状态更新与轨迹按需读取。 |

## 建议优化包与验收证据

以下是研究建议，不是已确认需求，也不是技术设计。主旨优先级来自用户，不预设模型、外部供应商或生产预算。

1. 通用跨项目工具：在已授权固定仓库内批量搜索、批量读取、读取锚点附近上下文；所有语言走同一路径。验收包括主仓库BASE/HEAD、关联仓库固定SHA、逐项失败、未读分页、权限撤销和源码来源。
2. 风险调查链路：入口、关键操作、保护条件、跨服务契约与关系假设分开保存；每项关联源码观察。验收包括同名但无调用关系的负例、消息字段不一致、上游授权仍有效与跨组调查。
3. 工作台核查体验：发现导航、严重度/复核/支持状态筛选、证据集中查看、跨仓库来源和覆盖缺口展示；修复复核冲突与草稿同步。验收需浏览器操作、窄屏、键盘和权限错误状态，不能只以tsc证明。
4. 互操作：SARIF导出与固定版本元数据；只导出当前有权限的已保存事实，不上传外部平台。验收格式、路径、BASE删除锚点、跨仓库来源及不完整状态。
5. 质量评测：扩充语言和框架独立语料，增加跨仓库正负对照，支持人工机制判定与成对统计。验收将基础设施失败、覆盖不足、定位错误与机制判断错误分开；评测标签不能进入模型输入。
6. 效率与恢复：固定对象清单缓存、轻量更新、增量轨迹和投影写入分切片实施；模型发送前持久化及旧owner fence保持。用命令/API数量、写入字节、P95及故障恢复证明收益，不先承诺提速百分比。

可选后续：授权的专项扫描结果适配、项目业务不变量、隔离运行验证、多供应商关联仓库。是否需要它们由质量基线和团队场景决定；不能以这些未实现能力宣称本轮已完成。

## 界面核查边界

阅读现有docs/images/audit-detail.jpg：摘要、覆盖说明、补审和用量纵向排列。该截图是合成fixture历史图，不是当前浏览器操作验证。建议把风险与待核查缺口放到可快速导航的位置，保留原全文和证据。未创建新UI、未改动用户README或截图。

## 当前交付状态

已完成一手资料阅读与本地差距核对；生产代码尚未修改。需求快照需要按用户“不限制语言、跨项目”的纠正扩展。设计、实现、回归、真实质量评测和发布均未完成，目标保持active。

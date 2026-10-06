# 实施与验证记录

## 切片1：上下文仓库批量读取

2026-10-06，已实现Agent工具read_repository_files(repository_id, files)。同一授权固定仓库一次读取1–8个范围，整体输出最多16000字节，保留编号源码、more和remaining。失败与超限整批不返回部分源码；读取前后权限核查；取消即使缓存命中也不返回源码。一次批量请求消费一次工具预算和观察，复用已有检查点与来源资格机制。主PR变更锚点要求保持。

新增context_repository_batch.go及测试；注册与来源资格增加工具名；策略版本v15，防止旧运行自动采用新工具契约。无需DB迁移。未修改用户README及docs/images。

验证：go test ./internal/platform -run Context -count=1通过（15.089s）；go test ./...通过（platform 42.750s）；新增覆盖状态/输出预算/取消测试后go test ./internal/platform -run TestContextBatch -count=1通过（10.848s）；git diff --check通过。没有真实模型质量对照，不能声称误报或召回改善。未修改UI，本切片无需界面验证。

代码复核：权限复用invokeContext前后检查；批量不循环调用外层工具、不产生未持久化子ID；跨仓库来源通过RepositoryID映射固定SHA；任何子项失败清空聚合输出；部分范围保留覆盖标记且继续读取后续请求项。该来源资格只证明取证，不证明调用关系或运行可利用性。

剩余：PR入口与风险知识、因果链、工作台与复核冲突、轻量更新/缓存、SARIF及多语言跨仓库实际质量评测均未完成。整体目标继续，首切片完成不代表REQ-002整体验收完成。

## 切片2当前实现：按风险加载反证问题

get_risk_checklist支持五类固定风险键；知识由本项目独立编写，使用正常工具预算和检查点，但evidence_eligible=false，不能链接为源码观察。主审提示要求从PR行为差异定位入口/契约、记录带来源事实及未知边；独立复核强调BASE/HEAD因果与历史缺陷排除。策略v16，历史记录仍可读。此增量尚不是完成的结构化入口侦察或交接能力，也未验证检测质量改善。

测试发现并修正复核阶段识别与旧工具数量断言的兼容问题；新增知识非证据、取消、预算及新上下文可用性测试。高负载时独立Git fixture一次超时，单独复跑通过（6.580s）；不提高测试时限掩盖失败。最终修正后go test ./...通过（platform 43.229s）；git diff --check通过。早期全量失败包含旧固定工具数量断言和已修正复核提示识别，保留原因，不解释为质量评测。

## 切片3A：结构化PR调查上下文与展示

Investigation.pr_context保存变更概述、BASE/HEAD行为、带观察ID的入口/保护陈述及未核实关系；服务端限制字段大小、观察来源及调查关联。非法更新不覆盖原调查，知识观察不能成为事实。Finding.pr_context仅从关联已验证ledger深拷贝，忽略模型自填内容；接受后的发现不随后续调查静默变化。历史空值保持兼容，策略v17。

前端PRInvestigationPanel复用于发现及调查记录，空值显示未记录，区分来源陈述与未知边，可通过既有证据按钮展开工具轨迹。React文本渲染，不执行模型HTML。Go全量测试通过（platform 43.488s），PRContext定向测试通过（1.221s），npm run typecheck与npm run build通过，git diff --check通过。嵌入前端资源与源码一起更新。

本地Vite隔离样例通过浏览器验证：Enter展开，BASE/HEAD与入口/防护/未知关系可见，点击观察按钮展开对应轨迹；历史空态显示正确。截图/tmp/aimangebot-pr-impact-proof.png为界面样例，不是实际审计结果。临时预览文件及服务已清理。尚未完成窄屏验证或真实模型填充率评测，也没有完整逐边风险链与跨组专用交接；不能标记全部REQ-003/005完成。

## 切片4：复核版本保护与草稿恢复

platform_reviews新增revision，历史记录迁移初始1，迁移幂等。Store和HTTP写入必须携带expected_revision：0表示无记录，正数表示读到的版本。条件插入/更新影响行数0回滚为冲突，不写历史、事件或评论任务；400缺失版本，409陈旧版本，权限先检查。前端未编辑时同步新决定；草稿冲突保留并阻止提交，用户明确加载最新或保留草稿采用新版本。提交时禁用编辑。测试fixture现显式读取版本，不提供生产绕过路径。

验证：Go全量通过（platform 53.825s）；复核及HTTP隔离定向race通过（5.948s）；定向版本测试通过；前端类型检查和最终build通过。测试覆盖首次并发、陈旧更新、评论generation不变、缺失条件、旧表迁移/历史保留及API400/409/204。早期全量失败为旧HTTP fixture未传版本，已迁移后重跑。

本地真实组件浏览器样例验证草稿保留、禁用提交、保留草稿恢复、加载最新及未编辑表单同步。截图/tmp/aimangebot-review-conflict-proof.png为受控样例，非生产操作；预览文件和服务已清理。未进行真实多人账号线上E2E，未部署。窄屏和网络错误的完整验证仍留在最终QA。

## 切片5A：固定Git文件清单缓存

GitRepository.paths复用每实例Directory+完整SHA清单，最多2项、估算8MiB；串行合并并发加载，失败不缓存，FIFO淘汰，返回切片复制。取消在命中前仍检查；上下文工具的权限检查保持，未缓存授权。超限拒绝完整请求，不返回截断清单。额外内存为每GitRepository最高估算8MiB缓存，返回切片及加载瞬间有额外临时开销，不能把该上限解释为整个审计内存上限。

检查门槛：原git_repository.go小于800行，新增缓存放独立模块；无UI/API/schema变更，不改变工具契约与策略。验证：定向race（GitPathCache与ContextBatch）通过8.221s；超大清单测试通过0.910s；Go全量通过（platform 44.472s）；git diff --check通过。实测8个并发同SHA请求只执行1次Git ls-tree，BASE另执行1次；目录变化、无效对象、取消和返回值修改均覆盖。此结果证明减少清单查询，未测端到端耗时。

仍需轻量任务状态轮询、发现筛选、SARIF、完整风险链与交接、多语言跨仓库质量评测及最终QA，整体目标保持未完成。

## 切片5B：授权轻量版本轮询

新增GET /runs/:id/status，只有版本、状态与队列等待，不读取完整result/trace。详情返回读取内容前取得的detail_version。项目级版本由同事务触发器记录报告、复核、评论及历史等变化，权限/项目配置变化递增全局版本；版本加入用户/角色、项目、评论开关与实时队列等待，包含不写DB的retry_delay时间变化。接口每次重新授权，不缓存权限。

活跃详情每2秒先查版本，相同不取全量，不同取新详情；查询不重叠，任务切换取消旧请求，轻量查询失败回退既有完整读取/权限错误路径。旧接口和显式刷新继续完整读取。项目内其他任务变更可能保守失效，尚未测量高并发时整体吞吐。新增触发器及计数表幂等，无历史数据删除。

检查门槛：HTTP/存储/轮询模块职责分开，新增版本和路由放独立文件；复用现有资源请求取消与错误状态，未新增UI控件。验证：Go全量通过（platform47.335s）、定向race通过3.435s、类型检查与build通过；补充项目隔离摘要/时间等待测试后定向版本与接口测试通过1.579s；git diff --check通过。测试覆盖稳定版本、同长度JSON改动、复核和评论状态、旧数据迁移、撤权和轻量不含源码。

本地浏览器用真实useResource及隔离HTTP样例验证：首次full=1/status=0；未变化后1/1；变更后2/2且显示新版本；轻量失败后3/3成功回退。截图/tmp/aimangebot-status-polling-proof.png为受控样例，临时预览已清理。合成大报告接口测试full251682字节、status124字节；不外推真实报告或总体性能。没有完成全部重试链/历史竞态端到端验证，最终QA仍需补齐。


## 切片6：授权 SARIF 导出

GET /runs/:id/sarif 在读取前与发送前检查 viewer 权限，返回 no-store 附件；前端提供下载及失败状态。固定 BASE/HEAD/关联仓库 SHA 用不同 URI 基址表达，删除代码仍定位 BASE，元数据不伪造行号。保留验证等级、覆盖不足及人工复核；静态关系只作为属性和相关位置，不生成代表运行轨迹的 codeFlows。未知关联仓库引用省略并计数，未核实历史结果标记 unverified。

维护门槛：导出映射、HTTP、UI 分模块，复用授权和 API 错误处理。Go 全量通过（platform 43.468s）；SARIF 与权限定向测试通过 1.021s；官方 OASIS SARIF 2.1.0 cos02 Schema 使用 Draft7Validator+FormatChecker 校验实际测试输出通过。Schema SHA256 ad6db49878699b091f3eeb765b6e29e92a34bad4da88664d000c923b549c3a25。前端构建通过，下载 Blob 延迟释放并挂载临时链接。

受控浏览器点击无报错，但两次下载事件均超时，不能声称文件落盘成功；截图 /tmp/aimangebot-sarif-download-proof.png 仅证明入口呈现。临时预览文件和服务已清理。官方 Schema 合规不代表 GitHub Code Scanning 原生上传兼容；虚拟快照 URI 供消费者识别版本。最终 QA 仍需实际下载落盘验证。

## 切片7：发现筛选与键盘导航

已实现标题/路径/描述/类型搜索、严重度、人工复核、独立静态复核组合筛选与匹配/总数提示；默认全部，未知复核枚举原文呈现，历史没有独立复核单独可选。仅筛选已授权加载数据，不新增 API。导航使用稳定 finding ID 锚点，键盘 Enter 聚焦对应卡片。过滤通过 hidden 保持卡片挂载，保留草稿及冲突处理；任务切换 key 重置筛选。不提供运行验证选项，静态复核和人工决定分别展示。

验证：前端类型检查与 production build 通过。浏览器真实 FindingCard/Workbench 受控样例：中文路径搜索 1/2；叠加高危变 0/2、显示零匹配提示；清空恢复 2/2 且先前复核草稿文本完整保留；人工待处理+独立支持匹配 1/2；导航 Enter 后 hash 指向 finding-two 且焦点进入该卡片。截图 /tmp/aimangebot-finding-workbench-proof.png。浏览器 Playwright 标签定位不可用，原生 AX 控件交互验证成功。临时预览文件、服务、标签已清理。没有提交样例复核，未测试所有未知枚举及窄屏，留最终 QA；隐藏不是虚拟化，不宣称减少初次渲染成本。纯 UI 切片没有后端变化，不重复全量 Go 测试。

## 切片8：固定快照重复读取保护

来源工具及目录/清单工具完全相同参数已经成功三次后，后续请求不执行读取，返回无进展说明和可复用的来源观察 ID。分页/范围/BASE/HEAD/仓库 ID 保留在精确请求键中；失败不计入成功次数；调查写入与报告提交不受该保护。保护计数是实例内内存，独立复核不共享；不会返回此前源码绕过当下授权。拦截仍计总工具预算并保存 trace/checkpoint/pending，覆盖不足按已有路径保留。并发的在途请求可能在三次完成前已进入读取；此机制不是严格并发限流或语义进展判断，整体模型调用仍受原轮次上限控制。

策略升至 v18；工具输出契约不变，无 UI/API 迁移。初次全量回归暴露旧上下文压缩样例靠相同源码反复读取堆长，保护使压缩不再发生；样例修正为读取11个不同文件，保留长调查压缩/失败中止/固定任务不变的原断言。修正后重复保护+长调查压缩定向测试通过 0.711s；定向 race 通过 1.672s。全量回归结果下述记录。没有实际模型质量/费用评测，不声称测得 token 降幅。

切片8修正后 Go 全量通过（platform 104.834s），git diff --check 通过。未做部署或外部评论。

## 切片9：来源关联的 PR 风险链

PRContext 新增可选 BASE/HEAD 观察链接、风险结果与已检查反例事实、逐边 from/to/relation/certainty/observation_ids。所有来源须归属当前调查且为成功源码观察；逐边 certainty 限 cited/inferred，未知关系继续 unresolved_edges。BASE/HEAD 单独检查主仓库工具参数：允许对应侧读取/搜索、同侧批读，或双侧 diff/compare/Git元数据；上下文来源、相反侧、混合侧批读不能充当单侧引用。该检查保证读取侧归属，仍不验证自然语言陈述语义。关系中的关联仓库来源沿用固定授权快照验证。

模型指引要求检查分派/参数绑定/跨项目契约，禁止名字匹配推导调用；UI 展示风险结果、反例、静态引用/推测关系及来源。主调查 ledger、finding 服务器派生、checkpoint 与上下文压缩继续携带完整结构；旧字段缺失兼容，空链明确不代表完整路径。规模沿用调查8000字节上限，新增数组每项最多8，策略 v19，无DB迁移。

类型检查/build 通过；关系/来源定向测试通过 1.235s，侧检查初版定向通过 1.277s。浏览器真实组件受控样例键盘展开后显示中文跨项目边、静态引用和推测关系、反例与未核实关系，截图 /tmp/aimangebot-pr-risk-chain-proof.png；临时文件/服务/标签清理。不是实际模型调查产出，不作审计质量证明。第一轮 Go 全量通过 platform41.748s；随后收紧批读侧检查，编辑中出现一次括号语法错误并修正，最终定向race与全量结果下述记录。尚需真实多语言/跨仓库质量评测及最终QA。

切片9最终收紧侧检查后：定向 race 通过 2.294s，Go 全量通过（platform42.086s），git diff --check 通过。

## 切片10A：关联仓库评测准备与 v19 运行证据

评测语料新增可选自建 context_repositories；准备器先验证全部ID/路径/预算再创建固定仓库，复用禁Git钩子/全局配置、确定性提交构建。关联仓库固定SHA传入正常Agent ContextSources与snapshot policy，receipt保留SHA，不传期望与rationale。历史Git案例暂不混用自建上下文。原11例兼容。定向评测包/CLI测试通过5.391s/2.563s；race通过5.656s/2.151s；git diff --check通过。没有生产Agent契约变化，不重复前端或平台全量测试。

当前配置最小探测HTTP200；以66b2dcf代码运行v19全部11例，自建语料，无关联仓库，共5条发现、464018报告token、全部有覆盖说明。事实表 evaluation-v19-facts.md；原始独立0600证据保留 /tmp/aimangebot-v19-evaluation-20261006，不能将这些状态称作生产准确率。case-001/007含错误观察/改动行提交被拒后重试的痕迹，尚待质量逐项判定。多语言跨仓库成对语料和实际运行仍未完成，目标保持完整。

## 切片10B：多语言跨仓库成对语料和实际评测

新增4例2组：TypeScript主PR/Python关联服务金额单位、Go主PR/Ruby关联服务动态SQL字段。每组相同主PR，关联服务分别保留风险契约/保护契约；外部gold说明缺失入口与部署假设，neutral ID，标签不入仓库。校验测试确认成对主源码一致、正负标签、全部主/关联仓库可构建固定Git提交，通过3.447s。真实模型运行4709955/v19，4例结束receipt，2条主要条件风险发现、2负例无发现，所有例保留覆盖限制，总报告437440token。报告 evaluation-cross-v19.md记录读取上下文、人工机制判定、错误归因及额外过强整数契约推断；不声称4例代表全语言/生产准确率。原11例人工条件机制判定和配置混杂的token对比补入 evaluation-v19-facts.md。

该轮体现关联仓库能改变判定，也暴露来源簿记和模型过强附带陈述的余项。没有重新跑同例提高成绩，目标尚未完成；继续改进工具错误可操作性、组间交接、同配置效率验证与最终QA。


两轮原始证据已复制到 /Users/worker/.codex/evaluation-artifacts/aimangebot 下 v19-20261006 和 cross-v19-20261006，逐文件SHA256清单验证通过，仍保留原临时轮次。摘要报告引用耐久路径，源码/结果未上传远端。

## 切片11：评测反馈的来源修复指引与复核约束

PRContext 的缺失/重复引用错误现在报出具体 ID，并指明补入当前提交 observation_ids/counter_observation_ids 或移除无依据引用；不自动补关系、不放宽证据权限。一般非来源错误明确 evidence_eligible=true 和目录/知识清单不能使用。独立复核提示新增逐项检查约束/后果，名字/类型/单位不能替代代码校验；附带推断无证据必须列入limitations，核心风险依赖该假设则inconclusive。策略v20，无DB/API/UI迁移。

定向来源/关系/复核测试通过1.341s；定向race通过2.714s；Go全量通过（platform44.277s）；git diff --check通过。测试证明拒绝/明确ID/显式修复和非来源门槛，不把提示字符串存在当语义质量证明。实际模型效果另轮同语料/预算回归保留；不能凭本次消息改动宣称减少token或杜绝过强推断。

## 切片12：独立复核关联来源覆盖门槛

v20同配置4例结果未满足负对照：102模型不查看已配置的billing.py而假设单位，且独立复核supported；101仍有过强整数要求。v20总460604报告token，比v19的437440高，并未证明总体节省。原轮次完整归档并形成 evaluation-cross-v20.md，metadata仅code_revision/policy变更，不能删除失败刷分。

新增服务器门槛：supported若缺少任务配置context仓库任一的本次成功固定源码引用，改inconclusive；正文明确缺失ID和未接受模型支持提案，原模型解释限定1000字内标为未确认，原限制不删并额外追加服务器说明。目录/失败/旧SHA/未引用的阅读均不能满足。无关联仓库兼容原规则；rejected仍校验真实反证。门槛仅验证来源覆盖，不自动判语义或删主发现；多关联仓库可能保守增加检查负担，预算不足保留不确定。提示同步要求检视配置的相关源码，策略v21，无DB/API/UI字段迁移。

定向Verification测试通过0.643s；覆盖/主锚点race通过1.672s；Go全量通过（platform46.042s）；git diff --check通过。证明实际缺源支持会降级、已有限制保留、正文有界、完整引用才保留supported，不声称模型从此不会误报。下一轮实际模型行为、跨组结构交接与最终QA仍需完成。

## v21 实际模型回归与交接缺口定位

420d213代码、相同4例语料/模型/预算运行结束，归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/cross-v21-20261006（逐文件SHA256验证）。101/103主风险条件成立、104无发现，但102误报且独立supported；本轮已读取下游源码，错误为把BASE当正确扣款基准，未识别HEAD修复单位错配。evaluation-cross-v21.md保留逐项用量、来源/错误数及失败判定，未改gold。服务器来源覆盖不能保证语义正确，质量负对照仍未达成。

读取当前分组实现确认：合并checkpoint保留ledger，但后一组只收到manifest，综合阶段不含未完成调查；AC003交接仍缺。已在design.md写出12KiB来源定位交接契约、权限及新观察要求，实施与验收下一切片完成。整体目标保持原确认范围，不因单轮失败缩减。

## 切片13：跨组来源导航与未完成调查交接

后续主调查收到12KiB有界prior_group_notes：已有调查/PRContext/下一步、风险标识与位置、关联成功源码工具参数定位。优先未完成调查，省略完整条目而不截断JSON，省略项/缺定位计数并记父覆盖说明；不复制trace源码输出，不导入新组canonical ledger，不把旧观察当新证据。真实两组SDK样例确认后一组初始任务收到风险标题、来源定位及group-1观察编号；新工具实例校验旧ID仍拒绝。无交接的普通审计不增加提示内容。

综合阶段增加investigations，仍受原64KiB输入预算。主交接与综合在组装前后检查实际阶段的固定来源及授权，取消/撤权不传既有事实；复用既有contextSource的ID/SHA一致性和可选Authorize约定，显式预授权本地来源兼容，生产回调继续检查当前权限。未经授权/不存在来源不视作已准备。没有新UI/API字段和DB迁移，策略v22。

回归曾两次暴露综合授权边界问题：先用了e容器而不是parent实际来源，修正后仍错误地要求预授权本地fixture必有回调。现统一复用已有固定来源规则，不改变合法路径期望断言。最终组/授权源码阶段定向通过4.416s；race通过6.786s；Go全量通过（platform47.127s）；新增投影期间撤权测试通过0.580s；git diff --check通过。覆核深复制、12KiB完整JSON、失败来源不定位、前/后撤权无事实、旧ID不当新源与实际后续组收包。未进行真实模型多组质量评测，省略/真实网络/最终QA仍需核验；单位负对照语义误报问题仍未解决。


## SARIF 下载落盘补验

此前两次浏览器 download 事件超时保留为观察事实；现针对预期文件名查到实际 /Users/worker/Downloads/aimangebot-run-7.sarif 与 aimangebot-run-7 (1).sarif，mtime 2026-10-06T02:35:20.279363 / 02:35:50.090744，与两次受控操作吻合。两文件解码JSON均等于当时受控HTTP样例输出，并用原官方Schema Draft7Validator+FormatChecker校验通过；两文件SHA256均ade285b4b97b184fd3c11833203836a87c1d9ea28379d0430f4f1d9d7db62512。下载文件/官方Schema/摘要另存 /Users/worker/.codex/evaluation-artifacts/aimangebot/sarif-download-20261006，未改用户下载原件。补验说明实际落盘成功，事件未返回不等同下载失败。仍是受控组件样例，生产授权由既有真实路由测试证明，不声称已线上部署。

## 切片14：预期契约对比与执行错误修复指引

主调查和独立复核加入预期契约/具体输入/BASE与HEAD可观察结果对比，不默认BASE正确，不把相对变化本身当损害。v23实际同配置4例运行揭示102调查超8000字节反复更新、103锚点与来源关联修复失败，均耗尽步骤，未证明质量提升。647184报告token；原始证据SHA256归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/cross-v23-20261006，evaluation-cross-v23.md保留状态/归因，不把退出0或零发现记成功。

据执行证据修正错误：调查超限报实际UTF8字节数和8000字节上限，建议精简重复源码和事实；空claim、ID长度分别解释。工具描述同步限制；finding关联错误包含缺失观察ID和调查ID、指明先更新关联；锚点错误指明HEAD新增/BASE删除与精确侧/行重新读取。所有门槛保持，不加模型/工具预算，不回显额外源码。策略v24。定向Oversized/PRReference/Verification/Group测试通过3.429s，Go全量通过（platform43.948s），git diff --check通过。超限失败保持原ledger，精简修复后可更新。新轮实际行为仍待评测；语义负对照、验收样本隔离与最终QA未完成。

## v24 实际模型回归

同预算4例完成receipt：101/103各1发现、102/104零发现，全为incomplete，没有步骤耗尽；505209报告token。102正确识别HEAD修复美元单位、104正确识别下游固定映射保护，执行修复有效于本轮。101仍有无商品交付证据的同等购买价值断言，103具体SQL片段未经运行验证，独立supported不能消除所有叙述过强。原始250文件逐项SHA256归档，详见evaluation-cross-v24.md。调试回归不当隔离验收，整体目标仍需独立样例、真实多组与最终UI/完整AC核验。

## 切片15：全文复核范围门槛

新增独立复核claim_coverage=full/partial/unknown声明，覆盖标题、描述、触发及具体附加后果。新supported且非full（含缺失）经服务器降为inconclusive，候选不删除，原解释与limitations保留；原有来源/锚点/固定提交限制继续先校验。旧已保存记录不重写；新字段兼容已有API展示与SARIF属性，策略v25。模型可能误判full，该门槛只防止显式部分支持成为全文背书，不宣称自动语义证明。

解析/完整来源/partial/unknown/缺字段/拒绝/1000字上限定向通过0.660s，实际SDK新partial与缺字段模式保留原候选且有覆盖说明，通过0.564s；范围与来源race通过1.755s；Go全量通过（platform42.606s）；git diff --check通过。真实模型对新增契约的遵循仍需另行验证，未把SDK合成输出当真实质量结果；独立验收集、真实多组和最终UI验收未完成。

## v25 实际复核与验收集冻结

真实4例复核均结束receipt；101/102/103各1候选且claim_coverage=partial，服务器降inconclusive；104零发现。490166 token。101伤害方向相反、102负例再次误报且复核计算错误，降级不等于消除误报，evaluation-cross-v25.md保留失败与逐项解释。250原始文件SHA256归档。

另外冻结未参与此前调优的6例corpus-acceptance-v1.json，外部人工真值包括Lua保护删除/修复、JS到Python身份头覆盖及独立认证反证、Go到Python兼容默认参数、Python无关日志PR与既存eval。协议明确不据同一验收输出调优再声称未见、逐条判断附加断言而非计数。SHA256 8fc3da20d047082dd6a2e37460afe4254ab4870e52c26735b27fbc563e0474af；临时只读输入验证程序使用实际LoadCorpus、BuildRepository、PrepareContextFixtures成功构造全部6个固定Git输入，未运行样例/模型，程序及生成目录已清理。无生产代码变更不重复全量测试。真实验收结果待运行，整体仍未完成。

## 切片16与隔离验收结果

FindingCard描述前复用VerificationNotice显示partial/unknown全文未获支持；主调查标签明确为主调查源码支持，VerificationEvidence显示full/partial/unknown及历史未声明范围，缺复核仍显示原空态。API可选类型同步，不改人工草稿/权限。类型与生产构建通过（Vite955ms）；真实组件加载v25失败样例，AX确认提示先于描述、full/历史/无记录状态分别可见；360×800截图 /tmp/aimangebot-verification-scope-proof.png，DOM scrollWidth=360等于viewport，Tab焦点到observation-1。恢复视口、关闭临时页并清理测试源码/服务。

隔离6例首次运行结束：两个条件风险核心机制正确识别，四个负例未报新增风险；但203主调查未读取授权下游，独立阶段补读而coverage仍称不可检查；205未读契约，不能称兼容判定通过。全部incomplete，273561token，未配置费用。206冻结外部锚点有人工标注行8/实际变更行7勘误，原输入保持不改，人工按实际diff核对。284文件SHA256归档，evaluation-acceptance-v25.md逐项保留质量/链路/覆盖缺口。整体尚需跨阶段事实一致、主调查上下文能力、真实多组与最终AC审查。

## 切片17：上下文预检与主阶段覆盖

有配置关联源时首次主模型请求前复用list_repositories，计入原工具预算并持久化trace/checkpoint，导航明确非源码；无关联源不改提示或增加调用。失败/取消/检查点失败不发请求。列表组装后重新检查每个固定源授权，期间撤权不暴露部分聚合。成功主阶段按源码trace的固定提交、EvidenceEligible、成功与阶段记录未读取来源；独立复核后来读取不追溯标成主调查已读，未读只说明相关性/下游保护未知，不推断不存在。策略v26。

实际SDK初始请求检验导航、available=true、evidence_eligible=false及先有检查点，撤权/取消/保存失败均0模型请求；无源路径原样，主阶段未读有服务器说明。来源资格测试排除失败/列表/旧SHA/独立阶段/不合格输出；聚合授权二次撤销不返回条目。定向3.450s、race3.340s、Go全量通过（platform43.590s），git diff --check通过。因参考了验收v1的缺口，这套从v26改称回归语料；历史独立v25结果与原标注不改。真实模型是否主动读取仍未证明，后续定向诊断须保留失败而非报准确率。

## v26 实际上下文定向诊断

只运行205/203，分别独立目录并保留全结果。205现读catalog.py/固定配置后基于默认20与校验判兼容，零发现、rejected调查有相关观察。203主阶段读profiles.py与deployment.conf，finding与PRContext影响引用双方来源，不再称关联源码缺失。两例incomplete、64840/97561token、8820/19300ms，无工具错误；相对之前读更全同时token上升，未宣称效率提升。203仍缺结构化before/after IDs及逐边关系，复核对内部网络可达性的补充推断无证据，模型full不当语义保证。来源能力本轮改善、完整链路与整体AC仍未完成。SHA256原始归档与evaluation-context-v26.md保留细节。

## 切片18：风险链缺口由服务器记录

有效候选合并后纯投影检查PRContext前后primary来源、逐边关系；缺字段明确coverage gap，推测关系即使unresolved_edges为空仍标未核实。不创建推断关系、不删除候选、不把独立supported/full当完整路径。canonical Git metadata本身经验证包含BASE/HEAD事实，无PRContext时不要求重复链路；有PRContext则可暴露其来源记录不足。策略v27。

首轮Go回归暴露旧metadata/group/HTTP/diagram的零覆盖缺口断言；先修正元数据重复要求，再把line候选测试改为精确检查新增缺链说明、HTTP保存为incomplete、分组/图阶段保留既有gap，原发现集合、图状态、筛选/复核/重试不变。来源校验不放宽。定向PR验证1.119s；scope/group race1.726s；修正后Go全量通过（platform44.425s）；git diff --check通过。实际SDK支持结果仍保留PRgap，完整字段/推测关系/无发现/元数据以及纯投影不改原事实有定向覆盖。未新增真实模型轮次（本切片只改变确定性投影），既有v26不完整链会被明确说明而非自动变完整。整体真实多组、未见验收和最终QA/生命周期报告仍未完成。

## 切片19：生产分组评测入口

CLI新增显式-grouped默认false，模式记录metadata，checkResumeMetadata继续全字段精确比较；失败恢复不改原记录。grouped复用生产PlanAuditGroups/AuditGroups，不改32KiB/24文件/8组、原模型/共享工具与token预算/240秒。单组路径保留。corpus-grouped-v1.json复用已见Lua机制与24个无害文档变更，人工边界诊断25文件成两组，不称独立质量benchmark。恢复分组/单组混用与损坏metadata拒绝、失败不覆盖文件有新测试。CLI测试0.545s、evaluation5.059s、git diff --check通过；平台未修改，上一切片Go全量仍为对应平台代码证据，未重复无关全量。真实多组诊断下一步启动，结果未出不声称交接质量验证完成。

## v27 真实多组诊断与完成矩阵

生产25文件成24+1两组，两组completed，最终1个group-1来源/调查的规范风险保留、独立supported/full，group2重新读取固定BASE/HEAD/diff使用新观察；status=incomplete、256290token、46391ms。后组只负责文档却重复提交首组Lua风险被正确scope拒绝；存在重复调查名、模型将范围拒绝称环境问题、汇总重复coverage/文件总数叙述不准。PRContext记录缺口如实保留，不做完整链路或效率结论。62原始文件SHA256归档、evaluation-grouped-v27.md记录事实。verification-matrix.md按R3所有12项核对当前证据，明确保留未完成项，整体完成审查未通过。下一改进明确当前组锚点范围，稳定去重相同覆盖说明，最终UI与新隔离验收/交付审查仍需完成。

## 切片20：跨组提交范围指引与覆盖去重

每组注入自身ID/文件清单完整JSON及说明：总manifest只导航、只有当前组diff可提交、先前规范finding由服务器保留无需重提，跨组可读但旧观察不能授予新锚点。普通单组不添加，16KiB超限省略文件列表但保留完整JSON及数量。ValidateFindings对未在当前scope的路径明确拒绝，原side/line/source边界不变。汇总成功及分组最终投影稳定去重完全相同coverage字符串，首次顺序和不同文字/不同来源说明保留；不改raw trace/历史，去重说明不能当工具失败次数。策略v28。

实际SDK后一组初始消息含当前group-2与不重复提交说明、保留旧来源导航；模拟综合回传重复原gap及重复新gap，最终精确保留原gap+新gap两条。纯helper无源审计不变、超限完整JSON、顺序/无别名、越组路径错误定向通过；初定向1.137s，补SDK去重后0.580s，race1.862s，Go全量通过（platform45.436s），git diff --check通过。SDK增加的去重断言在全量编译后定向补跑通过，生产代码没有再改。真实模型行为未以测试代替，下一轮生产两组诊断保持上一失败证据及预算，不宣称已避免所有重复调查或完整链路。

## v28 真实多组回归

同生产两组诊断结束：group2只有两次read_file，没有重复调查或越组提交；主风险/规范来源保留，两组completed、1发现、最终incomplete。覆盖9条全部不同。154401token/23733ms，相较v27 256290token/46391ms只报告样本差别，不宣称普遍效率/提示单因素效果。首组2个工具修复错误、PRContext双方来源/逐边缺口和模型计数叙述错误仍保留。SHA256原始归档与evaluation-grouped-v28.md记录全事实；真实多组所发现的两个问题本轮恢复，整体质量/UI/新隔离验收与生命周期审查仍需补齐。

## 切片21：详情旧快照提示与状态QA

详情500等刷新失败但缓存尚在时，共享ResourceRefreshFailure说明不是最新状态并给出重试；读取/动作错误分开展示，初次加载重试disabled。401/403/404继续原hook清空，未改写入/轮询/授权行为。类型与生产构建967ms通过、git diff --check通过。受控实际RunDetail检查加载/初次500键盘恢复/旧快照500/自动与手动恢复/403清除/队列/unknown评论/空结果，360px无溢出；实际复核内容在刷新失败后保留，fixture204不当数据库保存证据。ui-final-qa.md记录细节与限制，截图SHA256保存到私有本机证据目录，临时页/服务/源码全清理。对应前端状态验收补齐，整体新隔离验收、风险链质量与交付审查仍需完成。

## 隔离验收v2冻结与输入验证

当前v28生产策略保持不改，新增6例Rust保护移除/修复、PHP到Python可控owner及独立认证反证、C#到Ruby显式false兼容、Java无关日志PR与既存exec。类别重叠但源码/语言/仓库未用于此前调优，真值人工独立，双方条件与不运行假设记录到acceptance-protocol-v2.md。SHA256675923cd5ecf2a6831e17e65fe1ea3a53e183c23f3750dcd973d18b9ce35bc70。

临时Go验证器复用实际LoadCorpus/PrepareCase/PrepareContextFixtures/Changes/BuildDiff，6例固定源码与每个外部BASE/HEAD锚点均为真实变更行，上下文文件可列出。初次验证器ListFiles签名写错编译失败，按Repository接口修正后实际验证成功；没有将失败记输入通过。临时程序及全部Git目录已清理。未改生产代码，不重复全量测试；git diff --check通过。完整模型验收结果待运行，不声称新语料已达标；若据输出调优必须转回归。

### 最终证据与导航修复

v28 隔离验收v2首次运行已归档284文件、逐项SHA256复读验证，详见 evaluation-acceptance-v28-v2.md。两个正确条件风险、四个零告警，其中405下游未读不能认证兼容；6/6 incomplete、296749 tokens，不隐藏原始失败与过强网络断言。v2未用于模型调参。

最终审查发现 AWO-REV-001：发现快速导航更新hash会触发应用路由。改为原生button页内滚动/聚焦，实际Return/Tab验证route保持、草稿保留，类型/构建通过，详见ui-final-qa.md。

## v36 明确调查状态收尾

恢复回归704/705的四项计划已checked，但模型仅record_hypothesis，记录仍investigating；不是update验证失败。逐轮导航新增unresolved_ledger_count，只含服务器计算的计数，不包含模型id/claim。工具说明明确创建总是investigating，需显式update；状态相对于实际claim，支持兼容结论不等于需提交漏洞。未知关系和未检查任务保持原门禁，不自动收尾、不改预算、历史不重写。

定向回归 Test(ProgressNavigation|PrimaryProgress|InvestigationPlan|Recording|RepositoryUnavailable|ContextRepositoryUnavailable) exit0（27.685s）；SDK收尾状态投影与隐私投影race exit0（9.247s）；go vet ./internal/platform与diff --check通过。SDK验证创建后unresolved=1、显式update后=0；这些是导航/契约证据，尚未证明真实质量改善。下一轮固定v36代码复跑同样语料，仍为回归。

v36真实回归已完成，结果见 regression-v36.md。704/705调查显式收尾、702补账本，但全部incomplete，独立复核与风险链缺口仍保留；未声称总体质量提升。

## 独立复核引用安全诊断

v36历史invalid_observation的具体引用无法重构，未猜测也未回写。新增私有typed来源错误，保留原Error文本及验证接受/拒绝条件；后续model_response Error/安全shape code可细分 invalid_observation_unknown/wrong_stage/non_source/read_failed/malformed_output/snapshot_mismatch/empty_source。只输出枚举，不输出引用ID、原因正文、源码或底层错误。未知typed分类回落invalid_observation。未自动删除坏引用、重试或增加预算；复核仍unavailable，未修改提示及策略版本。

测试：来源分类/原验证条件/解析/上下文回归exit0（3.319s）；实际SDK伪造旧ID产生unknown安全分类，且候选/覆盖缺口保留；SDK/来源校验/取消race exit0（2.522s）。go vet与diff --check通过。隐私测试验证错误文本、未知分类和形状输出不含私有ID、模型正文或读取错误。初次定向选择未覆盖IndependentEino测试，随后显式race覆盖，未把无匹配用例当SDK证明。此仅改善诊断，不声称语义正确性或准确率改善。

## v37 显式记录纠正与只读边界

新增resolve_recording_errors，明确关联仍pending的失败与之后同local产物的成功回执，1–8对原子验证。同主快照/阶段、编号一致、身份相同且纠正成功才退役指定旧pending；失败trace和纠正事件保留，不能解除源码失败、分页、计划/关系或另一候选。无local id失败不能安全自动归属；详见recording-corrections.md。纠正工具自身拒绝只留trace，不制造递归pending，原工作缺口仍在。策略v37，原模型/预算不变。

独立复核和时序图共用source工具+有限navigation白名单；未知新工具默认排除，纠正工具非source证据。首次全量失败定位出旧15/12数量断言和时序图独立黑名单，已改成必需能力/禁止能力断言并统一白名单，不把失败隐瞒为首次通过。

验证：纠正/主调查/复核/导航定向exit0（16.322s）；SDK/纠正/复核/执行器停止race exit0（7.012s）；旧失败修复后的SDK/StandaloneGit/sequence/纠正race exit0（13.751s）；真实ValidatedFinding纠正与未知新工具拒绝race exit0（2.516s）。全量go test ./... -count=1 exit0：root11.161s、audit-eval1.271s、evaluation19.081s、platform97.477s。全量启动后仅追加产物保留的test文件，生产不再变动，额外定向race覆盖该新增测试。go vet与diff --check通过。包含批次原子性/跨产物/未知/逆序/跨快照/跨阶段/源码/partial/重复拒绝；history不改写、候选保留、计划/PR缺项仍可见。未改变UI，无需重复无变化界面验证。真实v37效果尚未评测，不据本地通过宣称完整质量改善。

v37独立复查固定256b6f9、base4946659，COMMENT，原子性/固定trace/源码不能退役/白名单/历史保留定向通过1.206s；没有访问凭据或模型API。复查指出同local ID不能证明同陈述：后续加上调查Claim与候选Title/Description/Trigger精确比较，防止另一陈述清除旧pending，源ID/锚点仍可纠正。变更后相关SDK/独立复核/时序图/原子纠正race通过3.717s、vet/diffcheck通过。一次编辑脚本语法失败未写入文件，其后重新应用并验证；此前4.399s为原匹配规则测试，不作为新增校验的证据。该比较仍不证明真实语义，不将ID或字节相同当作源事实。真实回归尚未启动；原256b6f9预备冻结不会发请求，新冻结使用修订代码。

## v38 合格纠正导航实现与验证

设计计划23a7886已推送；单对校验提取为持锁recordingCorrectionKeyLocked，resolver原条件及批次原子性不变，有限导航同样复用。最多4对规范主/分组编号，稳定选择后续最近合格成功记录；不修改pending，不把源/Claim/错误写入系统。实际SDK请求第4轮出现2→3，第5轮resolver完成后字段移除，历史与真正计划/未解决状态仍在。

首次定向测试失败5.343s：测试将提示中固定unknowns单词误当任意unknown源泄露；另一个夹具update缺少合法status。改为检测实际引用字符串并补investigating，随后recording/navigation/SDK/readOnly定向race exit0 4.608s。没有放宽生产门禁适配测试。真实旧801回执重建仅一个合格pair，另一同参submit已自然清除；详见regression-v37.md。临时检查文件不入仓库，无新真实模型请求，预算不变，策略v38。全量go test ./... -count=1 exit0：root9.641s、audit-eval1.404s、evaluation16.200s、platform111.645s，web无测试；go vet ./...、git diff --check exit0。首次定向失败和本地回执检查失败均保留。最终review/CI另观察，不由本地绿灯外推真实模型质量。

F5补充：da039f3源码CI run37427483339 success；三份独立冻结范围报告保存在工作区外，详情见completion-audit-v38.md。新面板当前RunDetail实际IAB集成补验已归档并独立核对、临时环境清理；工作台scope闭环，整体AER-001仍open。F6为部分交付，无批准/合并/部署。新的文字仅记录事实，不把历史真实评测改成当前v38执行。

## v39 主仓库 HEAD 目录预导航

首个模型请求前调用既有 list_files 第1页，计入原工具预算、原观察编号与持久化链路。目录仅进入不可信 user 消息；系统仅投影 not_inspected/unavailable/partial/complete 与文件计数，不能据此宣称已读源码或调用关系。普通失败保留待办并允许后续恢复，执行器不可用、取消和检查点失败在模型请求前停止。明确 PR changed-path manifest 并非整个仓库，删除文件仍需 BASE 检视。

分组最低有效预算为 HEAD 目录加一次源读取（2次）；有固定跨仓库配置再计已有授权导航（3次）。预算不足保留未处理组，不提高总预算。SDK夹具来源编号因真实新增首观察顺延；大仓库仅列第一页的 bounded list_files 缺口保留，未改生产门禁适配测试。

验证：首轮定向暴露旧编号与最低预算问题；校正后相关定向2.685s通过。后续原生Git/跨仓库定向29.399s失败于 worker 仍假设只有一个覆盖说明，更新为计划缺口加真实分页缺口，单测3.822s通过。首次全量135.091s同样是该旧断言失败；最终 go test ./... exit0，platform137.901s，其他包通过或缓存。新增导航/纠正SDK/进度/优先级 race exit0 7.244s，go vet ./... 和 diff --check exit0。UI未改动。策略v39；尚未运行v39真实模型，不能把工程验证当效果证明。整体完成阻塞 AER-001 仍未关闭，不自动合并/部署。

### V39-001 持久化失败阻断修复

独立检视冻结dda3eda报告REQUEST_CHANGES：首次list检查点普通失败或ErrConflict之后，外层回调成功使两组继续2个loopback模型HTTP请求，最终返回nil。保留finding ID V39-001。每组Progress wrapper现在同步保持首错误，后续checkpoint直接返回原error，不再尝试外部callback；child返回后先合并当前产物和trace，再标失败并返回原error链，后组维持unprocessed，补充阶段不执行。

独立反例转换为回归：普通错误/冲突，分别首次和第二个启动检查点失败，均0实际模型HTTP请求，callback停止于失败次数，list trace保留、后组unprocessed、errors.Is保留。首次race1.974s失败因测试误把第二个启动检查点当作模型请求后的外层检查点，实际也是请求前；校正预期后race2.930s通过。没有改生产逻辑适配该断言。vet/diffcheck通过；最终全量与新SHA独立复查待完成，不继承dda3eda结论。v39真实802回归绑定修复前dda3eda，详见regression-v39-navigation.md；原样本真值与预算未改。

V39-001修复冻结源码0e7db81bb84f2e0ba1b7d1f9f3291b6f27f75979。最终go test ./... exit0，platform102.421s，其他包通过缓存；并发启动的旧测试版本全量112.116s失败同前述第二检查点错误预期，未作为当前通过证据。修复race2.930s、vet/diffcheck通过。当前源码CI run37431241936运行中，旧dda3eda CI因后续push cancelled，不称失败或成功。真实模型定向17910tokens、原接口恢复的证据绑定dda3eda；全程无合并/部署。独立V39-001新SHA复查另补。

独立新SHA复查0e7db81报告COMMENT（仅本边界scope，非整分支批准），V39-001 resolved，无新blocking。原overlay正确反例race2.966s；额外已完成第一组finding→第二组普通/冲突检查点失败→第三组及补充阻断race2.177s，验证原finding/trace保留、仅前组2次HTTP、失败callback仅一次。外部首fixture遗漏coverage_notes的失败保留；修正夹具后通过，未改源。报告保存在 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v39-0e7db81/PR_REVIEW_REPORT.md。整体仍incomplete，不能继承旧分支批准。

## v40 状态对应的记录动作

新增纯next_recording_action建议，用原来源/ledger/plan/context/correction状态稳定选择；固定文字按动作说明操作，不把Claim/path/source/error插入系统。无context/纠正pair时不重复附无关说明；未解决关系和必要证据仍保留。明确每个PR/组包括no-findings也记录changed-behavior检查，创建仍investigating，safe claim可supported且不需finding。原模型参数、工具校验、末轮预算与调查记录语义不变，没有forced tool_choice或自动填计划/关系。策略v40。

定向exit0 2.866s；原计划/纠正/导航/分组停止与新增SDK/action race exit0 7.774s；vet/diffcheck通过。真实SDK三轮依次inspect_changed_source、record_changed_behavior、inspect_plan，实际记录四项pending、无checked来源/PR关系自动生成，仍incomplete。全量Go运行中；真实v40模型未执行，不把合成检查当质量证明。上一源码0e7db81 CI run37431241936已success；这不是v40 CI。

F5：最终full go test ./... exit0（root10.043s、audit-eval1.343s、evaluation26.160s、platform134.382s，web无测试）；race7.774s、vet/diffcheck通过。新源码23d3bc3独立scope COMMENT/mergeable=false、无新blocking，独立两批定向0.756/0.843s，actual SDK/有限枚举投影/来源/预算连接已核。不代表整分支批准，AER-001 open。真实802实际记录四项checked计划与显式纠正，但safeClaim错误rejected、关系/BASE HEAD链接缺失；102808tokens、高于v39，详见regression-v40-actions.md。无新增真实验收或全部completed门槛。

源码CI run37432182646（23d3bc3）目前in_progress，待终态，不继承0e7db81成功。最终覆盖实现另已定位：agent.go只把investigationPlanCoverage加到所有调查，PR recording gaps只对findingPRCoverage加入。因而safe/no-findings调查即使BASE/HEAD链接和关系缺失，也可能仅靠模型自行写coverage；导航与最终缺口来源需要统一，不能靠假设status或四项checked就当记录充分。下一切片同时保持原Claim极性问题open，不将增加覆盖告警冒充语义修复。

## v41 调查级最终PR覆盖

所有真实ledger调查（包括无finding且supported/rejected）使用prRecordingGaps投影缺结构、BASE/HEAD来源链接、缺/推断关系到最终CoverageNotes；不修改claim/status/plan/source/history。metadataOnly来源分类严格依固定主阶段/主仓库成功canonical、scope.valid/metadataOnly、source flag/编号/快照/全文逐项一致且无重复trace ID；没有引用、混源码、正文变化或错误来源均不授予豁免。此来源种类不是语义判断，不证明metadata claim的运行效果。分类共用于工具feedback、live progress和final coverage，保持纯metadata比较不必制造执行路径；plan门禁未松动。策略v41、UI未改，无新DB/API字段。

初定向1.874s及race4.646s通过。首全量131.408s失败于旧SDK假设plan勾完即无任何coverage，新增PR gap本应保留；改为assert无plan gap但仍有PR gap。追加完整记录正例首race6.321s失败：内存runRepo不支持get_diff原生Git，该工具真实被拒绝，不应当完成。改为一致BASE old/HEAD new的只读fixture仓库及分别真实读取双侧（仍由源工具给编号），完整记录正常succeeded，缺链接两种状态仍incomplete。校正race4.853s通过；元数据/live/原纠正/状态导航/分组停止广泛定向race8.043s通过；SARIF消费者追加断言race4.447s通过。第一次校正全量103.905s覆盖metadata共用前版本，不当作最终source proof。最终metadata共用源码全量运行中；vet/diffcheck通过，新增SARIF仅测试断言未改生产，全量启动后独立定向覆盖。

真实v40 case-802回执离线projection检验：新增BASE/HEAD链接及relationship三条说明，原rejected错误极性与全部原记录JSON未变；没有新增API请求或重新宣称模型质量。最终helper离线0.799s通过，前版本0.772s仅历史。外部检查文件/overlay/output/source SHA保存 /Users/worker/.codex/evaluation-artifacts/aimangebot/v41-frozen-v40-projection-20261006。旧真实回执不修改，所有语义/关系完成阻塞继续open。v40源码CI37432182646 success，不继承为v41 CI。

F5/F6：冻结83671eaecbdcdd1a8b71b8c77f68850cb81cd0b9。最终生产源码full go test ./... exit0 platform111.423s，其他包通过缓存；追加SARIF测试断言由race4.447s覆盖，无后续生产改动。独立fresh-context scope COMMENT、无blocking：自主定向9.457s及8模式source-basis overlay0.709s通过（counter-only正例，stale BASE/ineligible/scope removed/supplement/malformed args/reversed duplicate/mixed source负例）。报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v41-83671ea/PR_REVIEW_REPORT.md，完整大分支显式排除，不作为整体批准。异常返回路径仍只有中断总说明，但worker/SARIF不能变clean；没有据未执行的细项补造coverage。源码CI run37434193201后续核对终态success。离线归档5文件SHA与当前helper源SHA核对一致。整体AER-001 open：特别是v40 safeClaim错误rejected及跨项目/入口语义关系质量，v41只补最终说明，不声称判断修复。下一步应区分“是否支持实际Claim”与“是否有finding”的工具输入语义，减少状态误用；兼容旧记录与来源门禁，效果需原模型实际验证。

### v42 命题评估适配实现

模型update_investigation schema新增必填claim_assessment有限枚举，隐藏顶层legacy status并保留plan.status。独立DTO适配明确支持/反驳/证据不足到原状态；旧JSON status/direct update继续经过原ledger校验，矛盾新旧输入/未知枚举拒绝。真实提交DTO进入trace，标准状态写ledger；提交及规范化/保留数据预算均检查。来源资格、计划、纠正同陈述和所有审计预算未改。新增schema/兼容/冲突/无源码/字节预算测试；实际SDK HTTP捕获schema及新assessment解析为rejected。定向测试1.157秒、race含原纠正/动作/计划回归3.329秒通过；全量/vet进行中。真实模型语义效果未验证，AER-001保持open。

F5 v42：冻结b19ff4677f475d15bd5ab7d04e0da7c27bdd5807；全量Go exit0 platform137.600s、其他包通过，vet/diffcheck exit0。真实802一次原deepseek-chat/15steps80tools240回归，75067tokens、6轮、16.605秒、完整usage、无API余额错误；实际new assessment和兼容status一致支持守卫改善命题，调用关系及上游未知保留。结构化BASE/HEAD引用仍缺，两条最终说明保证incomplete；单例不推广质量/成本，无运行时执行。外部归档40文件SHA核验一致，详见regression-v42-assessment.md。独立fresh-context切片COMMENT/无blocking，自行定向0.788s通过；报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v42-b19ff46/PR_REVIEW_REPORT.md，排除整分支批准。源码CI run37435634059最后核对in_progress；AER-001继续open。

### v42 剩余质量检查（生产源码未改）

计划8129579已推送后，803/805各一次原模型/预算真实回归，actual revision8129579、生产源码b19ff46未变。803记录源支持跨项目关系和HEAD6条件风险，仍缺双侧结构化链接；805摘要正确default25兼容，但三次改写immutable plan question遭拒、拟提交命题极性仍错，保存调查investigating。总276845tokens、完整usage、无余额错误；归档132文件SHA核验一致，详见regression-v42-quality.md。不是新盲测、没提高预算/改真值/运行样本。AER-001仍open，不将工程测试或摘要当完整质量证明；明确下一步计划身份错误可恢复反馈。

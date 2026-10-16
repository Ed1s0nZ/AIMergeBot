# 原生仓库执行：工厂与完整消费者接入方案

状态：F2/F3作者稿，待技术评审；不是生产许可。2026-10-16，codex/sidebar-workflow。输入为Confirmed requirements.md、repository-recovery-contract.md及S4、budget/API diff/resource三个具体合同、d8a0047网络反例。REQ-001–023/AC-001–021保持原范围；本文实现GitHub/GitLab与跨仓执行，不把邮件、工单、人工处置剩余工作抹除。

最新N5细化见[repository-native-publication-contract.md](repository-native-publication-contract.md)：hook原body/delivery双键去重、异步队列及同budget恢复、跨run发布目标和未知回执、GitHub状态映射与context内容边界。GitLab固定对象probe已完成局部SDK/wire验证，见API diff合同；不是生产adapter或真实GitLab服务验收。后文“待补N5细节”的原作者记录以该补充为准，完整评审仍未执行。

## Workflow / Maintainability / Lifecycle Gate

P6→P7/P8；已经取得固定scope、身份schema2、凭据激活/运行时fence、GitHub对象reader、网络协议/URI反例、有界matcher与资源边界。主要缺口是把准入、执行、工具、后台和发布全部接线。允许本轮写交接方案；代码仍须正式评审通过后实施，所有管理写入口保持现有501/只读到S5/S6验收。

维护风险high：Runner、Settings、Store、GitRepository、工具注册及发布跨边界。采用工厂/session/admission、provider读取、transport/gateway、ledger、resource、publication独立文件；原文件只委托，不再把SQL、HTTP解析、diff、资源扫描放进Runner同一方法。既有agent_history.go/agent_explore.go直接识别*GitRepository；Git模式必须保留这个真实对象，不能包一层Repository后让高级工具悄悄消失。API-only始终零Git命令，不以缺高级工具为由偷偷启动Git。

## 版本、历史与开放条件

现有PolicyVersion为eino-audit-contract-v50；不能只改常量就把旧nil policy当成有scope/账本。提案保留该legacy版本，新增NativePolicyVersion=eino-audit-contract-v51。服务端从已验证准入种类决定版本，HTTP不得传version/actor/账本ID。v51严格必有RepositoryScope和RepositoryBudget、预算映射及合法digest；v50不得借新scope或绑定进入旧GitLab路径。

native准入只走专用EnqueueRepositoryAdmission内部接口，持有完整admission receipt且同事务recheck/attach；现有Enqueue/EnqueueUser保留legacy拒绝，不能简单删除requireLegacyRepositorySnapshot。v50历史报告不改写；已绑定项目的v50旧任务不自动复活。新binding恢复仍由S5显式身份变更产生新任务，不能克隆旧任务补造scope。

需要传播的版本检查逐点包括runs_store.go的查重/INSERT、runner.execute、audit_retry的策略判断与child INSERT、slack_reaudit固定旧HEAD准入、followup、standalone和导出。native retry保留父版本/完整policy/digest/同预算，legacy retry仍走原规则；不使用“版本字符串合法”替代身份授权。standalone没有服务端scope时仍使用原路径，不假装加入平台native账本。

## 内部对象与调用契约（全部拟议）

| 对象/字段 | 类型/必须/默认 | 来源与校验 | 生命周期/兼容 |
| --- | --- | --- | --- |
| RepositoryFactory.Store/Settings | 必须指针，无默认 | 服务启动组装；只能一个当前credential activation实例 | 不从请求替换Store或凭据 |
| AdmissionPrincipal | user或service二选一 | user来自session用户ID；service只能由已验证poll/webhook构造，并绑定启用项目/集成scope | actor=0不是通用权限后门 |
| RepositoryAdmission | opaque私有handle | budget admission owner、冻结target、source、context和观察、策略；调用方不能自行填内部字段 | Close/Attach终态；失效后不可重用 |
| RepositorySession | run/budget/owner/epoch、ctx、sources map、cleanup | OpenRun对当前run/worker/全部身份/账本二次验证 | 串行Git/匹配；整个审计共享；关闭后工具不可继续读 |
| RepositorySession.Primary | Repository，必须 | Git模式为真实*GitRepository；API-only为nativeAPIRepository | 不提供动态branch fallback |
| RepositorySession.ContextSources | map[int]ContextSource，非nil | key为内部项目ID，与scope.context和旧context投影严格相等 | 每个source固定单SHA、各自reference；同session ledger |
| AuxiliaryRead | 私有有界操作handle | purpose、actor/service、project、可选related_run、当前scope，独立新账本 | 不向closed audit账本续额；只用于明确的查询/发布/诊断 |

Factory调用形状：BeginAdmission(ctx,principal,internalProject,number)→Admission；Capture生成固定Snapshot/policy；Store专用入队事务Attach；OpenRun(ctx,run,workerOwner)→Session；OpenAuxiliary(ctx,purpose,run,principal)→AuxiliaryRead。错误只含有限分类；Close幂等、等待自身监督任务并保留cleanup失败。不能把返回的Repository单独缓存到全局跨run复用。

NTR-001修订：预算持久化principal_kind/user actor或service source证明，service的actor_id为NULL并使用严格版本化principal JSON，不能与原“actor必须正值”冲突。requested_by=0只是run兼容载体，worker/auxiliary需沿原根principal验证；普通HTTP无权提交service。具体union、SQL CHECK和各source恢复约束见budget合同。

## 固定身份与PR观察

1. Store预查target启用、永久identity kind、绑定revision、集成enabled/revision/project scope和principal；创建同一次admission账本。远端调用不在DB事务内。
2. GitHub按冻结target full_name调用repository/PR元数据，核对返回repo ID/owner/name、PR号和base repo。GitLab按绑定remoteID取project/MR，核对project ID/path_with_namespace，不用内部项目ID发请求。name只作为路由，不能替代ID。
3. 从PR/MR元数据取得source repo身份；先在Store找到精确provider/origin/remoteID的启用绑定并授权source，再读取source commit/tree/blob/compare内容。未知fork拒绝，不导入、不因公开可读而授予权限。target==source复用相同reference/client，仍验证一致。
4. GitHub固定base tip/head full SHA后，以固定SHA compare取得merge-base，校验返回head和merge-base；files/patch不作为差异目录权威。响应仍有API单体/整体上限，无法取得完整元数据明确失败，不当作无变更。GitLab固定MR diff_refs和matching diff version，校验base/head/start用途；不能在matching版本缺失时降级动态changes接口。
5. 原Snapshot.BaseSHA=merge-base，HeadSHA=head，MRIID兼容承载PR号；GitHub DiffVersionID=0，GitLab保留验证过的版本ID。RepositoryScope.Observation保存base tip/merge-base/head与provider/number。target/source同origin/provider及SHA格式严格验证，context可以其他provider/origin。
6. Capture当前context配置/精确SHA、workflow、模型配置、limits，normalize只用于producer；入队事务重新校验全部reference/角色/凭据代次/配额/context索引，插入run及同预算映射、事件原子提交。失败关闭admission保留消耗，不产生半个run或poll已见状态。

legacy_global credential reference只适用于身份登记能证明的legacy项目、冻结origin与active generation。未知历史origin不通过当前全局URL猜测，需S5明确恢复。普通未绑定legacy v50路径在过渡期仍按现有guard和配置运行，不因新原生入口未开放而全部停用。

## 双模式执行与工具

Git模式：OpenRun后创建一个私有resource session，按冻结target/source/context产生受控route；分别用该reference凭据取固定commit，所有工作区合计沿resource合同。target和source不同不共用一个远端token。需要的对象格式在初始化bare前由固定SHA/provider一致性确定；GitHub仅40，GitLab可40/64，混合base/head格式拒绝。fetch后cat-file验证预定commit，缺失/浅历史不能假完整。

保留*GitRepository及原Search/Read/List/history/blame/metadata工具。给它新增私有command executor委托，把所有command（含本地读）纳入session fence、前后扫描、deadline/输出控制；未提供委托的legacy/standalone保持原gitCommand。Metadata设置为固定native provider reader，CodeOwnersProvider准确返回github/gitlab。不能继承远端repo config、hooks、filters、submodule递归或任意外部helper。

API-only：nativeAPIRepository使用固定两侧reader，GitHub复用已经验证的对象逻辑并换同账本transport；GitLab目录按固定commit+path分页、blob按OID/raw取数，不把tree SHA当commit参数。unknown root OID不用于剪枝；401/403/404/分页不完整不能变成“entry不存在”。两树目录/rename/metadata与有界LCS按对应合同，最大301变化测试不依赖compare.files。symlink仅metadata与明确文本缺口，ReadFile拒绝把它当regular文件，禁止解引用；gitlink同样不访问外部仓库。

Git/API所有读取前后复查session、全部snapshot ACL/凭据；缓存命中也不能跳过。EinoAuditor及分组/验证/图表工具拿同一session sources。resource/网络/匹配超限或未知目录覆盖必须流向原CoverageNotes/最终incomplete，不能创建一个不被最终状态消费的新warning集合。

## Git网络gate：正式组合与边界

采用已经实际验证过的组合：本地受控reverse bridge持有远端token并固定route；Git HTTP侧强制forward proxy只允许该bridge authority，拒绝CONNECT与其他scheme/authority。两个listener都只绑定回环地址，按session创建/关闭，不建立通用代理或启动反向隧道。

- 本地capability随机32字节，constant-time核对，只在子进程受限环境的URL-scoped header中，不在argv/URL/FETCH_HEAD；远端token只留Go侧。route随机ID绑定一个不可变RepositoryReference和经过核对的clone地址，不能由模型传入新的上游URL。
- proxy只接受absolute HTTP请求，authority/Host必须精确匹配已注册bridge，拒绝userinfo、fragment、RawPath、异常端口/转义和矛盾Host；无CONNECT。只允许GET info/refs?service=git-upload-pack及POST git-upload-pack，route/path/query/content type匹配bridge allowlist。不得用path.Clean把恶意路径修成合法请求。
- bridge同样验证method/path/query/local capability和当前scope；只向该reference的固定HTTPS clone端点发请求。GitHub api.github.com唯一映射github.com；Enterprise必须/api/v3或根地址与同authority clone路径一致；GitLab origin为规范根地址。同provider远端name/ID/clone URL不一致拒绝，不对任意clone URL发token。
- Go上游复用受限DNS/IP allowlist、TLS、无环境proxy、无重定向的transport。只构造所需Authorization、Accept、User-Agent、Git-Protocol及Content-Type，不透传Host/cookie/connection/proxy auth/任意extra header。TLS/API错误及Git stderr均脱敏分类，provider正文不进trace/模型/通知。
- Git环境完整白名单，不继承NO_PROXY/no_proxy、HTTP(S)/ALL_PROXY、GIT_CONFIG_*、GIT_EXEC_PATH、HOME配置/SSH agent/trace；只由session生成必要GIT_CONFIG项和本地cap。命令固定protocol.allow=never、HTTP/HTTPS allow、redirect=false、URI/bundle显式关闭、无凭据helper/maintenance/gc。额外URI仍必须经过proxy；不能仅依赖关闭协商。
- 实际上游dispatch前扣请求，POST上传和响应实体读取前取持久byte grant；本地proxy→bridge不是第二次远端请求，不双扣同一payload。非200响应同样受bytes/时间约束，不无限drain正文。stream每次读前后fence，不能返回已撤权结果。
- read header5s/header16KiB、请求体1MiB、连接空闲5s；每次HTTP总期限不超过剩余session/命令期限。listener关闭先拒绝新请求，取消并等待已有请求，再清理；不自动fallback远端直连。

此组合不是OS网络沙箱；前提是受信Git二进制、不可被请求修改的环境/配置和当前有限协议集合。d8a0047已覆盖2.39.5/Apple2.50.1及NO_PROXY/FTP/file/host变体，正式handler还须补userinfo/Host/path解析、全部route/预算/撤权反例。没有这些不能解除guard。若为未知Git版本声明支持，需同类能力验证，不从版本号字符串直接假设保护成立。

## 辅助读取操作：结束后查询不复活审计账本

scope预览、用户CODEOWNERS建议、发布前PR身份核对、poll枚举和诊断，分别创建独立AuxiliaryRead，不重开closed run budget。服务端purpose固定，不由HTTP选择廉价purpose读取任意仓库。建议固定profile：scope preview512 requests/32MiB API/沿原Git额度；CODEOWNERS32/4MiB API、blob256KiB、30s且API读取；publication16/8MiB API/30s、零Git；poll每项目每轮16/8MiB API/15s、最多1000 PR；diagnostic8/1MiB API/15s、零Git。超限明确失败或分页不完整，不误记poll基线。

这些profile需在schema3预算表新增purpose TEXT必填（audit/scope_preview/codeowners/publication/poll/diagnostic）、related_run_id INTEGER FK可NULL：audit根仍由root_run_id唯一绑定；其它purpose禁止Attach，不进入run→audit预算映射。limits_json按purpose/version严格验证对应公式；低于单blob/响应上限时取min，不能保留大grant把小操作当无限额度。actor/project、owner lease、session_epoch、reservation和close/abandon沿原合同；非audit清理也须在lease/请求结束至少7天后。profile数字是待集成验证的版本化上限，不能说已实测最优。

AuditPolicy.RepositoryBudget仍只接受audit公式；辅助profile不写回历史policy。辅助op的run_id仍为NULL，related_run_id仅提供查询上下文、不授予Attach权限；owner_kind=admission，沿30s lease/5s心跳及更小deadline，到期关操作。它们的Git/upload额度除scope_preview外为0，text_match_cells/per_text_match_cells均0：scope preview只取得完整目录/metadata（Git --raw或API双树），不为显示可选文件计算文本hunk；其它查询也不运行LCS。单响应/并发等固定字段按min(既有值、profile API余额)/parallel=1验证；未知purpose及矛盾字段拒绝。相关预算数据字段以本稿和budget补充同时更新，不能只加purpose枚举。

用户viewer可按既有权限查询scope/owners，但不能获得operator入队权限；publication使用已授权outbox actor、配置revision及固定HEAD，并再次核对完整target/source/context。poll/webhook service principal只有显式启用的project与各reference凭据scope，不继承任意管理员身份。

## 消费者接入清单：不得仅接Submit

| 当前路径 | 新native行为 | 反例/保持边界 |
| --- | --- | --- |
| Runner.Submit / HTTP runs | Factory admission→专用原子入队 | 内部ID不冒充远端；未授权零内容读取 |
| Runner.execute / EinoAuditor | OpenRun提供Primary与全部ContextSources | 真实Git类型保留高级工具；模型前已验证完整目录 |
| Context runner / context tools | 从同session取得source，逐次Authorize | 撤权/cache/混合provider不会回退全局GitLab |
| retry/checkpoint/recover/finish | v51验证scope+映射+原根预算+owner/epoch | 重启/child不重置消耗；终态写保持原worker fence |
| followup/Scope | 旧固定HEAD的范围查询用scope_preview，明确新root审计 | 不把旧任务的SELECT files请求偷偷记成新HEAD复审 |
| Slack status/reaudit | status只读已授权结果；reaudit走正式固定观察/准入 | 不能让v51版本校验走旧Enqueue绕过ledger |
| CODEOWNERS/owner候选 | session内共用root；run结束后的查询用codeowners op | 建议不授予权限，不自动发负责人通知 |
| poll | 各provider有界列opened PR；baseline持久去重后逐项Factory admission | 枚举不完整不标初始化完成，失败不mark seen |
| webhook | 验签/幂等/内部绑定映射后Factory admission | payload URL/remote ID不能直接授予项目访问 |
| comment/check dispatch | Factory publication op取当前固定provider写适配器 | 不用全局GitLab credentials发布GitHub结果；unknown不盲重发 |
| diagnostics / Settings启动 | C3 bind激活、同factory验证配置与能力 | 没有token不远探测；测试发送仍需用户主动触发 |

DynamicRepository/DynamicAuditor保持legacy兼容，但所有有scope的调用禁止落入其NewGitLabRepository路径；新工厂调用者不通过它们“临时适配”。现有代码中的NewGitLabRepository调用点须逐一分类为legacy/standalone或已替换native，不能用只搜索factory定义证明接入完成。

## GitHub/GitLab后台和发布范围

GitHub webhook用独立已配置Secret验证原始有限body的HMAC-SHA256和delivery ID；GitLab保持既有token验证并核对绑定仓库ID。新native hook路径须显式包含project和binding revision，未知/旧revision拒绝；不把未配置secret的保存状态宣传为Webhook已接通。opened/reopened/synchronize对应新观察准入；closed/merged记录PR状态与去重依据，不把发现自动判fixed。事件receipt唯一键为provider/project/binding_revision/delivery ID，bounded state记录received/enqueued/rejected及run ID；未经验证不写可信receipt，不在失败后伪造成功。payload不存完整源码/密钥。

GitHub摘要使用PR issue-comment协议，检查使用Check Runs协议；不悄悄用commit status的success替代无法表达的neutral/cancelled。凭据缺少对应写权限时诊断与发送明确失败，默认发布/阻断沿原opt-in。沿既有持久outbox、安装namespace标记、发送lease、latest-run仲裁和unknown语义；native适配器通过冻结reference选择凭据。创建前与更新前复查权限；远端创建响应丢失不自动再次创建。所有状态绑定完整HEAD，不能声称旧HEAD的已派发检查覆盖竞态产生的新HEAD。GitLab既有publisher保留legacy路径，native改用remote ID与自己的冻结凭据。

资料核对：[GitHub Check Runs官方文档](https://docs.github.com/en/rest/checks/runs)的fine-grained token表要求Checks write，明确classic PAT/OAuth不能用于更新；总述与fine-grained token表范围需区别阅读。不能从“保存了GitHub token”推出能创建/更新checks，也不承诺已经实现GitHub App自动换token。该页当前示例API版本2026-03-10，现有read client固定2022-11-28；N5需单独固定实际写API版本并用对应schema验证，不能照抄新版字段却沿旧header。fork的pull_requests数组可能为空，发布回执按固定repo/head/远端ID校验，不以该数组为空推断PR不存在。[GitLab Repositories官方文档](https://docs.gitlab.com/api/repositories/)作为tree/raw路径依据；客户端受控协议与真实服务版本验收仍分别记录。

GitHub hook receipt schema、评论远端ID/marker解析和check映射仍须在对应后台/发布子计划给精确字段及受控协议样本后实施；本稿不会授权在缺这部分时直接开放所有native入口。UAR-002人工处置更新与UAR-004工单恢复仍是独立未闭合需求，不借新provider自动放行或重发。

## 失败、覆盖与最终状态矩阵

内部RepositoryFailure持有有限kind、phase（admission/catalog/text/tool/resource/accounting/publication）和经过规范校验的可选相对path；不持有原始provider error/body/URL/token。Error()只返回固定分类，反序列化/HTTP错误不接受客户端提供kind。只有下列有意路径映射为覆盖缺口，不把所有错误吞成Notes。

| 触发 | 准入/执行行为 | 结果消费者/重试 |
| --- | --- | --- |
| scope/budget JSON错误、身份/配置不一致 | 准入拒绝、零入队；既有run执行前发现为failed且不启动模型 | 明确重新配置/新任务；不得legacy fallback或补猜版本 |
| 权限/凭据撤销、项目停用 | 停止新请求与输出消费；授权有效的worker以cancelled结束 | 已验证checkpoint保留；不自动重试恢复权限，不把旧结果改为当前安全 |
| worker lease/epoch丢失 | 关闭本地session，旧worker不再终态写 | 留原durable checkpoint供现有Recover；不能用background context绕过worker fence |
| 固定commit/blob不存在、对象hash/返回身份矛盾 | 主PR固定对象/目录无法建立则failed，零模型 | 不跟随新branch/最新PR；已取得但未完全核验内容不入cache |
| catalog截断/分页不完整、共享请求/bytes耗尽 | catalog_complete=false，incomplete且零模型；诊断保留已核验目录片段 | 不返回空changes成功；不重置额度或跳过剩余目录 |
| 完整catalog中的单file binary/非UTF8/超大blob、hunk或cell耗尽 | 该file text gap，其他可信文件可继续；最终incomplete | 共享32M额度/byte charged保留；不得整片replace制造可用行锚 |
| resource阈值/扫描/容量/profile失败 | 主仓准备或catalog前失败则failed；模型阶段停止则保留证据并incomplete | 环境/预算类不进入temporary_network自动retry；cleanup pending独立诊断 |
| ledger/退款事务未知 | 立即停止下一请求/DP；没有结果则failed，有可信部分则incomplete | reservation保守charged，终态写也须合法worker fence；DB不可用保留checkpoint，不盲重放 |
| deadline / 显式用户cancel | timeout=incomplete；用户cancel沿已持久cancelled，worker不覆盖 | 任务内已消耗保留，不因Close退款；取消状态不等于安全 |
| 合法metadata-only / 未推断相似rename | 作为元数据/中性范围说明，不能仅因这些说明制造缺口 | 保留原metadata审计规则；不宣称gitlink内容或symlink目标已检查 |
| 普通工具查询可修复错误 | 沿既有工具反馈/证据修复语义，source-fatal错误仍停止会话 | 不用一次模型参数错误取消整个项目；未解决覆盖缺口仍进入CoverageNotes |
| publication read/write失败 | 更新outbox的failed/unknown/stale等真实状态，不改原run审计结论 | 结果未知不自动创建第二个远端对象；不得把failed检查“修复”为success |

Runner获取Changes前的错误分支须识别上述typed failure；不能只修改Audit结束后的auditCoverageStop。图表/验证/分组提交结果前都合并同一个session coverage集合；集合中每项有有限code与path/阶段，转成既有CoverageNotes安全文本并去重，避免某个成功分组覆盖全局缺口。检查继续消费run.status及实际CoverageNotes，默认不启用blocking。JSON run DTO、详情、SARIF properties及通知摘要复用这些真实状态；新budget字段在前端api.ts补readonly可选类型，但不展示未经证明的精确网络流量。没有新public gap enum，避免只有生产者没有消费者的字段。

受控验证必须同时断言：准入状态/行数、实际请求/模型次数、cache内容、run.status/CoverageNotes、check计算及导出都一致；只测错误字符串不能关闭该项。资源错误和源不可用不伪装temporary_network，真正上游限流/5xx仍按原有有限retry协议且共享根账本。

## 实施顺序、验收及回滚

| 顺序 | 文件/模块 | 必须交付的可验证行为 |
| --- | --- | --- |
| N1 | repository_budget_policy/store/reservation；schema guard；audit policy/version helpers | schema3停机迁移、严格limits/purpose、admission→run→retry映射；真实SQLite并发/重开/故障不重置，不开放绑定 |
| N2 | repository_native_transport/gateway；resource_session/scan；git executor委托 | 真实CGI两provider/多仓凭据、固定SHA、恶意URI/NO_PROXY/未知路径、超限/撤权/取消及遗留隔离；legacy回归 |
| N3 | github_pr_snapshot、gitlab_native_objects、native_api_repository/tree_diff/text_diff | 真实Git oracle支撑固定PR/树/raw协议，301变化、64位SHA、metadata/rename/无LF；禁Git PATH下API-only仍成功 |
| N4 | repository_factory/admission/session；Runner/context/retry/followup/Slack/CODEOWNERS | 登录HTTP提交→真实controlled repositories→模型fixture→证据/覆盖/结果/重试整链；任一consumer不绕回legacy |
| N5 | provider poll/webhook、comment/check adapter、diagnostic与C3装配 | 精确后台/发布子合同先审，HMAC/重放/乱序/未知发送/权限变化；实际外部请求计数与固定HEAD验证 |
| N6 | S5身份恢复、S6现有UI和API文档/正式assets/CHANGELOG | 完整create→audit→恢复及窄屏/权限/409/响应丢失验收后，才解除现有管理只读gate |

N1–N3可内部提交，但不计用户功能完成，不发布入口；N4是首个受控完整执行场景，N5/N6完成才可交付原生能力。不是允许以后留下未接入模块：每片验收记录明确producer/consumer证据与剩余依赖，完整目标保持。

验证按变化范围先专项，再全Go/race/vet/frontend/build及精确HEAD CI；模型/托管平台/邮件/机器人真实账号不在当前自动授权内，使用本机合成HTTP/TLS/CGI、临时SQLite及确定性模型fixture。不能把受控协议测试当真实GitHub/GitLab账号验收；发布前单独记录外部证明缺口。

schema3需要停止旧worker/HTTP并备份DB；旧程序应拒绝新schema。回滚恢复一致DB/程序备份，不降低版本或删除身份/预算。管理入口当前501保持到N6，不新增开关让管理员绕过factory readiness。每片按feature-lifecycle记录文档、提交和推送；不自动合并main、部署或发布。

## 正式审查前尚需补齐

本稿和N5补充已定义工厂、版本、消费者、错误矩阵、receipt/发布目标字段与协议；GitLab局部wire证据已取得。完整方案仍需技术评审，特别检查admission seed跨崩溃恢复、两个outbox与目标generation、context内容边界、schema3和v50/v51的完整传播；评审明确Pass/Fail，不凭局部探针或本文件长度批准开发。Linux逐进程增强暂缓，基础profile不能被文案夸大。真实GitLab版本和外部写账号验收在后续验证阶段单列，不能拿fixture代替。

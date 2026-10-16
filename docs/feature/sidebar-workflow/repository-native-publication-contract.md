# 原生Webhook与发布合同（N5作者稿）

2026-10-16；codex/sidebar-workflow；输入Confirmed requirements.md、native execution/budget/resource合同和现有comment/check outbox。P6/F2，跨HTTP、Store、worker和外部写；本稿只设计，不发送真实消息/检查、不部署、不开放管理入口。所有新表属于尚未实施schema3，完整方案仍待技术评审。模块拆分native_hook_http/store/worker、native_publication_target/store、github_comment/check协议；原Runner只委托，不将验签/SQL/协议混入一个文件。

## 对外入口和验证顺序

NTR-006作者修订见[普通GitLab切换合同](repository-legacy-transition-contract.md)：以下表格中的仓库键api_origin改为EndpointKeyV1的endpoint_key；所有新v51发布（含普通legacy）统一target协议。OwnedContentV1共同仓库字段同改endpoint_key；尚未生产的version1投影直接采用该规范，不接受两种字段二选一。legacy历史outbox保持NULL仅指旧v50，不能据此给切换后的新任务走无scope发布。原webhook的receipt source/authority union/去重scope按补充合同传播，原生路由仍只接受绑定项目。合同待复审，当前不启用生产。

`POST /api/v1/repository-hooks/:project/:binding_revision`为native接收入口，旧`/webhook`继续只服务可证明的legacy GitLab。project/revision为规范正十进制；路由不含token。body最多1MiB，读超限413；只接受application/json，无Content-Encoding，重复/矛盾身份字段拒绝。读取当前启用项目/绑定和对应凭据revision，但未验签前不发远端请求、不写可信事件。

GitHub使用原始body的HMAC-SHA256，严格sha256=64hex并constant-time核对；Secret复用服务端IntegrationCredentials.Secret，空值表示未配置Webhook。只接受pull_request事件及opened/reopened/synchronize/closed action，核对repository.id、PR号、base repo和source/head身份。GitLab核对当前集成Secret对应X-Gitlab-Token，event/object_kind/action与绑定project ID，不能用全局legacy secret接受native任意仓库。签名/secret错误统一401，不返回仓库是否存在。

GitHub X-GitHub-Delivery必须为规范UUID；GitLab优先规范X-Gitlab-Event-UUID，旧版本缺失时用body digest作delivery_key。传入header本身不是权限依据；HMAC签body不等于签delivery header，必须同时按原body SHA256去重，不能只防同delivery ID重放。action未知时验签后200 ignored，不触发审计或占用可信待处理队列。

HTTP只提交持久队列后返回202，body仅含receipt_id/state/可选run_id；不等待模型或远端PR元数据。重复合法事件返回原receipt，不新建预算/run；相同delivery key配不同body返回409。DB失败/待处理上限返回503，不假称已接收；每绑定最多32个非终态、100000个活跃版本receipt，超限给有限诊断，不删除去重证据换成功。

## 新增持久对象

关系型project/run/receipt/target/远端ID、revision和epoch为正int64；budget_id仍为64hex，namespace为既有32hex，claim_owner为服务器随机hex，不能混作数值。跨HTTP的新receipt/target ID用十进制字符串，避免JS精度损失；旧run ID保持既有兼容。时间为服务端UTC；无业务默认的必填列不以0/空值伪装有效对象。

| 表/字段组 | 类型/必须/来源 | 不变量与生命周期 |
| --- | --- | --- |
| platform_native_hook_receipts.id | INTEGER PK | 服务端生成，不作为认证凭据 |
| project_id/provider/api_origin/remote_id/binding_revision/integration_revision | INTEGER/TEXT/正ID，必须 | 由当前已验证绑定取得；provider有限github/gitlab；凭据更新不让旧job借新token执行 |
| delivery_key/body_digest/event/action | TEXT，必须 | key<=128、digest64hex；unique(project,binding_revision,delivery_key)及unique(project,binding_revision,body_digest)，跨integration revision保持去重 |
| envelope_json/envelope_digest | TEXT，必须，<=4096 bytes | 严格版本1：number、action、target remoteID、source remoteID（closed可0）、完整base/head SHA（closed可缺）；不存body/标题/URL/源码/secret；digest验证数据库内容未被误解释 |
| state/code/run_id | TEXT/TEXT/可NULL FK | pending/processing/admitted/ignored/failed；有限code；admitted必须有run_id，closed/ignored禁止伪造run |
| budget_id/attempts/claim_owner/claim_until | 可NULL FK/TEXT/INTEGER | pending首次claim前无budget；最多3次claim，30s lease/5s心跳，旧owner不得落状态；budget一旦绑定不可替换 |
| created_at/updated_at | TEXT，必须 | 不按payload时间排序授权；终态重复请求不重新激活 |
| platform_native_pr_states | project/provider/origin/remoteID/number复合PK；state/open-closed-merged；observed_head/observed_at | 只存当前远端核对结果，不直接按乱序payload改状态；不改变finding为fixed |

当前绑定版本终态receipt保留去重。只有该binding revision已永久失效（单调版本不会重用）且所有job/预算结束至少90天后，才允许整组归档/删除旧版本receipt；不能仅按时间删除当前版本去重。32/100000为初版有限容量，不代表吞吐最优；处理满额需要诊断与明确配置版本运维，不静默删表。

## Worker、准入与恢复

```mermaid
flowchart TD
  A[有限body和当前配置验签] --> B[原子receipt去重入队]
  B --> C[202 接收成功]
  B --> D[worker claim 有界租约]
  D --> E[冻结AdmissionSeed并绑定同一budget]
  E --> F[当前PR身份/HEAD/状态核对]
  F -->|已过期事件或closed| G[记录核实状态 不推断修复]
  F -->|当前可审计| H[Factory固定观察与全部授权]
  H --> I[单事务 enqueue/查重 + attach预算 + receipt回执]
  D --> J[有限恢复 原预算不重置]
```

首次claim同事务创建budget并保存AdmissionSeed，之后才能远端读取。Seed为内部严格版本1、<=64KiB的非秘密JSON：冻结核心AuditPolicy（此时repository_scope尚未完成）、target/source/context references及context集合digest、service principal、预期number/base/head/source ID。所有reference来自本地已授权绑定，不从payload URL构造；缺少source绑定的opened类事件直接failed，零source内容读取。存入budget新字段admission_seed_json/digest；没有seed的预算不能由hook重领。closed仅需要target元数据，使用poll辅助profile，不Attach审计run。

worker重新取PR/MR：payload head/source与当前不符则ignored/stale，不审计错误旧提交；closed事件也先查当前状态，不用晚到的closed覆盖已reopen。opened/update在同HEAD同policy已有run时，Store查重返回该run并同事务更新receipt，当前admission消耗保留且关闭，不给旧run追加额度。基线/合并去重只表示PR处理状态，未再次观察不自动等于修复。

临时限流/5xx/网络错误最多3次claim，均使用同一root counter/limits/seed；新worker只在原admission绝对deadline尚未到、旧owner lease已失效时CAS新owner并递增session_epoch，旧owner不能Attach/读/退款。audit绝对窗口仍min(policy timeout,60)s，poll按其15s；恢复不延长窗口，不把未知reservation退款。普通HTTP同步准入不向外暴露budget重领权；RecoverHookAdmission只能经持久receipt精确关联和当前worker实例调用。修改budget合同的这一例外必须与实现一起审查，不能只更新job state却遗忘ledger。

每次retry先结束当前只读attempt，原子把job从processing移回pending并失效旧admission owner/epoch；attempts在下次claim增加，等待Retry-After不得超过原绝对deadline。root仍admitting且绑定非终态hook时，通用budget recovery不得仅因lease过期先标abandoned：应交给受限RecoverHookAdmission，或者deadline/配置失效时同事务终结job/root。出现active/root_run_id非NULL而receipt非admitted是违反原子协议，诊断失败而不是重领。此顺序进入Store.Recover和worker启动测试，不留两个互相竞争的清理器。

seed/context集合、binding/integration revision、权限、凭据代次任一不再匹配即failed，不能用新配置默默替换。预算耗尽、绝对deadline或claim次数耗尽是明确failed，不新建root无限重试。用户可经现有审计提交入口明确发起新操作，或由已启用poll发现仍未审计HEAD；终态receipt重放不再次自动入队。S6在现有绑定详情/诊断显示最近receipt状态、PR号、有限code和run链接，失败提供预填提交入口（仅导航，不自动POST），并区分“接收成功”和“审计成功”。

管理端集成DTO新增可选只读has_webhook_secret（github/gitlab由c.Secret是否非空计算）；既有has_secret包含Token，不能用它推断Webhook已配置。旧响应缺该字段显示未验证，保存时客户端不得提交该派生状态。GET /projects/:id/repository-hook-deliveries?page/size为现有配置内的诊断数据，operator及以上且当前项目授权，size<=20，按id倒序，返回receipt十进制字符串ID、state/code、PR号、created/updated、可选run_id；不返回digest/envelope/claim/budget/secret/原payload。列表/总数同只读事务，权限/错误/空/加载/分页按现有页面模式处理。不是另加空菜单。

Store的native enqueue与receipt.admitted/run_id/claim fence必须同一个SQL事务；崩溃不能出现已入队却无法通过receipt找回的第二次创建窗口。seed、关联budget或receipt冲突全部失败，不通过查猜最大run ID修复。手工/并发poll查重仍由既有完整snapshot+policy digest唯一性承担。

## 摘要与检查的持久目标

现有outbox以run为主键；native不能因此为每次同HEAD审计都创建另一个未更新的检查，或让旧摘要覆盖新运行。新增platform_native_publication_targets，已有两个outbox增加nullable native_target_id/native_target_generation；legacy行保持NULL和原协议，不给旧remote_id换provider。

| target字段 | 类型/必须/来源 | 不变量 |
| --- | --- | --- |
| id/kind | INTEGER PK；comment/check | 服务器产生；kind不是客户端开关 |
| project_id/provider/api_origin/remote_id/number/namespace/head_key | 当前授权远端身份；namespace现有安装随机值；head_key comment为空、check为完整HEAD | 复合unique；binding/credential revision变更不丢弃同远端对象身份；远端repo变化使用新目标 |
| desired_run_id/generation | FK/正int64 | 同PR最新run仲裁后CAS更新；generation不回退；native outbox绑定该generation；不覆盖claimed字段 |
| claimed_run_id/claimed_target_generation/claimed_outbox_generation | FK/正int64，可NULL且成组 | claim前NULL；发送前同事务冻结当前run、target代次及per-run outbox代次；sending/unknown期间不可被新desired覆盖 |
| state/remote_object_id/remote_author_kind/remote_author_id | uncreated/ready/sending/unknown/conflict；可NULL正ID；user/app | 未ack前不伪造remote ID；GitHub检查记录真实回执app ID，评论记录真实author user ID；不能仅凭公共marker认领别人对象 |
| projection_version/ack_hash/attempted_hash | INTEGER固定1/可NULL SHA256 | 哈希对象为下文OwnedContentV1，不是HTTP JSON bytes；attempt含冻结run/generation/HEAD，unknown期间保留 |
| expected_author_user_id | 可NULL正int64 | GitHub评论首次写前若GET /user已核实则保存；NULL不授予unknown创建自动认领能力，不能从候选反推 |
| claim_owner/claim_until/updated_at | TEXT/UTC | 与per-run outbox claim同事务；发送期限20s、租约60s；过期sending变unknown，不能直接reclaim为一次新创建 |

claim时同时锁目标与outbox、复查当前desired generation/全部snapshot权限及配置。网络在事务外，写前再次fence；已dispatch后失去权限/代次时记录unknown，不称“未发送”。native caller使用冻结reference的remote ID/凭据，不走全局GitLab client。已知remote object GET确认身份后才更新；404/作者或marker矛盾是conflict，不自动另建。

### 冻结attempt与新desired（NTR-003修订）

native outbox增加native_claimed_target_generation（nullable正int64），与现有claimed_generation（该outbox的本地代次）分开；claim同时复制到target.claimed_*，freeze attempted_hash及projection version，然后才允许外部写。另增private native_claim_token（nullable随机32-byte hex），每次发送/只读reconcile claim新生成，target.claim_owner使用同token，不能复用Runner实例owner；worker实例租约仍另外验证。两个outbox及target同事务比较该token，防止同实例、同attempt重领后的迟到回调借新租约通过。

queue新run/同run新复核只更新desired及native_target_generation；不修改active claimed_*、attempted_hash或native_claimed_target_generation。target sending/unknown时不能为新desired另起POST；只读reconcile可换claim token但不能换原attempt三元组。legacy行native字段NULL，原claimed_generation/claim_owner协议不变。

Finish/只读reconcile必须匹配冻结的claimed run/target/outbox三元组、当前claim owner/租约和attempted hash，结算的是原attempt。新desired已经推进时，旧attempt仅更新自己的已发送回执和target最后ACK，不能标新desired完成；随后只为仍有效的最新desired排新pending。若原outbox已有更新desired_generation，只推进其sent_generation到旧claimed值，不能设sent_generation=最新值。unknown保留三元组供新reconcile owner读取，新owner不得重新生成旧attempt内容或把它改成最新run。

target只有在原attempt已ACK/可证明只读对账后才回ready；未确认的更新可能迟到，保持unknown并阻止新写。丢失owner的迟到回调不能覆盖新claim；不能因当前desired与原attempt不同而丢弃旧unknown再新建目标。测试A正在发送→B排队→崩溃→A回执/GET对账，只允许A完成，B仍需自己claim。

### OwnedContentV1：对账对象（NTR-002修订）

使用固定字段顺序的内部struct JSON、UTF8、SetEscapeHTML(false)、去Encoder末尾单个LF后SHA256。projection<=128KiB，strings禁止NUL/非法UTF8；仅renderer首次生成时规范输出LF，**对GET返回的body不再次换行/空白/Markdown归一化**。响应JSON字段顺序、额外id/app/timestamp不参与哈希；缺必须字段拒绝，不用zero-value假成功。未来version未知拒绝，不借新版本另建目标。

共同字段：version=1、kind、provider、api_origin、remote_id、namespace、project_id、number、head_sha、run_id、generation，类型为整数version及其余规范string（ID为正十进制、SHA/namespace按固定长度小写hex）。target的固定repo身份来自已验证请求上下文；其余身份必须由remote marker/external_id精确解析并与冻结claimed三元组匹配。不是从任意候选字段自动信任身份。

- comment额外字段仅body(string，含首行精确marker及完整正文，<=32KiB)。marker为<!-- aimangebot-native:v1:<namespace>:<project>:<number>:<run>:<generation>:<head> -->，各段不能包含分隔符/额外空白。producer用该body构造POST/PATCH {body}；GET从原始body解析相同marker并构造projection，另核对comment ID、author及issue_url属于被冻结repo/PR。额外HTML渲染/反应数不参与hash。
- check额外字段name/status/conclusion/details_url/output_title/output_summary/output_text。name固定安装/PR上下文，head_sha必须匹配目标；external_id使用同一marker字段组的无HTML前缀字符串。queued/in_progress的conclusion为null，completed必须为有限合法值。details_url/output_text在意图本来为空时允许远端缺省/null等价为空；其它必须字段缺失拒绝。GET从external_id、head_sha及output构造projection；create body可带head_sha而PATCH省略它，二者仍能比较同一个拥有内容。started_at/completed_at不参与拥有内容hash，不因服务端时间格式规范化破坏对账。

producer意图、ACK与GET使用同一projection函数和版本。每个ACK先验证HTTP状态、ID/repo/HEAD/author或app及owned字段，再存ack_hash；unknown对账只能将remote projection和已持久attempted_hash比较。不能把整个远端响应JSON hash当作原请求hash，也不从当前新reviews重建旧attempt再称为已发送内容。语义字段任何差异保持unknown/conflict；纯额外响应字段/JSON重排不应改变结果。

### 首次创建的作者依据（NTR-004修订）

GitHub评论在首次intent前用当前冻结token GET /user，结果必须是可核实正ID，成功则在目标存expected_author_user_id；后续credential revision变更时也须重新查询并与已有expected/ACK作者一致，不能默默切换作者认领旧评论。此GET计入publication读取预算，错误正文不得返回。

某些token种类不能通过该接口核实主体时，明确记录作者未核实：可以在用户已启用发布的前提下进行一次有界create，只有完整201回执中的正ID/准确body/author可作为ACK后身份；**创建回执丢失且expected_author_user_id为NULL时禁止列表自动认领**，保持unknown供人工核对。不能从列表第一个匹配marker的author设置expected再自证。若expected已存在，则首次201作者必须相同，否则unknown/conflict，不能覆盖expected。

Check首次创建的app ID同样不能从公共external_id推导；完整ACK可记录真实app ID，未知创建且无先验可信app身份保持unknown，不为“恢复闭环”假装支持所有token/App类型。已有可信remote ID的GET更新对账仍须匹配原ACK app ID。S6诊断分别显示读能力、写权限、作者已核实与结果未知；has_secret/token保存不代表这些状态已通过。

unknown create没有可信remote ID时不自动再POST。评论仅在完整有界列表中找到唯一“同已核实author+精确namespace/PR+attempted body hash”的对象时可只读对账；无/多候选或不完整分页仍unknown。Check Runs若没有可核实app身份，公共external_id/name不足以认领，保持unknown并人工核对。已知ID的unknown更新可读核对精确attempted hash后确认已应用；仍为前一ack hash时不能证明迟到请求不会应用，因此不盲PATCH。没有显式可靠恢复语义的路径保持禁用，不增加“强制重试创建”按钮。

## GitHub协议和内容边界

写API固定2022-11-28，和现有read client一致；每条请求明确Accept/API version，实际支持与权限由诊断/受控协议及真实服务验收分别证明，不能只看token保存成功。GitHub summary使用issue comments：GET列表/GET已知comment、POST /issues/:number/comments、PATCH /issues/comments/:id；不是inline review comments。使用原body marker理念，精确绑定安装namespace/目标PR及run generation，comment正文最多32KiB，只发送已验证主仓发现摘要和平台内部链接。

**有context仓库时只允许通用任务状态与工作台链接**：不外发发现数量、描述、代码、来源仓库/路径或内部引用；检查conclusion仍按已授权发布策略表达通用结果。保留现有跨仓publication guard的数据边界，不能因为publisher本人能读所有仓库就认为PR的所有读者也有权限。无有效PublicURL且无法产生可用安全链接时明确blocked，不发送不可定位的占位摘要。中性说明、缺口与失败不得写成“安全”。

check name固定aimangebot/<namespace>/project/<id>/pr/<number>，external_id为有限目标ID+generation+run+完整HEAD，创建/更新前后都核对repo/HEAD/name/已知app ID。POST /check-runs首次创建，后续同HEAD已知ID PATCH；返回201/200及完整身份、期望status/conclusion才算published。old generation不能更新当前目标；不同HEAD不会复用ID。回执pull_requests为空不直接视为错误，fork场景按实际repo/HEAD验证。

| 本地期望结果 | GitHub status/conclusion | 解释 |
| --- | --- | --- |
| 队列/执行中 | queued/in_progress，conclusion省略 | 仅有真实对应运行才发布 |
| 成功且阈值风险0 | completed/success | 不宣称证明绝对安全 |
| advisory有风险、skipped | completed/neutral | 不用success掩盖风险或未审计 |
| blocking阈值命中 | completed/failure | 与现有checkStateForRules的阈值一致 |
| failed/incomplete | blocking且相应BlockOn开关为true则failure，否则neutral | 正文始终保留失败/覆盖不足，不转换成“通过审计” |
| cancelled | completed/cancelled | 不等于安全或已修复 |

人工fixed/false_positive不自动改远端门禁，UAR-002仍待独立语义确认。token只有read或classic PAT无法写Checks时显示权限/能力失败，不偷偷换commit status协议；不承诺GitHub App自动签发/刷新凭据。原GitLab publisher在native下使用正确source/target remote ID，保持MR pipeline/精确SHA规则；不能借GitHub映射改变legacy行为。

## 实施与证明

N5实施须同时加入schema3迁移、严格seed解码/ledger重领、hook queue及worker、publications target+两个outbox fence、GitHub协议和现有诊断/详情投影；新字段从producer到Store/DTO/TS/测试完整传播。清理旧receipt必须证明binding版本永久失效，不在N5自动删除当前namespace发布对象。回滚恢复一致DB与程序备份，不能清空unknown或删目标来恢复旧版本。

受控验收至少包括：验签前零外部/可信写，body上限，换delivery ID重放、同ID异body、跨revision与乱序closed；接收响应丢失、claim崩溃、admission已commit但HTTP未回应、3次/原deadline/同根额度；同HEAD同policy仅一run、counter不重置；两run争一个目标、迟到旧lease、unknown创建/更新、错误author/app/marker、分页不完整、403/5xx/201坏回执、无checks权限、fork空pull_requests、所有中性/失败状态、context摘要零内部内容及PublicURL缺失。

上述是待实现验收，不是已运行测试。本轮GitLab对象探针仅证明SDK/wire/hash/分页边界，不证明这些后台和外部写协议。当前管理写gate继续关闭；完成全S4评审和受控端到端、S5恢复与S6 UI后才开放，不发布真实评论/检查或自动合并。

资料：[GitHub webhook最佳实践](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)、[issue comments](https://docs.github.com/en/rest/issues/comments)、[Checks](https://docs.github.com/en/rest/checks/runs)、[GitLab webhook events](https://docs.gitlab.com/user/project/integrations/webhook_events/)。provider文档支持协议边界；seed、双键去重、目标generation和具体限额是本项目设计提案，不称为官方现成实现。

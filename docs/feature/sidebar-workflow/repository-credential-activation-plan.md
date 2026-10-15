# S3：仓库凭据激活实施计划

2026-10-15；codex/sidebar-workflow；作者计划，尚非实施/批准记录。输入requirements.md为Confirmed，repository-recovery-contract.md的最终凭据修订及第三轮F2 Pass、F3完整序列不变。前置S2已推edf563f9f38187c480e7014cb7689fe7e5a7dbed；精确CI38750868382仍运行，不能称S2远端验收通过。完整REQ/AC与UAR-001/002/004保留。

## 工作流与目标

P6→P7/P8，F3细化；使用technical-plan-write和feature-lifecycle。目标为仓库凭据变更时，旧运行不会借新凭据继续读取/发布，配置回退不会复活旧凭据版本。保持配置兼容：不引入客户端/YAML counter，不用全局Settings.Revision作为凭据版本。单独Store协议只是S3依赖，直到Settings/启动/旧reader全部消费后才可称S3完成。

实现前需按本计划复审。当前允许docs与隔离探针；不根据研究脚本直接开放恢复或native执行。新scope正式producer与PolicyVersion切换仍在S4。

## 当前代码事实与兼容边界

- SettingsService保存次序为mu→校验/密钥空值保留→临时文件写/fsync/close→rename→atomic current.Store；缺文件/DB跨持久层激活阶段。
- main先OpenSettings/OpenStore/Bootstrap/ImportLegacy/恢复项目outbox/导入项目与context，再建DynamicRepository、Runner.Start和HTTP。激活必须在任何worker/HTTP/远端读之前完成；outbox项目同步只能改Projects，不能旋转凭据代次。
- 旧GitLab config只有URL/token，没有AllowedNetworks；validURL允许http/https与路径前缀。NewGitLabRepository实际BaseURL是TrimRight(URL,"/")+"/api/v4"；不能直接复用native绑定的HTTPS origin校验，不能丢路径或把两个不同前缀视为同仓。
- upstreamHTTPClient包装http.DefaultTransport，使用默认代理规则；不是新集成的DNS/CIDR受限transport。S3不能伪称旧HTTP已受native网络白名单控制。新Git/API网络策略属于S4，切换时须改变transport policy版本。
- Store.SyncProjectConfig读取项目事务已结束后才调用Settings.save，没有持DB事务反向拿Settings锁；必须保持该顺序。

## 对象与字段

| 对象 | 字段/类型 | 所有者/规则 |
| --- | --- | --- |
| 已有私有state | generation int64；key/fingerprint []byte | Store；generation0仅未激活，key32bytes且不重建，fingerprint NULL iff generation0；持久列严格typeof校验 |
| activationMaterial（新增私有） | version/transportPolicy字符串、effectiveBaseURL字符串、token精确bytes、network CIDR排序去重列表、effectiveProxy字符串 | 由实际client builder构造；固定struct JSON后HMAC；从不HTTP/事件/模型/错误输出。当前legacy没有CIDR字段，列表必须空，不借用通知配置；代理值可能含密码，只作为私有指纹输入 |
| activation state（新增私有） | phase unavailable/pending/active；epoch uint64；active descriptor不可变指针 | Settings配对激活器；epoch为进程内防ABA标记，不持久化、不被客户端设置；溢出拒绝；不取代DBgeneration |
| descriptor（新增私有） | config不可变副本、generation int64、epoch uint64、effective transport policy | 运行只捕获已激活描述；同次操作前后必须与当前/DB一致；token仍来自Settings，不另建凭据库；不序列化descriptor |
| scope已有字段 | legacy_repository_revision 精确十进制字符串 | 保持S1联合类型，无新JSON字段；旧nil scope不补造历史generation |
| 失败类别（新增有限error） | activation_pending / activation_unavailable / generation_exhausted | 固定不含配置原文；HTTP新增码在消费者实施时统一传播；当前通用400+err.Error不可直接用于底层文件/SQL错误 |

material应绑定真正生效的endpoint和transport，而不是任意配置文本。旧路径有效BaseURL保留path大小写与转义，不清理会改变请求目标的部分；去掉仅由现有构造器忽略的尾斜杠。主机、默认端口规范化必须使用与client实际构造完全相同的helper；不确定等价的URL宁可保留区别，不能错误合并身份。HTTP旧配置继续可用，但不因此生成要求HTTPS的native scope；将来scope准入需明确诊断不支持的legacy endpoint，不能静默降级。

默认代理输入由与实际transport相同的ProxyFromEnvironment对固定BaseURL解析得到；不额外读取或公开整份环境。进程内不承诺动态Setenv生效；启动使用当前有效代理重新激活。若后续换为显式代理/无代理/CIDR/dial策略，material协议和实际client必须一起升级，不能只改指纹版本。

## 状态与文件/DB/内存顺序

```mermaid
sequenceDiagram
  participant W as Settings writer
  participant A as Activation gate
  participant F as Config file
  participant D as SQLite state
  participant R as Runtime
  W->>A: pending + epoch rotate
  A-->>R: reject new operations / old result consumption
  W->>F: atomic replace + durability check
  alt persistence definitely failed before replace
    W->>A: restore previous active state
  else file replaced
    W->>D: short tx compare keyed material / advance generation
    alt DB commit failed or durability uncertain
      W->>A: keep pending; fixed error
    else DB committed
      W->>A: publish immutable full Settings + generation
      A->>R: cancel old legacy runs; new descriptor available
    end
  end
```

新key仅S2迁移创建；S3 state loader/activator绝不INSERT OR IGNORE补行。BEGIN IMMEDIATE串行同material激活：未变保持generation，变化+1；A→B→A为1→2→3；MAX_INT64仅允许相同指纹。事务提交不明时不宣布成功，下一次从真实文件与DB核对。key/fingerprint constant-time比较，日志仅固定类别。

Settings.mu→activation写gate→短Store事务；runtime在短gate读观察后释放，再做DB观察/网络调用，不在整个远端请求期间持gate锁。完成前再次核对epoch/phase+DBgeneration+当前material匹配，期间改变即丢弃内容且不能进模型/缓存/发布。任何Store事务内不调用Snapshot/save/activation以免单连接死锁；网络前后ACL/lease检查仍独立。不得把operation前检查等同于撤回已经发出的请求。

Save先完成旧配置CAS、默认值/空token保留、完整校验和序列化，再进入pending。纯模型/项目outbox变更不改变material、不旋转generation；若当前pending，禁止这种保存覆盖已落盘但未激活的新配置。rename后文件持久性不明（含目录sync失败）不能按“保存未发生”清pending；DB成功/内存发布前窗口同样pending。失败仅暴露有限错误；已替换文件不能回写旧文件假装回滚。

启动在HTTP/worker前同步激活；失败则不启动服务，保留文件/DB。直接编辑YAML只在启动或明确reload时加载；不自动监控、不按每个runtime操作读文件。

## 旧reader与撤权消费者

- DynamicRepository、DynamicAuditor、Runner.Submit/execute的pinned client、context_repository_runner、polling、followup、CODEOWNERS、comments、check dispatch等必须改走统一activated legacy client helper；不能仅给DynamicRepository加gate遗漏直接NewGitLabRepository。
- 旧nil scope运行不生成伪历史generation；执行时捕获当前descriptor，旧policy.RepositoryURL必须与该有效endpoint兼容；凭据变更开始时取消旧legacy pending/running，防止排队后借新token重新开始。S4有scope后按逐credential匹配精确generation，mixed run中任一legacy reference受影响即取消。
- gate pending时禁止新远端操作及结果消费；旧运行取消失败也不能放行。取消持久状态和同进程context.Cancel都必须做，跨连接的下一次派发/消费仍靠gate/DB复查。
- 含纯integration凭据且无需legacy的未来运行不因legacy变更停用；身份、integration revision、ACL和lease继续逐仓校验。旧global配置不代表所有仓库共用token。
- HTTP配置查询只展示active/pending/unavailable及有限code；不新增菜单。pending恢复必须在已有配置页有下一步，不能只实现错误码。

### 消费边界的具体落实

只在repository方法返回时检查不足：凭据可能在内容返回后、模型HTTP派发前改变。S3c须将同一个只读fence传入AgentConfig和所有主审/复核/压缩model builder；使用现有modelStartCallback与checkpoint作失败传播，同时在实际模型HTTP RoundTripper派发前检查，并在模型Generate返回/结果提交前复查。model_usage.go、primary_finalization_model.go、verification_agent.go、claim_verification_agent.go及agent_compression.go属于检查范围；不能只检查首次prompt或主审，遗漏独立复核/压缩。实际HTTP客户端与合成模型server断言撤权后新模型请求0；已发请求按合同不能撤回，但响应不能成为有效新证据。checkpoint是持久记录，不能冒充凭据授权。

material变更的持久generation更新与旧legacy pending/running取消必须在同一事务完成，提交后发布新完整配置、最后清pending，再调用同进程Cancel。不能先清pending再异步取消数据库旧任务，否则旧nil scope队列可捕获新descriptor继续。事务内不调用Settings或Runner；只收集取消ID，提交后通知Runner。文件成功但激活/取消事务失败保持pending；未变material不取消现有任务。pending开始时epoch fence立刻生效；失败回到原凭据时旧进行中操作仍因旧epoch被拒绝，不承诺失败保存不会中断正在读取的请求。

S3a material producer先以现有NewGitLabRepository构造客户端（构造本身零网络），取SDK Client.BaseURL().String()作为真实有效API前缀，再用相同endpoint求实际proxy。这样保留SDK既有斜杠/path行为，不另造一套URL规范化。仅支持现行legacy_http_default_v1传输策略、空CIDR列表；将来S4受限策略需新producer与policy版本，不通过函数参数伪装已经支持。token/proxy按UTF8精确编码固定struct JSON；非法UTF8拒绝，防止encoding/json替换字节导致凭据碰撞。S3a的Tx helper供S3b在同一事务追加旧任务取消；不强制调用无法组合事务的独立提交接口。私有material/state不得实现公开JSON导出，fmt诊断固定脱敏标签，错误不包装SDK或SQL原始正文。

服务以外的standalone一次性模式没有配置热更新/Store，不伪造generation；保留不可变配置单次执行，明确不属于平台多运行激活协议。测试替身仅通过显式构造注入，不允许生产启动缺激活器时静默使用旧client。

## 实施切片与证明

| Slice | 文件/责任 | 通过条件 |
| --- | --- | --- |
| S3a | legacy_repository_state.go + repository_credential_material.go + SQLite/纯函数tests | 私有state严格读取、规范material/HMAC、原子activate/validate，ABA/双连接同值/溢出/缺行/畸形类型/零秘密输出；不接启动、不旋转生产generation |
| S3b | repository_credential_activation.go + settings_persistence.go抽出原子写 + Settings薄hook + 启动assembly | pending与文件/DB/内存故障矩阵，目录持久性检查，完整快照一次发布；model/outbox不旋转；启动前激活；现有Settings保存CAS不退化 |
| S3c | activated_legacy_repository.go + runner/直接client消费者薄委托 + interruption + HTTP/settings消费者 | 无绕过reader；未激活/pending/撤权时下一网络请求与模型输入次数0；同进程和双连接变更都fence；公开错误/当前配置状态传播 |
| S3d | 同进程reload恢复/已有配置页反馈 + 浏览器/协议tests + docs | 按下方reload合同实现并验收；unknown持久化状态不被覆盖；完整Go/race/vet/typecheck/embed/精确HEAD CI |

S3a只是内部前置，不能用它宣称凭据撤权已接通。S3b/c/d的锁/消费者/恢复必须在各片开始前证明合同完备；S3整体完成需要所有行，不跳过难点。技术复审可只批准S3a明确边界，但须保留本完整目标并单独标出不能批准的部分。

验证需在合成配置和SQLite上故障注入：rename前失败、rename后DB失败、commit后发布前中断、重启重算、恢复旧文件、两个连接同配置/不同配置、MAXgeneration、缺key/坏row、模型/outbox同步不变、真实阻塞HTTP响应期间改token后结果不能进模型。外部发送次数与模型输入次数必须断言，不能只检查error。真实部署仍需配对备份演练；当前不调用真实模型/发送服务。

## 同进程reload恢复合同（拟议API，尚未实现）

在已有配置页内增加状态和恢复操作，不新增侧边栏入口。GET现有settings响应增加`repository_activation:{state,code}`，state只允许active/pending/unavailable；code是有限白名单，不暴露generation、epoch、HMAC、文件内容或路径。UI仅在pending/unavailable展示恢复说明，操作需明确点击；不能在保存503后自动reload/重发。

拟议 `POST /api/v1/settings/repository-activation/reload` 使用现有admin/session/CSRF要求，body严格为`{expected_config_revision:"正uint64十进制字符串"}`，不接受文件路径、凭据或generation。固定读取SettingsService初始化时绑定的文件；最大1MiB、UTF8、严格单文档YAML、拒绝重复key/未知字段、配置完整校验；不以当前Settings.Save的空token保留语义填补被恢复文件的凭据。加载文件即管理员明确选择将该文件作为下一有效配置；直接编辑文件尚未点击reload之前不影响当前内存授权。

reload锁顺序保持Settings.mu→gate→短Store事务。先核对请求Revision恰等于当前有效内存Revision，拒绝不规范/溢出/无前导零；读取文件Revision按现有OpenSettings规则缺失/0规范为1，但拒绝类型错误。计算新Revision=`max(current.Revision,file.Revision)+1`，达到MAX_UINT64拒绝，不能回绕或发布旧版本。忽略文件Projects并保留当前Settings的项目快照（项目/上下文权限由DB与原outbox负责，不通过reload导入）。通过与Save同一原子文件替换及激活流程写回这个新Revision；整个内存配置仅激活成功后一次发布。项目outbox在pending期间保持dirty，恢复后按原版本对账同步，不因reload清除dirty或更改DB成员。

这使还原旧文件得到新的全局Revision和新的凭据generation；旧客户端不能因Revision回退重新获得CAS。reload不调用外部服务，不清除任何成功回执。响应丢失后，同expectedRevision重放若上次已发布，返回409 settings_changed，客户端GET核实当前状态；不得自动换expectedRevision重新执行。若上次仍pending，内存Revision未变，可同请求再次尝试；重复文件替换可能推进文件Revision，但相同有效凭据HMAC不得再次推进generation。不承诺每次失败后文件Revision不变，UI须区分“文件已落盘/激活未完成”与“未保存”。

HTTP结果：200返回`{settings:脱敏公开配置,repository_activation:{state:"active",code:""}}`；400 invalid_activation_request/invalid_configuration；401/403原权限；409 settings_changed（客户端GET，不自动重试）；503 credential_activation_pending/credential_activation_unavailable；代次或全局Revision耗尽以409 activation_version_exhausted返回，保留旧state或pending且说明需维护。文件/SQL底层错误不能原样透传。普通Settings.Save在pending时返回503且不触碰文件；公开DecodePublic拒绝counter/activation输入，不静默接受。

恢复证明：旧fileRevision低于current、超2^53精确请求、MAX_UINT64、零/负/浮点/重复key/多文档YAML、空token、路径参数注入、pending时普通保存及outbox同步、发布前/后响应丢失、连续reload generation保持、旧CAS始终409，需真实HTTP路由和SQLite断言。前端409/503保留当前错误说明与重读入口，不使用旧GET数据自动授权操作。

## 尚待验证的工程边界

1. 上述reload单调Revision与文件/DB故障合同需要独立技术复审及实际Go故障注入，未经通过不实施。没有用户产品语义待猜，仍有工程正确性待证明。
2. 旧HTTP/path前缀继续供旧reader使用，新scope当前仅表示HTTPS origin。S4必须显式诊断不可表达的旧endpoint，保留历史查看；不得通过去路径或改协议伪造兼容。若要支持这些endpoint的native scope，需单独修订scope版本和迁移合同；本S3不悄悄扩大协议。
3. S4默认Git及API共享预算/网络限制须与指纹material同源；本计划不定义尚未落实的CIDR设置，也不把旧DefaultTransport伪装成受限transport。S3只记录实际有效HTTP传输策略与代理，对未来Git模式不提前作授权声明。

这些限制不缩减原目标。完整S3尚待审查，不用内部持久化helper冒充完整撤权能力。

## 提交与回滚

此F3文档单独提交推送；不触发生产CI（docs路径忽略），不取消正在运行的S2精确CI。后续每片F4/F5写真实记录后独立提交；生产HEAD只在上一生产CI终态后推送。schema2 key/generation不能删除或降回0；未开放native scope前可回到支持同schema2的程序，但回退旧凭据消费者可能绕过新gate，必须停止全部旧实例并明确版本支持，不能称无损热回滚。

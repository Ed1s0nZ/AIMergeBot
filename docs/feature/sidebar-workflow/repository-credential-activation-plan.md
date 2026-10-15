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

## 实施切片与证明

| Slice | 文件/责任 | 通过条件 |
| --- | --- | --- |
| S3a | legacy_repository_state.go + repository_credential_material.go + SQLite/纯函数tests | 私有state严格读取、规范material/HMAC、原子activate/validate，ABA/双连接同值/溢出/缺行/畸形类型/零秘密输出；不接启动、不旋转生产generation |
| S3b | repository_credential_activation.go + settings_persistence.go抽出原子写 + Settings薄hook + 启动assembly | pending与文件/DB/内存故障矩阵，目录持久性检查，完整快照一次发布；model/outbox不旋转；启动前激活；现有Settings保存CAS不退化 |
| S3c | activated_legacy_repository.go + runner/直接client消费者薄委托 + interruption + HTTP/settings消费者 | 无绕过reader；未激活/pending/撤权时下一网络请求与模型输入次数0；同进程和双连接变更都fence；公开错误/当前配置状态传播 |
| S3d | 同进程reload恢复/已有配置页反馈 + 浏览器/协议tests + docs | 下列reload兼容决策明确、实现并验收；unknown持久化状态不被覆盖；完整Go/race/vet/typecheck/embed/精确HEAD CI |

S3a只是内部前置，不能用它宣称凭据撤权已接通。S3b/c/d的锁/消费者/恢复必须在各片开始前证明合同完备；S3整体完成需要所有行，不跳过难点。技术复审可只批准S3a明确边界，但须保留本完整目标并单独标出不能批准的部分。

验证需在合成配置和SQLite上故障注入：rename前失败、rename后DB失败、commit后发布前中断、重启重算、恢复旧文件、两个连接同配置/不同配置、MAXgeneration、缺key/坏row、模型/outbox同步不变、真实阻塞HTTP响应期间改token后结果不能进模型。外部发送次数与模型输入次数必须断言，不能只检查error。真实部署仍需配对备份演练；当前不调用真实模型/发送服务。

## 开放决定与实施限制

1. 同进程reload的global config_revision：真实文件可能比内存旧，不能直接发布低Revision使旧客户端CAS复活；必须确定读文件后Revision如何保持单调并落盘、请求丢失后重放如何处理。DB凭据generation解决不了Settings全局CAS。需要专门合同/测试，未确定前不写reload接口或UI。
2. 旧HTTP/path前缀与新HTTPS origin scope的衔接：S3保持旧reader兼容，S4不得假称这些endpoint可进入当前新scope；需明确支持范围/迁移诊断，不能通过去路径或改协议解决。
3. S4默认Git及API共享预算/网络限制需与指纹material同源；本计划不定义尚未落实的CIDR设置或把旧DefaultTransport伪装成受限transport。

上述不是要求用户再次批准已确认需求，而是实现前需补齐的工程合同。S3a前置可以先审查；其余不成熟部分先不实现。恢复、native执行及完整原需求不因切片而缩减。

## 提交与回滚

此F3文档单独提交推送；不触发生产CI（docs路径忽略），不取消正在运行的S2精确CI。后续每片F4/F5写真实记录后独立提交；生产HEAD只在上一生产CI终态后推送。schema2 key/generation不能删除或降回0；未开放native scope前可回到支持同schema2的程序，但回退旧凭据消费者可能绕过新gate，必须停止全部旧实例并明确版本支持，不能称无损热回滚。

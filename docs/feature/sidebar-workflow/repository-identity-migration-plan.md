# S2 永久身份登记与schema2：实施补充

2026-10-15，codex/sidebar-workflow。输入F3 9c1ee2f及F2第三轮Pass；S1生产949c13a已推，本地全部检查通过，精确CI38740692571仍在执行。当前补充只触及docs，ci.yml的paths-ignore明确不触发新CI，不取消该精确run。前轮属于实际代码与证据进展，不是完成全部目标。

## Gate / 依赖与维护边界

P6→P7/P8、F3→F4/S2。原永久kind/schema2设计不变，补明确存储写入与迁移反例。Store.migrate只增加early version委托和新迁移委托；新schema/健康检查/登记Store分文件。project_identity_sync同时持有导入、导出、保存三种责任，high；仅委托kind helper，不新增provider分支大方法。Allowed adapter_extraction，先不广泛重排。只feature branch本地/临时DB，不执行用户真实DB迁移、main合并或部署。

## 迁移决策

- 任何DDL/数据写入前读取platform_schema：不存在为首次初始化；存在必须恰好1行、INTEGER version且为1/2；其它/重复/空/坏列直接拒绝。schema2所需身份/私有state/回执表缺失不能自动重建。禁止以重复运行migrate回填丢失的登记或密钥。
- 从旧格式升级前检查platform_worker_instance：存在有效owner/未来lease时拒绝，必须停旧worker。租约检查不证明其它旧连接都已关闭，停机/配对备份仍是部署前置。旧pending/running原结果/trace不改，状态failed并说明需要重新提交，不创建重试/伪造scope。
- 同一SQLite事务建立登记、恢复回执和私有legacy state表，分类旧项目、标必要config outbox dirty，最后写version2。任何失败回滚全部DDL/数据/version/outbox。新空库直接2但无项目时不产生虚假的config同步工作。
- 当前binding行、任何binding history、原生create receipt任一存在即internal；三者全无才legacy。initial identity revision1独立于历史binding revision。全部历史binding JSON/列/revision须合法，current必须匹配对应history且是最新版本；矛盾标repair_required，不按损坏JSON推断legacy。
- native create receipt是历史回执：只核对其原ProjectID、request/actor元数据、原始binding对应历史版本；不能拿它和当前binding或当前项目名称比较。正常rename/rebind不误报。native receipt缺当前binding、坏历史/重复创建回执均repair_required；原记录不删除。

## 新写入与消费者

| 路径 | 原子规则 |
| --- | --- |
| SaveProject/legacy import新项目 | 同事务创建legacy/revision1登记；已有项目只更新名称/启用，不能补猜丢失登记或改变kind |
| 原生create | 同事务创建internal/revision1、绑定、scope关联集成、原始receipt；第一次binding不额外旋转identity；私有创建入口明确此上下文，普通HTTP不能提交该标记 |
| 普通保存binding | 先校验登记/当前binding/CAS；health非valid或internal缺binding拒绝；首次legacy→internal以及之后每次变更递增identity revision；binding revision保持原单调CAS；不能回到legacy |
| requireLegacyProject/Snapshot/Run | 已有项目只有健康legacy登记且无任何显式证据才可走旧路径；missing/坏登记、internal、历史/receipt残留均拒绝，即使当前binding行丢失。不存在项目保持现有trusted内部primitive边界，不伪造登记 |
| config export/import | InternalProject依据持久kind；坏binding仍明确报错，不导出legacy；internal缺binding也不得变legacy。InternalProject引用只验证DB权威，绝不覆盖名称/启用/上下文 |
| ProjectsForUser | 原权限/同事务保持；internal缺binding明确不可用，不能让旧UI显示默认GitLab远端ID；健康已有binding响应shape保持 |

registry/state/receipt使用带typeof和范围的CHECK。旧坏记录的健康检查读取有限JSON（binding4KiB、create receipt8KiB），超限标repair，不输出原内容。revision非法类型/负数/溢出保持不可恢复状态，不静默归零；修复仍等待S5。私有legacy state首次crypto/rand32byte key、generation0/fingerprint NULL，S3激活前不冒充已经冻结凭据；schema2重开校验持久key而不重建。

实施中补充身份保留规则：新内部ID的上界必须包含projects/永久登记及所有历史run目标、来源和context索引ID，不能只看当前项目表。main.go在ImportLegacy完成后才注册HTTP，原始旧结果已先变成run引用；不按远端ID重新赋予历史任务归属。新增回归以原始分配器先证明旧报告可因新项目成员权限被读取，再验证修正后拒绝。ID仍是内部不透明值，无新公共字段或用户配置推断。

若schema1已存在原生创建receipt与legacy run目标/来源/context同ID，升级整体拒绝并保留旧库，要求离线核对，不自动重编号/移动报告或沿用新成员权限。schema1原生项目从未开放审计，故这种引用不是应继承给新项目的合法native运行证据；仅有普通bind history而无create receipt的既有项目改绑不按此规则误报。此为不猜测历史归属的明确失败边界，不表示已实现旧混合数据的自动修复。

## 验证计划

新真实SQLite测试：旧legacy/bound/native、正常改绑与rename、坏JSON/历史缺失/当前行丢失/receipt矛盾、永久kind重开、schema重复/未知版本零写、升级中故障全回滚、活worker拒绝、旧pending保留结果并停止；创建/导入/绑定CAS并发、MAX_INT64、无授权增量。捕获旧Store二进制打开升级DB拒绝且无schema/数据变化。配置同步失败/重启不可把internal导回legacy，原业务回归必须继续。

13处测试直接SQL插入project；如执行链要求永久登记，改为正式SaveProject构造合法fixture，保留各测试的原始业务断言；不用新增默认legacy的数据库触发器来迎合fixture。故意损坏/最高ID等边界仍需保留相同触发语义。

依次专项migration/identity/create/binding/guard/config/race→全Go/vet/build/typecheck。S1精确CI终态前不推新的生产提交，避免cancel-in-progress取消独立验证。S2实现完成需要自己的完整精确HEAD CI与代码复核。新schema部署会拒绝旧版服务，只在支持schema2程序间正常回滚；旧版回滚须停新程序、恢复配对备份，禁止改version或删表。未完成备份恢复证明不发布升级。

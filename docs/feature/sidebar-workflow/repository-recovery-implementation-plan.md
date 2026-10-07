# 仓库身份与恢复：F3实施计划

- Feature: 原生GitHub/多仓固定身份与UAR-001恢复。
- Branch: codex/sidebar-workflow。
- Confirmed input: requirements.md；设计repository-recovery-contract.md；F2审查technical-review-2026-10-08-repository-recovery-3.md @532db69 Pass。
- Current phase: F3；不修改完整REQ/AC，不表示整体通过、不合并main/发布。

## Workflow / Maintainability Gate

当前P6→P7/P8。完整设计已具备数据/状态/授权/反例；第一片数据契约允许实现，恢复API/native执行开放仍依赖后续完整factory与验证。缺失外部真实服务证明不阻止受控实现，但阻止声明完整交付。执行scope仅下面S1；S2–S6须在各片开始前确认所有上游代码与测试已完成，不从此计划推断尚未解决的Git预算/API模式已可用。

Maintainability：audit_policy.go41行，定义/捕获/digest；binding_guard59行，旧身份拒绝及poll写入；context_repository_access67行，ACL/SQL/解析；followup_audit324行，计划/覆盖/固定读取/提交，三个以上责任high；runner442/runs_store328同属高耦合编排。新scope类型、解析与纯验证分到新文件，不往runner堆字段分支。Refactor required first=no（窄委托，不增加大方法）；allowed adapter_extraction/narrow_fix。S1只修改policy字段/解码委托、现有guard、snapshot装载和followup scope深拷贝，runner保持调用原guard。后续factory另分模块；不进行无关大重排。

## 完整实施序列

| Slice | 文件/边界 | 交付行为与依赖 | 验证/回滚 |
| --- | --- | --- | --- |
| S1 | repository_scope.go、repository_scope_json.go、repository_scope_test.go；audit_policy.go；repository_binding_guard.go及测试；context_repository_access.go；followup_audit.go及测试 | 固定scope typed contract、严格16KiB解码/跨字段验证、三方context对账接口、不可变复制；factory未接入时所有带scope远端路径拒绝 | 真实SQLite入队/执行/retry/发布拒绝与零访问；旧nil行为回归。新增field未由正式producer发出，schema保持1、PolicyVersion保持v50；无需UI/assets迁移 |
| S2 | repository_identity_store.go、repository_identity_migration.go、store.go启动version检查；project_identity_sync/create/binding委托；migration tests | schema2原子升级、永久kind、健康诊断/版本、配置导出；加入恢复回执schema和legacy私有state | legacy/bound/坏历史/receipt/缺行/回滚/重开；真实旧Store拒绝；停止旧进程/备份后才部署。不能降version假回滚 |
| S3 | repository_credential_activation.go、legacy_repository_state.go、Settings.save hook及startup assembly；settings tests | HMAC代次、pending、文件/DB/内存顺序；runtime拿不可变逐仓描述 | token/origin/network变化、model/outbox不变、ABA、并发、故障/中断、秘密不外泄；旧legacy reader也guard。无客户端counter |
| S4 | repository_factory.go、github_pr_snapshot.go、Git/API固定工具适配、runner/context/CODEOWNERS/retry/followup/poll/webhook/publication | 全provider统一factory；source授权前零内容读取、固定SHA完整默认Git工具链/API模式覆盖/共享预算；所有入口同准入与runtime fence | fork、同数字ID、跨origin、>300文件、rename/mode/gitlink、撤权/超限/取消/coverage传播；正式scope producer前升级PolicyVersion为eino-audit-contract-v51。整体Git网络/预算计划需具体闭合后才写实现 |
| S5 | repository_recovery_service/store/HTTP、receipt校验、Runner批量取消；真实协议/SQL测试 | 诊断/验证/CAS/回执/取消/outbox闭环；仅factory完整支持的provider开放 | 同key跨route、响应丢失、坏回执、配置改后重放、history冲突、max/巨行、远端零/一次请求、恢复期间取消；禁止DELETE fallback |
| S6 | 现有repository-binding-editor/shared types/api feedback、浏览器fixture、正式assets、API/迁移文档及CHANGELOG | 在现有配置内完成用户操作与同请求恢复，展示真实可用性，不新增壳菜单 | 管理权限/窄屏/键盘/slow/error/pending、精确CAS、真实controlled create→audit→恢复链路、完整Go/race/vet/frontend/embed/精确HEAD CI及独立代码复审 |

这是完整依赖链，S1不是“Github支持完成”。UAR-001只有S2–S6完整用户链证明后关闭。UAR-002/004、通知/各bot联调、自动闭环及其它完整REQ保持另有工作，不从本计划消失。

## S1精确交接

- RepositoryScope：version int=1；Target/Source必填RepositoryReference；Context必须非null数组（可空，最多4）；Observation必填。Reference包含project/identity/provider/origin/remote/name/binding/credential；提交SHA不放reference；Credential仅integration或legacy_gitlab，禁止混合两类字段。legacy generation JSON严格正int64十进制字符串；binding_revision零只属于legacy，但字段必须实际存在，不能把遗漏当0。
- JSON层：范围限制、Token walk拒绝任意层重复key/超深嵌套，再DisallowUnknownFields解码scope全部结构；缺字段、null、错误类型、尾随JSON固定ErrRepositoryScope，不输出原payload/字段值。AuditPolicy通过小委托捕获repository_scope显式null/重复根key，旧缺字段policy继续兼容，不对旧合法JSON新增字段必填。未知scope version拒绝，不作为旧nil。
- 纯验证：canonical origin/fullname复用binding helpers；IDs/revisions正、credential互斥、target/source同provider/origin、同仓reference完全一致；GitHub40hex，GitLab既有40/64hex；Context排序/去重/不含target，跨平台允许且SHA按对应provider；Observation number/BaseSHA/HeadSHA与Snapshot严格相等，Github DiffVersionID必须0；scope.context投影恰等于旧policy.ContextRepositories（nil旧数组与空投影按空语义比较）。总序列化上限16KiB。
- 规范化与验证分离：producer helper返回deep copy及规范排序；decoder/runtime验证不悄悄修正错误输入。target/source/context及credential不能共享可变slice/pointer。clone用于followup，JSON roundtrip不是克隆的错误吞掉路径；验证调用前不改变parent。
- unsupported guard：即使scope合法且项目尚无binding行，也拒绝走旧GitLab；无scope的原guard保留。retry durable guard必须检查policy是否带scope，不能只查当前binding行；取消/错误重试原状态保留，不产生child。publication/context/CODEOWNERS继续沿原统一guard拒绝。没有任何valid scope自动获得授权或被入队去重吸收。
- snapshotForRun读取base/head/mr/diff_version_id以供未来完整校验；查询角色/错误顺序不改变，读事务内装载。context索引对账为Store helper同事务读，用于有scope的加载/准入后续接入；S1不运行新schema迁移，不填历史scope。若尚无factory能产生scope，不将合法fixture当实际可用。

## S1测试与命令

纯合同的区分性正反例：same-repo/fork/mixed context，缺字段/显式null/未知version/重复key（包括根scope及nested credential）/类型/超限/depth，canonical URL/name/凭据互斥，SHA用途与40/64规则，context顺序/重复/映射矛盾，clone修改不改变父scope，digest对身份/credential generation变化敏感而旧nil序列化保持。缺binding的scope仍不可执行，既有legacy scope nil仍沿原路径。

实际临时SQLite/受控repository证明：enqueue rejected零run/quota/event；queued执行零remote/auditor；FailAndRetry停止且零child；publication preflight拒绝且零远端写；context索引缺/额外/不同sha拒绝。只使用合成凭据和in-process受控读写计数，不发真实外部消息。

先专项scope/guard/followup/retry/policy/context测试和race，再go test ./...、go vet ./...、go build ./...及git diff --check；新field仅后端可选shape，无前端生产变更，typecheck确认现有客户端不依赖完整policy形状。没有asset源变化不重建dist。精确生产HEAD CI必须另验证，不用F2 docs或旧绿色run代替。

## Rollout / Records

每片F4/F5记录具体已实现与未覆盖证据，按feature-lifecycle独立提交同步。S1不开放新scope远端执行；rollback只撤销尚未使用的field/guard委托，不能在正式native运行已产生后回到支持schema1的程序。S2起采用schema2停机迁移，发布前完成备份/恢复手册与真实旧进程证明。最终交付保留完整原生执行与恢复目标。

本计划仅创建文档；首片生产工作应从S1开始。F2 Pass只是方案合格，不是功能完成；S4尚未闭合的Git总体预算/API模式依赖须另补具体计划，不能猜实现或永久改成受限API-only产品。

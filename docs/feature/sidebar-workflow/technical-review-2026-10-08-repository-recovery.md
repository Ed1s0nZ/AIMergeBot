# 技术方案审查：仓库身份与恢复

- 日期：2026-10-08。
- 快照：238ef1d8105299ca1ca08ca3222ae9cd21b609ca，codex/sidebar-workflow。
- 主文档：repository-recovery-contract.md；关联 github-provider-design.md、github-diff-design.md、Confirmed requirements.md。
- 视角：架构交接与实现前可执行性；同一作者独立第二轮源码核对，非独立代理审查。
- Final decision：**Fail**。恢复原则合理，但具体持久结构、请求合同和并发语义尚不足以交给开发实现。本结论阻止恢复功能进入 F3/F4，不改变完整需求，也不影响已实现功能的既有验证。

## Handoff Judgment

不应实现“删除绑定即可恢复”。持久内部身份与固定任务身份的方向正确，但仅有方向不足以实现安全恢复：目前普通读取坏绑定就返回冲突，修复请求无法从该读取接口取得可用 revision；版本最大值、缺行、重复历史和正在运行的请求也没有明确状态。需要先关闭下面四个缺口。

## Mandatory Criteria

| Criterion | Status | Evidence | Required fix |
| --- | --- | --- | --- |
| 背景与目标 | Pass | 明确 UAR-001、未来运行恢复、历史结果保留 | 无 |
| 数据结构清晰 | Fail | kind/scope 列表存在，但数据库与 JSON 合同不完整 | RRC-001 |
| 新改字段识别 | Pass | 对象表明确拟议新增与原绑定保留 | 仍需补齐字段默认/边界 |
| 数据流 | Fail | 准入图明确，恢复提交/验证/幂等路径没有具体合同 | RRC-002 |
| 生命周期 | Fail | 列举撤权/取消/旧版本，但保证与实际机制不一致 | RRC-003/004 |
| 流程图 | Pass | target 元数据→source/context ACL→内容→二次准入 | 增加恢复写入与响应丢失路径 |
| 开发交接 | Fail | 相关模块可识别，没有无隐藏决策的迁移与执行顺序 | 完成四项后另写 F3 |

## Qualified Areas

- 内部 ID、远端 identity、ACL 分开，恢复不猜数字 ID、不迁移历史任务、不复活 unknown 动作。
- source ACL 完成前禁止 compare/fetch，公开 fork 不是授权证据。
- 网络调用在 SQLite 写事务外，最后短事务重查；凭据不进入 policy。
- 不把 GitHub 内部读取模块或实验当完整原生审计，不删除 guard 来伪造可用性。

## Disqualified Gaps And Risks

| ID / Severity | Evidence / issue | Impact | Required fix | Blocks pass |
| --- | --- | --- | --- | --- |
| RRC-001 / Blocker | scope 只有字段列表，没有 target 的 base-tip/merge-base 具体 JSON、必填规则、同仓多角色一致性、context 排序与旧 ContextRepositories 对账；kind 表没定义 revision owner、缺行/坏历史的处理 | 开发会自行决定 digest、索引、映射，可能遗漏 ACL 或让历史语义变化 | 给出精确 schema/规范化/大小预算/互相校验；说明 retry/followup 及持久 context 索引的同事务传播；定义迁移输入来源、矛盾与缺失状态 | yes |
| RRC-002 / Blocker | 恢复没有 request/response/error/replay 合同；readRepositoryBinding 遇坏 JSON/列矛盾先 ErrConflict，旧 GET 不会返回修复所需的可信 revision | 坏绑定可发现但无法通过产品流程修复；响应丢失后可能多次验证与重复写版本 | 定义 admin 专属诊断元数据与可提交证据，不回显坏 JSON/密钥；定义 CAS 元组、验证超时、失败与提交后 sync pending、相同请求重放语义；不得用删除行回避 | yes |
| RRC-003 / Blocker | 文档要求绑定变化后停止读取并由已有取消机制终止；saveRepositoryBindingTx 只写绑定/历史/事件/outbox，并不取消 run；runner_lease.go 仅五秒轮询持久 run 状态。已经发出的网络请求也不能因 CAS 拒绝变成零次请求 | 宣称即时零访问无法被现有机制证明，远端验证期间竞态可能继续进入模型/发布 | 明确取消事务覆盖 target/source/context、进程通知与跨连接观察、每次读取前/结果消费前复查；区分已发出请求、被丢弃结果和新请求。定义可证明的撤权时点，不能承诺撤回已发生的读取 | yes |
| RRC-004 / Blocker | “最低支持版本”没有实现载体；AuditPolicy json.Unmarshal 忽略未知字段。PolicyVersion 在 execute 防旧策略进入 auditor，但 Claim 与恢复有独立入口；Store 只接受 platform_schema=1且事务失败回滚 | 仅加 scope JSON 不构成启动/写入/读取版本屏障；同库新旧程序或降级可能仍导入/同步/领取不支持的数据 | 明确 schema/policy 两级版本策略、升级顺序、旧版拒绝时点、旧任务展示与执行兼容、备份/恢复与禁止共享降级写入；用真实旧二进制打开升级 DB 验证 | yes |

RRC-003 是拟议设计的缺口，不声称当前未开放 native 运行已发生越权。RRC-004 有现成保护可复用，并不声称旧版在 binding 行存在时会绕过现有 guard。

## Data Structure Review

| 对象 | 现有证据 | 缺失合同 |
| --- | --- | --- |
| 身份登记 |拟议 kind legacy/internal；配置 InternalProject 目前来自有无绑定 | 列名/约束、迁移单事务/输入优先级、已删 binding 但 receipt 存在、缺失/矛盾记录、可诊断状态 |
| scope v1 | target/source/context 与逐仓 integration revision | 精确 JSON/types、有效 SHA、target 多提交、canonical 排序、重复/未知字段、最大总字节、context 索引一致性 |
| binding / history | 原有 CAS+追加历史+唯一远端 tuple | 损坏当前行对应 history revision 不一致、revision 溢出/缺失的显式修复方式；不能重置历史主键 |
| 恢复请求/回执 | 没有拟议 schema | 操作身份、actor 归属、原请求重放、诊断与编辑字段、提交后的 revision/sync 状态 |

## Data Flow And Lifecycle Review

源码核实：audit_retry.go 复制父 audit_policy_json 和 context 索引；followup_audit.go 使用原 policy 做固定选区准入；snapshotForRun 只装 project/source/policy，不装 base/head；新增 scope 校验不能默认该辅助函数已取得完整运行身份。当前 context ACL 使用旧 ContextRepositories 和 platform_run_context_repositories；新 scope 若独立新增 context 但不对账，会与已有授权查询分离。

saveRepositoryBindingTx 当前无运行状态更新。Runner.Cancel 可以调用本进程 active cancel，但 DB 直接取消只通过 leaseLoop 的后续扫描观察；已有 HTTP/Git 子进程中断还需实际证明。绑定及集成修改后的旧运行应该可显示保留结果，结果显示与远端内容读取授权必须区分。

## Flow Diagram Review

准入图合格；缺恢复诊断→管理员编辑→远端验证→短事务 CAS/取消/历史/回执/outbox→响应丢失重放图。新图要标出每次外部请求与状态提交，不把 CAS 失败画成“之前读取没发生”。

## Implementation Readiness

已识别模块：repository_binding、project_identity_sync、repository_project_create、audit_policy、runs_store/context_repository_access、runner/runner_lease、audit_retry/followup、provider factory、HTTP/现有仓库编辑页、publication preflight。先闭合数据和恢复合同，再拆 F3；不是先创建表或按钮后补权限。默认完整 Git 工具链、API 模式覆盖及预算仍是另一组必要上游依赖，本次审查不批准它们。

## Verification Readiness

设计已有正反例清单，仍缺“操作次数/时间顺序”的验证方案：验证读取已发出时改绑定、响应尚未消费时撤权、其他连接修改、Git 子进程中断、保存成功响应丢失、坏行修复再次响应丢失、原生项目缺绑定、旧二进制打开新 DB。普通单测使用当前结构反序列化成功不能证明降级安全。

不调用真实模型/远端消息/工单。此次针对已有 policy 和取消保护运行的检查只验证现有机制，不验收拟议恢复功能；结果见下方证据记录。

## Modification Recommendations

1. 先补精确 scope 和登记 schema，并保持旧 ContextRepositories/索引一致性，不引入第二套未经对账的 ACL。
2. 复用旧版已拒绝未知 schema 的机制作为可检验的降级屏障候选；PolicyVersion 用于任务策略，不能替代 DB 格式屏障。
3. 将恢复视为现有配置内的一次明确身份变更；禁止 DELETE fallback，补诊断/修复/幂等回执而非新增泛化控制台。
4. 撤权保证描述实际可证明的时点：已发出请求可能完成，但结果必须隔离、不能再喂给模型或发布；不要在文档写无法验收的零请求承诺。

## Re-review Requirements

RRC-001–004 均需在设计文档中得到具体解决，补恢复流程图与分模块 F3 前置。重新审查全部四项及其引入的权限/历史/并发影响，而非仅确认新增文字存在。未通过前不实现恢复 API、自动迁移或 native 执行开放。

## Validation Evidence

上述快照、macOS、Xcode PATH 下运行 `go test ./internal/platform -run 'TestPreviousReleasePolicyCannotAutomaticallyRetry|TestPreviousReleasePendingRunRejectedBeforeAuditor|TestWorkerCancellationWinsExpiredRecovery|TestCapturedPolicyContainsNoCredentials' -count=1`，exit0，platform0.994s。证明旧策略不自动重试、不进入 auditor、已取消任务不被恢复机制复活、现有 policy 无凭据；不证明拟议 scope/数据库迁移/恢复请求正确。源码确认 Store 未知 schema 拒绝发生于事务 commit 前。仅新增本审查文档，git diff --check 通过；没有执行真实外部请求、改生产代码或批准完整分支。

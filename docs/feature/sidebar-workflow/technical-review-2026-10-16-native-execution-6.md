# Technical Plan Re-review：固定仓库执行与兼容切换

- 日期：2026-10-16；Reviewer：Codex，架构交接审查。
- Snapshot：8459412ebd1686b333c9a8182c064b8d7ef2728f。
- Artifacts：repository-native-execution-plan.md、repository-budget-contract.md、repository-api-diff-contract.md、repository-resource-contract.md、repository-native-publication-contract.md、repository-legacy-transition-contract.md；对照S3 activation/runtime与recovery合同。
- Supersedes：第1–5轮设计决定；本次重新核对旧finding与作者修订传播，不继承批准。
- Final decision：**Pass（S4内部实施交接）**。NTR-001–008设计缺口已关闭。允许按N1→N5实施和验证；这不是代码批准、N6/S5/S6公共恢复接口批准、功能完成、外部发送/部署/main合并许可。

## Handoff Judgment

可以交接内部实现：身份、准入与运行生命周期、共享预算、Git/API取数、后台事件、发布目标及普通GitLab兼容有具体对象、调用方和失败边界。未见需要编码者再决定的阻塞设计空白。所有“已验证”的算法/协议研究仍只算局部证据；正式adapter、真实SQLite恢复、端到端及真实账号证明必须按各片验收取得。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| Background / goals | Pass | Confirmed REQ-001–023/AC-001–021保留；PR、多语言、跨仓、默认Git与API-only不缩减 |
| Data structures | Pass | scope v2 endpoint union、严格budget limits/principal/receipt、owner/epoch、hook seed、publication target/attempt、poll baseline字段明确 |
| New/changed fields | Pass | 各合同字段表；切换合同明确替代旧api_origin键、service顶层revision及scope v1之外的新union |
| Data flow | Pass | 全producer→原子admission/seen→session→覆盖→outbox；跨run发布target与迟到回执分离 |
| Core lifecycle | Pass | admission/worker/retry、有限hook恢复、census跨轮、资源隔离、unknown发布和旧v50切换均有终态/所有者 |
| Diagram | Pass | budget、API双树、resource状态、hook与新旧切换图覆盖主要边界 |
| Handoff readiness | Pass | N1–N5模块、顺序、测试/回滚/门禁明确；N6公共恢复/UI另依其设计评审 |

## Track A：Finding Closure

| ID | Status（仅设计层） | 当前证据 |
| --- | --- | --- |
| NTR-001 | resolved | user/service严格union；actor正ID/NULL CHECK；service authority逐integration/legacy分支验证 |
| NTR-002 | resolved | OwnedContentV1固定投影，POST/PATCH/GET共享语义；不是任意响应JSON hash |
| NTR-003 | resolved | claimed run/target/outbox三元组独立于desired；每claim随机token防ABA，迟到ACK不完成新desired |
| NTR-004 | resolved | expected author来自可信GET/ACK，unknown首次创建无可信主体时不从候选自证、不盲重POST |
| NTR-005 | resolved | 独立remote baseline、seen非observed、原子enqueue/CAS；census错误持久、单owner非重入，重启不提前initialized |
| NTR-006 | resolved | 切换后所有平台新任务v51/scope v2，legacy HTTP/path-prefix真实表达；历史v50不回填，全部producer与C3统一切换 |
| NTR-007 | resolved | material version2/legacy_fixed_session_v2与真实transport一起切换；startup/Save/Reload统一，不混旧profile |
| NTR-008 | resolved | 去重scope排除凭据代次；执行authority不可变保留原代次，重复事件不借新Token再入队 |

## Track B：Forward-risk Review

- endpoint key只规范身份，不修改wire URL；legacy/native同SDK base一致，不合并不同前缀。空api_origin仅v2严格legacy union可用，不放松native binding。
- v2 material变化会旋转代次；模型保存不旋转。固定proxy、无redirect与Git白名单需要实际请求验证，当前v1测试不可代替。
- 原webhook接持久队列但仍单独验签；service不由HTTP/模型输入，requested_by=0不授予权限。source/context读取前仍授权。
- credential失效不删除仍有效事件防重；重复返回原receipt，旧pending失败不换凭据。hook有限恢复不扩展deadline/额度。
- 共享预算是保守charged，不是精确流量/费用；receipt上限可先耗尽。LCS cells不是CPU硬quota。资源监控不是OS沙箱，跨崩溃未知目录不自动kill/delete。
- API目录不足停止模型且明确incomplete；text gap可保留其他证据；不把失败/取消/未再观察当安全或已修复。
- 未知外部创建不盲重发；context结果只发布通用状态/链接。人工处置改门禁、工单恢复仍属UAR-002/004，不在本次借机添加未明确规则。

## Data / Lifecycle and Implementation Readiness

N1须同时实现严格序列化、scope v2和schema3账本；不得开放新准入。N2/N3接真实transport/资源/对象与差异。N4须用登录HTTP到模型fixture及结果验证全部consumer，不能保留有scope的旧GitLab直连分支。N5完成hook/发布/启动整体装配。S5身份恢复与S6公开UI仍须其具体合同通过及完整验收后才能解除501；本次不替它们批准缺失的公开API。

## Verification Evidence / Limitations

本次静态复审上述合同及关键现有调用方，核对从第3轮到当前作者修订。执行文档相对链接与git diff空白检查；生产代码未变，未重复跑完整Go/前端CI。现有研究覆盖对象协议、Git URI反例、LCS与资源边界，不能证明本轮拟议codec/schema/session/发布已实现。尚未执行的SQLite并发/崩溃、HTTP/TLS/CGI、模型fixture整链和实际账号验证均保留为实施验收，不标通过。

## Recommendations / Re-review Triggers

非阻塞：实施时把旧稿中被显式替代的字段表归并到单一当前表，减少阅读跳转；保留历史报告但不复制旧状态为当前结论。任何修改endpoint/传输版本、预算公式、principal权限、重试/防重或unknown外部写规则的实质变更须再次评审。代码完成后另做证据驱动审查，不能用此Pass代替合并决定。

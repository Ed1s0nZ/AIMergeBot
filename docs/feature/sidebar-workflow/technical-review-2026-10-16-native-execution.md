# Technical Plan Review：原生执行整体验证

- 日期：2026-10-16；reviewer：Codex，同一agent独立review pass，不冒称另一位审计人。
- 快照：1d5cc97592dc84f086ebcf4386cd2d1872b2fd20，codex/sidebar-workflow。
- Artifacts：repository-native-execution-plan.md、repository-native-publication-contract.md、repository-budget-contract.md、repository-api-diff-contract.md、repository-resource-contract.md；结合Confirmed requirements、S3 owner/runtime合同、当前Store/Runner/comment/check源码及隔离实验。
- Final decision：**Fail**。4项交接缺口，其中1 Blocker、3 Major；不允许据此进入生产开发，不关闭UAR-001或原完整REQ/AC。

## Handoff Judgment

方案已经覆盖完整provider路径及必要资源边界，且不再把通用Repository包装当作高级Git工具仍然可用的证明。但后台principal与schema约束冲突，远端对账内容和发送代次仍缺精确身份，直接交给开发者会迫使其猜测。应先修正契约，再作新的整体裁决。

## Mandatory Criteria

| Criterion | Status | Evidence | Gap / required fix |
| --- | --- | --- | --- |
| Requirement background/goals | Pass | Confirmed REQ-007/012/020/021/023及完整范围 | 无范围重确认需求 |
| Data structures / changed fields | Fail | budget actor正值、N5 target/receipt矩阵 | NTR-001/002/003/004 |
| Data flow | Fail | seed→budget→enqueue→outbox完整图 | 请求/远端owned content不是同一hash对象 |
| Core object lifecycle | Fail | admission/session/retry、target sending/unknown | 旧claimed generation与新desired未分离 |
| Diagram | Pass | budget/resource/N5 Mermaid与consumer表 | 主工厂序列可继续补充，非blocker |
| Developer handoff | Fail | N1–N6边界和验证明确 | 下面4项须先闭合 |

## Qualified Areas

- 保留v50 legacy与v51 native的区别，不给历史任务补造scope/预算；专用准入防止删除guard后错误fallback。
- source授权先于内容读取、所有consumer同factory、Git模式保留*GitRepository高级工具，API-only不启动Git。
- 共享bytes/cells、保守crash计费、bounded matcher及真实CGI反例均是明确设计输入，未冒称生产实现。
- context外发限制、unknown不自动重建、旧工作区隔离和有限诊断避免虚假闭环。

## Disqualified Gaps

### NTR-001 — Blocker / principal与schema冲突

预算表要求project_id/actor_id均为正值；factory及N5却允许poll/webhook service principal，兼容run.requested_by=0。未定义预算如何表达可信后台发起者、怎样和HTTP用户区分。照表实现会拒绝所有后台预算，放宽为任意actor=0又可能造成通用绕权。

必须定义principal种类、actor_id的NULL/正值规则及服务端source证明、恢复时验证；不伪造admin、不从请求接收service属性。验收user/service合法场景与伪造/矛盾/丢失source均零dispatch/入队。

### NTR-002 — Major / 对账哈希没有可比较的远端表示

N5把ack_hash/attempted_hash定义成精确HTTP request bytes，却要求GET远端对象后核对同hash。Check创建/更新请求字段不同，GET返回额外ID/app/时间及不同JSON顺序；不能直接相等。实现者只能猜字段/规范化或永远unknown。

必须定义版本化owned-content projection、固定字段/编码/可选值/时间和marker验证，用同一规则从request intent与远端GET构造；或者明确哪些结果不能自动对账。不得把任意GET JSON序列化当请求hash。测试重排/额外字段/缺字段、POST/PATCH和正文差异。

### NTR-003 — Major / 当前desired与已派发attempt缺少独立持久身份

target只有desired_run_id/generation、claim owner和attempted_hash。新run或同run复核更新可推进desired；崩溃后的unknown reconciliation缺少不可变claimed_run_id/claimed_generation和其outbox generation，不能可靠把迟到回执归给原attempt。现有comment claimed_generation是旧per-run代次，不能自动等同新target代次。

必须独立保存发送attempt的run/target/outbox代次，claim时冻结，sending/unknown期间新desired不得覆盖；finish/reconcile只能结算原attempt，新desired另排队。测试A发送/B排队/重启/A回执，B目标不得被A错误完成或覆盖。

### NTR-004 — Major / unknown首次评论创建缺少预先核实的作者身份来源

N5只在回执后记录remote_author_id，同时要求创建响应丢失时按“同已核实author”找评论；首次create恰恰没有该回执。没有定义expected author的独立来源，marker/body hash本身不能证明作者归属。

必须选择并定义写前身份查询与持久expected principal，或没有可信身份时保持unknown、禁用自动认领。不能从候选评论取author再用它证明该候选是自己的。区分user token和无法确定主体的App token，不承诺普通token保存即支持全部发布能力。

## Data Structure Review

| 对象 | 需修字段 | 类型/必须/兼容 |
| --- | --- | --- |
| budget | principal kind / actor / service provenance | 新native必填，user正actor；service不伪造user；旧v50不回填 |
| target | owned projection version/hash | 非请求raw JSON；未发送可NULL，attempt冻结后必须可验证 |
| target/outbox | claimed run/target/outbox generation | 正ID、独立于desired、sending/unknown不可覆盖 |
| target/attempt | expected remote principal | 写前可核实时保存；未知不得从候选自证 |

## Flow And Lifecycle Review

准入/根run/自动child同账本方向正确；hook重领的seed/owner/epoch需事务验证，原截止时间不延长。当前四项集中在跨合同身份传播，并非要求再造更强OS沙箱。S3已有AcquireWorkerInstance后绑定、同owner激活和二实例零变化的明确合同；当前源码仍未生产装配，实施时必须落实，不能把现有无owner校验的私有helper直接放到main前面。此处作为既有已知依赖记录，不重复虚构新finding。

## Verification Readiness

本轮为设计/源码交叉审查，无生产代码修改、无外部发送。既有GitLab/URI/LCS/resource探针仅是分项证据，不是N1–N6集成验收。CI即使绿色也不能代替未实现的schema3/worker/outbox场景。新设计测试清单应增加上面4项的区分性反例；无需为文档变更重跑未变应用全套。

## Modification Recommendations / Re-review

作者分别修正budget principal、owned projection、claimed attempt与expected author，保持相关consumer文档一致；写清最小schema与恢复规则后提交writer变更。后续复审保留NTR编号，重新对照全部5份合同、S3启动约束和旧finding，给新Pass/Fail；本报告不因作者补文自动变Pass。余下生产/真实服务证据仍是交付条件，不把设计Pass等同功能完成。

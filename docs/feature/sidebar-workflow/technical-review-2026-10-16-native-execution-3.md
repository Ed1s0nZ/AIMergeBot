# Technical Plan Re-review：新旧准入与发布兼容

- 日期：2026-10-16；Reviewer：Codex，架构交接审查。
- Reviewed snapshot：6f3afee60ee8eae8eae9580344e3c6b72fb21cb8。
- Artifacts：repository-native-execution-plan.md、repository-native-publication-contract.md、repository-budget-contract.md；对照 repository-credential-runtime-plan.md、repository-credential-activation-plan.md、repository-recovery-contract.md 及 scope 实现。
- 前报告：technical-review-2026-10-16-native-execution-2.md @9f3dac4。旧 Fail 不因作者修订自动续为 Pass。
- Final decision：**Fail**。NTR-005 主体修订已补齐，仍有跨轮错误状态的小缺口；新增 NTR-006 Major，禁止据此启用生产 C3。

## Handoff Judgment

当前不能交给开发者直接启用完整链路：原生发布依赖固定 scope，但普通 legacy 新任务仍被规定保留无 scope 的 v50。必须先定义切换前后的普通 GitLab 新任务如何取得可发布证明；不能靠实现者临时决定继续旧凭据发布或全面停发。

## Mandatory Criteria

| Criterion | Status | Evidence / required fix |
| --- | --- | --- |
| Goals/scope | Pass | 原需求保留，多语言/跨仓未被删减 |
| Data structures / changed fields | Fail | native poll 主键、seen、cursor 已定义；普通 legacy 新任务的 scope/版本和发布凭据证明缺契约 |
| Data flow | Fail | C3 激活后 v50 producer 与 scope-only publisher 不衔接 |
| Core lifecycle | Fail | 缺普通项目的切换、凭据轮换后新旧发布生命周期 |
| Diagram | Pass | 已有跨边界图；需随新旧准入修订同步 |
| Developer handoff | Fail | NTR-006 必须明确，不能留给编码时猜测 |

## Track A / Qualified Areas

NTR-001–004 的 principal、内容投影、claim 独立代次和未知作者约束没有被本轮作者变更撤销。

NTR-005：6f3afee 已新增按 project/provider/origin/remote ID 隔离的 baseline 与 PR state；seen 和 observed 分离，准入与 seen 同事务，revision 防旧观察覆盖，初始化有条件写及有界分页。主体设计问题解决。余下 Minor：cursor 跨轮推进，但“整次枚举无错误”尚无持久累计错误标记；不能让前轮错误在后一轮到达终页后被遗忘。补充字段/状态转换及重启测试即可，不要求远端列表原子快照。

## NTR-006 — Major：普通 GitLab 新任务与固定 scope 发布的切换不闭合

- Evidence：execution “版本、历史与开放条件”第15–19行及第46行保留普通 legacy v50；runtime plan 第11行要求激活后新外部发布消费 S4 固定 scope、禁止给无 scope 的任务猜 generation。publication 合同只规定 native target 和 legacy outbox NULL 字段兼容，没有提供 v50 新任务所缺的凭据证明。
- Trigger：按 N5 启用全局凭据激活器/C3，普通未绑定 GitLab 项目随后提交新审计；其 v50 任务无 scope/generation，发布按 runtime 合同不能授权。若实现者为保持可用而绕过，则违背撤权与冻结凭据的约束。
- Impact：默认已有产品路径可能从“审计并发布”退化为“只能审计”；或出现新的发布授权绕过。此为拟议设计冲突，并非声称当前未启用的 C3 已造成生产事故。
- Required fix：明确过渡期与正式切换的不同准入；切换后新普通 legacy 任务必须有可验证固定身份和凭据代次，并定义其版本、预算、执行/发布消费者。已有无证明 v50 仅保留原历史规则，不能回填当前代次。定义所有新任务 producer（手工、hook、poll、Slack、retry）的传播。
- Endpoint boundary：activation plan 第15/30/115行保留既有 HTTP/path-prefix；scope 当前校验仅 native HTTPS origin。不能把路径去掉或改协议来生成 scope，也不能把“显示不可用诊断”称为已有发布能力保持。作者需明确兼容表达/分阶段切换条件；如果需 scope 新版本，先修订合同并验证，不临时放松现有 native endpoint 校验。
- Verification：相同部署内绑定项目及普通 GitLab 项目均完成新任务→审计→发布；HTTP/path-prefix 旧配置不误路由；token/代理/endpoint变化后旧任务零发布，新任务使用新代次；旧无 scope 待发布记录不被猜授权；第二实例不旋转代次。

## Data Structure / Lifecycle Review

native baseline 的复合主键、初始化标志、seen_reason/seen_run_id/revision 已可交接；补持久 round-error。普通 legacy 的 prospective scope 与历史 scope 回填是两个问题：recovery contract 允许新观察构造 legacy_gitlab reference，不允许用当前 URL 证明旧任务。不得混为一谈。

## Implementation and Verification Readiness

当前只允许作者修订兼容合同。schema3/N1–N6、C3 启用和端到端仍未授权通过。本轮未更改生产代码、未执行外部发布；静态检查 producer/consumer 合同与现有 scope 校验，未重复运行无关代码测试。本次聚焦兼容冲突，不声称完成所有设计的 Pass 验收。

## Modification Recommendations / Re-review Requirements

1. NTR-006：给出旧模式保留阶段、正式切换的新任务合同、历史任务处理以及 endpoint 兼容边界，更新相关执行/凭据/发布文档，不允许单独开 C3。
2. NTR-005：补跨 cycle/restart 的 census 错误记忆及何时允许 initialized。
3. 作者提交后重新对照所有 producer、消费者与验收矩阵；当前决定保持 Fail。

> STALE / SUPERSEDED：当前报告见 production-audit-412097d.md，本文保留历史快照。

# PR Review — 修复与分区检视 @ c674a58

## 1. Review Metadata

Repository Ed1s0nZ/AIMergeBot，分支codex/production-audit-readiness，无远端PR。
Base bcaeae230e5f101b484df6777c1a56aa8be80f39；Head c674a585eafee71bc51e3b88ab8052cbbb0a7c6e。日期2026-10-05。Codex与只读独立上下文budget_review/context_review/ops_review/deadline_usage_rereview。RE_REVIEW，限定快照；production-audit-b40389a.md旧决定STALE并由本报告取代。

## 2. Decision

INCOMPLETE，mergeable=false。PRR-001/002/003已在本轮检查范围得到修复证据；完整分支部分检视及必要真实外部验收未完成，不能批准合并。

## 3. Executive Summary

独立分区检查覆盖预算、重试、补审、跨仓库授权/来源、历史关联与运维工具，确认两项异常边界。用量汇总现拒绝负total及加法溢出；健康检查现对子进程整次观察设置timeout。另一次新上下文只读检查修复与调用链，未发现其引入可行动缺陷。完整回归通过，真实模型质量/GitLab写入未验证。

## 4. Scope and Change Map

旧报告head b40389a→当前head文件delta均已分类：检视报告及implementation（证据/设计），model_usage.go/test（统计），ops-healthcheck.py/test（期限），model-usage.tsx与web/dist资源（说明及生成产物）。生成资源以构建来源及reason传播检查，不逐行检视压缩代码。

分区状态：预算/重试/分组/选文件补审后端已独立检查；跨仓库配置→固定入队→Worker准备→逐次访问权限→报告ACL、来源引用与历史关联已独立检查；备份恢复/readyz/托管配置/容量恢复及GitLab工具已独立检查。主检查读取费用/重试/补审/报告/复核/工具观察前端，历史评测固定对象、路径隔离、标签匹配和时序布局。设置/项目完整交互、全部时序生成契约、其余公共API差异及整体文档对应仍未逐项完成最终对账，标记decision-relevant未覆盖，不以分区测试推导全分支批准。完整base→head路径列表已获取，无依赖清单变化。

## 5. Findings

PRR-001（S2）resolved：非succeeded收敛时禁止第二Webhook，既有localhost反例及当前完整Python回归仍通过。

PRR-002（S2，原blocking=true，High）resolved：旧model_usage汇总忽略负TotalTokens并直接int64累加，异常供应商响应可导致错误完整标志/负费用。现负total记unknown；全局及阶段输入/输出累加前检查上限，溢出记unknown、不改变已有合计、费用为空且reason=usage_overflow。测试覆盖同阶段/跨阶段与输入/输出溢出；正常已知部分费用及独立复核价格保持。

PRR-003（S2，原blocking=true，High）resolved：旧urllib只限制socket静默，慢字节能拖延readyz；独立localhost timeout=1反例2.24秒仍ready。现父进程timeout限制整次子进程观察并kill/wait回收；慢头与慢正文均在回收余量内unavailable。仍有操作系统创建/调度/回收开销，不声明硬实时。

## 6. Required Actions

完成未覆盖分区及完整契约/文档对账，取得R1/R3/R4授权实际样本与独立人工标签及真实模型质量记录，取得R7专用MR写入授权并执行验收，随后最终main与1234部署证明。不得以合成上游结果代替真实质量。

## 7. Risk Assessment

费用新增reason通过现有自由字符串JSON/TS契约，unknown保守处理、不修改模型预算策略。健康检查增加短生命周期只读子进程，不修改服务或通知；父进程只接受精确ready输出及exit0，失败/超时不可用。独立修复复检没有发现放宽权限/重试/突变门控。GitLab人工编辑仍非原子PUT，专用验收必须无并发BASE/设置/人工修改。

## 8. Validation Evidence

应用源码ac9a5b5：go test ./...与go vet ./... exit0（platform60.619秒）；Python全26项ResourceWarning=error，9.347秒通过；前端tsc/Vite2.56秒通过。current head只追加四行implementation记录，已核对该文档delta，没有应用变更。独立修复复检在ac9a5b5运行healthcheck3项和模型用量定向Go测试均exit0。先前f69e310分区定向Go预算4.722秒、上下文24.273秒，运维15项与GitLab10项及readyz测试通过；这些仅证明当时分区，后续应用修复由上述新回归覆盖。

## 9. Coverage and Limitations

usage_overflow路径：callback→ToolTrace→汇总→run/retry JSON→TS字符串→面板→生成bundle已独立检查。无enum注册环节，N/A（reason本来开放字符串）。真实localhost慢HTTP为判别测试；未重新生产部署或执行真实模型/外部GitLab。独立修复检查不覆盖完整历史PR，主检视仍有上述未覆盖区。未获得外部CI证据，不能假设已通过。

## 10. Open Questions and Assumptions

当前origin无历史PR；授权样本与专用测试MR仍未提供。实际配置模型是否可用、真实跨仓库误报/漏报及外部Webhook投递没有由本轮合成证明解决。1234仍不能推定是本次版本。

## 11. Non-blocking Recommendations

评测CLI既有顶层legacy token字段仍直接汇总，与新Usage结构语义不同；后续统一时保留兼容，不能用legacy字段作可信预算/费用。该旧行为不属于此次新增修复范围，不将其当作本轮introduced finding。

## 12. Machine-readable Summary

```json
{"decision":"INCOMPLETE","mergeable":false,"review_kind":"RE_REVIEW","resolved_findings":["PRR-001","PRR-002","PRR-003"],"blocking_findings":[],"full_branch_review_complete":false,"external_acceptance_complete":false,"base_sha":"bcaeae230e5f101b484df6777c1a56aa8be80f39","head_sha":"c674a585eafee71bc51e3b88ab8052cbbb0a7c6e"}
```

决定在报告提交前冻结；报告自身为文档delta，提交后不代表新head获批准，后续必须重新绑定快照。没有发送平台评论或批准。

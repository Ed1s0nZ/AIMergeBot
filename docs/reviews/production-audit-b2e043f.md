# PR Review — AIMergeBot production audit @ b2e043f

## 1. Review Metadata

Repository: Ed1s0nZ/AIMergeBot；PR：无，本地分支差异。Author：当前协作分支；Reviewer：Codex。
Base origin/main：bcaeae230e5f101b484df6777c1a56aa8be80f39。
Head codex/production-audit-readiness：b2e043f856acd7fc91f1e43a752121ea6fb4512e。
Reviewed at：2026-10-05T06:53:17Z。INITIAL，READ_ONLY，首次限定高风险路径检视；无前次报告。
报告状态：STALE，原决定对当前流程已SUPERSEDED。后续修复为b40389a，新复核见production-audit-b40389a.md；本文仅保留原发现证据，不作为当前合并结论。

## 2. Decision

REQUEST_CHANGES；mergeable=false；blocking findings=1（PRR-001）。验收程序与平台不完整结果重提交契约不一致。整体126文件尚未逐路径完成检视，外部质量和真实GitLab证据也缺失，不批准整个分支。

## 3. Executive Summary

本轮新增预算、独立复核、补审、跨仓库及运维验收。已检视预算及跨仓库关键授权链，并核对专用MR验收程序的状态与副作用。发现覆盖不完整终态后的Webhook重放可以新建第二次审计，而程序把它当去重检查。先修复程序的触发顺序/终态门控，保留平台允许重新审计不完整结果的行为，再继续其他路径检视。

## 4. Scope and Change Map

已读：model_budget.go、retry_model_budget.go祖先身份/停止逻辑、context_repository_access.go、context_repository_tools.go、context_repository_runner.go、runs_store.go入队去重、project_access.go授权、gitlab-write-acceptance.py及HTTP测试。预算共享与未知用量阻止新请求；跨仓库准备/缓存前后重新检查任务owner与固定授权，读者需要权限交集。
未完整检视：其余分组、迁移、历史关联、恢复工具、设置及UI的全部路径。生成web/dist未逐行审查，已有TS/build资源hash证据不能代替源码检视。无远端PR/CI记录；全部diff由本地Git可取得，没有平台截断。INITIAL，无re-review对账。

## 5. Findings

### PRR-001 — 不完整终态后的去重验收可能触发第二次审计

- Severity：S2；blocking=true；confidence=High；category=correctness/side-effects；status=open。
- Location：scripts/gitlab-write-acceptance.py/run：first=wait(converged)后的duplicate=trigger；internal/platform/runs_store.go/enqueue非force查询。Head如上。
- Observation：converged接受succeeded及incomplete。之后程序无条件重放Webhook，但非force去重仅复用pending/running/succeeded/skipped，不包括incomplete。
- Trigger：专用MR首个任务有有效发现、comment sent，但整体status=incomplete（预算、范围或阶段覆盖不足）。
- Impact：第二次POST /webhook会新建任务、再次消耗模型并可能发布另一条任务评论。脚本在返回新ID后才失败，未阻止已经发生的副作用；和验证同一任务去重的意图相悖。
- Evidence：两处精确状态集合和先后顺序直接构成调用路径。已运行原生成功fixture仅succeeded，HTTP测试终态也仅succeeded，不能否定该反例。
- Remediation：调整验收程序以符合平台终态重提交契约，不能在已知incomplete后为检查去重而无条件再次提交；无支持证据时明确停止/标记该项未验收。不应为了工具测试而禁止用户重新审计incomplete。记录各次请求副作用边界。
- Verification：新增incomplete+sent的HTTP反例，断言没有第二次Webhook/新审计；保留succeeded去重/同note更新/冲突原生验证。修复诱发的状态/等待边界需重新检视。

## 6. Required Actions

修复PRR-001并补区分度测试；继续未覆盖路径检视；获取必要外部样本/专用MR授权；完成最终main及部署验收。外部缺口属于证据限制，不冒充源码finding。

## 7. Risk Assessment

预算是下一请求停止阈值，单请求可超额；静态证据及独立模型不能当运行复现。权限撤销前后调用可能已有外部请求，需保留现有fence与中断。当前PRR-001增加测试MR额外模型/评论副作用，不涉及执行仓库代码。

## 8. Validation Evidence

此前实施记录：完整Go/vet exit0；platform race88.470秒exit0；Python24项和TS/build通过。v13两项升级race2.996秒exit0；干净a65441c原生恢复/评论验收exit0。这些提交与当前head之间只有文档变化，仍不视作未知路径覆盖。
本次只读检查：本地diff/上述源码读取；125最终文件及提交补丁凭据扫描此前通过，仅输出摘要。未重跑新的incomplete反例，尚无修复验收。

## 9. Coverage and Limitations

限定高风险初检，非完整126文件审查。外部模型质量/真实GitLab投递未取得；授权不包含任意真实MR评论。最终部署与回滚未执行。

## 10. Open Questions and Assumptions

需要授权历史样本及专用MR；假设真实测试MR可能出现incomplete，平台已有该合法终态与可重新审计入口支持。没有以未验证假设支持APPROVE。

## 11. Non-blocking Recommendations

不要以增加测试次数替代反例；验收失败必须区分“未发请求”和“请求可能已被接受”。

## 12. Machine-readable Summary

```json
{"decision":"REQUEST_CHANGES","mergeable":false,"blocking_findings":["PRR-001"],"review_kind":"INITIAL","scope":"partial high-risk review","base_sha":"bcaeae230e5f101b484df6777c1a56aa8be80f39","head_sha":"b2e043f856acd7fc91f1e43a752121ea6fb4512e"}
```

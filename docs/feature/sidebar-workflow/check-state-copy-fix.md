# 检查发布状态文案修正

## F2 / Workflow Gate

- 输入：Confirmed REQ-021 / AC-019、UAR-003及既有优化授权；分支codex/sidebar-workflow，P10维护修正。审计报告记录于docs/reviews/usability-audit-8ffb6b2.md，不将该报告当整个分支批准。
- 当前行为：RunCheckAdvice无条件声称“尚未发布、未启用阻断”，而后续段落按服务端回执显示已发布及阻断，互相矛盾。
- 目标：移除无条件的否认；复用现有publication_state/published/blocking渲染作为发布状态说明，保留运行固定HEAD不代表PR最新提交的限定。
- 契约：不更改API/schema、检查状态机、授权、默认开关或平台写入。无新增需求或自动放行。缺失发布状态仍沿用现有disabled回退，未知状态沿用待确认。
- Gate：该文案修正的边界及验收已明确，可在F3提交推送后实施。其余UAR-001/002/004及自动闭环、GitHub差异仍暂缓，未改变完整需求。
- Maintainability：仅修改小组件run-advice.tsx，不触碰复杂模块；无新依赖。报告文件为独立审计产物，不混入生产阶段提交。

# AIMergeBot 产品化升级：任务接入

日期：2026-10-04。阶段：F0 / P0。分支：codex/platform-modernization。

用户请求：深度分析和优化项目，以 React 重构前端，增加登录等产品能力，改善基于 Git 的代码审计，评估采用 Eino 重构 AI Agent。

## Repository hygiene

- 起始分支 main，工作区干净；origin 指向 github.com/Ed1s0nZ/AIMergeBot。
- Go 1.21 / Gin / SQLite / go-openai / GitLab API；前端为两个 HTML 文件，无 React 工程。
- 未发现跟踪的 AGENTS.md、需求、设计、API 契约或自动化测试文件；cmd/test_tools 为工具调用示例。
- 本阶段仅增加分析及需求文档，不变更生产代码，不启动服务或调用真实审计/评论接口。

## Workflow Gate Report

- 检测阶段：P0 → P1；类型：跨前后端产品化、认证与审计架构重构。
- 必要上游：确认需求、权限策略、页面与状态定义、API 契约、审计设计与迁移计划。
- 已有：README、源码、配置结构；缺少上述产品契约。
- 当前允许：只读分析、基线检查、需求草案；生产实现暂不允许。
- 前置工作：形成有证据的现状分析，提交可确认需求；确认后完善设计及切片计划。
- 范围：React、登录及权限、项目/审计任务/结果工作流、Git 提交一致性、Eino 评估、可靠性与测试。
- 验收：需求可追踪、假设明确、问题有源码位置、基线验证如实记录。
- 风险：登录方式与团队隔离未确定；GitLab MR 以外的范围待确认；不得把框架替换视为准确率提升证据。

## Maintainability Gate Report

- inspected：main.go、internal/*.go、web/*.html。
- 触发：跨边界重构；gitlab_mcp.go 2708 行，react_auditor.go 1523 行，index.html 1228 行。
- 风险：工具模块 blocked，Agent high；混合工具 schema、网络 IO、搜索/分析、编排与文本解析，缺乏自动测试。
- 先拆分：需要；允许类型 boundary_redesign_plan。
- 建议边界：认证、HTTP、任务执行、仓库读取、diff 解析、工具、Agent、结果验证、持久化、前端页面。
- 验收：统一审计入口、提交一致性、失败不报通过、任务有界、历史迁移可验证。
- 后续验证：Go 单元/集成与竞态检查、前端类型与构建、登录/任务/结果端到端、固定案例质量评估。

## Feature Lifecycle Report

- 当前场景：小工具缺少访问控制与任务管理，审计上下文和结果可信度不足。
- 目标：用户登录后管理项目、查看与执行可追踪审计，以证据支持发现和复核。
- 影响：web、HTTP API、storage、GitLab 集成、Agent；认证和数据库迁移将影响旧部署。
- 文档载体：本目录 intake.md、analysis.md、requirements.md；确认后新增设计/计划。
- 提交计划：每个完成阶段只提交本阶段文件并推送功能分支；不合并或发布。
- 当前 blocker：需求尚未确认；Eino 版本兼容与真实模型质量尚未验证。

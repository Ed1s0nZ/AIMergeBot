# Implementation Plan: AIMergeBot 产品化

日期：2026-10-04；分支 codex/platform-modernization；阶段 F3。
输入 design.md 与已确认 requirements.md；最终合入并推送 main。

|切片|模块|行为与验证|文档与回退|
|---|---|---|---|
|1|internal/platform/store.go、auth.go、types.go|独立迁移表、管理员引导、cookie 会话、角色与撤销；临时数据库认证/权限/并发测试|配置与迁移说明；原表不变|
|2|repository.go、diff.go、agent.go|GitLab 快照、fork refs、diff 行号、Eino 只读工具、结果证据校验；fake GitLab/model 测试|审计工具与局限；旧内核保留供数据兼容|
|3|runner.go、routes.go|统一入队、持久化幂等、bounded workers、取消/超时/重启、分页轮询与 token webhook；HTTP/任务回归|API/状态说明；回退旧版本读旧表|
|4|frontend React/TS/Vite、web embed、main.go|登录/概览/项目/任务详情/复核/用户/事件；类型检查、构建、浏览器 smoke|开发与打包指南；保留数据库备份|
|5|migration/测试/README/CHANGELOG|历史导入、可重复固定案例评估、端到端/竞态/发布构建；修复旧示例编译|结果与缺失实证明确记录|
|6|Git main|完成要求逐项审查、工作区检查、合并并推送 main；无需发布版本|记录提交与最终验证|

每个实施切片先跑覆盖改动的检查，F4 完整实施后提交及推送；F5 验证记录和 F6 changelog/合并准备分别提交。避免在超过 800 行的旧文件加新行为，新增模块各自保持单一责任。

验证命令：go test ./...、go test -race ./internal/platform/...、前端 npm ci/typecheck/build、git diff --check；浏览器实测登录、任务错误/成功/取消/复核与管理员操作。fixture 不调用真实 GitLab 评论。模型质量对比基于固定标签案例并披露模拟边界；真实模型评估需要有效配置，缺失时明确未验证。

部署：备份 pr_agent.db，设置首次管理员凭证、Webhook token 与 GitLab/model 配置，构建新版本，单实例启动验证；无需 Redis/Postgres。回退使用原二进制和备份；新任务数据不会自动降级到旧表。

当前无未决产品选项；真实服务可达性、工具调用支持及生产吞吐待验证。若依赖版本不兼容，选择能满足 Go 环境的固定版本并更新证据，不盲目使用 latest。

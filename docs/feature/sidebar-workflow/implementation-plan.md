# 完整范围实施计划

- Branch: codex/sidebar-workflow
- 输入：Confirmed requirements.md；design.md 为设计契约。
- 阶段：F3；所有切片验收后才能认定目标完成。

| 切片 | 文件/边界 | 范围 | 验证 |
| --- | --- | --- | --- |
| S1 | workspace_queries.go/http_workspace.go；workspace.tsx/navigation | REQ-001–005：授权服务端待办、发现分页与导航 | 项目/source/context ACL、分页与总数、筛选同一发现、空错/窄屏/键盘 |
| S2 | workflow_actions/store、comparison、run detail | REQ-016–022：复审闭环、归并、接受、责任人、覆盖、对比 | 版本冲突、接受到期、不自动判修复、固定身份、CODEOWNERS |
| S3 | workflow_policy、workspace_metrics、前端页面 | REQ-006–009：策略、关联入口、成本、质量、格式化噪音 | 数据真实范围、unknown、语言敏感空白、配置生效范围 |
| S4 | integrations_store、notification_dispatch、各适配器 | REQ-010–011/014：全部渠道、汇总、记录/重试 | 协议 mock、业务码、脱敏、网络超时/unknown、持久重启、去重 |
| S5 | provider GitHub、checks、tickets、callbacks | REQ-012–013/015/021：GitHub/GitLab、工单、身份交互、检查 | 两平台固定快照/源码权限、重复提交、外部创建不确定、重放/撤权 |
| S6 | diagnostics、frontend、文档与CHANGELOG | REQ-023 及全范围完成验收 | Go完整/race/vet、前端检查构建、权限/迁移/恢复、实际UI QA、需求逐项核验 |

每阶段更新此文档并独立 commit/push。原文件仅注册/委托，不堆积新职责。API 使用 design.md 的拟议契约，实际变化记录在实现节。

## 验证与交付

受控 mock 校验所有平台协议，用户配置测试渠道后再主动进行真实发送；无凭据不擅用生产配置，不将 mock 当真实投递。代码不执行被审计仓库。运行 targeted tests 后再按影响扩大；功能完成跑完整 Go/race/vet 和 npm typecheck/build，确保 web/dist 与源代码一致。每个 AC 单独记录证据和未完成状态。

## 回滚

默认新增自动化关闭；保留现有路由。切片可回滚代码，新表不删除数据；外部成功发送/建单记录需保留，回滚不重放已知成功请求。不自动合并 main 或发布。

## 实现进度

- F1：27855ad，需求已确认并推送。
- F2：b019a83，设计已提交并推送。
- S1–S6：未完成。

### S1 初始实现（尚未完成全部验收）

新增 /workspace/findings 和 /workspace/tasks；发现查询按单条问题过滤与分页，不让不同问题分别满足严重程度/类型而误匹配。SQL 复用 target/source/context 授权，总数与页面同读事务。前端新增待办/发现入口及日常/管理分组，保留旧路由。

已通过新增发现分页/同一问题筛选/撤销关联仓库权限/脱敏测试、既有会话授权测试、frontend typecheck 和 build。下一步仍需 HTTP 契约负例、待办分类测试、具体发现定位、实际窄屏与键盘验证。S2–S6 未完成；不认定完整目标已交付。

S1 补充：HTTP 分类与非法项目/筛选负例测试通过；具体发现链接采用 /runs/:id?finding= 编码身份，详情渲染后聚焦对应发现，旧链接兼容。已有复核状态沿用 pending/accepted/false_positive/fixed，没有另造 confirmed 状态。目标测试 0.874s；上一轮 race 4.372s；最终 UI build 通过。实际视觉/键盘验证尚未完成，S1 保持进行中。

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

### S2 对比切片

新增 GET /runs/:id/comparison?prior_id= 与详情对比面板。先在同一事务授权两份快照，再读取有界结果（1 MiB）；不同项目、来源或 MR 拒绝比较。源指纹须重算一致且两边唯一才能标再次观察；伪造/重复锚点不匹配；策略/BASE 差异、非成功运行、200 条输出上限显式提示。未再观察不自动判修复；不继承旧复核。

测试验证分页/ACL/比较拒绝/坏 trace 不影响结果、歧义锚点与伪造指纹；最终定向 Go 0.932s，前端 build 成功。首轮测试 fixture 违反已有运行唯一约束及前端引用错误已修正并复跑，未隐藏失败。此切片不等于风险接受、自动闭环、通知或 GitHub 集成已完成。

### S4 协议基础（部分实现）

新增飞书/钉钉/企业微信/Slack/Teams Workflows/Webhook 的纯请求构造与业务响应分类，签名/正文中不回传 secret，超长响应与未知业务状态不标送达，Webhook/Teams HTTP 接受和实际送达分离。当前没有持久集成配置、SMTP、发送 worker、汇总、页面或真实外部发送；协议测试仅为受控验证，不能认定 REQ-010–015 已交付。官方网页动态/读取限制已记录，后续继续核对协议。

### S4 配置与队列切片（仍未全部完成）

新增 platform_integrations 和 platform_notification_deliveries 表、迁移、管理员 GET/POST/PATCH /integrations，以及管理员集成页面。11 类配置类型包括邮件、各机器人/Webhook、GitLab/GitHub/Jira/Linear；类型支持配置不等同适配器或平台全链路已完成。查询只返回 has_endpoint/has_secret，地址/密钥/SMTP 收件人不回传；expected_revision 强制版本校验，凭据默认保留、可替换或明确清除。

配置更新取消 pending/retry，sending 标 unknown 并撤销 lease，避免旧 sender 确认新配置结果。队列事件键唯一、同事务领取、随机 lease token fence、最多 5 次有界限流重试；过期发送标 unknown，不自动重发。新增测试覆盖普通用户拒绝、脱敏、保留/清除、版本冲突、关闭重开数据库、重复事件/双领/过期发送/修改渠道竞态和邮件换行注入。

未实现：实际 HTTP/SMTP sender、自动事件采集、定时汇总、投递记录页面/主动测试/重试接口、工单和机器人交互、真实平台验证。仍需继续完整范围，不以配置菜单作为交付完成证据。

本切片最终验证：集成/队列/协议/工作台定向 race 11.038s 通过；frontend typecheck+Vite build 通过；补充 HTTP 未登录与普通成员拒绝集成读写测试通过。实际 UI 验证与完整回归尚未完成。

### S4 发送器与投递页面切片

新增 HTTPS sender（禁用 proxy/重定向、TLS≥1.2、超时及 64 KiB 响应上限、解析平台业务状态）、SMTP TLS/STARTTLS sender（固定头、收件人校验、超时/取消）。默认拒绝 loopback/link-local/未显式授权的私网，管理员写入完整凭据时可添加私网 CIDR；TLS 证书仍正常验证。SQLite 数据库及 WAL/SHM 在 OpenStore 后设置 0600。发送前重查配置版本及原审计发起者固定快照权限；配置变更不等于撤回已到达外部的请求，结果记录可为 unknown。

新增管理员 GET /deliveries、POST /deliveries/:id/retry、POST /integrations/:id/test，测试仅将固定摘要排入已保存并启用渠道。Runner 单独有界通知 loop，随生命周期停止，默认空队列不发请求。管理页面提供私网允许列表、主动测试与分页投递记录；unknown 重试必须明确 acknowledge_duplicate，已接受/送达不可重试，attempt≥5 不再重发。

验证：受控 TLS server 请求与业务码测试、地址限制、租约/去重/修改配置、持久配置/脱敏和显式未知重试的 targeted race 6.960s；前端 typecheck/build 通过。未发送真实外部消息，SMTP 尚需受控协议交互验证，实际 UI QA 未完成；自动审计事件、每日/周汇总、工单/Git平台/绑定机器人/生命周期其余范围仍未完成。

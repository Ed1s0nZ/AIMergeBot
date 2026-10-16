# 功能落地复核：先完成真实链路

## 快照与范围

2026-10-16；Ed1s0nZ/AIMergeBot，`codex/sidebar-workflow`。
HEAD：`95d107a165c72cd9b124971d264198ec74fd766e`；本地 origin/main：`c50c957838e0f569ea920e7c8f4939fc0c383a49`。
另读取工作区中未提交的 `repository_budget_run.go`、`repository_scope_authorization.go`、`repository_budget_admission.go` 修改；这些内部协议仍在实施，不视为已验收或交付。

这是针对用户“为了做而做、无法落地”问题的源码路径审计，不是完整 PR 合并审查。重新核对前份 `practicality-audit-6f3afee.md` 的 UAR-001/002/004；以下判断替代其相应功能状态判断，不继承整分支批准。

## 结论

存在外围扩展先于核心链路交付、异常后不能继续、功能名称大于实际能力的问题。不能把全部新增功能判成空壳，也不能凭内部模块和测试数量认定功能完成。原生多平台能力仍不满足完整交付；不据此批准整个分支合并。

## 已证实的问题

### UAR-001：原生多平台执行仍未接通

- 状态：open；完整原生交付的高优先级阻塞；信心 High。
- 证据：`main.go:88` 装配现有 Repository；`internal/platform/runner.go:140` 的 Submit 要求 legacy，随后构造 GitLabRepository。`repository_binding_guard.go:25` 拒绝带 scope/budget 的 snapshot。`http_repository_management.go:15` 返回 unavailable，写路由返回 501。
- 调用检索：`newGitHubReadClient`、`bindRepositoryCredentialActivation` 没有生产调用方；新 `beginUserRepositoryBudget` 只有定义和测试调用；工作区 enqueue/worker/retry budget helpers 尚未接入通用调度。
- 影响：显式 GitHub/多平台绑定不能完成提交至结果的真实审计。预算和凭据协议不能被描述为已经约束生产审计。
- 处置：保留只读隔离；暂停继续增加相同层次的抽象和入口，下一交付目标应是现有设计下的一条可验证执行链路，并保留旧 GitLab 兼容。不能靠删除 guard 放行。
- 验收：受控固定提交的准入、读取、执行、发布与失败恢复通过后，再开放入口。私有 helper 单测不替代此验收。

### UAR-004：工单发送前失败仍无法恢复

- 状态：open；中优先级可靠性缺口；信心 High；不支持宣称完整工单闭环。
- 证据：`ticket_dispatch.go:17` 权限/配置失败进入 failed；`ticket_delivery_store.go:25` 只领取 pending；`ticket_links.go:migrateTicketLinks` 唯一约束为 run/finding/integration；`finding-tickets.tsx` 已有同渠道记录即禁用创建。`http_tickets.go` 没有恢复处理器。
- 触发与影响：预留后权限或配置改变，确认零 POST 的失败也永久占用该发现/渠道的位置；修好配置仍不能完成创建。与 AC-011 的“失败明确可恢复”有差距。
- 处置：先定义确认未发送、明确拒绝、结果未知的证据及恢复权限，再实现显式恢复。未知结果保持禁止盲重发，不得删除记录绕过去重。
- 验收：确认零 POST 失败可以恢复；可能远端成功的超时不得重复创建，保留原记录和历史。

### UAR-002：人工复核与远端门禁没有形成同步闭环

- 状态：open；中优先级产品契约缺口；信心 High；若门禁明确定义为原始结果，本身不是计算错误。
- 证据：`check_assessment.go:19` 从原始 Result.Findings 统计风险；`check_collector.go:17` 排除已有 delivery 的 run，不按人工复核变化刷新。
- 触发与影响：已发布阻断后人工标为 false_positive，远端检查不会因此改变。不能描述成“处置后自动解除门禁”。
- 处置：门禁到底代表原始报告还是授权处置结果，尚需明确；在此之前暂缓自动解除。fixed、accepted 均不能直接当作安全证据。
- 验收：若决定同步，需覆盖同 HEAD 的处置/撤销、重复事件和新 HEAD 隔离。

### UAR-005：工单是定位记录，尚不是可直接处理的问题说明

- 状态：open；低优先级可用性限制；信心 High；非安全缺陷，不因该项要求自动外发源码。
- 证据：`ticket_dispatch.go:24,34` 标题为通用 run 编号，正文只有 run/finding/commit，工作台 URL 可选；没有发现标题、风险说明或文件位置。
- 触发与影响：接收人需要再进入工作台查明问题；未配置 PublicURL 时连直接入口也没有。现有实现确实创建工单，但只能称为定位工单，不能称为完整问题移交。
- 处置：先确定允许外发的最小内容与接收权限，或明确将能力限定为工作台链接；暂缓自动派单/自动关闭等扩展。不直接把整个模型报告加入正文。
- 验收：按确认后的外发契约验证可理解性和敏感信息边界。

## 有实际价值、应保留的部分与反证

- 邮件、机器人及 Webhook 有实际发送器：`notification_sender.go` 包含 SMTP/TLS、HTTP 请求与响应分类，`Runner.Start` 启动通知收集/发送链路。不是只有配置页面；未用真实账号验证渠道可达性。
- CODEOWNERS 推荐读取固定提交，显式映射本地成员，并在读取前后检查权限（`owner_recommendation.go:98`）。它是可用的推荐依据，不能等同于自动认定负责人。
- 运行对比明确返回 not_reobserved，并提示“未再次观察不表示已修复”（`run_comparison.go:40,104`）。没有发现按提交消息或发现消失直接自动判修复的规则。
- 风险处置界面明确人工确认，且声明不表示运行复现；统计接口说明人工反馈不是真值、不能计算召回率。不要把这些受限但诚实的能力误判为造假。

## 实施取舍

1. 优先补已有能力的可执行路径与可恢复路径，暂停新增菜单、渠道和未接入协议。
2. 原生 reader、预算账本、权限隔离有合理目的，但目前属于内部基础，不继续按模块数计用户进度。未提交 worker/retry helper 不算自动复审/恢复已完成。
3. 自动判修复、自动关闭/合并、未明确语义的门禁联动继续暂缓，遵守用户“没想清楚先不实现”。

## 验证与限制

在上述工作区使用 Go1.27.2 执行两个针对性命令，均 exit 0：

- `go test ./internal/platform -run 'Test(RepositoryManagementHTTPReadOnlyAndNoWriteSideEffects|RepositoryManagementLegacyRoutesCannotCreateNativeBinding|RunCheckAssessmentPreservesCommitAndFailureStates|TicketDispatcherPersistsReceiptWithoutReplay|TicketClaimExpiryAndReceiptFencing)$' -count=1`（1.706s）。
- `go test ./internal/platform -run 'Test(NotificationProtocolsAndBusinessFailures|RunComparisonDoesNotInferFixOrMatchAmbiguousAnchors|OwnerRecommendationMapsExplicitAliasesWithFullSnapshotACL|OwnerRecommendationRechecksPolicyViewerAndCandidateAfterSourceRead|FollowupPinnedScopeDedupAndParentPreserved)$' -count=1`（1.458s）。

测试为本地 fixture，共 10 个顶层测试，含子场景；没有真实邮件、机器人、工单或模型请求。测试通过证明相应边界行为，不证明未接入的新 worker 协议、全分支安全性或外部服务验收。本轮仅增加报告，未改变代码、删除功能、推送或合并。

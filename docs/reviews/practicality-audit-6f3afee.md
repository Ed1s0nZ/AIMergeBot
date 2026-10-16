# 功能落地审计

## Review Metadata

- Repository: Ed1s0nZ/AIMergeBot；branch: codex/sidebar-workflow；无指定 PR。
- Base（本地 origin/main）: c50c957838e0f569ea920e7c8f4939fc0c383a49。
- Head: 6f3afee60ee8eae8eae9580344e3c6b72fb21cb8。
- Date: 2026-10-16；Reviewer: Codex；RE_REVIEW。
- 本报告替代 practicality-audit-8d276cc.md 的功能落地判断；不继承旧批准或测试。仅审计下述路径，不是完整分支安全审查。

## Decision

REQUEST_CHANGES；mergeable=false（针对完整原生多平台能力的交付）；UAR-001 尚未完成。已关闭危险写入口，不能继续把旧“可保存新绑定”的触发条件描述为当前行为。

## Executive Summary

存在“内部组件多、生产链路未接通”和“能发出动作、异常后不能继续”的实现。不能因此认定所有新增功能都是空壳：通知有真实发送器，工单也有实际投递。优先完成已有流程，不应继续增加渠道和菜单来表示进度。

## Scope and Change Map

- 重新核对仓库管理 HTTP/UI → Runner/guard，发现处置 → check assessment/collector，工单预留 → claim/dispatch → UI，以及通知发送器。
- Track A: UAR-001 改为“原生能力未交付、入口已隔离”；UAR-002/004 保持 open。旧 UAR-003 不重开。
- Track B: `git diff ea25ab59f56507a4fc629713f38a55774f8f9299 HEAD -- internal cmd frontend` 无差异；该生产快照之后的设计/探针不算生产装配。当前新增合同不在本次设计批准范围。
- 未逐行审计完整 base/head diff；不得用本报告批准整个分支。

## Findings

### UAR-001 — S1 / Blocking for native delivery / High / integration / open

- Location: internal/platform/http_repository_management.go:15、repository_binding_guard.go:19、runner.go:140,297,362；github_read_client.go:newGitHubReadClient；repository_credential_activation.go:bindRepositoryCredentialActivation，均绑定上述 head。
- Observation: 管理写入口明确 501，UI 只读；Runner 仍走 GitLab，拒绝 scope/binding。GitHub reader 和凭据 activation binder 均无非测试调用方。
- Trigger/impact: 用户希望按显式 GitHub/GitLab 绑定完成审计时无法执行；内部 reader、表和测试不能等同于原生多平台交付。新配置陷阱已隔离，既有绑定仍未完成执行/恢复。
- Remediation: 保持入口关闭，直到固定提交、身份权限、读取、审计、结果发布和恢复端到端完成；不能仅删 guard 放行。
- Verification: 受控环境完成绑定至结果及失败恢复，验证旧 GitLab 路径不回归。本次仅验证只读和无写副作用。

### UAR-002 — S2 / Non-blocking for existing raw-result semantics / High / workflow / open

- Location: internal/platform/check_assessment.go:19、check_collector.go:16 @ head。
- Observation: assessment 读取原始 Result.Findings；collector 排除已经有 delivery 的 run，不读取人工复核表来刷新远端状态。
- Trigger/impact: 高风险结果发布阻断后，将发现标为 false_positive，远端检查不会因此更新。人工处置与门禁不是完整闭环；若门禁定义为原始报告，该行为本身可保持，但产品不能承诺已同步处置结果。
- Remediation: 先确定门禁代表原始结果还是授权处置结果，再定义同 HEAD 的更新/撤回规则；不能把 fixed/accepted 直接当安全证明。
- Verification: 同 HEAD 人工处置、撤销、重复事件与新 HEAD 不串扰；本次测试仅验证原始状态计算。

### UAR-004 — S2 / Non-blocking / High / recovery / open

- Location: internal/platform/ticket_dispatch.go:17–28（符号 DispatchFindingTicket）、ticket_delivery_store.go:25、ticket_links.go:migrateTicketLinks/ReserveFindingTicket；frontend/src/finding-tickets.tsx:189 @ head。
- Observation: 发送前权限/配置失败可直接终结为 failed；claim 只选 pending，唯一约束和 UI 已有记录判断阻止再次创建。HTTP 路由没有工单恢复入口。
- Trigger/impact: 零 POST 的发送前失败，修好配置后同发现/渠道仍不能完成创建。
- Remediation: 先区分确认未发送、明确拒绝、结果未知，再定义显式恢复。unknown 不盲重发，不能删除唯一记录绕过防重。
- Verification: 零 POST 失败可恢复，可能创建成功的超时不得重复；现有投递/receipt fencing 测试通过，尚无完整恢复验证。

## Required Actions

1. UAR-001：原生能力继续标记未完成，生产装配与恢复通过前不开放入口。
2. UAR-002/004：先明确语义，再实施各自闭环；暂缓扩展相似入口。

## Risk Assessment

主要风险为交付范围被夸大和失败后的操作断点。保护身份、未知结果不盲重发属于必要约束，不应为提高表面成功率移除。Jira/Linear 当前工单仅发送定位元数据及可选工作台链接，不是包含完整分析与修复方案的工单。

## Validation Evidence

在上述 worktree 执行 Go1.27.2：`go test ./internal/platform -run 'Test(RepositoryManagementHTTPReadOnlyAndNoWriteSideEffects|RepositoryManagementLegacyRoutesCannotCreateNativeBinding|RunCheckAssessmentPreservesCommitAndFailureStates|TicketDispatcherPersistsReceiptWithoutReplay|TicketClaimExpiryAndReceiptFencing)$' -count=1 -v`，exit 0，5 个顶层测试及 dispatcher 子场景通过（权限变化、缺配置、未知传输结果等）。测试使用本地 fixture；未发送真实工单/通知/模型请求。sonic 提示当前 Go 版本回退 encoding/json，测试正常结束。

## Coverage and Limitations

这是源码路径及针对性测试审计，没有运行真实 GitHub/GitLab/SMTP/Jira/Linear 服务验收，也没有重跑完整 CI。通知协议与 SMTP/HTTP 发送器确实存在，但不等于所有渠道在真实账号上已验收。语言无关性也不等于每种语言都具有同等检出率。

## Open Questions and Assumptions

人工处置是否改变合并门禁？工单失败/未知如何核对恢复？未决前不实现猜测规则。新提交仅说明代码变化，不能单凭提交消息或未再检出认定“已修复”。

## Non-blocking Recommendations

保留可运行的 PR 审计、关联仓库上下文、工作台和通知；暂缓自动判修复、自动关闭/合并等未明确能力。研发探针和设计文档标为研发证据，不列为用户可用功能。

## Machine-readable Summary

`{"head":"6f3afee60ee8eae8eae9580344e3c6b72fb21cb8","decision":"REQUEST_CHANGES","mergeable":false,"native_execution_ready":false,"native_management_writable":false,"open":["UAR-001","UAR-002","UAR-004"],"full_branch_review":false,"external_mutations":false}`

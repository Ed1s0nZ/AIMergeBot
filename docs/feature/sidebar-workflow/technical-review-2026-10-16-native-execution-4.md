# Technical Plan Re-review：传输策略版本

- Date：2026-10-16；Reviewer：Codex，架构交接。
- Snapshot：4941ad9；Artifacts：legacy-transition/execution/budget/publication/runtime/activation合同。
- Supersedes：technical-review-2026-10-16-native-execution-3.md 的设计判断，不继承旧决定。
- Final decision：**Fail**。NTR-005/006设计缺口关闭；NTR-007 Major须先修正。

## Mandatory Criteria

| Criterion | Status | Evidence / gap |
| --- | --- | --- |
| Goals and requirements | Pass | 全部新任务统一固定证明，原范围保留 |
| Structures / changed fields | Fail | endpoint union清楚，但新旧transport用同一版本值 |
| Data flow | Pass | 所有producer→scope/账本→session→target串联，legacy source authority已传播 |
| Core lifecycle | Fail | transport变化应旋转generation的触发条件不明确 |
| Diagram | Pass | 新旧切换及发布路径齐全 |
| Handoff readiness | Fail | NTR-007不能让实现者自行选择material协议 |

## Track A：旧问题

- NTR-001–004：principal、projection、claim/author约束保留；新的repository_authority union未把legacy变成免校验分支。
- NTR-005：census_generation/had_error、单实例串行且非重入、worker lease检查、跨轮/重启反例已定义；设计resolved。
- NTR-006：全部新任务统一v51/scope v2，普通GitLab HTTP/path-prefix专门表达；新旧outbox、receipt union、全部producer及startup gate明确；设计resolved。尚无生产端到端证明，不能视为功能已交付。

## NTR-007 — Major：改变transport却复用旧material/profile版本

- Evidence：legacy-transition把legacy_endpoint.transport_policy固定为legacy_http_default_v1，同时要求固定代理、Go bridge、无重定向。现有repository_credential_material.go生成该版本，旧客户端沿upstreamHTTPClient/http默认transport；activation计划已规定换代理/网络策略必须升级material与实际client。
- Trigger/impact：切换后endpoint/token不变时，同名profile/material不能表达实际网络行为变化；若仅切session transport却保持旧fingerprint/generation，将违反“凭据证明绑定真正生效transport”的约束。当前生产未启用，不声称已有事故。
- Required fix：为正式固定会话定义独立版本；scope、material producer、activation save/reload、API/Git实际transport与恢复消费者统一使用它。切换时即使token/endpoint相同也要经新material旋转代次；不能将old producer的值改名就宣称新transport已实现。
- Verification：旧→新profile同token/URL触发一次generation推进、旧任务隔离；相同新profile重启/非凭据保存不推进；API和clone实际proxy/redirect规则一致；未知profile拒绝、零fallback。

## Implementation Readiness / Re-review

先修writer合同，再审；本轮无生产修改/外部调用，未重复运行代码测试。现有两个material测试仅证明旧协议，不证明新profile。新方案需明确版本值和启动/保存传播后才能开始N1等生产实施。

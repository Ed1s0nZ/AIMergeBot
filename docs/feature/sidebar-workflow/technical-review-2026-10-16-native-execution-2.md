# Technical Plan Re-review：原生执行

- 日期：2026-10-16；Codex；快照9f3dac4；前报告technical-review-2026-10-16-native-execution.md @1d5cc97。
- Artifacts：execution/budget/API diff/resource/publication五份合同及S3启动依赖。
- Final decision：**Fail**。NTR-001–004设计缺口关闭；新增NTR-005 Major必须补齐。仅设计复审，不代表生产功能或完整代码审计。

## Mandatory Criteria

| Criterion | Status | Evidence / gap |
| --- | --- | --- |
| Goals/scope | Pass | 原Confirmed范围保留 |
| Data structures | Fail | principal、projection、claim身份已修；native poll持久基线缺字段/唯一键 |
| Data flow | Fail | admission/发布已清楚；observe与admitted在poll路径未区分 |
| Core lifecycle | Fail | 缺初始化与失败seen提交边界 |
| Diagram | Pass | 五份合同组合覆盖跨边界流程 |
| Handoff readiness | Fail | NTR-005需作者给可执行合同 |

## Track A：旧finding

| ID | 当前状态 | 当前证据 |
| --- | --- | --- |
| NTR-001 | resolved（设计层） | budget principal union、actor NULL/正ID及SQL CHECK；hook/poll/comment source明确且HTTP不能提交service |
| NTR-002 | resolved（设计层） | OwnedContentV1从意图/GET同构造，POST/PATCH差别、可选字段/时间/marker均定义；不hash任意GET JSON |
| NTR-003 | resolved（设计层） | 冻结claimed run/target/outbox三元组与独立每claim随机token；desired推进不覆写sending/unknown；旧回执不完成新代次 |
| NTR-004 | resolved（设计层） | 写前GET/user来源及expected author持久化；没有可信主体时unknown禁止候选自证，Check app身份区分ACK/未核实 |

## NTR-005 — Major：native poll基线尚未区分观察与成功准入

- 位置：native execution计划“poll”、N5 platform_native_pr_states字段组，以及现有polling.go/recordLegacyPollSeen。
- 观察：新合同只给PR当前状态observed_head/observed_at和“持久基线去重”要求，没有native seen/admitted字段、复合身份key、首次完整枚举标记与失败事务边界。现有legacy seen键只有project_id/mr_iid，不可直接作为native provider/仓库重绑定后的基线。
- 触发/影响：若实现者复用legacy seen，项目改绑另一远端可沿用旧seen；若用observed_head作为已处理HEAD，hook已观察但准入失败会使poll跳过，形成永久漏审。部分分页失败也可能提前宣称baseline完成。
- Required fix：作者定义native poll基线及每PR seen身份（至少内部项目/provider/origin/远端ID/PR），区分observed与admitted/initial-baseline；只有明确initial baseline或成功入队/查重才能更新seen，失败保持未见；初次枚举不完整不标initialized。说明有界分页怎样前进，避免预算每轮用尽却总从第一页开始重做昂贵Snapshot。
- Verification：同数字ID跨provider/改绑远端；observe成功但enqueue失败；hook/poll并发/重复；第一页成功第二页失败；初始不扫描选项；新HEAD/未知source/限额/重启。断言run/event/seen/initialized，而非只检查日志。

## Forward-risk Review

修订principal不把0变成通用后台权限；projection不把响应额外字段当body变化；claim token防同实例ABA；没有在本次开放App自动token生命周期。上述修正没有发现需恢复原错误模型的理由。Native baseline是新补齐需求，不要求所有OS先有硬沙箱，不改变语言/多项目目标。

## Implementation / Verification Readiness

未执行生产N1–N6，schema3/端到端/真实账号证明仍待实施。设计Pass应与生产验收分开。此次未运行代码测试，代码未改；静态对照现有legacy poll“失败不mark seen”行为，要求native至少保留这个不变量。

## Re-review Requirements

作者补native baseline数据、生命周期/分页和反例后提交writer变更；新评审保留NTR-005编号并核对全部合同及NTR-001–004传播。当前Fail不因作者补文自动转Pass，禁止据此开始生产实现。

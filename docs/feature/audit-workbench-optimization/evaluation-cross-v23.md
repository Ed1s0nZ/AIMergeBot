# v23 契约对比回归：执行失败保留

代码 dede8e52dadf0600200c65c5a01e227cd7b8c412；原始证据 `/Users/worker/.codex/evaluation-artifacts/aimangebot/cross-v23-20261006`，SHA256归档验证通过，进程退出0表示receipt写完，并不表示每例审计成功。

| 用例 | 状态 | 发现数 | 失败分类 | token |
|---|---|---:|---|---:|
| case-101 | incomplete | 1 | — | 136709 |
| case-102 | model_or_agent_failed | 0 | agent_step_budget | 207307 |
| case-103 | model_or_agent_failed | 0 | agent_step_budget | 197338 |
| case-104 | incomplete | 0 | — | 105830 |

合计报告token 647184，未证明效率提升。101主要单位错配条件风险有发现；104无发现且保留覆盖说明。102在8次超大调查更新后、103在无效锚点/未关联来源修复后耗尽agent_step_budget，均不能按零结果计安全或验收通过。

新的契约对比提示未证明消除语义误报：102没有完成判定；103正例最终没有接受发现，不能称召回成功。保留支持调查和失败trace，不增加预算或删除失败刷分。下一窄修复提供实际UTF8字节数/上限、来源ID/调查ID、正确BASE/HEAD锚点的修复指引；仍需要新轮次验证，不以更清楚错误消息证明模型一定修好。

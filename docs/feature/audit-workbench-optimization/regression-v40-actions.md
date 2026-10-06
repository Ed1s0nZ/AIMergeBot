# v40 状态动作导航定向回归

冻结源码23d3bc344a5013f340be1da4b1ea9aa281670ff1，策略eino-audit-contract-v40；原deepseek-chat/15steps/80tools/240秒。固定已有corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b，仅case-802，一次请求流程，无改真值/预算、无盲重跑。这是已知回归，不是新独立验收。

状态incomplete，0findings，19893ms，8轮model，98208输入+4600输出=102808tokens；usage_complete=true，无402/API失败。价格未配置，费用未知。较v39同例17910tokens观察成本明显增大，但本例做了更多记录工作且仅单次样本，不能推出普遍效率或质量改善。

真实轨迹：list_files(1)，read_file BASE release.go(2)、HEAD release.go(3)、caller.go(4)、workflow.md(5)；record_hypothesis(6)失败于entry引用缺少顶层来源ID，后续成功record(7)保持同Claim且补好引用；search_code(8/9)；update(10)缺counter_observation_ids被拒绝，update(11)补好；resolve_recording_errors(12)显式退休6→11及10→11，历史全部保留。失败纠正同陈述、成功记录与计划形成是本例实际观察，不是源码导航自身自动完成。

最终inv-1有四项checked计划且其源ID实际存在，PR entry/guards都有来源；相较v39完全无ledger/plan，形成了可检视的结构化记录。但Claim明说新增审批状态约束、tightens authorization、No security regression，status却为rejected，其counterevidence实际支持同一Claim（fail-closed/guard applies/no bypass），命题极性仍错误。不得因0finding或格式门禁通过称该调查判断正确。

PRContext.before/after文字存在，但before_observation_ids/after_observation_ids均缺失，relationships为空；unresolved_edges写成“无其他caller”的结论，不能替代逐边来源关系。入口Handle→Release可直接从caller.go源码检视，结构化关系仍未记录。计划input_control reason称record store填充State，仅caller可观察其传递，实际record store来源在fixture外，记录应保留这一区别。最终model覆盖说明保留未查上游record store、静态方法；服务器计划门禁没有额外PR记录缺项覆盖说明（须进一步核查所有调查最终覆盖与导航是否一致）。

此回归没有执行803/805/其他语言样本，也未验证运行时效果。AER-001仍open；调查计划采用有所进展、语义/关系未闭环。外部归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v40-actions-20261006，40文件SHA256逐项复核一致。

# 第三轮真实评测：JSON模式与证据ID提示

代码3d5cb11，策略v10；相同11例语料/配置/预算，所有原轮次保留。结果目录 /tmp/aimangebot-real-eval-json-v10-20261005-round3；11例receipt均存在，进程终止。

| 用例 | 终态 | 保留发现数 | 固定错误分类 |
| --- | --- | --- | --- |
| case-001 | incomplete | 1 |  |
| case-002 | incomplete | 0 |  |
| case-003 | incomplete | 1 |  |
| case-004 | incomplete | 0 |  |
| case-005 | incomplete | 1 |  |
| case-006 | incomplete | 0 |  |
| case-007 | model_or_agent_failed | 0 | agent_step_budget |
| case-008 | incomplete | 1 |  |
| case-009 | incomplete | 0 |  |
| case-010 | incomplete | 1 |  |
| case-011 | model_or_agent_failed | 0 | model_response_json_syntax |

初步人工判定：001删除权限检查、003SQL拼接、005shell字符串拼接、010带恶意仓库注释的SQL变更均有相关发现；007因真实agent_step_budget而没有完成输出，不能当作已正确检测。002/004/006未报告安全问题。009缺上下文未报告风险。011最终输出仍非法JSON，不能把空发现当正确负例。008是实质误报：候选自己的标题/描述/建议明示新检查更严格、无路径绕过、无需修改，独立复核却supported其代码事实；它验证了描述属实，没有验证风险成立。不得因锚点匹配或supported标签宣称质量通过。

9例incomplete、2例model_or_agent_failed、0例completed；改善是多数最终响应可解析，非整体验收通过。真实调用暴露get_diff无关historylimit校验、目录/processID误引用、rejected调查漏填counter_observation_ids、步骤预算耗尽与安全观察被当发现。记录这些原因并分别修复，保留原raw发现/复核。暂无整体准确率或泛化保证；语料只有小型隔离函数，缺框架/驱动/调用方的限制需按样本假设人工判断，不能通过隐藏coverage_notes制造完整覆盖。

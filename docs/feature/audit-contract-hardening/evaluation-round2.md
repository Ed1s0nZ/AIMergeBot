# 第二轮真实模型评测：连接恢复，质量验收未通过

模型配置已在本机更新，最小连接探测HTTP200。密钥只存在被Git忽略的config.yaml（0600），临时密钥文件已删除；Git跟踪文件匹配数0。此报告不包含密钥或原始服务响应。

实际调用模型deepseek-chat，代码b42bd8a，固定11例语料，原失败轮保留；当前本机结果目录 /tmp/aimangebot-real-eval-valid-20261005-round2。整套运行已终止且11例均有receipt。

| 用例 | 终态 | 保留发现数 | 耗时毫秒 |
| --- | --- | --- | --- |
| case-001 | model_or_agent_failed | 1 | 14922 |
| case-002 | model_or_agent_failed | 0 | 7126 |
| case-003 | incomplete | 1 | 16888 |
| case-004 | incomplete | 1 | 19511 |
| case-005 | incomplete | 1 | 16385 |
| case-006 | model_or_agent_failed | 0 | 12386 |
| case-007 | incomplete | 1 | 19445 |
| case-008 | model_or_agent_failed | 0 | 14818 |
| case-009 | incomplete | 1 | 19843 |
| case-010 | incomplete | 1 | 15986 |
| case-011 | model_or_agent_failed | 0 | 9390 |

结论：5例model_or_agent_failed（coverage明确Invalid final model response），6例incomplete，0例completed。保留下来的发现不等于通过；负例产生候选、独立复核unavailable及缺少调用上下文需要逐例归因。没有计算准确率或选择性删除失败结果。当前格式错误的具体原始输出未保存，不能拍脑门归因为Markdown/字段错误；需有界脱敏诊断证据后修复。真实质量验收尚未完成，main合并与1234部署仍保留在原目标内。

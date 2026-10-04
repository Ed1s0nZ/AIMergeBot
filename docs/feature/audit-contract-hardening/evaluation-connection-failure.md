# AIMergeBot 真实模型评测：连接失败记录

本轮没有产生可用于评估检测质量的模型结果。11 个用例均在主模型生成阶段失败；不能据此计算漏报率、误报率、precision 或 recall。候选发现数为零不代表代码安全，也不代表模型漏报全部风险。

- 模型：`deepseek-chat`
- 审计策略：`eino-audit-contract-v9`
- 已提交审计代码版本：`b664bc0df74b67ec332a6c8d110a3d43d920754d`；评测入口源文件另存于 `harness-source/`。
- 语料 SHA256：`bda2c67ece912b5d4ed0fa03a27016248f9e8c1f7df4db23541c337b07727319`
- 范围：11 个自建静态回归用例，正负对照、删除保护、跨文件保护、缺失上下文和仓库提示注入；不是 OWASP 官方 Benchmark 或生产代表性样本。
- 本轮独立复核开启、时序图关闭；由于主生成失败，均未进入复核。
- 没有执行被审计代码，没有访问生产 GitLab、数据库或修改模型配置。

| 用例 | 运行结果 | 发现数 | 耗时 ms | 已报告 token |
| --- | --- | ---: | ---: | ---: |
| case-001 | model_or_agent_failed | 0 | 994 | 0 |
| case-002 | model_or_agent_failed | 0 | 392 | 0 |
| case-003 | model_or_agent_failed | 0 | 295 | 0 |
| case-004 | model_or_agent_failed | 0 | 373 | 0 |
| case-005 | model_or_agent_failed | 0 | 378 | 0 |
| case-006 | model_or_agent_failed | 0 | 322 | 0 |
| case-007 | model_or_agent_failed | 0 | 408 | 0 |
| case-008 | model_or_agent_failed | 0 | 308 | 0 |
| case-009 | model_or_agent_failed | 0 | 327 | 0 |
| case-010 | model_or_agent_failed | 0 | 350 | 0 |
| case-011 | model_or_agent_failed | 0 | 367 | 0 |

## 连接诊断

独立的最小模型请求返回 HTTP 401，允许公开的错误码为 `invalid_api_key`。使用相同配置进行的单用例诊断仍未生成结果。原始11次失败保留，不覆盖、不计入有效检测分母。逐用例错误未保留原始提供商正文，无法逐条证明相同401；主失败原因与连接诊断一致，但应区别观察与推断。

已报告 token 为0，同时 usage不完整，不能据此声称请求免费或实际计费为0。没有可信的TP/FP/FN/TN计数，所有质量指标为未评估。

## 后续

在系统设置更新有效模型配置后，使用新的结果目录运行完整语料；保留本轮与后续轮次，明确配置和版本变化。真实返回结果须人工按风险机制、锚点和保护条件判定，机械锚点匹配只是辅助。单轮小样本不能外推全语言或生产准确率。

方法参考：[OWASP Benchmark](https://owasp.org/projects/benchmark) 的已知正负例与独立预期结果；本语料只借鉴评测区分方式，未运行其官方套件。

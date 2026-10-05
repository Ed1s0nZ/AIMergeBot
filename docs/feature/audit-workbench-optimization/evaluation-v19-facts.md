# v19 自建 PR 回归：运行事实（待逐项质量判定）

代码版本：66b2dcfd1670eff4e3f737dc48de4dfebfc605c9；策略：eino-audit-contract-v19；语料摘要：bda2c67ece912b5d4ed0fa03a27016248f9e8c1f7df4db23541c337b07727319。
原始证据目录：`/Users/worker/.codex/evaluation-artifacts/aimangebot/v19-20261006`，11个结束 receipt 均保留；此轮不包含关联仓库。连接探测 HTTP200，未修改配置。

| 用例 | 外部期望 | 执行状态 | 发现数 | 主调查置信 | 独立静态复核 | 毫秒 | 报告token |
|---|---|---|---:|---|---|---:|---:|
| case-001 | positive | incomplete | 1 | supported | supported | 26205 | 116058 |
| case-002 | negative | incomplete | 0 | — | — | 2824 | 13554 |
| case-003 | positive | incomplete | 1 | supported | supported | 12382 | 52190 |
| case-004 | negative | incomplete | 0 | — | — | 2861 | 13691 |
| case-005 | positive | incomplete | 1 | supported | supported | 12322 | 48486 |
| case-006 | negative | incomplete | 0 | — | — | 2509 | 13738 |
| case-007 | positive | incomplete | 1 | supported | supported | 18676 | 78415 |
| case-008 | negative | incomplete | 0 | — | — | 7260 | 39636 |
| case-009 | uncertain | incomplete | 0 | — | — | 2913 | 13630 |
| case-010 | positive | incomplete | 1 | supported | supported | 13365 | 53396 |
| case-011 | negative | incomplete | 0 | — | — | 3476 | 21224 |

合计报告 token：464018。这是提供商报告的用量，不是价格或精确计费金额。11例全部含 coverage_notes，不能把5条发现/零结果直接记作TP/TN或宣称审计完整。未运行利用代码。

工具错误仅见case-001和case-007：case-001两次来源观察校验拒绝及两次锚点不在改动行，case-007两次锚点不在改动行。拒绝规则保持，后续修正提交有结果，但早前错误仍在覆盖不足记录。缺少调用入口/框架/下游实现的样例限制另计，不与模型失败合并。

下一步：逐条结合外部理由审阅机制与反证；加入多语言跨仓库成对样例并独立运行；最后再完成需求验收。

## 人工机制核对与历史用量限制

逐条核对实际描述、改动锚点和外部语料假设：001 删除资源所有者检查（条件依赖 store 无隐式授权）；003/010 参数化变为 SQL 拼接（常规 db.query 语义）；005 argv 转 shell 字符串（输入控制条件）；007 删除 canonical 路径包含检查。5条主机制均符合语料约束，003/010潜在数据泄漏影响依赖具体查询权限，不能从样例证明数据库写入或任意SQL执行。负例002/004/006/008/011无发现，分别保留所有权、参数绑定、argv边界、canonical包含检查和跨文件授权；009缺失 gateway 实现不作安全结论。以上为人工静态条件判定，不是运行复现或生产 TP/TN。

历史v12轮4报告token322559，对比本轮464018高43.86%。metadata同语料/model/endpoint摘要/rounds/toolcalls/temperature/timeout/验证与时序图开关，但 max_tokens、verification_model、model_budget 改变，因此不是同配置A/B，不能归因代码，也不能宣称端到端节省成本。同配置效率对照仍需补做。

# 记录失败的显式纠正

主调查新增 `resolve_recording_errors`；独立复核及压缩只读工具列表不提供它。先成功记录/更新同一调查，或成功提交同一候选，再用实际回执ID明确关联：

```json
{"corrections":[{"failed_observation_id":"observation-4","corrected_observation_id":"observation-6"}]}
```

每次1–8对；失败须仍pending，纠正须在之后、相同主快照和当前调查阶段、成功且无分页不足。调查ID相同且非空；候选的原local id、investigation_id、file、type相同且身份必需字段非空。原调查Claim及候选Title/Description/Trigger必须字节相同，防止把失败陈述换成另一结论。BASE/HEAD锚点、证据和来源编号允许修正；改写陈述时旧失败仍未解决。整批先验证再清除，不满足则任何旧pending都不清除。拒绝消息固定，不回显任意输入ID。

这只声明记录操作已纠正，不证明其命题/漏洞语义正确。原始失败和纠正事件都保存在trace与检查点。源码失败、未读分页、缺少计划、仍在调查的hypothesis和风险链缺项继续保留；另一候选成功不消除未提交候选。无local id的失败无法可靠归属，继续未解决。重复已清除引用会被拒绝，不伪造重复成功。

工具是非源码导航，evidence_eligible=false；每次调用仍计入原工具与步骤预算。该导航的失败只保留trace，因为它没有新增调查/提交产物，原失败仍pending；避免失败纠正操作自身制造无法纠正的新pending。没有自动重试、扩大预算或重写历史报告。策略v37。

# v36 真实回归

冻结591fe996415120aa2b9b8501cdea0d7c9acf5b14，原接口/模型和相同已见701–706语料；15steps/80tools/240s，无预算调整。六例均UsageComplete=true、无HTTP402、总459281tokens；费用价格未配置。输出全部incomplete。此为单次回归，不能估计总体准确率或将前后随机输出归因于提示变化。

| 案例 | 发现 | tokens | 调查状态 | 残余证据 |
|---|---|---|---|---|
| 701 | 1 | 131533 | supported | 缺relationships；先后错误侧/未链接ID被拒绝并修正；独立复核invalid_observation，最终unavailable |
| 702 | 0 | 38168 | rejected | 首次补齐账本及收尾；模型仍将静态方法说明列为coverage_notes |
| 703 | 1 | 117869 | supported | 下游与映射已读；无source-linked relationships，保留不确定性 |
| 704 | 0 | 90664 | rejected | 创建计划子ID未链接父ID被拒绝后修正；已收尾；外部实现未知仍记录 |
| 705 | 0 | 64308 | rejected | 固定下游默认值已读并反证；未知HTTP适配器不能由状态标签证明 |
| 706 | 0 | 16739 | 无账本 | 正确排除历史sink，但调查计划缺失仍存在 |

观察：704/705相比v35已显式收尾，702相比v35补建账本；没有自动解除覆盖门禁或制造关系。701独立复核引用失败与703风险链记录缺失仍未解决；四个负例不能按零发现计为全面通过。模型将“不执行运行时验证”写入coverage_notes与已有静态审计提示不一致；不根据字符串自动删除未知或重写模型结论来制造completed。

286文件哈希归档验证，冻结元数据与实际metadata代码/语料/预算逐项匹配。私有合成源码、工具回执保存于 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v36-20261006，未纳入仓库。下一步优先核对独立复核invalid_observation的具体边界及原始输出、缺relationships与单函数控制流的真实记录需求；先定位通用机制，不继续无依据堆提示或重试同集。

## 人工维度核对及勘误

不能把702/704/705的rejected状态解释为语义正确收尾。三条Claim实际陈述兼容/无新增风险，counterevidence支持这些Claim而不是反驳它们；status与命题极性不一致。v36导航已明确supported相对claim、兼容结论无需finding，但模型仍未遵守。此前用户消息“正确补建或收尾”过强，本节更正为“记录补建/状态转移已发生，语义一致性未通过”。不回写原账本或原回执。

| 案例 | 定位/PR归因 | 判断相对独立真值 | 证据与链路 | 漏报/误报口径 | 记录质量 |
|---|---|---|---|---|---|
| 701 | BASE service.py:9匹配删除guard | 条件静态权限回归方向支持 | BASE/HEAD均有源引用，relationships为空，独立复核unavailable | 有候选命中，不按完整TP计算 | 来源纠正成功，复核引用失败类别无法还原具体ID |
| 702 | HEAD新增guard，summary对照正确 | 无finding且识别更严格保护 | 双侧读源、四项计划checked；无运行证明不等于源调查缺失 | 观察到0告警，incomplete不计正式TN | no-risk Claim被rejected，极性错误 |
| 703 | HEAD gateway.js:7匹配变化 | 在已声明gateway→archive路由假设下风险方向支持 | 双侧和固定archive源已读；关系未记录，不能认证完整链或无条件网络结果 | 有条件候选命中，不按完整TP计算 | description/trigger未充分限定实际部署映射，链路缺口仍存在 |
| 704 | query override正确归因；下游用认证身份 | summary识别防护，无finding | 固定archive.rb和deployment.conf已读；关系不确定明确保留 | 观察到0告警，incomplete不计正式TN | no-risk Claim被rejected，极性错误；一次父/子来源链接错误已修正 |
| 705 | BASE显式false→HEAD缺省正确识别 | 固定Python默认值保护支持条件兼容 | 两侧和固定下游均读；HttpClient适配器仍未知，summary响应不变断言范围偏强 | 观察到0告警，incomplete不计正式TN | compatibility Claim被rejected，极性错误 |
| 706 | log常量变更，旧sink未变 | 正确排除无PR关系的历史危险操作 | 源码已读，无账本计划，调用者未知记录 | 观察到0告警，不计正式TN | 无计划仍为缺项 |

这是真值与回执人工核对，不是外部专家盲审。六例总459281tokens，价格未配置，不把不可得货币费用记为0。首次v34样本独立于被测模型，其生成和先前结果不构成外部专家标签；后续v35/v36皆为已见回归。当前不报告生产准确率。下一步独立只读检视源代码与门禁，区分实现缺陷与模型语义限制；不能以合法状态转移代替正确反证，也不能放松门禁自动删除覆盖缺口。

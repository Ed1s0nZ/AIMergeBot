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

# F3 实施与验证计划

1. 专职verification_aspects.go与测试；finding_verification新增字段/strict parse/ID所有权检查/结果深拷贝；prompt声明四项；policy升级v29。
2. 前端finding-verification/API按新增字段显示判断/理由/来源，旧记录明确缺少分项，不回填。不改变复核写入、授权或路由。
3. meaningful tests验证missing/duplicate/unknown kind/nonfresh/unlinked/rejected/aspect partial以及四项正路径；SDK模拟改为明确分项，历史记录读取仍不改变。相关Go/race、全量、类型/构建、实际受控UI验证。
4. 冻结回归模型检查，保留预算、失败、tokens与分项质量；使用既有语料标作回归，不改叫隔离；后续独立/真实PR评测需要真实来源固定SHA与人工真值。
5. 调查计划/具体覆盖/风险排序/增量持久化另切片，各自有契约和验证；不以第一切片替代整体目标。

回滚：源码/静态资产一并回退，新增JSON字段旧读取不影响（历史查询由现有类型），已存记录保留；旧策略结果不重写。不会部署/合并。

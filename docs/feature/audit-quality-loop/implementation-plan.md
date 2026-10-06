# F3 实施与验证计划

1. 专职verification_aspects.go与测试；finding_verification新增字段/strict parse/ID所有权检查/结果深拷贝；prompt声明四项；policy升级v29。
2. 前端finding-verification/API按新增字段显示判断/理由/来源，旧记录明确缺少分项，不回填。不改变复核写入、授权或路由。
3. meaningful tests验证missing/duplicate/unknown kind/nonfresh/unlinked/rejected/aspect partial以及四项正路径；SDK模拟改为明确分项，历史记录读取仍不改变。相关Go/race、全量、类型/构建、实际受控UI验证。
4. 冻结回归模型检查，保留预算、失败、tokens与分项质量；使用既有语料标作回归，不改叫隔离；后续独立/真实PR评测需要真实来源固定SHA与人工真值。
5. 调查计划/具体覆盖/风险排序/增量持久化另切片，各自有契约和验证；不以第一切片替代整体目标。

回滚：源码/静态资产一并回退，新增JSON字段旧读取不影响（历史查询由现有类型），已存记录保留；旧策略结果不重写。不会部署/合并。

第二切片文件：investigation_plan.go/测试 + types/agent_investigation/agent最小接入 + investigation-plan.tsx/API/run-detail；SDK覆盖record→pending update失败→checked resolve、保留计划、禁止删改、错误ID、unavailable、不变历史、主返回覆盖notes与分组保留。全量Go/race/TypeScript/build、实际组件和真实模型回归核查（仍旧集回归不是新隔离）。不扩大预算/执行权限/源码读取身份。

第三切片：agent_investigation继承与getter克隆；真实Git工具来源测试覆盖省略保留、返回别名隔离、更新移除源失败保留账本、继承后预算、显式无效context失败；相关PR/plan/verification及全量Go回归，不改变旧记录。

第四切片实施计划：新增独立 audit_priority.go，词法信号最多权重3、默认1，不依据文件扩展名判断支持语言。audit_groups 排序和分组取最大权重；grouped_agent 的调用/时间按剩余权重分配，调用先给可覆盖的剩余组各保留1次，不增加全局工具预算。progress带 priority_weight，历史0按默认1解释；模型与UI明确这是调度提示。添加稳定排序、去掉guard优先、未知文本不漏、排除与总上限、预算极少/溢出保护的专项测试，再运行分组回归与全量。

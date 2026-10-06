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

第五切片文件：repository_availability.go及测试；git_repository最小错误分类；agent_tools记录标记/model_usage请求门禁/agent错误归因/grouped停止后续付费阶段。优先SDK真实HTTP计数测试，普通read错误仍可修复。环境许可未恢复，F2/F3与后续F4提交暂待，不自动同意许可。

第六切片文件：primary_round_budget.go/测试、agent最小重写包装、分组stop_reason/合并来源保护、audit-eval失败分类、UI类型与已有分组状态显示原因。验证原2/4/8轮最后一次请求可报告、不增加配置轮数、提醒不累计或污染原消息、原源码/工具消息不变、未完成不伪装通过、分组budget类型由真实SDK错误产生、模型伪造组原因被清除、receipt分类。Go相关/race/全量及前端构建/受控实际UI，保留现有真实失败而不为单例调高预算。

第六切片UI接线：沿用服务端optional stop_reason字段；failed组展开显示原因，未知/历史缺失分别提示，不显示模型自行宣称的原因，不将失败算完成。既有details键盘交互与移动端布局沿用。契约/设计已具备；P6→P9增量验证，实际组件控制fixture及TS/Vite检查。

第八切片：recording_feedback.go pure有限码与测试；toolOutput optional字段，ledgerChange/submit最小接线和prompt；真实SDK验证record后回复非源码缺项→update补有效来源→submit无gap，旧预算/坏ID仍拒绝。检查真实BASE/HEAD源关联与accepted快照，未完成feedback不能伪装source。相关/race/fullGo，不改前端，原真实失败保留；完成后先固定代码再比较原正/负例，并准备未用于调试的新隔离样例。

# 复核写入版本迁移

读取GET /api/v1/runs/:id返回reviews中的revision。PUT /api/v1/runs/:id/findings/:finding_id/review现在必须提交expected_revision。无复核记录传0，已有记录传读取到的revision；成功204后重新读取，不将旧条件反复重发。

```json
{"status":"accepted","reason":"检查到保护条件","expected_revision":1}
```

缺失或负数返回400/review_revision_required；陈旧条件返回409/review_conflict。收到409要保留草稿、刷新最新决定，由用户明确处理后重新提交，不能自动重试覆盖。状态允许pending/accepted/false_positive/fixed。Store.SaveReview及SaveReviewUser同样要求条件。

SQLite自动增列revision NOT NULL DEFAULT 1，原决定、历史和评论状态保留。新版前端配套提交条件。旧自定义调用脚本需要迁移；旧读取接口继续可用。降级旧服务会恢复无条件写入，因此多人复核期间不得随意降级；部署/回滚需另行执行。本轮未部署或合并。

## 轻量详情版本

新增GET /api/v1/runs/:id/status返回detail_version、status、queue_wait；完整详情也新增detail_version。比较版本相同可以跳过完整读取，版本变化时重读详情。版本是可见状态失效标识，不是源码哈希或安全性证明。每次请求仍必须授权，响应no-store。旧调用方仍可直接读取完整详情。SQLite增量创建项目版本表及触发器，部署前按现有备份流程准备；回退旧服务仍能读取原数据，但失去新版轻量协议，前端遇到旧接口错误会回退完整读取。


SARIF 导出新增 GET /runs/:id/sarif（viewer 权限、application/sarif+json、附件响应）。固定提交使用 aimangebot:// 快照 URI 基址，不承诺直接上传 GitHub Code Scanning；没有新数据库迁移。

重复读取保护令策略版本升为 eino-audit-contract-v18；旧任务继续保存其原策略，依既有策略一致性检查不按新契约自动重试。没有数据库字段变化，新的审计使用 v18。

PR 风险链扩展可选字段，旧 JSON 可继续读取；新的字段包含来源观察链接与逐边静态确定性，不表示运行复现。审计策略升为 eino-audit-contract-v19，仍沿用既有策略一致性/重试检查，不新增数据库迁移。

来源修复提示与逐项独立复核约束令策略升为 eino-audit-contract-v20；来源门槛、工具字段与存储模型不变，旧策略任务依既有版本一致性规则处理。

关联仓库独立复核覆盖门槛令策略升为 eino-audit-contract-v21。独立状态可能变为inconclusive，既有UI/API已支持；服务器可在模型最多8条限制后增加一条来源覆盖说明。主发现保留，不以缺来源自动判误报或安全。无数据库迁移。

跨组交接令审计策略升为 eino-audit-contract-v22，增加内部AgentConfig.PriorGroupNotes，不新增用户配置或API字段。旧组观察是导航提示，必须在后续组重新读取；授权失败不传既有事实，预算超限保留覆盖限制。普通单组请求内容兼容。

预期契约对比提示v23、来源/大小/锚点错误修复指引v24；不新增API/DB字段，不提高预算或降低校验门槛。旧版本失败证据保留，新审计用v24。

### v25 全文复核范围

FindingVerification新增可选claim_coverage（full/partial/unknown），无数据库迁移。历史已保存记录保持原判定，不追溯改写；新复核缺失字段按unknown，supported且非full降inconclusive，原因和原limitations保留。客户端原有inconclusive展示、覆盖说明与SARIFverification属性继续适用。字段只表示独立模型范围声明，不能当作运行时证明，也不能保证模型没有遗漏事实。

### v26 上下文预检

有配置关联源时，主模型首次请求前多一次list_repositories授权预检，计入原工具预算并保存trace/checkpoint；无关联源不增加调用。预检失败/取消/进度持久化失败停止请求，不降级为无关联源审计。来源列表明确非源码，不能支持调查或finding。主调查未读取的固定源会有服务器覆盖说明，后续独立复核不抹去该主阶段缺口。现有列表工具新增聚合后授权复查，不改变单次源码工具前后授权。无API/DB迁移。

### v27 风险链记录缺口

新增服务器coverage_notes，检查有效finding合并后的PRContext双方来源和逐边关系；推测关系即使模型待核实列表为空仍提示。元数据发现不强求运行路径。模型supported/full不抹去服务器记录缺口；这不是语义证明或自动补全事实。无API/数据库迁移，旧存储结果不重写，已有覆盖缺口UI/SARIFwarnings继续适用。

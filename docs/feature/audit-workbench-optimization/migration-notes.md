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

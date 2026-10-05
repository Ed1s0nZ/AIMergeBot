# 复核写入版本迁移

读取GET /api/v1/runs/:id返回reviews中的revision。PUT /api/v1/runs/:id/findings/:finding_id/review现在必须提交expected_revision。无复核记录传0，已有记录传读取到的revision；成功204后重新读取，不将旧条件反复重发。

```json
{"status":"accepted","reason":"检查到保护条件","expected_revision":1}
```

缺失或负数返回400/review_revision_required；陈旧条件返回409/review_conflict。收到409要保留草稿、刷新最新决定，由用户明确处理后重新提交，不能自动重试覆盖。状态允许pending/accepted/false_positive/fixed。Store.SaveReview及SaveReviewUser同样要求条件。

SQLite自动增列revision NOT NULL DEFAULT 1，原决定、历史和评论状态保留。新版前端配套提交条件。旧自定义调用脚本需要迁移；旧读取接口继续可用。降级旧服务会恢复无条件写入，因此多人复核期间不得随意降级；部署/回滚需另行执行。本轮未部署或合并。

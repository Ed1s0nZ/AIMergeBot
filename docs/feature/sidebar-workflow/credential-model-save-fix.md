# 纯模型配置保存误暂停仓库读取：修正计划

2026-10-15，codex/sidebar-workflow，基线c1513d513e8124abad67aca773e31849558592aa。用户要求最佳实践优化、没想清楚先不实现；本次修正既有S3合同的实现偏差，不新增功能或启用生产绑定。完整REQ/AC、原生GitHub、启动/发布/恢复入口目标保持。

## F2事实与合同

现有activation.save对material未变化的保存，也在文件落盘后无条件设置pending，再校验DB并发布内存。其代次虽然不变，但运行ctx看见pending会拒绝下一HTTP/模型操作，可能误停正在进行的审计。先前模型/outbox测试只在Save返回后验证descriptor有效，没有覆盖commit→publish窗口；不能据此前测试宣称整个保存期间都不中断。

新增确定性反例TestCredentialModelOnlySaveKeepsReadersActiveBeforePublish：实际SQLite/Settings先完成初次激活，单独修改模型，在beforePublish钩子暂停；DBgeneration不变、旧模型内存尚未发布，但旧ctx被拒绝且下一真实GitLab SDK/httptest读取也被拦截。修复前实际运行exit1（/tmp/aimangebot-model-only-before.log），两个明确失败断言；无真实外部请求。

正确合同：material相同时保留旧active cfg/generation/epoch直到新完整配置一次发布，运行继续使用原冻结配置；material变化才在文件写前进入pending。持久化结果未知、DB失败、提交后发布失败依然必须pending并阻止后续保存，不能把此次修复变成失败放行。当前ACL/lease检查不变。

## Workflow / Maintainability Gate 与F3

P10窄修正，既有F2/S3确认需求不变。repository_credential_activation.go约200行，已分离持久化和DB状态；allowed narrow_fix，无需大重构。只移除未变凭据正常路径的短暂pending，并对activateAndPublish返回错误统一设置pending；不改model/git token指纹、DBcounter、epoch规则或startup绑定。

```mermaid
flowchart LR
 A[已校验有效cfg与旧state] --> B{material改变?}
 B -->|是| C[pending后持久化]
 B -->|否| D[保持旧active后持久化]
 C --> E[同Tx状态验证/必要取消]
 D --> E
 E -->|成功| F[一次发布新完整cfg]
 E -->|失败或不明| G[pending 保留文件与旧内存]
```

验证：保留上述已证明会失败的回归；补充纯模型保存的替换前失败、替换后失败、commit后发布失败，分别确认旧有效性/不覆盖文件/不半发布/代次不变。再跑activation/实际Runner/descriptor专项及race、完整Go、vet/build/typecheck。测试只用合成配置、临时库和httptest；旧CI终态后才推新生产提交。修正没有新增用户审批或不确定语义，按原已批准消费者合同实施。

## F4/F5修正与验证

未变material的正常保存保持旧active view，继续按原冻结配置读取；activateAndPublish成功后一次切换完整cfg/view。该函数返回任何错误时再设置pending；已改变material仍在写文件前pending，替换结果不明仍立即pending。没有删除DB事务核验或放宽失效descriptor检查。

同一回归修正后专项exit0（platform2.280s，/tmp/aimangebot-model-only-after.log）；补充纯模型替换前明确失败仍可读、替换后失败/commit后发布失败保持pending、generation不变、内存不半更新、后续Save不覆盖落盘文件。修复前失败日志与原断言保留，未改断言让实现通过。

Go1.27.2：完整go test ./... exit0（platform103.222s，其余cached/无测试，/tmp/aimangebot-model-only-full.log）；activation/入队/epoch/实际Runner/HTTP及模型专项race exit0（3.705s，/tmp/aimangebot-model-only-race.log）；go vet ./...、go build ./...、frontend typecheck和diff --check均exit0。没有前端源/资产变更，无真实外部服务或模型调用。

当前生产启动尚未绑定激活器，本修正针对候选执行链缺陷，不声称发生过线上事故或完整S3已可用。启动/固定scope发布、显式reload/界面恢复、原生多平台factory及原确认其余REQ继续推进；没有main合并或部署。

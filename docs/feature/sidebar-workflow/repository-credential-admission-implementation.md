# S3c2：主运行与事务凭据检查实施记录

2026-10-15，codex/sidebar-workflow；F4/F5；上游repository-credential-admission-plan.md及technical-review-2026-10-15-credential-admission.md的限定Pass，私有服务绑定补充@6e2dc89已推后实施。完整REQ/AC、UAR-001/002/004、S3c3/S3d/S4–S6不缩减。

## Workflow / Maintainability Gate

P7/P8，adapter_extraction/narrow_fix。runner.execute新增捕获ctx/config及finish/retry委托，私有descriptor/事务检查约77行、最终失败收尾约36行拆模块；Store只多独立atomic绑定，不混用quotaSettings作安全开关。没有新增HTTP/YAML/JSON字段、数据库版本或恢复操作。main/Runner.Start仍不绑定激活器。

## 生产调用链改动

- Settings operation捕获的descriptor同时放入包私有ctx集合，继承父集合并按activation/epoch/generation去重；background checkpoint/finish/retry只复制描述，不复制会重入Store.DB的网络回调。
- descriptor校验可使用caller queryRower；同Tx检查generation、keyed material及当前内存phase/epoch，并验证属于同一Store当前activation。独立Store即使相同凭据/代次也不授予当前库权限；配额解绑不会移除保护。
- Submit、execute、context source、poll、Scope/SubmitFollowup贯穿同次操作ctx；execute还组合原有OwnedRun/身份/当前ACL检查。内层捕获新配置不能替换外层旧凭据。followup在外层捕获后一直保留到Enqueue，metadata结果不能逃出描述生命周期。
- Enqueue（含signed bot的共享Tx）、Claim、poll seen、Checkpoint、Finish、retry使用原事务验证。网络fence只在事务外使用，事务内用同Tx读取私有state，不因单连接重入死锁。既有同Tx权限/lease/幂等条件保留。
- 检查失败不会把新空结果/陈旧内存结果覆盖到旧checkpoint；执行收尾沿用FailWorker只改状态/固定错误且保留result/trace，仍受worker CAS保护。凭据暂停的过期孤儿在Recover中停止且不创建child，不把此状态误当整个worker租约维护失败。

## 反例与实际执行证据

1. 真实单连接BEGIN IMMEDIATE内enqueueTx成功，opaque网络回调计数0；相同descriptor去重，纯模型变更不使其失效。随后token旋转，旧metadata的直接Tx入队拒绝，run/event/seen无新增；background ctx只带descriptor同样拒绝。
2. 两个Store使用相同配置/generation=1，A的descriptor不能授权B；已有active绑定不能被另一Settings对象替换，拒绝不改原generation/有效性。
3. 真实文件替换后注入持久性失败进入pending；BindQuotaSettings(nil)不解除保护，Enqueue/Claim/Checkpoint/Finish/自动retry均拒绝。pending任务started_at仍空，原running结果保留、无child。Recover仅停止无租约孤儿并保留checkpoint，返回成功；此测试不是完整多实例租约演练。
4. 文件替换前失败恢复旧凭据但epoch已变，旧操作finish被拒绝，状态failed且LATEST-CHECKPOINT保留，未写STALE-RESULT。
5. 实际Runner.execute+真实GitLab/模型SDK+httptest：有效控制完成固定diff并实际到达模型；阻塞diff响应期间通过Settings.Save旋转凭据，仓库请求仅原来1次、模型请求0、run保持cancelled且RETAINED-CHECKPOINT不变。测试使用合法无owner内部原语，不是Runner.Start singleton绑定/整个服务部署验收。

所有凭据/模型均为合成值，未调用外部仓库、模型或真实通知。没有删除原测试断言迎合guard。

## 验证

Go1.27.2、macOS。
- 初始既有scope/activation/followup/context/binding/retry/checkpoint专项exit0，7.209s；独立入队专项2.604s；最终新入队/epoch/实际Runner专项2.576s，/tmp/aimangebot-s3c2-targeted.log。
- 完整go test ./... exit0，platform108.522s，其余通过/无测试，/tmp/aimangebot-s3c2-full.log。
- 专项race exit0，24.404s，/tmp/aimangebot-s3c2-race.log，覆盖新入队/Runner和既有activation、HTTP、followup、context、binding、retry/checkpoint/polling。
- go vet ./...、go build ./...、frontend typecheck、diff --check均exit0。前端无生产源变更，不重建dist。新精确HEAD CI在提交推送后单独读取，旧CI不代表当前代码成功。

## 交付边界

当前是C2内部接入；C3还需owner受控启动、固定scope的check/comment发布与有限错误传播，S3d需显式reload和已有配置页反馈。v50/nil scope没有凭据代次，不能用时间戳或补当前generation授权发布；启用仍依赖S4正式producer与恢复链路就绪。原生多provider Claim公平性未以本单项目测试证明，保持S4具体交接/验证义务。没有main合并、部署或关闭完整需求。

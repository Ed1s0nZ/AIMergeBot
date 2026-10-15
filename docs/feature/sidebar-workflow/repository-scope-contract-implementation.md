# S1：固定仓库身份契约与旧路径保护

日期2026-10-15；分支codex/sidebar-workflow；F3 9c1ee2f已同步后实施。输入为已确认完整需求、repository-recovery-contract.md及第三轮F2 Pass；范围是repository-recovery-implementation-plan.md的S1，不缩减S2–S6或其它完整REQ。

## F4 实现

- AuditPolicy增加可选repository_scope；旧nil序列化不增加字段，不回填历史任务。schema仍1、PolicyVersion仍v50，没有正式producer发出新scope，也没有开放任何native远端执行。
- repository_scope.go提供固定target/source/context/observation与无秘密credential版本引用，校验内部/远端身份、来源平台边界、同仓一致性、每平台SHA、context上限/排序/映射及提交用途。不同内部项目不能持有同一个远端identity。producer规范化和consumer验证分离，scope clone只复制值和独立context slice。
- repository_scope_json.go拒绝重复/未知/缺失/null/错误类型/过深/超过16KiB的新scope；精确key避免JSON大小写别名覆盖；代次十进制string保留超过2^53的精度。AuditPolicy根重复key、scope大小写别名及显式null不能隐藏新scope；旧合法optional字段保留。错误只返回固定ErrRepositoryScope，无payload/秘密。
- enqueue和readRun验证scope与Snapshot；snapshotForRun装载base/head/mr/diffVersion，并经同queryRower核对scope、旧ContextRepositories和持久context索引。caller原读事务保持，最多4项有界索引检查，不借校验授予ACL。
- 统一legacy snapshot guard拒绝任何新scope，即使没有binding行；durable retry guard通过JSON字段存在性（含大小写别名、显式null）及坏JSON拒绝旧路径。followup复制新的scope，避免子任务修改父scope。

未新增HTTP操作、DB迁移、token读取或外部写入。默认legacy路径继续使用原授权与绑定保护。此切片给完整factory提供输入和拒绝边界，不能当作用户已可用的GitHub审计或恢复功能。

## F5 本地验证

新纯合同测试覆盖same-repo、fork及混合GitLab context；缺字段/重复key/错误类型/未知version/显式null、大小写别名、过大/过深/坏UTF8、凭据互斥、SHA和snapshot矛盾、generation精度/上界/非规范值、父子scope复制、身份变化digest和producer规范化不修改来源。

真实临时SQLite/Runner验证：没有binding的新scope入队拒绝，run/event计数不变；模拟存储未来快照后实际worker失败且repository/model计数0；合法新scope停止自动retry且不生成child；非法/显式null/别名policy在解析阶段失败并保留原attempt，不伪造恢复成功；实际check dispatcher得到repository_unavailable，受控HTTP请求数0；context索引缺失、错误SHA、多余项均拒绝。没有真实外部通知、工单、平台发布或模型请求。

初次size夹具把空白加在整个JSON外，encoding/json在调用UnmarshalJSON前移除了外围空白；改成scope对象内的大空白证明16KiB对象上限。初次坏policy retry夹具错误期待写入failed回执；实际预算读取先拒绝非法policy并回滚。修正为断言原状态保持、零child，另直接验证durable guard拒绝该证据；未放宽生产恢复权限。后续专项测试通过（platform3.860s）。

当前已完成：`go test -race ./internal/platform -run 'TestRepositoryScope|TestRepositoryBindingGuard|TestRepositoryPublicationGuard|TestFollowup|TestContextRetry|TestPreviousReleasePolicy' -count=1` exit0，platform79.144s；`go vet ./...`、`go build ./...`和frontend `npm run typecheck` exit0。完整`go test ./...`现已exit0，platform160.553s，其余包通过或无测试。命令均使用Xcode开发工具PATH，Go测试GIN_MODE=release；不重建无源变更的web/dist。

## 待完成与交付边界

全Go终态已确认，可提交生产变更后按固定HEAD复核及独立精确HEAD CI。新scope字段目前只用于防止旧消费者误执行，完整provider factory、schema2/永久身份登记、私有凭据激活、恢复回执/取消和现有页面闭环仍未完成。UAR-001/002/004保持open；没有main合并或发布。后续审查如暴露问题，修复后重新验证，不用本记录替代代码审查。

精确生产提交949c13aee2055820130d80ced2e1934702c23c9e已推。CI38740692571现已completed/failure：Go tests and race checks（包含vet）及Operations tooling tests均success；Reachable Go dependency vulnerabilities在x/net0.57检出5个可达项失败，frontend/embed步骤skipped。不是S1测试失败，也不是CI整体通过。根因与最小版本修复/后续验证见ci-http2-security-update.md，不重跑旧脆弱依赖冒充修复，不用文档HEAD替代精确生产验收。

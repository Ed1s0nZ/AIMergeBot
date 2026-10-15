# S3c2主运行与事务边界审查

2026-10-15；codex/sidebar-workflow；reviewed head 270d7bacb95216c58e4526495bbc37105f791147，repository-credential-admission-plan.md；Confirmed requirements与S3合同保持。Codex同一作者分离复核，未使用独立代理。

## Decision

Pass（仅S3c2实施交接）；不批准S3c3启动/发布或整体启用。S3c1精确CI仍运行，本地可继续，下一生产HEAD推送等待其终态。后续原生producer、恢复UI、全入口验收仍须完成。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| 问题/目标 | Pass | 元数据后变凭据不入队、执行不借新token、历史checkpoint保留 |
| 对象与字段 | Pass | 包私有descriptor集合，继承/去重、同Store校验、不持久化假generation |
| 数据流 | Pass | operation ctx/config→网络→同Tx验证→原ACL/lease与写入；background仅复制descriptor |
| 生命周期 | Pass | pending、generation/epoch变化、重试拒绝、失效收尾沿用FailWorker且不覆盖结果 |
| 图 | Pass | 捕获/读取/BEGIN/校验/commit或rollback图 |
| 交接 | Pass | runner、所有读取入口、队列/seen/checkpoint/finish/retry模块与明确反例 |

## Qualified Areas / Residual Risks

事务不能执行完整ctx网络fence回调（会反向调用Store.DB），仅用同queryRower验证私有descriptor，再由原事务ACL/lease保障权限。跨Store同counter/material不可授权当前DB，必须指针归属匹配。操作必须从外层捕获并保留到最终入队，不能把desc限定在返回Changes的内部函数。

SQL串行化代次与取消保证写入在激活事务之前或之后；pending的原子内存观察不是撤销已经发出的请求/已经开始的事务。后续消费再次检查，不宣称瞬时撤回。失效后的终态收尾不得回写新result/trace或创建重试；旧状态已取消或租约变化则原CAS拒绝。

原生多provider公平Claim、固定scope producer、旧nil publication与owner启动不在本片批准范围，已列明S3c3/S4依赖；不能把Claim单项目反例当混合队列验收。未绑定服务行为保持，main不新增绑定。

## Validation / Re-review

必须真实单连接SQLite证明同Tx无死锁、旧metadata ctx零run/event/seen、pending零claim/checkpoint/finish/child、结果保留；实际Runner阻塞diff后变凭据模型0请求。完整Go/race/vet/build/typecheck及新精确HEAD CI另验收。改变公开字段、发布权限、scope/version或启动owner时重新审查对应合同；本Pass不是实现或部署验收。

## 私有服务绑定补充复核

C2实施前进一步对账确认：配额配置指针不是安全所有者。新增Store独立atomic activation绑定（nil仅未启用，私有启动所有，active不得替换）及descriptor当前指针核对，使BindQuotaSettings(nil)不能解除保护。该补充未改变generation/schema/public API或提前启用startup；当前同Store指针用于进程内一致性，不替代C3事务内owner/lease。限定Pass保持，须新增配额解绑/跨Store同值/旧绑定拒绝的实际反例。

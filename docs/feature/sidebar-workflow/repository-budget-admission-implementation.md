# N1c：准入账本预留与结算

2026-10-16，codex/sidebar-workflow；P7/P8/F4，输入为S4第7轮Pass及预算合同。N1b已有schema3/严格limits；本片实现真实SQLite的user准入预算、续租/结束及request/bytes/upload/text预留结算。worker Attach/retry、service principal和transport仍待后续，完整需求不缩减。

## Workflow / Maintainability Gate

允许内部Store操作，不增加HTTP/设置/模型工具入口，不启用C3/501写。预算只管理数量，不是仓库访问令牌；factory/transport仍需固定scope/全仓权限/凭据/worker检查。先支持用户audit/scope_preview/codeowners/diagnostic；poll/publication服务主体必须等待真实receipt/outbox授权，不用actor0占位。

新admission/reservation文件分别负责生命周期与事务计量，避免向Store290行或Runner追加SQL。风险medium/high：计量/所有权/事务跨界，先独立实现并以真实SQLite重开、双连接并发、过期/撤权/失败回滚反例验收。操作handle和receipt私有且脱敏；新预留key由随机nonce及budget/epoch/kind/requested的摘要派生，绑定原始操作参数，**不是签名或授权凭证**。同key只返回原receipt元数据，不能用来启动第二次物理读取；transport后续必须单消费。

## 验收及剩余边界

- root创建/续租复查目标启用、真实账号与当前角色；所有操作复查owner/epoch/limits/principal原值/绝对deadline和传入context fence，事务内只校验已捕获descriptor，禁止回调Store.DB死锁。
- 请求派发前计数不退款，独立并发slot；byte预留同时扣分类和总下载，按完整canonical grant，不能任意缩成1字节；upload按有界完整body，text按整个cells预扣且不退款。
- 相同key创建不重复扣减；相同used结算只退一次，不同used/旧owner/过期/未知结果零退款。结束或过期不把已预扣当0；无通用客户端恢复handle。
- 临时DB、双连接16并发、重开/未知预留、8192行、条件更新/插入失败回滚、权限/凭据fence、有限错误及handle脱敏；之后全Go/race/vet与精确CI。

## 实际实现与验证

N1c用户准入基础已完成：admission与reservation两个独立模块，294/312行；测试独立510行。只有服务端私有入口，未增加生产调用或HTTP/模型入口。普通actor0、未授权用户及尚无真实授权证明的poll/publication主体拒绝，后续需专用service principal，不降级成假管理员。

实际SQLite验证包括：两独立Store/连接16并发争最后一次请求仅1成功；同一byte receipt 16并发结算只退一次；相同操作key返回replayed元数据、不再扣减；变更key输入拒绝；request只释放slot、不退款，text按预扣全额结算。byte分类/总下载同时预扣/退款，最后7-byte额度仍可使用，用尽后不提供免费EOF探测。8192条上限、INSERT/settlement触发器失败回滚、有限错误/handle脱敏均通过。

补测曾证明原始fence可被Background上下文丢弃，修正后handle固定origin context，每次同时检查origin与调用ctx、合并原捕获descriptor，所有opaque callback仅在SQL事务外执行。真实captured credential与单连接Store验证无重入死锁；换token后旧操作拒绝。DB连接重开保留计量，不提供按budget ID恢复丢失capability的接口；owner/epoch/绝对截止时间变化或权限撤销均不得继续、退款或续期。清理允许撤权/过期后用原owner结束，但仅标unknown/终结、零退款。

最终源码在Go1.27.2/Darwin arm64、Xcode PATH下：

- `go test ./...` exit0，internal/platform 134.417s，其余包通过/无测试。
- `go test -race ./internal/platform -run '^TestRepositoryBudget' -count=1` exit0，22.145s，无race。
- `go vet ./...`、`git diff --check` exit0。

全部是临时SQLite/本机配置fixture，未发真实模型、托管平台或消息请求。上个代码c75f421的CI38861896365已查询completed/success；当前精确HEAD CI另行记录，旧成功不续用。

仍未完成：worker Attach/retry/Recover同根映射、service主体、请求体/响应body单消费meter、进程/完整factory及生产消费者。`replayed=true`永远不授权第二次HTTP/DP；N2的body计数必须在撤权擦除buffer之前记录底层实际n，不能把外层返回0当成未消耗。此片只证明持久事务数量及生命周期，不是已经实现网络计量、全链重启继承、原生审计或全部N1完成。

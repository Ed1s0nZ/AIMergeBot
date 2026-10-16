# N1d1：任务绑定、worker代次与同根重试映射

2026-10-16，codex/sidebar-workflow；P7/P8/F4，输入为S4第7轮Pass、budget/execution/legacy-transition合同。完整REQ/AC不缩减。此片将内部新任务、映射和root Attach保持同事务，提供worker会话代次与retry映射协议；生产Factory、通用Claim/Retry/Recover接线及worker实际扣减仍待N1d2/N4，不能宣称已支持自动闭环。

## Workflow / Maintainability Gate

允许私有Store协议及真实DB验证；不开放HTTP、C3或管理写，不发布结果。保持旧Enqueue对scope/budget的拒绝。新native版本单独使用v51；旧v50不改写。Private enqueue暂只有已证明user principal；service不得借actor0进入。legacy reference只有真实active material v2才能授权，现有v1不会伪装ready。

维护风险high：身份、版本、任务、worker、重试跨界；采用独立scope authorization、budget run lifecycle文件，原Run/Retry暂不加分支。当前Store354行、retry242行，允许adapter_extraction/narrow_fix；先内部协议测试，再下一片实际调度接线。身份验证仅本地DB/激活descriptor，无网络/模型/外发，捕获及opaque callback在SQL外。

## 核心不变量与验收

- Snapshot必须v2 scope+完整audit limits，和当前永久身份、绑定/集成revision、origin/远端ID、权限/启用相符；legacy需固定generation/config/profile v2。内部metadata producer仍负责证明远端观察，Store不伪称本地DB能证明commit存在。
- user准入root→新run/context/event→root active+mapping同事务；重复已有任务结束本次准入、保留消耗，不给旧任务加额。中途失败全回滚。
- worker需真实实例及run lease，固定policy/digest/映射/整条重试祖先，打开时递增epoch、旧未结算转unknown且不退款。没有仅凭budget ID重新创建的入口。
- retry child只继承父budget，snapshot/policy/digest/序号/根身份一致；失败回滚不得留下无映射child或父状态误完成。后续N1d2必须接实际FailAndRetry/Recover，不拿私有helper测试冒充自动恢复已完成。

## 本轮实施与复核

- 生命周期F4/F5，分支`codex/sidebar-workflow`，仍沿既有Confirmed需求和S4第7轮Pass。需求、权限、预算、版本和重试合同齐备，允许上述内部实施；公开恢复/UI、service principal和Factory仍缺验收，不能提前开放。不是缩减REQ/AC。
- 新任务使用内部v51 producer；run、context index、root Attach、mapping与event原子提交。重复任务结束自己的准入根并保留消耗，不给原任务补充额度。
- worker session要求当前实例和run租约；打开时递增epoch；原始上下文、固定策略、全部引用的当前身份/集成/绑定/权限、上下文索引和祖先映射持续复查。
- retry helper只接受failed parent→pending child，同一事务继承原根；拒绝改HEAD、取消父任务、异常child及失败的映射写入。调用方仍须接实际FailAndRetry/Recover，当前测试不声称自动重试集成已完成。
- 发现并修复重复任务路径仅验证“存在active映射”的缺口：反例将run映射到另一MR的预算根，修复前`TestRepositoryBudgetRunDuplicateRejectsForeignRoot`稳定失败；修复后核对原申请人、策略、root和整条祖先映射。completed duplicate必须对应closed ledger，closed根不能重新开启worker。
- 维护边界：scope授权95行、run生命周期290行、单独DB测试；未向Runner、旧v50或HTTP装配增加行为，没有另起大规模重构。

## 验收证据

本地Go1.27.2、SQLite真实fixture，不访问外部服务。目标命令`go test ./internal/platform -run '^TestRepositoryBudget(Run|Worker|Retry|Closed)' -count=1` exit0（最终链尾检查后3.116s）。

场景覆盖：准入已扣减→Attach不退款；重复准入不重置原根；mapping/event写入故障完整回滚且脱敏；worker换代、原始ctx取消、run/实例租约过期、数据库另一连接打开与epoch更新；集成revision/权限/项目停用；source/context撤权和索引缺失；retry同根、变造和回滚；closed duplicate仅可复用而不能执行。

最终源码验证：`go test ./...` exit0，platform 284.686s（期间进程保持运行，未重启；前一轮123.494s发生在最后链尾加固之前，不用该轮替代最终验证）；`go test -race ./internal/platform -run '^TestRepositoryBudget' -count=1` exit0，84.501s；`go vet ./...`和`git diff --check`均exit0。

复核测试质量时发现source撤权fixture原本用了不同provider，可能被scope结构校验提前拒绝；已改为合法同endpoint GitHub fork、独立remote ID，并在撤权前断言完整授权成功。此最后改动仅涉及测试，单独`go test -race ./internal/platform -run '^TestRepositoryBudgetRunChecksSourceAndContextIndex$' -count=1` exit0（13.508s），覆盖source撤权/context撤权/索引缺失，避免把无效输入当权限验证证据。全量测试进程编译在此测试fixture修正之前；最终生产源码一致。

数据库reopen测试保留同一进程内真实owner租约，不等同于进程重启恢复验收。尚无worker实际预留/transport扣减、通用Claim/Retry/Recover接线或原生端到端证明。平台集成测试没有访问真实模型、邮件、机器人或仓库服务。

## 兼容、回滚和下一步

无公开API/页面变化，不新增schema或对历史v50回填。回滚遵循schema3的一致二进制/数据库备份规则。下一步N1d2连接真实领取、完成/取消、失败重试和恢复，并验证坏native pending不会阻塞整个队列；之后worker实际计量与Factory消费者，不以孤立helper测试替代。

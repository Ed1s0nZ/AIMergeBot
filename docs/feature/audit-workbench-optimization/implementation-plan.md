# PR审计优化实施计划

输入：Confirmed R3；设计design.md；分支codex/audit-workbench-optimization。阶段F3，生产代码尚未修改。

| 切片 | 需求 | 文件/边界 | 验证与交付 |
| --- | --- | --- | --- |
| 1 上下文批量工具 | 001、002、012 | 新context_repository_batch.go及测试，agent_investigation.go，策略版本 | 固定SHA、编号行、未知ID、撤权、取消、范围及字节预算测试；回归上下文工具；文档新增工具；无DB迁移 |
| 2 PR调查与风险知识 | 003、004、006 | Agent提示、分组交接、独立知识模块 | 风险引入/修复及反证样例；预算与检查点回归；策略版本更新 |
| 3 PR因果链 | 005、006、012 | Finding/Investigation契约、验证、持久化及前端类型 | 逐边观察资格、BASE删除、跨仓库双方与未知边测试；历史JSON兼容 |
| 4 审查工作台 | 007、008 | Review事务/迁移、HTTP、独立React组件 | 并发首写、版本冲突、权限、草稿保存、键盘、加载错误与浏览器验证；记录旧调用方迁移 |
| 5 读取效率 | 009、012 | 状态读取HTTP、详情加载、Git清单缓存 | 无变化传输、变化恢复、撤权、提交隔离及有界资源测试；实测再报告效果 |
| 6 SARIF | 010 | 独立导出模块与授权路由 | 标准schema校验、BASE/HEAD来源、缺口、不确定边和访问控制 |
| 7 质量及完整验收 | 011、全部 | evaluation及验收报告、CHANGELOG | 新增语言和跨仓库PR对；独立真值；实际模型、成本、未完成及失败记录；逐REQ完成审计 |

每个切片开工前细化所需契约、字段及回滚；局部完成不标记目标完成。代码与实施说明按切片测试后提交推送；综合验收另有证据报告。用户README及docs/images不纳入本功能提交。

首切片检查：go test ./internal/platform -run Context；新增批量相关测试；完成后go test ./...。涉及UI再运行类型检查、构建与实际界面验证；涉及并发/事务运行race；不每次重复无关全量检查。

回滚：每切片可独立回退代码；迁移采用保留数据的增量方式，复核版本保护不能通过降级客户端静默绕过。保持旧报告可读；无自动合并、部署或发布。真实评测如缺外部条件明确报告缺口，不以unit test代替质量验证。

独立检视 AWO-REV-002（8e6bf31）：固定 context 读取擦除 ErrRepositoryUnavailable，违背既有执行器停止契约。局部修复仅保留安全 sentinel，不传播原错误文本；普通缺失文件保持可恢复。以真实 SDK 本地 HTTP 计数验证单组及分组首轮 context 失败，后续组停止及停止原因；不改变预算与语言范围。

### v36 计划状态收尾（P10 / F3）

依据真实恢复回归704/705：四项计划checked但仅record_hypothesis，状态仍investigating；源码reader与update门禁未失败。Confirmed R3 AC003/006/011，已有设计和调用契约允许此局部反馈迭代。Workflow Gate：允许，缺少状态反馈而非新产品决策；不涉及UI/API持久化迁移。Maintainability Gate：导航96行、职责单一；agent_investigation仅局部工具说明，保持现有校验，不广泛重构。

新增server-owned导航unresolved_ledger_count（纯计数、不插入模型claim/id），明确record永远创建investigating、必须显式update；supported/rejected相对实际假设而非是否报漏洞，兼容性结论不能靠标记rejected伪造反证。必要上下文未知保持investigating；四项checked不能自动收尾，不自动制造关系。工具说明同样明确创建状态；策略v36，历史不重写，预算不变。测试覆盖状态混合、隐私投影与实际SDK工具请求中的反馈；真实同集后续仅称回归。

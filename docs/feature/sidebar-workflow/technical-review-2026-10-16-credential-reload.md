# Technical Plan Review：显式凭据恢复后端

- 日期：2026-10-16；reviewer：Codex，同一作者单独复核，未使用独立代理。
- 输入：repository-credential-reload-plan.md、既有activation/runtime/admission实施记录与Settings、HTTP、状态表源码。
- 结论：Pass，仅授权S3d1后端和必要readonly字段传播；不授权main启用或宣称UAR-001完成。

## Workflow Gate Report / Feature Lifecycle

用户目标：最佳实践优化，未想清楚不实现。分支codex/sidebar-workflow，F3→F4/F5，P6→P7/P8，backend recovery。Confirmed requirements及repository-recovery-contract、S3计划、当前补充合同齐备；界面P2–P5仍留S3d2，本片无新视觉交互。Implementation allowed：yes，在下述边界。无外发、schema或发布变化。文档/代码各自提交并推送feature分支；main不合并。验收按计划真实SQLite/HTTP/故障与权限反例，不能以存在方法名代替生产验收。

## Maintainability Gate Report

Settings396行、routes499行、activation220行、frontend settings550行/api410行；Settings混合默认、IO与动态client，中等风险。允许narrow_fix/adapter_extraction：默认与严格文件解析单独模块，恢复协议/事务审计独立模块，HTTP独立适配。原模块仅共享helper/薄调用；不混入provider factory、启动所有权、UI设计或宽泛重构。测试沿既有Settings/activation/HTTP fixture。

## Mandatory Criteria

| 项 | 结论 | 证据 |
|---|---|---|
| 背景/目标 | Pass | 已落盘未发布的可恢复性，显式管理员操作 |
| 数据/新字段 | Pass | readonly status枚举，唯一精确revision string输入，有限错误 |
| 数据流 | Pass | file→规范化→保留有效Projects→版本→file→同Tx state/cancel/event→内存 |
| 生命周期 | Pass | 无新增持久核心对象；active/pending/unavailable投影及失败重试已定义 |
| 图 | Pass | sequenceDiagram包含file与DB边界 |
| 开发交接 | Pass | 模块、锁序、故障注入、前端传播、生成资源和验证范围明确 |

## Qualified Areas / Data Structure Review

只读repository_activation为可选对象，生产未绑定时省略。state/code/can_reload由服务器观察生成；客户端不得提交它。expected_config_revision唯一字符串正整数，拒绝混淆/重复/溢出，服务端拥有next revision。无schema迁移。复用平台events：settings.reload.committed只证明DB提交，包含actor和精确revision；不包含凭据或文件。

## Data Flow And Lifecycle / Flow Diagram Review

Settings.mu串行写入，Store事务不回调Settings；管理员预查及事务内复查。pending可以恢复，坏key不生成新key。保留当前Projects，空token遵从真实文件而非Save的保密占位规则。material未变保持已有active直到发布；改变先关闭旧epoch。file不确定替换后禁止伪回滚。状态与公开字段来自同一revision；跨观察不一致不给恢复许可。图满足后端交接，完整用户界面待S3d2。

## Disqualified Gaps And Risks

无阻止本片实施的未决项。残余边界：本片不闭合UAR-001；启动owner、正式scope及发布、UI恢复仍必需。不可直接调用bind来获得“已启用”的表象。

## Implementation And Verification Readiness

独立settings_file_decode、repository_credential_reload、http_repository_activation，activation只增加私有事务授权/事件hook；Settings.Public及HTTP共享单快照，前端只删除readonly payload字段。测试须覆盖准确大数、MAX、未发送/无写、role在replace后撤销、event失败回滚、commit后内存失败与同material恢复，以及严格JSON/YAML。全Go/race/vet/build/typecheck与前端生成资源后embed检查；外部服务本片不需要。所有失败返回固定脱敏信息。

## Recommendations / Re-review

按合同实施；若扩大到自动重试、provider或启动装配需新审查。生产发布前须S3d2实际交互与C3/S4完整调用验收。本Pass不继承为代码审查或合并批准。

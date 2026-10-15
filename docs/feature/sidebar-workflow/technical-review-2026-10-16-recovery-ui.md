# Technical Plan Review：设置页显式恢复

- 日期：2026-10-16；输入repository-credential-recovery-ui-plan.md及e7e0a77后端。
- Reviewer：Codex，同一作者独立复核阶段，未使用代理；Final decision：Pass。
- 仅针对S3d2交接准备，不是完整项目/分支批准或生产启用。

## Handoff Judgment / Mandatory Criteria

| 项目 | 结果 | 证据 |
|---|---|---|
| 用户问题与目标 | Pass | 已有后端缺少核对、草稿和异常恢复流程 |
| 数据与新字段 | Pass | 复用Settings与既有API，新增纯客户端内存状态/有限policy |
| 数据流 | Pass | GET→操作资格→单次写→200后只读GET；失败只允许手动核对 |
| 生命周期 | Pass | ref操作所有权、20秒timeout、卸载abort、草稿销毁和无持久化明确 |
| 图 | Pass | flowchart覆盖dirty、状态、未知结果与重读 |
| 交接/验收 | Pass | hook/policy/view边界，Node测试与浏览器次数/body、响应式证据 |

## Qualified Areas / Data Structure Review

现有API expected_config_revision严格string，客户端仅从正SafeInteger生成；无新schema、counter或可写readonly字段。只有缺失activation才用兼容路径，畸形/null不可回退。pending的can_reload只是UI提示，服务端仍独立检查权限/版本；unavailable不给操作。普通Save继续按后端现有secret占位语义，恢复明确读取文件真实值。

## Data Flow, Lifecycle And Diagram

每次操作同步占有ref以阻止重绘前重复提交，20秒后超时按未知处理；旧请求/卸载不发布状态。mutation成功后的GET只读取不重发，GET失败保持写锁；所有401/403清除内存草稿。草稿丢弃由明确按钮触发GET，失败保留草稿并锁定。图足以实施，无未定义持久生命周期。

## Maintainability / Implementation Readiness

允许adapter_extraction：原页面load/save与新恢复统一移至hook，policy纯函数、恢复展示使用既有样式组件；不在约550行页面继续叠加异步状态。scope只frontend/CI测试命令/生成资源与记录，后端调用不启用。P2/P3/P5/P6已有组件内补充，无新Figma资产依赖。

## Disqualified Gaps And Risks

本片无阻断设计缺口。残余风险：合成API浏览器证明只能覆盖UI行为，不能替代真实启动激活/仓库执行/外部服务验收。必须保留这一限定；不能用测试或按钮数量关闭UAR-001。

## Verification Readiness / Recommendations

Node policy测试须有矛盾与边界反例；浏览器须统计实际写请求次数，证明unknown后重绘/点击不会重发，重新GET后仍须再次明确点击。检查宽屏/移动布局、初次失败重试、保存期间禁用与权限清理。前一CI终态后推新代码，精确新HEAD验证；文档/CHANGELOG随代码提交。若改为自动POST或移除SafeInteger门禁需重新审查。

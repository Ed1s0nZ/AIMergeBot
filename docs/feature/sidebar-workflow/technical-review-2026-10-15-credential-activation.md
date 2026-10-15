# S3凭据激活实施计划审查

2026-10-15；codex/sidebar-workflow；输入repository-credential-activation-plan.md @37e1652d234fd6f67a73cfa876ae31ec6a85a7a0、已通过F2合同、Confirmed requirements、S2实现与Settings/Runner/model checkpoint现状。Codex，同一作者分离检查，未采用独立代理。

## Decision

Pass（实施计划交接），可从S3a开始按序实现；不批准跳过后续消费者、恢复界面、验证或合并部署。S2精确CI仍运行，只允许本地继续，下一生产提交推送等待该CI终态。不是S3运行能力验收。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| 背景与目标 | Pass | 凭据变更不允许旧运行继承，配置回退不复活旧generation；完整REQ/AC保留 |
| 对象/字段/校验 | Pass | DBstate、material、epoch、immutable descriptor、精确reload CAS及有限错误；无YAMLcounter |
| 数据流 | Pass | actual SDK BaseURL/有效proxy→私有HMAC→SQLite→单次完整发布；HTTP与模型派发/返回复查 |
| 生命周期 | Pass | pending、文件不明、DB失败、恢复/重启、ABA、溢出、取消与代次同事务；坏key不补 |
| 图 | Pass | Settings/gate/file/DB/runtime sequence及事务提交后发布约束 |
| 开发交接 | Pass | S3a–d文件职责、必需反例、真实次数断言、兼容/回退与公开消费者 |

## Qualified Areas

旧GitLab允许HTTP/path，不能套native HTTPS origin；使用实际SDK BaseURL避免错误合并不同path。旧transport没有CIDR设置，material只描述实际生效策略，不凭空宣称网络安全升级。reload以max(current,file)+1保持进程内全局CAS，恢复旧token仍旋转DBgeneration；文件Projects不变成权限输入。持久state和私有material不输出秘密。

## Data Structure and Lifecycle Review

schema2已有generation/key/fingerprint不变，不新建token副本。新增私有material v1、进程epoch与descriptor由服务构造；不替代ACL/lease/binding。公开reload expected_config_revision为正uint64十进制字符串，HTTP状态与secret redaction、未知/重复字段拒绝已有交接。启动前必须激活；S3a没有生产调用，不以其通过提前宣布active。

generation更新和旧legacy任务取消同事务；发布前pending维持；提交后才发布完整配置并解锁。读取后/模型发出前的窗口通过真实HTTP派发fence收紧；不能撤回已经发出的请求，返回结果仍需复查。失败保存可能中断旧进行中操作，计划已明确；不伪称无影响回滚。

## Gaps and Residual Risks

无阻塞计划交接的缺口；实现正确性未证明。有效代理与SDK URL来源必须保持相同，单连接Store不可在事务中回调Settings；S3c必须逐个验证所有直接client/model消费者，任何遗漏即不能关闭S3。reload会加载完整文件配置且推进Revision，需现有配置页明确手动操作，不能自动触发。旧HTTP/path无法进入当前native scope属S4明确边界，不能偷偷删除path。

## Verification Readiness

S3a需实际Go/SQLite覆盖ABA、双连接竞争、MAX、坏/缺state、精确material及秘密不外泄；S3b/c需文件/DB/发布故障、取消事务、真实阻塞HTTP与模型计数；S3d需真实HTTP解析/权限/CAS、前端失败反馈。完整Go/race/vet/typecheck/embed及精确生产HEAD CI必须另做，不能复用本Pass。未执行真实服务联调、外发或部署。

## Implementation Gate

product-workflow P7/P8，F4允许按S3a开始。Maintainability allowed narrow_fix/adapter_extraction：state与material拆新文件，Store/Settings仅薄委托；不得扩大到未完成S4网络factory或猜测自动修复。S3a独立phase记录与测试通过后提交，本地继续S3b前检查public contract/锁顺序。user原约束“没想清楚先不实现”保持，任何偏离本合同先修计划再复审。

## Re-review Triggers

改变material来源、generation/epoch生命周期、取消事务边界、文件失败策略、reload授权/字段或旧nil任务兼容时复审。完整S3依然待所有切片与失败链路证明；UAR-001/002/004仍open。

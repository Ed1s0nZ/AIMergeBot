# 仓库身份与恢复合同：第三轮设计复审

- 日期：2026-10-08；reviewed head 532db691b5f117cdb689c6e957045718d64b645b。
- 输入：repository-recovery-contract.md全部修订、credential-generation-evidence.md、前两轮review；Confirmed requirements.md不变。
- Final decision：**Pass（F2消费者合同与恢复设计）**。可进入F3分文件实施计划；不批准立即上线恢复/native执行，不批准整体分支合并。
- 同一reviewer分离第二次检查，非独立代理审查。前两份Fail报告保留历史；本报告在该快照取代F2设计裁决，不撤销尚未完成的实现/验证门禁。

## Handoff Judgment

可以按本合同制定实施计划。新的固定身份、凭据来源、恢复回执与取消/激活生命周期已具备字段、所有者、边界和反例；未实施范围明确，没有把实验当完整授权链路。正式恢复仍依赖完整provider factory、固定Git/API模式和全部消费者，不以通过设计审查声称UAR-001已关闭。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| 背景、目标与非目标 | Pass | UAR-001；未来运行恢复、历史身份保留、不自动授权/关闭发现 |
| 结构与新改字段 | Pass | 登记schema、scope/credential联合类型、回执独立列、私有legacy state，最新修订明确替代YAMLcounter |
| 数据流 | Pass | source ACL前零内容读取、二次CAS、恢复回执同事务与原请求重放、激活文件/DB/内存顺序 |
| 生命周期 | Pass | kind单向转换、revision高水位、unknown不复活、pending拒绝、重启/配置回退与代次耗尽 |
| 图 | Pass | 准入和恢复两个sequenceDiagram；激活状态表补充跨持久层行为 |
| 开发交接 | Pass | 模块责任、锁顺序、当前保护与兼容、新模块与必需反例明确；F3负责具体文件/实施顺序 |

## Findings Closure

| ID | F2状态 | 当前证据 / F3与F5必须保留的约束 |
| --- | --- | --- |
| RRC-001 | resolved | scope/旧ContextRepositories/索引三方对账；target/source与观察SHA用途分离；mixed legacy与integration；scope大小及重复key要求 |
| RRC-002 | resolved | 坏绑定有限诊断、etag、revision高水位、离线修复边界；有界readonly验证；回执和outbox同事务；sync_pending不伪装未提交 |
| RRC-003 | resolved | 不撤回已发请求；派发前/结果消费前复查，取消target/source/context，同进程cancel与跨连接轮询分开；断言下一请求和模型输入次数 |
| RRC-004 | resolved | schema2和policy双屏障、停止旧进程后单事务升级、配对备份回滚；不兼容旧进程已打开连接的滚动混用 |
| RRC-005 | resolved | digest含协议version/actor/route project/body；重放列/JSON/history完整对账；缺失/损坏回执不当首次请求；新CAS精确十进制字符串 |
| RRC-006 | resolved | 取消客户端/YAMLcounter；DB私有keyed fingerprint+单调generation；pending gate；文件成功/DB失败、DB成功/发布前中断及重启状态明确 |

这里resolved表示设计缺口已具体回应，**不表示对应生产缺陷或功能已修复**。UAR-001/002/004仍open。

## Forward-risk Review

- keyed fingerprint只用于私有凭据激活身份，不取代ACL、lease、provider身份、binding revision；HMAC不出HTTP/policy/模型/events，不复制token。损坏key/行拒绝，不静默生成新历史。
- scope SHA与内部project、origin、provider、remote ID联合验证；两平台同数字ID或公开fork不能通过新类型绕过授权。
- 回执重放可能返回历史配置，不能用其宣布当前可审计；前端须另查当前状态。用户操作入口需保留同请求恢复和有限不可恢复说明。
- schema升级未实现，不提前修改version行。旧Scope nil兼容保持独立；新增scope在factory接入前必须拒绝所有远端执行路径。
- JSON decoder拒绝scope重复/未知/缺字段，不改变旧policy未新增字段的默认兼容。新generation/CAS使用string表示，不能经float64往返。

## Data Flow / Implementation Readiness

按scope契约→持久身份/schema→凭据激活→provider factory/固定工具链→全入口执行/恢复→管理页/全验证推进。第一片只允许完成数据契约、拒绝未支持scope和实际消费者深拷贝，不对bound放行；这为后续factory提供统一输入，而不是作为独立用户功能交付。完整功能仍须一次证明create/恢复→授权→固定读取→审计结果→撤权/重试/发布链路。

## Verification Evidence And Limits

源码对账基于当前head的AuditPolicy、snapshotForRun、binding guard、retry复制、followup浅拷贝、Settings.save/worker取消机制。Store schema2拒绝探针和9个SQLitegeneration实验支持所选机制；没有生产scope或activation实现，当前没有新功能Go/浏览器/外部验收。该设计审查未重新跑全量测试，避免把不覆盖新合同的绿色测试当完成证据。

## Non-blocking Recommendations / Re-review Trigger

F3将旧计划的后续scope及工厂接入保持同一目标，不为容易测试而永久保留只配置模式。通过F2后不能跳过F3推送和maintainability gate。实现若改变类型、取消时点、回执幂等、私有key生命周期、schema升级或legacy兼容边界，重新审查受影响合同；全部反例应成为实际行为测试。实现完成后另做代码审查与精确HEAD CI，不能引用本Pass作为合并批准。

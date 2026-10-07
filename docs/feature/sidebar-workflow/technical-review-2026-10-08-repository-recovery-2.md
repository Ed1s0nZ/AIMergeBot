# 仓库恢复修订复审

- Reviewed head: 3fcc5071e4d278f606a338b9dbe01b3358af77c6；2026-10-08；codex/sidebar-workflow。
- Artifact: repository-recovery-contract.md（审查修订合同）；原审查 technical-review-2026-10-08-repository-recovery.md。
- Final decision: **Fail**；作者与审查者为同一代理第二轮分离检查，非独立审查。
- 原 RRC-001–004 的主要字段/流程/撤权时点/schema屏障已得到具体设计回应，但两个新合同缺口禁止进入F3/F4。不是运行时新发现，不把拟议功能称已可用。

## Handoff Judgment / Mandatory Criteria

| Criterion | Status | Evidence / required fix |
| --- | --- | --- |
| 目标与范围 | Pass | 原需求保留、恢复不重解释历史 |
| 字段/结构 | Fail | 凭据generation缺跨重启权威来源，RRC-006 |
| 数据流 | Fail | 回执digest未明确包含route project，RRC-005 |
| 生命周期 | Fail | 保存/重启/配置回退的generation检查不足，RRC-006 |
| 图与授权顺序 | Pass | 验证→CAS→回执/outbox流程已明确 |
| 开发交接 | Fail | 需关闭两项后再编写分文件F3 |

## Qualified Areas / 旧 finding 对账

- RRC-001：项目登记kind与历史scope区分；旧context policy/索引三方一致性、target提交用途、mixed credentials、规范大小和字段要求已补齐。新增专用generation的实现生命周期另见RRC-006，不能以此关闭整体结构门禁。
- RRC-002：坏配置有限诊断、etag、revision高水位、无法在线修复的边界、回执重放和sync_pending均已明确；新回执归属缺口见RRC-005。
- RRC-003：已发请求与后续结果消费区分，取消覆盖target/source/context，同/跨连接观察与模型输入隔离可验证；不再承诺撤回已发读取。
- RRC-004：schema2+独立policy版本、停旧程序后升级、配对备份/回滚已有明确操作序列；当前Store隔离探针证明拒绝事务不提交，完整旧进程与新迁移仍是F5证据，不能称已验收。

## Disqualified Gaps

### RRC-005 — Blocker：回执必须同时绑定路由项目和原请求

合同列出digest包含规范请求所有字段，但project_id在URL不在body；回执表以actor/request_id唯一。若digest只按body构造，同一管理员对项目A/B使用相同key/body，会命中A回执；目标B并未恢复，界面却可能收到成功。

要求：digest明确包括协议版本、已解析route project_id、actor（或actor在联合PK及校验中强绑定）、规范body。回执重放必须验证table.project_id、receipt.project_id、request_id、digest、repository/identity revision和JSON结构全部一致；缺失/损坏/跨字段矛盾不能返回成功或再次发请求。未收到响应的首次请求、同key跨route、篡改回执分别断言验证/写入次数。

反证核查：当前CreateRepositoryProject无route项目，它的请求body和回执已有身份核验，因此不是声称现有创建端点有该bug。风险来自新恢复请求的不同形状。

### RRC-006 — Blocker：专用凭据代次的所有者与跨重启核对未闭合

合同声明LegacyRepositoryRevision只在仓库凭据改变时递增，并拒绝回退，但没有说明重启后拿什么与配置文件比较；只读旧文件的counter无法判断它曾经更高。现有Settings.save只强制计算全局Revision，并保护Projects；新增专用字段若照普通Settings输入处理，管理员页面回传的旧值/直接修改值可能覆盖服务端代次。

要求：服务端计算专用代次，客户端输入不能指定/回退。明确缺失值首次初始化、配置落盘失败、配置成功但DB记账失败、进程中断、重启读旧文件、直接编辑YAML与备份恢复的状态机与来源；仅增加一个uint64字段不足以证明冻结凭据版本有效。区分旧项目同步、模型修改和仓库token/origin修改。用持久权威高水位或等效机制检测回退，跨DB/文件写入不能假称一个事务；不将秘密hash或token暴露到UI/事件。

## Data Flow / Lifecycle Review

恢复回执是持久历史证据，当前项目存在/账号admin仍要复查，但当前配置版本变化不能覆盖原回执。回执内部矛盾不是普通CAS失败，必须fail closed并保留证据。新generation同时参与scope admission和runtime读取，必须定义“已经写新配置、旧DB generation”的恢复，不允许先返回可审计再后台补齐权威状态。

## Verification Readiness

源码核查：repository_project_create.go对既有回执验证ProjectID/Binding/IntegrationRevision；settings.go的save服务端重算Revision，配置写文件后换内存。此次没有新的生产执行测试，旧Store schema2探针证据沿用修订文档明确范围；没有外部访问。文档检查不代替新增合同测试。

## Recommendations / Re-review Requirements

1. 明确route与receipt全部归属检查，保留历史回执，不在重放时再次验证远端。
2. 将专用generation视为冻结凭据的必要生命周期对象，补跨文件/DB写入与重启恢复；禁止照普通用户输入字段直接Save。
3. 关闭RRC-005/006并复核其影响，再做完整F3。schema升级/恢复API/native执行仍未获实施许可，完整G3–G6不缩减。

本报告只新增review文档；git diff --check需在提交前执行。无生产修改、main合并或远端review。

# 责任人通知路由与事件依据

对应已确认 REQ-010/011/019，继续完整 sidebar-workflow 范围。当前推荐只读、用户明确保存后才成为实际责任人。通知渠道仍由管理员配置，不能从仓库邮箱或用户名自动构造外部收件地址。

## Workflow / lifecycle gate

- 请求：最佳实践优化，继续责任人推荐的通知消费。
- 阶段：P8 / F2–F3；Confirmed requirements.md、design.md、owner-routing-design.md、现有发送器和权限测试已存在。
- 当前缺口：通知事件只含 run/kind/event_key；复核及风险到期事件没有独立 finding、HEAD 和责任人版本依据。event_key 是不透明去重标识，finding ID 可以含冒号，不能按分隔符猜测身份。
- 允许实施：是；先补齐不可变事件依据，再接入配置、路由、发送前复查和 UI。无真实外部发送、无合并或发布。
- 场景：责任人更换或撤权后，旧提醒不能根据最新负责人字段被静默转发。
- 公共影响：本基础阶段无 API/渠道行为变化；后续新增可选路由配置，旧渠道保持原项目范围。
- 文档与提交：本文件承载设计/计划；implementation-plan.md 承载实现与验证，逐阶段提交推送 codex/sidebar-workflow。
- CHANGELOG：最终收尾记录责任人路由及权限保护；不将基础表迁移描述成完整通知功能。

## Maintainability gate

- 已检查：notification_events.go（252 行，迁移/事件采集/汇总三职责）、finding_disposition.go（235 行，权限/存储/到期）、notification_access.go、notification_sender.go、integrations_store.go。
- 风险：medium，跨持久化、发送授权和集成配置；不向事件采集器堆积仓库网络读取。
- 改动类型：adapter_extraction；抽出事件迁移至独立 notification_event_migration.go，新 notification_event_findings.go 管理事件依据。原到期路径只委托一次写入。
- 先行重构：仅迁移职责抽出，保留函数与调用顺序；不做其他模块重排。
- 验证：事件原子性/迁移/重开/版本不变、review revision 旧库回归、通知与风险处理专项 race、完整 Go、vet/diff。
- 风险假设：存量事件缺少当时责任人证据，不能用当前状态回填。新增证据不得包含理由、代码、外部凭据或仓库身份映射。

## 数据契约与一致性

新增 platform_notification_event_findings，以 event_id 为主键并引用 platform_notification_events，保存 finding_id、disposition_revision、owner、head_sha。run_id 从事件表关联，不重复猜测。无 disposition 时 revision=0/owner=0，但仍保存复核 finding 和运行 HEAD；旧事件无依据保留原状。

复核 INSERT/UPDATE 在原通知触发器内同步插入依据，绑定该次复核事件。触发器升级在迁移事务中替换，失败整笔回滚，重开不会重建历史事件。风险到期先更新当前记录/追加历史，再在同一事务插入事件和依据；任一步失败整体回滚。已有 event_key 和通知去重不变。

路由使用事件记录的责任人及版本，不能用最新负责人替换历史身份。匹配依据和收到的 CODEOWNERS 推荐都不授予权限，不自动分配。未映射/来源不可用不产生未经确认的个人通知。未来自动推荐路由若启用，必须独立保存固定 HEAD、配置版本、来源证据及候选完整快照授权，不能在 collector 写事务执行仓库读取；该范围继续保留待交付。

## 后续路由契约

管理员可给通知渠道配置显式平台用户范围；空范围保持现有项目通知。配置仅作用已保存责任人的事件，渠道仍发送到管理员已保存的邮箱/机器人目的地，不能声称平台账号自动对应个人邮箱或私聊。账号须启用且具备选定项目权限，实际事件按完整 target/source/context 过滤。

collector 为进入 outbox 的每个事件保存来源事件依据；即时与日/周汇总均保留可复查的 event_id。发前验证配置版本、原发起者权限、记录责任人完整快照权限、当前 disposition 与捕获 HEAD/owner/revision 一致；失败取消并保留可解释状态。责任人更换不能将旧 outbox 重写成新目的地，unknown 沿用现有明确确认重试规则。

## 实施与验收

1. 独立事件依据表与迁移；复核/到期原子捕获；旧数据不猜测回填。专项测试不透明 finding ID、责任人变更后旧证据不变、触发失败整体回滚、数据库重开、旧 review migration。
2. 集成可选责任人范围与服务端权限/版本契约，登录 HTTP 正负例。
3. collector 路由与 durable delivery/event 关联；即时和汇总去重、重启与晚到事件。
4. 发送前完整权限/身份/版本复查与撤权/改派竞争验证。
5. 独立管理 UI、异常/冲突/只读/窄屏/键盘验收；完整回归、精确 CI 与逐项需求核验。

第一步不等于通知路由完成，全部范围验收前 REQ-019 / AC-017 保持未完成。

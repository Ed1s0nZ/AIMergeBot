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

## 已保存责任人渠道过滤 API 与实施 gate（第二阶段）

P8/F4，输入需求已确认，第一阶段39fedd7已推送。现有模块均低于800行，但integrations_store包含契约/配置验证/存储，notification_events包含采集/汇总；风险medium，允许adapter_extraction：新增notification_owner_routes.go处理配置权限与事件匹配、notification_owner_access.go处理发送前证据验证。原集成/采集/发送路径只委托，独立测试验证职责边界，无广泛重排。

GET/POST/PATCH integrations增加owner_ids数组，最多100个唯一正平台用户ID。创建时省略或空数组为原项目范围；更新省略保留原配置，显式[]清空，防旧客户端无意关闭路由。范围非空仅限email/feishu/dingtalk/wecom/slack/teams/webhook，事件须非空且仅finding.reviewed/risk.expired；run.completed/run.failed目前没有单一发现责任人依据，拒绝混合配置而非静默漏发。保存时所有选定用户启用且具备所有所选项目viewer权限，不授予ACL；实际collector再按完整运行快照过滤。范围保存在独立platform_integration_owner_routes，与集成revision、队列失效及审计事件同事务，避免改变旧表列迁移。

新增platform_notification_delivery_events(delivery_id,event_id)保留即时/汇总源事件。NotificationSummary内部sourceEventID不序列化到外部；禁止人工QueueNotification绕过非空owner_ids的依据检查，只有管理员主动渠道测试（run_id=0、test-前缀）可发送固定测试摘要。责任人汇总每份最多200个源事件，超出拆补充份；发送时有界读取全部事件并校验各run/项目与原发起者、owner、HEAD、disposition_revision及当前完整权限。当前版本/owner/HEAD变化即取消旧提醒，不改派旧正文。无证据旧队列不通过责任人路由。

验证需覆盖省略保留/显式清空、权限与输入拒绝、持久重开、原子写入失败、即时/日周汇总、晚到/去重、历史不明事件、变更owner/revision/HEAD、source/context撤权及停用、混入其他项目事件、无依据队列/人工绕过/明确测试。前端独立选择器与真实浏览器验收仍在后续，第二阶段API不代表UI或完整需求完成。

实施补强：delivery_event_counts保存预期源事件数量，与来源关联同事务累加且重复关联不增加；发前数量须一致，防部分关联丢失被剩余依据掩盖。明确渠道测试也先重查渠道revision/启用，再免除owner事件依据。

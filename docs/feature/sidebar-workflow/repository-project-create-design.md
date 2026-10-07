# 多平台项目创建与凭据初始化

Workflow Gate：REQ-012/019/021/023及Confirmed完整scope、github-provider-design/G2、github-project-identity-sync-design为输入；P6→P8/backend API，绑定/同步保护已推640f3f8。缺口为新安装没有项目时不能保存集成scope，以及新内部ID与binding/scope原子性。本设计补齐；implementation allowed=yes，先推设计/F3。完整邮件/bots、UI与G3–G6仍保留。F2，分支codex/sidebar-workflow，无main合并/发布。

目标用户路径：管理员先保存GitHub/GitLab凭据配置（scope可为空），再POST原生项目，服务端分配独立内部ID并把该项目加入所选集成scope；不创建任何用户ACL。所有scope/凭据选择由显式请求授权。不能创建先启用且无binding的临时legacy项目。

## Consumer contract

1. 原IntegrationInput允许project_ids=[]/nil仅kind github/gitlab；其他kind仍至少一个项目。为空表示“凭据已配置、未关联项目”，不赋予任何仓库读取权限，不能通知或触发审计。owner_ids仍沿用原限制；保存、脱敏、revision/CAS、凭据保留/清除/200容量和event语义保持，不声称远端连通性验证。当前前端仍要求项目，后续页面gate需一起接入空scope路径。
2. POST /api/v1/repository-projects，登录全局admin与同源guard，8KiB strict单一JSON值；字段request_id（UUID标准36字符）、name（1–200字符）、expected_integration_revision（必填正数小于MaxInt64）、provider/api_origin/remote_id/full_name/integration_id沿用binding校验。不接受内部id、binding revision、actor或凭据。
3. Store.CreateRepositoryProject短事务检查enabled admin，再读取durable actor/request_id receipt；同key同payload返回原创建receipt（replayed=true），不同payload409。不使用随机项目ID充当idempotency。receipt包含创建时Project、RepositoryBinding、IntegrationRevision，无token，返回的是创建记录而非实时项目配置；当前配置应另GET。新请求检查所选集成revision/kind/启用/非空token/canonical API origin；scope最多100，不移除已有项。
4. 内部ID取MAX(platform_projects.id)+1，空库从1，溢出409；SQLite写事务序列化并发。调用原project helper、集成保存helper（追加新scope、增加revision、保留原credentials及其他字段、原pending/sending清理/event语义）、binding保存helper（revision1/history/event/dirty），最后写receipt并commit。任何失败全部回滚。相同remote ID但不同provider/origin可以独立创建；重复远端身份409。后续同值legacy GitLab导入仍409，不按ID改provider。
5. 201新创建、200同请求重放。401/403原guard、400 malformed/字段校验、409 stale/同key不同payload/身份或scope不满足、404引用不存在、500固定内部错误。SQLitereceipt表主键(actor,request_id)，保存payload digest与有限脱敏receipt JSON；读取损坏/不一致fail closed。保存后有Settings则尝试配置同步；失败500/code project_config_sync_pending明确项目已创建，可用同request_id重试恢复相同receipt并尝试同步，绝不再建项目。身份guard仍repository_unavailable，factory完成后才实际审计。默认不发布checks、不直接merge。

Maintainability Gate：integrations_store.go约365行但credential/security/storage多责任high，repository_binding.go约200行多责任high。adapter_extraction，提取saveIntegrationTx及saveRepositoryBindingTx保持原Store wrapper/公开语义；新repository_project_create.go只编排短事务，http_repository_project_create.go独立解析并委托，http_auth.go仅两行装配。store.migrate只委托新receipt schema，不重构其他业务。网络/文件同步不在SQLite事务中。原Integration/Binding回归及ABORT失败注入证明提取不丢原权限/lease/CAS/事件。

此阶段仅backend创建协议；页面完整empty/loading/error/403/409/键盘/narrow proof另有gate。G3真实GitHub固定提交读取与factory随后实施，不能把配置存储当原生GitHub审计完成。

## F3 实施计划与证明

先提取两个现有事务helpers，原wrapper负责Begin/Commit；helpers保持validate/admin/CAS/凭据保留/notification取消/审计事件/绑定history及project_sync dirty同事务。仅两种repo integration放宽空项目scope。新Store创建编排及独立receipt迁移，HTTP独立strict decoder并注册admin POST。receipt仅表示创建历史，不按当前binding改写历史；重复请求可重试配置同步，meta不得包含凭据。

新增Store测试从零项目/空scope初始化、两provider同remoteID、已有scope保留、内部ID独立、同key重放/参数变化409、8并发同key只建一次及不同key相同integration版本仅一个成功、重复身份/未知或disabled/权限/endpoint/token/scope满/overflow拒绝、event ABORT整体回滚、实际SQLite重开receipt、无ACL/run。真实Register/Login HTTP初始无项目到创建、角色/Origin/8KiB/未知字段/尾随/缺版本、请求重放以及配置写失败可安全恢复，全部脱敏零真实远端/消息/模型调用。原Integration/Binding/Project/通知lease回归race，最后全Go/vet/build/diff。前端未改无需重构建。阶段说明/证据随代码commit，640f3f8精确CI37631183808终态后再push生产代码，保留其run；本阶段精确HEAD CI独立跟踪。回滚禁用新创建路由、保留identity/receipts/history/guard，不删除binding恢复legacy。

# G1 仓库绑定 Store 契约与实施 gate

P8/F3–F5，Confirmed原需求与cdae9f8身份设计已推送，允许实现内部存储；不开放HTTP/配置入口，也不改变现有Runner路由，避免Github配置尚未完整时进入GitLab。风险medium：新增独立repository_binding.go，Store.migrate仅委托；原store多职责不继续堆业务。所有已有表/ID/ACL/运行行为保持。

对象RepositoryBinding：revision、provider、api_origin、remote_id、full_name、integration_id；零revision表示未绑定（legacy），不回填猜测。SaveRepositoryBinding(ctx,internalProjectID,actor,expected,binding)仅当前启用admin，目标项目须存在/启用；expected非负且可递增。github/gitlab allowlist，远端ID与集成ID正值，HTTPS API根规范路径（github root或/api/v3；gitlab root），无userinfo/query/fragment/转义/控制字符。full_name github严格owner/repo、gitlab允许nested group但禁止空/./../转义/空白，最大255。所选集成须同provider、enabled、project_ids包含内部项目，token非空且endpoint规范后与api_origin一致。校验只读本地配置，不远端访问或确认真实账号资格。

独立current表具有(provider,api_origin,remote_id)唯一约束；同remoteID不同provider/origin合法，同身份不能映射两个内部项目。表/追加history/平台事件在同一SQLite事务，输入revision不可信以expected+1生成；保存不赋予项目权限、不改历史run/其他配置。保存冲突或后续ABORT整体回滚。查询须viewer，未知项目拒绝；当前行超过4KiB或损坏JSON/字段/版本不合法明确ErrConflict，不能降级legacy。凭据从集成读取校验但不存对象/历史/事件。

验证：身份格式/相同远端ID跨平台跨origin、重复身份、scope/kind/disabled/token/origin不符、admin/viewer/撤权；8并发仅一CAS成功；事件强制ABORT回滚；实际SQLite Close/Open与重复migration；legacy无绑定revision0且不新增ACL/改现有run。全Go/race/vet/build；更新implementation-plan证据。先commit/push本计划（docs-only不会取消cdae9f8 CI37623659140），生产推送等待该run终态。

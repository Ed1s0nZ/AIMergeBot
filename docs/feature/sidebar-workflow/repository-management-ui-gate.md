# 多平台仓库管理页面

Workflow Gate：Confirmed完整REQ及repository-project-create-design/绑定HTTP/identity-sync为输入；P6→P8/P9，API15c182d已推，本地完整验证通过，精确CI37633523912仍待终态。现有项目/集成布局和共享反馈组件可复用，无新设计系统/Figma依赖；缺少具体UI状态，本文件补齐。implementation allowed=yes（契约/F3先推）。F2，codex/sidebar-workflow，无main合并/发布；G3–G6及其余完整REQ不缩减。

API projection：GET /projects在当前用户一致权限/身份快照中追加repository?:RepositoryBinding与repository_execution_available?:boolean（当前bound false）。nil仍legacy GitLab；损坏binding整个请求固定错误、不标legacy。Store.ProjectsForUser实际读取enabled actor及role，项目列表与绑定同一短事务，拒绝stale caller role。当前Project原字段保持；Store.Projects内部不变。项目列表避免每卡发绑定HTTP，也不泄漏token。

UI场景：集成页GitHub/GitLab scope可空，前提仍项目数据成功加载；其他kind至少一启用项目，项目503/403/slow仍禁止保存。repo profile无需通知事件/频率操作（保留已有字段），说明空scope仅保存凭据、新项目创建会明确关联。启用repo且替换凭据时token必填。403/409及秘密清空沿用原机制。

Projects列表仅data ready时显示；刷新/loading/error不显示旧卡。legacy显示GitLab默认配置与远端ID；bound显示provider/full_name、remote ID与独立内部ID，当前审计未接入，不能用enabled冒充审计可用。member只看有权限项目，admin可打开binding editor；全局成员/owner/context工具保持。新增原生创建panel独立于旧GitLab默认配置添加入口，选择当前enabled repo profile（has_endpoint/has_secret、scope容量）及显式provider/origin/remote ID/full name/name，使用其revision。没有可用profile指向集成页；不自动保存凭据、授予ACL或调用远端。

创建状态：提交前sessionStorage按当前user ID保存不含凭据的完整payload与crypto.randomUUID，恢复时保持同参数/key。发出后冻结字段，unknown/network/500（含sync pending）只允许原请求重试；切页/重载保留attempt，busy不可重复submit。201/200后清attempt，刷新项目/集成并显示内部ID。409保留attempt/草稿锁定，显式核对项目并开始新请求才丢弃；403清编辑状态且锁定，新登录actor不能读取其他actor的attempt。storage失败禁止首次POST，损坏恢复记录不自动创建。关闭unknown panel不擦除attempt。新草稿只有列表data ready且当前选项仍可用时能提交，scope/配置变化须原CAS验证。

绑定状态：GET绑定与admin集成并行有界加载/abort，loading/error隐藏旧配置。expected revision严格来自GET；选择同provider/启用/包含当前项目的profile。403/404清配置并禁止写；409保留草稿锁定，仅明确丢弃并重读解锁。500同步pending表示已保存，GET重读新revision，不能按旧版本再写。表单保存前说明当前绑定执行暂不可用、历史任务不改解释；保存成功刷新list/provider。读写不发送真实消息或触发审计。close返回触发按钮focus，panel focus/label/fieldset/busy/role=status/error与窄屏无溢出。

Maintainability Gate：projects.tsx多责任medium/high，只组合独立repository-project-create.tsx、repository-binding-editor.tsx及共享repository-form.tsx/types；integrations.tsx压缩混合security/render/runtime high，只改scope/说明/token gate，不堆创建状态。page-utils useResource沿用；新增attempt helper负责有界会话存储与输入恢复，creation不混入Project主组件。ProjectForUser权限/IO high，独立身份投影委托或一致Tx保持小方法，不能先公开未授权metadata。adapter_extraction，先不广泛重构。

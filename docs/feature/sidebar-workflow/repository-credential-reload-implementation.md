# S3d1：显式配置恢复后端

2026-10-16，codex/sidebar-workflow；F4/F5；输入为repository-credential-reload-plan.md和technical-review-2026-10-16-credential-reload.md的限定Pass。完整REQ/AC不缩减，UAR-001/002/004仍未关闭。

## 行为与边界

新增admin POST /api/v1/settings/repository-activation/reload。唯一字段expected_config_revision为正uint64十进制字符串，按当前有效内存版本CAS；读取初始化绑定路径的已保存配置，不接受文件路径或凭据代次。新全局版本为max(内存,文件)+1；成功重复旧请求返回409且不再次写文件/事件。失败但尚未发布内存时，管理员可以同expected手动恢复；同material不重复推进凭据generation。

只接受≤1MiB、UTF8、单文档、唯一字符串key、无anchor/alias/merge、64层/100000节点且KnownFields的普通YAML；config_revision严格十进制integer，无/0按1。共享原启动默认值，但OpenSettings仍沿原有兼容解析。保留当前有效Projects，不导入文件项目或清除DB配置outbox；文件中的真实空token/API key/webhook token不会沿用旧密钥。序列化结果也不得超过恢复文件上限。

Settings.mu串行写入；管理员在文件操作前及激活事务内复查。健康state缺失/不合法不写文件、不重建key；已知版本耗尽也先拒绝。material改变先关闭旧epoch；未变且原active时保持旧读取至发布。文件可能替换后失败保留pending与旧内存，不回写旧文件。generation变更、旧任务取消及settings.reload.committed事件在同一事务；事件只证明数据库提交，目标含精确全局revision，不表示内存发布已经成功。

GET及保存/恢复响应在已绑定时附带readonly repository_activation；state/code/can_reload不含epoch/generation/HMAC/token/路径。公开字段与has_*、restart_required从同一Settings快照构造；view版本不匹配或观察变化不给恢复许可。未绑定时省略该对象、reload返回503。前端类型及普通PUT payload同步，剔除readonly对象；后端仍拒绝客户端提交该字段。新增有限错误的中文提示，没有恢复按钮或新增菜单。

## 模块与兼容

- settings_file_decode.go：原默认值提取及有界严格文件读取。
- repository_credential_reload.go：恢复、状态投影、私有事务授权/事件hook。
- http_repository_activation.go：严格body与有限错误映射；既有路由/Settings/activation仅薄调用。
- frontend api.ts/settings.tsx：必要readonly契约传播，资源随正常build更新。

无DB schema/PolicyVersion或依赖升级；原普通Save空密钥保留语义不变。未绑定的既有生产路径保留，main/Runner.Start不调用激活器。S3d2须在现有设置页交付草稿、手动恢复和未知响应处理；C3 owner、S4正式scope生产/发布和多provider执行仍未完成，不能把本片当作原生仓库接入完成。无main合并、发布、部署或外部消息。

## 验证记录

macOS，Go1.27.2，真实临时SQLite/配置文件、httptest与合成秘密，无真实模型/通知/工单。

- TestCredentialReload / TestSettingsReload专项：exit0，2.442s；后续增加的同事务取消与读取状态用例亦包含在全套验证。
- Settings/TrustedProxy/ModelOnlySave相关回归：exit0，2.534s。
- 定向race（Reload/Settings/Activation/ModelOnly/TrustedProxy）：exit0，15.379s。
- go vet ./...、npm run typecheck：exit0。
- go test ./...：exit0，platform 107.867s，其余包通过或无测试。
- go test -race ./internal/platform：exit0，364.624s；不是以定向race代替全平台检查。
- npm run build：exit0；正常生成index-BfDA8Ek5.js与index.html，CSS无变化。前端没有视觉/布局调整。
- go build -o /tmp/aimangebot-credential-reload .：exit0，在前端资源更新后验证最终embed。移除已无调用的私有publicSettings wrapper，由带请求context的单快照方法统一处理。
- git diff --check：exit0；生成JS只随前端源正常构建，不手工修改。

已覆盖：>2^53精确版本、旧文件版本/MAX、A→B→A/空token、文件Projects不导入/outbox不变；丢响应重放零写；文件替换前/后失败、DB事件失败、commit后发布失败；commit中途撤权；故障事务不取消旧任务，成功后才取消；同material恢复不重复generation；JSON重复/别名/未知字段/尾随/溢出/超限；YAML重复/未知/多文档/alias/非UTF8/depth/node/大小上限；状态版本不一致/坏state；真实HTTP权限、跨域、readonly拒绝、pending普通Save拒绝与显式恢复。

## 剩余验收

本片无视觉变化，不需要宣称浏览器恢复流程已验收；真正恢复界面留S3d2。没有远端服务联调、完整provider审计或生产撤权验收。旧审计报告固定旧HEAD，不因本片通过而变成全分支批准。

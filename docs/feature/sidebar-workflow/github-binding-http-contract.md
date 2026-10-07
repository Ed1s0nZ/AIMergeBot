# 仓库绑定 HTTP 契约

Workflow Gate Report：用户已确认完整sidebar scope，REQ-012/019/021/023及github-provider-design适用。P6→P8/backend API；Confirmed requirements、G1 Store、执行及发布guards存在，缺少具体HTTP契约，本文件补齐。允许实现：yes；只增加既有项目绑定读取/保存，不修改ACL或启动远端请求。页面、内部ID创建、同步碰撞与factory仍按完整G2–G6继续，不将这一接口当GitHub可用验收。

Lifecycle：codex/sidebar-workflow，F2 consumer contract。场景：管理员为既有内部项目显式配置provider身份，项目viewer读取脱敏配置。当前Store已支持，HTTP缺失。目标：登录会话、同源写保护、admin保存、Store CAS/权限二次校验，不接收凭据。release/changelog与整体F6一起整理；无main合并或发布。

- GET /api/v1/projects/:id/repository-binding：登录且该项目viewer；返回RepositoryBinding直接JSON，未绑定revision=0；未授权项目按现有规则404，管理员不存在项目404。无token字段；Cache-Control:no-store。
- PATCH 同路径：登录且全局admin，同源保护使用现有guard；JSON平铺expected_revision、provider、api_origin、remote_id、full_name、integration_id。expected_revision必填非负且小于MaxInt64；其他字段沿用G1验证。不接收revision/actor/token及未知字段，拒绝尾随JSON。8KiB body上限，错误固定消息不回显输入。
- 200返回服务端生成revision及规范化identity。400 malformed/未知字段/输入形状，409 stale/重复身份/绑定条件不满足，403非admin/跨源，401未登录，404项目或引用不存在，500内部失败。Store保持绑定/历史/event同事务；拒绝请求无修改、无网络调用、无ACL增加。
- 绑定保存不是远端连通性验证。当前显式绑定的审计及发布仍repository_unavailable；不能silent fallback到legacy GitLab。没有DELETE，避免旧任务被重新解释为legacy。

Maintainability Gate：http_auth.go约300行，session/security/route assembly有多责任，风险high；只新增两条路由委托，独立http_repository_binding.go负责有界strict JSON解析与Store调用，不重构原会话代码。Store不增加新逻辑；adapter_extraction，refactor required first=no。现有auth/owner-routing/guard测试可回归；新增真实Register/Login契约测试。

## F3 实施与验证计划

独立http_repository_binding.go：GET参数/Store/固定错误，PATCH 8KiB strict decoder+单一JSON值+expected_revision+G1 validate后调用Store；http_auth.go仅注册GET及admin PATCH。http_repository_binding_test.go真实Register/Login覆盖匿名/member/admin、same-origin/cross-origin/Sec-Fetch-Site、缺失ID/成员撤权、revision0与规范化保存/读回、stale及invalid/oversize/尾随/未知字段无历史变化、服务端actor/无凭据回显、保存后Submit明确503零HTTP。同时跑Binding/Publication/Auth/OwnerRouting相关race、全Go/vet/build及diff。无前端修改，不重复frontend build；仅受控httptest，无生产凭据、真实渠道或模型调用。前生产6b89c47精确CI37628377399终态后才推本生产代码，避免取消CI。文档更新记最终证据并提交推送。回滚禁用新增HTTP路由、保留Store历史和所有guard，不删除binding恢复legacy。后续页面仍需loading/error/403/409/narrow/keyboard真实证明。

## F4/F5 当前实现与证据

独立HTTP adapter及Register两条委托已实现，GET viewer脱敏读、PATCH全局admin与原同源guard；8KiB strict decoder拒绝unknown/server-owned字段、null/missing/overflow版本及尾随JSON，验证G1对象后调用原CAS Store。仅保存配置，无额外ACL和网络访问。

真实Register/Login HTTP专项19811 race3.613s通过；加入事务事件ABORT验证后扩展Binding/PublicationGuard/OwnerRoutingHTTP/SettingsHTTP/ProjectHTTP/Auth race26321 completed/success platform42.229s。覆盖匿名401、member保存403、跨源与Sec-Fetch-Site403、viewer无权限/撤权404、invalid ID400、revision0、规范化写读一致、stale及重复身份409、缺项目/集成404、错误origin409、内部写失败500固定脱敏并整体回滚、所有拒绝不写history/ACL/run、保存后Submit503且legacy reader零调用。最终完整Go10740 completed/success platform114.244s，其余包通过；全项目vet/build81826及diff检查通过。无前端改变或真实渠道/模型调用。

6b89c47精确CI37628377399仍in_progress。生产代码可本地提交，暂不推送以保留该run；终态后推送并跟踪本HEAD独立CI。仅API完成，G2页面、内部ID创建/同步防碰撞及G3–G6全链路和其余完整REQ继续保留，GitHub不能实际审计。phase push仍pending，不宣称F4/F5远端验收完成。

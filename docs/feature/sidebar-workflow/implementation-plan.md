# 完整范围实施计划

- Branch: codex/sidebar-workflow
- 输入：Confirmed requirements.md；design.md 为设计契约。
- 阶段：F3；所有切片验收后才能认定目标完成。

| 切片 | 文件/边界 | 范围 | 验证 |
| --- | --- | --- | --- |
| S1 | workspace_queries.go/http_workspace.go；workspace.tsx/navigation | REQ-001–005：授权服务端待办、发现分页与导航 | 项目/source/context ACL、分页与总数、筛选同一发现、空错/窄屏/键盘 |
| S2 | workflow_actions/store、comparison、run detail | REQ-016–022：复审闭环、归并、接受、责任人、覆盖、对比 | 版本冲突、接受到期、不自动判修复、固定身份、CODEOWNERS |
| S3 | workflow_policy、workspace_metrics、前端页面 | REQ-006–009：策略、关联入口、成本、质量、格式化噪音 | 数据真实范围、unknown、语言敏感空白、配置生效范围 |
| S4 | integrations_store、notification_dispatch、各适配器 | REQ-010–011/014：全部渠道、汇总、记录/重试 | 协议 mock、业务码、脱敏、网络超时/unknown、持久重启、去重 |
| S5 | provider GitHub、checks、tickets、callbacks | REQ-012–013/015/021：GitHub/GitLab、工单、身份交互、检查 | 两平台固定快照/源码权限、重复提交、外部创建不确定、重放/撤权 |
| S6 | diagnostics、frontend、文档与CHANGELOG | REQ-023 及全范围完成验收 | Go完整/race/vet、前端检查构建、权限/迁移/恢复、实际UI QA、需求逐项核验 |

每阶段更新此文档并独立 commit/push。原文件仅注册/委托，不堆积新职责。API 使用 design.md 的拟议契约，实际变化记录在实现节。

## 验证与交付

受控 mock 校验所有平台协议，用户配置测试渠道后再主动进行真实发送；无凭据不擅用生产配置，不将 mock 当真实投递。代码不执行被审计仓库。运行 targeted tests 后再按影响扩大；功能完成跑完整 Go/race/vet 和 npm typecheck/build，确保 web/dist 与源代码一致。每个 AC 单独记录证据和未完成状态。

## 回滚

默认新增自动化关闭；保留现有路由。切片可回滚代码，新表不删除数据；外部成功发送/建单记录需保留，回滚不重放已知成功请求。不自动合并 main 或发布。

## 实现进度

- F1：27855ad，需求已确认并推送。
- F2：b019a83，设计已提交并推送。
- S1–S6：未完成。

### S1 初始实现（尚未完成全部验收）

新增 /workspace/findings 和 /workspace/tasks；发现查询按单条问题过滤与分页，不让不同问题分别满足严重程度/类型而误匹配。SQL 复用 target/source/context 授权，总数与页面同读事务。前端新增待办/发现入口及日常/管理分组，保留旧路由。

已通过新增发现分页/同一问题筛选/撤销关联仓库权限/脱敏测试、既有会话授权测试、frontend typecheck 和 build。下一步仍需 HTTP 契约负例、待办分类测试、具体发现定位、实际窄屏与键盘验证。S2–S6 未完成；不认定完整目标已交付。

S1 补充：HTTP 分类与非法项目/筛选负例测试通过；具体发现链接采用 /runs/:id?finding= 编码身份，详情渲染后聚焦对应发现，旧链接兼容。已有复核状态沿用 pending/accepted/false_positive/fixed，没有另造 confirmed 状态。目标测试 0.874s；上一轮 race 4.372s；最终 UI build 通过。实际视觉/键盘验证尚未完成，S1 保持进行中。

### S2 对比切片

新增 GET /runs/:id/comparison?prior_id= 与详情对比面板。先在同一事务授权两份快照，再读取有界结果（1 MiB）；不同项目、来源或 MR 拒绝比较。源指纹须重算一致且两边唯一才能标再次观察；伪造/重复锚点不匹配；策略/BASE 差异、非成功运行、200 条输出上限显式提示。未再观察不自动判修复；不继承旧复核。

测试验证分页/ACL/比较拒绝/坏 trace 不影响结果、歧义锚点与伪造指纹；最终定向 Go 0.932s，前端 build 成功。首轮测试 fixture 违反已有运行唯一约束及前端引用错误已修正并复跑，未隐藏失败。此切片不等于风险接受、自动闭环、通知或 GitHub 集成已完成。

### S4 协议基础（部分实现）

新增飞书/钉钉/企业微信/Slack/Teams Workflows/Webhook 的纯请求构造与业务响应分类，签名/正文中不回传 secret，超长响应与未知业务状态不标送达，Webhook/Teams HTTP 接受和实际送达分离。当前没有持久集成配置、SMTP、发送 worker、汇总、页面或真实外部发送；协议测试仅为受控验证，不能认定 REQ-010–015 已交付。官方网页动态/读取限制已记录，后续继续核对协议。

### S4 配置与队列切片（仍未全部完成）

新增 platform_integrations 和 platform_notification_deliveries 表、迁移、管理员 GET/POST/PATCH /integrations，以及管理员集成页面。11 类配置类型包括邮件、各机器人/Webhook、GitLab/GitHub/Jira/Linear；类型支持配置不等同适配器或平台全链路已完成。查询只返回 has_endpoint/has_secret，地址/密钥/SMTP 收件人不回传；expected_revision 强制版本校验，凭据默认保留、可替换或明确清除。

配置更新取消 pending/retry，sending 标 unknown 并撤销 lease，避免旧 sender 确认新配置结果。队列事件键唯一、同事务领取、随机 lease token fence、最多 5 次有界限流重试；过期发送标 unknown，不自动重发。新增测试覆盖普通用户拒绝、脱敏、保留/清除、版本冲突、关闭重开数据库、重复事件/双领/过期发送/修改渠道竞态和邮件换行注入。

未实现：实际 HTTP/SMTP sender、自动事件采集、定时汇总、投递记录页面/主动测试/重试接口、工单和机器人交互、真实平台验证。仍需继续完整范围，不以配置菜单作为交付完成证据。

本切片最终验证：集成/队列/协议/工作台定向 race 11.038s 通过；frontend typecheck+Vite build 通过；补充 HTTP 未登录与普通成员拒绝集成读写测试通过。实际 UI 验证与完整回归尚未完成。

### S4 发送器与投递页面切片

新增 HTTPS sender（禁用 proxy/重定向、TLS≥1.2、超时及 64 KiB 响应上限、解析平台业务状态）、SMTP TLS/STARTTLS sender（固定头、收件人校验、超时/取消）。默认拒绝 loopback/link-local/未显式授权的私网，管理员写入完整凭据时可添加私网 CIDR；TLS 证书仍正常验证。SQLite 数据库及 WAL/SHM 在 OpenStore 后设置 0600。发送前重查配置版本及原审计发起者固定快照权限；配置变更不等于撤回已到达外部的请求，结果记录可为 unknown。

新增管理员 GET /deliveries、POST /deliveries/:id/retry、POST /integrations/:id/test，测试仅将固定摘要排入已保存并启用渠道。Runner 单独有界通知 loop，随生命周期停止，默认空队列不发请求。管理页面提供私网允许列表、主动测试与分页投递记录；unknown 重试必须明确 acknowledge_duplicate，已接受/送达不可重试，attempt≥5 不再重发。

验证：受控 TLS server 请求与业务码测试、地址限制、租约/去重/修改配置、持久配置/脱敏和显式未知重试的 targeted race 6.960s；前端 typecheck/build 通过。未发送真实外部消息，SMTP 尚需受控协议交互验证，实际 UI QA 未完成；自动审计事件、每日/周汇总、工单/Git平台/绑定机器人/生命周期其余范围仍未完成。

### S4 自动事件与汇总切片

新增 platform_notification_events、event_routes、collector 游标表及 run 完成/失败、review 更新触发器。事件与源结果同事务提交，回滚不遗留通知。run 事件从 NEW.result_json 计算风险等级，避免触发器顺序导致读旧投影。启用渠道按项目、事件、最低风险与配置更新时间筛选；低风险成功运行可过滤，失败/覆盖不完整仍提醒。收集时检查发起者快照权限，发送时再检查。

日/周窗口为 Asia/Shanghai 固定 UTC+8，周一边界；只处理已结束窗口。渠道版本+窗口/事件唯一键、路由回执与 outbox 同事务；重启重复采集不重建队列。已投递窗口有晚到补处理事件时创建补充汇总，不静默丢弃；摘要有界，超限提示到待办中心查看。每轮最多路由 200 事件，持久渠道轮转；当期未结束事件不阻挡其他渠道。

验证：新事件/回滚/风险等级/即时去重/日汇总/晚到补充/周一窗口，与通知/集成测试一起 race 4.806s 通过。首轮人为时钟 fixture 使旧事件时间落在新版配置之后，修正 fixture 后重跑。尚需高基数/权限撤销汇总/受控 SMTP/真实 UI 和全量回归验证。未发送真实外部消息；风险到期事件、生命周期、Git平台、工单及统计/诊断仍未完成。

### S2 风险处理与到期切片

新增 risk disposition 当前表/追加历史表、GET/PUT /runs/:id/findings/:fid/disposition 和详情惰性加载面板。项目 operator/admin 才可写，viewer 可读；接受风险需理由、具备该快照查看权限的责任人和未来到期时间，决定绑定 HEAD 且强制 expected_revision。人工确认修复必须 manual_resolution=true，展示为人工结论，不表示执行复现。复核表不修改，其他运行不继承。

通知 loop 每轮最多处理 100 条到期接受，原记录留历史，更新为 open、生成同事务 risk.expired 事件；工作台新增风险接受到期分类，发现列表显示 disposition、owner、expires_at。展开面板后关闭仍保留草稿，冲突加载不自动替换草稿。

最终定向 race（disposition/通知/工作台）10.079s 通过，前端 typecheck/build 通过；覆盖查看者拒写、版本冲突、HEAD 不符、到期幂等/历史、人工修复确认及跨运行不继承。首轮发现权限快照只有 ACL 身份而无 HEAD，以及 JOIN 后列名歧义，已修正并复跑。实际 UI/完整回归与自动修复验证仍待完成。此切片不自动判断已修复，不等于全部修复闭环/责任人路由已完成。

### CI 回归修复（用户 2026-10-07 提醒）

GitHub 73be028 的 run 37583262424 与 53068a2 的 run 37583804618 在全量 Go 测试中失败。TestReviewRevisionMigratesLegacyDecisionWithoutChangingHistory 用已升级数据库 DROP revision 模拟旧库，但新增通知触发器引用该字段，SQLite 在 DROP 时拒绝。真实旧库无这些触发器，因此修正 fixture：先移除两个依赖 revision 的新通知触发器，再重建旧结构；迁移后要求两触发器自动恢复、旧复核/通知历史均不重复。不删除生产触发器、不跳过或弱化迁移测试。

本地同名测试失败与 GitHub 日志一致；目标修复测试通过。此前定向通过不代表全量通过，当前需要完整 Go/race/vet 与最新提交 GitHub CI 成功才能关闭此回归。统计接口正在开发的未提交改动与此修复分开提交。

CI 修复关闭证据：b4c8a447ae19a9f4eceeb4e407a0c5b68aaa2cac 的 [GitHub CI 37584113122](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37584113122) completed/success，verify job 112670223096 全部检查成功。包含完整 Go/race/vet、operations tooling、govulncheck、npm audit/typecheck/build、embedded application build。本地 go test ./... platform 74.434s、完整 platform race 159.672s、go vet ./... 退出 0；本地统计接口未提交开发改动已明确与该修复 commit 分开，远端精确提交 CI 是本次回归关闭的权威证据。后续切片继续跟踪各自 CI；这不表示完整产品需求已经完成。

### S3 用量与质量统计切片（开发验证中）

新增 GET /workspace/usage、/workspace/quality 与侧边栏页面。时间范围按任务创建时间、起点包含/终点不包含、UTC 标准化，最长 366 天；项目与快照 ACL 复用工作台规则。用量 count/page 同一读事务，每页最多 20；逐运行显示模型、HEAD、耗时和已知消耗，不把本页合计冒充全范围账单。费用仅用运行策略内的价格；失败/带错误运行不标记记录完整；轨迹读取最多 1 MiB，超过预算或损坏明确未知，不返回原始错误、轨迹、源代码。质量按模型、策略版本及完整策略快照分组，只返回快照摘要，避免相同版本不同配置混合；显示人工反馈原始数量，不推导准确率/召回率，超过 500 分组明确截断。

定向测试通过：错误运行已知消耗、超预算/损坏历史、耗时、私密数据不返回、目标/来源权限、分页总数、不同策略配置分组、非法时间/项目和时区标准化。前端 typecheck/build 通过。本地全量 go test ./...（platform 73.972s）、go vet ./...、定向 race 3.079s 通过；远端 CI 待推送后验证，实际浏览器 QA 未完成。独立漏报真值登记、配额总览及全范围成本聚合仍需后续实现；本切片不宣称 REQ-008/009 全部完成。

### S3 关联仓库入口切片（开发验证中）

管理员导航新增关联仓库页，复用既有 ContextRepositories 和服务器权限/固定 SHA 管理 API，选择目标项目后编辑后续任务的关联配置。明确区分配置与历史运行实际快照，不把关联授权解释为用户权限。项目列表加载/失败时隐藏旧编辑器，刷新清除选择后重取项目；非管理员无导航入口且路由不渲染管理组件。既有项目页入口保留。前端 typecheck/build 通过，浏览器 QA 待验证。嵌入静态资源随此切片同步，包含统计页面与关联入口。

### S4 汇总发送权限复查（开发验证中）

补齐日/周汇总来源身份的服务器内部关联表，随 outbox 同一事务保存每个 run ID（包括摘要超限后省略的事件），不加入外部消息或公开投递记录。发送前在一个读事务复查所有来源运行发起者的目标/来源/关联仓库权限与目标项目身份。任一权限撤销时取消整个汇总，避免排队期间授权变化后仍发送旧摘要。没有可证明来源身份的旧汇总取消；向旧 pending 汇总追加新事件时先取消旧记录，再创建只含新事件且有来源回执的补充汇总。显式无运行测试消息保留现有行为。

定向通知 race 4.123s 通过（含事件与权限撤销），旧摘要隔离回归通过；通知定向 race 最终 4.454s、本地全量 Go（platform 75.079s，旧摘要追加修正另有定向覆盖）和 vet 通过，最新提交 CI 待推送后验证。统计提交 d7513e4 的远端 CI 37585456309 completed/success；关联入口 01904a2 已推送，CI 37585926395 正在运行。本修正不代表已完成所有高基数/真实渠道或生命周期需求。

### S3 项目审计策略关注与范围切片（开发验证中）

新增 workflow_policy 持久表与 GET/PUT /projects/:id/workflow-policy（管理员），强制 expected_revision、16 KiB 请求限制、封闭安全风险关注项和小写扩展名验证。关注项仅调整调查优先级，不限制其他风险或放宽证据门槛；项目扩展名与系统排除项合并，UI 明确其减少覆盖范围。新任务捕获不可变 workflow revision/配置，纳入既有完整 policy identity；相同提交不同策略产生独立运行，历史快照保持不变。管理员侧边栏独立审计策略页，冲突保留草稿，明确丢弃草稿并重载；默认无合并阻断。

定向测试覆盖角色拒写、非法配置、版本冲突、缺失项目、保存/读取、新旧运行快照与策略去重身份；最终定向 race 6.582s（含实际 worker 应用排除规则）通过，前端 typecheck/build、Go vet 通过。HTTP 缺失版本/冲突/请求预算验证通过；全量 Go platform 88.199s 通过，后续 worker 快照排除调整另有定向覆盖，远端精确提交 CI 待验证。实际 UI QA 待完成。合并检查 advisory/block 条件和可选格式噪声识别仍未实现，不宣称 REQ-006 已全部完成。

关联入口 01904a2 的远端 CI 37585926395 completed/success。通知权限修正 10414e2 已推送，CI 37586372762 已启动并跟踪。

### S3 可选空白变更提示（开发验证中）

项目策略新增默认关闭 format_noise_hints。任务完成后，按实际 included 文件查找新增/删除行顺序匹配、仅首尾空白不同的候选，最多 200 文件，不处理排除、删除/重命名/元数据或超读取预算变更。结果保存独立 format_hints，详情明确仅为词法提示：Python 缩进、字符串/模板空白仍可能改变行为，不能用于语义等价、安全或已修复结论。diff 与调查完全保留，不减少审计、不生成自动复核或改变运行状态。无匹配不意味着没有格式变更。

单元测试覆盖 Python 缩进、字符串内容行、token 变化、仅新增/相同/上下文、排除及不支持变更；断言完整 diff 未被删改。前端 typecheck/build 与相关策略/提示定向 race 4.750s 通过；实际 worker 可选开关/记录保存回归和全量 Go 正在执行，真实 UI QA 待完成。

通知权限修正 10414e2 的 CI 37586372762 completed/success。项目策略 7b03f13 已推送，CI 37586897943 正在运行。合并检查条件及其他完整要求仍未完成。

### S5 运行检查摘要基础（开发验证中）

任务详情增加本地 check_assessment，绑定 run ID 与 HEAD，区分 pending/running/failed/cancelled/skipped/incomplete/high_risk/completed/unknown。成功但有错误或覆盖缺口不降为完成；未知严重程度或缺失有效提交身份不视为正常完成。高风险计数保留，失败/不完整优先显示实际运行限制。始终 published=false/blocking=false，UI 明确当前未发布为 Git 平台合并检查；完成不等于安全或当前 PR 最新提交。展示模块独立于详情主文件，格式提示也复用该模块。

状态/身份/优先级定向测试通过，前端 typecheck/build 通过。本地完整 Go（platform 79.568s）覆盖空白提示切片，随后检查摘要新增逻辑另有定向测试覆盖；实际 worker 可选空白提示 race 3.688s 通过。提供者 API、持久发布队列、远端 HEAD 防旧结果覆盖、可配置阻断仍未实现，此基础不等于 REQ-021 完成。

项目策略 7b03f13 的远端 CI 37586897943 completed/success。

### S5 GitLab 提交检查协议适配（开发验证中）

依据 [GitLab Commits API](https://docs.gitlab.com/api/commits/)：状态发布目标为 MR 来源项目的固定 SHA；状态集合 pending/running/success/failed/canceled/skipped。新增显式 PublishRunCheck 适配器，先读目标 MR 并比对来源项目、BASE、HEAD，拒绝旧快照。检查名称带 run ID，隔离同 SHA 的不同运行；正文仅运行状态/高风险计数/模式，不发送发现描述、源代码或私密摘要。advisory 对高风险/失败/不完整用 skipped + 明确描述，不伪造 success；blocking 模式映射 failed。回执必须 ID>0 且 SHA/name/state 全部匹配，网络或异常回执保留 unknown，不当作成功。target URL 有界且禁止 userinfo。

受控 HTTP 协议测试覆盖 fork 项目路径、固定 SHA/分支/名称、过期 HEAD/BASE/来源变更零写入、非法 target URL、错误回执、服务端失败及 advisory/blocking 映射；定向测试通过，race/完整 Go 待收尾。该适配器尚未接入自动 loop，没有真实外部写入。持久发布队列/租约/撤权、最新运行仲裁与旧 blocking job 的处理、项目可配置条件、GitHub 全流程、真实服务验证仍待完成，不能宣称 REQ-021 已实现。

### S5 检查发布持久队列基础（开发验证中）

新增 platform_check_deliveries，以 run ID 为唯一键，保存 HEAD、发起管理员、模式、有限状态/错误码、租约和远端回执 ID。显式排队要求当前管理员快照权限、有效 HEAD、终态及同目标 MR 最新运行；重复同参数排队幂等，不覆写模式，不重试 unknown/published。claim 将旧 pending 运行标记 stale，过期 sending 保留 unknown，避免中断后盲目重发。finish 要求正确且未过期租约及 HEAD，确认成功要求有效回执 ID，错误码有限且不存原始提供者错误/URL。

协议+队列定向 race 初轮 2.530s，通过重复排队/模式冲突/旧运行/双 claim/旧租约/中断 unknown/空回执/重复确认；管理员身份持久字段与角色拒写扩展正在复验。协议切片完整 Go platform 77.758s 通过，随后队列表迁移另有定向测试覆盖。此队列尚无生产自动消费者或 HTTP 入口；发布前撤权/最新运行复查与状态展示将在后续接入，未发生真实外部写入。

空白提示与运行检查摘要 355c42c 的 CI 37587352105 completed/success。

队列角色回归发现 operator/admin 共用 roleRank=3，使内部 required=admin 判断允许 operator。HTTP 策略路由有 admin middleware，但内部 SaveWorkflowPolicy/QueueRunCheck 必须独立拒写。将 admin 提升为 rank 4（operator 保持 3），不改变 viewer/reviewer/operator 常规能力；补充实际 operator 策略拒写与发布排队拒写验证。当前尚无生产检查发布入口或自动消费者，未发生外部写入。修正后的项目/策略/队列 race 与全量回归需通过后提交。

### S5 检查发布前置复查与受控消费（开发验证中）

CheckPublicationRun 用单个读快照复查正确租约、最新运行、固定 HEAD、发布管理员、原发起者及目标/来源/关联仓库权限与启用状态；授权后才读最多 1 MiB 结果，保留 error 字段参与失败判定。Runner.DispatchRunCheck 串联 claim、复查、运行时凭据与固定仓库 origin 对照、协议发布、有限回执保存。协议适配器强制授权回调，并在远端 MR GET 之后、POST 之前再次调用，避免 GET 期间撤权绕过复查。

受控端到端测试覆盖有效 fork 检查回执落库、远端 HEAD 已变更零写入、GET 中途来源权限撤销零写入、POST 服务端异常记 unknown、终态不重复发送；前置测试覆盖租约不符、关联权限撤销、管理员禁用、来源项目禁用和排队后新运行。协议/队列/preflight/dispatcher 最终 targeted race 7.525s 通过。此前修正后的完整 Go platform 77.884s 通过；新增消费链路的完整 Go/vet 正在执行。无生产 loop 或 HTTP 入口，未写入真实服务，配置/状态展示/条件与自动事件接入仍待完成。

管理员权限修正 9c07eb4 的远端 CI 37588299319 completed/success。

消费链路完整 Go platform 77.493s 通过，随后错误分类/详情发布状态展示新增定向覆盖。最终 check/protocol/preflight/dispatcher/assessment race 8.043s 和前端 typecheck/build 通过。详情从持久记录展示 disabled/pending/sending/published/failed/stale/unknown；仅正确 HEAD、有效回执 ID 的 published 标为已确认，unknown 不假称未发送或成功。预检区分身份过期、权限变更与不可用错误，不混同所有失败。新增详情状态读取后的远端完整 CI 待推送验证。

### S5 可配置检查条件（开发验证中）

WorkflowPolicy 增加可选 checks：enabled（默认关闭）、advisory/blocking、最低风险等级、失败是否阻断、覆盖不完整是否阻断。仅管理员能保存，publisher 从实际保存者赋值，忽略客户端伪造身份；禁用清除 publisher。规范化在副本进行，不变异调用方或已有快照。项目策略 UI 保存关注项时保留读取到的 checks，避免默默清除其他配置。

发布适配器使用运行快照的等级/失败/覆盖条件，队列里显式授权的 blocking 模式决定输出模式；风险达阈值或未知等级不能伪造 success。失败/不完整在未选择阻断时用 skipped，并保留具体状态描述。删除被替代的旧映射，测试实际使用的映射路径。规则/发布者身份/不可变输入/策略回归 race 4.922s，以及检查链路 race 8.232s 通过；去重映射与前端保存保留改动另有定向验证。启用控件、自动收集/消费、配置撤销取消仍待接入，不把 API 数据字段当作已完成自动发布。

检查链路 ab56acb 的远端 CI 37589275964 completed/success。当前规则切片首次完整 Go 与前端 build 被错误地并行启动，Vite emptyOutDir 与 main CLI 构建读取 go:embed 静态目录发生竞争：main_audit_test 无可嵌入文件失败，platform 测试 87.110s 通过。这不是完整验证成功，也不归因于产品功能；前端 build 完成后顺序重跑完整 Go。后续所有涉及嵌入资源的 Go 测试/构建必须在前端输出重建完成后执行，不并行这两类操作。损坏历史检查规则追加拒发验证，不回退成假成功；最终定向 race 待收尾。

规则顺序验证关闭：完整 Go platform 93.968s 通过，前端静态目录构建竞争不再复现。最终规则/检查链路定向 race 8.960s 通过。追加策略变更撤销：与策略保存同事务取消项目 pending 检查、将 sending 标为 unknown 并清租约，旧授权/旧回执被 fence 拒绝；撤销回归 race 2.661s 通过。UI 显示 cancelled，记录未知的发送不伪装成取消前未送达。第一次撤销回归暴露代码插入未落入保存路径，修正并复跑；不隐藏首轮失败。本切片尚未接入自动采集/消费者和启用控件，远端精确提交 CI 待推送验证。

### S5 自动发布策略授权边界（开发验证中）

新增 QueueAutomaticRunCheck：从运行快照读取发布者/模式，但在实际排队事务内重新验证管理员权限、启用状态、发布者、模式、当前策略 revision 和完整 checks 条件均与捕获配置一致。未捕获授权、禁用/更新策略、伪造发布者或模式不允许进入队列；旧任务不能在配置撤销后重新自动排队。手动显式管理员排队保持独立语义。自动采集循环与启用控件尚待接入。

### S5 自动检查采集基础（开发验证中）

新增持久 last_run 游标与 CollectRunChecks，每批最多 100 条，只选启用项目、当前管理员发布者、捕获 enabled=true、当前相同策略 revision、最新 MR 终态且未已有投递的运行。实际排队仍通过事务内完整策略/权限校验。游标轮转公平重扫，单条失败不阻止本批其他行；游标推进与真实入队分开，异常/中断未入队的行后续仍可扫描，不标记成已发送。

106 条受控样本（含一条损坏快照）测试证明批次上限、异常行不饿死后续、105 条有效运行全部排队、DB 重开后不重复、修复损坏记录后补排、重复收集零新增；自动授权/采集 targeted race 3.214s 通过。自动授权/策略撤销/队列相关上一轮 race 3.594s 通过。无生产 loop/启用控件，未发生真实外部写入，阻断检查旧 job 与安装命名空间策略仍需完善。

480df1a 的远端 CI 37590526634 completed/success。当前切片完整 Go/vet 待验证。

自动采集完整 Go platform 79.373s 通过，后续发送前自动授权复查与迁移另有定向覆盖。队列持久 automatic 标记（旧表增列默认 false），claim 保留该身份；发送前对自动记录重新验证当前完整授权，租约校验同时绑定 automatic/blocking，不能用伪造显式标记绕过。旧版本记录保留显式管理员授权，不推断成自动。迁移逻辑独立模块，避免 Store 文件继续聚集职责。自动身份/当前策略撤销/100 条公平采集/队列/预检与迁移定向 race 5.236s、Go vet 通过；未接入生产自动循环。

### S5 检查发布安装身份隔离（开发验证中）

自动采集提交 1c1a249 的远端 CI 37591714750 completed/success。继续在现有已确认 REQ-021 范围内完善发布身份：检查名称使用数据库持久安装 namespace 加运行 ID，复用已有安装身份迁移，不生成每次启动变化的身份。发布者缺失身份或非 16 字节十六进制身份时拒发；回执仍须匹配完整名称和 HEAD。两个安装的同编号运行、同一提交在受控 GitLab 服务得到不同检查名，非法身份零新增 POST；检查协议/消费者定向 race 5.613s 通过。

本切片处于 P8/F4–F5，依赖已有确认需求、设计、队列与授权预检；无需新用户决策。完整 go test ./... 通过（platform 88.463s），go vet ./... 和 git diff --check 通过；远端精确提交 CI 待推送验证。安装隔离不解决旧运行阻断检查残留，自动消费循环、启用控件及旧检查处理仍未完成；没有发生真实外部检查写入。

### S5 同 MR 检查稳定名称（开发验证中）

P8/F4：现有确认需求 REQ-021 要求新结果对应当前提交、旧结果不能覆盖。检查名改为安装 namespace + 目标项目 + MR，不再为每次运行创建独立 job；运行编号留在描述与链接。依据 [GitLab 外部状态更新规则](https://docs.gitlab.com/ci/ci_cd_for_external_repos/external_commit_statuses/)，同 pipeline 同名终态可通过新状态重试并隐藏旧 job。保留发送前两次授权与 HEAD/base/source 预检。验证同 MR 不同运行名相同、不同 MR/安装名不同。尚需明确绑定 pipeline、处理发送竞态以及已有 run 名旧 job；稳定名称本身不证明完整阻断链路可用，自动发布仍未启用。

追加 MR head_pipeline 显式绑定：存在时要求 ID 正数、项目匹配源项目、SHA 匹配审计 HEAD，POST 带 pipeline_id；合并结果 SHA 或目标项目 pipeline 不冒充源提交 pipeline，拒绝写入。没有 head_pipeline 时保留 GitLab 默认选取行为，尚不能保证重复 pipeline 场景唯一性。实际 POST 参数、不同 SHA/项目零写入、稳定名称与发布预检定向 race 5.591s 通过；完整 Go/vet 待完成。没有执行旧 run 名检查清理，未启用生产发布。

本切片完整 Go 测试通过（platform 80.621s），go vet ./... 退出 0。上一笔 df043f1 的远端 CI 37592704787 仍 in_progress，等待终态后才推送下一笔，避免工作流并发取消已启动验证。检查适配器仍无生产自动消费，不把受控协议测试当作真实 GitLab 合并规则验证。

### S5 回执前本地身份复核（开发验证中）

P8/F4，沿用 REQ-021 与既有授权契约：POST 已发生后，保存 published 前再次验证租约、权限和最新运行。失效时记 unknown 而非未发送/成功，因为远端可能已接受；不自动重发。受控测试覆盖 POST handler 内权限撤销、新运行替代与正常回执。该复核限制本地成功声明，不是远端原子比较交换，发送期间的远端竞态与后续收敛仍需完善。

安装隔离 df043f1 的远端 CI 37592704787 completed/success。回执后复核定向 race 7.686s、完整 Go（platform 80.998s）、go vet ./... 与 diff 检查通过。远端有回执但本地权限/身份失效时持久保留 remote_id 与 unknown，详情不标 published；再次消费没有第二次 POST。与待推送稳定 MR 名称提交 99c2856 一起推送后，继续跟踪新精确 HEAD 的完整 CI。

### S5 管理员检查规则编辑（开发验证中）

P7/F4，沿用已确认 REQ-021 和既有 workflow-policy API：管理员页编辑 enabled、模式、最低等级、失败及不完整处理，默认关闭/提示/high，发布者由服务端赋值。加载、版本冲突保留草稿、保存忙态、项目切换和重新加载复用现有页面契约。明确显示当前自动发布尚未接入，保存不是远端发送成功；不改既有关注项或排除项契约。本切片先完成可审查配置入口，生产消费者、旧检查与远端竞态仍待收尾。

前端 npm run build（含 tsc -b）通过，嵌入静态产物一并更新；构建完成后顺序执行 go test ./... 全部通过（后端缓存结果），diff 检查通过。尚未做浏览器窄屏、键盘和完整 API 交互验证，不以构建替代 UI 验收。上一提交 983f773 的 CI 37593561827 仍运行中，本切片提交后等待其终态再推送。

### S5 自动检查消费者接入（开发验证中）

P8/F4，既有授权默认关闭且管理员编辑规则已完成。Runner 在有 Settings 时管理独立检查循环并等待关闭；每轮收集有界批次，然后最多消费 5 条，每次远端调用 20 秒超时。个别采集失败不饿死已入队记录，未获得租约时停止本轮，unknown 不重发。受控验证默认关闭零 POST、管理员授权新任务自动发布、重复采集不重发及禁用旧配置拒发。旧名称清理和远端发送竞态仍须继续验证，不能据此声称所有 GitLab 合并策略已验证。

983f773 的远端 CI 37593561827 completed/success。自动消费者默认关闭/启用/撤销/不重发/每轮 5 条及下轮补齐/cancel 退出定向 race 10.301s 通过；前端 build、完整 Go（platform 84.067s）、vet 和 diff 检查通过。前端文字更新为实际自动发布行为，明确未捕获授权的历史任务不补发；静态产物一起提交。验证使用受控 HTTP，尚缺实际 Runner 定时启动的端到端证明、窄屏/键盘验证和真实 GitLab 合并策略验证。

### S5 自动检查后台生命周期验证（开发验证中）

P9/F5：沿用既有消费者和 Runner 生命周期契约，新增实际 Start/Stop/重新 Start 的受控服务验证，不直接调用 checkCycle。要求定时器发布、成功回执落库、Stop 等待退出且重复停止安全，停止期间新任务不发布，重新启动只发送新增授权任务、不重放已确认任务。测试隔离配置同步文件写入，保持真实数据库、Runner lease、定时器与 HTTP 协议链路。

首次实际后台生命周期 race 6.201s 通过，随后追加停止后跨完整 2 秒定时周期的观察（2.2 秒），确认新运行既无入队也无 POST；重新启动后收到第二次且唯一的回执。生命周期 + 每轮发送边界 race 8.909s 通过，vet 退出 0；本次仅新增验证文件和记录，未改生产行为。前一功能提交 285ccb5 的 CI 37594213884 仍 in_progress，当前新增测试需随下一次推送验证完整 CI。浏览器交互、旧检查处理和真实平台合并规则仍未关闭。

### S5 规则页面权限失效修正（开发验证中）

P9/F4–F5，受控本地 API + 真实浏览器验证：默认关闭/提示/high，启用切换、阻断模式和等级可编辑；409 保留已编辑草稿并显示错误。403 首次验证发现旧编辑区仍可见，修正为 401/403/404 清除 draft 并显示错误/重试，409 和普通临时错误仍保留草稿。285ccb5 的完整远端 CI 37594213884 已 completed/success。本地 fixture 不使用真实凭据，不对真实代码平台写入。

403 修正浏览器复测：仅剩权限错误与重试按钮，旧字段和保存按钮消失，避免 useEffect 从缓存重新填充失效草稿。360×800 窄屏发现 checkbox 被通用 label 样式纵向分隔，复用已有 .check 样式修正，截图确认发布设置和保存按钮均无裁切；空格切换授权、Tab 到模式控件、回车保存，版本从 0→1 且显示成功。前端 build 通过；测试结束恢复视口、关闭临时 tab 与 localhost fixture。观察到窄屏侧边栏分组标题仍竖排，另需优化，未声称整站响应式验收完成。完整 Go 验证进行中。

完整 Go 验证通过（platform 85.895s），包含新增后台生命周期测试。待推送 15535b0 与页面修正一起推送，远端精确 HEAD CI 仍需跟踪终态。

### S5 手机导航分组排版（开发验证中）

P9/F4，确认 REQ-003/005 的窄屏和键盘要求：前次 360 像素浏览器截图中分组标题被 flex 压缩而逐字竖排。仅修改已有小型响应式样式文件，让标题与链接不收缩、标题单行居中，并提高暗背景标题对比度，保持横向滚动和原路由。验证 360 像素与桌面布局、键盘焦点可到末端导航。不改高复杂度主入口或新增组件。

实际浏览器 360×800 截图确认两组标题保持单行；Tab 从系统设置到操作日志时，横向容器自动滚动并显示末端焦点，回车后路由变为 /events 并展示操作日志空态。恢复默认桌面视口截图确认垂直分组保持原布局。npm run build（含 typecheck）通过，静态产物同步更新，diff 检查通过；仅 CSS 改动，不新增镜像实现的测试。临时 tab/viewport/localhost fixture 已清理。前一 e63aba7 的 CI 37595546480 仍运行中，下一笔推送等待其终态。

嵌入应用 go build 退出 0（输出 /tmp/aimangebot-responsive-smoke，不运行服务、不改部署）。本切片未改后端行为，因此没有重复全量后端测试；远端完整 CI 在推送后验证。

### S6 Linear 工单协议基础（开发验证中）

P8/F4，确认 REQ-013 与 design.md 工单契约：先实现独立请求构建及有界回执解析，再接关联表/权限/发送/UI。依据 [Linear 官方 GraphQL](https://linear.app/developers/graphql)，使用 issueCreate(input: $input)，variables 传值，回执必须无 errors、success=true 且 issue ID/URL 有效；HTTP 200 本身不代表成功。文本长度和团队 UUID 校验，错误内容不回传，未知结果不当作可直接重试。该基础不执行网络写入，不声称已有实际工单能力。

协议定向 race 1.648s、go vet ./... 和 diff 检查通过。验证注入式标题仍仅位于 variables、无团队名称替代 UUID、损坏/超限响应和 500 不报成功、200 携 errors 不报成功、凭据 URL/仿冒域名不留链接。尚未接入发送器、服务端团队配置、ticket_link 持久幂等表、权限预检或 UI，REQ-013 未完成。上一提交 CI 37595546480 仍运行，待其终态后推送本协议基础及待推送导航修复。

### S6 Linear 受限发送适配器（开发验证中）

P8/F4，沿用工单明确操作授权契约：仅官方 https://api.linear.app/graphql 收取 API token，调用已有禁代理/禁重定向、DNS/IP 校验和 TLS 发送客户端；请求 15 秒、响应 64 KiB 上限。每次创建需调用方提供授权复核，POST 前检查，成功回执后再次检查；写入后失效保留 unknown，不假装没有创建。发送器不自动重试。测试注入 RoundTripper，不对真实 Linear 发请求。持久幂等/团队配置/HTTP 和 UI 仍未接入。

发送协议 race 1.662s 通过，涵盖授权缺失/撤销零请求、合法 POST 带身份和 deadline、发送后撤销保留回执但 unknown、传输错误不泄露原始错误、响应超限和 GraphQL 错误不报 created。完整 Go（platform 86.848s）、vet、diff 检查通过。e63aba7 的远端 CI 37595546480 completed/success，待推送导航 36ffa5f、协议 b715636 与本适配器一起推送后跟踪新 HEAD CI。REQ-013 仍未完成，未发送真实工单。

### S6 工单关联持久预留（开发验证中）

P8/F4，确认 REQ-013：创建前按 run/finding/integration 唯一预留，随机持久 idempotency_key；现有记录无论 pending/created/unknown 均复用，不重置或覆盖。事务中检查 operator 的完整快照权限、发现存在、HEAD、启用渠道、Jira/Linear 类型、目标项目范围与渠道 expected revision。记录捕获 actor/HEAD/渠道 revision，读操作重新检查 viewer 快照权限。此阶段只预留 pending，没有网络写入；领取租约、发送前授权复核和 API/UI 后续接入。

首轮测试修正两处 fixture 错误：Store 私有迁移方法为 migrate；撤权后现有权限层隐藏资源，精确错误为 sql.ErrNoRows。修正后权限/HEAD/渠道 revision/重复 unknown 保留/重复迁移保持身份的定向 race 3.088s 通过，vet 和 diff 检查通过。完整 Go 验证进行中，前一 8299131 的 CI 37596410965 仍 in_progress。没有网络发送，本切片不构成工单功能全链路完成。

本切片完整 Go 验证通过（platform 86.327s）。后续仍需 ticket 租约/发送授权复查、明确团队与 Jira 项目映射、HTTP/UI、真实服务验证；当前保持唯一预留的安全基础，不把 pending 当作已建工单。

### S6 工单领取与回执栅栏（开发验证中）

P8/F4，沿用持久幂等契约：旧表增列 lease/lease_until/error_code，领取随机 token、60 秒期限；过期 sending 转 unknown，禁止自动重发。确认回执必须匹配 ID/HEAD/幂等身份/渠道版本/actor 和有效租约。created 必须有效 Linear 回执，Jira 暂无适配器时不得伪造 created。验证旧租约、过期确认、重复确认、迁移重复执行和 unknown 不再领取；尚未接入实际发送消费。

审核中修正空队列分支：过期 sending 的恢复与查询同事务，查询无 pending 时必须先提交恢复，再返回 sql.ErrNoRows，不能 defer rollback 丢失 unknown 状态。无效期限也转 unknown。租约/迁移/预留/Linear 协议定向 race 3.281s、完整 Go（platform 87.084s）、vet 与 diff 检查通过。unknown 可保留经过校验的远端回执；failed 不接受远端身份，任何状态的链接均拒绝仿冒域名/凭据 URL。前一 a5bf5a9 的 CI 37597019196 尚 in_progress，等待终态后推送。

### S6 工单发送授权快照（开发验证中）

P8/F4，沿用 run/finding 明确身份操作，不要求历史发现必须属于最新 MR 运行。在同一个读事务内重查 actor 的完整 snapshot operator 权限、发现存在、HEAD、目标/源/关联项目启用、发送租约完整身份、当前渠道版本/类型/启用/项目范围；凭据仅在所有预检通过后返回内部消费者，不向 API 暴露。限制凭据读取大小，不接受损坏 JSON。测试覆盖 fork/配置/角色/租约变化，尚未接入外部写入。

预检/领取/预留/协议定向 race 12.757s、完整 Go（platform 88.675s）、vet、diff 检查通过。独立受控样本验证 fork 源仓库撤权/禁用、渠道 revision/启用/项目范围变化、损坏凭据、旧租约、伪造 actor/HEAD 全部拒绝且返回空凭据和空 snapshot。a5bf5a9 的远端 CI 37597019196 completed/success；待推送 993b156 与本预检一起推送后跟踪精确 HEAD。实际消费者、团队映射、API/UI 与 Jira 仍未完成。

### S6 Linear 明确团队映射与消费（开发验证中）

P8/F4，确认工单操作范围：凭据加入可选 linear_team_id（UUID，服务器保存、不回传），消费者从持久记录读取该映射，不接受发起者覆盖团队。领取后预检，再由发送器 POST 前后复核；输出仅运行/发现 ID、提交和平台详情链接，不导出源码。Jira 未实现时记录明确 unsupported_provider，不虚构工单。没有 Runner 自动启动或 HTTP 创建入口，此阶段只消费显式预留记录，使用受控 HTTP 测试。

消费者/预检/Linear 协议定向 race 13.892s、完整 Go（platform 88.862s）、vet 和 diff 检查通过。受控数据库 + HTTP transport 验证配置映射确实进入 variables、有效回执 created 落库、发送后版本变化保留回执但 unknown、缺团队/已禁用零 POST、传输未知和终态不重发，原始审计摘要不进入发送 body。已有可选字段保持旧配置可读；尚需管理页面团队输入、HTTP 创建/查询与调度入口，不声称可从产品 UI 创建工单。前一 a9f113e 的 CI 37598077800 仍运行，待其终态后推送。

### S6 管理员 Linear 团队输入（开发验证中）

P7/F4，复用已有完整凭据替换/默认保留契约：Linear 提供团队 UUID 输入，启用并写入凭据时要求团队和 API token；说明官方 endpoint。返回元数据仅 has_team_mapping，不回传 UUID 或凭据值。只修改凭据编辑分支，不重构原有集成表单。保存不会创建工单；待 API/详情页明确用户操作后触发。

前端 build 通过。新团队元数据测试首次发现 SaveIntegration 的手工返回路径未设置 has_team_mapping，而列表扫描已设置；补齐后新建、仅改名称保留团队/token、列表仅返回存在标志的 targeted race 4.912s 通过。先前全量测试已用旧代码启动，为避免把旧版本结果当作修复验证，主动终止其具体 go test/platform.test 进程（退出 143）；修复后重新完整 Go/vet，非超时重启。a9f113e 的 CI 37598077800 completed/success；团队表单浏览器验证仍待完成。

修复后完整 Go（platform 98.143s）、vet 和 diff 检查通过，前端静态产物同步提交。Linear/Jira 不参与通知事件收集，工单配置保存不会隐式自动创建；现有 notification collector 的支持类型过滤已核对。待推送消费者 7903241 与本配置入口一起推送，下一精确 HEAD 的远端 CI 待跟踪。

### S6 工单查询与明确预留 API（开发验证中）

P8/F4，沿用已确认 design.md 的 operator 创建、viewer 查询契约。新增 POST /runs/:id/findings/:finding_id/tickets，要求 integration_id、渠道 expected_revision 和 HEAD；新预留返回 202，已存在返回 200，响应 ticket/state 不等同远端创建。GET 对应渠道记录重新检查完整快照 viewer 权限。沿用认证路由和 Store 授权；8 KiB 请求上限。没有在 HTTP handler 同步发送网络请求，后台调度和详情入口仍待接入。定向 HTTP 测试覆盖缺字段/无效 HEAD/超限体，完整认证/成功交互验证尚待补齐。

### S6 工单 API 真实会话验证（开发验证中）

P9/F5，实际 Store.Login 会话 cookie、HTTP.Register 认证/origin middleware、真实数据库与工单 Store 链路：无会话 401、viewer 写入 403、跨站写入 403、管理员新预留 202、已有 unknown 重复请求 200 且不重置、viewer 查询 200、项目撤权后 404，最终唯一记录数为 1。响应不含内部 idempotency_key 或 token。HTTP 定向 race 3.787s 通过，vet/diff 检查通过；没有绕过认证设置 currentUser。当前完整 Go 验证进行中，前一 c07558a 的 CI 37599173842 仍运行。

S6 真实会话验证的完整 Go 测试最终通过（platform 189.383s）；同一次运行持续观察至终态，没有因观察超时重启。远端 c07558a CI 37599173842 查询连续出现 GitHub API EOF，目前不能确认新终态，不推送新提交覆盖其运行。

### S6 显式工单后台消费生命周期（开发验证中）

P8/F4，Runner 启动独立 2 秒工单循环，单次最多消费 5 条、每条 20 秒超时，使用当前 public_url 和已有授权预检/租约消费者。循环纳入同一取消上下文与 WaitGroup；不收集审计事件自动建单，只处理明确预留记录。结果 unknown 保持终态，不自动重发。增加实际 Start/Stop/Restart 测试，通过受控 transport 验证停止后待处理记录不发送、恢复后发送且已创建记录在再次重启后不重复创建。测试注入为未导出的 Runner 字段，生产默认仍使用受限 HTTP 客户端。

HTTP/消费者/真实 Runner 生命周期定向 race 14.921s、vet 通过；当前新循环版本的完整 Go 测试已启动，等待终态。API/后台链路已接通，但安全可选渠道列表、发现详情操作 UI、Jira 适配器与实际浏览器验证仍未完成，不声明完整工单能力完成。

后台消费版本完整 Go 测试通过（platform 96.081s）。

### S6 发现可用工单渠道（开发验证中）

P8/F4，新增 operator 专用 GET /runs/:id/findings/:finding_id/ticket-channels，重新校验完整发现快照权限。返回仅 ID/revision/name/provider；按目标项目范围过滤，排除禁用、损坏或缺团队/token/非官方端点配置，Jira 适配器完成前不提供其选择。凭据读取在 SQL 限制 64 KiB，候选最多 500，超过明确失败不悄悄截断。真实数据库测试覆盖 viewer 拒绝、缺发现、项目范围/禁用/损坏配置/不支持类型过滤和输出不含 token/团队 UUID。初次定向 race 2.716s 通过，随后 SQL 凭据读取上限调整后重新验证；vet/diff 通过。详情 UI 与完整工单状态列表仍待完成。

SQL 上限调整后的定向 race 2.651s 通过。GitHub 恢复访问，精确 c07558a 的 CI 37599173842 已核实 completed/success。新渠道版本完整 Go 测试运行中，待终态后统一推送本地 API/后台/渠道提交并跟踪新 HEAD CI。

渠道版本完整 Go 测试通过（platform 103.192s）。

### S6 发现详情工单入口（开发验证中）

P8/F4，新增 viewer 可读的发现工单列表（完整 snapshot 权限、最多 500 超限明确失败、内部 key 不输出）。详情卡片按需打开工单区，operator 可选择服务器过滤后的渠道，POST 携带固定 HEAD/revision；响应展示 pending 而不虚构创建成功，已有任何终态/待处理记录禁用重复创建，unknown 提示人工核对且无自动重发，显式刷新状态。权限/提交错误清空旧数据并要求刷新，加载失败不展示旧选项。远端链接严格允许 Linear HTTPS issue 路径且不含凭据/query/fragment。创建仅使用 can_submit 显示权限，服务器仍独立校验完整 snapshot operator。

列表/渠道/HTTP 定向 race 5.911s、前端 build、diff 检查通过。首次写入命令工作目录误设 frontend 导致路径不存在，未产生源变更；改正根目录后写入并重新构建。尚未完成浏览器加载/错误/窄屏/键盘验证，因此不宣称 UI 验收完成。Jira/机器人和其余完整范围仍待实施。

详情入口版本完整 Go 测试通过（platform 115.810s），同一进程观察至终态。新增真实认证路由断言：admin 可读渠道、viewer 不可读创建渠道但可读 unknown 工单列表、列表不返回内部 key、撤权后列表 404；定向 race 4.270s 通过。

受控本地浏览器验证：打开实际编译页面的发现卡片，空状态和默认禁用创建按钮正确；选择渠道后用 Tab/Return 提交，fixture 严格校验 HEAD/revision/渠道 ID，unknown 显示核对提示且禁用重复创建；撤权模拟 403 后刷新清除工单及旧渠道，仅显示错误和刷新入口。360×800 截图检查错误区、按钮完整可见，恢复 viewport 并关闭临时 tab。未访问 Linear 真实账号、未创建远端工单。加载慢响应、409 草稿、viewer 和配置表单的完整浏览器样本仍待补齐。

### S6 入队前渠道准备状态（开发验证中）

P8/F4，明确创建入口与可选渠道使用相同 Linear 准备状态判定。ReserveFindingTicket 在同一授权写事务内有界读取凭据，要求官方端点、合法团队 UUID、非空有效 token 和凭据校验；未实现的 Jira 直接拒绝，不制造必然失败的 pending 记录。列表凭据 SQL 上限改为 BLOB 字节长度，避免多字节字符绕过字符数量界限。旧工单读取不受此限制；发送前后仍复核权限和配置。

预留/租约/发送/生命周期/API 定向 race 27.691s、渠道拒绝定向 race 2.638s、vet/diff 通过。旧测试有效渠道补齐团队映射；missing_team 消费测试改为先用有效映射预留、随后模拟配置被移除，仍覆盖排队后配置失效的零网络发送。增加缺团队专项入队拒绝样本，待最终结果。前一精确 HEAD 9241467 的 CI 37601382357 仍 in_progress，未推送覆盖。

### S6 完整快照项目启用状态（开发验证中）

P8/F4，将目标/fork 源/关联仓库启用状态检查提取为同一事务内 helper，由可选渠道、明确预留、发送预检调用。停用任一相关项目立即拒绝新的工单入队及渠道选择；viewer 读取已有工单历史仍沿用原完整快照权限，不用启用状态抹除历史。定向 race 14.380s、vet/diff 通过。测试覆盖完整 snapshot 的三个项目启用变化，以及真实 fork 发现预留拒绝后工单表仍为空。前一渠道准备版本的完整 Go 测试 82744 仍 live，等待原进程终态；新修改需随后执行自己的全量验证。CI 37601382357 重新核实仍在 Go tests and race checks 阶段，无终态，不因长耗时重启或推送覆盖。

### S6 工单安全错误诊断（开发验证中）

P8/F4，TicketLink 查询增加 updated_at 与安全 error_code，SQL 仅投影四个已定义诊断码，未知/损坏字段返回空码，不返回上游错误文本。详情页将配置失效、权限/配置变化、远端未确认、不支持渠道转为具体中文提示；unknown 继续要求先核对远端且不自动重发。消费者测试验证每种结果的错误码及更新时间，真实 HTTP 测试向 DB 注入非白名单 PRIVATE_UPSTREAM_ERROR 并确认列表不输出。定向 race 8.054s、前端 typecheck 通过。前一完整 Go 测试 24331 已重新确认进程仍 live，待其终态后构建新前端，避免 go:embed 与 Vite emptyOutDir 并发。新功能尚需 build、全量验证及浏览器诊断提示核验。

项目启用版本的完整 Go 测试通过（platform 112.270s）。诊断版本前端 build 通过，产物重建完成，随后启动新完整 Go 测试 14954（无 Vite/Go 并发）。受控实际页面验证 409 渠道版本冲突后清除旧选择/创建表单、显式刷新重新加载默认未选渠道；成功模拟 unknown 时具体 creation_unacknowledged 提示可见且创建禁用；can_submit=false 的 viewer 页面仅显示现有工单和刷新，无创建渠道与按钮。临时 tab 已关闭。前一精确 9241467 的 CI 37601382357 已核实 completed/success。慢响应加载和管理员团队表单浏览器验收仍未完成。

诊断与重建版本完整 Go 测试通过（platform 105.561s）。

### S6 工单实际数据库重开恢复（开发验证中）

P9/F5，使用独立文件 SQLite，实际 Close/OpenStore（执行迁移），验证 created/带回执 unknown/过期 sending 三种状态。重开后原 ID/idempotency_key、有效远端回执持久化；过期 sending 恢复 unknown 与 creation_unacknowledged，即使没有 pending 查询也提交恢复。再次明确预留返回原记录，不重置状态；旧发送确认全部拒绝。测试不使用真实远端，sql.ErrNoRows 证明无可领取记录。定向 race 3.295s、vet/diff 通过。产品版本完整测试已通过，本提交只新增重开验证与记录。

### S6 Jira v3 创建协议基础（开发验证中）

P8/F4，参考官方 https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/ 与 v3 概览，新增纯协议模块。项目/问题类型使用明确数字 ID，summary JSON 编码且有界，description 构造 ADF doc/paragraph/text；不拼接用户 JSON。仅接受 HTTP 201、有界合法 JSON、合法 id/key、无 errors/errorMessages，self 必须精确属于配置 HTTPS 根地址的 /rest/api/3/issue/<id>，展示链接由受信 endpoint 与校验 key 生成。其余结果 unknown，无原始远端错误输出。Jira 尚未接入凭据映射、消费者、持久回执验证和 UI，现有可用渠道继续不提供 Jira。

协议定向 race 1.679s 通过，覆盖字段映射/引号、安全 ID、伪造站点/key、错误响应、超限与非 201。官方大页直接 open 因内容过大失败，搜索官方源及概要核对，不以第三方替代官方协议依据。前一 9a80965 的 CI 37602358756 当前 in_progress，未追加推送。

### S6 Jira 受控发送器（开发验证中）

P8/F4，官方认证依据 https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/，显式 Username（账户邮箱）+Token（API token），不使用 Password 字段。发送器仅接收 HTTPS 根地址、无 user/query/fragment、有效凭据与固定项目/类型，POST /rest/api/3/issue，15 秒上下文、64 KiB 回执限制。生产默认复用禁止代理/重定向、DNS/IP 限制的 HTTP 客户端；必须传授权回调，POST 前/有效回执后复核，撤权后保留回执但 unknown。不自动重试、原始传输和权限错误不输出。

纯协议/发送器定向 race 1.668s 通过，受控 transport 覆盖 Basic header、目标 URL、deadline、前置撤权零 POST、有效回执、后置撤权保留身份、伪造来源、配置错误、传输未知。尚未调用真实 Jira，未接入 Store/Runner/UI，Jira 仍不提供创建选项。后续需项目/问题类型持久映射、渠道版本与可靠回执存储、用户详情入口、必填自定义字段处理和认证范围说明。

### S6 Jira 固定项目/类型映射（开发验证中）

P8/F4，IntegrationCredentials 增加可选 jira_project_id / jira_issue_type_id，配置校验拒绝非正数/超限/路径/换行 ID，旧无映射配置保持可读。公开 Integration 仅增加 has_jira_mapping 布尔，scan 与 Save 返回路径一致，不返回项目/类型 ID、邮箱或 token。真实 Store 测试验证新建、仅重命名保留凭据、列表存在标志、数据库持久映射与敏感值不出响应。Jira/Linear 相关定向 race 2.888s、vet/diff 通过；增加非法 ID 与旧配置专项后重新执行定向验证。Jira 仍未进入候选渠道/消费者/UI，不把持久配置当作完整创建能力。前一 Jira 发送层完整测试 13084 仍运行，等待同一进程终态后验证本版本；9a80965 CI 37602358756 仍 in_progress。

Jira 发送层前一完整 Go 测试通过（platform 101.501s）。

### S6 Jira 发送绑定配置映射（开发验证中）

P8/F4，jiraTicketReady 统一要求完整项目/问题类型 ID、HTTPS 根端点和合法账户/API token。发送器移除单独项目/类型参数，从服务端 IntegrationCredentials 获取映射，减少未来消费者覆写项目的入口。受控 transport 解码实际 POST，验证项目和类型来自配置。新增缺项目/类型、空认证、冒号用户名、换行 token、HTTP/路径/query 端点拒绝样本。Jira 全部定向 race 2.381s、vet/diff 通过。尚未接入工单预留/持久回执/详情入口；完整 Jira 能力仍未完成。

### S6 工单回执固定站点（开发验证中）

P8/F4，工单表迁移增加内部 endpoint_origin（默认空兼容旧记录），预留时保存凭据中的端点但不回传 API。领取包含固定端点；Finish 租约完整身份新增端点匹配。Jira created/带回执 unknown 必须数字远端 ID 与该固定 HTTPS 根站点的合法 /browse/<key>，Linear 沿用独立身份规则；failed 仍不接受回执。Jira 暂未开放预留，因此测试在独立数据库显式构造 Jira 记录验证持久层，不视作产品端到端能力。

协议/迁移/重开/HTTP 定向 race 8.068s，固定站点/租约拒绝定向 race 3.482s、vet/diff 通过。覆盖合法 Jira 回执持久化、外站链接拒绝、伪造 endpoint 即使配套伪造链接也因 SQL 身份不符拒绝；已有 Linear 租约测试补充端点伪造。前一映射绑定完整 Go 65571 仍运行，待同一进程终态；本版本全量验证尚待执行。

Jira 映射绑定版本完整 Go 测试通过（platform 114.462s）。

### S6 Jira 预留与消费接通（开发验证中）

P8/F4，可选渠道与预留通过同一 ticketProviderReady 分派 Linear/Jira，Jira 要求持久项目/类型与认证完整。后台 Dispatcher 分派 Jira sender，当前凭据端点必须与持久 endpoint_origin 完全一致，避免旧记录发送到新站点。现有 Runner 队列自动消费明确预留记录，仍无审计事件隐式建单。使用实际 Store/预留/授权快照/受控 HTTP/回执落库完整矩阵：有效 created、发送前禁用零 POST、缺项目映射零 POST、发送后 revision 变化 unknown 保留回执、传输未知不重发。原审计摘要不导出。

Jira/Linear 消费/渠道定向 race 8.779s、vet/diff 通过。尚需管理员 Jira 表单、Jira 查看链接前端适配、后台生命周期 Jira 专项及实际浏览器交互/必填字段与认证范围说明；不声称 Jira 功能完整验收。前一远端 CI 37602358756 待终态后再推送。

### S6 Jira 管理表单与查看链接（开发验证中）

P7/F4，既有凭据替换表单新增 Jira 账户邮箱、项目 ID、问题类型 ID（启用时必填），写入 username/jira_project_id/jira_issue_type_id；默认保留既有凭据，不回填秘密。公开状态显示 has_jira_mapping。表单说明直接站点 HTTPS 根地址、Cloud v3 与账户邮箱/API token；scoped-token 网关、Data Center、额外自定义必填字段暂未实现，需要无需额外必填字段的普通 issue 类型。保存不创建，用户从发现详情明确操作。

详情链接支持 Jira HTTPS /browse/<合法 key>，Store 查询先用固定 endpoint_origin 与 remote ID 再校验链接，避免损坏持久数据变成外站链接。数据库测试注入外站 url 并验证返回 URL 为空。Jira 回执/消费者定向 race 4.914s、前端 typecheck、vet/diff 通过。旧完整 Go 47530 仍运行，尚未执行本版本 build/全量/浏览器表单验收；避免 Vite 清理产物与 go:embed 并发。相关能力仍在开发验证阶段。

Jira 预留消费版本完整 Go 测试通过（platform 98.838s）。Jira 表单/链接前端 build 已顺序完成，vet/diff 通过。

### S6 Jira 实际 Runner 生命周期（开发验证中）

P9/F5，将已有真实 Start/Stop/Restart 测试改为 Linear/Jira 共享场景，复用同一生命周期验证逻辑，分别配置各自映射与受控回执。覆盖真实后台周期创建、Stop 连续调用及时结束、停止超过一个完整周期的新预留保持 pending、重启只处理新记录、再次重启不重复已有工单。两种 provider 定向 race 19.700s 通过，不访问真实远端。表单和 Jira 链接实际浏览器验收仍待执行，当前重建版本完整 Go 验证随后执行；先前 CI 9a80965 已 success，尚未追加推送。

### S6 Jira 配置浏览器与工单触发语义（开发验证中）

P9/F5，受控 localhost 页面加载真实编译产物，已有 Jira 默认不展开凭据；选择替换后邮箱/项目 ID/类型 ID 均为空，写入受控值保存。fixture 严格检查实际 PATCH JSON 的 username/token/jira_project_id/jira_issue_type_id，成功后版本 1→2、凭据输入清空、替换复选框关闭、映射存在标志保留，未发起真实外部请求。临时 tab 与 fixture server 已清理。

验收发现工单类型仍显示通知事件/频率/阈值，容易暗示自动建单。UI 对 Jira/Linear 改为展示“仅由发现详情明确创建”的说明，隐藏无效通知编辑项，保留既有存储字段以兼容配置验证。类型检查/diff 通过，新 UI 分支尚待产物重建与浏览器复验。完整 Go 12649 已实际进程核实仍 live，不因耗时重启。

工单配置与生命周期版本完整 Go 测试通过（platform 110.847s）。明确触发提示前端 build 与嵌入应用 go build 通过。实际受控浏览器复验 Jira 已有配置移除通知事件/频率/风险阈值，显示仅发现详情明确创建；新建 Linear 同样显示提示与团队输入；新建普通 Webhook 的原通知编辑项仍存在。临时 tab/server 已清理。提示改动只涉及 UI，不重复无关后端全量测试；后端最终全量及两种 provider 生命周期/race 已有证据，当前产物以应用 build 验证。待推送所有本地 Jira 提交并跟踪精确 HEAD CI，尚未完成完整需求审计/发布。

### S6 Jira 数据库重开与配置恢复说明（开发验证中）

P9/F5，将实际 Close/OpenStore 的持久恢复测试扩为 Linear/Jira 同场景，验证 created、带回执 unknown、过期 sending 三类状态，固定 endpoint_origin/记录 ID/内部唯一键在重开后保留，合法回执保留，过期发送转 unknown，旧确认拒绝，再次预留不创建新记录。两类 provider 定向 race 4.661s、vet/diff 通过。生产代码未变化，不重复前一已通过全量测试；design.md 增补实际配置、认证支持范围、角色、显式触发、202/pending 语义与 unknown 核对/不重发契约。当前 e6b29e5 的远端 CI 37603901544 仍 in_progress，等待其终态后推送测试和文档。完整需求验收仍有机器人回调/GitHub/诊断等缺口，不标记整体完成。

### S7 Slack 回调来源验证基础（开发验证中）

P8/F4，依据官方 https://docs.slack.dev/authentication/verifying-requests-from-slack/，对未经解析的原始字节校验 v0:<timestamp>:<body> HMAC-SHA256，hmac.Equal 常量时间比较，拒绝非数字/溢出时间、超过前后五分钟请求、非法版本/hex/长度、64 KiB 超限体及缺失 secret。函数只验证来源，不授予身份或权限，不消费 nonce，也不能阻止五分钟内相同签名重放；后续必须原子回放记录、主动用户绑定、当前完整项目权限重新校验。

定向 race 1.693s、vet/diff 通过，覆盖签名有效、秘密变化、用户 ID 篡改、等义 URL 编码但原始字节不同、过期/未来签名、时间溢出/签名版本/编码异常/超限体。无外部机器人消息发送，回调 HTTP 路由未开放。机器人完整 AC-013 仍未完成，不用签名 helper 代替身份绑定与防重放验收。当前 e6b29e5 CI 37603901544 尚 in_progress，未追加推送。

### S7 Slack 持久原子防重放（开发验证中）

P8/F4，迁移增加 callback receipts（integration_id + 原始 timestamp/body SHA256 摘要唯一，expires_at 索引），不保存回调正文、外部用户或签名秘密。内部 consumeSlackCallback 在同一事务读取当前启用 Slack 渠道/revision 的有界凭据、验证原始请求签名与时间，再原子 INSERT OR IGNORE；只有新记录提交成功才通过。先签名后清理过期摘要，过期请求本身始终拒绝；旧 revision/禁用渠道不通过。此 gate 只建立来源/一次性消费，用户绑定和项目授权仍需独立校验，尚无 HTTP 入口。

签名/持久 replay 定向 race 2.132s、vet/diff 通过：伪造正文拒绝且不污染合法回调，12 个并发消费者只有一个成功，实际 Close/OpenStore 后相同请求拒绝、数据库仅保存 64 字符摘要、过期/禁用拒绝。前一签名模块全量 Go 64265 仍运行，本次迁移版需其终态后执行新全量验证；不因为等待超时重启。AC-013 身份绑定/授权复审等仍未完成。

签名模块前一完整 Go 测试通过（platform 106.440s）。

### S7 用户主动一次性绑定挑战（开发验证中）

P8/F4，迁移增加按渠道/平台用户唯一的绑定挑战，只保存 SHA256 token_hash、渠道 revision 和十分钟过期时间。内部发行函数校验当前启用账号、当前启用 Slack 渠道/revision 与签名秘密配置，生成 24 字节随机 token 一次返回；重新发行原子替换旧挑战。函数不接受外部用户名，不增加项目成员/权限。尚无发行 HTTP/UI 或回调完成绑定入口，用户身份必须由已认证会话传入，后续消费仍需签名、持久 replay 与外部稳定身份域校验。

挑战/签名/replay 定向 race 2.711s、vet/diff 通过，验证数据库只保存摘要、再次发行不同 token且仅一条记录、版本变化/停用用户拒绝且不返回 token。新的 replay+challenge 迁移版本完整 Go 随后验证。AC-013 仍未完成，当前 e6b29e5 CI 37603901544 未确认终态，未推送覆盖。

### S7 Slack 签名绑定完成事务（开发验证中）

P8/F4，回调 gate 提取同事务 helper，使签名/持久 replay、挑战校验、绑定记录写入、挑战删除同一事务完成。外部身份为 integration/workspace T-ID/user U/W-ID；只接受单值 team_id/user_id/text 与 bind <48hex token>，不使用昵称，拒绝重复关键字段。挑战必须当前 revision、未过期、平台账号启用。绑定表约束同工作区外部身份和平台账号均唯一，INSERT 不覆盖已有身份。不添加项目权限，不执行复审，不公开 HTTP。

Slack 全部定向 race 3.217s、vet/diff 通过，验证有效绑定返回正确平台账号、挑战被消费、回调重放拒绝、伪造签名不污染合法绑定、新挑战不能覆盖既有外部身份、停用账号不能在其他工作区绑定。迁移与新完成流程完整验证仍待执行；前一全量 20208 同进程运行中。还需配置 Slack app/workspace 域、撤销绑定、用户认证发行 HTTP/UI、回调授权复审与错误恢复证据，AC-013 仍未完成。

### S7 Slack 应用/工作区身份域（开发验证中）

P8/F4，依据官方 https://docs.slack.dev/interactivity/implementing-slash-commands/ 的 api_app_id/team_id/user_id 契约，服务端凭据增加可选 slack_app_id/slack_workspace_id。旧出站通知配置仍可读，只有完整合法 A-ID/T-ID 与 signing secret 的渠道可发行绑定挑战。签名通过后再解析原始 form，要求 api_app_id/team_id 单值并精确匹配当前配置；来源正确也不能跨工作区/应用消费 nonce。此模块当前处理 form slash-command 回调，不声称支持 JSON Events API 或所有 provider。

Slack 定向 race 3.293s、vet/diff 通过，新增同秘密有效签名但不同 app、不同 workspace、重复 team_id 均拒绝，合法请求仍原子消费一次。绑定/挑战样本更新为明确域配置。前一完成绑定版本完整 Go 39176 仍运行，当前域校验版本全量验证待其终态后启动。绑定发行/撤销 UI、HTTP callback 和完整 snapshot 授权复审仍待完成，尚未发布回调入口。

### S7 用户主动撤销 Slack 绑定（开发验证中）

P8/F4，新增内部撤销事务，依据已认证平台账号删除该渠道下自己的绑定与全部未用挑战，不接受外部用户名或其他用户参数。渠道停用或重配置后仍可解除；停用账号拒绝调用；重复撤销返回未变化。删除挑战避免旧 token 在撤销后重新绑定。尚无撤销 HTTP/UI，此内部函数不能替代端到端身份认证证据。

Slack 定向 race 3.596s 通过，覆盖其他账号不能删除既有绑定、渠道停用后所有者仍可撤销、绑定与挑战共同清空、重复撤销幂等。完整 Go 与 vet 正在执行。此前 e6b29e5 的 CI 37603901544 已确认 completed/success；后续本地 Slack 切片未推送、未据此声明远端已验证。AC-013 仍缺认证发行/撤销入口、实际回调与项目快照权限授权。

### S7 自身绑定认证接口（开发验证中）

P8/F4，认证路由新增 POST /api/v1/bot-bindings/:id/challenge 与 DELETE /api/v1/bot-bindings/:id，沿用真实 session guard 和 origin 检查。挑战请求体限制 4KiB，必须提供当前渠道 revision；身份仅从 currentUser 取得。响应一次性返回 token/expires_at 且 no-store，不返回渠道 secret；删除接口只撤销当前登录用户的绑定/挑战，重复请求返回 revoked=false。发行不增加项目权限；当前仍没有公开 Slack callback 或操作授权入口。

真实 HTTP.Register+Store.Login 定向 race 与全部 Slack 测试 7.430s 通过，覆盖匿名 401、跨站发行/删除 403、无 revision 400、有效发行 201、请求 user_id 不能覆盖 session 身份、另一用户删除不影响 owner 挑战、重复撤销幂等。vet 与 diff 检查通过。此前 c6de0b7 完整 Go 61475 已 completed/success（platform 112.869s）；新增认证接口尚需自身全量验证，不能沿用前版结果。配置/自身绑定界面、公开签名回调和 snapshot 权限授权仍未完成。

### S7 Slack 签名绑定 HTTP 回调（开发验证中）

P8/F4，公开 POST /api/v1/bot-callbacks/slack/:id/:revision/bind，仅处理 application/x-www-form-urlencoded 与最多 64KiB 原始字节。绕开浏览器 session guard，使用 Slack timestamp/signature 和当前配置 app/workspace、revision、一用挑战及持久 replay 完成同事务绑定。认证失败统一 403，不公开账号、挑战或渠道是否存在；成功只返回 ephemeral 静态身份绑定提示，无审计内容、秘密或平台身份 ID。不增加项目权限、不执行复审或合并操作。渠道配置/rotation 后 callback URL 的 revision 必须同步更新；尚缺配置界面及操作回调完整权限检视。

真实 Register/SQLite/Login 的专项 race 与全部 Slack 测试 7.478s 通过：无需浏览器 session 的有效签名绑定成功、伪造签名 403、错误 media type/超大 body 400、持久重放 403、绑定对应 challenge 所属平台账号、成功 no-store/ephemeral。vet/diff 通过。认证接口前版完整测试 73886 仍运行，本回调新增版本需后续独立全量与远端 CI 证据；不据此前版测试声称本回调已全量验证。AC-013 仍未完成。

### S7 自身机器人绑定状态查询（开发验证中）

P8/F4，新增 GET /api/v1/bot-bindings 与 OwnBotBindings，只读事务内校验当前启用账号，按 session 用户筛选绑定，返回渠道 ID/name/enabled 与该用户自己的稳定外部身份/创建时间。停用渠道仍显示，以便撤销；不返回其他用户身份、渠道凭据、challenge token。最多 500 条，超过明确冲突而非静默截断；空结果 items=[]。

真实 HTTP/SQLite 专项 race 加全部 Slack 测试 7.677s 通过，覆盖匿名 401、其他账号空列表、停用渠道仍返回自己的绑定、无 secret 泄露、撤销后列表清空。vet/diff 通过。认证接口前版 14e7f9b 完整测试 73886 已 completed/success，platform 112.209s；当前回调加状态查询版本全量进程 27127 正在运行。本切片未推送，不声称远端验证完成，绑定状态 UI 和机器人操作权限仍待接入。

### S7 自身机器人绑定页面（开发验证中）

P8/F4，日常导航加入所有登录用户可见的「我的机器人绑定」。页面查询自身身份、明确停用渠道仍可撤销、删除同时撤销未用凭据、成功刷新服务端状态；加载时清空旧记录，失败显示错误与可刷新按钮，操作失败不静默重试。useEffect 清理防止离开页面后旧加载响应覆盖状态。无绑定创建表单/可用渠道选择，空状态明确说明绑定入口待完善，不声称机器人操作已经可用。

真实编译前端+本地受控 HTTP fixture 的浏览器验证：member 账号导航可见，停用 Controlled Slack 的 T1/U1 展示，实际 DELETE /bot-bindings/1 后成功提示和空列表可见。仅本地 fixture，无真实 Slack 操作。typecheck 与临时目录 Vite 构建通过；随后正式 npm run build 更新 embedded assets。后端 df527db 全量测试 27127 completed/success，platform 108.783s。移动布局、慢加载/失败/键盘回归、创建绑定流程及配置 UI 仍待补齐，AC-013 未完成。

### S7 可用绑定渠道与发行权限（开发验证中）

P8/F4，新增 GET /api/v1/bot-binding-channels，只返回启用 Slack、签名秘密/app/workspace 配置完整、且用户拥有任一启用 scope 项目 viewer 或更高权限的渠道 ID/revision/name/provider。管理员仍要求 scope 中存在启用项目。列表事务检查当前启用账号，凭据保持服务端，候选超过 500 明确冲突。挑战发行采用完全相同的 scope 权限谓词，不能通过猜渠道 ID 绕过列表；返回不可见 404。身份绑定本身不增加权限，真实审计操作仍须完整 snapshot 授权，尚待接入。

实际 HTTP/Register/Login/SQLite 专项 race 与全部 Slack 测试 6.288s 通过，验证无项目成员权限时列表为空且挑战发行 404、增加 viewer 后渠道可见、管理员可见配置完整渠道、响应不含签名 secret。vet/diff 通过。创建绑定界面仍待实施，AC-013 仍未完成；新切片全量验证在后续运行，不以旧版 CI 代替。

### S7 用户生成绑定凭据页面（开发验证中）

P8/F4，个人绑定页面同时加载自身绑定与可用授权渠道，明确手动选择渠道，提交 captured expected_revision，已绑定渠道不允许重复申请。一次性 token 仅 React 内存显示，无 URL/localStorage 持久化；刷新/换渠道/撤销/到期清除，重新发行替换旧凭据。显示 bind 参数、到期时间、勿分享说明和绑定完成后手动刷新；不猜测管理员实际 Slack 命令名称。异常响应 token/expiry 拒绝展示，发行失败清空渠道选择等待显式刷新，不自动重试。

typecheck 与临时独立目录 Vite build 成功。真实编译前端+受控本地 fixture 浏览器验证：默认空选择禁止发行、选择 Controlled Slack 后 POST 携带 expected_revision=7、有效 48hex token与到期提示显示、DELETE 撤销后令牌消失且选择回到空值。测试无实际 Slack 消息或真实凭据。当前 e5f88b3 后端全量 32125 仍运行，正式 embedded build 等该进程结束后执行；前一 e21a8a2 远端 CI 37606369057 仍 in_progress，不覆盖推送。创建 UI 的移动/失败/短期限回归、Slack 配置界面与操作权限仍待完成。

### S7 绑定创建页面错误与到期回归证据

ff33a4f 的真实编译前端在本地可切换受控 fixture 完成浏览器回归：POST challenge 返回 409 后渠道选择/生成/凭据区消失，只保留错误和可用刷新；显式刷新恢复空选择；随后签发四秒凭据，在到期前可见 bind 参数，到期后令牌区消失且显示重新生成提示。360x800 screenshot 检查标题、错误/到期提示、渠道 select 与生成按钮均在页面内，凭据未残留。结束恢复默认视口、关闭测试标签和 fixture，不操作真实 Slack 或生产身份。

本次验证未改动产品代码，只增加当前 UI 证据；尚未验证慢请求、所有键盘状态和实际 Slack 配置/回调联调。后端全量 32125 同一进程仍 live（go/platform.test 均在，未因观察超时重启）；精确 e21a8a2 CI 37606369057 仍 in_progress（Go tests and race checks）。暂不更新 embedded assets 或推送覆盖该 CI。完整需求保持未完成。

### S7 绑定完成时复查项目访问权限

P8/F4，Slack 完成绑定事务在 challenge 所属当前启用用户上复用 botChannelAccess，校验当前仍有任一启用 scope 项目权限。不能仅依赖发行时授权：用户成员权限被撤销或项目停用后，原签名/未过期挑战不能写入绑定。签名/replay、权限查询、绑定写入与挑战消费在同一事务；拒绝回滚全部副作用。不因此授权其他项目或审计操作。

全部 Slack 与真实认证 HTTP 专项 race 7.018s 通过，新测试覆盖发行后撤销成员权限拒绝且无绑定、恢复成员但停用项目仍拒绝、全部授权恢复后同一请求可完成（此前失败没有错误消费 nonce）。vet/diff 通过。前版 e5f88b3 全量 32125 completed/success，platform 202.977s；本回调新版本需独立全量。ff33a4f 创建页面的正式 npm run build 已成功更新 embedded assets index-CK-qagnj.js。远端精确 e21a8a2 CI 37606369057 仍 in_progress，本地后续提交尚未推送，不以该旧 CI证明本改动。AC-013 尚缺管理员域配置和机器人审计操作完整 ACL。

### S7 Slack 管理员域配置（开发验证中）

P8/F4，集成编辑增加可选 Slack 应用 A-ID/工作区 T-ID；填写其一要求完整双方与 signing secret，通知-only 旧配置保持可读。完整替换请求包含 slack_app_id/slack_workspace_id，选中保存后清空字段，保持既有凭据不重填也不丢失配置。页面显示保存渠道实际 ID/revision 的回调路径，明确需 HTTPS 站点根地址及版本更新后同步 Slack URL。目前回调仅身份绑定，不声称操作审计可用。

公开 Integration 增加 has_slack_binding 布尔元数据，Save 与列表 Scan 均依据完整合法配置生成，不回显 app/workspace/secret。专项 race 6.977s 加已有全部 Slack/HTTP 测试通过，新真实数据库测试验证元数据保存/读取、普通编辑保留域与secret、JSON 不含 workspace/secret。typecheck、vet/diff 通过。新表单的实际浏览器填表/保存与移动端验证仍待执行，正式构建尚未更新；不得以 typecheck 宣称界面验收完成。c0f1c8a 全量 11229 仍运行；精确 e21a8a2 CI 37606369057 已 completed/success。待当前本地验证完再推送新版本并检查其独立 CI。

### S7 Slack 管理员配置浏览器验收证据

ad9252e 正式 npm run build 成功，embedded asset 更新 index-D8QUL9Ut.js。真实编译前端+本地受控配置 HTTP fixture 浏览器验证：管理员选择已存 Slack 默认保留凭据；显式完整替换后实际填写 HTTPS、fixture signing secret、A123/T123，PATCH 服务端断言具体 JSON 成功。保存后 revision 1→2，回调路径 /api/v1/bot-callbacks/slack/1/2/bind 可见，替换开关复位；重新打开替换区域 HTTPS/secret/app/workspace 均空，只有已配置布尔元数据。不发送通知，不访问真实 Slack，无生产 secret。

上一版 c0f1c8a 全量 11229 已 completed/success，platform 110.763s。当前含 Slack 配置元数据的全量将单独执行；旧 e21a8a2 CI 37606369057 success 不替代本地后续代码远端验证。尚待当前 full/race/CI、配置移动/错误恢复、实际签名绑定联调与审计操作权限，AC-013 不宣称完成。

### S7 签名机器人状态读取授权（开发验证中）

P8/F4，新增内部 slackRunStatus，签名/app/workspace/revision/replay gate 后，仅解析单值稳定 user_id 与 status <runID>，匹配当前绑定及启用平台账号。同事务加载有界 policy JSON 与固定 HEAD，复用完整 requireSnapshotRole viewer（目标/来源/context）、所有项目启用状态以及当前渠道必须包含目标项目。仅返回任务 ID/HEAD/status，无发现、源码或 trace。授权与 replay 消费同事务，不允许身份绑定直接绕过项目 ACL。尚未公开 status HTTP 命令或增加复审动作。

SQLite fixture 定向 race 13.259s 通过：合法 bound actor 返回 pending 与固定 HEAD，重复请求拒绝，目标权限撤销、context 权限撤销、账号停用、context 项目停用、渠道范围移出目标项目、绑定撤销均无输出。初测发现新函数误用 queued，按实际 platform_runs.pending 修正后重跑通过，不改现有状态契约。来源仓库独立拒绝矩阵仍待新增，不能以目标/context 案例宣称该矩阵已验证。

前版495a8b1（Slack配置与正式前端）全量51586已completed/success，platform118.341s；本读取函数尚需完整全量与公开接口证据。精确e21a8a2远端CI已success，后续本地提交尚未推送。AC-013保持未完成，完整机器人操作与其他provider未覆盖。

### S7 统一 Slack 命令回调与来源仓库矩阵

P8/F4，新增 /api/v1/bot-callbacks/slack/:id/:revision/command，保持旧 /bind 只绑定。新路径读取有界原始 form，分派 bind 或 status <任务ID>，每条实际操作仍独立执行完整签名/replay/domain gate，不以未验证解析结果授权。status 返回 ephemeral 静态格式 task ID/status/HEAD，不携带发现、源码、trace或用户资料。管理员配置说明改为统一 command 路径与 status 使用方式；没有新增复审/合并写操作。

状态授权测试改为独立 target1/source3/context2，新增 source 权限撤销和 source 项目停用拒绝，合法测试保留全部访问。该矩阵 race18.039s通过。实际 Register/SQLite 测试追加无session签名status成功、伪签名403、HTTP重放403、ephemeral输出；与完整来源/context矩阵race16.039s通过，vet/diff通过。前端说明typecheck待当前进程结果，正式embedded assets尚需顺序更新。

旧6e04c85完整测试28677仍运行，不能据此证明本HTTP新增版本全量通过；新版本全量/远端CI、命令联调、其他机器人providers和复审授权仍待完成。AC-013未完成。

### S7 状态命令真实 HTTP 授权矩阵

27ab672 后的测试扩展将每个独立状态授权 fixture 通过实际 HTTP.Register 的 /command 路由执行有有效 Slack 签名的请求，不含浏览器 session。target1/source3/context2 三域权限分开：目标/来源/context 成员撤销、source/context 项目停用、平台账号停用、渠道 scope 变更、绑定撤销均返回相同 403 {"error":"callback rejected"}，无任务 ID、HEAD 或状态泄露。合法完整授权返回固定 HEAD 和 ephemeral 状态。内部成功与 HTTP 成功使用不同 timestamp，避免测试误命中内部消费 nonce。

定向 race14.960s、vet/diff通过。本次仅扩展测试，不改变生产行为；完整测试31572仍同进程运行，未将观察超时当失败。当前已确认远端CI仍为旧e21a8a2 success，新命令版本待本地全量终态后推送并验证独立CI。有效证据链接与复审操作、机器人provider范围及全部REQ仍待完成，状态查询不替代AC-013。

### S7 Slack 状态命令证据导航（开发验证中）

P8/F4，已授权 status 响应从 Settings.PublicURL 构造固定 /#/runs/<id> 证据页面，只采用服务端站点地址，不读取 callback response_url/仓库URL，不包含source/trace/匿名访问token。明确平台登录要求并设置 unfurl_links/media=false。未配置或不合法站点地址时省略链接，不影响纯状态；拒绝凭据userinfo/query/fragment/markup控制符。页面继续执行现有session与完整snapshot ACL。

测试增加URL拒绝矩阵及实际 SettingsService+HTTP.Register 合法状态回调链接验证。初始fixture错误使用带路径public_url，被真实配置校验拒绝；改为符合现有契约的HTTPS origin，不弱化配置规则，race重跑51710待终态。前一27ab672全量31572已success，platform121.234s。当前新增链接生产代码需自身全量与精确远端CI，仍未推送。该导航不替代复审交互或其他providers，AC-013未完成。

51710终态success，race22.951s，包含实际配置origin回调输出正确证据链接、关闭unfurl以及所有HTTP授权拒绝矩阵。上一vet已通过，diff通过；新增链接版本全量继续执行。

### S7 机器人绑定操作审计记录

P8/F4，挑战发行、有效绑定完成、实际撤销分别在同事务写入 platform_events，actor 为已认证/挑战所属平台用户，target 仅渠道 ID，action 为 bot.binding.challenge_issued/created/revoked。不保存外部用户、token、签名、secret 或 body。重复撤销无变更不新增历史；绑定 replay/伪造/冲突事务拒绝不新增成功记录。日志写入失败回滚绑定/挑战操作，避免状态已变却缺历史。

真实 SQLite 专项 race10.545s通过：发行两次/绑定一次/撤销一次的 actor+target 历史计数准确，重复撤销不增加，令牌不出现在target。受控 SQLite BEFORE INSERT trigger 强制审计写失败后撤销返回失败、挑战仍存在，验证同事务回滚。此前无故障注入版本专项15.206s也通过；vet/diff通过。新操作审计版本需全量，前版证据链接全量62154仍同进程运行，未假定已成功。

该历史补齐不替代机器人复审动作、其他providers、跨平台GitHub等完整需求；AC-013和整体目标仍未完成。后续推送依据本地全量与精确远端CI结果。

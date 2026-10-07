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

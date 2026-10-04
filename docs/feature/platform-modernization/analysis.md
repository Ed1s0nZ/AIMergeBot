# AIMergeBot 现状与优化分析

日期：2026-10-04。范围：当前本地源码静态检查和 Go 基线检查。没有运行真实 GitLab、模型审计、Webhook 或评论；以下不代表线上故障已复现。

## 判断

项目具备 GitLab MR 获取、模型调用、工具读取、结果展示的原型闭环，但访问控制、提交一致性、任务生命周期和结果证据不足。直接替换 UI 或 Agent 框架不会解决这些问题。应先建立可靠审计内核，再以 React 提供完整用户工作流。

## 核心发现

|问题|源码证据|影响与改进方向|
|---|---|---|
|工具上下文与待审计提交不一致|internal/gitlab_mcp.go:730、846、957 等固定 Ref=main|MR 修改在功能分支，工具却读主分支，可能误报/漏报；所有读取绑定审计快照的 head/base SHA，禁止静默读默认分支|
|MR 更新被跳过|internal/handler.go:481–486、internal/gitlab.go:201–205；storage.go 唯一键仅 project_id/mr_iid|完成一次后后续提交跳过；以项目、MR、head SHA、策略版本标识任务；强制重跑保留独立尝试|
|任务并发及失败处理不足|handler.go 先查询再写 processing；reanalyze_mr 启动独立 goroutine；多个错误路径直接返回|缺乏原子认领和有界并发，可能重复审计或一直 processing；统一队列入口、租约、失败状态、取消、恢复|
|分析和人工复核状态互相干扰|storage.go:SetAnalyzedStatus 使用 INSERT OR REPLACE，未包含 review_status|重跑替换原记录使人工复核重置；分离执行状态与复核记录，保留操作者和时间|
|解析失败制造发现|react_auditor.go:245 附近解析失败调用 extractSecurityIssues；1442 起依据关键词创建 medium 问题，没关键词也创建“已通过”的 low 问题|否定句也可能成为漏洞；格式错误不能产生通过或发现。严格 schema 校验，明确 incomplete/failed，证据不全的候选单独展示|
|步骤耗尽仍返回成功|react_auditor.go:146–311 循环退出直接 return result,nil|没有最终回答也可能作为成功并保存空结果；耗尽预算应明确 incomplete|
|结果筛选失效|handler.go:GetResultsHandler 注释 project_id 解析，level/type 仅用于排除空结果；filterResults 未调用|用户看到的项目和风险筛选不准确；数据库分页和统一过滤语义，列表统计口径明确|
|审计入口重复|handler.go 重跑/Webhook 和 gitlab.go 轮询分别编排 AI、保存等逻辑|行为漂移和修复遗漏；触发器只负责校验并入队，单一执行服务|
|访问控制缺失|main.go、handler.go 全部业务路由无认证；Webhook 无 token 验证及配置项目检查|结果和操作暴露；业务认证/权限与 Webhook 独立凭证，配置范围校验，限制请求体|
|持久化完整性不足|storage.go 初始化 Exec 忽略错误，GetAllResults 全表 JSON 读取并忽略坏记录；调用处先 done 后 AddResult 且忽略存储错误|可能显示已完成但无结果；迁移版本、事务、结构化字段和索引、明确错误、保存成功后完成|
|调用无法整体取消|react_auditor.go/openai.go 使用 context.Background；重试直接 Sleep|缺少任务级超时和取消传播；模型、仓库工具、重试等待共享任务 context|
|UI 工程与内容边界薄弱|web/index.html 1228 行，CSS/渲染/请求混合；969 附近 mr_url 直接放入 href|难以测试维护，URL 插值缺少独立校验；React 页面与组件分层、URL 协议检查、状态与错误处理|
|测试证据缺失|仅 cmd/test_tools/main.go，无 *_test.go|无法证明修改保持行为或提高审计质量；先补提交/解析/幂等/状态回归案例和模型评估集|

## Eino 适用性

官方 ReAct 组件以 ChatModel 和 Tools 图编排，要求模型具有 tool calling 能力，支持最大步骤配置；适合替代当前文本式 Thought/Action JSON 解析循环。官方概述也提供 ADK 和图编排。来源：

- https://www.cloudwego.io/zh/docs/eino/core_modules/flow_integration_components/react_agent_manual/
- https://www.cloudwego.io/docs/eino/overview/

建议作为待确认的迁移方向：由确定性流程控制快照、diff、预算、结果校验和存储，Eino Agent 负责有界上下文调查。工具限于读取绑定仓库与 SHA；评论、写仓库等行为由业务权限和策略控制，不能交给模型自行决定。

Eino 本身不提供漏洞真实性保证。需要分别验证模型 tool calling 兼容性、Eino/扩展版本与 Go 版本要求、超时与回调记录、结果 schema。当前尚未选定或安装版本，不能据此承诺升级后准确率。

## Git 审计应优化什么

1. 快照：记录项目、来源、MR、base/head SHA、配置与模型标识，处理 fork MR 的源项目。
2. diff：保留 old/new 路径、增删改/重命名、hunk 行号、二进制/截断标记；大型变更分片，显式记录跳过范围。
3. 工具：少量通用读取、目录、搜索、上下文接口；固定 ref、缓存、分页、有界输出。字符串匹配仅生成候选，不冒充数据流或漏洞验证。
4. 调查：围绕修改行定位调用者、输入来源、危险操作和已有防护；将仓库内容作为数据，防止其中的指令改变审计行为。
5. 发现：每项含文件、行号、SHA、证据、触发条件、影响、建议和置信度；校验引用存在、去重，区分已确认候选与覆盖不足。
6. 反馈：用户标记误报/接受/修复并保留原因；固定案例比较误报、漏报、定位准确性、耗时、token 与失败率。

## 建议交付顺序（待确认）

1. 审计正确性与可靠性基础：统一入口、快照一致性、严格结果、执行/复核状态、基线回归。
2. 产品闭环：登录与权限、React 工作台、项目管理、任务列表/详情、发现复核，API 契约。
3. Eino 迁移：工具抽象、模型兼容、有界编排、可观察性与对比评估。
4. 交付：旧 SQLite/配置迁移、构建部署、文档与端到端验证。

此顺序是建议，不是用户已确认的优先级。GitHub、本地仓库、多租户、SSO 等扩展需要明确范围后再设计。

## 基线验证

- `go test ./...`：失败。cmd/test_tools/main.go:48 对 *internal.ReActAuditor 做类型断言，但该变量不是 interface，工具命令不能编译。
- 主模块与 internal 输出 `[no test files]`；可编译不代表审计行为正确。
- `git diff --check`：通过；本轮仅文档变更。
- F0 文档提交 a1263bb，功能分支已推送 origin。需求为 Pending Confirmation，F1 尚未完成。

# AIMergeBot

团队自托管的 GitLab 代码安全审计工作台。React 前端、登录与管理员权限、SQLite 任务历史、Eino 工具调用 Agent，围绕具体 Git 提交收集证据并支持人工复核。

## 功能

- 登录/退出、管理员创建账号、禁用、角色变更、密码重置与会话撤销。
- 项目管理、手动审计、GitLab Webhook、分页轮询与提交级去重。
- 有界 worker、任务超时/取消、服务重启中断标记、独立重审记录。
- 固定 diff 版本 ID 和 base/head SHA；支持 fork MR、文件重命名、真实 diff 新增/删除行定位。
- Eino 标准 tool calling：`read_file`、`list_files`、`search_code`，有缓存与输出预算。
- 严格结果校验、提交代码证据匹配、候选标记、覆盖不足说明、工具与模型调用/token 记录。
- 项目/状态/等级/类型/复核过滤，发现复核及操作日志。
- 管理员系统设置：模型、GitLab、Webhook、审计策略保存到工作目录 `config.yaml`。
- React 静态资源嵌入 Go 二进制；旧 SQLite 结果导入并明确标记缺失提交证据。

## 快速开始

需要 Go（模块最低声明 1.21，当前验证环境为 1.27.1）、CGO/C 编译器；改动前端需要 Node.js 20.19+ / npm。SQLite 驱动需要 `CGO_ENABLED=1`。

```bash
# 安装并构建前端；产物进入 web/dist 并嵌入 Go 二进制
cd frontend
npm ci
npm run build
cd ..

go test ./...
go build -o aimangebot .

# 首次空数据库启动必须配置管理员，密码至少 12 字节、最多 72 字节
export AIM_ADMIN_USERNAME=admin
export AIM_ADMIN_PASSWORD='请替换为自己的强密码'
./aimangebot
```

打开 [http://localhost:8080](http://localhost:8080)。首次缺少 `config.yaml` 时，程序从 `config.example.yaml` 复制生成权限为 `0600` 的配置文件。已有 `config.yaml` 不覆盖；该文件和数据库不纳入 Git。初始账号创建后可移除引导环境变量，不会每次重置密码。

登录后进入 **系统设置**，填写模型 API 地址、模型名称/API Key、GitLab 实例/Token。模型须支持 OpenAI 兼容 tool calling；保存后写入本地 `config.yaml`，秘密字段留空保留原值。新审计采用保存后的模型/仓库设置；监听地址与 worker 数变化需要重启。直接手改文件需重启加载，不提供双向文件监控覆盖界面修改。

在 **项目** 添加 GitLab 数字 ID 和展示名称，再到 **审计任务** 填写 MR 编号。项目配置（含启用状态）也同步到 `config.yaml`；写文件失败会提示重试保存。项目 ID 是否可访问在发起审计时由 GitLab 验证。所有团队成员可读团队项目、执行审计及复核；只有管理员可以管理项目、用户、设置和查看操作日志。当前为单团队，不提供项目级成员隔离或多租户。

## 自动触发与评论

默认关闭轮询、Webhook 和评论。在系统设置中明确启用：

- Webhook URL：`https://你的服务/webhook`；事件选择 Merge request，Secret Token 与系统设置的 Webhook Token 一致。未签名和未配置项目的请求不会入队。
- 轮询约每 30 秒执行，遍历 opened MR 的分页。`scan_existing_mrs=false` 时第一次建立已见 SHA 基线，随后新 MR/新 SHA 执行审计；不制造“已完成”的基线任务。
- 相同项目/MR/head/审计策略重复事件去重；运行中的强制重审不创建副本，终态强制重审创建独立尝试。
- 评论只对成功、覆盖完整且当前 MR head 仍匹配的结果发送。发送失败记录 `comment.unknown`，避免网络结果不确定时自动重复发送。结果不会因评论失败丢失。

使用 TLS 反向代理时，在系统设置填写公开访问地址 `public_url: https://audit.example.com`，用于写请求 Origin 校验及 Secure Cookie。程序不信任任意 `X-Forwarded-*` 请求头。不要直接把 HTTP 服务开放到公网；生产代理应强制 HTTPS。单实例部署，数据库持久化磁盘应支持 SQLite WAL。

## 审计状态与可信度

`pending → running → succeeded / failed / incomplete / cancelled`。

- `succeeded` 表示流程和已请求的证据校验完成，不保证代码不存在漏洞。
- 模型 JSON 无效、文件/行/证据不匹配会失败，不通过关键词兜底制造问题。
- GitLab diff 状态/数量受限或 collapsed/too_large、被排除或预算外文件、工具失败/部分输出、任务超时标记 `incomplete`，显示覆盖原因。
- 文件读取最大 256 KiB；单次工具最多 200 行/16 KiB，目录最多 20 页，工具最多 40 次、每任务文件缓存最多 4 MiB，diff 最多 96 KiB。搜索按页最多读 20 文件；返回 `more` 为覆盖受限。
- 发现定位此次 diff 的 head 新增行或 base 删除行；删除防护逻辑也可报告风险，详情明确标识 HEAD/BASE，并验证对应提交的证据。候选 `candidate` 与证据支持 `supported` 都需要人工判断可利用性。
- 依赖清单和关键词不被默认判为漏洞；当前没有 CVE 数据库集成。
- 模型 token 数依赖兼容接口返回 usage；未返回时不能当作零成本。工具详情里 `model` 条目记录返回的 token 数。

## 结构与 API

```text
frontend/src/          React 页面、共享组件、请求与样式
web/dist/              可重复生成的嵌入静态产物
web/ui.go              SPA 静态资源与 API 404 边界
internal/platform/     auth / settings / store / runner / GitLab / Eino / HTTP
internal/*.go          旧版本兼容类型、工具与对比基线；新服务不注册旧业务路由
config.example.yaml    无真实密钥的默认配置
main.go                启动、资源装配、关闭
```

业务 API `/api/v1`；会话由 HttpOnly Cookie 认证；`/auth/login` 公开，`/auth/me` 和 `/auth/logout` 管理会话。详细契约见 [design.md](docs/feature/platform-modernization/design.md)。原 `/results`、`/mr_status`、`/reanalyze_mr` 等端点不再注册，旧 HTML 不再作为服务 UI。旧 ReAct/MCP 说明属于旧版本参考，当前运行采用 Eino。

## 升级与回退

1. 停止旧服务，备份 `pr_agent.db`（WAL 模式先完成 checkpoint/备份）和 `config.yaml`。
2. 构建新版本，设置首次管理员凭证，使用原工作目录启动。
3. 新表使用 `platform_` 前缀；原 `results` 等表保留。旧结果导入一次，标记 `incomplete`，不伪造 SHA/行号。损坏 JSON 会明确报错，修复或从备份恢复后重试。
4. 校验项目与历史，配置模型，发起一条审计后再启用自动触发。
5. 回退停止新版本并恢复旧二进制/数据库备份。新运行、账号和复核不会自动降级到旧表。

## 开发与验证

```bash
go test ./...
go test -race ./internal/platform
go vet ./...
cd frontend
npm run typecheck
npm run build
npm run dev
```

Vite 开发代理默认转发到 `localhost:8080`。提交前前端构建更新 `web/dist`，确保 Go 嵌入最新 UI。模块版本和前端依赖均有锁文件。

验证包含：会话/权限/跨站写入拒绝、提交去重/重审/并发、取消/超时/恢复、fork refs、diff 行号与证据、旧数据导入、Eino 实际工具调用、旧/新结果逻辑对比，以及本地浏览器登录/设置保存/复核/移动布局。详见 [验证记录](docs/feature/platform-modernization/verification.md)。固定模型响应测试证明流程行为，不证明真实模型准确率；真实私有 GitLab、模型精度和生产吞吐仍需部署样例验证。

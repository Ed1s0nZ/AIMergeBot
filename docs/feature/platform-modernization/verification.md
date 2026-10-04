# F5 验证记录

2026-10-04。实施提交 317761e。macOS Go 1.27.1，前端 Node/npm；Docker Linux Go 1.27 + Node 24。使用临时 SQLite、HTTP fake GitLab/模型和隔离 smoke 工作目录，无真实 MR 评论或外部模型调用。

## 检查结果

|检查|结果|
|---|---|
|go test ./...|PASS，17 个 platform 测试；原工具示例可编译|
|go test -race ./internal/platform|PASS，包含并发入队/领取、worker/取消/超时|
|go vet ./...|PASS|
|npm run build|PASS，严格 tsc + Vite；1590 模块；JS 256.61 kB、CSS 11.65 kB（压缩前）|
|git diff --check|PASS|
|docker build -t aimangebot:platform-review .|PASS，镜像内 go test ./... 与 CGO 编译通过|
|Docker 非 root 运行|PASS，UI 200、首次管理员登录 200、已登录设置 200；config.yaml 为 600/UID10001|
|浏览器本地 smoke|PASS，登录、工作台、设置保存并落盘、任务详情、复核持久化、390×844 窄屏；最终预览见 preview.png|

Docker 首次指定的 18082 端口被本地代理占用，切换到 Docker 自动分配端口后成功；未修改占用程序。Docker smoke 的第一条断言误以为脱敏会删除键，实际契约返回空字符串；按空值契约验证通过。Sonic 在宿主 Go 1.27 显示回退 encoding/json 警告，不影响上述测试。

## 验收追踪

|验收|证据与边界|
|---|---|
|AC-001/002|TestAuthLifecycle、TestHTTPAuthorizationAndSession：会话哈希、退出/禁用/改角色撤销、服务端角色、Origin拒绝；浏览器登录与页面|
|AC-003/006|TestRunStateAndReviewIsolation、TestConcurrentEnqueueSingleClaim、TestRunnerBoundedCancellationAndTimeout：12并发只入队/领取一次、新SHA、独立重审、取消、超时、重启恢复|
|AC-004/007|TestDiffAndEvidence、TestGitLabForkSnapshotAndMismatch、TestPinnedDiffVersionPaginationAndCoverage、TestRemovedGuardBaseEvidence：fork refs、版本分页/不匹配拒绝、截断说明、新增head/删除base证据|
|AC-005|TestStrictResultDoesNotInventFindings、Eino/runner测试：无效JSON与行证据拒绝、部分覆盖/工具错误不报通过、超时明确状态|
|AC-008/009|TestAuditHTTPWorkflowAndFilters、TestRunStateAndReviewIsolation：HTTP登录到运行/复核/筛选/重审，复核归属、理由/操作者、历史保留；浏览器保存复核|
|AC-010|TestEinoUsesToolsAndValidatesEvidence、TestLegacyCleanResponseComparison；真实Eino工具循环通过fake OpenAI协议，非真实模型质量评估|
|AC-011|TestLegacyImportIdempotent、HTTP webhook拒绝；旧表保留，默认关闭评论；真实评论未执行|
|AC-012|TestSettingsPersistenceAndRedaction、TestProjectConfigurationSync、TestSettingsHTTPWithTLSProxyOrigin：首次复制、原子写、权限、脱敏/空值保留、拒绝无效配置、配置快照与项目同步；浏览器模型修改与文件一致|

## 固定案例的改造对照

执行：`go test ./internal/platform -run TestLegacyCleanResponseComparison -v -count=1`。

同一“无证据支持问题”的预设响应：旧逻辑产生 1 条低风险误报，新 Eino 严格结果为 0 条。该次本地耗时旧 2.931833 ms、新 1.931417 ms；fake usage 为输入100/输出20，共120，回调正确记录。此数字仅证明响应解析、调用与usage归档，不是实际API耗时或费用。

该干净样例只有1例，两条固定响应执行失败率均0；清洁样例误报旧1/1、新0/1。漏报率、真实漏洞召回、真实位置准确率、真实token消耗/费用及生产吞吐未测；正向/反向证据单元用例是规则回归，不构成完整质量基准。没有设置或宣称基于真实模型的准确率提升阈值。

## 交付限制

本轮交付单团队、单实例 GitLab MR 工作台；未扩展 GitHub/本地仓库、多租户、分布式队列或 CVE 数据库。生产上线需按 README 备份旧数据库、引导管理员、配置支持工具调用的真实模型/GitLab，再验证代表性 MR。文件/工具/搜索预算的覆盖限制在结果中可见。旧 MR 级复核保存在原表；可定位的发现级复核导入，不臆造关联。

界面预览中的项目/候选为本地 smoke 数据，不是对真实仓库的安全结论。

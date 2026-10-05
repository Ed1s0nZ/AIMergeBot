# 最终验收审计（已交付）

当前结论：已完成本轮目标并交付。代码已推送 main（18a4c20），同一干净提交构建的二进制已运行于1234。健康、登录、设置保存、密钥脱敏与保留、原账号/项目、数据库完整性及实际部署页面均已核查。真实模型小型语料完成逐例人工判定；覆盖不足与静态证据限制仍明确保留。

| 原始要求/不变量 | 当前实现与可核查证据 | 当前判断 |
| --- | --- | --- |
| React产品化、登录、成员、项目、任务、复核、设置、日志 | frontend/src；platform auth/routes；platform-modernization/release-readiness；浏览器分阶段截图 | 已实现；生产设置页面实查通过，319px无横向溢出，保存按钮static；桌面/390px分阶段验证通过 |
| 设置同步项目config.yaml、首次生成、密钥保留/脱敏 | settings、revision、project sync outbox；设置冲突/同步故障/启动恢复测试 | 已验证；保存HTTP200，revision推进、密钥保留/脱敏，config权限0600 |
| Eino语言无关Git工具，不执行仓库代码 | GitRepository、agent tools/investigation；只读固定SHA；目录/检索/批量/历史/元数据共14工具 | 已实现；不提供任意跨仓库权限扩展或运行利用证明 |
| 固定身份与有效配置、base/head/fork、去重 | AuditPolicy、identity migration、enqueue；当前policyv12 | 已实现；旧策略需重新提交，生产迁移完整性ok，原账号/项目保留 |
| 删除保护、源码锚点、调查/反证/观察来源 | ValidateFindings、canonical registry、observation links；伪造/错误锚点/删除BASE测试 | 已实现；源码支持不等于语义或利用性证明 |
| 故障/取消保留发现与检查点 | frozen supplemental result、worker fences、恢复脚本真实SIGKILL | 已验证分阶段原生进程，不以单元测试替代崩溃证据 |
| 有界重试、Retry-After、租约/所有权、不能重复执行 | retry store/transport/worker instance；429/5xx/租约失效/迟到写测试与native proof | 已实现；永久模型错误脱敏已通过实际Eino回归及完整Go/race检查 |
| 权限与fork来源、并发/排队/每日配额、代理登录限流 | project_access、quotas、login_throttle、trusted_proxies；API与浏览器证明 | 已实现；支持单实例SQLite部署，非分布式限流 |
| 文件策略排除与覆盖不足区分、分页连续性 | BuildDiff、excluded fields、paging coverage；全排除skipped测试 | 已实现；预算省略不能作已审计/安全结论 |
| 元数据、symlink/gitlink/LFS/binary边界 | typed Git metadata、canonical anchor、API budgets；real Git fixtures | 已实现；不读取外部子模块/LFS内容 |
| 轻量列表及索引，不读取完整trace/result | finding projection、RunListItem、transactional list；多MiB结果/EXPLAIN/API测试 | 已验证；首次历史回填成本明确 |
| 问题时序图解释风险、静态/推测说明与证据 | sequence validation/context/checkpoints，metadata note-only；native/browser | 已实现；不是实际运行轨迹 |
| 复核与GitLab同一评论同步，未知发送不盲目重发 | durable delivery generations/IDs/marker/hash；lost ack/create/update/人工改动测试 | 已实现；生产评论保持关闭，未对真实GitLab发评论 |
| 独立上下文复核与反证，不自动升级/删除 | fresh verifier stage/observations、strict verdict；actual Eino synthetic proof | 已实现；同模型上下文不等于模型多样性或运行复现 |
| 大PR分组、跨组调查、共享预算及可见缺失 | AuditPlan/AuditGroups/read-only synthesis；25文件native2组/browser | 已实现；最大8组/96KiB，超过明确覆盖不足 |
| 稳定问题生命周期与人工历史 | exact fingerprint、ambiguity clearing、occurrence/history migration；3HEADnative/browser | 已实现；无模糊语义匹配、无自动继承/自动修复 |
| 真实模型评测与风险质量，不用fixture数字冒充 | evaluation corpus/CLI/probe；初轮401失败保留；第二/三轮真实结果与逐例归因已记录，第四轮v12已完成11例与人工判定 | 本轮作者语料达到预期；不代表生产准确率或完整覆盖 |
| 文档/变更说明/回滚方案 | implementation逐阶段记录、README；本审计 | 已提交阶段记录、评测与交付/回滚说明 |
| 最终main提交推送 | main18a4c20已推送且远程SHA一致 | 已完成；后续交付记录为仅文档提交 |
| 1234部署与生产数据保留 | 干净main18a4c20构建；私有一致备份；本机API/UI证明 | 已部署；健康/登录/保存200，原用户与项目相同，数据库完整性ok |

已执行交付步骤：完整工作区/远程分支核对；有效模型整套评测与人工判定（保留失败轮次）；最终Go/race/vet/frontend与迁移验证；记录实际范围、质量限制及CHANGELOG；一致备份DB与配置，合并并推送main确认SHA；构建同一main产物，正常停止旧服务后替换1234；确认健康、登录、设置脱敏/配置同步、任务详情历史/证据、权限与原数据，保留可回退旧二进制及备份。无需发布外部版本或发送真实MR评论。

各项具体测试名称、输出耗时、native证明路径、截图与剩余事项见implementation.md。临时目录证据是本机记录；最终报告需明确哪些被提交、哪些只在本机，不把不存在的发布或部署记录列为完成。

2026-10-05迁移实查：以SQLite只读连接获取当前部署数据库的一致副本，仅对副本调用当前OpenStore两次，不启动Agent/网络服务。证据目录 /var/folders/y0/q03mg01d2vv5twh26pdhkzbw0000gn/T/aimangebot-migration-acceptance.r8vds6in，副本与proof权限0600、目录0700。账号1/项目1、任务0/复核0/事件0迁移前后相同；integrity_check=ok、foreign_key_check=0。当前生产数据为空任务集，空集哈希不证明非空历史结果保留；非空历史JSON/trace、复核基线、重复迁移与回滚由既有migration测试覆盖。此副本用于迁移验收，不替代最终部署前的新鲜备份。

历史连接阻塞复查（已由后续有效密钥与第四轮评测解除）：当前工作区干净，远端开发分支56556468e344028201e9f4682b659c78a066e110与本地一致，远端main仍834738964e990889efb700d48a4205582c40d7ed。1234根页面HTTP200，验收端口19234/19235均已不可达。以当前config.yaml只读执行go run ./cmd/audit-eval -config config.yaml -probe，仍HTTP401/provider_code=invalid_api_key；未输出或持久化密钥、原始响应正文。连续阶段中已完成错误脱敏、数据副本迁移、当前设置UI和发布状态检查；有效质量评测仍需用户更新配置。不能用重复合成验证替代真实质量结果，不能把main合并/部署写成已完成。剩余最终质量判定、交付报告、main与1234发布按原范围保留，待配置恢复。


最终交付证据与边界见 release-readiness.md。本机API证明 /tmp/aimangebot-deployment-api-proof.json；生产页面截图 /tmp/aimangebot-main-settings.png。保存栏在正常文档流内。生产任务为空，因此任务详情、历史与证据的非空验收采用隔离native/browser及迁移回归，不伪称生产已有审计数据。

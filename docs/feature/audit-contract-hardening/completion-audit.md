# 最终验收审计（进行中）

当前结论：尚未完成整个目标。功能实现与分阶段证据已记录；真实模型质量评测因401无有效结果，最终main与1234交付仍未执行。此清单不会用单项测试替代整体完成证明。

| 原始要求/不变量 | 当前实现与可核查证据 | 当前判断 |
| --- | --- | --- |
| React产品化、登录、成员、项目、任务、复核、设置、日志 | frontend/src；platform auth/routes；platform-modernization/release-readiness；浏览器分阶段截图 | 已实现，最终部署UI复验待执行 |
| 设置同步项目config.yaml、首次生成、密钥保留/脱敏 | settings、revision、project sync outbox；设置冲突/同步故障/启动恢复测试 | 已实现，最终生产备份与落盘复验待执行 |
| Eino语言无关Git工具，不执行仓库代码 | GitRepository、agent tools/investigation；只读固定SHA；目录/检索/批量/历史/元数据共14工具 | 已实现；不提供任意跨仓库权限扩展或运行利用证明 |
| 固定身份与有效配置、base/head/fork、去重 | AuditPolicy、identity migration、enqueue；当前policyv9 | 已实现；旧策略需重新提交，最终迁移检查待执行 |
| 删除保护、源码锚点、调查/反证/观察来源 | ValidateFindings、canonical registry、observation links；伪造/错误锚点/删除BASE测试 | 已实现；源码支持不等于语义或利用性证明 |
| 故障/取消保留发现与检查点 | frozen supplemental result、worker fences、恢复脚本真实SIGKILL | 已验证分阶段原生进程，不以单元测试替代崩溃证据 |
| 有界重试、Retry-After、租约/所有权、不能重复执行 | retry store/transport/worker instance；429/5xx/租约失效/迟到写测试与native proof | 已实现；永久模型错误脱敏正在最终回归 |
| 权限与fork来源、并发/排队/每日配额、代理登录限流 | project_access、quotas、login_throttle、trusted_proxies；API与浏览器证明 | 已实现；支持单实例SQLite部署，非分布式限流 |
| 文件策略排除与覆盖不足区分、分页连续性 | BuildDiff、excluded fields、paging coverage；全排除skipped测试 | 已实现；预算省略不能作已审计/安全结论 |
| 元数据、symlink/gitlink/LFS/binary边界 | typed Git metadata、canonical anchor、API budgets；real Git fixtures | 已实现；不读取外部子模块/LFS内容 |
| 轻量列表及索引，不读取完整trace/result | finding projection、RunListItem、transactional list；多MiB结果/EXPLAIN/API测试 | 已验证；首次历史回填成本明确 |
| 问题时序图解释风险、静态/推测说明与证据 | sequence validation/context/checkpoints，metadata note-only；native/browser | 已实现；不是实际运行轨迹 |
| 复核与GitLab同一评论同步，未知发送不盲目重发 | durable delivery generations/IDs/marker/hash；lost ack/create/update/人工改动测试 | 已实现；生产评论保持关闭，未对真实GitLab发评论 |
| 独立上下文复核与反证，不自动升级/删除 | fresh verifier stage/observations、strict verdict；actual Eino synthetic proof | 已实现；同模型上下文不等于模型多样性或运行复现 |
| 大PR分组、跨组调查、共享预算及可见缺失 | AuditPlan/AuditGroups/read-only synthesis；25文件native2组/browser | 已实现；最大8组/96KiB，超过明确覆盖不足 |
| 稳定问题生命周期与人工历史 | exact fingerprint、ambiguity clearing、occurrence/history migration；3HEADnative/browser | 已实现；无模糊语义匹配、无自动继承/自动修复 |
| 真实模型评测与风险质量，不用fixture数字冒充 | evaluation corpus/CLI/probe；11真实失败+401invalid_api_key诊断与报告 | 未完成有效质量评测，待有效配置 |
| 文档/变更说明/回滚方案 | implementation逐阶段记录、README；本审计 | release/changelog最终版本仍待更新 |
| 最终main提交推送 | 开发分支已逐阶段推送 | 未执行最终合并/push；需核对远程main、无遗漏工作 |
| 1234部署与生产数据保留 | 当前仍运行原main版本 | 待新二进制、SQLite一致备份、配置字节/权限备份、健康/UI/版本复验 |

最终交付步骤：完整工作区/远程分支核对；有效模型整套评测与人工判定（保留失败轮次）；最终Go/race/vet/frontend与迁移验证；记录实际范围、质量限制及CHANGELOG；一致备份DB与配置，合并并推送main确认SHA；构建同一main产物，正常停止旧服务后替换1234；确认健康、登录、设置脱敏/配置同步、任务详情历史/证据、权限与原数据，保留可回退旧二进制及备份。无需发布外部版本或发送真实MR评论。

各项具体测试名称、输出耗时、native证明路径、截图与剩余事项见implementation.md。临时目录证据是本机记录；最终报告需明确哪些被提交、哪些只在本机，不把不存在的发布或部署记录列为完成。

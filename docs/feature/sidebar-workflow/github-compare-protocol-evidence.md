# GitHub 固定提交比较：协议证据与剩余决策

## 工作流门禁

输入：Confirmed requirements.md的REQ-012/019/021/023、github-provider-design.md及用户“没想清楚先不实现”。分支codex/sidebar-workflow，基线12f9668ea867324db976afcae304274c2a9358b4。阶段P5/P6、F2研究补充；仅更新设计证据，不作为技术方案批准，不进入PR/compare/factory生产实现。G1/G2及固定对象模块存在，但没有完整bound执行消费者；现有身份保护保留。完整REQ-001–023 / AC-001–021不缩减。

发现的上游薄弱点包括fork限定SHA语法、空commits语义及授权前访问来源内容。此次关闭其中部分协议疑问；GitHub Enterprise兼容性、完整差异及恢复设计仍未闭合。没有公开API/schema/数据库/配置变化、远端写入或新增依赖，无需运行无关构建。

## 文档事实

[官方compare说明](https://docs.github.com/en/rest/commits/commits#compare-two-commits)允许同一仓库网络内比较提交SHA，并规定跨仓库分支的owner限定写法。不分页的提交集合与分页集合具有不同末项语义；文件集合仅第一页、上限300，不能用提交分页补齐。此页面默认样例现已使用2026-03-10，不能因此改变本项目2022-11-28请求契约。

对应版本的固定[官方OpenAPI](https://github.com/github/rest-api-description/blob/734bc9c1030b774eb3fc909cce477aceea21cf77/descriptions/api.github.com/api.github.com.2022-11-28.json)证据见github-provider-design.md：没有head_commit，files非required。例子不足以证明所有状态、网络身份和授权条件；此次另外请求真实公开fork，避免把自行编写的fixture当协议证据。

## 真实只读协议探测

2026-10-07约15:53 UTC，公开[cli/cli PR #14617](https://github.com/cli/cli/pull/14617)提供样本。通过现有gh客户端执行GET，所有比较/对象/仓库请求显式设置X-GitHub-Api-Version: 2022-11-28。未修改该PR、发送评论或访问本项目生产通知/工单配置。首次匿名API读取因rate limit exceeded返回403，改用已有gh认证读取公开元数据；没有输出或保存认证值。

冻结样本（后续PR状态变化不改写此证据）：

| 对象 | 身份 |
| --- | --- |
| 目标 | cli/cli，repository ID 212613049，独立仓库GET确认fork=false |
| 来源 | bingtang9/cli，repository ID 1408291984，独立GET确认fork=true，parent/source ID均212613049 |
| B，目标分支观察提交 | 17142e08db2e300b37e6da1ddcfb651eb6d9c587 |
| H，来源提交 | 83e09829e010258ab80cfaba4daa22a167b9d8ba |
| 固定B对象 | 目标git/commits/B返回相同SHA，tree 03626743b05d209a4dcb592cb0f30e33761b3e6c |
| 固定H对象 | 来源git/commits/H返回相同SHA，tree 5817c860769933c789eb4743963be03f9a271551 |

请求前缀为GET /repos/cli/cli/compare/，全部不指定分页：

| basehead | 实际结果 |
| --- | --- |
| B...H（两处完整SHA） | ahead；base=B，merge_base=B；ahead_by=2，behind_by=0，total_commits=2，返回2个commits，末项=H，返回2个files，无head_commit |
| cli:B...bingtang9:H | 与上一行全部投影字段一致；证明本样本owner:完整SHA语法可用 |
| B...B | identical；base=merge_base=B；ahead_by=behind_by=total_commits=0，commits/files均空，末项null |
| bingtang9:H...cli:B | behind；base=H，merge_base=B；ahead_by=total_commits=0，behind_by=2，commits/files均空，末项null |

这不是自动化测试或本系统运行证明。它只验证GitHub.com、这个公开且fork名称仍为cli的网络、这些冻结提交和上述四种请求；不覆盖私有仓库/重命名fork/多fork歧义/Enterprise/diverged/超过250提交/force-push/配额耗尽/完整文件差异。反向比较是构造behind样本，不代表该PR自己的base/head顺序。

## 设计结论与消费者约束

1. owner:完整SHA在该版本/样本可用，既有“完全没有fork语法依据”的疑问可收敛；不能把用户名当repository ID证明。实际执行仍须核对目标/来源绑定ID、origin及独立仓库身份，不能从compare permalink猜来源。
2. identical和behind的合法响应没有HEAD提交末项。统一要求末项=HEAD会错误拒绝；统一允许空集合又会把缺失/异常响应洗为没有改动。候选状态判定须分别验证status、base_commit、merge_base_commit、固定B/H对象及计数，不用空集合单独证明。
3. 对behind候选，merge_base=H时merge-base→H没有源码改动；不能用B→H反向目标分支差异冒充PR新增改动。对identical候选，B=H=merge-base。上述关系判定仍须与正式对象/响应校验合同一起完成，不因单个观察自动授予实施许可。
4. 此探测尚未证明完整compare文件覆盖。完整两树差异方案必须定义模式变化、gitlink/symlink、路径集合、rename身份及补丁缺失如何传播到审计覆盖；普通源码ListFiles的2000项预算不是全仓差异保证。

拟议生产访问顺序（当前未实现）：

```mermaid
sequenceDiagram
 participant A as Admission与Store
 participant T as 目标GitHub元数据
 participant O as 固定对象与比较
 A->>A: 授权目标项目并冻结目标绑定/集成版本
 A->>T: 只读取仓库与PR身份元数据
 T-->>A: base/head仓库ID、名称、完整SHA
 A->>A: 来源身份映射为已有内部项目；授权来源与上下文
 A->>A: 核对来源启用、绑定、origin和集成版本
 A->>O: 才允许来源commit/tree/blob、compare及patch读取
 O-->>A: 已校验对象与显式完整/不完整结果
 A->>A: 写事务二次核对全部ACL/绑定/集成版本/配额后入队
```

compare也可能泄露来源提交描述、文件路径和patch，因此它属于来源内容访问，不能在来源ACL之前请求。公开fork本身不授予本应用权限。源码、PR正文和提交说明不作为授权依据。观察前后PR相同不能证明期间读取无变化；所有实际内容必须固定SHA。

## 尚未闭合的实施前置

| 决策或证据 | 当前状态 | 进入代码前所需证明 |
| --- | --- | --- |
| fork SHA路由 | GitHub.com同名公开fork有真实证据 | renamed/multiple fork歧义及允许origin支持范围明确 |
| HEAD关系 | ahead/identical/behind样本有证据 | diverged、超过250提交、坏/缺字段、有限计数与对象关系校验形成完整合同 |
| 差异覆盖 | compare文件上限已明确 | 固定树比较预算、rename/mode/gitlink/patch语义，截断不能无告警完成 |
| 来源权限 | 访问顺序已明确 | 未授权时零来源对象/compare访问；版本/ACL变更时入队拒绝的集成证明 |
| 入口恢复 | UAR-001仍open | 旧项目迁移/恢复及正式开放前置；不得删除身份guard或猜GitLab ID |

没有新增可被执行的核心对象；拟议Snapshot字段仍按github-provider-design.md矩阵，未新增字段。后续实现文件应分离PR观察、compare关系及固定树差异，并通过统一factory供所有入口消费，不能堆在runner.go。推出时必须与绑定开放/恢复及端到端验收一起评估；回滚保留历史身份，不将旧运行改解释为默认GitLab。当前无F3实施许可，无main合并或发布。

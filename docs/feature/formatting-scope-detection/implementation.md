# 代码格式化范围约束检测（上游 issue #2）

## F0 需求与边界

上游 issue #2：团队规则要求「格式化只作用于本次修改的部分，禁止对整个类或文件全局格式化」——成员 IDE 配置差异会让无关的格式变更混入提交记录，难以追溯与审查；希望 AIMergeBot 支持该规则的检测。

交付形态（与需求方确认）：违规文件产生一条 **low 严重度的确定性发现**（进入工作台、MR 评论、SARIF 与人工复核流程）；默认关闭，配置 `check_formatting_scope` 开启。

边界：检测是**语言无关的词法分类**（空白归一化），不是语义解析，不执行仓库代码；不改变既有安全审计契约——默认关闭时零行为变化，开启时新增发现与既有发现并列，不改写任何既有字段语义。

## F1 判定口径

- 数据来源与审计范围一致：固定 BASE/HEAD diff 中通过预算检查、真正进入审计范围的文本变更（与 `get_diff`、锚点校验共用同一份解析结果）。
- 行相等定义：逐行去除**全部空白**（`strings.Fields` 连接）后相同视为「仅空白差异」；原始行（保留行尾 `\r`）不同才算一次真实行变更。因此缩进改动、等号两侧空格、行尾空格、以及 CRLF→LF 重写都会被识别。
- **纯格式 hunk**：某个 hunk 的全部删除行与新增行在去空白后构成相同多重集（该区域没有任何内容修改），且至少有一对原始文本不同。判定用多重集比较，不依赖行序对齐，因此重复行、重排行不会错配。
- 锚点选择：先按序消费「原始文本相同」的行对，再取第一个真正发生变化的非空白新增行（HEAD 行号 + 行文本），保证锚点不是 git 重发的未变行。
- **违规判据**：文件存在 ≥1 个纯格式 hunk。混入内容改动的 hunk（即使含空白差异行）不计为纯格式 hunk——那属于「修改部分内的格式化」，规则允许；整文件重排会自然产生大量纯格式 hunk。
- 已知词法边界（有测试固定）：字符串字面量内部的空白差异无法与格式差异区分；跨行换行/重排不会配对为空白差异；新增/删除文件不判定。

## F2 设计

- **检测位置**：服务器侧、模型调用之前（`EinoAuditor.audit` 内）。确定性、可复现、零模型调用、随任务进度检查点保存。
- **发现形态**：每个违规文件至多一条 finding——`type=formatting scope`、`severity=low`、`confidence=candidate`（无调查链时唯一诚实取值）、锚定首个纯格式 hunk 的首个真实变更新增行、`evidence` 为该行文本；复用既有 `ValidateFindings` 校验（锚点必须属于本次审计范围的变更行、证据必须与 HEAD 快照对应行匹配）。
- **与安全管线隔离**：`origin=formatting_scope` 的发现**不进入安全向独立复核**（复核提示词明确要求把非安全发现一律驳回，交给它复核会被系统性误驳）、不生成问题链路图、不产生 PR 上下文记录缺口；工作台按既有「无独立复核记录」状态展示，发现正文自述「确定性检测，不构成安全结论」。
- **有界与透明**：每审计最多 20 条；被截断的文件与无法锚定的文件（如纯空白行变更没有可作证据的非空行）写入覆盖说明；`acceptFinding` 失败不静默丢弃。
- **配置**：`check_formatting_scope`（YAML/JSON，默认 false）随 Settings→AuditPolicy→AgentConfig 冻结进任务策略；设置界面保存不带该字段时不会覆盖其值（`DecodePublic` 以当前值起步）。
- **提示词**：开启时在系统提示中告知模型这些确定性发现已收录、无安全结论、勿重复勿反驳，保证模型总结不与之矛盾。

## F3 实现

| 文件 | 改动 |
| --- | --- |
| `internal/platform/formatting_scope.go` | 新增：diff 解析、多重集判定、锚点选择、发现注入（`recordFormattingScopeFindings`）与文案 |
| `internal/platform/diff.go` | `DiffScope.Formatting` 统计字段；`BuildDiff` 对范围内文本变更计算统计，同路径分块累加 |
| `internal/platform/diff_chunks.go` | `mergeScopeAnchors` 合并 Formatting 统计 |
| `internal/platform/types.go` | `Finding.Origin`（`formatting_scope` 标记，模型发现为空） |
| `internal/platform/agent.go` | `AgentConfig.CheckFormattingScope`；audit 内注入；开启时的系统提示；关闭复核分支跳过确定性发现 |
| `internal/platform/verification_agent.go` | 独立复核循环跳过确定性发现（保持 `Verification=nil`） |
| `internal/platform/sequence_agent.go` | 时序图循环跳过确定性发现并给出明确原因 |
| `internal/platform/pr_context_coverage.go` | 确定性发现不产生 PR 上下文记录缺口说明 |
| `internal/platform/settings.go`、`audit_policy.go`、`runner.go`、`standalone.go`、`cmd/audit-eval/main.go` | 配置字段与四处 AgentConfig 构建点接线 |
| `config.example.yaml` | `check_formatting_scope: false` 及注释 |

## F4 验证

- 单元测试（`formatting_scope_test.go`）：分类表 11 例（缩进、混合 hunk、整文件重排、CRLF、内部空格、新增/删除文件、字符串字面量边界、重发行不作为锚点、无 newline 标记、无锚点空白行）；BuildDiff 集成（锚点属于 Added、分块累加、scope 合并）；注入（字段与锚点、幂等、无 PR 缺口、上限 20 与跳过公开、设置默认关/落盘/省略字段保留/policy 捕获）。
- SDK 全链路测试（`formatting_scope_sdk_test.go`）：httptest 假模型驱动完整 `audit()`——开启时恰一条确定性发现、独立复核与时序图补充阶段**零模型调用**、`Verification=nil`、系统提示含公告；关闭时零发现；复核关闭分支同样不写入 verification。
- 门禁：`scripts/testtree.sh gates`（sync/verify + go 全量测试、race、vet、Python 门禁、前端产物漂移、CGO 构建）通过；前端未改动。
- 未验证：真实 GitLab MR + 真实模型的端到端效果；合成 fixture 不代表真实模型行为，也不代表团队规则在真实仓库中的误报率。

## 回滚

关闭 `check_formatting_scope` 即恢复原行为；`Finding.Origin` 为增量 JSON 字段，旧版本读取新报告时忽略未知字段；无数据库迁移。

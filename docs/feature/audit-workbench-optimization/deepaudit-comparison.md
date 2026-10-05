# DeepAudit源码借鉴评估

> 2026-10-06定位修正：本项目为PR增量审计。入口侦察、知识加载、跨仓库搜索和链路核查均由PR改动驱动，结论必须解释BASE/HEAD变化与风险的因果关系。全仓库取证不等于默认全项目扫描；最新需求为R3。

日期：2026-10-06。只读克隆并检查源码，未安装依赖、执行被审计代码或测试实际模型效果。固定版本：`630229163f10c9be74317d1edd06d16cfe6c59f0`。所有建议保持语言无关、跨仓库调查；不以AST、编译或第三方扫描器作为准入条件。

## 借鉴优先级

| 优先级 | DeepAudit实现 | AIMergeBot采用方式 |
| --- | --- | --- |
| P0 | Recon输出技术栈、入口、高风险区域和建议行动；阶段间通过TaskHandoff交接关键事实。 | 在已有分组调查之前增加有预算上限的入口侦察；每个事实携带仓库、固定SHA、文件与行锚点，跨组/跨仓库传递结构化证据及未解决问题。不能只传模型摘要。 |
| P0 | 知识文档按风险类别组织，含检测清单、危险示例、修复建议。 | 按候选风险加载小型知识模块；重点补权限、租户隔离、业务状态、幂等和竞态的反证问题。知识是检查提示，不能成为源码事实。 |
| P0 | Verification单独处理候选，循环检测重复工具请求。 | 加强现有独立复核：逐项记录成立条件、保护措施、反证和未核实边；重复调用应提示换调查路径并耗预算，不能据此确认漏洞。 |
| P1 | Agent树、阶段面板、日志与流连接状态。 | 展示任务、源码观察、待验证假设、覆盖缺口和预算；证据到源码可直达。流式展示需重连、游标补发和权限控制；无需展示模型内部思维。 |
| P1 | 语义检索后用关键词重排，返回文件位置和代码片段。 | 先完善跨仓库批量搜索/读取与锚点导航，再评估语义召回。索引按授权仓库与固定SHA隔离，失效时回退文本工具；不能依赖向量库支持某语言。 |
| P2 | 可选外部扫描器、代码执行与沙箱验证。 | 作为按需适配器或单独受控验证任务，不放入默认只读审计路径；执行成功和漏洞复现分别记录。 |

## 源码支持与边界

- [Recon交接](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/agents/recon.py#L781)和[Analysis交接](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/agents/analysis.py#L828)：结构化分阶段任务值得借鉴；多个项目可管理与一次调查能建立跨仓库证据链是不同能力，不能从介绍推断后者已成立。
- [知识文档结构](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/knowledge/base.py)：适合借鉴分类及检查问题的组织方式，由我们编写适合本项目的知识内容。
- [混合检索实现](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/rag/retriever.py#L508)：关键词仅重排已召回的语义候选，并非独立关键词召回与向量召回合并。不能保证找回语义召回遗漏的精确符号。
- [数据流工具](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/tools/code_analysis_tool.py#L173)：含模式检查与模型分析，不能把工具名称视为编译器级数据流证明。
- [验证完成门槛](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/agents/verification.py#L692)允许读取源码作为工具调用；[结果归一化](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/backend/app/services/agent/agents/verification.py#L879)可将模型的confirmed或高置信likely标成is_verified。因此该标记不能直接解释为已完成运行时复现。
- [README成果说明](https://github.com/lintsinghua/DeepAudit/blob/630229163f10c9be74317d1edd06d16cfe6c59f0/README.md#L87)明确将49个CVE和6个GHSA归于闭源版本；不能据此证明当前公开代码的检出率。
- 仓库LICENSE为AGPL-3.0。本轮只借鉴方法并独立实现，不复制源码；实际引入依赖或代码时另核查相应许可义务。

## 对当前项目的判断

AIMergeBot已经具有固定提交读取、授权上下文仓库、分页覆盖跟踪、独立复核和持久化工具证据，无需为了相似界面重建审计核心。最有价值的增量是：入口侦察 → 风险知识选择 → 带锚点的结构化交接 → 跨仓库链路核查 → 可核查的验证等级。多Agent数量、RAG存在与沙箱存在本身都不是准确率证据。

建议先用多语言和跨仓库正负例测量这些增量的召回、误报、证据完整度与成本，再决定是否加入向量索引和动态Agent编排。本文件为研究结论，不表示功能已实现或效果已验证。

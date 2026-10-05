# 正式发布差距检查与加固

## F0 / 2026-10-05 当前证据

用户要求：看看现在和正式版还有哪些差距或者bug要修，最佳实践优化。继承最终提交main、1234部署、敏感凭据不入Git、语言无关/只读Git约束。此前暂缓的真实历史质量、公网Webhook和Linux实测仍明确未验证，不能重新包装为通过。

起点main447fc73045766e862efb56f48492964659da6b3c，工作区干净，1234就绪。当前Eino v0.9.21；主审和独立复核使用flow/agent/react。官方ADK summarization模块确实在本机依赖中，但没有接入。主审具有大PR分组、有界工具返回和累计token停止阈值，尚无长对话自动摘要压缩。仓库没有.github CI工作流。登录限流、同源写检查、HttpOnly/SameSite cookie、请求体1MiB限制、HTTP header/idle timeout、readyz已有实现，尚需核对完整发布路径。

## Workflow Gate

阶段P10反馈迭代/P9发布验证；类型后端Agent和发布加固。上游为已有平台需求/design、生产迭代记录、源码与运行证据。当前允许只读核查和需求/设计记录；上下文中间件迁移会影响证据与用量，需要先形成设计及测试契约，不能直接替换Agent。执行范围覆盖Agent、权限/配置、任务可靠性、交付自动化及运维；逐项保留发现/修复/证据/未验证状态。验收：实际缺陷有反例和修复验证；摘要不赋予源码资格、不丢固定快照/调查状态、不破坏工具配对；成本和失败状态真实；最终main和部署具有确切版本。假设不执行仓库代码、不引入语言专属分析器、不新增外部自动写入。

## Feature Lifecycle Report

分支codex/formal-release-hardening；当前F0。场景：用户在长PR审计及正式部署中需要可靠结果和可维护发布。公开影响待设计核定，默认保持现有API和权限。阶段文档为本文及后续requirements/design/evidence；各完成阶段独立提交推送，最终合并main。需要Agent原生多轮/压缩/错误/预算测试、权限边界与完整Go/Python/前端检查、部署smoke。密钥、数据库和原始私有证明不提交。当前没有已确认代码修复，不能宣称正式发布完成。

## 核查队列

1. 上下文生命周期：官方ADK压缩/工具结果卸载接入、证据持久索引、恢复、用量/模型JSON契约。
2. 主审/独立复核/时序图：格式失败、覆盖解释、模型预算、工具错误恢复和来源隔离。
3. 用户/项目权限、同源写入、配置持久化与会话边界。
4. 队列、租约、重试、评论投递和取消恢复。
5. CI/版本/构建/备份/部署配置一致性；文档与实际默认值对照。
6. 未验证外部场景独立列出，不扩大合成测试含义。

## 基线验证与接入约束

当前38fc3f7源码不变：go test ./...通过（platform31.267秒、evaluation3.352秒）；Python27项6.194秒通过，ResourceWarning按error；预算/认证/就绪定向测试2.218秒通过。未声称已有race/vet/前端本轮验证。

本机Eino源码核对：ReAct的MessageRewriter每轮修改累积state.Messages，但函数不能返回error；ADK summarization.BeforeModelRewriteState可以直接调用、state包含Messages和ToolInfos且支持错误。因此适配器需要显式停止机制，不能摘要失败后继续发送超长原消息。摘要默认输出不是业务最终JSON，不应复用强制JSONObject的主审模型配置。其额外请求必须归入共享预算与独立compression阶段trace，工具源码证据仍以服务端原始索引核验；系统约束、固定提交、任务输入及未完成调查应保留。完整设计和反例测试是下一阶段，尚未改变Agent。

## F1 / 已授权需求与验收契约

用户已授权最佳实践优化及最终main交付；保守实现不改变写入权限、审计方法和数据保留策略。REQ-C1：长调查自动减少模型历史占用，保留系统指令、固定快照和原始任务；AC-C1：实际多轮HTTP模型请求触发摘要后继续工具调查。REQ-C2：原始工具证据、调查、反证与已接受发现仍由服务端完整核验；AC-C2：压缩摘要没有源码资格，任务账本原样保留。REQ-C3：摘要失败/空输出/取消停止并保留部分结果；AC-C3：失败不再向主模型发送超长上下文。REQ-C4：摘要调用纳入共享预算、检查点和compression阶段用量；AC-C4：未知usage/阈值使下一请求停止，统计不重复。REQ-C5：自动CI覆盖Go、Python、前端且不需要真实密钥；AC-C5：工作流仅只读仓库权限，不处理本机私有配置，不发布。发布差距其余核查保持原队列。

## F2 / 上下文压缩设计

保留现有ReAct，使用MessageRewriter适配官方summarization.BeforeModelRewriteState；每次Audit独立状态和可取消子context，摘要失败取消该主审上下文，服务端保留提交结果。摘要模型采用同模型/接口但不设置JSONObject，并用budgetModel共享累计预算；调用显式替换callback上下文，compression阶段独立检查点/用量。原始trace/cache/ledger不删除，摘要为不可信调查导航，不可作为源码观察。系统及初始用户任务原样保留；Finalize加入有界摘要、完整调查/已接受发现状态和原始观察索引；最近完整assistant/tool交换只有满足容量时保留，不能保留孤立tool。固定字节阈值128KiB保守触发（含工具schema计数）；压缩后消息112KiB上限，摘要16KiB/状态48KiB上限，超过即显式incomplete而非删证据。重复触发最多8次。默认自动启用，无需新增配置/API，合约升级v14。已有固定对象工具可重新读取摘要中索引对应源码，原观察仍可按原ID核验；不添加新源码资格工具。

CI采用GitHub Actions push/PR只读contents权限，Go全测/race/vet、Python工具测试、前端锁文件安装/typecheck/build；超时和并发取消，固定action提交、无真实模型/GitLabkey。部署仍手动受当前用户授权，CI不部署。

## F3 / 实现计划及维护门控

新增agent_compression.go/test以隔离消息计数、Finalize、官方适配和callback；agent.go仅初始化/挂钩/错误说明，runs_store.go仅升级策略常量。这些文件分别281行/小型编排，允许narrow_fix/adapter_extraction，不扩大大型模块。测试纯消息配对/状态保留/尺寸/取消和真实本机HTTP多轮压缩成功/失败/预算/用量，无真实凭据。新增.github/workflows/ci.yml及文档说明，不改生产权限。保留原部署备份，代码验证/检视后再合并和部署；任何真实模型质量声明需独立证据。补充CI和上下文正式文档/CHANGELOG，并完成原队列核查。

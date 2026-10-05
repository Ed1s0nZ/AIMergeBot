# 实施与验证记录

## 切片1：上下文仓库批量读取

2026-10-06，已实现Agent工具read_repository_files(repository_id, files)。同一授权固定仓库一次读取1–8个范围，整体输出最多16000字节，保留编号源码、more和remaining。失败与超限整批不返回部分源码；读取前后权限核查；取消即使缓存命中也不返回源码。一次批量请求消费一次工具预算和观察，复用已有检查点与来源资格机制。主PR变更锚点要求保持。

新增context_repository_batch.go及测试；注册与来源资格增加工具名；策略版本v15，防止旧运行自动采用新工具契约。无需DB迁移。未修改用户README及docs/images。

验证：go test ./internal/platform -run Context -count=1通过（15.089s）；go test ./...通过（platform 42.750s）；新增覆盖状态/输出预算/取消测试后go test ./internal/platform -run TestContextBatch -count=1通过（10.848s）；git diff --check通过。没有真实模型质量对照，不能声称误报或召回改善。未修改UI，本切片无需界面验证。

代码复核：权限复用invokeContext前后检查；批量不循环调用外层工具、不产生未持久化子ID；跨仓库来源通过RepositoryID映射固定SHA；任何子项失败清空聚合输出；部分范围保留覆盖标记且继续读取后续请求项。该来源资格只证明取证，不证明调用关系或运行可利用性。

剩余：PR入口与风险知识、因果链、工作台与复核冲突、轻量更新/缓存、SARIF及多语言跨仓库实际质量评测均未完成。整体目标继续，首切片完成不代表REQ-002整体验收完成。

## 切片2当前实现：按风险加载反证问题

get_risk_checklist支持五类固定风险键；知识由本项目独立编写，使用正常工具预算和检查点，但evidence_eligible=false，不能链接为源码观察。主审提示要求从PR行为差异定位入口/契约、记录带来源事实及未知边；独立复核强调BASE/HEAD因果与历史缺陷排除。策略v16，历史记录仍可读。此增量尚不是完成的结构化入口侦察或交接能力，也未验证检测质量改善。

测试发现并修正复核阶段识别与旧工具数量断言的兼容问题；新增知识非证据、取消、预算及新上下文可用性测试。高负载时独立Git fixture一次超时，单独复跑通过（6.580s）；不提高测试时限掩盖失败。最终修正后go test ./...通过（platform 43.229s）；git diff --check通过。早期全量失败包含旧固定工具数量断言和已修正复核提示识别，保留原因，不解释为质量评测。

## 切片3A：结构化PR调查上下文与展示

Investigation.pr_context保存变更概述、BASE/HEAD行为、带观察ID的入口/保护陈述及未核实关系；服务端限制字段大小、观察来源及调查关联。非法更新不覆盖原调查，知识观察不能成为事实。Finding.pr_context仅从关联已验证ledger深拷贝，忽略模型自填内容；接受后的发现不随后续调查静默变化。历史空值保持兼容，策略v17。

前端PRInvestigationPanel复用于发现及调查记录，空值显示未记录，区分来源陈述与未知边，可通过既有证据按钮展开工具轨迹。React文本渲染，不执行模型HTML。Go全量测试通过（platform 43.488s），PRContext定向测试通过（1.221s），npm run typecheck与npm run build通过，git diff --check通过。嵌入前端资源与源码一起更新。

本地Vite隔离样例通过浏览器验证：Enter展开，BASE/HEAD与入口/防护/未知关系可见，点击观察按钮展开对应轨迹；历史空态显示正确。截图/tmp/aimangebot-pr-impact-proof.png为界面样例，不是实际审计结果。临时预览文件及服务已清理。尚未完成窄屏验证或真实模型填充率评测，也没有完整逐边风险链与跨组专用交接；不能标记全部REQ-003/005完成。

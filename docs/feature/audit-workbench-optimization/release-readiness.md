# R3 分支交付

更新：接口已恢复，原六例v35回归完成，393448 tokens、六例均incomplete；HTTP402已不再阻塞。质量缺口仍未解决，详见 regression-v35-restored.md，旧402记录为历史失败。

## 当前状态（生产591fe99 / v36）

当前为部分交付，优化目标仍未通过逐项完成审计。最新生产代码591fe996415120aa2b9b8501cdea0d7c9acf5b14 CI全部成功（https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37423152398），后续提交为评测文档；context执行器停止修复及SDK/race验证通过。接口已恢复：v35恢复回归393448tokens，v36回归459281tokens，均六例incomplete。v36部分负例补账本/显式收尾，但Claim与rejected极性不一致；独立复核引用失败、关系缺项与过强部署断言仍是质量限制。详见regression-v35-restored.md、regression-v36.md与verification-matrix.md。HTTP402旧运行是保留的历史失败，不是当前阻塞。

当前PR描述补充：四方面来源检查、调查计划、记录导航与未收尾计数增强可追踪性；固定来源、证据资格、步骤和工具预算保持原约束；不可用执行器后停止后续模型请求并保留候选，分组停止原因由服务器生成。记录结构合法不保证语义正确，真实回归仍有上列限制；未创建PR、未合并或部署。

## 历史 v28 交付记录

本轮完成语言无关、跨固定仓库、以PR因果变化为中心的审计机制与工作台优化。保持只读Git调查，不自动执行样本或扩大为整仓漏洞盘点。需求逐项证据见verification-matrix.md，研究来源见research.md。

## 验证与限制

平台全量Go测试最后一次45.436s通过、相关race通过；CLI/evaluation定向检查通过，前端最终TypeScript/Vite构建871ms exit0，实际受控IAB检查窄屏、错误恢复、复核草稿、下载以及Return/Tab导航。详见implementation.md、ui-final-qa.md；不是生产环境端到端测试。

v28首次冻结隔离v2覆盖Rust/PHP/C#/Java及Python/Ruby下游；2个正确条件风险，4个负例零告警，405因主调查未读下游不足以认证兼容。总296749 tokens、6/6 incomplete，费用未配置。金融调试集仍有误报和数值方向错误；独立full是模型声明，引用与门禁不能保证语义。

人工复核请求必须传expected_revision；旧调用者需同步升级。SQLite迁移保留历史记录，历史claim_coverage缺失不改写。详情版本为无源码状态投影，SARIF为固定静态记录，没有伪造运行codeFlows。详见migration-notes.md。

## 最终审查

平台/前端/评测生产改动已风险导向核对；发现AWO-REV-001导航hash冲突并在41b13b4修复，浏览器验证route不变、目标聚焦、Tab到草稿。最终不可变head的正式报告保存在工作区外evaluation-artifacts目录，避免提交报告导致其审查SHA失效。决策为COMMENT，不授权合并：CI未观察、生产数据/部署/真实多人使用未验证，模型质量限制明确保留。

## PR描述草稿

问题：PR风险缺少结构化前后/反证记录，跨组容易重复调查，独立支持易被误读为全文/运行证明，多人复核覆盖决定、详情反复传输大报告。

变化：保持增量锚点和授权固定源，增加批读/来源链/有界交接/独立支持降级；工作台提供筛选、可靠页内导航、版本冲突、轻量刷新和SARIF。模型未经证明的内容和覆盖不足保留供人工判断。

验证：上述Go/race、CLI与前端检查，官方SARIF Schema及实际下载，受控IAB交互，冻结隔离v2逐项人工核对。限制：合成小样本、无代码执行，405未读下游及金融语义误报尚存。未创建PR、未合并、未部署；用户README.md与docs/images未纳入。

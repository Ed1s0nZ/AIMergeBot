# R3 分支交付

## 当前增量：83671ea（策略 v41），整体仍未完成

无finding调查也用共享PR recording gaps保留最终BASE/HEAD来源与关系缺口；metadata-only依固定来源分类，同步feedback/live/final，不制造执行路径。最终生产源码full Go exit0 platform111.423s、广泛定向race8.043s、追加SDK/worker/SARIF race4.447s、vet/diffcheck通过。独立切片COMMENT、无blocking，定向9.457s/自设计8类来源边界0.709s通过；非整分支批准。源码CI run37434193201运行中。

旧真实v40-802离线投影新增三条缺口，全部Claim/status/原记录JSON未变，无新模型请求，不当作准确率/效果回归。原真实模型的错误Claim极性、跨项目关系及部分入口质量仍未充分证明，AER-001 open；v41只防止最终说明丢失，不修复语义判断。未合并、未部署。

## 历史增量：23d3bc3（策略 v40），整体仍未完成

新增状态对应next_recording_action，建议真实源码/计划/PR链接/上下文/关系/收尾/纠正操作，未改变来源校验或自动确认记录。full Go exit0（platform134.382s）、race7.774s、vet/diffcheck通过；独立边界scope COMMENT/mergeable=false，无新blocking，非整体批准。源码CI run37432182646 success。

真实case-802在原模型/预算下102808tokens、四项checked计划与显式同陈述纠正已观察，但safeClaim错误rejected、结构化关系与BASE/HEAD链接仍缺，状态incomplete。详见regression-v40-actions.md；不能用结构化字段或0finding当质量证明，未重跑803/805，AER-001 open。只交付分支，未合并或部署。

## 历史增量：0e7db81（策略 v39），整体仍未完成

新增HEAD路径预导航保持非源身份和原预算。真实case-802定向回归绑定dda3eda：17910tokens、原API正常、caller.go/workflow.md确实读取，0finding但无调查计划仍incomplete；仅一个已知样本，不是普遍质量证明。详见regression-v39-navigation.md。

独立dda3eda检视发现V39-001检查点失败后仍继续分组；已在0e7db81修复，首错阻断后组与补充、保留原错误链和trace。当前全量Go exit0 platform102.421s、定向race2.930s、vet/diffcheck通过；源码CI run37431241936 success，独立修复复查COMMENT、V39-001 resolved，无新blocking（仅边界scope，非整体批准）。未修改UI，历史UI证据适用无变化组件，不当作全分支新批准。AER-001整体完成阻塞仍open：调查计划/跨项目关系/部分命题极性未充分证明；未合并、未部署。

## 历史状态（源码 da039f3 / v38）

当前为部分分支交付，整体目标未通过完成审计。最新源码da039f374bc26a3c6db6307ab0d2f13b4af80fae已推送codex/audit-quality-loop，CI run37427483339终态success（https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37427483339）；本地全量Go exit0（platform111.645s）、定向race4.608s、vet通过。已恢复原接口，v37真实回归479052tokens、六例incomplete、无HTTP402；旧402失败保留为历史。最新v38未调用真实模型，不宣称导航改善实际完成率或费用。

v38最多提示4对仍pending且符合原校验的记录回执；显式resolver保留原子性、历史及真实源/计划/关系缺口。旧801回执重建仅10→16实际合格；15→17完全同参，原机制已清除15，13陈述不同继续保留。补查纠正参数匹配与待办状态的混淆，不重写原模型结果。

独立v38切片COMMENT，无确认阻塞finding；工作台AC007–010/012与复核迁移有当前源码/定向/官方schema/构建一致性及受控浏览器证据。新RunDetail计划/分项复核面板已实际验证360px、Return/Tab来源导航、历史空态和route保留，临时环境清理。整体审计证据复查仍INCOMPLETE：AC002/003/005的来源遗漏、跨项目关系及部分命题极性不足；AC011建立与隔离记录已证明，不新增所有例completed或数字准确率门槛。详见completion-audit-v38.md。

本轮PR描述补充：新增分项验证/调查计划、有界风险预算、服务器停止原因、非源码状态导航、显式同陈述纠正；保留语言无关、授权固定源、主PR因果锚点、原模型与预算。记录结构合法不保证语义正确；提供可复核证据与真实限制，未创建PR、未合并或部署。当前剩余工作以原R3验收为准，不将机制通过替代审计质量。

## 历史 v28 交付记录

本轮完成语言无关、跨固定仓库、以PR因果变化为中心的审计机制与工作台优化。保持只读Git调查，不自动执行样本或扩大为整仓漏洞盘点。需求逐项证据见verification-matrix.md，研究来源见research.md。

## 验证与限制

平台全量Go测试最后一次45.436s通过、相关race通过；CLI/evaluation定向检查通过，前端最终TypeScript/Vite构建871ms exit0，实际受控IAB检查窄屏、错误恢复、复核草稿、下载以及Return/Tab导航。详见implementation.md、ui-final-qa.md；不是生产环境端到端测试。

v28首次冻结隔离v2覆盖Rust/PHP/C#/Java及Python/Ruby下游；2个正确条件风险，4个负例零告警，405因主调查未读下游不足以认证兼容。总296749 tokens、6/6 incomplete，费用未配置。金融调试集仍有误报和数值方向错误；独立full是模型声明，引用与门禁不能保证语义。

人工复核请求必须传expected_revision；旧调用者需同步升级。SQLite迁移保留历史记录，历史claim_coverage缺失不改写。详情版本为无源码状态投影，SARIF为固定静态记录，没有伪造运行codeFlows。详见migration-notes.md。

## 历史 v28 最终审查

平台/前端/评测生产改动已风险导向核对；发现AWO-REV-001导航hash冲突并在41b13b4修复，浏览器验证route不变、目标聚焦、Tab到草稿。最终不可变head的正式报告保存在工作区外evaluation-artifacts目录，避免提交报告导致其审查SHA失效。决策为COMMENT，不授权合并：CI未观察、生产数据/部署/真实多人使用未验证，模型质量限制明确保留。

## PR描述草稿

问题：PR风险缺少结构化前后/反证记录，跨组容易重复调查，独立支持易被误读为全文/运行证明，多人复核覆盖决定、详情反复传输大报告。

变化：保持增量锚点和授权固定源，增加批读/来源链/有界交接/独立支持降级；工作台提供筛选、可靠页内导航、版本冲突、轻量刷新和SARIF。模型未经证明的内容和覆盖不足保留供人工判断。

验证：上述Go/race、CLI与前端检查，官方SARIF Schema及实际下载，受控IAB交互，冻结隔离v2逐项人工核对。限制：合成小样本、无代码执行，405未读下游及金融语义误报尚存。未创建PR、未合并、未部署；用户README.md与docs/images未纳入。

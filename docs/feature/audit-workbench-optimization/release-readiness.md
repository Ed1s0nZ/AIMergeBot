# R3 分支交付

## 当前增量：v49 工程通过，真实取证改善，复核结构失败仍待定位

33eb7ee/full186.615s/race62.495s/精确CI37459758978 success，独立schema/反馈切片APPROVE。原模型803实际保存双侧引用及两条跨项目来源关系，fresh claim完整取证并consistent/true；finding复核结构被拒且缺分类诊断，原失败历史保留，receipt incomplete。详见regression-v49-feedback.md；AER-001 open，未合并main/部署。

## 当前增量：v48 独立来源要求工程通过，真实效果仍未验证

2eb8daf把mandatory context ID清单与原fresh门禁共用，增加具体子断言条件提示，原预算/历史/公共shape不变。full172.055s、race12.568s、vet/diffcheck/精确CI37457629388 success，独立scoped APPROVE（484hash、目标Go48.723s）。真实803一次在primary关系目标字段为空的重复拒绝中耗尽15决策，独立阶段未执行；结果model_or_agent_failed，不能声称新提示真实有效。详见regression-v48-source.md。下一步关系字段错误反馈，AER-001 open；原失败保留，未合并main/部署。


## 前一增量：v47 原预算收尾已验证，整体仍incomplete

297e610主审共享原决策硬上限，并对合法提前final最多请求一次补录；候选、压缩、token/取消门禁保持。full205.620s、受影响race39.055s、vet/diffcheck、精确CI37455551443 success；独立scoped APPROVE/PFR-001 resolved（full96.142s、race23.639s、481文件一致）。真实原模型802/803各一次已保存实际BASE/HEAD引用及cited调用/部署关系；803初审全读三份PHP上下文，不再把可用部署映射说成缺失。fresh claim漏相关仓库被正确保留unknown；具体SQL载荷条件/full coverage仍需改善。详见regression-v47-finalization.md；AER-001 open，未合并main/部署，不承诺总体准确率。


## 前一增量：v46 PR上下文原子补录已验证，整体仍incomplete

c5aa4d7提供typed record_pr_context，仅明确更换PRContext/合并显式来源，不改原命题判断、计划、反证及历史；full156.137s、race60.011s+9.047s、vet/diffcheck、精确CI37450141111success，独立新契约delta scoped APPROVE（476文件一致），不批准整分支。真实原模型802/803各一次：803确实采用并保存关系，但双方引用仍漏、可用deployment映射漏读且主审提前终止；详见regression-v46-context.md。AER-001 open，下一步原预算内收尾控制，未合并main/部署。

## 前一实现：v45 命题复核、检查点和UI消费已完成切片验证

986b336接入实际调查复核SDK、finding优先同40次/60秒pool、fresh来源/独立模型/usage/SARIF；其full194.834s/CI37444342848通过但独立REQUEST_CHANGES发现EXR-001取消冻结检查点漏review。dfc6340以每次checkpoint副本投影修复，原ledger与成功final不污染、group在ID合并后处理；实际Store/worker/group SDK取消、恢复与fence回归通过。最终full134.292s/CI37445789011success、独立scoped APPROVE/EXR-001 resolved；不批准全分支。UI五态/初审命题文案/长ID/键盘focus与配置消费已实现，实际360px/fresh source及running/cancelled/failed历史空态验证见claim-verification-ui-qa.md；53331c8 UI独立scoped APPROVE、无确认finding，冻结464文件、15项SSR状态检查、10项浏览器产物和8项源码hash复核通过；精确源码CI37447550923已核对success。该批准只覆盖dfc6340→53331c8 UI/文案/生成资源，不覆盖整分支语义质量。原模型805一次known regression已在53331c8验证fresh true捕获首审rejected实际命题分歧，27.005s/116724tokens/0finding/incomplete；详见regression-v45-claim.md。802/803原模型各一次回归见regression-v45-paired.md：安全改善判断正确、跨语言风险定位正确，但结构化关系/双侧引用及条件措辞仍缺口。这不是整体准确率或blind验收，R3完整证据尚未证明，AER-001 open，无合并部署。

## 历史基础：814c60a（策略v44），仅契约

新增共享原40次/60秒复核pool及server-owned调查命题复核契约；模型写入注入清除、严格JSON和fresh固定来源校验已实现。独立检视发现compare_files误算BASE路径的CVR-001，已修复并以真实本地Git反例验证；最终全量Go platform110.823s、定向race2.700s、vet/diffcheck通过。独立契约切片re-review APPROVE/CVR-001 resolved，不批准整分支；报告见implementation.md。源码CI37441430651已核对终态success（Go/race、运维工具、依赖检查、前端与嵌入构建均通过）。

当前supplement仍只执行原finding复核，没有调查命题复核请求。runner、UI/配置/SARIF和原模型真实质量验证继续待做；不能把数据契约测试当语义判断改善，AER-001保持open。已推送codex/audit-quality-loop，未自动合并/部署。


## 历史增量：217db21（策略 v44），整体仍未完成

空/未知update ID错误增加本次audit已有ID的有界排序JSON及复制说明；不自动选择记录或按Claim匹配。原验证顺序、源码门禁、未知ID拒绝、ledger/history/pending保持；跨identity resolver继续拒绝。相关race4.648s、vet/diffcheck通过；独立切片COMMENT、无blocking，自行定向1.000s/race4.265s通过，非整分支批准。报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v44-217db21/PR_REVIEW_REPORT.md。最终full Go exit0 platform127.828s，源码CI run37437834569终态success。

真实v43-805原预算59222tokens：五项question未改写、拟提交完整双侧引用/跨项目关系，但update ID空，保存结果仍incomplete；详见regression-v43-recovery.md。当前v44仅旧请求离线feedback0.745s，准确列出inv-1且不改原记录/错误assessment，无新模型请求，5文件SHA核验一致。非真实v44采用或准确率证明；AER-001极性和必要来源记录仍open，未合并/部署。

## 历史增量：d8fff22（策略 v43），整体仍未完成

计划身份冲突错误列出所有冲突任务的原id/kind/question；JSON≤8000字节且≤8条，异常历史数据退回固定错误。schema明确question不可改写。门禁、来源、正常预算、失败历史和显式resolver不变，不自动判断命题。full Go exit0 platform124.978s、相关race3.675s、vet/diffcheck通过。源码CI run37436813700终态success；独立fresh-context切片COMMENT/无blocking，自行定向0.967s及19类旧新门禁接受/产物一致性overlay0.939s通过；报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v43-d8fff22/PR_REVIEW_REPORT.md，非整分支批准。

旧v42-805三次失败输入离线验证：新反馈准确指向p3/p5原身份，模拟只修身份后通过plan校验，错误的evidence_refutes_claim仍保留；原回执/ledger不修改，无新模型请求。离线0.845s，外部归档5文件SHA核验一致。非新模型效果或准确率证明，AER-001仍open，特别是兼容命题极性及BASE/HEAD结构化引用。未合并、未部署。

## 历史增量：b19ff46（策略 v42），整体仍未完成

模型update_investigation改明确claim_assessment有限枚举，隐藏顶层旧status、保留plan.status，兼容旧JSON和直接Go调用。矛盾新旧值/未知枚举拒绝；原提交trace与规范化状态分别留存，来源/计划/PRContext及8000字节门禁不变。本地全量Go exit0 platform137.600s、定向1.157s、race3.329s、vet/diffcheck通过。独立fresh-context切片COMMENT、无blocking，自行定向0.788s通过，非整体批准；报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v42-b19ff46/PR_REVIEW_REPORT.md。源码CI run37435634059终态success。

原模型同预算802真实回归：75067tokens、6轮、0findings，守卫改善命题正确supported并记录调用关系与上游未知；仍缺结构化BASE/HEAD链接，最终两条对应缺口保留、状态incomplete。只有一个已知样本，不能推广准确率或费用改善。详见regression-v42-assessment.md；后续同生产源码另跑803/805：803记录跨项目关系与条件风险，805摘要正确但计划身份改写遭拒且拟提交极性仍错；详见regression-v42-quality.md。AER-001仍open。未合并、未部署。

## 历史增量：83671ea（策略 v41），整体仍未完成

无finding调查也用共享PR recording gaps保留最终BASE/HEAD来源与关系缺口；metadata-only依固定来源分类，同步feedback/live/final，不制造执行路径。最终生产源码full Go exit0 platform111.423s、广泛定向race8.043s、追加SDK/worker/SARIF race4.447s、vet/diffcheck通过。独立切片COMMENT、无blocking，定向9.457s/自设计8类来源边界0.709s通过；非整分支批准。源码CI run37434193201终态success。

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

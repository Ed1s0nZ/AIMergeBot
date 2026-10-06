# R3 原确认范围完成审计 v50

权威合同requirements.md Confirmed R3，REQ/AC001–012和ASM001–005保持原范围。冻结功能源码01446573f8964ab90be86774718dc7b01e43d4c2，policy50，分支codex/audit-quality-loop。本文记录源、测试、真实评测和交付证据；不构成整分支合并批准，不承诺模型零错误、一般准确率或运行复现。

| 原合同 | 权威证据及已核查范围 | 验收结论及限制 |
|---|---|---|
| REQ/AC001 | Git文本工具无语言/parser准入；当前unknown-language边界测试，历史首次多语言source、当前TS→PHP读取 | 基础语言无关能力成立；专用分析器可选，不声称所有语言准确率 |
| REQ/AC002 | 固定授权/context批读/cache撤权/SHA隔离测试；v50-803正例双方源及PR锚点，v37-804参数绑定/v47-802修复负例 | 有界跨项目正负取证验收成立；805历史命题极性误判仍明确保留 |
| REQ/AC003 | v47-802实际caller/workflow与BASE、修复命题/来源关系；current803入口/guard/部署/SQL固定引用；group source locator/namespacing测试 | 可用来源遗漏已闭环，实际事实/未知有来源；摘要不替代证据 |
| REQ/AC004 | 五类risk_checklist反证且noneligible；802guard/804binding/805默认与范围防护识别 | 有界知识与防护负例成立；不存在模型完全正确承诺 |
| REQ/AC005 | v49/current803 BASE/HEAD、cited静态部署/请求字段与SQL源关系；v37-806排除无关历史sink；UI来源导航 | 静态逐边证据和PR归因验收成立；driver/runtime/载荷未知保留，不造完整运行链 |
| REQ/AC006 | source/四方面门禁、SDK预算/重复读/取消与补录停止；静态vs运行等级；current真实finding/claim及分歧记录 | 有界停止及验证等级成立；supported/full不等于复现，incomplete可来自真实未知 |
| REQ/AC007 | 实际RunDetail筛选/来源/阶段/缺口；v38历史浏览器及v45五态/360px/ReturnTab/source文本逃逸；10产物8源码hash现仍匹配，整个frontend/web相同 | 有界受控浏览器验收及当前消费成立；继承QA不是新生产E2E，不展示模型内部思维 |
| REQ/AC008 | 版本迁移/唯一赢家/冲突不改review或delivery测试；当前表单草稿/刷新baseline/409显式重提逻辑，历史实际交互 | 并发复核与草稿恢复成立；缺revision拒绝，调用方迁移有文档 |
| REQ/AC009 | DetailVersion/轻量status权限与重试变化测试；有界Git目录/SHA cache、context撤权重查；frontend变化后才拉full | 轮询/分页/cache授权机制成立；不外推总体吞吐或费用收益 |
| REQ/AC010 | 当前实际803 receipt经现BuildSARIF生成，官方2.1.0 schema+formats验证；HEAD gateway.ts:6、fixedSHA、claim review和等级；HTTP权限测试 | 位置/证据/等级导出成立；runtimeReproduced=false，无伪codeFlows、不自动上传 |
| REQ/AC011 | 首次隔离v34/v36独立truth/freeze；known v37/v45/v47/v49/v50标回归；归因/定位/链路/判断/证据/FPFN限制与usage、infra分别保留 | 评测体系与真实执行成立；无blind总体准确率，price未配金额未知，不要求全部completed |
| REQ/AC012 | 当前fullGo125.438s、race/vet、source精确CI全部stage；checkpoint/lease/fence/model前持久化/取消/恢复/重试/comment/migration回归，frontend类型/build/嵌入byte | 工程与历史/恢复验收成立；测试使用受控repo/store/mock，未发远端评论或部署 |

ASM001默认只读，不执行被审计样本或生成PoC；ASM002授权固定关联仓库、主PR归因，无新托管平台/账号接入；ASM003版本条件写入及历史读取/迁移保留；ASM004沿现有界面，分阶段覆盖全部原目标，无强制向量库或动态Agent；ASM005交付源码/文档/测试/分支，无自动main合并/部署，真实模型及未知成本如实记录。原工作区用户README/docs/images不属于本分支交付，未改动。

## 独立裁决及残余风险

诊断切片review-v50-diagnostics-0144657/PR_REVIEW_REPORT.md仅slice APPROVE；能力审计r3-quality-completion-v50-0144657/PR_REVIEW_REPORT.md为COMMENT、原范围有界验收证据充分，AER-001 resolved。原802来源遗漏、803关系漏记与必要负例证据均有真实后续证明。805旧事实真claim被rejected未修复，原receipt和fresh true/disagreed保留为AER-003 S3非阻塞语义残余，后续能力仍可优化；没有改历史、自动翻转判断或引入隐式零语义错误门槛。

工作台/API/可靠性报告r3-workbench-completion-v50-0144657/workbench-second-pass-review.md独立第二遍对原AC007–010/012/ASM003核查；产物有源hash/真实SDK及实际SARIF对应。新agent创建受平台thread limit限制，本次复用独立reviewer作second pass，明确披露，不伪称新fresh-agent或整分支批准。

## 验证与交付边界

源0144657 CI37461595471/job112262248575 success，包括full/race/vet、运维测试、可达依赖漏洞、npm audit/TS/build及嵌入应用构建。当前原模型803一次40.361s/223396tokens，19calls usage完整，fresh finding supported/full与claim consistent/true，运行未知仍incomplete；无API错误、没有结构拒绝，因此不声称诊断实际纠错收益或v49原失败原因已确定。完整receipt69hash/固定Git归档与regression-v50-diagnostics.md对应。

最终本文及交付文档为docs-only增量；源0144657验证不是后续文档HEAD的CI。最终文档HEAD检查、源码字节继承及远端分支核验见外部/Users/worker/.codex/evaluation-artifacts/aimangebot/release-finalization-v50/finalization.json，避免写入报告自身导致再改变被检视HEAD。代码/文档提交及分支推送属于原授权；没有PR发布、main合并或部署。

工作台独立目标Go9.960s、typecheck/build1.85s、三项dist与嵌入字节相同，实际SARIF官方schema/formats及10UI产物/8源码hash再次核实；能力独立当前目标Go23.293s、最新8bundle/25fixed source blob/7receipt引用资格核查通过。评测来源、继承范围及second-pass限制均写在外部报告，未以全量测试替代语义证据。

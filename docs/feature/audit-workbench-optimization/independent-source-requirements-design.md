# v48 独立复核来源要求与子断言覆盖

Workflow Gate / F0-F2：最佳实践优化原Confirmed R3，P10 backend反馈迭代，AC003/004/005/006/011/012；源399c7f6 clean、独立v47 scoped批准及真实regression-v47-finalization.md齐备。实现允许，无新产品选择、权限或运行代码；不自动合并main/部署。当前claim门禁已要求主PR双侧及每个固定context source，但prompt仅泛称relevant repositories，真实803仅主PR便给true被serverunknown。具体载荷虽未核实，finding reviewer仍full，属于语义遵从缺口。

模型内部User输入新增source_requirements：主PR双侧/canonical metadata原则、排序去重的context_repository_ids（直接固定policy，不按主审叙述选取）、fresh citations要求。payload与最终门禁共用同ID helper，避免声明和执行漂移；未新增HTTP/Store/导出/UI或权限形状。snapshot/locators/Claim仍User untrusted，System固定指令解释要求，不含路径/源码/旧判断；来源清单只指必需固定仓库，阅读和语义判断仍由原只读agent执行。true/false须有每个仓库fresh源及主PR差异，未读或不能证实返回unknown；至少读一份不等于完成契约取证，不放松原门禁。

primary与finding固定prompt明确具体示例/替代载荷/执行结果都是实际断言；静态代码能支持有条件风险，但不自动支持每个具体方式或成功效果。未知细节要从肯定叙述收回并明确条件；复核如保留unsupported子断言则partial，核心依赖未知inconclusive。提示不使用样例ID/文件/语言/特定SQL关键字或按措辞正则猜真值，不自动改首审history。现有claim_coverage门禁保持，新增提示不是语义保证，必须真实模型验证。

Maintainability Gate：claim_verification_agent143/verification_agent185行，claim source helper单职责固定snapshot provenance；low-medium，proceed narrowly，不先广泛重构。只对现有payload与共享gate做一处元数据helper连接，agent.go仅改固定prompt；无新模型轮次、自动prefetch或额外预算。原8复核decisions/每项15秒/finding-first共享40tools60秒/globaltoken、压缩/取消/固定SHA/撤权不变。

必要验证：实际SDK模型输入0与多个context ID稳定排序/去重，无旧status/evidence/reason泄漏，无System Claim/path提升；原fresh gate在缺源/旧ID/错误SHA/部分源继续unknown或拒绝；实际SDK正例fresh双侧+context accepted、漏context真实降unknown；主审/复核已有完整/partial/unknown/取消/压缩/group/worker回归，race/full/vet/精确CI。冻结当前source要求/判断输入后独立审计；原803一次known regression观察初审条件表达、fresh claim必要contract source引用与full subsidiary一致性，不盲重跑或改truth/预算，不强求completed。失败/unknown/cost照实留档；通过prompt契约不能声称真实准确率。回滚payload/prompt/helper连接，旧门禁继续。

F4/F5：内部source_requirements已加，requiredClaimContextIDs供payload/原gate共用，排序去重且空表JSON[]；原policy不改、非法ID也不隐藏。固定prompt明确每仓库fresh与caller/mapping语义、具体子断言/部分覆盖；原记录、公共shape和预算不改，policy48。新真实SDK正例fresh批量BASE/HEAD+context为consistent，漏context即server unknown；输入metadata可见、主审判断不泄漏、不提升System、原40pool实际计费和原调查字节保留。初次fixture误按外层tool次数计费失败（complete剩36/missing37），按真实批次+两个child既有规则校正期望4/3，并未改变计费实现。相关fresh gates/独立claim/finding/四维覆盖/压缩/group cancellation race12.568s、vet/diffcheck通过；full exit0 platform172.055s；精确CI/独立freeze/原803真实效果仍待，不把输入契约当语义证明。

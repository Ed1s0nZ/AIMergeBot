# v49 关系字段反馈真实回归

源码33eb7ee0f8fcc5b4a6942635acd093523641b84d，策略eino-audit-contract-v49。原deepseek-chat/temp0.1/15steps80tools240秒，803仅一次known regression；没有API/余额错误，未改truth、预算或执行样本。

工程：full Go platform186.615s、相关race62.495s、vet/diffcheck通过；精确CI37459758978 success。独立review-v49-feedback-33eb7ee/PR_REVIEW_REPORT.md仅公共关系schema/字段反馈切片APPROVE，488文件hash匹配、独立检查102.292s。离线原v48解析后空目标输入验证1.042s通过，仅证明字段错误/原子性及显式修正契约，不证明模型质量。

真实CLI exit0，receipt仍incomplete；44.577秒、17次模型调用，195889tokens（input187419/output8470），usage完整、未知调用0；价格未配置，金额未知。primary9次/151011tokens，finding5次/31223tokens，claim3次/13655tokens。

初审读取固定gateway BASE/HEAD及授权context2的query.php、connection.php、deployment.conf；保存双侧引用、两条cited来源关系，正确定位HEAD gateway.ts:6。条件风险描述保留认证、驱动和运行未知，没有UNION/堆叠语句的确定性承诺。首次record_hypothesis缺top-level来源ID失败，后续显式记录成功但未退休旧失败，历史及coverage保留。本次无关系字段错误，不能证明模型使用新纠正反馈，也不能把随机差异归因于v49。

独立claim实际重新读取BASE/HEAD及全部三份上下文，返回consistent/true；条件性could enable命题有源代码支持，运行/驱动/凭据未知保留。独立finding也读取这些来源，但parseVerification拒绝最终结构，状态unavailable、无接受来源ID。当前parse失败分支未记录有界诊断或原始返回，归档无法进一步确定是JSON、枚举、长度还是checks约束；不猜测、不放宽门禁，不视为安全结论。

下一步补齐隐私安全的结构失败分类诊断及契约测试，再依据证据决定是否需要模型复测。整体R3/AER-001仍open；不是blind验收或总体准确率，不自动合并main/部署。

原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v49-feedback-20261006，sha256-manifest.json逐项核验，0700/0600；无配置/凭据副本。

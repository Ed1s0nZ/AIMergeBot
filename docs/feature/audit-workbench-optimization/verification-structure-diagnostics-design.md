# 独立漏洞复核结构失败诊断

## F0/F1 与工作流门禁

延续Confirmed R3 REQ-006/012及ASM-005，分支codex/audit-quality-loop，5eeac0d工作树干净。P10反馈迭代；已具备requirements、既有复核契约及v49实际receipt。允许实施，本次没有新产品/API决策需要确认。v49真实parse失败只有通用unavailable，trace没有结构失败分类，原始模型正文未保存，无法事后判断具体原因。

## F2 消费者契约及设计

在parseVerification失败时追加既有ToolTrace model_response、stage=verification。Error仅服务器有限分类；Output复用responseDiagnostic，只包含code、字节数、JSON形状等有界元信息，不保存模型正文、字段名、来源ID、provider错误。分类覆盖JSON语法/类型/未知字段、超预算、尾随数据、非法verdict/coverage、explanation/limitation、check数量/结构/来源链接；未知错误返回固定fallback。

保留parseVerification、validateVerificationChecks原错误文案与所有接受/拒绝条件；诊断不纠正、不重试、不增加模型或工具调用，不接受失败来源，不改变主审/复核状态及原历史。已有公共ToolTrace形状不扩字段，策略版本v50用于来源区分。原v49具体失败原因仍未知，不凭新代码反推旧receipt。

## 可维护性门禁

verification_agent.go185行、finding_verification.go151行、verification_aspects.go75行，单一复核职责；风险low，narrow_fix，无需先重构。新增有限分类纯helper放verification_diagnostics.go，编排仅追加诊断。既有parser/source/SDK/checkpoint测试覆盖拒绝门禁。

## 验收及验证

畸形/私密JSON和非法结构：保留拒绝、分类准确、无正文/私密字段/ID泄漏；合法响应不受影响。实际SDK负例证明诊断进入trace且原finding、unavailable、coverage及checkpoint保持。Go相关race/full/vet/diffcheck；源码精确CI及独立检视后再决定是否值得原模型复测。真实语义质量仍由实际证据验证，不以分类测试代替。无main合并/部署。

## F4/F5 实现及当前验证

有限分类helper及parse失败trace已实现，既有parser/check规则和公共结构保持。实际SDK invalid模式断言json_syntax元信息进入返回trace及最终checkpoint；正常结果/原finding/unavailable/coverage继续原断言。新parser负例覆盖全部新增服务器分类和私密正文/字段/ID不回显。首轮截断JSON测试失败（实际为io.ErrUnexpectedEOF），现明确归入json_syntax，不修改parser。修正后目标Go1.018s、相关race5.527s、最终SDK/checkpoint race4.649s通过；vet/diffcheck通过。首次full/race在修正前启动，race失败已保留，full仍待终态及最终源码重跑；不能当当前全量通过。精确CI、独立review和真实模型诊断效果待验证，无整体完成声明。

## F5/F6 最终证据

功能源码0144657最终full Go125.438s/race5.527s及最终SDK4.649s/vet通过；CI37461595471精确head全部success。独立诊断切片APPROVE（491hash、172main/8delta分类、race4.048s/隐私2.420s）。原803一次回归19calls/40.361s/223396tokens，必要fresh finding/claim来源与静态判断有效，但没有结构错误，故真实诊断纠错收益与v49原具体原因仍未证明；不重跑追求该错误。真实未知保留incomplete。逐原R3能力/工作台审计完成见completion-audit-v50.md，805残余语义分歧保留，不自动merge/deploy。

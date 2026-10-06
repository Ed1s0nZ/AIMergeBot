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

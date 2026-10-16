# N1a：Scope v2 codec实施记录

2026-10-16；codex/sidebar-workflow；输入为technical-review-2026-10-16-native-execution-6.md @8459412的S4内部实施Pass。

## Workflow / Lifecycle Gate

- 阶段P7/P8、F4；Confirmed需求及scope v2/endpoint合同已审。允许内部codec实现，不开放生产factory/绑定入口/C3；N1整片仍须schema3/ledger等后续实现。
- 用户场景：新普通GitLab任务需准确表达HTTP/path-prefix；旧scope v1语义保留。此片完成后仍无v51 producer，不称用户功能完成。
- 文件边界：repository_scope.go/JSON decoder只委托；新repository_scope_endpoint.go专管endpoint/profile/key/equality；测试独立文件。无新公开路由、设置或外部写。
- 验收：v1字节/校验语义不变；v2严格union/深拷贝、endpoint key不混前缀、不改wire URL；畸形/重复/null/错profile拒绝；所有scope仍被现有执行guard拒绝。

## Maintainability Gate

medium风险：原scope184行、JSON170行，分别结构校验和严格解析，两类职责，没有>800行热点。调用耦合为policy clone/digest、Store与guard；不向Runner/Settings加分支。先重构不必要，allowed adapter_extraction/narrow_fix；地址逻辑独立，新指针必须深拷贝，reference比较改成值语义。测试覆盖现有strict JSON、store guard与新v2反例。

## 验证与交付

**状态：生产尝试已撤回，未交付codec**。首轮专项因旧测试把version2当未知版本失败；调整未知值后，wire fixture发现SDK会重写部分prefix escaping，与作者的逐字节wire假设不符。本轮所有internal/platform未提交修改均已撤回，`git diff --exit-code -- internal cmd frontend` exit0；不把局部尝试算功能完成。

改为独立ignore探针scripts/probes/legacy_sdk_endpoint.go，以当前SDK和本机TLS服务器验证8种前缀。最终`go run -race`及`go vet` exit0：encoded dash/slash与literal产生不同BaseURL但相同wire；path大小写不同；encoded dot与root仍wire不同。最初对dot等价的猜测也被否决，因此不能手工path.Clean；最终逐case用SDK NewRequest有效前缀与实际GetProject wire核对。

作者已据此修订EndpointKey与material的EffectiveAPIBaseURL，需再审后重新实施。未跑未实现codec的完整测试，也未宣称原Pass能批准这个改变后的身份算法。上个代码HEAD b1f5687e74a71d8e21d83083f41d59a80ab97856的CI38854477051已查询completed/success，允许新probe后续推送。未发送真实模型/托管服务/通知，不合并main。

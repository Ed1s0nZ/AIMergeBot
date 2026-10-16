# N1a：Scope v2 codec实施记录

2026-10-16；codex/sidebar-workflow；当前输入为technical-review-2026-10-16-native-execution-7.md @3671bc0的S4内部实施Pass，取代第一轮使用的旧地址假设。

## Workflow / Lifecycle Gate

- 阶段P7/P8、F4；Confirmed需求及scope v2/endpoint合同已审。允许内部codec实现，不开放生产factory/绑定入口/C3；N1整片仍须schema3/ledger等后续实现。
- 用户场景：新普通GitLab任务需准确表达HTTP/path-prefix；旧scope v1语义保留。此片完成后仍无v51 producer，不称用户功能完成。
- 文件边界：repository_scope.go/JSON decoder只委托；新repository_scope_endpoint.go专管endpoint/profile/key/equality；测试独立文件。无新公开路由、设置或外部写。
- 验收：v1字节/校验语义不变；v2严格union/深拷贝、endpoint key不混前缀、不改wire URL；畸形/重复/null/错profile拒绝；所有scope仍被现有执行guard拒绝。

## Maintainability Gate

medium风险：原scope184行、JSON170行，分别结构校验和严格解析，两类职责，没有>800行热点。调用耦合为policy clone/digest、Store与guard；不向Runner/Settings加分支。先重构不必要，allowed adapter_extraction/narrow_fix；地址逻辑独立，新指针必须深拷贝，reference比较改成值语义。测试覆盖现有strict JSON、store guard与新v2反例。

## 首轮尝试（3671bc0历史记录）

**状态：生产尝试已撤回，未交付codec**。首轮专项因旧测试把version2当未知版本失败；调整未知值后，wire fixture发现SDK会重写部分prefix escaping，与作者的逐字节wire假设不符。本轮所有internal/platform未提交修改均已撤回，`git diff --exit-code -- internal cmd frontend` exit0；不把局部尝试算功能完成。

改为独立ignore探针scripts/probes/legacy_sdk_endpoint.go，以当前SDK和本机TLS服务器验证8种前缀。最终`go run -race`及`go vet` exit0：encoded dash/slash与literal产生不同BaseURL但相同wire；path大小写不同；encoded dot与root仍wire不同。最初对dot等价的猜测也被否决，因此不能手工path.Clean；最终逐case用SDK NewRequest有效前缀与实际GetProject wire核对。

作者已据此修订EndpointKey与material的EffectiveAPIBaseURL，需再审后重新实施。未跑未实现codec的完整测试，也未宣称原Pass能批准这个改变后的身份算法。上个代码HEAD b1f5687e74a71d8e21d83083f41d59a80ab97856的CI38854477051已查询completed/success，允许新probe后续推送。未发送真实模型/托管服务/通知，不合并main。

## 第二轮实现（当前）

- 新增repository_scope_endpoint.go：legacy endpoint联合结构、SDK原base/有效请求前缀、稳定EndpointKeyV1、reference值比较及深拷贝。scope v1保持原origin比较与序列化，v2使用实际wire身份比较；不改请求地址、不创建网络连接。
- 严格JSON分版本检查；legacy_endpoint只允许v2且仅legacy credential使用，缺失/null/重复/未知/大小写别名、错误profile/base/代次均拒绝。仓库名语法抽成共用纯函数，保留长度检查在Split前。
- 新增真实TLS wire、混合credential同仓ACL冲突、逐级深拷贝、v1固定JSON哈希/roundtrip测试。旧unsupported admission/execution测试扩展为v1 integration、v2 integration、v2 legacy：均零run/event写入、零repo/model调用。
- 没有v51 producer、账本、schema3迁移、profile v2实际transport或C3装配。当前schema2不变，没有新scope写入现有生产DB；schema3必须在新v51入队前完成。N1a是内部codec片段，N1及完整原生能力尚未交付；现有501及执行guard不变。

最终代码在Go1.27.2、Darwin arm64、Xcode编译器PATH下独立验证：

- `go test ./...` exit0，最终internal/platform为106.633s，其余包通过/无测试。
- `go test -race ./internal/platform -run '^TestRepository(Scope|EndpointKey|Binding|Management)' -count=1` exit0，35.422s，无race；包含新的v1固定JSON身份、v2联合结构与TLS wire测试、三种scope的真实Store/Runner拒绝路径，以及管理只读回归。
- `go vet ./...`、`git diff --check` exit0。测试只用临时DB、受控TLS与模型stub；未用真实托管账号/模型/通知。

不复用首轮撤回代码的结果。无前端或资源文件变化，不运行无关Vite构建。后续仍须ledger、全部session/consumer、恢复与UI端到端证据，不能用此片代替完整需求验收。远端精确HEAD CI另行记录，不能拿上一个probe的成功当当前代码成功。

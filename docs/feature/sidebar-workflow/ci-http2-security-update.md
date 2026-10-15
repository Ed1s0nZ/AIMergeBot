# CI可达HTTP/2依赖漏洞修复

## F2：现状与合同

2026-10-15，codex/sidebar-workflow。输入为最佳实践优化、已有CI门禁及S1精确HEAD949c13a的CI38740692571真实终态failure。Go全测试/race/vet与Operations tooling tests均success；Reachable Go dependency vulnerabilities失败，Frontend/Embedded构建skipped，不把整个CI称通过。失败日志保存在/tmp/aimangebot-s1-ci-38740692571.log。

扫描报告x/net v0.57.0存在5个可达漏洞，修复均为v0.60.0：GO-2026-6617/6612/6611/6610/6603。已有upstreamTransport的http.Transport调用在可达链中；不把扫描告警误称为scope单元测试失败，也不凭可达扫描声称已被利用。

Workflow Gate：P10维护/CI修复，允许最小依赖补丁；不新增用户需求、API/DB字段、scope producer或通知权限。消费者为已有HTTP客户端/服务器及其协议测试。选择固定x/net v0.60.0及Go最小版本选择所必需的传递依赖，不做无差别升级、不关闭govulncheck、不扩大allowlist或禁用TLS。先检查上游module的go版本与依赖，再执行；保留go.mod现有最低版本，若上游迫使提高需另明确兼容影响。

Maintainability：go.mod/go.sum纯依赖清单；生产传输包装不变，不新增绕过分支；allowed narrow_fix，无需重构。schema preflight修改当前仍单独在工作树且已本地全Go/race/vet/build通过，不将它和此次CI根因混淆。新验证在最终合并工作树上进行，精确生产HEAD重新验证整个CI。

来源：[GO-2026-6617](https://pkg.go.dev/vuln/GO-2026-6617)、[6612](https://pkg.go.dev/vuln/GO-2026-6612)、[6611](https://pkg.go.dev/vuln/GO-2026-6611)、[6610](https://pkg.go.dev/vuln/GO-2026-6610)、[6603](https://pkg.go.dev/vuln/GO-2026-6603)。上游报告同时标记标准库Go1.27.2/1.26.9修复；本地默认Go1.27.1，修复后的扫描须用匹配CI已修复工具链，不能因此误称x/net补丁没有修复或隐藏标准库发现。工具链选择仅本次命令环境，不修改用户全局go env。

## F3：实施与验证顺序

F2 af102c9已推后规划。上游v0.60.0的go.mod仍要求Go1.26.0；最低Go版本不变，MVS所需最小依赖为crypto0.57/sys0.48/text0.42（term0.46仅在实际module graph需要时出现），不运行go get -u。

固定`go get golang.org/x/net@v0.60.0`，审查go.mod/go.sum实际diff；以命令环境GOTOOLCHAIN=go1.27.2执行govulncheck@v1.8.0 ./...、全Go/vet/build及scope/guard/HTTP协议专项race。不得以仅升级清单断言漏洞消除；若发现其它可达项，按实际输出另处理，不修改CI检查规则。没有前端生产源变更，当前schema guard独立检查已全Go102.386s/race6.273s/vet/build通过，依赖变更后完整回归再跑。

最终记录原失败run及新依赖和工具链、真实扫描/测试结果；schema preflight与依赖修复可分别形成生产提交，再一并推送触发精确新HEAD CI。S1原run已经failure终态，不重跑原脆弱依赖或取消覆盖它。新CI要验证Go/运维/漏洞扫描/frontend/embed全部实际终态；不把success的部分步骤称整体通过。

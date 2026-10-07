# GitHub 默认 Git 工具链：网络与凭据验证

## 门禁

基线f64e262，分支codex/sidebar-workflow；输入Confirmed requirements.md、github-provider-design.md与github-diff-design.md。F2设计研究，未进入F3生产实施。完整REQ/AC、两平台/fork/跨项目与默认工具范围保持；用户“没想清楚先不实现”继续适用。仅验证受控原型并收敛方案，无生产源码、绑定数据、公共API/配置或默认开关变更。

先前的具体疑问：Git网络无法直接使用Go HTTP transport；prepareGitSources当前共用单token；为了完整补丁重新写算法或只审计compare的300项，均不能直接满足目标。目标是复用成熟固定对象工具，同时明确每来源授权、网络、凭据和预算。

## 官方机制与版本

[Git 2.37发布说明](https://github.com/git/git/blob/v2.37.0/Documentation/RelNotes/2.37.0.txt)引入http.curloptResolve；[Git 2.39配置文档](https://git-scm.com/docs/git-config/2.39.0)规定HOST:PORT:ADDRESS格式以及按完整URL路径边界匹配HTTP配置。由本程序先解析并检查IP，再把核准地址交给Git，可避免子进程另行解析未核准DNS结果；仍须保持原URL主机名和TLS校验，不把URL换成IP。

[Git 2.39.5实现](https://github.com/git/git/blob/v2.39.5/http.c)将该配置应用到libcurl CURLOPT_RESOLVE；这是支持依据，不证明任意旧Git/自定义构建都有该能力。旧版本可能忽略未知配置，因此新路径必须在网络访问前做版本/能力门禁，不能在不支持时降级为普通DNS。现有Dockerfile安装Git；本轮另核对已有aimangebot:git-investigation镜像实际为linux/arm64、Git 2.39.5，不声称重建或部署了当前HEAD。

HTTP Basic可按来源分别使用token作为密码：[GitHub PAT文档](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)说明认证依据token，而非用户名；[App安装认证文档](https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation)规定x-access-token用户名与Contents权限。选用此用户名不等于已实现App token获取/刷新；当前已保存token的生命周期仍按集成版本执行。token只进子进程环境的URL路径限定extraHeader，不放URL/argv/配置文件或报告。

另检查了可能产生二次地址访问的机制：[packfile URI文档](https://git-scm.com/docs/packfile-uri/2.38.0)默认fetch.uriprotocols为空；[bundle URI配置](https://git-scm.com/docs/git-config/2.50.0#Documentation/git-config.txt-transferbundleURI)默认关闭。[Git 2.39.5](https://github.com/git/git/blob/v2.39.5/http-walker.c)与[2.50.1](https://github.com/git/git/blob/v2.50.1/http-walker.c)的dumb HTTP alternate校验均受http.followRedirects控制，非always时拒绝alternate。新生产路径应显式禁止这些扩展，不继承仓库/用户配置；不只测试普通302。

## macOS 完整受控协议原型

2026-10-08上海，Git 2.50.1 Apple Git-155；脚本/tmp/aimangebot-git-network-research/probe.py。临时目录创建target/source/client bare仓库，合成blob/tree/commit并设置独立refs；所有对象由Git plumbing生成，不checkout、不执行来源代码。两个来源同主机、不同仓库路径，各自独立合成凭据。临时CA只用于受控TLS，证书SAN=audit-git.invalid；私钥未输出，所有临时文件、线程及server在finally清理。

通过localhost的git http-backend提供真实smart HTTP GET/POST，子进程固定地址映射、关闭proxy/redirect/global config/helpers/hooks，TLS验证保持true。两个fetch均请求完整SHA，no-tags/no-recurse-submodules/depth=2；成功后cat-file确认两个commit，并验证固定差异包含-base/+head。POST为upload-pack的只读获取，不执行远端写入。

| 观察 | 结果 |
| --- | --- |
| 同主机下两来源 | target与source各GET/POST一次；各自仅收到正确路径限定凭据，Host保持audit-git.invalid及原端口 |
| 固定对象 | 两个实际合成commit成功获取并可按完整SHA diff |
| 不提供地址映射 | .invalid主机不能到达fixture，失败；没有将普通DNS当成功路径 |
| 错误TLS主机名 | 即使固定到同IP，证书不匹配时HTTP handler收到0个请求 |
| 错用目标凭据请求来源 | 服务拒绝，1个明确不匹配请求；没有借同主机共享凭据 |
| 302重定向 | Git失败，目标trap请求数0 |
| dumb HTTP http-alternates | 实际收到alternate地址但未跟随，trap请求数0；使用新空client避免已有对象跳过此路径 |
| packfile URI | 服务实际advertise packfile-uris，客户端未请求该协议，trap请求数0 |

负例保留：初次packfile URI夹具使用旧两字段配置，Git 2.50 server失败；核对[对应源码](https://github.com/git/git/blob/v2.50.1/builtin/pack-objects.c)后修正为object-hash / pack-hash / URI三字段，完整重跑通过。这是研究夹具错误，不是本项目生产失败。没有放宽正式错误处理或取消TLS来通过验证。

## Linux 运行环境验证

本轮使用已有aimangebot:git-investigation镜像，--rm、--network none，挂载只读的合成Go transport probe；容器退出已清理。Go stdlib在容器内部创建临时TLS服务，无新增软件包或镜像构建。Git 2.39.5实际接收地址固定配置，在两个同主机路径收到正确独立凭据；target/source各有2个HTTP请求（refs及HEAD）。错误证书主机名在HTTP之前被拒绝，302的trap请求数0。

此Linux测试只验证transport和合成refs，没有fetch真实commit、dumb alternates或packfile URI集成。对应完整对象与扩展协议证据来自上方macOS；不能混称两个平台所有场景均通过。初轮Linux夹具错误地断言总共2个请求，因Git另读HEAD而失败；改为每来源存在且所有请求凭据/Host正确，重跑通过，不修改transport策略。

## 默认方案收敛

推荐默认GitAudit开启时采用“固定GitHub元数据＋每来源受限Git fetch＋既有GitRepository工具链”。默认模式直接复用固定Git对象的完整raw metadata/patch/search/history/CODEOWNERS能力，不强制先走API两树再重复遍历，也不再为默认路径新写diff算法。GitHub API compare负责固定merge-base关系，而不是提供默认完整文件清单。

GitAudit关闭时继续规划API固定对象/完整两树与明确patch覆盖限制，不偷偷运行Git。此范围保持，不用受限API模式代替默认工具链。no-index原型可作补丁备选证据，现阶段不作为默认路径或可交付功能。

拟议操作内对象（不持久化token）：

| CloneSource字段 | 合同 | 创建/失效 |
| --- | --- | --- |
| internal_project / repository_id / full_name | 来自已授权绑定及独立远端身份核对 | factory创建，不能由source clone URL授予权限 |
| binding/integration revision | 正数、与冻结policy一致 | 每来源访问前复查，入队和工具操作继续复查 |
| fixed_commit | 完整有效SHA，仅该SHA fetch | 不存在/force-push后不可取时明确不可用，不用分支代替 |
| canonical_clone_url | HTTPS，无user/query/fragment；与已验证身份和支持origin映射严格相符 | 元数据核对后生成，不任意follow clone_url |
| approved_addresses | 非空、由已有IP规则检查全部DNS结果后冻结；不使用带过期“+”映射 | 每次远端操作重新解析/核准，不让Git另行选择未核准IP |
| token | 仅对应来源冻结集成的内存值 | 只送该fetch环境；不复用target token推定source授权 |

GitHub.com的api.github.com→github.com映射与标准Enterprise /api/v3→同主机clone需要在factory合同明确；其它API根不猜clone origin。loopback/metadata/link-local等仍按既有allowedNotificationIP规则拒绝；本原型直接构造localhost仅为受控测试，不是允许生产loopback，也不修改allowedNetworks。

生产helper应逐来源独立生成环境；关闭外部proxy/redirect、packfile URI/bundle下载、helpers、submodule递归、LFS/checkout及用户配置。已有workspace总预算/超时/cleanup继续适用，新的所有source总预算、版本能力、IPv6/多地址、撤权与取消仍需专项证明。原GitLab/CLI路径保持兼容，不在本研究中无端改变其认证语义。

## 仍未满足的开放前置

默认方向已收敛，不等于F2整体完成：完整Snapshot与base/source ACL顺序、clone mapping/版本门禁、每来源捕获与共享预算/取消、API关闭Git模式、rename/metadata关联、统一factory及Submit/run/retry/context/poll/publication、UAR-001绑定恢复仍需合同及集成验收。没有真实私有GitHub认证、App刷新、Enterprise部署或全部IPv6/配额/竞态组合验证。禁止将此原型单独开放为GitHub支持；不修改确认需求，无main合并或发布。

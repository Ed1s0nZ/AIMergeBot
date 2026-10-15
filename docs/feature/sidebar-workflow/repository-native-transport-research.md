# S4前置研究：原生Git网络与整次审计预算

2026-10-16，codex/sidebar-workflow，F2研究；承接Confirmed requirements及repository-recovery-implementation-plan.md的S4。目的为闭合原生factory依赖，不缩减默认Git工具链或改成永久API-only产品。尚不是完整F3实施许可。

## Workflow / Maintainability Gate

用户目标：最佳实践优化；没想清楚的先不实现。当前P6研究/验证，backend跨provider网络和预算。已有scope/身份/凭据协议、恢复API/UI及只读GitHub对象client；缺少具体受限Git取数、共享预算、API双树比较及完整producer/consumer装配。Implementation allowed：生产代码no；隔离本地协议探针yes。只在临时目录生成合成仓库、httptest服务和凭据，不读取用户仓库内容、不调用实际模型/平台、不增加产品入口。runner/git_repository已有编排/IO多职责，不继续塞新分支；未来拆factory/session/transport/ledger/provider适配。本片不更改这些生产文件。

## 当前事实

- GitHub client已有每client 128次/32MiB读取限制，tree/object另有上限；多个client各拿一份不能算整次审计共享预算。
- gitCommand已禁止全局配置、hooks、外部diff与重定向，逐命令30秒/输出8MiB；prepareGitSources仍由Git/libcurl直接访问远端，把同一token放进子进程环境，并按单工作区扫描磁盘。不能据此声称Git与API沿同一DNS/网络授权或整次审计资源账本。
- GitHub公共API的origin与Git HTTPS origin不同；Enterprise/api/v3与GitLab还须核对clone URL路径/身份，不能直接信任远端任意URL或把API token发往新主机。
- GitHub compare文件列表有300上限，不能把compare成功或翻页当完整变更覆盖。API模式仍需固定两棵完整树、可追踪覆盖不足，不得偷偷执行Git。

## 候选方案与待证实点

候选受控smart-HTTP转发：Git只连回环listener；逐仓远端token留在Go HTTP侧，转发只接受info/refs的upload-pack discovery及git-upload-pack请求，禁receive-pack/dumb任意文件。逐次派发、消费与返回到工具前复查原scope/ACL/lease/凭据；同一ledger限制请求、上传/消费字节和所有源/上下文的工作区资源。

本地访问凭证只给Git子进程的受限环境/header，不能放URL/argv/FETCH_HEAD；它只授权该次本地转发，绝不是远端token。回环listener本身不证明本机多用户隔离；生产设计仍须明确同OS用户与凭证读取边界。每个route固定一个已授权仓库，不能成为通用HTTP代理。默认HTTPS远端TLS与现有受限dial保持，禁止重定向/任意Host/header透传；Git protocol v2 header要明确处理。

探针需用真实git fetch与git-http-backend，在独立合成TLS upstream验证：两仓不同凭据/对象；v2协商；同ledger不因换route重置；无效capability、receive-pack/未知path零上游请求；重定向零目标请求；撤权后零下一请求；body超限后停止，不宣称能撤回已派发字节。成功只证明协议候选，不证明生产factory、DNS策略、真实GitHub/GitLab身份或进程资源隔离。

仍需闭合：共享各类预算的准确数值/计量点、Git pack磁盘/内存边界与宿主资源配额、>300文件API双树diff及rename/metadata语义、fork固定SHA/merge-base验证、origin映射、正式v51 scope准入和全部消费者。未明事项不以猜测落生产代码；不能移除现有拒绝guard换取“可执行”表象。

## 资料与适用范围

- [Git smart HTTP协议](https://git-scm.com/docs/http-protocol)：fetch discovery和upload-pack是明确的请求协议；本片只借此设计有限转发入口，不实现通用Git服务器。
- [git-http-backend](https://git-scm.com/docs/git-http-backend)：官方CGI支持smart HTTP及v2协商，可作为合成远端。探针只导出临时仓库，禁receive-pack。
- [git-config](https://git-scm.com/docs/git-config)：HTTP header和redirect行为是独立配置；将远端授权移出Git是本项目候选设计推论，不是官方默认安全承诺。
- [GitHub compare commits](https://docs.github.com/en/rest/commits/commits#compare-two-commits)：只把merge-base等固定观察用于快照，不能从受限files字段证明完整覆盖。

记录实际探针结果后再决定是否采用。无main合并/部署；完整目标保持进行中。

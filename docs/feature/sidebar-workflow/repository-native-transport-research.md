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

## 2026-10-16协议探针结果

实际执行`GOTOOLCHAIN=go1.27.2 go run -race scripts/probes/git_http_bridge.go`，exit0，无race报告；Git 2.50.1 (Apple Git-155)。最终JSON保存在/tmp/aim-git-bridge-probe.json；可复现代码与边界见scripts/probes/README.md。

两个独立仓库的v2 fetch和一个v0 fetch共7次上游请求，读取到各自实际blob；另一次header过滤请求后累计8次。不是只看请求header：探针还观察实际v2 advertisement。逐仓凭据错误0；caller Cookie/Proxy-Authorization不向上游透传，Set-Cookie不回传；无效本地凭证、push/未知/dumb路由零上游请求；耗尽同一ledger后换仓库仍零新增上游请求；重定向目标请求0；discovery期间撤权后无下一请求；16字节实验上限使fetch失败。审计侧Git环境/临时工作区无上游合成token。CGI夹具验证HTTP凭据后也去除Authorization，再运行本地git-http-backend。

首次夹具seed本地push被隔离的file协议规则拒绝，只对创建合成源仓库的那次fixture复制显式允许file；审计侧fetch不继承此例外。探针不是生产实现，未调用或更改原有gitCommand/factory/Runner。race通过只覆盖本次实际HTTP/进程场景，不代表已证明并发边界预算。

### 不能从探针推导出的结论

- byte ledger是实验性的读取后记账，不能证明并发读取硬上限；未来必须明确原子预留/退款、超限导致的覆盖不足和所有API/Git消费者共享语义。
- 禁止HTTP redirect不等于禁止Git协议中所有其他URI出口。还需验证[Git v2扩展](https://git-scm.com/docs/gitprotocol-v2)的packfile/bundle URI等行为；[bundle配置](https://git-scm.com/docs/git-config)可能触发额外对象下载。普通CGI夹具未覆盖恶意扩展，不把“Git只经过桥”当已证明的全称断言。
- 仍未证明宿主进程内存/磁盘/CPU硬限额、真实DNS/CIDR/代理策略、GitHub/GitLab服务端兼容及fork固定观察；也未闭合API双树diff或正式scope装配。保持生产guard。

## 成熟实现替代方案：尚不选型

不能因为CLI桥探针可行就默认自建桥。已查阅[go-git扩展文档](https://github.com/go-git/go-git/blob/main/EXTENDING.md)与[options定义](https://github.com/go-git/go-git/blob/main/options.go)，main/v6已有自定义HTTP与逐操作ClientOptions，且文档描述SHA-256支持；应对“库负责受控fetch→现有Git仅做本地工具”做同夹具比较。不能沿用“go-git只支持SHA-1/只能全局注册transport”的旧印象来排除它。

[官方release列表](https://github.com/go-git/go-git/releases)本次查到v6.0.0-beta.1为Pre-release、v5.19.3为稳定线；main文档不能直接等同已选定版本的API。下一步固定候选tag，比较逐操作HTTP/取消/预算、shallow/fixed SHA、对象格式及现有本地工具兼容，再评估依赖与维护成本。不在主go.mod中试装依赖或先上线未经比较的方案。这是新增证据改变后的下一动作，不构成S4 F3 Pass。

上一代码59005b1的CI 38772237718已实际completed/success。此轮只有研究文档和隔离probe，不宣称新增生产能力；后续仍按完整S4–S6推进。

## 提交和验证追踪

研究入口dddb479、探针与结果6460c069f56712d2cba35b2c5d55469b3abefe1a均已推送。`go list ./...`实际确认probe未进入任何正常Go package；本轮未重跑未变化的生产代码全量测试。探针代码对应[CI 38774775279](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/38774775279)本轮重新查询已为completed/success，headSha精确匹配6460c069f56712d2cba35b2c5d55469b3abefe1a；此结果不覆盖后续扩展探针。

## 固定版本库方案对照：实验输入

2026-10-16，继续F2研究。固定go-git v6.0.0-beta.1，不以main文档代替该tag源码。比较取数层，不替换语言无关审计或现有本地Git工具：库通过每操作HTTP client下载到临时bare对象库，之后由实际Git读取/历史/差异工具验证兼容。脚本放独立probe module，主go.mod/go.sum、生产Runner/factory均不改变。生产实施仍不允许，隔离研究允许。

同样使用合成TLS git-http-backend及不同仓库的不同token，验证准确SHA、shallow、并发逐操作client隔离；禁重定向、取消/撤权后结果不交付、共享请求/字节限制、SHA-1及SHA-256对象与本地Git互操作。记录真实不支持/失败，不靠换成默认分支或移除认证来跑通。HTTP包装器是实验的策略适配，不把它冒充已经接入平台ACL/lease/代次。beta依赖不因协议测试通过而自动成为生产选型；仍需稳定性/漏洞/额外URI出口和完整S4设计审查。实验只用公开包下载和本地合成服务，无真实平台凭据、仓库内容或模型请求。


## 固定版本对照结果与选型边界

本轮F2研究与F5证据整理，用户目标/确认需求不变；Workflow Gate沿用本文件上游和范围：仅隔离实验允许，生产factory仍不允许。新增probe独立module与受控夹具，未改动主go.mod/go.sum或生产执行文件。维护边界为研究脚本，不往Runner/GitRepository增加协议职责。分支codex/sidebar-workflow；本轮代码/文档按显式文件提交推送，不包含其他未跟踪审计报告。

- 扩展CLI桥：3仓（2个SHA-1、1个SHA-256）并发拉取固定非tip提交，实际读取各自blob；v0/v2均可，凭据错误0。4次fetch共9次HTTP请求，加header过滤探针累计10次；禁止路由/无效capability、耗尽共享请求预算、redirect目标零请求、撤权与小body限制仍通过。
- 固定go-git v6.0.0-beta.1：逐操作独立HTTP client，3仓并发固定非tip SHA、depth1、CLI shallow标记/ref/show/fsck连接性检查通过；共享请求耗尽零新增上游，redirect目标0，discovery撤权停止下一请求，body上限失败，阻塞HTTP的context取消传递至服务端。取消实测毫秒值仅记录观测，不作为SLA。
- 第一轮库探针的race来自夹具误用httptest.Server.Client()共享指针并修改Transport/CheckRedirect；修正为每操作新http.Client后-race通过。这不是库缺陷。
- 注入含合成token的500正文后，库原始error确实包含该正文；未来adapter必须规范化错误，不能直接对外返回。该库尚未生产接入，不构成现有应用已泄露凭据的证据。
- 稳定v5.19.3的缓存源码hash_sha1.go/hash_sha256.go显示对象格式由全局sha256 build tag选择；不是“不支持SHA-256”，但不直接满足同一进程逐仓混合格式。v5可经全局安装一次context委托transport实现隔离，不能仅因FetchOptions没有ClientOptions就断言做不到。本轮未运行v5 fetch。

最初独立库依赖图的govulncheck@v1.8.0 verbose结果：0 reachable、0 imported-package、6 required-module advisories；其中5个x/net由0.60.0修复，另1个为未import的旧OpenPGP包。主项目已使用x/net0.60.0，不能声称加库会必然降级；提交的隔离module也显式保持0.60.0。后续扫描结果独立记录，不忽略仍存在的module提示。

阶段结论：两条取数候选均具备局部可行性。优先继续设计“现有Git负责对象/本地工具、Go负责受限HTTP”的方案，以复用已依赖工具链并避免立即引入beta对象解析栈；v6保持备选。不是完整架构批准，也不是认为自建桥已安全。两者的实验ledger均为读取后记账，不能证明并发硬字节限额；CLI桥尚未验收进程树取消/额外URI出口，库实验也不证明恶意pack资源边界。必须完成共享原子预算、网络出口、进程/工作区资源、API双树diff及身份/正式scope消费者设计后，才能进入生产实施。UAR-001仍open。


### 本轮最终复现记录

Go1.27.2、Git2.50.1，全部exit0，无race报告：

1. 根目录`go run -race scripts/probes/git_http_bridge.go`：3仓并发、固定旧SHA、SHA-256及原错误路径通过；成功fetch请求9次，header检查后10次。
2. 独立module中`go run -mod=readonly -race .`：混合格式/固定SHA/浅历史/取消均通过；取消实测4ms；原始错误含合成token的分类为true（只输出布尔值，不输出正文）。
3. 独立module中`go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 -show verbose ./...`：显式x/net0.60.0后0 reachable、0 imported-package，剩1条required-module advisory GO-2026-5932（未import的OpenPGP包），无忽略规则。扫描时点结果不保证未来漏洞状态。

正常根module不执行这两个probe；本轮没有以应用全量CI替代隔离实验检查，也没有重跑未变更的生产全量。研究结果不关闭原生执行UAR-001；下一步仍为共享预算/额外URI出口/进程资源及API双树diff的完整设计与反例验证。无main合并、发布或真实外部发送。


### 本轮提交追踪

隔离对照与证据提交4eac0e11c190ce8b271e2b99f91744bd6199e88e已推送codex/sidebar-workflow。已查询精确HEAD的[CI 38778466286](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/38778466286)：in_progress、conclusion为空；不得借用6460c06成功续期。当前应用CI不会执行nested module，其验证为上文显式命令。本条仅文档追踪，不改动受测代码。


## 进程取消验证输入

本轮沿用F2研究gate，先实测现有gitCommand而不改生产逻辑。临时package测试只初始化空bare仓库、请求回环httptest阻塞服务；服务收到discovery后取消调用ctx，分别观察命令返回与服务端请求context结束。1秒观察窗只作为反例判断，测试在失败前主动释放服务、等待命令退出并清理，避免留下helper。通过不意味着进程树取消在所有Git/helper/OS上均已证明；失败才进入独立修复方案。临时测试不提交，不向真实服务请求，也不把30秒逐命令timeout误认为全部后代进程已终止。Workflow phase=P6实验；production implementation=no；maintainability=narrow research，无Runner/GitRepository职责增加；验收为有界完成、明确命令/上游取消两项证据，且无遗留fixture进程。


## S4预算计量与失败规则：当前可确定的约束

以下为F2设计输入，不是已实施的ledger或F3许可。现有API client已采用请求前整份byte allowance预留/refund；桥probe仍是读取后记账。正式实现必须统一API/Git/主仓/source/context/CODEOWNERS/模型工具读数，不能每个adapter新建额度，也不能通过重试重置消耗。

- 请求计数在实际dispatch前原子扣减；已dispatch的失败/取消/拒绝也算请求，额度耗尽后零下一上游请求。本地路由/ACL拒绝不发送，不能把校验本身算远端访问。
- byte维度明确为应用实际读取的HTTP实体payload，而非TCP/TLS线上总字节；不能声称限制了已进入内核/transport缓冲的所有字节。关闭自动解压并明确拒绝/有界处理Content-Encoding；gzip小输入不能被当作小输出来绕过实体上限。
- 并发读取前先原子预留可读取的最大长度，Read的slice不得超过预留；完成后只退还未读部分，错误与丢弃内容的已读字节不退款。HTTP请求体写入亦独立计量，upload-pack POST不免费。
- 单响应/单对象上限与整次余额同时检查；用于探测超限的第1字节也计入已读，不能把limit+1绕过总预算。只获得部分tree/blob/pack或无法证明EOF时返回明确覆盖不足，不能返回部分成功或“无发现”。
- 时间、请求/字节、工具输出、pack解压后的disk/memory/CPU是不同维度；网络byte上限不能代替工作区或进程资源约束。已读取压缩pack不代表其展开体积已受限。
- 余额耗尽与权限撤销均停止继续读取和结果消费；已dispatch不能声称从未发生。原始HTTP/Git错误正文不进入用户文案、trace或通知。

仍待明确的实施阻塞：各维度准确值与配置/PolicyVersion冻结字段；准入snapshot与worker执行的额度归属；lease重领、进程重启及同run续跑的持久消耗继承；跨consumer的一次性reservation所有权与关闭；宿主资源限制能力。没有这些不能把“共享指针”或当前client里的128次/32MiB直接说成整次审计预算。下一正式设计需给字段矩阵、生命周期、事务/故障恢复及并发反例，不能先写半套ledger。

### 额外URI出口的官方依据

[Git packfile-uri设计](https://git-scm.com/docs/packfile-uri)说明fetch.uriprotocols默认空；[Git2.50配置文档](https://git-scm.com/docs/git-config/2.50.0)说明transfer.bundleURI默认false，但fetch.bundleURI可在fetch前引入额外对象下载。这支持未来显式固定禁用策略和受控反例测试，不能由“默认关闭”推出所有版本/配置下无额外出口。当前隔离仓库未继承全局配置，但普通CGI夹具仍未覆盖主动恶意URI广告；本轮不据文档就把该验收标为完成，也不在进程修复中混入协议配置变更。


### 现有Git取消实测与修复去向

原实现真实fetch取消反例已复现（1秒和最终测试3秒观察窗内命令/上游均未结束）；独立修复方案与评审见git-process-cancellation-plan.md及technical-review-2026-10-16-git-process.md。代码18409c0已推送：Linux/macOS独立进程组取消，所有平台WaitDelay有界；macOS真实HTTP取消、原实现反向overlay、全Go110.592s/platform race362.423s、vet/build及Windows交叉编译完成。精确Linux CI38780235612当前queued，未声称Linux已运行通过。该修复解决现有路径问题，不是进程沙箱/整次资源配额，不关闭native factory或UAR-001。


## 额外URI出口实验输入

2026-10-16，F2/P6研究继续；Confirmed REQ与用户“没想清楚先不实现”不变。Workflow Gate：已有固定Git/混合对象/取消证据，缺恶意URI广告与显式禁用反例；允许隔离probe，不允许生产factory。Maintainability Gate：新独立git_uri_egress.go，单一客户端协议研究职责；不向Runner/GitRepository或既有bridge继续塞实验状态。无公共API/依赖/配置/持久schema变化，主module通过ignore tag排除。

实验仅合成仓库与回环CGI服务，额外URI捕获端为另一回环HTTP origin。参考[Git v2.50.1自己的packfile-URI测试](https://github.com/git/git/blob/v2.50.1/t/t5702-protocol-v2.sh)，用实际pack-objects和uploadpack.blobpackfileuri生成可取对象URI，而非只搜索header。需要正对照：主动启用URI时捕获端必须收到请求，证明检测不是恒为零。再验证默认/显式关闭、local config污染被命令级配置覆盖，以及未经协商的URI响应；有效fetch需实际读取预定blob。

bundle URI独立验证fetch.bundleURI配置的正/负对照；transfer.bundleURI只作为clone相关选项，不能仅凭fetch不发bundle-uri命令证明clone安全。只测试本机Git版本的有限协议情况，不把零捕获请求称为所有额外出口均被阻断。禁用候选需确认空配置值的实际行为及内容成功后，才进入未来生产设计。

脚本不读取真实配置/凭据/代码、不访问外部托管服务；只输出类别/次数，不输出raw Git/provider错误。失败主动清理临时文件和server。正常root CI不运行独立probe，需显式-race复现。上一生产18409c0对应CI38780235612仍in_progress，研究代码等待其终态后推送，不能取消它。

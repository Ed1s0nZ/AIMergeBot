# F4 实施记录

第一切片新增四项source-linked checks，限定kind/status/理由/ID数，ID须属于本次独立复核且通过现有固定新来源校验。缺项/部分不支持降inconclusive；非法记录走原结构无效处理。既有主锚点、context与whole-claim门禁先执行，不替代原校验。新增结果深拷贝，不改历史JSON；policy v29。前端展示四项理由、来源和未记录提示，SARIF沿现有verification对象携带新增数据。

新专项检查exit0 0.594s；相关Verification/Context stage/Compression回归exit0 0.760s；相关race exit0 2.614s；TypeScript/Vite build exit0 887ms（npm ci --ignore-scripts按lock安装，审计0项）。首次旧测试失败原因：旧full-only提案按新契约被降级；测试更新为显式四项静态fixture，新缺项降级测试保留，不放宽源码或门禁。

四项仍为模型声明与记录完整性，不能证明语义正确。实际UI及真实模型使用效果未验收，后续调查计划、覆盖状态、调度和真实PR评测尚未实现；不能宣称整个目标完成。

受控IAB真实VerificationEvidence组件展示四项（两项supported、两项inconclusive）与历史缺少checks记录；360x800实际scrollWidth=360无横向溢出。截图/tmp/aimangebot-checks-v29-proof.png。组件fixture未含实际source IDs，不将此当源码导航/生产E2E证明。临时文件清理、视口reset、tab关闭。启动npm首次误用仓库根目录exit254，改frontend后启动成功。

最终go test ./... exit0：platform82.309s，其余包cached；更新SDK模拟后TestIndependentEinoVerification/Verification专项1.091s与race3.064s通过。先前全量43.619s失败仅旧supported响应缺checks，已保留此事实。后一次耗时增长未做归因，不能据此宣称性能改善。

F5首切片真实case203回归已归档64文件SHA256复读验证，见evaluation-v29-case203.md。模型实际填写四项/重读来源，但low-privilege附加断言无角色证据、主调查PRContext缺口仍在。目标继续active；需要调查计划、逐断言范围、覆盖展示和真实/隔离评测，不为刷过本例重复调参/复跑。

第二切片：新增来源关联调查计划、不可删除/改题的更新约束、继承后预算与来源所有权检查、深拷贝、四核心任务完成约束和最终计划缺口提示；policy v30。前端调查账本显示问题、状态、理由、来源与历史缺失提示。未执行被审仓库代码。

首次全量测试失败：旧零账本 SDK/worker/CLI fixture 仍期望 succeeded，新增计划缺口使其 incomplete；调整这些 fixture 的精确状态和 notes 数量，没有放宽门禁。补充真实 Eino SDK 工具序列测试：零账本不能认证完成、四任务关联实际成功工具源后可排除调查并无计划缺口。专项通过1.100s，race通过1.947s；最终 go test ./... exit0（root3.754s、platform43.627s，其他包缓存），TypeScript/Vite build通过1.12s，git diff --check通过。

本切片尚未做实际浏览器展示验收、真实模型使用调查计划回归与新真实PR评测；这些仍待完成。计划完成只表示记录/来源完整，不证明模型语义判断正确，不能把新增门禁当作准确率提升。

第三切片：更新省略PRContext保留既有记录，在保留后重新执行source ownership、snapshot side与总预算校验；getter深拷贝PRContext。新增真实Git源测试覆盖保留、返回别名隔离、去掉关联源被拒绝、继承后8000byte超限、显式空对象拒绝。专项4.326s、race4.512s通过；全量 go test ./... exit0（platform132.239s，其他包缓存）。耗时比上一轮增加，同期存在其他Go测试进程，未归因或宣称性能改善。git diff --check通过。

第二切片UI和真实模型验证见evaluation-v30-case203.md；真实模型仍有结构化PR链缺口和方法说明混入coverage的问题。没有将单例改进当准确率证明。

第四切片：audit_priority.go对固定改动的路径与新增/删除文本取最多三档词法检查权重（未知语言默认1）；排序后组取最高权重，未改变单组/总diff/组数上限。剩余预算足够时先为后续每组保留1调用，再分配额外调用；时间按剩余权重分配，原全局预算/超时保留。priority_weight只是调度提示，无finding/语义证据资格；模型导航与前端明确提示。policy v31。

新专项2.259s、分组回归10.767s、race5.286s、TypeScript/Vite4.08s、全量Go exit0（platform118.079s、root13.784s、evaluation22.164s）；git diff --check通过。专项覆盖去掉保护/危险操作排序、未知语言、排除不绕过、输入顺序稳定、组上限未处理缺口、低预算/MaxInt/Duration溢出、历史权重。尚未用真实模型分组对照证明漏报或速度改善。

实际受控AuditGroupProgress组件360x800无横向溢出，完成/未处理/历史缺优先级与可展开文件确认；截图 /Users/worker/.codex/evaluation-artifacts/aimangebot/priority-v31-ui-20261006/priority-proof.png，非生产E2E。临时文件/服务器/浏览器视口/标签清理。

第五切片（未提交/未推送，待环境恢复）：原生Git执行器exit69或执行文件缺失返回typed ErrRepositoryUnavailable；普通exit128/路径不存在不归fatal。tools在互斥保护下记录fatal，model Start回调取消下一请求；primary错误返回明确停止原因和已有finding/ledger/trace。grouped随后组仍unprocessed，跳过综合/复核等补充模型并返回原fatal，保留已合并结果。policy v32，未部署。

验证：默认go test在编译runtime/cgo阶段因Xcode许可未同意失败，未运行用例。CGO_ENABLED=0专项第一轮1.138s，增加真实SDK候选保留用例及优先级专项后1.214s通过；最终CGO_ENABLED=0仓库不可用/模型请求专项1.564s通过；CGO_ENABLED=0 go vet ./internal/platform通过。SDK实际HTTP计数验证：fatal源错误仅1次HTTP，普通源错误>=2次可继续；分组fatal跳过其他组/补充模型。另两个SDK用例先读取静态fixture并通过正常submit_finding接受candidate，再读fatal源，实际3次HTTP后停止，candidate及精确源不丢失。分类测试执行自有exit码fixture，未执行任何被审仓库代码。这是控制流/记录正确性证据，不是模型准确率证明。

还必须在合法Git/默认编译环境恢复后运行默认Go全量、相关race并提交推送；当前不能把纯Go专项当完整验收。Git仍直接exit69，未代用户同意/绕过Xcode许可。真实历史失败归档与evaluation-v31-real-history.md也尚待提交，已有0b7a4ad之前功能已推送。

环境恢复后的复验：系统git --version已exit0、默认CGO专项1.741s与相关race5.123s通过。首次默认PATH全量exit1：root CLI两个30s fixture Git调查超时（root106.757s）；platform345.685s、evaluation245.873s通过。独立git --version启动测量837/9361/2063ms；xcrun --find git25ms，返回的同版本Apple Git-155执行--version12ms。许可现已通过，不绕过许可、不提高生产预算。临时PATH选择已安装同版本Git的CLI对照保持30s配置，root专项23.233s通过；原失败完整保留，不以对照替代原环境事实。完整同版本Git对照 go test ./... exit0（root13.028s、evaluation14.189s、platform197.356s），未增大测试/生产audit超时或工具预算。默认启动器仍有已记录延迟/超时限制，不将对照称作默认PATH全量通过。git diff --check通过。第五切片准备提交推送，真实历史回归使用新目录且原失败保留。

第六切片实现：primary_round_budget.go在compression之后复制system消息，添加本地当前决策/上限提醒，最后三轮提示收尾、最后一轮要求严格JSON；原始消息/工具源不修改，提醒不累积。保留原graph上限。agent_stop_reason.go按errors.Is映射有限原因；grouped服务端写stop_reason，最终模型JSON仍不能控制AuditGroups；评测从failed组保留原因，不靠模型字符串判断。policy v33避免新请求复用旧策略结果。

专项：最后2/4/8轮真实SDK HTTP请求含当前提醒并能返回报告；原4轮边界忽略提醒仍只请求4次且typed耗尽/明确coverage；分组失败保留原因，旧policy隔离测试通过。两包专项4.935s/3.322s；纯helper race2.010s/2.718s。fake provider仅证明机制，不证明真实完成率。UI原因展示、真实模型回归以及完整R3/F6验收仍待完成。

第六切片全量验证：临时PATH选择已安装同版本合法Apple Git，go test ./... exit0：CLI12.050s/evalcmd1.561s/evaluation17.509s/platform138.681s；不是默认PATH启动延迟测试。SDK决策边界/分组保留race4.420s；go vet两包exit0，git diff --check通过。仅服务端与评测实现，无前端变更，未据此宣称真实模型准确率或完成率提升。

第六切片UI：分组optional stop_reason绑定服务端字段，failed组展开显示有限中文原因；历史缺失与未知值明确区分，未完成组不称风险已排除。实际组件控制fixture在360x800无横向溢出（scrollWidth360），Enter展开预算原因，并核查历史/未知显示；截图/Users/worker/.codex/evaluation-artifacts/aimangebot/stop-v33-ui-20261006/stop-proof.png。fixture、服务器、tab和viewport已清理。TS通过；首次Vite命令误在root执行报缺少index.html，修正frontend cwd后Vite951ms通过，临时输出在仓库外。真实v33回归失败详见独立报告，未宣称质量改善。

第七切片：stop_reason承接wrapped AuditResponseError有限code，字段未知、语法/类型、尾随数据、缺字段、数量/大小上限可区分；构造未知code统一invalid_structure，不泄漏任意文本；字符串相同不视为typed原因。UI翻译相同服务端有限码。真实SDK分组模拟unknown_field：仍failed/coverage不为空，synthesis不能洗成完成，具体原因保留。专项platform7.275s/eval6.610s；race含SDK分组platform7.421s/eval5.181s；TS和Vite10.03s通过，git diff --check通过。未改schema/自动修复/增加模型请求或预算，未重跑真实样本求通过，历史v33原receipt保持agent_failure。

取证现状核查：已有side/path固定快照缓存16MB；agent_repeat_guard已有三次成功相同参数读取上限，重复搜索cursor分开。因此不能把新优化说成首次提供缓存/重复停止；重叠但不同参数范围读取及缺失依赖搜索仍是剩余效率问题，应据当前实现设计通用导航，不直接拒绝所有重读（独立复核仍需要新来源）。

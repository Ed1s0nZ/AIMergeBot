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

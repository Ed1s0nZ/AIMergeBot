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

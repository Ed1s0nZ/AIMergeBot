# v45 原模型正负例真实回归

冻结CLI a390bf9a5b9603802b20c64074d838780524acfb；Go运行源码dfc6340、UI源码53331c8，策略eino-audit-contract-v45。原deepseek-chat/temperature0.1/15steps80tools240秒，原corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b。802、803各一次known regression，无重新请求805、无改真值/预算/执行样本或PoC。独立首次验收仍见历史AC011归档；本次不是盲测准确率。

| 样本 | 真实结果 | 耗时/完整token用量 | 命题复核 |
|---|---|---|---|
| 802 Go保护恢复负例 | incomplete、0finding | 46.607秒、175899tokens、14次模型调用 | 初审supported、fresh true、consistent |
| 803 TS→PHP风险正例 | incomplete、1finding，HEAD gateway.ts:6正确主PR锚点 | 47.232秒、161005tokens、17次模型调用 | 初审supported、fresh true、consistent |

802 BASE2f9e36bcb564071cc34a3ac841f0a0bb6a672ba8，HEAD8be04549c1244c96fad5aebc48180dc9e14cf561。初审与复核实际读取release.go/caller.go/workflow.md双侧，正确识别approved守卫恢复及原授权检查保留，没有把安全改善判成漏洞。原计划四项checked；错误BASE/HEAD引用经拒绝后重读并正确提交before/after ID。仍未保存Handle→Release的结构化relationships，coverage如实保留；上游State如何写入和bank执行不在fixture范围，不编造。复核一次read_files外层请求（6项子读取、7条非model trace），引用fresh batch来源，2次模型调用6609tokens；primary12次169290tokens。

803 BASE89f63fa45e420d84b764bcf1c03ba903dd2856f4，HEAD3d1f15da53dd09fffaf407c2ed740e33ed21cf9c，授权下游2固定c689760aa604513d3db170c756b9e3615ecd7f87。初审读主PR双侧、query.php/connection.php/deployment.conf，定位固定pending→调用者query.state，经form POST进入PHP原始SQL拼接。实际序列finding verification6次模型/6次工具先于claim review3次模型/4次工具，使用原共享pool，无额外预算重置。各阶段token分别111242、35708、14055，完整计量、零未知调用；价格未配置，金额未知，不能推断账单或总体费用改善。

803五项计划checked，静态独立finding四检查supported；fresh claim只证实实际命题中的输入变化、auth及form transport，不能解释成SQL安全。模型finding描述/trigger及outcome含“allowing/exfiltration”确定措辞，同时limitations承认PDO driver/credentials及实际执行未知；这是残余语义过强，不能当运行时已验证。原调查counterevidence仍含主仓库未见SQL而下游unproven的旧措辞，和后来固定下游证据不完全同步。结构化PRContext before_observation_ids/after_observation_ids与relationships未写入，11条coverage包括调查及finding对应缺口、record_hypothesis失败历史；不得用摘要或模型full替代逐边源码记录。

外部完整原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v45-paired-20261006，含freeze/corpus、两组metadata/独立真值/receipt/checkpoint/trace/run.log/固定Git；sha256-manifest.json逐文件核验通过，0700/0600，无真实config/credentials。两次CLI exit0，无余额错误；exit0表示评测已写回执，不表示审计completed。

结论：v45独立命题复核在805真实分歧、802安全负例及803跨语言正例有实际调用证据；未达到整体语义闭环。AER-001继续open：源已读但结构化主PR双侧/调用契约关系漏记、后续证据与旧反证不同步、条件风险文案过强。下一步应针对这些具体缺口改进记录收尾及条件判断，不能降低门禁、自动补造关系、隐藏failed/incomplete或盲重跑取得completed。

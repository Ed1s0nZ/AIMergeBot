# v47 原预算收尾真实回归

源码297e61034c6f40d5679008f7b86cbd896fda15d1，策略eino-audit-contract-v47。原deepseek-chat/temp0.1/15steps80tools240秒，原corpus1851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b；802/803各一次known regression，未改真值、增加预算、执行样本/PoC或重跑805。原配置最小连接探测HTTP200，两次CLI exit0，没有API/余额错误。

工程先决：本地full exit0 platform205.620s、五受影响fixture race39.055s、vet/diffcheck通过；精确CI37455551443在297e610全部stage success。独立冻结481文件全匹配，full96.142s、race23.639s、vet/diffcheck通过；review-v47-renewal-297e610/PR_REVIEW_REPORT.md scoped APPROVE，PFR-001 resolved。原18513f5 CI失败与REQUEST_CHANGES、待CI报告保留；批准仅预算/收尾切片，不是整分支或真实模型质量批准。

| 样例 | 实际调用与成本 | 结果及范围 |
|---|---|---|
| 802 Go守卫修复负例 | 29.148秒；11模型调用（primary9、claim2）；141001tokens（primary133936、claim7065） | 0finding；primary supported/fresh true consistent；双侧ID、cited调用关系与记录错误显式恢复均保存；两条必要静态/状态转换未知保留，status incomplete |
| 803 TS→PHP风险正例 | 46.175秒；16模型调用（primary10、finding4、claim2）；197560tokens（primary163561、finding25307、claim8692） | 1finding，正确HEAD gateway.ts:6；primary读全三份上下文，双侧/映射关系保存；fresh finding supported/full但载荷细节仍偏强，fresh claim漏上下文被server降为inconclusive/unknown；status incomplete |

固定SHA：802 BASE2f9e36bcb564071cc34a3ac841f0a0bb6a672ba8/HEAD8be04549c1244c96fad5aebc48180dc9e14cf561；803 BASE89f63fa45e420d84b764bcf1c03ba903dd2856f4/HEAD3d1f15da53dd09fffaf407c2ed740e33ed21cf9c/授权context2 c689760aa604513d3db170c756b9e3615ecd7f87。全部provider usage可用，未配置price，金额未知。已知样例单次差异不能证明总体准确率、收尾控制的单独因果效益或费用改善。

802初审read release.go BASE/HEAD、caller.go HEAD、workflow.md HEAD；原命题明确guard改善而非漏洞，五项计划checked。obs7/8更新因缺顶层ID和目录ID不eligible被拒，obs9显式修正；record_pr_context obs10保存before obs2/after obs3、4、5和Handle→Release cited关系，resolve_recording_errors obs11把原两次失败显式映射obs9，原失败trace保留。fresh claim实际重读双侧、caller和workflow共4个源，正确支持实际改善命题。状态如何变为approved缺源码是真实未知，不要求模型伪造completed或安全证明。

803初审obs8用read_repository_files实际读取query.php、connection.php、deployment.conf三份固定源码；后者服务/POST路径/form encoding证明gateway→PHP契约，connection明确PDO由部署提供，driver/凭据不在fixture。record_pr_context obs9记录原结构，update obs10保留命题与四项checked计划；finding obs11正确定位gateway.ts:6。收尾record_pr_context obs12显式保存before obs4/after obs3，关系用obs3/8 cited。最终finding context也含双侧与映射，来源未知保留，未把目录名当契约。

这里已不再出现v46的deployment.conf可用却漏读，或读双侧但最终无ID的缺口。primary next_steps称无更多源码，实际三份上下文均读；driver/网络凭据确实不在样本，不误判为可读却遗漏。fresh finding仅读query.php/deployment.conf而未读connection.php；其limitations注明UNION/stacked未演示、仅WHERE影响，但claim_coverage却full，原trigger仍列UNION/stacked并称altering executed SQL，条件表达与完整覆盖标签不足。fresh claim仅gateway双侧/get_diff而不读固定上下文；server保留其原解释但不接受true，明确unknown/缺必要context，没有虚构独立证据。首审summary、finding和独立review各自保存，不借review回写成已证明。

剩余优化：独立命题复核应按已有固定导航读并引用必要related source，仍在原finding-first共享40工具/60秒及每项限制内；模型具体载荷和数据库结果必须明确条件，fresh review full不能与未支持的子断言矛盾。AER-001整体语义证据闭环仍待当前范围审计，不能用两项source改善替代整个R3验收，也不新增数字准确率/全样本completed条件。

外部原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v47-finalization-20261006（109文件SHA256逐项复核；0700/0600；metadata/truth/receipt/checkpoint/trace/固定Git/corpus/freeze/probe/run.log，无config/credentials）。未自动合并main或部署，原工作区用户文件未改。

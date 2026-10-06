# v46 PR上下文工具真实回归

冻结CLI/Go head c5aa4d7627dd81ad3f290041fa0b399dce668714，策略46。工程先验：full Go exit0/platform156.137s，context/PR/纠正等race60.011s，独立claim SDK/分组取消/worker race9.047s，vet/diffcheck；精确CI37450141111success，独立新上下文工具delta scoped APPROVE（review-v46-context-c5aa4d7/PR_REVIEW_REPORT.md，476文件hash一致）。这些只证明工程切片，不批准整个分支或质量结论。

原deepseek-chat/temp0.1/15steps80tools240秒、原corpus1851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b；802/803各一次known regression，未改真值/预算/执行样本/PoC，未重跑805。两次CLI exit0、实际结果均incomplete，零API余额/调用错误。

| 样本 | 耗时/调用/完整tokens | 实际质量与工具采用 |
|---|---|---|
| 802 Go守卫修复负例 | 17.904秒；7调用（primary5、claim2）；68835tokens | 0finding、primary supported、fresh true一致；调用方/业务约束已读、cited Handle→Release关系保存；未调用record_pr_context，仍缺显式双侧ID |
| 803 TS→PHP风险正例 | 41.250秒；16调用（primary9、finding4、claim3）；178495tokens | 1candidate，HEAD gateway.ts:6正确锚点；record_pr_context成功obs11、update obs12保留；结构化跨项目inferred及PHP内部cited关系已保存，仍缺显式双侧ID |

固定SHA同v45：802 BASE2f9e36bcb564071cc34a3ac841f0a0bb6a672ba8/HEAD8be04549c1244c96fad5aebc48180dc9e14cf561；803 BASE89f63fa45e420d84b764bcf1c03ba903dd2856f4/HEAD3d1f15da53dd09fffaf407c2ed740e33ed21cf9c/授权context2 c689760aa604513d3db170c756b9e3615ecd7f87。802各阶段tokens61923/6912；803 primary138603、finding25753、claim14139。价格未配置，金额未知；单次随机已知样本差异不是准确率或总体费用改善证明。

802五任务checked并正确解释guard改善，实际初审读release.go BASE/HEAD、caller.go HEAD、workflow.md HEAD。首次record obs6缺调用方top引用遭拒、obs7修正成功；原失败历史未明确retire，保留coverage。fresh claim仅读release.go双侧/get_diff，没有独立读取workflow/caller，虽判断true但对“not a regression”的业务语义覆盖弱；不能把consistent等同完整安全证明。

803初审在obs7目录已看到connection.php/deployment.conf/query.php，却只读query.php(obs9)与connection.php(obs10)，没有读可用deployment.conf。obs11新工具实际保存两条关系：gateway→worker inferred（matching path/field），PHP输入→SQL cited。命题/update与finding仍称“没有source证明mapping/no further source available”，这是遗漏可用证据，不是外部不可解阻塞。新工具schema和门禁已证明可用，但它接受完整context的实际缺项并返回base_sources_missing/head_sources_missing，随后primary9/15决策就提前返回摘要；802同样primary5/15提前返回。不是预算耗尽，没有通过新增预算补造引用。

finding独立复核在自己的固定新鲜来源中读到了deployment.conf，正确说明service report.worker POST /query→query.php；它不自动覆盖初审记录，因此原inferred/误称无证据的摘要仍保留。claim独立复核读query.php/connection.php但没读deployment.conf，仍错误限制为“no proof same worker”。保留真实差异与10条coverage；不能将一次fresh true或finding full作为完整链路闭环。描述已有conditional if-routing措辞，但summary/反证与可用mapping仍未同步。

外部完整原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v46-context-20261006（108文件SHA256逐项核验、0700/0600、两组metadata/truth/receipt/checkpoint/trace/固定Git、freeze/corpus/run.log，无config或credentials）。

结论：新工具在真实803确实采用，结构化关系记录相比v45有进展；AER-001仍open。接下来必要改进是原预算内的最终记录收尾控制与未读相关上下文导航，不能只新增可被跳过的工具/重复prompt，不能自动cited/填来源/抹除unknown，也不能盲重跑取得completed。

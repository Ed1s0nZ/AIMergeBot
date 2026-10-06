# v48 独立来源要求真实回归

源2eb8daf74383960dafc69a79e07395660a4f870a，策略eino-audit-contract-v48。原deepseek-chat/temp0.1/15steps80tools240秒、原corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b，803仅一次known regression；未改truth、预算、执行样本或重跑802/805。

工程：full Go exit0 platform172.055s，相关race12.568s、vet/diffcheck通过；精确CI37457629388在2eb8daf全部stage success。独立review-v48-source-2eb8daf/PR_REVIEW_REPORT.md仅source/model-input trust slice APPROVE，无新findings；484SHA全匹配、独立目标Go48.723s、完整main diff164文件分类。该批准不覆盖整体R3/AER-001或原模型质量。前端/web与已批准53331c8无diff。

真实CLI exit0，但receipt status=model_or_agent_failed/error_class=agent_step_budget，42.333秒，0finding、调查仍investigating；不能当TN、安全结论或成功验收。15次模型调用均primary，已报告252141input+9556output=261697tokens，usage.complete=false（预算中断）；未知price，金额未知。没有API/余额错误，未发生finding/claim独立复核调用，因此本次不能证明新source_requirements或子断言提示的真实采用效果。

固定BASE89f63fa45e420d84b764bcf1c03ba903dd2856f4/HEAD3d1f15da53dd09fffaf407c2ed740e33ed21cf9c/授权context2 c689760aa604513d3db170c756b9e3615ecd7f87。初审读gateway BASE/HEAD(obs4/3)，hypothesis obs5因未提交top源码ID被拒、obs6显式纠正成功。实际context目录obs7列可用源码，search obs8及query.php obs9、connection.php obs10读到，但deployment.conf未读。

record_pr_context obs11至19共九次均报同一泛化“invalid PR relationship or certainty”；实际输入from/relation/certainty有值，但relationships[0].to为空字符串。原validator正确拒绝并保留先前ledger，重复失败均trace留存；反馈未说明是哪条边、哪个字段/限制，模型没有修正。最终15次decision硬cap停止，0finding不伪装无风险，原未完成计划/反证/未知保留。初审未到合法final，v47一次提前final收尾不适用；不提高budget或隐藏失败来继续。

下一步原范围优化：关系验证错误给有界字段路径/限制/合法显式修正说明，目标未知时明确允许省略该edge并保留unresolved_edges，不推断目标、填写source、改变certainty或放宽门禁；同时检查schema是否表达必需字段，实际失败输入离线证明消息准确及拒绝副作用。只有工程边界验证后再调用原模型，不在同源码上盲重跑。AER-001 open，不新增数值准确率/全部completed标准，不据一次随机差异归因新prompt导致退化。

完整外部原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v48-source-20261006，69文件SHA256逐项核验，0700/0600；freeze/corpus/metadata/truth/receipt/checkpoint/trace/run.log/固定Git，无config/credentials。未自动合并main或部署。

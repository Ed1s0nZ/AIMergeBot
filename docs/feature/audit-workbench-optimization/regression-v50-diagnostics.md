# v50 独立复核诊断与真实回归

固定源码01446573f8964ab90be86774718dc7b01e43d4c2，策略eino-audit-contract-v50。原deepseek-chat/temp0.1/15steps80tools240秒；803一次known regression，corpus SHA1851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b，未改truth/预算，未执行样本。

## 工程及诊断契约

新增parse失败服务器有限分类及有界JSON形状，保持原parser/source/check接受门禁，unavailable/coverage/原finding不变；不保存正文、私密字段名或来源ID，不增加重试/调用。初轮截断JSON归类测试失败（io.ErrUnexpectedEOF），修正后目标Go1.018s、相关race5.527s、最终实际SDK/checkpoint race4.649s、vet/diffcheck通过。最终full Go platform125.438s exit0；初次修正前full176.453s失败如实保留，不当当前验证。

源码精确CI37461595471/job112262248575全部success；独立review-v50-diagnostics-0144657/PR_REVIEW_REPORT.md切片APPROVE，491文件hash、172main路径及8delta分类；独立race4.048s、附加畸形/隐私2.420s通过，不批准整分支或模型质量。

## 原模型803实际证据

CLI exit0、receipt incomplete，40.361秒，19calls（primary11/finding5/claim3），usage完整、unknown_calls0。215601input+7795output=223396tokens；阶段179744/30046/13606tokens。价格未配置，金额未知；无API/余额错误。

主审实际读取固定gateway BASE/HEAD及授权context2全部query.php/connection.php/deployment.conf，保存双方source IDs、GET /report入口/认证、cited部署→PHP请求字段关系和SQL插值来源，定位HEAD gateway.ts:6；条件及未核实驱动保留，无具体载荷成功承诺。fresh finding和claim均重新读取这些来源，finding supported/full与claim consistent/true是静态判断，不代表复现。无failed tool/model_response；incomplete来自实际运行/driver/payload未知，不能当FN、失败计数或安全结论。

本次未触发结构错误，不能证明真实错误分类采用、不能恢复v49丢失的原始返回、不能归因诊断改动提升语义/成本。旧失败仍保留。源码路径和guard/契约记录较早缺项有真实证据，known regression不外推blind准确率。

## 原需求重审及限制

独立r3-quality-completion-v50-0144657/PR_REVIEW_REPORT.md按原R3重审AER-001：v47-802读可用caller/workflow并正确保存修复命题，v49/current803 source-linked跨仓库关系、既有804参数绑定负例满足有界验收，原阻塞关闭。805历史事实真claim被首审rejected仍存在，fresh true/disagreed及原历史保留，作为AER-003 S3非阻塞语义残余明确记录；没有宣称805修复或要求所有模型一次判对。工作台/工程完成证据另见completion-audit-v50.md，不由质量报告批准。

完整外部原始归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v50-diagnostics-20261006，69文件SHA逐项核验，0700/0600，无config/credentials。未自动合并main/部署。

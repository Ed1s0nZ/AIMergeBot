# v43 805 定向真实回归

冻结CLI revision53566c4278fd6d8a03018c9ea82fb318698a9af6、生产源码d8fff2219552f697006fc70e45bb4785a57c54d2（中间仅文档差异），策略v43；原deepseek-chat/temperature0.1/15steps80tools240秒，原corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b。805一次已知回归，不是新盲测、不改真值/预算、不执行fixture。

结果incomplete、0finding，13.264秒、5轮、56573输入+2649输出=59222tokens，usage_complete=true、无API余额错误；价格未配置、费用未知。比v42单例127160tokens少，但工作量/随机输出不同，不推断普遍效率。

主仓库BASE/HEAD client.go读到，固定下游server.py/deployment.conf批量读到；摘要正确default25及0..25 bounds保持兼容。此次update保留所有五个原question，填好before_observation_ids/after_observation_ids并提交cited HTTP→handler关系，说明模型能构造这些原契约字段。但update(8) id为空，未知hypothesis被正确拒绝，保存ledger仍inv-1 investigating、所有计划pending、无保存双侧引用或关系。记录(5)实际提交id=inv-1且成功；最终说明“generated id not accepted”与原调用不符，根因是更新请求id=""，不是生成器拒绝。不能将被拒的完整输入当作验收通过。

模型实际提交evidence_refutes_claim，Claim仍为初始payload变化/下游未知的复合陈述；已查下游支持兼容结论，并不否定payload变化事实。命题和所评估风险对象仍混淆，不宣称v43解决极性。未触发计划身份错误，故本例不能证明模型采用新错误恢复反馈；只是questions实际保持不改写的一个观察。

无重试，完整失败history与覆盖说明保留。下一步局部恢复反馈应指出当前audit已知ID，只作非源导航、不自动从Claim匹配/改ID，不掩盖真实语义未知。AER-001继续open。源码CI run37436813700已核对终态success；不是质量结论。归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v43-recovery-20261006，66文件SHA核验一致。

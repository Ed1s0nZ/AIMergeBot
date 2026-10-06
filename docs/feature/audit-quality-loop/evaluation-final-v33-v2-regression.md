# 当前六例回归：v33

代码1601410cbdf9d4c58ff16ce28ca4dc399025c0c0。原v2六例已被分析，本次为回归，不是隔离验收。运行前冻结corpus/code/hash，原配置15轮/80工具/240s，无执行样本代码、无提高预算、无挑选重跑。旧acceptance-protocol-v2写16轮与实际metadata不符；本次以配置元数据15为准，不能修改过去实际运行。

| case | findings | 状态 | elapsed ms | tokens | 失败类别 |
|---|---:|---|---:|---:|---|
|401|1|incomplete|23955|114211|—|
|402|0|incomplete|2460|15628|—|
|403|1|incomplete|20173|98812|—|
|404|0|model_or_agent_failed|13404|64521|model_response_json_syntax|
|405|0|incomplete|3382|17453|—|
|406|0|incomplete|3309|15707|—|

总326332tokens；价格未配置，不估成本。CLI exit0是执行归档完成，不等于审计完成。

人工机制核对：401正确BASE9删除所有权保护，bob/d-a/a-private均来自实际Rust源码；主调查plan四项齐全、独立四项支持，但before/after来源与逐边关系未记录，仍incomplete。403正确HEAD6参数owner替代actor，主调查与独立复核均读取固定Python下游；条件泄露符合人工真值，独立明确deployment绑定未证明，主PRContext仍未逐边记录。两个正例的位置正确，不代表语义普遍正确或运行利用。

402零告警识别新增保护，但无ledger/plan，不能作为完整负例通过；404实际读取下游保护并保存rejected调查，但无plan且最终JSON syntax失败，不能计TN；405没有读取已授权下游，兼容性负例仍证据不足，不计TN；406正确区分未变exec历史风险与当前日志字符串改动，无告警但缺plan。四个负例无告警不等于四个TN，无完整通过率/precision/recall的可靠分母。

工程改进实际保留了细分错误、固定来源、四项静态判断和计划，但当前模型仍经常跳过计划或关系记录、误放methodology于coverage，不能宣称完成率或准确率提升。未根据本次输出调整真值、清理失败或放宽门禁。

外部归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/final-v33-v2-regression-20261006：freeze/corpus、全部metadata/truth/receipts/checkpoints及固定Git对象，共286文件SHA256复读核对，sha256-manifest.json。原输出 /tmp/aimangebot-final-v33-v2-regression-20261006。

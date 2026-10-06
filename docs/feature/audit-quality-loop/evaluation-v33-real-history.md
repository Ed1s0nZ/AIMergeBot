# v33 同一真实历史样本回归

代码f599177；同一case601固定Express修复/父提交，使用v32归档corpus.json，原配置15轮/80工具/240s，临时PATH选择同版本已安装Apple Git。该公开历史样本此前已使用，不是新未见样本，也不是代表性质量验收。本次没有新增运行前冻结文件，不声称满足全套独立冻结评测协议。

结果：incomplete，0finding，2,828ms，13,570tokens，两个model调用（primary/synthesis）；主模型首轮直接返回无工具调用的结果，解析报typed invalid audit response: unknown_field。诊断仅保存code/bytes=80/valid_json=true，不保存原始异常回复；无法根据诊断重建具体未知字段。没有source工具或ledger记录，不得计true negative。收尾提醒最后三轮未到达，不能据本次证明完成率改善。分组stop_reason=agent_failure，receipt保留相同类别；诊断保留unknown_field但组级类别仍泛化，后续应承接typed AuditResponseError分类。

保留原失败，不通过针对本例放宽JSON schema、增加预算或反复重跑求通过。原输出/tmp/aimangebot-real-history-v33-case601-20261006；归档/Users/worker/.codex/evaluation-artifacts/aimangebot/real-history-v33-20261006/receipts，所有文件SHA256复读核对，清单receipt-hashes.json。

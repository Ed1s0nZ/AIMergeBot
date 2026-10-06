# v29 分项复核定向回归

代码fa85a9c，policy v29，既有acceptance-v1 case203已用于调优，仅为回归不是新隔离或真实PR。CLI使用原配置、240秒上限、16步/80工具，时序图关闭，真实deepseek-chat，只读源码。exit0、incomplete、1finding、20491ms、107244tokens，价格未配置费用null。

主风险：HEAD客户端x-principal覆盖gateway注入actor，固定Python下游按X-Principal返回private_email，锚点gateway.mjs HEAD7正确。主调查读BASE/HEAD及批读profiles.py/deployment.conf；独立复核重新读两侧与下游。四项checks实际填写并引用新verify命名空间来源，模型full/supported。

局限：模型把bob称“low-privilege”，源码只有两个身份并无角色/权限高低，属未支撑附加描述；全量自报full仍有不足，不把四项记录当全文语义证明。网络“仅经gateway”是声明的假设、deployment内部暴露标记不证明隔离。主调查record_hypothesis首次失败（PR facts缺来源），修正时移除PRContext；最终server明确保留缺结构化前后/风险链gap。coverage还混入无执行等方法说明，本轮未清理。

结果证明实际模型可以履行四项记录与新来源契约，不证明误报下降或生产准确率；不得以本例覆盖金额误报等旧问题。后续调查计划和断言范围审查仍需改进。

证据完整归档于/Users/worker/.codex/evaluation-artifacts/aimangebot/checks-v29-case203-20261006，逐文件SHA256复读校验。UI证据checks-v29-ui-20261006；其受控样例没有实际source IDs，不能作为生产导航E2E。

# F2 质量记录设计

第一实现切片：FindingVerification/verificationInput新增可选checks数组。每项kind=input_control|pr_causality|guards|outcome，status=supported|rejected|inconclusive，reason<=400字符，1–8新source observation IDs。最多4项、kind唯一。所有check ID须在本次顶层observation_ids中；现有新上下文、固定提交、源码、主锚点和跨仓库门禁不变。checks判断只是模型解释，不证明语义或执行。

supported提案只有四项齐全且全部supported才保留（还须原claim_coverage full及原其他门禁通过）；缺项/partial判断降inconclusive并保留提案解释和缺口。无checks历史数据只显示未记录，不重写。当前新policy明确要求新输出checks。非法结构返回不可用，符合现有strict parse失败处理。

数据流：fixed sources→fresh verifier→parse bounded checks→source validation→existing anchor/context/claim gates→aspect gate→saved result→UI sources。结果JSON additive，无DB列迁移；SARIF保存verification自动包含checks。

后续调查计划沿同样source证据边界承接，不用“无finding”替代必要契约检查；覆盖状态与调度必须另行实现和验证。跨仓库相关性筛选不得用模型无依据声明绕过现有授权/来源门禁。

## 第二切片：来源关联调查计划

Investigation新增plan（最多8项）：id、kind(input_control|pr_causality|guards|outcome|contract)、question<=400、status(pending|checked|unavailable)、observation_ids<=8、reason<=400。checked必须有本调查已关联成功源ID；unavailable必须写具体原因但不伪造源；pending不等于已检查。计划问题/种类和ID在更新时不可删除或更换，新增项仍允许至上限；省略plan继承原计划，并在继承后重新检查8000byte预算和source ID所有权。返回与保存深拷贝计划，跨组将任务ID视为该调查局部ID，旧source ID仍不可用于新组。

有计划的调查，supported/rejected要求输入、PR因果、保护、有害结果四种任务齐全，不能在pending或unavailable时被标为已解决，应继续investigating或用成功源解释完成项；无plan保留原读取/工具兼容，但最终追加明确“未记录计划”的gap，不能认证覆盖完成。重要假设要求模型先record pending plan，完成时关联source更新。没有任何ledger调查的结果也追加缺少调查记录，不将零finding视为计划完成。计划结构完整不保证语义，除来源/记录约束不推断自动安全。

UI在调查账本展示计划问题、状态、理由与source链接；历史缺少plan显示未记录，不迁移历史数据。policy升级v30。

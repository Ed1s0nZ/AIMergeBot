# F2 质量记录设计

第一实现切片：FindingVerification/verificationInput新增可选checks数组。每项kind=input_control|pr_causality|guards|outcome，status=supported|rejected|inconclusive，reason<=400字符，1–8新source observation IDs。最多4项、kind唯一。所有check ID须在本次顶层observation_ids中；现有新上下文、固定提交、源码、主锚点和跨仓库门禁不变。checks判断只是模型解释，不证明语义或执行。

supported提案只有四项齐全且全部supported才保留（还须原claim_coverage full及原其他门禁通过）；缺项/partial判断降inconclusive并保留提案解释和缺口。无checks历史数据只显示未记录，不重写。当前新policy明确要求新输出checks。非法结构返回不可用，符合现有strict parse失败处理。

数据流：fixed sources→fresh verifier→parse bounded checks→source validation→existing anchor/context/claim gates→aspect gate→saved result→UI sources。结果JSON additive，无DB列迁移；SARIF保存verification自动包含checks。

后续调查计划沿同样source证据边界承接，不用“无finding”替代必要契约检查；覆盖状态与调度必须另行实现和验证。跨仓库相关性筛选不得用模型无依据声明绕过现有授权/来源门禁。

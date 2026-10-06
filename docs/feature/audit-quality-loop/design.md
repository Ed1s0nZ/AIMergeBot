# F2 质量记录设计

第一实现切片：FindingVerification/verificationInput新增可选checks数组。每项kind=input_control|pr_causality|guards|outcome，status=supported|rejected|inconclusive，reason<=400字符，1–8新source observation IDs。最多4项、kind唯一。所有check ID须在本次顶层observation_ids中；现有新上下文、固定提交、源码、主锚点和跨仓库门禁不变。checks判断只是模型解释，不证明语义或执行。

supported提案只有四项齐全且全部supported才保留（还须原claim_coverage full及原其他门禁通过）；缺项/partial判断降inconclusive并保留提案解释和缺口。无checks历史数据只显示未记录，不重写。当前新policy明确要求新输出checks。非法结构返回不可用，符合现有strict parse失败处理。

数据流：fixed sources→fresh verifier→parse bounded checks→source validation→existing anchor/context/claim gates→aspect gate→saved result→UI sources。结果JSON additive，无DB列迁移；SARIF保存verification自动包含checks。

后续调查计划沿同样source证据边界承接，不用“无finding”替代必要契约检查；覆盖状态与调度必须另行实现和验证。跨仓库相关性筛选不得用模型无依据声明绕过现有授权/来源门禁。

## 第二切片：来源关联调查计划

Investigation新增plan（最多8项）：id、kind(input_control|pr_causality|guards|outcome|contract)、question<=400、status(pending|checked|unavailable)、observation_ids<=8、reason<=400。checked必须有本调查已关联成功源ID；unavailable必须写具体原因但不伪造源；pending不等于已检查。计划问题/种类和ID在更新时不可删除或更换，新增项仍允许至上限；省略plan继承原计划，并在继承后重新检查8000byte预算和source ID所有权。返回与保存深拷贝计划，跨组将任务ID视为该调查局部ID，旧source ID仍不可用于新组。

有计划的调查，supported/rejected要求输入、PR因果、保护、有害结果四种任务齐全，不能在pending或unavailable时被标为已解决，应继续investigating或用成功源解释完成项；无plan保留原读取/工具兼容，但最终追加明确“未记录计划”的gap，不能认证覆盖完成。重要假设要求模型先record pending plan，完成时关联source更新。没有任何ledger调查的结果也追加缺少调查记录，不将零finding视为计划完成。计划结构完整不保证语义，除来源/记录约束不推断自动安全。

UI在调查账本展示计划问题、状态、理由与source链接；历史缺少plan显示未记录，不迁移历史数据。policy升级v30。

第三切片（回归发现的记录修复）：更新省略PRContext继承已保存上下文，继承必须在来源所有权与BASE/HEAD来源校验之前，且仍受8000byte含继承数据预算。显式提交无效/空对象仍拒绝；不自动修补缺失source IDs或关系。调查getter深拷贝PRContext，避免返回值污染已验证账本。对应AC003/005/012，保留原证据边界。

第四切片设计草案：下一切片：风险优先的有界分组调度。仅使用固定PR的路径和diff文字中的通用输入/身份/保护/危险操作信号，作为词法调度提示，不能生成finding或支持语义结论，不要求语言解析器。未知语言/无命中权重仍为1，最大权重3，排序稳定；每组保留至少一次调用机会，剩余调用预算/时间按剩余权重分配，保持全局原上限。排除与未处理文件继续明确记录，不以低权重当作已检查。测试排序、删除保护信号、未知扩展名、输入顺序稳定、剩余预算极小、总上限、分组调用预算及进度展示。未测真实准确率不宣称质量提升。

第五切片（真实失败驱动）：原生Git的执行器不可用(exit69或找不到git执行程序)标为ErrRepositoryUnavailable；普通路径缺失/无搜索命中/一般exit128不归此类。不公开stderr。auditTools保留fatal标记，下一model请求前取消；Primary返回明确基础设施coverage并保留已接受finding/ledger/trace。grouped遇到fatal后停止其他组和补充模型、标明未处理，不将已读源说成全部未读。相关取消/租约门禁继续生效。SDK模拟验证fatal只发生一次HTTP请求、不收费重试、普通源错误可继续、分组后续/综合不调用、已接受证据保留。Git许可阻断下先做独立专项；全量Git回归与phase提交推送待合法Git恢复，不宣称完成。

第六切片：通用预算收尾与停止原因。保持agentGraphSteps/原配置预算不变。仅primary消息重写在原system指令后追加server-owned当前决策序号/上限；最后三轮提醒记录来源关联计划、明确缺口并收尾，最后一轮要求最终JSON而非新工具。覆盖上一轮提醒，不累计历史提醒，不把预算元数据当源码或私有思维。提示不能证明完成，忽略仍由原SDK上限停止。识别compose.ErrExceedMaxSteps，用明确coverage说明；AuditGroupProgress新增可选stop_reason（server-owned，模型不能声明组状态），评测receipt由此保留分组失败类型，解决AuditGroups返回nil造成typed原因丢失。普通历史数据不回填原因，未知错误保留未知类别，不能靠字符串猜因。

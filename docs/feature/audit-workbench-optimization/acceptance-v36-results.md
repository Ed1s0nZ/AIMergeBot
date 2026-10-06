# v36 首次新样本验收结果

首次冻结隔离输入，代码4946659967886825f2fc32db82e8de3ab10afe35；协议acceptance-v36-protocol.md。六例全部incomplete、exit0、无HTTP402，用量均完整，总588877tokens；价格未配置，货币费用未知。不是生产benchmark或外部专家盲审；作者真值独立于被测模型，但机制与旧语料重合。此次输出不得用于把已见语料继续称为首次验收。

| 案例 | 输出/定位 | 判断及反证 | 链路/记录/验证 | tokens |
|---|---|---|---|---|
| 801 Go支付审批 | 1，BASE release.go:7命中 | 在拥有草稿记录条件下，新授权返回与审批不变量冲突，方向正确；不宣称银行已转账 | BASE/HEAD与caller源码均引用，Handle→Release cited关系已记录，独立四checks supported/full；草稿记录取得途径未知保留；两次错误提交纠正后仍Tool failed | 151515 |
| 802 修复 | 0 | summary识别重新加入审批防护，无新增危险后果 | 四项计划检查、真实多文件读取；Claim描述安全修复却status rejected，语义极性错误；运行方法与未知外部调用保留 | 59225 |
| 803 TS→PHP SQL | 1，HEAD gateway.ts:6命中 | 明确输入从常量变为可控，经表单进入SQL拼接；条件风险方向支持 | 固定双方源码与路由映射已读，relationships缺项；独立supported/full不等于完整路径。description提到UNION/stacked仅依赖未知driver条件，不可算执行证据 | 124619 |
| 804 参数绑定负例 | 0 | summary识别PDO常量模板+绑定保护，不因可控字符串本身报漏洞 | 下游及映射已读；兼容Claim却rejected；一次父子来源链接错误修正后仍Tool failed | 92087 |
| 805 Go→Python缺省兼容 | 0 | JSON发送源码可见，Python default25与BASE25一致且范围限制保留 | 已读固定双方，关系仍inferred；兼容Claim却rejected；外部调用未查；一次记录错误修正后仍Tool failed | 78826 |
| 806 Ruby历史sink | 0 | 正确排除与常量日志改动无因果关系的旧命令执行 | 本次补调查账本，但安全Claim被rejected；曾用目录ID作source及缺反证ID被拒绝，修正后仍Tool failed | 82605 |

正例两条锚点命中、负例四条无告警是输出观察，不以此计算100%准确率或正式TP/TN；六例仍未完成，完整链记录仅801有实际调用关系支持，跨项目路径仍有条件/缺项。801独立full是模型判断，原始风险“取得draft记录”的条件来源未完全建模，不能据此宣称语义完全正确。四个负例说明防护/兼容/无关历史判断获得部分证据，但Claim状态极性问题再次出现。所有错误与未完成保留，不选择成功重试替换首次结果。

新诊断功能此次未出现invalid_observation，不能声称真实故障定位改善已被该轮证明；本地SDK/race证据见implementation.md。4946659完整CI success：https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37423916778。

额外定位：agent_explore.go unresolved读取pending；agent_tools.go pending以queryKey(name,args)跟踪，queryKey包含全部字段（只删cursor/page）。因此记录/提交验证失败后，即使模型修正来源ID或side并成功建立同一逻辑产物，新参数也无法清除旧pending；其后覆盖继续显示Tool failed。这是恢复建模问题，需要区分历史失败和未解决工作，不能简单过滤所有process错误或按宽泛investigation_id清空，以免隐藏漏提交的另一候选。后续修复须有明确产物身份/显式纠正关系，并保留原trace、全部源失败与分页不足，不改变静态证据门禁。

证据目录 /Users/worker/.codex/evaluation-artifacts/aimangebot/acceptance-v36-new-20261006。298文件SHA256逐项验证，freeze与metadata代码/语料/预算匹配；私有合成源码未纳入仓库。

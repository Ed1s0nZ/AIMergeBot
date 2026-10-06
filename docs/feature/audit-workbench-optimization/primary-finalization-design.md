# v47 原预算内最终记录收尾

P10/F2；Confirmed R3 AC003/005/006/011/012和v46实际802(primary5/15)/803(primary9/15)提前JSON回执齐备。新typed工具已真实采用，但单纯导航可被忽略。目标是在模型首次合法最终JSON仍有可处理结构缺项时，明确请求一次记录收尾；它仍可保留未知，不自动创建事实/补ID/改cited，不要求全部completed，不重启primary或预算。

选用primary ToolCallingChatModel薄适配，而不重复agent.Generate/reset图计数或重建源码history。适配共享每个primary审计的硬decision counter，经WithTools绑定仍共享；每次实际Generate决定都计入原MaxSteps（2..100），保留同ctx、原80工具计数、全审计token预算、compression独立边界、SDK callbacks/provider使用量。最后decision返回tool_calls即typed预算错误，不能在额外图节点执行工具。Streaming不用且显式拒绝旁路；supplemental model不包此适配，原共享40/60保持。

当返回无tool_calls、ParseResult合法、剩余>=2、ctx仍有效，且当前ledger缺context/双侧/关系/未收尾计划或有inferred关系，才在同Generate内额外发送一次明确收尾请求。同一audit最多一次，后续模型仍可最终返回原未完成；再次无进展不循环。输入复用实际本次模型history，追加这次assistant最终JSON和固定server纠正请求；系统仅放原trustedPrompt、实际decision数字与现有runtime元数据，原Claim/path/source正文不提升为system。请求强调复用实际BASE/HEAD IDs、record_pr_context、对inferred关联检查实际可用配置/契约、保留必要unknown，不能为了清缺口制造关系。新模型的工具调用交回原React ToolsNode执行，继续同ledger/检查点/固定授权来源。模型触发收尾前把其合法finding proposals经原acceptFinding门禁保存，避免补录期间取消/transport失败丢掉已验证候选；不接受模型investigations/metadata/review或自动清除失败。

adapter需覆盖原预算提示，显示真实累计decision（包含收尾追加请求）而非SDK节点次数；source消息保持，prompt不累积。Graph仍原2*MaxSteps-1，硬counter进一步防止wrapper内部调用或SDK重新绑定扩大决策预算。无新增API/DB/UI结构，仅policy47标明行为。

Workflow Gate：P10 backend控制适配，Confirmed R3/design/实际回执具备；允许设计，无新用户选择/权限，实施须先完成F3与SDK边界检查。Maintainability：agent.go336行一处薄委托；model预算单职责新文件，primary_round_budget纯提示helper共享；medium（callbacks/限额/早停/候选保存），adapter_extraction，不广泛重构。

必要验证：实际SDK early-final→明确补context/source→final、完整/必要unknown/无余量不续、第二次final不循环；WithTools共享实际counter、最后decision工具阻断、失败/取消前candidate通过原锚点/来源门禁保留，拒绝无证据candidate；真实usage无双callbacks/未知cost不编造；原主预算/压缩/group/取消/lease/token/独立复核回归、race/full/vet/CI；冻结公共预算及信任边界独立检视后原预算802/803各一次，不盲重跑。旧报告不回填，未自动合并部署；可回退adapter连接和policy，旧工具继续可用。

# v46 原子PR上下文记录适配

F2/P10；Confirmed R3 AC003/005/006/012、v45真实802/803已读却关系漏记的回执已具备。问题不是缺读工具，而是update_investigation必须重复Claim/plan/evidence/status等完整payload，模型只补摘要却漏双侧和关系。新增薄typed工具record_pr_context只更新已存在调查的PRContext及显式新增source IDs，保留原Claim、status、计划、正反证、next_steps及server-owned review。它不是自动语义推断、自动完成或强制cited；实际质量仍需真实回归。

输入id、claim必须精确匹配当前调查，pr_context为完整替换的现有PRInvestigationContext结构，observation_ids/counter_observation_ids显式增量合并到原列表；不自动从context抽取ID，不接受新增调查/状态/计划/复核字段。缺context、错误ID/Claim、超过8000字节输入或合并后完整调查、未知/失败/错阶段/授权快照来源、错误BASE/HEAD、重复contextID均拒绝且原ledger不变。锁内读取当前记录、构造新副本、验证、仅最后保存，避免旧副本覆盖并发更新；source/trace/门禁复用现有实现。返回完整实际记录及原recording_gaps，evidence_eligible=false。

流程：源读取→record_hypothesis/plan调查→record_pr_context完整双侧与来源关系→update_investigation实际Claim判断→submit_finding；已有finding的saved PRContext不自动改写，必要时明确重提。导航link_pr_context/link_pr_sides/record_relationships指向新工具，并给出真实字段from/to/relation/certainty/observation_ids，强调matching names不证明关系、inferred/unknown不能升级。不增加decision/tool/time预算。旧update路径继续可用，历史报告不回填；策略v46确保运行版本可追溯。

恢复：record_pr_context单独recording family，仅同id/claim的后续成功context工具调用可明确resolve_recording_errors；不与record/update/submit跨family消除失败。历史trace不删，语义/来源缺口仍保留。回退工具注册、导航和策略即可，存储结构不变。条件风险过强与旧反证同步属于后续必要工作，此适配不能声明它们已解决。

Workflow Gate：phase P10/F2；backend adapter；required artifacts为Confirmed R3、现有PRContext/source契约与v45原始回执，齐备；implementation allowed yes，无新权限/UX/产品决定，先计划提交后生产改动。Maintainability Gate：agent_investigation约216行一处注册、新adapter单职责；medium（model typed contract/原子ledger）；adapter_extraction，无广泛重构；现有验证复用，校验顺序与锁不放宽。

F4实现：record_pr_context已注册typed SDK；锁内以当前记录合并显式IDs，完整输入/合并后的8000字节门禁及原20条/列表门禁保持，额外要求唯一trace、primary stage、evidence_eligible=true、输出ID匹配、固定授权SHA和非空来源，防止复核结果回灌初审。只最后保存，原字段不重写；独立pr_context错误family不与完整调查/update混用。新的source-linked关系仍由模型显式记录，不检验名字为真语义，unknown/inferred保留。后续条件风险措辞及旧反证同步尚未处理。

F5初步：新增实际本地固定Git正反来源门禁/原子失败、输入及合并超限、错side/context冒充主侧/未知ID/错Claim、复核stage/duplicate trace/noneligible/stale/outputID、保留字段/旧finding快照与明确重提、同family显式纠正及失败历史、并发source merge；实际Eino SDK生成工具schema无status/plan/verdict可写，七轮源读→record→context补录→assessment→final真实链路与进度验证通过。定向test33.751s，先前基础定向45.147s，vet/diffcheck通过；相关调查/PR/纠正/SDK/只读/取消/分组定向race60.011s通过；full/精确CI和独立新契约检视待最终结果，不把synthetic SDK称语义模型效果。

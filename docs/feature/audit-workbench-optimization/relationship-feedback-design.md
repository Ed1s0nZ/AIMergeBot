# v49 关系字段schema与错误反馈

Workflow Gate / F0-F2：P10 backend迭代，Confirmed R3 AC003/005/006/011/012，原真实v48 input/trace/固定source及失败报告齐备；source9590d98 clean。实现允许，无新用户决策/权限/运行仓库代码，仍分支交付不自动main/部署。问题：relationships[0].to为空九次同泛化错误，原15决策预算正确停止但资源耗于无效修正，未进入independent阶段。

当前SDK v0.9.21实际InferTool反射已把非omitempty关系字段设required，但无minLength/maxLength/enum/minItems约束；required仅排除字段缺失，不排除空字符串。使用既有共享InvestigationRelationship jsonschema tags声明原server限额：from/to非空≤200字符、relation≤500、certainty只cited/inferred、source IDs1..8；required策略不变。约束描述保持语言无关、不自动引用/推断目标、不把inferred改cited。JSON公共字段/Store/UI/SARIF不变；工具schema更准确但原合法请求继续兼容，无新增API字段或模型轮次。

validatePRContext原复合条件拆为固定字段检查，错误指出有界pr_context.relationships[index].field与非空/UTF8/NUL/长度限制；certainty给合法枚举。只输出服务端固定字段/限制/index，不反射用户值/Claim/源码。目标或关系未知时允许显式省略该edge并保留unresolved_edges，不填造关系，不放宽任一validation或SOURCE/固定SHA/Claimidentity/atomic/historygate。记录工具description强调同schema边界。

Maintainability Gate：pr_investigation118行单职责typed PRcontext，注册/记录仍薄层；low-medium，proceed narrow，不广泛重构。新增小字段诊断helper或固定loop，最大8关系、每次第一个失败，固定长度；原校验顺序中大小/来源门禁不改，ledger失败保持原子，预算/重复读/停止机制不改。

验证：实际SDK发出的nested关系schema必需字段/长度/enum/数组约束；失败字段缺失/空/whitespace/过长/Unicode/非法UTF8/NUL/未知certainty、多条关系具体index，err不echo秘密字符串，原valid inferred/cited继续保留，不自动升级事实；工具失败atomic与显式修正/原failedtrace保留、错误feedback不eligible；原实际v48请求离线重放明确To缺项，显式已知目标或省略edge的正例通过原source门禁，无模型accuracy宣称。相关PRcontext/记录/恢复/finding/primaryfinalization、SDK/race/full/vet/精确CI；冻结当前公开tool schema/信任边界独立re-review后原803一次known regression。模型仍可忽略反馈，实际未验证或失败照实报告，保持AER001 open，不扩大15/80/240或复核pool。回滚tags/细化消息，原门禁始终保留。

F4/F5：shared DTO jsonschema原required字段继续，实际SDK四工具（record/update/context/submit）均发出from/to min1/max200、relation min1/max500、certainty双枚举、IDs min1/max8；server逐字段错误指明index/field/限制与未知edge省略路径，不echo值。Unicode边界/空白/NUL/非法UTF8/长度/枚举negative与valid cited/inferred、记录失败atomic通过。actual SDK先空To拒绝→显式补目标→显式resolve，原Claim/status/Plan/inferred和失败trace保留、三actual tool计费保持。相关PR/context/source/record/correction/finalization/compression race62.495s、vet/diffcheck通过，初始定向1.801s。

真实v48原第一个坏request及原snapshot/原source trace/原保存ledger离线overlay1.042s验证：空To定向字段且原子失败；显式已读query.php目标（mapping unconfirmed、inferred保留）或省略未知edge且保留unresolved可接受，未填新源或改原判断。外部v49-frozen-v48-field-feedback-20261006四载体文件SHA核验（另有manifest），源文件hash/原receipt hash齐备，原坏回执未修改。只是字段反馈/门禁契约，不是模型采用或语义正确证明。full exit0 platform186.615s；精确CI/冻结独立检视和原803真实效果仍待。

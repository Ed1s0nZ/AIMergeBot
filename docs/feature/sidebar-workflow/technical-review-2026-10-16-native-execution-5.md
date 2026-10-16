# Technical Plan Re-review：凭据轮换与事件防重

- 日期：2026-10-16；Codex；架构交接；snapshot e4c74fa。
- 范围：execution/budget/API diff/resource/publication/legacy-transition及S3依赖合同，重核NTR-001–007；替代第4轮决定。
- Final decision：**Fail**，NTR-007已在设计层关闭，新增NTR-008 Major。

## Mandatory Criteria

| Criterion | Status | Evidence / gap |
| --- | --- | --- |
| Goals / fields | Pass | scope v2、legacy authority、profile v2及新旧兼容字段已给出 |
| Data flow | Fail | receipt authority与事件防重身份被混为同一个credential revision键 |
| Lifecycle | Fail | 只换token即可重放同签名事件并重新准入 |
| Diagram | Pass | 全链及异常分支已具备 |
| Handoff | Fail | 先分开防重scope与执行authority，防实现者自行选择 |

## Old Findings / Qualified Areas

NTR-001–006设计修订保留；census持久错误/单owner、普通legacy新任务统一固定scope、全部producer已定义。NTR-007通过独立legacy_fixed_session_v2/material version2、startup/Save/Reload统一producer、同配置切profile也推进generation关闭。资源基线没有假称硬沙箱；API差异不依赖compare.files；scope/账本/覆盖与未知发布处理有明确传播。均不是生产验收结论。

## NTR-008 — Major：防重scope包含凭据代次，使轮换重开旧事件

- Evidence：legacy-transition“后台主体与原webhook”定义去重scope包含credential_revision。publication原合同要求同绑定内跨integration revision保持body去重；新scope规范覆盖了该不变量。
- Trigger：只轮换repo token，hook secret不变。旧合法body/HMAC被重放，新的credential revision产生另一个去重namespace；完整policy/scope又因revision改变不等于旧run，不能依赖run查重兜底。
- Impact：同一事件可再次消耗模型/读取预算、产生重复运行及通知；凭据授权版本与事件身份不应混用。
- Required fix：将receipt不可变执行authority（包含generation/revision）与事件去重scope分开。token/代理轮换不重置同repository identity/binding scope的body/delivery防重；重复返回原receipt，不给旧receipt换新凭据执行。endpoint/身份/明确binding版本切换的namespace规则独立定义。
- Verification：相同secret下token/proxy轮换→旧body配新delivery仍返回原receipt、零新budget/run；旧pending若凭据失效按失败收敛而非重绑；真实新body能按新authority接受；secret变更后旧签名在去重查询前拒绝。

## Implementation / Re-review

完整N1–N6仍待实现；当前仅静态复审，未运行外部服务。先补该唯一阻塞规则再审。实现时须按已定义真实SQLite/CGI/API/模型fixture证据验证，不以当前设计Pass代替生产完成。

# Technical Plan Review: Git 子进程取消修复

- Review date：2026-10-16
- Reviewed artifact：git-process-cancellation-plan.md @ c4f47b0
- Reviewer stance：同一agent另轮架构交付检查；非独立审计
- Final decision：Pass
- Decision summary：仅允许已复现的进程取消窄修复；不批准native factory、整次资源配额或其他平台树取消。

## Handoff Judgment

生产函数与合成回环反例已定位。Go1.27.2本地os/exec源码明确默认Cancel仅Process.Kill，WaitDelay默认0可能等待后代继承管道。方案对Linux/macOS进程组、错误分类和其他平台边界明确，可交付开发。

## Mandatory Criteria

| Criterion | Status | Evidence | Gap / required fix |
| --- | --- | --- | --- |
| Requirement background and goals | Pass | Confirmed REQ/用户约束；真实Git取消反例 | 无 |
| Data structure clarity | Pass | Cmd三个字段、内部helper与平台tags | 无 |
| New/changed fields highlighted | Pass | 字段矩阵；无API/schema/config变化 | 无 |
| Data flow clarity | Pass | Start→helper请求→cancel组kill→Wait→fence/ctx | 无 |
| Core object lifecycle | Pass | 每命令Cmd、无共享PGID；Run拥有Start/Wait | 无 |
| Flow diagram | Pass | sequenceDiagram含调用/网络/取消 | 无 |
| Developer handoff readiness | Pass | 独立平台helper、真实取消与成功/fence验证 | 无 |

## Qualified Areas

- 不移除身份guard，不把当前Git修复冒充未完成原生接入。
- 组取消只用于独立Setpgid的cmd；无Process/PID<=1拒绝，ESRCH按已退出处理。
- 30s执行deadline与2s退出管道等待分开，不能把后者误称整次审计资源上限。
- 失败原始stderr继续隐藏；无自动重试/外部状态变化。

## Disqualified Gaps And Risks

无阻塞缺口。Minor：正式回归应使用同一观察deadline等待命令与上游，避免原临时测试的即时default造成调度误判。该修改已在方案步骤2明示。其他OS树取消与主动逃逸后代不在此批准范围，但不能在交付文案声称已覆盖。

## Data Structure Review

Cmd.WaitDelay=2s为共享helper拥有的固定内部策略；Unix SysProcAttr.Setpgid=true及Cancel替换仅darwin/linux；其他平台默认Cancel保留。无数据迁移。helper不能覆盖调用环境/args或重新创建背景ctx。

## Data Flow And Lifecycle Review

每调用创建独立组，ctx取消由os/exec触发group kill；正常成功不kill，输出继承在WaitDelay结束时返回错误而非成功。Cmd不跨调用共享，避免错误清理其他审计。进程组不是沙箱，磁盘/内存配额仍需后续设计。

## Flow Diagram Review

已有sequenceDiagram，覆盖Start、HTTP请求、取消、Wait和错误返回；与实现路径一致。

## Implementation Readiness

git_repository.go只加委托。新增git_process.go和互斥平台文件、git_process_test.go；需要平台syscall的额外测试独立unix测试文件。临时研究fixture转换后删除原文件。不改主依赖、UI或factory。前一代码CI终态前不推代码。

## Verification Readiness

实测baseline exit1（1.04s）：command_returned=false、upstream_canceled=false；夹具主动断开后退出。需修复后真实fetch网络取消、预取消零请求、正常本地读取、目标race、完整Go/platform race/vet/build；Linux由CI实际执行，macOS本地执行。管道继承用受控shell/sleep测试时必须显式清理同组后代，不能仅靠测试timeout。交叉编译不代表其他OS树取消通过。

## Modification Recommendations

实施后以实测结果更新方案与CHANGELOG，仍明确UAR-001及整体目标未完成。不要顺带调整共享预算或更换协议库。

## Re-review Requirements

若增加Job Object、扩展支持OS、改变信号/超时语义或修改factory，必须重新设计评审。本Pass不覆盖新增行为。

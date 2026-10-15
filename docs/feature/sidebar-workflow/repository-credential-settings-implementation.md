# S3b：配置保存与凭据激活状态机

2026-10-15；codex/sidebar-workflow；F4/F5，基于d70ddf1及已推S3计划/技术审查。当前为Settings薄hook、私有启动绑定和故障恢复协议实现；main/Runner.Start尚未绑定激活器，不能称平台撤权已启用或完整S3b/S3已交付。

## 工作流与可维护性门禁

P7/P8；前置Confirmed requirements、repository-recovery-contract、S3 F3实施计划及technical-review-2026-10-15-credential-activation.md Pass存在。Maintainability allowed adapter_extraction/narrow_fix：从Settings提取47行文件持久化，约194行激活状态编排、58行事务取消独立，Settings仅保存委托/快照clone；不增加新菜单/HTTP/API/配置字段或网络操作。完整REQ/AC与UAR-001/002/004保留。

## 当前改动及消费者

SettingsService可在启动阶段私有绑定Store激活器；绑定后Save与项目同步均经过同一状态检查。未绑定的原服务/standalone保持旧保存路径，目录fsync只在激活保存路径启用，避免提前改变现有服务的跨层失败行为。global Settings.Revision达到uint64上限明确拒绝，不回绕0。

激活器发布不可变private view（phase/epoch/generation/material/config）。runtime descriptor无须持Settings写锁，capture/validate在pending时立即拒绝，先后读取view并校验DB，不能只信内存版本。Settings.Snapshot保持复制项目、上下文、enabled指针和其他slice，发布快照不共享调用者可变slice。

凭据变更先pending+新epoch，再原子文件替换/目录fsync，再短SQLite事务。事务内重新校验原generation/material（防文件写入期间另一连接旋转），更新generation并取消旧legacy任务；commit后才一次发布完整内存配置和active view。取消ID仅提交后通知本地interruption callback；Store事务不回调Settings/Runner。模型或Projects变更不旋转凭据、不取消已有任务。

取消使用真实Snapshot和严格AuditPolicy/scope解码判断：旧nil或坏/超限policy保守停止；新scope逐target/source/context检查legacy凭据。合法纯integration scope不因旧GitLab token变化取消。尚无生产native producer，测试用临时库注入未来合法scope，不冒充已经开放执行。

## 故障行为

| 窗口 | 已执行的真实断言 |
| --- | --- |
| 文件替换前明确失败 | DB与内存/文件保持旧配置，fresh descriptor仍可用；旧进行中descriptor因epoch改变被拒绝 |
| 文件已替换但目录持久性不明 | 文件保留新配置，DB未推进，内存不半更新；pending拒绝普通Save/项目outbox再次写文件 |
| DB取消任务语句故障 | 通过RAISE触发器验证generation与取消均回滚；已写新文件保留，gate保持pending |
| DB提交后、内存发布前中断 | generation和取消已共同提交；内存仍旧且pending，不对外假称新配置生效 |
| 另一连接在写文件期间旋转 | 事务内旧版本再次校验失败，不覆盖另一连接状态；本地文件等待重新激活 |
| 同进程writer停在文件写入 | capture/旧descriptor立即拒绝而不等待Settings.mu；Snapshot仍完整旧值 |
| 以上状态重启 | 真实重新OpenSettings+绑定同Store，按落盘文件激活；已提交同值不重复递增，未提交则新代次；旧run终态保留 |

固定错误不包装文件路径、SQL触发器、proxy/SDK或测试秘密正文；fmt/private JSON继续由前置协议保护。任何rename尝试后的错误均保守按replacement possible处理，不能把未知结果当未保存。pending模式不通过普通Save覆盖文件；同进程显式reload尚待S3d。

## 验证证据

Go1.27.2，合成配置和真实临时SQLite。无仓库/模型网络或真实通知；测试进程无主服务启动。

- 新状态机及legacy/Settings回归专项exit0，1.322s，/tmp/aimangebot-s3b-targeted.log。
- 完整go test ./... exit0，platform102.466s，其余通过/无测试，/tmp/aimangebot-s3b-full.log。
- go test -race ./internal/platform -run 'TestCredentialActivation|TestLegacyCredential|TestStaleSettings|TestProjectSync|TestRepositoryIdentity|TestPlatformSchema' -count=1 exit0，10.753s，/tmp/aimangebot-s3b-race.log。
- go vet ./...、go build ./...、frontend npm run typecheck及git diff --check均exit0。前端生产源码和锁文件未变，不生成dist。

## 尚未启用的生产边界

main与Runner.Start未调用bind，实际legacy reader、模型派发、发布消费者尚未消费descriptor，现有平台仍沿原路径；不能用这些测试声称撤权时真实HTTP/模型请求已经被阻止。S3c必须逐条接通并断言真实受控HTTP次数，S3d必须在已有配置页提供pending诊断/显式reload，才可正式启用。

启动assembly还须在激活前取得worker singleton所有权，并在激活事务核对该owner/租约；不能第二实例先改变generation/取消任务，随后才因Runner.Start租约冲突退出。当前bind为未暴露的协议入口，仅用于隔离测试；下一切片先明确Runner.Start acquire→激活→Recover→启动loop的顺序和失败release，不能直接在main的OpenSettings后调用。此约束保留到S3c的具体计划/复审，不在本切片猜测所有权。

下一步S3c完整runtime/启动/公开错误传播，S3d同进程reload/配置反馈，以及S4–S6原生审计与恢复仍未完成。无main合并、真实部署或外部联调。

# S2：永久仓库身份与schema2迁移实施记录

2026-10-15；codex/sidebar-workflow；基于5575c9cd841695d81637e3c489a845369d49f984。F4/F5阶段记录；不宣称整体需求或UAR-001完成，不合并main/发布/部署。

## 工作流与可维护性门禁

P10维护，F4/F5继续已确认仓库恢复目标。F2合同与评审(repository-recovery-contract.md、technical-review-2026-10-08-repository-recovery-3.md)、已推F3(repository-recovery-implementation-plan.md、a6fe8e7的repository-identity-migration-plan.md)均存在，S2实现允许。没有新增未确定产品操作；恢复写入、凭据激活、provider factory仍按后续切片推进。目标是让项目身份不随binding行丢失、配置同步或ID复用而改变；不是只增加无消费者的数据表。

Maintainability：narrow_fix/adapter extraction；Store289行仅委托迁移，身份读写88行、迁移111行、历史健康分类169行分离。现有project access/sync/create/binding消费者直接使用登记，未把网络请求或恢复状态机塞入Store。高风险面为数据迁移/ACL归属；真实SQLite、HTTP访问及回滚反例覆盖，不需要先做无关重构。

## 已实现与消费链

- schema2身份登记、恢复回执结构和私有legacy state在同一SQLite事务创建，最后推进version。新空库不制造配置同步任务；有旧项目则标outbox dirty。
- 已有binding、历史或native创建receipt任一存在即internal。历史/current/receipt结构、原始摘要与原始binding不一致标repair_required，不用当前rename/rebind否定合法历史回执。不读取凭据验证身份。
- 新旧项目保存、原生创建、普通改绑、配置导入导出、项目可用性与legacy执行guard都消费永久登记。internal缺binding不能改回legacy，也不能用普通保存伪装修复；登记丢失不能靠重启补回。
- 普通改绑递增永久identity revision；native首次创建保持1；溢出回滚binding、history、event、outbox所有写入。
- 状态密钥仅首次随机32bytes，generation0/fingerprint NULL，尚未激活为凭据版本；schema2重开缺表/缺key/缺登记明确拒绝，不能静默重置。
- 升级前拒绝未到期或畸形的活worker租约；旧pending/running停止为failed并说明重新提交，保留原result/trace、不自动新建任务。
- 新内部项目ID上界包含历史run目标/来源/context，不复用历史引用；schema1已有native receipt与legacy run引用碰撞时整体拒绝迁移，等待离线核对，不猜测归属或移动旧报告。

## 真实反例与修复证明

新增TestRepositoryIdentityAllocatorReservesHistoricalProjectIDs先在原分配器执行：target/source/context三例均复用3，真实登录新项目成员读取历史报告返回HTTP200；负例日志/tmp/aimangebot-s2-reservation-before.log。修正为保留历史ID后新建ID4，真实HTTP返回403/404且不包含旧证据。不是只比较分配器返回值。

迁移矩阵覆盖legacy/bound/native、正常rename/rebind、坏JSON/坏history/缺current/receipt摘要矛盾/超限/错误SQLite类型；缺state/key/表/version/登记重开拒绝且结构与私有状态不变；活worker拒绝及迁移末尾version触发器故障证明全部DDL/版本/outbox/run回滚；已发生native历史碰撞拒绝。13个原测试的合法项目fixture改走SaveProject，保留原业务断言；故意损坏schema1的测试继续手工建旧库。未加自动legacy触发器绕过登记不变量。

## 本地验证

macOS，所有新验证选择GOTOOLCHAIN=go1.27.2；合成凭据与受控fixture，无真实消息、工单、检查更新或模型调用。

- S2专项 identity/schema/project/binding/scope：exit0，8.259s，/tmp/aimangebot-s2-targeted.log。
- 完整go test ./...：exit0，platform109.654s，其余通过/无测试，/tmp/aimangebot-s2-full-final.log。
- 功能落地审计混合专项：exit0，3.829s，/tmp/aimangebot-usability-audit-5575.log。
- go vet ./...、go build ./...、npm --prefix frontend run typecheck：exit0，前端生产源和锁文件未变，不重新生成dist。
- 专项race：go test -race ./internal/platform -run 'TestRepositoryIdentity|TestPlatformSchema|TestRepositoryProject|TestProjectIdentity|TestRepositoryBinding|TestRepositoryScope|TestProjectAccess|TestLegacyImport|TestInternalProject' -count=1，exit0，55.397s，/tmp/aimangebot-s2-race-final.log。
- git diff --check：exit0。精确HEAD CI待本切片推送后核实，不用旧HEAD绿色替代。

早期完整回归曾发现13处直接SQL建project不符合永久登记，已改公共fixture并保留断言；没有删测试掩盖失败。后续完整回归已通过。

## 旧程序与备份兼容证明

冻结的schema1 Store可执行文件/tmp/aimangebot-s2-old-store来自5575c9c，未用新源码重编。实际以旧程序create创建schema1库，经当前OpenStore升级2并写入新project5；旧程序open升级库明确报unsupported platform schema 2，拒绝前后sqlite_master、projects、登记与私有state完整相等。升级前SQLite一致性backup可由旧程序打开且没有升级后project5。临时程序、DB与备份均自动清理，没有触碰用户库。

这是实际旧Store二进制和SQLite备份证明，不是应用与真实Settings配对备份/部署演练。部署仍须停止全部旧实例与连接（租约检查不能证明连接已关闭）、备份DB与Settings/秘密配置、完整provider与恢复消费者验收。回退schema1必须停新程序并恢复配对备份，不能改version、删登记/密钥或仅替换旧二进制。

## 交付边界与后续

当前可核验身份隔离和迁移，不代表“仓库绑定后可审计”。UAR-001仍open；S3凭据激活、S4原生provider全部消费者、S5恢复API/UI与S6部署/真实联调尚待完成。UAR-002/004及完整REQ/AC不从范围消失；没想清楚的操作先补合同，不能猜实现。此切片提交推送仅工作分支，精确新HEAD CI及代码检视另记录。

## S2精确HEAD远端验收

[CI38750868382](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/38750868382)已读取终态：headSha=edf563f9f38187c480e7014cb7689fe7e5a7dbed，completed/success。Go tests and race checks、Operations tooling tests、Reachable Go dependency vulnerabilities、Frontend build、Embedded application build全部success，无skipped。仅关闭S2该提交CI待验，不关闭UAR-001、不表示S3/S4已完成或生产部署。

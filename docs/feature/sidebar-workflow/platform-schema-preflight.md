# S2前置：迁移前格式检查

2026-10-15；codex/sidebar-workflow。基于已推repository-identity-migration-plan.md先落实early version guard；不是S2永久身份/schema2迁移完成。schema仍1、当前有效数据库行为保持，未碰真实用户DB。

## F4 实现

Store.migrate在任何migration statement前委托platform_schema_guard。没有版本表的旧库/新库可初始化；已有版本表必须是table、恰好1行、SQLite INTEGER且为当前支持的1。存在空表、重复行、未知/负/零版本、TEXT/REAL/NULL或缺列/VIEW时明确拒绝，不按第一行猜格式、不把已有坏表当新库。元数据名称按SQLite NOCASE查找，大小写不能绕过检查。固定错误不包含坏记录内容。

不新增新版本、永久identity表、凭据state或恢复API；S2后续实现须有完整schema2原子迁移、活旧worker检查、健康分类、创建/改绑/同步/guard消费者及旧程序验证，再切换允许版本。不能把本preflight作为schema2已落地。SQLite连接初始化的WAL PRAGMA不在“迁移语句”证明范围，不宣称拒绝打开连文件header都不改变。

## F5 本地证明

真实临时SQLite覆盖12类异常：空、重复当前、当前+未来、未来、大小写未来表、零、负数、TEXT、REAL、NULL、缺列、VIEW。每例设置迁移INSERT的RAISE触发器，证明错误来自版本检查而不是后续迁移；拒绝前后完整sqlite_master结构及sentinel保留。另验证当前1/无版本旧库能初始化并重复migrate。

最终casefold补强后专项go test exit0，platform0.582s；加入全部真实身份/绑定/快照回归的前次专项4.397s通过；schema guard当前版本的完整go test ./... exit0（platform102.386s），专项race6.273s、vet和build通过。以上使用原本地Go1.27.1；随后CI发现x/net可达漏洞，将依ci-http2-security-update.md在补丁依赖/Go1.27.2上重新完整验证，不把旧依赖的功能测试当新安全扫描通过。

补丁依赖及Go1.27.2最终工作树再次验证：完整go test ./... exit0（platform164.443s），扩大协议/身份/guard专项race51.411s、vet/build、模块verify通过；govulncheck可达0、npm audit total0。新数据库表/版本依然没有启用。精确CI要以最后依赖补丁生产提交为准，本提交的单独旧依赖图不作为安全发布版本。

## 交付范围

当前只提前拒绝不支持/损坏的版本元数据，持续保持schema1。F2/F3完整目标、UAR-001/002/004及S2–S6仍未完成；没有main合并、真实升级或部署。后续精确HEAD CI与源码复核独立记录。

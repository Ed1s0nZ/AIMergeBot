# S3d2：设置页恢复流程实施与验证

2026-10-16，codex/sidebar-workflow，F4/F5；上游c01bea5交互计划及91dcaf2技术审查均已推送后实施。当前为现有设置页对S3d1 API的消费，不启用main绑定、provider factory或发布链路，完整REQ/AC和UAR-001/002/004继续保留。

## 用户流程

已有设置页展示pending/unavailable及不可核对状态；仅合法pending、can_reload=true、无草稿、版本为正SafeInteger、未锁定且无进行中请求时允许手动从服务器文件恢复。明确说明完整文件/实际密钥/空值清除与页面草稿的区别。未绑定/active不展示恢复入口，普通保存继续既有语义。

失败/超时/409/503等结果需核对时，保留页面内容并锁定写入，只允许用户明确放弃草稿并GET核对；不自动重发或更换expected。仅明确在写入前拒绝的普通保存invalid_trusted_proxies/invalid_audit_quotas 400允许直接修正。200后再GET当前完整配置与项目同步状态；GET失败不重复写入，不误称未保存。锁定期间显示“上次读取的状态”，避免把旧pending当当前事实。

请求同步占有操作ref，禁用输入/按钮并由action guard二次拦截；20秒timeout，卸载abort并清理timer，旧请求不发布页面状态。401/403清除配置与密钥草稿；初次读取失败可重试。有格式草稿的可信代理文本也计入dirty，不能无提示丢弃。客户端不持久化秘密。

## 模块和工程边界

settings.tsx由约550行降为496行，委托use-system-settings.ts统一原load/save和新恢复。settings-recovery-policy.ts为纯操作资格判断，拒绝null/畸形/矛盾状态及不安全number；settings-recovery.tsx复用已有coverage/button样式。根fieldset统一只读，提交仍受同步guard保护。既有菜单与字段分组保持。

新增10项Node内置policy测试，使用现有TypeScript编译器，无新增依赖；npm test:settings接入CI。可复用本地合成HTTP fixture及说明置于frontend/tests，真实构建页面可重复验证请求次数/body、丢响应、权限与延迟，不依赖真实模型/仓库服务。fixture只监听127.0.0.1，永远不作为生产鉴权/后端验收证据。

## 验证证据

macOS；后端Go1.27.2；本轮无Go生产源码变更。前端最终资源index-Dkuz7mGd.js / index-cuXwW-Wb.css。

- npm run typecheck：exit0。
- npm run test:settings：10/10通过，最终166ms；覆盖active/unbound/pending、dirty/busy/locked、null/未知/矛盾状态、无快照及0/负数/小数/非number/>2^53等版本。
- npm run build：exit0；正常生成JS/CSS/index.html，不手工编辑bundle。
- go test ./internal/platform -run 'TestCredentialReload|TestSettingsReload|TestSettingsHTTP|TestSettingsPersistence|TestTrustedProxy' -count=1：exit0，2.870s。
- go build -o /tmp/aimangebot-settings-recovery-ui .：exit0，使用最终前端资源验证embed。
- python3 -m py_compile frontend/tests/settings-recovery-fixture.py：exit0。
- 上一代码HEAD e7e0a77的CI 38768825356已completed/success，所有步骤通过；本次新HEAD仍需单独CI，不能继承为本次通过。

### 浏览器和实际请求记录（合成API）

通过CUA操作真实构建页面，不注入组件state。fixture捕获请求；记录位于/tmp/aim-recovery-ui-proof/*.json，只含合成数据，未提交原始payload。初轮资源NLHkJF30用于交互探索；最终Dkuz7mGd复核正常保存、草稿失败→明确丢弃→恢复、只读/不安全/畸形状态、401、丢保存响应、初次读取失败和移动端。403等初轮用例在状态控制代码未变时完成；最终timeout/doubleclick再次复核通过：GET=2、POST=1，超时后锁定、明确GET后active且无重发。

| 场景 | GET数 | 写请求数 | 可观察结果 |
|---|---:|---:|---|
| pending成功恢复 | 2 | 1 POST | expected_config_revision恰为字符串"42"，仅此key |
| 最终构建：保存失败→丢弃→恢复 | 3 | 1 PUT+1 POST | 草稿保留、写锁；明确丢弃后按钮才可用；PUT不带readonly状态 |
| 恢复丢响应后核对 | 2 | 1 POST | 失败后不可再写，手动GET发现已active，无第二次POST |
| 保存丢响应后核对 | 2 | 1 PUT | 读取已保存model，仍只有一个PUT |
| 200后GET失败 | 2 | 1 POST | 明确成功返回但状态待读取，禁止重复恢复 |
| 双击/超时后核对（初轮及最终） | 2 | 1 POST | 25秒响应被客户端20秒timeout终止，只允许GET核对 |
| 操作中离开页面再进入 | 2 | 1 POST | 新页面正常读取active，不受旧请求影响 |
| 403 / 401 | 各1 | 各1 PUT | 403配置表单消失；401返回登录页，不保留旧密钥输入 |
| unavailable / unsafe / malformed | 各1 | 0 | 只读，不出现可操作恢复入口 |
| 未绑定正常保存 | 2 | 1 PUT | 初始无恢复入口，成功GET后显示已保存，按钮重新禁用 |
| 初次GET失败后重读 | 2 | 0 | 可重试，后续读取pending不自动恢复 |
| 确定未写入的参数错误 | 1 | 1 PUT | 草稿可继续修正，没有自动重复提交 |

密码输入在CUA中主动脱敏，不把字符串比较当作密钥保留/清除证明；合成字段发送回执、源代码状态转换与401/403后表单消失分别作为限定证据。移动390×844：document.scrollWidth=innerWidth=390，按钮换行，无横向溢出；截图/tmp/aim-settings-recovery-mobile.png。最终桌面截图/tmp/aim-settings-recovery-pending.png，确认恢复按钮可用而配置输入只读。导航验收遇到工具对侧栏链接的离屏定位错误，已核对实际项目页后使用已验证设置地址继续，未改产品导航逻辑。

## 局部复核与未完成范围

本片无新增后端可写状态/版本或远端副作用；错误/超时不调用第二次mutation；恢复动作仍由服务端核对权限/CAS/state。缺少原生factory、正式scope消费者/发布、worker owner受控启动与实际生产绑定，不因UI通过而关闭UAR-001。未实现用户暂缓的自动修复验证、人工误报自动放行、unknown工单重发。完整目标与合并/发布验收仍未完成；本片不合并main或部署。

本轮未重复运行无源码变化的后端全Go/race；已有e7e0a77全量CI及本次HTTP回归分别限定证据范围。git diff --check通过；完整新HEAD CI与提交信息在后续记录，不将前端合成API证明扩大为生产端到端。

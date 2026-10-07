# 责任人通知选择器 gate / UX / 实施计划

- 请求：最佳实践优化，完成已保存责任人路由的管理入口。
- Workflow phase：P8/F4–F5；Confirmed requirements、owner-notification-design及cdd2204后端/API存在，允许实现。
- 缺口：现有集成页未声明owner_ids，也没有候选授权状态；其保存409保留草稿但仍可继续编辑提交。
- 用户行为：在渠道主表单选择平台责任人，与渠道其余字段一次显式保存；空选为现有项目通知。不会自动填写邮箱/机器人目的地、自动分配发现或发送消息。
- API：沿用GET users、GET projects/:id/members和既有POST/PATCH integrations；owner_ids显式数组。选中账号须启用且属于全部已选启用项目（admin例外）；运行source/context由后端派发时再校验。
- UX：加载/刷新/换项目隐藏旧候选；失败可重试且保留选中ID，已选不可用账号仅显示ID及明确移除入口。多选上限100；责任人范围非空须只选复核/到期事件，不静默删除审计完成/失败事件。提供清空范围恢复项目通知；字段在busy/conflict中禁用。
- 并发：每轮候选读取AbortController；项目改变/卸载取消旧请求。候选就绪状态带项目、责任人、事件签名，父表单不能用旧scope的有效状态提交。409保留所有草稿但锁定保存/编辑，用户明确重新选择服务器配置或新建解除；401/403/404清除当前编辑内容、候选及凭据草稿并锁定，禁止继续提交旧权限配置。
- 授权：仅原管理员集成页，保存仍服务端认证/权限/CAS；所有浏览器写入只用受控fixture。

## Maintainability Gate Report

- inspected：integrations.tsx（37行，但多行极长，11种凭据/请求/渲染/错误处理混合）、owner-routing.tsx、api.ts、page-utils.tsx及新通知后端。
- trigger / risk：三职责以上与跨UI/契约，high；不能继续把候选加载/权限合并堆入原文件。
- allowed：adapter_extraction，新notification-owner-selector.tsx/css承载候选与状态验证，主文件仅类型/字段委托、状态签名与冲突禁用。
- refactor first：不做广泛格式化或重排；原请求生命周期只增加必要锁定与校验，保持现有凭据语义。
- acceptance：真实编译页面选择保存owner_ids/expected_revision；范围清空；不可用owner移除；503/403/旧请求/换项目；事件冲突不静默改动；409草稿锁定与重新选择；360窄屏、键盘多选；未点击保存不发PATCH。
- validation：npm typecheck、临时Vite build、受控浏览器验证、正式embedded build后Go build/vet/diff；无额外无关测试。
- lifecycle docs：implementation-plan.md写实现/证据，独立commit/push；最新cdd2204 CI先跟至终态后再推代码，避免取消。
- remaining：自动推荐路由、其他完整REQ仍保留；本UI完成不代替整体交付验收。

## 项目读取失败恢复补强（P9 / F2–F5）

Confirmed原需求及上述UI/后端契约允许实施。发现useResource在503保留缓存，而集成页仍渲染缓存项目、选择器仅接收loading，因此项目来源失败也可显示候选；刷新列表未重新读取projects。高复杂度原页仍采用窄委托，不广泛重排。

契约：项目请求loading/error/无data时隐藏缓存项目标签及候选，保留用户草稿ID且禁止保存（包括owner为空）；独立selector接收projectsError并阻止读取成员/使用旧候选。刷新列表同时重新读取integrations/projects，成功后允许用户继续原草稿，项目停用或消失必须明确调整；后端ACL/CAS仍是最终授权。仅GET刷新，不发送通知。既有403保存清除凭据/409锁定保持。

实施：integrations增加projectReady与项目有效性校验、错误/加载展示、复合刷新；notification-owner-selector增加项目错误状态。验证typecheck/build、真实编译产物受控浏览器项目503/403/延迟/恢复/项目停用及空owner保存限制，窄屏；Go embedded build/vet。文档阶段先提交推送；生产代码待529516d精确CI37622635264终态后推送，不覆盖live CI。更新implementation-plan证据，完整REQ仍继续推进。

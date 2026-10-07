# Maintainability Gate Report

- Requested change: 发现详情展示责任人推荐，并允许有管理权限的用户明确选择候选填入风险处理草稿。
- Files/modules inspected: finding-disposition.tsx、run-detail.tsx、page-utils.tsx、owner_recommendation.go及HTTP契约。
- Trigger: 风险处理模块压缩在18行，包含读取、草稿、保存、历史显示；新增功能跨UI与既有HTTP契约。
- Current risk level: medium。
- Responsibility count: 2，表单状态/保存编排与风险记录渲染；新推荐请求/状态不加入旧函数。
- Size/complexity signals: 文件短但单行密度高；run-detail只保留既有组件委托，不加入推荐逻辑。
- Coupling signals: 推荐独立读取，不自动修改后端；onChoose只填现有owner草稿，最终保存沿用expected_revision/head_sha权限验证。
- Tests covering the area: Store/真实HTTP完整snapshot权限、撤权/配置改变/截断已定向race通过，风险处理已有版本/权限/保存测试；新UI需真实浏览器mock验证。
- Refactor required first: no broad refactor；独立推荐模块，旧风险处理文件仅窄委托。
- Allowed change type: adapter_extraction。
- Proposed slice: 独立finding-owners.tsx惰性请求/来源/状态/候选渲染；风险处理编辑器只组合组件与显式回填回调。
- Acceptance criteria: 无自动分配；viewer可看不可选择；loading/error不展示旧候选；来源/HEAD/版本/规则行、unknown/filter/truncated均显式；选择仅回填草稿，保存前无PUT。
- Validation commands: npm typecheck、临时独立Vite build、受控浏览器成功/选择/失败/重试/只读/窄屏，正式build在Go全量终态后执行，git diff --check。
- Risks and assumptions: 推荐当前读取快照仅供人工参考；保存时现有Store再次授权。CODEOWNERS映射不等同代码平台必审人；通知消费和完整GitHub仍待交付。

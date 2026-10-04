# 登录页：右侧表单与设计调研

2026-10-05，用户明确要求表单右置并调研最佳实践。P10/F0：沿用product-workflow-gate/feature-lifecycle；工作区干净。现有认证/API明确，允许前端布局与交互优化，无权限变更。上轮居中布局被用户明确否决，本轮覆盖旧方案。

F1/F2：桌面为左侧品牌/产品流程介绍、右侧400px表单，共用中性浅色画布，不使用深浅硬切割。移除网格/轨道/巨大分支装饰与装饰句点；强调字重、排版比例、表单对比和有限蓝色。手机单列，缩短介绍并保留完整登录流程。

调研来源：GOV.UK Password input https://design-system.service.gov.uk/components/password-input/ 、Ask users for passwords https://design-system.service.gov.uk/patterns/passwords/ ；IBM Carbon Forms https://www.carbondesignsystem.com/building-blocks/core/components/form/guidelines 。采用持久字段标签、短说明、单一主动作、密码管理器autocomplete、允许粘贴、密码显示切换与清晰错误。右侧布局是用户指定的视觉选择，不称为普遍最佳实践。

F3：重写Login区域与login.css，删去过时响应式覆盖，按真实产品能力绘制左侧静态流程（无虚构审计数据）；显示密码按钮不提交表单，busy保持状态。严格构建、桌面/窄屏/密码切换与无效登录反馈验证；更新1234后台服务、保存截图，保留配置账号并最终合入main。

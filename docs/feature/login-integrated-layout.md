# 登录页整体比例与融合

2026-10-05。用户确认优化建议：保留右置、去除外层卡片背景/边框/阴影、缩短左右距离、对齐标题、缩小左侧标题和简化流程图。P10/F0–F3：沿用已读product-workflow-gate/feature-lifecycle，认证和设计上游完整，允许窄范围视觉修改；无新API/权限决策。工作区干净，分支codex/login-integrated-layout。实现仅Login JSX/CSS；构建、桌面与手机验证、更新1234且保留配置，最终按原授权合入main。新增规则通过编辑原规则实现，避免叠加覆盖。

F4/F5：表单外框完全移除，容器背景透明、无border/shadow，白色仅用于输入控件；左右区域收紧为1080px容器、360px表单，左标题43px以内，流程用三项内联说明替代卡片。既有认证、显示密码和错误逻辑保持。

严格TypeScript/Vite构建PASS（1593模块），Go嵌入构建及diff检查PASS；1234已更新且配置字节未改。浏览器1440×1000测得两侧标题top均322.148px，form背景透明、shadow none，页面无横向溢出；390×844页面scrollWidth=390，字段/密码切换/按钮完整。截图workspace-design/login-integrated.png。视觉修改不重复上轮已通过的认证及后端测试。

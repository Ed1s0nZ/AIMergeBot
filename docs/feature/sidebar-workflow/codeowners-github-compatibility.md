# GitHub CODEOWNERS 兼容适配

对应已确认REQ-019/AC-017，F2–F3/P8；Confirmed requirements、owner-routing-design、固定HEAD推荐链已存在，允许实施。当前hmarr/codeowners v1.2.1的parse.go只允许ASCII路径，邮箱正则TLD为2–6字母；match.go将无通配符的根规则/进入literal快速路径，剥离斜杠后空字符串越界。中文代码库合法路径或较长邮箱后缀会让整个推荐不可用，根规则可能使请求panic。

## Contract / Maintainability Gate

- 风险medium：codeowners_rules.go约145行，provider公共契约/解析/匹配混合；不引入编码路径映射，也不修改第三方缓存或运行仓库代码。
- 类型adapter_extraction：新增codeowners_github.go负责GitHub规则词法与bare owner；codeowners_github_pattern.go复用hmarr v1.2.1成熟RE2 glob编译算法，保留独立MIT许可归属。现有provider入口仅委托，GitLab实现不变。
- 公开接口保持ParseCodeOwnerRules/Match；推荐只读和显式alias/完整snapshot授权不变，不能声称等同GitHub原生审批或已验证外部账号资格。
- 保留预算256KiB/4096行/4096字节行/2048规则/1024模式/100 owner每条/8192总owner、UTF8/NUL/路径边界，错误整文件不可用，绝不返回部分规则当完整结果。该审计策略比GitHub跳过无效行严格，需继续明确区分。
- 新词法支持UTF8字面路径与普通标点，反斜杠转义空格，物理行号与原模式；owner支持@username、@org/team及net/mail校验的裸邮箱，保留原alias大小写和字节，不按同名账号授权。
- 根模式/显式无匹配，不访问空字符串。全局最后匹配、空owner覆盖、目录/root/globstar/单字符与大小写语义保留。
- 不支持的GitHub negation、未转义字符范围、起始转义#、模式内#及控制字符/残缺转义明确错误，不能截断成另一个有效模式；inline # comment只在模式分隔后处理。未确认的语法继续不可用，不猜测审批资格。
- 验证：先复现第三方Unicode/TLD限制及根规则panic；官方示例/中文/空格/字面标点/邮箱长TLD和RFC裸邮箱符号、错误后无partial、所有预算边界；ASCII共同支持范围与hmarr v1.2.1差分比对；固定HEAD来源/Store真实HTTP回归、race/vet/完整Go、精确CI。
- 生命周期：本设计先commit/push；代码/证据写implementation-plan并提交，当前c8ef2ea CI跟至终态再推代码。
- 范围保留：GitHub真实provider、自动推荐通知消费、其他所有完整REQ与UI错误综合验收仍未完成。

依据：[GitHub官方CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)、[Git官方gitignore](https://git-scm.com/docs/gitignore)、[hmarr匹配源码](https://github.com/hmarr/codeowners/blob/v1.2.1/match.go)、[hmarr解析源码](https://github.com/hmarr/codeowners/blob/v1.2.1/parse.go)。扩展路径字符由官方gitignore通用字面路径语义推导，不将第三方ASCII限制当平台要求。邮箱只提供显式alias候选，GitHub是否识别真实账号须由其平台决定。

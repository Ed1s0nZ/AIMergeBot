# 责任人推荐设计补充

对应 REQ-019 / AC-017。项目默认责任人与 CODEOWNERS 两种来源均保留在最终范围。当前发现及风险处理已有 owner 字段，但没有推荐和外部身份映射，不将其视为需求完成。

## 数据与权限

platform_owner_routing 保存 project_id、revision、policy_json、updated_at。策略含 default_owner（0 表示未配置）和 aliases（仓库用户名、团队或邮箱到显式平台用户 ID 列表）。管理员配置不增加成员权限；保存时校验被推荐账号启用且可访问目标项目。推荐时按该运行完整 target/source/context 权限再过滤。外部用户名与平台用户名相同不构成身份映射。

platform_owner_routing_history 按 project_id/revision 追加配置、actor 和时间。修改采用 expected_revision 防覆盖，与历史、platform_events 同事务；空配置显式保存以形成撤销历史。项目 viewer 可读取推荐所需配置，admin 才能修改。

## CODEOWNERS 来源与语义

文件只从运行固定 HEAD 获取，限制原始字节、行数、模式长度、规则/owner数量，禁止把未读或截断文件当完整规则。先读取运行的真实 provider，分别应用其文件查找顺序和规则。GitHub 最后匹配优先；GitLab 每个 section 独立最后匹配，并处理 section 默认 owner、重复 section、可选 section及排除语义。不能将 GitLab section压平为单个 GitHub ruleset。

优先采用已有成熟匹配器，检查其支持范围及资源限制。无法确认的语法明确标记无法推荐，保留项目默认来源及不足说明，不假称精确匹配。空owner的有效覆盖规则与文件读取失败区分处理。推荐记录匹配文件、行号、模式、section、固定HEAD、策略revision，以及alias是否成功映射；无法映射保持未分配，不新增ACL。

## 交付与验证

依次完成配置持久化/历史与权限，provider解析和固定版本读取，完整snapshot推荐，认证HTTP，管理员配置与发现推荐UI，通知路由消费和回归。测试包含配置竞争、撤权/停用、数据库重开、last match、GitLab多section/default/exclusion、文件缺失/截断、跨仓库权限、映射不授予权限和失败UI。

依据：[GitHub CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)、[GitLab CODEOWNERS syntax](https://docs.gitlab.com/user/project/codeowners/reference/)、[hmarr/codeowners](https://github.com/hmarr/codeowners)（GitHub匹配器候选，未引入依赖，不能据此声称GitLab支持）。

## 配置接口（已接入，推荐链仍待交付）

登录后 GET `/api/v1/projects/:id/owner-routing` 返回 `revision`、`default_owner` 和 `aliases`；未配置时版本0、默认责任人0、映射为空。当前项目 viewer 以上可读，其他项目配置不披露。

管理员 PUT 同一路径，提交 `expected_revision`、`default_owner` 和 `aliases`。默认责任人0清空默认值，空映射清空alias配置。每个映射必须明确列出平台用户ID，且这些用户当前启用并已具备目标项目读取权限。客户端传入的 `actor` 不参与授权或历史记录。配置自身不会赋予权限，也不会立即改变现有发现的负责人。

请求最大64KiB；格式或缺少版本400、无登录401、无修改权限或跨站请求403、不可访问项目/账号404、版本冲突409。发生冲突须重新读取并核对草稿，再提交新版本；不能自动覆盖。历史与审计事件写入失败时整笔配置更新回滚。本接口尚未提供推荐结果、CODEOWNERS解析或通知派发。

## 匹配模块实施记录

新增显式 provider 的有界规则解析模块；GitHub 使用 hmarr/codeowners v1.2.1；GitLab 使用独立 section/default/exclusion 解析和 doublestar v4.10.2 路径匹配。GitLab 匹配前按官方 File/PatternIndex 归一化相对路径、目录、转义空格、尾部 globstar 与字面花括号；不直接套用 GitHub 的根目录匹配。保留原模式、规则行、section、optional/approvals、owner声明及排除证据，不把声明转成平台审批资格。

预算为256KiB文件、4096行、4096字节单行、2048条有效规则、1024字节模式、100身份/规则、8192身份总数；输入必须UTF-8且无NUL。解析失败不返回部分规则作为完整结果。当前 hmarr 的字符/邮箱限制可能拒绝官方允许的路径或邮箱，这须后续兼容性补齐，并由调用方显示无法推荐；不能将严格解析失败当文件缺失或无owner。尚未接入固定HEAD文件查找、Store完整快照过滤、推荐HTTP/详情或通知。

GitLab依据同时核对官方源码 [File](https://gitlab.com/gitlab-org/gitlab/-/blob/master/ee/lib/gitlab/code_owners/file.rb)、[PatternIndex](https://gitlab.com/gitlab-org/gitlab/-/blob/master/ee/lib/gitlab/code_owners/pattern_index.rb)、[SectionParser](https://gitlab.com/gitlab-org/gitlab/-/blob/master/ee/lib/gitlab/code_owners/section_parser.rb)、[ReferenceExtractor](https://gitlab.com/gitlab-org/gitlab/-/blob/master/ee/lib/gitlab/code_owners/reference_extractor.rb)。当前角色/名字/邮箱提取只提供显式alias候选，不推断真实代码平台成员资格。HEAD上的责任人推荐与GitHub原生PR审批使用base分支CODEOWNERS是不同用途，后续UI须明确推荐的仓库与SHA，不声称等同原生必审人。

## 固定 HEAD 来源读取基础

LoadCodeOwnerDocument 仅接受实现 CodeOwnerDirectoryReader 的仓库；provider由实现声明，不从任务URL猜测。GitLab目录顺序为根、docs、.gitlab；GitHub为.github、根、docs。先完整列出根目录，确认候选目录存在，再完整列出该目录，选择第一个已知存在文件。高优先级文件读取/解析失败不会改用低优先级文件；空文件与全部已证明缺失明确区分。

GitLab每个目录最多20页×100项，强制固定source_project_id/HeadSHA且不递归，目录API报错、异常页码、无法证明分页结束、非法条目均不作为缺失。Local Git只用ls-tree/cat-file读固定提交对象，无checkout/执行；要求Metadata声明实际provider，未知provider返回不可用。每个本地目录最多2000项，文件仅允许100644/100755 blob，symlink/gitlink/tree不可作为规则读取。DynamicRepository在单次Load开始冻结客户端配置，目录与文件不因设置中途更新切换origin。

当前HEAD推荐用途须标明source仓库与SHA。GitHub和GitLab原生审批都使用目标/base分支CODEOWNERS，不能把此HEAD推荐声称为平台必审人或合并授权。真实GitHub客户端尚未实现，现有GitHub加载仅有受控目录fixture与共享接口，不能声明GitHub完整审计链完成。来源读取尚未接到发现API/通知；后续读取前后都要验证完整snapshot权限。

位置顺序依据：[GitLab CODEOWNERS file](https://docs.gitlab.com/user/project/codeowners/#codeowners-file)、[GitHub CODEOWNERS location](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners#codeowners-file-location)。

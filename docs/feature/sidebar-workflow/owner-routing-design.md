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

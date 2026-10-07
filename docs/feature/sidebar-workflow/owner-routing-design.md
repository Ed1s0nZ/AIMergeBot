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

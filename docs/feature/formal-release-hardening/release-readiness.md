# 正式发布加固交付核对

2026-10-05：源码bb19914已合入main；带检视文档的干净构建38a42fe19c48b937dded40c38ce071015d2d8d14已部署1234。此记录是本轮加固交付，不宣称所有团队规模/语言/模型场景已经达到生产准确率目标。

|核查项|发现与改进|当前证据/限制|
|---|---|---|
|Agent上下文|过去长工具历史未压缩；官方Eino摘要接入四阶段|HTTP协议成功/失败、空摘要、用量未知、预算停止、固定任务/完整工具配对/源码资格回归；字节估算阈值不是精确窗口|
|终态与部分结果|异常结果遗漏调查账本；压缩停止被笼统当失败|保留调查/发现/覆盖；持久Worker停止incomplete且无重试；实际CLI完成0/未完成2/失败1|
|大差异纯Git|CLI未复用工作台分组|真实Git多组/超总预算/排除路径与汇总压缩成功失败；不限制语言，不执行代码|
|成本|新增摘要请求可能重复统计/复核价不一致|显式独立callback与共享预算，复核摘要用实际复核模型价；每请求trace计数与HTTP一致；未知usage保守停止|
|权限与配置|核查既有控制，无本轮新增权限|完整回归包括未登录401、成员配置403、跨源403、退出撤销、项目/跨仓库授权、配置revision冲突/恢复；部署API凭据不回显且保存保留，源目录别名同步|
|任务可靠性|核查租约、取消、恢复、评论、预算链|完整回归覆盖过期旧owner不可写、重复接管、取消优先、重试有界、评论generation fence/去重/冲突与未知账单停止；外部请求不承诺exactly-once|
|HTTP|POST正文读取没有总期限|真实TCP慢body408并恢复正常请求；生产header10/read30/write60/idle60秒|
|依赖与存储|Go可达已知漏洞和旧SQLite C运行时|升级并扫描symbol/package0；openpgp模块已知无修复但未导入，不宣称所有模块0；SQLite3.53.4/WAL/业务重开/integrity，以及实际生产备份恢复|
|自动交付检查|无覆盖整个栈的只读CI|GitHub CI37294855358源码bb19914全成功：Go/full race/vet、Python、govulncheck、npm audit/typecheck/build、嵌入构建；action固定SHA/无本机凭据/不部署|
|发布和部署|main/运行版本需一致且可回退|main已推送38a42fe；二进制vcs.modified=false；旧配置/DB/二进制私有备份校验，恢复业务内容hash与数量一致；launchd单实例1234 readyz|

## 实际部署验证

运行目录为用户Library/Application Support/AIMergeBot/runtime-main-38a42fe，目录0700、二进制0700、配置/DB0600。source config.yaml仍链接到实际运行配置；以后备份使用物理运行文件。自动轮询、Webhook和评论保持原关闭状态，未扩大外部写入。升级停止旧服务、校验备份、恢复到新目录后换二进制；失败路径具备旧plist恢复。实际新服务就绪，生产业务表恢复hash一致。

API检查：readyz、管理员登录、设置保存与config_revision递增、刷新、模型/GitLab凭据保留且公开响应不泄漏、源配置别名一致、无需重启、项目列表、HTML及静态资源200。首次检查脚本误用revision字段，取得公开契约config_revision后修正；没有修改业务schema。浏览器登录页可见，窄视口正常渲染，无本轮新增UI布局修改。证明文件含本机路径/数据hash，仅私有保存，不提交。

## 仍需后续实际环境证据

沿用用户2026-10-05“无法验证先不验证，直接合并”的明确决定：历史PR人工标签及真实模型误报漏报、真实大PR/跨仓库覆盖、公网GitLabWebhook主动投递、Linux systemd托管仍未验证。此前真实DeepSeek/GitLab审计得到incomplete；模拟模型的真实GitLab评论专项不能用作准确率证据。真实准确率不能被HTTP脚本通过替代。

SSO、多实例/多租户、数据删除留存周期、集中指标与报警可按团队规模形成新需求；当前项目是单实例团队审计工作台，不能自动对外承诺这些能力。本轮不推断保留政策并删除历史数据，也不引入语言专属分析器。

## 最终交付证据

独立只读检视见[independent-review-38a42fe.md](independent-review-38a42fe.md)：无blocking finding，COMMENT，明确不等于真实准确率保证。原自检报告已标STALE，冻结时的INCOMPLETE作为历史保留。精确main head38a42fe的[CI37295639220](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37295639220)全部success，5分3秒，Go/full race/vet、Python、漏洞扫描、React及嵌入构建均成功。此前源码bb19914 CI亦成功，不以其冒充精确head检查。

CI有非阻塞维护提示：当前固定action版本声明Node20运行时，由GitHub runner强制Node24执行，当前两次验证正常；ubuntu-latest将迁移系统版本。后续升级action主版本并固定runner系统版本时，需重新验证整套CI，不把提示隐藏成零维护负担。

最终收尾只修改README/CHANGELOG与本轮文档，不改变应用源码、依赖、测试、工作流或构建资源；运行二进制仍为干净38a42fe，与收尾后的应用源码一致。私有配置、数据库、运行日志、备份和原始部署证明不入Git。

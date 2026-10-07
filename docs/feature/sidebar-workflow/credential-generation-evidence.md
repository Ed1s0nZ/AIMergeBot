# 旧GitLab凭据代次的隔离实验

状态：F2研究证据，未实施、未通过方案复审。不替代repository-recovery-contract.md；RRC-006仍open。输入为其复审发现：仅将counter放进YAML无法识别配置回退，客户端也不应成为代次所有者。

## 候选机制与选择依据

候选为数据库持久generation+私有keyed fingerprint。SQLite单例行保存generation、随机32byte HMAC key、32byte fingerprint；fingerprint来自有效仓库origin/token/网络策略的规范元组。token不在这张表中；key/fingerprint不进HTTP、policy、模型、错误或events；policy只携带generation。原有凭据仍只由配置服务持有，不复制原token建立一套新凭据库。

激活新有效配置时在短SQLite写事务内计算与当前指纹比较：相同保持generation；不同递增并保存指纹。A→B→A为1→2→3，不因A相同而复用1。重启加载当前配置再比对；恢复旧配置也是新的激活，不接受YAML声称的历史counter。非仓库字段不参与指纹；请求者不能设置generation。损坏key或达到最大generation时拒绝，不重新生成key或回到1。

这个候选替代“仅增加Settings.LegacyRepositoryRevision”的方向；该替代仍需作者更新正式字段/状态机及复审，当前没有生产结构。fingerprint使用HMAC而不是在日志/UI输出裸token hash；本实验不声明额外加密了现有配置或DB。

## 实际运行

2026-10-08，代码快照4435236bf5ab86a4e233e8d316828269d67997f2（只含审查文档变更）。Python标准库sqlite3/hmac，脚本 `/tmp/aimangebot-credential-generation-research/probe.py`，命令 `python3 /tmp/aimangebot-credential-generation-research/probe.py`，exit0。使用TemporaryDirectory创建临时DB和合成配置，不读取真实配置/凭据，不调用网络；结束清理DB/配置。脚本保留在/tmp，不提交生产源文件。

| 场景 | 实际断言 |
| --- | --- |
| 首次激活、未变化重启 | generation1，固定描述授权有效 |
| 模型/项目名称变化 | generation保持1 |
| A→B→A | 1/2/3；A的旧generation1不再授权 |
| 文件已写、DB事务提交前中断 | DB保留旧state；下次启动按新文件递增，旧描述不能用于新配置 |
| DB提交后、内存发布前重启 | 相同文件不重复递增 |
| 恢复更旧配置 | 得到新generation，旧A描述继续被拒绝 |
| 两个SQLite连接同时激活相同配置 | BEGIN IMMEDIATE串行；两者看到6，只递增一次 |
| 达到MAX_INT64 | 换凭据失败，旧active fingerprint不被修改 |
| 损坏持久key | 拒绝激活，授权失败；不重建key伪造新历史 |

程序输出仅场景名称和PASS，不输出key、fingerprint或合成token。线程有界join，断言全部结束；无后台进程遗留。

## 未证明的生产边界

本实验只证明上述单例state协议，不证明SettingsService与Store已集成。特别是“文件已写、DB未提交”期间，实验DB仍认为旧A有效；生产必须有activation_pending gate阻止继续读取，不能直接把该原型作为完整授权实现。

需补状态机：凭据改动前关闭legacy读准入；文件失败且DB未变时恢复旧active；文件成功、DB失败保持pending；DB成功后才发布可用内存描述。重启必须先完成文件/DB核对再启动worker。每个操作派发前和结果消费前核对当前scope generation、有效配置指纹与pending，取消/lease仍独立校验。仅对DB读state不足以发现当前有效配置已改变。

还需验证Go并发锁顺序、Settings.save失败/恢复、配置outbox不递增、管理HTTP忽略客户端counter、直接编辑配置的明确加载边界、canonical origin与网络策略语义、HMAC规范编码、DB键损坏/行缺失、代次JSON精确表示、文件与DB配对备份、原生/legacy混合逐凭据隔离、缓存授权及全部runtime调用方。不接受异步后台补齐generation后再称审计可用。

RRC-005可独立补齐route project_id/actor/protocol version进入digest及完整回执归属核验；该修正不需要引入新远端操作。两项都仍等待设计修订及正式复审，不进入F3/F4。

## 发布状态

本地审查4435236的Git推送连续收到GitHub Internal Server Error；初次read-only refs证实远端仍3fcc507，read-only commit API返回404。后续使用GitHub Git Database API上传原文档，生成tree ff06874f778a75062f777a99501796629b23703b、commit 4435236bf5ab86a4e233e8d316828269d67997f2，分别与本地Git对象逐SHA相同；再次核对远端仍为其parent后以force=false更新，readback验证4435236。未创建替代分支、强推或替换原提交。本证据及RRC-005/006作者修订现在可以独立提交，状态仍是设计证据，不表示生产功能通过。无main合并/部署。

使用官方[创建tree](https://docs.github.com/en/rest/git/trees#create-a-tree)、[创建commit](https://docs.github.com/en/rest/git/commits#create-a-commit)、[非强制更新ref](https://docs.github.com/en/rest/git/refs#update-a-reference)机制；本次保留原author/committer时间、message、tree与parent，只有SHA全部相同才移动ref。不能把新生成不同SHA的提交当原push成功。

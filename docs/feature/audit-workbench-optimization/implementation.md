# 实施与验证记录

## 切片1：上下文仓库批量读取

2026-10-06，已实现Agent工具read_repository_files(repository_id, files)。同一授权固定仓库一次读取1–8个范围，整体输出最多16000字节，保留编号源码、more和remaining。失败与超限整批不返回部分源码；读取前后权限核查；取消即使缓存命中也不返回源码。一次批量请求消费一次工具预算和观察，复用已有检查点与来源资格机制。主PR变更锚点要求保持。

新增context_repository_batch.go及测试；注册与来源资格增加工具名；策略版本v15，防止旧运行自动采用新工具契约。无需DB迁移。未修改用户README及docs/images。

验证：go test ./internal/platform -run Context -count=1通过（15.089s）；go test ./...通过（platform 42.750s）；新增覆盖状态/输出预算/取消测试后go test ./internal/platform -run TestContextBatch -count=1通过（10.848s）；git diff --check通过。没有真实模型质量对照，不能声称误报或召回改善。未修改UI，本切片无需界面验证。

代码复核：权限复用invokeContext前后检查；批量不循环调用外层工具、不产生未持久化子ID；跨仓库来源通过RepositoryID映射固定SHA；任何子项失败清空聚合输出；部分范围保留覆盖标记且继续读取后续请求项。该来源资格只证明取证，不证明调用关系或运行可利用性。

剩余：PR入口与风险知识、因果链、工作台与复核冲突、轻量更新/缓存、SARIF及多语言跨仓库实际质量评测均未完成。整体目标继续，首切片完成不代表REQ-002整体验收完成。

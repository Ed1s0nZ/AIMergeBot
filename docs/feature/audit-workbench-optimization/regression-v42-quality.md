# v42 跨项目与兼容缺口定向回归

## 冻结与边界

实际CLI code_revision=8129579a5146481cf9fa8de6337325971416376b，生产源码仍为b19ff4677f475d15bd5ab7d04e0da7c27bdd5807；两提交之间仅文档差异、工作树干净。策略eino-audit-contract-v42；原deepseek-chat、temperature0.1、15steps/80tools/240秒；仅803/805各一次，不改原真值/预算、不执行fixture、不盲重跑。corpus SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b。已知回归，不是新独立验收。

## 真实观察

| 案例 | 结果 | 调用与成本 | 原始来源/判断/关系 | 未完成原因 |
|---|---|---|---|---|
|803 TS→PHP条件风险|incomplete、1finding，gateway.ts HEAD6锚点正确|33.440秒，primary9+verification4轮，142734输入+6951输出=149685tokens|BASE/HEAD均读，固定PHP query/connection/deployment三文件批量读；登记cited请求→处理器/SQL关系；guard为Bearer staff，PR把常量改成query输入；独立静态复核4项supported|调查与已提交finding均缺结构化BASE/HEAD来源链接，各保留2条缺口；驱动/权限/网络与具体payload成功未运行|
|805 Go→Python兼容负例|incomplete、0finding|27.757秒，primary9轮，120681输入+6479输出=127160tokens|主仓库双侧及固定server.py/deployment.conf已读；摘要正确：省略limit仍default25并受0..25限制|三次update均违反计划身份不可改，保存调查仍investigating、contract/outcome pending、无保存关系；拟提交兼容Claim却选择evidence_refutes_claim，极性仍错|

共276845tokens，usage_complete均true，无402/API错误；价格未配置，费用未知。单次已知样本不外推语言/生产准确率、稳定性或节省费用。

803的关系实际引用observation-8主仓库search及observation-11固定PHP批量源码，原始工具可打开；endpoint描述“gateway.ts:7 fetch”并非精确fetch行（源码fetch在8，7为form body），以原始源码为准，不能当自动精确坐标证明。静态部署映射建立条件连接，不证明线上请求执行。SQL输入拼接机制源可观察，PDO驱动/权限与特定payload成功不可证明；复核full只是模型静态声明。此例相较v37关系空有具体进展，仍不足以关闭AC005所有必要证据缺项。

805初始record(5)引用缺少顶层来源，被拒后record(6)同Claim补好并通过；resolve(7)退休5→6保留历史。随后read_repository_files(9)取得真实下游default及deployment。update(10/11/12)把p3 question从“payload or response in this repository”改成“payload or downstream limit handling”，p5删除括号中的unbounded result set，均与保存身份不符；旧门禁正确拒绝、保存ledger未改。后两次并未修复这两个字段。拟提交关系和checked任务不能算已保存/验收通过。最终覆盖保留缺计划、缺来源、缺关系和工具失败。

805拟提交Claim明确“same default25 / identical outcome / no harmful behavior”，counterevidence真正支持它，却三次选择evidence_refutes_claim；v42更明确的枚举仍不能保证语义。不能把摘要正确或0finding当作完整兼容调查通过；也不能把被拒的update伪称实际rejected持久化。

## 对下一步的影响

AER-001继续open。802来源遗漏已有实证改善；803已形成跨项目静态关系但漏双侧结构化来源；805暴露可判别的记录恢复问题和命题极性问题。下一步先提供计划身份冲突的精确恢复反馈，让调用方复制原id/kind/question、只更新status/reason/source IDs；保持不可删改门禁与原预算。反馈属于原记录、非新源码事实，不能自动选极性、修改ledger或退休错误。极性需继续独立检查实际Claim，不从finding数量推断。

归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v42-quality-20261006，132文件SHA256逐项核验一致；所有旧回执/真值未修改。

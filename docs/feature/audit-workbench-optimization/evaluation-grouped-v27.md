# v27 生产两组实际模型诊断

代码24d310c，grouped=true，25个修改文件按生产24文件边界分为两组（24+1）。原始证据 `/Users/worker/.codex/evaluation-artifacts/aimangebot/grouped-v27-20261006` 逐文件SHA256验证；人工已见Lua机制，不计未见识别率。

两组均completed，最终status=incomplete、1个原风险、256290token、46391ms。规范finding保留group-1-hypothesis-1及group-1来源ID，BASE a_service.lua:6，独立supported/full。第二组重新读取同文件BASE/HEAD/diff，使用新的group-2观察ID、产生新调查，未复用旧观察当新证据；没有在无害文档创建有效新发现。核心所有权移除条件风险保留；未验证运行框架。

问题：第二组调查名自带group-2前缀，合并后为group-2-group-2-hypothesis-1；该记录虽可溯源但重复调查。第二组只负责docs/context-23.txt，却反复尝试提交a_service.lua:6/7，全部被本组锚点限制正确拒绝，主风险仍由第一组规范记录保留。该失败并非“validator环境异常”，而是跨组范围错误；模型coverage描述归因过强。需显式说明当前组scope及旧finding导航不可重复提交。get_change_metadata还有一次失败，不能移除工具错误后称全部通过。

PRContext双方source IDs及逐边关系缺失，v27服务器gap正确保留；synthesis又复述已有coverage，最终出现完全相同重复条目。汇总摘要将25文件误写24（后又列出所有docs范围），不能当可靠数量来源；结构化AuditGroups为权威数量。独立full仍不等于完整风险链或运行证明。

这轮证明生产多组入口运行、固定来源命名隔离、规范发现保留；未证明高效无重复交接、完整链路或稳定模型质量。不提高预算，不覆盖失败记录。下一窄修复须清楚说明当前组提交边界，并稳定去重完全相同coverage但不消除不同缺口。

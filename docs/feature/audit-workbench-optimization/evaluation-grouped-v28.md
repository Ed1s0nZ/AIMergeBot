# v28 生产两组回归事实

代码7c455b3，grouped=true；同25文件语料、生产24+1组、同模型/工具/token预算与240秒。原始证据 `/Users/worker/.codex/evaluation-artifacts/aimangebot/grouped-v28-20261006`，逐文件SHA256验证，v27失败保留。

两组均completed，status=incomplete，1个原风险保留在BASE a_service.lua:6，group-1-hypothesis-1及group-1源码观察ID保留，独立supported/full。group2只进行了两次read_file，没有record/update/submit_finding调用；最终只一个调查，没有此前group-2重复调查，也没有后组越组提交。模型coverage明确当前组docs/context-23.txt范围、之前风险由group1保留。9条最终coverage，全部字符串不同；对应SDK证明原gap和新增gap不因去重丢失，rawtrace未删。

本轮154401token、23733ms；v27为256290token、46391ms。单次输出变化同时涉及导航提示和随机模型决策，不能归因全部差额或外推普遍效率/稳定性提升。价格缺失不报费用。

仍有首组2个工具错误：update_investigation引用非源码观察被拒，submit_finding错误侧/变更行后恢复。PRContext before/after源ID和relationships缺失，服务器三个缺口均保留。汇总仍把25文件概述成two-file-scope，文字计数不可信；结构化group.files为权威24+1。独立full不是完整链或运行证明，部署保护未知，全部仍incomplete。此套已见诊断机制，不是新隔离验收；下一步最终UI状态与新验收/审查交付仍需完成。

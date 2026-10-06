# v34 新样本首次隔离验收结果

执行HEAD85e5a5b，生产代码89fe12c；policyv34。freeze与CLI metadata的code_revision/corpus_digest/15轮/80工具逐项一致，六例首次进入被测deepseek-chat，真值在运行前由源码/路由/契约独立制定。不是外部专家双盲，不是生产benchmark，机制与旧调试集重叠。样本未编译或执行；未修改输入/真值/预算，无挑选重跑。

| case | findings | 状态 | elapsed ms | tokens | model calls |
|---|---:|---|---:|---:|---:|
|701|1|incomplete|34213|154691|15|
|702|0|incomplete|2434|15820|2|
|703|1|incomplete|22706|113683|12|
|704|0|incomplete|22862|116914|10|
|705|0|incomplete|4916|17974|2|
|706|0|incomplete|4617|15864|2|

总434946tokens，用量都有返回；价格未配置，不估金额。CLI exit0是完成归档，6/6仍incomplete，不能报告完整通过率或将四个零告警计4TN。

逐项源码/真值核对：701 BASE9删除admin角色防护，reader凭据在固定身份映射中明确，条件读取fixture admin配置的风险机制/位置正确，四项独立静态支持；主plan四项记录，但BASE/HEAD引用与关系仍缺。701先提交错误HEAD锚点、随后修正BASE；坏source关联错误保留，没有伪造来源。702正确识别新增角色保护、无finding，但零ledger/plan，模型仍把只读方法说明混入coverage，不认证完整验收。

703 HEAD7由认证tenant改为query，源批读包含Ruby archive.rb及deployment.conf（observation-7），双方固定契约支持条件跨租户返回；四项独立支持。主调查记录一条cited静态绑定关系并保留运行/host resolution未知，但before/after_observation_ids未填，不是完整风险链。不能称真实网络可达或复现。

704实际读取固定Ruby身份绑定，正确说明body tenant不影响export选择，零finding；五次record_hypothesis失败均为plan source ID未在顶层调查列表关联，最终零ledger/plan，不能算完整负例通过或高效收尾。705没有读取可用固定Python下游，错误称实现不可用，并尝试提交无充分证据的候选；服务器拒绝该proposal，实际finding0。不得把此过滤结果称模型正确识别兼容性，也不能计TN。706准确区分日志改动和未变command.!!历史风险，无finding，但无plan，记录完成度不足。

及时反馈实际到达：701的record/update/submit均反复出现base_sources_missing/head_sources_missing/relationships_missing；703记录一条关系，但before/head缺项始终保留。证明接口反馈可见及门禁有效，不能证明模型采纳或完成率提升。最值得下一步处理的是：有限码映射到明确字段/当前可用源导航；每次决策明确提醒未读取的配置上下文，避免把“没读”写成“不可用”；计划source关联错误需可执行的字段级修正指导。不能自动生成事实、放宽来源校验、增加预算或为此套反复重跑求通过。

后续若依据结果优化，701–706立即转回归，不能再称该新策略的隔离验收；需要保留本次全部失败。

证据 /Users/worker/.codex/evaluation-artifacts/aimangebot/acceptance-v34-new-20261006：corpus/freeze及全套metadata/truth/receipts/checkpoints/Git对象，286文件SHA256复读校验，sha256-manifest.json。原输出/tmp/aimangebot-acceptance-v34-new-20261006。源码89fe12c对应远端CI37419258951已实际读取completed/success；这证明工程检查，不证明模型质量。

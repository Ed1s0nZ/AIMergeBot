# v39 主仓库导航定向回归

冻结源码 dda3eda4064ba545ffe0cf19b52e94aa4b66ac11，策略 eino-audit-contract-v39；使用已有801–806 corpus，SHA256 1851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b，仅选择case-802。此为已知样本定向回归，不是新的独立验收。原配置模型deepseek-chat，15步骤、80工具、240秒；配置只读，无预算增加，无重跑。

实际耗时3529ms，17459输入+451输出=17910 tokens，usage_complete=true，无HTTP402或其他API失败。原始结果incomplete、0 findings；无调查计划/关系账本。源码未执行，仅静态分析。币价未配置，费用未知，不解释为零费用。

list_files observation-1列出HEAD三个文件（非源）；随后read_file observation-2读取release.go HEAD、observation-3读取workflow.md HEAD、observation-4读取caller.go HEAD，均成功。实际未读BASE源码，只获得原PR diff。最终summary引用workflow批准约束和Handle→Release调用，正确识别新增状态校验。相较v37 case-802只有两次read且虚构没有其他调用方，这次确实读到未修改调用方与契约文件。此单样本现象不能证明普遍效果。

仍未记录任何hypothesis/source-linked plan，服务器保留plan recording gap，不能据0finding标为完成；没有结构化调用关系、独立运行验证。803跨项目关系与805命题极性旧问题没有在本次执行，不能宣称关闭。

外部只读归档：/Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v39-navigation-20261006，冻结、原corpus、回执和仓库共40文件SHA256复核一致（manifest自身另列）。独立工程边界review另记录，不把此模型结果当作边界验证。

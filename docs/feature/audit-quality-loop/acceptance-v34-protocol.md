# 新样本隔离验收 v34（运行前）

代码生产改动89fe12c，策略v34。六例此前未进入被测模型；分析者按明确源码/路由/契约制定真值，与被测deepseek-chat独立，不宣称外部专家双盲、未知漏洞类别或生产benchmark。机制与此前保护删除/修复等场景重叠；新文件/输入不等于新机制。冻结后不修改策略、输入、预算或真值；任何根据输出的修改使这套转为回归。

701 Python Flask删除角色保护，Bearer reviewer明确role=reader，BASE拒绝而HEAD返回仅admin可读的fixture配置；702相反修复。703 JS Express主入口由认证tenant改为查询tenant，Ruby下游忽略身份返回指定租户的private export；704相同主PR但固定Ruby按转发bearer重新选tenant，构成有效防护反证。705 Kotlin显式false变空JSON，Python缺省False的隐藏行过滤等价；HttpClient外部适配器未实证，条件等价不能认证运行或其他部署安全。706 Scala只改日志字符串，危险command.!!未变，排除无关历史风险。服务映射文件提供声明的条件绑定，不证明网络隔离/运行部署。

两正四负；源码取证、BASE/HEAD及跨仓库固定SHA必须实际读取。预期不作为输入事实传给模型，也不能根据危险函数/角色名字评分。负例没读必要保护/契约、JSON失败、budget终止均未完成，不计TN；正例位置匹配还须独立核对触发、因果、反证、有害结果、附加断言。关系与任务字段齐全不证明语义。记录source/plan/checks、逐边/unknown、full/partial/candidate区别、FP/FN条件与证据完整度、tokens/费用缺失及基础设施失败。

原配置15轮/80工具/240s，无样本代码执行、无依赖安装、无提高预算、无挑选重跑。每例一次；所有receipt/checkpoint/模型失败归档，若无完整分母不报precision/recall。此次使用现有独立评测CLI，非local-benchmark-evaluator/XBOW，不混用其专用技能。

语料在仓库外 /Users/worker/.codex/evaluation-artifacts/aimangebot/acceptance-v34-new-20261006/corpus.json，SHA256 990a51d8a91cde35a54eeae68b999fd98208b64e2da61b0831428b86890bd1a6。运行前原生Git输入验证：建立固定BASE/HEAD、BuildDiff检验实际changed line，六例所有anchor有效；三例授权context各一个固定源。自建输入验证器仅检查对象/锚点，不执行被审计源码，结束删除临时程序/仓库。固定执行HEAD、预算及期望hash于freeze.json后运行CLI，私有原始输出在独立新目录。

输入验证实际exit0：701/702/706 contexts0，703/704/705 contexts1，六例changed anchor均验证。初次仅临时验证器忘建context父目录而失败，修正验证器后通过；生产代码/语料未变化。临时程序及Git目录已删除。

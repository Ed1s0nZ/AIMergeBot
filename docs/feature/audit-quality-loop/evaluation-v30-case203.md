# v30 case203 既有语料回归

固定代码 d1bad0cdb65f2a33de76ef8b64cd94faf31f5f20，既有 corpus-acceptance-v1 case203，原配置 deepseek-chat、原预算、固定双仓库；此例已用于既有回归，不是新隔离/真实历史PR。原输出 /tmp/aimangebot-plan-v30-case203-20261006，64文件归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/plan-v30-case203-20261006，SHA256逐文件复读通过。

24,360ms；109,207 tokens（primary 87,135、verification 22,072），12调用，usage完整，未配置价格不估成本。结果 incomplete、1 finding。主模型读取 gateway BASE/HEAD 与 profiles.py/deployment.conf，record时已经填四项checked，并未按建议先记录pending再检索。update省略plan，服务端正确保留四项及源ID。独立复核重新读取来源、填四项supported并明确运行/部署假设。本次未再声称 bob 是 low-privilege；不能将单例差异当可靠准确率提升。

仍有四条coverage notes：静态不执行、跨仓库边只靠源码推断、小仓库无其他文件（前三条包含方法限制而非实际缺口），以及缺少结构化BASE/HEAD/风险链。原record PRContext有文字before/after但缺两侧ID与relationships；update省略PRContext将其丢失。需保留既有上下文并重新校验继承来源所有权/side/预算；不补造缺失源和关系、不把方法说明自动静默删除。

实际UI受控组件验证360x800，scrollWidth360；三种任务状态、历史缺失、来源按钮展开fixture均确认。截图 /Users/worker/.codex/evaluation-artifacts/aimangebot/plan-v30-ui-20261006/plan-proof.png；这不是生产E2E或真实源语义证明。临时文件/服务器/视口/标签均清理。

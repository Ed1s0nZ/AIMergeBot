# v26 上下文导航定向诊断

代码7807e3b。分别只运行case-205和case-203，两个独立新目录、原16轮/80工具预算；这是已见样例的诊断，不能报告整个语料成绩或隔离验收。原始证据分别 `/Users/worker/.codex/evaluation-artifacts/aimangebot/context-v26-case205-20261006` 和 `/Users/worker/.codex/evaluation-artifacts/aimangebot/context-v26-case203-20261006`，逐文件SHA256验证。

| 用例 | 状态 | 发现数 | token | ms | 主调查下游读取 |
|---|---|---:|---:|---:|---|
| 205 参数兼容 | incomplete | 0 | 64840 | 8820 | 有：catalog.py及固定配置 |
| 203 身份覆盖 | incomplete | 1 | 97561 | 19300 | 有：profiles.py及固定部署映射 |

205现在基于payload.get(limit,20)、整数1..20校验、items[:limit]判定新增常量20兼容，记录rejected调查与下游来源ID；不再声称配置源不可检查。203主调查读取下游，finding/调查关联主源码及下游观察ID，PRContext包含入口、保护与影响来源；不再错误称没有下游源码。独立supported/full为模型声明，核心条件风险人工吻合。

仍有缺口：203 PRContext未填写before/after observation_ids和逐边relationships，unresolved_edges空，但coverage仍提示跨服务映射非运行关系；不能称完整风险链。复核limitations将内部hostname与“唯一调用方”推成只能通过网关访问，源码没有网络边界证明，人工不接受该补充断言；full不保证模型无遗漏。两例均有静态方法/缺调用上下文限制，保留incomplete；零发现不计完整安全。

相较此前205未读下游16122token，本轮64840token；203从73976到97561token，不能称效率提升。预检仅增加一次低成本工具调用，但模型追加源码调查导致总用量上升。一次样本不能归因或证明通用稳定性；v25历史独立结果和既有失败不改。下一验收须新语料，真实多组与结构化逐边完整性仍未验证。

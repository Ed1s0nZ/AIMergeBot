# S3c1共用运行检查层实施审查

2026-10-15，codex/sidebar-workflow；reviewed head 6d9086fcceea4ada3b0e54ef3cc9a6de9c762e48，repository-credential-runtime-plan.md；输入Confirmed requirements、既有S3 Pass和源码调用链。Codex同一作者分离复核，非独立代理。

## Decision

Pass（仅S3c1可实施交接）；不批准S3c2/c3的消费者/启动/发布实施，不批准整个S3启用。范围不缩减完整目标，后续各项仍须实施前具体交接和验证。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| 目标/非目标 | Pass | 实际派发/消费检查；不把部分适配器当全平台保护 |
| 数据结构与字段 | Pass | 私有context key/authorize；沿用S3descriptor，无公共配置/API/DB字段 |
| 数据流 | Pass | descriptor→组合ctx→HTTP/Git→返回与buffer复查→动态客户端/模型消费 |
| 生命周期 | Pass | pending拒绝、父检查保留、请求前/返回后/body Read、Close、缓存命中与取消 |
| 图 | Pass | 上游S3跨层sequence继续适用，运行层输入/派发/消费表补足本切片 |
| 开发交接 | Pass | 共享transport、动态适配器、Git命令/cache职责及真实HTTP次数/清buffer反例明确 |

## Qualified Areas and Risks

五个模型builder共用upstreamHTTPClient，ctx中的检查须实际进入SDK请求；不能通过只检查首次prompt来声称复核/压缩均受保护。HTTP body延迟读取和cache命中都在范围，结果拒绝不能只置error仍返回内容。授权调用错误需规范化且不泄露SQL/secret。父ctx检查不可被捕获新descriptor替换。

目录/源码/快照SDK方法返回后仍复查，原standalone/未绑定路径不变。Go锁顺序无新增全程锁；检查DB只在网络/返回边界，S3c2入队事务必须用同Tx，不能重复Store.DB造成单连接死锁。

## Unapproved Dependencies

启动须先有worker单例所有权且事务内验证；旧nil scope外部发布缺generation证明，不能靠时间戳或补当前版本授权。这两项已作为S3c3/S4依赖保留，S3c1无生产绑定与外部发布改动，不声称已解决。未成熟部分保持不实施，完整UAR-001/002/004仍open。

## Validation and Re-review

必须实际httptest证明派发前撤权请求0、已发响应拒绝且buffer清除、动态客户端固定旧descriptor、组合ctx不降级；Git命令和cache命中检查；真实模型HTTP不能依赖源码搜索替代。再跑完整Go/race/vet/build/typecheck，精确新HEAD CI另验收。变更启动时机、公开错误、scope生产/发布授权或context组合语义时重审。此为文档静态审查，没有声称新实现测试或真实部署完成。

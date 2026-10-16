# S3c：运行检查与启用依赖细化

2026-10-15，codex/sidebar-workflow；基于8654211及已通过S3计划。F3补充，未启用生产激活器，完整REQ/AC及S4–S6不缩减。

## 新取得的调用链证据

runner.go、context_repository_runner、polling、followup、comments、check_dispatch和Settings动态适配器仍直接构造GitLab客户端。五个主审/复核/压缩model builder共用upstreamHTTPClient；Git网络子进程走gitCommand，path cache可直接返回。单独包装DynamicRepository不足以保护全部消费者。

Runner.Start先AcquireWorkerInstance再Recover/启动loop；这是激活器绑定位置的所有权前置。不可在main取得租约前激活，否则第二实例可以先旋转代次/取消任务，随后才因租约冲突退出。绑定的激活事务需核对同一有效owner，失败释放租约且不启动HTTP/worker。

旧v50/nil scope任务没有generation。仅取消pending/running不足以证明完成任务的旧check/comment投递匹配新凭据。不能用created_at时间戳猜归属，不能给旧记录补当前generation。新外部发布必须消费S4正式producer生成的固定scope并逐凭据匹配；旧无版本结果保留查看，任何待发布记录缺证明时明确阻止且保留unknown，不盲重发。完整启用因此依赖S4正式scope producer及S3d恢复入口，不在S3c部分接入时提前绑定生产。

## 可实施顺序

NTR-006修订见[普通GitLab切换合同](repository-legacy-transition-contract.md)：C3 readiness必须包含普通未绑定GitLab的新任务producer，而非只有显式绑定项目；切换后的所有平台新任务统一v51/scope v2，legacy HTTP/path-prefix由专用endpoint union保持，不丢协议/前缀。旧v50不回填。此为待评审作者方案，不表示当前已启用。

| Slice | 模块/行为 | 必须证明 |
| --- | --- | --- |
| S3c1 | 私有context fence、upstreamTransport派发与响应body消费检查；DynamicRepository/DynamicAuditor捕获同一个激活descriptor；Git命令/cache检查 | 无ctx fence旧行为保持；pending/撤权时HTTP请求0、已发响应丢弃、buffer清除；真实动态读适配器不切新凭据继续旧操作；主审/复核/压缩实际HTTP共用检查 |
| S3c2 | runner Submit/execute/context/poll/followup/CODEOWNERS全部传同一descriptor context；入队事务用同Tx校验descriptor，禁止在持Store tx时回调Store.DB | 元数据读取后变更凭据时零入队；fork/context ACL前零内容；缓存/下一Git命令/模型输入拒绝；race和同连接不死锁 |
| S3c3 | owner受控启动绑定、check/comment全部发布消费固定scope、公开有限错误传播 | 第二实例零generation/cancel变化；租约丢失拒绝；旧无版本报告不可猜授权；unknown不重发，完整S4 producer与S3d就绪后才启用 |

## S3c1实现合同与门禁

P7/P8，allowed adapter_extraction/narrow_fix。新context key/authorize函数为包私有，不是API/模型输入；nil fence保持旧路径，组合新fence必须保留父fence，不能覆盖旧ACL。每次检查先ctx.Err；有限ErrRepositoryUnavailable传播，不能输出原SQL/secret正文。

upstreamTransport在base.RoundTrip前和返回后检查；拒绝时关闭request/response body。成功响应body每次Read前后再查，失败返回0+固定错误并清除本次已经填入调用者buffer的字节；Close不发新请求。此为派发/消费观察，不承诺撤回已经发出的网络请求。实际model builders共用该transport，由审计ctx传播到SDK请求；不能只靠日志checkpoint充当授权。

DynamicRepository/DynamicAuditor调用私有Settings operation helper：未绑定保持旧cfg逻辑；已绑定捕获descriptor及其immutable config，组合到ctx。不得先拿cfgA再捕获generationB；各方法返回前再次校验，失败不返回内容/路径/快照。捕获新descriptor不能替换父ctx的旧fence。生产未绑定时不声明能力生效。

Git命令在派发与结果返回前查同ctx；path cache命中也查。standalone无fence保持只读本地/固定配置行为。错误/中断不返回Git stdout。子进程持有的旧token不会被刷新；取消由现有ctx负责。

验证用临时配置/SQLite和httptest服务器、受控RoundTripper/Reader，断言真实请求计数及buffer，而非仅error。覆盖metadata/源码、响应阻塞期间改凭据、模型请求前撤权/返回后撤权、context组合、缓存命中、下一Git进程0次、取消。新代码不调用真实外部模型/消息，不生成资产。

S3c1内部适配器完成仅证明共用执行层；S3c2/c3未接通不能宣称整个平台已受保护。完整Go/race/vet/typecheck及精确生产HEAD CI独立记录。旧CI终态前不推新生产HEAD，docs不触发CI。

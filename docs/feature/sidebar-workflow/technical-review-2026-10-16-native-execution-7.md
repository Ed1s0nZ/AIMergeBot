# Technical Plan Re-review：SDK有效请求前缀

- 日期：2026-10-16；Codex；架构交接审查。
- Snapshot：3671bc0dc6a6b65a106bcc6c0cceeca9ec59c105；Artifacts：legacy-transition合同的EndpointKey/EffectiveAPIBaseURL修订、N1a记录、实际wire探针；对照第6轮全部S4合同。
- Supersedes：第6轮对旧BaseURL键算法的判断；NTR-001–008的其他约束不撤销。
- Final decision：**Pass（S4内部实施交接）**。只允许按原N1–N5阶段实现/验证，绑定写/C3/N6交付、合并main及外部发送仍不因此开放。

## Mandatory Criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| Goals | Pass | 原PR、多语言、跨仓及旧GitLab兼容范围保留 |
| Structures / changed fields | Pass | scope保存SDK BaseURL；派生key使用SDK有效前缀；material v2增加EffectiveAPIBaseURL，三者用途明确 |
| Data flow | Pass | SDK只构造请求获得前缀，零网络；token/proxy不进入key；reference→同仓检查→target/baseline使用同一key |
| Lifecycle | Pass | SDK行为变化需复验并迁移传输证明；旧scope v1不重新解释，新profile启动/Save/Reload规则不变 |
| Diagram | Pass | 仍沿第6轮准入/session/发布图，本次只改变地址身份的派生来源 |
| Handoff | Pass | 函数来源、JSON格式、边界及真实wire反例齐全，可恢复N1a实施 |

## Evidence and Forward-risk Review

当前SDK NewRequest使用baseURL.Path拼接请求，不能假设BaseURL().String()中的RawPath原样上网。探针对8种路径实际发本机TLS GetProject请求：dash/slash编码别名同wire，大小写前缀与encoded-dot仍不同；空相对路径NewRequest生成的前缀与实际wire一致。之前dot等价的猜测已被实验否决，未通过手工path.Clean掩盖。

新key保留实际请求构造器的path语义，仅独立规范主机/默认端口；不会修改发送地址。实际同前缀别名可共用远端身份，而不同大小写/dot前缀不被错误合并。scope原SDK base仍严格匹配固定配置，不能因key相同放松身份revision、credential、ACL或完整reference相等检查。material同时绑定原base及有效前缀，不因SDK变更而继续旧网络证明。

NTR-001–008的principal、同根预算、seen/authority防重、profile、unknown发布规则仍有效。本轮不发现新的阻塞设计缺口。服务端可能额外归一化路径属于未知事实，不纳入自动等价规则；不推断真实GitLab版本全部兼容。

## Verification and Implementation Boundaries

探针实际go run -race / go vet exit0的证据见N1a记录，当前源码未变；本轮静态复审其与SDK v0.90.0实现及修订合同一致。此前生产codec试作已撤回，未继承其测试结果。新codec必须独立验证v1回归、v2严格JSON/深拷贝/混合credential别名、实际wire与guard；之后ledger/schema3、session/consumer、恢复/UI仍待完成，不将本Pass算功能完成或代码合并决定。

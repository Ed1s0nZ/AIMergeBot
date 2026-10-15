# S3c1：共用请求与消费检查层

2026-10-15；codex/sidebar-workflow；F4/F5。输入repository-credential-runtime-plan.md @6d9086f及technical-review-2026-10-15-runtime-fences.md @099bc47的限定Pass。仅完成S3c1内部执行层，不缩减完整S3c2/c3、S3d、S4–S6目标。

## 门禁与实际改动

P7/P8，adapter_extraction/narrow_fix。私有context helper与HTTP body wrapper独立文件；既有upstreamTransport、gitCommand/cache和Dynamic适配器薄委托。CODEOWNERS三个旧Dynamic.repo调用者随私有签名传播，保持冻结同一descriptor/ctx，返回前复查。无public API/JSON/DB字段、UI菜单或资产变更。

- 私有fence组合保留父检查，新捕获的descriptor不能替换旧操作的授权；ctx取消优先，内部授权错误规范为ErrRepositoryUnavailable，不输出原始SQL/secret。
- upstreamTransport在HTTP派发前、返回后和body每次Read前后检查。失效时关闭body、返回0/error，清除本次填入调用者buffer的字节，不把已填充但失效的内容交给解析器。
- DynamicRepository及DynamicAuditor从已绑定激活器一次捕获descriptor/config并传播ctx；方法返回前复查。未绑定保持原服务/standalone配置路径；没有暗中启用主服务激活器。
- CODEOWNERS加载冻结的reader与ctx同源，完成返回边界复查，不以失效目录结果证明文件不存在。
- Git命令派发/返回和路径缓存命中均检查同ctx；失败不返回stdout/paths；不改变Git网络策略、预算或source ACL。

## 真实证据

Go1.27.2，临时Settings/SQLite、httptest和临时受控Git脚本；未调用真实外部服务/模型。

1. 动态GitLab读：有效操作实际请求携带A；切换凭据后，旧操作再捕获新descriptor仍被父检查拒绝，真实请求数不增加；新操作可用B，不能靠隐式换token让旧操作继续。
2. 服务端阻塞在已发请求：改变凭据后释放响应，旧源码返回值为空且报错；只发生原先一次请求，不重发。
3. body Reader在填入PRIVATE字节的同一Read中撤权：返回0+固定错误，已填字节清除，body关闭。
4. 临时Git脚本marker：派发前拒绝时没有进程marker；返回后拒绝时marker存在但stdout为空。预填路径cache在拒绝后不返回路径。
5. 主审、独立finding复核、claim复核、压缩、group synthesis的真实生产model builder+SDK：有效控制均到达httptest；使用真实descriptor并经Settings.Save/SQLite旋转token后，旧操作HTTP请求0，检查确实执行。group synthesis用无fragment计划隔离聚合builder，不是完整多组审计验收；模拟模型输出仅用于到达HTTP，不证明审计质量。

## 本地验证与范围

- 初始编译发现Dynamic.repo私有签名的3个CODEOWNERS调用点未传播，已修正；没有保留无ctx旁路。
- 原专项HTTP/动态/Git/model测试exit0，2.180s，/tmp/aimangebot-s3c1-targeted.log。
- 完整go test ./... exit0，platform101.652s，其余通过/无测试，/tmp/aimangebot-s3c1-full.log。
- 同一生产源码广专项race exit0，5.759s；随后仅将模型测试从合成拒绝强化为实际Settings/SQLite凭据切换，广专项race再次exit0，7.091s，/tmp/aimangebot-s3c1-race-final.log；最终传入实际request ctx的五模型专项race exit0，3.057s，/tmp/aimangebot-s3c1-model-final.log。生产源未在全量回归后改变，新增测试强化未冒充之前已跑过。
- go vet ./...、go build ./...、frontend typecheck与diff --check exit0。前端无生产源变更，不重建dist；新精确生产HEAD CI另验收。

## 未完成的消费者与启用限制

Runner.Submit/execute、context准备、polling、followup的直接构造器尚未全部传递descriptor；入队须在同Tx检验generation，不能持单连接Store事务再次走Store.DB。模型返回到后续消费/持久化的最终边界仍须由这些实际运行消费者统一复查，不能把body检查当原子撤权保证。

启动尚未取得owner后绑定激活器；check/comment旧nil scope缺凭据版本，不按当前token/时间戳猜授权。必须完成S3c2/c3具体交接、S4正式scope producer和S3d恢复入口后，才可绑定生产。尚未部署、合并main、发送外部内容或关闭UAR-001/002/004；完整REQ/AC保持。

## S3c1精确HEAD CI

[CI38757566129](https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/38757566129)已读取completed/success，headSha=7b46000a630a0d6ed8f3197c804d8ab8ef4c1faa。Go/race、Operations、漏洞扫描、Frontend build、Embedded build全部success，无skipped；不能代替后续S3c2或整体启用验收。

# 整体检查：v35

检视基线 main 925dfffeb7b5519cc8b4be7a58ddbc41f2293be5；独立检视 head 8e6bf3105e59bad638af1c3a4011abc18f333897。覆盖生产差异与 checks、plan、来源链、预算、分组停止及消费者；生成 web/dist 未逐行检视。独立检视裁决 REQUEST_CHANGES，发现 AWO-REV-002（S2 blocker）。

## 本轮修复

固定跨项目 context 的读取包装丢失 ErrRepositoryUnavailable，使后续模型和分组仍可执行。真实 SDK 对本地 HTTP 服务的反例测试在修复前失败：单组 fatal 请求2次，分组4次。修复只包装安全 sentinel，不泄露底层错误内容；普通缺失文件不停止。修复后同一测试通过：fatal 请求1次、分组 failed/repository_unavailable、后续 unprocessed；普通错误可继续。相关主仓库停止、候选保留回归通过。此证据为本地模拟协议验证，不是实际服务质量评测。

## 质量与外部阻塞

同一六例 v35 回归已执行，接口持续 HTTP402：701 保留1条发现后失败（100715 reported tokens、UsageComplete=false），702–706 均失败且无有效模型输出。六例全部不能计为通过或真阴性；零 reported tokens 不能证明实际零计费。不推断具体余额或402原因。证据目录 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v35-20261006，286文件哈希验证；corpus SHA256 990a51d8a91cde35a54eeae68b999fd98208b64e2da61b0831428b86890bd1a6。冻结代码8e6bf31、15 steps / 80 tools / 240 seconds。停止重试，等待已授权接口恢复。

## 剩余空间

优先完成稳定的调查记录与独立质量验收：有效负例不能因零发现自动通过；跨项目结论必须有固定源码、调用/部署关系及条件证据；before/head/relationships 缺项须显式保持未完成。系统导航和结构门禁已经加入，但尚无证据证明语义正确性或总体质量提升。避免继续堆提示词或依赖单一已见语料；真实回归恢复后才评估下一步。新增语言不应靠白名单支持，跨项目应保持明确授权及固定SHA。当前不具备最终验收完成结论，未合并或部署本轮分支。

## v36 独立复查（当前生产591fe99）

固定base925dfffeb7b5519cc8b4be7a58ddbc41f2293be5，head6944232899e8e795077e5bd2bc4232a3b692ddc6；生产delta8e6bf31..591fe99，其后为文档。fresh-context只读复查裁决COMMENT，未发现新增生产blocker；AWO-REV-002 resolved。固定sentinel保留停止链并隐藏底层错误；导航只投射计数/有限gap/固定源ID，不插入claim或源码。定向真实SDK本地HTTP与导航测试exit0，平台3.055s；未请求真实模型或读凭据。591fe99 CI success，run37423152398。

范围限制：此复查不是全部PR重新逐行审查，不证明模型语义正确；AC002–006与011尚有证据强度缺口。v36两个正例有关系记录缺项及独立复核引用失败；负例claim/status极性错误，706无账本。合理unknown不要求completed，结构门禁也不能替代正确判断。当前仍为部分交付，后续需新冻结隔离验收以及更强的模型记录一致性证据；不自动合并或部署。

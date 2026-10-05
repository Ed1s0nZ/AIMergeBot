# 静态审计回归评测

`corpus-v1.json` 是11个自建正负对照用例和未确定边界，用于同一个语言无关的 Eino/Git 审计入口。它不是官方 Benchmark 或生产准确率样本。期望、风险假设、精确锚点存在被审计的临时 Git 仓库之外。

```sh
go run ./cmd/audit-eval --config config.yaml --output /tmp/aimangebot-evaluation-new
```

配置必须已存在；不修改配置、生产数据库或 GitLab。每轮使用新目录，顺序执行、每项最多240秒、独立复核开启、时序图关闭，不运行样例代码。输出保留固定提交、策略/模型/语料摘要、结果/工具观察、覆盖限制、耗时与提供商报告的 token。凭据和提供商地址不写入报告。`--resume` 仅恢复同一参数/语料/代码版本，保留已结束用例，包括失败；中断且无结束记录的目录需人工检查，不自动重跑或覆盖。

`--case case-001` 只运行诊断用例，不能作为完整语料成绩；`--probe` 用最小请求检查当前接口，仅输出状态及允许公开的错误码，不输出响应正文和密钥，不跟随重定向。

`expected_anchor_matched_preliminary_only` 只是机械匹配，不是TP结论。必须人工检查风险机制和保护条件，允许合法相邻锚点，不根据标题关键词给分。区分候选发现、主调查支持和独立静态复核，不声称运行利用验证。连接失败/覆盖不足单列，不能把无结果当成安全，也不能移除失败后伪造高分；usage缺失时不能报告实际计费为0。配置或代码修复后完整重跑要保留旧轮次及原因。

首次真实尝试见 [连接失败报告](../docs/feature/audit-contract-hardening/evaluation-connection-failure.md)，当时凭据返回401 invalid_api_key。后续 [第四轮真实评测](../docs/feature/audit-contract-hardening/evaluation-round4.md) 已完成策略v12的11例自建语料并记录人工判定；这不是当前策略的代表性质量基线，也不证明生产准确率或跨仓库覆盖。历史失败及各轮结果应一并保留，不能将历史凭据状态当作当前接口状态。

## 真实历史 PR 输入

已授权的本机 Git 仓库可以用独立、私有的语料文件指定固定提交。语料与结果目录必须在被审计仓库之外（含符号链接路径）；此模式不联网、不检出、不执行代码，也不创建 GitLab 评论。

```json
{
  "version": 1,
  "kind": "real-git-history-not-representative-benchmark",
  "cases": [{
    "id": "case-001",
    "expectation": "positive",
    "git": {
      "directory": "/absolute/path/to/authorized/repository",
      "base_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "head_sha": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
    },
    "expected_anchor": {"file": "path/to/file", "side": "head", "line": 1},
    "rationale": "人工标注的风险机制、假设和反证"
  }]
}
```

替换示例 SHA 为仓库中实际完整 commit ID，不能使用 HEAD、分支、标签或 blob ID。不能同时提供 `base_files/head_files`。使用 `--corpus /private/path/pr-corpus.json --output /private/path/new-results`；结果权限0600、目录0700，但含源码，仍需按仓库敏感级别保管。模型只收到中性编号、固定提交和实际差异，期望/理由留在仓库外，不用于模型输入。工作树未提交的修改不影响固定对象读取。

机械锚点匹配仍不是检测准确率；真实PR应由人工逐项判定误报、漏报、触发假设与覆盖不足，保留无结果、失败及不确定案例。没有真实仓库样本时，入口测试只能证明输入与隔离流程，不能生成真实质量结论。

## 自建关联仓库场景

自建 case 可额外提供 `context_repositories: [{"repository_id":2,"files":{"svc.py":"..."}}]`，最多8项，ID不得与主仓库1重复。上下文生成独立固定 Git 提交，正常 Eino 上下文工具只能读取显式列出的 ID/SHA；结束 receipt 保存 `context_repositories`，供核对来源。文件校验和隔离规则与主仓库相同，不执行源码、不克隆网络仓库、不向模型提供期望或判定理由。

当前扩展只接受自建上下文，真实历史 Git case 不接受该字段。准备器回归只证明固定对象和输入边界；跨仓库检测质量必须在标注的正负对照案例上实际运行并人工核验。

# 隔离验收 v2：v28 首次运行

代码快照：`f0f23e7cb351a8e42db2ac85a37a6abd9af94791`；策略 `eino-audit-contract-v28`。输入在运行前冻结，SHA256 `675923cd5ecf2a6831e17e65fe1ea3a53e183c23f3750dcd973d18b9ce35bc70`。来源为人工合成样本，机制与调试集有重叠；不代表未见漏洞类别或生产准确率。

命令：`go run ./cmd/audit-eval -config config.yaml -corpus evaluation/corpus-acceptance-v2.json -output /tmp/aimangebot-acceptance-v28-v2-evaluation-20261006 -timeout 240`，exit 0。deepseek-chat，单组，保留配置预算。未执行样本代码。

| 用例 | findings | 状态 | tokens | elapsed ms |
|---|---:|---|---:|---:|
| case-401 | 1 | incomplete | 70196 | 17076 |
| case-402 | 0 | incomplete | 14460 | 2437 |
| case-403 | 1 | incomplete | 104897 | 18360 |
| case-404 | 0 | incomplete | 67776 | 10264 |
| case-405 | 0 | incomplete | 16511 | 2490 |
| case-406 | 0 | incomplete | 22909 | 3897 |

总 tokens：296749；价格未配置，费用为 null。6/6 状态 incomplete；流程退出成功不等于审计完整。没有本轮预算耗尽或基础设施失败记录。

## 人工核对

- 401：Rust 删除所有权校验，finding 正确定位 BASE 第 9 行，独立复核 full/supported，示例 bob→d-a 与源码一致。首次提交错误使用 HEAD 锚点后修正，失败记录仍保留。外部入口与运行时可达性未证明。
- 402：新增同一校验，无 finding，正确识别修复。
- 403：PHP 接受 owner 参数并转发到 Python 下游，主调查批读 records.py/deployment.conf，独立复核重新读取。HEAD 第 6 行 finding 与条件风险相符，full/supported。主调查将 internal-service 推成“只能经 portal 可达”缺少网络策略证明；复核明确保留入口/网络未知，不能将其当已证明事实。
- 404：相同主 PR、不同下游实现；读取固定下游按 Authorization 派生 owner、忽略 body，拒绝风险并零 finding。HTTP 连接边标记 inferred，无运行时证明。
- 405：C#→Ruby 默认值兼容负例，零 finding，但主调查没有读可用下游；模型称其“不需要”不能证明兼容。服务端追加缺失上下文覆盖记录，判为证据不足的负例，不能算完整通过。
- 406：Java 未变 exec 历史缺陷，仅日志文字 PR，零 finding，正确区分历史缺陷与 PR 因果风险。

两个预设正例均返回条件成立的正确 finding；四个负例零告警。其中三个负例具有对应反证/因果判断，405 证据不足。没有将零告警直接计为完整验收成功。此前金融调试样本仍存在误报/方向错误，本轮结果不能覆盖或消除这些记录。

## 证据归档

完整 metadata、ground-truth、receipts、checkpoints、固定 Git 样本保留于 `/Users/worker/.codex/evaluation-artifacts/aimangebot/acceptance-v28-v2-20261006`；逐文件 SHA256 已复读校验，见 sha256-manifest.json。原始记录没有清理失败、改写覆盖说明或改变冻结期望。v2 尚未用于代码调参；未来用于修复后应改称回归集。

# v35 接口恢复回归

用户确认充值后，以原接口/模型、相同六例已见语料及预算重新执行。冻结代码31c301bdb556253e85cf325ff0e0a5b71c91b250，策略v35，15 steps、80 tools、240秒；corpus SHA256 990a51d8a91cde35a54eeae68b999fd98208b64e2da61b0831428b86890bd1a6。此为回归，不是新的隔离验收。执行 exit0；六例不再出现HTTP402、UsageComplete均true，价格未配置，不报告货币费用。

| 案例 | 状态 | 发现数 | tokens | 已观察缺口 |
|---|---|---|---|---|
| 701 删除权限检查 | incomplete | 1 | 133154 | 提交曾失败，发现未记录source-linked relationships |
| 702 恢复权限检查 | incomplete | 0 | 16206 | 无调查计划 |
| 703 跨项目租户覆盖 | incomplete | 1 | 105906 | 下游与部署映射已读，关系保留inferred |
| 704 下游独立认证 | incomplete | 0 | 61376 | inv-1未收尾 |
| 705 兼容默认值 | incomplete | 0 | 60536 | 已读固定下游；外部调用/适配器未知，inv-1未收尾 |
| 706 无关历史危险操作 | incomplete | 0 | 16270 | 无调查计划，未将旧sink当PR新增风险 |

合计393448 tokens。数量仅为模型输出事实，尚未完整逐条语义验收，不能据此认定两个正例完全正确或四个负例为通过。与上轮相比705固定下游读取缺口已消失，但缺乏受控重复比较，不能声称总体提升。

证据保存在工作区外 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v35-restored-20261006。冻结元数据与实际metadata的代码SHA、corpus、steps/tools/timeout逐项匹配，286文件SHA256归档后验证。下一步核对来源锚点、调查账本、计划状态及失败工具输入，优先定位通用契约原因，避免针对案例写特判或自动伪造证据。

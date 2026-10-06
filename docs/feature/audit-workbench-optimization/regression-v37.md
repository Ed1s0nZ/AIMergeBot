# v37 陈述保留回归

冻结e040b090b3e23483c0b21eaef25062ef9a2e7aa1、策略v37，使用已分析801–806语料（SHA2561851b934d057b5a5696ac9a26c461ab38087a5db04f86d90b7742ffac2e46b4b）；原模型/接口与15steps/80tools/240秒。无预算调整。此前256b6f9预备冻结未执行，目录保留not-executed声明；此次为回归而非新的隔离验收。

六例全部incomplete、exit0，UsageComplete全部true，总479052tokens、费用价格未配置、无HTTP402。真实resolve_recording_errors调用数为0；不能以本轮证明纠正效果，更不能把较少tokens归因为策略改善。

| 案例 | 发现/定位 | tokens | 人工维度核对 |
|---|---|---|---|
| 801 | 1，BASE release.go:7 | 179582 | 条件审批风险方向支持，双侧及实际Handle→Release关系记录、独立supported；草稿记录可达条件未知保留。记录错误侧和提交来源链接被拒后修正，旧pending仍在，未调用纠正 |
| 802 | 0 | 17033 | summary识别状态guard修复，但没有调查账本；错误声称仓库只有release.go，实际caller.go/workflow.md存在且未读，不能算完整防护负例通过 |
| 803 | 1，HEAD gateway.ts:6 | 117069 | 固定PHP源、表单传值和原始SQL条件风险方向支持，独立supported；relationships仍缺失，路由/driver限制明确保留 |
| 804 | 0 | 80154 | 固定下游参数绑定保护被识别，本次兼容Claim为supported并无finding，状态极性吻合；未知实际服务/运行限制作记录 |
| 805 | 0 | 68113 | 固定Python缺省25与范围guard已读；兼容Claim仍被rejected，命题极性错误，不能称正确收尾 |
| 806 | 0 | 17101 | 正确排除旧命令执行与日志常量改动因果关系，但没有账本计划 |

正例两条条件锚点命中、负例四条零告警是观察值，不按六例全部正确或完整TP/TN计算。801获得source-linked真实调用关系，但不证明银行实际结算；803仍不具完整跨项目路径。未改写模型coverage_notes或ledger，不用工具格式成功代替语义正确。

独立复查：256b6f9相对4946659限定COMMENT、未发现原子/源码/只读边界blocker，但local ID无法证明同陈述。父任务加上Claim及Title/Description/Trigger精确比较；e040b09补充只读复查COMMENT、定向go test TestRecordingCorrection exit0（0.765s），同ID改写陈述反例被拒。字节相同依然不证明证据关联、漏洞语义或运行可利用性。候选来源/锚点可修正，改写陈述须保留旧未解决项。父任务相关race3.717s/vet通过，整项全量证据见implementation.md；修订CI已终态success：e040b09对应run37425803623（https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37425803623），原256b6f9对应run37425429000也success。

298归档文件SHA256验证，freeze与metadata代码/语料/预算一致；证据目录 /Users/worker/.codex/evaluation-artifacts/aimangebot/regression-v37-statement-20261006，合成源码未进仓库。下一步先检查801实际失败/成功回执是否具备合法纠正条件及可发现性；不盲目重试、不自动解除真实缺口。

## 充值恢复后的回执补查

只读比较801 trace中的记录参数：失败observation-10与成功observation-11的id/claim精确相同；失败observation-15与成功observation-17的id/investigation_id/file/type/title/description/trigger精确相同，二者具备同陈述纠正的必要匹配条件。observation-13与成功observation-17的description不同，因此不能用当前工具解除该旧项。此处只核对记录身份与先后顺序，没有重放工具、修改回执或声称全部服务端条件已运行验证。

这将下一步缩小到合格记录纠正的可发现性：即使模型解除前两项，observation-13及其他实际证据/方法缺口仍不能自动消失；仍不能承诺801会成为completed。802漏读未修改关联文件与805命题极性错误是独立质量问题。充值恢复已验证，无须因这三个问题再次确认余额或盲目追加同语料评测。

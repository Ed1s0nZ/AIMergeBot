# README 展示图片

本目录保存根目录 README 使用的界面截图。使用英文语义文件名和相对路径，便于 GitHub 展示与后续替换。历史 `image/` 目录保持原样。

## 来源

截图日期：2026-10-06（Asia/Shanghai）。截图来自本地 AIMergeBot 工作台的合成示例任务 `Synthetic fixture: simplify resource deletion`，展示 `service.js` 中对象级权限检查被移除的静态审计结果。使用 Chrome 桌面界面重新截图，关键面板按内容区域截取特写，避免旧截图的文字模糊。另保留原生 SVG 时序图，支持无损放大。演示服务使用隔离的临时数据库和示例配置，不连接真实模型或 GitLab。

示例代码未执行；发现、独立复核和时序图用于界面演示，不作为真实业务漏洞、运行复现或模型质量评测。截图保留实际的覆盖不足、推测关系与未配置费用状态。

| 文件 | 展示内容 |
| --- | --- |
| `finding-sequence.svg` | 原生矢量时序图，可无损放大 |
| `finding-sequence.jpg` | 完整时序链路、风险步骤、推测关系与图形操作 |
| `sequence-evidence.jpg` | 选中风险步骤后的 BASE/HEAD 代码证据 |
| `independent-verification.jpg` | 发现依据、触发条件、建议与独立复核 |
| `audit-runs.jpg` | 审计提交入口、筛选与任务历史 |
| `audit-detail.jpg` | 固定提交、摘要、覆盖限制与补审入口 |
| `model-usage.jpg` | 分阶段请求与 token 用量、费用估算状态 |

## 更新约定

- 优先使用合成或公开示例；发布前检查是否包含私有代码、真实项目名称、成员信息、内部地址或凭据。
- 保留覆盖限制和推测标记；如截图来源或示例发生变化，同时更新本说明和 README 的描述。
- 图片只用于展示，不代替功能验收记录。新增图片后检查根 README 的相对链接。

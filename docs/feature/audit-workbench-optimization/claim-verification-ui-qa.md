# v45 调查命题复核工作台消费与 QA

Confirmed R3 AC006/007/010/012；阶段P10/F4–F5，分支codex/audit-quality-loop，无合并部署。运行契约基于dfc6340906bf4d2edf84b6ad84e1588fd1010df6（策略v45）。该后端修复full Go platform134.292s通过，CI37445789011已核对success；独立 scoped APPROVE、EXR-001 resolved，报告 /Users/worker/.codex/evaluation-artifacts/aimangebot/review-v45-checkpoint-dfc6340/PR_REVIEW_REPORT.md。该批准不覆盖本文 UI。

工作台新增可选ClaimVerification类型及单独展示组件，保持原命题/初审不变。supported/rejected分别显示“初审认为命题成立/不成立”，不把后者称“已排除风险”。显示consistent/disagreed/inconclusive/unavailable/disabled、assessed_claim、实际verdict、原因、复核模型（有记录时）、限制、固定PR SHA和fresh source链接；未执行状态不显示确定verdict。历史缺失、未收尾、进行中和终态缺失均不造结论。模型用量与trace新增命题复核/其压缩阶段；配置键verify_findings兼容，文案说明finding优先同预算、原判断保留。

ObservationLinks键盘导航：打开目标details并滚动，再focus其summary（preventScroll）。实际发现Return原先把焦点留在复核按钮，修复后Return焦点在对应fresh source summary，Tab到下一源码summary；#/runs/45不变。长digest按钮与trace证据编号都按anywhere换行；仅页面scrollWidth无溢出还不足，另核对编号元素clientWidth=scrollWidth=284。HTML样式的受控源码片段按文本显示，source img数量0。

实际IAB验证当前RunDetail组件，全部只读mock fetch；非生产数据库E2E、非真实审计质量。360×800、document.scrollWidth=360，五类状态均实际展开；初审rejected且独立true显示分歧，而非无漏洞。运行中unavailable明确尚未完成、nil显示等待记录；cancelled/failed缺失与历史不自动补填，investigating仍未收尾。保存两张有效截图、五类/运行中/取消/失败AX、Return/Tab测量及fixture；外部归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/current-claim-v45-ui-20261006/manifest.json 逐文件SHA256和八份UI源码digest绑定此次证据。首个源码截图遇到smooth-scroll未到目标，检查图像后补拍实际源码位置并核对编号在视口内；最终source-360.jpg为目标源码。

最终npm --prefix frontend run typecheck/build通过（Vite1627 modules、2.80s），产物index-iuBGSZKh.js/index-CfwgHs5D.css；go build嵌入应用、git diff --check通过。之前从frontend cwd使用根相对脚本路径失败，未产生源码改动；改为根cwd及npm --prefix frontend后完成。全部临时fixture已移到外部并删除本地副本，两个dev server停止、tab关闭、viewport reset。生成bundle按现有tracked策略随源提交。

待完成：当前UI frozen SHA独立契约/渲染/生成产物核对及exact-head CI，原模型真实回归，R3逐项完整证据审计。不能把合成fixture判断、绿构建或局部APPROVE当准确率提升或AER-001已关闭。

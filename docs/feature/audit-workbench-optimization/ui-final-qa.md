# 详情页受控组件最终状态QA

平台v28，实际RunDetail及useResource/子组件，临时Vite+HTTP fixture；没有部署、登录真实服务或真实评论写入。源码修复仅ResourceRefreshFailure及详情组合/加载重试禁用。

验证状态：初次500显示失败+重试，键盘Return后成功恢复；15秒延迟请求显示加载任务和disabled重试；运行中status/full500保留上次报告并明确旧快照提示；恢复后自动消失并显示最新摘要；pending队列显示项目并发等待、零发现不说安全；comment_sync unknown显示核对原评论、不重复创建；后续403轮询清除缓存报告，只剩权限不足+重试。

额外受控复核场景：实际FindingCard填写原因并向fixture PUT（synthetic204，不验证数据库保存），随后读取500，原finding和原因保持可见；恢复fixture后对新增重新获取按钮键盘Return，更新到最新零发现结果、旧快照提示清除。人工复核并发/数据库正确性由review_revision与HTTP原回归证明，这里不冒充真实后端保存。

360×800视口检查：失败提示/主要操作/固定SHA/评论状态布局截图，document.scrollWidth=360等于viewport；队列状态也无横向溢出。初始重试与缓存刷新按钮均有键盘成功恢复证据。截图及SHA256清单保存 `/Users/worker/.codex/evaluation-artifacts/aimangebot/detail-ui-20261006`，原截图 `/tmp/aimangebot-detail-refresh-proof.png`。恢复视口、关闭临时tab、停止2个临时服务并删除全部fixture源码。

类型和生产构建通过（Vite967ms），git diff --check通过。平台源码未改，不重复已通过Go全量；没有加入与实现镜像的UI单元测试，采用实际状态/浏览器验证。此前筛选/证据导航/草稿冲突/范围提示浏览器验收见implementation各切片。未验证所有浏览器/设备，未覆盖线上服务故障或真实多人协作，不外推为生产E2E。

## 最终审查修复：发现导航路由冲突

AWO-REV-001：原发现导航 href=#目标 与 main.tsx hashchange 路由冲突，可能丢失详情页。改为原生 button 的 scrollIntoView + focus，保留 route hash。实际 IAB 受控组件（真实 FindingWorkbench，#/runs/7）Return 激活后 hash 不变、hashchange=0、activeElement=_r_0_-finding-qa-1、scrollY=284.5；随后 Tab 到复核草稿，原值保留。截图 /tmp/aimangebot-navigation-proof.png。不是生产 API E2E。首次从仓库根启动 npm 因 package.json 不存在退出254，转 frontend 后启动成功；fixture CSS 拼写修正后验证，临时文件已清理。

最终 TypeScript/Vite build exit0，871ms；产物 index-qbvfyjN1.js/index-CQVLkub2.css。导航修复无模型或平台代码变化，未重复隔离评测。

## v38 当前RunDetail新面板补验

源码da039f3保持不变，真实RunDetail及当前CSS，以只读mock HTTP返回fixture，不包含真实漏洞/凭证。IAB实际360×800：四项静态复核（2supported/2inconclusive）及partial提示、历史缺checks提示；Return展开调查计划checked/pending/unavailable与旧无plan提示。Return计划observation-1与分项verify-fixture-observation-1分别打开实际trace详情/source；Tab继续到下一来源控件，hash仍#/runs/38，scrollWidth360。当前agent_step_budget显示决策轮数预算耗尽，初始fixture拼错枚举显示未知停止类型；修正的是fixture，无产品变更，未知处理保留截图。

归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/current-detail-v38-ui-20261006：五份fixture/截图SHA256和manifest；写操作显式禁止，不验证数据库保存或线上E2E。首次npm在根目录exit254，改frontend后Vite162ms ready；临时server73752终止exit130、tab14关闭、viewport reset、两fixture文件删除，git status空。独立工作台review核对五hash/当前source/完整截图，接受补证关闭AC007最小缺口；未冒充reviewer自己执行浏览器。

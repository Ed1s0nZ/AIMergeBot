# 详情页受控组件最终状态QA

平台v28，实际RunDetail及useResource/子组件，临时Vite+HTTP fixture；没有部署、登录真实服务或真实评论写入。源码修复仅ResourceRefreshFailure及详情组合/加载重试禁用。

验证状态：初次500显示失败+重试，键盘Return后成功恢复；15秒延迟请求显示加载任务和disabled重试；运行中status/full500保留上次报告并明确旧快照提示；恢复后自动消失并显示最新摘要；pending队列显示项目并发等待、零发现不说安全；comment_sync unknown显示核对原评论、不重复创建；后续403轮询清除缓存报告，只剩权限不足+重试。

额外受控复核场景：实际FindingCard填写原因并向fixture PUT（synthetic204，不验证数据库保存），随后读取500，原finding和原因保持可见；恢复fixture后对新增重新获取按钮键盘Return，更新到最新零发现结果、旧快照提示清除。人工复核并发/数据库正确性由review_revision与HTTP原回归证明，这里不冒充真实后端保存。

360×800视口检查：失败提示/主要操作/固定SHA/评论状态布局截图，document.scrollWidth=360等于viewport；队列状态也无横向溢出。初始重试与缓存刷新按钮均有键盘成功恢复证据。截图及SHA256清单保存 `/Users/worker/.codex/evaluation-artifacts/aimangebot/detail-ui-20261006`，原截图 `/tmp/aimangebot-detail-refresh-proof.png`。恢复视口、关闭临时tab、停止2个临时服务并删除全部fixture源码。

类型和生产构建通过（Vite967ms），git diff --check通过。平台源码未改，不重复已通过Go全量；没有加入与实现镜像的UI单元测试，采用实际状态/浏览器验证。此前筛选/证据导航/草稿冲突/范围提示浏览器验收见implementation各切片。未验证所有浏览器/设备，未覆盖线上服务故障或真实多人协作，不外推为生产E2E。

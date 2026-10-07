# 管理员权限边界修正

检查发布队列回归发现，roleRank 将 operator 与 admin 同设为 3，使 requireProjectRole(required=admin) 在内部允许 operator。既有项目策略 HTTP 路由另有 admin middleware，但 Store.SaveWorkflowPolicy 必须独立保持管理员边界，不能依赖调用者永远有正确中间件。

修正：admin=4、operator=3、reviewer=2、viewer=1。常规 submit/cancel/review 能力仍按原阈值，项目成员仍不允许授予 admin。策略测试改为真实拥有 operator 权限的成员，要求保存拒绝；管理员保存、冲突与新旧策略快照测试保留。

项目/策略/检查协议与队列的定向 race 9.977s 和 Go vet 通过。修正前已启动的完整测试以同一 operator 拒写回归失败（platform 79.096s），不能当作修正后的结果；修正后完整 Go 已重新启动，远端精确提交 CI 待推送后验证。检查队列实现仍单独开发，尚无生产发布入口/自动消费者，不包含在此权限修正提交。

关闭证据：9c07eb49368a265871085184502406af400578dd 的 GitHub CI 37588299319 completed/success。本地修正后完整 Go platform 77.884s 通过（含工作树检查队列切片）；远端精确提交验证管理员权限修正本身，未把未提交功能当作已部署能力。

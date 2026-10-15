# S3a私有凭据状态协议实施记录

2026-10-15；codex/sidebar-workflow；F4/F5；上游S3计划@37e1652与technical-review-2026-10-15-credential-activation.md的Pass。当前仅实现S3a，不声明完整S3撤权、仓库审计接入或恢复能力完成。

## 门禁与范围

P7/P8内部依赖，用户已确认的仓库恢复与固定身份目标不变。F3已提交推送后开始。Maintainability：两个新生产文件分别负责material构造和SQLite状态，既有Store/Settings未扩写；允许narrow_fix/adapter_extraction。新API均私有，没有public JSON、HTTP/YAML配置字段、schema或启动行为变化。没有绕过reader保护或增加UI壳入口。

## 已实现

repository_credential_material.go由现有GitLab SDK实际BaseURL得到完整API前缀，保留旧HTTP与大小写path并消除现有构造器忽略的尾斜杠区别；用同endpoint计算有效proxy。固定struct v1编码token精确UTF8、实际proxy和legacy_http_default_v1/空CIDR，再用持久key做HMAC-SHA256。不借用通知CIDR、不声称旧transport已受native网络限制。拒绝无效UTF8以避免JSON替换不同秘密字节；原SDK/proxy错误固定脱敏。私有对象fmt固定标签，JSON无公开字段。

legacy_repository_state.go从单条SQL同时读取singleton数量、generation/key/fingerprint及SQLite typeof；缺失/损坏拒绝，不初始化key或counter。Tx激活helper只在caller事务内UPDATE，以便后续generation与旧任务取消共同提交；首激活1，同值不变，变化递增，A→B→A不复用；MAX仅允许未变凭据。validate只读核对正generation和keyed fingerprint，不代替ACL/lease，也不自动激活。

这些helper尚无生产调用；state仍由S2保持generation0，现有运行不被悄悄改变。其价值需由S3b/c/d实际接入证明，不以本切片声称用户操作链路已交付。

## 反例与验证

合成配置、临时SQLite、纯SDK构造，无真实仓库HTTP/模型/外发。新增Go测试覆盖：
- 未激活拒绝、A→A→B→A的1/1/2/3、旧A与不同凭据拒绝。
- 激活后caller后续SQL失败并Rollback，generation/key/fingerprint完全保留；MAX不同凭据拒绝、相同允许。
- 两个真实OpenStore连接、8个并发请求同配置，仅一次changed且全为1；另一连接旋转后旧连接拒绝；重开同凭据保持代次。
- 缺行、坏key长度/SQLite TEXT、坏generation类型、generation/fingerprint矛盾、TEXT fingerprint、额外singleton行全部拒绝，原始状态manifest不变。
- token精确空格、path大小写/协议、proxy改变均改指纹；SDK等效尾斜杠不改；换key改指纹；格式化/JSON不导出秘密；错误不回显proxy正文。

最初专项go test ./internal/platform -run TestLegacyCredential -count=1 exit0（0.843s）；补充跨连接撤权/重开后，专项race已exit0（10.049s，含identity/schema回归），go vet ./...和go build ./... exit0，Go1.27.2。完整go test ./... exit0（platform100.964s，其余通过/无测试，/tmp/aimangebot-s3a-full.log）；race日志/tmp/aimangebot-s3a-race.log。前端生产源/锁文件/资产未变，未重建dist；新HEAD CI另核实。

## 尚未完成与后续

S3b配置文件/DB/内存的pending激活、同事务取消与启动；S3c全部legacy reader/模型请求/发布消费者的运行fence；S3d同进程reload及现有配置页恢复反馈仍未实现。S4原生provider及S5/S6恢复闭环、UAR-001/002/004及完整原需求继续保留。当前没有main合并、部署或真实服务联调。

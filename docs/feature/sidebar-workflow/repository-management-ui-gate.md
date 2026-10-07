# 多平台仓库管理页面

Workflow Gate：Confirmed完整REQ及repository-project-create-design/绑定HTTP/identity-sync为输入；P6→P8/P9，API15c182d已推，本地完整验证通过，精确CI37633523912仍待终态。现有项目/集成布局和共享反馈组件可复用，无新设计系统/Figma依赖；缺少具体UI状态，本文件补齐。implementation allowed=yes（契约/F3先推）。F2，codex/sidebar-workflow，无main合并/发布；G3–G6及其余完整REQ不缩减。

API projection：GET /projects在当前用户一致权限/身份快照中追加repository?:RepositoryBinding与repository_execution_available?:boolean（当前bound false）。nil仍legacy GitLab；损坏binding整个请求固定错误、不标legacy。Store.ProjectsForUser实际读取enabled actor及role，项目列表与绑定同一短事务，拒绝stale caller role。当前Project原字段保持；Store.Projects内部不变。项目列表避免每卡发绑定HTTP，也不泄漏token。

UI场景：集成页GitHub/GitLab scope可空，前提仍项目数据成功加载；其他kind至少一启用项目，项目503/403/slow仍禁止保存。repo profile无需通知事件/频率操作（保留已有字段），说明空scope仅保存凭据、新项目创建会明确关联。启用repo且替换凭据时token必填。403/409及秘密清空沿用原机制。

Projects列表仅data ready时显示；刷新/loading/error不显示旧卡。legacy显示GitLab默认配置与远端ID；bound显示provider/full_name、remote ID与独立内部ID，当前审计未接入，不能用enabled冒充审计可用。member只看有权限项目，admin可打开binding editor；全局成员/owner/context工具保持。新增原生创建panel独立于旧GitLab默认配置添加入口，选择当前enabled repo profile（has_endpoint/has_secret、scope容量）及显式provider/origin/remote ID/full name/name，使用其revision。没有可用profile指向集成页；不自动保存凭据、授予ACL或调用远端。

创建状态：提交前sessionStorage按当前user ID保存不含凭据的完整payload与crypto.randomUUID，恢复时保持同参数/key。发出后冻结字段，unknown/network/500（含sync pending）只允许原请求重试；切页/重载保留attempt，busy不可重复submit。201/200后清attempt，刷新项目/集成并显示内部ID。409保留attempt/草稿锁定，显式核对项目并开始新请求才丢弃；403清编辑状态且锁定，新登录actor不能读取其他actor的attempt。storage失败禁止首次POST，损坏恢复记录不自动创建。关闭unknown panel不擦除attempt。新草稿只有列表data ready且当前选项仍可用时能提交，scope/配置变化须原CAS验证。

绑定状态：GET绑定与admin集成并行有界加载/abort，loading/error隐藏旧配置。expected revision严格来自GET；选择同provider/启用/包含当前项目的profile。403/404清配置并禁止写；409保留草稿锁定，仅明确丢弃并重读解锁。500同步pending表示已保存，GET重读新revision，不能按旧版本再写。表单保存前说明当前绑定执行暂不可用、历史任务不改解释；保存成功刷新list/provider。读写不发送真实消息或触发审计。close返回触发按钮focus，panel focus/label/fieldset/busy/role=status/error与窄屏无溢出。

Maintainability Gate：projects.tsx多责任medium/high，只组合独立repository-project-create.tsx、repository-binding-editor.tsx及共享repository-form.tsx/types；integrations.tsx压缩混合security/render/runtime high，只改scope/说明/token gate，不堆创建状态。page-utils useResource沿用；新增attempt helper负责有界会话存储与输入恢复，creation不混入Project主组件。ProjectForUser权限/IO high，独立身份投影委托或一致Tx保持小方法，不能先公开未授权metadata。adapter_extraction，先不广泛重构。

## F3 实施计划

先加Project可选repository/projection状态，ProjectsForUser一致Tx及真实HTTPviewer/撤权/坏binding无泄漏测试；新增前端共享RepositoryBinding/RepositoryIntegration/receipt types与表单、creation attempt会话helper、独立创建/绑定组件。Projects/main只组合/传userID/focus恢复；integrations scope空仅repo、错误数据继续禁保存，api.ts脱敏code中文。CSS使用现有panel/inline-form/项目样式及必要responsive字段。

真实API回归race/完整Go/vet/build（后端projection变更）；frontend typecheck/临时build，再受控fixture服务/browser证明无项目repo profile可保存而webhook不可、项目error/slow保留禁止；创建成功及同key未知响应/重载/重试、409草稿、403清理、选项变化；binding初始/slow/error/409/500同步pending/403、正确provider/禁用状态/member只读；360×800与键盘focus。所有POST/PATCH仅本地fake API记账，不是远端发送或真实审计。测试fixture源码/tmp与截图不提交；最终正式embedded assets更新后再Go build/vet，避免同时删除embed目录。15c182d CI37633523912终态后才推生产，新增精确HEAD CI独立验证。F4/F5文档记录真实证据，未证明的项继续标未完成；整体scope及G3–G6不变。

## F4/F5 当前实现与增量验证（未完成整体验收）

已实现独立创建/绑定面板、共享字段与类型、按actor恢复的会话创建请求、平台身份卡片及同事务权限/身份投影；集成页允许GitHub/GitLab空scope凭据初始化。真实浏览器发现repo profile被错误显示工单说明，已修正为仅Jira/Linear显示，重新typecheck/临时build并重载核对消失。生产改动目前仍在工作树，尚未提交；本节只保存已经取得的证据，不把剩余矩阵视为完成。

最新后端15c182da2d3c178f75ef4d6e5bd86919d91035f2精确CI37633523912已completed/success。当前投影修改的全Go47118 completed/success（platform120.285s，其余包通过）；专项真实HTTP/SQLite权限与绑定race77383 completed/success（platform17.249s）。当前前端typecheck10385与修正文案后typecheck/临时Vite97968成功，临时assets index-Bz59-PGC.js/index-RiSjjNhI.css，不是正式embedded发布构建。

受控本地fixture127.0.0.1:8803，所有写入只作用于临时内存/JSONL，不读取真实凭据或发送外部通知。实际UI验证：
- 初次创建返回内部项目#3，列表区分内部ID与remote88并说明审计接入待完成；凭据版本刷新。
- create_unknown模拟服务器已提交但响应500，原请求冻结且提示待确认。浏览器重载/重新打开面板恢复相同UUID；重试成功返回已有项目#4。fixture日志对两次POST完整body逐字段比较完全相同（包含旧expected_integration_revision=2，虽然UI选项已是版本3），列表只有一个对应项目，成功后attempt提示清除。
- binding409保留org/conflict-draft并锁定字段/保存。显式丢弃并重读恢复版本1和原路径。binding_sync模拟提交后同步500，显示“已保存/同步待恢复”且禁止旧版本重写；重新GET读取版本2和已提交org/sync-committed。PATCH403清除全部编辑字段，仅显示权限不可用提示。
- 项目GET503后缓存卡片消失，创建入口disabled；显式刷新/恢复后列表能重新读取。
- GitHub空范围凭据使用模拟token成功保存；fixture只输出kind=github/project_ids=[]证据，未输出token。保存后凭据输入隐藏，只显示has_endpoint/has_secret元数据；重载后错误工单说明不再显示。
- 360×800实际完整截图检查创建字段/按钮可见且单列，无文档横溢（scrollWidth=innerWidth=360）。项目名称Tab到仓库平台select，关闭创建面板焦点回到创建触发按钮。结束恢复默认视口。

尚待：创建409/403的实际浏览器矩阵、profile与binding慢/错误恢复、空项目下repo与webhook对比、项目slow/error时集成保存禁止、member只读及绑定面板窄屏/关闭focus；最终正式embedded构建+Go vet/build、独立生产提交/精确CI。GitHub实际provider执行G3–G6、自动推荐路由、邮件/bots外部闭环及其他完整REQ继续保留。没有main合并或发布。

## F4/F5 页面切片收尾

补充实际浏览器矩阵：create409保留原UUID和草稿、字段/原请求重试锁定；显式放弃后字段解锁且下次POST使用新UUID。create403清全部可见编辑字段，关闭后原actor的耐久attempt仍可恢复，防止丢失不确定结果。profiles503时无新建表单/提交入口；原attempt恢复时错误明确显示，配置标签改为“尚未读取”，不再把读取失败误报为确实没有可用配置。profiles三秒慢加载显示加载提示并禁刷新，完成后恢复原草稿。binding三秒慢加载无旧字段；刷新GET503后清旧版本/字段并允许显式重读。

绑定成功后的父列表刷新原先会使子面板成功提示消失，已由父页面持有保存回执提示；选中仓库按钮采用React ref恢复，列表重新挂载后关闭仍能聚焦当前按钮。实际PATCH成功后列表org/saved-final与binding版本3显示，成功提示保留；360像素绑定表单所有字段宽258、文档宽360无横溢，关闭聚焦GitHub fixture卡片仓库绑定按钮。

空项目列表下，同一名称草稿的Webhook保存disabled、GitHub及GitLab保存enabled；前轮本地实际GitHub空scope保存已证明请求project_ids=[]，其真实后端创建/scope语义由15c182d精确CI证明。项目scope三秒慢读取与503时，即使repo草稿也disabled，不能用空范围绕过读取失败；项目页慢加载不显示身份卡且新建disabled。member重新认证加载后只显示平台/路径/内部ID/remoteID，没有创建/绑定/停用及管理导航。后端真实成员撤权/损坏身份/停用账户/伪admin caller由投影测试覆盖；fixture不作为服务端ACL证明。

修正profile提示时一次机械替换误加参数被tsc拒绝，立即删除误加实参后typecheck64056成功；最终焦点/反馈补强后typecheck和临时Vite54548成功。正式npm run build85336成功，JS index-DOrFma53.js与CSS index-RiSjjNhI.css的SHA256与实际浏览器验收的临时产物逐字一致。全Go47118 success（Go生产代码之后未变），最终正式embed Go build ./...及go vet ./...50469 success，git diff --check通过。当前切片可提交推送并由独立精确HEAD CI全量验证，不用旧CI替代。

仍未覆盖所有浏览器组合（存储禁止/损坏、切账号、写请求途中卸载、凭据选项在请求期间变更）；这些分支采取阻止新建或保留原请求、后台权限/CAS为最终边界。没有真实GitHub/GitLab凭据连通测试/外部发送/模型调用。真实GitHub provider审计与自动闭环、邮件/bots完整联调、原REQ整体验收及F6完整release审计继续未完成。此次是仓库管理配置页面切片，不声称原生GitHub审计已可用，未合并main/发布。

提交/推送：c73a1ddeadcbc2b2c9b836172c8165e888c58060已推codex/sidebar-workflow，工作树干净。精确CI37637933151已启动/in_progress：https://github.com/Ed1s0nZ/AIMergeBot/actions/runs/37637933151 。不把启动等同通过，不覆盖取消该run。本地验收tab17已关闭，临时fixture77215已停止，默认视口已恢复；生产环境/真实通知未操作。下一阶段G3实际provider及原完整REQ继续推进。

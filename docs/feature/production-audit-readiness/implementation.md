# 生产审计能力迭代

## F0 / Workflow Gate

用户授权：按上一轮列出的欠缺以最佳实践完成；最终提交main、部署1234；敏感key不得提交GitHub。P10反馈迭代，现有React/Eino/固定Git SHA平台为上游。工作区干净，分支codex/production-audit-readiness。已有阶段文档、源码和验收记录；本轮生产PR样本及真实GitLab验收证据缺失。允许先完成设计及本地可验证实现，不用合成数据替代外部实证。密钥/配置/数据库/私有仓库证据继续仅本机保存。每阶段提交前检查暂存内容。

## F1 / 已授权需求与验收清单

R1 真实历史PR评测：可输入授权Git仓库和固定base/head，标注独立于模型输入；保留失败与覆盖不足，人工判定误报漏报，不冒充生产准确率。
R2 成本：系统设置可配置任务token预算，各主审/分组/汇总/复核/图共享预算；缺失usage明确标记，超限停止后续调用并保留发现。提供报告用量及可配置价格估算，费用不作为实际账单。
R3 大PR：显式记录省略/未完成文件，用户可对固定快照选文件补审；父子审计证据独立、权限配额及去重仍有效；补审不宣称全量安全。
R4 跨仓库：管理员授权关联仓库，固定各仓库版本，只读检索与来源标识；不能扩大现有用户可见权限、跟随子模块或执行源码。
R5 复核模型：独立可配置模型及同模型默认兼容；冻结有效非秘密策略，分别记录用量及分歧，不替代人工复核。
R6 历史关联：重命名/移动关联仅为建议、人工确认，旧复核不自动继承，冲突及不确定可见。
R7 GitLab验收：真实专用测试MR验证Webhook/审计/同评论更新/冲突；没有授权测试对象时先提供可复用验收程序与清晰外部证据缺口，不发送真实评论。
R8 运维：进程托管、备份恢复程序及演练、就绪监控与容量实证；保持单实例SQLite边界，不虚构HA。

整体完成条件：逐项实际代码/流程/自动验证与必要外部证据对照，更新README/CHANGELOG，敏感内容检查通过，main推送及1234新版本验收。未获得真实PR/GitLab证据时保留未完成项，不缩减目标。

## F2/F3 设计与计划状态

正在检查各入口、固定策略、模型调用与分组边界；生产代码尚未修改。首先实现共享模型预算，再按独立切片推进R1/R3/R5与其余能力。每片设计先于代码提交，拒绝把token回调统计误认为硬计费上限；提供商最终usage只能在请求完成后得知，应明确最大单次超额边界。

## F2/F3 / 切片1：共享token停止阈值

设置新增model_budget.max_tokens，默认0保持历史配置兼容，范围0–10000000。冻结到AuditPolicy并参与摘要；Runner/Standalone/设置探测入口使用同一配置。每次Audit/AuditGroups新建上下文预算，子组及汇总/复核/时序图共享，WithTools保留同一预算。Generate前检查、完成后累计提供商prompt/completion用量；达到阈值或用量缺失阻止下一调用，当前已返回内容仍处理、有效发现保留。单请求可能超额，非实际账单硬限额；不支持带预算的流式调用（当前入口全为Generate），明确失败而不绕过。并发同一预算最多一个请求，不能同时抢占余额。失败请求用量未知时保守阻止后续调用。服务端错误仅固定类别，不含凭据。

独立模块model_budget.go及测试，agent/grouped_agent最小接线，设置/策略/入口及React独立字段组件。测试真实SDK本地HTTP与多轮/工具绑定、分组共享/缺失usage/错误/并发及设置兼容。后续成本报告从原始trace合计，缺失不作0；价格估算另一个切片，不把本切片冒充R2全部完成。

## F4/F5 / 切片1实现与检查

共享预算上下文与模型包装器已接线主审/分组/补充阶段；保留底层SDK回调所有权，防止包装器导致模型用量重复记录。设置往返落盘、旧客户端字段缺失保持已有预算、策略摘要变更、真实Eino工具绑定与下一请求停止均已测试。定向race2.243秒通过，cmd/audit-eval测试通过，前端生产构建2.15秒通过；完整Go测试通过（platform20.800秒，evaluation3.285秒）。新增提交前敏感内容检查，不上传私有验收内容。R2尚缺价格估算/成本聚合及跨自动重试预算；R1/R3–R8按清单继续，未完成整体目标，尚未更新main或1234。

## F2/F3 / 切片2：用量与估算

model_budget新增input_price_per_million、output_price_per_million、currency（默认空字符串表示未配置价格；非空仅CNY/USD）。价格0–1000000且有限，冻结进任务策略。详情API返回usage字段，由服务端trace计算，不接受模型结果自填。按primary/verification/diagram/synthesis阶段聚合调用和prompt/completion；无请求、缺失/无效usage或失败请求明确不完整。费用仅按已报告用量和冻结价格估算，未配置价格为null；不完整的数字标为已知部分，不能当完整账单。失败SDK调用补固定错误trace，不写原始错误。前端独立用量面板，配置在模型服务分组；CLI receipt同步该摘要。测试分阶段、零请求、缺失/失败、非有限价格、配置冻结、历史无价格兼容。跨自动重试会另行提供链汇总/执行控制，不能将当前单attempt摘要说成整个重试成本。

## F4/F5 / 切片2实现与验证

详情API新增服务端usage摘要，独立前端面板和设置价格字段；CLI receipt记录同结构。SDK失败回调仅固定错误文本，已验证401/403/404/422单次失败均保留一条未知用量记录且不泄露提供商正文。计价采用冻结策略，不给旧任务套当前价格；无调用/无价格的估算为null。修复CLI取消caseContext后错误判为不完整的时序，先计算完成状态再释放上下文。价格验证、分阶段统计、缺失/负数/零usage、运行中状态、秘密保留与冻结往返均通过。定向race2.162秒、完整Go(platform19.733秒)、vet、CLI及前端构建915ms通过。详情HTTP冻结价格验收单独执行。下一片R1真实PR入口；跨重试与部署/外部实证仍待完成。

## F2/F3 / 切片3：本机真实历史PR语料

沿用audit-eval的私有结果目录、顺序执行、checkpoint/receipt、严格resume与失败保留。在语料case中新增git字段{directory,base_sha,head_sha}，仅本机绝对路径与40/64位完整SHA；禁止与base_files/head_files混用，kind为real-git-history-not-representative-benchmark。Case ID保持中性，期望/rationale/锚点留在私有ground-truth，不传入Agent；模型只收到固定SHA、neutralID和实际Git差异。读取授权本机对象，不联网、不检出、不执行源码、不发送评论。启动case前验证两个对象均为commit；当前不接受branch/tag可移动ref。既有静态语料行为保留；快照使用已有GitRepository只读及输出预算边界。测试真实本机Git提交/内容/错误SHA/路径/混合语料/标签隔离；真实模型评测待用户提供仓库，不能用本地fixture测试替代。

## F4/F5 / 切片3实现与验证

历史PR语料使用本机仓库根目录和完整固定SHA，CLI预先验证所有源对象后才创建产物/调用模型。Git对象读取复用既有保护，不检出/联网/执行源码。真实源和fixture不能混用；ground-truth/output路径含符号链接的真实位置校验阻止标签进入任何源仓库，并拒绝用子目录冒充仓库根来绕过边界。测试验证固定BASE/HEAD读取、脏工作树保留、neutralID/标签隔离、无效ref/缺失commit、混合输入、直接与symlink路径隔离、嵌套源目录拒绝。完整Go(platform19.579秒)与vet通过，新增入口定向检查独立通过；没有真实授权PR质量记录，等待用户提供测试对象。已请求测试项目/MR与验收评论授权，不要求用户发送密钥。

## F2/F3 / 切片4：独立复核模型选择

新增verification_model（同一已配置兼容API中的模型名称，空继承主模型），不引入新凭据/新数据接收方。冻结策略及摘要；Agent仅在复核阶段创建该模型，主审/汇总/时序图保持主模型。复核初始化失败保留发现并标记unavailable，不静默回退主模型。复核trace标真实模型，FindingVerification展示server-owned model值；模型字段不从生成JSON接受。主审和复核共享token阈值。model_budget新增verification_pricing_configured及对应输入/输出单价；不同模型且未配置价格时不估算整个尝试总费用，不能套主模型价格。复核prompt改为configured verification model而非same model，仍明确静态非运行证明。测试模型请求路由、缺省同模型、独立配置/落盘/冻结/预算与计价不混用，前端展示选择与使用模型。外部多提供商端点与新凭据不在本切片，不扩大发送范围。

## F4/F5 / 切片4实现与验证

verification_model可在系统设置配置且落盘、旧客户端缺失字段保留现值，捕获策略并参与digest；复核阶段使用选择的模型与现有端点/凭据，记录server-owned模型名，图/汇总沿用主模型。新增独立复核计价配置，缺失不同模型单价时详情给出明确未估算原因。CLI冻结选择和价格；Standalone补齐已有verify_findings设置接线，行为与工作台一致。真实Eino本地HTTP测试断言主审/复核请求model路由、失败保持原发现、不同模型不绕过共享阈值、无自动推广以及计价不混用。配置往返/旧客户端/冻结、定向race2.872秒、完整Go(platform22.044秒/evaluation3.677秒)、vet和前端构建1.03秒通过。生产UI与真实模型差异质量尚待本轮后续验收，不把合成HTTP验证说成真实效果。R1真实对象仍未提供；其余项按原清单继续。

## F2/F3 / 切片5：固定快照选文件补审

新增详情GET /runs/:id/scope读取原任务固定快照的变更清单/排除/预算省略，不追随最新MR。POST /runs/:id/followup {files:[paths]}仅operator、原任务终态、项目启用及target/source权限均通过时提交；规范路径、去重排序、1–100文件，必须为固定快照真实变更路径，不能绕过原排除策略。任务AuditPolicy中新增followup_of与selected_files作为不可变范围并参与digest，无新数据库列；复制原非秘密策略，保留原BASE/HEAD/source/diff_version；历史无捕获策略任务拒绝并提示重提。重复原任务/同范围复用活动任务，配额仍由EnqueueUser事务检查。Worker只对选择文件建立diff锚点/分组，但Agent可只读检索全仓库作上下文；未选文件明确为范围外，不宣称全PR覆盖。重试保留策略范围与关联，原结果/复核不更改。评论使用独立任务标识且在正文说明补审范围，避免误当全量结论。新增独立页面面板（加载/空/失败/权限/提交/去重/禁用/响应式），选取失败或预算省略文件再审。Scope清单是输入覆盖说明，不把“纳入输入”作已经深入审计。测试ACL/fork/配额/禁用/活动父任务/无效路径/固定版本/去重、worker过滤/排除/跨文件上下文边界/父证据不变。

## F4/F5 / 切片5实现与验证

固定快照scope/followup API、独立前端选文件面板、捕获策略父任务/范围、Worker选择锚点与分组接线、评论范围说明已实现。补审关联独立于自动重试父子字段；无数据库DDL变化。EnqueueUser在事务中再次校验父快照/继承策略/项目启用状态，防止Git读取期间禁用与伪造策略。既有权限与配额仍在事务内执行。单次1–100文件/路径总16KiB；清单5000文件/200说明展示上限明确标缺失。失败/未完成分组可见，纳入输入不等于完成审计。SDK失败/空/无效最终响应与初始checkpoint同时保留排除和选择范围，避免中断丢失限制。

测试：固定BASE/HEAD/diff版本不追随新MR，排序去重/活动复用，父结果不变，排除/未知/遍历/控制字符拒绝，禁用竞态，operator/fork权限，活动/无捕获策略父任务拒绝，冻结策略伪造拒绝，真实Worker锚点过滤及incomplete范围，HTTP创建/去重/验证，现有配额限流与重复不多收费。定向race5.948秒、完整Go20.223秒(platform)、vet及前端构建924ms通过。浏览器实际补审交互留待本轮综合验收；超大单文件差异仍受预算限制，R3的进一步分片处理作为后续子项保留。下一片跨自动重试预算与费用链汇总，其余R4/R6/R7/R8继续。

## F2/F3 / 切片6：跨重试预算与请求持久化

模型请求发送前保存一条pending用量trace，完成/失败原位替换而非追加重复计数；请求前checkpoint失败时取消上下文阻止发送。这样崩溃留下pending记录，不能把未报告账单当0。ToolTrace新增total_tokens，budget累计取provider total与输入输出和较大者。启用预算时同一自动重试链（原尝试加最多2次重试）共用捕获max_tokens；新子任务启动前读取祖先已报告量，seed共享上下文，未知/超阈值停止。FailAndRetry事务检查当前及祖先trace，无余额或未知时不创建新子任务，retry_info.state标model_budget_exhausted/model_usage_unknown，保留结果/checkpoint。Git阶段失败没有模型请求可按既有退避重试。恢复路径同样检查持久化pending/未知，不盲目再请求；未启用预算保持既有重试。详情额外retry_usage链聚合保留各attempt统计/关联，链身份和策略必须一致且有界，不能读取不同项目；每项权限由原任务target/source关系保护。手动重审和选文件补审是明确新任务预算，不自动继承链。测试失败/恢复未知、已知余额、超额、无模型请求、祖先合计、checkpoint前发送防护及不重复统计。


## F4/F5 / 切片6实现与验证

模型请求前持久化pending记录，正常响应和失败原位替换；checkpoint写入失败或租约冲突取消请求上下文，本地真实SDK测试断言没有HTTP发送。记录provider total_tokens并共享重试链阈值；祖先用量未知或额度耗尽时事务内停止自动重试，保留发现。恢复路径使用已持久化记录，不把未知账单当零。重试链限定相同项目/source/MR/固定版本/捕获策略及最多3个attempt，比较实际策略摘要而非只信数据库digest列。禁用预算保持历史自动重试行为。详情保留当前attempt摘要并新增链合计/关联；链校验失败不隐藏原报告，前端持续刷新活动子任务。

验证：完整Go测试通过(platform20.288秒)，vet通过，前端生产构建932ms。最后身份校验补强后，定向race测试1.749秒通过，涵盖已知余额/累计阈值/未知请求/无模型调用/恢复checkpoint保留/跨项目与实际策略篡改拒绝/发送前持久化与写入失败阻止HTTP/旧无预算兼容。尚未完成启用预算的原生SIGKILL恢复演练或浏览器实证，不以SQL租约过期测试代替进程崩溃证明。R1真实样本、R3超大单文件、R4/R6/R7/R8和最终main/部署验收继续保留为未完成项。


## F2/F3 / 切片7：语言无关大文件差异分片

当前分组器按完整文件32KiB限制省略超大文件，Runner又在96KiB全文件输入为空时提前返回。改为对超过分组预算的文本Git diff按hunk/整行切分，重新计算每片旧/新起始行与计数，不截断源码行或伪造锚点。各片仍使用原固定SHA/文件路径与元数据；同文件可出现在多组，各组独立审计后去重发现，补充阶段合并所有Added/Removed锚点而非覆盖。同文件组状态以失败/未完成优先聚合，不能最后一组成功掩盖前一组失败。规范片段本身不算覆盖缺口；损坏hunk、单行超过预算和总预算/分组上限省略必须记录显式缺口。保留32KiB/24文件/8组限制，分组总字节上限提高至8*32KiB（模型token停止阈值和工具/超时预算继续共享），非分组审计接口保持96KiB兼容。超过总上限仍诚实部分覆盖，可选文件补审减少其他文件竞争，不能宣称无限制。

独立diff_chunks.go解析/分片，audit_groups.go规划，grouped_agent.go合并锚点，Runner在检查空scope前建立plan，makeRunScope聚合组状态，README说明边界。测试真实Git生成大单hunk与多hunk、删除/新增/重命名、固定行号等价、跨片锚点联合、UTF-8/无末尾换行标记、超长单行/损坏hunk/总预算与重复文件部分失败。先通过局部Go/race与完整回归，浏览器和真实模型演练继续列为后续综合验收。本切片不执行源码、不增加任何语言专用分析器。


## F4/F5 / 切片7实现与验证

新增语言无关统一差异分片器，hunk计数及整数范围校验后按整行分片，保留原Git行号、固定版本与路径；单行超限和损坏hunk留明确限制。分组总预算256KiB、每组32KiB/24项、最多8组，已有任务token/tool/时间阈值仍共用。Runner先建立plan，修复96KiB单文件scope为空提前返回；同文件锚点在组内和补充阶段均求并集，重复元数据及manifest文件去重。同文件失败/未完成组优先，部分纳入文件独立状态在scope与页面可见。

验证：真实本机Git生成超过96KiB的大单hunk，分片前后新增/删除锚点完全等价且所有片有界；新增/删除/重命名/多hunk/UTF-8/无末尾换行、损坏计数及整数溢出、超长源码行省略但后续行号保持、总预算省略/部分文件状态/早期组失败不被后续成功遮盖均通过。真实Eino本地HTTP加Worker测试确认超大单文件实际到达所有分组，未触发旧提前返回。定向race8.349秒、完整Go(platform22.119秒/evaluation3.967秒)、vet、前端生产构建1.08秒通过。测试使用临时fixture和合成HTTP响应，不是实际模型准确率或生产UI实证。R3基本分片路径已实现；最终浏览器和质量验收以及R4/R6/R7/R8仍按原清单继续。


## F2/F3 / 切片8：授权固定版本跨仓库上下文

管理员为主项目配置最多4个关联的已登记项目及40/64位完整Git提交SHA（仅当前GitLab实例；不接受URL/凭据/分支或模糊ref），保存数据库并通过项目配置同步outbox写入config.yaml。配置是读取授权，不是语义依赖声明；更新只影响新任务。捕获策略新增context_repositories有序固定列表，参与digest，补审/自动重试原样保留；手动关联仓库SHA由管理员选择，Git对象存在性由只读API/native读取验证，错误保留覆盖不足，不追随HEAD。无关联项目的旧客户端/历史任务兼容。

权限按实际冻结的关联项目取交集：提交operator需要各上下文项目viewer；报告详情/列表/复核/取消/历史关联也必须满足关联项目viewer，旧报告不套用新关联配置。运行开始及读取前重新检查关联授权、项目启用与请求者读权限，撤销成员或关联授权取消正在运行任务并保留checkpoint。新增运行依赖表用于SQL列表过滤和撤销联动，每次enqueue/retry与任务原子写入；历史关联限定同一上下文集合/版本，避免通过旧复核reason泄露不同上下文。关联配置的admin写API事务内重检管理员、项目存在/启用和规范SHA，max4/self/duplicate拒绝。只允许直接关联，不递归遍历/跟随子模块或LFS。跨仓库报告阻止自动MR评论：无法证明MR所有读者具有关联仓库权限，必须先保留工作台权限交集，不将跨仓库文本发布到主MR。

Agent新增list_repositories、read_repository_file、search_repository_code、list_repository_directory，接受已冻结project_id，返回服务端所有的repository_id/固定SHA和原有分页/预算边界。关联工具复用现有语言无关只读文本/Git能力，不执行源码或增加语言分析器；原任务发现锚点仍必须是主PR变更，关联仓库观察只能作为上下文与反证，不能凭同名snippet冒充主锚点。独立复核、分组汇总、时序图可读取同一冻结集合，阶段证据编号、共享工具/token/时间预算继续有效。最多4个上下文源，每源已有读取/cache/native仓库字节上限，聚合资源最坏为5份；需在运维容量验收中实际衡量，不虚称单份仓库预算是跨仓库总预算。

实施先交付配置/依赖ACL基础，再接提交冻结/Worker/工具/界面；基础阶段不启动跨仓库内容读取。新增独立context_repositories_store.go及权限模块、HTTP管理员接口、配置类型/同步；随后context_repository_tools.go、Runner准备与实时权限检查、root证据定位和评论隔离、React配置/固定版本说明。测试权限交集、未授权ID/自关联/数量/SHA/旧客户端、事务竞态/撤销、重试依赖保留/列表/历史隔离、同名文件不同源缓存/观察来源/分页/主锚点伪造、真实本机Git固定对象/SDK工具路由、config0600/秘密保留/恢复兼容，完整回归与浏览器验收。尚未获真实跨仓库GitLab对象，不以fixture声称实际跨服务漏洞发现质量。


## F4/F5 / 切片8A权限与配置基础

管理员GET/PUT /projects/:id/context-repositories（{items:[{project_id,sha}]}）与规范化/排序/max4/self/duplicate/完整SHA校验已实现，仅同实例已登记启用项目；清空为items:[]。授权与运行依赖分别保存，Enqueue/Retry依赖原子写入，报告详情/列表/复核/取消按冻结上下文权限交集检查。关联授权版本更新、读取成员撤销和关联项目停用取消未完成任务，HTTP最佳努力中断本地Worker，checkpoint保留。历史发现关联限定相同上下文列表及SHA，避免旧reason绕过权限；评论队列触发器和发送前检查共同阻止跨仓库正文发布，复核不重启blocked评论。

关联配置沿用项目同步outbox写入0600 config.yaml并保留凭据；设置快照深复制关联切片，启动先恢复未同步DB变化，再在全部项目登记后导入本地关联配置。旧配置nil字段不擦除已有授权。关联项目停用可正常启动但拒绝新读取。增量DDL和依赖回填有一次性迁移标记，不在每次启动重复扫描历史策略或恢复已撤销授权。回滚到旧版权限实现必须恢复升级前数据库备份，不能让旧二进制直接服务已有跨仓库结果的DB。

测试覆盖管理员/主项目operator边界、自关联/数量/重复/无效SHA/未知项目拒绝且失败不清除原配置、配置落盘/深复制/秘密保留/0600、旧配置兼容、真实HTTP授权与列表交集、事务依赖/重试继承、读取撤销/更换SHA/停用取消、人工复核不解除评论隔离、历史reason不跨上下文集合、配置替换失败后DB持久化/拒绝旧配置导入/恢复成功、本地Worker中断。定向race15.508秒、完整Go(platform23.944秒/evaluation4.205秒)、vet通过；加入一次性迁移保护后相关测试1.595秒通过，依赖迁移/授权不复活测试另行1.161秒通过。此阶段尚未将管理员列表捕获到生产Submit/Poll或接通Agent读取，不能宣称已具备完整跨仓库审计。8B继续工具/Worker/来源与界面接线，R6/R7/R8和最终main/1234综合验收保留未完成。


## F2/F3 / 切片8B接线细化

Submit及Poll从数据库读取管理员固定关联列表，复制到非秘密策略；enqueue事务仍重检授权对象及各仓库权限，补审/重试沿用原列表。Worker启动后先校验固定集合与请求者权限，使用当前同端点凭据检查commit对象并准备只读上下文源；源不可用保留每个仓库明确限制，不回退最新版本。原生Git按现有origin约束准备固定对象、退出清理，API模式只读固定SHA。工具调用前后重检任务租约/权限/授权及项目状态，缓存不能绕过撤销。

新增四个工具，关联源内部复用现有读取/搜索/目录处理，不重复增加外部工具计数或制造隐藏观察。工具结果新增repository_id（0保持原主PR观察兼容）及repositories枚举；每个关联观察必须返回其实际固定SHA而非主仓库SHA，错误无源文本。分页跟踪按工具+仓库+查询隔离；各阶段共享原有总工具/token/时间阈值，各源缓存独立有界。supported发现必须含主PR锚点观察，关联观察可链接为条件/反证但不冒充该锚点。独立复核的主锚点匹配同样排除关联仓库。时序图SequenceReference新增可选repository_id；服务端在该授权源的固定SHA重新读取并校验snippet，只有主PR引用可匹配风险锚点，关联引用标仓库ID，不能采用目录名/同名snippet证明调用关系。旧图JSON/图查看保留兼容。

独立React管理员配置面板（列表/编辑最多4行/项目选择/完整SHA/保存/清空/失败/禁用/同步待恢复状态）复用既有表单及页面设计；详情显示当前任务实际冻结的关联ID/SHA及仅工作台发布边界。不以当前项目设置替换旧报告版本。测试真实Git双仓库同名文件/不同固定内容/对象读取、SDK四工具schema与来源/主锚点伪造拒绝、分页/共享工具预算/撤销阻止缓存读取、各补充阶段来源传递、关联时序图对象校验与不可替代风险锚点、Submit/Poll冻结及设置变化、实际HTTPWorker路径与源失败保留。继续保持密钥仅忽略配置，完成整体验收前不替换1234进程或合入main。

### 8B实施中的验证记录（尚未完成F4/F5）

Submit/Poll捕获、Worker准备与前后权限校验、四个可选关联工具、独立阶段来源传递、主锚点防冒充、关联时序图固定对象校验及React配置/冻结来源展示已接线。没有配置关联的任务仍保留原14工具，避免改变旧接口工具集合。新增实际Git双源和真实Eino SDK本地HTTP验证，来源身份/同名缓存隔离、权限撤销后缓存不可读、错误提交不可用且不回退最新、关联内容不可代替主审及复核锚点通过。分页补充检查按仓库ID隔离查询，跳页/失败页不能清除缺口。Poll实际循环验证关联项目停用时不标记已处理，恢复后成功冻结关联SHA。

当前完整Go回归通过（platform27.198秒），vet通过，前端生产构建1.33秒通过；权限/来源/时序图定向race9.967秒通过。新增分页与Poll循环检查0.699秒通过。移除未使用的来源判断辅助函数；准备中断提示涵盖授权、可用性和任务超时，不将所有中断误归因权限。剩余8B证明包括上下文原生Git准备清理、实际Worker端到端及独立阶段工具路由、浏览器交互/响应式。尚未完成这一阶段提交，生产1234与main保持上一版；R1外部质量样本、R6/R7/R8及最终综合验收继续待完成。

## F4/F5 / 切片8B代码与自动验证

原生Git上下文准备新增真实本机smart HTTP测试：管理员所选提交早于远端HEAD，读取仍为选定对象，Git配置不保存fetch凭据，退出后临时对象目录移除。实际Worker队列运行Eino SDK关联读取，检查租约/实时授权与数据库持久化来源，确认仅一条对应工具观察；成功状态为平台`succeeded`，不是分组`completed`。独立复核与汇总分别通过真实SDK本地HTTP请求读取关联源，源SHA及阶段标识保留；复核必须另读主锚点，汇总不暴露发现写工具。

最终完整Go测试通过（platform33.003秒），vet通过。原生准备/实际Worker定向race5.625秒通过，独立复核/汇总定向race3.084秒通过；分页/Poll定向race3.380秒通过。前端产物使用本轮1.33秒通过的构建，README补齐配置入口、工具范围、来源与评论隔离、资源及回滚边界。此阶段代码和自动验证完成；浏览器交互、真实跨仓库PR模型质量及最终部署综合验收明确延后至整体验收，不能凭本地fixture视为已完成。下一阶段推进R6历史移动关联建议，R7/R8及真实样本证据仍保留。

## F2/F3 / 切片9历史移动关联建议

Workflow Gate：P10/F2–F3；现有固定SHA、服务端Git元数据、精确指纹、人工复核和权限交集是上游。R6已获用户授权，允许以独立模块和详情面板实现，不改模型或执行源码。现有指纹含路径，因此移动后不能串起历史；Git重命名依据内容相似度（https://git-scm.com/docs/git-diff-index/2.45.3.html），不将检测结果解释为语义同一或风险依然存在。

GET /runs/:id/associations按需获取移动关联建议及最新人工决定，viewer可读。仅终态当前任务、完整固定SHA、同一target/MR/source且冻结上下文集合/SHA完全相同的较早任务参与。使用当前结果中由服务端保存的有效rename元数据，限定普通文本blob（100644/100755），排除copy、symlink、gitlink和无元数据路径猜测。当前HEAD行发现的新路径对应rename.new_path，较早HEAD行发现的旧路径对应rename.old_path；风险类型、精确证据、触发条件必须相同且两端指纹有效。行号和标题可以不同，不能用模糊关键词或语言专用符号匹配扩大建议。每个候选显示旧/新任务、发现ID、路径、HEAD、当前rename BASE→HEAD依据、限制及人工状态；明确当前MR差异的rename不证明旧HEAD→新HEAD祖先关系或语义等价。重复锚点/多候选均显示不确定，不自动挑选。尚在运行时返回空候选及状态说明。

有界读取最近20个同范围较早任务，扫描每项result_json最多1MiB，超过跳过并显示限制；最多50候选，确定性排序，达到任何边界返回truncated/limitations。建议ID由两端run/finding/固定快照与实际源事实摘要生成，客户端不能指定任意来源/路径或冒充候选。无额外网络、Git执行或模型请求；缺少当前服务端rename元数据时明确不支持推断，而非断言没有历史关联。旧报告不会根据当前项目配置变化改写。

PUT /runs/:id/associations/:association_id {decision:pending|confirmed|rejected,reason,expected_revision}由reviewer操作。reason最多1000字符/4000字节，确认/拒绝必须填写理由，pending可撤回决定但旧记录保留。在同一事务重新读取固定报告、候选、两端完整权限和最新revision；expected_revision=0表示没有已有决定，冲突409要求刷新，不能最后写入者静默覆盖。当前finding最多确认一个旧finding，且同一当前任务内同一旧run/finding不能被多个当前finding确认，冲突显式返回。建议缺失、结果变更、无权限或候选摘要不一致拒绝，不支持任意手工拼接无依据任务。管理员亦遵守冲突/候选验证。

新增platform_finding_association_history追加表保存current_run/current_finding/prior_run/prior_finding/association_id/decision/reason/actor/time；revision为历史自增ID，最新记录为当前态，最多显示20条决定历史。外键、pair索引及查询范围保证只涉及当前两端，不建立传递闭包或自动关联其他任务。确认只是人工历史关联，不改变Finding.Fingerprint、confidence、severity、人工Review或comment body，也不自动复用“fixed/false_positive”。事件日志单独记录关联操作；没有对外评论发送或评论重试。权限撤销后不展示旧理由或关联内容，写入事务再次检查防止读取后权限变化。历史精确指纹面板仍保留，与人工移动关联分开展示，避免把关系确认当风险复核。

实施文件：独立finding_associations.go（候选纯逻辑）、finding_associations_store.go（有界读取/追加决定与迁移）、http_finding_associations.go（契约/权限）；store.go/routes.go窄接线。独立React finding-associations.tsx/css按需加载、空/运行中/限制/错误/刷新/查看两端/填写理由/确认/拒绝/撤回/冲突/只读/响应式状态，使用服务端API类型，不扩大workspace.css。详情当前can_review控制操作，服务端仍是权限权威。README说明精确历史与人工关联区别，CHANGELOG最终合并记录。

验证：真实Git rename/move元数据、路径变化+精确证据匹配、类型/trigger/证据变化不匹配、copy/symlink/gitlink/BASE引用拒绝；重复锚点/多候选和20任务/1MiB/50候选边界明确；target/source/MR/context跨范围无泄漏；确认/拒绝/撤回追加历史、理由/revision冲突、一对一冲突、无有效候选/伪造ID拒绝、权限撤销事务重检、旧Review/Fingerprint/评论队列不变；真实HTTP契约及React构建，完整Go/race/vet。浏览器综合验收保留本轮最终阶段，不把fixture当真实模型效果。新增表向后兼容；旧二进制不会显示人工关联，但跨仓库权限回滚仍需按8A恢复升级前DB。本片不声称追踪所有任意多次移动，未纳入/缺少元数据由限制说明承载。

### 切片9实现中的记录

候选纯逻辑、追加决定表/迁移、有界读取与GET/PUT接口已接线。使用事务内两端权限重检、候选重算、expected_revision和最新确认的一对一冲突检查；不修改发现指纹、风险复核或评论。移除所有权限后沿用现有404隐藏资源边界，不要求返回403。无效决定独立400错误类别。元数据按新路径索引、旧发现按旧路径索引，不做全元数据笛卡尔扫描。

定向race6.719秒通过：精确源事实/路径提示、多候选不自动选、scope/type/evidence/trigger/BASE/copy/特殊对象不匹配、决定追加/撤回/旧复核不继承/原发现不改、stale revision/伪造ID/空理由拒绝、viewer写拒绝及成员撤销后读拒绝、两个并发确认仅一成功、撤回后另一个选择可确认、最近20任务/每结果1MiB/决定20条限制可见。vet通过。剩余：真实Git移动fixture、跨仓库完整权限与HTTP契约、50候选上限及重复锚点、评论不变证明、React交互/构建和完整回归。本片代码尚未阶段提交，main与1234未更新。

## F4/F5 / 切片9实现与自动验证

按需加载的React历史移动关联面板已接线，展示两端固定路径/SHA、源事实、当前决定和追加历史；有界/空/运行中/加载/只读/失败/刷新/理由/确认拒绝撤回/冲突保留理由及响应式布局均有对应状态。真实Git移动且增添相同源码行的fixture确认metadata来自固定原生Git差异，当前引用确为实际新增HEAD锚点，两端证据均可固定对象重读。HTTP跨仓库权限验证读写交集、400无效理由、409陈旧版本/伪造候选，以及上下文权限撤销后404隐藏报告且不泄露私有决定。

增加50候选上限、反向多候选（一个旧问题被多个当前问题匹配）不确定标记、原始重复锚点被清除指纹后省略说明；候选ID同时包含冻结上下文集合，相关来源变更不会复用旧决定。确认/撤回前后实际评论投递记录完全相同，当前风险Review未继承、Fingerprint未修改。最近20任务/1MiB输入/20历史决定边界测试保留最新状态且限制可见，两个并发确认仅一个成功。

完整Go测试通过（platform55.654秒/evaluation14.783秒，含并行race运行），vet通过，定向race15.676秒通过，最终前端构建2.43秒通过。末尾补充指纹省略说明和评论不变断言后相关测试1.338秒通过。README更新操作与限制。浏览器实际交互/视觉验收仍留在最终综合验收，尚不宣称已完成整个R1–R8目标；R7/R8、真实PR/GitLab证据、main及1234部署继续未完成。本片新增表仅追加，不更改旧发现、复核或评论语义。

## F2/F3 / 切片10A私有备份恢复工具

Workflow Gate：P10运维迭代；已有SQLite WAL、实例租约、固定config.yaml、启动项目同步outbox和原生恢复演练是上游。R8已授权，本片补齐可复用私有恢复包，不调整当前1234服务或假装已完成整个运维。依据SQLite官方Backup API（https://www.sqlite.org/backup.html）使用Python标准库sqlite3.Connection.backup，不直接复制活动主DB/WAL文件或用未验证文件副本声称一致。数据库API能提供一致DB快照，但跨config/DB的一致性还需要停止应用写入。

scripts/ops-backup.py提供backup、verify、restore三个子命令。backup显式参数--database、--config、--output、--service-stopped，可选--binary。必须由操作者先停止进程托管及所有使用同一配置/DB的实例/写入者，再声明service-stopped；工具拒绝仍有有效platform_worker_instance租约的源（崩溃后等待最多30秒过期）。该声明和租约检查不证明其他外部进程已停止，不宣称提供全机进程扫描或在线跨文件快照。配置读取前后检查内容hash，任何变化则失败，不接受半途中变更；未来托管阶段提供正确停止顺序。

源必须为明确的普通文件，拒绝文件symlink与缺失来源；不得将输出放在源DB/配置/二进制的父目录内部或覆盖其目录。输出目录必须不存在，用mkdir独占创建0700，只在自建目录写入固定文件名pr_agent.db、config.yaml及可选aimangebot，禁止manifest指定任意路径。DB/配置0600、二进制0700、manifest.json0600。SQLite源以只读mode=ro连接，备份目标使用独立连接；备份完成执行PRAGMA integrity_check，结果必须唯一ok。超时/读取/损坏/租约/配置变化失败清理本次自建目录，不删除任何预先存在目标。完整manifest最后写入且flush/fsync，此前恢复包不被视为可用；不声称整个目录发布是跨文件原子事务。

manifest格式version=1、created_at、files映射固定文件名→{sha256,size}，不保存配置明文、凭据、原始路径、DB表内容或命令输出。verify拒绝缺文件、额外manifest文件项/未知字段版本/不合规hash与size/文件symlink及hash不符；校验DB integrity_check和私有权限。哈希证明内容一致，不证明外部包来源真实性；只恢复受信任的本地私有备份。输出仅状态/文件数量/结果类别，不打印配置、DB记录或可能含密钥的错误正文。

restore先完整verify，再写到不存在的新目录，目标同样独占0700，复制固定文件名，设置正确权限并复核hash/DB完整性后完成。不覆盖已存在目录，不执行备份二进制，不自动启动服务、不自动修改监听地址或清除任务/评论/租约。原服务和原目录保持原样，操作者在停服务窗口按同版本二进制迁移后切换；回滚须数据库+配置+兼容二进制成套恢复，旧跨仓库权限实现不能直接读取新跨仓库结果。源备份保护为本机私有文件，禁止放入GitHub/公共证明目录。

实施：纯标准库独立脚本与unittest（WAL中未checkpoint记录仍被正确备份、有效租约拒绝、缺失/损坏/hash篡改/symlink/权限/未知manifest项、非空或存在目标不覆盖、错误清理、源内容不变、0600/0700、配置秘密不出现在日志），文档记录准确命令及限制。随后10B原生二进制恢复演练，在隔离合成上游/临时配置/私有DB上停止→backup→restore→相同二进制启动，核对账号会话/ACL/固定任务/checkpoint/人工复核/关联历史/评论状态及健康就绪。当前生产不执行破坏性恢复，真实运行期备份安排在最终已授权切换窗口。后续10C进程托管（本机launchd可验证，Linux systemd模板不冒充已部署）、就绪监控、容量及token崩溃证明仍单独完成。

## F4/F5 / 切片10A实现与验证

ops-backup.py的backup/verify/restore三个命令使用Python标准库，源SQLite只读且API备份完整WAL快照，目标以独立DELETE日志形式落盘并校验。配置变更、有效实例租约、非私有配置、symlink、输出在源目录及已有目标均拒绝；独占新建目录失败仅清理自建路径，不覆盖原部署。校验固定文件集合/manifest版本/大小及hash/私有权限/DB integrity；恢复复制后二次检查，不执行二进制。输出固定摘要，无源路径或配置内容，目录0700、数据0600、可选二进制0700。

9项unittest通过（0.119秒）：未checkpoint WAL数据恢复、源主DB/config保持、权限/文件集合、租约与停写声明拒绝、不覆盖已有备份/恢复目录、symlink/非私有配置/源内部输出拒绝、hash篡改/未知manifest路径/缺失文件、即使匹配hash也拒绝损坏DB、配置变化与复制失败清理、截止时间及私有包目录、实际CLI成功和失败日志不暴露fixture秘密/路径。测试连接显式关闭，避免仅提交却保持SQLite文件句柄；Python缓存加入gitignore。README链接docs/operations.md，明确停止写入前提、命令、恢复流程、来源信任及验证边界。此次未更改生产1234，公共healthz实际返回ok。

本片只完成可复用备份工具；10B原生二进制业务恢复、10C托管/就绪/容量及预算崩溃证明仍未完成，R7真实MR授权与R1历史质量证据仍缺少。最终main/1234发布与浏览器综合验收按原清单保留，不能用这9项文件级测试代替整个R8验收。

## F2/F3 / 切片10B原生恢复演练

复用已有smoke-worker-recovery隔离真实二进制/合成上游/临时目录，新增--backup-preview，要求metadata/lifecycle/comment场景同时启用，保留既有SIGKILL与真实租约等待证明。独立ops_restore_drill.py协作模块承载恢复步骤，旧脚本仅参数和调用接线，保持文件小于800行。先通过真实HTTP创建受限成员、赋予项目viewer及第二个未授权项目，验证其能查看已授权历史、看不到未授权项目；独立CookieJar保存会话。成员身份/会话必须在恢复后直接使用，不能重新登录掩盖会话丢失。

关联历史使用明确标记的合成持久化报告fixture：复制已完成历史任务的固定策略/发现，构造普通文件移动元数据与新路径，按现有指纹契约计算候选源指纹并对原记录计算值作交叉校验。该合成报告仅用于恢复数据，不作为实际Git差异、Agent质量或真实PR证明。由真实GET/PUT关联接口创建人工确认记录，再验证关联不会改变评论。现有真实Worker完成的metadata/生命周期任务、SIGKILL父检查点、人工风险复核、评论单次创建/更新记录都纳入恢复检查。

停掉所有本演练的应用进程并等待退出，关闭SQLite测试连接，确认公共健康不可用。通过实际ops-backup.py CLI对临时DB/config创建并验证恢复包，再通过restore写到另一新目录；备份父目录独立于源目录，0700，本片不要求可选二进制入包，使用同一已验证构建二进制启动恢复目录。源与恢复包均保留本机私有，不上传内容。以逻辑业务表有序hash核对任务固定版本/result/trace/策略、用户/会话/ACL、人工Review/追加历史、关联历史、评论状态；Worker实例owner/lease必然变化不作相同断言。恢复后同一admin/member会话访问API，任务、检查点、人工风险复核及关联决定内容必须相同，受限成员ACL仍有效。确认合成上游模型/评论调用计数不增加，避免恢复重复对外操作。原目录保留且不再启动。

恢复进程纳入既有finally清理，证明JSON只输出断言布尔/数量与合成场景，文件0600，目录0700，不打印凭据或DB内容。演练仍约需真实租约等待40秒，等待工具必须使用原句柄并保持状态更新。命令为go build到任务自建临时目录，再运行smoke-worker-recovery.py --backup-preview --metadata-preview --lifecycle-preview --comment-preview（独立复核可加--verification-preview）；禁止生产1234/8080和真实GitLab/模型。当前1234不停止。本片完成后继续10C托管/就绪/容量及R7，不能以这次隔离恢复宣称实际生产迁移已验收。


## F4/F5 / 切片10B原生恢复实证

新增ops_restore_drill.py，接入smoke-worker-recovery --backup-preview；原生HTTP创建成员/ACL/会话及人工关联记录，真实CLI备份/校验/恢复与同一二进制启动，核对业务表hash、报告/检查点、人工Review、关联决定、既有会话权限和对外操作计数。恢复进程纳入finally清理，配置与证明0600，目录0700。修复演练生命周期合成模型按请求总次数响应的问题：依据独立复核/时序图阶段与新鲜tool observation返回各自协议；无真实漏洞的生命周期合成文本由复核fixture拒绝，metadata候选仍支持条件性说明。第一轮因fixture错误未完成，修正后完整重跑，不修改应用逻辑迁就fixture。

2026-10-05原生构建SHA256 036f311a486a61b310f4eff2545e2242ce71d6131d86cdc0f1d1ffb84af8c613，在19236/19237隔离端口执行metadata/lifecycle/comment/verification/backup全部场景，进程退出0。实际SIGKILL后等待原租约、检查点保持、恢复子任务成功；MR92模型5次、MR94模型9次；备份CLI三步成功、恢复前后9张业务表hash一致，admin/member原Cookie会话与viewer ACL保留，风险复核和关联追加历史保留，模型与评论请求零新增。9项备份单元测试再次通过0.119秒。私有证明位于本机临时目录，不上传源DB/config/包。1234生产未调整。

合成持久化移动报告仅证明恢复历史保存，不证明实际Git/Agent质量。R8尚需10C托管/就绪/容量，R2启用预算崩溃及R7实际GitLab验收、R1真实样本、浏览器综合验证、最终main和部署继续保留未完成。

## F2/F3 / 切片10C1租约就绪检查

Workflow Gate：P10运行可靠性，现有HTTP健康接口、实例租约/心跳与原生恢复证明为上游，允许实施。独立GET /readyz匿名只返回ready/unavailable，不包含owner、路径、仓库、队列或配置；兼容现有healthz。就绪需要Store可查询、Runner已启动且其上下文未停止，并在同一次有界SQL读取中验证当前实例owner相同且租约未过期。查询使用请求派生2秒截止，不调用远端模型/GitLab；部署可工作但外部服务断开时仍不虚称上游可用。owner加入不可变workerRunState，避免HTTP访问可变Runner.owner产生竞态。查询之后仍可能失租，任务写入继续由既有SQLfence保护，ready不是永久授权。

测试覆盖Runner未启动、活跃正确owner、过期、被其他owner替换、ctx取消、DB关闭和无私有信息响应，race检查；后续托管/监控实际使用readyz。本片不替代进程托管和容量证明；其余R1–R8与最终main/1234验收继续有效。

## F4/F5 / 切片10C1就绪实现与检查

新增readiness.go和readiness_test.go，注册匿名GET /readyz；返回固定ready/unavailable且no-store，当前workerRunState保存不可变owner，SQL验证实例有效归属，使用请求派生2秒ctx，不调用外部服务、不访问配置秘密。healthz保留。测试未启动、缺租约、正确归属、过期、owner替换、Worker取消、请求已取消、DB关闭、nil依赖以及固定无敏感响应。定向race最终1.638秒通过；完整go test ./...通过（platform34.883秒，evaluation3.903秒），go vet ./...通过。文档明确旧生产尚无新接口，最终发布后才生效。本片尚无原生就绪/launchd托管实证或容量结果，继续10C2与其余验收。

## F2/F3 / 切片10C2服务配置生成

P10运维反馈迭代，上游二进制、配置启动目录、租约与readyz契约已就绪。生成工具仅创建不存在的私有输出文件，不安装/卸载服务，不读取或复制配置内容。launchd plist采用绝对ProgramArguments/WorkingDirectory、RunAtLoad、KeepAlive.SuccessfulExit=false、ThrottleInterval=35（大于30秒租约）、Umask=077和私有目录日志。systemd模板采用指定非root用户/组、绝对ExecStart/WorkingDirectory、Restart=on-failure/RestartSec=35、TimeoutStopSec=45、UMask=0077和NoNewPrivileges。路径与标识限制明确，systemd路径需转义百分号/引号/反斜杠，禁止控制字符；不提供含凭据环境变量。已有DB部署不需要bootstrap凭据。操作文档解释新库需先私下bootstrap，退出0不重启，异常才自动重启，备份前必须bootout/stop禁止自动重启。本片先生成/验证，下一片隔离launchd真实异常重启与readyz proof，Linux配置不虚称已运行验证。

## F4/F5 / 切片10C2服务定义工具

实现纯标准库ops-service.py生成launchd/systemd定义；绝对路径、控制字符和标签验证，systemd非root用户/组及语法转义，私有输出且不覆盖，失败清理只限自己创建文件。不读取/内嵌秘密、不安装/启动服务。4项unittest通过：plist解析/异常重启与租约间隔、systemd空格/百分号/引号转义、非法身份/换行拒绝、0600/0700和已有文件保持。operations记录生成、安装/停止、权限、bootstrap、就绪、备份卸载和用户登录域限制。此为配置生成器验证；本机launchd异常恢复实证与容量仍待下一片，未停止1234或声称已托管。

## F2/F3 / 切片10C3隔离launchd实证

本机gui/501域实际可访问。新增原生演练脚本，要求绝对测试二进制、非生产19240端口；创建0700临时工作目录/0600合成JSON配置，远端固定本机关闭端口且poll/webhook/comment关闭。先前台启动用合成账号bootstrap，ready后SIGTERM正常退出；再使用ops-service生成随机任务标签plist，launchctl bootstrap。记录实际pid/SQL有效owner，SIGKILL该专属服务PID，等待35秒重启间隔和原租约到期，验证新PID/新owner/readyz、账号可登录且DB保留。最后bootout且确认服务不存在/端口关闭，证明JSON0600只存状态与计数，不存会话/配置内容。失败也卸载自己标签，不触及production。不能以管理器配置存在替代业务验证；脚本失败保留私有日志调查。Linux实证与容量仍单独验收。

## F4/F5 / 切片10C3原生launchd验收

新增smoke-launchd.py，实际合成配置bootstrap→正常退出→生成随机标签→launchctl bootstrap→readyz/SQL owner→SIGKILL专属PID→新PID/新有效owner/readyz→原账号登录→bootout，不读生产配置/不调用远端。失败finally只卸载自己标签，工作目录0700、config/log/证明0600。原生二进制包含readyz代码，在19240首轮全部通过35.55秒（该版耗时包含后续登录/卸载）；调整计时位置后19241复验全部通过，重启观察35.29秒、退出0。19240立即重跑曾因TCP端口仍占用被前置bind拒绝，未创建新服务；文档明确选空闲端口。服务已卸载，1234实际healthz保持ok。4项服务生成器测试再次通过，py_compile/diff检查通过。证明只保留私有本机，Linux未实测。继续容量/启用预算SIGKILL、R7、真实质量、UI和最终main/1234交付，不能以本机托管演练代替全部R8。

## F2/F3 / 切片10C4原生容量边界

复用smoke-worker-recovery新增独立--capacity-preview，在完成原租约SIGKILL基础实证后进行真实HTTP并发提交。合成GitLab支持独立MR100–149固定版本；这些合成模型请求由Event阻塞，确保观察窗口不被快速完成掩盖。global outstanding=4，project/user running=1，已有rate-limit pending占1；40个不同MR并发提交应仅3个创建、37个429，DB pending/running总数不超过4且running最多1。重复已接受MR应复用原任务，不因队列已满再次占配额；取消一个已接受任务后另一MR可入队。finally释放模型Event并清理已有fixture进程。结果仅是本机固定4容量边界，不宣称生产吞吐量或硬件上限；额度状态不泄漏其他用户/项目记录。其他preview组合不得以额外模型调用破坏基础证明计数，容量在现有断言之后独立执行。

## F4/F5 / 切片10C4容量边界原生验收

新增ops_capacity_drill.py，接入--capacity-preview与独立MR100–149阻塞合成模型。真实HTTP Barrier40并发，原生Worker/SQLite/global4/project-user运行1。首轮全部容量断言通过但清理重复取消已取消任务返回409；修正清理跳过已取消ID后，在19244/19245完整重跑SIGKILL真实租约恢复与容量流程退出0：40提交仅3新建/37明确quota429，加原pending总计4；一秒采样running=1、outstanding=4，满额duplicate复用、不新增；取消pending释放后MR149创建成功，总数仍4，清理所有新增任务。HTTP提交p95观察0.076秒，仅本机fixture值，不宣称真实模型吞吐量/绝对性能。相关Quota race测试5.763秒通过，py_compile/diff检查通过。私有proof/config/DB未上传；1234保持不变。R8实际生产监控/最终切换、R2预算崩溃、R7真实GitLab、R1质量与UI/main验收仍需完成。

## F2/F3 / 切片10C5启用预算原生崩溃

新增互斥--budget-crash-preview，复用固定合成MR91/真实二进制/临时DB，max_tokens=100，首轮真实SDK返回usage20并读取文件，第二模型请求等待响应。确认checkpoint中已知20和发送前pending usage未知记录后SIGKILL；立即新进程不得取得有效租约，等待原SQL租约自然到期再启动。必须保留相同trace/result、将父任务failed且retry_info.state=model_usage_unknown、不生成子任务、上游请求仍2（不重发未知消耗），原预算仍100。该策略停止自动恢复以避免未知请求重新获得预算，不声称未收到响应的消耗为0。finally清理全部自己的进程，私有证明0600。其他已知余额/阈值/跨retry的验证由现有race测试补充；本片补足实际SIGKILL边界，不执行真实模型付费请求。

## F4/F5 / 切片10C5预算SIGKILL实证

新增ops_budget_crash_drill.py与互斥--budget-crash-preview，MR91合成首轮响应补真实SDK usage字段20，预算100。19246/19247使用已构建真实二进制执行退出0：已知20+第二请求pending未知持久化、read_file证据存在→SIGKILL→立即重启有效租约拒绝→原lease自然过期→新进程就绪。trace/result逐字一致、冻结预算100不变、父failed状态model_usage_unknown、retry子任务0、模型HTTP累计2未重发。相关model/retry/request定向race1.716秒通过，py_compile/diff检查通过；证明私有0600，不包含真实凭据。该实证补足R2启用预算的进程崩溃边界，其他已知余额/超阈值语义仍由现有SDK/Store测试覆盖，不声称真实服务计费为20。当前1234未调整；继续R7真实GitLab验收工具、实际质量、UI及最终main/部署。

## F2/F3 / 切片11A GitLab只读验收凭据

R7先提供只读采证脚本：明确app/gitlab地址、run/project/MR、expected full HEAD SHA；凭据只从AIM_ACCEPT_SESSION/AIM_ACCEPT_GITLAB_TOKEN环境读取，不进入命令参数/证明。禁止HTTP重定向携带凭据，远端要求HTTPS，本机HTTP例外。读取readyz、完整指定run和GitLab MR/指定discussion；检查固定快照、当前MR HEAD、comment sent generation收敛、discussion/note一致。私有证明只保存run/version/评论标识、generation及body SHA256，不复制源码/评论/用户数据，0600不覆盖。脚本不写GitLab、不触发审计，可在授权的写入测试前后采证；这不是Webhook/同评论更新/冲突端到端全部完成，下一片写入验收只能对明确授权专用MR执行。

## F4/F5 / 切片11A只读GitLab收据工具

新增gitlab-acceptance-receipt.py，只GET readyz/run/MR/discussion，明确目标ID与full SHA、结果terminal、MR当前SHA和评论generation/标识对应。环境变量读取凭据，禁止跨重定向传递，HTTPS远端/本机HTTP限制，2MiB响应上限/10秒超时；只保存私有0600摘要，不覆盖。3项单元测试通过：正确收据不复制正文、任务/MR漂移/未完成/评论未收敛拒绝、地址传输规则。py_compile/diff检查通过。未实际连接用户GitLab/发送评论，完整Webhook/审计/更新/冲突及当前真实对象授权仍待11B，不能将该工具标记整个R7已完成。

## F2/F3 / 切片10C6单次就绪监控

P10运维迭代，readyz契约与原生launchd实证为上游，允许窄范围实现。纯标准库ops-healthcheck.py每次只GET明确endpoint/readyz，HTTPS远端/本机HTTP、不含凭据URL、不跟重定向，响应上限1KiB、超时1–10秒。仅HTTP200且精确JSON status ready算成功，退出0；其他状态/断连/超时/畸形/超限退出1并固定unavailable JSON，不输出URL/异常/正文。不会重启服务或写外部消息，适合现有监控定时调用；生产最终切换后验证真实1234。测试实际本机HTTP成功、503、302、非ready及超限，不把worker就绪解释为远端模型可用。

## F4/F5 / 切片10C6监控实现

实现ops-healthcheck.py单次只读检查与test_ops_healthcheck.py真实本机HTTP测试。200精确ready成功；503、302（未跟随）、非ready、畸形/超限、服务关闭拒绝，非法远端HTTP/凭据/路径/查询与超时范围拒绝。HTTPError响应显式close，ResourceWarning设为error复验通过2项0.56秒，防止周期监控遗留句柄。固定输出/退出码，不读取私有配置，不启动定时任务/写外部消息。operations记录运行及旧版本限制，最终部署后还需实际1234监控证明。

## F2/F3 / 切片12A真实界面排版修复

隔离原生实例19248/19249已完成metadata/lifecycle/comment/verification基础场景，真实浏览器登录并检查设置截图。发现独立复核字段和预算fieldset错误嵌入温度/轮次field-row，触发横向flex压缩成竖列。修复行容器的闭合位置，将独立复核与预算作为fields直接子级，利用既有fieldset全宽规则，不改业务API/值。先build/typecheck，再真实浏览器重验模型布局；此问题不能靠Go或TS成功证明视觉正确。其他跨仓库/历史关联/补审响应式与最终main仍需综合验收。

## F4 / 切片12A设置布局修正

真实19248实例界面已确认缺陷，修正settings.tsx field-row闭合位置并重新生产构建，TypeScript/构建通过1.42秒；新的JS index-H2kC4Rcz替换旧资源。此前整Go回归（缓存结果）、18项Python测试0.899秒及前端typecheck通过。首次npm build误在仓库根执行缺package.json，随后按frontend目录执行成功。真实修复后截图/交互尚未验收，不以构建成功标记F5；旧隔离进程已请求正常停止，下一步使用包含新静态资源的二进制继续实际界面验证。生产1234未替换，真实MR授权仍未收到。

## F5 / 切片12A实际浏览器复验

以aa1f817干净源码重新构建真实二进制至私有/tmp，19250/19251完整metadata/lifecycle/comment/verification原生fixture再次通过，保留UI实例供后续验收。浏览器实际登录合成账号并查看新资源：默认1280视口温度/轮次双列、复核模型独立全宽，预算fieldset不再挤压其他字段；390×844窄屏截图字段正常，DOM scrollWidth=390=innerWidth无横向溢出，随后reset视口。私有截图settings-fixed.jpg保存在本机fixture目录0600，未上传。任务5界面实际显示固定BASE/HEAD、评论同步generation2/2、5次usage未知、分阶段主审/复核/图、独立复核支持/静态限制、元数据note时序图与人工误报状态；展开历史移动关联显示无候选和明确限制。这里只完成设置布局及上述报告显示，尚未操作关联confirm/reject/撤销、跨仓库配置、补审或真实质量样本，保留这些综合验收项。生产1234未变，main未合并。

## F2/F3 / 切片12B补审原生成功fixture

实际浏览器任务5→固定清单entry.any→选择1→按钮启用→新任务9，BASE/HEAD与选择范围正确，原报告保留。但MR92合成响应依赖全局调用序号，第二审计首请求错误返回diagram JSON，真实SDK严格校验报unknown_field失败。不是补审创建失败，不能据此宣称成功审计。改fixture按system阶段/当前tool observation区分主审、独立复核、图，新增--followup-preview完整API成功验证，保留原错误响应拒绝行为。验证父结果/reviews与新任务独立、固定两端/selected_files；此为合成成功流程，不替代真实模型质量。

## F4/F5 / 切片12B补审流程实证

MR92合成模型改按system阶段与本轮tool observation响应，支持多次独立审计，新增--followup-preview。第一轮修正fixture后发现补审status=incomplete且有有效发现/复核/时序图，原“成功必须succeeded”断言不符合选择范围契约。修正断言为终态incomplete+1有效发现+显式范围说明，未改变应用状态来迎合测试。19254/19255完整原生重跑退出0，MR92累计10模型HTTP，任务6固定父5 BASE/HEAD、entry.any selection、父结果/reviews保持、子review空。Followup race6.440秒通过，py_compile/diff检查通过。真实浏览器已验证0选择禁用、1选择启用、创建任务/路由和错误响应拒绝；修复后的成功流程还需浏览器重验。未证明真实质量，main/1234未替换，截图和DB仅本机私有。

## F5 / 切片12C历史关联真实交互

在隔离19250实例使用seed_move_report由历史6构造明确标记的移动报告11；此fixture不证明Git/Agent质量。真实浏览器展开候选6/7，明确多候选不自动选择；填写理由确认6后显示已确认与风险独立。再确认7触发真实API冲突，界面刷新并保留理由；撤回6后待确认且历史保留；拒绝7后已拒绝并展开操作者/时间/理由历史。SQL核对追加序列confirmed→pending→rejected，当前11风险review零，旧6仍fixed。私有截图association-decisions.jpg 0600，不上传。完成确认/冲突/撤回/拒绝交互；权限只读、跨仓库与补审成功浏览器以及最终main/1234继续待验收。

## F2/F3 / 切片12D路由旧面板残留

真实浏览器由任务9切11再切projects，DOM region计数曾有两个选文件补审，项目页仍显示旧补审；不是AX缓存推断，Playwright实际DOM确认。源码同级FollowupAuditPanel和FindingAssociationsPanel都key=r.id，发生同级key冲突，生产React日志不输出警告。改为followup:ID/associations:ID分别唯一，保留任务变更时重置state语义。构建后重验连续任务/项目切换DOM计数与页面截图，不仅刷新页面掩盖残留。

## F4 / 切片12D唯一key修复

将同级补审和关联组件key改为分别带类型前缀的任务ID，既消除冲突又保留换任务重置。生产构建/TS通过1.22秒，资源更新index-DgxiO4_H；实际修复后路由重验仍待包含新资源的二进制，不将构建成功当作DOM残留已解决。新19256/19257旧资源fixture已完成固定补审原生API证明并保留UI，当前浏览器30仍在旧19250项目页；修复前截图私有0600。下一步新资源实际验收与其余跨仓库/真实质量/GitLab/main/部署继续。

## F5 / 切片12D路由清理及补审界面实证

从c789432干净源码构建真实二进制，19258/19259恢复+元数据+独立复核+补审fixture退出证明正确，UI进程继续用于验收。关闭此前自己19250/19256演练，原句柄均exit0，未改生产。浏览器实际登录，连续任务5→任务6→项目（未刷新掩盖）：真实DOM选文件补审region计数1→1→0，项目heading1，旧面板消失；任务6coverage不完整，展开固定清单显示原任务#5补审和entry.any，不声称全量完成。私有followup-validated.jpg/route-clean.jpg 0600留本机。此实证完成12D修复后验证及12B成功结果显示。生产仍旧二进制，无readyz，ops-healthcheck如预期unavailable，不解释为新发布成功。跨仓库界面/实际质量/GitLab授权/最终main与1234继续未完成。

## F5 / 切片12E跨仓库配置界面及持久化

隔离19258真实浏览器项目1无可关联项目时添加按钮禁用；通过界面登记合成项目2后可添加，选择2、输入invalid提交被完整40/64位SHA校验拒绝且保留输入。改为明确合成40个b保存成功，整页reload后重新展开仍为项目2及同一SHA。只读SQLite核对platform_context_repositories精确(1,2,合成SHA)，仅断言私有config.yaml包含预期SHA，不输出其他配置；系统Python无PyYAML，因此未声称完整YAML解析验证。截图context-saved.jpg本机0600。随后实际移除行并保存，界面显示空关联与同步成功，SQLite计数0且config.yaml不再含该SHA。此合成项目配置验收不代表该SHA存在于真实GitLab或Agent跨仓库质量，不改变生产1234。真实质量样本、权限界面、GitLab授权与最终main/部署仍待完成。

## F2/F3 / 切片11A2采证HTTP异常资源

P10窄范围验收工具维护，已有11A只读GET/凭据隔离/响应上限为上游，允许实现，无新增产品决策。检查发现get_json成功响应用with关闭，但HTTPError（含禁止跟随的302）不进入with，需显式关闭错误响应后重新抛出；保持统一main错误输出，不打印正文/凭据。新增实际本机HTTP测试覆盖成功、302只发一次、503、非法JSON与2MiB超限；直接断言HTTPError响应closed，并以ResourceWarning为error复验。仅涉及脚本和测试，不访问真实GitLab、不发送评论，不将此项算完整R7。

## F4/F5 / 切片11A2采证错误响应关闭

get_json对HTTPError显式close后原样抛出，main继续固定错误摘要。真实本机HTTP覆盖正确JSON、302未跟随且错误响应closed、503响应closed、非法JSON与2MiB超限。4项receipt测试以ResourceWarning=error执行0.538秒通过，diff检查通过。测试凭据明确synthetic-fixture-token，仅传本机测试服务器；未访问用户GitLab。保持只读收据范围，R7端到端授权验收仍待完成。

## F5 / 切片12F模型预算与独立价格界面持久化

隔离19258实际浏览器设置页：API Key留空，复核模型synthetic-independent-review、token阈值12000、CNY、主输入/输出2.5/5、启用独立复核价格3/6。复核价格从disabled切为可编辑，保存显示成功且无未保存更改，整页reload八个字段全部保留。仅对私有config.yaml上述八个键/值作精确行断言，不输出其他配置，全部通过。私有settings-persisted.jpg 0600。未调用该合成模型、不声称外部模型支持/实际计费验证，生产1234真实配置不变；设置保存/刷新/落盘界面验收完成，真实质量与GitLab以及最终main/部署仍保留。

## F6 / 发布证据清单（准备，未通过）

新增release-readiness.md按原R1–R8逐项区分实际已取得证据与缺口，包含真实质量/权限UI/GitLab授权/最终回归/main/私有备份/1234部署检查。CHANGELOG增加“未发布”条目，README链接清单，明确旧发布记录不代表本轮已部署。仅完成发布准备文档，不宣称F6合并准入或整体完成，不将必要外部证据降为可选。文件差异检查通过，提交前继续秘密扫描。

## F2/F3 / 切片11B专用MR写入验收程序

Workflow Gate P10，R7授权范围及现有Webhook/Review/评论generation/只读receipt为上游，允许先实现程序和本机测试，真实运行仍必须取得人类对专用MR的明确授权。脚本限定app/gitlab/project/MR/full HEAD，显式--allow-test-mr-writes；会话、GitLab token和Webhook token仅环境读取。preflight先GET readyz及MR核对HEAD，检查私有输出目录且拒绝覆盖，开始后保存逐阶段私有证据，失败不隐瞒已发生副作用。不修改仓库、项目配置或凭据，不自动删除评论或覆盖人工编辑。

流程：向应用重放该专用MR的open Webhook→要求created新run（已有去重结果不能冒充创建实证）→等待固定同一HEAD的succeeded/incomplete与sent→重放同事件确认同run且created=false；核对GitLab指定discussion/note及唯一任务标记。明确选定一个有效发现（零发现则停止且保留已创建审计证据），提交测试人工复核false_positive，等待generation递增且同discussion/note更新、正文hash改变。再通过GitLab API在此测试note追加显式验收人工编辑标记，随后改review为pending，等待comment state conflict，核对GitLab正文仍为人工编辑值且note身份不变。保留测试复核/冲突和人工标记，不自动清理或覆盖。每次写之前重新核对当前MR HEAD，漂移立即停止；并发人工编辑的检查与PUT不是原子条件写，必须专用无人并发测试MR，记录此边界。

10秒单请求、总体有界等待、响应2MiB、HTTPS远端/本机HTTP、禁止跟随重定向并关闭HTTPError，固定错误摘要不输出正文/凭据。私有证明0600只保存阶段、ID、generation、SHA/hash、布尔断言，不复制源码/评论/密钥。实际本机HTTP测试覆盖请求顺序、创建/去重/同note更新/冲突保留、漂移停止及失败阶段证据。重放Webhook仅证明应用入口，不证明GitLab网络投递；最终真实GitLab还需专用MR事件投递记录对照，不把重放冒充平台外部投递。

## F4/F5 / 切片11B写入验收程序及本机HTTP测试

新增gitlab-write-acceptance.py：明确测试写入标志、固定对象、三类环境凭据、preflight、逐次写前HEAD重检、新run要求、Webhook去重、唯一marker有界核对、有效finding复核→同note/generation/body变化→人工编辑→pending复核→conflict且人工正文保留。私有JSONL先独占创建并逐阶段flush/fsync，失败保留副作用边界，不复制评论源码或凭据；HTTPError关闭且不跟随重定向。ops说明真实运行授权、无人并发/非原子PUT边界和重放不等于外部投递。

4项真实本机HTTP测试2.341秒通过（ResourceWarning=error）：完整五次写入/同note更新/人工冲突保留、HEAD漂移阻止第一写、零finding不写review、403错误响应closed且停止。py_compile通过。此测试服务器模拟平台/GitLab状态，不是实际应用Worker/GitLab端到端，不替代已有原生合成集成或未来专用真实MR。真实授权未收到，未运行外部写入。R7可复用程序已实现，原生平台联测和真实专用MR验收仍待后续。

补充显式写入标志缺失时零网络请求检查；receipt与write共9项测试2.854秒通过（write共5项）。未扩大真实写入授权。

## F2/F3 / 切片11C原生写入程序联测

P10验收阶段，11B程序及现有原生恢复fixture为上游，允许窄范围联测。smoke-worker-recovery新增--gitlab-write-preview，拒绝与会占用MR92的metadata等其他preview混用，预算崩溃模式也互斥；保持既有恢复先验。仅此模式启用合成Webhook token与评论，fixture MR响应补真实GitLab receipt需要的project_id/sha。使用现有合成登录session启动验收CLI子进程，目标MR92固定b SHA；脚本凭据只传子进程环境，证明JSONL0600不输出。核对原生run/投递数据库conflict、upstream一次create/两次PUT（应用更新与显式人工编辑），命令退出成功。无需构建新应用代码，使用已经编译含readyz的隔离二进制；失败保留私有目录。此证明是真实应用/Eino/Worker对合成上游，不是用户GitLab投递/模型质量。

## F4/F5 / 切片11C原生写入联测与等待修复

新增独立--gitlab-write-preview、合成Webhook凭据及评论开关、MR project_id/sha与CLI环境session传递。首轮因macOS /var实际symlink的私有目录检查拒绝，在调用端传root.resolve真实路径，未降低工具规则。第二轮已创建pending run但真实API comment_sync=null触发程序AttributeError提前退出，原生证明未成立；修复detail/converged将null视为尚未建立同步，补真实HTTP首轮pending/null再终态测试。9项工具测试2.221秒通过。

第三轮19264/19265原生演练exit0：实例自然租约过期恢复保持既有证明；MR92通过Webhook新run5、去重同run、真实Eino两次HTTP、同discussion/note更新、显式人工编辑、第二次review进入conflict并保留正文。fixture核对create=1、PUT=2（应用更新+验收人工编辑），证明JSONL/总体proof本机0600。使用已有干净c789432应用二进制，当前脚本来自工作区；未声称此二进制包含最新脚本提交版本，未改1234。该证据完成程序与真实Worker本机集成，不证明用户GitLab网络投递或真实模型质量；授权外部MR和其他验收继续未完成。

## F5 / 切片12G查看成员实际界面权限

19258隔离实例通过管理员API建立合成ui-fixture-viewer成员及项目1 viewer授权，真实浏览器退出管理员再登录该合成成员。设置旧路由显示页面不存在或无权限，管理员导航隐藏；任务列表无新建入口且解释无可执行项目，已授权报告可读、复核控件隐藏并提示需授权，补审仅查看固定清单且提示提交需操作权限。历史移动关联空状态显示只读，项目页仅项目1且无添加/成员授权/关联仓库/停用入口，未授权项目2不显示。

元数据发现不符合行锚点移动关联，seed_move_report原样生成8无候选是预期，不能把空状态当候选只读验证。另以明确合成持久化记录9构造行发现（不改原生5/6结果），重新计算服务端兼容指纹，再生成10。实际界面展示9→10移动候选、两端链接/事实展开及只读提示，没有理由输入或确认/拒绝/撤回按钮。此fixture仅验证权限UI，不证明Git移动或模型质量，私有viewer-association.jpg 0600。完成R6候选viewer只读和R4管理员配置入口限制；跨仓库报告权限交集仍由已有实际HTTP/Store验证承担，不把此主仓库viewer界面当跨仓库授权撤销全覆盖。

## F6 / 当前综合回归（并发检查尚在运行）

8333ea8干净分支执行go test ./...与go vet ./...均exit0（Go测试缓存命中），Python全scripts测试以ResourceWarning=error执行24项3.552秒通过，React tsc+vite生产构建1.07秒通过、资源hash与已验收版本一致且未产生跟踪差异。git ls-files确认config.yaml/.env.deploy/pr_agent.db/aimangebot未被跟踪，暂存秘密检查通过。另启动完整go test -race ./internal/platform，进程句柄已多次确认仍运行、无输出，尚不能记录通过；下一轮只继续等待原句柄，不因无输出重启。真实外部质量与GitLab、main和1234仍未完成。

同一并发检查句柄随后正常exit0，完整platform race88.470秒通过；未重启检查。上述综合回归结果齐全，但不抵消外部验收缺口或自动证明当前1234部署版本。

## F2/F3 / 发布策略版本隔离

P10发布准备，原有PolicyVersion去重/Worker/重试隔离为上游，允许窄变更。当前仍v12，与已部署上一轮相同；本轮新增共享模型预算、独立复核、分片/补审和跨仓库策略，必须v13隔离执行语义。仅修改runs_store.go常量，历史报告/策略不重写，旧pending由既有Worker版本检查拒绝且要求新提交，旧重试不会自动生成新版子任务。README说明历史报告仍可查看，新提交使用新版本。先核对既有Worker与retry测试覆盖，再完整Go回归；不因版本升号宣称已部署，私有1234不变。

## F4/F5 / v13策略隔离实现

PolicyVersion升为eino-audit-contract-v13，历史不重写；README说明旧排队任务拒绝与重新提交。新增旧v12任务失败后retry state=policy_changed/无child/证据保留、新提交不同run且捕获v13测试，首次定向0.974秒通过；完整Go平台29.576秒与vet通过。随后补原生Runner旧pending验证，任务failed且固定升级提示，blockingAuditor从未启动；两项升级测试定向race2.996秒通过。此轮应用常量变更尚未重新构建/部署生产，之前c789432原生fixture仍v12，不追认旧演练为v13部署证明。后续需要新二进制升级验收、外部质量/GitLab与main。

## F5 / v13干净二进制原生复验

a65441c干净源码构建至本机0700临时目录，go version -m核对vcs.revision完整匹配、vcs.modified=false；二进制0700及SHA256仅私有proof。19266/19267使用该二进制跑--gitlab-write-preview实际exit0，恢复父检查点一致、替代进程等自然lease失效、恢复子成功；Webhook run5同note更新与人工冲突保留，上游90/91/92模型HTTP=1/3/2。只读SQLite全部任务policy_version精确v13，私有proof补版本/构建摘要0600。新版策略在实际Worker/Eino/评论链路已验证，生产1234尚未替换，main未合并；外部质量与专用GitLab授权仍缺。

## 外部样本可用性与演练清理

通过现有GitHub CLI只读查询当前origin仓库Ed1s0nZ/AIMergeBot全部状态PR，第一页per_page20结果为空数组，不能从该仓库选历史PR冒充R1实证。未修改外部对象，未发送评论。已请求授权历史PR/本机固定base-head样本及专用GitLab MR写入授权，无需密钥；缺口仍保留。已完成界面验收的19258合成服务父进程以精确PID92525正常SIGTERM，原工具句柄47463确认exit0，清理自己的应用子进程；私有证据目录保留，未触碰1234。当前真实外部验收资料未收到，不能将全部目标标记完成。

## 发布清单同步与分支秘密检查

当前5440f03工作区干净，origin/main仍bcaeae2；总差异126文件。发布清单将已通过的综合回归/升级检查与R2/R5/R6回归状态同步，未提前勾选外部质量、最终人工检视或部署。扫描125个ACMR最终文件及origin/main..HEAD所有提交补丁：凭据形态及从本机私有配置读取的已知秘密均未发现，控制台仅摘要，不输出秘密值；不等于所有未知秘密均能识别。私有config/.env/DB/binary继续ignore。真实样本与专用MR授权仍未收到，没有可领取的真实评测任务；main/部署仍未执行，整体未完成。

## F2/F3 / PRR-001修复

P10窄范围缺陷修复，冻结检视报告docs/reviews/production-audit-b2e043f.md为上游。平台非force允许重审incomplete是既有业务契约，不为工具改变。验收CLI在首轮converged后若status!=succeeded，记录audit_incomplete_stop及run ID后失败，禁止第二Webhook及任何review/人工编辑；保留初始任务/评论，明确去重未验收。补真实本机HTTP incomplete+sent反例，模拟应用第二提交会新run，断言只有初次Webhook一次写入。原succeeded路径仍验证去重/同note更新/冲突，随后re-review关闭PRR-001并检查门控新风险，不以此批准全126文件。

## F4/F5 / PRR-001修复与区分度验证

CLI首轮converged后新增succeeded门控，incomplete记录停止阶段与未验证去重，禁止第二Webhook及review/人工note改写。平台原有重审incomplete行为未修改。HTTP fixture按incomplete语义在第二提交会新建ID，反例断言只有一次POST；原成功/漂移/无finding/权限失败/无显式授权标志路径保留。receipt+write共10项ResourceWarning=error测试3.403秒exit0。ops说明停止边界，不宣称incomplete已完成去重验收；之前原生succeeded实证仍适用于未变化的成功分支，此次未重复启动原生进程。PRR-001代码和验证已修复，正式re-review报告待下一阶段，整体分支尚未批准。

## F2/F3 / PRR-002异常用量汇总修复

Workflow Gate P10，现有R2预算/费用契约与f69e310独立检视为上游，允许窄修复；分支codex/production-audit-readiness，无外部写入。异常供应商响应TotalTokens为负数时，汇总未按预算逻辑标记未知；多个极大计数可能int64溢出并生成负费用。修复model_usage.go及测试，拒绝负total；每次累加前检查全局/阶段两项和是否溢出，失败时该调用计入未知且不改变已知合计。遇到溢出整个费用估计不可用，reason=usage_overflow，不用部分金额冒充可靠费用。普通未知调用仍保留已知部分费用，零total但正prompt/completion继续兼容现有接口。不改变API字段、预算策略或历史数据。测试负total、同阶段及跨阶段prompt/completion溢出、已知部分保留及无负费用。完成后记录F4/F5并秘密扫描提交推送。

独立运维检视另确认PRR-003：urllib socket timeout不是总体deadline，慢响应可超过健康检查期限。此问题保留待单独设计和修复，不因PRR-002关闭而宣称完整检视或发布完成。

## F4/F5 / PRR-002修复及定向回归

用量汇总拒绝负TotalTokens，计数累加前检查全局及各阶段int64上限；溢出调用记未知、已知合计保持不变，费用估计为空并标明usage_overflow。新增同/跨阶段prompt与completion溢出及负total反例。go test ./internal/platform -run 'Test(ModelUsage|DifferentVerifierPrices)' -count=1通过（1.050秒），既有部分用量和独立复核价格测试保持通过。此修复不代表全分支检视或发布完成，PRR-003健康检查总体deadline仍待修复；真实外部验收及main/1234仍未完成。

## F2/F3 / PRR-003健康检查总体期限

P10，R8有界只读就绪检查与独立检视慢响应反例为上游，允许窄修复。urllib timeout只能限制socket静默，DNS/连接/头/正文没有总期限。把现有HTTP观察移入独立Python子进程，父进程subprocess.run(timeout=...)对整个观察施加期限，超时终止并回收子进程，返回unavailable。保留URL/凭据/协议/redirect/1KiB响应规则，子进程不写文件、不重启服务、不通知。隐藏内部观察参数仅供父进程，正常CLI行为不变，HTTPError继续关闭。测试慢正文和慢头，1秒deadline内不得接受最终ready，容许进程回收与调度余量；正常及失败边界保留。父进程生成的URL只有通过验证后才传入子进程。进程创建/回收有系统调度开销，不宣称硬实时保证。另为PRR-002前端增加usage_overflow说明，避免误显示为缺少价格；不改变API。

## F4/F5 / PRR-003整体期限与费用说明

健康检查父进程对独立观察子进程设置subprocess总timeout，超时kill/wait回收并报告unavailable；DNS、HTTP头与正文均在子进程，保留原有重定向/协议/长度/JSON规则。真实本机HTTP慢头和每0.12秒一个字节的慢正文均在1秒期限（含回收允许1.8秒）返回不可用，不再等待最终ready。3项healthcheck测试3.922秒通过，ResourceWarning=error；前端增加usage_overflow中文提示，tsc+Vite生产构建2.56秒通过并同步嵌入资源。整体回归及修复re-review待继续，未把本机慢服务测试当生产监控或完整发布证明。

## F5 / ac9a5b5整体回归及剩余检视

干净源码ac9a5b57823fd97d0cda44398dc7b0f523bb63b6运行go test ./...与go vet ./...均exit0，platform测试60.619秒；Python所有test_*.py共26项9.347秒通过且ResourceWarning=error。上一轮同源码前端构建已通过，没有追加应用修改。主检视继续读取模型预算/费用/重试说明、固定选文件补审、复核与工具观察、报告轮询调用，以及历史评测PrepareCase/路径隔离/commit验证/标签匹配和时序布局；未把阅读有限文件当完整分支审查。独立上下文正在只读检查f69e310→ac9a5b5修复delta（实际agent句柄），结果尚未取得，PRR-002/003正式re-review待完成。外部质量样本和专用GitLab授权缺口保留，main及1234未更新。

## F6 / 设置、公开API与时序契约补充检视

6a26fce干净工作区继续检查base→head设置/项目/API类型/时序界面/config示例及Go配置差异；对照settings DecodePublic→revision/空密钥保留→0600临时文件fsync/rename→Public→React保存反馈、项目配置专用接口与同步、关联仓库admin注册。读取时序reference验证与固定source/调用前后权限核对、每仓库缓存key、primary风险锚点限制、SDK提示与Mermaid仓库标签，并核对真实Git时序反例测试（错误SHA、未知仓库、关联替换primary锚点、权限撤销）及来源观察隔离。检查routes错误码与TS映射、run冻结费用与retry摘要、关联报告list ACL、polling失败不标记已见版本、取消中断与评论跨仓库禁止。README/evaluation/ops公开说明与实现对照，未发现新的可行动缺陷；健康检查操作说明补充整体子进程期限。此为源代码/契约检查，不替代真实外部调用、浏览器全部响应式状态或最终整个分支准入结论。

## F7 / 当前候选构建准备（未上线）

干净源码7ccf4896f6c4b65504d8f8ae588cdd5f045979a8执行go build生成私有临时目录候选二进制，go version -m精确核对vcs.revision及vcs.modified=false；目录/二进制0700、构建proof0600（只含版本、摘要与production_deployed=false，不入Git）。此构建含最新模型用量修复与前端嵌入资源，未替换现有1234，也不表示最终发布门槛通过。模型用量、冻结费用HTTP、重试预算和检查点定向race检查通过3.117秒；之前完整Go/vet/Python/React证据继续记录其实际源码快照，不追认新文档提交为已部署。外部真实质量样本与专用MR授权仍未收到。

## F2/F3 / 已授权真实GitLab测试与覆盖说明契约

用户明确授权使用其提供的GitLab凭据配置并创建模拟项目进行全程测试，允许该专用项目/MR测试评论。已通过实际/api/v4/user确认认证，使用原系统设置API保存GitLab URL/token到忽略的config.yaml（0600），不输出令牌；用户指定namespace143945267实际为group，按该namespace创建private aimangebot-e2e-test。MR1删除权限检查，MR2完整调用/存储边界禁用权限条件。新版隔离应用使用真实GitLab和已配置模型；两轮均发现风险且独立复核supported，但结果incomplete。MR1有无效行号工具错误，真实未完成证据保留；MR2覆盖说明仅包含静态未运行和单文件边界，未产生评论。两次等待已精确停止，应用正常清理，私有证据保留；不声称评论验收通过或用模拟样例当真实PR准确率。

Workflow Gate P10，上游为R2/R7静态审计契约与上述实际模型结果，允许窄提示词修复。coverage_notes专指具体未完成工作、读取/预算省略、无法核对的保护条件；不执行代码为既定方法，不单独作为覆盖失败；文件数量少本身不代表遗漏。方法和运行可利用性限制放summary/发现trigger，真正缺失调用/配置证据仍必须coverage_notes。只修改主Agent提示词，不移除或过滤服务端覆盖校验、工具失败、验证失败或真实结果notes，不追认此前任务为成功。未发布v13保持，生产仍v12。先定向Agent/覆盖回归，干净新构建后在新隔离实例重审同MR2，禁止覆盖旧证据，再验证真实评论链路。秘密扫描后提交推送。

## F4/F5 / 静态覆盖提示词澄清

主Agent提示词明确coverage_notes具体缺失含义，静态不执行和单文件大小本身放方法/触发限制；真实缺失调用、配置、保护证据仍必须说明。没有后处理删除notes、没有改变工具错误或服务端incomplete判定，原测试结果保持。定向Eino/覆盖/跨源时序与独立复核测试6.215秒通过；真实GitLab复测与评论链路尚未通过，随后使用干净新构建验证。

## F5 / 用户GitLab专用模拟项目实测

已授权目标为用户指定namespace下private项目aimangebot-e2e-test（https://gitlab.com/1-group2395641/aimangebot-e2e-test），MR1/2均不合并、不执行代码。三次真实DeepSeek+真实GitLab+原生Worker固定Git审计（MR1、MR2、64990bd重测MR2）各产生1个真实校验发现，独立复核supported；MR1曾错误提交未变更行，保留工具失败覆盖；MR2两轮仍将静态方法/仓库小描述为coverage_notes，状态incomplete、没有评论投递。提示词澄清未使真实模型自动达到succeeded，不能宣称该模型行为已完全修复。等待评论不可能收敛后精确停止工具子进程，应用正常退出，原始私有证据保留；未改变状态或删除覆盖说明。

随后独立隔离实例使用64990bd干净二进制、真实GitLab固定MR2对象与仓库读取，模型响应明确为synthetic transport fixture（假凭据、仅本机模型HTTP，不发送真实模型API Key）。模型复核/时序阶段在该评论专项关闭，因此不证明模型能力；正常Eino结果解析/证据校验→真实Worker→GitLab流程。写入验收程序exit0：新run→重复Webhook相同run且created=false→真实discussion/note首次创建→人工复核后同note更新generation1→2→通过GitLab显式人工编辑→第二复核使desired3/sent2且state=conflict，GitLab正文保持人工编辑内容、标记唯一。MR2保留专用测试评论和人工冲突证据，未自动清理/覆盖。测试服务和本机模型server正常清理。私有proof/配置/DB/日志均不入Git；生产1234未升级。

这是应用入口重放Webhook的实际GitLab读写集成，不证明GitLab从公网主动投递至本机（127.0.0.1无公网入口）。模拟仓库加真实模型仅验证这几个设定案例，不是授权历史PR准确率、误报/漏报统计或跨仓库大PR质量评测。当前已满足用户新授权的凭据配置、模拟项目/MR创建及上述链路实际测试，整体main/生产部署及原验收缺口仍需继续处理。

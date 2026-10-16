# API模式固定双树差异合同（F2设计）

2026-10-16；codex/sidebar-workflow；输入为Confirmed requirements.md的语言无关PR审计、REQ-007/012/020、AC-007/018，以及repository-recovery-contract.md、repository-budget-contract.md和S4计划。当前不是已实现API-only原生审计，也不是完整S4开发许可。

## Workflow / Maintainability Gate

P6/F2；已核对githubObjectReader.rootTree/tree/blob、githubTreeWalk/listFiles、GitChangeMetadata/BuildDiff、GitLab metadata读取以及Git默认Changes路径。缺少API双树完整changes producer、固定PR observation、共享预算接线与完整factory。允许本轮写具体合同及研究现有候选源码；生产实现no。维护风险high：graph遍历、provider IO、text matching、hunk渲染和scope传播不可混为单文件；建议repository_tree_diff.go只做不可变tree节点比较，repository_text_diff.go做纯文本适配，GitHub/GitLab对象读取各自保持provider模块，factory负责编排。

本合同保持默认Git完整工具链，API-only不得偷偷调用Git。不能把增加read client方法或一个纯diff函数算作原生审计交付。无新UI、外部写入、主依赖或schema变更；未来唯一新增policy预算字段按独立预算合同处理。

## 问题与期望

GitHub compare返回的files最多300，不能翻页获得完整文件集；现有listFiles只返回regular files且有2000文件限制，不能直接作为完整metadata树。期望在固定merge-base/head上，独立建立完整变化目录，超过覆盖/对象/预算限额明确incomplete，不把未观察路径视为不存在。PR base分支移动不影响已冻结run。

compare只可提供经固定观察验证的merge-base等元信息；其files/patch/rename不作为完整changes权威输入。主仓与fork source分别用授权reference/client读取，不能用一个token或target数字ID假读source。

## 提案内部对象（全部新增、尚未实现）

| 对象/字段 | 类型/必须 | 所有者/校验 | 兼容 |
| --- | --- | --- | --- |
| PinnedTreeSide.reference | RepositoryReference/必须 | scope冻结身份；使用前后原有fence，不能从path推授权 | 不替代scope或授予权限 |
| PinnedTreeSide.commit | string/必须 | base=observation.merge_base_sha，head=observation.head_sha，必须匹配Snapshot | full SHA，不用branch |
| TreeNode.path | string/必须 | provider返回规范相对路径，UTF8、无控制字符/空段/..，<=4096 bytes、depth<=64 | 无法表达路径记缺口，不修正/归一化成另一文件 |
| TreeNode.entry | GitEntry/必须 | mode/type/OID按已有validGitEntry；tree单独040000；SHA按provider格式 | 复用既有metadata |
| TreeNode.size/size_known | int64/bool | blob已知非负，缺失不能假0；同OID矛盾size拒绝 | GitLab树不提供size时由有界blob读取证明 |
| TreeDelta.changes | []Change/必须非null | 有界、唯一稳定排序；路径/flags/metadata相互一致 | 复用Repository.Changes输出 |
| TreeDelta.catalog_complete | bool/必须 | 全部变化目录已完整建立才true | false绝不能输出“无变更”安全结论 |
| TreeDelta.gaps | []typedGap/必须非null | 有限code、side/path/阶段/范围；不含原始响应/密钥 | 后续映射现有Notes/Coverage |
| TreeDelta.text_complete | bool/必须 | 目录完整不等于所有文本已检查 | 二进制/大文件/预算等仍可false |

这些不是新的公共HTTP字段；执行结果通过既有Change/Notes和覆盖投影导出。若需要public typedGap，必须另补producer→DTO→TS→导出/通知/检查传播，不能只发后端枚举。

## 双树目录算法

1. 按scope验证target/source/context、credential和预算op，源角色授权后才读取源commit/tree/blob。base与head分别取固定commit的tree，验证对象身份与完整节点响应；Github沿已有rootTree/tree方法，不能让递归tree truncated=true进入cache。
2. 使用队列对齐同一路径的base/head目录。已验证的两侧tree OID相等时，该子树可整体跳过；权限/fence仍执行。GitHub可按tree OID读取；GitLab若只能按commit+path列目录，其cache key必须包含固定commit+path，不把未知root OID伪造为已知。未知或不可信相等不能用于剪枝。
3. 每个目录必须完成所有页，才可判断某entry缺失。GitLab按per_page100顺序验证NextPage/Link；缺失/矛盾分页元数据或满页但无法证明末页是gap。GitHub单目录API明确truncated=false；不得用2000条截取后当完整目录。
4. 同名entry双侧比较：同mode/type/OID且大小不矛盾为不变；目录对目录递归；文件对目录先完整展开目录那侧，再表示旧文件删除及其子项新增，反向同理；不同类型普通entry形成type_change。
5. 单侧目录也需完整展开后输出叶子add/delete，包含regular、executable、symlink和gitlink元数据，不只列regular文件。gitlink仅描述记录的commit，不访问其外部仓库；symlink仅是blob对象，不解引用路径。
6. 每侧每条ancestor链检查cycle；重复tree可出现在多个prefix，不能因全局visited跳过另一路径。缓存命中不免expanded-entry/path-byte计量，防止小DAG指数展开。
7. 目录预算为两侧合计：expanded entries<=20000、每directory<=2000、输出叶子changes<=2000、累计规范path bytes<=8MiB、depth<=64；全程还受共享请求/API byte/grant预算及ctx约束。达到限额不返回半个目录为完整。
8. 目录异常时catalog_complete=false，保留已验证条目只供诊断；正常Repository.Changes包装不得将其送成完整PR差异开始模型审计。由Runner以coverage不足终结/明确incomplete，不把剩余路径推断为add/delete。文本阶段的单文件缺口可保留完整目录和其他有效证据。

对301个实际变更的受控fixture必须输出301项，不读取compare.files作为权威；2001项应显式不足而不是静默截断。相同tree剪枝需要固定对象身份依据，不能以“文件名没出现在compare”剪枝。

## 元数据与rename/copy规则

每个Change复用GitChangeMetadata，add/delete的OldPath/NewPath均填该路径以满足现有valid协议；缺侧entry为nil，同路径修改填实际两侧entry。可执行位变化但OID不变为metadata-only。目录不作为可锚定文件；gitlink只允许metadata锚，不能发明内部文件行号。

跨路径rename只在一个删除与一个新增具有完全相同mode/type/OID，且该组两侧各唯一时认定。多对多或带内容变化不猜测；保留完整delete/add，不能凭同名/标题/部分API列表合并。copy表示新增，不做推断。此规则可能与Git相似度rename结果不同，但对象/路径覆盖必须完整；不能因未认定rename丢掉文件或复核记录。若需要展示“未做相似度重命名推断”，必须用中性说明而非把完整覆盖标为失败。

## 文本取得与证据

- 只在目录已确定真实变更后读两侧必要blob；所有读继续同ledger/op，不为第二侧新建额度。
- regular/executable、symlink目标均可通过固定blob OID取原始bytes；symlink不跟随，不访问任意URL。本提案若扩展API ReadFile对symlink返回目标，必须明确标为link对象内容并补契约测试；未完成该一致性前symlink仅输出metadata和文本缺口，不能说其目标已审计。
- GitHub沿blob API base64/size/sha1校验；GitLab使用固定blob raw对象endpoint，验证响应实际size与按当前object format计算的blob OID。未知format或provider协议不支持时拒绝，不降到默认branch/Contents URL。正式GitLab raw协议及SHA256受控兼容测试是实施前依赖，尚未执行。
- 每blob<=256KiB。NUL、无效UTF8、大文件、缺失对象/内容或对方权限不可读均给对应路径gap；不空字符串代替。空blob是合法0字节，必须与读取失败不同。
- blob bytes在scope/credential绑定cache内不可变。重复相同OID可复用内容但cache命中仍做fence。文本hunk缓存按已授权两侧身份及OID对，最多512项/8MiB，防止同内容重复路径反复消耗匹配CPU；path/metadata不从别的文件复用。

## 行匹配与hunk渲染候选

拟固定github.com/pmezard/go-difflib v1.0.0，只使用NewMatcher/GetGroupedOpCodes(3)的成熟行匹配，不直接采用GetUnifiedDiffString输出。已读[该tag源码](https://github.com/pmezard/go-difflib/blob/v1.0.0/difflib/difflib.go)：writer原样拼接输入行，不能直接保证无末尾换行文件的Git sentinel语义；其buffer Flush错误也不作为调用返回。主go.mod当前未新增此依赖，go.sum已有校验不代表已经正式依赖或验收。

纯适配器需：

1. 按LF拆成保留原始换行的token；CRLF不归一化；空文件为0 token；最后无LF的token保持无LF。两侧token<=2000、bytes<=256KiB，否则仅标该文件text budget gap，不把整片中间区域全标新增。
2. 用matcher的grouped opcodes建立区间；对每opcode验证单调、无越界、两侧覆盖一致。未变区间确实bytes相等，replace/delete/insert按真实输入token渲染。
3. 输出从@@开始，与既有Git Changes一致；hunk起点/长度从token区间算，不从服务器patch抄。空range起点按unified diff规则为相邻位置，可为0；added/removed锚仍是原侧真实1-based行。
4. 每token写prefix后原始bytes；没有LF时补输出LF和`\ No newline at end of file`标记，不能把下一行header贴到同一行。不在源文本中补LF后再比较，必须保留仅末尾换行变化。
5. 显式有界builder检查每次写入，hunk输出<=512KiB/文件且整次<=8MiB；不靠defer Flush吞错。单文件超过则丢弃该文件不完整hunk并标gap，不返回截断hunk。
6. 匹配前后检查ctx；该库不支持中途ctx取消，2×2000行/2×256KiB为输入上限，不能宣称它提供硬CPU时间限制。正式采用前以重复长行/高重复/全部替换反例测运行开销；若不能满足运行预算则替换为可中断matcher，不能用goroutine超时遗留计算继续消耗CPU。

这是明确候选而非生产选型。API diff可与Git选择不同的有效匹配，但所有删除/新增锚必须来自能无损把base变为head的edit script；尤其不能把“首尾间全替换”当精准差异的捷径。匹配性能/边界与依赖安全扫描未验证，不能从成熟库标签推导已就绪。

## 数据流

```mermaid
flowchart TD
  A[固定scope与共享budget session] --> B[base/head分别授权的对象reader]
  B --> C[完整目录对齐 相等tree剪枝]
  C --> D{目录完整且预算有效}
  D -->|否| E[catalog gap 禁止当完整diff]
  D -->|是| F[唯一叶子变化与确证rename]
  F --> G[固定blob bytes与身份校验]
  G --> H[有界行匹配/渲染 或单文件gap]
  H --> I[Change + GitChangeMetadata + Notes]
  I --> J[既有BuildDiff 96KiB模型窗口及Added/Removed锚]
  J --> K[原有覆盖/验证/最终状态/发布协议]
```

BuildDiff的96KiB是模型初始窗口，不能替代完整变化目录；窗口遗漏仍沿现有覆盖说明。formatter中性信息、真实text gap和目录失败应区分：不能将所有Notes无差别变成阻断，也不能丢掉真实coverage gap。

## 实施交接与反例验收（尚未执行）

- 真实API transport fixture分别实现GitHub tree/blob与GitLab tree分页/raw blob，内容从同一临时真实Git仓库生成；禁Git PATH时API-only仍完成取数，运行命令计数为0。Git仅在测试侧生成oracle，不能由被测adapter偷偷使用。
- 301变化、2001变化上限、>2000个未变文件中的少量变更、双树相同OID剪枝、共享子树不同prefix/DAG、cycle、错误size/SHA/type、无效/非UTF8路径、分页重复/丢页/满页无末页证据、截断tree、source撤权中断均有判定结果。
- mode-only、regular↔symlink、文件↔目录、gitlink、空文件、全删除、全新增、单行无LF、只改末尾LF、CRLF、Unicode、二进制、大blob、重复长行、rename唯一/歧义/内容变动验证路径集合与metadata，不要求API相似度rename与Git猜测完全相同。
- text edit script应用base必须逐字节得到head；对简单非歧义case与实际git diff --no-ext-diff --no-textconv的Added/Removed行锚核对，BuildDiff不得产生不存在或未变化的锚；对于多种有效匹配的case验证重建与固定侧证据，不强行继承另一个算法的行号。
- budget/ctx/错误/缓存/revoke/fork分别断言实际请求次数与字节：不授权零内容读；双树同ledger；不完整目录不得返回空成功。无真实模型/托管服务请求。
- 集成条件：最终factory正式v51 producer、所有consumer和预算/网络/资源合同通过；先保持原guard，不因为纯算法测试通过开放绑定。

## 仍需关闭的实施决策

matcher正反例性能/依赖检查；GitLab固定raw对象及SHA256协议证据；symlink读取工具的一致语义；typed gaps到现有覆盖与状态投影的准确映射。不得默默猜测这些规则进入生产。完整资源隔离设计独立推进；本文件不宣称原生GitHub/API模式、跨仓完整流水线或UAR-001已完成。

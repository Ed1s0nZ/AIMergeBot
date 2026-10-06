# 调查命题独立复核（v45设计，契约已实现，执行与界面待接入）

## Workflow Gate / Lifecycle

用户目标：Confirmed R3完整最佳实践优化，语言无关、授权跨项目、PR增量。上一轮为progress（v44源码及SDK/独立/full/CI证据）。阶段P10/F2；AC002/003/005/006/007/010/011/012与v42/v43真实命题极性/空ID回执具备，允许设计与实现，不需新增权限/账号/运行时执行。现有复核只检查finding，无告警resolved investigation缺独立判断。缺口：新的持久化/模型/UI契约须先本文定义；不改用户确认需求，不提高预算以获得completed。分支codex/audit-quality-loop，无自动合并部署。

## 当前实现状态

共享预算基础3f493d3及server-owned/parser/来源契约814c60a已实现并通过全量Go、定向race和独立切片检视。调查复核runner、共享pool接入、UI/配置/SARIF和真实模型效果尚未完成；策略仍v44，没有新增调查复核调用。CVR-001比较BASE路径缺口已在814c60a修复，初版e52525d检视否决保留为历史。

## 依据与边界

采用独立回答核验问题、减少沿用首轮结论的思路：[Chain-of-Verification, ACL 2024](https://aclanthology.org/2024.findings-acl.212/)。论文对象是生成文本事实核验，不是本项目代码审计；本文将其应用到固定源码命题是工程推断，效果必须由本项目真实评测证明。此次不是再加一个语义枚举或提示词，也不保证模型消除错误。

## 场景和非目标

首审可能正确解释兼容default25，却把“保持兼容”命题标为rejected；只有工具/计划来源格式通过无法发现这种语义冲突。新增fresh read-only review判断实际Claim是否成立，服务器将独立verdict与原status比较，显示一致/分歧/未知；原Claim/status/evidence/失败历史完全保留。它不是安全证明、风险存在分类或运行复现，不自动翻转原判断，不产生finding或回到primary重试。investigating调查保持原未解决状态，不借复核替代其计划/记录。所有相关语言均用既有文本取证，不新增语言准入。

## 契约

Investigation新增可选server-owned claim_verification，历史缺失可读。字段：status=consistent|disagreed|inconclusive|unavailable|disabled；verdict=true|false|unknown（字符串、未执行时省略）；assessed_claim原命题快照；model；reason≤1000字符；limitations最多8条各≤240字符；observation_ids最多20唯一fresh源码ID；base_sha/head_sha。状态只由服务器生成：supported+true或rejected+false一致；相反为disagreed；unknown为inconclusive；不足/失败为unavailable；系统关闭为disabled。一致仅表示两次静态判断一致，不能叫命题已证明。

模型返回严格JSON仅verdict/reason/limitations/observation_ids，禁止额外字段/trailing/超限；不允许模型生成consistency/status/model/SHA等权威字段。初始user只给固定snapshot、actual Claim和有限source navigation，不给首审status、证据陈述或理由；Claim/路径为非可信数据，system只固定说明。复合命题须核查所有核心断言；必要断言未知则unknown，不因无finding判false。

record/update不得保存模型注入的claim_verification，schema隐藏它，历史字段可解码但由服务器清除；纯主审ledger不伪造独立结果。clone、group merge、checkpoint及最终JSON保留合法server-owned review；model final investigations仍由真实ledger覆盖。新fresh观察使用独立阶段investigation_verification和digest ID前缀，不得复用首审或另一复核ID。

## 来源和失败

readonly whitelist复用，未知/记录/resolve/submit/写入工具默认拒绝。每个verdict ID须确属本fresh context、成功source工具、evidence_eligible、仓库/固定SHA/非空源码且唯一；验证source两侧/主PR关联，以及所有必要配置上下文的实际读取。主仓库BASE/HEAD取证不能被其他仓库替代；metadata-only只采用原精确固定metadata机制，不能虚构执行路径。目录枚举不是源码。缺任一必要来源/失败/partial则不授予一致状态，保留inconclusive/unavailable说明。来源校验不能保证模型理解正确。

执行前checkpoint、模型回调、token总预算、取消、执行器不可用/租约冲突继续适用。复核无状态更新权；使用fresh工具记录source/usage至parent，失败只产生复核未完成，不抹掉原调查/发现。

## 共享预算和顺序

VerifyFindings保持配置键兼容；启用时同时允许findings和已resolved调查命题复核，界面/配置文案明确范围。不新增默默启用的配置。原verification总工具40次、总60秒、单项最多10次/15秒、graph8预算、最后10秒收尾和全audit token预算保持。finding按现有排序优先；调查按稳定ID排序使用剩余同一pool。不得为调查再创建40次/60秒预算，不能把不够的复核标成成功。所有未执行的resolved调查明确unavailable；disabled可辨。单项失败无盲重试。

## 消费与完成

工作台原status文案准确表示命题初审，不用“已排除”混同无漏洞；新增复核面板显示actual assessed_claim、模型、判断分歧/未知、限制和可打开fresh source。复用ObservationLinks及原details导航，历史空态、进行中、失败、关闭、小屏/键盘都明确。API/go/frontend类型一致，旧报告不回填。

disagreed/inconclusive/unavailable追加CoverageNotes，worker仍incomplete，0finding不能转为clean；既有SARIF notifications与executionSuccessful反映状态，run properties可携带调查复核但不造codeFlows/runtimeReproduced。合法一致不删除其他覆盖缺口。安全结论仍需人工复核。

## 切片与验证

1. 先抽取现有finding verification时间/工具pool为可共享私有helper；nil pool的兼容wrapper保留原单finding流程，零行为改动，策略仍v44。原fresh/source/独立模型/usage/checkpoint/停止测试须通过，shared pool耗尽/取消/余量测试证明不能双重分配。
2. 命题复核parser/provenance/consistency与server-owned字段，单元验证true/false/unknown交叉结果、注入清除、首审/跨复核/坏快照/重复/非源码/预算数据拒绝。
3. runner与共享pool接入（策略v45），真实SDK错误首审rejected+safeClaim经fresh支持true形成disagreed；未知保留、关闭无请求、预算耗尽无额外模型、checkpoint失败0后续HTTP。跨仓库/多组namespace、持久化与SARIF验证，原finding优先和model/token记录保持。
4. UI/types/配置说明/嵌入构建与实际360px/键盘/源码导航验证；冻结源码独立review，然后原模型/预算已知负例实际回归，记录语义/来源/判断/成本/失败。不将synthetic disagreement视为真实准确率，不把所有例completed当新门槛。

每切片提交推送文档和源码证据；单切片不标整体完成。回退新增runner/UI字段时历史JSON仍可读，pool基础可独立保留，原配置键及首审数据不迁移。AER-001保持open直至实际需求逐项证据审计。

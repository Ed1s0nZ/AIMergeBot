# F5 首切片回归协议

使用既有corpus-acceptance-v1 case203（JavaScript→Python身份绑定删除/头覆盖）作分项复核定向回归，不称新隔离验收或真实PR。固定提交/外部gold与真实模型配置由现有CLI维护；新目录保留所有trace、checkpoint、usage和失败。检查四项实际状态/理由/来源、顶层是否匹配、两侧/下游是否重新读取及附加断言是否过强。预算不提高，不清理失败或覆盖notes。一次正例不是整体准确率。

全量/相关Go/race与组件浏览器检查通过后提交F4固定代码，再运行CLI使metadata绑定不可变代码。真实PR验收仍须真实来源+固定SHA+人工机制标注与正常/修复对照；语料未准备不能宣称完成。后续调优依据本例时仅叫回归。

真实历史样本选取审查（2026-10-06）：Express官方[GHSA-rv95-896h-c2vc](https://github.com/expressjs/express/security/advisories/GHSA-rv95-896h-c2vc)明确有初修、功能回归修复、改进修复三步，不能把任何标为fix的提交自动当完整无风险真值。其引用的[PR5539](https://github.com/expressjs/express/pull/5539)当前为closed/deleted标题，不能冒称已合并真实修复PR。候选官方提交[0867302ddbde0e9463d0564fea5861feb708c2dd](https://github.com/expressjs/express/commit/0867302ddbde0e9463d0564fea5861feb708c2dd)、[0b746953c4bd8e377123527db11f9cd866e39f94](https://github.com/expressjs/express/commit/0b746953c4bd8e377123527db11f9cd866e39f94)仍需审查父提交、完整源码与回归条件；目前未冻结语料/运行评测，不计入通过率。不能把修复反向生成的改动叫历史真实PR。

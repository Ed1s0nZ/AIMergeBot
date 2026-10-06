# v31 真实历史提交回归（基础设施失败）

代码0b7a4ad，固定Express官方修复提交0b746953c4bd8e377123527db11f9cd866e39f94及真实父提交4f0f6cc67d531431c096ea006c2191b92931bbc3。官方GHSA-rv95-896h-c2vc列出这一改进修复。运行前检查完整改动、res.location源码与测试声明，未执行仓库代码或测试。语料在 /Users/worker/.codex/evaluation-artifacts/aimangebot/real-history-v31-20261006/corpus.json，freeze.json含SHA256与先验真值。真实提交集合，不冒称已合并PR；公开历史/模型训练熟悉度未知，不是代表性或已证明未见过的验收集。

实际生产grouped入口，原deepseek-chat配置、原预算、timeout240。20,853ms，236,854 tokens（primary234,102、synthesis2,752），16调用，usage完整，价格未配置不估成本。1组weight3。结果incomplete、0finding，不能计为true negative。主模型初期4次成功源码读取，随后原生Git ls-tree/cat-file/diff反复exit69；系统终端直接返回Xcode许可尚未同意。模型继续重试直到主生成失败；综合模型保留incomplete，未将零finding包装为通过。不能把综合文本“全部未审计”当精确事实，部分源码实际已读取。

归档 /Users/worker/.codex/evaluation-artifacts/aimangebot/real-history-v31-20261006/receipts，逐文件SHA256复读验证。原结果保留，不覆盖、清洗或重标为模型误报/漏报。恢复合法可用Git后若复跑必须用新输出目录；原失败属于基础设施不可用，仍统计真实已消耗tokens。

新发现：固定源执行器全局不可用时，下一次付费模型请求应尽早停止并保留已读取证据和已提交finding；普通不存在文件/搜索无匹配仍可继续修复，不能混为全局不可用。现有方法说明混入coverage、跨仓库相关性门禁、真实/隔离成组质量验收仍待处理。Xcode许可需要用户本人完成；本次未代为同意或绕过许可。

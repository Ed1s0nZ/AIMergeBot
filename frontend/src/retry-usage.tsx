import {ModelUsagePanel} from "./model-usage";
import type {RetryChainUsage} from "./api";
export function RetryUsagePanel({chain}:{chain?:RetryChainUsage}){
 if(!chain||chain.attempts.length<2)return null;
 return <section aria-label="自动重试链用量"><h2>自动重试链合计</h2><p className="muted">同一原任务与自动重试共用停止阈值。以下合计不会重复计算检查点；手动重审和选文件补审属于新任务。</p><ul>{chain.attempts.map(a=><li key={a.run_id}><a href={`#/runs/${a.run_id}`}>任务 #{a.run_id}</a> · {a.usage.calls} 次模型请求 · 输入 {a.usage.prompt_tokens.toLocaleString()} / 输出 {a.usage.completion_tokens.toLocaleString()}{a.usage.unknown_calls>0?` · ${a.usage.unknown_calls} 次用量未知`:""}</li>)}</ul><ModelUsagePanel usage={chain.usage} retryChain/></section>;
}

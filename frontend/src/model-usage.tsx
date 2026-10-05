import type {ModelUsage} from "./api";
export function ModelUsagePanel({usage}:{usage?:ModelUsage}) {
 if(!usage)return null;
 const reasons:Record<string,string>={price_not_configured:"未配置单价或币种",verification_price_missing:"独立复核模型尚未配置单价",no_reported_usage:"没有可计价的报告用量"};
 const names:Record<string,string>={primary:"主审",verification:"独立复核",synthesis:"跨组汇总",diagram:"时序图"};
 return <section className="panel trace"><h2>模型用量与成本</h2>
 <p>{usage.complete?"报告用量完整":"报告用量不完整 · 以下仅为已知部分"}</p>
 <p>输入 {usage.prompt_tokens.toLocaleString()} tokens · 输出 {usage.completion_tokens.toLocaleString()} tokens · {usage.calls} 次请求{usage.unknown_calls>0?` · ${usage.unknown_calls} 次用量未知`:""}</p>
 <p>{usage.estimated_cost==null?`费用未估算 · ${reasons[usage.estimate_unavailable_reason||""]||"价格信息不足"}`:`按已报告用量估算 ${usage.estimated_cost.toFixed(6)} ${usage.currency||""}`}</p>
 {usage.max_tokens>0&&<p className="muted">停止阈值 {usage.max_tokens.toLocaleString()} tokens，单次请求可能超额。</p>}
 <ul>{usage.stages.map(s=><li key={s.stage}>{names[s.stage]||s.stage}：{s.calls} 次 · 输入 {s.prompt_tokens.toLocaleString()} / 输出 {s.completion_tokens.toLocaleString()}{s.unknown_calls>0?` · ${s.unknown_calls} 次未知`:""}</li>)}</ul>
 <p className="muted">当前审计尝试的服务商报告量，失败请求可能有未报告用量。单价随任务固定，未计缓存折扣，不代表实际账单或重试链总费用。</p>
 </section>;
}

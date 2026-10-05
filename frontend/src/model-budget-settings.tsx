import type {Settings} from "./api";
export function ModelBudgetSettingsFields({value,onChange}:{value:Settings["model_budget"];onChange:(value:Settings["model_budget"])=>void}) {
 return <fieldset className="git-audit-settings"><legend>模型用量与成本</legend>
 <label>每次审计 token 停止阈值<input type="number" min={0} max={10000000} step={1000} value={value.max_tokens} onChange={e=>onChange({...value,max_tokens:Number(e.target.value)})}/><small>0 表示不启用。主审、分组、复核和时序图共用；达到阈值后停止新请求，单次请求可能超额。用量缺失时停止后续请求，已发现的问题保留。</small></label>
 <label>估算币种<select value={value.currency||""} onChange={e=>onChange({...value,currency:e.target.value})}><option value="">不估算费用</option><option value="CNY">CNY</option><option value="USD">USD</option></select></label>
 <label>每百万输入 token 单价<input type="number" min={0} max={1000000} step="any" value={value.input_price_per_million||0} onChange={e=>onChange({...value,input_price_per_million:Number(e.target.value)})}/></label>
 <label>每百万输出 token 单价<input type="number" min={0} max={1000000} step="any" value={value.output_price_per_million||0} onChange={e=>onChange({...value,output_price_per_million:Number(e.target.value)})}/><small>填写服务商对应模型价格。按报告用量估算，未计算缓存折扣等差异，不代表实际账单。</small></label>
 <label className="check"><input type="checkbox" checked={value.verification_pricing_configured||false} onChange={e=>onChange({...value,verification_pricing_configured:e.target.checked})}/>单独配置复核模型价格</label>
 <label>复核每百万输入 token 单价<input type="number" disabled={!value.verification_pricing_configured} min={0} max={1000000} step="any" value={value.verification_input_price_per_million||0} onChange={e=>onChange({...value,verification_input_price_per_million:Number(e.target.value)})}/></label>
 <label>复核每百万输出 token 单价<input type="number" disabled={!value.verification_pricing_configured} min={0} max={1000000} step="any" value={value.verification_output_price_per_million||0} onChange={e=>onChange({...value,verification_output_price_per_million:Number(e.target.value)})}/><small>币种与主审一致。使用不同复核模型但未填写价格时，总费用不予估算。</small></label>
 </fieldset>;
}

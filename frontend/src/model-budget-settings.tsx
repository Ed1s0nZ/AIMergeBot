export function ModelBudgetSettingsFields({value,onChange}:{value:{max_tokens:number};onChange:(value:{max_tokens:number})=>void}) {
 return <label>每次审计 token 停止阈值<input type="number" min={0} max={10000000} step={1000} value={value.max_tokens} onChange={e=>onChange({max_tokens:Number(e.target.value)})}/><small>0 表示不启用。主审、分组、复核和时序图共用；达到阈值后停止新请求，单次请求可能超额。用量缺失时停止后续请求，已发现的问题保留。</small></label>;
}

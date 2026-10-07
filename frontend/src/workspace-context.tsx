import { useState } from "react";
import { type Project } from "./api";
import { ContextRepositories } from "./context-repositories";
import { Empty, ErrorBox } from "./components";
import { Heading, useResource } from "./page-utils";

export function WorkspaceContext() {
 const projects=useResource<{items:Project[]}>("/projects");
 const [selected,setSelected]=useState<number|null>(null);
 const visible=!projects.loading&&!projects.error?projects.data?.items:undefined;
 const project=visible?.find(p=>p.id===selected);
 return <>
  <Heading title="关联仓库" sub="按目标项目配置可用于审计的关联仓库及固定提交。保存影响后续审计；历史任务实际使用的仓库和 SHA 请在任务详情查看。" action={<button onClick={()=>{setSelected(null);void projects.load();}}>刷新</button>}/>
  <ErrorBox error={projects.error}/>
  {projects.loading?<Empty>正在加载…</Empty>:visible?.length?<section className="panel"><label>目标项目<select value={selected??""} onChange={e=>setSelected(e.target.value?Number(e.target.value):null)}><option value="">请选择项目</option>{visible.map(p=><option key={p.id} value={p.id}>{p.name}{p.enabled?"":"（已禁用）"}</option>)}</select></label><p>关联授权不授予用户项目权限；审计发起者仍需具备目标、来源及关联项目的访问权限。</p></section>:!projects.error?<Empty>暂无项目，请先在项目页添加。</Empty>:null}
  {project&&visible&&<ContextRepositories key={project.id} project={project} projects={visible} onClose={()=>setSelected(null)}/>}
 </>;
}


import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';

let refreshGeneration=0;
let lastReleaseError='';

function renderProjects(){
 $('#projects').innerHTML=state.projects.map(project=>
  '<button data-project="'+escapeHTML(project.id)+'" class="'+(project.id===state.project?'active':'')+'" '+(project.id===state.project?'aria-current="page"':'')+'>'+escapeHTML(project.name)+'<small>'+escapeHTML(project.id)+'</small></button>'
 ).join('');
}

export async function refreshData(){
 const generation=++refreshGeneration;
 const previous=state.project;
 const projects=await api('/api/projects',{timeoutMs:5000});
 if(generation!==refreshGeneration)return;

 state.projects=Array.isArray(projects)?projects:[];
 if(!state.project||!state.projects.some(project=>project.id===state.project))state.project=state.projects[0]?.id||null;
 const current=state.project;

 // Project navigation and connection state are core bootstrap data. Render them
 // immediately; Release decoration/sync must never make the whole UI look
 // disconnected.
 renderProjects();
 $('#connection').textContent='服务已连接';

 if(previous!==current){
  state.releases=[];
  window.dispatchEvent(new CustomEvent('project-changed'));
  window.dispatchEvent(new CustomEvent('data-refreshed'));
 }
 if(!current){
  if(previous===current)window.dispatchEvent(new CustomEvent('data-refreshed'));
  return;
 }

 try{
  const releases=await api('/api/projects/'+encodeURIComponent(current)+'/releases',{timeoutMs:10000});
  if(generation!==refreshGeneration||current!==state.project)return;
  state.releases=Array.isArray(releases)?releases:[];
  lastReleaseError='';
  window.dispatchEvent(new CustomEvent('data-refreshed'));
 }catch(error){
  if(generation!==refreshGeneration||current!==state.project)return;
  console.warn('ILS release refresh deferred:',error);
  // Keep previously rendered Release data for the same project. On a project
  // change state.releases was already cleared above, so stale cross-project
  // cards cannot leak into the new selection.
  window.dispatchEvent(new CustomEvent('data-refreshed'));
  if(error.message!==lastReleaseError){
   lastReleaseError=error.message;
   notice('项目列表已连接，但 Release 数据暂时不可用：'+error.message,'error');
  }
 }
}

async function createProject(form){
 const button=form.querySelector('[type=submit]');
 button.disabled=true;
 const old=button.textContent;
 button.textContent='处理中…';
 form.querySelector('.form-error').textContent='';
 try{
  const project=await api('/api/projects',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(Object.fromEntries(new FormData(form)))
  });
  state.project=project.id;
  state.releases=[];
  form.closest('dialog').close();
  form.reset();
  window.dispatchEvent(new CustomEvent('project-changed'));
  window.dispatchEvent(new CustomEvent('data-refreshed'));
  await refreshData();
 }catch(error){
  form.querySelector('.form-error').textContent=error.message;
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
}

export function initProjects(){
 $('#projects').addEventListener('click',async event=>{
  const button=event.target.closest('[data-project]');
  if(!button)return;
  const next=button.dataset.project;
  if(next===state.project)return;
  state.project=next;
  state.releases=[];
  renderProjects();
  window.dispatchEvent(new CustomEvent('project-changed'));
  window.dispatchEvent(new CustomEvent('data-refreshed'));
  try{
   await refreshData();
  }catch(error){
   $('#connection').textContent='连接中断';
   notice(error.message,'error');
  }
 });

 $('#new-project').onclick=()=>{
  if(needAdmin())$('#project-dialog').showModal();
 };
 $('#project-form').onsubmit=event=>{
  event.preventDefault();
  createProject(event.target);
 };

 refreshData().catch(error=>{
  $('#connection').textContent='连接失败';
  notice(error.message,'error');
 });

 setInterval(()=>{
  if(document.hidden)return;
  refreshData().catch(()=>{$('#connection').textContent='连接中断';});
 },20000);
}

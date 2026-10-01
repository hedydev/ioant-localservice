
import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';

function renderProjects(){
 $('#projects').innerHTML=state.projects.map(project=>
  '<button data-project="'+escapeHTML(project.id)+'" class="'+(project.id===state.project?'active':'')+'" '+(project.id===state.project?'aria-current="page"':'')+'>'+escapeHTML(project.name)+'<small>'+escapeHTML(project.id)+'</small></button>'
 ).join('');
}

export async function refreshData(){
 const previous=state.project;
 const projects=await api('/api/projects');
 state.projects=projects;
 if(!state.project||!projects.some(project=>project.id===state.project))state.project=projects[0]?.id||null;

 const current=state.project;
 const releases=current?await api('/api/projects/'+current+'/releases'):[];
 if(current!==state.project)return;

 state.releases=releases;
 renderProjects();
 if(previous!==state.project)window.dispatchEvent(new CustomEvent('project-changed'));
 window.dispatchEvent(new CustomEvent('data-refreshed'));
 $('#connection').textContent='服务已连接';
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
  form.closest('dialog').close();
  form.reset();
  await refreshData();
  window.dispatchEvent(new CustomEvent('project-changed'));
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
  state.project=button.dataset.project;
  window.dispatchEvent(new CustomEvent('project-changed'));
  try{
   await refreshData();
   notice('');
  }catch(error){
   notice(error.message);
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
  notice(error.message);
 });

 setInterval(()=>{
  if(document.hidden)return;
  refreshData().catch(()=>{$('#connection').textContent='连接中断';});
 },20000);
}

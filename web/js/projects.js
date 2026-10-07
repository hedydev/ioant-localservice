import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';
import {readRoute,writeRoute,projectViews} from './router.js';

let projectsGeneration=0;
let releaseGeneration=0;
let lastReleaseError='';
let renderedProjectsSignature='';

function projectsSignature(projects){
 return projects.map(project=>project.id+'\u0000'+project.name).join('\u0001');
}

function updateProjectSelection(){
 document.querySelectorAll('#projects [data-project]').forEach(button=>{
  const active=button.dataset.project===state.project&&state.view!=='services';
  button.classList.toggle('active',active);
  if(active)button.setAttribute('aria-current','page');
  else button.removeAttribute('aria-current');
 });
}

function renderProjects({force=false}={}){
 const signature=projectsSignature(state.projects);
 if(force||signature!==renderedProjectsSignature){
  $('#projects').innerHTML=state.projects.map(project=>
   '<button data-project="'+escapeHTML(project.id)+'">'+escapeHTML(project.name)+'<small>'+escapeHTML(project.id)+'</small></button>'
  ).join('');
  renderedProjectsSignature=signature;
 }
 updateProjectSelection();
}

export async function loadProjectReleases(project=state.project){
 const generation=++releaseGeneration;
 if(!project){
  state.releases=[];
  window.dispatchEvent(new CustomEvent('data-refreshed'));
  return;
 }
 try{
  const releases=await api('/api/projects/'+encodeURIComponent(project)+'/releases',{timeoutMs:10000});
  if(generation!==releaseGeneration||project!==state.project)return;
  state.releases=Array.isArray(releases)?releases:[];
  lastReleaseError='';
  window.dispatchEvent(new CustomEvent('data-refreshed'));
 }catch(error){
  if(generation!==releaseGeneration||project!==state.project)return;
  console.warn('ILS release refresh deferred:',error);
  window.dispatchEvent(new CustomEvent('data-refreshed'));
  if(error.message!==lastReleaseError){
   lastReleaseError=error.message;
   notice('项目已切换，但 Release 数据暂时不可用：'+error.message,'error');
  }
 }
}

export async function selectProject(projectID,{updateRoute=true,replaceRoute=false,view=null}={}){
 if(!state.projects.some(project=>project.id===projectID))return false;
 const targetView=projectViews.has(view)?view:(state.view==='services'?'overview':(projectViews.has(state.view)?state.view:'overview'));
 if(projectID===state.project){
  updateProjectSelection();
  if(updateRoute)writeRoute(projectID,targetView,{replace:replaceRoute});
  return true;
 }
 state.project=projectID;
 state.releases=[];
 updateProjectSelection();
 if(updateRoute)writeRoute(projectID,targetView,{replace:replaceRoute});
 window.dispatchEvent(new CustomEvent('project-changed',{detail:{project:projectID}}));
 window.dispatchEvent(new CustomEvent('data-refreshed'));
 await loadProjectReleases(projectID);
 return true;
}

export async function refreshData(){
 const generation=++projectsGeneration;
 const previous=state.project;
 const projects=await api('/api/projects',{timeoutMs:5000});
 if(generation!==projectsGeneration)return;

 state.projects=Array.isArray(projects)?projects:[];
 const route=readRoute();
 const routedProject=route.kind==='project'&&route.project&&state.projects.some(project=>project.id===route.project)
  ?route.project
  :null;
 if(routedProject)state.project=routedProject;
 else if(!state.project||!state.projects.some(project=>project.id===state.project))state.project=state.projects[0]?.id||null;
 const current=state.project;

 renderProjects();
 $('#connection').textContent='服务已连接';

 if(previous!==current){
  state.releases=[];
  window.dispatchEvent(new CustomEvent('project-changed',{detail:{project:current}}));
  window.dispatchEvent(new CustomEvent('data-refreshed'));
 }

 if(route.kind==='project'&&current&&(route.legacy||route.project!==current)){
  writeRoute(current,projectViews.has(route.view)?route.view:'overview',{replace:true});
 }
 if(!current){
  if(previous===current)window.dispatchEvent(new CustomEvent('data-refreshed'));
  return;
 }
 await loadProjectReleases(current);
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
  state.projects=[...state.projects,project];
  renderProjects({force:true});
  form.closest('dialog').close();
  form.reset();
  await selectProject(project.id,{view:'overview'});
 }catch(error){
  form.querySelector('.form-error').textContent=error.message;
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
}

export function initProjects(){
 $('#projects').addEventListener('click',event=>{
  const button=event.target.closest('[data-project]');
  if(!button)return;
  const next=button.dataset.project;
  const targetView=state.view==='services'?'overview':state.view;
  void selectProject(next,{view:targetView});
 });

 $('#new-project').onclick=()=>{
  if(needAdmin())$('#project-dialog').showModal();
 };
 $('#project-form').onsubmit=event=>{
  event.preventDefault();
  void createProject(event.target);
 };

 refreshData().catch(error=>{
  $('#connection').textContent='连接失败';
  notice(error.message,'error');
 });

 window.addEventListener('view-changed',updateProjectSelection);
 setInterval(()=>{
  if(document.hidden)return;
  refreshData().catch(()=>{$('#connection').textContent='连接中断';});
 },20000);
}

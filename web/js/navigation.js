import {$,state} from './core.js';
import {readRoute,writeRoute,validViews} from './router.js';
import {selectProject} from './projects.js';

const adminViews=new Set(['services','builds','automation']);
const labels={overview:'项目概览',builds:'构建与发布',releases:'版本历史',ios:'iOS 发布',automation:'自动化 / API'};

function updateHeading(){
 if(state.view==='services')return;
 const project=state.projects.find(item=>item.id===state.project);
 $('#project-title').textContent=project?.name||'安装包分发';
 $('#project-subtitle').textContent=project?(project.id+' / '+labels[state.view]):'选择项目后查看构建与发布信息。';
}

function updateNavigationSelection(){
 const global=state.view==='services';
 $('#global-services-button')?.classList.toggle('active',global);
 if(global)$('#global-services-button')?.setAttribute('aria-current','page');
 else $('#global-services-button')?.removeAttribute('aria-current');

 document.querySelectorAll('#projects [data-project]').forEach(button=>{
  const active=!global&&button.dataset.project===state.project;
  button.classList.toggle('active',active);
  if(active)button.setAttribute('aria-current','page');
  else button.removeAttribute('aria-current');
 });
}

function updateGlobalNavigationVisibility(){
 const globalNav=$('#global-services-nav');
 const globalLabel=globalNav?.previousElementSibling;
 const projectLabel=$('#projects')?.previousElementSibling;
 const visible=state.admin;

 [globalLabel,globalNav].forEach(element=>{
  if(!element)return;
  element.hidden=!visible;
  element.style.display=visible?'':'none';
 });
 if(projectLabel)projectLabel.style.marginTop=visible?'28px':'0';
}

function updatePageShell(){
 const global=state.view==='services';
 const projectHeading=document.querySelector('main > .page-heading');
 const workspaceTabs=$('#workspace-tabs');
 if(projectHeading){
  projectHeading.hidden=global;
  projectHeading.style.display=global?'none':'';
 }
 if(workspaceTabs){
  workspaceTabs.hidden=global;
  workspaceTabs.style.display=global?'none':'';
 }
}

export function activateView(requested,{updateRoute=true,replaceRoute=false}={}){
 let view=validViews.has(requested)?requested:'overview';
 if(adminViews.has(view)&&!state.admin)view='overview';
 state.view=view;

 document.querySelectorAll('[data-view-panel]').forEach(panel=>{
  panel.hidden=panel.dataset.viewPanel!==view;
 });
 document.querySelectorAll('#workspace-tabs [data-view]').forEach(button=>{
  button.setAttribute('aria-pressed',String(button.dataset.view===view));
 });
 updatePageShell();
 updateHeading();
 updateNavigationSelection();

 if(updateRoute){
  if(view==='services')writeRoute(null,'services',{replace:replaceRoute});
  else if(state.project)writeRoute(state.project,view,{replace:replaceRoute});
 }
 window.dispatchEvent(new CustomEvent('view-changed',{detail:{view}}));
 return view;
}

async function applyRoute({canonicalize=false}={}){
 const route=readRoute();
 if(route.kind==='project'&&route.project&&route.project!==state.project){
  await selectProject(route.project,{updateRoute:false});
 }
 const requested=route.kind==='global'?'services':route.view;
 const actual=activateView(requested,{updateRoute:false});

 if(route.kind==='global'){
  if(actual==='services'){
   if(canonicalize||route.legacy)writeRoute(null,'services',{replace:true});
  }else if(state.project){
   writeRoute(state.project,actual,{replace:true});
  }
  return;
 }
 if(state.project&&(canonicalize||route.legacy||route.project!==state.project||route.view!==actual)){
  writeRoute(state.project,actual,{replace:true});
 }
}

export function initNavigation(){
 updateGlobalNavigationVisibility();

 $('#workspace-tabs').addEventListener('click',event=>{
  const button=event.target.closest('[data-view]');
  if(button)activateView(button.dataset.view);
 });
 $('#global-services-nav').addEventListener('click',event=>{
  const button=event.target.closest('[data-global-view]');
  if(button)activateView(button.dataset.globalView);
 });
 document.addEventListener('click',event=>{
  const button=event.target.closest('[data-go-view]');
  if(button)activateView(button.dataset.goView);
 });
 window.addEventListener('hashchange',()=>{void applyRoute();});
 window.addEventListener('data-refreshed',()=>{
  updateHeading();
  updateNavigationSelection();
 });
 window.addEventListener('project-changed',()=>{
  updateHeading();
  updateNavigationSelection();
 });
 window.addEventListener('admin-loaded',()=>{
  updateGlobalNavigationVisibility();
  void applyRoute({canonicalize:true});
 });
 window.addEventListener('admin-cleared',()=>{
  updateGlobalNavigationVisibility();
  void applyRoute({canonicalize:true});
 });
 void applyRoute({canonicalize:true});
}

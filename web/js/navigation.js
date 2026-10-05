
import {$,state} from './core.js';

const validViews=new Set(['services','overview','builds','releases','ios','automation']);
const adminViews=new Set(['services','builds','automation']);
const labels={overview:'项目概览',builds:'构建与发布',releases:'版本历史',ios:'iOS 发布',automation:'自动化 / API'};

function requestedView(){
 const value=location.hash.replace(/^#/,'');
 return validViews.has(value)?value:'overview';
}

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

export function activateView(requested,{updateHash=true}={}){
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

 if(updateHash&&location.hash!=='#'+view)history.replaceState(null,'','#'+view);
 window.dispatchEvent(new CustomEvent('view-changed',{detail:{view}}));
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
 $('#projects').addEventListener('click',event=>{
  const button=event.target.closest('[data-project]');
  if(button&&state.view==='services')activateView('overview');
 });
 document.addEventListener('click',event=>{
  const button=event.target.closest('[data-go-view]');
  if(button)activateView(button.dataset.goView);
 });
 window.addEventListener('hashchange',()=>activateView(requestedView(),{updateHash:false}));
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
  activateView(requestedView());
 });
 window.addEventListener('admin-cleared',()=>{
  updateGlobalNavigationVisibility();
  if(adminViews.has(state.view))activateView('overview');
  else activateView(state.view);
 });
 activateView(requestedView(),{updateHash:false});
}

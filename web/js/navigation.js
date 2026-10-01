
import {$,state} from './core.js';

const validViews=new Set(['overview','builds','releases','ios','automation']);
const adminViews=new Set(['builds','ios','automation']);
const labels={overview:'项目概览',builds:'构建与发布',releases:'版本历史',ios:'iOS / TestFlight',automation:'自动化 / API'};

function requestedView(){
 const value=location.hash.replace(/^#/,'');
 return validViews.has(value)?value:'overview';
}

function updateHeading(){
 const project=state.projects.find(item=>item.id===state.project);
 $('#project-title').textContent=project?.name||'安装包分发';
 $('#project-subtitle').textContent=project?(project.id+' / '+labels[state.view]):'选择项目后查看构建与发布信息。';
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
 updateHeading();

 if(updateHash&&location.hash!=='#'+view)history.replaceState(null,'','#'+view);
 window.dispatchEvent(new CustomEvent('view-changed',{detail:{view}}));
}

export function initNavigation(){
 $('#workspace-tabs').addEventListener('click',event=>{
  const button=event.target.closest('[data-view]');
  if(button)activateView(button.dataset.view);
 });
 document.addEventListener('click',event=>{
  const button=event.target.closest('[data-go-view]');
  if(button)activateView(button.dataset.goView);
 });
 window.addEventListener('hashchange',()=>activateView(requestedView(),{updateHash:false}));
 window.addEventListener('data-refreshed',updateHeading);
 window.addEventListener('project-changed',updateHeading);
 window.addEventListener('admin-loaded',()=>activateView(requestedView()));
 window.addEventListener('admin-cleared',()=>{
  if(adminViews.has(state.view))activateView('overview');
  else activateView(state.view);
 });
 activateView(requestedView(),{updateHash:false});
}

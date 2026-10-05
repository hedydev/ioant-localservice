import {$,state,api,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';
import {renderReleaseCard} from './release-ui.js';

function renderPublicIOS(){
 const releases=state.releases.filter(item=>item.platform==='ios').slice(0,5);
 const container=$('#ios-install-releases');

 if(!state.project){
  container.innerHTML='<div class="empty"><strong>请先选择项目</strong>选择项目后，这里会显示它的 iOS 发布。</div>';
  return;
 }

 if(!releases.length){
  container.innerHTML='<div class="empty"><strong>还没有 iOS 发布</strong>Ad Hoc 与 TestFlight 发布都会出现在这里。</div>';
  return;
 }

 container.innerHTML=releases.map(release=>renderReleaseCard(release)).join('');
}

async function pollSigningJob(){
 if(!state.admin||!state.job)return;
 try{
  const job=await api('/api/signing/'+state.job);
  $('#sign-state').textContent={
   running:'Mac 正在签名…',
   succeeded:'签名完成，安装包已发布。',
   failed:job.error||'签名失败'
  }[job.status]||job.status;
  if(job.status!=='running'){
   state.job=null;
   $('#sign-button').disabled=false;
   await refreshData();
  }
 }catch(error){
  $('#sign-state').textContent=error.message;
  state.job=null;
  $('#sign-button').disabled=false;
 }
}

function resetIOS(){
 state.job=null;
 $('#sign-state').textContent='';
 $('#sign-button').disabled=false;
 renderPublicIOS();
}

export function initIOS(){
 $('#sign-button').onclick=async()=>{
  if(!needAdmin())return;
  if(!state.project)return notice('请先创建项目');
  const button=$('#sign-button');
  button.disabled=true;
  try{
   const job=await api('/api/projects/'+state.project+'/signing',{method:'POST'});
   state.job=job.id;
   $('#sign-state').textContent='Mac 正在导出并签名，完成后自动发布。';
  }catch(error){
   $('#sign-state').textContent=error.message;
   button.disabled=false;
  }
 };

 window.addEventListener('data-refreshed',renderPublicIOS);
 window.addEventListener('project-changed',resetIOS);
 window.addEventListener('admin-cleared',resetIOS);
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='ios')renderPublicIOS();
 });

 setInterval(()=>{
  if(document.hidden)return;
  pollSigningJob();
 },3000);

 renderPublicIOS();
}

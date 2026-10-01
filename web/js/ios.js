
import {$,state,api,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';

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
 $('#devices-list').textContent='';
 $('#devices-list').hidden=true;
 $('#sign-state').textContent='';
 $('#sign-button').disabled=false;
}

export function initIOS(){
 $('#devices-button').onclick=async()=>{
  if(!needAdmin())return;
  try{
   const devices=await api('/api/devices');
   $('#devices-list').hidden=false;
   $('#devices-list').textContent=devices.length
    ?devices.map(device=>device.product+' / iOS '+device.version+'\nUDID: '+device.udid+'\n待 Apple 团队登记 · 设备身份尚未认证').join('\n\n')
    :'还没有收集到设备。';
  }catch(error){
   notice(error.message);
  }
 };

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

 window.addEventListener('project-changed',resetIOS);
 window.addEventListener('admin-cleared',resetIOS);

 setInterval(()=>{
  if(document.hidden)return;
  pollSigningJob();
 },3000);

 if(new URLSearchParams(location.search).get('enrollment')==='collected'){
  notice('设备信息已收集。请由管理员登记到 Apple 团队并更新分发描述文件，再请求签名。');
 }
}

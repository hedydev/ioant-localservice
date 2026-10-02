import {$,state,api,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';
import {renderReleaseCard} from './release-ui.js';

let gatewayConfigured=false;
let gatewaySyncing=false;

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

async function checkEnrollmentAvailability(){
 try{
  const health=await api('/api/health');
  const link=$('#enroll-device-link');
  if(health.public_enrollment_url){
   link.href=health.public_enrollment_url;
   link.removeAttribute('aria-disabled');
   link.classList.remove('disabled-link');
   $('#enrollment-availability').textContent='公网设备登记已启用。iPhone / iPad 可直接访问 OTA Gateway，不需要和这台 Mac 位于同一局域网。';
  }else if(health.ota_configured){
   link.href='/api/devices/enroll.mobileconfig';
   link.removeAttribute('aria-disabled');
   link.classList.remove('disabled-link');
   $('#enrollment-availability').textContent='本地 HTTPS 设备登记入口已启用。请使用 iPhone / iPad 的 Safari 打开此页面并安装登记描述文件。';
  }else{
   link.removeAttribute('href');
   link.setAttribute('aria-disabled','true');
   link.classList.add('disabled-link');
   $('#enrollment-availability').textContent='当前 ILS 尚未配置公网 OTA Gateway 或受 iPhone 信任的本地 HTTPS public-url，因此设备登记暂不可用。';
  }
 }catch(error){
  $('#enrollment-availability').textContent='无法读取设备登记状态：'+error.message;
 }
}

function sourceName(source){
 return source==='public_ota_gateway'?'公网 OTA Gateway':source==='local_ils'?'本地 ILS':'历史登记';
}

async function loadDevices(){
 const devices=await api('/api/devices');
 $('#devices-list').hidden=false;
 $('#devices-list').textContent=devices.length
  ?devices.map(device=>
    device.product+' / iOS '+device.version+
    '\nUDID: '+device.udid+
    '\n来源: '+sourceName(device.source)+
    '\n状态: '+(device.status||'pending_apple_registration')
   ).join('\n\n')
  :'还没有收集到设备。';
}

async function refreshOTAGatewayStatus(){
 if(!state.admin)return;
 try{
  const status=await api('/api/ota-gateway/config');
  gatewayConfigured=!!status.configured;
  if(!status.configured){
   $('#ota-gateway-status').textContent='尚未配置 OTA Gateway。请先运行部署脚本。';
   return status;
  }
  const parts=[
   status.connected?'已连接':'已配置，尚未验证',
   status.public_url||'',
   status.ssh_user&&status.ssh_host?(status.ssh_user+'@'+status.ssh_host):'',
   status.remote_root||''
  ].filter(Boolean);
  if(status.last_error)parts.push('最近错误：'+status.last_error);
  $('#ota-gateway-status').textContent=parts.join(' · ');
  return status;
 }catch(error){
  gatewayConfigured=false;
  $('#ota-gateway-status').textContent='无法读取 OTA Gateway 状态：'+error.message;
  return null;
 }
}

async function syncGatewayDevices({silent=false}={}){
 if(!state.admin||gatewaySyncing)return null;
 if(!gatewayConfigured){
  if(!silent)notice('OTA Gateway 尚未配置。','error');
  return null;
 }
 gatewaySyncing=true;
 const button=$('#ota-sync-devices');
 if(button)button.disabled=true;
 try{
  const report=await api('/api/ota-gateway/sync-devices',{method:'POST'});
  if(!$('#devices-list').hidden)await loadDevices();
  await refreshOTAGatewayStatus();
  if(!silent)notice('公网设备同步完成：待处理 '+report.pending+'，新导入 '+report.imported+'，已确认 '+report.acked+(report.failed?'，失败 '+report.failed:'')+'。');
  return report;
 }catch(error){
  if(!silent)notice(error.message,'error');
  return null;
 }finally{
  gatewaySyncing=false;
  if(button)button.disabled=false;
 }
}

async function syncGatewayArtifacts(){
 if(!needAdmin())return;
 if(!gatewayConfigured)return notice('OTA Gateway 尚未配置。','error');
 const button=$('#ota-sync-artifacts');
 button.disabled=true;
 try{
  const report=await api('/api/ota-gateway/sync-artifacts',{method:'POST'});
  await refreshData();
  notice('Ad Hoc OTA 同步完成：成功 '+report.artifacts+(report.failed?'，失败 '+report.failed:'')+'。',report.failed?'error':'info');
 }catch(error){
  notice(error.message,'error');
 }finally{
  button.disabled=false;
 }
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
 gatewayConfigured=false;
 gatewaySyncing=false;
 $('#devices-list').textContent='';
 $('#devices-list').hidden=true;
 $('#sign-state').textContent='';
 $('#sign-button').disabled=false;
 if($('#ota-gateway-status'))$('#ota-gateway-status').textContent='正在读取 OTA Gateway 配置…';
 renderPublicIOS();
}

export function initIOS(){
 $('#devices-button').onclick=async()=>{
  if(!needAdmin())return;
  try{
   await loadDevices();
  }catch(error){
   notice(error.message,'error');
  }
 };

 $('#ota-sync-devices').onclick=()=>syncGatewayDevices();
 $('#ota-sync-artifacts').onclick=()=>syncGatewayArtifacts();
 $('#ota-gateway-check').onclick=async()=>{
  if(!needAdmin())return;
  const button=$('#ota-gateway-check');
  button.disabled=true;
  try{
   const status=await api('/api/ota-gateway/check',{method:'POST'});
   gatewayConfigured=!!status.configured;
   await refreshOTAGatewayStatus();
   notice('OTA Gateway 连接正常。');
  }catch(error){
   await refreshOTAGatewayStatus();
   notice(error.message,'error');
  }finally{
   button.disabled=false;
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

 window.addEventListener('data-refreshed',renderPublicIOS);
 window.addEventListener('project-changed',resetIOS);
 window.addEventListener('admin-cleared',resetIOS);
 window.addEventListener('admin-loaded',()=>{
  if(state.view==='ios')refreshOTAGatewayStatus();
 });
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='ios'){
   renderPublicIOS();
   checkEnrollmentAvailability();
   if(state.admin)refreshOTAGatewayStatus();
  }
 });

 setInterval(()=>{
  if(document.hidden)return;
  pollSigningJob();
 },3000);

 setInterval(()=>{
  if(document.hidden||!state.admin||state.view!=='ios'||!gatewayConfigured)return;
  syncGatewayDevices({silent:true});
 },30000);

 if(new URLSearchParams(location.search).get('enrollment')==='collected'){
  notice('设备信息已提交。管理员还需要在 Apple Developer Team 中注册这台设备并重新发布 Ad Hoc 包，之后才能安装。');
 }

 renderPublicIOS();
 checkEnrollmentAvailability();
 if(state.admin)refreshOTAGatewayStatus();
}

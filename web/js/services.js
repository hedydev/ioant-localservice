import {$,state,api,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';

let gatewayConfigured=false;
let gatewaySyncing=false;

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
 if(!state.admin)return;
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
 if(!state.admin)return null;
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
  await loadDevices();
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

async function refreshGlobalServices({sync=true}={}){
 await checkEnrollmentAvailability();
 if(!state.admin)return;
 const status=await refreshOTAGatewayStatus();
 if(sync&&status?.configured)await syncGatewayDevices({silent:true});
 else await loadDevices();
}

function resetGlobalServices(){
 gatewayConfigured=false;
 gatewaySyncing=false;
 $('#devices-list').textContent='正在读取设备…';
 $('#devices-list').hidden=false;
 $('#ota-gateway-status').textContent='正在读取 OTA Gateway 配置…';
}

export function initServices(){
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
   await loadDevices();
   notice('OTA Gateway 连接正常。');
  }catch(error){
   await refreshOTAGatewayStatus();
   notice(error.message,'error');
  }finally{
   button.disabled=false;
  }
 };

 window.addEventListener('admin-loaded',()=>{
  if(state.view==='services')refreshGlobalServices({sync:true});
 });
 window.addEventListener('admin-cleared',resetGlobalServices);
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='services')refreshGlobalServices({sync:true});
 });

 setInterval(()=>{
  if(document.hidden||!state.admin||state.view!=='services'||!gatewayConfigured)return;
  syncGatewayDevices({silent:true});
 },30000);

 if(new URLSearchParams(location.search).get('enrollment')==='collected'){
  notice('设备信息已提交。管理员还需要在 Apple Developer Team 中注册这台设备并重新发布 Ad Hoc 包，之后才能安装。');
 }

 checkEnrollmentAvailability();
 if(state.view==='services')refreshGlobalServices({sync:true});
}

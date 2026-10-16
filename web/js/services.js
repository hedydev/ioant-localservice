import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';

let gatewayConfigured=false;
let gatewaySyncing=false;
let signingTeams=[];
let devices=[];
let ascTeamBinding=null;

function isIOSDevice(){
 const ua=navigator.userAgent||'';
 if(/iPhone|iPad|iPod/i.test(ua))return true;
 // iPadOS can expose a desktop-style MacIntel user agent.
 return navigator.platform==='MacIntel'&&navigator.maxTouchPoints>1;
}

function updateEnrollmentVisibility(){
 const link=$('#enroll-device-link');
 const card=link?.closest('article');
 const visible=isIOSDevice();
 if(card)card.hidden=!visible;
 return visible;
}

async function checkEnrollmentAvailability(){
 if(!updateEnrollmentVisibility())return;
 try{
  const health=await api('/api/health');
  const link=$('#enroll-device-link');
  if(health.public_enrollment_url){
   link.href=health.public_enrollment_url;
   link.removeAttribute('aria-disabled');
   link.classList.remove('disabled-link');
   $('#enrollment-availability').textContent='公网设备登记已启用。此 iPhone / iPad 可直接访问 OTA Gateway。';
  }else if(health.ota_configured){
   link.href='/api/devices/enroll.mobileconfig';
   link.removeAttribute('aria-disabled');
   link.classList.remove('disabled-link');
   $('#enrollment-availability').textContent='本地 HTTPS 设备登记入口已启用。请安装登记描述文件。';
  }else{
   link.removeAttribute('href');
   link.setAttribute('aria-disabled','true');
   link.classList.add('disabled-link');
   $('#enrollment-availability').textContent='当前 ILS 尚未配置可用的设备登记入口。';
  }
 }catch(error){
  $('#enrollment-availability').textContent='无法读取设备登记状态：'+error.message;
 }
}

function sourceName(source){
 return source==='public_ota_gateway'?'公网 OTA Gateway':source==='local_ils'?'本地 ILS':'历史登记';
}

function registrationLabel(registration){
 if(!registration)return '尚未注册';
 switch(registration.status){
  case 'registered': return 'Apple 已注册'+(registration.apple_status?' · '+registration.apple_status:'');
  case 'processing': return 'Apple 正在处理'+(registration.apple_status?' · '+registration.apple_status:'');
  case 'disabled': return 'Apple 已停用';
  case 'ineligible': return 'Apple 不可注册';
  case 'failed': return '注册失败'+(registration.last_error?' · '+registration.last_error:'');
  default: return registration.apple_status||registration.status||'等待注册';
 }
}

function teamLabel(team){
 const identities=Array.isArray(team.identities)?team.identities:[];
 if(!identities.length)return team.id;
 return team.id+' · '+identities[0];
}

function boundSigningTeam(){
 const teamID=ascTeamBinding?.configured?ascTeamBinding.team_id:'';
 return teamID?signingTeams.find(team=>team.id===teamID)||null:null;
}

function ensureDeviceListContainer(){
 const current=$('#devices-list');
 if(!current||current.tagName!=='PRE')return current;
 const replacement=document.createElement('div');
 replacement.id='devices-list';
 replacement.className='device-registry-list';
 current.replaceWith(replacement);
 return replacement;
}

function deviceRegistrationActions(device){
 if(!ascTeamBinding?.configured){
  const message=ascTeamBinding?.message||'请先在 App Store Connect 配置中绑定 Apple Team。';
  return '<p class="config-warning">'+escapeHTML(message)+' 设备注册已禁用。</p>';
 }
 const team=boundSigningTeam();
 if(!team){
  return '<p class="config-warning">当前 API Key 绑定 Apple Team '+escapeHTML(ascTeamBinding.team_id)+'，但这台 Mac 没有检测到该 Team 的代码签名身份；设备注册已禁用。</p>';
 }
 return '<div class="persistent-config-actions">'+
  '<label>Apple Team<select data-device-team="'+escapeHTML(device.udid)+'" disabled><option value="'+escapeHTML(team.id)+'" selected>'+escapeHTML(teamLabel(team))+'</option></select></label>'+
  '<button type="button" data-apple-register="'+escapeHTML(device.udid)+'">注册到 Apple / 刷新状态</button>'+
 '</div>';
}

function renderDevices(){
 const container=ensureDeviceListContainer();
 if(!container)return;
 if(!devices.length){
  container.innerHTML='<div class="persistent-config-empty"><strong>还没有登记设备</strong>新设备从公网 OTA Gateway 同步后会出现在这里。</div>';
  return;
 }
 container.innerHTML=devices.map(device=>{
  const registrations=device.apple_registrations||{};
  const registeredTeams=Object.values(registrations).filter(item=>item&&item.status==='registered');
  const summary=registeredTeams.length
   ?registeredTeams.map(item=>'Team '+item.team_id+' · '+registrationLabel(item)).join('；')
   :'尚未注册到 Apple Developer Team';
  const statusClass=registeredTeams.length?'config-ok':device.status==='apple_registration_attention'?'config-error':'config-warning';
  return '<div class="release">'+
   '<div class="section-heading"><div><h2>'+escapeHTML(device.product||'iOS Device')+'</h2><span class="meta">iOS '+escapeHTML(device.version||'')+' · '+escapeHTML(sourceName(device.source))+'</span></div><span class="badge">'+escapeHTML(device.status||'pending_apple_registration')+'</span></div>'+
   '<div class="persistent-config-grid">'+
    '<span><small>UDID</small><strong>'+escapeHTML(device.udid)+'</strong></span>'+
    '<span><small>Apple 注册状态</small><strong class="'+statusClass+'">'+escapeHTML(summary)+'</strong></span>'+
   '</div>'+deviceRegistrationActions(device)+
   '<p class="meta">Apple 注册只能使用当前 App Store Connect API Key 已绑定的 Apple Team；请让它与后续 Ad Hoc Release Profile 的 Apple Team 保持一致。</p>'+
  '</div>';
 }).join('');
}

async function loadSigningTeams(){
 if(!state.admin)return [];
 try{
  signingTeams=await api('/api/local/apple-signing-teams');
 }catch(error){
  signingTeams=[];
  notice('读取 Apple 签名 Team 失败：'+error.message,'error');
 }
 return signingTeams;
}

async function loadASCTeamBinding(){
 if(!state.admin)return null;
 try{
  ascTeamBinding=await api('/api/app-store-connect/team-binding');
 }catch(error){
  ascTeamBinding={configured:false,message:error.message};
 }
 return ascTeamBinding;
}

async function loadDevices(){
 if(!state.admin)return;
 devices=await api('/api/devices');
 renderDevices();
}

async function registerDeviceWithApple(udid,button){
 if(!needAdmin())return;
 if(!ascTeamBinding?.configured)return notice(ascTeamBinding?.message||'请先在 App Store Connect 配置中绑定 Apple Team。','error',8000);
 const selector=document.querySelector('[data-device-team="'+CSS.escape(udid)+'"]');
 const teamID=selector?.value||'';
 if(!teamID)return notice('当前绑定的 Apple Team 在这台 Mac 上不可用。','error');
 if(teamID!==ascTeamBinding.team_id)return notice('当前设备 Team 与 App Store Connect API Key 绑定 Team 不一致。','error',8000);
 button.disabled=true;
 const old=button.textContent;
 button.textContent='正在联系 Apple…';
 try{
  await api('/api/apple-devices/'+encodeURIComponent(udid)+'/register',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify({team_id:teamID})
  });
  await loadDevices();
  const device=devices.find(item=>item.udid===udid);
  const registration=device?.apple_registrations?.[teamID];
  if(registration?.status==='registered'){
   notice('设备已在 Apple Developer 中可用：Team '+teamID+'。','success');
  }else if(registration?.status==='processing'){
   notice('Apple 已接收设备，当前仍在 Processing；完成后再生成 Ad Hoc Profile。','warning');
  }else{
   notice('已刷新 Apple 设备状态：'+registrationLabel(registration),'warning');
  }
 }catch(error){
  await loadDevices().catch(()=>{});
  notice(error.message,'error',8000);
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
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
  if(!silent)notice('公网设备同步完成：待处理 '+(report.pending??0)+'，新导入 '+(report.imported??0)+'，已确认 '+(report.acked??0)+((report.failed??0)?'，失败 '+report.failed:'')+'。');
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
  const artifacts=report.artifacts??0;
  const failed=report.failed??0;
  notice('Ad Hoc OTA 同步完成：成功 '+artifacts+(failed?'，失败 '+failed:'')+'。',failed?'error':'success');
 }catch(error){
  notice(error.message,'error');
 }finally{
  button.disabled=false;
 }
}

async function refreshGlobalServices({sync=true}={}){
 await checkEnrollmentAvailability();
 if(!state.admin)return;
 await Promise.all([loadSigningTeams(),loadASCTeamBinding()]);
 const status=await refreshOTAGatewayStatus();
 if(sync&&status?.configured)await syncGatewayDevices({silent:true});
 else await loadDevices();
 renderDevices();
}

function resetGlobalServices(){
 gatewayConfigured=false;
 gatewaySyncing=false;
 signingTeams=[];
 devices=[];
 ascTeamBinding=null;
 const container=ensureDeviceListContainer();
 if(container)container.textContent='正在读取设备…';
 $('#ota-gateway-status').textContent='正在读取 OTA Gateway 配置…';
}

export function initServices(){
 updateEnrollmentVisibility();
 ensureDeviceListContainer();

 $('#devices-button').onclick=async()=>{
  if(!needAdmin())return;
  try{
   await Promise.all([loadSigningTeams(),loadASCTeamBinding(),loadDevices()]);
   renderDevices();
  }catch(error){
   notice(error.message,'error');
  }
 };

 $('#devices-list').addEventListener('click',event=>{
  const button=event.target.closest('[data-apple-register]');
  if(button)registerDeviceWithApple(button.dataset.appleRegister,button);
 });

 $('#ota-sync-devices').onclick=()=>syncGatewayDevices();
 $('#ota-sync-artifacts').onclick=syncGatewayArtifacts;
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
 window.addEventListener('asc-team-binding-changed',()=>{
  if(state.admin&&state.view==='services'){
   void Promise.all([loadSigningTeams(),loadASCTeamBinding()]).then(renderDevices);
  }
 });

 setInterval(()=>{
  if(document.hidden||!state.admin||state.view!=='services'||!gatewayConfigured)return;
  syncGatewayDevices({silent:true});
 },30000);

 if(isIOSDevice()&&new URLSearchParams(location.search).get('enrollment')==='collected'){
  notice('设备信息已提交。管理员还需要在 Apple Developer Team 中注册这台设备并重新发布 Ad Hoc 包，之后才能安装。');
 }

 checkEnrollmentAvailability();
 if(state.view==='services')refreshGlobalServices({sync:true});
}

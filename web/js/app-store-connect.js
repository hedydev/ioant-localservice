import {$,state,api,notice,escapeHTML,formatDate} from './core.js';
import {refreshData} from './projects.js';

function renderStatus(status){
 const form=$('#asc-form');
 if(!form)return;
 if(status.configured){
  form.elements.key_id.value=status.key_id||'';
  form.elements.issuer_id.value=status.issuer_id||'';
  form.elements.private_key_path.value=status.private_key_path||'';
 }

 const container=$('#asc-status');
 if(!status.configured){
  container.innerHTML='<div class="asc-status-card unconfigured"><div class="asc-status-head"><strong>尚未配置 App Store Connect API Key</strong><span class="release-status status-submitted">需要配置</span></div><p>TestFlight 仍可通过 Xcode 已登录账号上传，但 ILS 无法自动读取 Apple Processing、Beta 状态和 Public Link。</p></div>';
 }else{
  const team=status.key_type==='team';
  const connected=status.connected;
  const checking=status.refreshing;
  const keyType=team?'Team API Key':'Individual API Key';
  const uploadAuth=team?'Team API Key · 可注入 xcodebuild':'Xcode 登录账号 · API Key 仅用于状态查询';
  const lastCheck=status.last_check_at?formatDate(status.last_check_at):'尚未验证';
  const connectionText=checking?'正在同步 TestFlight 状态':connected?'App Store Connect 已连接':'已配置，连接尚未通过';
  const connectionClass=checking?'status-processing':connected?'status-available':'status-unavailable';
  container.innerHTML='<div class="asc-status-card">'+
   '<div class="asc-status-head"><strong>'+escapeHTML(connectionText)+'</strong><span class="release-status '+connectionClass+'">'+escapeHTML(keyType)+'</span></div>'+
   '<div class="asc-status-grid">'+
    '<span><small>Key ID</small><strong>'+escapeHTML(status.key_id||'—')+'</strong></span>'+
    '<span><small>状态同步</small><strong>'+escapeHTML(connected?'可用':'等待验证')+'</strong></span>'+
    '<span><small>TestFlight 上传认证</small><strong>'+escapeHTML(uploadAuth)+'</strong></span>'+
    '<span><small>最近检查</small><strong>'+escapeHTML(lastCheck)+'</strong></span>'+
   '</div>'+
   (status.private_key_mode&&status.private_key_mode!=='0600'?'<p class="asc-warning">私钥权限为 '+escapeHTML(status.private_key_mode)+'；建议改为 0600，仅当前用户可读写。</p>':'')+
   (status.last_error?'<p class="form-error">'+escapeHTML(status.last_error)+'</p>':'')+
  '</div>';
 }
 $('#asc-refresh').disabled=!status.configured||status.refreshing;
 $('#asc-check').disabled=!status.configured||status.refreshing;
 $('#asc-delete').disabled=!status.configured||status.refreshing;
}

async function loadConfig(){
 if(!state.admin)return;
 try{
  renderStatus(await api('/api/app-store-connect/config'));
 }catch(error){
  $('#asc-status').textContent=error.message;
 }
}

async function refreshStatus({announce=true}={}){
 const button=$('#asc-refresh');
 button.disabled=true;
 try{
  const result=await api('/api/app-store-connect/refresh',{method:'POST'});
  renderStatus(result.status);
  await refreshData();
  if(announce){
   notice(result.warning
    ?'已刷新 '+result.updated+' 条 TestFlight 发布状态；部分查询失败：'+result.warning
    :'已从 App Store Connect 刷新 '+result.updated+' 条 TestFlight 发布状态。');
  }
  return result;
 }catch(error){
  $('#asc-status').textContent=error.message;
  if(announce)notice(error.message);
  throw error;
 }finally{
  button.disabled=false;
 }
}

async function saveConfig(form){
 const button=form.querySelector('[type=submit]');
 button.disabled=true;
 try{
  const status=await api('/api/app-store-connect/config',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(Object.fromEntries(new FormData(form)))
  });
  renderStatus(status);
  if(status.connected){
   try{
    const result=await refreshStatus({announce:false});
    notice(result.warning
     ?'App Store Connect API 已连接；发布状态已刷新，但部分查询失败：'+result.warning
     :'App Store Connect API 已连接，并已同步 TestFlight 发布状态。');
   }catch{
    notice('App Store Connect API 已连接，但发布状态同步失败；请查看状态。');
   }
  }else{
   notice('配置已保存，但连接验证未通过；请查看状态。');
  }
 }catch(error){
  $('#asc-status').textContent=error.message;
 }finally{
  button.disabled=false;
 }
}

async function checkConnection(){
 const button=$('#asc-check');
 button.disabled=true;
 try{
  const status=await api('/api/app-store-connect/check',{method:'POST'});
  renderStatus(status);
  notice(status.connected?'App Store Connect API 连接正常。':'App Store Connect API 连接未通过。');
 }catch(error){
  $('#asc-status').textContent=error.message;
 }finally{
  button.disabled=false;
 }
}

function clearConfigUI(){
 $('#asc-status').textContent='';
 $('#asc-form')?.reset();
}

export function initAppStoreConnect(){
 $('#asc-form').onsubmit=event=>{
  event.preventDefault();
  saveConfig(event.target);
 };
 $('#asc-choose-key').onclick=async()=>{
  const button=$('#asc-choose-key');
  button.disabled=true;
  try{
   const result=await api('/api/local/select-app-store-connect-key',{method:'POST'});
   if(!result.cancelled)$('#asc-form').elements.private_key_path.value=result.path;
  }catch(error){
   $('#asc-status').textContent=error.message;
  }finally{
   button.disabled=false;
  }
 };
 $('#asc-check').onclick=checkConnection;
 $('#asc-refresh').onclick=()=>refreshStatus();
 $('#asc-delete').onclick=async()=>{
  if(!confirm('移除 ILS 的 App Store Connect API 配置？不会删除磁盘上的 .p8 私钥。'))return;
  try{
   const status=await api('/api/app-store-connect/config',{method:'DELETE'});
   $('#asc-form').reset();
   renderStatus(status);
   notice('App Store Connect API 配置已从 ILS 移除。');
  }catch(error){
   $('#asc-status').textContent=error.message;
  }
 };

 window.addEventListener('admin-cleared',clearConfigUI);
 window.addEventListener('admin-loaded',()=>{
  if(state.view==='ios')loadConfig();
 });
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='ios'&&state.admin)loadConfig();
 });

 setInterval(()=>{
  if(document.hidden||!state.admin||state.view!=='ios')return;
  loadConfig();
 },10000);
}

import {$,state,api,notice} from './core.js';
import {refreshData} from './projects.js';

function renderStatus(status){
 const form=$('#asc-form');
 if(!form)return;
 if(status.configured){
  form.elements.key_id.value=status.key_id||'';
  form.elements.issuer_id.value=status.issuer_id||'';
  form.elements.private_key_path.value=status.private_key_path||'';
 }
 const pieces=[];
 if(!status.configured)pieces.push('尚未配置 App Store Connect API Key。TestFlight 发布仍可上传，但 ILS 不能自动读取 Apple Processing / 可测试状态。');
 else if(status.connected)pieces.push('App Store Connect API 已连接。');
 else pieces.push('App Store Connect API 已配置，尚未验证连接。');
 if(status.key_type)pieces.push(status.key_type==='team'?'Team API Key':'Individual API Key');
 if(status.private_key_mode&&status.private_key_mode!=='0600')pieces.push('私钥权限 '+status.private_key_mode+'；建议使用 0600，仅当前用户可读写');
 if(status.last_error)pieces.push(status.last_error);
 $('#asc-status').textContent=pieces.join(' ');
 $('#asc-refresh').disabled=!status.configured||status.refreshing;
 $('#asc-check').disabled=!status.configured;
 $('#asc-delete').disabled=!status.configured;
}

async function loadConfig(){
 if(!state.admin)return;
 try{
  renderStatus(await api('/api/app-store-connect/config'));
 }catch(error){
  $('#asc-status').textContent=error.message;
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
  notice(status.connected?'App Store Connect API 已连接。':'配置已保存，但连接验证未通过；请查看状态。');
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

async function refreshStatus(){
 const button=$('#asc-refresh');
 button.disabled=true;
 try{
  const result=await api('/api/app-store-connect/refresh',{method:'POST'});
  renderStatus(result.status);
  await refreshData();
  notice(result.warning
   ?'已刷新 '+result.updated+' 条 TestFlight 发布状态；部分查询失败：'+result.warning
   :'已从 App Store Connect 刷新 '+result.updated+' 条 TestFlight 发布状态。');
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
 $('#asc-refresh').onclick=refreshStatus;
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
}

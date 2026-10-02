import {$,state,api,notice,escapeHTML,formatDate} from './core.js';
import {refreshData} from './projects.js';
import {loadBuildJobs} from './build-jobs.js';

let currentStatus=null;

function reconciliationSummary(report){
 if(!report)return '';
 const reasonNames={
  missing_result:'缺少结果',
  invalid_job_identity:'任务标识无效',
  unsupported_result_schema:'旧结果格式不支持',
  different_job_lane:'任务类型不是 TestFlight',
  different_result_lane:'结果类型不是 TestFlight',
  not_submitted_to_app_store_connect:'不是已提交的 App Store Connect 结果',
  missing_upload_succeeded_evidence:'缺少 upload-succeeded 证据',
  invalid_app_identity:'Bundle ID 或版本无效',
  invalid_build_number:'Build 号无效',
  unexpected_testflight_result:'TestFlight 结果字段异常',
  publish_failed:'创建 Release 失败',
  link_failed:'关联 Build Job 失败',
  read_failed:'读取任务失败',
  invalid_job_json:'任务记录损坏'
 };
 const skipped=Object.entries(report.skipped||{}).filter(([,count])=>count>0);
 const parts=[
  '扫描 '+(report.scanned||0)+' 个 TestFlight 任务',
  '可恢复 '+(report.eligible||0)+' 个',
  '新关联 '+(report.reconciled||0)+' 个'
 ];
 if(report.already_linked)parts.push('已关联 '+report.already_linked+' 个');
 if(report.active)parts.push('运行中 '+report.active+' 个');
 if(skipped.length){
  parts.push('跳过：'+skipped.map(([key,count])=>(reasonNames[key]||key)+' '+count+' 个').join('、'));
 }
 return parts.join('；');
}

function populateConfigForm(status=currentStatus){
 const form=$('#asc-form');
 form.reset();
 form.querySelector('.form-error').textContent='';
 $('#asc-dialog-status').textContent='';
 if(status?.configured){
  form.elements.key_id.value=status.key_id||'';
  form.elements.issuer_id.value=status.issuer_id||'';
  form.elements.private_key_path.value=status.private_key_path||'';
 }
}

function renderStatus(status){
 currentStatus=status;
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
 $('#asc-configure').textContent=status.configured?'编辑 App Store Connect':'配置 App Store Connect';
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
 if(announce)notice('正在同步 ILS Release 与 App Store Connect 状态…','progress',0);
 try{
  const result=await api('/api/app-store-connect/refresh',{method:'POST'});
  renderStatus(result.status);
  await refreshData();
  await loadBuildJobs();
  if(announce){
   const local=reconciliationSummary(result.reconciliation);
   const apple='Apple 状态更新 '+result.updated+' 条';
   const message=[local,apple,result.warning?'部分 Apple 查询失败：'+result.warning:''].filter(Boolean).join('；');
   const skipped=Object.values(result.reconciliation?.skipped||{}).reduce((sum,count)=>sum+count,0);
   notice(message,result.warning?'warning':(result.reconciled>0?'success':(skipped>0?'warning':'success')),7000);
  }
  return result;
 }catch(error){
  $('#asc-status').textContent=error.message;
  if(announce)notice('刷新失败：'+error.message,'error',7000);
  throw error;
 }finally{
  button.disabled=false;
 }
}

async function saveConfig(form){
 const button=form.querySelector('[type=submit]');
 button.disabled=true;
 form.querySelector('.form-error').textContent='';
 $('#asc-dialog-status').textContent='正在保存并验证…';
 try{
  const status=await api('/api/app-store-connect/config',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(Object.fromEntries(new FormData(form)))
  });
  renderStatus(status);
  if(status.connected){
   $('#asc-dialog').close();
   try{
    const result=await refreshStatus({announce:false});
    notice(result.warning
     ?'App Store Connect API 已连接；发布状态已刷新，但部分查询失败：'+result.warning
     :'App Store Connect API 已连接，并已同步 TestFlight 发布状态。',result.warning?'warning':'success',6500);
   }catch{
    notice('App Store Connect API 已连接，但发布状态同步失败；请查看状态。','warning',6500);
   }
  }else{
   $('#asc-dialog-status').textContent='配置已保存，但连接验证未通过；请检查 Key、Issuer ID 和私钥路径。';
  }
 }catch(error){
  form.querySelector('.form-error').textContent=error.message;
  $('#asc-dialog-status').textContent='';
 }finally{
  button.disabled=false;
 }
}

async function checkConnection(){
 const button=$('#asc-check');
 button.disabled=true;
 notice('正在验证 App Store Connect 连接…','progress',0);
 try{
  const status=await api('/api/app-store-connect/check',{method:'POST'});
  renderStatus(status);
  notice(
   status.connected?'App Store Connect API 连接正常。':'App Store Connect API 连接未通过。',
   status.connected?'success':'warning',
   5500
  );
 }catch(error){
  $('#asc-status').textContent=error.message;
  notice('连接验证失败：'+error.message,'error',7000);
 }finally{
  button.disabled=false;
 }
}

function clearConfigUI(){
 currentStatus=null;
 $('#asc-status').textContent='';
 populateConfigForm(null);
 if($('#asc-dialog')?.open)$('#asc-dialog').close();
}

export function initAppStoreConnect(){
 $('#asc-configure').onclick=()=>{
  populateConfigForm();
  $('#asc-dialog').showModal();
 };

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
   $('#asc-dialog-status').textContent=error.message;
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
   renderStatus(status);
   populateConfigForm(status);
   $('#asc-dialog').close();
   notice('App Store Connect API 配置已从 ILS 移除。','success');
  }catch(error){
   $('#asc-dialog-status').textContent=error.message;
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

import {$,state,api,notice,escapeHTML,formatDate} from './core.js';
import {refreshData} from './projects.js';
import {loadBuildJobs} from './build-jobs.js';

let currentStatus=null;
let currentTeamBinding=null;
let localSigningTeams=[];

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

function signingTeamLabel(team){
 const identities=Array.isArray(team?.identities)?team.identities:[];
 return identities.length?team.id+' · '+identities[0]:team.id;
}

function ensureTeamBindingField(){
 const form=$('#asc-form');
 if(!form||$('#asc-team-id'))return;
 const privateKey=form.querySelector('input[name="private_key_path"]')?.closest('label');
 if(!privateKey)return;
 const field=document.createElement('label');
 field.id='asc-team-binding-field';
 field.innerHTML='Apple Developer Team <span class="meta">设备注册与 Ad Hoc 使用；Team API Key 必须绑定</span><select id="asc-team-id" autocomplete="off"><option value="">请选择这台 Mac 的 Apple Team</option></select>';
 privateKey.before(field);
}

function renderTeamOptions(selected=''){
 ensureTeamBindingField();
 const select=$('#asc-team-id');
 if(!select)return;
 const options=['<option value="">请选择这台 Mac 的 Apple Team</option>'];
 for(const team of localSigningTeams){
  options.push('<option value="'+escapeHTML(team.id)+'">'+escapeHTML(signingTeamLabel(team))+'</option>');
 }
 select.innerHTML=options.join('');
 const wanted=selected||currentTeamBinding?.team_id||'';
 if(wanted&&localSigningTeams.some(team=>team.id===wanted))select.value=wanted;
}

function populateConfigForm(status=currentStatus,binding=currentTeamBinding){
 ensureTeamBindingField();
 const form=$('#asc-form');
 form.reset();
 form.querySelector('.form-error').textContent='';
 $('#asc-dialog-status').textContent='';
 if(status?.configured){
  form.elements.key_id.value=status.key_id||'';
  form.elements.issuer_id.value=status.issuer_id||'';
  form.elements.private_key_path.value=status.private_key_path||'';
 }
 renderTeamOptions(binding?.configured?binding.team_id:'');
}

function renderStatus(status,binding=currentTeamBinding){
 currentStatus=status;
 currentTeamBinding=binding||null;
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
  const teamText=binding?.configured?binding.team_id:'未绑定';
  container.innerHTML='<div class="asc-status-card">'+
   '<div class="asc-status-head"><strong>'+escapeHTML(connectionText)+'</strong><span class="release-status '+connectionClass+'">'+escapeHTML(keyType)+'</span></div>'+
   '<div class="asc-status-grid">'+
    '<span><small>Key ID</small><strong>'+escapeHTML(status.key_id||'—')+'</strong></span>'+
    '<span><small>Apple Team</small><strong>'+escapeHTML(teamText)+'</strong></span>'+
    '<span><small>状态同步</small><strong>'+escapeHTML(connected?'可用':'等待验证')+'</strong></span>'+
    '<span><small>TestFlight 上传认证</small><strong>'+escapeHTML(uploadAuth)+'</strong></span>'+
    '<span><small>最近检查</small><strong>'+escapeHTML(lastCheck)+'</strong></span>'+
   '</div>'+
   (team&&!binding?.configured?'<p class="asc-warning">Team API Key 尚未绑定 Apple Team；设备注册会被阻止，直到完成绑定。</p>':'')+
   (binding?.stale?'<p class="asc-warning">当前 API Key 已变更，请重新绑定 Apple Team。</p>':'')+
   (status.private_key_mode&&status.private_key_mode!=='0600'?'<p class="asc-warning">私钥权限为 '+escapeHTML(status.private_key_mode)+'；建议改为 0600，仅当前用户可读写。</p>':'')+
   (status.last_error?'<p class="form-error">'+escapeHTML(status.last_error)+'</p>':'')+
  '</div>';
 }
 $('#asc-configure').textContent=status.configured?'编辑 App Store Connect':'配置 App Store Connect';
 $('#asc-refresh').disabled=!status.configured||status.refreshing;
 $('#asc-check').disabled=!status.configured||status.refreshing;
 $('#asc-delete').disabled=!status.configured||status.refreshing;
}

async function loadTeamContext(){
 const [binding,teams]=await Promise.all([
  api('/api/app-store-connect/team-binding').catch(()=>null),
  api('/api/local/apple-signing-teams').catch(()=>[])
 ]);
 currentTeamBinding=binding;
 localSigningTeams=Array.isArray(teams)?teams:[];
 renderTeamOptions(binding?.configured?binding.team_id:'');
 return binding;
}

async function loadConfig(){
 if(!state.admin)return;
 try{
  const [status,binding]=await Promise.all([
   api('/api/app-store-connect/config'),
   loadTeamContext()
  ]);
  renderStatus(status,binding);
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
  renderStatus(result.status,currentTeamBinding);
  await refreshData();
  await loadBuildJobs();
  if(announce){
   const local=reconciliationSummary(result.reconciliation);
   const apple='Apple 状态更新 '+result.updated+' 条';
   const repairWarning=result.reconciliation_warning?'本地回填部分失败：'+result.reconciliation_warning:'';
   const appleWarning=result.warning?'部分 Apple 查询失败：'+result.warning:'';
   const message=[local,apple,repairWarning,appleWarning].filter(Boolean).join('；');
   const skipped=Object.values(result.reconciliation?.skipped||{}).reduce((sum,count)=>sum+count,0);
   const warned=Boolean(result.reconciliation_warning||result.warning||skipped>0);
   notice(message,warned?'warning':'success',7000);
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
 ensureTeamBindingField();
 const button=form.querySelector('[type=submit]');
 const teamID=$('#asc-team-id')?.value||'';
 const issuerID=String(form.elements.issuer_id?.value||'').trim();
 form.querySelector('.form-error').textContent='';
 if(issuerID&&!teamID){
  form.querySelector('.form-error').textContent='Team API Key 必须绑定这台 Mac 上对应的 Apple Team。';
  return;
 }
 button.disabled=true;
 $('#asc-dialog-status').textContent='正在保存并验证…';
 try{
  const status=await api('/api/app-store-connect/config',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(Object.fromEntries(new FormData(form)))
  });
  let binding=currentTeamBinding;
  if(teamID){
   binding=await api('/api/app-store-connect/team-binding',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({team_id:teamID})
   });
   currentTeamBinding=binding;
  }
  renderStatus(status,binding);
  window.dispatchEvent(new CustomEvent('asc-team-binding-changed',{detail:{binding}}));
  if(status.connected){
   $('#asc-dialog').close();
   try{
    const result=await refreshStatus({announce:false});
    const warning=result.reconciliation_warning||result.warning;
    notice(warning
     ?'App Store Connect API 已连接；Apple Team '+teamID+' 已绑定；发布状态已刷新，但存在警告：'+warning
     :'App Store Connect API 已连接，Apple Team '+teamID+' 已绑定，并已同步 TestFlight 发布状态。',warning?'warning':'success',6500);
   }catch{
    notice('App Store Connect API 已连接，Apple Team 已绑定，但发布状态同步失败；请查看状态。','warning',6500);
   }
  }else{
   $('#asc-dialog-status').textContent='配置已保存，但连接验证未通过；请检查 Key、Issuer ID、Apple Team 和私钥路径。';
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
  renderStatus(status,currentTeamBinding);
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
 currentTeamBinding=null;
 localSigningTeams=[];
 $('#asc-status').textContent='';
 populateConfigForm(null,null);
 if($('#asc-dialog')?.open)$('#asc-dialog').close();
}

export function initAppStoreConnect(){
 ensureTeamBindingField();
 $('#asc-configure').onclick=async()=>{
  await loadTeamContext();
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
   await api('/api/app-store-connect/team-binding',{method:'DELETE'}).catch(()=>null);
   const status=await api('/api/app-store-connect/config',{method:'DELETE'});
   currentTeamBinding=null;
   renderStatus(status,null);
   populateConfigForm(status,null);
   window.dispatchEvent(new CustomEvent('asc-team-binding-changed',{detail:{binding:null}}));
   $('#asc-dialog').close();
   notice('App Store Connect API 配置和 Apple Team 绑定已从 ILS 移除。','success');
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

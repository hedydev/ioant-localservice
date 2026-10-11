import {$,state,api,escapeHTML,notice,needAdmin,confirmAction,channelNames,formatSize,formatDate} from './core.js';
import {refreshData} from './projects.js';
import {renderReleaseCard} from './release-ui.js';
import {platformIcons,projectAppIcon} from './platform-ui.js';

function recordTime(record){
 const value=Date.parse(record?.created_at||'');
 return Number.isFinite(value)?value:0;
}

function internalTestChannel(record){
 return record.channel||'dev';
}

function internalTestVariant(record){
 return record.variant||'default';
}

function internalTestPresentation(record){
 const simulator=record.platform==='ios'||record.lane==='ios-simulator';
 return simulator
  ?{platform:'ios',targets:['iphone','ipad'],label:'iOS Simulator',kind:'Simulator Internal Test'}
  :{platform:'macos',targets:['mac'],label:'macOS',kind:'Internal Test'};
}

function internalTestCard(record,{featured=false,showDetails=false,allowDelete=false}={}){
 const version=record.version||'Internal Test';
 const presentation=internalTestPresentation(record);
 const meta=[
  record.build?'build '+record.build:'',
  channelNames[internalTestChannel(record)]||internalTestChannel(record),
  internalTestVariant(record),
  presentation.kind,
  formatDate(record.created_at)
 ].filter(Boolean).join(' · ');
 const download='<button type="button" class="download" data-history-internal-download="'+escapeHTML(record.id)+'" data-artifact-name="'+escapeHTML(record.filename||'Internal-Test')+'">下载 '+escapeHTML((record.filename||'测试包').split('.').pop().toUpperCase())+'</button>';
 const install=record.lane==='ios-simulator'
  ?'<button type="button" class="download secondary" data-history-simulator-install="'+escapeHTML(record.id)+'">安装并启动到 Simulator</button>'
  :'';
 const details=showDetails
  ?'<details><summary>构建信息</summary><p>'+escapeHTML(record.lane==='ios-simulator'
    ?'iOS Simulator Internal Test 只生成 iphonesimulator 测试包，不需要 Apple Team、Provisioning Profile 或 App Store Connect，也不创建正式 Release。'
    :'Internal Test 只生成测试安装包，不创建正式 Release，也不进行公证或正式发布。')+'</p><p>'+escapeHTML(record.filename||'')+'</p></details>'
  :'';
 const deleteAction=allowDelete
  ?'<div class="download-row release-delete-row"><button type="button" class="danger" data-delete-history-build="'+escapeHTML(record.id)+'">删除测试包</button><span class="meta">同时删除这个 Internal Test 的构建记录、日志和产物。</span></div>'
  :'';
 return '<article class="release release-card'+(featured?' featured':'')+'">'+
  '<div class="release-top">'+
   '<div class="release-identity">'+projectAppIcon(record.project_id,presentation.platform,{className:featured?'latest-app-icon':'release-app-icon',title:'App Icon'})+
    '<div class="release-heading-copy">'+
     '<div class="release-title-line"><strong>'+escapeHTML(version)+'</strong><span class="badge">'+platformIcons(presentation.targets,{className:'compact'})+escapeHTML(presentation.label)+'</span></div>'+
     '<div class="meta">'+escapeHTML(meta)+'</div>'+
    '</div>'+
   '</div>'+
   '<span class="release-status status-available">'+escapeHTML(record.lane==='ios-simulator'?'Simulator 测试包':'测试包')+'</span>'+
  '</div>'+
  '<p class="release-notes">'+escapeHTML(record.notes||(record.lane==='ios-simulator'?'Simulator Internal Test，可直接安装到当前已启动的 iOS Simulator。':'Internal Test 构建产物，可直接下载测试。'))+'</p>'+
  '<div class="download-row">'+download+install+'<span class="meta">'+formatSize(record.size||0)+' · '+escapeHTML(record.architecture||'')+' · '+escapeHTML(internalTestVariant(record))+'</span></div>'+
  details+
  deleteAction+
 '</article>';
}

function historyRows(){
 return [
  ...state.releases.map(value=>({kind:'release',value,created_at:value.created_at})),
  ...state.internalTests.map(value=>({kind:'internal-test',value,created_at:value.created_at}))
 ].sort((a,b)=>recordTime(b)-recordTime(a));
}

function filteredHistory(){
 return historyRows().filter(row=>{
  const item=row.value;
  const channel=row.kind==='internal-test'?internalTestChannel(item):item.channel;
  const variant=row.kind==='internal-test'?internalTestVariant(item):(item.variant||'default');
  return (state.platform==='all'||item.platform===state.platform)&&
   (state.channel==='all'||channel===state.channel)&&
   (state.variant==='all'||variant===state.variant);
 });
}

function currentDistributionKey(release){
 const variant=release.variant||'default';
 if(release.delivery==='testflight')return 'ios:testflight';
 if(release.platform==='ios')return 'ios:adhoc:'+variant;
 if(release.platform==='macos')return 'macos:'+variant+':'+(release.architecture||'');
 return (release.platform||'other')+':'+(release.delivery||'artifact')+':'+variant;
}

function currentDistributions(){
 const latest=new Map();
 [...state.releases]
  .sort((a,b)=>recordTime(b)-recordTime(a))
  .forEach(release=>{
   const key='release:'+currentDistributionKey(release);
   if(!latest.has(key))latest.set(key,{kind:'release',value:release,created_at:release.created_at});
  });
 [...state.internalTests]
  .sort((a,b)=>recordTime(b)-recordTime(a))
  .forEach(record=>{
   const key='internal-test:'+(record.platform||'macos')+':'+(record.lane||'macos-test')+':'+internalTestVariant(record)+':'+(record.architecture||'');
   if(!latest.has(key))latest.set(key,{kind:'internal-test',value:record,created_at:record.created_at});
  });
 return [...latest.values()].sort((a,b)=>recordTime(b)-recordTime(a));
}

function renderHistoryCard(row,{featured=false,showDetails=false,allowDelete=false}={}){
 return row.kind==='internal-test'
  ?internalTestCard(row.value,{featured,showDetails,allowDelete})
  :renderReleaseCard(row.value,{featured,showDetails,allowDelete});
}

function renderOverview(){
 const current=currentDistributions();
 const versions=new Set([
  ...state.releases.map(item=>String(item.version||'').trim()),
  ...state.internalTests.map(item=>String(item.version||'').trim())
 ].filter(Boolean));
 $('#overview-stats').innerHTML=[
  ['历史记录',state.releases.length+state.internalTests.length],
  ['当前分发',current.length],
  ['版本',versions.size]
 ].map(item=>'<div class="overview-stat"><strong>'+item[1]+'</strong><span>'+escapeHTML(item[0])+'</span></div>').join('');

 if(!current.length){
  $('#overview-latest').innerHTML='<div class="empty"><strong>暂无构建或发布</strong>Internal Test 测试包和正式 Release 完成后都会显示在这里；完整记录统一放在“版本历史”。</div>';
  return;
 }
 $('#overview-latest').innerHTML=''+
  '<div class="section-heading overview-current-heading"><div><h2>当前分发</h2><span class="meta">正式 Release 与 Internal Test 分开记录；每种分发方式只显示最新一条。</span></div></div>'+
  '<div class="overview-latest-grid">'+current.map(row=>renderHistoryCard(row,{featured:true})).join('')+'</div>';
}

function renderReleaseHistory(){
 const variants=[...new Set([
  ...state.releases.map(item=>item.variant||'default'),
  ...state.internalTests.map(item=>internalTestVariant(item))
 ])];
 if(state.variant!=='all'&&!variants.includes(state.variant))state.variant='all';
 $('#variant').innerHTML='<option value="all">全部发布</option>'+variants.map(value=>
  '<option value="'+escapeHTML(value)+'" '+(value===state.variant?'selected':'')+'>'+escapeHTML(value)+'</option>'
 ).join('');

 const rows=filteredHistory();
 $('#release-count').textContent=rows.length;
 if(!rows.length){
  $('#releases').innerHTML='<div class="empty"><strong>还没有匹配的记录</strong>调整平台、渠道或发布筛选。</div>';
  return;
 }
 $('#releases').innerHTML=rows.map(row=>renderHistoryCard(row,{showDetails:true,allowDelete:state.admin})).join('');
}

function renderReleases(){
 renderOverview();
 renderReleaseHistory();
}

async function downloadInternalArtifact(jobID,name,button){
 if(!needAdmin())return;
 const old=button.textContent;
 button.disabled=true;
 button.textContent='准备下载…';
 try{
  const response=await fetch('/api/builds/'+encodeURIComponent(jobID)+'/artifact',{
   headers:{Authorization:'Bearer '+state.token}
  });
  if(!response.ok){
   let message='下载失败（HTTP '+response.status+'）';
   try{const data=await response.json(); if(data?.error)message=data.error;}catch{}
   throw new Error(message);
  }
  const blob=await response.blob();
  const url=URL.createObjectURL(blob);
  const link=document.createElement('a');
  link.href=url;
  link.download=name||('ils-internal-test-'+jobID);
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(()=>URL.revokeObjectURL(url),1000);
 }catch(error){
  notice(error.message,'error');
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
}

async function installSimulatorArtifact(jobID,button){
 if(!needAdmin())return;
 const old=button.textContent;
 button.disabled=true;
 button.textContent='正在安装…';
 try{
  const result=await api('/api/builds/'+encodeURIComponent(jobID)+'/simulator-install',{method:'POST',timeoutMs:95000});
  if(result.warning)notice(result.warning,'warning',7000);
  else notice('已安装并启动到当前 iOS Simulator。','success');
 }catch(error){
  notice(error.message,'error',7000);
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
}

async function deleteReleaseRecord(button){
 if(!needAdmin())return;
 const id=button.dataset.deleteRelease||'';
 const release=state.releases.find(item=>item.id===id);
 if(!release)return notice('发布记录已经不存在。','error');
 const remoteOTA=release.ota?.status==='synced'||release.ota?.public_url||release.ota?.artifact_url||release.ota?.manifest_url;
 const confirmed=await confirmAction({
  title:'删除已发布安装包？',
  message:'将删除 '+(release.filename||release.version||'这个安装包')+' 以及 ILS 中的发布记录。',
  detail:remoteOTA
   ?'这个版本已同步到 OTA Gateway。ILS 会先删除公网 IPA / manifest；远端清理失败时不会删除本地记录。此操作不能撤销。'
   :'对应安装包文件和发布记录都会永久删除；关联 Build Job 会保留。此操作不能撤销。',
  confirmLabel:'删除安装包'
 });
 if(!confirmed)return;
 const old=button.textContent;
 button.disabled=true;
 button.textContent='正在删除…';
 try{
  const result=await api('/api/releases/'+encodeURIComponent(id),{method:'DELETE'});
  await refreshData();
  window.dispatchEvent(new CustomEvent('release-record-deleted',{detail:{releaseID:id}}));
  notice(result.warning||'安装包和发布记录已删除。',result.warning?'warning':'success');
 }catch(error){
  notice(error.message,'error');
  button.disabled=false;
  button.textContent=old;
 }
}

async function deleteInternalTestRecord(button){
 if(!needAdmin())return;
 const id=button.dataset.deleteHistoryBuild||'';
 const record=state.internalTests.find(item=>item.id===id);
 if(!record)return notice('Internal Test 记录已经不存在。','error');
 const confirmed=await confirmAction({
  title:'删除 Internal Test？',
  message:'将删除 '+(record.filename||'这个 Internal Test 测试包')+'。',
  detail:'对应构建记录、日志和测试包都会永久删除。此操作不能撤销。',
  confirmLabel:'删除测试包'
 });
 if(!confirmed)return;
 const old=button.textContent;
 button.disabled=true;
 button.textContent='正在删除…';
 try{
  await api('/api/builds/'+encodeURIComponent(id),{method:'DELETE'});
  await refreshData();
  window.dispatchEvent(new CustomEvent('build-record-deleted',{detail:{jobID:id}}));
  notice('Internal Test 构建记录、日志和测试包已删除。','success');
 }catch(error){
  notice(error.message,'error');
  button.disabled=false;
  button.textContent=old;
 }
}

async function publishManual(form){
 const button=form.querySelector('[type=submit]');
 button.disabled=true;
 const old=button.textContent;
 button.textContent='处理中…';
 form.querySelector('.form-error').textContent='';
 try{
  await api('/api/projects/'+state.project+'/releases',{method:'POST',body:new FormData(form)});
  form.closest('dialog').close();
  form.reset();
  await refreshData();
  notice('发布完成。','success');
 }catch(error){
  form.querySelector('.form-error').textContent=error.message;
 }finally{
  button.disabled=false;
  button.textContent=old;
 }
}

export function initReleases(){
 window.addEventListener('data-refreshed',renderReleases);

 $('#platforms').addEventListener('click',event=>{
  const button=event.target.closest('[data-platform]');
  if(!button)return;
  state.platform=button.dataset.platform;
  document.querySelectorAll('[data-platform]').forEach(item=>item.setAttribute('aria-pressed',String(item===button)));
  renderReleaseHistory();
 });
 $('#channel').onchange=event=>{state.channel=event.target.value;renderReleaseHistory();};
 $('#variant').onchange=event=>{state.variant=event.target.value;renderReleaseHistory();};
 $('#refresh').onclick=()=>refreshData().then(()=>notice('')).catch(error=>notice(error.message));

 const handleInternalAction=event=>{
  const download=event.target.closest('[data-history-internal-download]');
  if(download){
   void downloadInternalArtifact(download.dataset.historyInternalDownload,download.dataset.artifactName,download);
   return true;
  }
  const install=event.target.closest('[data-history-simulator-install]');
  if(install){
   void installSimulatorArtifact(install.dataset.historySimulatorInstall,install);
   return true;
  }
  return false;
 };
 $('#overview-latest').addEventListener('click',handleInternalAction);
 $('#releases').addEventListener('click',event=>{
  if(handleInternalAction(event))return;
  const releaseDelete=event.target.closest('[data-delete-release]');
  if(releaseDelete){
   void deleteReleaseRecord(releaseDelete);
   return;
  }
  const internalDelete=event.target.closest('[data-delete-history-build]');
  if(internalDelete)void deleteInternalTestRecord(internalDelete);
 });

 $('#publish-button').onclick=()=>{
  if(!needAdmin())return;
  if(!state.project){$('#project-dialog').showModal();return;}
  $('#publish-dialog').showModal();
 };
 $('#publish-form').onsubmit=event=>{
  event.preventDefault();
  if(state.project)publishManual(event.target);
 };

 $('#check-form').onsubmit=async event=>{
  event.preventDefault();
  if(!state.project)return notice('请先创建项目');
  try{
   const data=await api('/api/projects/'+state.project+'/updates?'+new URLSearchParams(new FormData(event.target)));
   $('#check-result').textContent=data.latest
    ?(data.update_available?'有更新：'+data.latest.version+' (build '+data.latest.build+')':'当前版本已是最新，或高于服务中的版本。')
    :'此平台、架构和渠道还没有正式发布。';
  }catch(error){
   $('#check-result').textContent=error.message;
  }
 };

 renderReleases();
}

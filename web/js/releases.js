import {$,state,api,escapeHTML,notice,needAdmin,channelNames,formatSize,formatDate} from './core.js';
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

function internalTestCard(record,{featured=false,showDetails=false}={}){
 const version=record.version||'Internal Test';
 const meta=[
  record.build?'build '+record.build:'',
  channelNames[internalTestChannel(record)]||internalTestChannel(record),
  internalTestVariant(record),
  'Internal Test',
  formatDate(record.created_at)
 ].filter(Boolean).join(' · ');
 const download='<button type="button" class="download" data-history-internal-download="'+escapeHTML(record.id)+'" data-artifact-name="'+escapeHTML(record.filename||'Internal-Test')+'">下载 '+escapeHTML((record.filename||'测试包').split('.').pop().toUpperCase())+'</button>';
 const details=showDetails
  ?'<details><summary>构建信息</summary><p>Internal Test 只生成测试安装包，不创建正式 Release，也不进行公证或正式发布。</p><p>'+escapeHTML(record.filename||'')+'</p></details>'
  :'';
 return '<article class="release release-card'+(featured?' featured':'')+'">'+
  '<div class="release-top">'+
   '<div class="release-identity">'+projectAppIcon(record.project_id,'macos',{className:featured?'latest-app-icon':'release-app-icon',title:'App Icon'})+
    '<div class="release-heading-copy">'+
     '<div class="release-title-line"><strong>'+escapeHTML(version)+'</strong><span class="badge">'+platformIcons(['mac'],{className:'compact'})+'macOS</span></div>'+
     '<div class="meta">'+escapeHTML(meta)+'</div>'+
    '</div>'+
   '</div>'+
   '<span class="release-status status-available">测试包</span>'+
  '</div>'+
  '<p class="release-notes">'+escapeHTML(record.notes||'Internal Test 构建产物，可直接下载测试。')+'</p>'+
  '<div class="download-row">'+download+'<span class="meta">'+formatSize(record.size||0)+' · '+escapeHTML(record.architecture||'')+' · '+escapeHTML(internalTestVariant(record))+'</span></div>'+
  details+
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
   const key='internal-test:macos:'+internalTestVariant(record)+':'+(record.architecture||'');
   if(!latest.has(key))latest.set(key,{kind:'internal-test',value:record,created_at:record.created_at});
  });
 return [...latest.values()].sort((a,b)=>recordTime(b)-recordTime(a));
}

function renderHistoryCard(row,{featured=false,showDetails=false}={}){
 return row.kind==='internal-test'
  ?internalTestCard(row.value,{featured,showDetails})
  :renderReleaseCard(row.value,{featured,showDetails});
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
 $('#releases').innerHTML=rows.map(row=>renderHistoryCard(row,{showDetails:true})).join('');
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
  notice('发布完成。');
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

 const handleInternalDownload=event=>{
  const button=event.target.closest('[data-history-internal-download]');
  if(!button)return;
  void downloadInternalArtifact(button.dataset.historyInternalDownload,button.dataset.artifactName,button);
 };
 $('#overview-latest').addEventListener('click',handleInternalDownload);
 $('#releases').addEventListener('click',handleInternalDownload);

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

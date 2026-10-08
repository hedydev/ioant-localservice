
import {$,state,api,escapeHTML,formatDate} from './core.js';
import {buildState} from './build-state.js';
import {platformIcons,targetsForJob,buildAppIcon} from './platform-ui.js';
import {releaseStatusLabel,testFlightLifecycleView,testFlightLinkSource} from './release-ui.js';

function ensureBuildJobsStyles(){
 if(document.querySelector('link[data-build-jobs-style]'))return;
 const link=document.createElement('link');
 link.rel='stylesheet';
 link.href='/build-jobs.css?v=20261007';
 link.dataset.buildJobsStyle='1';
 document.head.appendChild(link);
}

function logElement(jobID){
 return document.getElementById('build-log-'+jobID);
}

function updateBuildLog(jobID,text){
 const element=logElement(jobID);
 if(!element)return;
 const previous=element.textContent||'';
 const scrollTop=element.scrollTop;
 if(previous===text){
  buildState.logLoaded=true;
  return;
 }
 if(previous&&text.startsWith(previous)){
  element.append(document.createTextNode(text.slice(previous.length)));
 }else{
  element.textContent=text;
 }
 buildState.logLoaded=true;
 const maxScroll=Math.max(0,element.scrollHeight-element.clientHeight);
 element.scrollTop=Math.min(scrollTop,maxScroll);
 buildState.logScrollTop=element.scrollTop;
}

function chooseRunningLog(jobs){
 if(buildState.activeLog&&jobs.some(job=>job.id===buildState.activeLog))return;
 if(buildState.activeLog)buildState.activeLog=null;
 const running=jobs.find(job=>job.status==='running'&&!buildState.collapsedLogs.has(job.id));
 if(!running)return;
 buildState.activeLog=running.id;
 buildState.followLog=false;
 buildState.logLoaded=false;
 buildState.logScrollTop=0;
}

function laneFor(job){
 return job.lane||job.result?.lane||job.profile_id||'';
}

function buildTypeLabel(value){
 return ({native:'Native',expo:'Expo / React Native','hybrid-web-native':'Hybrid Web-Native',tauri:'Tauri / Rust'})[value]||value||'自定义脚本';
}

function linkedReleases(job){
 const ids=job.release_ids||[];
 return ids.map(id=>state.releases.find(release=>release.id===id)).filter(Boolean);
}

function linkedRelease(job){
 return linkedReleases(job)[0]||null;
}

function linkedReleaseActions(job){
 return linkedReleases(job).map(release=>{
  if(release.delivery==='testflight'){
   const source=testFlightLinkSource(release);
   return release.open_url
    ?'<span class="build-release-action"><a class="build-artifact" href="'+escapeHTML(release.open_url)+'">在 TestFlight 中打开</a>'+(source?'<span class="release-link-source">'+escapeHTML(source)+'</span>':'')+'</span>'
    :'<span class="meta">TestFlight 链接尚未可用</span>';
  }
  return release.download_url
   ?'<a class="build-artifact" href="'+escapeHTML(release.download_url)+'">下载 '+escapeHTML((release.filename||release.id.slice(0,8)))+'</a>'
   :'';
 }).join(' ');
}

function internalArtifactName(job){
 const raw=job.result?.artifact||'';
 const parts=raw.split(/[\\/]/);
 return parts[parts.length-1]||'Internal-Test';
}

function internalArtifactAction(job){
 if(laneFor(job)!=='macos-test'||job.status!=='succeeded'||!job.result?.artifact)return '';
 const name=internalArtifactName(job);
 return '<button type="button" class="build-artifact" data-download-internal-artifact="'+escapeHTML(job.id)+'" data-artifact-name="'+escapeHTML(name)+'">下载 '+escapeHTML(name)+'</button>';
}

async function downloadInternalArtifact(jobID,name,button){
 const oldText=button.textContent;
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
  console.warn('ILS internal artifact download failed:',error);
  button.textContent='下载失败';
  setTimeout(()=>{button.textContent=oldText;},1500);
  return;
 }finally{
  button.disabled=false;
 }
 button.textContent=oldText;
}

function stageLabel(job){
 const stages={
  preflight:'预检查',pull:'正在拉取',build:'正在构建',archive:'正在归档',validate:'正在验证',
  export:'正在导出',package:'正在打包',notarize:'正在公证',upload:'正在上传',
  publish:'正在发布到 ILS',submitted:'已提交到 App Store Connect',processing:'Apple Processing',
  available:'TestFlight 可测试',metadata:'读取版本',script:'项目脚本执行中',complete:'完成'
 };
 if(job.status==='running')return stages[job.stage]||'处理中';
 if(job.status==='succeeded'&&laneFor(job)==='ios-testflight'){
  const release=linkedRelease(job);
  if(release)return releaseStatusLabel(release);
  if(job.stage_state==='failed')return 'TestFlight 当前不可测试';
  if(job.stage==='available')return 'TestFlight 可测试';
  if(job.stage==='processing')return 'Apple Processing';
  return 'ILS Release 待同步';
 }
 if(job.status==='succeeded'&&laneFor(job)==='macos-test')return '测试包已生成';
 if(job.status==='succeeded')return '发布成功';
 return '失败';
}

function progressView(job){
 if(job.status!=='running')return '';
 if(Number.isFinite(job.progress)){
  return '<div class="job-progress determinate" data-job-progress="'+escapeHTML(job.id)+'"><div class="job-progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="100"><span class="job-progress-fill"></span><span class="job-progress-shine"></span></div><span data-progress-label></span></div>';
 }
 return '<div class="job-progress indeterminate" data-job-progress="'+escapeHTML(job.id)+'"><div class="job-progress-track" role="progressbar" aria-label="'+escapeHTML(stageLabel(job))+'"><span class="job-progress-fill"></span><span class="job-progress-shine"></span></div><span data-progress-label>'+escapeHTML(stageLabel(job))+'</span></div>';
}

function updateProgress(article,job){
 const root=article.querySelector('[data-job-progress]');
 if(!root){
  buildState.progressByJob.delete(job.id);
  return;
 }
 const fill=root.querySelector('.job-progress-fill');
 const label=root.querySelector('[data-progress-label]');
 const track=root.querySelector('.job-progress-track');
 if(!fill||!label||!track)return;
 if(!Number.isFinite(job.progress)){
  label.textContent=stageLabel(job);
  track.removeAttribute('aria-valuenow');
  buildState.progressByJob.delete(job.id);
  return;
 }
 const value=Math.max(0,Math.min(100,job.progress));
 label.textContent=value+'%';
 track.setAttribute('aria-valuenow',String(value));
 if(fill.dataset.progressValue===String(value))return;
 const previous=buildState.progressByJob.get(job.id);
 const from=Number.isFinite(previous)?Math.max(0,Math.min(100,previous)):0;
 fill.getAnimations().forEach(animation=>animation.cancel());
 fill.animate(
  [{transform:'scaleX('+(from/100)+')'},{transform:'scaleX('+(value/100)+')'}],
  {duration:from===value?0:1400,easing:'cubic-bezier(.16,1,.3,1)',fill:'forwards'}
 );
 fill.dataset.progressValue=String(value);
 buildState.progressByJob.set(job.id,value);
}

function resultView(job){
 if(!job.result)return '';
 const result=job.result;
 if(result.lane==='ios-testflight'){
  const release=linkedRelease(job);
  const releaseStatus=release?.status||'submitted';
  const presentation=release||{delivery:'testflight',status:releaseStatus,status_message:job.message||''};
  const stateText=release?releaseStatusLabel(presentation):'ILS Release 待同步';
  const openURL=release?.open_url||job.testflight_url||'';
  const source=release?testFlightLinkSource(release):'';
  const link=openURL
   ?'<span class="build-release-action"><a class="testflight-link" href="'+escapeHTML(openURL)+'">在 TestFlight 中打开</a>'+(source?'<span class="release-link-source">'+escapeHTML(source)+'</span>':'')+'</span>'
   :'';
  const detail=release?.status_message||
   (!release
    ?'上传结果已保存，但构建任务尚未关联到 ILS Release；服务会在重启或刷新 App Store Connect 状态时自动修复。'
    :'App Store Connect 状态：'+stateText);
  return '<div class="submission-result"><strong>iOS 发布</strong><span>'+escapeHTML(result.version)+' ('+escapeHTML(result.build)+')</span><span>'+escapeHTML(stateText)+'</span><small>'+escapeHTML(detail)+'</small>'+link+'</div>';
 }
 return '<div class="meta">结果：'+escapeHTML(result.lane||'artifact')+' · '+escapeHTML(result.version||'')+' ('+escapeHTML(result.build||'')+')</div>';
}

function pipelineView(job){
 const lane=laneFor(job);
 const labels={
  preflight:'预检查',build:'构建',archive:'归档',validate:'验证',
  export:'导出',package:'打包',notarize:'公证',upload:'上传',publish:'发布',complete:'完成'
 };
 if(lane==='ios-testflight'){
  const stages=['preflight','archive','validate','upload'];
  const terminalStages=['submitted','processing','available','complete'];
  let current=stages.indexOf(job.stage);
  if(current<0&&job.status==='running')current=0;
  if(job.status==='succeeded'||terminalStages.includes(job.stage))current=stages.length;
  const pipeline='<div class="pipeline pipeline-flow" aria-label="构建流水线">'+stages.map((stage,index)=>{
   let stepState='pending';
   if(index<current)stepState='done';
   else if(index===current){
    if(job.status==='failed'||job.stage_state==='failed')stepState='failed';
    else if(job.status==='running')stepState='active';
    else stepState='done';
   }
   return '<span class="pipeline-step '+stepState+'"><i aria-hidden="true"></i>'+escapeHTML(labels[stage]||stage)+'</span>';
  }).join('<span class="pipeline-arrow">→</span>')+'</div>';
  const release=linkedRelease(job);
  const presentation=release||(job.status==='succeeded'?{delivery:'testflight',status:'submitted',testflight:{}}:null);
  return pipeline+(presentation?testFlightLifecycleView(presentation,{compact:true}):'');
 }
 const lanes={
  'ios-adhoc':['preflight','archive','validate','export','publish','complete'],
  'macos-test':['preflight','build','package','complete'],
  'macos-release':['preflight','build','package','notarize','validate','publish','complete']
 };
 const stages=lanes[lane]||['preflight','build','validate','publish','complete'];
 let current=stages.indexOf(job.stage);
 if(current<0&&job.status==='running')current=0;
 if(job.status==='succeeded')current=stages.length;
 return '<div class="pipeline pipeline-flow" aria-label="构建流水线">'+stages.map((stage,index)=>{
  let stepState='pending';
  if(index<current)stepState='done';
  else if(index===current){
   if(job.status==='failed'||job.stage_state==='failed')stepState='failed';
   else if(job.status==='running')stepState='active';
   else stepState='done';
  }
  return '<span class="pipeline-step '+stepState+'"><i aria-hidden="true"></i>'+escapeHTML(labels[stage]||stage)+'</span>';
 }).join('<span class="pipeline-arrow">→</span>')+'</div>';
}

function sourceMeta(job){
 const branch=job.branch||'detached';
 const head=(job.commit||job.head||'').slice(0,12)||'unknown';
 const dirty=job.dirty?' · 本地改动':' · clean';
 const upstream=job.upstream?' · upstream '+job.upstream:'';
 return escapeHTML(branch)+' · HEAD '+escapeHTML(head)+dirty+escapeHTML(upstream);
}

function jobMainHTML(job){
 const releases=job.release_ids||[];
 return '<div class="build-time">'+escapeHTML(formatDate(job.created_at))+'</div>'+
  '<div class="build-title-row">'+buildAppIcon(job,{className:'job-app-icon',title:job.title||'App Icon'})+'<strong>'+platformIcons(targetsForJob(job))+escapeHTML(job.title||job.profile_id||job.script||'ILS Build')+'</strong> <span class="badge">'+escapeHTML(stageLabel(job))+'</span></div>'+
  '<div class="meta">'+escapeHTML(job.mode==='profile'?'ILS Profile':'Project Script')+' · '+escapeHTML(buildTypeLabel(job.build_type))+'</div>'+
  '<div class="meta git-build-meta">'+sourceMeta(job)+'</div>'+
  pipelineView(job)+
  (job.message?'<div class="job-message">'+escapeHTML(job.message)+'</div>':'')+
  progressView(job)+
  (job.error?'<p>'+escapeHTML(job.error)+'</p>':'')+
  resultView(job)+
  internalArtifactAction(job)+
  (releases.length?'<div class="meta">关联发布：'+releases.length+' 个</div>':'')+
  linkedReleaseActions(job);
}

function ensureLogPanel(article,job,expanded){
 const existing=article.querySelector('.build-log-panel');
 if(!expanded){
  if(existing)existing.remove();
  return;
 }
 if(existing)return;
 const panel=document.createElement('div');
 panel.className='build-log-panel';
 panel.dataset.logFor=job.id;
 panel.innerHTML='<div class="build-log-heading"><strong>'+(job.status==='running'?'实时日志':'任务日志')+'</strong><span class="meta">新内容只追加，不自动滚动</span></div><pre class="build-log-output" id="build-log-'+escapeHTML(job.id)+'" data-build-log-output="'+escapeHTML(job.id)+'" aria-label="'+escapeHTML(job.title||job.profile_id||job.script||'ILS Build')+' 构建日志"></pre>';
 article.appendChild(panel);
}

function renderJobs(jobs){
 jobs=[...jobs].sort((a,b)=>new Date(b.created_at)-new Date(a.created_at));
 const container=$('#build-jobs');
 const pageX=window.scrollX;
 const pageY=window.scrollY;
 if(!jobs.length){
  buildState.activeLog=null;
  buildState.progressByJob.clear();
  container.innerHTML='<div class="empty"><strong>还没有构建任务</strong>运行 Release Profile 后，任务状态会显示在这里。</div>';
  return;
 }
 chooseRunningLog(jobs);
 const keep=new Set(jobs.map(job=>job.id));
 container.querySelectorAll('[data-build-job-id]').forEach(article=>{
  if(!keep.has(article.dataset.buildJobId)){
   buildState.progressByJob.delete(article.dataset.buildJobId);
   article.remove();
  }
 });
 container.querySelector('.empty')?.remove();
 jobs.forEach((job,index)=>{
  let article=container.querySelector('[data-build-job-id="'+CSS.escape(job.id)+'"]');
  if(!article){
   article=document.createElement('article');
   article.dataset.buildJobId=job.id;
   article.innerHTML='<div class="build-job-main"></div><div class="build-job-actions"><button type="button"></button></div>';
  }
  const expanded=buildState.activeLog===job.id;
  article.className='build-job'+(expanded?' log-expanded':'')+(job.status==='running'?' build-running':'');
  const main=article.querySelector('.build-job-main');
  const html=jobMainHTML(job);
  if(main._ilsHTML!==html){
   main.innerHTML=html;
   main._ilsHTML=html;
  }
  updateProgress(article,job);
  const button=article.querySelector('.build-job-actions button');
  button.dataset.buildLog=job.id;
  button.setAttribute('aria-expanded',String(expanded));
  button.setAttribute('aria-controls','build-log-'+job.id);
  button.textContent=expanded?'收起日志':'查看日志';
  ensureLogPanel(article,job,expanded);
  const currentAtIndex=container.children[index]||null;
  if(currentAtIndex!==article)container.insertBefore(article,currentAtIndex);
 });
 if(window.scrollX!==pageX||window.scrollY!==pageY)window.scrollTo(pageX,pageY);
}

export async function loadBuildJobs(){
 if(!state.project||!state.admin||buildState.jobsLoading)return;
 buildState.jobsLoading=true;
 const project=state.project;
 try{
  const jobs=await api('/api/projects/'+project+'/builds');
  if(project!==state.project)return;
  renderJobs(jobs);
 }catch(error){
  if(project!==state.project)return;
  const container=$('#build-jobs');
  if(!container.querySelector('[data-build-job-id]'))container.innerHTML='<p class="form-error">'+escapeHTML(error.message)+'</p>';
  else console.warn('ILS build polling failed:',error);
 }finally{
  buildState.jobsLoading=false;
 }
}

function resetJobs(){
 buildState.activeLog=null;
 buildState.collapsedLogs.clear();
 buildState.followLog=false;
 buildState.logLoaded=false;
 buildState.logScrollTop=0;
 buildState.jobsLoading=false;
 buildState.progressByJob.clear();
 $('#build-jobs').innerHTML='';
}

export function initBuildJobs(){
 ensureBuildJobsStyles();
 const container=$('#build-jobs');
 container.onclick=event=>{
  const artifactButton=event.target.closest('[data-download-internal-artifact]');
  if(artifactButton){
   void downloadInternalArtifact(artifactButton.dataset.downloadInternalArtifact,artifactButton.dataset.artifactName,artifactButton);
   return;
  }
  const button=event.target.closest('[data-build-log]');
  if(!button)return;
  const jobID=button.dataset.buildLog;
  if(buildState.activeLog===jobID){
   buildState.activeLog=null;
   buildState.collapsedLogs.add(jobID);
  }else{
   buildState.activeLog=jobID;
   buildState.collapsedLogs.delete(jobID);
  }
  buildState.followLog=false;
  buildState.logLoaded=false;
  buildState.logScrollTop=0;
  loadBuildJobs();
 };
 window.addEventListener('build-started',event=>{
  const jobID=event.detail.job.id;
  buildState.activeLog=jobID;
  buildState.collapsedLogs.delete(jobID);
  buildState.followLog=false;
  buildState.logLoaded=false;
  buildState.logScrollTop=0;
  loadBuildJobs();
 });
 window.addEventListener('project-changed',resetJobs);
 window.addEventListener('admin-cleared',resetJobs);
 window.addEventListener('admin-loaded',()=>{
  if(state.view==='builds')loadBuildJobs();
 });
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='builds'&&state.admin)loadBuildJobs();
 });
 setInterval(()=>{
  if(document.hidden||!state.admin||state.view!=='builds')return;
  loadBuildJobs();
 },3000);
}

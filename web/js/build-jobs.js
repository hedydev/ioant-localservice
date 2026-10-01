
import {$,state,api,escapeHTML,formatDate} from './core.js';
import {buildState} from './build-state.js';
import {platformIcons,targetsForJob} from './platform-ui.js';

function logElement(jobID){
 return document.getElementById('build-log-'+jobID);
}

function updateBuildLog(jobID,text){
 const element=logElement(jobID);
 if(!element)return;
 const firstOpen=!buildState.logLoaded;
 const wasFollowing=firstOpen||buildState.followLog;
 element.textContent=text;
 buildState.logLoaded=true;
 if(wasFollowing){
  element.scrollTop=element.scrollHeight;
  buildState.followLog=true;
  buildState.logScrollTop=element.scrollTop;
 }else{
  const maxScroll=Math.max(0,element.scrollHeight-element.clientHeight);
  element.scrollTop=Math.min(buildState.logScrollTop,maxScroll);
 }
}

function chooseRunningLog(jobs){
 if(buildState.activeLog&&jobs.some(job=>job.id===buildState.activeLog))return;
 if(buildState.activeLog)buildState.activeLog=null;
 const running=jobs.find(job=>job.status==='running'&&!buildState.collapsedLogs.has(job.id));
 if(!running)return;
 buildState.activeLog=running.id;
 buildState.followLog=true;
 buildState.logLoaded=false;
 buildState.logScrollTop=0;
}

function stageLabel(job){
 const stages={
  preflight:'预检查',pull:'正在拉取',build:'正在构建',archive:'正在归档',validate:'正在验证',
  export:'正在导出',package:'正在打包',notarize:'正在公证',upload:'正在上传',
  publish:'正在发布到 ILS',submitted:'已提交',processing:'Apple 处理中',
  metadata:'读取版本',script:'项目脚本执行中',complete:'完成'
 };
 if(job.status==='running')return stages[job.stage]||'处理中';
 if(job.status==='succeeded'&&job.result?.lane==='ios-testflight')return 'TestFlight 已提交';
 if(job.status==='succeeded')return '发布成功';
 return '失败';
}

function progressView(job){
 if(job.status!=='running')return '';
 if(Number.isFinite(job.progress)){
  const value=Math.max(0,Math.min(100,job.progress));
  return '<div class="job-progress"><progress max="100" value="'+value+'"></progress><span>'+value+'%</span></div>';
 }
 return '<div class="job-progress indeterminate"><progress max="100" aria-label="'+escapeHTML(stageLabel(job))+'"></progress><span>'+escapeHTML(stageLabel(job))+'</span></div>';
}

function resultView(job){
 if(!job.result)return '';
 const result=job.result;
 if(result.lane==='ios-testflight'){
  return '<div class="submission-result"><strong>TestFlight</strong><span>'+escapeHTML(result.version)+' ('+escapeHTML(result.build)+')</span><span>'+escapeHTML(result.submission_result||result.status)+'</span><small>上传成功不等于 Apple 已完成 Processing。</small></div>';
 }
 return '<div class="meta">结果：'+escapeHTML(result.lane||'artifact')+' · '+escapeHTML(result.version||'')+' ('+escapeHTML(result.build||'')+')</div>';
}

function pipelineView(job){
 const lanes={
  'ios-testflight':['pull','preflight','archive','validate','upload','complete'],
  'ios-adhoc':['pull','preflight','archive','validate','export','publish','complete'],
  'macos-test':['pull','preflight','build','package','validate','publish','complete'],
  'macos-release':['pull','preflight','build','package','notarize','validate','publish','complete']
 };
 const labels={
  pull:'拉取',preflight:'预检查',build:'构建',archive:'归档',validate:'验证',
  export:'导出',package:'打包',notarize:'公证',upload:'上传',publish:'发布',complete:'完成'
 };
 const lane=job.result?.lane||job.profile_id||'';
 const stages=lanes[lane]||['pull','build','validate','publish','complete'];
 const current=stages.indexOf(job.stage);
 return '<div class="pipeline" aria-label="构建流水线">'+stages.map((stage,index)=>{
  let state='pending';
  if(index<current)state='done';
  else if(index===current)state=job.status==='failed'?'failed':job.status==='succeeded'?'done':'active';
  else if(job.status==='succeeded'&&stage==='complete')state='done';
  return '<span class="pipeline-step '+state+'">'+escapeHTML(labels[stage]||stage)+'</span>';
 }).join('<span class="pipeline-arrow">→</span>')+'</div>';
}

function renderJobs(jobs){
 jobs=[...jobs].sort((a,b)=>new Date(b.created_at)-new Date(a.created_at));
 if(!jobs.length){
  buildState.activeLog=null;
  $('#build-jobs').innerHTML='<div class="empty"><strong>还没有构建任务</strong>运行 Release Profile 后，任务状态会显示在这里。</div>';
  return;
 }

 chooseRunningLog(jobs);

 $('#build-jobs').innerHTML=jobs.map(job=>{
  const releases=job.release_ids||[];
  const expanded=buildState.activeLog===job.id;
  const logID='build-log-'+job.id;
  return '<article class="build-job'+(expanded?' log-expanded':'')+'">'+
   '<div class="build-job-row">'+
    '<div class="build-job-main">'+
     '<div class="build-time">'+escapeHTML(formatDate(job.created_at))+'</div>'+
     '<div class="build-title-row"><strong>'+platformIcons(targetsForJob(job))+escapeHTML(job.title||job.profile_id||job.script||'ILS Build')+'</strong> <span class="badge">'+escapeHTML(stageLabel(job))+'</span></div>'+
     '<div class="meta">'+escapeHTML(job.mode==='profile'?'ILS Profile':'Project Script')+' · commit '+escapeHTML((job.commit||'').slice(0,12))+'</div>'+
     pipelineView(job)+
     (job.message?'<div class="job-message">'+escapeHTML(job.message)+'</div>':'')+
     progressView(job)+
     (job.error?'<p>'+escapeHTML(job.error)+'</p>':'')+
     resultView(job)+
     (releases.length?'<div class="meta">关联安装包：'+releases.length+' 个</div>':'')+
     releases.map(release=>'<a class="build-artifact" href="/api/releases/'+escapeHTML(release)+'/download">下载 '+escapeHTML(release.slice(0,8))+'</a>').join(' ')+
    '</div>'+
    '<button data-build-log="'+escapeHTML(job.id)+'" aria-expanded="'+(expanded?'true':'false')+'" aria-controls="'+escapeHTML(logID)+'">'+(expanded?'收起日志':'查看日志')+'</button>'+
   '</div>'+
   (expanded
    ?'<div class="build-log-panel"><div class="build-log-heading"><strong>'+(job.status==='running'?'实时日志':'任务日志')+'</strong><span class="meta">'+(job.status==='running'?'任务执行时自动跟随最新输出':'已保存的任务输出')+'</span></div><pre class="build-log-output" id="'+escapeHTML(logID)+'" data-build-log-output="'+escapeHTML(job.id)+'" aria-label="'+escapeHTML(job.title||job.profile_id||job.script||'ILS Build')+' 构建日志"></pre></div>'
    :'')+
  '</article>';
 }).join('');
}

export async function loadBuildJobs(){
 if(!state.project||!state.admin||buildState.jobsLoading)return;
 buildState.jobsLoading=true;
 const project=state.project;
 try{
  const jobs=await api('/api/projects/'+project+'/builds');
  if(project!==state.project)return;
  renderJobs(jobs);
  if(buildState.activeLog){
   const data=await api('/api/builds/'+buildState.activeLog+'/log');
   if(project!==state.project)return;
   updateBuildLog(buildState.activeLog,data.log);
  }
 }catch(error){
  if(project===state.project)$('#build-jobs').innerHTML='<p class="form-error">'+escapeHTML(error.message)+'</p>';
 }finally{
  buildState.jobsLoading=false;
 }
}

function resetJobs(){
 buildState.activeLog=null;
 buildState.collapsedLogs.clear();
 buildState.followLog=true;
 buildState.logLoaded=false;
 buildState.logScrollTop=0;
 buildState.jobsLoading=false;
 $('#build-jobs').innerHTML='';
}

export function initBuildJobs(){
 $('#build-jobs').onclick=event=>{
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
  buildState.followLog=true;
  buildState.logLoaded=false;
  buildState.logScrollTop=0;
  loadBuildJobs();
 };

 $('#build-jobs').addEventListener('scroll',event=>{
  const element=event.target.closest('[data-build-log-output]');
  if(!element)return;
  buildState.followLog=(element.scrollHeight-element.scrollTop-element.clientHeight)<64;
  buildState.logScrollTop=element.scrollTop;
 },true);

 window.addEventListener('build-started',event=>{
  const jobID=event.detail.job.id;
  buildState.activeLog=jobID;
  buildState.collapsedLogs.delete(jobID);
  buildState.followLog=true;
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

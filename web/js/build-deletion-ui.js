import {state,api,notice,confirmAction,needAdmin} from './core.js';
import {buildState} from './build-state.js';
import {simulatorState} from './ios-simulators.js';
import {loadBuildJobs} from './build-jobs.js';

async function deleteBuildCascade(button){
 if(!needAdmin())return;
 const jobID=button.dataset.deleteBuildJob||'';
 const releaseCount=Number(button.dataset.releaseCount||0);
 const hasInternalArtifact=button.dataset.hasInternalArtifact==='1';
 const hasReleaseHistory=releaseCount>0;
 const confirmed=await confirmAction({
  title:hasInternalArtifact
   ?'删除构建和测试包？'
   :(hasReleaseHistory?'删除构建和发布？':'删除历史构建？'),
  message:hasInternalArtifact
   ?'这个 Build Job 的构建记录、日志和 Internal Test 测试包都会删除。'
   :(hasReleaseHistory
    ?'这个 Build Job 的构建记录、日志，以及关联的 Release / 版本历史都会一起删除。'
    :'这个 Build Job 的构建记录和日志都会删除；如果存在关联版本历史，也会一起清理。'),
  detail:hasReleaseHistory
   ?'Ad Hoc / 本地发布的安装包会一起删除；已经同步到 OTA Gateway 的远端安装包也会清理。TestFlight 只删除 ILS 中的构建与版本历史，不会删除 Apple 托管的 TestFlight Build。此操作不能撤销。'
   :'此操作不能撤销。',
  confirmLabel:hasInternalArtifact?'删除构建和测试包':(hasReleaseHistory?'删除构建和发布':'删除构建记录')
 });
 if(!confirmed)return;

 const old=button.textContent;
 button.disabled=true;
 button.textContent='正在删除…';
 try{
  const result=await api('/api/builds/'+encodeURIComponent(jobID),{method:'DELETE'});
  const deletedReleaseIDs=Array.isArray(result.deleted_release_ids)?result.deleted_release_ids:[];
  if(deletedReleaseIDs.length){
   const deleted=new Set(deletedReleaseIDs);
   state.releases=state.releases.filter(release=>!deleted.has(release.id));
  }
  if(hasInternalArtifact){
   state.internalTests=state.internalTests.filter(record=>record.id!==jobID);
  }
  if(buildState.activeLog===jobID)buildState.activeLog=null;
  buildState.collapsedLogs.delete(jobID);
  buildState.progressByJob.delete(jobID);
  delete simulatorState.buildDevices[jobID];
  button.closest('[data-build-job-id]')?.remove();

  if(deletedReleaseIDs.length||hasInternalArtifact){
   window.dispatchEvent(new CustomEvent('data-refreshed'));
  }
  window.dispatchEvent(new CustomEvent('build-record-deleted',{detail:{jobID,deletedReleaseIDs}}));

  let success=hasInternalArtifact
   ?'构建记录、日志和 Internal Test 测试包已删除。'
   :'构建记录和日志已删除。';
  if(deletedReleaseIDs.length){
   success='构建记录、日志和 '+deletedReleaseIDs.length+' 个关联版本历史已一起删除。';
  }
  notice(result.warning||success,result.warning?'warning':'success');
  await loadBuildJobs();
 }catch(error){
  notice(error.message,'error');
  button.disabled=false;
  button.textContent=old;
 }
}

export function initBuildDeletionUI(){
 const container=document.querySelector('#build-jobs');
 if(!container)return;
 // Capture the delete action before build-jobs.js's legacy bubbling handler so
 // the user sees the current cascade semantics and local Release state updates
 // immediately after the backend transaction succeeds.
 container.addEventListener('click',event=>{
  const button=event.target.closest('[data-delete-build-job]');
  if(!button)return;
  event.preventDefault();
  event.stopImmediatePropagation();
  void deleteBuildCascade(button);
 },true);
}


import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';
import {buildState} from './build-state.js';
import {startBuild} from './build-actions.js';

function profileSummary(profile){
 const standard=profile.result_contract==='ils-result-v1';
 const lane=profile.lane||(profile.platform==='ios'?'ios-adhoc':'macos-test');
 const metadata=standard
  ?'标准契约 · '+lane
  :(profile.platform==='ios'?'兼容模式：version/build 自动读取 IPA':'兼容模式 · version: '+profile.version_command+' · build: '+profile.build_number_command);
 const output=standard
  ?(lane==='ios-testflight'?'App Store Connect / TestFlight submission':'由 ILS_OUTPUT_DIR/ils-result.json 返回最终产物')
  :'产物：'+profile.artifact;
 return (profile.platform==='ios'?'iOS':'macOS')+' · '+escapeHTML(profile.architecture)+' · '+escapeHTML(profile.channel)+' · '+escapeHTML(profile.variant)+'<br>'+escapeHTML(metadata)+'<br>'+escapeHTML(output);
}

function renderProfiles(){
 const source=buildState.source;
 const blocked=!source||source.configured===false||source.blocker;
 $('#new-release-profile').disabled=!source||source.configured===false;

 if(!buildState.profiles.length){
  $('#release-profiles').innerHTML='<div class="profile-empty"><strong>还没有 ILS Release Profile</strong><p>在 ILS 中保存这个项目的构建、打包与发布参数。</p><button data-create-profile>创建 Release Profile</button></div>';
  return;
 }

 $('#release-profiles').innerHTML=buildState.profiles.map(profile=>
  '<article class="release">'+
   '<div class="release-top">'+
    '<div><strong>'+escapeHTML(profile.name)+'</strong><div class="meta">ILS / '+escapeHTML(profile.id)+'</div></div>'+
    '<div class="profile-actions"><button data-edit-profile="'+escapeHTML(profile.id)+'">编辑</button><button data-run-profile="'+escapeHTML(profile.id)+'" '+(blocked?'disabled':'')+'>构建并发布</button></div>'+
   '</div>'+
   '<p class="meta">'+profileSummary(profile)+'</p>'+
   '<details><summary>查看命令</summary><p><strong>Build</strong></p><pre>'+escapeHTML(profile.build_command)+'</pre>'+
    (profile.package_command?'<p><strong>Package</strong></p><pre>'+escapeHTML(profile.package_command)+'</pre>':'')+
   '</details>'+
  '</article>'
 ).join('');
}

export async function loadReleaseProfiles(){
 if(!state.project||!state.admin){
  buildState.profiles=[];
  renderProfiles();
  return;
 }
 const project=state.project;
 try{
  const profiles=await api('/api/projects/'+project+'/release-profiles');
  if(project!==state.project)return;
  buildState.profiles=profiles;
  renderProfiles();
 }catch(error){
  if(project===state.project)notice(error.message);
 }
}

function syncProfileForm(){
 const form=$('#profile-form');
 const platform=form.elements.platform.value;
 const contract=form.elements.result_contract.value;
 const lane=form.elements.lane;
 [...lane.options].forEach(option=>option.hidden=!option.value.startsWith(platform+'-'));
 if(!lane.value.startsWith(platform+'-'))lane.value=platform==='ios'?'ios-adhoc':'macos-test';
 const legacy=contract!=='ils-result-v1';
 form.elements.artifact.required=legacy;
 form.elements.version_command.required=legacy&&platform==='macos';
 form.elements.build_number_command.required=legacy&&platform==='macos';
 $('#profile-contract-help').textContent=legacy
  ?'兼容模式：ILS 根据 Artifact Path 发布；macOS 还需要 version/build 命令。'
  :(lane.value==='ios-testflight'
    ?'TestFlight：脚本上传 App Store Connect，并写入 ILS_OUTPUT_DIR/ils-result.json；不会创建伪造的本地安装包记录。'
    :'标准模式：脚本写入 ILS_OUTPUT_DIR/ils-result.json，ILS 验证最终产物后负责发布。');
}

function openProfile(profile=null){
 if(!needAdmin())return;
 if(!buildState.source||buildState.source.configured===false){
  notice('请先关联本地项目目录');
  return;
 }
 const form=$('#profile-form');
 form.reset();
 form.elements.id.readOnly=Boolean(profile);
 form.elements.platform.value='ios';
 form.elements.architecture.value='arm64';
 form.elements.channel.value='dev';
 form.elements.variant.value='default';
 form.elements.lane.value='ios-adhoc';
 form.elements.result_contract.value='ils-result-v1';
 if(profile){
  for(const [key,value] of Object.entries(profile)){
   if(form.elements[key])form.elements[key].value=value??'';
  }
 }
 syncProfileForm();
 $('#delete-profile').hidden=!profile;
 $('#profile-dialog-title').textContent=profile?'编辑 ILS Release Profile':'新建 ILS Release Profile';
 form.querySelector('.form-error').textContent='';
 $('#profile-dialog').showModal();
}

function resetProfiles(){
 buildState.profiles=[];
 $('#release-profiles').innerHTML='';
 $('#profile-form').reset();
}

export function initBuildProfiles(){
 $('#new-release-profile').onclick=()=>openProfile();
 $('#profile-form [name=platform]').onchange=syncProfileForm;
 $('#profile-form [name=lane]').onchange=syncProfileForm;
 $('#profile-form [name=result_contract]').onchange=syncProfileForm;

 $('#release-profiles').onclick=event=>{
  const create=event.target.closest('[data-create-profile]');
  if(create)return openProfile();

  const run=event.target.closest('[data-run-profile]');
  if(run)return startBuild({profile:run.dataset.runProfile},run);

  const edit=event.target.closest('[data-edit-profile]');
  if(edit){
   const profile=buildState.profiles.find(item=>item.id===edit.dataset.editProfile);
   if(profile)openProfile(profile);
  }
 };

 $('#profile-form').onsubmit=async event=>{
  event.preventDefault();
  if(!needAdmin()||!state.project)return;
  const button=event.target.querySelector('[type=submit]');
  button.disabled=true;
  const data=Object.fromEntries(new FormData(event.target));
  try{
   await api('/api/projects/'+state.project+'/release-profiles',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(data)
   });
   $('#profile-dialog').close();
   await loadReleaseProfiles();
   notice('ILS Release Profile 已保存到本机 ILS 数据目录。');
  }catch(error){
   event.target.querySelector('.form-error').textContent=error.message;
  }finally{
   button.disabled=false;
  }
 };

 $('#delete-profile').onclick=async()=>{
  const form=$('#profile-form');
  const id=form.elements.id.value;
  if(!id||!form.elements.id.readOnly)return;
  if(!confirm('删除 ILS Release Profile “'+id+'”？不会修改业务项目仓库。'))return;
  try{
   await api('/api/projects/'+state.project+'/release-profiles/'+encodeURIComponent(id),{method:'DELETE'});
   $('#profile-dialog').close();
   await loadReleaseProfiles();
   notice('Release Profile 已删除。');
  }catch(error){
   form.querySelector('.form-error').textContent=error.message;
  }
 };

 window.addEventListener('build-source-loaded',()=>{
  loadReleaseProfiles();
  renderProfiles();
 });
 window.addEventListener('project-changed',resetProfiles);
 window.addEventListener('admin-cleared',resetProfiles);
 window.addEventListener('admin-loaded',()=>{
  if(state.view==='builds')loadReleaseProfiles();
 });
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='builds'&&state.admin)loadReleaseProfiles();
 });
}

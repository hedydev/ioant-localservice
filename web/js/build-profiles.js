
import {$,state,api,escapeHTML,notice,needAdmin,confirmAction} from './core.js';
import {buildState} from './build-state.js';
import {startBuild} from './build-actions.js';
import {platformIcons,targetsForProfile,projectAppIcon} from './platform-ui.js';

let appleSigningTeams=[];

function buildTypeLabel(value){
 return ({native:'Native',expo:'Expo / React Native','hybrid-web-native':'Hybrid Web-Native',tauri:'Tauri / Rust'})[value]||value||'Native';
}

function ensureBuildTypeField(){
 const form=$('#profile-form');
 if(form.elements.build_type)return;
 const grid=form.querySelector('.form-grid');
 const label=document.createElement('label');
 label.innerHTML='构建类型<select name="build_type"><option value="native">Native</option><option value="expo">Expo / React Native</option><option value="hybrid-web-native">Hybrid Web-Native</option><option value="tauri">Tauri / Rust</option></select><span class="meta">仅用于描述构建方式；ILS 始终只调用下面一个标准脚本入口。</span>';
 const before=form.elements.result_contract?.closest('label')||null;
 grid.insertBefore(label,before);
}

function ensureSimulatorLaneOption(){
 const lane=$('#profile-form')?.elements?.lane;
 if(!lane||[...lane.options].some(option=>option.value==='ios-simulator'))return;
 const option=document.createElement('option');
 option.value='ios-simulator';
 option.textContent='iOS · Simulator Internal Test';
 const testflight=[...lane.options].find(item=>item.value==='ios-testflight');
 lane.insertBefore(option,testflight||null);
}

function teamLabel(team){
 const identity=team.identities?.[0]||'';
 const short=identity.replace(/\s*\([A-Z0-9]{10}\)\s*$/,'');
 const purpose=team.identities?.some(value=>value.includes('Apple Distribution:'))
  ?'发布 / TestFlight'
  :team.identities?.some(value=>value.includes('Apple Development:'))
   ?'开发签名'
   :'签名 Team';
 return purpose+' · '+team.id+(short?' · '+short:'');
}

async function loadAppleSigningTeams(selected=''){
 const form=$('#profile-form');
 const select=form.elements.apple_team_id;
 const help=$('#profile-apple-team-help');
 const save=form.querySelector('[type=submit]');
 select.disabled=true;
 save.disabled=true;
 help.textContent='正在读取这台 Mac 的 Apple 签名身份…';
 try{
  appleSigningTeams=await api('/api/local/apple-signing-teams');
  select.innerHTML='<option value="">自动检测（仅一个 Team 时）</option>'+
   appleSigningTeams.map(team=>'<option value="'+escapeHTML(team.id)+'">'+escapeHTML(teamLabel(team))+'</option>').join('');
  if(selected&&!appleSigningTeams.some(team=>team.id===selected)){
   select.insertAdjacentHTML('beforeend','<option value="'+escapeHTML(selected)+'">'+escapeHTML(selected)+' · 当前签名身份未检测到</option>');
  }
  if(selected)select.value=selected;
  else if(appleSigningTeams.length===1)select.value=appleSigningTeams[0].id;

  if(appleSigningTeams.length===0)help.textContent='未检测到有效代码签名身份；需要签名的 iOS 发布会在预检查阶段给出错误。';
  else if(appleSigningTeams.length===1)help.textContent='已检测到唯一 Apple Team，并自动选择。';
  else help.textContent='检测到 '+appleSigningTeams.length+' 个 Apple Team；Ad Hoc / TestFlight 必须明确选择一个。';
 }catch(error){
  appleSigningTeams=[];
  select.innerHTML='<option value="">自动检测（仅一个 Team 时）</option>';
  if(selected){
   select.insertAdjacentHTML('beforeend','<option value="'+escapeHTML(selected)+'">'+escapeHTML(selected)+'</option>');
   select.value=selected;
  }
  help.textContent='无法读取签名 Team：'+error.message;
 }finally{
  select.disabled=false;
  save.disabled=false;
  syncProfileForm();
 }
}

function profileSummary(profile){
 const standard=profile.result_contract==='ils-result-v1';
 const lane=profile.lane||(profile.platform==='ios'?'ios-adhoc':'macos-test');
 const simulator=lane==='ios-simulator';
 const metadata=standard
  ?'标准契约 · '+lane
  :(profile.platform==='ios'?'兼容模式：version/build 自动读取 IPA':'兼容模式 · version: '+profile.version_command+' · build: '+profile.build_number_command);
 const output=standard
  ?(lane==='ios-testflight'
    ?'App Store Connect / TestFlight submission'
    :simulator
     ?'Simulator .app ZIP · package-only · 可直接安装到已启动 Simulator'
     :'由 ILS_OUTPUT_DIR/ils-result.json 返回最终产物')
  :'产物：'+profile.artifact;
 const team=!simulator&&profile.apple_team_id?'<br>Apple Team · '+escapeHTML(profile.apple_team_id):'';
 const testflight=profile.testflight_url?'<br>TestFlight 邀请链接已配置':'';
 const group=profile.testflight_group_name
  ?'<br>TestFlight Group · '+escapeHTML(profile.testflight_group_name)+' · '+escapeHTML(profile.testflight_group_type||'internal')+
   (profile.testflight_create_group?' · 不存在时自动创建':'')+
   (profile.testflight_submit_beta_review?' · 自动提交 Beta Review':'')
  :'';
 const simulatorNote=simulator?'<br>Simulator Internal Test · 不需要 Apple Team / Provisioning / App Store Connect · 不创建 Release':'';
 return platformIcons(targetsForProfile(profile))+(profile.platform==='ios'?'iOS':profile.platform==='macos'?'macOS':escapeHTML(profile.platform))+' · '+escapeHTML(profile.architecture)+' · '+escapeHTML(profile.channel)+' · '+escapeHTML(profile.variant)+
  '<br>构建类型 · '+escapeHTML(buildTypeLabel(profile.build_type))+' · 单脚本入口'+
  '<br>'+escapeHTML(metadata)+'<br>'+escapeHTML(output)+team+testflight+group+simulatorNote;
}

function renderProfiles(){
 const source=buildState.source;
 const blocked=!source||source.configured===false;
 $('#new-release-profile').disabled=blocked;

 if(!buildState.profiles.length){
  $('#release-profiles').innerHTML='<div class="profile-empty"><strong>还没有 ILS Release Profile</strong><p>每个 Profile 描述构建类型和发布目标，但真正执行始终由一个标准脚本完成。</p><button data-create-profile>创建 Release Profile</button></div>';
  return;
 }

 $('#release-profiles').innerHTML=buildState.profiles.map(profile=>{
  const signingLane=profile.platform==='ios'&&profile.lane!=='ios-simulator';
  const teamRequired=signingLane&&appleSigningTeams.length>1&&!profile.apple_team_id;
  const runBlocked=blocked||teamRequired;
  const runLabel=profile.lane==='ios-simulator'?'构建 Simulator 测试包':profile.platform==='macos'&&profile.lane==='macos-test'?'构建测试包':'构建并发布';
  return '<article class="release">'+
   '<div class="release-top">'+
    '<div class="profile-title-block">'+projectAppIcon(state.project,profile.platform,{className:'profile-app-icon',title:profile.name})+'<div><strong>'+platformIcons(targetsForProfile(profile))+escapeHTML(profile.name)+'</strong><div class="meta">ILS / '+escapeHTML(profile.id)+'</div></div></div>'+
    '<div class="profile-actions"><button data-edit-profile="'+escapeHTML(profile.id)+'">编辑</button><button data-run-profile="'+escapeHTML(profile.id)+'" '+(runBlocked?'disabled':'')+'>'+escapeHTML(runLabel)+'</button></div>'+
   '</div>'+
   '<p class="meta">'+profileSummary(profile)+'</p>'+
   (teamRequired?'<div class="job-message">检测到多个 Apple Signing Team；请先编辑 Profile 并选择 Apple Team。</div>':'')+
   '<details><summary>查看标准脚本入口</summary><p><strong>Build</strong></p><pre>'+escapeHTML(profile.build_command)+'</pre>'+
    (profile.package_command?'<p><strong>Package（兼容）</strong></p><pre>'+escapeHTML(profile.package_command)+'</pre>':'')+
   '</details>'+
  '</article>';
 }).join('');
}

export async function loadReleaseProfiles(){
 if(!state.project||!state.admin){
  buildState.profiles=[];
  renderProfiles();
  return;
 }
 const project=state.project;
 try{
  const [profiles,teams]=await Promise.all([
   api('/api/projects/'+project+'/release-profiles'),
   api('/api/local/apple-signing-teams').catch(()=>null)
  ]);
  if(project!==state.project)return;
  buildState.profiles=profiles;
  if(Array.isArray(teams))appleSigningTeams=teams;
  renderProfiles();
 }catch(error){
  if(project===state.project)notice(error.message,'error');
 }
}

function syncProfileForm(){
 const form=$('#profile-form');
 ensureSimulatorLaneOption();
 const platform=form.elements.platform.value;
 const contract=form.elements.result_contract.value;
 const lane=form.elements.lane;
 [...lane.options].forEach(option=>option.hidden=!option.value.startsWith(platform+'-'));
 if(!lane.value.startsWith(platform+'-'))lane.value=platform==='ios'?'ios-adhoc':'macos-test';
 const legacy=contract!=='ils-result-v1';
 const multipleTeams=appleSigningTeams.length>1;
 const testflight=lane.value==='ios-testflight';
 const simulator=lane.value==='ios-simulator';
 const signingLane=platform==='ios'&&!simulator;
 form.elements.apple_team_id.required=signingLane&&multipleTeams;
 form.elements.apple_team_id.disabled=simulator;
 const teamHelp=$('#profile-apple-team-help');
 if(simulator)teamHelp.textContent='iOS Simulator 构建不签名，不需要 Apple Team 或 Provisioning Profile。';
 $('#profile-testflight-url-field').hidden=!testflight;
 $('#profile-testflight-automation').hidden=!testflight;
 form.elements.testflight_url.disabled=!testflight;
 form.elements.testflight_group_name.disabled=!testflight;
 form.elements.testflight_group_type.disabled=!testflight;
 form.elements.testflight_create_group.disabled=!testflight;
 form.elements.testflight_submit_beta_review.disabled=!testflight;
 const external=testflight&&form.elements.testflight_group_type.value==='external';
 form.elements.testflight_submit_beta_review.disabled=!external;
 if(!testflight){
  form.elements.testflight_url.value='';
  form.elements.testflight_group_name.value='';
  form.elements.testflight_group_type.value='internal';
  form.elements.testflight_create_group.checked=false;
  form.elements.testflight_submit_beta_review.checked=false;
 }
 if(!external)form.elements.testflight_submit_beta_review.checked=false;
 form.elements.artifact.required=legacy;
 form.elements.version_command.required=legacy&&platform==='macos';
 form.elements.build_number_command.required=legacy&&platform==='macos';
 $('#profile-contract-help').textContent=legacy
  ?'兼容模式：ILS 根据 Artifact Path 发布；macOS 还需要 version/build 命令。'
  :(testflight
    ?'单脚本模式：项目脚本完成预检查、构建和 TestFlight 上传，并写入 ILS_OUTPUT_DIR/ils-result.json。'
    :simulator
     ?'Simulator Internal Test：脚本构建 iphonesimulator .app，打包 ZIP 并写入 ils-result.json；ILS 不创建 Release，可直接安装到当前 Simulator。'
     :'单脚本模式：项目脚本完成项目内部所有构建步骤，写入 ILS_OUTPUT_DIR/ils-result.json，ILS 验证并发布最终产物。');
}

function openProfile(profile=null){
 if(!needAdmin())return;
 if(!buildState.source||buildState.source.configured===false){
  notice('请先关联本地项目目录');
  return;
 }
 ensureBuildTypeField();
 ensureSimulatorLaneOption();
 const form=$('#profile-form');
 form.reset();
 form.elements.id.readOnly=Boolean(profile);
 form.elements.platform.value='ios';
 form.elements.architecture.value='arm64';
 form.elements.channel.value='dev';
 form.elements.variant.value='default';
 form.elements.lane.value='ios-adhoc';
 form.elements.result_contract.value='ils-result-v1';
 form.elements.build_type.value='native';
 const selectedTeam=profile?.apple_team_id||'';
 if(profile){
  for(const [key,value] of Object.entries(profile)){
   if(key==='apple_team_id'||!form.elements[key])continue;
   if(form.elements[key].type==='checkbox')form.elements[key].checked=Boolean(value);
   else form.elements[key].value=value??'';
  }
 }
 form.elements.apple_team_id.innerHTML='<option value="">正在读取 Apple Team…</option>';
 syncProfileForm();
 $('#delete-profile').hidden=!profile;
 $('#profile-dialog-title').textContent=profile?'编辑 ILS Release Profile':'新建 ILS Release Profile';
 form.querySelector('.form-error').textContent='';
 $('#profile-dialog').showModal();
 loadAppleSigningTeams(selectedTeam);
}

function resetProfiles(){
 buildState.profiles=[];
 $('#release-profiles').innerHTML='';
 $('#profile-form').reset();
}

export function initBuildProfiles(){
 ensureBuildTypeField();
 ensureSimulatorLaneOption();
 $('#new-release-profile').onclick=()=>openProfile();
 $('#profile-form [name=platform]').onchange=syncProfileForm;
 $('#profile-form [name=lane]').onchange=syncProfileForm;
 $('#profile-form [name=result_contract]').onchange=syncProfileForm;
 $('#profile-form [name=testflight_group_type]').onchange=syncProfileForm;

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
  data.testflight_create_group=event.target.elements.testflight_create_group.checked;
  data.testflight_submit_beta_review=event.target.elements.testflight_submit_beta_review.checked;
  try{
   await api('/api/projects/'+state.project+'/release-profiles',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(data)
   });
   $('#profile-dialog').close();
   await loadReleaseProfiles();
   notice('ILS Release Profile 已保存。构建时只运行这一条标准脚本入口。','success');
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
  const confirmed=await confirmAction({
   title:'删除 Release Profile？',
   message:'删除 ILS Release Profile “'+id+'”？',
   detail:'只删除 ILS 本机的 Profile 配置，不会修改业务项目仓库。',
   confirmLabel:'删除 Profile'
  });
  if(!confirmed)return;
  try{
   await api('/api/projects/'+state.project+'/release-profiles/'+encodeURIComponent(id),{method:'DELETE'});
   $('#profile-dialog').close();
   await loadReleaseProfiles();
   notice('Release Profile 已删除。','success');
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

// ILS build and release management.
// Project-owned release*.sh scripts remain supported, while ILS Release Profiles
// keep build/package/publish configuration inside ILS instead of business repos.
function releaseMarkdown(markdown) {
 const inline = s => escape(s).replace(/`([^`]+)`/g,'<code>$1</code>').replace(/\*\*([^*]+)\*\*/g,'<strong>$1</strong>');
 let code=false, list=false, out='';
 for (const line of markdown.split('\n')) {
  if(line.startsWith('```')) {if(list){out+='</ul>';list=false;}out+=code?'</code></pre>':'<pre><code>';code=!code;continue;}
  if(code){out+=escape(line)+'\n';continue;}
  const item=line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)$/);
  if(item){if(!list){out+='<ul>';list=true;}out+=`<li>${inline(item[1])}</li>`;continue;}
  if(list){out+='</ul>';list=false;}
  const heading=line.match(/^(#{1,6})\s+(.*)$/);
  if(heading){out+=`<h3>${inline(heading[2])}</h3>`;}else if(line.trim()){out+=`<p>${inline(line)}</p>`;}
 }
 return out+(list?'</ul>':'')+(code?'</code></pre>':'');
}
let buildProject=null, activeLog=null, buildLoading=false, buildSourceState=null, releaseProfiles=[];

function profileSummary(p){
 const standard=p.result_contract==='ils-result-v1';
 const lane=p.lane||(p.platform==='ios'?'ios-adhoc':'macos-test');
 const metadata=standard
  ? `标准契约 · ${lane}`
  : (p.platform==='ios'?'兼容模式：version/build 自动读取 IPA':`兼容模式 · version: ${p.version_command} · build: ${p.build_number_command}`);
 const output=standard
  ? (lane==='ios-testflight'?'App Store Connect / TestFlight submission':'由 ILS_OUTPUT_DIR/ils-result.json 返回最终产物')
  : `产物：${p.artifact}`;
 return `${p.platform==='ios'?'iOS':'macOS'} · ${escape(p.architecture)} · ${escape(p.channel)} · ${escape(p.variant)}<br>${escape(metadata)}<br>${escape(output)}`;
}
function renderBuildDefinitions(){
 const source=buildSourceState;
 const blocked=!source||source.configured===false||source.blocker;
 $('#new-release-profile').disabled=!source||source.configured===false;
 $('#release-profiles').innerHTML=releaseProfiles.length
  ?'<h3>ILS Release Profiles</h3>'+releaseProfiles.map(p=>`<article class="release"><div class="release-top"><div><strong>${escape(p.name)}</strong><div class="meta">ILS / ${escape(p.id)}</div></div><div class="profile-actions"><button data-edit-profile="${escape(p.id)}">编辑</button><button data-run-profile="${escape(p.id)}" ${blocked?'disabled':''}>构建并发布</button></div></div><p class="meta">${profileSummary(p)}</p><details><summary>查看命令</summary><p><strong>Build</strong></p><pre>${escape(p.build_command)}</pre>${p.package_command?`<p><strong>Package</strong></p><pre>${escape(p.package_command)}</pre>`:''}</details></article>`).join('')
  :'<div class="profile-empty"><strong>还没有 ILS Release Profile</strong><p>在 ILS 中保存这个项目自己的构建、打包与发布参数，不需要修改业务仓库。</p><button data-create-profile>创建 Release Profile</button></div>';
 if(!source||source.configured===false){$('#build-scripts').innerHTML='';return;}
 $('#build-scripts').innerHTML='<h3>项目内发布脚本 <span class="meta">兼容模式</span></h3>'+
  (source.scripts.length?source.scripts.map(s=>`<article class="release"><div class="release-top"><div><strong>${escape(s.title)}</strong><div class="meta">${escape(s.path)}</div></div><button data-run-script="${escape(s.path)}" ${(!s.ready||source.blocker)?'disabled':''}>拉取并发布</button></div><p class="meta">${escape(s.reason||'已识别项目脚本与说明；实际产物以执行结果为准。')}</p>${s.markdown?`<details><summary>查看打包说明</summary><div class="release-markdown">${releaseMarkdown(s.markdown)}</div></details>`:''}</article>`).join('')
   :'<p class="meta">项目中没有 release*.sh。可以直接使用上面的 ILS Release Profile，无需给业务仓库添加发布脚本。</p>');
}
async function loadReleaseProfiles(){
 if(!state.project||!state.token){releaseProfiles=[];renderBuildDefinitions();return;}
 const id=state.project;
 try{const profiles=await api(`/api/projects/${id}/release-profiles`);if(state.project!==id)return;releaseProfiles=profiles;renderBuildDefinitions();}
 catch(e){if(state.project===id)notice(e.message);}
}
async function loadBuildSource(){
 if(!state.project||!state.token)return;
 const id=state.project;$('#source-info').textContent='正在读取 Git、ILS Profiles 与项目发布脚本…';
 try{
  const source=await api(`/api/projects/${id}/build-source`);
  if(state.project!==id)return;
  buildProject=id;buildSourceState=source;
  if(source.configured===false){
   $('#source-info').textContent='尚未关联本地目录。先关联项目目录，再创建 ILS Release Profile。';
   renderBuildDefinitions();await loadReleaseProfiles();return;
  }
  $('#source-form [name=path]').value=source.path;$('#source-form [name=branch]').value=source.branch;
  $('#source-info').textContent=`${source.path}\n分支 ${source.current_branch||'detached'} → ${source.upstream||'未配置 upstream'}\n${source.remote||''}\n${source.blocker||'工作目录干净，可由 ILS 拉取、构建并发布。'}`;
  await loadReleaseProfiles();renderBuildDefinitions();
 }catch(e){if(state.project===id){buildSourceState=null;$('#source-info').textContent=e.message;renderBuildDefinitions();}}
}
async function loadBuildJobs(){
 if(!state.project||!state.token||buildLoading)return;buildLoading=true;
 const id=state.project;
 try{
  const jobs=await api(`/api/projects/${id}/builds`);if(state.project!==id)return;
  const stages={
   preflight:'预检查',pull:'正在拉取',build:'正在构建',archive:'正在归档',validate:'正在验证',
   export:'正在导出',package:'正在打包',notarize:'正在公证',upload:'正在上传',
   publish:'正在发布到 ILS',submitted:'已提交',processing:'Apple 处理中',
   metadata:'读取版本',script:'项目脚本执行中',complete:'完成'
  };
  const statusLabel=j=>{
   if(j.status==='running')return stages[j.stage]||'处理中';
   if(j.status==='succeeded'&&j.result?.lane==='ios-testflight')return 'TestFlight 已提交';
   if(j.status==='succeeded')return '发布成功';
   return '失败';
  };
  const progressView=j=>{
   if(j.status!=='running'||!Number.isFinite(j.progress))return '';
   const value=Math.max(0,Math.min(100,j.progress));
   return `<div class="job-progress"><progress max="100" value="${value}"></progress><span>${value}%</span></div>`;
  };
  const resultView=j=>{
   if(!j.result)return '';
   const r=j.result;
   if(r.lane==='ios-testflight'){
    return `<div class="submission-result"><strong>TestFlight</strong><span>${escape(r.version)} (${escape(r.build)})</span><span>${escape(r.submission_result||r.status)}</span><small>上传成功不等于 Apple 已完成 Processing。</small></div>`;
   }
   return `<div class="meta">结果：${escape(r.lane||'artifact')} · ${escape(r.version||'')} (${escape(r.build||'')})</div>`;
  };
  $('#build-jobs').innerHTML=jobs.length?'<h3>构建记录</h3>'+jobs.map(j=>`<article class="build-job"><div class="build-job-main"><div><strong>${escape(j.title||j.profile_id||j.script||'ILS Build')}</strong> <span class="badge">${escape(statusLabel(j))}</span></div><div class="meta">${escape(j.mode==='profile'?'ILS Profile':'Project Script')} · ${date(j.created_at)} · ${escape((j.commit||'').slice(0,12))}</div>${j.message?`<div class="job-message">${escape(j.message)}</div>`:''}${progressView(j)}${j.error?`<p>${escape(j.error)}</p>`:''}${resultView(j)}${j.release_ids.length?`<div class="meta">关联安装包：${j.release_ids.length} 个</div>`:''}${j.release_ids.map(r=>`<a class="build-artifact" href="/api/releases/${escape(r)}/download">下载 ${escape(r.slice(0,8))}</a>`).join(' ')}</div><button data-build-log="${escape(j.id)}">查看日志</button></article>`).join(''):'';
  if(activeLog){const data=await api(`/api/builds/${activeLog}/log`);if(state.project!==id)return;$('#build-log').textContent=data.log;$('#build-log').hidden=false;}
 }catch(e){if(state.project===id)$('#source-info').textContent=e.message;}finally{buildLoading=false;}
}
function syncProfileForm(){
 const form=$('#profile-form');
 const platform=form.elements.platform.value;
 const contract=form.elements.result_contract.value;
 const lane=form.elements.lane;
 [...lane.options].forEach(o=>o.hidden=!o.value.startsWith(platform+'-'));
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
 if(!buildSourceState||buildSourceState.configured===false){notice('请先关联本地项目目录');return;}
 const form=$('#profile-form');form.reset();
 form.elements.id.readOnly=Boolean(profile);
 form.elements.platform.value='ios';form.elements.architecture.value='arm64';form.elements.channel.value='dev';form.elements.variant.value='default';
 form.elements.lane.value='ios-adhoc';form.elements.result_contract.value='ils-result-v1';
 if(profile){for(const [key,value] of Object.entries(profile)){if(form.elements[key])form.elements[key].value=value??'';}}
 syncProfileForm();
 $('#delete-profile').hidden=!profile;
 $('#profile-dialog-title').textContent=profile?'编辑 ILS Release Profile':'新建 ILS Release Profile';
 form.querySelector('.form-error').textContent='';
 $('#profile-dialog').showModal();
}
async function startBuild(payload,button){
 if(!needAdmin())return;
 const id=state.project;button.disabled=true;
 try{
  const job=await api(`/api/projects/${id}/builds`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});
  if(id!==state.project)return;
  activeLog=job.id;await loadBuildJobs();notice('ILS 发布任务已启动；构建、上传和 TestFlight 提交状态会在这里持续更新。');
 }catch(e){notice(e.message);}finally{button.disabled=false;}
}
$('#scan-builds').onclick=()=>{if(needAdmin()){loadBuildSource();loadBuildJobs();}};
$('#source-form').onsubmit=async e=>{e.preventDefault();if(!needAdmin())return;if(!state.project)return notice('请先创建项目');const b=e.target.querySelector('[type=submit]');b.disabled=true;try{await api(`/api/projects/${state.project}/build-source`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(Object.fromEntries(new FormData(e.target)))});await loadBuildSource();notice('项目目录已关联，现在可以创建 ILS Release Profile。');}catch(e){notice(e.message);}finally{b.disabled=false;}};
$('#new-release-profile').onclick=()=>openProfile();
$('#profile-form [name=platform]').onchange=syncProfileForm;
$('#profile-form [name=lane]').onchange=syncProfileForm;
$('#profile-form [name=result_contract]').onchange=syncProfileForm;
$('#release-profiles').onclick=async e=>{
 const create=e.target.closest('[data-create-profile]');if(create)return openProfile();
 const run=e.target.closest('[data-run-profile]');if(run)return startBuild({profile:run.dataset.runProfile},run);
 const edit=e.target.closest('[data-edit-profile]');if(edit){const p=releaseProfiles.find(x=>x.id===edit.dataset.editProfile);if(p)openProfile(p);return;}
};
$('#profile-form').onsubmit=async e=>{
 e.preventDefault();if(!needAdmin()||!state.project)return;
 const button=e.target.querySelector('[type=submit]');button.disabled=true;
 const data=Object.fromEntries(new FormData(e.target));
 try{
  await api(`/api/projects/${state.project}/release-profiles`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(data)});
  $('#profile-dialog').close();await loadReleaseProfiles();notice('ILS Release Profile 已保存到本机 ILS 数据目录。');
 }catch(err){e.target.querySelector('.form-error').textContent=err.message;}finally{button.disabled=false;}
};
$('#delete-profile').onclick=async()=>{
 const form=$('#profile-form'),id=form.elements.id.value;if(!id||!form.elements.id.readOnly)return;
 if(!confirm(`删除 ILS Release Profile “${id}”？不会修改业务项目仓库。`))return;
 try{await api(`/api/projects/${state.project}/release-profiles/${encodeURIComponent(id)}`,{method:'DELETE'});$('#profile-dialog').close();await loadReleaseProfiles();notice('Release Profile 已删除。');}
 catch(e){form.querySelector('.form-error').textContent=e.message;}
};
$('#build-scripts').onclick=e=>{const b=e.target.closest('[data-run-script]');if(b)startBuild({script:b.dataset.runScript},b);};
$('#build-jobs').onclick=e=>{const b=e.target.closest('[data-build-log]');if(b){activeLog=b.dataset.buildLog;loadBuildJobs();}};
function clearBuildPanel(){buildProject=null;activeLog=null;buildSourceState=null;releaseProfiles=[];$('#release-profiles').innerHTML='';$('#build-scripts').innerHTML='';$('#build-jobs').innerHTML='';$('#build-log').hidden=true;$('#source-form').reset();$('#source-info').textContent='关联已有 Git 项目后，可直接在 ILS 创建 Release Profile；项目内 release*.sh 仅作为兼容方式。';renderBuildDefinitions();loadBuildSource();loadBuildJobs();}
window.addEventListener('project-changed',clearBuildPanel);
window.addEventListener('admin-loaded',()=>{loadBuildSource();loadBuildJobs();});
setInterval(()=>{if(!document.hidden&&state.token)loadBuildJobs();},3000);
$('#choose-project-folder').onclick=async()=>{
 if(!needAdmin())return;
 const button=$('#choose-project-folder');const project=state.project;
 button.disabled=true;button.textContent='等待 Mac 选择…';
 $('#source-info').textContent='请在运行 ILS 的 Mac 上选择项目文件夹；首次使用可能需要允许控制 Finder。';
 try{
  const result=await api('/api/local/select-folder',{method:'POST'});
  if(state.project!==project)return;
  if(result.cancelled){$('#source-info').textContent='已取消选择，原路径未更改。';return;}
  $('#source-form [name=path]').value=result.path;
  $('#source-info').textContent='已选择 Git 项目：'+result.path+'。点击“关联目录”后即可配置 ILS Release Profile。';
  $('#source-form [type=submit]').focus();
 }catch(e){if(state.project===project)$('#source-info').textContent=e.message;}
 finally{button.disabled=false;button.textContent='选择文件夹…';}
};

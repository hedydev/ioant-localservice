const $ = s => document.querySelector(s);
const state = {projects:[], releases:[], project:null, platform:'all', channel:'all', variant:'all', token:'', admin:false, authEpoch:0, job:null};
const escape = value => String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const channelNames = {dev:'开发版',beta:'测试版',stable:'正式版'};
const size = bytes => bytes >= 1073741824 ? `${(bytes/1073741824).toFixed(2)} GB` : `${(bytes/1048576).toFixed(1)} MB`;
const date = value => new Date(value).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});
async function api(path, options={}) {
 const epoch=state.authEpoch;
 const response=await fetch(path,{...options,headers:{...(state.admin?{Authorization:`Bearer ${state.token}`} : {}),...options.headers}});
 const data=await response.json();
 if(epoch!==state.authEpoch)throw new Error('管理会话已更改，请重新操作');
 if(response.status===401&&state.admin){setAdmin(false);}
 if(!response.ok)throw new Error(data.error||'请求失败');
 return data;
}
function setAdmin(authenticated,token='') {
 state.authEpoch++;state.admin=authenticated;state.token=authenticated?token:'';state.job=null;
 document.querySelectorAll('[data-admin]').forEach(el=>{
  if(!authenticated&&el.tagName==='DIALOG'&&el.open)el.close();
  el.hidden=!authenticated;
 });
 $('#admin-button').textContent=authenticated?'管理员已验证':'管理员验证';
 $('#admin-button').hidden=authenticated;
 if(!authenticated){
  ['#project-form','#publish-form','#profile-form'].forEach(selector=>$(selector).reset());
  $('#devices-list').textContent='';$('#devices-list').hidden=true;
  $('#sign-state').textContent='';$('#sign-button').disabled=false;
 }
 render();
 window.dispatchEvent(new Event(authenticated?'admin-loaded':'admin-cleared'));
}
function notice(message){$('#notice').textContent=message;$('#notice').hidden=!message;}
function needAdmin(){if(state.admin)return true;$('#admin-dialog').showModal();return false;}
function installNote(r){if(r.platform!=='ios')return `${r.architecture} · macOS`;const i=r.ios;if(!i)return '签名状态未知';const name={development:'开发签名 · 需要开发者模式，使用 Xcode / Configurator 安装','ad-hoc':'Ad Hoc · 仅限描述文件内登记设备',enterprise:'企业分发 · 仅限组织内部',unsigned:'未签名 · 不能直接安装','app-store':'App Store 描述文件 · 不支持此处直接安装'}[i.profile_type]||i.profile_type;return name+(i.expires_at?` · ${new Date(i.expires_at).toLocaleDateString('zh-CN')} 到期`:'');}
function actions(r){return `<div class="download-row">${r.install_url?`<a class="download" href="${escape(r.install_url)}">尝试安装到 iPhone</a>`:''}<a class="download ${r.install_url?'secondary':''}" href="${escape(r.download_url)}">下载 ${escape(r.filename.split('.').pop().toUpperCase())}</a><span class="meta">${size(r.size)} · ${escape(r.architecture)} · ${escape(r.variant||'default')}</span></div>`;}
function filtered(){return state.releases.filter(r=>(state.platform==='all'||r.platform===state.platform)&&(state.channel==='all'||r.channel===state.channel)&&(state.variant==='all'||(r.variant||'default')===state.variant));}
function render(){
 $('#projects').innerHTML=state.projects.map(p=>`<button data-project="${escape(p.id)}" class="${p.id===state.project?'active':''}" ${p.id===state.project?'aria-current="page"':''}>${escape(p.name)}<small>${escape(p.id)}</small></button>`).join('');
 const p=state.projects.find(p=>p.id===state.project);$('#project-title').textContent=p?.name||'安装包分发';$('#project-subtitle').textContent=p?`${p.id} / ${state.admin?'构建与发布':'安装包下载'}`:(state.admin?'创建第一个项目，开始在局域网内分发安装包。':'查看已发布的安装包与历史版本。');
 const variants=[...new Set(state.releases.map(r=>r.variant||'default'))];if(state.variant!=='all'&&!variants.includes(state.variant))state.variant='all';$('#variant').innerHTML='<option value="all">全部安装包</option>'+variants.map(v=>`<option value="${escape(v)}" ${v===state.variant?'selected':''}>${escape(v)}</option>`).join('');
 const rows=filtered();$('#release-count').textContent=rows.length;
 $('#latest').innerHTML=rows.length?`<article class="latest-card"><div class="card-top"><span>最新匹配构建 · ${rows[0].platform==='ios'?'iOS':'macOS'}</span><span class="badge">${channelNames[rows[0].channel]}</span></div><div class="version-title">${escape(rows[0].version)} <small>build ${rows[0].build}</small></div><p>${escape(rows[0].notes||'暂无更新说明')}</p>${actions(rows[0])}<p class="meta">${escape(installNote(rows[0]))}</p></article>`:`<div class="empty"><strong>${p?'还没有匹配的安装包':'暂无已发布安装包'}</strong>${state.admin?(p?'上传一个构建，或调整平台与渠道筛选。':'点击左侧 ＋ 创建项目，然后上传第一个安装包。'):'可调整筛选条件，或等待管理员发布新版本。'}</div>`;
 $('#releases').innerHTML=rows.map(r=>`<article class="release"><div class="release-top"><div class="release-title">${escape(r.version)} <span class="badge">${r.platform==='ios'?'iOS':'macOS'} · ${channelNames[r.channel]}</span><div class="meta">build ${r.build} · ${date(r.created_at)}</div></div><span class="meta">${size(r.size)}</span></div><p>${escape(r.notes||'暂无更新说明')}</p>${actions(r)}<details><summary>包信息与签名条件</summary><p>${escape(installNote(r))}</p><p>${escape(r.filename)}</p><code>SHA-256: ${escape(r.sha256)}</code>${r.ios?`<p>${escape(r.bundle_id)} · 描述文件包含 ${r.ios.device_count} 台设备。此信息不代表签名已验证或当前设备获准安装。</p>`:''}</details></article>`).join('');
 $('#api-example').textContent=p?`LOCALSERVICE_URL=http://这台Mac的局域网IP:8787 \\\nLOCALSERVICE_TOKEN_FILE=/path/to/admin-token \\\n./scripts/push.sh ${p.id} 1.0.0 1 ios /path/to/app.ipa dev arm64`:'选择项目后显示命令';
}
async function refresh(){const projects=await api('/api/projects');state.projects=projects;if(!state.project||!projects.some(p=>p.id===state.project))state.project=projects[0]?.id||null;const current=state.project;const releases=current?await api(`/api/projects/${current}/releases`):[];if(current!==state.project)return;state.releases=releases;render();$('#connection').textContent='服务已连接';}
$('#projects').addEventListener('click',async e=>{const b=e.target.closest('[data-project]');if(!b)return;state.project=b.dataset.project;window.dispatchEvent(new Event('project-changed'));try{await refresh();notice('');}catch(e){notice(e.message);}});
$('#platforms').addEventListener('click',e=>{const b=e.target.closest('[data-platform]');if(!b)return;state.platform=b.dataset.platform;document.querySelectorAll('[data-platform]').forEach(x=>x.setAttribute('aria-pressed',String(x===b)));render();});
$('#channel').addEventListener('change',e=>{state.channel=e.target.value;render();});
$('#refresh').onclick=()=>refresh().then(()=>notice('')).catch(e=>notice(e.message));
$('#admin-button').onclick=()=>$('#admin-dialog').showModal();
let adminAttempt=0;
$('#admin-form').onsubmit=async e=>{
 e.preventDefault();const attempt=++adminAttempt;const token=$('#token').value.trim();const button=e.target.querySelector('[type=submit]');
 button.disabled=true;$('#admin-error').textContent='';
 try{
  const response=await fetch('/api/admin/session',{headers:{Authorization:`Bearer ${token}`}});
  const result=await response.json();
  if(!response.ok||result.authenticated!==true)throw new Error(result.error||'管理员验证失败');
  if(!$('#admin-dialog').open||attempt!==adminAttempt)return;
  $('#token').value='';$('#admin-dialog').close();setAdmin(true,token);notice('管理员验证成功。');
 }catch(err){$('#token').value='';$('#admin-error').textContent=err.message;}
 finally{button.disabled=false;}
};
$('#logout-button').onclick=()=>{setAdmin(false);notice('已退出管理，现在仅显示已发布安装包。');};
$('#admin-dialog').addEventListener('close',()=>{adminAttempt++;$('#token').value='';});
document.querySelectorAll('[data-close]').forEach(b=>b.onclick=()=>b.closest('dialog').close());
$('#new-project').onclick=()=>{if(needAdmin())$('#project-dialog').showModal();};
$('#publish-button').onclick=()=>{if(!needAdmin())return;if(!state.project){$('#project-dialog').showModal();return;}$('#publish-dialog').showModal();};
async function submit(form,fn){const button=form.querySelector('[type=submit]');button.disabled=true;const old=button.textContent;button.textContent='处理中…';form.querySelector('.form-error').textContent='';try{await fn();form.closest('dialog').close();form.reset();await refresh();notice('');}catch(e){form.querySelector('.form-error').textContent=e.message;}finally{button.disabled=false;button.textContent=old;}}
$('#project-form').onsubmit=e=>{e.preventDefault();submit(e.target,async()=>{const p=await api('/api/projects',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(Object.fromEntries(new FormData(e.target)))});state.project=p.id;});};
$('#publish-form').onsubmit=e=>{e.preventDefault();const project=state.project;submit(e.target,()=>api(`/api/projects/${project}/releases`,{method:'POST',body:new FormData(e.target)}));};
$('#check-form').onsubmit=async e=>{e.preventDefault();if(!state.project)return notice('请先创建项目');try{const data=await api(`/api/projects/${state.project}/updates?${new URLSearchParams(new FormData(e.target))}`);$('#check-result').textContent=data.latest?(data.update_available?`有更新：${data.latest.version} (build ${data.latest.build})`:'当前版本已是最新，或高于服务中的版本。'):'此平台、架构和渠道还没有安装包。';}catch(e){$('#check-result').textContent=e.message;}};
$('#sign-button').onclick=async()=>{if(!needAdmin())return;if(!state.project)return notice('请先创建项目');const b=$('#sign-button');b.disabled=true;try{const job=await api(`/api/projects/${state.project}/signing`,{method:'POST'});state.job=job.id;$('#sign-state').textContent='Mac 正在导出并签名，完成后自动发布。';}catch(e){$('#sign-state').textContent=e.message;b.disabled=false;}};
async function pollJob(){if(!state.admin||!state.job)return;try{const j=await api(`/api/signing/${state.job}`);$('#sign-state').textContent={running:'Mac 正在签名…',succeeded:'签名完成，安装包已发布。',failed:j.error||'签名失败'}[j.status];if(j.status!=='running'){state.job=null;$('#sign-button').disabled=false;await refresh();}}catch(e){$('#sign-state').textContent=e.message;state.job=null;$('#sign-button').disabled=false;}}
refresh().catch(e=>{$('#connection').textContent='连接失败';notice(e.message);});
setInterval(()=>{if(!document.hidden){refresh().catch(()=>{$('#connection').textContent='连接中断';});pollJob();}},20000);

$('#devices-button').onclick=async()=>{if(!needAdmin())return;try{const devices=await api('/api/devices');$('#devices-list').hidden=false;$('#devices-list').textContent=devices.length?devices.map(d=>`${d.product} / iOS ${d.version}\nUDID: ${d.udid}\n待 Apple 团队登记 · 设备身份尚未认证`).join('\n\n'):'还没有收集到设备。';}catch(e){notice(e.message);}};
if(new URLSearchParams(location.search).get('enrollment')==='collected')notice('设备信息已收集。请由管理员登记到 Apple 团队并更新分发描述文件，再请求签名。');

$('#variant').onchange=e=>{state.variant=e.target.value;render();};

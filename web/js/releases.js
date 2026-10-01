
import {$,state,api,escapeHTML,channelNames,formatSize,formatDate,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';
import {platformIcons,targetsForRelease,releaseAppIcon} from './platform-ui.js';

function installNote(release){
 if(release.platform!=='ios')return release.architecture+' · macOS';
 const info=release.ios;
 if(!info)return '签名状态未知';
 const names={
  development:'开发签名 · 需要开发者模式，使用 Xcode / Configurator 安装',
  'ad-hoc':'Ad Hoc · 仅限描述文件内登记设备',
  enterprise:'企业分发 · 仅限组织内部',
  unsigned:'未签名 · 不能直接安装',
  'app-store':'App Store 描述文件 · 不支持此处直接安装'
 };
 return (names[info.profile_type]||info.profile_type)+(info.expires_at?' · '+new Date(info.expires_at).toLocaleDateString('zh-CN')+' 到期':'');
}

function releaseActions(release){
 let html='<div class="download-row">';
 if(release.install_url)html+='<a class="download" href="'+escapeHTML(release.install_url)+'">尝试安装到 iPhone</a>';
 html+='<a class="download '+(release.install_url?'secondary':'')+'" href="'+escapeHTML(release.download_url)+'">下载 '+escapeHTML(release.filename.split('.').pop().toUpperCase())+'</a>';
 html+='<span class="meta">'+formatSize(release.size)+' · '+escapeHTML(release.architecture)+' · '+escapeHTML(release.variant||'default')+'</span></div>';
 return html;
}

function filteredReleases(){
 return state.releases.filter(release=>
  (state.platform==='all'||release.platform===state.platform)&&
  (state.channel==='all'||release.channel===state.channel)&&
  (state.variant==='all'||(release.variant||'default')===state.variant)
 );
}

function renderOverview(){
 const latest=state.releases[0];
 const iosCount=state.releases.filter(item=>item.platform==='ios').length;
 const macCount=state.releases.filter(item=>item.platform==='macos').length;
 $('#overview-stats').innerHTML=[
  ['全部版本',state.releases.length],
  ['iOS',iosCount],
  ['macOS',macCount]
 ].map(item=>'<div class="overview-stat"><strong>'+item[1]+'</strong><span>'+platformIcons(item[0]==='iOS'?['iphone','ipad']:item[0]==='macOS'?['mac']:[]) + item[0]+'</span></div>').join('');

 if(!latest){
  $('#overview-latest').innerHTML='<div class="empty"><strong>暂无已发布安装包</strong>发布完成后，最新版本会显示在这里。</div>';
  return;
 }
 $('#overview-latest').innerHTML=
  '<article class="latest-card">'+
   '<div class="latest-app-summary">'+releaseAppIcon(latest,{className:'latest-app-icon',title:'App Icon'})+'<div class="latest-app-copy">'+
    '<div class="card-top"><span>'+platformIcons(targetsForRelease(latest))+'最新发布 · '+(latest.platform==='ios'?'iOS':latest.platform==='macos'?'macOS':escapeHTML(latest.platform))+'</span><span class="badge">'+channelNames[latest.channel]+'</span></div>'+
    '<div class="version-title">'+escapeHTML(latest.version)+' <small>build '+latest.build+'</small></div>'+
   '</div></div>'+
   '<p>'+escapeHTML(latest.notes||'暂无更新说明')+'</p>'+
   releaseActions(latest)+
   '<p class="meta">'+escapeHTML(installNote(latest))+'</p>'+
  '</article>';
}

function renderReleaseHistory(){
 const variants=[...new Set(state.releases.map(item=>item.variant||'default'))];
 if(state.variant!=='all'&&!variants.includes(state.variant))state.variant='all';
 $('#variant').innerHTML='<option value="all">全部安装包</option>'+variants.map(value=>
  '<option value="'+escapeHTML(value)+'" '+(value===state.variant?'selected':'')+'>'+escapeHTML(value)+'</option>'
 ).join('');

 const rows=filteredReleases();
 $('#release-count').textContent=rows.length;
 if(!rows.length){
  $('#releases').innerHTML='<div class="empty"><strong>还没有匹配的安装包</strong>调整平台、渠道或安装包筛选。</div>';
  return;
 }
 $('#releases').innerHTML=rows.map(release=>{
  let details='<details><summary>包信息与签名条件</summary><p>'+escapeHTML(installNote(release))+'</p><p>'+escapeHTML(release.filename)+'</p><code>SHA-256: '+escapeHTML(release.sha256)+'</code>';
  if(release.ios)details+='<p>'+escapeHTML(release.bundle_id)+' · 描述文件包含 '+release.ios.device_count+' 台设备。此信息不代表签名已验证或当前设备获准安装。</p>';
  details+='</details>';
  return '<article class="release">'+
   '<div class="release-top">'+
    '<div class="release-identity">'+releaseAppIcon(release,{className:'release-app-icon',title:'App Icon'})+
     '<div class="release-title">'+escapeHTML(release.version)+' <span class="badge">'+platformIcons(targetsForRelease(release),{className:'compact'})+(release.platform==='ios'?'iOS':release.platform==='macos'?'macOS':escapeHTML(release.platform))+' · '+channelNames[release.channel]+'</span><div class="meta">build '+release.build+' · '+formatDate(release.created_at)+'</div></div>'+
    '</div>'+
    '<span class="meta">'+formatSize(release.size)+'</span>'+
   '</div>'+
   '<p>'+escapeHTML(release.notes||'暂无更新说明')+'</p>'+
   releaseActions(release)+details+
  '</article>';
 }).join('');
}

function renderReleases(){
 renderOverview();
 renderReleaseHistory();
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
  notice('安装包已发布。');
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
    :'此平台、架构和渠道还没有安装包。';
  }catch(error){
   $('#check-result').textContent=error.message;
  }
 };

 renderReleases();
}

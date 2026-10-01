import {escapeHTML,channelNames,formatSize,formatDate} from './core.js';
import {platformIcons,targetsForRelease,releaseAppIcon} from './platform-ui.js';

const statusLabels={
 published:'已发布',
 submitted:'已提交到 App Store Connect',
 processing:'Apple Processing',
 available:'TestFlight 可测试',
 unavailable:'当前不可测试'
};

export function releaseStatusLabel(release){
 return statusLabels[release?.status]||release?.status||'已发布';
}

export function releaseDeliveryLabel(release){
 if(release?.delivery==='testflight')return 'TestFlight';
 if(release?.platform==='ios')return 'Ad Hoc / IPA';
 return '安装包';
}

export function releaseInstallNote(release){
 if(release.delivery==='testflight'){
  const parts=[release.status_message||releaseStatusLabel(release)];
  if(release.testflight?.internal_build_state)parts.push('Internal '+release.testflight.internal_build_state);
  if(release.testflight?.external_build_state)parts.push('External '+release.testflight.external_build_state);
  return parts.join(' · ');
 }
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

export function testFlightLinkSource(release){
 if(release?.delivery!=='testflight'||!release.open_url)return '';
 if(release.testflight?.public_link&&release.testflight.public_link===release.open_url)return 'Apple Public Link';
 if(release.testflight?.fallback_url&&release.testflight.fallback_url===release.open_url)return 'Profile fallback';
 return 'TestFlight link';
}

export function testFlightLifecycleView(release,{compact=false}={}){
 if(release?.delivery!=='testflight')return '';
 const status=release.status||'submitted';
 const states=['submitted','processing','available'];
 const labels={submitted:'已提交',processing:'Apple Processing',available:'可测试'};
 const current=states.indexOf(status);
 const buildSeen=Boolean(release.testflight?.apple_build_id);
 const steps=states.map((stage,index)=>{
  let state='waiting';
  if(status==='unavailable'){
   if(index===2)state='failed';
   else if(index===0||buildSeen)state='done';
  }else if(current>=0){
   if(index<current)state='done';
   else if(index===current)state=status==='available'?'done':'active';
  }
  return '<span class="release-lifecycle-step '+state+'">'+escapeHTML(labels[stage])+'</span>';
 }).join('<span class="release-lifecycle-arrow">→</span>');
 const checked=release.testflight?.last_checked_at
  ?'<span class="release-last-sync">最近同步 '+escapeHTML(formatDate(release.testflight.last_checked_at))+'</span>'
  :'';
 return '<div class="release-lifecycle'+(compact?' compact':'')+'" aria-label="TestFlight 发布状态">'+steps+checked+'</div>';
}

export function releaseActionsView(release){
 let html='<div class="download-row">';
 if(release.delivery==='testflight'){
  if(release.open_url){
   html+='<a class="download" href="'+escapeHTML(release.open_url)+'">在 TestFlight 中打开</a>';
   const source=testFlightLinkSource(release);
   if(source)html+='<span class="release-link-source">'+escapeHTML(source)+'</span>';
  }else if(release.status==='available'){
   html+='<span class="meta">构建已可测试；尚未发现 Public Link</span>';
  }else{
   html+='<span class="meta">TestFlight 链接尚未可用</span>';
  }
  html+='</div>';
  return html;
 }
 if(release.install_url)html+='<a class="download" href="'+escapeHTML(release.install_url)+'">安装到 iPhone / iPad</a>';
 if(release.download_url){
  const ext=(release.filename||'artifact').split('.').pop().toUpperCase();
  html+='<a class="download '+(release.install_url?'secondary':'')+'" href="'+escapeHTML(release.download_url)+'">下载 '+escapeHTML(ext)+'</a>';
 }
 html+='<span class="meta">'+formatSize(release.size)+' · '+escapeHTML(release.architecture)+' · '+escapeHTML(release.variant||'default')+'</span></div>';
 return html;
}

function releaseDetailsView(release){
 let details='<details><summary>发布信息</summary><p>'+escapeHTML(releaseInstallNote(release))+'</p>';
 if(release.delivery==='testflight'){
  details+='<p>'+escapeHTML(release.bundle_id||'')+' · TestFlight</p>';
  if(release.testflight?.processing_state)details+='<p>Apple Processing: '+escapeHTML(release.testflight.processing_state)+'</p>';
  if(release.testflight?.internal_build_state)details+='<p>Internal Beta: '+escapeHTML(release.testflight.internal_build_state)+'</p>';
  if(release.testflight?.external_build_state)details+='<p>External Beta: '+escapeHTML(release.testflight.external_build_state)+'</p>';
  if(release.testflight?.public_link)details+='<p>Public Link: '+escapeHTML(release.testflight.public_link)+'</p>';
  if(release.testflight?.fallback_url)details+='<p>Profile fallback: '+escapeHTML(release.testflight.fallback_url)+'</p>';
  if(release.testflight?.last_error)details+='<p class="form-error">'+escapeHTML(release.testflight.last_error)+'</p>';
 }else{
  details+='<p>'+escapeHTML(release.filename||'')+'</p><code>SHA-256: '+escapeHTML(release.sha256||'')+'</code>';
  if(release.ios)details+='<p>'+escapeHTML(release.bundle_id)+' · 描述文件包含 '+release.ios.device_count+' 台设备。此信息不代表签名已验证或当前设备获准安装。</p>';
 }
 return details+'</details>';
}

export function renderReleaseCard(release,{featured=false,showDetails=false}={}){
 const platformName=release.platform==='ios'?'iOS':release.platform==='macos'?'macOS':release.platform;
 const status=release.delivery==='testflight'?releaseStatusLabel(release):'已发布';
 const meta=[
  'build '+release.build,
  channelNames[release.channel]||release.channel,
  release.variant||'default',
  releaseDeliveryLabel(release),
  formatDate(release.created_at)
 ].filter(Boolean).join(' · ');
 return '<article class="release release-card'+(featured?' featured':'')+'">'+
  '<div class="release-top">'+
   '<div class="release-identity">'+releaseAppIcon(release,{className:featured?'latest-app-icon':'release-app-icon',title:'App Icon'})+
    '<div class="release-heading-copy">'+
     '<div class="release-title-line"><strong>'+escapeHTML(release.version)+'</strong><span class="badge">'+platformIcons(targetsForRelease(release),{className:'compact'})+escapeHTML(platformName)+'</span></div>'+
     '<div class="meta">'+escapeHTML(meta)+'</div>'+
    '</div>'+
   '</div>'+
   '<span class="release-status status-'+escapeHTML(release.status||'published')+'">'+escapeHTML(status)+'</span>'+
  '</div>'+
  '<p class="release-notes">'+escapeHTML(release.notes||'暂无更新说明')+'</p>'+
  testFlightLifecycleView(release)+
  (release.delivery==='testflight'?'<p class="release-status-message">'+escapeHTML(releaseInstallNote(release))+'</p>':'')+
  releaseActionsView(release)+
  (showDetails?releaseDetailsView(release):'')+
 '</article>';
}

import {escapeHTML,channelNames,formatSize,formatDate} from './core.js';
import {platformIcons,targetsForRelease,releaseAppIcon} from './platform-ui.js';

const statusLabels={
 published:'已发布',
 submitted:'已提交到 App Store Connect',
 processing:'Apple Processing',
 available:'Ready for Testing',
 unavailable:'当前不可测试'
};

const uploadLabels={
 AWAITING_UPLOAD:'Awaiting Upload',
 PROCESSING:'Processing',
 COMPLETE:'Complete',
 FAILED:'Failed'
};

const betaLabels={
 READY_FOR_BETA_TESTING:'Ready for Testing',
 IN_BETA_TESTING:'Testing',
 READY_FOR_BETA_SUBMISSION:'Ready to Submit',
 WAITING_FOR_BETA_REVIEW:'Waiting for Review',
 IN_BETA_REVIEW:'In Review',
 BETA_APPROVED:'Approved',
 BETA_REJECTED:'Rejected',
 MISSING_EXPORT_COMPLIANCE:'Missing Export Compliance',
 IN_EXPORT_COMPLIANCE_REVIEW:'Export Compliance Review',
 PROCESSING_EXCEPTION:'Processing Exception',
 EXPIRED:'Expired'
};

function betaAvailable(state){
 return state==='READY_FOR_BETA_TESTING'||state==='IN_BETA_TESTING';
}

function betaLabel(state){
 return betaLabels[state]||state||'—';
}

function uploadLabel(state){
 return uploadLabels[state]||state||'—';
}

function betaTone(state){
 if(!state)return 'waiting';
 if(state==='READY_FOR_BETA_TESTING'||state==='IN_BETA_TESTING')return 'done';
 if(state==='BETA_REJECTED'||state==='PROCESSING_EXCEPTION'||state==='EXPIRED')return 'failed';
 if([
  'READY_FOR_BETA_SUBMISSION',
  'WAITING_FOR_BETA_REVIEW',
  'IN_BETA_REVIEW',
  'BETA_APPROVED',
  'MISSING_EXPORT_COMPLIANCE',
  'IN_EXPORT_COMPLIANCE_REVIEW'
 ].includes(state))return 'attention';
 return 'active';
}

function uploadTone(state){
 if(state==='COMPLETE')return 'done';
 if(state==='FAILED')return 'failed';
 if(state==='PROCESSING'||state==='AWAITING_UPLOAD')return 'active';
 return 'waiting';
}

export function releaseStatusLabel(release){
 if(release?.delivery==='testflight'){
  const info=release.testflight||{};
  if(info.build_upload_state==='FAILED')return 'Build Upload Failed';
  if(info.build_upload_state==='PROCESSING')return 'Processing';
  if(info.build_upload_state==='AWAITING_UPLOAD')return 'Awaiting Upload';
  if(info.external_build_state)return betaLabel(info.external_build_state);
  if(info.internal_build_state)return betaLabel(info.internal_build_state);
  if(info.build_upload_state==='COMPLETE')return 'Complete';
 }
 return statusLabels[release?.status]||release?.status||'已发布';
}

export function releaseStatusTone(release){
 if(release?.delivery!=='testflight')return release?.status||'published';
 const info=release.testflight||{};
 if(info.build_upload_state==='FAILED')return 'unavailable';
 if(info.build_upload_state==='PROCESSING'||info.build_upload_state==='AWAITING_UPLOAD')return 'processing';
 if(info.external_build_state){
  const tone=betaTone(info.external_build_state);
  if(tone==='done')return 'available';
  if(tone==='failed')return 'unavailable';
  if(tone==='attention')return 'attention';
  return 'processing';
 }
 if(info.internal_build_state){
  const tone=betaTone(info.internal_build_state);
  if(tone==='done')return 'available';
  if(tone==='failed')return 'unavailable';
  if(tone==='attention')return 'attention';
 }
 if(info.build_upload_state==='COMPLETE')return 'available';
 return release?.status||'submitted';
}

export function releaseDeliveryLabel(release){
 if(release?.delivery==='testflight')return 'TestFlight';
 if(release?.platform==='ios')return 'Ad Hoc / IPA';
 return '安装包';
}

export function releaseInstallNote(release){
 if(release.delivery==='testflight'){
  const info=release.testflight||{};
  const parts=[];
  if(info.build_upload_state)parts.push('Build Upload: '+uploadLabel(info.build_upload_state));
  if(info.internal_build_state)parts.push('Internal Testing: '+betaLabel(info.internal_build_state));
  if(info.external_build_state)parts.push('External Testing: '+betaLabel(info.external_build_state));
  return parts.length?parts.join(' · '):(release.status_message||releaseStatusLabel(release));
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
 let text=(names[info.profile_type]||info.profile_type)+(info.expires_at?' · '+new Date(info.expires_at).toLocaleDateString('zh-CN')+' 到期':'');
 if(release.ota?.status==='synced')text+=' · OTA Gateway 已同步';
 else if(release.ota?.status==='syncing'||release.ota?.status==='pending')text+=' · OTA Gateway 同步中';
 else if(release.ota?.status==='failed')text+=' · OTA Gateway 同步失败';
 return text;
}

export function testFlightLinkSource(release){
 if(release?.delivery!=='testflight'||!release.open_url)return '';
 if(release.testflight?.public_link&&release.testflight.public_link===release.open_url)return '链接来源：Apple Public Link';
 if(release.testflight?.fallback_url&&release.testflight.fallback_url===release.open_url)return '链接来源：Profile fallback';
 return '链接来源：TestFlight link';
}

export function testFlightLifecycleView(release,{compact=false}={}){
 if(release?.delivery!=='testflight')return '';
 const info=release.testflight||{};
 const uploadState=info.build_upload_state||'';
 const internalState=info.internal_build_state||'';
 const externalState=info.external_build_state||'';
 const uploadText=uploadState?uploadLabel(uploadState):(release.status==='submitted'?'Submitted':'—');
 const items=[
  ['Build Upload',uploadText,uploadState?uploadTone(uploadState):(release.status==='submitted'?'active':'waiting')],
  ['Internal Testing',betaLabel(internalState),betaTone(internalState)],
  ['External Testing',betaLabel(externalState),betaTone(externalState)]
 ];
 const checked=info.last_checked_at
  ?'<span class="release-last-sync">最近同步 '+escapeHTML(formatDate(info.last_checked_at))+'</span>'
  :'';
 return '<div class="testflight-state-wrap'+(compact?' compact':'')+'" aria-label="App Store Connect TestFlight 状态">'+
  '<div class="testflight-state-grid">'+items.map(item=>
   '<span class="testflight-state-item '+item[2]+'"><small>'+escapeHTML(item[0])+'</small><strong>'+escapeHTML(item[1])+'</strong></span>'
  ).join('')+'</div>'+checked+'</div>';
}

export function releaseActionsView(release){
 let html='<div class="download-row">';
 if(release.delivery==='testflight'){
  const info=release.testflight||{};
  if(release.open_url){
   html+='<a class="download" href="'+escapeHTML(release.open_url)+'">在 TestFlight 中打开</a>';
   const source=testFlightLinkSource(release);
   if(source)html+='<span class="release-link-source">'+escapeHTML(source)+'</span>';
  }else if(betaAvailable(info.external_build_state)){
   html+='<span class="meta">External Testing 已可测试；尚未发现 Public Link</span>';
  }else if(betaAvailable(info.internal_build_state)){
   html+='<span class="meta">Internal Testing: '+escapeHTML(betaLabel(info.internal_build_state))+' · External Testing: '+escapeHTML(betaLabel(info.external_build_state))+'</span>';
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
 if(release.platform==='ios'&&!release.install_url&&release.ota){
  if(release.ota.status==='pending'||release.ota.status==='syncing')html+='<span class="meta">正在同步到 OTA Gateway…</span>';
  if(release.ota.status==='failed')html+='<span class="meta">OTA Gateway 同步失败</span>';
 }
 html+='<span class="meta">'+formatSize(release.size)+' · '+escapeHTML(release.architecture)+' · '+escapeHTML(release.variant||'default')+'</span></div>';
 return html;
}

function releaseDetailsView(release){
 let details='<details><summary>发布信息</summary><p>'+escapeHTML(releaseInstallNote(release))+'</p>';
 if(release.delivery==='testflight'){
  const info=release.testflight||{};
  details+='<p>'+escapeHTML(release.bundle_id||'')+' · TestFlight</p>';
  if(info.build_upload_state){
   const counts=[];
   if(info.build_upload_error_count)counts.push(info.build_upload_error_count+' errors');
   if(info.build_upload_warning_count)counts.push(info.build_upload_warning_count+' warnings');
   if(info.build_upload_info_count)counts.push(info.build_upload_info_count+' infos');
   details+='<p>Build Upload: '+escapeHTML(uploadLabel(info.build_upload_state))+' <code>'+escapeHTML(info.build_upload_state)+'</code>'+(counts.length?' · '+escapeHTML(counts.join(' · ')):'')+'</p>';
  }
  if(info.processing_state)details+='<p>Build Processing: '+escapeHTML(info.processing_state)+'</p>';
  if(info.internal_build_state)details+='<p>Internal Testing: '+escapeHTML(betaLabel(info.internal_build_state))+' <code>'+escapeHTML(info.internal_build_state)+'</code></p>';
  if(info.external_build_state)details+='<p>External Testing: '+escapeHTML(betaLabel(info.external_build_state))+' <code>'+escapeHTML(info.external_build_state)+'</code></p>';
  if(info.public_link)details+='<p>Public Link: '+escapeHTML(info.public_link)+'</p>';
  if(info.fallback_url)details+='<p>Profile fallback: '+escapeHTML(info.fallback_url)+'</p>';
  if(info.target_group_name)details+='<p>TestFlight Group: '+escapeHTML(info.target_group_name)+' · '+escapeHTML(info.target_group_type||'internal')+(info.beta_group_assigned?' · 已关联':'')+'</p>';
  if(info.beta_review_state)details+='<p>Beta Review: '+escapeHTML(info.beta_review_state)+'</p>';
  if(info.automation_error)details+='<p class="form-error">自动分发：'+escapeHTML(info.automation_error)+'</p>';
  if(info.last_error)details+='<p class="form-error">'+escapeHTML(info.last_error)+'</p>';
 }else{
  details+='<p>'+escapeHTML(release.filename||'')+'</p><code>SHA-256: '+escapeHTML(release.sha256||'')+'</code>';
  if(release.ios)details+='<p>'+escapeHTML(release.bundle_id)+' · 描述文件包含 '+release.ios.device_count+' 台设备。此信息不代表签名已验证或当前设备获准安装。</p>';
  if(release.ota){
   details+='<p>OTA Gateway: '+escapeHTML(release.ota.status||'—')+(release.ota.synced_at?' · '+escapeHTML(formatDate(release.ota.synced_at)):'')+'</p>';
   if(release.ota.public_url)details+='<p>Public OTA: '+escapeHTML(release.ota.public_url)+'</p>';
   if(release.ota.last_error)details+='<p class="form-error">'+escapeHTML(release.ota.last_error)+'</p>';
  }
 }
 return details+'</details>';
}

export function renderReleaseCard(release,{featured=false,showDetails=false,allowDelete=false}={}){
 const platformName=release.platform==='ios'?'iOS':release.platform==='macos'?'macOS':release.platform;
 const status=release.delivery==='testflight'?releaseStatusLabel(release):'已发布';
 const tone=release.delivery==='testflight'?releaseStatusTone(release):(release.status||'published');
 const meta=[
  'build '+release.build,
  channelNames[release.channel]||release.channel,
  release.variant||'default',
  releaseDeliveryLabel(release),
  formatDate(release.created_at)
 ].filter(Boolean).join(' · ');
 const deleteAction=allowDelete&&release.delivery!=='testflight'
  ?'<div class="download-row release-delete-row"><button type="button" class="danger" data-delete-release="'+escapeHTML(release.id)+'">删除安装包</button><span class="meta">同时删除 ILS 中的发布记录；已同步 OTA 时也会先删除公网副本。</span></div>'
  :'';
 return '<article class="release release-card'+(featured?' featured':'')+'">'+
  '<div class="release-top">'+
   '<div class="release-identity">'+releaseAppIcon(release,{className:featured?'latest-app-icon':'release-app-icon',title:'App Icon'})+
    '<div class="release-heading-copy">'+
     '<div class="release-title-line"><strong>'+escapeHTML(release.version)+'</strong><span class="badge">'+platformIcons(targetsForRelease(release),{className:'compact'})+escapeHTML(platformName)+'</span></div>'+
     '<div class="meta">'+escapeHTML(meta)+'</div>'+
    '</div>'+
   '</div>'+
   '<span class="release-status status-'+escapeHTML(tone)+'">'+escapeHTML(status)+'</span>'+
  '</div>'+
  '<p class="release-notes">'+escapeHTML(release.notes||'暂无更新说明')+'</p>'+
  testFlightLifecycleView(release,{compact:featured})+
  (release.delivery==='testflight'?'<p class="release-status-message">'+escapeHTML(release.status_message||releaseInstallNote(release))+'</p>':'')+
  releaseActionsView(release)+
  (showDetails?releaseDetailsView(release):'')+
  deleteAction+
 '</article>';
}

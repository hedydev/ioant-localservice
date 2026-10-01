
import {$,state,api,notice,needAdmin,escapeHTML,formatSize,channelNames} from './core.js';
import {refreshData} from './projects.js';

function iosInstallNote(release){
 const info=release.ios;
 if(!info)return '签名状态未知';
 if(info.profile_type==='ad-hoc')return 'Ad Hoc · 仅限 provisioning profile 中已登记的设备';
 if(info.profile_type==='development')return '开发签名 · 通常需要 Xcode / Configurator 安装';
 if(info.profile_type==='enterprise')return '企业分发 · 仅限组织内部';
 if(info.profile_type==='app-store')return 'App Store / TestFlight 包不通过此页直接安装';
 return info.profile_type||'签名状态未知';
}

function renderPublicIOS(){
 const releases=state.releases.filter(item=>item.platform==='ios').slice(0,5);
 const container=$('#ios-install-releases');

 if(!state.project){
  container.innerHTML='<div class="empty"><strong>请先选择项目</strong>选择项目后，这里会显示它的 iOS 安装包。</div>';
  return;
 }

 if(!releases.length){
  container.innerHTML='<div class="empty"><strong>还没有 iOS 安装包</strong>管理员发布 Ad Hoc IPA 后，会出现在这里。</div>';
  return;
 }

 container.innerHTML=releases.map(release=>{
  const install=release.install_url
   ?'<a class="download" href="'+escapeHTML(release.install_url)+'">安装到 iPhone / iPad</a>'
   :'';
  const download='<a class="download '+(install?'secondary':'')+'" href="'+escapeHTML(release.download_url)+'">下载 IPA</a>';
  return '<article class="release ios-install-release">'+
   '<div class="release-top"><div><strong>'+escapeHTML(release.version)+'</strong> <span class="badge">'+escapeHTML(channelNames[release.channel]||release.channel)+'</span><div class="meta">build '+release.build+'</div></div><span class="meta">'+formatSize(release.size)+'</span></div>'+
   '<p>'+escapeHTML(release.notes||'暂无更新说明')+'</p>'+
   '<p class="meta">'+escapeHTML(iosInstallNote(release))+'</p>'+
   '<div class="download-row">'+install+download+'</div>'+
  '</article>';
 }).join('');
}

async function checkEnrollmentAvailability(){
 try{
  const health=await api('/api/health');
  const link=$('#enroll-device-link');
  if(health.ota_configured){
   link.href='/api/devices/enroll.mobileconfig';
   link.removeAttribute('aria-disabled');
   link.classList.remove('disabled-link');
   $('#enrollment-availability').textContent='设备登记入口已启用。请使用 iPhone / iPad 的 Safari 打开此页面并安装登记描述文件。';
  }else{
   link.removeAttribute('href');
   link.setAttribute('aria-disabled','true');
   link.classList.add('disabled-link');
   $('#enrollment-availability').textContent='当前 ILS 尚未配置受 iPhone 信任的 HTTPS public-url，因此设备登记和网页直接安装暂不可用；请联系管理员先配置 HTTPS。';
  }
 }catch(error){
  $('#enrollment-availability').textContent='无法读取设备登记状态：'+error.message;
 }
}

async function pollSigningJob(){
 if(!state.admin||!state.job)return;
 try{
  const job=await api('/api/signing/'+state.job);
  $('#sign-state').textContent={
   running:'Mac 正在签名…',
   succeeded:'签名完成，安装包已发布。',
   failed:job.error||'签名失败'
  }[job.status]||job.status;
  if(job.status!=='running'){
   state.job=null;
   $('#sign-button').disabled=false;
   await refreshData();
  }
 }catch(error){
  $('#sign-state').textContent=error.message;
  state.job=null;
  $('#sign-button').disabled=false;
 }
}

function resetIOS(){
 state.job=null;
 $('#devices-list').textContent='';
 $('#devices-list').hidden=true;
 $('#sign-state').textContent='';
 $('#sign-button').disabled=false;
 renderPublicIOS();
}

export function initIOS(){
 $('#devices-button').onclick=async()=>{
  if(!needAdmin())return;
  try{
   const devices=await api('/api/devices');
   $('#devices-list').hidden=false;
   $('#devices-list').textContent=devices.length
    ?devices.map(device=>device.product+' / iOS '+device.version+'\nUDID: '+device.udid+'\n待 Apple 团队登记 · 设备身份尚未认证').join('\n\n')
    :'还没有收集到设备。';
  }catch(error){
   notice(error.message);
  }
 };

 $('#sign-button').onclick=async()=>{
  if(!needAdmin())return;
  if(!state.project)return notice('请先创建项目');
  const button=$('#sign-button');
  button.disabled=true;
  try{
   const job=await api('/api/projects/'+state.project+'/signing',{method:'POST'});
   state.job=job.id;
   $('#sign-state').textContent='Mac 正在导出并签名，完成后自动发布。';
  }catch(error){
   $('#sign-state').textContent=error.message;
   button.disabled=false;
  }
 };

 window.addEventListener('data-refreshed',renderPublicIOS);
 window.addEventListener('project-changed',resetIOS);
 window.addEventListener('admin-cleared',resetIOS);
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='ios'){
   renderPublicIOS();
   checkEnrollmentAvailability();
  }
 });

 setInterval(()=>{
  if(document.hidden)return;
  pollSigningJob();
 },3000);

 if(new URLSearchParams(location.search).get('enrollment')==='collected'){
  notice('设备信息已提交。管理员还需要在 Apple Developer Team 中注册这台设备并重新发布 Ad Hoc 包，之后才能安装。');
 }

 renderPublicIOS();
 checkEnrollmentAvailability();
}

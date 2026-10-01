const definitions={
 mac:{
  label:'Mac',
  svg:'<svg viewBox="0 0 24 24" focusable="false"><rect x="4" y="5" width="16" height="11" rx="1.8"></rect><path d="M2.8 19h18.4"></path><path d="M9.5 16.2 8.8 19m5.7-2.8.7 2.8"></path></svg>'
 },
 iphone:{
  label:'iPhone',
  svg:'<svg viewBox="0 0 24 24" focusable="false"><rect x="7.2" y="2.5" width="9.6" height="19" rx="2.2"></rect><path d="M10.2 5h3.6"></path><path d="M10.5 18.8h3"></path></svg>'
 },
 ipad:{
  label:'iPad',
  svg:'<svg viewBox="0 0 24 24" focusable="false"><rect x="4.4" y="2.5" width="15.2" height="19" rx="2"></rect><circle cx="12" cy="18.8" r=".65"></circle></svg>'
 },
 android:{
  label:'Android',
  svg:'<svg viewBox="0 0 24 24" focusable="false"><path d="M7 9.2h10v7.3a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2Z"></path><path d="m8.3 7.2-1.4-2m8.8 2 1.4-2"></path><path d="M8.2 9.2a3.8 3.8 0 0 1 7.6 0"></path><circle cx="10" cy="7.7" r=".45"></circle><circle cx="14" cy="7.7" r=".45"></circle></svg>'
 }
};

function uniqueTargets(targets){
 const out=[];
 for(const target of targets||[]){
  if(definitions[target]&&!out.includes(target))out.push(target);
 }
 return out;
}

export function targetsForPlatform(platform,explicit){
 const selected=uniqueTargets(explicit);
 if(selected.length)return selected;
 if(platform==='macos')return ['mac'];
 if(platform==='ios')return ['iphone','ipad'];
 if(platform==='android')return ['android'];
 return [];
}

export function targetsForRelease(release){
 return targetsForPlatform(release?.platform,release?.ios?.device_targets||release?.device_targets);
}

export function targetsForProfile(profile){
 return targetsForPlatform(profile?.platform,profile?.device_targets);
}

export function targetsForJob(job){
 const explicit=job?.device_targets||job?.result?.device_targets;
 if(explicit?.length)return uniqueTargets(explicit);
 if(job?.result?.platform)return targetsForPlatform(job.result.platform);
 if(job?.platform)return targetsForPlatform(job.platform);
 const key=(job?.profile_id||'')+' '+(job?.title||'');
 if(/ios/i.test(key))return ['iphone','ipad'];
 if(/macos|mac\s/i.test(key))return ['mac'];
 if(/android/i.test(key))return ['android'];
 return [];
}

export function platformIcons(targets,{className=''}={}){
 const values=uniqueTargets(targets);
 if(!values.length)return '';
 const label=values.map(value=>definitions[value].label).join(', ');
 return '<span class="platform-icons '+className+'" aria-label="'+label+'" title="'+label+'">'+
  values.map(value=>'<span class="platform-icon platform-icon-'+value+'" aria-hidden="true">'+definitions[value].svg+'</span>').join('')+
 '</span>';
}

function escapeAttribute(value){
 return String(value??'').replace(/[&<>"']/g,char=>({
  '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
 })[char]);
}

export function appIcon(src,{className='',title=''}={}){
 if(!src)return '';
 return '<img data-app-icon class="app-icon '+escapeAttribute(className)+'" src="'+escapeAttribute(src)+'" alt="" loading="lazy"'+
  (title?' title="'+escapeAttribute(title)+'"':'')+'>';
}

function appIdentityIcon(src,targets,{className='',title=''}={}){
 const values=uniqueTargets(targets);
 const fallback=platformIcons(values,{className:'app-icon-fallback-platforms'});
 const badge=platformIcons(values,{className:'compact app-icon-platform-icons'});
 return '<span class="app-icon-shell '+escapeAttribute(className)+'"'+(title?' title="'+escapeAttribute(title)+'"':'')+'>'+
  '<span class="app-icon-fallback" aria-hidden="true">'+fallback+'</span>'+
  (src?'<img data-app-icon class="app-icon" src="'+escapeAttribute(src)+'" alt="" loading="lazy">':'')+
  (badge?'<span class="app-platform-badge">'+badge+'</span>':'')+
 '</span>';
}

export function projectAppIcon(project,platform,options={}){
 if(!project||!['ios','macos'].includes(platform))return '';
 return appIdentityIcon('/api/projects/'+encodeURIComponent(project)+'/icon?platform='+encodeURIComponent(platform),targetsForPlatform(platform),options);
}

export function buildAppIcon(job,options={}){
 if(!job?.id)return '';
 return appIdentityIcon('/api/builds/'+encodeURIComponent(job.id)+'/icon',targetsForJob(job),options);
}

export function releaseAppIcon(release,options={}){
 if(!release)return '';
 return appIdentityIcon(release.app_icon_url||'',targetsForRelease(release),options);
}

export function decoratePlatformIcons(root=document){
 root.querySelectorAll('[data-platform-targets]').forEach(slot=>{
  const targets=(slot.dataset.platformTargets||'').trim().split(/\s+/).filter(Boolean);
  slot.innerHTML=platformIcons(targets);
 });
}

export function initPlatformUI(){
 decoratePlatformIcons();
 document.addEventListener('error',event=>{
  const image=event.target;
  if(image?.matches?.('img[data-app-icon]'))image.remove();
 },true);
 window.addEventListener('admin-loaded',()=>decoratePlatformIcons());
 window.addEventListener('data-refreshed',()=>decoratePlatformIcons());
}

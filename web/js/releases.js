import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';
import {refreshData} from './projects.js';
import {renderReleaseCard} from './release-ui.js';

function filteredReleases(){
 return state.releases.filter(release=>
  (state.platform==='all'||release.platform===state.platform)&&
  (state.channel==='all'||release.channel===state.channel)&&
  (state.variant==='all'||(release.variant||'default')===state.variant)
 );
}

function releaseTime(release){
 const value=Date.parse(release.created_at||'');
 return Number.isFinite(value)?value:0;
}

function currentDistributionKey(release){
 const variant=release.variant||'default';
 if(release.delivery==='testflight')return 'ios:testflight';
 if(release.platform==='ios')return 'ios:adhoc:'+variant;
 if(release.platform==='macos')return 'macos:'+variant+':'+(release.architecture||'');
 return (release.platform||'other')+':'+(release.delivery||'artifact')+':'+variant;
}

function currentDistributions(){
 const latest=new Map();
 [...state.releases]
  .sort((a,b)=>releaseTime(b)-releaseTime(a))
  .forEach(release=>{
   const key=currentDistributionKey(release);
   if(!latest.has(key))latest.set(key,release);
  });
 return [...latest.values()].sort((a,b)=>releaseTime(b)-releaseTime(a));
}

function renderOverview(){
 const current=currentDistributions();
 const versions=new Set(state.releases.map(item=>String(item.version||'').trim()).filter(Boolean));
 $('#overview-stats').innerHTML=[
  ['发布记录',state.releases.length],
  ['当前分发',current.length],
  ['版本',versions.size]
 ].map(item=>'<div class="overview-stat"><strong>'+item[1]+'</strong><span>'+escapeHTML(item[0])+'</span></div>').join('');

 if(!current.length){
  $('#overview-latest').innerHTML='<div class="empty"><strong>暂无发布</strong>发布完成后，当前可交付的 Release 会显示在这里；历史记录统一放在“版本历史”。</div>';
  return;
 }
 $('#overview-latest').innerHTML=''+
  '<div class="section-heading overview-current-heading"><div><h2>当前分发</h2><span class="meta">每种分发方式只显示最新一条；完整旧版本请到“版本历史”查看。</span></div></div>'+
  '<div class="overview-latest-grid">'+current.map(release=>renderReleaseCard(release,{featured:true})).join('')+'</div>';
}

function renderReleaseHistory(){
 const variants=[...new Set(state.releases.map(item=>item.variant||'default'))];
 if(state.variant!=='all'&&!variants.includes(state.variant))state.variant='all';
 $('#variant').innerHTML='<option value="all">全部发布</option>'+variants.map(value=>
  '<option value="'+escapeHTML(value)+'" '+(value===state.variant?'selected':'')+'>'+escapeHTML(value)+'</option>'
 ).join('');

 const rows=filteredReleases();
 $('#release-count').textContent=rows.length;
 if(!rows.length){
  $('#releases').innerHTML='<div class="empty"><strong>还没有匹配的发布</strong>调整平台、渠道或发布筛选。</div>';
  return;
 }
 $('#releases').innerHTML=rows.map(release=>renderReleaseCard(release,{showDetails:true})).join('');
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
  notice('发布完成。');
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
    :'此平台、架构和渠道还没有发布。';
  }catch(error){
   $('#check-result').textContent=error.message;
  }
 };

 renderReleases();
}

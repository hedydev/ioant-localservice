import {api,state} from './core.js';

let logRequestInFlight=false;

function activeLogElement(){
 return document.querySelector('[data-build-log-output]');
}

function updateHeading(element,data){
 const panel=element.closest('.build-log-panel');
 const heading=panel?.querySelector('.build-log-heading');
 if(!heading)return;
 let meta=heading.querySelector('[data-live-log-meta]');
 if(!meta){
  meta=document.createElement('span');
  meta.className='meta';
  meta.dataset.liveLogMeta='1';
  heading.appendChild(meta);
 }
 const lines=Number.isFinite(data.tail_lines)?data.tail_lines:300;
 meta.textContent='仅显示最新 '+lines+' 行'+(data.truncated?' · 更早内容已隐藏':'')+' · 不自动滚动';

 let download=heading.querySelector('[data-download-build-log]');
 if(!download){
  download=document.createElement('button');
  download.type='button';
  download.dataset.downloadBuildLog=element.dataset.buildLogOutput;
  download.textContent='下载完整日志';
  heading.appendChild(download);
 }
}

function updateTail(element,text){
 const previous=element.textContent||'';
 if(previous===text)return;
 const scrollTop=element.scrollTop;
 element.textContent=text;
 const maxScroll=Math.max(0,element.scrollHeight-element.clientHeight);
 element.scrollTop=Math.min(scrollTop,maxScroll);
}

async function refreshActiveLog(){
 if(logRequestInFlight||document.hidden||!state.admin||state.view!=='builds')return;
 const element=activeLogElement();
 if(!element)return;
 const jobID=element.dataset.buildLogOutput;
 if(!jobID)return;
 logRequestInFlight=true;
 try{
  const data=await api('/api/builds/'+encodeURIComponent(jobID)+'/log');
  const current=activeLogElement();
  if(!current||current.dataset.buildLogOutput!==jobID)return;
  updateTail(current,data.log||'');
  updateHeading(current,data);
 }catch(error){
  // A just-created build can briefly have no log file. The normal build poll
  // will retry, so keep this lightweight poll silent.
  if(!String(error?.message||'').includes('日志尚未生成')){
   console.warn('ILS live log polling failed:',error);
  }
 }finally{
  logRequestInFlight=false;
 }
}

async function downloadFullLog(jobID,button){
 const oldText=button.textContent;
 button.disabled=true;
 button.textContent='准备下载…';
 try{
  const data=await api('/api/builds/'+encodeURIComponent(jobID)+'/log?full=1');
  const blob=new Blob([data.log||''],{type:'text/plain;charset=utf-8'});
  const url=URL.createObjectURL(blob);
  const link=document.createElement('a');
  link.href=url;
  link.download='ils-build-'+jobID+'.log';
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(()=>URL.revokeObjectURL(url),1000);
 }catch(error){
  console.warn('ILS full log download failed:',error);
  button.textContent='下载失败';
  setTimeout(()=>{ button.textContent=oldText; },1500);
  return;
 }finally{
  button.disabled=false;
 }
 button.textContent=oldText;
}

export function initBuildLogLive(){
 document.addEventListener('click',event=>{
  const button=event.target.closest('[data-download-build-log]');
  if(!button)return;
  const jobID=button.dataset.downloadBuildLog;
  if(jobID)void downloadFullLog(jobID,button);
 });
 window.addEventListener('build-started',()=>{ setTimeout(()=>void refreshActiveLog(),150); });
 window.addEventListener('view-changed',()=>{ void refreshActiveLog(); });
 window.addEventListener('project-changed',()=>{ void refreshActiveLog(); });
 setInterval(()=>{ void refreshActiveLog(); },1000);
}

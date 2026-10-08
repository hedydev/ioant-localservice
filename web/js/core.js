
export const $=selector=>document.querySelector(selector);
export const state={
 projects:[],releases:[],internalTests:[],project:null,
 platform:'all',channel:'all',variant:'all',
 token:'',admin:false,authEpoch:0,job:null,view:'overview'
};
export const channelNames={dev:'开发版',beta:'测试版',stable:'正式版'};
export const escapeHTML=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export const formatSize=bytes=>bytes>=1073741824?(bytes/1073741824).toFixed(2)+' GB':(bytes/1048576).toFixed(1)+' MB';
export const formatDate=value=>new Date(value).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});

const adminTokenStorageKey='ils.admin-token';

function readCachedAdminToken(){
 try{return (window.localStorage.getItem(adminTokenStorageKey)||'').trim();}
 catch{return '';}
}

function writeCachedAdminToken(token){
 try{
  if(token)window.localStorage.setItem(adminTokenStorageKey,token);
  else window.localStorage.removeItem(adminTokenStorageKey);
 }catch{
  // Browser storage may be unavailable; the in-memory admin session still works.
 }
}

async function fetchWithTimeout(path,options={},timeoutMs=10000){
 const controller=new AbortController();
 const timer=setTimeout(()=>controller.abort(),timeoutMs);
 try{
  return await fetch(path,{...options,signal:options.signal||controller.signal});
 }catch(error){
  if(error?.name==='AbortError')throw new Error('请求超时');
  throw error;
 }finally{
  clearTimeout(timer);
 }
}

export async function api(path,options={}){
 const epoch=state.authEpoch;
 const authenticatedAtStart=state.admin;
 const timeoutMs=options.timeoutMs??10000;
 const {timeoutMs:_timeoutMs,...fetchOptions}=options;
 const response=await fetchWithTimeout(path,{
  ...fetchOptions,
  headers:{...(authenticatedAtStart?{Authorization:'Bearer '+state.token}:{}),...fetchOptions.headers}
 },timeoutMs);
 let data={};
 const raw=await response.text();
 if(raw){
  try{data=JSON.parse(raw);}
  catch{data={};}
 }
 // Public requests remain valid when a remembered admin session finishes
 // restoring in parallel. Only requests that started authenticated are tied to
 // the authentication epoch.
 if(authenticatedAtStart&&epoch!==state.authEpoch)throw new Error('管理会话已更改，请重新操作');
 if((response.status===401||response.status===403)&&authenticatedAtStart&&state.admin)setAdmin(false);
 if(!response.ok)throw new Error(data.error||('请求失败（HTTP '+response.status+'）'));
 return data;
}

let noticeTimer=null;

export function notice(message,kind='info',timeout=4500){
 const element=$('#notice');
 if(noticeTimer){
  clearTimeout(noticeTimer);
  noticeTimer=null;
 }
 if(!message){
  element.textContent='';
  element.hidden=true;
  element.dataset.kind='';
  return;
 }
 element.textContent=message;
 element.dataset.kind=kind;
 element.hidden=false;
 element.setAttribute('role',kind==='error'?'alert':'status');
 if(timeout>0){
  noticeTimer=setTimeout(()=>{
   if(element.textContent===message){
    element.hidden=true;
    element.textContent='';
    element.dataset.kind='';
   }
   noticeTimer=null;
  },timeout);
 }
}

let actionConfirmDialog=null;
let actionConfirmResolver=null;

function ensureActionConfirmDialog(){
 if(actionConfirmDialog)return actionConfirmDialog;
 const dialog=document.createElement('dialog');
 dialog.className='action-confirm-dialog';
 dialog.setAttribute('aria-labelledby','action-confirm-title');
 dialog.setAttribute('aria-describedby','action-confirm-message');
 dialog.innerHTML='<form method="dialog" class="action-confirm-card">'+
  '<div class="action-confirm-icon" aria-hidden="true">!</div>'+
  '<div class="action-confirm-copy">'+
   '<h2 id="action-confirm-title"></h2>'+
   '<p id="action-confirm-message"></p>'+
   '<p class="action-confirm-detail" data-confirm-detail hidden></p>'+
  '</div>'+
  '<div class="action-confirm-actions">'+
   '<button type="submit" value="cancel" class="quiet" data-confirm-cancel>取消</button>'+
   '<button type="submit" value="confirm" class="danger action-confirm-primary" data-confirm-primary>确认</button>'+
  '</div>'+
 '</form>';
 document.body.appendChild(dialog);
 dialog.addEventListener('close',()=>{
  const resolve=actionConfirmResolver;
  actionConfirmResolver=null;
  if(resolve)resolve(dialog.returnValue==='confirm');
 });
 dialog.addEventListener('click',event=>{
  if(event.target===dialog)dialog.close('cancel');
 });
 actionConfirmDialog=dialog;
 return dialog;
}

export function confirmAction({
 title='确认操作',
 message='',
 detail='',
 confirmLabel='确认',
 cancelLabel='取消'
}={}){
 const dialog=ensureActionConfirmDialog();
 if(dialog.open)return Promise.resolve(false);
 dialog.querySelector('#action-confirm-title').textContent=title;
 dialog.querySelector('#action-confirm-message').textContent=message;
 const detailElement=dialog.querySelector('[data-confirm-detail]');
 detailElement.textContent=detail;
 detailElement.hidden=!detail;
 dialog.querySelector('[data-confirm-primary]').textContent=confirmLabel;
 dialog.querySelector('[data-confirm-cancel]').textContent=cancelLabel;
 dialog.returnValue='cancel';
 return new Promise(resolve=>{
  actionConfirmResolver=resolve;
  dialog.showModal();
  requestAnimationFrame(()=>dialog.querySelector('[data-confirm-primary]')?.focus());
 });
}

export function needAdmin(){
 if(state.admin)return true;
 $('#admin-dialog').showModal();
 return false;
}

export function setAdmin(authenticated,token=''){
 state.authEpoch++;
 state.admin=authenticated;
 state.token=authenticated?token:'';
 state.job=null;
 writeCachedAdminToken(authenticated?token:'');

 document.querySelectorAll('[data-admin]').forEach(el=>{
  if(!authenticated&&el.tagName==='DIALOG'&&el.open)el.close();
  el.hidden=!authenticated;
 });

 $('#admin-button').hidden=authenticated;
 $('#admin-button').textContent=authenticated?'管理员已验证':'管理员验证';

 if(!authenticated){
  ['#project-form','#publish-form','#profile-form'].forEach(selector=>$(selector)?.reset());
  $('#devices-list').textContent='';
  $('#devices-list').hidden=true;
  $('#sign-state').textContent='';
  $('#sign-button').disabled=false;
 }

 window.dispatchEvent(new CustomEvent(authenticated?'admin-loaded':'admin-cleared'));
 window.dispatchEvent(new CustomEvent('admin-changed',{detail:{authenticated}}));
}

export async function restoreAdminSession(){
 const token=readCachedAdminToken();
 if(!token)return;
 const button=$('#admin-button');
 button.disabled=true;
 button.textContent='恢复管理状态…';
 try{
  const response=await fetchWithTimeout('/api/admin/session',{headers:{Authorization:'Bearer '+token}},3000);
  let result={};
  try{result=await response.json();}catch{}
  if(response.status===401||response.status===403){
   setAdmin(false);
   return;
  }
  if(!response.ok)throw new Error(result.error||'ILS 暂时不可用');
  if(result.authenticated!==true){
   setAdmin(false);
   return;
  }
  setAdmin(true,token);
 }catch(error){
  // A stopped/restarting/slow ILS must not erase the browser's remembered token.
  // Public project bootstrap continues independently and a later refresh/reopen
  // can restore this cached session again.
  console.warn('ILS admin session restore deferred:',error);
 }finally{
  button.disabled=false;
  if(!state.admin)button.textContent='管理员验证';
 }
}

export function initCore(){
 const adminHelp=$('#admin-dialog p');
 if(adminHelp)adminHelp.textContent='输入这台 Mac 的发布密钥。验证成功后会保存在此浏览器，刷新或重新打开无需再次输入；只有退出管理或服务明确拒绝该密钥时才清除。';
 $('#admin-button').onclick=()=>$('#admin-dialog').showModal();

 let adminAttempt=0;
 $('#admin-form').onsubmit=async event=>{
  event.preventDefault();
  const attempt=++adminAttempt;
  const token=$('#token').value.trim();
  const button=event.target.querySelector('[type=submit]');
  button.disabled=true;
  $('#admin-error').textContent='';
  try{
   const response=await fetchWithTimeout('/api/admin/session',{headers:{Authorization:'Bearer '+token}},10000);
   let result={};
   try{result=await response.json();}catch{}
   if(!response.ok||result.authenticated!==true)throw new Error(result.error||'管理员验证失败');
   if(!$('#admin-dialog').open||attempt!==adminAttempt)return;
   $('#token').value='';
   $('#admin-dialog').close();
   setAdmin(true,token);
   notice('管理员验证成功，已记住在此浏览器。');
  }catch(error){
   $('#token').value='';
   $('#admin-error').textContent=error.message;
  }finally{
   button.disabled=false;
  }
 };

 $('#logout-button').onclick=()=>{
  setAdmin(false);
  notice('已退出管理，并清除本地登录状态。');
 };
 $('#admin-dialog').addEventListener('close',()=>{
  adminAttempt++;
  $('#token').value='';
 });
 document.querySelectorAll('[data-close]').forEach(button=>{
  button.onclick=()=>button.closest('dialog').close();
 });
}


export const $=selector=>document.querySelector(selector);
export const state={
 projects:[],releases:[],project:null,
 platform:'all',channel:'all',variant:'all',
 token:'',admin:false,authEpoch:0,job:null,view:'overview'
};
export const channelNames={dev:'开发版',beta:'测试版',stable:'正式版'};
export const escapeHTML=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export const formatSize=bytes=>bytes>=1073741824?(bytes/1073741824).toFixed(2)+' GB':(bytes/1048576).toFixed(1)+' MB';
export const formatDate=value=>new Date(value).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});

export async function api(path,options={}){
 const epoch=state.authEpoch;
 const response=await fetch(path,{...options,headers:{...(state.admin?{Authorization:'Bearer '+state.token}:{}),...options.headers}});
 const data=await response.json();
 if(epoch!==state.authEpoch)throw new Error('管理会话已更改，请重新操作');
 if(response.status===401&&state.admin)setAdmin(false);
 if(!response.ok)throw new Error(data.error||'请求失败');
 return data;
}

export function notice(message){
 $('#notice').textContent=message||'';
 $('#notice').hidden=!message;
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

export function initCore(){
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
   const response=await fetch('/api/admin/session',{headers:{Authorization:'Bearer '+token}});
   const result=await response.json();
   if(!response.ok||result.authenticated!==true)throw new Error(result.error||'管理员验证失败');
   if(!$('#admin-dialog').open||attempt!==adminAttempt)return;
   $('#token').value='';
   $('#admin-dialog').close();
   setAdmin(true,token);
   notice('管理员验证成功。');
  }catch(error){
   $('#token').value='';
   $('#admin-error').textContent=error.message;
  }finally{
   button.disabled=false;
  }
 };

 $('#logout-button').onclick=()=>{
  setAdmin(false);
  notice('已退出管理，现在仅显示已发布安装包。');
 };
 $('#admin-dialog').addEventListener('close',()=>{
  adminAttempt++;
  $('#token').value='';
 });
 document.querySelectorAll('[data-close]').forEach(button=>{
  button.onclick=()=>button.closest('dialog').close();
 });
}

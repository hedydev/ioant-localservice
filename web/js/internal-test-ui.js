import {buildState} from './build-state.js';

function syncInternalTestUI(){
 const root=document.querySelector('#release-profiles');
 if(!root)return;
 root.querySelectorAll('[data-run-profile]').forEach(button=>{
  const profile=buildState.profiles.find(item=>item.id===button.dataset.runProfile);
  if(!profile||profile.platform!=='macos'||profile.lane!=='macos-test')return;
  button.textContent='构建测试包';
  const article=button.closest('article.release');
  if(!article||article.querySelector('[data-internal-test-note]'))return;
  const note=document.createElement('div');
  note.className='job-message';
  note.dataset.internalTestNote='1';
  note.textContent='Internal Test：只生成安装包并挂到本次 Build Job；不创建 Release，不公证，不做深度安装包验证。';
  const details=article.querySelector('details');
  article.insertBefore(note,details||null);
 });
}

export function initInternalTestUI(){
 const root=document.querySelector('#release-profiles');
 if(root){
  new MutationObserver(syncInternalTestUI).observe(root,{childList:true,subtree:true});
 }
 window.addEventListener('build-source-loaded',syncInternalTestUI);
 window.addEventListener('admin-loaded',syncInternalTestUI);
 window.addEventListener('view-changed',syncInternalTestUI);
 queueMicrotask(syncInternalTestUI);
}


import {state,api,notice,needAdmin} from './core.js';
import {buildState} from './build-state.js';

function internalTestProfile(payload){
 if(!payload?.profile)return null;
 const profile=buildState.profiles.find(item=>item.id===payload.profile);
 return profile?.platform==='macos'&&profile?.lane==='macos-test'?profile:null;
}

export async function startBuild(payload,button){
 if(!needAdmin()||!state.project)return;
 const project=state.project;
 const internalTest=internalTestProfile(payload);
 const endpoint=internalTest
  ?'/api/projects/'+project+'/internal-builds'
  :'/api/projects/'+project+'/builds';
 const oldText=button.textContent;
 button.disabled=true;
 if(internalTest)button.textContent='正在打包…';
 try{
  const job=await api(endpoint,{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(payload)
  });
  if(project!==state.project)return;
  window.dispatchEvent(new CustomEvent('build-started',{detail:{job}}));
  notice(internalTest
   ?'Internal Test 打包任务已启动；完成后可直接下载安装包，不会创建 Release。'
   :'ILS 发布任务已启动；构建、上传和 TestFlight 提交状态会持续更新。');
 }catch(error){
  notice(error.message);
 }finally{
  button.disabled=false;
  button.textContent=oldText;
 }
}

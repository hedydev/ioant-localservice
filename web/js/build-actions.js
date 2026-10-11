
import {state,api,notice,needAdmin} from './core.js';
import {buildState} from './build-state.js';

function packageOnlyProfile(payload){
 if(!payload?.profile)return null;
 const profile=buildState.profiles.find(item=>item.id===payload.profile);
 if(!profile)return null;
 if(profile.platform==='macos'&&profile.lane==='macos-test')return profile;
 if(profile.platform==='ios'&&profile.lane==='ios-simulator')return profile;
 return null;
}

export async function startBuild(payload,button){
 if(!needAdmin()||!state.project)return;
 const project=state.project;
 const packageOnly=packageOnlyProfile(payload);
 const endpoint=packageOnly
  ?'/api/projects/'+project+'/internal-builds'
  :'/api/projects/'+project+'/builds';
 const oldText=button.textContent;
 button.disabled=true;
 if(packageOnly)button.textContent='正在构建测试包…';
 try{
  const job=await api(endpoint,{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(payload)
  });
  if(project!==state.project)return;
  window.dispatchEvent(new CustomEvent('build-started',{detail:{job}}));
  if(packageOnly?.lane==='ios-simulator'){
   notice('iOS Simulator 测试包构建已启动；完成后可直接安装并启动到当前 Simulator，不会创建 Release。');
  }else if(packageOnly){
   notice('Internal Test 打包任务已启动；完成后可直接下载安装包，不会创建 Release。');
  }else{
   notice('ILS 发布任务已启动；构建、上传和 TestFlight 提交状态会持续更新。');
  }
 }catch(error){
  notice(error.message,'error');
 }finally{
  button.disabled=false;
  button.textContent=oldText;
 }
}

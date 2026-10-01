
import {state,api,notice,needAdmin} from './core.js';

export async function startBuild(payload,button){
 if(!needAdmin()||!state.project)return;
 const project=state.project;
 button.disabled=true;
 try{
  const job=await api('/api/projects/'+project+'/builds',{
   method:'POST',
   headers:{'Content-Type':'application/json'},
   body:JSON.stringify(payload)
  });
  if(project!==state.project)return;
  window.dispatchEvent(new CustomEvent('build-started',{detail:{job}}));
  notice('ILS 发布任务已启动；构建、上传和 TestFlight 提交状态会持续更新。');
 }catch(error){
  notice(error.message);
 }finally{
  button.disabled=false;
 }
}

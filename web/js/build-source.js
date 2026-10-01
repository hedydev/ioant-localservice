
import {$,state,api,escapeHTML,notice,needAdmin} from './core.js';
import {buildState} from './build-state.js';
import {startBuild} from './build-actions.js';

function releaseMarkdown(markdown){
 const inline=value=>escapeHTML(value).replace(/`([^`]+)`/g,'<code>$1</code>').replace(/\*\*([^*]+)\*\*/g,'<strong>$1</strong>');
 let code=false,list=false,out='';
 for(const line of markdown.split('\n')){
  if(line.startsWith('```')){
   if(list){out+='</ul>';list=false;}
   out+=code?'</code></pre>':'<pre><code>';
   code=!code;
   continue;
  }
  if(code){out+=escapeHTML(line)+'\n';continue;}
  const item=line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)$/);
  if(item){
   if(!list){out+='<ul>';list=true;}
   out+='<li>'+inline(item[1])+'</li>';
   continue;
  }
  if(list){out+='</ul>';list=false;}
  const heading=line.match(/^(#{1,6})\s+(.*)$/);
  if(heading)out+='<h3>'+inline(heading[2])+'</h3>';
  else if(line.trim())out+='<p>'+inline(line)+'</p>';
 }
 return out+(list?'</ul>':'')+(code?'</code></pre>':'');
}

function renderCompatibilityScripts(){
 const container=$('#build-scripts');
 const source=buildState.source;
 if(!source||source.configured===false){
  container.innerHTML='';
  return;
 }
 const scripts=source.scripts||[];
 container.innerHTML='<div class="compatibility-block"><h3>项目内发布脚本 <span class="meta">兼容模式</span></h3>'+
  (scripts.length?scripts.map(script=>
   '<article class="release">'+
    '<div class="release-top"><div><strong>'+escapeHTML(script.title)+'</strong><div class="meta">'+escapeHTML(script.path)+'</div></div>'+
    '<button data-run-script="'+escapeHTML(script.path)+'" '+((!script.ready||source.blocker)?'disabled':'')+'>拉取并发布</button></div>'+
    '<p class="meta">'+escapeHTML(script.reason||'已识别项目脚本与说明；实际产物以执行结果为准。')+'</p>'+
    (script.markdown?'<details><summary>查看打包说明</summary><div class="release-markdown">'+releaseMarkdown(script.markdown)+'</div></details>':'')+
   '</article>'
  ).join(''):'<p class="meta">项目中没有 release*.sh。新项目优先使用 ILS Release Profile。</p>')+
 '</div>';
}

function resetSourceUI(){
 buildState.source=null;
 $('#source-form').reset();
 $('#source-form [name=branch]').value='main';
 $('#source-info').textContent='关联 Git 项目后，可以配置 ILS Release Profile。';
 $('#build-scripts').innerHTML='';
}

export async function loadBuildSource(){
 if(!state.project||!state.admin)return;
 const project=state.project;
 $('#source-info').textContent='正在读取 Git 项目状态…';
 try{
  const source=await api('/api/projects/'+project+'/build-source');
  if(project!==state.project)return;
  buildState.source=source;
  if(source.configured===false){
   $('#source-info').textContent='尚未关联本地目录。先关联项目目录，再创建 ILS Release Profile。';
   renderCompatibilityScripts();
   window.dispatchEvent(new CustomEvent('build-source-loaded',{detail:{source}}));
   return;
  }
  $('#source-form [name=path]').value=source.path;
  $('#source-form [name=branch]').value=source.branch;
  $('#source-info').textContent=
   source.path+'\n分支 '+(source.current_branch||'detached')+' → '+(source.upstream||'未配置 upstream')+'\n'+
   (source.remote||'')+'\n'+
   (source.blocker||'工作目录干净，可由 ILS 拉取、构建并发布。');
  renderCompatibilityScripts();
  window.dispatchEvent(new CustomEvent('build-source-loaded',{detail:{source}}));
 }catch(error){
  if(project===state.project){
   buildState.source=null;
   $('#source-info').textContent=error.message;
   renderCompatibilityScripts();
  }
 }
}

export function initBuildSource(){
 $('#scan-builds').onclick=()=>{
  if(needAdmin())loadBuildSource();
 };

 $('#source-form').onsubmit=async event=>{
  event.preventDefault();
  if(!needAdmin()||!state.project)return;
  const button=event.target.querySelector('[type=submit]');
  button.disabled=true;
  try{
   await api('/api/projects/'+state.project+'/build-source',{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify(Object.fromEntries(new FormData(event.target)))
   });
   await loadBuildSource();
   notice('项目目录已关联，现在可以创建 ILS Release Profile。');
  }catch(error){
   notice(error.message);
  }finally{
   button.disabled=false;
  }
 };

 $('#choose-project-folder').onclick=async()=>{
  if(!needAdmin())return;
  const button=$('#choose-project-folder');
  const project=state.project;
  button.disabled=true;
  button.textContent='等待 Mac 选择…';
  $('#source-info').textContent='请在运行 ILS 的 Mac 上选择项目文件夹；首次使用可能需要允许控制 Finder。';
  try{
   const result=await api('/api/local/select-folder',{method:'POST'});
   if(state.project!==project)return;
   if(result.cancelled){
    $('#source-info').textContent='已取消选择，原路径未更改。';
    return;
   }
   $('#source-form [name=path]').value=result.path;
   $('#source-info').textContent='已选择 Git 项目：'+result.path+'。点击“关联目录”后即可保存。';
   $('#source-form [type=submit]').focus();
  }catch(error){
   if(state.project===project)$('#source-info').textContent=error.message;
  }finally{
   button.disabled=false;
   button.textContent='选择文件夹…';
  }
 };

 $('#build-scripts').onclick=event=>{
  const button=event.target.closest('[data-run-script]');
  if(button)startBuild({script:button.dataset.runScript},button);
 };

 window.addEventListener('project-changed',()=>{
  resetSourceUI();
  if(state.admin&&state.view==='builds')loadBuildSource();
 });
 window.addEventListener('admin-loaded',()=>{
  if(state.view==='builds')loadBuildSource();
 });
 window.addEventListener('admin-cleared',resetSourceUI);
 window.addEventListener('view-changed',event=>{
  if(event.detail.view==='builds'&&state.admin)loadBuildSource();
 });
}

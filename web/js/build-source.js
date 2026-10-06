
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
    '<button data-run-script="'+escapeHTML(script.path)+'" '+(!script.ready?'disabled':'')+'>运行脚本</button></div>'+
    '<p class="meta">'+escapeHTML(script.reason||'直接在当前本地 checkout 中运行；ILS 不会自动 pull、stash、reset、clean 或切换分支。')+'</p>'+
    (script.markdown?'<details><summary>查看打包说明</summary><div class="release-markdown">'+releaseMarkdown(script.markdown)+'</div></details>':'')+
   '</article>'
  ).join(''):'<p class="meta">项目中没有 release*.sh。新项目优先使用 ILS Release Profile。</p>')+
 '</div>';
}

function populateSourceForm(source=null){
 const form=$('#source-form');
 form.reset();
 form.elements.branch.value=source?.current_branch||source?.branch||'main';
 form.elements.path.value=source?.path||'';
 form.querySelector('.form-error').textContent='';
 $('#source-dialog-status').textContent='';
}

function renderSourceSummary(source){
 const button=$('#configure-source');
 if(!source||source.configured===false){
  button.textContent='配置项目来源';
  $('#source-info').innerHTML='<div class="persistent-config-empty"><strong>尚未关联本地 Git 项目</strong><span>关联本地目录后，ILS 直接构建当前 checkout；不会自动同步远程仓库。</span></div>';
  return;
 }
 button.textContent='修改项目来源';
 const branch=source.current_branch||'detached HEAD';
 const head=(source.head||'').slice(0,12)||'—';
 const status=source.dirty?'有本地改动 · 允许构建':'工作目录 clean · 允许构建';
 const upstream=source.upstream||'未配置';
 $('#source-info').innerHTML='<div class="persistent-config-grid">'+
  '<span><small>项目目录</small><strong>'+escapeHTML(source.path||'—')+'</strong></span>'+
  '<span><small>当前分支</small><strong>'+escapeHTML(branch)+'</strong></span>'+
  '<span><small>HEAD / upstream</small><strong>'+escapeHTML(head+' · '+upstream)+'</strong></span>'+
  '<span><small>状态</small><strong class="'+(source.dirty?'config-warning':'config-ok')+'">'+escapeHTML(status)+'</strong></span>'+
 '</div>'+
 (source.remote?'<div class="persistent-config-detail">'+escapeHTML(source.remote)+'</div>':'')+
 '<div class="persistent-config-detail">构建记录会保存当前 branch、HEAD、dirty、upstream 和 remote；本地与远程不一致不会阻止构建。</div>';
}

function resetSourceUI(){
 buildState.source=null;
 populateSourceForm();
 renderSourceSummary(null);
 $('#build-scripts').innerHTML='';
}

export async function loadBuildSource(){
 if(!state.project||!state.admin)return;
 const project=state.project;
 $('#source-info').innerHTML='<div class="persistent-config-empty">正在读取 Git 项目状态…</div>';
 try{
  const source=await api('/api/projects/'+project+'/build-source');
  if(project!==state.project)return;
  buildState.source=source;
  populateSourceForm(source.configured===false?null:source);
  renderSourceSummary(source);
  renderCompatibilityScripts();
  window.dispatchEvent(new CustomEvent('build-source-loaded',{detail:{source}}));
 }catch(error){
  if(project===state.project){
   buildState.source=null;
   $('#source-info').innerHTML='<div class="persistent-config-empty config-error">'+escapeHTML(error.message)+'</div>';
   renderCompatibilityScripts();
  }
 }
}

export function initBuildSource(){
 const form=$('#source-form');
 const dialogHelp=$('#source-dialog > form > p');
 if(dialogHelp)dialogHelp.textContent='这里只关联本地 Git 项目目录。ILS 构建点击时实际存在的 checkout：任意分支、dirty 或本地/远程不一致都允许；不会 pull、切分支、stash、reset 或 clean。';
 const branchLabel=form.elements.branch?.closest('label');
 if(branchLabel){
  const textNode=[...branchLabel.childNodes].find(node=>node.nodeType===Node.TEXT_NODE);
  if(textNode)textNode.nodeValue='参考分支 ';
  if(!branchLabel.querySelector('.meta'))branchLabel.insertAdjacentHTML('beforeend','<span class="meta">仅作为项目元数据/导入默认值，不限制实际构建分支。</span>');
 }

 $('#scan-builds').onclick=()=>{
  if(needAdmin())loadBuildSource();
 };

 $('#configure-source').onclick=()=>{
  if(!needAdmin())return;
  if(!state.project)return notice('请先选择项目');
  populateSourceForm(buildState.source?.configured===false?null:buildState.source);
  $('#source-dialog').showModal();
 };

 form.onsubmit=async event=>{
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
   $('#source-dialog').close();
   await loadBuildSource();
   notice('项目来源已保存。ILS 将构建当前本地 checkout。');
  }catch(error){
   event.target.querySelector('.form-error').textContent=error.message;
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
  $('#source-dialog-status').textContent='请在运行 ILS 的 Mac 上选择项目文件夹；首次使用可能需要允许控制 Finder。';
  try{
   const result=await api('/api/local/select-folder',{method:'POST'});
   if(state.project!==project)return;
   if(result.cancelled){
    $('#source-dialog-status').textContent='已取消选择，原路径未更改。';
    return;
   }
   form.elements.path.value=result.path;
   $('#source-dialog-status').textContent='已选择：'+result.path;
   form.querySelector('[type=submit]').focus();
  }catch(error){
   if(state.project===project)$('#source-dialog-status').textContent=error.message;
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

// Small safe Markdown subset: headings, lists, paragraphs, code and emphasis.
// Raw HTML, images and links remain text; project documentation cannot run code.
function releaseMarkdown(markdown) {
 const inline = s => escape(s).replace(/`([^`]+)`/g,'<code>$1</code>').replace(/\*\*([^*]+)\*\*/g,'<strong>$1</strong>');
 let code=false, list=false, out='';
 for (const line of markdown.split('\n')) {
  if(line.startsWith('```')) {if(list){out+='</ul>';list=false;}out+=code?'</code></pre>':'<pre><code>';code=!code;continue;}
  if(code){out+=escape(line)+'\n';continue;}
  const item=line.match(/^\s*(?:[-*]|\d+\.)\s+(.*)$/);
  if(item){if(!list){out+='<ul>';list=true;}out+=`<li>${inline(item[1])}</li>`;continue;}
  if(list){out+='</ul>';list=false;}
  const heading=line.match(/^(#{1,6})\s+(.*)$/);
  if(heading){out+=`<h3>${inline(heading[2])}</h3>`;}else if(line.trim()){out+=`<p>${inline(line)}</p>`;}
 }
 return out+(list?'</ul>':'')+(code?'</code></pre>':'');
}
let buildProject=null, activeLog=null, buildLoading=false;
async function loadBuildSource(){
 if(!state.project||!state.token)return;
 const id=state.project;$('#source-info').textContent='正在读取 Git 与发布脚本…';
 try{const source=await api(`/api/projects/${id}/build-source`);if(state.project!==id)return;buildProject=id;
  if(source.configured===false){$('#source-info').textContent='尚未关联本地目录。';$('#build-scripts').innerHTML='';return;}
  $('#source-form [name=path]').value=source.path;$('#source-form [name=branch]').value=source.branch;
  $('#source-info').textContent=`${source.path}\n分支 ${source.current_branch||'detached'} → ${source.upstream||'未配置 upstream'}\n${source.remote||''}\n${source.blocker||'工作目录干净，可执行拉取与发布。'}`;
  $('#build-scripts').innerHTML=source.scripts.length?source.scripts.map(s=>`<article class="release"><div class="release-top"><div><strong>${escape(s.title)}</strong><div class="meta">${escape(s.path)}</div></div><button data-run-script="${escape(s.path)}" ${(!s.ready||source.blocker)?'disabled':''}>拉取并发布</button></div><p class="meta">${escape(s.reason||'已识别脚本与说明；实际产物以执行结果为准。')}</p>${s.markdown?`<details><summary>查看打包说明</summary><div class="release-markdown">${releaseMarkdown(s.markdown)}</div></details>`:''}</article>`).join(''):'<p>没有找到已纳入 Git 的 release*.sh。请让项目 AI 添加脚本及同名 .md，提交到发布分支。</p>';
 }catch(e){if(state.project===id){$('#source-info').textContent=e.message;$('#build-scripts').innerHTML='';}}
}
async function loadBuildJobs(){
 if(!state.project||!state.token||buildLoading)return;buildLoading=true;
 const id=state.project;
 try{const jobs=await api(`/api/projects/${id}/builds`);if(state.project!==id)return;
  $('#build-jobs').innerHTML=jobs.length?'<h3>构建记录</h3>'+jobs.map(j=>`<article class="build-job"><div><strong>${escape(j.script)}</strong> <span class="badge">${escape(j.status==='running'?(j.stage==='pull'?'正在拉取':'正在构建 / 发布'):j.status==='succeeded'?'发布成功':'未完成')}</span><div class="meta">${date(j.created_at)} · ${escape((j.commit||'').slice(0,12))}</div>${j.error?`<p>${escape(j.error)}</p>`:''}<div class="meta">关联安装包：${j.release_ids.length} 个</div>${j.release_ids.map(r=>`<a class="build-artifact" href="/api/releases/${escape(r)}/download">下载 ${escape(r.slice(0,8))}</a>`).join(' ')}</div><button data-build-log="${escape(j.id)}">查看日志</button></article>`).join(''):'';
  if(activeLog){const data=await api(`/api/builds/${activeLog}/log`);if(state.project!==id)return;$('#build-log').textContent=data.log;$('#build-log').hidden=false;}
 }catch(e){if(state.project===id)$('#source-info').textContent=e.message;}finally{buildLoading=false;}
}
$('#scan-builds').onclick=()=>{if(needAdmin()){loadBuildSource();loadBuildJobs();}};
$('#source-form').onsubmit=async e=>{e.preventDefault();if(!needAdmin())return;if(!state.project)return notice('请先创建项目');const b=e.target.querySelector('[type=submit]');b.disabled=true;try{await api(`/api/projects/${state.project}/build-source`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(Object.fromEntries(new FormData(e.target)))});await loadBuildSource();notice('项目目录已关联。');}catch(e){notice(e.message);}finally{b.disabled=false;}};
$('#build-scripts').onclick=async e=>{const b=e.target.closest('[data-run-script]');if(!b)return;if(!needAdmin())return;b.disabled=true;const id=state.project;try{const job=await api(`/api/projects/${id}/builds`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({script:b.dataset.runScript})});if(id!==state.project)return;activeLog=job.id;await loadBuildJobs();notice('任务已启动，构建成功后安装包会自动出现在版本列表。');}catch(e){notice(e.message);}finally{b.disabled=false;}};
$('#build-jobs').onclick=e=>{const b=e.target.closest('[data-build-log]');if(b){activeLog=b.dataset.buildLog;loadBuildJobs();}};
function clearBuildPanel(){buildProject=null;activeLog=null;$('#build-scripts').innerHTML='';$('#build-jobs').innerHTML='';$('#build-log').hidden=true;$('#source-form').reset();$('#source-info').textContent='关联已有 Git 项目，识别 release*.sh 与同名 Markdown 说明。';loadBuildSource();loadBuildJobs();}
window.addEventListener('project-changed',clearBuildPanel);
window.addEventListener('admin-loaded',()=>{loadBuildSource();loadBuildJobs();});
setInterval(()=>{if(!document.hidden&&state.token)loadBuildJobs();},3000);

$('#choose-project-folder').onclick=async()=>{
 if(!needAdmin())return;
 const button=$('#choose-project-folder');const project=state.project;
 button.disabled=true;button.textContent='等待 Mac 选择…';
 $('#source-info').textContent='请在运行服务的 Mac 上选择项目文件夹；首次使用可能需要允许控制 Finder。';
 try{
  const result=await api('/api/local/select-folder',{method:'POST'});
  if(state.project!==project)return;
  if(result.cancelled){$('#source-info').textContent='已取消选择，原路径未更改。';return;}
  $('#source-form [name=path]').value=result.path;
  $('#source-info').textContent='已选择 Git 项目：'+result.path+'。点击“关联目录”读取发布脚本。';
  $('#source-form [type=submit]').focus();
 }catch(e){if(state.project===project)$('#source-info').textContent=e.message;}
 finally{button.disabled=false;button.textContent='选择文件夹…';}
};

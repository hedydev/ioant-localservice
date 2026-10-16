export const projectViews=new Set(['overview','builds','releases','ios','automation']);
export const globalViews=new Set(['services','apple','settings']);
export const validViews=new Set([...globalViews,...projectViews]);

export function readRoute(){
 const raw=location.hash.replace(/^#/,'');
 const global=raw.replace(/^\//,'');
 if(globalViews.has(global))return {kind:'global',project:null,view:global,legacy:false};
 const match=raw.match(/^\/?projects\/([^/]+)\/([^/]+)$/);
 if(match){
  let project='';
  let view='overview';
  try{project=decodeURIComponent(match[1]);}catch{project=match[1];}
  try{view=decodeURIComponent(match[2]);}catch{view=match[2];}
  if(!projectViews.has(view))view='overview';
  return {kind:'project',project,view,legacy:false};
 }
 const legacy=raw.replace(/^\//,'');
 if(validViews.has(legacy)){
  return globalViews.has(legacy)
   ?{kind:'global',project:null,view:legacy,legacy:true}
   :{kind:'project',project:null,view:legacy,legacy:true};
 }
 return {kind:'project',project:null,view:'overview',legacy:true};
}

export function routeHash(project,view){
 if(globalViews.has(view))return '#/'+view;
 const safeView=projectViews.has(view)?view:'overview';
 if(!project)return '#'+safeView;
 return '#/projects/'+encodeURIComponent(project)+'/'+encodeURIComponent(safeView);
}

export function writeRoute(project,view,{replace=false}={}){
 const target=routeHash(project,view);
 if(location.hash===target)return false;
 if(replace)history.replaceState(null,'',target);
 else location.hash=target;
 return true;
}

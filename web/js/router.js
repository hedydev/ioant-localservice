export const projectViews=new Set(['overview','builds','releases','ios','automation']);
export const validViews=new Set(['services',...projectViews]);

export function readRoute(){
 const raw=location.hash.replace(/^#/,'');
 if(raw==='services'||raw==='/services')return {kind:'global',project:null,view:'services',legacy:false};
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
  return legacy==='services'
   ?{kind:'global',project:null,view:'services',legacy:true}
   :{kind:'project',project:null,view:legacy,legacy:true};
 }
 return {kind:'project',project:null,view:'overview',legacy:true};
}

export function routeHash(project,view){
 if(view==='services')return '#/services';
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

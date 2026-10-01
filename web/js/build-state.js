
export const buildState={
 source:null,
 profiles:[],
 activeLog:null,
 collapsedLogs:new Set(),
 jobsLoading:false,
 followLog:true,
 logLoaded:false,
 logScrollTop:0
};

export function resetBuildState(){
 buildState.source=null;
 buildState.profiles=[];
 buildState.activeLog=null;
 buildState.collapsedLogs.clear();
 buildState.jobsLoading=false;
 buildState.followLog=true;
 buildState.logLoaded=false;
 buildState.logScrollTop=0;
}


export const buildState={
 source:null,
 profiles:[],
 activeLog:null,
 jobsLoading:false,
 followLog:true
};

export function resetBuildState(){
 buildState.source=null;
 buildState.profiles=[];
 buildState.activeLog=null;
 buildState.jobsLoading=false;
 buildState.followLog=true;
}
